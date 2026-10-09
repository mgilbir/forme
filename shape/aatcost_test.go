package shape

import (
	"context"
	"encoding/binary"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/internal/costtest"
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
// advance keeps the machine on a glyph for thousands of transitions, each
// attaching it by its points, and the glyph is decoded once for all of them
// rather than twice at each: its size moves the time the run takes by what
// one decoding costs, not by thousands of them.
//
// The machine stays until the run's allowance is spent, where HarfBuzz gives
// up on the run, and a bounded call is refused for it; so the run is timed
// unbounded, which sets it as far as the machine got.
func TestAMachineStayingOnAGlyphReadsItsPointsOnce(t *testing.T) {
	small, large := pointsFace(t, 100, true), pointsFace(t, 400, true)
	if glyphs, _ := small.ShapeGlyphs("aa"); glyphs[1].XOffset == 0 {
		t.Fatalf("nothing was attached, so the points were never read: %v", glyphs)
	}
	run := func(f *Face) func() { return func() { f.ShapeGlyphs("aa") } }
	c := costtest.Time(t, "a machine staying on a glyph of n components, attaching it by its points",
		run(small), run(large))
	if c.Ratio > 2 {
		t.Errorf("a glyph of four times the components cost %s: the glyph is decoded at every transition", c)
	}
}

// TestAKerxMachineThatNeverAdvancesIsRefused: a kerx whose entry does not
// advance spends the run's allowance, as a morx's does, and HarfBuzz 14.5.0
// gives up on the run (uharfbuzz raises MemoryError for "aa" and "aaaa" on
// pointsFace's font). A bounded call is refused, naming the table; an
// unbounded one is set, and the face says what it ran out of. The same kerx
// advancing is not refused.
func TestAKerxMachineThatNeverAdvancesIsRefused(t *testing.T) {
	f := pointsFace(t, 1, true)
	_, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "aa"}, RunLimits{})
	if !errors.Is(err, ErrRunLimit) || !strings.Contains(err.Error(), "the kerx table of \"Points\" ran out of the 65536 operations") {
		t.Errorf("the bounded run returned %v", err)
	}
	if limits := f.LayoutLimits(); len(limits) != 0 {
		t.Errorf("a refused bounded run left the face reporting %q", limits)
	}
	f.ShapeGlyphs("aa")
	if limits := f.LayoutLimits(); len(limits) != 1 || !strings.Contains(limits[0], "its kerx table ran out of") {
		t.Errorf("unbounded, the face reports %q", limits)
	}
	if _, err := pointsFace(t, 1, false).ShapeGlyphsBounded(context.Background(), RunInput{Text: "aaaa"}, RunLimits{}); err != nil {
		t.Errorf("the kerx advancing was refused: %v", err)
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

// aatFonts are the fonts with a morx, mort or kerx in the tree's HarfBuzz
// fixtures, and in the Google Fonts checkout where it has been fetched.
func aatFonts(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"../testdata/harfbuzz/fonts", "../testdata/harfbuzz/aat/fonts",
		"../testdata/harfbuzz/aat-inhouse/fonts", "../testdata/googlefonts/"} {
		filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".ttf") && !strings.HasSuffix(path, ".otf") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			tables := font.SFNTTables(data)
			if tables["morx"] != nil || tables["mort"] != nil || tables["kerx"] != nil {
				files = append(files, path)
			}
			return nil
		})
	}
	return files
}

// harfBuzzGivesUp are HarfBuzz's fixtures of a morx whose machine rewinds,
// inserts or stays without end, and morx_fixture.py's MorxRunaway, on whose
// characters HarfBuzz 14.5.0 gives up (uharfbuzz raises MemoryError): each,
// by the length in bytes of the shortest of the two runs
// TestNoRealAATFontReachesTheAllowance sets, all of the characters as one run
// of 4 KB and that sixteen times, that HarfBuzz gives up on.
// TestMORXThirtytwo's rewinds cost more the longer the run, and HarfBuzz sets
// 8 KB of it and not 16. No other AAT font of the tree's fixtures or of
// Google Fonts is given up on, at either length.
var harfBuzzGivesUp = map[string]int{
	"TestMORXFourteen.ttf":   4096,
	"TestMORXTwentyfour.ttf": 4096,
	"TestMORXThirtyfour.ttf": 4096,
	"TestMORXThirtysix.ttf":  4096,
	"TestMORXThirtytwo.ttf":  65536,
	"MorxRunaway.ttf":        4096,
}

// givesUp says HarfBuzz gives up on a run of a font's characters n bytes
// long; see harfBuzzGivesUp.
func givesUp(path string, n int) bool {
	at, ok := harfBuzzGivesUp[filepath.Base(path)]
	return ok && n > at/2
}

// TestNoRealAATFontsTextIsRefusedForItsSubtables: charging the subtables a
// morx or kerx tries, and the glyphs each is asked about, leaves a run of
// every AAT font's characters, as long as a run may be, far inside the
// default limits: within a sixteenth of them, as TestTheDefaultLimitsAdmitOrdinaryText
// holds real text.
func TestNoRealAATFontsTextIsRefusedForItsSubtables(t *testing.T) {
	const maxInput = 4096 // RunLimits' default MaxInputBytes
	files := aatFonts(t)
	shaped, most, mostFont := 0, 0.0, ""
	for _, path := range files {
		if givesUp(path, 4096) {
			continue
		}
		data, _ := os.ReadFile(path)
		f, err := Load(data)
		if err != nil || f.morx == nil && f.kerx == nil {
			continue
		}
		var sb strings.Builder
		for r := rune(0x21); r < 0x10000 && sb.Len() < maxInput-4; r++ {
			// Not the C1 controls, nor a surrogate.
			if _, ok := f.GlyphID(r); ok && (r < 0x7F || r > 0x9F) && (r < 0xD800 || r > 0xDFFF) {
				sb.WriteRune(r)
			}
		}
		if sb.Len() == 0 {
			continue
		}
		text := strings.Repeat(sb.String(), maxInput/sb.Len())
		result, err := f.ShapeGlyphsContext(context.Background(), RunInput{Text: text, Kerns: true}, RunLimits{})
		if err != nil {
			t.Errorf("%s: %d bytes of its characters refused at the default limits: %v", filepath.Base(path), len(text), err)
			continue
		}
		if result.Work > (64<<20)/16 {
			t.Errorf("%s: %d bytes of its characters cost %d units, more than a sixteenth of the default",
				filepath.Base(path), len(text), result.Work)
		}
		shaped++
		if perByte := float64(result.Work) / float64(len(text)); perByte > most {
			most, mostFont = perByte, filepath.Base(path)
		}
	}
	// The tree's own fixtures hold more than this; fewer means the walk
	// found nothing, and a test that shapes nothing passes whatever it bounds.
	if shaped < 40 {
		t.Fatalf("only %d AAT fonts were shaped, which proves little", shaped)
	}
	t.Logf("%d AAT fonts shaped; the costliest, %s, %.0f units a byte", shaped, mostFont, most)
}

// emptyMorx is a morx of one chain of k noncontextual subtables, twelve bytes
// each, that name no glyph.
func emptyMorx(k int) []byte {
	chain := u32(nil, 1, 16+12*k, 0, k)
	for range k {
		chain = u32(chain, 12, 0x20000004, 1)
	}
	return append(u32(u16(nil, 2, 0), 1), chain...)
}

// emptyKerx is a kerx of k format 0 subtables, twelve bytes each, that kern
// no pair.
func emptyKerx(k int) []byte {
	b := u32(u16(nil, 2, 0), k)
	for range k {
		b = u32(b, 12, 0, 0)
	}
	return b
}

// TestTheSubtablesAMorxOrKerxTriesAreCharged: every subtable tried is charged,
// and every glyph it is asked whether it can start, as a GSUB subtable tried
// is. Neither was: a morx or kerx of 80,000 empty subtables, a megabyte,
// made a run of 4 KB of distinct glyphs ask each of them about every glyph,
// 1.2 seconds for the morx and 0.6 for the kerx, and was charged 9,558
// units, what the same run cost with no subtable at all.
func TestTheSubtablesAMorxOrKerxTriesAreCharged(t *testing.T) {
	for _, c := range []struct {
		tag   string
		table func(int) []byte
	}{{"morx", emptyMorx}, {"kerx", emptyKerx}} {
		work := func(k int) int64 {
			f := costFace(t, map[string][]byte{c.tag: c.table(k)})
			r, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "abcabc", Kerns: true}, RunLimits{})
			if err != nil {
				t.Fatal(err)
			}
			return r.Work
		}
		small, large := work(500), work(2000)
		t.Logf("%s: %d units at 500 subtables, %d at 2,000", c.tag, small, large)
		if float64(large) < 3*float64(small) {
			t.Errorf("a %s of 2,000 subtables cost %d units and one of 500 cost %d: the subtables tried are not charged",
				c.tag, large, small)
		}
	}
}

// trakFace is a face tracked by a trak of the given tracks, each at a value
// below the normal track, of one size, so that finding the normal track
// steps over every one; or, where tracks is negative, a twelve-byte header
// and a TrackData stating 65,535 tracks of 65,535 sizes and holding none.
func trakFace(t *testing.T, tracks int) *Face {
	t.Helper()
	trak := u16(u32(nil, 0x00010000), 0, 12, 0, 0)
	if tracks < 0 {
		trak = u32(u16(trak, 0xFFFF, 0xFFFF), 20)
	} else {
		sizes := 12 + 8 + 8*tracks
		values := sizes + 4
		trak = u32(u16(trak, tracks, 1), sizes)
		for range tracks {
			trak = u16(u32(trak, -65536), 256, values)
		}
		trak = u16(u32(trak, 12<<16), 25)
	}
	stat := u16(nil, 1, 1, 8, 0, 0, 0, 0, 0, 0, 0)
	return costFace(t, map[string][]byte{"trak": trak, "STAT": stat})
}

// trakWork is what a run on trakFace is charged, and its first glyph's
// advance.
func trakWork(t *testing.T, tracks int) (int64, float64) {
	t.Helper()
	f := trakFace(t, tracks)
	r, err := f.ShapeGlyphsBounded(context.Background(), RunInput{Text: "abc"}, RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	return r.Work, r.Glyphs[0].XAdvance
}

// TestATrakThatDoesNotFitIsNotWalked: a TrackData stating more tracks or
// sizes than its table holds is refused, as HarfBuzz's sanitizer refuses it,
// and not walked at every run. Read as zeros past the end of the table, a
// header of twenty bytes stating 65,535 tracks of 65,535 sizes was 131,070
// steps a run, and divided nought by nought for its tracking. That the
// refusal is HarfBuzz's is held by the Trak*PastEnd faces in
// TestAATPositioningAgreesWithHarfBuzz.
func TestATrakThatDoesNotFitIsNotWalked(t *testing.T) {
	work, advance := trakWork(t, -1)
	_, untracked := trakWork(t, 0)
	t.Logf("%d units, advancing %v", work, advance)
	if work > 100 {
		t.Errorf("a run on a trak stating 65,535 tracks it does not hold cost %d units", work)
	}
	if advance != untracked {
		t.Errorf("it was tracked: the first glyph advances %v, and %v untracked", advance, untracked)
	}
}

// TestTheTracksATrakWalksAreCharged: the tracks a run steps over to find the
// normal track are charged, as a subtable's rules tried are, so that a valid
// table of tens of thousands of tracks, walked at every run, is charged for
// it.
func TestTheTracksATrakWalksAreCharged(t *testing.T) {
	small, smallAdvance := trakWork(t, 1000)
	large, _ := trakWork(t, 4000)
	_, untracked := trakWork(t, 0)
	t.Logf("%d units at 1,000 tracks, %d at 4,000", small, large)
	if smallAdvance == untracked {
		t.Fatal("the run was not tracked, so the tracks were never walked")
	}
	if float64(large) < 3*float64(small) {
		t.Errorf("a trak of 4,000 tracks cost a run %d units and one of 1,000 cost %d: "+
			"the tracks walked are not charged", large, small)
	}
}

// thirtysix is HarfBuzz's fixture TestMORXThirtysix: an insertion whose
// entries never advance, so that the machine inserts a glyph after an "A" and
// is put back in front of it, for as long as the run's allowance lasts.
func thirtysix(t *testing.T) *Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "aat", "fonts", "TestMORXThirtysix.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.morx == nil || len(f.morx.chains) != 1 || len(f.morx.chains[0].subtables) != 1 ||
		f.morx.chains[0].subtables[0].kind != morxInsertion {
		t.Fatal("TestMORXThirtysix is not the one insertion it was")
	}
	return f
}

// TestAnInsertionThatNeverAdvancesCostsWhatItsAllowanceIs: the machine of
// TestMORXThirtysix, given an allowance four times as large, costs about four
// times as much. Each insertion puts the machine back in front of what it
// inserted, which moves the whole of the run behind it, and HarfBuzz charges
// the allowance for that move (shift_forward); uncharged, the run grew by a
// glyph for every four units of the allowance and moved all of itself each
// time, which is quadratic in the allowance.
func TestAnInsertionThatNeverAdvancesCostsWhatItsAllowanceIs(t *testing.T) {
	f := thirtysix(t)
	s := &f.morx.chains[0].subtables[0]
	a, ok := f.GlyphID('A')
	if !ok {
		t.Fatal("no A")
	}
	run := func(allowance int) func() {
		return func() {
			b := &aatBuf{info: []Glyph{{GID: a}}, ok: true, f: f, maxOps: allowance, maxLen: 1 << 30}
			shaper{f: f}.applyMorxSubtable(b, s, f.morx.numGlyphs)
			if len(b.info) < 2 {
				t.Fatal("nothing was inserted, so the machine never ran")
			}
		}
	}
	c := costtest.Time(t, "TestMORXThirtysix's insertion at an allowance of n", run(8192), run(4*8192))
	if c.Ratio > 8 {
		t.Errorf("four times the allowance cost %s: the glyphs a rewind shifts are not charged", c)
	}
}

// TestOneCharacterOfARunawayMorxIsQuick: one character set in
// TestMORXThirtysix, which HarfBuzz gives up on in a millisecond, is set in
// well under one: the machine stops where the run's allowance, 65,536 for a
// short run, is spent. It took three seconds, the glyphs every rewind
// shifted uncharged, and the run grew to 16,385 glyphs.
func TestOneCharacterOfARunawayMorxIsQuick(t *testing.T) {
	f := thirtysix(t)
	for _, text := range []string{"A", "A\u0301"} {
		start := time.Now()
		glyphs, _ := f.ShapeGlyphs(text)
		took := time.Since(start)
		t.Logf("%q: %d glyphs in %v", text, len(glyphs), took)
		if took > 500*time.Millisecond {
			t.Errorf("%q took %v", text, took)
		}
	}
}

// aatCharacters is every character a face maps from U+0021 to U+FFFF but for
// the C1 controls and the surrogates, as many as fit in n bytes.
func aatCharacters(f *Face, n int) []rune {
	var out []rune
	size := 0
	for r := rune(0x21); r < 0x10000 && size < n-4; r++ {
		if _, ok := f.GlyphID(r); ok && (r < 0x7F || r > 0x9F) && (r < 0xD800 || r > 0xDFFF) {
			out = append(out, r)
			size += len(string(r))
		}
	}
	return out
}

// TestNoRealAATFontReachesTheAllowance: the allowance HarfBuzz gives a run's
// state machines, whose spending refuses the run (refuseAAT), is not reached
// by any AAT font of the tree's fixtures or of Google Fonts on its own
// characters: each alone, then all of them as one run of as many bytes as
// RunLimits admits, and as one run sixteen times that, which a ShapingBudget
// admits, as layout shapes a long paragraph. The most any of them spends of
// it is logged, so that a font coming near it is seen before it is refused.
// Where HarfBuzz gives up on a run (harfBuzzGivesUp), it is refused.
func TestNoRealAATFontReachesTheAllowance(t *testing.T) {
	shaped, refused, worst, worstFont, worstGF, worstGFFont := 0, 0, 0.0, "", 0.0, "none"
	for _, path := range aatFonts(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		f, err := Load(data)
		if err != nil || f.morx == nil && f.kerx == nil {
			continue
		}
		name := filepath.Base(path)
		chars := aatCharacters(f, 4096)
		if len(chars) == 0 {
			continue
		}
		run := string(chars)
		long := strings.Repeat(run, 4096/len(run))
		if !givesUp(path, 4096) {
			owned := f.Clone()
			for _, r := range chars {
				if _, err := owned.ShapeGlyphsBounded(context.Background(), RunInput{Text: string(r), Kerns: true}, RunLimits{}); err != nil {
					t.Errorf("%s: %U alone was refused: %v", name, r, err)
				}
			}
		}
		for _, text := range []string{long, strings.Repeat(long, 16)} {
			budget, err := NewShapingBudget(context.Background(), RunLimits{})
			if err != nil {
				t.Fatal(err)
			}
			g := f.Clone()
			if err := budget.Bound(g); err != nil {
				t.Fatal(err)
			}
			_, err = budget.Run(func() error { g.ShapeGlyphsMerged(text, "", "", "", "", true, Features{}); return nil })
			if givesUp(path, len(text)) {
				if !errors.Is(err, ErrRunLimit) {
					t.Errorf("%s: HarfBuzz gives up on %d bytes of its characters, and they were not refused: %v", name, len(text), err)
				}
				refused++
				continue
			}
			if err != nil {
				t.Errorf("%s: %d bytes of its characters were refused: %v", name, len(text), err)
				continue
			}
			// What the run spent of the allowance, read where the run's
			// shaper left it: the last run of the text, which for a text of
			// one script is the whole of it.
			a := g.runScratch().aat
			allowance := max(a.glyphs*morxOpsPerGlyph, morxOpsFloor)
			spent := float64(allowance-a.ops) / float64(allowance)
			if spent > worst {
				worst, worstFont = spent, name
			}
			if strings.Contains(path, "googlefonts") && spent > worstGF {
				worstGF, worstGFFont = spent, name
			}
		}
		shaped++
	}
	// The tree's fixtures hold more than this; fewer means the walk found
	// nothing, and a test that shapes nothing passes whatever it bounds.
	if shaped < 40 || refused != 11 {
		t.Fatalf("%d AAT fonts were shaped and %d runs refused, which proves little", shaped, refused)
	}
	t.Logf("%d AAT fonts shaped; of the runs HarfBuzz sets, the most of the allowance spent was %.4f of it, by %s, "+
		"and in Google Fonts %.4f, by %s", shaped, worst, worstFont, worstGF, worstGFFont)
}

// stayingMorx is a morx of one contextual subtable whose entry for glyph 1
// sets the mark, which substitutes nothing, and does not advance: the machine
// stays on the glyph until the run's allowance is spent, and then moves on.
// Nothing it does to the run is charged, so it spends the allowance without
// running out of it. (Setting the mark is what puts the glyph in the
// subtable's initial set; without it the subtable is not tried.)
func stayingMorx() []byte {
	const classes = 5
	classTable := u16(nil, 8, 1, 1, 4) // format 8: glyph 1 is class 4
	states := u16(nil, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1)
	entries := u16(nil, 0, 0, 0xFFFF, 0xFFFF, 0, 0xC000, 0xFFFF, 0xFFFF)
	const header = 20
	classAt := header
	statesAt := classAt + len(classTable)
	entriesAt := statesAt + len(states)
	subsAt := entriesAt + len(entries)
	body := u32(nil, classes, classAt, statesAt, entriesAt, subsAt)
	body = append(append(append(body, classTable...), states...), entries...)
	sub := append(u32(nil, 12+len(body), 0x20000001, 1), body...)
	chain := append(u32(nil, 1, 16+len(sub), 0, 1), sub...)
	return append(u32(u16(nil, 2, 0), 1), chain...)
}

// TestAKerxHasWhatTheMorxLeftOfTheAllowance: a run's morx and kerx spend one
// allowance between them, as HarfBuzz's buffer has one max_ops for the whole
// of a shaping. A morx that stays on a glyph until the allowance is gone
// leaves the kerx none, and the run is refused at the kerx's first mark,
// where HarfBuzz 14.5.0 gives up on it too (uharfbuzz raises MemoryError for
// "aa" in this font, and sets it in a font of either table alone). Each table
// alone, on the same run, is not refused. With an allowance of its own, the
// kerx was not refused either.
func TestAKerxHasWhatTheMorxLeftOfTheAllowance(t *testing.T) {
	face := func(tables ...string) *Face {
		glyf, loca := pointsGlyf(1)
		extra := map[string][]byte{"glyf": glyf, "loca": loca}
		for _, tag := range tables {
			extra[tag] = map[string][]byte{"morx": stayingMorx(), "kerx": pointsKerx(false)}[tag]
		}
		f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
			Name:   "Staying",
			Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500, HasShape: true}, {Rune: 'b', Advance: 500, HasShape: true}},
			Extra:  extra,
		}))
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	for _, tables := range [][]string{{"morx"}, {"kerx"}} {
		if _, err := face(tables...).ShapeGlyphsBounded(context.Background(), RunInput{Text: "aa"}, RunLimits{}); err != nil {
			t.Errorf("a %s alone was refused: %v", tables[0], err)
		}
	}
	both := face("morx", "kerx")
	if both.morx == nil || both.kerx == nil {
		t.Fatal("the morx or the kerx was not read")
	}
	_, err := both.ShapeGlyphsBounded(context.Background(), RunInput{Text: "aa"}, RunLimits{})
	if !errors.Is(err, ErrRunLimit) || !strings.Contains(err.Error(), "the kerx table of") {
		t.Errorf("the run was not refused at its kerx: %v", err)
	}
}
