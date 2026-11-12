package main

import (
	"os"
	"runtime"
	"runtime/pprof"
	"syscall"
	"time"
)

// attachConsole gives the program the console it was started from, if
// any, for what it prints: built as a windowed program, it starts
// without one, so that opening a picture flashes no console window.
func attachConsole() {
	if _, err := os.Stdout.Stat(); err == nil {
		// Output already goes somewhere, as to a pipe or a file.
		return
	}
	const attachParentProcess = ^uintptr(0)
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole")
	if r, _, _ := proc.Call(attachParentProcess); r == 0 {
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}

// profile writes a CPU profile of the program's first moments to the
// file MYIMAGES_PROFILE names, for finding what its start waits on.
func profile() {
	path := os.Getenv("MYIMAGES_PROFILE")
	if path == "" {
		return
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	runtime.SetCPUProfileRate(4000)
	if pprof.StartCPUProfile(f) != nil {
		f.Close()
		return
	}
	time.AfterFunc(600*time.Millisecond, func() {
		pprof.StopCPUProfile()
		f.Close()
	})
}
