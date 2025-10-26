package capture

import (
	"fmt"
	"image"
	"image/draw"
)

// CaptureScreenshot captures the desktop. When a display selector is provided it will
// crop the result to the matching monitor.
func CaptureScreenshot(display string) (*image.RGBA, error) {
	img, err := screenshotProvider(false)
	if err != nil {
		return nil, fmt.Errorf("capture screenshot: %w", err)
	}
	if display == "" {
		return img, nil
	}
	monitors, err := ListMonitors()
	if err != nil {
		return nil, fmt.Errorf("list monitors: %w", err)
	}
	monitor, err := FindMonitor(monitors, display)
	if err != nil {
		return nil, fmt.Errorf("resolve monitor %q: %w", display, err)
	}
	cropped, err := cropToRect(img, monitor.Rect)
	if err != nil {
		return nil, fmt.Errorf("crop monitor %q: %w", display, err)
	}
	return cropped, nil
}

// CaptureWindowDetailed captures the window that matches the selector and returns
// both the image and the resolved window metadata. It prefers a direct X11 window
// capture and falls back to cropping a desktop screenshot if the compositor
// refuses to provide the pixels.
func CaptureWindowDetailed(selector string) (*image.RGBA, WindowInfo, error) {
	windows, err := ListWindows()
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("list windows: %w", err)
	}
	info, err := SelectWindow(selector, windows)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("select window %q: %w", selector, err)
	}
	if info.Rect.Empty() {
		return nil, WindowInfo{}, fmt.Errorf("window has empty geometry")
	}
	img, directErr := captureWindowImage(info.ID)
	if directErr == nil {
		return img, info, nil
	}
	shot, err := screenshotProvider(false)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("window capture %q: %v; fallback screenshot failed: %w", selector, directErr, err)
	}
	img, err = cropToRect(shot, info.Rect)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("window capture %q: %v; fallback crop failed: %w", selector, directErr, err)
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
	img, err := screenshotProvider(true)
	if err != nil {
		return nil, fmt.Errorf("interactive region capture: %w", err)
	}
	return img, nil
}

// CaptureRegionRect captures a specific rectangle in global screen coordinates.
func CaptureRegionRect(rect image.Rectangle) (*image.RGBA, error) {
	if rect.Empty() {
		return nil, fmt.Errorf("region is empty")
	}
	shot, err := screenshotProvider(false)
	if err != nil {
		return nil, fmt.Errorf("capture region screenshot: %w", err)
	}
	cropped, err := cropToRect(shot, rect)
	if err != nil {
		return nil, fmt.Errorf("crop region %v: %w", rect, err)
	}
	return cropped, nil
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

var screenshotProvider = portalScreenshot

// SetBackendForTests replaces the platform backend during tests and returns a restore function.
func SetBackendForTests(b platformBackend) func() {
	prev := backend
	backend = b
	return func() { backend = prev }
}

// SetScreenshotProviderForTests replaces the screenshot provider during tests and returns a restore function.
func SetScreenshotProviderForTests(fn func(interactive bool) (*image.RGBA, error)) func() {
	prev := screenshotProvider
	screenshotProvider = fn
	return func() { screenshotProvider = prev }
}

// portalScreenshot and loadPNG are implemented in platform-specific files.
