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

// The valign attribute, which HTML's table rendering section maps to
// vertical-align on every part of a table that can carry one.
//
// It is worth its own set because the mapping is not the identity — "center" is
// what a document writes and "middle" is what the property calls it — and
// because a cell reaches its value by a different route from a row's: td and th
// are read by cellHints and everything else by the table above it.

func TestValignMapsToVerticalAlign(t *testing.T) {
	for _, tc := range []struct{ attr, want string }{
		{"top", "top"},
		{"middle", "middle"},
		// The one name the two vocabularies do not share.
		{"center", "middle"},
		{"bottom", "bottom"},
		{"baseline", "baseline"},
		// Case-insensitively, which is what a document written in 1998 looks
		// like and the reason the attribute is worth reading at all.
		{"BOTTOM", "bottom"},
		{" Center ", "middle"},
	} {
		got := computed(t, `<table><tr id="r" valign="`+tc.attr+`"><td id="c" valign="`+tc.attr+`">x</td></tr></table>`)
		if v := got["r"]["vertical-align"]; v != tc.want {
			t.Errorf("<tr valign=%q> gave vertical-align %q, want %q", tc.attr, v, tc.want)
		}
		if v := got["c"]["vertical-align"]; v != tc.want {
			t.Errorf("<td valign=%q> gave vertical-align %q, want %q", tc.attr, v, tc.want)
		}
	}
}

// TestAnUnreadableValignIsIgnored. A word that is not one of the five is not an
// alignment, and passing it through would put it in the computed style as a
// value of vertical-align that nothing can read — a length where a keyword
// belongs, or a keyword the property has never had.
//
// Asserted against a cell with no attribute at all rather than against a
// stylesheet, because a hint loses to an author rule whatever it says: a test
// that let one compete would pass with the attribute value handed straight
// through.
func TestAnUnreadableValignIsIgnored(t *testing.T) {
	base := computed(t, `<table><tr><td id="c">x</td></tr></table>`)["c"]["vertical-align"]
	if base == "" {
		t.Fatal("a cell with no valign has no computed vertical-align, so this " +
			"test is comparing nothing")
	}
	for _, attr := range []string{"", "centre", "sub", "5", "top bottom", "super"} {
		got := computed(t, `<table><tr><td id="c" valign="`+attr+`">x</td></tr></table>`)["c"]["vertical-align"]
		if got != base {
			t.Errorf("<td valign=%q> computed vertical-align %q; a value that is "+
				"not one of the five leaves the cell as it was, which is %q",
				attr, got, base)
		}
	}
}

// TestValignIsAHintAndNotARule is the cascade half, and it is the half that
// decides whether the attribute is usable.
//
// The user-agent sheet says "tr, td, th { vertical-align: inherit }" — that rule
// is what carries a row's alignment to its cells, since the property does not
// inherit on its own — so a valign that lost to a user-agent rule would never
// apply to a cell at all. An author rule has to win, or a stylesheet could not
// take control of markup it did not write.
func TestValignIsAHintAndNotARule(t *testing.T) {
	got := computed(t, `<table><tr><td id="c" valign="bottom">x</td></tr></table>`,
		sheet(t, OriginUserAgent, `td { vertical-align: inherit }`))
	if v := got["c"]["vertical-align"]; v != "bottom" {
		t.Errorf("vertical-align is %q; a user-agent rule beat the valign attribute", v)
	}

	got = computed(t, `<table><tr><td id="c" valign="bottom">x</td></tr></table>`,
		sheet(t, OriginAuthor, `td { vertical-align: top }`))
	if v := got["c"]["vertical-align"]; v != "top" {
		t.Errorf("vertical-align is %q; the valign attribute beat an author rule", v)
	}
}

// TestValignOnARowReachesItsCells is the whole point of putting the attribute
// on the row groups and rows as well as on the cells: the property does not
// inherit, and the user-agent sheet's "inherit" is what makes it travel.
func TestValignOnARowReachesItsCells(t *testing.T) {
	for _, markup := range []string{
		`<table><tr valign="bottom"><td id="c">x</td></tr></table>`,
		`<table><tbody valign="bottom"><tr><td id="c">x</td></tr></tbody></table>`,
	} {
		got := computed(t, markup, sheet(t, OriginUserAgent,
			`thead, tbody, tfoot, table > tr { vertical-align: middle }
			 tr, td, th { vertical-align: inherit }`))
		if v := got["c"]["vertical-align"]; v != "bottom" {
			t.Errorf("in %s the cell's vertical-align is %q, want bottom", markup, v)
		}
	}
}

// TestACellsWidthAndHeightAreHints.
//
// HTML's table rendering section maps them exactly as it maps the table's own,
// and they were missing — so "<td width=50%>", which is how a table said what
// proportion a column takes and is still how most tables in older documents say
// it, set nothing and the column was sized by its content.
//
// The percentage is the whole point of them. A bare number is a pixel width a
// stylesheet could have given instead; a percentage is a statement about the
// table that nothing else in the markup can make.
func TestACellsWidthAndHeightAreHints(t *testing.T) {
	for _, c := range []struct{ cell, property, want string }{
		{`<td id="c" width="120">x</td>`, "width", "120px"},
		{`<td id="c" width="50%">x</td>`, "width", "50%"},
		{`<th id="c" width="25%">x</th>`, "width", "25%"},
		{`<td id="c" height="40">x</td>`, "height", "40px"},
		{`<th id="c" height="10%">x</th>`, "height", "10%"},
		// The refusals the table's own attributes take.
		{`<td id="c" width="120px">x</td>`, "width", "auto"},
		{`<td id="c" width="-1">x</td>`, "width", "auto"},
		{`<td id="c" width="florb">x</td>`, "width", "auto"},
	} {
		got := computed(t, `<table><tr>`+c.cell+`</tr></table>`)
		if v := got["c"][c.property]; v != c.want {
			t.Errorf("%s gave %s %q, want %q", c.cell, c.property, v, c.want)
		}
	}
	// And the two hints a cell had before still arrive, which is what says the
	// attribute table was added to the cell path rather than put in front of it.
	got := computed(t, `<table cellpadding="7"><tr>`+
		`<td id="c" width="120" nowrap valign="top">x</td></tr></table>`)
	for property, want := range map[string]string{
		"width": "120px", "padding-top": "7px",
		"text-wrap-mode": "nowrap", "vertical-align": "top",
	} {
		if v := got["c"][property]; v != want {
			t.Errorf("a cell with four hints on it gave %s %q, want %q", property, v, want)
		}
	}
}

// TestAZeroDimensionIsNoDimension.
//
// Most of these attributes are mapped "ignoring zero", which is not a detail:
// the wording sends the value through the rules for parsing *nonzero* dimension
// values, which error on a zero, so the attribute is absent rather than zero.
// "<img width=0>" is an image at its own width in every browser and was an
// invisible one here.
//
// It is a list and not a rule about dimensions, because the wording is not
// uniform and the difference is deliberate: a table's width ignores a zero and
// its height does not, in the same sentence of the same section.
func TestAZeroDimensionIsNoDimension(t *testing.T) {
	for _, c := range []struct{ markup, id, property, want string }{
		{`<img id="i" width="0" src="x">`, "i", "width", "auto"},
		{`<img id="i" height="0" src="x">`, "i", "height", "auto"},
		{`<img id="i" width="00" src="x">`, "i", "width", "auto"},
		{`<img id="i" width="0%" src="x">`, "i", "width", "auto"},
		{`<table id="t" width="0"><tr><td>x</td></tr></table>`, "t", "width", "auto"},
		{`<table><tr><td id="c" width="0">x</td></tr></table>`, "c", "width", "auto"},
		{`<table><tr><th id="c" height="0">x</th></tr></table>`, "c", "height", "auto"},
		// A table's *height* is mapped without the words, so its zero stands.
		{`<table id="t" height="0"><tr><td>x</td></tr></table>`, "t", "height", "0px"},
		// And an SVG's are SVG's own, where a zero means the element is not
		// rendered rather than that the attribute was not written.
		{`<svg id="s" width="0" height="0"></svg>`, "s", "width", "0px"},
		// The control: a dimension that is not zero is still read.
		{`<img id="i" width="1" src="x">`, "i", "width", "1px"},
	} {
		got := computed(t, c.markup)
		if v := got[c.id][c.property]; v != c.want {
			t.Errorf("%s gave %s %q, want %q", c.markup, c.property, v, c.want)
		}
	}
}

// TestATableBorderAttributeIsThreeThings.
//
// HTML's rendering section maps it to the four border widths on the table, and
// then gives two more rules whose condition the selector language cannot state:
//
//	table[border] { border-style: outset }  /* only if border is not equivalent to zero */
//	table[border] > tr > td, ... { border-width: 1px; border-style: inset }
//
// The comment is the specification's own, and it is a comment because "not
// equivalent to zero" means the value parsed as an integer, which no selector
// does: "0" and "00" are the same border and "[border=0]" tells them apart.
func TestATableBorderAttributeIsThreeThings(t *testing.T) {
	for _, c := range []struct{ attr, width, style string }{
		{`border="1"`, "1px", "outset"},
		{`border="5"`, "5px", "outset"},
		{`border="0"`, "0px", "none"},
		// The zero a selector cannot see. Both of these are the same border as
		// "0" and neither is the string "0".
		{`border="00"`, "0px", "none"},
		{`border=" 0"`, "0px", "none"},
		// "Rules for parsing non-negative integers" take the leading digits and
		// ignore what follows.
		{`border="3px"`, "3px", "outset"},
		// And what has no leading digits at all is the parse error the section
		// gives a default of one pixel for — which is the one thing about this
		// attribute nobody expects, since every other dimension attribute drops
		// what it cannot read.
		{`border="yes"`, "1px", "outset"},
		{`border=""`, "1px", "outset"},
		{`border`, "1px", "outset"},
		{`border="-1"`, "1px", "outset"},
	} {
		got := computed(t, `<table id="t" `+c.attr+`><tr><td id="c">x</td></tr></table>`)
		if w := got["t"]["border-top-width"]; w != c.width {
			t.Errorf("<table %s> gave the table border-top-width %q, want %q",
				c.attr, w, c.width)
		}
		if s := got["t"]["border-top-style"]; s != c.style {
			t.Errorf("<table %s> gave the table border-top-style %q, want %q",
				c.attr, s, c.style)
		}
		// And the cell, which takes a one-pixel inset border from a table that
		// draws one and nothing from a table that does not.
		wantCell, wantCellWidth := "none", "medium"
		if c.style != "none" {
			wantCell, wantCellWidth = "inset", "1px"
		}
		if s := got["c"]["border-left-style"]; s != wantCell {
			t.Errorf("<table %s> gave the cell border-left-style %q, want %q",
				c.attr, s, wantCell)
		}
		if w := got["c"]["border-left-width"]; w != wantCellWidth {
			t.Errorf("<table %s> gave the cell border-left-width %q, want %q",
				c.attr, w, wantCellWidth)
		}
	}
}

// TestANestedTablesCellsTakeTheirOwnTablesBorder is the child combinator in the
// specification's selector, said as a document.
//
// "table[border] > tr > td" reaches the cells of that table and not the cells
// of a table inside one of them, and the walk that finds the table has to stop
// at the first one for the same reason cellpadding's does.
func TestANestedTablesCellsTakeTheirOwnTablesBorder(t *testing.T) {
	got := computed(t, `<table border="3"><tr><td id="outer">`+
		`<table><tr><td id="inner">x</td></tr></table></td></tr></table>`)
	if s := got["outer"]["border-left-style"]; s != "inset" {
		t.Errorf("the outer cell has border-left-style %q, want inset", s)
	}
	if s := got["inner"]["border-left-style"]; s != "none" {
		t.Errorf("the inner cell has border-left-style %q; its own table has no "+
			"border attribute, and the one it sits inside is not its own", s)
	}
}

// TestTheBorderAttributeIsAHintLikeTheRest, which is where it sits in the
// cascade: below every author declaration and above the user agent sheet.
func TestTheBorderAttributeIsAHintLikeTheRest(t *testing.T) {
	got := computed(t, `<table id="t" border="4"><tr><td id="c">x</td></tr></table>`,
		author(t, `#t { border-top-style: dashed } #c { border-left-width: 9px }`))
	if s := got["t"]["border-top-style"]; s != "dashed" {
		t.Errorf("an author's border-top-style lost to the attribute: %q", s)
	}
	if w := got["c"]["border-left-width"]; w != "9px" {
		t.Errorf("an author's border-left-width lost to the attribute: %q", w)
	}
	// And the half the author did not write still comes from the attribute.
	if w := got["t"]["border-top-width"]; w != "4px" {
		t.Errorf("the table border-top-width is %q, want 4px", w)
	}
}

// TestTheBodyLinkAttributeColoursTheLinks.
//
// "<body link=#800080>" is how a document set its link colour before there was
// a selector to say it with, and it is the second hint that is not an attribute
// of the element it styles: written once on the body, it applies to "any
// element that is a link" — the set :link selects, asked with the same function
// so that the two cannot come to differ.
func TestTheBodyLinkAttributeColoursTheLinks(t *testing.T) {
	for _, c := range []struct{ markup, want, what string }{
		// The initial colour, because nothing applied: these tests carry no
		// user agent sheet, so the blue a document really gets is layout's and
		// is checked there. What is asserted here is that the hint did not.
		{`<body><a id="c" href="x">x</a></body>`, "black", "no attribute"},
		{`<body link="red"><a id="c" href="x">x</a></body>`, "red",
			"the attribute"},
		{`<body link="#800080"><a id="c" href="x">x</a></body>`, "#800080",
			"a hash colour"},
		{`<body link="RED"><a id="c" href="x">x</a></body>`, "RED",
			"a colour keyword's case is the value's business"},
		{`<body link="red"><div><p><a id="c" href="x">x</a></p></div></body>`, "red",
			"a link deeper in the document"},
		{`<body link="red"><map><area id="c" href="x"/></map></body>`, "red",
			"an <area>, which is a link too"},
		// Not a link, so not coloured: the attribute is about links and an <a>
		// with no href is not one.
		{`<body link="red"><a id="c">x</a></body>`, "black",
			"an <a> with no href"},
		{`<body link="red"><span id="c">x</span></body>`, "black",
			"an element that is not a link at all"},
		// A value that is not a colour leaves the default standing, which is
		// the same answer as the attribute not being there.
		{`<body link="florb"><a id="c" href="x">x</a></body>`, "black",
			"a value that is not a colour"},
	} {
		got := computed(t, c.markup)
		if v := got["c"]["color"]; v != c.want {
			t.Errorf("%s: the colour is %q, want %q", c.what, v, c.want)
		}
	}
}

// TestVlinkAndAlinkAreNotColoursOnPaper states the narrowing, because it is a
// decision rather than an omission.
//
// "vlink" is the colour of a *visited* link and "alink" of one being clicked.
// Nothing here is either: :visited is answered no — see the note beside it in
// match.go — and there is no pointer to hold down on a printed page.
//
// They are not reported, for the reason this engine reports anything: a browser
// printing the same document shows an unvisited, unclicked link too, so there
// is no difference to tell an author about.
func TestVlinkAndAlinkAreNotColoursOnPaper(t *testing.T) {
	for _, attr := range []string{`vlink="red"`, `alink="red"`, `vlink="red" alink="green"`} {
		got, findings := styledLink(t, `<body `+attr+`><a id="c" href="x">x</a></body>`)
		if got != "black" {
			t.Errorf("<body %s> coloured an unvisited link %q", attr, got)
		}
		for _, f := range findings {
			if f.Property == "vlink" || f.Property == "alink" {
				t.Errorf("<body %s> reported %q; a browser printing this shows "+
					"the same colour", attr, f.Message)
			}
		}
	}
	// And "link" beside them still applies, so the refusal is about those two
	// rather than about the body's attributes.
	if got, _ := styledLink(t,
		`<body link="red" vlink="green" alink="blue"><a id="c" href="x">x</a></body>`); got != "red" {
		t.Errorf("the colour is %q, want red: link applies whatever sits beside it", got)
	}
}

// TestTheLinkColourIsAHint, which is where it sits in the cascade: above the
// default sheet's blue and below anything an author wrote.
func TestTheLinkColourIsAHint(t *testing.T) {
	got := computed(t, `<body link="red"><a id="c" href="x">x</a></body>`,
		author(t, `a { color: rgb(1, 2, 3) }`))
	if v := got["c"]["color"]; v != "rgb(1, 2, 3)" {
		t.Errorf("an author's colour lost to the attribute: %q", v)
	}
}

// styledLink applies the user agent sheet to a document and answers #c's colour
// and the findings, which is what the two tests above need and computed does
// not give.
func styledLink(t *testing.T, markup string) (string, []Finding) {
	t.Helper()
	doc := parseDoc(t, markup)
	got := Apply(doc, nil)
	for n, cs := range got.Styles {
		if n.Type == html.ElementNode {
			if id, _ := n.Attr("id"); id == "c" {
				return cs["color"], got.Findings
			}
		}
	}
	t.Fatal("no element with id c")
	return "", nil
}

// TestBgcolorOnEveryPartOfATable.
//
// HTML maps it on <body> and on the seven table parts in the same words, and it
// was on <body> alone. The note here said why: the cell backgrounds a table's
// bgcolor sets are painted by machinery that would have to agree with it, and
// the rule was one element at a time, each when it can be checked.
//
// It can be checked now. Every part of a table paints its own background — that
// was measured on the page before this was written — so "<table bgcolor=...>"
// and "<td bgcolor=...>", which is how every document of a certain age colours
// a table, mean something at last.
func TestBgcolorOnEveryPartOfATable(t *testing.T) {
	for _, c := range []struct{ markup, want string }{
		{`<table id="c" bgcolor="red"><tr><td>x</td></tr></table>`, "red"},
		{`<table><thead id="c" bgcolor="red"><tr><td>x</td></tr></thead></table>`, "red"},
		{`<table><tbody id="c" bgcolor="red"><tr><td>x</td></tr></tbody></table>`, "red"},
		{`<table><tfoot id="c" bgcolor="red"><tr><td>x</td></tr></tfoot></table>`, "red"},
		{`<table><tr id="c" bgcolor="red"><td>x</td></tr></table>`, "red"},
		{`<table><tr><td id="c" bgcolor="red">x</td></tr></table>`, "red"},
		{`<table><tr><th id="c" bgcolor="red">x</th></tr></table>`, "red"},
		{`<body id="c" bgcolor="red">x</body>`, "red"},
		{`<table><tr><td id="c" bgcolor="#808000">x</td></tr></table>`, "#808000"},
		// A value that is not a colour leaves the background alone, which is
		// the same answer as the attribute not being there.
		{`<table id="c" bgcolor="florb"><tr><td>x</td></tr></table>`, "transparent"},
		{`<table id="c"><tr><td>x</td></tr></table>`, "transparent"},
		// It is not inherited: a table's colour is the table's, and a cell that
		// wants one says so. background-color does not inherit, so this falls
		// out — and it is asserted because a hint written on the wrong element
		// would look like inheritance working.
		{`<table bgcolor="red"><tr><td id="c">x</td></tr></table>`, "transparent"},
	} {
		got := computed(t, c.markup)
		if v := got["c"]["background-color"]; v != c.want {
			t.Errorf("%s gave background-color %q, want %q", c.markup, v, c.want)
		}
	}
}

// TestACellsOtherHintsStillArrive is the merge, checked once more now that a
// cell has five hints on it: two of its own attributes, one of its table's, and
// two that are its own but read elsewhere.
func TestACellsOtherHintsStillArrive(t *testing.T) {
	got := computed(t, `<table cellpadding="7" border="1"><tr>`+
		`<td id="c" width="120" bgcolor="red" nowrap valign="top">x</td></tr></table>`)
	for property, want := range map[string]string{
		"width": "120px", "background-color": "red", "padding-top": "7px",
		"text-wrap-mode": "nowrap", "vertical-align": "top",
		"border-left-style": "inset",
	} {
		if v := got["c"][property]; v != want {
			t.Errorf("a cell carrying every hint at once gave %s %q, want %q",
				property, v, want)
		}
	}
}
