package paragraph

import "testing"

// The keyword readers here take a computed value's text and compare it with
// the property's keywords, which CSS compares ASCII case-insensitively. They
// lowercased with strings.ToLower, which is Unicode's and maps U+212A KELVIN
// SIGN to "k", so "\u212Aeep-all" kept words together and
// "full-size-\u212Aana" enlarged small kana. The language tag readers are the
// same question of a BCP 47 tag, which is ASCII case-insensitive too.
func TestKeywordsAreFoldedAsASCII(t *testing.T) {
	if got := WordBreakOf("KEEP-ALL"); !got.KeepAll {
		t.Errorf("WordBreakOf(\"KEEP-ALL\") = %+v, want keep-all", got)
	}
	if got := WordBreakOf("\u212Aeep-all"); got.KeepAll {
		t.Errorf("WordBreakOf with a KELVIN SIGN read keep-all: %+v", got)
	}
	if got := TransformOf("FULL-SIZE-KANA"); got != TransformFullSizeKana {
		t.Errorf("TransformOf(\"FULL-SIZE-KANA\") = %v, want full-size-kana", got)
	}
	if got := TransformOf("full-size-\u212Aana"); got != TransformNone {
		t.Errorf("TransformOf with a KELVIN SIGN = %v, want none", got)
	}
	// A language tag: "\u212Ak" is not Kako, and "tr" with a dotted capital I
	// is not Turkish.
	if got, want := LanguageOf("TR"), LanguageOf("tr"); got != want || got == "" {
		t.Errorf("LanguageOf(\"TR\") = %q, want %q", got, want)
	}
	if got := LanguageOf("\u212Ak"); got == LanguageOf("kk") {
		t.Errorf("LanguageOf with a KELVIN SIGN read %q, the language of \"kk\"", got)
	}
}
