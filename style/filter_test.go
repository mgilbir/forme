package style

import "testing"

// TestTheFilterGrammarIsFilterEffects checks Filter Effects 1 §5 and §6.1's
// grammar: every function is valid CSS whether or not layout applies it, and
// what is not CSS is dropped so that the declaration before it stands.
func TestTheFilterGrammarIsFilterEffects(t *testing.T) {
	for _, v := range []string{
		"none", "blur(2px)", "blur()", "blur(0)", "opacity(50%)", "opacity(0.3)", "opacity()",
		"brightness(2)", "contrast(150%)", "grayscale(1)", "invert(0.5)", "saturate(3)",
		"sepia(100%)", "hue-rotate(90deg)", "hue-rotate(0)", "drop-shadow(2px 3px)",
		"drop-shadow(red 1px 2px 3px)", "drop-shadow(1px 2px 3px red)", "url(#f)",
		"blur(1px) opacity(0.5) url(x.svg#f)",
	} {
		if got := expandOf(t, "filter: blur(9px); filter: "+v).Get("filter"); got == "blur(9px)" {
			t.Errorf("%q was dropped", v)
		}
	}
	for _, v := range []string{
		"blur(-1px)", "blur(10%)", "blur(1px 2px)", "opacity(-1)", "hue-rotate(1px)",
		"none blur(1px)", "blur", "drop-shadow(1px)", "drop-shadow(1px 2px -3px)",
		"shake(2px)", "blur(1px),opacity(1)",
	} {
		if got := expandOf(t, "filter: blur(9px); filter: "+v).Get("filter"); got != "blur(9px)" {
			t.Errorf("%q was kept as %q", v, got)
		}
	}
}

// TestTheTextShadowGrammarIsTextDecoration3 checks §4's "none | [ <color>? &&
// [ <length>{2} <length [0,∞]>? ] ]#".
func TestTheTextShadowGrammarIsTextDecoration3(t *testing.T) {
	for _, v := range []string{
		"none", "1px 2px", "1px 2px 3px", "red 1px 2px", "1px 2px 3px red",
		"1px 2px, red 3px 4px 5px", "-1px -2px", "currentcolor 0 0",
	} {
		if got := expandOf(t, "text-shadow: 9px 9px; text-shadow: "+v).Get("text-shadow"); got == "9px 9px" {
			t.Errorf("%q was dropped", v)
		}
	}
	for _, v := range []string{
		"1px", "1px 2px 3px 4px", "1px 2px -3px", "red 1px red 2px", "1px red 2px",
		"none, 1px 2px", "1px 2px,", "inset 1px 2px", "1px 2px 3px 4px red",
	} {
		if got := expandOf(t, "text-shadow: 9px 9px; text-shadow: "+v).Get("text-shadow"); got != "9px 9px" {
			t.Errorf("%q was kept as %q", v, got)
		}
	}
	// And it inherits.
	doc := parseDoc(t, `<p id="t"><span id="s">x</span></p>`)
	got := Apply(doc, []Sheet{author(t, "#t { text-shadow: red 1px 1px }")})
	if v := got.Styles[elementFor(t, doc, "#s")].Get("text-shadow"); v == "" || v == "none" {
		t.Errorf("a child's text-shadow is %q, want the parent's", v)
	}
}
