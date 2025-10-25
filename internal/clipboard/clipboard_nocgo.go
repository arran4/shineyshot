//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !cgo

package clipboard

import (
	"errors"
	"image"
	"os"
	"sync"
)

var (
	initOnce       sync.Once
	initErr        error
	errNoDisplay   = errors.New("clipboard initialization requires DISPLAY or WAYLAND_DISPLAY")
	errCGODisabled = errors.New("clipboard operations require cgo support")
)

func ensureInit() error {
	initOnce.Do(func() {
		if hasDisplay() {
			initErr = errCGODisabled
			return
		}
		initErr = errNoDisplay
	})
	return initErr
}

func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func WriteImage(image.Image) error {
	return ensureInit()
}

func ReadImage() (image.Image, error) {
	if err := ensureInit(); err != nil {
		return nil, err
	}
	return nil, errCGODisabled
}

func WriteText(string) error {
	return ensureInit()
}

func ReadText() (string, error) {
	if err := ensureInit(); err != nil {
		return "", err
	}
	return "", errCGODisabled
}
