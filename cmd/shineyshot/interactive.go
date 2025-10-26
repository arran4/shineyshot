package main

import (
	"bufio"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/example/shineyshot/internal/appstate"
	"github.com/example/shineyshot/internal/capture"
	"github.com/example/shineyshot/internal/clipboard"
)

type interactiveCmd struct {
	r *root

	mu     sync.RWMutex
	img    *image.RGBA
	output string
	state  *appstate.AppState

	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer

	colorIdx int
	widthIdx int

	palette []appstate.PaletteColor
	widths  []int

	defaultPaletteSet map[color.RGBA]struct{}
	defaultWidthSet   map[int]struct{}

	backgroundSession string
	backgroundDir     string

	includeDecorations bool
	includeCursor      bool
}

func (i *interactiveCmd) writeln(w io.Writer, args ...any) {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		log.Printf("write line: %v", err)
	}
}

func (i *interactiveCmd) writef(w io.Writer, format string, args ...any) {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		log.Printf("write formatted: %v", err)
	}
}

func (i *interactiveCmd) withIO(in io.Reader, out, err io.Writer) func() {
	prevIn, prevOut, prevErr := i.stdin, i.stdout, i.stderr
	if in != nil {
		i.stdin = in
	}
	if out != nil {
		i.stdout = out
	}
	if err != nil {
		i.stderr = err
	}
	return func() {
		i.stdin = prevIn
		i.stdout = prevOut
		i.stderr = prevErr
	}
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
		stdin:             os.Stdin,
		stdout:            os.Stdout,
		stderr:            os.Stderr,
	}
}

func (i *interactiveCmd) captureOptions() capture.CaptureOptions {
	return capture.CaptureOptions{
		IncludeDecorations: i.includeDecorations,
		IncludeCursor:      i.includeCursor,
	}
}

func (i *interactiveCmd) Run() error {
	i.writeln(i.stdout, "Interactive mode. Type 'help' for commands.")
	scanner := bufio.NewScanner(i.stdin)
	for {
		i.writef(i.stdout, "> ")
		if !scanner.Scan() {
			break
		}
		done, err := i.executeLine(scanner.Text())
		if err != nil {
			return err
		}
		if done {
			return nil
		}
	}
	return scanner.Err()
}

func (i *interactiveCmd) executeLine(line string) (bool, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return false, nil
	}
	fields := strings.Fields(line)
	cmd := strings.ToLower(fields[0])
	args := fields[1:]

	switch cmd {
	case "quit", "exit":
		return true, nil
	case "help":
		i.printHelp()
	case "capture":
		i.handleCapture(args)
	case "windows":
		i.printWindowList()
	case "screens":
		i.printScreenList()
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
	case "tabs":
		i.handleTabs(args)
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
	case "background":
		i.handleBackground(args)
	default:
		i.writef(i.stderr, "unknown command: %s\n", cmd)
	}
	return false, nil
}

func (i *interactiveCmd) printHelp() {
	i.writeln(i.stdout, "Commands:")
	i.writeln(i.stdout, "  capture screen [DISPLAY]   capture full screen; use 'screens' to list displays")
	i.writeln(i.stdout, "  capture window [SELECTOR]   capture window by selector; defaults to active window; 'windows' lists options")
	i.writeln(i.stdout, "  capture region SCREEN X Y WIDTH HEIGHT   capture region on a screen; 'screens' lists displays")
	i.writeln(i.stdout, "  arrow x0 y0 x1 y1          draw arrow with current stroke")
	i.writeln(i.stdout, "  line x0 y0 x1 y1           draw line with current stroke")
	i.writeln(i.stdout, "  rect x0 y0 x1 y1           draw rectangle with current stroke")
	i.writeln(i.stdout, "  circle x y r               draw circle with current stroke")
	i.writeln(i.stdout, "  crop x0 y0 x1 y1           crop image to rectangle")
	i.writeln(i.stdout, "  color [value|list]         set or list palette colors")
	i.writeln(i.stdout, "  colors                     list palette colors")
	i.writeln(i.stdout, "  width [value|list]         set or list stroke widths")
	i.writeln(i.stdout, "  widths                     list stroke widths")
	i.writeln(i.stdout, "  show                       open synced annotation window")
	i.writeln(i.stdout, "  preview                    open copy in separate window")
	i.writeln(i.stdout, "  tabs [list|switch|next|prev|close]   manage annotation tabs")
	i.writeln(i.stdout, "  save FILE                  save image to FILE")
	i.writeln(i.stdout, "  savetmp                    save to /tmp with a unique filename")
	picturesHelp := "save to your Pictures directory"
	if dir, err := picturesDir(); err == nil {
		picturesHelp = fmt.Sprintf("save to your Pictures directory (%s)", dir)
	}
	i.writeln(i.stdout, fmt.Sprintf("  savepictures               %s", picturesHelp))
	i.writeln(i.stdout, "  savehome                   save to your home directory")
	i.writeln(i.stdout, "  copy                       copy image to clipboard")
	i.writeln(i.stdout, "  windows                    list available windows and selectors")
	i.writeln(i.stdout, "  screens                    list available screens/displays")
	i.writeln(i.stdout, "  copyname                   copy last saved filename")
	i.writeln(i.stdout, "  background start [NAME] [DIR]   launch a background socket session")
	i.writeln(i.stdout, "  background stop [NAME] [DIR]    stop a background socket session")
	i.writeln(i.stdout, "  background list [DIR]           list background sessions")
	i.writeln(i.stdout, "  background clean [DIR]          remove dead background sockets")
	i.writeln(i.stdout,
		"  background run [NAME] COMMAND [ARGS...]   "+
			"run a socket command (e.g., 'background run capture screen')")
	i.writeln(i.stdout, "  quit                       exit interactive mode")
	i.writeln(i.stdout, "")
	i.writeln(i.stdout, "Window selectors:")
	i.writeln(i.stdout, "  index:<n>        window list index (see 'windows')")
	i.writeln(i.stdout, "  id:<hex|dec>     X11 window id")
	i.writeln(i.stdout, "  pid:<pid>        process id that owns the window")
	i.writeln(i.stdout, "  exec:<name>      executable name substring")
	i.writeln(i.stdout, "  class:<name>     X11 WM_CLASS substring")
	i.writeln(i.stdout, "  title:<text>     window title substring (useful for literal words like 'list')")
	i.writeln(i.stdout, "  <text>           fallback substring match on title/executable/class")
}

func (i *interactiveCmd) handleCapture(args []string) {
	if len(args) < 1 {
		i.writeln(i.stderr, "usage: capture [screen|window|region] ...")
		return
	}
	mode := strings.ToLower(args[0])
	params := args[1:]
	var (
		img    *image.RGBA
		err    error
		target string
	)
	opts := i.captureOptions()
	switch mode {
	case "screen":
		if len(params) >= 1 && strings.EqualFold(params[0], "list") {
			i.printScreenList()
			return
		}
		display := ""
		if len(params) >= 1 {
			display = strings.Join(params, " ")
		}
		img, err = capture.CaptureScreenshot(display, opts)
		if err != nil && display == "" {
			img, err = capture.CaptureScreenshot("0", opts)
			if err == nil {
				target = "display 0"
			}
		}
		if err != nil {
			i.writeln(i.stderr, err)
			if len(params) == 0 || display != "" {
				i.printScreenList()
			}
			return
		}
		if target == "" {
			if display != "" {
				target = fmt.Sprintf("display %s", display)
			} else {
				target = "current display"
			}
		}
	case "window":
		if len(params) >= 1 && strings.EqualFold(params[0], "list") {
			i.printWindowList()
			return
		}
		selector := ""
		if len(params) > 0 {
			selector = strings.Join(params, " ")
		}
		var info capture.WindowInfo
		img, info, err = capture.CaptureWindowDetailed(selector, opts)
		if err != nil {
			i.writeln(i.stderr, err)
			i.printWindowList()
			return
		}
		target = formatWindowLabel(info)
	case "region":
		if len(params) >= 1 && strings.EqualFold(params[0], "list") {
			i.printScreenList()
			return
		}
		if len(params) < 5 {
			i.writeln(i.stderr, "usage: capture region SCREEN X Y WIDTH HEIGHT")
			i.printScreenList()
			return
		}
		monitors, mErr := capture.ListMonitors()
		if mErr != nil {
			i.writeln(i.stderr, mErr)
			return
		}
		monitor, mErr := capture.FindMonitor(monitors, params[0])
		if mErr != nil {
			i.writeln(i.stderr, mErr)
			i.printScreenList()
			return
		}
		coords, cErr := parseInts(params[1:], 4)
		if cErr != nil {
			i.writeln(i.stderr, cErr)
			return
		}
		if coords[2] <= 0 || coords[3] <= 0 {
			i.writeln(i.stderr, "width and height must be positive")
			return
		}
		rect := image.Rect(
			monitor.Rect.Min.X+coords[0],
			monitor.Rect.Min.Y+coords[1],
			monitor.Rect.Min.X+coords[0]+coords[2],
			monitor.Rect.Min.Y+coords[1]+coords[3],
		)
		img, err = capture.CaptureRegionRect(rect, opts)
		if err == nil {
			target = fmt.Sprintf("%s @ %dx%d+%d,%d", formatMonitorName(monitor), coords[2], coords[3], coords[0], coords[1])
		}
	default:
		i.writeln(i.stderr, "usage: capture [screen|window|region] ...")
		return
	}
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.setImage(img)
	if i.r != nil {
		detail := mode
		if target != "" {
			detail = fmt.Sprintf("%s %s", mode, target)
		}
		i.r.notifyCapture(strings.TrimSpace(detail), img)
	}
	if target != "" {
		i.writef(i.stdout, "captured %s %s\n", mode, target)
	} else {
		i.writef(i.stdout, "captured %s\n", mode)
	}
}

func (i *interactiveCmd) handleArrow(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawArrow(img, vals[0], vals[1], vals[2], vals[3], col, width)
		return nil
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "arrow drawn")
}

func (i *interactiveCmd) handleLine(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawLine(img, vals[0], vals[1], vals[2], vals[3], col, width)
		return nil
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "line drawn")
}

func (i *interactiveCmd) handleRect(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawRect(img, image.Rect(vals[0], vals[1], vals[2], vals[3]), col, width)
		return nil
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "rectangle drawn")
}

func (i *interactiveCmd) handleCircle(args []string) {
	vals, err := parseInts(args, 3)
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		col, width := i.strokeLocked()
		appstate.DrawCircle(img, vals[0], vals[1], vals[2], col, width)
		return nil
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "circle drawn")
}

func (i *interactiveCmd) handleCrop(args []string) {
	vals, err := parseInts(args, 4)
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if err := i.withImage(true, func(img *image.RGBA) error {
		cropped := appstate.CropImage(img, image.Rect(vals[0], vals[1], vals[2], vals[3]))
		*img = *cropped
		return nil
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "cropped")
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
			i.writef(i.stderr, "color index must be between 0 and %d\n", len(i.palette)-1)
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
		i.writef(i.stderr, "invalid color %q\n", arg)
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
		i.writef(i.stderr, "invalid width %q\n", args[0])
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
		i.writeln(i.stdout, "no colors available")
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
		i.writef(i.stdout, "%s %2d: %-12s %s %s%s\n", marker, idx, name, hex, block, suffix)
	}
}

func (i *interactiveCmd) printWidthList() {
	i.mu.RLock()
	defer i.mu.RUnlock()
	if len(i.widths) == 0 {
		i.writeln(i.stdout, "no widths available")
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
		i.writef(i.stdout, "%s %3dpx%s\n", marker, w, suffix)
	}
}

func (i *interactiveCmd) printScreenList() {
	monitors, err := capture.ListMonitors()
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if len(monitors) == 0 {
		i.writeln(i.stdout, "no screens available")
		return
	}
	i.writeln(i.stdout, "available screens:")
	for _, mon := range monitors {
		primary := ""
		if mon.Primary {
			primary = " [primary]"
		}
		rect := mon.Rect
		i.writef(i.stdout, "  %s -> %dx%d+%d,%d%s\n", formatMonitorName(mon), rect.Dx(), rect.Dy(), rect.Min.X, rect.Min.Y, primary)
	}
}

func formatMonitorName(mon capture.MonitorInfo) string {
	if mon.Name != "" {
		return fmt.Sprintf("#%d %s", mon.Index, mon.Name)
	}
	return fmt.Sprintf("#%d", mon.Index)
}

func (i *interactiveCmd) printWindowList() {
	windows, err := capture.ListWindows()
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	if len(windows) == 0 {
		i.writeln(i.stdout, "no windows available")
		return
	}
	i.writeln(i.stdout, "available windows (* marks the active window):")
	for _, win := range windows {
		marker := " "
		if win.Active {
			marker = "*"
		}
		i.writef(i.stdout, "%s %s\n", marker, formatWindowLabel(win))
	}
	i.writeln(i.stdout, "selectors: index:<n>, id:<hex>, pid:<pid>, exec:<name>, class:<name>, title:<text>, substring match")
}

func formatWindowLabel(info capture.WindowInfo) string {
	title := info.Title
	if title == "" {
		title = "(untitled)"
	}
	meta := make([]string, 0, 2)
	if info.Executable != "" {
		meta = append(meta, fmt.Sprintf("exec:%s", info.Executable))
	}
	if info.Class != "" {
		meta = append(meta, fmt.Sprintf("class:%s", info.Class))
	}
	extra := ""
	if len(meta) > 0 {
		extra = " (" + strings.Join(meta, ", ") + ")"
	}
	return fmt.Sprintf("#%d %q%s id:0x%X", info.Index, title, extra, info.ID)
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
	i.writeln(i.stdout, msg)
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
	i.writef(i.stdout, "width set to %dpx\n", width)
	i.printWidthList()
}

func (i *interactiveCmd) handleShow(copyImage bool) {
	i.mu.Lock()
	if i.img == nil {
		i.mu.Unlock()
		i.writeln(i.stderr, "no image loaded")
		return
	}
	if copyImage {
		dup := image.NewRGBA(i.img.Bounds())
		draw.Draw(dup, dup.Bounds(), i.img, image.Point{}, draw.Src)
		output := i.output
		colorIdx := i.colorIdx
		widthIdx := i.widthIdx
		detail := ""
		if output != "" {
			detail = filepath.Base(output)
		}
		background := i.backgroundSession
		i.mu.Unlock()
		st := appstate.New(
			appstate.WithImage(dup),
			appstate.WithOutput(output),
			appstate.WithColorIndex(colorIdx),
			appstate.WithWidthIndex(widthIdx),
			appstate.WithMode(appstate.ModePreview),
			appstate.WithTitle(windowTitle(titleOptions{
				Mode:       "Preview",
				Detail:     detail,
				Tab:        "Tab 1",
				LastSaved:  detail,
				Background: background,
			})),
			appstate.WithVersion(version),
		)
		go st.Run()
		i.writeln(i.stdout, "preview window opened")
		return
	}
	if i.state != nil {
		i.mu.Unlock()
		i.writeln(i.stderr, "annotation window already open")
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
	detail := ""
	if output != "" {
		detail = filepath.Base(output)
	}
	st = appstate.New(
		appstate.WithImage(img),
		appstate.WithOutput(output),
		appstate.WithColorIndex(colorIdx),
		appstate.WithWidthIndex(widthIdx),
		appstate.WithTitle(windowTitle(titleOptions{
			Mode:       "Annotate",
			Detail:     detail,
			Tab:        "Tab 1",
			LastSaved:  detail,
			Background: i.backgroundSession,
		})),
		appstate.WithVersion(version),
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
	st.SetTabListener(i.onTabChange)
	go st.Run()
	i.writeln(i.stdout, "annotation window opened")
}

func (i *interactiveCmd) onTabChange(change appstate.TabChange) {
	if change.Image == nil {
		return
	}
	i.mu.Lock()
	i.img = change.Image
	i.widthIdx = clampIndex(change.WidthIdx, len(i.widths))
	i.mu.Unlock()
}

func (i *interactiveCmd) handleTabs(args []string) {
	i.mu.RLock()
	st := i.state
	i.mu.RUnlock()
	if st == nil {
		i.writeln(i.stderr, "annotation window not open; run 'show' first")
		return
	}
	snapshot := st.TabsState()
	if len(snapshot.Tabs) == 0 {
		i.writeln(i.stderr, "no tabs available")
		return
	}
	if len(args) == 0 || strings.EqualFold(args[0], "list") {
		i.printTabList(snapshot)
		return
	}
	action := strings.ToLower(args[0])
	switch action {
	case "switch":
		if len(args) < 2 {
			i.writeln(i.stderr, "usage: tabs switch INDEX")
			return
		}
		idx, err := parseTabNumber(args[1])
		if err != nil {
			i.writeln(i.stderr, err.Error())
			return
		}
		if idx < 0 || idx >= len(snapshot.Tabs) {
			i.writef(i.stderr, "tab %d does not exist\n", idx+1)
			return
		}
		title := tabDisplayTitle(snapshot.Tabs[idx])
		if err := st.ActivateTab(idx); err != nil {
			i.writeln(i.stderr, err.Error())
			return
		}
		i.writef(i.stdout, "switched to tab %d (%s)\n", idx+1, title)
	case "next":
		idx := snapshot.Current
		if idx < 0 {
			idx = 0
		}
		idx++
		if idx >= len(snapshot.Tabs) {
			idx = 0
		}
		title := tabDisplayTitle(snapshot.Tabs[idx])
		if err := st.ActivateTab(idx); err != nil {
			i.writeln(i.stderr, err.Error())
			return
		}
		i.writef(i.stdout, "switched to tab %d (%s)\n", idx+1, title)
	case "prev":
		idx := snapshot.Current
		if idx < 0 {
			idx = 0
		}
		idx--
		if idx < 0 {
			idx = len(snapshot.Tabs) - 1
		}
		title := tabDisplayTitle(snapshot.Tabs[idx])
		if err := st.ActivateTab(idx); err != nil {
			i.writeln(i.stderr, err.Error())
			return
		}
		i.writef(i.stdout, "switched to tab %d (%s)\n", idx+1, title)
	case "close":
		idx := snapshot.Current
		if len(args) > 1 {
			parsed, err := parseTabNumber(args[1])
			if err != nil {
				i.writeln(i.stderr, err.Error())
				return
			}
			idx = parsed
		}
		if idx < 0 || idx >= len(snapshot.Tabs) {
			i.writef(i.stderr, "tab %d does not exist\n", idx+1)
			return
		}
		title := tabDisplayTitle(snapshot.Tabs[idx])
		if err := st.CloseTab(idx); err != nil {
			i.writeln(i.stderr, err.Error())
			return
		}
		i.writef(i.stdout, "closed tab %d (%s)\n", idx+1, title)
	default:
		i.writeln(i.stderr, "usage: tabs [list|switch INDEX|next|prev|close [INDEX]]")
	}
}

func (i *interactiveCmd) printTabList(state appstate.TabsState) {
	i.writeln(i.stdout, "tabs:")
	for _, tb := range state.Tabs {
		marker := " "
		if tb.Index == state.Current {
			marker = "*"
		}
		i.writef(i.stdout, "%s %d: %s\n", marker, tb.Index+1, tabDisplayTitle(tb))
	}
}

func (i *interactiveCmd) handleSave(args []string) {
	if len(args) != 1 {
		i.writeln(i.stderr, "usage: save FILE")
		return
	}
	path := args[0]
	if path == "" {
		i.writeln(i.stderr, "path must not be empty")
		return
	}
	if err := i.saveToPath(path); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSaveTmp() {
	path, err := i.saveToTmp()
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSavePictures() {
	dir, err := picturesDir()
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	path, err := i.saveAuto(dir, "shineyshot")
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleSaveHome() {
	home, err := os.UserHomeDir()
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	path, err := i.saveAuto(home, "shineyshot")
	if err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.finalizeSave(path)
}

func (i *interactiveCmd) handleCopy() {
	if err := i.withImage(false, func(img *image.RGBA) error {
		return clipboard.WriteImage(img)
	}); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "image copied to clipboard")
	if i.r != nil {
		i.r.notifyCopy("image")
	}
}

func (i *interactiveCmd) handleCopyName() {
	i.mu.RLock()
	output := i.output
	i.mu.RUnlock()
	if output == "" {
		i.writeln(i.stderr, "no saved file")
		return
	}
	if err := clipboard.WriteText(output); err != nil {
		i.writeln(i.stderr, err)
		return
	}
	i.writeln(i.stdout, "filename copied to clipboard")
	if i.r != nil {
		i.r.notifyCopy(output)
	}
}

func (i *interactiveCmd) handleBackground(args []string) {
	if len(args) == 0 {
		i.writeln(i.stderr, "usage: background [start|stop|list|clean|run] ...")
		return
	}
	action := strings.ToLower(args[0])
	switch action {
	case "start":
		name := ""
		dirArg := ""
		if len(args) > 1 {
			name = args[1]
		}
		if len(args) > 2 {
			dirArg = strings.Join(args[2:], " ")
		}
		if i.backgroundSession != "" {
			i.writef(i.stderr, "background session %s is already tracked; stop it before starting another\n", i.backgroundSession)
			return
		}
		sessionName, dir, err := i.startBackgroundSession(name, dirArg)
		if err != nil {
			i.writeln(i.stderr, err)
			return
		}
		i.backgroundSession = sessionName
		i.backgroundDir = dir
		i.writef(i.stdout, "background session %s ready at %s\n", sessionName, socketPath(dir, sessionName))
	case "stop":
		name := ""
		dirArg := ""
		if len(args) > 1 {
			name = args[1]
		}
		if len(args) > 2 {
			dirArg = strings.Join(args[2:], " ")
		}
		if name == "" {
			name = i.backgroundSession
		}
		if dirArg == "" {
			dirArg = i.backgroundDir
		}
		if name == "" {
			i.writeln(i.stderr, "usage: background stop [NAME] [DIR]")
			return
		}
		if err := i.stopBackgroundSession(name, dirArg); err != nil {
			i.writeln(i.stderr, err)
			return
		}
		if name == i.backgroundSession && (dirArg == "" || dirArg == i.backgroundDir) {
			i.backgroundSession = ""
			i.backgroundDir = ""
		}
		i.writef(i.stdout, "background session %s stop requested\n", name)
	case "list":
		dirArg := ""
		if len(args) > 1 {
			dirArg = strings.Join(args[1:], " ")
		}
		if err := i.listBackgroundSessions(dirArg); err != nil {
			i.writeln(i.stderr, err)
		}
	case "clean":
		dirArg := ""
		if len(args) > 1 {
			dirArg = strings.Join(args[1:], " ")
		}
		dir, err := resolveSocketDir(dirArg)
		if err != nil {
			i.writeln(i.stderr, err)
			return
		}
		if err := cleanSocketDir(dir, i.stdout); err != nil {
			i.writeln(i.stderr, err)
			return
		}
		if i.backgroundSession != "" {
			statuses, statErr := collectSocketStatuses(dir)
			if statErr == nil {
				tracked := false
				for _, st := range statuses {
					if st.name == i.backgroundSession && st.err == nil {
						tracked = true
						break
					}
				}
				if !tracked {
					i.backgroundSession = ""
					i.backgroundDir = ""
				}
			}
		}
	case "run":
		if len(args) < 2 && i.backgroundSession == "" {
			i.writeln(i.stderr, "usage: background run [NAME] COMMAND [ARGS...]")
			return
		}
		dirArg := i.backgroundDir
		tokens := args[1:]
		if len(tokens) > 0 && strings.HasPrefix(tokens[0], "dir=") {
			dirArg = strings.TrimPrefix(tokens[0], "dir=")
			tokens = tokens[1:]
		}
		dir, err := resolveSocketDir(dirArg)
		if err != nil {
			i.writeln(i.stderr, err)
			return
		}
		if len(tokens) == 0 {
			i.writeln(i.stderr, "usage: background run [NAME] COMMAND [ARGS...]")
			return
		}
		resolvedName, commandArgs, err := resolveRunTarget(dir, i.backgroundSession, tokens)
		if err != nil {
			i.writeln(i.stderr, err)
			return
		}
		command := strings.Join(commandArgs, " ")
		if err := runSocketCommands(dir, resolvedName, []string{command}, i.stdout, i.stderr); err != nil {
			i.writeln(i.stderr, err)
			return
		}
		i.backgroundSession = resolvedName
		i.backgroundDir = dir
	default:
		i.writef(i.stderr, "unknown background action: %s\n", action)
	}
}

func (i *interactiveCmd) startBackgroundSession(name, dirArg string) (string, string, error) {
	dir, err := resolveSocketDir(dirArg)
	if err != nil {
		return "", "", err
	}
	session, err := startBackgroundServer(dir, name, i.r)
	if err != nil {
		return "", "", err
	}
	return session, dir, nil
}

func (i *interactiveCmd) stopBackgroundSession(name, dirArg string) error {
	dir, err := resolveSocketDir(dirArg)
	if err != nil {
		return err
	}
	return stopSocket(dir, name)
}

func (i *interactiveCmd) listBackgroundSessions(dirArg string) error {
	dir, err := resolveSocketDir(dirArg)
	if err != nil {
		return err
	}
	return printSocketList(dir, i.stdout)
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
	s = strings.TrimPrefix(s, "#")
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
		if err := png.Encode(f, img); err != nil {
			if cerr := f.Close(); cerr != nil {
				return fmt.Errorf("encode image: %w (close error: %v)", err, cerr)
			}
			return err
		}
		return f.Close()
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
			if cerr := f.Close(); cerr != nil {
				return fmt.Errorf("encode image: %w (close error: %v)", err, cerr)
			}
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
		if trimmed := strings.TrimPrefix(p, "~/"); trimmed != p {
			return filepath.Join(home, trimmed), nil
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
	display := path
	if abs, err := filepath.Abs(path); err == nil {
		display = abs
	}
	i.mu.Lock()
	i.output = display
	i.mu.Unlock()
	i.writef(i.stdout, "saved %s\n", display)
	if i.r != nil {
		i.r.notifySave(display)
	}
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

func tabDisplayTitle(summary appstate.TabSummary) string {
	if strings.TrimSpace(summary.Title) != "" {
		return summary.Title
	}
	return fmt.Sprintf("%d", summary.Index+1)
}

func parseTabNumber(arg string) (int, error) {
	idx, err := strconv.Atoi(arg)
	if err != nil {
		return 0, fmt.Errorf("invalid tab number %q", arg)
	}
	if idx <= 0 {
		return 0, fmt.Errorf("tab numbers start at 1")
	}
	return idx - 1, nil
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
