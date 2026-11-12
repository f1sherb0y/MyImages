//go:build !windows

package shell

import "os"

// Delete removes a file: to the trash when recycle is set.
func Delete(path string, recycle bool) error {
	if recycle {
		return ErrUnsupported
	}
	return os.Remove(path)
}

func Reveal(owner uintptr, path string) error       { return ErrUnsupported }
func OpenWith(owner uintptr, path string) error     { return ErrUnsupported }
func Print(owner uintptr, path string) error        { return ErrUnsupported }
func Properties(owner uintptr, path string) error   { return ErrUnsupported }
func SetWallpaper(owner uintptr, path string) error { return ErrUnsupported }

// CopyImage puts a file and its pixels on the clipboard.
func CopyImage(path string, pix []byte, w, h int) error { return ErrUnsupported }

// DefaultApps opens the system's settings of default programs.
func DefaultApps() error { return ErrUnsupported }

// RegisterDefaults tells the system which kinds of files the program
// opens.
func RegisterDefaults(exe string, exts []string) error { return ErrUnsupported }

// IsDefaultFor reports whether the program is the default for an
// extension.
func IsDefaultFor(ext string) bool { return false }
