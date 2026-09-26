package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// XML names are exact, and HTML's are folded and then given their SVG case
// back: see svgNames in svg.go and html/foreignnames.go.

// TestAnSVGFileSpellsItsNames. An SVG file is XML, so "RECT" is not a rect and
// "VIEWBOX" is not a viewBox. Both were folded and read as SVG's. What the
// engine does with a name it does not know is refuse the picture, which is
// its answer for any element or attribute it has not classified.
func TestAnSVGFileSpellsItsNames(t *testing.T) {
	const ns = `xmlns="http://www.w3.org/2000/svg"`
	for _, tc := range []struct {
		body string
		ok   bool
	}{
		{`<svg ` + ns + ` width="20" height="10"><rect width="100%" height="100%" fill="blue"/></svg>`, true},
		{`<svg ` + ns + ` viewBox="0 0 2 1"><rect width="100%" height="100%" fill="blue"/></svg>`, true},
		{`<svg ` + ns + ` width="20" height="10"><RECT width="100%" height="100%" fill="blue"/></svg>`, false},
		{`<svg ` + ns + ` width="20" height="10"><rect WIDTH="100%" height="100%" fill="blue"/></svg>`, false},
		{`<svg ` + ns + ` VIEWBOX="0 0 2 1"><rect width="100%" height="100%" fill="blue"/></svg>`, false},
		{`<svg ` + ns + ` viewbox="0 0 2 1"><rect width="100%" height="100%" fill="blue"/></svg>`, false},
		{`<SVG ` + ns + ` width="20" height="10"><rect width="100%" height="100%" fill="blue"/></SVG>`, false},
	} {
		if got := svgOf(t, tc.body); (got != nil) != tc.ok {
			t.Errorf("%s: read = %v, want %v", tc.body, got != nil, tc.ok)
		}
	}
}

// TestAnInlineSVGInHTMLHasHTMLsNames. The same markup inside an HTML document
// goes through HTML's parser, which folds the names and gives SVG's own back
// by table, so every spelling of rect is a rect and every spelling of viewBox
// a viewBox, and the picture is drawn at the size they give it.
func TestAnInlineSVGInHTMLHasHTMLsNames(t *testing.T) {
	for _, markup := range []string{
		`<svg height="60" viewBox="0 0 200 100"><rect width="100%" height="100%" fill="blue"/></svg>`,
		`<svg height="60" VIEWBOX="0 0 200 100"><RECT WIDTH="100%" Height="100%" FILL="blue"/></svg>`,
		`<SVG HEIGHT="60" viewbox="0 0 200 100"><Rect width="100%" height="100%" fill="blue"></Rect></SVG>`,
	} {
		got := svgFills(t, markup, `#d { font-size: 0 }`, blue)
		if len(got) != 1 || got[0].W != bgpx(120) || got[0].H != bgpx(60) {
			t.Errorf("%s: fills %v, want one 120 by 60", markup, got)
		}
	}
}

// TestAnInlineSVGInXHTMLSpellsItsNames. In an XHTML document the <svg> is XML
// and nothing is folded: a RECT is not drawn.
func TestAnInlineSVGInXHTMLSpellsItsNames(t *testing.T) {
	fills := func(svg string) []FillRect {
		src := `<html xmlns="http://www.w3.org/1999/xhtml"><body><div id="d" style="font-size: 0">` +
			svg + `</div></body></html>`
		built := Build(Input{HTML: src, XHTML: true})
		if built.Root == nil {
			t.Fatal("no boxes")
		}
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(10000)
		return fillsOfColour(Paint(Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))), blue)
	}
	const ns = `xmlns="http://www.w3.org/2000/svg"`
	if got := fills(`<svg ` + ns + ` height="60" viewBox="0 0 200 100"><rect width="100%" height="100%" fill="blue"/></svg>`); len(got) != 1 {
		t.Fatalf("the lower-case picture drew %d blue fills, want 1", len(got))
	}
	for _, svg := range []string{
		`<svg ` + ns + ` height="60" viewBox="0 0 200 100"><RECT width="100%" height="100%" fill="blue"/></svg>`,
		`<svg ` + ns + ` height="60" VIEWBOX="0 0 200 100"><rect width="100%" height="100%" fill="blue"/></svg>`,
	} {
		if got := fills(svg); len(got) != 0 {
			t.Errorf("%s drew %v, want nothing: XML names are case-sensitive", svg, got)
		}
	}
}

// TestAContentLanguagePragmaReachesTheText. A document that gives its language
// only by <meta http-equiv="content-language"> is in that language: Turkish
// upper-cases i to a dotted capital. It was in no language at all, and the
// text came out with a plain I.
func TestAContentLanguagePragmaReachesTheText(t *testing.T) {
	upper := func(head string) string {
		root := layoutOf(t, 600, `<!DOCTYPE html><html><head>`+head+
			`</head><body><p style="text-transform: uppercase">i</p></body></html>`)
		var text string
		var walk func(*Fragment)
		walk = func(f *Fragment) {
			for _, l := range f.Lines {
				for _, r := range l.Runs {
					text += r.Text
				}
			}
			for _, c := range f.Children {
				walk(c)
			}
		}
		walk(root)
		return text
	}
	if got := upper(``); got != "I" {
		t.Fatalf("with no language the text is %q, want I", got)
	}
	if got := upper(`<meta http-equiv="content-language" content="tr">`); got != "\u0130" {
		t.Errorf("under the pragma the text is %q, want \u0130", got)
	}
}

// TestTheSizeOfAnUndrawableSVGIsReadByItsNames. An SVG this cannot draw is
// still sized from its root's attributes, and those are read by name without
// the attribute classifier in front of them — so the size reader has to spell
// its names itself. In a file "VIEWBOX" gives no ratio; in HTML it is viewBox.
func TestTheSizeOfAnUndrawableSVGIsReadByItsNames(t *testing.T) {
	ratio := func(root string, names svgNames) float64 {
		c := svgIntrinsicSize([]byte(root+`<path d="M0 0"/></svg>`), svgAsImage, names)
		if c == nil {
			t.Fatalf("%s: no size at all", root)
		}
		return c.Ratio
	}
	const ns = `xmlns="http://www.w3.org/2000/svg"`
	if got := ratio(`<svg `+ns+` viewBox="0 0 200 100">`, svgXMLNames); got != 2 {
		t.Fatalf("viewBox gave ratio %v, want 2", got)
	}
	if got := ratio(`<svg `+ns+` VIEWBOX="0 0 200 100">`, svgXMLNames); got != 0 {
		t.Errorf("a file's VIEWBOX gave ratio %v, want none", got)
	}
	if got := ratio(`<svg `+ns+` VIEWBOX="0 0 200 100">`, svgHTMLNames); got != 2 {
		t.Errorf("an HTML document's VIEWBOX gave ratio %v, want 2", got)
	}
}
