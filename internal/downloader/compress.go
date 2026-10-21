package downloader

import (
	"bytes"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoding for image.Decode

	_ "golang.org/x/image/webp" // register WebP decoding for image.Decode
)

// recompressToJPEG decodes an image (jpeg/png/webp) and re-encodes it as a
// JPEG at the given quality, trading a little fidelity for a much smaller
// PDF: scanned book pages are the dominant cost in the final file size.
// ok is false if the input couldn't be decoded, in which case callers
// should keep the original bytes.
func recompressToJPEG(data []byte, quality int) (out []byte, ok bool) {
	if quality <= 0 {
		quality = 75
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}
