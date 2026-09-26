package style

import (
	"reflect"
	"testing"

	"github.com/mgilbir/forme/css"
)

// TestGridTemplateAreasTokenizeByIdentCodePoints is CSS Grid 2 §7.3's
// tokenization of each string, run on the specification's own examples and on
// the edges of each of its four kinds of token.
func TestGridTemplateAreasTokenizeByIdentCodePoints(t *testing.T) {
	area := func(name string, row, column, rows, columns int) GridArea {
		return GridArea{Name: name, Row: row, Column: column, Rows: rows, Columns: columns}
	}
	for _, c := range []struct {
		value string
		want  GridTemplate
	}{
		// §7.3's example 1, the page of a game: a head across the top, a stats
		// column down the left, and so on.
		{`"title board" "stats board" "score ctrls"`, GridTemplate{Rows: 3, Columns: 2,
			Areas: []GridArea{area("title", 0, 0, 1, 1), area("board", 0, 1, 2, 1),
				area("stats", 1, 0, 1, 1), area("score", 2, 0, 1, 1), area("ctrls", 2, 1, 1, 1)}}},
		// §7.3's example 2: null cells, as one dot and as a run of them.
		{`"head head" "nav  main" "foot ...."`, GridTemplate{Rows: 3, Columns: 2,
			Areas: []GridArea{area("head", 0, 0, 1, 2), area("nav", 1, 0, 1, 1),
				area("main", 1, 1, 1, 1), area("foot", 2, 0, 1, 1)}}},
		// The note under the rules: names that are not <ident>s.
		{`"1st 2nd 3rd"`, GridTemplate{Rows: 1, Columns: 3,
			Areas: []GridArea{area("1st", 0, 0, 1, 1), area("2nd", 0, 1, 1, 1),
				area("3rd", 0, 2, 1, 1)}}},
		// Longest match: "." is not an ident code point, so it ends a name and
		// a name ends a run of dots.
		{`"a.b"`, GridTemplate{Rows: 1, Columns: 3,
			Areas: []GridArea{area("a", 0, 0, 1, 1), area("b", 0, 2, 1, 1)}}},
		{`"..a.."`, GridTemplate{Rows: 1, Columns: 3,
			Areas: []GridArea{area("a", 0, 1, 1, 1)}}},
		{`"."`, GridTemplate{Rows: 1, Columns: 1}},
		// Ident code points outside ASCII, and "-" and "_" and digits inside.
		{`"é 区域 -x_1 --"`, GridTemplate{Rows: 1, Columns: 4,
			Areas: []GridArea{area("é", 0, 0, 1, 1), area("区域", 0, 1, 1, 1),
				area("-x_1", 0, 2, 1, 1), area("--", 0, 3, 1, 1)}}},
		// A byte order mark is in CSS Syntax's list of non-ASCII ident code
		// points, so it is part of a name and does not separate two.
		{"\"a\ufeffb c\"", GridTemplate{Rows: 1, Columns: 2,
			Areas: []GridArea{area("a\ufeffb", 0, 0, 1, 1), area("c", 0, 1, 1, 1)}}},
		// An escape makes its code point, in the string as anywhere.
		{`"caf\e9  x"`, GridTemplate{Rows: 1, Columns: 2,
			Areas: []GridArea{area("café", 0, 0, 1, 1), area("x", 0, 1, 1, 1)}}},
		// CSS white space separates, including the tab, and the carriage
		// return and form feed an escape can put in a string. (Each escape
		// takes the one space after it.)
		{`"a` + "\t" + `b" "c\d  d" "e\c  f"`, GridTemplate{Rows: 3, Columns: 2}},
		// A name whose cells are a rectangle found out of reading order: its
		// second row begins a column before its first.
		{`". a a" "a a a"`, GridTemplate{}},
		{`"a a ." "a a ."`, GridTemplate{Rows: 2, Columns: 3,
			Areas: []GridArea{area("a", 0, 0, 2, 2)}}},
		{`". a" "a a"`, GridTemplate{}},
	} {
		vals, _ := css.ParseComponentValues(c.value)
		got, ok := ReadGridTemplateAreas(vals)
		switch {
		case c.want.Rows == 0:
			if ok {
				t.Errorf("%s read as %+v, want invalid", c.value, got)
			}
		case !ok:
			t.Errorf("%s is invalid, want %+v", c.value, c.want)
		case c.want.Areas == nil && got.Rows == c.want.Rows && got.Columns == c.want.Columns:
			// The white space case: only the shape is asked about.
		case !reflect.DeepEqual(got, c.want):
			t.Errorf("%s read as %+v, want %+v", c.value, got, c.want)
		}
	}
}

// TestAGridTemplateThatDoesNotDrawRectanglesIsInvalid is §7.3's three ways a
// declaration is invalid — a trash token, rows of different lengths, a name in
// cells that are not one rectangle — and a row that draws no cell.
func TestAGridTemplateThatDoesNotDrawRectanglesIsInvalid(t *testing.T) {
	for _, value := range []string{
		// Trash: neither an ident code point, a dot, nor CSS white space.
		`"a#b"`, `"a,b"`, `"!"`, `"a b" "c *"`,
		"\"a\u00a0b\"", "\"a\u2003b\"", "\"a\u3000b\"", "\"a\u0085b\"",
		// Rows of different lengths, counted in cells and not in words.
		`"a b" "c"`, `"a.b" "c d"`,
		// A name in two places, apart in a row, in a column, and diagonally.
		`"a b a"`, `"a" "b" "a"`, `"a b" "b a"`,
		// An L, which is connected and is not a rectangle.
		`"a a" "a b"`,
		// No cell at all.
		`""`, `"   "`, `"a" ""`,
		// Something that is not a string.
		`a b`, `"a" b`,
	} {
		vals, _ := css.ParseComponentValues(value)
		if got, ok := ReadGridTemplateAreas(vals); ok {
			t.Errorf("%s read as %+v, want invalid", value, got)
		}
		if ok, _ := JudgeValue("grid-template-areas", vals); ok {
			t.Errorf("the grammar takes grid-template-areas: %s", value)
		}
	}
	for _, value := range []string{`none`, `"a b" "c d"`, `"é ."`} {
		vals, _ := css.ParseComponentValues(value)
		if ok, _ := JudgeValue("grid-template-areas", vals); !ok {
			t.Errorf("the grammar refuses grid-template-areas: %s", value)
		}
	}
}
