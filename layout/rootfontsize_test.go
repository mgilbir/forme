package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// The root element's own font-size, which is resolved once.
//
// CSS Values §5.1.1: an em on the root element refers to the property's initial
// value. There is no parent to take a size from, so 16px is what "2em" is twice
// of — and that answer is also what every "rem" in the document means.
//
// It was resolved twice. The root's size was worked out against the initial
// value and then handed to the box builder *as the parent size*, where the root
// — an element like any other — resolved its own declared font-size against it a
// second time. A root saying "font-size: 2em" came out at 64px while every "rem"
// in the same document went on meaning 32px, so the two disagreed inside one
// page.

// rootEmHeight is the height of a box whose height is one em, which is the
// inherited font-size made visible.
func rootEmHeight(t *testing.T, rootRule string) style.Unit {
	t.Helper()
	ops := paintOf(t, `<div id="d"></div>`,
		rootRule+` #d { background: rgb(0,0,255); height: 1em; width: 10px }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1: %v", len(got), got)
	}
	return got[0].H
}

// TestARootEmIsTwiceTheInitialSize is the bug.
func TestARootEmIsTwiceTheInitialSize(t *testing.T) {
	for _, tc := range []struct {
		rule string
		want float64
	}{
		// Twice the initial 16px, and not twice that again.
		{`html { font-size: 2em }`, 32},
		{`html { font-size: 200% }`, 32},
		// A rem on the root is the same question with the same answer.
		{`html { font-size: 2rem }`, 32},
		// An absolute size is the control: it cannot be resolved twice, so it
		// was right before and must still be.
		{`html { font-size: 32px }`, 32},
		// And nothing stated at all.
		{``, 16},
	} {
		if got := rootEmHeight(t, tc.rule); got != bgpx(tc.want) {
			t.Errorf("%q: one em is %v, want %v", tc.rule, got, bgpx(tc.want))
		}
	}
}

// TestARootEmAgreesWithRem is the property the double resolution broke, and it
// is worth asserting separately: the two numbers came from different places and
// only one of them was resolved twice, so a document could disagree with itself.
func TestARootEmAgreesWithRem(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div><div id="b"></div>`,
		`html { font-size: 2em }
		 #a { background: rgb(0,0,255); height: 1em; width: 10px }
		 #b { background: rgb(0,128,0); height: 1rem; width: 10px }`)
	em := fillsOf(ops, blue)
	rem := fillsOf(ops, green)
	if len(em) != 1 || len(rem) != 1 {
		t.Fatalf("%d em fills and %d rem fills, want one each", len(em), len(rem))
	}
	if em[0].H != rem[0].H {
		t.Errorf("an inherited em is %v and a rem is %v; on a document whose root "+
			"declares its own size the two are the same length", em[0].H, rem[0].H)
	}
}

// TestADescendantEmIsStillNotCompounded. The fix hands the builder the initial
// size rather than the resolved one, and must not have disturbed the rule that
// an *inherited* font-size is not resolved again — a paragraph four elements
// inside "font-size: 2em" came out at 256px when it was.
func TestADescendantEmIsStillNotCompounded(t *testing.T) {
	ops := paintOf(t,
		`<div><div><div><div id="d"></div></div></div></div>`,
		`div { font-size: 2em } #d { background: rgb(0,0,255); height: 1em; width: 10px }`)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d fills, want 1", len(got))
	}
	// Four nested divs each doubling: 16 → 32 → 64 → 128 → 256. Each *declares*
	// a size, so each resolves one; what must not happen is the innermost also
	// re-resolving what it inherited.
	if got[0].H != bgpx(256) {
		t.Errorf("one em four levels in is %v, want %v", got[0].H, bgpx(256))
	}
}
