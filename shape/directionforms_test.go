package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// The forms a run's direction selects: 'rtlm', 'rtla', 'ltrm' and 'ltra'.
//
// A mirrored form is the glyph a character is drawn with when the line runs the
// other way. Unicode mirrors a bracket by character — U+0028 in a right-to-left
// paragraph is drawn as U+0029's shape — and a font may state the same thing by
// glyph instead, which is what 'rtlm' is for: it covers what the character
// property cannot, an integral sign or an arrow that leans.
//
// None of the four was applied. A font stating them was answered with the
// glyphs it states for the other direction, and nothing said so: a substitution
// that never runs leaves no trace, so the page carries a bracket pointing the
// wrong way and no finding.

// The fixture's glyph indices. SFNT numbers glyphs from one in the order given,
// glyph zero being notdef.
const (
	gidAleph  = 1 // an RTL letter, so a run holding it is an RTL run
	gidPlain  = 2
	gidTurned = 3
)

// directionFace builds a face whose only feature is the named one, turning
// gidPlain into gidTurned.
func directionFace(t *testing.T, tag string) *Face {
	t.Helper()
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "Direction",
		Glyphs: []fonttest.Glyph{
			{Rune: 'א', Advance: 500, HasShape: true},
			// A neutral character Unicode does *not* mirror, and its
			// replacement. A bracket would be no use here: rule L4 mirrors one
			// by character before the font is asked for a glyph at all, so a
			// turned bracket in a right-to-left run says nothing about whether
			// the font's own rule ran.
			{Rune: '#', Advance: 400, HasShape: true},
			{Rune: '@', Advance: 400, HasShape: true},
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBLookups([]fonttest.Lookup{{
				Type:      1,
				Subtables: [][]byte{fonttest.SingleSubst([]int{gidPlain}, []int{gidTurned})},
			}}, map[string][]int{tag: {0}}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// glyphsOf shapes a string and returns the glyph indices.
func glyphsOf(t *testing.T, f *Face, s string) []int {
	t.Helper()
	gs, _ := f.ShapeGlyphs(s)
	out := make([]int, len(gs))
	for i, g := range gs {
		out[i] = g.GID
	}
	return out
}

// TestADirectionSelectsTheFormsTheFontStatesForIt.
//
// Four features, two directions, and each pair has to apply in its own
// direction and in neither other. The run's direction comes from its
// characters: a bracket beside an aleph is in a right-to-left run, and the same
// bracket beside an "a" is not.
func TestADirectionSelectsTheFormsTheFontStatesForIt(t *testing.T) {
	for _, c := range []struct {
		tag     string
		applies bool // in the right-to-left run
	}{
		{"rtlm", true},
		{"rtla", true},
		{"ltrm", false},
		{"ltra", false},
	} {
		t.Run(c.tag, func(t *testing.T) {
			f := directionFace(t, c.tag)

			// A right-to-left run. The "#" is a neutral, so it is put between
			// two alephs: a neutral between two right-to-left characters
			// resolves to their direction, and one at the end of the string
			// would be a left-to-right run of its own.
			rtl := glyphsOf(t, f, "א#א")
			// A left-to-right run: the same character with nothing
			// right-to-left beside it.
			ltr := glyphsOf(t, f, "##")

			turnedInRTL := holdsGID(rtl, gidTurned)
			turnedInLTR := holdsGID(ltr, gidTurned)

			if turnedInRTL != c.applies {
				t.Errorf("in a right-to-left run %q %s; the font states it for "+
					"%s text. Glyphs: %v (%d is the replacement)", c.tag,
					didOrNot(turnedInRTL), directionOf(c.tag), rtl, gidTurned)
			}
			if turnedInLTR == c.applies {
				t.Errorf("in a left-to-right run %q %s; the font states it for "+
					"%s text. Glyphs: %v", c.tag,
					didOrNot(turnedInLTR), directionOf(c.tag), ltr)
			}
		})
	}
}

// TestADirectionFormIsListedAsAFeature is the other half: a caller that asks
// what a face offers has to be told about these too, since Features() is what
// "check one before asking for it" reads.
func TestADirectionFormIsListedAsAFeature(t *testing.T) {
	f := directionFace(t, "rtlm")
	if len(f.Features()) == 0 {
		t.Fatalf("the face lists no features at all")
	}
	found := false
	for _, tag := range f.Features() {
		if tag == "rtlm" {
			found = true
		}
	}
	if !found {
		t.Errorf("Features() came back with %v and the face states rtlm", f.Features())
	}
}

// holdsGID reports whether a shaped run drew a glyph.
func holdsGID(xs []int, want int) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

func didOrNot(b bool) string {
	if b {
		return "applied"
	}
	return "did not apply"
}

func directionOf(tag string) string {
	if tag[0] == 'r' {
		return "right-to-left"
	}
	return "left-to-right"
}
