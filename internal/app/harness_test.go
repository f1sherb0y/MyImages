package app

import (
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"myimages/internal/config"
)

// harness runs a window's view without a window on screen: real files
// in a temporary folder, real decoding, and input a test makes up.
type harness struct {
	t   *testing.T
	a   *App
	w   *Window
	tt  *ui.Tester
	dir string
}

// picture writes a test picture: a gradient with a square of a color in
// its middle, w×h, as a PNG or a JPEG by its name.
func picture(t *testing.T, path string, w, h int, c color.RGBA) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(255 * x / w), uint8(255 * y / h), 96, 255})
		}
	}
	for y := h / 3; y < 2*h/3; y++ {
		for x := w / 3; x < 2*w/3; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if filepath.Ext(path) == ".png" {
		err = png.Encode(f, img)
	} else {
		err = jpeg.Encode(f, img, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		t.Fatal(err)
	}
}

var (
	red   = color.RGBA{220, 30, 30, 255}
	green = color.RGBA{30, 200, 60, 255}
	blue  = color.RGBA{40, 60, 230, 255}
)

// newHarness makes a folder of three pictures and a window showing the
// first.
func newHarness(t *testing.T) *harness {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("MYIMAGES_CONFIG_DIR", filepath.Join(dir, "config"))
	pics := filepath.Join(dir, "pics")
	if err := os.Mkdir(pics, 0o755); err != nil {
		t.Fatal(err)
	}
	picture(t, filepath.Join(pics, "a1.png"), 400, 300, red)
	picture(t, filepath.Join(pics, "a2.jpg"), 3000, 2000, green)
	picture(t, filepath.Join(pics, "a10.png"), 300, 600, blue)
	a := &App{set: config.Defaults(), started: time.Now()}
	a.opt.Ephemeral = true
	h := &harness{t: t, a: a, dir: pics}
	h.w = a.window(filepath.Join(pics, "a1.png"))
	h.tt = ui.NewTester(h.w.build, 900, 600)
	h.settle()
	return h
}

// settle runs what goroutines asked of the main thread, and frames,
// until nothing is decoding and nothing waits.
func (h *harness) settle() {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	quiet := 0
	for quiet < 3 {
		if time.Now().After(deadline) {
			h.t.Fatal("the window did not settle")
		}
		h.w.mu.Lock()
		pending := h.w.pending
		h.w.pending = nil
		h.w.mu.Unlock()
		for _, fn := range pending {
			fn()
		}
		h.tt.Frame()
		busy := len(pending) > 0 || h.w.view.target != 0 || h.w.reading > 0
		for _, p := range h.w.photos {
			busy = busy || p.loading || p.loadingFull
		}
		if busy {
			quiet = 0
			time.Sleep(5 * time.Millisecond)
			h.tt.Advance(20 * time.Millisecond)
		} else {
			quiet++
		}
	}
}

// save writes what the window shows to out/<name>.png at the root of
// the repository when MYIMAGES_SHOTS is set, to look at.
func (h *harness) save(name string) {
	if os.Getenv("MYIMAGES_SHOTS") == "" {
		return
	}
	dir := filepath.Join("..", "..", "out")
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		h.t.Fatal(err)
	}
}

// at returns the color the window shows at a point.
func (h *harness) at(x, y int) color.RGBA {
	return h.tt.Image().RGBAAt(int(float32(x)), int(float32(y)))
}

// near reports whether two colors are about the same.
func near(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 40 && d(a.G, b.G) < 40 && d(a.B, b.B) < 40
}

// name is the name of the file showing.
func (h *harness) name() string {
	if p := h.w.current(); p != nil {
		return p.file.Name
	}
	return ""
}
