package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// A <textarea>'s content is its value, and an author's white-space does not
// collapse it.
//
// What the control holds is what the user typed and what the form would send:
// two spaces entered are two spaces, and a newline is a newline. So the
// collapsing half of the property does not reach it, while the wrapping half
// still does — "nowrap" changes how the value is shown and not one character of
// what it is.
//
// The rule *upgrades* the declared value rather than replacing it, which is what
// keeps "break-spaces" working: that value preserves spaces too, and adds that a
// space at the end of a line wraps rather than hangs. Replacing it with
// "preserve" — which a "!important" in the user-agent sheet would do, and which
// was the first attempt — throws that away, and the suite's two textarea
// documents then trade one for the other.

// textOf is what a control's lines actually read.
func textOf(t *testing.T, markup string) string {
	t.Helper()
	f := find(t, layoutOf(t, 800, markup, ``), "t")
	var b strings.Builder
	for _, ln := range f.Lines {
		for _, r := range ln.Runs {
			b.WriteString(r.Text)
		}
	}
	return b.String()
}

func TestATextareaKeepsTheSpacesItsValueHas(t *testing.T) {
	for _, value := range []string{
		"", "white-space: nowrap", "white-space: normal",
		"white-space: pre", "white-space: pre-line", "white-space: break-spaces",
		"white-space-collapse: collapse", "white-space-collapse: discard",
		"white-space-collapse: preserve-breaks",
	} {
		got := textOf(t, `<textarea id="t" style="`+value+`">a    b</textarea>`)
		if got != "a    b" {
			t.Errorf("%q: the textarea reads %q, want %q — its content is the "+
				"control's value and a stylesheet does not collapse it",
				value, got, "a    b")
		}
	}
}

// And the same for a newline, which "collapse" would turn into a space.
func TestATextareaKeepsTheLinesItsValueHas(t *testing.T) {
	for _, value := range []string{"", "white-space: nowrap", "white-space: normal"} {
		f := find(t, layoutOf(t,
			800, `<textarea id="t" style="`+value+`">a`+"\n"+`b</textarea>`, ``), "t")
		if len(f.Lines) < 2 {
			t.Errorf("%q: the textarea came out on %d line(s); the newline in its "+
				"value is a line break and not a space", value, len(f.Lines))
		}
	}
}

// TestATextareaUpgradesItsValueRatherThanReplacingIt is the rule the first
// attempt at this got wrong, stated where it can be seen.
//
// A "!important" in the user-agent sheet cannot be conditional: it replaces the
// author's value outright. That is wrong for "break-spaces", which already
// preserves every space and adds that one at the end of a line wraps rather than
// hanging — replacing it with "preserve" throws that away, and the suite's two
// textarea documents then trade one for the other with the count unmoved.
//
// So a value that already preserves is left exactly as it is, and only a value
// that would collapse is lifted.
func TestATextareaUpgradesItsValueRatherThanReplacingIt(t *testing.T) {
	area := &html.Node{Type: html.ElementNode, Name: "textarea"}
	text := &html.Node{Type: html.TextNode, Parent: area}
	para := &html.Node{Type: html.ElementNode, Name: "p"}
	inProse := &html.Node{Type: html.TextNode, Parent: para}

	for _, tc := range []struct {
		value, want, what string
	}{
		{"preserve", "preserve", "already keeps every space"},
		{"break-spaces", "break-spaces",
			"keeps them and wraps on them, and must not be flattened to preserve"},
		{"collapse", "preserve", "the initial value, which would fold them"},
		{"preserve-breaks", "preserve", "keeps the newlines and folds the spaces"},
		{"discard", "preserve", "would remove them outright"},
		{"BREAK-SPACES", "BREAK-SPACES", "matched case-insensitively and returned as written"},
		{"wibble", "preserve",
			"an invalid value, which the cascade has already made the initial one"},
	} {
		if got := preservedInAControl(text, tc.value); got != tc.want {
			t.Errorf("in a textarea %q became %q, want %q — %s",
				tc.value, got, tc.want, tc.what)
		}
		// And nothing outside a control is touched at all.
		if got := preservedInAControl(inProse, tc.value); got != tc.value {
			t.Errorf("in a <p> %q became %q; this rule is about a control's "+
				"value and prose is not one", tc.value, got)
		}
	}
}
