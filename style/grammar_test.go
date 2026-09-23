package style

import (
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
)

// judge parses a value and asks the grammar about it.
func judge(t *testing.T, name, value string) verdict {
	t.Helper()
	vals, errs := css.ParseComponentValues(value)
	if len(errs) != 0 {
		t.Fatalf("%q did not tokenize: %v", value, errs)
	}
	return judgeValue(name, vals)
}

// TestEveryPropertyHasAGrammar is what keeps the table complete. A property
// registered without a grammar is one whose value nothing checks, so an invalid
// declaration of it wins the cascade — the gap the table exists to close — and
// a grammar for a name that is not registered is a check nothing asks.
func TestEveryPropertyHasAGrammar(t *testing.T) {
	var missing []string
	for name := range properties {
		if valueGrammars[name] == nil {
			missing = append(missing, name)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("registered with no value grammar: %s", strings.Join(missing, ", "))
	}
	for name := range valueGrammars {
		if _, ok := properties[name]; !ok {
			t.Errorf("a grammar for %q, which is not a registered property", name)
		}
	}
}

// TestEveryInitialValueIsValid: a property's own initial value, written as a
// declaration, is one it takes. It is the cheapest check that a grammar was
// written against the right property.
func TestEveryInitialValueIsValid(t *testing.T) {
	for name, p := range properties {
		if got := judge(t, name, p.initial); !got.ok || got.unsupported != "" {
			t.Errorf("%s: its initial value %q judged %+v", name, p.initial, got)
		}
	}
}

// TestTheValueGrammarAgreesWithTheSpecifications is a sample of each shape the
// table has to tell apart, one or more per property family, written from the
// specifications' value definitions and not from the grammar.
func TestTheValueGrammarAgreesWithTheSpecifications(t *testing.T) {
	type c struct{ name, value string }
	validCases := []c{
		{"width", "100px"}, {"width", "50%"}, {"width", "calc(100% - 2em)"},
		{"width", "fit-content(20px)"}, {"width", "max-content"}, {"height", "0"},
		{"max-width", "none"}, {"min-height", "auto"},
		{"margin-top", "-10px"}, {"margin-left", "auto"}, {"padding-top", "1em"},
		{"top", "auto"}, {"left", "-5%"}, {"z-index", "-3"}, {"order", "2"},
		{"float", "inline-start"}, {"clear", "both"}, {"position", "sticky"},
		{"border-top-width", "thin"}, {"border-top-width", "2px"},
		{"border-left-style", "dashed"}, {"outline-style", "auto"},
		{"color", "red"}, {"color", "#abc"}, {"color", "rgb(1 2 3 / 50%)"},
		{"color", "currentcolor"}, {"outline-color", "invert"},
		{"font-family", `"Helvetica Neue", Arial, sans-serif`},
		{"font-family", "Times New Roman, serif"},
		{"font-size", "larger"}, {"font-size", "12pt"}, {"font-size", "150%"},
		{"font-size", "5vw"}, {"font-weight", "450"}, {"font-weight", "bolder"},
		{"font-style", "oblique 10deg"}, {"font-style", "italic"},
		{"font-variant-numeric", "oldstyle-nums tabular-nums"},
		{"font-feature-settings", `"liga" 0, "smcp"`}, {"line-height", "1.5"},
		{"line-height", "normal"}, {"line-height", "20px"},
		{"letter-spacing", "-0.05em"}, {"text-indent", "2em hanging each-line"},
		{"text-transform", "uppercase full-width"}, {"text-align-all", "justify"},
		{"vertical-align", "-2px"}, {"vertical-align", "text-top"},
		{"text-decoration-line", "underline overline"},
		{"word-space-transform", "auto-phrase space"},
		{"hanging-punctuation", "first allow-end last"},
		{"hyphenate-limit-chars", "6 2 auto"}, {"tab-size", "4"}, {"tab-size", "2em"},
		{"text-fit", "grow per-line 80%"}, {"line-clamp", "3"},
		{"line-clamp", `2 "…"`}, {"writing-mode", "tb-rl"},
		{"text-combine-upright", "digits 2"},
		{"flex-basis", "content"}, {"flex-basis", "30%"}, {"flex-grow", "1.5"},
		{"justify-content", "safe center"}, {"justify-content", "space-between"},
		{"align-items", "last baseline"}, {"align-self", "auto"},
		{"justify-items", "legacy center"}, {"justify-items", "right legacy"},
		{"grid-template-columns", "[a] 1fr repeat(2, minmax(10px, 1fr)) [b]"},
		{"grid-template-columns", "repeat(auto-fill, 100px)"},
		{"grid-template-areas", `"a b" "c d"`}, {"grid-auto-flow", "column dense"},
		{"grid-column-start", "span name"}, {"grid-row-start", "-1"},
		{"column-count", "3"}, {"column-gap", "normal"}, {"column-width", "10em"},
		{"content", `"a" counter(c) open-quote attr(title)`},
		{"content", `url(x.png) / "alt"`}, {"quotes", `"«" "»"`},
		{"counter-reset", "a 1 b reversed(c)"}, {"counter-increment", "list-item -1"},
		{"list-style-type", "lower-roman"}, {"list-style-type", `"-"`},
		{"background-image", "url(a.png), linear-gradient(red, blue)"},
		{"background-repeat", "no-repeat, repeat-x"},
		{"background-position", "right 10px bottom 20%, center"},
		{"background-position", "top left"}, {"background-position", "center left"},
		{"background-size", "cover, 50% auto"},
		{"background-clip", "text"}, {"border-spacing", "2px 4px"},
		{"clip", "rect(1px, auto, 3px, 4px)"}, {"clip", "rect(1px auto 3px 4px)"},
		{"opacity", "50%"}, {"object-position", "left 10px top"},
		{"aspect-ratio", "16 / 9"}, {"aspect-ratio", "auto 1"},
		{"display", "inline flex"},
		{"margin-inline-start", "3px"}, {"border-inline-start-color", "green"},
	}
	for _, tc := range validCases {
		if got := judge(t, tc.name, tc.value); !got.ok || got.unsupported != "" {
			t.Errorf("%s: %s judged %+v, want valid", tc.name, tc.value, got)
		}
	}

	invalidCases := []c{
		{"width", "foo"}, {"width", "10"}, {"width", "-1px"}, {"width", "1foo"},
		{"height", "10px 20px"}, {"margin-top", "10"}, {"margin-top", "red"},
		{"padding-left", "-1em"}, {"float", "sideways"}, {"position", "bogus"},
		{"z-index", "1.5"}, {"border-top-width", "10%"}, {"border-top-style", "wavy"},
		{"outline-style", "hidden"}, {"color", "'red'"}, {"color", "rgb(foo)"},
		{"color", "nonesuch"}, {"font-size", "foo"}, {"font-size", "-2px"},
		{"font-weight", "1001"}, {"font-weight", "0"}, {"font-style", "oblique 100deg"},
		{"line-height", "foo"}, {"line-height", "-1"}, {"opacity", "red"},
		{"font-feature-settings", `"toolong"`}, {"text-indent", "hanging"},
		{"text-transform", "uppercase lowercase"}, {"vertical-align", "centre"},
		{"text-decoration-line", "underline underline"}, {"tab-size", "-1"},
		{"flex-grow", "-1"}, {"column-count", "0"}, {"order", "1.5"},
		{"justify-content", "safe space-between"}, {"align-items", "left"},
		{"grid-template-columns", "1fr foo(1)"}, {"grid-row-start", "span -1"},
		{"grid-row-end", "0"},
		{"content", "12px"}, {"content", "foo"}, {"quotes", `"a"`},
		{"counter-reset", "none a"}, {"background-repeat", "repeat-x no-repeat"},
		{"background-position", "left left"}, {"background-size", "-1px"},
		{"border-spacing", "1px 2px 3px"}, {"clip", "rect(1px, 2px, 3px)"},
		{"display", "absolute"}, {"visibility", "gone"}, {"word-break", "loose"},
		{"width", "calc(1px + 2)"}, {"width", "calc(1px * 2px)"},
		{"margin-inline-start", "wide"}, {"border-inline-start-color", "'x'"},
		{"width", "-webkit-fill-available"},
	}
	for _, tc := range invalidCases {
		if got := judge(t, tc.name, tc.value); got.ok {
			t.Errorf("%s: %s judged %+v, want invalid", tc.name, tc.value, got)
		}
	}

	// Valid, and naming something this engine does not evaluate.
	unevaluatedCases := []struct{ name, value, what string }{
		{"color", "oklch(0.6 0.2 140)", "oklch()"}, {"color", "lab(50% 40 59)", "lab()"},
		{"color", "hwb(120 0% 0%)", "hwb()"}, {"color", "color(srgb 1 0 0)", "color()"},
		{"color", "color-mix(in srgb, red, blue)", "color-mix()"},
		{"color", "Canvas", "the system colour Canvas"},
		{"color", "rgb(calc(255) 0 0)", "rgb()"}, {"color", "light-dark(red, blue)", "light-dark()"},
		{"width", "min(10px, 50%)", "min()"}, {"width", "clamp(1px, 2vw, 3px)", "clamp()"},
		{"width", "calc(min(1px, 2px) + 1px)", "calc()"}, {"width", "10lh", "the unit lh"},
		{"line-height", "calc(1.5)", "calc()"}, {"z-index", "calc(2)", "calc()"},
		{"margin-top", "env(safe-area-inset-top)", "env()"},
		{"border-inline-start-color", "oklch(0.5 0.1 20)", "oklch()"},
	}
	for _, tc := range unevaluatedCases {
		if got := judge(t, tc.name, tc.value); !got.ok || got.unsupported != tc.what {
			t.Errorf("%s: %s judged %+v, want valid and naming %q", tc.name, tc.value,
				got, tc.what)
		}
	}
}
