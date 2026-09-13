package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The legacy 'kern' table: kerning as a font written before GPOS states it.
//
// readKernTable is only reached when GPOS carried no pairs, which is why no
// corpus font reaches it — every face in testdata/harfbuzz and every face the
// reftests load states its kerning in GPOS, and a face with both is read through
// GPOS by design. Coverage bore that out: readKernTable ran on the empty tables
// of fonts that have none and kernFormat0 had never been entered at all.
//
// That is the shape of fault worth a fixture rather than a deletion. The reader
// is not dead — a real face with a 'kern' table and no GPOS reaches it, and the
// format is what an old Type 1-derived TrueType font uses — it is only that
// nothing in the corpora is such a font. So the font is built here.
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

// firstAdvanceOf shapes a two-glyph string and returns the advance of the first
// glyph, which is the one a 'kern' record adjusts.
func firstAdvanceOf(t *testing.T, f *Face, s string) float64 {
	t.Helper()
	glyphs, missing := f.ShapeGlyphs(s)
	if missing != 0 {
		t.Fatalf("shaping %q: %d characters have no glyph", s, missing)
	}
	if len(glyphs) != 2 {
		t.Fatalf("shaping %q gave %d glyphs, want 2", s, len(glyphs))
	}
	return glyphs[0].XAdvance
}

func TestALegacyKernTableIsApplied(t *testing.T) {
	f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
		Coverage: fonttest.KernHorizontal,
		Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
	}}), nil)

	if got, want := firstAdvanceOf(t, f, "AV"), float64(lkAdvance+lkTighten); got != want {
		t.Errorf("AV set the A %v wide; the face's only kerning is a legacy "+
			"'kern' table closing the pair by %d, so the A should be %v",
			got, -lkTighten, want)
	}
	// The pair and nothing else: a record is a pair, not a glyph.
	if got, want := firstAdvanceOf(t, f, "VA"), float64(lkAdvance); got != want {
		t.Errorf("VA set the V %v wide; the table states A-V and not V-A, so "+
			"the V should be its own %v", got, want)
	}
	if got, want := firstAdvanceOf(t, f, "AA"), float64(lkAdvance); got != want {
		t.Errorf("AA set the first A %v wide; the table states A-V and not A-A, "+
			"so it should be its own %v", got, want)
	}
}

// The subtables the reader is documented to skip, each one stated as a table
// that would kern if it were taken.
//
// Every one of these describes positioning this package does not apply, and the
// failure they guard against is not a missing feature but a wrong number: a
// cross-stream value is a vertical movement, and a minimum is a floor rather
// than an adjustment, so reading either as an advance adjustment would close a
// gap the font never asked to close.
func TestOnlyPlainHorizontalKerningIsTaken(t *testing.T) {
	for _, c := range []struct {
		name     string
		coverage int
	}{
		{"vertical", 0x0000},              // bit 0 clear
		{"minimum values", 0x0003},        // bit 1
		{"cross-stream", 0x0005},          // bit 2
		{"override", 0x0009},              // bit 3
		{"format 1", 0x0101},              // high byte
		{"format 2", 0x0201},              //
		{"cross-stream override", 0x000D}, // bits 2 and 3
	} {
		t.Run(c.name, func(t *testing.T) {
			f := legacyKernFace(t, fonttest.LegacyKern([]fonttest.KernSubtable{{
				Coverage: c.coverage,
				Pairs:    []fonttest.KernPair{{Left: lkA, Right: lkV, Adjust: lkTighten}},
			}}), nil)
			if got, want := firstAdvanceOf(t, f, "AV"), float64(lkAdvance); got != want {
				t.Errorf("a %s subtable moved the A to %v; it describes "+
					"positioning this package does not apply, so it should be "+
					"skipped and the A left at %v", c.name, got, want)
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
		{Coverage: 0x0005, Pairs: []fonttest.KernPair{ // cross-stream, skipped
			{Left: lkA, Right: lkV, Adjust: 900},
			{Left: lkV, Right: lkA, Adjust: 900},
		}},
		{Coverage: fonttest.KernHorizontal, Pairs: []fonttest.KernPair{
			{Left: lkA, Right: lkV, Adjust: lkTighten},
		}},
	}), nil)

	if got, want := firstAdvanceOf(t, f, "AV"), float64(lkAdvance+lkTighten); got != want {
		t.Errorf("with a skipped subtable in front of it the A came out %v; "+
			"the second subtable states the pair and should still be found, "+
			"putting the A at %v", got, want)
	}
}

// GPOS wins, and the legacy table is not read at all.
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

	got := firstAdvanceOf(t, f, "AV")
	switch {
	case got == float64(lkAdvance+gposTighten):
		// What is wanted: GPOS alone.
	case got == float64(lkAdvance+lkTighten):
		t.Errorf("AV set the A %v wide, which is the legacy table's number; "+
			"a font stating both should be read through GPOS, giving %v",
			got, lkAdvance+gposTighten)
	case got == float64(lkAdvance+gposTighten+lkTighten):
		t.Errorf("AV set the A %v wide, which is both numbers; the font is "+
			"being kerned twice and GPOS alone should give %v",
			got, lkAdvance+gposTighten)
	default:
		t.Errorf("AV set the A %v wide; GPOS states %d, so it should be %v",
			got, gposTighten, lkAdvance+gposTighten)
	}
}
