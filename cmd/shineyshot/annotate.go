package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
	"github.com/example/shineyshot/internal/clipboard"
	"github.com/example/shineyshot/internal/render"
)

// annotateCmd represents the annotate subcommand.
type annotateCmd struct {
	action        string
	target        string
	selector      string
	rect          string
	file          string
	output        string
	shadow        bool
	shadowRadius  int
	shadowOffset  string
	shadowPoint   image.Point
	shadowOpacity float64
	fromClipboard bool
	*root
	fs *flag.FlagSet
}

func (a *annotateCmd) FlagSet() *flag.FlagSet {
	return a.fs
}

func parseAnnotateCmd(args []string, r *root) (*annotateCmd, error) {
	fs := flag.NewFlagSet("annotate", flag.ExitOnError)
	a := &annotateCmd{root: r, fs: fs}
	fs.Usage = usageFunc(a)
	defaults := render.DefaultShadowOptions()
	fs.StringVar(&a.file, "file", "", "image file to open in the editor")
	fs.StringVar(&a.selector, "select", "", "selector for screen or window capture")
	fs.StringVar(&a.rect, "rect", "", "capture rectangle x0,y0,x1,y1 when targeting a region")
	fs.BoolVar(&a.shadow, "shadow", false, "apply a drop shadow before opening the editor")
	fs.IntVar(&a.shadowRadius, "shadow-radius", defaults.Radius, "drop shadow blur radius in pixels")
	fs.StringVar(&a.shadowOffset, "shadow-offset", formatShadowOffset(defaults.Offset), "drop shadow offset as dx,dy")
	fs.Float64Var(&a.shadowOpacity, "shadow-opacity", defaults.Opacity, "drop shadow opacity between 0 and 1")
	fs.BoolVar(&a.fromClipboard, "from-clipboard", false, "load the input image from the clipboard")
	fs.BoolVar(&a.fromClipboard, "from-clip", false, "load the input image from the clipboard (alias)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	pt, err := parseShadowOffset(a.shadowOffset)
	if err != nil {
		return nil, err
	}
	a.shadowPoint = pt
	operands := fs.Args()
	if len(operands) == 0 {
		return nil, &UsageError{of: a}
	}
	a.action = strings.ToLower(strings.TrimSpace(operands[0]))
	switch a.action {
	case "capture":
		if a.fromClipboard {
			return nil, fmt.Errorf("-from-clipboard is not supported with annotate capture")
		}
		if len(operands) < 2 {
			return nil, &UsageError{of: a}
		}
		a.target = strings.ToLower(strings.TrimSpace(operands[1]))
		switch a.target {
		case "screen", "window", "region":
		default:
			return nil, &UsageError{of: a}
		}
		if len(operands) > 2 {
			arg := strings.TrimSpace(strings.Join(operands[2:], " "))
			if a.target == "region" {
				if a.rect == "" {
					a.rect = arg
				}
			} else if a.selector == "" {
				a.selector = arg
			}
		}
		if a.target != "region" && strings.TrimSpace(a.rect) != "" {
			return nil, &UsageError{of: a}
		}
	case "open":
		if a.file == "" && len(operands) > 1 {
			a.file = strings.TrimSpace(strings.Join(operands[1:], " "))
		}
		if !a.fromClipboard {
			if a.file == "" {
				return nil, &UsageError{of: a}
			}
		}
		a.output = a.file
	default:
		return nil, &UsageError{of: a}
	}
	return a, nil
}

func (a *annotateCmd) Run() error {
	var img *image.RGBA
	switch a.action {
	case "capture":
		var err error
		opts := capture.CaptureOptions{}
		switch a.target {
		case "screen":
			img, err = captureScreenshotFn(a.selector, opts)
		case "window":
			img, err = captureWindowFn(a.selector, opts)
		case "region":
			rectSpec := a.rect
			if rectSpec == "" {
				rectSpec = a.selector
			}
			if strings.TrimSpace(rectSpec) == "" {
				img, err = captureRegionFn(opts)
			} else {
				var rect image.Rectangle
				rect, err = parseRect(rectSpec)
				if err == nil {
					img, err = captureRegionRectFn(rect, opts)
				}
			}
		}
		if err != nil {
			return fmt.Errorf("failed to capture %s: %w", a.target, err)
		}
	case "open":
		if a.fromClipboard {
			src, err := clipboard.ReadImage()
			if err != nil {
				return fmt.Errorf("read clipboard image: %w", err)
			}
			img = image.NewRGBA(src.Bounds())
			draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
		} else {
			f, err := os.Open(a.file)
			if err != nil {
				return fmt.Errorf("open %q: %w", a.file, err)
			}
			dec, err := png.Decode(f)
			if cerr := f.Close(); cerr != nil && err == nil {
				err = cerr
			}
			if err != nil {
				return fmt.Errorf("decode %q: %w", a.file, err)
			}
			img = image.NewRGBA(dec.Bounds())
			draw.Draw(img, img.Bounds(), dec, image.Point{}, draw.Src)
		}
	}
	shadowOpts := a.shadowOptions()
	initialShadowOffset := image.Point{}
	if a.shadow && img != nil {
		res := render.ApplyShadow(img, shadowOpts)
		initialShadowOffset = image.Pt(-res.Offset.X, -res.Offset.Y)
		img = res.Image
	}
	if a.action == "capture" && a.root != nil {
		a.root.notifyCapture(a.captureDetail(), img)
	}
	detail := ""
	fileName := ""
	if a.action == "open" && a.file != "" {
		fileName = filepath.Base(a.file)
	}
	if a.output != "" {
		detail = filepath.Base(a.output)
	}
	lastSaved := detail
	opts := []appstate.Option{
		appstate.WithImage(img),
		appstate.WithOutput(a.output),
		appstate.WithTitle(windowTitle(titleOptions{
			File:      fileName,
			Mode:      "Annotate",
			Detail:    detail,
			Tab:       "Tab 1",
			LastSaved: lastSaved,
		})),
		appstate.WithVersion(version),
		appstate.WithShadowDefaults(shadowOpts),
		appstate.WithInitialShadowApplied(a.shadow),
		appstate.WithInitialShadowOffset(initialShadowOffset),
	}
	if strings.TrimSpace(a.output) != "" {
		opts = append(opts, appstate.WithOutput(a.output))
	}
	st := appstate.New(opts...)
	st.Run()
	return nil
}

func (a *annotateCmd) shadowOptions() render.ShadowOptions {
	opts := render.DefaultShadowOptions()
	if a.shadowRadius >= 0 {
		opts.Radius = a.shadowRadius
	} else {
		opts.Radius = 0
	}
	opts.Offset = a.shadowPoint
	if a.shadowOpacity <= 0 {
		opts.Opacity = 0
	} else if a.shadowOpacity >= 1 {
		opts.Opacity = 1
	} else {
		opts.Opacity = a.shadowOpacity
	}
	return opts
}

func (a *annotateCmd) captureDetail() string {
	mode := strings.TrimSpace(a.target)
	switch mode {
	case "screen":
		if strings.TrimSpace(a.selector) != "" {
			return fmt.Sprintf("screen %s", a.selector)
		}
	case "window":
		if strings.TrimSpace(a.selector) != "" {
			return fmt.Sprintf("window %s", a.selector)
		}
	case "region":
		if strings.TrimSpace(a.rect) != "" {
			return fmt.Sprintf("region %s", a.rect)
		}
	}
	if mode == "" {
		return "capture"
	}
	return fmt.Sprintf("%s capture", mode)
}
