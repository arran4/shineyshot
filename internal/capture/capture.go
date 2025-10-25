package capture

import (
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"
	"strings"

	"github.com/godbus/dbus/v5"
)

// CaptureScreenshot captures the desktop. When a display selector is provided it will
// crop the result to the matching monitor.
func CaptureScreenshot(display string) (*image.RGBA, error) {
	img, err := portalScreenshot(false)
	if err != nil {
		return nil, err
	}
	if display == "" {
		return img, nil
	}
	monitors, err := ListMonitors()
	if err != nil {
		return nil, err
	}
	monitor, err := FindMonitor(monitors, display)
	if err != nil {
		return nil, err
	}
	return cropToRect(img, monitor.Rect)
}

// CaptureWindowDetailed captures the window that matches the selector and returns
// both the image and the resolved window metadata. It prefers a direct X11 window
// capture and falls back to cropping a desktop screenshot if the compositor
// refuses to provide the pixels.
func CaptureWindowDetailed(selector string) (*image.RGBA, WindowInfo, error) {
	windows, err := ListWindows()
	if err != nil {
		return nil, WindowInfo{}, err
	}
	info, err := SelectWindow(selector, windows)
	if err != nil {
		return nil, WindowInfo{}, err
	}
	if info.Rect.Empty() {
		return nil, WindowInfo{}, fmt.Errorf("window has empty geometry")
	}
	img, directErr := captureWindowImage(info.ID)
	if directErr == nil {
		return img, info, nil
	}
	shot, err := portalScreenshot(false)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("window capture: %v; fallback screenshot failed: %w", directErr, err)
	}
	img, err = cropToRect(shot, info.Rect)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("window capture: %v; fallback crop failed: %w", directErr, err)
	}
	return img, info, nil
}

// CaptureWindow captures a single window specified by the selector string.
func CaptureWindow(selector string) (*image.RGBA, error) {
	img, _, err := CaptureWindowDetailed(selector)
	return img, err
}

// CaptureRegion uses the portal to allow the user to select a region interactively.
func CaptureRegion() (*image.RGBA, error) {
	return portalScreenshot(true)
}

// CaptureRegionRect captures a specific rectangle in global screen coordinates.
func CaptureRegionRect(rect image.Rectangle) (*image.RGBA, error) {
	if rect.Empty() {
		return nil, fmt.Errorf("region is empty")
	}
	shot, err := portalScreenshot(false)
	if err != nil {
		return nil, err
	}
	return cropToRect(shot, rect)
}

func cropToRect(src *image.RGBA, rect image.Rectangle) (*image.RGBA, error) {
	rect = rect.Intersect(src.Bounds())
	if rect.Empty() {
		return nil, fmt.Errorf("requested region outside captured image")
	}
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), src, rect.Min, draw.Src)
	return dst, nil
}

func portalScreenshot(interactive bool) (*image.RGBA, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("dbus connect: %w", err)
	}
	defer conn.Close()

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	opts := map[string]dbus.Variant{
		"interactive": dbus.MakeVariant(interactive),
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
					img, err := loadPNG(path)
					if err != nil {
						return nil, err
					}
					return img, nil
				}
			}
			break
		}
	}
	return nil, fmt.Errorf("screenshot failed")
}

func loadPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(path) // best effort cleanup

	img, err := png.Decode(f)
	if err != nil {
		return nil, err
	}
	rgba := image.NewRGBA(img.Bounds())
	draw.Draw(rgba, rgba.Bounds(), img, image.Point{}, draw.Src)
	return rgba, nil
}
