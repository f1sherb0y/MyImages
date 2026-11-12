package shell

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procSHFileOperation      = shell32.NewProc("SHFileOperationW")
	procShellExecuteEx       = shell32.NewProc("ShellExecuteExW")
	procSystemParametersInfo = user32.NewProc("SystemParametersInfoW")
	procOpenClipboard        = user32.NewProc("OpenClipboard")
	procCloseClipboard       = user32.NewProc("CloseClipboard")
	procEmptyClipboard       = user32.NewProc("EmptyClipboard")
	procSetClipboardData     = user32.NewProc("SetClipboardData")
	procGlobalAlloc          = kernel32.NewProc("GlobalAlloc")
	procGlobalLock           = kernel32.NewProc("GlobalLock")
	procGlobalUnlock         = kernel32.NewProc("GlobalUnlock")
	procGlobalFree           = kernel32.NewProc("GlobalFree")
)

// Delete removes a file: to the Recycle Bin when recycle is set, where
// the user can get it back.
func Delete(path string, recycle bool) error {
	if !recycle {
		return os.Remove(path)
	}
	// The list of files ends with an empty name.
	from, err := windows.UTF16FromString(path)
	if err != nil {
		return err
	}
	from = append(from, 0)
	const (
		foDelete     = 3
		fofSilent    = 0x4
		fofNoConfirm = 0x10
		fofAllowUndo = 0x40
		fofNoErrorUI = 0x400
	)
	op := struct {
		hwnd    uintptr
		fn      uint32
		from    *uint16
		to      *uint16
		flags   uint16
		aborted int32
		mapping uintptr
		title   *uint16
	}{fn: foDelete, from: &from[0], flags: fofSilent | fofNoConfirm | fofAllowUndo | fofNoErrorUI}
	r, _, _ := procSHFileOperation.Call(uintptr(unsafe.Pointer(&op)))
	runtime.KeepAlive(from)
	if r != 0 {
		return fmt.Errorf("the file could not be deleted (error 0x%X)", r)
	}
	if op.aborted != 0 {
		return errors.New("the file was not deleted")
	}
	return nil
}

// Reveal shows a file in the system's file manager, chosen.
func Reveal(owner uintptr, path string) error {
	cmd := exec.Command("explorer.exe")
	// Explorer reads its own command line: the path quoted as it is.
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: "explorer.exe /select,\"" + path + "\""}
	return cmd.Start()
}

// OpenWith asks which program opens a file.
func OpenWith(owner uintptr, path string) error {
	return exec.Command("rundll32.exe", "shell32.dll,OpenAs_RunDLL", path).Start()
}

// Print prints a file with the program that prints its type.
func Print(owner uintptr, path string) error { return execute(owner, "print", path) }

// Properties shows the system's properties of a file.
func Properties(owner uintptr, path string) error { return execute(owner, "properties", path) }

func execute(owner uintptr, verb, path string) error {
	v, _ := windows.UTF16PtrFromString(verb)
	f, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const invokeIDList = 0xC
	info := struct {
		size       uint32
		mask       uint32
		hwnd       uintptr
		verb       *uint16
		file       *uint16
		parameters *uint16
		directory  *uint16
		show       int32
		instApp    uintptr
		idList     uintptr
		class      *uint16
		keyClass   uintptr
		hotKey     uint32
		icon       uintptr
		process    uintptr
	}{mask: invokeIDList, hwnd: owner, verb: v, file: f, show: 1}
	info.size = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return e
	}
	return nil
}

// SetWallpaper makes a picture the desktop's background.
func SetWallpaper(owner uintptr, path string) error {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	const setDeskWallpaper = 0x14
	const updateAndTell = 3
	if r, _, e := procSystemParametersInfo.Call(setDeskWallpaper, 0, uintptr(unsafe.Pointer(p)), updateAndTell); r == 0 {
		return e
	}
	return nil
}

// CopyImage puts a file on the clipboard, with its pixels (premultiplied
// RGBA, rows of 4*w bytes) when there are some: file managers paste the
// file, editors the picture.
func CopyImage(path string, pix []byte, w, h int) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if r, _, e := procOpenClipboard.Call(0); r == 0 {
		return e
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	name, err := windows.UTF16FromString(path)
	if err != nil {
		return err
	}
	// DROPFILES: the offset of the names, a point, two flags, then the
	// names, the list ending with an empty one.
	const headerSize = 20
	drop := make([]byte, headerSize+2*(len(name)+1))
	drop[0] = headerSize
	drop[16] = 1 // wide characters
	for i, c := range name {
		drop[headerSize+2*i] = byte(c)
		drop[headerSize+2*i+1] = byte(c >> 8)
	}
	const cfHDrop = 15
	if err := setClipboard(cfHDrop, drop); err != nil {
		return err
	}
	if len(pix) == 0 || w <= 0 || h <= 0 || len(pix) < 4*w*h {
		return nil
	}
	// A bitmap of 32 bits a pixel, bottom row first, in blue, green,
	// red: over white where the picture is transparent, as programs that
	// paste it ignore the fourth byte.
	const infoSize = 40
	dib := make([]byte, infoSize+4*w*h)
	put32 := func(at int, v uint32) {
		dib[at], dib[at+1], dib[at+2], dib[at+3] = byte(v), byte(v>>8), byte(v>>16), byte(v>>24)
	}
	put32(0, infoSize)
	put32(4, uint32(w))
	put32(8, uint32(h))
	dib[12] = 1  // planes
	dib[14] = 32 // bits
	put32(20, uint32(4*w*h))
	for y := 0; y < h; y++ {
		src := pix[(h-1-y)*4*w : (h-y)*4*w]
		dst := dib[infoSize+y*4*w:]
		for i := 0; i < len(src); i += 4 {
			a := 255 - uint32(src[i+3])
			dst[i] = uint8(uint32(src[i+2]) + a)
			dst[i+1] = uint8(uint32(src[i+1]) + a)
			dst[i+2] = uint8(uint32(src[i]) + a)
			dst[i+3] = 255
		}
	}
	const cfDIB = 8
	return setClipboard(cfDIB, dib)
}

// setClipboard gives the clipboard, which is open, data in a format.
func setClipboard(format uintptr, data []byte) error {
	const movable = 2
	mem, _, e := procGlobalAlloc.Call(movable, uintptr(len(data)))
	if mem == 0 {
		return e
	}
	ptr, _, e := procGlobalLock.Call(mem)
	if ptr == 0 {
		procGlobalFree.Call(mem)
		return e
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(ptr)), len(data)), data)
	procGlobalUnlock.Call(mem)
	if r, _, e := procSetClipboardData.Call(format, mem); r == 0 {
		procGlobalFree.Call(mem)
		return e
	}
	// The clipboard owns the memory now.
	return nil
}

// DefaultApps opens the system's settings where the user chooses which
// program opens which files.
func DefaultApps() error {
	return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", "ms-settings:defaultapps").Start()
}
