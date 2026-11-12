package app

import (
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"myimages/internal/codec"
	"myimages/internal/folder"
)

// shot is a file's pixels as one decode gave them, upright: the levels
// of a pyramid, the largest first and each half the one before.
type shot struct {
	levels []*level
	// pix are the pixels of the largest level, w×h of them, for reading
	// colors and copying.
	pix  []byte
	w, h int
	// complete tells that the pixels are all of the file's, not a
	// reduced picture of them.
	complete bool
}

// photo is a file the window shows or may show next, with what was
// decoded of it. Only the main thread touches it.
type photo struct {
	path string
	file folder.File
	// w and h are the picture's size as it shows, upright, 0 until the
	// file was read.
	w, h int
	// orient is how the file's pixels lie; turns and flip are the
	// user's on top of it.
	orient int
	turns  int
	flip   bool
	format string
	via    string
	frames int
	frame  int
	opaque bool
	// vector is set for a drawing, which shows as large as the window
	// allows, and partial for a file of which only a preview shows.
	vector, partial bool
	// preview is a picture about as large as the window, and full all
	// of the file's pixels; one shot is both when the file is no larger.
	preview, full *shot
	err           error
	// loading and loadingFull tell that a decode is under way; gen
	// tells the decodes of a picture since turned from those that count.
	loading, loadingFull bool
	// larger asks for the preview again, of the size the area has now.
	larger bool
	gen    int
	// results holds what decodes finished, until the main thread takes
	// them.
	results chan *decoded
	// dropped is set once the window let go of the photo.
	dropped atomic.Bool
	// anim plays the frames of an animation.
	anim *animation
}

// decoded is what a decode gives the main thread.
type decoded struct {
	gen  int
	full bool
	shot *shot
	pic  *codec.Picture
	err  error
}

// best is the shot with the most pixels.
func (p *photo) best() *shot {
	if p.full != nil {
		return p.full
	}
	return p.preview
}

// box is a size in pixels to decode a preview for.
type box struct{ w, h int }

// photo returns the cached state of a file of the folder, made when
// missing.
func (w *Window) photo(i int) *photo {
	if i < 0 || i >= len(w.files) {
		return nil
	}
	f := w.files[i]
	path := filepath.Join(w.dir, f.Name)
	p := w.photos[path]
	if p == nil || p.file.Size != f.Size || !p.file.Mod.Equal(f.Mod) {
		p = &photo{path: path, file: f, orient: 1, results: make(chan *decoded, 4)}
		w.photos[path] = p
	}
	return p
}

// load starts decoding a photo, unless that is done or under way: its
// preview, or with full all of its pixels.
func (w *Window) load(p *photo, full bool) {
	if p == nil || p.err != nil {
		return
	}
	if full {
		if p.full != nil || p.loadingFull {
			return
		}
		p.loadingFull = true
	} else {
		if (p.preview != nil && !p.larger) || p.full != nil || p.loading {
			return
		}
		p.loading, p.larger = true, false
	}
	var b box
	if !full {
		b = w.previewBox()
	}
	gen, turns, flip, frame := p.gen, p.turns, p.flip, p.frame
	smooth := w.app.set.Smooth
	path := p.path
	results := p.results
	// The picture showing is decoded at once; the others wait their turn.
	prio := 0
	if p == w.current() {
		prio = 2
		if full {
			prio = 1
		}
	}
	w.app.jobs.add(prio, &p.dropped, func() {
		d := &decoded{gen: gen, full: full}
		pic, err := codec.Decode(path, codec.Options{MaxW: b.w, MaxH: b.h, Frame: frame})
		if err != nil {
			d.err = err
		} else {
			d.pic = pic
			d.shot = newShot(pic, turned(pic.Orient, turns, flip), smooth)
		}
		w.app.mark("decoded " + filepath.Base(path))
		results <- d
		w.update(func() { w.take(p) })
		w.app.tidySoon()
	})
}

// loadLarger decodes a photo's preview again, for the area as it is now.
func (w *Window) loadLarger(p *photo) {
	if p.loading || p.full != nil {
		return
	}
	p.larger = true
	w.load(p, false)
}

// newShot makes of a decoded picture what the window draws.
func newShot(pic *codec.Picture, orient int, smooth bool) *shot {
	pix, pw, ph := upright(pic.Pix, pic.W, pic.H, orient)
	s := &shot{pix: pix, w: pw, h: ph, complete: !pic.Reduced()}
	s.levels = append(s.levels, newLevel(pix, pw, ph, s.complete && !smooth))
	if s.complete {
		// Smaller levels, for showing it reduced without shimmer: made
		// here, off the main thread, rather than when first shown so.
		for pw > 2048 || ph > 2048 {
			pix, pw, ph = halve(pix, pw, ph)
			s.levels = append(s.levels, newLevel(pix, pw, ph, false))
		}
	}
	return s
}

// take gives a photo the decodes that finished. It reports whether any
// changed what the photo shows.
func (w *Window) take(p *photo) bool {
	changed := false
	for {
		select {
		case d := <-p.results:
			if d.full {
				p.loadingFull = false
			} else {
				p.loading = false
			}
			if d.gen != p.gen {
				// Decoded before the picture was turned.
				continue
			}
			changed = true
			if d.err != nil {
				if p.best() == nil {
					p.err = d.err
				}
				continue
			}
			pic := d.pic
			p.orient, p.format, p.via, p.frames, p.opaque = pic.Orient, pic.Format, pic.Via, pic.Frames, pic.Opaque
			p.vector = pic.Vector
			if d.full || p.full == nil {
				p.partial = pic.Preview
			}
			pw, ph := pic.FullW, pic.FullH
			if sideways(turned(pic.Orient, p.turns, p.flip)) {
				pw, ph = ph, pw
			}
			if d.full || p.full == nil {
				if d.full && p.w > 0 && pw != p.w && p == w.current() && !w.view.fitted {
					// The file's own pixels are of another size than the
					// preview that showed, as a raw file's: the view
					// stays on the same part of the picture.
					k := float64(pw) / float64(p.w)
					v := &w.view
					v.cx, v.cy, v.ax, v.ay = v.cx*k, v.cy*k, v.ax*k, v.ay*k
					v.zoom /= k
					if v.target > 0 {
						v.target /= k
					}
				}
				p.w, p.h = pw, ph
			}
			if d.shot.complete {
				p.full = d.shot
				if p.preview == nil {
					p.preview = d.shot
				}
			} else if p.preview == nil || d.shot.w > p.preview.w {
				p.preview = d.shot
			}
			if pic.Animated && p.anim == nil {
				p.anim = newAnimation(p.path)
			}
		default:
			if changed && p == w.current() {
				w.shown = p
				w.retitle()
			}
			return changed
		}
	}
}

// await waits for a photo's preview for a moment, on the main thread:
// one that comes in time is in the frame being built, which then shows
// the picture rather than the one before it.
func (w *Window) await(p *photo, limit time.Duration) {
	if p == nil || !p.loading || p.best() != nil {
		return
	}
	select {
	case d := <-p.results:
		p.results <- d
		w.take(p)
	case <-time.After(limit):
	}
}

// previewBox is the size previews are decoded for: the picture area, in
// pixels, or what it was the last time the program ran before the window
// has one.
func (w *Window) previewBox() box {
	if w.viewW > 0 && w.viewH > 0 {
		return box{w.viewW, w.viewH}
	}
	set := &w.app.set
	if set.ViewW > 0 && set.ViewH > 0 {
		return box{set.ViewW, set.ViewH}
	}
	return box{1920, 1080}
}

// How many pictures after and before the one showing are decoded ahead
// and kept. More are kept ahead in the direction the user is going.
const (
	aheadCount  = 3
	behindCount = 1
	keepCount   = 4
)

// preload decodes the previews of the pictures next to the one showing,
// nearest first, and lets go of those far from it.
func (w *Window) preload() {
	n := len(w.files)
	if n == 0 {
		return
	}
	dir := w.heading
	if dir == 0 {
		dir = 1
	}
	keep := map[string]bool{}
	near := func(d int) *photo {
		i := w.cur + d
		if w.app.set.Loop {
			i = ((i % n) + n) % n
		}
		if i < 0 || i >= n || i == w.cur {
			return nil
		}
		return w.photo(i)
	}
	if p := w.current(); p != nil {
		keep[p.path] = true
	}
	for d := 1; d <= keepCount; d++ {
		for _, sign := range [2]int{dir, -dir} {
			p := near(sign * d)
			if p == nil {
				continue
			}
			keep[p.path] = true
			if (sign == dir && d <= aheadCount) || (sign != dir && d <= behindCount) {
				w.load(p, false)
			}
			// Only the picture showing keeps all its pixels.
			if p.full != nil && p.full != p.preview {
				p.full = nil
			}
		}
	}
	for path, p := range w.photos {
		if !keep[path] {
			// What waits to be decoded for it is given up.
			w.drop(p)
		}
	}
}

// drop lets go of a photo: what waits to be decoded for it is given up,
// and its animation stops.
func (w *Window) drop(p *photo) {
	p.dropped.Store(true)
	p.anim.close()
	delete(w.photos, p.path)
	if w.shown == p {
		w.shown = nil
	}
}

// jobs runs decodes on a few threads, the most urgent first and, among
// equals, the newest first: what the user asked for last is what they
// are looking at.
type jobs struct {
	mu      sync.Mutex
	wake    *sync.Cond
	waiting []job
	started bool
}

type job struct {
	prio    int
	dropped *atomic.Bool
	run     func()
}

func (j *jobs) add(prio int, dropped *atomic.Bool, run func()) {
	if prio >= 2 {
		// Nothing is worth making the picture showing wait.
		go run()
		return
	}
	j.mu.Lock()
	if !j.started {
		j.started = true
		j.wake = sync.NewCond(&j.mu)
		for range max(2, min(runtime.NumCPU()-1, 4)) {
			go j.work()
		}
	}
	j.waiting = append(j.waiting, job{prio, dropped, run})
	j.mu.Unlock()
	j.wake.Signal()
}

func (j *jobs) work() {
	for {
		j.mu.Lock()
		for len(j.waiting) == 0 {
			j.wake.Wait()
		}
		best := len(j.waiting) - 1
		for i := best - 1; i >= 0; i-- {
			if j.waiting[i].prio > j.waiting[best].prio {
				best = i
			}
		}
		next := j.waiting[best]
		j.waiting = append(j.waiting[:best], j.waiting[best+1:]...)
		j.mu.Unlock()
		if next.dropped == nil || !next.dropped.Load() {
			next.run()
		}
	}
}

// tidySoon gives the memory that decoding left behind back to the
// system, once no decode has finished for a while.
func (a *App) tidySoon() {
	a.tidyMu.Lock()
	defer a.tidyMu.Unlock()
	if a.tidy != nil {
		a.tidy.Reset(2 * time.Second)
		return
	}
	a.tidy = time.AfterFunc(2*time.Second, debug.FreeOSMemory)
}
