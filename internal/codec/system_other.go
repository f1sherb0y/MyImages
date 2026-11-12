//go:build !windows

package codec

// Systems other than Windows have no codecs of their own here: the
// decoders written in Go take every file.
func decodeSystem(path string, opt Options) (*Picture, error) { return nil, ErrUnknown }

func decodeSystemBytes(data []byte, opt Options) (*Picture, error) { return nil, ErrUnknown }

// Metadata reads what a file says of its picture.
func Metadata(path string) []Field { return nil }

// Thumbnail returns a small picture of a file.
func Thumbnail(path string, px int) (*Picture, error) {
	return Decode(path, Options{MaxW: 2 * px, MaxH: 2 * px})
}

func decodeShell(path string, opt Options) (*Picture, error) { return nil, ErrUnknown }
