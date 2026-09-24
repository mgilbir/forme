package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// Syntax white space is the specification's and not Unicode's: see
// internal/ascii/space.go and cmd/whitespace_test.go. The characters are
// Unicode's white space that CSS and HTML do not count as any, and the byte
// order mark, which is neither's.
var notSyntaxSpace = []struct{ ch, name string }{
	{"\u00a0", "a no-break space"},
	{"\u2003", "an em space"},
	{"\u3000", "an ideographic space"},
	{"\ufeff", "a byte order mark"},
}

// TestAMediaFeatureIsReadWithCSSWhiteSpace. A media feature's name and its
// keyword value are identifiers, and a no-break space is a name code point:
// "(orientation: portrait\u00a0)" asks about a value no page has, and Media
// Queries 4 makes a query it cannot answer false. Trimmed by Unicode's white
// space, it was portrait, and the rules inside applied to a portrait page.
func TestAMediaFeatureIsReadWithCSSWhiteSpace(t *testing.T) {
	portrait := Media{Width: mustUnit(794), Height: mustUnit(1123)}
	matches := func(query string) (bool, string) {
		vals, errs := css.ParseComponentValues(query)
		if len(errs) > 0 {
			t.Fatalf("%q: %v", query, errs)
		}
		return MatchesMedia(vals, portrait)
	}
	for _, q := range []string{"(orientation: portrait)", "( orientation :\tportrait\n)", "(ORIENTATION: Portrait)", "(width)"} {
		if ok, unknown := matches(q); !ok || unknown != "" {
			t.Errorf("%q on a portrait page = %v (unknown %q), want true", q, ok, unknown)
		}
	}
	for _, s := range notSyntaxSpace {
		for _, q := range []string{
			"(orientation: portrait" + s.ch + ")",
			"(orientation:" + s.ch + "portrait)",
			"(width" + s.ch + ")",
			"(" + s.ch + "orientation: portrait)",
		} {
			if ok, _ := matches(q); ok {
				t.Errorf("%q matched: %s is part of the identifier it touches", q, s.name)
			}
		}
	}
}

// TestAnHTMLAttributeIsTrimmedOfASCIIWhiteSpaceOnly. The valign attribute is
// read after ASCII white space round it is taken off (see the note on
// TestValignMapsToVerticalAlign), and a no-break space is not ASCII white space:
// "\u00a0top" is no keyword and the cell keeps its default.
func TestAnHTMLAttributeIsTrimmedOfASCIIWhiteSpaceOnly(t *testing.T) {
	base := computed(t, `<table><tr><td id="c">x</td></tr></table>`)["c"].Get("vertical-align")
	for _, s := range notSyntaxSpace {
		for _, attr := range []string{s.ch + "top", "top" + s.ch} {
			got := computed(t, `<table><tr><td id="c" valign="`+attr+`">x</td></tr></table>`)["c"].Get("vertical-align")
			if got != base {
				t.Errorf("valign=%q computed vertical-align %q, want the default %q: %s is not white space to HTML",
					attr, got, base, s.name)
			}
		}
	}
}

// TestADimensionAttributeSkipsASCIIWhiteSpaceOnly. HTML's rules for a
// dimension skip ASCII white space before the digits and nothing else, so
// width="\u00a0100" has no number in it and maps to nothing.
func TestADimensionAttributeSkipsASCIIWhiteSpaceOnly(t *testing.T) {
	for _, v := range []string{"100", " 100", "\t\n\f\r100", "100px"} {
		if got := computed(t, `<img id="i" width="`+v+`">`)["i"].Get("width"); got != "100px" {
			t.Errorf("width=%q computed width %q, want 100px", v, got)
		}
	}
	for _, s := range notSyntaxSpace {
		if got := computed(t, `<img id="i" width="`+s.ch+`100">`)["i"].Get("width"); got == "100px" {
			t.Errorf("width of %s then 100 computed width %q: %s is not white space to HTML", s.name, got, s.name)
		}
	}
}
