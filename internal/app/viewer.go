package app

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// viewState is how the picture lies in its area.
type viewState struct {
	// zoom is the pixels of the screen a pixel of the picture takes, and
	// cx, cy the point of the picture at the middle of the area.
	zoom   float64
	cx, cy float64
	// fitted has the zoom follow the area, as the settings' mode says,
	// until the user zooms or moves the picture.
	fitted bool
	// target, unless 0, is the zoom the picture is easing to, about the
	// point of the picture (ax, ay) that stays at (sx, sy) pixels from
	// the middle of the area; at is when it last moved.
	target         float64
	ax, ay, sx, sy float64
	at             time.Time
	// dragging is set while the pointer moves the picture, from (px, py).
	dragging bool
	px, py   float32
	moved    bool
	// hover is the pointer over the area, in DIPs, when over is set.
	hx, hy float32
	over   bool
}

const (
	minZoom = 0.01
	maxZoom = 64
	// zoomStep is what a notch of the wheel, or a press of + or -,
	// zooms by.
	zoomStep = 1.2
	// zoomEase is how fast the zoom reaches its target.
	zoomEase = 45 * time.Millisecond
)

// fitZoom is the zoom at which a picture first shows in an area of
// pixels, as the settings' mode says.
func (w *Window) fitZoom(iw, ih int) float64 {
	if iw <= 0 || ih <= 0 || w.viewW <= 0 || w.viewH <= 0 {
		return 1
	}
	zx, zy := float64(w.viewW)/float64(iw), float64(w.viewH)/float64(ih)
	mode := w.app.set.Zoom
	if p := w.drawn(); p != nil && p.vector && mode == "auto" {
		// A drawing has no size of its own to keep to.
		mode = "fit"
	}
	switch mode {
	case "fit":
		return min(zx, zy)
	case "fill":
		return max(zx, zy)
	case "actual":
		return 1
	}
	return min(zx, zy, 1)
}

// place settles the view for a picture of a size: the zoom follows the
// area while fitted, eases to its target, and the picture stays within
// the area or centered in it.
func (w *Window) place(iw, ih int, now time.Time) (animating bool) {
	v := &w.view
	if v.fitted {
		v.zoom, v.cx, v.cy, v.target = w.fitZoom(iw, ih), float64(iw)/2, float64(ih)/2, 0
	}
	if v.zoom <= 0 {
		v.zoom = 1
	}
	if v.target > 0 {
		dt := now.Sub(v.at)
		v.at = now
		k := 1 - math.Exp(-float64(dt)/float64(zoomEase))
		v.zoom *= math.Pow(v.target/v.zoom, k)
		if math.Abs(math.Log(v.target/v.zoom)) < 0.003 {
			v.zoom, v.target = v.target, 0
		} else {
			animating = true
		}
		v.cx, v.cy = v.ax-v.sx/v.zoom, v.ay-v.sy/v.zoom
	}
	// A picture smaller than the area stays in its middle; a larger one
	// keeps the area covered.
	halfW, halfH := float64(w.viewW)/2/v.zoom, float64(w.viewH)/2/v.zoom
	if float64(iw) <= 2*halfW {
		v.cx = float64(iw) / 2
	} else {
		v.cx = min(max(v.cx, halfW), float64(iw)-halfW)
	}
	if float64(ih) <= 2*halfH {
		v.cy = float64(ih) / 2
	} else {
		v.cy = min(max(v.cy, halfH), float64(ih)-halfH)
	}
	return animating
}

// zoomTo eases the zoom to z about a point of the area, (sx, sy) pixels
// from its middle; at once when instant.
func (w *Window) zoomTo(z float64, sx, sy float64, instant bool) {
	p := w.drawn()
	if p == nil || p.w == 0 {
		return
	}
	v := &w.view
	z = min(max(z, min(minZoom, w.fitZoom(p.w, p.h))), maxZoom)
	// The point of the picture under (sx, sy) now stays there.
	v.ax, v.ay = v.cx+sx/v.zoom, v.cy+sy/v.zoom
	v.sx, v.sy = sx, sy
	v.fitted = false
	if instant {
		v.zoom, v.target = z, 0
		v.cx, v.cy = v.ax-sx/z, v.ay-sy/z
	} else {
		if v.target == 0 {
			v.at = time.Now()
		}
		v.target = z
	}
	w.invalidate()
}

// zoomBy zooms by a factor from where the zoom is going, stopping at
// the picture's own size on the way past it.
func (w *Window) zoomBy(f float64, sx, sy float64, instant bool) {
	v := &w.view
	from := v.zoom
	if v.target > 0 {
		from = v.target
	}
	z := from * f
	if (from < 1 && z > 1) || (from > 1 && z < 1) {
		z = 1
	}
	w.zoomTo(z, sx, sy, instant)
}

// fit shows the picture whole again, following the area.
func (w *Window) fit() {
	w.view.fitted, w.view.target = true, 0
	w.invalidate()
}

// drawn is the photo the area shows: the current one, or the one before
// it until the current one has pixels.
func (w *Window) drawn() *photo {
	if p := w.current(); p != nil && p.best() != nil {
		return p
	}
	if w.shown != nil && w.shown.best() != nil && len(w.files) > 0 {
		return w.shown
	}
	return nil
}

// pick chooses, of a photo's levels, the smallest that has a pixel for
// every pixel of the screen at a zoom, or the largest there is. It
// returns the level and the picture's pixels a pixel of it covers.
func pick(p *photo, zoom float64) (*level, float64) {
	var best *level
	var bestPer float64
	consider := func(s *shot) {
		if s == nil {
			return
		}
		for _, l := range s.levels {
			per := float64(p.w) / float64(l.w)
			if p.w == 0 {
				per = 1
			}
			switch {
			case best == nil:
			case zoom*per <= 1.001 && zoom*bestPer <= 1.001:
				// Both have enough pixels: the smaller.
				if per <= bestPer {
					continue
				}
			case per >= bestPer:
				// Not both enough: the larger.
				continue
			}
			best, bestPer = l, per
		}
	}
	consider(p.preview)
	if p.full != p.preview {
		consider(p.full)
	}
	return best, bestPer
}

// paint draws the picture area.
func (w *Window) paint(p *ui.Painter, r ui.Rect) {
	s := p.Scale()
	w.scale = s
	vw, vh := int(math.Round(float64(r.W*s))), int(math.Round(float64(r.H*s)))
	if vw != w.viewW || vh != w.viewH {
		w.viewW, w.viewH = vw, vh
		if set := &w.app.set; !w.fullscreen && (set.ViewW != vw || set.ViewH != vh) {
			set.ViewW, set.ViewH = vw, vh
			w.app.saveSoon()
		}
	}
	th := p.Theme()
	bg := th.Background
	switch w.app.set.Background {
	case "black":
		bg = ui.RGB(0, 0, 0)
	case "white":
		bg = ui.RGB(255, 255, 255)
	}
	if w.fullscreen || w.slideshow {
		bg = ui.RGB(0, 0, 0)
	}
	p.Fill(r, bg, 0)

	ph := w.drawn()
	if ph == nil || ph.w == 0 {
		return
	}
	if w.place(ph.w, ph.h, p.Now()) {
		p.AnimationFrame()
	}
	v := &w.view
	lvl, per := pick(ph, v.zoom)
	if lvl == nil {
		return
	}
	if ph.full == nil && ph == w.current() && max(v.zoom, v.target)*per > 1.02 {
		// Shown larger than the pixels decoded for the window: now all
		// of the file's are worth decoding. Until then, most pictures
		// are looked at and left, and cost a fraction of the memory.
		w.load(ph, true)
	}
	// The picture's corner in pixels of the area, on a whole pixel.
	ox := math.Round(float64(vw)/2 - v.cx*v.zoom)
	oy := math.Round(float64(vh)/2 - v.cy*v.zoom)
	k := v.zoom * per // pixels of the screen a pixel of the level takes
	dip := func(px float64) float32 { return float32(px) / s }
	whole := ui.Rect{X: r.X + dip(ox), Y: r.Y + dip(oy), W: dip(float64(ph.w) * v.zoom), H: dip(float64(ph.h) * v.zoom)}
	p.Clip(r, 0, func() {
		if !ph.opaque {
			w.checker(p, whole, r)
		}
		if ph.anim != nil {
			ph.anim.tick(w, p)
			if ph.anim.playing(ph) {
				p.ImageRegion(ph.anim.bm, ui.Rect{}, whole, ui.ImageOptions{})
				return
			}
		}
		for i := range lvl.tiles {
			t := &lvl.tiles[i]
			x0, y0 := ox+float64(t.x)*k, oy+float64(t.y)*k
			x1, y1 := ox+float64(t.x+t.w)*k, oy+float64(t.y+t.h)*k
			if t.x+t.w == lvl.w {
				x1 = ox + float64(ph.w)*v.zoom
			}
			if t.y+t.h == lvl.h {
				y1 = oy + float64(ph.h)*v.zoom
			}
			dst := ui.Rect{X: r.X + dip(x0), Y: r.Y + dip(y0), W: dip(x1 - x0), H: dip(y1 - y0)}
			p.ImageRegion(t.bm, t.src, dst, ui.ImageOptions{})
		}
	})
}

// checker draws the squares that show through a picture's transparent
// parts, where the picture is in the area.
func (w *Window) checker(p *ui.Painter, pic, area ui.Rect) {
	x0, y0 := max(pic.X, area.X), max(pic.Y, area.Y)
	x1, y1 := min(pic.X+pic.W, area.X+area.W), min(pic.Y+pic.H, area.Y+area.H)
	if x1 <= x0 || y1 <= y0 {
		return
	}
	light, dark := ui.RGB(255, 255, 255), ui.RGB(204, 204, 204)
	if p.Theme().Dark {
		light, dark = ui.RGB(72, 72, 72), ui.RGB(52, 52, 52)
	}
	p.Fill(ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, light, 0)
	const cell = 8
	p.Clip(ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}, 0, func() {
		// The squares stay where they are on the picture as it moves.
		for row, y := 0, pic.Y+float32(math.Floor(float64((y0-pic.Y)/cell)))*cell; y < y1; row, y = row+1, y+cell {
			first := int(math.Floor(float64((y - pic.Y) / cell)))
			for x := pic.X + float32(math.Floor(float64((x0-pic.X)/cell)))*cell; x < x1; x += cell {
				col := int(math.Floor(float64((x - pic.X) / cell)))
				if (first+col)%2 != 0 {
					p.Fill(ui.Rect{X: x, Y: y, W: cell, H: cell}, dark, 0)
				}
			}
		}
	})
}

// input takes the pointer and the wheel over the picture area.
func (w *Window) input(ev ui.InputEvent) bool {
	v := &w.view
	s := float64(w.scale)
	if s == 0 {
		s = 1
	}
	// The pointer in pixels from the middle of the area.
	sx := float64(ev.X)*s - float64(w.viewW)/2
	sy := float64(ev.Y)*s - float64(w.viewH)/2
	switch ev.Kind {
	case ui.InputScroll:
		if ev.Precise {
			// A touchpad moves the picture.
			if ev.Mods&ui.Ctrl != 0 {
				w.zoomBy(math.Exp(-float64(ev.DY)/120), sx, sy, true)
				return true
			}
			if !w.pannable() {
				return false
			}
			w.panBy(float64(ev.DX)*s, float64(ev.DY)*s)
			return true
		}
		if ev.DY == 0 {
			return false
		}
		browse := w.app.set.WheelBrowses
		if ev.Mods&ui.Ctrl != 0 {
			browse = !browse
		}
		if browse {
			if ev.DY > 0 {
				w.step(1)
			} else {
				w.step(-1)
			}
			return true
		}
		// A notch scrolls about 100 DIPs, more for a wheel turned fast.
		notches := min(max(-float64(ev.DY)/100, -4), 4)
		w.zoomBy(math.Pow(zoomStep, notches), sx, sy, false)
		return true
	case ui.InputZoom:
		w.zoomBy(float64(ev.Scale), sx, sy, true)
		return true
	case ui.InputPointerDown:
		switch ev.Button {
		case 0:
			if ev.Clicks == 2 {
				// Between whole and its own size, about the pointer.
				if v.fitted && math.Abs(v.zoom-1) > 0.001 {
					w.zoomTo(1, sx, sy, false)
				} else {
					w.fit()
				}
				return true
			}
			v.dragging, v.moved, v.px, v.py = true, false, ev.X, ev.Y
			return true
		case 2:
			w.fit()
			return true
		}
	case ui.InputPointerMove:
		v.hx, v.hy, v.over = ev.X, ev.Y, true
		if !v.dragging {
			if w.app.set.Status {
				w.invalidate()
			}
			return false
		}
		if ev.Button != 0 {
			v.dragging = false
			return false
		}
		dx, dy := float64(ev.X-v.px)*s, float64(ev.Y-v.py)*s
		v.px, v.py = ev.X, ev.Y
		if w.pannable() {
			w.panBy(-dx, -dy)
		}
		return true
	case ui.InputPointerUp:
		if v.dragging {
			v.dragging = false
			return true
		}
	}
	return false
}

// pannable tells whether the picture is larger than its area.
func (w *Window) pannable() bool {
	p := w.drawn()
	if p == nil {
		return false
	}
	z := w.view.zoom
	return float64(p.w)*z > float64(w.viewW)+0.5 || float64(p.h)*z > float64(w.viewH)+0.5
}

// panBy moves the area over the picture by pixels of the screen.
func (w *Window) panBy(dx, dy float64) {
	v := &w.view
	v.fitted, v.target = false, 0
	v.cx += dx / v.zoom
	v.cy += dy / v.zoom
	w.invalidate()
}

// pixelAt reads the color of the picture's pixel under a point of the
// area, in DIPs.
func (w *Window) pixelAt(x, y float32) (px, py int, c [4]uint8, ok bool) {
	p := w.drawn()
	if p == nil || p.w == 0 || w.view.zoom == 0 {
		return
	}
	v := &w.view
	s := float64(w.scale)
	fx := v.cx + (float64(x)*s-float64(w.viewW)/2)/v.zoom
	fy := v.cy + (float64(y)*s-float64(w.viewH)/2)/v.zoom
	if fx < 0 || fy < 0 || fx >= float64(p.w) || fy >= float64(p.h) {
		return
	}
	px, py = int(fx), int(fy)
	sh := p.best()
	ix, iy := px*sh.w/p.w, py*sh.h/p.h
	if ix >= sh.w || iy >= sh.h {
		return
	}
	i := (iy*sh.w + ix) * 4
	a := uint32(sh.pix[i+3])
	if a == 0 {
		return px, py, [4]uint8{}, true
	}
	// The pixels are premultiplied: the color as the file has it.
	un := func(v uint8) uint8 { return uint8(min(uint32(v)*255/a, 255)) }
	return px, py, [4]uint8{un(sh.pix[i]), un(sh.pix[i+1]), un(sh.pix[i+2]), uint8(a)}, true
}
