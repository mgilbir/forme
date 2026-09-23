package style

import (
	"strings"
	"testing"
)

// The integer attributes a presentational hint reads are read by HTML's own
// rules, §2.3.4.1 and §2.3.4.2, through html.ParseInteger and
// html.ParseNonNegativeInteger: the digits at the front are the value, and a
// run of them too long for any page saturates rather than being refused.
func TestHintIntegersAreReadByHTMLsRules(t *testing.T) {
	long := strings.Repeat("9", 40)
	for _, tc := range []struct{ doc, id, property, want string }{
		// <li value>, §2.3.4.1: a sign, then the leading digits.
		{`<ol><li id="e" value="7px">x</li></ol>`, "e", "counter-set", "list-item 7"},
		{`<ol><li id="e" value="-3.5">x</li></ol>`, "e", "counter-set", "list-item -3"},
		{`<ol><li id="e" value="` + long + `">x</li></ol>`, "e", "counter-set", "list-item 2147483647"},
		// <ol start>, the same rule, made a counter-reset one below it.
		{`<ol id="e" start="3px"><li>x</li></ol>`, "e", "counter-reset", "list-item 2"},
		// <table border>, §2.3.4.2: saturated past ten digits, not the
		// one-pixel default the refusal gave.
		{`<table id="e" border="` + long + `"><tr><td>x</td></tr></table>`, "e",
			"border-top-style", "outset"},
	} {
		got := computed(t, tc.doc)
		if v := got[tc.id].Get(tc.property); v != tc.want {
			t.Errorf("%s: %s is %q, want %q", tc.doc, tc.property, v, tc.want)
		}
	}
	got := computed(t, `<table id="e" border="`+long+`"><tr><td>x</td></tr></table>`)
	if w := got["e"].Get("border-top-width"); w == "1px" {
		t.Errorf("a border of forty nines was read as the one-pixel default")
	}
	// <hr size>, §2.3.4.2 as well: the leading digits, and saturation past
	// ten of them rather than no size at all.
	got = computed(t, `<hr id="e" size="6px" noshade>`)
	if w := got["e"].Get("border-top-width"); w != "3px" {
		t.Errorf("<hr size=6px noshade> gave border-top-width %q, want 3px", w)
	}
	plain := computed(t, `<hr id="e" noshade>`)["e"].Get("border-top-width")
	got = computed(t, `<hr id="e" size="`+long+`" noshade>`)
	if w := got["e"].Get("border-top-width"); w == plain {
		t.Errorf("<hr size=(forty nines) noshade> was read as no size (%q)", w)
	}
}
