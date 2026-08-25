package style

import (
	"testing"

	"github.com/mgilbir/forme/css"
)

// "font-size: 6ex", and whose x-height the six are six of.
//
// CSS Values §5.1.1 makes the font-relative units in a font-size refer to the
// parent element's font — the element's own is the thing being computed — and
// says to assume half an em "in the cases where it is impossible or impractical
// to determine the x-height". The cascade has no faces, so half an em was the
// only answer it ever gave, and a face that states a real x-height was never
// asked. Ahem's is eight tenths of an em, so "6ex" against it came out at three
// quarters of the size the author wrote.

// fakeMetrics answers a fixed fraction of the size and records what it was asked.
type fakeMetrics struct {
	fraction float64
	known    bool
	families []string
}

func (m *fakeMetrics) XHeight(cs ComputedStyle, size Unit) (Unit, bool) {
	m.families = append(m.families, cs["font-family"])
	if !m.known {
		return 0, false
	}
	return size.Mul(m.fraction), true
}

// sizeOf computes one element's font-size through the cascade.
func sizeOf(t *testing.T, m Metrics, sheet, selector string) float64 {
	t.Helper()
	rules, errs := css.ParseStylesheet(sheet)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, `<div id="parent"><div id="child">x</div></div>`)
	got := ApplyWith(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}, m)
	n := elementFor(t, doc, selector)
	vals, _ := css.ParseComponentValues(got.Styles[n]["font-size"])
	l, _, ok := ParseLength(vals, LengthContext{})
	if !ok || l.Kind != LengthAbsolute {
		t.Fatalf("the computed font-size of %s is %q, which is not an absolute "+
			"length", selector, got.Styles[n]["font-size"])
	}
	return l.Value.Px()
}

const exSheet = `#parent { font-size: 20px } #child { font-size: 6ex }`

// TestAnExFontSizeUsesTheFacesOwnXHeight is the bug.
func TestAnExFontSizeUsesTheFacesOwnXHeight(t *testing.T) {
	// Eight tenths of the parent's 20px is 16, and six of those is 96 — which
	// is what numbers-units-012 draws against an inch.
	if got := sizeOf(t, &fakeMetrics{fraction: 0.8, known: true}, exSheet, "#child"); got != 96 {
		t.Errorf("\"6ex\" against a face whose x-height is 0.8em of 20px computed "+
			"to %gpx, want 96", got)
	}
}

// TestWithoutAFaceItIsStillHalfAnEm — the fallback CSS names, and the answer
// this gave before there was anywhere to ask. A caller with no fonts must get
// the same page it always did.
func TestWithoutAFaceItIsStillHalfAnEm(t *testing.T) {
	if got := sizeOf(t, nil, exSheet, "#child"); got != 60 {
		t.Errorf("with no metrics \"6ex\" computed to %gpx, want 60 (six halves of "+
			"the parent's 20px)", got)
	}
	// And the same when a face was found and states no x-height, which is the
	// common case for the fourteen standard faces.
	if got := sizeOf(t, &fakeMetrics{}, exSheet, "#child"); got != 60 {
		t.Errorf("with a face that states no x-height \"6ex\" computed to %gpx, "+
			"want 60", got)
	}
}

// TestTheParentsFaceIsTheOneAsked. §5.1.1 says the units refer to the parent
// element's font, and an element may declare a family of its own beside the
// size — the two are not resolved together, and taking this element's family
// would measure the six against a face the size is not being set in.
func TestTheParentsFaceIsTheOneAsked(t *testing.T) {
	m := &fakeMetrics{fraction: 0.8, known: true}
	sizeOf(t, m, `#parent { font-size: 20px; font-family: Outer }
		#child { font-size: 6ex; font-family: Inner }`, "#child")
	found := false
	for _, f := range m.families {
		if f == "Inner" {
			t.Errorf("the x-height was asked of %q; the element's own family is "+
				"not what its font-size is relative to", f)
		}
		if f == "Outer" {
			found = true
		}
	}
	if !found {
		t.Errorf("the x-height was asked of %v, and never of the parent's family", m.families)
	}
}

// TestAnElementThatOnlyInheritsIsNotResolvedAgain. The child of an element with
// an ex size takes the number, not the declaration — resolving it again at every
// level is what OwnFontSize exists to stop.
func TestAnElementThatOnlyInheritsIsNotResolvedAgain(t *testing.T) {
	rules, errs := css.ParseStylesheet(`#parent { font-size: 20px } #child { font-size: 6ex }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, `<div id="parent"><div id="child"><div id="grand">x</div></div></div>`)
	got := ApplyWith(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}},
		&fakeMetrics{fraction: 0.8, known: true})
	child := got.Styles[elementFor(t, doc, "#child")]["font-size"]
	grand := got.Styles[elementFor(t, doc, "#grand")]["font-size"]
	if child != grand {
		t.Errorf("the child computed to %q and its own child to %q; the second "+
			"only inherited a number", child, grand)
	}
}
