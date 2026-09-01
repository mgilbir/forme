package paragraph

import "testing"

// The scripts whose words a dictionary would find, and what is done without one.
//
// UAX #14 gives them class SA and LB1 resolves it to AL — "no opportunity
// anywhere" — with a note that an implementation holding a dictionary does
// better. CSS Text §5.1 does not accept the resolution either way: "some form
// of fallback line breaking must occur even if the UA doesn't know how to
// perform it correctly. Overflowing is not allowed."
//
// So a line may end between two typographic character units of such a script.
// That is a place the words are not, which is why the engine reports it as well
// as does it — see UnsupportedScript.

// TestADictionaryScriptBreaksBetweenItsCharacters.
func TestADictionaryScriptBreaksBetweenItsCharacters(t *testing.T) {
	for _, tc := range []struct{ what, text, want string }{
		{"Thai", "กขค", "ก|ข|ค"},
		{"Lao", "ກຂຄ", "ກ|ຂ|ຄ"},
		{"Khmer", "កខគ", "ក|ខ|គ"},
		{"Myanmar", "ကခဂ", "က|ခ|ဂ"},
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

// TestTheReportSaysWhatWasDone. An author whose Thai paragraph is broken in the
// wrong places should be told, and the message has to describe what happened
// rather than what used to.
func TestTheReportSaysWhatWasDone(t *testing.T) {
	msg, ok := UnsupportedScript('ก')
	if !ok {
		t.Fatal("a Thai letter is reported as needing nothing")
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
