package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The spaces whose width is the whole of what they are.
//
// A face that cannot encode a character puts a space there instead, and for most
// of the space separators that substitution is harmless — a no-break space *is*
// a space, differing only in whether a line may break at it, which is settled
// long before a face is asked. So the engine skipped every space separator when
// deciding whether a face was missing anything, and never looked for another
// face on their account.
//
// That is wrong for the fixed-width ones. An ideographic space is one em; the
// fourteen standard PDF faces have no glyph for it and give it their ordinary
// space's quarter em. "ああ　" then came out four pixels short of the "あああ"
// it has to cover, which is trailing-ideographic-space-001 exactly.

// spaceWidth returns how wide one character came out.
//
// It is measured through a background rather than a fragment: an inline box
// lives in a line box and has no fragment of its own, and its background is
// painted at exactly its content width, which is the number under test.
func spaceWidth(t *testing.T, set FontSet, ch string) style.Unit {
	t.Helper()
	built := Build(Input{
		HTML: `<div id="d">x<span id="s">` + ch + `</span>x</div>`,
		CSS: []Stylesheet{{Source: `#d { font-size: 16px; white-space: pre }
			#s { background: rgb(0,0,255) }`}},
	})
	if built.Root == nil {
		t.Fatal("no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	got := fillsOf(Paint(frag), blue)
	if len(got) != 1 {
		t.Fatalf("%d fills for %q, want the span's background: %v", len(got), ch, got)
	}
	return got[0].W
}

// TestAFixedWidthSpaceTakesItsOwnWidth is the fix, and the fixture is the one
// the suite writes: an ideographic space beside the ideographs it has to line up
// with.
func TestAFixedWidthSpaceTakesItsOwnWidth(t *testing.T) {
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face that has them")
	}
	set := suiteFonts{standard: StandardFonts(), fallback: faces}

	// One em at 16px. The standard serif face has no glyph for it and would
	// give it a quarter em, which is the four pixels this is about.
	if got, want := spaceWidth(t, set, "　"), bgpx(16); got != want {
		t.Errorf("an ideographic space is %v wide, want %v — one em", got, want)
	}
	// A half em, and a face that has it says so.
	if got, want := spaceWidth(t, set, " "), bgpx(8); got != want {
		t.Errorf("an en space is %v wide, want %v — half an em", got, want)
	}
	if got, want := spaceWidth(t, set, " "), bgpx(16); got != want {
		t.Errorf("an em space is %v wide, want %v", got, want)
	}
}

// TestANoBreakSpaceIsStillLeftAlone is the other half, and the one this change
// most needed to keep.
//
// A no-break space is a space of the same width, so a face that lacks it loses
// nothing by substituting one — and reporting it, or going looking for another
// face on its account, was the most common thing this engine did: 154 documents
// in the suite raised a finding for U+00A0 alone.
func TestANoBreakSpaceIsStillLeftAlone(t *testing.T) {
	if !substitutesExactly(' ') {
		t.Errorf("a no-break space is no longer treated as substitutable by a " +
			"space; the finding it silences fired on 154 of the suite's documents")
	}
	for _, r := range []rune{0x3000, 0x2002, 0x2003, 0x2009, 0x205F, 0x1680} {
		if substitutesExactly(r) {
			t.Errorf("U+%04X is treated as substitutable by a space, and its width "+
				"is the whole of what it is for", r)
		}
	}
}

// TestASpacePieceTakesAFaceWithoutBeingSplit.
//
// A space piece is asked which face to use and is never cut into several. There
// is nothing to cut — a space piece is one character, or a run of the same
// character — and splitting one would share the flags that say whether it hangs,
// whether it is trimmed and whether a line may break after it between two items
// that cannot both own them.
func TestASpacePieceTakesAFaceWithoutBeingSplit(t *testing.T) {
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS to read a face that has them")
	}
	set := suiteFonts{standard: StandardFonts(), fallback: faces}
	// Three ideographic spaces preserved: three ems, in one run.
	if got, want := spaceWidth(t, set, "\u3000\u3000\u3000"), bgpx(48); got != want {
		t.Errorf("three ideographic spaces are %v wide, want %v", got, want)
	}
}
