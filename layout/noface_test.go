package layout

import (
	"strings"
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

// TestNoFaceNamesEachFamilyAndItsElement is issue #958's reproduction. A set
// restricted on purpose — here one with nothing in it, so that only what the
// document's @font-face rule brings can set text — leaves some boxes resolved
// and some not. The finding was raised once per document, quoted the initial
// "serif" whatever the box had asked for, and carried no source, so a caller
// was refused and could not tell which families to supply or where.
func TestNoFaceNamesEachFamilyAndItsElement(t *testing.T) {
	html := `<style>@font-face { font-family: Own; src: url(own.ttf) }</style>` +
		`<p id="own" style="font-family: Own">covered</p>` +
		`<p id="sans" style="font-family: sans-serif">not covered</p>` +
		`<p id="mono" style="font-family: monospace">not covered</p>` +
		`<p id="mono2" style="font-family: monospace">not covered either</p>` +
		`<p id="monob" style="font-family: monospace; font-weight: bold">nor this</p>` +
		`<p id="plain">not covered (initial family)</p>`
	got := Compose(Input{
		HTML:      html,
		Resources: &fileResolver{files: map[string][]byte{"own.ttf": realFont()}},
		Fonts:     emptyFontSet{},
	}, Options{})

	if !got.Refused {
		t.Fatal("text that could not be drawn did not refuse the document")
	}
	var noFace []Finding
	for _, f := range got.Findings {
		if f.Rule == RuleNoFace {
			noFace = append(noFace, f)
		}
	}
	// One per face to supply: sans-serif, monospace, bold monospace and the
	// initial family. The two regular monospace paragraphs are one finding,
	// since they need the same face; the bold one needs another, and says so.
	// The paragraph in Own is set and not reported.
	want := []struct{ quoted, element string }{
		{`"sans-serif", nor for the initial "serif"`, "sans"},
		{`"monospace", nor for the initial "serif"`, "mono"},
		{`"monospace" (weight 700), nor for the initial "serif"`, "monob"},
		{`"serif", the initial family`, "plain"},
	}
	if len(noFace) != len(want) {
		t.Fatalf("got %d no-face findings, want one per face to supply (%d): %v",
			len(noFace), len(want), noFace)
	}
	for i, w := range want {
		f := noFace[i]
		if !strings.Contains(f.Message, w.quoted) {
			t.Errorf("finding %d says %q, want it to name %s", i, f.Message, w.quoted)
		}
		// The source is the element whose text was dropped: its offset is
		// where that paragraph's tag begins in the document.
		at := f.Source.HTMLOffset
		if at < 0 || at >= len(html) || !strings.HasPrefix(html[at:], `<p id="`+w.element+`"`) {
			t.Errorf("finding %d points at offset %d, want the <p id=%q> whose text was not drawn",
				i, at, w.element)
		}
	}
	// And the covered paragraph was drawn: the restriction is not a blank page.
	var drew bool
	for _, op := range got.Ops {
		if d, ok := op.(DrawText); ok && strings.Contains(d.Text, "covered") {
			drew = true
		}
	}
	if !drew {
		t.Error("the paragraph in the document's own face was not drawn")
	}
	fired[RuleNoFace] = true
}
