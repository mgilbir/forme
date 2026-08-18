package layout

import (
	"testing"
)

// vertical-align on an inline-table.
//
// §17.4 puts an anonymous wrapper box round every table and lists the properties
// that belong to the wrapper rather than to the table: position, float, the
// margins and the offsets. vertical-align is not on that list and has to be
// there anyway, because for an inline-table the wrapper *is* the box that sits
// on the line — the table box inside it is block-level, and nothing ever asks a
// block-level box how it aligns.
//
// Left on the table the declaration did nothing whatever, so an inline-table
// asking for "vertical-align: top" was aligned on its baseline instead. The
// difference is the strut's descent: the line box came out three or four pixels
// taller than the table, and anything measuring the box around it — which is
// what a reftest does — measured a container that did not fit its contents.

// lineBoxAround returns the size of the wrapper's background, which is the line
// box the atomic inline sits on plus the wrapper's own border.
func lineBoxAround(t *testing.T, css string) Rect {
	t.Helper()
	ops := paintOf(t,
		`<div id="w"><div id="t"><div id="r"><div id="c"></div></div></div></div>`,
		`#w { background: rgb(0,0,255) }
		 #r { display: table-row }
		 #c { display: table-cell; width: 100px; height: 200px }
		 `+css)
	got := fillsOf(ops, blue)
	if len(got) != 1 {
		t.Fatalf("%d blue fills, want 1: %v", len(got), got)
	}
	return got[0]
}

// TestAnInlineTableIsAlignedByItsVerticalAlign.
//
// "top" puts the table's top at the top of the line box, so the line box is
// exactly as tall as the table and the box around it fits. On the baseline the
// strut hangs below it and the line box is taller — which is correct, and is the
// control that says the fixture can tell the two apart at all.
func TestAnInlineTableIsAlignedByItsVerticalAlign(t *testing.T) {
	top := lineBoxAround(t, `#t { display: inline-table; vertical-align: top }`)
	base := lineBoxAround(t, `#t { display: inline-table; vertical-align: baseline }`)

	if top.H != bgpx(200) {
		t.Errorf("the line box round a top-aligned inline-table is %v and the table "+
			"is 200px; nothing else on the line reaches above or below it", top.H)
	}
	if base.H <= top.H {
		t.Errorf("aligned on the baseline the line box is %v and top-aligned it is "+
			"%v; the strut's descent hangs below a baseline-aligned box, so the two "+
			"cannot be equal", base.H, top.H)
	}
}

// TestAnInlineTableAlignsLikeAnInlineBlock is the cross-check, and it is better
// evidence than a number: the two are atomic inlines under the same rules, so a
// value that moves one has to move the other by the same amount. The
// inline-block half worked before this and is untouched by it.
func TestAnInlineTableAlignsLikeAnInlineBlock(t *testing.T) {
	for _, value := range []string{"top", "bottom", "middle", "baseline", "text-top"} {
		table := lineBoxAround(t,
			`#t { display: inline-table; vertical-align: `+value+` }`)
		block := lineBoxAround(t,
			`#t { display: inline-block; vertical-align: `+value+` }
			 #r { display: block } #c { display: block }`)
		if table.H != block.H {
			t.Errorf("vertical-align: %s makes a line box %v round an inline-table "+
				"and %v round an inline-block of the same size", value, table.H, block.H)
		}
	}
}

// TestTheTableItselfIsNotAlignedTwice asserts that vertical-align on a
// block-level table changes nothing — neither its wrapper nor the table inside
// it is on a line, so neither has any business reading the value.
//
// It is worth saying plainly what this test is and is not, because it looks like
// a guard and is not one. The outcome it asserts is structural: the wrapper of a
// block-level table is block-level, the table box inside it is made block-level
// outright, and nothing ever asks a block-level box how it aligns. Four defects
// were planted trying to make it fail — the property taken off the wrapper list,
// the margins taken off it, the float taken off it, and the table left inline
// inside its own wrapper — and it passed under every one of them while its
// neighbours here failed.
//
// So it pins nothing, and a reader should not go looking for the line it
// protects. What it records is that the double application this change could
// have caused does not happen, which is a fact about the design worth writing
// down once and is the reason nothing else here has to defend against it.
func TestTheTableItselfIsNotAlignedTwice(t *testing.T) {
	plain := lineBoxAround(t, `#t { display: table }`)
	aligned := lineBoxAround(t, `#t { display: table; vertical-align: middle }`)
	if plain != aligned {
		t.Errorf("a block-level table is %v without vertical-align and %v with it; "+
			"neither it nor its wrapper is on a line", plain, aligned)
	}
}

// TestTheWrapperStillTakesTheOtherProperties. The list is shared, and a change
// to it is a change to every entry — so the ones that were already right are
// asserted here rather than assumed.
func TestTheWrapperStillTakesTheOtherProperties(t *testing.T) {
	// A margin on an inline-table moves the wrapper and is not applied again
	// inside it: 200px of table plus 2 × 20px of margin is 240, not 280.
	//
	// The height is what is measured because the width is not this fixture's to
	// state — #w is a block and fills the page whatever is inside it.
	got := lineBoxAround(t,
		`#t { display: inline-table; vertical-align: top; margin: 20px }`)
	if got.H != bgpx(240) {
		t.Errorf("the line box is %v tall, want 240 — the table's 200 and its two "+
			"20px margins", got.H)
	}
}

// TestAFloatedTableStillFloats guards the entry the list already had that is
// easiest to break by editing round it.
func TestAFloatedTableStillFloats(t *testing.T) {
	ops := paintOf(t,
		`<div id="t"><div id="r"><div id="c"></div></div></div><div id="after">x</div>`,
		`#t { display: table; float: left; background: rgb(0,0,255) }
		 #r { display: table-row } #c { display: table-cell; width: 50px; height: 50px }
		 #after { font-size: 20px }`)
	var text []Point
	for _, op := range ops {
		if v, ok := op.(DrawText); ok {
			text = append(text, v.At)
		}
	}
	if len(text) == 0 {
		t.Fatal("the fixture set no text")
	}
	if text[0].X < bgpx(50) {
		t.Errorf("the text after a floated table starts at %v; a 50px float is "+
			"beside it, not above it", text[0].X)
	}
}
