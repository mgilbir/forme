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

// TestScriptTagsAgreeWithHarfBuzz checks the script tags a run is looked up
// under in each language against what HarfBuzz answered, recorded by
// testdata/harfbuzz/scripttags.py for a Latin run and a Devanagari one. What a
// language tag can change about them is HarfBuzz's private-use "-hbsc", which
// names a script tag in place of the run's own; the tags asked about spell it
// every way HarfBuzz reads and refuses.
//
// It was not read, so "und-x-hbscdflt" set Devanagari by the font's 'dev2'
// rules and the Indic model, where HarfBuzz sets it by 'DFLT' and the default
// model. 326 of the 1,860 tags here name a script HarfBuzz reads, and without
// it 595 of the 3,720 answers differed from HarfBuzz's.
func TestScriptTagsAgreeWithHarfBuzz(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "scripttags.expected.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	src, err := os.ReadFile("langtags.go")
	if err != nil {
		t.Fatal(err)
	}
	scripts := map[string]uint16{"Latn": runScript("a"), "Deva": runScript("\u0915")}
	var order []string
	cases, bad, overridden := 0, 0, 0
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
		if v, ok := strings.CutPrefix(line, "# scripts "); ok {
			order = strings.Fields(v)
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(order) == 0 || len(fields) != 1+len(order) {
			t.Fatalf("a line that is not a tag and %d answers: %q", len(order), line)
		}
		cases++
		lang := openTypeLanguage(fields[0])
		if lang.script != "" {
			overridden++
		}
		for i, name := range order {
			script, ok := scripts[name]
			if !ok {
				t.Fatalf("%s answers for %s, which this test does not know", path, name)
			}
			if got, want := spellTags(lang.scriptTags(script)), fields[1+i]; got != want {
				bad++
				if bad <= 20 {
					t.Errorf("%q, a %s run: got %q, HarfBuzz gives %q", fields[0], name, got, want)
				}
			}
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if cases < 1000 || overridden < 100 {
		t.Fatalf("%d cases read from %s, %d of them naming a script; the file or its "+
			"reader is broken", cases, path, overridden)
	}
	if bad > 0 {
		t.Errorf("%d answers of %d differ from HarfBuzz's", bad, cases*len(order))
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
	fr, de := openTypeLanguage("fr"), openTypeLanguage("de")
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

// hbscFace has rules under 'dev2' and 'latn' and none under 'DFLT': a
// Devanagari run is set by the Indic model and its 'pres' form, and a Latin
// "a" takes its 'locl' form.
func hbscFace(t *testing.T) *Face {
	t.Helper()
	const gidKa, gidI, gidKaPres, gidA, gidALocl = 1, 2, 3, 4, 5
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Hbsc",
		Glyphs: []fonttest.Glyph{
			{Rune: 'क', Advance: 600, HasShape: true},
			{Rune: 'ि', Advance: 300, HasShape: true},
			{Rune: '', Advance: 650, HasShape: true},
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: '', Advance: 550, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(
				[]fonttest.Lookup{
					{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{gidKa}, []int{gidKaPres})}},
					{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{gidA}, []int{gidALocl})}},
				},
				[]fonttest.Feature{{Tag: "pres", Lookups: []int{0}}, {Tag: "locl", Lookups: []int{1}}},
				map[string]fonttest.Script{
					"DFLT": {Required: fonttest.NoFeature},
					"dev2": {Required: fonttest.NoFeature, Features: []int{0}},
					"latn": {Required: fonttest.NoFeature, Features: []int{1}},
				}),
		},
	}))
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestTheScriptALanguageNamesIsTheOneRead is "-hbsc" through shaping: the
// script tag it names is the one whose rules are read, and the one the model is
// chosen by. What each line wants is what HarfBuzz 14.5.0 gives for the same
// font and language.
func TestTheScriptALanguageNamesIsTheOneRead(t *testing.T) {
	f := hbscFace(t)
	for _, c := range []struct {
		text, lang string
		want       []int
	}{
		// The run's own tags: 'dev2', the Indic model, the i-sign drawn first
		// and 'pres' on the consonant.
		{"कि", "", []int{2, 3}},
		{"कि", "hi", []int{2, 3}},
		{"कि", "und-x-hbscdev2", []int{2, 3}},
		{"कि", "und-x-hbsc-64657632", []int{2, 3}},
		// 'DFLT' named: no rules, and the default model, which keeps the order
		// the text is written in.
		{"कि", "und-x-hbscdflt", []int{1, 2}},
		{"कि", "hi-x-hbotmar-hbscDFLT", []int{1, 2}},
		// 'latn' named for Devanagari: Latin's rules, and still not the Indic
		// model, since a font's 'latn' rules are written for stored order.
		{"कि", "x-hbsclatn", []int{1, 2}},
		// A tag the font does not have falls back to 'DFLT'.
		{"कि", "x-hbscabcd", []int{1, 2}},
		// And the Latin run: its 'locl' form, and not under 'DFLT'.
		{"a", "", []int{5}},
		{"a", "en-x-hbscdflt", []int{4}},
		{"a", "x-hbscdev2", []int{4}},
	} {
		g, _ := f.ShapeGlyphsInContext(c.text, "", "", Features{Language: c.lang})
		var got []int
		for _, x := range g {
			got = append(got, x.GID)
		}
		if !sameGIDs(got, c.want) {
			t.Errorf("%q in %q is drawn as %v, want %v", c.text, c.lang, got, c.want)
		}
	}
}
