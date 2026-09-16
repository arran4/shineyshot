//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
)

type mockPortalBus struct {
	uniqueName     string
	callFunc       func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error)
	addMatchErr    error
	removeMatchErr error
	signalChan     chan<- *dbus.Signal
	matchesAdded   int
	matchesRemoved int
	closeErr       error
}

func (m *mockPortalBus) UniqueName() string {
	return m.uniqueName
}

func (m *mockPortalBus) Call(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
	if m.callFunc != nil {
		return m.callFunc(dest, path, method, flags, args...)
	}
	return &dbus.Call{}, nil
}

func (m *mockPortalBus) AddMatchSignal(options ...dbus.MatchOption) error {
	m.matchesAdded++
	return m.addMatchErr
}

func (m *mockPortalBus) RemoveMatchSignal(options ...dbus.MatchOption) error {
	m.matchesRemoved++
	return m.removeMatchErr
}

func (m *mockPortalBus) Signal(ch chan<- *dbus.Signal) {
	m.signalChan = ch
}

func (m *mockPortalBus) RemoveSignal(ch chan<- *dbus.Signal) {
	if m.signalChan == ch {
		m.signalChan = nil
	}
}

func (m *mockPortalBus) Close() error {
	return m.closeErr
}

func TestPortalScreenshotOptions(t *testing.T) {
	tests := []struct {
		name        string
		interactive bool
		opts        Options
		wantCursor  string
		wantRestore bool
	}{
		{
			name:        "defaults",
			interactive: false,
			opts:        Options{},
			wantCursor:  "hidden",
			wantRestore: false,
		},
		{
			name:        "cursor and decorations",
			interactive: true,
			opts: Options{
				IncludeDecorations: true,
				IncludeCursor:      true,
			},
			wantCursor:  "embedded",
			wantRestore: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := portalScreenshotOptions(tc.interactive, tc.opts, "test-token")

			if got := boolVariant(t, values, "interactive"); got != tc.interactive {
				t.Fatalf("interactive = %v, want %v", got, tc.interactive)
			}
			if got := boolVariant(t, values, "modal"); got != tc.interactive {
				t.Fatalf("modal = %v, want %v", got, tc.interactive)
			}
			if got := stringVariant(t, values, "cursor_mode"); got != tc.wantCursor {
				t.Fatalf("cursor_mode = %q, want %q", got, tc.wantCursor)
			}
			if got := boolVariant(t, values, "restore_window"); got != tc.wantRestore {
				t.Fatalf("restore_window = %v, want %v", got, tc.wantRestore)
			}
			if got := stringVariant(t, values, "handle_token"); got != "test-token" {
				t.Fatalf("handle_token = %q, want %q", got, "test-token")
			}
			if len(values) != 5 {
				t.Fatalf("expected 5 options, got %d", len(values))
			}
		})
	}
}

func boolVariant(t *testing.T, values map[string]dbus.Variant, key string) bool {
	t.Helper()
	variant, ok := values[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	v, ok := variant.Value().(bool)
	if !ok {
		t.Fatalf("key %q value is %T, want bool", key, variant.Value())
	}
	return v
}

func stringVariant(t *testing.T, values map[string]dbus.Variant, key string) string {
	t.Helper()
	variant, ok := values[key]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	v, ok := variant.Value().(string)
	if !ok {
		t.Fatalf("key %q value is %T, want string", key, variant.Value())
	}
	return v
}

func TestPortalScreenshotWorkflow(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test_token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName: ":1.42",
	}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_42/test_token")

	mockBus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
		res := map[string]dbus.Variant{
			"uri": dbus.MakeVariant("file:///test/path.png"),
		}
		mockBus.signalChan <- &dbus.Signal{
			Path: expectedPath,
			Name: "org.freedesktop.portal.Request.Response",
			Body: []any{uint32(0), res},
		}

		return &dbus.Call{
			Body: []any{expectedPath},
		}, nil
	}

	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "portal screenshot image: open /test/path.png") {
		t.Fatalf("expected loadPNG error, got %v", err)
	}

	if mockBus.matchesAdded != 1 {
		t.Fatalf("expected 1 match added, got %d", mockBus.matchesAdded)
	}
	if mockBus.matchesRemoved != 1 {
		t.Fatalf("expected 1 match removed, got %d", mockBus.matchesRemoved)
	}
}

func TestPortalScreenshotCancellation(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test_token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName: ":1.42",
	}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_42/test_token")

	mockBus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
		mockBus.signalChan <- &dbus.Signal{
			Path: expectedPath,
			Name: "org.freedesktop.portal.Request.Response",
			Body: []any{uint32(1), map[string]dbus.Variant{}},
		}

		return &dbus.Call{
			Body: []any{expectedPath},
		}, nil
	}

	_, err := portalScreenshot(false, Options{})
	if err != ErrCancelled {
		t.Fatalf("expected ErrCancelled, got %v", err)
	}
}

func TestPortalScreenshotFailure(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test_token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName: ":1.42",
	}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_42/test_token")

	mockBus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
		mockBus.signalChan <- &dbus.Signal{
			Path: expectedPath,
			Name: "org.freedesktop.portal.Request.Response",
			Body: []any{uint32(2), map[string]dbus.Variant{}},
		}

		return &dbus.Call{
			Body: []any{expectedPath},
		}, nil
	}

	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "portal failed") {
		t.Fatalf("expected portal failed error, got %v", err)
	}
}

func TestPortalScreenshotLegacyPath(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test_token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName: ":1.42",
	}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	legacyPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/some_other_path")

	mockBus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
		res := map[string]dbus.Variant{
			"uri": dbus.MakeVariant("file:///test/path.png"),
		}

		go func() {
			mockBus.signalChan <- &dbus.Signal{
				Path: legacyPath,
				Name: "org.freedesktop.portal.Request.Response",
				Body: []any{uint32(0), res},
			}
		}()

		return &dbus.Call{
			Body: []any{legacyPath},
		}, nil
	}

	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "portal screenshot image: open /test/path.png") {
		t.Fatalf("expected loadPNG error on legacy path, got %v", err)
	}

	if mockBus.matchesAdded != 2 {
		t.Fatalf("expected 2 matches added (expected + legacy), got %d", mockBus.matchesAdded)
	}
	if mockBus.matchesRemoved != 2 {
		t.Fatalf("expected 2 matches removed, got %d", mockBus.matchesRemoved)
	}
}

func TestPortalScreenshotMalformedResponse(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test_token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName: ":1.42",
	}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_42/test_token")

	mockBus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
		mockBus.signalChan <- &dbus.Signal{
			Path: expectedPath,
			Name: "org.freedesktop.portal.Request.Response",
			Body: []any{uint32(0)},
		}

		return &dbus.Call{
			Body: []any{expectedPath},
		}, nil
	}

	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "malformed response, expected 2 arguments") {
		t.Fatalf("expected malformed response error, got %v", err)
	}
}
