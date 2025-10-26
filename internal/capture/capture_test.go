package capture

import (
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
)

type fakeBackend struct {
	monitors    []MonitorInfo
	monitorsErr error
	windows     []WindowInfo
	windowsErr  error
	windowImg   *image.RGBA
	captureErr  error
}

func (f fakeBackend) ListMonitors() ([]MonitorInfo, error) {
	if f.monitorsErr != nil {
		return nil, f.monitorsErr
	}
	return f.monitors, nil
}

func (f fakeBackend) ListWindows() ([]WindowInfo, error) {
	if f.windowsErr != nil {
		return nil, f.windowsErr
	}
	return f.windows, nil
}

func (f fakeBackend) CaptureWindowImage(uint32) (*image.RGBA, error) {
	if f.captureErr != nil {
		return nil, f.captureErr
	}
	if f.windowImg != nil {
		return f.windowImg, nil
	}
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.White}, image.Point{}, draw.Src)
	return img, nil
}

func stubScreenshotProvider(t *testing.T, fn func(bool) (*image.RGBA, error)) {
	t.Helper()
	restore := SetScreenshotProviderForTests(fn)
	t.Cleanup(restore)
}

func stubBackend(t *testing.T, fb platformBackend) {
	t.Helper()
	restore := SetBackendForTests(fb)
	t.Cleanup(restore)
}

func TestCaptureScreenshotPortalError(t *testing.T) {
	stubScreenshotProvider(t, func(bool) (*image.RGBA, error) {
		return nil, errors.New("dbus unavailable")
	})
	if _, err := CaptureScreenshot(""); err == nil || !strings.Contains(err.Error(), "capture screenshot") || !strings.Contains(err.Error(), "dbus unavailable") {
		t.Fatalf("expected wrapped portal error, got %v", err)
	}
}

func TestCaptureScreenshotMonitorError(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	stubScreenshotProvider(t, func(bool) (*image.RGBA, error) {
		return img, nil
	})
	stubBackend(t, fakeBackend{monitorsErr: errors.New("randr failed")})
	if _, err := CaptureScreenshot("primary"); err == nil || !strings.Contains(err.Error(), "list monitors") || !strings.Contains(err.Error(), "randr failed") {
		t.Fatalf("expected monitor error context, got %v", err)
	}
}

func TestCaptureWindowDetailedListWindowsError(t *testing.T) {
	stubBackend(t, fakeBackend{windowsErr: errors.New("x11 failed")})
	if _, _, err := CaptureWindowDetailed("active"); err == nil || !strings.Contains(err.Error(), "list windows") || !strings.Contains(err.Error(), "x11 failed") {
		t.Fatalf("expected list windows error, got %v", err)
	}
}
