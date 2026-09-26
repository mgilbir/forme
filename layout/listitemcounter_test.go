package layout

import "testing"

// CSS Lists 3 §4.6: every list item increments the list-item counter, unless
// its own counter-increment names that counter — a box whose display includes
// list-item, whatever element or pseudo-element it is. See listItemIncrements.
func TestEveryListItemCountsItself(t *testing.T) {
	const show = `.i { list-style: none } .i::before { content: counter(list-item) "." }`
	for _, tc := range []struct {
		html, css, want, what string
	}{
		{`<ol><li class="i">a</li><li class="i">b</li></ol>`, show,
			"1.|a|2.|b", "an <li>"},
		{`<div style="counter-reset: list-item"><div class="i" style="display: list-item">a</div>` +
			`<div class="i" style="display: list-item">b</div><div class="i" style="display: list-item">c</div></div>`,
			show, "1.|a|2.|b|3.|c", "a div that is a list item"},
		{`<div><p class="i" style="display: inline list-item">a</p> <p class="i" style="display: inline list-item">b</p></div>`,
			show, "1.|a| |2.|b", "an inline list item, which counts though its marker is not drawn"},
		// An author's own counter-increment names another counter, and the
		// item still counts itself.
		{`<ol><li class="i">a</li><li class="i">b</li></ol>`,
			show + ` li { counter-increment: chapter }`, "1.|a|2.|b", "an increment of another counter"},
		// One that names list-item replaces the automatic one, by any amount.
		{`<ol><li class="i">a</li><li class="i">b</li></ol>`,
			show + ` li { counter-increment: list-item 5 }`, "5.|a|10.|b", "an increment of list-item"},
		{`<ol><li class="i">a</li><li class="i">b</li></ol>`,
			show + ` li { counter-increment: list-item 0 }`, "0.|a|0.|b", "an increment of list-item by nothing"},
		// A block that is not a list item does not count.
		{`<ol><li class="i">a</li><div class="i" style="display: block">x</div><li class="i">b</li></ol>`,
			show, "1.|a|1.|x|2.|b", "a block between two items"},
		// A reversed list of them counts down from how many there are.
		{`<div style="counter-reset: reversed(list-item)"><div class="i" style="display: list-item">a</div>` +
			`<div class="i" style="display: list-item">b</div></div>`, show, "2.|a|1.|b", "reversed"},
	} {
		if got := generatedText(t, tc.html, tc.css); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestAPseudoElementListItemCountsItself. A ::before whose display is
// list-item is a list item, and counts; its content sees its own increment,
// as an element's marker does.
func TestAPseudoElementListItemCountsItself(t *testing.T) {
	got := generatedText(t, `<div id="d"><p>a</p><p>b</p></div>`,
		`#d { counter-reset: list-item } p::before { display: list-item; list-style: none; content: counter(list-item) "." }`)
	if want := "1.|a|2.|b"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}
