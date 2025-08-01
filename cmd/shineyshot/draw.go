package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
)

// drawCmd draws a simple line on an image expanding the canvas if needed.
type drawCmd struct {
	file   string
	output string
	color  color.Color
	width  int
	coords [4]int
	*root
	fs *flag.FlagSet
}

func (d *drawCmd) FlagSet() *flag.FlagSet {
	return d.fs
}

func parseColor(s string) (color.Color, error) {
	switch strings.ToLower(s) {
	case "red":
		return color.RGBA{255, 0, 0, 255}, nil
	case "green":
		return color.RGBA{0, 255, 0, 255}, nil
	case "blue":
		return color.RGBA{0, 0, 255, 255}, nil
	case "black":
		return color.Black, nil
	case "white":
		return color.White, nil
	default:
		if strings.HasPrefix(s, "#") && (len(s) == 7 || len(s) == 9) {
			r, _ := strconv.ParseUint(s[1:3], 16, 8)
			g, _ := strconv.ParseUint(s[3:5], 16, 8)
			b, _ := strconv.ParseUint(s[5:7], 16, 8)
			a := uint64(255)
			if len(s) == 9 {
				a, _ = strconv.ParseUint(s[7:9], 16, 8)
			}
			return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}, nil
		}
	}
	return nil, fmt.Errorf("invalid color %q", s)
}

func parseDrawCmd(args []string, r *root) (*drawCmd, error) {
	fs := flag.NewFlagSet("draw", flag.ExitOnError)
	file := fs.String("file", "", "input image file")
	output := fs.String("output", "output.png", "output file path")
	colorFlag := fs.String("color", "red", "drawing color")
	width := fs.Int("width", 2, "line width")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	d := &drawCmd{file: *file, output: *output, width: *width, root: r, fs: fs}
	if *file == "" || fs.NArg() < 4 {
		return nil, &UsageError{of: d}
	}
	var err error
	d.color, err = parseColor(*colorFlag)
	if err != nil {
		return nil, err
	}
	var coords [4]int
	for i := 0; i < 4; i++ {
		v, err := strconv.Atoi(fs.Arg(i))
		if err != nil {
			return nil, fmt.Errorf("invalid coordinate %q", fs.Arg(i))
		}
		coords[i] = v
	}
	d.coords = coords
	return d, nil
}

func (d *drawCmd) Run() error {
	f, err := os.Open(d.file)
	if err != nil {
		return err
	}
	defer func(f *os.File) {
		err := f.Close()
		if err != nil {
			log.Printf("error closing %q: %v", f.Name(), err)
		}
	}(f)
	img, err := png.Decode(f)
	if err != nil {
		return err
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	rect := image.Rect(d.coords[0], d.coords[1], d.coords[2], d.coords[3])
	if rect.Min.X > rect.Max.X {
		rect.Min.X, rect.Max.X = rect.Max.X, rect.Min.X
	}
	if rect.Min.Y > rect.Max.Y {
		rect.Min.Y, rect.Max.Y = rect.Max.Y, rect.Min.Y
	}
	var shift image.Point
	rgba, shift = appstate.ExpandCanvas(rgba, rect)
	d.coords[0] -= shift.X
	d.coords[2] -= shift.X
	d.coords[1] -= shift.Y
	d.coords[3] -= shift.Y
	appstate.DrawLine(rgba, d.coords[0], d.coords[1], d.coords[2], d.coords[3], d.color, d.width)
	out, err := os.Create(d.output)
	if err != nil {
		return err
	}
	defer func(out *os.File) {
		err := out.Close()
		if err != nil {
			log.Printf("error closing %q: %v", out.Name(), err)
		}
	}(out)
	return png.Encode(out, rgba)
}
