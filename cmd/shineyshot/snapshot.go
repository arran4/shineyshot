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
)

type snapshotCmd struct {
	output             string
	stdout             bool
	mode               string
	selector           string
	rect               string
	includeDecorations bool
	includeCursor      bool
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
	fs.StringVar(&s.output, "output", "screenshot.png", "write the capture to this file path")
	fs.BoolVar(&s.stdout, "stdout", false, "write PNG data to stdout")
	fs.StringVar(&s.selector, "select", "", "selector for screen or window capture")
	fs.StringVar(&s.rect, "rect", "", "capture rectangle x0,y0,x1,y1 when targeting a region")
	fs.BoolVar(&s.includeDecorations, "include-decorations", false, "request window decorations when capturing windows")
	fs.BoolVar(&s.includeCursor, "include-cursor", false, "embed the cursor in captures when supported")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	operands := fs.Args()
	if len(operands) == 0 {
		return nil, &UsageError{of: s}
	}
	if strings.EqualFold(operands[0], "capture") {
		operands = operands[1:]
	}
	if len(operands) == 0 {
		return nil, &UsageError{of: s}
	}
	s.mode = strings.ToLower(strings.TrimSpace(operands[0]))
	switch s.mode {
	case "screen", "window", "region":
	default:
		return nil, &UsageError{of: s}
	}
	if len(operands) > 1 {
		arg := strings.TrimSpace(strings.Join(operands[1:], " "))
		if s.mode == "region" {
			if s.rect == "" {
				s.rect = arg
			}
		} else if s.selector == "" {
			s.selector = arg
		}
	}
	return s, nil
}

func (s *snapshotCmd) Run() error {
	img, err := s.capture()
	if err != nil {
		return err
	}
	if s.root != nil {
		detail := s.describeCapture()
		s.root.notifyCapture(detail, img)
	}
	var w io.Writer
	if s.stdout {
		w = os.Stdout
	} else {
		f, err := os.Create(s.output)
		if err != nil {
			return err
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				log.Printf("close %s: %v", s.output, cerr)
			}
		}()
		w = f
	}
	if err := png.Encode(w, img); err != nil {
		return err
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
		return capture.CaptureScreenshot(s.selector, opts)
	case "window":
		return capture.CaptureWindow(s.selector, opts)
	case "region":
		if strings.TrimSpace(s.rect) == "" {
			return capture.CaptureRegion(opts)
		}
		rect, err := parseRect(s.rect)
		if err != nil {
			return nil, err
		}
		return capture.CaptureRegionRect(rect, opts)
	default:
		return nil, errors.New("unsupported capture mode")
	}
}

func (s *snapshotCmd) describeCapture() string {
	mode := strings.TrimSpace(s.mode)
	switch mode {
	case "screen":
		if strings.TrimSpace(s.selector) != "" {
			return fmt.Sprintf("screen %s", s.selector)
		}
	case "window":
		if strings.TrimSpace(s.selector) != "" {
			return fmt.Sprintf("window %s", s.selector)
		}
	case "region":
		if strings.TrimSpace(s.rect) != "" {
			return fmt.Sprintf("region %s", s.rect)
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
