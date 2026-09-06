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

// TestAPseudoElementThatIsParsedAndNotComputedIsReported is the gap between two
// lists.
//
// The selector parser's set of pseudo-elements and the cascade's are two
// answers to "which pseudo-elements does this engine have", and where they
// differ the rules written for the difference do nothing at all.
// "::first-letter" parsed, matched, and was never computed: a drop cap written
// the ordinary way was silently an ordinary first letter, on a page carrying no
// claim that anything was missing from it.
func TestAPseudoElementThatIsParsedAndNotComputedIsReported(t *testing.T) {
	got := findingsOf(t, `<p id="a">x</p>`, `#a::first-letter { font-size: 200% }`)
	found, unsupported := says(got, "::first-letter")
	if !found {
		t.Fatalf("raised %v, want a finding naming ::first-letter", got)
	}
	if !unsupported {
		t.Error("the finding does not claim the engine is missing anything, " +
			"so a page relying on it still counts as clean")
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
