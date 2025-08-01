package main

import (
	"flag"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
)

// annotateCmd represents the annotate subcommand.
type annotateCmd struct {
	mode   string
	file   string
	output string
	*root
	fs *flag.FlagSet
}

func (a *annotateCmd) FlagSet() *flag.FlagSet {
	return a.fs
}

func parseAnnotateCmd(args []string, r *root) (*annotateCmd, error) {
	fs := flag.NewFlagSet("annotate", flag.ExitOnError)
	file := fs.String("file", "", "image file to annotate")
	output := fs.String("output", "annotated.png", "output file path")
	a := &annotateCmd{file: *file, output: *output, root: r, fs: fs}
	if len(args) < 1 {
		return nil, &UsageError{of: a}
	}
	a.mode = args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *annotateCmd) Run() error {
	var img *image.RGBA
	switch a.mode {
	case "capture-screen":
		var err error
		img, err = capture.CaptureScreenshot()
		if err != nil {
			return err
		}
	case "capture-window":
		var err error
		img, err = capture.CaptureWindow()
		if err != nil {
			return err
		}
	case "capture-region":
		var err error
		img, err = capture.CaptureRegion()
		if err != nil {
			return err
		}
	case "open-file":
		if a.file == "" {
			return &UsageError{of: a}
		}
		f, err := os.Open(a.file)
		if err != nil {
			return err
		}
		defer f.Close()
		dec, err := png.Decode(f)
		if err != nil {
			return err
		}
		img = image.NewRGBA(dec.Bounds())
		draw.Draw(img, img.Bounds(), dec, image.Point{}, draw.Src)
	default:
		return &UsageError{of: a}
	}
	st := appstate.New(appstate.WithImage(img), appstate.WithOutput(a.output))
	st.Run()
	return nil
}
