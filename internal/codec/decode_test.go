package codec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Each format is decoded from a file ImageMagick writes of one reference
// picture, and compared with it. ImageMagick only makes the test's
// files: the program never runs it.
func TestFormats(t *testing.T) {
	magick, err := exec.LookPath("magick")
	if err != nil {
		t.Skip("no ImageMagick to make the files with")
	}
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref.png")
	// 97×61: odd, to catch rows padded wrongly. Opaque, so that formats
	// without transparency compare.
	run := func(args ...string) error {
		out, err := exec.Command(magick, args...).CombinedOutput()
		if err != nil {
			t.Logf("magick %v: %s", args, out)
		}
		return err
	}
	if run("-size", "97x61", "gradient:#ff3020-#2040ff", "-fill", "white", "-draw", "circle 48,30 48,12",
		"-fill", "#20c050", "-draw", "rectangle 6,6 30,22", "-depth", "8", ref) != nil {
		t.Fatal("no reference picture")
	}
	want, err := Decode(ref, Options{})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		// tolerance is the mean difference of a channel allowed, of 255;
		// loose for formats that keep fewer colors or only light.
		tolerance float64
		gray      bool
	}{
		{"a.bmp", nil, 1, false}, {"a.dib", nil, 1, false}, {"a.gif", nil, 12, false}, {"a.jpg", []string{"-quality", "95"}, 6, false},
		{"a.tif", nil, 1, false}, {"lzw.tif", []string{"-compress", "LZW"}, 1, false}, {"a.ico", nil, 1, false},
		{"a.webp", []string{"-define", "webp:lossless=true"}, 1, false}, {"a.avif", []string{"-quality", "95"}, 8, false},
		{"a.jxl", []string{"-quality", "100"}, 3, false}, {"a.jp2", []string{"-quality", "100"}, 3, false},
		{"a.dds", []string{"-define", "dds:compression=none"}, 1, false},
		{"a.tga", nil, 1, false}, {"rle.tga", []string{"-compress", "RLE"}, 1, false}, {"a.qoi", nil, 1, false},
		{"a.pcx", nil, 1, false}, {"a.ppm", nil, 1, false}, {"ascii.ppm", []string{"-compress", "none"}, 1, false},
		{"a.pgm", nil, 1, true}, {"a.pam", nil, 1, false}, {"a.pfm", []string{"-colorspace", "RGB"}, 4, false}, {"a.hdr", []string{"-colorspace", "RGB"}, 6, false},
		{"zip.exr", []string{"-colorspace", "RGB", "-compress", "Zip"}, 3, false}, {"piz.exr", []string{"-colorspace", "RGB", "-compress", "Piz"}, 3, false},
		{"rle.exr", []string{"-colorspace", "RGB", "-compress", "RLE"}, 3, false},
		{"a.psd", []string{"-depth", "8"}, 1, false}, {"deep.psd", []string{"-depth", "16", "-type", "TrueColor"}, 2, false}, {"indexed.psd", []string{"-type", "Palette"}, 12, false},
		{"raw.psd", []string{"-depth", "8", "-compress", "None"}, 1, false}, {"a.psb", []string{"-depth", "8"}, 1, false},
		{"a.fits", nil, 1, false}, {"deep.fits", []string{"-depth", "16"}, 20, false}, {"a.viff", nil, 1, false}, {"a.xv", nil, 1, false}, {"a.wpg", nil, 14, false},
		{"a.xpm", nil, 1, false}, {"a.svg", nil, 40, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.name)
			args := append([]string{ref}, c.args...)
			if run(append(args, path)...) != nil {
				t.Skip("ImageMagick does not write this format here")
			}
			if c.name == "a.svg" {
				t.Skip("ImageMagick's SVG is a picture inside a drawing, which the renderer leaves out")
			}
			got, err := Decode(path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			if got.Via != "wic" && got.Via != "go" {
				t.Errorf("decoded via %q", got.Via)
			}
			if got.W != want.W || got.H != want.H {
				t.Fatalf("decoded %d×%d, want %d×%d", got.W, got.H, want.W, want.H)
			}
			var sum float64
			for i := 0; i < len(want.Pix); i += 4 {
				r, g, b := float64(want.Pix[i]), float64(want.Pix[i+1]), float64(want.Pix[i+2])
				if c.gray {
					y := 0.2126*r + 0.7152*g + 0.0722*b
					r, g, b = y, y, y
				}
				for j, v := range [3]float64{r, g, b} {
					d := float64(got.Pix[i+j]) - v
					if d < 0 {
						d = -d
					}
					sum += d
				}
			}
			if mean := sum / float64(len(want.Pix)/4*3); mean > c.tolerance {
				t.Errorf("differs from the reference by %.1f a channel, more than %.0f", mean, c.tolerance)
			}
			// A file cut short, or with bytes changed, must not take the
			// program down.
			data, _ := os.ReadFile(path)
			for _, n := range []int{len(data) / 2, len(data) / 7, 30} {
				if n < len(data) {
					_, _ = DecodeBytes(data[:n], Options{})
				}
			}
			for at := 16; at < len(data); at += max(len(data)/9, 1) {
				bad := append([]byte(nil), data...)
				bad[at] ^= 0xFF
				_, _ = DecodeBytes(bad, Options{})
			}
		})
	}
}

// Formats ImageMagick writes no file of here, from bytes written by
// hand.
func TestSmallFormats(t *testing.T) {
	// 10×2: the first row black on its left half, the second all white.
	xbm := `#define t_width 10
#define t_height 2
static char t_bits[] = {
 0x1f, 0x00, 0x00, 0x00 };
`
	p, err := DecodeBytes([]byte(xbm), Options{})
	if err != nil || p.W != 10 || p.H != 2 {
		t.Fatalf("XBM: %v %+v", err, p)
	}
	if p.Pix[0] != 0 || p.Pix[4*4] != 0 || p.Pix[5*4] != 255 || p.Pix[10*4] != 255 {
		t.Errorf("XBM pixels: % x", p.Pix[:44])
	}
	// A Dr. Halo picture of 4×2: a run of four 9s, then 1 2 3 4 as they
	// are; each row behind its length and ended by a zero.
	cut := []byte{4, 0, 2, 0, 0, 0, 3, 0, 0x84, 9, 0, 6, 0, 0x04, 1, 2, 3, 4, 0}
	p, err = decodeCUT(cut, Options{})
	if err != nil || p.W != 4 || p.H != 2 {
		t.Fatalf("CUT: %v %+v", err, p)
	}
	// Grays spread from the least value, 1, to the most, 9.
	if p.Pix[0] != 255 || p.Pix[3*4] != 255 || p.Pix[4*4] != 0 || p.Pix[7*4] <= p.Pix[5*4] {
		t.Errorf("CUT pixels: % x", p.Pix)
	}
}
