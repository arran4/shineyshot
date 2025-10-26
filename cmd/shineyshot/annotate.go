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
	action string
	output string

	capture annotateCaptureConfig
	open    annotateOpenConfig

	shadow        bool
	shadowRadius  int
	shadowOffset  string
	shadowPoint   image.Point
	shadowOpacity float64

	commonFlags  *flag.FlagSet
	captureFlags *flag.FlagSet
	openFlags    *flag.FlagSet

	*root
	fs *flag.FlagSet
}

type annotateCaptureConfig struct {
	target             string
	selector           string
	rect               string
	includeDecorations bool
	includeCursor      bool
}

type annotateOpenConfig struct {
	file          string
	fromClipboard bool
}

type annotateFlagGroup struct {
	Title   string
	FlagSet *flag.FlagSet
}

func (a *annotateCmd) FlagSet() *flag.FlagSet {
	return a.fs
}

func parseAnnotateCmd(args []string, r *root) (*annotateCmd, error) {
	fs := flag.NewFlagSet("annotate", flag.ExitOnError)
	a := &annotateCmd{
		root:         r,
		fs:           fs,
		commonFlags:  flag.NewFlagSet("common", flag.ContinueOnError),
		captureFlags: flag.NewFlagSet("capture", flag.ContinueOnError),
		openFlags:    flag.NewFlagSet("open", flag.ContinueOnError),
	}
	fs.Usage = usageFunc(a)
	defaults := render.DefaultShadowOptions()
	stringFlag(fs, &a.open.file, "file", "", "image file to open in the editor", a.openFlags)
	stringFlag(fs, &a.capture.selector, "select", "", "selector for screen or window capture", a.captureFlags)
	stringFlag(fs, &a.capture.rect, "rect", "", "capture rectangle x0,y0,x1,y1 when targeting a region", a.captureFlags)
	boolFlag(fs, &a.shadow, "shadow", false, "apply a drop shadow before opening the editor", a.commonFlags)
	intFlag(fs, &a.shadowRadius, "shadow-radius", defaults.Radius, "drop shadow blur radius in pixels", a.commonFlags)
	stringFlag(fs, &a.shadowOffset, "shadow-offset", formatShadowOffset(defaults.Offset), "drop shadow offset as dx,dy", a.commonFlags)
	floatFlag(fs, &a.shadowOpacity, "shadow-opacity", defaults.Opacity, "drop shadow opacity between 0 and 1", a.commonFlags)
	boolFlag(fs, &a.open.fromClipboard, "from-clipboard", false, "load the input image from the clipboard", a.openFlags)
	boolFlag(fs, &a.open.fromClipboard, "from-clip", false, "load the input image from the clipboard (alias)", a.openFlags)
	boolFlag(fs, &a.capture.includeDecorations, "include-decorations", false, "request window decorations when capturing windows", a.captureFlags)
	boolFlag(fs, &a.capture.includeCursor, "include-cursor", false, "embed the cursor in captures when supported", a.captureFlags)
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
		if a.open.fromClipboard {
			return nil, fmt.Errorf("-from-clipboard is not supported with annotate capture")
		}
		if strings.TrimSpace(a.open.file) != "" {
			return nil, fmt.Errorf("-file cannot be used with annotate capture")
		}
		if len(operands) < 2 {
			return nil, &UsageError{of: a}
		}
		a.capture.target = strings.ToLower(strings.TrimSpace(operands[1]))
		switch a.capture.target {
		case "screen", "window", "region":
		default:
			return nil, &UsageError{of: a}
		}
		if len(operands) > 2 {
			arg := strings.TrimSpace(strings.Join(operands[2:], " "))
			if a.capture.target == "region" {
				if a.capture.rect == "" {
					a.capture.rect = arg
				}
			} else if a.capture.selector == "" {
				a.capture.selector = arg
			}
		}
		if a.capture.target != "region" && strings.TrimSpace(a.capture.rect) != "" {
			return nil, &UsageError{of: a}
		}
	case "open":
		if a.open.file == "" && len(operands) > 1 {
			a.open.file = strings.TrimSpace(strings.Join(operands[1:], " "))
		}
		if !a.open.fromClipboard {
			if a.open.file == "" {
				return nil, &UsageError{of: a}
			}
		}
		a.output = a.open.file
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
		opts := capture.CaptureOptions{
			IncludeDecorations: a.capture.includeDecorations,
			IncludeCursor:      a.capture.includeCursor,
		}
		switch a.capture.target {
		case "screen":
			img, err = captureScreenshotFn(a.capture.selector, opts)
		case "window":
			img, err = captureWindowFn(a.capture.selector, opts)
		case "region":
			rectSpec := a.capture.rect
			if rectSpec == "" {
				rectSpec = a.capture.selector
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
			return fmt.Errorf("failed to capture %s: %w", a.capture.target, err)
		}
	case "open":
		if a.open.fromClipboard {
			src, err := clipboard.ReadImage()
			if err != nil {
				return fmt.Errorf("read clipboard image: %w", err)
			}
			img = image.NewRGBA(src.Bounds())
			draw.Draw(img, img.Bounds(), src, image.Point{}, draw.Src)
		} else {
			f, err := os.Open(a.open.file)
			if err != nil {
				return fmt.Errorf("open %q: %w", a.open.file, err)
			}
			dec, err := png.Decode(f)
			if cerr := f.Close(); cerr != nil && err == nil {
				err = cerr
			}
			if err != nil {
				return fmt.Errorf("decode %q: %w", a.open.file, err)
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
	if a.action == "open" && a.open.file != "" {
		fileName = filepath.Base(a.open.file)
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
	mode := strings.TrimSpace(a.capture.target)
	switch mode {
	case "screen":
		if strings.TrimSpace(a.capture.selector) != "" {
			return fmt.Sprintf("screen %s", a.capture.selector)
		}
	case "window":
		if strings.TrimSpace(a.capture.selector) != "" {
			return fmt.Sprintf("window %s", a.capture.selector)
		}
	case "region":
		if strings.TrimSpace(a.capture.rect) != "" {
			return fmt.Sprintf("region %s", a.capture.rect)
		}
	}
	if mode == "" {
		return "capture"
	}
	return fmt.Sprintf("%s capture", mode)
}

func (a *annotateCmd) FlagGroups() []annotateFlagGroup {
	groups := []annotateFlagGroup{}
	if hasDefinedFlags(a.commonFlags) {
		groups = append(groups, annotateFlagGroup{Title: "Common flags", FlagSet: a.commonFlags})
	}
	if hasDefinedFlags(a.captureFlags) {
		groups = append(groups, annotateFlagGroup{Title: "Capture-only flags", FlagSet: a.captureFlags})
	}
	if hasDefinedFlags(a.openFlags) {
		groups = append(groups, annotateFlagGroup{Title: "Open-only flags", FlagSet: a.openFlags})
	}
	return groups
}

func stringFlag(fs *flag.FlagSet, target *string, name, value, usage string, groups ...*flag.FlagSet) {
	fs.StringVar(target, name, value, usage)
	for _, group := range groups {
		if group != nil {
			group.StringVar(new(string), name, value, usage)
		}
	}
}

func boolFlag(fs *flag.FlagSet, target *bool, name string, value bool, usage string, groups ...*flag.FlagSet) {
	fs.BoolVar(target, name, value, usage)
	for _, group := range groups {
		if group != nil {
			group.BoolVar(new(bool), name, value, usage)
		}
	}
}

func intFlag(fs *flag.FlagSet, target *int, name string, value int, usage string, groups ...*flag.FlagSet) {
	fs.IntVar(target, name, value, usage)
	for _, group := range groups {
		if group != nil {
			group.IntVar(new(int), name, value, usage)
		}
	}
}

func floatFlag(fs *flag.FlagSet, target *float64, name string, value float64, usage string, groups ...*flag.FlagSet) {
	fs.Float64Var(target, name, value, usage)
	for _, group := range groups {
		if group != nil {
			group.Float64Var(new(float64), name, value, usage)
		}
	}
}

func hasDefinedFlags(fs *flag.FlagSet) bool {
	if fs == nil {
		return false
	}
	count := 0
	fs.VisitAll(func(*flag.Flag) {
		count++
	})
	return count > 0
}
