package layout

import (
	"strconv"
	"testing"
)

// letter-spacing beside a picture, CSS Text §8.2.
//
// "For the purpose of letter-spacing, each consecutive run of atomic inlines
// (such as images and inline blocks) is treated as a single typographic
// character unit." So a picture is a character to this property — it takes a
// spacing after it exactly as a letter does — and a row of pictures is *one*
// character, so the spacing goes after the last of them and nowhere between.
//
// Courier at 20px, whose advance is 600 units of 1000: a character is 12px, and
// the boxes below are 10px so that the two are never confused in a sum.

const atomicSpacingCSS = noDefaults + `
#d { font-family: Courier; font-size: 20px; width: max-content }
.b { display: inline-block; width: 10px; height: 10px }
`

// atomicWidth shrink-wraps one line and returns how wide it came out.
func atomicWidth(t *testing.T, css, body string) float64 {
	t.Helper()
	root := layoutOf(t, 900, `<div id="d" style="`+css+`">`+body+`</div>`, atomicSpacingCSS)
	return find(t, root, "d").BorderRect.W.Px()
}

func TestARunOfPicturesTakesOneSpacingAfterIt(t *testing.T) {
	const box = `<span class="b"></span>`
	for _, tc := range []struct {
		body string
		want float64
		what string
	}{
		{"AD", 12 + 20 + 12, "two letters, for the arithmetic"},
		{"A" + box + "D", 12 + 20 + 10 + 20 + 12,
			"one picture, spaced on both sides like a letter"},
		{"A" + box + box + "D", 12 + 20 + 10 + 10 + 20 + 12,
			"two pictures are one unit, so there is no gap between them"},
		{"A" + box + box + box + "D", 12 + 20 + 10 + 10 + 10 + 20 + 12,
			"and three are still one"},
		{"A" + box, 12 + 20 + 10,
			"nothing follows the picture, so nothing follows it"},
		{box + "D", 10 + 20 + 12, "and a picture that begins the line"},
		{box, 10, "a picture on its own"},
		{box + box, 20, "and two, with no character anywhere"},
	} {
		if got := atomicWidth(t, "letter-spacing: 20px", tc.body); got != tc.want {
			t.Errorf("%s\n%s shrink-wraps to %gpx, want %g", tc.what, tc.body, got, tc.want)
		}
	}
}

// The spacing between two units is the innermost element containing both of
// them, which for a picture is the same rule as for a letter.
func TestThePictureTakesTheSpacingOfTheElementThatHoldsBoth(t *testing.T) {
	const box = `<span class="b"></span>`
	// The picture and the D are both in the outer element, which sets 20px; the
	// span around the picture sets 40 and governs nothing, because nothing
	// inside it is beside anything else.
	got := atomicWidth(t, "letter-spacing: 20px",
		`A<span style="letter-spacing: 40px">`+box+`</span>D`)
	if want := float64(12 + 20 + 10 + 20 + 12); got != want {
		t.Errorf("shrink-wraps to %gpx, want %g — the gap after the picture is "+
			"the outer element's and not the span's", got, want)
	}
}

// And the line leaves it out at its end, which is §8.2's other half: "it is not
// applied at the beginning or end of a line".
func TestASpacingAfterAPictureDoesNotWidenTheLineItEnds(t *testing.T) {
	const box = `<span class="b"></span>`
	// Exactly wide enough for "A", the picture and the gap between them. If the
	// gap after the picture counted, the line would not fit and the D would not
	// be the thing that wrapped.
	const width = 12 + 20 + 10
	root := layoutOf(t, 900,
		`<div id="d" style="letter-spacing: 20px; width: `+strconv.Itoa(width)+`px">A`+
			box+`D</div>`, atomicSpacingCSS+`#d { width: auto }`)
	if got := len(linesOf(t, root, "d")); got != 2 {
		t.Errorf("the line came out in %d pieces, want 2 — the gap after the "+
			"picture is at the end of the line and does not count towards it", got)
	}
}

// And the narrowest the content can be leaves it out as well, which is the same
// account the line fill keeps: a run that ends at the picture ends at the gap,
// and a gap a line break falls on is not a gap.
//
// The two measurements have to agree or a box is shrink-wrapped to a width its
// own content does not have.
func TestTheGapAfterAPictureIsNotPartOfTheNarrowestItCanBe(t *testing.T) {
	const box = `<span class="b"></span>`
	// Spaces either side, so the picture is a run of its own and the minimum is
	// the widest of the three. A letter is 12px and the picture 10, so the
	// answer is a letter — unless the 20px gap after the picture is counted,
	// which would make the picture the widest thing on the line.
	got := atomicWidth(t, "letter-spacing: 20px; width: min-content", "A "+box+" D")
	if want := float64(12); got != want {
		t.Errorf("the narrowest form is %gpx, want %g — the gap after the picture "+
			"was counted into a run that ends there", got, want)
	}
}
