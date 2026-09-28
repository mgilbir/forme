package shape

import (
	"encoding/binary"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What a CFF2 table may not make the reader or the writer do, and what they
// say when a font asks for it.

// cff2Font builds a face's bytes around a CFF2 table: an OpenType font whose
// glyph g, from 1, is glyphs[g-1].
func cff2Sfnt(table []byte, glyphs []fonttest.Glyph) []byte {
	tables := font.SFNTTables(fonttest.OTTO(nil, fonttest.SFNTOptions{Glyphs: glyphs}))
	delete(tables, "CFF ")
	tables["CFF2"] = table
	return assembleOTTO(tables)
}

// twoGlyphs is a font of .notdef and 'a', drawn by code.
func twoGlyphs(code []byte) cff2Table {
	return cff2Table{glyphs: [][]byte{{139, 139, 21}, code}}
}

var aGlyph = []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}}

// TestACFF2TableIsReadOnlyWhereItSays refuses each way a CFF2 table can point
// outside itself or at something that is not there, and reads the table they
// were made from. Each is a count or an offset the file states: none is
// believed before it is checked, and nothing is allocated for one first.
func TestACFF2TableIsReadOnlyWhereItSays(t *testing.T) {
	good := cff2Table{
		glyphs: [][]byte{{139, 139, 21}, {139, 139, 21, 239, 239, 5}},
		fds:    []int{0, 1},
		privates: [][]byte{
			{139, 149, 6},
			{139, 149, 6},
		},
		local: [][]byte{{5}},
		store: oneAxisStore(1),
	}.bytes()
	if _, err := readCFF2(good, 2, font.NewBudget(maxFontWork)); err != nil {
		t.Fatalf("the table the cases are made from does not read: %v", err)
	}
	topAt := 5
	hdrLen := int(binary.BigEndian.Uint16(good[3:]))
	// The operand of the Top DICT's entry for op, whose five bytes are rewritten.
	topOperand := func(op int) int {
		ops, _, err := parseCFF2Dict(good[topAt:topAt+hdrLen], func(int) int { return 0 })
		if err != nil {
			t.Fatal(err)
		}
		at := topAt
		for _, e := range ops {
			if e.op == op {
				return at + 1 // after the 29
			}
			at += len(e.raw)
		}
		t.Fatalf("no Top DICT entry %d", op)
		return 0
	}
	put32 := func(b []byte, at, v int) []byte {
		out := slices.Clone(b)
		binary.BigEndian.PutUint32(out[at:], uint32(int32(v)))
		return out
	}
	offsetOf := func(op int) int { return int(binary.BigEndian.Uint32(good[topOperand(op):])) }
	cases := map[string][]byte{
		"too short":                      good[:4],
		"version 1":                      append([]byte{1}, good[1:]...),
		"a Top DICT past the table":      append(good[:3:3], 0xFF, 0xFF),
		"CharStrings past the table":     put32(good, topOperand(opCharStrings), len(good)+10),
		"CharStrings at zero":            put32(good, topOperand(opCharStrings), 0),
		"an FDArray past the table":      put32(good, topOperand(opFDArray), len(good)),
		"an FDSelect past the table":     put32(good, topOperand(opFDSelect), len(good)-1),
		"a store past the table":         put32(good, topOperand(opVstore), len(good)-1),
		"a count of four billion":        put32(good, offsetOf(opCharStrings), -1),
		"a count past maxCFFIndexItems":  put32(good, offsetOf(opCharStrings), maxCFFIndexItems+1),
		"an FDSelect naming Font DICT 7": func() []byte { b := slices.Clone(good); b[offsetOf(opFDSelect)+2] = 7; return b }(),
		"an FDSelect format 5":           func() []byte { b := slices.Clone(good); b[offsetOf(opFDSelect)] = 5; return b }(),
		"a store longer than the table": func() []byte {
			b := slices.Clone(good)
			binary.BigEndian.PutUint16(b[offsetOf(opVstore):], 0xFFFF)
			return b
		}(),
		"a reserved byte in the Top DICT": func() []byte { b := slices.Clone(good); b[topAt] = 255; return b }(),
	}
	for name, table := range cases {
		n := 2
		if name == "a count past maxCFFIndexItems" || name == "a count of four billion" {
			n = 2
		}
		if _, err := readCFF2(table, n, font.NewBudget(maxFontWork)); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	// And a glyph count the table does not have.
	if _, err := readCFF2(good, 3, font.NewBudget(maxFontWork)); err == nil {
		t.Error("a table of two charstrings read for a font of three glyphs")
	}
}

// TestACFF2FDSelectCoversEveryGlyph: formats 3 and 4 are ranges, which have to
// start at the first glyph, rise, name Font DICTs the font has and end at the
// glyph count; format 4 is format 3 in wider fields.
func TestACFF2FDSelectCoversEveryGlyph(t *testing.T) {
	f3 := func(ranges [][2]int, sentinel int) []byte {
		b := []byte{3}
		b = binary.BigEndian.AppendUint16(b, uint16(len(ranges)))
		for _, r := range ranges {
			b = binary.BigEndian.AppendUint16(b, uint16(r[0]))
			b = append(b, byte(r[1]))
		}
		return binary.BigEndian.AppendUint16(b, uint16(sentinel))
	}
	f4 := func(ranges [][2]int, sentinel int) []byte {
		b := []byte{4}
		b = binary.BigEndian.AppendUint32(b, uint32(len(ranges)))
		for _, r := range ranges {
			b = binary.BigEndian.AppendUint32(b, uint32(r[0]))
			b = binary.BigEndian.AppendUint16(b, uint16(r[1]))
		}
		return binary.BigEndian.AppendUint32(b, uint32(sentinel))
	}
	for _, c := range []struct {
		name string
		sel  []byte
		want []int // nil: refused
	}{
		{"format 3", f3([][2]int{{0, 1}, {2, 0}}, 4), []int{1, 1, 0, 0}},
		{"format 4", f4([][2]int{{0, 0}, {1, 1}}, 4), []int{0, 1, 1, 1}},
		{"format 3 not from zero", f3([][2]int{{1, 0}}, 4), nil},
		{"format 4 falling", f4([][2]int{{0, 0}, {3, 1}, {2, 0}}, 4), nil},
		{"format 3 short of the count", f3([][2]int{{0, 0}}, 3), nil},
		{"format 4 past the count", f4([][2]int{{0, 0}}, 5), nil},
		{"format 4 naming Font DICT 2", f4([][2]int{{0, 2}}, 4), nil},
		{"format 3 with no ranges", f3(nil, 4), nil},
		{"format 4 claiming more ranges than it has", f4([][2]int{{0, 0}}, 4)[:9], nil},
		{"format 0 a byte short", []byte{0, 0, 0, 0}, nil},
	} {
		table := append([]byte{0xFF}, c.sel...) // at offset 1
		got, err := readCFF2FDSelect(table, 1, 4, 2)
		if c.want == nil {
			if err == nil {
				t.Errorf("%s: read as %v", c.name, got)
			}
			continue
		}
		if err != nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: %v, %v; want %v", c.name, got, err, c.want)
		}
	}
}

// TestACFF2DictBlendIsHeldToItsOperands: a blend in a DICT that asks for more
// operands than came before it, a count that is not a count, and a vsindex
// that is not an index are refused; a blend against a group the store does not
// have blends nothing.
func TestACFF2DictBlendIsHeldToItsOperands(t *testing.T) {
	two := func(int) int { return 2 }
	for name, dict := range map[string][]byte{
		"a blend of more than it has":   {139, 140, 141, 142, 23, 6}, // 0 1 2, count 3: needs 9
		"a negative count":              {139, 140, 138, 23, 6},      // count -1
		"a blend with no count":         {23},                        // nothing at all
		"a vsindex of two numbers":      {140, 140, 22},              //
		"a negative vsindex":            {138, 22},                   //
		"a real that never ends":        {30, 0x12, 0x34},            //
		"a real with a reserved nibble": {30, 0x1D, 0xFF, 6},         //
		"operands with no operator":     {139, 140},                  //
	} {
		if _, _, err := parseCFF2Dict(dict, two); err == nil {
			t.Errorf("%s: read", name)
		}
	}
	entries, _, err := parseCFF2Dict([]byte{149, 159, 169, 140, 23, 6}, two) // 10, deltas 20 30, one value
	if err != nil || len(entries) != 1 || len(entries[0].operands) != 1 ||
		!slices.Equal(entries[0].operands[0].deltas, []float64{20, 30}) {
		t.Errorf("a blend of one value and two deltas read as %+v, %v", entries, err)
	}
}

// TestACFF2GlyphThatCannotBeRunIsEmbeddedEmptyAndSaid: a charstring HarfBuzz
// cannot run — here one that pops an empty stack — has no outline to write.
// Its face still loads; the glyph is written as an empty one at its advance,
// its ink is that empty glyph's, and the face says which glyph it was.
func TestACFF2GlyphThatCannotBeRunIsEmbeddedEmptyAndSaid(t *testing.T) {
	data := cff2Sfnt(twoGlyphs([]byte{5, 21}).bytes(), aGlyph) // rlineto, rmoveto on an empty stack
	f, err := Load(data)
	if err != nil {
		t.Fatalf("a CFF2 font with one broken glyph does not load: %v", err)
	}
	f.Encode("a")
	prog, kept, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	cff := font.SFNTTables(prog)["CFF "]
	o, err := readCFFOutlines(cff, len(kept))
	if err != nil {
		t.Fatalf("the subset cannot be read: %v", err)
	}
	if segs, ok := cffOutline(o, 1); !ok || len(segs) != 0 {
		t.Errorf("the broken glyph was written as %v (%v), want an empty glyph", segs, ok)
	}
	if w := font.ParseCFF(cff).WidthByCID[1]; w != 500 {
		t.Errorf("the empty glyph's width is %v, want its advance, 500", w)
	}
	if e, ok := f.glyphExtents(1); !ok || e != (extents{}) {
		t.Errorf("its ink is %+v (%v), want the empty glyph's", e, ok)
	}
	if lim := strings.Join(f.LayoutLimits(), "\n"); !strings.Contains(lim, "[1]") ||
		!strings.Contains(lim, "embedded as empty glyphs") {
		t.Errorf("the face does not say glyph 1 was written empty: %q", lim)
	}
}

// TestACFF2NumberACFFCannotStateIsSaid: blended, a coordinate can move further
// than a Type 2 number reaches — past 32,767 — and a glyph whose outline needs
// one is written empty and said to be, rather than written with some other
// number.
func TestACFF2NumberACFFCannotStateIsSaid(t *testing.T) {
	// 0 0 rmoveto, then 32000 + 10000 at the axis's end: 42000 rlineto.
	code := cat([]byte{139, 139, 21}, t2(32000), t2(10000), []byte{140, 16, 139, 5})
	table := cff2Table{glyphs: [][]byte{{139, 139, 21}, code}, store: oneAxisStore(1)}.bytes()
	cf, err := readCFF2(table, 2, font.NewBudget(maxFontWork))
	if err != nil {
		t.Fatal(err)
	}
	at := func(c float64) *cff2Converted {
		conv, err := cff2ToCFF(cf, newFontToolsBlend(cf, []float64{c}, []string{"wght"}), []int{500, 500},
			"T", font.NewBudget(maxFontWork))
		if err != nil {
			t.Fatal(err)
		}
		return conv
	}
	if conv := at(0); len(conv.limits) != 0 {
		t.Fatalf("at the default, where the number is 32000: %q", conv.limits)
	}
	conv := at(1)
	if len(conv.limits) == 0 || !strings.Contains(conv.limits[0], "cannot state") {
		t.Errorf("a glyph drawing 42000 in one step was written without a word: %q", conv.limits)
	}
	if conv.bounds[1].set {
		t.Errorf("the glyph written empty has bounds %+v", conv.bounds[1])
	}
}

// TestACFF2WriterStopsWhereItsWorkAndSizeDo: a font whose glyphs each run a
// subroutine loop to HarfBuzz's cap spends the work one font may cost in a
// few dozen glyphs; the glyphs after that are written empty, and the writer
// says so. And with the size the charstrings may come to set low, the glyphs
// past it are written empty too.
func TestACFF2WriterStopsWhereItsWorkAndSizeDo(t *testing.T) {
	// Subroutine 1 draws a line; subroutine 0 calls it forty times; the glyph
	// calls subroutine 0 as often as its bytes allow.
	line := []byte{140, 140, 5}
	fan := []byte{}
	for i := 0; i < 40; i++ {
		fan = append(fan, callSubr(1)...)
	}
	glyph := []byte{139, 139, 21}
	for i := 0; i < 2000; i++ {
		glyph = append(glyph, callSubr(0)...)
	}
	glyphs := [][]byte{{139, 139, 21}}
	for g := 0; g < 64; g++ {
		glyphs = append(glyphs, glyph)
	}
	table := cff2Table{glyphs: glyphs, local: [][]byte{fan, line}}.bytes()
	cf, err := readCFF2(table, len(glyphs), font.NewBudget(maxFontWork))
	if err != nil {
		t.Fatal(err)
	}
	advances := make([]int, len(glyphs))
	w, err := newCFF2Writer(cf, newFontToolsBlend(cf, nil, nil), advances, font.NewBudget(cffConvertWork(len(table))))
	if err != nil {
		t.Fatal(err)
	}
	empty := 0
	for g := range glyphs {
		if _, b := w.glyph(g); !b.set {
			empty++
		}
	}
	if !w.spent || empty == 0 || empty == len(glyphs) {
		t.Errorf("spent %v, %d of %d glyphs empty: the work should run out part way", w.spent, empty, len(glyphs))
	}
	if lim := strings.Join(w.limits(), "\n"); !strings.Contains(lim, "work") {
		t.Errorf("the writer does not say its work ran out: %q", lim)
	}

	// Glyphs well under the cap, of about thirty kilobytes of lines each.
	medium := []byte{139, 139, 21}
	for i := 0; i < 400; i++ {
		medium = append(medium, callSubr(0)...)
	}
	for g := 1; g < len(glyphs); g++ {
		glyphs[g] = medium
	}
	table = cff2Table{glyphs: glyphs, local: [][]byte{fan, line}}.bytes()
	if cf, err = readCFF2(table, len(glyphs), font.NewBudget(maxFontWork)); err != nil {
		t.Fatal(err)
	}
	w, err = newCFF2Writer(cf, newFontToolsBlend(cf, nil, nil), advances, font.NewBudget(maxFontWork*64))
	if err != nil {
		t.Fatal(err)
	}
	w.sizeLimit = 4 << 10
	empty = 0
	for g := 1; g < 4; g++ {
		if _, b := w.glyph(g); !b.set {
			empty++
		}
	}
	if !w.full || empty == 0 {
		t.Errorf("full %v, %d empty: the size set should run out", w.full, empty)
	}
	if w.written > w.sizeLimit+64 {
		t.Errorf("%d bytes written past a limit of %d", w.written, w.sizeLimit)
	}
}

// TestACFF2WidthIsWrittenAgainstItsFontDict: a width is one Type 2 number from
// the nominal width. Advances spread further than one number reaches from the
// commonest are written against their middle, and ones spread further than
// that from the middle refuse the font rather than being written as other
// widths.
func TestACFF2WidthIsWrittenAgainstItsFontDict(t *testing.T) {
	table := cff2Table{glyphs: [][]byte{{139, 139, 21}, {139, 139, 21}, {139, 139, 21}}}.bytes()
	cf, err := readCFF2(table, 3, font.NewBudget(maxFontWork))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		advances []int
		ok       bool
	}{
		{[]int{500, 500, 40000}, true},
		{[]int{0, 0, 65535}, false},
	} {
		conv, err := cff2ToCFF(cf, newFontToolsBlend(cf, nil, nil), c.advances, "T", font.NewBudget(maxFontWork))
		if !c.ok {
			if err == nil {
				t.Errorf("advances %v were written", c.advances)
			}
			continue
		}
		if err != nil {
			t.Fatalf("advances %v: %v", c.advances, err)
		}
		prog := font.ParseCFF(conv.cff)
		if prog == nil {
			t.Fatal("the font package cannot read the CFF written")
		}
		for g, adv := range c.advances {
			if w := prog.WidthByCID[g]; w != float64(adv) {
				t.Errorf("advances %v: glyph %d's width is %v", c.advances, g, w)
			}
		}
	}
}

// TestACFF2FaceSubsetDrawsItsGlyphs subsets a CFF2 face read at its default:
// the kept glyphs renumbered, each keeping its glyph index as its CID and the
// width of its advance, and each drawing, point for point, what the CFF2 glyph
// draws there — which HarfBuzz is held to in cff2_test.go. And its clones
// share the one writer, from any number of goroutines.
func TestACFF2FaceSubsetDrawsItsGlyphs(t *testing.T) {
	data := harfbuzzFont(t, "CFF2Blend.otf")
	face, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	cf, _ := readCFF2Of(t, data)
	defaultOutlines := cf.outlines(newHarfBuzzBlend(cf, nil))
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(text string) {
			defer wg.Done()
			f := face.Clone()
			f.Encode(text)
			prog, kept, err := f.SubsetGlyphs()
			if err != nil {
				t.Error(err)
				return
			}
			cff := font.SFNTTables(prog)["CFF "]
			p := font.ParseCFF(cff)
			o, err := readCFFOutlines(cff, len(kept))
			if err != nil || p == nil {
				t.Errorf("the subset cannot be read: %v", err)
				return
			}
			for i, g := range kept {
				if p.GIDToCID[i] != g {
					t.Errorf("subset glyph %d, glyph %d of the font, has CID %d", i, g, p.GIDToCID[i])
				}
				if w := p.WidthByCID[g]; w != f.GlyphAdvance(g) {
					t.Errorf("glyph %d's width is %v and its advance %v", g, w, f.GlyphAdvance(g))
				}
				got, _ := cffOutline(o, i)
				want, _ := cffOutline(defaultOutlines, g)
				if !slices.Equal(got, want) {
					t.Errorf("glyph %d draws %v in the subset, and %v in the font", g, got, want)
				}
				if _, ok := f.glyphExtents(g); !ok {
					t.Errorf("glyph %d has no ink", g)
				}
			}
		}([]string{"abc", "defg", "hijk", "akb"}[i])
	}
	wg.Wait()
	if len(face.Program()) == 0 {
		t.Error("the face has no program")
	}
}

// TestACFF2IndexHoldsAtMostMaxCFFIndexItems: an INDEX of empty items costs a
// byte an item and could otherwise ask for as many as its count says.
func TestACFF2IndexHoldsAtMostMaxCFFIndexItems(t *testing.T) {
	index := func(n int) []byte {
		b := binary.BigEndian.AppendUint32(nil, uint32(n))
		b = append(b, 1)
		for i := 0; i <= n; i++ {
			b = append(b, 1)
		}
		return b
	}
	if idx, err := readCFF2Index(index(maxCFFIndexItems), 0); err != nil || len(idx.items) != maxCFFIndexItems {
		t.Errorf("an INDEX of %d items: %v", maxCFFIndexItems, err)
	}
	if _, err := readCFF2Index(index(maxCFFIndexItems+1), 0); err == nil {
		t.Errorf("an INDEX of %d items was read", maxCFFIndexItems+1)
	}
}
