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
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"golang.design/x/clipboard"

	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
)

const tabHeight = 24
const shortcutBarHeight = 24

type Shortcut struct {
	Key   rune
	Label string
}

var shortcuts = []Shortcut{
	{'N', "New"},
	{'D', "Delete"},
	{'C', "Copy"},
	{'S', "Save"},
	{'Q', "Quit"},
}

type Tab struct {
	Image *image.RGBA
	Title string
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

func drawShortcuts(dst *image.RGBA) {
	y := dst.Bounds().Dy() - shortcutBarHeight
	x := 0
	for _, s := range shortcuts {
		rect := image.Rect(x, y, x+80, y+shortcutBarHeight)
		draw.Draw(dst, rect, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.Black,
			Face: basicfont.Face7x13,
			Dot:  fixed.P(x+4, y+16),
		}
		d.DrawString(fmt.Sprintf("%c - %s", s.Key, s.Label))
		x += 80
	}
	draw.Draw(dst, image.Rect(x, y, dst.Bounds().Dx(), y+shortcutBarHeight), &image.Uniform{color.RGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)
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
		if err := clipboard.Init(); err != nil {
			log.Printf("clipboard init: %v", err)
		}

		imgHeight := rgba.Bounds().Dy()
		width := rgba.Bounds().Dx()
		height := imgHeight + tabHeight + shortcutBarHeight
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

		var snackbarMsg string
		var snackbarUntil time.Time

		var drawing bool
		var last image.Point
		col := color.RGBA{255, 0, 0, 255}

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case paint.Event:
				if current >= len(tabs) {
					current = len(tabs) - 1
				}
				if len(tabs) == 0 {
					draw.Draw(b.RGBA(), image.Rect(0, tabHeight, width, height-shortcutBarHeight), &image.Uniform{color.White}, image.Point{}, draw.Src)
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13}
					msg := "No screenshot available"
					x := (width - len(msg)*7) / 2
					y := tabHeight + (imgHeight / 2)
					d.Dot = fixed.P(x, y)
					d.DrawString(msg)
				} else {
					draw.Draw(b.RGBA(), image.Rect(0, tabHeight, width, tabHeight+imgHeight), tabs[current].Image, image.Point{}, draw.Src)
				}
				drawTabs(b.RGBA(), tabs, current)
				drawShortcuts(b.RGBA())
				if snackbarMsg != "" && time.Now().Before(snackbarUntil) {
					box := image.Rect(width/2-80, tabHeight+imgHeight/2-16, width/2+80, tabHeight+imgHeight/2+16)
					draw.Draw(b.RGBA(), box, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(box.Min.X+8, box.Min.Y+16)}
					d.DrawString(snackbarMsg)
				}
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.Y) < tabHeight {
					idx := int(e.X) / 80
					if idx >= 0 && idx < len(tabs) {
						current = idx
						w.Send(paint.Event{})
					}
					continue
				}
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.Y) >= height-shortcutBarHeight {
					idx := int(e.X) / 80
					if idx >= 0 && idx < len(shortcuts) {
						switch shortcuts[idx].Key {
						case 'N':
							img, err := captureScreenshot()
							if err != nil {
								log.Printf("capture screenshot: %v", err)
								break
							}
							tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
							current = len(tabs) - 1
							w.Send(paint.Event{})
						case 'D':
							if len(tabs) > 0 {
								tabs = append(tabs[:current], tabs[current+1:]...)
							}
							if current >= len(tabs) {
								current = len(tabs) - 1
							}
							w.Send(paint.Event{})
						case 'C':
							if len(tabs) > 0 {
								var buf bytes.Buffer
								png.Encode(&buf, tabs[current].Image)
								clipboard.Write(clipboard.FmtImage, buf.Bytes())
								snackbarMsg = "Copied to clipboard"
								snackbarUntil = time.Now().Add(2 * time.Second)
								w.Send(paint.Event{})
							}
						case 'S':
							if len(tabs) > 0 {
								out, err := os.Create(*output)
								if err != nil {
									log.Printf("save: %v", err)
									break
								}
								png.Encode(out, tabs[current].Image)
								out.Close()
								snackbarMsg = fmt.Sprintf("Saved %s", *output)
								snackbarUntil = time.Now().Add(2 * time.Second)
								w.Send(paint.Event{})
							}
						case 'Q':
							return
						}
					}
					continue
				}
				if e.Button == mouse.ButtonLeft {
					if e.Direction == mouse.DirPress {
						drawing = true
						last = image.Point{int(e.X), int(e.Y) - tabHeight}
					} else if e.Direction == mouse.DirRelease {
						drawing = false
					}
				}
				if drawing && e.Direction == mouse.DirNone {
					p := image.Point{int(e.X), int(e.Y) - tabHeight}
					drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col)
					last = p
					w.Send(paint.Event{})
				}
			case key.Event:
				if e.Direction == key.DirPress {
					handle := func(r rune) {
						switch r {
						case 's', 'S':
							if len(tabs) == 0 {
								break
							}
							out, err := os.Create(*output)
							if err != nil {
								log.Printf("save: %v", err)
								return
							}
							png.Encode(out, tabs[current].Image)
							out.Close()
							snackbarMsg = fmt.Sprintf("Saved %s", *output)
							snackbarUntil = time.Now().Add(2 * time.Second)
							w.Send(paint.Event{})
						case 'q', 'Q':
							return
						case 'n', 'N':
							img, err := captureScreenshot()
							if err != nil {
								log.Printf("capture screenshot: %v", err)
								return
							}
							tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
							current = len(tabs) - 1
							w.Send(paint.Event{})
						case 'd', 'D':
							if len(tabs) > 0 {
								tabs = append(tabs[:current], tabs[current+1:]...)
							}
							if current >= len(tabs) {
								current = len(tabs) - 1
							}
							w.Send(paint.Event{})
						case 'c', 'C':
							if len(tabs) == 0 {
								return
							}
							var buf bytes.Buffer
							png.Encode(&buf, tabs[current].Image)
							clipboard.Write(clipboard.FmtImage, buf.Bytes())
							snackbarMsg = "Copied to clipboard"
							snackbarUntil = time.Now().Add(2 * time.Second)
							w.Send(paint.Event{})
						}
					}
					handle(e.Rune)
				}
			}
		}
	})
}
