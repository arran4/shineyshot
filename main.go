package main

import (
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
	tabHeight      = 24
	shortcutHeight = 24
	leftBarWidth   = 80
)

type Tab struct {
	Image *image.RGBA
	Title string
}

func drawTabs(dst *image.RGBA, tabs []Tab, current int) {
	x := leftBarWidth
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

var shortcuts = []struct {
	Key   string
	Label string
}{
	{"N", "New"},
	{"D", "Delete"},
	{"C", "Copy"},
	{"S", "Save"},
	{"Q", "Quit"},
}

func drawShortcuts(dst *image.RGBA) {
	x := leftBarWidth
	y := dst.Bounds().Dy() - shortcutHeight
	for _, sc := range shortcuts {
		rect := image.Rect(x, y, x+80, y+shortcutHeight)
		draw.Draw(dst, rect, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.Black,
			Face: basicfont.Face7x13,
			Dot:  fixed.P(x+4, y+16),
		}
		d.DrawString(fmt.Sprintf("%s - %s", sc.Key, sc.Label))
		x += 80
	}
	draw.Draw(dst, image.Rect(x, y, dst.Bounds().Dx(), y+shortcutHeight), &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
}

var toolbarActions = []string{"Move", "Crop", "Draw"}

func drawToolbar(dst *image.RGBA, current int) {
	y := tabHeight
	for i, act := range toolbarActions {
		col := color.RGBA{220, 220, 220, 255}
		if i == current {
			col = color.RGBA{180, 180, 180, 255}
		}
		rect := image.Rect(0, y, leftBarWidth, y+24)
		draw.Draw(dst, rect, &image.Uniform{col}, image.Point{}, draw.Src)
		d := &font.Drawer{
			Dst:  dst,
			Src:  image.Black,
			Face: basicfont.Face7x13,
			Dot:  fixed.P(4, y+16),
		}
		d.DrawString(act)
		y += 24
	}
	draw.Draw(dst, image.Rect(0, y, leftBarWidth, dst.Bounds().Dy()-shortcutHeight), &image.Uniform{color.RGBA{230, 230, 230, 255}}, image.Point{}, draw.Src)
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
		width := rgba.Bounds().Dx() + leftBarWidth
		height := rgba.Bounds().Dy() + tabHeight + shortcutHeight
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
		var selecting bool
		var last image.Point
		var selectStart image.Point
		col := color.RGBA{255, 0, 0, 255}

		mode := 0 // 0 move, 1 crop, 2 draw
		offset := image.Point{}

		var snackMsg string
		var snackUntil time.Time

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					return
				}
			case paint.Event:
				draw.Draw(b.RGBA(), b.Bounds(), &image.Uniform{color.RGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)
				if len(tabs) > 0 && tabs[current].Image != nil {
					draw.Draw(b.RGBA(), image.Rect(leftBarWidth+offset.X, tabHeight+offset.Y, leftBarWidth+offset.X+tabs[current].Image.Bounds().Dx(), tabHeight+offset.Y+tabs[current].Image.Bounds().Dy()), tabs[current].Image, image.Point{}, draw.Over)
				} else {
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(leftBarWidth+10, tabHeight+20)}
					d.DrawString("No screenshot available")
				}
				if selecting {
					rect := image.Rect(selectStart.X+leftBarWidth+offset.X, selectStart.Y+tabHeight+offset.Y, last.X+leftBarWidth+offset.X, last.Y+tabHeight+offset.Y).Canon()
					draw.Draw(b.RGBA(), rect, &image.Uniform{color.RGBA{0, 0, 0, 50}}, image.Point{}, draw.Src)
				}
				drawTabs(b.RGBA(), tabs, current)
				drawShortcuts(b.RGBA())
				drawToolbar(b.RGBA(), mode)
				if snackMsg != "" && time.Now().Before(snackUntil) {
					d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: basicfont.Face7x13}
					w := b.Bounds().Dx()
					h := b.Bounds().Dy()
					d.Dot = fixed.P(w/2-len(snackMsg)*3, h/2)
					rect := image.Rect(w/2-80, h/2-10, w/2+80, h/2+10)
					draw.Draw(b.RGBA(), rect, &image.Uniform{color.RGBA{255, 255, 255, 200}}, image.Point{}, draw.Src)
					d.DrawString(snackMsg)
				}
				w.Upload(image.Point{}, b, b.Bounds())
				w.Publish()
			case mouse.Event:
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.Y) < tabHeight && int(e.X) >= leftBarWidth {
					idx := (int(e.X) - leftBarWidth) / 80
					if idx >= 0 && idx < len(tabs) {
						current = idx
						w.Send(paint.Event{})
					}
					continue
				}
				if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress && int(e.X) < leftBarWidth && int(e.Y) >= tabHeight && int(e.Y) < height-shortcutHeight {
					idx := (int(e.Y) - tabHeight) / 24
					if idx >= 0 && idx < len(toolbarActions) {
						mode = idx
						w.Send(paint.Event{})
					}
					continue
				}
				// shortcuts
				if e.Button == mouse.ButtonLeft && int(e.Y) >= height-shortcutHeight {
					idx := (int(e.X) - leftBarWidth) / 80
					if idx >= 0 && idx < len(shortcuts) {
						switch shortcuts[idx].Key {
						case "N":
							img, err := captureScreenshot()
							if err == nil {
								tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
								current = len(tabs) - 1
							} else {
								log.Printf("capture screenshot: %v", err)
							}
						case "D":
							if len(tabs) > 0 {
								tabs = append(tabs[:current], tabs[current+1:]...)
								if current >= len(tabs) {
									current = len(tabs) - 1
								}
							}
						case "S":
							out, err := os.Create(*output)
							if err == nil {
								png.Encode(out, tabs[current].Image)
								out.Close()
								snackMsg = fmt.Sprintf("saved %s", *output)
								snackUntil = time.Now().Add(2 * time.Second)
							} else {
								log.Printf("save: %v", err)
							}
						case "Q":
							return
						case "C":
							cmd := exec.Command("wl-copy")
							wIn, _ := cmd.StdinPipe()
							if err := cmd.Start(); err == nil {
								png.Encode(wIn, tabs[current].Image)
								wIn.Close()
								cmd.Wait()
							} else {
								log.Printf("wl-copy: %v", err)
							}
						}
						w.Send(paint.Event{})
					}
					continue
				}

				if e.Button == mouse.ButtonLeft {
					if e.Direction == mouse.DirPress {
						switch mode {
						case 0: // move
							drawing = true
							last = image.Point{int(e.X), int(e.Y)}
						case 1: // crop
							selecting = true
							selectStart = image.Point{int(e.X) - leftBarWidth - offset.X, int(e.Y) - tabHeight - offset.Y}
							last = selectStart
						case 2: // draw
							drawing = true
							last = image.Point{int(e.X) - leftBarWidth - offset.X, int(e.Y) - tabHeight - offset.Y}
						}
					} else if e.Direction == mouse.DirRelease {
						if mode == 0 {
							drawing = false
						} else if mode == 1 {
							selecting = false
						} else if mode == 2 {
							drawing = false
						}
					}
				}
				if e.Direction == mouse.DirNone {
					switch mode {
					case 0:
						if drawing {
							dx := int(e.X) - last.X
							dy := int(e.Y) - last.Y
							offset = offset.Add(image.Point{dx, dy})
							last = image.Point{int(e.X), int(e.Y)}
							w.Send(paint.Event{})
						}
					case 1:
						if selecting {
							last = image.Point{int(e.X) - leftBarWidth - offset.X, int(e.Y) - tabHeight - offset.Y}
							w.Send(paint.Event{})
						}
					case 2:
						if drawing {
							p := image.Point{int(e.X) - leftBarWidth - offset.X, int(e.Y) - tabHeight - offset.Y}
							drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col)
							last = p
							w.Send(paint.Event{})
						}
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
							snackMsg = fmt.Sprintf("saved %s", *output)
							snackUntil = time.Now().Add(2 * time.Second)
						} else {
							log.Printf("save: %v", err)
						}
					case 'q', 'Q':
						return
					case 'n', 'N':
						img, err := captureScreenshot()
						if err != nil {
							log.Printf("capture screenshot: %v", err)
							break
						}
						tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1)})
						current = len(tabs) - 1
						w.Send(paint.Event{})
					case 'd', 'D':
						if len(tabs) > 0 {
							tabs = append(tabs[:current], tabs[current+1:]...)
							if current >= len(tabs) {
								current = len(tabs) - 1
							}
							if len(tabs) == 0 {
								tabs = []Tab{}
							}
							w.Send(paint.Event{})
						}
					case 'c', 'C':
						cmd := exec.Command("wl-copy")
						wIn, _ := cmd.StdinPipe()
						if err := cmd.Start(); err == nil {
							png.Encode(wIn, tabs[current].Image)
							wIn.Close()
							cmd.Wait()
						} else {
							log.Printf("wl-copy: %v", err)
						}
					}
					if e.Code == key.CodeReturnEnter && mode == 1 {
						rect := image.Rect(selectStart.X, selectStart.Y, last.X, last.Y).Canon()
						if rect.Dx() > 0 && rect.Dy() > 0 {
							cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
							draw.Draw(cropped, cropped.Bounds(), tabs[current].Image, rect.Min, draw.Src)
							tabs[current].Image = cropped
							offset = image.Point{}
							w.Send(paint.Event{})
						}
					}
				}
			}
		}
	})
}
