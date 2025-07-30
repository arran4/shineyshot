package main

import (
	"bytes"
	"context"
	"flag"
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
	"image/png"
	"log"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/godbus/dbus/v5"

	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
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
	ToolView Tool = iota
	ToolModify
	ToolCrop
	ToolDraw
	ToolCircle
	ToolLine
	ToolArrow
	ToolRect
	ToolNumber
	ToolText
)

type Tab struct {
	Image *image.RGBA
	Title string
	// Offset is stored in image coordinates so it is independent of zoom.
	Offset     image.Point
	Zoom       float64
	NextNumber int
	WidthIdx   int
	Components []Component
}

// Component describes a drawable element on the canvas.
type Component interface {
	Draw(dst *image.RGBA, off image.Point)
	DrawScaled(dst *image.RGBA, off image.Point, zoom float64)
	Bounds() image.Rectangle
	MoveBy(dx, dy int)
	IsImage() bool
	Contains(p image.Point) bool
}

// ImageComponent draws a raster image at a position. If Image is true
// the element is considered an image for layering purposes.
type ImageComponent struct {
	Img   *image.RGBA
	Pos   image.Point
	Image bool
}

func (c *ImageComponent) Draw(dst *image.RGBA, off image.Point) {
	draw.Draw(dst, c.Img.Bounds().Add(c.Pos.Add(off)), c.Img, image.Point{}, draw.Over)
}

func (c *ImageComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	r := image.Rect(
		int(float64(c.Pos.X+off.X)*zoom),
		int(float64(c.Pos.Y+off.Y)*zoom),
		int(float64(c.Pos.X+off.X+c.Img.Bounds().Dx())*zoom),
		int(float64(c.Pos.Y+off.Y+c.Img.Bounds().Dy())*zoom),
	)
	xdraw.NearestNeighbor.Scale(dst, r, c.Img, c.Img.Bounds(), draw.Over, nil)
}

func (c *ImageComponent) Bounds() image.Rectangle     { return c.Img.Bounds().Add(c.Pos) }
func (c *ImageComponent) MoveBy(dx, dy int)           { c.Pos.X += dx; c.Pos.Y += dy }
func (c *ImageComponent) IsImage() bool               { return c.Image }
func (c *ImageComponent) Contains(p image.Point) bool { return p.In(c.Bounds()) }

// LineComponent draws a straight line between two points.
type LineComponent struct {
	Start, End image.Point
	Col        color.Color
	Width      int
}

func (c *LineComponent) Draw(dst *image.RGBA, off image.Point) {
	drawLine(dst, c.Start.X+off.X, c.Start.Y+off.Y, c.End.X+off.X, c.End.Y+off.Y, c.Col, c.Width)
}

func (c *LineComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	x0 := int(float64(c.Start.X+off.X) * zoom)
	y0 := int(float64(c.Start.Y+off.Y) * zoom)
	x1 := int(float64(c.End.X+off.X) * zoom)
	y1 := int(float64(c.End.Y+off.Y) * zoom)
	w := int(float64(c.Width) * zoom)
	if w < 1 {
		w = 1
	}
	drawLine(dst, x0, y0, x1, y1, c.Col, w)
}

func (c *LineComponent) Bounds() image.Rectangle {
	minX := c.Start.X
	if c.End.X < minX {
		minX = c.End.X
	}
	minY := c.Start.Y
	if c.End.Y < minY {
		minY = c.End.Y
	}
	maxX := c.Start.X
	if c.End.X > maxX {
		maxX = c.End.X
	}
	maxY := c.Start.Y
	if c.End.Y > maxY {
		maxY = c.End.Y
	}
	pad := c.Width + 2
	return image.Rect(minX-pad, minY-pad, maxX+pad, maxY+pad)
}

func (c *LineComponent) MoveBy(dx, dy int) {
	c.Start = c.Start.Add(image.Pt(dx, dy))
	c.End = c.End.Add(image.Pt(dx, dy))
}

func (c *LineComponent) IsImage() bool               { return false }
func (c *LineComponent) Contains(p image.Point) bool { return p.In(c.Bounds()) }

func (c *LineComponent) HandleRects() []image.Rectangle {
	hs := handleSize / 2
	rects := make([]image.Rectangle, 8)
	rects[0] = image.Rect(c.Start.X-hs, c.Start.Y-hs, c.Start.X+hs, c.Start.Y+hs)
	rects[4] = image.Rect(c.End.X-hs, c.End.Y-hs, c.End.X+hs, c.End.Y+hs)
	return rects
}

func (c *LineComponent) Resize(mode cropAction, start image.Rectangle, dx, dy int) {
	switch mode {
	case cropMove:
		c.MoveBy(dx, dy)
	case cropResizeTL, cropResizeL, cropResizeBL:
		c.Start = compStartA.Add(image.Pt(dx, dy))
	case cropResizeBR, cropResizeR, cropResizeTR:
		c.End = compStartB.Add(image.Pt(dx, dy))
	}
}

// ArrowComponent draws a line with an arrow head.
type ArrowComponent struct {
	Start, End image.Point
	Col        color.Color
	Width      int
}

func (c *ArrowComponent) Draw(dst *image.RGBA, off image.Point) {
	drawArrow(dst, c.Start.X+off.X, c.Start.Y+off.Y, c.End.X+off.X, c.End.Y+off.Y, c.Col, c.Width)
}

func (c *ArrowComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	x0 := int(float64(c.Start.X+off.X) * zoom)
	y0 := int(float64(c.Start.Y+off.Y) * zoom)
	x1 := int(float64(c.End.X+off.X) * zoom)
	y1 := int(float64(c.End.Y+off.Y) * zoom)
	w := int(float64(c.Width) * zoom)
	if w < 1 {
		w = 1
	}
	drawArrow(dst, x0, y0, x1, y1, c.Col, w)
}

func (c *ArrowComponent) Bounds() image.Rectangle {
	minX := c.Start.X
	if c.End.X < minX {
		minX = c.End.X
	}
	minY := c.Start.Y
	if c.End.Y < minY {
		minY = c.End.Y
	}
	maxX := c.Start.X
	if c.End.X > maxX {
		maxX = c.End.X
	}
	maxY := c.Start.Y
	if c.End.Y > maxY {
		maxY = c.End.Y
	}
	pad := c.Width + 10
	return image.Rect(minX-pad, minY-pad, maxX+pad, maxY+pad)
}

func (c *ArrowComponent) MoveBy(dx, dy int) {
	c.Start = c.Start.Add(image.Pt(dx, dy))
	c.End = c.End.Add(image.Pt(dx, dy))
}

func (c *ArrowComponent) IsImage() bool               { return false }
func (c *ArrowComponent) Contains(p image.Point) bool { return p.In(c.Bounds()) }

func (c *ArrowComponent) HandleRects() []image.Rectangle {
	hs := handleSize / 2
	rects := make([]image.Rectangle, 8)
	rects[0] = image.Rect(c.Start.X-hs, c.Start.Y-hs, c.Start.X+hs, c.Start.Y+hs)
	rects[4] = image.Rect(c.End.X-hs, c.End.Y-hs, c.End.X+hs, c.End.Y+hs)
	return rects
}

func (c *ArrowComponent) Resize(mode cropAction, start image.Rectangle, dx, dy int) {
	switch mode {
	case cropMove:
		c.MoveBy(dx, dy)
	case cropResizeTL, cropResizeL, cropResizeBL:
		c.Start = compStartA.Add(image.Pt(dx, dy))
	case cropResizeBR, cropResizeR, cropResizeTR:
		c.End = compStartB.Add(image.Pt(dx, dy))
	}
}

// RectComponent draws a rectangle.
type RectComponent struct {
	Rect  image.Rectangle
	Col   color.Color
	Width int
}

func (c *RectComponent) Draw(dst *image.RGBA, off image.Point) {
	drawRect(dst, c.Rect.Add(off), c.Col, c.Width)
}

func (c *RectComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	r := image.Rect(
		int(float64(c.Rect.Min.X+off.X)*zoom),
		int(float64(c.Rect.Min.Y+off.Y)*zoom),
		int(float64(c.Rect.Max.X+off.X)*zoom),
		int(float64(c.Rect.Max.Y+off.Y)*zoom),
	)
	w := int(float64(c.Width) * zoom)
	if w < 1 {
		w = 1
	}
	drawRect(dst, r, c.Col, w)
}

func (c *RectComponent) Bounds() image.Rectangle {
	pad := c.Width / 2
	return c.Rect.Inset(-pad - 1)
}

func (c *RectComponent) MoveBy(dx, dy int)           { c.Rect = c.Rect.Add(image.Pt(dx, dy)) }
func (c *RectComponent) IsImage() bool               { return false }
func (c *RectComponent) Contains(p image.Point) bool { return p.In(c.Rect) }
func (c *RectComponent) HandleRects() []image.Rectangle {
	return cropHandleRects(c.Rect)
}

func (c *RectComponent) Resize(mode cropAction, start image.Rectangle, dx, dy int) {
	r := start
	switch mode {
	case cropMove:
		r = r.Add(image.Pt(dx, dy))
	case cropResizeTL:
		r.Min.X = start.Min.X + dx
		r.Min.Y = start.Min.Y + dy
	case cropResizeT:
		r.Min.Y = start.Min.Y + dy
	case cropResizeTR:
		r.Min.Y = start.Min.Y + dy
		r.Max.X = start.Max.X + dx
	case cropResizeR:
		r.Max.X = start.Max.X + dx
	case cropResizeBR:
		r.Max.X = start.Max.X + dx
		r.Max.Y = start.Max.Y + dy
	case cropResizeB:
		r.Max.Y = start.Max.Y + dy
	case cropResizeBL:
		r.Min.X = start.Min.X + dx
		r.Max.Y = start.Max.Y + dy
	case cropResizeL:
		r.Min.X = start.Min.X + dx
	}
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	c.Rect = r
}

// CircleComponent draws a circle.
type CircleComponent struct {
	Center image.Point
	Radius int
	Col    color.Color
	Width  int
}

func (c *CircleComponent) Draw(dst *image.RGBA, off image.Point) {
	drawCircle(dst, c.Center.X+off.X, c.Center.Y+off.Y, c.Radius, c.Col, c.Width)
}

func (c *CircleComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	cx := int(float64(c.Center.X+off.X) * zoom)
	cy := int(float64(c.Center.Y+off.Y) * zoom)
	r := int(float64(c.Radius) * zoom)
	w := int(float64(c.Width) * zoom)
	if w < 1 {
		w = 1
	}
	drawCircle(dst, cx, cy, r, c.Col, w)
}

func (c *CircleComponent) Bounds() image.Rectangle {
	pad := c.Radius + c.Width/2 + 1
	return image.Rect(c.Center.X-pad, c.Center.Y-pad, c.Center.X+pad, c.Center.Y+pad)
}

func (c *CircleComponent) MoveBy(dx, dy int) { c.Center = c.Center.Add(image.Pt(dx, dy)) }
func (c *CircleComponent) IsImage() bool     { return false }
func (c *CircleComponent) Contains(p image.Point) bool {
	dx := p.X - c.Center.X
	dy := p.Y - c.Center.Y
	r := c.Radius + c.Width/2
	return dx*dx+dy*dy <= r*r
}

func (c *CircleComponent) HandleRects() []image.Rectangle {
	hs := handleSize / 2
	rr := c.Radius + c.Width/2
	ang := []float64{3 * math.Pi / 4, math.Pi / 2, math.Pi / 4, 0, -math.Pi / 4, -math.Pi / 2, -3 * math.Pi / 4, math.Pi}
	out := make([]image.Rectangle, 0, 8)
	for _, a := range ang {
		x := c.Center.X + int(math.Cos(a)*float64(rr))
		y := c.Center.Y - int(math.Sin(a)*float64(rr))
		out = append(out, image.Rect(x-hs, y-hs, x+hs, y+hs))
	}
	return out
}

func (c *CircleComponent) Resize(mode cropAction, start image.Rectangle, dx, dy int) {
	r := start
	switch mode {
	case cropMove:
		r = r.Add(image.Pt(dx, dy))
	case cropResizeTL:
		r.Min.X = start.Min.X + dx
		r.Min.Y = start.Min.Y + dy
	case cropResizeT:
		r.Min.Y = start.Min.Y + dy
	case cropResizeTR:
		r.Min.Y = start.Min.Y + dy
		r.Max.X = start.Max.X + dx
	case cropResizeR:
		r.Max.X = start.Max.X + dx
	case cropResizeBR:
		r.Max.X = start.Max.X + dx
		r.Max.Y = start.Max.Y + dy
	case cropResizeB:
		r.Max.Y = start.Max.Y + dy
	case cropResizeBL:
		r.Min.X = start.Min.X + dx
		r.Max.Y = start.Max.Y + dy
	case cropResizeL:
		r.Min.X = start.Min.X + dx
	}
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2
	radx := (r.Max.X-r.Min.X)/2 - c.Width/2 - 1
	rady := (r.Max.Y-r.Min.Y)/2 - c.Width/2 - 1
	if radx > rady {
		c.Radius = radx
	} else {
		c.Radius = rady
	}
	if c.Radius < 1 {
		c.Radius = 1
	}
	c.Center = image.Point{cx, cy}
}

// NumberComponent draws a numbered marker.
type NumberComponent struct {
	Pos  image.Point
	Num  int
	Size int
	Col  color.Color
}

func (c *NumberComponent) Draw(dst *image.RGBA, off image.Point) {
	drawNumberBox(dst, c.Pos.X+off.X, c.Pos.Y+off.Y, c.Num, c.Col, c.Size)
}

func (c *NumberComponent) DrawScaled(dst *image.RGBA, off image.Point, zoom float64) {
	px := int(float64(c.Pos.X+off.X) * zoom)
	py := int(float64(c.Pos.Y+off.Y) * zoom)
	s := int(float64(c.Size) * zoom)
	if s < 1 {
		s = 1
	}
	drawNumberBox(dst, px, py, c.Num, c.Col, s)
}

func (c *NumberComponent) Bounds() image.Rectangle {
	r := c.Size + 2
	return image.Rect(c.Pos.X-r, c.Pos.Y-r, c.Pos.X+r, c.Pos.Y+r)
}

func (c *NumberComponent) MoveBy(dx, dy int)           { c.Pos = c.Pos.Add(image.Pt(dx, dy)) }
func (c *NumberComponent) IsImage() bool               { return false }
func (c *NumberComponent) Contains(p image.Point) bool { return p.In(c.Bounds()) }

const handleSize = 16

var compStartA image.Point
var compStartB image.Point

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

var palette = []color.RGBA{
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

var widths = []int{1, 2, 4, 6, 8}
var numberSizes = []int{8, 12, 16, 20, 24}

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
	case ToolView, ToolModify:
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
			{label: "^Shift+V:paste new", action: func() { trigger("pasteNew") }},
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

func drawDashedCircle(img *image.RGBA, cx, cy, r, dash, thickness int, c1, c2 color.Color) {
	steps := int(2 * math.Pi * float64(r))
	if steps < 1 {
		steps = 1
	}
	for i := 0; i < steps; i += dash * 2 {
		for j := 0; j < dash && i+j < steps; j++ {
			a0 := 2 * math.Pi * float64(i+j) / float64(steps)
			a1 := 2 * math.Pi * float64(i+j+1) / float64(steps)
			x0 := cx + int(math.Cos(a0)*float64(r))
			y0 := cy - int(math.Sin(a0)*float64(r))
			x1 := cx + int(math.Cos(a1)*float64(r))
			y1 := cy - int(math.Sin(a1)*float64(r))
			drawLine(img, x0, y0, x1, y1, c1, thickness)
		}
		for j := 0; j < dash && i+dash+j < steps; j++ {
			a0 := 2 * math.Pi * float64(i+dash+j) / float64(steps)
			a1 := 2 * math.Pi * float64(i+dash+j+1) / float64(steps)
			x0 := cx + int(math.Cos(a0)*float64(r))
			y0 := cy - int(math.Sin(a0)*float64(r))
			x1 := cx + int(math.Cos(a1)*float64(r))
			y1 := cy - int(math.Sin(a1)*float64(r))
			drawLine(img, x0, y0, x1, y1, c2, thickness)
		}
	}
}

func freehandComponent(points []image.Point, col color.Color, width int) Component {
	if len(points) < 2 {
		img := image.NewRGBA(image.Rect(0, 0, width, width))
		drawFilledCircle(img, width/2, width/2, width/2, col)
		return &ImageComponent{Img: img, Pos: points[0], Image: false}
	}
	minX, minY := points[0].X, points[0].Y
	maxX, maxY := minX, minY
	for _, p := range points[1:] {
		if p.X < minX {
			minX = p.X
		}
		if p.Y < minY {
			minY = p.Y
		}
		if p.X > maxX {
			maxX = p.X
		}
		if p.Y > maxY {
			maxY = p.Y
		}
	}
	pad := width/2 + 1
	rect := image.Rect(minX-pad, minY-pad, maxX+pad, maxY+pad)
	img := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	last := points[0]
	for _, p := range points[1:] {
		drawLine(img, last.X-rect.Min.X, last.Y-rect.Min.Y, p.X-rect.Min.X, p.Y-rect.Min.Y, col, width)
		last = p
	}
	return &ImageComponent{Img: img, Pos: rect.Min, Image: false}
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

// flatten merges all components of the tab into a single image and clears the list.
func flatten(t *Tab) *image.RGBA {
	union := t.Image.Bounds()
	for _, c := range t.Components {
		union = union.Union(c.Bounds())
	}
	out := image.NewRGBA(image.Rect(0, 0, union.Dx(), union.Dy()))
	shift := image.Point{-union.Min.X, -union.Min.Y}
	draw.Draw(out, t.Image.Bounds().Add(shift), t.Image, image.Point{}, draw.Src)
	for _, c := range t.Components {
		c.Draw(out, shift)
	}
	t.Components = nil
	t.Image = out
	t.Offset = t.Offset.Add(union.Min)
	return out
}

func captureScreenshot() (*image.RGBA, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := map[string]dbus.Variant{
		"interactive": dbus.MakeVariant(false),
	}
	var handle dbus.ObjectPath
	call := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts)
	if call.Err != nil {
		return nil, call.Err
	}
	if err := call.Store(&handle); err != nil {
		return nil, err
	}

	sigc := make(chan *dbus.Signal, 1)
	conn.Signal(sigc)
	rule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", handle)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return nil, err
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule)

	for sig := range sigc {
		if sig.Path == handle && sig.Name == "org.freedesktop.portal.Request.Response" {
			if len(sig.Body) >= 2 {
				res := sig.Body[1].(map[string]dbus.Variant)
				if uriVar, ok := res["uri"]; ok {
					uri := uriVar.Value().(string)
					path := strings.TrimPrefix(uri, "file://")
					f, err := os.Open(path)
					if err != nil {
						return nil, err
					}
					defer f.Close()
					img, err := png.Decode(f)
					if err != nil {
						return nil, err
					}
					rgba := image.NewRGBA(img.Bounds())
					draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
					return rgba, nil
				}
			}
			break
		}
	}
	return nil, fmt.Errorf("screenshot failed")
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
	selected        int
	preview         Component
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
	basePos := dst.Min
	comps := st.tabs[st.current].Components
	for _, c := range comps {
		if c.IsImage() {
			c.DrawScaled(b.RGBA(), basePos, zoom)
			if ctx.Err() != nil {
				return
			}
		}
	}
	for i, c := range comps {
		if !c.IsImage() {
			c.DrawScaled(b.RGBA(), basePos, zoom)
			if ctx.Err() != nil {
				return
			}
			if st.selected == i {
				if cc, ok := c.(*CircleComponent); ok {
					cx := int(float64(basePos.X+cc.Center.X) * zoom)
					cy := int(float64(basePos.Y+cc.Center.Y) * zoom)
					r := int(float64(cc.Radius+cc.Width/2+handleSize/2) * zoom)
					drawDashedCircle(b.RGBA(), cx, cy, r, 4, 1, color.White, color.Black)
				} else {
					br := c.Bounds().Add(basePos)
					r := image.Rect(
						int(float64(br.Min.X)*zoom),
						int(float64(br.Min.Y)*zoom),
						int(float64(br.Max.X)*zoom),
						int(float64(br.Max.Y)*zoom),
					)
					drawDashedRect(b.RGBA(), r, 4, 1, color.White, color.Black)
				}
				if hrProvider, ok := c.(interface{ HandleRects() []image.Rectangle }); ok {
					for _, hr := range hrProvider.HandleRects() {
						sr := image.Rect(
							int(float64(basePos.X+hr.Min.X)*zoom),
							int(float64(basePos.Y+hr.Min.Y)*zoom),
							int(float64(basePos.X+hr.Max.X)*zoom),
							int(float64(basePos.Y+hr.Max.Y)*zoom),
						)
						draw.Draw(b.RGBA(), sr, &image.Uniform{color.White}, image.Point{}, draw.Src)
						drawRect(b.RGBA(), sr, color.Black, 1)
						drawDashedRect(b.RGBA(), sr, 2, 1, color.RGBA{255, 0, 0, 255}, color.RGBA{0, 0, 255, 255})
					}
				}
			}
		}
	}
	if st.preview != nil {
		st.preview.DrawScaled(b.RGBA(), basePos, zoom)
	}
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

func main() {
	screenshot := flag.String("screenshot", "", "existing screenshot to annotate")
	output := flag.String("output", "annotated.png", "output file path")
	flag.Parse()

	var rgba *image.RGBA
	if *screenshot == "" {
		var err error
		rgba, err = captureScreenshot()
		if err != nil {
			log.Fatalf("capture screenshot: %v", err)
		}
	} else {
		f, err := os.Open(*screenshot)
		if err != nil {
			log.Fatalf("open screenshot: %v", err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			log.Fatalf("decode screenshot: %v", err)
		}
		rgba = image.NewRGBA(img.Bounds())
		draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	}

	// ensure the toolbar is wide enough for the program title
	d := &font.Drawer{Face: basicfont.Face7x13}
	titleWidth := d.MeasureString("ShineyShot").Ceil() + 8 // padding
	if titleWidth > toolbarWidth {
		toolbarWidth = titleWidth
	}

	driver.Main(func(s screen.Screen) {
		width := rgba.Bounds().Dx() + toolbarWidth
		height := rgba.Bounds().Dy() + tabHeight + bottomHeight
		w, err := s.NewWindow(&screen.NewWindowOptions{Width: width, Height: height})
		if err != nil {
			log.Fatalf("new window: %v", err)
		}
		defer w.Release()

		tabs := []Tab{{Image: rgba, Title: "1", Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: 2}}
		current := 0

		var active actionType
		var cropMode cropAction
		var moveStart image.Point
		var compStartRect image.Rectangle
		var compMode cropAction
		var selectedComp int
		var previewComp Component
		var drawPoints []image.Point
		var last image.Point
		var cropStart image.Point
		var cropStartRect image.Rectangle
		var cropRect image.Rectangle
		var message string
		var messageUntil time.Time
		var confirmDelete bool
		var textInputActive bool
		var textInput string
		var textPos image.Point
		tool := ToolView
		selectedComp = -1
		colorIdx := 2 // red
		numberIdx := 0
		var paintMu sync.Mutex
		var paintCancel context.CancelFunc
		var dropCount int
		var lastPaint paintState
		_ = lastPaint
		paintCh := make(chan paintState, 1)
		go func() {
			for st := range paintCh {
				ctx, cancel := context.WithCancel(context.Background())
				paintMu.Lock()
				paintCancel = cancel
				paintMu.Unlock()
				drawFrame(ctx, s, w, st)
				paintMu.Lock()
				paintCancel = nil
				if ctx.Err() == nil {
					lastPaint = st
					dropCount = 0
				}
				paintMu.Unlock()
			}
		}()

		col := palette[colorIdx]
		tabs[current].Zoom = fitZoom(rgba, width, height)

		toolButtons = []*CacheButton{
			{Button: &ToolButton{label: "V:View", tool: ToolView, atype: actionMove}},
			{Button: &ToolButton{label: "M:Mod", tool: ToolModify, atype: actionMove}},
			{Button: &ToolButton{label: "R:Crop", tool: ToolCrop, atype: actionCrop}},
			{Button: &ToolButton{label: "B:Draw", tool: ToolDraw, atype: actionDraw}},
			{Button: &ToolButton{label: "O:Circle", tool: ToolCircle, atype: actionDraw}},
			{Button: &ToolButton{label: "L:Line", tool: ToolLine, atype: actionDraw}},
			{Button: &ToolButton{label: "A:Arrow", tool: ToolArrow, atype: actionDraw}},
			{Button: &ToolButton{label: "X:Rect", tool: ToolRect, atype: actionDraw}},
			{Button: &ToolButton{label: "H:Num", tool: ToolNumber, atype: actionDraw}},
			{Button: &ToolButton{label: "T:Text", tool: ToolText, atype: actionNone}},
		}
		for _, cb := range toolButtons {
			tb := cb.Button.(*ToolButton)
			t := tb
			tb.onSelect = func() {
				tool = t.tool
				active = actionNone
			}
		}

		keyboardAction = map[KeyShortcut]string{}

		actions := map[string]func(){}

		register := func(name string, keys KeyboardShortcuts, fn func()) {
			actions[name] = fn
			if keys != nil {
				for _, sc := range keys.KeyboardShortcuts() {
					keyboardAction[sc] = name
				}
			}
		}

		handleShortcut := func(action string) {
			if fn, ok := actions[action]; ok {
				fn()
			}
			w.Send(paint.Event{})
		}

		register("capture", shortcutList{{Rune: 'n', Modifiers: key.ModControl}}, func() {
			img, err := captureScreenshot()
			if err != nil {
				log.Printf("capture screenshot: %v", err)
				return
			}
			tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: 2})
			current = len(tabs) - 1
			tabs[current].Zoom = fitZoom(tabs[current].Image, width, height)
			message = "captured screenshot"
			log.Print(message)
			messageUntil = time.Now().Add(2 * time.Second)
		})

		register("dup", shortcutList{{Rune: 'u', Modifiers: key.ModControl}}, func() {
			dup := image.NewRGBA(tabs[current].Image.Bounds())
			draw.Draw(dup, dup.Bounds(), tabs[current].Image, image.Point{}, draw.Src)
			tabs = append(tabs, Tab{Image: dup, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: tabs[current].Offset, Zoom: tabs[current].Zoom, NextNumber: tabs[current].NextNumber, WidthIdx: tabs[current].WidthIdx})
			current = len(tabs) - 1
		})

		register("paste", shortcutList{{Rune: 'v', Modifiers: key.ModControl}}, func() {
			out, err := exec.Command("wl-paste", "--no-newline", "--type", "image/png").Output()
			if err != nil {
				log.Printf("paste: %v", err)
				return
			}
			img, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				log.Printf("paste decode: %v", err)
				return
			}
			rgba := image.NewRGBA(img.Bounds())
			draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
			tabs[current].Components = append(tabs[current].Components, &ImageComponent{Img: rgba, Pos: image.Point{}, Image: true})
			message = "pasted"
			log.Print(message)
			messageUntil = time.Now().Add(2 * time.Second)
		})

		register("pasteNew", shortcutList{{Rune: 'v', Modifiers: key.ModControl | key.ModShift}}, func() {
			out, err := exec.Command("wl-paste", "--no-newline", "--type", "image/png").Output()
			if err != nil {
				log.Printf("paste: %v", err)
				return
			}
			img, err := png.Decode(bytes.NewReader(out))
			if err != nil {
				log.Printf("paste decode: %v", err)
				return
			}
			rgba := image.NewRGBA(img.Bounds())
			draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
			tabs = append(tabs, Tab{Image: rgba, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: 2})
			current = len(tabs) - 1
			message = "pasted new tab"
			log.Print(message)
			messageUntil = time.Now().Add(2 * time.Second)
		})

		register("delete", shortcutList{{Rune: 'd', Modifiers: key.ModControl}}, func() {
			if len(tabs) > 1 {
				tabs = append(tabs[:current], tabs[current+1:]...)
				if current >= len(tabs) {
					current = len(tabs) - 1
				}
			}
		})

		register("copy", shortcutList{{Rune: 'c', Modifiers: key.ModControl}}, func() {
			img := flatten(&tabs[current])
			var buf bytes.Buffer
			png.Encode(&buf, img)
			cmd := exec.Command("wl-copy", "--type", "image/png")
			cmd.Stdin = &buf
			if err := cmd.Run(); err != nil {
				log.Printf("copy: %v", err)
			} else {
				message = "image copied to clipboard"
				log.Print(message)
				messageUntil = time.Now().Add(2 * time.Second)
			}
		})

		register("save", shortcutList{{Rune: 's', Modifiers: key.ModControl}}, func() {
			out, err := os.Create(*output)
			if err != nil {
				log.Printf("save: %v", err)
				return
			}
			img := flatten(&tabs[current])
			png.Encode(out, img)
			out.Close()
			message = fmt.Sprintf("saved %s", *output)
			log.Print(message)
			messageUntil = time.Now().Add(2 * time.Second)
		})

		register("textdone", shortcutList{{Code: key.CodeReturnEnter}}, func() {
			d := &font.Drawer{Face: textFaces[textSizeIdx]}
			w := d.MeasureString(textInput).Ceil()
			metrics := textFaces[textSizeIdx].Metrics()
			h := metrics.Ascent.Ceil() + metrics.Descent.Ceil()
			img := image.NewRGBA(image.Rect(0, 0, w, h))
			d = &font.Drawer{Dst: img, Src: image.NewUniform(palette[colorIdx]), Face: textFaces[textSizeIdx]}
			d.Dot = fixed.P(0, metrics.Ascent.Ceil())
			d.DrawString(textInput)
			pos := image.Pt(textPos.X, textPos.Y-metrics.Ascent.Ceil())
			comp := &ImageComponent{Img: img, Pos: pos, Image: false}
			tabs[current].Components = append(tabs[current].Components, comp)
			textInputActive = false
		})

		register("textcancel", shortcutList{{Code: key.CodeEscape}}, func() {
			textInputActive = false
		})

		register("crop", shortcutList{{Code: key.CodeReturnEnter}, {Rune: '\r', Code: key.CodeReturnEnter}}, func() {
			if tool == ToolCrop && !cropRect.Empty() {
				cropped := cropImage(tabs[current].Image, cropRect)
				tabs[current].Image = cropped
				tabs[current].Offset = tabs[current].Offset.Add(cropRect.Min)
				active = actionNone
				cropRect = image.Rectangle{}
			}
		})

		register("croptab", shortcutList{{Code: key.CodeReturnEnter, Modifiers: key.ModControl}, {Rune: '\r', Code: key.CodeReturnEnter, Modifiers: key.ModControl}}, func() {
			if tool == ToolCrop && !cropRect.Empty() {
				cropped := cropImage(tabs[current].Image, cropRect)
				off := tabs[current].Offset.Add(cropRect.Min)
				tabs = append(tabs, Tab{Image: cropped, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: off, Zoom: tabs[current].Zoom, NextNumber: 1, WidthIdx: tabs[current].WidthIdx})
				current = len(tabs) - 1
				active = actionNone
				cropRect = image.Rectangle{}
			}
		})

		register("cropcancel", shortcutList{{Code: key.CodeEscape}}, func() {
			if tool == ToolCrop {
				cropRect = image.Rectangle{}
				active = actionNone
			}
		})

		for {
			e := w.NextEvent()
			switch e := e.(type) {
			case lifecycle.Event:
				if e.To == lifecycle.StageDead {
					paintMu.Lock()
					if paintCancel != nil {
						paintCancel()
					}
					paintMu.Unlock()
					return
				}
			case size.Event:
				width = e.WidthPx
				height = e.HeightPx
				w.Send(paint.Event{})
			case paint.Event:
				paintMu.Lock()
				if paintCancel != nil {
					if dropCount < frameDropThreshold {
						paintCancel()
						dropCount++
					}
				}
				paintMu.Unlock()
				st := paintState{
					width:           width,
					height:          height,
					tabs:            tabs,
					current:         current,
					tool:            tool,
					colorIdx:        colorIdx,
					numberIdx:       numberIdx,
					cropping:        active == actionCrop,
					cropRect:        cropRect,
					cropStart:       cropStart,
					textInputActive: textInputActive,
					textInput:       textInput,
					textPos:         textPos,
					message:         message,
					messageUntil:    messageUntil,
					handleShortcut:  handleShortcut,
					selected:        selectedComp,
					preview:         previewComp,
				}
				select {
				case paintCh <- st:
				default:
					<-paintCh
					paintCh <- st
				}
				lastPaint = st
			case mouse.Event:
				if message != "" && time.Now().Before(messageUntil) && e.Direction == mouse.DirPress {
					messageUntil = time.Time{}
					w.Send(paint.Event{})
					continue
				}
				if int(e.Y) >= height-bottomHeight {
					p := image.Point{int(e.X), int(e.Y)}
					hoverShortcut = -1
					for i, sc := range shortcutRects {
						if p.In(sc.rect) {
							hoverShortcut = i
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								sc.Activate()
							}
							break
						}
					}
					if e.Direction == mouse.DirNone {
						w.Send(paint.Event{})
					}
					continue
				}
				if int(e.Y) < tabHeight {
					hoverTab = -1
					p := image.Point{int(e.X), int(e.Y)}
					for i, tb := range tabButtons {
						if p.In(tb.rect) {
							hoverTab = i
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								current = i
								w.Send(paint.Event{})
							}
							break
						}
					}
					if e.Direction == mouse.DirNone {
						w.Send(paint.Event{})
					}
					continue
				}

				if int(e.X) < toolbarWidth && int(e.Y) >= tabHeight {
					pos := int(e.Y) - tabHeight
					idx := pos / 24
					if idx < len(toolButtons) {
						if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
							toolButtons[idx].Activate()
							w.Send(paint.Event{})
						}
						hoverTool = idx
						if e.Direction == mouse.DirNone {
							w.Send(paint.Event{})
						}
						continue
					}
					pos -= len(toolButtons) * 24
					pos -= 4
					paletteCols := toolbarWidth / 18
					rows := (len(palette) + paletteCols - 1) / paletteCols
					paletteHeight := rows * 18
					if pos >= 0 && pos < paletteHeight {
						colX := (int(e.X) - 4) / 18
						colY := pos / 18
						cidx := colY*paletteCols + colX
						if cidx >= 0 && cidx < len(palette) {
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								colorIdx = cidx
								col = palette[colorIdx]
							}
							hoverPalette = cidx
							if e.Direction == mouse.DirNone {
								w.Send(paint.Event{})
							}
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								w.Send(paint.Event{})
							}
							continue
						}
					}
					pos -= paletteHeight
					pos -= 4
					if (tool == ToolDraw || tool == ToolCircle || tool == ToolLine || tool == ToolArrow || tool == ToolRect) && pos >= 0 {
						widx := pos / 16
						if widx >= 0 && widx < len(widths) {
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								tabs[current].WidthIdx = widx
							}
							hoverWidth = widx
							if e.Direction == mouse.DirNone {
								w.Send(paint.Event{})
							}
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								w.Send(paint.Event{})
							}
							continue
						}
					} else if tool == ToolNumber && pos >= 0 {
						rem := pos
						for i, s := range numberSizes {
							h := numberBoxHeight(s)
							if rem < h {
								if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
									numberIdx = i
								}
								hoverNumber = i
								if e.Direction == mouse.DirNone {
									w.Send(paint.Event{})
								}
								if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
									w.Send(paint.Event{})
								}
								break
							}
							rem -= h
						}
						continue
					} else if tool == ToolText && pos >= 0 {
						idx := pos / 24
						if idx >= 0 && idx < len(textFaces) {
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								textSizeIdx = idx
							}
							hoverTextSize = idx
							if e.Direction == mouse.DirNone {
								w.Send(paint.Event{})
							}
							if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
								w.Send(paint.Event{})
							}
							continue
						}
					}
					if e.Direction == mouse.DirNone {
						hoverTool = -1
						hoverPalette = -1
						hoverWidth = -1
						hoverNumber = -1
						hoverTextSize = -1
						w.Send(paint.Event{})
					}
				}

				baseRect := imageRect(tabs[current].Image, width, height, tabs[current].Zoom)

				mx := int((float64(e.X)-float64(baseRect.Min.X))/tabs[current].Zoom) - tabs[current].Offset.X
				my := int((float64(e.Y)-float64(baseRect.Min.Y))/tabs[current].Zoom) - tabs[current].Offset.Y
				if e.Button == mouse.ButtonLeft || e.Button == mouse.ButtonRight {
					if e.Direction == mouse.DirPress {
						act := actionOfTool(tool)
						switch tool {
						case ToolView:
							active = act
							moveStart = image.Point{int(e.X), int(e.Y)}
						case ToolModify:
							active = act
							moveStart = image.Point{int(e.X), int(e.Y)}
							selectedComp = -1
							p := image.Point{mx, my}
							for i := len(tabs[current].Components) - 1; i >= 0; i-- {
								c := tabs[current].Components[i]
								if c.Contains(p) {
									selectedComp = i
									break
								}
							}
							if selectedComp >= 0 {
								comp := tabs[current].Components[selectedComp]
								tabs[current].Components = append(tabs[current].Components[:selectedComp], tabs[current].Components[selectedComp+1:]...)
								if e.Button == mouse.ButtonRight {
									tabs[current].Components = append([]Component{comp}, tabs[current].Components...)
									selectedComp = 0
								} else {
									tabs[current].Components = append(tabs[current].Components, comp)
									selectedComp = len(tabs[current].Components) - 1
								}
								if hrProvider, ok := comp.(interface{ HandleRects() []image.Rectangle }); ok {
									for i, hr := range hrProvider.HandleRects() {
										if p.In(hr) {
											active = actionCrop
											compMode = cropAction(i + int(cropResizeTL))
											compStartRect = comp.Bounds()
											if lc, ok := comp.(*LineComponent); ok {
												compStartA = lc.Start
												compStartB = lc.End
											}
											if ac, ok := comp.(*ArrowComponent); ok {
												compStartA = ac.Start
												compStartB = ac.End
											}
											moveStart = p
											break
										}
									}
								}
							}
						case ToolCrop:
							p := image.Point{mx, my}
							action := cropNone
							for i, hr := range cropHandleRects(cropRect) {
								if p.In(hr) {
									action = cropAction(i + int(cropResizeTL))
									break
								}
							}
							if action == cropNone {
								if !cropRect.Empty() && p.In(cropRect) {
									action = cropMove
								} else {
									action = cropResizeBR
									cropRect = image.Rect(mx, my, mx, my)
								}
							}
							active = act
							cropMode = action
							cropStart = p
							cropStartRect = cropRect
							w.Send(paint.Event{})
						case ToolDraw:
							active = act
							drawPoints = []image.Point{{mx, my}}
							last = image.Point{mx, my}
						case ToolCircle, ToolLine, ToolArrow, ToolRect, ToolNumber:
							active = act
							last = image.Point{mx, my}
						case ToolText:
							if textInputActive {
								textPos = image.Point{mx, my}
							} else {
								textInputActive = true
								textInput = ""
								textPos = image.Point{mx, my}
							}
							w.Send(paint.Event{})
						}
					} else if e.Direction == mouse.DirRelease {
						if active == actionCrop && tool == ToolCrop {
							dx := mx - cropStart.X
							dy := my - cropStart.Y
							r := cropStartRect
							switch cropMode {
							case cropMove:
								r = r.Add(image.Pt(dx, dy))
							case cropResizeTL:
								r.Min.X = cropStartRect.Min.X + dx
								r.Min.Y = cropStartRect.Min.Y + dy
							case cropResizeT:
								r.Min.Y = cropStartRect.Min.Y + dy
							case cropResizeTR:
								r.Min.Y = cropStartRect.Min.Y + dy
								r.Max.X = cropStartRect.Max.X + dx
							case cropResizeR:
								r.Max.X = cropStartRect.Max.X + dx
							case cropResizeBR:
								r.Max.X = cropStartRect.Max.X + dx
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeB:
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeBL:
								r.Min.X = cropStartRect.Min.X + dx
								r.Max.Y = cropStartRect.Max.Y + dy
							case cropResizeL:
								r.Min.X = cropStartRect.Min.X + dx
							}
							if r.Min.X > r.Max.X {
								r.Min.X, r.Max.X = r.Max.X, r.Min.X
							}
							if r.Min.Y > r.Max.Y {
								r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
							}
							cropRect = r
						}
						if active == actionCrop && tool == ToolModify && selectedComp >= 0 {
							dx := mx - moveStart.X
							dy := my - moveStart.Y
							if rc, ok := tabs[current].Components[selectedComp].(interface {
								Resize(cropAction, image.Rectangle, int, int)
							}); ok {
								rc.Resize(compMode, compStartRect, dx, dy)
							}
						}
						if active == actionDraw && tool != ToolCrop {
							var comp Component
							switch tool {
							case ToolDraw:
								drawPoints = append(drawPoints, image.Pt(mx, my))
								comp = freehandComponent(drawPoints, col, widths[tabs[current].WidthIdx])
							case ToolLine:
								comp = &LineComponent{Start: last, End: image.Pt(mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
							case ToolCircle:
								r := int(math.Hypot(float64(mx-last.X), float64(my-last.Y)))
								comp = &CircleComponent{Center: last, Radius: r, Col: col, Width: widths[tabs[current].WidthIdx]}
							case ToolArrow:
								comp = &ArrowComponent{Start: last, End: image.Pt(mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
							case ToolRect:
								comp = &RectComponent{Rect: image.Rect(last.X, last.Y, mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
							case ToolNumber:
								s := numberSizes[numberIdx]
								comp = &NumberComponent{Pos: image.Pt(mx, my), Num: tabs[current].NextNumber, Size: s, Col: col}
								tabs[current].NextNumber++
							}
							if comp != nil {
								tabs[current].Components = append(tabs[current].Components, comp)
							}
							previewComp = nil
							drawPoints = nil
							w.Send(paint.Event{})
						}
						if active == actionMove && tool == ToolView {
							dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
							dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
							tabs[current].Offset = tabs[current].Offset.Add(image.Pt(dx, dy))
							moveStart = image.Point{int(e.X), int(e.Y)}
							w.Send(paint.Event{})
						}
						if active == actionMove && tool == ToolModify && selectedComp >= 0 {
							dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
							dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
							tabs[current].Components[selectedComp].MoveBy(dx, dy)
							moveStart = image.Point{int(e.X), int(e.Y)}
							w.Send(paint.Event{})
						}
						active = actionNone
					}
				}

				if active == actionCrop && tool == ToolCrop && e.Direction == mouse.DirNone {
					dx := mx - cropStart.X
					dy := my - cropStart.Y
					r := cropStartRect
					switch cropMode {
					case cropMove:
						r = r.Add(image.Pt(dx, dy))
					case cropResizeTL:
						r.Min.X = cropStartRect.Min.X + dx
						r.Min.Y = cropStartRect.Min.Y + dy
					case cropResizeT:
						r.Min.Y = cropStartRect.Min.Y + dy
					case cropResizeTR:
						r.Min.Y = cropStartRect.Min.Y + dy
						r.Max.X = cropStartRect.Max.X + dx
					case cropResizeR:
						r.Max.X = cropStartRect.Max.X + dx
					case cropResizeBR:
						r.Max.X = cropStartRect.Max.X + dx
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeB:
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeBL:
						r.Min.X = cropStartRect.Min.X + dx
						r.Max.Y = cropStartRect.Max.Y + dy
					case cropResizeL:
						r.Min.X = cropStartRect.Min.X + dx
					}
					if r.Min.X > r.Max.X {
						r.Min.X, r.Max.X = r.Max.X, r.Min.X
					}
					if r.Min.Y > r.Max.Y {
						r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
					}
					cropRect = r
					w.Send(paint.Event{})
				}

				if active == actionCrop && tool == ToolModify && e.Direction == mouse.DirNone && selectedComp >= 0 {
					dx := mx - moveStart.X
					dy := my - moveStart.Y
					if rc, ok := tabs[current].Components[selectedComp].(interface {
						Resize(cropAction, image.Rectangle, int, int)
					}); ok {
						rc.Resize(compMode, compStartRect, dx, dy)
					}
					w.Send(paint.Event{})
				}

				if active == actionDraw && tool != ToolCrop && e.Direction == mouse.DirNone {
					var pc Component
					switch tool {
					case ToolDraw:
						drawPoints = append(drawPoints, image.Pt(mx, my))
						pc = freehandComponent(drawPoints, col, widths[tabs[current].WidthIdx])
					case ToolLine:
						pc = &LineComponent{Start: last, End: image.Pt(mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
					case ToolCircle:
						r := int(math.Hypot(float64(mx-last.X), float64(my-last.Y)))
						pc = &CircleComponent{Center: last, Radius: r, Col: col, Width: widths[tabs[current].WidthIdx]}
					case ToolArrow:
						pc = &ArrowComponent{Start: last, End: image.Pt(mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
					case ToolRect:
						pc = &RectComponent{Rect: image.Rect(last.X, last.Y, mx, my), Col: col, Width: widths[tabs[current].WidthIdx]}
					case ToolNumber:
						pc = &NumberComponent{Pos: image.Pt(mx, my), Num: tabs[current].NextNumber, Size: numberSizes[numberIdx], Col: col}
					}
					previewComp = pc
					w.Send(paint.Event{})
				}
				if active == actionMove && tool == ToolView && e.Direction == mouse.DirNone {
					dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
					dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
					tabs[current].Offset = tabs[current].Offset.Add(image.Pt(dx, dy))
					moveStart = image.Point{int(e.X), int(e.Y)}
					w.Send(paint.Event{})
				}
				if active == actionMove && tool == ToolModify && e.Direction == mouse.DirNone && selectedComp >= 0 {
					dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
					dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
					tabs[current].Components[selectedComp].MoveBy(dx, dy)
					moveStart = image.Point{int(e.X), int(e.Y)}
					w.Send(paint.Event{})
				}
			case key.Event:
				if e.Direction == key.DirPress {
					if textInputActive {
						switch e.Code {
						case key.CodeReturnEnter:
							d := &font.Drawer{Face: textFaces[textSizeIdx]}
							width := d.MeasureString(textInput).Ceil()
							metrics := textFaces[textSizeIdx].Metrics()
							h := metrics.Ascent.Ceil() + metrics.Descent.Ceil()
							img := image.NewRGBA(image.Rect(0, 0, width, h))
							d = &font.Drawer{Dst: img, Src: image.NewUniform(palette[colorIdx]), Face: textFaces[textSizeIdx]}
							d.Dot = fixed.P(0, metrics.Ascent.Ceil())
							d.DrawString(textInput)
							pos := image.Pt(textPos.X, textPos.Y-metrics.Ascent.Ceil())
							comp := &ImageComponent{Img: img, Pos: pos, Image: false}
							tabs[current].Components = append(tabs[current].Components, comp)
							textInputActive = false
							w.Send(paint.Event{})
							continue
						case key.CodeEscape:
							textInputActive = false
							w.Send(paint.Event{})
							continue
						case key.CodeDeleteBackspace:
							if len(textInput) > 0 {
								textInput = textInput[:len(textInput)-1]
								w.Send(paint.Event{})
							}
							continue
						}
						if e.Rune > 0 {
							textInput += string(e.Rune)
							w.Send(paint.Event{})
						}
						continue
					}
					confirmDelete = false
					ks := KeyShortcut{Rune: unicode.ToLower(e.Rune), Code: e.Code, Modifiers: e.Modifiers}
					if action, ok := keyboardAction[ks]; ok {
						if action == "delete" {
							if !confirmDelete {
								confirmDelete = true
								message = "press D again to delete"
								log.Print(message)
								messageUntil = time.Now().Add(2 * time.Second)
								w.Send(paint.Event{})
								continue
							}
							confirmDelete = false
						}
						handleShortcut(action)
						continue
					}
					switch e.Rune {
					case 'm', 'M':
						tool = ToolModify
						active = actionNone
						w.Send(paint.Event{})
					case 'v', 'V':
						tool = ToolView
						active = actionNone
						w.Send(paint.Event{})
					case 'e', 'E':
						if tool == ToolModify && selectedComp >= 0 {
							tabs[current].Components = append(tabs[current].Components[:selectedComp], tabs[current].Components[selectedComp+1:]...)
							selectedComp = -1
							w.Send(paint.Event{})
						}
					case 'r', 'R':
						tool = ToolCrop
						active = actionNone
						w.Send(paint.Event{})
					case 'b', 'B':
						tool = ToolDraw
						active = actionNone
						w.Send(paint.Event{})
					case 'o', 'O':
						tool = ToolCircle
						active = actionNone
						w.Send(paint.Event{})
					case 'l', 'L':
						tool = ToolLine
						active = actionNone
						w.Send(paint.Event{})
					case 'a', 'A':
						tool = ToolArrow
						active = actionNone
						w.Send(paint.Event{})
					case 'x', 'X':
						tool = ToolRect
						active = actionNone
						w.Send(paint.Event{})
					case 't', 'T':
						tool = ToolText
						active = actionNone
						w.Send(paint.Event{})
					case 'h', 'H':
						tool = ToolNumber
						active = actionNone
						w.Send(paint.Event{})
					case '1', '2', '3', '4', '5', '6', '7', '8', '9':
						if e.Modifiers&key.ModControl != 0 {
							idx := int(e.Rune - '1')
							if idx >= 0 && idx < len(tabs) {
								current = idx
								w.Send(paint.Event{})
							}
						}
					case 'q', 'Q':
						paintMu.Lock()
						if paintCancel != nil {
							paintCancel()
						}
						paintMu.Unlock()
						return
					case '+', '=':
						tabs[current].Zoom *= 1.25
						if tabs[current].Zoom < 0.1 {
							tabs[current].Zoom = 0.1
						}
						w.Send(paint.Event{})
					case '-':
						tabs[current].Zoom /= 1.25
						if tabs[current].Zoom < 0.1 {
							tabs[current].Zoom = 0.1
						}
						w.Send(paint.Event{})
					case -1:
						switch e.Code {
						case key.CodeLeftArrow:
							if tool == ToolView {
								tabs[current].Offset.X -= 10
								w.Send(paint.Event{})
							}
							if tool == ToolModify && selectedComp >= 0 {
								tabs[current].Components[selectedComp].MoveBy(-10, 0)
								w.Send(paint.Event{})
							}
						case key.CodeRightArrow:
							if tool == ToolView {
								tabs[current].Offset.X += 10
								w.Send(paint.Event{})
							}
							if tool == ToolModify && selectedComp >= 0 {
								tabs[current].Components[selectedComp].MoveBy(10, 0)
								w.Send(paint.Event{})
							}
						case key.CodeUpArrow:
							if tool == ToolView {
								tabs[current].Offset.Y -= 10
								w.Send(paint.Event{})
							}
							if tool == ToolModify && selectedComp >= 0 {
								tabs[current].Components[selectedComp].MoveBy(0, -10)
								w.Send(paint.Event{})
							}
						case key.CodeDownArrow:
							if tool == ToolView {
								tabs[current].Offset.Y += 10
								w.Send(paint.Event{})
							}
							if tool == ToolModify && selectedComp >= 0 {
								tabs[current].Components[selectedComp].MoveBy(0, 10)
								w.Send(paint.Event{})
							}
						}
					}
				}
			}
		}
	})
}
