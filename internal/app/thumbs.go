package app

import (
	"image"
	"math"
	"path/filepath"
	"sync/atomic"

	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
)

// thumbStrip is the row of small pictures of the folder under the
// picture. It is painted, not built of elements: a folder of ten
// thousand pictures costs what shows.
type thumbStrip struct {
	// scroll is how far the row is scrolled, in DIPs, and follow has it
	// bring the picture showing into view at the next frame.
	scroll float32
	follow bool
	// cache holds the thumbnails by path.
	cache map[string]*thumb
	// width is the strip's, as last painted; hover the index under the
	// pointer, -1 for none.
	width float32
	hover int
	// press is where a press began, to tell a click from a drag.
	pressing bool
	pressX   float32
	dragged  bool
}

type thumb struct {
	bm      *ui.Bitmap
	w, h    int
	failed  bool
	dropped atomic.Bool
}

const (
	thumbGap = 3
	thumbPad = 4
	// thumbsKept is how many thumbnails stay in memory.
	thumbsKept = 800
)

// build makes the strip's element.
func (s *thumbStrip) build(c *ui.Context, w *Window) {
	th := c.Theme()
	size := w.app.set.ThumbSize
	e := ui.Box(c.Key("thumbs")).Height(size + 2*thumbPad).Shrink(0).Background(th.Surface).Clip()
	e.Draw(func(p *ui.Painter, r ui.Rect) { s.paint(p, r, w) })
	e.HandleInput(func(ev ui.InputEvent) bool { return s.input(ev, w) })
}

// cell is the width a thumbnail takes with its gap.
func (s *thumbStrip) cell(w *Window) float32 { return w.app.set.ThumbSize + thumbGap }

// limit keeps the scroll within the row.
func (s *thumbStrip) limit(w *Window) {
	total := float32(len(w.files))*s.cell(w) - thumbGap + 2*thumbPad
	s.scroll = max(min(s.scroll, total-s.width), 0)
}

func (s *thumbStrip) paint(p *ui.Painter, r ui.Rect, w *Window) {
	th := p.Theme()
	size := w.app.set.ThumbSize
	cell := s.cell(w)
	s.width = r.W
	if s.follow {
		// The picture showing, in the middle where the row allows.
		s.follow = false
		s.scroll = thumbPad + float32(w.cur)*cell + size/2 - r.W/2
	}
	s.limit(w)
	if s.cache == nil {
		s.cache = map[string]*thumb{}
	}
	first := max(int((s.scroll-thumbPad)/cell), 0)
	last := min(int((s.scroll+r.W-thumbPad)/cell), len(w.files)-1)
	px := int(math.Ceil(float64(size * p.Scale())))
	for i := first; i <= last; i++ {
		box := ui.Rect{X: r.X + thumbPad + float32(i)*cell - s.scroll, Y: r.Y + thumbPad, W: size, H: size}
		t := s.thumb(w, i, px)
		if t.bm != nil {
			// The middle square of the picture.
			src := ui.Rect{W: float32(t.w), H: float32(t.h)}
			if t.w > t.h {
				src.X, src.W = float32(t.w-t.h)/2, float32(t.h)
			} else {
				src.Y, src.H = float32(t.h-t.w)/2, float32(t.w)
			}
			p.ImageRegion(t.bm, src, box, ui.ImageOptions{})
		} else {
			p.Fill(box, th.Background, 2)
		}
		switch {
		case i == w.cur:
			p.Stroke(ui.Rect{X: box.X - 1.5, Y: box.Y - 1.5, W: box.W + 3, H: box.H + 3}, th.Accent, 3, 2)
		case i == s.hover:
			p.Stroke(ui.Rect{X: box.X - 1, Y: box.Y - 1, W: box.W + 2, H: box.H + 2}, th.TextMuted, 2, 1)
		}
	}
	// Those far from what shows are let go.
	if len(s.cache) > thumbsKept {
		for path, t := range s.cache {
			i := w.index(filepath.Base(path))
			if i < first-thumbsKept/3 || i > last+thumbsKept/3 {
				t.dropped.Store(true)
				delete(s.cache, path)
			}
		}
	}
}

// thumb returns the thumbnail of the picture at an index, asking for it
// to be made when it was not.
func (s *thumbStrip) thumb(w *Window, i, px int) *thumb {
	path := filepath.Join(w.dir, w.files[i].Name)
	if t := s.cache[path]; t != nil {
		return t
	}
	t := &thumb{}
	s.cache[path] = t
	w.app.jobs.add(-1, &t.dropped, func() {
		pic, err := codec.Thumbnail(path, px)
		if err != nil {
			w.update(func() { t.failed = true })
			return
		}
		pix, tw, th := upright(pic.Pix, pic.W, pic.H, pic.Orient)
		bm := ui.NewBitmap(&image.RGBA{Pix: pix, Stride: 4 * tw, Rect: image.Rect(0, 0, tw, th)})
		w.update(func() { t.bm, t.w, t.h = bm, tw, th })
	})
	return t
}

// index finds a file of the folder by name.
func (w *Window) index(name string) int {
	for i := range w.files {
		if w.files[i].Name == name {
			return i
		}
	}
	return -1
}

func (s *thumbStrip) at(x float32, w *Window) int {
	i := int((x + s.scroll - thumbPad) / s.cell(w))
	if x+s.scroll < thumbPad || i < 0 || i >= len(w.files) {
		return -1
	}
	return i
}

func (s *thumbStrip) input(ev ui.InputEvent, w *Window) bool {
	switch ev.Kind {
	case ui.InputScroll:
		// The wheel goes along the row.
		d := ev.DX
		if d == 0 {
			d = ev.DY
		}
		s.scroll += d
		s.limit(w)
		w.invalidate()
		return true
	case ui.InputPointerDown:
		if ev.Button != 0 {
			return false
		}
		s.pressing, s.pressX, s.dragged = true, ev.X, false
		return true
	case ui.InputPointerMove:
		if s.pressing && ev.Button == 0 {
			if d := ev.X - s.pressX; s.dragged || d > 4 || d < -4 {
				s.dragged = true
				s.scroll -= d
				s.pressX = ev.X
				s.limit(w)
				w.invalidate()
			}
			return true
		}
		s.pressing = false
		if i := s.at(ev.X, w); i != s.hover {
			s.hover = i
			w.invalidate()
		}
	case ui.InputPointerUp:
		if s.pressing {
			s.pressing = false
			if !s.dragged && !ev.Canceled {
				if i := s.at(ev.X, w); i >= 0 {
					w.goTo(i, i-w.cur)
					s.follow = false
				}
			}
			return true
		}
	}
	return false
}
