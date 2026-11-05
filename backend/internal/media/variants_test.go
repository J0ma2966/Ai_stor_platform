package media

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"

	"github.com/google/uuid"
)

func TestMakeVariants(t *testing.T) {
	var b bytes.Buffer
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3000, 4000)), &jpeg.Options{Quality: 90})
	out, err := MakeVariants(b.Bytes(), ProductVariants)
	if err != nil {
		t.Fatal(err)
	}
	for _, side := range ProductVariants {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(out[side]))
		if err != nil {
			t.Fatalf("variant %d: %v", side, err)
		}
		if cfg.Height != side || cfg.Width != side*3/4 {
			t.Fatalf("variant %d: %dx%d", side, cfg.Width, cfg.Height)
		}
	}
	// маленький оригинал не увеличивается
	b.Reset()
	_ = jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 300, 400)), nil)
	out, err = MakeVariants(b.Bytes(), ProductVariants)
	if err != nil {
		t.Fatal(err)
	}
	if cfg, _ := jpeg.DecodeConfig(bytes.NewReader(out[VariantLarge])); cfg.Height != 400 {
		t.Fatalf("small original resized to %d", cfg.Height)
	}
	if _, err := MakeVariants([]byte("not an image"), ProductVariants); err != ErrCorrupt {
		t.Fatalf("corrupt: %v", err)
	}
}

func TestVariantKeyIsNotOwnable(t *testing.T) {
	owner := uuid.New()
	k := NewKey(owner, PurposeProductImage)
	v := VariantKey(k, VariantCard)
	if v == k || OwnsKey(owner, PurposeProductImage, v) {
		t.Fatalf("variant key %q must differ from original and not pass OwnsKey", v)
	}
	if PublicURL("https://m", v) == "" {
		t.Fatal("variant of public key must be public")
	}
}
