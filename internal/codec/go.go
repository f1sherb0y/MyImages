package codec

import (
	"bytes"
	"image"
	"image/draw"
	"io"
	"runtime"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// maxPixels is the most pixels a picture may have: 4 bytes each, what
// fits in memory with room to spare.
const maxPixels = 1 << 30

// decoder decodes the bytes of a file in one format.
type decoder func(data []byte, opt Options) (*Picture, error)

// goDecoders are the decoders written in Go, by the extensions of the
// files they are tried on first. sniffers find them by a file's content.
var goDecoders = map[string]decoder{}

// systemFirst are the extensions of goDecoders that the system's codecs
// get first where they know the format: ours stand in where it has none.
var systemFirst = map[string]bool{}

// sniffer tells whether the bytes a file starts with are of its format.
type sniffer struct {
	match  func(head []byte) bool
	decode decoder
}

var sniffers []sniffer

// register adds a decoder for files of the extensions, and of the
// content match tells, if not nil.
func register(decode decoder, match func(head []byte) bool, exts ...string) {
	for _, e := range exts {
		goDecoders[e] = decode
	}
	if match != nil {
		sniffers = append(sniffers, sniffer{match, decode})
	}
}

func decodeGo(path string, d decoder, opt Options) (*Picture, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return finish(d(data, opt))
}

// decodeSniffed decodes a file by what its content says it is.
func decodeSniffed(path string, opt Options) (*Picture, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return DecodeBytes(data, opt)
}

// DecodeBytes decodes an image in memory, by its content, with the
// decoders written in Go.
func DecodeBytes(data []byte, opt Options) (*Picture, error) {
	head := data[:min(len(data), 512)]
	var firstErr error
	for _, s := range sniffers {
		if !s.match(head) {
			continue
		}
		p, err := finish(s.decode(data, opt))
		if err == nil {
			return p, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = ErrUnknown
	}
	return nil, firstErr
}

// finish fills in what decoders leave out.
func finish(p *Picture, err error) (*Picture, error) {
	if err != nil || p == nil {
		if err == nil {
			err = ErrUnknown
		}
		return nil, err
	}
	if p.FullW == 0 {
		p.FullW, p.FullH = p.W, p.H
	}
	if p.Orient == 0 {
		p.Orient = 1
	}
	if p.Frames == 0 {
		p.Frames = 1
	}
	if p.Via == "" {
		p.Via = "go"
	}
	return p, nil
}

// fromImage makes a picture of an image of package image, reduced as
// the options ask.
func fromImage(img image.Image, format string, opt Options) *Picture {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	p := &Picture{FullW: w, FullH: h, Format: format}
	p.W, p.H = fit(w, h, opt.MaxW, opt.MaxH)
	dst := image.NewRGBA(image.Rect(0, 0, p.W, p.H))
	if p.W == w && p.H == h {
		if src, ok := img.(*image.NRGBA); ok {
			premultiply(dst.Pix, src.Pix, w, h, src.Stride)
		} else {
			draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
		}
	} else {
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, xdraw.Src, nil)
	}
	p.Pix = dst.Pix
	switch m := img.(type) {
	case *image.YCbCr, *image.Gray, *image.Gray16, *image.CMYK:
		p.Opaque = true
	case interface{ Opaque() bool }:
		p.Opaque = m.Opaque()
	default:
		p.Opaque = opaque(p.Pix)
	}
	return p
}

// fromNRGBA makes a picture of straight (not premultiplied) RGBA pixels
// in rows of 4*w bytes, which it takes over, reduced as the options ask.
func fromNRGBA(pix []byte, w, h int, format string, opt Options) *Picture {
	premultiply(pix, pix, w, h, 4*w)
	return fromRGBA(pix, w, h, format, opt)
}

// fromRGBA makes a picture of premultiplied pixels, which it takes over.
func fromRGBA(pix []byte, w, h int, format string, opt Options) *Picture {
	p := &Picture{W: w, H: h, FullW: w, FullH: h, Pix: pix, Format: format}
	if tw, th := fit(w, h, opt.MaxW, opt.MaxH); tw != w || th != h {
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		xdraw.ApproxBiLinear.Scale(dst, dst.Bounds(), p.RGBA(), image.Rect(0, 0, w, h), xdraw.Src, nil)
		p.W, p.H, p.Pix = tw, th, dst.Pix
	}
	p.Opaque = opaque(p.Pix)
	return p
}

// premultiply writes src's straight RGBA rows to dst premultiplied. dst
// may be src.
func premultiply(dst, src []byte, w, h, stride int) {
	parallel(h, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			s := src[y*stride : y*stride+4*w]
			d := dst[y*4*w : y*4*w+4*w]
			for i := 0; i < len(s); i += 4 {
				a := uint32(s[i+3])
				switch a {
				case 255:
					d[i], d[i+1], d[i+2], d[i+3] = s[i], s[i+1], s[i+2], 255
				case 0:
					d[i], d[i+1], d[i+2], d[i+3] = 0, 0, 0, 0
				default:
					d[i] = uint8((uint32(s[i])*a + 127) / 255)
					d[i+1] = uint8((uint32(s[i+1])*a + 127) / 255)
					d[i+2] = uint8((uint32(s[i+2])*a + 127) / 255)
					d[i+3] = uint8(a)
				}
			}
		}
	})
}

// opaque tells whether no pixel is transparent.
func opaque(pix []byte) bool {
	n := len(pix) / 4
	var mu sync.Mutex
	all := true
	parallel(n, func(i0, i1 int) {
		for i := i0*4 + 3; i < i1*4; i += 4 {
			if pix[i] != 255 {
				mu.Lock()
				all = false
				mu.Unlock()
				return
			}
		}
	})
	return all
}

// parallel runs fn over the parts of 0..n on the machine's cores, when
// there is enough to be worth it.
func parallel(n int, fn func(from, to int)) {
	workers := runtime.GOMAXPROCS(0)
	if n < 1<<16 || workers < 2 {
		fn(0, n)
		return
	}
	workers = min(workers, 8)
	var wg sync.WaitGroup
	step := (n + workers - 1) / workers
	for from := 0; from < n; from += step {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn(from, min(from+step, n))
		}()
	}
	wg.Wait()
}

// std decodes with a decoder of Go's own packages.
func std(format string, decode func(io.Reader) (image.Image, error)) decoder {
	return func(data []byte, opt Options) (*Picture, error) {
		img, err := decode(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		return fromImage(img, format, opt), nil
	}
}

func hasPrefix(magic string) func([]byte) bool {
	return func(head []byte) bool { return bytes.HasPrefix(head, []byte(magic)) }
}
