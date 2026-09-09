package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §5.12.2's ::first-letter: the first letter of the first formatted line
// of a block container, set in the pseudo-element's own type.
//
// It parsed and matched and was never computed, so a rule written for one did
// nothing at all — which the cascade reported, and reporting is not doing. What
// it is now is a division of the text: the letter is a text box of its own with
// the pseudo-element's declarations written over the style it would have had.

// letterBoxes is every text box of a document, in order, with its colour and
// size — which is what a ::first-letter divides and restyles.
func letterBoxes(t *testing.T, htmlSrc, css string) []*Box {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: css}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	var out []*Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b == nil {
			return
		}
		if b.IsText() {
			out = append(out, b)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	return out
}

func TestAFirstLetterIsSetInItsOwnStyle(t *testing.T) {
	for _, tc := range []struct {
		html, css string
		want      []string
		what      string
	}{
		{`<div id="d">hello</div>`, `#d::first-letter { color: red }`,
			[]string{"h", "ello"}, "one letter"},
		{`<div id="d"><span>hello</span></div>`, `#d::first-letter { color: red }`,
			[]string{"h", "ello"}, "the letter inside a span"},

		// css-pseudo-4: punctuation that precedes or follows the letter is part
		// of it, which is what makes a quoted opening word a drop cap and not a
		// quote mark set on its own.
		{`<div id="d">"hello" there</div>`, `#d::first-letter { color: red }`,
			[]string{"\"h", "ello\" there"}, "an opening quote"},
		{`<div id="d">(a) b</div>`, `#d::first-letter { color: red }`,
			[]string{"(a)", " b"}, "a bracketed letter, closed on both sides"},

		// The unit is the typographic character unit and not the code point: a
		// letter and the mark on it are one letter. combining-characters-002 is
		// that assertion and nothing else.
		{"<div id=\"d\">e\u0301llo</div>", `#d::first-letter { color: red }`,
			[]string{"e\u0301", "llo"}, "a letter and its combining mark"},
	} {
		got := letterBoxes(t, tc.html, tc.css)
		if len(got) != len(tc.want) {
			t.Errorf("%s: %d text boxes, want %d", tc.what, len(got), len(tc.want))
			continue
		}
		for i := range got {
			if got[i].Text != tc.want[i] {
				t.Errorf("%s: text boxes are %q, want %q",
					tc.what, textsOf(got), tc.want)
				break
			}
		}
		if got[0].Style["color"] == got[len(got)-1].Style["color"] {
			t.Errorf("%s: the letter and the rest are both %q",
				tc.what, got[0].Style["color"])
		}
	}
}

// TestAFirstLetterTransformsOnlyTheLetter. text-transform applies to a
// ::first-letter, and it is the property combining-characters-002 uses: the
// element's own transform has already run when the division happens, so the
// pseudo-element's is a second one over the letter alone.
func TestAFirstLetterTransformsOnlyTheLetter(t *testing.T) {
	got := letterBoxes(t, `<div id="d">hello</div>`,
		`#d::first-letter { text-transform: uppercase }`)
	if len(got) != 2 || got[0].Text != "H" || got[1].Text != "ello" {
		t.Errorf("the text boxes are %q, want [\"H\" \"ello\"]", textsOf(got))
	}
	// And the same mark, which is what makes the unit the cluster: uppercasing
	// "e" with an acute on it gives one letter and not a letter and a mark.
	got = letterBoxes(t, "<div id=\"d\">e\u0301llo</div>",
		`#d::first-letter { text-transform: uppercase }`)
	if len(got) != 2 || got[0].Text != "E\u0301" {
		t.Errorf("the text boxes are %q, want the letter uppercased whole",
			textsOf(got))
	}
}

// TestAFirstLetterFontSizeIsResolvedAgain. The size the box builder gave the
// text box is the element's, and this style is not the element's — so a
// "font-size: 200%" on the pseudo-element has to be resolved against the
// element's own before the letter is measured.
func TestAFirstLetterFontSizeIsResolvedAgain(t *testing.T) {
	got := letterBoxes(t, `<div id="d">hello</div>`,
		`#d { font-size: 20px } #d::first-letter { font-size: 200% }`)
	if len(got) != 2 {
		t.Fatalf("%d text boxes, want two", len(got))
	}
	want, _ := style.FromPx(40)
	if got[0].FontSize != want {
		t.Errorf("the letter is set at %v, want twice the element's 20px", got[0].FontSize)
	}
	if rest, _ := style.FromPx(20); got[1].FontSize != rest {
		t.Errorf("the rest is set at %v, want the element's own 20px", got[1].FontSize)
	}
}

// TestAFirstLetterWithNoRuleChangesNothing is the containment case: the division
// costs a document that writes no such rule nothing, and a rule naming only
// properties this engine cannot act on divides nothing either.
func TestAFirstLetterWithNoRuleChangesNothing(t *testing.T) {
	for _, css := range []string{
		``,
		`#d::first-letter { float: left }`,
		`#d::first-letter { border: 1px solid red; padding: 2px }`,
	} {
		got := letterBoxes(t, `<div id="d">hello</div>`, css)
		if len(got) != 1 || got[0].Text != "hello" {
			t.Errorf("%q divided the text into %q", css, textsOf(got))
		}
	}
}

// TestAFirstLetterSaysWhatItCannotDo. The properties that need the letter to
// have a box of its own — a float, a border, a background — are what a drop cap
// is written with, and an author who writes one has to be told the letter came
// out ordinary.
func TestAFirstLetterSaysWhatItCannotDo(t *testing.T) {
	for _, tc := range []struct{ css, name string }{
		{`#d::first-letter { float: left }`, "float"},
		{`#d::first-letter { border: 1px solid red }`, "border-left-width"},
		{`#d::first-letter { background-color: red }`, "background-color"},
		{`#d::first-letter { margin-left: 2px }`, "margin-left"},
	} {
		built := Build(Input{HTML: `<div id="d">hello</div>`,
			CSS: []Stylesheet{{Source: tc.css}}})
		var said bool
		for _, f := range built.Findings {
			if f.Rule == RuleUnsupportedValue && f.Property == tc.name {
				said = true
			}
		}
		if !said {
			t.Errorf("%q reported %v, want the %s named", tc.css, built.Findings, tc.name)
		}
	}
	// And a rule this engine does act on says nothing, or every drop cap would
	// carry a finding about the part that worked.
	built := Build(Input{HTML: `<div id="d">hello</div>`,
		CSS: []Stylesheet{{Source: `#d::first-letter { color: red; font-size: 200% }`}}})
	if len(built.Findings) != 0 {
		t.Errorf("a ::first-letter this engine applies reported %v", built.Findings)
	}
}

// textsOf is the text of each box, for a failure message.
func textsOf(boxes []*Box) []string {
	out := make([]string, len(boxes))
	for i, b := range boxes {
		out[i] = b.Text
	}
	return out
}
