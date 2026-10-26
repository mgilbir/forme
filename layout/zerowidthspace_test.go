package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A zero-width space between two spaces, which §4.1.1 does not collapse.
//
// The rule collapses white space that is *adjacent*: "any collapsible space
// immediately following another collapsible space is collapsed to have zero
// advance width". A zero-width space is not white space — it is a character —
// so two spaces with one between them are not immediately following anything
// and both survive.
//
// The suite states it as an equivalence in a comment of its own, "U+00A0 is
// exactly equivalent to U+200B U+0020 U+200B", and tests it four times: a cell
// holding five rows that must each be five characters wide, one row written with
// three no-break spaces and two written with spaces fenced by zero-width ones.
//
// The character was dropped outright — "a break opportunity and nothing else" —
// so by the time the collapsing ran the spaces were adjacent and two of the
// three vanished. The rows came out three characters wide inside a five-character
// box, which is a red square showing through a green one.

// spanWidth is the width of the background of the one span in a document, which
// is the width of the content inside it.
func spanWidth(t *testing.T, inner string) style.Unit {
	t.Helper()
	got := fillsOf(paintOf(t, `<div id="d">X<span id="s">`+inner+`</span>X</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }
		 #s { background: rgb(0,128,0) }`), green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds for %q: %v", len(got), inner, got)
	}
	return got[0].W
}

// TestAZeroWidthSpaceKeepsTheSpacesEitherSideOfItApart is the suite's
// equivalence, measured.
func TestAZeroWidthSpaceKeepsTheSpacesEitherSideOfItApart(t *testing.T) {
	// Three no-break spaces: three characters, and nothing collapses because a
	// no-break space is not white space for this purpose at all.
	want := spanWidth(t, "\u00a0\u00a0\u00a0")
	if want != bgpx(36) {
		t.Fatalf("three no-break spaces are %v wide, want 36 — three 12px characters",
			want)
	}
	for _, tc := range []struct{ what, inner string }{
		{"spaces fenced on both sides", "​ ​ ​ ​"},
		{"spaces fenced between them", " ​ ​ "},
	} {
		if got := spanWidth(t, tc.inner); got != want {
			t.Errorf("%s: %v wide, and three no-break spaces are %v; the suite calls "+
				"the two spellings exactly equivalent", tc.what, got, want)
		}
	}
}

// TestAdjacentSpacesStillCollapse is the containment case, and it is the rule
// itself: with nothing between them, three spaces are one.
func TestAdjacentSpacesStillCollapse(t *testing.T) {
	if got := spanWidth(t, "   "); got != bgpx(12) {
		t.Errorf("three adjacent spaces are %v wide, want 12 — they collapse to one",
			got)
	}
	// And across an inline boundary, which is the half of the rule that needs
	// the state to travel between boxes. It is measured between the two Xs
	// rather than by the first span's background: the span that loses its space
	// is the *second* one, and the first is 12px wide either way.
	if got := xSpan(t, `X<span> </span><span> </span>X`); got != bgpx(24) {
		t.Errorf("a space in one span followed by a space in another put the two Xs "+
			"%v apart, want 24 — one for the X and one for the single space they "+
			"collapse to", got)
	}
}

// xSpan is the distance between the first and last runs of text a document set.
func xSpan(t *testing.T, markup string) style.Unit {
	t.Helper()
	var xs []style.Unit
	for _, op := range paintOf(t, `<div id="d">`+markup+`</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }`) {
		if v, ok := op.(DrawText); ok {
			xs = append(xs, v.At.X)
		}
	}
	if len(xs) < 2 {
		t.Fatalf("%d runs of text for %q", len(xs), markup)
	}
	return xs[len(xs)-1].Sub(xs[0])
}

// TestAZeroWidthSpaceInABoxOfItsOwnStillSeparates. The character may be the
// whole content of an element, which is how it is written when a stylesheet
// rather than the prose decides where a word may break — and the state has to
// cross that boundary for the rule to see it.
func TestAZeroWidthSpaceInABoxOfItsOwnStillSeparates(t *testing.T) {
	got := fillsOf(paintOf(t,
		`<div id="d">X<span id="s"> </span><span>​</span><span> </span>X</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }
		 #s { background: rgb(0,128,0) }`), green)
	if len(got) != 1 {
		t.Fatalf("%d span backgrounds: %v", len(got), got)
	}
	if got[0].W != bgpx(12) {
		t.Errorf("the first span is %v wide, want 12", got[0].W)
	}
	// The second space survived, so the run from the first X to the last is
	// three characters rather than two.
	if got := xSpan(t, `X<span> </span><span>​</span><span> </span>X`); got != bgpx(36) {
		t.Errorf("the two Xs are %v apart, want 36 — one for the X and two for the "+
			"spaces the zero-width space kept apart", got)
	}
}

// A zero-width space that suppresses a segment break from the node before.
//
// §4.1.1 removes a segment break whose neighbouring character is a zero-width
// space: an author who hard-wrapped their source and marked the wrap meant the
// break opportunity and not a space as well. The rule is written over
// *characters*, and css-text-4 says what that means where markup gets in the
// way — "intervening inline box boundaries must be ignored".
//
// So "aa&#x200b;<span></span>\nbb" is the same text as "aa&#x200b;\nbb" and must
// come out the same way. It did not: Phase I ran per text node and each node
// began with no idea what preceded it, so the mark in one node did nothing about
// the break in the next and a space appeared in the middle of a word.

// lineTextOf is the one line a wide block puts its text on.
func lineTextOf(t *testing.T, htmlSrc string) string {
	t.Helper()
	got := lineTextsOf(t, layoutOf(t, 600, `<div id="d">`+htmlSrc+`</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }`), "d")
	if len(got) != 1 {
		t.Fatalf("%q made %d lines %q, want 1", htmlSrc, len(got), got)
	}
	return got[0]
}

// TestABoxBoundaryDoesNotHideTheMarkFromTheBreak is the rule, and the table is
// the boundaries css-text-4 names: a box that opens around the mark, a box that
// closes before it, and an empty one between the two.
func TestABoxBoundaryDoesNotHideTheMarkFromTheBreak(t *testing.T) {
	for _, src := range []string{
		"aa&#8203;\nbb",                     // one node: this always worked
		"aa<span>&#8203;</span>\nbb",        // the mark in a box of its own
		"aa<span></span>&#8203;\nbb",        // an empty box before the mark
		"aa&#8203;<span></span>\nbb",        // an empty box between mark and break
		"aa<span><b>&#8203;</b></span>\nbb", // two boxes deep
	} {
		if got := lineTextOf(t, src); got != "aabb" {
			t.Errorf("%q came out %q, want \"aabb\": the zero-width space is the "+
				"character before the break however many boxes are written around it", src, got)
		}
	}
}

// TestAnOutOfFlowBoxIsNotBetweenThem. An overlay hung off the middle of a
// paragraph is not in the paragraph's text: nothing of it sits between the
// characters either side, so it is neither the character before the break nor
// the character after the mark.
//
// seg-break-transformation-019 writes it four ways, and this is those four.
func TestAnOutOfFlowBoxIsNotBetweenThem(t *testing.T) {
	for _, css := range []string{
		"position:absolute", "position:fixed", "float:right", "float:left",
	} {
		src := `aa&#8203;<span style="` + css + `">foo</span>` + "\nbb"
		if got := lineTextOf(t, src); got != "aabb" {
			t.Errorf("with %q between them the text is %q, want \"aabb\": the "+
				"box is not in the text either side of it", css, got)
		}
	}
}

// TestTextInAnOrdinaryBoxIsStillTextBetweenThem is the containment. A box that
// is *in* the flow and holds a character puts that character between the mark
// and the break, and then there is no mark before the break at all.
func TestTextInAnOrdinaryBoxIsStillTextBetweenThem(t *testing.T) {
	for _, src := range []string{
		"aa<span>x</span>\nbb",
		"aa&#8203;<span>x</span>\nbb",
	} {
		if got := lineTextOf(t, src); got != "aax bb" {
			t.Errorf("%q came out %q, want \"aax bb\": the character before the "+
				"break is the \"x\", so the break becomes a space", src, got)
		}
	}
}

// TestTheEastAsianRuleCrossesABoundaryToo. §4.1.1's other exception is about the
// last character a *reader* would see rather than the last one written, and it
// travels across a box boundary for the same reason and by the same route.
//
// A paragraph of Japanese hard-wrapped in the source gains a space at the end of
// every line it was wrapped at, in the middle of words, all through the text —
// which is the most visible thing an engine can get wrong about CJK, and wrong
// in the direction that looks deliberate.
func TestTheEastAsianRuleCrossesABoundaryToo(t *testing.T) {
	if got := lineTextOf(t, "漢<span></span>\n字"); got != "漢字" {
		t.Errorf("a break between two ideographs written either side of a box came "+
			"out %q, want \"漢字\"", got)
	}
	// And the containment: a Latin letter before the break is not an ideograph,
	// so the break is a space however the boxes fall.
	if got := lineTextOf(t, "漢x<span></span>\n字"); got != "漢x 字" {
		t.Errorf("a break after a Latin letter came out %q, want \"漢x 字\"", got)
	}
	// A variation selector written after the ideograph is not the character
	// before the break: it is default-ignorable, nothing is drawn for it, and
	// the rule is about what a reader would see. The suite tests that within one
	// node — segment-break-transformation-ignorable-1 — and it has to hold
	// across a boundary for the same reason everything else here does.
	if got := lineTextOf(t, "漢\ufe00<span></span>\n字"); got != "漢\ufe00字" {
		t.Errorf("a break after an ideograph and its variation selector came out "+
			"%q, want the two ideographs with no space", got)
	}
}

// TestTheCharacterAfterTheBreakCrossesABoundaryToo is the other half of the two
// tests above. They carry the character *before* a break across a box boundary;
// this is the character after it, which a node ending in the break cannot see.
//
// FuzzBoundaryLines found it: "あ\n㛂" set as one line of two ideographs, and the
// same text with the newline in a <span> of its own set a space between them.
// Phase I decides a run of white space where the run ends, and a run that ends
// with its node ended before the ideograph that removes the break, so it fell
// back to the space. "漢\n<span>字</span>" is the everyday spelling — Japanese
// hard-wrapped in the source with an element starting the next line — and it
// gained a space in the middle of a word at every such line. See
// paragraph.OpenRun.
func TestTheCharacterAfterTheBreakCrossesABoundaryToo(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"漢\n<span>字</span>", "漢字"},               // the next character in a box
		{"漢\n<span></span>字", "漢字"},               // an empty box after the break
		{"漢<span>\n</span>字", "漢字"},               // the break in a box of its own
		{"漢 <span>\n</span>字", "漢字"},              // a run begun before the box and broken inside it
		{"漢\n<span> </span>字", "漢字"},              // a box of nothing but the run's spaces
		{"漢\n<span>\n</span><b>字</b>", "漢字"},      // two boxes on, and two breaks
		{"<span>漢\n</span><span>字</span>", "漢字"},  // a box closing after the break
		{"漢\n&#x202a;<span>字</span>", "漢\u202a字"}, // a control in the run is not between them
		// An overlay is not in the text either side of it, and the text inside
		// it does not decide the run it was hung off.
		{"漢\n<span style=\"position: absolute\">x</span>字", "漢字"},
		{"漢\n<span style=\"float: left\">x</span>字", "漢字"},
	} {
		if got := lineTextOf(t, tc.src); got != tc.want {
			t.Errorf("%q came out %q, want %q: the character after the break is "+
				"the ideograph however many boxes are written around it", tc.src, got, tc.want)
		}
	}
	// The zero-width space is the rule's other neighbour, and it is decided the
	// same way: a mark after the break, in the next box, takes the break away.
	if got := lineTextOf(t, "aa\n<span>&#8203;bb</span>"); got != "aabb" {
		t.Errorf("a break before a zero-width space in the next box came out %q, "+
			"want \"aabb\"", got)
	}
	// The containment: a Latin letter after the break is not an ideograph, so
	// the break is a space however the boxes fall.
	if got := lineTextOf(t, "漢\n<span>x</span>"); got != "漢 x" {
		t.Errorf("a break before a Latin letter came out %q, want \"漢 x\"", got)
	}
}

// TestAnAtomicInlineEndsTheRunBesideIt. A picture or an inline-block after a
// break is what the break is beside, and the text past it is not the character
// after the break. css-text-4's "intervening inline box boundaries must be
// ignored" is about inline boxes, and an atomic inline is the inline-level box
// that is not one. See standsAsAnObject.
func TestAnAtomicInlineEndsTheRunBesideIt(t *testing.T) {
	for _, src := range []string{
		`漢` + "\n" + `<img style="width: 10px; height: 10px">字`,
		`漢` + "\n" + `<span style="display: inline-block"></span>字`,
	} {
		if got := lineTextOf(t, src); got != "漢 字" {
			t.Errorf("%q came out %q, want \"漢 字\": the break is beside the "+
				"atomic inline and not the ideograph past it", src, got)
		}
	}
	// And the text inside one is not after the break either.
	got := lineTextsOf(t, layoutOf(t, 600,
		`<div id="d">漢`+"\n"+`<span style="display: inline-block">字</span></div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px }`), "d")
	if len(got) != 1 || got[0] != "漢 " {
		t.Errorf("a break before an inline-block holding an ideograph left the line %q, "+
			"want [\"漢 \"]: the ideograph inside it is not after the break", got)
	}
}

// TestAnAtomicInlineIsTheCharacterBeforeTheBreak is the other way round from the
// test above. An atomic inline is one U+FFFC in the text, so a break after one
// is beside the picture, and the ideograph or the zero width space before the
// picture is not the character before the break. Blink builds its text the same
// way and decides the break from that character. See standsAsAnObject.
func TestAnAtomicInlineIsTheCharacterBeforeTheBreak(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`漢<img style="width: 10px; height: 10px">` + "\n字", "漢 字"},
		{`漢<span style="display: inline-block"></span>` + "\n字", "漢 字"},
		{`aa&#8203;<img style="width: 10px; height: 10px">` + "\nbb", "aa bb"},
		// The break in a box of its own, after the picture.
		{`漢<img style="width: 10px; height: 10px"><span>` + "\n字</span>", "漢 字"},
		// The text inside an inline-block is not before the break either.
		{`<span style="display: inline-block">漢</span>` + "\n字", " 字"},
		// The containment: an inline box is not an atomic inline, and the
		// ideograph before an empty one is still the character before the break.
		{"漢<span></span>\n字", "漢字"},
	} {
		if got := lineTextOf(t, tc.src); got != tc.want {
			t.Errorf("%q came out %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestASpaceAfterAnAtomicInlineIsNotTheSpaceBeforeIt. The picture is between the
// two spaces of "a <img> b", so they are two runs of white space and not one:
// the flattening keeps both. Under "full-width" the spaces become U+3000 before
// the flattening sees them, and the bit that says a run continues from the node
// before was carried through the picture, so the second was dropped.
func TestASpaceAfterAnAtomicInlineIsNotTheSpaceBeforeIt(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{`a <img style="width: 10px; height: 10px"> b`, "a  b"},
		{`<span style="text-transform: full-width">a <img style="width: 10px; height: 10px"> b</span>`,
			"ａ　　ｂ"},
		// The containment: across an inline box it is one run, and one space.
		{`<span style="text-transform: full-width">a <span></span> b</span>`, "ａ　ｂ"},
	} {
		if got := lineTextOf(t, tc.src); got != tc.want {
			t.Errorf("%q came out %q, want %q", tc.src, got, tc.want)
		}
	}
	// And the text inside an inline-block begins afresh, as a block's does: its
	// first space is not the second half of the run before the box.
	got := lineTextsOf(t, layoutOf(t, 600,
		`<div id="d">a <span id="ib" style="display: inline-block"> b</span></div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px; text-transform: full-width }`), "ib")
	block := lineTextsOf(t, layoutOf(t, 600, `<div id="d"> b</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px; text-transform: full-width }`), "d")
	if len(got) != 1 || len(block) != 1 || got[0] != block[0] {
		t.Errorf("an inline-block after a space set %q, want what a block sets, %q", got, block)
	}
}

// TestTheLangAttributeReachesTheSegmentBreakRule. §4.1.1's second sentence asks
// what writing system the break is in, and the answer is an HTML attribute
// rather than a CSS property — read off the nearest element at or above the
// text, the same walk :lang() does.
//
// writing-system-segment-break-001 writes lang="ain-Kana": Ainu, which is not
// Japanese, in katakana, which is.
func TestTheLangAttributeReachesTheSegmentBreakRule(t *testing.T) {
	for _, tc := range []struct{ what, html, want string }{
		{"katakana named by a script subtag",
			"<div id=\"d\" lang=\"ain-Kana\">“\nア</div>", "“ア"},
		{"the language on its own",
			"<div id=\"d\" lang=\"ja\">“\nア</div>", "“ア"},
		{"the attribute on an ancestor",
			"<section lang=\"ja\"><div id=\"d\">“\nア</div></section>", "“ア"},
		{"a document that says nothing about its language",
			"<div id=\"d\">“\nア</div>", "“ ア"},
		{"a writing system the sentence does not name",
			"<div id=\"d\" lang=\"en\">“\nア</div>", "“ ア"},
		{"Japanese romanised, which is written with spaces",
			"<div id=\"d\" lang=\"ja-Latn\">“\nア</div>", "“ ア"},
	} {
		got := lineTextsOf(t, layoutOf(t, 600, tc.html,
			`#d { font-family: Courier; font-size: 20px; width: 400px }`), "d")
		if len(got) != 1 || got[0] != tc.want {
			t.Errorf("%s: set %q, want [%q]", tc.what, got, tc.want)
		}
	}
}
