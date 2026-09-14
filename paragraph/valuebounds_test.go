package paragraph

import (
	"strings"
	"testing"
)

// TestALineClampOfAbsurdDigitsIsClamped is the bound on the count a
// line-clamp property may state.
//
// The value is text from the document, so it is as many digits as the author
// wrote. Reading them all as arithmetic is what the bound stops, and past it
// the digits that follow cannot change the answer — every caller wants a count
// of lines and there are not a million of them.
//
// Nothing had stated one: every fixture reads a small count, so the paragraph
// suite passes with maxClampLines raised.
func TestALineClampOfAbsurdDigitsIsClamped(t *testing.T) {
	// Within the bound, the number is itself.
	if n, ok := PositiveInteger("12"); !ok || n != 12 {
		t.Fatalf("a count of twelve read as %d, %v; the fixture is wrong", n, ok)
	}
	for _, value := range []string{
		"99999999",                    // past the bound, a plausible typo
		strings.Repeat("9", 400),      // past anything, an attack
		"+" + strings.Repeat("9", 40), // the sign the grammar allows
	} {
		n, ok := PositiveInteger(value)
		if !ok {
			t.Errorf("%.20q… was refused; a number past the bound is clamped, not "+
				"rejected — the property still applies", value)
			continue
		}
		if n != maxClampLines {
			t.Errorf("%.20q… read as %d; past the bound it is the bound, so that "+
				"the digits after it are never arithmetic", value, n)
		}
	}
}

// TestTheContextHandedToAShaperIsTrimmed is the bound on how much text either
// side of a run is kept.
//
// A shaper is told what stands next to a run so that it can decide a joining
// form, and a text node is as long as the document says. The bound is what
// stops the whole of the rest of the paragraph being handed over for a question
// a shaper answers from one or two characters.
//
// Nothing had handed it a long one: every fixture passes a few characters, so
// the paragraph suite passes with maxContextBytes raised.
func TestTheContextHandedToAShaperIsTrimmed(t *testing.T) {
	long := strings.Repeat("x", 4096)
	for _, c := range []struct {
		name string
		got  string
	}{
		{"after", ContextAfter(long)},
		{"before", ContextBefore(long)},
	} {
		// A literal, deliberately not maxContextBytes. A test that measures
		// against the constant it is testing cannot fail when the constant
		// moves: raising the bound raises the assertion with it, and this
		// passed with maxContextBytes at a million. The number here is a
		// statement about what a shaper needs — a few hundred bytes is already
		// far more than the one or two characters it reads — and it is the
		// thing that would have to be argued with, rather than a copy of the
		// value under test.
		const farMoreThanAShaperReads = 512
		if len(c.got) > farMoreThanAShaperReads {
			t.Errorf("the context %s a run came to %d bytes of %d; it is trimmed to "+
				"what a shaper reads across a boundary, not handed the rest of the "+
				"paragraph", c.name, len(c.got), len(long))
		}
		if len(c.got) == 0 {
			t.Errorf("the context %s a run came to nothing; it is trimmed, not "+
				"dropped", c.name)
		}
	}

	// A short one is handed over whole, or the trimming is not trimming.
	if got := ContextAfter("ab"); got != "ab" {
		t.Errorf("a two-byte context came back as %q", got)
	}

	// And the trim never separates a letter from its accent: a window cut
	// mid-cluster would change the very form it is here to decide.
	marks := strings.Repeat("é", 200)
	if got := ContextAfter(marks); !utf8Valid(got) {
		t.Errorf("the context was cut mid-rune: %q", got)
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == 0xFFFD && !strings.Contains(s, "�") {
			return false
		}
	}
	return true
}
