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

	"github.com/arran4/spacemap"
	"github.com/arran4/spacemap/simplearray"
	"github.com/example/shineyshot/assets"
	"github.com/example/shineyshot/internal/theme"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
)

const (
	tabHeight    = 24
	bottomHeight = 24
)

const ProgramTitle = "ShineyShot"

var toolbarWidth = 48

func CalculateToolbarWidth(versionLabel string) int {
	d := &font.Drawer{Face: basicfont.Face7x13}
	max := d.MeasureString(ProgramTitle).Ceil() + 8 // padding
	if icon := toolbarIconImage(); icon != nil {
		max += icon.Bounds().Dx() + 4
	}
	if versionLabel != "" {
		if w := d.MeasureString(versionLabel).Ceil() + 8; w > max {
			max = w
		}
	}
	toolLabels := []string{"Move(M)", "Crop(R)", "Draw(B)", "Circle(O)", "Line(L)", "Arrow(A)", "Rect(X)", "Num(H)", "Text(T)", "Shadow($)"}
	for _, lbl := range toolLabels {
		w := d.MeasureString(lbl).Ceil() + 8
		if w > max {
			max = w
		}
	}
	if max < 48 {
		return 48
	}
	return max
}

var (
	toolbarIconOnce sync.Once
	toolbarIcon     image.Image
)

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
	ToolShadow
)

// Mode controls the available interactions in the UI.
type Mode int

const (
	// ModeAnnotate enables the full annotation toolset.
	ModeAnnotate Mode = iota
	// ModePreview restricts the UI to viewing until annotation is requested.
	ModePreview
)

const (
	defaultColorIndex = 2
	defaultWidthIndex = 2
)

type Tab struct {
	Image *image.RGBA
	Title string
	// Offset is stored in image coordinates so it is independent of zoom.
	Offset        image.Point
	Zoom          float64
	NextNumber    int
	WidthIdx      int
	ShadowApplied bool
}

// TabSummary provides identifying information for an open annotation tab.
type TabSummary struct {
	Index int
	Title string
}

// TabsState reports the collection of tabs alongside the active index.
type TabsState struct {
	Tabs    []TabSummary
	Current int
}

// TabChange describes a tab update emitted from the UI.
type TabChange struct {
	TabsState
	Image    *image.RGBA
	WidthIdx int
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

type UIType int

const (
	UITypeTab UIType = iota
	UITypeTool
	UITypePalette
	UITypeWidth
	UITypeNumber
	UITypeTextSize
	UITypeShortcut
)

type UIShape struct {
	Rect  image.Rectangle
	Type  UIType
	Index int
}

func (u *UIShape) PointIn(x, y int) bool {
	return image.Pt(x, y).In(u.Rect)
}

func (u *UIShape) Bounds() image.Rectangle {
	return u.Rect
}

func (u *UIShape) String() string {
	return fmt.Sprintf("UI:%v:%d", u.Type, u.Index)
}

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

var textSizes = []float64{12, 16, 20, 24, 32}
var textFaces []font.Face
var textSizeIdx int
var messageFace font.Face
var goregularFont *opentype.Font

func init() {
	var err error
	goregularFont, err = opentype.Parse(goregular.TTF)
	if err != nil {
		log.Fatalf("parse font: %v", err)
	}
	for _, sz := range textSizes {
		face, err := opentype.NewFace(goregularFont, &opentype.FaceOptions{Size: sz, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			log.Fatalf("font face: %v", err)
		}
		textFaces = append(textFaces, face)
	}
	messageFace, err = opentype.NewFace(goregularFont, &opentype.FaceOptions{Size: 48, DPI: 72, Hinting: font.HintingFull})
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

func toolbarIconImage() image.Image {
	toolbarIconOnce.Do(func() {
		for _, size := range []int{24, 22, 16, 32} {
			img, err := assets.IconImage(size)
			if err == nil {
				toolbarIcon = img
				return
			}
		}
	})
	return toolbarIcon
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
func drawBackdrop(dst *image.RGBA, t *theme.Theme) {
	b := dst.Bounds()
	if backdropCache == nil || backdropCache.Bounds() != b {
		backdropCache = image.NewRGBA(b)
	}
	drawCheckerboard(backdropCache, backdropCache.Bounds(), 8, t.CheckerLight, t.CheckerDark)
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
	Draw(dst *image.RGBA, state ButtonState, t *theme.Theme)
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

func (cb *CacheButton) Draw(dst *image.RGBA, state ButtonState, t *theme.Theme) {
	rect := cb.Button.Rect()
	img := image.NewRGBA(rect)
	draw.Draw(img, rect, image.Transparent, image.Point{}, draw.Src)
	cb.Button.Draw(img, state, t)
	draw.Draw(dst, cb.Button.Rect(), img, cb.Button.Rect().Min, draw.Over)
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

func (s *Shortcut) Draw(dst *image.RGBA, state ButtonState, t *theme.Theme) {
	col := t.ButtonBackground
	textCol := t.ButtonText
	switch state {
	case StateHover:
		col = t.ButtonBackgroundHover
		textCol = t.ButtonTextHover
	case StatePressed:
		col = t.ButtonBackgroundPress
		textCol = t.ButtonTextPress
	}
	draw.Draw(dst, s.rect, &image.Uniform{col}, image.Point{}, draw.Src)
	drawRect(dst, s.rect, t.ButtonBorder, 1)
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(textCol), Face: basicfont.Face7x13,
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

func (tb *ToolButton) Draw(dst *image.RGBA, state ButtonState, t *theme.Theme) {
	c := t.ButtonBackground
	textCol := t.ButtonText
	switch state {
	case StateHover:
		c = t.ButtonBackgroundHover
		textCol = t.ButtonTextHover
	case StatePressed:
		c = t.ButtonBackgroundPress
		textCol = t.ButtonTextPress
	}
	draw.Draw(dst, tb.rect, &image.Uniform{c}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(textCol), Face: basicfont.Face7x13,
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

// ActionButton represents a toolbar button that performs a single action.
type ActionButton struct {
	label      string
	rect       image.Rectangle
	onActivate func()
}

var _ Button = (*ActionButton)(nil)

func (ab *ActionButton) Draw(dst *image.RGBA, state ButtonState, t *theme.Theme) {
	c := t.ButtonBackground
	textCol := t.ButtonText
	switch state {
	case StateHover:
		c = t.ButtonBackgroundHover
		textCol = t.ButtonTextHover
	case StatePressed:
		c = t.ButtonBackgroundPress
		textCol = t.ButtonTextPress
	}
	draw.Draw(dst, ab.rect, &image.Uniform{c}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(textCol), Face: basicfont.Face7x13,
		Dot: fixed.P(ab.rect.Min.X+4, ab.rect.Min.Y+16)}
	d.DrawString(ab.label)
}

func (ab *ActionButton) Rect() image.Rectangle { return ab.rect }

func (ab *ActionButton) SetRect(r image.Rectangle) {
	if r != ab.rect {
		ab.rect = r
	}
}

func (ab *ActionButton) Activate() {
	if ab.onActivate != nil {
		ab.onActivate()
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

func (tb *TabButton) Draw(dst *image.RGBA, state ButtonState, t *theme.Theme) {
	c := t.TabBackground
	textCol := t.TabText
	switch state {
	case StateHover:
		c = t.TabHover
		textCol = t.TabTextHover
	case StatePressed:
		c = t.TabActive
		textCol = t.TabTextActive
	}
	draw.Draw(dst, tb.rect, &image.Uniform{c}, image.Point{}, draw.Src)
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(textCol), Face: basicfont.Face7x13,
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

func drawTabs(dst *image.RGBA, tabs []Tab, current int, t *theme.Theme, sm spacemap.Interface) {
	// background for title area
	draw.Draw(dst, image.Rect(0, 0, toolbarWidth, tabHeight),
		&image.Uniform{t.ToolbarBackground}, image.Point{}, draw.Src)

	// program title in the top-left corner
	title := ProgramTitle
	icon := toolbarIconImage()
	textX := 4
	if icon != nil {
		bounds := icon.Bounds()
		iconY := (tabHeight - bounds.Dy()) / 2
		if iconY < 0 {
			iconY = 0
		}
		rect := image.Rect(textX, iconY, textX+bounds.Dx(), iconY+bounds.Dy())
		draw.Draw(dst, rect, icon, bounds.Min, draw.Over)
		textX = rect.Max.X + 4
	}
	d := &font.Drawer{Dst: dst, Src: image.NewUniform(t.Foreground), Face: basicfont.Face7x13,
		Dot: fixed.P(textX, 16)}
	d.DrawString(title)

	tabButtons = tabButtons[:0]
	x := toolbarWidth
	for i, t2 := range tabs {
		tb := TabButton{label: t2.Title, onSelect: nil}
		tb.SetRect(image.Rect(x, 0, x+80, tabHeight))
		if sm != nil {
			sm.Add(&UIShape{Rect: tb.Rect(), Type: UITypeTab, Index: i}, 0)
		}
		state := StateDefault
		switch i {
		case current:
			state = StatePressed
		case hoverTab:
			state = StateHover
		}
		tb.Draw(dst, state, t)
		tabButtons = append(tabButtons, tb)
		x += 80
	}
	// fill remainder of bar
	draw.Draw(dst, image.Rect(x, 0, dst.Bounds().Dx(), tabHeight),
		&image.Uniform{t.ToolbarBackground}, image.Point{}, draw.Src)
}

func drawShortcuts(dst *image.RGBA, width, height int, tool Tool, textMode bool, z float64, trigger func(string), annotationEnabled bool, versionLabel string, t *theme.Theme, sm spacemap.Interface) {
	rect := image.Rect(0, height-bottomHeight, width, height)
	draw.Draw(dst, rect, &image.Uniform{t.ToolbarBackground}, image.Point{}, draw.Src)
	shortcutRects = shortcutRects[:0]
	zoomStr := fmt.Sprintf("+/-:zoom (%.0f%%)", z*100)
	var shortcuts []Shortcut
	if textMode {
		shortcuts = []Shortcut{
			{label: "Enter:place", action: func() { trigger("textdone") }},
			{label: "Esc:cancel", action: func() { trigger("textcancel") }},
		}
	} else {
		if annotationEnabled {
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
		} else {
			shortcuts = []Shortcut{
				{label: zoomStr, action: func() { trigger("zoom") }},
				{label: "^C:copy image", action: func() { trigger("copy") }},
				{label: "^S:save", action: func() { trigger("save") }},
				{label: "A:annotate", action: func() { trigger("annotate") }},
				{label: "Q:quit", action: func() { trigger("quit") }},
			}
		}
	}
	x := toolbarWidth + 4
	y := height - bottomHeight + 16
	if versionLabel != "" {
		d := &font.Drawer{Dst: dst, Src: image.NewUniform(t.Foreground), Face: basicfont.Face7x13,
			Dot: fixed.P(4, y)}
		d.DrawString(versionLabel)
	}
	meas := &font.Drawer{Face: basicfont.Face7x13}
	for i := range shortcuts {
		sc := &shortcuts[i]
		w := meas.MeasureString(sc.label).Ceil()
		sc.SetRect(image.Rect(x-2, y-14, x+w+2, y+4))
		if sm != nil {
			sm.Add(&UIShape{Rect: sc.Rect(), Type: UITypeShortcut, Index: i}, 0)
		}
		state := StateDefault
		if i == hoverShortcut {
			state = StateHover
		}
		sc.Draw(dst, state, t)
		shortcutRects = append(shortcutRects, *sc)
		x = sc.rect.Max.X + 8
	}
}

func drawToolbar(dst *image.RGBA, tool Tool, colIdx, widthIdx, numberIdx int, annotationEnabled bool, shadowUsed bool, buttons []Button, t *theme.Theme, sm spacemap.Interface) {
	y := tabHeight
	for i, cb := range buttons {
		r := image.Rect(0, y, toolbarWidth, y+24)
		cb.SetRect(r)
		if sm != nil {
			sm.Add(&UIShape{Rect: cb.Rect(), Type: UITypeTool, Index: i}, 0)
		}
		state := StateDefault

		var inner Button = cb
		if cache, ok := cb.(*CacheButton); ok {
			inner = cache.Button
		}

		switch b := inner.(type) {
		case *ToolButton:
			if b.tool == ToolShadow && shadowUsed {
				state = StatePressed
			} else if b.tool == tool {
				state = StatePressed
			} else if i == hoverTool {
				state = StateHover
			}
		default:
			if i == hoverTool {
				state = StateHover
			}
		}
		cb.Draw(dst, state, t)
		y += 24
	}

	if !annotationEnabled {
		return
	}

	// color palette below tools
	y += 4
	x := 4
	paletteRects = paletteRects[:0]
	for i, p := range palette {
		rect := image.Rect(x, y, x+16, y+16)
		if sm != nil {
			sm.Add(&UIShape{Rect: rect, Type: UITypePalette, Index: i}, 0)
		}
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
			if sm != nil {
				sm.Add(&UIShape{Rect: rect, Type: UITypeWidth, Index: i}, 0)
			}
			c := t.ButtonBackground
			switch i {
			case widthIdx:
				c = t.ButtonBackgroundPress
			case hoverWidth:
				c = t.ButtonBackgroundHover
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.NewUniform(t.ButtonText), Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
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
			if sm != nil {
				sm.Add(&UIShape{Rect: rect, Type: UITypeNumber, Index: i}, 0)
			}
			c := t.ButtonBackground
			switch i {
			case numberIdx:
				c = t.ButtonBackgroundPress
			case hoverNumber:
				c = t.ButtonBackgroundHover
			}
			draw.Draw(dst, rect, &image.Uniform{c}, image.Point{}, draw.Src)
			d := &font.Drawer{Dst: dst, Src: image.NewUniform(t.ButtonText), Face: basicfont.Face7x13, Dot: fixed.P(4, y+12)}
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
			if sm != nil {
				sm.Add(&UIShape{Rect: rect, Type: UITypeTextSize, Index: i}, 0)
			}
			c := t.ButtonBackground
			switch i {
			case textSizeIdx:
				c = t.ButtonBackgroundPress
			case hoverTextSize:
				c = t.ButtonBackgroundHover
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

type PaintState struct {
	Width, Height     int
	Tabs              []Tab
	Current           int
	Tool              Tool
	ColorIdx          int
	NumberIdx         int
	Cropping          bool
	CropRect          image.Rectangle
	CropStart         image.Point
	TextInputActive   bool
	TextInput         string
	TextPos           image.Point
	Message           string
	MessageUntil      time.Time
	HandleShortcut    func(string)
	AnnotationEnabled bool
	VersionLabel      string
	Theme             *theme.Theme
	ToolButtons       []Button
	SetUIMap          func(spacemap.Interface)
}

func DefaultToolButtons(annotationEnabled bool) []Button {
	var buttons []Button
	if annotationEnabled {
		buttons = []Button{
			&CacheButton{Button: &ToolButton{label: "Move(M)", tool: ToolMove, atype: actionMove}},
			&CacheButton{Button: &ToolButton{label: "Crop(R)", tool: ToolCrop, atype: actionCrop}},
			&CacheButton{Button: &ToolButton{label: "Draw(B)", tool: ToolDraw, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Circle(O)", tool: ToolCircle, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Line(L)", tool: ToolLine, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Arrow(A)", tool: ToolArrow, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Rect(X)", tool: ToolRect, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Num(H)", tool: ToolNumber, atype: actionDraw}},
			&CacheButton{Button: &ToolButton{label: "Text(T)", tool: ToolText, atype: actionNone}},
			&CacheButton{Button: &ToolButton{label: "Shadow($)", tool: ToolShadow, atype: actionNone}},
		}
	} else {
		buttons = []Button{
			&CacheButton{Button: &ActionButton{label: "Annotate"}},
		}
	}
	return buttons
}

func DrawScene(ctx context.Context, b *image.RGBA, st PaintState) {
	sm := simplearray.New()

	t := st.Theme
	if t == nil {
		t = theme.Default()
	}

	// Ensure toolbar width is correct for the current state
	toolbarWidth = CalculateToolbarWidth(st.VersionLabel)

	drawBackdrop(b, t)
	if ctx != nil && ctx.Err() != nil {
		return
	}

	img := st.Tabs[st.Current].Image
	zoom := st.Tabs[st.Current].Zoom
	base := imageRect(img, st.Width, st.Height, zoom)
	off := st.Tabs[st.Current].Offset
	dst := base.Add(image.Pt(int(float64(off.X)*zoom), int(float64(off.Y)*zoom)))
	xdraw.NearestNeighbor.Scale(b, dst, img, img.Bounds(), draw.Over, nil)
	if ctx != nil && ctx.Err() != nil {
		return
	}

	if st.Tool == ToolCrop && (st.Cropping || !st.CropRect.Empty()) {
		sel := st.CropRect
		if st.Cropping {
			sel = image.Rect(st.CropStart.X, st.CropStart.Y, st.CropStart.X, st.CropStart.Y).Union(sel)
		}
		r := image.Rect(
			dst.Min.X+int(float64(sel.Min.X)*zoom),
			dst.Min.Y+int(float64(sel.Min.Y)*zoom),
			dst.Min.X+int(float64(sel.Max.X)*zoom),
			dst.Min.Y+int(float64(sel.Max.Y)*zoom),
		)
		drawDashedRect(b, r, 4, 2, color.White, color.Black)
		for _, hr := range cropHandleRects(r) {
			if ctx != nil && ctx.Err() != nil {
				return
			}
			draw.Draw(b, hr, &image.Uniform{color.White}, image.Point{}, draw.Src)
			drawRect(b, hr, color.Black, 1)
			drawDashedRect(b, hr, 2, 1, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
		}
	}

	if ctx != nil && ctx.Err() != nil {
		return
	}

	drawTabs(b, st.Tabs, st.Current, t, sm)
	drawToolbar(b, st.Tool, st.ColorIdx, st.Tabs[st.Current].WidthIdx, st.NumberIdx, st.AnnotationEnabled, st.Tabs[st.Current].ShadowApplied, st.ToolButtons, t, sm)
	drawShortcuts(b, st.Width, st.Height, st.Tool, st.TextInputActive, zoom, st.HandleShortcut, st.AnnotationEnabled, st.VersionLabel, t, sm)

	if st.SetUIMap != nil {
		st.SetUIMap(sm)
	}

	if ctx != nil && ctx.Err() != nil {
		return
	}

	if st.Message != "" && time.Now().Before(st.MessageUntil) {
		d := &font.Drawer{Dst: b, Src: image.Black, Face: messageFace}
		wmsg := d.MeasureString(st.Message).Ceil()
		ascent := messageFace.Metrics().Ascent.Ceil()
		descent := messageFace.Metrics().Descent.Ceil()
		px := (st.Width - wmsg) / 2
		py := (st.Height-ascent-descent)/2 + ascent
		rect := image.Rect(px-8, py-ascent-8, px+wmsg+8, py+descent+8)
		draw.Draw(b, rect, &image.Uniform{color.RGBA{255, 255, 255, 230}}, image.Point{}, draw.Over)
		drawRect(b, rect, color.Black, 2)
		d.Dot = fixed.P(px, py)
		d.DrawString(st.Message)
	}

	if ctx != nil && ctx.Err() != nil {
		return
	}

	if st.TextInputActive {
		d := &font.Drawer{Dst: b, Src: image.NewUniform(palette[st.ColorIdx]), Face: textFaces[textSizeIdx]}
		px := dst.Min.X + int(float64(st.TextPos.X)*zoom)
		py := dst.Min.Y + int(float64(st.TextPos.Y)*zoom)
		d.Dot = fixed.P(px, py)
		d.DrawString(st.TextInput + "|")
	}
}

func drawFrame(ctx context.Context, s screen.Screen, w screen.Window, st PaintState) {
	b, err := s.NewBuffer(image.Point{st.Width, st.Height})
	if err != nil {
		log.Printf("new buffer: %v", err)
		return
	}
	defer b.Release()

	DrawScene(ctx, b.RGBA(), st)

	if ctx.Err() != nil {
		return
	}

	w.Upload(image.Point{}, b, b.Bounds())
	w.Publish()
}
