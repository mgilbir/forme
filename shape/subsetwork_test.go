package shape

import (
	"context"
	"encoding/binary"
	"errors"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What one subset may cost (subsetwork.go): its work and its bytes, each
// refused with ErrSubsetLimit where it would run past.

// overlappingGlyphs is a TrueType font of n glyphs whose loca ascends and
// descends by turns — 0, L, 0, L, … — so that every even glyph is the whole of
// glyf, one simple glyph of points points, and every odd glyph ends before it
// starts.
func overlappingGlyphs(t *testing.T, n, points int) []byte {
	t.Helper()
	var gs []fonttest.Glyph
	for i := 1; i < n; i++ {
		// One mapped, so that the face has a character map to load with.
		gs = append(gs, fonttest.Glyph{Rune: 'a', Unmapped: i > 1, Advance: 500, HasShape: true})
	}
	tables := map[string][]byte{}
	for tag, b := range font.SFNTTables(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: gs})) {
		tables[tag] = b
	}
	g := []byte{0, 1, 0, 0, 0, 0, 0, 0, 0, 0}
	g = binary.BigEndian.AppendUint16(g, uint16(points-1)) // one contour
	g = append(g, 0, 0)                                    // no instructions
	for range points {
		g = append(g, 0x01) // on-curve, x and y as words
	}
	g = append(g, make([]byte, 4*points)...)
	loca := make([]byte, 4*(n+1))
	for i := 0; i <= n; i++ {
		if i%2 == 1 {
			binary.BigEndian.PutUint32(loca[4*i:], uint32(len(g)))
		}
	}
	head := append([]byte(nil), tables["head"]...)
	binary.BigEndian.PutUint16(head[50:], 1)
	tables["glyf"], tables["loca"], tables["head"] = g, loca, head
	return assembleSFNT(tables)
}

// TestOverlappingGlyphsStopAtTheSubsetsBytes: every even glyph of the font is
// the whole of glyf and sound on its own, so a subset keeping them all was
// the glyph count times the table, with nothing to stop it. It stops where the
// subset's bytes run out, and stops there: what it allocated is about the
// allowance, not the subset it was refused.
func TestOverlappingGlyphsStopAtTheSubsetsBytes(t *testing.T) {
	const n, points = 401, 4000
	data := overlappingGlyphs(t, n, points)
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if err := f.CheckGlyph(1); err == nil {
		t.Fatal("an odd glyph, which ends before it starts, was accepted; the fixture is not the one meant")
	}
	for gid := 0; gid < n; gid += 2 {
		if err := f.CheckGlyph(gid); err != nil {
			t.Fatalf("CheckGlyph(%d) = %v; the fixture is not the one meant", gid, err)
		}
		f.Use(gid)
	}
	whole := (n + 1) / 2 * 5 * points // the subset it would have been
	limit := 4 * len(data)
	if whole < 10*limit {
		t.Fatalf("the subset would be %d bytes, too few to tell from the limit of %d", whole, limit)
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, _, err = f.subset(f.newSubsetWork(context.Background(), SubsetLimits{MaxBytes: limit}))
	runtime.ReadMemStats(&after)
	if !errors.Is(err, ErrSubsetLimit) || !strings.Contains(err.Error(), "glyf") {
		t.Fatalf("a subset of %d bytes under a limit of %d returned %v, want ErrSubsetLimit", whole, limit, err)
	}
	if got := after.TotalAlloc - before.TotalAlloc; got > uint64(4*limit) {
		t.Errorf("the refused subset allocated %d bytes under a limit of %d", got, limit)
	}

	// The default is the larger of twice the program and maxCFFSize.
	if w := f.newSubsetWork(context.Background(), SubsetLimits{}); w.maxBytes != max(maxCFFSize, 2*len(data)) {
		t.Errorf("the default allowance is %d bytes, want %d", w.maxBytes, max(maxCFFSize, 2*len(data)))
	}
}

// TestASubsetsBytesAreCountedOnEveryPath: glyf, VARC written out as glyf,
// CFF and CFF2 subsets each count at least what can be seen they wrote, from
// the program they produced (writtenAtLeast), so a write that stopped being
// counted shows; an allowance one byte short of the count refuses the subset
// with ErrSubsetLimit, and one of exactly it does not.
func TestASubsetsBytesAreCountedOnEveryPath(t *testing.T) {
	glyf := fiveLetters(t, nil)
	cff, err := Load(fonttest.OTTO(fonttest.CFF(fonttest.CFFOptions{Glyphs: 3}), fonttest.SFNTOptions{Glyphs: []fonttest.Glyph{
		{Rune: 'a', Advance: 500, HasShape: true},
		{Rune: 'b', Advance: 500, HasShape: true},
	}}))
	if err != nil {
		t.Fatal(err)
	}
	cff2, err := Load(cff2Sfnt(twoGlyphs([]byte{139, 139, 21}).bytes(), aGlyph))
	if err != nil {
		t.Fatal(err)
	}
	var golden *varcGolden
	for _, g := range readVARCGolden(t) {
		if g.loc == nil && g.name == "VarComposite.ttf" {
			golden = g
		}
	}
	varc, err := Load(varcFont(t, golden))
	if err != nil {
		t.Fatal(err)
	}
	for name, f := range map[string]*Face{"glyf": glyf, "VARC": varc, "CFF": cff, "CFF2": cff2} {
		if name == "VARC" {
			for gid := range f.NumGlyphs() {
				if f.varc.t.coverageIndex(gid) >= 0 {
					f.Use(gid)
				}
			}
		} else {
			f.Encode("a")
		}
		w := f.newSubsetWork(context.Background(), SubsetLimits{})
		prog, _, err := f.subset(w)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		spent := w.maxBytes - w.bytes
		if least := writtenAtLeast(t, f, prog); spent < least {
			t.Errorf("%s: the subset counted %d bytes and wrote at least %d", name, spent, least)
		}
		if _, _, err := f.subset(f.newSubsetWork(context.Background(), SubsetLimits{MaxBytes: spent - 1})); !errors.Is(err, ErrSubsetLimit) {
			t.Errorf("%s: a subset of %d bytes under %d returned %v, want ErrSubsetLimit", name, spent, spent-1, err)
		}
		if _, _, err := f.subset(f.newSubsetWork(context.Background(), SubsetLimits{MaxBytes: spent})); err != nil {
			t.Errorf("%s: a subset of %d bytes under as many: %v", name, spent, err)
		}
	}
}

// TestASubsetsWorkIsItsLimitToo: the work running out is ErrSubsetLimit on
// the glyf path, which spent from nothing. The CFF path's is in
// TestASubsetOfSeacWalksPastTheBudgetIsRefused.
func TestASubsetsWorkIsItsLimitToo(t *testing.T) {
	f := fiveLetters(t, map[int][]byte{1: compositeOf(2, 3, 4)})
	f.Use(1)
	// .notdef and the four glyphs the composite keeps, a unit each, and a
	// unit for each of the three components it names: eight.
	if _, _, err := f.subset(f.newSubsetWork(context.Background(), SubsetLimits{MaxWork: 7})); !errors.Is(err, ErrSubsetLimit) {
		t.Errorf("a glyf subset of eight units' work under seven returned %v, want ErrSubsetLimit", err)
	}
	if _, _, err := f.subset(f.newSubsetWork(context.Background(), SubsetLimits{MaxWork: 8})); err != nil {
		t.Errorf("the same under eight units: %v", err)
	}
}

// writtenAtLeast is what a subset of f can be seen to have written from the
// program it produced: the program, and the tables it was assembled from
// before that — glyf, each VARC glyph written out before it was copied in, and
// loca; or the CFF table, or for a CFF2 face the charstrings it was written
// from, twice: once in their INDEX and once in the table.
func writtenAtLeast(t *testing.T, f *Face, prog []byte) int {
	t.Helper()
	tables := font.SFNTTables(prog)
	n := len(prog)
	if cff := tables["CFF "]; cff != nil {
		if f.cff2 == nil {
			return n + len(cff)
		}
		o, err := readCFFOutlines(cff, font.Be16(tables["maxp"], 4))
		if err != nil {
			t.Fatal(err)
		}
		for _, cs := range o.charStrings {
			n += 2 * len(cs)
		}
		return n
	}
	n += len(tables["glyf"]) + len(tables["loca"])
	if f.varc != nil {
		sub, err := Load(prog)
		if err != nil {
			t.Fatal(err)
		}
		g, err := sub.glyfTables()
		if err != nil {
			t.Fatal(err)
		}
		for gid := range f.NumGlyphs() {
			if f.varc.t.coverageIndex(gid) >= 0 && f.used[gid] {
				b, err := g.entry(gid)
				if err != nil {
					t.Fatal(err)
				}
				n += max(0, len(b)-3) // its record, less what padding it may have
			}
		}
	}
	return n
}

// countdown is a context that is done once it has been asked whether it is
// done more than left times: a cancellation that lands part way through.
type countdown struct {
	context.Context
	left int
}

func (c *countdown) Err() error {
	if c.left--; c.left < 0 {
		return context.Canceled
	}
	return nil
}

// TestASubsetStopsWhenItsContextIsDone: done before it starts, and done at
// each point it asks — between the glyphs of a glyf subset, of one whose VARC
// glyphs are written out, and inside the CFF walks, which the budget asks.
// Wherever the cancellation lands, the subset returns the context's error and
// no program, and never says it was a limit.
func TestASubsetStopsWhenItsContextIsDone(t *testing.T) {
	glyf := fiveLetters(t, map[int][]byte{1: compositeOf(2, 3, 4)})
	glyf.Use(1, 5)
	done, cancel := context.WithCancel(context.Background())
	cancel()
	if prog, _, err := glyf.SubsetGlyphsContext(done, SubsetLimits{}); !errors.Is(err, context.Canceled) || prog != nil {
		t.Errorf("a subset under a context already done returned %d bytes and %v", len(prog), err)
	}

	var golden *varcGolden
	for _, g := range readVARCGolden(t) {
		if g.loc == nil && g.name == "VarComposite.ttf" {
			golden = g
		}
	}
	varc, err := Load(varcFont(t, golden))
	if err != nil {
		t.Fatal(err)
	}
	for gid := range varc.NumGlyphs() {
		if varc.varc.t.coverageIndex(gid) >= 0 {
			varc.Use(gid)
		}
	}

	for name, f := range map[string]*Face{"glyf": glyf, "VARC": varc, "CFF": seacSubrTree(t, 40)} {
		// How many times a subset that is not cancelled asks.
		count := &countdown{context.Background(), 1 << 30}
		if _, _, err := f.SubsetGlyphsContext(count, SubsetLimits{}); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		asks := 1<<30 - count.left
		if asks < 3 {
			t.Fatalf("%s: a subset asked its context %d times", name, asks)
		}
		// The CFF walks ask through the budget as they spend it: forty
		// glyphs' seac walks of 65,536 operators each, asked every 1,024.
		if name == "CFF" && asks < 1000 {
			t.Errorf("CFF: a subset asked its context %d times; its walks did not ask as they went", asks)
		}
		for k := 0; k < asks; k += 1 + asks/60 {
			prog, _, err := f.SubsetGlyphsContext(&countdown{context.Background(), k}, SubsetLimits{})
			if !errors.Is(err, context.Canceled) || errors.Is(err, ErrSubsetLimit) || prog != nil {
				t.Fatalf("%s: a subset cancelled at the %dth of %d asks returned %d bytes and %v", name, k+1, asks, len(prog), err)
			}
		}
	}
}

// TestASubsetsLimitsAreTheCallers: zero is SubsetGlyphs' own, a negative one
// is refused, and each limit refuses a subset that needs more.
func TestASubsetsLimitsAreTheCallers(t *testing.T) {
	f := fiveLetters(t, map[int][]byte{1: compositeOf(2, 3, 4)})
	f.Use(1)
	want, _, err := f.SubsetGlyphs()
	if err != nil {
		t.Fatal(err)
	}
	got, _, err := f.SubsetGlyphsContext(context.Background(), SubsetLimits{})
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("a subset under zero limits is %d bytes (%v), and SubsetGlyphs' %d", len(got), err, len(want))
	}
	for _, l := range []SubsetLimits{{MaxWork: -1}, {MaxBytes: -1}} {
		if _, _, err := f.SubsetGlyphsContext(context.Background(), l); err == nil {
			t.Errorf("limits %+v were taken", l)
		}
	}
	for _, l := range []SubsetLimits{{MaxWork: 7}, {MaxBytes: len(want)}} {
		if _, _, err := f.SubsetGlyphsContext(context.Background(), l); !errors.Is(err, ErrSubsetLimit) {
			t.Errorf("a subset under %+v returned %v, want ErrSubsetLimit", l, err)
		}
	}
	if _, _, err := f.SubsetGlyphsContext(context.Background(), SubsetLimits{MaxWork: 8, MaxBytes: 4 * len(want)}); err != nil {
		t.Errorf("a subset under limits it fits: %v", err)
	}
}
