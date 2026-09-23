package shape

import (
	"encoding/binary"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/font"
	"github.com/mgilbir/forme/fonttest"
)

// What shaping costs per character, against the text shapes that made it grow
// with the length of the run instead: a letter carrying a long run of marks, a
// cluster the grammar lets grow without end, a string cut into a run per digit.
// Each is pinned as a ratio of the cost at n and at 4n, for the reason
// runcost_test.go gives, using the helpers tablecost_test.go keeps (best,
// growth).

// corpusFace loads one of the fonts the HarfBuzz oracle is run over.
func corpusFace(t *testing.T, name string) *Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(harfbuzzDir, "fonts", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading %s: %v", name, err)
	}
	return f
}

// TestALongUniversalClusterIsNotReorderedQuadratically is audit C52. The
// universal engine's grammar reads a letter followed by any number of pre-base
// vowel signs as one cluster, and each sign was rotated to the front of it in
// turn: "ᬓ" with sixty-four thousand U+1B3E after it took fifteen seconds. A
// cluster is now cut at maxIndicSyllable, as the other syllabic scanners cut
// theirs.
//
// The ratio is taken over the two steps that are the cost — cutting the run
// into clusters and reordering each — rather than over the whole of shaping.
// Through a font the quadratic only shows past eight thousand signs, where the
// rest of shaping stops hiding it (Noto Sans Balinese, before this: 8,000 in
// 0.33 s and 32,000 in 5.3 s), and a test that size is too slow to run on
// every change. Without the font it shows at a tenth of that.
func TestALongUniversalClusterIsNotReorderedQuadratically(t *testing.T) {
	reorder := func(n int) time.Duration {
		runes := []rune("ᬓ" + strings.Repeat("ᬾ", n))
		info := make([]useInfo, len(runes))
		for i, r := range runes {
			info[i].cat, info[i].pos = useCategoryOf(r)
			info[i].mark = isCombiningMark(r)
		}
		buf := make([]Glyph, len(runes))
		return best(func() {
			for _, c := range useClusters(info) {
				reorderUseCluster(buf, info, c.start, c.end)
			}
		})
	}
	growth(t, "cutting and reordering a letter and n pre-base vowel signs, at 4n against n",
		reorder, 2000, 8000, 8)
}

// TestAUniversalClusterIsCutAtTheBound pins where the cut falls and that it is
// in the grammar: a cluster longer than the bound becomes clusters of the bound,
// and a real one is untouched.
func TestAUniversalClusterIsCutAtTheBound(t *testing.T) {
	long := useClustersOf("ᬓ" + strings.Repeat("ᬾ", 3*maxIndicSyllable))
	if len(long) < 3 {
		t.Fatalf("a letter and %d vowel signs came to %d clusters; it should be cut",
			3*maxIndicSyllable, len(long))
	}
	covered := 0
	for i, c := range long {
		if c.end-c.start > maxIndicSyllable {
			t.Errorf("cluster %d runs %d..%d, longer than %d", i, c.start, c.end, maxIndicSyllable)
		}
		if c.start != covered {
			t.Errorf("cluster %d starts at %d, want %d: the clusters must cover the run", i, c.start, covered)
		}
		covered = c.end
	}
	if covered != 1+3*maxIndicSyllable {
		t.Errorf("the clusters cover %d characters of %d", covered, 1+3*maxIndicSyllable)
	}
	if short := useClustersOf("ᬓᬾᬾ"); len(short) != 1 {
		t.Errorf("a letter and two vowel signs came to %d clusters, want 1", len(short))
	}
}

// TestARunPerDigitCostsWhatItsTextDoes is audit C53. A bidirectional string is
// cut into a run per stretch of digits, and each run looked for its script by
// walking everything before it, and was handed everything before and after it
// as context by concatenating both: "ب1" sixteen thousand times over took four
// and a half seconds.
func TestARunPerDigitCostsWhatItsTextDoes(t *testing.T) {
	f := corpusFace(t, "NotoSansArabic.ttf")
	for _, unit := range []string{"ب1", "ب 1 "} {
		shape := func(n int) time.Duration {
			text := strings.Repeat(unit, n)
			f.ShapeGlyphs(text)
			return best(func() { f.ShapeGlyphs(text) })
		}
		growth(t, "shaping "+unit+" n times, at 4n against n", shape, 1000, 4000, 8)
	}
}

// TestAContextCostsWhatIsReadOfIt is the same shape one level up. A caller hands
// a run the text either side of it, and the whole of that text was decoded to
// find its last few characters, so a short run set against a long neighbour paid
// for the neighbour. The ratio is of one run against contexts of n and 4n
// characters, and what it may grow by is nothing much.
func TestAContextCostsWhatIsReadOfIt(t *testing.T) {
	f := corpusFace(t, "NotoSansArabic.ttf")
	shape := func(n int) time.Duration {
		before, after := strings.Repeat("ب", n), strings.Repeat("ب", n)
		f.ShapeGlyphsInContext("بل", before, after, Features{})
		return best(func() { f.ShapeGlyphsInContext("بل", before, after, Features{}) })
	}
	growth(t, "shaping one run against n characters of context, at 4n against n", shape, 250000, 1000000, 2)
}

// TestScriptsAroundAnswersAsOnePieceAtATimeDid holds scriptsAround to the
// definition it replaced, over random strings cut into random pieces: a piece's
// own script, else the last one behind it, else the first one after.
func TestScriptsAroundAnswersAsOnePieceAtATimeDid(t *testing.T) {
	one := func(s string, start, end int) uint16 {
		if sc := runScript(s[start:end]); sc != scriptUnknown {
			return sc
		}
		last := uint16(scriptUnknown)
		for _, r := range s[:start] {
			if sc := scriptOf(r); decides(sc) {
				last = sc
			}
		}
		if last != scriptUnknown {
			return last
		}
		return runScript(s[end:])
	}
	alphabet := []rune("ab بل 12١א́.,क")
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 2000; trial++ {
		var b strings.Builder
		for i, n := 0, rng.Intn(20); i < n; i++ {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		s := b.String()
		// Cut at character boundaries into pieces, then shuffle them, since the
		// runs arrive in visual order.
		var cuts []int
		for i := range s {
			if i > 0 && rng.Intn(3) == 0 {
				cuts = append(cuts, i)
			}
		}
		cuts = append(append([]int{0}, cuts...), len(s))
		var pieces [][2]int
		for i := 0; i+1 < len(cuts); i++ {
			pieces = append(pieces, [2]int{cuts[i], cuts[i+1]})
		}
		rng.Shuffle(len(pieces), func(i, j int) { pieces[i], pieces[j] = pieces[j], pieces[i] })
		got := scriptsAround(s, pieces)
		for i, p := range pieces {
			if want := one(s, p[0], p[1]); got[i] != want {
				t.Fatalf("%q, piece %d..%d: script %d, want %d", s, p[0], p[1], got[i], want)
			}
		}
	}
}

// TestAContextKeepsTheEndsThatAreRead pins what contextBefore and contextAfter
// keep: the ends of the concatenation, as many characters as contextRunes, the
// joining scan's answer unchanged, and no character cut in half.
func TestAContextKeepsTheEndsThatAreRead(t *testing.T) {
	long := strings.Repeat("ب", 3*contextRunes)
	for _, tc := range []struct{ outer, inner string }{
		{"", ""}, {"ab", "cd"}, {long, "x"}, {"x", long}, {long, long},
		{"ب" + strings.Repeat("َ", 10), "ََ"},
	} {
		whole := tc.outer + tc.inner
		want := lastRunes(whole, contextRunes)
		if got := contextBefore(tc.outer, tc.inner); got != want {
			t.Errorf("contextBefore(%d, %d runes) = %q, want %q", len([]rune(tc.outer)), len([]rune(tc.inner)), got, want)
		}
		after := tc.inner + tc.outer
		if got, want := contextAfter(tc.inner, tc.outer), firstRunes(after, contextRunes); got != want {
			t.Errorf("contextAfter = %q, want %q", got, want)
		}
		// What the joining scan reads of the trimmed context is what it read of
		// the whole, while the marks fit in the window.
		gb, ga := shapeContext{before: want}.runes()
		wb, wa := shapeContext{before: whole}.runes()
		if string(gb) != string(wb) || string(ga) != string(wa) {
			t.Errorf("the joining scan reads %q of the trimmed context and %q of the whole", string(gb), string(wb))
		}
	}
}

// TestLastRunesIsTheEndOfTheString holds the backwards walk to the forwards one
// it replaced, including over bytes that are not UTF-8.
func TestLastRunesIsTheEndOfTheString(t *testing.T) {
	for _, s := range []string{"", "a", "日本語テキスト", "ab\xe3\x81cd", "\xff\xfe", "\xe3\x81\x81\x81x", "áb\U0001F600"} {
		var starts []int
		for i := range s {
			starts = append(starts, i)
		}
		for n := 0; n <= len(starts)+1; n++ {
			want := s
			switch {
			case n == 0:
				want = ""
			case n < len(starts):
				want = s[starts[len(starts)-n]:]
			}
			if got := lastRunes(s, n); got != want {
				t.Errorf("lastRunes(%q, %d) = %q, want %q", s, n, got, want)
			}
		}
	}
}

// markStackFace has one mark-to-mark lookup that looks only at marks of
// attachment class 2, and GDEF puts the acute in class 1: every acute is a mark
// the lookup steps over.
func markStackFace(t *testing.T) *Face {
	const a, acute = 1, 4
	mkmk := fonttest.MarkAttachSubtable(
		[]fonttest.MarkAttachment{{Glyph: acute, Class: 0, Anchor: fonttest.Anchor{X: 100, Y: 700}}},
		[]fonttest.BaseAttachment{{Glyph: acute, Anchors: map[int]fonttest.Anchor{0: {X: 100, Y: 900}}}},
	)
	gdef := fonttest.GDEF(map[int]int{a: classBase, acute: classMark})
	// A mark attachment class definition, which fonttest's GDEF leaves out:
	// appended, and its offset written into the header.
	markAttach := u16(nil, 2, 1, acute, acute, 1)
	binary.BigEndian.PutUint16(gdef[10:], uint16(len(gdef)))
	gdef = append(gdef, markAttach...)
	return costFace(t, map[string][]byte{
		"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{{Type: 6, Flag: 0x0200, Subtables: [][]byte{mkmk}}},
			map[string][]int{"mkmk": {0}}),
		"GDEF": gdef,
	})
}

// TestMarkToMarkDoesNotWalkBackOverTheMarksItIgnores is audit C77. Mark-to-mark
// looked for the mark a mark stacks on by walking back over every glyph its
// lookup steps over, and a letter carrying a long run of marks outside the
// lookup's attachment class made each of them walk the whole run: "a" with
// sixteen thousand U+0301 took 2.8 seconds and climbed by four and a half per
// doubling.
func TestMarkToMarkDoesNotWalkBackOverTheMarksItIgnores(t *testing.T) {
	f := markStackFace(t)
	if len(f.layout.markMark) == 0 {
		t.Fatal("the fixture's mark-to-mark lookup was not read; the test would time nothing")
	}
	shape := func(n int) time.Duration {
		text := "a" + strings.Repeat("́", n)
		f.ShapeGlyphs(text)
		return best(func() { f.ShapeGlyphs(text) })
	}
	growth(t, "shaping a and n marks its mark-to-mark lookup ignores, at 4n against n", shape, 2000, 8000, 8)
}

// TestTheMarkStackTrackerFindsWhatTheWalkFound holds the tracker to the walk it
// replaced, over random runs of bases and marks of two attachment classes, two
// of them in a filtering set, against subtables of every flag combination.
func TestTheMarkStackTrackerFindsWhatTheWalkFound(t *testing.T) {
	const base, m1, m2, m3 = 1, 2, 3, 4
	l := &layout{
		glyphClass: classTableOf(map[int]int{base: classBase, m1: classMark, m2: classMark, m3: classMark}),
		markAttach: classTableOf(map[int]int{m1: 1, m2: 2, m3: 1}),
		markSets:   []coverageTable{coverageOf(m1, m2)},
	}
	for _, fl := range []struct{ flags, set int }{
		{0, -1}, {0x0100, -1}, {0x0200, -1}, {flagUseMarkFilteringSet, 0},
		{flagUseMarkFilteringSet | 0x0100, 0}, {flagIgnoreMarks | 0x0200, -1}, {flagUseMarkFilteringSet, 5},
	} {
		l.markMark = append(l.markMark, markAttachment{flags: fl.flags, markSet: fl.set})
	}
	rng := rand.New(rand.NewSource(2))
	glyphs := []int{base, m1, m2, m3}
	for trial := 0; trial < 300; trial++ {
		buf := make([]Glyph, 1+rng.Intn(30))
		for i := range buf {
			buf[i].GID = glyphs[rng.Intn(len(glyphs))]
		}
		stack := newMarkStackTracker(l)
		for i := range buf {
			for k := range l.markMark {
				st := &l.markMark[k]
				j := i - 1
				for j >= 0 && l.ignoresIn(st.flags&^markStackIgnore, st.markSet, buf[j]) {
					j--
				}
				if got := stack.nearest(k); got != j {
					t.Fatalf("trial %d, glyph %d, subtable %d: the tracker gives %d and the walk %d",
						trial, i, k, got, j)
				}
			}
			stack.passed(buf[i], i)
		}
	}
}

// varStoreBytes is an item variation store of groups group offsets that all
// name one block of items rows, over two regions of one axis, with one word
// column and one byte column.
func varStoreBytes(groups, items int) []byte {
	// Header, then the offsets, then the region list, then the one block.
	head := u16(nil, 1)
	regionsAt := 8 + 4*groups
	head = binary.BigEndian.AppendUint32(head, uint32(regionsAt))
	head = u16(head, groups)
	regions := u16(nil, 1, 2, 0, 1<<14, 1<<14, 0, 1<<13, 1<<14)
	blockAt := regionsAt + len(regions)
	for i := 0; i < groups; i++ {
		head = binary.BigEndian.AppendUint32(head, uint32(blockAt))
	}
	block := u16(nil, items, 1, 2, 0, 1)
	for i := 0; i < items; i++ {
		block = u16(block, i%500-250)
		block = append(block, byte(int8(i%100-50)))
	}
	return append(append(head, regions...), block...)
}

// TestAliasedDeltaSetGroupsAreDecodedOnce is audit C56. A store's groups are
// named by offset and may all name one block, and each was decoded whole into
// a slice per row: four hundred offsets to one block of sixty thousand rows kept
// 733 MB. The rows are decoded when asked for now, and a group named twice is
// parsed once, so what reading a store allocates does not follow how many rows
// its aliased groups hold.
func TestAliasedDeltaSetGroupsAreDecodedOnce(t *testing.T) {
	bytes := func(items int) uint64 {
		t.Helper()
		b := varStoreBytes(100, items)
		return allocated(func() {
			if _, err := parseVarStore(b); err != nil {
				t.Fatal(err)
			}
		})
	}
	small, large := bytes(1000), bytes(16000)
	if float64(large) > 2*float64(small) {
		t.Errorf("a store of 100 groups naming 1,000 rows allocated %d bytes and 16,000 "+
			"rows %d: every group decodes the rows again", small, large)
	}
}

// TestADeltaSetGroupNamedTwiceIsParsedOnce is the other half: a group's region
// list is as long as its bytes allow, and it was parsed once per offset naming
// it. What reading the store allocates must not follow how many offsets name
// the one group.
func TestADeltaSetGroupNamedTwiceIsParsedOnce(t *testing.T) {
	bytes := func(groups int) uint64 {
		t.Helper()
		b := varStoreBytes(groups, 10)
		// Widen the block's region list to 4,000 entries, all naming region 0,
		// with no rows: every group that parses it allocates a list that long.
		block := len(b) - (10 + 10*3)
		b = b[:block]
		b = u16(b, 0, 0, 4000)
		b = append(b, make([]byte, 2*4000)...)
		return allocated(func() {
			if _, err := parseVarStore(b); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Sixteen times the offsets. Each costs a small entry of its own, which is
	// what a group per offset is; parsed per offset, each would cost the
	// region list again, and sixteen times the offsets would be sixteen times
	// the bytes.
	small, large := bytes(50), bytes(800)
	if float64(large) > 4*float64(small) {
		t.Errorf("50 offsets to one group allocated %d bytes and 800 offsets %d: the group "+
			"is parsed once per offset", small, large)
	}
}

// TestADeltaIsReadFromItsRow pins the arithmetic the lazy decode does, word and
// byte columns alike, at a location inside both regions.
func TestADeltaIsReadFromItsRow(t *testing.T) {
	s, err := parseVarStore(varStoreBytes(3, 600))
	if err != nil {
		t.Fatal(err)
	}
	coords := []float64{0.75}
	// Region 0 peaks at 1 from 0: 0.75. Region 1 runs 0.5..1 peaking at 1:
	// (0.75-0.5)/(1-0.5) = 0.5.
	for _, item := range []int{0, 1, 257, 599} {
		want := float64(item%500-250)*0.75 + float64(int8(item%100-50))*0.5
		for group := 0; group < 3; group++ {
			if got := s.delta(group, item, coords); got != want {
				t.Errorf("group %d item %d: delta %v, want %v", group, item, got, want)
			}
		}
	}
	if got := s.delta(0, 600, coords); got != 0 {
		t.Errorf("an item past the group gave %v, want 0", got)
	}
}

// compositeChain is a glyf table in which glyph k is a composite of glyph k-1,
// down to glyph 1, which is a simple glyph; and its loca offsets.
func compositeChain(n int) ([]uint32, []byte) {
	var glyf []byte
	offsets := make([]uint32, n+1)
	for gid := 0; gid < n; gid++ {
		offsets[gid] = uint32(len(glyf))
		switch {
		case gid == 0:
		case gid == 1:
			glyf = u16(glyf, 1, 0, 0, 10, 10, 0, 0) // one contour ending at point 0
			glyf = append(glyf, 0x01, 0, 0, 0, 0)   // one on-curve point, at the origin
		default:
			// numberOfContours -1, a bounding box, then one component: flags
			// ARGS_ARE_XY_VALUES with byte arguments, glyph gid-1, offsets zero.
			glyf = u16(glyf, 0xFFFF, 0, 0, 10, 10, 0x0002, gid-1)
			glyf = append(glyf, 0, 0)
		}
	}
	offsets[n] = uint32(len(glyf))
	return offsets, glyf
}

// TestTheCompositeClosureIsLinearInTheChain is audit C181. The closure over a
// kept glyph's components re-read every kept glyph per round and added one level
// per round, so a chain of composites cost its length squared, and a set of the
// whole font per round.
func TestTheCompositeClosureIsLinearInTheChain(t *testing.T) {
	run := func(n int) time.Duration {
		offsets, glyf := compositeChain(n)
		f := &Face{used: map[int]bool{n - 1: true}}
		keep := f.keepSet(offsets, glyf, n)
		for gid := 0; gid < n; gid++ {
			if !keep[gid] {
				t.Fatalf("a chain of %d composites lost glyph %d", n, gid)
			}
		}
		return best(func() { f.keepSet(offsets, glyf, n) })
	}
	growth(t, "closing a chain of n composites, at 4n against n", run, 4000, 16000, 8)
}

// TestComponentGlyphsWalksAsMarkCompositeDoes holds the worklist's reader of a
// composite to font.MarkComposite, the one every other caller shares, over every
// glyph of the bundled face and over glyphs of random bytes.
func TestComponentGlyphsWalksAsMarkCompositeDoes(t *testing.T) {
	check := func(g []byte, n int) {
		t.Helper()
		want := make([]bool, n)
		font.MarkComposite(g, n, want)
		got := make([]bool, n)
		for _, c := range componentGlyphs(g, n, nil) {
			got[c] = true
		}
		for i := range want {
			if want[i] != got[i] {
				t.Fatalf("glyph %x: component %d marked %v by MarkComposite and %v here", g, i, want[i], got[i])
			}
		}
	}
	data := notoSansBytes(t)
	tables := font.SFNTTables(data)
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	n := f.NumGlyphs()
	long := binary.BigEndian.Uint16(tables["head"][50:]) == 1
	offsets, err := parseLoca(tables["loca"], n, long)
	if err != nil {
		t.Fatal(err)
	}
	composites := 0
	for gid := 0; gid < n; gid++ {
		g := tables["glyf"][offsets[gid]:offsets[gid+1]]
		if len(g) >= 2 && int16(binary.BigEndian.Uint16(g)) == -1 {
			composites++
		}
		check(g, n)
	}
	if composites == 0 {
		t.Fatal("the face has no composite glyphs; the comparison would prove nothing")
	}
	rng := rand.New(rand.NewSource(3))
	for trial := 0; trial < 5000; trial++ {
		g := make([]byte, rng.Intn(40))
		rng.Read(g)
		if len(g) >= 2 && rng.Intn(2) == 0 {
			g[0], g[1] = 0xFF, 0xFF
		}
		check(g, 1+rng.Intn(300))
	}
}
