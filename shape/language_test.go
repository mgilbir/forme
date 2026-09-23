package shape

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// TestLanguageTagsAgreeWithHarfBuzz checks openTypeLanguages against what
// HarfBuzz's hb_ot_tags_from_script_and_language answered for the same BCP 47
// tags, recorded by testdata/harfbuzz/langtags.py.
//
// The table the two read is the same — langtags.go is generated from HarfBuzz's
// header — so what this checks is the reading: the order the rules are tried
// in, where a tag's own subtags end, the extended language subtag, the blocked
// subtags, the capitalised ISO 639-3 fallback and the "-hbot" private use. The
// tags asked about are made from the header rather than written down, so every
// entry of the table and every rule is asked about at least once.
func TestLanguageTagsAgreeWithHarfBuzz(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "langtags.expected.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()

	// The answers are HarfBuzz's at one release, and the table is HarfBuzz's at
	// one release. Compared across two, a disagreement would be a registry
	// update and not a defect, so the two have to be the same one.
	src, err := os.ReadFile("langtags.go")
	if err != nil {
		t.Fatal(err)
	}
	cases, bad := 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if v, ok := strings.CutPrefix(line, "# harfbuzz "); ok {
			if !strings.Contains(string(src), "/harfbuzz/"+v+"/src/") {
				t.Fatalf("%s is HarfBuzz %s's answers and langtags.go is not from that "+
					"release; regenerate one of them", path, v)
			}
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		lang, want, ok := strings.Cut(line, "\t")
		if !ok {
			t.Fatalf("a line with no tab: %q", line)
		}
		cases++
		got := spellTags(openTypeLanguages(lang))
		if got != want {
			bad++
			if bad <= 20 {
				t.Errorf("%q: got %q, HarfBuzz gives %q", lang, got, want)
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if cases < 10000 {
		t.Fatalf("only %d cases read from %s; the file or its reader is broken", cases, path)
	}
	if bad > 0 {
		t.Errorf("%d of %d language tags map to other language systems than HarfBuzz's", bad, cases)
	}
}

// spellTags is langtags.py's spelling of a tag list.
func spellTags(tags []string) string {
	out := make([]string, len(tags))
	for i, tag := range tags {
		printable := len(tag) == 4
		for j := 0; j < len(tag); j++ {
			if tag[j] < 0x20 || tag[j] > 0x7E || tag[j] == '|' {
				printable = false
			}
		}
		if printable {
			out[i] = tag
			continue
		}
		var v uint32
		for j := 0; j < len(tag); j++ {
			v = v<<8 | uint32(tag[j])
		}
		out[i] = fmt.Sprintf("0x%08x", v)
	}
	return strings.Join(out, "|")
}

// TestALanguageTagEndsAtACharacterNoTagHolds is the one place the reading
// differs from HarfBuzz's, and it is where HarfBuzz's own answer is an
// accident. A character that cannot be in a tag ends it — "en us" is English,
// in both — except as the first character, where HarfBuzz's parser reads on past
// the end of the string it made and answers with whatever it finds there. Here
// " en" is no language, which is what the attribute's author gets from every
// other reader of it.
func TestALanguageTagEndsAtACharacterNoTagHolds(t *testing.T) {
	for lang, want := range map[string]string{
		"en us":    "ENG ",
		"sr\u00a0": "SRB ",
		" en":      "",
		"*":        "",
		"\u00e9":   "",
	} {
		if got := spellTags(openTypeLanguages(lang)); got != want {
			t.Errorf("%q: got %q, want %q", lang, got, want)
		}
	}
}

// TestAPlanIsSharedAcrossLanguages: the language chooses the layout a plan is
// built over and asks nothing of the plan itself, so two languages a font sets
// alike — French and German in Noto Sans, which names neither — share one plan
// rather than each building and keeping their own.
func TestAPlanIsSharedAcrossLanguages(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	f = f.Clone()
	fr, de := openTypeLanguages("fr"), openTypeLanguages("de")
	l := f.layoutFor(runScript("abc"), fr)
	if l != f.layoutFor(runScript("abc"), de) {
		t.Fatal("the fixture assumption is gone: Noto Sans sets French and German by different rules")
	}
	plans := func() int {
		l.plans.mu.Lock()
		defer l.plans.mu.Unlock()
		return len(l.plans.plans)
	}
	f.ShapeGlyphsInContext("office", "", "", Features{Language: "fr"})
	before := plans()
	f.ShapeGlyphsInContext("office", "", "", Features{Language: "de"})
	if after := plans(); after != before {
		t.Errorf("shaping in German after French built %d plans more over the same layout", after-before)
	}
}

// TestAPairAcrossABoundaryIsFoundInTheRunsLanguage: the neighbour a boundary
// pair is looked up against is shaped by the run's own rules, its language
// among them. The fixture's Romanian rule turns x into y, and the font kerns y
// before z; "x" before a run "z" in Romanian is a y on the page, and the pair
// is the font's.
func TestAPairAcrossABoundaryIsFoundInTheRunsLanguage(t *testing.T) {
	const gX, gY, gZ = 1, 2, 3
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{gX}, []int{gY})}}},
		[]fonttest.Feature{{Tag: "locl", Lookups: []int{0}}},
		map[string]fonttest.Script{"latn": {
			Required: fonttest.NoFeature,
			Langs:    map[string]fonttest.LangSys{"ROM ": {Required: fonttest.NoFeature, Features: []int{0}}},
		}},
	)
	pairs := fonttest.PairPosBothSides([]fonttest.KernPair{{Left: gY, Right: gZ, Adjust: -100, SecondAdjust: -50}})
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Glyphs: []fonttest.Glyph{
			{Rune: 'x', Advance: 500, HasShape: true},
			{Rune: 'y', Advance: 500, HasShape: true},
			{Rune: 'z', Advance: 500, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": gsub,
			"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{{Type: 2, Subtables: [][]byte{pairs}}},
				map[string][]int{"kern": {0}}),
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := f.ShapeGlyphsInContext("x", "", "", Features{Language: "ro"}); len(g) != 1 || g[0].GID != gY {
		t.Fatalf("the fixture assumption is gone: x in Romanian shapes to %v", g)
	}
	whole, _ := f.ShapeGlyphsInContext("xz", "", "", Features{Language: "ro"})
	run, _ := f.ShapeGlyphsInContext("z", "x", "", Features{Language: "ro"})
	if len(run) != 1 || len(whole) != 2 || run[0].XAdvance != whole[1].XAdvance {
		t.Errorf("z after x in Romanian advances %v as a run of its own and %v in the word; "+
			"the neighbour is a y, and the pair is the font's", advances(run), advances(whole))
	}
}
