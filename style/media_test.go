package style

import (
	"strings"
	"testing"
)

// Media Queries 4, for a medium that is a sheet of paper.
//
// Every test here asks the same question — which rules reached the cascade —
// because that is the only thing an @media block decides. It adds no
// specificity and no priority: the rules inside it are ordinary rules that
// either arrive or do not.

// underQuery is the colour a paragraph ends up with when a query wraps a rule
// that would make it red.
func underQuery(t *testing.T, query string, media Media) string {
	t.Helper()
	doc := parseDoc(t, "<p id='p'>x</p>")
	src := "#p { color: blue } @media " + query + " { #p { color: red } }"
	got := ApplyIn(doc, []Sheet{author(t, src)}, nil, media)
	n := elementFor(t, doc, "#p")
	return got.Styles[n]["color"]
}

var a4ish = Media{Width: unitOf(794), Height: unitOf(1123)}

func unitOf(px float64) Unit {
	v, _ := FromPx(px)
	return v
}

// TestTheMediumIsPaper. A media type is not a question about the page but about
// the engine, and this one renders for print: "print" and "all" match, and
// every other medium CSS has ever named is a device this is not.
func TestTheMediumIsPaper(t *testing.T) {
	for _, c := range []struct {
		query string
		want  string
	}{
		{"print", "red"},
		{"all", "red"},
		{"screen", "blue"},
		{"speech", "blue"},
		{"tty", "blue"},
		// "not" negates the whole query, and "only" is the keyword that hid a
		// query from the parsers of the 1990s: it means nothing to anything
		// written since.
		{"not print", "blue"},
		{"not screen", "red"},
		{"only print", "red"},
		{"only screen", "blue"},
		// A comma is "or": the list matches when any query in it does.
		{"screen, print", "red"},
		{"screen, tty", "blue"},
	} {
		if got := underQuery(t, c.query, a4ish); got != c.want {
			t.Errorf("@media %s left the paragraph %s, want %s", c.query, got, c.want)
		}
	}
}

// TestASheetAnswersAboutItsOwnSize. The two features a page can answer are its
// width and its height, and they are the page box — the paper — rather than the
// area inside its margins.
func TestASheetAnswersAboutItsOwnSize(t *testing.T) {
	for _, c := range []struct {
		query string
		want  string
	}{
		{"(min-width: 700px)", "red"},
		{"(min-width: 900px)", "blue"},
		{"(max-width: 900px)", "red"},
		{"(max-width: 700px)", "blue"},
		{"(min-height: 1000px)", "red"},
		{"(max-height: 1000px)", "blue"},
		// A length in any absolute unit, which is how a stylesheet for paper
		// writes one.
		{"(min-width: 5in)", "red"},
		{"(min-width: 20in)", "blue"},
		// The page is taller than it is wide, so it is portrait.
		{"(orientation: portrait)", "red"},
		{"(orientation: landscape)", "blue"},
		// "and" joins them, and every part has to hold.
		{"print and (min-width: 700px)", "red"},
		{"print and (min-width: 900px)", "blue"},
		{"screen and (min-width: 700px)", "blue"},
	} {
		if got := underQuery(t, c.query, a4ish); got != c.want {
			t.Errorf("@media %s left the paragraph %s, want %s", c.query, got, c.want)
		}
	}

	// An em in a query is the *initial* font size and never the document's:
	// §1.3 says so, and the reason is that a query is evaluated before any
	// element has a style — so there is no element whose font an em could be
	// relative to. Sixty of them is 960px, which this sheet is not.
	if got := underQuery(t, "(min-width: 60em)", a4ish); got != "blue" {
		t.Errorf("a 794px sheet matched (min-width: 60em), which is 960px")
	}
	if got := underQuery(t, "(min-width: 40em)", a4ish); got != "red" {
		t.Errorf("a 794px sheet did not match (min-width: 40em), which is 640px")
	}

	// A sheet as wide as it is tall is portrait: §4's orientation is portrait
	// unless the width is *greater*, so the square case belongs to it.
	square := Media{Width: unitOf(800), Height: unitOf(800)}
	if got := underQuery(t, "(orientation: portrait)", square); got != "red" {
		t.Errorf("a square sheet was not portrait")
	}
	if got := underQuery(t, "(orientation: landscape)", square); got != "blue" {
		t.Errorf("a square sheet was landscape")
	}

	// The same queries against a wider, shorter sheet: the answers are the
	// page's and not the engine's.
	landscape := Media{Width: unitOf(1123), Height: unitOf(794)}
	if got := underQuery(t, "(orientation: landscape)", landscape); got != "red" {
		t.Errorf("a sheet wider than it is tall answered portrait")
	}
	if got := underQuery(t, "(min-width: 900px)", landscape); got != "red" {
		t.Errorf("a 1123px sheet said it was narrower than 900px")
	}
}

// TestAQueryAskedAboutNothingIsAskedAboutNothing. A caller that did not say
// what it was printing on gets a sheet of no size, and a query about a width is
// false against it — while the media type is answered either way, because that
// one is a fact about the engine.
func TestAQueryAskedAboutNothingIsAskedAboutNothing(t *testing.T) {
	if got := underQuery(t, "print", Media{}); got != "red" {
		t.Errorf("@media print did not match a caller that named no sheet")
	}
	if got := underQuery(t, "(min-width: 1px)", Media{}); got != "blue" {
		t.Errorf("@media (min-width: 1px) matched a sheet of no size")
	}
}

// TestAQueryThisEngineCannotAnswerIsReported. Media Queries 4 makes an unknown
// feature false, so the rules inside are dropped either way — but a browser
// printing the same document may know what the feature is, and its page would
// then differ from this one. That is what the finding is for.
func TestAQueryThisEngineCannotAnswerIsReported(t *testing.T) {
	for _, query := range []string{
		"(prefers-color-scheme: dark)",
		"(hover: hover)",
		"(min-resolution: 2dppx)",
		"print and (color)",
		// A syntax beyond the "and"-joined list this reads.
		"(min-width: 100px) or (min-height: 100px)",
	} {
		doc := parseDoc(t, "<p id='p'>x</p>")
		src := "@media " + query + " { #p { color: red } }"
		got := ApplyIn(doc, []Sheet{author(t, src)}, nil, a4ish)
		n := elementFor(t, doc, "#p")
		if c := got.Styles[n]["color"]; c == "red" {
			t.Errorf("@media %s applied its rules", query)
		}
		said := false
		for _, f := range got.Findings {
			if f.Property == "@media" && f.Unsupported {
				said = true
				if !strings.Contains(f.Message, "not applied") {
					t.Errorf("the finding for %s is %q and does not say what "+
						"happened to the rules", query, f.Message)
				}
			}
		}
		if !said {
			t.Errorf("@media %s dropped its rules and said nothing: %v", query,
				got.Findings)
		}
	}
}

// TestAQueryThatMatchesReportsNothing is the containment argument: a finding on
// every media query would be a report about every stylesheet written for paper.
func TestAQueryThatMatchesReportsNothing(t *testing.T) {
	doc := parseDoc(t, "<p id='p'>x</p>")
	for _, query := range []string{"print", "all", "screen", "not screen",
		"(min-width: 100px)", "print and (orientation: portrait)"} {
		got := ApplyIn(doc, []Sheet{author(t, "@media "+query+" { p { color: red } }")},
			nil, a4ish)
		for _, f := range got.Findings {
			if f.Property == "@media" {
				t.Errorf("@media %s reported %q", query, f.Message)
			}
		}
	}
}

// TestTheRulesInsideAreOrderedWhereTheBlockIs. §3: an @media block adds no
// specificity and no priority, so a rule inside it beats one before it and
// loses to one after it — exactly as though the block were not there.
func TestTheRulesInsideAreOrderedWhereTheBlockIs(t *testing.T) {
	doc := parseDoc(t, "<p id='p'>x</p>")
	n := elementFor(t, doc, "#p")

	after := ApplyIn(doc, []Sheet{author(t,
		"#p { color: blue } @media print { #p { color: red } }")}, nil, a4ish)
	if got := after.Styles[n]["color"]; got != "red" {
		t.Errorf("a rule inside a later @media lost to one before it: %s", got)
	}

	before := ApplyIn(doc, []Sheet{author(t,
		"@media print { #p { color: red } } #p { color: blue }")}, nil, a4ish)
	if got := before.Styles[n]["color"]; got != "blue" {
		t.Errorf("a rule inside an earlier @media beat one after it: %s", got)
	}

	// And specificity is untouched: a less specific rule inside a query still
	// loses to a more specific one outside it.
	weaker := ApplyIn(doc, []Sheet{author(t,
		"#p { color: blue } @media print { p { color: red } }")}, nil, a4ish)
	if got := weaker.Styles[n]["color"]; got != "blue" {
		t.Errorf("a rule inside @media won on the block rather than on its "+
			"selector: %s", got)
	}
}

// TestAQueryInsideAQuery. Both have to match, and nesting is how a stylesheet
// says "on paper, and only if there is room".
func TestAQueryInsideAQuery(t *testing.T) {
	doc := parseDoc(t, "<p id='p'>x</p>")
	n := elementFor(t, doc, "#p")
	for _, c := range []struct{ src, want string }{
		{"@media print { @media (min-width: 100px) { #p { color: red } } }", "red"},
		{"@media print { @media (min-width: 9000px) { #p { color: red } } }", "blue"},
		{"@media screen { @media print { #p { color: red } } }", "blue"},
	} {
		got := ApplyIn(doc, []Sheet{author(t, "#p { color: blue } "+c.src)}, nil, a4ish)
		if colour := got.Styles[n]["color"]; colour != c.want {
			t.Errorf("%s left the paragraph %s, want %s", c.src, colour, c.want)
		}
	}
}
