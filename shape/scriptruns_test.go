package shape

import (
	"math/rand"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/mgilbir/forme/fonttest"
)

// Audit C63: a face shapes a string that changes script as one run per script.

// TestAStringThatChangesScriptIsShapedPerScript is the rule on the fixture whose
// two rules are selected one by 'latn' and one by 'grek'. Each piece is set by
// its own script's rule, and a star, which is in no script, takes the script
// before it — or, at the start, the one after.
func TestAStringThatChangesScriptIsShapedPerScript(t *testing.T) {
	f := scriptFace(t, map[string]fonttest.Script{
		"latn": {Required: fonttest.NoFeature, Features: []int{0}},
		"grek": {Required: fonttest.NoFeature, Features: []int{1}},
	})
	for text, want := range map[string][]int{
		"α*x*":  {scAlpha, scZ, scY, scY},
		"x*α*":  {scY, scY, scAlpha, scZ},
		"*α":    {scZ, scAlpha},
		"**x":   {scY, scY, scY},
		"x**α*": {scY, scY, scY, scAlpha, scZ},
	} {
		if got := shapedGIDs(t, f, text); !equalInts(got, want) {
			t.Errorf("%q shaped to %v, want %v", text, got, want)
		}
	}
}

// TestADevanagariSuffixAfterLatinIsReordered is the audit's case, through every
// path layout shapes by. "PDFकि" in Noto Sans, which covers both scripts, was
// set as Latin: the vowel sign i, written after its consonant and drawn before
// it, was left where it was written. Its glyphs are now the ones "कि" alone
// shapes to.
func TestADevanagariSuffixAfterLatinIsReordered(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs("कि")
	alone := gidsOfGlyphs(glyphs)
	if len(alone) != 2 {
		t.Fatalf("the fixture assumption is gone: कि shapes to %v in Noto Sans", alone)
	}
	tail := func(glyphs []Glyph) []int {
		g := gidsOfGlyphs(glyphs)
		if len(g) < 2 {
			return g
		}
		return g[len(g)-2:]
	}
	glyphs, _ = f.ShapeGlyphs("PDFकि")
	if got := tail(glyphs); !equalInts(got, alone) {
		t.Errorf("ShapeGlyphs: PDFकि ends %v, and कि alone is %v", got, alone)
	}
	// The breaker measures the group whole, and the painter shapes the run with
	// the group's other runs merged in; both have to cut it the same way.
	if got := tail(f.ShapeGroup("PDFकि", "", "", true, Features{})); !equalInts(got, alone) {
		t.Errorf("ShapeGroup: PDFकि ends %v, and कि alone is %v", got, alone)
	}
	merged, _ := f.ShapeGlyphsMerged("कि", "", "", "PDF", "", true, Features{})
	if got := gidsOfGlyphs(merged); !equalInts(got, alone) {
		t.Errorf("ShapeGlyphsMerged: कि after a merged PDF is %v, and कि alone is %v", got, alone)
	}
}

// TestAMarkStaysWithWhatItIsWrittenOn: no cut falls between a character and the
// combining marks after it, whatever script a mark is from. A Devanagari vowel
// sign written on a Latin letter is part of the Latin run — which is also what
// HarfBuzz, given the string, does — and is not cut off into a run of its own,
// where it would be a broken cluster and get a dotted circle.
func TestAMarkStaysWithWhatItIsWrittenOn(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	dotted, ok := f.GlyphID(dottedCircle)
	if !ok {
		t.Fatal("the fixture assumption is gone: Noto Sans has no dotted circle")
	}
	glyphs, _ := f.ShapeGlyphs("a\u093F")
	for _, g := range glyphs {
		if g.GID == dotted {
			t.Errorf("a with a vowel sign shaped to %v, with a dotted circle: the sign was cut "+
				"from the letter it is written on", gidsOfGlyphs(glyphs))
		}
	}
	if pieces := scriptRuns("a\u093F", scriptUnknown, scriptUnknown, nil); len(pieces) != 1 {
		t.Errorf("a with a vowel sign is %d runs, want 1", len(pieces))
	}
}

// TestScriptRunsFollowTheirDefinition holds scriptRuns to its rules read the
// slow way: every unit's scripts as a set, a run the intersection of its
// units' sets, cut where that would be empty. Random strings of Latin, Greek,
// Devanagari, Tamil, Grantha, Kaithi, the two kana, Han, marks of several
// scripts, characters in no script and characters in several.
//
// And Stack.ShapeRuns, given one face and text in one direction, starts its
// runs exactly there: a face cuts a string where a stack of faces would.
func TestScriptRunsFollowTheirDefinition(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	stack := NewStack(f)
	alphabet := []rune("ab αβ कखि\u0301\u0951ひカ漢1,.-()\u200C\u200D" +
		"தக\u0BCD\u0BE7\uA8F3\u0966\u0964\u30FC\U00011315\U00011303\U00011083\U000110B0")
	rng := rand.New(rand.NewSource(63))
	for n := 0; n < 5000; n++ {
		var b strings.Builder
		for k := rng.Intn(12); k >= 0; k-- {
			b.WriteRune(alphabet[rng.Intn(len(alphabet))])
		}
		s := b.String()
		beside := []uint16{scriptUnknown, scriptOf('a'), scriptOf('漢'), scriptOf('क')}
		behind, ahead := beside[rng.Intn(len(beside))], beside[rng.Intn(len(beside))]
		want := slowScriptRuns(s, behind, ahead)
		got := scriptRuns(s, behind, ahead, nil)
		if len(got) != len(want) {
			t.Fatalf("%q between %d and %d: scriptRuns gives %v, the definition %v", s, behind, ahead, got, want)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("%q between %d and %d: scriptRuns gives %v, the definition %v", s, behind, ahead, got, want)
			}
		}
		if behind != scriptUnknown || ahead != scriptUnknown {
			continue
		}
		got = scriptRuns(s, scriptUnknown, scriptUnknown, nil)
		var starts []int
		runs, _ := stack.ShapeRuns(s)
		for _, r := range runs {
			starts = append(starts, r.Start)
		}
		var cuts []int
		for _, p := range got {
			cuts = append(cuts, p.start)
		}
		if !equalInts(starts, cuts) {
			t.Fatalf("%q: Stack.ShapeRuns starts runs at %v, and scriptRuns at %v", s, starts, cuts)
		}
	}
}

// slowScriptRuns is scriptRuns's documentation, followed literally.
func slowScriptRuns(s string, behind, ahead uint16) []scriptRun {
	type unit struct {
		start  int
		script uint16
		set    []uint16
	}
	var units []unit
	rs := []rune(s)
	at := 0
	for i := 0; i < len(rs); {
		u := unit{start: at, script: scriptOf(rs[i])}
		first := rs[i]
		at += len(string(rs[i]))
		i++
		for i < len(rs) && unicode.Is(unicode.M, rs[i]) {
			if !decides(u.script) {
				u.script, first = scriptOf(rs[i]), rs[i]
			}
			at += len(string(rs[i]))
			i++
		}
		if decides(u.script) {
			u.set = []uint16{u.script}
			if ext := scriptExtensionsOf(first); len(ext) > 0 {
				u.set = ext
			}
		}
		units = append(units, u)
	}
	in := func(x uint16, set []uint16) bool {
		for _, y := range set {
			if sameShaping(x, y) {
				return true
			}
		}
		return false
	}
	var out []scriptRun
	var cand []uint16
	var opened uint16
	start := 0
	// What comes before the string: its script, until the string says
	// something, holds the characters that say nothing.
	borrowed := decides(behind)
	if borrowed {
		cand, opened = []uint16{behind}, behind
	}
	pick := func() uint16 {
		for _, c := range cand {
			if c == opened {
				return c
			}
		}
		return cand[0]
	}
	for _, u := range units {
		if u.set == nil {
			continue
		}
		if cand == nil || (borrowed && u.start == start) {
			cand, opened, borrowed = u.set, u.script, false
			continue
		}
		borrowed = false
		var kept []uint16
		for _, c := range cand {
			if in(c, u.set) {
				kept = append(kept, c)
			}
		}
		if kept == nil {
			out = append(out, scriptRun{start: start, end: u.start, script: pick()})
			start, cand, opened = u.start, u.set, u.script
			continue
		}
		cand = kept
	}
	script := ahead
	if cand != nil {
		script = pick()
	}
	return append(out, scriptRun{start: start, end: len(s), script: script})
}

// TestACharacterOfNoScriptTakesTheScriptBesideTheRun: a run whose characters
// decide no script — a colon in a span of its own — is in the script of the
// text beside it, which is what it is when the same characters are one string.
// The fixture's star is in no script and is set by the Greek rule among Greek
// and the Latin one among Latin.
func TestACharacterOfNoScriptTakesTheScriptBesideTheRun(t *testing.T) {
	f := scriptFace(t, map[string]fonttest.Script{
		"latn": {Required: fonttest.NoFeature, Features: []int{0}},
		"grek": {Required: fonttest.NoFeature, Features: []int{1}},
	})
	for _, c := range []struct {
		s, before, after string
		want             []int
	}{
		{"*", "α", "", []int{scZ}},
		{"*", "x", "", []int{scY}},
		{"*", "", "α", []int{scZ}},
		{"*", "α,", "x", []int{scZ}},
		{"*x", "α", "", []int{scZ, scY}},
		{"*α", "x", "", []int{scY, scAlpha}},
		{"α*", "x", "", []int{scAlpha, scZ}},
	} {
		glyphs, _ := f.ShapeGlyphsInContext(c.s, c.before, c.after, Features{})
		if got := gidsOfGlyphs(glyphs); !equalInts(got, c.want) {
			t.Errorf("%q between %q and %q shaped to %v, want %v", c.s, c.before, c.after, got, c.want)
		}
		// The same through the paths that merge a neighbour in, which have
		// to cut the same string the same way.
		whole := c.before + c.s + c.after
		group := f.ShapeGroup(whole, "", "", true, Features{})
		var mine []int
		for _, g := range group {
			if g.Cluster >= len(c.before) && g.Cluster < len(c.before)+len(c.s) {
				mine = append(mine, g.GID)
			}
		}
		if !equalInts(mine, c.want) {
			t.Errorf("%q inside %q shaped to %v, want %v", c.s, whole, mine, c.want)
		}
	}
	// And a run whose neighbour after it is merged in, with plain context
	// before: the star is between Greek and the Latin y it is shaped with, and
	// is Greek.
	merged, _ := f.ShapeGlyphsMerged("*", "α", "y", "", "y", true, Features{})
	if got := gidsOfGlyphs(merged); !equalInts(got, []int{scZ}) {
		t.Errorf("* after α, shaped with a merged y, is %v; want the Greek rule's %d", got, scZ)
	}
}

// TestACharacterOfSeveralScriptsContinuesTheRun is the half of UAX #24 that
// Script alone gets wrong, on the cases the oracle sweep over Google Fonts
// found when a run was cut by Script: each is one run.
func TestACharacterOfSeveralScriptsContinuesTheRun(t *testing.T) {
	for _, c := range []struct{ what, s string }{
		{"a Devanagari digit in Kaithi", "\U00011083\u096B"},
		{"a Tamil digit in Grantha", "\U00011315\u0BEA\U00011302"},
		{"Grantha opening with a Tamil digit", "\u0BEA\U00011315"},
		{"a Vedic sign in Tamil", "\u0BB0\uA8F3"},
		{"an Arabic full stop in Hanifi Rohingya", "\U00010D1C\u06D4"},
		{"a Bengali digit in Chakma", "\U00011119\u09E8"},
		{"Hiragana and Katakana", "ひらカタ"},
	} {
		if got := scriptRuns(c.s, scriptUnknown, scriptUnknown, nil); len(got) != 1 {
			t.Errorf("%s: %q is cut into %v", c.what, c.s, got)
		}
	}
	// And the script it is set in is the one it opened with, where that is
	// still possible, and otherwise the one the rest of it decided.
	if got := scriptRuns("\u0BEA\U00011315", scriptUnknown, scriptUnknown, nil)[0].script; got != scriptOf(0x11315) {
		t.Errorf("Grantha opening with a Tamil digit is set as script %d, want Grantha's %d",
			got, scriptOf(0x11315))
	}
	if got := scriptRuns("\u0BEA", scriptUnknown, scriptUnknown, nil)[0].script; got != scriptOf(0x0BEA) {
		t.Errorf("a Tamil digit alone, which Grantha also writes, is set as script %d, want Tamil's %d",
			got, scriptOf(0x0BEA))
	}
	if got := scriptRuns("\u0BB0\uA8F3", scriptUnknown, scriptUnknown, nil)[0].script; got != scriptOf(0x0BB0) {
		t.Errorf("Tamil with a Vedic sign is set as script %d, want Tamil's %d", got, scriptOf(0x0BB0))
	}
}

// TestScriptsOneRunApartShapeAlike is what lets sameShaping merge two scripts:
// scripts looked up under the same tags are set by the same model, whatever
// tag the font chose for them.
func TestScriptsOneRunApartShapeAlike(t *testing.T) {
	chosen := []string{"", "DFLT", "latn", "kana", "hani", "dev2", "dev3", "deva", "mymr", "mym2", "arab", "thai"}
	pairs := 0
	for a := 0; a < len(scriptOpenTypeTags); a++ {
		for b := a + 1; b < len(scriptOpenTypeTags); b++ {
			if !sameShaping(uint16(a), uint16(b)) {
				continue
			}
			pairs++
			for _, c := range chosen {
				if ma, mb := categorize(uint16(a), c), categorize(uint16(b), c); ma != mb {
					t.Errorf("scripts %d and %d are one run, and under %q one is set by model %d "+
						"and the other by %d", a, b, c, ma, mb)
				}
			}
		}
	}
	if pairs == 0 {
		t.Fatal("no two scripts share tags; Hiragana and Katakana both being 'kana' was the case")
	}
}

// TestNoPairIsKernedAcrossAScriptCut: a script change ends a run of the font's
// rules, so the pair a font states between a Latin and a Greek letter is not
// applied between the two runs — to either side of it — as Stack.ShapeRuns does
// not apply it. The same pair inside one script still is.
func TestNoPairIsKernedAcrossAScriptCut(t *testing.T) {
	const gX, gAlpha, gBeta = 1, 2, 3
	pairs := fonttest.PairPosBothSides([]fonttest.KernPair{
		{Left: gX, Right: gAlpha, Adjust: -100, SecondAdjust: -50},
		{Left: gBeta, Right: gAlpha, Adjust: -100, SecondAdjust: -50},
	})
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'x', Advance: 500, HasShape: true},
			{Rune: 'α', Advance: 500, HasShape: true},
			{Rune: 'β', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{"GPOS": fonttest.GPOSLookups(
			[]fonttest.Lookup{{Type: 2, Subtables: [][]byte{pairs}}},
			map[string][]int{"kern": {0}})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, via := range []struct {
		name  string
		shape func(string) []Glyph
	}{
		{"ShapeGlyphs", func(s string) []Glyph { g, _ := f.ShapeGlyphs(s); return g }},
		{"ShapeGroup", func(s string) []Glyph { return f.ShapeGroup(s, "", "", true, Features{}) }},
	} {
		across := via.shape("xα")
		if len(across) != 2 || across[0].XAdvance != 500 || across[1].XAdvance != 500 {
			t.Errorf("%s: x and α advance %v; the pair spans a change of script and "+
				"is not the font's to apply, to either of them", via.name, advances(across))
		}
		within := via.shape("βα")
		if len(within) != 2 || within[0].XAdvance != 400 || within[1].XAdvance != 450 {
			t.Errorf("%s: β and α advance %v, want the pair's 400 and 450", via.name, advances(within))
		}
	}
}

func advances(glyphs []Glyph) []float64 {
	out := make([]float64, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.XAdvance
	}
	return out
}

// TestARightToLeftRunInTwoScriptsIsDrawnInOrder: the pieces of a right-to-left
// run come back in the order they are drawn, which is the last piece first.
func TestARightToLeftRunInTwoScriptsIsDrawnInOrder(t *testing.T) {
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'א', Advance: 500, HasShape: true},
			{Rune: 'ב', Advance: 500, HasShape: true},
			{Rune: 'ء', Advance: 500, HasShape: true},
			{Rune: 'ا', Advance: 500, HasShape: true},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	s := "אבءا"
	if n := len(scriptRuns(s, scriptUnknown, scriptUnknown, nil)); n != 2 {
		t.Fatalf("the fixture assumption is gone: %q is %d runs", s, n)
	}
	glyphs, _ := f.ShapeGlyphs(s)
	var clusters []int
	for _, g := range glyphs {
		clusters = append(clusters, g.Cluster)
	}
	// Right to left: the last character written is drawn leftmost.
	if want := []int{len("אבء"), len("אב"), len("א"), 0}; !equalInts(clusters, want) {
		t.Errorf("%q is drawn from characters at %v, want %v", s, clusters, want)
	}
}

// TestAStringChangingScriptOftenCostsWhatItsTextDoes: a string that changes
// script every character is a run per character, and each run is handed the
// ends of the string either side as context. What that costs has to be in
// proportion to the string — a Japanese sentence changes script every few
// characters.
func TestAStringChangingScriptOftenCostsWhatItsTextDoes(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	for _, unit := range []string{"aα", "a α "} {
		shape := func(n int) time.Duration {
			text := strings.Repeat(unit, n)
			f.ShapeGlyphs(text)
			return best(func() { f.ShapeGlyphs(text) })
		}
		growth(t, "shaping "+unit+" n times, at 4n against n", shape, 500, 2000, 8)
	}
}
