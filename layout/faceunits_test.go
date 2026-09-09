package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// "Nch" is the width of N digits, and that is what the unit is for.
//
// CSS Values §5.1.1: ch is "the advance measure of the '0' glyph in the
// element's font". An author writes "width: 40ch" for a column forty digits
// wide, and the digits are set from the same face by the same arithmetic — so
// the two have to be the same length, and they were not.
//
// A face's advance is not a whole number of layout units: Courier's digit at
// 13px is 7.8px, which is 499.2 sixty-fourths. Quantising the advance and then
// multiplying is N quantisations, up to N-1 units short of the one quantisation
// of the product — so a box of 4ch was a sixty-fourth of a pixel narrower than
// the four digits it was built from, and the word broke. The suite writes it as
// both overflow-wrap-*-003, which put "PASS FAIL" in a box of 4ch.
//
// The units a face measures are therefore carried in pixels through the cascade
// and quantised once, at the end, where the length lands on the page. See
// style.LengthContext and paragraph.Breaker.MeasurePx.
func TestABoxOfNFaceUnitsHoldsNOfThem(t *testing.T) {
	for _, tc := range []struct {
		family, unit, sample string
		size                 float64
	}{
		{"Courier", "ch", "0", 13},
		{"Courier", "ch", "0", 20},
		{"Helvetica", "ch", "0", 13},
		{"Helvetica", "ch", "0", 17},
		{"Times-Roman", "ch", "0", 13},
	} {
		face, err := shape.Standard(tc.family)
		if err != nil {
			t.Fatalf("loading %s: %v", tc.family, err)
		}
		for _, n := range []int{2, 4, 7, 16, 40} {
			decl := strings.NewReplacer("$n", itoa(n), "$u", tc.unit,
				"$f", tc.family, "$s", ftoa(tc.size))
			css := decl.Replace(`#a { font-family: $f; font-size: $spx; width: $n$u }`)
			root := layoutOf(t, 4000, `<div id="a"></div>`, noDefaults+css)
			got := find(t, root, "a").BorderRect.W

			// N of the sample measured as one string, which is what the box is
			// supposed to hold and what a line would put in it.
			want, _ := style.FromPx(face.Measure(strings.Repeat(tc.sample, n), tc.size))
			if got != want {
				t.Errorf("%s at %gpx: %d%s is %v and %d of %q measure %v",
					tc.family, tc.size, n, tc.unit, got, n, tc.sample, want)
			}
		}
	}
}
