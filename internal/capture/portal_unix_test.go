//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"errors"
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
	}{
		{
			name:        "defaults",
			interactive: false,
			opts:        Options{},
		},
		{
			name:        "cursor and decorations",
			interactive: true,
			opts: Options{
				IncludeDecorations: true,
				IncludeCursor:      true,
			},
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
			if got := stringVariant(t, values, "handle_token"); got != "test-token" {
				t.Fatalf("handle_token = %q, want %q", got, "test-token")
			}
			if len(values) != 3 {
				t.Fatalf("expected 3 options, got %d", len(values))
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
	portalHandleToken = func() (string, error) { return "test_token", nil }
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
	portalHandleToken = func() (string, error) { return "test_token", nil }
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
	portalHandleToken = func() (string, error) { return "test_token", nil }
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
	portalHandleToken = func() (string, error) { return "test_token", nil }
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
	portalHandleToken = func() (string, error) { return "test_token", nil }
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

func TestPortalHandleTokenAndPath(t *testing.T) {
	token, err := newPortalHandleToken()
	if err != nil {
		t.Fatalf("newPortalHandleToken() failed: %v", err)
	}
	if token == "" {
		t.Fatal("newPortalHandleToken() returned empty token")
	}

	// Verify token contains only valid D-Bus object-path-element characters: [A-Za-z0-9_]
	for _, c := range token {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			t.Fatalf("token %q contains invalid D-Bus object path element character %q", token, c)
		}
	}

	// Verify expected request path with real token is valid
	pathWithRealToken := expectedRequestPath(":1.42", token)
	if !pathWithRealToken.IsValid() {
		t.Fatalf("expected request path %q is not a valid dbus.ObjectPath", pathWithRealToken)
	}

	// Verify repeated generation does not trivially return the same token
	seen := make(map[string]bool)
	const iterations = 50
	for i := 0; i < iterations; i++ {
		tok, err := newPortalHandleToken()
		if err != nil {
			t.Fatalf("iteration %d: newPortalHandleToken() failed: %v", i, err)
		}
		if seen[tok] {
			t.Fatalf("iteration %d: duplicate token generated: %q", i, tok)
		}
		seen[tok] = true
	}

	// Verify predictable-path construction converts senders (e.g., :1.42 -> 1_42)
	testCases := []struct {
		sender   string
		token    string
		wantPath dbus.ObjectPath
	}{
		{
			sender:   ":1.42",
			token:    "token_1",
			wantPath: "/org/freedesktop/portal/desktop/request/1_42/token_1",
		},
		{
			sender:   ":1.100.2",
			token:    "token_multi",
			wantPath: "/org/freedesktop/portal/desktop/request/1_100_2/token_multi",
		},
		{
			sender:   ":1.0",
			token:    token,
			wantPath: dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_0/" + token),
		},
	}
	for _, tc := range testCases {
		got := expectedRequestPath(tc.sender, tc.token)
		if got != tc.wantPath {
			t.Errorf("expectedRequestPath(%q, %q) = %q, want %q", tc.sender, tc.token, got, tc.wantPath)
		}
		if !got.IsValid() {
			t.Errorf("path %q is not a valid dbus.ObjectPath", got)
		}
	}
}

func TestPortalScreenshotTokenError(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() (string, error) { return "", errors.New("entropy failure") }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{uniqueName: ":1.42"}
	connectPortalBus = func() (portalBus, error) { return mockBus, nil }

	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "portal handle token: entropy failure") {
		t.Fatalf("expected entropy failure error, got %v", err)
	}
}

func TestPortalScreenshotResponseValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       []any
		wantErrSub string
	}{
		{
			name:       "short response body",
			body:       []any{uint32(0)},
			wantErrSub: "malformed response, expected 2 arguments",
		},
		{
			name:       "malformed response code type",
			body:       []any{"not_uint32", map[string]dbus.Variant{}},
			wantErrSub: "expected uint32 response code",
		},
		{
			name:       "unknown response code",
			body:       []any{uint32(99), map[string]dbus.Variant{}},
			wantErrSub: "unknown response code 99",
		},
		{
			name:       "malformed results type",
			body:       []any{uint32(0), "not a results map"},
			wantErrSub: "expected map[string]dbus.Variant results",
		},
		{
			name:       "missing uri",
			body:       []any{uint32(0), map[string]dbus.Variant{}},
			wantErrSub: "missing uri in response",
		},
		{
			name: "malformed uri type",
			body: []any{
				uint32(0),
				map[string]dbus.Variant{
					"uri": dbus.MakeVariant(123),
				},
			},
			wantErrSub: "malformed uri, expected string",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prevToken := portalHandleToken
			portalHandleToken = func() (string, error) { return "test_token", nil }
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
					Body: tc.body,
				}

				return &dbus.Call{
					Body: []any{expectedPath},
				}, nil
			}

			_, err := portalScreenshot(false, Options{})
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErrSub)
			}
			if !strings.Contains(err.Error(), tc.wantErrSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErrSub, err)
			}
		})
	}
}

func TestPortalScreenshotCleanupError(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() (string, error) { return "test_token", nil }
	t.Cleanup(func() { portalHandleToken = prevToken })

	prevConnect := connectPortalBus
	t.Cleanup(func() { connectPortalBus = prevConnect })

	mockBus := &mockPortalBus{
		uniqueName:     ":1.42",
		removeMatchErr: errors.New("simulated match removal failure"),
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

	// Even if RemoveMatchSignal fails, portalScreenshot should proceed to return the primary result
	// without failing solely due to the cleanup error.
	_, err := portalScreenshot(false, Options{})
	if err == nil || !strings.Contains(err.Error(), "portal screenshot image: open /test/path.png") {
		t.Fatalf("expected loadPNG error, got %v", err)
	}

	if mockBus.matchesRemoved != 1 {
		t.Fatalf("expected 1 match removed attempt, got %d", mockBus.matchesRemoved)
	}
}

func TestPortalScreenshotURIParsing(t *testing.T) {
	tests := []struct {
		name      string
		uri       string
		wantError string
		wantPath  string
	}{
		{
			name:      "ordinary file uri",
			uri:       "file:///tmp/example.png",
			wantError: "",
			wantPath:  "/tmp/example.png",
		},
		{
			name:      "percent-escaped local pathname",
			uri:       "file:///tmp/Shiney%20Shot.png",
			wantError: "",
			wantPath:  "/tmp/Shiney Shot.png",
		},
		{
			name:      "malformed uri",
			uri:       "://invalid",
			wantError: "invalid uri",
		},
		{
			name:      "unsupported scheme",
			uri:       "http://example.com/image.png",
			wantError: "unsupported uri scheme \"http\"",
		},
		{
			name:      "unsupported host",
			uri:       "file://remotehost/tmp/image.png",
			wantError: "unsupported uri host \"remotehost\"",
		},
		{
			name:      "localhost host",
			uri:       "file://localhost/tmp/image.png",
			wantError: "",
			wantPath:  "/tmp/image.png",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bus := &mockPortalBus{
				uniqueName: ":1.123",
			}
			expectedPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/1_123/test_token")

			bus.callFunc = func(dest string, path dbus.ObjectPath, method string, flags dbus.Flags, args ...any) (*dbus.Call, error) {
				if bus.signalChan != nil {
					bus.signalChan <- &dbus.Signal{
						Path: expectedPath,
						Name: "org.freedesktop.portal.Request.Response",
						Body: []any{
							uint32(0),
							map[string]dbus.Variant{
								"uri": dbus.MakeVariant(tc.uri),
							},
						},
					}
				}
				return &dbus.Call{
					Body: []any{expectedPath},
				}, nil
			}

			prevConnect := connectPortalBus
			prevToken := portalHandleToken
			connectPortalBus = func() (portalBus, error) {
				return bus, nil
			}
			portalHandleToken = func() (string, error) {
				return "test_token", nil
			}
			t.Cleanup(func() {
				connectPortalBus = prevConnect
				portalHandleToken = prevToken
			})

			_, err := portalScreenshot(false, Options{})
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("expected error containing %q, got %v", tc.wantError, err)
				}
			} else {
				// The test will fail in loadPNG because the file does not exist,
				// which is expected. We just want to check the parsed path.
				if err == nil {
					t.Fatalf("expected error from loadPNG, got nil")
				}
				if !strings.Contains(err.Error(), tc.wantPath) {
					t.Fatalf("expected loadPNG error for path %q, got %v", tc.wantPath, err)
				}
			}
		})
	}
}
