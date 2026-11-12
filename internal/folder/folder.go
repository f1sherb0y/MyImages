// Package folder lists the pictures of a directory, in the order a file
// manager shows them.
package folder

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// File is a picture in a folder.
type File struct {
	Name string
	Size int64
	Mod  time.Time
	// key orders names: lower case, compared with numbers by value.
	key string
}

// Order is what a listing is sorted by.
type Order struct {
	// By is "name", "date", "size" or "type".
	By         string
	Descending bool
}

// List reads the pictures of a directory: the files whose extension
// (lower case, without the dot) known reports. Hidden files are left out
// unless hidden is set.
func List(dir string, known func(ext string) bool, hidden bool, order Order) ([]File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	files := make([]File, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !known(Ext(name)) {
			continue
		}
		// On Windows the listing brought the details along: no file is
		// opened for them.
		info, err := e.Info()
		if err != nil {
			continue
		}
		if !hidden && (isHidden(info) || strings.HasPrefix(name, ".")) {
			continue
		}
		files = append(files, File{Name: name, Size: info.Size(), Mod: info.ModTime(), key: strings.ToLower(name)})
	}
	Sort(files, order)
	return files, nil
}

// Ext is a file name's extension in lower case, without its dot.
func Ext(name string) string {
	return strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
}

// Sort orders files.
func Sort(files []File, order Order) {
	cmp := func(a, b File) int { return natural(a.key, b.key) }
	switch order.By {
	case "date":
		cmp = func(a, b File) int {
			if c := a.Mod.Compare(b.Mod); c != 0 {
				return c
			}
			return natural(a.key, b.key)
		}
	case "size":
		cmp = func(a, b File) int {
			if a.Size != b.Size {
				if a.Size < b.Size {
					return -1
				}
				return 1
			}
			return natural(a.key, b.key)
		}
	case "type":
		cmp = func(a, b File) int {
			if c := strings.Compare(Ext(a.key), Ext(b.key)); c != 0 {
				return c
			}
			return natural(a.key, b.key)
		}
	}
	if order.Descending {
		asc := cmp
		cmp = func(a, b File) int { return asc(b, a) }
	}
	slices.SortStableFunc(files, cmp)
}

// Index finds a file by name, whatever its case; -1 when it is not
// there.
func Index(files []File, name string) int {
	key := strings.ToLower(name)
	for i := range files {
		if files[i].key == key {
			return i
		}
	}
	return -1
}

// natural compares names as file managers do: runs of digits by their
// value, so that "img9" comes before "img10".
func natural(a, b string) int {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		ca, cb := a[i], b[j]
		if isDigit(ca) && isDigit(cb) {
			// Skip leading zeros, then the longer run is the larger.
			si := i
			for i < len(a) && a[i] == '0' {
				i++
			}
			sj := j
			for j < len(b) && b[j] == '0' {
				j++
			}
			ni := i
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			nj := j
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			if (i - ni) != (j - nj) {
				return (i - ni) - (j - nj)
			}
			if c := strings.Compare(a[ni:i], b[nj:j]); c != 0 {
				return c
			}
			// Equal values: fewer leading zeros first.
			if c := (ni - si) - (nj - sj); c != 0 {
				return c
			}
			continue
		}
		if ca != cb {
			return int(ca) - int(cb)
		}
		i++
		j++
	}
	return (len(a) - i) - (len(b) - j)
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
