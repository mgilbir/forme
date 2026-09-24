package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// ASCII case-insensitive means ASCII, in every place a stylesheet is read.
//
// CSS Syntax 3 §2.1: keywords, property names, at-rule names, pseudo-classes,
// units, function names, media features and named colours are compared ASCII
// case-insensitively, which folds A–Z to a–z and nothing else. They were
// compared with strings.EqualFold and strings.ToLower, which are Unicode's:
// U+212A KELVIN SIGN lowercases to "k", U+017F LATIN SMALL LETTER LONG S folds
// to "s", so "blac\u212A" was black and "ſolid" was solid. The third lookalike,
// U+0130 LATIN CAPITAL LETTER I WITH DOT ABOVE, lowercases to two code points
// and so never matched anything; it is here because it is the one that changes
// a string's length, and nothing here may depend on that either.
//
// Each case is written twice: the lookalike, which must not be read as the
// keyword, and the keyword in ASCII capitals, which must — so a check that
// refused everything would fail as surely as one that folded too much.

const (
	kelvin  = "\u212A" // KELVIN SIGN
	longS   = "ſ"      // LATIN SMALL LETTER LONG S
	dottedI = "İ"      // LATIN CAPITAL LETTER I WITH DOT ABOVE
)

func TestAKeywordIsFoldedAsASCII(t *testing.T) {
	for _, tc := range []struct{ what, sheet, selector, property, want string }{
		// Keywords, through the grammar that validates them and the computed
		// value that reads them.
		{"a keyword with a long s", `#p { border-top-style: dotted; border-top-style: ` + longS + `olid }`,
			"#p", "border-top-style", "dotted"},
		{"a keyword in ASCII capitals", `#p { border-top-style: dotted; border-top-style: SOLID }`,
			"#p", "border-top-style", "SOLID"},
		{"a keyword with a KELVIN SIGN", `#g { display: bloc` + kelvin + ` }`,
			"#g", "display", "inline"},
		{"that keyword in capitals", `#g { display: BLOCK }`, "#g", "display", "BLOCK"},
		{"a keyword with a dotted I", `#g { display: INLINE; display: BLOCK; display: ` + dottedI + `NLINE }`,
			"#g", "display", "BLOCK"},
		// Property names.
		{"a property name with a KELVIN SIGN", `#p { word-brea` + kelvin + `: break-all }`,
			"#p", "word-break", "normal"},
		{"a property name with a long s", `#p { font-` + longS + `ize: 20px }`,
			"#p", "font-size", "16px"},
		{"a property name in capitals", `#p { WORD-BREAK: break-all }`, "#p", "word-break", "break-all"},
		// Named colours.
		{"a colour name with a KELVIN SIGN", `#p { color: red; color: blac` + kelvin + ` }`,
			"#p", "color", "red"},
		{"a colour name in capitals", `#p { color: red; color: BLACK }`, "#p", "color", "BLACK"},
		// At-rule names.
		{"an at-rule name with a long s", `#p { color: red } @` + longS + `upports (display: block) { #p { color: blue } }`,
			"#p", "color", "red"},
		{"an at-rule name in capitals", `#p { color: red } @SUPPORTS (display: block) { #p { color: blue } }`,
			"#p", "color", "blue"},
		{"an at-rule name with a dotted I", `#p { color: red } @MED` + dottedI + `A print { #p { color: blue } }`,
			"#p", "color", "red"},
		{"that at-rule in capitals", `#p { color: red } @MEDIA print { #p { color: blue } }`,
			"#p", "color", "blue"},
		// Media features and their values.
		{"a media feature value with a long s", `#p { color: red } @media (orientation: land` + longS + `cape), (orientation: ` + longS + `quare) { #p { color: blue } }`,
			"#p", "color", "red"},
		{"a media feature in capitals", `#p { color: red } @media (ORIENTATION: PORTRAIT) { #p { color: blue } }`,
			"#p", "color", "blue"},
		// Units.
		{"a unit with a dotted I", `#p { margin-left: 7px; margin-left: 1` + dottedI + `N }`,
			"#p", "margin-left", "7px"},
		{"a unit in capitals", `#p { margin-left: 7px; margin-left: 1IN }`, "#p", "margin-left", "1IN"},
		// Function names.
		{"a function name with a long s", `#p { list-style-type: square; list-style-type: ` + longS + `ymbols(cyclic "*") }`,
			"#p", "list-style-type", "square"},
		{"a function name in capitals", `#p { list-style-type: square; list-style-type: SYMBOLS(cyclic "*") }`,
			"#p", "list-style-type", `SYMBOLS(cyclic "*")`},
	} {
		doc := parseDoc(t, nested)
		got := styleOf(t, doc, []Sheet{author(t, tc.sheet)}, tc.selector, tc.property)
		if got != tc.want {
			t.Errorf("%s: %s gave %s %q, want %q", tc.what, tc.sheet, tc.property, got, tc.want)
		}
	}
}

// TestASelectorIsFoldedAsASCII is the selector half: pseudo-class names,
// attribute names and element names.
func TestASelectorIsFoldedAsASCII(t *testing.T) {
	doc := parseDoc(t, `<a id="l" href="x">l</a><kbd id="k">k</kbd>`+
		`<track id="t" kind="subtitles"/><span id="s">s</span><math-α id="m">m</math-α>`)
	check(t, doc, map[string]string{
		// Element names. A name outside ASCII matches itself and no other.
		"KBD":                                 "k",
		kelvin + "bd":                         "",
		"SPAN":                                "s",
		longS + "pan":                         "",
		"MATH-α":                              "m",
		"math-Α":                              "",
		"a:LINK":                              "l",
		"track[KIND]":                         "t",
		"track[" + kelvin + "ind]":            "",
		`track[kind=SUBTITLES i]`:             "t",
		`track[kind=` + longS + `ubtitles i]`: "",
	})
	// A pseudo-class this engine does not know is refused, and a lookalike of
	// one it does know is one it does not.
	for _, sel := range []string{"a:lin" + kelvin, "a:" + longS + "cope", "a:L" + dottedI + "NK"} {
		vals, _ := css.ParseComponentValues(sel)
		if _, _, ok := css.ParseSelectorList(vals); ok {
			t.Errorf("%q was read as a selector; its pseudo-class is not one CSS names", sel)
		}
	}
	// The rules the cascade walks for an element come from an index keyed by
	// the same fold, so a lookalike rule is not applied either, and a name
	// outside ASCII, which the index does not file, still reaches its element.
	got := styleOf(t, doc, []Sheet{author(t, `kbd { color: red } `+kelvin+`BD { color: blue }`)},
		"#k", "color")
	if got != "red" {
		t.Errorf("a type selector with a KELVIN SIGN reached <kbd>: color %q", got)
	}
	got = styleOf(t, doc, []Sheet{author(t, `math-α { color: red } MATH-α { color: blue }`)},
		"#m", "color")
	if got != "blue" {
		t.Errorf("a type selector outside ASCII did not reach its element: color %q", got)
	}
}

// TestALegacyColourNameIsFoldedAsASCII is HTML's half of the colours: the
// rules for parsing a legacy colour value compare a named colour ASCII
// case-insensitively, and a name that is not one goes on to the hex fallback.
func TestALegacyColourNameIsFoldedAsASCII(t *testing.T) {
	doc := parseDoc(t, `<table><tr><td id="a" bgcolor="BLACK">a</td>`+
		`<td id="b" bgcolor="blac`+kelvin+`">b</td></tr></table>`)
	got := Apply(doc, nil)
	a := got.Styles[elementFor(t, doc, "#a")].Get("background-color")
	b := got.Styles[elementFor(t, doc, "#b")].Get("background-color")
	if a != "BLACK" {
		t.Errorf("bgcolor=BLACK gave %q, want the name", a)
	}
	// Unicode's folding on purpose: the name was once passed through as
	// written, "blac\u212A", once it had matched, and only the broader fold
	// sees that as the name it only looks like.
	if strings.EqualFold(b, "black") || strings.TrimSpace(b) == "" {
		t.Errorf("bgcolor with a KELVIN SIGN gave %q, the colour of the name it only looks like", b)
	}
}
