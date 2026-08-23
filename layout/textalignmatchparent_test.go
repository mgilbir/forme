package layout

import "testing"

// text-align: match-parent, CSS Text §7.1.
//
// "This value behaves the same as inherit, except that an inherited value of
// start or end is calculated against the parent's direction value and results in
// a computed value of either left or right."
//
// The first half is nothing — text-align inherits already — and the second half
// is the whole property. It is for a paragraph of one direction quoted inside a
// block of another: the quote is aligned with the text around it rather than
// with itself, which is what a reader of the surrounding language expects.
//
// Positions below are arithmetic rather than recorded numbers, as in
// textalign_test.go beside this: Courier is 600/1000, so "abcdef" at 20px is
// 72px, and in a 300px line left is 0 and right is 228.

// matchParentCSS is a 300px block of Courier with the value on every inner div,
// which is how the suite's own fixture writes it.
const matchParentCSS = `#p, #outer { font-family: Courier; font-size: 20px; width: 300px }
	#p { text-align: match-parent }`

// TestMatchParentResolvesAgainstTheParentsDirection is the property, and the
// fixture is text-align-match-parent-01's own table: the parent's value and
// direction against the child's, eight rows of which the four that matter are
// the ones where the two disagree.
func TestMatchParentResolvesAgainstTheParentsDirection(t *testing.T) {
	for _, tc := range []struct {
		align, parentDir, childDir string
		want                       float64
	}{
		// start, and the parent's direction decides — including where the
		// child's is the other one, which is the case with no other answer.
		{"start", "ltr", "ltr", 0},
		{"start", "ltr", "rtl", 0},
		{"start", "rtl", "ltr", 228},
		{"start", "rtl", "rtl", 228},
		// end is the same rule at the other edge.
		{"end", "ltr", "ltr", 228},
		{"end", "ltr", "rtl", 228},
		{"end", "rtl", "ltr", 0},
		{"end", "rtl", "rtl", 0},
		// left and right are physical and no direction touches them.
		{"left", "ltr", "rtl", 0},
		{"left", "rtl", "ltr", 0},
		{"right", "ltr", "rtl", 228},
		{"right", "rtl", "ltr", 228},
	} {
		root := layoutOf(t, 600,
			`<div id="outer" dir="`+tc.parentDir+`"><div id="p" dir="`+tc.childDir+
				`">abcdef</div></div>`,
			matchParentCSS+` #outer { text-align: `+tc.align+` }`)
		if got := lineX(t, root, "p"); got != tc.want {
			t.Errorf("a %s parent in %s with a %s child: the line is at %gpx, want %g",
				tc.align, tc.parentDir, tc.childDir, got, tc.want)
		}
	}
}

// TestMatchParentIsNotTheSameAsInheriting is what says the rows above are
// measuring something. Without the value the child inherits "start" and resolves
// it against its *own* direction, so the two disagree exactly where the two
// directions do — and a fixture whose parent and child share a direction would
// pass with the property unimplemented.
func TestMatchParentIsNotTheSameAsInheriting(t *testing.T) {
	const markup = `<div id="outer" dir="ltr"><div id="p" dir="rtl">abcdef</div></div>`
	inherited := layoutOf(t, 600, markup,
		`#p, #outer { font-family: Courier; font-size: 20px; width: 300px }
		 #outer { text-align: start }`)
	if got := lineX(t, inherited, "p"); got != 228 {
		t.Fatalf("without match-parent the line is at %gpx; an inherited start "+
			"resolves against the child's own direction, which is rtl, so 228", got)
	}
	matched := layoutOf(t, 600, markup, matchParentCSS+` #outer { text-align: start }`)
	if got := lineX(t, matched, "p"); got != 0 {
		t.Errorf("with match-parent the line is at %gpx, want 0 — the parent's "+
			"direction is ltr and its start is the left edge", got)
	}
}

// TestMatchParentOffTheTopOfTheTreeStaysLogical is the root case, and the one
// that is easy to get backwards.
//
// The specification computes match-parent against the parent's value; the root
// has no parent, so there is nothing to make physical against and the value
// stays "start". text-align-match-parent-root-logical is built to catch the
// other reading: the root is dir=rtl, the body inside it is dir=ltr, and the
// line must come out flush left. Answering the root's own direction gives right.
func TestMatchParentOffTheTopOfTheTreeStaysLogical(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="outer" dir="ltr"><div id="p">abcdef</div></div>`,
		`html { direction: rtl; text-align: match-parent }
		 #p, #outer { font-family: Courier; font-size: 20px; width: 300px }`)
	if got := lineX(t, root, "p"); got != 0 {
		t.Errorf("the line is at %gpx, want 0: match-parent on the root is start "+
			"and stays logical, so the ltr block inside it aligns left", got)
	}
	// And the same document in the other direction throughout, so the row above
	// is not passing because the answer is left whatever happens.
	rtl := layoutOf(t, 600,
		`<div id="outer" dir="rtl"><div id="p">abcdef</div></div>`,
		`html { direction: rtl; text-align: match-parent }
		 #p, #outer { font-family: Courier; font-size: 20px; width: 300px }`)
	if got := lineX(t, rtl, "p"); got != 228 {
		t.Errorf("the control is at %gpx, want 228", got)
	}
}

// TestAChainOfMatchParentsReachesThroughToTheValue. "div > div { text-align:
// match-parent }" makes every div in a nest one, which is how the suite writes
// it, and each link has to be answered with the direction of the box above it
// until one of them has a value that is not match-parent.
func TestAChainOfMatchParentsReachesThroughToTheValue(t *testing.T) {
	root := layoutOf(t, 600,
		`<div id="outer" dir="rtl"><div dir="ltr"><div dir="ltr"><div id="p" dir="ltr">abcdef`+
			`</div></div></div></div>`,
		`#outer { text-align: start; width: 300px }
		 div { text-align: match-parent; font-family: Courier; font-size: 20px; width: 300px }`)
	if got := lineX(t, root, "p"); got != 228 {
		t.Errorf("the line is at %gpx, want 228: the value at the top of the chain "+
			"is start and the block that holds it is rtl", got)
	}
}

// TestTheOtherValuesAreUnchanged is the containment case. alignmentOf was a
// switch and is now a walk, and every document in the suite goes through it.
func TestTheOtherValuesAreUnchanged(t *testing.T) {
	for _, tc := range []struct {
		value string
		dir   string
		want  float64
	}{
		{"left", "ltr", 0}, {"right", "ltr", 228}, {"center", "ltr", 114},
		{"start", "ltr", 0}, {"end", "ltr", 228},
		{"start", "rtl", 228}, {"end", "rtl", 0},
		{"left", "rtl", 0}, {"right", "rtl", 228},
		// Nothing at all, and something nobody has heard of, are both start.
		{"", "ltr", 0}, {"", "rtl", 228},
		{"not-a-value", "ltr", 0}, {"not-a-value", "rtl", 228},
	} {
		css := `#p { font-family: Courier; font-size: 20px; width: 300px }`
		if tc.value != "" {
			css += ` #p { text-align: ` + tc.value + ` }`
		}
		root := layoutOf(t, 600, `<div id="p" dir="`+tc.dir+`">abcdef</div>`, css)
		if got := lineX(t, root, "p"); got != tc.want {
			t.Errorf("text-align:%q in %s put the line at %gpx, want %g",
				tc.value, tc.dir, got, tc.want)
		}
	}
}
