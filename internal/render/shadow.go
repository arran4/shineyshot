package render

import (
	"image"
	"image/color"
	"image/draw"
)

// ShadowOptions configures the drop shadow effect applied to an image.
type ShadowOptions struct {
	Radius  int
	Offset  image.Point
	Opacity float64
}

// ShadowResult captures the output of ApplyShadow.
type ShadowResult struct {
	// Image is the composited image that includes the blurred shadow.
	Image *image.RGBA
	// Offset reports how far the original image content was translated when
	// rebasing onto the expanded canvas. It can be used by callers to adjust
	// viewport offsets so the on-screen location of the content remains
	// stable.
	Offset image.Point
}

// DefaultShadowOptions returns a conservative drop shadow configuration that
// works well with most screenshots.
func DefaultShadowOptions() ShadowOptions {
	return ShadowOptions{
		Radius:  24,
		Offset:  image.Pt(16, 16),
		Opacity: 0.55,
	}
}

// ApplyShadow composites img with a blurred drop shadow using opts. The result
// always has a non-negative origin so it can be used directly with RGBA
// routines that expect zero-based bounds. The returned Offset indicates where
// the original image's top-left corner ended up inside the expanded canvas.
func ApplyShadow(img *image.RGBA, opts ShadowOptions) ShadowResult {
	if img == nil {
		return ShadowResult{}
	}
	if img.Bounds().Empty() {
		return ShadowResult{Image: img}
	}
	if opts.Opacity <= 0 {
		return ShadowResult{Image: img}
	}
	opacity := opts.Opacity
	if opacity > 1 {
		opacity = 1
	}
	radius := opts.Radius
	if radius < 0 {
		radius = 0
	}

	srcBounds := img.Bounds()
	paddedBounds := srcBounds
	if radius > 0 {
		paddedBounds = paddedBounds.Inset(-radius)
	}

	shadowBounds := paddedBounds.Add(opts.Offset)
	compositeBounds := srcBounds.Union(shadowBounds)
	dstRect := compositeBounds.Sub(compositeBounds.Min)
	width := dstRect.Dx()
	height := dstRect.Dy()
	if width <= 0 || height <= 0 {
		return ShadowResult{Image: img}
	}

	shift := srcBounds.Min.Sub(compositeBounds.Min)
	shadowOrigin := shadowBounds.Min.Sub(compositeBounds.Min)

	mask := image.NewGray(paddedBounds.Sub(paddedBounds.Min))
	for y := srcBounds.Min.Y; y < srcBounds.Max.Y; y++ {
		for x := srcBounds.Min.X; x < srcBounds.Max.X; x++ {
			a := img.RGBAAt(x, y).A
			if a == 0 {
				continue
			}
			mx := x - paddedBounds.Min.X
			my := y - paddedBounds.Min.Y
			mask.SetGray(mx, my, color.Gray{Y: a})
		}
	}

	blurred := blurGray(mask, radius)

	dst := image.NewRGBA(dstRect)
	draw.Draw(dst, dst.Bounds(), image.Transparent, image.Point{}, draw.Src)
	shadowAlpha := uint8(opacity*255 + 0.5)
	if shadowAlpha > 0 {
		draw.DrawMask(dst, blurred.Bounds().Add(shadowOrigin), image.NewUniform(color.RGBA{0, 0, 0, shadowAlpha}), image.Point{}, blurred, blurred.Bounds().Min, draw.Over)
	}
	draw.Draw(dst, srcBounds.Sub(compositeBounds.Min), img, srcBounds.Min, draw.Over)

	return ShadowResult{Image: dst, Offset: shift}
}

func blurGray(src *image.Gray, radius int) *image.Gray {
	if radius <= 0 {
		out := image.NewGray(src.Bounds())
		copy(out.Pix, src.Pix)
		return out
	}
	bounds := src.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()
	tmp := image.NewGray(bounds)
	dst := image.NewGray(bounds)

	for y := 0; y < h; y++ {
		rowStart := y * src.Stride
		tmpStart := y * tmp.Stride
		prefix := make([]int, w+1)
		for x := 0; x < w; x++ {
			prefix[x+1] = prefix[x] + int(src.Pix[rowStart+x])
		}
		for x := 0; x < w; x++ {
			x0 := x - radius
			if x0 < 0 {
				x0 = 0
			}
			x1 := x + radius
			if x1 >= w {
				x1 = w - 1
			}
			sum := prefix[x1+1] - prefix[x0]
			count := x1 - x0 + 1
			tmp.Pix[tmpStart+x] = uint8(sum / count)
		}
	}

	for x := 0; x < w; x++ {
		prefix := make([]int, h+1)
		for y := 0; y < h; y++ {
			prefix[y+1] = prefix[y] + int(tmp.Pix[y*tmp.Stride+x])
		}
		for y := 0; y < h; y++ {
			y0 := y - radius
			if y0 < 0 {
				y0 = 0
			}
			y1 := y + radius
			if y1 >= h {
				y1 = h - 1
			}
			sum := prefix[y1+1] - prefix[y0]
			count := y1 - y0 + 1
			dst.Pix[y*dst.Stride+x] = uint8(sum / count)
		}
	}

	return dst
}
