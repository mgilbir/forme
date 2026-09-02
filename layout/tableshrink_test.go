package layout

import "testing"

// A width declared on a cell, and what happens when the table has less room
// than it asks for.
//
// §17.5.2.2 says "if the specified width of the cell is greater than MCW, W is
// the minimum cell width", which read literally makes the declaration a floor:
// a table of one empty "width: 100px" cell would be a hundred pixels wide in
// fifty pixels of room, and would sit on whatever is in the other fifty. No
// browser does that, and the suite says so from both sides —
// table-sizing-with-adjacent-floats puts such a table in a fifty-pixel band
// beside two floats and draws it fifty wide, while inline-formatting-context-015's
// own reference declares a 110px cell in a 200px table and expects the 110.
//
// So a declared width settles the column wherever the table has room for it and
// gives way where it has not, down to what the content needs and no further.

const shrinkCSS = `table { border-spacing: 0 } td { padding: 0 }
	body { font-family: Courier; font-size: 20px }`

// TestADeclaredCellWidthGivesWayToTheRoomThereIs.
func TestADeclaredCellWidthGivesWayToTheRoomThereIs(t *testing.T) {
	// 66px of page is 50px of body, the margin being 8px a side.
	root := layoutOf(t, 66,
		`<table id="t"><tr><td style="width:100px;height:10px"></td></tr></table>`,
		shrinkCSS)
	if got := find(t, root, "t").BorderRect.W; got.Px() != 50 {
		t.Errorf("a table of one empty 100px cell in 50px of room came out %gpx "+
			"wide, want 50 — the declaration is what gives way, not the room",
			got.Px())
	}
}

// TestADeclaredCellWidthIsKeptWhereThereIsRoomForIt is the other side of it,
// and is inline-formatting-context-015's reference: a 110px cell in a 200px
// table beside a column of text that would happily take the space.
func TestADeclaredCellWidthIsKeptWhereThereIsRoomForIt(t *testing.T) {
	root := layoutOf(t, 600,
		`<table id="t" style="width:200px"><tr>`+
			`<td id="c" style="width:110px"></td>`+
			`<td>Filler Text Filler Text</td></tr></table>`, shrinkCSS)
	if got := find(t, root, "c").BorderRect.W; got.Px() != 110 {
		t.Errorf("the declared column came out %gpx wide in a 200px table, "+
			"want 110 — a width the table has room for is not a preference",
			got.Px())
	}
}

// TestAColumnStillCannotGoBelowItsContent. What gives way is the declaration;
// the content's own minimum is where the giving stops.
func TestAColumnStillCannotGoBelowItsContent(t *testing.T) {
	// "MMMM" in 20px Courier is four characters of 12px: 48.
	wide := layoutOf(t, 600,
		`<table id="t"><tr><td>MMMM</td></tr></table>`, shrinkCSS)
	want := find(t, wide, "t").BorderRect.W
	if want.Px() != 48 {
		t.Fatalf("the word measures %gpx, so this fixture cannot say what it "+
			"means to say", want.Px())
	}
	got := layoutOf(t, 46, // 30px of body, less than the word
		`<table id="t"><tr><td style="width:100px">MMMM</td></tr></table>`,
		shrinkCSS)
	if w := find(t, got, "t").BorderRect.W; w != want {
		t.Errorf("a table holding a %gpx word in 30px of room came out %gpx "+
			"wide, want %g — the content's minimum is a floor and not a "+
			"preference", want.Px(), w.Px(), want.Px())
	}
}

// TestATableBesideFloatsTakesTheNarrowestBandItSpans is the suite's
// table-sizing-with-adjacent-floats: a table whose declared column would not
// fit beside the floats it spans shrinks into the band rather than dropping
// below them.
//
// The two floats are the point. The first leaves 200px beside it and the second
// leaves 50, and a table that only looked at the band at its own top would take
// the 200, find itself lying across the second float once it had a height, and
// drop below both.
func TestATableBesideFloatsTakesTheNarrowestBandItSpans(t *testing.T) {
	root := layoutOf(t, 266,
		`<div style="width:250px">`+
			`<div style="float:right;width:50px;height:50px"></div>`+
			`<div style="clear:right;float:right;width:200px;height:50px"></div>`+
			`<table id="t"><tr><td style="width:100px;height:100px"></td></tr></table>`+
			`</div>`, shrinkCSS)
	got := find(t, root, "t").BorderRect
	if got.W.Px() != 50 {
		t.Errorf("the table came out %gpx wide, want 50 — it has to fit the "+
			"narrowest band it spans and not the widest", got.W.Px())
	}
	if got.Y.Px() != 8 {
		t.Errorf("the table starts at y=%g, want 8 — it fits beside the floats "+
			"and so should not have dropped below them", got.Y.Px())
	}
}

// TestTheColumnsGiveBackInProportionToWhatTheyAsked. Two declarations that
// cannot both be met are not met half each: each column gives back a share of
// what it asked for beyond its content, which is the same rule the surplus
// above the minimum is shared by, run backwards.
func TestTheColumnsGiveBackInProportionToWhatTheyAsked(t *testing.T) {
	root := layoutOf(t, 76, // 60px of body
		`<table id="t"><tr>`+
			`<td id="a" style="width:100px;height:10px"></td>`+
			`<td id="b" style="width:20px"></td></tr></table>`, shrinkCSS)
	a := find(t, root, "a").BorderRect.W
	b := find(t, root, "b").BorderRect.W
	if a.Px() != 50 || b.Px() != 10 {
		t.Errorf("the columns came out %gpx and %gpx of 60, want 50 and 10 — "+
			"sixty pixels shared over demands of a hundred and twenty",
			a.Px(), b.Px())
	}
}

// TestASpanningCellsContentIsAFloorToo. What a cell spanning two columns needs
// is a floor under those columns together, and it has to travel with the other
// two widths — a table squeezed past it would clip the cell that spans it.
func TestASpanningCellsContentIsAFloorToo(t *testing.T) {
	// "MMMMMMMM" in 20px Courier is eight characters of 12px: 96.
	root := layoutOf(t, 46, // 30px of body, far less than the word
		`<table id="t"><tr><td style="width:5px"></td><td style="width:5px"></td></tr>`+
			`<tr><td colspan="2">MMMMMMMM</td></tr></table>`, shrinkCSS)
	if got := find(t, root, "t").BorderRect.W; got.Px() != 96 {
		t.Errorf("a table holding a 96px word across both its columns came out "+
			"%gpx wide in 30px of room, want 96", got.Px())
	}
}
