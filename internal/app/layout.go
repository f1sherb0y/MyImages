package app

import (
	"fmt"
	"time"

	"github.com/egoist/mygo/ui"

	"myimages/locales"
)

// The heights of the window's bars, in DIPs. The interface is compact on
// purpose: the picture gets the room.
const (
	stripHeight  = 32
	statusHeight = 22
	controlSize  = 24
)

func svg(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.3" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

// The icons of the interface's buttons, drawn in the color of the text.
var (
	icoOpen     = svg(`<path d="M2 12.5V4a1 1 0 0 1 1-1h3l1.5 1.5H13a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1z"/>`)
	icoPrev     = svg(`<path d="M9.5 3.5L5 8l4.5 4.5"/>`)
	icoNext     = svg(`<path d="M6.5 3.5L11 8l-4.5 4.5"/>`)
	icoZoomIn   = svg(`<circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14M5 7h4M7 5v4"/>`)
	icoZoomOut  = svg(`<circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5L14 14M5 7h4"/>`)
	icoFit      = svg(`<path d="M2.5 6V3.5a1 1 0 0 1 1-1H6M10 2.5h2.5a1 1 0 0 1 1 1V6M13.5 10v2.5a1 1 0 0 1-1 1H10M6 13.5H3.5a1 1 0 0 1-1-1V10"/>`)
	icoActual   = svg(`<path d="M3.5 5.5L5 4.5v7M11 5.5l1.5-1v7"/><circle cx="8.25" cy="6.5" r="0.5"/><circle cx="8.25" cy="9.5" r="0.5"/>`)
	icoRotateL  = svg(`<path d="M3 8a5 5 0 1 0 1.5-3.55M3 2.5v3h3"/>`)
	icoRotateR  = svg(`<path d="M13 8a5 5 0 1 1-1.5-3.55M13 2.5v3h-3"/>`)
	icoTrash    = svg(`<path d="M3 4.5h10M6.5 4.5V3h3v1.5M4.5 4.5l.5 8.5h6l.5-8.5M6.75 7v3.5M9.25 7v3.5"/>`)
	icoThumbs   = svg(`<rect x="2.5" y="3" width="11" height="6.5" rx="1"/><path d="M3 12.5h2M7 12.5h2M11 12.5h2"/>`)
	icoInfo     = svg(`<circle cx="8" cy="8" r="5.5"/><path d="M8 7.5v3.5"/><circle cx="8" cy="5.25" r="0.4"/>`)
	icoMore     = svg(`<circle cx="4" cy="8" r="0.6"/><circle cx="8" cy="8" r="0.6"/><circle cx="12" cy="8" r="0.6"/>`)
	icoFull     = svg(`<path d="M9.5 2.5h4v4M6.5 13.5h-4v-4M13.5 2.5L9 7M2.5 13.5L7 9"/>`)
	icoPlay     = svg(`<path d="M5 3.5v9l7-4.5z"/>`)
	icoUpdate   = svg(`<path d="M8 2.5v7.5M4.75 6.75L8 10l3.25-3.25M3 13h10"/>`)
	icoSettings = svg(`<path d="M2.5 5h5M11 5h2.5M2.5 11h2.5M8.5 11h5"/><circle cx="9.25" cy="5" r="1.75"/><circle cx="6.75" cy="11" r="1.75"/>`)
)

// iconButton is a small square button showing an icon, which lights up
// under the pointer.
func iconButton(c *ui.Context, key any, icon *ui.SVG, tip string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c.Key(key)).Size(controlSize+2, controlSize).Shrink(0).Radius(4).Center().Tooltip(tip).Label(tip).TabStop(false)
	// The hover is painted, not built: the pointer crossing the button
	// draws a frame without running the view.
	b.Draw(func(p *ui.Painter, r ui.Rect) {
		switch {
		case b.IsDisabled():
		case b.Pressed():
			p.Fill(r, t.SurfacePressed, 4)
		case b.Hovered():
			p.Fill(r, t.SurfaceHover, 4)
		}
	})
	b.Children(func() { ui.Icon(c, icon).FontSize(15) })
	return b
}

// gap is a little room between groups of buttons.
func gap(c *ui.Context) { ui.Box(c).Width(6).Shrink(0) }

// build is the window's view: the strip of buttons and the title, the
// picture, the thumbnails and the status bar.
func (w *Window) build(c *ui.Context) {
	a := w.app
	if w.firstAt.IsZero() {
		w.firstAt = time.Now()
		a.mark("first build")
		// The picture the program was started with is in the window's
		// first frame where it comes in time.
		w.await(w.current(), 250*time.Millisecond)
		a.mark("first picture awaited")
	}
	t := *c.Theme()
	t.FontSize = 12
	t.Spacing = 3
	t.Radius = 4
	c.SetTheme(&t)

	if p := w.current(); p != nil {
		w.take(p)
	}
	if d := w.drawn(); d != nil && d.w > 0 && w.viewW > 0 {
		if w.place(d.w, d.h, c.Now()) {
			c.AnimationFrame()
		}
	}
	w.shortcuts(c)
	bare := w.fullscreen || w.slideshow
	ui.Column(c).Fill().Children(func() {
		if !bare {
			w.strip(c)
		}
		ui.Row(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
			w.viewer(c)
			if a.set.Info && !bare {
				w.infoPanel(c)
			}
		})
		if a.set.Thumbnails && !bare && len(w.files) > 1 {
			w.thumbs.build(c, w)
		}
		if a.set.Status && !bare {
			w.status(c)
		}
	})
	w.dialogs(c)
	if w.benchRun != nil {
		w.benchRun.frame(c)
	}
}

// viewer is the picture area.
func (w *Window) viewer(c *ui.Context) {
	v := ui.Box(c.Key("viewer")).Grow(1).Clip()
	v.Draw(w.paint)
	v.HandleInput(w.input)
	if w.pannable() {
		if w.view.dragging {
			v.Cursor(ui.CursorGrabbing)
		} else {
			v.Cursor(ui.CursorGrab)
		}
	}
	v.ContextMenu(w.menu)
	if files := v.DroppedFiles(); len(files) > 0 {
		w.openPath(files[0])
	}
	v.DrawOver(func(p *ui.Painter, r ui.Rect) { w.overlay(p, r) })
}

// overlay draws what shows over the picture: a message, an error, or
// what to do with an empty window.
func (w *Window) overlay(p *ui.Painter, r ui.Rect) {
	th := p.Theme()
	center := func(text string, color ui.Color, size float32) {
		span := ui.Span{Text: text, Color: color, Size: size}
		tw, tht := p.MeasureText(r.W-40, span)
		p.RichText(r.X+(r.W-tw)/2, r.Y+(r.H-tht)/2, r.W-40, span)
	}
	cur := w.current()
	switch {
	case len(w.files) == 0 && w.dir == "":
		center(locales.EmptyHint(), th.TextMuted, 13)
	case len(w.files) == 0:
		center(locales.EmptyFolder(), th.TextMuted, 13)
	case cur != nil && cur.err != nil && cur.best() == nil:
		center(locales.CannotShow(cur.file.Name, cur.err.Error()), th.TextMuted, 13)
	}
	if w.message != "" {
		if left := w.messageUntil.Sub(p.Now()); left > 0 {
			span := ui.Span{Text: w.message, Color: ui.RGB(255, 255, 255), Size: 12}
			tw, tht := p.MeasureText(r.W-60, span)
			box := ui.Rect{X: r.X + (r.W-tw)/2 - 12, Y: r.Y + r.H - tht - 34, W: tw + 24, H: tht + 12}
			p.Fill(box, ui.RGBA(20, 20, 20, 0.82), 6)
			p.RichText(box.X+12, box.Y+6, r.W-60, span)
			p.After(left)
		} else {
			w.message = ""
		}
	}
}

// strip is the bar at the top: the buttons, and the picture's name, by
// which the window is dragged.
func (w *Window) strip(c *ui.Context) {
	a := w.app
	th := c.Theme()
	bar := c.TitleBar()
	width, _ := c.Size()
	has := w.current() != nil
	many := len(w.files) > 1
	row := ui.Row(c.Key("strip")).Height(stripHeight).Shrink(0).AlignItems(ui.Center).
		Padding(0, bar.Right+6, 0, bar.Left+6).Gap(1).Background(th.Surface).DragWindow()
	row.Children(func() {
		if !a.set.Toolbar {
			w.title(c)
			return
		}
		if iconButton(c, "open", icoOpen, locales.Open()+" (Ctrl+O)").Clicked() {
			w.openDialog()
		}
		gap(c)
		if iconButton(c, "prev", icoPrev, locales.Previous()+" (←)").Disabled(!many).Clicked() {
			w.step(-1)
		}
		if iconButton(c, "next", icoNext, locales.Next()+" (→)").Disabled(!many).Clicked() {
			w.step(1)
		}
		gap(c)
		if iconButton(c, "zoomout", icoZoomOut, locales.ZoomOut()+" (-)").Disabled(!has).Clicked() {
			w.zoomBy(1/zoomStep, 0, 0, false)
		}
		zoom := ui.ButtonBase(c.Key("zoom")).Height(controlSize).Width(46).Shrink(0).Radius(4).Center().TabStop(false).
			Tooltip(locales.ZoomMode()).Disabled(!has)
		zoom.Draw(func(p *ui.Painter, r ui.Rect) {
			if zoom.Hovered() {
				p.Fill(r, th.SurfaceHover, 4)
			}
		})
		zoom.Children(func() {
			ui.Text(c, fmt.Sprintf("%.0f%%", w.view.zoom*100)).SingleLine().TextColor(th.TextMuted)
		})
		zoom.Menu(w.zoomMenu)
		if iconButton(c, "zoomin", icoZoomIn, locales.ZoomIn()+" (+)").Disabled(!has).Clicked() {
			w.zoomBy(zoomStep, 0, 0, false)
		}
		if width > 560 {
			if iconButton(c, "fit", icoFit, locales.Fit()+" (0)").Disabled(!has).Clicked() {
				w.fit()
			}
			if iconButton(c, "actual", icoActual, locales.ActualSize()+" (1)").Disabled(!has).Clicked() {
				w.zoomTo(1, 0, 0, false)
			}
		}
		if width > 680 {
			gap(c)
			if iconButton(c, "rotl", icoRotateL, locales.RotateLeft()+" (L)").Disabled(!has).Clicked() {
				w.turn(-1)
			}
			if iconButton(c, "rotr", icoRotateR, locales.RotateRight()+" (R)").Disabled(!has).Clicked() {
				w.turn(1)
			}
			gap(c)
			if iconButton(c, "delete", icoTrash, locales.Delete()+" (Del)").Disabled(!has).Clicked() {
				w.delete(false)
			}
		}
		w.title(c)
		if a.updateReady != "" {
			w.updateButton(c)
		}
		if width > 480 {
			if iconButton(c, "thumbs", icoThumbs, locales.Thumbnails()+" (T)").Clicked() {
				w.toggle(&a.set.Thumbnails)
			}
			if iconButton(c, "info", icoInfo, locales.Info()+" (I)").Clicked() {
				w.toggle(&a.set.Info)
			}
			if iconButton(c, "full", icoFull, locales.FullScreen()+" (F11)").Clicked() {
				w.toggleFullScreen()
			}
		}
		more := iconButton(c, "more", icoMore, locales.More())
		more.Menu(w.menu)
	})
}

// title is the name of the picture showing, in the room the buttons
// leave, with its place among the folder's.
func (w *Window) title(c *ui.Context) {
	th := c.Theme()
	ui.Row(c).Grow(1).Shrink(1).MinWidth(0).Height(stripHeight).Center().Gap(8).Padding(0, 8).Clip().Children(func() {
		if w.cur < 0 || w.cur >= len(w.files) {
			ui.Text(c, "MyImages").SingleLine().TextColor(th.TextMuted)
			return
		}
		ui.Text(c, w.files[w.cur].Name).SingleLine().Shrink(1).MinWidth(0).Ellipsis("…")
		if len(w.files) > 1 {
			ui.Text(c, fmt.Sprintf("%d / %d", w.cur+1, len(w.files))).SingleLine().Shrink(0).TextColor(th.TextMuted)
		}
	})
}

// status is the bar at the bottom: what the picture is, and the color
// under the pointer.
func (w *Window) status(c *ui.Context) {
	th := c.Theme()
	p := w.current()
	row := ui.Row(c.Key("status")).Height(statusHeight).Shrink(0).AlignItems(ui.Center).Padding(0, 8).Gap(12).Background(th.Surface)
	row.Children(func() {
		muted := func(s string) { ui.Text(c, s).SingleLine().Shrink(0).FontSize(11).TextColor(th.TextMuted) }
		if p == nil {
			ui.Spacer(c)
			return
		}
		if p.w > 0 {
			muted(fmt.Sprintf("%d × %d", p.w, p.h))
		}
		if p.file.Size > 0 {
			muted(formatSize(p.file.Size))
		}
		if p.format != "" {
			if p.partial {
				muted(locales.PreviewOf(p.format))
			} else {
				muted(p.format)
			}
		}
		if p.frames > 1 {
			muted(locales.FrameOf(p.frame+1, p.frames))
		}
		if !p.file.Mod.IsZero() {
			muted(p.file.Mod.Format("2006-01-02 15:04"))
		}
		ui.Spacer(c)
		if w.view.over {
			if x, y, col, ok := w.pixelAt(w.view.hx, w.view.hy); ok {
				ui.Box(c).Size(10, 10).Radius(2).Border(1, th.Border).Background(ui.RGBA(col[0], col[1], col[2], float32(col[3])/255))
				muted(fmt.Sprintf("#%02X%02X%02X  %d, %d", col[0], col[1], col[2], x, y))
			}
		}
		if !w.app.set.Toolbar {
			muted(fmt.Sprintf("%.0f%%", w.view.zoom*100))
		}
	})
}

// formatSize writes a number of bytes as people read them.
func formatSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
	return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
}

// toggle flips a setting that shows or hides a part of the window.
func (w *Window) toggle(v *bool) {
	*v = !*v
	w.app.saveSoon()
	w.app.invalidate()
}

// toggleFullScreen fills the screen with the picture, or gives the
// window back.
func (w *Window) toggleFullScreen() {
	if w.win != nil {
		w.win.ToggleFullScreen()
	}
}
