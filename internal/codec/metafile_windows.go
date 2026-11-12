package codec

import (
	"encoding/binary"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	gdi32  = windows.NewLazySystemDLL("gdi32.dll")
	user32 = windows.NewLazySystemDLL("user32.dll")

	procSetEnhMetaFileBits    = gdi32.NewProc("SetEnhMetaFileBits")
	procSetWinMetaFileBits    = gdi32.NewProc("SetWinMetaFileBits")
	procGetEnhMetaFileHeader  = gdi32.NewProc("GetEnhMetaFileHeader")
	procPlayEnhMetaFile       = gdi32.NewProc("PlayEnhMetaFile")
	procDeleteEnhMetaFile     = gdi32.NewProc("DeleteEnhMetaFile")
	procCreateCompatibleDC    = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection      = gdi32.NewProc("CreateDIBSection")
	procSelectObject          = gdi32.NewProc("SelectObject")
	procDeleteObject          = gdi32.NewProc("DeleteObject")
	procDeleteDC              = gdi32.NewProc("DeleteDC")
	procSetStretchBltMode     = gdi32.NewProc("SetStretchBltMode")
	procGdiFlush              = gdi32.NewProc("GdiFlush")
	procFillRect              = user32.NewProc("FillRect")
	procGetStockObject        = gdi32.NewProc("GetStockObject")
	procGetDIBits             = gdi32.NewProc("GetDIBits")
	procGetObject             = gdi32.NewProc("GetObjectW")
	procSHCreateItemFromParse = windows.NewLazySystemDLL("shell32.dll").NewProc("SHCreateItemFromParsingName")
)

// Windows' metafiles, recordings of drawing calls, which the system
// plays back.
func init() {
	register(decodeEMF, func(h []byte) bool {
		return len(h) >= 44 && binary.LittleEndian.Uint32(h) == 1 && string(h[40:44]) == " EMF"
	}, "emf")
	register(decodeWMF, func(h []byte) bool {
		return len(h) >= 4 && binary.LittleEndian.Uint32(h) == 0x9AC6CDD7
	}, "wmf")
}

func decodeEMF(data []byte, opt Options) (*Picture, error) {
	h, _, _ := procSetEnhMetaFileBits.Call(uintptr(len(data)), uintptr(unsafe.Pointer(&data[0])))
	if h == 0 {
		return nil, errors.New("not a metafile the system can play")
	}
	defer procDeleteEnhMetaFile.Call(h)
	return playMetafile(h, 0, 0, "EMF", opt)
}

func decodeWMF(data []byte, opt Options) (*Picture, error) {
	if len(data) < 40 {
		return nil, ErrUnknown
	}
	var w, hgt float64
	body := data
	if binary.LittleEndian.Uint32(data) == 0x9AC6CDD7 {
		// A placeable metafile: its box and how many of its units make
		// an inch come before the recording.
		left, top := int16(binary.LittleEndian.Uint16(data[6:])), int16(binary.LittleEndian.Uint16(data[8:]))
		right, bottom := int16(binary.LittleEndian.Uint16(data[10:])), int16(binary.LittleEndian.Uint16(data[12:]))
		inch := float64(binary.LittleEndian.Uint16(data[14:]))
		if inch == 0 {
			inch = 1440
		}
		w = float64(int(right)-int(left)) / inch * 96
		hgt = float64(int(bottom)-int(top)) / inch * 96
		body = data[22:]
	}
	if w <= 0 || hgt <= 0 {
		w, hgt = 0, 0
	}
	// How the recording's units map to a page, where it does not say.
	const anisotropic = 8
	pict := struct {
		mm, xExt, yExt int32
		hmf            uintptr
	}{mm: anisotropic, xExt: int32(w / 96 * 2540), yExt: int32(hgt / 96 * 2540)}
	var hint unsafe.Pointer
	if w > 0 {
		hint = unsafe.Pointer(&pict)
	}
	h, _, _ := procSetWinMetaFileBits.Call(uintptr(len(body)), uintptr(unsafe.Pointer(&body[0])), 0, uintptr(hint))
	if h == 0 {
		return nil, errors.New("not a metafile the system can play")
	}
	defer procDeleteEnhMetaFile.Call(h)
	return playMetafile(h, w, hgt, "WMF", opt)
}

// playMetafile draws an enhanced metafile on white. w and h are its
// nominal size in pixels, or 0 for the one its header gives.
func playMetafile(h uintptr, w, hgt float64, format string, opt Options) (*Picture, error) {
	// ENHMETAHEADER: the bounds in device units at 8, the frame in
	// hundredths of a millimeter at 24, the reference device's size in
	// pixels at 72 and in millimeters at 80.
	var header [108]byte
	if n, _, _ := procGetEnhMetaFileHeader.Call(h, uintptr(len(header)), uintptr(unsafe.Pointer(&header[0]))); n == 0 {
		return nil, errors.New("the metafile has no header")
	}
	i32 := func(at int) float64 { return float64(int32(binary.LittleEndian.Uint32(header[at:]))) }
	if w <= 0 || hgt <= 0 {
		// The frame, in pixels of the device it was recorded for.
		devW, devH, mmW, mmH := i32(72), i32(76), i32(80), i32(84)
		if devW > 0 && mmW > 0 && devH > 0 && mmH > 0 {
			w = (i32(32) - i32(24)) / 100 * devW / mmW
			hgt = (i32(36) - i32(28)) / 100 * devH / mmH
		}
		if w < 1 || hgt < 1 {
			w, hgt = i32(16)-i32(8)+1, i32(20)-i32(12)+1
		}
	}
	if w < 1 || hgt < 1 || w > 1<<20 || hgt > 1<<20 {
		return nil, errors.New("the metafile has no size")
	}
	pw, ph, partial := vectorSize(w, hgt, opt)
	dc, _, _ := procCreateCompatibleDC.Call(0)
	if dc == 0 {
		return nil, errors.New("no device to draw the metafile on")
	}
	defer procDeleteDC.Call(dc)
	info := bitmapInfo{width: int32(pw), height: -int32(ph), planes: 1, bits: 32}
	info.size = uint32(unsafe.Sizeof(info))
	var bits unsafe.Pointer
	bm, _, _ := procCreateDIBSection.Call(dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bm == 0 || bits == nil {
		return nil, errors.New("no memory to draw the metafile in")
	}
	defer procDeleteObject.Call(bm)
	old, _, _ := procSelectObject.Call(dc, bm)
	defer procSelectObject.Call(dc, old)
	rect := [4]int32{0, 0, int32(pw), int32(ph)}
	const whiteBrush = 0
	white, _, _ := procGetStockObject.Call(whiteBrush)
	procFillRect.Call(dc, uintptr(unsafe.Pointer(&rect)), white)
	const halftone = 4
	procSetStretchBltMode.Call(dc, halftone)
	procPlayEnhMetaFile.Call(dc, h, uintptr(unsafe.Pointer(&rect)))
	procGdiFlush.Call()
	src := unsafe.Slice((*byte)(bits), 4*pw*ph)
	pix := make([]byte, len(src))
	for i := 0; i < len(src); i += 4 {
		pix[i], pix[i+1], pix[i+2], pix[i+3] = src[i+2], src[i+1], src[i], 255
	}
	p := &Picture{W: pw, H: ph, Pix: pix, Format: format, Opaque: true, Vector: true, Partial: partial}
	p.FullW, p.FullH = max(int(w+0.5), 1), max(int(hgt+0.5), 1)
	return p, nil
}

// bitmapInfo is the system's description of a bitmap's pixels.
type bitmapInfo struct {
	size          uint32
	width, height int32
	planes, bits  uint16
	compression   uint32
	sizeImage     uint32
	xPPM, yPPM    int32
	used, needed  uint32
	masks         [3]uint32
}

var iidShellItemImageFactory = guid("bcc18b79-ba16-442f-80c4-8a59c30c463b")

// decodeShell asks the system for its picture of a file, as its file
// manager shows one: for the files of programs that installed a handler
// for their type, and nothing else knows.
func decodeShell(path string, opt Options) (p *Picture, err error) {
	err = ErrUnknown
	onCOM(func() {
		var factory *object
		hr, _, _ := procSHCreateItemFromParse.Call(uintptr(unsafe.Pointer(utf16(path))), 0,
			uintptr(unsafe.Pointer(&iidShellItemImageFactory)), uintptr(unsafe.Pointer(&factory)))
		if hresult(hr).failed() || factory == nil {
			return
		}
		defer factory.release()
		side := 1024
		if opt.MaxW > 0 && opt.MaxH > 0 {
			side = min(max(opt.MaxW, opt.MaxH), 2048)
		}
		// The size goes by value: two 32-bit numbers in one argument.
		size := uintptr(uint32(side)) | uintptr(uint32(side))<<32
		// The picture of the content only: never the type's icon.
		const thumbnailOnly = 0x8
		const getImage = 3
		var bm uintptr
		if factory.call(getImage, size, thumbnailOnly, uintptr(unsafe.Pointer(&bm))).failed() || bm == 0 {
			return
		}
		defer procDeleteObject.Call(bm)
		// BITMAP: its type, then its width and height.
		var desc [32]byte
		if n, _, _ := procGetObject.Call(bm, uintptr(len(desc)), uintptr(unsafe.Pointer(&desc[0]))); n == 0 {
			return
		}
		w, h := int(int32(binary.LittleEndian.Uint32(desc[4:]))), int(int32(binary.LittleEndian.Uint32(desc[8:])))
		if w <= 0 || h <= 0 || w > 1<<14 || h > 1<<14 {
			return
		}
		dc, _, _ := procCreateCompatibleDC.Call(0)
		if dc == 0 {
			return
		}
		defer procDeleteDC.Call(dc)
		info := bitmapInfo{width: int32(w), height: -int32(h), planes: 1, bits: 32}
		info.size = 40
		buf := make([]byte, 4*w*h)
		if n, _, _ := procGetDIBits.Call(dc, bm, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&info)), 0); n == 0 {
			return
		}
		// Blue, green, red, alpha; pictures without transparency come
		// with an alpha of 0 throughout.
		blank := true
		for i := 3; i < len(buf); i += 4 {
			if buf[i] != 0 {
				blank = false
				break
			}
		}
		for i := 0; i < len(buf); i += 4 {
			buf[i], buf[i+2] = buf[i+2], buf[i]
			if blank {
				buf[i+3] = 255
			}
		}
		p = &Picture{W: w, H: h, FullW: w, FullH: h, Pix: buf, Orient: 1, Frames: 1, Via: "shell", Opaque: blank || opaque(buf)}
		p.Preview = true
		err = nil
	})
	return p, err
}
