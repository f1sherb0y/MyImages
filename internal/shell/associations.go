package shell

// CommonImageExtensions is deliberately narrower than the decoder list: a
// user choosing "Make default" does not also hand over SVG, PSD or RAW files.
// Return a fresh slice so callers cannot change the application's policy.
func CommonImageExtensions() []string {
	return []string{"jpg", "jpeg", "png", "gif", "webp", "bmp", "tif", "tiff", "heic", "heif", "avif"}
}

// DefaultResult lists only associations verified by the Windows shell.
type DefaultResult struct {
	Succeeded []string
}
