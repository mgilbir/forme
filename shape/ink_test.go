package shape

import "testing"

// InkExtent, which is the question "where does this text actually mark the
// page" as against "how much room does a line of this face need".
//
// The two answers differ by a lot and the difference is the point of it, so
// every test below asserts the gap rather than the number alone: an assertion
// that only checked the extent would pass just as well against the face's own
// ascent and descent, and prove nothing about which of them was consulted.

// TestInkExtentIsTighterThanTheFaceForTextThatDoesNotUseIt is the case the
// method exists for.
//
// A horizontal ellipsis is three dots on the baseline, and Courier's descenders
// go 157 units below it. Anything asking whether a box that ends just under the
// baseline cuts the mark gets the wrong answer from the face and the right one
// from here.
func TestInkExtentIsTighterThanTheFaceForTextThatDoesNotUseIt(t *testing.T) {
	f, err := Standard("Courier")
	if err != nil {
		t.Fatal(err)
	}
	d := f.Descriptor()

	above, below, ok := f.InkExtent("…", 1000)
	if !ok {
		t.Fatal("a standard face could not say where its ellipsis puts ink")
	}
	// Adobe's own numbers for the glyph: B 37 -15 563 111.
	if above != 111 || below != 15 {
		t.Errorf("the ellipsis reaches %v above and %v below the baseline, want "+
			"111 and 15 — Adobe's published box for the glyph", above, below)
	}
	// And the gap, which is what makes this worth having. Without it a caller
	// would be told the mark descends ten times as far as it does.
	if float64(-d.Descent) <= below*3 {
		t.Errorf("the face descends %v and the ellipsis %v — too close together "+
			"for this test to show that the glyph was consulted rather than the "+
			"face", -d.Descent, below)
	}
}

// TestInkExtentFollowsTheTextAndNotTheFace: the same face gives three different
// answers for three strings, which nothing derived from the face can do.
func TestInkExtentFollowsTheTextAndNotTheFace(t *testing.T) {
	f, err := Standard("Times-Roman")
	if err != nil {
		t.Fatal(err)
	}
	extent := func(s string) (float64, float64) {
		t.Helper()
		above, below, ok := f.InkExtent(s, 1000)
		if !ok {
			t.Fatalf("no extent for %q", s)
		}
		return above, below
	}

	_, capsBelow := extent("ABC")
	pAbove, pBelow := extent("p")
	kAbove, _ := extent("k")

	// Not zero: a round capital overshoots the baseline by a few units, which
	// is how a "C" is drawn so that it looks as though it sits on the line. The
	// claim is that capitals stay near it while a descender does not.
	if capsBelow >= pBelow/4 {
		t.Errorf("capitals reach %v below the baseline and a descender %v, want "+
			"the capitals far nearer the line", capsBelow, pBelow)
	}
	if pBelow <= 0 {
		t.Errorf("a \"p\" reaches %v below the baseline, want a descender", pBelow)
	}
	if kAbove <= pAbove {
		t.Errorf("an ascender reaches %v and an \"p\" %v, want the ascender higher",
			kAbove, pAbove)
	}
	// A run's extent is the union of its letters', so adding the descender to
	// the capitals has to lower the run.
	if _, both := extent("ABCp"); both != pBelow {
		t.Errorf("\"ABCp\" reaches %v below the baseline, want the \"p\"'s %v",
			both, pBelow)
	}
}

// TestInkExtentScalesWithTheSize, since a caller asks in the size it will draw.
func TestInkExtentScalesWithTheSize(t *testing.T) {
	f, err := Standard("Courier")
	if err != nil {
		t.Fatal(err)
	}
	a1, b1, _ := f.InkExtent("Ap", 1000)
	a2, b2, _ := f.InkExtent("Ap", 16)
	if a2 != a1*16/1000 || b2 != b1*16/1000 {
		t.Errorf("at 16 the extent is %v/%v, want %v/%v — a scaling of the 1000 "+
			"case", a2, b2, a1*16/1000, b1*16/1000)
	}
}

// TestInkExtentIgnoresWhatIsNotDrawn keeps this and Measure telling the same
// story about which characters are on the page.
//
// A space marks nothing, so it neither raises nor lowers a run; a run of
// nothing but spaces marks nowhere at all and has no extent to report. The
// characters shaping drops are the same set Measure charges nothing for.
func TestInkExtentIgnoresWhatIsNotDrawn(t *testing.T) {
	f, err := Standard("Courier")
	if err != nil {
		t.Fatal(err)
	}
	plain, _, _ := f.InkExtent("Ap", 1000)
	spaced, _, ok := f.InkExtent("  A p  ", 1000)
	if !ok || spaced != plain {
		t.Errorf("spaces changed the extent to %v, want the unspaced %v",
			spaced, plain)
	}
	if _, _, ok := f.InkExtent("   ", 1000); ok {
		t.Error("a run of spaces reported an extent; it marks nothing")
	}
	if _, _, ok := f.InkExtent("", 1000); ok {
		t.Error("the empty string reported an extent")
	}
}

// TestInkExtentReadsAGlyfFontsOwnHeaders is the other source: a real font,
// where the numbers come from each glyph's header rather than from a table.
func TestInkExtentReadsAGlyfFontsOwnHeaders(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Skipf("the bundled face is not available: %v", err)
	}
	d := f.Descriptor()

	capsAbove, capsBelow, ok := f.InkExtent("ABC", float64(f.UnitsPerEm()))
	if !ok {
		t.Fatal("a glyf-based face could not say where its capitals put ink")
	}
	if capsAbove <= 0 || capsAbove >= float64(d.Ascent) {
		t.Errorf("capitals reach %v above the baseline; want something above zero "+
			"and below the face's ascent of %v", capsAbove, d.Ascent)
	}
	_, pBelow, _ := f.InkExtent("p", float64(f.UnitsPerEm()))
	if pBelow <= 0 {
		t.Errorf("a \"p\" reaches %v below the baseline, want a descender", pBelow)
	}
	// The gap is the assertion: capitals barely cross the line — a round one
	// overshoots it by a few units — and a descender goes most of the way to
	// the face's own descent. A number taken from the face could not tell the
	// two runs apart at all.
	if capsBelow >= pBelow/4 {
		t.Errorf("capitals reach %v below the baseline and a descender %v, want "+
			"the capitals far nearer the line", capsBelow, pBelow)
	}
	if pBelow <= float64(-d.Descent)/2 {
		t.Errorf("a descender reaches %v against the face's declared %v, want it "+
			"to account for most of what the face reserves", pBelow, -d.Descent)
	}
}

// TestInkExtentSaysSoWhenItCannotAnswer.
//
// The extents of a CFF-flavoured font's glyphs are in its charstrings and
// cannot be had without interpreting them. Answering with the face's ascent and
// descent instead would be a plausible number that is not what was asked for,
// and a caller cannot tell one from the other — so it does not answer, and the
// caller falls back knowingly.
func TestInkExtentSaysSoWhenItCannotAnswer(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Skipf("the bundled face is not available: %v", err)
	}
	if f.prog == nil || f.prog.GlyphBBox == nil {
		t.Skip("the bundled face has no glyf table to remove")
	}
	// The same face with the table taken away, which is the shape a CFF font
	// arrives in.
	blind := *f
	prog := *f.prog
	prog.GlyphBBox = nil
	blind.prog = &prog
	if _, _, ok := blind.InkExtent("Ap", 1000); ok {
		t.Error("a face with no glyph boxes reported an extent")
	}
}
