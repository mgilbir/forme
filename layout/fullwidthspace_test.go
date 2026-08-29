package layout

import (
	"strings"
	"testing"
)

// A space that "full-width" has already frozen cannot be collapsed afterwards,
// so the run it belongs to has to be closed before the transform reaches it.
//
// §4.1.1 collapses a run of white space wherever it is written, and css-text-4
// says "intervening inline box boundaries must be ignored" — so a run that a
// node boundary falls inside is one run and collapses to one space. This engine
// does the crossing part in the flattening, which is the better place for it:
// the space that goes still leaves the break opportunity §4.1.1's parenthesis
// asks for. What the flattening cannot do is collapse a character that is no
// longer white space, and "full-width" maps U+0020 to U+3000 IDEOGRAPHIC SPACE
// — one of §4.1's "other space separators", which nothing collapses — one node
// at a time, before the flattening sees any of it.
//
// word-space-transform-009 is the suite's case and it writes the boundary six
// ways in one line.

// paintedStrings is the strings a document draws, in paint order.
func paintedStrings(t *testing.T, htmlSrc string) []string {
	t.Helper()
	var out []string
	for _, op := range paintOf(t, htmlSrc, "") {
		if v, ok := op.(DrawText); ok {
			out = append(out, v.Text)
		}
	}
	return out
}

const fullWidthLine = `<div style="word-space-transform:space;text-transform:full-width">` +
	`a b<wbr>c&#x200b;d <wbr>e &#x200b;f<wbr>&#x200b;g</div>`

func TestAFrozenSpaceIsCollapsedBeforeItIsFrozen(t *testing.T) {
	// Six boundaries, each written a different way: a space, a <wbr>, a zero
	// width space, a space beside a <wbr>, a space beside a zero width space,
	// and a <wbr> beside a zero width space. Every one of them is one run and
	// comes out as one ideographic space, which is what
	// word-space-transform-009's reference draws.
	got := strings.Join(paintedStrings(t, fullWidthLine), "")
	const want = "ａ　ｂ　ｃ　ｄ　ｅ　ｆ　ｇ"
	if got != want {
		t.Errorf("the line drew %q, want %q: a run of white space is one run "+
			"however many nodes it is written across, and \"full-width\" freezes "+
			"whatever has not been collapsed by the time it runs", got, want)
	}
}

func TestOnlyTheFrozenRunIsClosedEarly(t *testing.T) {
	// Without "full-width" the flattening does the crossing, and it must go on
	// doing it: the space it drops leaves a break opportunity behind and the
	// space this would drop does not. Fifty-seven documents turn on the
	// difference — every border-top-width and content-counter pair among them.
	got := paintedStrings(t, `<div>a <span> b</span></div>`)
	if len(got) != 3 || got[1] != " " {
		t.Errorf("without a transform the line drew %q, want one space between "+
			"the two words", got)
	}
}

func TestTheRunIsClosedOnlyAtItsStart(t *testing.T) {
	// The bit says what this node *follows*, so it is spent on the first run
	// and says nothing about the second. A node whose own text opens with a
	// letter has ended the run before its spaces begin.
	got := strings.Join(paintedStrings(t,
		`<div style="text-transform:full-width">a <span>b c</span></div>`), "")
	const want = "ａ　ｂ　ｃ"
	if got != want {
		t.Errorf("the line drew %q, want %q: the space between b and c is inside "+
			"the node and is not the run that crossed into it", got, want)
	}
}

func TestAPreservedBreakIsNotASpace(t *testing.T) {
	// pre-line collapses spaces and keeps segment breaks, and a break is not a
	// space: closing a run must not swallow one. The two words are on two lines,
	// so the second begins where the first did.
	ops := paintOf(t, `<div style="white-space:pre-line;text-transform:full-width">`+
		`a <span>`+"\n"+`b</span></div>`, "")
	var first, second *DrawText
	for i := range ops {
		v, ok := ops[i].(DrawText)
		if !ok || strings.TrimSpace(v.Text) == "" {
			continue
		}
		if first == nil {
			first = &v
			continue
		}
		if second == nil {
			second = &v
		}
	}
	if first == nil || second == nil {
		t.Fatalf("the document drew %d runs, want two words", len(ops))
	}
	if second.At.Y <= first.At.Y {
		t.Errorf("%q is at y=%v and %q at y=%v; the preserved break puts the "+
			"second on a line of its own", first.Text, first.At.Y, second.Text,
			second.At.Y)
	}
	if second.At.X != first.At.X {
		t.Errorf("%q begins at x=%v and %q at x=%v; a line begins where a line "+
			"begins, and the run that crossed the boundary left nothing in front "+
			"of it", first.Text, first.At.X, second.Text, second.At.X)
	}
}
