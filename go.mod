module myimages

go 1.27.3

require (
	github.com/ajroetker/go-jpeg2000 v0.0.2
	github.com/egoist/mygo v0.9.0
	github.com/ftrvxmtrx/tga v0.0.0-20150524081124-bd8e8d5be13a
	github.com/gen2brain/avif v0.6.0
	github.com/gen2brain/jpegxl v0.6.0
	github.com/mdouchement/hdr v0.2.4
	github.com/mokiat/goexr v0.1.0
	github.com/oov/psd v0.0.0-20260818185439-a5d50ec0acac
	github.com/samuel/go-pcx v1.0.0
	github.com/spakin/netpbm v1.3.2
	github.com/xfmoulet/qoi v0.2.0
	golang.org/x/image v0.46.0
	golang.org/x/sys v0.48.0
)

require (
	github.com/ProtonMail/go-crypto v1.4.1 // indirect
	github.com/ajroetker/go-highway v0.0.4 // indirect
	github.com/andybalholm/brotli v1.2.1 // indirect
	github.com/cloudflare/circl v1.6.2 // indirect
	github.com/ebitengine/purego v0.11.1 // indirect
	github.com/go-text/typesetting v0.3.5 // indirect
	github.com/gopherjs/gopherjs v1.21.0 // indirect
	github.com/klauspost/compress v1.18.6 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/tetratelabs/wazero v1.12.0 // indirect
	github.com/ulikunitz/xz v0.5.15 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.digitalxero.dev/rpm v0.3.4 // indirect
	golang.org/x/crypto v0.57.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gonum.org/v1/gonum v0.12.0 // indirect
)

tool github.com/egoist/mygo/cmd/mygo

replace github.com/egoist/mygo => ./third_party/mygo
