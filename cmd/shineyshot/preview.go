package main

import (
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/clipboard"
)

type previewCmd struct {
	file          string
	fromClipboard bool
	*root
	fs *flag.FlagSet
}

func (p *previewCmd) FlagSet() *flag.FlagSet {
	return p.fs
}

func parsePreviewCmd(args []string, r *root) (*previewCmd, error) {
	fs := flag.NewFlagSet("preview", flag.ExitOnError)
	c := &previewCmd{root: r, fs: fs}
	fs.Usage = usageFunc(c)
	fs.StringVar(&c.file, "file", "", "image file to open")
	fs.BoolVar(&c.fromClipboard, "from-clipboard", false, "load the input image from the clipboard")
	fs.BoolVar(&c.fromClipboard, "from-clip", false, "load the input image from the clipboard (alias)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if !c.fromClipboard && c.file == "" {
		return nil, &UsageError{of: c}
	}
	return c, nil
}

func (p *previewCmd) Run() error {
	var (
		src image.Image
		err error
	)
	if p.fromClipboard {
		src, err = clipboard.ReadImage()
		if err != nil {
			return fmt.Errorf("read clipboard image: %w", err)
		}
	} else {
		f, err := os.Open(p.file)
		if err != nil {
			return err
		}
		src, err = png.Decode(f)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, image.Point{}, draw.Src)
	fileName := ""
	if p.file != "" {
		fileName = filepath.Base(p.file)
	}
	st := appstate.New(
		appstate.WithImage(rgba),
		appstate.WithMode(appstate.ModePreview),
		appstate.WithTitle(windowTitle(titleOptions{
			File: fileName,
			Mode: "Preview",
			Tab:  "Tab 1",
		})),
	)
	st.Run()
	return nil
}
