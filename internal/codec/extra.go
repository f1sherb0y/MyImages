package codec

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"io"
	"math"

	jpeg2000 "github.com/ajroetker/go-jpeg2000"
	"github.com/astrogo/fitsio"
	"github.com/gen2brain/avif"
	"github.com/gen2brain/heic"
	"github.com/gen2brain/jpegxl"
	"golang.org/x/image/ccitt"
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
	// HEIF, where the system has no codec for it or for the video codec
	// inside (HEVC).
	register(func(data []byte, opt Options) (*Picture, error) {
		img, err := heic.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return fromImage(img, "HEIF", opt), nil
	}, isBMFF("heic", "heix", "hevc", "hevx", "heim", "heis", "mif1", "msf1"), "heic", "heif", "hif", "heics", "heifs")
	for _, e := range []string{"heic", "heif", "hif", "heics", "heifs"} {
		systemFirst[e] = true
	}
	// FITS, of telescopes: the first picture in the file, its values
	// spread over the range a screen shows.
	register(func(data []byte, opt Options) (p *Picture, err error) {
		defer func() {
			if recover() != nil {
				p, err = nil, ErrUnknown
			}
		}()
		f, err := fitsio.Open(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		defer f.Close()
		for _, hdu := range f.HDUs() {
			im, ok := hdu.(fitsio.Image)
			if !ok {
				continue
			}
			axes, bitpix := hdu.Header().Axes(), hdu.Header().Bitpix()
			if len(axes) < 2 || axes[0] <= 0 || axes[1] <= 0 || axes[0]*axes[1] > maxPixels {
				continue
			}
			w, h, planes := axes[0], axes[1], 1
			if len(axes) >= 3 && axes[2] == 3 {
				planes = 3 // red, green and blue, one after the other
			}
			raw := im.Raw()
			size := bitpix / 8
			if size < 0 {
				size = -size
			}
			if size == 0 || len(raw) < w*h*planes*size {
				continue
			}
			// The values as numbers, and the range they lie in.
			vals := make([]float64, w*h*planes)
			lo, hi := math.Inf(1), math.Inf(-1)
			for i := range vals {
				b := raw[i*size:]
				var v float64
				switch bitpix {
				case 8:
					v = float64(b[0])
				case 16:
					v = float64(int16(binary.BigEndian.Uint16(b)))
				case 32:
					v = float64(int32(binary.BigEndian.Uint32(b)))
				case 64:
					v = float64(int64(binary.BigEndian.Uint64(b)))
				case -32:
					v = float64(math.Float32frombits(binary.BigEndian.Uint32(b)))
				case -64:
					v = math.Float64frombits(binary.BigEndian.Uint64(b))
				}
				if v != v {
					v = 0
				}
				vals[i] = v
				lo, hi = min(lo, v), max(hi, v)
			}
			if bitpix == 8 {
				lo, hi = 0, 255 // bytes show as they are
			}
			if hi <= lo {
				hi = lo + 1
			}
			// Measurements may all lie in a sliver of their range: they
			// are spread over the screen's. The first row is the bottom
			// one.
			pix := make([]byte, 4*w*h)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					o := pix[4*((h-1-y)*w+x):]
					for c := 0; c < 3; c++ {
						o[c] = uint8((vals[min(c, planes-1)*w*h+y*w+x]-lo)/(hi-lo)*255 + 0.5)
					}
					o[3] = 255
				}
			}
			return fromRGBA(pix, w, h, "FITS", opt), nil
		}
		return nil, ErrUnknown
	}, hasPrefix("SIMPLE  ="), "fits", "fit", "fts")
	// A fax: one page of Group 3 data, 1728 dots wide.
	register(func(data []byte, opt Options) (*Picture, error) {
		const width = 1728
		r := ccitt.NewReader(bytes.NewReader(data), ccitt.MSB, ccitt.Group3, width, ccitt.AutoDetectHeight, &ccitt.Options{})
		raw, _ := io.ReadAll(r)
		rows := len(raw) / (width / 8)
		if rows == 0 {
			return nil, ErrUnknown
		}
		img := image.NewGray(image.Rect(0, 0, width, rows))
		for i := 0; i < width*rows; i++ {
			if raw[i/8]&(0x80>>(i%8)) == 0 {
				img.Pix[i] = 255
			}
		}
		return fromImage(img, "Fax", opt), nil
	}, nil, "fax", "g3")
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

// stretch spreads the grays of a picture of measurements, whose values
// may all lie in a sliver of the range, over all of it; pictures in
// color are left as they are.
func stretch(img image.Image) image.Image {
	b := img.Bounds()
	lo, hi := uint32(0xFFFF), uint32(0)
	gray := image.NewGray16(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			if r != g || g != bl {
				return img
			}
			gray.SetGray16(x-b.Min.X, y-b.Min.Y, color.Gray16{Y: uint16(r)})
			lo, hi = min(lo, r), max(hi, r)
		}
	}
	if hi <= lo {
		return img
	}
	for i := 0; i < len(gray.Pix); i += 2 {
		v := ((uint32(gray.Pix[i])<<8 | uint32(gray.Pix[i+1])) - lo) * 0xFFFF / (hi - lo)
		gray.Pix[i], gray.Pix[i+1] = uint8(v>>8), uint8(v)
	}
	return gray
}
