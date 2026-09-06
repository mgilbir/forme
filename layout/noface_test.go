package layout

import (
	"testing"
)

// TestAPageWithNoFaceAtAllIsRefused is the loudest thing this engine ought to
// be able to say, and it said nothing at all.
//
// fontFor falls back to font-family's initial value when the families a
// document named are not available, and reports that it did. When the initial
// family is not available either it returned "no face" — and every box that
// asked simply had its text dropped. A document with words in it came out as a
// blank page, with no findings and Refused false, which a caller cannot tell
// apart from a document that said nothing.
func TestAPageWithNoFaceAtAllIsRefused(t *testing.T) {
	in := Input{HTML: `<p>hello</p>`, Fonts: emptyFontSet{}}

	built := Build(in)
	got := Compose(in, Options{})

	if !hasRule(got.Findings, RuleNoFace) {
		t.Errorf("a document set in a set with no faces raised %v, want the no-face finding",
			ruleNames(got.Findings))
	}
	if !got.Refused {
		t.Error("a document that drew no text at all was not refused")
	}
	for _, op := range got.Ops {
		if _, ok := op.(DrawText); ok {
			t.Error("text was drawn by a set with no faces")
		}
	}
	if built.Root == nil {
		t.Fatal("the box tree was not built; the finding is about the text, not the boxes")
	}
	fired[RuleNoFace] = true
}

// TestTheNoFaceFindingIsRaisedOnce keeps it from becoming one finding per box:
// every box in the document ends up at the same answer, and a page of a
// thousand paragraphs would otherwise fill the finding list with one fact.
func TestTheNoFaceFindingIsRaisedOnce(t *testing.T) {
	var html string
	for i := 0; i < 50; i++ {
		html += "<p>hello</p>"
	}
	got := Compose(Input{HTML: html, Fonts: emptyFontSet{}}, Options{})
	n := 0
	for _, f := range got.Findings {
		if f.Rule == RuleNoFace {
			n++
		}
	}
	if n != 1 {
		t.Errorf("fifty paragraphs raised the no-face finding %d times, want once", n)
	}
}

// TestASetThatHasTheInitialFamilyIsNotRefused is the control. A set that lacks
// the family a document named but has the initial one is the ordinary
// substitution, which is reported as a fallback and is not this.
func TestASetThatHasTheInitialFamilyIsNotRefused(t *testing.T) {
	got := Compose(Input{
		HTML:  `<p style="font-family: Nonesuch">hello</p>`,
		Fonts: StandardFonts(),
	}, Options{})

	if hasRule(got.Findings, RuleNoFace) {
		t.Error("a document whose named family was missing was reported as having no face at all")
	}
	if !hasRule(got.Findings, RuleFontFallback) {
		t.Errorf("raised %v, want the fallback finding", ruleNames(got.Findings))
	}
	if got.Refused {
		t.Errorf("a substituted family refused the document: %v", got.Findings)
	}
	var drew bool
	for _, op := range got.Ops {
		if _, ok := op.(DrawText); ok {
			drew = true
		}
	}
	if !drew {
		t.Error("no text was drawn, so the two cases are not being told apart")
	}
}
