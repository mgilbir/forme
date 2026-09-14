package html

import (
	"strings"
	"testing"
)

// The tokenizer must not swallow a byte in silence.
//
// Every step consumes a span of the input, src[before:after], and either
// produces a token or does not. A step that produces one has said what those
// bytes were. A step that produces none has thrown them away, and HTML only
// permits that for constructs it defines as discarded: the bogus comment family
// — "<!" and "<![" declarations, "<?" processing instructions — a comment with
// no terminator, and an end tag with no name. Anything else consumed without a
// token is a character the author wrote being deleted from the page.
//
// That is not a hypothetical. Until this line of work a "<" that begins no tag
// took exactly this path: one byte consumed, no token, and "a<0b" reached the
// page as "a0b". FuzzParse could not see it, because a tree with a character
// missing is still a perfectly well-formed tree — every structural invariant it
// checks held. The defect was only visible from the input side, which is what
// this checks and that one cannot.
//
// discardedPrefixes is deliberately a list of prefixes rather than a count. A
// count would pass while the set changed underneath it.
var discardedPrefixes = []string{
	"<!", // a declaration, or a comment with no terminator: a bogus comment
	"<?", // a processing instruction, which this engine has nothing to follow
	"</", // reached here only with no name, which is also a bogus comment
}

// accountForEveryByte drives the tokenizer over src and reports, in the words a
// failure should use, any byte that no token accounts for. It is shared by the
// table test and the fuzz target so that both check the same property.
func accountForEveryByte(t *testing.T, src string) {
	t.Helper()

	tk := newTokenizer(src)

	// The constructor consumes a leading byte order mark before any step runs:
	// those three bytes are an encoding statement, not content. Every other
	// byte has to be accounted for by a step.
	if tk.pos != 0 && !strings.HasPrefix(src, bom) {
		t.Fatalf("the tokenizer started at %d on input that has no byte order mark", tk.pos)
	}

	// Bounded, because a tokenizer that fails to advance must fail this test
	// rather than hang it. One step can consume one byte at least, so the
	// input's length plus the end is every step there can be.
	for steps := 0; ; steps++ {
		if steps > len(src)+1 {
			t.Fatalf("still going after %d steps over %d bytes, so a step is not advancing",
				steps, len(src))
		}

		before := tk.pos
		tok, ok := tk.step()
		after := tk.pos

		// ok first, and only then the kind. tokEOF is the zero value of
		// tokenKind, so the token{} a discarded construct returns reads as an
		// end of input that has not been reached — which stops the walk early
		// and blames the remaining bytes on the tokenizer. Written the other
		// way round this test reported "<?x?>tail" as losing "tail".
		if ok && tok.kind == tokEOF {
			if after < len(src) {
				t.Errorf("the end was reached at %d of %d bytes, so %q was never read",
					after, len(src), src[after:])
			}
			return
		}

		if after <= before {
			t.Fatalf("a step at %d consumed nothing, so the next one repeats it for ever", before)
		}
		if ok {
			continue // The token accounts for the bytes.
		}

		span := src[before:after]
		for _, p := range discardedPrefixes {
			if strings.HasPrefix(span, p) {
				span = ""
				break
			}
		}
		if span != "" {
			t.Errorf("%q at offset %d was consumed without a token and begins no "+
				"construct HTML discards, so those bytes are gone from the page",
				span, before)
		}
	}
}

func TestTheTokenizerAccountsForEveryByte(t *testing.T) {
	for _, src := range []string{
		// The character this property was written for.
		"a<0b", "a<", "a< b", "1<2 and 3>2", "a<=b", "a<<b",

		// The constructs that legitimately keep nothing.
		"a<!b", "a</0b", "<!---", "<![CDATA[x]]>", "<?x?>", "<?x", "</ >", "</ ",

		// The same constructs with input after them. A discarded construct
		// returns token{}, and tokEOF is that token's zero kind, so these are
		// the cases that catch a walk which mistakes one for the end.
		"<?x?>tail", "<![CDATA[x]]>tail", "</ >tail", "<!--a-->tail",

		// And ordinary markup, which must not be reported as a loss.
		"<p>hello</p>", "<ul><li>a<li>b</ul>", "<p>&amp;&#65;</p>",
		bom + "<p>x</p>", "<script>var x = 1 < 2;</script>",
		"<style>a{b:c}</style>", "<textarea><p></textarea>",
		"", "<", ">", "&", "\x00",
	} {
		t.Run(src, func(t *testing.T) { accountForEveryByte(t, src) })
	}
}

func FuzzTheTokenizerAccountsForEveryByte(f *testing.F) {
	for _, s := range []string{
		"a<0b", "a<", "1<2 and 3>2", "a<!b", "a</0b", "<!---", "<![CDATA[x]]>",
		"<?x?>", "</ >", "<p>hello</p>", bom + "<p>x</p>", "<script>a<b</script>",
		"<", ">", "&", "\x00", strings.Repeat("<div>", 100),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > maxInputBytes {
			return
		}
		accountForEveryByte(t, src)
	})
}
