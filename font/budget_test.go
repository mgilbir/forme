package font

import (
	"encoding/binary"
	"runtime"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The work one font may cost to read. Every test here is about a shape where a
// font's bytes name the same work again — a subtable named by many records, a
// subroutine called many times, a range of glyphs claimed twice — and the tests
// that measure cost measure a ratio: the same shape at n and at 4n, where work
// the font repeats comes out at sixteen times or more and work it does once
// comes out at four.
//
// The ratio is of units of the font's budget, counted, and not of time. Every
// parser that can be made to repeat itself charges the budget for each step it
// takes — a code a cmap walk visits, a byte of a Private DICT, an entry of an
// INDEX, a charstring operator — so the budget's Spent is already the count of
// the work these tests are about, and a count is the same number on a laptop,
// on a shared CI runner and under the race detector. A clock is not: these
// parses take a millisecond or two, and a window that short was spoiled often
// enough by the rest of the suite running beside it to put a linear curve past
// eight on CI.
//
// What a count cannot see is work nothing charges. That is not a hole in these
// tests so much as a defect in the budget, which is meant to cover every piece
// of a font's work that the font can make happen more than once (budget.go says
// which work is left out, and why it cannot be repeated): a parser that repeats
// work without charging for it is one the budget does not bound either, and the
// fix for that is a charge, which these tests would then count. Each test here
// was checked against the defect it was written for — the memo or the rule
// neutralised, and the counted ratio seen to go past eight.

// testBudget is a budget no fixture in this package comes near, for the tests
// that reach a parser directly and are about something other than its bound.
func testBudget() *Budget { return NewBudget(testWork) }

const testWork = 1 << 24

// linearIn compares the units small spends, reading the shape at n, with the
// units large spends reading it at 4n, each against a budget of its own, and
// fails the test when 4n costs more than eight times n: linear is four and the
// quadratic every one of these shapes had is sixteen, so eight is a factor of
// two from each.
//
// Both sides must have spent something, and neither may have run out. A side
// that charged nothing is a comparison of nothing with nothing, which any shape
// passes — a parse that failed early, or a fixture that stopped reaching the
// parser — and a side that ran out measured the bound and not the work.
func linearIn(t *testing.T, what string, small, large func(*Budget)) {
	t.Helper()
	spent := func(f func(*Budget)) int {
		t.Helper()
		b := testBudget()
		f(b)
		if b.Exhausted() {
			t.Fatalf("%s: a read spent the whole of its budget, so what was "+
				"counted is the bound and not the work (%v)", what, b.Err())
		}
		return b.Spent()
	}
	a, b := spent(small), spent(large)
	if a <= 0 {
		t.Fatalf("%s: the shape at n charged nothing to the budget, so there is "+
			"no work to compare the shape at 4n with", what)
	}
	ratio := float64(b) / float64(a)
	t.Logf("%s: 4n spends %.1f times n (%d units and %d)", what, ratio, a, b)
	if ratio > 8 {
		t.Errorf("%s: the shape at n spent %d units and at 4n %d, %.1f times — "+
			"linear is four, and the work being done again for every reference "+
			"to it is sixteen", what, a, b, ratio)
	}
}

// allocated is how many bytes f allocates.
func allocated(f func()) uint64 {
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// sfntWith is a font of nothing but the given tables, for the tests that need a
// table the fixtures elsewhere would write for them.
func sfntWith(tables map[string][]byte) []byte {
	tags := make([]string, 0, len(tables))
	for tag := range tables {
		tags = append(tags, tag)
	}
	head := make([]byte, 12+16*len(tags))
	binary.BigEndian.PutUint32(head, 0x00010000)
	binary.BigEndian.PutUint16(head[4:], uint16(len(tags)))
	out := head
	for i, tag := range tags {
		rec := 12 + 16*i
		copy(out[rec:], tag)
		binary.BigEndian.PutUint32(out[rec+8:], uint32(len(out)))
		binary.BigEndian.PutUint32(out[rec+12:], uint32(len(tables[tag])))
		out = append(out, tables[tag]...)
	}
	return out
}

// cmapOf is a cmap table whose records all name Unicode (3,10) and point at the
// given offsets into the subtables that follow them; offsets are relative to
// the first subtable.
func cmapOf(offsets []int, subtables []byte) []byte {
	head := make([]byte, 4+8*len(offsets))
	binary.BigEndian.PutUint16(head[2:], uint16(len(offsets)))
	for i, off := range offsets {
		binary.BigEndian.PutUint16(head[4+8*i:], 3)
		binary.BigEndian.PutUint16(head[4+8*i+2:], 10)
		binary.BigEndian.PutUint32(head[4+8*i+4:], uint32(len(head)+off))
	}
	return append(head, subtables...)
}

// TestACmapSubtableIsReadOncePerFont is audit C9: N encoding records naming one
// expensive subtable, and N records naming N of them, where one is all that is
// kept.
//
// A record is eight bytes and a format 13 subtable is twenty-eight however many
// codes it maps, so the font is small and the work it names is N × codes. Read
// once per record it was; read once per offset, and only the best that reads,
// it is the codes alone.
//
// The third case is the one where the two rules are not the same rule. A
// subtable that maps nothing — every glyph index past 0xFFFF — does not end the
// search, so every record naming it is tried; it is walked once only because
// what it came to is kept by its offset.
func TestACmapSubtableIsReadOncePerFont(t *testing.T) {
	aliased := func(n int) []byte {
		sub := fonttest.CmapFormat13([][3]uint32{{0, uint32(1024*n - 1), 1}})
		return sfntWith(map[string][]byte{"cmap": cmapOf(make([]int, 64*n), sub)})
	}
	distinct := func(n int) []byte {
		var subs []byte
		offs := make([]int, 64*n)
		for i := range offs {
			offs[i] = len(subs)
			subs = append(subs, fonttest.CmapFormat13([][3]uint32{{0, uint32(1024*n - 1), 1}})...)
		}
		return sfntWith(map[string][]byte{"cmap": cmapOf(offs, subs)})
	}
	unreadable := func(n int) []byte {
		sub := fonttest.CmapFormat12([][3]uint32{{0, uint32(16384*n - 1), 0x10000}})
		return sfntWith(map[string][]byte{"cmap": cmapOf(make([]int, 16*n), sub)})
	}
	for _, tc := range []struct {
		what   string
		font   func(n int) []byte
		mapped bool
	}{
		{"records naming one subtable", aliased, true},
		{"records naming equally ranked subtables", distinct, true},
		{"records naming one subtable that maps nothing", unreadable, false},
	} {
		small, large := tc.font(1), tc.font(4)
		for _, data := range [][]byte{small, large} {
			fp := ParseSFNT(data, testWork)
			if fp == nil || fp.CmapPartial || fp.BudgetExhausted || (len(fp.Cmap) > 0) != tc.mapped {
				t.Fatalf("%s: the fixture did not read in full, so this measures "+
					"something else", tc.what)
			}
		}
		linearIn(t, tc.what,
			func(b *Budget) { ParseSFNTWithin(small, b) },
			func(b *Budget) { ParseSFNTWithin(large, b) })
	}
}

// TestTheCmapBudgetIsTheFonts is the budget half of C9, which the rule above
// does not reach: subtables that cannot be read are each tried in turn, because
// a worse one may be the only one that reads, and each is walked before it is
// known to map nothing. Charged per subtable, eight walks that each fit are
// eight times the bound; charged to the font, they are the bound.
func TestTheCmapBudgetIsTheFonts(t *testing.T) {
	// Glyph indices past 0xFFFF name no glyph, so every code is walked and
	// none is kept: a subtable that reads as unreadable, at full cost.
	const codes, budget = 1 << 14, 1 << 16
	var subs []byte
	offs := make([]int, 8)
	for i := range offs {
		offs[i] = len(subs)
		subs = append(subs, fonttest.CmapFormat12([][3]uint32{{0, codes - 1, 0x10000}})...)
	}
	fp := ParseSFNT(sfntWith(map[string][]byte{"cmap": cmapOf(offs, subs)}), budget)
	if fp == nil {
		t.Fatal("the fixture did not parse")
	}
	if !fp.CmapPartial || !fp.BudgetExhausted {
		t.Errorf("eight walks of %d codes under a budget of %d finished (partial=%v, "+
			"exhausted=%v); each fits and all of them do not, so the budget is "+
			"being given again to each", codes, budget, fp.CmapPartial, fp.BudgetExhausted)
	}

	// And one of them fits, so the refusal above is the sum and not the part.
	fp = ParseSFNT(sfntWith(map[string][]byte{"cmap": cmapOf(offs[:1], subs)}), budget)
	if fp == nil || fp.CmapPartial || fp.BudgetExhausted {
		t.Error("one walk of the eight did not fit its budget; the test above " +
			"proves nothing about sharing it")
	}
}

// TestAWalkOverOverlappingGlyphsIsCharged: loca is required to ascend and
// nothing makes it, so entries alternating 0, L, 0, L give every other glyph
// the whole of glyf, and each of those glyphs walks all of its components.
func TestAWalkOverOverlappingGlyphsIsCharged(t *testing.T) {
	const components, glyphs = 1000, 20
	composite := make([]byte, 10)
	binary.BigEndian.PutUint16(composite, 0xFFFF) // numberOfContours -1
	for range components {
		// MORE_COMPONENTS, byte arguments: flags, glyph index, two bytes.
		composite = append(composite, 0, 0x20, 0, 1, 0, 0)
	}
	loca := make([]byte, 4*(glyphs+2))
	for i := 1; i < glyphs+2; i += 2 {
		binary.BigEndian.PutUint32(loca[4*i:], uint32(len(composite)))
	}
	gs := make([]fonttest.Glyph, glyphs)
	for i := range gs {
		gs[i] = fonttest.Glyph{Rune: rune('A' + i), Advance: 500}
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: gs,
		Extra:  map[string][]byte{"glyf": composite, "loca": loca},
	})
	// Ten glyphs of a thousand components each, against a budget of four
	// thousand: one glyph's walk fits and ten do not.
	fp := ParseSFNT(data, 4000)
	if fp == nil {
		t.Fatal("the fixture did not parse")
	}
	if !fp.BudgetExhausted {
		t.Errorf("ten walks of %d components each finished under a budget of "+
			"4000; the glyphs share the bytes, and the walk is repeated for each",
			components)
	}
}

// TestParseCFFRefusesASubroutineFanOut is audit C4: a subroutine calling the
// next sixteen times, seven deep, is 16^7 calls — a quarter of a billion — out
// of 250 bytes, and every one of them is within the depth the specification
// allows. The budget is what stops it, and it stops it by refusing the font:
// a width that was not found reads as the default, which looks like an answer.
func TestParseCFFRefusesASubroutineFanOut(t *testing.T) {
	if fp := ParseCFF(fonttest.SubrFanOut(2, 7, 2)); fp == nil {
		t.Fatal("a fan-out of two, 128 calls, was refused; the refusal below " +
			"would then say nothing about the budget")
	}
	if fp := ParseCFF(fonttest.SubrFanOut(16, 7, 2)); fp != nil {
		t.Errorf("a charstring whose subroutines make 16^7 calls was read, with "+
			"width %v; finding it took every one of them", fp.WidthByGID)
	}
}

// TestTheCharstringBudgetIsTheFontsNotTheGlyphs: every glyph calls the same
// subroutine tree, each well within a budget and all of them not.
func TestTheCharstringBudgetIsTheFontsNotTheGlyphs(t *testing.T) {
	// 4^6 is four thousand calls, about sixteen thousand steps a glyph.
	const budget = 1 << 16
	if parseCFF(fonttest.SubrFanOut(4, 6, 2), NewBudget(budget), true) == nil {
		t.Fatal("one glyph's walk did not fit the budget, so the refusal below " +
			"says nothing about sharing it")
	}
	if parseCFF(fonttest.SubrFanOut(4, 6, 20), NewBudget(budget), true) != nil {
		t.Error("nineteen glyphs each walking sixteen thousand steps were read " +
			"under a budget of sixty-five thousand; the budget is being given " +
			"again to each glyph")
	}
}

// TestParseCFFGlyphsDoesNotInterpretCharstrings is the other half of C4: what
// shape.Load reads is the glyph count, the charset and the collection, and
// none of it needs a charstring interpreted. So the fan-out costs it nothing
// at all, however wide: the two fonts differ only in the length of each
// subroutine, which no INDEX entry or DICT byte charged here depends on, and
// the counts come out the same. Interpreted, each glyph's walk is charged a
// step an operator, and a fan-out four times as wide five deep is a thousand
// times the steps.
func TestParseCFFGlyphsDoesNotInterpretCharstrings(t *testing.T) {
	small, large := fonttest.SubrFanOut(4, 5, 2), fonttest.SubrFanOut(16, 5, 2)
	for _, data := range [][]byte{small, large} {
		b := NewBudget(maxTestLoadWork)
		if fp := ParseCFFGlyphs(data, b); fp == nil || fp.NumGlyphs != 2 {
			t.Fatalf("the fixture did not read (%v)", b.Err())
		}
	}
	linearIn(t, "a subroutine fan-out of 4 and of 16",
		func(b *Budget) { ParseCFFGlyphs(small, b) },
		func(b *Budget) { ParseCFFGlyphs(large, b) })
}

// maxTestLoadWork is shape.Load's budget, which this package cannot see.
const maxTestLoadWork = 1 << 22

// TestAnEndcharInItsSeacFormHasNoWidth is audit C196. endchar takes no
// arguments or, in the deprecated seac form, four — adx ady bchar achar — so a
// width is there when it is given one operand or five. Four is not "more than
// none": it is an accented glyph, and its adx is not its width.
func TestAnEndcharInItsSeacFormHasNoWidth(t *testing.T) {
	const endchar = 14
	seac := []byte{num(100), num(0), num(65), num(70), endchar}
	for _, tc := range []struct {
		name  string
		cs    []byte
		width float64
		has   bool
	}{
		{"no operands", []byte{endchar}, 0, false},
		{"a width alone", []byte{num(42), endchar}, 42, true},
		{"the seac form", seac, 0, false},
		{"the seac form with a width", append([]byte{num(42)}, seac...), 42, true},
	} {
		w, has := type2CharstringWidth(tc.cs, cffIndex{}, cffIndex{}, testBudget())
		if has != tc.has || w != tc.width {
			t.Errorf("%s: width %v (found %v), want %v (found %v)",
				tc.name, w, has, tc.width, tc.has)
		}
	}

	// And through ParseCFF, where it is the number a caller reads: the seac
	// glyph states no width, so it takes the Private DICT's default, 500.
	fp := ParseCFF(fonttest.CFF(fonttest.CFFOptions{
		Glyphs:      2,
		Charstrings: [][]byte{{endchar}, seac},
	}))
	if fp == nil {
		t.Fatal("ParseCFF refused the fixture")
	}
	if got := fp.WidthByGID[1]; got != 500 {
		t.Errorf("the seac glyph is %v wide; it states no width, so it is the "+
			"default, 500 — and 100 is its accent's offset", got)
	}
}

// TestFDSelectRangesAreReadInOrder is audit C127. Format 3's ranges are
// ordered by their first glyph, and each runs to the next one's. Unordered,
// firsts alternating 0 and n made every other range the whole font.
func TestFDSelectRangesAreReadInOrder(t *testing.T) {
	sel := func(glyphs int) []byte {
		b := []byte{3, byte(glyphs >> 8), byte(glyphs)}
		for r := range glyphs {
			first := 0
			if r%2 == 1 {
				first = glyphs - 1
			}
			b = append(b, byte(first>>8), byte(first), 1)
		}
		return append(b, byte(glyphs>>8), byte(glyphs))
	}
	// Counted rather than timed: the writes are charged a unit a glyph, and a
	// read that stops at the second range costs forty microseconds, which is
	// too little for a clock to tell apart from what the race detector does to
	// allocating the answer — it put this linear curve at fourteen for four.
	spent := func(glyphs int) int {
		data, top := cidFont(t, sel(glyphs), privateDict(500, 0), privateDict(1000, 0))
		b := testBudget()
		parseCFFFDs(newCFFPrivates(data, b), top, glyphs, true)
		if b.Exhausted() {
			t.Fatalf("%d glyphs: the budget ran out, so nothing was measured", glyphs)
		}
		return b.Spent()
	}
	const n = 4000
	small, large := spent(n), spent(4*n)
	if ratio := float64(large) / float64(small); ratio > 8 || large > 2*4*n {
		t.Errorf("FDSelect ranges alternating 0 and n: %d glyphs cost %d units and %d "+
			"cost %d, %.1f times for four times the glyphs; read in order the writes "+
			"come to the glyph count, and a range allowed to start behind the last one "+
			"rewrites the whole font every other range, which is sixteen", n, small,
			4*n, large, ratio)
	}

	// What the order means for the answer. The third range starts before the
	// second and claims glyphs 1 and 2 for FD 1, which the first gave to FD 0;
	// read in order it is not a range at all.
	data, top := cidFont(t, []byte{
		3, 0, 3, // three ranges
		0, 0, 0, // glyphs 0-2 in FD 0
		0, 3, 1, // glyphs 3-5 in FD 1, if the next range were in order
		0, 1, 1, // first=1: before the range above it
		0, 6, // sentinel
	}, privateDict(500, 0), privateDict(1000, 0))
	fdOf, _ := parseCFFFDs(newCFFPrivates(data, testBudget()), top, 6, true)
	if fdOf[1] != 0 || fdOf[2] != 0 {
		t.Errorf("glyphs went to FDs %v; a range out of order reassigned glyphs "+
			"an earlier range had already placed", fdOf)
	}
}

// cffIndex16 is cffIndexOf with two-byte offsets, for a fixture past 255 bytes.
func cffIndex16(items ...[]byte) []byte {
	out := []byte{byte(len(items) >> 8), byte(len(items)), 2}
	off := 1
	out = binary.BigEndian.AppendUint16(out, uint16(off))
	for _, it := range items {
		off += len(it)
		out = binary.BigEndian.AppendUint16(out, uint16(off))
	}
	for _, it := range items {
		out = append(out, it...)
	}
	return out
}

// op3 is a DICT operand in the three-byte form, whatever its value.
func op3(v int) []byte { return []byte{28, byte(v >> 8), byte(v)} }

// fdArrayNaming lays out a CID-keyed font's Private DICTs as the given bytes,
// at a fixed offset, and an FDArray of Font DICTs each naming a (size, offset)
// pair relative to the start of them. It returns the bytes and the top DICT.
func fdArrayNaming(privs []byte, names [][2]int) ([]byte, map[int][]float64) {
	data := make([]byte, 16)
	for i := range data {
		data[i] = 1 // an operator that means nothing here; see cidFont
	}
	at := len(data)
	data = append(data, privs...)
	fontDicts := make([][]byte, len(names))
	for i, n := range names {
		fontDicts[i] = append(append(op3(n[0]), op3(at+n[1])...), 18)
	}
	fdaOff := len(data)
	data = append(data, cffIndex16(fontDicts...)...)
	return data, map[int][]float64{
		1230: {0, 0, 0},         // ROS: CID-keyed
		1236: {float64(fdaOff)}, // FDArray; no FDSelect, so every glyph is in FD 0
	}
}

// TestAPrivateDictIsReadOncePerFont: every Font DICT of a CID-keyed font may
// name the same Private DICT, and every Private DICT the same Subrs. Read once
// per Font DICT, N of them naming a DICT of K operators and an INDEX of M
// subroutines is N × (K + M); read once per offset, it is N + K + M.
func TestAPrivateDictIsReadOncePerFont(t *testing.T) {
	font := func(n int) ([]byte, map[int][]float64) {
		var priv []byte
		for range 256 * n {
			priv = append(append(priv, op3(500)...), 20) // defaultWidthX, again
		}
		subrs := make([][]byte, 256*n)
		for i := range subrs {
			subrs[i] = []byte{11} // return
		}
		// Subrs is relative to the Private DICT, and sits right after it.
		priv = append(append(priv, op3(len(priv)+4)...), 19)
		all := append(priv, cffIndex16(subrs...)...)
		names := make([][2]int, 256*n)
		for i := range names {
			names[i] = [2]int{len(priv), 0}
		}
		return fdArrayNaming(all, names)
	}
	run := func(n int) func(*Budget) {
		data, top := font(n)
		_, privs := parseCFFFDs(newCFFPrivates(data, testBudget()), top, 1, true)
		if len(privs) != 256*n || privs[len(privs)-1].def != 500 || len(privs[0].subrs.items) != 256*n {
			t.Fatalf("the fixture did not read: %d Private DICTs", len(privs))
		}
		return func(b *Budget) { parseCFFFDs(newCFFPrivates(data, b), top, 1, true) }
	}
	small, large := run(1), run(4)
	linearIn(t, "Font DICTs naming one Private DICT", small, large)

	// And distinct Private DICTs naming one Subrs INDEX, which what has been
	// read of the DICTs cannot stand in for: each is four bytes, its own, and
	// names the INDEX that follows them all, at whatever distance that is.
	shared := func(n int) ([]byte, map[int][]float64) {
		subrs := make([][]byte, 1024*n)
		for i := range subrs {
			subrs[i] = []byte{11}
		}
		const dict = 4 // a three-byte operand and the Subrs operator
		var all []byte
		names := make([][2]int, 256*n)
		for i := range names {
			names[i] = [2]int{dict, len(all)}
			all = append(append(all, op3(dict*(len(names)-i))...), 19)
		}
		return fdArrayNaming(append(all, cffIndex16(subrs...)...), names)
	}
	runShared := func(n int) func(*Budget) {
		data, top := shared(n)
		_, privs := parseCFFFDs(newCFFPrivates(data, testBudget()), top, 1, true)
		if len(privs) != 256*n || len(privs[0].subrs.items) != 1024*n || len(privs[len(privs)-1].subrs.items) != 1024*n {
			t.Fatalf("the fixture did not read: %d Private DICTs", len(privs))
		}
		return func(b *Budget) { parseCFFFDs(newCFFPrivates(data, b), top, 1, true) }
	}
	small, large = runShared(1), runShared(4)
	linearIn(t, "Private DICTs naming one Subrs INDEX", small, large)
}

// TestAPrivateDictIsCharged: Font DICTs naming Private DICTs that overlap are
// naming different DICTs, so what has been read cannot stand in for them, and
// a DICT's bytes are charged instead. Sixty-four DICTs of four kilobytes, each a
// byte shorter than the last, are a quarter of a megabyte of parsing out of
// four kilobytes of font.
func TestAPrivateDictIsCharged(t *testing.T) {
	const size, fds, budget = 4096, 64, 1 << 16
	priv := make([]byte, size)
	for i := range priv {
		priv[i] = 1 // an operator with nothing to do
	}
	names := make([][2]int, fds)
	for i := range names {
		names[i] = [2]int{size - i, 0}
	}
	data, top := fdArrayNaming(priv, names)
	p := newCFFPrivates(data, NewBudget(budget))
	parseCFFFDs(p, top, 1, true)
	if !p.bud.Exhausted() {
		t.Errorf("%d Private DICTs of about %d bytes each were read under a "+
			"budget of %d", fds, size, budget)
	}
}

// TestAnINDEXIsCharged: an INDEX's entries are charged as they are read, since
// a CFF may name any number of them from any offset.
func TestAnINDEXIsCharged(t *testing.T) {
	data := fonttest.CFF(fonttest.CFFOptions{Glyphs: 200})
	if ParseCFFGlyphs(data, NewBudget(1000)) == nil {
		t.Fatal("the fixture did not read under a budget it fits, so the refusal " +
			"below says nothing about the charge")
	}
	b := NewBudget(100)
	if fp := ParseCFFGlyphs(data, b); fp != nil || !b.Exhausted() {
		t.Errorf("two hundred charstrings were read under a budget of a hundred "+
			"(exhausted=%v)", b.Exhausted())
	}
}

// TestNoCountAFileStatesSizesAnAllocation is the rule the containers' own
// comments state: an allocation is sized from bytes that are there, never from
// a number the file declares. Each case is a small file declaring a large
// count, and each is refused; what is measured is what the refusal cost.
func TestNoCountAFileStatesSizesAnAllocation(t *testing.T) {
	// Built outside the measurement: the metrics a WOFF 2 hmtx is rebuilt
	// against, as a glyf declaring the whole 16-bit range would leave them.
	hmtxFont := &woff2Font{numGlyphs: 65535, numHMetrics: 65535, xMins: make([]int16, 65535)}
	bomb := fonttest.WOFFBomb(1000, 64<<20-64)
	for _, tc := range []struct {
		name string
		run  func() error
		// What a refusal may cost. A zlib reader and its window, and the
		// first 64 KB of room an inflate is given, are fixed; everything else
		// here is a scratch slice or two.
		limit uint64
	}{
		{"a WOFF table declaring 64 MiB (audit C126)", func() error {
			_, err := DecodeWOFF(bomb)
			return err
		}, 1 << 20},
		{"an sfnt directory declaring 65535 tables", func() error {
			if SFNTTables([]byte{0, 1, 0, 0, 0xFF, 0xFF, 0, 0, 0, 0, 0, 0}) != nil {
				return nil
			}
			return errRefused
		}, 48 << 10},
		{"a WOFF 2 directory declaring 4096 tables", func() error {
			h := make([]byte, woff2HeaderSize)
			binary.BigEndian.PutUint32(h, woff2Signature)
			binary.BigEndian.PutUint32(h[8:], uint32(len(h)))
			binary.BigEndian.PutUint16(h[12:], maxWOFFTables)
			_, err := DecodeWOFF2(h)
			return err
		}, 48 << 10},
		{"a WOFF 2 glyf declaring 65535 glyphs", func() error {
			// The box bitmap is there, 8 KB of it, and the contour counts
			// are not.
			_, err := rebuildGlyf(65535, 1, glyfStreams{bboxes: bboxBitmapFor(65535)})
			return err
		}, 48 << 10},
		{"a WOFF 2 glyph declaring 65534 contours", func() error {
			_, err := rebuildGlyf(1, 1, glyfStreams{
				nContours: []byte{0xFF, 0xFE},
				bboxes:    bboxBitmapFor(1),
			})
			return err
		}, 48 << 10},
		{"a WOFF 2 glyph declaring 65535 bytes of instructions", func() error {
			_, err := rebuildGlyf(1, 1, glyfStreams{
				nContours: []byte{0, 1},
				nPoints:   []byte{1},
				flags:     []byte{0},
				// One point's byte, then an instruction length of 65535.
				glyphs: []byte{0, 253, 0xFF, 0xFF},
				bboxes: bboxBitmapFor(1),
			})
			return err
		}, 48 << 10},
		{"a WOFF 2 composite declaring 65535 bytes of instructions", func() error {
			bb := bboxBitmapFor(1)
			bb[0] = 0x80 // a composite states its box
			_, err := rebuildGlyf(1, 1, glyfStreams{
				nContours: []byte{0xFF, 0xFF},
				// WE_HAVE_INSTRUCTIONS, one component, byte arguments.
				composites: []byte{0x01, 0x00, 0, 1, 0, 0},
				glyphs:     []byte{253, 0xFF, 0xFF},
				bboxes:     append(bb, make([]byte, 8)...),
			})
			return err
		}, 48 << 10},
		{"a WOFF 2 hmtx declaring 65535 advances", func() error {
			_, _, err := reconstructHmtx(nil, []byte{1}, hmtxFont)
			return err
		}, 48 << 10},
	} {
		var err error
		n := allocated(func() { err = tc.run() })
		if err == nil {
			t.Errorf("%s: accepted", tc.name)
			continue
		}
		if n > tc.limit {
			t.Errorf("%s: refused (%v) after allocating %d bytes", tc.name, err, n)
		}
	}
}

var errRefused = errorString("refused")

type errorString string

func (e errorString) Error() string { return string(e) }

// The budget's own error names what it was reading and says what it did not
// do, so that a refusal reaching a caller explains itself.
func TestABudgetThatRanOutSaysWhat(t *testing.T) {
	b := NewBudget(1)
	if b.Err() != nil || b.Exhausted() {
		t.Fatal("a budget nothing has spent reports itself spent")
	}
	if !b.charge(1, "x") || b.charge(1, "the character map") || b.charge(0, "y") {
		t.Fatal("a budget of one allowed more than one unit, or recovered")
	}
	if msg := b.Err().Error(); !strings.Contains(msg, "the character map") ||
		!strings.Contains(msg, "not read in full") {
		t.Errorf("the error %q does not say what was being read and that it was not", msg)
	}
	if NewBudget(-5).charge(1, "x") {
		t.Error("a negative budget allowed work")
	}
}
