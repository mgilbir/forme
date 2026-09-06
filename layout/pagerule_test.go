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

// TestAPageRuleThatSelectsSomePagesIsReported. ":left", ":right", ":blank" and
// a named page each pick pages out of a sequence this engine does not have.
// Applying such a rule anyway would be a guess about which page this is;
// dropping it silently would print a document with the margins of a page it was
// never meant to have.
func TestAPageRuleThatSelectsSomePagesIsReported(t *testing.T) {
	for _, css := range []string{
		`@page :left { margin: 2in }`,
		`@page :right { margin: 2in }`,
		`@page :blank { margin: 2in }`,
		`@page narrow { margin: 2in }`,
		`@page :first:left { margin: 2in }`,
		`@page :fist { margin: 2in }`,
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
		`@page { marks: crop }`,
		`@page { bleed: 6pt }`,
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
		CSS:  []Stylesheet{{Name: "theme.css", Source: `@page { marks: crop }`}},
	}, sheet600x800())
	for _, f := range got.Findings {
		if strings.Contains(f.Message, "marks") {
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

// pageSheetPx is the sheet a document ended up being laid out on, in pixels,
// measured rather than read off a field: the block that fills the page is as
// wide as the content area, and the margin is nought in every case below.
func pageSheetPx(t *testing.T, css string, opts Options) float64 {
	t.Helper()
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		CSS: []Stylesheet{{Name: "sheet.css",
			Source: "body { margin: 0 } @page { margin: 0 }" + css}},
	}, opts)
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatalf("the box is not on the page at all")
	}
	return frag.BorderRect.W.Px()
}

// pageSheetHeightPx is the height of the sheet a document was laid out on. It
// is read from the scale rather than from a field: a box of a known height that
// does not fit is shrunk by exactly the ratio of the two, which is §5's factor.
func pageSheetHeightPx(t *testing.T, css string, opts Options) float64 {
	t.Helper()
	const boxPx = 960 // 10in.
	got := Compose(Input{
		HTML: `<div id="a" style="height: 10in"></div>`,
		CSS: []Stylesheet{{Name: "sheet.css",
			Source: "body { margin: 0 } @page { margin: 0 }" + css}},
	}, Options{Page: opts.Page, MinScale: 0.01})
	if got.Scale == 0 {
		t.Fatal("the document was scaled to nothing")
	}
	if got.Scale == 1 {
		t.Fatalf("the box fitted, so the height of the sheet cannot be read from the scale")
	}
	return boxPx * got.Scale
}

// TestAPageSizeChoosesThePaper is the descriptor doing its job: a document that
// says what it is to be printed on is printed on it, whatever the caller passed
// in Options.
func TestAPageSizeChoosesThePaper(t *testing.T) {
	// 10in by 5in is 960 by 480px.
	w := pageSheetPx(t, `@page { size: 10in 5in }`, sheet600x800())
	if w != 960 {
		t.Errorf("@page { size: 10in 5in } laid out on a %gpx sheet, want 960", w)
	}
	// One length is a square page, which is the only way to ask for one — so
	// the height has to be measured too, or a size that took the second
	// dimension from somewhere else would look right across the page.
	w = pageSheetPx(t, `@page { size: 7in }`, sheet600x800())
	if w != 672 {
		t.Errorf("@page { size: 7in } laid out on a %gpx sheet, want 672", w)
	}
	if h := pageSheetHeightPx(t, `@page { size: 7in }`, sheet600x800()); h != 672 {
		t.Errorf("@page { size: 7in } is %gpx tall, want the square 672", h)
	}
	// "auto" is the caller's sheet, which is what it means and not nothing.
	w = pageSheetPx(t, `@page { size: auto }`, sheet600x800())
	if w != 800 {
		t.Errorf("@page { size: auto } laid out on a %gpx sheet, want the caller's 800", w)
	}
}

// TestAPageSizeCanNameThePaper pins the sizes of §5.1 against their real
// dimensions. A4 is 210mm wide and letter is 8.5in, and a table that had them
// the other way round would still produce a page.
func TestAPageSizeCanNameThePaper(t *testing.T) {
	cases := []struct {
		name string
		want float64 // the width in px: 210mm is 210 / 25.4 * 96.
	}{
		{"A5", 148.0 / 25.4 * 96},
		{"A4", 210.0 / 25.4 * 96},
		{"A3", 297.0 / 25.4 * 96},
		{"B5", 176.0 / 25.4 * 96},
		{"B4", 250.0 / 25.4 * 96},
		{"JIS-B5", 182.0 / 25.4 * 96},
		{"JIS-B4", 257.0 / 25.4 * 96},
		{"letter", 8.5 * 96},
		{"legal", 8.5 * 96},
		{"ledger", 11 * 96},
	}
	for _, tc := range cases {
		w := pageSheetPx(t, `@page { size: `+tc.name+` }`, sheet600x800())
		// Within a hundredth of a pixel: the sheet is stored in layout units
		// and a millimetre is not a whole number of them.
		if diff := w - tc.want; diff > 0.01 || diff < -0.01 {
			t.Errorf("size: %s laid out on a %gpx sheet, want %g", tc.name, w, tc.want)
		}
	}
	// The JIS B series is a different paper of the same name, which is why the
	// specification lists both — a table that aliased one to the other would
	// pass every case above on its own.
	iso := pageSheetPx(t, `@page { size: B5 }`, sheet600x800())
	jis := pageSheetPx(t, `@page { size: JIS-B5 }`, sheet600x800())
	if iso == jis {
		t.Errorf("B5 and JIS-B5 are the same %gpx sheet; they are different paper", iso)
	}
}

// TestAPageSizeTurnsTheSheet. "landscape" is a turn and not a size: it swaps a
// named sheet that is taller than it is wide, and asking for the orientation a
// sheet already has changes nothing.
func TestAPageSizeTurnsTheSheet(t *testing.T) {
	portrait := pageSheetPx(t, `@page { size: A4 }`, sheet600x800())
	landscape := pageSheetPx(t, `@page { size: A4 landscape }`, sheet600x800())
	want := 297.0 / 25.4 * 96
	if diff := landscape - want; diff > 0.01 || diff < -0.01 {
		t.Errorf("A4 landscape is %gpx wide, want its long edge of %g", landscape, want)
	}
	if landscape <= portrait {
		t.Errorf("A4 landscape (%g) is no wider than A4 (%g)", landscape, portrait)
	}
	// Either order, because §5.1 joins the two terms with "||".
	other := pageSheetPx(t, `@page { size: landscape A4 }`, sheet600x800())
	if other != landscape {
		t.Errorf("landscape A4 is %gpx wide and A4 landscape is %g", other, landscape)
	}
	// Asking for the orientation it already has is not a swap.
	if got := pageSheetPx(t, `@page { size: A4 portrait }`, sheet600x800()); got != portrait {
		t.Errorf("A4 portrait is %gpx wide and A4 is %g", got, portrait)
	}
	// With no size of its own it turns the caller's sheet, which is 800 by
	// 1066.67px and so becomes 1066.67 wide.
	turned := pageSheetPx(t, `@page { size: landscape }`, sheet600x800())
	if diff := turned - 1066.666; diff > 0.01 || diff < -0.01 {
		t.Errorf("size: landscape gave a %gpx sheet, want the caller's long edge", turned)
	}
}

// TestAPageMarginIsAPercentageOfTheSizeTheSameRuleChose. The two descriptors
// are one statement about one page: "size: A5; margin: 10%" is a tenth of the
// A5 it just asked for, not a tenth of the sheet the caller happened to pass.
// Reading them in the order they were written would make the second depend on
// which came first.
func TestAPageMarginIsAPercentageOfTheSizeTheSameRuleChose(t *testing.T) {
	got := Compose(Input{
		HTML: `<div id="a">x</div>`,
		CSS: []Stylesheet{{Source: "body { margin: 0 }" +
			`@page { margin: 10%; size: 10in 5in }`}},
	}, sheet600x800())
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatal("the box is not on the page at all")
	}
	// 10% of the 960px sheet is 96px a side: 960 - 192.
	if w := frag.BorderRect.W.Px(); w != 768 {
		t.Errorf("the margin came out %gpx wide, want 768 — a tenth of the size the rule chose", w)
	}
}

// TestAMediaQueryIsAnsweredAboutTheSheetTheDocumentChose. A query asks about
// the paper, and after this the paper may be what the document asked for — so
// the size is settled before the cascade runs rather than after it. A query
// answered about the caller's sheet and then printed on another is a document
// styled for a page it is not on.
func TestAMediaQueryIsAnsweredAboutTheSheetTheDocumentChose(t *testing.T) {
	const doc = `<p id="a">a</p>`
	// The caller's sheet is 800px wide, so this query is false on it. The
	// document then asks for a 960px sheet, on which it is true.
	css := `@page { size: 10in 5in } @media (min-width: 900px) { #a { display: none } }`

	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}}, sheet600x800())
	if fragmentFor(got.Root, "a") != nil {
		t.Error("the query was answered about the caller's sheet, not the one the document chose")
	}
}

// TestAnUnreadablePageSizeKeepsTheSheetItHad. A size this engine cannot read is
// not a reason to print on a page of nothing, and the combinations refused here
// are the ones §5.1's grammar does not allow: two named sizes, a length with a
// keyword, two orientations, a percentage of a page that is what percentages
// are of, and a sheet with no extent.
func TestAnUnreadablePageSizeKeepsTheSheetItHad(t *testing.T) {
	for _, css := range []string{
		`@page { size: A4 A5 }`,
		`@page { size: 10in landscape }`,
		`@page { size: landscape portrait }`,
		`@page { size: auto A4 }`,
		`@page { size: 50% }`,
		`@page { size: 0in }`,
		`@page { size: -3in 4in }`,
		`@page { size: 1in 2in 3in }`,
		`@page { size: quarto }`,
		`@page { size: }`,
	} {
		if w := pageSheetPx(t, css, sheet600x800()); w != 800 {
			t.Errorf("%s changed the sheet to %gpx, want the caller's 800", css, w)
		}
		if !reportsPage(t, css, "is not a sheet this engine can read") {
			t.Errorf("%s was dropped with nothing said about it", css)
		}
	}
}

// TestTheLastPageSizeWins, and an important one wins before that — the size is
// decided by the same term as the margins, which is the same term the cascade
// decides everything by.
func TestTheLastPageSizeWins(t *testing.T) {
	w := pageSheetPx(t, `@page { size: 10in 5in } @page { size: 6in 5in }`, sheet600x800())
	if w != 576 {
		t.Errorf("the later size did not win: %gpx, want 576", w)
	}

	got := Compose(Input{
		HTML:    `<div id="a">x</div>`,
		UserCSS: `@page { size: 10in 5in !important; margin: 0 }`,
		CSS:     []Stylesheet{{Source: `body { margin: 0 } @page { size: 6in 5in; margin: 0 }`}},
	}, sheet600x800())
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatal("the box is not on the page at all")
	}
	if w := frag.BorderRect.W.Px(); w != 960 {
		t.Errorf("the author's later size won over an important user one: %gpx, want 960", w)
	}
}

// TestAPageSizeSetsTheHeightAsWellAsTheWidth. Every case above is measured
// across the page, and a size that took only the width would pass all of them.
// The height shows in the scale: a document taller than the sheet is shrunk to
// fit, by exactly the ratio §5 computes.
func TestAPageSizeSetsTheHeightAsWellAsTheWidth(t *testing.T) {
	const doc = `<div id="a" style="height: 10in"></div>`
	sheet := func(css string) float64 {
		return Compose(Input{HTML: doc, CSS: []Stylesheet{{
			Source: "body { margin: 0 } @page { margin: 0 }" + css}}}, sheet600x800()).Scale
	}
	// The caller's sheet is 1066.67px tall and the box is 960, so it fits.
	if got := sheet(""); got != 1 {
		t.Fatalf("the box already had to be scaled by %g on the caller's sheet", got)
	}
	// A 5in page is 480px tall, and 480 / 960 is a half.
	if got := sheet(`@page { size: 10in 5in }`); got != 0.5 {
		t.Errorf("on a 5in-tall page the document was scaled by %g, want 0.5", got)
	}
}

// TestAPageRuleInsideAMediaQueryIsRead. This is where a print stylesheet
// actually puts one: a document that is also read on a screen writes
// "@media print { @page { margin: 0 } }", and reading only the top-level rules
// would miss most of the @page rules that exist.
func TestAPageRuleInsideAMediaQueryIsRead(t *testing.T) {
	// The medium is paper, so this applies: 800 - 2 * 96.
	if got := pageWidthPx(t, `@media print { @page { margin: 1in } }`, sheet600x800()); got != 608 {
		t.Errorf("an @page inside @media print left %gpx of content, want 608", got)
	}
	// And this does not, because the page is not a screen.
	if got := pageWidthPx(t, `@media screen { @page { margin: 1in } }`, sheet600x800()); got != 800 {
		t.Errorf("an @page inside @media screen was applied to paper: %gpx of content", got)
	}
}

// TestNestedMediaQueriesAroundAPageRuleAllHaveToMatch. Nesting is "and", and a
// rule reached through a query that does not match is not reached at all.
func TestNestedMediaQueriesAroundAPageRuleAllHaveToMatch(t *testing.T) {
	both := `@media print { @media (min-width: 400px) { @page { margin: 1in } } }`
	if got := pageWidthPx(t, both, sheet600x800()); got != 608 {
		t.Errorf("both queries matched but the rule was dropped: %gpx of content", got)
	}
	inner := `@media print { @media (min-width: 900px) { @page { margin: 1in } } }`
	if got := pageWidthPx(t, inner, sheet600x800()); got != 800 {
		t.Errorf("the inner query did not match and the rule was applied anyway: %gpx", got)
	}
	// The outer one counts too, and it is the one a reader who only checked the
	// nearest query would lose.
	outer := `@media (min-width: 900px) { @media print { @page { margin: 1in } } }`
	if got := pageWidthPx(t, outer, sheet600x800()); got != 800 {
		t.Errorf("the outer query did not match and the rule was applied anyway: %gpx", got)
	}
	// Two blocks at the same depth, each holding a rule: the second must not
	// take the first's query with it. This is the shape that breaks when the
	// chain is grown by appending into shared capacity, and it needs four
	// levels before the spare capacity exists to be shared.
	deep := `@media print { @media print { @media print {
		@media (min-width: 400px) { @page { margin-left: 1in } }
		@media (min-width: 900px) { @page { margin-right: 2in } }
	} } }`
	// Only the first applies: 800 - 96.
	if got := pageWidthPx(t, deep, sheet600x800()); got != 704 {
		t.Errorf("nested sibling queries came out at %gpx of content, want 704", got)
	}
}

// TestAPageRuleInAQueryThisEngineCannotAnswerIsReported. An unknown feature is
// false, so the rule is dropped — but a browser printing the same document may
// know the feature, and its page would differ from this one. That is worth a
// finding whichever way the query then went.
func TestAPageRuleInAQueryThisEngineCannotAnswerIsReported(t *testing.T) {
	css := `@media (prefers-color-scheme: dark) { @page { margin: 1in } }`
	if got := pageWidthPx(t, css, sheet600x800()); got != 800 {
		t.Errorf("a query this engine cannot answer was treated as matching: %gpx", got)
	}
	if !reportsPage(t, css, "which this engine cannot answer") {
		t.Error("the unanswerable query around an @page rule was not reported")
	}
}

// TestAQueryAroundAPageRuleIsAnsweredAboutTheSheetItWasGiven. The rule inside
// may change the sheet, so the query outside it cannot be answered about the
// sheet it produces — that is a circle. It is answered about the page the
// caller asked for, which is the only order that terminates, and the rest of
// the document is then styled against whatever the rule chose.
func TestAQueryAroundAPageRuleIsAnsweredAboutTheSheetItWasGiven(t *testing.T) {
	// The caller's sheet is 800px wide, so this matches and the page becomes
	// 1440px wide — on which the query would have been true anyway.
	wide := `@media (min-width: 400px) { @page { size: 15in 5in } }`
	if got := pageSheetPx(t, wide, sheet600x800()); got != 1440 {
		t.Errorf("the query was not answered about the caller's 800px sheet: %gpx", got)
	}
	// Here the query is false on the caller's sheet, and the size it would
	// have chosen is one on which it would have been true. It stays false.
	narrow := `@media (min-width: 1000px) { @page { size: 15in 5in } }`
	if got := pageSheetPx(t, narrow, sheet600x800()); got != 800 {
		t.Errorf("the query was answered about the sheet its own rule would have chosen: %gpx", got)
	}
}

// TestAPageRuleSomewhereElseIsNotReadAsOne. Only @media is descended into. An
// @page written inside anything else is not a page rule this engine has a way
// to decide, and the at-rule holding it is reported by the cascade as one it
// does not apply.
func TestAPageRuleSomewhereElseIsNotReadAsOne(t *testing.T) {
	for _, css := range []string{
		`@supports (display: grid) { @page { margin: 1in } }`,
		`div { @page { margin: 1in } }`,
		// The prelude of an at-rule that is not @media is not a media query,
		// and reading one as though it were would apply a rule that was never
		// in a query at all — this one reads as the media type "print", which
		// is the paper the document is on.
		`@layer print { @page { margin: 1in } }`,
	} {
		if got := pageWidthPx(t, css, sheet600x800()); got != 800 {
			t.Errorf("%s was read as a page rule: %gpx of content, want 800", css, got)
		}
	}
}

// TestTheOnePageIsTheFirstPage. "@page :first { margin-top: 0 }" is how a title
// page is written, and it is the commonest pseudo-page there is. A document is
// one page in this engine and that page is the first one, so the rule applies.
func TestTheOnePageIsTheFirstPage(t *testing.T) {
	if got := pageWidthPx(t, `@page :first { margin: 1in }`, sheet600x800()); got != 608 {
		t.Errorf("@page :first left %gpx of content, want 608", got)
	}
	// The spelling is a colon and the word, in either case, and nothing else.
	if got := pageWidthPx(t, `@page :FIRST { margin: 1in }`, sheet600x800()); got != 608 {
		t.Errorf("@page :FIRST left %gpx of content, want 608", got)
	}
}

// TestAFirstPageRuleBeatsAPlainOneWhicheverWasWrittenFirst. CSS 2.1 §13.2.4
// orders the page selectors, and a rule with a pseudo-page is the more
// particular one — so this is a real cascade term and not a matter of where the
// rules happen to sit. An engine deciding it by order alone gets the right
// answer for the stylesheet that puts :first last and the wrong one for the
// stylesheet that does not.
func TestAFirstPageRuleBeatsAPlainOneWhicheverWasWrittenFirst(t *testing.T) {
	after := `@page { margin: 1in } @page :first { margin: 2in }`
	if got := pageWidthPx(t, after, sheet600x800()); got != 416 {
		t.Errorf("with :first written last the content is %gpx, want 416", got)
	}
	before := `@page :first { margin: 2in } @page { margin: 1in }`
	if got := pageWidthPx(t, before, sheet600x800()); got != 416 {
		t.Errorf("with :first written first the content is %gpx, want 416", got)
	}
	// It is the *third* term, though: an important declaration in a user
	// stylesheet still beats a more particular author rule, because importance
	// and origin are settled before specificity is looked at.
	got := Compose(Input{
		HTML:    `<div id="a">x</div>`,
		UserCSS: `@page { margin: 3in !important }`,
		CSS:     []Stylesheet{{Source: `body { margin: 0 } @page :first { margin: 2in }`}},
	}, sheet600x800())
	frag := fragmentFor(got.Root, "a")
	if frag == nil {
		t.Fatal("the box is not on the page at all")
	}
	if w := frag.BorderRect.W.Px(); w != 224 {
		t.Errorf(":first beat an important user rule: %gpx, want 224", w)
	}
}

// TestAFirstPageRuleDecidesTheSizeToo. The size is a declaration in the same
// rule and nothing about it is a different kind of question, so it is decided
// by the same three terms.
func TestAFirstPageRuleDecidesTheSizeToo(t *testing.T) {
	css := `@page :first { size: 10in 5in } @page { size: 6in 5in }`
	if got := pageSheetPx(t, css, sheet600x800()); got != 960 {
		t.Errorf("the plain rule's size won over the :first rule's: %gpx, want 960", got)
	}
}
