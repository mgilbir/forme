package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The legacy 'kern' table: kerning as a font written before GPOS states it.
//
// No face in testdata/harfbuzz and none the reftests load has one, so the font
// is built here. What each fixture asserts is what HarfBuzz 14.5.0 answers for
// it, and the Google Fonts sweep is where the rule was measured: 756 Latin
// strings in faces with only a kern table were set differently before the
// table was applied as HarfBuzz applies it (legacykern.go).
const (
	lkAdvance = 500
	lkTighten = -150
)

// lkA and lkV are the glyphs of the pair; glyph 0 is .notdef, so the first
// supplied glyph is 1.
const (
	lkA = 1
	lkV = 2
)

// legacyKernFace builds a face with the given 'kern' table and, optionally, a
// GPOS stating pairs of its own.
func legacyKernFace(t *testing.T, kern []byte, gpos []byte) *Face {
	t.Helper()
	extra := map[string][]byte{}
	if kern != nil {
		extra["kern"] = kern
	}
	if gpos != nil {
		extra["GPOS"] = gpos
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "LegacyKern",
		Glyphs: []fonttest.Glyph{
			{Rune: 'A', Advance: lkAdvance, HasShape: true},
			{Rune: 'V', Advance: lkAdvance, HasShape: true},
		},
		Extra: extra,
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// shapedUnits shapes a string and returns each glyph's advance and offsets in
// the face's units, which the fixtures here are stated in.
func shapedUnits(t *testing.T, f *Face, s string) [][3]float64 {
	t.Helper()
	glyphs, missing := f.ShapeGlyphs(s)
	if missing != 0 {
		t.Fatalf("shaping %q: %d characters have no glyph", s, missing)
	}
	var out [][3]float64
	for _, g := range glyphs {
		out = append(out, [3]float64{g.XAdvance, g.XOffset, g.YOffset})
	}
	return out
}

func sameUnits(a, b [][3]float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestALegacyKernTableIsApplied is the table applied as HarfBuzz applies it:
// half the number on the first glyph's advance and the rest on the second's,
// with the second drawn back by that rest, so the pair ends where the whole
// number would have put it and each glyph carries half. The fixture's answers
// are HarfBuzz 14.5.0's for this font, shaped through uharfbuzz.
func TestALegacyKernTableIsApplied(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)

	half := float64(lkTighten / 2)
	if got, want := shapedUnits(t, f, "AV"), [][3]float64{
		{lkAdvance + half, 0, 0}, {lkAdvance + half, half, 0}}; !sameUnits(got, want) {
		t.Errorf("AV is %v; the face's only kerning is a legacy 'kern' table closing "+
			"the pair by %d, which HarfBuzz sets as %v", got, -lkTighten, want)
	}
	// The pair and nothing else: a record is a pair, not a glyph.
	plain := [][3]float64{{lkAdvance, 0, 0}, {lkAdvance, 0, 0}}
	if got := shapedUnits(t, f, "VA"); !sameUnits(got, plain) {
		t.Errorf("VA is %v; the table states A-V and not V-A", got)
	}
	if got := shapedUnits(t, f, "AA"); !sameUnits(got, plain) {
		t.Errorf("AA is %v; the table states A-V and not A-A", got)
	}
}

// TestAnOddKernIsSplitAsHarfBuzzSplitsIt: the first glyph's half is the number
// shifted right, which rounds a negative odd number down, and the second takes
// what is left.
func TestAnOddKernIsSplitAsHarfBuzzSplitsIt(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: -151}},
	}}), nil)
	if got, want := shapedUnits(t, f, "AV"), [][3]float64{
		{lkAdvance - 76, 0, 0}, {lkAdvance - 75, -75, 0}}; !sameUnits(got, want) {
		t.Errorf("AV kerned by -151 is %v, want %v", got, want)
	}
}

// Which subtables are applied: every format 0 subtable that kerns along the
// line, whatever it says about minimums and overrides — HarfBuzz reads neither
// bit — and one that kerns across it, as a movement across. A vertical one is
// not, in a horizontal run. The answers are HarfBuzz 14.5.0's.
func TestWhichLegacyKernSubtablesAreApplied(t *testing.T) {
	half := float64(lkTighten / 2)
	kerned := [][3]float64{{lkAdvance + half, 0, 0}, {lkAdvance + half, half, 0}}
	plain := [][3]float64{{lkAdvance, 0, 0}, {lkAdvance, 0, 0}}
	across := [][3]float64{{lkAdvance, 0, 0}, {lkAdvance, 0, lkTighten}}
	for _, c := range []struct {
		name     string
		coverage int
		want     [][3]float64
	}{
		{"vertical", 0x0000, plain},               // bit 0 clear
		{"minimum values", 0x0003, kerned},        // bit 1
		{"cross-stream", 0x0005, across},          // bit 2
		{"override", 0x0009, kerned},              // bit 3
		{"cross-stream override", 0x000D, across}, // bits 2 and 3
	} {
		t.Run(c.name, func(t *testing.T) {
			f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
				Coverage: c.coverage,
				Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
			}}), nil)
			if got := shapedUnits(t, f, "AV"); !sameUnits(got, c.want) {
				t.Errorf("a %s subtable sets AV as %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// TestAKernAcrossTheLineCarriesWhatFollows: HarfBuzz ties the whole run into a
// chain for a subtable that kerns across the line, so a glyph raised or
// lowered takes every glyph after it along.
func TestAKernAcrossTheLineCarriesWhatFollows(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: 0x0005,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)
	if got, want := shapedUnits(t, f, "AVA"), [][3]float64{
		{lkAdvance, 0, 0}, {lkAdvance, 0, lkTighten}, {lkAdvance, 0, lkTighten}}; !sameUnits(got, want) {
		t.Errorf("AVA across the line is %v, want %v", got, want)
	}
}

// The formats this package does not read are passed over, and a subtable
// after one is still reached. That is this package's limitation and not
// HarfBuzz's answer — HarfBuzz reads format 2's classes and format 1's state
// machine — so what is asserted is only that the pairs of a format 0 reading
// are not taken out of bytes that are something else.
func TestAKernFormatThisDoesNotReadIsPassedOver(t *testing.T) {
	for _, c := range []struct {
		name     string
		coverage int
	}{{"format 1", 0x0101}, {"format 2", 0x0201}} {
		t.Run(c.name, func(t *testing.T) {
			f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
				Coverage: c.coverage,
				Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
			}}), nil)
			if got := shapedUnits(t, f, "AV"); got[0][0] != lkAdvance {
				t.Errorf("a %s subtable was read as pairs: %v", c.name, got)
			}
		})
	}
}

// A skipped subtable must not cost the one behind it.
//
// The reader steps to the next subtable by the length in the header it has just
// rejected, so a rejection is exactly where stepping can go wrong — and a reader
// that stopped at the first subtable it did not want, or stepped by the body
// rather than the whole subtable, would pass every fixture above.
func TestAKernSubtableAfterASkippedOneIsStillRead(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{
		{Coverage: 0x0000, Pairs: []fonttest.KernPair{ // vertical, skipped
			{Left: lkA, Right: lkV, Adjust: 900},
			{Left: lkV, Right: lkA, Adjust: 900},
		}},
		{Coverage: fonttest.KernHorizontal, Pairs: []fonttest.KernPair{
			{Left: lkA, Right: lkV, Adjust: lkTighten},
		}},
	}), nil)

	half := float64(lkTighten / 2)
	if got, want := shapedUnits(t, f, "AV"), [][3]float64{
		{lkAdvance + half, 0, 0}, {lkAdvance + half, half, 0}}; !sameUnits(got, want) {
		t.Errorf("with a skipped subtable in front of it AV is %v; the second "+
			"subtable states the pair and should still be found, giving %v", got, want)
	}
}

// GPOS wins where it offers 'kern', and the legacy table is not read at all.
//
// A font with both is a font being migrated, and its 'kern' table is the older
// statement. Taking both would apply the kerning twice.
func TestGPOSIsPreferredToTheLegacyKernTable(t *testing.T) {
	const gposTighten = -40
	f := legacyKernFace(t,
		fonttest.LegacyKern([]fonttest.KernSubtable{{
			Coverage: fonttest.KernHorizontal,
			Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
		}}),
		fonttest.GPOS([]fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: gposTighten}}))

	if got, want := shapedUnits(t, f, "AV"), [][3]float64{
		{lkAdvance + gposTighten, 0, 0}, {lkAdvance, 0, 0}}; !sameUnits(got, want) {
		t.Errorf("AV is %v; a font stating both should be kerned by GPOS alone, giving %v",
			got, want)
	}
}

// TestTheLegacyTableKernsBesideAGPOSWithNoKerning: what decides is whether
// GPOS offers the run 'kern', not whether there is a GPOS. A face whose GPOS
// only attaches marks is kerned by its kern table, as HarfBuzz kerns it.
func TestTheLegacyTableKernsBesideAGPOSWithNoKerning(t *testing.T) {
	f := legacyKernFace(t,
		fonttest.LegacyKern([]fonttest.KernSubtable{{
			Coverage: fonttest.KernHorizontal,
			Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
		}}),
		fonttest.GPOSPairsUnder("mark", []fonttest.KernPair{{Left: lkV, Right: lkA, Adjust: -10}}))

	half := float64(lkTighten / 2)
	if got, want := shapedUnits(t, f, "AV"), [][3]float64{
		{lkAdvance + half, 0, 0}, {lkAdvance + half, half, 0}}; !sameUnits(got, want) {
		t.Errorf("AV is %v; GPOS states no 'kern', so the legacy table should give %v",
			got, want)
	}
}

// TestFontKerningNoneTurnsTheLegacyTableOff: it is kerning, and a document
// that turned kerning off has turned it off.
func TestFontKerningNoneTurnsTheLegacyTableOff(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)
	glyphs, _ := f.ShapeGlyphsInContext("AV", "", "", Features{NoKerning: true})
	if len(glyphs) != 2 || glyphs[0].XAdvance != lkAdvance || glyphs[1].XOffset != 0 {
		t.Errorf("with kerning off AV is %+v", glyphs)
	}
}
