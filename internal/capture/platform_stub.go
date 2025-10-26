//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

type unsupportedBackend struct{}

func newBackend() platformBackend {
	return unsupportedBackend{}
}

func (unsupportedBackend) ListMonitors() ([]MonitorInfo, error) {
	return nil, fmt.Errorf("monitor listing is not supported on this platform")
}

func (unsupportedBackend) ListWindows() ([]WindowInfo, error) {
	return nil, fmt.Errorf("window listing is not supported on this platform")
}

func (unsupportedBackend) CaptureWindowImage(uint32) (*image.RGBA, error) {
	return nil, fmt.Errorf("window capture is not supported on this platform")
}

func runningOnWayland() bool { return false }
