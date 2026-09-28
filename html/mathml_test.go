package html

import (
	"strings"
	"testing"
)

// outline writes a subtree as names and text: "p[#a m:math[m:mi[#x]]]", a
// MathML element prefixed "m:", an SVG one "s:", text as "#text".
func outline(n *Node) string {
	var b strings.Builder
	var walk func(n *Node)
	walk = func(n *Node) {
		switch n.Type {
		case TextNode:
			b.WriteString("#" + n.Text)
			return
		case ElementNode:
			switch n.Namespace {
			case NamespaceMathML:
				b.WriteString("m:")
			case NamespaceSVG:
				b.WriteString("s:")
			case NamespaceOther:
				b.WriteString("?:")
			}
			b.WriteString(n.Name)
		}
		if len(n.Children) == 0 {
			return
		}
		b.WriteString("[")
		for i, c := range n.Children {
			if i > 0 {
				b.WriteString(" ")
			}
			walk(c)
		}
		b.WriteString("]")
	}
	walk(n)
	return b.String()
}

func bodyOf(t *testing.T, doc *Node) *Node {
	t.Helper()
	for _, c := range doc.Children[0].Children {
		if c.Name == "body" {
			return c
		}
	}
	t.Fatal("no body")
	return nil
}

// TestMathMLIsParsedAsATree is HTML's rules for foreign content, case by case,
// against the tree a browser builds for the same markup.
func TestMathMLIsParsedAsATree(t *testing.T) {
	for _, tc := range []struct {
		what, src, want string
		errs            []string
	}{
		{"a formula in a paragraph",
			`<p>a<math><mi>x</mi><mo>+</mo><mn>1</mn></math>b</p>`,
			`body[p[#a m:math[m:mi[#x] m:mo[#+] m:mn[#1]] #b]]`, nil},
		{"any start tag is MathML inside MathML, whatever its name",
			`<math><mrow><form><mfrac></mfrac></form></mrow></math>`,
			`body[m:math[m:mrow[m:form[m:mfrac]]]]`, nil},
		{"a token element holds HTML",
			`<math><mtext><b>x</b> y</mtext></math>`,
			`body[m:math[m:mtext[b[#x] # y]]]`, nil},
		{"except mglyph and malignmark, which are MathML there too",
			`<math><mi><mglyph></mglyph><malignmark></malignmark></mi></math>`,
			`body[m:math[m:mi[m:mglyph m:malignmark]]]`, nil},
		{"a break-out tag ends the MathML and is read as HTML after it",
			`<math><mi>x</mi><div>y</div></math>z`,
			`body[m:math[m:mi[#x]] div[#y] #z]`,
			[]string{"<div> cannot be inside MathML", "</math> closes nothing"}},
		{"<font> breaks out only with a presentational attribute",
			`<math><font>a</font><font color=red>b</font></math>`,
			`body[m:math[m:font[#a]] font[#b]]`,
			[]string{"<font> cannot be inside MathML"}},
		{"a paragraph in an mtext does not close the one the formula is in",
			`<p>a<math><mtext><p>b</p></mtext></math>c</p>`,
			`body[p[#a m:math[m:mtext[p[#b]]] #c]]`, nil},
		{"an end tag closes the MathML element of its name, and what is open inside it",
			`<math><mrow><mi>x</mrow><mn>2</mn></math>`,
			`body[m:math[m:mrow[m:mi[#x]] m:mn[#2]]]`,
			[]string{"</mrow> is written inside <mi>"}},
		{"</p> inside MathML breaks out and closes the paragraph",
			`<p>a<math><mi>x</mi></p>b`,
			`body[p[#a m:math[m:mi[#x]]] #b]`,
			[]string{"</p> cannot be inside MathML"}},
		{"an end tag for an HTML element is read by HTML's rules past the MathML",
			`<div><math><mrow><mi>x</mi></div>y`,
			`body[div[m:math[m:mrow[m:mi[#x]]]] #y]`,
			[]string{"</div> is written inside <mrow>", "</div> closes <div>, and <mrow> inside it is still open"}},
		{"an annotation-xml holding HTML",
			`<math><semantics><mi>x</mi><annotation-xml encoding="TEXT/HTML"><div>h</div></annotation-xml></semantics></math>`,
			`body[m:math[m:semantics[m:mi[#x] m:annotation-xml[div[#h]]]]]`, nil},
		{"an annotation-xml of another encoding holds MathML, and a <div> breaks out of it",
			`<math><annotation-xml encoding="application/mathml+xml"><apply></apply><div>h</div></annotation-xml></math>`,
			`body[m:math[m:annotation-xml[m:apply]] div[#h]]`,
			[]string{"<div> cannot be inside MathML", "</annotation-xml> closes nothing"}},
		{"an svg in an annotation-xml is SVG",
			`<math><annotation-xml><svg><rect/></svg></annotation-xml></math>`,
			`body[m:math[m:annotation-xml[s:svg]]]`, nil},
		{"a CDATA section in MathML is text",
			`<math><mi><![CDATA[x<y]]></mi></math>`,
			`body[m:math[m:mi[#x<y]]]`, nil},
		{"a self-closing MathML element is empty",
			`<math><mspace width="1em"/><mn>2</mn></math>`,
			`body[m:math[m:mspace m:mn[#2]]]`, nil},
		{"a <style> in MathML is an element, not a stylesheet",
			`<math><mrow><style><mi>x</mi></style></mrow></math>`,
			`body[m:math[m:mrow[m:style[m:mi[#x]]]]]`, nil},
		{"a math in a table is foster-parented, and its content goes into it",
			`<table><math><mi>x</mi></math><tr><td>c</td></tr></table>`,
			`body[m:math[m:mi[#x]] table[tr[td[#c]]]]`,
			[]string{"<math> is not table content"}},
		{"outside MathML an <mi> is an HTML element, and no scope boundary",
			`<p>a<mi>b<p>c`,
			`body[p[#a mi[#b]] p[#c]]`,
			[]string{"<p> closes <p>, and <mi> inside it is still open"}},
		{"a math in a token element is a formula of its own",
			`<math><mtext><math><mn>1</mn></math></mtext></math>`,
			`body[m:math[m:mtext[m:math[m:mn[#1]]]]]`, nil},
	} {
		doc, errs, _ := Parse(tc.src)
		got := outline(bodyOf(t, doc))
		if got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.what, got, tc.want)
		}
		var msgs []string
		for _, e := range errs {
			msgs = append(msgs, e.Message)
		}
		all := strings.Join(msgs, "\n")
		for _, want := range tc.errs {
			if !strings.Contains(all, want) {
				t.Errorf("%s: findings %q do not say %q", tc.what, msgs, want)
			}
		}
		if tc.errs == nil && len(msgs) > 0 {
			t.Errorf("%s: unexpected findings %q", tc.what, msgs)
		}
	}
}

// TestMathMLAttributesAreAdjusted: definitionURL gets its case back on every
// MathML element, as §13.2.6.1's table gives it, not only on the root.
func TestMathMLAttributesAreAdjusted(t *testing.T) {
	doc, _, _ := Parse(`<math><mi DEFINITIONURL="u" MathVariant="normal">x</mi></math>`)
	mi := bodyOf(t, doc).Children[0].Children[0]
	if _, ok := mi.AttrExact("definitionURL"); !ok {
		t.Errorf("attributes %v; want definitionURL", mi.Attrs)
	}
	if _, ok := mi.AttrExact("mathvariant"); !ok {
		t.Errorf("attributes %v; an HTML document folds the rest to lower case", mi.Attrs)
	}
}

// TestMathMLInXHTMLIsTheTreeItsTagsWrite: XML has no break-outs; an element
// inside MathML is MathML unless its xmlns or its prefix says otherwise.
func TestMathMLInXHTMLIsTheTreeItsTagsWrite(t *testing.T) {
	src := `<html xmlns="http://www.w3.org/1999/xhtml" xmlns:m="http://www.w3.org/1998/Math/MathML"` +
		` xmlns:h="http://www.w3.org/1999/xhtml" xmlns:x="urn:x"><body>` +
		`<math xmlns="http://www.w3.org/1998/Math/MathML"><mi>x</mi><div>d</div>` +
		`<mtext><b xmlns="http://www.w3.org/1999/xhtml">y<i>z</i></b><h:em>w</h:em></mtext>` +
		`<x:foo><mn>2</mn></x:foo></math>` +
		`<m:math><m:mn>1</m:mn></m:math><math><mi>q</mi></math></body></html>`
	doc, _, _ := ParseXHTML(src)
	got := outline(bodyOf(t, doc))
	want := `body[m:math[m:mi[#x] m:div[#d] m:mtext[b[#y i[#z]] em[#w]] ?:x:foo[m:mn[#2]]] m:math[m:mn[#1]] m:math[m:mi[#q]]]`
	if got != want {
		t.Errorf("\n got %s\nwant %s", got, want)
	}
}

// TestXMLLangOnAMathMLElement: "adjust foreign attributes" puts an xml:lang on
// any MathML element in the XML namespace, and a lang on one is not MathML's.
func TestXMLLangOnAMathMLElement(t *testing.T) {
	doc, _, _ := Parse(`<p lang="en"><math xml:lang="tr"><mi xml:lang="de">x</mi><mn lang="fr">1</mn></math></p>`)
	math := bodyOf(t, doc).Children[0].Children[0]
	for _, tc := range []struct {
		n    *Node
		want string
	}{{math.Children[0], "de"}, {math.Children[1], "tr"}} {
		if got, _ := tc.n.Language(); got != tc.want {
			t.Errorf("<%s> is in %q, want %q", tc.n.Name, got, tc.want)
		}
	}
}

// TestNestedMathMLIsBounded: MathML nests as deep as markup does, and the
// depth bound stops it as it stops any element.
func TestNestedMathMLIsBounded(t *testing.T) {
	src := "<math>" + strings.Repeat("<mrow>", 5000) + "x"
	doc, errs, _ := Parse(src)
	if doc == nil {
		t.Fatal("no tree")
	}
	found := false
	for _, e := range errs {
		if e.Limit && strings.Contains(e.Message, "nested more deeply") {
			found = true
		}
	}
	if !found {
		t.Errorf("five thousand nested <mrow>s were read without the depth bound saying so")
	}
	depth := 0
	for n := bodyOf(t, doc); len(n.Children) > 0; n = n.Children[0] {
		depth++
	}
	if depth > maxDepth+2 {
		t.Errorf("the tree is %d deep, past the bound of %d", depth, maxDepth)
	}
}
