//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"fmt"
	"image"
)

func pipewireScreenshot(opts CaptureOptions) (*image.RGBA, error) {
	return nil, fmt.Errorf("pipewire screenshot capture is not implemented")
}
