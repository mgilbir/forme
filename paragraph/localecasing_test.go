package paragraph

import (
	"strings"
	"testing"
)

// The case mappings that depend on the language or on the text around them.
//
// Every row below is one of SpecialCasing.txt's sixteen conditional entries,
// and the file's own line is quoted where the row is not obvious. This is not a
// nicety: Turkish and Azerbaijani have two i's, dotted and dotless, and they are
// different letters. Lowercasing "I" to "i" in Turkish spells a different word,
// which is the same class of error as lowercasing "O" to "e" in English.

// cased is the transform applied to one string in one language.
func casedIn(t *testing.T, text string, kind TextTransform, lang string) string {
	t.Helper()
	got, _ := TransformText(text, kind, false, LanguageOf(lang))
	return got
}

// TestTheDottedAndDotlessI, which is what tr and az are about.
func TestTheDottedAndDotlessI(t *testing.T) {
	for _, tc := range []struct {
		text, lang, want string
		kind             TextTransform
		what             string
	}{
		// "0069; 0069; 0130; 0130; tr" — an ordinary i uppercases to the dotted
		// capital, because the dotless one is a different letter.
		{"i", "tr", "İ", TransformUppercase, "i uppercases to a dotted capital"},
		{"i", "az", "İ", TransformUppercase, "the same in Azerbaijani"},
		{"i", "en", "I", TransformUppercase, "and to a plain capital in English"},
		{"i", "", "I", TransformUppercase, "and where the document said nothing"},

		// "0049; 0131; ...; tr Not_Before_Dot" — a capital I lowercases to the
		// dotless one.
		{"I", "tr", "ı", TransformLowercase, "I lowercases to a dotless i"},
		{"I", "az", "ı", TransformLowercase, "the same in Azerbaijani"},
		{"I", "en", "i", TransformLowercase, "and to a dotted one in English"},

		// "0130; 0069; ...; tr" — the dotted capital lowercases to a plain i,
		// where the unconditional mapping keeps the dot as a mark of its own.
		{"İ", "tr", "i", TransformLowercase, "the dotted capital in Turkish"},
		{"İ", "en", "i̇", TransformLowercase, "and in English"},

		// Not_Before_Dot and After_I together: "I" followed by a combining dot
		// is the dotted i written in two characters, so the I keeps its dot and
		// the mark goes rather than the other way round.
		{"İ", "tr", "i", TransformLowercase, "I and a combining dot above"},
		{"İ", "en", "i̇", TransformLowercase, "the same in English"},

		// The condition is about what *follows*, so a mark that is not the dot
		// does not trigger it.
		{"Í", "tr", "ı́", TransformLowercase, "I and an acute"},

		// And an intervening mark of class 0 or 230 breaks the run: the dot no
		// longer belongs to this I.
		{"Í̇", "tr", "ı́̇", TransformLowercase,
			"an acute between the I and the dot"},
	} {
		if got := casedIn(t, tc.text, tc.kind, tc.lang); got != tc.want {
			t.Errorf("%s (%s): %q became %q, want %q",
				tc.what, tc.lang, tc.text, got, tc.want)
		}
	}
}

// TestLithuanianKeepsTheDot. Lithuanian writes the dot on a lower-case i even
// under an accent, which is the opposite of what every other language does.
func TestLithuanianKeepsTheDot(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		// "0049; 0069 0307; ...; lt More_Above" and its two neighbours.
		{"Í", "i̇́", "I under an acute"},
		{"J́", "j̇́", "J under an acute"},
		{"Į́", "į̇́", "I with ogonek under an acute"},
		// The three precomposed letters, which carry the accent already.
		{"Ì", "i̇̀", "I with grave"},
		{"Í", "i̇́", "I with acute"},
		{"Ĩ", "i̇̃", "I with tilde"},
		// "0307; 0307; ; ; lt After_Soft_Dotted" — the *lower*case field is the
		// one that keeps the dot, which is the whole point of the language's
		// tailoring. See TestLithuanianDropsTheDotWhenItCapitalizes for the two
		// fields that remove it.
		{"i\u0307", "i\u0307", "an explicit dot after a soft-dotted letter"},
		{"i\u0307\u0301", "i\u0307\u0301", "and the same under an accent"},
		// Without an accent there is nothing to keep the dot from, so the
		// ordinary mapping applies.
		{"I", "i", "a bare I"},
		{"J", "j", "a bare J"},
	} {
		if got := casedIn(t, tc.text, TransformLowercase, "lt"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
	// And none of it happens in a language that is not Lithuanian.
	if got := casedIn(t, "Í", TransformLowercase, "en"); got != "í" {
		t.Errorf("in English the dot was kept: %q", got)
	}
}

// TestLithuanianDropsTheDotWhenItCapitalizes is the other half of the same line
// of SpecialCasing.txt, and the half this engine had backwards.
//
//	0307; 0307;     ;     ; lt After_Soft_Dotted
//
// The four fields are the character, its lowercase, its titlecase and its
// uppercase, and the two that are empty are the last two — which is what the
// file's own heading says in words: "Remove DOT ABOVE after \"i\" with upper or
// titlecase". A capital I has no dot of its own for a second one to sit above.
//
// It reads the other way round at a glance. The rule *sounds* like "Lithuanian
// keeps one dot and not two", which is true — of the lowercase, where the dot
// is what is kept.
func TestLithuanianDropsTheDotWhenItCapitalizes(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"i\u0307", "I", "the dot a lowercase i carries"},
		{"i\u0307\u0301", "I\u0301", "with an accent above it as well"},
		{"j\u0307", "J", "and on a j"},
		{"\u012f\u0307", "\u012e", "and on an i with an ogonek"},
		{"i\u0307i\u0307", "II", "twice over"},
		// The dot goes only where a soft-dotted letter is what it is above. The
		// suite's text-transform-upperlower-044 writes this row itself, with the
		// comment "check that dot isn't deleted in other contexts".
		{"x\u0307", "X\u0307", "a dot above something that is not soft-dotted"},
		{"a\u0307", "A\u0307", "and above an a"},
	} {
		if got := casedIn(t, tc.text, TransformUppercase, "lt"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
	// And nowhere else: English uppercases the letter and keeps the mark.
	if got := casedIn(t, "i\u0307", TransformUppercase, "en"); got != "I\u0307" {
		t.Errorf("in English the dot was dropped: %q", got)
	}
}

// TestTheFinalSigma is the one condition that is not about language at all.
//
// "03A3; 03C2; ...; Final_Sigma": a lower-case sigma ending a word is ς and one
// inside a word is σ, and a reader of Greek sees the difference immediately.
func TestTheFinalSigma(t *testing.T) {
	for _, tc := range []struct{ text, want, what string }{
		{"ΟΔΟΣ", "οδος", "at the end of a word"},
		{"ΣΟΦΟΣ", "σοφος", "at the start of one and at the end of another"},
		{"ΟΔΟΣ ΟΔΟΣ", "οδος οδος", "twice"},
		{"ΣΑ", "σα", "at the start, with a letter after it"},
		// A case-ignorable character after it does not make it non-final: an
		// apostrophe or a full stop is not a letter.
		{"ΟΔΟΣ.", "οδος.", "before a full stop"},
		{"ΟΔΟΣ'", "οδος'", "before an apostrophe"},
		// Nothing before it that is cased, so it is not final either.
		{"Σ", "σ", "on its own"},
		{".Σ", ".σ", "after a full stop"},
	} {
		if got := casedIn(t, tc.text, TransformLowercase, "el"); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
		// It is not a Greek-language rule: the same answer with no language.
		if got := casedIn(t, tc.text, TransformLowercase, ""); got != tc.want {
			t.Errorf("%s with no language: %q became %q, want %q",
				tc.what, tc.text, got, tc.want)
		}
	}
}

// TestALanguageIsAScriptAsWellAsALanguage.
//
// The suite forces this rather than leaving it to taste:
// writing-system-text-transform-001 is Turkish written in Cyrillic — lang="tr-Cyrl"
// — and asks for the I to lowercase to a *dotted* i. The dotless one is a letter
// of the Turkish Latin alphabet and there is no such letter in Cyrillic.
func TestALanguageIsAScriptAsWellAsALanguage(t *testing.T) {
	for _, tc := range []struct{ tag, want string }{
		{"tr", "tr"},
		{"TR", "tr"},
		{"tr-CY", "tr"},      // a region, not a script
		{"tr-Latn", "tr"},    // the script it is written in anyway
		{"tr-Latn-TR", "tr"}, // and with a region after it
		{"tr-Cyrl", ""},      // a different alphabet: no tailoring
		{"az-Cyrl", ""},      //
		// Which alphabet counts depends on the language: Greek drops its
		// accents in the Greek one, so "not Latin" would be the wrong test.
		{"el-Grek", "el"},
		{"el-Latn", ""},
		// A language with no tailoring is not asked about its script at all.
		{"ru-Cyrl", "ru"},
		{"en-Latn", "en"},
		{"lt-Latn", "lt"}, //
		{"", ""},          //
		{"  El  ", "el"},  // trimmed and lowercased
	} {
		if got := string(LanguageOf(tc.tag)); got != tc.want {
			t.Errorf("%q read as %q, want %q", tc.tag, got, tc.want)
		}
	}
	// And through the transform, which is what it is for.
	if got := casedIn(t, "I", TransformLowercase, "tr-Cyrl"); got != "i" {
		t.Errorf("Turkish in Cyrillic lowercased I to %q, want a dotted i", got)
	}
	if got := casedIn(t, "I", TransformLowercase, "tr"); got != "ı" {
		t.Errorf("Turkish in its own alphabet lowercased I to %q, want a dotless one", got)
	}
}

// TestNoLanguageChangesOrdinaryText is the containment case, and it is most of
// every document: text with none of these characters in it must come out of the
// conditional path exactly as it came out of the unconditional one.
func TestNoLanguageChangesOrdinaryText(t *testing.T) {
	// None of these holds an i, a j, a sigma, a combining dot or a Greek letter,
	// which is what makes them ordinary: a Turkish page really does uppercase
	// its i's differently and a Greek one really does drop its accents, so a
	// fixture containing either would be testing the tailoring rather than its
	// absence.
	for _, text := range []string{
		"hello world", "HELLO WORLD", "straße", "日本語", "abcdefgh", "ABCDEFGH",
		"", " ", "123", "Ölaf", "Здравствуйте", "ЗДРАВСТВУЙТЕ",
	} {
		for _, kind := range []TextTransform{TransformUppercase, TransformLowercase} {
			want, _ := TransformText(text, kind, false, "")
			for _, lang := range []string{"tr", "az", "lt", "el", "en", "de"} {
				if got := casedIn(t, text, kind, lang); got != want {
					t.Errorf("%q in %s: %q, want %q", text, lang, got, want)
				}
			}
		}
	}
}

// TestTheConditionalPathIsTakenOnlyWhenItCouldMatter. The whole of the
// conditional machinery is skipped unless the text holds a character one of the
// mappings is about, which is what keeps every other document on the path it was
// always on.
func TestTheConditionalPathIsTakenOnlyWhenItCouldMatter(t *testing.T) {
	for _, tc := range []struct {
		text  string
		upper bool
		want  bool
		what  string
	}{
		{"hello", false, false, "no I, no sigma, nothing"},
		{"HELLO", false, false, "capitals, but none of them an I"},
		{"HI", false, true, "a capital I"},
		{"ΟΔΟΣ", false, true, "a capital sigma"},
		{"hi", true, true, "a lower-case i, which Turkish uppercases specially"},
		{"hello", true, false, "no i at all"},
	} {
		got := firstConditional(tc.text, "tr", tc.upper) >= 0
		if got != tc.want {
			t.Errorf("%s: %q took the conditional path = %v, want %v",
				tc.what, tc.text, got, tc.want)
		}
	}
}

// TestTheConditionsScanPastTheRightThings. Each of the four is a scan with a
// stopping rule, and the stopping rule is where they differ.
func TestTheConditionsScanPastTheRightThings(t *testing.T) {
	// Not_Before_Dot: stop at combining class 0 or 230.
	if !beforeDot("̧̇") {
		t.Error("a cedilla (class 202) should not stop the scan for a dot above")
	}
	if beforeDot("́̇") {
		t.Error("an acute (class 230) should stop it")
	}
	if beforeDot("ȧ") {
		t.Error("a letter (class 0) should stop it")
	}
	// After_I: the same rule, backwards.
	if !afterI("I̧") {
		t.Error("a cedilla between the I and here should not stop the scan")
	}
	if afterI("Í") {
		t.Error("an acute should stop it")
	}
	if afterI("Ia") {
		t.Error("a letter should stop it")
	}
	// More_Above: stop at 0 or 230, and the answer is whether it was 230.
	if !moreAbove("́") {
		t.Error("an acute is a mark above")
	}
	if moreAbove("̧") {
		t.Error("a cedilla is not")
	}
	if moreAbove("") || moreAbove("a") {
		t.Error("nothing above is not a mark above")
	}
	// After_Soft_Dotted.
	if !afterSoftDotted("i") {
		t.Error("i is soft-dotted")
	}
	if afterSoftDotted("a") {
		t.Error("a is not")
	}
	if afterSoftDotted("í") {
		t.Error("an acute between them should stop the scan")
	}
}

// TestCasedAndCaseIgnorableAreUnicodesOwn, which Final_Sigma is stated over and
// which Go's tables do not carry by those names.
func TestCasedAndCaseIgnorableAreUnicodesOwn(t *testing.T) {
	for _, r := range []rune{'a', 'A', 'ǅ', 'α', 'Ω', 'ᵃ'} {
		if !cased(r) {
			t.Errorf("%#04X %q is cased", r, r)
		}
	}
	for _, r := range []rune{'1', ' ', '.', '中', '́'} {
		if cased(r) {
			t.Errorf("%#04X %q is not cased", r, r)
		}
	}
	for _, r := range []rune{'\'', '.', ':', '́', '­', 'ʰ', '^'} {
		if !caseIgnorable(r) {
			t.Errorf("%#04X %q is case-ignorable", r, r)
		}
	}
	for _, r := range []rune{'a', 'A', '1', ' ', '中', ','} {
		if caseIgnorable(r) {
			t.Errorf("%#04X %q is not case-ignorable", r, r)
		}
	}
}

// TestTheBackwardScanDecodesWholeCharacters, because every one of the three
// backward conditions walks a string by bytes and a mistake there reads half of
// a character as a whole one.
func TestTheBackwardScanDecodesWholeCharacters(t *testing.T) {
	for _, s := range []string{"a", "ü", "中", "𝔘", "İ", "ΟΔΟ"} {
		want := []rune(s)[len([]rune(s))-1]
		got, size := lastRuneIn(s)
		if got != want || size != len(string(want)) {
			t.Errorf("%q: last rune %#04X of %d bytes, want %#04X of %d",
				s, got, size, want, len(string(want)))
		}
	}
	if r, _ := lastRuneIn(""); r != 0 {
		t.Errorf("the empty string gave %#04X", r)
	}
	// And it agrees with the standard library over a whole alphabet, which is
	// the check that matters: this exists to read as a scan, not to be a second
	// implementation with a second set of bugs.
	for _, s := range []string{"abc", "αβγ", "日本語", "áb̧"} {
		for i := 1; i <= len(s); i++ {
			if !utf8ValidPrefix(s, i) {
				continue
			}
			want, wantSize := lastRuneOf(s[:i])
			got, size := lastRuneIn(s[:i])
			if got != want || size != wantSize {
				t.Errorf("%q[:%d]: %#04X/%d, want %#04X/%d", s, i, got, size, want, wantSize)
			}
		}
	}
}

func utf8ValidPrefix(s string, i int) bool {
	return i == len(s) || s[i]&0xC0 != 0x80
}

func lastRuneOf(s string) (rune, int) {
	if s == "" {
		return 0, 1
	}
	r := []rune(s)
	last := r[len(r)-1]
	return last, len(string(last))
}

var _ = strings.TrimSpace
