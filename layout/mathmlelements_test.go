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

// TestAMathMLInputOrLinkIsNeither: inside a formula, <input> and <a> are
// MathML elements that share HTML's names — an unknown one, laid out as a
// row, and one MathML Core gives no link — and neither is a form control or
// a hyperlink.
func TestAMathMLInputOrLinkIsNeither(t *testing.T) {
	src := `<math><input id="i" value="typed"/><a href="https://example.com/"><mn>1</mn></a></math>`
	b := Build(Input{HTML: src})
	frag := Layout(b.Root, A4.Content(), b.Fonts, nil)
	if w := find(t, frag, "i").BorderRect.W; w != 0 {
		t.Errorf("a MathML <input> is %d wide; an empty row is nothing, and a field is twenty characters", w)
	}
	out := Compose(Input{HTML: src}, Options{})
	for _, op := range out.Ops {
		switch v := op.(type) {
		case Link:
			t.Errorf("a MathML <a> is a link: %+v", v)
		case DrawText:
			if v.Text == "typed" {
				t.Errorf("a MathML <input> drew a field's value")
			}
		}
	}
}

// TestAMathMLVideoIsNoVideo: a <video> or an <iframe> inside a formula is a
// MathML element sharing the name, and nothing loads or reports a picture for
// it.
func TestAMathMLVideoIsNoVideo(t *testing.T) {
	control := Build(Input{HTML: `<video poster="p.png"></video><iframe src="f.html"></iframe>`})
	if len(control.Findings) == 0 {
		t.Fatal("an HTML video and iframe reported nothing; the test would test nothing")
	}
	b := Build(Input{HTML: `<math><video poster="p.png"></video><iframe src="f.html"></iframe></math>`})
	for _, f := range b.Findings {
		if strings.Contains(f.Message, "video") || strings.Contains(f.Message, "iframe") {
			t.Errorf("a MathML element was loaded as a picture: %v", f)
		}
	}
}
