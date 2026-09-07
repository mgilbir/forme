package layout

import (
	"path/filepath"
	"strings"
	"testing"
)

// Where a styling finding happened.
//
// A document is styled by several sheets — its own <style>, every <link>, and
// everything those @import — and a byte offset means nothing without the name
// of the one it is into. Every finding the cascade raised came out as "byte N
// of the stylesheet" with no name on it, which for a document with five sheets
// is an offset into one of five files and no way to tell which.
//
// Two of the three answers were not offsets into a stylesheet at all: a
// declaration written in a style attribute is in the markup, and a finding
// about the styling as a whole — matching that stopped early, "revert" being
// unimplemented — is in no file.

// findingFor returns the first finding whose message holds substr.
func findingFor(t *testing.T, fs []Finding, substr string) Finding {
	t.Helper()
	for _, f := range fs {
		if strings.Contains(f.Message, substr) {
			return f
		}
	}
	t.Fatalf("no finding mentioning %q; findings: %v", substr, fs)
	return Finding{}
}

// TestAStylingFindingNamesTheSheetItIsIn.
func TestAStylingFindingNamesTheSheetItIsIn(t *testing.T) {
	dir := cssDir(t)
	writeCSS(t, filepath.Join(dir, "a.css"), "p { mix-blend-mode: multiply }")
	writeCSS(t, filepath.Join(dir, "deep", "b.css"), "p { backdrop-filter: blur(2px) }")
	writeCSS(t, filepath.Join(dir, "outer.css"), `@import "deep/b.css";`)

	built := buildLinking(t, dir,
		`<link rel=stylesheet href="a.css">`+
			`<link rel=stylesheet href="outer.css">`+
			`<style>p { text-size-adjust: none }</style><p id=p>x</p>`)

	for _, tc := range []struct{ property, sheet, what string }{
		{"mix-blend-mode", "a.css", "a linked sheet"},
		{"backdrop-filter", "deep/b.css", "a sheet reached by @import"},
		{"text-size-adjust", "", "the document's own <style>, which has no name"},
	} {
		f := findingFor(t, built.Findings, tc.property)
		if f.Source.Sheet != tc.sheet {
			t.Errorf("%s: the finding about %s names sheet %q, want %q",
				tc.what, tc.property, f.Source.Sheet, tc.sheet)
		}
		if f.Source.CSSOffset < 0 {
			t.Errorf("%s: the finding about %s has no stylesheet offset",
				tc.what, tc.property)
		}
	}
}

// TestAFindingFromAStyleAttributeIsInTheMarkup. The declaration was written in
// the document, and its own offset is into the attribute's value — a string the
// author has no file of. What they can be pointed at is the element.
func TestAFindingFromAStyleAttributeIsInTheMarkup(t *testing.T) {
	const doc = `<p>x</p><p id=p style="mix-blend-mode: multiply">y</p>`
	built := Build(Input{HTML: doc})
	f := findingFor(t, built.Findings, "mix-blend-mode")
	if f.Source.CSSOffset >= 0 {
		t.Errorf("the finding is at stylesheet byte %d; the declaration is in the "+
			"markup and there is no stylesheet", f.Source.CSSOffset)
	}
	want := strings.Index(doc, `<p id=p`)
	if f.Source.HTMLOffset != want {
		t.Errorf("the finding is at markup byte %d, want %d — the element carrying "+
			"the attribute", f.Source.HTMLOffset, want)
	}
}

// TestAFindingAboutTheWholeStylingIsInNoFile. "revert" is not implemented and
// is reported once for the document; reporting it at byte nought of a
// stylesheet sends an author to the top of a file to look for something that is
// not there.
func TestAFindingAboutTheWholeStylingIsInNoFile(t *testing.T) {
	built := Build(Input{
		HTML: `<p id=p>x</p>`,
		CSS:  []Stylesheet{{Source: "p { color: red } p { color: revert }"}},
	})
	f := findingFor(t, built.Findings, "revert")
	if f.Source.CSSOffset >= 0 || f.Source.HTMLOffset >= 0 {
		t.Errorf("the finding is at CSS byte %d and markup byte %d; it is about the "+
			"document's styling and is in neither",
			f.Source.CSSOffset, f.Source.HTMLOffset)
	}
}
