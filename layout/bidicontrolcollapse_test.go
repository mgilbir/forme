package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// A bidi control does not break a run of white space in two, wherever it is
// written.
//
// CSS Text §4.1.1 collapses a run of white space to one space across an inline
// boundary as readily as within one, and a bidi control is not a character of
// the text — it is an instruction to the bidirectional algorithm. So "a ‮ b"
// has one space in it and not two, and CollapseWhitespaceAfter says exactly
// that, citing bidi-003: that document writes one boundary as a control and the
// same boundary as "</span><span>" and asks for the two to render identically.
//
// The run that crosses a box boundary is collapsed somewhere else — flatten.go's
// piece loop, off AfterCollapsibleSpace — and it did not know the rule. A
// control in a box of its own is the two spellings bidi-003 compares met in the
// middle, and it agreed with neither: the space survived.
//
// Found by FuzzRunTiling, as a width. The two spellings are the same text, so
// they are the same width, and " ‮ ‮" cut after the first space was zero wide
// whole and a space wide cut.
func TestABidiControlInABoxOfItsOwnDoesNotSeparateTwoSpaces(t *testing.T) {
	const lro = "‭"
	for _, tc := range []struct {
		what, whole, cut string
	}{
		{"a control between two spaces", "a " + lro + " b",
			`a <span>` + lro + `</span> b`},
		{"two of them", "a " + lro + lro + " b",
			`a <span>` + lro + `</span><span>` + lro + `</span> b`},
		{"the fuzzer's own", " " + lro + " " + lro,
			`<span> </span><span>` + lro + " " + lro + `</span>`},
	} {
		whole := widthOfMarkup(t, tc.whole)
		cut := widthOfMarkup(t, tc.cut)
		if whole != cut {
			t.Errorf("%s: %q is %v wide written plainly and %v in spans; a bidi "+
				"control is not a character of the text and may not keep two "+
				"spaces from collapsing", tc.what, tc.whole, whole, cut)
		}
	}
}

// TestADefaultIgnorableThatIsNotAControlStillSeparatesTwoSpaces is the
// containment case, and it is asserted against the *text* rather than against
// another spelling of it.
//
// That is the whole reason it is written this way. Widening isBidiControlOnly to
// isDefaultIgnorable is the obvious over-reach, and it moves both spellings
// together — "a &#xFE0F; b" and "a <span>&#xFE0F;</span> b" both collapse to one
// space — so a test comparing the two agrees with itself and says nothing. A
// planted widening passed every test in this package until this one was written
// against the width.
//
// What it would break is layout agreeing with Phase I.
// CollapseWhitespaceAfter makes the bidi controls transparent and nothing else,
// so it has already kept two spaces around a variation selector by the time the
// piece loop sees them. A variation selector, a soft hyphen and a joiner are
// characters of the text — they belong to what is beside them — and a zero width
// space is the case the suite states outright, "U+00A0 is exactly equivalent to
// U+200B U+0020 U+200B", four times over.
func TestADefaultIgnorableThatIsNotAControlStillSeparatesTwoSpaces(t *testing.T) {
	one := widthOfMarkup(t, "a b")
	for _, tc := range []struct{ what, char string }{
		{"a variation selector", "\uFE0F"},
		{"a soft hyphen", "\u00AD"},
		{"a zero width joiner", "\u200D"},
		{"a zero width space", "\u200B"},
	} {
		for _, markup := range []string{
			"a " + tc.char + " b",
			`a <span>` + tc.char + `</span> b`,
		} {
			got := widthOfMarkup(t, markup)
			if got <= one {
				t.Errorf("%s: %q is %v wide and \"a b\" is %v; it is a character "+
					"of the text and not an instruction to the bidirectional "+
					"algorithm, so the two spaces it stands between do not "+
					"collapse and it must be the wider of the two",
					tc.what, markup, got, one)
			}
		}
	}
}

// TestAPieceThatIsNotOnlyControlsStillEndsTheRun is the other containment case:
// the rule is about a piece that is controls and *nothing else*.
//
// SplitAtBreaks gathers a control with the text beside it — "a &#x202D;b c"
// comes out as "a", " ", "\u202Db", " ", "c" — so a piece holding a control is
// usually a piece holding a letter too, and that one ends the run of white space
// like any other text. Asking whether a piece contains *any* control instead of
// whether it is *all* controls swallows the space after it.
//
// Like the case above this is asserted against the text, because that plant also
// moves both spellings together: written plainly and written with a span, both
// lost the same space. A bidi control is drawn nowhere and collapses nothing
// here, so the text sets the width it would set without one — exactly, since the
// pieces are cut at the spaces either way.
func TestAPieceThatIsNotOnlyControlsStillEndsTheRun(t *testing.T) {
	const lro = "\u202D"
	for _, tc := range []struct{ what, with, without string }{
		{"written plainly", "a " + lro + "b c", "a b c"},
		{"the space in a span of its own",
			`a ` + lro + `b<span> c</span>`, `a b<span> c</span>`},
	} {
		got, want := widthOfMarkup(t, tc.with), widthOfMarkup(t, tc.without)
		if got != want {
			t.Errorf("%s: %q is %v wide and %q is %v; a bidi control draws "+
				"nothing and collapses nothing, so the two are the same width "+
				"and the difference is a space that should not have gone",
				tc.what, tc.with, got, tc.without, want)
		}
	}
}

// widthOfMarkup is the summed width of the runs on the one line the markup sets,
// in the bundled Noto Sans under nowrap.
func widthOfMarkup(t *testing.T, markup string) style.Unit {
	t.Helper()
	return widthOfMarkupIn(t, "T", markup)
}

// widthOfMarkupIn is widthOfMarkup in a named family. "T" is the bundled Noto
// Sans and "Courier" one of the standard fourteen, which is the pair
// FuzzRunTiling holds every case in: Courier joins, kerns and ligates in none of
// the ways Noto Sans does, so a merge group that goes wrong there goes unnoticed
// in the other.
func widthOfMarkupIn(t *testing.T, family, markup string) style.Unit {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	w, ok := tiledWidth(t, set, markup,
		`#d { font-family: `+family+`; font-size: 16px; white-space: nowrap }`)
	if !ok {
		t.Fatalf("%q did not set exactly one line", markup)
	}
	return w
}
