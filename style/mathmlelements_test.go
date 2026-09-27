package style

import (
	"testing"

	"github.com/mgilbir/forme/html"
)

// An element inside MathML that shares its name with an HTML element is not
// that HTML element: none of HTML's presentational hints apply to it, and an
// <a> there is not a link. Before MathML was parsed as a tree there were no
// such elements to confuse; now a <td> inside a <math> is a MathML element
// called td.
func TestHTMLsElementRulesDoNotReachMathML(t *testing.T) {
	doc, _, _ := html.Parse(`<table><tr><td id="h" bgcolor="red">x</td></tr></table>` +
		`<math><td id="m" bgcolor="red">x</td><a id="ma" href="u">y</a></math><a id="ha" href="u">z</a>`)
	sheets := []Sheet{author(t, `:link { color: green }`)}
	if got := styleOf(t, doc, sheets, "#h", "background-color"); got == "transparent" || got == "" {
		t.Fatalf("the HTML <td bgcolor> is %q; the fixture is not testing anything", got)
	}
	if got := styleOf(t, doc, sheets, "#m", "background-color"); got != "transparent" {
		t.Errorf("a MathML <td bgcolor> has background-color %q; HTML's hint is HTML's", got)
	}
	if got := styleOf(t, doc, sheets, "#ha", "color"); got != "green" {
		t.Fatalf("the HTML <a href> is %q; the fixture is not testing anything", got)
	}
	if got := styleOf(t, doc, sheets, "#ma", "color"); got == "green" {
		t.Errorf("a MathML <a href> matched :link")
	}
}
