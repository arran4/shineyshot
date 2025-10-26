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

func portalScreenshot(interactive bool, captureOpts CaptureOptions) (*image.RGBA, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			fmt.Fprintf(os.Stderr, "dbus close: %v\n", cerr)
		}
	}()

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := portalScreenshotOptions(interactive, captureOpts)
	var handle dbus.ObjectPath
	call := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts)
	if call.Err != nil {
		return nil, fmt.Errorf("portal screenshot call: %w", call.Err)
	}
	if err := call.Store(&handle); err != nil {
		return nil, fmt.Errorf("portal screenshot response: %w", err)
	}

	sigc := make(chan *dbus.Signal, 1)
	conn.Signal(sigc)
	rule := fmt.Sprintf("type='signal',interface='org.freedesktop.portal.Request',member='Response',path='%s'", handle)
	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
		return nil, fmt.Errorf("portal screenshot subscribe: %w", err)
	}
	defer conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule)

	for sig := range sigc {
		if sig.Path == handle && sig.Name == "org.freedesktop.portal.Request.Response" {
			if len(sig.Body) >= 2 {
				res := sig.Body[1].(map[string]dbus.Variant)
				if uriVar, ok := res["uri"]; ok {
					uri := uriVar.Value().(string)
					path := strings.TrimPrefix(uri, "file://")
					img, err := loadPNG(path)
					if err != nil {
						return nil, fmt.Errorf("portal screenshot image: %w", err)
					}
					return img, nil
				}
			}
			break
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

func portalScreenshotOptions(interactive bool, captureOpts CaptureOptions) map[string]dbus.Variant {
	cursorMode := "hidden"
	if captureOpts.IncludeCursor {
		cursorMode = "embedded"
	}
	return map[string]dbus.Variant{
		"interactive":    dbus.MakeVariant(interactive),
		"handle_token":   dbus.MakeVariant(portalHandleToken()),
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
