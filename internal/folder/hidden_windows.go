package folder

import (
	"io/fs"
	"syscall"
)

func isHidden(info fs.FileInfo) bool {
	d, ok := info.Sys().(*syscall.Win32FileAttributeData)
	return ok && d.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0
}
