package layout

import (
	"strings"
	"testing"
)

// word-space-transform where it meets <wbr>, which is the half of the property
// that is not about text.
//
// It expands a *virtual word separator*, and there are two of them: U+200B,
// which a document writes in its text, and <wbr>, which it writes as an element.
// The box builder gives the element a zero width space of its own, which is what
// HTML says it is rendered as, and that puts the two on one path — the same
// collapsing, the same transform, the same measuring, the same breaking.

// wstText is the text of an element's lines, joined, which is what a reader
// would see and is what every assertion here is about.
func wstText(t *testing.T, markup, css string) string {
	t.Helper()
	root := layoutOf(t, 900, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 900px } `+css)
	var b strings.Builder
	for _, l := range find(t, root, "p").Lines {
		for _, r := range l.Runs {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

// TestBothKindsOfSeparatorAreExpanded is the property, and the fixture is the
// suite's own: a <wbr> and a zero width space in one line, which must come out
// alike or the element and the character are two features rather than one.
func TestBothKindsOfSeparatorAreExpanded(t *testing.T) {
	for _, tc := range []struct{ value, want string }{
		{"space", "a b c"},
		{"ideographic-space", "a　b　c"},
		{"none", "abc"},
	} {
		got := wstText(t, `a<wbr>b&#x200b;c`, `#p { word-space-transform: `+tc.value+` }`)
		if got != tc.want {
			t.Errorf("at %q: %q, want %q", tc.value, got, tc.want)
		}
	}
}

// TestTheValueOnTheBoxHoldingTheSeparatorDecides. The property inherits, so a
// rule on a container reaches the marks inside it — and a rule on the mark's own
// element overrules that. The suite writes both: -004 turns it off on the <wbr>,
// -005 on a <span> around a zero width space, -006 turns it on for those two
// alone.
func TestTheValueOnTheBoxHoldingTheSeparatorDecides(t *testing.T) {
	for _, tc := range []struct{ markup, css, want, what string }{
		{`a<wbr>b&#x200b;c`,
			`#p { word-space-transform: space } wbr { word-space-transform: none }`,
			"ab c", "off on the wbr alone"},
		{`<span>a&#x200b;b</span>&#x200b;c`,
			`#p { word-space-transform: space } span { word-space-transform: none }`,
			"ab c", "off on a span alone"},
		{`a<wbr>b<span>&#x200b;c</span>`,
			`wbr, span { word-space-transform: space }`,
			"a b c", "on for those two alone"},
		{`<span>a<wbr>b</span>`,
			`#p { word-space-transform: space }`,
			"a b", "inherited through a span"},
	} {
		if got := wstText(t, tc.markup, tc.css); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestTheSuitesOwnFourteenWaysOfWritingIt.
//
// word-space-transform-007 is one line with every arrangement of separators and
// spaces in it, and its reference is ten letters with one space between each.
// Getting it right is the whole of "the property happens between the two phases
// of white space processing": a separator beside a space is one place a line may
// end, not two.
func TestTheSuitesOwnFourteenWaysOfWritingIt(t *testing.T) {
	const markup = `a<wbr> b <wbr>c &#x200B;d&#x200B; e<wbr><wbr>f&#x200B;` +
		`&#x200B;g<wbr>&#x200B;h&#x200B;<wbr>i <wbr> &#x200B; j`
	got := wstText(t, markup, `#p { word-space-transform: space }`)
	if want := "a b c d e f g h i j"; got != want {
		t.Errorf("%q,\nwant %q", got, want)
	}
	// And the same line where nothing collapses, which is -008: every separator
	// is its own and the doubled spaces are drawn.
	got = wstText(t, markup, `#p { word-space-transform: space; white-space: pre }`)
	if want := "a  b  c  d  e  f  g  h  i     j"; got != want {
		t.Errorf("under pre: %q,\nwant %q", got, want)
	}
}

// TestAnExpandedSeparatorBreaksALineEvenUnderKeepAll. The space is a real space
// and offers what one offers, which is what -013 and -014 ask by name: the
// author asked for the word divisions to be *shown*, and a value that stops
// lines breaking between words is not a reason to set them as one.
func TestAnExpandedSeparatorBreaksALineEvenUnderKeepAll(t *testing.T) {
	root := layoutOf(t, 600, `<div id="p">aa<wbr>bb<wbr>cc</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 84px;
		      word-space-transform: space; word-break: keep-all }`)
	got := lineTextsOf(t, root, "p")
	if strings.Join(got, "|") != "aa bb|cc" {
		t.Errorf("%q, want [\"aa bb\" \"cc\"] — seven characters of room and the "+
			"inserted spaces are places a line may end", got)
	}
}

// TestADocumentThatDoesNotAskIsUnchanged is the containment case. The property
// is read for every text node in every document, and at its initial value a zero
// width space must stay what it was and a <wbr> must stay the empty element the
// flattening makes a break opportunity of.
func TestADocumentThatDoesNotAskIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ markup, want string }{
		{`a<wbr>b`, "ab"},
		// A zero width space is drawn by nothing, so it is not in the run
		// either; what says it is still there is the break it offers below.
		{`a&#x200b;b`, "ab"},
		{`the quick brown fox`, "the quick brown fox"},
		{`a <span>b</span> c`, "a b c"},
		{`sur<wbr>name`, "surname"},
	} {
		if got := wstText(t, tc.markup, ``); got != tc.want {
			t.Errorf("%q came out %q, want %q", tc.markup, got, tc.want)
		}
	}
	// And the <wbr> still offers its opportunity, which is what it was for
	// before this property existed.
	root := layoutOf(t, 600, `<div id="p">aaaa<wbr>bbbb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 48px }`)
	if got := lineTextsOf(t, root, "p"); len(got) != 2 {
		t.Errorf("the element stopped offering a break: %q", got)
	}
}

// TestAutoPhraseIsReported. The value asks for word separators to be *invented*
// at phrase boundaries the author did not mark, which for Japanese means a
// dictionary and a segmentation model. What the document gets is its explicit
// marks expanded and a finding for the rest — too few spaces rather than none,
// and a caller who is told.
func TestAutoPhraseIsReported(t *testing.T) {
	built := Build(Input{
		HTML: `<div id="p">a<wbr>b</div>`,
		CSS:  []Stylesheet{{Source: `#p { word-space-transform: space auto-phrase }`}},
	})
	found := false
	for _, f := range built.Findings {
		if f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "auto-phrase") {
			found = true
		}
	}
	if !found {
		t.Errorf("auto-phrase was not reported: %v", built.Findings)
	}
	// The half that is done is still done.
	if got := wstText(t, `a<wbr>b`, `#p { word-space-transform: space auto-phrase }`); got != "a b" {
		t.Errorf("the explicit mark was not expanded: %q", got)
	}
	// And a document that does not write it is not reported.
	plain := Build(Input{
		HTML: `<div id="p">a<wbr>b</div>`,
		CSS:  []Stylesheet{{Source: `#p { word-space-transform: space }`}},
	})
	for _, f := range plain.Findings {
		if strings.Contains(f.Message, "word-space-transform") {
			t.Errorf("a document that asked for nothing unusual was reported: %s", f.Message)
		}
	}
}
