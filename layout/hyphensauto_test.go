package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// "hyphens: auto" and the half of §6.1 that is about what a UA is *required* to
// do.
//
// This engine has a hyphenation resource for one language, so a word in English
// is broken where the dictionary allows and a word in anything else only where a
// soft hyphen asks — which is "manual" — and it says so. That is a page with
// looser lines than a browser would set, which a reader cannot see and a finding
// has to name.
//
// But §6.1 does not ask a UA to hyphenate everything:
//
//	Correct automatic hyphenation requires a hyphenation resource appropriate
//	to the language of the text being broken. The UA is therefore only required
//	to automatically hyphenate text for which the author has declared a language
//	(e.g. via HTML lang or XML xml:lang) and for which it has an appropriate
//	hyphenation resource.
//
// A document that never says what language it is in gets no hyphenation from any
// conforming engine, so this one's page is not missing anything: it is the page
// the specification asks for, and there is nothing to report. The suite says so
// by name — hyphens-auto-001 is titled "automatic hyphenation must not work
// without language tagging" and passes by nothing being hyphenated.
//
// Reporting it anyway was the same mistake inert.go corrects for a declaration
// at its initial value: the finding was true of the property rather than of what
// the property was being asked to do.

// hyphensFindings lays a document out and returns what was said about hyphens.
func hyphensFindings(t *testing.T, htmlSrc string) []Finding {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults +
		`div, span, p { font-family: Courier; font-size: 20px; width: 60px }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, StandardFonts(), rec)
	var out []Finding
	for _, f := range append(append([]Finding(nil), built.Findings...), rec.Findings()...) {
		if f.Property == "hyphens" {
			out = append(out, f)
		}
	}
	return out
}

// TestHyphensAutoIsNotReportedWithoutALanguage is the bug.
func TestHyphensAutoIsNotReportedWithoutALanguage(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"nothing says what language it is in",
			`<div style="hyphens:auto">implementation</div>`},
		{"an empty lang, which HTML says is no language at all",
			`<div lang="" style="hyphens:auto">implementation</div>`},
	} {
		if got := hyphensFindings(t, tc.html); len(got) != 0 {
			t.Errorf("%s: reported %q — no conforming engine hyphenates text with no "+
				"declared language, so this page is the one §6.1 asks for",
				tc.what, got[0].Message)
		}
	}
}

// TestHyphensAutoIsStillReportedWithALanguageThisHasNoPatternsFor is the half
// the finding keeps, and it is the reason §6.1's second condition matters.
//
// The sentence has two: a language the author declared, *and* "an appropriate
// hyphenation resource". This engine ships one language's patterns, so a
// document in German asks for something it will not get — the page really does
// differ from the one the author asked for and from the one every browser
// produces, and that is a limitation worth naming.
func TestHyphensAutoIsStillReportedWithALanguageThisHasNoPatternsFor(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"a language with no patterns here",
			`<div lang="de" style="hyphens:auto">Wiedervereinigung</div>`},
		{"on an ancestor, which is where a document usually says it",
			`<div lang="fr"><p style="hyphens:auto">implementation</p></div>`},
		{"a region subtag, which is a language all the same",
			`<div lang="de-AT" style="hyphens:auto">Wiedervereinigung</div>`},
	} {
		got := hyphensFindings(t, tc.html)
		if len(got) == 0 {
			t.Errorf("%s: nothing was reported; the author asked for hyphenation in a "+
				"language this engine has no resource for", tc.what)
			continue
		}
		if !strings.Contains(got[0].Message, "read as manual") {
			t.Errorf("%s: reported %q", tc.what, got[0].Message)
		}
		if !got[0].Unsupported() {
			t.Errorf("%s: the finding was not marked unsupported", tc.what)
		}
	}
}

// TestHyphensAutoInEnglishIsNotReported is the other side of the same sentence.
// The resource is there, the words are broken where it says, and there is
// nothing missing from the page to tell an author about.
func TestHyphensAutoInEnglishIsNotReported(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"on the element itself",
			`<div lang="en" style="hyphens:auto">implementation</div>`},
		{"on an ancestor",
			`<div lang="en"><p style="hyphens:auto">implementation</p></div>`},
		{"a region subtag",
			`<div lang="en-US" style="hyphens:auto">implementation</div>`},
		{"a region and a variant",
			`<div lang="en-GB-oxendict" style="hyphens:auto">implementation</div>`},
	} {
		if got := hyphensFindings(t, tc.html); len(got) != 0 {
			t.Errorf("%s: reported %q — English is hyphenated here", tc.what, got[0].Message)
		}
	}
}

// TestTheOtherHyphensValuesAreNeverReported, with a language or without one.
// Both are implemented, and a finding about either would be a finding about a
// page that is exactly right.
func TestTheOtherHyphensValuesAreNeverReported(t *testing.T) {
	for _, value := range []string{"manual", "none"} {
		for _, lang := range []string{"", ` lang="en"`} {
			html := `<div` + lang + ` style="hyphens:` + value + `">imple&shy;mentation</div>`
			if got := hyphensFindings(t, html); len(got) != 0 {
				t.Errorf("hyphens: %s%s reported %q", value, lang, got[0].Message)
			}
		}
	}
}

// TestTheLanguageDecidesWhetherTheWordIsBroken is §6.1's first condition,
// measured on the page rather than in the report.
//
// "The UA is therefore only required to automatically hyphenate text for which
// the author has declared a language". A document that says nothing about its
// language is not hyphenated by any conforming engine, so the same word is one
// line untagged and several tagged — and hyphens-auto-001, "automatic
// hyphenation must not work without language tagging", passes by the first of
// those.
//
// This test asserted the opposite while there was no dictionary here to apply:
// the language then changed the finding and nothing else, and a page that moved
// with it would have meant the finding was hiding a difference. Now there is
// one, and the page moving is the feature.
func TestTheLanguageDecidesWhetherTheWordIsBroken(t *testing.T) {
	lines := func(htmlSrc string) []string {
		t.Helper()
		root := layoutOf(t, 600, htmlSrc,
			noDefaults+`div { font-family: Courier; font-size: 20px; width: 60px }`)
		var out []string
		var walk func(*Fragment)
		walk = func(f *Fragment) {
			if f == nil {
				return
			}
			if f.Box != nil && f.Box.Element != nil {
				if id, _ := f.Box.Element.Attr("id"); id == "d" {
					for _, ln := range f.Lines {
						var b strings.Builder
						for _, r := range ln.Runs {
							b.WriteString(r.Text)
						}
						out = append(out, b.String())
					}
				}
			}
			for _, c := range f.Children {
				walk(c)
			}
		}
		walk(root)
		return out
	}
	// Untagged: one line, overflowing, which is what hyphens-auto-001 asserts.
	plain := lines(`<div id="d" style="hyphens:auto">implementation</div>`)
	if len(plain) != 1 || plain[0] != "implementation" {
		t.Errorf("the untagged document set %q; with no declared language there is "+
			"nothing to break the word with", plain)
	}
	// Tagged English: broken where the dictionary allows. "implementation" is
	// im-ple-men-ta-tion, and 60px of Courier at 20px is five characters, so
	// every part but the last takes a line of its own with its hyphen.
	tagged := lines(`<div id="d" lang="en" style="hyphens:auto">implementation</div>`)
	want := []string{"im-", "ple-", "men-", "ta-", "tion"}
	if strings.Join(tagged, "|") != strings.Join(want, "|") {
		t.Errorf("the tagged document set %q, want %q", tagged, want)
	}
	// And a language with no patterns here is the untagged case again.
	german := lines(`<div id="d" lang="de" style="hyphens:auto">implementation</div>`)
	if strings.Join(german, "|") != strings.Join(plain, "|") {
		t.Errorf("a document tagged de set %q and an untagged one %q; there are no "+
			"German patterns here, so neither is hyphenated", german, plain)
	}
}
