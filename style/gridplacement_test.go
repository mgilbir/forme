package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// CSS Grid 2 §8.4's three placement shorthands, grid-row, grid-column and
// grid-area, expanded into their four longhands like every other shorthand.

// TestTheGridPlacementShorthandsExpand is §8.4's expansion, including the one
// rule each has about what it leaves out: an omitted line is the one it is
// copied from when that is a lone <custom-ident>, and "auto" otherwise.
func TestTheGridPlacementShorthandsExpand(t *testing.T) {
	longhands := []string{"grid-row-start", "grid-column-start", "grid-row-end",
		"grid-column-end"}
	for _, tc := range []struct {
		decl string
		want [4]string // row-start, column-start, row-end, column-end
	}{
		{"grid-column: 2", [4]string{"auto", "2", "auto", "auto"}},
		{"grid-column: 2 / 4", [4]string{"auto", "2", "auto", "4"}},
		{"grid-column: main", [4]string{"auto", "main", "auto", "main"}},
		{"grid-column: span 2 / 3", [4]string{"auto", "span 2", "auto", "3"}},
		{"grid-row: 1 / span 2", [4]string{"1", "auto", "span 2", "auto"}},
		{"grid-row: main", [4]string{"main", "auto", "main", "auto"}},
		// grid-area is the block axis first: row-start, column-start, row-end,
		// column-end.
		{"grid-area: main", [4]string{"main", "main", "main", "main"}},
		{"grid-area: a / b", [4]string{"a", "b", "a", "b"}},
		{"grid-area: 1 / 2", [4]string{"1", "2", "auto", "auto"}},
		{"grid-area: 1 / 2 / 3", [4]string{"1", "2", "3", "auto"}},
		{"grid-area: 1 / 2 / 3 / 4", [4]string{"1", "2", "3", "4"}},
		{"grid-area: span 2", [4]string{"span 2", "auto", "auto", "auto"}},
	} {
		got := computedUnder(t, tc.decl)
		for i, name := range longhands {
			if v := got.Get(name); v != tc.want[i] {
				t.Errorf("%s: %s is %q, want %q", tc.decl, name, v, tc.want[i])
			}
		}
	}
}

// TestTheGridPlacementShorthandsAreJudgedAsTheirLonghands: a shorthand is valid
// when each longhand it sets is, and has no more slashes than it has lines.
func TestTheGridPlacementShorthandsAreJudgedAsTheirLonghands(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		ok          bool
	}{
		{"grid-row", "1 / span 2", true},
		{"grid-column", "auto", true},
		{"grid-column", "main", true},
		{"grid-area", "a / b / c / d", true},
		{"grid-row", "span -1", false},
		{"grid-row", "0", false},
		{"grid-row", "1 / 2 / 3", false},
		{"grid-row", "1 /", false},
		{"grid-row", "/ 2", false},
		{"grid-area", "a / b / c / d / e", false},
	} {
		vals, errs := css.ParseComponentValues(tc.value)
		if len(errs) != 0 {
			t.Fatalf("%q did not tokenize", tc.value)
		}
		if got := supportsDeclaration(tc.name, vals); got != tc.ok {
			t.Errorf("%s: %s is supported %v, want %v", tc.name, tc.value, got, tc.ok)
		}
	}
}

// TestAGridPlacementShorthandMeetsItsLonghandsInTheCascade is audit C107. The
// three shorthands were registered as properties of their own and layout read
// them in front of their longhands, so a shorthand won whatever the cascade
// said. Expanded, the later or more specific declaration of each longhand
// wins, whichever spelling wrote it.
func TestAGridPlacementShorthandMeetsItsLonghandsInTheCascade(t *testing.T) {
	for _, tc := range []struct {
		sheet string
		name  string
		want  string
	}{
		// A more specific longhand beats a shorthand, and the shorthand's other
		// longhand stands.
		{`.x { grid-column: 1 / 2 } #a { grid-column-start: 3 }`, "grid-column-start", "3"},
		{`.x { grid-column: 1 / 2 } #a { grid-column-start: 3 }`, "grid-column-end", "2"},
		// And in the other order of the sheet, specificity still decides.
		{`#a { grid-column-start: 3 } .x { grid-column: 1 / 2 }`, "grid-column-start", "3"},
		// At equal specificity the later one wins, the shorthand included.
		{`.x { grid-column-start: 3 } .y { grid-column: 1 / 2 }`, "grid-column-start", "1"},
		{`.y { grid-column: 1 / 2 } .x { grid-column-start: 3 }`, "grid-column-start", "3"},
		// grid-area against grid-column: the more specific grid-column sets the
		// column lines and the rows stay the area's.
		{`.x { grid-area: p } #a { grid-column: 2 }`, "grid-column-start", "2"},
		{`.x { grid-area: p } #a { grid-column: 2 }`, "grid-column-end", "auto"},
		{`.x { grid-area: p } #a { grid-column: 2 }`, "grid-row-start", "p"},
	} {
		rules, errs := css.ParseStylesheet(tc.sheet)
		if len(errs) > 0 {
			t.Fatalf("%q does not parse: %v", tc.sheet, errs)
		}
		doc, _, _ := html.Parse(`<div id="a" class="x y">x</div>`)
		out := Apply(doc, []Sheet{{Rules: rules, Name: "a.css"}})
		var got ComputedStyle
		doc.Walk(func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Name == "div" {
				got = out.Styles[n]
			}
			return true
		})
		if v := got.Get(tc.name); v != tc.want {
			t.Errorf("%s: %s is %q, want %q", tc.sheet, tc.name, v, tc.want)
		}
	}
}

// TestNoShorthandIsRegisteredAsAProperty is the shape C107 was one instance
// of: a shorthand in the registry is a property the cascade cannot order
// against its own longhands, so it is decided by whichever one layout happens
// to read first. The list is every shorthand of the specifications whose
// longhands this engine has.
func TestNoShorthandIsRegisteredAsAProperty(t *testing.T) {
	for _, name := range []string{
		"margin", "padding", "border", "border-top", "border-right", "border-bottom",
		"border-left", "border-width", "border-style", "border-color", "outline",
		"background", "list-style", "font", "font-variant", "text-decoration",
		"white-space", "text-wrap", "text-align", "overflow", "flex", "flex-flow",
		"gap", "place-items", "place-content", "place-self", "columns",
		"grid-row", "grid-column", "grid-area", "grid-template", "grid",
		"inset", "margin-block", "margin-inline", "padding-block",
		"padding-inline", "border-block", "border-inline", "word-wrap",
	} {
		if _, ok := properties[name]; ok {
			t.Errorf("%q is a shorthand and is registered as a property", name)
		}
	}
}
