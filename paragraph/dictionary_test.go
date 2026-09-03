package paragraph

import "testing"

// The scripts whose words a dictionary finds, and what is done for the ones
// this engine has no dictionary for.
//
// UAX #14 gives them class SA and LB1 resolves it to AL — "no opportunity
// anywhere" — with a note that an implementation holding a dictionary does
// better. CSS Text §5.1 does not accept the resolution either way: "some form
// of fallback line breaking must occur even if the UA doesn't know how to
// perform it correctly. Overflowing is not allowed."
//
// So a script with no vocabulary here breaks between typographic character
// units, which is a place the words are not — and the engine reports it as well
// as does it, see UnsupportedScript. A script with one breaks where its words
// are, and there is nothing to report; see dictionarybreak_test.go, which is
// about that half.
//
// The fallback is not only for the scripts with no list at all. A stretch of
// Thai the vocabulary does not have falls back the same way and for the same
// reason, which is what the first test below is: three Thai letters that are
// not a word.

// TestADictionaryScriptBreaksBetweenItsCharacters.
func TestADictionaryScriptBreaksBetweenItsCharacters(t *testing.T) {
	for _, tc := range []struct{ what, text, want string }{
		{"Thai", "กขค", "ก|ข|ค"},
		{"Lao", "ກຂຄ", "ກ|ຂ|ຄ"},
		{"Khmer", "កខគ", "ក|ខ|គ"},
		{"Myanmar", "ကခဂ", "က|ခ|ဂ"},
		// Tai Tham, which has no vocabulary here and none to have: the four
		// scripts above fall back this way only where their own vocabulary has
		// nothing, and this one falls back everywhere.
		{"Tai Tham", "ᨠᨡᨢ", "ᨠ|ᨡ|ᨢ"},
	} {
		if got := marks(t, tc.text, WordBreak{}, LineBreak{}); got != tc.want {
			t.Errorf("%s: %q breaks as %q, want %q — without a dictionary the "+
				"only fallback is the character boundary, and overflowing is "+
				"not allowed", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestADictionaryScriptDoesNotBreakInsideACharacter. The unit is the
// typographic character unit and not the code point: a Thai letter carries its
// vowel signs and tone marks with it.
func TestADictionaryScriptDoesNotBreakInsideACharacter(t *testing.T) {
	// KO KAI with SARA I above it and MAI EK over that, then KHO KHAI.
	const text = "กิ่ข"
	if got := marks(t, text, WordBreak{}, LineBreak{}); got != "กิ่|ข" {
		t.Errorf("%q breaks as %q, want %q — the marks belong to the letter "+
			"under them", text, got, "กิ่|ข")
	}
}

// TestAScriptWithSpacesIsNotOneOfThem. Class SA is a list of scripts and not
// "everything that is not Latin": a rule that read it that way would break
// Greek, Cyrillic, Hebrew, Devanagari and Ethiopic words in half.
func TestAScriptWithSpacesIsNotOneOfThem(t *testing.T) {
	for _, text := range []string{"abc", "αβγ", "абв", "אבג", "एकदो", "ኵሉሰብእ"} {
		if got := marks(t, text, WordBreak{}, LineBreak{}); got != text {
			t.Errorf("%q breaks as %q, want no opportunity in it", text, got)
		}
	}
	if NeedsDictionaryBreaking('a') || NeedsDictionaryBreaking('क') {
		t.Error("a Latin or Devanagari letter is in the dictionary class")
	}
}

// TestKeepAllSuppressesTheFallbackToo, for the reason it suppresses every other
// implicit opportunity between letter units: §5.2 says so.
func TestKeepAllSuppressesTheFallbackToo(t *testing.T) {
	const text = "กขค"
	if got := marks(t, text, WordBreak{KeepAll: true}, LineBreak{}); got != text {
		t.Errorf("under keep-all %q breaks as %q, want no opportunity in it",
			text, got)
	}
}

// TestTheReportSaysWhatWasDone. An author whose paragraph is broken in the
// wrong places should be told, and the message has to describe what happened
// rather than what used to.
func TestTheReportSaysWhatWasDone(t *testing.T) {
	// Tai Tham, which has no word list here and none published anywhere. The
	// four scripts that do have one are not reported — that is the other half,
	// and it is asserted where the dictionary is.
	msg, ok := UnsupportedScript('ᨠ')
	if !ok {
		t.Fatal("a Tai Tham letter is reported as needing nothing")
	}
	if !contains(msg, "between typographic character units") {
		t.Errorf("the report reads %q; it has to say where the line is broken "+
			"instead", msg)
	}
	if _, ok := UnsupportedScript('a'); ok {
		t.Error("a Latin letter is reported as needing a dictionary")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
