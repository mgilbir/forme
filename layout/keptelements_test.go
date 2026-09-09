package layout

import (
	"strings"
	"testing"
)

// Three more elements refused for what they *do*, each an ordinary box while it
// does it.
//
// The pattern this table keeps repeating: an element whose dynamic half this
// engine cannot have was dropped whole, and what went with it was the author's
// own content. An <iframe> lost its box, the form controls lost their text, a
// <canvas> lost its bitmap's size, a <map> lost what it held. These three are
// the same again.

// keptText is every string a document's boxes would set.
func keptText(t *testing.T, markup string) string {
	t.Helper()
	built := Build(Input{HTML: markup})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	return textOfTree(built.Root)
}

// TestAnOutputKeepsItsText. "An output is computed by script" describes what
// fills one, not what one is: a document that writes the number has written it.
func TestAnOutputKeepsItsText(t *testing.T) {
	if got := keptText(t, `<p>total: <output id="o">42</output></p>`); !strings.Contains(got, "42") {
		t.Errorf("an <output>'s text is not on the page: %q", got)
	}
	built := Build(Input{HTML: `<output>42</output>`})
	if boxFor(built.Root, "output") == nil {
		t.Error("an <output> generated no box")
	}
}

// TestASlotRendersWhatItHolds. There is no shadow tree here and never will be,
// which is the case HTML's fallback content is written for. "display: contents"
// is HTML's own rule, so the content reaches the page without the slot's box.
func TestASlotRendersWhatItHolds(t *testing.T) {
	if got := keptText(t, `<div><slot>fallback</slot></div>`); !strings.Contains(got, "fallback") {
		t.Errorf("a <slot>'s fallback content is not on the page: %q", got)
	}
	// "display: contents" and not a box of its own, which is the difference
	// between rendering the children and rendering the slot.
	built := Build(Input{HTML: `<div><slot>fallback</slot></div>`})
	if boxFor(built.Root, "slot") != nil {
		t.Error("a <slot> generated a box of its own; HTML gives it " +
			"\"display: contents\", which is the children and not the element")
	}
}

// TestAMarqueeStandsStill. A page laid out once shows it not moving, which is
// what a browser asked to print one draws. What it must not do is lose the
// words.
func TestAMarqueeStandsStill(t *testing.T) {
	if got := keptText(t, `<marquee id="m">scrolling</marquee>`); !strings.Contains(got, "scrolling") {
		t.Errorf("a <marquee>'s text is not on the page: %q", got)
	}
}

// TestNoneOfTheThreeIsReportedAsUnsupported. What each was refused for is not
// something a printed page has, so none of them is a page missing what a reader
// would have seen.
func TestNoneOfTheThreeIsReportedAsUnsupported(t *testing.T) {
	for _, markup := range []string{
		`<output>42</output>`, `<slot>fallback</slot>`, `<marquee>scrolling</marquee>`,
	} {
		built := Build(Input{HTML: markup})
		for _, f := range built.Findings {
			if f.Unsupported() {
				t.Errorf("%s was reported as unsupported: %s", markup, f.Error())
			}
		}
	}
}

// TestTheWidgetsAreStillRefused is the boundary, and it is a test so that
// widening it is a decision somebody takes rather than one that slips through.
// Each of these needs something drawn from a state — a disclosure triangle and
// a rule that hides a closed element's content, or a control rendered from a
// value — which is a thing to build rather than a refusal to lift.
//
// <video> left the list, and the decision is the one this test exists to make
// visible. It is not a widget: it is a replaced element with a size HTML states,
// and the widget in it is the control bar, which is refused where it is asked
// for and reported there. See TestAVideoWithControlsSaysTheBarIsNotDrawn.
// <audio> stays, because HTML renders one without controls as "display: none"
// and one with them as a player whose size no specification states — there is no
// box to lose by refusing it.
func TestTheWidgetsAreStillRefused(t *testing.T) {
	for _, name := range []string{"details", "summary", "dialog", "audio", "progress", "meter"} {
		built := Build(Input{HTML: `<` + name + `>x</` + name + `>`})
		var said bool
		for _, f := range built.Findings {
			if f.Unsupported() && strings.Contains(f.Message, name) {
				said = true
			}
		}
		if !said {
			t.Errorf("<%s> is no longer refused and nothing says so: %v", name, built.Findings)
		}
	}
}
