package appstate

import (
	"image"
	"image/color"
	"image/draw"
)

// ExpandCanvas enlarges img so that rect fits within it. It returns the new
// image and the amount the coordinate space shifted.
func ExpandCanvas(img *image.RGBA, rect image.Rectangle) (*image.RGBA, image.Point) {
	b := img.Bounds()
	minX := 0
	if rect.Min.X < 0 {
		minX = rect.Min.X
	}
	minY := 0
	if rect.Min.Y < 0 {
		minY = rect.Min.Y
	}
	maxX := b.Max.X
	if rect.Max.X > maxX {
		maxX = rect.Max.X
	}
	maxY := b.Max.Y
	if rect.Max.Y > maxY {
		maxY = rect.Max.Y
	}
	if minX == 0 && minY == 0 && maxX == b.Max.X && maxY == b.Max.Y {
		return img, image.Point{}
	}
	newImg := image.NewRGBA(image.Rect(0, 0, maxX-minX, maxY-minY))
	draw.Draw(newImg, newImg.Bounds(), image.Transparent, image.Point{}, draw.Src)
	draw.Draw(newImg, b.Add(image.Pt(-minX, -minY)), img, image.Point{}, draw.Src)
	return newImg, image.Pt(minX, minY)
}

// DrawLine draws a line between the two points with the given thickness and color.
func DrawLine(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	drawLine(img, x0, y0, x1, y1, col, thick)
}

// DrawArrow draws an arrow between the two points with the given thickness and color.
func DrawArrow(img *image.RGBA, x0, y0, x1, y1 int, col color.Color, thick int) {
	drawArrow(img, x0, y0, x1, y1, col, thick)
}

// DrawRect draws a rectangle on the image with the given thickness and color.
func DrawRect(img *image.RGBA, rect image.Rectangle, col color.Color, thick int) {
	drawRect(img, rect, col, thick)
}

// DrawCircle draws a circle centred at (cx, cy) with radius r.
func DrawCircle(img *image.RGBA, cx, cy, r int, col color.Color, thick int) {
	drawCircle(img, cx, cy, r, col, thick)
}

// CropImage returns a copy of the given rectangle from img.
func CropImage(img *image.RGBA, rect image.Rectangle) *image.RGBA {
	return cropImage(img, rect)
}
