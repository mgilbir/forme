package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// nestedIs is k levels of :is() around ".nowhere .x", each level adding a
// descendant ".x": ":is(:is(.nowhere .x) .x) .x" for k = 2. It matches nothing,
// and learns so only at the root of the tree.
func nestedIs(k int) string {
	sel := ".nowhere .x"
	for i := 1; i < k; i++ {
		sel = ":is(" + sel + ") .x"
	}
	return sel
}

// TestNestedIsIsLinearInItsNesting is the cost of an argument inside an
// argument, counted in the matcher's own steps, which is the work each rule is
// charged for against its budget for the document.
//
// Each level of :is() asked the level inside it about every ancestor of every
// element it was asked about, afresh, so the levels multiplied: four levels
// against forty nested elements spent the whole per-match budget on every one
// of them. Each argument list's answer is remembered per element now, so a level
// costs a walk of the ancestors of each element once, and the levels add.
func TestNestedIsIsLinearInItsNesting(t *testing.T) {
	doc, _, _ := html.Parse(strings.Repeat(`<div class="x">`, 40) + "<p class=x>p</p>")
	var all []*html.Node
	doc.Walk(func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			all = append(all, n)
		}
		return true
	})
	work := func(k int) (steps int) {
		vals, _ := css.ParseComponentValues(nestedIs(k))
		sels, errs, ok := css.ParseSelectorList(vals)
		if !ok || len(sels) != 1 {
			t.Fatalf("%q was refused: %v", nestedIs(k), errs)
		}
		m := NewMatcher(doc)
		for _, n := range all {
			if m.Match(sels[0], n) {
				t.Fatalf("%q matched an element; it matches nothing", nestedIs(k))
			}
		}
		if m.Tripped() {
			t.Errorf("%d levels of :is() ran out of the per-match budget", k)
		}
		steps, _ = m.takeWork()
		return steps
	}
	small, large := work(4), work(16)
	if r := float64(large) / float64(small); r > 8 {
		t.Errorf("four levels of :is() took %d steps and sixteen took %d, a factor of "+
			"%.1f: the levels add, which is four, and multiplied they are far past the "+
			"budget", small, large, r)
	}
}

// TestARememberedAnswerIsStillCharged: the memo saves the walk and not the
// accounting. Each rule's matching is charged against its budget for the
// document, and a lookup is a step of it, so a selector that asks the same
// question on every element still pays for asking.
func TestARememberedAnswerIsStillCharged(t *testing.T) {
	doc, _, _ := html.Parse(strings.Repeat(`<div class="x">`, 10) + "<p class=x>p</p>")
	vals, _ := css.ParseComponentValues(":is(.nowhere .x)")
	sels, _, _ := css.ParseSelectorList(vals)
	p := doc.Element("p")
	m := NewMatcher(doc)
	m.Match(sels[0], p)
	if first, _ := m.takeWork(); first < 10 {
		t.Fatalf("the first match took %d steps; it walks ten ancestors", first)
	}
	m.Match(sels[0], p)
	// One for the compound, one for the answer it remembered.
	if again, _ := m.takeWork(); again != 2 {
		t.Errorf("the same question again took %d steps, want 2: one for the "+
			"compound and one for the remembered answer", again)
	}
}
