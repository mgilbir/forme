package layout

import (
	"testing"
)

// Whether a line may end beside an atomic inline, CSS Text §5.1.
//
//	for soft wrap opportunities defined by the boundary between two
//	characters, the white-space property on the nearest common ancestor of the
//	two characters controls breaking
//
// A picture or an inline-block is a character unit for that purpose, and the
// opportunity §5.1 grants around one is a soft wrap opportunity like any other.
// This granted it unconditionally, so two inline-blocks written side by side
// under "white-space: nowrap" wrapped anyway — which is the one thing that value
// is for.

// atomicWrapLines lays out two atomic inlines too wide for their box and returns
// how many lines they came out on. atomicLines elsewhere in this package is a
// different fixture with the same shape of name.
func atomicWrapLines(t *testing.T, outer, inner string) int {
	t.Helper()
	ib := `<span style="display: inline-block; width: 60px; height: 10px; ` + inner + `"></span>`
	root := layoutOf(t, 600, `<div id="d">`+ib+ib+`</div>`,
		noDefaults+`#d { width: 100px; font-size: 10px; `+outer+` }`)
	return len(linesOf(t, root, "d"))
}

// TestALineDoesNotEndBesideAnAtomicInlineWhereWrappingIsForbidden.
func TestALineDoesNotEndBesideAnAtomicInlineWhereWrappingIsForbidden(t *testing.T) {
	for _, ws := range []string{"nowrap", "pre"} {
		if got := atomicWrapLines(t, "white-space: "+ws, ""); got != 1 {
			t.Errorf("white-space: %s put two inline-blocks on %d lines; the value "+
				"says a line may not end between them", ws, got)
		}
	}
}

// TestALineStillEndsBesideAnAtomicInlineWhereWrappingIsAllowed is the other half
// and the half §5.1 is mostly about: the opportunity is there by default, so two
// pictures that do not fit take a line each.
func TestALineStillEndsBesideAnAtomicInlineWhereWrappingIsAllowed(t *testing.T) {
	for _, ws := range []string{"normal", "pre-wrap", "pre-line"} {
		if got := atomicWrapLines(t, "white-space: "+ws, ""); got != 2 {
			t.Errorf("white-space: %s put two inline-blocks on %d lines; §5.1 grants "+
				"an opportunity around each one", ws, got)
		}
	}
}

// TestTheNearestCommonAncestorDecides, which is the whole of §5.1's sentence: it
// is neither box's own value that settles a boundary between them.
func TestTheNearestCommonAncestorDecides(t *testing.T) {
	// "nowrap" on the boxes themselves and not on what contains them. Their
	// nearest common ancestor is the wrapping div, so the line may end between
	// them — a value on a box says what happens *inside* it.
	if got := atomicWrapLines(t, "white-space: normal", "white-space: nowrap"); got != 2 {
		t.Errorf("two \"nowrap\" inline-blocks in a wrapping parent came out on %d "+
			"lines; the boundary between them belongs to the parent", got)
	}
	// And the mirror: the boxes wrap and the parent does not.
	if got := atomicWrapLines(t, "white-space: nowrap", "white-space: normal"); got != 1 {
		t.Errorf("two wrapping inline-blocks in a \"nowrap\" parent came out on %d "+
			"lines; the boundary between them belongs to the parent", got)
	}
}

// TestAWordJoinerStillHoldsAPicture. The near-side exception §5.1 states in as
// many words — "a&#8288;<img>" is written to keep them together — is a different
// rule and is untouched by this one.
func TestAWordJoinerStillHoldsAPicture(t *testing.T) {
	const ib = `<span style="display: inline-block; width: 60px; height: 10px"></span>`
	root := layoutOf(t, 600, `<div id="d">aaaa&#8288;`+ib+`</div>`,
		noDefaults+`#d { width: 100px; font-family: Courier; font-size: 10px }`)
	if got := len(linesOf(t, root, "d")); got != 1 {
		t.Errorf("a word joiner before an inline-block let the line end there: %d "+
			"lines, want 1", got)
	}
}
