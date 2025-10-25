//go:build linux || freebsd || openbsd || netbsd || dragonfly

package clipboard

import (
	"errors"
	"sync"
	"testing"
)

func TestEnsureInitWithoutDisplay(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")

	initOnce = sync.Once{}
	initErr = nil

	err := WriteText("hello world")
	if !errors.Is(err, errNoDisplay) {
		t.Fatalf("expected errNoDisplay, got %v", err)
	}
}
