package media

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/google/uuid"
)

func pngBytes(t *testing.T, w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	img.Set(1, 1, color.RGBA{255, 0, 0, 255})
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// jpegWithEXIF вставляет APP1/Exif сегмент с "GPS" сразу после SOI.
func jpegWithEXIF(t *testing.T) []byte {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewGray(image.Rect(0, 0, 8, 8)), nil)
	raw := b.Bytes()
	payload := append([]byte("Exif\x00\x00"), []byte("GPS-SECRET-LOCATION")...)
	seg := []byte{0xFF, 0xE1, byte((len(payload) + 2) >> 8), byte(len(payload) + 2)}
	seg = append(seg, payload...)
	return append(append(append([]byte{}, raw[:2]...), seg...), raw[2:]...)
}

func TestProcessReencodesAndStripsEXIF(t *testing.T) {
	in := jpegWithEXIF(t)
	if !bytes.Contains(in, []byte("GPS-SECRET")) {
		t.Fatal("fixture broken")
	}
	out, err := Process(bytes.NewReader(in), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte("GPS-SECRET")) || bytes.Contains(out, []byte("Exif")) {
		t.Fatal("EXIF not stripped")
	}
	if !bytes.HasPrefix(out, []byte{0xFF, 0xD8}) {
		t.Fatal("output is not JPEG")
	}
}

func TestProcessRejects(t *testing.T) {
	cases := map[string]struct {
		data []byte
		max  int64
		want error
	}{
		"svg":       {[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 1 << 20, ErrUnsupported},
		"html":      {[]byte(`<html><body>hi</body></html>`), 1 << 20, ErrUnsupported},
		"too large": {pngBytes(t, 10, 10), 10, ErrTooLarge},
		"corrupt":   {append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 50)...), 1 << 20, ErrCorrupt},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Process(bytes.NewReader(c.data), c.max); !errors.Is(err, c.want) {
				t.Fatalf("got %v, want %v", err, c.want)
			}
		})
	}
}

func TestProcessDownscales(t *testing.T) {
	out, err := Process(bytes.NewReader(pngBytes(t, 5000, 100)), 10<<20)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out))
	if cfg.Width != MaxSide {
		t.Fatalf("width %d, want %d", cfg.Width, MaxSide)
	}
}

func TestOwnsKey(t *testing.T) {
	u := uuid.New()
	k := NewKey(u, PurposeDeliveryPhoto)
	if !OwnsKey(u, PurposeDeliveryPhoto, k) {
		t.Fatal("own key rejected")
	}
	if OwnsKey(uuid.New(), PurposeDeliveryPhoto, k) {
		t.Fatal("foreign key accepted")
	}
	if OwnsKey(u, PurposeBodyPhoto, k) {
		t.Fatal("wrong purpose accepted")
	}
	if OwnsKey(u, PurposeDeliveryPhoto, "u/"+u.String()+"/delivery/../../etc/passwd.jpg") {
		t.Fatal("path traversal accepted")
	}
}
