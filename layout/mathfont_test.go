package layout

import (
	"encoding/binary"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// TestTheConstantsFallBackAsSection51Says: a face with no MATH table is laid
// out by §5.1's fallbacks — multiples of the default rule thickness (post's
// underline thickness), fractions of OS/2's x-height, OS/2's script offsets,
// fractions of an em, and nought for the rest — and a face with one by its
// table, every constant of it, nought included.
func TestTheConstantsFallBackAsSection51Says(t *testing.T) {
	os2 := make([]byte, 96)
	binary.BigEndian.PutUint16(os2[0:], 2)   // version 2, which states sxHeight
	binary.BigEndian.PutUint16(os2[16:], 96) // ySubscriptYOffset
	binary.BigEndian.PutUint16(os2[24:], 160)
	binary.BigEndian.PutUint16(os2[86:], 512) // sxHeight
	post := make([]byte, 32)
	binary.BigEndian.PutUint32(post[0:], 0x00030000)
	binary.BigEndian.PutUint16(post[10:], 32) // underlineThickness
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{UnitsPerEm: 1024,
		Glyphs: []fonttest.Glyph{{Rune: 'a', Advance: 500}},
		Extra:  map[string][]byte{"OS/2": os2, "post": post}}))
	if err != nil {
		t.Fatal(err)
	}
	// 16px over 1024 units: one unit is one style.Unit.
	m := mathFont{face: face, size: 16, scale: 16.0 / 1024, desc: face.Descriptor()}
	em := style.Unit(1024)
	for c, want := range map[shape.MathConstant]style.Unit{
		shape.MathAxisHeight:                        256,
		shape.MathAccentBaseHeight:                  512,
		shape.MathSubscriptShiftDown:                96,
		shape.MathSubscriptTopMax:                   512 * 4 / 5,
		shape.MathSuperscriptShiftUp:                160,
		shape.MathSuperscriptBottomMin:              128,
		shape.MathSuperscriptBottomMaxWithSubscript: 512 * 4 / 5,
		shape.MathSubSuperscriptGapMin:              128,
		shape.MathSpaceAfterScript:                  em.Mul(1.0 / 24),
		shape.MathStackGapMin:                       96,
		shape.MathStackDisplayStyleGapMin:           224,
		shape.MathFractionRuleThickness:             32,
		shape.MathFractionNumDisplayStyleGapMin:     96,
		shape.MathRadicalVerticalGap:                40,
		shape.MathRadicalDisplayStyleVerticalGap:    32 + 128,
		shape.MathRadicalKernBeforeDegree:           em.Mul(5.0 / 18),
		shape.MathRadicalKernAfterDegree:            em.Mul(-10.0 / 18),
		shape.MathFractionNumeratorShiftUp:          0,
		shape.MathUpperLimitGapMin:                  0,
		shape.MathDisplayOperatorMinHeight:          0,
	} {
		if got := m.constant(c); got != want {
			t.Errorf("%v falls back to %d, want %d", c, got, want)
		}
	}
	if got := m.radicalDegreeBottomRaise(); got != 0.6 {
		t.Errorf("radicalDegreeBottomRaisePercent falls back to %v, want 0.6", got)
	}

	// With a table, every constant is the table's, nought included.
	withTable := mathTestFace(t)
	tbl, _ := withTable.MathTable()
	mt := mathFont{face: withTable, table: tbl, size: 16, scale: 16.0 / 1024, desc: withTable.Descriptor()}
	if got := mt.constant(shape.MathAxisHeight); got != 256 {
		t.Errorf("the table's axis height is read as %d, want 256", got)
	}
	if got := mt.constant(shape.MathStackGapMin); got != 0 {
		t.Errorf("the table's stack gap is read as %d, want its nought", got)
	}
	if got := mt.radicalDegreeBottomRaise(); got != 0 {
		t.Errorf("the table's radicalDegreeBottomRaisePercent is read as %v, want its nought", got)
	}
}

// TestAFormulasFontIsItsFirstAvailableFontAtItsSize: the constants are the
// table's scaled to the element's size — the axis at 256 units of 1024 is
// 8px at 32px — and the table is read once for the face, where
// Face.MathTable reads it afresh each time.
func TestAFormulasFontIsItsFirstAvailableFontAtItsSize(t *testing.T) {
	set := namedFaceSet{family: "math", face: mathTestFace(t), standard: StandardFonts()}
	b := Build(Input{HTML: `<math><mn id="n" style="font-size: 32px">1</mn></math>`, Fonts: set})
	l := newLayouter(b.Root, A4.Content(), set, NewRecorder(nil))
	var n *Box
	var walk func(*Box)
	walk = func(x *Box) {
		if x.Element != nil {
			if id, _ := x.Element.Attr("id"); id == "n" {
				n = x
			}
		}
		for _, c := range x.Children {
			walk(c)
		}
	}
	walk(b.Root)
	m := l.mathFontFor(n)
	if got := m.constant(shape.MathAxisHeight); got != 512 {
		t.Errorf("the axis at 32px is %d, want 512", got)
	}
	if again := l.mathFontFor(n); again.table != m.table || m.table == nil {
		t.Errorf("the MATH table was read twice (%p, %p)", m.table, again.table)
	}

	// radicalDegreeBottomRaisePercent is a percentage.
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{UnitsPerEm: 1024, Glyphs: mathTestGlyphs,
		Extra: map[string][]byte{"MATH": fonttest.MATH(fonttest.MathOptions{
			Constants: map[string]int{"RadicalDegreeBottomRaisePercent": 65}})}}))
	if err != nil {
		t.Fatal(err)
	}
	tbl, _ := face.MathTable()
	if got := (mathFont{face: face, table: tbl}).radicalDegreeBottomRaise(); got != 0.65 {
		t.Errorf("a raise of 65%% is read as %v", got)
	}
}
