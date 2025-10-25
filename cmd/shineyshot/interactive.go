package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
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
			fmt.Fprintln(os.Stdout, "  capture screen            capture full screen")
			fmt.Fprintln(os.Stdout, "  capture window            capture active window")
			fmt.Fprintln(os.Stdout, "  capture region            capture screen region")
			fmt.Fprintln(os.Stdout, "  arrow x0 y0 x1 y1         draw arrow")
			fmt.Fprintln(os.Stdout, "  line x0 y0 x1 y1          draw line")
			fmt.Fprintln(os.Stdout, "  rect x0 y0 x1 y1          draw rectangle")
			fmt.Fprintln(os.Stdout, "  circle x y r              draw circle")
			fmt.Fprintln(os.Stdout, "  crop x0 y0 x1 y1          crop image")
			fmt.Fprintln(os.Stdout, "  show                      display annotation window")
			fmt.Fprintln(os.Stdout, "  save FILE                 save image")
			fmt.Fprintln(os.Stdout, "  copy                      copy image to clipboard")
			fmt.Fprintln(os.Stdout, "  copyname                  copy saved filename")
			fmt.Fprintln(os.Stdout, "  quit                      exit interactive mode")
			continue
		case "capture":
			if len(args) < 1 {
				fmt.Fprintln(os.Stderr, "usage: capture [screen|window|region]")
				continue
			}
			var (
				img *image.RGBA
				err error
			)
			switch args[0] {
			case "screen":
				img, err = capture.CaptureScreenshot()
			case "window":
				img, err = capture.CaptureWindow()
			case "region":
				img, err = capture.CaptureRegion()
			default:
				fmt.Fprintln(os.Stderr, "usage: capture [screen|window|region]")
				continue
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				continue
			}
			i.img = img
			fmt.Fprintf(os.Stdout, "captured %s\n", args[0])
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
			appstate.DrawArrow(i.img, vals[0], vals[1], vals[2], vals[3], color.Black, 2)
			fmt.Fprintln(os.Stdout, "arrow drawn")
		case "line":
			if len(args) != 4 {
				fmt.Fprintln(os.Stderr, "usage: line x0 y0 x1 y1")
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
			appstate.DrawLine(i.img, vals[0], vals[1], vals[2], vals[3], color.Black, 2)
			fmt.Fprintln(os.Stdout, "line drawn")
		case "rect":
			if len(args) != 4 {
				fmt.Fprintln(os.Stderr, "usage: rect x0 y0 x1 y1")
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
			appstate.DrawRect(i.img, image.Rect(vals[0], vals[1], vals[2], vals[3]), color.Black, 2)
			fmt.Fprintln(os.Stdout, "rectangle drawn")
		case "circle":
			if len(args) != 3 {
				fmt.Fprintln(os.Stderr, "usage: circle x y r")
				continue
			}
			if i.img == nil {
				fmt.Fprintln(os.Stderr, "no image loaded")
				continue
			}
			vals := make([]int, 3)
			var err error
			for j := 0; j < 3; j++ {
				vals[j], err = strconv.Atoi(args[j])
				if err != nil {
					fmt.Fprintf(os.Stderr, "invalid number %q\n", args[j])
					continue
				}
			}
			appstate.DrawCircle(i.img, vals[0], vals[1], vals[2], color.Black, 2)
			fmt.Fprintln(os.Stdout, "circle drawn")
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
			i.img = appstate.CropImage(i.img, image.Rect(vals[0], vals[1], vals[2], vals[3]))
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
