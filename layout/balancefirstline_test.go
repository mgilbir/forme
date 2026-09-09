package layout

import "testing"

// §5.1's balancing and §5.12.1's first line.
//
// Balancing is a *search*: it looks for the narrowest width that still makes as
// many lines as the full width did, and hands that width back as a cap for the
// real loop to break against. The search and the loop have to be breaking the
// same paragraph, and where ::first-line changes the type the first line is set
// in they were not — the search measured that line in the block's own type and
// settled on a width the loop could not reproduce.
//
// What that costs is a line, and a line is the one thing balancing may not cost:
// the search's whole constraint is "the same count as the full width", so a cap
// that produces a different count is an answer to a question nobody asked.

// balancedLines is how a block breaks with and without balancing, which is the
// comparison the invariant is stated over.
func balancedLines(t *testing.T, css, text string) (plain, balanced []string) {
	t.Helper()
	const src = `<div id="d">%</div>`
	body := replaceOnce(src, "%", text)
	plain = lineTextsOf(t, layoutOf(t, 10000, body, noDefaults+mono+css), "d")
	balanced = lineTextsOf(t, layoutOf(t, 10000, body,
		noDefaults+mono+css+`#d { text-wrap-style: balance }`), "d")
	return plain, balanced
}

// replaceOnce is strings.Replace with a count of one, spelled out so the test
// reads as a fixture rather than as string handling.
func replaceOnce(s, old, new string) string {
	for i := 0; i+len(old) <= len(s); i++ {
		if s[i:i+len(old)] == old {
			return s[:i] + new + s[i+len(old):]
		}
	}
	return s
}

// TestBalancingAFirstLineDoesNotCostALine.
//
// The word-spacing is on ::first-line alone, so the first line is wider than the
// search would think and every later line is not. A search that measured it in
// the block's type chose a cap that fitted a first line the layout then could
// not fit, and the paragraph came out a line longer than the same paragraph
// unbalanced — which is the failure, and it is visible without knowing what the
// balanced break points should be.
func TestBalancingAFirstLineDoesNotCostALine(t *testing.T) {
	const css = `#d { width: 20ch } #d::first-line { word-spacing: 4ch }`
	const text = `aa bb cc dd ee ff gg hh ii jj kk ll`

	plain, balanced := balancedLines(t, css, text)
	if len(plain) < 2 {
		t.Fatalf("the fixture is one line unbalanced (%q), so there is nothing "+
			"for balancing to be wrong about", plain)
	}
	if len(balanced) != len(plain) {
		t.Errorf("balanced the block is %d lines and unbalanced it is %d: %q "+
			"against %q. Balancing chooses the narrowest width with the *same* "+
			"count, so a different count is a width the search measured in a "+
			"type the first line is not set in", len(balanced), len(plain),
			balanced, plain)
	}
}

// TestBalancingAFirstLineChoosesTheWidthTheLoopWillUse is the same fault where
// it does not cost a line, and it is the sharper of the two: at this width both
// readings make two lines, and they make them in different places. The search
// that measured the first line without its word-spacing thinks five words fit
// there, so it settles on a width the loop then fills with three.
//
// The break point is written down rather than derived because deriving it is
// the search's own job, and a test that recomputed it would be the
// implementation twice. What makes it checkable is the line beside it: the same
// block with the spacing on the block instead of on its first line balances to a
// break of its own, and the two are not the same answer by accident.
func TestBalancingAFirstLineChoosesTheWidthTheLoopWillUse(t *testing.T) {
	const css = `#d { width: 30ch } #d::first-line { word-spacing: 4ch }`
	const text = `aa bb cc dd ee ff gg hh ii jj kk ll`

	_, balanced := balancedLines(t, css, text)
	if len(balanced) != 2 || balanced[0] != "aa bb cc" {
		t.Errorf("balanced, the block breaks as %q; the first line carries a "+
			"four-character word-spacing that none of the others do, and a "+
			"search that measured it without one puts more on it than the "+
			"layout can", balanced)
	}
}

// TestBalancingWithoutAFirstLineIsUnchanged is the control. The same fixture
// with the word-spacing on the block rather than on its first line balances the
// way it always did, so what the test above catches is the pseudo-element and
// not the search.
func TestBalancingWithoutAFirstLineIsUnchanged(t *testing.T) {
	const css = `#d { width: 30ch; word-spacing: 4ch }`
	const text = `aa bb cc dd ee ff gg hh ii jj kk ll`

	plain, balanced := balancedLines(t, css, text)
	if len(balanced) != len(plain) {
		t.Errorf("balanced the block is %d lines and unbalanced it is %d: %q "+
			"against %q", len(balanced), len(plain), balanced, plain)
	}
}
