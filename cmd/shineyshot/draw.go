package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/clipboard"
	"golang.org/x/image/colornames"
)

// drawCmd performs simple markup operations on an image expanding the canvas when needed.
type drawCmd struct {
	file          string
	output        string
	fromClipboard bool
	toClipboard   bool
	colorSpec     string
	color         color.RGBA
	width         int
	shape         string
	coords        []int
	text          string
	textSize      float64
	number        int
	numberSize    int
	maskOpacity   int
	*root
	fs *flag.FlagSet
}

func (d *drawCmd) FlagSet() *flag.FlagSet {
	return d.fs
}

func parseColor(s string) (color.RGBA, error) {
	spec := strings.ToLower(strings.TrimSpace(s))
	if spec == "" {
		return color.RGBA{}, fmt.Errorf("color cannot be empty")
	}
	if c, ok := colornames.Map[spec]; ok {
		return c, nil
	}
	for _, entry := range appstate.PaletteColors() {
		if strings.EqualFold(entry.Name, s) {
			return entry.Color, nil
		}
	}
	if strings.HasPrefix(spec, "#") && (len(spec) == 7 || len(spec) == 9) {
		r, err := strconv.ParseUint(spec[1:3], 16, 8)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid color %q", s)
		}
		g, err := strconv.ParseUint(spec[3:5], 16, 8)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid color %q", s)
		}
		b, err := strconv.ParseUint(spec[5:7], 16, 8)
		if err != nil {
			return color.RGBA{}, fmt.Errorf("invalid color %q", s)
		}
		a := uint64(255)
		if len(spec) == 9 {
			val, err := strconv.ParseUint(spec[7:9], 16, 8)
			if err != nil {
				return color.RGBA{}, fmt.Errorf("invalid color %q", s)
			}
			a = val
		}
		return color.RGBA{uint8(r), uint8(g), uint8(b), uint8(a)}, nil
	}
	return color.RGBA{}, fmt.Errorf("invalid color %q", s)
}

func parseDrawCmd(args []string, r *root) (*drawCmd, error) {
	fs := flag.NewFlagSet("draw", flag.ExitOnError)
	d := &drawCmd{root: r, fs: fs}
	fs.Usage = usageFunc(d)
	fs.StringVar(&d.file, "file", "", "input image file")
	fs.StringVar(&d.output, "output", "", "output file path (defaults to input file)")
	fs.BoolVar(&d.fromClipboard, "from-clipboard", false, "read the input image from the clipboard")
	fs.BoolVar(&d.fromClipboard, "from-clip", false, "read the input image from the clipboard (alias)")
	fs.BoolVar(&d.toClipboard, "to-clipboard", false, "copy the result to the clipboard")
	fs.BoolVar(&d.toClipboard, "to-clip", false, "copy the result to the clipboard (alias)")
	fs.StringVar(&d.colorSpec, "color", "red", "stroke or fill color name or hex value")
	fs.IntVar(&d.width, "width", 2, "stroke width in pixels")
	fs.Float64Var(&d.textSize, "text-size", appstate.DefaultTextSize(), "text size in points")
	fs.IntVar(&d.numberSize, "number-size", 16, "radius of numbered markers in pixels")
	fs.IntVar(&d.maskOpacity, "mask-opacity", 160, "mask opacity between 0 (transparent) and 255 (opaque)")

	flagArgs, positionals, err := splitDrawArgs(args)
	if err != nil {
		return nil, err
	}
	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	if len(positionals) < 1 {
		return nil, &UsageError{of: d}
	}
	d.shape = strings.ToLower(positionals[0])
	remaining := positionals[1:]
	switch d.shape {
	case "line", "arrow", "rect":
		d.coords, err = expectInts(remaining, 4, d.shape)
	case "circle":
		d.coords, err = expectInts(remaining, 3, d.shape)
	case "number":
		if len(remaining) != 3 {
			return nil, fmt.Errorf("number requires x y value")
		}
		var coords []int
		coords, err = expectInts(remaining, 3, d.shape)
		if err != nil {
			return nil, err
		}
		d.coords = coords[:2]
		d.number = coords[2]
	case "text":
		if len(remaining) < 3 {
			return nil, fmt.Errorf("text requires x y and content")
		}
		var coords []int
		coords, err = expectInts(remaining[:2], 2, d.shape)
		if err != nil {
			return nil, err
		}
		d.coords = coords
		d.text = strings.Join(remaining[2:], " ")
		if strings.TrimSpace(d.text) == "" {
			return nil, fmt.Errorf("text content cannot be empty")
		}
	case "mask":
		d.coords, err = expectInts(remaining, 4, d.shape)
	default:
		return nil, fmt.Errorf("unsupported shape %q", d.shape)
	}
	if err != nil {
		return nil, err
	}
	colorVal, err := parseColor(d.colorSpec)
	if err != nil {
		return nil, err
	}
	d.color = colorVal
	if d.fromClipboard {
		if d.output == "" {
			if d.file != "" {
				d.output = d.file
			} else {
				return nil, fmt.Errorf("output file is required when reading from the clipboard")
			}
		}
	} else {
		if d.file == "" {
			return nil, fmt.Errorf("input file is required")
		}
		if d.output == "" {
			d.output = d.file
		}
	}
	if d.width < 1 {
		d.width = 1
	}
	if d.numberSize <= 0 {
		d.numberSize = 16
	}
	if d.textSize <= 0 {
		d.textSize = appstate.DefaultTextSize()
	}
	if d.maskOpacity < 0 || d.maskOpacity > 255 {
		return nil, fmt.Errorf("mask-opacity must be between 0 and 255")
	}
	return d, nil
}

func (d *drawCmd) Run() error {
	src, err := d.loadSource()
	if err != nil {
		return err
	}
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, image.Point{}, draw.Src)
	rgba, err = d.applyShape(rgba)
	if err != nil {
		return err
	}
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
	if err := png.Encode(out, rgba); err != nil {
		return err
	}
	saved := d.output
	if abs, err := filepath.Abs(d.output); err == nil {
		saved = abs
	}
	fmt.Fprintf(os.Stderr, "saved %s\n", saved)
	if d.root != nil {
		d.root.notifySave(saved)
	}
	if d.toClipboard {
		if err := clipboard.WriteImage(rgba); err != nil {
			return fmt.Errorf("copy PNG to clipboard: %w", err)
		}
		detail := filepath.Base(d.output)
		if detail == "" {
			detail = "image"
		}
		fmt.Fprintf(os.Stderr, "copied %s to clipboard\n", detail)
		if d.root != nil {
			d.root.notifyCopy(detail)
		}
	}
	return nil
}

func (d *drawCmd) loadSource() (image.Image, error) {
	if d.fromClipboard {
		img, err := clipboard.ReadImage()
		if err != nil {
			return nil, fmt.Errorf("read clipboard image: %w", err)
		}
		return img, nil
	}
	f, err := os.Open(d.file)
	if err != nil {
		return nil, err
	}
	img, err := png.Decode(f)
	if err != nil {
		if cerr := f.Close(); cerr != nil {
			log.Printf("error closing %q: %v", f.Name(), cerr)
		}
		return nil, err
	}
	if err := f.Close(); err != nil {
		log.Printf("error closing %q: %v", f.Name(), err)
	}
	return img, nil
}

func expectInts(args []string, n int, shape string) ([]int, error) {
	if len(args) != n {
		return nil, fmt.Errorf("%s requires %d integer arguments", shape, n)
	}
	vals := make([]int, n)
	for i, raw := range args {
		v, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("invalid integer %q", raw)
		}
		vals[i] = v
	}
	return vals, nil
}

func (d *drawCmd) applyShape(img *image.RGBA) (*image.RGBA, error) {
	switch d.shape {
	case "line":
		return d.drawLine(img, false)
	case "arrow":
		return d.drawLine(img, true)
	case "rect":
		return d.drawRect(img)
	case "circle":
		return d.drawCircle(img)
	case "number":
		return d.drawNumber(img)
	case "text":
		return d.drawText(img)
	case "mask":
		return d.drawMask(img)
	default:
		return nil, errors.New("unhandled shape")
	}
}

func (d *drawCmd) drawLine(img *image.RGBA, arrow bool) (*image.RGBA, error) {
	if len(d.coords) != 4 {
		return nil, fmt.Errorf("expected 4 coordinates for %s", d.shape)
	}
	x0, y0, x1, y1 := d.coords[0], d.coords[1], d.coords[2], d.coords[3]
	rect := boundsForLine(x0, y0, x1, y1, d.width)
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	d.coords[0] = x0 - shift.X
	d.coords[1] = y0 - shift.Y
	d.coords[2] = x1 - shift.X
	d.coords[3] = y1 - shift.Y
	if arrow {
		appstate.DrawArrow(img, d.coords[0], d.coords[1], d.coords[2], d.coords[3], d.color, d.width)
	} else {
		appstate.DrawLine(img, d.coords[0], d.coords[1], d.coords[2], d.coords[3], d.color, d.width)
	}
	return img, nil
}

func (d *drawCmd) drawRect(img *image.RGBA) (*image.RGBA, error) {
	if len(d.coords) != 4 {
		return nil, fmt.Errorf("expected 4 coordinates for rect")
	}
	rect := orderedRect(d.coords[0], d.coords[1], d.coords[2], d.coords[3])
	rect = inflateRect(rect, d.width)
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	rect = rect.Sub(shift)
	appstate.DrawRect(img, rect, d.color, d.width)
	return img, nil
}

func (d *drawCmd) drawCircle(img *image.RGBA) (*image.RGBA, error) {
	if len(d.coords) != 3 {
		return nil, fmt.Errorf("expected center x y radius for circle")
	}
	cx, cy, radius := d.coords[0], d.coords[1], d.coords[2]
	if radius <= 0 {
		return nil, fmt.Errorf("radius must be positive")
	}
	rect := image.Rect(cx-radius, cy-radius, cx+radius, cy+radius)
	rect = inflateRect(rect, d.width)
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	cx -= shift.X
	cy -= shift.Y
	appstate.DrawCircle(img, cx, cy, radius, d.color, d.width)
	return img, nil
}

func (d *drawCmd) drawNumber(img *image.RGBA) (*image.RGBA, error) {
	if len(d.coords) != 2 {
		return nil, fmt.Errorf("expected x y for number")
	}
	cx, cy := d.coords[0], d.coords[1]
	radius := d.numberSize
	rect := image.Rect(cx-radius, cy-radius, cx+radius, cy+radius)
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	cx -= shift.X
	cy -= shift.Y
	appstate.DrawNumber(img, cx, cy, d.number, d.numberSize, d.color)
	return img, nil
}

func (d *drawCmd) drawText(img *image.RGBA) (*image.RGBA, error) {
	if len(d.coords) != 2 {
		return nil, fmt.Errorf("expected x y for text")
	}
	x, y := d.coords[0], d.coords[1]
	width, height, _, err := appstate.MeasureText(d.text, d.textSize)
	if err != nil {
		return nil, err
	}
	rect := image.Rect(x, y, x+width, y+height)
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	x -= shift.X
	y -= shift.Y
	if err := appstate.DrawText(img, x, y, d.text, d.color, d.textSize); err != nil {
		return nil, err
	}
	return img, nil
}

func (d *drawCmd) drawMask(img *image.RGBA) (*image.RGBA, error) {
	if len(d.coords) != 4 {
		return nil, fmt.Errorf("expected 4 coordinates for mask")
	}
	rect := orderedRect(d.coords[0], d.coords[1], d.coords[2], d.coords[3])
	var shift image.Point
	img, shift = appstate.ExpandCanvas(img, rect)
	rect = rect.Sub(shift)
	fill := color.RGBA{R: d.color.R, G: d.color.G, B: d.color.B, A: uint8(d.maskOpacity)}
	appstate.DrawMask(img, rect, fill)
	return img, nil
}

func boundsForLine(x0, y0, x1, y1, width int) image.Rectangle {
	minX := minInt(x0, x1) - width
	maxX := maxInt(x0, x1) + width
	minY := minInt(y0, y1) - width
	maxY := maxInt(y0, y1) + width
	if minX > maxX {
		minX, maxX = maxX, minX
	}
	if minY > maxY {
		minY, maxY = maxY, minY
	}
	if minX == maxX {
		maxX = minX + width
	}
	if minY == maxY {
		maxY = minY + width
	}
	return image.Rect(minX, minY, maxX, maxY)
}

func orderedRect(x0, y0, x1, y1 int) image.Rectangle {
	minX := minInt(x0, x1)
	minY := minInt(y0, y1)
	maxX := maxInt(x0, x1)
	maxY := maxInt(y0, y1)
	return image.Rect(minX, minY, maxX, maxY)
}

func inflateRect(r image.Rectangle, amount int) image.Rectangle {
	if amount <= 0 {
		return r
	}
	amount = int(math.Ceil(float64(amount) / 2.0))
	return image.Rect(r.Min.X-amount, r.Min.Y-amount, r.Max.X+amount, r.Max.Y+amount)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var drawFlagNames = map[string]struct{}{
	"file":           {},
	"output":         {},
	"from-clipboard": {},
	"from-clip":      {},
	"color":          {},
	"width":          {},
	"text-size":      {},
	"number-size":    {},
	"mask-opacity":   {},
}

var drawBoolFlags = map[string]struct{}{
	"from-clipboard": {},
	"from-clip":      {},
}

func splitDrawArgs(args []string) ([]string, []string, error) {
	var flags []string
	var positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if name == "" {
			positionals = append(positionals, arg)
			continue
		}
		parts := strings.SplitN(name, "=", 2)
		base := strings.ToLower(parts[0])
		if _, ok := drawFlagNames[base]; !ok {
			positionals = append(positionals, arg)
			continue
		}
		// Normalise to single dash form for the flag parser.
		norm := "-" + base
		if len(parts) == 2 {
			flags = append(flags, norm+"="+parts[1])
			continue
		}
		if _, ok := drawBoolFlags[base]; ok {
			flags = append(flags, norm)
			continue
		}
		if i+1 >= len(args) {
			return nil, nil, fmt.Errorf("flag %s requires a value", arg)
		}
		flags = append(flags, norm, args[i+1])
		i++
	}
	return flags, positionals, nil
}
