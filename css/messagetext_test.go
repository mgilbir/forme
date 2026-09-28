package css

import (
	"testing"

	"github.com/mgilbir/forme/internal/diag"
)

// A message quotes the stylesheet, and is made text wherever it is written.
//
// The tokenizer decodes the input, so a byte that begins no UTF-8 character
// never reaches a message; a control character does. "a\1 b" is an identifier
// holding U+0001, and a raw control byte is a delimiter token that a value or a
// selector quotes as it stands. html's reader found the same fault with bytes
// that are not UTF-8: see html/messagetext_test.go and internal/diag.

// TestASelectorMessageIsText is the selector parser's messages, which quote a
// pseudo-class's name as the stylesheet wrote it.
func TestASelectorMessageIsText(t *testing.T) {
	for _, tc := range []struct{ sheet, want string }{
		{`p:a\1 b{}`, `the pseudo-class ":a?b" is not implemented`},
		{`p:a\85 {}`, `the pseudo-class ":a?" is not implemented`},
		{`p::a\1b{}`, `the pseudo-element ::a? is not implemented`},
	} {
		rules, _ := ParseStylesheet(tc.sheet)
		if len(rules) != 1 {
			t.Fatalf("%q: %d rules", tc.sheet, len(rules))
		}
		_, errs, _ := ParseSelectorList(rules[0].Prelude)
		found := false
		for _, e := range errs {
			if !diag.IsText(e.Message) {
				t.Errorf("%q: a message that is not text: %q", tc.sheet, e.Message)
			}
			found = found || e.Message == tc.want
		}
		if !found {
			t.Errorf("%q: no message %q among %q", tc.sheet, tc.want, errs)
		}
	}
}

// TestEveryCSSMessageIsMadeText drives the three places a problem is recorded,
// with messages written as a careless new call site would write them.
func TestEveryCSSMessageIsMadeText(t *testing.T) {
	tz := newTokenizer("")
	tz.fail(0, "a\x01 by the tokenizer")
	p := &parser{}
	p.fail(0, "a\u0085 by the parser")
	sp := &selParser{}
	sp.fail(0, "a\x1b by the selector parser")
	sp.unsupported(0, "a\x7f by the selector parser")
	sp.inapplicable(0, "a\x93 by the selector parser")

	for _, list := range [][]Error{tz.errs, p.errs, sp.errs} {
		if len(list) == 0 {
			t.Fatal("a problem was not recorded")
		}
		for _, e := range list {
			if !diag.IsText(e.Message) {
				t.Errorf("recorded a message that is not text: %q", e.Message)
			}
		}
	}
}
