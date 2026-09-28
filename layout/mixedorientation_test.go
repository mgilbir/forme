package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Two orientations on one vertical line, and where across the line each run
// sits.
//
// The face is VerticalFallbacks.ttf, which is committed and states the numbers
// every assertion below is made of: an em of 1000 units, a hhea ascent of 1000
// and descent of 250, no vertical metrics, and horizontal advances of 500 for
// "o" and "x". At 20px that is an ascent of 20 and a descent of 5, so its
// central baseline — halfway between them — is 7.5 above the alphabetic one;
// "o" and "x" advance 10 lying along the line; and a character standing upright
// advances CSS Writing Modes §4.4's synthesized em, 20. A 25px line holds the
// face with no leading, so its alphabetic baseline is 20 in from the line's
// over edge and its central one is the line's middle, 12.5 in.
//
// The fixture never puts "o" and "x" side by side in one run: the face kerns
// that pair, and the numbers are about orientation and not about kerning.
//
// The reftests cannot see any of this. Both halves of a reftest go through the
// same turn and the same baseline, so a run drawn on the wrong one agrees with
// its reference; see layout/writingmode.go.

const mixedCSS = `body { margin: 0 }
	#d { font-family: V; font-size: 20px; line-height: 25px;
	     writing-mode: vertical-rl; width: 50px; height: 400px }`

// mixedRuns lays a document out in the fixture face and returns its runs.
func mixedRuns(t *testing.T, set FontSet, htmlSrc, cssSrc string) []DrawText {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(1000)
	var out []DrawText
	for _, op := range Paint(Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil))) {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) != "" {
			out = append(out, v)
		}
	}
	return out
}

func verticalFallbacksSet(t *testing.T) FontSet {
	t.Helper()
	return namedFaceSet{family: "V", face: uprightFace(t, "VerticalFallbacks.ttf"),
		standard: StandardFonts()}
}

// TestAMixedLineSetsEachRunTheWayItFaces.
//
// A span asking for "upright" inside a box in the initial orientation: one
// line, three runs, the middle one standing. Along the line the runs follow
// each other by what each measures — two letters lying down are 20, two
// standing are 40 — and across it the lying runs sit on the alphabetic
// baseline, 20 in from the right edge, while the standing one is hung from the
// line's middle, 12.5 in: §4.2 makes the central baseline the dominant one
// here, and an upright glyph's own middle is what goes on it.
func TestAMixedLineSetsEachRunTheWayItFaces(t *testing.T) {
	runs := mixedRuns(t, verticalFallbacksSet(t),
		`<div id="d">oo<span style="text-orientation: upright">xx</span>oo</div>`, mixedCSS)
	if len(runs) != 3 {
		t.Fatalf("the line drew %d runs, want 3: %+v", len(runs), runs)
	}
	want := []struct {
		text    string
		upright bool
		x, y    float64
	}{
		{"oo", false, 30, 0},
		{"xx", true, 37.5, 20},
		{"oo", false, 30, 60},
	}
	for i, w := range want {
		r := runs[i]
		if r.Text != w.text || r.Upright != w.upright || !r.Sideways {
			t.Errorf("run %d is %q sideways=%v upright=%v, want %q upright=%v on a "+
				"turned line", i, r.Text, r.Sideways, r.Upright, w.text, w.upright)
		}
		if r.At.X.Px() != w.x || r.At.Y.Px() != w.y {
			t.Errorf("run %d (%q) is drawn from (%g, %g), want (%g, %g)", i, r.Text,
				r.At.X.Px(), r.At.Y.Px(), w.x, w.y)
		}
	}
	// And the standing run's ink is the em box centred on the line: 25 to 45
	// across a line box that runs from 25 to 50, which is its middle ± 10.
	ink := textInk(runs[1])
	if ink.X.Px() != 27.5 || ink.W.Px() != 20 || ink.Y.Px() != 20 || ink.H.Px() != 40 {
		t.Errorf("the standing run inks %+v in px (%g,%g %gx%g), want (27.5,20 20x40) — "+
			"two ems down the line, one across it, centred on the line's middle",
			ink, ink.X.Px(), ink.Y.Px(), ink.W.Px(), ink.H.Px())
	}
}

// TestUAX50DecidesEachCharacterUnderMixed is the same line without the span:
// the text alone decides. An ideograph stands and a Latin letter lies down,
// so one text box is cut into three runs where the answer changes.
//
// The face has no glyph for the ideograph and sets .notdef for it, which does
// not change what it measures to standing up: an em, as for any character in a
// face that states no vertical metrics.
//
// text-autospace is at its initial value, and CSS Text 4 §8.4.1 spaces an
// ideograph from a non-ideographic letter lying along the line: an eighth of
// the ideograph's em, 2.5, either side of it. It excludes only a letter that
// stands upright, so it is the letter's orientation that is asked and not the
// ideograph's — which stands under mixed, as ideographs do. With
// "no-autospace" the same three runs follow each other with nothing between.
func TestUAX50DecidesEachCharacterUnderMixed(t *testing.T) {
	for _, c := range []struct {
		autospace string
		y         [3]float64
	}{
		{"normal", [3]float64{0, 22.5, 45}},
		{"no-autospace", [3]float64{0, 20, 40}},
	} {
		runs := mixedRuns(t, verticalFallbacksSet(t), `<div id="d">oo日oo</div>`,
			mixedCSS+` #d { text-autospace: `+c.autospace+` }`)
		if len(runs) != 3 {
			t.Fatalf("%s: the line drew %d runs, want 3: %+v", c.autospace, len(runs), runs)
		}
		for i, w := range []struct {
			text    string
			upright bool
		}{{"oo", false}, {"日", true}, {"oo", false}} {
			if r := runs[i]; r.Text != w.text || r.Upright != w.upright || r.At.Y.Px() != c.y[i] {
				t.Errorf("%s: run %d is %q upright=%v at y=%g, want %q upright=%v at y=%g",
					c.autospace, i, r.Text, r.Upright, r.At.Y.Px(), w.text, w.upright, c.y[i])
			}
		}
	}
	// And where nothing else cuts the text. Between the ideographs and the
	// letters above, a line may break and text-autospace may open a gap, and
	// either cuts the text into runs of its own; a bracket holds on to what it
	// encloses (UAX #14's OP and CL), so "(日)" is one piece of text with two
	// orientations in it. The face has no brackets and sets .notdef for them,
	// which lies along the line at its advance of 500 units, 10px.
	bracketed := mixedRuns(t, verticalFallbacksSet(t), `<div id="d">(日)</div>`, mixedCSS)
	if len(bracketed) != 3 {
		t.Fatalf("\"(日)\" drew %d runs, want 3: %+v", len(bracketed), bracketed)
	}
	for i, w := range []struct {
		text    string
		upright bool
		y       float64
	}{{"(", false, 0}, {"日", true, 10}, {")", false, 30}} {
		if r := bracketed[i]; r.Text != w.text || r.Upright != w.upright || r.At.Y.Px() != w.y {
			t.Errorf("run %d is %q upright=%v at y=%g, want %q upright=%v at y=%g",
				i, r.Text, r.Upright, r.At.Y.Px(), w.text, w.upright, w.y)
		}
	}
	// "text-orientation: sideways" lays the ideograph down with the rest.
	for _, r := range mixedRuns(t, verticalFallbacksSet(t), `<div id="d">oo日oo</div>`,
		mixedCSS+` #d { text-orientation: sideways }`) {
		if r.Upright {
			t.Errorf("under sideways the run %q stands upright", r.Text)
		}
	}
}

// TestAnUprightLineIsCentredOnItsColumn.
//
// The whole line upright, in both vertical modes. The standing glyphs are hung
// from the line's middle — 12.5 from its over edge, which is its right in
// vertical-rl and, because vertical-lr turns its glyphs clockwise too, its
// right in vertical-lr as well. They used to be hung from the alphabetic
// baseline, 20 in, which put every glyph of an upright column 7.5px off the
// middle of its line towards the under side.
func TestAnUprightLineIsCentredOnItsColumn(t *testing.T) {
	for _, c := range []struct {
		mode string
		x    float64
	}{
		// The first line box is 25..50 in vertical-rl and 0..25 in
		// vertical-lr; its middle is 37.5 and 12.5.
		{"vertical-rl", 37.5},
		{"vertical-lr", 12.5},
	} {
		runs := mixedRuns(t, verticalFallbacksSet(t), `<div id="d">xx</div>`,
			mixedCSS+` #d { text-orientation: upright; writing-mode: `+c.mode+` }`)
		if len(runs) != 1 || !runs[0].Upright {
			t.Fatalf("%s drew %+v, want one upright run", c.mode, runs)
		}
		if got := runs[0].At.X.Px(); got != c.x {
			t.Errorf("%s: the upright run is hung from x=%g, want %g — the middle of "+
				"its line", c.mode, got, c.x)
		}
	}
}

// TestAnOrientationChangeIsNotShapedAcross.
//
// The face kerns "o" before "x" by 80 units, 1.6px at 20px, and a pair is
// normally kerned across a span boundary: the two runs are one font at one
// size. Not when one of them stands up. The upright run is shaped top to
// bottom and the lying one left to right, so neither is the other's context,
// and the "o" keeps its whole 10px advance.
func TestAnOrientationChangeIsNotShapedAcross(t *testing.T) {
	runs := mixedRuns(t, verticalFallbacksSet(t),
		`<div id="d">o<span style="text-orientation: upright">x</span></div>`, mixedCSS)
	if len(runs) != 2 {
		t.Fatalf("the line drew %d runs, want 2", len(runs))
	}
	if got := runs[1].At.Y.Px(); got != 10 {
		t.Errorf("the standing \"x\" starts %gpx down the line, want 10 — the "+
			"lying \"o\" was kerned against a glyph shaped another way", got)
	}
	// The control: the same boundary lying down both sides is kerned, which is
	// what says the fixture's pair is there to be lost.
	plain := mixedRuns(t, verticalFallbacksSet(t),
		`<div id="d">o<span style="color: red">x</span></div>`, mixedCSS)
	if len(plain) != 2 || plain[1].At.Y.Px() != 8.390625 {
		t.Errorf("the control drew %+v, want \"x\" at 8.390625 — ten less the "+
			"pair's 1.6, in 64ths", plain)
	}
}

// fallbackSet is VerticalFallbacks as the family "V" and Courier as the face
// for whatever it cannot set.
type fallbackSet struct {
	namedFaceSet
	fallback *shape.Face
}

func (s fallbackSet) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	return s.fallback, true
}

// TestAFallbackFaceIsCentredOnTheBoxsCentralBaseline.
//
// CSS Writing Modes §4.4 aligns the glyphs of two fonts in one box by their
// dominant baselines, and on a vertical line that is the central one. The box
// is Courier, which cannot set the Thai letter; the fixture face can, and sets
// it lying along the line (UAX #50 gives Thai R).
//
// Courier states an ascent of 805 and a descent of 250 — 16.09375 and 5 at
// 20px, the ascent quantized to a 64th — so its central baseline is 5.546875
// above its alphabetic one. The fixture's is 7.5 above its own. Matching the
// two puts the fixture's alphabetic baseline 1.953125 below Courier's, and its
// extents — 20 up and 5 down from there — reach 18.046875 above Courier's
// baseline and 6.953125 below. That is the tallest thing on the line, so the
// line box's over edge is 18.046875 above Courier's baseline: Courier is drawn
// that far in from the right, and the Thai letter 20 in.
//
// On the alphabetic baseline both would be 20 in.
func TestAFallbackFaceIsCentredOnTheBoxsCentralBaseline(t *testing.T) {
	courier, _ := StandardFonts().Face("Courier", false, false)
	fixture := uprightFace(t, "VerticalFallbacks.ttf")
	// Courier is the box's face, as family "C", and the fixture sets what it
	// cannot.
	set := fallbackSet{namedFaceSet{family: "C", face: courier, standard: StandardFonts()}, fixture}
	runs := mixedRuns(t, set, `<div id="d">a&#x0E01;a</div>`,
		`body { margin: 0 }
	#d { font-family: C; font-size: 20px; line-height: normal;
	     writing-mode: vertical-rl; width: 50px; height: 400px }`)
	if len(runs) != 3 {
		t.Fatalf("the line drew %d runs, want 3: %+v", len(runs), runs)
	}
	for i, want := range []struct {
		face *shape.Face
		x    float64
	}{{courier, 50 - 18.046875}, {fixture, 30}, {courier, 50 - 18.046875}} {
		r := runs[i]
		if r.Face != want.face || r.Upright {
			t.Fatalf("run %d is %q in %v upright=%v, want it lying along the line in "+
				"the other face", i, r.Text, r.Face, r.Upright)
		}
		if got := r.At.X.Px(); got != want.x {
			t.Errorf("run %d (%q) is drawn from x=%g, want %g", i, r.Text, got, want.x)
		}
	}
	// Where the alphabetic baseline is the dominant one — §4.2 says so under
	// "text-orientation: sideways", and a sideways mode is a horizontal
	// typographic mode — the three runs share it: the fixture's ascent of 20
	// sets the line's over edge and both faces are drawn 20 in.
	for _, decl := range []string{"text-orientation: sideways", "writing-mode: sideways-rl"} {
		for _, r := range mixedRuns(t, set, `<div id="d">a&#x0E01;a</div>`,
			`body { margin: 0 }
	#d { font-family: C; font-size: 20px; line-height: normal;
	     writing-mode: vertical-rl; width: 50px; height: 400px; `+decl+` }`) {
			if got := r.At.X.Px(); got != 30 {
				t.Errorf("under %q the run %q is drawn from x=%g, want 30 — on the "+
					"alphabetic baseline", decl, r.Text, got)
			}
		}
	}
	// And on a horizontal line nothing moves: the same three runs share one
	// alphabetic baseline, as they always have.
	flat := mixedRuns(t, set, `<div id="d">a&#x0E01;a</div>`,
		`body { margin: 0 } #d { font-family: C; font-size: 20px; line-height: normal }`)
	for _, r := range flat {
		if r.At.Y != flat[0].At.Y {
			t.Errorf("on a horizontal line %q is on y=%g and the first run on %g",
				r.Text, r.At.Y.Px(), flat[0].At.Y.Px())
		}
	}
}

// TestAnUprightFallbackRunIsCentredOnItsLine is the other half of the case
// above: a character the box's face cannot set, standing upright. The box is
// Courier again and the fixture sets the ideograph, as .notdef. Its extents
// are the fixture's centred on Courier's central baseline, so the line box is
// the fixture's 25, from 18.046875 above Courier's baseline to 6.953125 below
// it — and the upright glyph is hung from that line's middle, 12.5 in from the
// right edge, where the Latin on either side of it is 18.046875 in.
func TestAnUprightFallbackRunIsCentredOnItsLine(t *testing.T) {
	courier, _ := StandardFonts().Face("Courier", false, false)
	fixture := uprightFace(t, "VerticalFallbacks.ttf")
	set := fallbackSet{namedFaceSet{family: "C", face: courier, standard: StandardFonts()}, fixture}
	runs := mixedRuns(t, set, `<div id="d">a日a</div>`,
		`body { margin: 0 }
	#d { font-family: C; font-size: 20px; line-height: normal;
	     writing-mode: vertical-rl; width: 50px; height: 400px }`)
	if len(runs) != 3 {
		t.Fatalf("the line drew %d runs, want 3: %+v", len(runs), runs)
	}
	for i, want := range []struct {
		upright bool
		x       float64
	}{{false, 50 - 18.046875}, {true, 37.5}, {false, 50 - 18.046875}} {
		r := runs[i]
		if r.Upright != want.upright {
			t.Fatalf("run %d (%q) upright=%v, want %v", i, r.Text, r.Upright, want.upright)
		}
		if got := r.At.X.Px(); got != want.x {
			t.Errorf("run %d (%q) is drawn from x=%g, want %g", i, r.Text, got, want.x)
		}
	}
}

// TestAHyphenInAFallbackFaceIsCentredLikeARun.
//
// The hyphen a line ends with is measured apart from the word it follows, in
// the face that has its glyph, and that face can be another than the box's.
// Here the box is Courier and the hyphenate-character is the Thai letter only
// the fixture sets, so the hyphen at the end of the first line is a fallback
// run of its own: its extents move with it, and the line's over edge is the
// fixture's, 18.046875 above Courier's baseline, as in the tests above. The
// hyphen is drawn from its own alphabetic baseline 1.953125 below Courier's,
// which is 20 in.
//
// Four Courier letters are 48px and the line is 40, so the soft hyphen breaks
// the word after two.
func TestAHyphenInAFallbackFaceIsCentredLikeARun(t *testing.T) {
	courier, _ := StandardFonts().Face("Courier", false, false)
	fixture := uprightFace(t, "VerticalFallbacks.ttf")
	set := fallbackSet{namedFaceSet{family: "C", face: courier, standard: StandardFonts()}, fixture}
	runs := mixedRuns(t, set, `<div id="d">aa&shy;aa</div>`,
		`body { margin: 0 }
	#d { font-family: C; font-size: 20px; line-height: normal;
	     hyphenate-character: "\0E01";
	     writing-mode: vertical-rl; width: 50px; height: 40px }`)
	if len(runs) < 3 {
		t.Fatalf("the fixture drew %d runs, want the first line's word, its hyphen "+
			"and the second line's word: %+v", len(runs), runs)
	}
	word, hyphen := runs[0], runs[1]
	if strings.TrimSuffix(word.Text, "\u00ad") != "aa" || hyphen.Face != fixture {
		t.Fatalf("the first line drew %q and then %q; want \"aa\" and a hyphen "+
			"in the fixture face", word.Text, hyphen.Text)
	}
	if got := word.At.X.Px(); got != 50-18.046875 {
		t.Errorf("the word is drawn from x=%g, want %g — the hyphen's extents set "+
			"the line's over edge", got, 50-18.046875)
	}
	if got := hyphen.At.X.Px(); got != 30 {
		t.Errorf("the hyphen is drawn from x=%g, want 30", got)
	}
}
