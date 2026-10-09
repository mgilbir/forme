package shape

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// MissingGlyphs against ShapeGlyphs: the same count for every face and every
// text, and nothing recorded as used.
//
// The count is decided by the characters a face is asked for, and what those
// are is not the text: a run is cut by direction and by script, mirrored,
// rearranged for Thai and Lao, recomposed or decomposed into the spelling the
// face draws, and has dotted circles put into it before any glyph is chosen. So
// the two have to be compared over fonts that take each of those paths and over
// texts that reach each of them, which is what the lists below are.

// missingGlyphsTexts is the text set: whole strings that mix what the
// preprocessing treats differently, every character of the BMP's first three
// planes' worth of blocks and a sample of the rest, and each base with each
// mark.
func missingGlyphsTexts() []string {
	texts := []string{
		"", " ", "Hello, world.", "office affine ffl", "1\u20442 and 3\u20444",
		// Combining sequences, precomposed and not, and ones that only
		// decompose.
		"e\u0301", "\u00e9", "a\u0308\u0323", "\u1e09", "\u01d6", "o\u0302\u0301",
		"\u212b", "\u2126", "A\u030a", "q\u0307\u0323", "\u0344", "\u0f73", "\u0f77",
		"a" + strings.Repeat("\u0316\u0323\u0301\u0302", 8),
		// Hangul: syllables, conjoining jamo, a jamo run that composes and one
		// that cannot, the fillers, the tone marks.
		"한국어", "\u1100\u1161\u11a8", "\u1100\u1161", "\u1100\u1100\u1161",
		"\u115f\u1160", "\u3164", "\uffa0", "가\u302e", "\u302e\u302f", "\u1100\u200d\u1161",
		// Thai and Lao, with the sara am both scripts decompose.
		"ภาษาไทย", "กำ", "ก\u0e48ำ", "ນ\u0ec9ຳ", "ກຳ", "ำ", "ຳ",
		// Indic, with the invalid vowel spellings a dotted circle goes into.
		"नमस\u094dत\u0947", "क\u094dषत\u094dर\u093fय", "अ\u093e", "अ\u0946", "अ\u0946", "\u093e",
		"ব\u09be\u0982ল\u09be", "অ\u09be", "தம\u0bbfழ\u0bcd", "ஆ\u0bcd", "ಕನ\u0ccdನಡ", "മലയ\u0d3eള\u0d02",
		"ഇ\u0d57", "ස\u0dd2\u0d82හල", "ગ\u0ac1જર\u0abeત\u0ac0", "ਪ\u0a70ਜ\u0a3eਬ\u0a40",
		// Southeast Asian scripts the universal model sets.
		"မ\u103cန\u103aမ\u102c", "ខ\u17d2ម\u17c2រ", "ᬩᬮ\u1b36", "ꦗꦮ",
		// Right to left, with brackets to mirror and numbers to embed.
		"שלום", "(שלום)", "א[ב]", "א\u05b8ב\u05b0\u05bc", "مرحبا", "(مرحبا)",
		"سلام ١٢٣ (abc)", "﷼", "ܣܘܪܝܝܐ", "ދ\u07a8ވ\u07acހ\u07a8", "ߒߞߏ", "abc אבג 123 عربي def",
		"لا", "ــ", "\u064b\u0651",
		// Mongolian, Tibetan, and CJK with its variation sequences.
		"ᠮᠣᠩᠭᠣᠯ", "ᠠ\u180b", "བ\u0f7cད་ས\u0f90ད", "日本語", "葛\U000e0100", "漢字かなカナ",
		"\u3000", "、。", "\U00020000",
		// Emoji: presentation selectors, skin tones, a ZWJ family, tags,
		// keycaps and flags.
		"❤\ufe0f", "❤\ufe0e", "\U0001f44d\U0001f3fd", "\U0001f468\u200d\U0001f4bb",
		"\U0001f468\u200d\U0001f469\u200d\U0001f467", "\U0001f3f4\U000e0067\U000e0062\U000e0065\U000e006e\U000e0067\U000e007f",
		"1\ufe0f\u20e3", "\U0001f1ef\U0001f1f5", "⛹\U0001f3ff\u200d♀\ufe0f",
		// Controls, format characters and the separators.
		"\x00\x01\t\n\r\x7f", "\u0085", "a\u00adb", "a\u200bb", "\ufeffx", "a\u00a0b",
		"\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a", "\u1680",
		"\u202f\u205f", "a\u2011b", "\u200c\u200d", "\u202ea\u202c", "\u2066a\u2069",
		"\u061c", "\u034f", "\u2060\u2061", "\U000e0001",
		// Unassigned, noncharacters, private use, and bytes that are not UTF-8.
		"\u0378", "\u0bfe", "\uffff", "\ufdd0", "\ue000", "\U000f0000", "\U0010ffff",
		"\U000e0080", "\xff\xfe", "a\xc0b", "\xed\xa0\x80",
		// Dotted circle written by the author, and a stretch of everything.
		"◌", "◌\u093e",
		"Les Mis\u00e9rables — 日本 שלום مرحبا नमस\u094dत\u0947 ภาษา 한국 \U0001f600 ❤\ufe0f!",
	}
	// Every character up to the CJK ideographs, which is every script a model
	// of its own sets, and a sample beyond. Under the race detector, which
	// slows shaping tenfold and is not what this test is about, a sample of
	// the first part too.
	step := rune(1)
	if raceEnabled {
		step = 7
	}
	for r := rune(0); r < 0x3400; r += step {
		texts = append(texts, string(r))
	}
	for r := rune(0x3400); r <= 0x10ffff; r += 97 {
		texts = append(texts, string(r))
	}
	bases := []rune{'a', 'e', 'o', 'A', '0', ' ', 'क', 'ক', 'ก', 'ກ', 0x1100, '가', 'א', 'ب', '日', 0x25cc, 0x0378}
	marks := []rune{0x0300, 0x0301, 0x0308, 0x0323, 0x0338, 0x093c, 0x093e, 0x094d, 0x0946,
		0x09be, 0x0e31, 0x0e33, 0x0e48, 0x0eb3, 0x05b8, 0x064e, 0x0651, 0x302e,
		0xfe0f, 0xfe00, 0x200d, 0x200c, 0x1161, 0x11a8, 0x20e3}
	for _, b := range bases {
		for _, m := range marks {
			texts = append(texts, string([]rune{b, m}), string([]rune{b, m, m}))
		}
	}
	return texts
}

// missingGlyphsFonts is every font the checkout holds and every corpus the
// environment names, as the paths a face is loaded from.
func missingGlyphsFonts(t *testing.T) []string {
	t.Helper()
	var paths []string
	dirs := []string{"../testdata/harfbuzz", "../testdata/freetype", "../fonts/notosans"}
	for _, env := range []string{"NOTO_FONTS", "NOTO_CJK", "CFF_FONTS", "EMOJI_FONTS"} {
		if dir := os.Getenv(env); dir != "" {
			dirs = append(dirs, dir)
		} else {
			t.Logf("%s is not set; its fonts are not compared", env)
		}
	}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			switch strings.ToLower(filepath.Ext(path)) {
			case ".ttf", ".otf", ".ttc", ".otc":
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", dir, err)
		}
	}
	return paths
}

// sameMissingCount compares the two answers for every text, on a face nothing
// has shaped yet — so the count is asked of caches no ShapeGlyphs call has
// filled — and checks the count recorded nothing.
func sameMissingCount(t *testing.T, name string, f *Face, texts []string) (compared int) {
	t.Helper()
	counts := make([]int, len(texts))
	for i, s := range texts {
		counts[i] = f.MissingGlyphs(s)
	}
	if used := f.Used(); len(used) != 0 {
		t.Errorf("%s: MissingGlyphs recorded %d glyphs as used, e.g. %v; a face "+
			"that was only asked has set nothing", name, len(used), used[:min(5, len(used))])
	}
	failures := 0
	for i, s := range texts {
		_, want := f.ShapeGlyphs(s)
		if counts[i] != want {
			failures++
			if failures <= 5 {
				t.Errorf("%s: %+q: MissingGlyphs says %d missing and ShapeGlyphs %d",
					name, s, counts[i], want)
			}
		}
	}
	if failures > 5 {
		t.Errorf("%s: and %d more", name, failures-5)
	}
	return len(texts)
}

// tooSlowToShapeEverything is the fonts the comparison leaves out, and why.
//
// TestMORXThirtysix is HarfBuzz's fixture for a morx state machine that runs
// until its limit, and ShapeGlyphs takes four seconds for a single "A" in it:
// half an hour for the text set above. That is the shaper's cost and not
// MissingGlyphs', whose answer for the font is a cmap lookup.
var tooSlowToShapeEverything = map[string]string{
	"TestMORXThirtysix.ttf": "ShapeGlyphs takes seconds per character in it",
}

// TestMissingGlyphsIsShapeGlyphsCount compares the two over every font the
// checkout and the corpora hold, the standard faces, and a face set by code.
func TestMissingGlyphsIsShapeGlyphsCount(t *testing.T) {
	texts := missingGlyphsTexts()
	faces, compared := 0, 0
	for _, path := range missingGlyphsFonts(t) {
		if why, slow := tooSlowToShapeEverything[filepath.Base(path)]; slow {
			t.Logf("%s is not compared: %s", filepath.Base(path), why)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(data)
		if err != nil {
			// The harness fonts include ones made to be refused.
			continue
		}
		faces++
		compared += sameMissingCount(t, filepath.Base(path), f, texts)
		// The same program set by code, where it can be: shapeByCode's count.
		if s, err := LoadSimple(data); err == nil {
			faces++
			compared += sameMissingCount(t, filepath.Base(path)+" (simple)", s, texts)
		}
	}
	for _, name := range StandardNames() {
		f, err := Standard(name)
		if err != nil {
			t.Fatal(err)
		}
		faces++
		compared += sameMissingCount(t, name, f, texts)
	}
	// The harness fonts alone are about ninety; fewer means the walk found
	// less than it should and the comparison above is thinner than it reads.
	if faces < 80 {
		t.Fatalf("only %d faces were compared", faces)
	}
	t.Logf("%d faces, %d texts each, %d comparisons", faces, len(texts), compared)
}

// TestMissingGlyphsOverGoogleFonts is the same comparison over every font in
// Google's collection, where it is linked in at ../testdata/googlefonts, with
// the mixed strings only: the collection is four thousand fonts.
func TestMissingGlyphsOverGoogleFonts(t *testing.T) {
	root := "../testdata/googlefonts/"
	if _, err := os.Stat(root); err != nil {
		t.Skip("the Google Fonts collection is not linked at ../testdata/googlefonts")
	}
	var texts []string
	for _, s := range missingGlyphsTexts() {
		if utf8.RuneCountInString(s) > 1 {
			texts = append(texts, s)
		}
	}
	faces := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ttf", ".otf":
		default:
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		f, err := Load(data)
		if err != nil {
			return nil
		}
		faces++
		sameMissingCount(t, path, f, texts)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d faces, %d texts each", faces, len(texts))
}

// FuzzMissingGlyphsAgreesWithShaping is the comparison over texts nobody wrote
// down, in the bundled Noto Sans and in the harness fonts that take the paths
// it does not: an Arabic face, a face the universal model sets, a morx face,
// and one built around the characters nothing is drawn for.
func FuzzMissingGlyphsAgreesWithShaping(f *testing.F) {
	var faces [][]byte
	for _, path := range []string{
		"../fonts/notosans/NotoSans-Variable.ttf",
		"../testdata/harfbuzz/fonts/NotoSansArabic.ttf",
		"../testdata/harfbuzz/fonts/NotoSansKhmer.ttf",
		"../testdata/harfbuzz/fonts/MorxCases.ttf",
		"../testdata/harfbuzz/fonts/Ignorables.ttf",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			f.Fatal(err)
		}
		faces = append(faces, data)
	}
	for i, s := range missingGlyphsTexts()[:200] {
		f.Add(uint8(i), s)
	}
	f.Fuzz(func(t *testing.T, which uint8, text string) {
		if len(text) > 1024 {
			return
		}
		face, err := Load(faces[int(which)%len(faces)])
		if err != nil {
			t.Fatal(err)
		}
		sameMissingCount(t, "fuzz", face, []string{text})
	})
}
