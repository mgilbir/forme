package layout

import (
	"math"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
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
		{"the font shorthand", "font: small-caps 20px Cap", shape.CapsSmall},
		{"normal", "font-variant-caps: normal", shape.CapsNormal},
		{"all-small-caps", "font-variant-caps: all-small-caps", shape.CapsAllSmall},
		// §6.6's one fallback between values: Noto Sans declares 'smcp' and
		// 'c2sc' and no petite capitals at all, so a document asking for petite
		// ones is given the small ones rather than a synthesis.
		{"petite-caps falls back", "font-variant: petite-caps", shape.CapsSmall},
		{"all-petite-caps falls back", "font-variant: all-petite-caps", shape.CapsAllSmall},
		{"unicase", "font-variant: unicase", shape.CapsUnicase},
		{"titling-caps", "font-variant: titling-caps", shape.CapsTitling},
		{"a value of no level at all", "font-variant-caps: sideways", shape.CapsNormal},
	} {
		// Through the face that declares small capitals, so that what reaches
		// the run is what the declaration asked for and not what synthesis had
		// to do instead: a synthesised run carries CapsNormal by construction,
		// because it must not ask the face for the feature it stands in for.
		frag, _ := layoutWith(t, smallCapsFontSet(t),
			`<div id="d" style="`+c.css+`">office</div>`,
			`body{margin:0} #d{font-family:Cap; font-size:20px}`)
		var got shape.Features
		var found bool
		for _, op := range Paint(frag) {
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
	frag, _ := layoutWith(t, smallCapsFontSet(t),
		`<div id="d">plain <span id="s">inner</span></div>`,
		`body{margin:0} #d{font-family:Cap; font-size:20px; font-variant: small-caps}`)
	var runs int
	for _, op := range Paint(frag) {
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

// TestCapsAreReportedWhenTheFaceHasNoneAndNothingIsSynthesised.
//
// The standard fourteen carry no OpenType features at all, so a document that
// asks them for capitals of a kind this engine does not synthesise gets the
// letters it wrote. Nothing about the page says so, which is why the finding
// has to.
//
// Four of the six are not in the list, and that is the point of the list.
// Neither of the two that are is a letter drawn smaller: "unicase" asks for a
// face's own single-height forms of both cases and "titling-caps" for capitals
// cut lighter, and there is nothing to scale a capital into for either. The
// other four are synthesised — see TestEveryValueOfTheFamilyIsSynthesised.
func TestCapsAreReportedWhenTheFaceHasNoneAndNothingIsSynthesised(t *testing.T) {
	for _, c := range []struct{ value, tags string }{
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
// may declare one of them. What the face does is half the work; this engine does
// the other half, and the finding has to say which half is whose — an author
// told only that "all-small-caps was synthesised" would look at a line whose
// small capitals are the designer's and wonder what was wrong with them.
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
	if f.Rule != RuleCapsSynthesised {
		t.Errorf("the finding is %s, want %s", f.Rule, RuleCapsSynthesised)
	}
	if strings.Contains(f.Message, "no c2sc or smcp") {
		t.Errorf("the finding is %q; the face declares smcp and did half the "+
			"work, so naming both is telling the author their small capitals "+
			"were made here when they were drawn", f.Message)
	}
	if !strings.Contains(f.Message, "no c2sc") {
		t.Errorf("the finding is %q and does not name c2sc, which is the half "+
			"the face has not got", f.Message)
	}
	if !strings.Contains(f.Message, "the capitals were made") {
		t.Errorf("the finding is %q; it has to say which case was made here, "+
			"which for a face with smcp and no c2sc is the capitals", f.Message)
	}
}

// TestHalfAnAnsweredRequestSetsOnlyItsOwnHalf is the same face, on the page.
//
// The lowercase letters go through the face's 'smcp' at the box's own size — a
// small capital the designer drew is already the right height — and only the
// capitals are shrunk. Shrinking both would draw the designer's small capitals
// at three-quarters of the size they were cut for.
func TestHalfAnAnsweredRequestSetsOnlyItsOwnHalf(t *testing.T) {
	face := halfCapsFace(t)
	set := namedFaceSet{family: "Half", face: face, standard: StandardFonts()}
	runs := synthesisedRuns(t, set,
		`<p id="p">iF</p>`,
		`body{margin:0} #p { font-family: Half; font-size: 100px;
		 font-variant-caps: all-small-caps }`)
	full, _ := style.FromPx(100)
	if len(runs) != 2 {
		var got []string
		for _, r := range runs {
			got = append(got, r.Text)
		}
		t.Fatalf("the page was drawn as %d runs %q, want two: one half through "+
			"the face and one made here", len(runs), got)
	}
	if runs[0].Text != "i" || runs[0].Size != full {
		t.Errorf("the lowercase run is %q at %v; the face declares smcp, so it "+
			"is set as written at the box's %v and the face lowers it",
			runs[0].Text, runs[0].Size, full)
	}
	if runs[0].Features.Caps != shape.CapsAllSmall {
		t.Errorf("the lowercase run asks the face for %v, want %v — the half "+
			"the face can do is still asked for", runs[0].Features.Caps,
			shape.CapsAllSmall)
	}
	if runs[1].Text != "F" || runs[1].Size >= full {
		t.Errorf("the capital run is %q at %v; the face declares no c2sc, so it "+
			"is shrunk here and its letter is left alone", runs[1].Text, runs[1].Size)
	}
	if runs[1].Features.Caps != shape.CapsNormal {
		t.Errorf("the synthesised run asks the face for %v as well",
			runs[1].Features.Caps)
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

// Small capitals made out of the capitals, for a face that drew none.
//
// §6.6 allows it and does not say how, and it is what the great majority of
// documents get: the fourteen standard PDF faces declare no OpenType feature at
// all, so a page set in the default serif has no 'smcp' to ask for. The runs are
// cut where the case changes and each piece set at its own size, which is what
// makes this different from every other thing done to a run of text here.

// synthesisedRuns is what a document was drawn as: the text of each run and the
// size it was set at.
func synthesisedRuns(t *testing.T, set FontSet, htmlSrc, cssSrc string) []DrawText {
	t.Helper()
	frag, _ := layoutWith(t, set, htmlSrc, cssSrc)
	var out []DrawText
	for _, op := range Paint(frag) {
		if v, ok := op.(DrawText); ok && v.Text != "" {
			out = append(out, v)
		}
	}
	return out
}

// TestSmallCapsAreSynthesisedWhenTheFaceHasNone is the capability, end to end.
//
// "Filler Text" in a face with no small capitals comes out as four runs: the
// capitals at the box's own size, and the letters between them uppercased and
// set smaller. Every one of those four facts is load-bearing — a version that
// uppercased without shrinking would draw a line of capitals, and one that
// shrank without uppercasing would draw small lowercase letters.
func TestSmallCapsAreSynthesisedWhenTheFaceHasNone(t *testing.T) {
	runs := synthesisedRuns(t, StandardFonts(),
		`<p id="p">Filler Text</p>`,
		`body{margin:0} #p { font-family: Helvetica; font-size: 20px;
		 font-variant: small-caps }`)
	full, _ := style.FromPx(20)
	want := []struct {
		text  string
		small bool
	}{
		{"F", false},    // written as a capital, so already the right letter
		{"ILLER", true}, // uppercased and shrunk
		{" ", false},    // no case, so nothing to do to it
		{"T", false},    //
		{"EXT", true},   //
	}
	if len(runs) != len(want) {
		var got []string
		for _, r := range runs {
			got = append(got, r.Text)
		}
		t.Fatalf("the page was drawn as %d runs %q, want %d", len(runs), got, len(want))
	}
	var drawn string
	for i, r := range runs {
		drawn += r.Text
		if r.Text != want[i].text {
			t.Errorf("run %d is %q, want %q", i, r.Text, want[i].text)
			continue
		}
		if small := r.Size < full; small != want[i].small {
			t.Errorf("the run %q was set at %v against the box's %v; want "+
				"smaller=%v", r.Text, r.Size, full, want[i].small)
		}
	}
	if drawn != "FILLER TEXT" {
		t.Errorf("the page spells %q, want \"FILLER TEXT\"; a synthesised small "+
			"capital is the uppercase letter", drawn)
	}
}

// TestTheSynthesisedSizeComesFromTheFace.
//
// A small capital is a capital cut to about the height of the lowercase
// letters, so the face states the number: its x-height over its cap height.
// Taking it from the face rather than from a constant is what makes a face with
// unusually large lowercase letters get capitals that match them — Times is
// 0.680 and Courier 0.758, and a page set in one and a page set in the other
// are not the same page.
func TestTheSynthesisedSizeComesFromTheFace(t *testing.T) {
	sizes := map[string]style.Unit{}
	for _, family := range []string{"Times", "Courier", "Helvetica"} {
		runs := synthesisedRuns(t, StandardFonts(),
			`<p id="p">filler</p>`,
			`body{margin:0} #p { font-family: `+family+`; font-size: 100px;
			 font-variant: small-caps }`)
		if len(runs) != 1 {
			t.Fatalf("%s drew %d runs, want one: the text is all lowercase",
				family, len(runs))
		}
		sizes[family] = runs[0].Size

		face, ok := StandardFonts().Face(family, false, false)
		if !ok {
			t.Fatalf("no %s face", family)
		}
		d := face.Descriptor()
		want, _ := style.FromPx(100 * float64(d.XHeight) / float64(d.CapHeight))
		if runs[0].Size != want {
			t.Errorf("%s set its synthesised capitals at %v; its x-height is %d "+
				"and its cap height %d, which is %v", family, runs[0].Size,
				d.XHeight, d.CapHeight, want)
		}
	}
	if sizes["Times"] == sizes["Courier"] {
		t.Errorf("Times and Courier both set their synthesised capitals at %v; "+
			"their x-heights differ and the size is taken from the face",
			sizes["Times"])
	}
}

// TestASynthesisedRunDoesNotAskTheFaceForTheFeatureItStandsIn.
//
// The two go together and the second is not tidiness. A face with 'c2sc' and no
// 'smcp' asked for "all-small-caps" would find the letters the synthesis has
// just uppercased and lower them a second time — capitals at the small-capital
// size, scaled again.
func TestASynthesisedRunDoesNotAskTheFaceForTheFeatureItStandsIn(t *testing.T) {
	for _, r := range synthesisedRuns(t, StandardFonts(),
		`<p id="p">filler</p>`,
		`body{margin:0} #p { font-family: Helvetica; font-size: 20px;
		 font-variant: small-caps }`) {
		if r.Features.Caps != shape.CapsNormal {
			t.Errorf("the synthesised run %q asks the face for %v as well",
				r.Text, r.Features.Caps)
		}
	}
}

// TestSynthesisDoesNotChangeTheHeightOfTheLine.
//
// A line's height is the box's, not the run's. A paragraph whose lines grew and
// shrank with the case of their letters would be set on a ragged baseline, and
// two paragraphs of the same text in the same face would be different heights
// because one of them is in small capitals.
func TestSynthesisDoesNotChangeTheHeightOfTheLine(t *testing.T) {
	const doc = `<p id="p">Filler Text</p>`
	const css = `body{margin:0} #p { font-family: Helvetica; font-size: 20px`
	plain, _ := layoutWith(t, StandardFonts(), doc, css+` }`)
	small, _ := layoutWith(t, StandardFonts(), doc, css+`; font-variant: small-caps }`)
	a, b := find(t, plain, "p"), find(t, small, "p")
	if a.BorderRect.H != b.BorderRect.H {
		t.Errorf("the paragraph is %v high in small capitals and %v without; "+
			"the line takes its height from the box and not from the size a "+
			"run happens to be set at", b.BorderRect.H, a.BorderRect.H)
	}
	if len(a.Lines) != len(b.Lines) {
		t.Errorf("the paragraph has %d lines in small capitals and %d without",
			len(b.Lines), len(a.Lines))
	}
}

// TestSynthesisIsReportedAndIsNotAGap.
//
// §6.6 names the technique, so a page that got its capitals this way is a page
// CSS asked for and §7.1's companion signal must not see a gap — a reftest whose
// two documents both synthesised is still comparing the thing it is about.
//
// It is reported all the same, for two reasons a substituted font does not have:
// a scaled capital is not the one a designer would have drawn, and the page
// carries the uppercase text, so a reader copying a synthesised line out of the
// PDF gets "FILLER" where the document said "Filler".
func TestSynthesisIsReportedAndIsNotAGap(t *testing.T) {
	fired[RuleCapsSynthesised] = true

	_, findings := layoutWith(t, StandardFonts(),
		`<p id="p">Filler Text</p>`,
		`#p { font-family: Helvetica; font-size: 20px; font-variant: small-caps }`)
	f, ok := findingNaming(findings, "font-variant-caps")
	if !ok {
		t.Fatalf("synthesised capitals were not reported at all: %v", findings)
	}
	if f.Rule != RuleCapsSynthesised {
		t.Errorf("the finding is %s, want %s", f.Rule, RuleCapsSynthesised)
	}
	if f.Unsupported() {
		t.Error("the finding claims the engine is missing something; §6.6 names " +
			"the synthesis as a thing a user agent may do, and a reftest whose " +
			"two documents both did it is still comparing what it is about")
	}
	if !strings.Contains(f.Message, "uppercase text") {
		t.Errorf("the finding is %q and does not say that the page carries the "+
			"uppercase text, which is the half of this an author cannot see",
			f.Message)
	}
}

// TestEveryValueOfTheFamilyIsSynthesised.
//
// The four values that are a letter drawn smaller, over a face that declares
// none of them. What separates them on the page is the capitals: "small-caps"
// and "petite-caps" leave them alone, and the two "all" values lower them too,
// so a line under one is in two heights of letter and a line under the other is
// in one.
func TestEveryValueOfTheFamilyIsSynthesised(t *testing.T) {
	full, _ := style.FromPx(100)
	for _, c := range []struct {
		value          string
		capitalsShrunk bool
	}{
		{"small-caps", false},
		{"petite-caps", false},
		{"all-small-caps", true},
		{"all-petite-caps", true},
	} {
		runs := synthesisedRuns(t, StandardFonts(),
			`<p id="p">Fi</p>`,
			`body{margin:0} #p { font-family: Helvetica; font-size: 100px;
			 font-variant-caps: `+c.value+` }`)
		var drawn string
		for _, r := range runs {
			drawn += r.Text
		}
		if drawn != "FI" {
			t.Errorf("%s drew %q, want \"FI\"", c.value, drawn)
			continue
		}
		if c.capitalsShrunk {
			if len(runs) != 1 {
				t.Errorf("%s drew %d runs, want one: both cases are lowered, so "+
					"the whole of it is set at one size", c.value, len(runs))
				continue
			}
			if runs[0].Size >= full {
				t.Errorf("%s set the whole run at %v, want below the box's %v",
					c.value, runs[0].Size, full)
			}
			continue
		}
		if len(runs) != 2 {
			t.Errorf("%s drew %d runs, want two: the capital stands and the "+
				"lowercase letter is lowered", c.value, len(runs))
			continue
		}
		if runs[0].Text != "F" || runs[0].Size != full {
			t.Errorf("%s set the capital %q at %v, want the box's %v — it leaves "+
				"the capitals alone", c.value, runs[0].Text, runs[0].Size, full)
		}
		if runs[1].Text != "I" || runs[1].Size >= full {
			t.Errorf("%s set the lowercase letter as %q at %v, want \"I\" below "+
				"the box's %v", c.value, runs[1].Text, runs[1].Size, full)
		}
	}
}

// TestNothingWithoutACaseIsShrunk.
//
// "all-small-caps" lowers both cases, and a space has neither. A line whose
// spaces were three-quarters of a space wide is spaced wrong between every pair
// of words — and it is the failure that looks like success, because every letter
// on it is right.
//
// It is why the case cut has three kinds and not two. A version telling only
// lowercase from everything else passes every fixture that has no space in it,
// which is most of them.
func TestNothingWithoutACaseIsShrunk(t *testing.T) {
	full, _ := style.FromPx(100)
	runs := synthesisedRuns(t, StandardFonts(),
		`<p id="p">Ab&nbsp;Cd</p>`,
		`body{margin:0} #p { font-family: Helvetica; font-size: 100px;
		 font-variant-caps: all-small-caps }`)
	var got []string
	for _, r := range runs {
		got = append(got, r.Text)
	}
	if want := []string{"AB", "\u00a0", "CD"}; !equalStrings(got, want) {
		t.Fatalf("the page was drawn as %q, want %q: the space has no case, so "+
			"it is a run of its own between two that are lowered", got, want)
	}
	if runs[1].Size != full {
		t.Errorf("the space was set at %v, want the box's %v; nothing lowers a "+
			"character that has no case to lower", runs[1].Size, full)
	}
	for _, i := range []int{0, 2} {
		if runs[i].Size >= full {
			t.Errorf("the run %q was set at %v, want below the box's %v",
				runs[i].Text, runs[i].Size, full)
		}
	}
}

// TestPetiteCapitalsFallBackToSmallOnesBeforeAnythingIsSynthesised.
//
// §6.6's one fallback between values: "if petite capital glyphs are not
// available, small capital glyphs are used". Noto Sans declares 'smcp' and
// 'c2sc' and no petite capitals at all, which is what almost every face with
// small capitals in it declares — so without the fallback the commonest face in
// the checkout would synthesise where it has the designer's own capitals to
// hand.
func TestPetiteCapitalsFallBackToSmallOnesBeforeAnythingIsSynthesised(t *testing.T) {
	set := smallCapsFontSet(t)
	for _, c := range []struct {
		value string
		want  shape.Caps
	}{
		{"petite-caps", shape.CapsSmall},
		{"all-petite-caps", shape.CapsAllSmall},
	} {
		runs := synthesisedRuns(t, set,
			`<p id="p">Filler</p>`,
			`body{margin:0} #p { font-family: Cap; font-size: 20px;
			 font-variant-caps: `+c.value+` }`)
		if len(runs) != 1 {
			t.Errorf("%s drew %d runs; the face has small capitals, so nothing "+
				"is cut and nothing is synthesised", c.value, len(runs))
			continue
		}
		if runs[0].Text != "Filler" {
			t.Errorf("%s drew %q; the face's own capitals need no uppercasing",
				c.value, runs[0].Text)
		}
		if runs[0].Features.Caps != c.want {
			t.Errorf("%s asked the face for %v, want %v", c.value,
				runs[0].Features.Caps, c.want)
		}
	}
	// And it is not reported, because nothing was made here: the page is set in
	// capitals a designer drew, and the only thing the document did not get is
	// the second, shorter cut that this face has never had.
	_, findings := layoutWith(t, set,
		`<p id="p">Filler</p>`,
		`#p { font-family: Cap; font-size: 20px; font-variant-caps: petite-caps }`)
	if f, ok := findingNaming(findings, "font-variant-caps"); ok {
		t.Errorf("the fallback found the face's small capitals and the page was "+
			"reported anyway: %s", f.Message)
	}
}

// TestPetiteFallsBackAtTheValueAndNotAtTheTag.
//
// §6.6's sentence asks whether the face has *petite capitals*, and a face with
// none of them is asked for small ones. Reading it per tag instead — this half
// petite and that half small — would set a line in two designs, which is not
// what either value means.
//
// The fixture is the face that has 'pcap' and no 'c2pc': under a per-tag
// reading its capitals would come from 'c2sc', and under the value reading they
// are made here, beside the petite capitals the face really drew.
func TestPetiteFallsBackAtTheValueAndNotAtTheTag(t *testing.T) {
	face := petiteAndSmallFace(t)
	set := namedFaceSet{family: "Both", face: face, standard: StandardFonts()}
	runs := synthesisedRuns(t, set,
		`<p id="p">Fi</p>`,
		`body{margin:0} #p { font-family: Both; font-size: 100px;
		 font-variant-caps: all-petite-caps }`)
	if len(runs) != 2 {
		var got []string
		for _, r := range runs {
			got = append(got, r.Text)
		}
		t.Fatalf("the page was drawn as %d runs %q, want two", len(runs), got)
	}
	for _, r := range runs {
		if r.Features.Caps != shape.CapsNormal && r.Features.Caps != shape.CapsAllPetite {
			t.Errorf("the run %q asks the face for %v; the face has petite "+
				"capitals, so the value stays what the document wrote",
				r.Text, r.Features.Caps)
		}
	}
	full, _ := style.FromPx(100)
	if runs[0].Text != "F" || runs[0].Size >= full {
		t.Errorf("the capital run is %q at %v; the face declares no c2pc and "+
			"does not borrow c2sc, so its capital is shrunk here",
			runs[0].Text, runs[0].Size)
	}
	if runs[1].Text != "i" || runs[1].Size != full {
		t.Errorf("the lowercase run is %q at %v; the face declares pcap and "+
			"lowers it itself, at the box's %v", runs[1].Text, runs[1].Size, full)
	}

	// And the other way round, which is the half the reading above cannot be
	// told apart from unless the *capitals* are the half the face has. This
	// face declares 'c2pc' and 'smcp': a per-tag reading would find no 'pcap'
	// for the lowercase half and borrow the small capitals, setting a line whose
	// two cases are two different cuts.
	other := capitalsPetiteOnlyFace(t)
	runs = synthesisedRuns(t, namedFaceSet{family: "Other", face: other, standard: StandardFonts()},
		`<p id="p">Fi</p>`,
		`body{margin:0} #p { font-family: Other; font-size: 100px;
		 font-variant-caps: all-petite-caps }`)
	if len(runs) != 2 {
		var got []string
		for _, r := range runs {
			got = append(got, r.Text)
		}
		t.Fatalf("the page was drawn as %d runs %q, want two", len(runs), got)
	}
	if runs[0].Text != "F" || runs[0].Size != full ||
		runs[0].Features.Caps != shape.CapsAllPetite {
		t.Errorf("the capital run is %q at %v asking for %v; the face declares "+
			"c2pc, so its own petite capital is used at the box's %v",
			runs[0].Text, runs[0].Size, runs[0].Features.Caps, full)
	}
	if runs[1].Text != "I" || runs[1].Size >= full ||
		runs[1].Features.Caps != shape.CapsNormal {
		t.Errorf("the lowercase run is %q at %v asking for %v; the face has "+
			"petite capitals, so the value does not fall back and the other "+
			"half is made here", runs[1].Text, runs[1].Size, runs[1].Features.Caps)
	}
}

// TestARestyledFirstLineKeepsItsSynthesis.
//
// ::first-line restyles the items on the first line from a box of its own, and
// a synthesised run cannot be rebuilt from a box: its text has already been
// uppercased, and setting it back to the box's size draws a line of capitals at
// full height where small capitals were asked for. The two things the synthesis
// did are re-applied over the new size instead.
//
// The second half is the one that is invisible: an item put back to its box's
// features asks the face for the very feature the run is standing in for.
func TestARestyledFirstLineKeepsItsSynthesis(t *testing.T) {
	runs := synthesisedRuns(t, StandardFonts(),
		`<p id="p">filler text here and more words to wrap onto a second line</p>`,
		`body{margin:0} #p { font-family: Helvetica; font-size: 20px; width: 200px;
		 font-variant: small-caps }
		 #p::first-line { font-size: 40px }`)
	if len(runs) < 2 {
		t.Fatalf("the fixture drew %d runs", len(runs))
	}
	face, _ := StandardFonts().Face("Helvetica", false, false)
	d := face.Descriptor()
	ratio := float64(d.XHeight) / float64(d.CapHeight)
	first, _ := style.FromPx(40 * ratio)
	rest, _ := style.FromPx(20 * ratio)
	if runs[0].Size != first {
		t.Errorf("the first line's synthesised run is %v, want %v — the "+
			"::first-line size shrunk in proportion, not the size itself",
			runs[0].Size, first)
	}
	if runs[0].Features.Caps != shape.CapsNormal {
		t.Errorf("the restyled run asks the face for %v; it is standing in for "+
			"that feature and asking for it as well applies it twice",
			runs[0].Features.Caps)
	}
	// And the lines below it are untouched, so what is being checked is the
	// restyle and not a size the whole paragraph happens to have.
	var found bool
	for _, r := range runs {
		if r.Size == rest {
			found = true
		}
	}
	if !found {
		t.Errorf("no run is at the paragraph's own synthesised size %v, so the "+
			"fixture has only one line and tests no restyle", rest)
	}
}

// TestSynthesisCutsMaximalStretches.
//
// Every boundary costs a run, and a run is measured, shaped and drawn on its
// own: a word cut into one run per letter loses every kern and every ligature
// inside it. So a character with no case of its own is not a cut — it joins
// whichever side it is next to.
func TestSynthesisCutsMaximalStretches(t *testing.T) {
	runs := synthesisedRuns(t, StandardFonts(),
		`<p id="p">Filler&nbsp;Text</p>`,
		`body{margin:0} #p { font-family: Helvetica; font-size: 20px;
		 font-variant: small-caps }`)
	var got []string
	for _, r := range runs {
		got = append(got, r.Text)
	}
	want := []string{"F", "ILLER", "\u00a0T", "EXT"}
	if !equalStrings(got, want) {
		t.Errorf("the page was drawn as %q, want %q: the no-break space has no "+
			"case, so it is neither cut at nor made a run of its own", got, want)
	}
}

// TestCutAtCaseIsTheReaderOnItsOwn.
//
// The third kind is the one worth having a table for. A space, a digit and a
// full stop are neither lowered by "small-caps" nor *shrunk* by
// "all-small-caps", and a version that told only lowercase from everything else
// would set a line whose spaces were three-quarters of a space wide.
func TestCutAtCaseIsTheReaderOnItsOwn(t *testing.T) {
	for _, c := range []struct {
		text string
		want []casePart
	}{
		{"", []casePart{{text: "", kind: caseNone}}},
		{"abc", []casePart{{text: "abc", kind: caseLower}}},
		{"ABC", []casePart{{text: "ABC", kind: caseUpper}}},
		{"Filler", []casePart{{text: "F", kind: caseUpper}, {text: "iller", kind: caseLower}}},
		{"aB", []casePart{{text: "a", kind: caseLower}, {text: "B", kind: caseUpper}}},
		// Digits, punctuation and a script with one case have no form of the
		// other case and are a stretch of their own — which for a run between
		// two lowercase stretches means three runs and not one.
		{"a1b", []casePart{{text: "a", kind: caseLower}, {text: "1", kind: caseNone},
			{text: "b", kind: caseLower}}},
		{"A B", []casePart{{text: "A", kind: caseUpper}, {text: " ", kind: caseNone},
			{text: "B", kind: caseUpper}}},
		{"日本語", []casePart{{text: "日本語", kind: caseNone}}},
	} {
		got := cutAtCase(c.text)
		if len(got) != len(c.want) {
			t.Errorf("cutAtCase(%q) = %+v, want %+v", c.text, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("cutAtCase(%q)[%d] = %+v, want %+v",
					c.text, i, got[i], c.want[i])
			}
		}
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

// capitalsPetiteOnlyFace declares 'c2pc' and 'smcp': petite capitals for the
// capitals, small ones for the lowercase letters, and neither of the two the
// other half of each pair would need.
//
// It is the fixture for whether §6.6's fallback is read at the value or at the
// tag, and it is the only shape of font that can tell them apart. A per-tag
// reading finds no 'pcap' for the lowercase half, falls back to the 'smcp' this
// face has, and sets a line whose capitals are petite and whose lowercase
// letters are small — two cuts of the same design, side by side. Reading the
// sentence at the value keeps the petite capitals the face really drew and makes
// the other half here.
func capitalsPetiteOnlyFace(t *testing.T) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "CapitalsPetite",
		Glyphs: []fonttest.Glyph{
			{Rune: 'i', Advance: 500, HasShape: true},
			{Rune: 'F', Advance: 500, HasShape: true},
			{Rune: 0xE000, Advance: 400, HasShape: true}, // i.smcp
			{Rune: 0xE001, Advance: 400, HasShape: true}, // F.c2pc
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"smcp": {{1}, {3}},
				"c2pc": {{2}, {4}},
			}),
		},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatalf("loading the fixture face: %v", err)
	}
	return face
}

// petiteAndSmallFace declares 'pcap' and 'c2sc' and neither of the two the
// other of each pair would need: petite capitals for the lowercase letters, and
// small ones for the capitals.
//
// It is the fixture for the question "does the fallback read §6.6's sentence at
// the value or at the tag", and it is the only shape of font that can answer it.
func petiteAndSmallFace(t *testing.T) *shape.Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "PetiteAndSmall",
		Glyphs: []fonttest.Glyph{
			{Rune: 'i', Advance: 500, HasShape: true},
			{Rune: 'F', Advance: 500, HasShape: true},
			{Rune: 0xE000, Advance: 400, HasShape: true}, // i.pcap
			{Rune: 0xE001, Advance: 400, HasShape: true}, // F.c2sc
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBForms(map[string][2][]int{
				"pcap": {{1}, {3}},
				"c2sc": {{2}, {4}},
			}),
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
