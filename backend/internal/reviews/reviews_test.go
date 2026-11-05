package reviews

import "testing"

func TestShortName(t *testing.T) {
	for in, want := range map[string]string{
		"Азиз Каримов":         "Азиз К.",
		"Азиз <b>Каримов</b>":  "Азиз К.",
		"  zokir   abdirimov ": "zokir A.",
		"Мадина":               "Мадина",
		"<script></script>":    "",
		"":                     "",
		"Ali 123":              "Ali",
	} {
		if got := ShortName(in); got != want {
			t.Errorf("ShortName(%q) = %q, want %q", in, got, want)
		}
	}
}
