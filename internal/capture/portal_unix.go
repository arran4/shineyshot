//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

var portalHandleToken = newPortalHandleToken

type portalBus interface {
	UniqueName() string
	Call(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error)
	AddMatchSignal(options ...dbus.MatchOption) error
	RemoveMatchSignal(options ...dbus.MatchOption) error
	Signal(ch chan<- *dbus.Signal)
	RemoveSignal(ch chan<- *dbus.Signal)
	Close() error
}

type realPortalBus struct {
	conn *dbus.Conn
}

func (b *realPortalBus) UniqueName() string {
	names := b.conn.Names()
	for _, n := range names {
		if strings.HasPrefix(n, ":") {
			return n
		}
	}
	return ""
}

func (b *realPortalBus) Call(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
	obj := b.conn.Object(dest, path)
	call := obj.Call(method, flags, args...)
	if call.Err != nil {
		return nil, call.Err
	}
	return call, nil
}

func (b *realPortalBus) AddMatchSignal(options ...dbus.MatchOption) error {
	return b.conn.AddMatchSignal(options...)
}

func (b *realPortalBus) RemoveMatchSignal(options ...dbus.MatchOption) error {
	return b.conn.RemoveMatchSignal(options...)
}

func (b *realPortalBus) Signal(ch chan<- *dbus.Signal) {
	b.conn.Signal(ch)
}

func (b *realPortalBus) RemoveSignal(ch chan<- *dbus.Signal) {
	b.conn.RemoveSignal(ch)
}

func (b *realPortalBus) Close() error {
	return b.conn.Close()
}

var connectPortalBus = func() (portalBus, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}
	return &realPortalBus{conn: conn}, nil
}

func expectedRequestPath(uniqueName string, token string) dbus.ObjectPath {
	sender := strings.ReplaceAll(strings.TrimPrefix(uniqueName, ":"), ".", "_")
	path := fmt.Sprintf("/org/freedesktop/portal/desktop/request/%s/%s", sender, token)
	return dbus.ObjectPath(path)
}

func portalScreenshot(interactive bool, captureOpts Options) (*image.RGBA, error) {
	bus, err := connectPortalBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer func() {
		if cerr := bus.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "dbus close: %v\n", cerr)
		}
	}()

	token := portalHandleToken()
	expectedPath := expectedRequestPath(bus.UniqueName(), token)

	sigc := make(chan *dbus.Signal, 2)
	bus.Signal(sigc)
	defer bus.RemoveSignal(sigc)

	matchExpectedPath := dbus.WithMatchObjectPath(expectedPath)
	matchIface := dbus.WithMatchInterface("org.freedesktop.portal.Request")
	matchMember := dbus.WithMatchMember("Response")

	if err := bus.AddMatchSignal(matchExpectedPath, matchIface, matchMember); err != nil {
		return nil, fmt.Errorf("portal screenshot subscribe expected path: %w", err)
	}
	defer func() {
		_ = bus.RemoveMatchSignal(matchExpectedPath, matchIface, matchMember)
	}()

	opts := portalScreenshotOptions(interactive, captureOpts, token)
	call, err := bus.Call("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop", "org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts)
	if err != nil {
		return nil, fmt.Errorf("portal screenshot call: %w", err)
	}

	var handle dbus.ObjectPath
	if err := call.Store(&handle); err != nil {
		return nil, fmt.Errorf("portal screenshot response: %w", err)
	}

	if handle != expectedPath {
		matchHandlePath := dbus.WithMatchObjectPath(handle)
		if err := bus.AddMatchSignal(matchHandlePath, matchIface, matchMember); err != nil {
			return nil, fmt.Errorf("portal screenshot subscribe legacy path: %w", err)
		}
		defer func() {
			_ = bus.RemoveMatchSignal(matchHandlePath, matchIface, matchMember)
		}()
	}

	for sig := range sigc {
		if (sig.Path == handle || sig.Path == expectedPath) && sig.Name == "org.freedesktop.portal.Request.Response" {
			if len(sig.Body) < 2 {
				return nil, fmt.Errorf("portal screenshot: malformed response, expected 2 arguments, got %d", len(sig.Body))
			}

			responseCode, ok := sig.Body[0].(uint32)
			if !ok {
				return nil, fmt.Errorf("portal screenshot: malformed response, expected uint32 response code, got %T", sig.Body[0])
			}

			if responseCode == 1 {
				return nil, ErrCancelled
			} else if responseCode == 2 {
				return nil, fmt.Errorf("portal screenshot: portal failed")
			} else if responseCode != 0 {
				return nil, fmt.Errorf("portal screenshot: unknown response code %d", responseCode)
			}

			res, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				return nil, fmt.Errorf("portal screenshot: malformed response, expected map[string]dbus.Variant results, got %T", sig.Body[1])
			}

			uriVar, ok := res["uri"]
			if !ok {
				return nil, fmt.Errorf("portal screenshot: missing uri in response")
			}

			uri, ok := uriVar.Value().(string)
			if !ok {
				return nil, fmt.Errorf("portal screenshot: malformed uri, expected string, got %T", uriVar.Value())
			}

			path := strings.TrimPrefix(uri, "file://")
			img, err := loadPNG(path)
			if err != nil {
				return nil, fmt.Errorf("portal screenshot image: %w", err)
			}
			return img, nil
		}
	}
	return nil, fmt.Errorf("portal screenshot: response missing image data")
}

func isPortalUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	var dbusErr *dbus.Error
	if errors.As(err, &dbusErr) {
		switch dbusErr.Name {
		case "org.freedesktop.portal.Error.NotSupported":
			return true
		case "org.freedesktop.DBus.Error.ServiceUnknown":
			// The portal service is not available on the session bus.
			return true
		case "org.freedesktop.DBus.Error.NoReply", "org.freedesktop.DBus.Error.Disconnected":
			// The portal service crashed or exited before replying. Treat this the
			// same as the portal not being supported so that we can fall back to
			// PipeWire-based capture methods.
			return true
		}
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "not supported") {
		return true
	}
	return strings.Contains(lower, "disconnected from message bus without replying")
}

func newPortalHandleToken() string {
	return fmt.Sprintf("shineyshot-%d", time.Now().UnixNano())
}

func portalScreenshotOptions(interactive bool, captureOpts Options, token string) map[string]dbus.Variant {
	cursorMode := "hidden"
	if captureOpts.IncludeCursor {
		cursorMode = "embedded"
	}
	return map[string]dbus.Variant{
		"interactive":    dbus.MakeVariant(interactive),
		"handle_token":   dbus.MakeVariant(token),
		"modal":          dbus.MakeVariant(interactive),
		"cursor_mode":    dbus.MakeVariant(cursorMode),
		"restore_window": dbus.MakeVariant(captureOpts.IncludeDecorations),
	}
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "close %s: %v\n", path, cerr)
		}
	}()
	defer func() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "remove %s: %v\n", path, err)
		}
	}() // best effort cleanup

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	return rgba, nil
}
