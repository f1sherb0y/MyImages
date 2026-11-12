package app

import (
	"image"
	"image/color"
	"image/gif"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// The middle of the picture area of a 900×600 window with its strip and
// status bar.
const midX, midY = 450, 305

func TestShowsThePictureAndItsNeighbors(t *testing.T) {
	h := newHarness(t)
	if h.name() != "a1.png" || len(h.w.files) != 3 {
		t.Fatalf("showing %q of %d files", h.name(), len(h.w.files))
	}
	if c := h.at(midX, midY); !near(c, red) {
		t.Errorf("the middle of the first picture is %v, want red", c)
	}
	h.save("first")
	// Names sort as people count: a1, a2, a10.
	h.tt.Key(0, ui.KeyRight)
	h.settle()
	if h.name() != "a2.jpg" {
		t.Fatalf("after →, showing %q", h.name())
	}
	if c := h.at(midX, midY); !near(c, green) {
		t.Errorf("the middle of the second picture is %v, want green", c)
	}
	h.tt.Key(0, ui.KeyRight)
	h.settle()
	if h.name() != "a10.png" {
		t.Fatalf("after → →, showing %q", h.name())
	}
	// Past the last, the first again.
	h.tt.Key(0, ui.KeyRight)
	h.settle()
	if h.name() != "a1.png" {
		t.Fatalf("after the last, showing %q", h.name())
	}
	h.tt.Key(0, ui.KeyEnd)
	h.settle()
	if h.name() != "a10.png" {
		t.Fatalf("after End, showing %q", h.name())
	}
}

func TestZoom(t *testing.T) {
	h := newHarness(t)
	h.tt.Key(0, ui.KeyRight) // 3000×2000: larger than the window
	h.settle()
	fit := h.w.view.zoom
	if fit >= 1 || !h.w.view.fitted {
		t.Fatalf("a large picture shows at %v, fitted %v", fit, h.w.view.fitted)
	}
	// Whole, it needs no more pixels than the window has.
	if p := h.w.current(); p.full != nil || p.preview == nil || p.preview.w >= 3000 {
		t.Fatalf("shown whole, the picture decoded is %+v, %+v", p.preview, p.full)
	}
	// 1 shows it at its own size, which takes all of them.
	h.tt.Key(0, ui.Key1)
	h.settle()
	if math.Abs(h.w.view.zoom-1) > 1e-6 {
		t.Errorf("after 1, the zoom is %v", h.w.view.zoom)
	}
	if p := h.w.current(); p.full == nil || p.full.w != 3000 {
		t.Fatalf("all the pixels were not decoded: %+v", p.full)
	}
	h.save("actual")
	// The wheel zooms about the pointer: the point under it stays.
	h.tt.Key(0, ui.Key0)
	h.settle()
	if !h.w.view.fitted || math.Abs(h.w.view.zoom-fit) > 1e-6 {
		t.Fatalf("after 0, the zoom is %v, fitted %v", h.w.view.zoom, h.w.view.fitted)
	}
	x, y := float32(300), float32(200)
	ix, iy, _, ok := h.w.pixelAt(x-0, y-stripHeight)
	if !ok {
		t.Fatal("no pixel under the pointer")
	}
	h.tt.Wheel(x, y, 0, -100)
	h.settle()
	if h.w.view.zoom <= fit {
		t.Fatalf("the wheel did not zoom in: %v", h.w.view.zoom)
	}
	jx, jy, _, _ := h.w.pixelAt(x, y-stripHeight)
	if d := math.Hypot(float64(jx-ix), float64(jy-iy)); d > 6 {
		t.Errorf("the point under the pointer moved by %.1f pixels of the picture", d)
	}
	// Dragging moves the picture.
	h.tt.Key(0, ui.Key1)
	h.settle()
	cx := h.w.view.cx
	h.tt.Press(midX, midY)
	h.tt.Move(midX-100, midY)
	h.tt.Release(midX-100, midY)
	h.settle()
	if h.w.view.cx <= cx {
		t.Errorf("dragging left did not move the view right: %v → %v", cx, h.w.view.cx)
	}
	// The next picture shows whole again.
	h.tt.Key(0, ui.KeyRight)
	h.settle()
	if !h.w.view.fitted {
		t.Error("the next picture is not fitted")
	}
}

func TestSmallPictureShowsAtItsSize(t *testing.T) {
	h := newHarness(t)
	if z := h.w.view.zoom; math.Abs(z-1) > 1e-6 {
		t.Errorf("a picture smaller than the window shows at %v", z)
	}
}

func TestRotate(t *testing.T) {
	h := newHarness(t)
	p := h.w.current()
	if p.w != 400 || p.h != 300 {
		t.Fatalf("the picture is %d×%d", p.w, p.h)
	}
	h.tt.Key(0, ui.KeyR)
	h.settle()
	p = h.w.current()
	if p.w != 300 || p.h != 400 {
		t.Errorf("turned, the picture is %d×%d", p.w, p.h)
	}
	// The gradient runs red to the right in the file: turned a quarter
	// clockwise, it runs red downward.
	top, bottom := h.at(midX, midY-150), h.at(midX, midY+150)
	if top.R >= bottom.R {
		t.Errorf("turned clockwise, red is %d at the top and %d at the bottom", top.R, bottom.R)
	}
	h.save("turned")
	h.tt.Key(0, ui.KeyL)
	h.settle()
	if p := h.w.current(); p.w != 400 || p.h != 300 {
		t.Errorf("turned back, the picture is %d×%d", p.w, p.h)
	}
}

func TestDeleteGoesToTheNext(t *testing.T) {
	h := newHarness(t)
	h.w.removed("a1.png")
	h.settle()
	if h.name() != "a2.jpg" || len(h.w.files) != 2 {
		t.Errorf("after removing the first, showing %q of %d", h.name(), len(h.w.files))
	}
}

func TestPanelsAndSettings(t *testing.T) {
	h := newHarness(t)
	h.tt.Key(0, ui.KeyT)
	h.tt.Key(0, ui.KeyI)
	h.settle()
	if !h.a.set.Thumbnails || !h.a.set.Info {
		t.Fatal("T and I did not show the thumbnails and the info")
	}
	if !h.tt.HasText("400 × 300  (0.1 MP)") {
		t.Errorf("the info does not tell the size: %q", h.tt.Texts())
	}
	h.save("panels")
	h.tt.Key(ui.Cmd, ui.KeyComma)
	h.settle()
	if !h.tt.HasText("Settings") {
		t.Errorf("the settings do not show: %q", h.tt.Texts())
	}
	h.save("settings")
}

func TestOpenAnotherFolder(t *testing.T) {
	h := newHarness(t)
	other := filepath.Join(filepath.Dir(h.dir), "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	picture(t, filepath.Join(other, "z.png"), 200, 200, blue)
	h.w.openPath(other)
	h.settle()
	if h.name() != "z.png" || len(h.w.files) != 1 {
		t.Errorf("opened a folder: showing %q of %d", h.name(), len(h.w.files))
	}
}

func TestAnimationPlays(t *testing.T) {
	h := newHarness(t)
	// Three frames, each of one color.
	pal := color.Palette{red, green, blue}
	g := &gif.GIF{}
	for i := range pal {
		frame := image.NewPaletted(image.Rect(0, 0, 120, 90), pal)
		for j := range frame.Pix {
			frame.Pix[j] = uint8(i)
		}
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 10)
	}
	path := filepath.Join(h.dir, "moving.gif")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, g); err != nil {
		t.Fatal(err)
	}
	f.Close()
	h.w.openPath(path)
	h.settle()
	if h.name() != "moving.gif" {
		t.Fatalf("showing %q", h.name())
	}
	seen := map[color.RGBA]bool{}
	for i := 0; i < 200 && len(seen) < 3; i++ {
		h.tt.Advance(60 * time.Millisecond)
		h.tt.Frame()
		time.Sleep(3 * time.Millisecond)
		c := h.at(midX, midY)
		for _, want := range []color.RGBA{red, green, blue} {
			if near(c, want) {
				seen[want] = true
			}
		}
	}
	if len(seen) < 3 {
		t.Errorf("the animation showed %d of its 3 frames", len(seen))
	}
}

func TestDrawingFillsTheWindow(t *testing.T) {
	h := newHarness(t)
	path := filepath.Join(h.dir, "drawing.svg")
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="30"><rect width="40" height="30" fill="#1e3ce6"/></svg>`
	if err := os.WriteFile(path, []byte(svg), 0o644); err != nil {
		t.Fatal(err)
	}
	h.w.openPath(path)
	h.settle()
	p := h.w.current()
	if p == nil || !p.vector || p.w != 40 || p.h != 30 {
		t.Fatalf("the drawing is %+v", p)
	}
	if h.w.view.zoom < 10 {
		t.Errorf("a drawing of 40×30 shows at zoom %v in a window of 900×600", h.w.view.zoom)
	}
	if c := h.at(midX, midY); !near(c, blue) {
		t.Errorf("the middle of the drawing is %v", c)
	}
	h.save("drawing")
	// Sharp at that size: its edge is one pixel wide, not a blur of the
	// 40 pixels it says it has.
	edge := 0
	y := midY
	for x := 0; x < 900; x++ {
		c := h.at(x, y)
		if !near(c, blue) && !near(c, h.at(5, y)) {
			edge++
		}
	}
	if edge > 6 {
		t.Errorf("the drawing's edges are %d pixels of blur", edge)
	}
}
