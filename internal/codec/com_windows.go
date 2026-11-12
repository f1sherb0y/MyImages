package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ole32 = windows.NewLazySystemDLL("ole32.dll")

	procCoInitializeEx   = ole32.NewProc("CoInitializeEx")
	procCoCreateInstance = ole32.NewProc("CoCreateInstance")
)

// guid reads a GUID as it is written, without braces.
func guid(s string) windows.GUID {
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	if err != nil || len(b) != 16 {
		panic("codec: bad GUID " + s)
	}
	g := windows.GUID{
		Data1: binary.BigEndian.Uint32(b),
		Data2: binary.BigEndian.Uint16(b[4:]),
		Data3: binary.BigEndian.Uint16(b[6:]),
	}
	copy(g.Data4[:], b[8:])
	return g
}

// object is a COM object as its users hold it: a pointer to the pointer
// to its table of methods. The memory is the system's, not Go's.
type object struct {
	vtbl *[64]uintptr
}

// call calls the method at an index of the object's table, the first
// three being IUnknown's, with the object before the arguments.
//
//go:uintptrescapes
func (o *object) call(method int, args ...uintptr) hresult {
	var a [10]uintptr
	if len(args) >= len(a) {
		panic("codec: too many arguments to a COM method")
	}
	a[0] = uintptr(unsafe.Pointer(o))
	n := 1 + copy(a[1:], args)
	r, _, _ := syscall.SyscallN(o.vtbl[method], a[:n]...)
	return hresult(r)
}

// release gives up a reference to the object, which may be nil.
func (o *object) release() {
	if o != nil {
		o.call(2)
	}
}

// query is another interface of the object, to be released, or nil.
func (o *object) query(iid *windows.GUID) *object {
	var out *object
	if o.call(0, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out))).failed() {
		return nil
	}
	return out
}

// hresult is what a COM call returns: negative for a failure.
type hresult uint32

func (hr hresult) failed() bool { return int32(hr) < 0 }

func (hr hresult) err() error {
	if !hr.failed() {
		return nil
	}
	if hr == 0x88982F50 || hr == 0x88982F07 { // WINCODEC_ERR_COMPONENTNOTFOUND, _UNKNOWNIMAGEFORMAT
		return ErrUnknown
	}
	return comError(hr)
}

type comError hresult

func (e comError) Error() string {
	if msg := windows.Errno(e).Error(); !strings.HasPrefix(msg, "winapi error") {
		return msg
	}
	return fmt.Sprintf("error 0x%08X", uint32(e))
}

// The system's codecs are COM objects, which a thread must have entered
// COM to use. Work on them runs on threads kept for it.
var (
	comOnce sync.Once
	comJobs chan func()
)

// onCOM runs fn on a thread that entered COM, and waits for it.
func onCOM(fn func()) {
	comOnce.Do(func() {
		comJobs = make(chan func())
		for range max(2, min(runtime.NumCPU(), 4)) {
			go func() {
				runtime.LockOSThread()
				const multithreaded = 0
				procCoInitializeEx.Call(0, multithreaded)
				for job := range comJobs {
					job()
				}
			}()
		}
	})
	done := make(chan struct{})
	comJobs <- func() {
		defer close(done)
		fn()
	}
	<-done
}

func utf16(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString("")
	}
	return p
}
