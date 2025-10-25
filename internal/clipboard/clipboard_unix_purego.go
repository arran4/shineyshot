//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !cgo

package clipboard

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"sync"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

var (
	initOnce     sync.Once
	initErr      error
	errNoDisplay = errors.New("clipboard initialization requires DISPLAY or WAYLAND_DISPLAY")
	backend      *x11Clipboard
)

func ensureInit() error {
	initOnce.Do(func() {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			initErr = errNoDisplay
			return
		}
		clip := &x11Clipboard{}
		if err := clip.initialize(); err != nil {
			initErr = err
			return
		}
		backend = clip
	})
	return initErr
}

// WriteImage encodes the provided image as PNG and publishes it to the clipboard.
func WriteImage(img image.Image) error {
	if err := ensureInit(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	return backend.writeImage(buf.Bytes())
}

// ReadImage retrieves PNG image data from the clipboard and decodes it.
func ReadImage() (image.Image, error) {
	if err := ensureInit(); err != nil {
		return nil, err
	}
	data, err := backend.readSelection(backend.atoms.png)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard does not contain image data")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// WriteText writes text data to the clipboard.
func WriteText(text string) error {
	if err := ensureInit(); err != nil {
		return err
	}
	return backend.writeText([]byte(text))
}

// ReadText returns UTF-8 text data from the clipboard.
func ReadText() (string, error) {
	if err := ensureInit(); err != nil {
		return "", err
	}
	data, err := backend.readSelection(backend.atoms.utf8)
	if err != nil {
		data, err = backend.readSelection(xproto.AtomString)
		if err != nil {
			return "", err
		}
	}
	if len(data) == 0 {
		return "", fmt.Errorf("clipboard does not contain text data")
	}
	// Trim trailing null byte some applications include in STRING responses.
	if data[len(data)-1] == 0 {
		data = data[:len(data)-1]
	}
	return string(data), nil
}

type x11Clipboard struct {
	conn      *xgb.Conn
	window    xproto.Window
	atoms     atomSet
	mu        sync.RWMutex
	textData  []byte
	imageData []byte
}

type atomSet struct {
	clipboard xproto.Atom
	targets   xproto.Atom
	utf8      xproto.Atom
	textPlain xproto.Atom
	png       xproto.Atom
	property  xproto.Atom
}

func (c *x11Clipboard) initialize() error {
	conn, err := xgb.NewConn()
	if err != nil {
		return err
	}
	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)
	window, err := xproto.NewWindowId(conn)
	if err != nil {
		conn.Close()
		return err
	}
	const eventMask = xproto.EventMaskPropertyChange | xproto.EventMaskStructureNotify
	if err := xproto.CreateWindowChecked(conn, screen.RootDepth, window, screen.Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOutput, screen.RootVisual, xproto.CwEventMask, []uint32{eventMask}).Check(); err != nil {
		conn.Close()
		return err
	}
	atoms, err := internAtoms(conn)
	if err != nil {
		xproto.DestroyWindow(conn, window)
		conn.Close()
		return err
	}
	c.conn = conn
	c.window = window
	c.atoms = atoms
	go c.eventLoop()
	return nil
}

func internAtoms(conn *xgb.Conn) (atomSet, error) {
	get := func(name string) (xproto.Atom, error) {
		reply, err := xproto.InternAtom(conn, true, uint16(len(name)), name).Reply()
		if err != nil {
			return 0, err
		}
		return reply.Atom, nil
	}
	clipboard, err := get("CLIPBOARD")
	if err != nil {
		return atomSet{}, err
	}
	targets, err := get("TARGETS")
	if err != nil {
		return atomSet{}, err
	}
	utf8, err := get("UTF8_STRING")
	if err != nil {
		return atomSet{}, err
	}
	textPlain, err := get("text/plain;charset=utf-8")
	if err != nil {
		return atomSet{}, err
	}
	png, err := get("image/png")
	if err != nil {
		return atomSet{}, err
	}
	property, err := get("SHINEYSHOT_CLIPBOARD")
	if err != nil {
		return atomSet{}, err
	}
	return atomSet{clipboard: clipboard, targets: targets, utf8: utf8, textPlain: textPlain, png: png, property: property}, nil
}

func (c *x11Clipboard) writeText(data []byte) error {
	c.mu.Lock()
	c.textData = append([]byte(nil), data...)
	c.imageData = nil
	c.mu.Unlock()
	return c.setSelectionOwner()
}

func (c *x11Clipboard) writeImage(data []byte) error {
	c.mu.Lock()
	c.imageData = append([]byte(nil), data...)
	c.textData = nil
	c.mu.Unlock()
	return c.setSelectionOwner()
}

func (c *x11Clipboard) setSelectionOwner() error {
	return xproto.SetSelectionOwnerChecked(c.conn, c.window, c.atoms.clipboard, xproto.TimeCurrentTime).Check()
}

func (c *x11Clipboard) eventLoop() {
	for {
		ev, err := c.conn.WaitForEvent()
		if err != nil {
			return
		}
		switch e := ev.(type) {
		case xproto.SelectionRequestEvent:
			c.handleSelectionRequest(e)
		case xproto.SelectionClearEvent:
			c.handleSelectionClear()
		}
	}
}

func (c *x11Clipboard) handleSelectionRequest(e xproto.SelectionRequestEvent) {
	property := e.Property
	if property == xproto.AtomNone {
		property = e.Target
	}

	c.mu.RLock()
	text := c.textData
	image := c.imageData
	c.mu.RUnlock()

	var (
		targetType xproto.Atom
		format     byte
		payload    []byte
	)

	switch e.Target {
	case c.atoms.targets:
		targets := []xproto.Atom{c.atoms.targets}
		if len(text) > 0 {
			targets = append(targets, c.atoms.utf8, xproto.AtomString, c.atoms.textPlain)
		}
		if len(image) > 0 {
			targets = append(targets, c.atoms.png)
		}
		payload = atomsToBytes(targets)
		targetType = xproto.AtomAtom
		format = 32
	case c.atoms.utf8, xproto.AtomString, c.atoms.textPlain:
		if len(text) == 0 {
			property = xproto.AtomNone
			break
		}
		payload = text
		targetType = c.atoms.utf8
		format = 8
	case c.atoms.png:
		if len(image) == 0 {
			property = xproto.AtomNone
			break
		}
		payload = image
		targetType = c.atoms.png
		format = 8
	default:
		property = xproto.AtomNone
	}

	if property != xproto.AtomNone {
		var length uint32
		switch format {
		case 8:
			length = uint32(len(payload))
		case 16:
			length = uint32(len(payload) / 2)
		case 32:
			length = uint32(len(payload) / 4)
		}
		xproto.ChangeProperty(c.conn, xproto.PropModeReplace, e.Requestor, property, targetType, format, length, payload)
	}

	notify := xproto.SelectionNotifyEvent{
		Time:      e.Time,
		Requestor: e.Requestor,
		Selection: e.Selection,
		Target:    e.Target,
		Property:  property,
	}
	_ = xproto.SendEvent(c.conn, false, e.Requestor, 0, string(notify.Bytes()))
}

func (c *x11Clipboard) handleSelectionClear() {
	c.mu.Lock()
	c.textData = nil
	c.imageData = nil
	c.mu.Unlock()
}

func (c *x11Clipboard) readSelection(target xproto.Atom) ([]byte, error) {
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	screen := setup.DefaultScreen(conn)

	window, err := xproto.NewWindowId(conn)
	if err != nil {
		return nil, err
	}
	if err := xproto.CreateWindowChecked(conn, 0, window, screen.Root, 0, 0, 1, 1, 0, xproto.WindowClassInputOnly, 0, xproto.CwEventMask, []uint32{xproto.EventMaskPropertyChange}).Check(); err != nil {
		return nil, err
	}
	defer xproto.DestroyWindow(conn, window)

	if err := xproto.DeletePropertyChecked(conn, window, c.atoms.property).Check(); err != nil {
		return nil, err
	}
	if err := xproto.ConvertSelectionChecked(conn, window, c.atoms.clipboard, target, c.atoms.property, xproto.TimeCurrentTime).Check(); err != nil {
		return nil, err
	}

	for {
		ev, err := conn.WaitForEvent()
		if err != nil {
			return nil, err
		}
		switch e := ev.(type) {
		case xproto.SelectionNotifyEvent:
			if e.Property == xproto.AtomNone {
				return nil, fmt.Errorf("clipboard target unavailable")
			}
			if e.Property != c.atoms.property {
				continue
			}
			reply, err := xproto.GetProperty(conn, false, window, c.atoms.property, xproto.GetPropertyTypeAny, 0, (1<<31)-1).Reply()
			if err != nil {
				return nil, err
			}
			data := make([]byte, len(reply.Value))
			copy(data, reply.Value)
			return data, nil
		}
	}
}

func atomsToBytes(atoms []xproto.Atom) []byte {
	buf := make([]byte, len(atoms)*4)
	for i, atom := range atoms {
		xgb.Put32(buf[i*4:], uint32(atom))
	}
	return buf
}
