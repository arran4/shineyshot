//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && cgo

package clipboard

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"sync"

	"golang.design/x/clipboard"
)

var (
	initOnce sync.Once
	initErr  error
)

func ensureInit() error {
	initOnce.Do(func() {
		initErr = clipboard.Init()
	})
	return initErr
}

// WriteImage encodes the provided image as PNG and publishes it to the clipboard.
func WriteImage(img image.Image) error {
	if err := ensureInit(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return err
	}
	clipboard.Write(clipboard.FmtImage, buf.Bytes())
	return nil
}

// ReadImage retrieves PNG image data from the clipboard and decodes it.
func ReadImage() (image.Image, error) {
	if err := ensureInit(); err != nil {
		return nil, err
	}
	data := clipboard.Read(clipboard.FmtImage)
	if len(data) == 0 {
		return nil, fmt.Errorf("clipboard does not contain image data")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return img, nil
}

// WriteText writes text data to the clipboard.
func WriteText(text string) error {
	if err := ensureInit(); err != nil {
		return err
	}
	clipboard.Write(clipboard.FmtText, []byte(text))
	return nil
}

// ReadText returns UTF-8 text data from the clipboard.
func ReadText() (string, error) {
	if err := ensureInit(); err != nil {
		return "", err
	}
	data := clipboard.Read(clipboard.FmtText)
	if len(data) == 0 {
		return "", fmt.Errorf("clipboard does not contain text data")
	}
	return string(data), nil
}
