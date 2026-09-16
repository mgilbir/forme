package style

import (
	"github.com/mgilbir/forme/css"
)

// CSS 2.1 §4.2's other half: a declaration whose *value* is not one the
// property takes is dropped whole, and what stands is whatever the cascade
// would have produced without it.
//
// These six questions were written inline in expand, one after another, each
// ending in a report and a dropped declaration. They are gathered here because
// a second caller needs the same answers without the reports: @supports asks
// whether this engine understands a declaration, and the honest answer to
// "(display: grid)" is the one the cascade would give the declaration itself.
// Asked in two places from two lists, the two would drift, and the drift would
// be invisible — a condition answering yes about a declaration the very next
// rule drops is exactly the failure supportsDeclaration is written to avoid.
//
// What is *not* here is every other property. This engine has no single place
// that says whether a value parses — that is decided per property, in the stage
// that reads it — so these are the ones whose value the cascade reads early
// enough to refuse. For the rest the question is still answered about the
// property alone, and supports.go says what that costs.

// dropsForValue reports whether §4.2 drops this declaration for its value, and
// the reason to give if it does.
func dropsForValue(name string, value []css.ComponentValue) (string, bool) {
	if nonNegative[name] && hasNegativeNumber(value) {
		// A declaration whose value is illegal is not a declaration with a
		// strange value: CSS 2.1 §4.2 says the whole declaration is dropped, and
		// what stands is whatever the cascade would have produced without it.
		//
		// That is why this cannot be done where the value is read. "height: 0;
		// height: -1px" has to compute to zero, and a layout that refuses the
		// negative number sees only the last declaration and falls back to
		// auto — which is a full-height box where the author asked for none.
		// The suite has thirty-five tests of exactly that shape, one per
		// property per unit, and they are what found it.
		//
		// The finding is not marked unsupported. Nothing is missing from the
		// engine here; a stylesheet said something CSS forbids and CSS says
		// what to do about it.
		return "\"" + name + ": " + serialize(value) + "\" is negative, which " +
			name + " does not allow, so the declaration was dropped", true
	}

	if colourValued[name] && !legalColour(name, value) {
		// §4.2 again, and the same reason it cannot wait until the value is
		// read: "color: 'red'" is a string where a colour belongs, so the
		// declaration is invalid and is dropped, and what stands is whatever the
		// cascade would have produced without it.
		//
		// Read at use time instead, the invalid declaration is still the winning
		// one — it has the higher specificity, that is why it is there — and the
		// colour comes out as the property's initial value. colors-007 is four
		// paragraphs that must each be green, and two of them came out black:
		// the lower-specificity "p.incorrect { color: green }" never got to
		// apply, because the declaration that should have been thrown away was
		// still standing in front of it.
		//
		// Not marked unsupported. Nothing is missing from the engine; a
		// stylesheet said something CSS forbids and CSS says what to do about
		// it.
		return "\"" + name + ": " + serialize(value) + "\" is not a colour, " +
			"so the declaration was dropped", true
	}

	if name == "background-image" && !legalBackgroundImage(value) {
		// §4.2 a third time. "background-image: url(x.png) repeat" is a
		// background-repeat value written where only an <image> belongs, so
		// there is no declaration here at all and nothing paints — which is
		// what every browser shows, and is why this is not a gap in the engine.
		//
		// It matters that this is not the unsupported report the painter would
		// otherwise raise. That report says "a browser draws something here and
		// this does not", and the whole reftest ratchet is built on the
		// difference: CSS2/backgrounds/background-image-005 asks for green text
		// and gets it, and was counted as a vacuous pass for years because the
		// engine claimed to be missing an image no engine draws.
		//
		// Not marked unsupported, for the reason the checks above are not.
		return "\"background-image: " + serialize(value) + "\" is not an " +
			"image, so the declaration was dropped", true
	}

	if name == "display" && !legalDisplay(value) {
		// §4.2 once more, and this one has a visible cost in the other
		// direction. An engine that reads an unrecognised display value as the
		// property's *initial* value makes the element inline, and the initial
		// value is what CSS says the property means when nobody has set it —
		// which is not this case. The declaration is invalid, so it never
		// happened, and what stands is what the cascade would have produced
		// without it: the user agent sheet's "div { display: block }".
		//
		// The two answers are as far apart as they can be. CSS2/abspos/
		// static-fixed-inside-abspos writes "display: absolute" — the author
		// meant "position" — on a div whose background is the green square the
		// test is about. Read as inline, the div has no in-flow content, so it
		// has no line box, so nothing of it is painted at all and the page is
		// the red square underneath.
		//
		// It is also what makes the prefixed idiom work. An author who writes
		// "display: -moz-box; display: flex" is relying on the first
		// declaration being thrown away by everything that does not know it,
		// and an engine that instead lets it stand as "inline" gets neither.
		//
		// Not marked unsupported, for the reason the checks above are not:
		// nothing is missing here, a stylesheet said something CSS forbids and
		// CSS says what to do about it.
		return "\"display: " + serialize(value) + "\" is not a display " +
			"value, so the declaration was dropped", true
	}

	if name == "quotes" && !legalQuotes(value) {
		// §12.3.2's grammar is "[<string> <string>]+ | none", so an odd number of
		// strings names a level with an opening mark and no closing one and is not
		// a value at all. §4.2 drops it, and dropping it here rather than where it
		// is read is what makes the *inherited* pairs stand: a child of an element
		// that set two good pairs must go on using them, and an engine that fell
		// back to the initial value at read time would quote the child in a
		// different alphabet from its parent.
		//
		// Not marked unsupported, for the same reason the negative lengths above
		// are not: nothing is missing from the engine, and CSS says what to do.
		return "\"quotes: " + serialize(value) + "\" is not a list of pairs of " +
			"strings, so the declaration was dropped", true
	}

	if name == "content" && !legalCounterFunctions(value) {
		// §12.2's grammar gives the two counter functions fixed argument lists:
		//
		//	counter(<identifier>) | counter(<identifier>, <list-style-type>)
		//	counters(<identifier>, <string>) | counters(<identifier>, <string>, <list-style-type>)
		//
		// so "counter(c, '.')" names a separator on the function that has none,
		// and "counter(c, decimal, decimal)" gives two styles to a function that
		// takes one. Neither is a value, and §4.2 drops the declaration.
		//
		// Dropping it here is what makes the *earlier* declaration stand, which
		// is the whole of what a test of this can observe: content-counter-016
		// writes "content: counter(c)" and then four malformed ones after it,
		// and requires the numbering to come out 1 to 12 from the first. Read at
		// use time instead, the last declaration is the only one left and the
		// page numbers every item 1000.
		//
		// Not marked unsupported, for the same reason the two checks above are
		// not: nothing is missing from the engine, and CSS says what to do.
		return "\"content: " + serialize(value) + "\" calls a counter " +
			"function with arguments it does not take, so the declaration was dropped", true
	}

	return "", false
}
