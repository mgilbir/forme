package shape

import (
	"encoding/binary"
	"errors"
	"math/rand"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// Whether a glyph can be kept, asked of glyphs broken in each of the ways a
// subset used to carry, skip or fail on (glyphcheck.go): CheckGlyph refuses
// the glyph and names it, a subset of a face that used it is refused the same
// way, and a subset of a face that did not is written as it always was.

// withGlyphs is the TrueType font data with the glyf entries entries names
// replaced, and loca written again to match.
func withGlyphs(t *testing.T, data []byte, entries map[int][]byte) []byte {
	t.Helper()
	tables := font.SFNTTables(data)
	n := font.Be16(tables["maxp"], 4)
	glyfT, err := (&Face{prog: &font.Program{NumGlyphs: n}, tables: tables}).glyfTables()
	if err != nil {
		t.Fatal(err)
	}
	var glyf []byte
	loca := make([]byte, 4*(n+1))
	for gid := 0; gid < n; gid++ {
		binary.BigEndian.PutUint32(loca[4*gid:], uint32(len(glyf)))
		b, ok := entries[gid]
		if !ok {
			if b, err = glyfT.entry(gid); err != nil {
				t.Fatal(err)
			}
		}
		glyf = append(glyf, b...)
		for len(glyf)%4 != 0 {
			glyf = append(glyf, 0)
		}
	}
	binary.BigEndian.PutUint32(loca[4*n:], uint32(len(glyf)))
	out := map[string][]byte{}
	for tag, b := range tables {
		out[tag] = b
	}
	head := append([]byte(nil), tables["head"]...)
	binary.BigEndian.PutUint16(head[50:], 1)
	out["head"], out["glyf"], out["loca"] = head, glyf, loca
	return assembleSFNT(out)
}

// compositeOf is a composite glyph's record: each component placed at an offset
// of words, MORE_COMPONENTS on all but the last.
func compositeOf(comps ...int) []byte {
	b := []byte{0xFF, 0xFF, 0, 0, 0, 0, 0x03, 0xE8, 0x03, 0xE8}
	for i, c := range comps {
		flags := compArgsAreWords | compArgsAreXY
		if i < len(comps)-1 {
			flags |= compMoreComponents
		}
		b = binary.BigEndian.AppendUint16(b, uint16(flags))
		b = binary.BigEndian.AppendUint16(b, uint16(c))
		b = append(b, 0, 0, 0, 0)
	}
	return b
}

// square is the fixture's own simple glyph: one contour of four points.
func square(t *testing.T) []byte {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	tables, err := f.glyfTables()
	if err != nil {
		t.Fatal(err)
	}
	b, err := tables.entry(1)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), b...)
}

// fiveLetters is a face of a to e, glyphs 1 to 5, with the entries given in
// place of theirs.
func fiveLetters(t *testing.T, entries map[int][]byte) *Face {
	t.Helper()
	var gs []fonttest.Glyph
	for r := 'a'; r <= 'e'; r++ {
		gs = append(gs, fonttest.Glyph{Rune: r, Advance: 500, HasShape: true})
	}
	data := withGlyphs(t, fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs}), entries)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

func TestCheckGlyphRefusesWhatASubsetMustNotCarry(t *testing.T) {
	sq := square(t)
	overrun := append([]byte(nil), sq...)
	binary.BigEndian.PutUint16(overrun[12:], 0xFFFF) // instructionLength
	truncated := compositeOf(2, 3)
	truncated = truncated[:len(truncated)-6] // the second component cut short
	gid24 := compositeOf(2)
	gid24[10] |= compGID24 >> 8
	for _, c := range []struct {
		name    string
		entries map[int][]byte
		gid     int
		// fault is the glyph whose own data is at fault, and want what the
		// error says about it.
		fault int
		want  string
	}{
		{"an instruction length past the record", map[int][]byte{1: overrun}, 1, 1, "instruction"},
		{"a glyph header cut short", map[int][]byte{1: sq[:6]}, 1, 1, "too short"},
		{"a simple glyph whose coordinates are cut short", map[int][]byte{1: sq[:len(sq)-6]}, 1, 1, "coordinates"},
		{"a truncated composite", map[int][]byte{1: truncated}, 1, 1, "component"},
		{"a component the font does not have", map[int][]byte{1: compositeOf(2, 999)}, 1, 1, "999"},
		{"a composite of a broken glyph", map[int][]byte{1: compositeOf(2, 3), 3: overrun}, 1, 3, "built from glyph 3"},
		{"a composite of a composite of a broken glyph", map[int][]byte{1: compositeOf(2), 2: compositeOf(3), 3: overrun}, 1, 3, "built from glyph 3"},
		{"a composite built from itself", map[int][]byte{1: compositeOf(2), 2: compositeOf(3), 3: compositeOf(1)}, 1, -1, "built, through its components, from this glyph"},
		{"a composite naming itself", map[int][]byte{1: compositeOf(1)}, 1, 1, "names glyph 1"},
		{"a component named by a 24-bit index", map[int][]byte{1: gid24}, 1, 1, "24-bit"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := fiveLetters(t, c.entries)
			var ge *GlyphError
			err := f.CheckGlyph(c.gid)
			if !errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{c.gid}) {
				t.Fatalf("CheckGlyph(%d) = %v, want a GlyphError naming it", c.gid, err)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("CheckGlyph(%d) says %q, want it to say %q", c.gid, err, c.want)
			}
			if c.fault >= 0 {
				if err := f.CheckGlyph(c.fault); !errors.As(err, &ge) {
					t.Errorf("CheckGlyph(%d) = %v for the glyph at fault", c.fault, err)
				}
				// And decided the other way round: the glyph at fault first,
				// so that the one built from it meets a verdict already made.
				g := fiveLetters(t, c.entries)
				_ = g.CheckGlyph(c.fault)
				if err := g.CheckGlyph(c.gid); !errors.As(err, &ge) {
					t.Errorf("CheckGlyph(%d) = %v after glyph %d was decided", c.gid, err, c.fault)
				}
			}
			// Glyph 5 is sound and built from nothing broken.
			if err := f.CheckGlyph(5); err != nil {
				t.Errorf("CheckGlyph(5) = %v for a sound glyph", err)
			}

			used := f.Clone()
			used.Use(c.gid, 5)
			if _, _, err := used.SubsetGlyphs(); !errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{c.gid}) {
				t.Errorf("a subset of a face that used glyph %d returned %v, want a GlyphError naming it", c.gid, err)
			}

			clean := f.Clone()
			clean.Use(5)
			prog, kept, err := clean.SubsetGlyphs()
			if err != nil {
				t.Fatalf("a subset of a face that used only a sound glyph: %v", err)
			}
			if !slices.Equal(kept, []int{0, 5}) {
				t.Errorf("kept %v, want [0 5]", kept)
			}
			sub, err := Load(prog)
			if err != nil {
				t.Fatal(err)
			}
			for gid := range sub.NumGlyphs() {
				if err := sub.CheckGlyph(gid); err != nil {
					t.Errorf("the subset carries glyph %d, which cannot be kept: %v", gid, err)
				}
			}
		})
	}
}

// TestAGlyphOutsideGlyfIsTheGlyphsFault: a loca entry past the end of glyf
// refused every subset of the face, whichever glyphs it used. It is the one
// glyph's, and refuses that glyph.
func TestAGlyphOutsideGlyfIsTheGlyphsFault(t *testing.T) {
	f := fiveLetters(t, nil)
	tables := map[string][]byte{}
	for tag, b := range f.sfntTables() {
		tables[tag] = b
	}
	loca := append([]byte(nil), tables["loca"]...)
	binary.BigEndian.PutUint32(loca[4*4:], 0x7FFFFFF0) // glyph 3 ends there; glyph 4 starts there
	tables["loca"] = loca
	f, err := Load(assembleSFNT(tables))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	var ge *GlyphError
	for _, gid := range []int{3, 4} {
		if err := f.CheckGlyph(gid); !errors.As(err, &ge) || !strings.Contains(err.Error(), "outside the glyf table") {
			t.Errorf("CheckGlyph(%d) = %v, want the glyph refused as outside glyf", gid, err)
		}
	}
	f.Use(1, 5)
	if _, err := f.Subset(); err != nil {
		t.Errorf("a subset that does not use the glyphs outside glyf: %v", err)
	}
}

// TestANotdefThatCannotBeKeptIsWrittenEmpty: .notdef is in every subset, used
// or not. Unused, one that cannot be kept is written as an empty glyph, which
// is a valid .notdef that nothing drew; used, it is refused like any glyph.
func TestANotdefThatCannotBeKeptIsWrittenEmpty(t *testing.T) {
	f := fiveLetters(t, map[int][]byte{0: {0, 1, 0, 0}})
	var ge *GlyphError
	if err := f.CheckGlyph(0); !errors.As(err, &ge) {
		t.Fatalf("CheckGlyph(0) = %v for a four-byte .notdef", err)
	}
	unused := f.Clone()
	unused.Use(2)
	prog, kept, err := unused.SubsetGlyphs()
	if err != nil {
		t.Fatalf("a subset that did not use .notdef: %v", err)
	}
	if !slices.Equal(kept, []int{0, 2}) {
		t.Errorf("kept %v, want [0 2]: .notdef is in the subset, written empty", kept)
	}
	sub, err := Load(prog)
	if err != nil {
		t.Fatal(err)
	}
	subTables, err := sub.glyfTables()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := subTables.entry(0); err != nil || len(b) != 0 {
		t.Errorf(".notdef was written as %v (%v), want an empty glyph", b, err)
	}

	used := f.Clone()
	used.Use(0, 2)
	if _, _, err := used.SubsetGlyphs(); !errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{0}) {
		t.Errorf("a subset that used .notdef returned %v, want a GlyphError naming it", err)
	}
}

// TestASubsetNamesEveryGlyphItRefuses: every used glyph that cannot be kept,
// not the first.
func TestASubsetNamesEveryGlyphItRefuses(t *testing.T) {
	sq := square(t)
	f := fiveLetters(t, map[int][]byte{2: sq[:6], 4: compositeOf(999)})
	f.Use(1, 2, 3, 4, 5)
	_, err := f.Subset()
	var ge *GlyphError
	if !errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{2, 4}) {
		t.Fatalf("Subset = %v, want a GlyphError naming glyphs 2 and 4", err)
	}
	if !strings.Contains(err.Error(), "glyph 2") {
		t.Errorf("the error does not say why the first was refused: %q", err)
	}
}

// TestCheckGlyphAndSubsetGiveOneAnswer holds the two to the same decision
// over a real face whose glyf has been damaged at random: for each glyph,
// CheckGlyph refuses it exactly when a subset of a face that used it alone is
// refused, naming it. It replaces the test that held the subsetter's own
// reader of a composite to font.MarkComposite: the subset no longer has a
// reader of its own to agree with anything.
func TestCheckGlyphAndSubsetGiveOneAnswer(t *testing.T) {
	data := append([]byte(nil), notoSansBytes(t)...)
	tables := font.SFNTTables(data)
	glyf := tables["glyf"] // aliases data, so writing it damages the font
	rng := rand.New(rand.NewSource(974))
	for range 3000 {
		glyf[rng.Intn(len(glyf))] = byte(rng.Intn(256))
	}
	f, err := Load(data)
	if err != nil {
		t.Skipf("the damaged face does not load: %v", err)
	}
	refused := 0
	for gid := 0; gid < f.NumGlyphs(); gid += 1 + rng.Intn(4) {
		checked := f.CheckGlyph(gid)
		var ge *GlyphError
		if checked != nil && !errors.As(checked, &ge) {
			t.Fatalf("CheckGlyph(%d) = %v, which is no glyph's fault", gid, checked)
		}
		c := f.Clone()
		c.Use(gid)
		_, _, err := c.SubsetGlyphs()
		switch {
		case checked == nil && err != nil:
			t.Fatalf("glyph %d: CheckGlyph keeps it and the subset says %v", gid, err)
		case checked != nil && (!errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{gid})):
			t.Fatalf("glyph %d: CheckGlyph refuses it (%v) and the subset says %v", gid, checked, err)
		}
		if checked != nil {
			refused++
		}
	}
	if refused == 0 {
		t.Fatal("no glyph was damaged enough to be refused; the comparison proves nothing")
	}
}

// TestEveryGlyphOfASoundFaceCanBeKept: the check refuses nothing the bundled
// face holds, glyf or CFF.
func TestEveryGlyphOfASoundFaceCanBeKept(t *testing.T) {
	noto, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	for gid := range noto.NumGlyphs() {
		if err := noto.CheckGlyph(gid); err != nil {
			t.Fatalf("Noto Sans: %v", err)
		}
	}
}

// TestCheckGlyphRunsACFFGlyphsCharstring: a CFF glyph is kept when its
// charstring runs, and refused when it does not.
func TestCheckGlyphRunsACFFGlyphsCharstring(t *testing.T) {
	cff := fonttest.CFF(fonttest.CFFOptions{Glyphs: 3, Charstrings: [][]byte{
		{14},        // .notdef
		{5, 21, 14}, // rlineto and rmoveto on an empty stack
		{139, 139, 21, 14},
	}})
	f, err := Load(fonttest.OTTO(cff, fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	var ge *GlyphError
	if err := f.CheckGlyph(1); !errors.As(err, &ge) || !strings.Contains(err.Error(), "cannot be run") {
		t.Errorf("CheckGlyph(1) = %v, want the charstring refused", err)
	}
	if err := f.CheckGlyph(2); err != nil {
		t.Errorf("CheckGlyph(2) = %v for a charstring that runs", err)
	}
	f.Encode("ab")
	if _, _, err := f.SubsetGlyphs(); !errors.As(err, &ge) || !slices.Equal(ge.GIDs, []int{1}) {
		t.Errorf("a subset carrying the broken charstring returned %v, want a GlyphError naming glyph 1", err)
	}
}

// TestACIDSubsetWritesAnUnkeptNotdefEmpty: the one subset whose .notdef is
// not simply a glyph left out, since a CID-keyed CFF is renumbered and glyph 0
// is in it whatever keep says.
func TestACIDSubsetWritesAnUnkeptNotdefEmpty(t *testing.T) {
	cff := cidCFF(3, []cidPrivate{{0, 2}}, []byte{139, 20})
	keep := []bool{false, true, false}
	sub, order, err := subsetCFF(cff, keep, fullBudget())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(order, []int{0, 1}) {
		t.Fatalf("order %v, want [0 1]", order)
	}
	o, err := readCFFOutlines(sub, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := o.charStrings[0]; !slices.Equal(got, []byte{14}) {
		t.Errorf(".notdef was written as %v, want a bare endchar", got)
	}
	if got := o.charStrings[1]; slices.Equal(got, []byte{14}) {
		t.Errorf("the kept glyph was written empty")
	}
}

// TestClonesShareTheirVerdicts: a face's clones decide each glyph once
// between them, from as many goroutines as lay documents out at once.
func TestClonesShareTheirVerdicts(t *testing.T) {
	sq := square(t)
	f := fiveLetters(t, map[int][]byte{3: sq[:6]})
	var wg sync.WaitGroup
	for range 8 {
		c := f.Clone()
		wg.Go(func() {
			for gid := range c.NumGlyphs() {
				_ = c.CheckGlyph(gid)
			}
		})
	}
	wg.Wait()
	if n := len(f.checks.verdicts); n != f.NumGlyphs() {
		t.Errorf("the face holds %d verdicts for %d glyphs", n, f.NumGlyphs())
	}
	var ge *GlyphError
	if err := f.CheckGlyph(3); !errors.As(err, &ge) {
		t.Errorf("CheckGlyph(3) = %v after its clones decided it", err)
	}
}

// TestCheckGlyphRefusesWhatTheFaceDoesNotHave: an index past the face, and a
// face with nothing to subset.
func TestCheckGlyphRefusesWhatTheFaceDoesNotHave(t *testing.T) {
	f := fiveLetters(t, nil)
	var ge *GlyphError
	for _, gid := range []int{-1, f.NumGlyphs()} {
		if err := f.CheckGlyph(gid); !errors.As(err, &ge) {
			t.Errorf("CheckGlyph(%d) = %v, want a GlyphError", gid, err)
		}
	}
	std, err := Standard("Helvetica")
	if err != nil {
		t.Fatal(err)
	}
	if err := std.CheckGlyph(0); err == nil || errors.As(err, &ge) {
		t.Errorf("a standard face's CheckGlyph(0) = %v, want an error that is no glyph's", err)
	}
}
