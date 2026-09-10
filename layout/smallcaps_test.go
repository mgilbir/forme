package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
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
	for _, tag := range shape.CapsAllSmall.Features() {
		if !faceDeclares(face, tag) {
			t.Fatalf("the embedded face declares %v and not %s; the fixtures "+
				"below would be asking for something no face here has",
				face.Features(), tag)
		}
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
		want      shape.Caps
	}{
		{"nothing declared", "", shape.CapsNormal},
		{"the longhand", "font-variant-caps: small-caps", shape.CapsSmall},
		{"the font-variant shorthand", "font-variant: small-caps", shape.CapsSmall},
		{"the font shorthand", "font: small-caps 20px serif", shape.CapsSmall},
		{"normal", "font-variant-caps: normal", shape.CapsNormal},
		{"all-small-caps", "font-variant-caps: all-small-caps", shape.CapsAllSmall},
		{"petite-caps", "font-variant: petite-caps", shape.CapsPetite},
		{"all-petite-caps", "font-variant: all-petite-caps", shape.CapsAllPetite},
		{"unicase", "font-variant: unicase", shape.CapsUnicase},
		{"titling-caps", "font-variant: titling-caps", shape.CapsTitling},
		{"a value of no level at all", "font-variant-caps: sideways", shape.CapsNormal},
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
		if got.Caps != c.want {
			t.Errorf("%s: the run carries Caps=%v, want %v",
				c.what, got.Caps, c.want)
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
		if v.Features.Caps != shape.CapsSmall {
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
	small, _ := face.ShapeGlyphsInContext("office", "", "", shape.Features{Caps: shape.CapsSmall})
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

// TestCapsAreReportedWhenTheFaceHasNone.
//
// The standard fourteen carry no OpenType features at all, so a document that
// asks them for capitals of any kind gets the letters it wrote. Nothing about
// the page says so, which is why the finding has to.
func TestCapsAreReportedWhenTheFaceHasNone(t *testing.T) {
	for _, c := range []struct{ value, tags string }{
		{"small-caps", "smcp"},
		{"all-small-caps", "c2sc and smcp"},
		{"petite-caps", "pcap"},
		{"all-petite-caps", "c2pc and pcap"},
		{"unicase", "unic"},
		{"titling-caps", "titl"},
	} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">Filler Text</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-caps: `+c.value+` }`)
		f, ok := findingNaming(findings, "font-variant-caps")
		if !ok {
			t.Errorf("%q was not reported: %v", c.value, findings)
			continue
		}
		if !f.Unsupported() {
			t.Errorf("the finding for %q does not claim the engine is missing "+
				"anything; §7.1's companion signal counts on it, because two "+
				"documents whose capitals both came out as written match each "+
				"other and demonstrate nothing", c.value)
		}
		// The value and the features it needed, so that an author knows what to
		// look for in the face they chose rather than only that something is
		// wrong with it.
		if !strings.Contains(f.Message, c.value) {
			t.Errorf("the finding for %q says %q and does not name the value",
				c.value, f.Message)
		}
		if !strings.Contains(f.Message, c.tags) {
			t.Errorf("the finding for %q says %q and does not name %s",
				c.value, f.Message, c.tags)
		}
		if !strings.Contains(f.Message, "the letters it is written with") {
			t.Errorf("the message is %q; it has to say what the page came out as",
				f.Message)
		}
	}
}

// TestCapsAreNotReportedWhenTheFaceHasThem is the other half, and the half that
// decides whether the report is worth having.
//
// A finding raised whenever the property is written would be raised on every
// page where it worked, which is a report that tells an author nothing and
// holds a correct document out of the clean count for ever.
//
// Noto Sans declares 'smcp' and 'c2sc', so it can carry out both of the values
// that need only those — which is what makes "all-small-caps" a case worth
// having here and not only in the shaping tests.
func TestCapsAreNotReportedWhenTheFaceHasThem(t *testing.T) {
	for _, value := range []string{"normal", "small-caps", "all-small-caps"} {
		_, findings := layoutWith(t, smallCapsFontSet(t),
			`<p id="p">Filler Text</p>`,
			`#p { font-family: Cap; font-size: 20px; font-variant-caps: `+value+` }`)
		if f, ok := findingNaming(findings, "font-variant-caps"); ok {
			t.Errorf("the face declares what %q needs and the page was reported "+
				"anyway: %s", value, f.Message)
		}
	}
}

// TestHalfAnAnsweredRequestIsReportedAsHalf.
//
// "all-small-caps" is two features over two disjoint sets of letters, and a face
// may declare one of them. What comes out is the lowercase letters lowered and
// the capitals left standing — a line in two heights of letter, which is neither
// what was asked for nor obviously wrong to look at.
//
// So the finding names the tag that is missing rather than the value that failed,
// and says that *part* of the text was set as written. An author told only that
// "all-small-caps was not applied" would go looking for a page with no small
// capitals on it at all, and find one covered in them.
func TestHalfAnAnsweredRequestIsReportedAsHalf(t *testing.T) {
	// Noto Sans has both halves, so the half-answering face is one built for it:
	// 'smcp' declared, 'c2sc' not.
	face := halfCapsFace(t)
	set := namedFaceSet{family: "Half", face: face, standard: StandardFonts()}
	_, findings := layoutWith(t, set,
		`<p id="p">Filler Text</p>`,
		`#p { font-family: Half; font-size: 20px; font-variant-caps: all-small-caps }`)
	f, ok := findingNaming(findings, "font-variant-caps")
	if !ok {
		t.Fatalf("nothing was reported: %v", findings)
	}
	if strings.Contains(f.Message, "no c2sc or smcp") {
		t.Errorf("the finding is %q; the face declares smcp and carried out half "+
			"the request, so naming both is telling the author their small "+
			"capitals are absent when they are on the page", f.Message)
	}
	if !strings.Contains(f.Message, "no c2sc") {
		t.Errorf("the finding is %q and does not name c2sc, which is the half "+
			"the face has not got", f.Message)
	}
	if !strings.Contains(f.Message, "that part of the text") {
		t.Errorf("the finding is %q; a request half carried out is not a page "+
			"set entirely as written", f.Message)
	}
}

// TestCapsAreNotReportedWhereTheyWouldChangeNothing is the narrowing
// reportKerning makes for a "kern" a face has not got, one property along.
//
// Each of these features acts on one case or the other, so a run without a
// letter of that case is set identically with the feature and without it — and a
// finding about it would be this engine calling a correct page a failure.
func TestCapsAreNotReportedWhereTheyWouldChangeNothing(t *testing.T) {
	for _, c := range []struct{ what, text, value string }{
		{"no letters at all", "1234", "small-caps"},
		{"punctuation", "!?.,", "all-small-caps"},
		{"a script with one case", "日本語", "small-caps"},
		{"no lowercase for smcp", "FILLER TEXT", "small-caps"},
		{"no lowercase for pcap", "FILLER TEXT", "petite-caps"},
		{"no capitals for titl", "filler text", "titling-caps"},
	} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">`+c.text+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-caps: `+c.value+` }`)
		if f, ok := findingNaming(findings, "font-variant-caps"); ok {
			t.Errorf("%s: %q under %q was reported and is set identically "+
				"either way: %s", c.what, c.text, c.value, f.Message)
		}
	}
}

// TestOnlyTheHalfWithNothingToActOnIsLeftOut is the two narrowings meeting.
//
// "all-small-caps" over text in one case only: the tag for the other case has
// nothing to act on and is not named, and the tag for this one still is.
// Naming both would point at a feature whose absence changes nothing; naming
// neither would leave a page that really is wrong with no claim on it.
//
// Both directions, because a rule that looked at the wrong case would pass one
// of them.
func TestOnlyTheHalfWithNothingToActOnIsLeftOut(t *testing.T) {
	for _, c := range []struct{ text, named, notNamed string }{
		{"FILLER TEXT", "no c2sc", "smcp"},
		{"filler text", "no smcp", "c2sc"},
	} {
		_, findings := layoutWith(t, StandardFonts(),
			`<p id="p">`+c.text+`</p>`,
			`#p { font-family: Helvetica; font-size: 20px;
			      font-variant-caps: all-small-caps }`)
		f, ok := findingNaming(findings, "font-variant-caps")
		if !ok {
			t.Errorf("%q under all-small-caps was not reported at all: %v",
				c.text, findings)
			continue
		}
		if !strings.Contains(f.Message, c.named) {
			t.Errorf("the finding for %q is %q and does not say %q",
				c.text, f.Message, c.named)
		}
		// Named in the list of what is missing, which is the clause after
		// "declares"; the value's own features are listed before it and name
		// both tags whatever the text is.
		if _, after, found := strings.Cut(f.Message, "declares "); found &&
			strings.Contains(after, c.notNamed) {
			t.Errorf("the finding for %q is %q; %q has no letter for it to act "+
				"on, so its absence changes nothing about this page",
				c.text, f.Message, c.notNamed)
		}
	}
}

// TestAValueOfNoLevelAtAllIsReported.
//
// All six of §6.6 are read, so a value outside them is either a mistake the
// author made or a value from a level this engine has not read. It cannot tell
// the two apart and reports the second, which is the direction to err in.
func TestAValueOfNoLevelAtAllIsReported(t *testing.T) {
	_, findings := layoutWith(t, smallCapsFontSet(t),
		`<p id="p">Filler Text</p>`,
		`#p { font-family: Cap; font-size: 20px; font-variant-caps: sideways-caps }`)
	f, ok := findingNaming(findings, "font-variant-caps")
	if !ok {
		t.Fatalf("nothing was reported: %v", findings)
	}
	if !strings.Contains(f.Message, "sideways-caps") {
		t.Errorf("the finding is %q and does not name the value", f.Message)
	}
	if !f.Unsupported() {
		t.Error("the finding does not claim the engine is missing anything")
	}
}

// TestCapsOfReadsTheProperty is the reader on its own.
//
// All six of §6.6, and the features each of them asks a face for — which is the
// half a reader that merely told them apart would leave unchecked. Two values
// mapping to the same tag list is the mistake this catches, and it is one
// nothing else would: the page would come out right for one of them.
func TestCapsOfReadsTheProperty(t *testing.T) {
	for _, c := range []struct {
		raw       string
		want      shape.Caps
		features  []string
		unhandled string
	}{
		{raw: "", want: shape.CapsNormal},
		{raw: "normal", want: shape.CapsNormal},
		{raw: "  Small-Caps  ", want: shape.CapsSmall, features: []string{"smcp"}},
		{raw: "all-small-caps", want: shape.CapsAllSmall, features: []string{"c2sc", "smcp"}},
		{raw: "petite-caps", want: shape.CapsPetite, features: []string{"pcap"}},
		{raw: "all-petite-caps", want: shape.CapsAllPetite, features: []string{"c2pc", "pcap"}},
		{raw: "unicase", want: shape.CapsUnicase, features: []string{"unic"}},
		{raw: "titling-caps", want: shape.CapsTitling, features: []string{"titl"}},
		{raw: "nonsense", want: shape.CapsNormal, unhandled: "nonsense"},
	} {
		got, unhandled := capsOf(c.raw)
		if got != c.want || unhandled != c.unhandled {
			t.Errorf("capsOf(%q) = %v, %q; want %v, %q",
				c.raw, got, unhandled, c.want, c.unhandled)
			continue
		}
		if tags := got.Features(); !equalStrings(tags, c.features) {
			t.Errorf("%q asks a face for %v, want %v", c.raw, tags, c.features)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	glyphs, _ := face.ShapeGlyphsInContext("m", "", "", shape.Features{Caps: shape.CapsSmall})
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
	var small shape.Caps
	for _, op := range Paint(frag) {
		if v, ok := op.(DrawText); ok && v.Text == "f" {
			small = v.Features.Caps
		}
	}
	if small != shape.CapsSmall {
		t.Error("the span was not drawn with small capitals, so the two sides " +
			"of the boundary ask for the same thing and the group may merge")
	}
}

// halfCapsFace declares 'smcp' and not 'c2sc': a face that can carry out half of
// "all-small-caps".
//
// Built rather than fetched, because it is not a face anyone ships — a font with
// small capitals almost always has both halves, and Noto Sans does. What it
// stands for is real all the same: 'c2sc' is the later of the two additions and
// a subset that dropped it is exactly this font.
func halfCapsFace(t *testing.T) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "HalfCaps",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 500, HasShape: true},
			{Rune: 'F', Advance: 500, HasShape: true},
			{Rune: 'i', Advance: 500, HasShape: true},
			{Rune: 'l', Advance: 500, HasShape: true},
			{Rune: 'e', Advance: 500, HasShape: true},
			{Rune: 'r', Advance: 500, HasShape: true},
			{Rune: 'T', Advance: 500, HasShape: true},
			{Rune: 'x', Advance: 500, HasShape: true},
			{Rune: 't', Advance: 500, HasShape: true},
			{Rune: ' ', Advance: 250, HasShape: true},
			{Rune: 0xE000, Advance: 400, HasShape: true}, // i.smcp
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{"smcp": {{3}, {11}}}),
		},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
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
