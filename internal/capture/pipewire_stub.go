//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

func pipewireScreenshot(Options) (*image.RGBA, error) {
	return nil, fmt.Errorf("pipewire screenshot is not supported on this platform")
}
