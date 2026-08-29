package layout

import (
	"strconv"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A line break beside an ideograph, over a document.
//
// paragraph/ideographboundary_test.go states the rule over one run of text.
// These are the two things layout has to add: the same opportunity where the
// two characters are in different boxes, and what happens to §8.1's gap when a
// line ends at the boundary it was inserted into.

// cjkFaceLines lays a document out in the fallback faces and returns its lines.
// cjkLines in linebreak_test.go is the same over the standard faces, which have
// no ideographs; these fixtures measure a gap and need faces that do.
func cjkFaceLines(t *testing.T, faces []*shape.Face, htmlSrc, cssSrc string) []string {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(1000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	return lineTexts(linesOf(t, frag, "d"))
}

// cjkNaturalWidth is how wide a fixture's content is when nothing wraps, so a
// test can set a box to exactly that and let one rule decide the line.
func cjkNaturalWidth(t *testing.T, faces []*shape.Face, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(10000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	lines := linesOf(t, frag, "d")
	if len(lines) != 1 {
		t.Fatalf("the fixture wrapped in 10000px: %q", lineTexts(lines))
	}
	var total style.Unit
	for _, r := range lines[0].Runs {
		total = total.Add(r.Width)
	}
	return total
}

// TestABoxBoundaryDoesNotHideTheOpportunityBeforeAnIdeograph.
//
// SplitAtBreaks is given one box's text, and this boundary has a character on
// each side of it in two different boxes — so the box holding the later
// character is the one that can say the opportunity is there. Without it the
// same text answers differently depending on whether the author wrote a <span>,
// which is the one thing §5.1's nearest-common-ancestor rule is written to
// prevent.
//
// The box is a hair narrower than the whole text, so the one opportunity in it
// is what decides the line: with it the ideograph begins the second line, and
// without it there is nowhere to break before the "def" and the first line takes
// the ideograph too.
func TestABoxBoundaryDoesNotHideTheOpportunityBeforeAnIdeograph(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const base = `#d { font-family: monospace; font-size: 20px; ` +
		`text-autospace: no-autospace`
	const plainHTML = `<div id="d" lang="ja">abc永def</div>`
	const splitHTML = `<div id="d" lang="ja">abc<span>永def</span></div>`

	// A hair narrower than "abc永", so the ideograph is what does not fit.
	head := cjkNaturalWidth(t, faces, `<div id="d" lang="ja">abc永</div>`, base+` }`)
	css := base + `; width: ` + ftoaPx(head.Sub(bgpx(1))) + ` }`

	plain := cjkFaceLines(t, faces, plainHTML, css)
	if len(plain) == 0 || plain[0] != "abc" {
		t.Fatalf("the plain fixture came out as %q; the box is a pixel narrower "+
			"than \"abc永\", so the line ends in front of the ideograph", plain)
	}
	split := cjkFaceLines(t, faces, splitHTML, css)
	if len(split) != len(plain) || split[0] != plain[0] {
		t.Errorf("the text breaks as %q written plainly and as %q with a <span> in "+
			"front of the ideograph; the boundary is not a place the answer may "+
			"change", plain, split)
	}
}

// TestAGapAtALineEndDoesNotCount.
//
// §8.1 puts an eighth of an em between an ideograph and the Latin beside it, and
// the gap is *between* two characters. A line break at that boundary puts them
// on different lines, where they are not adjacent and there is no gap — so a run
// whose far edge carries one is not an eighth of an em too wide to end a line it
// fits on. It is the same rule §8.2 states for letter-spacing in as many words.
//
// The box is exactly the width the content has with no gap in it, so the gap is
// the whole of what decides.
func TestAGapAtALineEndDoesNotCount(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const base = `#d { font-family: monospace; font-size: 20px`
	// "ab永" on its own: the gap §8.1 puts between the b and the 永 is in it,
	// and there is nothing after the 永 for a second gap to go to. So this is
	// exactly what the first line of the fixture below has to hold.
	head := cjkNaturalWidth(t, faces, `<div id="d" lang="ja">ab永</div>`, base+` }`)
	plain := cjkNaturalWidth(t, faces, `<div id="d" lang="ja">ab永</div>`,
		base+`; text-autospace: no-autospace }`)
	if head == plain {
		t.Skip("this font library puts no gap at the fixture's boundary, so the " +
			"fixture cannot tell the two answers apart")
	}

	got := cjkFaceLines(t, faces, `<div id="d" lang="ja">ab永cd</div>`,
		base+`; width: `+ftoaPx(head)+` }`)
	if len(got) == 0 || got[0] != "ab永" {
		t.Errorf("the first line is %q, want \"ab永\" — the gap §8.1 puts after the "+
			"ideograph is at the line's end once the line breaks there, and a gap "+
			"a line break falls on is not between two adjacent characters", got)
	}
}

// TestAGapInsideALineStillCounts is the containment half. The rule is about the
// gap the break replaces and nothing else: one with text on both sides of it on
// the same line is between two adjacent characters and is exactly what §8.1 is
// for.
func TestAGapInsideALineStillCounts(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const base = `#d { font-family: monospace; font-size: 20px; display: inline-block`
	width := func(extra string) style.Unit {
		built := Build(Input{HTML: `<div id="d" lang="ja">ab永</div>`,
			CSS: []Stylesheet{{Source: noDefaults + base + extra + ` }`}}})
		w, _ := style.FromPx(1000)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h},
			suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
		return find(t, frag, "d").BorderRect.W
	}
	with, without := width(""), width("; text-autospace: no-autospace")
	if with.Sub(without) <= 0 {
		t.Errorf("the box is %v with the gap and %v without it; a gap between two "+
			"characters of one line is what §8.1 is for", with, without)
	}
}

// ftoaPx renders a width as a CSS length, so a fixture can set a box to exactly
// what it measured. ftoa beside it truncates to whole pixels, which is a
// rounding these fixtures are about — an eighth of an em at 20px is 2.5.
func ftoaPx(u style.Unit) string { return strconv.FormatFloat(u.Px(), 'f', -1, 64) + "px" }

// TestAShrinkWrappedBoxDoesNotHoldTheGapAtItsEdge.
//
// The intrinsic pass and the fill are two measurements of the same line, and a
// line whose measure differs between them is a box shrink-wrapped to a width its
// own content does not have. The pass already discounts the letter-spacing after
// a run's last character, for §8.2's reason; §8.1's gap sits at the same edge
// and goes the same way.
//
// The suite's word-break-keep-all-011 is the fixture that found it: a
// "width: min-content" paragraph of "中文english中文english…", whose narrowest
// unbreakable run is the "english" — and whose box came out an eighth of an em
// wider than the letters in it, because the run carried the gap to the ideograph
// after it.
func TestAShrinkWrappedBoxDoesNotHoldTheGapAtItsEdge(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "永"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const base = `#d { font-family: monospace; font-size: 20px; width: min-content`
	width := func(html, extra string) style.Unit {
		built := Build(Input{HTML: html,
			CSS: []Stylesheet{{Source: noDefaults + base + extra + ` }`}}})
		w, _ := style.FromPx(1000)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h},
			suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
		return find(t, frag, "d").BorderRect.W
	}
	// The narrowest unbreakable run of "永abc永" is the "abc", which carries the
	// gap to the 永 after it. The box is that run and nothing else.
	got := width(`<div id="d" lang="ja">永abc永</div>`, "")
	want := width(`<div id="d" lang="ja">abc</div>`, `; text-autospace: no-autospace`)
	if got != want {
		t.Errorf("the box is %v wide and the run it shrank to is %v; the gap at the "+
			"run's far edge is at the line's end, where §8.1 puts none", got, want)
	}
	// And the fixture is one where a gap exists at all, or the equality above
	// holds for a reason that has nothing to do with the discount. It is asked
	// of max-content, where the gap is *between* two characters of one line and
	// so is exactly what §8.1 adds — asking min-content would be asking the
	// same discounted number twice.
	wide := func(extra string) style.Unit {
		built := Build(Input{HTML: `<div id="d" lang="ja">abc永</div>`,
			CSS: []Stylesheet{{Source: noDefaults +
				`#d { font-family: monospace; font-size: 20px; width: max-content` +
				extra + ` }`}}})
		w, _ := style.FromPx(1000)
		h, _ := style.FromPx(10000)
		frag := Layout(built.Root, Size{W: w, H: h},
			suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
		return find(t, frag, "d").BorderRect.W
	}
	if wide("") == wide(`; text-autospace: no-autospace`) {
		t.Skip("this font library puts no gap at the fixture's boundary")
	}
}
