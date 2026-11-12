// Package shell is what MyImages asks of the desktop besides drawing:
// the Recycle Bin, the clipboard's pictures, the file manager, the
// wallpaper. The functions block: call them off the main thread.
package shell

import "errors"

// ErrUnsupported is returned where a system has no such thing.
var ErrUnsupported = errors.New("not supported on this system")
