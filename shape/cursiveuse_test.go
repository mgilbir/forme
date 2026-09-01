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

// TestOnlyACursiveRunIsMarked. The decision is membership of
// ArabicShaping.txt, which is what HarfBuzz decides by script and this decides
// by character — the same answer, since the file names every character of every
// cursive-joining script and nothing else.
//
// Javanese and Balinese are set by the same shaper and do not join, so the four
// features stay where they were for them. Getting this wrong the other way
// would change every script the universal shaper sets.
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
	} {
		if got := anyCursive([]rune(tc.text)); got != tc.want {
			t.Errorf("%s: anyCursive(%q) = %v, want %v", tc.what, tc.text,
				got, tc.want)
		}
	}
}
