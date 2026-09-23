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

// TestWhichCharactersAreIdeographs is CSS Text 4's list, row by row: U+3041 to
// U+30FF less its punctuation, the CJK strokes, the katakana phonetic
// extensions, and Han.
//
// The entries that matter are the ones a script test alone gets wrong in either
// direction: an iteration mark and the prolonged sound mark are Common script
// and are in the list's range, and the double hyphen U+30A0 is in the range and
// is punctuation, so it is not. Bopomofo and the halfwidth katakana are in no
// row of the list — this engine used to count both, by script — and are not
// ideographs; the halfwidth katakana are letters instead (see
// TestWhichCharactersAreTheOtherSide).
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
		{'ｱ', false, "halfwidth katakana, which no row of the list names"},
		{'ㄅ', false, "bopomofo, which no row of the list names"},
		{0x30A0, false, "the double hyphen, which is in the range and is punctuation"},
		{0x30FB, false, "the katakana middle dot, likewise"},
		{0x3099, true, "the combining voiced sound mark, which is in the range"},
		{0x31C0, true, "a CJK stroke"},
		{0x31F0, true, "a katakana phonetic extension"},
		{0x31350, true, "a Han ideograph of extension H, which Go's own tables predate"},
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
		// The width clause: a letter that is Wide or Fullwidth is not the other
		// side, so neither Korean beside a Hanja nor a fullwidth Latin letter
		// among ideographs is spaced. Audit C119.
		{'가', false, false, "Hangul, a letter whose East Asian Width is W"},
		{'Ａ', false, false, "a fullwidth Latin letter"},
		{'ｱ', true, false, "halfwidth katakana, a letter whose width is H"},
		{0xFFA1, true, false, "a halfwidth Hangul letter, likewise"},
		{'ㄅ', false, false, "bopomofo, which is neither: not listed, and Wide"},
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

// TestAHalfwidthSoundMarkIsNotTheOtherSideOfABoundary.
//
// "ｼﾞ" is the shape: a halfwidth katakana and its own dakuten, one syllable to a
// reader and two characters to a table. The suite's
// hanging-punctuation-allow-end-001 measured its halfwidth rows an eighth of an
// em per syllable too wide when the katakana was an ideograph and its mark a
// letter, and broke its lines early.
//
// CSS Text 4's list names neither, so both are letters — the katakana is Lo and
// the three marks Lm, and all four are width H — and a boundary between two
// letters is not one §8.1 spaces. That was once got by calling all four
// ideographs; the list says the other thing and it gives the same answer here.
// What differs is the boundary with a fullwidth ideograph, which the list
// spaces: that is the reading of Issue 9503's "classes under review" this takes,
// and it is stated at IsAutospaceIdeograph.
func TestAHalfwidthSoundMarkIsNotTheOtherSideOfABoundary(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		what string
	}{
		{0xFF70, "the halfwidth prolonged sound mark, the twin of U+30FC"},
		{0xFF9E, "the halfwidth voiced sound mark"},
		{0xFF9F, "the halfwidth semi-voiced sound mark"},
	} {
		if IsAutospaceIdeograph(tc.r) || !IsAutospaceLetter(tc.r) || !IsAutospaceLetter('ｼ') {
			t.Errorf("%s (U+%04X) and the katakana it follows are not both letters",
				tc.what, tc.r)
		}
	}
	// The boundary the bug actually opened, stated as the suite states it.
	as := Autospace{IdeographAlpha: true, IdeographNumeric: true}
	for _, tc := range []struct {
		a, b rune
		what string
	}{
		{'ｼ', 0xFF9E, "a halfwidth katakana and its own dakuten"},
		{'ｱ', 0xFF70, "a halfwidth katakana and a prolonged sound mark"},
		{0xFF9E, 'ｼ', "the same pair the other way round"},
	} {
		if AutospaceAt(tc.a, tc.b, as) {
			t.Errorf("%s (U+%04X U+%04X) got an eighth of an em between them",
				tc.what, tc.a, tc.b)
		}
	}
}
