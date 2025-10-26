package main

import (
	"errors"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/example/shineyshot/internal/capture"
	"github.com/example/shineyshot/internal/clipboard"
	"github.com/example/shineyshot/internal/render"
)

type snapshotCmd struct {
	output             string
	stdout             bool
	toClipboard        bool
	mode               string
	display            string
	window             string
	region             string
	selector           string
	rect               string
	includeDecorations bool
	includeCursor      bool
	shadow             bool
	shadowRadius       int
	shadowOffset       string
	shadowPoint        image.Point
	shadowOpacity      float64
	*root
	fs *flag.FlagSet
}

func (s *snapshotCmd) FlagSet() *flag.FlagSet {
	return s.fs
}

func parseSnapshotCmd(args []string, r *root) (*snapshotCmd, error) {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	s := &snapshotCmd{root: r, fs: fs}
	fs.Usage = usageFunc(s)
	defaults := render.DefaultShadowOptions()
	fs.StringVar(&s.output, "output", "screenshot.png", "write the capture to this file path")
	fs.StringVar(&s.mode, "mode", "", "capture mode: screen, window, or region")
	fs.StringVar(&s.display, "display", "", "target display selector for screen captures")
	fs.StringVar(&s.window, "window", "", "target window selector for window captures")
	fs.StringVar(&s.region, "region", "", "capture rectangle x0,y0,x1,y1 when targeting a region")
	fs.BoolVar(&s.stdout, "stdout", false, "write PNG data to stdout")
	fs.BoolVar(&s.toClipboard, "to-clipboard", false, "copy the capture to the clipboard")
	fs.BoolVar(&s.toClipboard, "to-clip", false, "copy the capture to the clipboard (alias)")
	fs.StringVar(&s.selector, "select", "", "selector for screen or window capture")
	fs.StringVar(&s.rect, "rect", "", "capture rectangle x0,y0,x1,y1 when targeting a region")
	fs.BoolVar(&s.includeDecorations, "include-decorations", false, "request window decorations when capturing windows")
	fs.BoolVar(&s.includeCursor, "include-cursor", false, "embed the cursor in captures when supported")
	fs.BoolVar(&s.shadow, "shadow", false, "apply a drop shadow to the captured image")
	fs.IntVar(&s.shadowRadius, "shadow-radius", defaults.Radius, "drop shadow blur radius in pixels")
	fs.StringVar(&s.shadowOffset, "shadow-offset", formatShadowOffset(defaults.Offset), "drop shadow offset as dx,dy")
	fs.Float64Var(&s.shadowOpacity, "shadow-opacity", defaults.Opacity, "drop shadow opacity between 0 and 1")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	pt, err := parseShadowOffset(s.shadowOffset)
	if err != nil {
		return nil, err
	}
	s.shadowPoint = pt
	if s.toClipboard && s.stdout {
		return nil, fmt.Errorf("-stdout cannot be used with -to-clipboard")
	}
	operands := fs.Args()
	if len(operands) > 0 && strings.EqualFold(operands[0], "capture") {
		operands = operands[1:]
	}
	if strings.TrimSpace(s.mode) == "" {
		if len(operands) == 0 {
			return nil, &UsageError{of: s}
		}
		s.mode = strings.ToLower(strings.TrimSpace(operands[0]))
		operands = operands[1:]
	} else {
		s.mode = strings.ToLower(strings.TrimSpace(s.mode))
	}
	switch s.mode {
	case "screen", "window", "region":
	default:
		return nil, &UsageError{of: s}
	}
	if len(operands) > 0 {
		arg := strings.TrimSpace(strings.Join(operands, " "))
		switch s.mode {
		case "screen":
			if s.display == "" && s.selector == "" {
				s.display = arg
			}
		case "window":
			if s.window == "" && s.selector == "" {
				s.window = arg
			}
		case "region":
			if s.region == "" && s.rect == "" {
				s.region = arg
			}
		}
	}
	return s, nil
}

func (s *snapshotCmd) Run() error {
	img, err := s.capture()
	if err != nil {
		return fmt.Errorf("failed to capture %s: %w", s.mode, err)
	}
	if s.shadow {
		res := render.ApplyShadow(img, s.shadowOptions())
		img = res.Image
	}
	if s.root != nil {
		detail := s.describeCapture()
		s.root.notifyCapture(detail, img)
	}
	if s.toClipboard {
		if err := clipboard.WriteImage(img); err != nil {
			return fmt.Errorf("copy PNG to clipboard: %w", err)
		}
		detail := s.describeCapture()
		if detail == "" {
			detail = "image"
		}
		fmt.Fprintf(os.Stderr, "copied %s to clipboard\n", detail)
		if s.root != nil {
			s.root.notifyCopy(detail)
		}
		return nil
	}
	var w io.Writer
	if s.stdout {
		w = os.Stdout
	} else {
		f, err := os.Create(s.output)
		if err != nil {
			return fmt.Errorf("create output %q: %w", s.output, err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				log.Printf("close %s: %v", s.output, cerr)
			}
		}()
		w = f
	}
	if err := png.Encode(w, img); err != nil {
		if s.stdout {
			return fmt.Errorf("write PNG to stdout: %w", err)
		}
		return fmt.Errorf("write PNG to %q: %w", s.output, err)
	}
	if s.stdout {
		fmt.Fprintln(os.Stderr, "wrote PNG data to stdout")
		return nil
	}
	saved := s.output
	if abs, err := filepath.Abs(s.output); err == nil {
		saved = abs
	}
	fmt.Fprintf(os.Stderr, "saved %s\n", saved)
	if s.root != nil {
		s.root.notifySave(saved)
	}
	return nil
}

func (s *snapshotCmd) capture() (*image.RGBA, error) {
	opts := s.captureOptions()
	switch s.mode {
	case "screen":
		target := firstNonEmpty(s.display, s.selector)
		return captureScreenshotFn(target, opts)
	case "window":
		target := firstNonEmpty(s.window, s.selector)
		return captureWindowFn(target, opts)
	case "region":
		region := firstNonEmpty(s.region, s.rect)
		if strings.TrimSpace(region) == "" {
			return captureRegionFn(opts)
		}
		rect, err := parseRect(region)
		if err != nil {
			return nil, err
		}
		return captureRegionRectFn(rect, opts)
	default:
		return nil, errors.New("unsupported capture mode")
	}
}

func (s *snapshotCmd) describeCapture() string {
	mode := strings.TrimSpace(s.mode)
	switch mode {
	case "screen":
		target := strings.TrimSpace(firstNonEmpty(s.display, s.selector))
		if target != "" {
			return fmt.Sprintf("screen %s", target)
		}
	case "window":
		target := strings.TrimSpace(firstNonEmpty(s.window, s.selector))
		if target != "" {
			return fmt.Sprintf("window %s", target)
		}
	case "region":
		region := strings.TrimSpace(firstNonEmpty(s.region, s.rect))
		if region != "" {
			return fmt.Sprintf("region %s", region)
		}
	}
	if mode == "" {
		return "capture"
	}
	return mode
}

func (s *snapshotCmd) captureOptions() capture.CaptureOptions {
	return capture.CaptureOptions{
		IncludeDecorations: s.includeDecorations,
		IncludeCursor:      s.includeCursor,
	}
}

func (s *snapshotCmd) shadowOptions() render.ShadowOptions {
	opts := render.DefaultShadowOptions()
	if s.shadowRadius >= 0 {
		opts.Radius = s.shadowRadius
	} else {
		opts.Radius = 0
	}
	opts.Offset = s.shadowPoint
	if s.shadowOpacity <= 0 {
		opts.Opacity = 0
	} else if s.shadowOpacity >= 1 {
		opts.Opacity = 1
	} else {
		opts.Opacity = s.shadowOpacity
	}
	return opts
}

func parseShadowOffset(val string) (image.Point, error) {
	parts := strings.Split(val, ",")
	if len(parts) != 2 {
		return image.Point{}, fmt.Errorf("invalid shadow offset %q", val)
	}
	vals := make([]int, 2)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return image.Point{}, fmt.Errorf("invalid shadow offset %q", val)
		}
		vals[i] = v
	}
	return image.Pt(vals[0], vals[1]), nil
}

func formatShadowOffset(pt image.Point) string {
	return fmt.Sprintf("%d,%d", pt.X, pt.Y)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func parseRect(val string) (image.Rectangle, error) {
	parts := strings.Split(val, ",")
	if len(parts) != 4 {
		return image.Rectangle{}, fmt.Errorf("invalid region %q", val)
	}
	nums := make([]int, 4)
	for i, p := range parts {
		v, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			return image.Rectangle{}, fmt.Errorf("invalid region %q", val)
		}
		nums[i] = v
	}
	rect := image.Rect(nums[0], nums[1], nums[2], nums[3])
	if rect.Empty() {
		return image.Rectangle{}, fmt.Errorf("region %q is empty", val)
	}
	return rect, nil
}
