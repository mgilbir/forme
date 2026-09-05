package layout

import (
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// text-autospace laid out, CSS Text 4 §8.1.
//
// Courier at 16px, so an eighth of the em is 2px and a character is 9.59375 —
// which is why every assertion below is about a *difference* between two
// renderings rather than about an absolute number: the gap is the thing under
// test and the advance of a Han character in a Latin face is not.

// autospaceRuns is where each run of a box begins and how wide it is.
func autospaceRuns(t *testing.T, htmlSrc, extra string) []TextRun {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, noDefaults+
		`div, span { font-family: Courier; font-size: 16px }`+extra)
	var out []TextRun
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				for _, ln := range f.Lines {
					out = append(out, ln.Runs...)
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// contentWidth is how far the runs of a box reach.
func contentWidth(runs []TextRun) style.Unit {
	var end style.Unit
	for _, r := range runs {
		if e := r.X.Add(r.Width); e > end {
			end = e
		}
	}
	return end
}

// eighthOfAnEm at 16px.
func eighthOfAnEm(t *testing.T) style.Unit {
	t.Helper()
	u, ok := style.FromPx(2)
	if !ok {
		t.Fatal("2px is not representable")
	}
	return u
}

// TestAnIdeographBesideALetterIsSpaced is the property, and the count of
// boundaries is the assertion.
//
// "国国AA国国" has two of them. The same text with the property turned off has
// none, and the difference between the two renderings is exactly two eighths of
// an em — which is what the suite's own reference writes by hand, as
// "margin: calc(1em / 8)" on a span around the Latin.
func TestAnIdeographBesideALetterIsSpaced(t *testing.T) {
	on := autospaceRuns(t, `<div id="d">国国AA国国</div>`, ``)
	off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">国国AA国国</div>`, ``)
	gap := eighthOfAnEm(t)
	if got, want := contentWidth(on).Sub(contentWidth(off)), gap.Mul(2); got != want {
		t.Errorf("the spaced text is %v wider and there are two boundaries in it at "+
			"%v each, so it should be %v", got, gap, want)
	}
	// And the text is unchanged: the gap is spacing, not a character, so
	// nothing is added to what a reader copies out of the page.
	if got, want := autospaceText(on), autospaceText(off); got != want {
		t.Errorf("the spaced text reads %q and the plain one %q", got, want)
	}
	if want := "国国AA国国"; autospaceText(on) != want {
		t.Errorf("the text reads %q, want %q", autospaceText(on), want)
	}
}

// autospaceText is what the runs of a box read, joined.
func autospaceText(runs []TextRun) string {
	var b strings.Builder
	for _, r := range runs {
		b.WriteString(r.Text)
	}
	return b.String()
}

// TestEachBoundaryGetsOneGapAndOnlyOne. The rule is about a boundary, not about
// a run: a Latin word between two ideographs is spaced on both sides, and a
// document with no boundary in it is not spaced at all.
func TestEachBoundaryGetsOneGapAndOnlyOne(t *testing.T) {
	gap := eighthOfAnEm(t)
	for _, tc := range []struct {
		text string
		gaps int
	}{
		{"国A国", 2},
		{"国国AA国国", 2},
		{"A国", 1},
		{"国A", 1},
		{"国12国", 2},
		{"AA国国AA国国", 3},
		// Nothing at all: no ideograph, or no other side to one.
		{"AAAA", 0},
		{"国国国国", 0},
		// A space or a punctuation mark between them is neither class, so the
		// two characters are not adjacent and there is no boundary.
		{"国 A", 0},
		{"国,A", 0},
		{"国、A", 0},
	} {
		on := autospaceRuns(t, `<div id="d">`+tc.text+`</div>`, ``)
		off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">`+tc.text+`</div>`, ``)
		got := contentWidth(on).Sub(contentWidth(off))
		if want := gap.Mul(float64(tc.gaps)); got != want {
			t.Errorf("%q: the spacing added %v, want %d gaps of %v = %v",
				tc.text, got, tc.gaps, gap, want)
		}
	}
}

// TestTheTwoClassesAreAskedForSeparately, through the declaration rather than
// through the parser, so that the value reaches the layout.
func TestTheTwoClassesAreAskedForSeparately(t *testing.T) {
	gap := eighthOfAnEm(t)
	for _, tc := range []struct {
		value string
		text  string
		gaps  int
		what  string
	}{
		{"ideograph-alpha", "国A国", 2, "letters are spaced"},
		{"ideograph-alpha", "国1国", 0, "and digits are not"},
		{"ideograph-numeric", "国1国", 2, "digits are spaced"},
		{"ideograph-numeric", "国A国", 0, "and letters are not"},
		{"normal", "国A国", 2, "normal asks for both"},
		{"normal", "国1国", 2, "both of them"},
	} {
		on := autospaceRuns(t, `<div id="d" style="text-autospace:`+tc.value+`">`+tc.text+`</div>`, ``)
		off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">`+tc.text+`</div>`, ``)
		got := contentWidth(on).Sub(contentWidth(off))
		if want := gap.Mul(float64(tc.gaps)); got != want {
			t.Errorf("%s (%s, %q): added %v, want %v", tc.what, tc.value, tc.text, got, want)
		}
	}
}

// TestABoundaryAcrossTwoBoxesIsSpacedToo, which is the case the run-splitting
// cannot reach and the pass over the finished items is for.
func TestABoundaryAcrossTwoBoxesIsSpacedToo(t *testing.T) {
	gap := eighthOfAnEm(t)
	for _, tc := range []struct {
		html string
		gaps int
		what string
	}{
		{`国<span>AA</span>国`, 2, "the Latin in a span of its own"},
		{`<span>国</span>AA<span>国</span>`, 2, "the ideographs in spans"},
		{`国<b>A</b><i>A</i>国`, 2, "three boxes, and the middle boundary is A|A"},
	} {
		on := autospaceRuns(t, `<div id="d">`+tc.html+`</div>`, ``)
		off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">`+tc.html+`</div>`, ``)
		got := contentWidth(on).Sub(contentWidth(off))
		if want := gap.Mul(float64(tc.gaps)); got != want {
			t.Errorf("%s: added %v, want %d gaps = %v", tc.what, got, tc.gaps, want)
		}
	}
}

// TestTheInnermostElementContainingBothDecides.
//
// The suite's text-autospace-elements-001 is the fixture and it is the one that
// says this is not "either character's own value": a div that turned the
// property off holds a span that turned it on, and the boundaries *inside* the
// span are spaced, because the span is the innermost element containing both
// sides of them.
func TestTheInnermostElementContainingBothDecides(t *testing.T) {
	gap := eighthOfAnEm(t)
	inner := autospaceRuns(t,
		`<div id="d" style="text-autospace:no-autospace">`+
			`<span style="text-autospace:normal">国国AA国国</span></div>`, ``)
	off := autospaceRuns(t,
		`<div id="d" style="text-autospace:no-autospace">`+
			`<span style="text-autospace:no-autospace">国国AA国国</span></div>`, ``)
	if got, want := contentWidth(inner).Sub(contentWidth(off)), gap.Mul(2); got != want {
		t.Errorf("the span asked for the spacing and got %v of it, want %v: the span "+
			"is what contains both sides of each boundary", got, want)
	}

	// And the other way round, which is the half that says the *outer* value is
	// not simply ignored: a boundary between a character in the span and one
	// outside it belongs to the div, which turned the property off.
	across := autospaceRuns(t,
		`<div id="d" style="text-autospace:no-autospace">`+
			`国<span style="text-autospace:normal">AA</span>国</div>`, ``)
	acrossOff := autospaceRuns(t,
		`<div id="d" style="text-autospace:no-autospace">`+
			`国<span style="text-autospace:no-autospace">AA</span>国</div>`, ``)
	if got := contentWidth(across).Sub(contentWidth(acrossOff)); got != 0 {
		t.Errorf("a boundary between the span's text and the div's was spaced by %v; "+
			"the innermost element containing both is the div, which turned it off", got)
	}
}

// TestAMarkDoesNotBreakTheBoundary. A variation selector after an ideograph
// leaves an ideograph, and a combining acute over a letter leaves a letter — so
// the boundary is where it would have been without them.
func TestAMarkDoesNotBreakTheBoundary(t *testing.T) {
	gap := eighthOfAnEm(t)
	for _, tc := range []struct{ what, text string }{
		{"a variation selector", "国︀A"},
		{"one from the supplement", "国\U000E0100A"},
		{"a combining mark on the letter", "国át"},
	} {
		on := autospaceRuns(t, `<div id="d">`+tc.text+`</div>`, ``)
		off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">`+tc.text+`</div>`, ``)
		got := contentWidth(on).Sub(contentWidth(off))
		// Within one layout unit, which is a sixty-fourth of a pixel.
		//
		// A mark does not begin a soft wrap opportunity, so "国<VS>A" arrives as
		// one run and is cut at the boundary to make room for the gap — and two
		// runs measured apart can round differently from one measured whole. It
		// is the same rounding the engine already accepts wherever a run is cut
		// for a change of face, and it is a sixtieth of the gap being asserted.
		if d := got.Sub(gap); d > 1 || d < -1 {
			t.Errorf("%s: the boundary added %v, want one gap of %v", tc.what, got, gap)
		}
	}
}

// TestADocumentWithNoIdeographsIsUntouched is the containment case, and the one
// that matters most: the property's initial value asks for the spacing, so this
// walk runs over every paragraph of every document in the corpus and must change
// nothing about the ones with no ideograph in them.
func TestADocumentWithNoIdeographsIsUntouched(t *testing.T) {
	for _, text := range []string{
		"The quick brown fox jumps over the lazy dog",
		"one two three",
		"123 456",
		"a-b, c. d! e?",
	} {
		on := autospaceRuns(t, `<div id="d">`+text+`</div>`, ``)
		off := autospaceRuns(t, `<div id="d" style="text-autospace:no-autospace">`+text+`</div>`, ``)
		if len(on) != len(off) {
			t.Errorf("%q was cut into %d runs with the property on and %d with it off",
				text, len(on), len(off))
			continue
		}
		for i := range on {
			if on[i].X != off[i].X || on[i].Width != off[i].Width {
				t.Errorf("%q run %d moved from %v+%v to %v+%v",
					text, i, off[i].X, off[i].Width, on[i].X, on[i].Width)
			}
		}
	}
}

// TestTheGapIsSizedByTheIdeographsFont.
//
// The suite writes its references as "0.125ic" on the element holding the
// ideographs, so the measure is the ideograph's and not the letter's. A document
// that sets its Latin larger than its Japanese would otherwise get a gap sized
// to the wrong one — and the fixture below is that document, twice over, with
// the two sizes exchanged.
func TestTheGapIsSizedByTheIdeographsFont(t *testing.T) {
	measure := func(htmlSrc string) style.Unit {
		t.Helper()
		on := autospaceRuns(t, htmlSrc, ``)
		off := autospaceRuns(t, strings.Replace(htmlSrc, `id="d"`,
			`id="d" style="text-autospace:no-autospace"`, 1), ``)
		return contentWidth(on).Sub(contentWidth(off))
	}
	// The ideographs at 16px and the Latin at 32px: one gap of 16/8 = 2px.
	small := measure(`<div id="d">国<span style="font-size:32px">A</span></div>`)
	// The sizes exchanged: one gap of 32/8 = 4px.
	large := measure(`<div id="d"><span style="font-size:32px">国</span>A</div>`)
	want2, _ := style.FromPx(2)
	want4, _ := style.FromPx(4)
	if small != want2 {
		t.Errorf("with the ideograph at 16px the gap is %v, want %v", small, want2)
	}
	if large != want4 {
		t.Errorf("with the ideograph at 32px the gap is %v, want %v", large, want4)
	}
}

// TestTheGapCountsTowardsAnIntrinsicWidth. A box shrink-wrapped around its
// content has to be wide enough for the spacing as well, or the text it was
// sized to hold overflows it by an eighth of an em per boundary.
func TestTheGapCountsTowardsAnIntrinsicWidth(t *testing.T) {
	width := func(css string) style.Unit {
		t.Helper()
		root := layoutOf(t, 600, `<div id="d" style="float:left; `+css+`">国AA国</div>`,
			noDefaults+`div { font-family: Courier; font-size: 16px }`)
		return find(t, root, "d").ContentRect().W
	}
	on := width(``)
	off := width(`text-autospace:no-autospace`)
	if got, want := on.Sub(off), eighthOfAnEm(t).Mul(2); got != want {
		t.Errorf("the shrink-to-fit width grew by %v, want %v — two boundaries of "+
			"an eighth of an em", got, want)
	}
}

// The gap goes where a reader sees the boundary, which after the bidi
// resolution is not where the text says it is.
//
// §8.1's spacing is between two characters, and the two characters a reader sees
// side by side at the edge of a right-to-left word are not the ones the text has
// there: the word's *last* letter is at its visual left and its first is at its
// right. A gap added to the run that logically precedes the boundary therefore
// lands inside the word rather than beside it — but only when the word is split
// across an inline boundary, because a word in one run has no inside for it to
// land in.
//
// The assertions are on the *glyphs* and not on the runs, and that is the whole
// difficulty. A gap is added to a run's width, so a run whose gap is in the
// wrong place still abuts its neighbour and still ends where it should: the run
// boxes agree in both readings and only the letters inside them move. It is the
// warning forme's letter-spacing notes give in as many words — check the display
// list, not the run box.

// rtlOverride turns the letters inside an <i> right to left without needing a
// right-to-left script, so these fixtures stay in Courier with the rest.
const rtlOverride = ` #d i { direction: rtl; unicode-bidi: bidi-override }`

// splitSlack is how far a letter may move when the run holding it is cut in
// two, and it is one layout unit: the two halves are measured separately and
// each rounds its own advance, where the whole word rounded once. It is the
// smallest difference the engine can represent, and the gap this file is about
// is a hundred and twenty-eight of them.
const splitSlack = style.Unit(1)

func within(a, b, slack style.Unit) bool {
	if a > b {
		a, b = b, a
	}
	return b.Sub(a) <= slack
}

// letterMarks is where every non-Han glyph of a document is drawn, left to
// right.
func letterMarks(t *testing.T, htmlSrc, extra string) []style.Unit {
	t.Helper()
	ops := paintOf(t, htmlSrc, noDefaults+
		`div, span, i { font-family: Courier; font-size: 16px }`+extra)
	var out []style.Unit
	for _, op := range ops {
		v, ok := op.(DrawText)
		if !ok || strings.ContainsRune(v.Text, '\u56fd') {
			continue
		}
		for _, m := range glyphMarks(v, "run", "run", false) {
			out = append(out, m.x)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TestASplitRightToLeftWordIsSpacedAtItsEdges.
//
// The same three letters, once in one run and once cut in two by a span. Both
// documents draw the same picture — a gap on each side of the word and none
// inside it — so every letter is asserted against the undivided document rather
// than against a number, and a gap that moved shows up as a letter that moved.
func TestASplitRightToLeftWordIsSpacedAtItsEdges(t *testing.T) {
	whole := letterMarks(t, `<div id="d">国<i>abc</i>国</div>`, rtlOverride)
	split := letterMarks(t, `<div id="d">国<i><span>ab</span>c</i>国</div>`, rtlOverride)
	if len(whole) != 3 || len(split) != 3 {
		t.Fatalf("the fixtures drew %d and %d letters, want 3 each", len(whole), len(split))
	}
	for i := range whole {
		if !within(split[i], whole[i], splitSlack) {
			t.Errorf("letter %d is at %v where the undivided word puts it at %v; "+
				"splitting a right-to-left word across a span must not move a "+
				"letter, and a gap opened at the wrong end of it does",
				i, split[i], whole[i])
		}
	}
	// The fixture really is split, and really is right to left: two runs, and
	// the one holding the *first* two letters is the one further right.
	var runs []DrawText
	for _, op := range paintOf(t, `<div id="d">国<i><span>ab</span>c</i>国</div>`,
		noDefaults+`div, span, i { font-family: Courier; font-size: 16px }`+rtlOverride) {
		if v, ok := op.(DrawText); ok && !strings.ContainsRune(v.Text, '\u56fd') {
			runs = append(runs, v)
		}
	}
	if len(runs) != 2 {
		t.Fatalf("the split fixture drew %d Latin runs, want 2 — it cannot say "+
			"what it means to say", len(runs))
	}
	ab, c := runs[0], runs[1]
	if ab.Text != "ab" {
		ab, c = c, ab
	}
	if !ab.RTL || ab.At.X <= c.At.X {
		t.Fatalf("the fixture drew %q at %v and %q at %v (rtl=%v); it is not the "+
			"reordered pair this test is about", ab.Text, ab.At.X, c.Text, c.At.X, ab.RTL)
	}
}

// TestALeftToRightDocumentIsSpacedWhereItAlwaysWas is the control. Visual order
// and logical order are the same thing there, so the change that made the test
// above pass must move nothing at all.
func TestALeftToRightDocumentIsSpacedWhereItAlwaysWas(t *testing.T) {
	whole := letterMarks(t, `<div id="d">国abc国</div>`, "")
	split := letterMarks(t, `<div id="d">国<span>ab</span>c国</div>`, "")
	if len(whole) != 3 || len(split) != 3 {
		t.Fatalf("the fixtures drew %d and %d letters, want 3 each", len(whole), len(split))
	}
	for i := range whole {
		if !within(split[i], whole[i], splitSlack) {
			t.Errorf("letter %d is at %v and %v; in a left-to-right document a "+
				"span changes nothing", i, split[i], whole[i])
		}
	}
}

// TestAForcedBreakIsTwoParagraphsAndTwoOrderings.
//
// The items of a block span as many bidi paragraphs as it has forced breaks in
// it, and the reordering is per paragraph. One order taken over two of them
// gives the second paragraph's items no levels at all — they fall outside the
// first paragraph's extent — and they then inherit whatever the item before them
// had, which is the *first* paragraph's direction. A right-to-left line after a
// left-to-right one is laid out as though it were left to right, and its gaps go
// back where the text says rather than where they are seen.
//
// The first line holds no letters, so every letter below belongs to the second
// and can be compared against the same line standing alone.
func TestAForcedBreakIsTwoParagraphsAndTwoOrderings(t *testing.T) {
	alone := letterMarks(t, `<div id="d">国<i><span>ab</span>c</i>国</div>`, rtlOverride)
	after := letterMarks(t, `<div id="d">国国<br>国<i><span>ab</span>c</i>国</div>`, rtlOverride)
	if len(alone) != 3 || len(after) != 3 {
		t.Fatalf("the fixtures drew %d and %d letters, want 3 each — the first "+
			"line of the second must hold none", len(alone), len(after))
	}
	for i := range alone {
		if !within(after[i], alone[i], splitSlack) {
			t.Errorf("letter %d is at %v after a forced break and at %v without "+
				"one; the break makes a second paragraph and it is ordered on "+
				"its own", i, after[i], alone[i])
		}
	}
}

// TestWhichEndOfARightToLeftRunDecides.
//
// The two characters at a boundary are the ones a reader sees there, so for a
// right-to-left run it is the *first* character of the text that stands at the
// run's right-hand end and the last that stands at its left. Where both ends
// are letters that distinction changes nothing, which is why the tests above
// cannot see it: this fixture puts a numeral at one end and a letter at the
// other, and asks for "ideograph-alpha" alone so that only one of the two is a
// boundary at all.
//
// Written "国<i>1ab</i>国" and read "国 b a 1 国": the left-hand boundary is
// 国 against a letter and gets a gap, the right-hand one is a numeral against
// 国 and gets none. Reading the run's ends in text order gives exactly the
// opposite — one gap still, at the other end of the word — so the line comes
// out the same width and every letter is in the wrong place.
func TestWhichEndOfARightToLeftRunDecides(t *testing.T) {
	const doc = `<div id="d">国<i>1ab</i>国</div>`
	const alpha = rtlOverride + ` #d { text-autospace: ideograph-alpha }`
	const none = rtlOverride + ` #d { text-autospace: no-autospace }`

	spaced := letterMarks(t, doc, alpha)
	plain := letterMarks(t, doc, none)
	if len(spaced) != 3 || len(plain) != 3 {
		t.Fatalf("the fixtures drew %d and %d letters, want 3 each", len(spaced), len(plain))
	}
	// An eighth of a 16px em is 2px, and it goes in front of the letters
	// because the letter is what stands at the left of the run.
	if got := spaced[0].Sub(plain[0]).Px(); got != 2 {
		t.Errorf("the first letter moved %gpx when the spacing was turned on, want "+
			"2 — the boundary at the left of the run is 国 against a letter, and "+
			"the letter is the run's *last* character", got)
	}

	// And exactly one gap over the whole line, which is what says the other
	// boundary — a numeral against 国 — got none.
	wide := contentWidth(autospaceRuns(t, doc, alpha))
	narrow := contentWidth(autospaceRuns(t, doc, none))
	if got := wide.Sub(narrow).Px(); got != 2 {
		t.Errorf("the spacing added %gpx to the line, want 2 — one boundary of the "+
			"two is a numeral against an ideograph, and \"ideograph-alpha\" alone "+
			"does not space it", got)
	}
}
