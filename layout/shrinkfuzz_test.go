package layout

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Two ways of measuring one paragraph, asked to agree.
//
// A box that shrink-wraps its content — a float, an inline-block, a table cell,
// an absolutely positioned box with one offset — is sized by §10.3.5's
// shrink-to-fit from the *intrinsic* widths, which a pass computes without
// breaking any lines. The lines are then filled by the breaker, which is a
// different walk over the same items with a different accounting.
//
// The two have to arrive at the same number, and nothing in either of them says
// so. Where they differ the failure is quiet in both directions: a float wider
// than its text is a gap nobody asked for, and a float narrower than its text is
// content over the edge of a box that was supposed to be wrapped around it.
//
// They have differed. A word split by a <span> measured one way in the intrinsic
// pass and another on the line, because the run widths were quantized
// separately; a cursive letter was measured in its isolated form and drawn in a
// medial one. Both were found from the other end — a reftest, a fixture — and
// both would have been found here.
//
// The invariant is stated at a viewport wide enough that the shrink-to-fit is
// the *preferred* width rather than the available one, because that is the case
// where the two walks are answering the same question. The text is bounded so
// that no document the fuzzer writes can reach the edge of it.
func FuzzShrinkToFit(f *testing.F) {
	for _, text := range shrinkTexts {
		for i := range shrinkStyles {
			f.Add(text, i)
		}
	}
	f.Fuzz(func(t *testing.T, text string, which int) {
		checkShrinkToFit(t, text, which)
	})
}

// shrinkTexts are the shapes that have historically measured differently on the
// two walks: a word an element boundary divides, a cursive script whose letters
// change form, white space a line edge takes away, a character that is not text,
// and the two directions in one line.
var shrinkTexts = []string{
	"", "hello world", "a b c d e f g h",
	"a<span>b</span>c", "<span>hello</span> <span>world</span>",
	"العربية مرحبا", "ع<span>ع</span>ع",
	"高高高 abc", "one two  three   four",
	"a­b", "x y", "a​b",
	"abc אבג def",
	"\tx\ty", "line\nbreak",
	"\xff\xfe", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
}

// shrinkStyles are the declarations that change how a run is measured, each of
// which the two walks have to account for the same way.
var shrinkStyles = []string{
	"",
	"letter-spacing: 2px",
	"word-spacing: 5px",
	"white-space: pre-wrap",
	"white-space: pre",
	"text-align: justify",
	"hyphens: auto",
	"text-autospace: normal",
	"font-size: 13px",
	"text-transform: uppercase",
	"direction: rtl",
	"letter-spacing: 2px; word-spacing: 5px; white-space: pre-wrap",
}

// checkShrinkToFit is the body, and is a function of its own so a test can drive
// it with a fixture: a fuzz target whose assertion nothing has been seen to
// break asserts nothing.
//
// The two walks are compared by *laying the same content out at an explicit
// width*, which is what keeps this from asking one number about itself. A line
// box is stretched to its containing block, so "the widest line equals the box"
// is true of every box ever laid out and says nothing at all — the first version
// of this asserted exactly that and found nothing in a quarter of a million
// runs, which is how it was noticed.
//
// What is asked instead is what the width is *for*:
//
//   - At the width the shrink-wrap chose, the content breaks into the same lines
//     it breaks into at an unbounded width. A box narrower than its content is
//     one whose text wraps where the author put no break.
//   - Nothing unbreakable overflows it. In a viewport this wide the
//     shrink-to-fit is the preferred width, which is at least the widest run
//     that cannot be broken — so a report is the intrinsic pass having measured
//     something smaller than the fill did, and the page has content over the
//     edge of the box meant to hold it.
//
// # What is not asserted, and why it cannot be
//
// The other direction: that the box is no *wider* than it needs. The obvious
// form of it — one layout unit narrower, the content breaks differently — is
// false, and the fuzzer found the counterexample before this comment was
// written. "0 \f0000" breaks at the form feed whatever the width, so the box is
// as wide as "0000"; take a unit off and the same two lines come back with the
// second one over the edge, because a line whose widest run cannot be broken
// does not gain a line, it spills. The same holds under "white-space: pre" for
// every text.
//
// Distinguishing "exactly wide enough" from "a unit too wide" therefore needs
// the width the fill *used*, and the only thing that knows it is the fill's own
// accounting — which is the thing under test. So it is left out rather than
// stated in a form that is nearly true: a box wider than its content is a gap
// nobody asked for, and the two above are the direction that loses text.
func checkShrinkToFit(t *testing.T, text string, which int) {
	// Bounded so that the shrink-to-fit is the preferred width and not the
	// available one. The viewport below is 100,000px and the largest a
	// character can measure here is one em, so a thousand of them cannot reach
	// it — and past that the box is clamped, the lines are broken to fit, and
	// the width is no longer the number the intrinsic pass computed.
	if len(text) > 1000 {
		return
	}
	if which < 0 {
		which = -which
	}
	decl := shrinkStyles[which%len(shrinkStyles)]
	const viewport = 100000

	built := Build(Input{HTML: `<div id="f">` + text + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#f { float: left; font-family: Courier; font-size: 20px; ` + decl + ` }`}}})
	if built.Root == nil {
		return
	}
	w, _ := style.FromPx(viewport)
	h, _ := style.FromPx(10000)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)

	// Nothing unbreakable overflows a box that was wrapped around it. In a
	// viewport this wide the shrink-to-fit is the preferred width, which is at
	// least the widest run that cannot be broken — so a report here is the
	// intrinsic pass having measured something smaller than the fill did, and
	// the page has content over the edge of the box meant to hold it.
	for _, got := range rec.Findings() {
		if got.Rule == RuleUnbreakableOverflow {
			t.Fatalf("%q with %q: %s", text, decl, got.Message)
		}
	}

	f := fragmentFor(root, "f")
	if f == nil {
		// A float that produced no fragment is a legitimate answer for text
		// that collapsed to nothing.
		return
	}
	box := f.ContentRect().W
	if box <= 0 {
		return
	}

	lines := func(px float64) (int, bool) {
		css := fmt.Sprintf(`#f { font-family: Courier; font-size: 20px; `+
			`width: %.6fpx; %s }`, px, decl)
		in := Build(Input{HTML: `<div id="f">` + text + `</div>`,
			CSS: []Stylesheet{{Source: noDefaults + css}}})
		if in.Root == nil {
			return -1, false
		}
		r := NewRecorder(nil)
		g := fragmentFor(Layout(in.Root, Size{W: w, H: h}, in.Fonts, r), "f")
		if g == nil {
			return -1, false
		}
		spills := false
		for _, got := range r.Findings() {
			if got.Rule == RuleUnbreakableOverflow {
				spills = true
			}
		}
		return len(g.Lines), spills
	}

	wide, _ := lines(viewport)
	if wide < 0 {
		return
	}
	if got, _ := lines(box.Px()); got != wide {
		t.Fatalf("%q with %q: shrink-wrapped to %v it lays out in %d lines and "+
			"unbounded in %d, so the box is narrower than its own content",
			text, decl, box, got, wide)
	}
}

// TestTheSpacingThatHangsIsNotAnOverflow is the defect the target above found,
// pinned on its own.
//
// §8.2 puts a letter-spacing after the last character of a run as well as
// between them, and a line ending there hangs it: it is not width the line has
// to find, which is why the fill discounts it and why a box shrink-wrapped
// around the text is that much narrower than the run's declared width.
//
// The report did not discount it. A word that exactly filled the box wrapped
// around it was named as overflowing by the gap hanging off its end — the fill
// knew it fitted and the finding said it did not, which is the two halves of one
// question answered differently.
func TestTheSpacingThatHangsIsNotAnOverflow(t *testing.T) {
	const face = `font-family: Courier; font-size: 20px`
	built := Build(Input{HTML: `<div id="f">aaaaaaaaaa</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#f { float: left; ` + face + `; letter-spacing: 2px }`}}})
	w, _ := style.FromPx(100000)
	h, _ := style.FromPx(10000)
	rec := NewRecorder(nil)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	for _, f := range rec.Findings() {
		if f.Rule == RuleUnbreakableOverflow {
			t.Errorf("a word that fills the box wrapped around it was reported: %s",
				f.Message)
		}
	}

	// And the containment case: a word that really is wider than its box is
	// still named, or the change above would have turned the rule off.
	built = Build(Input{HTML: `<div id="f">aaaaaaaaaa</div>`,
		CSS: []Stylesheet{{Source: noDefaults +
			`#f { width: 20px; ` + face + `; letter-spacing: 2px }`}}})
	rec = NewRecorder(nil)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	var said bool
	for _, f := range rec.Findings() {
		if f.Rule == RuleUnbreakableOverflow {
			said = true
		}
	}
	if !said {
		t.Error("a word ten times its box's width was not reported at all")
	}
}

// TestTheIdeographGapInsideARightToLeftLineIsNotDiscounted is the second defect
// the target above found, pinned on its own.
//
// §8.1 puts an eighth of an em between an ideograph and a letter beside it, and
// the gap is added to the width of the run it follows — which is the run to the
// *visual left* of the boundary, because a run's glyphs are drawn from its
// origin rightwards whatever direction they read in, so width added to a run
// appears past its rightmost glyph.
//
// A line ends at that edge only where the run reads left to right. On a
// right-to-left line the last run is the leftmost one, so a gap on it sits
// between that run and the one before it — inside the line. The measure
// discounted it anyway, on the reading that a gap at a run's far edge always
// hangs off the end, and "㌱ب" in a float came out 24px wide around 26.5px of
// content: the Arabic letter drawn two and a half pixels past the left edge of
// the box that had been sized to hold it.
//
// The fixture is the same characters set both ways round, because the number the
// engine must not get wrong is a number the two directions share: the same
// glyphs with the same gap between them need the same room whichever way they
// are read. That is a stronger statement than either width on its own, and it is
// the one that failed — the left-to-right measurement was right the whole time.
func TestTheIdeographGapInsideARightToLeftLineIsNotDiscounted(t *testing.T) {
	const text = "㌱ب"
	w, _ := style.FromPx(100000)
	h, _ := style.FromPx(10000)
	width := func(decl string) style.Unit {
		built := Build(Input{HTML: `<div id="f">` + text + `</div>`,
			CSS: []Stylesheet{{Source: noDefaults +
				`#f { float: left; font-family: Courier; font-size: 20px; ` + decl + ` }`}}})
		f := fragmentFor(Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil)), "f")
		if f == nil {
			t.Fatalf("%q with %q laid out no float", text, decl)
		}
		return f.ContentRect().W
	}
	rtl, ltr := width("direction: rtl"), width("direction: ltr")
	if rtl != ltr {
		t.Errorf("%q shrink-wraps to %v read right to left and %v read left to "+
			"right; the same glyphs with the same gap need the same room", text, rtl, ltr)
	}
	// And the gap is in both numbers rather than out of both, which the equality
	// above cannot tell apart from the engine having stopped inserting it. Two
	// Courier characters at 20px are 12px each and §8.1's gap is an eighth of an
	// em: 26.5px, not 24.
	want, _ := style.FromPx(26.5)
	if ltr != want {
		t.Errorf("%q shrink-wraps to %v, want %v — the two characters and the "+
			"ideograph gap between them", text, ltr, want)
	}
	// The fuzz body's own question, on the fixture that failed it: at the width
	// the shrink-wrap chose, the text breaks into the lines it breaks into
	// unbounded.
	for i, decl := range shrinkStyles {
		if decl == "direction: rtl" {
			checkShrinkToFit(t, text, i)
		}
	}
}
