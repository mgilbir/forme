package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A grid area is named by ident code points, and nothing narrower.
//
// CSS Grid 2 §7.3 tokenizes each string of grid-template-areas, with
// longest-match semantics, into named cell tokens — "a sequence of ident code
// points" — null cell tokens, which are runs of ".", white space, and trash;
// and an item reaches a named area by a <custom-ident>, which the tokenizer
// reads. This engine read both with a rule of its own, ASCII letters, digits,
// "-" and "_", not starting with a digit, and compared the item's name as the
// cascade had stored it, escapes and all. So an area named "é" was refused with
// a finding, "1st" could not be named at all, and "a.b" was one word nobody
// could use where §7.3 makes it three cells.

// arrangedWithoutFinding fails the test if composing the document reported
// anything about a grid container, which is what a refused grid says.
func arrangedWithoutFinding(t *testing.T, htmlSrc, cssSrc string) {
	t.Helper()
	got := Compose(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: gridCSS + cssSrc}}},
		Options{})
	for _, f := range got.Findings {
		if strings.Contains(f.Message, "grid container") || f.Property == "grid-template-areas" {
			t.Errorf("%s: %q", cssSrc, f.Message)
		}
	}
}

// TestAGridAreaIsNamedByIdentCodePoints is §7.3's tokenization, and §8.3's
// <custom-ident> on the item's side of it.
func TestAGridAreaIsNamedByIdentCodePoints(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      [][4]float64
	}{
		{
			// Names outside ASCII, which CSS Syntax's ident code points include.
			"names outside ASCII",
			`#g { width: 300px; grid-template-areas: "é 区域" "x x" }` +
				`#h { grid-area: é } #n { grid-area: 区域 } #m { grid-area: x }`,
			[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}, {0, 20, 300, 20}},
		},
		{
			// §7.3's own note: "1st 2nd 3rd" are cell names, which an item
			// writes escaped because an identifier cannot begin with a digit.
			// The cascade keeps "\31st" as "\31 st", and the name is what the
			// escape makes, not the text.
			"names beginning with a digit",
			`#g { width: 300px; grid-template-areas: "1st 2nd" }` +
				`#h { grid-area: \31st } #n { grid-area: \32 nd }`,
			[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}, {0, 20, 150, 20}},
		},
		{
			// An escape inside the string is the tokenizer's too: the cell is
			// named "café" however the stylesheet spelled it. (The escape takes
			// the one space after it, so two are written.)
			"an escape in the template",
			`#g { width: 300px; grid-template-areas: "caf\e9  x" }` +
				`#h { grid-area: café } #n { grid-area: x }`,
			[][4]float64{{0, 0, 150, 20}, {150, 0, 150, 20}, {0, 20, 150, 20}},
		},
		{
			// "." is not an ident code point, so longest-match ends the name
			// there: "a.b" is a named cell, a null cell and a named cell. The
			// automatic item goes to the null one, which is free.
			"a dot between two names",
			`#g { width: 300px; grid-template-areas: "a.b" }` +
				`#h { grid-area: a } #n { grid-area: b }`,
			[][4]float64{{0, 0, 100, 20}, {200, 0, 100, 20}, {100, 0, 100, 20}},
		},
		{
			// And a run of dots before a name is one null cell.
			"dots before a name",
			`#g { width: 300px; grid-template-areas: "..a" }` +
				`#h { grid-area: a }`,
			[][4]float64{{150, 0, 150, 20}, {0, 0, 150, 20}, {0, 20, 150, 20}},
		},
		{
			// §7.3.2's implicit line names, reached by the same names.
			"the lines an area names",
			`#g { width: 300px; grid-template-areas: "é 区域" }` +
				`#h { grid-row: é; grid-column: 区域-start / 区域-end }` +
				`#n { grid-column: é }`,
			[][4]float64{{150, 0, 150, 20}, {0, 0, 150, 20}, {0, 20, 150, 20}},
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			arrangedWithoutFinding(t, threeAreas, c.css)
			wantCells(t, gridCells(t, threeAreas, c.css), c.want, c.what)
		})
	}
}

// TestACharacterThatIsNotAnIdentCodePointIsTrash. A no-break space and an
// ideographic space are neither CSS white space nor ident code points, and
// neither is "!": each is a trash token, and §7.3 makes the declaration
// invalid, so the cascade drops it and says so.
func TestACharacterThatIsNotAnIdentCodePointIsTrash(t *testing.T) {
	for _, row := range []string{"a\u00a0b", "a\u3000b", "a!", "!"} {
		css := `#g { width: 300px; grid-template-areas: "` + row + `" }`
		if !droppedAsInvalid(t, threeAreas, gridCSS+css, "grid-template-areas") {
			t.Errorf("%q was not dropped and reported as invalid CSS", row)
		}
	}
}

// TestASpanIsWrittenEitherSideOfItsCount. §8.3's "span && <integer>": the two
// in either order. The count after the keyword was read and the count before it
// refused the grid.
func TestASpanIsWrittenEitherSideOfItsCount(t *testing.T) {
	for _, css := range []string{`#a { grid-column: span 2 }`, `#a { grid-column: 2 span }`} {
		arrangedWithoutFinding(t, threeCells, threeColumns+css)
		wantCells(t, gridCells(t, threeCells, threeColumns+css),
			[][4]float64{{0, 0, 200, 20}, {200, 0, 100, 20}, {0, 20, 100, 20}}, css)
	}
}

// TestATemplateTheCascadeDidNotJudgeIsStillRefused. The cascade drops a
// template that is not CSS, but a value can reach layout without it — a
// caller's computed style, the engine's own — and layout reads the template by
// the same rules rather than trusting it was judged. Each of these is refused,
// and the one valid template is read.
func TestATemplateTheCascadeDidNotJudgeIsStillRefused(t *testing.T) {
	var l layouter
	for _, value := range []string{`"a b" "c"`, `"a b a"`, `"a#b"`, `""`, "\"a\u00a0b\""} {
		b := &Box{Outer: OuterBlock, Style: style.Initial().With("grid-template-areas", value)}
		if got, ok := l.areasOf(b); ok {
			t.Errorf("%s was read as %+v, want refused", value, got)
		}
	}
	b := &Box{Outer: OuterBlock, Style: style.Initial().With("grid-template-areas", `"é é" "x ."`)}
	got, ok := l.areasOf(b)
	if !ok || got.rows != 2 || got.columns != 2 || got.at["é"][1].span != 2 || got.at["x"][0].start != 1 {
		t.Errorf(`"é é" "x ." was read as %+v, %v`, got, ok)
	}
}
