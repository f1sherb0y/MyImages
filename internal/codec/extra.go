package codec

import (
	"bytes"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
	"github.com/gen2brain/avif"
	"github.com/gen2brain/jpegxl"
)

// Formats whose decoders are libraries: AVIF and JPEG XL stand in where
// the system has no codec for them (Windows has one only once the user
// installed its extension from the Store), and JPEG 2000 has none there
// at all.
func init() {
	register(decodeAVIF, isBMFF("avif", "avis"), "avif", "avifs")
	systemFirst["avif"], systemFirst["avifs"] = true, true
	register(decodeJXL, func(h []byte) bool {
		return hasPrefix("\xff\x0a")(h) || hasPrefix("\x00\x00\x00\x0cJXL \r\n\x87\n")(h)
	}, "jxl")
	systemFirst["jxl"] = true
	register(decodeJP2, func(h []byte) bool {
		return hasPrefix("\x00\x00\x00\x0cjP  \r\n\x87\n")(h) || hasPrefix("\xff\x4f\xff\x51")(h)
	}, "jp2", "j2k", "j2c", "jpc", "jpf", "jpx", "jpm")
}

// isBMFF matches files of the ISO base media format of the brands given,
// as AVIF's and HEIF's are.
func isBMFF(brands ...string) func([]byte) bool {
	return func(h []byte) bool {
		if len(h) < 12 || string(h[4:8]) != "ftyp" {
			return false
		}
		// The major brand, then the compatible ones.
		size := min(int(h[0])<<24|int(h[1])<<16|int(h[2])<<8|int(h[3]), len(h))
		for i := 8; i+4 <= size; i += 4 {
			if i == 12 {
				continue // the minor version
			}
			for _, b := range brands {
				if string(h[i:i+4]) == b {
					return true
				}
			}
		}
		return false
	}
}

func decodeAVIF(data []byte, opt Options) (*Picture, error) {
	img, err := avif.Decode(bytes.NewReader(data), avif.Options{AutoRotate: true})
	if err != nil {
		return nil, err
	}
	return fromImage(img, "AVIF", opt), nil
}

func decodeJXL(data []byte, opt Options) (*Picture, error) {
	img, err := jpegxl.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return fromImage(img, "JPEG XL", opt), nil
}

func decodeJP2(data []byte, opt Options) (*Picture, error) {
	img, err := jpeg2000.DecodeWithUpsampling(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return fromImage(img, "JPEG 2000", opt), nil
}
