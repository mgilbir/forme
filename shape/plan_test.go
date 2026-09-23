package shape

import (
	"slices"
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The shaping plan, tested against fonts whose declarations the tests state.
//
// Each test is one decision the plan takes that the old arrangement took
// differently or not at all, stated as the glyphs a font with the right tables
// gives — so that the assertion is about what reaches the page and not about
// the plan's own bookkeeping. The oracle sweep over the Google Fonts tree is
// what measured each of them against HarfBuzz; these pin the mechanism.

// planFace is a face with a glyph for each rune, in order — the glyph of
// runes[i] is i+1 — and the layout tables given.
func planFace(t *testing.T, runes []rune, tables map[string][]byte) *Face {
	t.Helper()
	glyphs := make([]fonttest.Glyph, len(runes))
	for i, r := range runes {
		glyphs[i] = fonttest.Glyph{Rune: r, Advance: 500, HasShape: true}
	}
	f, err := Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: glyphs, Extra: tables}))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// planGIDs is the glyph indices of a shaped run, in visual order.
func planGIDs(glyphs []Glyph) []int {
	out := make([]int, len(glyphs))
	for i, g := range glyphs {
		out[i] = g.GID
	}
	return out
}

// onlyScript is a script list with one script selecting every feature.
func onlyScript(tag string, features int) map[string]fonttest.Script {
	return map[string]fonttest.Script{tag: fonttest.AllFeatures(features)}
}

// TestTheModelIsChosenFromTheFontsScriptTagToo is audit C42.
//
// Each font states one 'calt' ligature over a consonant and the vowel sign
// written after it. In stored order the two ligate; the script's syllable model
// puts the sign in front of the consonant first, and then they cannot. A font
// that states its rules under 'DFLT' or 'latn' — or, for Myanmar, under the
// pre-model 'mymr' — was written for the stored order, and HarfBuzz gives it the
// default model: the ligature forms. Under the script's own tag, or where the
// font chose no tag at all, the model reorders and the ligature does not form.
func TestTheModelIsChosenFromTheFontsScriptTagToo(t *testing.T) {
	const lig = 3
	for _, c := range []struct {
		what        string
		consonant   rune
		sign        rune
		script      string
		wantLigated bool
	}{
		{"Devanagari under DFLT", 0x0915, 0x093F, "DFLT", true},
		{"Devanagari under latn", 0x0915, 0x093F, "latn", true},
		{"Devanagari under dev2", 0x0915, 0x093F, "dev2", false},
		{"Devanagari under deva", 0x0915, 0x093F, "deva", false},
		{"Devanagari under a script it does not name", 0x0915, 0x093F, "grek", false},
		{"Myanmar under mymr", 0x1000, 0x1031, "mymr", true},
		{"Myanmar under DFLT", 0x1000, 0x1031, "DFLT", true},
		{"Myanmar under mym2", 0x1000, 0x1031, "mym2", false},
		{"Tai Tham under DFLT", 0x1A20, 0x1A6E, "DFLT", true},
		{"Tai Tham under lana", 0x1A20, 0x1A6E, "lana", false},
	} {
		gsub := fonttest.GSUBTable(
			[]fonttest.Lookup{{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(
				[]fonttest.Ligature{{Components: []int{1, 2}, Glyph: lig}})}}},
			[]fonttest.Feature{{Tag: "calt", Lookups: []int{0}}},
			onlyScript(c.script, 1))
		f := planFace(t, []rune{c.consonant, c.sign, 0xE000}, map[string][]byte{"GSUB": gsub})
		got, _ := f.ShapeGlyphs(string([]rune{c.consonant, c.sign}))
		ligated := slices.Equal(planGIDs(got), []int{lig})
		if ligated != c.wantLigated {
			t.Errorf("%s: glyphs %v, ligated %v, want %v", c.what, planGIDs(got), ligated, c.wantLigated)
		}
	}
}

// TestSyriacUnderDFLTIsNotJoined is the Syriac half of C42: HarfBuzz gives
// Syriac the Arabic model only where the font's rules were read under something
// other than 'DFLT'. Under 'DFLT' no letter takes a joining form.
func TestSyriacUnderDFLTIsNotJoined(t *testing.T) {
	const beth, final = 1, 2
	for _, c := range []struct {
		script string
		want   []int
	}{
		{"DFLT", []int{beth, beth}},
		{"syrc", []int{beth, final}},
	} {
		gsub := fonttest.GSUBTable(
			[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{beth}, []int{final})}}},
			[]fonttest.Feature{{Tag: "fina", Lookups: []int{0}}},
			onlyScript(c.script, 1))
		f := planFace(t, []rune{0x0712, 0xE000}, map[string][]byte{"GSUB": gsub})
		got, _ := f.ShapeGlyphs("ܒܒ")
		// Syriac is right to left; the glyphs come back in visual order.
		slices.Reverse(got)
		if !slices.Equal(planGIDs(got), c.want) {
			t.Errorf("Syriac under %s: glyphs %v, want %v", c.script, planGIDs(got), c.want)
		}
	}
}

// TestRvrnComesFirst is audit C65. 'rvrn' substitutes what a variable font
// requires at the location it was cut at, and every other rule is written
// against its output: it is a stage of its own, before everything. Here 'ccmp'
// is lookup 0 and rewrites what 'rvrn', lookup 1, produces — which it can only
// do if 'rvrn' has run first, whatever the lookup order says.
func TestRvrnComesFirst(t *testing.T) {
	const a, b, c = 1, 2, 3
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{b}, []int{c})}},
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{a}, []int{b})}},
	}, map[string][]int{"ccmp": {0}, "rvrn": {1}})
	f := planFace(t, []rune{'a', 0xE000, 0xE001}, map[string][]byte{"GSUB": gsub})
	got, _ := f.ShapeGlyphs("a")
	if !slices.Equal(planGIDs(got), []int{c}) {
		t.Errorf("glyphs %v, want [%d]: 'rvrn' then 'ccmp'", planGIDs(got), c)
	}
}

// TestAStageIsAppliedInLookupOrder. The default model's features are one stage,
// and HarfBuzz applies a stage's lookups in the order of their index. This font
// states 'liga' as lookup 0, forming fi, and 'ccmp' as lookup 1, turning i into
// a dotless i: in lookup order the ligature forms first; feature by feature,
// 'ccmp' first, it never can.
func TestAStageIsAppliedInLookupOrder(t *testing.T) {
	const f_, i_, fi, dotless = 1, 2, 3, 4
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(
			[]fonttest.Ligature{{Components: []int{f_, i_}, Glyph: fi}})}},
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{i_}, []int{dotless})}},
	}, map[string][]int{"liga": {0}, "ccmp": {1}})
	face := planFace(t, []rune{'f', 'i', 0xE000, 0xE001}, map[string][]byte{"GSUB": gsub})
	got, _ := face.ShapeGlyphs("fi")
	if !slices.Equal(planGIDs(got), []int{fi}) {
		t.Errorf("glyphs %v, want [%d]", planGIDs(got), fi)
	}
}

// TestADefaultFeatureAskedForAgainRunsOnce is audit C74. 'calt' here maps b to
// X and X to Y in one lookup: applied once, "b" is X. A document asking for
// "calt" by font-feature-settings asks for a feature the shaper applies anyway,
// and it is the same feature: applied once more, over its own output, it made Y.
func TestADefaultFeatureAskedForAgainRunsOnce(t *testing.T) {
	const b, x, y = 1, 2, 3
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{b, x}, []int{x, y})}},
	}, map[string][]int{"calt": {0}})
	f := planFace(t, []rune{'b', 0xE000, 0xE001}, map[string][]byte{"GSUB": gsub})
	for _, c := range []struct {
		what string
		off  Features
	}{
		{"by default", Features{}},
		{"asked for by tag", Features{Tags: "calt"}},
		{"asked for with others", Features{Tags: "calt,liga,ss01"}},
	} {
		got, _ := f.ShapeGlyphsInContext("b", "", "", c.off)
		if !slices.Equal(planGIDs(got), []int{x}) {
			t.Errorf("%s: glyphs %v, want [%d]", c.what, planGIDs(got), x)
		}
	}
	// And by a caller naming the tag, which is the same request.
	if got, _ := f.ShapeGlyphsWith("b", "calt"); !slices.Equal(planGIDs(got), []int{x}) {
		t.Errorf("named by a caller: glyphs %v, want [%d]", planGIDs(got), x)
	}
	// Two features naming one lookup are one piece of work as well: a font's
	// 'ss01' that reuses its 'calt' lookup, asked for alongside it.
	shared := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{b, x}, []int{x, y})}},
	}, map[string][]int{"calt": {0}, "ss01": {0}})
	f = planFace(t, []rune{'b', 0xE000, 0xE001}, map[string][]byte{"GSUB": shared})
	if got, _ := f.ShapeGlyphsInContext("b", "", "", Features{Tags: "ss01"}); !slices.Equal(planGIDs(got), []int{x}) {
		t.Errorf("one lookup named by two features: glyphs %v, want [%d]", planGIDs(got), x)
	}
}

// TestCallerNamedFeaturesAreInTheSameStage is audit C75. ShapeGlyphsWith
// applied the features a caller named after the ligatures, tag by tag, so
// "office" in small capitals kept its ffi ligature among five small capitals;
// asked for through Features it was six. The two are one request now, and give
// one answer.
func TestCallerNamedFeaturesAreInTheSameStage(t *testing.T) {
	f, err := NotoSans()
	if err != nil {
		t.Fatal(err)
	}
	named, _ := f.ShapeGlyphsWith("office", "smcp")
	asked, _ := f.ShapeGlyphsInContext("office", "", "", Features{Caps: CapsSmall})
	if !slices.Equal(planGIDs(named), planGIDs(asked)) {
		t.Errorf("named %v, asked %v: the same request gave two answers", planGIDs(named), planGIDs(asked))
	}
	if len(named) != 6 {
		t.Errorf("%d glyphs for \"office\" in small capitals, want 6 — the ffi ligature "+
			"is stated over lowercase letters, which 'smcp' comes before", len(named))
	}
}

// TestReverseChainingIsApplied is audit C76. GSUB lookup type 8 is applied from
// the end of a run back to its start, so a rule whose context is what follows
// sees it already substituted. This one turns a into a.alt before b or a.alt:
// from the end, "aab" becomes a.alt a.alt b; forwards it could only reach the
// second a. It was not applied at all.
func TestReverseChainingIsApplied(t *testing.T) {
	const a, b, alt = 1, 2, 3
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 8, Subtables: [][]byte{fonttest.ReverseChainSubst(
			[]int{a}, []int{alt}, nil, [][]int{{b, alt}})}},
	}, map[string][]int{"ccmp": {0}})
	f := planFace(t, []rune{'a', 'b', 0xE000}, map[string][]byte{"GSUB": gsub})
	got, _ := f.ShapeGlyphs("aab")
	if want := []int{alt, alt, b}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("glyphs %v, want %v", planGIDs(got), want)
	}
	// Through an extension lookup, which is how a large font states it.
	ext := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 7, Subtables: [][]byte{fonttest.ExtensionSubst(8, fonttest.ReverseChainSubst(
			[]int{a}, []int{alt}, nil, [][]int{{b, alt}}))}},
	}, map[string][]int{"ccmp": {0}})
	f = planFace(t, []rune{'a', 'b', 0xE000}, map[string][]byte{"GSUB": ext})
	if got, _ := f.ShapeGlyphs("aab"); !slices.Equal(planGIDs(got), []int{alt, alt, b}) {
		t.Errorf("through an extension: glyphs %v, want %v", planGIDs(got), []int{alt, alt, b})
	}
	// And never from inside another lookup: the format forbids it and HarfBuzz
	// refuses it, so a contextual rule naming one does nothing.
	nested := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 6, Subtables: [][]byte{fonttest.ChainedContext3(nil, [][]int{{a}}, nil,
			[]fonttest.SeqLookup{{At: 0, Lookup: 1}})}},
		{Type: 8, Subtables: [][]byte{fonttest.ReverseChainSubst(
			[]int{a}, []int{alt}, nil, [][]int{{b, alt}})}},
	}, map[string][]int{"calt": {0}})
	f = planFace(t, []rune{'a', 'b', 0xE000}, map[string][]byte{"GSUB": nested})
	if got, _ := f.ShapeGlyphs("ab"); !slices.Equal(planGIDs(got), []int{a, b}) {
		t.Errorf("from inside a contextual rule: glyphs %v, want [%d %d]", planGIDs(got), a, b)
	}
}

// TestAutomaticFractions. HarfBuzz applies 'numr' to the digits before a
// fraction slash, 'dnom' to the digits after, and 'frac' to all of it, by
// default — so "1⁄2" is a fraction in any font that has the forms. Not around
// an ordinary slash, and not around a fraction slash with no digits on one side.
func TestAutomaticFractions(t *testing.T) {
	const one, slash, two, numr1, dnom2, ascii = 1, 2, 3, 4, 5, 6
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{one}, []int{numr1})}},
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{two}, []int{dnom2})}},
	}, map[string][]int{"numr": {0}, "dnom": {1}})
	f := planFace(t, []rune{'1', 0x2044, '2', 0xE000, 0xE001, '/'}, map[string][]byte{"GSUB": gsub})
	for _, c := range []struct {
		text string
		want []int
	}{
		{"1⁄2", []int{numr1, slash, dnom2}},
		{"1/2", []int{one, ascii, two}},
		{"1⁄", []int{one, slash}},
		{"12", []int{one, two}},
	} {
		if got, _ := f.ShapeGlyphs(c.text); !slices.Equal(planGIDs(got), c.want) {
			t.Errorf("%q: glyphs %v, want %v", c.text, planGIDs(got), c.want)
		}
	}
}

// TestMirroredFormsTheFontGives. In a right-to-left run a character with a
// Unicode mirror is drawn as its mirror, and where the face cannot draw the
// mirror it is drawn as itself and its own glyph is what 'rtlm' is asked about.
// HarfBuzz does both; this drew .notdef for a mirror the face lacked, and
// applied no 'rtlm' at all.
func TestMirroredFormsTheFontGives(t *testing.T) {
	const alef, open, rtlm, close_ = 1, 2, 3, 4
	gsub := fonttest.GSUBLookups([]fonttest.Lookup{
		{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{open}, []int{rtlm})}},
	}, map[string][]int{"rtlm": {0}})
	// No glyph for ')': '(' is kept, and 'rtlm' gives the font's mirrored form.
	f := planFace(t, []rune{0x05D0, '(', 0xE000}, map[string][]byte{"GSUB": gsub})
	got, _ := f.ShapeGlyphs("א(")
	if want := []int{rtlm, alef}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("with no mirror glyph: glyphs %v, want %v", planGIDs(got), want)
	}
	// With one: Unicode's mirror is drawn, and 'rtlm' is not asked about it.
	f = planFace(t, []rune{0x05D0, '(', 0xE000, ')'}, map[string][]byte{"GSUB": gsub})
	got, _ = f.ShapeGlyphs("א(")
	if want := []int{close_, alef}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("with a mirror glyph: glyphs %v, want %v", planGIDs(got), want)
	}
}

// TestTheRequiredFeatureIsApplied. A language system's required feature applies
// whatever its tag, and HarfBuzz applies one no model asks for in the first
// stage. Its tag here is one nothing names.
func TestTheRequiredFeatureIsApplied(t *testing.T) {
	const a, b = 1, 2
	scripts := map[string]fonttest.Script{"latn": {Required: 0, Features: []int{}}}
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{a}, []int{b})}}},
		[]fonttest.Feature{{Tag: "zzzz", Lookups: []int{0}}},
		scripts)
	f := planFace(t, []rune{'a', 0xE000}, map[string][]byte{"GSUB": gsub})
	if got, _ := f.ShapeGlyphs("a"); !slices.Equal(planGIDs(got), []int{b}) {
		t.Errorf("glyphs %v, want [%d]", planGIDs(got), b)
	}
}

// TestAMaskedFeatureReadsItsContextInTheSyllable. A feature for part of an
// Indic syllable — 'half' for the consonants before the base — starts only at
// the glyphs it is for and matches its input only over them, and reads what
// follows as context wherever it is in the syllable. This font forms the half
// Ka only before a Ta, which is the base: the rule's context is outside the
// stretch 'half' is for, and holding the context to that stretch, as this did,
// never formed it. HarfBuzz forms it.
func TestAMaskedFeatureReadsItsContextInTheSyllable(t *testing.T) {
	const ka, virama, ta, half = 1, 2, 3, 4
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{
			{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(
				[]fonttest.Ligature{{Components: []int{ka, virama}, Glyph: half}})}},
			{Type: 6, Subtables: [][]byte{fonttest.ChainedContext3(nil, [][]int{{ka}, {virama}}, [][]int{{ta}},
				[]fonttest.SeqLookup{{At: 0, Lookup: 0}})}},
		},
		[]fonttest.Feature{{Tag: "half", Lookups: []int{1}}},
		onlyScript("dev2", 1))
	f := planFace(t, []rune{0x0915, 0x094D, 0x0924, 0xE000}, map[string][]byte{"GSUB": gsub})
	got, _ := f.ShapeGlyphs("क्त")
	if want := []int{half, ta}; !slices.Equal(planGIDs(got), want) {
		t.Errorf("glyphs %v, want %v", planGIDs(got), want)
	}
	// And a masked feature does not start at a glyph it is not for: the same
	// rule stated without context under 'half' does not touch a Ka after the
	// base, where 'half' is not for it.
	plain := fonttest.GSUBTable(
		[]fonttest.Lookup{{Type: 4, Subtables: [][]byte{fonttest.LigatureSubst(
			[]fonttest.Ligature{{Components: []int{ka, virama}, Glyph: half}})}}},
		[]fonttest.Feature{{Tag: "half", Lookups: []int{0}}},
		onlyScript("dev2", 1))
	f = planFace(t, []rune{0x0915, 0x094D, 0x0924, 0xE000}, map[string][]byte{"GSUB": plain})
	// Ta, virama, Ka: the base is Ka, and the Ta before it is the half form's.
	got, _ = f.ShapeGlyphs("त्क्")
	for _, g := range got {
		if g.GID == half {
			t.Errorf("glyphs %v: a Ka after the base was given its half form", planGIDs(got))
		}
	}
}

// TestMyanmarFeaturesStepOverANonJoinerInContext is audit C188. HarfBuzz
// enables the Myanmar features with only the zero width joiner manual, so a
// rule's context steps over a non-joiner. They were enabled with both.
func TestMyanmarFeaturesStepOverANonJoinerInContext(t *testing.T) {
	const ka, aa, alt = 1, 2, 3
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{aa}, []int{alt})}},
			{Type: 6, Subtables: [][]byte{fonttest.ChainedContext3([][]int{{ka}}, [][]int{{aa}}, nil,
				[]fonttest.SeqLookup{{At: 0, Lookup: 0}})}},
		},
		[]fonttest.Feature{{Tag: "psts", Lookups: []int{1}}},
		onlyScript("mym2", 1))
	f := planFace(t, []rune{0x1000, 0x102B, 0xE000, 0x200C}, map[string][]byte{"GSUB": gsub})
	for _, text := range []string{"ကါ", "က‌ါ"} {
		got, _ := f.ShapeGlyphs(text)
		if !slices.Contains(planGIDs(got), alt) {
			t.Errorf("%+q: glyphs %v, want the alternate", text, planGIDs(got))
		}
	}
}

// TestUniversalFormsAreByCluster is audit C70. For a script that does not join,
// HarfBuzz gives each cluster the positional form of its place among its
// neighbours — the first initial, the middle medial, the last final — and
// applies each form's feature to its own clusters. This applied all four to
// every glyph, in lookup order, so the first of them won everywhere.
func TestUniversalFormsAreByCluster(t *testing.T) {
	const ka, isol, init_, medi, fina = 1, 2, 3, 4, 5
	var lookups []fonttest.Lookup
	for _, to := range []int{isol, init_, medi, fina} {
		lookups = append(lookups, fonttest.Lookup{Type: 1, Subtables: [][]byte{
			fonttest.SingleSubst([]int{ka}, []int{to})}})
	}
	gsub := fonttest.GSUBTable(lookups, []fonttest.Feature{
		{Tag: "isol", Lookups: []int{0}}, {Tag: "init", Lookups: []int{1}},
		{Tag: "medi", Lookups: []int{2}}, {Tag: "fina", Lookups: []int{3}},
	}, onlyScript("bali", 4))
	f := planFace(t, []rune{0x1B13, 0xE000, 0xE001, 0xE002, 0xE003}, map[string][]byte{"GSUB": gsub})
	for _, c := range []struct {
		text string
		want []int
	}{
		{"ᬓ", []int{isol}},
		{"ᬓᬓ", []int{init_, fina}},
		{"ᬓᬓᬓ", []int{init_, medi, fina}},
	} {
		if got, _ := f.ShapeGlyphs(c.text); !slices.Equal(planGIDs(got), c.want) {
			t.Errorf("%+q: glyphs %v, want %v", c.text, planGIDs(got), c.want)
		}
	}
}

// TestSyllabicRunsTakeTheRequestedFeatures. A document's font-variant and
// font-feature-settings reached every run but a syllabic one: the four
// syllabic models applied their own lists and nothing else, so turning the
// contextual alternates off left them on in Devanagari, and asking for a
// feature by tag did nothing there. They are requests to the plan now, and
// every model reads its plan.
func TestSyllabicRunsTakeTheRequestedFeatures(t *testing.T) {
	const ka, alt, ss = 1, 2, 3
	gsub := fonttest.GSUBTable(
		[]fonttest.Lookup{
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{ka}, []int{alt})}},
			{Type: 1, Subtables: [][]byte{fonttest.SingleSubst([]int{ka}, []int{ss})}},
		},
		[]fonttest.Feature{{Tag: "calt", Lookups: []int{0}}, {Tag: "ss01", Lookups: []int{1}}},
		onlyScript("dev2", 2))
	f := planFace(t, []rune{0x0915, 0xE000, 0xE001}, map[string][]byte{"GSUB": gsub})
	for _, c := range []struct {
		what string
		off  Features
		want int
	}{
		{"by default", Features{}, alt},
		{"with the alternates turned off", Features{NoContextualAlternates: true}, ka},
		{"with ss01 asked for", Features{NoContextualAlternates: true, Tags: "ss01"}, ss},
	} {
		got, _ := f.ShapeGlyphsInContext("क", "", "", c.off)
		if !slices.Equal(planGIDs(got), []int{c.want}) {
			t.Errorf("%s: glyphs %v, want [%d]", c.what, planGIDs(got), c.want)
		}
	}
}
