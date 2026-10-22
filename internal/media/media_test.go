package media

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func pngOf(w, h int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, x%h, color.NRGBA{R: 200, A: 128})
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func TestProcessValidImage(t *testing.T) {
	src := append(pngOf(2400, 3600), []byte("<?php system($_GET['x']); ?>")...) // trailing payload
	format, vs, err := Process(src)
	if err != nil || format != "png" || len(vs) != 2 {
		t.Fatalf("process: %v %s %d", err, format, len(vs))
	}
	if vs[0].Name != "full" || vs[0].Height != 2000 || vs[0].Width != 1333 {
		t.Fatalf("full: %dx%d", vs[0].Width, vs[0].Height)
	}
	if vs[1].Name != "card" || vs[1].Width != 480 {
		t.Fatalf("card: %dx%d", vs[1].Width, vs[1].Height)
	}
	for _, v := range vs {
		if bytes.Contains(v.Data, []byte("<?php")) || !bytes.HasPrefix(v.Data, []byte{0xFF, 0xD8}) {
			t.Fatalf("%s is not a clean JPEG", v.Name)
		}
	}
}

func TestProcessRejects(t *testing.T) {
	cases := map[string]struct {
		in   []byte
		want error
	}{
		"script disguised as png": {[]byte("<script>alert(1)</script>"), ErrNotAnImage},
		"svg":                     {[]byte(`<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`), ErrNotAnImage},
		"truncated png":           {pngOf(100, 100)[:60], ErrDecodeFailed},
		"tiny":                    {pngOf(8, 8), ErrDimensions},
		"huge dimensions":         {pngOf(5001, 40), ErrDimensions},
		"too many bytes":          {make([]byte, MaxBytes+1), ErrTooLarge},
	}
	for name, c := range cases {
		if _, _, err := Process(c.in); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", name, err, c.want)
		}
	}
}
