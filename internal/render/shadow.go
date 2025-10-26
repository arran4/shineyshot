package render

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// ShadowOptions configures how ApplyShadow renders the drop shadow.
type ShadowOptions struct {
	// Radius controls the blur radius in pixels. Values less than zero are treated as zero.
	Radius int
	// Offset translates the shadow relative to the original image bounds.
	Offset image.Point
	// Opacity controls the strength of the shadow. It is clamped to the range [0,1].
	Opacity float64
}

// ApplyShadow renders a blurred drop shadow behind img and returns the composited result.
// The returned image may be larger than the input when the blur radius or offset cause the
// shadow to extend beyond the original bounds.
func ApplyShadow(img *image.RGBA, opts ShadowOptions) *image.RGBA {
	if img == nil {
		return nil
	}
	radius := opts.Radius
	if radius < 0 {
		radius = 0
	}
	opacity := opts.Opacity
	if opacity < 0 {
		opacity = 0
	} else if opacity > 1 {
		opacity = 1
	}
	bounds := img.Bounds()
	if opacity <= 0 {
		clone := image.NewRGBA(bounds)
		draw.Draw(clone, clone.Bounds(), img, bounds.Min, draw.Src)
		return clone
	}
	// Determine the final bounds required to hold both the original image and the shadow.
	shadowBounds := bounds.Add(opts.Offset)
	shadowBounds = shadowBounds.Inset(-radius)
	finalMin := image.Point{X: min(bounds.Min.X, shadowBounds.Min.X), Y: min(bounds.Min.Y, shadowBounds.Min.Y)}
	finalMax := image.Point{X: max(bounds.Max.X, shadowBounds.Max.X), Y: max(bounds.Max.Y, shadowBounds.Max.Y)}
	finalRect := image.Rectangle{Min: finalMin, Max: finalMax}
	width := finalRect.Dx()
	height := finalRect.Dy()
	alphaBuf := make([]float64, width*height)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			px := img.RGBAAt(x, y)
			if px.A == 0 {
				continue
			}
			destX := x + opts.Offset.X - finalRect.Min.X
			destY := y + opts.Offset.Y - finalRect.Min.Y
			if destX < 0 || destX >= width || destY < 0 || destY >= height {
				continue
			}
			idx := destY*width + destX
			alpha := (float64(px.A) / 255.0) * opacity
			if alpha > alphaBuf[idx] {
				alphaBuf[idx] = alpha
			}
		}
	}
	if radius > 0 {
		alphaBuf = blurAlpha(alphaBuf, width, height, radius)
	}
	out := image.NewRGBA(finalRect)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			alpha := alphaBuf[y*width+x]
			if alpha <= 0 {
				continue
			}
			a := uint8(math.Round(math.Max(0, math.Min(1, alpha)) * 255.0))
			if a == 0 {
				continue
			}
			out.Set(finalRect.Min.X+x, finalRect.Min.Y+y, color.RGBA{A: a})
		}
	}
	drawOffset := bounds.Min.Sub(finalRect.Min)
	draw.Draw(out, bounds.Add(drawOffset), img, bounds.Min, draw.Src)
	return out
}

func blurAlpha(src []float64, width, height, radius int) []float64 {
	if radius <= 0 {
		return src
	}
	window := radius*2 + 1
	tmp := make([]float64, len(src))
	// Horizontal pass
	for y := 0; y < height; y++ {
		row := y * width
		sum := 0.0
		for dx := -radius; dx <= radius; dx++ {
			idx := clampIndex(dx, width)
			sum += src[row+idx]
		}
		for x := 0; x < width; x++ {
			tmp[row+x] = sum / float64(window)
			left := clampIndex(x-radius, width)
			right := clampIndex(x+radius+1, width)
			sum -= src[row+left]
			sum += src[row+right]
		}
	}
	// Vertical pass
	dst := make([]float64, len(src))
	for x := 0; x < width; x++ {
		sum := 0.0
		for dy := -radius; dy <= radius; dy++ {
			idx := clampIndex(dy, height)
			sum += tmp[idx*width+x]
		}
		for y := 0; y < height; y++ {
			dst[y*width+x] = sum / float64(window)
			top := clampIndex(y-radius, height)
			bottom := clampIndex(y+radius+1, height)
			sum -= tmp[top*width+x]
			sum += tmp[bottom*width+x]
		}
	}
	return dst
}

func clampIndex(v, limit int) int {
	if v < 0 {
		return 0
	}
	if v >= limit {
		return limit - 1
	}
	return v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
