// Decbench times the decoders on files: decbench [-max N] [-n runs] file...
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"time"

	"myimages/internal/codec"
)

func main() {
	maxSide := flag.Int("max", 0, "decode reduced to fit a square of this side")
	runs := flag.Int("n", 3, "runs per file")
	out := flag.String("o", "", "write the last picture decoded as a PNG")
	flag.StringVar(&codec.Force, "via", "", "decode with \"wic\" or \"go\" only")
	flag.Parse()
	var last *codec.Picture
	for _, path := range flag.Args() {
		best := time.Duration(1 << 62)
		var p *codec.Picture
		var err error
		for range *runs {
			t := time.Now()
			p, err = codec.Decode(path, codec.Options{MaxW: *maxSide, MaxH: *maxSide})
			if d := time.Since(t); d < best {
				best = d
			}
			if err != nil {
				break
			}
		}
		if err != nil {
			fmt.Printf("%-40s ERROR %v\n", path, err)
			continue
		}
		last = p
		fmt.Printf("%8.1f ms  %5dx%-5d of %5dx%-5d %-8s via %-5s orient %d frames %d opaque %v  %s\n",
			float64(best.Microseconds())/1000, p.W, p.H, p.FullW, p.FullH, p.Format, p.Via, p.Orient, p.Frames, p.Opaque, path)
	}
	if *out != "" && last != nil {
		f, err := os.Create(*out)
		if err == nil {
			err = png.Encode(f, last.RGBA())
			f.Close()
		}
		if err != nil {
			fmt.Println(err)
		}
	}
}
