package layout

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Where CSS Fonts 4 §7.2 sets a variable face, and that the face set there is
// the one shaped, measured and drawn. See fontinstance.go.

// axesFont is testdata/harfbuzz/fonts/VariedAxes.ttf: weight 100..400..900,
// width 50..100..150, optical size 8..14..144, slant -15..0 and italic 0..1,
// with instances named "Bold" and "Display Light Condensed".
func axesFont(t *testing.T) *shape.Face {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", "VariedAxes.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func settingsOf(t *testing.T, s string) []variationSetting {
	t.Helper()
	vals, _ := css.ParseComponentValues(s)
	out, _ := variationSettingsIn(vals)
	return out
}

// TestSection72OrdersTheVariations is §7.2's order, one step overriding the
// one before it on the axes it sets: the weight, width and style; the named
// instance; the descriptor's settings; the optical size; the property's
// settings — and every value clamped to the face.
func TestSection72OrdersTheVariations(t *testing.T) {
	face := axesFont(t)
	req := func(w, wd float64, slope FontSlope, angle float64) FontRequest {
		return FontRequest{Weight: w, Width: wd, Slope: slope, Angle: angle}
	}
	rule := func(r fontFaceRule) *documentFace {
		if r.weight == (faceRange{}) {
			r.weight.auto = true
		}
		if r.width == (faceRange{}) {
			r.width.auto = true
		}
		return &documentFace{rule: r, match: r.matchable()}
	}
	for _, tc := range []struct {
		name string
		ask  variationAsk
		want map[string]float64
	}{
		{"the request", variationAsk{r: req(700, 75, SlopeNormal, 0), size: 16},
			map[string]float64{"wght": 700, "wdth": 75, "slnt": 0}},
		{"clamped to the face", variationAsk{r: req(1000, 10, SlopeNormal, 0), size: 16},
			map[string]float64{"wght": 900, "wdth": 50, "slnt": 0}},
		{"italic sets ital and not slnt", variationAsk{r: req(400, 100, SlopeItalic, 0), size: 16},
			map[string]float64{"wght": 400, "wdth": 100, "ital": 1}},
		// CSS leans right for a positive angle and 'slnt' left, so the sign
		// turns; and the face goes no further than -15.
		{"oblique sets slnt", variationAsk{r: req(400, 100, SlopeOblique, 10), size: 16},
			map[string]float64{"slnt": -10}},
		{"oblique past the axis", variationAsk{r: req(400, 100, SlopeOblique, 40), size: 16},
			map[string]float64{"slnt": -15}},
		// A backslant is not on this face's axis at all: §5.2 finds upright.
		{"a backslant", variationAsk{r: req(400, 100, SlopeOblique, -10), size: 16},
			map[string]float64{"slnt": 0}},
		// The descriptors clamp the request; auto does not.
		{"descriptor weight", variationAsk{r: req(800, 100, SlopeNormal, 0), size: 16,
			df: rule(fontFaceRule{weight: faceRange{valueRange: valueRange{300, 600}}})},
			map[string]float64{"wght": 600}},
		{"descriptor width", variationAsk{r: req(400, 60, SlopeNormal, 0), size: 16,
			df: rule(fontFaceRule{width: faceRange{valueRange: valueRange{75, 125}}})},
			map[string]float64{"wdth": 75}},
		{"auto clamps nothing", variationAsk{r: req(850, 100, SlopeNormal, 0), size: 16, df: rule(fontFaceRule{})},
			map[string]float64{"wght": 850}},
		// A declared oblique range: the angle §5.2 rested at, within it.
		{"descriptor oblique", variationAsk{r: req(400, 100, SlopeOblique, 20), size: 16,
			df:    rule(fontFaceRule{style: faceStyle{kind: styleOblique, angles: valueRange{5, 8}}}),
			match: faceMatch{slope: slopeMatch{oblique: true, value: 8}}},
			map[string]float64{"slnt": -8}},
		// 5 over 2: the named instance's weight over the request's.
		{"named over the request", variationAsk{r: req(300, 100, SlopeNormal, 0), size: 16,
			df: func() *documentFace {
				d := rule(fontFaceRule{namedInstance: "bold"})
				d.named = map[string]float64{"wght": 700, "wdth": 100, "opsz": 14, "slnt": 0, "ital": 0}
				return d
			}()},
			map[string]float64{"wght": 700}},
		// 6 over 5.
		{"descriptor settings over named", variationAsk{r: req(300, 100, SlopeNormal, 0), size: 16,
			df: func() *documentFace {
				d := rule(fontFaceRule{namedInstance: "bold", variations: settingsOf(t, `"wght" 250, "wdth" 120`)})
				d.named = map[string]float64{"wght": 700, "wdth": 100}
				return d
			}()},
			map[string]float64{"wght": 250, "wdth": 120}},
		// 9 over 6: the optical size follows the font size unless it is
		// turned off, whatever the descriptor said.
		{"optical size over the descriptor", variationAsk{r: req(400, 100, SlopeNormal, 0), size: 48, optical: true,
			df: rule(fontFaceRule{variations: settingsOf(t, `"opsz" 100`)})},
			map[string]float64{"opsz": 48}},
		{"optical size off", variationAsk{r: req(400, 100, SlopeNormal, 0), size: 48,
			df: rule(fontFaceRule{variations: settingsOf(t, `"opsz" 100`)})},
			map[string]float64{"opsz": 100}},
		{"optical size clamped", variationAsk{r: req(400, 100, SlopeNormal, 0), size: 4, optical: true},
			map[string]float64{"opsz": 8}},
		// 12 over everything, a later tag over an earlier, and a tag the face
		// has no axis for ignored.
		{"property over all", variationAsk{r: req(900, 100, SlopeItalic, 0), size: 48, optical: true,
			df:       rule(fontFaceRule{variations: settingsOf(t, `"wght" 250`)}),
			settings: settingsOf(t, `"wght" 500, "opsz" 20, "ital" 0, "wght" 650, "XXXX" 3`)},
			map[string]float64{"wght": 650, "opsz": 20, "ital": 0}},
	} {
		got := instanceCoords(face, tc.ask)
		for tag, want := range tc.want {
			if v, ok := got[tag]; !ok || v != want {
				t.Errorf("%s: %s is %v (set %v), want %v; all %v", tc.name, tag, v, ok, want, got)
			}
		}
		// font-style sets one of the two (§7.2's second step); a named
		// instance or a setting may set either afterwards.
		if _, both := got["slnt"]; both && len(tc.ask.settings) == 0 && (tc.ask.df == nil || tc.ask.df.named == nil) {
			if _, ital := got["ital"]; ital {
				t.Errorf("%s: both slnt and ital were set: %v", tc.name, got)
			}
		}
		if _, set := got["opsz"]; set && !tc.ask.optical && tc.want["opsz"] == 0 && (tc.ask.df == nil || tc.ask.df.named == nil) {
			t.Errorf("%s: opsz was set with optical sizing off and no setting: %v", tc.name, got)
		}
		for tag, v := range got {
			if v == 0 && math.Signbit(v) {
				t.Errorf("%s: %s is negative zero, which spells a second instance of the default", tc.name, tag)
			}
		}
		if _, ok := got["XXXX"]; ok {
			t.Errorf("%s: an axis the face does not have was set", tc.name)
		}
	}
}

// variedDoc lays a document out in a set whose one face is the bundled
// variable Noto Sans, the way a PDF backend lends it, and returns the lines of #p and
// what was reported.
func variedDoc(t *testing.T, html string) (*Fragment, []Finding, FontSet) {
	t.Helper()
	face, err := shape.Load(realFont())
	if err != nil {
		t.Fatal(err)
	}
	built := Build(Input{HTML: html, Fonts: singleFaceSet{face}})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(20000)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	return root, append(built.Findings, rec.Findings()...), built.Fonts
}

// TestABoldRunIsSetInTheInstanceItAsks is the point of the change: bold text
// in a variable face is shaped, measured and drawn in the face at weight 700,
// which is what shape.LoadInstance cuts there — its weight, its advances and
// its name — and regular text in the face as it loaded.
func TestABoldRunIsSetInTheInstanceItAsks(t *testing.T) {
	root, findings, _ := variedDoc(t, `<p id="p">AVATAR <b>AVATAR</b> <span style="font-weight: 300; font-stretch: 75%">AVATAR</span></p>`)
	for _, f := range findings {
		if f.Property == "font-variation-settings" {
			t.Errorf("reported: %v", f)
		}
	}
	runs := linesOf(t, root, "p")[0].Runs
	var regular, bold, light *shape.Face
	for _, r := range runs {
		if !strings.Contains(r.Text, "AVATAR") {
			continue
		}
		switch {
		case regular == nil:
			regular = r.Face
		case bold == nil:
			bold = r.Face
		default:
			light = r.Face
		}
	}
	if regular == nil || bold == nil || light == nil {
		t.Fatalf("three runs of AVATAR were not found: %v", runs)
	}
	if !regular.IsVariable() {
		t.Errorf("the regular run is set in %q, an instance; it asks for the face's default", regular.Name())
	}
	for _, c := range []struct {
		face   *shape.Face
		coords map[string]float64
	}{
		{bold, map[string]float64{"wght": 700, "wdth": 100}},
		{light, map[string]float64{"wght": 300, "wdth": 75}},
	} {
		want, err := shape.LoadInstance(realFont(), c.coords)
		if err != nil {
			t.Fatal(err)
		}
		got := c.face
		if got.IsVariable() || got.Name() != want.Name() || got.Descriptor().Weight != want.Descriptor().Weight {
			t.Errorf("%v: the run is set in %q weight %d, want %q weight %d", c.coords,
				got.Name(), got.Descriptor().Weight, want.Name(), want.Descriptor().Weight)
		}
		if g, w := got.Measure("AVATAR", 100), want.Measure("AVATAR", 100); g != w {
			t.Errorf("%v: AVATAR measures %v, want the instance's %v", c.coords, g, w)
		}
	}
	// And the display list carries it: a backend embeds the instance, whose
	// program is the one cut at 700.
	found := false
	for _, op := range Paint(root) {
		if d, ok := op.(DrawText); ok && d.Face == bold {
			found = true
			if d.Face.IsVariable() {
				t.Error("the bold run is drawn in a variable face")
			}
		}
	}
	if !found {
		t.Error("no DrawText carries the bold instance")
	}
}

// singleFaceSet answers every family with one face, as a caller lending one face does.
type singleFaceSet struct{ f *shape.Face }

func (s singleFaceSet) Face(string, bool, bool) (*shape.Face, bool) { return s.f, true }

// TestAnInstanceIsCutOncePerDocument: a thousand bold words are one instance,
// and a second weight is a second.
func TestAnInstanceIsCutOncePerDocument(t *testing.T) {
	var b strings.Builder
	b.WriteString(`<p id="p">`)
	for i := 0; i < 1000; i++ {
		b.WriteString(`<b>word</b> <span style="font-weight: 600">word</span> `)
	}
	b.WriteString(`</p>`)
	face, err := shape.Load(realFont())
	if err != nil {
		t.Fatal(err)
	}
	built := Build(Input{HTML: b.String(), Fonts: singleFaceSet{face}})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(200000)
	Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	in := instancerOf(built.Fonts)
	if in == nil {
		t.Fatal("the document has no instancer")
	}
	if in.count != 2 || len(in.made) != 2 {
		t.Errorf("%d instances were cut (%d kept) for two weights over a thousand runs, want 2", in.count, len(in.made))
	}
}

// TestTheInstanceBudgetIsReported: past maxDocumentInstances a face is set at
// its default instance, and the document is told so, once.
func TestTheInstanceBudgetIsReported(t *testing.T) {
	saved := maxDocumentInstances
	maxDocumentInstances = 2
	defer func() { maxDocumentInstances = saved }()
	_, findings, set := variedDoc(t, `<p id="p"><span style="font-weight: 200">a</span>
		<span style="font-weight: 500">b</span><span style="font-weight: 600">c</span>
		<span style="font-weight: 800">d</span></p>`)
	n := 0
	for _, f := range findings {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "instances of variable faces") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d limit findings, want 1: %v", n, findings)
	}
	if in := instancerOf(set); in.count != 2 {
		t.Errorf("%d instances were cut under a budget of 2", in.count)
	}
	fired[RuleLimit] = true

	// The bytes too.
	savedBytes := maxDocumentInstanceBytes
	maxDocumentInstanceBytes = len(realFont()) + 1
	defer func() { maxDocumentInstanceBytes = savedBytes }()
	maxDocumentInstances = saved
	_, findings, set = variedDoc(t, `<p id="p"><b>a</b><span style="font-weight: 200">b</span></p>`)
	n = 0
	for _, f := range findings {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "instances of variable faces") {
			n++
		}
	}
	if n != 1 || instancerOf(set).count != 1 {
		t.Errorf("a byte budget of one font program cut %d instances and reported %d times",
			instancerOf(set).count, n)
	}
}

// TestALongVariationListKeepsItsLastEntries: past maxVariationSettings the
// earlier entries are dropped — a later setting overrides an earlier one, so
// the last ones are the ones that count — and that is reported.
func TestALongVariationListKeepsItsLastEntries(t *testing.T) {
	var parts []string
	for i := 0; i < maxVariationSettings+10; i++ {
		parts = append(parts, fmt.Sprintf(`"wght" %d`, 100+i))
	}
	vals, _ := css.ParseComponentValues(strings.Join(parts, ", "))
	settings, over := variationSettingsIn(vals)
	if over != 10 || len(settings) != maxVariationSettings || settings[len(settings)-1].value != float64(100+maxVariationSettings+9) {
		t.Errorf("%d settings kept, %d over, last %v", len(settings), over, settings[len(settings)-1])
	}
	_, findings, _ := variedDoc(t, `<p id="p" style='font-variation-settings: `+strings.Join(parts, ", ")+`'>x</p>`)
	found := false
	for _, f := range findings {
		found = found || f.Rule == RuleLimit && f.Property == "font-variation-settings"
	}
	if !found {
		t.Errorf("a list of %d settings was not reported: %v", len(parts), findings)
	}
}

// TestTheDescriptorsPlaceADocumentsFace: an @font-face rule's
// font-variation-settings and font-named-instance are applied, a name the face
// does not have is reported, and the rule's font-feature-settings survive onto
// the instance.
func TestTheDescriptorsPlaceADocumentsFace(t *testing.T) {
	res := &fileResolver{files: map[string][]byte{"v.ttf": realFont()}}
	built := Build(Input{
		HTML: `<style>
			@font-face { font-family: Named; src: url(v.ttf); font-named-instance: "semibold"; }
			@font-face { font-family: Set; src: url(v.ttf); font-variation-settings: "wght" 350, "wdth" 80;
				font-feature-settings: "smcp"; }
			@font-face { font-family: Missing; src: url(v.ttf); font-named-instance: "Heavy Italic"; }
		</style><p id="a" style="font-family: Named">x</p><p id="b" style="font-family: Set">x</p>`,
		Resources: res,
	})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(800)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	requireFinding(t, built.Findings, RuleUnsupportedValue, "Heavy Italic")
	for id, want := range map[string]int{"a": 600, "b": 350} {
		f := linesOf(t, root, id)[0].Runs[0].Face
		if f.IsVariable() || f.Descriptor().Weight != want {
			t.Errorf("#%s is set at weight %d (variable %v), want an instance at %d", id, f.Descriptor().Weight, f.IsVariable(), want)
		}
	}
	b := linesOf(t, root, "b")[0].Runs[0].Face
	has := false
	for _, s := range b.FeatureSettings() {
		has = has || s.Tag == "smcp" && s.On
	}
	if !has {
		t.Errorf("the instance lost the rule's font-feature-settings: %v", b.FeatureSettings())
	}
	if w := b.Descriptor(); w.Weight != 350 {
		t.Errorf("weight %d", w.Weight)
	}
}

// TestAnExIsTheInstancesXHeight: "font-size: 6ex" inside bold Noto Sans is six
// of the bold instance's x-heights — MVAR raises it — and not the Regular's.
func TestAnExIsTheInstancesXHeight(t *testing.T) {
	root, _, _ := variedDoc(t, `<div style="font-size: 100px; font-weight: 900"><p id="p" style="font-size: 6ex; margin: 0">x</p></div>`)
	bold, err := shape.LoadInstance(realFont(), map[string]float64{"wght": 900})
	if err != nil {
		t.Fatal(err)
	}
	regular, err := shape.Load(realFont())
	if err != nil {
		t.Fatal(err)
	}
	xh := func(f *shape.Face) float64 {
		return float64(f.Descriptor().XHeight) / float64(f.UnitsPerEm()) * 100
	}
	if xh(bold) == xh(regular) {
		t.Fatal("the bold instance's x-height is the regular one's, so this test can tell nothing apart")
	}
	got := linesOf(t, root, "p")[0].Runs[0].Size.Px()
	if want := 6 * xh(bold); math.Abs(got-want) > 0.01 {
		t.Errorf("6ex inside weight 900 is %vpx, want %vpx (the regular face's would be %vpx)", got, want, 6*xh(regular))
	}
}

// variableFallback lends the standard faces and falls back to the bundled
// variable face for what they cannot set.
type variableFallback struct {
	FontSet
	f *shape.Face
}

func (s variableFallback) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	if _, missing := s.f.ShapeGlyphs(text); missing == 0 {
		return s.f, true
	}
	return nil, false
}

// TestAFallbackFaceIsSetAtTheWeightAsked: bold Cyrillic the standard faces
// cannot set falls back to the variable face, and is drawn at weight 700 like
// any other bold text — and its Latin neighbour stays in the standard bold.
func TestAFallbackFaceIsSetAtTheWeightAsked(t *testing.T) {
	face, err := shape.Load(realFont())
	if err != nil {
		t.Fatal(err)
	}
	built := Build(Input{HTML: `<p id="p"><b>Hi Журнал</b></p>`,
		Fonts: variableFallback{FontSet: StandardFonts(), f: face}})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(800)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	seen := false
	for _, r := range linesOf(t, root, "p")[0].Runs {
		if !strings.Contains(r.Text, "Журнал") {
			continue
		}
		seen = true
		if r.Face.IsVariable() || r.Face.Descriptor().Weight != 700 {
			t.Errorf("the fallback run is set in %q at weight %d, want an instance at 700",
				r.Face.Name(), r.Face.Descriptor().Weight)
		}
	}
	if !seen {
		t.Fatal("no run holds the Cyrillic")
	}
}

// TestTheOpticalSizeFollowsTheFontSize: a face with an 'opsz' axis is set at
// the used font size in CSS pixels, and not where font-optical-sizing is none.
func TestTheOpticalSizeFollowsTheFontSize(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "harfbuzz", "fonts", "VariedAxes.ttf"))
	if err != nil {
		t.Fatal(err)
	}
	res := &fileResolver{files: map[string][]byte{"a.ttf": data}}
	built := Build(Input{
		HTML: `<style>@font-face { font-family: Axes; src: url(a.ttf); } p { font-family: Axes; margin: 0 }</style>
			<p id="a" style="font-size: 48px">AB</p><p id="b" style="font-size: 48px; font-optical-sizing: none">AB</p>
			<p id="c" style="font-size: 14px">AB</p>`,
		Resources: res,
	})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(800)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	a := linesOf(t, root, "a")[0].Runs[0].Face
	if a.IsVariable() || !strings.Contains(a.Name(), "opsz48") {
		t.Errorf("48px text is set in %q (variable %v), want the instance at opsz 48", a.Name(), a.IsVariable())
	}
	if b := linesOf(t, root, "b")[0].Runs[0].Face; !b.IsVariable() {
		t.Errorf("with font-optical-sizing: none the text is set in %q, want the face at its default", b.Name())
	}
	// 14px is the axis's default, so it is the face as it loaded.
	if c := linesOf(t, root, "c")[0].Runs[0].Face; !c.IsVariable() {
		t.Errorf("14px text is set in %q, want the face at its default", c.Name())
	}
}

// withTableTag renames one table of an sfnt's directory, leaving its bytes
// where they are.
func withTableTag(t *testing.T, data []byte, from, to string) []byte {
	t.Helper()
	out := append([]byte(nil), data...)
	n := int(out[4])<<8 | int(out[5])
	for i := 0; i < n; i++ {
		rec := 12 + 16*i
		if string(out[rec:rec+4]) == from {
			copy(out[rec:rec+4], to)
			return out
		}
	}
	t.Fatalf("the font has no %q table", from)
	return nil
}

// TestACFF2FaceIsReportedAsCFF2: a font whose outlines are a CFF2 table is a
// variable font this engine cannot read at any instance. It is reported as
// that, by name — not as a font with no outlines, and not drawn as some other
// instance or face — and its text is set in the next family.
func TestACFF2FaceIsReportedAsCFF2(t *testing.T) {
	cff2 := withTableTag(t, realFont(), "glyf", "CFF2")
	res := &fileResolver{files: map[string][]byte{"v.otf": cff2}}
	built := Build(Input{
		HTML: `<style>@font-face { font-family: Blended; src: url(v.otf); }</style>
			<p id="p" style="font-family: Blended, serif; font-weight: 700">x</p>`,
		Resources: res,
	})
	w, _ := style.FromPx(800)
	h, _ := style.FromPx(800)
	rec := NewRecorder(nil)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, rec)
	findings := append(built.Findings, rec.Findings()...)
	// The family's summary, which carries the load's own reason, as every
	// font that arrived and did not become a face does (TestFontUndecodable).
	requireFinding(t, findings, RuleResourceBlocked, "outlines are CFF2")
	for _, f := range findings {
		if strings.Contains(f.Message, "neither glyf nor CFF") {
			t.Errorf("a CFF2 font is reported as having no outlines: %v", f)
		}
	}
	if f := linesOf(t, root, "p")[0].Runs[0].Face; f == nil || strings.Contains(f.Name(), "NotoSans") {
		t.Errorf("the text was not set in the next family, serif, but in %v", f)
	}
}
