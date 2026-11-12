package codec

import (
	"fmt"
	"math"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procPropVariantClear = ole32.NewProc("PropVariantClear")

const (
	// IWICBitmapFrameDecode
	wicFrameGetThumbnail = 10
	// IWICBitmapDecoder
	wicDecoderGetThumbnail = 11
	wicDecoderGetPreview   = 9
)

// propVariant is the system's value of any type: its type, then the
// value 8 bytes in.
type propVariant struct {
	vt   uint16
	_    [6]byte
	val  uint64
	val2 uint64
}

const (
	vtR8       = 5
	vtUI1      = 17
	vtUI2      = 18
	vtUI4      = 19
	vtUI8      = 21
	vtLPSTR    = 30
	vtLPWSTR   = 31
	vtFiletime = 64
	vtVector   = 0x1000
)

// query reads a value of a frame's metadata by name, with whether it
// was there. The caller clears it.
func query(reader *object, name string, v *propVariant) bool {
	*v = propVariant{}
	return !reader.call(wicGetMetadataByName, uintptr(unsafe.Pointer(utf16(name))), uintptr(unsafe.Pointer(v))).failed() && v.vt != 0
}

func (v *propVariant) clear() { procPropVariantClear.Call(uintptr(unsafe.Pointer(v))) }

// text is the value as a string, "" for one that is none.
func (v *propVariant) text() string {
	switch v.vt {
	case vtLPWSTR:
		if v.val != 0 {
			return strings.TrimSpace(windows.UTF16PtrToString((*uint16)(unsafe.Pointer(uintptr(v.val)))))
		}
	case vtLPSTR:
		if v.val != 0 {
			return strings.TrimSpace(windows.BytePtrToString((*byte)(unsafe.Pointer(uintptr(v.val)))))
		}
	}
	return ""
}

// number is the value as a number, with whether it is one.
func (v *propVariant) number() (float64, bool) {
	switch v.vt {
	case vtR8:
		return math.Float64frombits(v.val), true
	case vtUI1:
		return float64(uint8(v.val)), true
	case vtUI2:
		return float64(uint16(v.val)), true
	case vtUI4:
		return float64(uint32(v.val)), true
	case vtUI8:
		// A fraction, as EXIF writes them: the numerator, then the
		// denominator.
		num, den := uint32(v.val), uint32(v.val>>32)
		if den != 0 {
			return float64(num) / float64(den), true
		}
	}
	return 0, false
}

// Metadata reads what a file says of its picture: when and with what it
// was taken. Files that say nothing have none.
func Metadata(path string) (fields []Field) {
	onCOM(func() {
		f, err := factory()
		if err != nil {
			return
		}
		var dec *object
		const genericRead = 0x80000000
		if f.call(wicCreateDecoderFromFilename, uintptr(unsafe.Pointer(utf16(path))), 0, genericRead, 0,
			uintptr(unsafe.Pointer(&dec))).failed() {
			return
		}
		defer dec.release()
		var frame *object
		if dec.call(wicGetFrame, 0, uintptr(unsafe.Pointer(&frame))).failed() {
			return
		}
		defer frame.release()
		var reader *object
		if frame.call(wicGetMetadataQueryReader, uintptr(unsafe.Pointer(&reader))).failed() || reader == nil {
			return
		}
		defer reader.release()

		var v propVariant
		add := func(key, value string) {
			if value != "" {
				fields = append(fields, Field{key, value})
			}
		}
		str := func(name string) string {
			if !query(reader, name, &v) {
				return ""
			}
			defer v.clear()
			return v.text()
		}
		num := func(name string) (float64, bool) {
			if !query(reader, name, &v) {
				return 0, false
			}
			defer v.clear()
			return v.number()
		}
		// The names are the system's, which it finds in EXIF, XMP and
		// the like, whichever the file has.
		if query(reader, "System.Photo.DateTaken", &v) {
			if v.vt == vtFiletime {
				ft := windows.Filetime{LowDateTime: uint32(v.val), HighDateTime: uint32(v.val >> 32)}
				add("taken", time.Unix(0, ft.Nanoseconds()).Format("2006-01-02 15:04:05"))
			}
			v.clear()
		}
		maker, model := str("System.Photo.CameraManufacturer"), str("System.Photo.CameraModel")
		if model != "" && maker != "" && !strings.HasPrefix(strings.ToLower(model), strings.ToLower(strings.Fields(maker)[0])) {
			model = maker + " " + model
		}
		add("camera", model)
		add("lens", str("System.Photo.LensModel"))
		if t, ok := num("System.Photo.ExposureTime"); ok && t > 0 {
			if t < 1 {
				add("exposure", fmt.Sprintf("1/%.0f s", 1/t))
			} else {
				add("exposure", fmt.Sprintf("%.1f s", t))
			}
		}
		if n, ok := num("System.Photo.FNumber"); ok && n > 0 {
			add("aperture", fmt.Sprintf("f/%.1f", n))
		}
		if n, ok := num("System.Photo.ISOSpeed"); ok && n > 0 {
			add("iso", fmt.Sprintf("%.0f", n))
		}
		if n, ok := num("System.Photo.FocalLength"); ok && n > 0 {
			s := fmt.Sprintf("%.1f mm", n)
			if eq, ok := num("System.Photo.FocalLengthInFilm"); ok && eq > 0 {
				s += fmt.Sprintf("  (%.0f mm)", eq)
			}
			add("focal", s)
		}
		if n, ok := num("System.Photo.Flash"); ok {
			if int(n)&1 != 0 {
				add("flash", "✓")
			}
		}
		lat, okLat := degrees(reader, "System.GPS.Latitude", "System.GPS.LatitudeRef", "S")
		lon, okLon := degrees(reader, "System.GPS.Longitude", "System.GPS.LongitudeRef", "W")
		if okLat && okLon {
			add("location", fmt.Sprintf("%.6f, %.6f", lat, lon))
		}
		add("title", str("System.Title"))
		add("author", firstOf(reader, "System.Author"))
		add("copyright", str("System.Copyright"))
		add("software", str("System.ApplicationName"))
	})
	return fields
}

// firstOf is the first string of a value that is a list of them.
func firstOf(reader *object, name string) string {
	var v propVariant
	if !query(reader, name, &v) {
		return ""
	}
	defer v.clear()
	if v.vt == vtVector|vtLPWSTR && uint32(v.val) > 0 && v.val2 != 0 {
		first := *(**uint16)(unsafe.Pointer(uintptr(v.val2)))
		return strings.TrimSpace(windows.UTF16PtrToString(first))
	}
	return v.text()
}

// degrees reads a place's latitude or longitude: degrees, minutes and
// seconds, negative on the side of the world ref names as negative.
func degrees(reader *object, name, refName, negative string) (float64, bool) {
	var v propVariant
	if !query(reader, name, &v) {
		return 0, false
	}
	defer v.clear()
	n := int(uint32(v.val))
	if v.vt != vtVector|vtR8 || n < 3 || v.val2 == 0 {
		return 0, false
	}
	parts := unsafe.Slice((*float64)(unsafe.Pointer(uintptr(v.val2))), n)
	d := parts[0] + parts[1]/60 + parts[2]/3600
	var r propVariant
	if query(reader, refName, &r) {
		if strings.EqualFold(r.text(), negative) {
			d = -d
		}
		r.clear()
	}
	return d, true
}

// Thumbnail returns a small picture of a file, at least px pixels on its
// shorter side where the file has as many: the one a camera put in the
// file where it is large enough, which costs a fraction of decoding the
// picture, or else the picture decoded reduced.
func Thumbnail(path string, px int) (*Picture, error) {
	var p *Picture
	onCOM(func() {
		f, err := factory()
		if err != nil {
			return
		}
		var dec *object
		const genericRead = 0x80000000
		if f.call(wicCreateDecoderFromFilename, uintptr(unsafe.Pointer(utf16(path))), 0, genericRead, 0,
			uintptr(unsafe.Pointer(&dec))).failed() {
			return
		}
		defer dec.release()
		var frame *object
		if dec.call(wicGetFrame, 0, uintptr(unsafe.Pointer(&frame))).failed() {
			return
		}
		defer frame.release()
		var thumb *object
		if frame.call(wicFrameGetThumbnail, uintptr(unsafe.Pointer(&thumb))).failed() || thumb == nil {
			return
		}
		defer thumb.release()
		var w, h uint32
		if thumb.call(wicGetSize, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h))).failed() {
			return
		}
		// Enough of it for the square shown, or nearly.
		if int(min(w, h))*4 < px*3 || w > 4096 || h > 4096 {
			return
		}
		var conv *object
		if f.call(wicCreateFormatConverter, uintptr(unsafe.Pointer(&conv))).failed() {
			return
		}
		defer conv.release()
		if conv.call(wicInitialize, uintptr(unsafe.Pointer(thumb)), uintptr(unsafe.Pointer(&wicPRGBA)), 0, 0, 0, 0).failed() {
			return
		}
		pix := make([]byte, 4*w*h)
		if conv.call(wicCopyPixels, 0, uintptr(4*w), uintptr(len(pix)), uintptr(unsafe.Pointer(&pix[0]))).failed() {
			return
		}
		p = &Picture{W: int(w), H: int(h), FullW: int(w), FullH: int(h), Pix: pix, Orient: wicOrientation(frame), Frames: 1, Via: "wic"}
	})
	if p != nil {
		return p, nil
	}
	// Decoders reduce by halves: twice the square asked for leaves no
	// side short.
	return Decode(path, Options{MaxW: 2 * px, MaxH: 2 * px})
}
