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
	sub = append(sub, make([]byte, 6)...)       // the value record
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
	budget := coverageBudget(sub)
	got := coverageGlyphs(sub, 8, &budget)
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
	budget := 100
	if got := coverageGlyphs(sub, 8, &budget); len(got) != 100 {
		t.Errorf("a budget of 100 produced %d glyphs", len(got))
	}
	if budget != 0 {
		t.Errorf("the budget is %d after being spent, want 0", budget)
	}
	if got := coverageGlyphs(sub, 8, &budget); len(got) != 0 {
		t.Errorf("a spent budget produced %d glyphs; the allowance is the table's "+
			"and not each call's", len(got))
	}
}

// TestAnUnbudgetedReaderExpandsNoCoverage pins the fail-closed answer, the same
// one an unbudgeted shaper gets from recurse. Every reader draws on either the
// layout being built or the run being shaped, so one holding neither was
// assembled outside both — which is the state the bound exists to make
// impossible.
func TestAnUnbudgetedReaderExpandsNoCoverage(t *testing.T) {
	sub := append(make([]byte, 8), coverageFormat2(1)...)
	if got := coverageGlyphs(sub, 8, nil); got != nil {
		t.Errorf("a reader with no allowance expanded %d glyphs", len(got))
	}
}

// TestTheRunAllowanceCoversMoreThanARunAsks is the shaping-time half of the
// bound. A mark subtable a rule names is read where it is applied rather than
// at load, so the layout's allowance cannot cover it and the run needs its own.
//
// A limit a real run reaches is a correctness bug rather than a safety
// property, so what this asserts is the headroom: a whole glyph space for every
// glyph in the run, against the few hundred glyphs a real mark coverage names.
func TestTheRunAllowanceCoversMoreThanARunAsks(t *testing.T) {
	// A word, and a paragraph.
	for _, glyphs := range []int{1, 5, 500} {
		got := *markCoverageBudget(glyphs)
		// Two coverages a subtable, and no run applies more mark subtables than
		// it has glyphs.
		const realMarkCoverage = 512
		if want := 2 * glyphs * realMarkCoverage; got < want {
			t.Errorf("a run of %d glyphs is allowed %d, which is less than the %d a "+
				"real face could ask for", glyphs, got, want)
		}
	}
	// And it stops growing, so that a very long run cannot ask for unbounded
	// work.
	huge, longer := *markCoverageBudget(1 << 20), *markCoverageBudget(1 << 30)
	if huge != longer {
		t.Errorf("a run of 2^20 glyphs is allowed %d and one of 2^30 is allowed %d; "+
			"the allowance has no ceiling", huge, longer)
	}
	if huge <= 0 {
		t.Errorf("a very long run is allowed %d, which refuses every coverage", huge)
	}
}
