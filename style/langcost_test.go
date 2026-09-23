package style

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/costtest"
)

// TestLangIsAnsweredOncePerElement is :lang() tried on every element of a deep
// document.
//
// It asked html.Node.Language, which walks to the root reading the attributes of
// every element on the way — so a rule with :lang() in it, tried on each element
// of a chain of nested ones, cost the square of the depth times the attributes
// on each. The matcher answers from an html.Languages now, which is one step per
// element for the whole document.
//
// The chain is built rather than parsed, because the parser stops nesting at
// 256 and the curve wants more room than that to show.
func TestLangIsAnsweredOncePerElement(t *testing.T) {
	vals, _ := css.ParseComponentValues(`:lang(tr)`)
	sels, _, ok := css.ParseSelectorList(vals)
	if !ok || len(sels) != 1 {
		t.Fatal("the selector was refused")
	}
	chain := func(depth int) (*html.Node, []*html.Node) {
		doc := &html.Node{Type: html.DocumentNode}
		top := &html.Node{Type: html.ElementNode, Name: "html", Parent: doc,
			Attrs: []html.Attribute{{Name: "lang", Value: "tr"}}}
		doc.Children = []*html.Node{top}
		all := []*html.Node{top}
		parent := top
		for i := 0; i < depth; i++ {
			el := &html.Node{Type: html.ElementNode, Name: "span", Parent: parent}
			for a := 0; a < 30; a++ {
				el.Attrs = append(el.Attrs, html.Attribute{Name: fmt.Sprintf("a%05d", a)})
			}
			parent.Children = []*html.Node{el}
			all = append(all, el)
			parent = el
		}
		return doc, all
	}
	match := func(depth int) func() {
		doc, all := chain(depth)
		return func() {
			m := NewMatcher(doc)
			for _, n := range all {
				if !m.Match(sels[0], n) {
					panic("an element of a Turkish document is not :lang(tr)")
				}
			}
		}
	}
	// Timed, not counted: the matcher's steps count what it compares, and the
	// walk this is about was a question the matcher asked of the tree, which
	// nothing counts. See costtest.Time.
	c := costtest.Time(t, ":lang(tr) on every element of a chain", match(500), match(2000))
	if c.Ratio > 8 {
		t.Errorf(":lang(tr) on every element of a chain of 500 took %v and of 2000 "+
			"took %v, a factor of %.1f: linear is four and a walk to the root per "+
			"element is sixteen", c.Small, c.Large, c.Ratio)
	}
}
