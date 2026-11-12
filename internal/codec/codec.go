// Package codec turns image files into pixels. On Windows the system's
// codecs (WIC) decode what they know, which is fast and covers what the
// user installed (HEIF, AVIF, WebP, JPEG XL, camera raw); decoders
// written in Go take the formats the system has none for.
//
// Everything here runs off the main thread.
package codec

import (
	"bytes"
	"errors"
	"image"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Picture is a decoded image: premultiplied RGBA, rows of 4*W bytes.
type Picture struct {
	W, H int
	Pix  []byte
	// FullW and FullH are the size of the image in the file, which W
	// and H are smaller than when the picture was decoded reduced.
	FullW, FullH int
	// Orient is the EXIF orientation still to apply when showing the
	// pixels, 1 to 8; 1, upright, for pictures that need none.
	Orient int
	// Frames is how many frames or pages the file holds, and Frame the
	// one decoded.
	Frames, Frame int
	// Format names the file's format, as "JPEG".
	Format string
	// Opaque tells that no pixel is transparent.
	Opaque bool
	// Via tells what decoded it, for the curious: "wic", "go", "shell".
	Via string
	// Animated tells that the frames are those of an animation, to play
	// with Animation, rather than pages.
	Animated bool
	// Delay is how long a frame of an animation shows, 0 where the file
	// does not say.
	Delay time.Duration
	// Vector tells that the file is a drawing, which has no pixels of
	// its own: FullW and FullH are its nominal size, and W and H may be
	// larger, as it is drawn with more pixels to stay sharp enlarged.
	// Partial is set when it was drawn with fewer than it would be
	// without a box to fit.
	Vector, Partial bool
	// Preview tells that the pixels are not the file's own but a
	// smaller picture found in it or made of it by the system, as a
	// camera's raw file's preview: the best there is to show.
	Preview bool
}

// RGBA returns the picture as an image of package image, sharing its
// pixels.
func (p *Picture) RGBA() *image.RGBA {
	return &image.RGBA{Pix: p.Pix, Stride: 4 * p.W, Rect: image.Rect(0, 0, p.W, p.H)}
}

// Reduced tells whether the picture has fewer pixels than the file.
func (p *Picture) Reduced() bool { return p.Partial || p.W < p.FullW || p.H < p.FullH }

// Options say how to decode.
type Options struct {
	// MaxW and MaxH, unless zero, ask for a picture no larger than
	// needed to cover that box: decoders that can decode reduced do so,
	// which is several times faster, and the others scale down. The box
	// is for the picture upright, as its orientation will show it.
	MaxW, MaxH int
	// Frame is the frame or page to decode.
	Frame int
}

// ErrUnknown is returned for files no decoder knows.
var ErrUnknown = errors.New("unknown image format")

// Decode reads the picture a file holds.
func Decode(path string, opt Options) (*Picture, error) {
	p, err := decode(path, opt)
	if err == nil && !p.Animated && (p.Format == "PNG" || p.Format == "WebP") && animated(path) {
		p.Animated = true
	}
	return p, err
}

// animated tells whether a PNG or WebP file holds an animation: a PNG
// says so before its picture, a WebP in its header.
func animated(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 4096)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if animatedWebP(head) {
		return true
	}
	at := bytes.Index(head, []byte("acTL"))
	return at > 0 && (bytes.Index(head, []byte("IDAT")) < 0 || at < bytes.Index(head, []byte("IDAT")))
}

// Force, for measuring, decodes with one kind of decoder only: "wic" or
// "go".
var Force string

func decode(path string, opt Options) (*Picture, error) {
	switch Force {
	case "wic":
		return decodeSystem(path, opt)
	case "go":
		return decodeSniffed(path, opt)
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	var firstErr error
	try := func(fn func() (*Picture, error)) *Picture {
		p, err := fn()
		if err == nil && p != nil {
			return p
		}
		if firstErr == nil && err != nil && !errors.Is(err, ErrUnknown) {
			firstErr = err
		}
		return nil
	}
	// Formats the system has no codec for, or decodes worse than we do,
	// go to our decoders first, by their extension; then the system's;
	// then ours again by the file's content, whatever its name says.
	if d := goDecoders[ext]; d != nil && !systemFirst[ext] {
		if p := try(func() (*Picture, error) { return decodeGo(path, d, opt) }); p != nil {
			return p, nil
		}
	}
	if p := try(func() (*Picture, error) { return decodeSystem(path, opt) }); p != nil {
		return p, nil
	}
	if p := try(func() (*Picture, error) { return decodeSniffed(path, opt) }); p != nil {
		return p, nil
	}
	if rawExts[ext] {
		// A camera's raw file the system has no codec for: the JPEG
		// preview the camera put in it.
		if p := try(func() (*Picture, error) { return decodeEmbedded(path, opt) }); p != nil {
			return p, nil
		}
	}
	if p := try(func() (*Picture, error) { return decodeExternal(path, opt) }); p != nil {
		if p.Format == "" {
			p.Format = strings.ToUpper(ext)
		}
		return p, nil
	}
	if p := try(func() (*Picture, error) { return decodeShell(path, opt) }); p != nil {
		return p, nil
	}
	if firstErr == nil {
		firstErr = ErrUnknown
	}
	return nil, firstErr
}

// readFile reads a whole file. Decoders in Go work on the bytes.
func readFile(path string) ([]byte, error) { return os.ReadFile(path) }

// fit returns the size of a w×h picture reduced to cover no more than
// needed of a box, keeping its proportions; the size itself when the box
// is zero or larger.
func fit(w, h, maxW, maxH int) (int, int) {
	if maxW <= 0 || maxH <= 0 || (w <= maxW && h <= maxH) {
		return w, h
	}
	// Scale by the larger of the two ratios that still fits both.
	nw, nh := maxW, h*maxW/w
	if nh > maxH {
		nw, nh = w*maxH/h, maxH
	}
	return max(nw, 1), max(nh, 1)
}
