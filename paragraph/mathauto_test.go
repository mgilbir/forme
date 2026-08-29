package paragraph

import "testing"

// text-transform: math-auto, and the table it is.
//
// Mathematics sets a single-letter variable in italic and everything else
// upright: "x" is a variable and "sin" is a function name. The italic letters
// are characters of their own in the Mathematical Alphanumeric Symbols block, so
// the transform is a character mapping rather than a font choice — which is why
// it is text-transform's business and not font-style's.

// mathItalicPairs is every mapping MathML Core defines, in full.
//
// It is written out rather than derived, and that is the point: mathauto.go
// derives it, from two contiguous stretches of Unicode and four characters that
// sit outside them, and a derivation is exactly the kind of thing that is right
// about a hundred and eight characters and wrong about four. The list here is
// the one the CSS Working Group's own tests are generated from — its
// mathml/tools/mathvariant.py writes both — so agreeing with it is agreeing with
// what the suite will ask.
var mathItalicPairs = [][2]rune{
	{0x41, 0x1D434},
	{0x42, 0x1D435},
	{0x43, 0x1D436},
	{0x44, 0x1D437},
	{0x45, 0x1D438},
	{0x46, 0x1D439},
	{0x47, 0x1D43A},
	{0x48, 0x1D43B},
	{0x49, 0x1D43C},
	{0x4A, 0x1D43D},
	{0x4B, 0x1D43E},
	{0x4C, 0x1D43F},
	{0x4D, 0x1D440},
	{0x4E, 0x1D441},
	{0x4F, 0x1D442},
	{0x50, 0x1D443},
	{0x51, 0x1D444},
	{0x52, 0x1D445},
	{0x53, 0x1D446},
	{0x54, 0x1D447},
	{0x55, 0x1D448},
	{0x56, 0x1D449},
	{0x57, 0x1D44A},
	{0x58, 0x1D44B},
	{0x59, 0x1D44C},
	{0x5A, 0x1D44D},
	{0x61, 0x1D44E},
	{0x62, 0x1D44F},
	{0x63, 0x1D450},
	{0x64, 0x1D451},
	{0x65, 0x1D452},
	{0x66, 0x1D453},
	{0x67, 0x1D454},
	{0x68, 0x210E},
	{0x69, 0x1D456},
	{0x6A, 0x1D457},
	{0x6B, 0x1D458},
	{0x6C, 0x1D459},
	{0x6D, 0x1D45A},
	{0x6E, 0x1D45B},
	{0x6F, 0x1D45C},
	{0x70, 0x1D45D},
	{0x71, 0x1D45E},
	{0x72, 0x1D45F},
	{0x73, 0x1D460},
	{0x74, 0x1D461},
	{0x75, 0x1D462},
	{0x76, 0x1D463},
	{0x77, 0x1D464},
	{0x78, 0x1D465},
	{0x79, 0x1D466},
	{0x7A, 0x1D467},
	{0x131, 0x1D6A4},
	{0x237, 0x1D6A5},
	{0x391, 0x1D6E2},
	{0x392, 0x1D6E3},
	{0x393, 0x1D6E4},
	{0x394, 0x1D6E5},
	{0x395, 0x1D6E6},
	{0x396, 0x1D6E7},
	{0x397, 0x1D6E8},
	{0x398, 0x1D6E9},
	{0x399, 0x1D6EA},
	{0x39A, 0x1D6EB},
	{0x39B, 0x1D6EC},
	{0x39C, 0x1D6ED},
	{0x39D, 0x1D6EE},
	{0x39E, 0x1D6EF},
	{0x39F, 0x1D6F0},
	{0x3A0, 0x1D6F1},
	{0x3A1, 0x1D6F2},
	{0x3F4, 0x1D6F3},
	{0x3A3, 0x1D6F4},
	{0x3A4, 0x1D6F5},
	{0x3A5, 0x1D6F6},
	{0x3A6, 0x1D6F7},
	{0x3A7, 0x1D6F8},
	{0x3A8, 0x1D6F9},
	{0x3A9, 0x1D6FA},
	{0x2207, 0x1D6FB},
	{0x3B1, 0x1D6FC},
	{0x3B2, 0x1D6FD},
	{0x3B3, 0x1D6FE},
	{0x3B4, 0x1D6FF},
	{0x3B5, 0x1D700},
	{0x3B6, 0x1D701},
	{0x3B7, 0x1D702},
	{0x3B8, 0x1D703},
	{0x3B9, 0x1D704},
	{0x3BA, 0x1D705},
	{0x3BB, 0x1D706},
	{0x3BC, 0x1D707},
	{0x3BD, 0x1D708},
	{0x3BE, 0x1D709},
	{0x3BF, 0x1D70A},
	{0x3C0, 0x1D70B},
	{0x3C1, 0x1D70C},
	{0x3C2, 0x1D70D},
	{0x3C3, 0x1D70E},
	{0x3C4, 0x1D70F},
	{0x3C5, 0x1D710},
	{0x3C6, 0x1D711},
	{0x3C7, 0x1D712},
	{0x3C8, 0x1D713},
	{0x3C9, 0x1D714},
	{0x2202, 0x1D715},
	{0x3F5, 0x1D716},
	{0x3D1, 0x1D717},
	{0x3F0, 0x1D718},
	{0x3D5, 0x1D719},
	{0x3F1, 0x1D71A},
	{0x3D6, 0x1D71B},
}

// TestEveryItalicMappingIsTheOneMathMLCoreDefines.
func TestEveryItalicMappingIsTheOneMathMLCoreDefines(t *testing.T) {
	want := map[rune]rune{}
	for _, p := range mathItalicPairs {
		want[p[0]] = p[1]
		if got := mathItalicOf(p[0]); got != p[1] {
			t.Errorf("%#04X maps to %#04X, want %#04X", p[0], got, p[1])
		}
	}
	if len(want) != 112 {
		t.Errorf("the table holds %d mappings, want 112 — the file it came from "+
			"has that many and a short one would pass every row above", len(want))
	}
	// And nothing outside it maps to anything. A derivation that ran a range one
	// character too far would map a character MathML Core leaves alone, which no
	// row above could catch.
	for r := rune(0); r <= 0x2FFF; r++ {
		if _, ok := want[r]; ok {
			continue
		}
		if got := mathItalicOf(r); got != r {
			t.Errorf("%#04X is not in the mapping and came back as %#04X", r, got)
		}
	}
}

// TestMathAutoOnlyTransformsASingleCharacter, which is the whole of what makes
// it automatic: "x" becomes a variable and "sin" stays a function name, without
// the author marking either.
func TestMathAutoOnlyTransformsASingleCharacter(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"\u2202", "\U0001D715", "one character"},
		{"\u2202\u2207", "\u2202\u2207", "two"},
		{"\u2202\u2207\u0237", "\u2202\u2207\u0237", "three"},
		{"", "", "none at all"},
		// A letter and a combining mark is two characters, so it is left alone.
		{"x\u0302", "x\u0302", "a letter and a combining mark"},
		// A character with no italic form of its own.
		{"1", "1", "a digit"},
		{"(", "(", "a bracket"},
		{"\u4e2d", "\u4e2d", "an ideograph"},
	} {
		if got := mathAuto(tc.text); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestMathAutoReachesTheTextThroughTheProperty is the wiring, and it is a test
// of its own because everything above tests the mapping: a value read correctly
// and a mapping computed correctly still leave the page unchanged if
// TransformText never asks.
func TestMathAutoReachesTheTextThroughTheProperty(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"\u2202", "\U0001D715"},
		{"x", "\U0001D465"},
		{"h", "\u210E"},
		{"sin", "sin"},
	} {
		got, _ := TransformText(tc.text, TransformOf("math-auto"), false, LanguageOf(""))
		if got != tc.want {
			t.Errorf("math-auto over %q gave %q, want %q", tc.text, got, tc.want)
		}
	}
}

// TestMathAutoIsAValueOfItsOwn. §2.1.1's grammar is "none | math-auto |
// [ [capitalize|uppercase|lowercase] || full-width || full-size-kana ]", so
// math-auto shares the alternation with "none" rather than joining the set: it
// is valid alone and invalid beside anything.
func TestMathAutoIsAValueOfItsOwn(t *testing.T) {
	if got := TransformOf("math-auto"); got != TransformMathAuto {
		t.Errorf("math-auto read as %v", got)
	}
	for _, value := range []string{
		"math-auto uppercase", "uppercase math-auto", "math-auto full-width",
		"math-auto none", "math-auto math-auto",
	} {
		if got := TransformOf(value); got != TransformNone {
			t.Errorf("%q read as %v, want none — the grammar allows math-auto alone",
				value, got)
		}
	}
}

// TestTheOtherTransformsAreUntouched is the containment case: math-auto is a
// branch of its own and must not reach a document that asked for anything else.
func TestTheOtherTransformsAreUntouched(t *testing.T) {
	for _, tc := range []struct{ value, text, want string }{
		{"uppercase", "\u2202", "\u2202"},
		{"uppercase", "x", "X"},
		{"lowercase", "X", "x"},
		{"capitalize", "x", "X"},
		{"full-width", "x", "\uFF58"},
		{"none", "x", "x"},
		{"none", "\u2202", "\u2202"},
	} {
		got, _ := TransformText(tc.text, TransformOf(tc.value), false, LanguageOf(""))
		if got != tc.want {
			t.Errorf("%s over %q gave %q, want %q", tc.value, tc.text, got, tc.want)
		}
	}
}
