package codec

import (
	"bytes"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"

	"github.com/gen2brain/webp"
	"golang.org/x/image/bmp"
	"golang.org/x/image/tiff"
)

// The formats Go's own packages decode. GIFs are theirs: their frames
// are played from the same bytes. The others stand in where the system
// has no codec for them.
func init() {
	register(decodeGIF, hasPrefix("GIF8"), "gif")
	register(std("PNG", png.Decode), hasPrefix("\x89PNG\r\n\x1a\n"))
	register(std("JPEG", jpeg.Decode), hasPrefix("\xff\xd8\xff"))
	register(std("BMP", bmp.Decode), hasPrefix("BM"))
	register(std("TIFF", tiff.Decode), func(h []byte) bool { return hasPrefix("II*\x00")(h) || hasPrefix("MM\x00*")(h) })
	register(std("WebP", func(r io.Reader) (image.Image, error) { return webp.Decode(r) }), func(h []byte) bool {
		return len(h) >= 12 && string(h[:4]) == "RIFF" && string(h[8:12]) == "WEBP"
	})
}

// decodeGIF decodes a GIF's first frame, and tells whether more follow.
func decodeGIF(data []byte, opt Options) (*Picture, error) {
	img, err := gif.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	p := fromImage(img, "GIF", opt)
	// Whether it moves is found by playing it; most do.
	p.Animated = true
	return p, nil
}
