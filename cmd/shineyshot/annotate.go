package main

import (
	"errors"
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
	root   *root
}

func parseAnnotateCmd(args []string, r *root) (*annotateCmd, error) {
	fs := flag.NewFlagSet("annotate", flag.ExitOnError)
	file := fs.String("file", "", "image file to annotate")
	output := fs.String("output", "annotated.png", "output file path")
	if len(args) < 1 {
		return nil, errors.New(annotateHelp(fs, r))
	}
	mode := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return nil, err
	}
	return &annotateCmd{mode: mode, file: *file, output: *output, root: r}, nil
}

func (c *annotateCmd) Run() error {
	var img *image.RGBA
	switch c.mode {
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
		if c.file == "" {
			return errors.New(annotateHelp(flag.NewFlagSet("annotate", flag.ContinueOnError), c.root))
		}
		f, err := os.Open(c.file)
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
		return errors.New(annotateHelp(flag.NewFlagSet("annotate", flag.ContinueOnError), c.root))
	}
	st := appstate.New(appstate.WithImage(img), appstate.WithOutput(c.output))
	st.Run()
	return nil
}
