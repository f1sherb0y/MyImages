package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image/png"
	"os/exec"
	"sync"

	"golang.org/x/image/bmp"
)

// A Windows bitmap without the header of a file, as the clipboard and
// some programs write it: given one, it is a BMP.
func init() {
	register(func(data []byte, opt Options) (*Picture, error) {
		if hasPrefix("BM")(data) {
			return std("BMP", bmp.Decode)(data, opt)
		}
		if len(data) < 40 {
			return nil, ErrUnknown
		}
		size := binary.LittleEndian.Uint32(data)
		bits := binary.LittleEndian.Uint16(data[14:])
		compression := binary.LittleEndian.Uint32(data[16:])
		colors := binary.LittleEndian.Uint32(data[32:])
		if size < 40 || size > 124 || int(size) > len(data) {
			return nil, ErrUnknown
		}
		if colors == 0 && bits <= 8 {
			colors = 1 << bits
		}
		offset := 14 + size + 4*colors
		if size == 40 && compression == 3 {
			offset += 12 // the masks of the colors
		}
		head := make([]byte, 14, 14+len(data))
		copy(head, "BM")
		binary.LittleEndian.PutUint32(head[2:], uint32(14+len(data)))
		binary.LittleEndian.PutUint32(head[10:], offset)
		file := append(head, data...)
		if p, err := decodeSystemBytes(file, opt); err == nil {
			return p, nil
		}
		return std("BMP", bmp.Decode)(file, opt)
	}, nil, "dib")
}

var (
	magickOnce sync.Once
	magickPath string
)

// decodeExternal has ImageMagick decode a file, where the user installed
// it: the last resort, for the rare formats nothing here knows. It knows
// some two hundred.
func decodeExternal(path string, opt Options) (*Picture, error) {
	magickOnce.Do(func() { magickPath, _ = exec.LookPath("magick") })
	if magickPath == "" {
		return nil, ErrUnknown
	}
	args := []string{fmt.Sprintf("%s[%d]", path, max(opt.Frame, 0)), "-auto-orient"}
	if opt.MaxW > 0 && opt.MaxH > 0 {
		args = append(args, "-resize", fmt.Sprintf("%dx%d>", opt.MaxW, opt.MaxH))
	}
	// A PNG barely compressed: quick to write and to read.
	args = append(args, "-define", "png:compression-level=1", "png:-")
	cmd := exec.Command(magickPath, args...)
	hideWindow(cmd)
	var out, errs bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errs
	if err := cmd.Run(); err != nil || out.Len() == 0 {
		if msg := bytes.TrimSpace(errs.Bytes()); len(msg) > 0 {
			if i := bytes.IndexByte(msg, '\n'); i > 0 {
				msg = msg[:i]
			}
			return nil, errors.New(string(msg))
		}
		return nil, ErrUnknown
	}
	p, err := finish(std("", png.Decode)(out.Bytes(), Options{}))
	if err != nil {
		return nil, err
	}
	p.Via = "magick"
	if opt.MaxW > 0 && opt.MaxH > 0 && (p.W >= opt.MaxW || p.H >= opt.MaxH) {
		// Perhaps reduced: all of it is asked for without a box.
		p.Partial = true
	}
	return p, nil
}
