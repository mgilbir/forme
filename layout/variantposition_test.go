package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4 §6.5's font-variant-position: the small raised and lowered forms
// a font draws for the characters that get them.
//
// It is the odd one of the family in two ways. Its value is one of three rather
// than a set, because a run is a subscript or a superscript or neither. And it
// is the only one §6.5 lets a user agent *synthesize* — by scaling and
// repositioning the ordinary glyphs — which this engine does not do, for the
// reason reportPosition gives: a font's own superscript is a second drawing with
// its weight adjusted for the size it is set at, and a scaled copy of the
// ordinary glyph reads thin beside the letters around it.

// positionFontSet is a set whose "Pos" family declares 'subs' and 'sups'.
func positionFontSet(t *testing.T) FontSet {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	for _, tag := range []string{"subs", "sups"} {
		if !faceDeclares(face, tag) {
			t.Fatalf("the embedded face declares %v and not %s", face.Features(), tag)
		}
	}
	return namedFaceSet{family: "Pos", face: face, standard: StandardFonts()}
}

// TestPositionReachesTheRunThatIsDrawn is the plumbing, end to end.
func TestPositionReachesTheRunThatIsDrawn(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      shape.Position
	}{
		{"nothing declared", "", shape.PositionNormal},
		{"normal", "font-variant-position: normal", shape.PositionNormal},
		{"the longhand", "font-variant-position: sub", shape.PositionSub},
		{"the shorthand", "font-variant: super", shape.PositionSuper},
		{"both at once", "font-variant: sub super", shape.PositionNormal},
		{"a value of no level at all", "font-variant-position: inline", shape.PositionNormal},
	} {
		frag, _ := layoutWith(t, positionFontSet(t),
			`<div id="d" style="`+c.css+`">2</div>`,
			`body{margin:0} #d{font-family:Pos; font-size:20px}`)
		var got shape.Features
		var found bool
		for _, op := range Paint(frag) {
			if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "2") {
				got, found = v.Features, true
			}
		}
		if !found {
			t.Fatalf("%s: the fixture drew no run", c.what)
		}
		if got.Position != c.want {
			t.Errorf("%s: the run carries Position=%v, want %v",
				c.what, got.Position.Features(), c.want.Features())
		}
	}
}

// TestTheFaceDrawsItsOwnSubscriptsAndSuperscripts.
//
// The numbers are HarfBuzz's over the bundled Noto Sans, and the third row is
// the one worth having: a font's 'sups' covers the characters its designer drew
// it for and no others. Noto Sans covers the digits, the lowercase letters and
// the arithmetic signs and leaves the capitals, so "H2O" comes out with only its
// 2 lowered — which is what a chemical formula wants and is not something a
// whole-run displacement could produce.
func TestTheFaceDrawsItsOwnSubscriptsAndSuperscripts(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	for _, c := range []struct {
		what, text string
		p          shape.Position
		want       []int
	}{
		{"plain", "2", shape.PositionNormal, []int{21}},
		{"sub", "2", shape.PositionSub, []int{2254}},
		{"super", "2", shape.PositionSuper, []int{116}},
		{"a formula", "H2O", shape.PositionSub, []int{43, 2254, 50}},
	} {
		got, _ := face.ShapeGlyphsInContext(c.text, "", "",
			shape.Features{Position: c.p})
		var ids []int
		for _, g := range got {
			ids = append(ids, g.GID)
		}
		if len(ids) != len(c.want) {
			t.Errorf("%s set %q as %v, want HarfBuzz's %v", c.what, c.text, ids, c.want)
			continue
		}
		for i := range ids {
			if ids[i] != c.want[i] {
				t.Errorf("%s set %q as %v, want HarfBuzz's %v",
					c.what, c.text, ids, c.want)
				break
			}
		}
	}
	// And the run is narrower, which is the half a reader sees: the face's
	// subscript two is 350 units where the ordinary one is 572.
	plain, _ := face.ShapeGlyphsInContext("2", "", "", shape.Features{})
	low, _ := face.ShapeGlyphsInContext("2", "", "",
		shape.Features{Position: shape.PositionSub})
	if shape.MeasureGlyphs(low, 1000) >= shape.MeasureGlyphs(plain, 1000) {
		t.Errorf("the subscript measures %g and the ordinary glyph %g; the "+
			"face's own subscript is a narrower drawing",
			shape.MeasureGlyphs(low, 1000), shape.MeasureGlyphs(plain, 1000))
	}
}

// TestPositionIsReportedWhenTheFaceHasNeither.
//
// The fourteen standard PDF faces declare no OpenType feature at all, so a
// document that asks them for a subscript gets its 2 the same size as the H
// beside it. That reads as ordinary text rather than as something missing, which
// is the shape of failure §6.3's findings exist for.
func TestPositionIsReportedWhenTheFaceHasNeither(t *testing.T) {
	for _, c := range []struct{ value, tag, which string }{
		{"sub", "subs", "a subscript"},
		{"super", "sups", "a superscript"},
	} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">H2O</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-position: `+c.value+` }`)
		f, ok := findingNaming(findings, "font-variant-position")
		if !ok {
			t.Errorf("%q was not reported: %v", c.value, findings)
			continue
		}
		if !f.Unsupported() {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything; §7.1's companion signal counts on it", c.value)
		}
		if !strings.Contains(f.Message, c.tag) {
			t.Errorf("the finding for %q says %q and does not name %s",
				c.value, f.Message, c.tag)
		}
		// Which of the two, because the page is wrong in opposite directions:
		// an exponent on the baseline and a formula's number on the baseline
		// look different and an author looking for one will not find the other.
		if !strings.Contains(f.Message, c.which) {
			t.Errorf("the finding for %q says %q and does not say which of the "+
				"two was asked for", c.value, f.Message)
		}
	}
}

// TestPositionIsNotReportedWhenTheFaceHasIt.
func TestPositionIsNotReportedWhenTheFaceHasIt(t *testing.T) {
	for _, value := range []string{"normal", "sub", "super"} {
		_, findings := layoutWith(t, positionFontSet(t),
			`<p id="p">H2O</p>`,
			`#p { font-family: Pos; font-size: 20px;
			      font-variant-position: `+value+` }`)
		if f, ok := findingNaming(findings, "font-variant-position"); ok {
			t.Errorf("the face declares what %q needs and the page was reported "+
				"anyway: %s", value, f.Message)
		}
	}
}

// TestOnlyOneHalfOfTheFeatureIsReported is the narrowing the *other* three
// properties do not need.
//
// A face may declare 'sups' and not 'subs' — a Latin face cut for setting
// footnote marks and nothing else is exactly that — and asking it for a
// subscript is a request it cannot carry out while a superscript is one it can.
// A report keyed on "the face has neither" would say nothing about the first.
func TestOnlyOneHalfOfTheFeatureIsReported(t *testing.T) {
	set := namedFaceSet{family: "Sup", face: superscriptOnlyFace(t),
		standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p id="p">2</p>`,
		`#p { font-family: Sup; font-size: 20px; font-variant-position: super }`)
	if f, ok := findingNaming(findings, "font-variant-position"); ok {
		t.Errorf("the face declares sups and a superscript was reported: %s",
			f.Message)
	}
	_, findings = layoutWith(t, set,
		`<p id="p">2</p>`,
		`#p { font-family: Sup; font-size: 20px; font-variant-position: sub }`)
	f, ok := findingNaming(findings, "font-variant-position")
	if !ok {
		t.Fatalf("the face declares no subs and a subscript was not reported: %v",
			findings)
	}
	if !strings.Contains(f.Message, "subs") || strings.Contains(f.Message, "sups") {
		t.Errorf("the finding is %q; it has to name the half the face has not "+
			"got and not the half it has", f.Message)
	}
}

// TestPositionIsNotReportedWhereThereIsNothingToRaise.
//
// The narrowing is the loosest of the four, and that is the property rather than
// a corner cut: which characters a font's 'sups' covers is the designer's
// choice, and what a face declaring *nothing* would have covered cannot be known
// from here. What can be known is that a raised space is a space.
func TestPositionIsNotReportedWhereThereIsNothingToRaise(t *testing.T) {
	for _, text := range []string{" ", " ", "  "} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">a<span id="s">`+text+`</span>b</p>`,
			`#p { font-family: Helvetica; font-size: 20px }
			 #s { font-variant-position: super }`)
		if f, ok := findingNaming(findings, "font-variant-position"); ok {
			t.Errorf("%q holds nothing a superscript could change and was "+
				"reported: %s", text, f.Message)
		}
	}
}

// TestVariantPositionOfReadsTheProperty is the reader on its own.
func TestVariantPositionOfReadsTheProperty(t *testing.T) {
	for _, c := range []struct {
		raw       string
		want      shape.Position
		unhandled string
	}{
		{raw: ""},
		{raw: "normal"},
		{raw: "  Sub  ", want: shape.PositionSub},
		{raw: "super", want: shape.PositionSuper},
		{raw: "sub super", unhandled: "sub super"},
		{raw: "inline", unhandled: "inline"},
	} {
		got, unhandled := variantPositionOf(c.raw)
		if got != c.want || unhandled != c.unhandled {
			t.Errorf("variantPositionOf(%q) = %v, %q; want %v, %q", c.raw,
				got.Features(), unhandled, c.want.Features(), c.unhandled)
		}
	}
}

// superscriptOnlyFace declares 'sups' and not 'subs': a face cut for setting
// footnote marks and nothing below the line.
func superscriptOnlyFace(t *testing.T) *shape.Face {
	t.Helper()
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "SupOnly",
		Glyphs: []fonttest.Glyph{
			{Rune: '2', Advance: 500, HasShape: true},
			{Rune: 0xE000, Advance: 300, HasShape: true}, // 2.sups
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{"sups": {{1}, {2}}}),
		},
	}))
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}
