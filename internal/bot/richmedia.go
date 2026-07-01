package bot

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	_ "image/gif" // register GIF decoder for image.Decode
	_ "image/png" // register PNG decoder for image.Decode
)

// flattenToWhite decodes an image and composites it over an opaque white
// background, re-encoding as JPEG. Transparent PNG figures (ФИПИ schemes,
// geometry drawings) would otherwise be invisible on Telegram's dark chat
// background; on white they stay legible in both themes — same trick the web
// uses (the always-white figure container in MediaBlock). Opaque images pass
// through the same composite unchanged. If decoding fails (unsupported format),
// the original bytes are returned so sending still works as a best effort.
func flattenToWhite(raw []byte) []byte {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw
	}
	b := img.Bounds()
	canvas := image.NewRGBA(b)
	draw.Draw(canvas, b, image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(canvas, b, img, b.Min, draw.Over)
	var out bytes.Buffer
	if err := jpeg.Encode(&out, canvas, &jpeg.Options{Quality: 90}); err != nil {
		return raw
	}
	return out.Bytes()
}
