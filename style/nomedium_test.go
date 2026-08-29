package style

import (
	"strings"
	"testing"
)

// A property with nothing to apply to on a page, and what dropping one claims.
//
// Finding.Unsupported means the page differs from the one the stylesheet
// describes. Nobody puts a text cursor in a printed paragraph, so "caret-color:
// red" colours nothing there — and a browser printing the same document applies
// it exactly as little. The page *is* the one CSS describes.
//
// It is still reported, for the reason the prefixed properties are: an author
// who wrote it has had it dropped and may want to know. What changes is the
// claim, not the message. See vendorprefix_test.go next door, which is the same
// distinction drawn about a name, and css/selector_test.go, which is it drawn
// about a selector.

func TestAPropertyWithNothingToApplyToIsReportedWithoutClaimingThePageIsWrong(t *testing.T) {
	for _, decl := range []string{
		// The insertion point, which this engine paints nowhere.
		"caret-color: red",
		"caret-shape: block",
		"caret-animation: manual",
		// The pointer and the selection, which a page has neither of.
		"cursor: pointer",
		"pointer-events: none",
		"user-select: none",
		"touch-action: pan-x",
		// Scrolling, which a page does not do.
		"scroll-behavior: smooth",
		"overscroll-behavior: contain",
		"scroll-snap-type: x mandatory",
		"scroll-snap-align: center",
		// Change over time, of which a page rendered once has none.
		"transition: all 1s",
		"transition-duration: 2s",
		// And the hint whose definition says it has no rendering effect.
		"will-change: transform",
	} {
		f, ok := findingFor(t, decl)
		if !ok {
			t.Errorf("%q was dropped without a word", decl)
			continue
		}
		if f.Unsupported {
			t.Errorf("%q was reported as unsupported (%q); there is nothing on a "+
				"page for it to have applied to", decl, f.Message)
		}
		if !strings.Contains(f.Message, "not implemented") {
			t.Errorf("%q was reported as %q, which does not say it was not applied",
				decl, f.Message)
		}
	}
}

// TestAPropertyThatWouldHaveChangedThePageIsStillUnsupported is the containment
// argument, and the reason this is a list rather than a rule about interaction.
//
// "animation" is the near miss. An animation with a delay, a fill mode or an
// infinite duration puts an element in a state that is not the one the other
// declarations describe, and a page rendered once shows that state — so a
// dropped animation really can leave the page different.
//
// "overflow" and "resize" are the miss from the other side: both change the
// *layout* of a page nobody scrolls or drags. Only the act is missing.
func TestAPropertyThatWouldHaveChangedThePageIsStillUnsupported(t *testing.T) {
	for _, decl := range []string{
		"animation: spin 1s infinite",
		"animation-delay: -1s",
		"overflow: hidden",
		"resize: both",
		"mix-blend-mode: multiply",
		"text-emphasis: filled dot",
		"clip-path: circle(50%)",
	} {
		f, ok := findingFor(t, decl)
		if !ok {
			continue // implemented, and then it is not this test's business
		}
		if !f.Unsupported {
			t.Errorf("%q was reported without claiming the page differs (%q); "+
				"dropping it changes what is drawn", decl, f.Message)
		}
	}
}

// TestNothingInTheTableIsImplemented keeps the table from becoming a lie.
//
// An entry claims a property is dropped *and* that dropping it changes nothing.
// If the property were implemented the first half would be false, and the entry
// would be quietly downgrading a finding that is never raised — which reads as
// harmless and is how a table stops describing the engine.
func TestNothingInTheTableIsImplemented(t *testing.T) {
	for name := range noEffectOnAPage {
		if _, ok := properties[name]; ok {
			t.Errorf("%q is in the registry, so it is read; an entry here claims "+
				"it is dropped", name)
		}
		if _, ok := shorthands[name]; ok {
			t.Errorf("%q is a shorthand this engine expands, so it is not dropped", name)
		}
	}
}

// TestTheTableIsPropertiesAndNotValues, because the two mechanisms beside each
// other are easy to confuse and mean different things.
//
// inert.go asks whether a *value* asks for the page that is already there, and
// silences the finding where it does — there is nothing to tell an author about
// "user-select: auto". This table says no value of these properties can change a
// page, so a declaration of one is still reported and still is not a page this
// engine got wrong.
func TestTheTableIsPropertiesAndNotValues(t *testing.T) {
	// The initial value: silenced by inert.go, so no finding at all.
	if _, ok := findingFor(t, "user-select: auto"); ok {
		f, _ := findingFor(t, "user-select: auto")
		t.Errorf("the initial value was reported (%q); it asks for the page that "+
			"is already there", f.Message)
	}
	// A value that asks for something: reported, and not unsupported.
	f, ok := findingFor(t, "user-select: none")
	if !ok {
		t.Fatal("\"user-select: none\" was dropped without a word")
	}
	if f.Unsupported {
		t.Errorf("\"user-select: none\" claimed the page differs (%q)", f.Message)
	}
}
