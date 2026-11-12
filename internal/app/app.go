// Package app is MyImages's interface: a window showing one picture of
// a folder at a time. It draws with MyGo's native UI.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
	"myimages/internal/config"
	"myimages/internal/shell"
)

// Version is the program's version, as mygo.json has it when the
// program was built: "dev" for a build that was not packaged.
func Version() string {
	if v := mygo.App.Version(); v != "" {
		return v
	}
	return "dev"
}

// Options are what the command line asks for besides files.
type Options struct {
	// Bench, unless empty, measures going through the pictures of the
	// folder of the file given and prints what it took, then quits.
	Bench bool
	// Screenshot writes a picture of the window to this file, then
	// quits.
	Screenshot string
	// Ephemeral leaves the settings on disk as they are, and starts
	// beside an instance that runs.
	Ephemeral bool
}

// App is the running program: what its windows share.
type App struct {
	set     config.Settings
	opt     Options
	windows []*Window
	jobs    jobs
	started time.Time
	// tidy is the timer that returns memory to the system.
	tidyMu sync.Mutex
	tidy   *time.Timer
	trace  bool

	// saving is set while a write of the settings waits.
	saveMu sync.Mutex
	saving bool

	// updateReady is the newer version downloaded and in place, which
	// runs once the program starts again; "" for none.
	updateReady string
	updating    bool

	// langs are the languages the program is written in, and lang the
	// one chosen for it, "" while it follows the system's.
	langs []string
	lang  string
}

// Run shows the files given, or an empty window, until the last window
// closes. started is when the process began, for measuring.
func Run(paths []string, opt Options, started time.Time) error {
	a := &App{opt: opt, started: started, trace: os.Getenv("MYIMAGES_TRACE") != ""}
	a.mark("run")
	// The GPU's driver loads while the picture is read and the window
	// made.
	ui.Prewarm()
	a.set = config.Load()
	a.mark("settings read")
	solo := opt.Bench || opt.Screenshot != "" || opt.Ephemeral
	if !solo && !mygo.App.RequestSingleInstanceLock() {
		// The running instance got the command line and shows it.
		return nil
	}
	mygo.App.SetName("MyImages")
	if runtime.GOOS != "darwin" {
		// No menu bar: the window draws its own menus.
		mygo.App.SetMenu(nil)
	}
	a.applyTheme()

	// The first picture is read while the system makes the window.
	a.open(paths, true)
	a.mark("opened")

	mygo.App.OnSecondInstance(func(args []string, dir string) {
		a.open(absolute(files(args), dir), false)
	})
	mygo.App.OnOpenFile(func(path string) { a.open([]string{path}, false) })
	mygo.App.OnWindowAllClosed(func() { mygo.App.Quit() })
	mygo.App.WhenReady(func() {
		a.mark("ready")
		a.langs, a.lang = mygo.App.Languages(), mygo.App.ChosenLocale()
		for _, w := range a.windows {
			a.show(w)
		}
		a.mark("window made")
		if !solo || os.Getenv("MYIMAGES_UPDATE_DIR") != "" {
			// Also when trying an update before it is published, beside
			// the copy installed.
			a.watchUpdates()
		}
		if !solo && mygo.Updater.Enabled() && a.set.Registered != Version() {
			// An installed copy tells the system, once a version, which
			// files it opens: the settings' page of default programs
			// lists it, and formats nothing opens yet become its own.
			a.set.Registered = Version()
			a.saveSoon()
			go func() {
				if exe, err := os.Executable(); err == nil {
					_, _ = shell.RegisterDefaults(exe, codec.Extensions())
				}
			}()
		}
	})
	return mygo.App.Run()
}

// mark prints how long after the start of the process something
// happened, when MYIMAGES_TRACE is set: for finding what the start waits
// on.
func (a *App) mark(what string) {
	if a.trace {
		fmt.Fprintf(os.Stderr, "%7.1f ms  %s\n", float64(time.Since(a.started).Microseconds())/1000, what)
	}
}

// files are the arguments of a command line that name files: those that
// are no options.
func files(args []string) []string {
	var out []string
	for _, arg := range args {
		if arg != "" && arg[0] != '-' {
			out = append(out, arg)
		}
	}
	return out
}

// absolute makes paths given relative to a directory absolute.
func absolute(paths []string, dir string) []string {
	for i, p := range paths {
		if !filepath.IsAbs(p) {
			paths[i] = filepath.Join(dir, p)
		}
	}
	return paths
}

// open shows files: the first in the window used last, when the
// settings keep to one window, or in a new one. first is set for what
// the program started with, which gets a window even without a file.
func (a *App) open(paths []string, first bool) {
	if len(paths) == 0 {
		if first {
			a.window("")
		} else if n := len(a.windows); n > 0 {
			a.windows[n-1].raise()
		}
		return
	}
	path := paths[0]
	for _, w := range a.windows {
		if p := w.current(); p != nil && samePath(p.path, path) {
			// Showing already, as when the system hands over the file
			// the program was started with.
			w.raise()
			return
		}
	}
	if n := len(a.windows); n > 0 && a.set.OneWindow {
		w := a.windows[n-1]
		w.openPath(path)
		w.raise()
		return
	}
	w := a.window(path)
	if mygo.App.IsReady() {
		a.show(w)
	}
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return filepath.Clean(a) == filepath.Clean(b) || equalFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// known reports whether files of an extension are pictures.
func known(ext string) bool { return codec.Known(ext) }

// isDir reports whether a path is a directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// applyTheme makes the program light, dark, or as the system.
func (a *App) applyTheme() {
	switch a.set.Theme {
	case "light":
		mygo.Theme.SetSource(mygo.ThemeLight)
	case "dark":
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
}

// saveSoon writes the settings in a moment, once for the changes that
// come until then.
func (a *App) saveSoon() {
	if a.opt.Ephemeral || a.opt.Bench || a.opt.Screenshot != "" {
		return
	}
	a.saveMu.Lock()
	pending := a.saving
	a.saving = true
	a.saveMu.Unlock()
	if pending {
		return
	}
	time.AfterFunc(600*time.Millisecond, func() {
		mygo.RunOnMain(func() {
			a.saveMu.Lock()
			a.saving = false
			a.saveMu.Unlock()
			_ = a.set.Save()
		})
	})
}

// invalidate draws every window again.
func (a *App) invalidate() {
	for _, w := range a.windows {
		w.invalidate()
	}
}

// remove forgets a window that closed.
func (a *App) remove(w *Window) {
	for i, o := range a.windows {
		if o == w {
			a.windows = append(a.windows[:i], a.windows[i+1:]...)
			break
		}
	}
}
