package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Which two characters a letter-spacing gap falls between, CSS Text §8.2.
//
// "Letter spacing is inserted after bidi reordering", so the pair a gap belongs
// to is the pair that is actually next to each other on the page. §8.2's
// boundary rule then asks the innermost element containing *those* two, which
// for text that reads in one direction is the pair the markup put together and
// for text that changes direction is not.
//
// The suite states it twice, in two families that give different answers to the
// obvious first reading and the same answer to this one. See gapNeighbour, where
// both are worked through.

// lastRunAt is where the visually last run of a line begins.
//
// It counts every gap *between* the runs and no others: the gap a run carries
// sits at its right edge, so the one the last run carries is past the point
// measured here and the ones before it are all inside it. That is exactly the
// set the boundary rule is about — a gap at the end of a line hangs and is not a
// gap between two characters at all.
//
// The block is given a width so that nothing is shrink-wrapped: what is under
// test is where the content came out, and a line box is as wide as it was told
// to be.
func lastRunAt(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	root := layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 3000px }`+css)
	runs := runsOf(t, root, "p")
	if len(runs) < 2 {
		t.Fatalf("%q produced %d runs; this measures the gaps between them",
			markup, len(runs))
	}
	at := runs[0].X
	for _, r := range runs[1:] {
		if r.X > at {
			at = r.X
		}
	}
	return at
}

// lineSpan is everything the runs of a line occupy, the gap the last of them
// carries included. It is what a box shrink-wrapped to the line has to be.
func lineSpan(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	root := layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 3000px }`+css)
	runs := runsOf(t, root, "p")
	if len(runs) == 0 {
		t.Fatalf("%q produced no runs", markup)
	}
	lo, hi := runs[0].X, runs[0].X.Add(runs[0].Width)
	for _, r := range runs[1:] {
		if r.X < lo {
			lo = r.X
		}
		if end := r.X.Add(r.Width); end > hi {
			hi = end
		}
	}
	return hi.Sub(lo)
}

// spacedDecls is a letter-spacing of two characters' width, so that a gap is
// impossible to confuse with a glyph: Courier at 20px is 12px a character.
const spacedDecls = ` .ls { letter-spacing: 24px }`

// TestAGapNeedsThePairToBeNextToEachOther is the rule, over the suite's own
// fixture.
//
// letter-spacing-bidi-001 is "a<span>bא</span>ב" with the letter-spacing on the
// span alone, and its assert is that "letter spacing cannot apply to any of the
// letters in the span, since they get split apart". The line reads a, b, ב, א
// once the Hebrew is reordered, so the pairs that are actually adjacent are
// (a,b), (b,ב) and (ב,א) — and every one of them has the *div* as its innermost
// common ancestor, which sets no letter-spacing. So there is no gap anywhere,
// and the line is exactly as wide as the same four characters with the
// declaration removed.
//
// Reading the logically next run instead puts a gap between b and א, because
// those two are the pair the span holds — and the line comes out a gap wider
// than four characters.
func TestAGapNeedsThePairToBeNextToEachOther(t *testing.T) {
	const markup = `a<span class="ls">b&#x5d0;</span>&#x5d1;`
	spaced := lastRunAt(t, markup, spacedDecls)
	plain := lastRunAt(t, markup, ``)
	if spaced != plain {
		t.Errorf("the last character starts at %gpx with the span's "+
			"letter-spacing and %gpx without it; the span's two letters are not "+
			"next to each other once the line is reordered, so nothing "+
			"separates them", spaced.Px(), plain.Px())
	}
}

// TestTheIntrinsicWidthAgreesWithTheLine, which is the rule's other half and the
// one an intrinsic measurement may never get wrong.
//
// A box shrink-wrapped to its content has to be as wide as the content will be.
// The measuring pass resolves the same bidi paragraph the fill does for exactly
// this reason: without it the run "bא" is one run while measuring and two while
// filling, no boundary rule can make those agree, and the box comes out a gap
// wider than the line inside it.
func TestTheIntrinsicWidthAgreesWithTheLine(t *testing.T) {
	const markup = `a<span class="ls">b&#x5d0;</span>&#x5d1;`
	line := lineSpan(t, markup, spacedDecls)
	box := boxWidth(t, markup, spacedDecls)
	if box != line {
		t.Errorf("max-content is %gpx and the line it holds is %gpx; a box "+
			"shrink-wrapped to a width its own content does not have is a box "+
			"with a gap down one side", box.Px(), line.Px())
	}
}

// TestTextThatReadsOneWayIsUnchanged is the containment case, and it is most of
// the web.
//
// Where nothing is reordered the visually next run is the logically next one, so
// every answer here is the answer before this rule was written down. The
// right-to-left row is the one worth having: a run's gap sits at its *left* when
// it is laid out right to left, so the run across it is the visually previous
// one — which, in text that is all one direction, is the logically next one
// again. The two readings agree, and this is what says so.
func TestTextThatReadsOneWayIsUnchanged(t *testing.T) {
	// What lastRunAt measures is the gaps *between* the runs, which is what the
	// rule is about. A gap the last run carries is past the point measured and a
	// gap at the end of a line hangs anyway.
	for _, tc := range []struct {
		what, markup string
		gaps         float64
	}{
		// One: the pair inside the span. The pairs either side of it belong to
		// the div, which sets nothing.
		{"left to right", `a<span class="ls">bc</span>d`, 1},
		// The same, mirrored. Reordering puts the span's run at the left of the
		// line and the div's letter at the right, and the pair between them is
		// still the div's.
		{"right to left", `&#x5d0;<span class="ls">&#x5d1;&#x5d2;</span>`, 1},
		// Two, from three letters in the span.
		{"three in the span", `a<span class="ls">bcd</span>e`, 2},
	} {
		spaced := lastRunAt(t, tc.markup, spacedDecls)
		plain := lastRunAt(t, tc.markup, ``)
		if got := spaced.Sub(plain).Px(); got != tc.gaps*24 {
			t.Errorf("%s: %q came out %gpx wider than the same text without the "+
				"declaration, want %g — that is %g gaps of 24px",
				tc.what, tc.markup, got, tc.gaps*24, tc.gaps)
		}
	}
}

// TestASpacingOnTheBlockSurvivesADirectionChange is the containment case that
// rules out the reading this one replaced.
//
// "Drop the gap where the two characters end up in different level runs" is the
// obvious summary of bidi-001's assert, and CSS2's bidi-005 through bidi-010
// disprove it: each builds a to m out of nested overrides and asks it to render
// identically to the same letters written plainly, with a letter-spacing on the
// paragraph. Every pair there is governed by the paragraph whichever side the
// gap falls on, so every gap survives — and a rule that dropped them at a
// direction change would take the line to half its width.
func TestASpacingOnTheBlockSurvivesADirectionChange(t *testing.T) {
	const onBlock = ` #p { letter-spacing: 24px }`
	for _, markup := range []string{
		`a&#x5d0;b`,
		`a<span>&#x5d0;</span>b`,
		`<span>a</span>&#x5d0;<span>b</span>`,
	} {
		spaced := lastRunAt(t, markup, onBlock)
		plain := lastRunAt(t, markup, ``)
		// Three runs, so two gaps between them, and both are the block's. That
		// is the whole of the claim — a direction change moves *which pair* a
		// gap is asked about and cannot change the answer when one element
		// governs them all.
		if got := spaced.Sub(plain).Px(); got != 48 {
			t.Errorf("%q put its last character %gpx further along with the "+
				"block's letter-spacing, want 48 — two gaps, and a direction "+
				"change removes neither", markup, got)
		}
	}
}

// TestVisualPositionsInvertsTheOrder is a unit test rather than a document, and
// it is one because no document could be built to catch what it catches.
//
// LineVisualOrder gives the items in the order they are drawn; the walk here
// needs the other direction, a position per item. Getting the inversion
// backwards is invisible to every fixture in this file, and to the suite: L2's
// reordering is a reversal of a contiguous block, a reversal is its own inverse,
// and so is any single one of them. Only a permutation built from reversals at
// two different levels tells the two apart, and putting one on a page means
// nesting overrides three deep for a distinction nothing else in the file is
// about.
func TestVisualPositionsInvertsTheOrder(t *testing.T) {
	// Position 0 draws item 0, position 1 draws item 3, and so on.
	order := []int{0, 3, 1, 2}
	// So item 1 is drawn third, item 2 fourth and item 3 second.
	want := []int{0, 2, 3, 1}
	got := visualPositions(order, len(order))
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("item %d is at position %d, want %d (whole answer %v, want %v)",
				i, got[i], want[i], got, want)
		}
	}
	// And a nil order is the logical one, which the walk reads as "no table".
	if visualPositions(nil, 4) != nil {
		t.Error("a nil order produced a table; the logical order needs none")
	}
}
