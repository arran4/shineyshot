package main

import (
	"flag"
	"image/png"
	"io"
	"os"

	"github.com/example/shineyshot/internal/capture"
)

type snapshotCmd struct {
	output string
	stdout bool
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
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *snapshotCmd) Run() error {
	img, err := capture.CaptureScreenshot()
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
		defer f.Close()
		w = f
	}
	if err := png.Encode(w, img); err != nil {
		return err
	}
	return nil
}
