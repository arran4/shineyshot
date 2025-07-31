package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"strconv"
	"strings"
)

// drawCmd draws a simple line on an image expanding the canvas if needed.
type drawCmd struct {
	file   string
	output string
	color  color.Color
	width  int
	coords [4]int
	root   *root
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
	if *file == "" || fs.NArg() < 4 {
		return nil, errors.New(drawHelp(fs, r))
	}
	col, err := parseColor(*colorFlag)
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
	return &drawCmd{file: *file, output: *output, color: col, width: *width, coords: coords, root: r}, nil
}

func expandCanvas(img *image.RGBA, rect image.Rectangle) (*image.RGBA, image.Point) {
	b := img.Bounds()
	minX := b.Min.X
	if rect.Min.X < minX {
		minX = rect.Min.X
	}
	minY := b.Min.Y
	if rect.Min.Y < minY {
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
	if minX == b.Min.X && minY == b.Min.Y && maxX == b.Max.X && maxY == b.Max.Y {
		return img, image.Point{}
	}
	newImg := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	draw.Draw(newImg, newImg.Bounds(), image.Transparent, image.Point{}, draw.Src)
	draw.Draw(newImg, b.Add(image.Pt(-minX, -minY)), img, image.Point{}, draw.Src)
	return newImg, image.Pt(minX, minY)
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
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
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

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (c *drawCmd) Run() error {
	f, err := os.Open(c.file)
	if err != nil {
		return err
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		return err
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	rect := image.Rect(c.coords[0], c.coords[1], c.coords[2], c.coords[3])
	if rect.Min.X > rect.Max.X {
		rect.Min.X, rect.Max.X = rect.Max.X, rect.Min.X
	}
	if rect.Min.Y > rect.Max.Y {
		rect.Min.Y, rect.Max.Y = rect.Max.Y, rect.Min.Y
	}
	var shift image.Point
	rgba, shift = expandCanvas(rgba, rect)
	c.coords[0] -= shift.X
	c.coords[2] -= shift.X
	c.coords[1] -= shift.Y
	c.coords[3] -= shift.Y
	drawLine(rgba, c.coords[0], c.coords[1], c.coords[2], c.coords[3], c.color, c.width)
	out, err := os.Create(c.output)
	if err != nil {
		return err
	}
	defer out.Close()
	return png.Encode(out, rgba)
}
