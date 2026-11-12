package codec

import (
	"slices"
	"strings"
)

// systemExts are the extensions of the formats the system's codecs
// decode, with the extensions Windows installs or its users add from the
// Store (HEIF, AV1, WebP, JPEG XL, camera raw).
var systemExts = strings.Fields(`
	jpg jpeg jpe jfif jif exif
	png apng bmp dib rle gif gifv
	tif tiff fax
	ico cur
	dds
	jxr hdp wdp
	heic heif hif heics heifs avci avcs
	avif avifs
	webp
	jxl
`)

// rawList are the extensions of cameras' raw files: the system's raw
// codec decodes most, and the preview the camera put in the file shows
// for the others.
var rawList = strings.Fields(`
	3fr ari arw bay cap cr2 cr3 crw dcr dcs dng drf eip erf fff gpr iiq
	k25 kdc mdc mef mos mrw nef nrw orf ori pef ptx pxn r3d raf raw rw2
	rwl rwz sr2 srf srw x3f
`)

// rareList are extensions with no decoder of their own, which show
// where what they hold is of another format (an MJPEG's first frame is a
// JPEG) or the system has a handler for them.
var rareList = strings.Fields(`mjpeg flif obm`)

var rawExts = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range rawList {
		m[e] = true
	}
	return m
}()

var known = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range systemExts {
		m[e] = true
	}
	for _, e := range rawList {
		m[e] = true
	}
	for _, e := range rareList {
		m[e] = true
	}
	return m
}()

// Known reports whether files of an extension (lower case, without the
// dot) are pictures the program shows.
func Known(ext string) bool { return known[ext] || goDecoders[ext] != nil }

// Extensions lists the extensions of the pictures the program shows, in
// order.
func Extensions() []string {
	all := map[string]bool{}
	for e := range known {
		all[e] = true
	}
	for e := range goDecoders {
		all[e] = true
	}
	out := make([]string, 0, len(all))
	for e := range all {
		out = append(out, e)
	}
	slices.Sort(out)
	return out
}

// Field is something a file says of its picture, as the camera that
// took it: Key names it ("camera", "exposure") and Value is written for
// people.
type Field struct {
	Key, Value string
}
