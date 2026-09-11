package layout

import (
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
// Both cases below were found by FuzzRunTiling, which asserts the arithmetic —
// two runs quantized separately are a sixty-fourth of a pixel away from one —
// and both turned out to be line breaking rather than arithmetic.

// linesOfMarkup lays markup out in a box of the given width and returns the
// lines as text.
func linesOfMarkup(t *testing.T, markup string, px float64) []string {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	built := Build(Input{HTML: `<div id="d">` + markup + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d{font-family:T;font-size:16px}`}}})
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
