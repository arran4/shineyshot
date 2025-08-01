package main

import (
	"errors"
	"flag"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/example/shineyshot/internal/appstate"
)

type previewCmd struct {
	file string
	root *root
}

func parsePreviewCmd(args []string, r *root) (*previewCmd, error) {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	file := fs.String("file", "", "image file to open")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if *file == "" {
		return nil, errors.New(previewHelp(fs, r))
	}
	return &previewCmd{file: *file, root: r}, nil
}

func (c *previewCmd) Run() error {
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
	st := appstate.New(appstate.WithImage(rgba))
	st.Run()
	return nil
}
