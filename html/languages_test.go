package html

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
)

// randomLanguageTree is a tree of elements and text with language attributes
// scattered through it: lang, xml:lang, both on one element, the empty value
// that says the language is unknown, and a document node that is sometimes XML
// and sometimes not, sometimes missing — a subtree built on its own has none —
// and sometimes a second one inside the tree, which XMLDocument answers by the
// nearer. The elements are sometimes the foreign roots and sometimes carry a
// prefix, which are the names the rule reads differently.
func randomLanguageTree(r *rand.Rand) (root *Node, all []*Node) {
	tags := []string{"", "tr", "en-GB", "de"}
	var grow func(parent *Node, depth int)
	add := func(parent, n *Node) {
		n.Parent = parent
		if parent != nil {
			parent.Children = append(parent.Children, n)
		}
		all = append(all, n)
	}
	grow = func(parent *Node, depth int) {
		for k := r.Intn(4); k > 0 && depth < 8; k-- {
			switch r.Intn(10) {
			case 0:
				add(parent, &Node{Type: TextNode, Text: "x"})
			case 1:
				doc := &Node{Type: DocumentNode, XML: r.Intn(2) == 0}
				add(parent, doc)
				grow(doc, depth+1)
			default:
				names := []string{"span", "span", "svg", "math", "o:p"}
				el := &Node{Type: ElementNode, Name: names[r.Intn(len(names))]}
				for f, n := 0, r.Intn(3); f < n; f++ {
					el.Attrs = append(el.Attrs, Attribute{Name: fmt.Sprintf("data-%d", f)})
				}
				if r.Intn(3) == 0 {
					el.Attrs = append(el.Attrs, Attribute{Name: "lang", Value: tags[r.Intn(len(tags))]})
				}
				if r.Intn(3) == 0 {
					el.Attrs = append(el.Attrs, Attribute{Name: "xml:lang", Value: tags[r.Intn(len(tags))]})
				}
				add(parent, el)
				grow(el, depth+1)
			}
		}
	}
	if r.Intn(4) == 0 {
		root = &Node{Type: ElementNode, Name: "div"}
	} else {
		root = &Node{Type: DocumentNode, XML: r.Intn(2) == 0}
	}
	add(nil, root)
	grow(root, 0)
	return root, all
}

// TestLanguagesAnswersWhatLanguageDoes holds the memo to the walk it replaces,
// asked about every node of random trees in a random order — so that some
// questions are answered from the memo, some fill it from below an answered
// node, and some from a node nothing has been asked about yet.
func TestLanguagesAnswersWhatLanguageDoes(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	xmlTagged := 0
	for i := 0; i < 5000; i++ {
		_, all := randomLanguageTree(r)
		var memo Languages
		r.Shuffle(len(all), func(a, b int) { all[a], all[b] = all[b], all[a] })
		for _, n := range all {
			wantTag, wantOK := n.Language()
			gotTag, gotOK := memo.Of(n)
			if gotTag != wantTag || gotOK != wantOK {
				t.Fatalf("tree %d: the memo says (%q, %v) where Language says (%q, %v)",
					i, gotTag, gotOK, wantTag, wantOK)
			}
			if wantOK && n.XMLDocument() {
				xmlTagged++
			}
		}
	}
	if xmlTagged == 0 {
		t.Fatal("no node was in an XML document, so xml:lang was never read")
	}
	var memo Languages
	if tag, ok := memo.Of(nil); tag != "" || ok {
		t.Errorf("a nil node has the language (%q, %v)", tag, ok)
	}
}

// TestLanguagesCostsOneWalkPerNode is the cost the memo is for.
//
// Every text node of a document asks its language several times over, and the
// walk to the root reads the attributes of every element on the way. So a tree
// of nested elements with a word in each cost the square of its depth times the
// attributes on each element. The memo stops each walk at the first node
// already answered, so asking every node of the tree costs one step per node.
func TestLanguagesCostsOneWalkPerNode(t *testing.T) {
	chain := func(depth int) []*Node {
		root := &Node{Type: DocumentNode}
		top := &Node{Type: ElementNode, Name: "html", Parent: root,
			Attrs: []Attribute{{Name: "lang", Value: "tr"}}}
		all := []*Node{root, top}
		parent := top
		for i := 0; i < depth; i++ {
			el := &Node{Type: ElementNode, Name: "span", Parent: parent}
			for a := 0; a < 30; a++ {
				el.Attrs = append(el.Attrs, Attribute{Name: fmt.Sprintf("a%05d", a)})
			}
			text := &Node{Type: TextNode, Text: "i", Parent: el}
			all = append(all, el, text)
			parent = el
		}
		return all
	}
	ask := func(all []*Node) func() {
		return func() {
			var memo Languages
			for _, n := range all {
				if tag, _ := memo.Of(n); n.Type == TextNode && tag != "tr" {
					panic("the language of the text is " + tag)
				}
			}
		}
	}
	small, large := chain(500), chain(2000)
	c := costtest.Time(t, "the language of every node of a chain", ask(small), ask(large))
	if c.Ratio > 8 {
		t.Errorf("the language of every node of a chain of 500 elements took %v and "+
			"of 2000 took %v, a factor of %.1f: linear is four and a walk to the "+
			"root per node is sixteen", c.Small, c.Large, c.Ratio)
	}
}

// TestTheLanguageIsReadInTheSectionsOrder is HTML §3.2.6.2's steps, one element
// at a time: "a lang attribute in the XML namespace" first, then "a lang in no
// namespace" on an HTML or an SVG element, then the parent.
//
// Which attribute is in the XML namespace is a question about the document and
// the element. In XHTML the prefix "xml" is bound to it by definition. In an
// HTML document the parser stores "xml:lang" as a name in no namespace, which
// the section gives "no effect on language processing" — except on <svg> and
// <math>, whose start tags the tree builder puts through "adjust foreign
// attributes", which moves it into the XML namespace. And lang in no namespace
// is not a MathML attribute, nor one of an element in a namespace this engine
// does not know, which in XHTML is one whose name kept its prefix.
func TestTheLanguageIsReadInTheSectionsOrder(t *testing.T) {
	xhtml := func(body string) string {
		return xhtmlDoctype + `<html xmlns="http://www.w3.org/1999/xhtml" ` +
			`xmlns:o="urn:schemas-microsoft-com:office:office" lang="en"><body>` +
			body + `</body></html>`
	}
	for _, tc := range []struct {
		what, src, element, want string
		xml                      bool
	}{
		{"xml:lang wins over lang on one element in XHTML",
			xhtml(`<p xml:lang="tr" lang="de">x</p>`), "p", "tr", true},
		{"xml:lang is no language on an HTML element in an HTML document",
			`<html lang="en"><body><p xml:lang="tr">x</p></body></html>`, "p", "en", false},
		{"nor does it beat the lang beside it there",
			`<html lang="en"><body><p xml:lang="tr" lang="de">x</p></body></html>`, "p", "de", false},
		{"xml:lang on an <svg> in an HTML document is in the XML namespace",
			`<html lang="en"><body><p><svg xml:lang="tr" lang="de"></svg></p></body></html>`, "svg", "tr", false},
		{"lang on an <svg> counts",
			`<html lang="en"><body><p><svg lang="de"></svg></p></body></html>`, "svg", "de", false},
		{"xml:lang on a <math> in an HTML document is in the XML namespace",
			`<html lang="en"><body><p><math xml:lang="tr"></math></p></body></html>`, "math", "tr", false},
		{"lang on a <math> is not MathML's, so the parent's language holds",
			`<html lang="en"><body><p><math lang="de"></math></p></body></html>`, "math", "en", false},
		{"and not in XHTML either",
			xhtml(`<p><math lang="de"></math></p>`), "math", "en", true},
		{"lang on a prefixed element of an unknown namespace in XHTML is not HTML's",
			xhtml(`<p><o:p lang="de">x</o:p></p>`), "o:p", "en", true},
		{"but in an HTML document <o:p> is an HTML element, and its lang counts",
			`<html lang="en"><body><p><o:p lang="de">x</o:p></p></body></html>`, "o:p", "de", false},
	} {
		doc, _, _ := Parse(tc.src)
		if doc.XML != tc.xml {
			t.Fatalf("%s: the fixture was read as XML=%v, want %v", tc.what, doc.XML, tc.xml)
		}
		el := findElement(doc, tc.element)
		if el == nil {
			t.Fatalf("%s: no <%s> in the tree", tc.what, tc.element)
		}
		if got, ok := el.Language(); !ok || got != tc.want {
			t.Errorf("%s: the language is %q (stated %v), want %q", tc.what, got, ok, tc.want)
		}
		var memo Languages
		if got, ok := memo.Of(el); !ok || got != tc.want {
			t.Errorf("%s: the memo says %q (stated %v), want %q", tc.what, got, ok, tc.want)
		}
	}
}
