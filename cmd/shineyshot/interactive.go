package main

import (
	"bufio"
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
)

type interactiveCmd struct {
	r *root

	mu     sync.RWMutex
	img    *image.RGBA
	output string
	state  *appstate.AppState

	colorIdx int
	widthIdx int

	palette []color.RGBA
	widths  []int
}

func newInteractiveCmd(r *root) *interactiveCmd {
	palette := appstate.Palette()
	widths := appstate.WidthOptions()
	return &interactiveCmd{
		r:        r,
		colorIdx: clampIndex(appstate.DefaultColorIndex(), len(palette)),
		widthIdx: clampIndex(appstate.DefaultWidthIndex(), len(widths)),
		palette:  palette,
		widths:   widths,
	}
}

func (i *interactiveCmd) Run() error {
	fmt.Fprintln(os.Stdout, "Interactive mode. Type 'help' for commands.")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Fprint(os.Stdout, "> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		cmd := strings.ToLower(fields[0])
		args := fields[1:]

		switch cmd {
		case "quit", "exit":
			return nil
		case "help":
			i.printHelp()
		case "capture":
			i.handleCapture(args)
		case "arrow":
			i.handleArrow(args)
		case "line":
			i.handleLine(args)
		case "rect":
			i.handleRect(args)
		case "circle":
			i.handleCircle(args)
		case "crop":
			i.handleCrop(args)
		case "color":
			i.handleColor(args)
		case "width":
			i.handleWidth(args)
		case "show":
			i.handleShow(false)
		case "preview":
			i.handleShow(true)
		case "save":
			i.handleSave(args)
		case "copy":
			i.handleCopy()
		case "copyname":
			i.handleCopyName()
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n", cmd)
		}
	}
	return scanner.Err()
}

func (i *interactiveCmd) printHelp() {
	fmt.Fprintln(os.Stdout, "Commands:")
	fmt.Fprintln(os.Stdout, "  capture screen DISPLAY     capture full screen on DISPLAY")
	fmt.Fprintln(os.Stdout, "  capture window WINDOW      capture WINDOW by identifier")
	fmt.Fprintln(os.Stdout, "  capture region             capture screen region")
	fmt.Fprintln(os.Stdout, "  arrow x0 y0 x1 y1          draw arrow with current stroke")
	fmt.Fprintln(os.Stdout, "  line x0 y0 x1 y1           draw line with current stroke")
	fmt.Fprintln(os.Stdout, "  rect x0 y0 x1 y1           draw rectangle with current stroke")
	fmt.Fprintln(os.Stdout, "  circle x y r               draw circle with current stroke")
	fmt.Fprintln(os.Stdout, "  crop x0 y0 x1 y1           crop image to rectangle")
	fmt.Fprintln(os.Stdout, "  color [index|list]         set or list palette colors")
	fmt.Fprintln(os.Stdout, "  width [value|list]         set or list stroke widths")
	fmt.Fprintln(os.Stdout, "  show                       open synced annotation window")
	fmt.Fprintln(os.Stdout, "  preview                    open copy in separate window")
	fmt.Fprintln(os.Stdout, "  save FILE                  save image to FILE")
	fmt.Fprintln(os.Stdout, "  copy                       copy image to clipboard")
	fmt.Fprintln(os.Stdout, "  copyname                   copy last saved filename")
	fmt.Fprintln(os.Stdout, "  quit                       exit interactive mode")
}

func (i *interactiveCmd) handleCapture(args []string) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: capture [screen|window|region] ...")
		return
	}
	mode := strings.ToLower(args[0])
	var (
		img    *image.RGBA
		err    error
		target string
	)
	switch mode {
	case "screen":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: capture screen DISPLAY")
			return
		}
		target = args[1]
		img, err = capture.CaptureScreenshot(target)
	case "window":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: capture window WINDOW")
			return
		}
		target = args[1]
		img, err = capture.CaptureWindow(target)
	case "region":
		img, err = capture.CaptureRegion()
	default:
		fmt.Fprintln(os.Stderr, "usage: capture [screen|window|region] ...")
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.setImage(img)
	if target != "" {
		fmt.Fprintf(os.Stdout, "captured %s %s\n", mode, target)
	} else {
		fmt.Fprintf(os.Stdout, "captured %s\n", mode)
	}
}

func (i *interactiveCmd) handleArrow(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawArrow(img, vals[0], vals[1], vals[2], vals[3], col, width)
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "arrow drawn")
}

func (i *interactiveCmd) handleLine(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawLine(img, vals[0], vals[1], vals[2], vals[3], col, width)
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "line drawn")
}

func (i *interactiveCmd) handleRect(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawRect(img, image.Rect(vals[0], vals[1], vals[2], vals[3]), col, width)
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "rectangle drawn")
}

func (i *interactiveCmd) handleCircle(args []string) {
	vals, err := parseInts(args, 3)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawCircle(img, vals[0], vals[1], vals[2], col, width)
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "circle drawn")
}

func (i *interactiveCmd) handleCrop(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		cropped := appstate.CropImage(img, image.Rect(vals[0], vals[1], vals[2], vals[3]))
		*img = *cropped
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "cropped")
}

func (i *interactiveCmd) handleColor(args []string) {
	if len(args) == 0 {
		i.mu.RLock()
		idx := i.colorIdx
		i.mu.RUnlock()
		fmt.Fprintf(os.Stdout, "current color: %d %s\n", idx, formatColor(i.palette, idx))
		return
	}
	if strings.ToLower(args[0]) == "list" {
		for idx, col := range i.palette {
			fmt.Fprintf(os.Stdout, "  %2d: #%02X%02X%02X\n", idx, col.R, col.G, col.B)
		}
		return
	}
	idx, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid color index %q\n", args[0])
		return
	}
	if idx < 0 || idx >= len(i.palette) {
		fmt.Fprintf(os.Stderr, "color index must be between 0 and %d\n", len(i.palette)-1)
		return
	}
	i.mu.Lock()
	i.colorIdx = idx
	widthIdx := i.widthIdx
	state := i.state
	i.mu.Unlock()
	if state != nil {
		state.ApplySettings(idx, widthIdx)
	}
	fmt.Fprintf(os.Stdout, "color set to %d %s\n", idx, formatColor(i.palette, idx))
}

func (i *interactiveCmd) handleWidth(args []string) {
	if len(args) == 0 {
		i.mu.RLock()
		idx := i.widthIdx
		width := widthAt(i.widths, idx)
		i.mu.RUnlock()
		fmt.Fprintf(os.Stdout, "current width: %dpx\n", width)
		return
	}
	if strings.ToLower(args[0]) == "list" {
		for _, w := range i.widths {
			fmt.Fprintf(os.Stdout, "  %dpx\n", w)
		}
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid width %q\n", args[0])
		return
	}
	idx := findWidthIndex(i.widths, val)
	if idx == -1 {
		fmt.Fprintf(os.Stderr, "unsupported width %d\n", val)
		return
	}
	i.mu.Lock()
	i.widthIdx = idx
	colorIdx := i.colorIdx
	state := i.state
	i.mu.Unlock()
	if state != nil {
		state.ApplySettings(colorIdx, idx)
	}
	fmt.Fprintf(os.Stdout, "width set to %dpx\n", i.widths[idx])
}

func (i *interactiveCmd) handleShow(copyImage bool) {
	i.mu.Lock()
	if i.img == nil {
		i.mu.Unlock()
		fmt.Fprintln(os.Stderr, "no image loaded")
		return
	}
	if copyImage {
		dup := image.NewRGBA(i.img.Bounds())
		draw.Draw(dup, dup.Bounds(), i.img, image.Point{}, draw.Src)
		output := i.output
		colorIdx := i.colorIdx
		widthIdx := i.widthIdx
		i.mu.Unlock()
		st := appstate.New(
			appstate.WithImage(dup),
			appstate.WithOutput(output),
			appstate.WithColorIndex(colorIdx),
			appstate.WithWidthIndex(widthIdx),
		)
		go st.Run()
		fmt.Fprintln(os.Stdout, "preview window opened")
		return
	}
	if i.state != nil {
		i.mu.Unlock()
		fmt.Fprintln(os.Stderr, "annotation window already open")
		return
	}
	img := i.img
	output := i.output
	colorIdx := i.colorIdx
	widthIdx := i.widthIdx
	var st *appstate.AppState
	onClose := func() {
		i.mu.Lock()
		if i.state == st {
			i.state = nil
		}
		i.mu.Unlock()
		i.r.state = nil
	}
	st = appstate.New(
		appstate.WithImage(img),
		appstate.WithOutput(output),
		appstate.WithColorIndex(colorIdx),
		appstate.WithWidthIndex(widthIdx),
		appstate.WithSettingsListener(func(cIdx, wIdx int) {
			i.mu.Lock()
			i.colorIdx = cIdx
			i.widthIdx = wIdx
			i.mu.Unlock()
		}),
		appstate.WithOnClose(onClose),
	)
	i.state = st
	i.r.state = st
	i.mu.Unlock()
	go st.Run()
	fmt.Fprintln(os.Stdout, "annotation window opened")
}

func (i *interactiveCmd) handleSave(args []string) {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "usage: save FILE")
		return
	}
	path := args[0]
	if err := i.withImage(false, func(img *image.RGBA) error {
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.mu.Lock()
	i.output = path
	i.mu.Unlock()
	fmt.Fprintf(os.Stdout, "saved %s\n", path)
}

func (i *interactiveCmd) handleCopy() {
	if err := i.withImage(false, func(img *image.RGBA) error {
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		cmd := exec.Command("wl-copy", "--type", "image/png")
		cmd.Stdin = &buf
		return cmd.Run()
	}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "image copied to clipboard")
}

func (i *interactiveCmd) handleCopyName() {
	i.mu.RLock()
	output := i.output
	i.mu.RUnlock()
	if output == "" {
		fmt.Fprintln(os.Stderr, "no saved file")
		return
	}
	cmd := exec.Command("wl-copy")
	cmd.Stdin = strings.NewReader(output)
	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Fprintln(os.Stdout, "filename copied to clipboard")
}

func (i *interactiveCmd) withImage(write bool, fn func(img *image.RGBA) error) error {
	if write {
		i.mu.Lock()
		defer i.mu.Unlock()
		if i.img == nil {
			return fmt.Errorf("no image loaded")
		}
		if err := fn(i.img); err != nil {
			return err
		}
		i.notifyLocked()
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	if i.img == nil {
		return fmt.Errorf("no image loaded")
	}
	return fn(i.img)
}

func (i *interactiveCmd) setImage(img *image.RGBA) {
	if img == nil {
		return
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.img == nil || i.state == nil {
		i.img = img
	} else {
		*i.img = *img
	}
	i.output = ""
	i.notifyLocked()
}

func (i *interactiveCmd) notifyLocked() {
	if i.state != nil {
		i.state.NotifyImageChanged()
	}
}

func (i *interactiveCmd) strokeLocked() (color.Color, int) {
	idx := clampIndex(i.colorIdx, len(i.palette))
	widthIdx := clampIndex(i.widthIdx, len(i.widths))
	return i.palette[idx], i.widths[widthIdx]
}

func parseInts(args []string, count int) ([]int, error) {
	if len(args) != count {
		return nil, fmt.Errorf("expected %d arguments", count)
	}
	vals := make([]int, count)
	for idx, arg := range args {
		v, err := strconv.Atoi(arg)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q", arg)
		}
		vals[idx] = v
	}
	return vals, nil
}

func clampIndex(idx, size int) int {
	if size == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= size {
		return size - 1
	}
	return idx
}

func formatColor(palette []color.RGBA, idx int) string {
	if len(palette) == 0 {
		return ""
	}
	idx = clampIndex(idx, len(palette))
	col := palette[idx]
	return fmt.Sprintf("(#%02X%02X%02X)", col.R, col.G, col.B)
}

func findWidthIndex(widths []int, target int) int {
	for idx, w := range widths {
		if w == target {
			return idx
		}
	}
	return -1
}

func widthAt(widths []int, idx int) int {
	if len(widths) == 0 {
		return 0
	}
	idx = clampIndex(idx, len(widths))
	return widths[idx]
}
