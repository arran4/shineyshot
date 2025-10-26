package appstate

import (
	"context"
	"fmt"
	"github.com/example/shineyshot/internal/capture"
	"github.com/example/shineyshot/internal/clipboard"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
	"image"
	"image/draw"
	"image/png"
	"log"
	"math"
	"os"
	"sync"
	"time"
	"unicode"

	"golang.org/x/exp/shiny/driver"
	"golang.org/x/exp/shiny/screen"
	"golang.org/x/mobile/event/key"
	"golang.org/x/mobile/event/lifecycle"
	"golang.org/x/mobile/event/mouse"
	"golang.org/x/mobile/event/paint"
	"golang.org/x/mobile/event/size"
)

// AppState holds application configuration for the UI.
type AppState struct {
	Image    *image.RGBA
	Output   string
	ColorIdx int
	WidthIdx int
	Mode     Mode

	updateCh    chan struct{}
	sendControl func(controlEvent)

	settingsMu sync.Mutex
	settingsFn func(colorIdx, widthIdx int)

	onClose   func()
	closeOnce sync.Once
}

// Option modifies an AppState during creation.
type Option func(*AppState)

// WithImage sets the image displayed by the application.
func WithImage(img *image.RGBA) Option { return func(a *AppState) { a.Image = img } }

// WithOutput sets the output file path used when saving annotations.
func WithOutput(out string) Option { return func(a *AppState) { a.Output = out } }

// WithColorIndex sets the initial palette index for drawing tools.
func WithColorIndex(idx int) Option { return func(a *AppState) { a.ColorIdx = idx } }

// WithWidthIndex sets the initial stroke width index for drawing tools.
func WithWidthIndex(idx int) Option { return func(a *AppState) { a.WidthIdx = idx } }

// WithMode configures the UI mode for the state machine.
func WithMode(mode Mode) Option { return func(a *AppState) { a.Mode = mode } }

// WithSettingsListener registers a callback for when drawing settings change.
func WithSettingsListener(fn func(colorIdx, widthIdx int)) Option {
	return func(a *AppState) { a.settingsFn = fn }
}

// WithOnClose registers a callback invoked when the window closes.
func WithOnClose(fn func()) Option { return func(a *AppState) { a.onClose = fn } }

// New creates an AppState with the provided options.
func New(opts ...Option) *AppState {
	a := &AppState{
		ColorIdx: defaultColorIndex,
		WidthIdx: defaultWidthIndex,
		Mode:     ModeAnnotate,
		updateCh: make(chan struct{}, 1),
	}
	for _, o := range opts {
		o(a)
	}
	a.ColorIdx = clampColorIndex(a.ColorIdx)
	a.WidthIdx = clampWidthIndex(a.WidthIdx)
	return a
}

type controlEvent struct {
	ColorIdx *int
	WidthIdx *int
}

// NotifyImageChanged requests a repaint of the UI when the image mutates.
func (a *AppState) NotifyImageChanged() {
	if a.updateCh == nil {
		return
	}
	select {
	case a.updateCh <- struct{}{}:
	default:
	}
}

// ApplySettings synchronizes drawing settings between the CLI and UI.
func (a *AppState) ApplySettings(colorIdx, widthIdx int) {
	colorIdx = clampColorIndex(colorIdx)
	widthIdx = clampWidthIndex(widthIdx)

	a.settingsMu.Lock()
	a.ColorIdx = colorIdx
	a.WidthIdx = widthIdx
	fn := a.settingsFn
	sender := a.sendControl
	a.settingsMu.Unlock()

	if sender != nil {
		ci := colorIdx
		wi := widthIdx
		sender(controlEvent{ColorIdx: &ci, WidthIdx: &wi})
	}
	if fn != nil {
		fn(colorIdx, widthIdx)
	}
}

func (a *AppState) applySettingsFromUI(colorIdx, widthIdx int) {
	colorIdx = clampColorIndex(colorIdx)
	widthIdx = clampWidthIndex(widthIdx)

	a.settingsMu.Lock()
	a.ColorIdx = colorIdx
	a.WidthIdx = widthIdx
	fn := a.settingsFn
	a.settingsMu.Unlock()

	if fn != nil {
		fn(colorIdx, widthIdx)
	}
}

func (a *AppState) setControlSender(fn func(controlEvent)) {
	a.settingsMu.Lock()
	a.sendControl = fn
	a.settingsMu.Unlock()
}

func (a *AppState) notifyClose() {
	a.closeOnce.Do(func() {
		a.setControlSender(nil)
		if a.onClose != nil {
			a.onClose()
		}
	})
}

// Run executes the UI loop using shiny's driver.
func (a *AppState) Run() { driver.Main(a.Main) }

func (a *AppState) Main(s screen.Screen) {
	rgba := a.Image
	output := a.Output
	colorIdx := clampColorIndex(a.ColorIdx)
	widthIdx := clampWidthIndex(a.WidthIdx)

	// Ensure the toolbar is wide enough to fit the program title and all
	// tool button labels so the UI contents are not clipped on start up.
	d := &font.Drawer{Face: basicfont.Face7x13}
	max := d.MeasureString("ShineyShot").Ceil() + 8 // padding
	toolLabels := []string{"M:Move", "R:Crop", "B:Draw", "O:Circle", "L:Line", "A:Arrow", "X:Rect", "H:Num", "T:Text"}
	for _, lbl := range toolLabels {
		w := d.MeasureString(lbl).Ceil() + 8
		if w > max {
			max = w
		}
	}
	if max > toolbarWidth {
		toolbarWidth = max
	}

	width := rgba.Bounds().Dx() + toolbarWidth
	height := rgba.Bounds().Dy() + tabHeight + bottomHeight
	w, err := s.NewWindow(&screen.NewWindowOptions{Width: width, Height: height})
	if err != nil {
		log.Fatalf("new window: %v", err)
	}
	defer w.Release()

	defer a.notifyClose()

	if a.updateCh != nil {
		done := make(chan struct{})
		go func() {
			for {
				select {
				case <-a.updateCh:
					w.Send(paint.Event{})
				case <-done:
					return
				}
			}
		}()
		defer close(done)
	}

	a.setControlSender(func(ev controlEvent) { w.Send(ev) })

	tabs := []Tab{{Image: rgba, Title: "1", Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: widthIdx}}
	current := 0

	var active actionType
	var cropMode cropAction
	var moveStart image.Point
	var moveOffset image.Point
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
	tool := ToolMove
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

	col := paletteColorAt(colorIdx)
	tabs[current].Zoom = fitZoom(rgba, width, height)
	a.applySettingsFromUI(colorIdx, tabs[current].WidthIdx)

	annotationEnabled := a.Mode != ModePreview

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

	var configureMode func()

	configureMode = func() {
		actions = map[string]func(){}
		keyboardAction = map[KeyShortcut]string{}
		hoverTool = -1
		hoverPalette = -1
		hoverWidth = -1
		hoverNumber = -1
		hoverTextSize = -1

		registerCopy := func() {
			register("copy", shortcutList{{Rune: 'c', Modifiers: key.ModControl}}, func() {
				if err := clipboard.WriteImage(tabs[current].Image); err != nil {
					log.Printf("copy: %v", err)
					return
				}
				message = "image copied to clipboard"
				log.Print(message)
				messageUntil = time.Now().Add(2 * time.Second)
			})
		}

		registerSave := func() {
			register("save", shortcutList{{Rune: 's', Modifiers: key.ModControl}}, func() {
				out, err := os.Create(output)
				if err != nil {
					log.Printf("save: %v", err)
					return
				}
				if err := png.Encode(out, tabs[current].Image); err != nil {
					log.Printf("save: %v", err)
					if cerr := out.Close(); cerr != nil {
						log.Printf("save: closing file: %v", cerr)
					}
					return
				}
				if err := out.Close(); err != nil {
					log.Printf("save: closing file: %v", err)
					return
				}
				message = fmt.Sprintf("saved %s", output)
				log.Print(message)
				messageUntil = time.Now().Add(2 * time.Second)
			})
		}

		registerCommonActions := func() {
			registerCopy()
			registerSave()
		}

		if !annotationEnabled {
			toolButtons = []*CacheButton{
				{Button: &ActionButton{label: "Annotate", onActivate: func() {
					if annotationEnabled {
						return
					}
					annotationEnabled = true
					tool = ToolMove
					active = actionNone
					configureMode()
					w.Send(paint.Event{})
				}}},
			}
			registerCommonActions()
			register("annotate", shortcutList{{Rune: 'a'}}, func() {
				if annotationEnabled {
					return
				}
				annotationEnabled = true
				tool = ToolMove
				active = actionNone
				configureMode()
				w.Send(paint.Event{})
			})
			return
		}

		toolButtons = []*CacheButton{
			{Button: &ToolButton{label: "M:Move", tool: ToolMove, atype: actionMove}},
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
			tb, ok := cb.Button.(*ToolButton)
			if !ok {
				continue
			}
			t := tb
			tb.onSelect = func() {
				tool = t.tool
				active = actionNone
			}
		}

		registerCommonActions()

		register("capture", shortcutList{{Rune: 'n', Modifiers: key.ModControl}}, func() {
			img, err := capture.CaptureScreenshot("")
			if err != nil {
				log.Printf("capture screenshot: %v", err)
				return
			}
			tabs = append(tabs, Tab{Image: img, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: a.WidthIdx})
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
			img, err := clipboard.ReadImage()
			if err != nil {
				log.Printf("paste: %v", err)
				return
			}
			rgba := image.NewRGBA(img.Bounds())
			draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
			tabs = append(tabs, Tab{Image: rgba, Title: fmt.Sprintf("%d", len(tabs)+1), Offset: image.Point{}, Zoom: 1, NextNumber: 1, WidthIdx: a.WidthIdx})
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

		register("textdone", shortcutList{{Code: key.CodeReturnEnter}}, func() {
			d := &font.Drawer{Dst: tabs[current].Image, Src: image.NewUniform(paletteColorAt(colorIdx)), Face: textFaces[textSizeIdx]}
			d.Dot = fixed.P(textPos.X, textPos.Y)
			d.DrawString(textInput)
			textInputActive = false
		})

		register("textcancel", shortcutList{{Code: key.CodeEscape}}, func() {
			textInputActive = false
		})

		register("crop", shortcutList{{Code: key.CodeReturnEnter}}, func() {
			if tool == ToolCrop && !cropRect.Empty() {
				cropped := cropImage(tabs[current].Image, cropRect)
				tabs[current].Image = cropped
				tabs[current].Offset = tabs[current].Offset.Add(cropRect.Min)
				active = actionNone
				cropRect = image.Rectangle{}
			}
		})

		register("croptab", shortcutList{{Code: key.CodeReturnEnter, Modifiers: key.ModControl}}, func() {
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

	}

	handleShortcut := func(action string) {
		if fn, ok := actions[action]; ok {
			fn()
		}
		w.Send(paint.Event{})
	}

	configureMode()

	for {
		e := w.NextEvent()
		switch e := e.(type) {
		case controlEvent:
			if e.ColorIdx != nil {
				colorIdx = clampColorIndex(*e.ColorIdx)
				col = paletteColorAt(colorIdx)
			}
			if e.WidthIdx != nil {
				tabs[current].WidthIdx = clampWidthIndex(*e.WidthIdx)
			}
			a.applySettingsFromUI(colorIdx, tabs[current].WidthIdx)
			w.Send(paint.Event{})
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
				width:             width,
				height:            height,
				tabs:              tabs,
				current:           current,
				tool:              tool,
				colorIdx:          colorIdx,
				numberIdx:         numberIdx,
				cropping:          active == actionCrop,
				cropRect:          cropRect,
				cropStart:         cropStart,
				textInputActive:   textInputActive,
				textInput:         textInput,
				textPos:           textPos,
				message:           message,
				messageUntil:      messageUntil,
				handleShortcut:    handleShortcut,
				annotationEnabled: annotationEnabled,
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
							a.applySettingsFromUI(colorIdx, tabs[current].WidthIdx)
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
				if !annotationEnabled {
					hoverTool = -1
					hoverPalette = -1
					hoverWidth = -1
					hoverNumber = -1
					hoverTextSize = -1
					if e.Direction == mouse.DirNone {
						w.Send(paint.Event{})
					}
					continue
				}
				pos -= len(toolButtons) * 24
				pos -= 4
				paletteCols := toolbarWidth / 18
				rows := (paletteLen() + paletteCols - 1) / paletteCols
				paletteHeight := rows * 18
				if pos >= 0 && pos < paletteHeight {
					colX := (int(e.X) - 4) / 18
					colY := pos / 18
					cidx := colY*paletteCols + colX
					if cidx >= 0 && cidx < paletteLen() {
						if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
							colorIdx = cidx
							col = paletteColorAt(colorIdx)
							a.applySettingsFromUI(colorIdx, tabs[current].WidthIdx)
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
					if widx >= 0 && widx < widthsLen() {
						if e.Button == mouse.ButtonLeft && e.Direction == mouse.DirPress {
							tabs[current].WidthIdx = widx
							a.applySettingsFromUI(colorIdx, tabs[current].WidthIdx)
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
			if e.Button == mouse.ButtonLeft {
				if !annotationEnabled && tool != ToolMove {
					continue
				}
				if e.Direction == mouse.DirPress {
					act := actionOfTool(tool)
					switch tool {
					case ToolMove:
						active = act
						moveStart = image.Point{int(e.X), int(e.Y)}
						moveOffset = tabs[current].Offset
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
					if !annotationEnabled {
						active = actionNone
						continue
					}
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
					if annotationEnabled && active == actionDraw && tool != ToolCrop {
						switch tool {
						case ToolDraw:
							minX, minY := last.X, last.Y
							maxX, maxY := mx, my
							if mx < minX {
								minX = mx
							}
							if my < minY {
								minY = my
							}
							if last.X > maxX {
								maxX = last.X
							}
							if last.Y > maxY {
								maxY = last.Y
							}
							br := image.Rect(minX, minY, maxX, maxY).Inset(-widthAt(tabs[current].WidthIdx) - 2)
							shift := ensureCanvasContains(&tabs[current], br)
							last = last.Sub(shift)
							mx -= shift.X
							my -= shift.Y
							drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, widthAt(tabs[current].WidthIdx))
						case ToolCircle:
							rx := int(math.Abs(float64(mx - last.X)))
							ry := int(math.Abs(float64(my - last.Y)))
							br := image.Rect(last.X-rx-widthAt(tabs[current].WidthIdx), last.Y-ry-widthAt(tabs[current].WidthIdx), last.X+rx+widthAt(tabs[current].WidthIdx)+1, last.Y+ry+widthAt(tabs[current].WidthIdx)+1)
							shift := ensureCanvasContains(&tabs[current], br)
							last = last.Sub(shift)
							mx -= shift.X
							my -= shift.Y
							drawEllipse(tabs[current].Image, last.X, last.Y, rx, ry, col, widthAt(tabs[current].WidthIdx))
						case ToolLine:
							minX, minY := last.X, last.Y
							maxX, maxY := mx, my
							if mx < minX {
								minX = mx
							}
							if my < minY {
								minY = my
							}
							if last.X > maxX {
								maxX = last.X
							}
							if last.Y > maxY {
								maxY = last.Y
							}
							br := image.Rect(minX, minY, maxX, maxY).Inset(-widthAt(tabs[current].WidthIdx) - 2)
							shift := ensureCanvasContains(&tabs[current], br)
							last = last.Sub(shift)
							mx -= shift.X
							my -= shift.Y
							drawLine(tabs[current].Image, last.X, last.Y, mx, my, col, widthAt(tabs[current].WidthIdx))
						case ToolArrow:
							minX, minY := last.X, last.Y
							maxX, maxY := mx, my
							if mx < minX {
								minX = mx
							}
							if my < minY {
								minY = my
							}
							if last.X > maxX {
								maxX = last.X
							}
							if last.Y > maxY {
								maxY = last.Y
							}
							br := image.Rect(minX, minY, maxX, maxY).Inset(-widthAt(tabs[current].WidthIdx) - 10)
							shift := ensureCanvasContains(&tabs[current], br)
							last = last.Sub(shift)
							mx -= shift.X
							my -= shift.Y
							drawArrow(tabs[current].Image, last.X, last.Y, mx, my, col, widthAt(tabs[current].WidthIdx))
						case ToolRect:
							minX, minY := last.X, last.Y
							maxX, maxY := mx, my
							if mx < minX {
								minX = mx
							}
							if my < minY {
								minY = my
							}
							if last.X > maxX {
								maxX = last.X
							}
							if last.Y > maxY {
								maxY = last.Y
							}
							br := image.Rect(minX, minY, maxX, maxY).Inset(-widthAt(tabs[current].WidthIdx) - 2)
							shift := ensureCanvasContains(&tabs[current], br)
							last = last.Sub(shift)
							mx -= shift.X
							my -= shift.Y
							drawRect(tabs[current].Image, image.Rect(last.X, last.Y, mx, my), col, widthAt(tabs[current].WidthIdx))
						case ToolNumber:
							s := numberSizes[numberIdx]
							br := image.Rect(mx-s, my-s, mx+s, my+s)
							shift := ensureCanvasContains(&tabs[current], br)
							mx -= shift.X
							my -= shift.Y
							drawNumberBox(tabs[current].Image, mx, my, tabs[current].NextNumber, col, s)
							tabs[current].NextNumber++
						}
						w.Send(paint.Event{})
					}
					if active == actionMove && tool == ToolMove {
						dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
						dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
						tabs[current].Offset = moveOffset.Add(image.Pt(dx, dy))
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

			if annotationEnabled && active == actionDraw && tool == ToolDraw && e.Direction == mouse.DirNone {
				p := image.Point{mx, my}
				minX, minY := last.X, last.Y
				maxX, maxY := p.X, p.Y
				if p.X < minX {
					minX = p.X
				}
				if p.Y < minY {
					minY = p.Y
				}
				if last.X > maxX {
					maxX = last.X
				}
				if last.Y > maxY {
					maxY = last.Y
				}
				br := image.Rect(minX, minY, maxX, maxY).Inset(-widthAt(tabs[current].WidthIdx) - 2)
				shift := ensureCanvasContains(&tabs[current], br)
				last = last.Sub(shift)
				p = p.Sub(shift)
				drawLine(tabs[current].Image, last.X, last.Y, p.X, p.Y, col, widthAt(tabs[current].WidthIdx))
				last = p
				w.Send(paint.Event{})
			}
			if active == actionMove && tool == ToolMove && e.Direction == mouse.DirNone {
				dx := int(float64(int(e.X)-moveStart.X) / tabs[current].Zoom)
				dy := int(float64(int(e.Y)-moveStart.Y) / tabs[current].Zoom)
				tabs[current].Offset = moveOffset.Add(image.Pt(dx, dy))
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
						br := image.Rect(textPos.X, textPos.Y-metrics.Ascent.Ceil(), textPos.X+width, textPos.Y+metrics.Descent.Ceil())
						shift := ensureCanvasContains(&tabs[current], br)
						textPos = textPos.Sub(shift)
						d = &font.Drawer{Dst: tabs[current].Image, Src: image.NewUniform(paletteColorAt(colorIdx)), Face: textFaces[textSizeIdx]}
						d.Dot = fixed.P(textPos.X, textPos.Y)
						d.DrawString(textInput)
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
						handleShortcut(action)
						continue
					}
					confirmDelete = false
					handleShortcut(action)
					continue
				}
				confirmDelete = false
				switch e.Rune {
				case 'm', 'M':
					tool = ToolMove
					active = actionNone
					w.Send(paint.Event{})
				case 'r', 'R':
					if !annotationEnabled {
						continue
					}
					tool = ToolCrop
					active = actionNone
					w.Send(paint.Event{})
				case 'b', 'B':
					if !annotationEnabled {
						continue
					}
					tool = ToolDraw
					active = actionNone
					w.Send(paint.Event{})
				case 'o', 'O':
					if !annotationEnabled {
						continue
					}
					tool = ToolCircle
					active = actionNone
					w.Send(paint.Event{})
				case 'l', 'L':
					if !annotationEnabled {
						continue
					}
					tool = ToolLine
					active = actionNone
					w.Send(paint.Event{})
				case 'a', 'A':
					if !annotationEnabled {
						continue
					}
					tool = ToolArrow
					active = actionNone
					w.Send(paint.Event{})
				case 'x', 'X':
					if !annotationEnabled {
						continue
					}
					tool = ToolRect
					active = actionNone
					w.Send(paint.Event{})
				case 't', 'T':
					if !annotationEnabled {
						continue
					}
					tool = ToolText
					active = actionNone
					w.Send(paint.Event{})
				case 'h', 'H':
					if !annotationEnabled {
						continue
					}
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
						if tool == ToolMove {
							tabs[current].Offset.X -= 10
							w.Send(paint.Event{})
						}
					case key.CodeRightArrow:
						if tool == ToolMove {
							tabs[current].Offset.X += 10
							w.Send(paint.Event{})
						}
					case key.CodeUpArrow:
						if tool == ToolMove {
							tabs[current].Offset.Y -= 10
							w.Send(paint.Event{})
						}
					case key.CodeDownArrow:
						if tool == ToolMove {
							tabs[current].Offset.Y += 10
							w.Send(paint.Event{})
						}
					}
				}
			}
		}
	}
}
