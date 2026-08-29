package paragraph

import "testing"

// The writing system, and the second sentence of §4.1.1 it lets this engine
// read.
//
// The first sentence removes a segment break between two wide characters. The
// second removes one where only *one* side is wide and the other is the
// punctuation East Asian text is written with — a quotation mark, whose East
// Asian Width is Ambiguous and which the first sentence therefore says nothing
// about.

func TestTheScriptSubtagDecidesTheWritingSystem(t *testing.T) {
	for _, tc := range []struct {
		tag  string
		want WritingSystem
		why  string
	}{
		// The suite's own case: Ainu written in katakana. The content language
		// is not Japanese and the writing system is.
		{"ain-Kana", WritingSystemJapanese, "a script subtag beats the language"},
		// And the other way, which is why the language alone will not do.
		{"ja-Latn", WritingSystemOther, "Japanese romanised is written with spaces"},
		{"ja", WritingSystemJapanese, "no script subtag, so the language answers"},
		{"ja-JP", WritingSystemJapanese, "a region is not a script"},
		{"JA", WritingSystemJapanese, "a tag is not case sensitive"},
		{"zh", WritingSystemChinese, ""},
		{"zh-Hant", WritingSystemChinese, ""},
		{"und-Hani", WritingSystemChinese, "an undetermined language in Han"},
		{"ii", WritingSystemYi, ""},
		// Korean is not one of the three, and the Hangul script is not either:
		// Korean is written with spaces between its words.
		{"ko", WritingSystemOther, ""},
		{"ko-Hang", WritingSystemOther, ""},
		{"en", WritingSystemOther, ""},
		{"", WritingSystemOther, "a document that says nothing"},
	} {
		if got := WritingSystemOf(tc.tag); got != tc.want {
			t.Errorf("%q gave %d, want %d — %s", tc.tag, got, tc.want, tc.why)
		}
	}
	if WritingSystemOther.SpacesNoWords() {
		t.Error("the rule applies to a writing system it does not name")
	}
	for _, w := range []WritingSystem{WritingSystemChinese, WritingSystemJapanese, WritingSystemYi} {
		if !w.SpacesNoWords() {
			t.Errorf("%d is one of the three the rule names", w)
		}
	}
}

// collapsedIn is Phase I over a scrap of text in a writing system.
func collapsedIn(text string, w WritingSystem) string {
	return CollapseWhitespaceAfter(text, "normal", WordSpaceTransform{}, Boundary{}, w)
}

// TestAQuotationMarkTakesTheSegmentBreakWithIt is the rule, and the fixture is
// the suite's: “ is punctuation whose East Asian Width is Ambiguous, next to
// katakana, which is Wide.
func TestAQuotationMarkTakesTheSegmentBreakWithIt(t *testing.T) {
	if got := collapsedIn("“\nア", WritingSystemJapanese); got != "“ア" {
		t.Errorf("got %q, want \"“ア\" — the break between the quote and the "+
			"katakana is not a word boundary", got)
	}
	// Both ways round: "before or after ... the other side" is symmetric.
	if got := collapsedIn("ア\n”", WritingSystemJapanese); got != "ア”" {
		t.Errorf("got %q, want \"ア”\" — the punctuation is on the far side here", got)
	}
	// And the same text in a writing system the sentence does not name keeps
	// its break, which is the whole reason the writing system is asked about.
	if got := collapsedIn("“\nア", WritingSystemOther); got != "“ ア" {
		t.Errorf("got %q, want \"“ ア\" — an English document keeps the space", got)
	}
}

// TestTheOtherSideHasToBeWide. The sentence asks for F, W or H there, and a
// Latin letter is none of them: a quote against an English word is an ordinary
// line wrap and the space belongs.
func TestTheOtherSideHasToBeWide(t *testing.T) {
	if got := collapsedIn("“\na", WritingSystemJapanese); got != "“ a" {
		t.Errorf("got %q, want \"“ a\"", got)
	}
}

// TestHangulIsCarvedOutOfThisSentenceToo, for the reason it is carved out of the
// first: Korean is written with spaces between its words, so a break beside a
// syllable is a word boundary however the other side is classified.
func TestHangulIsCarvedOutOfThisSentenceToo(t *testing.T) {
	if got := collapsedIn("“\n한", WritingSystemJapanese); got != "“ 한" {
		t.Errorf("got %q, want \"“ 한\"", got)
	}
}

// TestAnEmojiIsBothSidesOfTheSentence. It appears in both halves and does
// opposite things in them: on the near side it is one of the characters that
// triggers the rule, and on the far side it is carved out of the wide ones that
// would otherwise satisfy it.
func TestAnEmojiIsBothSidesOfTheSentence(t *testing.T) {
	// Near side: an emoji is a symbol and is Emoji, so it stands where the
	// quotation mark stood.
	//
	// "#" and not a face, and that is what makes it a test of the emoji clause:
	// U+0023 is Emoji and is *not* Ambiguous and *not* wide, so neither the
	// other half of this sentence nor the sentence before it can reach it. A
	// smiling face is wide, and the first sentence would join it to the
	// katakana whatever this one said.
	if got := collapsedIn("#\nア", WritingSystemJapanese); got != "#ア" {
		t.Errorf("got %q, want \"#ア\" — U+0023 is Emoji, which is what the "+
			"sentence asks for where the width does not answer", got)
	}
	// Far side: an emoji is wide, and the sentence takes it back out. A break
	// between a quotation mark and a picture is a break between two things.
	if got := collapsedIn("“\n\U0001F600", WritingSystemJapanese); got != "“ \U0001F600" {
		t.Errorf("got %q, want the space kept", got)
	}
}

// TestAnAmbiguousLetterIsNotPunctuation is the containment on the near side.
// Greek is Ambiguous-width and is not punctuation or a symbol, so the sentence
// says nothing about it — reading the width alone would join a Greek letter to
// the katakana after it.
func TestAnAmbiguousLetterIsNotPunctuation(t *testing.T) {
	if got := collapsedIn("α\nア", WritingSystemJapanese); got != "α ア" {
		t.Errorf("got %q, want \"α ア\"", got)
	}
}

// TestTheFirstSentenceIsUnchanged. Two wide characters either side of a break
// need no writing system: that rule is about the characters and nothing else,
// and it answered the same way before this one existed.
func TestTheFirstSentenceIsUnchanged(t *testing.T) {
	for _, w := range []WritingSystem{WritingSystemOther, WritingSystemJapanese} {
		if got := collapsedIn("ア\nイ", w); got != "アイ" {
			t.Errorf("in writing system %d: got %q, want \"アイ\"", w, got)
		}
	}
}

// TestChineseOrJapaneseIsNotTheSameCarveOutAsSpacesNoWords, which is the reason
// there are two predicates and not one.
//
// §4.1.1's segment break rule says "Chinese, Japanese, or Yi" and §5.3's line
// break tailoring says "in Chinese and Japanese". Yi is the difference, and it
// is a difference the specification states rather than one this engine chose,
// so reusing either predicate for the other rule would quietly widen or narrow
// a carve-out that two working groups wrote deliberately.
func TestChineseOrJapaneseIsNotTheSameCarveOutAsSpacesNoWords(t *testing.T) {
	for _, tc := range []struct {
		system             WritingSystem
		spacesNoWords, cjk bool
		what               string
	}{
		{WritingSystemChinese, true, true, "Chinese"},
		{WritingSystemJapanese, true, true, "Japanese"},
		{WritingSystemYi, true, false, "Yi, which §4.1.1 names and §5.3 does not"},
		{WritingSystemOther, false, false, "anything else"},
	} {
		if got := tc.system.SpacesNoWords(); got != tc.spacesNoWords {
			t.Errorf("%s: SpacesNoWords is %v, want %v", tc.what, got, tc.spacesNoWords)
		}
		if got := tc.system.ChineseOrJapanese(); got != tc.cjk {
			t.Errorf("%s: ChineseOrJapanese is %v, want %v", tc.what, got, tc.cjk)
		}
	}
}
