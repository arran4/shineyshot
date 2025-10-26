//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

func pipewireScreenshot(CaptureOptions) (*image.RGBA, error) {
	return nil, fmt.Errorf("pipewire screenshot is not supported on this platform")
}
