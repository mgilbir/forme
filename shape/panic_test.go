package shape

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

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
		"NotoSansBalinese.ttf", // 338 contextual positioning subtables
		"NotoSansJavanese.ttf", // the universal engine's features
		"NotoSansArabic.ttf",   // cursive joining and mark filtering
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
		}
	})
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
	_ = face.Name()
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
	_ = face.Language()
	face.SetLanguage("sr")
	_ = face.Features()
	_ = face.HasScript("arab")
	_ = face.HasKerning()
	_ = face.HasLigatures()
	_ = face.HasJoiningForms()
	_, _, _, _ = face.CharacterCollection()
	_ = face.GlyphAdvances()

	// Per glyph, past the end of the table as well: a count a font states and a
	// table that does not hold it is the shape of the bug this is looking for.
	for _, gid := range []int{-1, 0, 1, face.NumGlyphs(), face.NumGlyphs() + 1} {
		_ = face.GlyphAdvance(gid)
		_ = face.GlyphCode(gid)
	}
	for _, r := range []rune{'a', 0x0628, 0x0915, 0x1B13, 0x10FFFF} {
		_, _ = face.GlyphID(r)
		_, _ = face.Advance(r)
		_, _ = face.GlyphIDForTest(r)
	}

	clone := face.Clone()
	stack := NewStack(face, clone)
	_ = stack.Faces()

	off := Features{NoOptionalLigatures: true}
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

	whole := face.ShapeGroup(text, before, after, true, off)
	_, _ = GroupContext(before, after, before, after)
	cum := GroupAdvances(whole, len(text))
	_, _ = GroupSpan(cum, 0, len(cum), 10)
	for _, r := range text {
		_ = stack.Covers(r)
	}

	_, _ = face.Subset()
	_, _, _ = face.SubsetGlyphs()
}

// TestAFontDeclaringNoGlyphsIsRefused pins the crash the fuzzer found.
//
// maxp holds the glyph count, and a font can declare zero. Every sfnt has
// .notdef at index zero, so that is malformed rather than empty — but the
// subsetter believed it, and writing .notdef into a slice sized from the count
// indexed past the end of nothing. The fuzz corpus is not committed, so the
// case is stated here instead.
func TestAFontDeclaringNoGlyphsIsRefused(t *testing.T) {
	good := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
	})
	broken := corruptMaxpGlyphCount(t, good, 0)

	face, err := Load(broken)
	if err != nil {
		t.Skipf("the reader rejected it first, which is also fine: %v", err)
	}
	noPanic(t, "Subset", func() {
		if _, err := face.Subset(); err == nil {
			t.Error("a font declaring no glyphs was subsetted rather than refused")
		}
	})
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
			if !strings.Contains(string(body), entry+"(") {
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
