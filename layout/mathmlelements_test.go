package layout

import (
	"strings"
	"testing"
)

// A <style>, a <link> and a <base> inside MathML are MathML's elements that
// share a name with HTML's, and do none of what HTML's do: no stylesheet is
// read from them and no base URL is set by them.
func TestHTMLsDocumentElementsInsideMathMLAreNotHTMLs(t *testing.T) {
	b := Build(Input{HTML: `<math><mrow><style>p { color: red }</style>` +
		`<base href="elsewhere/"></mrow></math><p id=p>x</p><img src="a.png">`})
	for n, cs := range b.Styles {
		if n.Name == "p" && cs.Get("color") != "black" {
			t.Errorf("the <p> is %q: a MathML <style> was read as a stylesheet", cs.Get("color"))
		}
	}
	if base := documentBaseOf(b.Document); strings.Contains(base.href, "elsewhere") {
		t.Errorf("the document's base is %q: a MathML <base> set it", base.href)
	}
}
