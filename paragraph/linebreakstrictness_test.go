package paragraph

import (
	"strings"
	"testing"
)

// line-break, CSS Text §5.3: how strictly Chinese and Japanese may break.
//
// UAX #14 gives the default, and §5.3 tailors it three ways. The tailoring is
// not a refinement — it moves characters in both directions — so "strict"
// forbids what "normal" allows and "loose" allows what "normal" forbids, and
// each needs its own table.
//
// The rows below are the suite's own statement, read out of its fixtures. Each
// of the css-text/line-break tests puts one character in a paragraph ten
// characters wide and shows, in a control paragraph beside it, where the break
// must fall. Reading all forty of them gives this table, and it is what the
// tables in linebreaktable.go were generated to produce.

// breaksBefore reports whether the splitter offers a line break in front of a
// character, with ideographs either side of it so that there is somewhere else
// to break.
//
// The text is Chinese, so the value says so. §5.3's tailoring is stated "in
// Chinese and Japanese" and one of its rules is gated on it, so a LineBreak
// value alone does not settle what a line may begin with — see the
// ChineseOrJapanese field. Setting it here rather than in each row keeps the
// table below about the property, which is what it is a reading of.
func breaksBefore(t *testing.T, r rune, lb LineBreak) bool {
	t.Helper()
	lb.ChineseOrJapanese = true
	text := "中中" + string(r) + "文"
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		WordBreak{}, lb, Hyphens{})
	for _, p := range pieces {
		if strings.HasPrefix(p.Text, string(r)) {
			return p.BreakBefore
		}
	}
	return false
}

// TestTheStrictnessValuesTailorTheDefault.
func TestTheStrictnessValuesTailorTheDefault(t *testing.T) {
	const (
		Y = true
		N = false
	)
	for _, tc := range []struct {
		r                           rune
		auto, normal, strict, loose bool
		what                        string
	}{
		// §5.3 names four characters as hyphens and does not treat them alike.
		// U+2010 and U+2013 are class HH, which no unconditional rule mentions,
		// so every value would let a line begin with one and three of them have
		// to be told otherwise. U+301C and U+30A0 are class NS, so the base
		// table forbids them and two values let them through.
		//
		// The suite says it as plainly as it can be said:
		// line-break-loose-hyphens-001 reads "the second line starts with a
		// hyphen" and line-break-normal-hyphens-001, over the same text, reads
		// "ends with a hyphen".
		{0x2010, N, N, N, Y, "a hyphen"},
		{0x2013, N, N, N, Y, "an en dash"},
		{0x301C, N, Y, N, Y, "a wave dash"},
		{0x30A0, N, Y, N, Y, "a katakana-hiragana double hyphen"},

		// Class CJ, the small kana and the prolonged sound mark. UAX #14 leaves
		// the class to a tailoring; §5.3 resolves it to NS under strict and to
		// ID under everything else.
		{0x3041, Y, Y, N, Y, "hiragana small a"},
		{0x3049, Y, Y, N, Y, "hiragana small o"},
		{0x30FC, Y, Y, N, Y, "the prolonged sound mark"},
		{0xFF70, Y, Y, N, Y, "its halfwidth form"},

		// The iteration marks and the centred punctuation, which §5.3 names one
		// at a time. They are class NS, so only loose lets them through.
		{0x3005, N, N, N, Y, "an ideographic iteration mark"},
		{0x303B, N, N, N, Y, "a vertical ideographic iteration mark"},
		{0x309D, N, N, N, Y, "a hiragana iteration mark"},
		{0x30FD, N, N, N, Y, "a katakana iteration mark"},
		{0x30FB, N, N, N, Y, "a katakana middle dot"},
		{0xFF1A, N, N, N, Y, "a fullwidth colon"},
		{0xFF65, N, N, N, Y, "a halfwidth katakana middle dot"},
		{0x203C, N, N, N, Y, "a double exclamation mark"},
		{0xFF01, N, N, N, Y, "a fullwidth exclamation mark"},
		{0xFF1F, N, N, N, Y, "a fullwidth question mark"},

		// Class IN, the inseparable characters, which §5.3 names whole.
		{0x2025, N, N, N, Y, "a two dot leader"},
		{0x2026, N, N, N, Y, "a horizontal ellipsis"},

		// Class PO, the postfixes. UAX #14 has no unconditional rule about them,
		// so this is the one part of the tailoring that *adds* a prohibition —
		// and auto, which is untailored, keeps allowing the break.
		{0x00B0, Y, N, N, Y, "a degree sign"},
		{0x2030, Y, N, N, Y, "a per mille sign"},
		{0x2103, Y, N, N, Y, "degrees Celsius"},
		{0xFF05, Y, N, N, Y, "a fullwidth per cent sign"},
		{0xFFE0, Y, N, N, Y, "a fullwidth cent sign"},

		// Class PR, the prefixes. Nothing forbids a line beginning with one; what
		// §5.3 says about them is about the other side, and is below.
		{0x20AC, Y, Y, Y, Y, "a euro sign"},
		{0xFFE5, Y, Y, Y, Y, "a fullwidth yen sign"},
	} {
		for _, v := range []struct {
			name string
			lb   LineBreak
			want bool
		}{
			{"auto", LineBreak{}, tc.auto},
			{"normal", LineBreak{Normal: true}, tc.normal},
			{"strict", LineBreak{Strict: true}, tc.strict},
			{"loose", LineBreak{Loose: true}, tc.loose},
		} {
			if got := breaksBefore(t, tc.r, v.lb); got != v.want {
				t.Errorf("%s (%#04X) under %s: a line may begin with it = %v, want %v",
					tc.what, tc.r, v.name, got, v.want)
			}
		}
	}
}

// TestAutoIsNotNormal, which is the one place this engine had to choose.
//
// §5.3 leaves auto to the engine — "the UA determines the set of line-breaking
// restrictions to use" — and the suite forces the choice. Its
// css3-text-line-break-opclns tests set no value and assert UAX #14's own
// behaviour for the wave dash and the double hyphen; line-break-normal-013 sets
// "normal" and asserts the opposite. Both are satisfiable only if the two
// differ, so auto is the untailored report and normal is §5.3's tailoring of it.
func TestAutoIsNotNormal(t *testing.T) {
	// Through the parser, because the claim is about what the two *values* mean
	// and not about two structs: reading "normal" as the zero value is exactly
	// the mistake this records, and a test that built the struct itself would
	// not see it.
	auto, _ := LineBreakOf("auto")
	normal, _ := LineBreakOf("normal")
	if auto == normal {
		t.Fatalf("auto and normal read as the same value, %+v", auto)
	}
	if absent, _ := LineBreakOf(""); absent != auto {
		t.Errorf("no value at all read as %+v and auto as %+v", absent, auto)
	}
	for _, r := range []rune{0x301C, 0x30A0} {
		if breaksBefore(t, r, auto) {
			t.Errorf("%#04X: auto let a line begin with it; UAX #14 forbids it and "+
				"auto is this engine's untailored answer", r)
		}
		if !breaksBefore(t, r, normal) {
			t.Errorf("%#04X: normal did not let a line begin with it; §5.3 names it "+
				"as one of four hyphens that normal allows", r)
		}
	}
}

// TestLooseLetsALineEndAfterAPrefix, which is §5.3's one rule stated the other
// way round: a currency sign belongs to the figure that follows it, and no
// other value lets a line come between them.
func TestLooseLetsALineEndAfterAPrefix(t *testing.T) {
	breaksAfter := func(r rune, lb LineBreak) bool {
		text := "中中" + string(r) + "文"
		pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
			WordBreak{}, lb, Hyphens{})
		for _, p := range pieces {
			if strings.HasPrefix(p.Text, "文") {
				return p.BreakBefore
			}
		}
		return false
	}
	for _, r := range []rune{0x20AC, 0x2116, 0xFF04, 0xFFE1, 0xFFE5} {
		if !breaksAfter(r, LineBreak{Loose: true}) {
			t.Errorf("%#04X: loose did not let a line end after it", r)
		}
		for _, v := range []struct {
			name string
			lb   LineBreak
		}{{"auto", LineBreak{}}, {"normal", LineBreak{Normal: true}},
			{"strict", LineBreak{Strict: true}}} {
			if breaksAfter(r, v.lb) {
				t.Errorf("%#04X: %s let a line end after it; the sign belongs to the "+
					"figure that follows it", r, v.name)
			}
		}
	}
}

// TestTheTailoringDoesNothingToLatinText is the containment case, and the reason
// the property could be reported over CJK text alone before it was implemented:
// the three values differ from auto only in how CJK breaks, and the suite has
// three tests (pre-wrap-004, -005 and -006) whose whole assertion is that
// "XX    XX" wraps the same under loose as under auto.
func TestTheTailoringDoesNothingToLatinText(t *testing.T) {
	ws := WhiteSpace{Collapse: true, Wrap: true}
	for _, text := range []string{
		"hello world", "a-b", "one, two; three!", "a (b) c", "50% off", "x…y",
		"don't", "e.g. this", "1,000",
	} {
		want, _ := SplitAtBreaks(text, ws, WordBreak{}, LineBreak{}, Hyphens{})
		for _, v := range []struct {
			name string
			lb   LineBreak
		}{{"normal", LineBreak{Normal: true}}, {"strict", LineBreak{Strict: true}},
			{"loose", LineBreak{Loose: true}}} {
			got, _ := SplitAtBreaks(text, ws, WordBreak{}, v.lb, Hyphens{})
			if len(got) != len(want) {
				t.Errorf("%q under %s: %d pieces, want %d", text, v.name, len(got), len(want))
				continue
			}
			for i := range got {
				if got[i] != want[i] {
					t.Errorf("%q under %s: piece %d is %+v, want %+v",
						text, v.name, i, got[i], want[i])
				}
			}
		}
	}
}

// TestTheTailoringTablesAreSortedAndDisjoint, which the search over them needs.
// An out-of-order range is not found, and the rule it holds then quietly stops
// applying to whatever is in it.
func TestTheTailoringTablesAreSortedAndDisjoint(t *testing.T) {
	for _, tc := range []struct {
		name  string
		table []struct{ lo, hi rune }
	}{
		{"noBreakBeforeRanges", noBreakBeforeRanges[:]},
		{"strictNoBreakRanges", strictNoBreakRanges[:]},
		{"looseBreakRanges", looseBreakRanges[:]},
		{"prefixRanges", prefixRanges[:]},
		{"postfixRanges", postfixRanges[:]},
		{"inseparableRanges", inseparableRanges[:]},
	} {
		if len(tc.table) == 0 {
			t.Fatalf("%s is empty", tc.name)
		}
		for i, s := range tc.table {
			if s.lo > s.hi {
				t.Errorf("%s[%d] is %#04X..%#04X, which is empty", tc.name, i, s.lo, s.hi)
			}
			if i > 0 && s.lo <= tc.table[i-1].hi+1 {
				t.Errorf("%s[%d] starts at %#04X and the range before ends at %#04X",
					tc.name, i, s.lo, tc.table[i-1].hi)
			}
			for _, r := range []rune{s.lo, s.hi, s.lo + (s.hi-s.lo)/2} {
				if !inLineBreakRanges(r, tc.table) {
					t.Errorf("%s: %#04X is in range %d and was not found", tc.name, r, i)
				}
			}
		}
	}
}

// TestTheLooseHyphensNeedTheWritingSystem is the gate seen from this side: the
// two class NS hyphens are the only characters in the table above whose answer
// the LineBreak value does not settle by itself.
//
// §5.3 states the whole tailoring "in Chinese and Japanese". For every other
// row that is a description of the text rather than a condition on it — nothing
// but Chinese or Japanese contains an iteration mark or a halfwidth katakana
// middle dot — so the tailoring can be applied blind and the containment case
// below is what makes that safe. U+301C and U+30A0 are the exception, because
// Japanese written in another script still has them: see
// layout/linebreak_test.go, where a document supplies the answer.
func TestTheLooseHyphensNeedTheWritingSystem(t *testing.T) {
	for _, r := range []rune{0x301C, 0x30A0} {
		for _, v := range []struct {
			name string
			lb   LineBreak
		}{{"normal", LineBreak{Normal: true}}, {"loose", LineBreak{Loose: true}}} {
			// breaksBefore sets the field, so this is the row the table asserts.
			if !breaksBefore(t, r, v.lb) {
				t.Errorf("%#04X: %s did not let Chinese begin a line with it", r, v.name)
			}
			// And the same value over text that is neither.
			text := "中中" + string(r) + "文"
			pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
				WordBreak{}, v.lb, Hyphens{})
			for _, p := range pieces {
				if strings.HasPrefix(p.Text, string(r)) && p.BreakBefore {
					t.Errorf("%#04X: %s let a line begin with it where the writing "+
						"system is neither Chinese nor Japanese", r, v.name)
				}
			}
		}
	}
}
