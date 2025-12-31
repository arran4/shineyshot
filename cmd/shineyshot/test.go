package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"time"

	"github.com/example/shineyshot/internal/appstate"
)

type testVerificationCmd struct {
	*root
	fs     *flag.FlagSet
	input  string
	output string
}

func parseTestCmd(args []string, r *root) (*testVerificationCmd, error) {
	fs := flag.NewFlagSet("test", flag.ExitOnError)
	c := &testVerificationCmd{
		root: r,
		fs:   fs,
	}
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	if fs.NArg() < 1 {
		return nil, &UsageError{of: c}
	}
	cmd := fs.Arg(0)
	if cmd != "verification" {
		return nil, &UsageError{of: c}
	}
	c.fs = flag.NewFlagSet("test verification", flag.ExitOnError)
	c.fs.StringVar(&c.input, "input", "", "input configuration file (JSON)")
	c.fs.StringVar(&c.output, "output", "", "output PNG file")
	if err := c.fs.Parse(fs.Args()[1:]); err != nil {
		return nil, err
	}
	if c.input == "" || c.output == "" {
		return nil, &UsageError{of: c}
	}
	return c, nil
}

type VerificationConfig struct {
	Width             int         `json:"width"`
	Height            int         `json:"height"`
	CurrentTab        int         `json:"current_tab"`
	Tool              int         `json:"tool"`
	ColorIdx          int         `json:"color_idx"`
	NumberIdx         int         `json:"number_idx"`
	Cropping          bool        `json:"cropping"`
	CropRect          [4]int      `json:"crop_rect"`
	CropStart         [2]int      `json:"crop_start"`
	TextInputActive   bool        `json:"text_input_active"`
	TextInput         string      `json:"text_input"`
	TextPos           [2]int      `json:"text_pos"`
	Message           string      `json:"message"`
	AnnotationEnabled bool        `json:"annotation_enabled"`
	VersionLabel      string      `json:"version_label"`
	Tabs              []TabConfig `json:"tabs"`
}

type TabConfig struct {
	Title         string  `json:"title"`
	Offset        [2]int  `json:"offset"`
	Zoom          float64 `json:"zoom"`
	NextNumber    int     `json:"next_number"`
	WidthIdx      int     `json:"width_idx"`
	ShadowApplied bool    `json:"shadow_applied"`
	ImageColor    [4]int  `json:"image_color"` // R, G, B, A
	ImageSize     [2]int  `json:"image_size"`
}

func (c *testVerificationCmd) Run() error {
	f, err := os.Open(c.input)
	if err != nil {
		return fmt.Errorf("open input: %w", err)
	}
	defer f.Close()

	var cfg VerificationConfig
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return fmt.Errorf("decode input: %w", err)
	}

	tabs := make([]appstate.Tab, len(cfg.Tabs))
	for i, t := range cfg.Tabs {
		img := image.NewRGBA(image.Rect(0, 0, t.ImageSize[0], t.ImageSize[1]))
		col := color.RGBA{
			R: uint8(t.ImageColor[0]),
			G: uint8(t.ImageColor[1]),
			B: uint8(t.ImageColor[2]),
			A: uint8(t.ImageColor[3]),
		}
		draw.Draw(img, img.Bounds(), &image.Uniform{col}, image.Point{}, draw.Src)

		tabs[i] = appstate.Tab{
			Image:         img,
			Title:         t.Title,
			Offset:        image.Point{X: t.Offset[0], Y: t.Offset[1]},
			Zoom:          t.Zoom,
			NextNumber:    t.NextNumber,
			WidthIdx:      t.WidthIdx,
			ShadowApplied: t.ShadowApplied,
		}
	}

	st := appstate.PaintState{
		Width:             cfg.Width,
		Height:            cfg.Height,
		Tabs:              tabs,
		Current:           cfg.CurrentTab,
		Tool:              appstate.Tool(cfg.Tool),
		ColorIdx:          cfg.ColorIdx,
		NumberIdx:         cfg.NumberIdx,
		Cropping:          cfg.Cropping,
		CropRect:          image.Rect(cfg.CropRect[0], cfg.CropRect[1], cfg.CropRect[2], cfg.CropRect[3]),
		CropStart:         image.Point{X: cfg.CropStart[0], Y: cfg.CropStart[1]},
		TextInputActive:   cfg.TextInputActive,
		TextInput:         cfg.TextInput,
		TextPos:           image.Point{X: cfg.TextPos[0], Y: cfg.TextPos[1]},
		Message:           cfg.Message,
		MessageUntil:      time.Now().Add(time.Hour), // Ensure message is visible
		HandleShortcut:    func(string) {},
		AnnotationEnabled: cfg.AnnotationEnabled,
		VersionLabel:      cfg.VersionLabel,
	}

	if cfg.Message == "" {
		st.MessageUntil = time.Time{}
	}

	outImg := image.NewRGBA(image.Rect(0, 0, cfg.Width, cfg.Height))
	appstate.DrawScene(nil, outImg, st) // Passing nil context is fine as we don't need cancellation

	outF, err := os.Create(c.output)
	if err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	defer outF.Close()

	if err := png.Encode(outF, outImg); err != nil {
		return fmt.Errorf("encode output: %w", err)
	}

	return nil
}

func (c *testVerificationCmd) FlagSet() *flag.FlagSet {
	return c.fs
}
