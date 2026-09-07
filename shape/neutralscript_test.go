package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// Which script a run of digits inside Arabic is shaped as.
//
// A bidirectional run is cut by *direction*. A European digit is class EN and
// the letters around it are AL, so a number written inside an Arabic word comes
// out as a piece of its own — and every character in that piece is Common, which
// decides no script. The piece was therefore shaped as "unknown", which selects
// DFLT, so a font stating its digit forms under 'arab' had them selected away by
// the direction the digits are read in.
//
// UAX #24's resolution is that a Common character takes the script around it,
// and that is what the piece takes now.

const (
	arabicBeh = 0x0628 // a dual-joining letter
	// A European digit, which is where the case is: Unicode gives U+0660..0669
	// the Arabic script, so an Arabic-Indic digit decides one on its own and was
	// never the problem. U+0031 is Common and decides nothing.
	commonOne = '1'
)

// The fixture's glyphs.
const (
	gidDigit = 1 + iota
	gidBeh
	gidLatinLetter
	gidArabForm    // what 'calt' makes of the digit, declared under 'arab'
	gidDefaultForm // what 'rclt' makes of it, declared under DFLT
)

// twoScriptDigitFace declares one form of the digit under 'arab' and another
// under the default script, so that the glyph that comes out names which script
// was chosen. A font declaring nothing under the default would not: a table that
// settles nothing hands back every feature, which is the arab one too.
func twoScriptDigitFace(t *testing.T) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "TwoScripts",
		Glyphs: []fonttest.Glyph{
			{Rune: commonOne, Advance: 500, HasShape: true},
			{Rune: arabicBeh, Advance: 500, HasShape: true},
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 0xE020, Advance: 200, HasShape: true},
			{Rune: 0xE021, Advance: 300, HasShape: true},
		},
		Extra: map[string][]byte{
			// Indexed in tag order, so 'calt' is feature 0 and 'rclt' is 1.
			"GSUB": fonttest.GSUBFormsIn(map[string][2][]int{
				"calt": {{gidDigit}, {gidArabForm}},
				"rclt": {{gidDigit}, {gidDefaultForm}},
			}, map[string]fonttest.Script{
				"arab": {Required: fonttest.NoFeature, Features: []int{0}},
				"DFLT": {Required: fonttest.NoFeature, Features: []int{1}},
			}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// formOf is the glyph the digit came out as.
func formOf(t *testing.T, f *Face, s string) int {
	t.Helper()
	glyphs, _ := f.ShapeGlyphs(s)
	for _, g := range glyphs {
		if g.GID == gidArabForm || g.GID == gidDefaultForm || g.GID == gidDigit {
			return g.GID
		}
	}
	t.Fatalf("shaping %q produced no digit at all: %v", s, glyphs)
	return 0
}

// TestANumberInsideArabicIsShapedAsArabic.
func TestANumberInsideArabicIsShapedAsArabic(t *testing.T) {
	f := twoScriptDigitFace(t)

	if got := formOf(t, f, string([]rune{arabicBeh, commonOne, arabicBeh})); got != gidArabForm {
		t.Errorf("a digit written inside an Arabic word came out as glyph %d, "+
			"want the 'arab' form %d: the piece it is cut into is all Common, "+
			"and the script around it is Arabic", got, gidArabForm)
	}

	// The controls, and the reason this is not simply "always Arabic": a digit
	// with no script around it has none, and one between Latin letters takes
	// Latin's — neither of which this font declares, so both fall to the
	// default.
	for _, tc := range []struct {
		name string
		s    string
	}{
		{"on its own", string([]rune{commonOne})},
		{"between Latin letters", "a" + string([]rune{commonOne}) + "a"},
	} {
		if got := formOf(t, f, tc.s); got != gidDefaultForm {
			t.Errorf("a digit %s came out as glyph %d, want the default form %d",
				tc.name, got, gidDefaultForm)
		}
	}
}
