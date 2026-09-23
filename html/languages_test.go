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
// nearer.
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
				el := &Node{Type: ElementNode, Name: "span"}
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
