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
//
// # Why the box is not simply the extent of the runs
//
// §8.2 puts a letter-spacing after a run's last character as well as between
// them, and that last one *hangs*: it sits past the end of the line rather than
// being width the line has to find, which is why the fill discounts it and why a
// box shrink-wrapped around the text is that much narrower than the runs reach.
// So "the box equals the extent" is true only of a line with nothing hanging off
// its right, and this file's own fixture is not one — it was, while the discount
// was taken from the last character in *logical* order, which on a line that
// ends in Hebrew is not the character at the line's right-hand end.
//
// The hang is written out per fixture rather than computed, because computing it
// is what is under test. Courier at 20px is 12px a character and the spacing is
// 24px, so a hang is 24 and no hang is 0.
func TestTheIntrinsicWidthAgreesWithTheLine(t *testing.T) {
	for _, tc := range []struct {
		markup string
		hang   float64
		what   string
	}{
		// The bidi fixture. Visually the line reads a, b, ב, א — so the
		// character at its right-hand end is the span's last, and the span's
		// spacing is what hangs.
		{`a<span class="ls">b&#x5d0;</span>&#x5d1;`, 24,
			"the span's last character ends up at the line's right"},
		// The same shape with nothing to reorder, where the character at the
		// right is the last one written and carries the paragraph's spacing.
		{`<span class="ls">ab</span>`, 24, "a span that ends the line"},
		{`a<span class="ls">b</span>`, 24, "a span of one character that ends the line"},
		// And a line with no spacing anywhere, where the box is the extent.
		{`ab`, 0, "nothing hangs"},
	} {
		line := lineSpan(t, tc.markup, spacedDecls)
		box := boxWidth(t, tc.markup, spacedDecls)
		hang, _ := style.FromPx(tc.hang)
		if want := line.Sub(hang); box != want {
			t.Errorf("%s: max-content is %gpx and the line it holds reaches %gpx "+
				"with %gpx hanging off its right, so the box should be %gpx — a box "+
				"shrink-wrapped to a width its own content does not have is a box "+
				"with a gap down one side",
				tc.what, box.Px(), line.Px(), tc.hang, want.Px())
		}
	}
}

// TestTheSpacingThatHangsIsTheRightmostRuns is the defect the two halves above
// were measured against, stated on a line where the answer differs.
//
// §8.2's spacing after a run's last character sits at the run's *visual right*
// whatever direction the run reads — gapNeighbour checked that against the
// display list — so what hangs off a line is the spacing of the run nothing is
// drawn to the right of. Both the fill and the intrinsic pass asked the item
// that comes *last*, which on a right-to-left line is the leftmost one.
//
// It shows only where the two runs carry different spacings, which is why no
// fixture had it: with one value on the paragraph, whichever run is asked gives
// the same number. Here one span has 2px and the other 16px, and the difference
// between asking the right run and the wrong one is fourteen pixels of text
// outside a float that was shrink-wrapped around it.
//
// The oracle is the pair: the same two spans in the two source orders, laid out
// in the two directions, make *the same visual line* twice over — same runs,
// same widths, same order — so they must shrink-wrap to the same width. That is
// a stronger statement than any single number, and three of the four were wrong.
func TestTheSpacingThatHangsIsTheRightmostRuns(t *testing.T) {
	const sheet = `#p { font-family: Courier; font-size: 20px; float: left }` +
		`#a { letter-spacing: 16px } #b { letter-spacing: 2px }`
	width := func(markup, dir string) style.Unit {
		t.Helper()
		f := find(t, layoutOf(t, 100000,
			`<div id="p" style="`+dir+`">`+markup+`</div>`, sheet), "p")
		if len(f.Lines) != 1 {
			t.Fatalf("%q under %q laid out as %d lines", markup, dir, len(f.Lines))
		}
		return f.ContentRect().W
	}
	const (
		aFirst = `<span id=a>&#x5d0;&#x5d1;&#x5d2;</span> <span id=b>abc</span>`
		bFirst = `<span id=b>abc</span> <span id=a>&#x5d0;&#x5d1;&#x5d2;</span>`
	)
	want := width(aFirst, "direction: ltr")
	for _, tc := range []struct{ markup, dir string }{
		{aFirst, "direction: rtl"},
		{bFirst, "direction: ltr"},
		{bFirst, "direction: rtl"},
	} {
		if got := width(tc.markup, tc.dir); got != want {
			t.Errorf("%q under %q shrink-wraps to %v and the same visual line "+
				"written the other way round to %v; the spacing that hangs is the "+
				"rightmost run's, and the same line has the same rightmost run",
				tc.markup, tc.dir, got, want)
		}
	}
	// And what the width *is*, so that four boxes agreeing about the wrong
	// number would not pass.
	//
	// It is read off the display list rather than written down: the box is the
	// ink the line reaches, less the spacing of the run at its right-hand end,
	// which is the sentence this whole file is about turned into arithmetic. A
	// number worked out by hand here would only be this same sum done less
	// carefully — and the one this test was written with was wrong.
	for _, dir := range []string{"direction: ltr", "direction: rtl"} {
		f := find(t, layoutOf(t, 100000,
			`<div id="p" style="`+dir+`">`+aFirst+`</div>`, sheet), "p")
		var lo, hi style.Unit
		var rightmost TextRun
		for i, r := range f.Lines[0].Runs {
			if i == 0 || r.X < lo {
				lo = r.X
			}
			if end := r.X.Add(r.Width); i == 0 || end > hi {
				hi, rightmost = end, r
			}
		}
		ink := hi.Sub(lo)
		hang := rightmost.LetterSpacing
		if got := width(aFirst, dir); got != ink.Sub(hang) {
			t.Errorf("under %q the line reaches %v with %v hanging off the run at "+
				"its right, so the box should be %v and is %v",
				dir, ink, hang, ink.Sub(hang), got)
		}
	}
}
