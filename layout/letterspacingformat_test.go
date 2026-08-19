package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// letter-spacing and the characters nothing is drawn for, CSS Text §8.2.
//
// The spacing goes after each typographic character unit, and a character
// nothing is drawn for is not one — so "let<zwsp>ter" with four pixels of
// letter-spacing has to be exactly as wide as "letter". The suite writes it out
// in letter-spacing-control-chars-001: two paragraphs, one salted with zero
// width format characters and one not, and they must match.
//
// Two separate faults made them differ, and the second is the worse of the two
// because it is not about letter-spacing at all.

// spacedWidth is the width of one line of text under the given declarations.
func spacedWidth(t *testing.T, text, decl string) style.Unit {
	t.Helper()
	f := find(t, layoutOf(t, 4000, `<div id="p">`+text+`</div>`,
		`#p { font-family: Courier; font-size: 12px; `+decl+` }`), "p")
	if len(f.Lines) != 1 {
		t.Fatalf("%q laid out as %d lines", text, len(f.Lines))
	}
	var w style.Unit
	for _, r := range f.Lines[0].Runs {
		w = w.Add(r.Width)
	}
	return w
}

// TestNoLetterSpacingAfterACharacterNothingIsDrawnFor.
func TestNoLetterSpacingAfterACharacterNothingIsDrawnFor(t *testing.T) {
	plain := spacedWidth(t, "letter", "letter-spacing: 4px")
	for _, tc := range []struct{ text, what string }{
		{"le\u200Btter", "a zero width space"},
		{"le\u200Ctter", "a zero width non-joiner"},
		{"le\u200Dtter", "a zero width joiner"},
		{"le\u00ADtter", "a soft hyphen"},
		{"le\uFEFFtter", "a byte order mark"},
		{"le\u2060tter", "a word joiner"},
		{"le\u206Atter", "a deprecated format control"},
		{"le\u061Ctter", "the Arabic letter mark"},
		{"\u200Ble\u200Btte\u200Br\u200B", "several of them, including at the edges"},
	} {
		if got := spacedWidth(t, tc.text, "letter-spacing: 4px"); got != plain {
			t.Errorf("%s: %v against %v for the same word without it", tc.what, got, plain)
		}
	}
}

// TestLetterSpacingIsStillAddedAfterEveryLetter is the containment case, and it
// has to be here: every assertion above is an equality between two widths, and
// they would all hold if letter-spacing did nothing whatever.
func TestLetterSpacingIsStillAddedAfterEveryLetter(t *testing.T) {
	plain := spacedWidth(t, "letter", "")
	spaced := spacedWidth(t, "letter", "letter-spacing: 4px")
	// Six characters, six spacings — CSS Text adds one after the last character
	// too, and it is the trailing one that hangs at the end of a line.
	want, _ := style.FromPx(24)
	if got := spaced.Sub(plain); got != want {
		t.Errorf("letter-spacing added %v to a six-character word at 4px each, want %v",
			got, want)
	}
}

// TestAJoinerIsNotDrawnAsASpace is the second fault, stated on its own because
// it is not about letter-spacing and shows up wherever a standard PDF face is
// used.
//
// A join control is kept through shaping so the cursive joining and the
// syllable models can read it, and taken back out before anything is
// positioned. A simple face — one code per character, no shaping at all — has
// no pass in which to take it out again, so it fell through to the substitution
// an unmapped character gets, which is a space. A document spelling a word with
// a non-joiner came out with a gap in the middle of it.
func TestAJoinerIsNotDrawnAsASpace(t *testing.T) {
	plain := spacedWidth(t, "letter", "")
	for _, tc := range []struct{ text, what string }{
		{"le\u200Ctter", "a non-joiner"},
		{"le\u200Dtter", "a joiner"},
	} {
		if got := spacedWidth(t, tc.text, ""); got != plain {
			t.Errorf("%s in a standard face measured %v against %v, with no "+
				"letter-spacing involved at all", tc.what, got, plain)
		}
	}
}
