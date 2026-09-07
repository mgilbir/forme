package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The three things the cursive model does and this engine does not.
//
// They are named in arabic.go's header, and the reason to test an absence is
// that a list of what is missing goes stale the moment something stops being
// missing. Each of these declares the feature in a font and requires that
// nothing happen — so implementing one fails the test that says it is absent,
// and whoever implements it is sent to the list.

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

// TestSyriacsAlaphIsNotShaped. The letter takes a final form decided by what
// comes before it, which the model states as three features of its own. None of
// them is applied here.
func TestSyriacsAlaphIsNotShaped(t *testing.T) {
	f := declaringFace(t, "syrc", syriacAlaph, syriacBeth, "fin2", "fin3", "med2")
	if applied(t, f, string([]rune{syriacBeth, syriacAlaph})) {
		t.Error("a Syriac Alaph was given one of its own final forms; arabic.go " +
			"says that rule is absent, and the header has to say so no longer")
	}
	// The control: the four ordinary forms *are* applied, so the fixture is not
	// silent for want of a working font. Two Beths, because Beth joins forward
	// and Alaph does not — the second of them is the one in final position.
	g := declaringFace(t, "syrc", syriacBeth, syriacAlaph, "fina")
	if !applied(t, g, string([]rune{syriacBeth, syriacBeth})) {
		t.Error("the ordinary final form was not applied either, so this test is " +
			"watching nothing")
	}
}

// TestStchIsNotApplied. Syriac stretches a letter to fill a line rather than
// spacing its words, and the feature that says which letter is not applied.
func TestStchIsNotApplied(t *testing.T) {
	f := declaringFace(t, "syrc", syriacBeth, syriacAlaph, "stch")
	if applied(t, f, string([]rune{syriacBeth, syriacAlaph})) {
		t.Error("'stch' was applied; arabic.go says it is absent")
	}
}

// TestThereIsNoFallbackShaping. A font that declares none of the four positional
// features is set in the letters as written — Unicode's presentation forms hold
// those shapes as characters, and a shaper with nothing else to go on maps to
// them. This one does not.
func TestThereIsNoFallbackShaping(t *testing.T) {
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "NoForms",
		Glyphs: []fonttest.Glyph{
			{Rune: beh, Advance: 500, HasShape: true},
			// The initial form of beh as a character of its own, which a
			// fallback would map to.
			{Rune: 0xFE91, Advance: 300, HasShape: true},
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	if f.HasJoiningForms() {
		t.Fatal("the fixture declares positional forms after all")
	}
	glyphs, _ := f.ShapeGlyphs(string([]rune{beh, beh}))
	if len(glyphs) != 2 {
		t.Fatalf("two letters came to %d glyphs", len(glyphs))
	}
	if glyphs[0].GID != glyphs[1].GID {
		t.Errorf("the two letters were drawn as %d and %d: something chose a "+
			"form for them, and arabic.go says nothing does",
			glyphs[0].GID, glyphs[1].GID)
	}
}
