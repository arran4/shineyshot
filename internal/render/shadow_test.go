package render

import (
	"image"
	"image/color"
	"testing"
)

func TestApplyShadowBoundsGrowth(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	drawColor := color.RGBA{255, 0, 0, 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.SetRGBA(x, y, drawColor)
		}
	}
	opts := ShadowOptions{Radius: 4, Offset: image.Pt(6, 3), Opacity: 0.5}
	res := ApplyShadow(img, opts)
	if res.Image == nil {
		t.Fatalf("expected image result")
	}
	wantBounds := image.Rect(0, 0, 18, 18)
	if res.Image.Bounds() != wantBounds {
		t.Fatalf("bounds mismatch: got %v want %v", res.Image.Bounds(), wantBounds)
	}
	if res.Offset != image.Pt(4, 4) {
		t.Fatalf("offset mismatch: got %v want %v", res.Offset, image.Pt(4, 4))
	}
}

func TestApplyShadowOpacityZeroPassthrough(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.SetRGBA(0, 0, color.RGBA{10, 20, 30, 255})
	opts := ShadowOptions{Radius: 8, Offset: image.Pt(12, 12), Opacity: 0}
	res := ApplyShadow(img, opts)
	if res.Image != img {
		t.Fatalf("expected original image pointer, got %p want %p", res.Image, img)
	}
	if res.Offset != (image.Point{}) {
		t.Fatalf("expected zero offset, got %v", res.Offset)
	}
}

func TestApplyShadowBlurSpread(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.SetRGBA(0, 0, color.RGBA{255, 255, 255, 255})
	opts := ShadowOptions{Radius: 2, Offset: image.Point{}, Opacity: 1}
	res := ApplyShadow(img, opts)
	if got, want := res.Image.Bounds(), image.Rect(0, 0, 5, 5); got != want {
		t.Fatalf("bounds mismatch: got %v want %v", got, want)
	}
	if alpha := res.Image.RGBAAt(4, 2).A; alpha == 0 {
		t.Fatalf("expected blur to spread alpha, got 0 at (4,2)")
	}
}
