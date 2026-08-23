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
		"column-gap: normal",
		"column-fill: balance",
		"filter: none",
		"opacity: 1",
		"transform-style: flat",
		"text-decoration-skip-ink: auto",
		"border-radius: 0",
		"border-radius: 0px",
		"border-radius: 0em",
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
		"scroll-snap-type: initial",
		"mix-blend-mode: initial",
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
		"page-break-inside: avoid",
		"column-fill: auto", // the initial value is "balance"
		"column-gap: 0",     // the initial value is "normal"
		"column-width: 100px",
		"filter: blur(1px)",
		"opacity: 0.5",
		"opacity: 0",
		"border-radius: 20px",
		"text-decoration-skip-ink: none",
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

// TestTheOtherCSSWideKeywordsAreNotAssumedInert.
//
// "inherit" takes the parent's value, which for an inherited property can be
// anything; "unset" is inherit or initial depending on the property; "revert"
// depends on the cascade origin. None is resolved here, so none may be treated
// as the initial value — reading "unset" as inert would be right about half the
// properties in CSS and wrong about the other half.
func TestTheOtherCSSWideKeywordsAreNotAssumedInert(t *testing.T) {
	for _, decl := range []string{
		"resize: inherit",
		"resize: unset",
		"resize: revert",
	} {
		if !reportsUnsupported(t, decl) {
			t.Errorf("%q was not reported; this engine does not resolve that keyword, "+
				"so it cannot know the declaration asks for nothing", decl)
		}
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
	if got.Styles[n]["color"] == "" {
		t.Errorf("the declaration beside the inert one did not survive; suppressing a " +
			"finding must not drop the rule it was about")
	}
}
