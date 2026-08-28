package layout

import "testing"

// A width written on a <col>, and what it is.
//
// §17.5.2.2 reads it as one more demand to be maximised with the cells' — "the
// maximum is that required by the cell with the largest maximum cell width, or
// the column 'width', whichever is larger" — which makes a narrow column holding
// a long sentence come out as wide as the sentence, so the declaration does
// nothing at all. No browser does that, and the working group's own references
// are drawn against the browsers: vertical-align-baseline-003's reference is a
// table with "col#middle { width: 80px }" and six lines of text wrapped inside
// it.
//
// So the width caps the column and the cells wrap in it, exactly as the same
// width written on the cell would.

// colTable lays out a one-column table and returns its width and height.
func colTable(t *testing.T, markup string) Rect {
	t.Helper()
	root := layoutOf(t, 600, markup,
		`table { border-spacing: 0; font-family: Courier; font-size: 20px }
		 td { padding: 0 }`)
	return find(t, root, "t").BorderRect
}

// TestAColumnWidthIsWhatTheColumnIs, and the proof is that it gives the same
// table as the same width written on the cell.
func TestAColumnWidthIsWhatTheColumnIs(t *testing.T) {
	col := colTable(t,
		`<table id="t"><col style="width:80px"><tr><td>aaaa bbbb cccc</td></tr></table>`)
	cell := colTable(t,
		`<table id="t"><tr><td style="width:80px">aaaa bbbb cccc</td></tr></table>`)
	if col != cell {
		t.Errorf("the width on the column gave %v and on the cell %v; they are the "+
			"same declaration about the same column", col, cell)
	}
	if col.W.Px() != 80 {
		t.Errorf("the table is %gpx wide, want 80", col.W.Px())
	}
	// And the sentence wrapped, which is what the width is for: without it the
	// table is one line and as wide as the sentence.
	wide := colTable(t, `<table id="t"><col><tr><td>aaaa bbbb cccc</td></tr></table>`)
	if wide.W <= col.W || wide.H >= col.H {
		t.Errorf("without a column width the table is %v and with one %v; the "+
			"declaration should have narrowed it and made it taller", wide, col)
	}
}

// TestAColumnIsNeverNarrowerThanItsContent. The width caps rather than fixes: a
// column cannot be narrower than the smallest thing in it, and the same floor is
// under a cell's own width — it is what stops "width: 1px" clipping a word.
func TestAColumnIsNeverNarrowerThanItsContent(t *testing.T) {
	// "aaaa" at 12px a character is 48, which is the column's minimum.
	got := colTable(t,
		`<table id="t"><col style="width:10px"><tr><td>aaaa bbbb cccc</td></tr></table>`)
	if got.W.Px() != 48 {
		t.Errorf("a 10px column holding a 48px word came out %gpx wide, want 48",
			got.W.Px())
	}
}

// TestAColumnGroupWithNoColumnsSpeaksForThem is §17.2's second rule, which the
// engine already followed: a group with no <col> children generates the columns
// itself, so its own width is theirs — they have no box of their own for one to
// be written on.
func TestAColumnGroupWithNoColumnsSpeaksForThem(t *testing.T) {
	got := colTable(t,
		`<table id="t"><colgroup style="width:80px"></colgroup>`+
			`<tr><td>aaaa bbbb cccc</td></tr></table>`)
	if got.W.Px() != 80 {
		t.Errorf("the table is %gpx wide, want the group's 80", got.W.Px())
	}
}

// TestAColumnWithNoWidthIsLeftToTheCells is the containment: this changes what a
// *declared* width does and nothing about a column that declares none.
func TestAColumnWithNoWidthIsLeftToTheCells(t *testing.T) {
	with := colTable(t, `<table id="t"><col><tr><td>aaaa bbbb cccc</td></tr></table>`)
	without := colTable(t, `<table id="t"><tr><td>aaaa bbbb cccc</td></tr></table>`)
	if with != without {
		t.Errorf("a column with no width gave %v and no column at all %v", with, without)
	}
}
