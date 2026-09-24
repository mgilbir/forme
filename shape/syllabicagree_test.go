package shape

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Fixtures for the places audit C66–C72 and C185–C194 found the shapers
// disagreeing with HarfBuzz. Each is a font built here and small enough to
// read, and each answer asserted was HarfBuzz 14.5.0's for the same font —
// shaped through uharfbuzz with REMOVE_DEFAULT_IGNORABLES, as the corpus
// oracle is — when the test was written. Positions are in font units, which
// for these 1000-unit fonts are the thousandths of an em a Glyph carries.

// agreeFont is a font whose glyphs are the runes given, in order, from glyph 1,
// with the tables given; script, when not empty, is the one script tag its GSUB
// declares, so that the run is set by that script's model.
func agreeFont(t *testing.T, glyphs []fonttest.Glyph, script string, gsub []fonttest.Lookup,
	feats []fonttest.Feature, extra map[string][]byte) *Face {
	t.Helper()
	tables := map[string][]byte{}
	for k, v := range extra {
		tables[k] = v
	}
	if script != "" {
		idx := make([]int, len(feats))
		for i := range idx {
			idx[i] = i
		}
		tables["GSUB"] = fonttest.GSUBTable(gsub, feats, map[string]fonttest.Script{
			script: {Required: fonttest.NoFeature, Features: idx},
		})
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Agree", Glyphs: glyphs, Extra: tables}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// shapedAs is a run as the oracle files write it: glyph, advance, and the
// offsets where they are not zero.
type shapedAs struct {
	gid         int
	adv, dx, dy float64
}

func checkShaped(t *testing.T, what string, got []Glyph, want []shapedAs) {
	t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = got[i].GID == want[i].gid && got[i].XAdvance == want[i].adv &&
			got[i].XOffset == want[i].dx && got[i].YOffset == want[i].dy
	}
	if ok {
		return
	}
	var have []shapedAs
	for _, g := range got {
		have = append(have, shapedAs{g.GID, g.XAdvance, g.XOffset, g.YOffset})
	}
	t.Errorf("%s: got %v, HarfBuzz gives %v", what, have, want)
}

// TestASpacingMarkIsSpacedInAFontWithNoGDEF is C66. A font that classifies no
// glyph is classified from the characters, and HarfBuzz makes a mark of a
// non-spacing mark only: the Sinhala anusvara (Mc) keeps its advance and stays
// after its letter, and an enclosing circle (Me) is spaced as a base. The
// vowel sign i (Mn) is a mark, and its advance goes — and with no GPOS to place
// it, the offset moves with it (C67), so that it stays over its letter.
//
// A Latin acute in the same font would be a mark too, and is not asked about
// here: HarfBuzz places marks itself in a font with no GPOS for the default
// model (its fallback mark positioning), which this package does not do.
func TestASpacingMarkIsSpacedInAFontWithNoGDEF(t *testing.T) {
	sinhala := agreeFont(t, []fonttest.Glyph{
		{Rune: 0x0D9A, Advance: 600, HasShape: true},
		{Rune: 0x0D82, Advance: 300, HasShape: true},
		{Rune: 0x0DD2, Advance: 250, HasShape: true},
	}, "sinh", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{1})}}},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}}, nil)
	got, _ := sinhala.ShapeGlyphs("\u0D9A\u0D82")
	checkShaped(t, "Sinhala ka and anusvara", got, []shapedAs{{1, 600, 0, 0}, {2, 300, 0, 0}})
	got, _ = sinhala.ShapeGlyphs("\u0D9A\u0DD2")
	checkShaped(t, "Sinhala ki", got, []shapedAs{{1, 600, 0, 0}, {3, 0, -250, 0}})

	latin := agreeFont(t, []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 0x20DD, Advance: 700, HasShape: true},
	}, "", nil, nil, nil)
	got, _ = latin.ShapeGlyphs("a\u20DD")
	checkShaped(t, "a in an enclosing circle", got, []shapedAs{{1, 500, 0, 0}, {2, 700, 0, 0}})

	for r, want := range map[rune]int{
		0x0301: classMark, 0x0DD2: classMark, 0x0D82: classBase, 0x20DD: classBase,
		0x180B: classBase, 0x034F: classBase, 'a': classBase,
	} {
		if got := classOfRune(r); got != want {
			t.Errorf("U+%04X is class %d, want %d", r, got, want)
		}
	}
}

// zeroingFont is a letter and a mark with an advance of its own, in a font
// whose GDEF says the mark is one and whose GSUB declares script; with gpos, it
// has a GPOS table as well, stating nothing about either glyph.
func zeroingFont(t *testing.T, letter, mark rune, script string, gpos bool) *Face {
	t.Helper()
	extra := map[string][]byte{"GDEF": fonttest.GDEF(map[int]int{1: 1, 2: 3})}
	if gpos {
		extra["GPOS"] = fonttest.GPOS([]fonttest.KernPair{{Left: 1, Right: 1, Adjust: -10}})
	}
	return agreeFont(t, []fonttest.Glyph{
		{Rune: letter, Advance: 600, HasShape: true},
		{Rune: mark, Advance: 300, HasShape: true},
	}, script, []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{1})}}},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}}, extra)
}

// TestAZeroedMarkMovesOnlyWhereTheFontPlacesNone is C67. Cancelling a mark's
// advance moves its offset with it, so that it stays over its letter, only in a
// font with no GPOS: one that has GPOS places its marks against a pen that has
// already stopped, and moving the mark as well draws it an advance short —
// Padauk's medial ra, 219 units left. The same for the models that cancel
// before the rules (the universal engine, Balinese here) and after them (Thai).
// Right to left the mark is never moved (Adlam).
func TestAZeroedMarkMovesOnlyWhereTheFontPlacesNone(t *testing.T) {
	for _, c := range []struct {
		script       string
		letter, mark rune
		gpos         bool
		want         []shapedAs
	}{
		{"thai", 0x0E01, 0x0E34, false, []shapedAs{{1, 600, 0, 0}, {2, 0, -300, 0}}},
		{"thai", 0x0E01, 0x0E34, true, []shapedAs{{1, 600, 0, 0}, {2, 0, 0, 0}}},
		{"bali", 0x1B13, 0x1B36, false, []shapedAs{{1, 600, 0, 0}, {2, 0, -300, 0}}},
		{"bali", 0x1B13, 0x1B36, true, []shapedAs{{1, 600, 0, 0}, {2, 0, 0, 0}}},
		{"adlm", 0x1E900, 0x1E944, false, []shapedAs{{2, 0, 0, 0}, {1, 600, 0, 0}}},
	} {
		f := zeroingFont(t, c.letter, c.mark, c.script, c.gpos)
		got, _ := f.ShapeGlyphs(string([]rune{c.letter, c.mark}))
		checkShaped(t, fmt.Sprintf("%s, GPOS %t", c.script, c.gpos), got, c.want)
	}
	// A legacy kern table that kerns across the line counts as placing them.
	crossOT := []byte{0, 0, 0, 1, 0, 0, 0, 14, 0x00, 0x04, 0, 0, 0, 0, 0, 0, 0, 0}
	crossAAT := []byte{0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 16, 0x40, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	plain := []byte{0, 0, 0, 1, 0, 0, 0, 14, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 0, 0}
	for name, c := range map[string]struct {
		table []byte
		want  bool
	}{"OpenType cross-stream": {crossOT, true}, "Apple cross-stream": {crossAAT, true},
		"horizontal": {plain, false}, "none": {nil, false}} {
		if got := hasCrossStreamKerning(c.table); got != c.want {
			t.Errorf("%s: cross-stream %t, want %t", name, got, c.want)
		}
	}
}

// TestAMarkFollowsItsTargetAlongTheLineAndNotAcross is the Tibetan difference
// the audit carried over: U+0F67 U+0FAC U+0FB9 U+0F77 in Noto Serif Tibetan.
// Lookup 20 stacks the subjoined ra on the mark before it; lookup 21 then
// attaches that mark elsewhere. HarfBuzz keeps the height the ra was given and
// moves it along the line with its target, and so does everything stacked on
// the ra. Settling both axes when each mark was attached drew the ra and the
// mark on it 42 units short.
func TestAMarkFollowsItsTargetAlongTheLineAndNotAcross(t *testing.T) {
	f := corpusFace(t, "NotoSerifTibetan.ttf")
	got, _ := f.ShapeGlyphs("\u0F67\u0FAC\u0FB9\u0F77")
	// HarfBuzz 14.5.0, in font units; the face is 1000 units to the em.
	if f.UnitsPerEm() != 1000 {
		t.Fatalf("the fixture assumption is gone: Noto Serif Tibetan is %d units to the em", f.UnitsPerEm())
	}
	checkShaped(t, "U+0F67 U+0FAC U+0FB9 U+0F77", got, []shapedAs{
		{134, 562, 0, 0}, {1736, 0, -574, -6}, {1836, 0, -533, -702},
		{1420, 0, -491, -273}, {1422, 0, -358, -790}, {1347, 0, -557, 0},
	})
}

// mongolianMVSFont is a Mongolian font whose feature tag, when it is not
// empty, gives the vowel separator a glyph of its own (glyph 3, the narrow
// space the font draws before a final A).
func mongolianMVSFont(t *testing.T, tag string) *Face {
	t.Helper()
	from, to, feature := []int{1}, []int{1}, "ccmp"
	if tag != "" {
		from, to, feature = []int{2}, []int{3}, tag
	}
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x1820, Advance: 500, HasShape: true},
		{Rune: 0x180E, Advance: 0, HasShape: false},
		{Rune: 0xE000, Advance: 55, HasShape: false},
	}, "mong", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst(from, to)}}},
		[]fonttest.Feature{{Tag: feature, Lookups: []int{0}}}, nil)
}

// balineseSplitFont is a Balinese font whose 'ccmp' takes the vowel sign
// taling tarung (U+1B3E, drawn before its letter) apart into a taling (glyph
// 3) and a tedung (glyph 4, U+1B35).
func balineseSplitFont(t *testing.T) *Face {
	t.Helper()
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x1B13, Advance: 600, HasShape: true},
		{Rune: 0x1B3E, Advance: 400, HasShape: true},
		{Rune: 0xE000, Advance: 200, HasShape: true},
		{Rune: 0x1B35, Advance: 200, HasShape: true},
	}, "bali", []fonttest.Lookup{{Type: 2, Subtables: [][]byte{fonttest.MultipleSubst([]int{2}, [][]int{{3, 4}})}}},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}}, nil)
}

// TestAnIgnorableTheFontDrawsIsKept is C185. The Mongolian vowel separator is
// a character nothing is drawn for, and a font may draw it anyway: Noto Sans
// Mongolian substitutes a narrow or a wide space for it before a final A or E,
// in 'calt'. HarfBuzz keeps a glyph a substitution touched and drops the rest.
// It forgets, in the universal engine, what the pre-processing features did —
// it clears the record after them to see where 'rphf' and 'pref' apply — so
// the same substitution in 'ccmp' is dropped like an untouched one.
func TestAnIgnorableTheFontDrawsIsKept(t *testing.T) {
	for _, c := range []struct {
		tag  string
		want []shapedAs
	}{
		{"calt", []shapedAs{{1, 500, 0, 0}, {3, 55, 0, 0}, {1, 500, 0, 0}}},
		{"ccmp", []shapedAs{{1, 500, 0, 0}, {1, 500, 0, 0}}},
		{"", []shapedAs{{1, 500, 0, 0}, {1, 500, 0, 0}}},
	} {
		got, _ := mongolianMVSFont(t, c.tag).ShapeGlyphs("\u1820\u180E\u1820")
		checkShaped(t, "the vowel separator substituted in "+c.tag, got, c.want)
	}
}

// TestOnlyTheFirstPartOfASplitVowelMoves is C71. 'ccmp' takes the Balinese
// taling tarung apart into a taling, drawn before the letter, and a tedung,
// drawn after it; the engine moves the first part of the decomposition and
// leaves the rest. Nothing numbered the parts, so the guard that says "the
// first" passed every one of them and the tedung went to the front too.
func TestOnlyTheFirstPartOfASplitVowelMoves(t *testing.T) {
	got, _ := balineseSplitFont(t).ShapeGlyphs("\u1B13\u1B3E")
	checkShaped(t, "ka with taling tarung", got, []shapedAs{{3, 200, 0, 0}, {1, 600, 0, 0}, {4, 200, 0, 0}})
}

// TestAVowelNobodyWritesIsCircledInTheUniversalEngine is C69. The table of
// sequences that spell a vowel with the wrong characters covers seven scripts
// the universal engine sets, and only the Indic model consulted it, so Sinhala
// අා (U+0D85 U+0DCF, which is ආ written in two pieces) was drawn as though it
// were a word.
func TestAVowelNobodyWritesIsCircledInTheUniversalEngine(t *testing.T) {
	f := agreeFont(t, []fonttest.Glyph{
		{Rune: 0x0D85, Advance: 600, HasShape: true},
		{Rune: 0x0DCF, Advance: 300, HasShape: true},
		{Rune: dottedCircle, Advance: 500, HasShape: true},
	}, "sinh", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{1})}}},
		[]fonttest.Feature{{Tag: "ccmp", Lookups: []int{0}}}, nil)
	got, _ := f.ShapeGlyphs("\u0D85\u0DCF")
	checkShaped(t, "Sinhala a with aa-sign", got, []shapedAs{{1, 600, 0, 0}, {3, 500, 0, 0}, {2, 300, 0, 0}})
}

// devaChainedBlwf is a 'blwf' written as a chained rule: after a Ka, a virama
// and Ra are the below-base Ra.
func devaChainedBlwf() devaFeature {
	return devaFeature{tag: "blwf", build: func(base int) ([]fonttest.Lookup, []int) {
		return []fonttest.Lookup{
			{Type: 6, Subtables: [][]byte{fonttest.ChainedContext3(
				[][]int{{gidDKa}}, [][]int{{gidVirama}, {gidDRa}}, nil,
				[]fonttest.SeqLookup{{At: 0, Lookup: base + 1}})}},
			{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
				{Components: []int{gidVirama, gidDRa}, Glyph: gidRakar}})}},
		}, []int{base}
	}}
}

// TestTheFontIsAskedAboutAConsonantAsHarfBuzzAsksIt is C72 and C194: which
// consonants the font draws below or after the base, which the base search
// turns on.
//
// A font stating its below-base forms under 'vatu' alone was asked nothing —
// the question was skipped for want of 'blwf', 'pstf' and 'pref' — so त्र had
// no base but the Ra. A 'pstf' holding a lone alternate for Ra answered "post-
// base" for Ra, though a single substitution takes one glyph and not the
// virama and Ra it was asked about. And a chained rule that needs a letter
// before the pair counts for a font written to the first-generation
// specification and not for one written to the second, which HarfBuzz asks
// with no context at all.
func TestTheFontIsAskedAboutAConsonantAsHarfBuzzAsksIt(t *testing.T) {
	const kra, tra = "\u0915\u094D\u0930", "\u0924\u094D\u0930"
	for _, c := range []struct {
		what string
		face *Face
		text string
		want []shapedAs
	}{
		{"below-base forms under 'vatu' alone", devaFaceIn(t, "dev2",
			devaLigatures("vatu", fonttest.Ligature{Components: []int{gidVirama, gidDRa}, Glyph: gidRakar}), devaHalf()),
			tra, []shapedAs{{gidDTa, 310, 0, 0}, {gidRakar, 450, 0, 0}}},
		{"a lone alternate for Ra in 'pstf'", devaFaceIn(t, "dev2",
			devaSingle("pstf", []int{gidDRa}, []int{gidRaAlt}), devaHalf()),
			kra, []shapedAs{{gidDKa, 300, 0, 0}, {gidVirama, 330, 0, 0}, {gidDRa, 320, 0, 0}}},
		{"the same, after a letter the font makes half", devaFaceIn(t, "dev2",
			devaSingle("pstf", []int{gidDRa}, []int{gidRaAlt}), devaHalf()),
			tra, []shapedAs{{gidTaHalf, 400, 0, 0}, {gidDRa, 320, 0, 0}}},
		{"a chained 'blwf', second generation", devaFaceIn(t, "dev2", devaChainedBlwf(), devaHalf()),
			kra, []shapedAs{{gidDKa, 300, 0, 0}, {gidVirama, 330, 0, 0}, {gidDRa, 320, 0, 0}}},
		{"a chained 'blwf', first generation", devaFaceIn(t, "deva", devaChainedBlwf(), devaHalf()),
			kra, []shapedAs{{gidDKa, 300, 0, 0}, {gidDRa, 320, 0, 0}, {gidVirama, 330, 0, 0}}},
	} {
		got, _ := c.face.ShapeGlyphs(c.text)
		checkShaped(t, c.what, got, c.want)
	}
}

// malayalamDotRephFont is a Malayalam font, written to the second-generation
// specification, with the dot reph, a digit, a no-break space, a letter and
// the dotted circle.
func malayalamDotRephFont(t *testing.T) *Face {
	t.Helper()
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x0D4E, Advance: 0, HasShape: true},
		{Rune: 0x0D66, Advance: 500, HasShape: true},
		{Rune: 0x00A0, Advance: 250, HasShape: true},
		{Rune: dottedCircle, Advance: 450, HasShape: true},
		{Rune: 0x0D15, Advance: 600, HasShape: true},
	}, "mlm2", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{5}, []int{5})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}}, nil)
}

// TestADotRephBeforeAPlaceholderIsOneCluster is C68. HarfBuzz's grammar reads
// a reph before a placeholder — a digit, a no-break space — as a standalone
// cluster and sets the reph on it. Taken as a broken cluster first, the dot
// reph was put on a dotted circle of its own and the placeholder left apart.
func TestADotRephBeforeAPlaceholderIsOneCluster(t *testing.T) {
	f := malayalamDotRephFont(t)
	for text, want := range map[string][]shapedAs{
		"\u0D4E\u0D66": {{2, 500, 0, 0}, {1, 0, 0, 0}},
		"\u0D4E\u00A0": {{3, 250, 0, 0}, {1, 0, 0, 0}},
		"\u0D4E\u0D15": {{5, 600, 0, 0}, {1, 0, 0, 0}},
		"\u0D4E":       {{4, 450, 0, 0}, {1, 0, 0, 0}},
	} {
		got, _ := f.ShapeGlyphs(text)
		checkShaped(t, fmt.Sprintf("%+q", text), got, want)
	}
}

// TestTheDottedCircleComesAfterCcmp is C189. HarfBuzz puts the dotted circle
// into a broken Indic cluster after 'locl' and 'ccmp' have run, so a font
// whose 'ccmp' composes a dotted circle with a vowel sign composes the one the
// text wrote and not the one the shaper added. Inserted before them, the
// shaper's own circle was composed too.
func TestTheDottedCircleComesAfterCcmp(t *testing.T) {
	f := devaFaceIn(t, "dev2", devaLigatures("ccmp",
		fonttest.Ligature{Components: []int{gidDotted, gidIMatra}, Glyph: gidKaWithI}), devaHalf())
	for text, want := range map[string][]shapedAs{
		"\u093F":       {{gidIMatra, 340, 0, 0}, {gidDotted, 520, 0, 0}},
		"\u25CC\u093F": {{gidKaWithI, 430, 0, 0}},
	} {
		got, _ := f.ShapeGlyphs(text)
		checkShaped(t, fmt.Sprintf("%+q", text), got, want)
	}
}

// TestACharacterThatSaysNothingDoesNotEndAWord is C192: 'init' is for an
// i-sign that opens a word, and HarfBuzz reads a private-use character or an
// unassigned one before it as part of the word, as it reads a letter.
func TestACharacterThatSaysNothingDoesNotEndAWord(t *testing.T) {
	f := devaFaceIn(t, "dev2", devaSingle("init", []int{gidIMatra}, []int{gidIMatraIni}), devaHalf())
	for text, want := range map[string][]shapedAs{
		"\uE000\u0915\u093F": {{gidReph, 390, 0, 0}, {gidIMatra, 340, 0, 0}, {gidDKa, 300, 0, 0}},
		"\u0378\u0915\u093F": {{0, 0, 0, 0}, {gidIMatra, 340, 0, 0}, {gidDKa, 300, 0, 0}},
		" \u0915\u093F":      {{gidSpace, 470, 0, 0}, {gidIMatraIni, 420, 0, 0}, {gidDKa, 300, 0, 0}},
	} {
		got, _ := f.ShapeGlyphs(text)
		checkShaped(t, fmt.Sprintf("%+q", text), got, want)
	}
}

// granthaFont is a Grantha font with a letter, the visarga, the combining
// anusvara above and the dotted circle.
func granthaFont(t *testing.T) *Face {
	t.Helper()
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x11314, Advance: 600, HasShape: true},
		{Rune: 0x11303, Advance: 300, HasShape: true},
		{Rune: 0x11300, Advance: 0, HasShape: true},
		{Rune: dottedCircle, Advance: 450, HasShape: true},
	}, "gran", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{1})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}}, nil)
}

// TestAGranthaVisargaTakesACombiningAnusvara is C187: HarfBuzz places the
// Grantha visarga above, as a vowel modifier the combining anusvara may follow,
// and reads 𑌔𑌃𑌀 as one cluster. Read as post-base, the anusvara after it began
// a broken cluster with a dotted circle of its own.
func TestAGranthaVisargaTakesACombiningAnusvara(t *testing.T) {
	got, _ := granthaFont(t).ShapeGlyphs("\U00011314\U00011303\U00011300")
	checkShaped(t, "Grantha o with visarga and anusvara", got,
		[]shapedAs{{1, 600, 0, 0}, {2, 300, 0, 0}, {3, 0, 0, 0}})
}

// TestAGlyphTheShaperMakesIsClassified is C193. Every glyph made from a
// character is classified as the character implies where a font classifies
// nothing, and the ones a shaper made carried no class and were read off the
// mark tables instead. The parts of a split vowel sign are classified as the
// characters they are — HarfBuzz makes them in normalization, before it infers
// classes. A dotted circle the shaper inserts HarfBuzz gives no class at all:
// not a mark, stepped over by no lookup flag, whatever GDEF says of the glyph,
// until a substitution touches it.
func TestAGlyphTheShaperMakesIsClassified(t *testing.T) {
	tamil := agreeFont(t, []fonttest.Glyph{
		{Rune: 0x0B95, Advance: 600, HasShape: true},
		{Rune: 0x0BCA, Advance: 700, HasShape: true},
		{Rune: 0x0BC6, Advance: 300, HasShape: true},
		{Rune: 0x0BBE, Advance: 400, HasShape: true},
	}, "tml2", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1}, []int{1})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}}, nil)
	sh := shaper{f: tamil, l: tamil.layoutFor(scriptOf(0x0B95), otLanguage{})}
	buf, runes := sh.splitMatras([]Glyph{{GID: 1}, {GID: 2}}, []rune{0x0B95, 0x0BCA})
	if len(buf) != 3 || len(runes) != 3 {
		t.Fatalf("the fixture assumption is gone: ொ split into %d glyphs", len(buf)-1)
	}
	for i, r := range runes[1:] {
		if got, want := buf[1+i].class, classOfRune(r); got != want {
			t.Errorf("the part U+%04X is class %d, want %d", r, got, want)
		}
	}

	for _, gdef := range []bool{false, true} {
		extra := map[string][]byte{}
		if gdef {
			extra["GDEF"] = fonttest.GDEF(map[int]int{1: classBase, 2: classMark})
		}
		f := agreeFont(t, []fonttest.Glyph{
			{Rune: dottedCircle, Advance: 450, HasShape: true},
			{Rune: 0x0301, Advance: 0, HasShape: true},
		}, "", nil, nil, extra)
		l := f.layoutFor(scriptOf('a'), otLanguage{})
		sh := shaper{f: f, l: l}
		inserted, _ := sh.insertGlyphAt([]Glyph{{GID: 2}}, []indicInfo{{}}, 0, 1, indicInfo{})
		circle := inserted[0]
		if got := l.classOf(circle); got != classUnclassified {
			t.Errorf("GDEF %t: an inserted dotted circle is class %d, want none", gdef, got)
		}
		if l.isMark(circle) {
			t.Errorf("GDEF %t: an inserted dotted circle reads as a mark", gdef)
		}
		for _, flag := range []int{flagIgnoreBaseGlyphs, flagIgnoreMarks, flagIgnoreLigatures} {
			if l.ignoresIn(flag, -1, circle) {
				t.Errorf("GDEF %t: an inserted dotted circle is stepped over by flag %#x", gdef, flag)
			}
		}
		circle.substituted = true
		want := classUnclassified
		if gdef {
			want = classBase
		}
		if got := l.classOf(circle); got != want {
			t.Errorf("GDEF %t: a substituted dotted circle is class %d, want %d", gdef, got, want)
		}
	}
}

// TestMarksOfDifferentLigaturePartsDoNotLigate is the difference the
// differential fuzzer kept finding in Noto Sans Javanese: ꦰꦑ꦳ꦾꦷ. 'psts' makes
// kha and pengkal one glyph across the cecak telu between them, which is then a
// mark of the ligature's first part; the vowel sign ii after it belongs to no
// part. 'abvs' ligates cecak telu and ii on one letter, and HarfBuzz does not
// apply it to these two: an input may not mix marks of different parts of a
// ligature.
func TestMarksOfDifferentLigaturePartsDoNotLigate(t *testing.T) {
	f := corpusFace(t, "NotoSansJavanese.ttf")
	if f.UnitsPerEm() != 1000 {
		t.Fatalf("the fixture assumption is gone: Noto Sans Javanese is %d units to the em", f.UnitsPerEm())
	}
	got, _ := f.ShapeGlyphs("\uA9B0\uA991\uA9B3\uA9BE\uA9B7")
	checkShaped(t, "ssa kha cecak-telu pengkal ii", got, []shapedAs{
		{59, 1012, 0, 0}, {144, 1665, 0, 0}, {62, 0, -637, 10}, {84, 0, -425, 10},
	})
}

// ligaturePartsFont is a Latin font in which 'liga' makes x and y one glyph
// across the marks between them (glyph 5), and 'calt' turns a mark m followed
// by a mark n into m′ (glyph 6) — by a contextual rule whose input is m, n.
func ligaturePartsFont(t *testing.T) *Face {
	t.Helper()
	const x, y, m, n, xy, mAlt = 1, 2, 3, 4, 5, 6
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 4, Flag: 0x08, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{x, y}, Glyph: xy}})}},
		{Type: 5, Subtables: [][]byte{fonttest.SequenceContext3([][]int{{m}, {n}}, []fonttest.SeqLookup{{At: 0, Lookup: 2}})}},
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{m}, []int{mAlt})}},
	}, map[string][]int{"liga": {0}, "calt": {1}})
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 'x', Advance: 500, HasShape: true},
		{Rune: 'y', Advance: 500, HasShape: true},
		{Rune: 0x0301, Advance: 0, HasShape: true},
		{Rune: 0x0302, Advance: 0, HasShape: true},
		{Rune: 0xE000, Advance: 900, HasShape: true},
		{Rune: 0xE001, Advance: 0, HasShape: true},
	}, "", nil, nil, map[string][]byte{
		"GSUB": gsub,
		"GDEF": fonttest.GDEF(map[int]int{x: 1, y: 1, m: 3, n: 3, xy: 2, mAlt: 3}),
	})
}

// TestAContextInputDoesNotMixLigatureParts is the same rule for a contextual
// rule's input: the acute is a mark of the xy ligature's first part and the
// circumflex after the ligature belongs to no part, so the rule written about
// an acute and a circumflex on one letter does not apply to them. Written on
// one letter, it does.
func TestAContextInputDoesNotMixLigatureParts(t *testing.T) {
	f := ligaturePartsFont(t)
	for text, want := range map[string][]int{
		"x\u0301y\u0302": {5, 3, 4},
		"x\u0301\u0302":  {1, 6, 4},
	} {
		got, _ := f.ShapeGlyphs(text)
		if gids := gidsOfGlyphs(got); !equalInts(gids, want) {
			t.Errorf("%+q shaped to %v, HarfBuzz gives %v", text, gids, want)
		}
	}
}

// nuktaFont is a Devanagari font that has ऩ (U+0929) whole as well as न and
// the nukta it is made of, and the aa-sign; and balineseTedungFont is a
// Balinese one that has ᬈ (U+1B08) whole as well as ᬇ and the tedung.
func nuktaFont(t *testing.T) *Face {
	t.Helper()
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x0929, Advance: 700, HasShape: true},
		{Rune: 0x0928, Advance: 600, HasShape: true},
		{Rune: 0x093C, Advance: 0, HasShape: true},
		{Rune: 0x093E, Advance: 250, HasShape: true},
	}, "dev2", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{2}, []int{2})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}}, nil)
}

func balineseTedungFont(t *testing.T) *Face {
	t.Helper()
	return agreeFont(t, []fonttest.Glyph{
		{Rune: 0x1B08, Advance: 900, HasShape: true},
		{Rune: 0x1B07, Advance: 700, HasShape: true},
		{Rune: 0x1B35, Advance: 250, HasShape: true},
	}, "bali", []fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{2}, []int{2})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}}, nil)
}

// TestASyllabicRunIsComposedOnlyWhereTheTextHadMarks is C186. A syllabic run
// is taken apart whether or not the face has a character whole, and HarfBuzz
// composes it again only where the text itself wrote a mark: ऩ alone stays न
// and a nukta, and the font's own 'nukt' is what joins them if it means to;
// ऩ with a vowel sign after it is composed. Composing both drew the
// precomposed glyph where HarfBuzz draws the parts, at another width.
func TestASyllabicRunIsComposedOnlyWhereTheTextHadMarks(t *testing.T) {
	nukta := nuktaFont(t)
	for text, want := range map[string][]shapedAs{
		"\u0929":       {{2, 600, 0, 0}, {3, 0, 0, 0}},
		"\u0929\u093E": {{1, 700, 0, 0}, {4, 250, 0, 0}},
	} {
		got, _ := nukta.ShapeGlyphs(text)
		checkShaped(t, fmt.Sprintf("%+q", text), got, want)
	}
	got, _ := balineseTedungFont(t).ShapeGlyphs("\u1B08")
	checkShaped(t, "Balinese akara tedung", got, []shapedAs{{2, 700, 0, 0}, {3, 250, 0, 0}})
}
