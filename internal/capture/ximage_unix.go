//go:build linux || freebsd || openbsd || netbsd || dragonfly

package capture

import (
	"fmt"
	"image"

	"github.com/jezek/xgb/xproto"
)

func xImageToRGBA(setup *xproto.SetupInfo, reply *xproto.GetImageReply, width, height int, kind string) (*image.RGBA, error) {
	if setup == nil {
		return nil, fmt.Errorf("xproto setup unavailable")
	}
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("%s has empty geometry", kind)
	}
	if reply == nil {
		return nil, fmt.Errorf("%s pixels: missing reply", kind)
	}
	if len(reply.Data) == 0 {
		return nil, fmt.Errorf("%s pixels: empty image data", kind)
	}

	bitsPerPixel := 0
	for _, format := range setup.PixmapFormats {
		if format.Depth == reply.Depth {
			bitsPerPixel = int(format.BitsPerPixel)
			break
		}
	}
	if bitsPerPixel == 0 {
		return nil, fmt.Errorf("unsupported %s depth %d", kind, reply.Depth)
	}
	bytesPerPixel := bitsPerPixel / 8
	if bytesPerPixel < 3 {
		return nil, fmt.Errorf("unsupported %s pixel format %d bpp", kind, bitsPerPixel)
	}

	stride := len(reply.Data) / height
	if stride*height != len(reply.Data) {
		return nil, fmt.Errorf("%s pixels: unexpected stride", kind)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		row := reply.Data[y*stride : (y+1)*stride]
		for x := 0; x < width; x++ {
			off := x * bytesPerPixel
			if off+3 > len(row) {
				break
			}
			b := row[off]
			g := row[off+1]
			r := row[off+2]
			a := byte(0xFF)
			if bytesPerPixel >= 4 && off+3 < len(row) {
				a = row[off+3]
			}
			pix := img.PixOffset(x, y)
			img.Pix[pix+0] = r
			img.Pix[pix+1] = g
			img.Pix[pix+2] = b
			img.Pix[pix+3] = a
		}
	}
	return img, nil
}
