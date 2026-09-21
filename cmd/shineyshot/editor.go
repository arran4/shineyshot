package main

import (
	"flag"
	"image"
	"image/color"
	"image/draw"

	"github.com/arran4/shineyshot/internal/appstate"
)

type editorCmd struct {
	*root
	fs *flag.FlagSet
}

func parseEditorCmd(args []string, r *root) (*editorCmd, error) {
	fs := flag.NewFlagSet("editor", flag.ExitOnError)
	c := &editorCmd{root: r, fs: fs}
	fs.Usage = usageFunc(c)
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *editorCmd) Run() error {
	// Create an empty, transparent image to prevent panics in appstate
	rect := image.Rect(0, 0, 800, 600)
	img := image.NewRGBA(rect)
	draw.Draw(img, img.Bounds(), &image.Uniform{color.Transparent}, image.Point{}, draw.Src)

	opts := []appstate.Option{
		appstate.WithImage(img),
		appstate.WithTitle(windowTitle(titleOptions{
			Mode: "Editor",
		})),
		appstate.WithVersion(version),
		appstate.WithTheme(c.activeTheme),
	}

	st := appstate.New(opts...)
	st.Run()
	return nil
}

func (c *editorCmd) FlagSet() *flag.FlagSet {
	return c.fs
}
