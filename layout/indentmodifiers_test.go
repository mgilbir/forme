package layout

import (
	"fmt"
	"strings"
	"testing"
)

// css-text-3 §7.1's two modifiers on text-indent.
//
// The length says how far the indent goes; "each-line" and "hanging" say which
// lines it goes on, and neither changes the number. That is why reading them as
// part of the length was refused rather than guessed at — indenting the wrong
// lines by the right amount is not closer to right than not indenting at all.
//
//   - plain: the block's first line.
//   - each-line: the first line, and the line after every forced break.
//   - hanging: every line the plain form would have left alone.
//   - both: every line except the first and the ones after a forced break.

// indentStarts is where the text of each line of #p begins, in px.
func indentStarts(t *testing.T, indent, body string) []float64 {
	t.Helper()
	root := layoutOf(t, 600, `<div id="p">`+body+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 120px; text-indent: `+indent+` }`)
	var out []float64
	for _, ln := range find(t, root, "p").Lines {
		if len(ln.Runs) == 0 {
			out = append(out, -1)
			continue
		}
		out = append(out, ln.Runs[0].X.Px())
	}
	return out
}

func startsAre(got []float64) string {
	var parts []string
	for _, x := range got {
		parts = append(parts, fmt.Sprintf("%g", x))
	}
	return strings.Join(parts, ",")
}

// TestTheFourFormsOfTextIndent is §7.1's table, on one document that has both
// kinds of line beginning in it: a soft wrap and a forced break.
//
// Courier at 20px is 12px a character, so "aaaaa aaaaa" wraps in a 120px box
// and each half of the <br> makes two lines. The four values therefore differ
// on lines the others agree about, which is what makes one fixture enough.
func TestTheFourFormsOfTextIndent(t *testing.T) {
	const body = `aaaaa aaaaa<br>aaaaa aaaaa`
	for _, tc := range []struct {
		indent string
		want   string
	}{
		{"40px", "40,0,0,0"},
		{"40px hanging", "0,40,40,40"},
		{"40px each-line", "40,0,40,0"},
		{"40px each-line hanging", "0,40,0,40"},
		// The order of the modifiers is free, and either may be written before
		// the length.
		{"40px hanging each-line", "0,40,0,40"},
		{"hanging 40px", "0,40,40,40"},
		{"each-line hanging 40px", "0,40,0,40"},
	} {
		if got := startsAre(indentStarts(t, tc.indent, body)); got != tc.want {
			t.Errorf("%q starts its lines at %s, want %s", tc.indent, got, tc.want)
		}
	}
}

// TestHangingIsNotANegativeIndent. The two are written for the same effect and
// are not the same thing: a negative indent pulls the first line out of the box,
// a hanging one pushes every other line in. On a single-line block the negative
// form moves the text and the keyword does not.
func TestHangingIsNotANegativeIndent(t *testing.T) {
	if got := startsAre(indentStarts(t, "40px hanging", "aaa")); got != "0" {
		t.Errorf("a single line under \"40px hanging\" starts at %s, want 0; there "+
			"is no line for the indent to be on", got)
	}
	if got := startsAre(indentStarts(t, "-40px", "aaa")); got != "-40" {
		t.Errorf("a single line under \"-40px\" starts at %s, want -40", got)
	}
}

// TestEachLineFollowsAForcedBreakAndNotASoftOne is the distinction "each-line"
// is entirely about, and the one an implementation is most likely to lose: it
// counts the line after a break the author wrote, not the line after a break the
// breaker chose.
func TestEachLineFollowsAForcedBreakAndNotASoftOne(t *testing.T) {
	if got := startsAre(indentStarts(t, "40px each-line", "aaaaa aaaaa aaaaa")); got != "40,0,0" {
		t.Errorf("three soft-wrapped lines start at %s, want 40,0,0 — a soft wrap "+
			"does not begin a line \"each-line\" indents", got)
	}
	if got := startsAre(indentStarts(t, "40px each-line", "aaa<br>aaa<br>aaa")); got != "40,40,40" {
		t.Errorf("three forced lines start at %s, want 40,40,40", got)
	}
}

// TestAPlainIndentIsUnchanged is the containment argument: nearly every
// text-indent in every document is a bare length, and none of them may move.
func TestAPlainIndentIsUnchanged(t *testing.T) {
	if got := startsAre(indentStarts(t, "40px", "aaaaa aaaaa aaaaa")); got != "40,0,0" {
		t.Errorf("a plain indent starts its lines at %s, want 40,0,0", got)
	}
	if got := startsAre(indentStarts(t, "0", "aaaaa aaaaa")); got != "0,0" {
		t.Errorf("no indent starts its lines at %s, want 0,0", got)
	}
}

// TestAHangingIndentWidensAnIntrinsicWidth. A shrink-to-fit box has to be wide
// enough for the lines that are indented, and with "hanging" those are the ones
// after the first — the other half of the same split the plain form uses.
func TestAHangingIndentWidensAnIntrinsicWidth(t *testing.T) {
	// "aaa" is 36px and "bbbbb" is 60px. Under a plain 30px indent the box is
	// as wide as the greater of 36+30 and 60; under a hanging one, of 36 and
	// 60+30.
	width := func(indent string) float64 {
		root := layoutOf(t, 600, `<div><div id="f">aaa<br>bbbbb</div></div>`,
			`#f { float: left; font-family: Courier; font-size: 20px;
				text-indent: `+indent+` }`)
		return find(t, root, "f").BorderRect.W.Px()
	}
	if got := width("30px"); got != 66 {
		t.Errorf("a plain 30px indent sizes the float to %gpx, want 66 (36 + 30)", got)
	}
	if got := width("30px hanging"); got != 90 {
		t.Errorf("a hanging 30px indent sizes the float to %gpx, want 90 (60 + 30)", got)
	}
}

// TestASingleLineIsNotSizedForALineItDoesNotHave. The other half of the
// intrinsic rule, and the one that goes wrong quietly: with "hanging" there is
// nothing to indent in a one-line box, so a wide indent must not become the
// box's width.
func TestASingleLineIsNotSizedForALineItDoesNotHave(t *testing.T) {
	root := layoutOf(t, 600, `<div><div id="f">aaa</div></div>`,
		`#f { float: left; font-family: Courier; font-size: 20px;
			text-indent: 500px hanging }`)
	if got := find(t, root, "f").BorderRect.W.Px(); got != 36 {
		t.Errorf("the float is %gpx wide, want 36 — its one line is not indented, "+
			"so the indent asks for no room at all", got)
	}
}
