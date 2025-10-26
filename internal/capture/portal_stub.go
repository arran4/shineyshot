//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package capture

import (
	"fmt"
	"image"
)

func portalScreenshot(interactive bool, _ CaptureOptions) (*image.RGBA, error) {
	return nil, fmt.Errorf("portal screenshot is not supported on this platform")
}

func isPortalUnsupportedError(error) bool { return false }
