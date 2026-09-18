package layout

import (
	"strings"
	"testing"
)

// TestALineSeparatorIsOneBidiParagraph is the layout-level half of the rule, on
// the shape CSS2's bidi-breaking-003 is built from.
//
// The same text either side of a separator, with a Hebrew letter at each far
// end and neutrals between them. Across U+2029 the two halves are separate bidi
// paragraphs, so each neutral run sits at a paragraph edge and takes the block's
// own left-to-right direction. Across U+2028 it is one paragraph, the neutrals
// are between two right-to-left characters, and UAX #9's N1 gives them that
// direction — so every run on both lines reads right to left and the order
// reverses.
//
// The assertion is the direction of the runs rather than their positions,
// because that is the rule: N1 is about what the neutrals resolve to.
func TestALineSeparatorIsOneBidiParagraph(t *testing.T) {
	const decl = `#p { font-family: Courier; font-size: 16px; white-space: pre }`
	for _, tc := range []struct {
		sep     string
		wantRTL bool
		what    string
	}{
		{"\u2029", false, "U+2029 PARAGRAPH SEPARATOR ends the paragraph, so the neutrals take the block's direction"},
		{"\n", false, "a preserved newline ends it too"},
		{"\u2028", true, "U+2028 LINE SEPARATOR does not, so the neutrals sit between two right-to-left letters"},
	} {
		src := "\u05d0 + \u00a0" + tc.sep + "\u00a0 + \u05ea"
		f := find(t, layoutOf(t, 4000, `<div id="p">`+src+`</div>`, decl), "p")
		if len(f.Lines) != 2 {
			t.Fatalf("%s: laid out as %d lines, want 2", tc.what, len(f.Lines))
		}
		for n, line := range f.Lines {
			for _, r := range line.Runs {
				if r.Text == "+" && r.RTL != tc.wantRTL {
					t.Errorf("%s: the neutral on line %d reads rtl=%v, want %v",
						tc.what, n, r.RTL, tc.wantRTL)
				}
			}
		}
	}
}

// TestASeparatorPutsNoGlyphOnThePage is the other half, and the one the reftest
// counts: the two separators are consumed by the break they make, so nothing of
// them reaches the page.
//
// The assertion is on the text that is *drawn*, not on the number of runs. A
// separator kept in the text merges into the run beside it rather than making
// one of its own, so counting runs cannot see it — which a plant showed, by
// passing.
func TestASeparatorPutsNoGlyphOnThePage(t *testing.T) {
	const decl = `#p { font-family: Courier; font-size: 16px; white-space: pre }`
	drawn := func(markup string) string {
		f := find(t, layoutOf(t, 4000, `<div id="p">`+markup+`</div>`, decl), "p")
		var b strings.Builder
		for _, line := range f.Lines {
			for _, r := range line.Runs {
				b.WriteString(r.Text)
			}
		}
		return b.String()
	}
	for _, tc := range []struct{ sep, what string }{
		{"\u2028", "U+2028 LINE SEPARATOR, category Zl"},
		{"\u2029", "U+2029 PARAGRAPH SEPARATOR, category Zp"},
	} {
		got := drawn("ab" + tc.sep + "cd")
		if strings.ContainsRune(got, []rune(tc.sep)[0]) {
			t.Errorf("%s reached the page: the text drawn is %q, and a separator "+
				"is consumed by the break it makes", tc.what, got)
		}
		if want := drawn("ab\ncd"); got != want {
			t.Errorf("%s drew %q where a newline draws %q", tc.what, got, want)
		}
	}
	// And a control character is still set, which is the containment half: three
	// of the suite's control-chars documents are mismatch references against a
	// blank page and say the character must be visible.
	for _, tc := range []struct{ sep, what string }{
		{"\u000b", "U+000B LINE TABULATION"},
		{"\u000c", "U+000C FORM FEED"},
	} {
		got := drawn("ab" + tc.sep + "cd")
		if !strings.ContainsRune(got, []rune(tc.sep)[0]) {
			t.Errorf("%s did not reach the page: the text drawn is %q, and a "+
				"control character is rendered as a visible glyph", tc.what, got)
		}
	}
}
