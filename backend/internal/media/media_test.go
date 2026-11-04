package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
	"time"

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

// PNG 6300x6300 весит ~50 КБ, но декодируется в ~160 МБ — должен отклоняться до декодирования.
func TestProcessRejectsDecompressionBomb(t *testing.T) {
	bomb := pngBytes(t, 6300, 6300)
	if _, err := Process(bytes.NewReader(bomb), 10<<20); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("bomb accepted: %v", err)
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
	pk := NewKey(u, PurposeProductImage)
	if !strings.HasPrefix(pk, "pub/") || !strings.HasPrefix(k, "priv/") {
		t.Fatalf("visibility prefixes wrong: %s %s", pk, k)
	}
	if PublicURL("https://cdn", k) != "" || PublicURL("https://cdn/", pk) != "https://cdn/"+pk {
		t.Fatal("PublicURL must only expose pub/ keys")
	}
	if OwnsKey(u, PurposeDeliveryPhoto, "priv/u/"+u.String()+"/delivery/../../etc/passwd.jpg") {
		t.Fatal("path traversal accepted")
	}
}

func TestBiometricRouting(t *testing.T) {
	owner := uuid.New()
	body, deliv, res := NewKey(owner, PurposeBodyPhoto), NewKey(owner, PurposeDeliveryPhoto), NewKey(owner, PurposeTryOnResult)
	if !IsBiometricKey(body) || !IsBiometricKey(res) || IsBiometricKey(deliv) || IsBiometricKey("priv/u/x/body") {
		t.Fatal("biometric key detection")
	}
	main, bio := NewMemory(), NewMemory()
	r := Routed{Default: main, Biometric: bio}
	ctx := context.Background()
	for _, k := range []string{body, deliv, res} {
		if err := r.Put(ctx, k, []byte("x"), "image/jpeg"); err != nil {
			t.Fatal(err)
		}
	}
	if !bio.Has(body) || !bio.Has(res) || bio.Has(deliv) || !main.Has(deliv) || main.Has(body) {
		t.Fatal("biometric objects must go only to the biometric storage")
	}
	if b, err := r.Get(ctx, body); err != nil || string(b) != "x" {
		t.Fatal("routed get")
	}
	var listed []string
	_ = r.List(ctx, "priv/", time.Now().Add(time.Second), func(k string) error { listed = append(listed, k); return nil })
	if len(listed) != 3 {
		t.Fatalf("list: %v", listed)
	}
	_ = r.Delete(ctx, body)
	if bio.Has(body) {
		t.Fatal("routed delete")
	}
	if _, err := main.Get(ctx, "nope"); err != ErrObjectNotFound {
		t.Fatal("missing object error")
	}
}

// jpegWithOrientation вставляет минимальный EXIF (II, IFD0 с одним тегом Orientation).
func jpegWithOrientation(t *testing.T, w, h int, o uint16) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w/4; x++ { // метка в левом верхнем углу
		for y := 0; y < h/4; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var b bytes.Buffer
	_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 95})
	tiff := []byte{'I', 'I', 42, 0, 8, 0, 0, 0, 1, 0, 0x12, 0x01, 3, 0, 1, 0, 0, 0, byte(o), 0, 0, 0, 0, 0, 0, 0}
	seg := append([]byte("Exif\x00\x00"), tiff...)
	app1 := []byte{0xFF, 0xE1, byte((len(seg) + 2) >> 8), byte(len(seg) + 2)}
	raw := b.Bytes()
	return append(append(append([]byte{}, raw[:2]...), append(app1, seg...)...), raw[2:]...)
}

func TestProcessAppliesEXIFOrientation(t *testing.T) {
	for o, wantPortrait := range map[uint16]bool{1: false, 3: false, 6: true, 8: true} {
		in := jpegWithOrientation(t, 80, 40, o)
		if got := jpegOrientation(in); got != int(o) {
			t.Fatalf("parsed orientation %d, want %d", got, o)
		}
		out, err := Process(bytes.NewReader(in), 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		img, err := jpeg.Decode(bytes.NewReader(out))
		if err != nil {
			t.Fatal(err)
		}
		b := img.Bounds()
		if (b.Dy() > b.Dx()) != wantPortrait {
			t.Fatalf("orientation %d: got %dx%d", o, b.Dx(), b.Dy())
		}
		// красная метка из левого верхнего угла исходника: при 3 (180°) — справа внизу
		if o == 3 {
			r, _, _, _ := img.At(b.Dx()-3, b.Dy()-3).RGBA()
			if r>>8 < 200 {
				t.Fatal("180° rotation not applied")
			}
		}
	}
	if jpegOrientation([]byte("not a jpeg")) != 1 {
		t.Fatal("garbage must be orientation 1")
	}
}
