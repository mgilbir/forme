package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// A vendor-prefixed property, and what dropping one claims.
//
// Finding.Unsupported means the page differs from the one the stylesheet
// describes. "-moz-tab-size" is Gecko's property rather than CSS's, and every
// engine that is not Gecko drops it — which is what the prefix is for, and why
// an author writes the standard property beside it. So the stylesheet describes
// this page to every engine but one, and claiming otherwise makes an ordinary
// document look defective.
//
// It is still reported. An author who wrote only the prefixed spelling has a
// page missing what they asked for and no other way to learn it. What changes is
// the claim, not the message.

// findingFor returns the first finding a declaration raised.
func findingFor(t *testing.T, decl string) (Finding, bool) {
	t.Helper()
	for _, f := range findingsFor(t, decl) {
		return f, true
	}
	return Finding{}, false
}

func TestAPrefixedPropertyIsReportedWithoutClaimingThePageIsWrong(t *testing.T) {
	for _, decl := range []string{
		"-moz-tab-size: 100px",
		"-webkit-text-stroke-width: 2px",
		"-ms-flex-direction: row",
		"-o-transition: all 1s",
		// A prefix nobody has heard of has the same shape and the same meaning.
		"-forme-invented: 1px",
	} {
		f, ok := findingFor(t, decl)
		if !ok {
			t.Errorf("%q was dropped without a word", decl)
			continue
		}
		if f.Unsupported {
			t.Errorf("%q was reported as unsupported (%q); it is another engine's "+
				"property and dropping it is what every other engine does",
				decl, f.Message)
		}
		if !strings.Contains(f.Message, "not implemented") {
			t.Errorf("%q was reported as %q, which does not say it was not applied",
				decl, f.Message)
		}
	}
}

// TestAnUnprefixedPropertyNobodyImplementsIsStillUnsupported is the containment
// argument. A name without a prefix is CSS's, or is meant to be, and a page
// missing it is a page this engine got wrong.
func TestAnUnprefixedPropertyNobodyImplementsIsStillUnsupported(t *testing.T) {
	for _, decl := range []string{
		"scroll-snap-type: x mandatory",
		"mix-blend-mode: multiply",
		"not-a-property-xyzzy: 1px",
		// A leading dash with no vendor identifier after it is not the reserved
		// shape: "--x" is a custom property and "-x" is nobody's prefix.
		"-x: 1px",
	} {
		f, ok := findingFor(t, decl)
		if !ok {
			t.Errorf("%q was dropped without a word", decl)
			continue
		}
		if !f.Unsupported {
			t.Errorf("%q was reported without claiming the page differs (%q)",
				decl, f.Message)
		}
	}
}

// TestAPrefixedPropertyThisEngineImplementsIsNotReportedAtAll, which is the case
// the prefix rule must not swallow: being prefixed is not what makes a property
// unread.
func TestAPrefixedPropertyThisEngineImplementsIsNotReportedAtAll(t *testing.T) {
	for _, decl := range []string{
		"-webkit-line-clamp: 3",
		"-webkit-box-orient: vertical",
	} {
		if _, ok := findingFor(t, decl); ok {
			f, _ := findingFor(t, decl)
			t.Errorf("%q was reported (%q), and this engine reads it", decl, f.Message)
		}
	}
}

// TestTheDeclarationBesideItStillCascades, so that a prefixed property in the
// middle of a rule cannot take the rule with it.
func TestTheDeclarationBesideItStillCascades(t *testing.T) {
	doc := parseDoc(t, `<div id="target">x</div>`)
	rules, errs := css.ParseStylesheet(
		`#target { -moz-tab-size: 100px; tab-size: 100px; color: red }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := got.Styles[elementFor(t, doc, "#target")]
	if cs["tab-size"] != "100px" || cs["color"] != "red" {
		t.Errorf("the declarations beside the prefixed one came out %q and %q",
			cs["tab-size"], cs["color"])
	}
}
