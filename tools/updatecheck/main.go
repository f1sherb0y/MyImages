// Updatecheck tries the update button in a real window: it waits for
// the "Restart to Update" button to show in a running MyImages window,
// clicks it, and reports whether the program came back as another
// process showing the same picture.
//
//	updatecheck -title picture.jpg [-wait 60s]
//
// The button is found by its look: the widest run of one saturated color
// in the strip at the top of the window, which nothing else there has.
// The click is posted to that window alone.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32 = syscall.NewLazyDLL("user32.dll")
	gdi32  = syscall.NewLazyDLL("gdi32.dll")

	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowTextW           = user32.NewProc("GetWindowTextW")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowRect            = user32.NewProc("GetWindowRect")
	procScreenToClient           = user32.NewProc("ScreenToClient")
	procPrintWindow              = user32.NewProc("PrintWindow")
	procPostMessageW             = user32.NewProc("PostMessageW")
	procGetDC                    = user32.NewProc("GetDC")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procSetProcessDPIAware       = user32.NewProc("SetProcessDPIAware")
	procCreateCompatibleDC       = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection         = gdi32.NewProc("CreateDIBSection")
	procSelectObject             = gdi32.NewProc("SelectObject")
	procDeleteObject             = gdi32.NewProc("DeleteObject")
	procDeleteDC                 = gdi32.NewProc("DeleteDC")
)

var (
	wantTitle string
	found     uintptr
	findCB    = syscall.NewCallback(func(hwnd, _ uintptr) uintptr {
		if v, _, _ := procIsWindowVisible.Call(hwnd); v == 0 {
			return 1
		}
		var buf [512]uint16
		n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
		if n > 0 && strings.Contains(syscall.UTF16ToString(buf[:n]), wantTitle) {
			found = hwnd
			return 0
		}
		return 1
	})
)

func findWindow(title string) uintptr {
	wantTitle, found = title, 0
	procEnumWindows.Call(findCB, 0)
	return found
}

func pid(hwnd uintptr) uint32 {
	var id uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&id)))
	return id
}

// strip returns the pixels of the top rows of a window, with its width.
func strip(hwnd uintptr, rows int) ([]uint32, int) {
	var rect [4]int32
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	w, h := int(rect[2]-rect[0]), int(rect[3]-rect[1])
	if w <= 0 || h < rows {
		return nil, 0
	}
	screen, _, _ := procGetDC.Call(0)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	info := struct {
		size          uint32
		width, height int32
		planes, bits  uint16
		compression   uint32
		rest          [5]uint32
	}{width: int32(w), height: -int32(h), planes: 1, bits: 32}
	info.size = uint32(unsafe.Sizeof(info))
	var bits unsafe.Pointer
	bm, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bm == 0 {
		return nil, 0
	}
	defer procDeleteObject.Call(bm)
	procSelectObject.Call(mem, bm)
	const renderFullContent = 2
	procPrintWindow.Call(hwnd, mem, renderFullContent)
	all := unsafe.Slice((*uint32)(bits), w*h)
	return append([]uint32(nil), all[:w*rows]...), w
}

// button finds the update button in the strip: the middle of the widest
// run of saturated pixels, in pixels of the window; ok when it is wide
// enough to be a button.
func button(pix []uint32, w int) (x, y int, ok bool) {
	best := 0
	for row := 0; row < len(pix)/w; row++ {
		run := 0
		for col := 0; col <= w; col++ {
			saturated := false
			if col < w {
				p := pix[row*w+col]
				r, g, b := int(p>>16&0xFF), int(p>>8&0xFF), int(p&0xFF)
				saturated = max(r, g, b)-min(r, g, b) > 90
			}
			if saturated {
				run++
				continue
			}
			if run > best {
				best, x, y = run, col-run/2, row
			}
			run = 0
		}
	}
	return x, y, best >= 50
}

func main() {
	title := flag.String("title", "", "text of the window's title, as the picture's name")
	wait := flag.Duration("wait", 60*time.Second, "how long to wait for the button")
	flag.Parse()
	if *title == "" {
		fmt.Fprintln(os.Stderr, "usage: updatecheck -title picture.jpg")
		os.Exit(2)
	}
	procSetProcessDPIAware.Call()
	start := time.Now()
	var hwnd uintptr
	var x, y int
	for {
		if time.Since(start) > *wait {
			fmt.Println("FAIL: no update button showed")
			os.Exit(1)
		}
		if hwnd = findWindow(*title); hwnd != 0 {
			if pix, w := strip(hwnd, 30); pix != nil {
				var ok bool
				if x, y, ok = button(pix, w); ok {
					break
				}
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	before := pid(hwnd)
	top := hwnd
	_ = top
	fmt.Printf("the button showed after %.1f s, at %d, %d of window %#x (process %d)\n", time.Since(start).Seconds(), x, y, hwnd, before)

	// The point in the window's client area.
	var rect [4]int32
	procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	pt := [2]int32{rect[0] + int32(x), rect[1] + int32(y)}
	// The window that draws may be one inside it: the one at the point.
	target := hwnd
	screen := uintptr(uint32(pt[0])) | uintptr(uint32(pt[1]))<<32
	if inner, _, _ := user32.NewProc("WindowFromPoint").Call(screen); inner != 0 {
		if root, _, _ := user32.NewProc("GetAncestor").Call(inner, 2); root == hwnd {
			target = inner
		} else {
			fmt.Println("another window covers the button: the click goes to the window itself")
		}
	}
	fmt.Printf("clicking window %#x\n", target)
	hwnd = target
	procScreenToClient.Call(hwnd, uintptr(unsafe.Pointer(&pt)))
	at := uintptr(uint32(uint16(pt[0])) | uint32(uint16(pt[1]))<<16)
	const mouseMove, buttonDown, buttonUp, leftDown = 0x200, 0x201, 0x202, 1
	procPostMessageW.Call(hwnd, mouseMove, 0, at)
	time.Sleep(60 * time.Millisecond)
	procPostMessageW.Call(hwnd, buttonDown, leftDown, at)
	time.Sleep(60 * time.Millisecond)
	procPostMessageW.Call(hwnd, buttonUp, 0, at)
	clicked := time.Now()

	for time.Since(clicked) < 20*time.Second {
		if next := findWindow(*title); next != 0 && pid(next) != before {
			fmt.Printf("OK: %.1f s after the click the picture shows again, in process %d\n", time.Since(clicked).Seconds(), pid(next))
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("FAIL: the program did not come back with the picture")
	os.Exit(1)
}
