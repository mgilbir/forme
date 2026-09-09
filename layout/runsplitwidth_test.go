package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// One word is one width, however inline boxes cut it into runs.
//
// CSS Text §8.1: the boundary between two inline elements does not break
// shaping. This engine honours that for the shaping itself — the runs of a word
// split by a <span> are shaped as one string, so a ligature or a joining form
// still crosses the boundary — and it did not honour it for the *arithmetic*.
//
// A run's width is a length on the page, so it is quantized to a sixty-fourth of
// a pixel. Measured separately, each of a word's runs is quantized on its own
// and the sum of the parts is not the quantization of the whole: "let<span>ter"
// in Courier at 12px came out a sixty-fourth of a pixel narrower than "letter",
// and written with a span around every letter, a sixty-fourth wider. What a
// document sees is a box shrink-wrapped around a word that is not the width of
// the word, and a line that fits one spelling of a word and not the other.
//
// The fix is not new machinery: the merge group already measures a run as the
// two ends of its own stretch of the group's single shaping, each quantized, so
// the runs of a group tile it exactly. It was gated on whether the face's
// shaping could change with its context — joining forms, kerning, ligatures —
// and a face with none of those got no group and no tiling. The gate belongs on
// the shaping and not on the arithmetic.
func TestOneWordMeasuresTheSameHoweverInlineBoxesCutIt(t *testing.T) {
	const decl = `#p { font-family: Courier; font-size: 12px }`

	width := func(markup string) (style.Unit, int) {
		t.Helper()
		f := find(t, layoutOf(t, 4000, `<div id="p">`+markup+`</div>`, decl), "p")
		if len(f.Lines) != 1 {
			t.Fatalf("%q laid out as %d lines", markup, len(f.Lines))
		}
		var w style.Unit
		for _, r := range f.Lines[0].Runs {
			w = w.Add(r.Width)
		}
		return w, len(f.Lines[0].Runs)
	}

	whole, runs := width("letter")
	if runs != 1 {
		t.Fatalf("the unsplit word is %d runs, so there is nothing to compare against", runs)
	}

	for _, tc := range []struct{ markup, what string }{
		{`let<span>ter</span>`, "one span"},
		{`<span>let</span>ter`, "a span at the start"},
		{`le<span>tt</span>er`, "a span in the middle"},
		{`l<span>e</span>t<span>t</span>e<span>r</span>`, "a span at every other letter"},
		{`<span>l</span><span>e</span><span>t</span><span>t</span><span>e</span><span>r</span>`,
			"a span around every letter"},
		{`let<span><span>ter</span></span>`, "nested spans"},
	} {
		got, n := width(tc.markup)
		if n < 2 {
			t.Errorf("%s (%s): laid out as %d run, so it does not test a split",
				tc.what, tc.markup, n)
			continue
		}
		if got != whole {
			t.Errorf("%s (%s): %d runs measuring %v against %v for the same word "+
				"unsplit", tc.what, tc.markup, n, got, whole)
		}
	}
}

// TestABreakOpportunityStillEndsTheGroup is the containment case.
//
// The group is bounded by the places a line may end, and it has to stay bounded:
// the runs of a group are shaped as one string, and a ligature formed across an
// opportunity would put half a glyph on each of two lines. Widening the gate
// above must not widen that.
//
// It is asserted as a *shape* rather than as a width, because the widths on
// either side of an opportunity are exactly what cannot be required to add up —
// two runs that may land on different lines are quantized separately by
// necessity. What is required is that no group holds both of two words, which is
// what MergePre and MergePost say: together with the run's own text they are the
// string it is shaped as part of.
//
// A word does take the space after it into its group, and that is not the
// boundary being tested: the opportunity is after a space and not before one, so
// the space belongs to the word in front of it and travels with it. What may not
// happen is the group reaching past that space to the next word.
func TestABreakOpportunityStillEndsTheGroup(t *testing.T) {
	f := find(t, layoutOf(t, 4000, `<div id="p">one two</div>`,
		`#p { font-family: Courier; font-size: 12px }`), "p")
	if len(f.Lines) != 1 {
		t.Fatalf("the fixture laid out as %d lines", len(f.Lines))
	}
	var words int
	for _, r := range f.Lines[0].Runs {
		if r.Text != "one" && r.Text != "two" {
			continue
		}
		words++
		group := r.MergePre + r.Text + r.MergePost
		if strings.Contains(group, "one") && strings.Contains(group, "two") {
			t.Errorf("%q is shaped as part of %q, which holds both words and so "+
				"spans the opportunity between them", r.Text, group)
		}
	}
	if words != 2 {
		t.Fatalf("the fixture drew %d of its two words, so it says nothing", words)
	}
}
