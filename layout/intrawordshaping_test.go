package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// CSS Text §5.4's shaping across an intra-word break, over a document.
//
// paragraph/intrawordshaping_test.go states it over the item, which is where
// the cut is made. This is the page it produces, and it is the suite's own
// fixture: overflow-wrap-shaping-001 sets an Arabic word in a box too narrow
// for it and compares the result against the same word written out as
// presentation forms, so that there is nothing to argue about.

// glyphsOfLines shapes every run of every line as it will be drawn and returns
// the glyph ids in the order the *text* is written in.
//
// The shaper hands back visual order, which for a right-to-left run is the
// reverse of the logical one — so a run is turned back before the lines are
// joined. Without that the two halves of a broken Arabic word come out in the
// order the pen met them, which is a fact about the pen and not about the
// shaping this is asking about.
func glyphsOfLines(lines []LineFragment) []int {
	var out []int
	for _, line := range lines {
		for _, r := range line.Runs {
			if r.Face == nil {
				continue
			}
			gs, _ := r.Face.ShapeGlyphsInContext(r.Text, r.PreContext, r.PostContext, shape.Features{})
			ids := make([]int, 0, len(gs))
			for _, g := range gs {
				ids = append(ids, g.GID)
			}
			if r.RTL {
				reverseInts(ids)
			}
			out = append(out, ids...)
		}
	}
	return out
}

func reverseInts(a []int) {
	for i, j := 0, len(a)-1; i < j; i, j = i+1, j-1 {
		a[i], a[j] = a[j], a[i]
	}
}

func glyphsOf(f *shape.Face, s string) []int {
	gs, _ := f.ShapeGlyphs(s)
	out := make([]int, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.GID)
	}
	return out
}

// TestAWordBrokenByOverflowWrapKeepsItsJoiningForms is the bug.
func TestAWordBrokenByOverflowWrapKeepsItsJoiningForms(t *testing.T) {
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face that joins")
	}
	const word = "عائلة"
	face, ok := faceWithGlyphFor(faces, word)
	if !ok || !face.HasJoiningForms() {
		t.Skip("no face here sets the fixture with positional forms")
	}

	// Narrow enough that the word has to be cut, and there is nothing else on
	// the line to break at.
	built := Build(Input{HTML: `<div id="d" dir="rtl" lang="ar">` + word + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-size: 64px; width: 100px; overflow-wrap: break-word }`}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	lines := linesOf(t, frag, "d")
	if len(lines) < 2 {
		t.Fatalf("the word was not broken: %d line(s), %v", len(lines), lineTexts(lines))
	}

	// The fixture is one right-to-left word, so the glyphs the face hands back
	// for it whole are in visual order and the comparison is against the
	// logical one.
	want := glyphsOf(face, word)
	reverseInts(want)
	got := glyphsOfLines(lines)
	if !sameInts(got, want) {
		t.Errorf("the broken word is set as glyphs %v and the whole word is %v; "+
			"§5.4 shapes the characters as if the word were not broken", got, want)
	}
	// The fixture has to be a word whose forms depend on their neighbours, or
	// the equality above holds for a reason that has nothing to do with the cut.
	var apart []int
	for _, line := range lines {
		for _, r := range line.Runs {
			ids := glyphsOf(face, r.Text)
			if r.RTL {
				reverseInts(ids)
			}
			apart = append(apart, ids...)
		}
	}
	if sameInts(apart, want) {
		t.Fatalf("the two halves shape to the same glyphs alone as joined, so this " +
			"face does not distinguish the fixture")
	}
}

// TestACutHalfIsAsWideAsItIsDrawn is the other half of the same rule, and the
// one the line depends on rather than the letters.
//
// A cursive letter's advance depends on the form it takes. A head measured
// alone is measured to one width and painted at another, and the fill that
// chose where to cut chose against the wrong number — so the line is filled to
// a width the page does not have.
func TestACutHalfIsAsWideAsItIsDrawn(t *testing.T) {
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face that joins")
	}
	const word = "عائلة"
	face, ok := faceWithGlyphFor(faces, word)
	if !ok || !face.HasJoiningForms() {
		t.Skip("no face here sets the fixture with positional forms")
	}
	built := Build(Input{HTML: `<div id="d" dir="rtl" lang="ar">` + word + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-size: 64px; width: 100px; overflow-wrap: break-word }`}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))

	for i, line := range linesOf(t, frag, "d") {
		for _, r := range line.Runs {
			if r.Face == nil {
				continue
			}
			want, _ := style.FromPx(r.Face.MeasureShapedInContext(r.Text, r.Size.Px(),
				r.PreContext, r.PostContext, r.ContextKerns, r.Features))
			if r.Width != want {
				t.Errorf("line %d's run %q is %v wide and is drawn %v wide",
					i, r.Text, r.Width, want)
			}
		}
	}
}

// TestTheCutIsChosenAgainstTheWidthTheHeadWillHave.
//
// The prefix is found by bisection over the cluster boundaries, and what it
// measures has to be what the page draws. A prefix measured *alone* is measured
// with its last letter in its final form, and the letter will be drawn in a
// medial one — in NotoSansArabic at 64px three joined behs are 61px in context
// and 109px alone, so a box that holds three of them was told it held one.
//
// The expectation is computed from the face rather than written down, so this is
// the greedy rule and not this font's numbers: the largest prefix that fits, and
// nothing about which prefix that is.
func TestTheCutIsChosenAgainstTheWidthTheHeadWillHave(t *testing.T) {
	faces := notoFaces()
	if len(faces) == 0 {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) to read a face that joins")
	}
	const word = "بببببب" // six behs, which join throughout
	const size, box = 64.0, 65.0
	face, ok := faceWithGlyphFor(faces, word)
	if !ok || !face.HasJoiningForms() {
		t.Skip("no face here sets the fixture with positional forms")
	}

	runes := []rune(word)
	best := 0
	for n := 1; n < len(runes); n++ {
		prefix, rest := string(runes[:n]), string(runes[n:])
		if face.MeasureShapedInContext(prefix, size, "", rest, true, shape.Features{}) <= box {
			best = n
		}
	}
	if best < 2 {
		t.Skipf("only %d letter(s) fit in %gpx of this face, so the fixture cannot "+
			"tell the two measurements apart", best, box)
	}

	built := Build(Input{HTML: `<div id="d" dir="rtl" lang="ar">` + word + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#d { font-size: 64px; width: 65px; overflow-wrap: break-word }`}}})
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	lines := lineTexts(linesOf(t, frag, "d"))
	if len(lines) < 2 {
		t.Fatalf("the word was not broken: %v", lines)
	}
	if got := len([]rune(lines[0])); got != best {
		t.Errorf("the first line holds %d letter(s) and %d fit in %gpx; the cut is "+
			"chosen against the width the head will be drawn at", got, best, box)
	}
}
