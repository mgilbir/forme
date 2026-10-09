package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// The CSS-wide keywords in an @page rule.
//
// CSS Page 3 §1.1 gives every descriptor of the page context the CSS-wide
// keywords, and §4.4 cascades the page context "just like declarations in
// style rule for elements". They were read as lengths, and every one of them
// was reported as invalid CSS — "the @page margin "initial" is not a margin" —
// with the page keeping the margin an earlier declaration had set, which is
// the opposite of what "initial" asks for.
//
// The caller's margin is this engine's user agent stylesheet: what a sheet no
// rule spoke about is printed with. So "initial" and "unset" are the margin's
// initial value, zero, which consults no stylesheet; "revert" removes the
// author's rules and lands on the caller's; and "inherit" asks for the root
// element's margin, which is not known when the page is settled, and is
// reported as unsupported.

// widekeywordOpts is a 600 by 800 point sheet with the caller's margin at half
// an inch: 800px wide, 704px of it inside the margins.
func widekeywordOpts() Options { return Options{Page: PageSizePt(600, 800).WithMarginPt(36)} }

// pageFindings is every finding about sheet.css in a document whose
// stylesheet is css.
func pageFindings(css string) []Finding {
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		CSS:  []Stylesheet{{Name: "sheet.css", Source: "body { margin: 0 }" + css}},
	}, widekeywordOpts())
	var out []Finding
	for _, f := range got.Findings {
		if f.Source.Sheet == "sheet.css" {
			out = append(out, f)
		}
	}
	return out
}

// TestAPageMarginTakesTheCSSWideKeywords is each keyword's margin, and that
// none of them is reported: each is a value the cascade answers.
func TestAPageMarginTakesTheCSSWideKeywords(t *testing.T) {
	for _, c := range []struct {
		css  string
		want float64
	}{
		// The margin's initial value is zero, however it is spelled, and it
		// beats an earlier declaration as any later one does.
		{`@page { margin: initial }`, 800},
		{`@page { margin: INITIAL }`, 800},
		{`@page { margin: unset }`, 800},
		{`@page { margin: 1in; margin: initial }`, 800},
		{`@page { margin-left: initial }`, 752},
		{`@page { margin-right: unset }`, 752},
		{`@page { margin: initial; margin-left: 1in }`, 704},
		// revert takes away every author rule, the earlier one too, and the
		// caller's margin is what is under them.
		{`@page { margin: revert }`, 704},
		{`@page { margin: 1in; margin: revert }`, 704},
		{`@page { margin: 1in } @page { margin-left: revert }`, 656},
		// And every layer of it, which is what tells it from revert-layer.
		{`@layer a { @page { margin: 1in } } @page { margin: revert }`, 704},
		// An important revert removes the author's normal declarations as
		// well: the whole origin goes.
		{`@page { margin: 2in } @page { margin: revert !important }`, 704},
		// revert-layer takes away its own layer and nothing else.
		{`@layer a { @page { margin: 1in } } @page { margin: revert-layer }`, 608},
		{`@layer a { @page { margin: 1in } } @layer b { @page { margin: revert-layer } }`, 608},
		{`@page { margin: 1in } @page { margin: revert-layer }`, 704},
		// A roll-back that meets another rolls back again.
		{`@layer a { @page { margin: 2in } } @layer b { @page { margin: revert } } ` +
			`@page { margin: revert-layer }`, 704},
	} {
		if got := pageWidthPx(t, c.css, widekeywordOpts()); got != c.want {
			t.Errorf("%s: %gpx of content, want %g", c.css, got, c.want)
		}
		if fs := pageFindings(c.css); len(fs) != 0 {
			t.Errorf("%s was reported: %v", c.css, fs)
		}
	}
}

// TestAPageMarginThatInheritsIsReportedAsUnsupported is the one keyword that
// cannot be answered: the page context inherits from the root element (CSS
// Page 3 §6), and the margins are settled before any element is styled. It is
// valid CSS, so it is this engine's gap and not the author's mistake, and the
// declaration is dropped so what it would have overridden stands.
func TestAPageMarginThatInheritsIsReportedAsUnsupported(t *testing.T) {
	for _, c := range []struct {
		css  string
		want float64
	}{
		{`@page { margin: inherit }`, 704},
		{`@page { margin: 1in; margin: inherit }`, 608},
		{`@page { margin-top: inherit }`, 704},
	} {
		if got := pageWidthPx(t, c.css, widekeywordOpts()); got != c.want {
			t.Errorf("%s: %gpx of content, want %g", c.css, got, c.want)
		}
		fs := pageFindings(c.css)
		if len(fs) != 1 || !fs[0].Unsupported() ||
			!strings.Contains(fs[0].Message, `"inherit" takes the root element's margin`) {
			t.Errorf("%s: want one unsupported finding about inherit, got %v", c.css, fs)
		}
	}
	// One among other values is not the keyword at all, and is invalid.
	fs := pageFindings(`@page { margin: 1cm initial }`)
	if len(fs) != 1 || fs[0].Rule != RuleInvalidCSS {
		t.Errorf(`"margin: 1cm initial": want one invalid-css finding, got %v`, fs)
	}
}

// TestAPageSizeTakesTheCSSWideKeywords: size's initial value is "auto", the
// caller's sheet, and it does not inherit — the root element has no size but
// the initial one — so every keyword but the roll-backs is that. revert
// removes the author's rules, and under them is the same sheet.
func TestAPageSizeTakesTheCSSWideKeywords(t *testing.T) {
	for _, css := range []string{
		`@page { size: A5; size: initial }`,
		`@page { size: A5; size: unset }`,
		`@page { size: A5; size: inherit }`,
		`@page { size: A5; size: revert }`,
		`@page { size: 10in 5in } @page { size: revert-layer }`,
	} {
		if got := pageSheetPx(t, css, sheet600x800()); got != 800 {
			t.Errorf("%s: the sheet is %gpx wide, want the caller's 800", css, got)
		}
		if fs := pageFindings(css); len(fs) != 0 {
			t.Errorf("%s was reported: %v", css, fs)
		}
	}
	// And revert-layer onto a lower layer's size.
	if got := pageSheetPx(t, `@layer a { @page { size: 10in 5in } } @page { size: revert-layer }`,
		sheet600x800()); got != 960 {
		t.Errorf("revert-layer onto a layer's 10in: the sheet is %gpx wide, want 960", got)
	}
}

// TestRevertRollsBackToTheOriginBelow is revert across origins, which the
// documents above cannot reach: Compose takes author sheets only. An author's
// revert falls to the user's rule, and a user's to the user agent's.
func TestRevertRollsBackToTheOriginBelow(t *testing.T) {
	rule := func(src string, origin style.Origin) pendingPage {
		rules, errs := css.ParseStylesheet(src)
		if len(errs) != 0 || len(rules) != 1 {
			t.Fatalf("%s: %d rules, %v", src, len(rules), errs)
		}
		return pendingPage{rule: rules[0], origin: origin}
	}
	base := PageSizePt(600, 800).WithMarginPt(36)
	ua := rule(`@page { margin-left: 1in }`, style.OriginUserAgent)
	user := rule(`@page { margin-left: 2in }`, style.OriginUser)
	for _, c := range []struct {
		what  string
		pages []pendingPage
		want  float64
	}{
		{"an author's revert over a user's rule", []pendingPage{ua, user,
			rule(`@page { margin-left: revert }`, style.OriginAuthor)}, 192},
		{"a user's revert over a user agent's rule", []pendingPage{ua,
			rule(`@page { margin-left: revert }`, style.OriginUser)}, 96},
		{"a user agent's revert", []pendingPage{
			rule(`@page { margin-left: revert }`, style.OriginUserAgent)}, 48},
	} {
		rec := NewRecorder(nil)
		got := applyPageRules(base, c.pages, rec).Margin.Left.Px()
		if got != c.want {
			t.Errorf("%s: the left margin is %gpx, want %g", c.what, got, c.want)
		}
		if fs := rec.Findings(); len(fs) != 0 {
			t.Errorf("%s was reported: %v", c.what, fs)
		}
	}
}
