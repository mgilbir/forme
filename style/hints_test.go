package style

import (
	"testing"

	"github.com/mgilbir/forme/html"
)

// Presentational hints, and above all where they sit in the cascade.
//
// The value of a hint is easy and its priority is not: a hint that beat an
// author's stylesheet would make a document impossible to restyle, and one that
// lost to the user-agent sheet would never apply at all. Both are silent
// failures — the page simply comes out at the wrong size — so each is asserted
// by having something else compete with it.

func computed(t *testing.T, markup string, sheets ...Sheet) map[string]ComputedStyle {
	t.Helper()
	doc, _, _ := html.Parse(markup)
	styled := Apply(doc, sheets)
	out := map[string]ComputedStyle{}
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if id, ok := n.Attr("id"); ok {
			out[id] = styled.Styles[n]
		}
		return true
	})
	return out
}

func TestHintAppliesWithNothingElseSaying(t *testing.T) {
	got := computed(t, `<img id="i" width="5" height="96">`)
	if w := got["i"]["width"]; w != "5px" {
		t.Errorf("width is %q, want 5px", w)
	}
	if h := got["i"]["height"]; h != "96px" {
		t.Errorf("height is %q, want 96px", h)
	}
}

// TestHintBeatsTheUserAgentSheet: a hint that lost to the defaults would never
// apply, since the defaults set a value for every property.
func TestHintBeatsTheUserAgentSheet(t *testing.T) {
	got := computed(t, `<img id="i" width="5">`,
		sheet(t, OriginUserAgent, `img { width: 999px }`))
	if w := got["i"]["width"]; w != "5px" {
		t.Errorf("width is %q; a user-agent rule beat a presentational hint", w)
	}
}

// TestAuthorSheetBeatsHint is the rule that makes a stylesheet able to take
// control of markup it did not write.
func TestAuthorSheetBeatsHint(t *testing.T) {
	got := computed(t, `<img id="i" width="5">`,
		sheet(t, OriginAuthor, `img { width: 60px }`))
	if w := got["i"]["width"]; w != "60px" {
		t.Errorf("width is %q; a presentational hint beat an author rule", w)
	}
}

// TestTheWeakestAuthorRuleStillBeatsAHint pins the specificity: a hint has
// none, so even a universal selector — the weakest thing an author can write —
// wins.
func TestTheWeakestAuthorRuleStillBeatsAHint(t *testing.T) {
	got := computed(t, `<img id="i" width="5">`,
		sheet(t, OriginAuthor, `* { width: 7px }`))
	if w := got["i"]["width"]; w != "7px" {
		t.Errorf("width is %q; a hint beat a universal author rule", w)
	}
}

// TestInlineStyleBeatsHint, since a style attribute is above every author rule.
func TestInlineStyleBeatsHint(t *testing.T) {
	got := computed(t, `<img id="i" width="5" style="width: 11px">`)
	if w := got["i"]["width"]; w != "11px" {
		t.Errorf("width is %q; a hint beat a style attribute", w)
	}
}

// TestHintValueSyntax pins HTML's dimension-value grammar. Everything outside
// it is ignored rather than guessed at: a value this cannot read must not
// become a length it invented.
func TestHintValueSyntax(t *testing.T) {
	cases := map[string]string{
		"5":     "5px",
		"050":   "050px",
		"5%":    "5%",
		" 5 ":   "5px",
		"5px":   "auto",
		"-5":    "auto",
		"5.5":   "auto",
		"abc":   "auto",
		"":      "auto",
		"5 6":   "auto",
		"99999": "99999px",
		// Longer than the digit bound, which exists so that an untrusted
		// attribute cannot state a number nobody meant.
		"12345678901": "auto",
	}
	for value, want := range cases {
		got := computed(t, `<img id="i" width="`+value+`">`)
		if w := got["i"]["width"]; w != want {
			t.Errorf("width=%q gave %q, want %q", value, w, want)
		}
	}
}

// TestHintsApplyOnlyToTheElementsThatHaveThem keeps the table from leaking: a
// width attribute on a <div> is not a presentational hint in HTML, and treating
// it as one would silently size boxes from stray markup.
func TestHintsApplyOnlyToTheElementsThatHaveThem(t *testing.T) {
	got := computed(t, `<div id="d" width="5"><span id="s" height="9">x</span></div>`)
	if w := got["d"]["width"]; w != "auto" {
		t.Errorf("a <div>'s width attribute set width to %q", w)
	}
	if h := got["s"]["height"]; h != "auto" {
		t.Errorf("a <span>'s height attribute set height to %q", h)
	}
}

// TestTableWidthAttributeIsAHint pins the entry the table algorithm made
// meaningful.
//
// The HTML Standard's table rendering section maps <table width> to the width
// property as a dimension value, so a bare number is pixels and a trailing
// per-cent sign is a percentage. Without it the suite's dbaron float tests size
// their table from its content and lay the document out at half the width the
// reference does.
func TestTableWidthAttributeIsAHint(t *testing.T) {
	cases := map[string]string{
		"300":  "300px",
		"100%": "100%",
		// Not a dimension value, so not a hint. A length with a unit is HTML's
		// own refusal, and it must not become a length this guessed at.
		"300px": "auto",
		"-1":    "auto",
	}
	for value, want := range cases {
		got := computed(t, `<table id="t" width="`+value+`"><tr><td>x</td></tr></table>`)
		if w := got["t"]["width"]; w != want {
			t.Errorf("<table width=%q> gave width %q, want %q", value, w, want)
		}
	}
}

// TestTableHeightAttributeIsAHint replaces a test that asserted the opposite.
//
// It recorded <table height> as a deliberate absence, on the reasoning that the
// standard describes no such mapping and browsers honour it only as a legacy.
// That reasoning was wrong about the standard — the attribute is in the same
// list as width, mapped the same way — and the suite says so without the prose:
// the reference for floats-wrap-bfc-005 draws with "height: 20px" on a div what
// the test writes as <table height="20">.
func TestTableHeightAttributeIsAHint(t *testing.T) {
	cases := map[string]string{
		"300": "300px",
		"50%": "50%",
		// The same refusals width takes: a dimension value is digits and an
		// optional per-cent sign, and anything else is not one.
		"300px": "auto",
		"-1":    "auto",
	}
	for value, want := range cases {
		got := computed(t, `<table id="t" height="`+value+`"><tr><td>x</td></tr></table>`)
		if h := got["t"]["height"]; h != want {
			t.Errorf("<table height=%q> gave height %q, want %q", value, h, want)
		}
	}
}

// TestNowrapAttributeOnACellIsAHint is HTML's table rendering section, which
// states it as a rule rather than as an attribute mapping:
//
//	td[nowrap], th[nowrap] { white-space: nowrap }
//
// It is a boolean attribute, so what matters is that it is there at all.
//
// It sets the two longhands rather than the shorthand, and has to: a hint goes
// straight into the cascade without passing through the expander, so naming
// white-space here would set a property nothing reads. Both of them, because
// that is what the rule says — a cell with nowrap inside a "white-space: pre"
// table collapses its spaces as well as refusing to wrap.
func TestNowrapAttributeOnACellIsAHint(t *testing.T) {
	got := computed(t, `<table><tr><td id="a" nowrap>x</td><td id="b">y</td></tr></table>`)
	if v := got["a"]["text-wrap-mode"]; v != "nowrap" {
		t.Errorf("<td nowrap> has text-wrap-mode %q, want nowrap", v)
	}
	if v := got["a"]["white-space-collapse"]; v != "collapse" {
		t.Errorf("<td nowrap> has white-space-collapse %q, want collapse", v)
	}
	// And the cell beside it is untouched, which is what makes it the
	// attribute's doing rather than a rule about cells.
	if v := got["b"]["text-wrap-mode"]; v != "wrap" {
		t.Errorf("a cell without the attribute has text-wrap-mode %q, want wrap", v)
	}
	// An author's own rule still beats it, which is where a hint sits.
	got = computed(t, `<table><tr><td id="a" nowrap>x</td></tr></table>`,
		sheet(t, OriginAuthor, `td { white-space: normal }`))
	if v := got["a"]["text-wrap-mode"]; v != "wrap" {
		t.Errorf("an author rule lost to the hint: text-wrap-mode is %q", v)
	}
}

// <font>, which is three presentational attributes and nothing else.
//
// HTML's rendering section maps them by name: colour, family, and — through a
// table of seven steps — size. They are the reason the element is worth laying
// out at all, since without them a <font> is a <span>.

// TestFontSizeAttributeIsTheSevenStepScale.
func TestFontSizeAttributeIsTheSevenStepScale(t *testing.T) {
	for _, tc := range []struct{ attr, want string }{
		// The scale itself, from the first entry to the seventh.
		{"1", "x-small"}, {"2", "small"}, {"3", "medium"}, {"4", "large"},
		{"5", "x-large"}, {"6", "xx-large"}, {"7", "xxx-large"},
		// Past either end, clamped to the entry there. "the seventh entry" and
		// "the first entry" are what the prose says.
		{"0", "x-small"}, {"8", "xxx-large"}, {"99", "xxx-large"},
		// Signed values are relative to step three, which is the default and is
		// what "medium" means.
		{"+1", "large"}, {"+4", "xxx-large"}, {"-1", "small"}, {"-2", "x-small"},
		{"-9", "x-small"}, {"+0", "medium"}, {"-0", "medium"},
		// Leading and trailing space, as every other hint allows.
		{"  5  ", "x-large"},
	} {
		got, ok := fontSizeValue(tc.attr)
		if !ok {
			t.Errorf("size=%q was refused; it is a step of the scale", tc.attr)
			continue
		}
		if got != tc.want {
			t.Errorf("size=%q gave font-size %q, want %q", tc.attr, got, tc.want)
		}
	}

	// And the keyword reaches the computed style as the length it means. It is
	// asked here rather than above because a computed font-size is an absolute
	// length — see computed.go — so the seven keywords arrive as seven numbers,
	// and asserting the scale on those would be asserting two things at once.
	for _, tc := range []struct{ attr, want string }{
		{"1", "10px"}, {"3", "16px"}, {"7", "48px"},
	} {
		cs := computed(t, `<font id="f" size="`+tc.attr+`">x</font>`)
		if s := cs["f"]["font-size"]; s != tc.want {
			t.Errorf("size=%q computed to %q, want %q", tc.attr, s, tc.want)
		}
	}
}

// TestAnUnreadableFontSizeIsIgnored, which is the same rule every other hint
// follows: a value this cannot read must not become one it guessed at.
func TestAnUnreadableFontSizeIsIgnored(t *testing.T) {
	for _, attr := range []string{"", " ", "large", "5px", "5.5", "3em", "+", "-", "1x", "x1"} {
		if got, ok := fontSizeValue(attr); ok {
			t.Errorf("size=%q was read as %q; it is not a size", attr, got)
		}
		// And nothing reached the element, so the initial value stands. It is
		// 16px rather than "medium" because a computed font-size is an absolute
		// length; the two are the same value written two ways, and the
		// assertion above is what tells "ignored" from "read as medium".
		cs := computed(t, `<font id="f" size="`+attr+`">x</font>`)
		if s := cs["f"]["font-size"]; s == "" {
			t.Errorf("size=%q left no font-size at all", attr)
		} else if s != "16px" {
			t.Errorf("size=%q gave font-size %q; it is not a size and the initial "+
				"value should stand", attr, s)
		}
	}
}

// TestFontColourAndFaceAttributes. The colour takes HTML's legacy colour value,
// which the other colour hints already read; the face is the font-family
// property written as an attribute.
func TestFontColourAndFaceAttributes(t *testing.T) {
	got := computed(t, `<font id="f" color="green" face="Courier">x</font>`)
	if c := got["f"]["color"]; c != "green" {
		t.Errorf("color is %q, want green", c)
	}
	if f := got["f"]["font-family"]; f != `"Courier"` {
		t.Errorf("font-family is %q, want the family quoted", f)
	}
}

// TestAFaceIsQuotedBecauseAnAttributeIsNotAStylesheet.
//
// An unquoted family name in CSS is a sequence of identifiers; an attribute may
// hold anything at all. "PASS PASS" is one family name in an attribute and two
// identifiers in a stylesheet, and content-076 writes exactly that.
func TestAFaceIsQuotedBecauseAnAttributeIsNotAStylesheet(t *testing.T) {
	for _, tc := range []struct{ attr, want string }{
		{"PASS PASS", `"PASS PASS"`},
		{"Courier, monospace", `"Courier", "monospace"`},
		{"  Courier  ,  serif  ", `"Courier", "serif"`},
	} {
		got := computed(t, `<font id="f" face="`+tc.attr+`">x</font>`)
		if f := got["f"]["font-family"]; f != tc.want {
			t.Errorf("face=%q gave %q, want %q", tc.attr, f, tc.want)
		}
	}
	// A quote or a backslash cannot be quoted here without an escaping pass, and
	// a family by that name is not one anybody has. The whole list is refused
	// rather than half of it.
	// Single-quoted in the markup, because a double quote in the value would
	// end a double-quoted attribute and the fixture would not be the one
	// described.
	for _, attr := range []string{`a"b`, `a\b`, `ok, a"b`, ``, `,`} {
		got := computed(t, `<font id="f" face='`+attr+`'>x</font>`)
		if f := got["f"]["font-family"]; f != "serif" {
			t.Errorf("face=%q gave %q; it is not a family list and the initial "+
				"value should stand", attr, f)
		}
	}
}
