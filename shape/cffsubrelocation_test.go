package shape

import (
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// A CFF's local subroutines are named from inside the Private DICT that owns
// them, as a distance from that DICT's start — so the two have to move as a
// unit or the operand names the distance the INDEX used to be at.
//
// Nothing in the format says they are adjacent. The subsetter copied the DICT
// and the INDEX out separately and wrote them back to back, which closes any
// gap between them and leaves every charstring's callsubr reading whatever now
// sits at the old distance.

// cffFaceWithSubrs builds an OpenType/CFF face whose Private DICT names local
// subroutines with gap bytes of padding in front of them.
func cffFaceWithSubrs(t *testing.T, subrs, gap int) *Face {
	t.Helper()
	glyphs := []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
		{Rune: 'c', Advance: 500, HasShape: true},
	}
	cff := fonttest.CFF(fonttest.CFFOptions{
		Glyphs: len(glyphs) + 1, LocalSubrs: subrs, LocalSubrsGap: gap,
	})
	f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading a CFF with %d local subrs and a %d-byte gap: %v",
			subrs, gap, err)
	}
	return f
}

// subrsOfSubset reads back where a subset's Private DICT says its local
// subroutines are, and what is there.
func subrsOfSubset(t *testing.T, f *Face) (offset int, items [][]byte) {
	t.Helper()
	data, err := f.Subset()
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	cff := font.SFNTTables(data)["CFF "]
	if cff == nil {
		t.Fatal("the subset has no CFF table")
	}
	priv, err := PrivateDictForTest(cff)
	if err != nil {
		t.Fatalf("reading the subset's Private DICT: %v", err)
	}
	ops, _, err := parseCFFDict(priv)
	if err != nil {
		t.Fatalf("parsing the subset's Private DICT: %v", err)
	}
	for _, e := range ops {
		if e.op == opSubrs && len(e.operands) == 1 {
			offset = e.operands[0]
		}
	}
	if offset == 0 {
		t.Fatal("the subset's Private DICT names no local subroutines")
	}
	// Where the DICT sits in the table, so the operand can be resolved.
	top, err := topDictOf(cff)
	if err != nil {
		t.Fatalf("reading the subset's Top DICT: %v", err)
	}
	privAt := 0
	for _, e := range top {
		if e.op == opPrivate && len(e.operands) == 2 {
			privAt = e.operands[1]
		}
	}
	idx, err := readCFFIndex(cff, privAt+offset)
	if err != nil {
		t.Fatalf("the subset's local subroutine INDEX is not readable at the "+
			"offset its Private DICT names (%d + %d): %v", privAt, offset, err)
	}
	return offset, idx.items
}

// TestASubsetKeepsItsLocalSubroutinesWhereTheDictSaysTheyAre.
func TestASubsetKeepsItsLocalSubroutinesWhereTheDictSaysTheyAre(t *testing.T) {
	for _, gap := range []int{0, 1, 7, 64} {
		f := cffFaceWithSubrs(t, 3, gap)
		if _, missing := f.Encode("ab"); missing != 0 {
			t.Fatalf("%d runes of the fixture are missing", missing)
		}
		_, items := subrsOfSubset(t, f)
		if len(items) != 3 {
			t.Errorf("a %d-byte gap: the subset's local subroutine INDEX holds %d "+
				"entries, want 3 — the operand names something else", gap, len(items))
			continue
		}
		for i, it := range items {
			if len(it) != 1 || it[0] != 11 {
				t.Errorf("a %d-byte gap: subroutine %d is %v, want a single return",
					gap, i, it)
			}
		}
	}
}

// TestASubsetWithNoLocalSubroutinesIsUnchangedByThis is the control: the common
// font names none, and nothing above may make its Private DICT longer or its
// subset larger.
func TestASubsetWithNoLocalSubroutinesIsUnchangedByThis(t *testing.T) {
	f := cffFaceWithSubrs(t, 0, 0)
	if _, missing := f.Encode("ab"); missing != 0 {
		t.Fatalf("%d runes of the fixture are missing", missing)
	}
	data, err := f.Subset()
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}
	cff := font.SFNTTables(data)["CFF "]
	priv, err := PrivateDictForTest(cff)
	if err != nil {
		t.Fatalf("reading the Private DICT: %v", err)
	}
	ops, _, err := parseCFFDict(priv)
	if err != nil {
		t.Fatalf("parsing the Private DICT: %v", err)
	}
	for _, e := range ops {
		if e.op == opSubrs {
			t.Errorf("a font with no local subroutines came out of the subsetter "+
				"naming some at %v", e.operands)
		}
	}
}
