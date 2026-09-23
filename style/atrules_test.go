package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// The walk that prepares a stylesheet is the one that finds its @page and
// @font-face rules, so the two cannot disagree about which blocks are live.

func prepareSheet(t *testing.T, src string, media Media) *Prepared {
	t.Helper()
	rules, errs := css.ParseStylesheet(src)
	if len(errs) != 0 {
		t.Fatalf("the fixture did not parse: %v", errs)
	}
	return Prepare([]Sheet{{Origin: OriginAuthor, Rules: rules, Name: "s.css"}}, media)
}

// TestTheWalkHandsOverTheAtRulesItReaches is audit C33 and C138 from this side:
// an @page or @font-face is found wherever the cascade would apply a rule, with
// the layer it was written in, and nowhere else.
func TestTheWalkHandsOverTheAtRulesItReaches(t *testing.T) {
	a4 := Media{Width: mustUnit(794), Height: mustUnit(1123)}
	p := prepareSheet(t, `
		@page { margin: 1px }
		@media print { @page :first { margin: 2px } @font-face { font-family: a } }
		@media screen { @page { margin: 3px } @font-face { font-family: b } }
		@supports (display: block) { @page { margin: 4px } @font-face { font-family: c } }
		@supports (display: nonesuch) { @page { margin: 5px } }
		@layer x { @page { margin: 6px } @font-face { font-family: d } }
		@media (min-width: 1000px) { @page { margin: 7px } }
		p { @page { margin: 8px } @font-face { font-family: e } }
	`, a4)
	var pages []string
	for _, r := range p.Pages {
		pages = append(pages, strings.TrimSpace(serialize(r.Rule.Block)))
		if r.Sheet != "s.css" || r.Origin != OriginAuthor {
			t.Errorf("%v was handed over from sheet %q, origin %d", r.Rule, r.Sheet, r.Origin)
		}
	}
	if got, want := strings.Join(pages, " | "),
		"margin: 1px | margin: 2px | margin: 4px | margin: 6px"; got != want {
		t.Errorf("the pages handed over were %q, want %q", got, want)
	}
	var faces []string
	for _, r := range p.FontFaces {
		faces = append(faces, strings.TrimSpace(serialize(r.Rule.Block)))
	}
	if got, want := strings.Join(faces, " | "),
		"font-family: a | font-family: c | font-family: d"; got != want {
		t.Errorf("the faces handed over were %q, want %q", got, want)
	}
	// The layered ones carry their layer, and the rest none.
	if p.Pages[3].Layer == 0 || p.Pages[0].Layer != 0 {
		t.Errorf("layers %d and %d, want the fourth layered and the first not",
			p.Pages[3].Layer, p.Pages[0].Layer)
	}
	// And the two written inside a style rule are reported, not silent.
	n := 0
	for _, f := range p.findings {
		if strings.Contains(f.Message, "cannot be written inside a style rule") {
			n++
		}
	}
	if n != 2 {
		t.Errorf("%d nested at-rules were reported, want 2: %v", n, p.findings)
	}
}

// TestAPreparedSheetStylesMoreThanOneDocument: Apply changes nothing it was
// given, so a preparation used twice styles the second document as it did the
// first.
func TestAPreparedSheetStylesMoreThanOneDocument(t *testing.T) {
	p := prepareSheet(t, `p { font-family: wins } p { color: 'x' }`, Media{})
	for i := 0; i < 2; i++ {
		doc := parseDoc(t, `<p id="p">x</p>`)
		got := p.Apply(doc, nil)
		if v := got.Styles[elementFor(t, doc, "#p")].Get("font-family"); v != "wins" {
			t.Errorf("document %d: font-family %q", i, v)
		}
		if len(got.Findings) != 1 {
			t.Errorf("document %d got %d findings, want the one", i, len(got.Findings))
		}
	}
}

// TestTheUserAgentMemoKnowsTheMediumAndTheLength is audit C156: the one-slot
// memo of a user agent sheet's preparation was keyed on the first rule alone,
// so the same sheet prepared for a narrow page and then a wide one reused the
// narrow preparation, and rules[:k] of the same array reused the whole one.
func TestTheUserAgentMemoKnowsTheMediumAndTheLength(t *testing.T) {
	rules, errs := css.ParseStylesheet(
		`@media (min-width: 100px) { p { font-family: wide } } p { color: green }`)
	if len(errs) != 0 {
		t.Fatal(errs)
	}
	ua := Sheet{Origin: OriginUserAgent, Rules: rules}
	family := func(media Media, sheet Sheet) string {
		doc := parseDoc(t, `<p id="p">x</p>`)
		return ApplyIn(doc, []Sheet{sheet}, nil, media).
			Styles[elementFor(t, doc, "#p")].Get("font-family")
	}
	narrow, wide := Media{Width: mustUnit(50)}, Media{Width: mustUnit(500)}
	if got := family(narrow, ua); got != "serif" {
		t.Fatalf("at 50px the family is %q, want serif", got)
	}
	if got := family(wide, ua); got != "wide" {
		t.Errorf("at 500px the family is %q: the narrow preparation was reused", got)
	}
	// The same first rule, fewer of them: the second rule, which sets the
	// colour, is not in this sheet, and a preparation of the whole of it must
	// not be reused for it.
	doc := parseDoc(t, `<p id="p">x</p>`)
	got := ApplyIn(doc, []Sheet{{Origin: OriginUserAgent, Rules: rules[:1]}}, nil, wide).
		Styles[elementFor(t, doc, "#p")].Get("color")
	if got != "black" {
		t.Errorf("rules[:1] of the same array computed color %q, want black: the "+
			"whole sheet's preparation was reused", got)
	}
}
