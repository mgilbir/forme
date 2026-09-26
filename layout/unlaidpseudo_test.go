package layout

import (
	"strings"
	"testing"
)

// Every accepted display value is laid out as it says or reported. Two kinds
// of box were neither: an annotation standing outside any ruby, and a
// ::before or ::after, which the report never visited because it walked the
// elements. And "display: contents" on a pseudo-element was laid out as an
// inline box, with the pseudo-element's background, padding and decoration,
// where the element beside it with the same value generates none.

// pathedDisplayFindings is every finding about display, as path and message.
func pathedDisplayFindings(t *testing.T, in Input) [][2]string {
	t.Helper()
	var out [][2]string
	for _, f := range Compose(in, Options{}).Findings {
		if f.Property == "display" {
			out = append(out, [2]string{f.Path, f.Message})
		}
	}
	return out
}

func displayFindingsOf(t *testing.T, doc, css string) [][2]string {
	t.Helper()
	return pathedDisplayFindings(t, Input{HTML: doc, CSS: []Stylesheet{{Source: css}}})
}

// TestAnAnnotationOutsideAnyRubyIsReported is css-ruby-1 §2.2: an annotation
// with no ruby around it is wrapped in an anonymous ruby of its own and lifted
// above an empty base, and this engine lays it out as an inline box. Inside a
// ruby the report is the ruby's and not the annotation's, and a blockified
// annotation is a block (css-display-3 §2.7), which is what is laid out.
func TestAnAnnotationOutsideAnyRubyIsReported(t *testing.T) {
	const doc = `<div id="w"><p id="p">a<span id="a">b</span></p></div>`
	for _, value := range []string{"ruby-text", "ruby-text-container"} {
		got := displayFindingsOf(t, doc, `#a { display: `+value+` }`)
		if len(got) != 1 || !strings.HasSuffix(got[0][0], "span#a") ||
			!strings.Contains(got[0][1], "outside any ruby") {
			t.Errorf("%s outside any ruby reported %v, want one finding about the span", value, got)
		}
	}
	for _, c := range []struct{ what, css string }{
		{"inside a ruby", `#w { display: ruby } #a { display: ruby-text }`},
		{"inside an inline ruby", `#w { display: inline ruby } #a { display: ruby-text }`},
		{"inside a block ruby", `#w { display: block ruby } #a { display: ruby-text }`},
	} {
		got := displayFindingsOf(t, doc, c.css)
		if len(got) != 1 || !strings.HasSuffix(got[0][0], "div#w") {
			t.Errorf("%s: reported %v, want the ruby's one finding and not the annotation's",
				c.what, got)
		}
	}
	// A ruby is a ruby however its display is spelled, so a nested "inline
	// ruby" or "block ruby" holds its own annotation, and the ruby around it
	// does not report it; and an annotation nobody lays out is not one.
	const nested = `<div id="w"><p id="p">a<span id="a">b</span></p></div>`
	for _, inner := range []string{"inline ruby", "block ruby"} {
		got := displayFindingsOf(t, nested, `#w { display: ruby } #p { display: `+inner+` }
			#a { display: ruby-text }`)
		if len(got) != 1 || !strings.HasSuffix(got[0][0], "p#p") {
			t.Errorf("an annotation in a nested %q reported %v, want the nested ruby's "+
				"one finding", inner, got)
		}
	}
	if got := displayFindingsOf(t, doc, `#w { display: ruby } #p { display: none }
		#a { display: ruby-text }`); len(got) != 0 {
		t.Errorf("a ruby whose only annotation is under display: none reported %v", got)
	}
	for _, c := range []struct{ what, css string }{
		{"a base", `#a { display: ruby-base }`},
		{"floated", `#a { display: ruby-text; float: left }`},
		{"absolutely positioned", `#a { display: ruby-text; position: absolute }`},
		{"a flex item", `#p { display: flex } #a { display: ruby-text }`},
		{"a grid item", `#p { display: grid } #a { display: ruby-text }`},
		{"a flex item through contents", `#p { display: flex } #c { display: contents }
			#a { display: ruby-text }`},
		{"under display: none", `#p { display: none } #a { display: ruby-text }`},
	} {
		d := doc
		if strings.Contains(c.css, "#c") {
			d = `<div id="w"><p id="p"><span id="c"><span id="a">b</span></span></p></div>`
		}
		if got := displayFindingsOf(t, d, c.css); len(got) != 0 {
			t.Errorf("%s: reported %v, and it is laid out as asked", c.what, got)
		}
	}
}

// TestAPseudoElementsDisplayIsReported: a ::before or an ::after is a box like
// any other, and a display this engine lays out as something else is reported
// for it as it is for an element — only where it generates a box.
func TestAPseudoElementsDisplayIsReported(t *testing.T) {
	const doc = `<div id="w"><p id="p">a</p></div>`
	for _, c := range []struct{ css, says string }{
		{`#p::before { content: "x"; display: run-in }`, "::before"},
		{`#p::after { content: "x"; display: inline list-item }`, "::after"},
		{`#p::before { content: "x"; display: ruby-text }`, "outside any ruby"},
	} {
		got := displayFindingsOf(t, doc, c.css)
		if len(got) != 1 || !strings.Contains(got[0][1], c.says) ||
			!strings.HasSuffix(got[0][0], "p#p") {
			t.Errorf("%q reported %v, want one finding about #p saying %q", c.css, got, c.says)
		}
	}
	for _, c := range []struct{ what, css string }{
		{"no content", `#p::before { display: run-in }`},
		{"content: none", `#p::before { content: none; display: run-in }`},
		{"a ruby, which holds no annotation", `#p::before { content: "x"; display: ruby }`},
		{"a block", `#p::before { content: "x"; display: block }`},
		{"a flex container", `#p::before { content: "x"; display: inline-flex }`},
		{"an annotation in a flex container, which makes it a block", `#p { display: flex }
			#p::before { content: "x"; display: ruby-text }`},
		{"a floated annotation, which is a block", `#p::before { content: "x";
			display: ruby-text; float: left }`},
		{"an absolutely positioned annotation, which is a block", `#p::before { content: "x";
			display: ruby-text; position: absolute }`},
		{"under display: none", `#w { display: none } #p::before { content: "x"; display: run-in }`},
	} {
		if got := displayFindingsOf(t, doc, c.css); len(got) != 0 {
			t.Errorf("%s: reported %v", c.what, got)
		}
	}
	// An annotation ::before of a ruby, or of an element inside one, is the
	// ruby's annotation: the ruby reports it, and the pseudo-element does not.
	for _, css := range []string{
		`#w { display: ruby } #w::before { content: "x"; display: ruby-text }`,
		`#w { display: ruby } #p::after { content: "x"; display: ruby-text }`,
	} {
		got := displayFindingsOf(t, doc, css)
		if len(got) != 1 || !strings.HasSuffix(got[0][0], "div#w") {
			t.Errorf("%q reported %v, want the ruby's one finding", css, got)
		}
	}
}

// TestDisplayContentsOnAPseudoElementGeneratesNoBox is css-display-3 on a
// pseudo-element: no box, and its content generated as normal. The text is
// drawn in the pseudo-element's colour, which it inherits, and nothing of the
// pseudo-element's own box — its background, its padding, its underline — is
// on the page, exactly as for the element beside it with the same value.
func TestDisplayContentsOnAPseudoElementGeneratesNoBox(t *testing.T) {
	const doc = `<p id="p"><span id="s">x</span></p>`
	const face = `#p { font-family: Courier; font-size: 20px }`
	const decor = `background-color: #ff0000; padding: 0 5px; text-decoration: underline;
		color: #0000ff`
	pseudo := paintOf(t, doc, noDefaults+face+`#s::before { content: "y"; display: contents; `+decor+` }`)
	element := paintOf(t, `<p id="p"><span id="s"><span id="y">y</span>x</span></p>`,
		noDefaults+face+`#y { display: contents; `+decor+` }`)
	for what, ops := range map[string][]Op{"the pseudo-element": pseudo, "the element": element} {
		if got := fillsOf(ops, red); len(got) != 0 {
			t.Errorf("%s: its background was drawn: %v", what, got)
		}
		if got := fillsOf(ops, blue); len(got) != 0 {
			t.Errorf("%s: its underline was drawn: %v", what, got)
		}
		y, x := paintedRuns(ops, "y"), paintedRuns(ops, "x")
		if len(y) != 1 || len(x) != 1 {
			t.Fatalf("%s: %d runs of y and %d of x\n%s", what, len(y), len(x), sketchClips(ops))
		}
		if y[0].Color != blue {
			t.Errorf("%s: y is drawn in %v, want the inherited blue", what, y[0].Color)
		}
		// Courier is 0.6em: 12px for the "y", and no padding either side.
		if got := x[0].At.X.Sub(y[0].At.X).Px(); got != 12 {
			t.Errorf("%s: x is %vpx after y, want 12 with no padding", what, got)
		}
	}
	// The control: as an inline box it has all three.
	inline := paintOf(t, doc, noDefaults+face+`#s::before { content: "y"; display: inline; `+decor+` }`)
	if len(fillsOf(inline, red)) == 0 || len(fillsOf(inline, blue)) == 0 {
		t.Errorf("control: an inline ::before drew no background or no underline\n%s",
			sketchClips(inline))
	}
}

// TestDisplayContentsOnAPictureIsReported: a pseudo-element whose content is
// one picture is a replaced element, and "display: contents" is not honoured on
// one — it keeps its box, as an <img> does, and says so.
func TestDisplayContentsOnAPictureIsReported(t *testing.T) {
	res := mapResolver{"mark.png": encodePNG(t, 20, 10)}
	got := pathedDisplayFindings(t, Input{
		HTML:      `<p id="p">x</p>`,
		CSS:       []Stylesheet{{Source: `#p::before { content: url(mark.png); display: contents }`}},
		Resources: res,
	})
	if len(got) != 1 || !strings.Contains(got[0][1], "::before whose content is a picture") {
		t.Errorf("reported %v, want one finding that the picture kept its box", got)
	}
	if got := pathedDisplayFindings(t, Input{
		HTML:      `<p id="p">x</p>`,
		CSS:       []Stylesheet{{Source: `#p::before { content: "a"; display: contents }`}},
		Resources: res,
	}); len(got) != 0 {
		t.Errorf("text content is honoured, and reported %v", got)
	}
}
