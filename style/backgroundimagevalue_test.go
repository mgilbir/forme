package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// A background-image value that is not an image, and the difference between the
// two things an engine can say about a bare box.
//
// "background-image: url(x.png) repeat" is a background-repeat value written
// where only an <image> belongs. It is not a hard image or an exotic one — it is
// not a declaration at all, CSS 2.1 §4.2 drops it, and every browser paints
// nothing. An engine that instead reports "this is an image I cannot paint" is
// claiming a gap it does not have, which is the difference between a reftest
// that passed and a reftest that was never really run.

// bgImageOf runs a two-rule cascade and returns the computed background-image
// together with what the cascade said about it.
func bgImageOf(t *testing.T, decl string) (string, []Finding) {
	t.Helper()
	author, errs := css.ParseStylesheet(
		`div { background-image: linear-gradient(green, green) } #target { ` + decl + ` }`)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet %q reported %v", decl, errs)
	}
	doc := parseDoc(t, `<div id="target">x</div>`)
	got := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: author}})
	n := elementFor(t, doc, "#target")
	return got.Styles[n]["background-image"], got.Findings
}

// TestAValueThatIsNotAnImageLeavesTheEarlierDeclarationStanding is why this
// belongs in the cascade and not in the painter.
//
// Read at paint time, the invalid declaration is still the winning one — it has
// the higher specificity, that is why it is there — and the box comes out bare.
// Dropped where CSS drops it, the rule underneath gets to apply.
func TestAValueThatIsNotAnImageLeavesTheEarlierDeclarationStanding(t *testing.T) {
	got, _ := bgImageOf(t, "background-image: url(x.png) repeat")
	if !strings.Contains(got, "gradient") {
		t.Errorf("background-image computed to %q, want the div rule's gradient: the "+
			"invalid declaration should never have reached the cascade's answer", got)
	}
}

// TestAnInvalidBackgroundImageIsReportedWithoutBeingCalledUnsupported.
//
// The author still has to learn their declaration went nowhere. What must not
// happen is the engine calling it a missing feature: nothing is missing, a
// stylesheet said something CSS forbids and CSS says what becomes of it.
func TestAnInvalidBackgroundImageIsReportedWithoutBeingCalledUnsupported(t *testing.T) {
	_, findings := bgImageOf(t, "background-image: url(x.png) repeat")
	found := false
	for _, f := range findings {
		if f.Property != "background-image" {
			continue
		}
		found = true
		if f.Unsupported {
			t.Errorf("the finding is marked unsupported: %q", f.Message)
		}
		if !strings.Contains(f.Message, "repeat") {
			t.Errorf("the finding does not name the value: %q", f.Message)
		}
	}
	if !found {
		t.Error("nothing was reported for \"background-image: url(x.png) repeat\"")
	}
}

// TestEveryImageValueStillApplies is the half that must not be got wrong.
//
// Dropping a value that really is an image would put a *lower* rule back in
// charge of the box, and would do it silently — the page still has a background,
// just the wrong one. Which of these this engine can paint is a separate
// question, asked later and answered with a finding of its own.
func TestEveryImageValueStillApplies(t *testing.T) {
	for _, decl := range []string{
		"background-image: none",
		"background-image: url(a.png)",
		`background-image: url("a.png")`,
		"background-image: linear-gradient(red, blue)",
		"background-image: LINEAR-GRADIENT(red, blue)",
		"background-image: repeating-radial-gradient(red, blue)",
		"background-image: image-set(url(a.png) 1x)",
		"background-image: cross-fade(url(a.png), url(b.png))",
		"background-image: element(#x)",
		"background-image: -webkit-linear-gradient(red, blue)",
		// A layer list is a list of images, and "none" is one of them.
		"background-image: none, url(a.png)",
		"background-image: url(a.png), linear-gradient(red, blue), none",
		// The CSS-wide keywords are the cascade's own business.
		"background-image: inherit",
		"background-image: initial",
		"background-image: unset",
		"background-image: revert",
	} {
		got, findings := bgImageOf(t, decl)
		if strings.Contains(got, "gradient(green, green)") {
			t.Errorf("%q was dropped: background-image computed to the div rule's %q", decl, got)
		}
		for _, f := range findings {
			if f.Property == "background-image" && !f.Unsupported {
				t.Errorf("%q was reported as invalid: %q", decl, f.Message)
			}
		}
	}
}

// TestAValueThatIsNotAnImageIsDropped is the containment in the other
// direction, and each entry is a shape that would otherwise reach the painter
// and be reported as a feature this engine lacks.
func TestAValueThatIsNotAnImageIsDropped(t *testing.T) {
	for _, decl := range []string{
		// background-image-005's own declaration: a repeat where an image goes.
		"background-image: url(x.png) repeat",
		"background-image: url(a.png) url(b.png)",
		"background-image: none none",
		"background-image: red",
		"background-image: 1px",
		"background-image: \"a.png\"",
		"background-image: blah",
		// A CSS-wide keyword is the whole value or it is nothing.
		"background-image: none, inherit",
	} {
		got, _ := bgImageOf(t, decl)
		if !strings.Contains(got, "gradient(green, green)") {
			t.Errorf("%q computed to %q, want the div rule's gradient: it is not an "+
				"image and the declaration is invalid", decl, got)
		}
	}
}
