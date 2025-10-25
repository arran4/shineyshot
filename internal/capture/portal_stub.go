//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

func portalScreenshot(interactive bool) (*image.RGBA, error) {
	return nil, fmt.Errorf("portal screenshot is not supported on this platform")
}
