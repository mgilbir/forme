package shape

import (
	"encoding/binary"
	"runtime"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/internal/costtest"
)

// What reading a layout table costs, against the shapes a font can take that
// cost more than their bytes.
//
// Each of these is a table that states something about a range of glyphs, or a
// count, in a few bytes, and that a reader turned into work the size of what
// was stated rather than the size of the bytes: a class table expanded into a
// map per subtable and per glyph, a coverage zero-filled up to its first index,
// a class row decoded once per glyph that shared it, a rule allocating by the
// counts it declares, a feature list walked once per tag. See the note at the
// top of layout.go's table readers.
//
// Most are pinned as the ratio of the cost at two sizes, for the reason
// runcost_test.go gives: four times the input is four times the work when it is
// linear and sixteen when it is quadratic, and a bound between them survives
// the race detector. Where the fault was a cost that follows a number the font
// states rather than the length of anything, the two sizes are two values of
// that number with everything else the same, and the bound is on how much the
// cost may follow it. And where a defence against a hostile table is also what
// lets a legitimate one be read whole — reading an aliased subtable once, a
// class row once — the test is that the table is read whole, since the
// allowance behind it would otherwise stop the read, and say so.

// costGlyphs are the glyphs every font here maps: a, b, c at 1-3 and the acute
// at 4.
var costGlyphs = []fonttest.Glyph{
	{Rune: 'a', Advance: 500, HasShape: true},
	{Rune: 'b', Advance: 500, HasShape: true},
	{Rune: 'c', Advance: 500, HasShape: true},
	{Rune: 0x0301, Advance: 0, HasShape: true},
}

func costFace(t testing.TB, extra map[string][]byte) *Face {
	t.Helper()
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs, Extra: extra}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// u16 appends big-endian sixteen-bit values.
func u16(b []byte, vs ...int) []byte {
	for _, v := range vs {
		b = binary.BigEndian.AppendUint16(b, uint16(v))
	}
	return b
}

// rangeTable is a coverage or class table of format 2 with one record: the
// glyphs first..last, at a coverage index or of a class.
func rangeTable(first, last, value int) []byte {
	return u16(nil, 2, 1, first, last, value)
}

// withLookupList puts a hand-built LookupList in place of the one a fonttest
// table ends with. fonttest writes the list last, so everything before its
// offset is the header, the ScriptList and the FeatureList, which is what the
// replacement keeps; the placeholder lookups it was built with are there only
// so that the features name indices that exist.
func withLookupList(table, list []byte) []byte {
	off := int(binary.BigEndian.Uint16(table[8:]))
	return append(append([]byte(nil), table[:off]...), list...)
}

// aliasedLookupList is a LookupList of lookups lookups, all at one offset, each
// naming subs subtable offsets that all point at one subtable.
func aliasedLookupList(kind, lookups, subs int, sub []byte) []byte {
	list := u16(nil, lookups)
	for i := 0; i < lookups; i++ {
		list = u16(list, 2+2*lookups)
	}
	list = u16(list, kind, 0, subs)
	for i := 0; i < subs; i++ {
		list = u16(list, 6+2*subs)
	}
	return append(list, sub...)
}

// tableWith is a GPOS or GSUB carrying the given LookupList under one feature
// naming lookups 0..n-1.
func tableWith(tag string, lookups int, list []byte) []byte {
	placeholder := make([]fonttest.Lookup, lookups)
	named := make([]int, lookups)
	for i := range placeholder {
		placeholder[i] = fonttest.Lookup{Type: 1}
		named[i] = i
	}
	return withLookupList(fonttest.GPOSLookups(placeholder, map[string][]int{tag: named}), list)
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

// growth fails the test when the cost at the larger size is more than limit
// times the cost at the smaller. at builds the input at a size, outside the
// timing, and returns the work to time; costtest.Time times the two, and says
// how a busy machine is kept from deciding the ratio.
func growth(t *testing.T, what string, at func(n int) func(), small, large int, limit float64) {
	t.Helper()
	c := costtest.Time(t, what, at(small), at(large))
	if c.Ratio > limit {
		t.Errorf("%s: %v against %v, a factor of %.1f where %.0f is the most the "+
			"input allows", what, c.Large, c.Small, c.Ratio, limit)
	}
}

// wideClassPairSubtable is a PairPos format 2 subtable whose coverage and first
// class table name glyphs 0..width, all of first class 0, against class2Count
// second classes of which the last is glyphs 0..width and kerns -10 — so that
// finding out whether the row says anything means reading all of it.
func wideClassPairSubtable(width, class2Count int) []byte {
	const header = 16
	records := 2 * class2Count
	cov := header + records
	cd1 := cov + 10
	cd2 := cd1 + 10
	sub := u16(nil, 2, cov, 0x0004, 0, cd1, cd2, 1, class2Count)
	for c := 0; c < class2Count; c++ {
		v := 0
		if c == class2Count-1 {
			v = -10
		}
		sub = u16(sub, v)
	}
	sub = append(sub, rangeTable(0, width, 0)...)
	sub = append(sub, rangeTable(0, width, 0)...)
	return append(sub, rangeTable(0, width, class2Count-1)...)
}

// TestAliasedPairSubtablesAreReadOnce is audit C5. A kern lookup of n subtable
// offsets that all point at one class-pair subtable was read n times, each
// expanding two class tables of the whole glyph space into maps: 844 bytes took
// ten seconds to load.
//
// A subtable repeated in one lookup can never be the one that applies, so it is
// read once. What this asserts is that such a font is read whole — no limit
// tripped — and kerns: n readings of sixteen thousand glyphs each would spend
// the table's allowance many times over, and the face would say so.
func TestAliasedPairSubtablesAreReadOnce(t *testing.T) {
	const aliases, width = 400, 16000
	list := aliasedLookupList(2, 1, aliases, wideClassPairSubtable(width, 2))
	f := costFace(t, map[string][]byte{"GPOS": tableWith("kern", 1, list)})
	if limits := f.LayoutLimits(); len(limits) != 0 {
		t.Errorf("%d offsets to one subtable spent the allowance: %q", aliases, limits)
	}
	got, _ := f.ShapeGlyphs("ab")
	if len(got) != 2 || got[0].XAdvance != 490 {
		t.Errorf("the pair did not kern: %+v", got)
	}
}

// pairSemanticsFace is a face with one kern lookup of three subtables, which
// between them state every case the order of subtables decides: an explicit
// pair of zero, a class pair that adjusts nothing, a second glyph the class
// table does not name, and a pair two subtables both state.
func pairSemanticsFace(t *testing.T) *Face {
	const a, b, c, d = 1, 2, 3, 4
	subs := [][]byte{
		fonttest.PairPosSubtable([]fonttest.KernPair{{Left: a, Right: b, Adjust: 0}}),
		fonttest.PairPosClassSubtable([]int{a, c}, map[int]int{a: 1, c: 1}, map[int]int{b: 1, a: 2}, 2, 3,
			[]fonttest.ClassPair{{Class1: 1, Class2: 1, Adjust: -50}, {Class1: 1, Class2: 0, Adjust: -70}}),
		fonttest.PairPosClassSubtable([]int{a, c}, map[int]int{a: 1, c: 1}, map[int]int{b: 1, a: 1}, 2, 2,
			[]fonttest.ClassPair{{Class1: 1, Class2: 1, Adjust: -30}}),
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "Pairs",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true}, {Rune: 'b', Advance: 500, HasShape: true},
			{Rune: 'c', Advance: 500, HasShape: true}, {Rune: 'd', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"GPOS": fonttest.GPOSLookups(
			[]fonttest.Lookup{{Type: 2, Subtables: subs}}, map[string][]int{"kern": {0}})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// TestAPairIsFoundAsTheListingFoundIt pins that searching the subtables for a
// pair answers what listing them into a map answered, case by case — which
// subtable wins, and what counts as a subtable naming a pair at all.
//
// Two of the cases are this engine's reading and not the specification's, and
// they are kept as they were because changing them changes what real fonts
// kern: a class pair that adjusts nothing does not stop the search (by the
// specification it matches, and applies nothing), and a second glyph the class
// table does not name is not paired (by the specification it is class 0, and
// the class 0 column applies to it). Both are marked below.
func TestAPairIsFoundAsTheListingFoundIt(t *testing.T) {
	f := pairSemanticsFace(t)
	for _, tc := range []struct {
		text string
		want float64
		why  string
	}{
		{"ab", 500, "an explicit pair of zero is a match, and the class pair after it is not reached"},
		{"cb", 450, "the first subtable to name the pair wins over the third"},
		// This engine's reading, not the specification's.
		{"ca", 470, "a class pair that adjusts nothing does not stop the search, so the third subtable applies"},
		{"cd", 500, "a second glyph the class table does not name is not paired, whatever class 0 says"},
		{"ad", 500, "nothing names d"},
	} {
		got, _ := f.ShapeGlyphs(tc.text)
		if len(got) != 2 || got[0].XAdvance != tc.want {
			t.Errorf("%q: the first glyph advances %v, want %v: %s", tc.text, got[0].XAdvance, tc.want, tc.why)
		}
	}
}

// TestAliasedPairSubtablesCostWhatTheirBytesDo is the same font as a ratio: n
// offsets to a subtable naming 16n glyphs, at n and 4n. With the subtable read
// once per offset the work is n times 16n, sixteen times over for four times
// the font; read once it is the glyphs it names.
func TestAliasedPairSubtablesCostWhatTheirBytesDo(t *testing.T) {
	load := func(n int) func() {
		width := 16 * n
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GPOS": tableWith("kern", 1,
				aliasedLookupList(2, 1, n, wideClassPairSubtable(width, 2)))}})
		return func() {
			if _, err := Load(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	growth(t, "loading n offsets to one subtable naming 16n glyphs, at 4n against n",
		load, 250, 1000, 8)
}

// TestAClassRowIsReadOnceNotPerGlyph is audit C6. A class-pair subtable whose
// coverage names every glyph, all of one first class, against four thousand
// second classes, re-decoded the class's row for every covered glyph: eight
// kilobytes of GPOS took seven seconds to load.
//
// The row is read once. Read per glyph, sixty-five thousand readings of four
// thousand records would spend the allowance at once; read once, the table is
// read whole and the last glyph it covers kerns.
func TestAClassRowIsReadOnceNotPerGlyph(t *testing.T) {
	const width, classes = 0xFFFF, 4000
	list := aliasedLookupList(2, 1, 1, wideClassPairSubtable(width, classes))
	f := costFace(t, map[string][]byte{"GPOS": tableWith("kern", 1, list)})
	if limits := f.LayoutLimits(); len(limits) != 0 {
		t.Errorf("one class row shared by every glyph spent the allowance: %q", limits)
	}
	got, _ := f.ShapeGlyphs("ab")
	if len(got) != 2 || got[0].XAdvance != 490 {
		t.Errorf("the pair did not kern: %+v", got)
	}
}

// TestChainedClassRulesDoNotExpandTheirClasses is audit C7. A chained context
// in class form carries three class tables, and all three were built into maps
// of every glyph they named before the coverage said whether any rule could
// apply — at every glyph the lookup was tried on, whether or not it covered it.
// One 60-byte subtable made each glyph cost 34 ms.
//
// A class is now searched for where it is compared, and coverage is asked
// first. The ratio is over text of k glyphs and class tables naming 64k: built
// per glyph, the work is k times 64k.
func TestChainedClassRulesDoNotExpandTheirClasses(t *testing.T) {
	face := func(width int) *Face {
		// Coverage names only c, which the text does not contain; every class
		// table names 0..width.
		const header = 14
		sub := u16(nil, 2, header, header+6, header+16, header+26, 1, 0)
		sub = append(sub, u16(nil, 1, 1, 3)...) // coverage: c
		sub = append(sub, rangeTable(0, width, 1)...)
		sub = append(sub, rangeTable(0, width, 1)...)
		sub = append(sub, rangeTable(0, width, 1)...)
		return costFace(t, map[string][]byte{"GSUB": fonttest.GSUBLookups(
			[]fonttest.Lookup{{Type: 6, Subtables: [][]byte{sub}}},
			map[string][]int{"calt": {0}})})
	}
	shape := func(k int) func() {
		f := face(min(64*k, 0xFFFF))
		text := strings.Repeat("ab", k/2)
		f.ShapeGlyphs(text)
		return func() { f.ShapeGlyphs(text) }
	}
	growth(t, "shaping k glyphs past class tables naming 64k, at 4k against k",
		shape, 250, 1000, 8)
}

// TestARuleIsCheckedAgainstItsBytesBeforeItIsMatched is audit C8. A chained
// rule states three counts, and each was allocated for before any byte behind
// it was checked: a rule set of two thousand offsets to one two-byte rule
// declaring a backtrack of 65,535 allocated half a megabyte per rule per glyph.
//
// A rule whose parts are not all in its bytes is now skipped before anything is
// matched or allocated. What is asserted is that the allocation does not follow
// the declared count: the same font declaring 4,096 and 65,535.
func TestARuleIsCheckedAgainstItsBytesBeforeItIsMatched(t *testing.T) {
	face := func(declared int) *Face {
		const rules = 50
		// ChainContext format 1: coverage of b, one rule set, rules offsets all
		// to one rule that is only its backtrack count.
		setAt := 8
		covAt := setAt + 2 + 2*rules + 2
		sub := u16(nil, 1, covAt, 1, setAt)
		sub = u16(sub, rules)
		for i := 0; i < rules; i++ {
			sub = u16(sub, 2+2*rules)
		}
		sub = u16(sub, declared)
		sub = append(sub, u16(nil, 1, 1, 2)...) // coverage: b
		return costFace(t, map[string][]byte{"GSUB": fonttest.GSUBLookups(
			[]fonttest.Lookup{{Type: 6, Subtables: [][]byte{sub}}},
			map[string][]int{"calt": {0}})})
	}
	bytes := func(declared int) uint64 {
		f := face(declared)
		text := strings.Repeat("b", 10)
		f.ShapeGlyphs(text)
		return allocated(func() { f.ShapeGlyphs(text) })
	}
	small, large := bytes(4096), bytes(65535)
	if float64(large) > 2*float64(small) {
		t.Errorf("shaping past rules declaring a backtrack of 4,096 allocated %d bytes "+
			"and 65,535 allocated %d: the allocation follows a count the font states "+
			"with nothing behind it", small, large)
	}
}

// TestACoverageStartingLateFillsNothing is audit C54. A coverage record may
// start its range at any coverage index, and the reader built a slice indexed
// by coverage index: one record naming glyph a at index 65,535 zero-filled
// sixty-five thousand entries — naming glyph 0 at every one of them — for one
// unit of allowance, per subtable that pointed at it.
//
// The coverage is now walked by its own records. What is asserted is that the
// allocation does not follow the index a record starts at, and that the glyph
// the filled slots named is not given the adjustment.
func TestACoverageStartingLateFillsNothing(t *testing.T) {
	data := func(start int) []byte {
		// SinglePos format 1: XAdvance +7 for its coverage, which is a at
		// coverage index start.
		sub := u16(nil, 1, 8, 0x0004, 7)
		sub = append(sub, rangeTable(1, 1, start)...)
		return fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GPOS": tableWith("kern", 1, aliasedLookupList(1, 1, 500, sub))}})
	}
	bytes := func(start int) uint64 {
		d := data(start)
		return allocated(func() {
			if _, err := Load(d); err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := bytes(4096), bytes(65535)
	if float64(large) > 2*float64(small) {
		t.Errorf("loading a coverage that starts at index 4,096 allocated %d bytes and "+
			"one at 65,535 allocated %d: the reader fills up to the first index", small, large)
	}
	f, err := Load(data(65535))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := f.layout.singlePos[0]; ok {
		t.Error("glyph 0 was given the adjustment: the reader named it at every index " +
			"it filled in")
	}
	if got, _ := f.ShapeGlyphs("a"); len(got) != 1 || got[0].XAdvance != 507 {
		t.Errorf("the covered glyph was not adjusted: %+v", got)
	}
}

// TestTheFeatureListIsWalkedOncePerRead is audit C55. Every distinct tag the
// font declared re-walked the whole FeatureList and rebuilt the lookup list, so
// n features with distinct tags cost n² — sixty-four kilobytes of GSUB took 1.7
// seconds, on the Load path and again per script a document sets.
func TestTheFeatureListIsWalkedOncePerRead(t *testing.T) {
	load := func(n int) func() {
		lookups := make([]fonttest.Lookup, n)
		features := map[string][]int{}
		for i := range lookups {
			// No subtables: what is being asked about is the walk of the
			// lists, and a subtable per lookup would only add to both sizes
			// alike.
			lookups[i] = fonttest.Lookup{Type: 1}
			tag := string([]byte{'z', byte('a' + i/676%26), byte('a' + i/26%26), byte('a' + i%26)})
			features[tag] = []int{i}
		}
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GSUB": fonttest.GSUBLookups(lookups, features)}})
		return func() {
			if _, err := Load(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	growth(t, "loading n features with distinct tags, at 4n against n", load, 900, 3600, 8)
}

// TestALongLookupOrderIsNotInsertionSorted is the same shape in the readers
// that put a feature's lookups in list order: an insertion sort, on the grounds
// that there are a handful. A feature may name every lookup there is, in
// descending order.
func TestALongLookupOrderIsNotInsertionSorted(t *testing.T) {
	load := func(n int) func() {
		// n lookups, all one single adjustment, named by 'kern' last first.
		named := make([]int, n)
		for i := range named {
			named[i] = n - 1 - i
		}
		placeholder := make([]fonttest.Lookup, n)
		for i := range placeholder {
			placeholder[i] = fonttest.Lookup{Type: 1}
		}
		sub := u16(nil, 1, 8, 0x0004, 1)
		sub = append(sub, u16(nil, 1, 1, 1)...)
		table := withLookupList(fonttest.GPOSLookups(placeholder, map[string][]int{"kern": named}),
			aliasedLookupList(1, n, 1, sub))
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GPOS": table}})
		return func() {
			if _, err := Load(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	growth(t, "loading a feature naming n lookups in descending order, at 4n against n",
		load, 3000, 12000, 8)
}

// TestLigaturesAreSortedOnceNotPerSubtable is the same shape in the ligature
// reader, which sorted every glyph's ligatures longest first after every
// subtable, over every list read so far: a glyph starting m ligatures, followed
// by s subtables of anything, sorted m ligatures s times.
//
// The fixture is one lookup whose one subtable gives a 4n ligatures — one
// ligature table named 4n times, two bytes each — and a second lookup of 4n
// offsets to one small subtable after it. Sorted after each subtable that is
// 16n² steps.
func TestLigaturesAreSortedOnceNotPerSubtable(t *testing.T) {
	load := func(n int) func() {
		m := 4 * n
		// LigatureSubst format 1: a's one ligature set of m offsets to one
		// ligature, a b -> c.
		big := u16(nil, 1, 0, 1, 8)
		big = u16(big, m)
		for i := 0; i < m; i++ {
			big = u16(big, 2+2*m)
		}
		big = u16(big, 3, 2, 2)
		binary.BigEndian.PutUint16(big[2:], uint16(len(big)))
		big = u16(big, 1, 1, 1) // coverage: a
		small := fonttest.LigatureSubst([]fonttest.Ligature{{Components: []int{2, 3}, Glyph: 1}})
		// Two lookups: the one big subtable, then 4n offsets to the small one.
		first := u16(nil, 4, 0, 1, 8)
		first = append(first, big...)
		second := aliasedLookupList(4, 1, 4*n, small)[4:]
		list := u16(nil, 2, 6, 6+len(first))
		list = append(append(list, first...), second...)
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GSUB": tableWith("liga", 2, list)}})
		// The fixture has to reach the reader, or this times nothing.
		if f, err := Load(data); err != nil || len(f.layout.ligatures[1]) != m || len(f.layout.ligatures[2]) != 4*n {
			t.Fatalf("the fixture was not read as it was written: %v", err)
		}
		return func() {
			if _, err := Load(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	growth(t, "loading 4n ligatures for one glyph followed by 4n subtables, at 4n against n",
		load, 750, 3000, 8)
}

// wideMarkRuleFace is a face whose GPOS applies a mark-to-base subtable through a
// contextual rule at every acute. The subtable's two coverages name glyphs
// 0..width, while its mark and base arrays hold only the records the fixture's
// glyphs need.
func wideMarkRuleFace(t testing.TB, width int) *Face {
	// MarkBasePos format 1: mark coverage, base coverage, one class, then the
	// arrays. The acute is glyph 4, so the mark array has five records; a is
	// glyph 1, so the base array has two.
	const markArray, baseArray = 12, 12 + 2 + 5*4 + 6
	baseCov := baseArray + 2 + 2*2 + 6
	markCov := baseCov + 10
	sub := u16(nil, 1, markCov, baseCov, 1, markArray, baseArray)
	sub = u16(sub, 5)
	for i := 0; i < 5; i++ {
		sub = u16(sub, 0, 2+5*4) // class 0, the anchor after the records
	}
	sub = u16(sub, 1, 100, 700) // the mark's anchor
	sub = u16(sub, 2, 2+2*2, 2+2*2)
	sub = u16(sub, 1, 250, 650) // the base's anchor
	sub = append(sub, rangeTable(0, width, 0)...)
	sub = append(sub, rangeTable(0, width, 0)...)
	rule := fonttest.SequenceContext3([][]int{{4}}, []fonttest.SeqLookup{{At: 0, Lookup: 0}})
	return costFace(t, map[string][]byte{
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
			{Type: 4, Subtables: [][]byte{sub}},
			{Type: 7, Subtables: [][]byte{rule}},
		}, map[string][]int{"kern": {1}}),
		"GDEF": fonttest.GDEF(map[int]int{1: classBase, 4: classMark}),
	})
}

// TestAMarkRuleReachedFromAContextReadsOnlyWhatItAsks is the same shape at
// shaping time. A mark subtable a contextual rule reaches is read where it is
// applied, and it was read whole at every application — both coverages into
// maps, every anchor — so a run of marks reaching a subtable whose coverages
// name the whole glyph space paid for the glyph space at each mark. A budget on
// the run was what stopped it.
//
// It is now searched: four lookups into the bytes. What is asserted is that the
// cost does not follow how many glyphs the coverages name, and that the mark is
// placed.
func TestAMarkRuleReachedFromAContextReadsOnlyWhatItAsks(t *testing.T) {
	text := "a" + strings.Repeat("́", 64)
	shape := func(width int) func() {
		f := wideMarkRuleFace(t, width)
		f.ShapeGlyphs(text)
		return func() { f.ShapeGlyphs(text) }
	}
	growth(t, "shaping 64 marks through a rule whose subtable's coverages name 65,535 "+
		"glyphs against 4,096", shape, 4096, 0xFFFF, 4)

	got, _ := wideMarkRuleFace(t, 0xFFFF).ShapeGlyphs("á")
	if len(got) != 2 || got[1].YOffset == 0 {
		t.Errorf("the mark was not placed: %+v", got)
	}
}

// TestAliasedMarkSubtablesAreReadOnce is aliasing across lookups: n mark
// lookups that all point at one subtable whose coverages name sixteen thousand
// glyphs. The anchors are the same bytes whoever names them, so the subtable
// is read once; read once per lookup, it would spend the allowance, and the
// face would say so.
func TestAliasedMarkSubtablesAreReadOnce(t *testing.T) {
	const lookups, width = 200, 16000
	// The subtable wideMarkRuleFace applies through a rule, taken out of it and
	// named here by 'mark' directly, from n lookups.
	sub := u16(nil, 1, 0, 0, 1, 12, 12+2+5*4+6)
	sub = u16(sub, 5)
	for i := 0; i < 5; i++ {
		sub = u16(sub, 0, 2+5*4)
	}
	sub = u16(sub, 1, 100, 700)
	sub = u16(sub, 2, 2+2*2, 2+2*2)
	sub = u16(sub, 1, 250, 650)
	baseCov := len(sub)
	sub = append(sub, rangeTable(0, width, 0)...)
	markCov := len(sub)
	sub = append(sub, rangeTable(0, width, 0)...)
	binary.BigEndian.PutUint16(sub[2:], uint16(markCov))
	binary.BigEndian.PutUint16(sub[4:], uint16(baseCov))

	f := costFace(t, map[string][]byte{
		"GPOS": tableWith("mark", lookups, aliasedLookupList(4, lookups, 1, sub)),
		"GDEF": fonttest.GDEF(map[int]int{1: classBase, 4: classMark}),
	})
	if limits := f.LayoutLimits(); len(limits) != 0 {
		t.Errorf("%d lookups naming one subtable spent the allowance: %q", lookups, limits)
	}
	got, _ := f.ShapeGlyphs("á")
	if len(got) != 2 || got[1].YOffset == 0 {
		t.Errorf("the mark was not placed: %+v", got)
	}

	// And one lookup naming it n times keeps it once: within a lookup the
	// first subtable that applies wins, so the copies can never apply, and
	// every copy kept is one more for every mark in every run to be tried
	// against.
	g := costFace(t, map[string][]byte{
		"GPOS": tableWith("mark", 1, aliasedLookupList(4, 1, lookups, sub)),
		"GDEF": fonttest.GDEF(map[int]int{1: classBase, 4: classMark}),
	})
	if n := len(g.layout.markBase); n != 1 {
		t.Errorf("one lookup naming a subtable %d times keeps %d copies of it, want 1", lookups, n)
	}
}

// TestGlyphClassesAreNotExpandedPerRecord is the class table GDEF states. It
// was built into a map by writing every glyph of every record, so n records
// overlapping over the same wide range cost n times its width. It is now filled
// once per glyph, however many records name it: at n records each naming 16n
// glyphs, and 4n, the work is the glyph space and not the product.
func TestGlyphClassesAreNotExpandedPerRecord(t *testing.T) {
	load := func(n int) func() {
		width := 16 * n
		cd := u16(nil, 2, n)
		for i := 0; i < n; i++ {
			// Backwards, so that no reading of the records in order is also a
			// reading in glyph order.
			cd = u16(cd, 0, width, classMark-(i%2))
		}
		gdef := u16(nil, 1, 0, 12, 0, 0, 0)
		gdef = append(gdef, cd...)
		data := fonttest.SFNT(fonttest.SFNTOptions{Name: "Cost", Glyphs: costGlyphs,
			Extra: map[string][]byte{"GDEF": gdef}})
		return func() {
			if _, err := Load(data); err != nil {
				t.Fatal(err)
			}
		}
	}
	growth(t, "loading a glyph class table of n records each naming 16n glyphs, at 4n "+
		"against n", load, 250, 1000, 8)
}

// TestTheReadingAllowanceTrippingIsReported pins that the bound says so. A
// table asking for more flattening than its size allows stops being read, and
// the face reports it rather than shaping as though it had read everything.
func TestTheReadingAllowanceTrippingIsReported(t *testing.T) {
	// SinglePos format 1 with a real adjustment over twenty thousand records
	// naming the whole glyph space: a hundred and twenty kilobytes asking for
	// 1.3 billion glyphs.
	f, err := Load(wideCoverageFont(20000))
	if err != nil {
		t.Fatal(err)
	}
	limits := f.LayoutLimits()
	if len(limits) != 1 || !strings.Contains(limits[0], "GPOS") ||
		!strings.Contains(limits[0], "not read") {
		t.Errorf("the allowance tripped and the face reports %q", limits)
	}
	// And a font well inside it reports nothing.
	if limits := costFace(t, nil).LayoutLimits(); len(limits) != 0 {
		t.Errorf("a face with no layout tables reports %q", limits)
	}
}

// TestAClassOrCoverageIsSearchedAsHarfBuzzSearchesIt pins the two searches the
// shaping path now makes instead of reading a map, on the tables where the
// answer depends on the search: ranges out of order and overlapping. HarfBuzz
// binary-searches both, so a malformed table answers as it does there rather
// than as a scan from the front would.
func TestAClassOrCoverageIsSearchedAsHarfBuzzSearchesIt(t *testing.T) {
	// Three ranges, the middle one out of order: 10..19, 40..49, 20..29.
	table := append([]byte{0, 0}, u16(nil, 2, 3, 10, 19, 1, 40, 49, 2, 20, 29, 3)...)
	for _, tc := range []struct {
		gid, class int
		named      bool
	}{
		{10, 1, true}, {19, 1, true},
		// The search goes right at 40..49 for 25, finds nothing past it and
		// stops: the out-of-order range is not reached, as in HarfBuzz.
		{25, 0, false},
		{45, 2, true},
		{9, 0, false}, {50, 0, false}, {-1, 0, false},
	} {
		if c, named := classNamed(table, 2, tc.gid); c != tc.class || named != tc.named {
			t.Errorf("class of %d is %d (named %v), want %d (%v)", tc.gid, c, named, tc.class, tc.named)
		}
		i, covered := coverageIndex(table, 2, tc.gid)
		if covered != tc.named || (covered && i != tc.class+tc.gid-[]int{0, 10, 40, 20}[tc.class]) {
			t.Errorf("coverage of %d is %d (%v)", tc.gid, i, covered)
		}
	}
	// A count past the bytes is read as far as the bytes go.
	truncated := append([]byte{0, 0}, u16(nil, 2, 9, 10, 19, 5)...)
	if c := classAt(truncated, 2, 15); c != 5 {
		t.Errorf("class of 15 in a truncated table is %d, want 5", c)
	}
	if c := classAt(truncated, 2, 30); c != 0 {
		t.Errorf("class of 30 in a truncated table is %d, want 0", c)
	}
	// Format 1, and a table GDEF would be read into.
	f1 := append([]byte{0, 0}, u16(nil, 1, 5, 3, 7, 8, 9)...)
	for gid, want := range map[int]int{4: 0, 5: 7, 6: 8, 7: 9, 8: 0} {
		if c := classAt(f1, 2, gid); c != want {
			t.Errorf("format 1: class of %d is %d, want %d", gid, c, want)
		}
		if c := readClassTable(f1, 2).of(gid); c != want {
			t.Errorf("format 1 read whole: class of %d is %d, want %d", gid, c, want)
		}
		if c := readClassTable(table, 2).of(gid + 40); c != classAt(table, 2, gid+40) {
			t.Errorf("format 2 read whole disagrees with the search at %d", gid+40)
		}
	}
	for gid := 0; gid < 60; gid++ {
		if a, b := readClassTable(table, 2).of(gid), classAt(table, 2, gid); a != b {
			t.Errorf("read whole, glyph %d is class %d; searched, %d", gid, a, b)
		}
	}
}
