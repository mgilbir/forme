package style

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
)

// GridTemplate is a grid-template-areas value read by CSS Grid 2 §7.3: how
// many rows and columns the picture draws, and the band of cells each name
// covers.
type GridTemplate struct {
	Rows, Columns int
	// Areas are the named areas, in the order their first cells are met
	// reading the rows from the top and each row from its start. A name is
	// here once, however many cells it covers.
	Areas []GridArea
}

// GridArea is one named area: the first row and column it covers, counted
// from nought, and how many of each.
type GridArea struct {
	Name          string
	Row, Column   int
	Rows, Columns int
}

// ReadGridTemplateAreas reads the strings of a grid-template-areas value, or
// reports that the declaration is invalid. "none" is the caller's; this is
// the <string>+ branch.
//
// §7.3 tokenizes each string on its own, with longest-match semantics, into:
//
//   - a sequence of ident code points, which is a named cell token whose name
//     is those code points;
//   - a sequence of one or more "." (U+002E), which is a null cell token;
//   - a sequence of white space, which is no token;
//   - a sequence of anything else, which is a trash token.
//
// and the declaration is invalid if a string holds a trash token, if the
// strings do not all make the same number of cells, or if a name's cells do
// not fill a single rectangle. A string that makes no cells at all is refused
// too: the grammar is a row per string and a column per cell, and a row of
// none is not a row of a grid (every browser rejects '""').
//
// An ident code point is CSS Syntax's and the tokenizer's, which is the point
// of asking css.IsIdentCodePoint rather than having a rule here: a name is
// matched against the <custom-ident> an item writes in grid-area, which the
// tokenizer read. So "é" and "区域" are names, "1st" is a name — which an item
// has to write as "\31st", because an identifier cannot begin with a digit —
// and "a.b" is three cells, a name, a null cell and a name, since "." is not an
// ident code point. A no-break space is neither white space nor an ident code
// point, so "a\u00a0b" holds a trash token and the declaration is invalid.
//
// White space is CSS's — space, tab, line feed, and the carriage return and
// form feed that preprocessing makes line feeds of — and not Unicode's. An
// escape can put a carriage return or a form feed into a string without
// preprocessing seeing it, and reading it as white space is what the
// browsers do.
//
// The work is one pass over the text and one over the cells: a name's
// rectangle is found as the bounds of its cells, and then every cell inside
// the bounds is checked once. A cell belongs to at most one name, so the
// checking stops at the first cell that does not carry the name it should,
// and visits no cell twice before it does.
func ReadGridTemplateAreas(vals []css.ComponentValue) (GridTemplate, bool) {
	var rows [][]string
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if !v.IsToken() || v.Token.Kind != css.String {
			return GridTemplate{}, false
		}
		cells, ok := gridAreaCells(v.Token.Value)
		if !ok || len(cells) == 0 || (len(rows) > 0 && len(cells) != len(rows[0])) {
			return GridTemplate{}, false
		}
		rows = append(rows, cells)
	}
	if len(rows) == 0 {
		return GridTemplate{}, false
	}

	out := GridTemplate{Rows: len(rows), Columns: len(rows[0])}
	index := map[string]int{}
	for r, cells := range rows {
		for c, name := range cells {
			if name == "" {
				continue
			}
			i, seen := index[name]
			if !seen {
				index[name] = len(out.Areas)
				out.Areas = append(out.Areas, GridArea{Name: name, Row: r, Column: c,
					Rows: 1, Columns: 1})
				continue
			}
			// The bounds of the name's cells so far. The rows are read in
			// order, so the first row is already the least; a column may be
			// before the first one met, in a later row.
			a := &out.Areas[i]
			if c < a.Column {
				a.Columns += a.Column - c
				a.Column = c
			}
			a.Columns = max(a.Columns, c-a.Column+1)
			a.Rows = max(a.Rows, r-a.Row+1)
		}
	}
	for _, a := range out.Areas {
		for r := a.Row; r < a.Row+a.Rows; r++ {
			for c := a.Column; c < a.Column+a.Columns; c++ {
				if rows[r][c] != a.Name {
					return GridTemplate{}, false
				}
			}
		}
	}
	return out, true
}

// gridAreaCells is §7.3's tokenization of one string: its cells in order, a
// null cell token as "", and false for a string that holds a trash token.
func gridAreaCells(s string) ([]string, bool) {
	var cells []string
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r < utf8.RuneSelf && ascii.IsCSSSpace(byte(r)):
			i += size
		case r == '.':
			for i < len(s) && s[i] == '.' {
				i++
			}
			cells = append(cells, "")
		case css.IsIdentCodePoint(r):
			start := i
			for i < len(s) {
				r, size := utf8.DecodeRuneInString(s[i:])
				if !css.IsIdentCodePoint(r) {
					break
				}
				i += size
			}
			cells = append(cells, s[start:i])
		default:
			return nil, false
		}
	}
	return cells, true
}

// gridTemplateAreas is the <string>+ branch of grid-template-areas' grammar.
func gridTemplateAreas(it []css.ComponentValue) verdict {
	if _, ok := ReadGridTemplateAreas(it); ok {
		return valid
	}
	return invalid
}
