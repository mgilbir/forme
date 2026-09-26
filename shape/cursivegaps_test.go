package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The things the cursive model does that this engine once did not.
//
// They were named in arabic.go's header, and the reason to test an absence is
// that a list of what is missing goes stale the moment something stops being
// missing. Each of these declared the feature in a font and required that
// nothing happen — so implementing one failed the test that said it was
// absent, and whoever implemented it was sent to the list. All three have been
// implemented that way: each test failed, the header was changed, and each now
// says what the engine does. The full answers, against HarfBuzz, are in
// arabicfallback_test.go and syriac_test.go.

// The Syriac letter whose final form is chosen by what precedes it, and the
// three feature tags the model states that rule with.
const (
	syriacAlaph = 0x0710
	syriacBeth  = 0x0712 // dual-joining, so an Alaph after it joins backwards
)

// declaringFace builds a face for one script that declares a feature over a
// single substitution, so that a test can ask whether this engine applies it.
func declaringFace(t *testing.T, script string, from, to rune, tags ...string) *Face {
	t.Helper()
	forms := map[string][2][]int{}
	for _, tag := range tags {
		forms[tag] = [2][]int{{1}, {3}}
	}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Cursive",
		Glyphs: []fonttest.Glyph{
			{Rune: from, Advance: 500, HasShape: true},   // 1
			{Rune: to, Advance: 500, HasShape: true},     // 2
			{Rune: 0xE010, Advance: 200, HasShape: true}, // 3, what the feature would make
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBFormsIn(forms, map[string]fonttest.Script{
				script: fonttest.AllFeatures(len(tags)),
				"DFLT": {Required: fonttest.NoFeature},
			}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

// applied reports whether the face's declared substitution happened anywhere in
// the shaped run.
func applied(t *testing.T, f *Face, s string) bool {
	t.Helper()
	glyphs, _ := f.ShapeGlyphs(s)
	for _, g := range glyphs {
		if g.GID == 3 {
			return true
		}
	}
	return false
}

// TestSyriacsAlaphIsShaped. The letter takes a final form decided by what
// comes before it, which the model states as three features of its own: an
// Alaph after a letter that does not join forward is in its second final form
// ('fin2'), which is what the second of two Alaphs is. It was the test that
// none of them was applied.
func TestSyriacsAlaphIsShaped(t *testing.T) {
	f := declaringFace(t, "syrc", syriacAlaph, syriacBeth, "fin2", "fin3", "med2")
	if !applied(t, f, string([]rune{syriacAlaph, syriacAlaph})) {
		t.Error("the second of two Syriac Alaphs was not given its second final form")
	}
	// After a Beth, which joins forward, an Alaph is final in the ordinary way,
	// and none of the three is for it.
	if applied(t, f, string([]rune{syriacBeth, syriacAlaph})) {
		t.Error("an Alaph after a Beth was given one of its own final forms")
	}
	// The control: the four ordinary forms are applied, so the fixture is not
	// silent for want of a working font. Two Beths, because Beth joins forward
	// and Alaph does not — the second of them is the one in final position.
	g := declaringFace(t, "syrc", syriacBeth, syriacAlaph, "fina")
	if !applied(t, g, string([]rune{syriacBeth, syriacBeth})) {
		t.Error("the ordinary final form was not applied either, so this test is " +
			"watching nothing")
	}
}

// TestStchIsApplied. Syriac's abbreviation mark is drawn stretched over its
// word, and the feature that takes it apart into the pieces it is stretched
// with is applied. It was the test that it was not.
func TestStchIsApplied(t *testing.T) {
	f := declaringFace(t, "syrc", syriacBeth, syriacAlaph, "stch")
	if !applied(t, f, string([]rune{syriacBeth, syriacAlaph})) {
		t.Error("'stch' was not applied")
	}
}

// TestAFaceWithNoFormsTakesThemFromItsCharacterMap. A font that declares none
// of the four positional features is set in the forms its character map holds:
// Unicode's presentation forms hold those shapes as characters, and HarfBuzz
// maps to them. This one declares none and maps the initial form of beh, so the
// first of two behs is drawn in it and the second, whose final form the face
// does not map, is drawn as the letter. HarfBuzz 14.5.0 draws the same. It was
// the test that the fallback was absent; see arabicfallback.go.
func TestAFaceWithNoFormsTakesThemFromItsCharacterMap(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "NoForms",
		Glyphs: []fonttest.Glyph{
			{Rune: beh, Advance: 500, HasShape: true},
			// The initial form of beh as a character of its own, which the
			// fallback maps to.
			{Rune: 0xFE91, Advance: 300, HasShape: true},
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	glyphs, _ := f.ShapeGlyphs(string([]rune{beh, beh}))
	// Drawn right to left: the final beh, then the initial form.
	checkShaped(t, "two behs in a face with no forms", glyphs, []shapedAs{{1, 500, 0, 0}, {2, 300, 0, 0}})
}
