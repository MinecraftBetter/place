package main

import (
	"image"
	"image/color"
	"testing"
)

func TestSquareCanvasExtendsAtTheBottom(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 6))
	red := color.NRGBA{255, 0, 0, 255}
	src.SetNRGBA(1, 5, red)

	m := squareCanvas(src)
	if b := m.Bounds(); b.Dx() != 8 || b.Dy() != 8 {
		t.Fatalf("size = %dx%d, want 8x8", b.Dx(), b.Dy())
	}
	if got := m.NRGBAAt(1, 5); got != red {
		t.Errorf("existing pixel moved or changed: %v", got)
	}
	for _, p := range []image.Point{{0, 6}, {7, 7}, {3, 6}} {
		if got := m.NRGBAAt(p.X, p.Y); got != (color.NRGBA{255, 255, 255, 255}) {
			t.Errorf("new pixel %v = %v, want opaque white", p, got)
		}
	}
}

func TestSquareCanvasKeepsASquareImage(t *testing.T) {
	m := squareCanvas(image.NewNRGBA(image.Rect(0, 0, 5, 5)))
	if b := m.Bounds(); b.Dx() != 5 || b.Dy() != 5 {
		t.Fatalf("size = %dx%d, want 5x5", b.Dx(), b.Dy())
	}
}
