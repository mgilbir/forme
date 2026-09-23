package paragraph

import "testing"

// The character questions this package asks are answered from the release its
// tables are generated from, and not from the toolchain's.
//
// Every character below was assigned by Unicode 16, which is after the 15.0.0
// that package unicode answers from under Go 1.26 — the release go.mod names —
// and every question below was package unicode's until internal/charprop
// answered it. Under that toolchain each of them came out as though the
// character did not exist: a Garay letter was not a letter, a Garay digit not a
// digit, and U+0897 ARABIC PEPET not a mark. With a toolchain whose release is
// the pinned one the old answers and these agree, which is why the check that
// nothing asks package unicode at all is cmd/pinnedunicode_test.go and not
// this.
func TestTheCharacterQuestionsAreThePinnedReleases(t *testing.T) {
	const (
		garayCapitalA = "\U00010D50"
		garaySmallA   = "\U00010D70"
		garaySmallCa  = "\U00010D71"
		garayDigit0   = 0x10D40
		pepet         = "ࢗ"
	)
	for _, tc := range []struct {
		in   string
		kind TextTransform
		want string
		what string
	}{
		// Final_Sigma: a sigma after a cased letter and before none is final.
		// A Garay capital is cased.
		{garayCapitalA + "Σ", TransformLowercase, garaySmallA + "ς",
			"a sigma after a Unicode 16 capital ends the word"},
		// And a mark between the sigma and the next letter is case-ignorable,
		// so the sigma is not the end of the word.
		{"ΑΣ" + pepet + "Α", TransformLowercase, "ασ" + pepet + "α",
			"a Unicode 16 mark does not end the word it is in"},
		// Case_Ignorable was typed out here, and seven of the characters it
		// named are not in it: the Armenian hyphen is one, so the sigma before
		// it ends the word.
		{"ΑΣ\u058AΑ", TransformLowercase, "ας\u058Aα",
			"a character that is not Case_Ignorable ends the word"},
		// A word begins at a letter, and a Garay letter is one.
		{garaySmallA + garaySmallCa + " x", TransformCapitalize, garayCapitalA + garaySmallCa + " X",
			"a word in a Unicode 16 script is capitalised"},
	} {
		if got, _ := TransformText(tc.in, tc.kind, WordClosed, ""); got != tc.want {
			t.Errorf("%s: %+q became %+q, want %+q", tc.what, tc.in, got, tc.want)
		}
	}
	if !IsAutospaceNumeral(garayDigit0) {
		t.Error("a Garay digit is not a numeral to text-autospace")
	}
	if !IsAutospaceLetter(0x10D50) {
		t.Error("a Garay letter is not a letter to text-autospace")
	}
	if AutospaceBase(0x0897) {
		t.Error("U+0897 ARABIC PEPET is a base to text-autospace, and it is a mark")
	}
	if !IsLetterUnit(0x10D50) || !IsLetterUnit(garayDigit0) {
		t.Error("a Garay letter or digit is not a typographic letter unit")
	}
	if got := DescribeRune(0x10D50); got != "U+10D50 ("+garayCapitalA+")" {
		t.Errorf("DescribeRune(U+10D50) = %q, which does not show the character", got)
	}
}
