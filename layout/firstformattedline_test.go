package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// Which line is the first line, when a block container's inline content is
// broken up by a block child.
//
// §16.1 indents "the first line of a block container", and §5.12.1 adds the part
// that decides this case: the first formatted line of an element may occur
// inside a block-level descendant, provided that descendant is in the same flow.
// So a <section> holding "text, a <div>, more text" has exactly one first line,
// at the top, and the run of text after the div is a continuation of the same
// flow rather than the beginning of a new one.
//
// The engine wrapped each run in an anonymous block and let each of them inherit
// text-indent and act on it, so a paragraph interrupted by a figure came back
// indented as though it had begun again — which reads as a deliberate second
// paragraph, and is the kind of wrongness that survives a proofread.

// textStarts returns the x of the first glyph on each line, top to bottom.
func textStarts(t *testing.T, htmlSrc, cssSrc string) []style.Unit {
	t.Helper()
	type at struct{ x, y style.Unit }
	var all []at
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if v, ok := op.(DrawText); ok {
			all = append(all, at{v.At.X, v.At.Y})
		}
	}
	var out []style.Unit
	seen := map[style.Unit]bool{}
	for _, a := range all {
		if !seen[a.y] {
			seen[a.y] = true
			out = append(out, a.x)
		}
	}
	return out
}

// TestOnlyTheFirstLineOfABlockContainerIsIndented is the suite's own fixture,
// written as the specification's two sentences: the section's own first line and
// the block descendant's first line are both indented, and the run of text after
// the block is not.
func TestOnlyTheFirstLineOfABlockContainerIsIndented(t *testing.T) {
	got := textStarts(t,
		`<section id="s">aaa<div id="d">bbb</div>ccc</section>`,
		`#s { text-indent: 40px; font-size: 20px; width: 400px }`)
	if len(got) != 3 {
		t.Fatalf("%d lines, want three: %v", len(got), got)
	}
	// Both are first lines — the section's own and the div's, the div having
	// inherited the indent — so they agree.
	if got[0] != got[1] {
		t.Errorf("the section's first line starts at %v and the div's at %v; the "+
			"div inherits text-indent and its first line is a first line",
			got[0], got[1])
	}
	if got[2] == got[0] {
		t.Errorf("the run after the div starts at %v, the same as the first line; "+
			"it is a continuation of the section's flow and has no first line of "+
			"its own", got[2])
	}
	if got[0].Sub(got[2]) != bgpx(40) {
		t.Errorf("the indent is %v, and the declaration asks for 40px",
			got[0].Sub(got[2]))
	}
}

// TestAnInlineBrokenAroundABlockDoesNotIndentTwice is the same rule reached
// through §9.2.1.1, which is where the suite tests it: the two halves of the
// broken inline are wrapped in two anonymous blocks, and only the first of them
// is where the section's first line falls.
func TestAnInlineBrokenAroundABlockDoesNotIndentTwice(t *testing.T) {
	got := textStarts(t,
		`<section id="s"><span>aaa<div>bbb</div>ccc</span></section>`,
		`#s { text-indent: 40px; font-size: 20px; width: 400px }`)
	if len(got) != 3 {
		t.Fatalf("%d lines, want three: %v", len(got), got)
	}
	if got[2] == got[0] {
		t.Errorf("the second half of the inline starts at %v, indented like the "+
			"first; the inline was broken, not begun again", got[2])
	}
	if got[0].Sub(got[2]) != bgpx(40) {
		t.Errorf("the indent is %v, want 40px", got[0].Sub(got[2]))
	}
}

// TestABlockContainerWithNoBlockChildrenIsUnchanged is the containment case, and
// it is almost every document: no block child means no anonymous block, so
// nothing here may alter what an ordinary indented paragraph does.
func TestABlockContainerWithNoBlockChildrenIsUnchanged(t *testing.T) {
	got := textStarts(t,
		`<p id="p">aaaa aaaa aaaa aaaa aaaa aaaa</p>`,
		`#p { text-indent: 40px; font-size: 20px; width: 120px }`)
	if len(got) < 3 {
		t.Fatalf("%d lines, want the text to wrap: %v", len(got), got)
	}
	if got[0].Sub(got[1]) != bgpx(40) {
		t.Errorf("the first line is indented by %v against the second, want 40px",
			got[0].Sub(got[1]))
	}
	for i := 2; i < len(got); i++ {
		if got[i] != got[1] {
			t.Errorf("line %d starts at %v and line 1 at %v; only the first line "+
				"is indented", i, got[i], got[1])
		}
	}
}

// TestAnIndentedBlockAfterOneThatIsNotStillIndents.
//
// The rule is about the *anonymous* blocks a container was broken into, and it
// must not reach a real element that happens to come second. A <p> after a <div>
// begins its own flow and its own first line, whatever came before it.
func TestAnIndentedBlockAfterOneThatIsNotStillIndents(t *testing.T) {
	got := textStarts(t,
		`<div id="c"><div>aaa</div><p id="p">bbb</p></div>`,
		`#c { font-size: 20px; width: 400px } #p { text-indent: 40px; margin: 0 }`)
	if len(got) != 2 {
		t.Fatalf("%d lines, want two: %v", len(got), got)
	}
	if got[1].Sub(got[0]) != bgpx(40) {
		t.Errorf("the paragraph starts at %v against %v; it is an element of its "+
			"own and its first line is a first line", got[1], got[0])
	}
}

// TestAnAnonymousBlockBeforeAnyOtherContentIsStillTheFirst: a container whose
// inline content comes *first* is indented, and one whose block child comes first
// is not — which is §5.12.1's "may occur inside a block-level descendant" read
// the other way round.
func TestAnAnonymousBlockBeforeAnyOtherContentIsStillTheFirst(t *testing.T) {
	first := textStarts(t,
		`<section id="s">aaa<div>bbb</div></section>`,
		`#s { text-indent: 40px; font-size: 20px; width: 400px }`)
	after := textStarts(t,
		`<section id="s"><div>bbb</div>aaa</section>`,
		`#s { text-indent: 40px; font-size: 20px; width: 400px }`)
	if len(first) != 2 || len(after) != 2 {
		t.Fatalf("%v and %v; both fixtures should make two lines", first, after)
	}
	if first[0] != after[0] {
		t.Errorf("the two documents' first lines start at %v and %v; in both the "+
			"first line is the first thing in the flow", first[0], after[0])
	}
	if after[1] == after[0] {
		t.Errorf("the text after the div starts at %v, indented; the div's line "+
			"was already the section's first", after[1])
	}
}
