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
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

	palette []appstate.PaletteColor
	widths  []int

	defaultPaletteSet map[color.RGBA]struct{}
	defaultWidthSet   map[int]struct{}
}

func newInteractiveCmd(r *root) *interactiveCmd {
	palette := appstate.PaletteColors()
	widths := appstate.WidthOptions()
	paletteSet := make(map[color.RGBA]struct{}, len(palette))
	for _, entry := range palette {
		paletteSet[entry.Color] = struct{}{}
	}
	widthSet := make(map[int]struct{}, len(widths))
	for _, w := range widths {
		widthSet[w] = struct{}{}
	}
	return &interactiveCmd{
		r:                 r,
		colorIdx:          clampIndex(appstate.DefaultColorIndex(), len(palette)),
		widthIdx:          clampIndex(appstate.DefaultWidthIndex(), len(widths)),
		palette:           palette,
		widths:            widths,
		defaultPaletteSet: paletteSet,
		defaultWidthSet:   widthSet,
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
		case "colors":
			i.handleColorList()
		case "width":
			i.handleWidth(args)
		case "widths":
			i.handleWidthList()
		case "show":
			i.handleShow(false)
		case "preview":
			i.handleShow(true)
		case "save":
			i.handleSave(args)
		case "savetmp":
			i.handleSaveTmp()
		case "savepictures":
			i.handleSavePictures()
		case "savehome":
			i.handleSaveHome()
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
	fmt.Fprintln(os.Stdout, "  capture screen [DISPLAY]   capture full screen (defaults to current display)")
	fmt.Fprintln(os.Stdout, "  capture window WINDOW      capture WINDOW by identifier (not implemented)")
	fmt.Fprintln(os.Stdout, "  capture region             capture screen region (not implemented)")
	fmt.Fprintln(os.Stdout, "  arrow x0 y0 x1 y1          draw arrow with current stroke")
	fmt.Fprintln(os.Stdout, "  line x0 y0 x1 y1           draw line with current stroke")
	fmt.Fprintln(os.Stdout, "  rect x0 y0 x1 y1           draw rectangle with current stroke")
	fmt.Fprintln(os.Stdout, "  circle x y r               draw circle with current stroke")
	fmt.Fprintln(os.Stdout, "  crop x0 y0 x1 y1           crop image to rectangle")
	fmt.Fprintln(os.Stdout, "  color [value|list]         set or list palette colors")
	fmt.Fprintln(os.Stdout, "  colors                     list palette colors")
	fmt.Fprintln(os.Stdout, "  width [value|list]         set or list stroke widths")
	fmt.Fprintln(os.Stdout, "  widths                     list stroke widths")
	fmt.Fprintln(os.Stdout, "  show                       open synced annotation window")
	fmt.Fprintln(os.Stdout, "  preview                    open copy in separate window")
	fmt.Fprintln(os.Stdout, "  save FILE                  save image to FILE")
	fmt.Fprintln(os.Stdout, "  savetmp                    save to /tmp with a unique filename")
	fmt.Fprintln(os.Stdout, "  savepictures               save to your Pictures directory")
	fmt.Fprintln(os.Stdout, "  savehome                   save to your home directory")
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
		display := ""
		if len(args) >= 2 {
			display = args[1]
		}
		img, err = capture.CaptureScreenshot(display)
		if err != nil && display == "" {
			display = "0"
			img, err = capture.CaptureScreenshot(display)
		}
		target = display
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
	} else if mode == "screen" {
		fmt.Fprintln(os.Stdout, "captured screen (current display)")
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
	i.refreshPalette()
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		i.printColorList()
		return
	}
	arg := args[0]
	if idx, err := strconv.Atoi(arg); err == nil {
		if idx < 0 || idx >= len(i.palette) {
			fmt.Fprintf(os.Stderr, "color index must be between 0 and %d\n", len(i.palette)-1)
			return
		}
		i.applyColorIndex(idx)
		return
	}
	for idx, entry := range i.palette {
		if entry.Name != "" && strings.EqualFold(entry.Name, arg) {
			i.applyColorIndex(idx)
			return
		}
	}
	col, err := parseHexColor(arg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid color %q\n", arg)
		return
	}
	idx := appstate.EnsurePaletteColor(col, "")
	i.refreshPalette()
	i.applyColorIndex(idx)
}

func (i *interactiveCmd) handleColorList() {
	i.refreshPalette()
	i.printColorList()
}

func (i *interactiveCmd) handleWidth(args []string) {
	i.refreshWidths()
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		i.printWidthList()
		return
	}
	val, err := strconv.Atoi(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid width %q\n", args[0])
		return
	}
	idx := appstate.EnsureWidth(val)
	i.refreshWidths()
	i.applyWidthIndex(idx)
}

func (i *interactiveCmd) handleWidthList() {
	i.refreshWidths()
	i.printWidthList()
}

func (i *interactiveCmd) refreshPalette() {
	palette := appstate.PaletteColors()
	i.mu.Lock()
	i.palette = palette
	i.colorIdx = clampIndex(i.colorIdx, len(i.palette))
	i.mu.Unlock()
}

func (i *interactiveCmd) refreshWidths() {
	widths := appstate.WidthOptions()
	i.mu.Lock()
	i.widths = widths
	i.widthIdx = clampIndex(i.widthIdx, len(i.widths))
	i.mu.Unlock()
}

func (i *interactiveCmd) printColorList() {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.palette) == 0 {
		fmt.Fprintln(os.Stdout, "no colors available")
		return
	}
	current := clampIndex(i.colorIdx, len(i.palette))
	for idx, entry := range i.palette {
		marker := " "
		if idx == current {
			marker = "*"
		}
		name := entry.Name
		hex := fmt.Sprintf("#%02X%02X%02X", entry.Color.R, entry.Color.G, entry.Color.B)
		if name == "" {
			name = hex
		}
		block := fmt.Sprintf("\x1b[48;2;%d;%d;%dm  \x1b[0m", entry.Color.R, entry.Color.G, entry.Color.B)
		suffix := ""
		if _, ok := i.defaultPaletteSet[entry.Color]; !ok {
			suffix = " (custom)"
		}
		fmt.Fprintf(os.Stdout, "%s %2d: %-12s %s %s%s\n", marker, idx, name, hex, block, suffix)
	}
}

func (i *interactiveCmd) printWidthList() {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.widths) == 0 {
		fmt.Fprintln(os.Stdout, "no widths available")
		return
	}
	current := clampIndex(i.widthIdx, len(i.widths))
	for idx, w := range i.widths {
		marker := " "
		if idx == current {
			marker = "*"
		}
		suffix := ""
		if _, ok := i.defaultWidthSet[w]; !ok {
			suffix = " (custom)"
		}
		fmt.Fprintf(os.Stdout, "%s %3dpx%s\n", marker, w, suffix)
	}
}

func (i *interactiveCmd) applyColorIndex(idx int) {
	i.mu.Lock()
	i.colorIdx = clampIndex(idx, len(i.palette))
	colorIdx := i.colorIdx
	widthIdx := clampIndex(i.widthIdx, len(i.widths))
	state := i.state
	msg := fmt.Sprintf("color set to %s", formatColor(i.palette, colorIdx))
	i.mu.Unlock()
	if state != nil {
		state.ApplySettings(colorIdx, widthIdx)
	}
	fmt.Fprintln(os.Stdout, msg)
	i.printColorList()
}

func (i *interactiveCmd) applyWidthIndex(idx int) {
	i.mu.Lock()
	i.widthIdx = clampIndex(idx, len(i.widths))
	widthIdx := i.widthIdx
	width := widthAt(i.widths, widthIdx)
	colorIdx := i.colorIdx
	state := i.state
	i.mu.Unlock()
	if state != nil {
		state.ApplySettings(colorIdx, widthIdx)
	}
	fmt.Fprintf(os.Stdout, "width set to %dpx\n", width)
	i.printWidthList()
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
	if path == "" {
		fmt.Fprintln(os.Stderr, "path must not be empty")
		return
	}
	if err := i.saveToPath(path); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSaveTmp() {
	path, err := i.saveToTmp()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSavePictures() {
	dir, err := picturesDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	path, err := i.saveAuto(dir, "shineyshot")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSaveHome() {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	path, err := i.saveAuto(home, "shineyshot")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	i.finalizeSave(path)
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
	return i.palette[idx].Color, i.widths[widthIdx]
}

func parseHexColor(s string) (color.RGBA, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "#") {
		s = s[1:]
	}
	if len(s) != 6 {
		return color.RGBA{}, fmt.Errorf("expected 6 hexadecimal digits")
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{R: uint8(v >> 16), G: uint8((v >> 8) & 0xFF), B: uint8(v & 0xFF), A: 255}, nil
}

func (i *interactiveCmd) saveToPath(path string) error {
	return i.withImage(false, func(img *image.RGBA) error {
		dir := filepath.Dir(path)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return png.Encode(f, img)
	})
}

func (i *interactiveCmd) saveToTmp() (string, error) {
	var path string
	err := i.withImage(false, func(img *image.RGBA) error {
		f, err := os.CreateTemp("/tmp", "shineyshot-*.png")
		if err != nil {
			return err
		}
		path = f.Name()
		if err := png.Encode(f, img); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	})
	return path, err
}

func (i *interactiveCmd) saveAuto(dir, prefix string) (string, error) {
	ts := time.Now().Format("20060102-150405")
	base := fmt.Sprintf("%s-%s.png", prefix, ts)
	path := filepath.Join(dir, base)
	counter := 1
	for {
		if _, err := os.Stat(path); err == nil {
			path = filepath.Join(dir, fmt.Sprintf("%s-%s-%02d.png", prefix, ts, counter))
			counter++
			continue
		} else if !os.IsNotExist(err) {
			return "", err
		}
		break
	}
	if err := i.saveToPath(path); err != nil {
		return "", err
	}
	return path, nil
}

func picturesDir() (string, error) {
	if dir := os.Getenv("XDG_PICTURES_DIR"); dir != "" {
		return expandUserPath(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Pictures"), nil
}

func expandUserPath(p string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if p == "~" {
			return home, nil
		}
		if strings.HasPrefix(p, "~/") {
			return filepath.Join(home, p[2:]), nil
		}
	}
	if filepath.IsAbs(p) {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p), nil
}

func (i *interactiveCmd) finalizeSave(path string) {
	i.mu.Lock()
	i.output = path
	i.mu.Unlock()
	fmt.Fprintf(os.Stdout, "saved %s\n", path)
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

func formatColor(palette []appstate.PaletteColor, idx int) string {
	if len(palette) == 0 {
		return ""
	}
	idx = clampIndex(idx, len(palette))
	entry := palette[idx]
	hex := fmt.Sprintf("#%02X%02X%02X", entry.Color.R, entry.Color.G, entry.Color.B)
	if entry.Name == "" {
		return hex
	}
	return fmt.Sprintf("%s (%s)", entry.Name, hex)
}

func widthAt(widths []int, idx int) int {
	if len(widths) == 0 {
		return 0
	}
	idx = clampIndex(idx, len(widths))
	return widths[idx]
}
