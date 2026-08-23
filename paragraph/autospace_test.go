package paragraph

import (
	"strings"
	"testing"
)

// text-autospace's character rules, CSS Text 4 §8.1.
//
// The property is about a boundary between two classes of character, so almost
// everything here is a table of which class a character is in. The classes are
// named in the specification by Unicode script rather than by a property, and
// the interesting entries are the ones a script test alone gets wrong.

func TestAutospaceOfReadsTheValue(t *testing.T) {
	for _, tc := range []struct {
		value     string
		want      Autospace
		unhandled string
	}{
		// The initial value, and what a box with nothing computed for it gets.
		{"", Autospace{true, true}, ""},
		{"normal", Autospace{true, true}, ""},
		{"NORMAL", Autospace{true, true}, ""},
		{"  normal  ", Autospace{true, true}, ""},
		{"no-autospace", Autospace{}, ""},
		// The classes on their own and together.
		{"ideograph-alpha", Autospace{IdeographAlpha: true}, ""},
		{"ideograph-numeric", Autospace{IdeographNumeric: true}, ""},
		{"ideograph-alpha ideograph-numeric", Autospace{true, true}, ""},
		{"ideograph-numeric ideograph-alpha", Autospace{true, true}, ""},
		// "insert" is the half of the pair this engine does, so it is read and
		// adds nothing of its own.
		{"ideograph-alpha insert", Autospace{IdeographAlpha: true}, ""},
		// And the parts that are not done are named rather than dropped.
		{"punctuation", Autospace{}, "punctuation"},
		{"ideograph-alpha punctuation", Autospace{IdeographAlpha: true}, "punctuation"},
		{"ideograph-alpha replace", Autospace{IdeographAlpha: true}, "replace"},
		// One finding rather than a list, on the model of the other reports.
		{"punctuation replace", Autospace{}, "punctuation"},
	} {
		got, unhandled := AutospaceOf(tc.value)
		if got != tc.want || unhandled != tc.unhandled {
			t.Errorf("AutospaceOf(%q) = %+v, %q; want %+v, %q",
				tc.value, got, unhandled, tc.want, tc.unhandled)
		}
	}
}

// TestWhichCharactersAreIdeographs.
//
// The scripts are the ones written without word spaces, and the entries that
// matter are the ones a script test alone gets wrong: an iteration mark repeats
// the character before it and a prolonged sound mark lengthens the kana before
// it, and Unicode gives both the Common script — so testing the script alone
// would put a boundary in the middle of a Japanese word.
func TestWhichCharactersAreIdeographs(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		{'国', true, "a Han ideograph"},
		{'永', true, "another"},
		{'水', true, "the water ideograph itself"},
		{0x20000, true, "a Han ideograph above the BMP"},
		{'あ', true, "hiragana"},
		{'ア', true, "katakana"},
		{'ｱ', true, "halfwidth katakana, which is katakana all the same"},
		{'ㄅ', true, "bopomofo"},
		{0x3005, true, "the ideographic iteration mark, which Unicode calls Common"},
		{0x30FC, true, "the prolonged sound mark, which Unicode calls Common"},
		{0x309D, true, "the hiragana iteration mark"},
		{'A', false, "a Latin letter"},
		{'가', false, "Hangul, which is written with word spaces"},
		{'α', false, "Greek"},
		{'1', false, "a digit"},
		{'１', false, "a fullwidth digit, which is a numeral and not an ideograph"},
		{'、', false, "an ideographic comma, which is punctuation"},
		{' ', false, "a space"},
		{0x3000, false, "an ideographic space, which is a space"},
	} {
		if got := IsAutospaceIdeograph(tc.r); got != tc.want {
			t.Errorf("%s (U+%04X): %v, want %v", tc.what, tc.r, got, tc.want)
		}
	}
}

// TestWhichCharactersAreTheOtherSide: the letters and the numerals a boundary
// with an ideograph is worth spacing.
func TestWhichCharactersAreTheOtherSide(t *testing.T) {
	for _, tc := range []struct {
		r             rune
		letter, digit bool
		what          string
	}{
		{'A', true, false, "a Latin letter"},
		{'z', true, false, "another"},
		{'α', true, false, "Greek"},
		{'д', true, false, "Cyrillic"},
		{'א', true, false, "Hebrew"},
		{'م', true, false, "Arabic"},
		{'가', true, false, "Hangul, a letter that is not an ideograph"},
		{'国', false, false, "an ideograph is neither"},
		{'あ', false, false, "kana is neither"},
		{'1', false, true, "a digit"},
		{'٥', false, true, "an Arabic-Indic digit"},
		{'１', false, false, "a fullwidth digit, which is set on the ideographic advance"},
		{'.', false, false, "punctuation is neither"},
		{' ', false, false, "a space is neither"},
	} {
		if got := IsAutospaceLetter(tc.r); got != tc.letter {
			t.Errorf("%s (U+%04X) as a letter: %v, want %v", tc.what, tc.r, got, tc.letter)
		}
		if got := IsAutospaceNumeral(tc.r); got != tc.digit {
			t.Errorf("%s (U+%04X) as a numeral: %v, want %v", tc.what, tc.r, got, tc.digit)
		}
	}
}

// TestAutospaceAtIsSymmetric. The rule is about a boundary, and which side the
// ideograph is on does not change whether there is one.
func TestAutospaceAtIsSymmetric(t *testing.T) {
	normal, _ := AutospaceOf("normal")
	for _, tc := range []struct {
		a, b rune
		want bool
		what string
	}{
		{'国', 'A', true, "an ideograph then a letter"},
		{'A', '国', true, "and the other way round"},
		{'国', '1', true, "an ideograph then a digit"},
		{'1', '国', true, "and the other way round"},
		{'国', '国', false, "two ideographs"},
		{'A', 'B', false, "two letters"},
		{'国', ' ', false, "a space is neither class"},
		{'国', ',', false, "and neither is punctuation"},
		{'A', '1', false, "a letter and a digit, with no ideograph in it"},
	} {
		if got := AutospaceAt(tc.a, tc.b, normal); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.what, got, tc.want)
		}
	}
}

// TestEachClassIsAskedForSeparately, which is the whole reason the value has two
// keywords rather than one.
func TestEachClassIsAskedForSeparately(t *testing.T) {
	alpha, _ := AutospaceOf("ideograph-alpha")
	numeric, _ := AutospaceOf("ideograph-numeric")
	none, _ := AutospaceOf("no-autospace")
	for _, tc := range []struct {
		as   Autospace
		a, b rune
		want bool
		what string
	}{
		{alpha, '国', 'A', true, "ideograph-alpha spaces a letter"},
		{alpha, '国', '1', false, "and leaves a digit alone"},
		{numeric, '国', '1', true, "ideograph-numeric spaces a digit"},
		{numeric, '国', 'A', false, "and leaves a letter alone"},
		{none, '国', 'A', false, "no-autospace spaces nothing"},
		{none, '国', '1', false, "nothing at all"},
	} {
		if got := AutospaceAt(tc.a, tc.b, tc.as); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.what, got, tc.want)
		}
	}
}

// TestAMarkBelongsToTheCharacterBeforeIt.
//
// §8.1 is stated over typographic character units, and a mark is part of the one
// before it: "c" with an acute over it is a Latin letter however many marks
// follow, and a variation selector after an ideograph leaves an ideograph. The
// suite writes both — text-autospace-elements-006 for the marks and
// text-autospace-vs-001 for the selectors — and both put the spacing exactly
// where the unmarked text would have it.
func TestAMarkBelongsToTheCharacterBeforeIt(t *testing.T) {
	for _, tc := range []struct {
		text string
		want rune
		what string
	}{
		{"abć", 'c', "a letter under a combining acute"},
		{"国︀", '国', "an ideograph under a variation selector"},
		{"国\U000E0100", '国', "and under one from the supplement"},
		{"A​", 'A', "a letter before a zero width space, which draws nothing"},
	} {
		got, ok := LastAutospaceBase(tc.text)
		if !ok || got != tc.want {
			t.Errorf("%s: last base of %q is %q (%v), want %q",
				tc.what, tc.text, got, ok, tc.want)
		}
	}
	// A run with nothing but marks in it has no base of its own, and the answer
	// is "none" rather than the mark: the boundary is with whatever stands
	// beyond the run, which the caller keeps walking for.
	for _, text := range []string{"︀", "́̂", "​"} {
		if _, ok := LastAutospaceBase(text); ok {
			t.Errorf("%q was given a base character; it has none", text)
		}
		if _, ok := FirstAutospaceBase(text); ok {
			t.Errorf("%q was given a first base character; it has none", text)
		}
	}
}

// TestSplitAtAutospaceCutsWhereTheGapGoes.
//
// The gap is between two runs, because that is the only shape a backend can be
// handed. A boundary inside one run therefore has to become a boundary between
// two, and this is the cut that makes it.
func TestSplitAtAutospaceCutsWhereTheGapGoes(t *testing.T) {
	normal, _ := AutospaceOf("normal")
	none, _ := AutospaceOf("no-autospace")
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"国A国", []string{"国", "A", "国"}},
		{"国国AA国国", []string{"国国", "AA", "国国"}},
		{"国12", []string{"国", "12"}},
		{"国︀A", []string{"国︀", "A"}},
		{"abć永", []string{"abć", "永"}},
		// Nothing to cut: the answer is nil rather than a one-element slice, so
		// the caller can tell "no boundary" from "one piece" without a length
		// test — and so that the common case allocates nothing.
		{"AAAA", nil},
		{"国国国", nil},
		{"国 A", nil},
		{"国,A", nil},
		{"", nil},
	} {
		got := SplitAtAutospace(tc.text, normal)
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("SplitAtAutospace(%q) = %q, want %q", tc.text, got, tc.want)
		}
		// Whatever it cuts, the pieces are the text: nothing added and nothing
		// dropped. A cut that lost a character would lose it from the page and
		// from what a reader copies out of it.
		if got != nil && strings.Join(got, "") != tc.text {
			t.Errorf("SplitAtAutospace(%q) joins back to %q", tc.text, strings.Join(got, ""))
		}
		// And a document that turned the property off is never cut.
		if got := SplitAtAutospace(tc.text, none); got != nil {
			t.Errorf("no-autospace cut %q into %q", tc.text, got)
		}
	}
}
