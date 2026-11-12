package app

import (
	_ "embed"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"myimages/internal/folder"
)

// iconPNG is the program's icon, which its windows show in the taskbar.
//
//go:embed icon.png
var iconPNG []byte

// Window is a window: a folder's pictures, one of them showing.
type Window struct {
	app *App
	// win is the native window, nil until the window is shown; mu
	// guards it and pending, what goroutines asked to run before.
	win     *mygo.Window
	mu      sync.Mutex
	pending []func()

	// dir is the folder, files its pictures in order, and cur the one
	// showing. listing counts the readings of folders, to tell a late
	// one from the one that counts.
	dir     string
	files   []folder.File
	cur     int
	listing int
	// reading counts the folders being read.
	reading int
	// photos is what was decoded, by path, and shown the photo last
	// drawn with pixels: it stays until the next one has some.
	photos map[string]*photo
	shown  *photo
	// heading is the way the user last went, 1 forward or -1 back.
	heading int
	// nav counts the pictures shown, to tell whether one still shows
	// once a timer for it fires.
	nav int

	view viewState

	// viewW and viewH are the picture area's size in pixels, and scale
	// the pixels of a DIP, as last drawn.
	viewW, viewH int
	scale        float32

	fullscreen bool
	// slideshow is set while pictures follow each other on their own.
	slideshow bool
	// message shows over the picture for a moment, until messageUntil.
	message      string
	messageUntil time.Time
	// ask is the question in the window, nil for none.
	ask *question
	// settings is set while the settings show, and renaming while a
	// name is asked for, newName.
	settings bool
	renaming bool
	newName  string
	// slideOwnsScreen tells that the slideshow filled the screen, and
	// gives the window back when it ends.
	slideOwnsScreen bool
	// info is what the info panel shows of the picture.
	meta     *metadata
	metaPath string
	thumbs   thumbStrip
	// timing
	benchRun *bench
	firstAt  time.Time
}

// question is something asked over the window, with what to do on yes.
type question struct {
	open           bool
	title, message string
	confirm        string
	yes            func()
}

// window makes a window's state, showing a file or nothing, before it
// has a native window to draw in.
func (a *App) window(path string) *Window {
	w := &Window{app: a, photos: map[string]*photo{}, cur: -1}
	w.view.fitted = true
	a.windows = append(a.windows, w)
	if path != "" {
		w.openPath(path)
	}
	return w
}

// show gives a window its native window, on screen.
func (a *App) show(w *Window) {
	if w.win != nil {
		return
	}
	opts := mygo.WindowOptions{
		Title:          "MyImages",
		Width:          1100,
		Height:         760,
		MinWidth:       360,
		MinHeight:      240,
		TitleBarStyle:  mygo.TitleBarHidden,
		TitleBarHeight: stripHeight,
		Content:        ui.View(w.build),
	}
	if len(a.windows) == 1 && !a.opt.Ephemeral && !a.opt.Bench && a.opt.Screenshot == "" {
		opts.StateKey = "main"
	}
	win := mygo.NewWindow(opts)
	// The icon is no part of the first frame: set once that is out.
	go func() { _ = win.SetIcon(iconPNG) }()
	win.OnFocus(func() {
		// The window used last is the one pictures open in.
		a.remove(w)
		a.windows = append(a.windows, w)
	})
	win.OnEnterFullScreen(func() { w.fullscreen = true; w.invalidate() })
	win.OnLeaveFullScreen(func() { w.fullscreen = false; w.invalidate() })
	win.OnClosed(func() {
		for _, p := range w.photos {
			w.drop(p)
		}
		a.remove(w)
	})
	w.mu.Lock()
	w.win = win
	pending := w.pending
	w.pending = nil
	w.mu.Unlock()
	for _, fn := range pending {
		win.Update(fn)
	}
	w.retitle()
	if a.opt.Bench {
		w.benchRun = newBench(w)
	}
	if a.opt.Screenshot != "" {
		w.screenshotSoon(a.opt.Screenshot)
	}
}

// update runs fn on the main thread and draws the window again, from
// any goroutine. Before the window has a native window, fn waits for
// it.
func (w *Window) update(fn func()) {
	w.mu.Lock()
	win := w.win
	if win == nil {
		w.pending = append(w.pending, fn)
	}
	w.mu.Unlock()
	if win != nil {
		win.Update(fn)
	}
}

// invalidate draws the window again.
func (w *Window) invalidate() {
	w.mu.Lock()
	win := w.win
	w.mu.Unlock()
	if win != nil {
		win.Invalidate()
	}
}

// raise brings the window to the front.
func (w *Window) raise() {
	if w.win == nil {
		return
	}
	if w.win.IsMinimized() {
		w.win.Restore()
	}
	w.win.Show()
	w.win.Focus()
}

// current is the photo showing, nil for none.
func (w *Window) current() *photo {
	if w.cur < 0 || w.cur >= len(w.files) {
		return nil
	}
	return w.photo(w.cur)
}

// openPath shows a file, with the pictures of its folder to go through,
// or the first picture of a folder.
func (w *Window) openPath(path string) {
	path = filepath.Clean(path)
	dir, name := filepath.Dir(path), filepath.Base(path)
	if isDir(path) {
		dir, name = path, ""
	}
	w.listing++
	w.reading++
	listing := w.listing
	set := &w.app.set
	order := folder.Order{By: set.SortBy, Descending: set.SortDescending}
	hidden := set.ShowHidden
	if name != "" {
		// The file shows at once, alone, while its folder is read.
		w.dir = dir
		w.files = []folder.File{{Name: name}}
		w.cur = 0
		w.shownAt(0)
	}
	go func() {
		files, err := folder.List(dir, known, hidden, order)
		w.update(func() {
			w.reading--
			if listing != w.listing {
				return
			}
			if err != nil && name == "" {
				w.say(err.Error())
				return
			}
			i := folder.Index(files, name)
			if i < 0 && name != "" {
				// A file of a type that is none of ours, or hidden: shown
				// all the same, since it was asked for.
				files = append(files, folder.File{Name: name})
				folder.Sort(files, order)
				i = folder.Index(files, name)
			}
			old := w.current()
			w.dir, w.files = dir, files
			if name == "" {
				if len(files) == 0 {
					w.cur = -1
					w.retitle()
					return
				}
				w.cur = 0
				w.shownAt(0)
				return
			}
			w.cur = max(i, 0)
			if old != nil {
				// The photo decoded while the folder was read is the one
				// in the list now.
				old.file = files[w.cur]
				w.photos[old.path] = old
			}
			w.preload()
			w.retitle()
		})
	}()
}

// relist reads the folder again, as after its order changed or a file
// went, keeping the picture showing.
func (w *Window) relist() {
	if w.dir == "" {
		return
	}
	name := ""
	if w.cur >= 0 && w.cur < len(w.files) {
		name = w.files[w.cur].Name
	}
	if name == "" {
		w.openPath(w.dir)
		return
	}
	w.openPath(filepath.Join(w.dir, name))
}

// shownAt starts showing the picture at an index, which cur is already:
// its preview is decoded first, then those next to it.
func (w *Window) shownAt(heading int) {
	w.nav++
	w.heading = heading
	p := w.current()
	if p == nil {
		return
	}
	if !w.app.set.KeepZoom {
		w.view.fitted = true
		w.view.target = 0
	}
	w.take(p)
	w.load(p, false)
	if p.best() != nil {
		w.shown = p
	}
	w.preload()
	w.retitle()
	w.invalidate()
}

// step goes to the picture after the one showing, or before it.
func (w *Window) step(d int) {
	n := len(w.files)
	if n == 0 {
		return
	}
	i := w.cur + d
	if w.app.set.Loop {
		i = ((i % n) + n) % n
	} else if i < 0 || i >= n {
		return
	}
	w.goTo(i, d)
}

// goTo shows the picture at an index.
func (w *Window) goTo(i, heading int) {
	if i < 0 || i >= len(w.files) || i == w.cur {
		return
	}
	w.cur = i
	if heading > 0 {
		heading = 1
	} else if heading < 0 {
		heading = -1
	}
	w.shownAt(heading)
	w.thumbs.follow = true
}

// retitle names the window after the picture showing.
func (w *Window) retitle() {
	if w.win == nil {
		return
	}
	title := "MyImages"
	if w.cur >= 0 && w.cur < len(w.files) {
		title = w.files[w.cur].Name + " - MyImages"
	}
	w.win.SetTitle(title)
}

// say shows a message over the picture for a few seconds.
func (w *Window) say(msg string) {
	w.message, w.messageUntil = msg, time.Now().Add(3*time.Second)
	w.invalidate()
}

// report shows an error, if there is one.
func (w *Window) report(err error) {
	if err != nil {
		w.update(func() { w.say(err.Error()) })
	}
}
