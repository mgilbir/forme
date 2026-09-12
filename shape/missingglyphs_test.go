package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// What the second result of ShapeGlyphs counts.
//
// It is the characters the face could not draw, and a caller's font fallback
// reads it to decide whether a face can set a run at all — so a character
// counted here that was never going to be drawn sends the text to a worse face.
//
// That is what happened to every emoji sequence. An emoji written with a zero
// width joiner in it — "👨‍💻", "👩‍👧‍👦", the flag of Wales — is *one* grapheme
// cluster, so the fallback asks one face for the whole of it; the joiner reaches
// the glyph loop because the joining scan has to see it, and it has no glyph in
// any face that is not a text font. A face holding both emoji answered "one
// missing" and was passed over for one holding neither, which then set both as
// spaces.

// TestACharacterNothingDrawsIsNotOneTheFaceIsMissing.
func TestACharacterNothingDrawsIsNotOneTheFaceIsMissing(t *testing.T) {
	f := twoLetterFace(t)
	for _, c := range []struct {
		what, text string
		want       int
	}{
		{"nothing hidden", "ab", 0},
		// The join controls, which survive dropHiddenCharacters because the
		// joining scan reads them and are taken out again by hideJoiners.
		{"a zero width joiner", "a‍b", 0},
		{"a zero width non-joiner", "a‌b", 0},
		// And the rest of the default-ignorable characters, which are dropped
		// earlier and were never counted.
		{"a zero width space", "a​b", 0},
		{"a left-to-right mark", "a‎b", 0},
		// A character the face really has not got still counts: that is the
		// whole of what the number is for.
		{"a letter the face has not got", "acb", 1},
		{"a joiner beside one", "a‍cb", 1},
	} {
		if _, missing := f.ShapeGlyphs(c.text); missing != c.want {
			t.Errorf("%s: %q reported %d missing, want %d",
				c.what, c.text, missing, c.want)
		}
	}
}

// TestAHangulFillerIsDrawnAndStillCounts is the carve-out inside the carve-out.
//
// The fillers are default-ignorable by property and are *drawn*: a Hangul
// syllable written with one is a syllable with a blank where a jamo would be,
// and a face without the glyph cannot show it. hiddenAfterShaping is the list
// that says so, and using isDefaultIgnorable in its place would quietly stop
// reporting them.
func TestAHangulFillerIsDrawnAndStillCounts(t *testing.T) {
	f := twoLetterFace(t)
	if _, missing := f.ShapeGlyphs("aㅤb"); missing != 1 {
		t.Errorf("a Hangul filler reported %d missing, want 1: it is "+
			"default-ignorable and it is drawn", missing)
	}
}

// TestTheFallbackQuestionIsAnsweredForAnEmojiSequence is the defect as its
// caller sees it.
//
// A font set asks "can this face set the whole of this text" by shaping it and
// reading the count. The two emoji here are the only visible characters in the
// cluster, so a face holding both can set it — and said it could not.
func TestTheFallbackQuestionIsAnsweredForAnEmojiSequence(t *testing.T) {
	f := twoEmojiFace(t)
	const sequence = "\u26F9‍\u2640" // the shape of "⛹️‍♀️", without its modifiers
	glyphs, missing := f.ShapeGlyphs(sequence)
	if missing != 0 {
		t.Errorf("a face holding both emoji of %q reported %d missing; a "+
			"fallback reading that passes it over for a face holding neither",
			sequence, missing)
	}
	if len(glyphs) != 2 {
		t.Errorf("the sequence came out as %d glyphs, want 2: the joiner is "+
			"taken out before anything is drawn", len(glyphs))
	}
}

// twoLetterFace draws 'a' and 'b' and nothing else — no joiner, no space, no
// filler. Every fixture above is a character it has not got, which is what makes
// the count the only thing being measured.
func twoLetterFace(t *testing.T) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "TwoLetters",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 500, HasShape: true},
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// twoEmojiFace draws the two characters of an emoji sequence and not the joiner
// between them, which is every emoji font there is.
//
// The two are in the basic plane because the fixture builder writes a cmap that
// reaches no further, and the plane is not what is being tested: U+26F9 and
// U+2640 are the basketball player and the female sign of "⛹️‍♀️", which is the
// same shape of sequence as "👨‍💻" and the same question about the joiner in it.
func twoEmojiFace(t *testing.T) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "TwoEmoji",
		Glyphs: []fonttest.Glyph{
			{Rune: 0x26F9, Advance: 1000, HasShape: true},
			{Rune: 0x2640, Advance: 1000, HasShape: true},
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}
