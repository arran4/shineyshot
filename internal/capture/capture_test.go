package capture

import (
	"errors"
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
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

func TestScreenshotFallsBackToPipewire(t *testing.T) {
	t.Helper()

	prevPortal := portalScreenshotFn
	prevPipewire := pipewireScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		pipewireScreenshotFn = prevPipewire
	})

	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) {
		return nil, &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	}

	called := false
	want := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pipewireScreenshotFn = func(CaptureOptions) (*image.RGBA, error) {
		called = true
		return want, nil
	}

	got, err := CaptureScreenshot("", CaptureOptions{})
	if err != nil {
		t.Fatalf("CaptureScreenshot returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected pipewire fallback to be used")
	}
	if got != want {
		t.Fatalf("expected pipewire result, got %#v", got)
	}
}

func TestScreenshotFallsBackWhenPortalDisconnects(t *testing.T) {
	t.Helper()

	prevPortal := portalScreenshotFn
	prevPipewire := pipewireScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		pipewireScreenshotFn = prevPipewire
	})

	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) {
		return nil, fmt.Errorf("portal screenshot call: %w", &dbus.Error{Name: "org.freedesktop.DBus.Error.Disconnected"})
	}

	called := false
	want := image.NewRGBA(image.Rect(0, 0, 1, 1))
	pipewireScreenshotFn = func(CaptureOptions) (*image.RGBA, error) {
		called = true
		return want, nil
	}

	got, err := CaptureScreenshot("", CaptureOptions{})
	if err != nil {
		t.Fatalf("CaptureScreenshot returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected pipewire fallback to be used")
	}
	if got != want {
		t.Fatalf("expected pipewire result, got %#v", got)
	}
}

func TestScreenshotFallbackPipewireFailure(t *testing.T) {
	t.Helper()

	prevPortal := portalScreenshotFn
	prevPipewire := pipewireScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		pipewireScreenshotFn = prevPipewire
	})

	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) {
		return nil, &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	}

	pipewireCalled := false
	pipewireScreenshotFn = func(CaptureOptions) (*image.RGBA, error) {
		pipewireCalled = true
		return nil, errors.New("pipewire unavailable")
	}

	_, err := CaptureScreenshot("", CaptureOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !pipewireCalled {
		t.Fatalf("expected pipewire fallback to be attempted")
	}
	if !strings.Contains(err.Error(), "pipewire fallback") {
		t.Fatalf("expected pipewire fallback context, got %v", err)
	}
}

func TestInteractiveScreenshotDoesNotFallbackToPipewire(t *testing.T) {
	t.Helper()

	prevPortal := portalScreenshotFn
	prevPipewire := pipewireScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		pipewireScreenshotFn = prevPipewire
	})

	portalErr := &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	portalScreenshotFn = func(bool, CaptureOptions) (*image.RGBA, error) {
		return nil, portalErr
	}

	pipewireCalled := false
	pipewireScreenshotFn = func(CaptureOptions) (*image.RGBA, error) {
		pipewireCalled = true
		return nil, errors.New("pipewire should not be used")
	}

	_, err := CaptureRegion(CaptureOptions{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if pipewireCalled {
		t.Fatalf("did not expect pipewire fallback for interactive capture")
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) {
		t.Fatalf("expected wrapped portal error, got %v", err)
	}
}
