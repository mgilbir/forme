package shape

import (
	"testing"

	"github.com/mgilbir/forme/fonttest"
)

// A repha is not a character. It is a *form*: a font may draw the consonant at
// the head of a syllable as a mark above it, written first and drawn last, and
// the only way to find out is that the font declares 'rphf' over that
// consonant. The Universal Shaping Engine therefore reads the feature twice
// over — once to apply it and once to see where it applied — and treats what it
// applied to as a repha from then on, which is what the reordering moves.
//
// Javanese, because that is the script the bundled corpus measures and its
// categories are already pinned by TestUseCategoriesAreWhatTheScriptsNeed.
const (
	javaRA     = 'ꦫ' // the consonant this font gives a repha
	javaKA     = 'ꦏ' // an ordinary base
	javaPangkn = '꧀' // the halant, which stacks what follows

	gidJavaRA = 1 + iota - 3
	gidJavaKA
	gidJavaPangkon
	gidJavaRepha
)

// javaRephaFace is a Javanese font whose only rule is the one that makes a
// repha: the letter RA and the pangkon after it become one glyph.
func javaRephaFace(t *testing.T) *Face {
	t.Helper()
	lookups := []fonttest.Lookup{{
		Type: 4,
		Subtables: [][]byte{fonttest.LigatureSubst([]fonttest.Ligature{
			{Components: []int{gidJavaRA, gidJavaPangkon}, Glyph: gidJavaRepha},
		})},
	}}
	data := fonttest.SFNT(fonttest.SFNTOptions{
		Name: "JavaRepha",
		Glyphs: []fonttest.Glyph{
			{Rune: javaRA, Advance: 500, HasShape: true},
			{Rune: javaKA, Advance: 500, HasShape: true},
			{Rune: javaPangkn, Advance: 0, HasShape: true},
			{Rune: 0xA9AC, Advance: 0, HasShape: true}, // the repha glyph
		},
		Extra: map[string][]byte{
			"GSUB": fonttest.GSUBTable(lookups,
				[]fonttest.Feature{{Tag: "rphf", Lookups: []int{0}}},
				map[string]fonttest.Script{"java": fonttest.AllFeatures(1)}),
		},
	})
	f, err := Load(data)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	return f
}

// TestARephaTheFontMadeIsMovedAfterTheBase.
//
// The feature ran and the glyph changed, and nothing recorded that what it
// produced was a repha — so the reordering, which looks for one at the head of
// the cluster, found an ordinary consonant and left the mark in front of the
// letter it belongs to. On the page that is the mark of the whole syllable
// drawn on the wrong side of it.
func TestARephaTheFontMadeIsMovedAfterTheBase(t *testing.T) {
	f := javaRephaFace(t)
	got := gidsOf(t, f, string([]rune{javaRA, javaPangkn, javaKA}))
	want := []int{gidJavaKA, gidJavaRepha}
	if !equalInts(got, want) {
		t.Errorf("RA pangkon KA is %v, want %v: the repha the font made is drawn "+
			"before the base rather than after it", got, want)
	}
}

// TestARephaIsOnlyMadeAtTheHeadOfTheCluster is the other half. The feature is
// offered the first three glyphs of the cluster and no more: a rule matching
// further in is matching a letter in the middle of a syllable, and a letter in
// the middle of a syllable is not what a repha is.
func TestARephaIsOnlyMadeAtTheHeadOfTheCluster(t *testing.T) {
	f := javaRephaFace(t)
	// KA pangkon KA pangkon RA pangkon KA: one cluster, with the RA that the
	// font has a repha for at its fifth glyph.
	s := string([]rune{javaKA, javaPangkn, javaKA, javaPangkn, javaRA, javaPangkn, javaKA})
	got := gidsOf(t, f, s)
	want := []int{
		gidJavaKA, gidJavaPangkon, gidJavaKA, gidJavaPangkon,
		gidJavaRA, gidJavaPangkon, gidJavaKA,
	}
	if !equalInts(got, want) {
		t.Errorf("a RA deep in a cluster is %v, want %v: 'rphf' was applied past "+
			"the head of the cluster and made a repha out of a medial letter", got, want)
	}
}

func equalInts(a, b []int) bool {
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
