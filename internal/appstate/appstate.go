package appstate

import (
	"context"
	"fmt"
	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"image"
	"image/color"
	"image/draw"
	"log"
	"math"
	"sort"
	"sync"
	"time"

	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
)

const (
	tabHeight    = 24
	bottomHeight = 24
)

var toolbarWidth = 48

// frameDropThreshold specifies how many consecutive frames can be canceled
// before a draw is allowed to complete to keep the UI responsive.
const frameDropThreshold = 10

type Tool int

const (
	ToolMove Tool = iota
	ToolCrop
	ToolDraw
	ToolCircle
	ToolLine
	ToolArrow
	ToolRect
	ToolNumber
	ToolText
)

const (
	defaultColorIndex = 2
	defaultWidthIndex = 2
)

type Tab struct {
	Image *image.RGBA
	Title string
	// Offset is stored in image coordinates so it is independent of zoom.
	Offset     image.Point
	Zoom       float64
	NextNumber int
	WidthIdx   int
}

const handleSize = 8

type cropAction int

const (
	cropNone cropAction = iota
	cropMove
	cropResizeTL
	cropResizeT
	cropResizeTR
	cropResizeR
	cropResizeBR
	cropResizeB
	cropResizeBL
	cropResizeL
)

type actionType int

const (
	actionNone actionType = iota
	actionMove
	actionCrop
	actionDraw
)

type PaletteColor struct {
	Name  string
	Color color.RGBA
}

var (
	paletteMu sync.RWMutex
	palette   = []color.RGBA{
		{0, 0, 0, 255},       // black
		{255, 255, 255, 255}, // white
		{255, 0, 0, 255},
		{0, 255, 0, 255},
		{0, 0, 255, 255},
		{255, 255, 0, 255},
		{0, 255, 255, 255},
		{255, 0, 255, 255},
		{128, 0, 0, 255},
		{0, 128, 0, 255},
		{0, 0, 128, 255},
		{128, 128, 0, 255},
		{0, 128, 128, 255},
		{128, 0, 128, 255},
		{192, 192, 192, 255},
		{128, 128, 128, 255},
	}
	paletteNames = []string{
		"Black",
		"White",
		"Red",
		"Lime",
		"Blue",
		"Yellow",
		"Cyan",
		"Magenta",
		"Maroon",
		"Green",
		"Navy",
		"Olive",
		"Teal",
		"Purple",
		"Silver",
		"Gray",
	}
)

var checkerLight = color.RGBA{220, 220, 220, 255}
var checkerDark = color.RGBA{192, 192, 192, 255}

var textSizes = []float64{12, 16, 20, 24, 32}
var textFaces []font.Face
var textSizeIdx int
var messageFace font.Face

func init() {
	f, err := opentype.Parse(goregular.TTF)
	if err != nil {
		log.Fatalf("parse font: %v", err)
	}
	for _, sz := range textSizes {
		face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: sz, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			log.Fatalf("font face: %v", err)
		}
		textFaces = append(textFaces, face)
	}
	messageFace, err = opentype.NewFace(f, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		log.Fatalf("font face: %v", err)
	}
}

func fitZoom(img *image.RGBA, winW, winH int) float64 {
	availW := winW - toolbarWidth
	availH := winH - tabHeight - bottomHeight
	zx := float64(availW) / float64(img.Bounds().Dx())
	zy := float64(availH) / float64(img.Bounds().Dy())
	if zx < zy {
		return zx
	}
	return zy
}

// imageRect returns the destination rectangle for drawing the image. It anchors
// the canvas origin just below the toolbar instead of centering it so that the
// image position remains stable even when the canvas grows or shrinks.
func imageRect(img *image.RGBA, winW, winH int, zoom float64) image.Rectangle {
	w := int(float64(img.Bounds().Dx()) * zoom)
	h := int(float64(img.Bounds().Dy()) * zoom)
	x0 := toolbarWidth
	y0 := tabHeight
	return image.Rect(x0, y0, x0+w, y0+h)
}

// drawCheckerboard fills rect of dst with a checkerboard pattern of the given
// colors. size controls the checker square size.
func drawCheckerboard(dst *image.RGBA, rect image.Rectangle, size int, light, dark color.Color) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			if ((x/size)+(y/size))%2 == 0 {
				dst.Set(x, y, light)
			} else {
				dst.Set(x, y, dark)
			}
		}
	}
}

// drawBackdrop fills dst with a cached checkerboard pattern.
func drawBackdrop(dst *image.RGBA) {
	b := dst.Bounds()
	if backdropCache == nil || backdropCache.Bounds() != b {
		backdropCache = image.NewRGBA(b)
		drawCheckerboard(backdropCache, backdropCache.Bounds(), 8, checkerLight, checkerDark)
	}
	draw.Draw(dst, b, backdropCache, image.Point{}, draw.Src)
}

var (
	widthsMu sync.RWMutex
	widths   = []int{1, 2, 4, 6, 8}
)
var numberSizes = []int{8, 12, 16, 20, 24}

// DefaultColorIndex returns the default palette index used for drawing tools.
func DefaultColorIndex() int { return defaultColorIndex }

// DefaultWidthIndex returns the default stroke width index used for drawing tools.
func DefaultWidthIndex() int { return defaultWidthIndex }

// Palette returns a copy of the available drawing colors.
func Palette() []color.RGBA {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	out := make([]color.RGBA, len(palette))
	copy(out, palette)
	return out
}

// PaletteColors returns palette entries annotated with their display names.
func PaletteColors() []PaletteColor {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	out := make([]PaletteColor, len(palette))
	for i := range palette {
		out[i] = PaletteColor{Name: paletteNames[i], Color: palette[i]}
	}
	return out
}

// EnsurePaletteColor makes sure col is present in the palette and returns its index.
func EnsurePaletteColor(col color.RGBA, name string) int {
	paletteMu.Lock()
	defer paletteMu.Unlock()
	for idx, existing := range palette {
		if existing == col {
			if name != "" && paletteNames[idx] == "" {
				paletteNames[idx] = name
			}
			return idx
		}
	}
	if name == "" {
		name = fmt.Sprintf("#%02X%02X%02X", col.R, col.G, col.B)
	}
	palette = append(palette, col)
	paletteNames = append(paletteNames, name)
	return len(palette) - 1
}

// WidthOptions returns a copy of the available stroke widths.
func WidthOptions() []int {
	widthsMu.RLock()
	defer widthsMu.RUnlock()
	out := make([]int, len(widths))
	copy(out, widths)
	return out
}

// EnsureWidth makes sure width is included in the options and returns its index.
func EnsureWidth(width int) int {
	if width < 1 {
		width = 1
	}
	widthsMu.Lock()
	defer widthsMu.Unlock()
	for idx, existing := range widths {
		if existing == width {
			return idx
		}
	}
	widths = append(widths, width)
	sort.Ints(widths)
	for idx, existing := range widths {
		if existing == width {
			return idx
		}
	}
	return 0
}

func paletteLen() int {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	return len(palette)
}

func paletteColorAt(idx int) color.RGBA {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	if len(palette) == 0 {
		return color.RGBA{}
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(palette) {
		idx = len(palette) - 1
	}
	return palette[idx]
}

func paletteNameAt(idx int) string {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	if len(paletteNames) == 0 {
		return ""
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(paletteNames) {
		idx = len(paletteNames) - 1
	}
	return paletteNames[idx]
}

func clampColorIndex(idx int) int {
	paletteMu.RLock()
	defer paletteMu.RUnlock()
	if len(palette) == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= len(palette) {
		return len(palette) - 1
	}
	return idx
}

func widthsLen() int {
	widthsMu.RLock()
	defer widthsMu.RUnlock()
	return len(widths)
}

func widthAt(idx int) int {
	widthsMu.RLock()
	defer widthsMu.RUnlock()
	if len(widths) == 0 {
		return 0
	}
	if idx < 0 {
		idx = 0
	}
	if idx >= len(widths) {
		idx = len(widths) - 1
	}
	return widths[idx]
}

func clampWidthIndex(idx int) int {
	widthsMu.RLock()
	defer widthsMu.RUnlock()
	if len(widths) == 0 {
		return 0
	}
	if idx < 0 {
		return 0
	}
	if idx >= len(widths) {
		return len(widths) - 1
	}
	return idx
}

// KeyShortcut describes a keyboard combination that triggers an action.
type KeyShortcut struct {
	Rune      rune
	Code      key.Code
	Modifiers key.Modifiers
}

// KeyboardShortcuts returns the shortcuts associated with an action.
type KeyboardShortcuts interface {
	KeyboardShortcuts() []KeyShortcut
}

// shortcutList is a helper to easily satisfy the KeyboardShortcuts interface.
type shortcutList []KeyShortcut

func (s shortcutList) KeyboardShortcuts() []KeyShortcut { return []KeyShortcut(s) }

// ButtonState describes the visual state of a button.
type ButtonState int

const (
	StateDefault ButtonState = iota
	StateHover
	StatePressed
)

// Button represents an interactive UI element.
// Activate performs the button's action when clicked.
type Button interface {
	Draw(dst *image.RGBA, state ButtonState)
	Rect() image.Rectangle
	SetRect(r image.Rectangle)
	Activate()
}

// CacheButton wraps another Button and caches its rendered states.
// It delegates all interface methods to the wrapped Button while
// caching the result of Draw for each state.
type CacheButton struct {
	Button
	cache [3]*image.RGBA
}

var _ Button = (*CacheButton)(nil)

func (cb *CacheButton) Draw(dst *image.RGBA, state ButtonState) {
	if cb.cache[state] == nil {
		rect := cb.Button.Rect()
		img := image.NewRGBA(rect)
		cb.Button.Draw(img, state)
		cb.cache[state] = img
	}
	draw.Draw(dst, cb.Button.Rect(), cb.cache[state], cb.Button.Rect().Min, draw.Src)
}

func (cb *CacheButton) Rect() image.Rectangle { return cb.Button.Rect() }

func (cb *CacheButton) SetRect(r image.Rectangle) {
	if r != cb.Button.Rect() {
		cb.Button.SetRect(r)
		cb.cache = [3]*image.RGBA{}
	}
}

func (cb *CacheButton) Activate() { cb.Button.Activate() }

type Shortcut struct {
	label  string
	action func()
	rect   image.Rectangle
}

func (s *Shortcut) Draw(dst *image.RGBA, state ButtonState) {
	col := color.RGBA{200, 200, 200, 255}
	switch state {
	case StateHover:
		col = color.RGBA{180, 180, 180, 255}
	case StatePressed:
		col = color.RGBA{150, 150, 150, 255}
	}
	draw.Draw(dst, s.rect, &image.Uniform{col}, image.Point{}, draw.Src)
	drawRect(dst, s.rect, color.Black, 1)
	d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13,
		Dot: fixed.P(s.rect.Min.X+2, s.rect.Min.Y+14)}
	d.DrawString(s.label)
}

func (s *Shortcut) Rect() image.Rectangle { return s.rect }

func (s *Shortcut) SetRect(r image.Rectangle) {
	if r != s.rect {
		s.rect = r
	}
}
func (s *Shortcut) Activate() {
	if s.action != nil {
		s.action()
	}
}

// ToolButton represents a toolbar button that selects a drawing tool.
type ToolButton struct {
	label string
	tool  Tool
	atype actionType
	rect  image.Rectangle
	// onSelect is called when the button is activated.
	onSelect func()
}

func (tb *ToolButton) Draw(dst *image.RGBA, state ButtonState) {
	c := color.RGBA{200, 200, 200, 255}
	switch state {
	case StateHover:
		c = color.RGBA{180, 180, 180, 255}
	case StatePressed:
		c = color.RGBA{150, 150, 150, 255}
	}
	draw.Draw(dst, tb.rect, &image.Uniform{c}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13,
		Dot: fixed.P(tb.rect.Min.X+4, tb.rect.Min.Y+16)}
	d.DrawString(tb.label)
}

func (tb *ToolButton) Rect() image.Rectangle { return tb.rect }

func (tb *ToolButton) SetRect(r image.Rectangle) {
	if r != tb.rect {
		tb.rect = r
	}
}
func (tb *ToolButton) Activate() {
	if tb.onSelect != nil {
		tb.onSelect()
	}
}

func actionOfTool(t Tool) actionType {
	for _, cb := range toolButtons {
		tb := cb.Button.(*ToolButton)
		if tb.tool == t {
			return tb.atype
		}
	}
	switch t {
	case ToolMove:
		return actionMove
	case ToolCrop:
		return actionCrop
	case ToolDraw, ToolCircle, ToolLine, ToolArrow, ToolRect, ToolNumber:
		return actionDraw
	default:
		return actionNone
	}
}

var shortcutRects []Shortcut
var hoverShortcut = -1

var tabButtons []TabButton
var toolButtons []*CacheButton
var paletteRects []image.Rectangle
var widthRects []image.Rectangle
var numberRects []image.Rectangle

// backdropCache holds a cached checkerboard backdrop.
var backdropCache *image.RGBA

// keyboardAction maps a keyboard shortcut to the action name.
var keyboardAction = map[KeyShortcut]string{}
var textSizeRects []image.Rectangle
var hoverTab = -1
var hoverTool = -1
var hoverPalette = -1
var hoverWidth = -1
var hoverNumber = -1
var hoverTextSize = -1

// TabButton draws a tab title in the header bar.
type TabButton struct {
	label    string
	rect     image.Rectangle
	onSelect func()
}

func (tb *TabButton) Draw(dst *image.RGBA, state ButtonState) {
	c := color.RGBA{200, 200, 200, 255}
	switch state {
	case StateHover:
		c = color.RGBA{180, 180, 180, 255}
	case StatePressed:
		c = color.RGBA{150, 150, 150, 255}
	}
	draw.Draw(dst, tb.rect, &image.Uniform{c}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13,
		Dot: fixed.P(tb.rect.Min.X+4, tb.rect.Min.Y+16)}
	d.DrawString(tb.label)
}

func (tb *TabButton) Rect() image.Rectangle { return tb.rect }

func (tb *TabButton) SetRect(r image.Rectangle) {
	if r != tb.rect {
		tb.rect = r
	}
}

func (tb *TabButton) Activate() {
	if tb.onSelect != nil {
		tb.onSelect()
	}
}

func numberBoxHeight(size int) int {
	h := 2*size + 4
	if h < 16 {
		return 16
	}
	return h
}

func drawTabs(dst *image.RGBA, tabs []Tab, current int) {
	// background for title area
	draw.Draw(dst, image.Rect(0, 0, toolbarWidth, tabHeight),
		&image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)

	// program title in the top-left corner
	title := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13,
		Dot: fixed.P(4, 16)}
	title.DrawString("ShineyShot")

	tabButtons = tabButtons[:0]
	x := toolbarWidth
	for i, t := range tabs {
		tb := TabButton{label: t.Title, onSelect: nil}
		tb.SetRect(image.Rect(x, 0, x+80, tabHeight))
		state := StateDefault
		if i == current {
			state = StatePressed
		} else if i == hoverTab {
			state = StateHover
		}
		tb.Draw(dst, state)
		tabButtons = append(tabButtons, tb)
		x += 80
	}
	// fill remainder of bar
	draw.Draw(dst, image.Rect(x, 0, dst.Bounds().Dx(), tabHeight),
		&image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
}

func drawShortcuts(dst *image.RGBA, width, height int, tool Tool, textMode bool, z float64, trigger func(string)) {
	rect := image.Rect(0, height-bottomHeight, width, height)
	draw.Draw(dst, rect, &image.Uniform{color.RGBA{220, 220, 220, 255}}, image.Point{}, draw.Src)
	shortcutRects = shortcutRects[:0]
	zoomStr := fmt.Sprintf("+/-:zoom (%.0f%%)", z*100)
	var shortcuts []Shortcut
	if textMode {
		shortcuts = []Shortcut{
			{label: "Enter:place", action: func() { trigger("textdone") }},
			{label: "Esc:cancel", action: func() { trigger("textcancel") }},
		}
	} else {
		shortcuts = []Shortcut{
			{label: "^N:capture", action: func() { trigger("capture") }},
			{label: "^U:dup", action: func() { trigger("dup") }},
			{label: "^V:paste", action: func() { trigger("paste") }},
			{label: zoomStr, action: func() { trigger("zoom") }},
			{label: "^D:delete", action: func() { trigger("delete") }},
			{label: "^C:copy image", action: func() { trigger("copy") }},
			{label: "^S:save", action: func() { trigger("save") }},
			{label: "Q:quit", action: func() { trigger("quit") }},
		}
		if tool == ToolCrop {
			shortcuts = append(shortcuts,
				Shortcut{label: "Enter:crop", action: func() { trigger("crop") }},
				Shortcut{label: "Ctrl+Enter:new tab", action: func() { trigger("croptab") }},
				Shortcut{label: "Esc:cancel", action: func() { trigger("cropcancel") }},
			)
		}
	}
	x := toolbarWidth + 4
	y := height - bottomHeight + 16
	meas := &font.Drawer{Face: basicfont.Face7x13}
	for i := range shortcuts {
		sc := &shortcuts[i]
		w := meas.MeasureString(sc.label).Ceil()
		sc.SetRect(image.Rect(x-2, y-14, x+w+2, y+4))
		state := StateDefault
		if i == hoverShortcut {
			state = StateHover
		}
		sc.Draw(dst, state)
		shortcutRects = append(shortcutRects, *sc)
		x = sc.rect.Max.X + 8
	}
}

func drawToolbar(dst *image.RGBA, tool Tool, colIdx, widthIdx, numberIdx int) {
	y := tabHeight
	for i, cb := range toolButtons {
		r := image.Rect(0, y, toolbarWidth, y+24)
		cb.SetRect(r)
		tb := cb.Button.(*ToolButton)
		state := StateDefault
		if tb.tool == tool {
			state = StatePressed
		} else if i == hoverTool {
			state = StateHover
		}
		cb.Draw(dst, state)
		y += 24
	}

	// color palette below tools
	y += 4
	x := 4
	paletteRects = paletteRects[:0]
	for i, p := range palette {
		rect := image.Rect(x, y, x+16, y+16)
		draw.Draw(dst, rect, &image.Uniform{p}, image.Point{}, draw.Src)
		if i == hoverPalette {
			draw.Draw(dst, rect, &image.Uniform{color.RGBA{255, 255, 255, 80}}, image.Point{}, draw.Over)
		}
		if i == colIdx {
			draw.Draw(dst, rect, &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Over)
			drawLine(dst, rect.Min.X, rect.Min.Y, rect.Max.X-1, rect.Min.Y, color.White, 1)
			drawLine(dst, rect.Min.X, rect.Min.Y, rect.Min.X, rect.Max.Y-1, color.White, 1)
			drawLine(dst, rect.Max.X-1, rect.Min.Y, rect.Max.X-1, rect.Max.Y-1, color.White, 1)
			drawLine(dst, rect.Min.X, rect.Max.Y-1, rect.Max.X-1, rect.Max.Y-1, color.White, 1)
		}
		paletteRects = append(paletteRects, rect)
		x += 18
		if x+16 > toolbarWidth {
			x = 4
			y += 18
		}
	}

	if tool == ToolDraw || tool == ToolCircle || tool == ToolLine || tool == ToolArrow || tool == ToolRect {
		y += 4
		col := palette[colIdx]
		widthRects = widthRects[:0]
		for i, w := range widths {
			rect := image.Rect(0, y, toolbarWidth, y+16)
			c := color.RGBA{200, 200, 200, 255}
			if i == widthIdx {
				c = color.RGBA{150, 150, 150, 255}
			} else if i == hoverWidth {
				c = color.RGBA{180, 180, 180, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
			d.DrawString(fmt.Sprintf("%d", w))
			lineY := y + 8
			drawLine(dst, 30, lineY, toolbarWidth-4, lineY, col, w)
			widthRects = append(widthRects, rect)
			y += 16
		}
	}
	if tool == ToolNumber {
		y += 4
		col := palette[colIdx]
		numberRects = numberRects[:0]
		for i, s := range numberSizes {
			h := numberBoxHeight(s)
			rect := image.Rect(0, y, toolbarWidth, y+h)
			c := color.RGBA{200, 200, 200, 255}
			if i == numberIdx {
				c = color.RGBA{150, 150, 150, 255}
			} else if i == hoverNumber {
				c = color.RGBA{180, 180, 180, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.Black, Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
			d.DrawString(fmt.Sprintf("%d", s))
			drawFilledCircle(dst, (toolbarWidth+30)/2, y+h/2, s, col)
			numberRects = append(numberRects, rect)
			y += h
		}
	}
	if tool == ToolText {
		y += 4
		col := palette[colIdx]
		textSizeRects = textSizeRects[:0]
		for i, face := range textFaces {
			rect := image.Rect(0, y, toolbarWidth, y+24)
			c := color.RGBA{200, 200, 200, 255}
			if i == textSizeIdx {
				c = color.RGBA{150, 150, 150, 255}
			} else if i == hoverTextSize {
				c = color.RGBA{180, 180, 180, 255}
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.NewUniform(col), Face: face}
			baseline := y + face.Metrics().Ascent.Ceil()
			d.Dot = fixed.P(4, baseline)
			d.DrawString("Ab3")
			textSizeRects = append(textSizeRects, rect)
			y += 24
		}
	}
}

func setThickPixel(img *image.RGBA, x, y, thick int, col color.Color) {
	r := thick / 2
	for dx := -r; dx <= r; dx++ {
		for dy := -r; dy <= r; dy++ {
			px := x + dx
			py := y + dy
			if image.Pt(px, py).In(img.Bounds()) {
				img.Set(px, py, col)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	dx := math.Abs(float64(x1 - x0))
	dy := math.Abs(float64(y1 - y0))
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx - dy
	for {
		setThickPixel(img, x0, y0, thick, col)
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func drawCircleThin(img *image.RGBA, cx, cy, r int, col color.Color) {
	x := r
	y := 0
	err := 1 - r
	for x >= y {
		pts := [][2]int{{x, y}, {y, x}, {-y, x}, {-x, y}, {-x, -y}, {-y, -x}, {y, -x}, {x, -y}}
		for _, p := range pts {
			px := cx + p[0]
			py := cy + p[1]
			if image.Pt(px, py).In(img.Bounds()) {
				img.Set(px, py, col)
			}
		}
		y++
		if err < 0 {
			err += 2*y + 1
		} else {
			x--
			err += 2 * (y - x + 1)
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, r int, col color.Color, thick int) {
	if thick <= 0 {
		drawCircleThin(img, cx, cy, r, col)
		return
	}
	start := -thick / 2
	for i := 0; i < thick; i++ {
		rr := r + start + i
		if rr >= 0 {
			drawCircleThin(img, cx, cy, rr, col)
		}
	}
}

func drawEllipse(img *image.RGBA, cx, cy, rx, ry int, col color.Color, thick int) {
	steps := int(math.Ceil(2 * math.Pi * math.Sqrt(float64(rx*rx+ry*ry))))
	if steps < 8 {
		steps = 8
	}
	var prevX, prevY int
	for i := 0; i <= steps; i++ {
		angle := 2 * math.Pi * float64(i) / float64(steps)
		x := cx + int(math.Cos(angle)*float64(rx))
		y := cy + int(math.Sin(angle)*float64(ry))
		if i > 0 {
			drawLine(img, prevX, prevY, x, y, col, thick)
		} else {
			setThickPixel(img, x, y, thick, col)
		}
		prevX, prevY = x, y
	}
}

func drawArrow(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	drawLine(img, x0, y0, x1, y1, col, thick)
	angle := math.Atan2(float64(y1-y0), float64(x1-x0))
	size := float64(6 + thick*2)
	a1 := angle + math.Pi/6
	a2 := angle - math.Pi/6
	x2 := x1 - int(math.Cos(a1)*size)
	y2 := y1 - int(math.Sin(a1)*size)
	x3 := x1 - int(math.Cos(a2)*size)
	y3 := y1 - int(math.Sin(a2)*size)
	drawLine(img, x1, y1, x2, y2, col, thick)
	drawLine(img, x1, y1, x3, y3, col, thick)
}

func drawFilledCircle(img *image.RGBA, cx, cy, r int, col color.Color) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				px := cx + dx
				py := cy + dy
				if image.Pt(px, py).In(img.Bounds()) {
					img.Set(px, py, col)
				}
			}
		}
	}
}

func drawFilledEllipse(img *image.RGBA, cx, cy, rx, ry int, col color.Color) {
	for dy := -ry; dy <= ry; dy++ {
		span := int(float64(rx) * math.Sqrt(1.0-float64(dy*dy)/float64(ry*ry)))
		for dx := -span; dx <= span; dx++ {
			px := cx + dx
			py := cy + dy
			if image.Pt(px, py).In(img.Bounds()) {
				img.Set(px, py, col)
			}
		}
	}
}

// drawNumberBox draws a numbered annotation with the circle centred at (cx, cy).
// size controls the radius of the circle.
func drawNumberBox(img *image.RGBA, cx, cy, num int, col color.Color, size int) {
	r := size
	drawFilledCircle(img, cx, cy, r, col)

	cr, cg, cb, _ := col.RGBA()
	brightness := 0.299*float64(cr>>8) + 0.587*float64(cg>>8) + 0.114*float64(cb>>8)
	textCol := color.Black
	if brightness < 128 {
		textCol = color.White
	}

	text := fmt.Sprintf("%d", num)
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textCol),
		Face: basicfont.Face7x13,
	}
	w := d.MeasureString(text).Ceil()
	d.Dot = fixed.P(cx-w/2, cy+4)
	d.DrawString(text)
}

// ensureCanvasContains expands the tab's image so that rect (in image coordinates)
// fits within it. Existing image content keeps its on-screen position by
// adjusting the tab's offset when expansion occurs.
func ensureCanvasContains(t *Tab, rect image.Rectangle) image.Point {
	b := t.Image.Bounds()
	minX := 0
	if rect.Min.X < 0 {
		minX = rect.Min.X
	}
	minY := 0
	if rect.Min.Y < 0 {
		minY = rect.Min.Y
	}
	maxX := b.Max.X
	if rect.Max.X > maxX {
		maxX = rect.Max.X
	}
	maxY := b.Max.Y
	if rect.Max.Y > maxY {
		maxY = rect.Max.Y
	}
	if minX == 0 && minY == 0 && maxX == b.Max.X && maxY == b.Max.Y {
		return image.Point{}
	}
	newW := maxX - minX
	newH := maxY - minY
	newImg := image.NewRGBA(image.Rect(0, 0, newW, newH))
	// Fill the expanded canvas with transparency so the checkerboard shows through.
	draw.Draw(newImg, newImg.Bounds(), image.Transparent, image.Point{}, draw.Src)
	draw.Draw(newImg, b.Add(image.Pt(-minX, -minY)), t.Image, image.Point{}, draw.Src)
	t.Image = newImg
	t.Offset = t.Offset.Add(image.Pt(minX, minY))
	return image.Pt(minX, minY)
}

func drawDashedLine(img *image.RGBA, x0, y0, x1, y1, dash, thickness int, c1, c2 color.Color) {
	horiz := y0 == y1
	length := x1 - x0
	if !horiz {
		length = y1 - y0
	}
	if length < 0 {
		length = -length
	}
	for i := 0; i <= length; i += dash * 2 {
		for j := 0; j < dash && i+j <= length; j++ {
			col := c1
			if horiz {
				for t := 0; t < thickness; t++ {
					if x0 < x1 {
						img.Set(x0+i+j, y0+t, col)
					} else {
						img.Set(x0-i-j, y0+t, col)
					}
				}
			} else {
				for t := 0; t < thickness; t++ {
					if y0 < y1 {
						img.Set(x0+t, y0+i+j, col)
					} else {
						img.Set(x0+t, y0-i-j, col)
					}
				}
			}
		}
		for j := 0; j < dash && i+dash+j <= length; j++ {
			col := c2
			if horiz {
				for t := 0; t < thickness; t++ {
					if x0 < x1 {
						img.Set(x0+i+dash+j, y0+t, col)
					} else {
						img.Set(x0-i-dash-j, y0+t, col)
					}
				}
			} else {
				for t := 0; t < thickness; t++ {
					if y0 < y1 {
						img.Set(x0+t, y0+i+dash+j, col)
					} else {
						img.Set(x0+t, y0-i-dash-j, col)
					}
				}
			}
		}
	}
}

func drawDashedRect(img *image.RGBA, rect image.Rectangle, dash, thickness int, c1, c2 color.Color) {
	drawDashedLine(img, rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Max.X, rect.Min.Y, rect.Max.X, rect.Max.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Max.X, rect.Max.Y, rect.Min.X, rect.Max.Y, dash, thickness, c1, c2)
	drawDashedLine(img, rect.Min.X, rect.Max.Y, rect.Min.X, rect.Min.Y, dash, thickness, c1, c2)
}

func drawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thick int) {
	drawLine(img, rect.Min.X, rect.Min.Y, rect.Max.X-1, rect.Min.Y, col, thick)
	drawLine(img, rect.Max.X-1, rect.Min.Y, rect.Max.X-1, rect.Max.Y-1, col, thick)
	drawLine(img, rect.Max.X-1, rect.Max.Y-1, rect.Min.X, rect.Max.Y-1, col, thick)
	drawLine(img, rect.Min.X, rect.Max.Y-1, rect.Min.X, rect.Min.Y, col, thick)
}

func cropHandleRects(rect image.Rectangle) []image.Rectangle {
	hs := handleSize / 2
	cx := (rect.Min.X + rect.Max.X) / 2
	cy := (rect.Min.Y + rect.Max.Y) / 2
	return []image.Rectangle{
		image.Rect(rect.Min.X-hs, rect.Min.Y-hs, rect.Min.X+hs, rect.Min.Y+hs), // tl
		image.Rect(cx-hs, rect.Min.Y-hs, cx+hs, rect.Min.Y+hs),                 // t
		image.Rect(rect.Max.X-hs, rect.Min.Y-hs, rect.Max.X+hs, rect.Min.Y+hs), // tr
		image.Rect(rect.Max.X-hs, cy-hs, rect.Max.X+hs, cy+hs),                 // r
		image.Rect(rect.Max.X-hs, rect.Max.Y-hs, rect.Max.X+hs, rect.Max.Y+hs), // br
		image.Rect(cx-hs, rect.Max.Y-hs, cx+hs, rect.Max.Y+hs),                 // b
		image.Rect(rect.Min.X-hs, rect.Max.Y-hs, rect.Min.X+hs, rect.Max.Y+hs), // bl
		image.Rect(rect.Min.X-hs, cy-hs, rect.Min.X+hs, cy+hs),                 // l
	}
}

// cropImage returns a copy of the given rectangle from img. If rect extends
// outside img, the missing areas are left transparent so the canvas can grow.
func cropImage(img *image.RGBA, rect image.Rectangle) *image.RGBA {
	if rect.Empty() {
		return img
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	src := rect.Intersect(img.Bounds())
	if !src.Empty() {
		draw.Draw(out, src.Sub(rect.Min), img, src.Min, draw.Src)
	}
	return out
}

type paintState struct {
	width, height   int
	tabs            []Tab
	current         int
	tool            Tool
	colorIdx        int
	numberIdx       int
	cropping        bool
	cropRect        image.Rectangle
	cropStart       image.Point
	textInputActive bool
	textInput       string
	textPos         image.Point
	message         string
	messageUntil    time.Time
	handleShortcut  func(string)
}

func drawFrame(ctx context.Context, s screen.Screen, w screen.Window, st paintState) {
	b, err := s.NewBuffer(image.Point{st.width, st.height})
	if err != nil {
		log.Printf("new buffer: %v", err)
		return
	}
	defer b.Release()

	drawBackdrop(b.RGBA())
	if ctx.Err() != nil {
		return
	}

	img := st.tabs[st.current].Image
	zoom := st.tabs[st.current].Zoom
	base := imageRect(img, st.width, st.height, zoom)
	off := st.tabs[st.current].Offset
	dst := base.Add(image.Pt(int(float64(off.X)*zoom), int(float64(off.Y)*zoom)))
	xdraw.NearestNeighbor.Scale(b.RGBA(), dst, img, img.Bounds(), draw.Over, nil)
	if ctx.Err() != nil {
		return
	}

	if st.tool == ToolCrop && (st.cropping || !st.cropRect.Empty()) {
		sel := st.cropRect
		if st.cropping {
			sel = image.Rect(st.cropStart.X, st.cropStart.Y, st.cropStart.X, st.cropStart.Y).Union(sel)
		}
		r := image.Rect(
			dst.Min.X+int(float64(sel.Min.X)*zoom),
			dst.Min.Y+int(float64(sel.Min.Y)*zoom),
			dst.Min.X+int(float64(sel.Max.X)*zoom),
			dst.Min.Y+int(float64(sel.Max.Y)*zoom),
		)
		drawDashedRect(b.RGBA(), r, 4, 2, color.White, color.Black)
		for _, hr := range cropHandleRects(r) {
			if ctx.Err() != nil {
				return
			}
			draw.Draw(b.RGBA(), hr, &image.Uniform{color.White}, image.Point{}, draw.Src)
			drawRect(b.RGBA(), hr, color.Black, 1)
			drawDashedRect(b.RGBA(), hr, 2, 1, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
		}
	}

	if ctx.Err() != nil {
		return
	}

	drawTabs(b.RGBA(), st.tabs, st.current)
	drawToolbar(b.RGBA(), st.tool, st.colorIdx, st.tabs[st.current].WidthIdx, st.numberIdx)
	drawShortcuts(b.RGBA(), st.width, st.height, st.tool, st.textInputActive, zoom, st.handleShortcut)

	if ctx.Err() != nil {
		return
	}

	if st.message != "" && time.Now().Before(st.messageUntil) {
		d := &font.Drawer{Dst: b.RGBA(), Src: image.Black, Face: messageFace}
		wmsg := d.MeasureString(st.message).Ceil()
		ascent := messageFace.Metrics().Ascent.Ceil()
		descent := messageFace.Metrics().Descent.Ceil()
		px := (st.width - wmsg) / 2
		py := (st.height-ascent-descent)/2 + ascent
		rect := image.Rect(px-8, py-ascent-8, px+wmsg+8, py+descent+8)
		draw.Draw(b.RGBA(), rect, &image.Uniform{color.RGBA{255, 255, 255, 230}}, image.Point{}, draw.Over)
		drawRect(b.RGBA(), rect, color.Black, 2)
		d.Dot = fixed.P(px, py)
		d.DrawString(st.message)
	}

	if ctx.Err() != nil {
		return
	}

	if st.textInputActive {
		d := &font.Drawer{Dst: b.RGBA(), Src: image.NewUniform(palette[st.colorIdx]), Face: textFaces[textSizeIdx]}
		px := dst.Min.X + int(float64(st.textPos.X)*zoom)
		py := dst.Min.Y + int(float64(st.textPos.Y)*zoom)
		d.Dot = fixed.P(px, py)
		d.DrawString(st.textInput + "|")
	}

	if ctx.Err() != nil {
		return
	}

	w.Upload(image.Point{}, b, b.Bounds())
	w.Publish()
}
