package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/style"
)

// The same text breaks in the same places however it is marked up.
//
// CSS Text §8.1's boundary between two inline elements does not break shaping,
// and it does not break line breaking either. An opportunity found at the end of
// one box is offered at the start of the next, and the rules that decide whether
// it survives need the character on the far side of the boundary — which is in
// another box and which neither box can see on its own.
//
// So the scan says what it leaves and the next box asks the question: see
// paragraph.Trailing, whose two fields are the two facts that cannot be read
// back off the text.
//
// The ten defects here were found by FuzzRunTiling and FuzzBoundaryLines, which asserts the
// arithmetic — two runs quantized separately are a sixty-fourth of a pixel away
// from one — and every one of them turned out to be line breaking rather than
// arithmetic. That is not a coincidence: a merge group is exactly the run of
// text no line may fall inside, so a lost or invented opportunity changes what
// is shaped together and the widths say so.
//
// Two of the twelve are containment rather than regression — LB7 in front of a
// preserved space, and the opportunity a space takes — and each names the fix
// that broke it. They are the rules an attempt here is most likely to cost.

// linesOfMarkup lays markup out in a box of the given width and returns the
// lines as text, in the bundled Noto Sans at 16px.
func linesOfMarkup(t *testing.T, markup string, px float64) []string {
	t.Helper()
	return linesOfMarkupCSS(t, markup, px, `#d{font-family:T;font-size:16px}`)
}

// linesOfMarkupCSS is linesOfMarkup with the declarations for the test, which
// are appended to the defaults above and so may override them. Everything is in
// one div with the id "d"; a rule for anything else needs a class.
func linesOfMarkupCSS(t *testing.T, markup string, px float64, css string) []string {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="d">` + markup + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d{font-family:T;font-size:16px} ` + css}}})
	if built.Root == nil {
		t.Fatalf("%q produced no boxes", markup)
	}
	w, _ := style.FromPx(px)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))
	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found != nil || f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "d" {
				found = f
				return
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if found == nil {
		t.Fatalf("%q laid out to no fragment", markup)
	}
	var out []string
	for _, line := range found.Lines {
		var text string
		for _, r := range line.Runs {
			text += r.Text
		}
		out = append(out, text)
	}
	return out
}

// TestABoundaryDoesNotInventABreakTheTextDoesNotHave.
//
// "0|!" is one unbreakable run. U+007C is UAX #14 class BA so a line may end
// after it, U+0021 is class EX so a line may not begin with one, and the second
// rule wins — SplitAtBreaks agrees and cuts the text into one piece.
//
// Written with a span between them the opportunity reached the next box as a
// bare flag, and the rule that would have refused it was a character away in
// another box. In an eighteen-pixel box the first spelling set one line and the
// second set two.
//
// The old answer to "may the next character refuse this" was to look at the last
// character of the box before and ask whether it was an ideograph. That is right
// for an ideograph and wrong for every other character that defers an
// opportunity, which is the whole of class BA.
func TestABoundaryDoesNotInventABreakTheTextDoesNotHave(t *testing.T) {
	for _, px := range []float64{18, 20, 40} {
		whole := linesOfMarkup(t, "0|!", px)
		cut := linesOfMarkup(t, "<span>0|</span><span>!</span>", px)
		if len(whole) != 1 {
			t.Fatalf("at %gpx \"0|!\" set %d lines %q; a line may not begin with "+
				"an exclamation mark, so there is nowhere for it to break and "+
				"the comparison below is against the wrong answer", px, len(whole), whole)
		}
		if len(cut) != len(whole) {
			t.Errorf("at %gpx \"0|!\" set %d lines %q and the same text in two "+
				"spans set %d %q", px, len(whole), whole, len(cut), cut)
		}
	}
}

// TestABoundaryDoesNotLoseABreakTheTextHas is the same rule in the other
// direction, and the second thing the invariant found.
//
// "中中、中" breaks after the comma: the opportunity between the second and third
// characters is one UAX #14 will not let a line begin with a comma at, so it
// moves past it and lands on the last ideograph. That is a *held* opportunity,
// and a box that ran out of text while holding one dropped it — so
// "中中、" and "中" in two spans was one unbreakable run.
func TestABoundaryDoesNotLoseABreakTheTextHas(t *testing.T) {
	// Narrow enough that the four ideographs cannot share a line: each is about
	// nine and a half pixels here.
	const narrow = 25
	whole := linesOfMarkup(t, "中中、中", narrow)
	cut := linesOfMarkup(t, "<span>中中、</span><span>中</span>", narrow)
	if len(whole) < 2 {
		t.Fatalf("\"中中、中\" set %d lines %q at %gpx; without a break there is "+
			"nothing for the span version to lose", len(whole), whole, float64(narrow))
	}
	if len(cut) != len(whole) {
		t.Errorf("\"中中、中\" set %d lines %q and the same text in two spans set "+
			"%d %q", len(whole), whole, len(cut), cut)
	}
}

// TestABoundaryLeavesBothABreakAndAHold is the other half of the test below,
// and the two together are why Trailing has three fields rather than an enum.
//
// "0|-!00" sets three lines in a box narrower than a character: the hyphen takes
// an unconditional break, and the opportunity the vertical line offered — which
// the hyphen held, because a line may not begin with one — lands on the "0" that
// the exclamation mark refused it in front of. Two breaks, from two rules, one
// of them at the boundary and one of them two characters past it.
//
// A box can only say one thing about its far edge, so it says three. Saying only
// the hold lost the break, which is the test below; saying only the taken one
// lost the hold, which is this: "<span>0|-</span><span>!00</span>" set two lines
// where the text sets three.
func TestABoundaryLeavesBothABreakAndAHold(t *testing.T) {
	// Narrower than one character, so every opportunity there is gets taken and
	// a lost one is a line that did not happen.
	const narrow = 8
	for _, whole := range []string{"0|-!00", "0|-!0", "|-!00"} {
		got := linesOfMarkup(t, whole, narrow)
		at := strings.Index(whole, "!")
		cut := linesOfMarkup(t,
			`<span>`+whole[:at]+`</span><span>`+whole[at:]+`</span>`, narrow)
		if len(got) < 3 {
			t.Fatalf("%q set %d lines %q; the hyphen takes one break and the "+
				"vertical line's hold lands past the exclamation mark, so there "+
				"are at least three", whole, len(got), got)
		}
		if strings.Join(cut, "\x00") != strings.Join(got, "\x00") {
			t.Errorf("%q set %q and the same text cut before the exclamation "+
				"mark set %q; the box left a taken break *and* a hold, and the "+
				"one that was dropped is a line", whole, got, cut)
		}
	}
}

// TestATakenBreakIsWhatTheBoundaryIs, and the three kinds are not exclusive.
//
// "|-!" sets two lines. The vertical line is class BA and offers a break; the
// hyphen is one a line may not begin with, so that opportunity is *held* past
// it; and the hyphen then takes an unconditional opportunity of its own, which
// is what lets a hyphenated compound break where it is written. Both land at
// the same offset, and an exclamation mark refuses the first and not the second.
//
// At the end of a box the two coincide at the boundary, and Trailing said
// "held" — so the next box did what a box handed a hold is meant to do, ran the
// prohibition, and had nothing left. "<span>|-</span><span>!</span>" set one
// line where the text sets two.
//
// It is the one case in this file where the box before is not merely reporting
// what it left but choosing between two things it left at once.
func TestATakenBreakIsWhatTheBoundaryIs(t *testing.T) {
	// Narrow enough that "|-" and the character after it cannot share a line.
	const narrow = 14
	for _, tc := range []struct{ whole, cut string }{
		{"|-!", `<span>|-</span><span>!</span>`},
		{"|-)", `<span>|-</span><span>)</span>`},
		{"|\u2010!", `<span>|\u2010</span><span>!</span>`},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if len(whole) != 2 {
			t.Fatalf("%q set %d lines %q; the hyphen takes a break nothing "+
				"after it refuses, so it is two and the comparison below is "+
				"against the wrong answer", tc.whole, len(whole), whole)
		}
		if len(cut) != len(whole) {
			t.Errorf("%q set %d lines %q and the same text in two spans set "+
				"%d %q", tc.whole, len(whole), whole, len(cut), cut)
		}
	}
}

// TestALineDoesNotEndInFrontOfAPreservedSpaceAcrossABoundary is LB7 at the
// boundary, and the containment case for the fix above.
//
// A line may not end in front of a space. An opportunity that arrives at a box
// boundary has to be asked that question by the box it arrives in, because the
// space is on the far side of the boundary and the box that offered the
// opportunity cannot see it.
//
// Here the div preserves its spaces and does not wrap; the span inside it does
// both. The span's space collapses and offers a break, the space after it
// belongs to the div and is preserved, and LB7 withholds the opportunity — the
// line overflows instead. Seeding the scan with the opportunity but not the rule
// broke the line in front of that space, which is what white-space-mixed-001 is:
// a "pre" div whose spans hand it a space apiece.
func TestALineDoesNotEndInFrontOfAPreservedSpaceAcrossABoundary(t *testing.T) {
	// Courier is monospace, so at 20px every character is 12 wide and the
	// twenty that follow overflow a 240-pixel box on their own. Without the
	// overflow nothing has to look for a break and the rule is never reached.
	const css = `#d{white-space:pre;font-family:Courier;font-size:20px} .n{white-space:normal}`
	const markup = `x<span class="n"> </span> xxxxxxxxxxxxxxxxxxxx`
	got := linesOfMarkupCSS(t, markup, 240, css)
	if len(got) != 1 {
		t.Errorf("a collapsible space in front of a preserved one set %d lines "+
			"%q; the only opportunity is in front of the preserved space, where "+
			"a line may not end, so it should be one overflowing line",
			len(got), got)
	}
}

// TestAHeldOpportunityKeepsBeingRefused is the rule applied twice.
//
// A prohibition moves an opportunity rather than deleting one, so a line may not
// begin with a closing bracket however many are written in a row. "|!!" is one
// unbreakable run: the vertical line offers a break, the first exclamation mark
// refuses it, and the second has to refuse it again.
//
// Written as three spans the hold crossed the second box and arrived at the
// third having forgotten it was one — so the third box took a break the text
// does not have. It is the same defect as the two above, one box further along,
// and it is here because the fuzzer found it after the other two were fixed.
func TestAHeldOpportunityKeepsBeingRefused(t *testing.T) {
	for _, px := range []float64{14, 18, 30} {
		whole := linesOfMarkup(t, "|!!", px)
		cut := linesOfMarkup(t, "<span>|</span><span>!</span><span>!</span>", px)
		if len(whole) != 1 {
			t.Fatalf("at %gpx \"|!!\" set %d lines %q; a line may begin with "+
				"neither exclamation mark, so there is nowhere to break",
				px, len(whole), whole)
		}
		if len(cut) != len(whole) {
			t.Errorf("at %gpx \"|!!\" set %d lines %q and the same text in "+
				"three spans set %d %q", px, len(whole), whole, len(cut), cut)
		}
	}
}

// TestAHoldIsTakenUpInsideTheNextBox is the fourth of the family, and the one
// that decided where the fix belongs.
//
// "|!0" is one piece: the vertical line offers a break, the exclamation mark
// refuses it, and the digit takes it — inside a run the scan holds the
// opportunity and moves it along, which is what makes "|" and "!0" one
// unbreakable unit and the break fall before nothing at all.
//
// Written as two spans the hold has to be taken up *inside* the second box, at
// its second character. Layout could not do that: it resumes a hold at the next
// *piece*, and "!0" is one piece. So the boundary is handed to SplitAtBreaks and
// the scan runs its own machinery over it — see paragraph.Carried, which is the
// whole of why this is a struct and not a flag.
func TestAHoldIsTakenUpInsideTheNextBox(t *testing.T) {
	for _, px := range []float64{14, 18, 30} {
		whole := linesOfMarkup(t, "|!0", px)
		cut := linesOfMarkup(t, "<span>|</span><span>!0</span>", px)
		if len(cut) != len(whole) {
			t.Errorf("at %gpx \"|!0\" set %d lines %q and the same text in two "+
				"spans set %d %q", px, len(whole), whole, len(cut), cut)
		}
	}
}

// TestABoxsFirstCharacterMayOfferItsOwnBreak is the family's other direction,
// and the one the arithmetic invariant found.
//
// Every case above is an opportunity the box *before* left behind. This one is
// made by the next box's own first character: the rules that put a break in
// front of a character rather than after one — the ideograph's, the aksara's
// and §5.1's fallback for a script with no dictionary.
//
// New Tai Lue is such a script, so "0ᦤ" breaks between the two characters and
// "<span>0</span><span>ᦤ</span>" has to as well. It did not: at the second
// box's first character there was no text to flush and nothing carried, so the
// opportunity was dropped.
//
// FuzzRunTiling found it as a width rather than as a line — the two spans were
// one unbreakable run, so they were shaped as one merge group and tiled, and
// the whole text's two runs were quantized apart — which is the invariant doing
// what it is for: a break opportunity a ligature may not span is a fact about
// the arithmetic too. testdata/fuzz/FuzzRunTiling holds the input.
func TestABoxsFirstCharacterMayOfferItsOwnBreak(t *testing.T) {
	// Narrow enough that two characters cannot share a line.
	const narrow = 12
	for _, tc := range []struct{ whole, cut string }{
		{"0ᦤ", `<span>0</span><span>ᦤ</span>`},
		{"ᦤᦤ", `<span>ᦤ</span><span>ᦤ</span>`},
		{"0ᦤᦤ", `<span>0ᦤ</span><span>ᦤ</span>`},
		// A box ending in a literal replacement character has a last character
		// like any other. Reading utf8.RuneError as "there is none" gave this
		// one the paragraph's own answer and lost the break. See lastRuneOf.
		{"\uFFFDᦤ", `<span>` + "\uFFFD" + `</span><span>ᦤ</span>`},
	} {
		whole := linesOfMarkup(t, tc.whole, narrow)
		cut := linesOfMarkup(t, tc.cut, narrow)
		if len(whole) < 2 {
			t.Fatalf("%q set %d lines %q; without a break there is nothing for "+
				"the span version to lose", tc.whole, len(whole), whole)
		}
		if len(cut) != len(whole) {
			t.Errorf("%q set %d lines %q and the same text in spans set %d %q",
				tc.whole, len(whole), whole, len(cut), cut)
		}
	}
}

// TestLB7IsAboutASpaceAndNotEveryWhiteCharacter is the last of the family, and
// the one with no symptom in the lines at all.
//
// A line may not end between two ordinary spaces, and that is the whole of what
// the boundary withholds — see betweenTwoSpaces. Everything else the scan gives
// a white-space Piece of its own — the tab, and §4.1's other space separators —
// is class BA, which a line may perfectly well end in front of. The boundary
// borrowed startsSpacePiece, which is the wider set and is the right answer to
// a different question (see its note), and so withheld an opportunity from a
// character the rule says nothing about.
//
// Inside a run the question never arises: the space arm offers the opportunity
// and no test stands between. So the two spellings disagreed, and only about
// the *arithmetic* — the opportunity is at the same offset either way once the
// line is long enough to reach it, and what changed is whether a merge group
// was allowed to span it. A ligature may not cross a break opportunity; a run
// that is wrongly one group is quantized as one, and comes out a sixty-fourth
// of a pixel from the same text written plainly.
//
// Which is why this is a width and the lines are identical under the defect,
// and why it took an invariant to find. In Courier because that is where the
// tiling shows: "⭋ &#x2000;" is what FuzzRunTiling minimized it to.
func TestLB7IsAboutASpaceAndNotEveryWhiteCharacter(t *testing.T) {
	const (
		quad = "\u2000" // EN QUAD, UAX #14 class BA
		ideo = "\u3000" // IDEOGRAPHIC SPACE, class BA too
		sp   = "\u0020" // the one LB7 is actually about
	)
	for _, tc := range []struct{ what, whole, cut string }{
		{"an en quad", "a " + quad,
			"<span>a </span><span>" + quad + "</span>"},
		{"two of them", "a " + quad + quad,
			"<span>a " + quad + "</span><span>" + quad + "</span>"},
		{"an ideographic space", "a " + ideo,
			"<span>a </span><span>" + ideo + "</span>"},
		{"one with text after it", "a " + quad + "b",
			"<span>a </span><span>" + quad + "b</span>"},
		// And the character the rule *is* about, which must still withhold it.
		{"an ordinary space does", "a " + sp + "b",
			"<span>a </span><span>" + sp + "b</span>"},
	} {
		whole := widthOfMarkupIn(t, "Courier", tc.whole)
		cut := widthOfMarkupIn(t, "Courier", tc.cut)
		if whole != cut {
			t.Errorf("%s: %q is %v wide written plainly and %v in spans; LB7 is "+
				"about a space and a zero width space, so the opportunity the "+
				"space took reaches this character and no merge group may span it",
				tc.what, tc.whole, whole, cut)
		}
	}
}

// TestLB7ReadsBothSidesOfTheBoundary is the same rule from the other end, and
// the half that reading LB7 as "× SP" alone gets wrong.
//
// "a&#x2000; &#x2000;" breaks in front of its ordinary space: the EN QUAD before
// it is class BA, the scan cuts a Piece at each of them, and an opportunity
// falls between. Withholding at a boundary because the character *after* it is a
// space asks only half the question, and the two spellings of that text came out
// a sixty-fourth of a pixel apart — the merge group was let across an
// opportunity a ligature may not span, exactly as in the test above.
//
// Asking about the white-space value instead — is this an element that gathers a
// run of spaces — is the other wrong answer, and it costs white-space-mixed-001
// a line. The two spaces at such a boundary can be in elements that answer
// differently, and §5.1's common ancestor is not the element whose run the
// question is about.
func TestLB7ReadsBothSidesOfTheBoundary(t *testing.T) {
	const (
		quad = "\u2000"
		sp   = "\u0020"
	)
	for _, tc := range []struct{ what, whole, cut string }{
		{"a quad, then a space", "a" + quad + sp + quad,
			"<span>a" + quad + "</span><span>" + sp + quad + "</span>"},
		{"without the trailing quad", "a" + quad + sp,
			"<span>a" + quad + "</span><span>" + sp + "</span>"},
		{"a quad on both sides", "a" + quad + sp + quad + "b",
			"<span>a" + quad + "</span><span>" + sp + quad + "b</span>"},
	} {
		whole := widthOfMarkupIn(t, "Courier", tc.whole)
		cut := widthOfMarkupIn(t, "Courier", tc.cut)
		if whole != cut {
			t.Errorf("%s: %q is %v wide written plainly and %v in spans; the "+
				"character in front of the boundary is an EN QUAD, which a line "+
				"may end after, so the opportunity reaches the space and no "+
				"merge group may span it", tc.what, tc.whole, whole, cut)
		}
	}
}

// TestAHyphenAtABoxEdgeLooksAtWhatFollowsIt is the last of the family, and the
// one where the box that is wrong is the box *before* the boundary.
//
// A hyphen ends a run and the next may begin a line, which is what lets a
// hyphenated compound break where it is written. Unless white space follows it:
// a line that ends in front of a space has nothing to move down to the next one,
// so the opportunity is not taken and the scan's arm is gated on exactly that.
//
// At the end of a box the white space is in the next box, and the gate answered
// "no white space" because it could not see. So "⭋‐&#x2000;" written in one box
// takes no opportunity at its hyphen and written in two takes one, and the two
// spellings came out a sixty-fourth of a pixel apart — the merge group was
// refused across a boundary that has no opportunity on it.
//
// Two more arms are gated the same way and are fixed with it: the soft hyphen,
// and §5.3's loose-break characters. Neither is easy to reach from a document,
// which is why they are named in the note on startsSpace rather than tested here.
func TestAHyphenAtABoxEdgeLooksAtWhatFollowsIt(t *testing.T) {
	const (
		quad   = "\u2000"
		hyphen = "\u2010"
	)
	for _, tc := range []struct{ what, whole, cut string }{
		{"a hyphen then an en quad", "a" + hyphen + quad,
			"<span>a" + hyphen + "</span><span>" + quad + "</span>"},
		{"a hyphen then a space", "a" + hyphen + " b",
			"<span>a" + hyphen + "</span><span> b</span>"},
		{"a hyphen-minus then a space", "a- b",
			"<span>a-</span><span> b</span>"},
		// And the case the gate exists for, which must keep working: a hyphen
		// with a letter after it breaks, wherever the boundary falls.
		{"a hyphen then a letter", "high" + hyphen + "way",
			"<span>high" + hyphen + "</span><span>way</span>"},
	} {
		whole := widthOfMarkupIn(t, "Courier", tc.whole)
		cut := widthOfMarkupIn(t, "Courier", tc.cut)
		if whole != cut {
			t.Errorf("%s: %q is %v wide written plainly and %v in spans; what "+
				"the hyphen does depends on the character after it, and a box "+
				"boundary does not hide that character", tc.what, tc.whole,
				whole, cut)
		}
	}
	// The letter case says the same thing in lines, where it is plainer: the
	// compound breaks at its hyphen either way.
	const narrow = 40
	whole := linesOfMarkup(t, "high"+hyphen+"way", narrow)
	cut := linesOfMarkup(t, "<span>high"+hyphen+"</span><span>way</span>", narrow)
	if len(whole) != 2 {
		t.Fatalf("%q set %d lines %q; a hyphenated compound breaks where it is "+
			"written", "high"+hyphen+"way", len(whole), whole)
	}
	if strings.Join(cut, "\x00") != strings.Join(whole, "\x00") {
		t.Errorf("%q set %q and the same text in two spans set %q",
			"high"+hyphen+"way", whole, cut)
	}
}

// TestAnOpportunityAfterASpaceStillCrossesABoundary is the containment case, and
// the one a fix here is most likely to break.
//
// The prohibition applies to the opportunities the scan *offers* and not to the
// one a space takes: "AA )BB" breaks after the space and always has. A box
// boundary must not change that either — which is what
// TestAnOpportunityFromASpaceIsNotWithheld says from the other side, and what
// the first attempt at this fix got wrong by four reftests.
func TestAnOpportunityAfterASpaceStillCrossesABoundary(t *testing.T) {
	for _, mark := range []string{")", "…", "！"} {
		whole := linesOfMarkup(t, "AA "+mark+"BB", 40)
		cut := linesOfMarkup(t, "AA <span>"+mark+"BB</span>", 40)
		if len(whole) < 2 {
			t.Fatalf("%q set one line, so there is no break for the span "+
				"version to lose", "AA "+mark+"BB")
		}
		if len(cut) != len(whole) {
			t.Errorf("%q set %d lines %q and the same text in a span set %d %q",
				"AA "+mark+"BB", len(whole), whole, len(cut), cut)
		}
	}
}
