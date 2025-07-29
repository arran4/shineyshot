package main

import (
	"bytes"
	"flag"
	"fmt"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

const (
	tabHeight    = 24
	bottomHeight = 24
)

var toolbarWidth = 48

type Tool int

const (
	ToolMove Tool = iota
	ToolCrop
	ToolDraw
	ToolCircle
	ToolLine
	ToolArrow
	ToolNumber
)

type Tab struct {
	Image *image.RGBA
	Title string
	// Offset is stored in image coordinates so it is independent of zoom.
	Offset     image.Point
	Zoom       float64
	NextNumber int
	WidthIdx   int
}

const handleSize = 8

type cropAction int

const (
	cropNone cropAction = iota
	cropMove
	cropResizeTL
	cropResizeT
	cropResizeTR
	cropResizeR
	cropResizeBR
	cropResizeB
	cropResizeBL
	cropResizeL
)

var palette = []color.RGBA{
	{0, 0, 0, 255},       // black
	{255, 255, 255, 255}, // white
	{255, 0, 0, 255},
	{0, 255, 0, 255},
	{0, 0, 255, 255},
	{255, 255, 0, 255},
	{0, 255, 255, 255},
	{255, 0, 255, 255},
	{128, 0, 0, 255},
	{0, 128, 0, 255},
	{0, 0, 128, 255},
	{128, 128, 0, 255},
	{0, 128, 128, 255},
	{128, 0, 128, 255},
	{192, 192, 192, 255},
	{128, 128, 128, 255},
}

var checkerLight = color.RGBA{220, 220, 220, 255}
var checkerDark = color.RGBA{192, 192, 192, 255}

func fitZoom(img *image.RGBA, winW, winH int) float64 {
	availW := winW - toolbarWidth
	availH := winH - tabHeight - bottomHeight
	zx := float64(availW) / float64(img.Bounds().Dx())
	zy := float64(availH) / float64(img.Bounds().Dy())
	if zx < zy {
		return zx
	}
	return zy
}

// imageRect returns the destination rectangle for drawing the image. It anchors
// the canvas origin just below the toolbar instead of centering it so that the
// image position remains stable even when the canvas grows or shrinks.
func imageRect(img *image.RGBA, winW, winH int, zoom float64) image.Rectangle {
	w := int(float64(img.Bounds().Dx()) * zoom)
	h := int(float64(img.Bounds().Dy()) * zoom)
	x0 := toolbarWidth
	y0 := tabHeight
	return image.Rect(x0, y0, x0+w, y0+h)
}

// drawCheckerboard fills rect of dst with a checkerboard pattern of the given
// colors. size controls the checker square size.
func drawCheckerboard(dst *image.RGBA, rect image.Rectangle, size int, light, dark color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if ((x/size)+(y/size))%2 == 0 {
				dst.Set(x, y, light)
			} else {
				dst.Set(x, y, dark)
			}
		}
	}
}

var widths = []int{1, 2, 4, 6, 8}
var numberSizes = []int{8, 12, 16, 20, 24}

func numberBoxHeight(size int) int {
	h := 2*size + 4
	if h < 16 {
		return 16
	}
	return h
}

func drawTabs(dst *image.RGBA, tabs []Tab, current int) {
	// background for title area
	draw.Draw(dst, image.Rect(0, 0, toolbarWidth, tabHeight),
		&image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)

	// program title in the top-left corner
	title := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13,
		Dot: fixed.P(4, 16)}
	title.DrawString("ShineyShot")

	x := toolbarWidth
	for i, t := range tabs {
		col := color.RGBA{200, 200, 200, 255}
		if i == current {
			col = color.RGBA{150, 150, 150, 255}
		}
		rect := image.Rect(x, 0, x+80, tabHeight)
		draw.Draw(dst, rect, &image.Uniform{col}, image.Point{}, draw.Src)
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.Black,
			Face: basicfont.Face7x13,
			Dot:  fixed.P(x+4, 16),
		}
		d.DrawString(t.Title)
		x += 80
	}
	// fill remainder of bar
	draw.Draw(dst, image.Rect(x, 0, dst.Bounds().Dx(), tabHeight),
		&image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
}

func drawShortcuts(dst *image.RGBA, width, height int, tool Tool, z float64) {
	rect := image.Rect(0, height-bottomHeight, width, height)
	draw.Draw(dst, rect, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
	zoomStr := fmt.Sprintf("+/-:zoom (%.0f%%)", z*100)
	shortcuts := []string{"N:new", zoomStr, "D:delete", "C:copy", "S:save", "Q:quit"}
	if tool == ToolCrop {
		shortcuts = append(shortcuts, "Enter:crop", "Ctrl+Enter:new tab", "Esc:cancel")
	}
	x := toolbarWidth + 4
	y := height - bottomHeight + 16
	for _, sc := range shortcuts {
		d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
		d.DrawString(sc)
		x += d.MeasureString(sc).Ceil() + 20
	}
}

func drawToolbar(dst *image.RGBA, tool Tool, colIdx, widthIdx, numberIdx int) {
	y := tabHeight
	tools := []string{"Move", "Crop", "Draw", "Circle", "Line", "Arrow", "Num"}
	for i, name := range tools {
		c := color.RGBA{200, 200, 200, 255}
		if Tool(i) == tool {
			c = color.RGBA{150, 150, 150, 255}
		}
		rect := image.Rect(0, y, toolbarWidth, y+24)
		draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
		d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+16)}
		d.DrawString(name)
		y += 24
	}

	// color palette below tools
	y += 4
	x := 4
	for i, p := range palette {
		rect := image.Rect(x, y, x+16, y+16)
		draw.Draw(dst, rect, &image.Uniform{p}, image.Point{}, draw.Src)
		if i == colIdx {
			draw.Draw(dst, rect, &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Over)
			drawLine(dst, rect.Min.X, rect.Min.Y, rect.Max.X-1, rect.Min.Y, color.White, 1)
			drawLine(dst, rect.Min.X, rect.Min.Y, rect.Min.X, rect.Max.Y-1, color.White, 1)
			drawLine(dst, rect.Max.X-1, rect.Min.Y, rect.Max.X-1, rect.Max.Y-1, color.White, 1)
			drawLine(dst, rect.Min.X, rect.Max.Y-1, rect.Max.X-1, rect.Max.Y-1, color.White, 1)
		}
		x += 18
		if x+16 > toolbarWidth {
			x = 4
			y += 18
		}
	}

	if tool == ToolDraw || tool == ToolCircle || tool == ToolLine || tool == ToolArrow {
		y += 4
		col := palette[colIdx]
		for i, w := range widths {
			rect := image.Rect(0, y, toolbarWidth, y+16)
			c := color.RGBA{200, 200, 200, 255}
			if i == widthIdx {
				c = color.RGBA{150, 150, 150, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
			d.DrawString(fmt.Sprintf("%d", w))
			lineY := y + 8
			drawLine(dst, 30, lineY, toolbarWidth-4, lineY, col, w)
			y += 16
		}
	}
	if tool == ToolNumber {
		y += 4
		col := palette[colIdx]
		for i, s := range numberSizes {
			h := numberBoxHeight(s)
			rect := image.Rect(0, y, toolbarWidth, y+h)
			c := color.RGBA{200, 200, 200, 255}
			if i == numberIdx {
				c = color.RGBA{150, 150, 150, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
			d.DrawString(fmt.Sprintf("%d", s))
			drawFilledCircle(dst, (toolbarWidth+30)/2, y+h/2, s, col)
			y += h
		}
	}
}

func setThickPixel(img *image.RGBA, x, y, thick int, col color.Color) {
	r := thick / 2
	for dx := -r; dx <= r; dx++ {
		for dy := -r; dy <= r; dy++ {
			px := x + dx
			py := y + dy
			if image.Pt(px, py).In(img.Bounds()) {
				img.Set(px, py, col)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	dx := math.Abs(float64(x1 - x0))
	dy := math.Abs(float64(y1 - y0))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy
	for {
		setThickPixel(img, x0, y0, thick, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, r int, col color.Color, thick int) {
	x := r
	y := 0
	err := 0
	for x >= y {
		pts := [][2]int{{x, y}, {y, x}, {-y, x}, {-x, y}, {-x, -y}, {-y, -x}, {y, -x}, {x, -y}}
		for _, p := range pts {
			px := cx + p[0]
			py := cy + p[1]
			setThickPixel(img, px, py, thick, col)
		}
		y++
		if err <= 0 {
			err += 2*y + 1
		} else {
			x--
			err -= 2*x + 1
		}
	}
}

func drawArrow(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	drawLine(img, x0, y0, x1, y1, col, thick)
	angle := math.Atan2(float64(y1-y0), float64(x1-x0))
	size := float64(6 + thick*2)
	a1 := angle + math.Pi/6
	a2 := angle - math.Pi/6
	x2 := x1 - int(math.Cos(a1)*size)
	y2 := y1 - int(math.Sin(a1)*size)
	x3 := x1 - int(math.Cos(a2)*size)
	y3 := y1 - int(math.Sin(a2)*size)
	drawLine(img, x1, y1, x2, y2, col, thick)
	drawLine(img, x1, y1, x3, y3, col, thick)
}

func drawFilledCircle(img *image.RGBA, cx, cy, r int, col color.Color) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px := cx + dx
				py := cy + dy
				if image.Pt(px, py).In(img.Bounds()) {
					img.Set(px, py, col)
				}
			}
		}
	}
}

// drawNumberBox draws a numbered annotation with the circle centred at (cx, cy).
// size controls the radius of the circle.
func drawNumberBox(img *image.RGBA, cx, cy, num int, col color.Color, size int) {
	r := size
	drawFilledCircle(img, cx, cy, r, col)

	cr, cg, cb, _ := col.RGBA()
	brightness := 0.299*float64(cr>>8) + 0.587*float64(cg>>8) + 0.114*float64(cb>>8)
	textCol := color.Black
	if brightness < 128 {
		textCol = color.White
	}

	text := fmt.Sprintf("%d", num)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textCol),
		Face: basicfont.Face7x13,
	}
	w := d.MeasureString(text).Ceil()
	d.Dot = fixed.P(cx-w/2, cy+4)
	d.DrawString(text)
}

// ensureCanvasContains expands the tab's image so that rect (in image coordinates)
// fits within it. Existing image content keeps its on-screen position by
// adjusting the tab's offset when expansion occurs.
func ensureCanvasContains(t *Tab, rect image.Rectangle) image.Point {
	b := t.Image.Bounds()
	minX := 0
	if rect.Min.X < 0 {
		minX = rect.Min.X
	}
	minY := 0
	if rect.Min.Y < 0 {
		minY = rect.Min.Y
	}
	maxX := b.Max.X
	if rect.Max.X > maxX {
		maxX = rect.Max.X
	}
	maxY := b.Max.Y
	if rect.Max.Y > maxY {
		maxY = rect.Max.Y
	}
	if minX == 0 && minY == 0 && maxX == b.Max.X && maxY == b.Max.Y {
		return image.Point{}
	}
	newW := maxX - minX
	newH := maxY - minY
	newImg := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.Draw(newImg, b.Add(image.Pt(-minX, -minY)), t.Image, image.Point{}, draw.Src)
	t.Image = newImg
	t.Offset = t.Offset.Add(image.Pt(minX, minY))
	return image.Pt(minX, minY)
}

func drawDashedLine(img *image.RGBA, x0, y0, x1, y1, dash, thickness int, c1, c2 color.Color) {
	horiz := y0 == y1
	length := x1 - x0
	if !horiz {
		length = y1 - y0
	}
	if length < 0 {
		length = -length
	}
	for i := 0; i <= length; i += dash * 2 {
		for j := 0; j < dash && i+j <= length; j++ {
			col := c1
			if horiz {
				for t := 0; t < thickness; t++ {
					if x0 < x1 {
						img.Set(x0+i+j, y0+t, col)
					} else {
						img.Set(x0-i-j, y0+t, col)
					}
				}
			} else {
				for t := 0; t < thickness; t++ {
					if y0 < y1 {
						img.Set(x0+t, y0+i+j, col)
					} else {
						img.Set(x0+t, y0-i-j, col)
					}
				}
			}
		}
		for j := 0; j < dash && i+dash+j <= length; j++ {
			col := c2
			if horiz {
				for t := 0; t < thickness; t++ {
					if x0 < x1 {
						img.Set(x0+i+dash+j, y0+t, col)
					} else {
						img.Set(x0-i-dash-j, y0+t, col)
					}
				}
			} else {
				for t := 0; t < thickness; t++ {
					if y0 < y1 {
						img.Set(x0+t, y0+i+dash+j, col)
					} else {
						img.Set(x0+t, y0-i-dash-j, col)
					}
				}
			}
		}
	}
}

func drawDashedRect(img *image.RGBA, rect image.Rectangle, dash, thickness int, c1, c2 color.Color) {
	drawDashedLine(img, rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Max.X, rect.Min.Y, rect.Max.X, rect.Max.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Max.X, rect.Max.Y, rect.Min.X, rect.Max.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Min.X, rect.Max.Y, rect.Min.X, rect.Min.Y, dash, thickness, c1, c2)
}

func drawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thick int) {
	drawLine(img, rect.Min.X, rect.Min.Y, rect.Max.X-1, rect.Min.Y, col, thick)
	drawLine(img, rect.Max.X-1, rect.Min.Y, rect.Max.X-1, rect.Max.Y-1, col, thick)
	drawLine(img, rect.Max.X-1, rect.Max.Y-1, rect.Min.X, rect.Max.Y-1, col, thick)
	drawLine(img, rect.Min.X, rect.Max.Y-1, rect.Min.X, rect.Min.Y, col, thick)
}

func cropHandleRects(rect image.Rectangle) []image.Rectangle {
	hs := handleSize / 2
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2
	return []image.Rectangle{
		image.Rect(rect.Min.X-hs, rect.Min.Y-hs, rect.Min.X+hs, rect.Min.Y+hs), // tl
		image.Rect(cx-hs, rect.Min.Y-hs, cx+hs, rect.Min.Y+hs),                 // t
		image.Rect(rect.Max.X-hs, rect.Min.Y-hs, rect.Max.X+hs, rect.Min.Y+hs), // tr
		image.Rect(rect.Max.X-hs, cy-hs, rect.Max.X+hs, cy+hs),                 // r
		image.Rect(rect.Max.X-hs, rect.Max.Y-hs, rect.Max.X+hs, rect.Max.Y+hs), // br
		image.Rect(cx-hs, rect.Max.Y-hs, cx+hs, rect.Max.Y+hs),                 // b
		image.Rect(rect.Min.X-hs, rect.Max.Y-hs, rect.Min.X+hs, rect.Max.Y+hs), // bl
		image.Rect(rect.Min.X-hs, cy-hs, rect.Min.X+hs, cy+hs),                 // l
	}
}

// cropImage returns a copy of the given rectangle from img. If rect extends
// outside img, the missing areas are left transparent so the canvas can grow.
func cropImage(img *image.RGBA, rect image.Rectangle) *image.RGBA {
	if rect.Empty() {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	src := rect.Intersect(img.Bounds())
	if !src.Empty() {
		draw.Draw(out, src.Sub(rect.Min), img, src.Min, draw.Src)
	}
	return out
}

func captureScreenshot() (*image.RGBA, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := map[string]dbus.Variant{
		"interactive": dbus.MakeVariant(false),
	}
	var handle dbus.ObjectPath
	call := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts)
	if call.Err != nil {
		return nil, call.Err
	}
	if err := call.Store(&handle); err != nil {
		return nil, err
	}

	sigc := make(chan *dbus.Signal, 1)
	conn.Signal(sigc)
	rule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", handle)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return nil, err
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule)

	for sig := range sigc {
		if sig.Path == handle && sig.Name == "org.freedesktop.portal.Request.Response" {
			if len(sig.Body) >= 2 {
				res := sig.Body[1].(map[string]dbus.Variant)
				if uriVar, ok := res["uri"]; ok {
					uri := uriVar.Value().(string)
					path := strings.TrimPrefix(uri, "file://")
					f, err := os.Open(path)
					if err != nil {
						return nil, err
					}
					defer f.Close()
					img, err := png.Decode(f)
					if err != nil {
						return nil, err
					}
					rgba := image.NewRGBA(img.Bounds())
					draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
					return rgba, nil
				}
			}
			break
		}
	}
	return nil, fmt.Errorf("screenshot failed")
}

func main() {
	screenshot := flag.String("screenshot", "", "existing screenshot to annotate")
	output := flag.String("output", "annotated.png", "output file path")
	flag.Parse()

	var rgba *image.RGBA
	if *screenshot == "" {
		var err error
		rgba, err = captureScreenshot()
		if err != nil {
			log.Fatalf("capture screenshot: %v", err)
		}
	} else {
		f, err := os.Open(*screenshot)
		if err != nil {
			log.Fatalf("open screenshot: %v", err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			log.Fatalf("decode screenshot: %v", err)
		}
		rgba = image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	}

	// ensure the toolbar is wide enough for the program title
	d := &font.Drawer{Face: basicfont.Face7x13}
	titleWidth := d.MeasureString("ShineyShot").Ceil() + 8 // padding
	if titleWidth > toolbarWidth {
		toolbarWidth = titleWidth
	}

	driver.Main(func(s screen.Screen) {
		width := rgba.Bounds().Dx() + toolbarWidth
		height := rgba.Bounds().Dy() + tabHeight + bottomHeight
		w, err := s.NewWindow(&screen.NewWindowOptions{Width: width, Height: height})
		if err != nil {
			log.Fatalf("new window: %v", err)
		}
		defer w.Release()
		var bufs [2]screen.Buffer
		for i := 0; i < 2; i++ {
			var err error
			bufs[i], err = s.NewBuffer(image.Point{width, height})
			if err != nil {
				log.Fatalf("new buffer: %v", err)
			}
			defer bufs[i].Release()
		}
		bufIdx := 0

		tabs := []Tab{{Image: rgba, Title: "1", Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: 2}}
		current := 0

		var drawing bool
		var cropping bool
		var cropMode cropAction
		var moving bool
		var moveStart image.Point
		var moveOffset image.Point
		var last image.Point
		var cropStart image.Point
		var cropStartRect image.Rectangle
		var cropRect image.Rectangle
		var message string
		var messageUntil time.Time
		tool := ToolMove
		colorIdx := 2 // red
		numberIdx := 0

		col := palette[colorIdx]
		tabs[current].Zoom = fitZoom(rgba, width, height)

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case size.Event:
				width = e.WidthPx
				height = e.HeightPx
				for i := 0; i < 2; i++ {
					bufs[i].Release()
					var err error
					bufs[i], err = s.NewBuffer(image.Point{width, height})
					if err != nil {
						log.Fatalf("new buffer: %v", err)
					}
				}
				w.Send(paint.Event{})
			case paint.Event:
				b := bufs[bufIdx]
				bufIdx = 1 - bufIdx

				// clear background
				drawCheckerboard(b.RGBA(), b.Bounds(), 8, checkerLight, checkerDark)

				// draw the current tab’s image, scaled and offset
				img := tabs[current].Image
				zoom := tabs[current].Zoom
				base := imageRect(img, width, height, zoom)
				off := tabs[current].Offset
				dst := base.Add(image.Pt(
					int(float64(off.X)*zoom),
					int(float64(off.Y)*zoom),
				))
				xdraw.NearestNeighbor.Scale(b.RGBA(), dst, img, img.Bounds(), draw.Src, nil)

				// if cropping, draw the selection box (and handles)
				if tool == ToolCrop && (cropping || !cropRect.Empty()) {
					// build the rectangle in image‐coords
					sel := cropRect
					if cropping {
						sel = image.Rect(cropStart.X, cropStart.Y, cropStart.X, cropStart.Y).Union(sel)
					}
					// scale it to screen‐coords
					r := image.Rect(
						dst.Min.X+int(float64(sel.Min.X)*tabs[current].Zoom),
						dst.Min.Y+int(float64(sel.Min.Y)*tabs[current].Zoom),
						dst.Min.X+int(float64(sel.Max.X)*tabs[current].Zoom),
						dst.Min.Y+int(float64(sel.Max.Y)*tabs[current].Zoom),
					)
					// dashed outline
					drawDashedRect(b.RGBA(), r, 4, 2, color.White, color.Black)
					// little handles at each corner/edge
					for _, hr := range cropHandleRects(r) {
						draw.Draw(b.RGBA(), hr, &image.Uniform{color.White}, image.Point{}, draw.Src)
						drawRect(b.RGBA(), hr, color.Black, 1)
						drawDashedRect(b.RGBA(), hr, 2, 1, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
					}
				}

				// UI chrome
				drawTabs(b.RGBA(), tabs, current)
				drawToolbar(b.RGBA(), tool, colorIdx, tabs[current].WidthIdx, numberIdx)
				drawShortcuts(b.RGBA(), width, height, tool, tabs[current].Zoom)

				// transient message overlay
				if message != "" && time.Now().Before(messageUntil) {
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13}
					w := d.MeasureString(message).Ceil()
					px := toolbarWidth + (dst.Dx()-w)/2
					py := tabHeight + dst.Dy()
					// position and draw
					d.Dot = fixed.P(px, py)
					d.DrawString(message)
				}
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.Y) < tabHeight {
					idx := (int(e.X) - toolbarWidth) / 80
					if idx >= 0 && idx < len(tabs) {
						current = idx
						w.Send(paint.Event{})
					}
					continue
				}

				if e.Button == mouse.ButtonLeft && int(e.X) < toolbarWidth && int(e.Y) >= tabHeight {
					pos := int(e.Y) - tabHeight
					idx := pos / 24
					if idx < 7 {
						tool = Tool(idx)
						cropping = false
						w.Send(paint.Event{})
						continue
					}
					pos -= 7 * 24
					pos -= 4
					paletteCols := toolbarWidth / 18
					rows := (len(palette) + paletteCols - 1) / paletteCols
					paletteHeight := rows * 18
					if pos >= 0 && pos < paletteHeight {
						colX := (int(e.X) - 4) / 18
						colY := pos / 18
						cidx := colY*paletteCols + colX
						if cidx >= 0 && cidx < len(palette) {
							colorIdx = cidx
							col = palette[colorIdx]
							w.Send(paint.Event{})
							continue
						}
					}
					pos -= paletteHeight
					pos -= 4
					if (tool == ToolDraw || tool == ToolCircle || tool == ToolLine || tool == ToolArrow) && pos >= 0 {
						widx := pos / 16
						if widx >= 0 && widx < len(widths) {
							tabs[current].WidthIdx = widx
							w.Send(paint.Event{})
							continue
						}
					} else if tool == ToolNumber && pos >= 0 {
						rem := pos
						for i, s := range numberSizes {
							h := numberBoxHeight(s)
							if rem < h {
								numberIdx = i
								w.Send(paint.Event{})
								break
							}
							rem -= h
						}
						continue
					}
				}

				baseRect := imageRect(tabs[current].Image, width, height, tabs[current].Zoom)

				mx := int((float64(e.X)-float64(baseRect.Min.X))/tabs[current].Zoom) - tabs[current].Offset.X
				my := int((float64(e.Y)-float64(baseRect.Min.Y))/tabs[current].Zoom) - tabs[current].Offset.Y
				if e.Button == mouse.ButtonLeft {
					if e.Direction == mouse.DirPress {
						switch tool {
						case ToolMove:
							moving = true
							moveStart = image.Point{int(e.X), int(e.Y)}
							moveOffset = tabs[current].Offset
						case ToolCrop:
							p := image.Point{mx, my}
							action := cropNone
							for i, hr := range cropHandleRects(cropRect) {
								if p.In(hr) {
									action = cropAction(i + int(cropResizeTL))
									break
								}
							}
							if action == cropNone {
								if !cropRect.Empty() && p.In(cropRect) {
									action = cropMove
								} else {
									action = cropResizeBR
									cropRect = image.Rect(mx, my, mx, my)
								}
							}
							cropping = true
							cropMode = action
							cropStart = p
							cropStartRect = cropRect
							w.Send(paint.Event{})
						case ToolDraw:
							drawing = true
							last = image.Point{mx, my}
						case ToolCircle, ToolLine, ToolArrow, ToolNumber:
							drawing = true
							last = image.Point{mx, my}
						}
					} else if e.Direction == mouse.DirRelease {
						if cropping && tool == ToolCrop {
							dx := mx - cropStart.X
							dy := my - cropStart.Y
							r := cropStartRect
							switch cropMode {
							case cropMove:
								r = r.Add(image.Pt(dx, dy))
							case cropResizeTL:
								r.Min.X = cropStartRect.Min.X + dx
								r.Min.Y = cropStartRect.Min.Y + dy
							case cropResizeT:
								r.Min.Y = cropStartRect.Min.Y + dy
							case cropResizeTR:
								r.Min.Y = cropStartRect.Min.Y + dy
								r.Max.X = cropStartRect.Max.X + dx
							case cropResizeR:
								r.Max.X = cropStartRect.Max.X + dx
							case cropResizeBR:
								r.Max.X = cropStartRect.Max.X + dx
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeB:
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeBL:
								r.Min.X = cropStartRect.Min.X + dx
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeL:
								r.Min.X = cropStartRect.Min.X + dx
							}
							if r.Min.X > r.Max.X {
								r.Min.X, r.Max.X = r.Max.X, r.Min.X
							}
							if r.Min.Y > r.Max.Y {
								r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
							}
							cropRect = r
						}
						if drawing && tool != ToolCrop {
							switch tool {
							case ToolDraw:
								minX, minY := last.X, last.Y
								maxX, maxY := mx, my
								if mx < minX {
									minX = mx
								}
								if my < minY {
									minY = my
								}
								if last.X > maxX {
									maxX = last.X
								}
								if last.Y > maxY {
									maxY = last.Y
								}
								br := image.Rect(minX, minY, maxX, maxY).Inset(-widths[tabs[current].WidthIdx] - 2)
								shift := ensureCanvasContains(&tabs[current], br)
								last = last.Sub(shift)
								mx -= shift.X
								my -= shift.Y
								drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, widths[tabs[current].WidthIdx])
							case ToolCircle:
								r := int(math.Hypot(float64(mx-last.X), float64(my-last.Y)))
								br := image.Rect(last.X-r-widths[tabs[current].WidthIdx], last.Y-r-widths[tabs[current].WidthIdx], last.X+r+widths[tabs[current].WidthIdx]+1, last.Y+r+widths[tabs[current].WidthIdx]+1)
								shift := ensureCanvasContains(&tabs[current], br)
								last = last.Sub(shift)
								mx -= shift.X
								my -= shift.Y
								drawCircle(tabs[current].Image, last.X, last.Y, r, col, widths[tabs[current].WidthIdx])
							case ToolLine:
								minX, minY := last.X, last.Y
								maxX, maxY := mx, my
								if mx < minX {
									minX = mx
								}
								if my < minY {
									minY = my
								}
								if last.X > maxX {
									maxX = last.X
								}
								if last.Y > maxY {
									maxY = last.Y
								}
								br := image.Rect(minX, minY, maxX, maxY).Inset(-widths[tabs[current].WidthIdx] - 2)
								shift := ensureCanvasContains(&tabs[current], br)
								last = last.Sub(shift)
								mx -= shift.X
								my -= shift.Y
								drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, widths[tabs[current].WidthIdx])
							case ToolArrow:
								minX, minY := last.X, last.Y
								maxX, maxY := mx, my
								if mx < minX {
									minX = mx
								}
								if my < minY {
									minY = my
								}
								if last.X > maxX {
									maxX = last.X
								}
								if last.Y > maxY {
									maxY = last.Y
								}
								br := image.Rect(minX, minY, maxX, maxY).Inset(-widths[tabs[current].WidthIdx] - 10)
								shift := ensureCanvasContains(&tabs[current], br)
								last = last.Sub(shift)
								mx -= shift.X
								my -= shift.Y
								drawArrow(tabs[current].Image, last.X, last.Y, mx, my, col, widths[tabs[current].WidthIdx])
							case ToolNumber:
								s := numberSizes[numberIdx]
								br := image.Rect(mx-s, my-s, mx+s, my+s)
								shift := ensureCanvasContains(&tabs[current], br)
								mx -= shift.X
								my -= shift.Y
								drawNumberBox(tabs[current].Image, mx, my, tabs[current].NextNumber, col, s)
								tabs[current].NextNumber++
							}
							w.Send(paint.Event{})
						}
						if moving && tool == ToolMove {
							dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
							dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
							tabs[current].Offset = moveOffset.Add(image.Pt(dx, dy))
							w.Send(paint.Event{})
						}
						drawing = false
						cropping = false
						moving = false
					}
				}

				if cropping && tool == ToolCrop && e.Direction == mouse.DirNone {
					dx := mx - cropStart.X
					dy := my - cropStart.Y
					r := cropStartRect
					switch cropMode {
					case cropMove:
						r = r.Add(image.Pt(dx, dy))
					case cropResizeTL:
						r.Min.X = cropStartRect.Min.X + dx
						r.Min.Y = cropStartRect.Min.Y + dy
					case cropResizeT:
						r.Min.Y = cropStartRect.Min.Y + dy
					case cropResizeTR:
						r.Min.Y = cropStartRect.Min.Y + dy
						r.Max.X = cropStartRect.Max.X + dx
					case cropResizeR:
						r.Max.X = cropStartRect.Max.X + dx
					case cropResizeBR:
						r.Max.X = cropStartRect.Max.X + dx
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeB:
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeBL:
						r.Min.X = cropStartRect.Min.X + dx
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeL:
						r.Min.X = cropStartRect.Min.X + dx
					}
					if r.Min.X > r.Max.X {
						r.Min.X, r.Max.X = r.Max.X, r.Min.X
					}
					if r.Min.Y > r.Max.Y {
						r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
					}
					cropRect = r
					w.Send(paint.Event{})
				}

				if drawing && tool == ToolDraw && e.Direction == mouse.DirNone {
					p := image.Point{mx, my}
					minX, minY := last.X, last.Y
					maxX, maxY := p.X, p.Y
					if p.X < minX {
						minX = p.X
					}
					if p.Y < minY {
						minY = p.Y
					}
					if last.X > maxX {
						maxX = last.X
					}
					if last.Y > maxY {
						maxY = last.Y
					}
					br := image.Rect(minX, minY, maxX, maxY).Inset(-widths[tabs[current].WidthIdx] - 2)
					shift := ensureCanvasContains(&tabs[current], br)
					last = last.Sub(shift)
					p = p.Sub(shift)
					drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col, widths[tabs[current].WidthIdx])
					last = p
					w.Send(paint.Event{})
				}
				if moving && tool == ToolMove && e.Direction == mouse.DirNone {
					dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
					dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
					tabs[current].Offset = moveOffset.Add(image.Pt(dx, dy))
					w.Send(paint.Event{})
				}
			case key.Event:
				if e.Direction == key.DirPress {
					switch e.Rune {
					case 's', 'S':
						out, err := os.Create(*output)
						if err != nil {
							log.Printf("save: %v", err)
							continue
						}
						png.Encode(out, tabs[current].Image)
						out.Close()
						message = fmt.Sprintf("saved %s", *output)
						messageUntil = time.Now().Add(2 * time.Second)
						w.Send(paint.Event{})
					case 'd', 'D':
						if len(tabs) > 1 {
							tabs = append(tabs[:current], tabs[current+1:]...)
							if current >= len(tabs) {
								current = len(tabs) - 1
							}
						} else {
							bnd := tabs[current].Image.Bounds()
							tabs[0].Image = image.NewRGBA(image.Rect(0, 0, bnd.Dx(), bnd.Dy()))
							tabs[0].Title = "1"
							message = "no screenshot available"
							messageUntil = time.Now().Add(2 * time.Second)
						}
						w.Send(paint.Event{})
					case 'c', 'C':
						var buf bytes.Buffer
						png.Encode(&buf, tabs[current].Image)
						cmd := exec.Command("wl-copy", "--type", "image/png")
						cmd.Stdin = &buf
						if err := cmd.Run(); err != nil {
							log.Printf("copy: %v", err)
						} else {
							message = "copied to clipboard"
							messageUntil = time.Now().Add(2 * time.Second)
						}
						w.Send(paint.Event{})
					case 'q', 'Q':
						return
					case 'n', 'N':
						img, err := captureScreenshot()
						if err != nil {
							log.Printf("capture screenshot: %v", err)
							continue
						}
						tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: 2})
						current = len(tabs) - 1
						tabs[current].Zoom = fitZoom(tabs[current].Image, width, height)
						w.Send(paint.Event{})
					case '+', '=':
						tabs[current].Zoom *= 1.25
						if tabs[current].Zoom < 0.1 {
							tabs[current].Zoom = 0.1
						}
						w.Send(paint.Event{})
					case '-':
						tabs[current].Zoom /= 1.25
						if tabs[current].Zoom < 0.1 {
							tabs[current].Zoom = 0.1
						}
						w.Send(paint.Event{})
					case -1:
						switch e.Code {
						case key.CodeReturnEnter:
							if tool == ToolCrop && !cropRect.Empty() {
								cropped := cropImage(tabs[current].Image, cropRect)
								off := tabs[current].Offset.Add(cropRect.Min)
								if e.Modifiers&key.ModControl != 0 {
									tabs = append(tabs, Tab{Image: cropped, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: off, Zoom: tabs[current].Zoom, NextNumber: 1, WidthIdx: tabs[current].WidthIdx})
									current = len(tabs) - 1
								} else {
									tabs[current].Image = cropped
									tabs[current].Offset = off
								}
								cropping = false
								cropRect = image.Rectangle{}
								w.Send(paint.Event{})
							}
						case key.CodeEscape:
							if tool == ToolCrop {
								cropRect = image.Rectangle{}
								cropping = false
								w.Send(paint.Event{})
							}
						}
					}
				}
			}
		}
	})
}
