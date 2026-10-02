package shape

import (
	"context"
	"encoding/binary"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// The font API against input it did not choose.
//
// A font file is the least trustworthy thing this module is handed. It is
// offset-driven and self-referential throughout — every table points at another
// by byte offset, and every one of those offsets is a number in the file — so a
// reader that trusts one indexes wherever it is told. This package is on the
// *writing* side, which makes it worse rather than better: a caller embedding a
// user-supplied font in a generated document is running this on bytes an
// attacker chose, inside a process that is doing something else.
//
// So the contract is the same as for the document writer: every entry point
// reports, and none of them panics.

// allocator is the smallest thing Embed needs.
type allocator struct{ n int }

func noPanic(t *testing.T, what string, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s panicked: %v", what, r)
		}
	}()
	f()
}

// hostileFontBytes are the shapes a font file can take that are not a font.
func hostileFontBytes() map[string][]byte {
	good := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	})
	truncated := append([]byte(nil), good[:len(good)/2]...)

	// A valid header whose table directory points past the end.
	badOffsets := append([]byte(nil), good...)
	for i := 12; i+16 <= 12+16*4 && i+16 <= len(badOffsets); i += 16 {
		badOffsets[i+8] = 0x7F // an offset near the top of the range
		badOffsets[i+9] = 0xFF
	}

	zeroed := make([]byte, len(good))
	copy(zeroed, good[:12])

	return map[string][]byte{
		"nil":                       nil,
		"empty":                     {},
		"one byte":                  {0x00},
		"the sfnt magic alone":      {0x00, 0x01, 0x00, 0x00},
		"OTTO magic alone":          []byte("OTTO"),
		"a truncated font":          truncated,
		"offsets past the end":      badOffsets,
		"a header and nothing else": zeroed,
		"text":                      []byte(strings.Repeat("not a font ", 100)),
		"all zeroes":                make([]byte, 4096),
		"all ones":                  bytesRepeat(0xFF, 4096),
	}
}

func bytesRepeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}

// exerciseFace runs everything a caller can do with a face that loaded. A font
// that parses is not a font that is sane: the tables may still disagree with
// each other, and it is the second stage that meets that.
func exerciseFace(t *testing.T, f *Face) {
	t.Helper()
	const sample = "Hello — Ωμέγα 日本 ́́ ﬁ \u0930\u094D\u0915\u094D\u0924\u093F\u0902"

	noPanic(t, "Name", func() { _ = f.Name() })
	noPanic(t, "NumGlyphs", func() { _ = f.NumGlyphs() })
	noPanic(t, "IsSimple/IsStandard", func() { _, _ = f.IsSimple(), f.IsStandard() })
	noPanic(t, "Features", func() { _ = f.Features() })
	noPanic(t, "GlyphID", func() {
		for _, r := range []rune{0, 'a', 0x10FFFF, -1, 0xFFFD} {
			_, _ = f.GlyphID(r)
		}
	})
	noPanic(t, "Advance", func() {
		for _, r := range []rune{0, 'a', 0x10FFFF} {
			_, _ = f.Advance(r)
		}
	})
	noPanic(t, "Measure", func() { _ = f.Measure(sample, 12) })
	noPanic(t, "MeasureShaped", func() { _ = f.MeasureShaped(sample, 12) })
	noPanic(t, "Encode", func() { _, _ = f.Encode(sample) })
	noPanic(t, "ShapeGlyphs", func() { _, _ = f.ShapeGlyphs(sample) })
	noPanic(t, "measuring hand-made glyphs", func() {
		// Glyph indices a caller could hold from another font entirely.
		_ = MeasureGlyphs([]Glyph{
			{GID: -1, XAdvance: 1},
			{GID: 1 << 20, XAdvance: -1},
			{GID: 0, XOffset: 1e300, YOffset: -1e300},
		}, 12)
	})
	noPanic(t, "Subset", func() { _, _ = f.Subset() })
	noPanic(t, "Used", func() { _ = f.Used() })
}

// fuzzTexts carry one character of every kind the shaping paths branch on.
//
// They are separate strings rather than one, because which of the font's rules
// apply is decided by the *run's* script and a run takes the script of its
// first letter that has one: a Devanagari syllable written after a Latin letter
// is shaped as Latin, and the reordering is never reached. A shaper that is not
// reached is not fuzzed, and each of the three is reached only by a run of its
// own.
var fuzzTexts = []string{
	// A plain letter, a ligature and an unmapped ideograph.
	"aﬁ日 ",
	// Devanagari: a reph, a conjunct and a pre-base vowel sign.
	"\u0930\u094D\u0915\u094D\u0924\u093F\u0902",
	// Khmer: a subscript Ro, which moves to the front of the syllable, and a
	// vowel sign written as one character and drawn as two.
	"\u1780\u17D2\u179A\u17C4\u17C7",
	// Myanmar: a kinzi, a medial Ra and a below-base sign with a mark after it.
	"\u1004\u103A\u1039\u1000\u103C\u102F\u1036",
	// Javanese: a stacked pair carrying a vowel, through the universal engine.
	"\uA98F\uA9C0\uA9A0\uA9BA",
	// Balinese: the split vowel sign, drawn as two marks on opposite sides.
	"\u1B13\u1B44\u1B14\u1B40",
	// Tibetan: a consonant with a subjoined form and a vowel, which is where the
	// mark glyph sets and the long lookup list are read.
	"\u0F40\u0F90\u0F71\u0F72",
	// Arabic: letters that join both ways, a hamza and a vowel, so the joining
	// forms and the mark ordering are both reached.
	"\u0628\u0640\u0628\u0648\u0655\u064E",
	// Both directions in one string, with a number between them. Nothing above
	// reaches the reordering: every one of them is a run of one direction, and
	// the code that cuts a string into runs and puts them back in drawing order
	// only runs when there is more than one.
	"a\u05D0b 12 \u0628\u0644\u0627 c",
	// A directional control with nothing to match it, which is the shape of an
	// override a document can carry.
	"a\u202Eb\u0628c",
}

// FuzzLoadAndUse drives the whole writing pipeline on arbitrary bytes: parse,
// shape, subset, embed. Fuzzing fails on a panic by itself, so the body only
// has to reach every stage.
//
// The seeds are the synthetic fonts this package's own tests use, so the corpus
// starts from things that are *nearly* valid — which is where the interesting
// failures are. A file that is obviously not a font is rejected in the first
// four bytes and exercises nothing.
//
// Two of the files in testdata/fuzz/FuzzLoadAndUse are what this target found
// before the engine was a module of its own, and until then they sat where the
// move had put them, under testdata/testdata, where no test read them.
// efac75b6c4c86727 declares numGlyphs of zero, and panicked in the subsetter
// until the subsetter refused a font that declares no .notdef; it panics again
// with that refusal taken out. 308a0a71a6690515 is 533 KB mutated from a real font whose
// lookups each claim room for tens of thousands of subtables, and took half a
// minute to read until the subtables shared one budget per table. It cannot
// fail on its own — without the budget it only takes eleven times as long —
// and TestADenseLookupListIsBoundedByTheTable is what holds that line; the file
// is kept because a mutation of a real font is where the fuzzer finds the next
// one.
func FuzzLoadAndUse(f *testing.F) {
	f.Add(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	}))
	f.Add(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'f', Advance: 300, HasShape: true}},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUB([]fonttest.Ligature{{Components: []int{1, 1}, Glyph: 1}}),
			"GPOS": fonttest.GPOS([]fonttest.KernPair{{Left: 1, Right: 1, Adjust: -50}}),
			"GDEF": fonttest.GDEF(map[int]int{1: 1}),
		},
	}))
	// A Devanagari seed: the reordering reads the font too — it asks whether
	// there is a reph for this Ra and what the below-base forms cover — so a
	// crafted font reaches it, and only a seed declaring those features gets the
	// fuzzer near enough to try.
	f.Add(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 0x0915, Advance: 500, HasShape: true}, // ka
			{Rune: 0x0930, Advance: 500, HasShape: true}, // ra
			{Rune: 0x094D, Advance: 0, HasShape: true},   // virama
			{Rune: 0x093F, Advance: 0, HasShape: true},   // the i-sign
			{Rune: 0xE000, Advance: 200, HasShape: true}, // a reph
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{{Type: 4, Subtables: [][]byte{
					fonttest.LigatureSubst([]fonttest.Ligature{
						{Components: []int{2, 3}, Glyph: 5},
					}),
				}}},
				[]fonttest.Feature{
					{Tag: "blwf", Lookups: []int{0}},
					{Tag: "half", Lookups: []int{0}},
					{Tag: "rphf", Lookups: []int{0}},
				},
				map[string]fonttest.Script{"dev2": fonttest.AllFeatures(3)},
			),
		},
	}))
	// A contextual rule whose first record ligates everything it matched and
	// whose second names a position the ligature swallowed. Moving that position
	// by the change put it at -1, and the second record applied a lookup there:
	// an index out of range from one font. The three glyphs are the first three
	// characters of the first of fuzzTexts, which is what reaches the rule.
	f.Add(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'ﬁ', Advance: 500, HasShape: true},
			{Rune: '日', Advance: 500, HasShape: true},
			{Rune: 'X', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{
				{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
					{Components: []int{1, 2, 3}, Glyph: 4},
				})}},
				{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{1, 2, 3}, []int{4, 4, 4})}},
				{Type: 5, Subtables: [][]byte{fonttest.SequenceContext3(
					[][]int{{1}, {2}, {3}},
					[]fonttest.SeqLookup{{At: 0, Lookup: 0}, {At: 1, Lookup: 1}},
				)}},
			}, map[string][]int{"calt": {2}}),
		},
	}))
	f.Add([]byte("OTTO\x00\x00\x00\x00"))
	f.Add([]byte{})

	// And the real fonts, which are the only seeds that reach most of this.
	//
	// A synthetic seed declares a handful of lookups and one subtable each; the
	// fonts in testdata/harfbuzz declare up to 1190 lookups, up to 738 subtables
	// in a single lookup, twenty mark glyph sets and every kind of contextual
	// rule. Those are the paths where the bounds live, and a mutation of a real
	// font arrives at them already well-formed enough to get inside — which is
	// exactly where a reader that trusted a count would be caught.
	//
	// The largest is left out on purpose: it is 2 MB, and a fuzzer spends its
	// time proportionally to the size of what it mutates.
	for _, name := range []string{
		"NotoSansBalinese.ttf",  // 338 contextual positioning subtables
		"NotoSansJavanese.ttf",  // the universal engine's features
		"NotoSansArabic.ttf",    // cursive joining and mark filtering
		"CFF2Blend.otf",         // CFF2: blends, Font DICTs, HVAR, VVAR and VORG
		"SbixInk.ttf",           // sbix strikes, duplicates and image formats
		"PointMatchPhantom.ttf", // components placed by matching points, instanced
		"PointMatch.ttf",        // and matched as an instance keeps them
		"VarComposite.ttf",      // variable composites (VARC), measured, drawn, instanced
	} {
		if data, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", name)); err == nil {
			f.Add(data)
		}
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		for _, load := range []func([]byte) (*Face, error){Load, LoadSimple} {
			face, err := load(data)
			if err != nil || face == nil {
				continue
			}
			useFace(face)

			// The instance, which is a second reader over the same bytes: the
			// variation tables are read here and nowhere else, and a face
			// carrying a hostile fvar or gvar reaches this and not Load.
			coords := map[string]float64{}
			for _, a := range face.Axes() {
				// The far end of the range rather than the default, so that the
				// deltas are actually applied.
				coords[a.Tag] = a.Max
			}
			if inst, err := LoadInstance(data, coords); err == nil && inst != nil {
				useFace(inst)
			}
			// And with an axis the font does not declare, and a coordinate
			// outside the range it does.
			if inst, err := LoadInstance(data, map[string]float64{
				"wght": 1e9, "zzzz": -1e9,
			}); err == nil && inst != nil {
				useFace(inst)
			}
			checkClusters(t, face)
			checkSubset(t, face)
		}
	})
}

// checkSubset asserts that a font this package writes is a font it can read,
// and that the glyphs it kept are the glyphs the page will draw with.
//
// Subsetting is the last thing that happens to a font before it goes into a PDF
// and the first place a fault in it is invisible: the document opens, the page
// has text on it, and a letter is drawn at the wrong width or with the wrong
// outline. Nothing downstream can notice — the reader is given a font and
// believes it.
//
// Three things are asked, and the first is the one that makes the other two
// mean anything:
//
//   - The program parses. A subsetter that emits a table it cannot read back is
//     emitting one no reader can either.
//   - Every glyph the face used is in it. That is what the subset is *for*, and
//     a glyph left out is a letter missing from the page.
//   - Every kept glyph has the advance it had. The indices are retained on
//     purpose — see the note on Subset — and hmtx is kept at full length, so
//     this is a property the design promises rather than one it merely happens
//     to have. A width that moved is a line drawn to the wrong length with the
//     right letters in it.
//
// An error is a legitimate answer and is not one of the three: a standard font
// has no program to subset, and a font whose loca and glyf disagree is one
// Subset refuses rather than lies about.
//
// "Parses" is Load, except for one kind of subset Load cannot take: a
// renumbered one whose kept glyphs map no character, whose character map is
// empty because nothing true could go in it. That one is parsed by the sfnt
// and CFF readers instead, and held to the same glyphs and advances.
func checkSubset(t *testing.T, f *Face) {
	t.Helper()
	prog, kept, err := f.SubsetGlyphs()
	if err != nil || len(prog) == 0 {
		return
	}
	in := map[int]bool{}
	for _, gid := range kept {
		in[gid] = true
	}
	for _, gid := range f.Used() {
		if !in[gid] {
			t.Fatalf("glyph %d was used and is not in the subset", gid)
		}
	}
	sub, err := Load(prog)
	if err != nil && keptMapNoCharacter(f, kept) {
		// A renumbered subset whose glyphs map no character maps nothing,
		// and Load refuses a face that can set no text (see
		// unmappedsubset_test.go). It is still the program a document
		// embeds, and it is held to that instead.
		checkUnmappedSubset(t, f, prog, kept)
		return
	}
	if err != nil || sub == nil {
		t.Fatalf("the subset this package wrote cannot be read back: %v", err)
	}
	for _, gid := range kept {
		if got, want := sub.advanceGID(gid), f.advanceGID(gid); got != want {
			t.Fatalf("glyph %d advances %v in the subset and %v in the font",
				gid, got, want)
		}
	}
}

// keptMapNoCharacter reports whether no glyph of kept but .notdef has a
// character in f's own map, which is when a renumbered subset's map is empty.
func keptMapNoCharacter(f *Face, kept []int) bool {
	in := map[int]bool{}
	for _, gid := range kept {
		in[gid] = true
	}
	for _, gid := range f.Cmap() {
		if gid != 0 && in[gid] {
			return false
		}
	}
	return true
}

// checkUnmappedSubset holds a subset Load refuses for its empty character map
// to what the subset is for. Its map has to be the reason, read and empty; the
// program has to parse, with its CFF, as exactly the kept glyphs, renumbered
// in order; and each has to advance as it did in the face.
func checkUnmappedSubset(t *testing.T, f *Face, prog []byte, kept []int) {
	t.Helper()
	fp := font.ParseSFNT(prog, maxFontWork)
	if fp == nil {
		t.Fatal("the subset this package wrote is not an sfnt")
	}
	if len(fp.Cmap) != 0 || fp.CmapPartial {
		t.Fatalf("the subset maps %d characters and Load refuses it", len(fp.Cmap))
	}
	if fp.NumGlyphs != len(kept) {
		t.Fatalf("the subset holds %d glyphs and kept %d", fp.NumGlyphs, len(kept))
	}
	if cff := font.ParseCFFGlyphs(font.SFNTTables(prog)["CFF "], font.NewBudget(maxFontWork)); cff == nil || cff.NumGlyphs != len(kept) {
		t.Fatalf("the subset's CFF does not parse as its %d glyphs", len(kept))
	}
	for i, gid := range kept {
		if got, want := fp.WidthByGID[i], f.advanceGID(gid); got != want {
			t.Fatalf("glyph %d advances %v in the subset and %v in the font", gid, got, want)
		}
	}
}

// checkClusters asserts the contract the group arithmetic rests on: every glyph
// is charged to a byte of the run it came from.
//
// It is not a panic and it is not visible in a glyph. GroupAdvances builds the
// cumulative advance of a run by adding each glyph to its own cluster, and it
// guards the index — a glyph whose cluster is outside the text is skipped. That
// guard is right as a guard and wrong as an answer: what it produces is a run
// whose group width is *narrower* than the same run measured directly, so a line
// is filled to one width and painted at another with nothing in either call's
// output to show it. See MeasureShaped, which is written about that failure.
//
// Every run of every document goes through the group path when its box's text is
// cut into more than one run, so this is not a corner: it is the arithmetic a
// word split by a <span> is measured with.
//
// A hostile font is what makes it worth fuzzing. The clusters come from the
// shaper, which reorders for the syllabic scripts and merges for ligatures, and
// a font whose lookups say something no designer would say is exactly the input
// that would produce one out of range.
func checkClusters(t *testing.T, f *Face) {
	t.Helper()
	for _, text := range fuzzTexts {
		glyphs, _ := f.ShapeGlyphs(text)
		for i, g := range glyphs {
			if g.Cluster < 0 || g.Cluster >= len(text) {
				t.Fatalf("glyph %d of %q is charged to byte %d of %d",
					i, text, g.Cluster, len(text))
			}
			if !utf8.RuneStart(text[g.Cluster]) {
				t.Fatalf("glyph %d of %q is charged to byte %d, which is inside "+
					"a character", i, text, g.Cluster)
			}
		}
		// And the total, which is what the range above buys: a glyph the group
		// walk skipped is width the run loses.
		//
		// Within a tolerance rather than exactly, and the reason is arithmetic
		// rather than doubt: the two sums add the same numbers in different
		// orders — one per glyph, one per cluster and then prefixed — and
		// floating-point addition is not associative. The tolerance is far below
		// a layout unit at any size a document uses.
		cum := GroupAdvances(glyphs, len(text))
		_, through := GroupSpan(cum, 0, len(text), 1000)
		direct := MeasureGlyphs(glyphs, 1000)
		if d := through - direct; d > 1e-6 || d < -1e-6 {
			t.Fatalf("the group walk over %q makes it %v wide and the glyphs "+
				"measure %v", text, through, direct)
		}
	}
}

// useFace drives everything a caller can ask a face, which is what the fuzzer is
// for: a panic anywhere here is a panic in a process that was doing something
// else with a font somebody supplied.
//
// It is a list rather than a handful of calls, and
// TestTheFuzzTargetReachesEveryEntryPoint is what keeps it one: this used to
// measure, encode, shape and subset, and the four of them missed the whole of
// the context and merging API, the named features, the stack, and instancing —
// which is the reader most exposed to a crafted file, since it is the only one
// that walks the variation tables.
func useFace(face *Face) {
	_, _ = face.WithShapingLimits(context.Background(), RunLimits{}, func(f *Face) error { f.ShapeGlyphs("abc"); return nil })
	_, _ = face.ShapeGlyphsContext(context.Background(), RunInput{Text: "abc"}, RunLimits{})

	_ = face.Name()
	_ = face.Family()
	_ = face.Subfamily()
	_ = face.Axes()
	_ = face.IsVariable()
	_ = face.IsSimple()
	_ = face.IsCFF()
	_ = face.IsStandard()
	_ = face.UnitsPerEm()
	_ = face.NumGlyphs()
	_ = face.Descriptor()
	_ = face.Cmap()
	_ = face.Used()
	_ = face.Scripts()
	_ = face.Features()
	_ = face.HasScript("arab")
	_ = face.HasKerning()
	_ = face.HasLigatures()
	_ = face.HasJoiningForms()
	_, _, _, _ = face.CharacterCollection()
	_ = face.IsCIDKeyed()
	_, _ = face.EmbeddingPermissions()
	_ = face.Program()
	_ = face.GlyphAdvances()
	_ = face.StatesVerticalMetrics()
	_, _ = face.CentredVerticalOrigins()
	// Every instance's name is looked at: a match that accepts nothing walks
	// the whole of fvar and the name table.
	_, _ = face.NamedInstance(func(string) bool { return false })
	_, _ = face.NamedInstance(func(string) bool { return true })

	// Per glyph, past the end of the table as well: a count a font states and a
	// table that does not hold it is the shape of the bug this is looking for.
	for _, gid := range []int{-1, 0, 1, face.NumGlyphs(), face.NumGlyphs() + 1} {
		_ = face.GlyphAdvance(gid)
		_, _, _ = face.GlyphVerticalMetrics(gid)
		_ = face.GlyphCode(gid)
		_, _, _ = face.HalfWidthTrim(gid)
		_, _, _, _, _ = face.GlyphExtents(gid)
		_ = face.GlyphOutline(gid, func(Segment) bool { return true })
	}
	// And the ink of the first few dozen glyphs, which is read from whichever
	// table answers for each — a bitmap's strike, a colour glyph's paint, a
	// composite's components — and each of those is its own reader of the
	// file's offsets. The glyphs shaped below reach only the few the cmap
	// maps the sample text to.
	for gid := 2; gid < min(face.NumGlyphs(), 64); gid++ {
		_, _, _, _, _ = face.GlyphExtents(gid)
		// The outline of each, drawn from the same tables by the same walks,
		// and a stop after the first segment as well as a full one.
		_ = face.GlyphOutline(gid, func(Segment) bool { return true })
		_ = face.GlyphOutline(gid, func(Segment) bool { return false })
	}
	_, _, _ = face.ScriptOffsets()
	// The MATH table, where the font has one: every question, for glyphs in
	// range and out. See FuzzMathTable for a target on the table alone.
	if m, err := face.MathTable(); err == nil && m != nil {
		for c := MathConstant(0); c < MathConstantCount; c++ {
			_, _ = m.Constant(c)
		}
		_ = m.MinConnectorOverlap()
		for _, gid := range []int{-1, 0, 1, face.NumGlyphs(), face.NumGlyphs() + 1} {
			_, _ = m.ItalicsCorrection(gid)
			_, _ = m.TopAccentAttachment(gid)
			_ = m.IsExtendedShape(gid)
			_, _ = m.Kern(gid, MathKernTopRight, 0)
			for _, v := range []bool{false, true} {
				_ = m.Variants(gid, v)
				_, _ = m.Assembly(gid, v)
				_, _ = m.Stretch(gid, v, 5000)
			}
			_ = m.PreferredStretchWidth(gid)
		}
		_ = m.Limits()
	}
	for _, r := range []rune{'a', 0x0628, 0x0915, 0x1B13, 0x2011, 0x3000, 0x10FFFF} {
		_, _ = face.GlyphID(r)
		_, _ = face.Advance(r)
		_, _ = face.GlyphIDForTest(r)
		_ = face.StandsIn(r)
		_, _ = face.MirroredForm(r)
	}
	// The characters a font's 'rtlm' is for: a bracket with a mirror, and
	// operators with none.
	for _, r := range []rune{'(', 0x221A, 0x2211, 0x222B} {
		_, _ = face.MirroredForm(r)
	}

	clone := face.Clone()
	// And a copy loaded with settings of its own, turning on what nothing asks
	// for by default and off what is on, so that the plans built from the
	// font's bytes are asked with a second step in them.
	settled := face.WithFeatureSettings([]FeatureSetting{
		{Tag: "smcp", On: true}, {Tag: "liga"}, {Tag: "kern"}, {Tag: "zzzz", On: true},
	})
	_ = settled.FeatureSettings()
	stack := NewStack(face, clone, settled)
	_ = stack.Faces()

	// With a language, so that the language systems a font names are read as
	// well as its defaults.
	off := Features{NoOptionalLigatures: true, Language: "sr"}
	// Every text through the basic calls, because each is a different script
	// and a different model.
	for _, text := range fuzzTexts {
		_ = face.Measure(text, 10)
		_, _ = face.Encode(text)
		_, _, _ = face.InkExtent(text, 10)
		_ = face.MeasureShaped(text, 10)
		glyphs, _ := face.ShapeGlyphs(text)
		_ = MeasureGlyphs(glyphs, 10)
		runs, _ := stack.ShapeRuns(text)
		_ = MeasureRuns(runs, 10)
	}

	// And one of them through the rest, which is about reaching each entry point
	// rather than about the text: the whole list through all of these is ten
	// times the work for the same coverage, and a fuzzer's budget is executions.
	text := fuzzTexts[len(fuzzTexts)-1]
	before, after := fuzzTexts[0], fuzzTexts[1]
	_ = face.MeasureShapedInContext(text, 10, before, after, true, off)
	_ = face.MeasureShapedMerged(text, 10, before, after, before, after, false, off)
	_, _ = face.MeasureShapedMergedSpan(text, 10, before, after, before, after, true, off)
	_, _ = face.ShapeGlyphsWith(text, "smcp", "zzzz")
	_, _ = face.ShapeGlyphsInContext(text, before, after, off)
	_, _ = face.ShapeGlyphsAcrossFaces(text, before, after, off)
	_, _ = face.ShapeGlyphsMerged(text, before, after, before, after, false, off)
	_, _ = face.ShapeGlyphsInContextOrAcross(text, before, after, true, off)
	_ = face.ContextCanChange(text, off)
	_ = face.FormsFollowNeighbours(text, off)
	_, _ = settled.ShapeGlyphsInContext(text, before, after, off)
	_ = settled.ContextCanChange(text, off)

	whole := face.ShapeGroup(text, before, after, true, off)
	_, _ = GroupContext(before, after, before, after)
	cum := GroupAdvances(whole, len(text))
	_, _ = GroupSpan(cum, 0, len(cum), 10)
	for _, r := range text {
		_ = stack.Covers(r)
	}

	_, _ = face.Subset()
	_, _, _ = face.SubsetGlyphs()
	// Glyphs drawn by index, as a formula's size variants are, which no
	// character reached: the last the face has and two it does not, on a
	// record of their own, so the subset is asked for a glyph only Use named.
	clone.Use(-1, face.NumGlyphs()-1, face.NumGlyphs())
	_, _, _ = clone.SubsetGlyphs()
	// Last, so that it reads every layout the shaping above caused to be read.
	_ = face.LayoutLimits()
}

// TestAFontDeclaringNoGlyphsIsRefused pins the crash the fuzzer found, and the
// same crash in the CFF subsetter, which issue #863 found.
//
// maxp holds the glyph count, and a font can declare zero. It can also carry no
// maxp at all, which the reader takes as zero: the font in #863 had its tag
// mangled to "maxi". Every sfnt has .notdef at index zero, so that is malformed
// rather than empty — but the subsetter believed it, and writing .notdef into a
// slice sized from the count indexed past the end of nothing. The guard was
// written in the glyf path only, and a CFF face sized its own keep set from the
// same count.
//
// So every kind of face a program can become is here, each broken both ways:
// the outline formats, the containers that unwrap to them, the simple
// embedding and the two kinds of instance. A CFF2 face and an instance never
// reach Subset declaring none — the CFF2 reader refuses a table whose
// charstrings the count does not match, and the instancer a font without
// glyphs — and the cases say so rather than skip, so that a reader which
// starts accepting them is noticed here. The fuzz corpus is not committed, so
// the cases are stated here instead.
func TestAFontDeclaringNoGlyphsIsRefused(t *testing.T) {
	var latin []fonttest.Glyph
	for r := 'A'; r <= 'z'; r++ {
		latin = append(latin, fonttest.Glyph{Rune: r, Advance: 500, HasShape: true})
	}
	glyf := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: aGlyph})
	cff := fonttest.OTTO(fonttest.CFF(fonttest.CFFOptions{Glyphs: 2}), fonttest.SFNTOptions{Glyphs: aGlyph})
	cff2 := cff2Sfnt(twoGlyphs([]byte{139, 139, 21}).bytes(), aGlyph)
	cff2Tables := font.SFNTTables(cff2)
	cff2Tables["fvar"] = fonttest.FVAR(wghtWdth, nil)
	cff2Variable := assembleOTTO(cff2Tables)
	instance := func(data []byte) (*Face, error) {
		return LoadInstance(data, map[string]float64{"wght": 700})
	}

	type kind struct {
		name string
		data func(t *testing.T) []byte
		wrap func([]byte) []byte
		load func([]byte) (*Face, error)
		// refused is a face Load turns away once it declares no glyphs.
		refused bool
	}
	fixed := func(b []byte) func(*testing.T) []byte { return func(*testing.T) []byte { return b } }
	kinds := []kind{
		{name: "glyf", data: fixed(glyf), load: Load},
		{name: "glyf simple", data: fixed(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: latin})), load: LoadSimple},
		{name: "CFF", data: fixed(cff), load: Load},
		{name: "WOFF glyf", data: fixed(glyf), wrap: asWOFF, load: Load},
		{name: "WOFF CFF", data: fixed(cff), wrap: asWOFF, load: Load},
		{name: "WOFF2 CFF", data: fixed(cff), wrap: asWOFF2, load: Load},
		{name: "CFF2", data: fixed(cff2), load: Load, refused: true},
		{name: "glyf instance", data: fixed(varyingVariableFont(t, nil)), load: instance, refused: true},
		{name: "CFF2 instance", data: fixed(cff2Variable), load: instance, refused: true},
		// fonttest's CID-keyed CFF has no FDSelect, which a subset needs, so
		// the CID-keyed case is a real one.
		{name: "CID-keyed CFF", data: func(t *testing.T) []byte {
			return fonttest.CJKFile(t, "NotoSansJP-Regular.otf")
		}, load: Load},
	}
	for _, k := range kinds {
		for _, broken := range []struct {
			name  string
			apply func(*testing.T, []byte) []byte
		}{
			{"maxp declaring 0", func(t *testing.T, b []byte) []byte { return corruptMaxpGlyphCount(t, b, 0) }},
			{"no maxp", renameMaxp},
		} {
			t.Run(k.name+"/"+broken.name, func(t *testing.T) {
				wrap := func(b []byte) []byte { return b }
				if k.wrap != nil {
					wrap = k.wrap
				}
				data := k.data(t)
				if _, err := k.load(wrap(data)); err != nil {
					t.Fatalf("the fixture does not load before it is broken: %v", err)
				}
				noPanic(t, "Load and Subset", func() {
					face, err := k.load(wrap(broken.apply(t, data)))
					if k.refused {
						if err == nil {
							t.Error("the face was loaded, and this test assumed it never could be: " +
								"check that its subsetter is guarded, then move it to the others")
						}
						return
					}
					if err != nil {
						t.Fatalf("the face was refused at load, and this test assumed it would not be: %v", err)
					}
					face.Use(0, 1, 2)
					if _, err := face.Subset(); err == nil {
						t.Error("Subset wrote a font declaring no glyphs rather than refusing it")
					}
					if _, _, err := face.SubsetGlyphs(); err == nil {
						t.Error("SubsetGlyphs wrote a font declaring no glyphs rather than refusing it")
					}
				})
			})
		}
	}
}

// renameMaxp gives a font's maxp record another tag, so the font carries no
// maxp at all, as the font in #863 did.
func renameMaxp(t *testing.T, data []byte) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	for i := 0; i < int(be16(out, 4)); i++ {
		rec := 12 + 16*i
		if rec+16 <= len(out) && string(out[rec:rec+4]) == "maxp" {
			copy(out[rec:], "maxi")
			return out
		}
	}
	t.Fatal("the fixture has no maxp table to rename")
	return nil
}

// asWOFF and asWOFF2 wrap an sfnt, table for table, in each container.
func asWOFF(sfnt []byte) []byte {
	var tables []fonttest.WOFFTable
	for _, tag := range sortedTags(sfnt) {
		tables = append(tables, fonttest.WOFFTable{Tag: tag, Data: font.SFNTTables(sfnt)[tag]})
	}
	return fonttest.WOFF(fonttest.WOFFOptions{Flavor: binary.BigEndian.Uint32(sfnt), Tables: tables})
}

func asWOFF2(sfnt []byte) []byte {
	var tables []fonttest.WOFF2Table
	for _, tag := range sortedTags(sfnt) {
		tables = append(tables, fonttest.WOFF2Table{Tag: tag, Data: font.SFNTTables(sfnt)[tag]})
	}
	return fonttest.WOFF2(fonttest.WOFF2Options{Flavor: binary.BigEndian.Uint32(sfnt), Tables: tables, SpellOutTags: true})
}

func sortedTags(sfnt []byte) []string {
	tags := slices.Collect(maps.Keys(font.SFNTTables(sfnt)))
	slices.Sort(tags)
	return tags
}

// corruptMaxpGlyphCount rewrites the glyph count in a font's maxp table, which
// is the one number the subsetter sizes everything from.
func corruptMaxpGlyphCount(t *testing.T, data []byte, count uint16) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	numTables := int(be16(out, 4))
	for i := 0; i < numTables; i++ {
		rec := 12 + 16*i
		if rec+16 > len(out) {
			break
		}
		if string(out[rec:rec+4]) != "maxp" {
			continue
		}
		off := int(be32(out, rec+8))
		if off+6 > len(out) {
			break
		}
		// version (4 bytes), then numGlyphs.
		out[off+4] = byte(count >> 8)
		out[off+5] = byte(count)
		return out
	}
	t.Fatal("the fixture has no maxp table to corrupt")
	return nil
}

func be16(b []byte, i int) uint16 { return uint16(b[i])<<8 | uint16(b[i+1]) }

// entryPointPattern matches the exported entry points of this package: a
// function, or a method on a face.
var entryPointPattern = regexp.MustCompile(`(?m)^func (?:\(f \*Face\) |\(s \*Stack\) )?([A-Z]\w*)\(`)

// notAboutFontBytes are the exported names the fuzz target does not have to
// reach, each with the reason it does not.
//
// The list is short on purpose. Everything that reads what a font says has to be
// on the other side of it: the reader is what an attacker's bytes reach, and a
// reader nothing drives is a reader nothing has ever tried to break.
var notAboutFontBytes = map[string]string{
	"Load":               "named rather than called: the target's own loader loop takes it as a value",
	"LoadSimple":         "the same",
	"Standard":           "one of the built-in faces, which are not read from bytes a caller supplied",
	"StandardNames":      "the names of those, which are a constant",
	"InCursiveScript":    "a property of a character, and no font is consulted",
	"DrawsNothing":       "the same",
	"CombiningClass":     "the same",
	"ComposeCanonically": "the same",
	"PrivateDictForTest": "reached through Load in the CFF tests, which fuzz those bytes themselves",
	"CharStringsForTest": "the same",
}

// TestTheFuzzTargetReachesEveryEntryPoint is what keeps useFace a list.
//
// FuzzLoadAndUse measured, encoded, shaped and subsetted, and its own comment
// said it drove the whole pipeline. The four of them missed the context and
// merging API, the named features, the stack, the group measurements and
// instancing — and instancing is the reader most exposed to a crafted file,
// since it is the only one that walks the variation tables. A gap like that is
// invisible: the fuzzer runs, finds nothing, and says nothing about what it
// never called.
//
// So the entry points are read off the package rather than remembered, and an
// exported one that is neither driven nor excused fails this. Adding a method is
// then a decision about whether hostile bytes can reach it.
func TestTheFuzzTargetReachesEveryEntryPoint(t *testing.T) {
	body, err := os.ReadFile("panic_test.go")
	if err != nil {
		t.Fatal(err)
	}
	names, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, name := range names {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range entryPointPattern.FindAllStringSubmatch(string(src), -1) {
			entry := m[1]
			found++
			if why, excused := notAboutFontBytes[entry]; excused {
				if why == "" {
					t.Errorf("%s is excused with no reason", entry)
				}
				continue
			}
			// A whole word: "FuzzLoadAndUse(" is not a call of Use.
			if !regexp.MustCompile(`\b` + entry + `\(`).Match(body) {
				t.Errorf("%s (%s) is exported and the fuzz target never calls it; "+
					"drive it in useFace, or say in notAboutFontBytes why bytes a "+
					"caller supplied cannot reach it", entry, name)
			}
		}
	}
	// The pattern is what everything above rests on, so it has to have matched
	// something like the number of entry points this package has.
	if found < 40 {
		t.Errorf("only %d entry points were found in the package; the pattern "+
			"that reads them off is not matching what it should", found)
	}
}
