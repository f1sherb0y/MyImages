package app

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
	"myimages/locales"
)

// metadata is what a file says of its picture: the camera's notes.
type metadata struct {
	fields []codec.Field
}

const infoWidth = 250

// infoPanel is the panel beside the picture that tells what it is.
func (w *Window) infoPanel(c *ui.Context) {
	th := c.Theme()
	p := w.current()
	if p != nil && w.metaPath != p.path {
		// Read off the main thread, for the picture showing.
		w.metaPath, w.meta = p.path, nil
		path := p.path
		w.app.jobs.add(1, nil, func() {
			fields := codec.Metadata(path)
			w.update(func() {
				if w.metaPath == path {
					w.meta = &metadata{fields: fields}
				}
			})
		})
	}
	ui.Scroll(c.Key("info")).Width(infoWidth).Shrink(0).Background(th.Surface).Padding(10, 12).Gap(7).Children(func() {
		if p == nil {
			return
		}
		row := func(name, value string) {
			if value == "" {
				return
			}
			ui.Column(c).Gap(1).Children(func() {
				ui.Text(c, name).FontSize(10.5).TextColor(th.TextMuted)
				ui.Text(c, value).Selectable()
			})
		}
		row(locales.InfoName(), p.file.Name)
		row(locales.InfoFolder(), w.dir)
		if p.w > 0 {
			mp := float64(p.w) * float64(p.h) / 1e6
			row(locales.InfoDimensions(), fmt.Sprintf("%d × %d  (%.1f MP)", p.w, p.h, mp))
		}
		if p.file.Size > 0 {
			row(locales.InfoSize(), formatSize(p.file.Size))
		}
		format := p.format
		if p.frames > 1 {
			format += "  ·  " + locales.FrameOf(p.frame+1, p.frames)
		}
		row(locales.InfoFormat(), format)
		if !p.file.Mod.IsZero() {
			row(locales.InfoModified(), p.file.Mod.Format("2006-01-02 15:04:05"))
		}
		if w.meta != nil {
			for _, f := range w.meta.fields {
				row(fieldName(f.Key), f.Value)
			}
		}
		if os.Getenv("MYIMAGES_DEBUG") != "" {
			row("Decoder", p.via)
		}
	})
}

// fieldName is what the user reads for a field of a file's metadata.
func fieldName(key string) string {
	switch key {
	case "taken":
		return locales.InfoTaken()
	case "camera":
		return locales.InfoCamera()
	case "lens":
		return locales.InfoLens()
	case "exposure":
		return locales.InfoExposure()
	case "aperture":
		return locales.InfoAperture()
	case "iso":
		return "ISO"
	case "focal":
		return locales.InfoFocal()
	case "flash":
		return locales.InfoFlash()
	case "location":
		return locales.InfoLocation()
	case "software":
		return locales.InfoSoftware()
	case "title":
		return locales.InfoTitle()
	case "author":
		return locales.InfoAuthor()
	case "copyright":
		return locales.InfoCopyright()
	case "color":
		return locales.InfoColor()
	}
	return strings.ToUpper(key[:1]) + key[1:]
}

// bench measures how long pictures take to show: the first, from the
// start of the process, and those after it, from the step to them.
type bench struct {
	w       *Window
	steps   int
	stepAt  time.Time
	waiting bool
	times   []time.Duration
	first   time.Duration
	done    bool
}

func newBench(w *Window) *bench { return &bench{w: w} }

// frame runs as each frame is built.
func (b *bench) frame(c *ui.Context) {
	w := b.w
	if b.done {
		return
	}
	cur := w.current()
	if cur == nil || cur.best() == nil {
		if cur != nil && cur.err != nil && b.waiting {
			// Not one we can show: on to the next.
			b.waiting = false
		} else {
			return
		}
	}
	now := time.Now()
	if b.first == 0 {
		b.first = now.Sub(w.app.started)
	} else if b.waiting {
		b.times = append(b.times, now.Sub(b.stepAt))
	}
	b.waiting = false
	if b.steps >= min(40, len(w.files)-1) {
		if len(w.files) < 2 && w.listing > 0 && b.steps == 0 && now.Sub(w.firstAt) < time.Second {
			// The folder is still being read.
			c.After(20 * time.Millisecond)
			return
		}
		b.done = true
		b.report()
		return
	}
	// The next step comes once this frame showed, and a little later:
	// as a person goes through pictures, not faster than they show.
	time.AfterFunc(60*time.Millisecond, func() {
		w.update(func() {
			if b.done || b.waiting {
				return
			}
			b.steps++
			b.waiting = true
			b.stepAt = time.Now()
			w.step(1)
		})
	})
}

func (b *bench) report() {
	fmt.Printf("first picture: %.1f ms after the process started\n", ms(b.first))
	if n := len(b.times); n > 0 {
		var sum, worst time.Duration
		for _, t := range b.times {
			sum += t
			worst = max(worst, t)
		}
		fmt.Printf("next picture:  %.1f ms on average over %d, %.1f ms at worst\n", ms(sum)/float64(n), n, ms(worst))
	}
	os.Stdout.Sync()
	time.AfterFunc(50*time.Millisecond, func() { os.Exit(0) })
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// screenshotSoon writes a picture of the window to a file once it has
// shown its picture whole, then quits.
func (w *Window) screenshotSoon(path string) {
	delay := 1500 * time.Millisecond
	if ms, err := strconv.Atoi(os.Getenv("MYIMAGES_SHOT_DELAY")); err == nil && ms > 0 {
		delay = time.Duration(ms) * time.Millisecond
	}
	time.AfterFunc(delay, func() {
		w.update(func() {
			data, err := w.win.CapturePage()
			if err == nil {
				err = os.WriteFile(path, data, 0o644)
			}
			if err != nil {
				fmt.Fprintln(os.Stderr, "myimages:", err)
				os.Exit(1)
			}
			os.Exit(0)
		})
	})
}
