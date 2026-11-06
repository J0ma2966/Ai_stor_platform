package tryon

import (
	"strings"
	"testing"
)

// «Двойка»: верх и низ на одном фото — Garment{Zone: top, Set: true}.
func TestTwoPieceSetGarments(t *testing.T) {
	set := Garment{Zone: ZoneTop, Set: true}
	if got := strings.Join(set.covers(), ","); got != "top,bottom" {
		t.Fatalf("set covers %s", got)
	}
	if got := strings.Join((Garment{Zone: ZoneBottom, Set: true}).covers(), ","); got != "bottom" {
		t.Fatalf("set flag outside top zone must be ignored: %s", got)
	}
	if fashnCategoryOf(set) != "one-pieces" || fashnCategoryOf(Garment{Zone: ZoneTop}) != "tops" {
		t.Fatal("fashn category for a set must be one-pieces")
	}
	if !strings.Contains(garmentText(set), "two-piece set") || garmentText(Garment{Zone: ZoneTop}) != zoneText[ZoneTop] {
		t.Fatal("gemini label")
	}

	r := compReq(t, ZoneTop, ZoneShoes)
	r.Garments[0].Set = true
	imgs, missing, err := PrepareComposite(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(imgs[3].label, "TWO-PIECE SET") {
		t.Fatalf("set label: %q", imgs[3].label)
	}
	if strings.Join(missing, ",") != "headwear" {
		t.Fatalf("bottom is covered by the set, missing = %v", missing)
	}
	p := OpenAIPrompt(imgs, missing, r.Gender, r.Body)
	if !strings.Contains(p, "Image 4: TWO-PIECE SET") || strings.Contains(p, "BOTTOM, ") || strings.Contains(p, "for: HEADWEAR, BOTTOM") {
		t.Fatalf("prompt:\n%s", p)
	}

	withBottom := compReq(t, ZoneTop, ZoneBottom)
	withBottom.Garments[0].Set = true
	if _, _, err := PrepareComposite(withBottom); ErrorCode(err) != CodeUnsupported {
		t.Fatalf("set + bottom must be rejected: %v", err)
	}
}
