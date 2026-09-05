package paragraph

import "testing"

// §2.2's virtual word separators in a language whose words come from a
// dictionary rather than from a phrase model.
//
// Every test here names the language as well as the text, and that is the point:
// the dictionary is keyed by script and would answer for any Thai characters at
// all, while the rule is about the *content language*. A document that declares
// none must get no separators, however its characters read.

const thaiSentence = "กรุงเทพคือสวยงาม"

// TestAWordBoundaryIsWhereAThaiSeparatorGoes. Thai is written without spaces
// between its words, and the dictionary that already decides where a line may be
// divided decides where a separator goes. The suite says so from the other side:
// word-space-transform-018 writes this sentence and its reference puts a space
// after กรุงเทพ and after คือ, which is exactly the segmentation below.
func TestAWordBoundaryIsWhereAThaiSeparatorGoes(t *testing.T) {
	got := SeparatorBoundaries(thaiSentence, "th", WritingSystemOther)
	want := map[int]bool{
		len("กรุงเทพ"):    true,
		len("กรุงเทพคือ"): true,
	}
	for at, boundary := range want {
		if got[at] != boundary {
			t.Errorf("offset %d is %v, want %v — that is where a word ends",
				at, got[at], boundary)
		}
	}
	for at, boundary := range got {
		if boundary && !want[at] {
			t.Errorf("offset %d was called a boundary and is inside a word", at)
		}
	}
}

// TestAnUntaggedDocumentGetsNoSeparators is §2.2's own gate: "if the content
// language is unknown, or if the user agent does not support detecting phrase
// boundaries for that language, there are no virtual expandable separators".
//
// It is the half a script-keyed dictionary cannot answer on its own, and the
// half that would go wrong silently: untagged Thai is still Thai to the eye.
func TestAnUntaggedDocumentGetsNoSeparators(t *testing.T) {
	for _, lang := range []Language{"", "en", "zh", "fr"} {
		if got := SeparatorBoundaries(thaiSentence, lang, WritingSystemOther); len(got) != 0 {
			t.Errorf("lang=%q found %v; §2.2 gives separators only where the "+
				"content language is one the engine models", lang, got)
		}
	}
}

// TestTheDictionaryIsNotOfferedAsAPhraseModel.
//
// A dictionary knows words and "auto-phrase" line breaking suppresses breaks
// *inside a phrase*, which is a coarser thing — a Japanese bunsetsu is several
// words. Reading a word list as a phrase model would claim knowledge it does not
// have, in the direction that glues text together, so PhraseBreaks must go on
// saying it has nothing to say about Thai.
func TestTheDictionaryIsNotOfferedAsAPhraseModel(t *testing.T) {
	if got := PhraseBreaks(thaiSentence, WritingSystemOther); len(got) != 0 {
		t.Errorf("PhraseBreaks answered for Thai with %v; the dictionary is a word "+
			"model and phrase-based line breaking must not read it as one", got)
	}
	if HasPhraseModel(WritingSystemOther) {
		t.Error("a writing system with no phrase model reports that it has one")
	}
	// And the separator source does answer, which is what makes the line above
	// a distinction rather than a way of saying nothing works.
	if got := SeparatorBoundaries(thaiSentence, "th", WritingSystemOther); len(got) == 0 {
		t.Error("SeparatorBoundaries found no Thai word boundaries, so the test " +
			"above proves nothing")
	}
}

// TestAPhraseModelWinsOverADictionary. Japanese has both a model and, for any
// Thai in the same run, a dictionary. The model is the answer §2.2 defers to and
// is asked first.
func TestAPhraseModelWinsOverADictionary(t *testing.T) {
	const ja = "東京へ行きましょう。"
	got := SeparatorBoundaries(ja, "ja", WritingSystemJapanese)
	if !got[len("東京へ")] {
		t.Errorf("the Japanese model found %v, want a boundary after 東京へ", got)
	}
	// The same text with no model available falls to the dictionary, which has
	// no Japanese in it and so finds nothing.
	if got := SeparatorBoundaries(ja, "ja", WritingSystemOther); len(got) != 0 {
		t.Errorf("without the model the boundaries were %v, want none", got)
	}
}

// TestTheFourDictionaryLanguagesAreTheFourDictionaries. The table is a claim
// about what is checked into the repository, and a language named there without
// a word list would be a promise the engine cannot keep.
func TestTheFourDictionaryLanguagesAreTheFourDictionaries(t *testing.T) {
	// Each sample is three words taken from the list itself and run together,
	// so that a dictionary which is present but never consulted is caught here
	// rather than being taken on trust.
	for lang, sample := range map[Language]string{
		"th": "กรุงเทพคือสวยงาม",
		"lo": "ກງສູນກຣັງລີເອີກຣາຟິກ",
		"km": "កកកុញកកកុះកកឈាម",
		"my": "ကကတစ်ကကတိုးကကုသန်ဘုရား",
	} {
		if !HasWordDictionary(lang) {
			t.Errorf("%q has a word list and is not named", lang)
			continue
		}
		if len(DictionaryBreaks(sample)) == 0 {
			t.Errorf("%q is named and its dictionary found no boundary in %q, so "+
				"the entry is a promise with nothing behind it", lang, sample)
		}
	}
	for _, lang := range []Language{"", "en", "ja", "zh", "ko"} {
		if HasWordDictionary(lang) {
			t.Errorf("%q is named as having a word list and has none", lang)
		}
	}
}
