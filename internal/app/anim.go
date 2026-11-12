package app

import (
	"bytes"
	"errors"
	"image"
	"image/draw"
	"io"
	"os"
	"time"

	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
)

// animation plays the frames of an animated picture, as a GIF's: one
// bitmap, which takes each frame as it comes due. The frames are decoded
// on another thread, a couple ahead.
type animation struct {
	path string
	// frames brings the frames decoded, in order, again and again; stop
	// ends the decoding. started is set once it was asked for.
	frames  chan animFrame
	stop    chan struct{}
	started bool
	// still is set for a file that turned out to hold one frame, or none
	// we can play.
	still bool
	// bm shows the frame; due is when the next one comes.
	bm  *ui.Bitmap
	due time.Time
}

type animFrame struct {
	pix   *image.RGBA
	delay time.Duration
	// last marks the frame after which the file had no more: one that is
	// also the first is no animation.
	only bool
}

// animBuffers is how many frames' pixels go around: those waiting, the
// one being decoded, the one showing, and one to spare.
const animBuffers = 5

func newAnimation(path string) *animation { return &animation{path: path} }

// playing reports whether the animation has a frame to show in place of
// the still picture.
func (a *animation) playing(p *photo) bool {
	return a != nil && a.bm != nil && !a.still && p.turns == 0 && !p.flip
}

// close ends the decoding.
func (a *animation) close() {
	if a != nil && a.stop != nil {
		close(a.stop)
		a.stop = nil
	}
}

// tick starts the decoding when first drawn, and from then on shows the
// frame that is due, asking for the window to be painted again when the
// next one is.
func (a *animation) tick(w *Window, p *ui.Painter) {
	if a.still {
		return
	}
	if !a.started {
		a.started = true
		a.frames, a.stop = make(chan animFrame, 2), make(chan struct{})
		go a.decode(w, a.frames, a.stop)
	}
	now := p.Now()
	if a.bm != nil && now.Before(a.due) {
		p.After(a.due.Sub(now))
		return
	}
	select {
	case f, ok := <-a.frames:
		if !ok || f.only {
			a.still = true
			return
		}
		if a.bm == nil || func() bool { bw, bh := a.bm.Size(); return bw != f.pix.Rect.Dx() || bh != f.pix.Rect.Dy() }() {
			a.bm = ui.NewBitmap(f.pix)
			if !w.app.set.Smooth {
				a.bm.Pixelated()
			}
		}
		a.bm.Update(f.pix)
		d := f.delay
		if d <= 10*time.Millisecond {
			d = 100 * time.Millisecond
		}
		// Late frames keep the pace rather than drift.
		if a.due.IsZero() || now.Sub(a.due) > d {
			a.due = now
		}
		a.due = a.due.Add(d)
		p.After(a.due.Sub(now))
	default:
		// Not decoded yet: the frame showing stays a little longer. The
		// decoder draws the window again when it has one.
	}
}

// decode sends the file's frames, again from the first after the last,
// until stop closes.
func (a *animation) decode(w *Window, frames chan<- animFrame, stop <-chan struct{}) {
	defer close(frames)
	dec, err := openAnimation(a.path)
	if err != nil {
		return
	}
	var ring [animBuffers]*image.RGBA
	n := 0
	send := func(f animFrame) bool {
		select {
		case frames <- f:
			// The frame showing may be waiting for this one.
			w.invalidate()
			return true
		case <-stop:
			return false
		}
	}
	for count := 0; ; {
		frame, err := dec.Next()
		if errors.Is(err, io.EOF) {
			if count < 2 {
				send(animFrame{only: true})
				return
			}
			if dec.Rewind() != nil {
				return
			}
			continue
		}
		if err != nil || frame.Image == nil {
			return
		}
		count++
		// The decoder draws the next frame into the image it gave: the
		// window gets a copy, in pixels that go around.
		b := frame.Image.Bounds()
		buf := ring[n%animBuffers]
		if buf == nil || buf.Rect.Dx() != b.Dx() || buf.Rect.Dy() != b.Dy() {
			buf = image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
			ring[n%animBuffers] = buf
		}
		n++
		if src, ok := frame.Image.(*image.RGBA); ok && len(src.Pix) == len(buf.Pix) {
			copy(buf.Pix, src.Pix)
		} else {
			draw.Draw(buf, buf.Rect, frame.Image, b.Min, draw.Src)
		}
		if !send(animFrame{pix: buf, delay: frame.Duration}) {
			return
		}
	}
}

// openAnimation reads a file's frames: a GIF's or an animated PNG's with
// MyGo's decoders, which compose them, and those of other formats as the
// system's codecs give them.
func openAnimation(path string) (ui.VideoDecoder, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	var head [12]byte
	_, _ = io.ReadFull(f, head[:])
	f.Close()
	switch {
	case bytes.HasPrefix(head[:], []byte("GIF8")):
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return ui.NewGIFDecoder(data)
	case bytes.HasPrefix(head[:], []byte("\x89PNG")):
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return ui.NewAPNGDecoder(data)
	case string(head[:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		frames, delays, err := codec.WebPFrames(data)
		if err != nil {
			return nil, err
		}
		return &frameList{frames: frames, delays: delays}, nil
	}
	return &frameSequence{path: path}, nil
}

// frameList plays frames decoded already.
type frameList struct {
	frames []image.Image
	delays []time.Duration
	next   int
}

func (l *frameList) Next() (ui.VideoFrame, error) {
	if l.next >= len(l.frames) {
		return ui.VideoFrame{}, io.EOF
	}
	i := l.next
	l.next++
	return ui.VideoFrame{Image: l.frames[i], Duration: l.delays[i]}, nil
}

func (l *frameList) Rewind() error {
	l.next = 0
	return nil
}

// frameSequence plays a file's frames as the codecs decode them, one at
// a time.
type frameSequence struct {
	path  string
	next  int
	count int
}

func (s *frameSequence) Next() (ui.VideoFrame, error) {
	if s.count > 0 && s.next >= s.count {
		return ui.VideoFrame{}, io.EOF
	}
	pic, err := codec.Decode(s.path, codec.Options{Frame: s.next})
	if err != nil {
		return ui.VideoFrame{}, err
	}
	s.count = pic.Frames
	s.next++
	return ui.VideoFrame{Image: pic.RGBA(), Duration: pic.Delay}, nil
}

func (s *frameSequence) Rewind() error {
	s.next = 0
	return nil
}
