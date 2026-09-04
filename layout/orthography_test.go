package layout

import (
	"strconv"
	"strings"
	"testing"
)

// CSS Text §6.3's language-specific hyphenation, over a document.
//
// paragraph/orthography_test.go states the rules over the characters at a
// break. What a document adds is the three places the answer is read: the text
// printed at the end of the broken line, the face it is printed in, and the
// character taken off the start of the next one.
//
// Courier at 20px, whose advance is 600 units of 1000, so a width in characters
// is exact. It is also a face without U+2010, which is why the hyphens below are
// U+002D — see hyphenTextFor.

// hyphenatedLines lays out one word in a box that many characters wide and
// returns its lines with the marks that draw nothing taken out.
//
// The soft hyphen is still in the run: SplitAtBreaks keeps it, the shaper gives
// it no glyph and no advance, and what is asserted here is what a reader sees.
func hyphenatedLines(t *testing.T, chars int, markup, css string) []string {
	t.Helper()
	root := layoutOf(t, 600, `<div id="d">`+markup+`</div>`,
		noDefaults+`#d { font-family: Courier; font-size: 20px; width: `+
			strconv.Itoa(chars*12)+`px; hyphens: manual } `+css)
	var out []string
	for _, line := range lineTextsOf(t, root, "d") {
		out = append(out, strings.Map(func(r rune) rune {
			if isDefaultIgnorable(r) {
				return -1
			}
			return r
		}, line))
	}
	return out
}

func TestAWordIsBrokenTheWayItsLanguageBreaksIt(t *testing.T) {
	for _, tc := range []struct {
		chars int
		lang  string
		word  string
		want  []string
		what  string
	}{
		{5, "en", "un&shy;broken", []string{"un-", "broken"},
			"English, which is the row that needs nothing"},
		{5, "hu", "&Ouml;s&shy;szeg", []string{"Ösz-", "szeg"},
			"Hungarian writes the doubled digraph out — hyphens-i18n-manual-002"},
		{3, "zh-Latn-pinyin", "t&uacute;&shy;&#x2019;&agrave;n", []string{"tú-", "àn"},
			"pinyin drops the syllable separator — hyphens-i18n-manual-003"},
		{5, "", "&Ouml;s&shy;szeg", []string{"Ös-", "szeg"},
			"with no language declared, no rule applies"},
		{5, "de", "&Ouml;s&shy;szeg", []string{"Ös-", "szeg"},
			"and none in a language that has no rule here"},
	} {
		got := hyphenatedLines(t, tc.chars, `<span lang="`+tc.lang+`">`+tc.word+`</span>`, "")
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s\nlang=%q %s in %dch: got %q, want %q",
				tc.what, tc.lang, tc.word, tc.chars, got, tc.want)
		}
	}
}

// The letters a language restores are letters of the word, so the hyphen beside
// them is chosen for the face they are set in.
//
// The face for a hyphen is found for the hyphen, and the two Latin hyphens are
// chosen between by asking whether that face has the better one — so a "z" the
// language restored was carried along to whatever in the fallback stack has
// U+2010, and the word came out with a U+2010 in a face that draws it as
// nothing. Asking with the letter in front of it keeps both in the word's own
// face, which for Courier means U+002D.
//
// It needs a fallback library to be observable at all: with only the standard
// fourteen there is nowhere else for the hyphen to go, and both readings give
// the same answer.
func TestTheLettersALanguageRestoresChooseTheHyphenBesideThem(t *testing.T) {
	faces := kernFaces(t)
	frag := kernLayout(t, faces, `<div id="d" lang="hu">&Ouml;s&shy;szeg</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 60px; hyphens: manual }`)
	got := lineTexts(linesOf(t, frag, "d"))
	for i, line := range got {
		got[i] = strings.Map(func(r rune) rune {
			if isDefaultIgnorable(r) {
				return -1
			}
			return r
		}, line)
	}
	want := []string{"Ösz-", "szeg"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the word came out %q, want %q — the hyphen was chosen for a "+
			"face the restored letter is not in", got, want)
	}
}

// A character the language would drop is only dropped where the line actually
// breaks there. It is a rendering effect and not an edit: §6.3 says hyphenation
// "must have no effect on the underlying document content".
func TestTheDroppedCharacterComesBackWhenTheLineDoesNotBreak(t *testing.T) {
	got := hyphenatedLines(t, 20,
		`<span lang="zh-Latn-pinyin">t&uacute;&shy;&#x2019;&agrave;n</span>`, "")
	if len(got) != 1 || got[0] != "tú’àn" {
		t.Errorf("in a box with room the word is %q, want the whole of it — the "+
			"apostrophe goes only where the hyphen takes its place", got)
	}
}

// And the document has the last word about the character, which is what
// §6.3.2's hyphenate-character is for.
func TestTheDocumentOverrulesTheLanguagesHyphen(t *testing.T) {
	got := hyphenatedLines(t, 5, `<span lang="hu">&Ouml;s&shy;szeg</span>`,
		`#d { hyphenate-character: "=" }`)
	if len(got) == 0 || got[0] != "Ösz=" {
		t.Errorf("the first line is %q, want \"Ösz=\" — the author's own "+
			"character, with the language's spelling change still applied", got)
	}
}

// hyphenCharacter says whether the author supplied one, which is what lets a
// language fill the gap where they did not. "Nothing said" and "said that
// nothing is to be printed" are two different answers.
func TestTheEmptyHyphenCharacterIsAnAnswerAndNotASilence(t *testing.T) {
	for _, tc := range []struct {
		value string
		text  string
		said  bool
	}{
		{"", "", false},
		{"auto", "", false},
		{"AUTO", "", false},
		{`"="`, "=", true},
		{`""`, "", true},
		{`"a" "b"`, "", false},
		{"nonsense", "", false},
	} {
		text, said := hyphenCharacter(tc.value)
		if text != tc.text || said != tc.said {
			t.Errorf("hyphenate-character: %s -> %q,%v, want %q,%v",
				tc.value, text, said, tc.text, tc.said)
		}
	}
}

// Uyghur hyphenates with U+0640 ARABIC TATWEEL: the stroke that extends a
// joined letter, which is what the Arabic script draws at the end of a divided
// word. A hyphen there would be a Latin mark in the middle of a Uyghur word.
//
// It needs a face that has the letters, so it needs the fallback library — the
// standard fourteen have no Arabic — and it is the one row of §6.3's table
// where the character itself is the whole of the rule.
func TestUyghurHyphenatesWithATatweel(t *testing.T) {
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "داميىـ"); !ok {
		t.Skip("no face here can set the fixture")
	}
	const css = `#d { font-size: 20px; hyphens: manual }`
	// Exactly as wide as the half that ends the line, so the word has to break
	// and there is no room for anything else.
	width := cjkNaturalWidth(t, faces,
		`<div id="d" lang="ug" dir="rtl">دامي</div>`, css)
	frag := kernLayout(t, faces,
		`<div id="d" lang="ug" dir="rtl">دامي&shy;دى</div>`,
		css+` #d { width: `+fmtPx(width)+` }`)
	lines := lineTexts(linesOf(t, frag, "d"))
	if len(lines) != 2 {
		t.Fatalf("the word came out on %d lines: %q", len(lines), lines)
	}
	if !strings.ContainsRune(lines[0], 'ـ') {
		t.Errorf("the broken half is %q, want it to end at a tatweel", lines[0])
	}
	for _, wrong := range []rune{'‐', '-'} {
		if strings.ContainsRune(lines[0], wrong) {
			t.Errorf("the broken half is %q — it was hyphenated with %q, which is "+
				"a Latin mark in the middle of a Uyghur word", lines[0], wrong)
		}
	}
}
