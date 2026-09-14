package shape

import "testing"

// TestTheDecompositionTableIsShallowerThanTheBound records that this bound sits
// above what the data it walks can produce.
//
// maxDecompositionDepth guards a recursion over Unicode's canonical
// decompositions, and that table is generated from the UCD rather than read out
// of a document — so unlike the bounds on a font or a stylesheet, nothing a page
// can send reaches it. What it defends against is a table with a chain deeper
// than the recursion expects, or a cycle, which the UCD does not contain and a
// generator bug could introduce.
//
// So there is no input that exercises it, and a test claiming to would be
// asserting nothing. This asserts the relationship instead, over the whole of
// Unicode: the deepest chain the table actually holds, against the bound that
// walks it. If a future UCD adds a deeper one — or a generator starts emitting a
// cycle — this fails and names the character, rather than the recursion quietly
// returning false and a glyph quietly not being decomposed.
func TestTheDecompositionTableIsShallowerThanTheBound(t *testing.T) {
	// depthOf is the recursion decomposeInto performs, counted rather than
	// followed: only the first part is ever taken further, which is the rule
	// the walk itself relies on.
	var depthOf func(r rune, seen int) int
	depthOf = func(r rune, seen int) int {
		if seen > maxDecompositionDepth+4 {
			t.Fatalf("U+%04X decomposes without end; the table holds a cycle", r)
		}
		a, _, ok := canonicalDecompose(r)
		if !ok {
			return 0
		}
		return 1 + depthOf(a, seen+1)
	}

	deepest, at := 0, rune(0)
	for r := rune(0); r <= 0x10FFFF; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue // surrogates are not characters
		}
		if d := depthOf(r, 0); d > deepest {
			deepest, at = d, r
		}
	}

	if deepest == 0 {
		t.Fatal("nothing in Unicode decomposed at all; the table is not being read")
	}
	if deepest >= maxDecompositionDepth {
		t.Errorf("U+%04X decomposes %d deep and the bound that walks it is %d; the "+
			"bound no longer sits above the data, so a decomposition is being cut "+
			"short", at, deepest, maxDecompositionDepth)
	}
	t.Logf("the deepest canonical decomposition is U+%04X at %d; the bound is %d",
		at, deepest, maxDecompositionDepth)
}
