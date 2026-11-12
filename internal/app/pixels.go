package app

import (
	"image"
	"runtime"
	"sync"

	"github.com/egoist/mygo/ui"
)

// level is a picture's pixels at one size, cut into tiles: the GPU takes
// a tile when it first shows, so that a large picture uploads only where
// it is looked at, and no texture is larger than a GPU allows.
type level struct {
	w, h  int
	tiles []tile
}

// tile is a part of a level. Its bitmap holds a row of pixels more on
// each side that has a neighbor, so that blending across the seam reads
// the neighbor's pixels: sx, sy, sw, sh are the tile within its bitmap.
type tile struct {
	x, y, w, h int
	bm         *ui.Bitmap
	src        ui.Rect
}

const (
	// tileBytes is about what a tile holds: what uploads in a frame
	// without being felt.
	tileBytes = 16 << 20
	// maxTexture is the widest a tile may be.
	maxTexture = 8192
)

// newLevel cuts pixels (premultiplied RGBA, rows of 4*w bytes) into
// tiles. Pictures no wider than a texture are cut into strips of whole
// rows, which share the pixels' memory; wider ones are copied into
// columns of strips. crisp shows the pixels as squares when enlarged.
func newLevel(pix []byte, w, h int, crisp bool) *level {
	l := &level{w: w, h: h}
	cols := (w + maxTexture - 1) / maxTexture
	colW := (w + cols - 1) / cols
	rows := max(tileBytes/(4*colW), 64)
	for x := 0; x < w; x += colW {
		tw := min(colW, w-x)
		// With a column more on each side that has a neighbor.
		x0, x1 := max(x-1, 0), min(x+tw+1, w)
		for y := 0; y < h; y += rows {
			th := min(rows, h-y)
			y0, y1 := max(y-1, 0), min(y+th+1, h)
			var img *image.RGBA
			if cols == 1 {
				img = &image.RGBA{Pix: pix[y0*4*w : y1*4*w], Stride: 4 * w, Rect: image.Rect(0, 0, w, y1-y0)}
			} else {
				img = image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
				for r := y0; r < y1; r++ {
					copy(img.Pix[(r-y0)*img.Stride:], pix[r*4*w+4*x0:r*4*w+4*x1])
				}
			}
			bm := ui.NewBitmap(img)
			if crisp {
				bm.Pixelated()
			}
			l.tiles = append(l.tiles, tile{x: x, y: y, w: tw, h: th, bm: bm,
				src: ui.Rect{X: float32(x - x0), Y: float32(y - y0), W: float32(tw), H: float32(th)}})
		}
	}
	return l
}

// halve returns pixels at half the size, each the average of the 2×2 it
// covers; an odd last row or column counts twice.
func halve(pix []byte, w, h int) ([]byte, int, int) {
	hw, hh := max((w+1)/2, 1), max((h+1)/2, 1)
	out := make([]byte, 4*hw*hh)
	parallel(hh, func(y0, y1 int) {
		for y := y0; y < y1; y++ {
			r0 := pix[(2*y)*4*w:]
			r1 := r0
			if 2*y+1 < h {
				r1 = pix[(2*y+1)*4*w:]
			}
			d := out[y*4*hw : (y+1)*4*hw]
			for x := 0; x < hw; x++ {
				a := 8 * x
				b := a + 4
				if 2*x+1 >= w {
					b = a
				}
				d[4*x] = uint8((uint32(r0[a]) + uint32(r0[b]) + uint32(r1[a]) + uint32(r1[b]) + 2) >> 2)
				d[4*x+1] = uint8((uint32(r0[a+1]) + uint32(r0[b+1]) + uint32(r1[a+1]) + uint32(r1[b+1]) + 2) >> 2)
				d[4*x+2] = uint8((uint32(r0[a+2]) + uint32(r0[b+2]) + uint32(r1[a+2]) + uint32(r1[b+2]) + 2) >> 2)
				d[4*x+3] = uint8((uint32(r0[a+3]) + uint32(r0[b+3]) + uint32(r1[a+3]) + uint32(r1[b+3]) + 2) >> 2)
			}
		}
	})
	return out, hw, hh
}

// The eight ways a picture lies, numbered as EXIF numbers them: 1 is
// upright, 6 needs a quarter turn clockwise to show upright, and so on.
// Each is a matrix taking a direction in the file to one on screen.
var orientations = [9][4]int{
	1: {1, 0, 0, 1},
	2: {-1, 0, 0, 1},
	3: {-1, 0, 0, -1},
	4: {1, 0, 0, -1},
	5: {0, 1, 1, 0},
	6: {0, -1, 1, 0},
	7: {0, -1, -1, 0},
	8: {0, 1, -1, 0},
}

// turned is an orientation after the user's turns and flip: quarter
// turns clockwise, then mirrored left to right.
func turned(orient, turns int, flip bool) int {
	if orient < 1 || orient > 8 {
		orient = 1
	}
	m := orientations[orient]
	for range ((turns % 4) + 4) % 4 {
		// A quarter turn clockwise: (x, y) to (-y, x).
		m = [4]int{-m[2], -m[3], m[0], m[1]}
	}
	if flip {
		m = [4]int{-m[0], -m[1], m[2], m[3]}
	}
	for o := 1; o <= 8; o++ {
		if orientations[o] == m {
			return o
		}
	}
	return 1
}

// sideways tells whether an orientation swaps width and height.
func sideways(orient int) bool { return orient >= 5 && orient <= 8 }

// upright returns pixels turned as an orientation says, with their size.
// Orientation 1 returns them as they are.
func upright(pix []byte, w, h, orient int) ([]byte, int, int) {
	if orient <= 1 || orient > 8 {
		return pix, w, h
	}
	dw, dh := w, h
	if sideways(orient) {
		dw, dh = h, w
	}
	out := make([]byte, len(pix))
	parallel(dh, func(y0, y1 int) {
		for dy := y0; dy < y1; dy++ {
			d := out[dy*4*dw : (dy+1)*4*dw]
			// Along a row of the result, the source moves by a constant
			// step from a start.
			var sx, sy, stepX, stepY int
			switch orient {
			case 2:
				sx, sy, stepX = w-1, dy, -1
			case 3:
				sx, sy, stepX = w-1, h-1-dy, -1
			case 4:
				sx, sy, stepX = 0, h-1-dy, 1
			case 5:
				sx, sy, stepY = dy, 0, 1
			case 6:
				sx, sy, stepY = dy, h-1, -1
			case 7:
				sx, sy, stepY = w-1-dy, h-1, -1
			case 8:
				sx, sy, stepY = w-1-dy, 0, 1
			}
			s := (sy*w + sx) * 4
			step := (stepY*w + stepX) * 4
			for i := 0; i < len(d); i += 4 {
				d[i], d[i+1], d[i+2], d[i+3] = pix[s], pix[s+1], pix[s+2], pix[s+3]
				s += step
			}
		}
	})
	return out, dw, dh
}

// parallel runs fn over the parts of 0..n on the machine's cores, when
// there is enough to be worth it.
func parallel(n int, fn func(from, to int)) {
	workers := min(runtime.GOMAXPROCS(0), 8)
	if n < 256 || workers < 2 {
		fn(0, n)
		return
	}
	var wg sync.WaitGroup
	step := (n + workers - 1) / workers
	for from := 0; from < n; from += step {
		wg.Go(func() { fn(from, min(from+step, n)) })
	}
	wg.Wait()
}
