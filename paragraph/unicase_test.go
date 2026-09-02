package paragraph

import "testing"

// Georgian is unicase, and "text-transform: uppercase" must leave it alone.
//
// Unicode 11 added the Mtavruli capitals at U+1C90 and gave every Mkhedruli
// letter a simple uppercase mapping to one, so unicode.ToUpper turns ა into Ა.
// That is not what the property is for: Mtavruli is a display style set
// deliberately for a heading or a sign, and running Georgian text is never
// written in it. The suite says so in one line — text-transform-unicase-001
// writes the letter twice, uppercases one, and asks for the two to look exactly
// alike.

func upperOf(text string) string {
	out, _ := TransformText(text, TransformUppercase, false, "")
	return out
}

func lowerOf(text string) string {
	out, _ := TransformText(text, TransformLowercase, false, "")
	return out
}

func TestUppercaseLeavesMkhedruliAlone(t *testing.T) {
	for _, tc := range []struct{ text, what string }{
		{"ა", "the suite's own single letter"},
		{"ქართული", "a word"},
		{"აbcა", "Georgian either side of Latin, which must still uppercase"},
		{"ჺ", "the last letter of the first run"},
		{"ჽ", "the first letter of the second run"},
		{"ჿ", "the last letter of the second run"},
	} {
		if got := upperOf(tc.text); got != wantUpper(tc.text) {
			t.Errorf("%s: %q uppercased to %q, want %q", tc.what, tc.text, got,
				wantUpper(tc.text))
		}
	}
}

// wantUpper is the text with every character uppercased except the Mkhedruli.
func wantUpper(text string) string {
	out := []rune{}
	for _, r := range text {
		switch {
		case isMkhedruli(r):
			out = append(out, r)
		case r >= 'a' && r <= 'z':
			out = append(out, r-'a'+'A')
		default:
			out = append(out, r)
		}
	}
	return string(out)
}

func TestTheCarveOutIsUppercaseOnly(t *testing.T) {
	// Lowercasing a Mtavruli capital is exactly the mapping Unicode added the
	// characters for, and it still happens.
	if got := lowerOf("Ა"); got != "ა" {
		t.Errorf("Ა lowercased to %q, want %q — the mapping this way round is "+
			"what Mtavruli is for", got, "ა")
	}
	// And Asomtavruli, the historic capitals, keep their traditional mapping.
	if got := lowerOf("Ⴀ"); got != "ⴀ" {
		t.Errorf("Ⴀ lowercased to %q, want %q — Asomtavruli's pair is Nuskhuri "+
			"and is untouched by this", got, "ⴀ")
	}
}

func TestTheRunsAreUnicodesOwn(t *testing.T) {
	// U+10FB and U+10FC fall between the two runs: a paragraph separator and a
	// modifier letter, neither of which has a Mtavruli form. They are not
	// letters this is about, and the gap in the test is deliberate.
	for _, r := range []rune{0x10CF, 0x10FB, 0x10FC, 0x1C90} {
		if isMkhedruli(r) {
			t.Errorf("U+%04X is not a Mkhedruli letter and is reported as one", r)
		}
	}
	for _, r := range []rune{0x10D0, 0x10E5, 0x10FA, 0x10FD, 0x10FF} {
		if !isMkhedruli(r) {
			t.Errorf("U+%04X is a Mkhedruli letter and is not reported as one", r)
		}
	}
}

func TestTheRestOfTheTextStillTransforms(t *testing.T) {
	// The carve-out must not turn the whole-string fast path off for text that
	// has no Georgian in it, nor swallow the rest of a string that has.
	if got := upperOf("straße"); got != "STRASSE" {
		t.Errorf("%q uppercased to %q, want %q — the full mappings still apply",
			"straße", got, "STRASSE")
	}
	if got := upperOf("aა" + "ß"); got != "Aა"+"SS" {
		t.Errorf("%q uppercased to %q, want %q", "aაß", got, "Aა"+"SS")
	}
}
