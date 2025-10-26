package capture

import (
	"errors"
	"image"
	"strings"
	"testing"
)

type fakeBackend struct {
	monitors    []MonitorInfo
	windows     []WindowInfo
	monitorsErr error
	windowsErr  error
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
	return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
}

func TestCaptureScreenshotPortalError(t *testing.T) {
	t.Helper()

	originalPortal := portalScreenshotFn
	sentinel := errors.New("portal down")
	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) { return nil, sentinel }
	t.Cleanup(func() { portalScreenshotFn = originalPortal })

	if _, err := CaptureScreenshot("", CaptureOptions{}); err == nil {
		t.Fatalf("expected error")
	} else {
		if !errors.Is(err, sentinel) {
			t.Fatalf("expected sentinel error, got %v", err)
		}
		if want := "capture screenshot via portal"; !strings.Contains(err.Error(), want) {
			t.Fatalf("expected error to contain %q, got %v", want, err)
		}
	}
}

func TestCaptureScreenshotDisplayMonitorError(t *testing.T) {
	t.Helper()

	originalPortal := portalScreenshotFn
	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) {
		return image.NewRGBA(image.Rect(0, 0, 2, 2)), nil
	}
	t.Cleanup(func() { portalScreenshotFn = originalPortal })

	originalBackend := backend
	monitorErr := errors.New("monitors offline")
	backend = fakeBackend{monitorsErr: monitorErr}
	t.Cleanup(func() { backend = originalBackend })

	if _, err := CaptureScreenshot("primary", CaptureOptions{}); err == nil {
		t.Fatalf("expected error")
	} else {
		if !errors.Is(err, monitorErr) {
			t.Fatalf("expected wrapped monitor error, got %v", err)
		}
		if want := "capture screenshot for display"; !strings.Contains(err.Error(), want) {
			t.Fatalf("expected context in error, got %v", err)
		}
	}
}

func TestCaptureWindowDetailedListWindowsError(t *testing.T) {
	t.Helper()

	originalBackend := backend
	windowsErr := errors.New("windows unavailable")
	backend = fakeBackend{windowsErr: windowsErr}
	t.Cleanup(func() { backend = originalBackend })

	if _, _, err := CaptureWindowDetailed("foo", CaptureOptions{}); err == nil {
		t.Fatalf("expected error")
	} else {
		if !errors.Is(err, windowsErr) {
			t.Fatalf("expected wrapped windows error, got %v", err)
		}
		if want := "capture window \"foo\""; !strings.Contains(err.Error(), want) {
			t.Fatalf("expected selector context, got %v", err)
		}
	}
}
