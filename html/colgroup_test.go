package html

import "testing"

// What ends a <colgroup>, and what ends a <caption>.
//
// Both have optional end tags, and both are inside a table where the next thing
// is usually a cell. HTML's "in column group" insertion mode has exactly one
// positive case — a <col> — and sends everything else to "anything else", which
// pops the colgroup and reprocesses the tag; its "in caption" mode ends the
// caption on any of the table's own elements. So "<colgroup><td>" is a cell of
// the table, not something inside the column group, and the same is true after
// a caption.
//
// It matters more than a stray element in a tree: a cell inside a column group
// is not a cell at all. The suite's border-conflict-style-107 writes
// "<colgroup class=loser><td class=winner>" and loses the whole table.

func TestATableCellEndsAColumnGroup(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"cell", `<table><colgroup><td>x</td></table>`},
		{"header cell", `<table><colgroup><th>x</th></table>`},
		{"row", `<table><colgroup><tr><td>x</td></tr></table>`},
		{"body", `<table><colgroup><tbody><td>x</td></tbody></table>`},
		{"caption", `<table><colgroup><caption>x</caption></table>`},
		// A second column group ends the first, which the old table did not say
		// either.
		{"another column group", `<table><colgroup><colgroup><td>x</td></table>`},
	} {
		doc := mustParseHTML(t, tc.src)
		cg := doc.Element("colgroup")
		if cg == nil {
			t.Errorf("%s: no <colgroup>:\n%s", tc.name, tree(doc))
			continue
		}
		if len(cg.Children) != 0 {
			t.Errorf("%s: the column group holds %d children, want none:\n%s",
				tc.name, len(cg.Children), tree(doc))
		}
	}
}

// TestAColumnStaysInItsColumnGroup is the one positive case, and without it the
// rule above would empty every column group there is.
func TestAColumnStaysInItsColumnGroup(t *testing.T) {
	doc := mustParseHTML(t, `<table><colgroup><col><col></colgroup><td>x</td></table>`)
	cg := doc.Element("colgroup")
	if cg == nil {
		t.Fatalf("no <colgroup>:\n%s", tree(doc))
	}
	if len(cg.Children) != 2 {
		t.Errorf("the column group holds %d children, want its two <col>s:\n%s",
			len(cg.Children), tree(doc))
	}
}

// TestATableCellEndsACaption, the other half of the same shape.
func TestATableCellEndsACaption(t *testing.T) {
	for _, src := range []string{
		`<table><caption>c<td>x</td></table>`,
		`<table><caption>c<th>x</th></table>`,
		`<table><caption>c<col></table>`,
		`<table><caption>c<caption>d</caption></table>`,
	} {
		doc := mustParseHTML(t, src)
		cap := doc.Element("caption")
		if cap == nil {
			t.Errorf("%s: no <caption>:\n%s", src, tree(doc))
			continue
		}
		if got := cap.TextContent(); got != "c" {
			t.Errorf("%s: the caption holds %q, want %q:\n%s",
				src, got, "c", tree(doc))
		}
	}
}
