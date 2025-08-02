package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
)

type interactiveCmd struct {
	r      *root
	img    *image.RGBA
	output string
}

func drawArrow(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	appstate.DrawLine(img, x0, y0, x1, y1, col, thick)
	angle := math.Atan2(float64(y1-y0), float64(x1-x0))
	size := float64(6 + thick*2)
	a1 := angle + math.Pi/6
	a2 := angle - math.Pi/6
	x2 := x1 - int(math.Cos(a1)*size)
	y2 := y1 - int(math.Sin(a1)*size)
	x3 := x1 - int(math.Cos(a2)*size)
	y3 := y1 - int(math.Sin(a2)*size)
	appstate.DrawLine(img, x1, y1, x2, y2, col, thick)
	appstate.DrawLine(img, x1, y1, x3, y3, col, thick)
}

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

func (i *interactiveCmd) Run() error {
	fmt.Fprintln(os.Stdout, "Interactive mode. Type 'help' for commands.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		cmd := strings.ToLower(fields[0])
		args := fields[1:]

		switch cmd {
		case "quit", "exit":
			return nil
		case "help":
			fmt.Fprintln(os.Stdout, "Commands:")
			fmt.Fprintln(os.Stdout, "  screenshot                capture full screen")
			fmt.Fprintln(os.Stdout, "  arrow x0 y0 x1 y1         draw arrow")
			fmt.Fprintln(os.Stdout, "  crop x0 y0 x1 y1          crop image")
			fmt.Fprintln(os.Stdout, "  show                      display annotation window")
			fmt.Fprintln(os.Stdout, "  save FILE                 save image")
			fmt.Fprintln(os.Stdout, "  copy                      copy image to clipboard")
			fmt.Fprintln(os.Stdout, "  copyname                  copy saved filename")
			fmt.Fprintln(os.Stdout, "  quit                      exit interactive mode")
			continue
		case "screenshot":
			img, err := capture.CaptureScreenshot()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			i.img = img
			fmt.Fprintln(os.Stdout, "captured screenshot")
		case "arrow":
			if len(args) != 4 {
				fmt.Fprintln(os.Stderr, "usage: arrow x0 y0 x1 y1")
				continue
			}
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			vals := make([]int, 4)
			var err error
			for j := 0; j < 4; j++ {
				vals[j], err = strconv.Atoi(args[j])
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid number %q\n", args[j])
					continue
				}
			}
			drawArrow(i.img, vals[0], vals[1], vals[2], vals[3], color.Black, 2)
			fmt.Fprintln(os.Stdout, "arrow drawn")
		case "crop":
			if len(args) != 4 {
				fmt.Fprintln(os.Stderr, "usage: crop x0 y0 x1 y1")
				continue
			}
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			vals := make([]int, 4)
			var err error
			for j := 0; j < 4; j++ {
				vals[j], err = strconv.Atoi(args[j])
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid number %q\n", args[j])
					continue
				}
			}
			i.img = cropImage(i.img, image.Rect(vals[0], vals[1], vals[2], vals[3]))
			fmt.Fprintln(os.Stdout, "cropped")
		case "show", "preview":
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			img := image.NewRGBA(i.img.Bounds())
			draw.Draw(img, img.Bounds(), i.img, image.Point{}, draw.Src)
			st := appstate.New(appstate.WithImage(img))
			go st.Run()
		case "save":
			if len(args) != 1 {
				fmt.Fprintln(os.Stderr, "usage: save FILE")
				continue
			}
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			f, err := os.Create(args[0])
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			if err := png.Encode(f, i.img); err != nil {
				fmt.Fprintln(os.Stderr, err)
			} else {
				i.output = args[0]
				fmt.Fprintf(os.Stdout, "saved %s\n", args[0])
			}
			f.Close()
		case "copy":
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			var buf bytes.Buffer
			if err := png.Encode(&buf, i.img); err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			cmd := exec.Command("wl-copy", "--type", "image/png")
			cmd.Stdin = &buf
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			fmt.Fprintln(os.Stdout, "image copied to clipboard")
		case "copyname":
			if i.output == "" {
				fmt.Fprintln(os.Stderr, "no saved file")
				continue
			}
			cmd := exec.Command("wl-copy")
			cmd.Stdin = strings.NewReader(i.output)
			if err := cmd.Run(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			fmt.Fprintln(os.Stdout, "filename copied to clipboard")
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		}
	}
	return scanner.Err()
}
