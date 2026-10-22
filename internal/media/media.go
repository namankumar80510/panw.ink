// Package media validates and re-encodes uploaded images (IMPLEMENTATION.md
// §13.2 item 8): the real format is sniffed and decoded (JPEG, PNG, WebP
// only; never SVG), dimensions are checked from the header before a full
// decode, and the image is re-encoded as baseline JPEG, which drops all
// metadata (EXIF, GPS, ICC, comments) and any appended payload.
package media

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png" // register decoder
	"net/http"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // register decoder
)

// Limits.
const (
	MaxBytes     = 5 << 20
	MaxDimension = 5000
	MinDimension = 32
)

// Variant sizes: the longest edge of "full", the width of "card".
const (
	FullMaxEdge = 2000
	CardWidth   = 480
)

// Rejection reasons (stable codes for assets.error_code).
var (
	ErrTooLarge     = errors.New("too_large")
	ErrNotAnImage   = errors.New("not_an_image")
	ErrFormat       = errors.New("unsupported_format")
	ErrDimensions   = errors.New("bad_dimensions")
	ErrDecodeFailed = errors.New("decode_failed")
)

// Variant is one re-encoded output.
type Variant struct {
	Name          string // full | card
	Data          []byte
	Width, Height int
}

// Process validates raw and returns the variants.
func Process(raw []byte) (format string, out []Variant, err error) {
	if len(raw) > MaxBytes {
		return "", nil, ErrTooLarge
	}
	sniff := http.DetectContentType(raw)
	switch sniff {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return "", nil, ErrNotAnImage
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return "", nil, ErrDecodeFailed
	}
	if format != "jpeg" && format != "png" && format != "webp" {
		return "", nil, ErrFormat
	}
	if cfg.Width < MinDimension || cfg.Height < MinDimension || cfg.Width > MaxDimension || cfg.Height > MaxDimension {
		return "", nil, ErrDimensions
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", nil, ErrDecodeFailed
	}
	full, err := encode(fit(img, FullMaxEdge, FullMaxEdge))
	if err != nil {
		return "", nil, err
	}
	card, err := encode(fit(img, CardWidth, MaxDimension))
	if err != nil {
		return "", nil, err
	}
	return format, []Variant{
		{Name: "full", Data: full.data, Width: full.w, Height: full.h},
		{Name: "card", Data: card.data, Width: card.w, Height: card.h},
	}, nil
}

// fit scales img down (never up) to fit in maxW x maxH, flattening any
// transparency onto white.
func fit(img image.Image, maxW, maxH int) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	scale := min(1, min(float64(maxW)/float64(w), float64(maxH)/float64(h)))
	nw, nh := max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	return dst
}

type encoded struct {
	data []byte
	w, h int
}

func encode(img image.Image) (encoded, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return encoded{}, fmt.Errorf("media: encode: %w", err)
	}
	b := img.Bounds()
	return encoded{data: buf.Bytes(), w: b.Dx(), h: b.Dy()}, nil
}
