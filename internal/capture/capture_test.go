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
