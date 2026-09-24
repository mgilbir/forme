package layout

import (
	"strings"
	"testing"
)

// An SVG is read by its own reader, not through the cascade, so nothing in
// style stands between its names and values and the comparisons made on them.
// Those compared with strings.EqualFold and strings.ToLower, which are
// Unicode's, so a fill of "blac\u212A" (KELVIN SIGN) was black. It is not: a
// presentation attribute is a CSS value, whose colour names are ASCII
// case-insensitive. (A root named with a LONG S, "<\u017Fvg>", was compared the
// same way, but encoding/xml refuses the name as it reads it — it classes name
// characters by XML 1.0's older tables, which leave U+017F out — and the
// comparison is never reached, so there is no case to write for it.)
func TestAnSVGsNamesAndKeywordsAreFoldedAsASCII(t *testing.T) {
	const rect = `<rect width="100%" height="100%" fill="green"/>`
	for _, tc := range []struct {
		what, body string
		drawn      bool
	}{
		{"an svg root", `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` + rect + `</svg>`, true},
		{"a fill in capitals", `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
			`<rect width="100%" height="100%" fill="BLACK"/></svg>`, true},
		{"a fill with a KELVIN SIGN", `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10">` +
			`<rect width="100%" height="100%" fill="` + "blac\u212A" + `"/></svg>`, false},
	} {
		got := svgOf(t, tc.body)
		if drawn := got != nil && got.Solid != nil; drawn != tc.drawn {
			t.Errorf("%s: reduced to a colour %v, want %v", tc.what, drawn, tc.drawn)
		}
	}
}

// TestAFeatureTagIsCaseSensitive is the other direction. CSS Fonts 4 §6.12
// makes an <opentype-tag> case-sensitive, so "KERN" is not "kern" but some
// other feature, and the one thing this can say about kerning on a face with
// none is not said about it. It was compared with strings.EqualFold.
func TestAFeatureTagIsCaseSensitive(t *testing.T) {
	if got := unappliedFontFeatures(`"kern" 0`, nil); got != "" {
		t.Errorf(`"kern" 0 on a face with no kerning was reported: %s`, got)
	}
	if got := unappliedFontFeatures(`"KERN" 0`, nil); !strings.Contains(got, "KERN") {
		t.Errorf(`"KERN" 0 was taken for "kern": the report is %q`, got)
	}
}
