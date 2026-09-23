package shape

import (
	"sort"
	"testing"
)

// TestTheTransparentDefaultIsGenerated.
//
// ArabicShaping.txt lists some characters and states a default for the rest:
// an unlisted non-spacing mark, enclosing mark or format character is
// transparent. cmd/genjoining now applies that default from the release's own
// UnicodeData.txt, into defaultTransparentRanges, so the generated tables hold
// the answer for every character. It used to be left to package unicode, whose
// release is older than the table's: U+0897 ARABIC PEPET, a mark Unicode 16
// added, came out non-joining.
//
// It reads the tables, and then joiningTypeOf, which asked package unicode
// after the listed table where it now asks the generated default.
func TestTheTransparentDefaultIsGenerated(t *testing.T) {
	listed := func(r rune) (joiningType, bool) {
		i := sort.Search(len(joiningRanges), func(i int) bool { return joiningRanges[i].hi >= r })
		if i < len(joiningRanges) && r >= joiningRanges[i].lo {
			return joiningRanges[i].t, true
		}
		return 0, false
	}
	byDefault := func(r rune) bool {
		i := sort.Search(len(defaultTransparentRanges), func(i int) bool { return defaultTransparentRanges[i].hi >= r })
		return i < len(defaultTransparentRanges) && r >= defaultTransparentRanges[i].lo
	}
	for _, r := range []rune{
		0x0301,  // COMBINING ACUTE ACCENT, which no Arabic file lists
		0x0897,  // ARABIC PEPET, Unicode 16
		0x10EFA, // ARABIC DOUBLE VERTICAL BAR BELOW, Unicode 16
		0x10EFB, // ARABIC SMALL LOW NOON, Unicode 16
		0x10EFC, // ARABIC COMBINING ALEF OVERLAY, Unicode 16
		0x20DD,  // COMBINING ENCLOSING CIRCLE, Me
		0x200B,  // ZERO WIDTH SPACE, Cf
	} {
		if _, ok := listed(r); ok || !byDefault(r) {
			t.Errorf("U+%04X: listed %v, transparent by default %v; want only the second",
				r, ok, byDefault(r))
		}
	}
	// What the file lists is in the listed table and not the default one,
	// including format characters it gives another type.
	for r, want := range map[rune]joiningType{0x0600: joinU, 0x200D: joinC, 0x0640: joinC, 0x200C: joinU} {
		if got, ok := listed(r); !ok || got != want || byDefault(r) {
			t.Errorf("U+%04X is (%d, %v) in the listed table and %v by default, want %d and not by default",
				r, got, ok, byDefault(r), want)
		}
	}
	// U+1171E was a non-spacing mark in Unicode 15 and is a spacing one in 17,
	// so the release the table is from makes it non-joining.
	if _, ok := listed(0x1171E); ok || byDefault(0x1171E) {
		t.Error("U+1171E AHOM CONSONANT SIGN MEDIAL RA, Mc since Unicode 16, is transparent")
	}

	// And the shaper reads them.
	for r, want := range map[rune]joiningType{
		0x0897: joinT, 0x10EFC: joinT, 0x0301: joinT, 0x1171E: joinU, 0x0628: joinD, 0x200D: joinC,
	} {
		if got := joiningTypeOf(r); got != want {
			t.Errorf("joiningTypeOf(U+%04X) = %d, want %d", r, got, want)
		}
	}
	// Which is what keeps a join across a pepet.
	if forms := joinForms([]rune{0x0628, 0x0897, 0x0628}, nil, nil); forms[0] != featInitial || forms[2] != featFinal {
		t.Errorf("beh, pepet, beh took the forms %q; the pepet broke the join", forms)
	}
}
