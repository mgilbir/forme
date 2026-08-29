package paragraph

import "testing"

// The pair rules of UAX #14 that a manufactured opportunity runs into.
//
// linebreak.go's noBreakBefore answers the rules written "× X", the ones that
// need nothing on the left: a line may not begin with a closing bracket, a
// hyphen, a non-starter. This is the rest of what a break-all document meets,
// and it is a pair because the rules are — LB11 is "× WJ" *and* "WJ ×", LB12 is
// "GL ×", and neither can be answered from one side.
//
// It matters only where an opportunity was manufactured. Ordinary text offers
// one at a space and after an ideograph, and none of the characters here is
// either, so nothing asks. word-break: break-all offers one at every character
// boundary inside a word, and then the question is real at every one of them:
// §5.2 allows breaking "between typographic character units", and UAX #14 still
// says which of those boundaries are not there.

// breakAllValue is read through the parser rather than built here, for the reason
// keepAll gives beside it: reading the value as the zero value is exactly the
// mistake a test that built the struct itself would not see.
func breakAllValue(t *testing.T) WordBreak {
	t.Helper()
	wb, unhandled := WordBreakOf("break-all")
	if unhandled != "" {
		t.Fatalf("break-all was reported as unhandled: %q", unhandled)
	}
	if wb == (WordBreak{}) {
		t.Fatal("break-all read as the initial value")
	}
	return wb
}

// TestNothingBreaksEitherSideOfAGlueCharacter.
//
// A no-break space, a word joiner and a zero width joiner each hold the two
// characters around them together, and break-all does not overrule that. The
// fixture puts one in the middle of a word so that the surrounding boundaries
// *are* offered — "a|b" and "c|d" are in every expectation below, and they are
// what says the value is doing its work and only these boundaries were withheld.
func TestNothingBreaksEitherSideOfAGlueCharacter(t *testing.T) {
	ba := breakAllValue(t)
	for _, tc := range []struct {
		r    rune
		what string
	}{
		{0x00A0, "a no-break space"},
		{0x2060, "a word joiner"},
		{0x200D, "a zero width joiner"},
		{0x180E, "a Mongolian vowel separator"},
		{0x0F0C, "a Tibetan delimiter"},
	} {
		text := "ab" + string(tc.r) + "cd"
		want := "a|b" + string(tc.r) + "c|d"
		if got := splits(t, text, ba); got != want {
			t.Errorf("%s: %q, want %q — nothing breaks on either side of it",
				tc.what, got, want)
		}
	}
}

// TestALineMayNotEndAfterAPrefix. U+005C is class PR, which is a surprise until
// one remembers the class is about what a character introduces rather than what
// it looks like: a currency sign, a number sign, a backslash.
//
// word-break-break-all-023 and -024 name the rule in their own assert —
// "break-all breaks before the first backslash character because UAX14 rules
// forbid to break after PR class" — and -024's fixture is the second row here.
func TestALineMayNotEndAfterAPrefix(t *testing.T) {
	ba := breakAllValue(t)
	for _, tc := range []struct{ text, want, what string }{
		{"ab\\cd", "a|b|\\c|d", "a backslash in a word"},
		{"XXX\\X", "X|X|X|\\X", "word-break-break-all-024's own text"},
		{"¥100", "¥1|0|0", "a currency sign and the figure it introduces"},
		{"a\u2116" + "1", "a|\u2116" + "1", "a numero sign"},
	} {
		if got := splits(t, tc.text, ba); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestLooseLetsBreakAllEndALineAfterAPrefix, which is §5.3's one rule the other
// way round: under loose a line may end after a currency sign, because a
// newspaper column is narrow enough to need it.
//
// It is here to say that break-all does not take that away, and not to pin
// anything in gluedPair — the exemption lives in SplitAtBreaks, in a branch that
// flushes the piece itself, and gluedPair was written with a matching "!Loose"
// that could not be made to fail. See the note there.
func TestLooseLetsBreakAllEndALineAfterAPrefix(t *testing.T) {
	ba := breakAllValue(t)
	loose, unhandled := LineBreakOf("loose")
	if unhandled != "" {
		t.Fatalf("loose was reported as unhandled: %q", unhandled)
	}
	got := splitsWith(t, "ab\\cd", ba, loose)
	if want := "a|b|\\|c|d"; got != want {
		t.Errorf("under loose: %q, want %q — the break after a prefix is loose's "+
			"to allow, and break-all does not take it back", got, want)
	}
	// And no other value allows it, so the exception is the exception.
	for _, value := range []string{"normal", "strict", "auto"} {
		lb, unhandled := LineBreakOf(value)
		if unhandled != "" {
			t.Fatalf("%s was reported as unhandled: %q", value, unhandled)
		}
		if got := splitsWith(t, "ab\\cd", ba, lb); got != "a|b|\\c|d" {
			t.Errorf("under %s: %q, want %q", value, got, "a|b|\\c|d")
		}
	}
}

// TestARefusedOpportunityIsGoneRatherThanMoved.
//
// A prohibition on what a line may *begin* with moves an opportunity forward:
// "× CL" says nothing against a line beginning with what follows the bracket, so
// the next character is asked in its turn. A prohibition on what a line may
// *end* after is not like that, and holding one forward puts the break one
// character past the glue — which is the answer the rule exists to prevent.
//
// "中\u00a0ab" is where the two differ, and it took some finding: an ideograph
// defers an opportunity, the no-break space refuses it on both sides, and the
// characters after it offer none of their own. Held, it surfaces between the a
// and the b. Dropped, the line does not break at all, which is right.
func TestARefusedOpportunityIsGoneRatherThanMoved(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"\u4e2d\u00a0ab", "\u4e2d\u00a0ab", "a no-break space"},
		{"\u4e2d\u2060ab", "\u4e2d\u2060ab", "a word joiner"},
	} {
		if got := splits(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%s: %q, want %q — the opportunity the glue refused was moved "+
				"past it rather than dropped", tc.what, got, tc.want)
		}
	}
	// The control: the same shape with something the glue does not hold on to
	// still breaks, so the rows above are not passing because nothing ever does.
	if got := splits(t, "\u4e2d\u00a0\u6587\u6587", WordBreak{}); got != "\u4e2d\u00a0\u6587|\u6587" {
		t.Errorf("the control came out %q; an ideograph after the glue still offers "+
			"an opportunity of its own", got)
	}
}

// TestTheSuiteFixtureSetsThreeCharactersOnTheFirstLine is the whole point,
// written as the reftest states it.
//
// "XXXX&nbsp;XXXX X X" in four characters of room. The fourth X is glued to the
// no-break space and the space to the X after it, so those three are one unit
// and will not fit beside the three before them. The right answer sets three
// characters on the first line; the answer without the rule sets four and leaves
// the no-break space hanging at a line end, which is the one thing its name
// forbids. word-break-break-all-018, -021 and -022 are that fixture.
func TestTheSuiteFixtureSetsThreeCharactersOnTheFirstLine(t *testing.T) {
	got := splits(t, "XXXX\u00a0XXXX X X", breakAllValue(t))
	want := "X|X|X|X\u00a0X|X|X|X |X |X"
	if got != want {
		t.Fatalf("%q, want %q", got, want)
	}
	// Said as the reftest says it: the first four characters are not four
	// separate opportunities, because the fourth is glued to what follows.
	if splits(t, "XXXX", breakAllValue(t)) != "X|X|X|X" {
		t.Errorf("the same four characters without the glue are four " +
			"opportunities; the fixture above is measuring the glue")
	}
}

// TestNormalTextIsUnchanged is the containment case, and it is the one that
// matters most: this rule sits in the path every paragraph goes through, and
// the characters it is about are ones no ordinary document contains.
func TestNormalTextIsUnchanged(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"the quick brown fox", "the |quick |brown |fox"},
		{"a-b c", "a-|b |c"},
		{"日本語日本語", "日|本|語|日|本|語"},
		{"one\u00a0two three", "one\u00a0two |three"},
		{"ab\\cd ef", "ab\\cd |ef"},
	} {
		if got := splits(t, tc.text, WordBreak{}); got != tc.want {
			t.Errorf("%q came out %q, want %q — nothing about the pair rules may "+
				"reach a document that offers no opportunity there",
				tc.text, got, tc.want)
		}
	}
}

// TestAnywhereOverrulesThePairRules. §5.3 puts an opportunity around every
// typographic character unit "including around any punctuation character or
// preserved white space", and it is a value whose whole purpose is to overrule
// what is here — the same exemption noBreakBefore is given beside it.
func TestAnywhereOverrulesThePairRules(t *testing.T) {
	anywhere, unhandled := LineBreakOf("anywhere")
	if unhandled != "" {
		t.Fatalf("anywhere was reported as unhandled: %q", unhandled)
	}
	got := splitsWith(t, "ab\u00a0cd", WordBreak{}, anywhere)
	if want := "a|b|\u00a0|c|d"; got != want {
		t.Errorf("under anywhere: %q, want %q", got, want)
	}
}

// TestAnywhereOverrulesTheNoBreakSpaceSeparators is the same exemption, one
// character class over, and it is a different code path.
//
// U+2007 FIGURE SPACE and U+202F NARROW NO-BREAK SPACE are *other space
// separators* — §4.1.2 hangs them at the end of a line like any other space —
// and they are class GL, so UAX #14 offers no opportunity after them. That
// combination is read where the separators are split off, and it was read
// without asking whether §5.3 had overruled it: "XX\u202FX" under
// line-break: anywhere offered no break at all after the separator, so a box
// three characters wide set all four on one line and let the fourth overflow.
//
// The suite writes it as line-break-anywhere-overrides-uax-behavior-008, whose
// title is the rule in as many words: "line-break: anywhere overrides behavior
// defined for the WJ, ZW, GL, and ZWJ classes".
func TestAnywhereOverrulesTheNoBreakSpaceSeparators(t *testing.T) {
	anywhere, unhandled := LineBreakOf("anywhere")
	if unhandled != "" {
		t.Fatalf("anywhere was reported as unhandled: %q", unhandled)
	}
	for _, tc := range []struct {
		what string
		r    rune
	}{
		{"a narrow no-break space", 0x202F},
		{"a figure space", 0x2007},
	} {
		text := "XX" + string(tc.r) + "X"
		got := splitsWith(t, text, WordBreak{}, anywhere)
		want := "X|X|" + string(tc.r) + "|X"
		if got != want {
			t.Errorf("%s: %q, want %q — §5.3 puts an opportunity around every "+
				"typographic character unit, and the glue classes are named as "+
				"among what it overrules", tc.what, got, want)
		}
	}
}

// TestTheGlueSeparatorsStillHoldWithoutAnywhere is the containment case, and it
// is the whole reason the two are read apart rather than together.
//
// A figure space is what keeps a column of digits from being split across two
// lines and a narrow no-break space is a no-break space by name. Offering a
// break after either of them by default would undo the one thing each is for.
func TestTheGlueSeparatorsStillHoldWithoutAnywhere(t *testing.T) {
	for _, tc := range []struct {
		what string
		r    rune
		want string
	}{
		{"a narrow no-break space", 0x202F, "XX X"},
		{"a figure space", 0x2007, "XX X"},
		// An en space is class BA and does offer one, so this is about the two
		// no-break separators and not about separators in general.
		{"an en space", 0x2002, "XX |X"},
		// And an ideographic space is class ID. The break before it is the one
		// UAX #14's LB7 withholds — a line may not end in front of white space —
		// so what is left is the one after it, which is the half this is about.
		{"an ideographic space", 0x3000, "XX　|X"},
	} {
		got := splitsWith(t, "XX"+string(tc.r)+"X", WordBreak{}, LineBreak{})
		if got != tc.want {
			t.Errorf("%s: %q, want %q", tc.what, got, tc.want)
		}
	}
}

// splitsWith is splits with a line-break value as well.
func splitsWith(t *testing.T, text string, wb WordBreak, lb LineBreak) string {
	t.Helper()
	pieces, _ := SplitAtBreaks(text, WhiteSpace{Collapse: true, Wrap: true}, wb, lb, Hyphens{})
	out := ""
	for _, p := range pieces {
		if p.BreakBefore {
			out += "|"
		}
		out += p.Text
	}
	return out
}
