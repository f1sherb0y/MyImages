package shell

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// These tests do things to the real desktop: they replace what the
// clipboard holds, and open the
// system's dialogs (and close them). They run only when
// MYIMAGES_DESKTOP_TESTS is set, on a desktop someone is logged in to.
func desktop(t *testing.T) string {
	t.Helper()
	if os.Getenv("MYIMAGES_DESKTOP_TESTS") == "" {
		t.Skip("set MYIMAGES_DESKTOP_TESTS=1 to run what touches the real desktop")
	}
	path := filepath.Join(t.TempDir(), "myimages-desktop-test.png")
	img := image.NewRGBA(image.Rect(0, 0, 64, 48))
	for i := 0; i < len(img.Pix); i += 4 {
		c := color.RGBA{30, 90, 200, 255}
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}

// Setting the wallpaper is not tried here. A test did once, and put
// back the path the system reported before; but that path was the
// system's own cached copy, which setting the test's picture had
// overwritten, and the user's wallpaper was lost (2026-11-12). There is
// no way to put a wallpaper back that is sure for every way one can be
// set (a slideshow, Spotlight, a theme), so the real desktop's is left
// alone.

func TestCopyImage(t *testing.T) {
	path := desktop(t)
	pix := make([]byte, 4*8*6)
	for i := range pix {
		pix[i] = 255
	}
	if err := CopyImage(path, pix, 8, 6); err != nil {
		t.Fatal(err)
	}
	available := user32.NewProc("IsClipboardFormatAvailable")
	const cfDIB, cfHDrop = 8, 15
	for name, format := range map[string]uintptr{"the file": cfHDrop, "the pixels": cfDIB} {
		if r, _, _ := available.Call(format); r == 0 {
			t.Errorf("the clipboard does not hold %s", name)
		}
	}
}

func TestDeleteToRecycleBin(t *testing.T) {
	path := desktop(t)
	if err := Delete(path, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("the file is still there: %v", err)
	}
}

// windowsNow lists the visible top-level windows, by handle.
var (
	listed   map[uintptr]string
	listProc = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if v, _, _ := user32.NewProc("IsWindowVisible").Call(hwnd); v != 0 {
			var buf [256]uint16
			n, _, _ := user32.NewProc("GetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
			listed[hwnd] = windows.UTF16ToString(buf[:n])
		}
		return 1
	})
)

func windowsNow() map[uintptr]string {
	listed = map[uintptr]string{}
	user32.NewProc("EnumWindows").Call(listProc, 0)
	return listed
}

// opens runs something that should open a dialog of the system's, waits
// for a new window with a title, reports it, and closes it.
func opens(t *testing.T, what string, do func() error) {
	t.Helper()
	before := windowsNow()
	if err := do(); err != nil {
		t.Fatalf("%s: %v", what, err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for hwnd, title := range windowsNow() {
			if _, was := before[hwnd]; was || title == "" {
				continue
			}
			t.Logf("%s opened the window %q", what, title)
			const wmClose = 0x10
			user32.NewProc("PostMessageW").Call(hwnd, wmClose, 0, 0)
			time.Sleep(500 * time.Millisecond)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("%s opened no window", what)
}

func TestSystemDialogs(t *testing.T) {
	path := desktop(t)
	opens(t, "Properties", func() error { return Properties(0, path) })
	opens(t, "Open With", func() error { return OpenWith(0, path) })
	opens(t, "Print", func() error { return Print(0, path) })
	opens(t, "Show in File Explorer", func() error { return Reveal(0, path) })
	// What the dialogs left running.
	_ = exec.Command("taskkill", "/F", "/IM", "OpenWith.exe").Run()
}
