package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
)

// text-combine-upright: all, and where its composition goes.
//
// The fixture is mixedorientation_test.go's: VerticalFallbacks.ttf at 20px on
// a 25px line, in a vertical-rl box 50px wide. "o" advances 500 units, 10px,
// lying along the line or across the page; the face's ascent and descent are
// 20 and 5; the line's middle — the central baseline — is 37.5 across the
// page. So a composition's square is 20px along the line and centred on
// x = 37.5, and its text, set across the page, is centred in it: by its width
// across, and by the 25px of its ascent and descent down, which puts its
// baseline 20 − 2.5 = 17.5 below the top of the square.

// combineRuns is mixedRuns with a composition in the middle of the line.
func combineRuns(t *testing.T, htmlSrc, extraCSS string) []DrawText {
	t.Helper()
	return mixedRuns(t, verticalFallbacksSet(t), htmlSrc, mixedCSS+extraCSS)
}

// TestACompositionIsOneEmOfTheLine.
//
// Along the line the composition is an em whatever its text measures: two
// letters lying down are 20, the composition after them 20, and the letters
// after it start at 40. Across the page its text is a horizontal run — neither
// Sideways nor Upright — centred on the line's middle.
//
// "oo" is 20px across, exactly an em, so it is drawn as it is: from
// 37.5 − 10 = 27.5. "o" is 10px, from 32.5. "ooo" is 30px and is squeezed to
// the em by two thirds, so it is drawn 20px wide from 27.5 with WidthScale
// 2/3.
func TestACompositionIsOneEmOfTheLine(t *testing.T) {
	for _, c := range []struct {
		text  string
		x     float64
		scale float64
	}{
		{"oo", 27.5, 0},
		{"o", 32.5, 0},
		{"ooo", 27.5, 2.0 / 3},
	} {
		runs := combineRuns(t,
			`<div id="d">oo<span style="text-combine-upright: all">`+c.text+`</span>oo</div>`, "")
		if len(runs) != 3 {
			t.Fatalf("%q: the line drew %d runs, want 3: %+v", c.text, len(runs), runs)
		}
		before, tcy, after := runs[0], runs[1], runs[2]
		if !before.Sideways || !after.Sideways || after.At.Y.Px() != 40 {
			t.Errorf("%q: the letters after the composition start at y=%g, want 40 — "+
				"twenty for the two before it and an em for it", c.text, after.At.Y.Px())
		}
		if tcy.Text != c.text || tcy.Sideways || tcy.Upright || tcy.Anticlockwise {
			t.Errorf("%q: the composition drew %q sideways=%v upright=%v; want its text "+
				"drawn across the page", c.text, tcy.Text, tcy.Sideways, tcy.Upright)
		}
		if tcy.At.X.Px() != c.x || tcy.At.Y.Px() != 37.5 {
			t.Errorf("%q: the composition is drawn from (%g, %g), want (%g, 37.5)",
				c.text, tcy.At.X.Px(), tcy.At.Y.Px(), c.x)
		}
		if got := tcy.WidthScale; got < c.scale-1e-9 || got > c.scale+1e-9 {
			t.Errorf("%q: the composition is squeezed by %v, want %v", c.text, got, c.scale)
		}
		// Its ink is the em square it was given, across: 27.5 to 47.5 for the
		// two that reach it, which is the line's middle ± 10.
		if c.text != "o" {
			ink := textInk(tcy)
			if ink.X.Px() != 27.5 || ink.W.Px() != 20 {
				t.Errorf("%q: the composition inks x=%g..%g, want 27.5..47.5", c.text,
					ink.X.Px(), ink.X.Px()+ink.W.Px())
			}
		}
	}
}

// TestACompositionIsOneCharacterForSpacing.
//
// §9.1.2 treats a composition as "a single glyph representing the Object
// Replacement Character U+FFFC" for spacing: letter-spacing goes after it
// once, and not after each of its characters, and none goes inside it.
func TestACompositionIsOneCharacterForSpacing(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">ooo</span>o</div>`,
		` #d { letter-spacing: 3px }`)
	if len(runs) != 3 {
		t.Fatalf("the line drew %d runs, want 3: %+v", len(runs), runs)
	}
	// "o" and its spacing, 13; the composition's em and its spacing, 23.
	if got := runs[2].At.Y.Px(); got != 36 {
		t.Errorf("the letter after the composition is at y=%g, want 36 — one "+
			"spacing after the composition and not three", got)
	}
	if runs[1].CharSpacing != 0 {
		t.Errorf("the composition's text is drawn with a letter-spacing of %v; "+
			"§9.1.2 composes it ignoring letter-spacing", runs[1].CharSpacing)
	}
}

// TestACompositionIsNotSpacedFromIdeographs.
//
// §8.4.1's autospace is between an ideograph and a letter, and the letters of
// a composition are not one of those: the composition is U+FFFC, and the
// section excludes a letter "upright in vertical text flow using ... the
// text-combine-upright property" by name. The control is the same letters in
// an ordinary span, which lie along the line and take their eighth of an em
// on each side.
func TestACompositionIsNotSpacedFromIdeographs(t *testing.T) {
	combined := combineRuns(t,
		`<div id="d">日<span style="text-combine-upright: all">oo</span>日</div>`, "")
	plain := combineRuns(t, `<div id="d">日<span>oo</span>日</div>`, "")
	if len(combined) != 3 || len(plain) != 3 {
		t.Fatalf("the lines drew %d and %d runs, want 3 each", len(combined), len(plain))
	}
	if got := combined[2].At.Y.Px(); got != 40 {
		t.Errorf("the ideograph after a composition is at y=%g, want 40 — an em "+
			"for the first and an em for the composition, and nothing between", got)
	}
	if got := plain[2].At.Y.Px(); got != 45 {
		t.Errorf("the control's second ideograph is at y=%g, want 45 — 2.5 either "+
			"side of the letters", got)
	}
}

// TestACompositionIsTheWholeOfARunOfText.
//
// §9.1.1: the composition is plain text not interrupted by a box boundary, and
// a candidate whose characters carry on into a neighbouring box that would
// combine too is not combined at all. "<tcy>oo<span>oo</span></tcy>" is four
// characters a boundary interrupts, so neither half combines; with the span
// first and nothing combinable before it, the span's "oo" is the whole of its
// sequence and combines. An element with nothing combinable either side is
// the ordinary case.
func TestACompositionIsTheWholeOfARunOfText(t *testing.T) {
	for _, c := range []struct {
		html     string
		combined int
	}{
		{`<span class="t">oo<span>oo</span></span>`, 0},
		{`oo<span class="t"><span>oo</span></span>`, 1},
		{`oo<span class="t">oo</span><span class="t">oo</span>`, 0},
		{`oo<span class="t">oo</span> <span class="t">oo</span>`, 2},
	} {
		runs := combineRuns(t, `<div id="d">`+c.html+`</div>`,
			` .t { text-combine-upright: all }`)
		n := 0
		for _, r := range runs {
			if !r.Sideways {
				n++
			}
		}
		if n != c.combined {
			t.Errorf("%s drew %d compositions, want %d: %+v", c.html, n, c.combined, runs)
		}
	}
}

// TestACompositionTrimsTheSpaceAtItsEnds.
//
// White space at the start and end of the combined text "is processed as at
// the start/end of such an inline block": a collapsible space there is gone,
// and the composition is its letters alone. The space in the middle stays.
func TestACompositionTrimsTheSpaceAtItsEnds(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all"> o o </span>o</div>`, "")
	var tcy *DrawText
	for i := range runs {
		if !runs[i].Sideways {
			tcy = &runs[i]
		}
	}
	if tcy == nil {
		t.Fatalf("no composition among %+v", runs)
	}
	if tcy.Text != "o o" {
		t.Errorf("the composition is %q, want \"o o\"", tcy.Text)
	}
	// And the space after the composition, outside it, is a space of its own:
	// the composition's trailing space is gone, so this one does not collapse
	// into it. "o", the em, and the fixture's space of 230 units, 4.6px, which
	// is 4.59375 in layout units.
	after := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">oo </span> o</div>`, "")
	last := after[len(after)-1]
	if got := last.At.Y.Px(); got != 30+4.59375 {
		t.Errorf("the letter after the composition and a space is at y=%g, want %g",
			got, 30+4.59375)
	}
}

// TestACompositionIsNeverCut.
//
// A line shorter than an em, and "overflow-wrap: anywhere" allowing a break
// between any two characters: the composition is one glyph, so it overflows
// the line whole rather than being cut into letters on two lines.
func TestACompositionIsNeverCut(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d"><span style="text-combine-upright: all">ooo</span></div>`,
		` #d { height: 15px; overflow-wrap: anywhere }`)
	if len(runs) != 1 || runs[0].Text != "ooo" || runs[0].Sideways {
		t.Errorf("drew %+v, want one composition of \"ooo\"", runs)
	}
}

// TestACompositionTakesItsCharactersBackFromFullWidth.
//
// §9.1.3.1: a composition of more than one character sets its full-width
// characters in the forms they were made from before it is compressed, so
// "text-transform: full-width" on a date does not squeeze two em-wide digits
// into one em. One character is left as it is.
func TestACompositionTakesItsCharactersBackFromFullWidth(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">&#xFF4F;&#xFF4F;</span>`+
			`o<span style="text-combine-upright: all">&#xFF4F;</span></div>`, "")
	var got []string
	for _, r := range runs {
		if !r.Sideways {
			got = append(got, r.Text)
		}
	}
	if len(got) != 2 || got[0] != "oo" || got[1] != "\uFF4F" {
		t.Errorf("the compositions drew %q, want [\"oo\" \"\\uFF4F\"]", got)
	}
	if paragraph.FromFullWidth("\uFF12\uFF10\u3000\uFF21") != "20 A" {
		t.Errorf("FromFullWidth(%q) = %q, want \"20 A\"", "\uFF12\uFF10\u3000\uFF21",
			paragraph.FromFullWidth("\uFF12\uFF10\u3000\uFF21"))
	}
}

// TestACompositionOnlyAppliesOnAVerticalLine.
//
// §9.1 "only has an effect in vertical writing modes", and a sideways mode is
// not one. And "digits" is refused, for a box it is declared in: the turn is
// reported, not the value quietly dropped.
func TestACompositionOnlyAppliesOnAVerticalLine(t *testing.T) {
	for _, mode := range []string{"horizontal-tb", "sideways-rl", "sideways-lr"} {
		runs := combineRuns(t,
			`<div id="d">o<span style="text-combine-upright: all">ooo</span>o</div>`,
			` #d { writing-mode: `+mode+` }`)
		var text strings.Builder
		for _, r := range runs {
			text.WriteString(r.Text)
			if r.Sideways != (mode != "horizontal-tb") || r.Upright || r.WidthScale != 0 {
				t.Errorf("%s: the run %q is sideways=%v upright=%v squeezed %v; want "+
					"it set as the line is", mode, r.Text, r.Sideways, r.Upright, r.WidthScale)
			}
		}
		// And the letters follow one another at their own advances: the last
		// is 40 along, four letters of 10, and not 30, which a one-em
		// composition would have put it at.
		last := runs[len(runs)-1]
		along := last.At.X.Px()
		switch mode {
		case "sideways-rl":
			along = last.At.Y.Px()
		case "sideways-lr":
			along = 400 - last.At.Y.Px()
		}
		if text.String() != "ooooo" || along != 40 {
			t.Errorf("%s: drew %q with the last letter %g along the line, want "+
				"\"ooooo\" and 40", mode, text.String(), along)
		}
	}
	var said string
	for _, f := range findingsOf(t,
		`<div id="d">o<span style="text-combine-upright: digits 2">12</span></div>`,
		`#d { writing-mode: vertical-rl; width: 60px; height: 100px }`) {
		if f.Property == "writing-mode" {
			said = f.Message
		}
	}
	if !strings.Contains(said, "digits") {
		t.Errorf("\"text-combine-upright: digits 2\" was reported as %q", said)
	}
}

// TestACompositionInTwoFacesIsReported.
//
// A composition is one run, in one face. Where the fallback stack sets part of
// its text in another, it is laid out as though it had not asked, and the page
// says so. The box is Courier and the Thai letter is the fixture face's.
func TestACompositionInTwoFacesIsReported(t *testing.T) {
	courier, _ := StandardFonts().Face("Courier", false, false)
	fixture := uprightFace(t, "VerticalFallbacks.ttf")
	set := fallbackSet{namedFaceSet{family: "C", face: courier, standard: StandardFonts()}, fixture}
	const css = `body { margin: 0 }
	#d { font-family: C; font-size: 20px; line-height: normal;
	     writing-mode: vertical-rl; width: 50px; height: 400px }`
	html := `<div id="d">a<span style="text-combine-upright: all">a&#x0E01;</span>a</div>`
	for _, r := range mixedRuns(t, set, html, css) {
		if !r.Sideways {
			t.Errorf("the run %q was drawn as a composition", r.Text)
		}
	}
	built := Build(Input{HTML: html, CSS: []Stylesheet{{Source: noDefaults + css}}})
	rec := NewRecorder(nil)
	Layout(built.Root, Size{W: 40000, H: 100000}, set, rec)
	var said string
	for _, f := range rec.Findings() {
		if f.Property == "text-combine-upright" {
			said = f.Message
		}
	}
	if !strings.Contains(said, "more than one face") {
		t.Errorf("a composition in two faces was reported as %q", said)
	}
}

// TestACompositionUsesTheWidthVariantThatFitsIt.
//
// §9.1.3: "OpenType implementations must use width-specific variants ... in
// cases where those variants are available for all typographic character
// units in the composition". Noto Sans JP has 'hwid', and sets both of "12"
// in it, so two digits are drawn with the feature and need no squeezing:
// half-width digits are two halves of an em. It has no 'twid', so three are
// set in their own widths and squeezed.
func TestACompositionUsesTheWidthVariantThatFitsIt(t *testing.T) {
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS to the fonts-noto corpus")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansJP-VF.ttf"))
	if err != nil {
		t.Fatalf("reading Noto Sans JP from the corpus: %v", err)
	}
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "J", face: face, standard: StandardFonts()}
	for _, c := range []struct {
		text    string
		tag     string
		squeeze bool
	}{
		{"12", "hwid", false},
		{"123", "", true},
		// Two units, and the face's hwid does not cover the ideograph: the
		// variant is for all of them or none, so none, and the two are
		// squeezed.
		{"1\u56fd", "", true},
	} {
		var tcy *DrawText
		runs := mixedRuns(t, set,
			`<div id="d">国<span style="text-combine-upright: all">`+c.text+`</span>国</div>`,
			`body { margin: 0 }
	#d { font-family: J; font-size: 20px; writing-mode: vertical-rl;
	     width: 50px; height: 400px }`)
		for i := range runs {
			if !runs[i].Sideways {
				tcy = &runs[i]
			}
		}
		if tcy == nil {
			t.Fatalf("%q: no composition among %+v", c.text, runs)
		}
		if tcy.Features.Tags != c.tag {
			t.Errorf("%q: the composition is set with tags %q, want %q", c.text,
				tcy.Features.Tags, c.tag)
		}
		if squeezed := tcy.WidthScale > 0; squeezed != c.squeeze {
			t.Errorf("%q: squeezed by %v, want squeezed=%v", c.text, tcy.WidthScale, c.squeeze)
		}
		// And what a backend draws is the em: the glyphs ShapedGlyphs gives it,
		// with the run's features, their advances squeezed as it is told.
		glyphs, _ := ShapedGlyphs(*tcy)
		across := 0.0
		for _, g := range glyphs {
			across += g.XAdvance
		}
		across *= tcy.Size.Px() / 1000
		if tcy.WidthScale > 0 {
			across *= tcy.WidthScale
		}
		// Within a sixty-fourth: the squeeze is the em over the width layout
		// measured, which is quantized to a layout unit, and this sum is not.
		if across > 20+1.0/64 || across < 20-1.0/64 {
			t.Errorf("%q: a backend draws the composition %gpx across, want the em, 20",
				c.text, across)
		}
	}
}

// TestACompositionOfAnIdeographIsNotSpacedEither.
//
// The composition's own first character is an ideograph here, and the letter
// in front of it lies along the line: were the composition its text, that
// would be the boundary §8.4.1 spaces. It is U+FFFC, so it is not. The
// ordinary span beside it is the control.
func TestACompositionOfAnIdeographIsNotSpacedEither(t *testing.T) {
	combined := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">日o</span></div>`, "")
	plain := combineRuns(t, `<div id="d">o<span>日o</span></div>`, "")
	if len(combined) < 2 || len(plain) < 2 {
		t.Fatalf("the lines drew %+v and %+v", combined, plain)
	}
	if got := combined[1].At.Y.Px(); got < 10 || got > 30 {
		t.Fatalf("the composition's square does not start at 10: its text is at y=%g", got)
	}
	// The composition's text is centred in a square that begins where the
	// letter ends, 10 down; with a gap in front it would begin at 12.5.
	sq := combined[1].At.Y.Sub(combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">日o</span></div>`,
		` #d { text-autospace: no-autospace }`)[1].At.Y).Px()
	if sq != 0 {
		t.Errorf("the composition moved %gpx when autospace was turned off; there "+
			"is no gap in front of it to take away", sq)
	}
	if got := plain[1].At.Y.Px(); got != 12.5 {
		t.Errorf("the control's ideograph is at y=%g, want 12.5 — spaced from the "+
			"letter before it", got)
	}
}

// TestACompositionIsNotShapedWithItsNeighbours is sameShaping asked directly:
// two runs alike in everything, both upright, one of them a composition. The
// first two tests of this file cannot see it — a composition's neighbour is
// either lying along the line, which the orientation rule already refuses, or
// standing upright in a face whose upright advances an em whatever its context.
func TestACompositionIsNotShapedWithItsNeighbours(t *testing.T) {
	a := inlineItem{Text: "o", Upright: true}
	b := a
	if !sameShaping(a, b) {
		t.Fatal("the control: two upright runs alike in everything do not shape together")
	}
	b.Combine = true
	if sameShaping(a, b) || sameShaping(b, a) {
		t.Error("an upright run and a composition beside it shape together")
	}
}

// TestACompositionIsOneBidiCharacter.
//
// §9.1.2 bidi-isolates the composition's text and reorders it as one upright
// character, so a composition holding a Latin letter and a Hebrew one is one
// composition and not two cut where the level changes.
func TestACompositionIsOneBidiCharacter(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">o&#x05D0;</span>o</div>`, "")
	n := 0
	for _, r := range runs {
		if !r.Sideways {
			n++
		}
	}
	if n != 1 {
		t.Errorf("drew %d compositions, want 1: %+v", n, runs)
	}
}

// TestACompositionIsAsTallAsItsSquare.
//
// The square is an em across the line and it is on the line: "similar to the
// contents of an inline-block box ... with a line-height of 1em". Under a
// 10px line-height the box's own text reaches 12.5 above its baseline and
// −2.5 below; the square, an em centred on the central baseline 7.5 above
// it, reaches 17.5 and 2.5. So the first line is 20 wide, its baseline 17.5 in
// from the right edge; the second starts 20 in and has its baseline 12.5
// further. Without the square the lines would be 10 apart.
func TestACompositionIsAsTallAsItsSquare(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">oo</span><br>o</div>`,
		` #d { line-height: 10px }`)
	if len(runs) != 3 {
		t.Fatalf("drew %d runs, want 3: %+v", len(runs), runs)
	}
	if got := runs[0].At.X.Px(); got != 50-17.5 {
		t.Errorf("the first line's baseline is at x=%g, want %g", got, 50-17.5)
	}
	if got := runs[2].At.X.Px(); got != 50-32.5 {
		t.Errorf("the second line's baseline is at x=%g, want %g", got, 50-32.5)
	}
}

// TestACompositionIsOneUnitToJustify.
//
// Inter-character justification puts its slack between typographic character
// units, and a composition is one. "o", a composition of three and "oo" are
// four units, three opportunities; a 110px line holds 50px of them, so each
// opportunity is 20 and the "oo" starts at 10 + 20 + 20 + 20 = 70. Counted as
// its three letters the composition would make six units and five
// opportunities of 12, and the "oo" would start at 78.
func TestACompositionIsOneUnitToJustify(t *testing.T) {
	runs := combineRuns(t,
		`<div id="d">o<span style="text-combine-upright: all">ooo</span>oo</div>`,
		` #d { height: 110px; text-align: justify; text-align-last: justify;
		       text-justify: inter-character }`)
	var last *DrawText
	for i := range runs {
		if runs[i].Sideways && runs[i].At.Y.Px() > 30 {
			last = &runs[i]
			break
		}
	}
	if last == nil {
		t.Fatalf("no run after the composition among %+v", runs)
	}
	if got := last.At.Y.Px(); got != 70 {
		t.Errorf("the letters after the composition start at y=%g, want 70", got)
	}
}

// TestACompositionIsNotCutForItsPunctuation.
//
// The rules that cut one character off the end of a run — a stop that hangs
// past the line's end, a closing bracket whose blank half is trimmed there —
// and the one that hangs a mark off the line's start, would each cut a
// composition in two if they read its text. It is one glyph, U+FFFC, which is
// none of those, so each leaves it whole.
func TestACompositionIsNotCutForItsPunctuation(t *testing.T) {
	for _, c := range []struct{ css, text string }{
		{"hanging-punctuation: allow-end", "o。"},
		{"hanging-punctuation: last", "o。"},
		{"hanging-punctuation: first", "「o"},
		{"text-spacing-trim: trim-both", "o）"},
	} {
		// At the end of the line, and for "first" at its start.
		html := `<div id="d">o<span style="text-combine-upright: all">` + c.text + `</span></div>`
		if strings.HasSuffix(c.css, "first") {
			html = `<div id="d"><span style="text-combine-upright: all">` + c.text + `</span>o</div>`
		}
		runs := combineRuns(t, html, ` #d { `+c.css+` }`)
		var got []string
		for _, r := range runs {
			if !r.Sideways {
				got = append(got, r.Text)
			}
		}
		// Two characters, so their full-width forms are taken back (§9.1.3.1):
		// the ideographic stop becomes the halfwidth one, and so on.
		if want := paragraph.FromFullWidth(c.text); len(got) != 1 || got[0] != want {
			t.Errorf("under %q the compositions are %q, want one of %q", c.css, got, want)
		}
	}
}
