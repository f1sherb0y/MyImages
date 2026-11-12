package codec

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// Old, simple formats for which there is no library to use: decoded
// here, each in a few dozen lines.
func init() {
	register(decodeXBM, func(h []byte) bool {
		return bytes.HasPrefix(h, []byte("#define")) && bytes.Contains(h, []byte("_width"))
	}, "xbm")
	register(decodeXPM, func(h []byte) bool {
		return bytes.HasPrefix(h, []byte("/* XPM */")) || bytes.HasPrefix(h, []byte("! XPM2"))
	}, "xpm")
	register(decodeVIFF, func(h []byte) bool { return len(h) > 2 && h[0] == 0xAB && h[1] == 0x01 }, "viff", "xv")
	register(decodeWPG, hasPrefix("\xffWPC"), "wpg")
	register(decodeCUT, nil, "cut")
}

var errBroken = errors.New("the file is damaged")

// sized checks a picture's size before memory is made for it.
func sized(w, h int) error {
	if w <= 0 || h <= 0 || w > 1<<20 || h > 1<<20 || w*h > maxPixels {
		return errBroken
	}
	return nil
}

var (
	xbmDefine = regexp.MustCompile(`#define\s+\S*_(width|height)\s+(\d+)`)
	xbmByte   = regexp.MustCompile(`0[xX]([0-9a-fA-F]{1,4})`)
)

// decodeXBM decodes an X bitmap: C source of an array of bits, the
// lowest of each byte first, set for black.
func decodeXBM(data []byte, opt Options) (*Picture, error) {
	var w, h int
	for _, m := range xbmDefine.FindAllSubmatch(data, -1) {
		n, _ := strconv.Atoi(string(m[2]))
		if string(m[1]) == "width" {
			w = n
		} else {
			h = n
		}
	}
	if err := sized(w, h); err != nil {
		return nil, err
	}
	at := bytes.IndexByte(data, '{')
	if at < 0 {
		return nil, errBroken
	}
	// The X10 kind has 16 bits a number; X11's has 8.
	short := bytes.Contains(data[:at], []byte("short"))
	var bits []byte
	for _, m := range xbmByte.FindAllSubmatch(data[at:], -1) {
		v, _ := strconv.ParseUint(string(m[1]), 16, 16)
		bits = append(bits, byte(v))
		if short {
			bits = append(bits, byte(v>>8))
		}
	}
	stride := (w + 7) / 8
	if short {
		stride = (w + 15) / 16 * 2
	}
	if len(bits) < stride*h {
		return nil, errBroken
	}
	img := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if bits[y*stride+x/8]>>(x%8)&1 == 0 {
				img.Pix[y*w+x] = 255
			}
		}
	}
	return fromImage(img, "XBM", opt), nil
}

// xpmColors are the colors X names that pictures use most.
var xpmColors = map[string]color.NRGBA{
	"black": {0, 0, 0, 255}, "white": {255, 255, 255, 255}, "red": {255, 0, 0, 255}, "green": {0, 255, 0, 255},
	"blue": {0, 0, 255, 255}, "yellow": {255, 255, 0, 255}, "cyan": {0, 255, 255, 255}, "magenta": {255, 0, 255, 255},
	"gray": {190, 190, 190, 255}, "grey": {190, 190, 190, 255}, "orange": {255, 165, 0, 255}, "brown": {165, 42, 42, 255},
	"pink": {255, 192, 203, 255}, "purple": {160, 32, 240, 255}, "navy": {0, 0, 128, 255}, "gold": {255, 215, 0, 255},
	"darkgray": {169, 169, 169, 255}, "darkgrey": {169, 169, 169, 255}, "lightgray": {211, 211, 211, 255},
	"lightgrey": {211, 211, 211, 255}, "dimgray": {105, 105, 105, 255}, "maroon": {176, 48, 96, 255},
	"darkblue": {0, 0, 139, 255}, "darkgreen": {0, 100, 0, 255}, "darkred": {139, 0, 0, 255}, "silver": {192, 192, 192, 255},
}

// xpmColor reads a color as XPM writes one.
func xpmColor(s string) (color.NRGBA, bool) {
	s = strings.TrimSpace(s)
	if strings.EqualFold(s, "none") {
		return color.NRGBA{}, true
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		n := len(hex) / 3
		if n < 1 || n > 4 || len(hex) != 3*n {
			return color.NRGBA{}, false
		}
		var c [3]uint8
		for i := range c {
			v, err := strconv.ParseUint(hex[i*n:(i+1)*n], 16, 16)
			if err != nil {
				return color.NRGBA{}, false
			}
			// To 8 bits, from however many the file has.
			switch n {
			case 1:
				v *= 17
			case 3:
				v >>= 4
			case 4:
				v >>= 8
			}
			c[i] = uint8(v)
		}
		return color.NRGBA{c[0], c[1], c[2], 255}, true
	}
	name := strings.ToLower(strings.ReplaceAll(s, " ", ""))
	if c, ok := xpmColors[name]; ok {
		return c, true
	}
	// gray0 to gray100.
	for _, prefix := range []string{"gray", "grey"} {
		if rest, ok := strings.CutPrefix(name, prefix); ok {
			if n, err := strconv.Atoi(rest); err == nil && n >= 0 && n <= 100 {
				v := uint8(float64(n)*2.55 + 0.5)
				return color.NRGBA{v, v, v, 255}, true
			}
		}
	}
	return color.NRGBA{}, false
}

// decodeXPM decodes an X pixmap: C source of strings, the first telling
// the size and how many colors, then a string a color, then one a row.
func decodeXPM(data []byte, opt Options) (*Picture, error) {
	var lines []string
	if bytes.HasPrefix(data, []byte("! XPM2")) {
		sc := bufio.NewScanner(bytes.NewReader(data))
		sc.Buffer(nil, 1<<26)
		sc.Scan()
		for sc.Scan() {
			lines = append(lines, sc.Text())
		}
	} else {
		// The strings, outside the comments.
		for i := 0; i < len(data); i++ {
			switch {
			case data[i] == '/' && i+1 < len(data) && data[i+1] == '*':
				end := bytes.Index(data[i+2:], []byte("*/"))
				if end < 0 {
					i = len(data)
				} else {
					i += end + 3
				}
			case data[i] == '"':
				end := bytes.IndexByte(data[i+1:], '"')
				if end < 0 {
					return nil, errBroken
				}
				lines = append(lines, string(data[i+1:i+1+end]))
				i += end + 1
			}
		}
	}
	if len(lines) == 0 {
		return nil, errBroken
	}
	head := strings.Fields(lines[0])
	if len(head) < 4 {
		return nil, errBroken
	}
	w, _ := strconv.Atoi(head[0])
	h, _ := strconv.Atoi(head[1])
	ncolors, _ := strconv.Atoi(head[2])
	cpp, _ := strconv.Atoi(head[3])
	if err := sized(w, h); err != nil {
		return nil, err
	}
	if ncolors <= 0 || ncolors > 1<<24 || cpp <= 0 || cpp > 8 || len(lines) < 1+ncolors+h {
		return nil, errBroken
	}
	colors := make(map[string]color.NRGBA, ncolors)
	for _, line := range lines[1 : 1+ncolors] {
		if len(line) < cpp {
			return nil, errBroken
		}
		key, rest := line[:cpp], strings.Fields(line[cpp:])
		// Pairs of a kind and a color: "c" is the one for color screens;
		// a color's name may be several words.
		var best color.NRGBA
		found := false
		for i := 0; i < len(rest); i++ {
			kind := rest[i]
			if kind != "c" && kind != "g" && kind != "g4" && kind != "m" && kind != "s" {
				continue
			}
			j := i + 1
			for j < len(rest) && rest[j] != "c" && rest[j] != "g" && rest[j] != "g4" && rest[j] != "m" && rest[j] != "s" {
				j++
			}
			if kind != "s" && (kind == "c" || !found) {
				if c, ok := xpmColor(strings.Join(rest[i+1:j], " ")); ok {
					best, found = c, true
				}
			}
			i = j - 1
		}
		colors[key] = best
	}
	pix := make([]byte, 4*w*h)
	for y := 0; y < h; y++ {
		row := lines[1+ncolors+y]
		if len(row) < w*cpp {
			return nil, errBroken
		}
		for x := 0; x < w; x++ {
			c := colors[row[x*cpp:(x+1)*cpp]]
			i := (y*w + x) * 4
			pix[i], pix[i+1], pix[i+2], pix[i+3] = c.R, c.G, c.B, c.A
		}
	}
	return fromNRGBA(pix, w, h, "XPM", opt), nil
}

// decodeVIFF decodes a Khoros picture: a header of 1024 bytes, maps of
// colors if any, then the bands one after the other.
func decodeVIFF(data []byte, opt Options) (*Picture, error) {
	if len(data) < 1024 {
		return nil, errBroken
	}
	var order binary.ByteOrder = binary.BigEndian
	if data[4] == 0x08 || data[4] == 0x04 {
		order = binary.LittleEndian
	}
	u := func(at int) int { return int(order.Uint32(data[at:])) }
	w, h := u(520), u(524)
	bands, storage := u(560), u(564)
	mapScheme, mapStorage, mapRows, mapCols := u(572), u(576), u(580), u(584)
	if err := sized(w, h); err != nil {
		return nil, err
	}
	if bands < 1 || bands > 4 {
		return nil, errBroken
	}
	sizes := map[int]int{1: 1, 2: 2, 4: 4, 5: 4, 9: 8}
	body := data[1024:]
	var palette []byte
	if mapScheme != 0 {
		n := mapRows * mapCols * sizes[mapStorage]
		if sizes[mapStorage] == 0 || n <= 0 || n > len(body) {
			return nil, errBroken
		}
		if mapStorage == 1 {
			palette = body[:n]
		}
		body = body[n:]
	}
	// A sample of a band, from 0 to 1.
	var sample func(band, i int) float64
	switch storage {
	case 0: // bits, the lowest of a byte first
		stride := (w + 7) / 8
		if len(body) < stride*h*bands {
			return nil, errBroken
		}
		sample = func(band, i int) float64 {
			x, y := i%w, i/w
			return float64(body[band*stride*h+y*stride+x/8] >> (x % 8) & 1)
		}
	case 1:
		if len(body) < w*h*bands {
			return nil, errBroken
		}
		sample = func(band, i int) float64 { return float64(body[band*w*h+i]) / 255 }
	case 2:
		if len(body) < 2*w*h*bands {
			return nil, errBroken
		}
		sample = func(band, i int) float64 { return float64(order.Uint16(body[2*(band*w*h+i):])) / 65535 }
	case 4:
		if len(body) < 4*w*h*bands {
			return nil, errBroken
		}
		sample = func(band, i int) float64 { return float64(order.Uint32(body[4*(band*w*h+i):])) / 4294967295 }
	case 5:
		if len(body) < 4*w*h*bands {
			return nil, errBroken
		}
		sample = func(band, i int) float64 {
			v := float64(math.Float32frombits(order.Uint32(body[4*(band*w*h+i):])))
			return min(max(v, 0), 1)
		}
	default:
		return nil, errors.New("a kind of VIFF this program does not read")
	}
	pix := make([]byte, 4*w*h)
	to8 := func(v float64) byte { return byte(v*255 + 0.5) }
	for i := 0; i < w*h; i++ {
		o := pix[4*i : 4*i+4]
		o[3] = 255
		switch {
		case palette != nil && bands == 1 && storage == 1:
			// The map holds its reds, then its greens, then its blues.
			v := int(body[i])
			if mapRows >= 3 && v < mapCols {
				o[0], o[1], o[2] = palette[v], palette[mapCols+v], palette[2*mapCols+v]
			} else if v < mapCols {
				o[0], o[1], o[2] = palette[v], palette[v], palette[v]
			}
		case bands >= 3:
			o[0], o[1], o[2] = to8(sample(0, i)), to8(sample(1, i)), to8(sample(2, i))
			if bands == 4 {
				o[3] = to8(sample(3, i))
			}
		default:
			v := to8(sample(0, i))
			o[0], o[1], o[2] = v, v, v
			if bands == 2 {
				o[3] = to8(sample(1, i))
			}
		}
	}
	return fromNRGBA(pix, w, h, "VIFF", opt), nil
}

// decodeWPG decodes the bitmap of a WordPerfect graphic of the first
// version of the format: records, of which one holds the colors and one
// the pixels, in runs.
func decodeWPG(data []byte, opt Options) (*Picture, error) {
	if len(data) < 16 {
		return nil, errBroken
	}
	at := int(binary.LittleEndian.Uint32(data[4:]))
	if data[10] != 1 {
		return nil, errors.New("a version of WPG this program does not read")
	}
	palette := make([]color.NRGBA, 256)
	for i := range palette {
		v := uint8(i)
		palette[i] = color.NRGBA{v, v, v, 255}
	}
	palette[1] = color.NRGBA{255, 255, 255, 255}
	paletted := false
	for at+2 <= len(data) {
		kind := data[at]
		size := int(data[at+1])
		at += 2
		if size == 0xFF {
			if at+2 > len(data) {
				break
			}
			size = int(binary.LittleEndian.Uint16(data[at:]))
			at += 2
			if size&0x8000 != 0 {
				if at+2 > len(data) {
					break
				}
				size = (size&0x7FFF)<<16 | int(binary.LittleEndian.Uint16(data[at:]))
				at += 2
			}
		}
		if size < 0 || at+size > len(data) {
			break
		}
		rec := data[at : at+size]
		at += size
		switch kind {
		case 0x0E: // the colors
			if len(rec) < 4 {
				continue
			}
			start, count := int(binary.LittleEndian.Uint16(rec)), int(binary.LittleEndian.Uint16(rec[2:]))
			for i := 0; i < count && 4+3*i+2 < len(rec) && start+i < 256; i++ {
				palette[start+i] = color.NRGBA{rec[4+3*i], rec[5+3*i], rec[6+3*i], 255}
			}
			paletted = true
		case 0x0B, 0x14: // the pixels
			if kind == 0x14 {
				if len(rec) < 10 {
					return nil, errBroken
				}
				rec = rec[10:]
			}
			if len(rec) < 10 {
				return nil, errBroken
			}
			w, h := int(binary.LittleEndian.Uint16(rec)), int(binary.LittleEndian.Uint16(rec[2:]))
			depth := int(binary.LittleEndian.Uint16(rec[4:]))
			if err := sized(w, h); err != nil {
				return nil, err
			}
			if depth != 1 && depth != 2 && depth != 4 && depth != 8 {
				return nil, errBroken
			}
			stride := (w*depth + 7) / 8
			rows := wpgRuns(rec[10:], stride, h)
			pix := make([]byte, 4*w*h)
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					bit := x * depth
					v := int(rows[y*stride+bit/8]>>(8-depth-bit%8)) & (1<<depth - 1)
					c := palette[v]
					if depth == 1 && !paletted {
						c = palette[0]
						if v != 0 {
							c = palette[1]
						}
					}
					i := (y*w + x) * 4
					pix[i], pix[i+1], pix[i+2], pix[i+3] = c.R, c.G, c.B, 255
				}
			}
			return fromNRGBA(pix, w, h, "WPG", opt), nil
		}
	}
	return nil, errors.New("the WPG holds no bitmap")
}

// wpgRuns undoes the runs of a WPG bitmap into rows of stride bytes.
func wpgRuns(src []byte, stride, h int) []byte {
	out := make([]byte, 0, stride*h)
	total := stride * h
	for i := 0; i < len(src) && len(out) < total; {
		b := src[i]
		i++
		n := int(b & 0x7F)
		switch {
		case b&0x80 != 0 && n != 0: // a byte, n times
			if i >= len(src) {
				return append(out, make([]byte, total-len(out))...)
			}
			for ; n > 0 && len(out) < total; n-- {
				out = append(out, src[i])
			}
			i++
		case b&0x80 != 0: // 0xFF, as many times as the next byte says
			if i >= len(src) {
				break
			}
			for n = int(src[i]); n > 0 && len(out) < total; n-- {
				out = append(out, 0xFF)
			}
			i++
		case n != 0: // n bytes as they are
			for ; n > 0 && i < len(src) && len(out) < total; n-- {
				out = append(out, src[i])
				i++
			}
		default: // the row before, as many times as the next byte says
			if i >= len(src) {
				break
			}
			n = int(src[i])
			i++
			// To the end of the row under way first.
			for len(out)%stride != 0 && len(out) < total {
				out = append(out, 0)
			}
			for ; n > 0 && len(out) >= stride && len(out)+stride <= total; n-- {
				out = append(out, out[len(out)-stride:]...)
			}
		}
	}
	if len(out) < total {
		out = append(out, make([]byte, total-len(out))...)
	}
	return out
}

// decodeCUT decodes a Dr. Halo picture: rows of runs of bytes that index
// a palette kept in another file, shown here as grays.
func decodeCUT(data []byte, opt Options) (*Picture, error) {
	if len(data) < 6 {
		return nil, errBroken
	}
	w, h := int(binary.LittleEndian.Uint16(data)), int(binary.LittleEndian.Uint16(data[2:]))
	if err := sized(w, h); err != nil {
		return nil, err
	}
	img := image.NewGray(image.Rect(0, 0, w, h))
	at := 6
	for y := 0; y < h && at+2 <= len(data); y++ {
		// How many bytes the row takes, then its runs, ended by a zero.
		at += 2
		x := 0
		for at < len(data) {
			b := data[at]
			at++
			if b == 0 {
				break
			}
			n := int(b & 0x7F)
			if b&0x80 != 0 {
				if at >= len(data) {
					return nil, errBroken
				}
				for ; n > 0 && x < w; n-- {
					img.Pix[y*w+x] = data[at]
					x++
				}
				at++
			} else {
				for ; n > 0 && at < len(data); n-- {
					if x < w {
						img.Pix[y*w+x] = data[at]
						x++
					}
					at++
				}
			}
		}
	}
	return fromImage(stretch(img), "CUT", opt), nil
}

// decodePSDComposite decodes the picture a Photoshop file keeps of all
// its layers together, of any depth, without its transparency.
func decodePSDComposite(data []byte, opt Options) (*Picture, error) {
	if len(data) < 30 || string(data[:4]) != "8BPS" {
		return nil, ErrUnknown
	}
	be := binary.BigEndian
	big := be.Uint16(data[4:]) == 2
	channels := int(be.Uint16(data[12:]))
	h, w := int(be.Uint32(data[14:])), int(be.Uint32(data[18:]))
	depth, mode := int(be.Uint16(data[22:])), int(be.Uint16(data[24:]))
	if err := sized(w, h); err != nil {
		return nil, err
	}
	if depth != 8 && depth != 16 && depth != 32 {
		return nil, errors.New("a depth of Photoshop file this program does not read")
	}
	at := 26
	// The colors of an indexed picture, the resources, the layers: each
	// behind its length.
	var palette []byte
	for section := 0; section < 3; section++ {
		if at+4 > len(data) {
			return nil, errBroken
		}
		n := int(be.Uint32(data[at:]))
		at += 4
		if section == 2 && big {
			if at+4 > len(data) {
				return nil, errBroken
			}
			n = n<<32 | int(be.Uint32(data[at:]))
			at += 4
		}
		if n < 0 || at+n > len(data) {
			return nil, errBroken
		}
		if section == 0 {
			palette = data[at : at+n]
		}
		at += n
	}
	if at+2 > len(data) {
		return nil, errBroken
	}
	compression := be.Uint16(data[at:])
	at += 2
	want := map[int]int{1: 1, 2: 1, 3: 3, 4: 4, 8: 1, 9: 3}[mode]
	if want == 0 || channels < want {
		return nil, errors.New("a kind of Photoshop file this program does not read")
	}
	bps := depth / 8
	rowBytes := w * bps
	planes := make([][]byte, want)
	switch compression {
	case 0:
		for c := range planes {
			if at+rowBytes*h > len(data) {
				return nil, errBroken
			}
			planes[c] = data[at : at+rowBytes*h]
			at += rowBytes * h
		}
	case 1:
		// The lengths of every row of every channel come first.
		lenSize := 2
		if big {
			lenSize = 4
		}
		lengths := at
		at += channels * h * lenSize
		if at > len(data) {
			return nil, errBroken
		}
		for c := range planes {
			plane := make([]byte, 0, rowBytes*h)
			for y := 0; y < h; y++ {
				l := lengths + (c*h+y)*lenSize
				n := int(be.Uint16(data[l:]))
				if big {
					n = int(be.Uint32(data[l:]))
				}
				if n < 0 || at+n > len(data) {
					return nil, errBroken
				}
				plane = unpackBits(plane, data[at:at+n], rowBytes)
				at += n
			}
			planes[c] = plane
		}
	default:
		return nil, errors.New("a compression of Photoshop file this program does not read")
	}
	// A sample as 8 bits.
	get := func(c, i int) uint8 {
		p := planes[c][i*bps:]
		switch bps {
		case 2:
			return p[0]
		case 4:
			v := math.Float32frombits(be.Uint32(p))
			// Light as it is measured: with the screen's curve.
			return uint8(math.Pow(float64(min(max(v, 0), 1)), 1/2.2)*255 + 0.5)
		}
		return p[0]
	}
	pix := make([]byte, 4*w*h)
	for i := 0; i < w*h; i++ {
		o := pix[4*i : 4*i+4]
		o[3] = 255
		switch mode {
		case 3, 9:
			o[0], o[1], o[2] = get(0, i), get(1, i), get(2, i)
		case 4:
			// Stored inverted: 255 is no ink.
			c, m, y, k := uint32(get(0, i)), uint32(get(1, i)), uint32(get(2, i)), uint32(get(3, i))
			o[0], o[1], o[2] = uint8(c*k/255), uint8(m*k/255), uint8(y*k/255)
		case 2:
			v := int(get(0, i))
			if len(palette) >= 768 {
				o[0], o[1], o[2] = palette[v], palette[256+v], palette[512+v]
			}
		default:
			v := get(0, i)
			o[0], o[1], o[2] = v, v, v
		}
	}
	return fromRGBA(pix, w, h, "Photoshop", opt), nil
}

// unpackBits undoes PackBits runs into a row of n bytes, appended to
// dst.
func unpackBits(dst, src []byte, n int) []byte {
	end := len(dst) + n
	for i := 0; i < len(src) && len(dst) < end; {
		b := int(int8(src[i]))
		i++
		switch {
		case b >= 0:
			for b++; b > 0 && i < len(src) && len(dst) < end; b-- {
				dst = append(dst, src[i])
				i++
			}
		case b != -128:
			if i >= len(src) {
				break
			}
			for b = 1 - b; b > 0 && len(dst) < end; b-- {
				dst = append(dst, src[i])
			}
			i++
		}
	}
	for len(dst) < end {
		dst = append(dst, 0)
	}
	return dst
}

// decodeDDS decodes a DirectDraw surface that is not compressed: pixels
// of 8 to 32 bits, each color where its mask says. The compressed kinds
// are the system's codec's.
func decodeDDS(data []byte, opt Options) (*Picture, error) {
	if len(data) < 128 {
		return nil, errBroken
	}
	le := binary.LittleEndian
	h, w := int(le.Uint32(data[12:])), int(le.Uint32(data[16:]))
	flags, bits := le.Uint32(data[80:]), int(le.Uint32(data[88:]))
	const hasAlpha, fourCC, rgb, luminance = 0x1, 0x4, 0x40, 0x20000
	if flags&fourCC != 0 || flags&(rgb|luminance) == 0 || bits%8 != 0 || bits < 8 || bits > 32 {
		return nil, errors.New("a kind of DDS this program does not read")
	}
	if err := sized(w, h); err != nil {
		return nil, err
	}
	size := bits / 8
	body := data[128:]
	if len(body) < w*h*size {
		return nil, errBroken
	}
	masks := [4]uint32{le.Uint32(data[92:]), le.Uint32(data[96:]), le.Uint32(data[100:]), le.Uint32(data[104:])}
	if flags&hasAlpha == 0 {
		masks[3] = 0
	}
	if flags&luminance != 0 {
		masks[1], masks[2] = masks[0], masks[0]
	}
	// A channel of a pixel as 8 bits: shifted down from where its mask
	// is, and scaled from as many bits as the mask has.
	var shift, top [4]uint32
	for c, m := range masks {
		if m == 0 {
			continue
		}
		for m&1 == 0 {
			m >>= 1
			shift[c]++
		}
		top[c] = m
	}
	pix := make([]byte, 4*w*h)
	for i := 0; i < w*h; i++ {
		var v uint32
		for b := 0; b < size; b++ {
			v |= uint32(body[i*size+b]) << (8 * b)
		}
		for c := 0; c < 4; c++ {
			if top[c] == 0 {
				pix[4*i+c] = 255 * uint8(c/3)
				continue
			}
			pix[4*i+c] = uint8((v & masks[c] >> shift[c]) * 255 / top[c])
		}
	}
	return fromNRGBA(pix, w, h, "DDS", opt), nil
}
