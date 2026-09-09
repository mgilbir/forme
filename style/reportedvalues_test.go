package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// findingsOf runs an author stylesheet over a document and returns what it
// raised.
func findingsOf(t *testing.T, htmlSrc, authorCSS string) []Finding {
	t.Helper()
	rules, errs := css.ParseStylesheet(authorCSS)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet reported %v", errs)
	}
	doc := parseDoc(t, htmlSrc)
	return Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}}).Findings
}

// says reports whether any finding contains a phrase, and whether it claims the
// engine is missing something.
func says(findings []Finding, phrase string) (found, unsupported bool) {
	for _, f := range findings {
		if strings.Contains(f.Message, phrase) {
			return true, f.Unsupported
		}
	}
	return false, false
}

// TestEveryPseudoElementTheParserAcceptsIsComputed is the gap between two lists,
// and it is now empty.
//
// The selector parser's set of pseudo-elements and the cascade's are two answers
// to "which pseudo-elements does this engine have", and where they differ the
// rules written for the difference do nothing at all. "::first-letter" parsed,
// matched, and was never computed: a drop cap written the ordinary way was
// silently an ordinary first letter, on a page carrying no claim that anything
// was missing from it. It is computed now, which closes the gap — and closing a
// gap is exactly when the test for it has to stop naming the one case and start
// asserting the property.
//
// Driven from the parser rather than from a list written here: a name the parser
// refuses is refused as an unknown selector, which is a different report and not
// this one's business.
func TestEveryPseudoElementTheParserAcceptsIsComputed(t *testing.T) {
	// Candidates rather than the parser's own set, which is unexported. Every
	// pseudo-element css-pseudo-4 defines, so that one the parser learns to
	// accept is asked about here whether or not anybody remembers to add it.
	candidates := []string{
		"before", "after", "marker", "first-line", "first-letter",
		"selection", "target-text", "spelling-error", "grammar-error",
		"backdrop", "placeholder", "file-selector-button", "part", "slotted",
		"cue", "highlight",
	}
	accepted := 0
	for _, name := range candidates {
		vals, errs := css.ParseComponentValues("p::" + name)
		if len(errs) != 0 {
			continue
		}
		sels, _, ok := css.ParseSelectorList(vals)
		if !ok || len(sels) != 1 || sels[0].PseudoElement != name {
			continue
		}
		accepted++
		got := findingsOf(t, `<p id="a">x</p>`, `#a::`+name+` { color: red }`)
		if found, _ := says(got, "::"+name); found {
			t.Errorf("::%s is a selector this engine accepts and a style it does "+
				"not compute: %v", name, got)
		}
	}
	if accepted == 0 {
		t.Fatal("the parser accepted none of the candidates, so this asserts nothing")
	}
}

// TestAPseudoElementThatIsParsedAndNotComputedIsReported drives the backstop
// directly, because nothing else can reach it any more.
//
// reportUncomputedPseudo is derived from the two lists rather than written out,
// so that a pseudo-element the parser learns to accept is reported until this
// stage learns to compute it. With the lists equal there is no document that
// reaches it, and a guard nothing has been seen to fire is not a guard — so it
// is fired here, with a name from the far side of the gap the test above says is
// closed.
func TestAPseudoElementThatIsParsedAndNotComputedIsReported(t *testing.T) {
	s := Styler{seen: map[string]bool{}}
	s.reportUncomputedPseudo("first-word", 7)
	found, unsupported := says(s.findings, "::first-word")
	if !found {
		t.Fatalf("raised %v, want a finding naming ::first-word", s.findings)
	}
	if !unsupported {
		t.Error("the finding does not claim the engine is missing anything, " +
			"so a page relying on it still counts as clean")
	}
	// And a name it does compute says nothing, which is the half that keeps the
	// report off every document with a ::before in it.
	q := Styler{seen: map[string]bool{}}
	q.reportUncomputedPseudo("before", 7)
	if len(q.findings) != 0 {
		t.Errorf("a pseudo-element this stage computes was reported: %v", q.findings)
	}
}

// TestAPseudoElementThatIsComputedIsNotReported is the control: the four this
// stage does compute must say nothing, or every document with a ::before in it
// would claim a gap.
func TestAPseudoElementThatIsComputedIsNotReported(t *testing.T) {
	for _, name := range []string{"before", "after", "marker", "first-line"} {
		got := findingsOf(t, `<p id="a">x</p>`, `#a::`+name+` { color: red }`)
		if found, _ := says(got, "::"+name); found {
			t.Errorf("::%s is computed and was reported anyway: %v", name, got)
		}
	}
}

// TestSmallCapsInsideTheFontShorthandIsReported is the same gap between a
// longhand and the shorthand that sets it.
//
// "font-variant: small-caps" is reported as unimplemented and the same request
// written inside "font" was swallowed, so a page whose small capitals came out
// as ordinary letters carried no claim that anything was missing from it.
func TestSmallCapsInsideTheFontShorthandIsReported(t *testing.T) {
	got := findingsOf(t, `<p id="a">x</p>`, `#a { font: small-caps 12px serif }`)
	found, unsupported := says(got, "small-caps")
	if !found {
		t.Fatalf("raised %v, want a finding naming small-caps", got)
	}
	if !unsupported {
		t.Error("the finding does not claim the engine is missing anything")
	}
	// And the rest of the shorthand still applies, since the variant changes
	// nothing about what is produced.
	doc := parseDoc(t, `<p id="a">x</p>`)
	rules, _ := css.ParseStylesheet(`#a { font: small-caps 12px serif }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if !strings.Contains(cs["font-size"], "12") {
		t.Errorf("the size computed to %q; the shorthand still sets what it can",
			cs["font-size"])
	}
}

// TestASystemFontIsReportedOnceAndNotContradicted is the second finding that
// said the opposite of the first.
//
// "font: menu" is a system font: the shorthand says so and reports it as
// something this engine cannot produce, and then a second finding called the
// declaration "not a value this engine can read" — which is a statement about
// the author's CSS and is untrue.
func TestASystemFontIsReportedOnceAndNotContradicted(t *testing.T) {
	got := findingsOf(t, `<p id="a">x</p>`, `#a { font: menu }`)
	if found, _ := says(got, "system font"); !found {
		t.Fatalf("raised %v, want a finding naming the system font", got)
	}
	if found, _ := says(got, "not a value this engine can read"); found {
		t.Errorf("a system font was also reported as unreadable CSS: %v", got)
	}
}

// TestAnUnreadableShorthandIsStillReported is the control for that: a value the
// author really did get wrong must still be named.
func TestAnUnreadableShorthandIsStillReported(t *testing.T) {
	got := findingsOf(t, `<p id="a">x</p>`, `#a { font: nonsense }`)
	if found, _ := says(got, "not a value this engine can read"); !found {
		t.Errorf("raised %v, want the unreadable-value finding", got)
	}
}
