package html

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// The shapes of markup whose cost was not linear in their length, each guarded
// by the shape of its curve rather than by a time.
//
// A time cannot be both tight enough to catch a quadratic and loose enough for
// the race detector's job, which runs everything ten or more times slower. A
// ratio is the same on any machine: four times the input is four times the work
// when the parse is linear and sixteen when it is not, and eight is between
// them with room on both sides.

// scaling measures one shape at n and at four times n, and says what one run
// of each took and the factor between them.
//
// The two are timed in windows of equal length, turn about: four runs of the
// small case against one of the large, the least of nine rounds each. Timed
// one after the other, a run at n took under a millisecond, and whatever else
// the machine was doing in that millisecond landed on one side of the ratio
// and not the other — the rest of the suite running beside it put a linear
// curve at 8.3 for four. Windows the same length, interleaved, are slowed
// alike by a busy machine, and the ratio is the curve's.
func scaling(small, large func()) (lo, hi time.Duration, ratio float64) {
	bestSmall, bestLarge := time.Duration(1<<62), time.Duration(1<<62)
	for r := 0; r < 9; r++ {
		start := time.Now()
		for i := 0; i < 4; i++ {
			small()
		}
		if el := time.Since(start); el < bestSmall {
			bestSmall = el
		}
		start = time.Now()
		large()
		if el := time.Since(start); el < bestLarge {
			bestLarge = el
		}
	}
	lo, hi = bestSmall/4, bestLarge
	if lo <= 0 {
		return lo, hi, 0
	}
	return lo, hi, float64(hi) / float64(lo)
}

// attrs is n distinct attributes, " a00000 a00001 …". The names are all one
// length, so that comparing one with a name being looked up costs the same
// whichever it is: with "a0" to "a7999", the ones as long as "lang" are a
// tenth of a large fixture and most of a small one, and a quadratic measured
// on them came out at half its real slope.
func attrs(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, " a%05d", i)
	}
	return b.String()
}

// TestAttributesAreBoundedPerElement is maxAttributes, at its boundary, on the
// two paths an element gets attributes by: one tag carrying them, and a frame
// element's start tag written again and again with one each.
//
// The boundary is what makes this a test of the comparison rather than of the
// existence of a bound: the last attribute kept and the first one dropped are
// both named, and the one dropped has to be reported as a limit reached rather
// than as markup that is wrong.
func TestAttributesAreBoundedPerElement(t *testing.T) {
	var many strings.Builder
	for i := 0; i <= maxAttributes; i++ {
		fmt.Fprintf(&many, "<body a%05d>", i)
	}
	last, first := fmt.Sprintf("a%05d", maxAttributes-1), fmt.Sprintf("a%05d", maxAttributes)
	for _, tc := range []struct{ name, src, element string }{
		{"one tag", "<div" + attrs(maxAttributes+1) + ">x</div>", "div"},
		{"one frame tag", "<body" + attrs(maxAttributes+1) + ">x", "body"},
		{"a frame tag written again", many.String() + "x", "body"},
	} {
		doc, errs, ok := Parse(tc.src)
		el := doc.Element(tc.element)
		if el == nil {
			t.Fatalf("%s: no <%s>", tc.name, tc.element)
		}
		if len(el.Attrs) != maxAttributes || !el.HasAttr(last) {
			t.Errorf("%s: <%s> kept %d attributes; it should keep the first %d, up to %q",
				tc.name, tc.element, len(el.Attrs), maxAttributes, last)
		}
		if el.HasAttr(first) {
			t.Errorf("%s: <%s> kept %q, which is past the bound", tc.name, tc.element, first)
		}
		reported := false
		for _, e := range errs {
			if e.Limit && strings.Contains(e.Message, `"`+first+`"`) {
				reported = true
			}
		}
		if ok || !reported {
			t.Errorf("%s: the attributes dropped were not reported as a limit naming %q: %v",
				tc.name, first, errs)
		}
	}

	// And exactly at the bound nothing is dropped and nothing is said.
	doc, errs, _ := Parse("<div" + attrs(maxAttributes) + ">x</div>")
	if el := doc.Element("div"); len(el.Attrs) != maxAttributes || len(errs) != 0 {
		t.Errorf("a <div> with exactly %d attributes kept %d and reported %v",
			maxAttributes, len(el.Attrs), errs)
	}
}

// TestFosterParentingIsLinear is content written inside a table, which goes in
// front of it.
//
// Each node fostered out of a table was placed by finding the table among its
// parent's children from the front — and the table sits behind every node
// fostered before it, so the search grew by one with each: eighty thousand
// "<br>" took 1.4 seconds and four times as long with each doubling. Text takes
// the same path to find the node it merges with, so a run of text between each
// element is one of the shapes.
func TestFosterParentingIsLinear(t *testing.T) {
	for _, tc := range []struct{ name, each string }{
		{"a void element", "<br>"},
		{"an element with an end tag", "<i></i>"},
		{"text and an element", "x<br>"},
	} {
		build := func(n int) string { return "<table>" + strings.Repeat(tc.each, n) + "</table>" }

		// The fixture has to be fostered, or the ratio measures an ordinary
		// append: everything but the table lands in front of it.
		doc, _, _ := Parse(build(3))
		body := doc.Element("body")
		if kids := body.Children; len(kids) == 0 || kids[len(kids)-1].Name != "table" ||
			len(kids[len(kids)-1].Children) != 0 {
			t.Fatalf("%s: the fixture was not fostered: %s", tc.name, shapeOfTree(doc))
		}

		const n = 10000
		small, large := build(n), build(4*n)
		lo, hi, ratio := scaling(func() { Parse(small) }, func() { Parse(large) })
		if lo <= 0 {
			t.Fatalf("%s: %d fostered in %v; there is nothing to compare", tc.name, n, lo)
		}
		if ratio > 8 {
			t.Errorf("%s: %d fostered in %v and %d in %v, a factor of %.1f for four times "+
				"the input; putting a node in front of a table must not walk what was "+
				"put there before it", tc.name, n, lo, 4*n, hi, ratio)
		}
	}
}
