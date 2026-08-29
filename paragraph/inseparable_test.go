package paragraph

import "testing"

// Breaking between inseparable characters, CSS Text §5.3.
//
// "loose" is described in one sentence with several clauses, and two of them are
// about the ellipses: a line may *begin* with one, and a line may break
// *between* two of them. They read as one rule and are not — the first is UAX
// #14's LB22 relaxed and the second is an opportunity nothing else creates —
// which is why an engine can have the first and still fail the test written for
// the second. line-break-loose-015's assert names both characters: "'line-break:
// loose' allows line breaking between inseparable characters such as TWO DOT
// LEADER (U+2025) and HORIZONTAL ELLIPSIS (U+2026)".
//
// The fixtures put the pair between ideographs so that there is somewhere else
// to break, which is what makes "no break inside the pair" visible as an answer
// rather than as an absence.

// splitLoose is splits under one strictness value, over text this engine will
// treat as Chinese or Japanese.
func splitLoose(t *testing.T, text, value string) string {
	t.Helper()
	lb, unhandled := LineBreakOf(value)
	if unhandled != "" {
		t.Fatalf("%q was reported as unhandled: %q", value, unhandled)
	}
	lb.ChineseOrJapanese = true
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true},
		WordBreak{}, lb, Hyphens{})
	out := ""
	for _, p := range pieces {
		if p.BreakBefore {
			out += "|"
		}
		out += p.Text
	}
	return out
}

// TestLooseBreaksBetweenTwoEllipses is the rule, and the three values beside it
// are what say it is the rule rather than the default.
//
// What those three do *not* prove is that the rule is gated on "loose": class IN
// is in looseBreakRanges, so at every other value a line may not begin with an
// ellipsis and the opportunity would be refused whether or not it was offered.
// The gate is the rule as §5.3 writes it and no document can tell it from its
// absence — see the note in breaks.go.
func TestLooseBreaksBetweenTwoEllipses(t *testing.T) {
	for _, tc := range []struct{ text, what string }{
		{"中中‥‥中", "two dot leaders"},
		{"中中……中", "horizontal ellipses"},
		{"中中‥…中", "one of each"},
	} {
		if got, want := splitLoose(t, tc.text, "loose"), "中|中|"; len(got) < len(want) ||
			got[:len(want)] != want {
			t.Errorf("%s under loose: %q", tc.what, got)
		}
		// The pair is one piece under everything else.
		for _, v := range []string{"auto", "normal", "strict"} {
			got := splitLoose(t, tc.text, v)
			if got != "中|中"+tc.text[len("中中"):len(tc.text)-len("中")]+"|中" {
				t.Errorf("%s under %s: %q, want the pair kept together",
					tc.what, v, got)
			}
		}
	}
}

// TestEveryInseparableCharacterIsOne walks the class. Three of the four ranges
// are a single character each, and a table that lost one would go on passing the
// test above.
func TestEveryInseparableCharacterIsOne(t *testing.T) {
	for _, r := range []rune{0x2024, 0x2025, 0x2026, 0x22EF, 0xFE19, 0x10AF6} {
		if !isInseparable(r) {
			t.Errorf("%#04X is class IN and this engine does not think so", r)
		}
		pair := "中中" + string(r) + string(r) + "中"
		if got := splitLoose(t, pair, "loose"); got != "中|中|"+string(r)+"|"+string(r)+"中" {
			t.Errorf("%#04X under loose: %q, want a break between the two", r, got)
		}
	}
	// And characters that are not in the class, including the ones next to it in
	// the code space and the ellipsis-looking one that is class NS.
	for _, r := range []rune{0x2023, 0x2027, 0x22EE, 0xFE18, 0x2047, '.', '…' + 1} {
		if isInseparable(r) {
			t.Errorf("%#04X is not class IN and this engine thinks it is", r)
		}
	}
}

// TestNoOpportunityIsInventedWhereThereWasNone is the containment case, and the
// one that matters most: this rule sits in the path every paragraph goes
// through, and the characters it is about turn up in ordinary prose.
//
// "loose" relaxes prohibitions. It may not make a break where UAX #14 has no
// opportunity at all — an ellipsis in the middle of an English sentence is not a
// place a line may end, at any value.
func TestNoOpportunityIsInventedWhereThereWasNone(t *testing.T) {
	// Latin text only, and deliberately: beside an ideograph there *is* an
	// opportunity — the one the ideograph rule makes — and loose relaxing the
	// prohibition in front of the ellipsis is the other half of §5.3's sentence
	// doing its job. "中‥" breaking under loose and not under normal is right.
	for _, text := range []string{"a…b", "wait…what", "a‥b", "…", "e.g. this"} {
		plain := splitLoose(t, text, "normal")
		loose := splitLoose(t, text, "loose")
		if plain != loose {
			t.Errorf("%q came out %q under normal and %q under loose; there is "+
				"one ellipsis and nothing to break between", text, plain, loose)
		}
	}
}

// TestTheOtherHalfOfTheSentenceStillHolds, because the two clauses are easy to
// conflate and this one was implemented first: a line may *begin* with an
// ellipsis under loose and may not under the other three.
func TestTheOtherHalfOfTheSentenceStillHolds(t *testing.T) {
	for _, r := range []rune{0x2025, 0x2026} {
		for _, tc := range []struct {
			value string
			begin bool
		}{{"auto", false}, {"normal", false}, {"strict", false}, {"loose", true}} {
			lb, _ := LineBreakOf(tc.value)
			if got := MayNotBeginLine(string(r), lb); got == tc.begin {
				t.Errorf("%#04X under %q: may not begin a line = %v, want %v",
					r, tc.value, got, !tc.begin)
			}
		}
	}
}
