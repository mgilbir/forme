package shape

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The positioning lookups applied as a font lists them, and the marks of a
// face with no positioning placed without it. See position.go and
// fallback.go.
//
// Each fixture's answer is HarfBuzz 14.5.0's for the same font, shaped through
// uharfbuzz: the fonts are built here, and TestWriteGPOSOrderFixtures writes them out
// for asking it (FORME_GPOS_FIXTURES=<dir>). Every case is one the Google
// Fonts sweep found in a real face first; the face is named at each.

// The glyphs every fixture here maps: a letter, a second letter, two marks, a
// glyph that 'ccmp' takes apart and its two parts, a Hebrew letter and point,
// and the ligature of the two letters. Glyph 0 is .notdef, so these are 1 to
// 10.
const (
	goA = 1 + iota
	goB
	goAcute
	goCedilla
	goX
	goPart1
	goPart2
	goBet
	goQamats
	goAB
)

func gposOrderGlyphs(ink bool) []fonttest.Glyph {
	g := []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 600, HasShape: true},
		{Rune: 0x0301, Advance: 0, HasShape: true},
		{Rune: 0x0327, Advance: 0, HasShape: true},
		{Rune: 'x', Advance: 700, HasShape: true},
		{Rune: 0xE000, Advance: 350, HasShape: true},
		{Rune: 0xE001, Advance: 350, HasShape: true},
		{Rune: 0x05D1, Advance: 640, HasShape: true},
		{Rune: 0x05B8, Advance: 0, HasShape: true},
		{Rune: 0xE002, Advance: 1100, HasShape: true},
	}
	if ink {
		// Boxes of their own, for the fixtures that place marks by their ink.
		g[0].Ink = [4]int{50, 0, 450, 520}
		g[1].Ink = [4]int{80, -10, 560, 700}
		g[2].Ink = [4]int{30, 560, 190, 720}
		g[3].Ink = [4]int{20, -210, 170, 0}
		g[7].Ink = [4]int{40, 0, 600, 540}
		g[8].Ink = [4]int{10, -180, 210, -40}
		g[9].Ink = [4]int{50, 0, 1050, 520}
	}
	return g
}

// markOnA is the anchors of the acute on 'a', and of the cedilla under it.
func markOnA() []byte {
	return fonttest.MarkAttachSubtable(
		[]fonttest.MarkAttachment{
			{Glyph: goAcute, Class: 0, Anchor: fonttest.Anchor{X: 100, Y: 600}},
			{Glyph: goCedilla, Class: 1, Anchor: fonttest.Anchor{X: 90, Y: 0}},
		},
		[]fonttest.BaseAttachment{
			{Glyph: goA, Anchors: map[int]fonttest.Anchor{0: {X: 250, Y: 650}, 1: {X: 240, Y: -20}}},
			{Glyph: goPart1, Anchors: map[int]fonttest.Anchor{0: {X: 175, Y: 640}, 1: {X: 175, Y: -30}}},
		})
}

func gposOrderGDEF() []byte {
	return fonttest.GDEF(map[int]int{goA: classBase, goB: classBase, goAcute: classMark,
		goCedilla: classMark, goX: classBase, goPart1: classBase, goPart2: classBase})
}

// gposOrderFixtures are the fonts, by name.
func gposOrderFixtures() map[string][]byte {
	font := func(ink bool, extra map[string][]byte) []byte {
		return fonttest.SFNT(fonttest.SFNTOptions{Name: "GPOSOrder", Glyphs: gposOrderGlyphs(ink), Extra: extra})
	}
	single := func(glyph, x, y, adv int) fonttest.Lookup {
		return fonttest.Lookup{Type: 1, Subtables: [][]byte{fonttest.SinglePosSubtable(glyph, x, y, adv)}}
	}
	out := map[string][]byte{}

	// A single adjustment after the mark it adjusts is attached, and two
	// single adjustments of one glyph: Noto Sans Gujarati UI lowers a vowel
	// sign by twenty units in 'dist' after 'blwm' attaches it.
	out["after-attach"] = font(false, map[string][]byte{
		"GDEF": gposOrderGDEF(),
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 4, Subtables: [][]byte{markOnA()}},
			single(goAcute, 0, -20, 0),
			single(goA, 0, 0, 10),
			single(goA, 0, 0, 20),
		}, map[string][]int{"mark": {0}, "dist": {1}, "kern": {2, 3}}),
	})

	// A contextual rule that moves a mark after a mark lookup placed it:
	// Arimo corrects some Hebrew points that way, and Mukta its reph.
	out["context-after-attach"] = font(false, map[string][]byte{
		"GDEF": gposOrderGDEF(),
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 4, Subtables: [][]byte{markOnA()}},
			{Type: 8, Subtables: [][]byte{fonttest.ChainedContext3(nil, [][]int{{goA}, {goAcute}}, nil,
				[]fonttest.SeqLookup{{At: 1, Lookup: 2}})}},
			single(goAcute, 15, 0, 0),
		}, map[string][]int{"mark": {0, 1}}),
	})

	// A mark after a glyph 'ccmp' took apart goes on its first part:
	// AlexBrush's 'ccmp' takes U+01C9 apart, and its cedilla goes on the l.
	out["multiplied"] = font(false, map[string][]byte{
		"GDEF": gposOrderGDEF(),
		"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
			{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{goX}, [][]int{{goPart1, goPart2}})}},
		}, map[string][]int{"ccmp": {0}}),
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 4, Subtables: [][]byte{markOnA()}},
		}, map[string][]int{"mark": {0}}),
	})

	// No GPOS: the marks are placed by their ink and combining classes. The
	// M+ families, and a thousand faces of Latin, Greek and Cyrillic.
	out["fallback"] = font(true, nil)
	// A GDEF that calls the marks bases changes nothing about it: what places
	// them is what the characters say.
	out["fallback-gdef"] = font(true, map[string][]byte{
		"GDEF": fonttest.GDEF(map[int]int{goA: classBase, goAcute: classBase, goCedilla: classBase}),
	})
	// Positioning stated for no script but the default, which the Hebrew model
	// does not apply: the points are placed as for a face with none, and the
	// letters are not kerned. The M+ families state theirs so.
	out["hebrew-dflt"] = font(true, map[string][]byte{
		"GDEF": fonttest.GDEF(map[int]int{goBet: classBase, goQamats: classMark}),
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable(
				[]fonttest.KernPair{{Left: goBet, Right: goBet, Adjust: -100}})}},
			{Type: 4, Subtables: [][]byte{fonttest.MarkAttachSubtable(
				[]fonttest.MarkAttachment{{Glyph: goQamats, Class: 0, Anchor: fonttest.Anchor{X: 100, Y: 0}}},
				[]fonttest.BaseAttachment{{Glyph: goBet, Anchors: map[int]fonttest.Anchor{0: {X: 300, Y: 0}}}})}},
		}, map[string][]int{"kern": {0}, "mark": {1}}),
	})
	// A ligature's marks without the font: each is placed over the part of
	// the ligature it was written after, a share of its advance.
	out["fallback-ligature"] = font(true, map[string][]byte{
		"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{Type: 4, Flag: flagIgnoreMarks,
			Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{goA, goB}, Glyph: goAB}})}}},
			map[string][]int{"liga": {0}}),
	})
	// A mark on a letter that hangs from a cursive chain: its height is taken
	// from the whole chain above its letter, since the letters' own heights
	// are added up along the chain only at the end. Noto Serif Tibetan's
	// subjoined letters, and every Arabic font whose joins climb.
	out["cursive-mark"] = font(false, map[string][]byte{
		"GDEF": gposOrderGDEF(),
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 3, Subtables: [][]byte{fonttest.CursivePosSubtable([]fonttest.CursiveAnchor{
				{Glyph: goA, Exit: fonttest.Anchor{X: 480, Y: 100}, HasExit: true},
				{Glyph: goB, Entry: fonttest.Anchor{X: 20, Y: 0}, Exit: fonttest.Anchor{X: 580, Y: 100},
					HasEntry: true, HasExit: true},
			})}},
			{Type: 4, Subtables: [][]byte{fonttest.MarkAttachSubtable(
				[]fonttest.MarkAttachment{{Glyph: goAcute, Class: 0, Anchor: fonttest.Anchor{X: 100, Y: 600}}},
				[]fonttest.BaseAttachment{{Glyph: goB, Anchors: map[int]fonttest.Anchor{0: {X: 300, Y: 700}}}})}},
		}, map[string][]int{"curs": {0}, "mark": {1}}),
	})
	// A ligature of a mark and a letter, in a face with no positioning: it is
	// drawn as a letter and is not placed as a mark.
	out["fallback-mark-ligature"] = font(true, map[string][]byte{
		"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{Type: 4,
			Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{goAcute, goB}, Glyph: goAB}})}}},
			map[string][]int{"liga": {0}}),
	})
	// A rule that takes its first glyph apart and then ligates the second part
	// with the glyph after it, which the decomposition pushed one place
	// further on. Padauk's 'rlig' does this with U+AA69 and U+1084.
	out["nested-growth"] = font(false, map[string][]byte{
		"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
			{Type: 5, Subtables: [][]byte{fonttest.SequenceContext3([][]int{{goA}, {goB}},
				[]fonttest.SeqLookup{{At: 0, Lookup: 1}, {At: 1, Lookup: 2}})}},
			{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{goA}, [][]int{{goX, goA}})}},
			{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{goA, goB}, Glyph: goA}})}},
		}, map[string][]int{"rlig": {0}}),
	})
	// Kerning stated for Cyrillic alone, which a Latin run does not get: the
	// table names none of 'latn', 'DFLT', 'dflt', so HarfBuzz selects nothing
	// from it. Rubik One names only a script tagged with four spaces.
	out["gpos-other-script"] = font(false, map[string][]byte{
		"GPOS": fonttest.GPOSTable([]fonttest.Lookup{
			{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable([]fonttest.KernPair{{Left: goA, Right: goB, Adjust: -30}})}}},
			[]fonttest.Feature{{Tag: "kern", Lookups: []int{0}}},
			map[string]fonttest.Script{"cyrl": {Required: fonttest.NoFeature, Features: []int{0}}}),
	})
	// An optional positioning feature, which applies only when it is asked
	// for: 'halt' trimming the a to half its width and moving it back, as the
	// CJK faces trim a bracket.
	out["requested"] = font(false, map[string][]byte{
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{single(goA, -250, 0, -250)},
			map[string][]int{"halt": {0}}),
	})
	// 'kern' and 'dist' pairs, for turning kerning off: a over b is kerning,
	// b over a is the distance a font states for its script.
	out["kern-and-dist"] = font(false, map[string][]byte{
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable([]fonttest.KernPair{{Left: goA, Right: goB, Adjust: -30}})}},
			{Type: 2, Subtables: [][]byte{fonttest.PairPosSubtable([]fonttest.KernPair{{Left: goB, Right: goA, Adjust: -40}})}},
		}, map[string][]int{"kern": {0}, "dist": {1}}),
	})
	// A required positioning feature, which applies whatever it is called.
	out["required"] = font(false, map[string][]byte{
		"GPOS": fonttest.GPOSTable([]fonttest.Lookup{single(goA, 0, 0, 7)},
			[]fonttest.Feature{{Tag: "zzzz", Lookups: []int{0}}},
			map[string]fonttest.Script{"DFLT": {Required: 0}}),
	})
	return out
}

// TestPositioningFollowsTheLookupOrder is the fixtures' answers, as HarfBuzz
// gives them.
func TestPositioningFollowsTheLookupOrder(t *testing.T) {
	fonts := gposOrderFixtures()
	for _, c := range []struct {
		font, text string
		want       []shapedAs
		why        string
	}{
		{"after-attach", "a\u0301", []shapedAs{{goA, 530, 0, 0}, {goAcute, 0, -380, 30}},
			"the two advances on the a add up, and the acute keeps 'dist' lowering it after 'mark' placed it"},
		{"context-after-attach", "a\u0301", []shapedAs{{goA, 500, 0, 0}, {goAcute, 0, -335, 50}},
			"the rule after the mark lookup moves the acute fifteen units on from where it was placed"},
		{"multiplied", "x\u0327", []shapedAs{{goPart1, 350, 0, 0}, {goPart2, 350, 0, 0}, {goCedilla, 0, -615, -30}},
			"the cedilla steps over the second part to the first, which the lookup covers"},
		{"fallback", "a\u0301", []shapedAs{{goA, 500, 0, 0}, {goAcute, 0, -360, 22}},
			"the acute is centred over the a's advance and lifted a sixteenth of an em clear of its ink"},
		{"fallback", "a\u0327", []shapedAs{{goA, 500, 0, 0}, {goCedilla, 0, -345, 0}},
			"the cedilla, attached below, is centred under the a and hung from its ink with no gap"},
		{"fallback", "a\u0301\u0301", []shapedAs{{goA, 500, 0, 0}, {goAcute, 0, -360, 22}, {goAcute, 0, -360, 244}},
			"the second acute is stacked on the first"},
		{"fallback", "b\u0301", []shapedAs{{goB, 600, 0, 0}, {goAcute, 0, -410, 202}},
			"over a taller letter the acute sits higher"},
		{"fallback-gdef", "a\u0301", []shapedAs{{goA, 500, 0, 0}, {goAcute, 0, -360, 22}},
			"GDEF calling the acute a base does not stop it being placed as a mark"},
		{"hebrew-dflt", "\u05D1\u05B8\u05D1", []shapedAs{{goBet, 640, 0, 0}, {goQamats, 0, 210, -22}, {goBet, 640, 0, 0}},
			"the Hebrew model does not apply positioning stated only for 'DFLT': the pair is not kerned and the qamats is placed under its bet by its ink"},
		{"fallback-ligature", "a\u0301b\u0301", []shapedAs{{goAB, 1100, 0, 0}, {goAcute, 0, -935, 22}, {goAcute, 0, -385, 22}},
			"each acute is centred over its half of the ligature"},
		{"fallback-ligature", "a\u0327b\u0301", []shapedAs{{goAB, 1100, 0, 0}, {goCedilla, 0, -920, 0}, {goAcute, 0, -385, 22}},
			"the cedilla under the first half and the acute over the second"},
		{"cursive-mark", "abb\u0301", []shapedAs{{goA, 480, 0, 0}, {goB, 560, -20, 100}, {goB, 580, -20, 200}, {goAcute, 0, -400, 300}},
			"the acute takes the height of the whole chain its b hangs from"},
		{"fallback-mark-ligature", "a\u0301b", []shapedAs{{goA, 500, 0, 0}, {goAB, 1100, 0, 0}},
			"a ligature beginning with a mark is drawn as the letter it joins and is not placed"},
		{"nested-growth", "ab", []shapedAs{{goX, 700, 0, 0}, {goA, 500, 0, 0}},
			"the ligature the second record names reaches the b the first record pushed on"},
		{"gpos-other-script", "ab", []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}},
			"positioning stated for another script alone is not applied"},
		{"required", "a", []shapedAs{{goA, 507, 0, 0}},
			"the language system's required feature applies whatever its tag"},
	} {
		f, err := Load(fonts[c.font])
		if err != nil {
			t.Fatalf("%s: %v", c.font, err)
		}
		got, _ := f.ShapeGlyphs(c.text)
		checkShaped(t, fmt.Sprintf("%s %+q: %s", c.font, c.text, c.why), got, c.want)
	}
}

// TestARequestedPositioningFeatureIsApplied: a positioning feature a caller or
// a document asks for is applied, as HarfBuzz applies it, and not otherwise.
// It was not applied at all, so a document's font-feature-settings: 'halt'
// did nothing to the glyphs.
func TestARequestedPositioningFeatureIsApplied(t *testing.T) {
	f, err := Load(gposOrderFixtures()["requested"])
	if err != nil {
		t.Fatal(err)
	}
	plain, _ := f.ShapeGlyphs("ab")
	checkShaped(t, "ab, nothing asked for", plain, []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}})
	named, _ := f.ShapeGlyphsWith("ab", "halt")
	checkShaped(t, "ab with 'halt' named", named, []shapedAs{{goA, 250, -250, 0}, {goB, 600, 0, 0}})
	doc, _ := f.ShapeGlyphsInContext("ab", "", "", Features{Tags: "halt"})
	checkShaped(t, "ab with font-feature-settings: 'halt'", doc, []shapedAs{{goA, 250, -250, 0}, {goB, 600, 0, 0}})
}

// TestTurningKerningOffLeavesDist: font-kerning: none turns 'kern' off and
// nothing else. 'dist' is the distance a font states for its script, which an
// Indic font states its conjuncts' spacing in, and HarfBuzz applies it with
// 'kern' off.
func TestTurningKerningOffLeavesDist(t *testing.T) {
	f, err := Load(gposOrderFixtures()["kern-and-dist"])
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		text string
		off  bool
		want []shapedAs
	}{
		{"ab", false, []shapedAs{{goA, 470, 0, 0}, {goB, 600, 0, 0}}},
		{"ba", false, []shapedAs{{goB, 560, 0, 0}, {goA, 500, 0, 0}}},
		{"ab", true, []shapedAs{{goA, 500, 0, 0}, {goB, 600, 0, 0}}},
		{"ba", true, []shapedAs{{goB, 560, 0, 0}, {goA, 500, 0, 0}}},
	} {
		got, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{NoKerning: c.off})
		checkShaped(t, fmt.Sprintf("%q with kerning off %t", c.text, c.off), got, c.want)
	}
}

// TestWriteGPOSOrderFixtures writes the fixtures out for asking HarfBuzz, where
// FORME_GPOS_FIXTURES names a directory; it does nothing otherwise.
func TestWriteGPOSOrderFixtures(t *testing.T) {
	dir := os.Getenv("FORME_GPOS_FIXTURES")
	if dir == "" {
		t.Skip("FORME_GPOS_FIXTURES is not set")
	}
	for name, data := range gposOrderFixtures() {
		if err := os.WriteFile(filepath.Join(dir, name+".ttf"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
