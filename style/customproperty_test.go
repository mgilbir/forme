package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// Custom properties, and the three different answers one feature used to get.
//
// "--c: red" was reported as an unimplemented property; "color: var(--c)" was
// dropped for not being a colour; "width: var(--w)" was kept verbatim. None of
// the three was marked unsupported — a name beginning with two dashes matched
// the vendor-prefix test, and a vendor prefix is another engine's spelling of
// something this engine does implement — so a page set in the wrong colour
// carried no claim that anything was missing from it, and the reftest ratchet
// counted it as clean.

// cascadeOf runs a two-origin cascade over one document and returns the
// computed styles and the findings.
func cascadeOf(t *testing.T, htmlSrc, authorCSS string) (Styled, *docFor) {
	t.Helper()
	ua, errs := css.ParseStylesheet(`div { display: block } p { display: block; color: black }`)
	if len(errs) != 0 {
		t.Fatalf("the user agent sheet reported %v", errs)
	}
	author, errs := css.ParseStylesheet(authorCSS)
	if len(errs) != 0 {
		t.Fatalf("the stylesheet %q reported %v", authorCSS, errs)
	}
	doc := parseDoc(t, htmlSrc)
	got := Apply(doc, []Sheet{
		{Origin: OriginUserAgent, Rules: ua},
		{Origin: OriginAuthor, Rules: author},
	})
	return got, &docFor{t: t, doc: doc, styles: got.Styles}
}

type docFor struct {
	t      *testing.T
	doc    *html.Node
	styles map[*html.Node]ComputedStyle
}

func (d *docFor) style(selector string) ComputedStyle {
	d.t.Helper()
	return d.styles[elementFor(d.t, d.doc, selector)]
}

// TestACustomPropertyIsReportedAsUnsupported is the first of the three answers.
func TestACustomPropertyIsReportedAsUnsupported(t *testing.T) {
	got, _ := cascadeOf(t, `<div id="a">x</div>`, `#a { --c: red; --d: 4px }`)
	var seen []Finding
	for _, f := range got.Findings {
		if strings.Contains(f.Message, "custom property") {
			seen = append(seen, f)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("two custom properties raised %d findings: %v", len(seen), got.Findings)
	}
	for _, f := range seen {
		if !f.Unsupported {
			t.Errorf("%q is not marked unsupported, so a page relying on it counts as clean",
				f.Message)
		}
	}
}

// TestAValueUsingACustomPropertyComputesToUnset is the other two answers, made
// one — and made the answer a browser gives.
//
// CSS Variables §3.3: a declaration whose value cannot be substituted is
// "invalid at computed-value time", which is not the same as an invalid
// declaration. It still wins the cascade, and it computes to "unset" — the
// parent's value for an inherited property, the initial value for every other
// one. Dropping it instead restores whatever the user agent sheet said, which
// is a third wrong answer.
func TestAValueUsingACustomPropertyComputesToUnset(t *testing.T) {
	got, doc := cascadeOf(t,
		`<div id="outer"><p id="inner">x</p></div>`,
		`#outer { color: rgb(0, 128, 0) }
		 #inner { color: var(--c); width: var(--w); display: var(--d) }`)

	// color inherits, so "unset" is the parent's green rather than the user
	// agent sheet's black.
	if c := doc.style("#inner")["color"]; !strings.Contains(c, "128") {
		t.Errorf("color computed to %q, want the inherited green: an unresolvable "+
			"value is unset, and color inherits", c)
	}
	// width does not inherit, so "unset" is its initial value.
	if w := doc.style("#inner")["width"]; w != "" && w != "auto" {
		t.Errorf("width computed to %q, want its initial value", w)
	}
	// display does not inherit either, so it is "inline" and not the user agent
	// sheet's "block" — which is what dropping the declaration would give.
	if d := doc.style("#inner")["display"]; d == "block" {
		t.Error("display computed to \"block\", which is the user agent sheet's answer; " +
			"the declaration was dropped rather than computed to unset")
	}

	for _, prop := range []string{"color", "width", "display"} {
		var found bool
		for _, f := range got.Findings {
			if f.Property == prop && f.Unsupported &&
				strings.Contains(f.Message, "custom property") {
				found = true
			}
		}
		if !found {
			t.Errorf("no unsupported finding names %q; the page is not what the "+
				"document asked for and nothing says so: %v", prop, got.Findings)
		}
	}
}

// TestACustomPropertyInsideAFunctionIsFound: a var() inside a calc() inside a
// shorthand is still a value this engine cannot know.
func TestACustomPropertyInsideAFunctionIsFound(t *testing.T) {
	for _, decl := range []string{
		"width: var(--w)",
		"width: calc(var(--w) + 1px)",
		"margin: 0 var(--m)",
		"background: url(x.png) var(--pos)",
		"font: var(--f)",
		"width: calc(calc(var(--w)))",
	} {
		got, _ := cascadeOf(t, `<div id="a">x</div>`, `#a { `+decl+` }`)
		var found bool
		for _, f := range got.Findings {
			if f.Unsupported && strings.Contains(f.Message, "custom property") {
				found = true
			}
		}
		if !found {
			t.Errorf("%q raised %v, want an unsupported finding naming the custom property",
				decl, got.Findings)
		}
	}
}

// TestAnOrdinaryDeclarationIsNotCalledACustomProperty is the control. A finding
// every page carries says nothing.
func TestAnOrdinaryDeclarationIsNotCalledACustomProperty(t *testing.T) {
	got, _ := cascadeOf(t, `<div id="a">x</div>`,
		`#a { color: red; width: calc(2px + 3px); background: url(x.png); font: 12px serif }`)
	for _, f := range got.Findings {
		if strings.Contains(f.Message, "custom property") {
			t.Errorf("an ordinary stylesheet raised %q", f.Message)
		}
	}
}

// TestTwoLeadingDashesIsNotAVendorPrefix pins the mistake underneath all of it.
// CSS Variables §2 reserves that shape for custom properties, and a vendor
// prefix means the opposite thing about a page: it is another engine's spelling
// of something this engine does implement, so a page missing it is not missing
// anything.
func TestTwoLeadingDashesIsNotAVendorPrefix(t *testing.T) {
	for _, name := range []string{"--c", "--main-colour", "--x"} {
		if vendorPrefixed(name) {
			t.Errorf("%q is treated as a vendor prefix", name)
		}
		if !isCustomProperty(name) {
			t.Errorf("%q is not treated as a custom property", name)
		}
	}
	for _, name := range []string{"-webkit-box-orient", "-moz-appearance", "-ms-filter", "-x-thing"} {
		if !vendorPrefixed(name) {
			t.Errorf("%q is not treated as a vendor prefix", name)
		}
		if isCustomProperty(name) {
			t.Errorf("%q is treated as a custom property", name)
		}
	}
}
