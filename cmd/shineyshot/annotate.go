package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
)

// annotateCmd represents the annotate subcommand.
type annotateCmd struct {
	action   string
	target   string
	selector string
	rect     string
	file     string
	output   string
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
	fs.StringVar(&a.file, "file", "", "image file to open in the editor")
	fs.StringVar(&a.selector, "select", "", "selector for screen or window capture")
	fs.StringVar(&a.rect, "rect", "", "capture rectangle x0,y0,x1,y1 when targeting a region")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	operands := fs.Args()
	if len(operands) == 0 {
		return nil, &UsageError{of: a}
	}
	a.action = strings.ToLower(strings.TrimSpace(operands[0]))
	switch a.action {
	case "capture":
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
		if a.file == "" {
			if len(operands) < 2 {
				return nil, &UsageError{of: a}
			}
			a.file = strings.TrimSpace(strings.Join(operands[1:], " "))
		}
		if a.file == "" {
			return nil, &UsageError{of: a}
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
		switch a.target {
		case "screen":
			img, err = capture.CaptureScreenshot(a.selector)
		case "window":
			img, err = capture.CaptureWindow(a.selector)
		case "region":
			rectSpec := a.rect
			if rectSpec == "" {
				rectSpec = a.selector
			}
			if strings.TrimSpace(rectSpec) == "" {
				img, err = capture.CaptureRegion()
			} else {
				var rect image.Rectangle
				rect, err = parseRect(rectSpec)
				if err == nil {
					img, err = capture.CaptureRegionRect(rect)
				}
			}
		}
		if err != nil {
			return err
		}
		if a.root != nil {
			a.root.notifyCapture(a.captureDetail(), img)
		}
	case "open":
		f, err := os.Open(a.file)
		if err != nil {
			return err
		}
		dec, err := png.Decode(f)
		if cerr := f.Close(); cerr != nil {
			if err == nil {
				err = cerr
			}
		}
		if err != nil {
			return err
		}
		img = image.NewRGBA(dec.Bounds())
		draw.Draw(img, img.Bounds(), dec, image.Point{}, draw.Src)
	}
	opts := []appstate.Option{appstate.WithImage(img)}
	if strings.TrimSpace(a.output) != "" {
		opts = append(opts, appstate.WithOutput(a.output))
	}
	st := appstate.New(opts...)
	st.Run()
	return nil
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
