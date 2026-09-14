package css

import (
	"strings"
	"testing"
)

// The tokenizer must not swallow a code point in silence.
//
// Every call to token consumes a span of the input and returns one token. CSS
// lets exactly one thing be consumed without appearing in that token: a comment,
// which §4.3.2 removes before the token proper begins. Everything else between
// where the last token ended and where this one starts is a character the author
// wrote that no token accounts for, and a stylesheet is the wrong place to find
// out that one went missing — a dropped "!" is a rule that stops winning, and a
// dropped delimiter is a selector that stops matching.
//
// This is the same property html's tokenizer is held to, and it is here because
// the existing oracles cannot see the failure. Planted with a stray control
// character deleted rather than emitted as a Delim, the css unit suite passed,
// the css-parsing-tests conformance corpus passed, and FuzzTokenize passed
// 4.5 million executions. They check the tokens against each other — offsets
// run forwards, the last one lands on the end — and never against the input.
//
// commentsOnly is what makes that precise, and the plant that has to miss is
// widening it to excuse anything else.
func commentsOnly(s string) bool {
	for s != "" {
		if !strings.HasPrefix(s, "/*") {
			return false
		}
		// An unterminated comment runs to the end of the input, which §4.3.2
		// consumes with a parse error rather than rejecting.
		end := strings.Index(s[2:], "*/")
		if end < 0 {
			return true
		}
		s = s[2+end+2:]
	}
	return true
}

func accountForEveryCodePoint(t *testing.T, input string) {
	t.Helper()

	tk := newTokenizer(input)
	for steps := 0; ; steps++ {
		// Bounded, so a tokenizer that stops advancing fails here rather than
		// hanging the run. No token can consume less than one code point.
		if steps > len(input)+1 {
			t.Fatalf("still going after %d steps over %d bytes, so a step is not advancing",
				steps, len(input))
		}

		before := tk.offset()
		tok := tk.token()
		after := tk.offset()

		// What was consumed before this token began is only allowed to be
		// comments. tok.Offset is where the token itself starts.
		if tok.Offset < before || tok.Offset > len(input) {
			t.Fatalf("a %v token says it starts at %d, outside the %d..%d it consumed",
				tok.Kind, tok.Offset, before, after)
		}
		if skipped := input[before:tok.Offset]; !commentsOnly(skipped) {
			t.Errorf("%q at offset %d was consumed before a %v token and is not a "+
				"comment, so those characters are gone from the stylesheet",
				skipped, before, tok.Kind)
		}

		if tok.Kind == EOF {
			if after < len(input) {
				t.Errorf("the end was reached at %d of %d bytes, so %q was never read",
					after, len(input), input[after:])
			}
			return
		}
		if after <= before {
			t.Fatalf("a %v token at %d consumed nothing, so the next call repeats it for ever",
				tok.Kind, before)
		}
	}
}

func TestTheTokenizerAccountsForEveryCodePoint(t *testing.T) {
	for _, input := range []string{
		// Ordinary stylesheets.
		"a{b:c}", "a { color : red ; }", ".x > .y ~ .z {}",
		"@media (min-width: 30em) { a { b: c } }",
		"a{margin:0 auto !important}", "a[href^=\"x\"]{}",

		// Comments, which are the one thing allowed to vanish.
		"/*c*/a{b:c}", "a/*c*/{b:c}", "a{/*c*/b:c}", "/*a*//*b*/x", "a{b:c}/*unterminated",
		"/*", "/**/", "/*/", "/**//**/",

		// Stray characters that begin nothing, which must still be emitted.
		"a{b:c}!", "!", "~", "^", "&", "|", "$", "*", "%",
		"\x01", "a\x01b", "a{\x01}", "\x7f",

		// Malformed, where a tokenizer is most tempted to just skip ahead.
		"a{", "}", "a{b:", "\"unterminated", "'unterminated", "url(", "\\",
		"@", "#", "0", "-", ".", "+", "<", "<!--", "-->",
		"", " ", "\r\n", "\x00", "a\x00b",
	} {
		t.Run(input, func(t *testing.T) { accountForEveryCodePoint(t, input) })
	}
}

func FuzzTheTokenizerAccountsForEveryCodePoint(f *testing.F) {
	for _, s := range fuzzSeeds() {
		f.Add(s)
	}
	for _, s := range []string{"a{b:c}!", "\x01", "/*", "/*/", "a/*c*/{b:c}", "a\x00b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, input string) {
		accountForEveryCodePoint(t, input)
	})
}
