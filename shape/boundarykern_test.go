package shape

import (
	"os"
	"path/filepath"
	"testing"
)

// Pair kerning across the boundary between two runs.
//
// CSS Text §8.1 says an element boundary does not break shaping, so "A<span>V"
// is one kern pair set as two runs — and so is any pair the engine's own passes
// cut apart, which §8.4's hanging punctuation does to a full stop at the end of
// a line. A run shaped alone never sees the glyph on the far side, so the pair
// came out unkerned and the two characters were drawn further apart than the
// font asks for. See boundarykern.go.

// kerningFace loads a face known to carry pair kerning.
func kerningFace(t *testing.T, name string) *Face {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face with kern pairs")
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Skipf("no such font in this checkout: %v", err)
	}
	face, err := Load(data)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !face.HasKerning() {
		t.Skipf("%s carries no kerning this package could read", name)
	}
	return face
}

// advanceOf sums a shaped run, which is what a line is filled from.
func advanceOf(gs []Glyph) float64 {
	total := 0.0
	for _, g := range gs {
		total += g.XAdvance
	}
	return total
}

func wholeAdvance(f *Face, s string) float64 {
	gs, _ := f.ShapeGlyphs(s)
	return advanceOf(gs)
}

func contextAdvance(f *Face, s, before, after string) float64 {
	gs, _ := f.ShapeGlyphsInContext(s, before, after)
	return advanceOf(gs)
}

// kernedPair finds a pair this face actually kerns, so that the tests below
// assert something rather than comparing two numbers a face never changed.
func kernedPair(t *testing.T, f *Face, pairs ...string) string {
	t.Helper()
	for _, p := range pairs {
		if wholeAdvance(f, p) != wholeAdvance(f, p[:len(p)/2])+wholeAdvance(f, p[len(p)/2:]) {
			return p
		}
	}
	t.Skipf("none of %v is kerned by this face", pairs)
	return ""
}

// TestASplitPairIsStillKerned is the bug: the same two characters, shaped whole
// and shaped as two runs, must occupy the same width.
//
// It is the invariant the whole change exists for and is also the guard against
// the obvious way of getting it wrong. A pair record says something about each
// of its two glyphs, so an implementation that gave both halves the whole
// adjustment would make the split pair *narrower* than the joined one.
func TestASplitPairIsStillKerned(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	pair := kernedPair(t, f, "AV", "To", "LT", "Yo", "AW", "P,")
	left, right := pair[:1], pair[1:]

	whole := wholeAdvance(f, pair)
	split := contextAdvance(f, left, "", right) + contextAdvance(f, right, left, "")
	if split != whole {
		t.Errorf("%q is %v wide shaped whole and %v as two runs; an element boundary "+
			"does not break shaping, so the pair is kerned either way",
			pair, whole, split)
	}
	// And the split really is split — the halves alone differ from the whole,
	// which is what makes the equality above a statement about the context.
	if apart := wholeAdvance(f, left) + wholeAdvance(f, right); apart == whole {
		t.Fatalf("%q measures the same joined and apart, so this face does not kern "+
			"it and the test asserts nothing", pair)
	}
}

// TestTheContextOnlyChangesTheRunItIsGivenTo. Neither half may collect the other
// half's share of the adjustment, or two runs that are laid out separately drift
// apart by however many boundaries stand between them.
func TestTheContextOnlyChangesTheRunItIsGivenTo(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	pair := kernedPair(t, f, "AV", "To", "LT", "Yo", "AW", "P,")
	left, right := pair[:1], pair[1:]

	// What each glyph is given inside the joined run, which is the answer the
	// two runs have to add up to.
	gs, _ := f.ShapeGlyphs(pair)
	if len(gs) != 2 {
		t.Skipf("%q shapes to %d glyphs, so the halves do not correspond", pair, len(gs))
	}
	if got := contextAdvance(f, left, "", right); got != gs[0].XAdvance {
		t.Errorf("the left half is %v with the right for context and %v inside the "+
			"joined run", got, gs[0].XAdvance)
	}
	if got := contextAdvance(f, right, left, ""); got != gs[1].XAdvance {
		t.Errorf("the right half is %v with the left for context and %v inside the "+
			"joined run", got, gs[1].XAdvance)
	}
}

// TestAContextThatIsNotTheOtherHalfChangesNothing is the containment case: the
// pair is looked up, not assumed, so a neighbour the font says nothing about
// leaves the run exactly as it was.
func TestAContextThatIsNotTheOtherHalfChangesNothing(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	for _, s := range []string{"n", "m", "o"} {
		alone := wholeAdvance(f, s)
		for _, after := range []string{"n", "m", "o"} {
			if wholeAdvance(f, s+after) != alone+wholeAdvance(f, after) {
				continue // this face does kern the pair; not a case for this test
			}
			if got := contextAdvance(f, s, "", after); got != alone {
				t.Errorf("%q before %q is %v and alone is %v; the font kerns the pair "+
					"by nothing, so the context may not move it", s, after, got, alone)
			}
		}
	}
}

// TestTheBoundaryPairIsFoundThroughTheNeighboursGlyph, and not through its
// characters.
//
// A neighbour is shaped before it is paired, so a ligature or a contextual form
// on the far side is looked up as the glyph it becomes. This is the reason
// boundaryGlyphs is a shaping pass rather than a GlyphID call.
//
// No face in this checkout kerns anything against a ligature differently from
// how it kerns the ligature's first letter, so the pair is planted — the same
// white-box argument as the test below, and for the same reason: the branch
// cannot otherwise be told apart from the cheap version of itself.
func TestTheBoundaryPairIsFoundThroughTheNeighboursGlyph(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	if !f.HasLigatures() {
		t.Skip("this face substitutes nothing, so a neighbour is its own glyph")
	}
	lig, _ := f.ShapeGlyphs("fi")
	if len(lig) != 1 {
		t.Skipf("this face sets \"fi\" as %d glyphs, so there is no ligature to "+
			"tell apart from its first letter", len(lig))
	}
	first, ok := f.GlyphID('f')
	if !ok || first == lig[0].GID {
		t.Skip("the ligature is the letter's own glyph here")
	}
	left, ok := f.GlyphID('n')
	if !ok {
		t.Skip("this face has no glyph for \"n\"")
	}
	if wholeAdvance(f, "nfi") != wholeAdvance(f, "n")+wholeAdvance(f, "fi") {
		t.Skip("this face already kerns n before the fi ligature")
	}
	// Stated on the ligature and on nothing else. An implementation pairing the
	// characters would look for "n" before "f" and find nothing.
	l := f.layoutFor(runScript("nfi"))
	l.kern = append(l.kern, kernLookup{
		pairs: map[[2]int]pairAdjust{{left, lig[0].GID}: {firstAdvance: -100}},
	})

	whole := wholeAdvance(f, "nfi")
	if apart := wholeAdvance(f, "n") + wholeAdvance(f, "fi"); whole == apart {
		t.Fatalf("the planted pair changed nothing: %v joined and %v apart", whole, apart)
	}
	split := contextAdvance(f, "n", "", "fi") + contextAdvance(f, "fi", "n", "")
	if split != whole {
		t.Errorf("\"nfi\" is %v wide joined and %v as two runs; the pair is stated on "+
			"the ligature, so the neighbour has to be shaped before it is looked up",
			whole, split)
	}
}

// TestTheNeighboursShareOfThePairGoesToTheNeighboursRun.
//
// A GPOS pair record states a ValueRecord for *each* of its two glyphs, and
// either may be the one that moves. Every face in this checkout states the pair
// on the first glyph alone — 93,606 pairs across eight Noto faces, none with a
// second record — so nothing here can exercise the other half by loading a font,
// and the branch that reads it would go untested and could be quietly wrong.
//
// So the pair is written into a loaded face's own table. That is white-box, and
// it is the only way to state the rule at all: what is asserted is exactly what
// the format allows and what a face that used it would get.
func TestTheNeighboursShareOfThePairGoesToTheNeighboursRun(t *testing.T) {
	f := kerningFace(t, "NotoSans-Regular.ttf")
	left, right := "n", "o"
	lg, ok1 := f.GlyphID(rune(left[0]))
	rg, ok2 := f.GlyphID(rune(right[0]))
	if !ok1 || !ok2 {
		t.Skipf("this face has no glyph for %q or %q", left, right)
	}
	if wholeAdvance(f, left+right) != wholeAdvance(f, left)+wholeAdvance(f, right) {
		t.Skipf("this face already kerns %q, so the planted pair would not be the "+
			"whole of the difference", left+right)
	}
	// A pair whose whole adjustment is on the *second* glyph, which is the case
	// no font here exercises. It goes into the layout the shaper reads for this
	// run's script rather than the face's own, which is where a font's pairs
	// arrive once the script has selected among its features.
	const units = -100
	l := f.layoutFor(runScript(left + right))
	l.kern = append(l.kern, kernLookup{
		pairs: map[[2]int]pairAdjust{{lg, rg}: {secondAdvance: units}},
	})

	whole := wholeAdvance(f, left+right)
	if apart := wholeAdvance(f, left) + wholeAdvance(f, right); whole == apart {
		t.Fatalf("the planted pair changed nothing: %v joined and %v apart", whole, apart)
	}
	split := contextAdvance(f, left, "", right) + contextAdvance(f, right, left, "")
	if split != whole {
		t.Errorf("%q is %v wide joined and %v as two runs; the adjustment is stated "+
			"on the second glyph, so it is the second run that has to collect it",
			left+right, whole, split)
	}
	// And it went to the run it belongs to rather than to either of them.
	if got, alone := contextAdvance(f, left, "", right), wholeAdvance(f, left); got != alone {
		t.Errorf("the first run is %v with the second for context and %v alone; this "+
			"pair says nothing about its first glyph", got, alone)
	}
}

// TestTheBoundaryWindowCountsCharacters. The cap is a bound on work, so it is
// stated in characters: cutting a string at a byte would hand the shaper half a
// character, and half a character has no glyph.
func TestTheBoundaryWindowCountsCharacters(t *testing.T) {
	const cjk = "まだよくています。しかし特"
	for _, tc := range []struct {
		s           string
		n           int
		first, last string
	}{
		{"abcdef", 2, "ab", "ef"},
		{"abcdef", 0, "", ""},
		{"abcdef", 99, "abcdef", "abcdef"},
		{"", 4, "", ""},
		{cjk, 1, "ま", "特"},
		{cjk, 3, "まだよ", "しかし特"[len("しかし特")-len("特")*1:]},
	} {
		if got := firstRunes(tc.s, tc.n); got != tc.first {
			t.Errorf("firstRunes(%q, %d) = %q, want %q", tc.s, tc.n, got, tc.first)
		}
	}
	// The last n, stated by counting rather than by slicing, so the expectation
	// is not written the same way the code is.
	for _, n := range []int{0, 1, 3, 13, 99} {
		runes := []rune(cjk)
		from := len(runes) - n
		if from < 0 {
			from = 0
		}
		if n == 0 {
			from = len(runes)
		}
		if got, want := lastRunes(cjk, n), string(runes[from:]); got != want {
			t.Errorf("lastRunes(%q, %d) = %q, want %q", cjk, n, got, want)
		}
	}
}
