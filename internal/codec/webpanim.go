package codec

import (
	"bytes"
	"image"
	"time"

	"github.com/gen2brain/webp"
)

// animatedWebP tells whether the bytes a file starts with are those of
// an animated WebP: its header has a flag for it.
func animatedWebP(head []byte) bool {
	return len(head) >= 21 && string(head[:4]) == "RIFF" && string(head[8:16]) == "WEBPVP8X" && head[20]&0x02 != 0
}

// WebPFrames decodes the frames of an animated WebP, each composed as it
// shows, with how long each does.
func WebPFrames(data []byte) ([]image.Image, []time.Duration, error) {
	all, err := webp.DecodeAll(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	delays := make([]time.Duration, len(all.Image))
	for i := range delays {
		if i < len(all.Delay) {
			delays[i] = time.Duration(all.Delay[i]) * time.Millisecond
		}
	}
	return all.Image, delays, nil
}
