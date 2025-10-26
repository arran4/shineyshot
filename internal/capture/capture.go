package capture

import (
	"errors"
	"fmt"
	"image"
	"image/draw"
)

var portalScreenshotFn = portalScreenshot

// CaptureScreenshot captures the desktop. When a display selector is provided it will
// crop the result to the matching monitor.
func CaptureScreenshot(display string) (*image.RGBA, error) {
	img, err := portalScreenshotFn(false)
	if err != nil {
		return nil, fmt.Errorf("capture screenshot via portal: %w", err)
	}
	if display == "" {
		return img, nil
	}
	monitors, err := ListMonitors()
	if err != nil {
		return nil, fmt.Errorf("capture screenshot for display %q: %w", display, err)
	}
	monitor, err := FindMonitor(monitors, display)
	if err != nil {
		return nil, fmt.Errorf("capture screenshot for display %q: %w", display, err)
	}
	cropped, err := cropToRect(img, monitor.Rect)
	if err != nil {
		return nil, fmt.Errorf("capture screenshot for display %q: %w", display, err)
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
		return nil, WindowInfo{}, fmt.Errorf("capture window %q: %w", selector, err)
	}
	info, err := SelectWindow(selector, windows)
	if err != nil {
		return nil, WindowInfo{}, fmt.Errorf("capture window %q: %w", selector, err)
	}
	if info.Rect.Empty() {
		return nil, WindowInfo{}, fmt.Errorf("window has empty geometry")
	}
	img, directErr := captureWindowImage(info.ID)
	if directErr == nil {
		return img, info, nil
	}
	directErr = fmt.Errorf("direct window capture: %w", directErr)
	shot, err := portalScreenshotFn(false)
	if err != nil {
		fallbackErr := fmt.Errorf("fallback portal screenshot: %w", err)
		return nil, WindowInfo{}, fmt.Errorf("window capture failed: %w", errors.Join(directErr, fallbackErr))
	}
	img, err = cropToRect(shot, info.Rect)
	if err != nil {
		fallbackErr := fmt.Errorf("fallback crop: %w", err)
		return nil, WindowInfo{}, fmt.Errorf("window capture failed: %w", errors.Join(directErr, fallbackErr))
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
	img, err := portalScreenshotFn(true)
	if err != nil {
		return nil, fmt.Errorf("capture region: %w", err)
	}
	return img, nil
}

// CaptureRegionRect captures a specific rectangle in global screen coordinates.
func CaptureRegionRect(rect image.Rectangle) (*image.RGBA, error) {
	if rect.Empty() {
		return nil, fmt.Errorf("region is empty")
	}
	shot, err := portalScreenshotFn(false)
	if err != nil {
		return nil, fmt.Errorf("capture screenshot via portal: %w", err)
	}
	img, err := cropToRect(shot, rect)
	if err != nil {
		return nil, fmt.Errorf("crop region: %w", err)
	}
	return img, nil
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

// portalScreenshot and loadPNG are implemented in platform-specific files.
