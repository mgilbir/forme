package layout

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/mgilbir/forme/shape"
)

// composeFonts lays a document out in a set, with a stylesheet and the
// resources its @font-face rules fetch.
func composeFonts(set FontSet, html, css string, res ResourceResolver) Composed {
	in := Input{HTML: html, Fonts: set, Resources: res}
	if css != "" {
		in.CSS = []Stylesheet{{Source: css}}
	}
	return Compose(in, Options{})
}

// needOf is the FontNeed of the first finding of a rule, or nil.
func needOf(c Composed, rule Rule) (*FontNeed, Finding) {
	for _, f := range c.Findings {
		if f.Rule == rule {
			return f.Font, f
		}
	}
	return nil, Finding{}
}

// TestEachFontFindingSaysWhatWouldClearIt is the contract of Finding.Font, rule
// by rule: each finding a different face would clear carries the families to
// supply it under, exactly as the font set is asked for them, the request, and
// the characters or features the face must have.
func TestEachFontFindingSaysWhatWouldClearIt(t *testing.T) {
	noto := embeddedFallback(t)
	for _, tc := range []struct {
		what  string
		set   FontSet
		html  string
		css   string
		res   ResourceResolver
		rule  Rule
		check func(t *testing.T, n *FontNeed)
	}{{
		what: "no face at all",
		set:  emptyFontSet{},
		html: `<p style="font-family: 'My Font', monospace; font-weight: bold; font-style: italic">x</p>`,
		rule: RuleNoFace,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "My Font", "monospace", "serif")
			if n.Request.Weight != 700 || n.Request.Slope != SlopeItalic || !n.Request.Bold() || !n.Request.Italic() {
				t.Errorf("request %+v, want bold italic", n.Request)
			}
			if n.Text != "" || n.Used != "" {
				t.Errorf("text %q, used %q; want any face of the family, and nothing used", n.Text, n.Used)
			}
		},
	}, {
		what: "a family nobody has",
		set:  StandardFonts(),
		html: `<p style="font-family: Nonesuch">x</p>`,
		rule: RuleFontFallback,
		check: func(t *testing.T, n *FontNeed) {
			// Not "serif": the text is already in it.
			wantFamilies(t, n, "Nonesuch")
			if n.Used == "" {
				t.Error("the face the text was set in instead is not named")
			}
		},
	}, {
		what: "a family without the glyphs",
		set:  oneFaceSet{fallback: noto, standard: StandardFonts()},
		html: `<p style="font-family: Helvetica">привет</p>`,
		rule: RuleFontSubstituted,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "Helvetica")
			if n.Text != "привет" {
				t.Errorf("text %q, want the characters the other face set, each once", n.Text)
			}
			if n.Used != noto.Name() {
				t.Errorf("used %q, want %q", n.Used, noto.Name())
			}
		},
	}, {
		what: "a character no face has",
		set:  StandardFonts(),
		html: `<p style="font-family: sans-serif">Ω</p>`,
		rule: RuleGlyphMissing,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "sans-serif")
			if n.Text != "Ω" {
				t.Errorf("text %q, want the missing character", n.Text)
			}
		},
	}, {
		what: "small capitals a face does not have",
		set:  StandardFonts(),
		html: `<p style="font-variant-caps: small-caps">abc</p>`,
		rule: RuleCapsSynthesised,
		check: func(t *testing.T, n *FontNeed) {
			if !slices.Contains(n.Features, "smcp") {
				t.Errorf("features %q, want smcp", n.Features)
			}
		},
	}, {
		what: "oldstyle figures a face does not have",
		set:  StandardFonts(),
		html: `<p style="font-variant-numeric: oldstyle-nums">123</p>`,
		rule: RuleUnsupportedValue,
		check: func(t *testing.T, n *FontNeed) {
			if !slices.Equal(n.Features, []string{"onum"}) {
				t.Errorf("features %q, want onum", n.Features)
			}
		},
	}, {
		what: "an emphasis mark no face has",
		set:  StandardFonts(),
		html: `<p style="text-emphasis: '★'">abc</p>`,
		rule: RuleGlyphMissing,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "serif")
			if n.Text != "★" || n.Used != "" {
				t.Errorf("text %q used %q, want the mark and nothing used: the marks were not drawn", n.Text, n.Used)
			}
		},
	}, {
		what: "a superscript form a face does not have",
		set:  StandardFonts(),
		html: `<p style="font-variant-position: super">abc</p>`,
		rule: RuleUnsupportedValue,
		check: func(t *testing.T, n *FontNeed) {
			if !slices.Equal(n.Features, []string{"sups"}) {
				t.Errorf("features %q, want sups", n.Features)
			}
		},
	}, {
		what: "capitals a face does not have and this engine does not make",
		set:  StandardFonts(),
		html: `<p style="font-variant-caps: unicase">abc</p>`,
		rule: RuleUnsupportedValue,
		check: func(t *testing.T, n *FontNeed) {
			if !slices.Equal(n.Features, []string{"unic"}) {
				t.Errorf("features %q, want unic", n.Features)
			}
		},
	}, {
		what: "East Asian forms a face does not have",
		set:  namedFaceSet{family: "Noto", face: noto, standard: StandardFonts()},
		html: `<p style="font-family: Noto; font-variant-east-asian: jis78">漢</p>`,
		rule: RuleUnsupportedValue,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "Noto")
			if !slices.Equal(n.Features, []string{"jp78"}) {
				t.Errorf("features %q, want jp78", n.Features)
			}
		},
	}, {
		what: "a formula in a face with no MATH table",
		set:  StandardFonts(),
		html: `<math><mi>x</mi></math>`,
		rule: RuleMathFallback,
		check: func(t *testing.T, n *FontNeed) {
			if !slices.Contains(n.Features, "MATH") {
				t.Errorf("features %q, want MATH", n.Features)
			}
		},
	}, {
		what: "an @font-face that loaded nothing",
		set:  StandardFonts(),
		html: `<p style="font-family: Own">x</p>`,
		css:  `@font-face { font-family: Own; src: url(gone.ttf); font-weight: 700; font-style: italic }`,
		res:  &fileResolver{files: map[string][]byte{}},
		rule: RuleResourceBlocked,
		check: func(t *testing.T, n *FontNeed) {
			wantFamilies(t, n, "Own")
			if n.Request.Weight != 700 || n.Request.Slope != SlopeItalic {
				t.Errorf("request %+v, want the rule's own descriptors: 700 italic", n.Request)
			}
		},
	}} {
		t.Run(tc.what, func(t *testing.T) {
			got := composeFonts(tc.set, tc.html, tc.css, tc.res)
			n, f := needOf(got, tc.rule)
			if f.Rule == "" {
				t.Fatalf("no %s finding: %v", tc.rule, got.Findings)
			}
			if n == nil {
				t.Fatalf("the %s finding carries no FontNeed: %s", tc.rule, f.Message)
			}
			tc.check(t, n)
		})
	}
}

func wantFamilies(t *testing.T, n *FontNeed, want ...string) {
	t.Helper()
	if !slices.Equal(n.Families, want) {
		t.Errorf("families %q, want %q", n.Families, want)
	}
}

// TestAFontNeedIsBounded: everything in a need comes from the document, so each
// part is capped, and the caps are where no real document reaches.
func TestAFontNeedIsBounded(t *testing.T) {
	var families []string
	for i := 0; i < 100; i++ {
		families = append(families, "F"+strings.Repeat("x", i))
	}
	families = append(families, "f", "F", strings.Repeat("y", maxNeedFamilyBytes+1))
	var text strings.Builder
	for r := rune(0x4E00); r < 0x4E00+1000; r++ {
		text.WriteRune(r)
		text.WriteRune(r)
	}
	var features []string
	for i := 0; i < 40; i++ {
		features = append(features, "f"+strings.Repeat("e", i%4)+string(rune('a'+i%26)))
	}
	n := needFor(families, true, normalRequest, text.String(), features...)
	if len(n.Families) != maxNeedFamilies {
		t.Errorf("%d families kept, want the cap of %d", len(n.Families), maxNeedFamilies)
	}
	for _, f := range n.Families {
		if len(f) > maxNeedFamilyBytes {
			t.Errorf("a family of %d bytes was kept", len(f))
		}
	}
	if got := utf8.RuneCountInString(n.Text); got != maxNeedRunes {
		t.Errorf("%d characters kept, want the cap of %d", got, maxNeedRunes)
	}
	if len(n.Features) > maxNeedFeatures {
		t.Errorf("%d features kept, want at most %d", len(n.Features), maxNeedFeatures)
	}

	// Each character once, and a family named twice in different case once:
	// families are matched case-insensitively, as CSS matches them.
	n = needFor([]string{"Own", "own", "OWN"}, false, normalRequest, "abcabc")
	wantFamilies(t, n, "Own")
	if n.Text != "abc" {
		t.Errorf("text %q, want each character once", n.Text)
	}
	// And the initial family is not added twice where the list already ends in it.
	n = needFor([]string{"serif"}, true, normalRequest, "")
	wantFamilies(t, n, "serif")
}

// TestFontNeedsMergesWhatOneFaceWouldClear: the glyph-missing findings for one
// box are one per character, and one face covers all of them, so FontNeeds
// gives one need holding every character.
func TestFontNeedsMergesWhatOneFaceWouldClear(t *testing.T) {
	got := composeFonts(StandardFonts(), `<p style="font-family: sans-serif">ΩΣΔ</p>`, "", nil)
	var missing int
	for _, f := range got.Findings {
		if f.Rule == RuleGlyphMissing {
			missing++
		}
	}
	if missing < 2 {
		t.Fatalf("want a finding per missing character, got %d: %v", missing, got.Findings)
	}
	needs := got.FontNeeds()
	var glyphs []FontNeed
	for _, n := range needs {
		if n.Text != "" {
			glyphs = append(glyphs, n)
		}
	}
	if len(glyphs) != 1 || !sameRunes(glyphs[0].Text, "ΩΣΔ") {
		t.Errorf("FontNeeds gave %+v, want one need for sans-serif covering ΩΣΔ", glyphs)
	}
}

func sameRunes(a, b string) bool {
	x, y := []rune(a), []rune(b)
	slices.Sort(x)
	slices.Sort(y)
	return slices.Equal(x, y)
}

// TestAFormulaSaysWhichOperatorItNeedsAFaceFor: a MATH table without larger
// forms of an operator, and one without a mirrored form, name the operator as
// the character a face must have, with the table.
func TestAFormulaSaysWhichOperatorItNeedsAFaceFor(t *testing.T) {
	for _, tc := range []struct {
		html, says, text string
		features         []string
	}{
		{`<math display="block"><mo>∑</mo></math>`, "no larger forms of U+2211", "∑", []string{"MATH"}},
		{`<math dir="rtl" display="block"><mo>∑</mo></math>`, "no mirrored form of U+2211", "∑", []string{"MATH", "rtlm"}},
	} {
		_, findings := mathLayout(t, tc.html)
		var n *FontNeed
		for _, f := range findings {
			if f.Rule == RuleMathFallback && strings.Contains(f.Message, tc.says) {
				n = f.Font
			}
		}
		if n == nil {
			t.Errorf("%s: no %q finding carrying a FontNeed: %v", tc.html, tc.says, findings)
			continue
		}
		if n.Text != tc.text || !slices.Equal(n.Features, tc.features) || n.Used == "" {
			t.Errorf("%s: need %+v, want text %q, features %q and the face used", tc.html, *n, tc.text, tc.features)
		}
	}
}

// needSet answers each family with one face, which is what a caller building
// its retry set from FontNeeds does.
type needSet map[string]*shape.Face

func (s needSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	f, ok := s[familyKey(family)]
	return f, ok
}

// TestTheRetryLoopClearsTheFindings is the point of all of it: a caller reads
// FontNeeds, supplies a face for each under the family it names, and the next
// Compose is not refused.
func TestTheRetryLoopClearsTheFindings(t *testing.T) {
	const html = `<p style="font-family: 'House Sans', sans-serif">One</p>` +
		`<p style="font-family: monospace; font-weight: bold">Two</p>` +
		`<p>Three</p>`
	first := composeFonts(emptyFontSet{}, html, "", nil)
	if !first.Refused {
		t.Fatal("a document with no faces was not refused")
	}
	needs := first.FontNeeds()
	if len(needs) == 0 {
		t.Fatal("a refused document gave no FontNeeds to act on")
	}
	set := needSet{}
	face := embeddedFallback(t)
	for _, n := range needs {
		if n.Text != "" || len(n.Features) != 0 {
			t.Fatalf("a need for no face at all asks for more than a family: %+v", n)
		}
		set[familyKey(n.Families[0])] = face
	}
	second := composeFonts(set, html, "", nil)
	if second.Refused {
		t.Errorf("supplying a face for every need left the document refused: %v", second.Findings)
	}
	for _, f := range second.Findings {
		if f.Font != nil && f.Rule == RuleNoFace {
			t.Errorf("still reported after the retry: %s", f.Message)
		}
	}
}

// TestTwoNeedsThatReadAlikeAreTwoFindings: a message quotes a family list cut
// at forty characters, so two lists that share their first forty read the
// same, and the recorder merged them into one finding — one family list the
// caller never heard of. The need is part of what makes a finding distinct.
func TestTwoNeedsThatReadAlikeAreTwoFindings(t *testing.T) {
	const prefix = "A Family Name Long Enough To Be Cut Off In The Message"
	got := composeFonts(emptyFontSet{},
		`<p style="font-family: '`+prefix+` One'">x</p><p style="font-family: '`+prefix+` Two'">y</p>`, "", nil)
	var families []string
	for _, f := range got.Findings {
		if f.Rule == RuleNoFace && f.Font != nil {
			families = append(families, f.Font.Families[0])
		}
	}
	if !slices.Equal(families, []string{prefix + " One", prefix + " Two"}) {
		t.Errorf("no-face findings name %q, want both families, each whole", families)
	}
}
