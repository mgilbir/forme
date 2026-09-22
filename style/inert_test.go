package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// A declaration of an unimplemented property, at that property's initial value.
//
// The finding this narrows is a good one and stays: a page where a property was
// dropped is plausible and wrong, and the author has no other way to learn it.
// What it was not is *true of every such declaration*. A property's initial
// value is what the property means when nobody has said anything, and an engine
// that does not implement a property renders as though nobody had.
//
// Everything below is about keeping the rule on the value rather than on the
// property. "resize: none" asks for the engine's own behaviour; "resize: both"
// asks for a box a user can drag, which a browser gives a grab handle and this
// does not.

// findingsFor returns what the cascade reported about one declaration.
func findingsFor(t *testing.T, decl string) []Finding {
	t.Helper()
	doc := parseDoc(t, `<div id="target">x</div>`)
	rules, errs := css.ParseStylesheet(`#target { ` + decl + ` }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet %q reported %v", decl, errs)
	}
	return Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}).Findings
}

// reportsUnsupported says whether a declaration raised the unimplemented-property
// finding.
func reportsUnsupported(t *testing.T, decl string) bool {
	t.Helper()
	for _, f := range findingsFor(t, decl) {
		if f.Unsupported && strings.Contains(f.Message, "is not implemented") {
			return true
		}
	}
	return false
}

// TestADeclarationAtItsInitialValueIsNotReported is the case that prompted this:
// nineteen of the suite's reftests are textareas carrying the defensive
// boilerplate an author writes to make a control look like plain text, and
// "resize: none" was the one line of it that raised a finding.
func TestADeclarationAtItsInitialValueIsNotReported(t *testing.T) {
	for _, decl := range []string{
		"resize: none",
		"resize: NONE",
		"resize:none",
		"page-break-inside: auto",
		// "avoid" asks for no break inside the box, and this engine puts none
		// inside any box: it does not fragment at all, so a document that does
		// not fit is scaled to the page rather than broken across two of them.
		// The box the author did not want split is not split.
		//
		// This was in the list below, on the reading that "avoid" asks for
		// something. It asks for the *absence* of something, which is the case
		// an engine that never does it satisfies — the same shape as
		// "text-decoration-skip-ink: none".
		"page-break-inside: avoid",
		"break-inside: avoid",
		// The other four, for the same two values and by the same argument.
		// "auto" is the initial value and means "break here if the
		// fragmentation wants to"; where nothing fragments it is the page that
		// is already there. "avoid" asks for the *absence* of a break, which an
		// engine that never breaks satisfies.
		//
		// They were reported until the day this list was checked against what
		// the engine does rather than against what the properties are for, and
		// the finding they raised was about a difference that does not exist.
		"break-before: auto",
		"break-after: auto",
		"page-break-before: auto",
		"page-break-after: auto",
		"break-before: avoid",
		"break-after: avoid",
		"page-break-before: avoid",
		"page-break-after: avoid",
		"break-before: AVOID",
		"column-gap: normal",
		"column-fill: balance",
		"filter: none",
		"opacity: 1",
		"transform-style: flat",
		"border-radius: 0",
		"border-radius: 0px",
		"border-radius: 0em",
		// Both values of this one, and it is the only property here with two.
		// A decoration is drawn straight through a descender: "auto" permits
		// that and "none" asks for it, so an engine that always does it
		// satisfies either. layout/textdecoration_test.go holds the fact.
		"text-decoration-skip-ink: auto",
		"text-decoration-skip-ink: none",
		// The rest of the defensive reset a real stylesheet writes. Every one of
		// these was reported until the table was checked against what this
		// engine draws, and every one of them says "make sure nothing here is
		// doing anything" about a thing it never does.
		"box-shadow: none",
		"text-shadow: none",
		"cursor: auto",
		"content-visibility: visible",
		"contain: none",
		"isolation: auto",
		"mix-blend-mode: normal",
		"backdrop-filter: none",
		"appearance: auto",
		"clip-path: none",
		"mask: none",
		"perspective: none",
		"text-emphasis: none",
		"text-emphasis-style: none",
		"font-variant-alternates: normal",
		"scroll-snap-type: none",
		"scroll-margin: 0",
		"scroll-margin: 0px",
		"scroll-padding: auto",
		"overflow-anchor: auto",
		"scrollbar-color: auto",
		"outline-offset: 0",
		"outline-offset: 0px",
		// The second property here with two values, and for the same reason as
		// the first: "auto" leaves the underline's position to the user agent
		// and "from-font" demands it come from the face's own metrics. This
		// takes it from the face, so it satisfies the permitting value and the
		// demanding one. layout/textdecoration_test.go holds that fact too.
		"text-underline-position: auto",
		"text-underline-position: from-font",
		// The one property whose every value is inert, in four spellings of two
		// different points. It names where a transform turns about, and nothing
		// is transformed.
		"transform-origin: 50% 50%",
		"transform-origin: center",
		"transform-origin: top left",
		"transform-origin: 0 0",
		// The hyphens case again, found this time by looking for it rather than
		// by being caught out: what this engine produces is "none", because it
		// applies no variation to a face at all. The initial value is "auto",
		// and "auto" is in the list below.
		"font-optical-sizing: none",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported, and it asks for the page that is already there", decl)
		}
	}
}

// TestTheInitialKeywordIsResolvedRatherThanAssumedInert.
//
// "initial" stands for the property's initial value, which is *usually* what an
// engine that does not implement the property produces — and for hyphens is not.
// So it is resolved through the table and compared like any other value, rather
// than waved through.
//
// A property with no entry has no initial value here, so "initial" tells this
// nothing about it and it is reported as before. That is the conservative half
// and it matters: the alternative is assuming a coincidence for every property
// in CSS on the strength of it holding for most.
func TestTheInitialKeywordIsResolvedRatherThanAssumedInert(t *testing.T) {
	for _, decl := range []string{
		"resize: initial",
		"border-radius: initial",
		"column-gap: initial",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported, and this property's initial value is what the "+
				"engine produces", decl)
		}
	}
	for _, decl := range []string{
		// No entry, so nothing is known about what its initial value would mean.
		//
		// "scroll-snap-type" used to be one of these and is not any more: it
		// joined nomedium.go, so it is reported without the unsupported claim
		// and reportsUnsupported answers false for the reason this test is not
		// about. The two rules are independent and both examples below are
		// properties whose absence really does change a page.
		//
		// "mix-blend-mode" and "text-emphasis" stood here until they were given
		// entries of their own, which is what an example of "no entry" is always
		// at risk of. These two are picked to be far from the reset vein: a
		// shape a float wraps to, and a border painted from an image.
		"shape-outside: initial",
		"mask-border: initial",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported; this engine does not know that its initial "+
				"value is the behaviour it produces", decl)
		}
	}
}

// The two kerning properties were in both lists and are in neither now. They are
// in the registry, so the cascade says nothing about either — and the finding
// they had is raised by layout instead, which is the only place that can decide
// it: "font-kerning: none" asks for a face's kerning to be turned off, and a
// face with no kerning in it has none to turn off. See layout/textchecks.go.
//
// The four column properties went the same way for the same reason. A
// "column-count: 2" is laid out on a box this engine can divide and refused on
// the box beside it that holds a float, and only something that can see the box
// can tell those apart. See layout/multicol.go.

// "opacity" left both lists last, and the two declarations that were in the
// list below — "opacity: 0.5" and "opacity: 0" — went with it. Neither asks for
// a page this engine does not produce any more: the alpha is folded into every
// mark the box paints, which is exactly the group where the box paints one mark
// and exactly nothing where the alpha is zero. What it is not exact for is
// reported, at the box, because whether it was is a fact about what the box
// turned out to paint. See layout/opacity.go.

// TestADeclarationThatAsksForSomethingIsStillReported is the containment
// argument, and the half this change most needs to keep.
//
// Every one of these asks for a page different from the one produced. Going
// quiet about any of them would be the silent, plausible wrongness the finding
// exists to prevent — and would look exactly like this change if it went wrong.
func TestADeclarationThatAsksForSomethingIsStillReported(t *testing.T) {
	for _, decl := range []string{
		"resize: both",
		"resize: horizontal",
		// The break properties' other values, which ask for a break rather than
		// for the absence of one. An author who wrote any of them gets a page
		// that runs on, and that is the line between these and the entries
		// above: "auto" and "avoid" are satisfied by never breaking, and these
		// are not.
		"page-break-before: always",
		"page-break-after: always",
		"break-before: always",
		"break-after: always",
		"break-before: page",
		"break-after: page",
		"break-before: left",
		"break-before: column",
		"filter: blur(1px)",
		"border-radius: 20px",
		// The other half of every entry added above: the value that asks for a
		// page this engine does not draw. A shadow, a blend, a clip, a mask, a
		// mark over the text, an alternate glyph, a ring held off the border
		// edge — none of them arrives, and the author has no other way to learn
		// it.
		"box-shadow: 1px 1px red",
		"text-shadow: 1px 1px red",
		"content-visibility: hidden",
		"contain: paint",
		"isolation: isolate",
		"mix-blend-mode: multiply",
		"backdrop-filter: blur(2px)",
		// The opposite request to the one above: stripping a field back to a
		// plain box is what this engine does not do, because the chrome comes
		// from its user agent sheet and stays.
		"appearance: none",
		"clip-path: circle(40%)",
		"perspective: 500px",
		"text-emphasis: dot",
		"text-emphasis-style: circle",
		"font-variant-alternates: historical-forms",
		"outline-offset: 4px",
		"text-underline-position: under",
		// And the hyphens case from the other side. "auto" asks for the face's
		// optical size axis to be set from the font size, and this sets no axis;
		// "initial" stands for "auto" and is reported with it. A table written
		// from the specifications rather than from this engine would have these
		// two silent and "none" reported, which is exactly backwards.
		"font-optical-sizing: auto",
		"font-optical-sizing: initial",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported, and it asks for a page this engine does "+
				"not produce", decl)
		}
	}
}

// TestAnUnknownPropertyIsStillReported: the table narrows the finding for
// properties it names and must not widen anything. A property nobody has heard
// of has no initial value here and is reported as it always was.
func TestAnUnknownPropertyIsStillReported(t *testing.T) {
	for _, decl := range []string{
		"not-a-property-xyzzy: none",
		"not-a-property-xyzzy: auto",
		"resizey: none",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported", decl)
		}
	}
}

// TestACSSWideKeywordIsResolvedBeforeItIsJudgedInert.
//
// A keyword stands for a value rather than being one, so a comparison against
// the value this engine produces has to resolve it first. Which value it stands
// for depends on the property, and that is the whole difficulty: "unset" is the
// initial value on a property that does not inherit and the parent's on one that
// does, and "revert" and "revert-layer" come to the same thing here — this
// engine reverts to nothing and has no cascade layers.
//
// So the three of them are resolved for an entry that does not inherit and left
// alone for one that does. "inherit" is never resolved: the parent's value can
// be anything, and this is a question about one declaration.
//
// Only "initial" was resolved, and the rest were reported on the reading that
// treating them as inert "would be right about half the properties in CSS and
// wrong about the other half" — which is true, and is why inertValue now records
// which half each of its entries is in.
func TestACSSWideKeywordIsResolvedBeforeItIsJudgedInert(t *testing.T) {
	// "resize" does not inherit and this engine produces "none", which is what
	// all four of these come to.
	for _, decl := range []string{
		"resize: none", "resize: initial", "resize: unset",
		"resize: revert", "resize: revert-layer",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported; it is the page this engine already draws", decl)
		}
	}
	// "font-variation-settings" does inherit, so the parent's value decides and
	// this cannot know it.
	for _, decl := range []string{
		"font-variation-settings: unset", "font-variation-settings: revert",
		"font-variation-settings: revert-layer",
		"text-decoration-skip-ink: unset",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported; the property inherits, so the keyword "+
				"stands for a value this cannot see", decl)
		}
	}
	// And "inherit" is never resolvable, whichever half the property is in.
	for _, decl := range []string{"resize: inherit", "font-variation-settings: inherit"} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported; the parent's value can be anything", decl)
		}
	}
	// The control: a value that really does ask for something else.
	if !reportsUnsupported(t, "resize: both") {
		t.Error("\"resize: both\" was not reported, so this test is watching nothing")
	}
}

// TestAnInertDeclarationStillCascades. The finding is what changes, and nothing
// else: the value is still parsed and still ordered, so the day the property is
// implemented there is nothing here to undo.
func TestAnInertDeclarationStillCascades(t *testing.T) {
	doc := parseDoc(t, `<div id="target">x</div>`)
	rules, errs := css.ParseStylesheet(`#target { resize: none; color: red }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	n := elementFor(t, doc, "#target")
	if got.Styles[n].Get("color") == "" {
		t.Errorf("the declaration beside the inert one did not survive; suppressing a " +
			"finding must not drop the rule it was about")
	}
}

// TestTheDecorationStyleThisEngineDrawsIsInert. A decoration is drawn as a solid
// line, which is what text-decoration-style's initial value asks for — so
// "solid" asks for the page that is already there, whether it is written on its
// own or as a component of the text-decoration shorthand.
//
// The four it is not: each asks for a line this engine does not draw, and an
// author who wrote one would see a solid line instead and have no other way to
// find out.
func TestTheDecorationStyleThisEngineDrawsIsInert(t *testing.T) {
	for _, decl := range []string{
		"text-decoration-style: solid",
		"text-decoration-style: initial",
		"text-decoration: underline solid",
		"text-decoration: solid underline blue",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported, and it asks for the line that is already "+
				"drawn", decl)
		}
	}
	for _, decl := range []string{
		"text-decoration-style: double",
		"text-decoration-style: wavy",
		"text-decoration: underline dotted",
		"text-decoration: underline dashed blue",
		// Not a style at all: it asks for something this engine does not do.
		"text-decoration: underline blink",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported, and it asks for a line this engine does "+
				"not draw", decl)
		}
	}
}

// TestTheShorthandStillSetsTheLine, so that swallowing the style component
// cannot quietly swallow the declaration around it.
func TestTheShorthandStillSetsTheLine(t *testing.T) {
	doc := parseDoc(t, `<div id="target">x</div>`)
	rules, errs := css.ParseStylesheet(`#target { text-decoration: underline solid blue }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := got.Styles[elementFor(t, doc, "#target")]
	if cs.Get("text-decoration-line") != "underline" {
		t.Errorf("the line came out %q, want underline", cs.Get("text-decoration-line"))
	}
	if cs.Get("text-decoration-color") != "blue" {
		t.Errorf("the colour came out %q, want blue", cs.Get("text-decoration-color"))
	}
}

// TestNothingIsFragmented is what the "avoid" entries above claim, checked
// against the engine rather than asserted about it.
//
// A page-break-inside entry that is wrong is wrong invisibly: the finding it
// suppresses is the only thing that would have said so. So the claim is pinned
// where it can fail — the day this engine fragments, a box with "avoid" on it
// may be broken and the entry has to come out.
func TestNothingIsFragmented(t *testing.T) {
	// The property is registered nowhere and read nowhere: it is in
	// unimplementedProperties' spirit rather than its map, since being inert is
	// what keeps it quiet. If either of those changes, the entry needs revisiting.
	if _, ok := properties["page-break-inside"]; ok {
		t.Errorf("page-break-inside is in the registry now, so something reads it; " +
			"the inert entry claims nothing does")
	}
	if _, ok := properties["break-inside"]; ok {
		t.Errorf("break-inside is in the registry now, so something reads it")
	}
}

// TestAPropertyInBothTablesStillReportsItsOtherValues is the seam between this
// file and nomedium.go, and the new entries widened it.
//
// Several properties are named by both — "cursor", "pointer-events",
// "user-select" and "scroll-snap-type" among them — and the two say different
// things. inert.go says the do-nothing value asks for the page that is already
// there, so nothing is reported. nomedium.go says *no* value of the property
// changes this medium, so every other value is still reported and simply does
// not carry the claim that the page came out wrong.
//
// reportsUnsupported answers false for both, which is why this asks the weaker
// question instead: a finding was raised at all. An entry here with the wrong
// produced value would swallow these silently, and nothing else in this file
// would notice.
func TestAPropertyInBothTablesStillReportsItsOtherValues(t *testing.T) {
	for _, decl := range []string{
		"cursor: pointer",
		"cursor: not-allowed",
		"scroll-snap-type: x mandatory",
		"scroll-margin: 20px",
		"scroll-padding: 10px",
		"overflow-anchor: none",
		"scrollbar-color: red blue",
		"pointer-events: none",
		"user-select: none",
	} {
		if len(findingsFor(t, decl)) == 0 {
			t.Errorf("%q raised nothing; it was dropped, and an author who wrote it "+
				"is entitled to know that much", decl)
		}
	}
}

// TestNothingIsTransformed is to transform-origin what TestNothingIsFragmented
// is to the break properties, and it guards a stronger claim: that entry says
// *every* value of transform-origin is inert, which is only true while there is
// no transformation for an origin to belong to.
//
// The day any of these is registered and read, an origin decides where a box
// ends up and the entry has to come out with the same change.
func TestNothingIsTransformed(t *testing.T) {
	for _, name := range []string{"transform", "rotate", "scale", "translate", "perspective"} {
		if _, ok := properties[name]; ok {
			t.Errorf("%q is in the registry now, so something reads it; the "+
				"transform-origin entry claims every origin is inert because "+
				"nothing is transformed", name)
		}
	}
}

// TestAPropertyWithNoEffectInThisMediumIsInertWhateverItSays.
//
// CSS Writing Modes 4 §4.1's own note is the argument: text-orientation "has no
// effect in horizontal writing modes". A horizontal page is what almost every
// document is, so every value of it asks there for the page that is already
// there and there is no value to compare against.
//
// The three properties are registered now rather than inert, because layout
// reads them — see layout/writingmode.go — and what this asks is that
// registering them did not turn a silence into noise. A stylesheet that writes
// any of them still says nothing by itself. What says something is a *box* that
// declares a vertical mode this engine cannot lay out, and the claim that it
// does is carried by TestABoxThisEngineCannotTurnIsReported, which is in the
// package that can see a box.
func TestAPropertyWithNoEffectInThisMediumIsInertWhateverItSays(t *testing.T) {
	for _, decl := range []string{
		"text-orientation: mixed",
		"text-orientation: upright",
		"text-orientation: sideways",
		"text-orientation: initial",
		"text-orientation: UPRIGHT",
		// §9.1 says the same of this one: "no effect in horizontal typographic
		// modes". text-autospace-003 writes the two together under one comment.
		"text-combine-upright: none",
		"text-combine-upright: all",
		"text-combine-upright: digits 3",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported, and no value of it changes a horizontal page",
				decl)
		}
	}
	// And the property that decides the mode, which used to be reported here and
	// is not any more. The declaration alone no longer says whether the page is
	// wrong: the same "writing-mode: vertical-rl" is laid out on a box with a
	// height and refused on the box beside it with an automatic one, and only
	// something that can see the box can tell those apart.
	for _, decl := range []string{
		"writing-mode: horizontal-tb",
		"writing-mode: vertical-rl",
		"writing-mode: vertical-lr",
		"writing-mode: sideways-rl",
		"writing-mode: sideways-lr",
	} {
		if reportsUnsupported(t, decl) {
			t.Errorf("%q was reported by the cascade; whether a vertical box is "+
				"laid out is a question about the box, and layout answers it", decl)
		}
	}
	// All three cascade, which is what makes the layout-time answer possible at
	// all: a property the cascade dropped would never reach a box.
	for _, name := range []string{"writing-mode", "text-orientation", "text-combine-upright"} {
		if _, ok := properties[name]; !ok {
			t.Errorf("%q is not a registered property, so no computed style carries "+
				"it and layout cannot read it", name)
		}
	}
	// And it inherits, which is what makes one declaration on a container turn
	// everything inside it — the way every document that uses it is written.
	if !properties["writing-mode"].inherits {
		t.Error("writing-mode does not inherit, so a rule on a container leaves " +
			"its paragraphs horizontal")
	}
}
