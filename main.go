package main

import (
	"flag"
	"fmt"
	"golang.design/x/clipboard"
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
	"strings"

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
	footerHeight = 24
	sideWidth    = 40
)

type Tab struct {
	Image  *image.RGBA
	Title  string
	Offset image.Point
}

const (
	toolMove = iota
	toolCrop
	toolDraw
	toolCircle
	toolLine
	toolArrow
	toolNumber
)

var toolNames = []string{"Move", "Crop", "Draw", "Circle", "Line", "Arrow", "Number"}

var palette = []color.RGBA{
	{0, 0, 0, 255}, {128, 0, 0, 255}, {0, 128, 0, 255}, {128, 128, 0, 255},
	{0, 0, 128, 255}, {128, 0, 128, 255}, {0, 128, 128, 255}, {192, 192, 192, 255},
	{128, 128, 128, 255}, {255, 0, 0, 255}, {0, 255, 0, 255}, {255, 255, 0, 255},
	{0, 0, 255, 255}, {255, 0, 255, 255}, {0, 255, 255, 255}, {255, 255, 255, 255},
}

var activeTool = toolMove
var activeColor = 0
var numberCount = 1

var message string
var messageFrames int

func drawFooter(dst *image.RGBA, width, height int) {
	keys := []string{"N", "D", "C", "S", "Q"}
	labels := []string{"New", "Delete", "Copy", "Save", "Quit"}
	x := sideWidth
	for i, k := range keys {
		rect := image.Rect(x, height-footerHeight, x+80, height)
		draw.Draw(dst, rect, &image.Uniform{color.RGBA{200, 200, 200, 255}}, image.Point{}, draw.Src)
		d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(x+4, height-footerHeight+16)}
		d.DrawString(k + " - " + labels[i])
		x += 80
	}
	draw.Draw(dst, image.Rect(x, height-footerHeight, width, height), &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
}

func drawSideBar(dst *image.RGBA, height int, currentTool int, currentColor int) {
	y := tabHeight
	for i, name := range toolNames {
		col := color.RGBA{200, 200, 200, 255}
		if i == currentTool {
			col = color.RGBA{150, 150, 150, 255}
		}
		rect := image.Rect(0, y, sideWidth, y+24)
		draw.Draw(dst, rect, &image.Uniform{col}, image.Point{}, draw.Src)
		d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+16)}
		d.DrawString(string(name[0]))
		y += 24
	}
	// palette
	y += 8
	for i, pc := range palette {
		rect := image.Rect(4, y, sideWidth-4, y+12)
		draw.Draw(dst, rect, &image.Uniform{pc}, image.Point{}, draw.Src)
		if i == currentColor {
			drawRectOutline(dst, rect.Inset(-1), color.Black)
		}
		y += 14
	}
}

func drawMessage(dst *image.RGBA, width, height int) {
	if message == "" {
		return
	}
	d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13}
	w := d.MeasureString(message).Round()
	x := (width - w) / 2
	y := (height - footerHeight - tabHeight) / 2
	rect := image.Rect(x-4, y-12, x+w+4, y+4)
	draw.Draw(dst, rect, &image.Uniform{color.RGBA{255, 255, 224, 255}}, image.Point{}, draw.Src)
	d.Dot = fixed.P(x, y)
	d.DrawString(message)
}

func drawTabs(dst *image.RGBA, tabs []Tab, current int) {
	x := 0
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
	draw.Draw(dst, image.Rect(x, 0, dst.Bounds().Dx(), tabHeight), &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
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
		if image.Pt(x0, y0).In(img.Bounds()) {
			img.Set(x0, y0, col)
		}
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

func drawRectOutline(img *image.RGBA, r image.Rectangle, col color.Color) {
	drawLine(img, r.Min.X, r.Min.Y, r.Max.X, r.Min.Y, col)
	drawLine(img, r.Max.X, r.Min.Y, r.Max.X, r.Max.Y, col)
	drawLine(img, r.Max.X, r.Max.Y, r.Min.X, r.Max.Y, col)
	drawLine(img, r.Min.X, r.Max.Y, r.Min.X, r.Min.Y, col)
}

func drawCircle(img *image.RGBA, cx, cy, r int, col color.Color) {
	x, y, d := r-1, 0, 1-r
	for y <= x {
		pts := [][2]int{
			{cx + x, cy + y}, {cx + y, cy + x},
			{cx - y, cy + x}, {cx - x, cy + y},
			{cx - x, cy - y}, {cx - y, cy - x},
			{cx + y, cy - x}, {cx + x, cy - y},
		}
		for _, p := range pts {
			if image.Pt(p[0], p[1]).In(img.Bounds()) {
				img.Set(p[0], p[1], col)
			}
		}
		if d < 0 {
			d += 2*y + 3
		} else {
			d += 2*(y-x) + 5
			x--
		}
		y++
	}
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
	clipboard.Init()

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
		width := rgba.Bounds().Dx() + sideWidth
		height := rgba.Bounds().Dy() + tabHeight + footerHeight
		w, err := s.NewWindow(&screen.NewWindowOptions{Width: width, Height: height})
		if err != nil {
			log.Fatalf("new window: %v", err)
		}
		defer w.Release()
		b, err := s.NewBuffer(image.Point{width, height})
		if err != nil {
			log.Fatalf("new buffer: %v", err)
		}
		defer b.Release()

		tabs := []Tab{{Image: rgba, Title: "1"}}
		current := 0

		var drawing bool
		var last image.Point
		col := palette[activeColor]
		var cropRect image.Rectangle
		var cropping bool

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case paint.Event:
				draw.Draw(b.RGBA(), b.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
				off := tabs[current].Offset
				imgRect := image.Rect(sideWidth+off.X, tabHeight+off.Y, sideWidth+off.X+tabs[current].Image.Bounds().Dx(), tabHeight+off.Y+tabs[current].Image.Bounds().Dy())
				draw.Draw(b.RGBA(), imgRect, tabs[current].Image, image.Point{}, draw.Src)
				if cropping {
					r := cropRect.Add(image.Point{sideWidth, tabHeight})
					drawRectOutline(b.RGBA(), r, color.RGBA{0, 0, 255, 255})
				}
				drawTabs(b.RGBA(), tabs, current)
				drawSideBar(b.RGBA(), height, activeTool, activeColor)
				drawFooter(b.RGBA(), width, height)
				drawMessage(b.RGBA(), width, height)
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
				if messageFrames > 0 {
					messageFrames--
					if messageFrames == 0 {
						message = ""
					}
				}
			case mouse.Event:
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
					if int(e.Y) < tabHeight {
						idx := int(e.X) / 80
						if idx >= 0 && idx < len(tabs) {
							current = idx
							w.Send(paint.Event{})
						}
						continue
					}
					if int(e.X) < sideWidth {
						idx := (int(e.Y) - tabHeight) / 24
						if idx >= 0 && idx < len(toolNames) {
							activeTool = idx
							drawing = false
							cropping = false
							w.Send(paint.Event{})
							continue
						}
						palStart := tabHeight + len(toolNames)*24 + 8
						if int(e.Y) >= palStart {
							cidx := (int(e.Y) - palStart) / 14
							if cidx >= 0 && cidx < len(palette) {
								activeColor = cidx
								col = palette[cidx]
								w.Send(paint.Event{})
							}
							continue
						}
					}
					if int(e.Y) >= height-footerHeight {
						idx := (int(e.X) - sideWidth) / 80
						switch idx {
						case 0:
							img, err := captureScreenshot()
							if err == nil {
								tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
								current = len(tabs) - 1
							}
						case 1:
							if len(tabs) > 0 {
								tabs = append(tabs[:current], tabs[current+1:]...)
								if current >= len(tabs) {
									current = len(tabs) - 1
								}
								if len(tabs) == 0 {
									tabs = []Tab{{Image: image.NewRGBA(image.Rect(0, 0, 400, 300)), Title: "Empty"}}
								}
							}
						case 2:
							clipboard.Write(clipboard.FmtImage, tabs[current].Image.Pix)
						case 3:
							out, err := os.Create(*output)
							if err == nil {
								png.Encode(out, tabs[current].Image)
								out.Close()
								message = "Saved to " + *output
								messageFrames = 120
							}
						case 4:
							return
						}
						w.Send(paint.Event{})
						continue
					}
					if activeTool == toolMove {
						drawing = true
						last = image.Point{int(e.X), int(e.Y)}
					} else if activeTool == toolCrop {
						cropping = true
						off := tabs[current].Offset
						cropRect.Min = image.Point{int(e.X) - sideWidth - off.X, int(e.Y) - tabHeight - off.Y}
						cropRect.Max = cropRect.Min
					} else {
						drawing = true
						off := tabs[current].Offset
						last = image.Point{int(e.X) - sideWidth - off.X, int(e.Y) - tabHeight - off.Y}
					}
				} else if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirRelease {
					if activeTool == toolMove {
						drawing = false
					} else if activeTool == toolCrop {
						cropping = true
					} else {
						drawing = false
					}
				} else if e.Direction == mouse.DirNone {
					if drawing && activeTool == toolMove {
						dx := int(e.X) - last.X
						dy := int(e.Y) - last.Y
						tabs[current].Offset.X += dx
						tabs[current].Offset.Y += dy
						last = image.Point{int(e.X), int(e.Y)}
						w.Send(paint.Event{})
					} else if drawing && activeTool == toolDraw {
						off := tabs[current].Offset
						p := image.Point{int(e.X) - sideWidth - off.X, int(e.Y) - tabHeight - off.Y}
						drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col)
						last = p
						w.Send(paint.Event{})
					} else if activeTool == toolCrop && cropping {
						off := tabs[current].Offset
						cropRect.Max = image.Point{int(e.X) - sideWidth - off.X, int(e.Y) - tabHeight - off.Y}
						w.Send(paint.Event{})
					}
				}
			case key.Event:
				if e.Direction == key.DirPress {
					switch e.Rune {
					case 's', 'S':
						out, err := os.Create(*output)
						if err == nil {
							png.Encode(out, tabs[current].Image)
							out.Close()
							message = "Saved to " + *output
							messageFrames = 120
						}
					case 'q', 'Q':
						return
					case 'n', 'N':
						img, err := captureScreenshot()
						if err == nil {
							tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
							current = len(tabs) - 1
						}
						w.Send(paint.Event{})
					case 'd', 'D':
						if len(tabs) > 0 {
							tabs = append(tabs[:current], tabs[current+1:]...)
							if current >= len(tabs) {
								current = len(tabs) - 1
							}
							if len(tabs) == 0 {
								tabs = []Tab{{Image: image.NewRGBA(image.Rect(0, 0, 400, 300)), Title: "Empty"}}
							}
						}
						w.Send(paint.Event{})
					case 'c', 'C':
						clipboard.Write(clipboard.FmtImage, tabs[current].Image.Pix)
					}
					if e.Code == key.CodeReturnEnter && cropping {
						r := cropRect.Canon()
						if r.Dx() > 0 && r.Dy() > 0 {
							newImg := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
							draw.Draw(newImg, newImg.Bounds(), tabs[current].Image, r.Min, draw.Src)
							tabs[current].Image = newImg
							tabs[current].Offset = image.Point{}
						}
						cropping = false
						w.Send(paint.Event{})
					}
				}
			}
		}
	})
}
