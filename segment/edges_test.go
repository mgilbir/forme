package segment

import (
	"testing"
	"unicode/utf8"
)

// TestAPictographicBelowTheFastPathIsStillPictographic.
//
// The fast path answered "not pictographic" for everything below the combining
// marks, and U+00A9 © and U+00AE ® are — the two oldest characters Unicode
// calls Extended_Pictographic. So GB11, which holds an emoji joined to another
// emoji together, did not apply to the two characters it was most likely to be
// asked about, and "©‍©" came apart at the joiner.
func TestAPictographicBelowTheFastPathIsStillPictographic(t *testing.T) {
	for _, tc := range []struct {
		s    string
		want int
		what string
	}{
		{"©‍©", 1, "two copyright signs joined"},
		{"®‍®", 1, "two registered signs joined"},
		{"\U0001f468‍\U0001f469", 1, "two emoji joined, which always worked"},
		{"©©", 2, "two copyright signs not joined"},
		{"a‍a", 2, "two letters joined, which GB11 is not about: the joiner " +
			"attaches to the letter before it and the one after starts a cluster"},
	} {
		if got := Count(tc.s); got != tc.want {
			t.Errorf("%s: %d clusters, want %d", tc.what, got, tc.want)
		}
	}
	if !propsOf('©').pict {
		t.Error("U+00A9 is not read as pictographic")
	}
	if !propsOf('®').pict {
		t.Error("U+00AE is not read as pictographic")
	}
}

// TestTheFastPathFloorIsWhereTheTablesStart pins what makes the above true, so
// that a Unicode release adding a lower entry cannot quietly put it back.
func TestTheFastPathFloorIsWhereTheTablesStart(t *testing.T) {
	if pictRanges[0].lo < asciiLimit || conjunctRanges[0].lo < asciiLimit {
		t.Errorf("the fast path answers for everything below U+%04X, and the "+
			"tables it answers for begin at U+%04X and U+%04X",
			asciiLimit, pictRanges[0].lo, conjunctRanges[0].lo)
	}
	// And it is not lower than it needs to be, or ASCII pays for nothing.
	if asciiLimit > 0x300 {
		t.Errorf("the fast path floor is U+%04X, above the combining marks", asciiLimit)
	}
}

// TestCountAndBoundariesAgreeOnEveryString.
//
// Two answers about the same string that disagree are worse than either.
// Boundaries cut on both sides of a byte that is not UTF-8 and started the
// scanner again; Count let range's U+FFFD through as an ordinary character, so
// a following combining mark attached to it in one and not in the other.
func TestCountAndBoundariesAgreeOnEveryString(t *testing.T) {
	for _, tc := range []struct{ s, what string }{
		{"", "the empty string"},
		{"abc", "letters"},
		{"áb", "a letter and a combining mark"},
		{"\xff", "one byte that is not UTF-8"},
		{"a\xffb", "an invalid byte between two letters"},
		{"a\xff́b", "a combining mark after an invalid byte"},
		{"\xff\xfe\xfd", "three invalid bytes"},
		{"\xed\xa0\x80", "an encoded surrogate, which is three invalid bytes"},
		{"caf\xe9", "windows-1252 text"},
		{"\U0001f1e6\U0001f1e7\xff\U0001f1e8", "an invalid byte between regional indicators"},
	} {
		count := Count(tc.s)
		want := len(Boundaries(nil, tc.s))
		if tc.s != "" {
			want++ // Boundaries leaves out the one at the start
		}
		if count != want {
			t.Errorf("%s (%q): Count says %d clusters and Boundaries says %d",
				tc.what, tc.s, count, want)
		}
		// And a Scanner, which is the third way to ask.
		var sc Scanner
		n := 0
		for _, r := range tc.s {
			if sc.Boundary(r) {
				n++
			}
		}
		if tc.s != "" && !utf8.ValidString(tc.s) {
			// A Scanner is handed runes and cannot see the bytes behind them,
			// so it is the one that legitimately answers differently — which is
			// worth stating rather than leaving for someone to find.
			continue
		}
		if n != count {
			t.Errorf("%s (%q): a Scanner says %d clusters and Count says %d",
				tc.what, tc.s, n, count)
		}
	}
}
