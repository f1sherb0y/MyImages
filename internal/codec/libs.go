package codec

import (
	"bytes"
	"image"
	"io"

	"github.com/ftrvxmtrx/tga"
	"github.com/mdouchement/hdr"
	"github.com/mdouchement/hdr/codec/pfm"
	"github.com/mdouchement/hdr/codec/rgbe"
	"github.com/mdouchement/hdr/tmo"
	"github.com/mokiat/goexr/exr"
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
				// More light than a screen shows: brought into its range.
				img = tmo.NewDefaultReinhard05(h).Perform()
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
	register(lib("OpenEXR", exr.Decode), hasPrefix("\x76\x2f\x31\x01"), "exr")
	register(lib("Photoshop", func(r io.Reader) (image.Image, error) {
		img, _, err := psd.Decode(r, &psd.DecodeOptions{SkipLayerImage: true})
		if err != nil {
			return nil, err
		}
		return img.Picker, nil
	}), hasPrefix("8BPS"), "psd", "psb")
}
