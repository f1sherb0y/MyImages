package codec

import (
	"encoding/binary"

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
