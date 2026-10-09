package shape

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// What the AAT tables cost a run, against tables that make the run do more
// than it is charged for.

// u32 appends big-endian thirty-two-bit values.
func u32(b []byte, vs ...int) []byte {
	for _, v := range vs {
		b = binary.BigEndian.AppendUint32(b, uint32(v))
	}
	return b
}

// pointsGlyf is a glyf and long loca of three glyphs: .notdef empty, glyph 1
// a composite of components references to glyph 2, and glyph 2 a square of
// four points. Glyph 1's outline is four points a component.
func pointsGlyf(components int) (glyf, loca []byte) {
	square := u16(nil, 1, 0, 0, 100, 100, 3, 0)
	square = append(square, 1, 1, 1, 1)
	square = u16(square, 0, 100, 0, -100, 0, 0, 100, 0)
	composite := u16(nil, 0xFFFF, 0, 0, 100, 100)
	for i := range components {
		flags := 0x0002 // ARGS_ARE_XY_VALUES, bytes
		if i < components-1 {
			flags |= 0x0020 // MORE_COMPONENTS
		}
		composite = append(u16(composite, flags, 2), 0, 0)
	}
	if len(composite)%2 != 0 {
		composite = append(composite, 0)
	}
	glyf = append(composite, square...)
	loca = u32(nil, 0, 0, len(composite), len(glyf))
	return glyf, loca
}

// pointsKerx is a kerx of one format 4 subtable attaching by points of the
// outlines (action 0): glyph 1 is marked at every transition, and each
// transition after the first attaches it to the mark by their first points.
// dontAdvance keeps the machine on the glyph, as long as the run's allowance
// for that lasts.
func pointsKerx(dontAdvance bool) []byte {
	const classes = 5
	flags := 0x8000
	if dontAdvance {
		flags |= 0x4000
	}
	classTable := u16(nil, 8, 1, 1, 4) // format 8: glyph 1 is class 4
	states := u16(nil, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1)
	entries := u16(nil, 0, 0, 0xFFFF, 0, flags, 0)
	actions := u16(nil, 0, 0)
	const header = 20
	at := header
	classAt := at
	at += len(classTable)
	statesAt := at
	at += len(states)
	entriesAt := at
	at += len(entries)
	actionsAt := at
	body := u32(nil, classes, classAt, statesAt, entriesAt, 0<<30|actionsAt)
	body = append(append(append(append(body, classTable...), states...), entries...), actions...)
	sub := u32(nil, 12+len(body), 4, 0)
	sub = append(sub, body...)
	return append(u32(u16(nil, 2, 0), 1), sub...)
}

// pointsFace is a face whose glyph 1, 'a', is a composite of components
// squares, positioned by pointsKerx.
func pointsFace(t testing.TB, components int, dontAdvance bool) *Face {
	t.Helper()
	glyf, loca := pointsGlyf(components)
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Points",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'b', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"glyf": glyf, "loca": loca, "kerx": pointsKerx(dontAdvance)},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if f.kerx == nil {
		t.Fatal("the kerx was not read")
	}
	return f
}

// pointsWork is the work a bounded run of text is charged on pointsFace.
func pointsWork(t *testing.T, components int, dontAdvance bool, text string) int64 {
	t.Helper()
	f := pointsFace(t, components, dontAdvance)
	r, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: text}, RunLimits{MaxWork: 1 << 40})
	if err != nil {
		t.Fatal(err)
	}
	attached := false
	for _, g := range r.Glyphs[1:] {
		attached = attached || g.XOffset != 0
	}
	if !attached {
		t.Fatalf("nothing was attached, so the points were never read: %v", r.Glyphs)
	}
	return r.Work
}

// TestAGlyphsOutlinePointsAreChargedToTheRun: kerx format 4 attaching by
// points of the outlines decodes each glyph it attaches, and a run is charged
// for that, so a glyph four times the size costs about four times as much.
// It was charged a unit a transition however large the glyph, and one
// character of a composite of 1,600 squares, which a machine stayed on, took
// thirty-seven seconds for 65,544 units.
func TestAGlyphsOutlinePointsAreChargedToTheRun(t *testing.T) {
	small, large := pointsWork(t, 1000, false, "aaaa"), pointsWork(t, 4000, false, "aaaa")
	t.Logf("%d units at 1,000 components, %d at 4,000", small, large)
	if float64(large) < 3*float64(small) {
		t.Errorf("a glyph of 4,000 components cost %d units and one of 1,000 cost %d: "+
			"decoding the outline is not charged", large, small)
	}
}

// TestAMachineStayingOnAGlyphReadsItsPointsOnce: an entry that does not
// advance keeps the machine on a glyph for four thousand transitions, each
// attaching it by its points, and the glyph is decoded once for all of them
// rather than twice at each: its size moves the run's work by what one
// decoding costs, not by sixty-five thousand of them.
func TestAMachineStayingOnAGlyphReadsItsPointsOnce(t *testing.T) {
	small, large := pointsWork(t, 100, true, "aa"), pointsWork(t, 400, true, "aa")
	t.Logf("%d units at 100 components, %d at 400", small, large)
	if float64(large) > 1.5*float64(small) {
		t.Errorf("a glyph of 400 components cost %d units and one of 100 cost %d: "+
			"the glyph is decoded at every transition", large, small)
	}
}

// TestACFFGlyphsPointsSayWhatTheyCost: the work reading a CFF glyph's points
// reports, which kerx charges to the run, is the charstring's: at least a unit
// for each point it draws.
func TestACFFGlyphsPointsSayWhatTheyCost(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", "KerxPointsCFF.otf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	read := 0
	for gid := range f.NumGlyphs() {
		points, work := f.cffContourPoints(gid)
		if len(points) == 0 {
			continue
		}
		read++
		if work < len(points) {
			t.Errorf("glyph %d has %d points and says reading them cost %d units", gid, len(points), work)
		}
	}
	if read == 0 {
		t.Fatal("no glyph had points, so this measures nothing")
	}
}
