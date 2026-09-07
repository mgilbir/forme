package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// cascadeWith runs a cascade over three origins and returns one element's
// computed style.
func cascadeWith(t *testing.T, htmlSrc, ua, author string) ComputedStyle {
	t.Helper()
	var sheets []Sheet
	for _, s := range []struct {
		origin Origin
		src    string
	}{{OriginUserAgent, ua}, {OriginAuthor, author}} {
		if s.src == "" {
			continue
		}
		rules, errs := css.ParseStylesheet(s.src)
		if len(errs) != 0 {
			t.Fatalf("the %v sheet reported %v", s.origin, errs)
		}
		sheets = append(sheets, Sheet{Origin: s.origin, Rules: rules})
	}
	doc := parseDoc(t, htmlSrc)
	got := Apply(doc, sheets)
	return got.Styles[elementFor(t, doc, "#a")]
}

// TestAnImportantStyleAttributeBeatsAnImportantRule is Cascade 4 §3.1.
//
// A style attribute is an author declaration whose specificity is above every
// selector, so it is decided by the same two terms every other declaration is.
// Importance is the first and inverts the origins; the specificity is the
// second and the attribute always wins it. This was read as "inline wins unless
// the author rule is important", which says the opposite about the one case
// authors write "!important" in a style attribute for.
func TestAnImportantStyleAttributeBeatsAnImportantRule(t *testing.T) {
	got := cascadeWith(t,
		`<p id="a" style="color: rgb(255, 0, 0) !important">x</p>`, "",
		`#a { color: rgb(0, 0, 255) !important }`)
	if c := got["color"]; !strings.Contains(c, "255, 0, 0") {
		t.Errorf("the colour is %q, want the red the attribute asked for", c)
	}
}

// TestAnOrdinaryStyleAttributeStillLosesToAnImportantRule is the half that was
// right, kept: importance is the higher sort key, so a normal declaration loses
// to an important one however specific it is.
func TestAnOrdinaryStyleAttributeStillLosesToAnImportantRule(t *testing.T) {
	got := cascadeWith(t,
		`<p id="a" style="color: rgb(255, 0, 0)">x</p>`, "",
		`#a { color: rgb(0, 0, 255) !important }`)
	if c := got["color"]; !strings.Contains(c, "0, 0, 255") {
		t.Errorf("the colour is %q, want the blue the important rule asked for", c)
	}
}

// TestAnImportantStyleAttributeStillLosesToTheUserAgent is the other end of the
// same order: importance inverts the origins, so an important user-agent
// declaration outranks an important author one however it was written.
func TestAnImportantStyleAttributeStillLosesToTheUserAgent(t *testing.T) {
	got := cascadeWith(t,
		`<p id="a" style="color: rgb(255, 0, 0) !important">x</p>`,
		`p { color: rgb(0, 128, 0) !important }`, "")
	if c := got["color"]; !strings.Contains(c, "0, 128, 0") {
		t.Errorf("the colour is %q, want the green the user agent insisted on", c)
	}
}

// TestAnOrdinaryStyleAttributeBeatsAnOrdinaryRule is the everyday case, which
// must not move.
func TestAnOrdinaryStyleAttributeBeatsAnOrdinaryRule(t *testing.T) {
	got := cascadeWith(t,
		`<p id="a" style="color: rgb(255, 0, 0)">x</p>`, "",
		`#a { color: rgb(0, 0, 255) }`)
	if c := got["color"]; !strings.Contains(c, "255, 0, 0") {
		t.Errorf("the colour is %q, want the red the attribute asked for", c)
	}
}

// TestCurrentColorOnColorItselfInherits is CSS Color 4 §7.2: "if the
// 'currentcolor' keyword is set on the 'color' property itself, it is treated
// as 'color: inherit'".
//
// It was answered at paint time, where there is no parent to ask, and came out
// as the initial value: black, on a paragraph inside a green div that had asked
// for the green.
func TestCurrentColorOnColorItselfInherits(t *testing.T) {
	got := cascadeWith(t,
		`<div id="d" style="color: rgb(0, 128, 0)"><p id="a" style="color: currentcolor">x</p></div>`,
		"", "")
	if c := got["color"]; !strings.Contains(c, "0, 128, 0") {
		t.Errorf("the colour is %q, want the green it inherits", c)
	}
}

// TestCurrentColorElsewhereIsStillTheElementsOwnColour is the control: on every
// other property "currentcolor" means this element's colour, and resolving it
// as an inherit would take the parent's.
func TestCurrentColorElsewhereIsStillTheElementsOwnColour(t *testing.T) {
	got := cascadeWith(t,
		`<div id="d" style="color: rgb(0, 128, 0)">`+
			`<p id="a" style="color: rgb(255, 0, 0); border-top-color: currentcolor">x</p></div>`,
		"", "")
	if c := got["border-top-color"]; !strings.EqualFold(strings.TrimSpace(c), "currentcolor") {
		t.Errorf("the border colour computed to %q; it is resolved where it is drawn", c)
	}
	if c := got["color"]; !strings.Contains(c, "255, 0, 0") {
		t.Errorf("the colour is %q, want the element's own red", c)
	}
}

// TestANegativeValueIsDroppedWhateverTheNameItWasWrittenUnder is §4.2 over the
// properties the list did not have.
//
// A declaration whose value is illegal is not a declaration with a strange
// value: the whole declaration is dropped and what stands is whatever the
// cascade would have produced without it. The logical longhands were never
// looked at — the rename to a physical name happens per element, several steps
// later — and neither were the properties added after the list was written.
//
// The cost is not that the negative was drawn. It is that it was kept: layout
// could make nothing of "flex-grow: -1", fell back to the initial value of
// zero, and the author's earlier "flex-grow: 2" was lost.
func TestANegativeValueIsDroppedWhateverTheNameItWasWrittenUnder(t *testing.T) {
	for _, tc := range []struct{ name, decl, prop, want string }{
		{"a logical size", "inline-size: 8px; inline-size: -8px", "width", "8px"},
		{"a logical padding",
			"padding-inline-start: 8px; padding-inline-start: -8px", "padding-left", "8px"},
		{"a logical padding shorthand",
			"padding-inline: 8px; padding-inline: -8px", "padding-left", "8px"},
		{"a logical border width",
			"border-inline-start-width: 8px; border-inline-start-width: -8px",
			"border-left-width", "8px"},
		{"flex-grow", "flex-grow: 2; flex-grow: -1", "flex-grow", "2"},
		{"flex-shrink", "flex-shrink: 2; flex-shrink: -1", "flex-shrink", "2"},
		{"flex-basis", "flex-basis: 8px; flex-basis: -8px", "flex-basis", "8px"},
		{"column-count", "column-count: 2; column-count: -1", "column-count", "2"},
		{"column-width", "column-width: 8px; column-width: -8px", "column-width", "8px"},
		{"a gap", "gap: 8px; gap: -8px", "row-gap", "8px"},
	} {
		got := cascadeWith(t, `<p id="a">x</p>`, "", `#a { `+tc.decl+` }`)
		if v := strings.TrimSpace(got[tc.prop]); v != tc.want {
			t.Errorf("%s: %s computed to %q, want %q — the negative declaration is "+
				"dropped and the one before it stands", tc.name, tc.prop, v, tc.want)
		}
	}
}

// TestALegalNegativeIsStillLegal is the control, and it is the reason the list
// is a list. A negative margin, text-indent, letter-spacing or background
// position is legal and useful, and dropping one would break a page doing
// nothing wrong.
func TestALegalNegativeIsStillLegal(t *testing.T) {
	for _, tc := range []struct{ name, decl, prop string }{
		{"a margin", "margin-left: -8px", "margin-left"},
		{"a logical margin", "margin-inline-start: -8px", "margin-left"},
		{"a logical inset", "inset-inline-start: -8px", "left"},
		{"a text indent", "text-indent: -8px", "text-indent"},
		{"letter-spacing", "letter-spacing: -1px", "letter-spacing"},
		{"an order", "order: -1", "order"},
	} {
		got := cascadeWith(t, `<p id="a">x</p>`, "", `#a { `+tc.decl+` }`)
		if v := strings.TrimSpace(got[tc.prop]); !strings.Contains(v, "-") {
			t.Errorf("%s: %s computed to %q; a negative value is legal here",
				tc.name, tc.prop, v)
		}
	}
}

var _ = html.ElementNode
