package products

import "testing"

func TestParsePrice(t *testing.T) {
	cases := map[string]int64{"150000": 15000000, "1 500 000": 150000000, "99.5": 9950, "99,05": 9905, "10 сум": 1000}
	for in, want := range cases {
		if got, ok := parsePrice(in); !ok || got != want {
			t.Errorf("parsePrice(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "0", "-5", "abc", "1.234", "1e5"} {
		if _, ok := parsePrice(in); ok {
			t.Errorf("parsePrice(%q) accepted", in)
		}
	}
}

func TestParseImageName(t *testing.T) {
	cases := []struct{ in, sku, angle string }{
		{"A-100_front.jpg", "A-100", "front"},
		{"folder/A_B_back.PNG", "A_B", "back"},
		{`C:\photos\X1_2.webp`, "X1", "side"},
		{"Z9_сзади.jpeg", "Z9", "back"},
	}
	for _, c := range cases {
		sku, angle, ok := ParseImageName(c.in)
		if !ok || sku != c.sku || angle != c.angle {
			t.Errorf("ParseImageName(%q) = %q %q %v", c.in, sku, angle, ok)
		}
	}
	for _, in := range []string{"front.jpg", "A100.jpg", "A_top.jpg", "_front.jpg", "A_.jpg"} {
		if _, _, ok := ParseImageName(in); ok {
			t.Errorf("ParseImageName(%q) accepted", in)
		}
	}
}

func TestParseBool(t *testing.T) {
	for _, s := range []string{"Да", "yes", "1", "ha", " + "} {
		if v, ok := parseBool(s); !ok || !v {
			t.Errorf("%q", s)
		}
	}
	for _, s := range []string{"нет", "No", "0", "yo'q"} {
		if v, ok := parseBool(s); !ok || v {
			t.Errorf("%q", s)
		}
	}
	if _, ok := parseBool("maybe"); ok {
		t.Error("maybe accepted")
	}
}

func TestParseSizeList(t *testing.T) {
	got, ok := ParseSizeList("s:2; M=5, 42 : 0")
	if !ok || len(got) != 3 || got[0].Size != "S" || got[0].Stock != 2 || got[2].Size != "42" || got[2].Stock != 0 {
		t.Fatalf("got %v %v", got, ok)
	}
	for _, bad := range []string{"S", "S:x", "S:1; s:2", "S:-1", ";"} {
		if _, ok := ParseSizeList(bad); ok {
			t.Errorf("%q accepted", bad)
		}
	}
}
