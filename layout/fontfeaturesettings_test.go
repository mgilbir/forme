package layout

import (
	"slices"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonttest"
	"github.com/mgilbir/forme/shape"
)

// TestFontFeatureSettingsAsksTheFaceForTheTag is the property a real document
// uses and the suite does not: a named OpenType feature, by tag.
//
// The suite writes font-feature-settings five times and every one of them names
// "kern" over the default serif face, which carries no kerning — so the whole
// property was reported as unapplied and measured worth zero reftests. Real
// documents are not like that: "tnum" is what an invoice's columns are set with,
// "ss01" is how a face's alternate letterforms are asked for, and neither has a
// font-variant keyword to ask through.
//
// The test is self-checking rather than a pinned number. "onum" through
// font-feature-settings and oldstyle-nums through font-variant-numeric are the
// same request to the face — the property and the descriptor name one feature
// between them — so a box shrunk to fit its figures has to come out the same
// width either way, and a different width from the lining figures it replaces.
// Nothing here has to know what an oldstyle figure looks like.
func TestFontFeatureSettingsAsksTheFaceForTheTag(t *testing.T) {
	set := numericFontSet(t)
	// The declaration goes in the stylesheet rather than a style attribute: its
	// value carries quotation marks, and nesting those inside an attribute's own
	// quotes ends the attribute rather than the tag name. The first version of
	// this measured 600 — the whole line box — because the float had stopped
	// being a float.
	widthOf := func(decl string) float64 {
		t.Helper()
		frag, _ := layoutWith(t, set,
			`<div id="d">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px; float:left; `+decl+`}`)
		return find(t, frag, "d").BorderRect.W.Px()
	}

	plain := widthOf("")
	variant := widthOf("font-variant-numeric: oldstyle-nums")
	if plain == variant {
		t.Fatalf("the figures are %g wide with and without oldstyle-nums; the "+
			"fixture is not reaching the face's own feature", plain)
	}

	if got := widthOf(`font-feature-settings: "onum"`); got != variant {
		t.Errorf(`font-feature-settings: "onum" gave %g and `+
			`font-variant-numeric: oldstyle-nums gave %g; they are one request to `+
			`the face and have to reach it the same way (the lining figures are %g)`,
			got, variant, plain)
	}
	// The forms that mean the same thing. "1" and "on" are the value's own
	// spellings of what a bare tag already says.
	for _, css := range []string{
		`font-feature-settings: "onum" 1`,
		`font-feature-settings: "onum" on`,
		`font-feature-settings: 'onum'`,
	} {
		if got := widthOf(css); got != variant {
			t.Errorf("%s gave %g, want %g — the same feature by another spelling",
				css, got, variant)
		}
	}
}

// TestFontFeatureSettingsReportsWhatItCannotDo is the other half, and it is
// what keeps the half above from being a claim that everything works.
//
// One thing in this property is not carried out: a tag turned on that the face
// has not got changes nothing, which is what reportCaps says about a face with
// no small capitals. A tag turned *off* is carried out — CSS Fonts 4 §7.2 puts
// this property above font-variant-ligatures and font-kerning — and so is not
// reported, whether or not the face has the feature: turning off what is not
// there asks for the page that is already there.
func TestFontFeatureSettingsReportsWhatItCannotDo(t *testing.T) {
	set := numericFontSet(t)
	findings := func(decl string) []Finding {
		t.Helper()
		_, got := layoutWith(t, set,
			`<div id="d">0123</div>`,
			`body{margin:0} #d{font-family:Num; font-size:20px; `+decl+`}`)
		var out []Finding
		for _, f := range got {
			if f.Property == "font-feature-settings" {
				out = append(out, f)
			}
		}
		return out
	}

	// A tag the face has, turned on: applied, and nothing to say about it.
	if got := findings(`font-feature-settings: "onum"`); len(got) != 0 {
		t.Errorf(`"onum" on a face that declares it reported %q; it is applied`,
			got[0].Message)
	}
	// A tag the face has not got: carried out and inert, which is worth saying.
	if got := findings(`font-feature-settings: "zzzz"`); len(got) == 0 {
		t.Error(`"zzzz" on a face that does not declare it said nothing; asking a ` +
			`face for a feature it has not got changes no glyph and the page ` +
			`does not show that it was asked`)
	}
	// A tag turned off: applied, whether the face has it or not.
	for _, decl := range []string{
		`font-feature-settings: "liga" 0`,
		`font-feature-settings: "liga" off`,
		`font-feature-settings: "zzzz" 0`,
		`font-kerning: none; font-feature-settings: "kern" 1`,
	} {
		if got := findings(decl); len(got) != 0 {
			t.Errorf("%s reported %q; it is applied", decl, got[0].Message)
		}
	}
}

// TestFontFeatureSettingsSettlesTheOrderOfItsTags is what lets two declarations
// naming the same features share the shaped group the breaker memoises.
//
// The order a document writes them in is not the order they are applied in —
// that is the font's, by lookup index — so the two are one request and have to
// be spelt the same way by the time they reach the key.
func TestFontFeatureSettingsSettlesTheOrderOfItsTags(t *testing.T) {
	first, _ := featureSettingsOf(`"tnum", "onum"`)
	second, _ := featureSettingsOf(`"onum", "tnum"`)
	if first != second {
		t.Errorf("the same two features came out as %q and %q; they are one "+
			"request and share a memo entry only if they are spelt alike",
			first, second)
	}
	if first == "" {
		t.Fatal("two tags came out as nothing")
	}
	// And a tag named twice is one tag.
	if once, _ := featureSettingsOf(`"onum", "onum"`); once != "onum" {
		t.Errorf(`"onum" twice came out as %q, want "onum"`, once)
	}
	// And a tag given two settings takes the last, in either direction, so a
	// tag is never both turned on and turned off.
	for _, c := range []struct {
		value, on string
		off       []string
	}{
		{`"liga" 0, "liga" 1`, "liga", nil},
		{`"liga" 1, "liga" 0`, "", []string{"liga"}},
		{`"kern" 0, "onum", "kern" on, "liga" off`, "kern,onum", []string{"liga"}},
		{`"liga" 0, "calt" 0, "liga" 0`, "", []string{"calt", "liga"}},
	} {
		on, off := featureSettingsOf(c.value)
		if on != c.on || !slices.Equal(off, c.off) {
			t.Errorf("%s came out as on %q, off %q; want on %q, off %q",
				c.value, on, off, c.on, c.off)
		}
	}
}

// TestAPositioningFeatureIsNotReportedAsMissing: a tag the face offers through
// positioning alone is applied, and is not reported as one the face has not got.
//
// 'halt' is how the suite's text-spacing-trim references draw a trimmed
// bracket, and every CJK face states it in GPOS and nowhere in GSUB. The
// shaping plan applied it; the check beside it asked the face for its
// substitution features only, so eleven references were each reported as
// asking for something the face did not have, over a page drawn as asked.
//
// The fixture is a face whose 'halt' takes half the advance off 'a'. That the
// width changes is what makes the silence a claim and not an omission: the
// feature reached the glyphs.
func TestAPositioningFeatureIsNotReportedAsMissing(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Halt",
		Glyphs: []fonttest.Glyph{
			{Rune: 'a', Advance: 1000, HasShape: true},
			{Rune: ' ', Advance: 250},
			{Rune: '（', Advance: 1000, HasShape: true},
		},
		Extra: map[string][]byte{
			"GPOS": fonttest.GPOSLookups([]fonttest.Lookup{
				{Type: 1, Subtables: [][]byte{fonttest.SinglePosSubtable(1, -500, 0, -500)}},
				{Type: 1, Subtables: [][]byte{fonttest.SinglePosSubtable(3, -500, 0, -500)}},
			}, map[string][]int{"halt": {0, 1}}),
		},
	})
	face, err := shape.Load(data)
	if err != nil {
		t.Fatal(err)
	}
	set := namedFaceSet{family: "Halt", face: face, standard: StandardFonts()}
	runIn := func(set FontSet, family, text, decl string) (float64, []Finding) {
		t.Helper()
		frag, got := layoutWith(t, set, `<div id="d">`+text+`</div>`,
			`body{margin:0} #d{font-family:`+family+`; font-size:20px; float:left; `+decl+`}`)
		var out []Finding
		for _, f := range got {
			if f.Property == "font-feature-settings" {
				out = append(out, f)
			}
		}
		return find(t, frag, "d").BorderRect.W.Px(), out
	}
	run := func(decl string) (float64, []Finding) {
		t.Helper()
		return runIn(set, "Halt", "aa", decl)
	}

	plain, _ := run("")
	halted, findings := run(`font-feature-settings: "halt" 1`)
	if plain != 40 || halted != 20 {
		t.Fatalf("the run is %gpx plain and %gpx with 'halt', want 40 and 20; the "+
			"fixture is not reaching the face's positioning feature", plain, halted)
	}
	if len(findings) != 0 {
		t.Errorf(`"halt" on a face that positions with it reported %q; it was applied`,
			findings[0].Message)
	}
	// The same face asked for something it has not got still says so, which
	// keeps the silence above from being a check that stopped looking.
	// The message quotes the declaration first, so what it names as missing is
	// what follows "asks for".
	_, findings = run(`font-feature-settings: "halt" 1, "zzzz" 1`)
	if len(findings) == 0 {
		t.Fatal(`"halt" and "zzzz" together said nothing; the face has no "zzzz"`)
	}
	msg := findings[0].Message
	named := msg[strings.Index(msg, "asks for")+1:]
	if !strings.Contains(named, `"zzzz"`) || strings.Contains(named, `"halt"`) {
		t.Errorf(`"halt" and "zzzz" together reported %q; want "zzzz" named as `+
			`missing and "halt" not`, msg)
	}

	// And the face asked is the one that sets the text. Helvetica has no
	// full-width bracket, so the brackets are set in the fallback face, which
	// applies 'halt' to them. The question was asked of Helvetica, which set
	// none of the text, and the feature that was carried out was reported as
	// missing.
	fallback := oneFaceSet{fallback: face, standard: StandardFonts()}
	plain, _ = runIn(fallback, "Helvetica", "（（", "")
	halted, findings = runIn(fallback, "Helvetica", "（（", `font-feature-settings: "halt" 1`)
	if plain != 40 || halted != 20 {
		t.Fatalf("the brackets are %gpx plain and %gpx with 'halt', want 40 and 20; "+
			"the fixture is not setting them in the fallback face", plain, halted)
	}
	if len(findings) != 0 {
		t.Errorf(`"halt" carried out by the family that set the text reported %q`,
			findings[0].Message)
	}
}
