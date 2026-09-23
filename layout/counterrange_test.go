package layout

import (
	"strings"
	"testing"
)

// Counter values are integers in a 32-bit range (audit C128), and every way a
// counter is given a value keeps it there — so an increment can never lower
// one, and no number is converted to an int beyond what an int holds.
func TestACounterValueIsAnIntegerInRange(t *testing.T) {
	for _, tc := range []struct {
		css  string
		want string
	}{
		// Saturated at the reset, so the increment has nowhere further to go
		// rather than being clamped from a value above it.
		{`#d { counter-reset: c 5000000000; counter-increment: c }`, "2147483647"},
		{`#d { counter-reset: c -5000000000; counter-increment: c -1 }`, "-2147483648"},
		{`#d { counter-set: c 5000000000 }`, "2147483647"},
		{`#d { counter-reset: c 1; counter-increment: c 5000000000 }`, "2147483647"},
		// Not an integer: the cascade's grammar refuses the declaration, so
		// nothing is reset and the increment starts the counter at nought.
		{`#d { counter-reset: c 2.7; counter-increment: c }`, "1"},
		{`#d { counter-reset: c 1e30; counter-increment: c }`, "1"},
	} {
		got := generatedText(t, `<div id="d"><span id="s"></span></div>`,
			tc.css+` #s::before { content: counter(c) }`)
		// A run boundary is not part of the number: a minus sign may be set
		// as a run of its own.
		got = strings.ReplaceAll(got, "|", "")
		if got != tc.want {
			t.Errorf("%s: the counter reads %q, want %q", tc.css, got, tc.want)
		}
	}
}

// TestTheCounterListReaderRefusesWhatIsNotAnInteger holds parseCounterList to
// the grammar on its own, for a value that reaches it by a path other than the
// cascade's check: every such path found today is refused before it gets here,
// and the reader does not rely on that.
func TestTheCounterListReaderRefusesWhatIsNotAnInteger(t *testing.T) {
	for _, raw := range []string{"c 2.7", "c 1e30", "c 2.0", "c 1 d 0.5"} {
		if got := parseCounterList(raw, 0); got != nil {
			t.Errorf("%q read as %v; a number that is not an integer makes the list invalid", raw, got)
		}
	}
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{"c 5000000000", 1<<31 - 1},
		{"c -5000000000", -(1 << 31)},
		{"c 7", 7},
	} {
		got := parseCounterList(tc.raw, 0)
		if len(got) != 1 || got[0].value != tc.want {
			t.Errorf("%q read as %v, want the value %d", tc.raw, got, tc.want)
		}
	}
}
