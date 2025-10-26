package main

import (
	"flag"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/example/shineyshot/internal/appstate"
)

type previewCmd struct {
	file string
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
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if c.file == "" {
		return nil, &UsageError{of: c}
	}
	return c, nil
}

func (p *previewCmd) Run() error {
	f, err := os.Open(p.file)
	if err != nil {
		return err
	}
	img, err := png.Decode(f)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	st := appstate.New(
		appstate.WithImage(rgba),
		appstate.WithMode(appstate.ModePreview),
	)
	st.Run()
	return nil
}
