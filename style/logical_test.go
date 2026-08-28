package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// CSS Logical Properties: a property that names a side by where the text starts
// rather than by where the page's left is.
//
// The engine lays out one writing mode, so the block axis is the vertical one
// and what is left to decide is "direction" — which is the element's own answer
// and is why the mapping cannot be done where the shorthands are expanded.

// logicalStyle applies one declaration and returns the element's computed style.
func logicalStyle(t *testing.T, decl string) (ComputedStyle, []Finding) {
	t.Helper()
	rules, errs := css.ParseStylesheet(`#t { ` + decl + ` }`)
	if len(errs) != 0 {
		t.Fatalf("%q: %v", decl, errs)
	}
	doc := parseDoc(t, `<div id="t">x</div>`)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	return got.Styles[elementFor(t, doc, "#t")], got.Findings
}

// sampleFor is a value the property will take: a border style is a keyword, a
// border colour is a colour, and everything else here is a length.
func sampleFor(name string) string {
	switch {
	case strings.HasSuffix(name, "-style"):
		return "dashed"
	case strings.HasSuffix(name, "-color"):
		return "red"
	}
	return "7px"
}

// TestEveryLogicalLonghandSetsItsPhysicalOne walks the table itself, so the test
// cannot drift from it: a longhand added to logical.go without an answer here is
// a longhand this checks anyway.
func TestEveryLogicalLonghandSetsItsPhysicalOne(t *testing.T) {
	for logical, sides := range logicalSides {
		value := sampleFor(logical)
		for i, dir := range []string{"ltr", "rtl"} {
			cs, findings := logicalStyle(t, "direction: "+dir+"; "+logical+": "+value)
			if got := cs[sides[i]]; got != value {
				t.Errorf("%s: %s in %s set %s to %q, want %q",
					logical, logical, dir, sides[i], got, value)
			}
			// And it set *only* that one: the other side of the same axis is
			// untouched, which is what makes the flip a flip rather than both.
			if other := sides[1-i]; other != sides[i] && cs[other] == value {
				t.Errorf("%s in %s also set %s", logical, dir, other)
			}
			for _, f := range findings {
				if f.Unsupported {
					t.Errorf("%s in %s was reported: %q", logical, dir, f.Message)
				}
			}
		}
	}
}

// TestALogicalAndAPhysicalDeclarationCompeteInOrder is why the rename happens
// before the winner is chosen rather than after.
//
// css-logical says the two set the same thing, so the later declaration wins —
// whichever of the two spellings it is. An engine that kept them apart and read
// one after the other would answer by which one layout happened to look at.
func TestALogicalAndAPhysicalDeclarationCompeteInOrder(t *testing.T) {
	cs, _ := logicalStyle(t, "margin-left: 1px; margin-inline-start: 2px")
	if cs["margin-left"] != "2px" {
		t.Errorf("the logical declaration written second lost: margin-left is %q",
			cs["margin-left"])
	}
	cs, _ = logicalStyle(t, "margin-inline-start: 2px; margin-left: 1px")
	if cs["margin-left"] != "1px" {
		t.Errorf("the physical declaration written second lost: margin-left is %q",
			cs["margin-left"])
	}
	// And importance beats order, as it does between any two declarations.
	cs, _ = logicalStyle(t, "margin-left: 1px !important; margin-inline-start: 2px")
	if cs["margin-left"] != "1px" {
		t.Errorf("an important physical declaration lost to a later logical one: %q",
			cs["margin-left"])
	}
}

// TestTheDirectionIsTheElementsOwn. The mapping is per element, so an element
// that sets its own direction is mapped by that and not by its parent's — and
// one that sets none inherits the question along with the answer.
func TestTheDirectionIsTheElementsOwn(t *testing.T) {
	rules, errs := css.ParseStylesheet(
		`#outer { direction: rtl } #t { margin-inline-start: 3px }` +
			`#own { direction: ltr; margin-inline-start: 4px }`)
	if len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
	doc := parseDoc(t, `<div id="outer"><p id="t">x</p><p id="own">y</p></div>`)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})

	inherited := got.Styles[elementFor(t, doc, "#t")]
	if inherited["margin-right"] != "3px" || inherited["margin-left"] == "3px" {
		t.Errorf("inside a right-to-left parent the start margin is the right one; "+
			"left=%q right=%q", inherited["margin-left"], inherited["margin-right"])
	}
	own := got.Styles[elementFor(t, doc, "#own")]
	if own["margin-left"] != "4px" || own["margin-right"] == "4px" {
		t.Errorf("an element that sets its own direction is mapped by it; "+
			"left=%q right=%q", own["margin-left"], own["margin-right"])
	}
}

// TestAStyleAttributeIsMappedToo, and the direction it is mapped by may itself
// come from the attribute.
func TestAStyleAttributeIsMappedToo(t *testing.T) {
	doc := parseDoc(t,
		`<div id="t" style="direction: rtl; padding-inline-start: 5px">x</div>`)
	got := Apply(doc, nil)
	cs := got.Styles[elementFor(t, doc, "#t")]
	if cs["padding-right"] != "5px" {
		t.Errorf("padding-right is %q, want 5px: the attribute set the direction too",
			cs["padding-right"])
	}
	// A style attribute saying it both ways takes the later of the two, which
	// is the rule for any two declarations in one block.
	doc = parseDoc(t, `<div id="t" style="margin-left: 1px; margin-inline-start: 2px">x</div>`)
	cs = Apply(doc, nil).Styles[elementFor(t, doc, "#t")]
	if cs["margin-left"] != "2px" {
		t.Errorf("margin-left is %q, want the later 2px", cs["margin-left"])
	}
	doc = parseDoc(t, `<div id="t" style="margin-inline-start: 2px; margin-left: 1px">x</div>`)
	cs = Apply(doc, nil).Styles[elementFor(t, doc, "#t")]
	if cs["margin-left"] != "1px" {
		t.Errorf("margin-left is %q, want the later 1px", cs["margin-left"])
	}
}

// TestALogicalShorthandSetsBothEnds. The two-value form is start and then end,
// which for the inline axis is left and then right in English.
func TestALogicalShorthandSetsBothEnds(t *testing.T) {
	for _, tc := range []struct{ decl, a, av, b, bv string }{
		{"margin-inline: 1px 2px", "margin-left", "1px", "margin-right", "2px"},
		{"margin-block: 1px 2px", "margin-top", "1px", "margin-bottom", "2px"},
		{"padding-inline: 3px", "padding-left", "3px", "padding-right", "3px"},
		{"inset-inline: 4px 5px", "left", "4px", "right", "5px"},
		{"inset-block: 4px 5px", "top", "4px", "bottom", "5px"},
		{"inset: 1px 2px 3px 4px", "top", "1px", "left", "4px"},
		{"border-inline-width: 1px 2px", "border-left-width", "1px", "border-right-width", "2px"},
		{"border-inline: 1px solid red", "border-left-style", "solid", "border-right-color", "red"},
		{"border-block-start: 2px dotted blue", "border-top-width", "2px", "border-top-color", "blue"},
	} {
		cs, findings := logicalStyle(t, tc.decl)
		if cs[tc.a] != tc.av || cs[tc.b] != tc.bv {
			t.Errorf("%q set %s=%q %s=%q, want %q and %q",
				tc.decl, tc.a, cs[tc.a], tc.b, cs[tc.b], tc.av, tc.bv)
		}
		for _, f := range findings {
			if f.Unsupported {
				t.Errorf("%q was reported: %q", tc.decl, f.Message)
			}
		}
	}
}

// TestAWideKeywordOnALogicalShorthandReachesThePhysicalProperty. "border: inherit"
// sets all twelve longhands, and the logical shorthands take the keyword by the
// same route — through the declared longhand list, which for these is a list of
// logical names that are then renamed.
func TestAWideKeywordOnALogicalShorthandReachesThePhysicalProperty(t *testing.T) {
	rules, errs := css.ParseStylesheet(
		`#outer { margin: 9px } #t { margin-inline: inherit }`)
	if len(errs) != 0 {
		t.Fatalf("%v", errs)
	}
	doc := parseDoc(t, `<div id="outer"><p id="t">x</p></div>`)
	cs := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}).
		Styles[elementFor(t, doc, "#t")]
	if cs["margin-left"] != "9px" || cs["margin-right"] != "9px" {
		t.Errorf("margin-inline: inherit gave left=%q right=%q, want 9px both",
			cs["margin-left"], cs["margin-right"])
	}
	if cs["margin-top"] == "9px" {
		t.Error("margin-inline: inherit reached the block axis as well")
	}
}

// TestALogicalNameIsNotAComputedProperty. It never survives the cascade: what is
// computed, inherited and read afterwards is the physical property it set, and
// an entry of its own would be a claim the engine does not keep.
func TestALogicalNameIsNotAComputedProperty(t *testing.T) {
	cs, _ := logicalStyle(t, "margin-inline-start: 7px")
	for name := range logicalSides {
		if _, ok := cs[name]; ok {
			t.Errorf("%q is in the computed style; it should have been renamed away", name)
		}
	}
	for name := range logicalSides {
		if _, ok := properties[name]; ok {
			t.Errorf("%q is in the property registry", name)
		}
	}
}
