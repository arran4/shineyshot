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

func TestWindowDetailedListWindowsError(t *testing.T) {
	t.Helper()

	originalBackend := backend
	windowsErr := errors.New("windows unavailable")
	backend = fakeBackend{windowsErr: windowsErr}
	t.Cleanup(func() { backend = originalBackend })

	if _, _, err := WindowDetailed("foo", Options{}); err == nil {
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

func TestScreenshotFallsBackToX11Root(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	}

	called := false
	want := image.NewRGBA(image.Rect(0, 0, 1, 1))
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		called = true
		return want, nil
	}

	got, err := Screenshot("", Options{})
	if err != nil {
		t.Fatalf("Screenshot returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected X11 root fallback to be used")
	}
	if got != want {
		t.Fatalf("expected X11 root result, got %#v", got)
	}
}

func TestScreenshotFallsBackWhenPortalDisconnects(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, fmt.Errorf("portal screenshot call: %w", &dbus.Error{Name: "org.freedesktop.DBus.Error.Disconnected"})
	}

	called := false
	want := image.NewRGBA(image.Rect(0, 0, 1, 1))
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		called = true
		return want, nil
	}

	got, err := Screenshot("", Options{})
	if err != nil {
		t.Fatalf("Screenshot returned error: %v", err)
	}
	if !called {
		t.Fatalf("expected X11 root fallback to be used")
	}
	if got != want {
		t.Fatalf("expected X11 root result, got %#v", got)
	}
}

func TestScreenshotFallbackX11RootFailure(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	}

	x11RootCalled := false
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		x11RootCalled = true
		return nil, errors.New("X11 root unavailable")
	}

	_, err := Screenshot("", Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if !x11RootCalled {
		t.Fatalf("expected X11 root fallback to be attempted")
	}
	if !strings.Contains(err.Error(), "X11 root fallback") {
		t.Fatalf("expected X11 root fallback context, got %v", err)
	}
}

func TestInteractiveScreenshotDoesNotFallbackToX11Root(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", ":0")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalErr := &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, portalErr
	}

	x11RootCalled := false
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		x11RootCalled = true
		return nil, errors.New("X11 root should not be used")
	}

	_, err := Region(Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if x11RootCalled {
		t.Fatalf("did not expect X11 root fallback for interactive capture")
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) {
		t.Fatalf("expected wrapped portal error, got %v", err)
	}
}

func TestScreenshotDoesNotFallbackOnWayland(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalErr := &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, portalErr
	}

	x11RootCalled := false
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		x11RootCalled = true
		return nil, errors.New("X11 root should not be used on Wayland")
	}

	_, err := Screenshot("", Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if x11RootCalled {
		t.Fatalf("did not expect X11 root fallback on Wayland")
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) {
		t.Fatalf("expected wrapped portal error, got %v", err)
	}
}

func TestScreenshotDoesNotFallbackOnWaylandDisplay(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	t.Setenv("DISPLAY", ":0")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalErr := &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, portalErr
	}

	x11RootCalled := false
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		x11RootCalled = true
		return nil, errors.New("X11 root should not be used on Wayland")
	}

	_, err := Screenshot("", Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if x11RootCalled {
		t.Fatalf("did not expect X11 root fallback on Wayland")
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) {
		t.Fatalf("expected wrapped portal error, got %v", err)
	}
}

func TestScreenshotX11RootFailureWhenDisplayMissing(t *testing.T) {
	t.Helper()
	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("DISPLAY", "")

	prevPortal := portalScreenshotFn
	prevX11Root := x11RootScreenshotFn
	t.Cleanup(func() {
		portalScreenshotFn = prevPortal
		x11RootScreenshotFn = prevX11Root
	})

	portalErr := &dbus.Error{Name: "org.freedesktop.portal.Error.NotSupported"}
	portalScreenshotFn = func(bool, Options) (*image.RGBA, error) {
		return nil, portalErr
	}

	x11RootCalled := false
	x11RootScreenshotFn = func(Options) (*image.RGBA, error) {
		x11RootCalled = true
		return nil, errors.New("X11 root should not be used when DISPLAY is missing")
	}

	_, err := Screenshot("", Options{})
	if err == nil {
		t.Fatalf("expected error")
	}
	if x11RootCalled {
		t.Fatalf("did not expect X11 root fallback when DISPLAY is missing")
	}
	var dbusErr *dbus.Error
	if !errors.As(err, &dbusErr) {
		t.Fatalf("expected wrapped portal error to be preserved, got %v", err)
	}
}
