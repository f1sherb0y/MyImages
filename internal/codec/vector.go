package codec

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"math"

	"github.com/egoist/mygo/ui"
)

// vectorSide is about how long the longer side of a drawing is drawn,
// in pixels: a drawing has no pixels of its own, so it is drawn larger
// than its nominal size, to stay sharp when enlarged.
const vectorSide = 4096

// vectorSize returns the pixels to draw a drawing of a nominal size
// with: a whole multiple of it, about vectorSide on the longer side, or
// the size reduced to the box the options ask for.
func vectorSize(w, h float64, opt Options) (pw, ph int, partial bool) {
	long := max(w, h)
	k := max(math.Floor(vectorSide/long), 1)
	if long > vectorSide {
		k = vectorSide / long
	}
	fw, fh := w*k, h*k
	if opt.MaxW > 0 && opt.MaxH > 0 {
		// Enough for the box, and no more.
		f := min(float64(opt.MaxW)/fw, float64(opt.MaxH)/fh)
		if f < 1 {
			fw, fh, partial = fw*f, fh*f, true
		}
	}
	return max(int(math.Round(fw)), 1), max(int(math.Round(fh)), 1), partial
}

func init() {
	register(decodeSVG, func(h []byte) bool {
		if hasPrefix("\x1f\x8b")(h) {
			return false // gzip of anything: the extension says
		}
		return bytes.Contains(h, []byte("<svg"))
	}, "svg", "svgz")
}

// decodeSVG draws an SVG, gzipped or not. Its nominal size is the one
// it says it has; it is drawn with more pixels than that.
func decodeSVG(data []byte, opt Options) (*Picture, error) {
	if hasPrefix("\x1f\x8b")(data) {
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		if data, err = io.ReadAll(io.LimitReader(zr, 256<<20)); err != nil {
			return nil, err
		}
	}
	svg, err := ui.ParseSVG(data)
	if err != nil {
		return nil, err
	}
	w, h := svg.Size()
	if w <= 0 || h <= 0 {
		return nil, errors.New("the drawing has no size")
	}
	pw, ph, partial := vectorSize(float64(w), float64(h), opt)
	img := svg.Image(pw, ph, nil)
	p := &Picture{W: pw, H: ph, Pix: img.Pix, Format: "SVG", Partial: partial}
	p.FullW, p.FullH = max(int(math.Round(float64(w))), 1), max(int(math.Round(float64(h))), 1)
	p.Vector = true
	p.Opaque = opaque(p.Pix)
	return p, nil
}
