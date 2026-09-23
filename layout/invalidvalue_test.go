package layout

import (
	"strings"
	"testing"
)

// A value that is not CSS never reaches layout now: the cascade's value grammar
// drops it (CSS 2.1 §4.2), says so as the author's mistake, and the box is laid
// out with whatever the declaration would have overridden. Several of this
// package's gates used a value that is not CSS to stand for one they cannot
// arrange; those cases are asserted here instead, as what they are.

// droppedAsInvalid reports whether the cascade dropped a declaration of property
// as invalid CSS.
func droppedAsInvalid(t *testing.T, htmlSrc, cssSrc, property string) bool {
	t.Helper()
	for _, f := range build(t, htmlSrc, cssSrc).Findings {
		if f.Rule == RuleInvalidCSS && f.Property == property &&
			strings.Contains(f.Message, "dropped") {
			return true
		}
	}
	return false
}

func TestAValueThatIsNotCSSIsDroppedByTheCascade(t *testing.T) {
	for _, tc := range []struct{ html, css, property string }{
		{threeItems, flexCSS + `#f { flex-direction: sideways }`, "flex-direction"},
		{threeItems, flexCSS + `#f { flex-wrap: reverse }`, "flex-wrap"},
		{fourItems, gridCSS + `#g { grid-template-columns: minmax(1fr, 2fr) }`, "grid-template-columns"},
		{fourItems, gridCSS + `#g { grid-template-columns: minmax(100px) }`, "grid-template-columns"},
		{fourItems, gridCSS + `#g { grid-template-columns: repeat(2, repeat(2, 1fr)) }`, "grid-template-columns"},
		{fourItems, gridCSS + `#g { grid-auto-flow: sideways }`, "grid-auto-flow"},
		{fourItems, gridCSS + `#g { align-items: left }`, "align-items"},
		{threeCells, gridCSS + threeColumns + `#a { grid-column: 1 / 2 / 3 }`, "grid-column"},
		{threeAreas, gridCSS + `#g { grid-template-areas: a b }`, "grid-template-areas"},
		{objectFitDoc, objectFitCSS + ` img { object-fit: fit }`, "object-fit"},
		{objectFitDoc, objectFitCSS + ` img { object-position: sideways }`, "object-position"},
		{`<p id="p">x</p>`, `#p { font-variant-caps: sideways-caps }`, "font-variant-caps"},
		{`<p id="p">x</p>`, `#p { text-fit: grow sideways }`, "text-fit"},
		{`<p id="p">x</p>`, `#p { text-fit: shrink upside-down }`, "text-fit"},
		{`<p id="p">x</p>`, `#p { text-fit: wobble }`, "text-fit"},
		{`<p id="p">x</p>`, `#p { text-justify: inter-ideograph }`, "text-justify"},
		{`<p id="p">x</p>`, `#p { text-indent: 2quips }`, "text-indent"},
		{`<p>x</p>`, `p::before { content: elephant }`, "content"},
	} {
		if !droppedAsInvalid(t, tc.html, tc.css, tc.property) {
			t.Errorf("%s was not dropped and reported as invalid CSS", tc.css)
		}
	}
}
