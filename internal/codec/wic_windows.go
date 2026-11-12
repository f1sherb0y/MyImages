package codec

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The Windows Imaging Component: its classes, interfaces and formats.
var (
	clsidWICFactory = guid("cacaf262-9370-4615-a13b-9f5539da4c0a")
	iidWICFactory   = guid("ec5ec8a9-c395-4314-9c77-54d7a935ff70")

	wicPRGBA = guid("3cc4a650-a527-4d37-a916-3142c7ebedba") // 32bppPRGBA
	wicBGR24 = guid("6fddc324-4e03-4bfe-b185-3d77768dc90c") // 24bppBGR
	wicGray8 = guid("6fddc324-4e03-4bfe-b185-3d77768dc908") // 8bppGray

	iidWICSourceTransform = guid("3b16811b-6a43-4ec9-b713-3d5a0c13b940")

	wicContainers = map[windows.GUID]string{
		guid("0af1d87e-fcfe-4188-bdeb-a7906471cbe3"): "BMP",
		guid("1b7cfaf4-713f-473c-bbcd-6137425faeaf"): "PNG",
		guid("a3a860c4-338f-4c17-919a-fba4b5628f21"): "ICO",
		guid("19e4a5aa-5662-4fc5-a0c0-1758028e1057"): "JPEG",
		guid("163bcc30-e2e9-4f0b-961d-a3e9fdb788a3"): "TIFF",
		guid("1f8a5601-7d4d-4cbd-9c82-1bc8d4eeb9a5"): "GIF",
		guid("57a37caa-367a-4540-916b-f183c5093a4b"): "JPEG XR",
		guid("9967cb95-2e85-4ac8-8ca2-83d7ccd425c9"): "DDS",
		guid("e1e62521-6787-405b-a339-500715b5763f"): "HEIF",
		guid("e094b0e2-67f2-45b3-b0ea-115337ca7cf3"): "WebP",
		guid("fe99ce60-f19c-433c-a3ae-00acefa9ca21"): "RAW",
		guid("f3ff6d0d-38c0-41c4-b1fe-1f3824f17b84"): "DNG",
	}
	wicJPEG = guid("19e4a5aa-5662-4fc5-a0c0-1758028e1057")
)

// Method indexes, after IUnknown's three.
const (
	// IWICImagingFactory
	wicCreateDecoderFromFilename = 3
	wicCreateDecoderFromStream   = 4
	wicCreateFormatConverter     = 10
	wicCreateBitmapScaler        = 11
	wicCreateStream              = 14
	// IWICBitmapDecoder
	wicGetContainerFormat = 5
	wicGetFrameCount      = 12
	wicGetFrame           = 13
	// IWICBitmapSource
	wicGetSize    = 3
	wicCopyPixels = 7
	// IWICBitmapFrameDecode
	wicGetMetadataQueryReader = 8
	// IWICBitmapScaler, IWICFormatConverter
	wicInitialize = 8
	// IWICMetadataQueryReader
	wicGetMetadataByName = 5
	// IWICBitmapSourceTransform
	wicTransformCopyPixels   = 3
	wicGetClosestSize        = 4
	wicGetClosestPixelFormat = 5
	// IWICStream
	wicInitializeFromMemory = 16
)

var (
	wicOnce    sync.Once
	wicFactory *object
	wicErr     error
)

// factory returns the imaging factory, made once. Call it on a COM
// thread.
func factory() (*object, error) {
	wicOnce.Do(func() {
		const inprocServer = 1
		hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidWICFactory)), 0, inprocServer,
			uintptr(unsafe.Pointer(&iidWICFactory)), uintptr(unsafe.Pointer(&wicFactory)))
		wicErr = hresult(hr).err()
	})
	return wicFactory, wicErr
}

// decodeSystem decodes a file with the system's codecs.
func decodeSystem(path string, opt Options) (p *Picture, err error) {
	onCOM(func() {
		var f *object
		if f, err = factory(); err != nil {
			return
		}
		var dec *object
		const genericRead = 0x80000000
		const cacheOnDemand = 0
		hr := f.call(wicCreateDecoderFromFilename, uintptr(unsafe.Pointer(utf16(path))), 0, genericRead, cacheOnDemand,
			uintptr(unsafe.Pointer(&dec)))
		if err = hr.err(); err != nil {
			return
		}
		defer dec.release()
		p, err = wicDecode(f, dec, opt)
		if p != nil && p.Format == "" {
			p.Format = strings.ToUpper(strings.TrimPrefix(filepath.Ext(path), "."))
		}
	})
	return p, err
}

// decodeSystemBytes decodes an image in memory with the system's
// codecs.
func decodeSystemBytes(data []byte, opt Options) (p *Picture, err error) {
	if len(data) == 0 {
		return nil, ErrUnknown
	}
	onCOM(func() {
		var f *object
		if f, err = factory(); err != nil {
			return
		}
		var stream *object
		if err = f.call(wicCreateStream, uintptr(unsafe.Pointer(&stream))).err(); err != nil {
			return
		}
		defer stream.release()
		if err = stream.call(wicInitializeFromMemory, uintptr(unsafe.Pointer(&data[0])), uintptr(uint32(len(data)))).err(); err != nil {
			return
		}
		var dec *object
		if err = f.call(wicCreateDecoderFromStream, uintptr(unsafe.Pointer(stream)), 0, 0, uintptr(unsafe.Pointer(&dec))).err(); err != nil {
			return
		}
		defer dec.release()
		p, err = wicDecode(f, dec, opt)
	})
	// The stream reads the bytes where they are, until here.
	runtime.KeepAlive(data)
	return p, err
}

func wicDecode(f, dec *object, opt Options) (*Picture, error) {
	var container windows.GUID
	dec.call(wicGetContainerFormat, uintptr(unsafe.Pointer(&container)))
	var frames uint32
	if err := dec.call(wicGetFrameCount, uintptr(unsafe.Pointer(&frames))).err(); err != nil {
		return nil, err
	}
	if frames == 0 {
		return nil, errors.New("the file holds no image")
	}
	index := min(max(opt.Frame, 0), int(frames)-1)
	var frame *object
	if err := dec.call(wicGetFrame, uintptr(index), uintptr(unsafe.Pointer(&frame))).err(); err != nil {
		return nil, err
	}
	defer frame.release()
	var w, h uint32
	if err := frame.call(wicGetSize, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h))).err(); err != nil {
		return nil, err
	}
	if w == 0 || h == 0 {
		return nil, errors.New("the image is empty")
	}
	p := &Picture{FullW: int(w), FullH: int(h), Frames: int(frames), Frame: index, Format: wicContainers[container], Via: "wic"}
	p.Orient = wicOrientation(frame)
	if frames > 1 && p.Format == "GIF" {
		p.Animated = true
		p.Delay = wicDelay(frame)
	}
	maxW, maxH := opt.MaxW, opt.MaxH
	if p.Orient >= 5 {
		// The box is for the picture upright: this one lies on its side.
		maxW, maxH = maxH, maxW
	}
	p.W, p.H = fit(p.FullW, p.FullH, maxW, maxH)
	if p.W != p.FullW || p.H != p.FullH {
		if wicReduced(frame, p) {
			return p, nil
		}
	}

	src := frame
	if p.W != p.FullW || p.H != p.FullH {
		// The scaler asks the decoder for a reduced picture where it
		// can make one, as JPEG's does by eighths, and scales the rest.
		var scaler *object
		if err := f.call(wicCreateBitmapScaler, uintptr(unsafe.Pointer(&scaler))).err(); err != nil {
			return nil, err
		}
		defer scaler.release()
		const fant = 4
		if err := scaler.call(wicInitialize, uintptr(unsafe.Pointer(src)), uintptr(p.W), uintptr(p.H), fant).err(); err != nil {
			return nil, err
		}
		src = scaler
	}
	var conv *object
	if err := f.call(wicCreateFormatConverter, uintptr(unsafe.Pointer(&conv))).err(); err != nil {
		return nil, err
	}
	defer conv.release()
	if err := conv.call(wicInitialize, uintptr(unsafe.Pointer(src)), uintptr(unsafe.Pointer(&wicPRGBA)), 0, 0, 0, 0).err(); err != nil {
		return nil, err
	}
	stride := 4 * p.W
	if int64(stride)*int64(p.H) > maxPixels*4 {
		return nil, errors.New("the image is too large")
	}
	p.Pix = make([]byte, stride*p.H)
	// A call copies at most 2 GB: larger pictures go in bands.
	band := max((1<<30)/stride, 1)
	for y := 0; y < p.H; y += band {
		rows := min(band, p.H-y)
		rect := [4]int32{0, int32(y), int32(p.W), int32(rows)}
		buf := p.Pix[y*stride : (y+rows)*stride]
		if err := conv.call(wicCopyPixels, uintptr(unsafe.Pointer(&rect)), uintptr(stride), uintptr(len(buf)),
			uintptr(unsafe.Pointer(&buf[0]))).err(); err != nil {
			return nil, err
		}
	}
	p.Opaque = container == wicJPEG || opaque(p.Pix)
	return p, nil
}

// wicOrientation reads the EXIF orientation of a frame, 1 where it has
// none.
func wicOrientation(frame *object) int {
	var reader *object
	if frame.call(wicGetMetadataQueryReader, uintptr(unsafe.Pointer(&reader))).failed() || reader == nil {
		return 1
	}
	defer reader.release()
	for _, name := range []string{"/app1/ifd/{ushort=274}", "/ifd/{ushort=274}"} {
		// A PROPVARIANT: its type, then its value 8 bytes in.
		var v [3]uint64
		if reader.call(wicGetMetadataByName, uintptr(unsafe.Pointer(utf16(name))), uintptr(unsafe.Pointer(&v))).failed() {
			continue
		}
		const ui2 = 18
		if uint16(v[0]) == ui2 {
			if o := int(uint16(v[1])); o >= 1 && o <= 8 {
				return o
			}
		}
	}
	return 1
}

// wicReduced has the decoder itself decode a frame smaller, where it
// can: JPEG's decodes at a half, a quarter or an eighth for a fraction of
// the work. The picture comes out at the decoder's size next above the
// one asked for, which costs less to show than to scale down here. It
// reports whether it did.
func wicReduced(frame *object, p *Picture) bool {
	tr := frame.query(&iidWICSourceTransform)
	if tr == nil {
		return false
	}
	defer tr.release()
	w, h := uint32(p.W), uint32(p.H)
	if tr.call(wicGetClosestSize, uintptr(unsafe.Pointer(&w)), uintptr(unsafe.Pointer(&h))).failed() {
		return false
	}
	if w == 0 || h == 0 || int(w) >= p.FullW || int(w) < p.W || int(h) < p.H {
		return false
	}
	format := wicPRGBA
	if tr.call(wicGetClosestPixelFormat, uintptr(unsafe.Pointer(&format))).failed() {
		return false
	}
	var bpp int
	switch format {
	case wicPRGBA:
		bpp = 4
	case wicBGR24:
		bpp = 3
	case wicGray8:
		bpp = 1
	default:
		return false
	}
	// Rows padded to 4 bytes, as the codecs like them.
	stride := (int(w)*bpp + 3) &^ 3
	buf := make([]byte, stride*int(h))
	if tr.call(wicTransformCopyPixels, 0, uintptr(w), uintptr(h), uintptr(unsafe.Pointer(&format)), 0,
		uintptr(stride), uintptr(len(buf)), uintptr(unsafe.Pointer(&buf[0]))).failed() {
		return false
	}
	p.W, p.H = int(w), int(h)
	if bpp == 4 {
		p.Pix = buf
		p.Opaque = opaque(buf)
		return true
	}
	p.Pix = make([]byte, 4*p.W*p.H)
	parallel(p.H, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			s := buf[y*stride:]
			d := p.Pix[y*4*p.W : (y+1)*4*p.W]
			if bpp == 3 {
				for x, i := 0, 0; x < len(d); x, i = x+4, i+3 {
					d[x], d[x+1], d[x+2], d[x+3] = s[i+2], s[i+1], s[i], 255
				}
			} else {
				for x, i := 0, 0; x < len(d); x, i = x+4, i+1 {
					d[x], d[x+1], d[x+2], d[x+3] = s[i], s[i], s[i], 255
				}
			}
		}
	})
	p.Opaque = true
	return true
}

// wicDelay reads how long a frame of an animation shows, 0 where the
// file does not say.
func wicDelay(frame *object) time.Duration {
	var reader *object
	if frame.call(wicGetMetadataQueryReader, uintptr(unsafe.Pointer(&reader))).failed() || reader == nil {
		return 0
	}
	defer reader.release()
	var v propVariant
	// A GIF's, in hundredths of a second; a WebP's, in milliseconds.
	if query(reader, "/grctlext/Delay", &v) {
		n, ok := v.number()
		v.clear()
		if ok {
			return time.Duration(n) * 10 * time.Millisecond
		}
	}
	if query(reader, "/ANMF/FrameDuration", &v) {
		n, ok := v.number()
		v.clear()
		if ok {
			return time.Duration(n) * time.Millisecond
		}
	}
	return 0
}
