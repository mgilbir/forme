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
	return cffFaceWith(t, fonttest.CFFOptions{LocalSubrs: subrs, LocalSubrsGap: gap})
}

// cffFaceWith builds an OpenType/CFF face of three glyphs, a, b and c, from
// opts, which is given the glyph count.
func cffFaceWith(t *testing.T, opts fonttest.CFFOptions) *Face {
	t.Helper()
	glyphs := []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
		{Rune: 'c', Advance: 500, HasShape: true},
	}
	opts.Glyphs = len(glyphs) + 1
	f, err := Load(fonttest.OTTO(fonttest.CFF(opts), fonttest.SFNTOptions{Glyphs: glyphs}))
	if err != nil {
		t.Fatalf("loading a CFF with %d local subrs and a %d-byte gap: %v",
			opts.LocalSubrs, opts.LocalSubrsGap, err)
	}
	return f
}

// cffFaceCallingSubrs is cffFaceWithSubrs with three local subroutines that a
// and b call between them, so that a subset keeping a and b keeps all three.
//
// A subset carries only the subroutines its glyphs call, so a face whose
// glyphs call none has none to relocate. whole makes b also do arithmetic,
// which the subsetter's walk cannot follow; the subroutines are then copied
// whole, gap and all, rather than rebuilt behind the DICT — and the operand
// has to be right on both roads.
func cffFaceCallingSubrs(t *testing.T, gap int, whole bool) *Face {
	t.Helper()
	const callsubr, endchar, add = 10, 14, 10
	a := []byte{32, callsubr, 33, callsubr, endchar} // subroutines 0 and 1: -107 and -106
	b := []byte{34, callsubr, endchar}               // subroutine 2
	if whole {
		b = []byte{34, callsubr, 139, 139, 12, add, endchar}
	}
	return cffFaceWith(t, fonttest.CFFOptions{
		LocalSubrs: 3, LocalSubrsGap: gap,
		Charstrings: [][]byte{{endchar}, a, b, {endchar}},
	})
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
	for _, whole := range []bool{false, true} {
		for _, gap := range []int{0, 1, 7, 64} {
			f := cffFaceCallingSubrs(t, gap, whole)
			if _, missing := f.Encode("ab"); missing != 0 {
				t.Fatalf("%d runes of the fixture are missing", missing)
			}
			offset, items := subrsOfSubset(t, f)
			// Which road was taken: copied whole, the INDEX keeps the gap in
			// front of it; rebuilt, it follows the DICT directly.
			data, err := f.Subset()
			if err != nil {
				t.Fatal(err)
			}
			priv, err := PrivateDictForTest(font.SFNTTables(data)["CFF "])
			if err != nil {
				t.Fatal(err)
			}
			if want := map[bool]int{true: gap, false: 0}[whole]; offset-len(priv) != want {
				t.Errorf("a %d-byte gap (copied whole: %v): the INDEX is %d bytes past the "+
					"DICT's end, want %d", gap, whole, offset-len(priv), want)
			}
			if len(items) != 3 {
				t.Errorf("a %d-byte gap (copied whole: %v): the subset's local subroutine "+
					"INDEX holds %d entries, want 3 — the operand names something else",
					gap, whole, len(items))
				continue
			}
			for i, it := range items {
				if len(it) != 1 || it[0] != 11 {
					t.Errorf("a %d-byte gap (copied whole: %v): subroutine %d is %v, "+
						"want a single return", gap, whole, i, it)
				}
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
