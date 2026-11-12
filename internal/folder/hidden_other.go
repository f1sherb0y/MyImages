//go:build !windows

package folder

import "io/fs"

func isHidden(fs.FileInfo) bool { return false }
