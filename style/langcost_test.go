package style

import (
	"fmt"
	"testing"
	"time"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// scalingOf measures one shape at n and at four times n, the way
// html/parsecost_test.go's scaling does: in windows of equal length, turn
// about, the least of nine rounds each, so that a busy machine slows both sides
// alike and the ratio is the curve's.
func scalingOf(small, large func()) (lo, hi time.Duration, ratio float64) {
	bestSmall, bestLarge := time.Duration(1<<62), time.Duration(1<<62)
	for r := 0; r < 9; r++ {
		start := time.Now()
		for i := 0; i < 4; i++ {
			small()
		}
		bestSmall = min(bestSmall, time.Since(start))
		start = time.Now()
		large()
		bestLarge = min(bestLarge, time.Since(start))
	}
	lo, hi = bestSmall/4, bestLarge
	if lo <= 0 {
		return lo, hi, 0
	}
	return lo, hi, float64(hi) / float64(lo)
}

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
	lo, hi, ratio := scalingOf(match(500), match(2000))
	if ratio > 8 {
		t.Errorf(":lang(tr) on every element of a chain of 500 took %v and of 2000 "+
			"took %v, a factor of %.1f: linear is four and a walk to the root per "+
			"element is sixteen", lo, hi, ratio)
	}
}
