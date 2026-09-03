package paragraph

import (
	"testing"

	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// CSS Text §8.2's cursive tracking.
//
//	Letter spacing must not be applied between the characters of a cursive
//	script, as it would break the cursive connection.
//
// The rule is about the *script* and not about the pair, and the suite states it
// that way: letter-spacing-cursive-001 asks for Arabic set with and without a
// letter-spacing to render identically, and "مرحباً" has an unjoined pair in the
// middle of it — a reh joins only to its right. An implementation that asked
// "do these two letters connect" would space that gap and fail the test it was
// written from.

var em, _ = style.FromPx(10)

// TestSpacingIsNotAddedInsideACursiveScript is the rule.
func TestSpacingIsNotAddedInsideACursiveScript(t *testing.T) {
	for _, tc := range []struct {
		text string
		what string
	}{
		{"مرحبا", "an Arabic word"},
		{"مرحباً", "an Arabic word with a tanwin on its last letter"},
		{"الإبداع", "an Arabic word with a hamza"},
		{"ء", "a hamza alone, which is Arabic and joins to nothing"},
		{"ܫܠܡܐ", "Syriac"},
		{"ߛߊߟߌ", "N'Ko"},
	} {
		if got := SpacedUnits(tc.text); got != 0 {
			t.Errorf("%s (%q) counts %d spaced unit(s); §8.2 adds none inside a "+
				"cursive script", tc.what, tc.text, got)
		}
	}
}

// TestSpacingIsStillAddedBesideACursiveScript is the half cursive-002 measures.
//
// Two Arabic words and a space between them under "letter-spacing: 1em" get one
// em and not two: the rule is on the character the spacing would go *after*, so
// the last letter of the first word takes none and the space takes one. The
// reference beside that test inserts exactly one inline-block an em wide.
func TestSpacingIsStillAddedBesideACursiveScript(t *testing.T) {
	if got := SpacedUnits("الإبداع المتجدد"); got != 1 {
		t.Errorf("%d spaced units in two Arabic words and a space, want 1 — the "+
			"space, and neither word", got)
	}
	// Latin beside Arabic: the Latin counts and the Arabic does not.
	if got := SpacedUnits("abمرحبا"); got != 2 {
		t.Errorf("%d spaced units in \"ab\" before an Arabic word, want 2", got)
	}
}

// TestAMarkTakesTheTrackingOfWhatItSitsOn.
//
// Unicode's joining table names the letters of the cursive scripts and not the
// marks written on them, so a fathatan reads as an ordinary character and would
// collect a spacing of its own — which is exactly what left "مرحباً" ten pixels
// wider than the same word without a letter-spacing.
//
// It takes it only from a *cursive* base, and the Latin row below is what says
// so: a letter and its accent are one unit there because a cluster is one unit,
// not because the mark was suppressed. The two rules meet on the same text and
// give it the same count for different reasons, so each is asserted with a
// number the other cannot produce — zero for the cursive pair, one for the
// Latin one.
func TestAMarkTakesTheTrackingOfWhatItSitsOn(t *testing.T) {
	if got := SpacedUnits("مً"); got != 0 {
		t.Errorf("an Arabic letter and a fathatan count %d units, want 0", got)
	}
	if got := SpacedUnits("e\u0301"); got != 1 {
		t.Errorf("a Latin letter and a combining acute count %d units, want 1 — "+
			"one grapheme cluster is one typographic character unit", got)
	}
	// A mark with nothing before it has no base to take an answer from, and is
	// counted as it always was.
	if got := SpacedUnits("ً"); got != 1 {
		t.Errorf("a lone mark counts %d units, want 1", got)
	}
}

// TestTheCutMatchesTheCount is the invariant the display list depends on: a run
// carries one letter-spacing, so every piece of a cut run has to be all-spaced
// or all-suppressed.
func TestTheCutMatchesTheCount(t *testing.T) {
	for _, text := range []string{
		"abمرحبا", "مرحباab", "aمb", "abc", "مرحبا", "áمًb",
		"الإبداع المتجدد", "", "a",
	} {
		parts := SplitAtCursiveTracking(text)
		if parts == nil {
			// Nothing to cut: the whole run is one answer or the other.
			n := SpacedUnits(text)
			runes := countSpacedRunes(text)
			if n != 0 && n != runes {
				t.Errorf("%q was not cut and counts %d of %d units", text, n, runes)
			}
			continue
		}
		var joined string
		for _, p := range parts {
			joined += p
			n, runes := SpacedUnits(p), countSpacedRunes(p)
			if n != 0 && n != runes {
				t.Errorf("%q cut out of %q counts %d of %d units; a piece has to be "+
					"all spaced or all suppressed", p, text, n, runes)
			}
		}
		if joined != text {
			t.Errorf("the pieces of %q join to %q", text, joined)
		}
	}
}

// countSpacedRunes is how many characters SpacedUnits would count if nothing
// suppressed any of them, which is what "all spaced" means.
func countSpacedRunes(text string) int {
	// Units and not runes, which is what SpacedUnits counts: the invariant
	// above is about whether a piece is all-spaced or all-suppressed, and
	// comparing a count of clusters with a count of code points would fail on
	// every accented letter for a reason that has nothing to do with cursive
	// tracking. What is wanted is the same count with the cursive rule taken
	// out, which is the cluster count of the characters that are drawn.
	n := 0
	for i, start := 0, 0; start < len(text); i++ {
		end := len(text)
		if bounds := segment.Boundaries(nil, text); i < len(bounds) {
			end = bounds[i]
		}
		drawn := false
		for _, r := range text[start:end] {
			if !IsDefaultIgnorable(r) {
				drawn = true
			}
		}
		if drawn {
			n++
		}
		start = end
	}
	return n
}

// TestTheAdvanceFollowsTheCount, so that nothing between the rule and the width
// can quietly disagree with it.
func TestTheAdvanceFollowsTheCount(t *testing.T) {
	sp := TextSpacing{Letter: em}
	if got := SpacingAdvance("مرحبا", sp); got != 0 {
		t.Errorf("an Arabic word takes %v of letter-spacing, want none", got)
	}
	if got, want := SpacingAdvance("abc", sp), em.Mul(3); got != want {
		t.Errorf("\"abc\" takes %v of letter-spacing, want %v", got, want)
	}
}

// TestALineEndDiscountsOnlyTheSpacingTheRunActuallyHas.
//
// §8.2 does not apply letter-spacing at the end of a line, so the fill leaves
// the last item's trailing spacing out of the measure — and a run of a cursive
// script has none to leave out. Discounting one it never had makes the run a
// spacing narrower than it is, which keeps a word on a line it does not fit on.
//
// It asks overflows directly. The arithmetic is three terms wide and every
// route to it through a document also goes through shaping, a face and a
// fallback stack, so a fixture that laid one out would be evidence about all
// four.
func TestALineEndDiscountsOnlyTheSpacingTheRunActuallyHas(t *testing.T) {
	sp := TextSpacing{Letter: em}
	width := em.Mul(3)
	// Both runs are exactly the width of the room, so what decides each is the
	// discount alone.
	latin := Item{Text: "abc", Width: em.Mul(4), Spacing: sp}
	arabic := Item{Text: "مرحبا", Width: em.Mul(4), Spacing: sp}

	if overflows(0, latin, width) {
		t.Errorf("a Latin run of %v overflowed %v; the spacing after its last "+
			"character is not applied at the end of a line", latin.Width, width)
	}
	if !overflows(0, arabic, width) {
		t.Errorf("a cursive run of %v fitted %v; §8.2 gave it no spacing after its "+
			"last character, so there is none to leave out", arabic.Width, width)
	}
	// And an item that is not a run of text keeps the declared value, since an
	// atomic inline is a character unit letter-spacing goes after.
	atomic := Item{Width: em.Mul(4), Spacing: sp}
	if overflows(0, atomic, width) {
		t.Errorf("an atomic inline of %v overflowed %v", atomic.Width, width)
	}
}
