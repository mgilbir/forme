package font

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The predefined charsets, TN #5176 Appendix C.
//
// A charset is what gives a non-CID CFF glyph its name, and the reader treated
// Expert and Expert Subset as the identity. That does not leave a glyph unknown;
// it leaves it known wrongly, which is the worse of the two.

// namesOf reads a synthetic CFF and returns the glyph names it declares.
func namesOf(t *testing.T, opts fonttest.CFFOptions) map[string]bool {
	t.Helper()
	fp := ParseCFF(fonttest.CFF(opts))
	if fp == nil {
		t.Fatalf("ParseCFF returned nil for %+v", opts)
	}
	return fp.GlyphNames
}

// TestTheExpertCharsetIsNotTheIdentity is the defect. GID 2 of an Expert font is
// "exclamsmall"; the identity calls it "quotedbl", which is a different glyph in
// a different font — so the width recorded under "quotedbl" belonged to a
// small-cap exclamation mark, and a consumer comparing a PDF's declared widths
// against the font's own compared two unrelated glyphs and believed the answer.
func TestTheExpertCharsetIsNotTheIdentity(t *testing.T) {
	for _, tc := range []struct {
		charset  int
		what     string
		want     string // the name GID 2 really has
		identity string // the name the identity would have given it
	}{
		{1, "Expert", "exclamsmall", "quotedbl"},
		{2, "Expert Subset", "dollaroldstyle", "quotedbl"},
	} {
		names := namesOf(t, fonttest.CFFOptions{Glyphs: 3, Charset: tc.charset})
		if !names[tc.want] {
			t.Errorf("%s charset: GID 2 is not named %q; got %v", tc.what, tc.want, names)
		}
		if names[tc.identity] {
			t.Errorf("%s charset: GID 2 named %q, which is what the identity calls "+
				"it and not what the charset does", tc.what, tc.identity)
		}
	}
}

// TestISOAdobeStaysTheIdentity: charset 0 is the identity and must not have moved.
func TestISOAdobeStaysTheIdentity(t *testing.T) {
	// GID 1 is SID 1 "space", GID 2 is SID 2 "exclam", GID 3 is SID 3 "quotedbl".
	names := namesOf(t, fonttest.CFFOptions{Glyphs: 4})
	for _, want := range []string{"space", "exclam", "quotedbl"} {
		if !names[want] {
			t.Errorf("ISOAdobe: %q missing; got %v", want, names)
		}
	}
}

// TestAPredefinedCharsetDoesNotNameGlyphsItDoesNotHave.
//
// A predefined charset is a fixed list. A font declaring one and carrying more
// glyphs than it holds has named none of the extras, and continuing the identity
// past the end hands them the names of unrelated standard strings — which is how
// a width gets recorded against a glyph the font does not have, the exact shape
// of the fault this whole change is about.
//
// Expert Subset holds 87 glyphs, so a 90-glyph font has three past the end.
func TestAPredefinedCharsetDoesNotNameGlyphsItDoesNotHave(t *testing.T) {
	fp := ParseCFF(fonttest.CFF(fonttest.CFFOptions{Glyphs: 90, Charset: 2}))
	if fp == nil {
		t.Fatal("ParseCFF returned nil")
	}
	if got, want := len(fp.GlyphNames), 87; got != want {
		t.Errorf("a 90-glyph font on the 87-glyph Expert Subset charset declares %d "+
			"names, want %d", got, want)
	}
	if fp.GlyphNames[""] {
		t.Errorf(`a glyph the charset does not name was recorded as named ""`)
	}
	// Every glyph still has a width by index: leaving a glyph unnamed must not
	// lose it, only stop it being addressed by a name it does not have.
	if got, want := len(fp.WidthByGID), 90; got != want {
		t.Errorf("WidthByGID has %d entries, want %d", got, want)
	}
}

// TestACIDFontKeepsIdentityCIDs. A CID-keyed font's charset holds CIDs, not glyph
// names, so the three predefined *name* tables mean nothing there. Identity is
// what "Identity" ordering asks for and what every consumer already reads, and
// applying the Expert names to it would renumber a font's CIDs.
func TestACIDFontKeepsIdentityCIDs(t *testing.T) {
	for _, charset := range []int{0, 1, 2} {
		fp := ParseCFF(fonttest.CFF(fonttest.CFFOptions{
			Glyphs: 5, CIDKeyed: true, Charset: charset,
		}))
		if fp == nil {
			t.Fatalf("charset %d: ParseCFF returned nil", charset)
		}
		for g := 0; g < 5; g++ {
			if fp.GIDToCID[g] != g {
				t.Errorf("charset %d: GID %d is CID %d, want the identity",
					charset, g, fp.GIDToCID[g])
			}
		}
	}
}

// TestThePredefinedCharsetsResolve is the check on the tables themselves.
//
// They are written as glyph names and turned into SIDs through this module's own
// cffStandardStrings, so a name that is not a standard string would resolve
// silently to SID 0 and give a glyph the name ".notdef". Nothing downstream
// would look wrong; the font would simply have one glyph fewer than it does.
func TestThePredefinedCharsetsResolve(t *testing.T) {
	std := make(map[string]bool, len(cffStandardStrings))
	for _, s := range cffStandardStrings {
		std[s] = true
	}
	for what, names := range map[string][]string{
		"Expert":        cffExpertCharsetNames,
		"Expert Subset": cffExpertSubsetCharsetNames,
	} {
		for i, n := range names {
			if !std[n] {
				t.Errorf("%s charset entry %d is %q, which is not a standard string; "+
					"it would resolve to SID 0 and name the glyph .notdef", what, i, n)
			}
		}
	}
}

// TestTheCharsetsAreTheSizesTheSpecificationGives, and start where it says.
//
// A table one entry short would shift every glyph after the gap by one, which is
// a wrong name for every glyph rather than a missing one — and would show up
// nowhere except as widths that do not match.
func TestTheCharsetsAreTheSizesTheSpecificationGives(t *testing.T) {
	for _, tc := range []struct {
		what  string
		names []string
		want  int
	}{
		{"Expert", cffExpertCharsetNames, 166},
		{"Expert Subset", cffExpertSubsetCharsetNames, 87},
	} {
		if got := len(tc.names); got != tc.want {
			t.Errorf("%s charset has %d entries, want %d (TN #5176 Appendix C)",
				tc.what, got, tc.want)
		}
		if tc.names[0] != ".notdef" || tc.names[1] != "space" {
			t.Errorf("%s charset starts %q, %q; every charset starts .notdef, space",
				tc.what, tc.names[0], tc.names[1])
		}
	}
	if got, want := isoAdobeCharsetLen, 229; got != want {
		t.Errorf("isoAdobeCharsetLen is %d, want %d", got, want)
	}
}

// TestTheExpertSubsetIsASubsetOfTheExpertCharset is the one relation between the
// two tables that neither one states about itself, which is what makes it worth
// asserting: a transcription error in either would have to be matched exactly by
// one in the other to survive it.
func TestTheExpertSubsetIsASubsetOfTheExpertCharset(t *testing.T) {
	full := make(map[string]bool, len(cffExpertCharsetNames))
	for _, n := range cffExpertCharsetNames {
		full[n] = true
	}
	for i, n := range cffExpertSubsetCharsetNames {
		if !full[n] {
			t.Errorf("Expert Subset entry %d is %q, which the Expert charset does "+
				"not hold; one of the two tables is wrong", i, n)
		}
	}
}

// TestAPredefinedCharsetUsesOnlyPredefinedStrings. That is what makes it
// predefined: a font reading one carries no string INDEX entry for any of it, so
// an SID past the standard 391 would resolve against whatever strings that
// particular font happened to carry.
func TestAPredefinedCharsetUsesOnlyPredefinedStrings(t *testing.T) {
	for what, id := range map[string]int{"Expert": 1, "Expert Subset": 2} {
		set, ok := cffPredefinedCharset(id)
		if !ok {
			t.Fatalf("%s: charset id %d is not predefined", what, id)
		}
		for gid, sid := range set {
			if sid < 0 || sid >= len(cffStandardStrings) {
				t.Errorf("%s: GID %d has SID %d, outside the %d standard strings",
					what, gid, sid, len(cffStandardStrings))
			}
		}
	}
}

// TestOnlyThreeCharsetIdsArePredefined: anything else is an offset into the font,
// and reading it as a predefined id would send the parser to a table that is not
// there.
func TestOnlyThreeCharsetIdsArePredefined(t *testing.T) {
	for _, id := range []int{-1, 3, 4, 391, 1 << 20} {
		if _, ok := cffPredefinedCharset(id); ok {
			t.Errorf("charset id %d reported as predefined", id)
		}
	}
	if set, ok := cffPredefinedCharset(0); !ok || set != nil {
		t.Errorf("ISOAdobe: got (%v, %v), want (nil, true) — it is the identity",
			set, ok)
	}
}
