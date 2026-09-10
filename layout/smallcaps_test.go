package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4 §6.6's small capitals: the one font-variant value this engine
// sets, and the first request in the whole of fontfeatures.go that goes the
// other way — it asks a face for a rule rather than taking one away.
//
// That difference is the whole of what these tests are about. Turning a rule off
// is right whether or not the face has it, so nothing about the face has to be
// asked; turning one on can only be done by a face that declares it, and a face
// that does not sets the text in ordinary letters at ordinary size. A page that
// came out that way is not visibly wrong — a paragraph meant to be in small
// capitals reads perfectly well in lowercase — which is exactly the failure
// §6.3's findings exist for.
//
// The fixtures need both kinds of face and both are to hand: the bundled Noto
// Sans declares 'smcp', and the fourteen standard PDF faces declare no
// OpenType features at all.

// namedFaceSet answers one family by name and leaves the rest to the standard
// fourteen, which is how a document gets a face with small capitals in it
// without needing the fetched font library.
type namedFaceSet struct {
	family   string
	face     *shape.Face
	standard FontSet
}

func (s namedFaceSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if strings.EqualFold(strings.TrimSpace(family), s.family) {
		return s.face, true
	}
	return s.standard.Face(family, bold, italic)
}

// smallCapsFontSet is a set whose "Cap" family has small capitals in it.
func smallCapsFontSet(t *testing.T) FontSet {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	if !faceHasSmallCaps(face) {
		t.Fatalf("the embedded face declares %v and not smcp; every fixture "+
			"below would be asking for something no face here has",
			face.Features())
	}
	return namedFaceSet{family: "Cap", face: face, standard: StandardFonts()}
}

// TestSmallCapsReachesTheRunThatIsDrawn is the plumbing, end to end.
//
// The property is read where the box is and the answer has to travel to the
// backend, because the backend shapes the run for itself: a run measured with
// small capitals and drawn without them is a line filled to one width and
// painted at another. That is the same argument the ligature and kerning flags
// are carried for, which is why it is the same field.
func TestSmallCapsReachesTheRunThatIsDrawn(t *testing.T) {
	for _, c := range []struct {
		what, css string
		want      bool
	}{
		{"nothing declared", "", false},
		{"the longhand", "font-variant-caps: small-caps", true},
		{"the font-variant shorthand", "font-variant: small-caps", true},
		{"the font shorthand", "font: small-caps 20px serif", true},
		{"normal", "font-variant-caps: normal", false},
		{"a caps value this engine does not set", "font-variant-caps: unicase", false},
	} {
		var got shape.Features
		var found bool
		for _, op := range Paint(layoutOf(t, 400,
			`<div id="d" style="`+c.css+`">office</div>`,
			`body{margin:0} #d{font-size:20px}`)) {
			if v, ok := op.(DrawText); ok && strings.Contains(v.Text, "off") {
				got, found = v.Features, true
			}
		}
		if !found {
			t.Fatalf("%s: the fixture drew no run", c.what)
		}
		if got.SmallCaps != c.want {
			t.Errorf("%s: the run carries SmallCaps=%v, want %v",
				c.what, got.SmallCaps, c.want)
		}
	}
}

// TestSmallCapsIsInheritedThroughTheInlineBoxThatCarriesTheText.
//
// The property is read off the box the text is in, and that box is a span or an
// anonymous box rather than the element the rule matched. It works because the
// property inherits — which is a fact about the registry, and the thing that
// would silently stop this working if it were ever changed there.
func TestSmallCapsIsInheritedThroughTheInlineBoxThatCarriesTheText(t *testing.T) {
	var runs int
	for _, op := range Paint(layoutOf(t, 400,
		`<div id="d">plain <span id="s">inner</span></div>`,
		`body{margin:0} #d{font-size:20px; font-variant: small-caps}`)) {
		v, ok := op.(DrawText)
		if !ok || strings.TrimSpace(v.Text) == "" {
			continue
		}
		runs++
		if !v.Features.SmallCaps {
			t.Errorf("the run %q was drawn without small capitals; the "+
				"declaration is on the container and the property inherits",
				v.Text)
		}
	}
	if runs < 2 {
		t.Fatalf("the fixture drew %d runs, want the container's text and the "+
			"span's; with one of them missing the inheritance is not tested", runs)
	}
}

// TestSmallCapsChangesTheGlyphsAndTheWidth is the feature actually applied.
//
// Carrying the flag to the backend proves nothing on its own: a run that is
// marked and then set identically is the property doing nothing with a page
// that says it did. So this asks the face — the same face, the same string, the
// flag the only difference — for the glyphs and for the width.
func TestSmallCapsChangesTheGlyphsAndTheWidth(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	plain, _ := face.ShapeGlyphsInContext("office", "", "", shape.Features{})
	small, _ := face.ShapeGlyphsInContext("office", "", "", shape.Features{SmallCaps: true})
	if len(small) != 6 {
		t.Errorf("small capitals set \"office\" as %d glyphs, want 6; the "+
			"ligature is stated over the lowercase letters and there are none "+
			"left by the time it is reached", len(small))
	}
	if shape.MeasureGlyphs(plain, 20) == shape.MeasureGlyphs(small, 20) {
		t.Error("the run is the same width either way; a small capital is not " +
			"the letter it replaces and the measurement has to follow the shaping")
	}
}

// TestSmallCapsIsReportedWhenTheFaceHasNone.
//
// The standard fourteen carry no OpenType features at all, so a document that
// asks them for small capitals gets ordinary letters. Nothing about the page
// says so, which is why the finding has to.
func TestSmallCapsIsReportedWhenTheFaceHasNone(t *testing.T) {
	_, findings := layoutWith(t, StandardFonts(),
		`<p id="p">Filler Text</p>`,
		`#p { font-family: Helvetica; font-size: 20px; font-variant: small-caps }`)
	f, ok := findingNaming(findings, "font-variant-caps")
	if !ok {
		t.Fatalf("nothing was reported: %v", findings)
	}
	if !f.Unsupported() {
		t.Error("the finding does not claim the engine is missing anything; " +
			"§7.1's companion signal counts on it, because two documents whose " +
			"small capitals both came out as lowercase match each other and " +
			"demonstrate nothing")
	}
	if !strings.Contains(f.Message, "ordinary letters") {
		t.Errorf("the message is %q; it has to say what the page came out as",
			f.Message)
	}
}

// TestSmallCapsIsNotReportedWhenTheFaceHasThem is the other half, and the half
// that decides whether the report is worth having.
//
// A finding raised whenever the property is written would be raised on every
// page where it worked, which is a report that tells an author nothing and
// holds a correct document out of the clean count for ever.
func TestSmallCapsIsNotReportedWhenTheFaceHasThem(t *testing.T) {
	_, findings := layoutWith(t, smallCapsFontSet(t),
		`<p id="p">Filler Text</p>`,
		`#p { font-family: Cap; font-size: 20px; font-variant: small-caps }`)
	if f, ok := findingNaming(findings, "font-variant-caps"); ok {
		t.Errorf("the face has small capitals and the page was reported anyway: %s",
			f.Message)
	}
}

// TestSmallCapsIsNotReportedWhereItWouldChangeNothing is the narrowing
// reportKerning makes for a "kern" a face has not got, one property along.
//
// Small capitals replace lowercase letters. A run of capitals, of digits or of
// a script with one case is set identically with the feature and without it, so
// a finding about it would be this engine calling a correct page a failure.
func TestSmallCapsIsNotReportedWhereItWouldChangeNothing(t *testing.T) {
	for _, text := range []string{"FILLER TEXT", "1234", "!?.,", "日本語"} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">`+text+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px; font-variant: small-caps }`)
		if f, ok := findingNaming(findings, "font-variant-caps"); ok {
			t.Errorf("%q has no lowercase letter in it and was reported anyway: %s",
				text, f.Message)
		}
	}
}

// TestACapsValueThisEngineDoesNotSetIsReported.
//
// Five of §6.6's six values are read, cascaded and not applied: "all-small-caps"
// wants 'c2sc' as well, the two petite values want a second set of capitals few
// faces draw, and "unicase" and "titling-caps" are neither. A document writing
// one of them gets its text as written, and would carry no claim that anything
// was missing from it.
func TestACapsValueThisEngineDoesNotSetIsReported(t *testing.T) {
	for _, value := range []string{
		"all-small-caps", "petite-caps", "all-petite-caps", "unicase", "titling-caps",
	} {
		// Through the font set that *has* small capitals, so what is being
		// reported is the value and not the face.
		_, findings := layoutWith(t, smallCapsFontSet(t),
			`<p id="p">Filler Text</p>`,
			`#p { font-family: Cap; font-size: 20px; font-variant-caps: `+value+` }`)
		f, ok := findingNaming(findings, "font-variant-caps")
		if !ok {
			t.Errorf("%q was not reported: %v", value, findings)
			continue
		}
		if !strings.Contains(f.Message, value) {
			t.Errorf("the finding for %q says %q and does not name the value",
				value, f.Message)
		}
		if !f.Unsupported() {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything", value)
		}
	}
}

// TestCapsOfReadsTheProperty is the reader on its own, including the values it
// refuses.
func TestCapsOfReadsTheProperty(t *testing.T) {
	for _, c := range []struct {
		raw       string
		want      caps
		unhandled string
	}{
		{"", capsNormal, ""},
		{"normal", capsNormal, ""},
		{"  Small-Caps  ", capsSmall, ""},
		{"all-small-caps", capsNormal, "all-small-caps"},
		{"titling-caps", capsNormal, "titling-caps"},
		{"nonsense", capsNormal, "nonsense"},
	} {
		got, unhandled := capsOf(c.raw)
		if got != c.want || unhandled != c.unhandled {
			t.Errorf("capsOf(%q) = %v, %q; want %v, %q",
				c.raw, got, unhandled, c.want, c.unhandled)
		}
	}
}

// TestSmallCapsNarrowsAMinContentBoxToTheCapitalItActuallySets.
//
// "overflow-wrap: anywhere" at "width: min-content" narrows a box to its widest
// grapheme cluster, and that width was measured without the features the item
// carries — the one width in the engine that was not. Nothing showed it, because
// the three flags a document turns *off* are inert over a single cluster: one
// cluster cannot ligate with itself and has no pair to kern.
//
// Small capitals are not inert. A small capital is not the letter it replaces
// and is not its width, so the box was narrowed to the wrong character and the
// text it was sized to hold overflowed it.
func TestSmallCapsNarrowsAMinContentBoxToTheCapitalItActuallySets(t *testing.T) {
	set := smallCapsFontSet(t)
	const doc = `<div id="d">mmmm</div>`
	const css = `body { margin: 0 }
		#d { font-family: Cap; font-size: 40px;
		     width: min-content; overflow-wrap: anywhere }`

	plain, _ := layoutWith(t, set, doc, css)
	small, _ := layoutWith(t, set, doc, css+`
		#d { font-variant: small-caps }`)
	wide := find(t, plain, "d").BorderRect.W
	narrow := find(t, small, "d").BorderRect.W
	if wide == 0 {
		t.Fatal("the plain box has no width; the fixture measures nothing")
	}
	if wide == narrow {
		t.Errorf("the box is %v wide either way; a small capital M is not an "+
			"m and the width the box is narrowed to has to follow the glyph "+
			"that will be set in it", wide)
	}
	// And the width is the one the run is actually set at, which is what makes
	// this a min-content width rather than a number that merely differs: one
	// cluster of a four-cluster run, so a quarter of what the whole run
	// measures.
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	glyphs, _ := face.ShapeGlyphsInContext("m", "", "", shape.Features{SmallCaps: true})
	want := shape.MeasureGlyphs(glyphs, 40)
	// Within one unit: a length here is a whole number of 64ths of a pixel, and
	// the face's advance is not one. See style.Unit.
	if got := narrow.Px(); math.Abs(got-want) > 1.0/64 {
		t.Errorf("the box narrowed to %gpx and a small capital m is %gpx wide",
			got, want)
	}
}

// TestNoGlyphIsSharedAcrossASmallCapsBoundary is the defect small capitals
// exposed in a rule that had been right by coincidence.
//
// §8.1's boundary between two inline elements does not break shaping, so
// "of<span>f</span>ice" is one word and the face's ffi ligature is what a reader
// of it expects. The runs of such a group are shaped as one string and cut by
// cluster — and one shaping applies one set of the font's rules. Two runs that
// disagree about those rules cannot be that shaping.
//
// Nothing showed it while every rule a document could state was a *suppression*:
// the two readings of the group agree everywhere the ligature is not. Small
// capitals add a glyph. The plain runs shaped "office" and formed the ffi
// ligature, the span shaped it with 'smcp' and formed none, and the page got the
// ligature *and* a small capital F — the letter drawn twice, in the middle of a
// word, with the run after it missing its "i".
func TestNoGlyphIsSharedAcrossASmallCapsBoundary(t *testing.T) {
	set := smallCapsFontSet(t)
	frag, _ := layoutWith(t, set,
		`<div id="d">of<span id="s">f</span>ice</div>`,
		`body{margin:0} #d{font-family:Cap; font-size:40px}
		 #s{font-variant: small-caps}`)

	var drawn string
	var runs int
	for _, op := range Paint(frag) {
		v, ok := op.(DrawText)
		if !ok || v.Text == "" {
			continue
		}
		runs++
		glyphs, _ := ShapedGlyphs(v)
		if len(glyphs) != len([]rune(v.Text)) {
			t.Errorf("the run %q was set as %d glyphs; a group shaped under one "+
				"run's rules and cut for another leaves a ligature in one piece "+
				"and a hole in the next", v.Text, len(glyphs))
		}
		drawn += v.Text
	}
	if runs != 3 {
		t.Fatalf("the fixture drew %d runs, want three; without the span as a "+
			"run of its own there is no boundary to test", runs)
	}
	if drawn != "office" {
		t.Errorf("the runs spell %q, want \"office\"", drawn)
	}
	// And the span really is set in small capitals, so what is being checked is
	// a boundary between two different sets of rules and not two runs that
	// happen to agree.
	var small bool
	for _, op := range Paint(frag) {
		if v, ok := op.(DrawText); ok && v.Text == "f" {
			small = v.Features.SmallCaps
		}
	}
	if !small {
		t.Error("the span was not drawn with small capitals, so the two sides " +
			"of the boundary ask for the same thing and the group may merge")
	}
}

// findingNaming returns the first finding about a property.
func findingNaming(findings []Finding, property string) (Finding, bool) {
	for _, f := range findings {
		if f.Property == property {
			return f, true
		}
	}
	return Finding{}, false
}
