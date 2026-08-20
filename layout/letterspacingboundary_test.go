package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// letter-spacing at an element boundary, CSS Text §8.2.
//
// The property adds space after each typographic character unit, and the
// question is *whose* letter-spacing applies between two characters in
// different elements. The answer is the innermost element containing both.
//
// The suite states it in two rows of letter-spacing-203, the same six
// characters and the same two values in both:
//
//	<p class="ls1"><span class="ls0">AAA</span><span class="ls0">BBB</span></p>
//	    sets   AAA BBB      one gap, and it is the paragraph's
//	<p><span class="ls1">AAA</span><span class="ls1">BBB</span></p>
//	    sets   A A AB B B   spacing inside each span and none between them
//
// Opposite answers, and the only thing that differs is which element holds the
// boundary. Reading the spacing of the run *before* it gives the second row's
// answer for the first row and the first row's for the second.

// boxWidth is how wide a paragraph is when it is shrink-wrapped around its
// content, at Courier 20px where a character is 12px. It counts everything on
// the line — the runs and any inline box's own edges — which is what makes it
// the measurement to compare two markups by.
func boxWidth(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	f := find(t, layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: max-content }`+css), "p")
	return f.ContentRect().W
}

// TestTheSpacingAtABoundaryIsTheInnermostElementsThatHoldsBoth walks the two
// rows of letter-spacing-203 that make the rule visible, and a third that says
// the two answers are really different.
//
// Six characters at 12px is 72px of glyphs. What varies is the number of gaps:
// letter-spacing goes after every character, the last included, so a value on
// the paragraph gives six of them and a value on two spans of three gives three
// each — with the boundary between the spans belonging to the paragraph either
// way.
func TestTheSpacingAtABoundaryIsTheInnermostElementsThatHoldsBoth(t *testing.T) {
	const decls = `.ls0 { letter-spacing: 0 } .ls1 { letter-spacing: 24px }`
	const glyphs = 72.0
	for _, tc := range []struct {
		markup, block string
		gaps          float64
		what          string
	}{
		// Nothing anywhere: the baseline the rest are measured against.
		{`AAABBB`, `0`, 0, "no spacing at all"},

		// The paragraph has it and the spans have none. Five gaps inside the
		// spans are theirs and are nothing; the one at the boundary is the
		// paragraph's, because the paragraph is the innermost element holding
		// the characters either side of it.
		//
		// One gap and not two: the gap after the *last* character has no
		// character after it, so there is no boundary and no common ancestor to
		// ask. It belongs to the element the character is in, which is the
		// second span, and that span has none.
		{`<span class=ls0>AAA</span><span class=ls0>BBB</span>`, `24px`, 1,
			"the spacing on the block and none on the spans"},

		// The spans have it and the paragraph has none. Two gaps inside each
		// span are theirs; the boundary is the paragraph's and is nothing. The
		// gap after the last character has no run after it and stays the second
		// span's.
		{`<span class=ls1>AAA</span><span class=ls1>BBB</span>`, `0`, 5,
			"the spacing on the spans and none on the block"},

		// Both: every gap is 24px, which is the same six as the plain paragraph.
		{`<span class=ls1>AAA</span><span class=ls1>BBB</span>`, `24px`, 6,
			"the spacing on both"},
		{`AAABBB`, `24px`, 6, "the spacing on the block, with no spans at all"},
	} {
		got := boxWidth(t, tc.markup, `#p { letter-spacing: `+tc.block+` }`+decls)
		want := px2(glyphs + tc.gaps*24)
		if got != want {
			t.Errorf("%s: the box is %v, want %v — 72px of glyphs and %g gaps of 24",
				tc.what, got, want, tc.gaps)
		}
	}
}

// TestTheBoundaryReachesTheLineAndNotOnlyTheIntrinsicWidth.
//
// The rule is applied twice — once over the items an intrinsic width is
// measured from, once over the items a line is filled from — and a fixture that
// shrink-wraps its box exercises only the first. Both matter: the intrinsic
// width sizes the box and the line decides where the words go inside it.
func TestTheBoundaryReachesTheLineAndNotOnlyTheIntrinsicWidth(t *testing.T) {
	const decls = `.ls0 { letter-spacing: 0 } .ls1 { letter-spacing: 24px }`
	runWidth := func(markup, block string) style.Unit {
		t.Helper()
		// A fixed width, so the box is not sized from the intrinsic measurement
		// and what is summed is what the line filling produced.
		f := find(t, layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
			`#p { font-family: Courier; font-size: 20px; width: 400px;
			      letter-spacing: `+block+` }`+decls), "p")
		var w style.Unit
		for _, line := range f.Lines {
			for _, r := range line.Runs {
				w = w.Add(r.Width)
			}
		}
		return w
	}
	// Six characters at 12px, one gap of 24 at the boundary the paragraph owns,
	// and nothing else: the spans have no spacing of their own.
	got := runWidth(`<span class=ls0>AAA</span><span class=ls0>BBB</span>`, `24px`)
	if want := px2(72 + 24); got != want {
		t.Errorf("the runs are %v wide, want %v — 72px of glyphs and the one gap "+
			"between the spans, which is the paragraph's", got, want)
	}
	// And with the paragraph at nothing there is no gap at all, so this cannot
	// pass by measuring the same thing twice.
	if got := runWidth(`<span class=ls0>AAA</span><span class=ls0>BBB</span>`, `0`); got != px2(72) {
		t.Errorf("with no spacing anywhere the runs are %v wide, want 72px", got)
	}
}

// TestNestingDoesNotChangeWhoOwnsABoundary. The innermost element containing
// *both* characters is not the innermost element containing either, so wrapping
// one side in another span changes nothing about the gap between them.
func TestNestingDoesNotChangeWhoOwnsABoundary(t *testing.T) {
	const css = `.ls0 { letter-spacing: 0 } .ls1 { letter-spacing: 24px }
		#p { letter-spacing: 0 }`
	flat := boxWidth(t, `<span class=ls1>AAA</span><span class=ls1>BBB</span>`, css)
	nested := boxWidth(t,
		`<span class=ls0><span class=ls1>AAA</span></span><span class=ls1>BBB</span>`, css)
	if flat != nested {
		t.Errorf("nested %v against flat %v; the boundary between the two words is "+
			"the paragraph's either way", nested, flat)
	}
}

// TestTheLastCharacterKeepsItsOwnSpacing. There is no run after it to share a
// boundary with, so the spacing after it is its own — which is the engine's
// model and is what §4.1.2 leaves hanging at the end of a line.
func TestTheLastCharacterKeepsItsOwnSpacing(t *testing.T) {
	const css = `.ls1 { letter-spacing: 24px } #p { letter-spacing: 0 }`
	// Two characters at 12px, and two spacings of 24: one between them and one
	// after the last, which no run follows.
	if got, want := boxWidth(t, `<span class=ls1>AB</span>`, css), px2(24+48); got != want {
		t.Errorf("%v, want %v — 24px of glyphs and two gaps of 24", got, want)
	}
}

// TestABoundaryInsideOneElementIsThatElements is the containment case, and it is
// most of every document: text that is not split at all has one answer
// throughout and must be untouched.
func TestABoundaryInsideOneElementIsThatElements(t *testing.T) {
	for _, css := range []string{
		`#p { letter-spacing: 0 }`,
		`#p { letter-spacing: 24px }`,
		`#p { letter-spacing: -2px }`,
	} {
		one := boxWidth(t, "AAABBB", css)
		// The same text with a span around all of it: one element still holds
		// every boundary, so nothing moves.
		wrapped := boxWidth(t, "<span>AAABBB</span>", css)
		if one != wrapped {
			t.Errorf("%s: %v unwrapped and %v wrapped in a span that changes nothing",
				css, one, wrapped)
		}
	}
}

// TestABoxEdgeIsNotACharacter. An inline box's own margin, border and padding
// sit between the two runs and are not what the spacing goes in front of: the
// boundary is still between the two *characters*, and its owner is still the
// innermost element holding both.
func TestABoxEdgeIsNotACharacter(t *testing.T) {
	const css = `.ls1 { letter-spacing: 24px } #p { letter-spacing: 24px }
		.pad { padding: 0 10px }`
	plain := boxWidth(t, `<span>AAA</span><span>BBB</span>`, css)
	padded := boxWidth(t, `<span class=pad>AAA</span><span>BBB</span>`, css)
	// The padding adds twenty pixels and nothing else.
	if want := plain.Add(px2(20)); padded != want {
		t.Errorf("padded %v against %v, want %v: the padding adds its own width and "+
			"does not change whose spacing is at the boundary", padded, plain, want)
	}
	// And the same where the two sides disagree with the paragraph, which is
	// where stopping at the edge would show: the boundary is still the
	// paragraph's 24px, and the padding is still 20px more.
	const split = `.ls0 { letter-spacing: 0 } .ls1 { letter-spacing: 24px }
		#p { letter-spacing: 24px } .pad { padding: 0 10px }`
	bare := boxWidth(t, `<span class=ls0>AAA</span><span class=ls0>BBB</span>`, split)
	withPad := boxWidth(t, `<span class="ls0 pad">AAA</span><span class=ls0>BBB</span>`, split)
	if want := px2(72 + 24); bare != want {
		t.Errorf("without the padding the box is %v, want %v — the boundary is the "+
			"paragraph's even though neither span has any spacing", bare, want)
	}
	if want := bare.Add(px2(20)); withPad != want {
		t.Errorf("with a padding between them the box is %v, want %v: an inline "+
			"box's own edge is not a character and is not what the spacing goes "+
			"in front of", withPad, want)
	}
}

// px2 is style.FromPx without the second result.
func px2(v float64) style.Unit { u, _ := style.FromPx(v); return u }

// TestTheTrailingLetterSpacingHangs.
//
// §8.2 adds spacing after the last character too, and that last one hangs at
// the end of the line: the line's measure ends at its last glyph rather than a
// tracking width past it. Counting it wrapped a paragraph set with wide tracking
// one word early on every line — the wider the tracking, the earlier — and
// contradicted what this engine already said about itself in
// TestLetterSpacingIsStillAddedAfterEveryLetter.
//
// No reftest moves either way; see the comment on paragraph's overflows for why
// the three that are written for this cannot. So the arithmetic is what has to
// be pinned, and it is pinned exactly: "AAA BBB" is seven characters at 12px
// and seven gaps of 24px, which is 252 with the trailing gap and 228 without.
func TestTheTrailingLetterSpacingHangs(t *testing.T) {
	lines := func(width int) int {
		t.Helper()
		css := `#p { font-family: Courier; font-size: 20px; letter-spacing: 24px;
			width: ` + itoaPx(width) + ` }`
		return len(find(t, layoutOf(t, 4000, `<div id="p">AAA BBB</div>`, css), "p").Lines)
	}
	// The whole thing, trailing gap included, obviously fits.
	if got := lines(252); got != 1 {
		t.Errorf("at 252px: %d lines, want 1", got)
	}
	// And so does the text without it, which is the claim.
	if got := lines(228); got != 1 {
		t.Errorf("at 228px: %d lines, want 1 — the gap after the last character "+
			"hangs past the end of the line and is not part of what has to fit", got)
	}
	// One unit narrower and it does not, so this is a boundary rather than a
	// fixture that would fit at any width.
	if got := lines(227); got != 2 {
		t.Errorf("at 227px: %d lines, want 2 — 228px of glyphs and internal gaps "+
			"is one pixel more than the room", got)
	}
}

// TestARightAlignedLineEndsAtItsLastGlyph is the other half of the hang, and it
// is what a reader sees rather than what the breaking computes.
//
// A line is placed by what it measures. If the trailing tracking is counted, a
// right-aligned line of tracked text sits a whole tracking width in from the
// edge and a centred one half of it — visibly, and the wider the tracking the
// more so.
func TestARightAlignedLineEndsAtItsLastGlyph(t *testing.T) {
	// Two characters at 12px is 24px of glyphs, two gaps of 24px is 48. The
	// second gap hangs, so the line measures 24+24 = 48 and sits 48 from the
	// right edge of a 400px box.
	f := find(t, layoutOf(t, 4000, `<div id="p">AB</div>`,
		`#p { font-family: Courier; font-size: 20px; letter-spacing: 24px;
		      width: 400px; text-align: right }`), "p")
	if len(f.Lines) != 1 || len(f.Lines[0].Runs) == 0 {
		t.Fatalf("%d lines", len(f.Lines))
	}
	if got, want := f.Lines[0].Runs[0].X, px2(400-48); got != want {
		t.Errorf("the line begins at %v, want %v: the gap after the last character "+
			"hangs past the right edge and is not what the line is placed by",
			got, want)
	}
	// The control: without the property the line is its glyphs alone.
	plain := find(t, layoutOf(t, 4000, `<div id="p">AB</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 400px; text-align: right }`), "p")
	if got, want := plain.Lines[0].Runs[0].X, px2(400-24); got != want {
		t.Errorf("without letter-spacing the line begins at %v, want %v", got, want)
	}
}

// itoaPx renders a whole number of CSS pixels.
func itoaPx(v int) string {
	digits := ""
	for n := v; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	if digits == "" {
		digits = "0"
	}
	return digits + "px"
}
