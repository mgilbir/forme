package charprop

import (
	"testing"
	"unicode"
)

// TestTheReleaseItSaysItIs asks the tables about characters whose answer
// changed after Unicode 15.0, which is the release package unicode answers
// from under Go 1.26 — the toolchain go.mod names. Tables generated from that
// release, or from the toolchain rather than the pinned files, get every one
// of these wrong.
func TestTheReleaseItSaysItIs(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want Set
		what string
	}{
		{0x0897, Mn, "ARABIC PEPET, a mark since Unicode 16"},
		{0x1C89, Lu, "CYRILLIC CAPITAL LETTER TJE, Unicode 16"},
		{0x1C8A, Ll, "CYRILLIC SMALL LETTER TJE, Unicode 16"},
		{0x10D40, Nd, "GARAY DIGIT ZERO, Unicode 16"},
		{0x10D6E, Pd, "GARAY HYPHEN, Unicode 16"},
		{0xA7CC, Lu, "LATIN CAPITAL LETTER S WITH DIAGONAL STROKE, Unicode 17"},
		{0x0295, Lo, "LATIN LETTER PHARYNGEAL VOICED FRICATIVE, Ll until Unicode 16"},
		{0x1171E, Mc, "AHOM CONSONANT SIGN MEDIAL RA, Mn until Unicode 16"},
		{0x0378, Cn, "unassigned"},
		{-1, Cn, "not a code point"},
		{0x110000, Cn, "past the end of the code space"},
	} {
		if got := Of(tc.r); got != tc.want {
			t.Errorf("Of(U+%04X) = %b, want %b: %s", tc.r, got, tc.want, tc.what)
		}
	}
	if !Cased(0x1C8A) || !Cased(0x10D70) || Cased(0x10D40) {
		t.Error("Cased is wrong about the Unicode 16 letters and digits")
	}
	if !CaseIgnorable(0x0897) || !CaseIgnorable('\'') || !CaseIgnorable(0x00AD) || CaseIgnorable('a') {
		t.Error("Case_Ignorable is wrong about a mark, the apostrophe, the soft hyphen or a letter")
	}
	if !SoftDotted('i') || !SoftDotted(0x1DF1A) || SoftDotted('l') {
		t.Error("Soft_Dotted is wrong")
	}
	if !WhiteSpace(0x00A0) || !WhiteSpace(0x2028) || !WhiteSpace('\t') || WhiteSpace(0x200B) {
		t.Error("White_Space is wrong")
	}
}

// TestEveryRangeIsFound. The tables are searched by halving, which finds
// nothing, quietly, in a table that is out of order — and the first 256 code
// points are answered from a copy made at start-up rather than by the search.
// So every range is asked for at both ends, and just outside them.
func TestEveryRangeIsFound(t *testing.T) {
	for i, s := range categoryRanges {
		if i > 0 && s.lo <= categoryRanges[i-1].hi {
			t.Fatalf("categoryRanges[%d] begins at U+%04X, inside the range before it", i, s.lo)
		}
		if Of(s.lo) != s.cat || Of(s.hi) != s.cat {
			t.Errorf("U+%04X..U+%04X is %b in the table and %b, %b by Of", s.lo, s.hi, s.cat, Of(s.lo), Of(s.hi))
		}
		// Adjacent runs of one category are one run in the table.
		if Of(s.hi+1) == s.cat {
			t.Errorf("U+%04X continues the run before it, which the table ends", s.hi+1)
		}
	}
	for name, tab := range map[string][]struct{ lo, hi rune }{
		"White_Space":    whiteSpaceRanges[:],
		"Soft_Dotted":    softDottedRanges[:],
		"Cased":          casedRanges[:],
		"Case_Ignorable": caseIgnorableRanges[:],
	} {
		for i, s := range tab {
			if i > 0 && s.lo <= tab[i-1].hi+1 {
				t.Fatalf("%s[%d] at U+%04X overlaps or adjoins the range before it", name, i, s.lo)
			}
			if !inRanges(s.lo, tab) || !inRanges(s.hi, tab) || inRanges(s.lo-1, tab) || inRanges(s.hi+1, tab) {
				t.Errorf("%s: U+%04X..U+%04X is not found as the table states it", name, s.lo, s.hi)
			}
		}
	}
}

// TestAgreesWithTheToolchainWhereTheReleasesAgree holds the lookups to an
// implementation that shares none of their code, over the whole code space.
//
// This is the one place package unicode is read outside cmd/, and it is read
// as an oracle and not an answer: cmd/pinnedunicode_test.go names this file.
// Where the toolchain's release is this one, every category, White_Space and
// Soft_Dotted must be the same. Where it is not, the toolchain is older, and
// what Unicode guarantees across releases must still hold: a character it
// assigns is assigned here, and White_Space — which Unicode has not changed
// since 6.3 — is the same.
func TestAgreesWithTheToolchainWhereTheReleasesAgree(t *testing.T) {
	names := map[string]Set{
		"Lu": Lu, "Ll": Ll, "Lt": Lt, "Lm": Lm, "Lo": Lo, "Mn": Mn, "Mc": Mc, "Me": Me,
		"Nd": Nd, "Nl": Nl, "No": No, "Pc": Pc, "Pd": Pd, "Ps": Ps, "Pe": Pe, "Pi": Pi,
		"Pf": Pf, "Po": Po, "Sm": Sm, "Sc": Sc, "Sk": Sk, "So": So, "Zs": Zs, "Zl": Zl,
		"Zp": Zp, "Cc": Cc, "Cf": Cf, "Cs": Cs, "Co": Co,
	}
	tables := map[Set]*unicode.RangeTable{}
	for n, s := range names {
		tables[s] = unicode.Categories[n]
		if tables[s] == nil {
			t.Fatalf("package unicode has no category %s", n)
		}
	}
	same := unicode.Version == UnicodeVersion
	differ, added := 0, 0
	for r := rune(0); r <= unicode.MaxRune; r++ {
		theirs := Cn
		for s, tab := range tables {
			if unicode.Is(tab, r) {
				theirs = s
				break
			}
		}
		ours := Of(r)
		switch {
		case theirs == ours:
		case theirs == Cn:
			added++
		default:
			differ++
			if same || ours == Cn {
				t.Errorf("U+%04X is %b in Unicode %s and %b here", r, theirs, unicode.Version, ours)
			}
		}
		if unicode.IsSpace(r) != WhiteSpace(r) {
			t.Errorf("U+%04X: White_Space differs from Unicode %s", r, unicode.Version)
		}
		if same && unicode.Is(unicode.Soft_Dotted, r) != SoftDotted(r) {
			t.Errorf("U+%04X: Soft_Dotted differs from Unicode %s", r, unicode.Version)
		}
	}
	if !same && added == 0 {
		t.Errorf("the toolchain is Unicode %s and the tables %s, and they assign the same characters",
			unicode.Version, UnicodeVersion)
	}
	t.Logf("toolchain Unicode %s, tables %s: %d characters assigned here only, %d recategorised",
		unicode.Version, UnicodeVersion, added, differ)
}
