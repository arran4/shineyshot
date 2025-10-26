//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"fmt"
	"image"

	"github.com/jezek/xgb"
	"github.com/jezek/xgb/xproto"
)

func pipewireScreenshot(opts CaptureOptions) (*image.RGBA, error) {
	_ = opts
	conn, err := xgb.NewConn()
	if err != nil {
		return nil, fmt.Errorf("connect X server: %w", err)
	}
	defer conn.Close()

	setup := xproto.Setup(conn)
	if setup == nil {
		return nil, fmt.Errorf("xproto setup unavailable")
	}
	screen := setup.DefaultScreen(conn)
	if screen == nil {
		return nil, fmt.Errorf("xproto screen unavailable")
	}

	geom, err := xproto.GetGeometry(conn, xproto.Drawable(screen.Root)).Reply()
	if err != nil {
		return nil, fmt.Errorf("screen geometry: %w", err)
	}
	width := int(geom.Width)
	height := int(geom.Height)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("screen has empty geometry")
	}

	reply, err := xproto.GetImage(conn, xproto.ImageFormatZPixmap, xproto.Drawable(screen.Root), 0, 0, geom.Width, geom.Height, ^uint32(0)).Reply()
	if err != nil {
		return nil, fmt.Errorf("screen pixels: %w", err)
	}

	img, err := xImageToRGBA(setup, reply, width, height, "screen")
	if err != nil {
		return nil, err
	}
	return img, nil
}
