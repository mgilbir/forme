package layout

import (
	"strings"
	"testing"
)

// The sheet is 600 x 800pt, which is 800 x 1066.67px, and every margin below is
// a whole number of inches — 96px each — so the content width is exact and a
// margin read a unit wrong shows as a wrong number rather than a rounding.
func pageWidthPx(t *testing.T, css string, opts Options) float64 {
	t.Helper()
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		// The body's own margin is the user agent's eight pixels and would sit
		// between the page margin and the box measured here, hiding the number
		// this is about behind a constant.
		CSS: []Stylesheet{{Name: "sheet.css", Source: "body { margin: 0 }" + css}},
	}, opts)
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatalf("the box is not on the page at all")
	}
	return frag.BorderRect.W.Px()
}

func sheet600x800() Options { return Options{Page: PageSizePt(600, 800)} }

// TestAPageRuleSetsTheMargin is the whole feature in one document: a stylesheet
// that says how wide the margins are gets them, and the block that fills the
// page is narrower by exactly what it asked for.
//
// A block-level box with an auto width fills its containing block, and the
// containing block of the root is the content area of the sheet — so its width
// *is* the page's content width, measured rather than asserted about a field.
func TestAPageRuleSetsTheMargin(t *testing.T) {
	if got := pageWidthPx(t, "", sheet600x800()); got != 800 {
		t.Fatalf("without an @page rule the content is %gpx wide, want the whole 800px sheet", got)
	}
	// 800 - 2 * 96.
	if got := pageWidthPx(t, `@page { margin: 1in }`, sheet600x800()); got != 608 {
		t.Errorf("@page { margin: 1in } left %gpx of content, want 608", got)
	}
}

// TestAPageMarginSpreadsOverTheSidesTheShorthandOmits pins the one-to-four
// value form. Only the horizontal margins can be seen in a width, so each case
// is chosen to give the left and right sides different values.
func TestAPageMarginSpreadsOverTheSidesTheShorthandOmits(t *testing.T) {
	cases := []struct {
		css  string
		want float64
	}{
		// One value is every side: 800 - 2 * 96.
		{`@page { margin: 1in }`, 608},
		// Two are vertical then horizontal, so both sides are the second:
		// 800 - 2 * 192.
		{`@page { margin: 1in 2in }`, 416},
		// Three leave the left to be taken from the right: 800 - 2 * 192.
		{`@page { margin: 1in 2in 3in }`, 416},
		// Four are top, right, bottom, left: 800 - 192 - 384.
		{`@page { margin: 1in 2in 3in 4in }`, 224},
	}
	for _, tc := range cases {
		if got := pageWidthPx(t, tc.css, sheet600x800()); got != tc.want {
			t.Errorf("%s left %gpx of content, want %g", tc.css, got, tc.want)
		}
	}
}

// TestAPageMarginLonghandLeavesTheOtherSidesAlone is the difference between a
// side the document set and a side it never mentioned. The three it did not
// name keep the caller's margin, which is not the same as nothing: a stylesheet
// that says only "margin-left" has said nothing about the right, and printing
// that side at nought would take the document to the edge of the paper on the
// strength of a rule that never mentioned it.
func TestAPageMarginLonghandLeavesTheOtherSidesAlone(t *testing.T) {
	// 36pt is 48px, and the caller asked for it on all four sides.
	opts := Options{Page: PageSizePt(600, 800).WithMarginPt(36)}

	if got := pageWidthPx(t, "", opts); got != 704 {
		t.Fatalf("the caller's own margin left %gpx of content, want 800 - 2 * 48", got)
	}
	// The left becomes 96 and the right stays the caller's 48.
	if got := pageWidthPx(t, `@page { margin-left: 1in }`, opts); got != 656 {
		t.Errorf("@page { margin-left: 1in } left %gpx of content, want 656", got)
	}
	// A side the document sets to nothing is nothing — an author asking for a
	// bleed to the edge is not the same as an author saying nothing.
	if got := pageWidthPx(t, `@page { margin-left: 0 }`, opts); got != 752 {
		t.Errorf("@page { margin-left: 0 } left %gpx of content, want 800 - 48", got)
	}
}

// TestAPageMarginPercentageIsOfTheSheet. A percentage margin is of the page box
// — the whole sheet, not what is left of it — and of the dimension the side
// runs along: the left and right of its width, the top and bottom of its
// height. A percentage taken against the wrong axis would be wrong by the
// aspect ratio and invisible on a square page, so the sheet here is not square.
func TestAPageMarginPercentageIsOfTheSheet(t *testing.T) {
	// 10% of 800px is 80px a side; the page is 1066.67px tall, so a percentage
	// read against the height would leave 800 - 213.3 instead.
	if got := pageWidthPx(t, `@page { margin: 10% }`, sheet600x800()); got != 640 {
		t.Errorf("@page { margin: 10%% } left %gpx of content, want 640", got)
	}
}

// TestAPageMarginInEmIsTheInitialFontSize. There is no element here whose font
// an em could be relative to — the page is not styled by anything — so it is
// the initial size, exactly as a media query's em is. Reading the document's
// own font size instead would make the paper depend on the text printed on it.
func TestAPageMarginInEmIsTheInitialFontSize(t *testing.T) {
	const css = `html { font-size: 40px } @page { margin: 2em }`
	// 2em of the initial 16px is 32px a side. Of the document's 40px it would
	// be 80, leaving 640.
	if got := pageWidthPx(t, css, sheet600x800()); got != 736 {
		t.Errorf("@page { margin: 2em } left %gpx of content, want 800 - 2 * 32", got)
	}
}

// TestTheLastPageMarginDeclarationWins: two rules saying different things about
// the same side is the ordinary cascade question, and the answer is the
// ordinary one.
func TestTheLastPageMarginDeclarationWins(t *testing.T) {
	css := `@page { margin: 1in } @page { margin-left: 2in }`
	// The second sets only the left: 800 - 96 - 192.
	if got := pageWidthPx(t, css, sheet600x800()); got != 512 {
		t.Errorf("the later @page rule did not win: %gpx of content, want 512", got)
	}
}

// TestAnImportantPageMarginBeatsALaterOne pins that importance and origin
// decide this before order does, which is what makes a reader's stylesheet able
// to force a margin a document cannot take away. It is the same term the
// cascade uses on every other declaration, and it is the same code.
func TestAnImportantPageMarginBeatsALaterOne(t *testing.T) {
	got := Compose(Input{
		HTML:    `<div id="a">x</div>`,
		UserCSS: `@page { margin: 2in !important }`,
		CSS:     []Stylesheet{{Source: `body { margin: 0 } @page { margin: 1in }`}},
	}, sheet600x800())
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatal("the box is not on the page at all")
	}
	// The user's important 2in stands against the author's later 1in.
	if w := frag.BorderRect.W.Px(); w != 416 {
		t.Errorf("the author's later margin won over an important user one: %gpx, want 416", w)
	}
}

// TestAPageRuleIsNoLongerAnUnsupportedAtRule. The rule is applied, so reporting
// it as one this engine does not apply would be untrue — and the control is an
// at-rule that genuinely is not, so that "nothing was reported" cannot pass for
// "this one is implemented".
func TestAPageRuleIsNoLongerAnUnsupportedAtRule(t *testing.T) {
	built := Build(Input{HTML: `<style>@page { margin: 1cm }</style><p>x</p>`})
	for _, f := range built.Findings {
		if f.Rule == RuleUnsupportedAtRule {
			t.Errorf("@page is still reported as an at-rule that is not applied: %s", f.Message)
		}
	}

	other := Build(Input{HTML: `<style>@supports (display: grid) { p { color: red } }</style><p>x</p>`})
	found := false
	for _, f := range other.Findings {
		if f.Rule == RuleUnsupportedAtRule {
			found = true
		}
	}
	if !found {
		t.Error("no at-rule is reported at all any more, so this test proves nothing")
	}
}

// TestAPageRuleThatSelectsSomePagesIsReported. ":first", ":left" and a named
// page each pick pages out of a sequence, and this engine composes one page.
// Applying such a rule anyway would be a guess about which page this is;
// dropping it silently would print a document with the margins of a page it was
// never meant to have.
func TestAPageRuleThatSelectsSomePagesIsReported(t *testing.T) {
	for _, css := range []string{
		`@page :first { margin: 2in }`,
		`@page :left { margin: 2in }`,
		`@page narrow { margin: 2in }`,
	} {
		if got := pageWidthPx(t, css, sheet600x800()); got != 800 {
			t.Errorf("%s changed the page anyway: %gpx of content, want the untouched 800", css, got)
		}
		if !reportsPage(t, css, "selects some pages") {
			t.Errorf("%s was dropped with nothing said about it", css)
		}
	}
}

// TestAPageDescriptorThatIsNotAMarginIsReported. Everything else an @page block
// can hold changes the paper in a way this engine does not: "size" chooses it
// outright. Passing over one silently would print a page the author did not ask
// for with nothing saying so.
func TestAPageDescriptorThatIsNotAMarginIsReported(t *testing.T) {
	for _, css := range []string{
		`@page { size: A5 }`,
		`@page { marks: crop }`,
		`@page { background: red }`,
	} {
		if !reportsPage(t, css, "is not applied") {
			t.Errorf("%s was ignored with nothing said about it", css)
		}
	}
}

// TestAnUnreadablePageMarginKeepsTheOneItHad. A value this engine cannot read
// is not a reason to print to the edge of the sheet: the margin that was there
// stands, and the author is told which declaration was dropped.
//
// "auto" is in the list on purpose. On an element it means "let the layout work
// it out", and the calculation that would is the one centring a box in its
// containing block — there is nothing for the paper to be centred in, which is
// why CSS leaves an auto page margin to the printer. Reading it as nought would
// print to the edge.
func TestAnUnreadablePageMarginKeepsTheOneItHad(t *testing.T) {
	opts := Options{Page: PageSizePt(600, 800).WithMarginPt(36)}
	for _, css := range []string{
		`@page { margin: auto }`,
		`@page { margin: red }`,
		`@page { margin: 1in 2in 3in 4in 5in }`,
		`@page { margin-left: banana }`,
		`@page { margin: }`,
	} {
		if got := pageWidthPx(t, css, opts); got != 704 {
			t.Errorf("%s changed the page: %gpx of content, want the caller's 704", css, got)
		}
		if !reportsPage(t, css, "is not a margin this engine can read") {
			t.Errorf("%s was dropped with nothing said about it", css)
		}
	}
}

// TestAPageWithNoBlockIsReported. "@page" with nothing after it says nothing
// about the page, and an author who wrote one meant to write more.
func TestAPageWithNoBlockIsReported(t *testing.T) {
	if got := pageWidthPx(t, `@page`, sheet600x800()); got != 800 {
		t.Errorf("a blockless @page changed the page: %gpx of content, want 800", got)
	}
	if !reportsPage(t, `@page`, "says nothing about the page") {
		t.Error("a blockless @page was dropped with nothing said about it")
	}
}

// TestAPageRuleSaysWhereItCameFrom. A finding that cannot be traced back to the
// stylesheet it is about is one the author has to guess at, and an @page rule
// is written in a file the document may never name anywhere else.
func TestAPageRuleSaysWhereItCameFrom(t *testing.T) {
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		CSS:  []Stylesheet{{Name: "theme.css", Source: `@page { size: A5 }`}},
	}, sheet600x800())
	for _, f := range got.Findings {
		if strings.Contains(f.Message, "size") {
			if f.Source.Sheet != "theme.css" {
				t.Errorf("the finding points at %q, want theme.css", f.Source.Sheet)
			}
			return
		}
	}
	t.Error("the descriptor was not reported at all")
}

// reportsPage reports whether an @page rule raised a finding about itself,
// mentioning want.
func reportsPage(t *testing.T, css, want string) bool {
	t.Helper()
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		CSS:  []Stylesheet{{Name: "sheet.css", Source: css}},
	}, sheet600x800())
	for _, f := range got.Findings {
		if f.Source.Sheet != "sheet.css" {
			continue
		}
		if want == "" || strings.Contains(f.Message, want) {
			return true
		}
	}
	return false
}
