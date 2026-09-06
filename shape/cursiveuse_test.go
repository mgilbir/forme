package shape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/font"
)

// A script the universal shaper sets, whose letters join.
//
// A dozen of them: N'Ko, Adlam, Mongolian, Hanifi Rohingya, Sogdian, Old
// Uyghur, Phags-pa, Manichaean, Psalter Pahlavi, Chorasmian, Yezidi. The
// universal model has nothing to say about which of the four shapes a letter
// takes — that is the Arabic model's question — and the four features that
// answer it sat in useFinalFeatures, applied to every glyph of the run.
//
// Applied to every glyph they are wrong for every glyph. A font states them as
// three single-substitution lookups and the lookup list is walked in index
// order, so in Noto Sans N'Ko 'fina' matched first and every letter of a word
// came out in its final shape, with nothing left for 'medi' or 'init' to match.
// The page that produces is not merely unjoined: it is the wrong glyph in every
// position.

// nkoFace loads the Web Platform Tests' Noto Sans N'Ko, which is the font the
// suite's shaping-020 through -022 are written against.
func nkoFace(t *testing.T) *Face {
	t.Helper()
	root := os.Getenv("WPT_TESTS")
	if root == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`) for a cursive font the " +
			"universal shaper sets")
	}
	data, err := os.ReadFile(filepath.Join(root, "fonts", "noto",
		"NotoSansNko-regular-webfont.woff2"))
	if err != nil {
		t.Skipf("the N'Ko font is not in the checkout: %v", err)
	}
	if font.IsWOFF2(data) {
		out, err := font.DecodeWOFF(data)
		if err != nil {
			t.Fatalf("the N'Ko font did not decompress: %v", err)
		}
		data = out
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("the N'Ko font did not load: %v", err)
	}
	return f
}

// gidsOf is what a face makes of a string.
func gidsOf(t *testing.T, f *Face, s string) []int {
	t.Helper()
	gs, _ := f.ShapeGlyphs(s)
	out := make([]int, 0, len(gs))
	for _, g := range gs {
		out = append(out, g.GID)
	}
	return out
}

// TestACursiveScriptTheUniversalShaperSetsTakesItsForms.
func TestACursiveScriptTheUniversalShaperSetsTakesItsForms(t *testing.T) {
	f := nkoFace(t)
	// Three N'Ko KA in a row: initial, medial and final, which are three
	// different glyphs. The run comes back in visual order, so the final one is
	// first.
	got := gidsOf(t, f, "ߞߞߞ")
	if len(got) != 3 {
		t.Fatalf("three letters shaped to %v", got)
	}
	if got[0] == got[1] || got[1] == got[2] || got[0] == got[2] {
		t.Errorf("the three letters came out as %v — a word of one repeated "+
			"shape is the whole fault this is about", got)
	}
	// And the isolated form is none of them: a letter alone is not a letter in
	// a word.
	alone := gidsOf(t, f, "ߞ")
	if len(alone) != 1 {
		t.Fatalf("one letter shaped to %v", alone)
	}
	for _, g := range got {
		if g == alone[0] {
			t.Errorf("a letter inside the word took the shape %d it has alone",
				g)
		}
	}
}

// TestAJoinerDecidesTheFormAcrossARunEnd is what the suite's tests are made of:
// the two documents write the same three letters, one with joiners between them
// and one with non-joiners, and must not look alike.
func TestAJoinerDecidesTheFormAcrossARunEnd(t *testing.T) {
	f := nkoFace(t)
	alone := gidsOf(t, f, "ߞ")
	joined := gidsOf(t, f, "ߞ‍")
	notJoined := gidsOf(t, f, "ߞ‌")
	if len(joined) != 1 || len(notJoined) != 1 {
		t.Fatalf("the joiners left glyphs of their own: %v and %v",
			joined, notJoined)
	}
	if joined[0] == alone[0] {
		t.Errorf("a zero width joiner after the letter left it in its lone "+
			"shape %d", joined[0])
	}
	if notJoined[0] != alone[0] {
		t.Errorf("a zero width non-joiner after the letter gave it shape %d, "+
			"want the lone shape %d", notJoined[0], alone[0])
	}
}

// TestOnlyACursiveRunIsMarked. The decision is the script of the run's
// characters, which is what HarfBuzz decides of the run as a whole.
//
// Javanese and Balinese are set by the same shaper and do not join, so the four
// features stay where they were for them. Getting this wrong the other way
// would change every script the universal shaper sets — and it did, for any run
// holding a joiner or a narrow no-break space, because those are listed in
// ArabicShaping.txt and membership of that file was what this asked.
func TestOnlyACursiveRunIsMarked(t *testing.T) {
	for _, tc := range []struct {
		what string
		text string
		want bool
	}{
		{"N'Ko", "ߞߞ", true},
		{"Adlam", "𞤀𞤁", true},
		{"Mongolian", "ᠮᠣᠩ", true},
		{"Javanese", "ꦲꦏ", false},
		{"Balinese", "ᬳᬓ", false},
		{"Tibetan", "བོད", false},
		{"Latin", "ab", false},

		// The characters ArabicShaping.txt lists that are not text of a
		// cursive script. A Javanese word is written with a joiner where two
		// letters must or must not join, and one of these in it used to turn
		// the four positional forms on for the whole run — which applies all
		// four to every glyph and substitutes each letter three times over.
		{"Javanese with a zero-width joiner", "ꦲ‍ꦏ", false},
		{"Javanese with a zero-width non-joiner", "ꦲ‌ꦏ", false},
		{"Javanese with a narrow no-break space", "ꦲ ꦏ", false},
		{"Latin in a bidi isolate", "⁦ab⁩", false},
		{"Kaithi with its number sign", "𑂽𑄏", false},
	} {
		if got := anyCursive([]rune(tc.text)); got != tc.want {
			t.Errorf("%s: anyCursive(%q) = %v, want %v", tc.what, tc.text,
				got, tc.want)
		}
	}
}

// TestWhatCountsAsCursiveText pins the predicate itself, on both sides of what
// changed.
//
// It was membership of ArabicShaping.txt. That file gives a joining type to
// every character of every cursive-joining script, which is why it looked like
// the property — but it is a file about joining and not about scripts, so it
// leaves out everything of those scripts that does not join and takes in a
// handful of characters that join in any script at all.
func TestWhatCountsAsCursiveText(t *testing.T) {
	for _, tc := range []struct {
		r    rune
		want bool
		what string
	}{
		// What the file lists and is cursive text: the letters, and the marks
		// and signs written among them.
		{0x0628, true, "ARABIC LETTER BEH"},
		{0x0621, true, "ARABIC LETTER HAMZA, which is non-joining and still Arabic"},
		{0x0640, true, "ARABIC TATWEEL, a Common-script character that is Arabic text"},
		{0x0605, true, "ARABIC NUMBER MARK ABOVE, likewise Common and likewise Arabic"},
		{0x07CA, true, "NKO LETTER A"},
		{0x0840, true, "MANDAIC LETTER HALQA"},

		// What the file leaves out and is cursive text all the same. A
		// thousand and some characters, which letter-spacing was being
		// inserted into.
		{0x0660, true, "ARABIC-INDIC DIGIT ZERO"},
		{0x060E, true, "ARABIC POETIC VERSE SIGN"},
		{0xFE8D, true, "ARABIC LETTER ALEF ISOLATED FORM, a presentation form"},
		{0xFEFB, true, "ARABIC LIGATURE LAM WITH ALEF ISOLATED FORM"},
		{0x0730, true, "SYRIAC PTHAHA ABOVE"},
		{0x1800, true, "MONGOLIAN BIRGA"},

		// What the file lists and is not text of a cursive script at all.
		{0x200D, false, "ZERO WIDTH JOINER, which joins in every script and is of none"},
		{0x200C, false, "ZERO WIDTH NON-JOINER"},
		{0x202F, false, "NARROW NO-BREAK SPACE, a space of no script"},
		{0x2066, false, "LEFT-TO-RIGHT ISOLATE"},
		{0x2069, false, "POP DIRECTIONAL ISOLATE"},
		{0x00AD, false, "SOFT HYPHEN, which the file lists as transparent"},
		{0x110BD, false, "KAITHI NUMBER SIGN, which is Kaithi and does not join"},

		// And the marks, which have no script of their own to answer with:
		// Unicode calls an Arabic fatha Inherited because it takes the script
		// of the letter it is written on. Every reader resolves that from the
		// base instead of asking here.
		{0x064E, false, "ARABIC FATHA, which Unicode calls Inherited"},
		{0x0670, false, "ARABIC LETTER SUPERSCRIPT ALEF, likewise Inherited"},
		{0x060C, false, "ARABIC COMMA, which Unicode calls Common: Syriac and Thaana write it too"},

		// And ordinary text of scripts that do not join.
		{'a', false, "a Latin letter"},
		{' ', false, "a space"},
		{0x0301, false, "a Latin combining acute"},
		{0xA98F, false, "JAVANESE LETTER KA"},
		{0x0915, false, "DEVANAGARI LETTER KA"},
	} {
		if got := InCursiveScript(tc.r); got != tc.want {
			t.Errorf("U+%04X %s: InCursiveScript = %v, want %v",
				tc.r, tc.what, got, tc.want)
		}
	}
}
