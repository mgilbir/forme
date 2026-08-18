package layout

import (
	"strconv"
	"testing"

	"github.com/mgilbir/forme/style"
)

// What §16.1's indent does to a box's intrinsic widths.
//
// It moves the first line and no other, which makes the answer the greater of
// two different lines: the first line with the indent applied to it, and the
// widest of the ones after it. Neither "the widest line" nor "the widest line
// plus the indent" is that number, and a box with only one line cannot tell the
// three apart — which is how the old arithmetic survived.
//
// The minimum was not measured at all, on the reasoning that a minimum may come
// out small and must never come out large. That reasoning is right in general
// and wrong here: a float in a container narrower than its content is sized to
// its min-content width, so a float holding an indented paragraph was sized to
// the paragraph alone and its first line ran the whole indent past the border.
//
// The hanging indent is the same rule with the other sign. It narrows the first
// line, and a box sized as though it had not is wider than its content asks for
// by the whole hang.

// floatWidth is the content width a shrink-to-fit float settled on.
//
// The container is one pixel wide, which is how the suite forces the question:
// with no room at all the float takes its min-content width, and nothing else
// about the containing block can enter the answer.
func floatWidth(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	ops := paintOf(t, `<div id="outer">`+markup+`</div>`,
		`#outer { width: 1px; font-size: 10px }
		 #f { float: left; background: rgb(0,0,255); height: 10px }
		 span { display: inline-block; height: 10px; background: rgb(0,128,0) }
		 `+css)
	// The content is checked first, so that "no float background" can be read
	// as "a float of no width" rather than as a fixture that laid out nothing.
	// A box of zero width paints no rectangle, and zero is an answer this asks
	// for: a first line hung further out than it is long needs no room at all.
	if len(fillsOf(ops, green)) == 0 {
		t.Fatalf("nothing inside the float was painted; the fixture laid out nothing")
	}
	got := fillsOf(ops, blue)
	switch len(got) {
	case 0:
		return 0
	case 1:
		return got[0].W
	}
	t.Fatalf("%d float fills, want at most 1: %v", len(got), got)
	return 0
}

// TestAPositiveIndentWidensTheFloatThatHoldsIt is the bug in one line: the
// min-content width of an indented paragraph includes the indent.
func TestAPositiveIndentWidensTheFloatThatHoldsIt(t *testing.T) {
	plain := floatWidth(t, `<div id="f"><span style="width: 10px"></span></div>`, ``)
	indented := floatWidth(t, `<div id="f"><span style="width: 10px"></span></div>`,
		`#f { text-indent: 30px }`)
	if plain != bgpx(10) {
		t.Fatalf("the unindented float is %v wide, want 10", plain)
	}
	if indented != bgpx(40) {
		t.Errorf("a float holding a 10px line indented by 30px is %v wide, want 40",
			indented)
	}
}

// TestTheIndentIsAddedToTheFirstLineAndNotToTheWidest.
//
// Two lines, and which of them is wider decides the answer. It is the case a
// one-line fixture cannot see, and the one the old arithmetic got wrong in both
// directions: it added the indent to whichever line was wider.
func TestTheIndentIsAddedToTheFirstLineAndNotToTheWidest(t *testing.T) {
	for _, tc := range []struct {
		what        string
		first, next float64
		want        float64
	}{
		// The indented first line wins: 10 + 30 against 20.
		{"a narrow first line", 10, 20, 40},
		// The second line wins on its own: 10 + 30 against 60.
		{"a wide second line", 10, 60, 60},
		// They are equal, which is the boundary between the two.
		{"lines that agree", 10, 40, 40},
	} {
		got := floatWidth(t,
			`<div id="f"><span style="width: `+lenOf(tc.first)+`px"></span><br/>`+
				`<span style="width: `+lenOf(tc.next)+`px"></span></div>`,
			`#f { text-indent: 30px }`)
		if got != bgpx(tc.want) {
			t.Errorf("%s (%v then %v, indented 30): the float is %v wide, want %v",
				tc.what, tc.first, tc.next, got, bgpx(tc.want))
		}
	}
}

// TestAHangingIndentNarrowsTheFirstLine is the same rule with the other sign,
// and the clamp is the part worth writing a case for: a line hung further out
// than it is long asks for no width at all, not a negative one.
func TestAHangingIndentNarrowsTheFirstLine(t *testing.T) {
	for _, tc := range []struct {
		what        string
		first, next float64
		indent      float64
		want        float64
	}{
		// One line, hung by less than it is wide.
		{"a hang shorter than the line", 50, 0, -30, 20},
		// One line, hung by more: nothing is left to ask for.
		{"a hang longer than the line", 10, 0, -30, 0},
		// Two lines: the second is untouched and decides.
		{"a second line that decides", 50, 10, -30, 20},
		{"a second line that is wider", 10, 30, -30, 30},
	} {
		markup := `<div id="f"><span style="width: ` + lenOf(tc.first) + `px"></span>`
		if tc.next > 0 {
			markup += `<br/><span style="width: ` + lenOf(tc.next) + `px"></span>`
		}
		markup += `</div>`
		got := floatWidth(t, markup, `#f { text-indent: `+lenOf(tc.indent)+`px }`)
		if got != bgpx(tc.want) {
			t.Errorf("%s (%v then %v, hung by %v): the float is %v wide, want %v",
				tc.what, tc.first, tc.next, tc.indent, got, bgpx(tc.want))
		}
	}
}

// TestAHangingIndentOnASingleLineThatMayStillBreak.
//
// One line and two pieces: the box is broken by an opportunity rather than by a
// forced break, so its preferred width is the whole line and its minimum is the
// wider piece. Hanging that line by more than its first piece is wide takes the
// preferred width under the minimum, and shrink-to-fit reads the two in the
// order min(max(minimum, available), preferred) — so the preferred width becomes
// a ceiling under a floor and the piece that still has to fit does not.
func TestAHangingIndentOnASingleLineThatMayStillBreak(t *testing.T) {
	// Pieces of 10 and 30 with a break opportunity between them, hung by 30.
	// The first line is 40 wide and hangs to 10; the wider piece is 30, and 30
	// is what the box needs.
	got := floatWidth(t,
		`<div id="f"><span style="width: 10px"></span>&#8203;`+
			`<span style="width: 30px"></span></div>`,
		`#f { text-indent: -30px }`)
	if got != bgpx(30) {
		t.Errorf("the float is %v wide, want 30 — the hang shortens the one line it "+
			"has, and the widest piece on it still has to fit", got)
	}
}

// TestABoxWithNoIndentIsUnchanged is the containment case, and it is nearly
// every box: the split measurement is computed for all of them and must change
// none of them.
func TestABoxWithNoIndentIsUnchanged(t *testing.T) {
	for _, tc := range []struct {
		what   string
		markup string
		want   float64
	}{
		{"one line", `<span style="width: 40px"></span>`, 40},
		{"two lines, the second wider",
			`<span style="width: 10px"></span><br/><span style="width: 40px"></span>`, 40},
		{"two lines, the first wider",
			`<span style="width: 40px"></span><br/><span style="width: 10px"></span>`, 40},
		{"a break opportunity rather than a forced break",
			`<span style="width: 10px"></span>&#8203;<span style="width: 40px"></span>`, 40},
	} {
		got := floatWidth(t, `<div id="f">`+tc.markup+`</div>`, ``)
		if got != bgpx(tc.want) {
			t.Errorf("%s: the float is %v wide, want %v", tc.what, got, bgpx(tc.want))
		}
	}
}

// TestTheIndentLandsOnTheFirstRunRatherThanTheFirstThingMeasured.
//
// An atomic inline offers a break opportunity before itself, so a box whose
// content begins with one ends a run that never began. Taking that empty run as
// the first line made the indent apply to nothing, and a float holding a single
// indented box came out the width of the indent alone.
func TestTheIndentLandsOnTheFirstRunRatherThanTheFirstThingMeasured(t *testing.T) {
	got := floatWidth(t, `<div id="f"><span style="width: 10px"></span></div>`,
		`#f { text-indent: 30px }`)
	if got == bgpx(30) {
		t.Fatalf("the float is exactly the indent wide; the box inside it was not " +
			"counted as the first line")
	}
	if got != bgpx(40) {
		t.Errorf("the float is %v wide, want 40 — 30px of indent and a 10px box", got)
	}
}

// TestAPercentageIndentContributesNothing. There is no containing block to take
// a percentage of while an intrinsic width is being measured, and CSS Sizing
// says such a percentage behaves as auto — which is a basis of zero.
func TestAPercentageIndentContributesNothing(t *testing.T) {
	got := floatWidth(t, `<div id="f"><span style="width: 40px"></span></div>`,
		`#f { text-indent: 50% }`)
	if got != bgpx(40) {
		t.Errorf("a float indented by 50%% is %v wide, want 40 — the percentage has "+
			"nothing to resolve against", got)
	}
}

// lenOf writes a whole number of pixels for a stylesheet.
func lenOf(v float64) string { return strconv.Itoa(int(v)) }
