//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import "testing"

func TestRunningOnWayland(t *testing.T) {
	t.Setenv("XDG_SESSION_TYPE", "wayland")
	t.Setenv("WAYLAND_DISPLAY", "")
	if !runningOnWayland() {
		t.Fatalf("expected wayland session when XDG_SESSION_TYPE=wayland")
	}

	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "wayland-0")
	if !runningOnWayland() {
		t.Fatalf("expected wayland session when WAYLAND_DISPLAY is set")
	}

	t.Setenv("XDG_SESSION_TYPE", "x11")
	t.Setenv("WAYLAND_DISPLAY", "")
	if runningOnWayland() {
		t.Fatalf("did not expect wayland session when indicators are absent")
	}
}
