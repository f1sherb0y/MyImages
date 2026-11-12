package app

import (
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"myimages/internal/codec"
	"myimages/internal/shell"
	"myimages/locales"
)

// shortcuts handles the keys of the window.
func (w *Window) shortcuts(c *ui.Context) {
	if w.ask != nil || w.settings || w.renaming {
		return
	}
	a := w.app
	key := func(mods ui.Modifiers, k ui.Key) bool { return c.Shortcut(mods, k) }
	switch {
	case key(0, ui.KeyRight), key(0, ui.KeyPageDown), key(0, ui.KeySpace), key(0, ui.KeyForward):
		w.step(1)
	case key(0, ui.KeyLeft), key(0, ui.KeyPageUp), key(0, ui.KeyBackspace), key(0, ui.KeyBack):
		w.step(-1)
	case key(0, ui.KeyHome):
		w.goTo(0, -1)
	case key(0, ui.KeyEnd):
		w.goTo(len(w.files)-1, 1)
	case key(0, ui.KeyEqual), key(ui.Shift, ui.KeyEqual), key(ui.Cmd, ui.KeyEqual), key(0, ui.KeyUp):
		w.zoomBy(zoomStep, 0, 0, false)
	case key(0, ui.KeyMinus), key(ui.Cmd, ui.KeyMinus), key(0, ui.KeyDown):
		w.zoomBy(1/zoomStep, 0, 0, false)
	case key(0, ui.Key0), key(ui.Cmd, ui.Key0):
		w.fit()
	case key(0, ui.Key1), key(ui.Cmd, ui.Key1):
		w.zoomTo(1, 0, 0, false)
	case key(0, ui.KeyR), key(ui.Cmd, ui.KeyPeriod):
		w.turn(1)
	case key(0, ui.KeyL), key(ui.Shift, ui.KeyR):
		w.turn(-1)
	case key(0, ui.KeyH):
		w.mirror()
	case key(0, ui.KeyF11), key(0, ui.KeyF), key(ui.Alt, ui.KeyEnter):
		w.toggleFullScreen()
	case key(0, ui.KeyEscape):
		switch {
		case w.slideshow:
			w.slide(false)
		case w.fullscreen:
			w.toggleFullScreen()
		default:
			w.win.Close()
		}
	case key(ui.Cmd, ui.KeyW):
		w.win.Close()
	case key(ui.Cmd, ui.KeyO):
		w.openDialog()
	case key(0, ui.KeyDelete):
		w.delete(false)
	case key(ui.Shift, ui.KeyDelete):
		w.delete(true)
	case key(ui.Cmd, ui.KeyC):
		w.copy()
	case key(ui.Cmd|ui.Shift, ui.KeyC):
		w.copyPath()
	case key(ui.Cmd, ui.KeyV):
		w.paste()
	case key(0, ui.KeyF5), key(ui.Cmd, ui.KeyR):
		w.reload()
	case key(0, ui.KeyF2):
		w.rename()
	case key(0, ui.KeyT):
		w.toggle(&a.set.Thumbnails)
	case key(0, ui.KeyI):
		w.toggle(&a.set.Info)
	case key(0, ui.KeyB):
		w.cycleBackground()
	case key(0, ui.KeyS), key(0, ui.KeyF12):
		w.slide(!w.slideshow)
	case key(ui.Cmd, ui.KeyE):
		w.reveal()
	case key(ui.Cmd, ui.KeyP):
		w.shellDo(shell.Print)
	case key(ui.Cmd, ui.KeyComma):
		w.settings = true
	case key(ui.Cmd, ui.KeyRight), key(0, ui.KeyPeriod):
		w.page(1)
	case key(ui.Cmd, ui.KeyLeft), key(0, ui.KeyComma):
		w.page(-1)
	}
}

// menu is the picture's menu: that of a right click, and of the strip's
// last button.
func (w *Window) menu(m *ui.Menu) {
	a := w.app
	p := w.current()
	none := p == nil
	if m.Item(locales.Open()).Shortcut(ui.Cmd, ui.KeyO).Chosen() {
		w.openDialog()
	}
	if m.Item(locales.OpenWith()).Disabled(none).Chosen() {
		w.shellDo(shell.OpenWith)
	}
	if m.Item(locales.Reveal()).Shortcut(ui.Cmd, ui.KeyE).Disabled(none).Chosen() {
		w.reveal()
	}
	m.Separator()
	if m.Item(locales.Copy()).Shortcut(ui.Cmd, ui.KeyC).Disabled(none).Chosen() {
		w.copy()
	}
	if m.Item(locales.CopyPath()).Shortcut(ui.Cmd|ui.Shift, ui.KeyC).Disabled(none).Chosen() {
		w.copyPath()
	}
	if m.Item(locales.Rename()).Shortcut(0, ui.KeyF2).Disabled(none).Chosen() {
		w.rename()
	}
	if m.Item(locales.Delete()).Shortcut(0, ui.KeyDelete).Disabled(none).Chosen() {
		w.delete(false)
	}
	m.Separator()
	m.Submenu(locales.View(), func(m *ui.Menu) {
		w.zoomMenu(m)
		m.Separator()
		if m.Item(locales.RotateRight()).Shortcut(0, ui.KeyR).Disabled(none).Chosen() {
			w.turn(1)
		}
		if m.Item(locales.RotateLeft()).Shortcut(0, ui.KeyL).Disabled(none).Chosen() {
			w.turn(-1)
		}
		if m.Item(locales.Mirror()).Shortcut(0, ui.KeyH).Disabled(none).Chosen() {
			w.mirror()
		}
		m.Separator()
		if m.Item(locales.Reload()).Shortcut(0, ui.KeyF5).Disabled(none).Chosen() {
			w.reload()
		}
	})
	m.Submenu(locales.SortBy(), func(m *ui.Menu) {
		for _, by := range []struct{ id, label string }{
			{"name", locales.SortName()}, {"date", locales.SortDate()}, {"size", locales.SortSize()}, {"type", locales.SortType()},
		} {
			if m.Item(by.label).Radio(a.set.SortBy == by.id).Chosen() {
				a.set.SortBy = by.id
				a.saveSoon()
				w.relist()
			}
		}
		m.Separator()
		if m.Item(locales.SortDescending()).Checked(a.set.SortDescending).Chosen() {
			a.set.SortDescending = !a.set.SortDescending
			a.saveSoon()
			w.relist()
		}
	})
	m.Submenu(locales.Show(), func(m *ui.Menu) {
		if m.Item(locales.Toolbar()).Checked(a.set.Toolbar).Chosen() {
			w.toggle(&a.set.Toolbar)
		}
		if m.Item(locales.Thumbnails()).Shortcut(0, ui.KeyT).Checked(a.set.Thumbnails).Chosen() {
			w.toggle(&a.set.Thumbnails)
		}
		if m.Item(locales.Info()).Shortcut(0, ui.KeyI).Checked(a.set.Info).Chosen() {
			w.toggle(&a.set.Info)
		}
		if m.Item(locales.StatusBar()).Checked(a.set.Status).Chosen() {
			w.toggle(&a.set.Status)
		}
	})
	if m.Item(locales.FullScreen()).Shortcut(0, ui.KeyF11).Checked(w.fullscreen).Chosen() {
		w.toggleFullScreen()
	}
	if m.Item(locales.Slideshow()).Shortcut(0, ui.KeyS).Checked(w.slideshow).Chosen() {
		w.slide(!w.slideshow)
	}
	m.Separator()
	if m.Item(locales.Wallpaper()).Disabled(none).Chosen() {
		w.shellDo(shell.SetWallpaper)
	}
	if m.Item(locales.Print()).Shortcut(ui.Cmd, ui.KeyP).Disabled(none).Chosen() {
		w.shellDo(shell.Print)
	}
	if m.Item(locales.Properties()).Disabled(none).Chosen() {
		w.shellDo(shell.Properties)
	}
	m.Separator()
	if m.Item(locales.Settings()).Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
		w.settings = true
	}
}

// zoomMenu lists how a picture first shows, and the zooms to go to.
func (w *Window) zoomMenu(m *ui.Menu) {
	a := w.app
	for _, mode := range []struct{ id, label string }{
		{"auto", locales.ZoomAuto()}, {"fit", locales.ZoomFit()}, {"fill", locales.ZoomFill()}, {"actual", locales.ZoomActual()},
	} {
		if m.Item(mode.label).Radio(a.set.Zoom == mode.id).Chosen() {
			a.set.Zoom = mode.id
			a.saveSoon()
			w.fit()
		}
	}
	m.Separator()
	if m.Item(locales.KeepZoom()).Checked(a.set.KeepZoom).Chosen() {
		a.set.KeepZoom = !a.set.KeepZoom
		a.saveSoon()
	}
	m.Separator()
	for _, z := range []float64{0.25, 0.5, 1, 2, 4, 8} {
		if m.Item(locales.Percent(int(z * 100))).Chosen() {
			w.zoomTo(z, 0, 0, false)
		}
	}
}

// turn turns the picture by quarter turns clockwise. The file stays as
// it is.
func (w *Window) turn(quarters int) {
	p := w.current()
	if p == nil || p.best() == nil {
		return
	}
	if p.flip {
		// Mirrored, a turn one way is a turn the other way before the
		// mirror.
		quarters = -quarters
	}
	p.turns = ((p.turns+quarters)%4 + 4) % 4
	w.reorient(p)
}

// mirror flips the picture left to right.
func (w *Window) mirror() {
	p := w.current()
	if p == nil || p.best() == nil {
		return
	}
	p.flip = !p.flip
	w.reorient(p)
}

// reorient shows a photo as its turns now say: decoded again, which the
// cache of the system's files makes quick.
func (w *Window) reorient(p *photo) {
	p.gen++
	p.preview, p.full = nil, nil
	p.loading, p.loadingFull = false, false
	w.view.fitted, w.view.target = true, 0
	w.load(p, false)
	w.await(p, 150*time.Millisecond)
	w.invalidate()
}

// reload reads the picture showing again, and its folder.
func (w *Window) reload() {
	p := w.current()
	if p == nil {
		return
	}
	w.drop(p)
	w.relist()
	w.shownAt(0)
}

// page shows the page after the one showing of a file with several, or
// the one before it.
func (w *Window) page(d int) {
	p := w.current()
	if p == nil || p.frames < 2 || p.anim != nil {
		return
	}
	p.frame = ((p.frame+d)%p.frames + p.frames) % p.frames
	w.reorient(p)
}

// openDialog asks for a picture to show.
func (w *Window) openDialog() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  w.win,
			Title:   locales.Open(),
			Filters: []mygo.FileFilter{{Name: locales.Pictures(), Extensions: codec.Extensions()}, {Name: locales.AllFiles(), Extensions: []string{"*"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		w.update(func() { w.openPath(paths[0]) })
	}()
}

// delete moves the picture showing to the Recycle Bin, or deletes it
// for good, after asking where the settings or the deed call for it.
func (w *Window) delete(forGood bool) {
	p := w.current()
	if p == nil {
		return
	}
	do := func() {
		path, name := p.path, p.file.Name
		go func() {
			err := shell.Delete(path, !forGood)
			w.update(func() {
				if err != nil {
					w.say(err.Error())
					return
				}
				w.removed(name)
			})
		}()
	}
	if forGood || w.app.set.ConfirmDelete {
		msg := locales.DeleteAsk(p.file.Name)
		if forGood {
			msg = locales.DeleteForGoodAsk(p.file.Name)
		}
		w.ask = &question{open: true, title: locales.Delete(), message: msg, confirm: locales.Delete(), yes: do}
		return
	}
	do()
}

// removed takes a file that went out of the list, and shows the one
// after it.
func (w *Window) removed(name string) {
	for i := range w.files {
		if w.files[i].Name != name {
			continue
		}
		path := filepath.Join(w.dir, name)
		if p := w.photos[path]; p != nil {
			w.drop(p)
		}
		w.files = append(w.files[:i], w.files[i+1:]...)
		if i < w.cur {
			w.cur--
		} else if i == w.cur {
			if w.cur >= len(w.files) {
				w.cur = len(w.files) - 1
			}
			if w.cur < 0 {
				w.retitle()
				w.invalidate()
				return
			}
			w.shownAt(1)
		}
		w.thumbs.follow = true
		w.invalidate()
		return
	}
}

// copy puts the picture on the clipboard, as a file and as pixels.
func (w *Window) copy() {
	p := w.current()
	if p == nil {
		return
	}
	path, orient := p.path, turned(p.orient, p.turns, p.flip)
	frame := p.frame
	go func() {
		// All of its pixels, which the window may not have decoded.
		var pix []byte
		var pw, ph int
		if pic, err := codec.Decode(path, codec.Options{Frame: frame}); err == nil {
			pix, pw, ph = upright(pic.Pix, pic.W, pic.H, turned(pic.Orient, 0, false))
			if orient != pic.Orient {
				pix, pw, ph = upright(pic.Pix, pic.W, pic.H, orient)
			}
		}
		if err := shell.CopyImage(path, pix, pw, ph); err != nil {
			w.report(err)
			return
		}
		w.update(func() { w.say(locales.Copied()) })
	}()
}

// copyPath puts the picture's path on the clipboard.
func (w *Window) copyPath() {
	if p := w.current(); p != nil {
		mygo.Clipboard.WriteText(p.path)
		w.say(locales.CopiedPath())
	}
}

// paste shows the file the clipboard holds.
func (w *Window) paste() {
	go func() {
		paths, err := mygo.Clipboard.ReadFiles()
		if err != nil || len(paths) == 0 {
			return
		}
		w.update(func() { w.openPath(paths[0]) })
	}()
}

// reveal shows the picture in the system's file manager.
func (w *Window) reveal() { w.shellDo(shell.Reveal) }

// shellDo has the system do something with the picture's file, off the
// main thread.
func (w *Window) shellDo(fn func(owner uintptr, path string) error) {
	p := w.current()
	if p == nil {
		return
	}
	owner := w.win.NativeHandle()
	path := p.path
	go func() { w.report(fn(owner, path)) }()
}

// cycleBackground shows the next of the backgrounds behind the picture.
func (w *Window) cycleBackground() {
	set := &w.app.set
	order := []string{"theme", "black", "white"}
	next := order[0]
	for i, b := range order {
		if b == set.Background {
			next = order[(i+1)%len(order)]
		}
	}
	set.Background = next
	w.app.saveSoon()
	w.app.invalidate()
}

// slide starts going through the pictures on its own, or stops.
func (w *Window) slide(on bool) {
	if on == w.slideshow || (on && len(w.files) < 2) {
		return
	}
	w.slideshow = on
	if on {
		if !w.fullscreen {
			w.slideOwnsScreen = true
			w.toggleFullScreen()
		}
		w.slideNext()
	} else if w.slideOwnsScreen {
		w.slideOwnsScreen = false
		if w.fullscreen {
			w.toggleFullScreen()
		}
	}
	w.invalidate()
}

// slideNext shows the next picture once the one showing had its time,
// unless the user went elsewhere meanwhile, which starts its time over.
func (w *Window) slideNext() {
	nav := w.nav
	d := time.Duration(w.app.set.SlideSeconds * float64(time.Second))
	time.AfterFunc(d, func() {
		w.update(func() {
			if !w.slideshow {
				return
			}
			if nav == w.nav {
				if !w.app.set.Loop && w.cur == len(w.files)-1 {
					w.slide(false)
					return
				}
				w.step(1)
			}
			w.slideNext()
		})
	})
}

// rename asks for another name for the picture's file.
func (w *Window) rename() {
	p := w.current()
	if p == nil {
		return
	}
	w.renaming = true
	w.newName = p.file.Name
}

// applyRename gives the picture's file the name asked for.
func (w *Window) applyRename() {
	p := w.current()
	w.renaming = false
	if p == nil || w.newName == "" || w.newName == p.file.Name {
		return
	}
	if filepath.Base(w.newName) != w.newName {
		w.say(locales.BadName())
		return
	}
	to := filepath.Join(w.dir, w.newName)
	if _, err := os.Lstat(to); err == nil && !samePath(to, p.path) {
		w.say(locales.NameTaken(w.newName))
		return
	}
	if err := os.Rename(p.path, to); err != nil {
		w.say(err.Error())
		return
	}
	w.openPath(to)
}
