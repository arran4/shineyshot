//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

func x11RootScreenshot(Options) (*image.RGBA, error) {
	return nil, fmt.Errorf("x11 root screenshot is not supported on this platform")
}
