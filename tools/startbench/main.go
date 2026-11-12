// Startbench times how long a picture viewer takes to show a picture,
// from the outside, the same way for any viewer: it starts the program
// with a file and watches the screen until a window whose title names
// the file shows something other than a flat background in its middle.
//
//	startbench [-n runs] [-keys N] viewer.exe picture.jpg
//
// With -keys it then presses the right arrow N times, each once the
// window's picture changed after the press before, and reports how long
// each change took.
//
// It reads the window as the window manager composes it, so the window
// need not be in front, and presses keys by posting them to the window
// alone.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procEnumWindows        = user32.NewProc("EnumWindows")
	procGetWindowTextW     = user32.NewProc("GetWindowTextW")
	procIsWindowVisible    = user32.NewProc("IsWindowVisible")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procPostMessageW       = user32.NewProc("PostMessageW")
	procGetWindowRect      = user32.NewProc("GetWindowRect")
	procPrintWindow        = user32.NewProc("PrintWindow")
	procEnumChildWindows   = user32.NewProc("EnumChildWindows")
	procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
)

// findWindow returns a visible top-level window whose title contains
// text, 0 for none.
func findWindow(text string) uintptr {
	var found uintptr
	cb := syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		var buf [512]uint16
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 && strings.Contains(syscall.UTF16ToString(buf[:n]), text) {
			found = hwnd
			return 0
		}
		return 1
	})
	procEnumWindows.Call(cb, 0)
	return found
}

// grabber copies what a window shows into memory, as the window
// manager has it: covered by other windows or not.
type grabber struct {
	screen, mem, bitmap uintptr
	bits                unsafe.Pointer
	w, h                int
}

func (g *grabber) resize(w, h int) {
	if g.screen == 0 {
		g.screen, _, _ = procGetDC.Call(0)
		g.mem, _, _ = procCreateCompatibleDC.Call(g.screen)
	}
	if w == g.w && h == g.h {
		return
	}
	if g.bitmap != 0 {
		procDeleteObject.Call(g.bitmap)
	}
	info := struct {
		size          uint32
		width, height int32
		planes, bits  uint16
		compression   uint32
		rest          [5]uint32
	}{width: int32(w), height: -int32(h), planes: 1, bits: 32}
	info.size = uint32(unsafe.Sizeof(info))
	g.bitmap, _, _ = procCreateDIBSection.Call(g.mem, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&g.bits)), 0, 0)
	procSelectObject.Call(g.mem, g.bitmap)
	g.w, g.h = w, h
}

// grab returns the pixels of a square in the middle of a window, nil
// while the window has no size.
func (g *grabber) grab(hwnd uintptr, size int) []uint32 {
	var rect [4]int32
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	w, h := int(rect[2]-rect[0]), int(rect[3]-rect[1])
	if w < size || h < size {
		return nil
	}
	g.resize(w, h)
	const renderFullContent = 2
	if r, _, _ := procPrintWindow.Call(hwnd, g.mem, renderFullContent); r == 0 {
		return nil
	}
	all := unsafe.Slice((*uint32)(g.bits), w*h)
	out := make([]uint32, 0, size*size)
	x0, y0 := (w-size)/2, (h-size)/2
	for y := y0; y < y0+size; y++ {
		out = append(out, all[y*w+x0:y*w+x0+size]...)
	}
	return out
}

// pictured reports whether pixels are a picture's rather than a flat
// background: whether they hold more than a few colors.
func pictured(pix []uint32) bool {
	colors := map[uint32]bool{}
	for i := 0; i < len(pix); i += 7 {
		colors[pix[i]&0xF8F8F8] = true
		if len(colors) > 24 {
			return true
		}
	}
	return false
}

func sum(pix []uint32) uint64 {
	var s uint64
	for i, p := range pix {
		s = s*31 + uint64(p) + uint64(i)
	}
	return s
}

// press presses and lets go of a key in a window, by the messages the
// keyboard would send it: nothing goes to any other window.
func press(hwnd uintptr, vk uintptr) {
	const keyDown, keyUp = 0x100, 0x101
	targets := []uintptr{hwnd}
	// The keyboard's window may be one inside it.
	cb := syscall.NewCallback(func(child, _ uintptr) uintptr {
		targets = append(targets, child)
		return 1
	})
	procEnumChildWindows.Call(hwnd, cb, 0)
	for _, t := range targets {
		procPostMessageW.Call(t, keyDown, vk, 1|0x014D0000)
		procPostMessageW.Call(t, keyUp, vk, 1|0xC14D0000)
	}
}

func main() {
	runs := flag.Int("n", 5, "runs")
	keys := flag.Int("keys", 0, "after the first picture, go through this many more with the right arrow")
	title := flag.String("title", "", "text of the window's title to look for (default: the file's name)")
	flag.Parse()
	if flag.NArg() < 2 {
		fmt.Fprintln(os.Stderr, "usage: startbench [-n runs] [-keys N] viewer.exe picture")
		os.Exit(2)
	}
	procSetProcessDPIAware.Call()
	viewer, picture := flag.Arg(0), flag.Arg(1)
	want := *title
	if want == "" {
		want = filepath.Base(picture)
	}
	const size = 96
	g := &grabber{}
	var firsts []time.Duration
	var steps []time.Duration
	for run := 0; run < *runs; run++ {
		cmd := exec.Command(viewer, append(flag.Args()[2:], picture)...)
		start := time.Now()
		if err := cmd.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		var hwnd uintptr
		var shown time.Duration
		for time.Since(start) < 20*time.Second {
			if hwnd == 0 {
				hwnd = findWindow(want)
			}
			if hwnd != 0 {
				if pix := g.grab(hwnd, size); pix != nil && pictured(pix) {
					shown = time.Since(start)
					break
				}
			}
			time.Sleep(time.Millisecond)
		}
		if shown == 0 {
			fmt.Println("no picture showed within 20 s")
		} else {
			firsts = append(firsts, shown)
			fmt.Printf("run %d: first picture after %.0f ms\n", run+1, float64(shown.Microseconds())/1000)
		}
		if hwnd != 0 && *keys > 0 && shown != 0 {
			time.Sleep(1500 * time.Millisecond) // let it settle, and preload what it preloads
			for k := 0; k < *keys; k++ {
				before := sum(g.grab(hwnd, size))
				at := time.Now()
				const vkRight = 0x27
				press(hwnd, vkRight)
				for time.Since(at) < 5*time.Second {
					if sum(g.grab(hwnd, size)) != before {
						steps = append(steps, time.Since(at))
						break
					}
					time.Sleep(500 * time.Microsecond)
				}
				time.Sleep(350 * time.Millisecond)
			}
		}
		// Close it, and wait for it to go.
		if hwnd != 0 {
			const wmClose = 0x10
			procPostMessageW.Call(hwnd, wmClose, 0, 0)
		}
		done := make(chan struct{})
		go func() { cmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			cmd.Process.Kill()
		}
		for i := 0; i < 100 && findWindow(want) != 0; i++ {
			time.Sleep(30 * time.Millisecond)
		}
		time.Sleep(400 * time.Millisecond)
	}
	report("first picture", firsts)
	report("next picture", steps)
}

func report(what string, d []time.Duration) {
	if len(d) == 0 {
		return
	}
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	msOf := func(t time.Duration) float64 { return float64(t.Microseconds()) / 1000 }
	fmt.Printf("%s: median %.0f ms, best %.0f ms, worst %.0f ms (%d samples)\n", what, msOf(d[len(d)/2]), msOf(d[0]), msOf(d[len(d)-1]), len(d))
}
