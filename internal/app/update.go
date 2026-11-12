package app

import (
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"myimages/locales"
)

// Updates are looked for as the program starts, on another thread, which
// is not in its way, and then every few hours for as long as it runs.
const (
	updateEvery = 6 * time.Hour
	// updateLimit is how long a check and its download may take.
	updateLimit = 15 * time.Minute
)

// watchUpdates looks for newer versions from now on.
func (a *App) watchUpdates() {
	a.checkUpdate()
	go func() {
		for range time.Tick(updateEvery) {
			mygo.RunOnMain(a.checkUpdate)
		}
	}()
}

// checkUpdate looks for a newer version, unless the settings say not to
// or one is there already: found, it is downloaded and put in place of
// this one on another thread, with nothing shown until it is ready. The
// program goes on as it is; the button that then shows starts it again,
// as the new version.
func (a *App) checkUpdate() {
	if !a.set.AutoUpdate || a.updating || a.updateReady != "" {
		return
	}
	a.updating = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateLimit)
		defer cancel()
		// What goes wrong, as being offline, is tried again at the next
		// check: nothing of it shows.
		version, err := fetchUpdate(ctx)
		mygo.RunOnMain(func() {
			a.updating = false
			if err == nil && version != "" {
				a.updateReady = version
				a.invalidate()
			}
		})
	}()
}

// fetchUpdate looks for a newer version and puts it in place of this
// one. It returns the version, "" when there is none or the program
// cannot update itself, as a build that was not installed.
func fetchUpdate(ctx context.Context) (string, error) {
	if !mygo.Updater.Enabled() {
		return "", nil
	}
	if dir := os.Getenv("MYIMAGES_UPDATE_DIR"); dir != "" {
		// For trying an update before it is published: the files a build
		// wrote, read from a folder. They are checked as the published
		// ones are, against the key built into the program.
		mygo.Updater.SetSource(updateDir(dir))
	}
	up, err := mygo.Updater.Check(ctx)
	if err != nil || up == nil {
		return "", err
	}
	if err := up.Install(ctx, nil); err != nil {
		return "", err
	}
	return up.Version, nil
}

// updateDir serves a build's update files from the folder they are in.
type updateDir string

func (d updateDir) Manifest(context.Context, bool) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(d), "update-"+runtime.GOOS+"-"+runtime.GOARCH+".json"))
}

// Open reads the file a URL of the manifest names, by its last part.
func (d updateDir) Open(_ context.Context, url string) (io.ReadCloser, error) {
	return os.Open(filepath.Join(string(d), path.Base(url)))
}

// updateButton shows, once a newer version is in place, the button that
// starts the program again with it.
func (w *Window) updateButton(c *ui.Context) {
	a := w.app
	th := c.Theme()
	b := ui.ButtonBase(c.Key("restart")).Height(22).Padding(0, 9, 0, 7).Gap(5).Radius(11).Shrink(0).Center().
		Background(th.Accent).Tooltip(locales.UpdateRestartTip(a.updateReady)).TabStop(false)
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		if b.Hovered() {
			p.Fill(r, th.AccentText.Alpha(0.14), 11)
		}
	})
	b.Children(func() {
		ui.Icon(c, icoUpdate).FontSize(13).TextColor(th.AccentText)
		ui.Text(c, locales.UpdateRestart()).SingleLine().FontWeight(600).TextColor(th.AccentText)
	})
	if b.Clicked() {
		a.restart()
	}
}

// restart starts the program again, as the version put in place, with
// the picture showing.
func (a *App) restart() {
	_ = a.set.Save()
	var args []string
	if n := len(a.windows); n > 0 {
		if p := a.windows[n-1].current(); p != nil {
			args = append(args, p.path)
		}
	}
	mygo.App.Relaunch(args...)
}
