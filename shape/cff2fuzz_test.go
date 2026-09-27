package shape

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mgilbir/forme/font"
)

// The CFF2 reader and the writer that turns it into CFF, on arbitrary bytes.
//
// A CFF2 table is font input, and every count and offset in it is a number in
// the file: this is what holds the reader to not trusting one. And the writer
// has a property that needs no oracle: a glyph written as CFF draws the
// points the CFF2 glyph draws. The target runs every glyph both ways and
// compares them, so a mutation that finds a charstring the writer says wrongly
// fails here rather than on a page.

// cff2Table builds a CFF2 table: the charstrings, global and local
// subroutines, the Font DICT of each glyph (nil for all in the first) and each
// Font DICT's Private DICT entries before its Subrs, and a variation store
// (nil for none). Offsets are written five bytes wide, so one measuring pass
// lays it out.
type cff2Table struct {
	glyphs, global, local [][]byte
	fds                   []int
	privates              [][]byte
	store                 []byte
}

func cff2Index(items [][]byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(items)))
	if len(items) == 0 {
		return out
	}
	idx := writeCFFIndex(items)
	// A CFF INDEX's count is two bytes; the rest is the same.
	return append(out, idx[2:]...)
}

func (c cff2Table) bytes() []byte {
	nFD := len(c.privates)
	if nFD == 0 {
		nFD = 1
	}
	private := func(fd int) []byte {
		var p []byte
		if fd < len(c.privates) {
			p = append(p, c.privates[fd]...)
		}
		if fd == 0 && len(c.local) > 0 {
			p = append(append(p, cffInt(len(p)+6)...), 19)
		}
		return p
	}
	top := func(cs, fdArray, fdSelect, vstore int) []byte {
		d := append(cffInt(cs), 17)
		d = append(append(d, cffInt(fdArray)...), 12, 36)
		if c.fds != nil {
			d = append(append(d, cffInt(fdSelect)...), 12, 37)
		}
		if c.store != nil {
			d = append(append(d, cffInt(vstore)...), 24)
		}
		return d
	}
	fdArray := func(privAt []int) []byte {
		dicts := make([][]byte, nFD)
		for i := range dicts {
			dicts[i] = append(append(cffInt(len(private(i))), cffInt(privAt[i])...), 18)
		}
		return cff2Index(dicts)
	}
	var fdSelect []byte
	if c.fds != nil {
		fdSelect = []byte{0}
		for _, fd := range c.fds {
			fdSelect = append(fdSelect, byte(fd))
		}
	}
	var vstore []byte
	if c.store != nil {
		vstore = binary.BigEndian.AppendUint16(nil, uint16(len(c.store)))
		vstore = append(vstore, c.store...)
	}
	hdr := len(top(0, 0, 0, 0))
	at := 5 + hdr + len(cff2Index(c.global))
	vstoreAt := at
	at += len(vstore)
	csAt := at
	at += len(cff2Index(c.glyphs))
	fdSelectAt := at
	at += len(fdSelect)
	fdArrayAt := at
	privAt := make([]int, nFD)
	at += len(fdArray(privAt))
	for i := range privAt {
		privAt[i] = at
		at += len(private(i))
		if i == 0 && len(c.local) > 0 {
			at += len(cff2Index(c.local))
		}
	}
	out := []byte{2, 0, 5, byte(hdr >> 8), byte(hdr)}
	out = append(out, top(csAt, fdArrayAt, fdSelectAt, vstoreAt)...)
	out = append(out, cff2Index(c.global)...)
	out = append(out, vstore...)
	out = append(out, cff2Index(c.glyphs)...)
	out = append(out, fdSelect...)
	out = append(out, fdArray(privAt)...)
	for i := range privAt {
		out = append(out, private(i)...)
		if i == 0 && len(c.local) > 0 {
			out = append(out, cff2Index(c.local)...)
		}
	}
	return out
}

// oneAxisStore is an item variation store of one axis with one region, peaking
// at the axis's maximum, in groups of that region: groups copies of it.
func oneAxisStore(groups int) []byte {
	regions := []byte{0, 1, 0, 1, 0, 0, 0x40, 0, 0x40, 0} // one axis, one region (0, 1, 1)
	out := []byte{0, 1}
	out = binary.BigEndian.AppendUint32(out, uint32(8+4*groups))
	out = binary.BigEndian.AppendUint16(out, uint16(groups))
	dataAt := 8 + 4*groups + len(regions)
	for i := 0; i < groups; i++ {
		out = binary.BigEndian.AppendUint32(out, uint32(dataAt+8*i))
	}
	out = append(out, regions...)
	for i := 0; i < groups; i++ {
		// No items, no words, one region: region 0.
		out = append(out, 0, 0, 0, 0, 0, 1, 0, 0)
	}
	return out
}

// cff2GlyphCount reads how many charstrings a CFF2 table holds, the count a
// font's maxp would state for it.
func cff2GlyphCount(t []byte) (int, bool) {
	if len(t) < 5 {
		return 0, false
	}
	hdr, n := int(t[2]), int(binary.BigEndian.Uint16(t[3:]))
	if hdr+n > len(t) {
		return 0, false
	}
	top, _, err := parseCFF2Dict(t[hdr:hdr+n], func(int) int { return 0 })
	if err != nil {
		return 0, false
	}
	for _, e := range top {
		if e.op == opCharStrings && len(e.operands) == 1 {
			idx, err := readCFF2Index(t, int(e.operands[0].v))
			if err != nil {
				return 0, false
			}
			return len(idx.items), true
		}
	}
	return 0, false
}

// cff2Seeds are the tables the target starts from: small ones of each shape
// the reader reads, and the fixture's.
func cff2Seeds() [][]byte {
	move := []byte{139, 139, 21}
	seeds := [][]byte{
		cff2Table{glyphs: [][]byte{nil, append(move, 239, 239, 5)}}.bytes(),
		// Blends, in a glyph and in a subroutine, against a store.
		cff2Table{
			glyphs: [][]byte{nil,
				append(slices.Clone(move), 239, 149, 139, 139, 141, 16, 5),                        // 100 10 0 0 2 blend rlineto
				append(slices.Clone(move), 239, 149, 149, 139, 141, 32, 10),                       // … callsubr 0
				append([]byte{140, 15}, append(slices.Clone(move), 239, 149, 140, 16, 189, 5)...), // vsindex 1 … 1 blend 50 rlineto
			},
			local:    [][]byte{{16, 5}},
			store:    oneAxisStore(2),
			privates: [][]byte{{140, 22}},
		}.bytes(),
		// Two Font DICTs and an FDSelect, and hints.
		cff2Table{
			glyphs: [][]byte{nil, append([]byte{149, 159, 18, 149, 159, 19, 0xC0}, move...)},
			fds:    []int{0, 1},
			privates: [][]byte{
				{139, 149, 6},
				{139, 149, 6},
			},
		}.bytes(),
	}
	if data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", "CFF2Blend.otf")); err == nil {
		seeds = append(seeds, font.SFNTTables(data)["CFF2"])
	}
	return seeds
}

func FuzzCFF2(f *testing.F) {
	for _, s := range cff2Seeds() {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, table []byte) {
		n, ok := cff2GlyphCount(table)
		if !ok || n == 0 || n > 512 {
			return
		}
		budget := font.NewBudget(maxFontWork)
		cf, err := readCFF2(table, n, budget)
		if err != nil {
			return
		}
		checkCFF2RoundTrip(t, cf, len(table), nil, nil)
		checkCFF2RoundTrip(t, cf, len(table), []float64{0.5, -1}, []string{"wght", "wdth"})
		// And as HarfBuzz reads it, which the writer never does.
		o := cf.outlines(newHarfBuzzBlend(cf, []int{8192, -16384}))
		for g := 0; g < n; g++ {
			r := t2Run{o: o, budget: font.NewBudget(maxFontWork)}
			r.bounds(g, false)
		}
	})
}

// checkCFF2RoundTrip writes a CFF2 font as CFF at a location and holds every
// glyph it did not write empty to drawing what the CFF2 glyph draws there.
func checkCFF2RoundTrip(t *testing.T, cf *cff2Font, tableLen int, coords []float64, tags []string) {
	t.Helper()
	n := len(cf.charStrings)
	advances := make([]int, n)
	for g := range advances {
		advances[g] = 500 + g
	}
	blend := newFontToolsBlend(cf, coords, tags)
	conv, err := cff2ToCFF(cf, blend, advances, "Fuzz", font.NewBudget(cffConvertWork(tableLen)))
	if err != nil {
		return
	}
	written, err := readCFFOutlines(conv.cff, n)
	if err != nil {
		t.Fatalf("the CFF written from a CFF2 font cannot be read: %v", err)
	}
	for g := 0; g < n; g++ {
		var want []t2Seg
		r := t2Run{o: cf.outlines(newFontToolsBlend(cf, coords, tags)), budget: font.NewBudget(maxFontWork),
			draw: true, path: func(s t2Seg) { want = append(want, s) }}
		if _, ok := r.bounds(g, false); !ok {
			continue // written empty
		}
		got, ok := cffOutline(written, g)
		if !ok {
			t.Fatalf("glyph %d: the CFF written cannot be run", g)
		}
		if len(got) == 0 && len(want) > 0 {
			continue // written empty: a number it draws the CFF form cannot state
		}
		// A path that starts without a move starts at the origin, which the
		// CFF form says with a move there.
		if len(want) > 0 && want[0].op != 'M' && len(got) > 0 && got[0].op == 'M' &&
			got[0].pts[0] == 0 && got[0].pts[1] == 0 {
			got = got[1:]
		}
		if !slices.Equal(got, want) {
			t.Fatalf("glyph %d: written as CFF it draws %v, and as CFF2 %v", g, got, want)
		}
	}
}

// TestTheCFF2SeedsAreRead: a seed the reader refuses starts the fuzzer from
// nothing, so each is read, written as CFF with nothing left empty, and held to
// the round trip at the default and away from it.
func TestTheCFF2SeedsAreRead(t *testing.T) {
	seeds := cff2Seeds()
	if len(seeds) < 4 {
		t.Fatalf("%d seeds; the fixture's table is missing", len(seeds))
	}
	for i, s := range seeds {
		n, ok := cff2GlyphCount(s)
		if !ok {
			t.Fatalf("seed %d: no glyph count", i)
		}
		cf, err := readCFF2(s, n, font.NewBudget(maxFontWork))
		if err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
		for _, coords := range [][]float64{nil, {0.5, -1}} {
			conv, err := cff2ToCFF(cf, newFontToolsBlend(cf, coords, []string{"wght", "wdth"}),
				make([]int, n), "Seed", font.NewBudget(maxFontWork))
			if err != nil || len(conv.limits) > 0 {
				t.Fatalf("seed %d at %v: %v %q", i, coords, err, conv.limits)
			}
			checkCFF2RoundTrip(t, cf, len(s), coords, []string{"wght", "wdth"})
		}
	}
}
