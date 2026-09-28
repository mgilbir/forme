package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Fractions and scripts, in exact arithmetic: the face of mathlayout_test.go
// with the constants each test names, every other constant nought (a MATH
// table states all of them), set at 16px so that a font unit is a style.Unit.
// The tokens are held at 16px by the author sheet, so that a script is not
// made smaller than its base and every number stays whole.

func mathFaceWith(t testing.TB, constants map[string]int) *shape.Face {
	t.Helper()
	c := map[string]int{"ScriptPercentScaleDown": 70, "ScriptScriptPercentScaleDown": 50, "AxisHeight": 256}
	for k, v := range constants {
		c[k] = v
	}
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "MathTest", UnitsPerEm: 1024, Ascent: 1024, Descent: -256, Glyphs: mathTestGlyphs,
		Extra: map[string][]byte{"MATH": fonttest.MATH(fonttest.MathOptions{
			Constants:           c,
			ItalicsCorrection:   map[int]int{mathGlyphItalicX: 60, mathGlyphItalicF: 100, mathGlyphIntegral: 100},
			TopAccentAttachment: map[int]int{mathGlyphItalicX: 300},
		})},
	}))
	if err != nil {
		t.Fatal(err)
	}
	return face
}

func mathLayoutIn(t *testing.T, face *shape.Face, doc string) (*Fragment, []Finding) {
	t.Helper()
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: doc, CSS: []Stylesheet{{Source: `body { margin: 0; font-size: 16px }
		mi, mn, mo, mtext { font-size: 16px }`}}, Fonts: set})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(b.Root, Size{W: w, H: h}, set, rec)
	return frag, append(b.Findings, rec.Findings()...)
}

// mathBox is a fragment's box as a formula measures it, from the content box
// of the fragment it is in: its offset, its width, how far it reaches above
// its baseline and below.
type mathGeom struct{ x, top, w, h style.Unit }

func geomIn(t *testing.T, root *Fragment, id, in string) mathGeom {
	t.Helper()
	f := find(t, root, id)
	x, y := at(f, find(t, root, in))
	return mathGeom{x, y, f.BorderRect.W, f.BorderRect.H}
}

// checkMath asserts each child's offset along the line and its top, from the
// top of the element's content box, and the element's content box's size and
// baseline.
func checkMath(t *testing.T, root *Fragment, el string, w, ascent, descent style.Unit, kids map[string][2]style.Unit) {
	t.Helper()
	f := find(t, root, el)
	c := f.ContentRect()
	if c.W != w || c.H != ascent.Add(descent) || f.mathBaseline != ascent {
		t.Errorf("#%s is %d by %d with its baseline %d down, want %d by %d and %d",
			el, c.W, c.H, f.mathBaseline, w, ascent.Add(descent), ascent)
	}
	for id, want := range kids {
		g := geomIn(t, root, id, el)
		if g.x != want[0] || g.top != want[1] {
			t.Errorf("#%s in #%s at (%d, %d), want (%d, %d)", id, el, g.x, g.top, want[0], want[1])
		}
	}
}

func mathFinding(findings []Finding, rule Rule, text string) bool {
	for _, f := range findings {
		if f.Rule == rule && strings.Contains(f.Message, text) {
			return true
		}
	}
	return false
}

// TestAFractionIsShiftedAboutTheAxis is §3.3.2.1 in display style: the "1"
// over the "y", with a bar 64 thick on the axis at 256. The numerator's shift
// is the font's 900, which beats 256 + 32 + 100 + its ink descent of 0; the
// denominator's is the font's 700, beating 32 + 120 + 448 − 256. The fraction
// reaches the numerator's top, 900 + 704, and the denominator's bottom,
// 700 + 192. Its padding is a pixel either side.
func TestAFractionIsShiftedAboutTheAxis(t *testing.T) {
	face := mathFaceWith(t, map[string]int{
		"FractionRuleThickness": 64, "FractionNumeratorDisplayStyleShiftUp": 900,
		"FractionNumDisplayStyleGapMin": 100, "FractionDenominatorDisplayStyleShiftDown": 700,
		"FractionDenomDisplayStyleGapMin": 120,
		// The compact ones, which a display fraction does not read.
		"FractionNumeratorShiftUp": 5000, "FractionDenominatorShiftDown": 5000,
	})
	root, findings := mathLayoutIn(t, face, `<math display="block"><mfrac id="f"><mn id="n">1</mn><mn id="d">y</mn></mfrac></math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	checkMath(t, root, "f", 512, 1604, 892, map[string][2]style.Unit{
		"n": {0, 0}, "d": {0, 1604 + 700 - 448},
	})
	if f := find(t, root, "f"); f.BorderRect.W != 512+128 {
		t.Errorf("the fraction is %d wide, want 640 with its padding", f.BorderRect.W)
	}
	// The bar, on the axis: its top 256 + 32 above the baseline.
	if got := find(t, root, "f").mathMarks; len(got) != 1 || got[0] != (Rect{X: 0, Y: 1604 - 288, W: 512, H: 64}) {
		t.Errorf("the bar is %v, want one at (0, %d) 512 by 64", got, 1604-288)
	}
}

// TestACompactFractionIsShiftedByItsGaps: in inline mathematics the compact
// constants are read, and here the gaps win. The "y" over the "1": 256 + 32 +
// 50 + the y's descent of 192 is 530, over the font's 300; and 32 + 60 + 704
// − 256 is 540, over 200.
func TestACompactFractionIsShiftedByItsGaps(t *testing.T) {
	face := mathFaceWith(t, map[string]int{
		"FractionRuleThickness": 64, "FractionNumeratorShiftUp": 300, "FractionNumeratorGapMin": 50,
		"FractionDenominatorShiftDown": 200, "FractionDenominatorGapMin": 60,
		"FractionNumeratorDisplayStyleShiftUp": 5000, "FractionDenominatorDisplayStyleShiftDown": 5000,
	})
	root, _ := mathLayoutIn(t, face, `<math><mfrac id="f"><mn id="n">y</mn><mn id="d">1</mn></mfrac></math>`)
	checkMath(t, root, "f", 512, 978, 540, map[string][2]style.Unit{
		"n": {0, 0}, "d": {0, 978 + 540 - 704},
	})
}

// TestAFractionWithNoBarIsAStack is §3.3.2.2: the two shifts, 800 and 600,
// leave 952 between the "1"'s ink and the "y"'s, and the least is 1000, so
// each moves 24 further out.
func TestAFractionWithNoBarIsAStack(t *testing.T) {
	face := mathFaceWith(t, map[string]int{
		"FractionRuleThickness":       64,
		"StackTopDisplayStyleShiftUp": 800, "StackBottomDisplayStyleShiftDown": 600,
		"StackDisplayStyleGapMin": 1000,
	})
	for _, lt := range []string{"0", "-5px"} {
		root, _ := mathLayoutIn(t, face, `<math display="block"><mfrac id="f" linethickness="`+lt+`">`+
			`<mn id="n">1</mn><mn id="d">y</mn></mfrac></math>`)
		checkMath(t, root, "f", 512, 1528, 816, map[string][2]style.Unit{
			"n": {0, 0}, "d": {0, 1528 + 624 - 448},
		})
		if got := find(t, root, "f").mathMarks; len(got) != 0 {
			t.Errorf("linethickness=%s: a stack draws %v", lt, got)
		}
	}
}

// TestALineThicknessIsALengthOrAPercentage: a percentage of the font's, and
// MathML 3's keywords and plain numbers not at all — reported.
func TestALineThicknessIsALengthOrAPercentage(t *testing.T) {
	face := mathFaceWith(t, map[string]int{"FractionRuleThickness": 64})
	for _, tc := range []struct {
		attr     string
		want     style.Unit
		reported bool
	}{
		{"200%", 128, false},
		{"3px", 192, false},
		{"thick", 64, true},
		{"2", 64, true},
		{"bad", 64, true},
	} {
		root, findings := mathLayoutIn(t, face, `<math display="block"><mfrac id="f" linethickness="`+tc.attr+`">`+
			`<mn>1</mn><mn>1</mn></mfrac></math>`)
		if got := find(t, root, "f").mathMarks; len(got) != 1 || got[0].H != tc.want {
			t.Errorf("linethickness=%q draws %v, want a bar %d thick", tc.attr, got, tc.want)
		}
		if got := mathFinding(findings, RuleUnsupportedValue, "linethickness"); got != tc.reported {
			t.Errorf("linethickness=%q reported %v, want %v: %v", tc.attr, got, tc.reported, findings)
		}
	}
	for _, v := range []string{"thick", "2", "+1.5", "-.5"} {
		_, findings := mathLayoutIn(t, face, `<math><mfrac linethickness="`+v+`"><mn>1</mn><mn>1</mn></mfrac></math>`)
		if !mathFinding(findings, RuleUnsupportedValue, "MathML 3") {
			t.Errorf("linethickness=%q, MathML 3's, is not said to be: %v", v, findings)
		}
	}
	for _, v := range []string{"nan", "1.2.3", "+", "0x1p3"} {
		_, findings := mathLayoutIn(t, face, `<math><mfrac linethickness="`+v+`"><mn>1</mn><mn>1</mn></mfrac></math>`)
		if mathFinding(findings, RuleUnsupportedValue, "MathML 3") || !mathFinding(findings, RuleUnsupportedValue, "linethickness") {
			t.Errorf("linethickness=%q is said to be MathML 3's, or nothing: %v", v, findings)
		}
	}
	// An odd thickness puts its extra unit above the axis: the bar's top is
	// 256 + 33 above the baseline.
	root, _ := mathLayoutIn(t, mathFaceWith(t, map[string]int{"FractionRuleThickness": 65}),
		`<math><mfrac id="f"><mn>1</mn><mn>1</mn></mfrac></math>`)
	f := find(t, root, "f")
	if got := f.mathMarks; len(got) != 1 || got[0].Y != f.mathBaseline-289 || got[0].H != 65 {
		t.Errorf("a bar 65 thick is %v, want its top at %d", got, f.mathBaseline-289)
	}
}

// TestAFractionReachesItsBar: the fraction's line-ascent is at least the top
// of its bar and its line-descent at least the bar's bottom and nought, which
// a font whose gaps are negative can make the numerator's and the
// denominator's are not. The fraction is of two empty rows.
func TestAFractionReachesItsBar(t *testing.T) {
	for _, tc := range []struct {
		name            string
		constants       map[string]int
		ascent, descent style.Unit
	}{
		// Neither row reaches above the bar's top, 256 + 32.
		{"bar top", map[string]int{"FractionNumeratorGapMin": -500}, 288, 0},
		// Everything is above the baseline: nothing below it.
		{"nought", map[string]int{"FractionNumeratorShiftUp": 100, "FractionNumeratorGapMin": -500,
			"FractionDenominatorShiftDown": -300, "FractionDenominatorGapMin": -100}, 300, 0},
		// The axis below the baseline: the bar's bottom is 300 + 32 down.
		{"bar bottom", map[string]int{"AxisHeight": -300, "FractionDenominatorGapMin": -100}, 0, 332},
	} {
		tc.constants["FractionRuleThickness"] = 64
		root, _ := mathLayoutIn(t, mathFaceWith(t, tc.constants), `<math><mfrac id="f"><mrow></mrow><mrow></mrow></mfrac></math>`)
		f := find(t, root, "f")
		if f.mathBaseline != tc.ascent || f.ContentRect().H != tc.ascent.Add(tc.descent) {
			t.Errorf("%s: the fraction reaches %d up and %d down, want %d and %d", tc.name,
				f.mathBaseline, f.ContentRect().H.Sub(f.mathBaseline), tc.ascent, tc.descent)
		}
	}
	// A stack's line-descent is not less than nought either: a font's shifts
	// of 100 up and 300 up, with a least gap it already has.
	root, _ := mathLayoutIn(t, mathFaceWith(t, map[string]int{"StackTopShiftUp": 100,
		"StackBottomShiftDown": -300, "StackGapMin": -1000}),
		`<math><mfrac id="f" linethickness="0"><mrow></mrow><mrow></mrow></mfrac></math>`)
	if f := find(t, root, "f"); f.mathBaseline != 300 || f.ContentRect().H != 300 {
		t.Errorf("the stack reaches %d up and is %d tall, want 300 and 300", f.mathBaseline, f.ContentRect().H)
	}
}

// TestANumeratorAndDenominatorAreCentred: the "1" over the "11" is 256 in.
func TestANumeratorAndDenominatorAreCentred(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, map[string]int{"FractionRuleThickness": 64}),
		`<math><mfrac id="f"><mn id="n">1</mn><mn id="d">11</mn></mfrac></math>`)
	if n, d := geomIn(t, root, "n", "f"), geomIn(t, root, "d", "f"); n.x != 256 || d.x != 0 {
		t.Errorf("the numerator at %d and the denominator at %d, want 256 and 0", n.x, d.x)
	}
}

// TestTheInkOfAnElementIsItsBox: §3.1.2, an element whose algorithm says
// nothing of its ink has its ink where its line-ascent and line-descent are —
// which is what a stretchy operator beside it is stretched to cover.
func TestTheInkOfAnElementIsItsBox(t *testing.T) {
	c := map[string]int{"FractionRuleThickness": 64}
	for k, v := range mathScriptConstants {
		c[k] = v
	}
	for k, v := range mathUnderOverConstants {
		c[k] = v
	}
	face := mathFaceWith(t, c)
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	for _, inner := range []string{
		`<mfrac id="e"><mn>1</mn><mn>y</mn></mfrac>`,
		`<mfrac id="e" linethickness="0"><mn>1</mn><mn>y</mn></mfrac>`,
		`<msubsup id="e"><mi>x</mi><mn>y</mn><mn>1</mn></msubsup>`,
		`<munderover id="e"><mn>x</mn><mn>y</mn><mn>1</mn></munderover>`,
	} {
		b := Build(Input{HTML: `<math>` + inner + `</math>`, Fonts: set,
			CSS: []Stylesheet{{Source: `mn, mi { font-size: 16px }`}}})
		l := newLayouter(b.Root, A4.Content(), set, nil)
		var e *Box
		var walk func(*Box)
		walk = func(x *Box) {
			if x.Element != nil {
				if id, _ := x.Element.Attr("id"); id == "e" {
					e = x
				}
			}
			for _, k := range x.Children {
				walk(k)
			}
		}
		walk(b.Root)
		got := l.mathBox(e, 0, mathStretch{})
		if got.inkAscent != got.ascent || got.inkDescent != got.descent || got.ascent <= 0 || got.descent <= 0 {
			t.Errorf("%s: ink %d up and %d down, line %d and %d", inner, got.inkAscent, got.inkDescent, got.ascent, got.descent)
		}
	}
}

// TestTheBarIsPaintedInTheFractionsColour: §3.3.2.1, in the <mfrac>'s colour
// and only where it is visible; across the content box, which a stated width
// makes wider than the fraction's content.
func TestTheBarIsPaintedInTheFractionsColour(t *testing.T) {
	face := mathFaceWith(t, map[string]int{"FractionRuleThickness": 64})
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	bars := func(doc string) []FillRect {
		out := Compose(Input{HTML: doc, Fonts: set, CSS: []Stylesheet{{Source: `mn { font-size: 16px }`}}}, Options{})
		var got []FillRect
		for _, op := range out.Ops {
			if f, ok := op.(FillRect); ok && f.Color.B == 255 {
				got = append(got, f)
			}
		}
		return got
	}
	got := bars(`<math><mfrac style="color: rgb(0, 0, 255); width: 20px"><mn>1</mn><mn>1</mn></mfrac></math>`)
	if len(got) != 1 || got[0].Rect.W != 20*64 || got[0].Rect.H != 64 {
		t.Errorf("the bar is drawn as %v, want one 20px wide and 64 thick", got)
	}
	// It starts where the fraction's content box does: 384 before the
	// numerator, which is centred in the 1280 of it.
	out := Compose(Input{HTML: `<math><mfrac style="color: rgb(0, 0, 255); width: 20px"><mn>1</mn><mn>2</mn></mfrac></math>`,
		Fonts: set, CSS: []Stylesheet{{Source: `mn { font-size: 16px }`}}}, Options{})
	var num *DrawText
	var bar *FillRect
	for _, op := range out.Ops {
		switch v := op.(type) {
		case DrawText:
			if v.Text == "1" && num == nil {
				num = &v
			}
		case FillRect:
			if v.Color.B == 255 {
				bar = &v
			}
		}
	}
	if num == nil || bar == nil || bar.Rect.X != num.At.X.Sub(384) {
		t.Errorf("the bar is at %v and the numerator %v, want the bar 384 before it", bar, num)
	}
	// And its top is at the numerator's baseline: the numerator is raised
	// 256 + 32 + a gap of nought, and has no ink below its baseline.
	if num != nil && bar != nil && bar.Rect.Y != num.At.Y {
		t.Errorf("the bar's top is at %d and the numerator's baseline at %d", bar.Rect.Y, num.At.Y)
	}
	if got := bars(`<math><mfrac style="color: rgb(0, 0, 255); visibility: hidden"><mn>1</mn><mn>1</mn></mfrac></math>`); len(got) != 0 {
		t.Errorf("a hidden fraction's bar is drawn: %v", got)
	}
	if got := bars(`<math style="color: rgb(0, 0, 255)"><mphantom><mfrac><mn>1</mn><mn>1</mn></mfrac></mphantom></math>`); len(got) != 0 {
		t.Errorf("a phantom fraction's bar is drawn: %v", got)
	}
}

// The script constants: distinct values, so that a shift taken from the wrong
// one shows.
var mathScriptConstants = map[string]int{
	"SubscriptShiftDown": 150, "SubscriptTopMax": 400, "SubscriptBaselineDropMin": 50,
	"SuperscriptShiftUp": 350, "SuperscriptShiftUpCramped": 250, "SuperscriptBottomMin": 100,
	"SuperscriptBaselineDropMax": 300, "SubSuperscriptGapMin": 200,
	"SuperscriptBottomMaxWithSubscript": 380, "SpaceAfterScript": 40,
}

// TestScriptsAreShiftedPastTheirBase is §3.4.1 on the italic x, 540 wide and
// leaning 60 past it.
//
// The subscript "1" is lowered by the greatest of 150, its ink's 704 less the
// 400 it may reach, and 50 below the x's ink: 304. It starts where the x's
// advance ends, since the x is not a large operator. The superscript "2" is
// raised by the greatest of 350, 100 over its ink's bottom, and 448 − 300: 350;
// and starts past the x's italic correction, at 600. Each is followed by 40.
//
// With both, the "y" under the "1": 150 and 350 leave 52 between them, and the
// least is 200. The superscript rises 30, to put its bottom at 380, and the
// subscript falls the remaining 118.
func TestScriptsAreShiftedPastTheirBase(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, findings := mathLayoutIn(t, face, `<math>`+
		`<msub id="sb"><mi id="b1">x</mi><mn id="s1">1</mn></msub>`+
		`<msup id="sp"><mi id="b2">x</mi><mn id="s2">2</mn></msup>`+
		`<msubsup id="ss"><mi id="b3">x</mi><mn id="s3">y</mn><mn id="s4">1</mn></msubsup>`+
		`<msup id="cr" style="math-shift: compact"><mi>x</mi><mn id="s5">2</mn></msup>`+
		`</math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	checkMath(t, root, "sb", 540+512+40, 448, 304, map[string][2]style.Unit{
		"b1": {0, 0}, "s1": {540, 448 + 304 - 704},
	})
	checkMath(t, root, "sp", 600+512+40, 704+350, 0, map[string][2]style.Unit{
		"b2": {0, 704 + 350 - 448}, "s2": {600, 0},
	})
	checkMath(t, root, "ss", 600+512+40, 704+380, 192+268, map[string][2]style.Unit{
		"b3": {0, 704 + 380 - 448}, "s3": {540, 704 + 380 + 268 - 448}, "s4": {600, 0},
	})
	// Cramped: the lesser 250.
	checkMath(t, root, "cr", 600+512+40, 704+250, 0, map[string][2]style.Unit{"s5": {600, 0}})
}

// TestALargeOperatorsSubscriptTucksUnderIt: the integral's italic correction
// of 100 moves its subscript back and its superscript not out (§3.4.1.2,
// §3.4.1.3).
func TestALargeOperatorsSubscriptTucksUnderIt(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math>`+
		`<msubsup id="i"><mo>∫</mo><mn id="lo">1</mn><mn id="hi">2</mn></msubsup></math>`)
	for id, want := range map[string]style.Unit{"lo": 300, "hi": 400} {
		if g := geomIn(t, root, id, "i"); g.x != want {
			t.Errorf("#%s at %d, want %d", id, g.x, want)
		}
	}
	if w := find(t, root, "i").ContentRect().W; w != 400+512+40 {
		t.Errorf("the integral with scripts is %d wide, want %d", w, 400+512+40)
	}
}

// TestPrescriptsAndPostscripts is §3.4.3: a column of prescripts, 40 and
// then the wider of the "y" and the "1"; the base; and a column of
// postscripts, whose shifts are the greatest either pair asks for. The
// prescript pair asks 268 and 380, as <msubsup>'s did; the postscript pair
// asks 304 and 350, which leave −50 between the "1" and the "2", so the
// superscript rises 30 and the subscript falls 220 more: 524 and 380. The
// "y" is lowered with the rest, and reaches 524 + 192 below the baseline.
func TestPrescriptsAndPostscripts(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, findings := mathLayoutIn(t, face, `<math><mmultiscripts id="m"><mi id="b">x</mi>`+
		`<mn id="sub">1</mn><mn id="sup">2</mn><mprescripts id="p"/><mn id="psub">y</mn><mn id="psup">1</mn>`+
		`</mmultiscripts></math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	asc := style.Unit(704 + 380)
	checkMath(t, root, "m", 1152+512+40, asc, 524+192, map[string][2]style.Unit{
		"psub": {40, asc + 524 - 448}, "psup": {40, 0},
		"b": {552, asc - 448}, "sub": {1092, asc + 524 - 704}, "sup": {1152, 0},
	})
}

// TestScriptsThatAreNotTheRightChildrenAreARow: MathML Core lays out an
// element with the wrong children as a row, and the markup is reported.
func TestScriptsThatAreNotTheRightChildrenAreARow(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	for _, doc := range []string{
		`<math><mfrac id="e"><mn>1</mn><mn>1</mn><mn>1</mn></mfrac></math>`,
		`<math><msub id="e"><mn>1</mn><mn>1</mn><mn>1</mn></msub></math>`,
		`<math><msubsup id="e"><mn>1</mn><mn>1</mn></msubsup></math>`,
		`<math><munder id="e"><mn>1</mn><mn>1</mn><mn>1</mn></munder></math>`,
		`<math><munderover id="e"><mn>1</mn><mn>1</mn><mn>1</mn><mn>1</mn></munderover></math>`,
		`<math><mmultiscripts id="e"><mn>1</mn><mn>1</mn></mmultiscripts></math>`,
		`<math><mmultiscripts id="e"><mn>1</mn><mprescripts/><mn>1</mn><mn>1</mn><mprescripts/></mmultiscripts></math>`,
		`<math><mmultiscripts id="e"><mprescripts/><mn>1</mn><mn>1</mn></mmultiscripts></math>`,
		`<math><mmultiscripts id="e"><mn>1</mn><mn>1</mn><mprescripts/><mn>1</mn><mn>1</mn></mmultiscripts></math>`,
	} {
		root, findings := mathLayoutIn(t, face, doc)
		if !mathFinding(findings, RuleInvalidMarkup, "as a row") {
			t.Errorf("%s: not reported: %v", doc, findings)
		}
		if w := find(t, root, "e").ContentRect().W; w != 512*3 && w != 512*4 && w != 512*2 {
			t.Errorf("%s: %d wide, not a row of its children", doc, w)
		}
	}
	root, _ := mathLayoutIn(t, face, `<math><mfrac id="e"><mn>1</mn><mn>1</mn><mn>1</mn></mfrac></math>`)
	if w := find(t, root, "e").ContentRect().W; w != 512*3 {
		t.Errorf("a fraction of three is %d wide, want the row's %d", w, 512*3)
	}
}

// The under- and overscript constants.
var mathUnderOverConstants = map[string]int{
	"UnderbarVerticalGap": 30, "UnderbarExtraDescender": 20, "OverbarVerticalGap": 40,
	"OverbarExtraAscender": 25, "AccentBaseHeight": 500, "LowerLimitGapMin": 70,
	"LowerLimitBaselineDropMin": 600, "UpperLimitGapMin": 80, "UpperLimitBaselineRiseMin": 650,
	"StretchStackGapAboveMin": 11, "StretchStackBottomShiftDown": 12,
	"StretchStackGapBelowMin": 13, "StretchStackTopShiftUp": 14,
}

// TestUnderAndOverscriptsStackOnTheirBase is §3.4.2 on a "1", 704 tall, with
// a "y" under it and a "2" over it: the y's top 30 below the 1's bottom and
// 20 of extra descender under the y; the 2's bottom 40 above the 1's top and
// 25 of extra ascender over it. An accent sits at AccentBaseHeight over a
// short base — the x, 448 tall — and on a tall one; an accent under touches.
func TestUnderAndOverscriptsStackOnTheirBase(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	root, findings := mathLayoutIn(t, face, `<math display="block">`+
		`<munderover id="uo"><mn id="b">1</mn><mn id="u">y</mn><mn id="o">2</mn></munderover>`+
		`<mover id="a1" accent="true"><mn>x</mn><mn id="o1">2</mn></mover>`+
		`<mover id="a2" accent="TRUE"><mn>1</mn><mn id="o2">2</mn></mover>`+
		`<munder id="a3" accentunder="true"><mn>1</mn><mn id="u3">y</mn></munder>`+
		`</math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	asc := style.Unit(704 + 744 + 25)
	checkMath(t, root, "uo", 512, asc, 30+448+192+20, map[string][2]style.Unit{
		"b": {0, asc - 704}, "u": {0, asc + 30}, "o": {0, 25},
	})
	checkMath(t, root, "a1", 512, 500+704+25, 0, map[string][2]style.Unit{"o1": {0, 25}})
	checkMath(t, root, "a2", 512, 704+704+25, 0, map[string][2]style.Unit{"o2": {0, 25}})
	checkMath(t, root, "a3", 512, 704, 448+192+20, map[string][2]style.Unit{"u3": {0, 704}})
}

// TestLimitsAreSetByTheLimitConstants: under and over the integral, a large
// operator, the limits are placed by LowerLimit… and UpperLimit…: the "1"
// below drops max(600, 70 + its 704) = 774 below the integral's ink, which
// reaches 200 below the baseline; the "2" above rises max(650, 80) = 650
// above the integral's 800. The integral leans 100, and its limits are 50
// apart either way.
func TestLimitsAreSetByTheLimitConstants(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	root, _ := mathLayoutIn(t, face, `<math display="block"><munderover id="l">`+
		`<mo id="b">∫</mo><mn id="u">1</mn><mn id="o">2</mn></munderover></math>`)
	asc := style.Unit(800 + 650 + 704)
	// The integral is 400 wide from −200; the "1" from −256 − 50, the "2"
	// from −256 + 50: 612 from −306.
	checkMath(t, root, "l", 612, asc, 200+774, map[string][2]style.Unit{
		"b": {106, asc - 800}, "u": {0, asc + 200 + 774 - 704}, "o": {100, 0},
	})
}

// TestMovableLimitsAreScriptsInCompactMathematics: §3.4.2.1, a ∑ with
// movablelimits in inline mathematics has its limits as scripts; in display
// mathematics, under and over it.
func TestMovableLimitsAreScriptsInCompactMathematics(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math><munder id="m"><mo>∑</mo><mn id="u">1</mn></munder></math>`)
	if g := geomIn(t, root, "u", "m"); g.x != 800 {
		t.Errorf("the limit of an inline sum is at %d, want 800, beside it", g.x)
	}
	root, _ = mathLayoutIn(t, face, `<math display="block"><munder id="m"><mo>∑</mo><mn id="u">1</mn></munder></math>`)
	if g := geomIn(t, root, "u", "m"); g.x != 144 {
		t.Errorf("the limit of a display sum is at %d, want 144, under it", g.x)
	}
	// An operator that is not movable keeps its limits under it inline.
	root, _ = mathLayoutIn(t, face, `<math><munder id="m"><mo movablelimits="false">∑</mo><mn id="u">1</mn></munder></math>`)
	if g := geomIn(t, root, "u", "m"); g.x != 144 {
		t.Errorf("the limit of a sum that is not movable is at %d, want 144", g.x)
	}
}

// TestAnAccentIsCentredByItsAttachment: the italic x over a "1" is placed so
// that its accent attachment, 300 along it, is over the 1's middle, 256.
func TestAnAccentIsCentredByItsAttachment(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	root, _ := mathLayoutIn(t, face, `<math><mover id="m"><mn id="b">1</mn><mi id="o">x</mi></mover></math>`)
	// The x from −300, the 1 from −256: the x at 0, the 1 at 44; the x ends
	// at 540, the 1 at 556.
	if b, o := geomIn(t, root, "b", "m"), geomIn(t, root, "o", "m"); b.x != 44 || o.x != 0 {
		t.Errorf("the 1 at %d and the x at %d, want 44 and 0", b.x, o.x)
	}
	if w := find(t, root, "m").ContentRect().W; w != 556 {
		t.Errorf("the mover is %d wide, want 556", w)
	}
}

// TestAStretchyOperatorOverABaseIsAskedToReachAcrossIt: §3.4.2.2 lays the
// arrow out last, to the widest of the others — which this change does not
// stretch it to, and says so.
func TestAStretchyOperatorOverABaseIsAskedToReachAcrossIt(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	_, findings := mathLayoutIn(t, face, `<math><mover><mspace width="3em"></mspace><mo>→</mo></mover></math>`)
	if !mathFinding(findings, RuleUnsupportedValue, "stretchy") {
		t.Errorf("an arrow over a wide base is not reported unstretched: %v", findings)
	}
	_, findings = mathLayoutIn(t, face, `<math><mover><mspace width="5px"></mspace><mo>→</mo></mover></math>`)
	if mathFinding(findings, RuleUnsupportedValue, "stretchy") {
		t.Errorf("an arrow over a narrow base is reported: %v", findings)
	}
}

// TestAScriptedFormulaShrinksToItsWidth: the intrinsic sizes of the elements
// with algorithms of their own are the algorithms' — a float holding each is
// as wide as it lays out.
func TestAScriptedFormulaShrinksToItsWidth(t *testing.T) {
	c := map[string]int{"FractionRuleThickness": 64}
	for k, v := range mathScriptConstants {
		c[k] = v
	}
	face := mathFaceWith(t, c)
	for _, inner := range []string{
		`<mfrac><mn>1</mn><mn>11</mn></mfrac>`,
		`<msubsup><mi>x</mi><mn>y</mn><mn>1</mn></msubsup>`,
		`<mmultiscripts><mi>x</mi><mn>1</mn><mn>2</mn><mprescripts/><mn>y</mn><mn>1</mn></mmultiscripts>`,
		`<msub><mo>∫</mo><mn>1</mn></msub>`,
		`<munderover><mo>∫</mo><mn>1</mn><mn>2</mn></munderover>`,
		`<mover><mn>1</mn><mi>x</mi></mover>`,
	} {
		root, _ := mathLayoutIn(t, face, `<div id="f" style="float: left"><math id="m">`+inner+`</math></div>`)
		f, m := find(t, root, "f"), find(t, root, "m")
		if f.BorderRect.W != m.BorderRect.W || m.BorderRect.W == 0 {
			t.Errorf("%s: the float is %d wide and the formula %d", inner, f.BorderRect.W, m.BorderRect.W)
		}
		var widest style.Unit
		for _, c := range find(t, root, "m").Children {
			widest = style.Max(widest, c.MarginRect().W)
		}
		if m.ContentRect().W < widest {
			t.Errorf("%s: the formula is %d wide, narrower than its child's %d", inner, m.ContentRect().W, widest)
		}
	}
}

// TestRightToLeftScriptsAreMirrored: in an rtl formula the subscript is to
// the base's left.
func TestRightToLeftScriptsAreMirrored(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math dir="rtl"><msub id="s"><mi id="b">x</mi><mn id="u">1</mn></msub></math>`)
	w := style.Unit(540 + 512 + 40)
	if b, u := geomIn(t, root, "b", "s"), geomIn(t, root, "u", "s"); b.x != w-540 || u.x != w-540-512 {
		t.Errorf("the base at %d and the subscript at %d, want %d and %d", b.x, u.x, w-540, w-540-512)
	}
	c := map[string]int{"FractionRuleThickness": 64}
	root, _ = mathLayoutIn(t, mathFaceWith(t, c), `<math dir="rtl"><mfrac id="f" style="padding-left: 3px">`+
		`<mn>1</mn><mn>11</mn></mfrac></math>`)
	if got := find(t, root, "f").mathMarks; len(got) != 1 || got[0].X != 0 || got[0].W != 1024 {
		t.Errorf("an rtl fraction's bar is %v, want it across the content box", got)
	}
}

// TestAStretchIsPassedToTheOperatorAnElementEmbellishes: a fraction whose
// numerator is an operator, a script whose base is one, and an under- and
// overscript whose base is one are each the operator, and a row asks each to
// stretch as it would the operator — which each passes to it, and nowhere
// else: not to a denominator, nor to a script.
func TestAStretchIsPassedToTheOperatorAnElementEmbellishes(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	for _, tc := range []struct {
		inner string
		asked bool
	}{
		{`<mfrac><mo>(</mo><mn>1</mn></mfrac>`, true},
		{`<msub><mo>(</mo><mn>1</mn></msub>`, true},
		{`<mmultiscripts><mo>(</mo><mn>1</mn><mn>1</mn></mmultiscripts>`, true},
		{`<munder><mo>(</mo><mn>1</mn></munder>`, true},
		{`<mfrac><mn>1</mn><mo>(</mo></mfrac>`, false},
		{`<msub><mn>1</mn><mo>(</mo></msub>`, false},
	} {
		_, findings := mathLayoutIn(t, face, `<math>`+tc.inner+`<mspace height="4em"></mspace></math>`)
		if got := mathFinding(findings, RuleUnsupportedValue, "stretchy"); got != tc.asked {
			t.Errorf("%s: asked to stretch %v, want %v", tc.inner, got, tc.asked)
		}
	}
	// Only the numerator of a fraction of two operators, only the base of a
	// script that is one.
	for _, inner := range []string{
		`<mfrac><mo id="a">(</mo><mo id="b">(</mo></mfrac>`,
		`<msub><mo id="a">(</mo><mo id="b">(</mo></msub>`,
		`<munder><mo id="a">(</mo><mo id="b">(</mo></munder>`,
	} {
		_, findings := mathLayoutIn(t, face, `<math>`+inner+`<mspace height="4em"></mspace></math>`)
		n := 0
		for _, f := range findings {
			if f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "stretchy") {
				n++
			}
		}
		if n != 1 {
			t.Errorf("%s: %d operators asked to stretch, want the first alone: %v", inner, n, findings)
		}
	}
}

// TestScriptsReachPastTheirShifts: each term of §3.4.1's shifts and extents,
// each made the one that decides by a face whose constants make it so.
func TestScriptsReachPastTheirShifts(t *testing.T) {
	// A base with ink below its baseline pulls its subscript down with it:
	// 50 below the y's 192.
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math><msub id="s"><mn>y</mn><mn id="u">x</mn></msub></math>`)
	if g := geomIn(t, root, "u", "s"); g.top != 448+242-448 {
		t.Errorf("the subscript of a y is at %d, want %d, 242 down", g.top, 448+242-448)
	}
	// A superscript with ink below its baseline is raised to clear it: 300
	// over the y's 192.
	c := map[string]int{}
	for k, v := range mathScriptConstants {
		c[k] = v
	}
	c["SuperscriptBottomMin"] = 300
	root, _ = mathLayoutIn(t, mathFaceWith(t, c), `<math><msup id="s"><mn>x</mn><mn id="o">y</mn></msup></math>`)
	if f := find(t, root, "s"); f.mathBaseline != 492+448 {
		t.Errorf("the superscript y is raised to %d, want 492 + 448", f.mathBaseline)
	}
	// And one raised less than its own depth reaches below the baseline: a
	// least of −100 over the y's 192 is 92, and 100 of the y is below.
	c["SuperscriptBottomMin"], c["SuperscriptShiftUp"], c["SuperscriptBaselineDropMax"] = -100, 50, 5000
	root, _ = mathLayoutIn(t, mathFaceWith(t, c), `<math><msup id="s"><mn>x</mn><mn>y</mn></msup></math>`)
	if f := find(t, root, "s"); f.ContentRect().H.Sub(f.mathBaseline) != 100 {
		t.Errorf("the msup reaches %d below its baseline, want 100", f.ContentRect().H.Sub(f.mathBaseline))
	}
}

// TestEveryPairOfScriptsSharesTheShifts: §3.4.3 shifts every subscript by the
// greatest any pair asks for, whichever pair that is; here the prescripts ask
// more than the postscripts. Columns are as wide as their wider script, and a
// narrower script is at its column's end before the base and its start after
// it; the second column of postscripts starts after the first and its space.
func TestEveryPairOfScriptsSharesTheShifts(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math><mmultiscripts id="m"><mn id="b">x</mn>`+
		`<mn id="s1">1</mn><mn id="p1">11</mn><mn id="s2">y</mn><mn id="p2">1</mn>`+
		`<mprescripts id="pre" style="padding-left: 1px"/><mn id="ps">11</mn><mn id="pp">2</mn>`+
		`</mmultiscripts></math>`)
	// The prescript pair asks 524 and 380 (as the postscripts of
	// TestPrescriptsAndPostscripts did), and so does the first postscript
	// pair; the second, the last, asks only 268 and 380.
	asc := style.Unit(704 + 380)
	checkMath(t, root, "m", 40+1024+512+512+40+1024+40, asc, 524+192, map[string][2]style.Unit{
		"ps": {40, asc + 524 - 704}, "pp": {40 + 512, 0},
		"pre": {1064, asc}, "b": {1064, asc - 448},
		"s1": {1576, asc + 524 - 704}, "p1": {1576, 0},
		"s2": {1576 + 1024 + 40, asc + 524 - 448}, "p2": {1576 + 1024 + 40, 0},
	})
}

// TestUnderAndOverscriptsReachPastTheirShifts: the rest of §3.4.2's terms. A
// stretchy operator of inline axis as the base is stacked by the stretch
// stack constants; one of block axis, a parenthesis, is not, and is not asked
// to stretch across its underscript either. Limits clear a script's own ink
// where their gap is what decides; and a gap below nought lets a script reach
// past the base on the far side.
func TestUnderAndOverscriptsReachPastTheirShifts(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	// The arrow's ink is 200 to 400 up. The 1 under it drops max(12, 11 +
	// 704) = 715 below its ink's bottom; the y over it rises max(14, 13 +
	// 192) = 205 above its top.
	root, _ := mathLayoutIn(t, face, `<math display="block"><munderover id="a"><mo id="b">→</mo>`+
		`<mn id="u">1</mn><mn id="o">y</mn></munderover></math>`)
	asc := style.Unit(400 + 205 + 448)
	checkMath(t, root, "a", 600, asc, 715-200, map[string][2]style.Unit{
		"b": {0, asc - 400}, "u": {44, asc + 715 - 200 - 704}, "o": {44, 0},
	})
	// A parenthesis is under- and overscripted by the bar gaps.
	root, findings := mathLayoutIn(t, face, `<math display="block"><munder id="p"><mo>(</mo>`+
		`<mspace width="3em" height="1px"></mspace></munder></math>`)
	if f := find(t, root, "p"); f.ContentRect().H.Sub(f.mathBaseline) != 256+30+64+20 {
		t.Errorf("the parenthesis's underscript reaches %d down, want the underbar gap's %d",
			f.ContentRect().H.Sub(f.mathBaseline), 256+30+64+20)
	}
	if mathFinding(findings, RuleUnsupportedValue, "stretchy") {
		t.Errorf("a parenthesis is asked to stretch across its underscript: %v", findings)
	}
	_, findings = mathLayoutIn(t, face, `<math display="block"><munder><mspace width="3em" height="1px"></mspace>`+
		`<mo>(</mo></munder></math>`)
	if mathFinding(findings, RuleUnsupportedValue, "stretchy") {
		t.Errorf("a parenthesis under a base is asked to stretch across it: %v", findings)
	}

	c := map[string]int{}
	for k, v := range mathUnderOverConstants {
		c[k] = v
	}
	c["UpperLimitGapMin"] = 600
	// accent="false" is no accent: the bar gap, 40, and not AccentBaseHeight.
	root, _ = mathLayoutIn(t, face, `<math display="block"><mover id="n" accent="false"><mn>x</mn><mn>2</mn></mover></math>`)
	if f := find(t, root, "n"); f.mathBaseline != 448+40+704+25 {
		t.Errorf("an overscript that is not an accent is %d up, want %d", f.mathBaseline, 448+40+704+25)
	}
	root, _ = mathLayoutIn(t, mathFaceWith(t, c), `<math display="block"><mover id="l"><mo>∫</mo><mn>y</mn></mover></math>`)
	if f := find(t, root, "l"); f.mathBaseline != 800+600+192+448 {
		t.Errorf("the upper limit's top is %d up, want %d", f.mathBaseline, 800+600+192+448)
	}

	c["UnderbarVerticalGap"], c["OverbarVerticalGap"] = -2000, -2000
	root, _ = mathLayoutIn(t, mathFaceWith(t, c), `<math display="block">`+
		`<munder id="u"><mn>1</mn><mn>1</mn></munder><mover id="o"><mn>1</mn><mn>y</mn></mover></math>`)
	// The 1 under the 1 is raised 2000 − 704 above the base's bottom, and its
	// top is 2000 up; the y over it is lowered, and its bottom is 2000 − 704
	// − 192 below the baseline.
	if f := find(t, root, "u"); f.mathBaseline != 2000 {
		t.Errorf("the underscript reaches %d up, want 2000", f.mathBaseline)
	}
	if f := find(t, root, "o"); f.ContentRect().H.Sub(f.mathBaseline) != 2000-704-192+192 {
		t.Errorf("the overscript reaches %d down, want %d", f.ContentRect().H.Sub(f.mathBaseline), 2000-704-192+192)
	}
}

// TestAnOverscriptOnAMovableOperatorIsASuperscript: §3.4.2.1's <mover>. The
// sum's ink reaches 800 up, so its superscript is raised 800 − 300.
func TestAnOverscriptOnAMovableOperatorIsASuperscript(t *testing.T) {
	face := mathFaceWith(t, mathScriptConstants)
	root, _ := mathLayoutIn(t, face, `<math><mover id="m"><mo>∑</mo><mn id="o">1</mn></mover></math>`)
	if g, f := geomIn(t, root, "o", "m"), find(t, root, "m"); g.x != 800 || g.top != 0 || f.mathBaseline != 704+500 {
		t.Errorf("the overscript of an inline sum is at (%d, %d) with the sum's baseline %d down, want (800, 0) and %d",
			g.x, g.top, f.mathBaseline, 704+500)
	}
}

// TestAStretchyOperatorIsStretchedToTheWidestOfTheOthers: §3.4.2.2's target
// is the widest child laid out before the operators, not the last of them.
func TestAStretchyOperatorIsStretchedToTheWidestOfTheOthers(t *testing.T) {
	face := mathFaceWith(t, mathUnderOverConstants)
	_, findings := mathLayoutIn(t, face, `<math><munderover><mspace width="3em"></mspace><mo>→</mo><mn>1</mn></munderover></math>`)
	if !mathFinding(findings, RuleUnsupportedValue, "stretchy") {
		t.Errorf("an arrow under a wide base, over a narrow script, is not asked to reach the base: %v", findings)
	}
}

// TestAPrescriptColumnEndsAtTheBase: a prescript narrower than its column is
// at the column's end, beside the base.
func TestAPrescriptColumnEndsAtTheBase(t *testing.T) {
	root, _ := mathLayoutIn(t, mathFaceWith(t, mathScriptConstants), `<math><mmultiscripts id="m"><mn>x</mn>`+
		`<mprescripts/><mn id="ps">1</mn><mn id="pp">11</mn></mmultiscripts></math>`)
	if ps, pp := geomIn(t, root, "ps", "m"), geomIn(t, root, "pp", "m"); ps.x != 40+512 || pp.x != 40 {
		t.Errorf("the prescripts are at %d and %d, want %d and 40", ps.x, pp.x, 40+512)
	}
}

// TestAnElementsIntrinsicWidthIsItsLayoutsWidth: every element's min-content
// and max-content sizes are its inline size laid out — nothing in a formula
// wraps — for each algorithm, with the italic corrections and accent
// attachments its inline half reads, and the edges that move them.
func TestAnElementsIntrinsicWidthIsItsLayoutsWidth(t *testing.T) {
	c := map[string]int{"FractionRuleThickness": 64}
	for k, v := range mathScriptConstants {
		c[k] = v
	}
	face := mathFaceWith(t, c)
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	for _, inner := range []string{
		`<mfrac id="e"><mn>1</mn><mn>11</mn></mfrac>`,
		`<msubsup id="e"><mi style="padding-right: 3px">x</mi><mn>y</mn><mn>1</mn></msubsup>`,
		`<mmultiscripts id="e"><mi>x</mi><mn>1</mn><mn>2</mn><mprescripts/><mn>y</mn><mn>11</mn></mmultiscripts>`,
		`<msub id="e"><mo>∫</mo><mn>1</mn></msub>`,
		`<munderover id="e"><mo>∫</mo><mn>1</mn><mn>2</mn></munderover>`,
		`<mover id="e"><mn>1</mn><mi>x</mi></mover>`,
		`<mover id="e"><mn>1</mn><mi style="padding-left: 5px">x</mi></mover>`,
		`<munder id="e" style="padding: 2px"><mo>∑</mo><mn>11</mn></munder>`,
	} {
		b := Build(Input{HTML: `<math display="block">` + inner + `</math>`, Fonts: set,
			CSS: []Stylesheet{{Source: `mn, mi, mo { font-size: 16px }`}}})
		l := newLayouter(b.Root, A4.Content(), set, nil)
		var e *Box
		var walk func(*Box)
		walk = func(x *Box) {
			if x.Element != nil {
				if id, _ := x.Element.Attr("id"); id == "e" {
					e = x
				}
			}
			for _, k := range x.Children {
				walk(k)
			}
		}
		walk(b.Root)
		size := l.mathContentSize(e)
		laid := l.mathContentOf(e, 0, mathStretch{})
		if size.min != laid.width || size.max != laid.width {
			t.Errorf("%s: sizes %d and %d, laid out %d", inner, size.min, size.max, laid.width)
		}
	}
	// A child laid out as CSS has a min-content size less than its
	// max-content one — "a b" in an inline-block is as narrow as its wider
	// word, 512, and as wide as the three characters, 1280 — and each
	// algorithm's is from its children's of the same kind.
	for _, inner := range []string{
		`<msub id="e"><mtext style="display: inline-block">a b</mtext><mn>1</mn></msub>`,
		`<munder id="e"><mtext style="display: inline-block">a b</mtext><mn>1</mn></munder>`,
	} {
		b := Build(Input{HTML: `<math display="block">` + inner + `</math>`, Fonts: set,
			CSS: []Stylesheet{{Source: `mn, mi, mo { font-size: 16px }`}}})
		l := newLayouter(b.Root, A4.Content(), set, nil)
		var e *Box
		var walk func(*Box)
		walk = func(x *Box) {
			if x.Element != nil {
				if id, _ := x.Element.Attr("id"); id == "e" {
					e = x
				}
			}
			for _, k := range x.Children {
				walk(k)
			}
		}
		walk(b.Root)
		if size := l.mathContentSize(e); size.max.Sub(size.min) != 768 {
			t.Errorf("%s: min-content %d and max-content %d, want 768 apart", inner, size.min, size.max)
		}
	}
}
