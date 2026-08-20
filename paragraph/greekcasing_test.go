package paragraph

import "testing"

// Uppercasing Greek, which drops the accents, and capitalising Dutch, which does
// not drop a letter.
//
// Neither is in SpecialCasing.txt: the mappings there are the same in every
// language, and these two are tailorings CLDR carries for one. Every row below
// is one of the suite's own fixtures, and the four Greek ones are four different
// rules rather than four examples of one.

// TestGreekDropsItsAccentsInCapitals.
func TestGreekDropsItsAccentsInCapitals(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		// The accent goes. Greek is written with one on the stressed syllable
		// of every polysyllabic word and is not written with one in capitals.
		{"καλημέρα αύριο", "ΚΑΛΗΜΕΡΑ ΑΥΡΙΟ", "the tonos"},
		{"εγώ", "ΕΓΩ", "on the last letter"},
		{"ό,τι", "Ο,ΤΙ", "before a comma"},

		// The dialytika is not an accent and stays.
		{"θεϊκό", "ΘΕΪΚΟ", "the dialytika"},
		// Both together: the accent goes, the dialytika stays.
		{"ευφυΐα", "ΕΥΦΥΪΑ", "a dialytika and an accent on one letter"},

		// And one is *added*: the accent on the alpha was saying that the alpha
		// and the iota are two syllables rather than the diphthong "αι", so
		// removing it puts a dialytika on the iota to say the same thing.
		{"Νεράιδα", "ΝΕΡΑΪΔΑ", "a dialytika added where an accent was removed"},
		{"μαϊμού", "ΜΑΪΜΟΥ", "one that was already there"},
		// An accent on the *second* element is a diphthong with the stress on
		// it, and stays a diphthong: no dialytika.
		{"αύριο", "ΑΥΡΙΟ", "an accent on the second half of a diphthong"},
		// And where nothing was removed, nothing is added.
		{"και", "ΚΑΙ", "a diphthong with no accent at all"},

		// Text already in capitals, with the accents still on it, which is the
		// fault the rule is about as much as the lower-case case is.
		{"ΚΑΛΗΜΈΡΑ", "ΚΑΛΗΜΕΡΑ", "capitals that kept their accents"},
	} {
		if got := casedIn(t, tc.text, TransformUppercase, "el"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestTheDisjunctiveEtaKeepsItsAccent. "ή" on its own is the word for "or", and
// without its accent it would be "Η", the definite article.
func TestTheDisjunctiveEtaKeepsItsAccent(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"ήσουν ή εγώ ή εσύ", "ΗΣΟΥΝ Ή ΕΓΩ Ή ΕΣΥ", "the suite's own line"},
		{"ή", "Ή", "the word on its own"},
		{"α ή β", "Α Ή Β", "between two others"},
		{"ή,", "Ή,", "before a comma"},
		// Inside a word it is an ordinary eta and loses the accent like any
		// other, which is the whole distinction.
		{"ήσουν", "ΗΣΟΥΝ", "at the start of a word"},
		{"μή", "ΜΗ", "at the end of one"},
	} {
		if got := casedIn(t, tc.text, TransformUppercase, "el"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
}

// TestGreekIsTailoredOnlyForGreek is the containment case. The tailoring is
// CLDR's for the Greek *language*, so a Greek quotation in an English document
// keeps its accents — which is what a document that has not said otherwise gets.
func TestGreekIsTailoredOnlyForGreek(t *testing.T) {
	for _, lang := range []string{"", "en", "de", "tr"} {
		if got := casedIn(t, "καλημέρα", TransformUppercase, lang); got != "ΚΑΛΗΜΈΡΑ" {
			t.Errorf("in %q: %q, want the accent kept", lang, got)
		}
	}
	// And the language may be written with a region or a script after it.
	for _, lang := range []string{"el", "el-GR", "el-Grek", "EL"} {
		if got := casedIn(t, "καλημέρα", TransformUppercase, lang); got != "ΚΑΛΗΜΕΡΑ" {
			t.Errorf("in %q: %q, want the accent dropped", lang, got)
		}
	}
	// Lower-casing is not touched by any of it.
	if got := casedIn(t, "ΚΑΛΗΜΕΡΑ", TransformLowercase, "el"); got != "καλημερα" {
		t.Errorf("lowercasing gave %q", got)
	}
}

// TestNonGreekTextIsUntouchedByTheGreekRule, which is what keeps the whole of it
// off every other document: the run is checked for a Greek character before
// anything else happens.
func TestNonGreekTextIsUntouchedByTheGreekRule(t *testing.T) {
	for _, text := range []string{
		"hello", "straße", "日本語", "Здравствуйте", "", "123 !?",
	} {
		want, _ := TransformText(text, TransformUppercase, false, "")
		if got := casedIn(t, text, TransformUppercase, "el"); got != want {
			t.Errorf("%q in Greek: %q, want %q", text, got, want)
		}
	}
	if greekUppercase("hello") != "" {
		t.Error("a run with no Greek in it went through the tailoring")
	}
	if greekUppercase("καλημέρα") == "" {
		t.Error("a run with Greek in it did not")
	}
}

// TestDutchCapitalisesBothLettersOfIJ. IJ is one letter of the Dutch alphabet
// written as two, and "Ijsland" is as wrong as it would look in English.
func TestDutchCapitalisesBothLettersOfIJ(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"ijsland", "IJsland", "the suite's own word"},
		{"ijs", "IJs", "a short one"},
		{"ij", "IJ", "the letter alone"},
		{"een ijsje", "Een IJsje", "not at the start of the text"},
		{"IJsland", "IJsland", "already capitalised"},
		// A word beginning with i and not ij takes one capital like any other.
		{"ik", "Ik", "an i that is not an ij"},
		{"i", "I", "an i alone"},
		// And it is the *start* of a word: an ij inside one is two ordinary
		// letters.
		{"bijna", "Bijna", "an ij inside a word"},
	} {
		if got := casedIn(t, tc.text, TransformCapitalize, "nl"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
	// The suite writes the language tag in capitals in one of its two fixtures,
	// which is legal and means the same thing.
	if got := casedIn(t, "ijsland", TransformCapitalize, "NL"); got != "IJsland" {
		t.Errorf(`lang="NL" gave %q`, got)
	}
	// Every other language capitalises the i alone.
	for _, lang := range []string{"", "en", "de"} {
		if got := casedIn(t, "ijsland", TransformCapitalize, lang); got != "Ijsland" {
			t.Errorf("in %q: %q, want Ijsland", lang, got)
		}
	}
	// And the rule is titlecasing's alone: uppercasing gives two capitals in
	// every language and lowercasing has nothing to do.
	if got := casedIn(t, "ijsland", TransformUppercase, "nl"); got != "IJSLAND" {
		t.Errorf("uppercasing gave %q", got)
	}
	if got := casedIn(t, "IJSLAND", TransformLowercase, "nl"); got != "ijsland" {
		t.Errorf("lowercasing gave %q", got)
	}
}
