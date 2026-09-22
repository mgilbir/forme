package layout

import (
	"fmt"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// TestAClusterWhoseHalvesLiveInDifferentFacesIsSplit is the emoji every
// document with a skin tone in it contains.
//
// A face is chosen per grapheme cluster, and the question asked is whether one
// face can set the whole of it. "⛹🏿" is one cluster of two characters that no
// single face here has: Noto Sans Symbols has the person, Unifont Upper has the
// skin tone modifier, and neither has both. The answer was therefore "no face",
// which left the cluster with the primary — a face that has *neither* — so both
// characters came out as nothing, in a document the engine has faces for.
//
// Each half alone finds its face. It is only together that they find none, which
// is what makes this worth splitting: the cluster is already being drawn as
// blanks, so setting it in two faces cannot be worse and is what a browser does.
func TestAClusterWhoseHalvesLiveInDifferentFacesIsSplit(t *testing.T) {
	set := fallbackFontSet(t)
	primary, ok := set.Face("serif", false, false)
	if !ok || primary == nil {
		t.Fatal("the standard serif face is not in the set")
	}
	l := &layouter{fontSet: set}
	b := &Box{}

	// Each half on its own, to show the faces are there to be found. Without
	// this the test below could pass on a set that has nothing at all.
	for _, half := range []string{"⛹", "\U0001F3FF"} {
		runs := l.faceRunsFor(b, primary, half)
		if len(runs) != 1 || runs[0].Face == nil {
			t.Fatalf("%q alone gave %d runs; the halves have to be findable for "+
				"the pair to be worth splitting", half, len(runs))
		}
		for _, r := range half {
			if _, has := runs[0].Face.GlyphID(r); !has {
				t.Fatalf("%q alone went to %s, which has no glyph for U+%04X",
					half, runs[0].Face.Name(), r)
			}
		}
	}

	const both = "⛹\U0001F3FF"
	runs := l.faceRunsFor(b, primary, both)

	var missing []string
	for _, r := range runs {
		if r.Face == nil {
			missing = append(missing, fmt.Sprintf("%q has no face", r.Text))
			continue
		}
		for _, ru := range r.Text {
			if isDefaultIgnorable(ru) {
				continue
			}
			if _, has := r.Face.GlyphID(ru); !has {
				missing = append(missing, fmt.Sprintf("U+%04X is not in %s",
					ru, r.Face.Name()))
			}
		}
	}
	if len(missing) > 0 {
		t.Errorf("%q was set as %d run(s) and %v; each half has a face and the "+
			"cluster has to be cut so that each half reaches it",
			both, len(runs), missing)
	}

	// The whole sequence the suite writes — person, skin tone, joiner, female
	// sign, variation selector — which is what line-breaking-013 and -014 are
	// made of. The cut must not fall on the joiner: a character that sets no
	// paper decides no face and starts no stretch, or the sequence is cut into
	// pieces at the very places that hold it together, and a face is chosen for
	// a joiner alone.
	const sequence = "\u26F9\U0001F3FF\u200D\u2640\uFE0F"
	for _, r := range l.faceRunsFor(b, primary, sequence) {
		if r.Text == "" {
			t.Errorf("%q produced an empty run", sequence)
			continue
		}
		if drawsNoPaper(r.Text) {
			t.Errorf("%q was cut so that %q is a run of its own; a character that "+
				"sets no paper stays with the stretch it follows rather than "+
				"being given a face of its own", sequence, r.Text)
		}
	}
}

// noFaceSet answers "no face" to everything, which is the set a cluster nobody
// can set is being asked about.
type noFaceSet struct{}

func (noFaceSet) Face(string, bool, bool) (*shape.Face, bool)    { return nil, false }
func (noFaceSet) FaceFor(string, bool, bool) (*shape.Face, bool) { return nil, false }

// TestAClusterNoFaceCanSetIsLeftWhole is the other half of the rule, and the
// half no document reaches: the fallback library here covers nearly all of
// Unicode, so a cluster with no face at all is not something the corpus has.
//
// It is left whole, and by the rule that is already there rather than by one
// written for it: every character that finds no face keeps the primary, so the
// stretches are all the same face and merge back into one. A separate "did any
// part find a face" test was written here first and was dead — it cannot be
// false while there are two stretches to cut between.
//
// Which is worth a test of its own, because the reasoning is the only thing
// holding it up: if a part that finds nothing is ever given some other face, a
// cluster nobody can set starts being cut into pieces that each report the same
// missing glyph.
func TestAClusterNoFaceCanSetIsLeftWhole(t *testing.T) {
	if runs := clusterFaceRuns(noFaceSet{}, "⛹\U0001F3FF", nil, false, false); runs != nil {
		t.Errorf("a cluster no face can set was cut into %d runs; it is left whole, "+
			"because cutting it reports the same missing glyphs several times and "+
			"changes nothing about what is drawn", len(runs))
	}
}
