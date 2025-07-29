package main

import (
	"bytes"
	"flag"
	"fmt"
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
)

const (
	tabHeight    = 24
	bottomHeight = 24
	toolbarWidth = 48
)

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
}

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

var widths = []int{1, 2, 4, 6, 8}

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

func drawShortcuts(dst *image.RGBA, width, height int) {
	rect := image.Rect(0, height-bottomHeight, width, height)
	draw.Draw(dst, rect, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)

	shortcuts := []string{"N:new", "D:delete", "C:copy", "S:save", "Q:quit"}
	x := toolbarWidth + 4
	y := height - bottomHeight + 16
	for _, sc := range shortcuts {
		d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(x, y)}
		d.DrawString(sc)
		x += d.MeasureString(sc).Ceil() + 20
	}
}

func drawToolbar(dst *image.RGBA, tool Tool, colIdx, widthIdx int) {
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

	if tool == ToolCircle || tool == ToolLine || tool == ToolArrow {
		y += 4
		for i, w := range widths {
			rect := image.Rect(0, y, toolbarWidth, y+16)
			c := color.RGBA{200, 200, 200, 255}
			if i == widthIdx {
				c = color.RGBA{150, 150, 150, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
			d.DrawString(fmt.Sprintf("%d", w))
			y += 16
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
	const size = 6
	a1 := angle + math.Pi/6
	a2 := angle - math.Pi/6
	x2 := x1 - int(math.Cos(a1)*size)
	y2 := y1 - int(math.Sin(a1)*size)
	x3 := x1 - int(math.Cos(a2)*size)
	y3 := y1 - int(math.Sin(a2)*size)
	drawLine(img, x1, y1, x2, y2, col, thick)
	drawLine(img, x1, y1, x3, y3, col, thick)
}

func drawNumberBox(img *image.RGBA, x, y, num int, col color.Color) {
	text := fmt.Sprintf("%d", num)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x+4, y+12),
	}
	rect := image.Rect(x, y, x+20, y+16)
	draw.Draw(img, rect, &image.Uniform{color.RGBA{255, 255, 255, 200}}, image.Point{}, draw.Src)
	draw.Draw(img, rect, &image.Uniform{col}, image.Point{}, draw.Over)
	d.DrawString(text)
}

func cropImage(img *image.RGBA, rect image.Rectangle) *image.RGBA {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), img, rect.Min, draw.Src)
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

		tabs := []Tab{{Image: rgba, Title: "1"}}
		current := 0

		var drawing bool
		var cropping bool
		var last image.Point
		var cropStart image.Point
		var cropRect image.Rectangle
		var message string
		var messageUntil time.Time
		nextNumber := 1
		tool := ToolMove
		colorIdx := 2 // red
		widthIdx := 0

		col := palette[colorIdx]

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case paint.Event:
				b := bufs[bufIdx]
				bufIdx = 1 - bufIdx
				draw.Draw(b.RGBA(), b.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
				imgRect := image.Rect(toolbarWidth, tabHeight, toolbarWidth+tabs[current].Image.Bounds().Dx(), tabHeight+tabs[current].Image.Bounds().Dy())
				draw.Draw(b.RGBA(), imgRect, tabs[current].Image, image.Point{}, draw.Src)
				if tool == ToolCrop && (cropping || !cropRect.Empty()) {
					r := cropRect
					if cropping {
						r = image.Rect(cropStart.X, cropStart.Y, cropStart.X, cropStart.Y).Union(r)
					}
					r = r.Add(image.Pt(toolbarWidth, tabHeight))
					drawLine(b.RGBA(), r.Min.X, r.Min.Y, r.Max.X, r.Min.Y, color.Black, 1)
					drawLine(b.RGBA(), r.Min.X, r.Min.Y, r.Min.X, r.Max.Y, color.Black, 1)
					drawLine(b.RGBA(), r.Max.X, r.Min.Y, r.Max.X, r.Max.Y, color.Black, 1)
					drawLine(b.RGBA(), r.Min.X, r.Max.Y, r.Max.X, r.Max.Y, color.Black, 1)
				}
				drawTabs(b.RGBA(), tabs, current)
				drawToolbar(b.RGBA(), tool, colorIdx, widthIdx)
				drawShortcuts(b.RGBA(), width, height)
				if message != "" && time.Now().Before(messageUntil) {
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13}
					w := d.MeasureString(message).Ceil()
					px := toolbarWidth + (imgRect.Dx()-w)/2
					py := tabHeight + imgRect.Dy()/2
					d.Dot = fixed.P(px, py)
					d.DrawString(message)
				}
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.Y) < tabHeight {
					idx := int((e.X - toolbarWidth)) / 80
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
					if (tool == ToolCircle || tool == ToolLine || tool == ToolArrow) && pos >= 0 {
						widx := pos / 16
						if widx >= 0 && widx < len(widths) {
							widthIdx = widx
							w.Send(paint.Event{})
							continue
						}
					}
				}

				if int(e.X) < toolbarWidth || int(e.Y) < tabHeight || int(e.Y) > tabHeight+tabs[current].Image.Bounds().Dy() {
					break
				}

				mx := int(e.X) - toolbarWidth
				my := int(e.Y) - tabHeight
				if e.Button == mouse.ButtonLeft {
					if e.Direction == mouse.DirPress {
						switch tool {
						case ToolCrop:
							cropping = true
							cropStart = image.Point{mx, my}
							cropRect = image.Rect(mx, my, mx, my)
						case ToolDraw:
							drawing = true
							last = image.Point{mx, my}
						case ToolCircle, ToolLine, ToolArrow, ToolNumber:
							drawing = true
							last = image.Point{mx, my}
						}
					} else if e.Direction == mouse.DirRelease {
						if cropping && tool == ToolCrop {
							cropRect = cropRect.Union(image.Rect(mx, my, mx, my))
						}
						if drawing && tool != ToolCrop {
							switch tool {
							case ToolDraw:
								drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, 1)
							case ToolCircle:
								r := int(math.Hypot(float64(mx-last.X), float64(my-last.Y)))
								drawCircle(tabs[current].Image, last.X, last.Y, r, col, widths[widthIdx])
							case ToolLine:
								drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, widths[widthIdx])
							case ToolArrow:
								drawArrow(tabs[current].Image, last.X, last.Y, mx, my, col, widths[widthIdx])
							case ToolNumber:
								drawNumberBox(tabs[current].Image, mx, my, nextNumber, col)
								nextNumber++
							}
							w.Send(paint.Event{})
						}
						drawing = false
						cropping = false
					}
				}

				if drawing && tool == ToolDraw && e.Direction == mouse.DirNone {
					p := image.Point{mx, my}
					drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col, 1)
					last = p
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
						tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
						current = len(tabs) - 1
						w.Send(paint.Event{})
					case '\r':
						if tool == ToolCrop && !cropRect.Empty() {
							tabs[current].Image = cropImage(tabs[current].Image, cropRect)
							cropping = false
							w.Send(paint.Event{})
						}
					}
				}
			}
		}
	})
}
