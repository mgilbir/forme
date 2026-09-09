package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A column group with no column children describes one column when it is an
// element and none when it is a pseudo-element.
//
// CSS 2.1 §17.2 takes the count from the group's "span" and from its column
// children, and with no children the span is the whole of it. "span" is an
// attribute: HTML's table model reads it off the element and defaults it to one,
// and this engine applies that default to any element a "display" value has made
// a column group of — which is what the suite's six "applies to elements with
// display set to table-column-group" tests are, each a <div> with a width and no
// children.
//
// A pseudo-element is not an element and carries no attributes at all, so there
// is no span to read and none to default. before-after-table-parts-001 puts
// "display: table-column-group" on both of one table's pseudo-elements, and its
// reference draws that table with a single column and nothing generated in it.
func TestAGeneratedColumnGroupDescribesNoColumns(t *testing.T) {
	// A table of one row and one cell, built out of "display" so that the markup
	// survives the parser wherever a column group is written. What says how many
	// columns it ended up with is its width: an empty second column costs a
	// border-spacing, and there is one here to cost.
	const css = `#t { display: table; border-spacing: 2px;
			font-family: Courier; font-size: 16px }
		#r { display: table-row } #c { display: table-cell; padding: 0 }`
	const doc = `<div id="t"><div id="r"><div id="c">Inner</div></div></div>`

	width := func(extra string) style.Unit {
		t.Helper()
		root := layoutOf(t, 4000, doc, noDefaults+css+extra)
		return find(t, root, "t").BorderRect.W
	}

	one := width("")
	for _, tc := range []struct{ extra, what string }{
		{` #t::before { content: "B"; display: table-column-group }`,
			"one generated column group"},
		{` #t::before, #t::after { content: "B"; display: table-column-group }`,
			"two of them"},
		{` #t::before, #t::after { content: ""; display: table-column-group }`,
			"two with no content at all"},
		{` #t::before, #t::after { content: "B"; display: table-column-group }
		   #t::before { width: 50px }`,
			"two, one of them carrying a width"},
	} {
		if got := width(tc.extra); got != one {
			t.Errorf("%s: the table is %v wide and the same table with one column "+
				"is %v — the generated group described a column of its own",
				tc.what, got, one)
		}
	}

	// And an element still does, which is the half the six "applies to" tests
	// need: a box with a width and no children is a column, and the table is as
	// wide as the width says.
	root := layoutOf(t, 4000,
		`<div id="t"><div id="g"></div><div id="r"><div id="c">Inner</div></div></div>`,
		noDefaults+css+` #g { display: table-column-group; width: 200px }`)
	if w := find(t, root, "t").BorderRect.W; w <= one {
		t.Errorf("a column group 200px wide left the table %v wide, no more than "+
			"the %v its one cell needs", w, one)
	}
}
