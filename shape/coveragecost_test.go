package shape

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// coverageFormat2 builds a coverage table of n records, each naming the whole
// glyph space. Six bytes a record, sixty-five thousand glyphs a record.
func coverageFormat2(n int) []byte {
	out := make([]byte, 4+6*n)
	binary.BigEndian.PutUint16(out[0:], 2)
	binary.BigEndian.PutUint16(out[2:], uint16(n))
	for i := 0; i < n; i++ {
		rec := 4 + 6*i
		binary.BigEndian.PutUint16(out[rec:], 0)        // first glyph
		binary.BigEndian.PutUint16(out[rec+2:], 0xFFFF) // last glyph
		binary.BigEndian.PutUint16(out[rec+4:], 0)      // its coverage index
	}
	return out
}

// wideCoverageFont is a font whose GPOS holds one lookup whose coverage states
// records naming the whole glyph space.
func wideCoverageFont(records int) []byte {
	sub := make([]byte, 6)
	binary.BigEndian.PutUint16(sub[0:], 1)      // posFormat 1
	binary.BigEndian.PutUint16(sub[2:], 12)     // coverage offset
	binary.BigEndian.PutUint16(sub[4:], 0x0004) // XAdvance only
	// The value record: an advance of one unit, so that the adjustment is
	// something and the reader has to visit every glyph it applies to.
	sub = append(sub, 0, 1, 0, 0, 0, 0)
	sub = append(sub, coverageFormat2(records)...)

	return fonttest.SFNT(fonttest.SFNTOptions{
		Name:   "WideCoverage",
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}},
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOSLookups(
				[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{sub}}},
				map[string][]int{"kern": {0}}),
		},
	})
}

// TestCoverageExpansionIsBoundedForTheWholeTable is the cost a font could ask
// for at load.
//
// A format 2 record is six bytes and may name sixty-five thousand glyphs, and a
// table may hold as many records as its bytes allow. Bounded per range and not
// in total, a hundred and twenty kilobytes of coverage cost two and a half
// seconds before a word had been shaped — the audit measured it linear in the
// bytes at about thirteen microseconds each. The class definition beside it has
// had a total guard since it was written.
//
// What it asserts is that the bound engaged, not how long the load took: a
// clock reading is the symptom and the spent allowance is the thing. The
// allowance and the shape of the font make the two the same claim — a hundred
// and twenty kilobytes states 1.3 billion glyphs and is allowed about a
// million.
func TestCoverageExpansionIsBoundedForTheWholeTable(t *testing.T) {
	const records = 20000 // about 120 KB of coverage
	f, err := Load(wideCoverageFont(records))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	// The tables are read per script, so this is the read a document causes.
	l := readPositioning(f.layoutTables, nil, nil)
	budget := coverageBudget(f.layoutTables["GPOS"], f.layoutTables["GDEF"],
		f.layoutTables["kern"])
	if l.covWork > 0 {
		t.Errorf("%d records naming %d glyphs each spent %d of a %d-glyph allowance; "+
			"the expansion is bounded for each range and not for the table",
			records, 1<<16, budget-l.covWork, budget)
	}
	// And the read still ended, with the glyph the font actually has.
	if got, _ := f.ShapeGlyphs("a"); len(got) != 1 {
		t.Errorf("shaping one letter gave %d glyphs, want 1", len(got))
	}
}

// TestARealFaceDoesNotSpendItsCoverageAllowance is the control on live data. A
// budget a shipping font exhausts would drop kerning, ligatures or mark
// attachment silently, which is worse than the load it saves.
func TestARealFaceDoesNotSpendItsCoverageAllowance(t *testing.T) {
	names := notoTTFs(t)
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		f, err := Load(data)
		if err != nil {
			t.Fatalf("loading %s: %v", name, err)
		}
		// Every feature of every script, which is more coverage than any one
		// document asks this face to read.
		pos := readPositioning(f.layoutTables, nil, nil)
		sub := readLayout(f.layoutTables, nil, pos, nil)
		for _, l := range []struct {
			what string
			left int
			of   int
		}{
			{"GPOS", pos.covWork, coverageBudget(f.layoutTables["GPOS"],
				f.layoutTables["GDEF"], f.layoutTables["kern"])},
			{"GSUB", sub.covWork, coverageBudget(f.layoutTables["GSUB"])},
		} {
			if l.left <= 0 {
				t.Errorf("%s: reading %s spent the whole %d-glyph allowance, so some "+
					"coverage was cut short", filepath.Base(name), l.what, l.of)
			}
		}
	}
}

// TestAnOrdinaryCoverageIsReadWhole is the control. A budget tight enough to
// cut a real font's coverage short would lose kerning, ligatures and mark
// attachment silently, which is worse than the load it saves.
func TestAnOrdinaryCoverageIsReadWhole(t *testing.T) {
	// Three ranges, of the sizes a real font states: sixteen, one and two
	// hundred glyphs.
	table := make([]byte, 4+6*3)
	binary.BigEndian.PutUint16(table[0:], 2)
	binary.BigEndian.PutUint16(table[2:], 3)
	for i, r := range [][3]int{{10, 25, 0}, {40, 40, 16}, {100, 299, 17}} {
		rec := 4 + 6*i
		binary.BigEndian.PutUint16(table[rec:], uint16(r[0]))
		binary.BigEndian.PutUint16(table[rec+2:], uint16(r[1]))
		binary.BigEndian.PutUint16(table[rec+4:], uint16(r[2]))
	}
	// An offset of zero is "no coverage here", so the table sits where a
	// subtable would put it.
	sub := append(make([]byte, 8), table...)
	l := &layout{covWork: coverageBudget(sub)}
	got := map[int]int{}
	l.eachCovered(sub, 8, func(index, gid int) bool {
		got[index] = gid
		return true
	})
	if len(got) != 16+1+200 {
		t.Fatalf("the table names %d glyphs, want %d", len(got), 16+1+200)
	}
	for i, want := range []struct{ at, gid int }{
		{0, 10}, {15, 25}, {16, 40}, {17, 100}, {216, 299},
	} {
		if got[want.at] != want.gid {
			t.Errorf("case %d: coverage index %d is glyph %d, want %d",
				i, want.at, got[want.at], want.gid)
		}
	}
}

// TestTheCoverageBudgetIsSharedAcrossTheTable says what makes the bound a bound:
// one allowance for every coverage a table's lookups reach, since those lookups
// may point at the same record over and over.
func TestTheCoverageBudgetIsSharedAcrossTheTable(t *testing.T) {
	sub := append(make([]byte, 8), coverageFormat2(1)...)
	l := &layout{covWork: 100}
	count := func() int {
		n := 0
		l.eachCovered(sub, 8, func(int, int) bool { n++; return true })
		return n
	}
	if got := count(); got != 100 {
		t.Errorf("a budget of 100 produced %d glyphs", got)
	}
	if l.covWork != 0 {
		t.Errorf("the budget is %d after being spent, want 0", l.covWork)
	}
	if got := count(); got != 0 {
		t.Errorf("a spent budget produced %d glyphs; the allowance is the table's "+
			"and not each call's", got)
	}
	if !l.workSpent {
		t.Error("the allowance ran out and the layout does not know it did; " +
			"it could not say so")
	}
}

// TestAnUnbudgetedReaderExpandsNoCoverage pins the fail-closed answer, the same
// one an unbudgeted shaper gets from recurse. Every reader draws on the layout
// being built, so one with no allowance was assembled outside it — which is
// the state the bound exists to make impossible.
func TestAnUnbudgetedReaderExpandsNoCoverage(t *testing.T) {
	sub := append(make([]byte, 8), coverageFormat2(1)...)
	l := &layout{}
	n := 0
	l.eachCovered(sub, 8, func(int, int) bool { n++; return true })
	if n != 0 {
		t.Errorf("a reader with no allowance expanded %d glyphs", n)
	}
}

// classTableOf is a classTable holding the given classes, for a test that
// builds a layout by hand.
func classTableOf(classes map[int]int) classTable {
	highest := -1
	for g := range classes {
		highest = max(highest, g)
	}
	c := classTable{dense: make([]uint16, highest+1), named: len(classes) > 0}
	for g, class := range classes {
		c.dense[g] = uint16(class)
	}
	return c
}

// coverageOf is a format 1 coverage table naming the given glyphs.
func coverageOf(gids ...int) coverageTable {
	sorted := append([]int(nil), gids...)
	sortInts(sorted)
	b := make([]byte, 2+4+2*len(sorted)) // two bytes in front: a zero offset is no table
	binary.BigEndian.PutUint16(b[2:], 1)
	binary.BigEndian.PutUint16(b[4:], uint16(len(sorted)))
	for i, g := range sorted {
		binary.BigEndian.PutUint16(b[6+2*i:], uint16(g))
	}
	return coverageTable{base: b, off: 2}
}
