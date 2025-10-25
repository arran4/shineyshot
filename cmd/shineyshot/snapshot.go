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
	root   *root
}

func parseSnapshotCmd(args []string, r *root) (*snapshotCmd, error) {
	fs := flag.NewFlagSet("snapshot", flag.ExitOnError)
	output := fs.String("output", "screenshot.png", "output file path")
	stdout := fs.Bool("stdout", false, "write PNG to stdout")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &snapshotCmd{output: *output, stdout: *stdout, root: r}, nil
}

func (s *snapshotCmd) Run() error {
	img, err := capture.CaptureScreenshot("")
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
