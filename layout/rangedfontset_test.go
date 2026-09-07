package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// RangedFontSet is the interface a caller implements to say "this family is for
// these characters and that one is for those" — what an @font-face
// unicode-range descriptor says inside a document, offered to a set that holds
// its faces some other way.
//
// It was never asked. The one gate in front of the whole interface read the
// engine's *own* set to find out whether any family in the box's list carried a
// range, and answered false for every implementation but that one — so a
// caller's set could implement RangedFontSet, be recognised as one, and never
// have FaceForFamily called.

// twoScriptSet is a caller's own set: a Latin family and a Hebrew one, each
// answering only for the characters it is for.
type twoScriptSet struct {
	standard FontSet
	hebrew   *shape.Face
	asked    []string // the (family, text) pairs FaceForFamily was called with
}

func (s *twoScriptSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if strings.EqualFold(strings.TrimSpace(family), "Ancient") {
		return s.hebrew, true
	}
	return s.standard.Face(family, bold, italic)
}

// FaceForFamily is the ranged answer: "Modern" is for ASCII and "Ancient" for
// everything else, which is the shape of a document that names a Latin webfont
// and a Hebrew one.
func (s *twoScriptSet) FaceForFamily(family, text string, bold, italic bool) (*shape.Face, bool) {
	s.asked = append(s.asked, family+"/"+text)
	ascii := true
	for _, r := range text {
		if r > 0x7F {
			ascii = false
		}
	}
	switch {
	case strings.EqualFold(strings.TrimSpace(family), "Modern") && ascii:
		return s.standard.Face("Helvetica", bold, italic)
	case strings.EqualFold(strings.TrimSpace(family), "Ancient") && !ascii:
		return s.hebrew, true
	}
	return nil, false
}

// TestACallersRangedFontSetIsAsked.
func TestACallersRangedFontSetIsAsked(t *testing.T) {
	set := &twoScriptSet{standard: StandardFonts(), hebrew: loadHebrew(t)}
	// "Modern" resolves first and is for the Latin; the Hebrew is what only
	// "Ancient" is for, so setting it at all means the second family was asked.
	frag, _ := layoutWith(t, set, `<p id="p">ab שלום</p>`,
		`#p { font-family: Modern, Ancient; font-size: 20px }`)
	if len(set.asked) == 0 {
		t.Fatal("FaceForFamily was never called; a caller's RangedFontSet is not consulted")
	}
	var latin, hebrew int
	for _, line := range find(t, frag, "p").Lines {
		for _, r := range line.Runs {
			if strings.TrimSpace(r.Text) == "" {
				continue
			}
			if r.Face != nil && r.Face.Name() == set.hebrew.Name() {
				hebrew++
				continue
			}
			latin++
		}
	}
	if hebrew == 0 {
		t.Errorf("the Hebrew was not set in the family declared for it; the set was "+
			"asked %v", set.asked)
	}
	if latin == 0 {
		t.Errorf("the Latin was not set in the family declared for it; the set was "+
			"asked %v", set.asked)
	}
}

// TestARangedSetThatRestrictsNothingChangesNothing is the control on the other
// side of the gate. The walk is per grapheme cluster, so it must be invisible
// where the set answers the same thing for every one of them: a document set in
// a caller's ranged set that restricts nothing has to come out exactly as the
// same document set in a plain set.
func TestARangedSetThatRestrictsNothingChangesNothing(t *testing.T) {
	plain, _ := layoutWith(t, StandardFonts(), `<p id="p">abc def</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	set := &unrestrictedSet{standard: StandardFonts()}
	ranged, _ := layoutWith(t, set, `<p id="p">abc def</p>`,
		`#p { font-family: Helvetica; font-size: 20px }`)
	if len(set.asked) == 0 {
		t.Fatal("FaceForFamily was never called, so this proves nothing about the walk")
	}
	a, b := rangedRunTexts(t, plain, "p"), rangedRunTexts(t, ranged, "p")
	if strings.Join(a, "|") != strings.Join(b, "|") {
		t.Errorf("a set that restricts nothing divided the text as %v, and a plain "+
			"set as %v; the per-cluster walk is not invisible", b, a)
	}
}

// unrestrictedSet is a RangedFontSet that declines every question, which is
// what a set holding no ranges looks like from here.
type unrestrictedSet struct {
	standard FontSet
	asked    []string
}

func (s *unrestrictedSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	return s.standard.Face(family, bold, italic)
}

func (s *unrestrictedSet) FaceForFamily(family, text string, bold, italic bool) (*shape.Face, bool) {
	s.asked = append(s.asked, family+"/"+text)
	return nil, false
}

// rangedRunTexts is the text of each run of an element's lines, which is how the
// division into runs is compared.
func rangedRunTexts(t *testing.T, frag *Fragment, id string) []string {
	t.Helper()
	var out []string
	for _, line := range find(t, frag, id).Lines {
		for _, r := range line.Runs {
			out = append(out, r.Text)
		}
	}
	return out
}
