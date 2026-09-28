package layout

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The layout of MathML, in exact arithmetic.
//
// The face below is 1024 units to the em and the formulas are set at 16px, so
// one font unit is one sixty-fourth of a pixel — exactly one style.Unit. Every
// width, ascent and offset the algorithms produce is then a whole number of
// units a reader can work out from the glyph boxes and MATH constants here,
// and is asserted as that number.

// The glyphs of the math face: what each character is and where its ink is,
// in font units.
var mathTestGlyphs = []fonttest.Glyph{
	{Rune: ' ', Advance: 256},
	{Rune: 'x', Advance: 512, HasShape: true, Ink: [4]int{0, 0, 512, 448}},
	{Rune: 0x1D465, Advance: 540, HasShape: true, Ink: [4]int{20, 0, 600, 448}},   // 𝑥, italic x
	{Rune: 0x1D453, Advance: 320, HasShape: true, Ink: [4]int{0, -128, 420, 704}}, // 𝑓
	{Rune: '1', Advance: 512, HasShape: true, Ink: [4]int{64, 0, 448, 704}},
	{Rune: '2', Advance: 512, HasShape: true, Ink: [4]int{64, 0, 448, 704}},
	{Rune: '+', Advance: 768, HasShape: true, Ink: [4]int{64, 64, 704, 576}},
	{Rune: '-', Advance: 512, HasShape: true, Ink: [4]int{64, 288, 448, 352}},
	{Rune: 'a', Advance: 512, HasShape: true, Ink: [4]int{0, 0, 512, 448}},
	{Rune: 'b', Advance: 512, HasShape: true, Ink: [4]int{0, 0, 512, 704}},
	{Rune: 'y', Advance: 512, HasShape: true, Ink: [4]int{0, -192, 512, 448}},
	{Rune: '(', Advance: 384, HasShape: true, Ink: [4]int{64, -256, 320, 768}},
	{Rune: ')', Advance: 384, HasShape: true, Ink: [4]int{64, -256, 320, 768}},
	{Rune: 0x222B, Advance: 400, HasShape: true, Ink: [4]int{0, -200, 500, 800}}, // ∫
	{Rune: 0x2211, Advance: 800, HasShape: true, Ink: [4]int{0, -200, 800, 800}}, // ∑
	{Rune: 0x2192, Advance: 600, HasShape: true, Ink: [4]int{0, 200, 600, 400}},  // →
	// The radical sign, 900 tall, and the forms larger than their text that a
	// stretchy face (mathStretchFace) builds constructions from. No character
	// maps to any of them after the sign.
	{Rune: 0x221A, Advance: 600, HasShape: true, Ink: [4]int{0, -100, 600, 800}},      // 17 √
	{Unmapped: true, Advance: 650, HasShape: true, Ink: [4]int{0, -300, 650, 1200}},   // 18 √, 1500 tall
	{Unmapped: true, Advance: 700, HasShape: true, Ink: [4]int{0, 0, 700, 600}},       // 19 √'s bottom
	{Unmapped: true, Advance: 700, HasShape: true, Ink: [4]int{500, 0, 600, 400}},     // 20 √'s extender
	{Unmapped: true, Advance: 700, HasShape: true, Ink: [4]int{500, 0, 700, 600}},     // 21 √'s top
	{Unmapped: true, Advance: 450, HasShape: true, Ink: [4]int{50, -500, 400, 1100}},  // 22 (, 1600 tall
	{Unmapped: true, Advance: 500, HasShape: true, Ink: [4]int{50, 0, 450, 600}},      // 23 ('s bottom
	{Unmapped: true, Advance: 500, HasShape: true, Ink: [4]int{50, 0, 150, 400}},      // 24 ('s extender
	{Unmapped: true, Advance: 500, HasShape: true, Ink: [4]int{50, 0, 450, 600}},      // 25 ('s top
	{Unmapped: true, Advance: 1200, HasShape: true, Ink: [4]int{0, -400, 1200, 1200}}, // 26 ∑ in display
	{Unmapped: true, Advance: 1200, HasShape: true, Ink: [4]int{0, 200, 1200, 400}},   // 27 →, 1200 long
	{Unmapped: true, Advance: 460, HasShape: true, Ink: [4]int{50, -500, 410, 1100}},  // 28 ), 1600 tall
	{Unmapped: true, Advance: 1500, HasShape: true, Ink: [4]int{0, -500, 1500, 1500}}, // 29 ∑, 2000 tall
}

// Glyph indices in the face: one more than the position above.
const (
	mathGlyphItalicX  = 3
	mathGlyphItalicF  = 4
	mathGlyphIntegral = 14
	mathGlyphParen    = 12
	mathGlyphCloser   = 13
	mathGlyphSum      = 15
	mathGlyphArrow    = 16
	mathGlyphRadical  = 17
)

func mathTestFace(t testing.TB) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "MathTest", UnitsPerEm: 1024, Ascent: 1024, Descent: -256,
		Glyphs: mathTestGlyphs,
		Extra: map[string][]byte{"MATH": fonttest.MATH(fonttest.MathOptions{
			Constants: map[string]int{
				"ScriptPercentScaleDown": 70, "ScriptScriptPercentScaleDown": 50,
				"AxisHeight": 256,
			},
			ItalicsCorrection:   map[int]int{mathGlyphItalicX: 60, mathGlyphItalicF: 100},
			TopAccentAttachment: map[int]int{mathGlyphItalicX: 300},
		})},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// mathLayout lays a document out with the math face as the "math" family, at
// 16px, on a page 600px wide.
func mathLayout(t *testing.T, doc string) (*Fragment, []Finding) {
	t.Helper()
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	b := Build(Input{HTML: doc, CSS: []Stylesheet{{Source: `body { margin: 0; font-size: 16px }`}}, Fonts: set})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	frag := Layout(b.Root, Size{W: w, H: h}, set, rec)
	return frag, append(b.Findings, rec.Findings()...)
}

// at is a fragment's border box, from the content box of another.
func at(of, in *Fragment) (x, y style.Unit) {
	c := in.ContentRect()
	return of.BorderRect.X.Sub(c.X), of.BorderRect.Y.Sub(c.Y)
}

// TestARowIsSpacedAndItsItalicCorrected is §3.3.1.2 on "𝑥 + 1": the italic x
// is 540 wide and leans 60 past it, and is followed by that 60 because the
// "+" after it is not slanted; the "+" is an infix operator of category B,
// spaced by 4/18 of an em either side — 227 units of 1024 at 16px, cut to the
// unit; the "1" follows. The row is as tall as its tallest ink, the "1"'s 704,
// and reaches nothing below the baseline: no child's ink does.
func TestARowIsSpacedAndItsItalicCorrected(t *testing.T) {
	root, findings := mathLayout(t, `<math id="m"><mi id="x">x</mi><mo id="p">+</mo><mn id="n">1</mn></math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	m := find(t, root, "m")
	want := map[string][2]style.Unit{
		// id: x offset, y offset (top of the box, from the top of the row)
		"x": {0, 704 - 448},
		"p": {540 + 60 + 227, 704 - 576},
		"n": {540 + 60 + 227 + 768 + 227, 0},
	}
	for id, w := range want {
		x, y := at(find(t, root, id), m)
		if x != w[0] || y != w[1] {
			t.Errorf("#%s at (%d, %d), want (%d, %d)", id, x, y, w[0], w[1])
		}
	}
	if c := m.ContentRect(); c.W != 540+60+227+768+227+512 || c.H != 704 {
		t.Errorf("the row is %d by %d, want %d by 704", c.W, c.H, 540+60+227+768+227+512)
	}
	if !m.hasMathBaseline || m.mathBaseline != 704 {
		t.Errorf("the row's baseline is %d down (%v), want 704", m.mathBaseline, m.hasMathBaseline)
	}
	// The glyph's own box, not the line box: the x is 448 tall, and its text
	// sits on the box's bottom edge.
	if x := find(t, root, "x"); x.BorderRect.H != 448 || x.Lines[0].Rect.Y.Add(x.Lines[0].Baseline) != 448 {
		t.Errorf("the x is %d tall with its baseline at %d, want 448 and 448",
			x.BorderRect.H, x.Lines[0].Rect.Y.Add(x.Lines[0].Baseline))
	}
}

// TestAnOperatorsFormDecidesItsSpacing: a "-" first in a row of several is a
// prefix operator, of category D, with no space either side; one between two
// operands is infix, B; a "+" last is postfix, which the dictionary has no
// "+" for, so §3.2.4.2 looks it up as infix and spaces it as B; and the
// attributes override all three — lspace in ems, rspace as a percentage of
// the dictionary's value, and form.
func TestAnOperatorsFormDecidesItsSpacing(t *testing.T) {
	root, _ := mathLayout(t, `<math>`+
		`<mrow id="r1"><mo>-</mo><mn id="a1">1</mn></mrow>`+
		`<mrow id="r2"><mn>1</mn><mo>-</mo><mn id="a2">1</mn></mrow>`+
		`<mrow id="r3"><mn>1</mn><mo id="o3">+</mo></mrow>`+
		`<mrow id="r4"><mo lspace="1em" rspace="50%" form="infix">-</mo><mn id="a4">1</mn></mrow>`+
		`<mrow id="r5"><mrow><mo>-</mo></mrow><mn id="a5">1</mn></mrow>`+
		`<mrow id="r6"><mspace></mspace><mo>-</mo><mn id="a6">1</mn></mrow>`+
		`<mrow id="r7"><mn>1</mn><mo form="prefix">+</mo><mn id="a7">1</mn></mrow>`+
		`</math>`)
	b := style.Unit(1024).Mul(4.0 / 18) // 227
	for _, tc := range []struct {
		row, id string
		x       style.Unit
	}{
		{"r1", "a1", 512},
		{"r2", "a2", 512 + b + 512 + b},
		{"r3", "o3", 512 + b},
		{"r4", "a4", 1024 + 512 + b.Mul(0.5)},
		// The inner row is the embellished operator the outer row sees, and
		// its form is where it stands — first, so prefix, so no space — and
		// not where its <mo> stands inside it, alone, which is infix. Inside,
		// an embellished operator adds no space of its own.
		{"r5", "a5", 512},
		// First among the children that are not space-like.
		{"r6", "a6", 512},
		{"r7", "a7", 512 + 768},
	} {
		if x, _ := at(find(t, root, tc.id), find(t, root, tc.row)); x != tc.x {
			t.Errorf("#%s at %d in #%s, want %d", tc.id, x, tc.row, tc.x)
		}
	}
}

// TestAFormulaSitsOnTheLinesBaseline: an inline <math> is an atomic inline
// whose baseline is its alphabetic baseline, and a block one is a block whose
// content is centred.
func TestAFormulaSitsOnTheLinesBaseline(t *testing.T) {
	root, _ := mathLayout(t, `<p id="p">a<math id="m"><mn>1</mn><mi mathvariant="normal">y</mi></math>b</p>`+
		`<div id="d" style="width: 200px"><math id="b" display="block"><mn id="n">1</mn></math></div>`)
	p := find(t, root, "p")
	m := find(t, root, "m")
	line := p.Lines[0]
	lineBase := p.ContentRect().Y.Add(line.Rect.Y).Add(line.Baseline)
	if got := m.BorderRect.Y.Add(m.mathBaseline); got != lineBase {
		t.Errorf("the formula's baseline is at %d, the line's at %d", got, lineBase)
	}
	// The "y" reaches 192 below the baseline, the "1" 704 above.
	if m.BorderRect.H != 704+192 {
		t.Errorf("the formula is %d tall, want %d", m.BorderRect.H, 704+192)
	}
	// 200px is 12800 units; the "1" is 512 wide, and centred.
	if x, _ := at(find(t, root, "n"), find(t, root, "b")); x != (12800-512)/2 {
		t.Errorf("the block formula's content is at %d, want %d", x, (12800-512)/2)
	}
}

// TestSpaceAndPadding: §3.2.5's <mspace> and §3.3.6's <mpadded>.
func TestSpaceAndPadding(t *testing.T) {
	root, _ := mathLayout(t, `<math id="m">`+
		`<mspace id="s" width="2em" height="1em" depth="0.5em"></mspace>`+
		`<mpadded id="p" width="20px" height="3px" depth="4px" lspace="5px" voffset="6px">`+
		`<mn id="n">1</mn></mpadded>`+
		`<mpadded id="q" height="10%" depth="bad"><mn>1</mn></mpadded></math>`)
	s := find(t, root, "s")
	if s.BorderRect.W != 2048 || s.BorderRect.H != 1024+512 || s.mathBaseline != 1024 {
		t.Errorf("mspace is %dx%d with its baseline %d down, want 2048x1536 and 1024",
			s.BorderRect.W, s.BorderRect.H, s.mathBaseline)
	}
	p := find(t, root, "p")
	if p.BorderRect.W != 20*64 || p.BorderRect.H != 7*64 || p.mathBaseline != 3*64 {
		t.Errorf("mpadded is %dx%d with its baseline %d down, want 1280x448 and 192",
			p.BorderRect.W, p.BorderRect.H, p.mathBaseline)
	}
	// Its row starts 5px in, with its baseline 6px up: the "1"'s 704 of ink
	// top is 704 + 384 above the mpadded's baseline, which is 192 down.
	if x, y := at(find(t, root, "n"), p); x != 5*64 || y != 192-384-704 {
		t.Errorf("the mpadded's content is at (%d, %d), want (320, %d)", x, y, 192-384-704)
	}
	// A percentage or a value that does not read is no height or depth at
	// all: the row's own ascent and descent stand.
	if q := find(t, root, "q"); q.BorderRect.H != 704 || q.mathBaseline != 704 {
		t.Errorf("the second mpadded is %d tall, baseline %d; want the row's 704", q.BorderRect.H, q.mathBaseline)
	}
}

// TestAPhantomIsLaidOutAndNotDrawn: <mphantom> is the row it holds, invisible.
func TestAPhantomIsLaidOutAndNotDrawn(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	out := Compose(Input{HTML: `<math><mphantom id="ph"><mn>2</mn></mphantom><mn>1</mn></math>`, Fonts: set}, Options{})
	var texts []string
	for _, op := range out.Ops {
		if d, ok := op.(DrawText); ok {
			texts = append(texts, d.Text)
		}
	}
	if got := strings.Join(texts, ","); got != "1" {
		t.Errorf("drawn text %q, want only the 1", got)
	}
}

// TestMerrorIsFramed: the user agent sheet's red border and light yellow
// background, on the fragment of the element.
func TestMerrorIsFramed(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	out := Compose(Input{HTML: `<math><merror><mn>1</mn></merror></math>`, Fonts: set}, Options{})
	var yellow, red bool
	for _, op := range out.Ops {
		if f, ok := op.(FillRect); ok {
			c := f.Color
			yellow = yellow || c.R == 255 && c.G == 255 && c.B == 224
			red = red || c.R == 255 && c.G == 0 && c.B == 0
		}
	}
	if !yellow || !red {
		t.Errorf("the error's background drawn %v and its border %v", yellow, red)
	}
}

// TestAFormulasIntrinsicWidthIsItsLayoutsWidth: a float holding a formula
// shrinks to the formula's inline size, the italic correction included — the
// min-content and max-content sizes of §3.3.1.2 — and the formula fits in it.
func TestAFormulasIntrinsicWidthIsItsLayoutsWidth(t *testing.T) {
	root, _ := mathLayout(t, `<div id="f" style="float: left"><math id="m"><mi>x</mi><mn>1</mn></math></div>`)
	f := find(t, root, "f")
	m := find(t, root, "m")
	if f.BorderRect.W != 540+60+512 || m.BorderRect.W != 540+60+512 {
		t.Errorf("the float is %d wide and the formula %d, want both %d", f.BorderRect.W, m.BorderRect.W, 540+60+512)
	}
}

// TestARightToLeftFormulaIsMirrored: the row's first child is at its right end.
func TestARightToLeftFormulaIsMirrored(t *testing.T) {
	root, _ := mathLayout(t, `<math id="m" dir="rtl"><mn id="a">1</mn><mn id="b">2</mn><mi id="c">x</mi></math>`)
	m := find(t, root, "m")
	w := style.Unit(512 + 512 + 540 + 60)
	for id, x := range map[string]style.Unit{"a": w - 512, "b": w - 1024, "c": w - 1024 - 540} {
		if got, _ := at(find(t, root, id), m); got != x {
			t.Errorf("#%s at %d, want %d", id, got, x)
		}
	}
}

// TestTextOutsideATokenIsReported: MathML places elements; text written
// straight into a row is not drawn and is said to be, and so is anything
// written inside an <mspace>. White space between elements is neither.
func TestTextOutsideATokenIsReported(t *testing.T) {
	_, findings := mathLayout(t, "<math>\n <mrow> x <mn>1</mn> </mrow><mspace>y</mspace>\n</math>")
	var saw []string
	for _, f := range findings {
		if f.Rule == RuleInvalidMarkup {
			saw = append(saw, f.Message)
		}
	}
	joined := strings.Join(saw, "\n")
	if len(saw) != 2 || !strings.Contains(joined, "inside <mrow>") || !strings.Contains(joined, "<mspace> is empty") {
		t.Errorf("findings %q, want the text in the row and in the space", saw)
	}
	// An element inside an <mspace> is not laid out either.
	root, findings := mathLayout(t, `<math><mspace width="1em"><mn id="n">1</mn></mspace></math>`)
	reported := false
	for _, f := range findings {
		reported = reported || f.Rule == RuleInvalidMarkup && strings.Contains(f.Message, "<mspace> is empty")
	}
	var walk func(*Fragment) bool
	walk = func(f *Fragment) bool {
		if f.Box.Element != nil {
			if id, _ := f.Box.Element.Attr("id"); id == "n" {
				return true
			}
		}
		for _, c := range f.Children {
			if walk(c) {
				return true
			}
		}
		return false
	}
	if !reported || walk(root) {
		t.Errorf("an element inside an mspace: reported %v, laid out %v", reported, walk(root))
	}
}

// TestAMathMLBoxWithAnotherDisplayIsLaidOutAsCSS: an author's "display:
// block" on an <mrow> lays it out as a block, placed in the formula as a box
// on its first baseline; and a token holding HTML is the block it is.
func TestAMathMLBoxWithAnotherDisplayIsLaidOutAsCSS(t *testing.T) {
	root, _ := mathLayout(t, `<math id="m"><mn id="n">1</mn>`+
		`<mrow id="r" style="display: block; font: 10px serif; line-height: 20px">`+
		`<mn id="r2">2</mn><mi mathvariant="normal">y</mi></mrow>`+
		`<mtext id="t" style="padding-top: 10px; border-top: 3px solid"><b>b</b></mtext></math>`)
	m := find(t, root, "m")
	// The block holds one MathML box, whose baseline is the block's first:
	// the row aligns that with the "1"'s. (Measured on the page, where every
	// fragment's position is its own.)
	lineAt := func(f *Fragment) style.Unit {
		return f.ContentRect().Y.Add(f.Lines[0].Rect.Y).Add(f.Lines[0].Baseline)
	}
	if got, want := lineAt(find(t, root, "r2")), lineAt(find(t, root, "n")); got != want {
		t.Errorf("the block's 2 sits at %d, the formula's 1 at %d", got, want)
	}
	// The mtext's own line, on the page, against the formula's baseline.
	tt := find(t, root, "t")
	lineBase := tt.ContentRect().Y.Add(tt.Lines[0].Rect.Y).Add(tt.Lines[0].Baseline)
	if want := m.ContentRect().Y.Add(m.mathBaseline); lineBase != want {
		t.Errorf("the mtext's baseline is at %d, the formula's at %d", lineBase, want)
	}
}

// TestTheEmbellishedOperatorsAndTheSpaceLike is §3.2.4.1 and §3.2.5.1.
func TestTheEmbellishedOperatorsAndTheSpaceLike(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	b := Build(Input{HTML: `<math>` +
		`<mrow id="g"><mo>+</mo></mrow>` +
		`<mrow id="gs"><mspace></mspace><mo>+</mo><mtext></mtext></mrow>` +
		`<mrow id="gg"><mo>+</mo><mo>-</mo></mrow>` +
		`<msub id="s"><mo>+</mo><mi>i</mi></msub>` +
		`<msub id="s2"><mi>i</mi><mo>+</mo></msub>` +
		`<mfrac id="f"><mo>+</mo><mi>i</mi></mfrac>` +
		`<mpadded id="p"><mo>+</mo></mpadded>` +
		`<msqrt id="q"><mo>+</mo></msqrt>` +
		`<mrow id="sp"><mtext></mtext><mspace></mspace></mrow>` +
		`<mrow id="e"></mrow><mphantom id="ph"><mspace></mspace></mphantom>` +
		`</math>`, Fonts: set})
	l := newLayouter(b.Root, A4.Content(), set, nil)
	boxes := map[string]*Box{}
	var walk func(*Box)
	walk = func(x *Box) {
		if x.Element != nil {
			if id, ok := x.Element.Attr("id"); ok {
				boxes[id] = x
			}
		}
		for _, c := range x.Children {
			walk(c)
		}
	}
	walk(b.Root)
	for id, want := range map[string][2]bool{ // embellished, space-like
		"g": {true, false}, "gs": {true, false}, "gg": {false, false},
		"s": {true, false}, "s2": {false, false}, "f": {true, false}, "p": {true, false},
		"q": {false, false}, "sp": {false, true}, "e": {false, true}, "ph": {false, true},
	} {
		c := l.mathClassOf(boxes[id])
		if c.embellished != want[0] || c.spaceLike != want[1] {
			t.Errorf("#%s embellished %v space-like %v, want %v %v", id, c.embellished, c.spaceLike, want[0], want[1])
		}
	}
}

// TestAFloatInAFormulaDoesNotFloat: §2.2.2, a child of a MathML box laid out
// as math is placed by its parent's algorithm whatever its float says.
func TestAFloatInAFormulaDoesNotFloat(t *testing.T) {
	root, _ := mathLayout(t, `<math id="m"><mn>1</mn><mn id="f" style="float: right">2</mn></math>`)
	if x, _ := at(find(t, root, "f"), find(t, root, "m")); x != 512 {
		t.Errorf("the floated 2 is at %d, want 512, beside the 1", x)
	}
}

// TestWhatAFormulaCannotDoIsReported: an operator a face has no larger form
// of is not stretched, and a face with no MATH table sets a formula by the
// fallbacks; each says so. A formula that asks for neither reports nothing.
func TestWhatAFormulaCannotDoIsReported(t *testing.T) {
	has := func(findings []Finding, rule Rule, text string) bool {
		for _, f := range findings {
			if f.Rule == rule && strings.Contains(f.Message, text) {
				return true
			}
		}
		return false
	}
	_, quiet := mathLayout(t, `<math><mrow><mo>(</mo><mn>1</mn><mo>)</mo></mrow></math>`)
	for _, f := range quiet {
		t.Errorf("a formula this change lays out whole reported %v", f)
	}
	// This face has a MATH table and no larger forms of anything: an
	// operator asked to be larger than its glyph is drawn at its text size,
	// as §3.2.4.3 says, and said to be.
	_, got := mathLayout(t, `<math><mo>(</mo><mspace height="3em"></mspace></math>`)
	if !has(got, RuleMathFallback, "no larger forms of U+0028") {
		t.Errorf("an operator that should stretch is not reported: %v", got)
	}
	_, got = mathLayout(t, `<math display="block"><mo>∑</mo></math>`)
	if !has(got, RuleMathFallback, "no larger forms of U+2211") {
		t.Errorf("a large operator in display is not reported: %v", got)
	}
	_, got = mathLayout(t, `<math><mo>∑</mo></math>`)
	if has(got, RuleMathFallback, "no larger forms") {
		t.Errorf("a large operator in inline mathematics is reported: %v", got)
	}
	_, got = mathLayout(t, `<math><mo>(</mo><mspace depth="2em"></mspace></math>`)
	if !has(got, RuleMathFallback, "no larger forms") {
		t.Errorf("an operator that should stretch below the baseline is not reported: %v", got)
	}
	_, got = mathLayout(t, `<math><mo stretchy="false">(</mo><mspace height="3em"></mspace></math>`)
	if has(got, RuleMathFallback, "no larger forms") {
		t.Errorf("an operator that is not stretchy is reported as not stretched: %v", got)
	}
	_, got = mathLayout(t, `<math style="font-family: serif"><mn>1</mn></math>`)
	if !has(got, RuleMathFallback, "has no MATH table") {
		t.Errorf("a formula in a face with no MATH table is not reported: %v", got)
	} else {
		fired[RuleMathFallback] = true
	}

	// A MATH table part of which cannot be read is read for the rest, and
	// what was not read is reported.
	math := fonttest.MATH(fonttest.MathOptions{Constants: map[string]int{}})
	binary.BigEndian.PutUint16(math[4:], uint16(len(math)-4)) // MathConstants past the end
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: mathTestGlyphs,
		UnitsPerEm: 1024, Extra: map[string][]byte{"MATH": math}}))
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mn>1</mn></math>`, Fonts: set})
	rec := NewRecorder(nil)
	Layout(b.Root, A4.Content(), set, rec)
	if !has(rec.Findings(), RuleLimit, "MathConstants") {
		t.Errorf("a MATH table read in part is not reported: %v", rec.Findings())
	}
}

// TestAnUnreadableMathTableIsReported: a face whose MATH table is of a version
// this engine does not read is laid out by the fallbacks and says why.
func TestAnUnreadableMathTableIsReported(t *testing.T) {
	math := fonttest.MATH(fonttest.MathOptions{Constants: map[string]int{}})
	math[1] = 2 // major version 2
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{Glyphs: mathTestGlyphs,
		UnitsPerEm: 1024, Extra: map[string][]byte{"MATH": math}}))
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mn>1</mn></math>`, Fonts: set})
	rec := NewRecorder(nil)
	Layout(b.Root, A4.Content(), set, rec)
	found := false
	for _, f := range rec.Findings() {
		found = found || f.Rule == RuleFontUndecodable && strings.Contains(f.Message, "version 2")
	}
	if !found {
		t.Errorf("findings %v do not report the unreadable table", rec.Findings())
	}
}

// TestARowReachesNoFurtherThanItsChildren: a row's line-ascent and
// line-descent are the greatest of its children's, which may be below
// nought: a row holding only a "-" reaches from 352 above its baseline to 288
// above it, and is 64 tall.
func TestARowReachesNoFurtherThanItsChildren(t *testing.T) {
	root, _ := mathLayout(t, `<math><mrow id="r"><mo>-</mo></mrow></math>`)
	if r := find(t, root, "r"); r.BorderRect.H != 64 || r.mathBaseline != 352 {
		t.Errorf("the row is %d tall with its baseline %d down, want 64 and 352", r.BorderRect.H, r.mathBaseline)
	}
}

// TestABorderIsInk: §3.1.2, a box's border moves its ink edge out to the
// border box — which a stretchy operator beside it is stretched to. The "1"
// is 704 tall, a parenthesis 768, so beside a bare "1" the parenthesis is
// tall enough; beside one with a border above it, it is not, and a face with
// no larger parenthesis says it draws it at its text size.
func TestABorderIsInk(t *testing.T) {
	stretchReported := func(doc string) bool {
		_, findings := mathLayout(t, doc)
		for _, f := range findings {
			if f.Rule == RuleMathFallback && strings.Contains(f.Message, "no larger forms") {
				return true
			}
		}
		return false
	}
	if stretchReported(`<math><mo>(</mo><mn>1</mn></math>`) {
		t.Error("a parenthesis beside a 1 was reported as needing to stretch")
	}
	if !stretchReported(`<math><mo>(</mo><mn style="border-top: 2px solid">1</mn></math>`) {
		t.Error("a parenthesis beside a bordered 1 was not reported as needing to stretch")
	}
}

// TestATokenLaidOutAsABlockIsCentred: a token whose parent is a CSS block is a
// block-level box of its own, and its line is centred in it and measured by
// its ink like any token's.
func TestATokenLaidOutAsABlockIsCentred(t *testing.T) {
	root, _ := mathLayout(t, `<math><mrow style="display: block; width: 200px"><mn id="n">1</mn></mrow></math>`)
	n := find(t, root, "n")
	if n.BorderRect.H != 704 || n.Lines[0].Rect.X != (12800-512)/2 {
		t.Errorf("the token is %d tall with its line at %d, want 704 and %d", n.BorderRect.H, n.Lines[0].Rect.X, (12800-512)/2)
	}
}

// TestOnlyOneGlyphHasAnItalicCorrection: §3.2.1.1 gives a token its glyph's
// italic correction only where its text is one glyph. Two italic x's lean as
// much as one does, and the token is still not slanted.
func TestOnlyOneGlyphHasAnItalicCorrection(t *testing.T) {
	root, _ := mathLayout(t, `<math id="m"><mi>𝑥𝑥</mi><mn id="n">1</mn></math>`)
	if x, _ := at(find(t, root, "n"), find(t, root, "m")); x != 1080 {
		t.Errorf("the 1 is at %d, want 1080, with no italic correction before it", x)
	}
}

// TestAFloatInAFormulaIsNoFloat: the box of a child of a formula that says
// "float" is no float, for anything that asks — painting included, which paints
// floats in a phase of their own.
func TestAFloatInAFormulaIsNoFloat(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mn id="f" style="float: right">2</mn></math>`, Fonts: set})
	var found *Box
	var walk func(*Box)
	walk = func(x *Box) {
		if x.Element != nil {
			if id, _ := x.Element.Attr("id"); id == "f" {
				found = x
			}
		}
		for _, c := range x.Children {
			walk(c)
		}
	}
	walk(b.Root)
	if found == nil || found.Float != FloatNone {
		t.Errorf("the child of a formula floats: %+v", found)
	}
}

// TestATokenHoldingABlockIsABlock: a token whose content is text and a block
// is a block container like any other, with anonymous blocks round its text.
func TestATokenHoldingABlockIsABlock(t *testing.T) {
	root, _ := mathLayout(t, `<math><mtext id="t">a<div>b</div>a</mtext></math>`)
	lines := 0
	var count func(*Fragment)
	count = func(f *Fragment) {
		lines += len(f.Lines)
		for _, c := range f.Children {
			count(c)
		}
	}
	count(find(t, root, "t"))
	if lines != 3 {
		t.Errorf("the token laid out %d lines, want 3", lines)
	}
}

// TestFirstLineDoesNotReachAFormula: §2.2.2, ::first-line does not apply to a
// MathML box.
func TestFirstLineDoesNotReachAFormula(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	out := Compose(Input{HTML: `<math><mi>1</mi></math>`,
		CSS: []Stylesheet{{Source: `mi::first-line { color: rgb(0, 128, 0) }`}}, Fonts: set}, Options{})
	for _, op := range out.Ops {
		if d, ok := op.(DrawText); ok && d.Color.G == 128 {
			t.Errorf("the token's text took its ::first-line colour")
		}
	}
}

// TestAFormulasBaselineIsItsOwn: a formula whose first or last child is not on
// its baseline — raised by an <mpadded> — is still aligned by its alphabetic
// baseline: on a line, where an inline-block takes its last line's, and as a
// block in a table row, which takes a cell's first.
func TestAFormulasBaselineIsItsOwn(t *testing.T) {
	lineBase := func(f *Fragment) style.Unit {
		return f.ContentRect().Y.Add(f.Lines[0].Rect.Y).Add(f.Lines[0].Baseline)
	}
	root, _ := mathLayout(t, `<p id="p">a<math><mn id="n">1</mn><mpadded voffset="5px"><mn>2</mn></mpadded></math>b</p>`)
	if got, want := lineBase(find(t, root, "n")), lineBase(find(t, root, "p")); got != want {
		t.Errorf("on a line, the formula's 1 sits at %d and the text at %d", got, want)
	}
	root, _ = mathLayout(t, `<table><tr><td id="x" style="vertical-align: baseline">x</td>`+
		`<td style="vertical-align: baseline"><math display="block"><mpadded voffset="5px"><mn>2</mn></mpadded>`+
		`<mn id="n">1</mn></math></td></tr></table>`)
	if got, want := lineBase(find(t, root, "n")), lineBase(find(t, root, "x")); got != want {
		t.Errorf("in a row, the formula's 1 sits at %d and the text at %d", got, want)
	}
}

// TestFirstLetterDoesNotReachAFormula: §2.2.2, nor does ::first-letter — a
// token is a block container, for its line, and is not one ::first-letter
// looks inside.
func TestFirstLetterDoesNotReachAFormula(t *testing.T) {
	got := textsOf(letterBoxes(t, `<math><mi class="c">ab</mi></math>`, `.c::first-letter { font-size: 40px }`))
	if strings.Join(got, "|") != "ab" {
		t.Errorf("the token's text is %q; ::first-letter does not apply to it", got)
	}
}

// TestAMathMLBoxKeepsOnlyItsElements: the text written straight inside a row
// is not one of its children, and nor is anything inside an <mspace>; and a
// MathML box says it is one in the box tree.
func TestAMathMLBoxKeepsOnlyItsElements(t *testing.T) {
	got := bodyBoxes(t, `<math><mrow> x <mn>1</mn> </mrow><mspace><mn>2</mn></mspace></math>`)
	want := "math inline/math\n" +
		"  mrow block/math\n" +
		"    mn block/math\n" +
		"      text \"1\"\n" +
		"  mspace block/math\n"
	if got != want {
		t.Errorf("the box tree is\n%s\nwant\n%s", got, want)
	}
}

// TestATokensInkIsItsGlyphsInk: a glyph with no ink — a space — reaches
// nowhere, so "- -" is the 64 units of its dashes' ink; and a glyph a font
// moves up is measured where it is drawn: an "a" raised 100 units reaches 548
// above the baseline and 100 above it at its bottom.
func TestATokensInkIsItsGlyphsInk(t *testing.T) {
	root, _ := mathLayout(t, `<math><mtext id="t">- -</mtext></math>`)
	if f := find(t, root, "t"); f.BorderRect.H != 64 || f.mathBaseline != 352 {
		t.Errorf("\"- -\" is %d tall with its baseline %d down, want 64 and 352", f.BorderRect.H, f.mathBaseline)
	}
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Raised", UnitsPerEm: 1024, Ascent: 1024, Descent: -256, Glyphs: mathTestGlyphs,
		Extra: map[string][]byte{"GPOS": fonttest.GPOSSingle(9, 0, 100, 0)}, // the "a"
	}))
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mtext id="t">a</mtext></math>`,
		CSS: []Stylesheet{{Source: `body { margin: 0; font-size: 16px }`}}, Fonts: set})
	frag := Layout(b.Root, A4.Content(), set, NewRecorder(nil))
	if f := find(t, frag, "t"); f.BorderRect.H != 448 || f.mathBaseline != 548 {
		t.Errorf("the raised a is %d tall with its baseline %d down, want 448 and 548", f.BorderRect.H, f.mathBaseline)
	}
}

// TestAGlyphFromAnotherFaceHasNoneOfTheMathFontsCorrections: the italic
// correction a MATH table states is for its own face's glyph. A "Q" the math
// face does not have is drawn from the next family, where it is glyph 3 —
// which in the math face is the italic x, with an italic correction of 60 —
// and the "1" after it follows it with no correction at all.
func TestAGlyphFromAnotherFaceHasNoneOfTheMathFontsCorrections(t *testing.T) {
	other, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Other", UnitsPerEm: 1024, Ascent: 1024, Descent: -256,
		Glyphs: []fonttest.Glyph{
			{Rune: ' ', Advance: 256},
			{Rune: 'z', Advance: 512, HasShape: true, Ink: [4]int{0, 0, 512, 448}},
			{Rune: 'Q', Advance: 700, HasShape: true, Ink: [4]int{0, 0, 700, 704}},
		},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if gs, _ := ShapedGlyphs(DrawText{Text: "Q", Face: other, Size: 1024}); len(gs) != 1 || gs[0].GID != mathGlyphItalicX {
		t.Fatalf("the Q is %v in the other face, want glyph %d", gs, mathGlyphItalicX)
	}
	set := mathFallbackSet{namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}, other}
	b := Build(Input{HTML: `<math id="m"><mtext>Q</mtext><mn id="n">1</mn></math>`,
		CSS: []Stylesheet{{Source: `body { margin: 0; font-size: 16px }`}}, Fonts: set})
	frag := Layout(b.Root, A4.Content(), set, NewRecorder(nil))
	if x, _ := at(find(t, frag, "n"), find(t, frag, "m")); x != 700 {
		t.Errorf("the 1 is at %d, want 700: the Q has no italic correction", x)
	}
}

// mathFallbackSet is the math face, and another face to fall back to for the
// text it has no glyphs for.
type mathFallbackSet struct {
	namedFaceSet
	fallback *shape.Face
}

func (s mathFallbackSet) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	if _, missing := s.fallback.ShapeGlyphs(text); missing == 0 {
		return s.fallback, true
	}
	return nil, false
}

// TestAFormulaInAStandardFontIsMeasuredByItsCharacters: the fourteen standard
// faces state no glyph's ink, and a token set in one is measured by the boxes
// of its characters instead — not by nothing.
func TestAFormulaInAStandardFontIsMeasuredByItsCharacters(t *testing.T) {
	root, _ := mathLayout(t, `<math style="font-family: serif"><mn id="n">1</mn></math>`)
	n := find(t, root, "n")
	face, _ := StandardFonts().Face("serif", false, false)
	above, below, ok := face.InkExtent("1", 16)
	if !ok {
		t.Fatal("the standard serif states no box for 1")
	}
	wantA, _ := style.FromPx(above)
	wantD, _ := style.FromPx(below)
	if n.mathBaseline != wantA || n.BorderRect.H != wantA.Add(wantD) || wantA == 0 {
		t.Errorf("the 1 is %d tall with its baseline %d down, want %d and %d",
			n.BorderRect.H, n.mathBaseline, wantA.Add(wantD), wantA)
	}
}
