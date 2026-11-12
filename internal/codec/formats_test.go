package codec

import (
	"strings"
	"testing"
)

// The extensions ImageGlass 10 opens by default: all are ours too.
const imageGlass = `3fr ari arw avif bay bmp cap cr2 cr3 crw cur cut dcr dcs dds dib dng drf eip emf erf exif exr fax fff fits flif gif gifv gpr hdr heic heif hif ico iiq jfif jp2 jpe jpeg jpg jxl k25 kdc mdc mef mjpeg mos mrw nef nrw obm orf pbm pcx pef pgm png ppm psb psd ptx pxn qoi r3d raf raw rw2 rwl rwz sr2 srf srw svg svgz tga tif tiff viff webp wmf wpg x3f xbm xpm xv`

func TestKnowsWhatImageGlassOpens(t *testing.T) {
	for _, ext := range strings.Fields(imageGlass) {
		if !Known(ext) {
			t.Errorf("%s is not known", ext)
		}
	}
	if n := len(Extensions()); n < 100 {
		t.Errorf("only %d extensions", n)
	}
}
