package html

import (
	"strings"
	"testing"
)

// TestContentWrittenInsideATableGoesBeforeIt is HTML's foster parenting, which
// every browser does and which this parser did not.
//
// Content that is not table content, written inside a table, is inserted
// immediately in front of the table. Left where it stands it is a table child,
// and CSS 2.1 §17.2.1 then wraps it in an anonymous row of its own — so a
// paragraph between two rows became a row, and the table gained a line nobody
// asked for. It was accepted in silence.
func TestContentWrittenInsideATableGoesBeforeIt(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"a block straight inside a table",
			`<table><div>x</div></table>`,
			"#doc(<html>(<head><body>(<div>('x')<table>)))"},
		{"a block between two rows",
			`<table><tr><td>a</td></tr><div>x</div><tr><td>b</td></tr></table>`,
			"#doc(<html>(<head><body>(<div>('x')<table>(<tr>(<td>('a'))<tr>(<td>('b'))))))"},
		{"a block inside a row",
			`<table><tr><div>x</div></tr></table>`,
			"#doc(<html>(<head><body>(<div>('x')<table>(<tr>))))"},
		{"a block inside a row group",
			`<table><tbody><div>x</div></tbody></table>`,
			"#doc(<html>(<head><body>(<div>('x')<table>(<tbody>))))"},
		{"text straight inside a table",
			`<table>stray</table>`,
			"#doc(<html>(<head><body>('stray'<table>)))"},
		{"text between two rows",
			`<table><tr><td>a</td></tr>stray</table>`,
			"#doc(<html>(<head><body>('stray'<table>(<tr>(<td>('a'))))))"},
	} {
		doc, errs, ok := Parse(tc.src)
		if got := shapeOfTree(doc); got != tc.want {
			t.Errorf("%s: %s\n   want %s", tc.name, got, tc.want)
		}
		if ok {
			t.Errorf("%s: accepted without a word; the markup is wrong even though "+
				"the reading of it is a browser's", tc.name)
		}
		var said bool
		for _, e := range errs {
			if strings.Contains(e.Message, "belongs before the table") {
				said = true
			}
		}
		if !said {
			t.Errorf("%s: reported %v, none of them saying where the content went",
				tc.name, errs)
		}
	}
}

// TestTableContentStaysInTheTable is the control, and it is most of what a
// table is. Moving any of this out would take the table apart.
func TestTableContentStaysInTheTable(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"rows and cells", `<table><tr><td>a</td><td>b</td></tr></table>`,
			"#doc(<html>(<head><body>(<table>(<tr>(<td>('a')<td>('b'))))))"},
		{"a row group", `<table><tbody><tr><td>a</td></tr></tbody></table>`,
			"#doc(<html>(<head><body>(<table>(<tbody>(<tr>(<td>('a')))))))"},
		{"a caption", `<table><caption>c</caption><tr><td>a</td></tr></table>`,
			"#doc(<html>(<head><body>(<table>(<caption>('c')<tr>(<td>('a'))))))"},
		{"a column group", `<table><colgroup><col></colgroup><tr><td>a</td></tr></table>`,
			"#doc(<html>(<head><body>(<table>(<colgroup>(<col>)<tr>(<td>('a'))))))"},
		// A cell written straight inside a table is not what HTML says either,
		// and the row it implies is an anonymous box CSS makes. Moving the cell
		// out of the table would lose it.
		{"a cell with no row", `<table><td>a</td></table>`,
			"#doc(<html>(<head><body>(<table>(<td>('a')))))"},
		{"white space between rows", `<table><tr><td>a</td></tr> <tr><td>b</td></tr></table>`,
			"#doc(<html>(<head><body>(<table>(<tr>(<td>('a'))<tr>(<td>('b'))))))"},
		{"content inside a cell", `<table><tr><td><div>x</div></td></tr></table>`,
			"#doc(<html>(<head><body>(<table>(<tr>(<td>(<div>('x')))))))"},
		{"content inside a caption", `<table><caption><div>x</div></caption></table>`,
			"#doc(<html>(<head><body>(<table>(<caption>(<div>('x'))))))"},
	} {
		doc, _, _ := Parse(tc.src)
		if got := shapeOfTree(doc); got != tc.want {
			t.Errorf("%s: %s\n   want %s", tc.name, got, tc.want)
		}
	}
}

// TestFosterParentingStopsAtTheInnermostTable: content written inside a table
// nested in a cell belongs before *that* table and not before the outer one.
func TestFosterParentingStopsAtTheInnermostTable(t *testing.T) {
	doc, _, _ := Parse(`<table><tr><td><table><div>x</div></table></td></tr></table>`)
	const want = "#doc(<html>(<head><body>(<table>(<tr>(<td>(<div>('x')<table>))))))"
	if got := shapeOfTree(doc); got != want {
		t.Errorf("%s\n   want %s", got, want)
	}
}
