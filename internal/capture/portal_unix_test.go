//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestPortalScreenshotOptions(t *testing.T) {
	prevToken := portalHandleToken
	portalHandleToken = func() string { return "test-token" }
	t.Cleanup(func() { portalHandleToken = prevToken })

	tests := []struct {
		name                  string
		interactive           bool
		opts                  CaptureOptions
		wantCursor            string
		wantRestore           bool
		wantIncludeDecoration bool
	}{
		{
			name:                  "defaults",
			interactive:           false,
			opts:                  CaptureOptions{},
			wantCursor:            "hidden",
			wantRestore:           false,
			wantIncludeDecoration: false,
		},
		{
			name:        "cursor and decorations",
			interactive: true,
			opts: CaptureOptions{
				IncludeDecorations: true,
				IncludeCursor:      true,
			},
			wantCursor:            "embedded",
			wantRestore:           true,
			wantIncludeDecoration: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			values := portalScreenshotOptions(tc.interactive, tc.opts)

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
			if got := boolVariant(t, values, "include-decoration"); got != tc.wantIncludeDecoration {
				t.Fatalf("include-decoration = %v, want %v", got, tc.wantIncludeDecoration)
			}
			if got := stringVariant(t, values, "handle_token"); got != "test-token" {
				t.Fatalf("handle_token = %q, want %q", got, "test-token")
			}
			if len(values) != 6 {
				t.Fatalf("expected 6 options, got %d", len(values))
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
