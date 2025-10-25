//go:build !(linux || freebsd || openbsd || netbsd || dragonfly)

package clipboard

import (
	"fmt"
	"image"
)

func WriteImage(image.Image) error {
	return fmt.Errorf("clipboard image operations are not supported on this platform")
}

func ReadImage() (image.Image, error) {
	return nil, fmt.Errorf("clipboard image operations are not supported on this platform")
}

func WriteText(string) error {
	return fmt.Errorf("clipboard text operations are not supported on this platform")
}

func ReadText() (string, error) {
	return "", fmt.Errorf("clipboard text operations are not supported on this platform")
}
