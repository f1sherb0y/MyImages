package codec

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image/jpeg"
)

// decodeEmbedded shows the picture a camera put in its raw file beside
// the sensor's data: a JPEG, often of the full size, which every raw
// format has somewhere. It finds the largest one in the file, whatever
// the format, by looking for where JPEGs start.
func decodeEmbedded(path string, opt Options) (*Picture, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	at, size := -1, 0
	tried := 0
	for i := 0; i+4 < len(data) && tried < 64; {
		j := bytes.Index(data[i:], []byte{0xFF, 0xD8, 0xFF})
		if j < 0 {
			break
		}
		i += j
		// What follows a JPEG's start is one of its markers.
		if m := data[i+3]; m == 0xE0 || m == 0xE1 || m == 0xDB || m == 0xC4 || m == 0xEE || m == 0xE2 {
			tried++
			if cfg, err := jpeg.DecodeConfig(bytes.NewReader(data[i:])); err == nil && cfg.Width*cfg.Height > size {
				at, size = i, cfg.Width*cfg.Height
			}
		}
		i += 3
	}
	if at < 0 {
		return nil, errors.New("the raw file holds no preview")
	}
	p, err := decodeSystemBytes(data[at:], opt)
	if err != nil {
		p, err = finish(std("JPEG", jpeg.Decode)(data[at:], opt))
	}
	if err != nil {
		return nil, err
	}
	if p.Orient <= 1 {
		// The preview says nothing of how the camera was held: the raw
		// file around it does, where it is laid out as a TIFF.
		p.Orient = tiffOrientation(data)
	}
	p.Format = "RAW"
	p.Preview = true
	return p, nil
}

// tiffOrientation reads the orientation a TIFF's first directory gives,
// 1 where it gives none or the bytes are no TIFF.
func tiffOrientation(data []byte) int {
	if len(data) < 16 {
		return 1
	}
	var order binary.ByteOrder
	switch string(data[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	dir := int(order.Uint32(data[4:]))
	if dir < 8 || dir+2 > len(data) {
		return 1
	}
	n := int(order.Uint16(data[dir:]))
	for i := 0; i < n; i++ {
		e := dir + 2 + 12*i
		if e+12 > len(data) {
			break
		}
		if order.Uint16(data[e:]) == 274 {
			if o := int(order.Uint16(data[e+8:])); o >= 1 && o <= 8 {
				return o
			}
		}
	}
	return 1
}
