package shape

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// CSS Fonts 4 §7.2's order of precedence, as a plan applies it: the features
// on by default, then the font-variant properties and font-kerning, then the
// properties that turn features off for reasons of their own, then
// font-feature-settings above all of them. See Features.requested.
//
// A Features value is one CSS request and HarfBuzz takes a flat list of
// features, so each case below names the list the request comes to — the one a
// browser hands HarfBuzz after resolving the precedence — and the answer is
// HarfBuzz 14.5.0's for that list, shaped through uharfbuzz 0.56.2 over the
// fixtures TestWritePrecedenceFixtures writes out. font-kerning: none with
// "kern" 1 comes to "kern"; "liga" 0 comes to "-liga".

// precedenceFixtures are the fonts, by name.
//
// "features" maps the gposorder glyphs and states three rules: 'liga' makes
// a+b the ligature glyph, 'smcp' turns a into x (a feature only a font-variant
// property asks for), and 'kern' closes b-a by 40. "legacy" kerns A-V by 150
// in a legacy kern table and nowhere else. "hangul" turns 가 into 각 under
// 'calt', which the Hangul model applies to everything but the jamo.
func precedenceFixtures() map[string][]byte {
	return map[string][]byte{
		"features": fonttest.SFNT(fonttest.SFNTOptions{
			Name:   "Precedence",
			Glyphs: gposOrderGlyphs(false),
			Extra: map[string][]byte{
				"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
					{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
						{Components: []int{goA, goB}, Glyph: goAB}})}},
					{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{goA}, []int{goX})}},
				}, map[string][]int{"liga": {0}, "smcp": {1}}),
				"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
					{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable(
						[]fonttest.KernPair{{Left: goB, Right: goA, Adjust: -40}})}},
				}, map[string][]int{"kern": {0}}),
			},
		}),
		"hangul": fonttest.SFNT(fonttest.SFNTOptions{
			Name: "PrecedenceHangul",
			Glyphs: []fonttest.Glyph{
				{Rune: 0xAC00, Advance: 1000, HasShape: true},
				{Rune: 0xAC01, Advance: 900, HasShape: true},
			},
			Extra: map[string][]byte{
				"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
					{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{2})}},
				}, map[string][]int{"calt": {0}}),
			},
		}),
		"legacy": fonttest.SFNT(fonttest.SFNTOptions{
			Name: "PrecedenceLegacy",
			Glyphs: []fonttest.Glyph{
				{Rune: 'A', Advance: lkAdvance, HasShape: true},
				{Rune: 'V', Advance: lkAdvance, HasShape: true},
			},
			Extra: map[string][]byte{"kern": fonttest.LegacyKern([]fonttest.KernSubtable{{
				Coverage: fonttest.KernHorizontal,
				Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
			}})},
		}),
	}
}

// TestFeatureSettingsHaveTheLastWord is each step of the order, both ways.
func TestFeatureSettingsHaveTheLastWord(t *testing.T) {
	fonts := map[string]*Face{}
	for name, data := range precedenceFixtures() {
		f, err := Load(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		fonts[name] = f
	}
	ligature := []shapedAs{{goAB, 1100, 0, 0}}
	apart := []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}}
	kerned := []shapedAs{{goB, 560, 0, 0}, {goA, 500, 0, 0}}
	unkerned := []shapedAs{{goB, 600, 0, 0}, {goA, 500, 0, 0}}
	small := []shapedAs{{goX, 700, 0, 0}}
	plainA := []shapedAs{{goA, 500, 0, 0}}
	half := float64(lkTighten / 2)
	legacyKerned := []shapedAs{{lkA, lkAdvance + half, 0, 0}, {lkV, lkAdvance + half, half, 0}}
	legacyPlain := []shapedAs{{lkA, lkAdvance, 0, 0}, {lkV, lkAdvance, 0, 0}}
	alternate := []shapedAs{{2, 900, 0, 0}}
	syllable := []shapedAs{{1, 1000, 0, 0}}

	for _, c := range []struct {
		what, font, text string
		features         Features
		harfbuzz         string // the list the request comes to, as HarfBuzz is asked
		want             []shapedAs
	}{
		// Step 1 alone: the defaults.
		{"nothing asked", "features", "ab", Features{}, "", ligature},
		{"nothing asked", "features", "ba", Features{}, "", kerned},

		// Step 3 against step 1: font-variant-ligatures and font-kerning.
		{"font-variant-ligatures: none", "features", "ab",
			Features{NoOptionalLigatures: true, NoContextualAlternates: true}, "-liga,-clig,-dlig,-hlig,-calt", apart},
		{"font-kerning: none", "features", "ba", Features{NoKerning: true}, "-kern", unkerned},
		{"font-kerning: none, legacy table", "legacy", "AV", Features{NoKerning: true}, "-kern", legacyPlain},

		// Step 5 against step 1: font-feature-settings turning a default off.
		{`"liga" 0`, "features", "ab", Features{TagsOff: "liga"}, "-liga", apart},
		{`"kern" 0`, "features", "ba", Features{TagsOff: "kern"}, "-kern", unkerned},
		{`"kern" 0, legacy table`, "legacy", "AV", Features{TagsOff: "kern"}, "-kern", legacyPlain},

		// Step 5 against step 3, both ways. font-feature-settings wins.
		{`font-kerning: none; "kern" 1`, "features", "ba",
			Features{NoKerning: true, Tags: "kern"}, "kern", kerned},
		{`font-kerning: none; "kern" 1, legacy table`, "legacy", "AV",
			Features{NoKerning: true, Tags: "kern"}, "kern", legacyKerned},
		{`font-kerning: normal; "kern" 0`, "features", "ba",
			Features{TagsOff: "kern"}, "-kern", unkerned},
		{`font-variant-ligatures: none; "liga" 1`, "features", "ab",
			Features{NoOptionalLigatures: true, NoContextualAlternates: true, Tags: "liga"},
			"-clig,-dlig,-hlig,-calt,liga", ligature},
		{`font-variant-caps: small-caps`, "features", "a",
			Features{Caps: CapsSmall}, "smcp", small},
		{`font-variant-caps: small-caps; "smcp" 0`, "features", "a",
			Features{Caps: CapsSmall, TagsOff: "smcp"}, "-smcp", plainA},

		// The Hangul model's own 'calt', which it asks for after everything
		// else: a "calt" 0 keeps it off, as HarfBuzz keeps it off.
		{"Hangul, nothing asked", "hangul", "\uAC00", Features{}, "", alternate},
		{`Hangul, "calt" 0`, "hangul", "\uAC00", Features{TagsOff: "calt"}, "-calt", syllable},
		{`Hangul, font-variant-ligatures: no-contextual`, "hangul", "\uAC00",
			Features{NoContextualAlternates: true}, "-calt", syllable},
		{`Hangul, font-variant-ligatures: no-contextual; "calt" 1`, "hangul", "\uAC00",
			Features{NoContextualAlternates: true, Tags: "calt"}, "calt", alternate},

		// Step 5 against step 4: letter-spacing turns the optional ligatures
		// off, and font-feature-settings turns them back on.
		{`letter-spacing; "liga" 1`, "features", "ab",
			Features{NoOptionalLigatures: true, Tags: "liga"}, "-clig,-dlig,-hlig,liga", ligature},
	} {
		got, missing := fonts[c.font].ShapeGlyphsInContext(c.text, "", "", c.features)
		if missing != 0 {
			t.Fatalf("%s: %d characters have no glyph", c.what, missing)
		}
		checkShaped(t, c.what+" ("+c.harfbuzz+") over "+c.text, got, c.want)
	}
}

// TestThePairAcrossARunsEdgeFollowsTheOrder: the pair kerned across the edge of
// a run is 'kern' too, and is off or on as the rest of the run's kerning is.
// "b" before "a" is the pair the fixture closes by 40, with the "a" in the next
// run. The answers are the ones the same two letters get shaped as one run,
// which the case list above holds against HarfBuzz.
func TestThePairAcrossARunsEdgeFollowsTheOrder(t *testing.T) {
	f, err := Load(precedenceFixtures()["features"])
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what     string
		features Features
		want     []shapedAs
	}{
		{"nothing asked", Features{}, []shapedAs{{goB, 560, 0, 0}}},
		{"font-kerning: none", Features{NoKerning: true}, []shapedAs{{goB, 600, 0, 0}}},
		{`"kern" 0`, Features{TagsOff: "kern"}, []shapedAs{{goB, 600, 0, 0}}},
		{`font-kerning: none; "kern" 1`, Features{NoKerning: true, Tags: "kern"},
			[]shapedAs{{goB, 560, 0, 0}}},
	} {
		got, _ := f.ShapeGlyphsInContext("b", "", "a", c.features)
		checkShaped(t, c.what+": b before a in the next run", got, c.want)
	}
}

// TestWhetherANeighbourCanKernFollowsTheOrder: ContextCanChange says a run's
// neighbours can move it where the pair across the edge would be kerned, and
// that is whether kerning ends up on. A face whose only rule is a legacy kern
// pair has nothing else a neighbour could change.
func TestWhetherANeighbourCanKernFollowsTheOrder(t *testing.T) {
	f, err := Load(precedenceFixtures()["legacy"])
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		what     string
		features Features
		can      bool
	}{
		{"nothing asked", Features{}, true},
		{"font-kerning: none", Features{NoKerning: true}, false},
		{`"kern" 0`, Features{TagsOff: "kern"}, false},
		{`font-kerning: none; "kern" 1`, Features{NoKerning: true, Tags: "kern"}, true},
	} {
		if got := f.ContextCanChange("A", c.features); got != c.can {
			t.Errorf("%s: ContextCanChange(\"A\") = %v, want %v", c.what, got, c.can)
		}
	}
}

// TestTurnsOffFollowsTheOrder is the answer the positioning pass and the
// boundary pair read, asked directly: whether kerning ends up off.
func TestTurnsOffFollowsTheOrder(t *testing.T) {
	for _, c := range []struct {
		f   Features
		tag string
		off bool
	}{
		{Features{}, "kern", false},
		{Features{NoKerning: true}, "kern", true},
		{Features{NoKerning: true, Tags: "kern"}, "kern", false},
		{Features{TagsOff: "kern"}, "kern", true},
		{Features{NoOptionalLigatures: true}, "liga", true},
		{Features{NoOptionalLigatures: true, Tags: "liga,onum"}, "liga", false},
		{Features{NoContextualAlternates: true, Tags: "calt"}, "calt", false},
		{Features{TagsOff: "calt,liga"}, "calt", true},
		// A tag is matched whole: "kern" is not in "kerx".
		{Features{TagsOff: "kerx"}, "kern", false},
	} {
		if got := c.f.turnsOff(c.tag); got != c.off {
			t.Errorf("%+v: turnsOff(%q) = %v, want %v", c.f, c.tag, got, c.off)
		}
	}
}

// TestWritePrecedenceFixtures writes the fixtures out for asking HarfBuzz,
// where FORME_PRECEDENCE_FIXTURES names a directory; it does nothing
// otherwise.
func TestWritePrecedenceFixtures(t *testing.T) {
	dir := os.Getenv("FORME_PRECEDENCE_FIXTURES")
	if dir == "" {
		t.Skip("FORME_PRECEDENCE_FIXTURES is not set")
	}
	for name, data := range precedenceFixtures() {
		if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
