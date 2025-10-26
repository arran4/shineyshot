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
	output  string
	stdout  bool
	mode    string
	window  string
	display string
	region  string
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
	fs.StringVar(&s.output, "output", "screenshot.png", "output file path")
	fs.BoolVar(&s.stdout, "stdout", false, "write PNG to stdout")
	fs.StringVar(&s.mode, "mode", "screen", "capture mode: screen, window, or region")
	fs.StringVar(&s.window, "window", "", "window selector when mode is window")
	fs.StringVar(&s.display, "display", "", "monitor selector when mode is screen")
	fs.StringVar(&s.region, "region", "", "capture rectangle x0,y0,x1,y1 when mode is region")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	s.mode = strings.ToLower(strings.TrimSpace(s.mode))
	switch s.mode {
	case "screen", "window", "region":
	default:
		return nil, fmt.Errorf("invalid mode %q", s.mode)
	}
	return s, nil
}

func (s *snapshotCmd) Run() error {
	img, err := s.capture()
	if err != nil {
		return err
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
	if abs, err := filepath.Abs(s.output); err == nil {
		fmt.Fprintf(os.Stderr, "saved %s\n", abs)
	} else {
		fmt.Fprintf(os.Stderr, "saved %s\n", s.output)
	}
	return nil
}

func (s *snapshotCmd) capture() (*image.RGBA, error) {
	switch s.mode {
	case "screen":
		return capture.CaptureScreenshot(s.display)
	case "window":
		return capture.CaptureWindow(s.window)
	case "region":
		if strings.TrimSpace(s.region) == "" {
			return capture.CaptureRegion()
		}
		rect, err := parseRect(s.region)
		if err != nil {
			return nil, err
		}
		return capture.CaptureRegionRect(rect)
	default:
		return nil, errors.New("unsupported capture mode")
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
