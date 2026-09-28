package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Stretched and enlarged operators and radicals, in exact arithmetic: the face
// of mathlayout_test.go with larger forms of the parenthesis and its mirror,
// the radical sign, the sum and the arrow, at 16px, where a font unit is a
// style.Unit.
//
// The parenthesis's text is 1024 tall, from 256 below the baseline; its size
// variant is 1600 and 450 wide; its assembly is a bottom and a top of 600 and
// an extender of 400, joined by connectors of 100, with a least overlap of 50,
// and 500 wide. The radical sign's text is 900 tall, from 100 below; its
// variant 1500 and 650 wide; its assembly as the parenthesis's, 700 wide.

// mathRadicalConstants are the radical's, each distinct.
var mathRadicalConstants = map[string]int{
	"RadicalVerticalGap": 50, "RadicalDisplayStyleVerticalGap": 90, "RadicalRuleThickness": 40,
	"RadicalExtraAscender": 30, "RadicalKernBeforeDegree": 100, "RadicalKernAfterDegree": -200,
	"RadicalDegreeBottomRaisePercent": 60, "DisplayOperatorMinHeight": 1500,
}

func mathStretchFace(t testing.TB, constants map[string]int) *shape.Face {
	t.Helper()
	return mathStretchFaceWith(t, constants, nil, nil)
}

// mathStretchFaceWith is mathStretchFace with its MATH table and its font
// changed by a test before they are written.
func mathStretchFaceWith(t testing.TB, constants map[string]int,
	math func(*fonttest.MathOptions), font func(*fonttest.SFNTOptions)) *shape.Face {
	t.Helper()
	c := map[string]int{"ScriptPercentScaleDown": 70, "ScriptScriptPercentScaleDown": 50, "AxisHeight": 256}
	for k, v := range mathRadicalConstants {
		c[k] = v
	}
	for k, v := range constants {
		c[k] = v
	}
	assembly := func(bottom, ext, top int) fonttest.MathAssembly {
		return fonttest.MathAssembly{Parts: []fonttest.MathPart{
			{Glyph: bottom, Start: 0, End: 100, Full: 600},
			{Glyph: ext, Start: 100, End: 100, Full: 400, Extender: true},
			{Glyph: top, Start: 100, End: 0, Full: 600},
		}}
	}
	mo := fonttest.MathOptions{
		Constants:           c,
		ItalicsCorrection:   map[int]int{mathGlyphItalicX: 60, mathGlyphItalicF: 100, mathGlyphIntegral: 100, 22: 30},
		TopAccentAttachment: map[int]int{mathGlyphItalicX: 300},
		MinConnectorOverlap: 50,
		VertVariants: map[int][]fonttest.MathVariant{
			mathGlyphParen:   {{Glyph: mathGlyphParen, Advance: 1024}, {Glyph: 22, Advance: 1600}},
			mathGlyphCloser:  {{Glyph: mathGlyphCloser, Advance: 1024}, {Glyph: 28, Advance: 1600}},
			mathGlyphRadical: {{Glyph: mathGlyphRadical, Advance: 900}, {Glyph: 18, Advance: 1500}},
			mathGlyphSum:     {{Glyph: mathGlyphSum, Advance: 1000}, {Glyph: 26, Advance: 1600}, {Glyph: 29, Advance: 2000}},
		},
		VertAssembly: map[int]fonttest.MathAssembly{
			mathGlyphParen:   assembly(23, 24, 25),
			mathGlyphRadical: assembly(19, 20, 21),
		},
		HorizVariants: map[int][]fonttest.MathVariant{
			mathGlyphArrow: {{Glyph: mathGlyphArrow, Advance: 600}, {Glyph: 27, Advance: 1200}},
		},
	}
	if math != nil {
		math(&mo)
	}
	so := fonttest.SFNTOptions{Name: "MathTest", UnitsPerEm: 1024, Ascent: 1024, Descent: -256, Glyphs: mathTestGlyphs,
		Extra: map[string][]byte{}}
	if font != nil {
		font(&so)
	}
	so.Extra["MATH"] = fonttest.MATH(mo)
	face, err := shape.Load(fonttest.SFNT(so))
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// mathComposed lays a document out in the stretchy face and paints it: the
// fragments and the display list, and what was reported.
func mathComposed(t *testing.T, face *shape.Face, doc string) (*Fragment, []Op, []Finding) {
	t.Helper()
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	out := Compose(Input{HTML: doc, Fonts: set, CSS: []Stylesheet{{Source: `body { margin: 0; font-size: 16px }
		mi, mn, mo, mtext { font-size: 16px }`}}}, Options{})
	return out.Root, out.Ops, out.Findings
}

// glyphsIn is every DrawGlyphs in a display list.
func glyphsIn(ops []Op) []DrawGlyphs {
	var out []DrawGlyphs
	for _, op := range ops {
		if g, ok := op.(DrawGlyphs); ok {
			out = append(out, g)
		}
	}
	return out
}

// thousandths is a length in font units, 1024 to the em, as shaping states
// an offset: in thousandths of an em.
func thousandths(units int) float64 { return float64(units) * 1000 / 1024 }

// checkGlyphs asserts a DrawGlyphs's glyphs and their offsets up, and where
// its origin is from a fragment's content box.
func checkGlyphs(t *testing.T, what string, g DrawGlyphs, in *Fragment, x, y style.Unit, gids, ys []int, advance int) {
	t.Helper()
	c := in.ContentRect()
	if g.At.X.Sub(c.X) != x || g.At.Y.Sub(c.Y) != y {
		t.Errorf("%s: drawn from (%d, %d) in its box, want (%d, %d)", what, g.At.X.Sub(c.X), g.At.Y.Sub(c.Y), x, y)
	}
	if len(g.Glyphs) != len(gids) {
		t.Fatalf("%s: %d glyphs, want %d: %+v", what, len(g.Glyphs), len(gids), g.Glyphs)
	}
	for i, gl := range g.Glyphs {
		want := shape.Glyph{GID: gids[i], YOffset: thousandths(ys[i])}
		if advance > 0 {
			want.XAdvance = thousandths(advance)
		}
		if gl.GID != want.GID || gl.XOffset != 0 || gl.YOffset != want.YOffset || gl.XAdvance != want.XAdvance {
			t.Errorf("%s: glyph %d is %+v, want %+v", what, i, gl, want)
		}
	}
}

// TestAParenthesisIsBuiltToCoverWhatIsBesideIt: beside a space 1536 up and
// 512 down, a symmetric parenthesis is asked to reach 1280 either side of the
// axis at 256 — 2560 in all. Its variant is 1600, so it is its assembly: at
// the least overlap each extender adds 350, and five of them are needed; with
// seven pieces the even share of the overlap would be 106⅔, more than a
// connector's 100, so they overlap 100 and the assembly is 2600 tall, 500
// wide. Δ centres it on the target: (2600 − 0 − (1536 − 1024)) / 2 = 1044.
func TestAParenthesisIsBuiltToCoverWhatIsBesideIt(t *testing.T) {
	root, ops, findings := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo id="p" mathcolor="#0000ff">(</mo><mspace height="1.5em" depth="0.5em"></mspace></math>`)
	for _, f := range findings {
		t.Errorf("finding: %v", f)
	}
	p := find(t, root, "p")
	if p.BorderRect.W != 500 || p.BorderRect.H != 2600 || p.mathBaseline != 1556 {
		t.Errorf("the parenthesis is %d by %d with its baseline %d down, want 500 by 2600 and 1556",
			p.BorderRect.W, p.BorderRect.H, p.mathBaseline)
	}
	gs := glyphsIn(ops)
	if len(gs) != 1 {
		t.Fatalf("%d DrawGlyphs, want the one: %v", len(gs), ops)
	}
	checkGlyphs(t, "the assembly", gs[0], p, 0, 2600,
		[]int{23, 24, 24, 24, 24, 24, 25}, []int{0, 500, 800, 1100, 1400, 1700, 2000}, 0)
	if g := gs[0]; g.Text != "(" || g.Color.B != 255 || g.Size != 1024 || g.Face == nil {
		t.Errorf("the assembly stands for %q in %v at %d, want \"(\" in blue at 1024", g.Text, g.Color, g.Size)
	}
	for _, op := range ops {
		if d, ok := op.(DrawText); ok && d.Text == "(" {
			t.Errorf("the parenthesis's text is drawn as well: %+v", d)
		}
	}
}

// TestAParenthesisTakesTheFirstFormThatReaches: asked to reach 1024 up and
// 512 down, 1536, the parenthesis is its variant, 1600 tall — 1100 above its
// baseline and 500 below — and 450 wide, moved down by (600 − 512) / 2 = 44.
func TestAParenthesisTakesTheFirstFormThatReaches(t *testing.T) {
	root, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo id="p">(</mo><mspace height="0.75em" depth="0.5em"></mspace></math>`)
	p := find(t, root, "p")
	if p.BorderRect.W != 450 || p.BorderRect.H != 1600 || p.mathBaseline != 1056 {
		t.Errorf("the parenthesis is %d by %d with its baseline %d down, want 450 by 1600 and 1056",
			p.BorderRect.W, p.BorderRect.H, p.mathBaseline)
	}
	gs := glyphsIn(ops)
	if len(gs) != 1 {
		t.Fatalf("%d DrawGlyphs, want the one", len(gs))
	}
	checkGlyphs(t, "the variant", gs[0], p, 0, 1100, []int{22}, []int{0}, 450)
}

// TestAParenthesisThatIsTallEnoughIsItsText: beside a "1" the parenthesis
// is asked for less than its own 1024, and is its text. Symmetric, it is where
// it always is; not symmetric, it is asked to cover the 1's 704 above the
// baseline and nothing below, which minsize — its own height — scales about
// the axis to 907 and 117, and its glyph is moved up by (790 − 512) / 2 = 139
// to be centred on that.
func TestAParenthesisThatIsTallEnoughIsItsText(t *testing.T) {
	face := mathStretchFace(t, nil)
	root, ops, _ := mathComposed(t, face, `<math><mo id="p">(</mo><mn>1</mn>`+
		`<mo id="q" symmetric="false">(</mo><mn>1</mn></math>`)
	if gs := glyphsIn(ops); len(gs) != 0 {
		t.Errorf("a parenthesis as tall as it is asked is drawn as glyphs: %+v", gs)
	}
	p, q := find(t, root, "p"), find(t, root, "q")
	if p.BorderRect.H != 1024 || p.mathBaseline != 768 {
		t.Errorf("the symmetric parenthesis is %d tall, baseline %d down, want 1024 and 768", p.BorderRect.H, p.mathBaseline)
	}
	if q.BorderRect.H != 1024 || q.mathBaseline != 907 {
		t.Errorf("the other parenthesis is %d tall, baseline %d down, want 1024 and 907", q.BorderRect.H, q.mathBaseline)
	}
	// Its line is where the glyph is: 768 below its top, 139 above the
	// formula's baseline.
	if line := q.Lines[0]; line.Rect.Y.Add(line.Baseline) != 768 {
		t.Errorf("the moved parenthesis's baseline is %d below its top, want 768", line.Rect.Y.Add(line.Baseline))
	}
	texts := 0
	for _, op := range ops {
		if d, ok := op.(DrawText); ok && d.Text == "(" {
			texts++
		}
	}
	if texts != 2 {
		t.Errorf("%d parentheses drawn as text, want both", texts)
	}
}

// TestMinsizeAndMaxsizeBoundTheTarget: §3.2.4.3's minsize and maxsize. A
// parenthesis alone is asked for nothing, and minsize="2em" makes that 2048,
// centred on the axis: 1280 up and 768 down. Its assembly reaches exactly
// 2048 with three extenders overlapping by 88. maxsize="100%" — of its own
// 1024 — holds the 2560 of TestAParenthesisIsBuiltToCoverWhatIsBesideIt to
// its text.
func TestMinsizeAndMaxsizeBoundTheTarget(t *testing.T) {
	face := mathStretchFace(t, nil)
	root, ops, _ := mathComposed(t, face, `<math><mo id="p" minsize="2em">(</mo></math>`)
	p := find(t, root, "p")
	if p.BorderRect.H != 2048 || p.mathBaseline != 1280 {
		t.Errorf("the parenthesis is %d tall with its baseline %d down, want 2048 and 1280", p.BorderRect.H, p.mathBaseline)
	}
	if gs := glyphsIn(ops); len(gs) != 1 {
		t.Errorf("%d DrawGlyphs, want the assembly", len(gs))
	} else {
		checkGlyphs(t, "the assembly", gs[0], p, 0, 2048, []int{23, 24, 24, 24, 25}, []int{0, 512, 824, 1136, 1448}, 0)
	}
	root, ops, _ = mathComposed(t, face,
		`<math><mo id="p" maxsize="100%">(</mo><mspace height="1.5em" depth="0.5em"></mspace></math>`)
	if p := find(t, root, "p"); p.BorderRect.H != 1024 || p.mathBaseline != 768 || len(glyphsIn(ops)) != 0 {
		t.Errorf("the bounded parenthesis is %d tall with its baseline %d down and %d DrawGlyphs, want its text",
			p.BorderRect.H, p.mathBaseline, len(glyphsIn(ops)))
	}
	// A percentage is of the glyph's own height: minsize="200%" is 2048, as
	// 2em is; maxsize="150%" holds the 2560 to 1536, which is the variant.
	// And a parenthesis that is not symmetric, asked for nothing, is minsize
	// centred on the axis all the same.
	for _, doc := range []string{`<math><mo id="p" minsize="200%">(</mo></math>`,
		`<math><mo id="p" minsize="2em" symmetric="false">(</mo></math>`} {
		root, _, _ = mathComposed(t, face, doc)
		if p := find(t, root, "p"); p.BorderRect.H != 2048 || p.mathBaseline != 1280 {
			t.Errorf("%s: the parenthesis is %d tall with its baseline %d down, want 2048 and 1280", doc, p.BorderRect.H, p.mathBaseline)
		}
	}
	root, ops, _ = mathComposed(t, face,
		`<math><mo id="p" maxsize="150%">(</mo><mspace height="1.5em" depth="0.5em"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 1 || gs[0].Glyphs[0].GID != 22 {
		t.Errorf("a parenthesis bounded to 1536 is drawn as %+v, want its variant", gs)
	}
}

// TestAnOperatorHoldingMoreThanTextIsItsContent: an <mo> with an element in
// it is not one character, and is laid out as the token it is.
func TestAnOperatorHoldingMoreThanTextIsItsContent(t *testing.T) {
	_, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo>(<mspace width="1px"></mspace></mo><mspace height="1.5em" depth="0.5em"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 0 {
		t.Errorf("an operator holding an element is drawn as a construction: %+v", gs)
	}
}

// TestAStretchedOperatorLeansAsItsConstructionDoes: the variant of the
// parenthesis leans 30, and a superscript on it is 30 past its advance.
func TestAStretchedOperatorLeansAsItsConstructionDoes(t *testing.T) {
	root, _, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><msup id="s"><mo>(</mo><mn id="n">2</mn></msup><mspace height="0.75em" depth="0.5em"></mspace></math>`)
	if g := geomIn(t, root, "n", "s"); g.x != 450+30 {
		t.Errorf("the superscript is at %d, want 480", g.x)
	}
}

// TestAnArrowIsStretchedAcrossItsBase: §3.4.2.2 asks the arrow over a base
// 1100 wide to reach 1100; its text is 600, its variant 1200.
func TestAnArrowIsStretchedAcrossItsBase(t *testing.T) {
	root, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mover id="m"><mspace width="17.1875px"></mspace><mo id="a">→</mo></mover></math>`)
	a := find(t, root, "a")
	if a.BorderRect.W != 1200 || a.BorderRect.H != 200 || a.mathBaseline != 400 {
		t.Errorf("the arrow is %d by %d with its baseline %d down, want 1200 by 200 and 400",
			a.BorderRect.W, a.BorderRect.H, a.mathBaseline)
	}
	if gs := glyphsIn(ops); len(gs) != 1 {
		t.Errorf("%d DrawGlyphs, want the arrow's variant", len(gs))
	} else {
		checkGlyphs(t, "the arrow", gs[0], a, 0, 400, []int{27}, []int{0}, 1200)
	}
}

// TestALargeOperatorIsLargeInDisplay: §3.2.4.3's largeop, in display
// mathematics the first of its variants at least DisplayOperatorMinHeight
// tall — the sum's are its own glyph, 1000, then 1600 and 2000 — and where
// none is, the largest that is not its own glyph; in inline mathematics, or
// where its own glyph is tall enough, its text.
func TestALargeOperatorIsLargeInDisplay(t *testing.T) {
	for _, tc := range []struct {
		min, gid              int
		width, height, ascent int
	}{
		{1500, 26, 1200, 1600, 1200},
		{1600, 26, 1200, 1600, 1200},
		{1601, 29, 1500, 2000, 1500},
		{2500, 29, 1500, 2000, 1500},
	} {
		root, ops, _ := mathComposed(t, mathStretchFace(t, map[string]int{"DisplayOperatorMinHeight": tc.min}),
			`<math display="block"><mo id="s">∑</mo></math>`)
		s := find(t, root, "s")
		if s.BorderRect.W != style.Unit(tc.width) || s.BorderRect.H != style.Unit(tc.height) || s.mathBaseline != style.Unit(tc.ascent) {
			t.Errorf("DisplayOperatorMinHeight %d: the sum is %d by %d with its baseline %d down, want %d by %d and %d",
				tc.min, s.BorderRect.W, s.BorderRect.H, s.mathBaseline, tc.width, tc.height, tc.ascent)
		}
		if gs := glyphsIn(ops); len(gs) != 1 {
			t.Errorf("DisplayOperatorMinHeight %d: %d DrawGlyphs, want the display sum", tc.min, len(gs))
		} else {
			checkGlyphs(t, "the sum", gs[0], s, 0, style.Unit(tc.ascent), []int{tc.gid}, []int{0}, tc.width)
		}
	}
	root, ops, findings := mathComposed(t, mathStretchFace(t, map[string]int{"DisplayOperatorMinHeight": 900}),
		`<math display="block"><mo id="s">∑</mo></math>`)
	if s := find(t, root, "s"); s.BorderRect.W != 800 || len(glyphsIn(ops)) != 0 || len(findings) != 0 {
		t.Errorf("a sum already tall enough is %d wide with %d DrawGlyphs and %v, want its text, 800, and nothing said",
			s.BorderRect.W, len(glyphsIn(ops)), findings)
	}
	root, ops, _ = mathComposed(t, mathStretchFace(t, nil), `<math><mo id="s">∑</mo></math>`)
	if s := find(t, root, "s"); s.BorderRect.W != 800 || len(glyphsIn(ops)) != 0 {
		t.Errorf("an inline sum is %d wide with %d DrawGlyphs, want its text, 800", s.BorderRect.W, len(glyphsIn(ops)))
	}
	// A large operator laid out as a block of its own is large too.
	root, ops, _ = mathComposed(t, mathStretchFace(t, nil),
		`<math display="block"><mrow style="display: block; width: 100px"><mo id="s">∑</mo></mrow></math>`)
	if s := find(t, root, "s"); s.BorderRect.H != 1600 || s.mathBaseline != 1200 || len(glyphsIn(ops)) != 1 {
		t.Errorf("a block sum is %d tall with its baseline %d down and %d DrawGlyphs, want 1600, 1200 and one",
			s.BorderRect.H, s.mathBaseline, len(glyphsIn(ops)))
	} else {
		checkGlyphs(t, "the block sum", glyphsIn(ops)[0], s, (6400-1200)/2, 1200, []int{26}, []int{0}, 1200)
	}
}

// TestASquareRootCoversItsBase is §3.3.3.2 on a "1", 704 tall. In inline
// mathematics the sign is asked for the rule, 40, the gap, 50, and the 1:
// 794, which its own 900 covers, so it is its text. The root reaches the 1's
// top and the gap, rule and extra ascender over it, 824; and the sign's
// bottom, 30 of extra ascender below the bar's top plus the sign's 900, less
// that: 106. The bar is across the 1, its top 30 below the root's; the sign's
// ink top is at the bar's top.
func TestASquareRootCoversItsBase(t *testing.T) {
	root, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><msqrt id="q" mathcolor="#0000ff"><mn id="n">1</mn></msqrt></math>`)
	checkMath(t, root, "q", 1112, 824, 106, map[string][2]style.Unit{"n": {600, 120}})
	q := find(t, root, "q")
	if got := q.mathMarks; len(got) != 1 || got[0] != (Rect{X: 600, Y: 30, W: 512, H: 40}) {
		t.Errorf("the overbar is %v, want 512 by 40 at (600, 30)", got)
	}
	var sign *DrawText
	for _, op := range ops {
		if d, ok := op.(DrawText); ok && d.Text == "√" {
			sign = &d
		}
	}
	c := q.ContentRect()
	if sign == nil || sign.At.X != c.X || sign.At.Y.Sub(c.Y) != 830 || sign.Color.B != 255 {
		t.Errorf("the sign is drawn as %+v, want \"√\" in blue at (0, 830) in the root", sign)
	}
	// In display mathematics the gap is 90.
	root, _, _ = mathComposed(t, mathStretchFace(t, nil), `<math display="block"><msqrt id="q"><mn>1</mn></msqrt></math>`)
	checkMath(t, root, "q", 1112, 864, 66, nil)
}

// TestASquareRootOfSomethingTallIsAssembled: over a space 1536 tall the sign
// is asked for 1626; its variant is 1500, so it is its assembly, two
// extenders overlapping by 100: 1700 tall and 700 wide, from the bar's top
// down to 74 below the baseline.
func TestASquareRootOfSomethingTallIsAssembled(t *testing.T) {
	root, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><msqrt id="q"><mspace width="1em" height="1.5em"></mspace></msqrt></math>`)
	checkMath(t, root, "q", 1724, 1656, 74, nil)
	q := find(t, root, "q")
	if got := q.mathMarks; len(got) != 1 || got[0] != (Rect{X: 700, Y: 30, W: 1024, H: 40}) {
		t.Errorf("the overbar is %v, want 1024 by 40 at (700, 30)", got)
	}
	if gs := glyphsIn(ops); len(gs) != 1 {
		t.Errorf("%d DrawGlyphs, want the sign", len(gs))
	} else {
		checkGlyphs(t, "the sign", gs[0], q, 0, 1656+74, []int{19, 20, 20, 21}, []int{0, 500, 800, 1100}, 0)
		if gs[0].Text != "√" {
			t.Errorf("the sign stands for %q", gs[0].Text)
		}
	}
}

// TestARootHasItsIndexOverItsSign is §3.3.3.3: the square root of the 1,
// 1112 by 930, with the 2 raised 60% of 930, 558, above its bottom — 452
// above the baseline — and kerned 100 in and 200 back over the sign.
func TestARootHasItsIndexOverItsSign(t *testing.T) {
	root, _, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mroot id="r"><mn id="b">1</mn><mn id="i">2</mn></mroot></math>`)
	checkMath(t, root, "r", 1524, 1156, 106, map[string][2]style.Unit{
		"i": {100, 0}, "b": {1012, 1156 - 704},
	})
	if got := find(t, root, "r").mathMarks; len(got) != 1 || got[0] != (Rect{X: 1012, Y: 1156 - 794, W: 512, H: 40}) {
		t.Errorf("the overbar is %v, want 512 by 40 at (1012, %d)", got, 1156-794)
	}
}

// TestARightToLeftFormulaMirrorsItsConstructions: the parenthesis is drawn as
// its mirror image, ")" and its variant, and a square root has its sign on the
// right — drawn as it faces, which is reported.
func TestARightToLeftFormulaMirrorsItsConstructions(t *testing.T) {
	face := mathStretchFace(t, nil)
	root, ops, _ := mathComposed(t, face,
		`<math dir="rtl"><mo id="p">(</mo><mspace height="0.75em" depth="0.5em"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 1 || len(gs[0].Glyphs) != 1 || gs[0].Glyphs[0].GID != 28 {
		t.Errorf("an rtl parenthesis is drawn as %+v, want the closing one's variant", gs)
	}
	if p := find(t, root, "p"); p.BorderRect.W != 460 {
		t.Errorf("the rtl parenthesis is %d wide, want its mirror's 460", p.BorderRect.W)
	}
	root, _, findings := mathComposed(t, face, `<math dir="rtl"><msqrt id="q"><mn id="n">1</mn></msqrt></math>`)
	q := find(t, root, "q")
	if got := q.mathMarks; len(got) != 1 || got[0].X != 0 {
		t.Errorf("the rtl overbar is %v, want it at the left", got)
	}
	if got := q.mathGlyphs; len(got) != 1 || got[0].at.X != 512 {
		t.Errorf("the rtl sign is %+v, want it from 512", got)
	}
	if !mathFinding(findings, RuleUnsupportedValue, "right-to-left") {
		t.Errorf("an rtl radical is not reported: %v", findings)
	}
}

// TestAStretchedOperatorsSizeIsItsWidestForm: §3.2.4.3 gives a stretchy
// operator of block axis the width of the widest form of it its font has — the
// parenthesis's assembly pieces, 500 — whatever it is stretched to; a large
// operator the width of its display form; a radical the widest form of its
// sign, 700, and its base.
func TestAStretchedOperatorsSizeIsItsWidestForm(t *testing.T) {
	face := mathStretchFace(t, nil)
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	for _, tc := range []struct {
		inner string
		want  style.Unit
	}{
		{`<mo id="e">(</mo>`, 500},
		{`<mo id="e">→</mo>`, 600},
		{`<mo id="e">∑</mo>`, 1200},
		{`<msqrt id="e"><mn>1</mn></msqrt>`, 700 + 512},
		{`<mroot id="e"><mn>1</mn><mn>2</mn></mroot>`, 100 + 512 - 200 + 700 + 512},
	} {
		b := Build(Input{HTML: `<math display="block">` + tc.inner + `</math>`, Fonts: set,
			CSS: []Stylesheet{{Source: `mn, mo { font-size: 16px }`}}})
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
		size := l.mathOuterSize(e)
		if size.min != tc.want || size.max != tc.want {
			t.Errorf("%s: sizes %d and %d, want %d", tc.inner, size.min, size.max, tc.want)
		}
	}
}

// TestAnAssemblyIsBoundedAndSaysSo: a parenthesis beside something 20000px
// tall would take over seven thousand extenders; it is built with the 4096
// glyphs the bound allows, and reported.
func TestAnAssemblyIsBoundedAndSaysSo(t *testing.T) {
	_, ops, findings := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo>(</mo><mspace height="20000px"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 1 || len(gs[0].Glyphs) != shape.MaxMathAssemblyGlyphs {
		n := -1
		if len(gs) == 1 {
			n = len(gs[0].Glyphs)
		}
		t.Errorf("the assembly is %d glyphs, want %d", n, shape.MaxMathAssemblyGlyphs)
	}
	if !mathFinding(findings, RuleLimit, "more than 4096 glyphs") {
		t.Errorf("the bound is not reported: %v", findings)
	}
}

// TestAnAssemblyIsChargedToTheWorkBudget: with a budget smaller than an
// assembly, the operator is its glyph unstretched, and the budget says it
// stopped something.
func TestAnAssemblyIsChargedToTheWorkBudget(t *testing.T) {
	defer func(a, b int) { maxLayoutWork, minLayoutWork = a, b }(maxLayoutWork, minLayoutWork)
	maxLayoutWork, minLayoutWork = 4, 2000
	_, ops, findings := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo>(</mo><mspace height="20000px"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 0 {
		t.Errorf("an assembly past the work budget is drawn: %d glyphs", len(gs[0].Glyphs))
	}
	// It is no assembly in the layout either: not one the painting then
	// declined to draw.
	root, _, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo id="p">(</mo><mspace height="20000px"></mspace></math>`)
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		for _, g := range f.mathGlyphs {
			t.Errorf("an assembly of %d glyphs was laid out past the work budget", len(g.glyphs))
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	limited := false
	for _, f := range findings {
		limited = limited || f.Rule == RuleLimit
	}
	if !limited {
		t.Errorf("the work budget's cut is not reported: %v", findings)
	}
}

// TestAHiddenOperatorDrawsNoGlyphs: §3.2.4.3, "only painted if the visibility
// of the <mo> element is visible".
func TestAHiddenOperatorDrawsNoGlyphs(t *testing.T) {
	_, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo style="visibility: hidden">(</mo><mspace height="1.5em" depth="0.5em"></mspace>`+
			`<mphantom><msqrt><mspace width="1em" height="1.5em"></mspace></msqrt></mphantom></math>`)
	if gs := glyphsIn(ops); len(gs) != 0 {
		t.Errorf("hidden constructions are drawn: %+v", gs)
	}
	for _, op := range ops {
		if f, ok := op.(FillRect); ok {
			t.Errorf("a hidden radical's bar is drawn: %+v", f)
		}
	}
}

// TestARadicalWithoutAGlyphIsReported: the standard serif has no radical
// sign, and a square root set in it says so; its base is still laid out.
func TestARadicalWithoutAGlyphIsReported(t *testing.T) {
	root, findings := mathLayout(t, `<math style="font-family: serif"><msqrt id="q"><mn id="n">1</mn></msqrt></math>`)
	if !mathFinding(findings, RuleGlyphMissing, "U+221A") {
		t.Errorf("the missing radical sign is not reported: %v", findings)
	}
	if n := find(t, root, "n"); n.BorderRect.W == 0 {
		t.Error("the base of a root with no sign is not laid out")
	}
}

// TestAnOperatorGivenStretchyIsStillOneCharacter: an <mo> made stretchy by its
// attribute and holding an element as well as its character is not one
// character, and is laid out as the token it is.
func TestAnOperatorGivenStretchyIsStillOneCharacter(t *testing.T) {
	_, ops, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mo stretchy="true">(<mspace width="1px"></mspace></mo><mspace height="1.5em" depth="0.5em"></mspace></math>`)
	if gs := glyphsIn(ops); len(gs) != 0 {
		t.Errorf("an operator holding an element is drawn as a construction: %+v", gs)
	}
}

// TestTheLargestFormIsNotTheGlyphItself: where no variant is tall enough the
// large operator is the largest that is not its own glyph, wherever the table
// lists its own glyph.
func TestTheLargestFormIsNotTheGlyphItself(t *testing.T) {
	face := mathStretchFaceWith(t, map[string]int{"DisplayOperatorMinHeight": 3000}, func(o *fonttest.MathOptions) {
		o.VertVariants[mathGlyphSum] = []fonttest.MathVariant{{Glyph: 26, Advance: 1600}, {Glyph: mathGlyphSum, Advance: 1000}}
	}, nil)
	_, ops, _ := mathComposed(t, face, `<math display="block"><mo>∑</mo></math>`)
	if gs := glyphsIn(ops); len(gs) != 1 || gs[0].Glyphs[0].GID != 26 {
		t.Errorf("the sum is drawn as %+v, want its display form", gs)
	}
}

// TestNoConstructionIsBuiltPastTheWorkBudget: ten parentheses beside
// something 20000px tall would be ten assemblies of 4096 glyphs. Past a
// budget one of them spends, none more is built: the work stops growing by
// an assembly per operator.
func TestNoConstructionIsBuiltPastTheWorkBudget(t *testing.T) {
	defer func(a, b int) { maxLayoutWork, minLayoutWork = a, b }(maxLayoutWork, minLayoutWork)
	maxLayoutWork, minLayoutWork = 4, 2000
	set := namedFaceSet{family: "math", face: mathStretchFace(t, nil), standard: StandardFonts()}
	for _, doc := range []string{
		`<math>` + strings.Repeat(`<mo>(</mo>`, 10) + `<mspace height="20000px"></mspace></math>`,
		`<math>` + strings.Repeat(`<msqrt><mspace height="20000px"></mspace></msqrt>`, 10) + `</math>`,
	} {
		b := Build(Input{HTML: doc, Fonts: set})
		l := newLayouter(b.Root, A4.Content(), set, nil)
		l.layout()
		if l.work > l.workLimit+shape.MaxMathAssemblyGlyphs+1000 {
			t.Errorf("%s: the layout worked %d against a budget of %d: assemblies were built past it", doc[:40], l.work, l.workLimit)
		}
	}
}

// TestAnAssemblyIsChargedAsItsGlyphsToThePainting: the painting charges each
// assembly as the marks it is; with a budget that holds two, the rest are cut
// and the cut is reported.
func TestAnAssemblyIsChargedAsItsGlyphsToThePainting(t *testing.T) {
	saved := workFloor
	workFloor = 1 << 20
	defer func() { workFloor = saved }()
	_, ops, findings := mathComposed(t, mathStretchFace(t, nil),
		`<math>`+strings.Repeat(`<mo>(</mo>`, 5)+`<mspace height="20000px"></mspace></math>`)
	if n := len(glyphsIn(ops)); n >= 5 {
		t.Errorf("all %d assemblies were painted within a budget for two", n)
	}
	if !mathFinding(findings, RuleLimit, "the marks past that point") {
		t.Errorf("the painting's cut is not reported: %v", findings)
	}
}

// TestARadicalSignIsTheSmallestFormThatReaches: over a base 1024 up, the sign
// is asked for 40 + 50 + 1024 = 1114, and is its variant, 1500, which no text
// shapes to. Over a base 800 up and 200 down it is asked for 1090: the depth
// counts.
func TestARadicalSignIsTheSmallestFormThatReaches(t *testing.T) {
	for _, base := range []string{`<mspace width="1em" height="1em"></mspace>`,
		`<mspace width="1em" height="12.5px" depth="3.125px"></mspace>`} {
		_, ops, _ := mathComposed(t, mathStretchFace(t, nil), `<math><msqrt>`+base+`</msqrt></math>`)
		if gs := glyphsIn(ops); len(gs) != 1 || len(gs[0].Glyphs) != 1 || gs[0].Glyphs[0].GID != 18 {
			t.Errorf("%s: the sign is drawn as %+v, want its variant", base, gs)
		}
	}
	// And over something taller than the bound allows, it says so.
	_, _, findings := mathComposed(t, mathStretchFace(t, nil),
		`<math><msqrt><mspace width="1em" height="40000px"></mspace></msqrt></math>`)
	if !mathFinding(findings, RuleLimit, "more than 4096 glyphs") {
		t.Errorf("an assembled sign past the bound is not reported: %v", findings)
	}
}

// TestAConstructionShapingWouldChangeIsDrawnAsGlyphs: in a face whose shaping
// draws "(" and "√" with other glyphs than their characters' own — a
// substitution every run gets — their constructions are drawn by index even
// at their own size: the text would draw something else.
func TestAConstructionShapingWouldChangeIsDrawnAsGlyphs(t *testing.T) {
	face := mathStretchFaceWith(t, nil, nil, func(o *fonttest.SFNTOptions) {
		o.Extra["GSUB"] = fonttest.GSUBSingle("ccmp", []int{mathGlyphParen, mathGlyphRadical}, []int{22, 18})
	})
	for doc, want := range map[string]int{
		`<math><mo>(</mo><mn>1</mn></math>`:      mathGlyphParen,
		`<math><msqrt><mn>1</mn></msqrt></math>`: mathGlyphRadical,
	} {
		_, ops, _ := mathComposed(t, face, doc)
		if gs := glyphsIn(ops); len(gs) != 1 || len(gs[0].Glyphs) != 1 || gs[0].Glyphs[0].GID != want {
			t.Errorf("%s: drawn as %+v, want glyph %d, its own", doc, gs, want)
		}
	}
}

// noMathFace is the test face with no MATH table.
func noMathFace(t *testing.T) *shape.Face {
	t.Helper()
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{Name: "NoMath", UnitsPerEm: 1024,
		Ascent: 1024, Descent: -256, Glyphs: mathTestGlyphs}))
	if err != nil {
		t.Fatal(err)
	}
	return face
}

// TestARadicalInAFaceWithNoMathTableIsItsText: the sign is its text, measured
// as text — 600 wide, its ink 800 up — with the bar after it and the sign's
// ink's top at the bar's top; and its preferred width is its advance.
func TestARadicalInAFaceWithNoMathTableIsItsText(t *testing.T) {
	face := noMathFace(t)
	root, ops, _ := mathComposed(t, face, `<math><msqrt id="q"><mn>1</mn></msqrt></math>`)
	q := find(t, root, "q")
	if got := q.mathMarks; len(got) != 1 || got[0].X != 600 || got[0].W != 512 {
		t.Errorf("the bar is %v, want it from 600 across the 512 of the 1", got)
	}
	// The face states no underline, so its rules are nought thick and the bar
	// paints nothing; where it is, is still where the sign hangs from.
	var sign *DrawText
	for _, op := range ops {
		if v, ok := op.(DrawText); ok && v.Text == "√" {
			sign = &v
		}
	}
	if sign == nil || len(q.mathMarks) != 1 || sign.At.Y.Sub(q.ContentRect().Y.Add(q.mathMarks[0].Y)) != 800 {
		t.Errorf("the sign %+v and the bar %v: want the sign's baseline 800 below the bar's top", sign, q.mathMarks)
	}
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: `<math><msqrt id="q"><mn>1</mn></msqrt></math>`, Fonts: set,
		CSS: []Stylesheet{{Source: `mn { font-size: 16px }`}}})
	l := newLayouter(b.Root, A4.Content(), set, nil)
	var e *Box
	var walk func(*Box)
	walk = func(x *Box) {
		if x.Element != nil {
			if id, _ := x.Element.Attr("id"); id == "q" {
				e = x
			}
		}
		for _, k := range x.Children {
			walk(k)
		}
	}
	walk(b.Root)
	if size := l.mathOuterSize(e); size.max != 600+512 {
		t.Errorf("the root's size is %d, want 1112", size.max)
	}
}

// TestARootsIndexIsPlacedByItsOwnBox: an index with a descender — the y — is
// raised so that its bottom, 192 below its baseline, is at the 60% of the
// root's height; its sign is drawn after the index and its kerns, at 412; a
// raise of less than nothing puts the index's bottom below the root's; and
// the kerns are clamped, the one before to nothing and the one after to no
// further back than the index is wide.
func TestARootsIndexIsPlacedByItsOwnBox(t *testing.T) {
	root, _, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mroot id="r"><mn>1</mn><mn id="i">y</mn></mroot></math>`)
	// The square root of the 1 is 930 tall, from 106 below; the y's bottom is
	// 558 − 106 = 452 up, its baseline 644 up, its top 1092 up.
	checkMath(t, root, "r", 1524, 1092, 106, map[string][2]style.Unit{"i": {100, 0}})
	if got := find(t, root, "r").mathGlyphs; len(got) != 1 || got[0].at.X != 412 {
		t.Errorf("the sign is %+v, want it from 412", got)
	}
	root, _, _ = mathComposed(t, mathStretchFace(t, map[string]int{"RadicalDegreeBottomRaisePercent": -50}),
		`<math><mroot id="r"><mn>1</mn><mn>2</mn></mroot></math>`)
	// −465 of the 930, from 106 below: the 2's bottom is 571 below.
	if f := find(t, root, "r"); f.ContentRect().H.Sub(f.mathBaseline) != 571 {
		t.Errorf("the root reaches %d below its baseline, want 571", f.ContentRect().H.Sub(f.mathBaseline))
	}
	root, _, _ = mathComposed(t, mathStretchFace(t, map[string]int{"RadicalKernBeforeDegree": -100,
		"RadicalKernAfterDegree": -2000}), `<math><mroot id="r"><mn id="b">1</mn><mn id="i">2</mn></mroot></math>`)
	if i, b := geomIn(t, root, "i", "r"), geomIn(t, root, "b", "r"); i.x != 0 || b.x != 600 {
		t.Errorf("the index at %d and the base at %d, want 0 and 600: the sign over the index", i.x, b.x)
	}
}

// TestARootLaidOutAsABlockIsCentredWithItsBar: a square root whose parent is a
// CSS box is a block of its own, its content centred in it — its bar with it.
func TestARootLaidOutAsABlockIsCentredWithItsBar(t *testing.T) {
	root, _, _ := mathComposed(t, mathStretchFace(t, nil),
		`<math><mrow style="display: block; width: 100px"><msqrt id="q"><mn>1</mn></msqrt></mrow></math>`)
	dx := style.Unit(6400 - 1112).Div(2)
	if got := find(t, root, "q").mathMarks; len(got) != 1 || got[0].X != dx.Add(600) {
		t.Errorf("the bar is %v, want it from %d", got, dx.Add(600))
	}
}

// TestAnInlineOperatorsSizeIsItsText: an operator of inline axis is not given
// the width of a vertical construction, even one its font has.
func TestAnInlineOperatorsSizeIsItsText(t *testing.T) {
	face := mathStretchFaceWith(t, nil, func(o *fonttest.MathOptions) {
		o.VertVariants[mathGlyphArrow] = []fonttest.MathVariant{{Glyph: 27, Advance: 1200}}
	}, nil)
	set := namedFaceSet{family: "math", face: face, standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mo id="e">→</mo></math>`, Fonts: set,
		CSS: []Stylesheet{{Source: `mo { font-size: 16px }`}}})
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
	if size := l.mathOuterSize(e); size.max != 600 {
		t.Errorf("the arrow's size is %d, want its text's 600", size.max)
	}
}
