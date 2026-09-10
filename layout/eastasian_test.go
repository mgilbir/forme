package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4 §6.9's font-variant-east-asian, in layout: read off the box,
// carried to the run, and reported where the face cannot carry it out.
//
// What is different about it from the figures beside it is the reach. The six
// national forms and 'ruby' act on East Asian characters and a run of Latin is
// set identically with them and without — but the width pair acts on Latin as
// well, because a Japanese font draws the ASCII letters twice, and
// "full-width" asks for the second drawing. So the narrowing is three classes
// and not one.

// eastAsianFontSet is a set whose "EA" family declares all nine of §6.9's
// features, over one character of each class.
func eastAsianFontSet(t *testing.T) FontSet {
	t.Helper()
	return namedFaceSet{family: "EA", face: layoutEastAsianFace(t),
		standard: StandardFonts()}
}

// layoutEastAsianFace is the shape package's fixture built again here, because
// nothing in the checkout declares all nine: Noto Sans JP has six of them.
func layoutEastAsianFace(t *testing.T) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "EastAsian",
		Glyphs: []fonttest.Glyph{
			{Rune: '漢', Advance: 1000, HasShape: true},    // 1
			{Rune: 'A', Advance: 500, HasShape: true},     // 2
			{Rune: 'Ａ', Advance: 1000, HasShape: true},    // 3
			{Rune: 'あ', Advance: 1000, HasShape: true},    // 4
			{Rune: 0xE000, Advance: 1000, HasShape: true}, // 5, 漢.jp78
			{Rune: 0xE001, Advance: 1000, HasShape: true}, // 6, A.fwid
			{Rune: 0xE002, Advance: 500, HasShape: true},  // 7, Ａ.pwid
			{Rune: 0xE003, Advance: 1000, HasShape: true}, // 8, あ.ruby
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"jp78": {{1}, {5}},
				"jp83": {{1}, {5}},
				"jp90": {{1}, {5}},
				"jp04": {{1}, {5}},
				"smpl": {{1}, {5}},
				"trad": {{1}, {5}},
				"fwid": {{2}, {6}},
				"pwid": {{3}, {7}},
				"ruby": {{4}, {8}},
			}),
		},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}

// TestEastAsianReachesTheRunThatIsDrawn is the plumbing, end to end.
func TestEastAsianReachesTheRunThatIsDrawn(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      shape.EastAsian
	}{
		{"nothing declared", "", 0},
		{"normal", "font-variant-east-asian: normal", 0},
		{"the longhand", "font-variant-east-asian: jis78", shape.EastAsianJis78},
		{"the shorthand", "font-variant: jis04", shape.EastAsianJis04},
		{"two groups", "font-variant-east-asian: traditional full-width",
			shape.EastAsianTraditional | shape.EastAsianFullWidth},
		{"written the other way round", "font-variant-east-asian: full-width traditional",
			shape.EastAsianTraditional | shape.EastAsianFullWidth},
		{"all three groups", "font-variant-east-asian: jis90 proportional-width ruby",
			shape.EastAsianJis90 | shape.EastAsianProportionalWidth | shape.EastAsianRuby},
		{"a group written twice", "font-variant-east-asian: jis78 jis83", 0},
		{"a word of no level at all", "font-variant-east-asian: jis2000", 0},
	} {
		frag, _ := layoutWith(t, eastAsianFontSet(t),
			`<div id="d" style="`+c.css+`">漢</div>`,
			`body{margin:0} #d{font-family:EA; font-size:20px}`)
		var got shape.Features
		var found bool
		for _, op := range Paint(frag) {
			if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "漢") {
				got, found = v.Features, true
			}
		}
		if !found {
			t.Fatalf("%s: the fixture drew no run", c.what)
		}
		if got.EastAsian != c.want {
			t.Errorf("%s: the run carries EastAsian=%v, want %v",
				c.what, got.EastAsian.Features(), c.want.Features())
		}
	}
}

// TestFullWidthChangesTheWidthOfALatinRun is the half of §6.9 that reaches text
// nobody would call East Asian.
//
// A Japanese font draws the ASCII letters twice, once proportionally and once on
// the ideographic advance, and "full-width" asks for the second drawing. It is
// the case that stops the narrowing below from being "the run has an ideograph
// in it".
func TestFullWidthChangesTheWidthOfALatinRun(t *testing.T) {
	widths := map[string]float64{}
	for _, css := range []string{"", "font-variant-east-asian: full-width"} {
		frag, _ := layoutWith(t, eastAsianFontSet(t),
			`<div id="d" style="`+css+`; float: left">A</div>`,
			`body{margin:0} #d{font-family:EA; font-size:100px}`)
		widths[css] = find(t, frag, "d").BorderRect.W.Px()
	}
	if got := widths["font-variant-east-asian: full-width"]; got != 100 {
		t.Errorf("a full-width \"A\" at 100px is %gpx wide, want 100 — one em", got)
	}
	if widths[""] >= widths["font-variant-east-asian: full-width"] {
		t.Errorf("the letter is %gpx proportionally and %gpx at full width",
			widths[""], widths["font-variant-east-asian: full-width"])
	}
}

// TestEastAsianIsReportedWhenTheFaceHasNone.
//
// A face that draws the characters and declares no feature over them, which is
// what most East Asian faces are: Noto Sans JP has six of the nine and no font
// in the checkout has all of them, so a fixture set in the *fallback* library
// would be reporting on a face that can do the job.
func TestEastAsianIsReportedWhenTheFaceHasNone(t *testing.T) {
	set := namedFaceSet{family: "Plain", face: featurelessEastAsianFace(t),
		standard: StandardFonts()}
	for _, c := range []struct{ value, text, tags string }{
		{"jis78", "漢", "jp78"},
		{"jis83", "漢", "jp83"},
		{"jis90", "漢", "jp90"},
		{"jis04", "漢", "jp04"},
		{"simplified", "漢", "smpl"},
		{"traditional", "漢", "trad"},
		{"ruby", "あ", "ruby"},
		{"full-width", "A", "fwid"},
		{"proportional-width", "Ａ", "pwid"},
		{"jis78 full-width", "漢A", "jp78 or fwid"},
	} {
		_, findings := layoutWith(t, set,
			`<p id="p">`+c.text+`</p>`,
			`#p { font-family: Plain; font-size: 20px;
			      font-variant-east-asian: `+c.value+` }`)
		f, ok := findingNaming(findings, "font-variant-east-asian")
		if !ok {
			t.Errorf("%q over %q was not reported: %v", c.value, c.text, findings)
			continue
		}
		if !f.Unsupported() {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything; §7.1's companion signal counts on it", c.value)
		}
		if !strings.Contains(f.Message, c.tags) {
			t.Errorf("the finding for %q says %q and does not name %s",
				c.value, f.Message, c.tags)
		}
	}
}

// TestEastAsianIsNotReportedWhenTheFaceHasIt.
func TestEastAsianIsNotReportedWhenTheFaceHasIt(t *testing.T) {
	for _, value := range []string{
		"normal", "jis78", "jis04", "simplified", "traditional",
		"full-width", "proportional-width", "ruby",
		"jis90 proportional-width ruby",
	} {
		_, findings := layoutWith(t, eastAsianFontSet(t),
			`<p id="p">漢AＡあ</p>`,
			`#p { font-family: EA; font-size: 20px;
			      font-variant-east-asian: `+value+` }`)
		if f, ok := findingNaming(findings, "font-variant-east-asian"); ok {
			t.Errorf("the face declares what %q needs and the page was reported "+
				"anyway: %s", value, f.Message)
		}
	}
}

// TestEastAsianIsNotReportedWhereItWouldChangeNothing is the narrowing, and it
// is three classes because the nine features are.
//
// The six national forms and 'ruby' need an East Asian character. The two widths
// need a character with a width to change — one that has a full-width form for
// 'fwid', one that is a full-width form for 'pwid' — and an East Asian character
// counts for both, because a font may set its kana proportionally.
//
// The rows that matter most are the two Latin ones: a document that sets §6.9 on
// its body has Latin runs in a Latin face throughout, and reporting every one of
// them would hold a correct page out of the clean count for ever.
func TestEastAsianIsNotReportedWhereItWouldChangeNothing(t *testing.T) {
	for _, c := range []struct{ what, text, value string }{
		{"Latin under a national form", "Filler", "jis78"},
		{"Latin under ruby", "Filler", "ruby"},
		{"Cyrillic under full-width", "привет", "full-width"},
		{"already-proportional Latin under proportional-width", "Filler",
			"proportional-width"},
		{"a script with no width twin", "שלום", "full-width"},
	} {
		set := namedFaceSet{family: "Plain", face: featurelessEastAsianFace(t),
			standard: StandardFonts()}
		_, findings := layoutWith(t, set,
			`<p id="p">`+c.text+`</p>`,
			`#p { font-family: Plain; font-size: 20px;
			      font-variant-east-asian: `+c.value+` }`)
		if f, ok := findingNaming(findings, "font-variant-east-asian"); ok {
			t.Errorf("%s: %q under %q was reported and is set identically "+
				"either way: %s", c.what, c.text, c.value, f.Message)
		}
	}
}

// TestEastAsianOfReadsTheProperty is the reader on its own.
func TestEastAsianOfReadsTheProperty(t *testing.T) {
	for _, c := range []struct {
		raw       string
		want      shape.EastAsian
		unhandled string
	}{
		{raw: ""},
		{raw: "normal"},
		{raw: "  JIS78  ", want: shape.EastAsianJis78},
		{raw: "ruby full-width",
			want: shape.EastAsianFullWidth | shape.EastAsianRuby},
		{raw: "traditional proportional-width ruby",
			want: shape.EastAsianTraditional | shape.EastAsianProportionalWidth |
				shape.EastAsianRuby},
		// The first group is six alternatives and not a pair: any two of them
		// ask for the same ideograph in two shapes.
		{raw: "jis78 jis04", unhandled: "jis04"},
		{raw: "simplified traditional", unhandled: "traditional"},
		{raw: "jis90 simplified", unhandled: "simplified"},
		{raw: "full-width proportional-width", unhandled: "proportional-width"},
		{raw: "ruby ruby", unhandled: "ruby"},
		{raw: "jis2000", unhandled: "jis2000"},
	} {
		got, unhandled := eastAsianOf(c.raw)
		if got != c.want || unhandled != c.unhandled {
			t.Errorf("eastAsianOf(%q) = %v, %q; want %v, %q", c.raw,
				got.Features(), unhandled, c.want.Features(), c.unhandled)
		}
	}
}

// featurelessEastAsianFace draws the four characters the fixtures use and
// declares no OpenType feature at all: an East Asian face asked for forms its
// designer did not cut.
func featurelessEastAsianFace(t *testing.T) *shape.Face {
	t.Helper()
	face, err := shape.Load(fonttest.SFNT(fonttest.SFNTOptions{
		Name: "PlainEastAsian",
		Glyphs: []fonttest.Glyph{
			{Rune: '漢', Advance: 1000, HasShape: true},
			{Rune: 'A', Advance: 500, HasShape: true},
			{Rune: 'Ａ', Advance: 1000, HasShape: true},
			{Rune: 'あ', Advance: 1000, HasShape: true},
			{Rune: 'F', Advance: 500, HasShape: true},
			{Rune: 'i', Advance: 500, HasShape: true},
			{Rune: 'l', Advance: 500, HasShape: true},
			{Rune: 'e', Advance: 500, HasShape: true},
			{Rune: 'r', Advance: 500, HasShape: true},
			{Rune: 'п', Advance: 500, HasShape: true},
			{Rune: 'ש', Advance: 500, HasShape: true},
		},
	}))
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}
