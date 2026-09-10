package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4 §6.7's font-variant-numeric: which of a face's digits a run is
// set in, how they are spaced, and what the face does with a fraction, an
// ordinal and a zero.
//
// It is a *set* where the capitals beside it are one value — a document may ask
// for oldstyle figures, tabular spacing and a slashed zero at once — and none of
// its eight keywords is synthesised anywhere. An oldstyle figure is a shape a
// designer drew and there is nothing to make one out of, so a face that declares
// none of what a value asks for sets the figures it has and the finding is the
// whole of what this engine can do about it.

// numericFontSet is a set whose "Num" family declares seven of the eight.
func numericFontSet(t *testing.T) FontSet {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	if !faceDeclares(face, "onum") {
		t.Fatalf("the embedded face declares %v and not onum; the fixtures "+
			"below would be asking for something no face here has", face.Features())
	}
	return namedFaceSet{family: "Num", face: face, standard: StandardFonts()}
}

// TestNumericReachesTheRunThatIsDrawn is the plumbing, end to end.
//
// The property is read where the box is and the answer has to travel to the
// backend, because the backend shapes the run for itself: a run measured with
// oldstyle figures and drawn without them is a line filled to one width and
// painted at another.
func TestNumericReachesTheRunThatIsDrawn(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      shape.Numeric
	}{
		{"nothing declared", "", 0},
		{"normal", "font-variant-numeric: normal", 0},
		{"the longhand", "font-variant-numeric: oldstyle-nums", shape.NumericOldstyle},
		{"the shorthand", "font-variant: oldstyle-nums", shape.NumericOldstyle},
		{"two groups", "font-variant-numeric: oldstyle-nums slashed-zero",
			shape.NumericOldstyle | shape.NumericSlashedZero},
		{"written the other way round", "font-variant-numeric: slashed-zero oldstyle-nums",
			shape.NumericOldstyle | shape.NumericSlashedZero},
		{"all five groups",
			"font-variant-numeric: oldstyle-nums tabular-nums diagonal-fractions ordinal slashed-zero",
			shape.NumericOldstyle | shape.NumericTabular |
				shape.NumericDiagonalFractions | shape.NumericOrdinal |
				shape.NumericSlashedZero},
		{"a group written twice", "font-variant-numeric: lining-nums oldstyle-nums", 0},
		{"a word of no level at all", "font-variant-numeric: roman-nums", 0},
	} {
		frag, _ := layoutWith(t, numericFontSet(t),
			`<div id="d" style="`+c.css+`">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px}`)
		var got shape.Features
		var found bool
		for _, op := range Paint(frag) {
			if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "0") {
				got, found = v.Features, true
			}
		}
		if !found {
			t.Fatalf("%s: the fixture drew no run", c.what)
		}
		if got.Numeric != c.want {
			t.Errorf("%s: the run carries Numeric=%v, want %v",
				c.what, got.Numeric.Features(), c.want.Features())
		}
	}
}

// TestNumericChangesTheGlyphsAndTheWidth is the feature actually applied.
//
// Carrying the set to the backend proves nothing on its own: a run that is
// marked and then set identically is the property doing nothing with a page that
// says it did.
func TestNumericChangesTheGlyphsAndTheWidth(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	plain, _ := face.ShapeGlyphsInContext("0123", "", "", shape.Features{})
	old, _ := face.ShapeGlyphsInContext("0123", "", "",
		shape.Features{Numeric: shape.NumericOldstyle})
	if shape.MeasureGlyphs(plain, 20) == shape.MeasureGlyphs(old, 20) {
		t.Error("the run is the same width either way; an oldstyle figure is " +
			"not the lining one it replaces and the measurement has to follow " +
			"the shaping")
	}
	// And the page follows the measurement, which is what carrying the set to
	// the backend is for.
	widths := map[string]float64{}
	for _, css := range []string{"", "font-variant-numeric: oldstyle-nums"} {
		frag, _ := layoutWith(t, numericFontSet(t),
			`<div id="d" style="`+css+`; float: left">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px}`)
		widths[css] = find(t, frag, "d").BorderRect.W.Px()
	}
	if widths[""] == widths["font-variant-numeric: oldstyle-nums"] {
		t.Errorf("a box shrunk to fit is %g wide either way; the figures it "+
			"holds are a different width", widths[""])
	}
}

// TestNumericIsReportedWhenTheFaceHasNone.
//
// The fourteen standard PDF faces carry no OpenType feature at all, so a
// document that asks them for oldstyle figures gets the lining ones. Nothing
// about the page says so.
func TestNumericIsReportedWhenTheFaceHasNone(t *testing.T) {
	for _, c := range []struct{ value, tags string }{
		{"oldstyle-nums", "onum"},
		{"lining-nums", "lnum"},
		{"diagonal-fractions", "frac"},
		{"stacked-fractions", "afrc"},
		{"ordinal", "ordn"},
		{"slashed-zero", "zero"},
		{"oldstyle-nums slashed-zero", "onum or zero"},
	} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">0123</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-numeric: `+c.value+` }`)
		f, ok := findingNaming(findings, "font-variant-numeric")
		if !ok {
			t.Errorf("%q was not reported: %v", c.value, findings)
			continue
		}
		if !f.Unsupported() {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything; §7.1's companion signal counts on it, because two "+
				"documents whose figures both came out as the face draws them "+
				"match each other and demonstrate nothing", c.value)
		}
		if !strings.Contains(f.Message, c.tags) {
			t.Errorf("the finding for %q says %q and does not name %s",
				c.value, f.Message, c.tags)
		}
	}
}

// TestNumericIsNotReportedWhenTheFaceHasIt.
func TestNumericIsNotReportedWhenTheFaceHasIt(t *testing.T) {
	for _, value := range []string{
		"normal", "oldstyle-nums", "lining-nums", "proportional-nums",
		"diagonal-fractions", "ordinal", "slashed-zero",
		"oldstyle-nums proportional-nums slashed-zero",
	} {
		_, findings := layoutWith(t, numericFontSet(t),
			`<p id="p">0123</p>`,
			`#p { font-family: Num; font-size: 20px;
			      font-variant-numeric: `+value+` }`)
		if f, ok := findingNaming(findings, "font-variant-numeric"); ok {
			t.Errorf("the face declares what %q needs and the page was reported "+
				"anyway: %s", value, f.Message)
		}
	}
}

// TestNumericIsNotReportedWhereItWouldChangeNothing.
//
// Every one of §6.7's features acts on digits, so a run with none in it is set
// identically with them and without — and a finding about it would be this
// engine calling a correct page a failure. It is the narrowing reportKerning
// makes for a "kern" a face has not got, one property along.
func TestNumericIsNotReportedWhereItWouldChangeNothing(t *testing.T) {
	for _, text := range []string{"Filler Text", "!?.,", "日本語", "१२३"} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">`+text+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-numeric: oldstyle-nums slashed-zero }`)
		if f, ok := findingNaming(findings, "font-variant-numeric"); ok {
			t.Errorf("%q has no digit in it and was reported anyway: %s",
				text, f.Message)
		}
	}
}

// TestTabularFiguresAreNotReportedWhereTheFaceAlreadySetsThem.
//
// "tabular-nums" asks for digits that all take the same room, and almost every
// text face draws them that way to begin with — the fourteen standard PDF faces
// do, and so does Noto Sans. A face already setting the page the declaration
// asks for is not a gap, and reporting it would hold a correct document out of
// the clean count for ever: it is the argument reportKerning makes for a "kern"
// a face has not got, arrived at from the other side.
//
// "proportional-nums" is the same question with the answer reversed, and the
// fixture is a face built to answer it — nothing in the checkout has digits of
// differing widths and no 'pnum' to fix them.
func TestTabularFiguresAreNotReportedWhereTheFaceAlreadySetsThem(t *testing.T) {
	// Helvetica: every digit 556 units, no OpenType feature at all.
	_, findings := layoutWith(t, StandardFonts(),
		`<p id="p">0123</p>`,
		`#p { font-family: Helvetica; font-size: 20px;
		      font-variant-numeric: tabular-nums }`)
	if f, ok := findingNaming(findings, "font-variant-numeric"); ok {
		t.Errorf("a face whose digits already share an advance was reported for "+
			"tabular-nums: %s", f.Message)
	}
	// And the request that face cannot answer still is.
	_, findings = layoutWith(t, StandardFonts(),
		`<p id="p">0123</p>`,
		`#p { font-family: Helvetica; font-size: 20px;
		      font-variant-numeric: proportional-nums }`)
	if _, ok := findingNaming(findings, "font-variant-numeric"); !ok {
		t.Errorf("a face whose digits all share an advance cannot set them "+
			"proportionally and was not reported: %v", findings)
	}

	// The other way round, on a face whose digits differ.
	set := namedFaceSet{family: "Prop", face: proportionalDigitFace(t),
		standard: StandardFonts()}
	_, findings = layoutWith(t, set,
		`<p id="p">0123</p>`,
		`#p { font-family: Prop; font-size: 20px;
		      font-variant-numeric: proportional-nums }`)
	if f, ok := findingNaming(findings, "font-variant-numeric"); ok {
		t.Errorf("a face whose digits already differ was reported for "+
			"proportional-nums: %s", f.Message)
	}
	_, findings = layoutWith(t, set,
		`<p id="p">0123</p>`,
		`#p { font-family: Prop; font-size: 20px;
		      font-variant-numeric: tabular-nums }`)
	if _, ok := findingNaming(findings, "font-variant-numeric"); !ok {
		t.Errorf("a face whose digits differ cannot line them up and was not "+
			"reported: %v", findings)
	}
}

// TestNumericOfReadsTheProperty is the reader on its own.
func TestNumericOfReadsTheProperty(t *testing.T) {
	for _, c := range []struct {
		raw       string
		want      shape.Numeric
		unhandled string
	}{
		{raw: ""},
		{raw: "normal"},
		{raw: "  Oldstyle-Nums  ", want: shape.NumericOldstyle},
		{raw: "ordinal slashed-zero",
			want: shape.NumericOrdinal | shape.NumericSlashedZero},
		{raw: "tabular-nums lining-nums",
			want: shape.NumericLining | shape.NumericTabular},
		// A group written twice is not a value: §6.7's grammar is a "||" of
		// five terms and a term may appear once.
		{raw: "lining-nums oldstyle-nums", unhandled: "oldstyle-nums"},
		{raw: "ordinal ordinal", unhandled: "ordinal"},
		{raw: "roman-nums", unhandled: "roman-nums"},
		{raw: "oldstyle-nums roman-nums", unhandled: "roman-nums"},
	} {
		got, unhandled := numericOf(c.raw)
		if got != c.want || unhandled != c.unhandled {
			t.Errorf("numericOf(%q) = %v, %q; want %v, %q", c.raw,
				got.Features(), unhandled, c.want.Features(), c.unhandled)
		}
	}
}

// proportionalDigitFace is a face whose digits are of differing widths and which
// declares no feature at all: one that is already setting proportional figures
// and cannot be asked to line them up.
//
// Built rather than fetched. Nothing in the checkout is in that state — a text
// face with proportional figures almost always carries 'tnum' to fix them — and
// it is the only shape of font that can tell the two halves of the narrowing
// apart.
func proportionalDigitFace(t *testing.T) *shape.Face {
	t.Helper()
	glyphs := make([]fonttest.Glyph, 0, 10)
	for i, r := range "0123456789" {
		glyphs = append(glyphs, fonttest.Glyph{
			Rune: r, Advance: 400 + 20*i, HasShape: true,
		})
	}
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "PropDigits", Glyphs: glyphs,
	}))
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}
