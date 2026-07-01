package bot

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	_ "image/jpeg"
)

// TestFlattenToWhite_TransparentBecomesOpaqueWhite builds a fully transparent PNG
// and checks that flattening yields an opaque image with a white background — the
// fix for transparent ФИПИ figures vanishing on Telegram's dark theme.
func TestFlattenToWhite_TransparentBecomesOpaqueWhite(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 8, 8)) // all zero = transparent black
	var in bytes.Buffer
	if err := png.Encode(&in, src); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	out := flattenToWhite(in.Bytes())
	if bytes.Equal(out, in.Bytes()) {
		t.Fatalf("expected re-encoded bytes, got original")
	}
	img, _, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode flattened: %v", err)
	}
	r, g, b, a := img.At(2, 2).RGBA()
	if a != 0xffff {
		t.Fatalf("expected opaque pixel, got alpha %d", a)
	}
	// White (allow JPEG rounding).
	if r < 0xf000 || g < 0xf000 || b < 0xf000 {
		t.Fatalf("expected white background, got r=%d g=%d b=%d", r, g, b)
	}
}

// TestFlattenToWhite_BadBytesPassThrough returns the input unchanged when it isn't
// a decodable image (graceful fallback).
func TestFlattenToWhite_BadBytesPassThrough(t *testing.T) {
	raw := []byte("not an image")
	if got := flattenToWhite(raw); !bytes.Equal(got, raw) {
		t.Fatalf("expected passthrough for undecodable bytes")
	}
}
