package main

import (
	"flag"
	"fmt"
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
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
)

const tabHeight = 30

var tabs = []string{"Brush", "Rect"}
var activeTab int

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

func drawRect(img *image.RGBA, x0, y0, x1, y1 int, col color.Color) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for x := x0; x <= x1; x++ {
		if image.Pt(x, y0).In(img.Bounds()) {
			img.Set(x, y0, col)
		}
		if image.Pt(x, y1).In(img.Bounds()) {
			img.Set(x, y1, col)
		}
	}
	for y := y0; y <= y1; y++ {
		if image.Pt(x0, y).In(img.Bounds()) {
			img.Set(x0, y, col)
		}
		if image.Pt(x1, y).In(img.Bounds()) {
			img.Set(x1, y, col)
		}
	}
}

func drawString(img *image.RGBA, x, y int, s string) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.Black),
		Face: basicfont.Face7x13,
		Dot:  fixed.P(x, y+13),
	}
	d.DrawString(s)
}

func drawTabs(img *image.RGBA) {
	tabWidth := img.Bounds().Dx() / len(tabs)
	for i, t := range tabs {
		r := image.Rect(i*tabWidth, 0, (i+1)*tabWidth, tabHeight)
		bg := color.RGBA{230, 230, 230, 255}
		if i == activeTab {
			bg = color.RGBA{200, 200, 200, 255}
		}
		draw.Draw(img, r, &image.Uniform{bg}, image.Point{}, draw.Src)
		drawString(img, i*tabWidth+5, 5, t)
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
		w, err := s.NewWindow(&screen.NewWindowOptions{Width: rgba.Bounds().Dx(), Height: rgba.Bounds().Dy() + tabHeight})
		if err != nil {
			log.Fatalf("new window: %v", err)
		}
		defer w.Release()
		b, err := s.NewBuffer(image.Point{X: rgba.Bounds().Dx(), Y: rgba.Bounds().Dy() + tabHeight})
		if err != nil {
			log.Fatalf("new buffer: %v", err)
		}
		defer b.Release()

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
				draw.Draw(b.RGBA(), b.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
				drawTabs(b.RGBA())
				draw.Draw(b.RGBA(), image.Rect(0, tabHeight, rgba.Bounds().Dx(), rgba.Bounds().Dy()+tabHeight), rgba, image.Point{}, draw.Src)
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Y < tabHeight {
					if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
						tabWidth := rgba.Bounds().Dx() / len(tabs)
						idx := int(e.X) / tabWidth
						if idx >= 0 && idx < len(tabs) {
							activeTab = idx
							w.Send(paint.Event{})
						}
					}
					break
				}
				y := int(e.Y) - tabHeight
				if activeTab == 0 { // Brush
					if e.Button == mouse.ButtonLeft {
						if e.Direction == mouse.DirPress {
							drawing = true
							last = image.Point{int(e.X), y}
						} else if e.Direction == mouse.DirRelease {
							drawing = false
						}
					}
					if drawing && e.Direction == mouse.DirNone {
						p := image.Point{int(e.X), y}
						drawLine(rgba, last.X, last.Y, p.X, p.Y, col)
						last = p
						w.Send(paint.Event{})
					}
				} else if activeTab == 1 { // Rect
					if e.Button == mouse.ButtonLeft {
						if e.Direction == mouse.DirPress {
							drawing = true
							last = image.Point{int(e.X), y}
						} else if e.Direction == mouse.DirRelease {
							drawing = false
							p := image.Point{int(e.X), y}
							drawRect(rgba, last.X, last.Y, p.X, p.Y, col)
							w.Send(paint.Event{})
						}
					}
				}
			case key.Event:
				if e.Direction == key.DirPress {
					if e.Rune == 's' || e.Rune == 'S' {
						out, err := os.Create(*output)
						if err != nil {
							log.Printf("save: %v", err)
							continue
						}
						png.Encode(out, rgba)
						out.Close()
						log.Printf("saved %s", *output)
					} else if e.Rune == 'q' || e.Rune == 'Q' {
						return
					}
				}
			}
		}
	})
}
