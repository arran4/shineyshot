package appstate

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var extraTextFaces sync.Map // map[float64]font.Face

// TextSizes returns the available point sizes for text annotations.
func TextSizes() []float64 {
	out := make([]float64, len(textSizes))
	copy(out, textSizes)
	return out
}

// DefaultTextSize returns the smallest configured text size.
func DefaultTextSize() float64 {
	if len(textSizes) == 0 {
		return 12
	}
	return textSizes[0]
}

func faceForSize(size float64) (font.Face, error) {
	if size <= 0 {
		size = DefaultTextSize()
	}
	// If the size matches one of the predefined faces use it directly.
	for i, s := range textSizes {
		if math.Abs(s-size) < 0.01 {
			return textFaces[i], nil
		}
	}
	if goregularFont == nil {
		return nil, fmt.Errorf("text font not initialised")
	}
	if face, ok := extraTextFaces.Load(size); ok {
		return face.(font.Face), nil
	}
	face, err := opentype.NewFace(goregularFont, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	extraTextFaces.Store(size, face)
	return face, nil
}

// MeasureText returns the dimensions of text rendered at the provided size.
// The returned width and height represent the bounding box, while baseline is
// the offset from the top to the text baseline.
func MeasureText(text string, size float64) (width, height, baseline int, err error) {
	face, err := faceForSize(size)
	if err != nil {
		return 0, 0, 0, err
	}
	drawer := &font.Drawer{Face: face}
	width = drawer.MeasureString(text).Ceil()
	metrics := face.Metrics()
	ascent := metrics.Ascent.Ceil()
	descent := metrics.Descent.Ceil()
	baseline = ascent
	height = ascent + descent
	return
}

// DrawText renders the provided text with its top-left corner at (x, y).
func DrawText(img *image.RGBA, x, y int, text string, col color.Color, size float64) error {
	face, err := faceForSize(size)
	if err != nil {
		return err
	}
	metrics := face.Metrics()
	baseline := y + metrics.Ascent.Ceil()
	drawer := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, baseline),
	}
	drawer.DrawString(text)
	return nil
}

// DrawNumber renders a numbered marker centred at (cx, cy).
func DrawNumber(img *image.RGBA, cx, cy, value, size int, col color.Color) {
	if size <= 0 {
		size = numberSizes[0]
	}
	drawNumberBox(img, cx, cy, value, col, size)
}

// DrawMask darkens the provided rectangle with the supplied colour. The colour
// alpha controls the mask strength.
func DrawMask(img *image.RGBA, rect image.Rectangle, col color.Color) {
	draw.Draw(img, rect, image.NewUniform(col), image.Point{}, draw.Over)
}
