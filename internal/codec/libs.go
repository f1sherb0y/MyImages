package codec

import (
	"bytes"
	"image"
	"io"
	"math"

	"github.com/ftrvxmtrx/tga"
	"github.com/mdouchement/hdr"
	"github.com/mdouchement/hdr/codec/pfm"
	"github.com/mdouchement/hdr/codec/rgbe"
	"github.com/mrjoshuak/go-openexr/exr"
	"github.com/oov/psd"
	"github.com/samuel/go-pcx/pcx"
	"github.com/spakin/netpbm"
	"github.com/xfmoulet/qoi"
)

// Formats the system has no codec for, decoded by libraries.
func init() {
	lib := func(format string, decode func(io.Reader) (image.Image, error)) decoder {
		return func(data []byte, opt Options) (p *Picture, err error) {
			// A library that trips over a broken file must not take the
			// program down with it.
			defer func() {
				if recover() != nil {
					p, err = nil, ErrUnknown
				}
			}()
			img, err := decode(bytes.NewReader(data))
			if err != nil {
				return nil, err
			}
			if b := img.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 || b.Dx()*b.Dy() > maxPixels {
				return nil, ErrUnknown
			}
			if h, ok := img.(hdr.Image); ok {
				b := img.Bounds()
				pix := fromLight(b.Dx(), b.Dy(), func(x, y int) (float32, float32, float32, float32) {
					r, g, bl, _ := h.HDRAt(b.Min.X+x, b.Min.Y+y).HDRRGBA()
					return float32(r), float32(g), float32(bl), 1
				})
				return fromRGBA(pix, b.Dx(), b.Dy(), format, opt), nil
			}
			return fromImage(img, format, opt), nil
		}
	}
	// A Targa file says nothing by which to know it: by its name only.
	register(lib("TGA", tga.Decode), nil, "tga", "icb", "vda", "vst")
	register(lib("QOI", qoi.Decode), hasPrefix("qoif"), "qoi")
	register(lib("PCX", pcx.Decode), nil, "pcx")
	register(lib("Netpbm", func(r io.Reader) (image.Image, error) { return netpbm.Decode(r, nil) }), func(h []byte) bool {
		return len(h) > 2 && h[0] == 'P' && h[1] >= '1' && h[1] <= '7' && (h[2] == '\n' || h[2] == ' ' || h[2] == '\r' || h[2] == '\t')
	}, "pbm", "pgm", "ppm", "pnm", "pam")
	register(lib("PFM", pfm.Decode), func(h []byte) bool { return hasPrefix("PF\n")(h) || hasPrefix("Pf\n")(h) }, "pfm")
	register(lib("Radiance HDR", rgbe.Decode), func(h []byte) bool { return hasPrefix("#?RADIANCE")(h) || hasPrefix("#?RGBE")(h) }, "hdr", "rgbe")
	// DirectDraw surfaces without compression, which the system's codec
	// leaves out.
	register(decodeDDS, hasPrefix("DDS "), "dds")
	systemFirst["dds"] = true
	register(decodeEXR, hasPrefix("\x76\x2f\x31\x01"), "exr")
	photoshop := lib("Photoshop", func(r io.Reader) (image.Image, error) {
		img, _, err := psd.Decode(r, &psd.DecodeOptions{SkipLayerImage: true})
		if err != nil {
			return nil, err
		}
		return img.Picker, nil
	})
	register(func(data []byte, opt Options) (*Picture, error) {
		// The library reads 8 bits a channel in full color, with the
		// transparency; the other kinds are read here, without.
		if len(data) > 26 && data[23] == 8 && data[25] != 2 && data[25] != 0 {
			if p, err := photoshop(data, opt); err == nil {
				return p, nil
			}
		}
		return decodePSDComposite(data, opt)
	}, hasPrefix("8BPS"), "psd", "psb")
}

// decodeEXR decodes an OpenEXR picture: light as numbers without a
// ceiling, brought to what a screen shows by cutting off what is
// brighter than white.
func decodeEXR(data []byte, opt Options) (p *Picture, err error) {
	defer func() {
		if recover() != nil {
			p, err = nil, ErrUnknown
		}
	}()
	img, err := exr.Decode(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	w, h := img.Rect.Dx(), img.Rect.Dy()
	if w <= 0 || h <= 0 || w*h > maxPixels {
		return nil, ErrUnknown
	}
	pix := fromLight(w, h, func(x, y int) (r, g, b, a float32) {
		return img.RGBA(img.Rect.Min.X+x, img.Rect.Min.Y+y)
	})
	return fromRGBA(pix, w, h, "OpenEXR", opt), nil
}

// fromLight makes premultiplied pixels of light measured in numbers
// without a ceiling, as HDR formats keep it: what is no brighter than
// white shows as it is, and a picture with more light than that is
// brought into the screen's range, the brightest part to white, the
// darker parts barely changed.
func fromLight(w, h int, at func(x, y int) (r, g, b, a float32)) []byte {
	// The screen's curve, by a table of 4096 steps of light.
	var curve [4097]uint8
	for i := range curve {
		v := float64(i) / 4096
		if v <= 0.0031308 {
			v *= 12.92
		} else {
			v = 1.055*math.Pow(v, 1/2.4) - 0.055
		}
		curve[i] = uint8(v*255 + 0.5)
	}
	light := make([]float32, 4*w*h)
	peaks := make([]float32, h)
	parallel(h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			for x := 0; x < w; x++ {
				r, g, b, a := at(x, y)
				i := (y*w + x) * 4
				for c, v := range [3]float32{r, g, b} {
					if !(v > 0) { // negative, or no number
						v = 0
					}
					light[i+c] = min(v, 1e6)
					peaks[y] = max(peaks[y], light[i+c])
				}
				light[i+3] = min(max(a, 0), 1)
			}
		}
	})
	var peak float32
	for _, p := range peaks {
		peak = max(peak, p)
	}
	pix := make([]byte, 4*w*h)
	white := peak * peak
	parallel(h, func(y0, y1 int) {
		for i := y0 * w * 4; i < y1*w*4; i += 4 {
			r, g, b, a := light[i], light[i+1], light[i+2], light[i+3]
			if peak > 1.001 {
				// Reinhard's curve on the brightest channel, which keeps
				// the color.
				if l := max(r, g, b); l > 0 {
					k := (1 + l/white) / (1 + l)
					r, g, b = r*k, g*k, b*k
				}
			}
			pix[i] = uint8(float32(curve[int(min(r, 1)*4096)])*a + 0.5)
			pix[i+1] = uint8(float32(curve[int(min(g, 1)*4096)])*a + 0.5)
			pix[i+2] = uint8(float32(curve[int(min(b, 1)*4096)])*a + 0.5)
			pix[i+3] = uint8(a*255 + 0.5)
		}
	})
	return pix
}
