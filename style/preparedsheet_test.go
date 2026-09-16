package style

import (
	"reflect"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// Preparing a stylesheet is parsing every selector and expanding every
// shorthand, and for the user agent's sheet it is the same work for every
// document. The memo's whole correctness is that reusing it is invisible.
//
// So the first test is differential: the same document styled twice, once
// through the memo and once past it, and every computed value of every element
// required to agree. A test of the memo's *internals* would pass on a memo that
// returned the right rules and the wrong findings, or the right findings at the
// wrong point in the cascade order.

const memoSheet = `
	p { color: red; margin: 1px 2px }
	div p, .c { padding-top: 3px }
	p::before { content: "x" }
	li { border: 1px solid blue }
	* { outline-width: 2px }
	p { color: green }
`

const memoDoc = `<div><p id="a" class="c">one</p></div><ul><li id="b">two</li></ul><p id="c">three</p>`

// styleEverything applies one user agent sheet and one author sheet to a
// document and answers every element's computed style, keyed by id.
func styleEverything(t *testing.T, ua Sheet, authorSrc string) map[string]ComputedStyle {
	t.Helper()
	doc := parseDoc(t, memoDoc)
	got := Apply(doc, []Sheet{ua, author(t, authorSrc)})
	out := map[string]ComputedStyle{}
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if id, ok := n.Attr("id"); ok {
			out[id] = got.Styles[n]
		}
		return true
	})
	return out
}

// copyRules is a sheet whose rules are the same rules in a different slice,
// which is a sheet the memo cannot recognise. It is how this test asks for the
// answer the engine gave before the memo existed.
func copyRules(s Sheet) Sheet {
	s.Rules = append([]css.Rule(nil), s.Rules...)
	return s
}

// TestTheMemoIsInvisible.
func TestTheMemoIsInvisible(t *testing.T) {
	ua := sheet(t, OriginUserAgent, memoSheet)
	// Twice through the memo, then once past it.
	first := styleEverything(t, ua, `#a { color: blue }`)
	second := styleEverything(t, ua, `#a { color: blue }`)
	fresh := styleEverything(t, copyRules(ua), `#a { color: blue }`)
	if len(first) != 3 {
		t.Fatalf("the fixture styled %d elements, want 3", len(first))
	}
	for id := range first {
		if !reflect.DeepEqual(first[id], second[id]) {
			t.Errorf("#%s differs between the first document and the second", id)
		}
		if !reflect.DeepEqual(first[id], fresh[id]) {
			t.Errorf("#%s differs between the memo and a sheet prepared afresh", id)
		}
	}
}

// TestTheMemoDoesNotSwallowTheFindings.
//
// Preparation is where an unsupported property is reported, and those belong to
// the document being styled. A memo that reported them once would put them on
// whichever document was styled first in the process and on none of the others,
// which is worse than not reporting them at all.
//
// The default sheet raises none today — a user agent rule is exempt from the
// unimplemented report — so this asks with a sheet that does: an unreadable
// selector is reported whatever the origin.
func TestTheMemoDoesNotSwallowTheFindings(t *testing.T) {
	ua := sheet(t, OriginUserAgent, `p:: { color: red } p { color: red }`)
	counts := []int{}
	for i := 0; i < 3; i++ {
		doc := parseDoc(t, memoDoc)
		counts = append(counts, len(Apply(doc, []Sheet{ua}).Findings))
	}
	if counts[0] == 0 {
		t.Fatalf("the fixture raised no finding at all; it does not reach the replay")
	}
	if counts[1] != counts[0] || counts[2] != counts[0] {
		t.Errorf("the findings were %v across three documents; the second and "+
			"third must match the first", counts)
	}
}

// TestOnlyTheDefaultSheetIsRemembered is the memory behaviour, and it is the
// reason the slot is a slot.
//
// Every author sheet is a fresh rule slice, so a memo that kept them would add
// an entry per document and never drop one — the leak the note on the hint
// cache in hints.go was written about. Keeping only the user agent's means the
// one sheet that is handed over unchanged is the one that is kept.
func TestOnlyTheDefaultSheetIsRemembered(t *testing.T) {
	prepared.Store(nil)
	doc := parseDoc(t, memoDoc)
	Apply(doc, []Sheet{author(t, memoSheet)})
	if got := prepared.Load(); got != nil {
		t.Error("an author sheet was remembered; every document brings a new one")
	}
	ua := sheet(t, OriginUserAgent, memoSheet)
	Apply(doc, []Sheet{ua})
	got := prepared.Load()
	if got == nil {
		t.Fatal("the user agent sheet was not remembered")
	}
	if got.key != &ua.Rules[0] {
		t.Error("the slot holds a sheet that is not the one handed over")
	}
	// And a second user agent sheet replaces the first rather than joining it.
	other := sheet(t, OriginUserAgent, `q { color: red }`)
	Apply(doc, []Sheet{other})
	if k := prepared.Load().key; k != &other.Rules[0] {
		t.Error("the slot did not take the second sheet; it is not one slot")
	}
}

// TestTheMemoIsRefusedAtADifferentPointInTheOrder.
//
// Every declaration carries the number the shared counter gave it and the
// cascade breaks its last tie with it, so rules prepared from a counter at zero
// are only valid at zero. A sheet put second is at a different point and has to
// be prepared again — which for the default sheet never happens, and is checked
// because "never happens" is what a memo keyed on the wrong thing also says.
func TestTheMemoIsRefusedAtADifferentPointInTheOrder(t *testing.T) {
	ua := sheet(t, OriginUserAgent, memoSheet)
	doc := parseDoc(t, memoDoc)
	Apply(doc, []Sheet{ua})
	done := prepared.Load()
	if done == nil {
		t.Fatal("the sheet was not remembered")
	}
	if _, ok := preparedBefore(ua, done.start); !ok {
		t.Fatal("the sheet was not answered from the memo at the point it was prepared")
	}
	if _, ok := preparedBefore(ua, done.start+1); ok {
		t.Error("the sheet was answered from the memo one place later in the " +
			"order; every declaration's order number would be wrong by one")
	}
}

// TestASheetThatDeclaresALayerIsNotRemembered.
//
// Preparing an @layer assigns a layer number out of the Styler's own counter,
// and preparing a nested one raises a note kept to one per document. Both are
// state the next document would have to move again, so a sheet whose
// preparation touched either is not remembered at all.
//
// The default sheet has no at-rule of any kind, so this never fires for it, and
// the refusal is precautionary rather than load-bearing: a plant that removes
// it fails this test and changes no page, because only a user agent sheet is
// ever remembered and the origin term of the cascade is compared *above* the
// layer term — so a layer number carried over from another document's counter
// can only collide with an author's layer, which origin decides first.
//
// It is kept because the reason it cannot be seen is a property of what is
// remembered, and the day that widens the guard is what stops the widening
// being silent. The test pins the refusal rather than a page, and says so.
func TestASheetThatDeclaresALayerIsNotRemembered(t *testing.T) {
	prepared.Store(nil)
	ua := sheet(t, OriginUserAgent, `@layer base { p { color: red } } p { color: blue }`)
	doc := parseDoc(t, memoDoc)
	first := Apply(doc, []Sheet{ua})
	if got := prepared.Load(); got != nil {
		t.Error("a sheet that declared a layer was remembered")
	}
	// And the second document is styled the same, which is what the refusal is
	// for: an unlayered declaration beats a layered one, and the layer numbers
	// the first document assigned are not the second document's to inherit.
	second := Apply(parseDoc(t, memoDoc), []Sheet{ua})
	a := doc.Element("p")
	if a == nil {
		t.Fatal("no paragraph")
	}
	if got := first.Styles[a]["color"]; got != "blue" {
		t.Errorf("the first document's colour is %q, want blue: an unlayered "+
			"declaration beats a layered one", got)
	}
	for n, cs := range second.Styles {
		if n.Type == html.ElementNode && n.Name == "p" && cs["color"] != "blue" {
			t.Errorf("the second document's colour is %q, want blue", cs["color"])
		}
	}
}
