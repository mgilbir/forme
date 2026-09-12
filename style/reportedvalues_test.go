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

// TestSmallCapsInsideTheFontShorthandSetsTheLonghand is the gap between a
// longhand and the shorthand that sets it, closed.
//
// It used to be a report: neither "font-variant: small-caps" nor the same
// request written inside "font" set anything, and only the longhand said so —
// the shorthand swallowed the keyword, so a page whose small capitals came out
// as ordinary letters carried no claim that anything was missing from it. Both
// now set font-variant-caps, and whether the page is wrong is a question about
// the face that sets it, which layout asks. See layout/textchecks.go.
func TestSmallCapsInsideTheFontShorthandSetsTheLonghand(t *testing.T) {
	if got := findingsOf(t, `<p id="a">x</p>`, `#a { font: small-caps 12px serif }`); len(got) != 0 {
		t.Errorf("raised %v; the shorthand sets a property this engine reads "+
			"and the stylesheet has nothing wrong with it", got)
	}
	doc := parseDoc(t, `<p id="a">x</p>`)
	rules, _ := css.ParseStylesheet(`#a { font: small-caps 12px serif }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if cs["font-variant-caps"] != "small-caps" {
		t.Errorf("font-variant-caps computed to %q, want small-caps",
			cs["font-variant-caps"])
	}
	// And the rest of the shorthand still applies.
	if !strings.Contains(cs["font-size"], "12") {
		t.Errorf("the size computed to %q; the shorthand still sets what it can",
			cs["font-size"])
	}
}

// TestTheFontShorthandResetsSmallCaps is the half of the shorthand rule that is
// easy to leave out.
//
// "font: 12px serif" names no variant, so it sets font-variant-caps back to
// "normal" — which is what makes the shorthand undo an inherited small-caps
// rather than quietly keeping it.
func TestTheFontShorthandResetsSmallCaps(t *testing.T) {
	doc := parseDoc(t, `<div id="outer"><p id="a">x</p></div>`)
	rules, _ := css.ParseStylesheet(
		`#outer { font-variant: small-caps } #a { font: 12px serif }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if cs := styled.Styles[elementFor(t, doc, "#outer")]; cs["font-variant-caps"] != "small-caps" {
		t.Fatalf("the container's font-variant-caps is %q; without it the reset "+
			"below has nothing to undo", cs["font-variant-caps"])
	}
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if cs["font-variant-caps"] != "normal" {
		t.Errorf("font-variant-caps is %q inside a small-caps container after "+
			"\"font: 12px serif\", want normal", cs["font-variant-caps"])
	}
}

// TestFontVariantSetsBothLonghandsItControls is the reset in the other
// direction, which is the whole reason "font-variant" is expanded rather than
// read as a value.
//
// §6.10 makes it a shorthand, so "font-variant: small-caps" on a span inside a
// paragraph that turned its ligatures off puts them back.
func TestFontVariantSetsBothLonghandsItControls(t *testing.T) {
	doc := parseDoc(t, `<p id="outer"><span id="a">x</span></p>`)
	rules, _ := css.ParseStylesheet(
		`#outer { font-variant: none } #a { font-variant: small-caps }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if cs := styled.Styles[elementFor(t, doc, "#outer")]; cs["font-variant-ligatures"] != "none" {
		t.Fatalf("\"font-variant: none\" left font-variant-ligatures at %q, "+
			"want none", cs["font-variant-ligatures"])
	}
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if cs["font-variant-caps"] != "small-caps" {
		t.Errorf("font-variant-caps is %q, want small-caps", cs["font-variant-caps"])
	}
	if cs["font-variant-ligatures"] != "normal" {
		t.Errorf("font-variant-ligatures is %q inside a container that turned "+
			"them off, want the normal the shorthand resets it to",
			cs["font-variant-ligatures"])
	}
}

// TestAFontVariantValueThisEngineHasNoLonghandForIsReported is what is left of
// the groups the shorthand controls.
//
// Five of the seven longhands are registered, and of the two that are not,
// font-variant-emoji is not this shorthand's business at all — so what reaches
// this is font-variant-alternates, whose one keyword has nothing to be set on.
// Refusing the declaration whole is what raises the unsupported-property
// finding, and swallowing it silently is what that finding exists to stop.
func TestAFontVariantValueThisEngineHasNoLonghandForIsReported(t *testing.T) {
	for _, value := range []string{
		"historical-forms", "styleset(ss01)", "swash(x)",
		// font-variant-emoji's three, which the shorthand's grammar has taken
		// on in the current draft. They are a choice between the glyphs of
		// different *fonts* rather than a feature of one, so they are not the
		// kind of request the rest of this family makes and are refused with
		// the alternates beside them.
		"text", "emoji", "unicode",
	} {
		got := findingsOf(t, `<p id="a">1</p>`, `#a { font-variant: `+value+` }`)
		found, unsupported := says(got, "font-variant")
		if !found {
			t.Errorf("%q raised %v, want a finding naming font-variant", value, got)
			continue
		}
		if !unsupported {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything", value)
		}
	}
}

// TestAFontVariantNumericValueSetsTheLonghand is the group that stopped being
// one of them.
//
// §6.7's eight keywords were reported as a part of the shorthand that is not
// implemented, and are now expanded into font-variant-numeric like the
// ligatures and the capitals beside them.
func TestAFontVariantNumericValueSetsTheLonghand(t *testing.T) {
	if got := findingsOf(t, `<p id="a">1</p>`,
		`#a { font-variant: oldstyle-nums tabular-nums slashed-zero }`); len(got) != 0 {
		t.Errorf("raised %v; the shorthand sets a property this engine reads "+
			"and the stylesheet has nothing wrong with it", got)
	}
	doc := parseDoc(t, `<p id="a">1</p>`)
	rules, _ := css.ParseStylesheet(
		`#a { font-variant: oldstyle-nums tabular-nums slashed-zero }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if want := "oldstyle-nums tabular-nums slashed-zero"; cs["font-variant-numeric"] != want {
		t.Errorf("font-variant-numeric computed to %q, want %q",
			cs["font-variant-numeric"], want)
	}
	// And the two beside it are reset, which is the whole reason the property
	// is expanded rather than read.
	if cs["font-variant-caps"] != "normal" {
		t.Errorf("font-variant-caps is %q, want normal", cs["font-variant-caps"])
	}
	if cs["font-variant-ligatures"] != "normal" {
		t.Errorf("font-variant-ligatures is %q, want normal",
			cs["font-variant-ligatures"])
	}
}

// TestTheFontVariantShorthandResetsTheNumericLonghand is the other half of the
// reset, and the half a shorthand is most likely to be missing.
//
// "font-variant: small-caps" on a span inside a paragraph that asked for
// oldstyle figures puts the figures back, because a shorthand sets every
// longhand it controls. A version that set only what was written would leave the
// span's digits oldstyle, which is a page the stylesheet does not explain.
func TestTheFontVariantShorthandResetsTheNumericLonghand(t *testing.T) {
	doc := parseDoc(t, `<p id="outer"><span id="a">1</span></p>`)
	rules, _ := css.ParseStylesheet(
		`#outer { font-variant: oldstyle-nums } #a { font-variant: small-caps }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if cs := styled.Styles[elementFor(t, doc, "#outer")]; cs["font-variant-numeric"] != "oldstyle-nums" {
		t.Fatalf("the container's font-variant-numeric is %q; without it the "+
			"reset below has nothing to undo", cs["font-variant-numeric"])
	}
	cs := styled.Styles[elementFor(t, doc, "#a")]
	if cs["font-variant-numeric"] != "normal" {
		t.Errorf("font-variant-numeric is %q inside an oldstyle container after "+
			"\"font-variant: small-caps\", want normal", cs["font-variant-numeric"])
	}
	if cs["font-variant-caps"] != "small-caps" {
		t.Errorf("font-variant-caps is %q, want small-caps", cs["font-variant-caps"])
	}
}

// TestAFontVariantEastAsianValueSetsTheLonghand, and the reset that comes with
// it.
//
// §6.9's nine keywords were reported as a part of the shorthand that is not
// implemented, and are now expanded into font-variant-east-asian like the three
// beside them.
func TestAFontVariantEastAsianValueSetsTheLonghand(t *testing.T) {
	if got := findingsOf(t, `<p id="a">x</p>`,
		`#a { font-variant: jis78 full-width ruby }`); len(got) != 0 {
		t.Errorf("raised %v; the shorthand sets a property this engine reads "+
			"and the stylesheet has nothing wrong with it", got)
	}
	doc := parseDoc(t, `<p id="outer"><span id="a">x</span></p>`)
	rules, _ := css.ParseStylesheet(
		`#outer { font-variant: jis78 full-width ruby }
		 #a { font-variant: small-caps }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	outer := styled.Styles[elementFor(t, doc, "#outer")]
	if want := "jis78 full-width ruby"; outer["font-variant-east-asian"] != want {
		t.Fatalf("font-variant-east-asian computed to %q, want %q",
			outer["font-variant-east-asian"], want)
	}
	// And the span resets it, which is the whole reason the property is
	// expanded rather than read.
	if cs := styled.Styles[elementFor(t, doc, "#a")]; cs["font-variant-east-asian"] != "normal" {
		t.Errorf("font-variant-east-asian is %q inside a jis78 container after "+
			"\"font-variant: small-caps\", want normal", cs["font-variant-east-asian"])
	}
}

// TestAFontVariantPositionValueSetsTheLonghand, and the reset with it.
func TestAFontVariantPositionValueSetsTheLonghand(t *testing.T) {
	if got := findingsOf(t, `<p id="a">x</p>`, `#a { font-variant: super }`); len(got) != 0 {
		t.Errorf("raised %v; the shorthand sets a property this engine reads "+
			"and the stylesheet has nothing wrong with it", got)
	}
	doc := parseDoc(t, `<p id="outer"><span id="a">x</span></p>`)
	rules, _ := css.ParseStylesheet(
		`#outer { font-variant: super } #a { font-variant: small-caps }`)
	styled := Apply(doc, []Sheet{{Origin: OriginAuthor, Rules: rules}})
	if cs := styled.Styles[elementFor(t, doc, "#outer")]; cs["font-variant-position"] != "super" {
		t.Fatalf("font-variant-position computed to %q, want super",
			cs["font-variant-position"])
	}
	if cs := styled.Styles[elementFor(t, doc, "#a")]; cs["font-variant-position"] != "normal" {
		t.Errorf("font-variant-position is %q inside a super container after "+
			"\"font-variant: small-caps\", want normal", cs["font-variant-position"])
	}
}

// TestBothPositionsAtOnceAreRefused.
//
// §6.5 is one group of two: a value naming both asks for the same character
// above and below the line at once.
func TestBothPositionsAtOnceAreRefused(t *testing.T) {
	got := findingsOf(t, `<p id="a">x</p>`, `#a { font-variant: sub super }`)
	found, unsupported := says(got, "not a value this engine can read")
	if !found {
		t.Fatalf("raised %v, want it refused as a value", got)
	}
	if unsupported {
		t.Error("it is invalid CSS and was reported as a missing feature")
	}
}

// TestAGroupOfFontVariantEastAsianWrittenTwiceIsRefused.
//
// §6.9's first group is six alternatives rather than a pair: the four Japanese
// standards were revisions of one thing and "simplified" and "traditional" are
// two forms of one character, so any two of the six ask for the same ideograph
// in two shapes.
func TestAGroupOfFontVariantEastAsianWrittenTwiceIsRefused(t *testing.T) {
	for _, value := range []string{
		"jis78 jis83",
		"jis04 traditional",
		"simplified traditional",
		"full-width proportional-width",
		"ruby ruby",
	} {
		got := findingsOf(t, `<p id="a">x</p>`, `#a { font-variant: `+value+` }`)
		found, unsupported := says(got, "not a value this engine can read")
		if !found {
			t.Errorf("%q raised %v, want it refused as a value", value, got)
			continue
		}
		if unsupported {
			t.Errorf("%q is invalid CSS and was reported as a missing feature",
				value)
		}
	}
}

// TestAGroupOfFontVariantNumericWrittenTwiceIsRefused.
//
// §6.7's grammar is a "||" of five terms and a term may appear once, so
// "lining-nums oldstyle-nums" asks for both sets of figures at once and is not
// a value. It is the author's mistake rather than a missing feature, which is a
// different report — see the message the finding carries.
func TestAGroupOfFontVariantNumericWrittenTwiceIsRefused(t *testing.T) {
	for _, value := range []string{
		"lining-nums oldstyle-nums",
		"tabular-nums proportional-nums",
		"diagonal-fractions stacked-fractions",
		"ordinal ordinal",
	} {
		got := findingsOf(t, `<p id="a">1</p>`, `#a { font-variant: `+value+` }`)
		found, unsupported := says(got, "not a value this engine can read")
		if !found {
			t.Errorf("%q raised %v, want it refused as a value", value, got)
			continue
		}
		if unsupported {
			t.Errorf("%q is invalid CSS and was reported as a missing feature",
				value)
		}
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
