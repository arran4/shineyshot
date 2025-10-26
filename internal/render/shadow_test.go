package render

import (
	"image"
	"image/color"
	"testing"
)

func TestApplyShadowExpandsBounds(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	subject := image.Pt(5, 5)
	img.Set(subject.X, subject.Y, color.RGBA{R: 255, A: 255})

	opts := ShadowOptions{Radius: 4, Offset: image.Pt(8, 6), Opacity: 0.5}
	out := ApplyShadow(img, opts)
	if out == nil {
		t.Fatal("expected output image")
	}
	expected := image.Rect(0, 0, 22, 20)
	if !out.Bounds().Eq(expected) {
		t.Fatalf("unexpected bounds %v, want %v", out.Bounds(), expected)
	}
	// Spot check that the shadow alpha was written near the offset pixel.
	shadowOrigin := subject.Add(opts.Offset)
	shadowPt := shadowOrigin
	if out.RGBAAt(shadowPt.X, shadowPt.Y).A == 0 {
		t.Fatalf("expected shadow alpha at %v", shadowPt)
	}
}

func TestApplyShadowNoShadowWhenOpacityZero(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	fill := color.RGBA{R: 200, G: 100, B: 50, A: 255}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, fill)
		}
	}
	out := ApplyShadow(img, ShadowOptions{Radius: 12, Offset: image.Pt(20, 10), Opacity: 0})
	if out == nil {
		t.Fatal("expected output image")
	}
	if !out.Bounds().Eq(img.Bounds()) {
		t.Fatalf("bounds changed unexpectedly: %v vs %v", out.Bounds(), img.Bounds())
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			if got := out.RGBAAt(x, y); got != fill {
				t.Fatalf("pixel mismatch at (%d,%d): got %+v want %+v", x, y, got, fill)
			}
		}
	}
}

func TestApplyShadowBlurredAlpha(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{A: 255})
	opts := ShadowOptions{Radius: 2, Offset: image.Pt(3, 0), Opacity: 1}

	out := ApplyShadow(img, opts)
	if out == nil {
		t.Fatal("expected output image")
	}
	if out.Bounds().Dx() <= img.Bounds().Dx() {
		t.Fatalf("expected wider output bounds")
	}
	// Check that blur spreads alpha beyond the exact offset location.
	base := img.Bounds().Min.Add(opts.Offset)
	baseAlpha := out.RGBAAt(base.X, base.Y).A
	if baseAlpha == 0 {
		t.Fatal("expected alpha at base shadow location")
	}
	// Neighbor pixel should also have alpha because of blur.
	neighbor := out.RGBAAt(base.X+1, base.Y)
	if neighbor.A == 0 {
		t.Fatalf("expected blurred alpha to reach neighbor, base alpha=%d", baseAlpha)
	}
}
