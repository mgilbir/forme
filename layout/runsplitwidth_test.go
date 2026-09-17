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

// TestAnInvisibleDoesNotMoveTheOpportunityBeforeAnIdeograph is the tiling
// question asked about the *opportunity* rather than about the group.
//
// A break opportunity ends a merge group, which is the test above. So a rule
// that finds an opportunity in one spelling of a text and not in another cuts
// the two into different groups, and the runs of a group tile it exactly while
// runs measured alone do not: the same text comes out a sixty-fourth of a pixel
// apart. That is how FuzzRunTiling found a line-breaking defect from an
// arithmetic invariant.
//
// The defect was that §5.1's opportunity before an ideograph asked what the
// character before it was, rather than what the typographic character unit
// before it was — so one right-to-left mark, which sets no paper and takes no
// room, deleted the only opportunity in "0‏逭". Both halves of the rule had
// it: the scan inside a text node, and endsLetterUnit at a box boundary.
//
// The six spellings are the same three characters divided every way an author
// could divide them, and the assertion is that one width answers for all of
// them. See paragraph's TestAnInvisibleDoesNotDeleteTheOpportunityBeforeAnIdeograph,
// which holds the opportunity itself.
func TestAnInvisibleDoesNotMoveTheOpportunityBeforeAnIdeograph(t *testing.T) {
	const decl = `#p { font-family: Courier; font-size: 12px; white-space: nowrap }`
	width := func(markup string) style.Unit {
		t.Helper()
		f := find(t, layoutOf(t, 4000, `<div id="p">`+markup+`</div>`, decl), "p")
		if len(f.Lines) != 1 {
			t.Fatalf("%q laid out as %d lines", markup, len(f.Lines))
		}
		var w style.Unit
		for _, r := range f.Lines[0].Runs {
			w = w.Add(r.Width)
		}
		return w
	}
	// The character between the digit and the ideograph, which is the one the
	// rule must look past. The two are the two ways of not being a base: a
	// format character that sets no paper, and a spacing mark that sets ink and
	// belongs to the character in front of it all the same.
	for _, mid := range []struct{ text, what string }{
		{"\u200f", "a right-to-left mark"},
		{"\u1064", "a Myanmar spacing mark"},
	} {
		want := width("0" + mid.text + "\u9038")
		for _, markup := range []string{
			"<span>0</span><span>" + mid.text + "</span><span>\u9038</span>",
			"<span>0" + mid.text + "</span><span>\u9038</span>",
			"<span>0</span><span>" + mid.text + "\u9038</span>",
			"<span>0</span>" + mid.text + "<span>\u9038</span>",
			"0<span>" + mid.text + "\u9038</span>",
			"<span>0</span><span>" + mid.text + "</span>\u9038",
		} {
			if got := width(markup); got != want {
				t.Errorf("with %s, %s is %v wide and the same text whole is %v; a "+
					"box boundary does not change what the text is",
					mid.what, markup, got, want)
			}
		}
	}
	// The joiner is the other way round, and it is here because making the rule
	// right about letter units exposed it. UAX #14's LB8a forbids a break after a
	// zero width joiner wherever one falls, and a box boundary is nowhere
	// special — so "0\u200d\u9038" has no opportunity in it however it is cut,
	// and every spelling is the width of the three characters with none.
	{
		want := width("0\u200d\u9038")
		for _, markup := range []string{
			"<span>0\u200d</span><span>\u9038</span>",
			"<span>0</span><span>\u200d</span><span>\u9038</span>",
			"<span>0</span><span>\u200d\u9038</span>",
			"0<span>\u200d</span>\u9038",
		} {
			if got := width(markup); got != want {
				t.Errorf("with a zero width joiner, %s is %v wide and the same text "+
					"whole is %v; a line may not end after a joiner, in any spelling",
					markup, got, want)
			}
		}
	}
	// And the invisible changes nothing at all, which is the stronger statement
	// and the one that says which width is right: a character that sets no paper
	// and takes no room cannot make the line wider. Only the format character
	// can be asked this — a spacing mark has an advance of its own.
	if plain, marked := width("0\u9038"), width("0\u200f\u9038"); plain != marked {
		t.Errorf("with a right-to-left mark between them the text is %v wide and "+
			"without it %v; the mark draws nothing and takes no room", marked, plain)
	}
}
