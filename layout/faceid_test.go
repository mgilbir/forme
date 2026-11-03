package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
)

// A document's faces are its own — a clone of the caller's, an instance cut
// for it, a font its @font-face rule loaded — so the same font comes out as a
// new *shape.Face in every document. shape.Face.ID is what stays the same, and
// these say it does through each of the three.

// textFaces is the faces a display list draws text in, in order.
func textFaces(ops []Op) []*shape.Face {
	var out []*shape.Face
	for _, op := range ops {
		if d, ok := op.(DrawText); ok {
			out = append(out, d.Face)
		}
	}
	return out
}

func TestAFaceKeepsItsIDFromDocumentToDocument(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	caller := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	res := mapResolver{"w.ttf": notosans.Regular()}
	for _, c := range []struct {
		name string
		in   Input
		// face is the face the text has to be set in for the case to be
		// the one it says, rather than a fallback.
		face string
	}{
		{"a face of the caller's", Input{Fonts: caller,
			HTML: `<p style="font-family: T">words</p>`}, "NotoSans-Regular"},
		{"an instance cut for the document", Input{Fonts: caller,
			HTML: `<p style="font-family: T; font-variation-settings: 'wght' 700">words</p>`}, "NotoSans-Bold"},
		{"a face the document's @font-face loaded", Input{Resources: res,
			HTML: `<style>@font-face { font-family: W; src: url(w.ttf) }</style>
				<p style="font-family: W">words</p>`}, "NotoSans-Regular"},
	} {
		first := textFaces(Compose(c.in, Options{}).Ops)
		second := textFaces(Compose(c.in, Options{}).Ops)
		if len(first) == 0 || len(first) != len(second) {
			t.Fatalf("%s: %d and %d runs of text, want the same, and some", c.name, len(first), len(second))
		}
		for i := range first {
			if first[i].Name() != c.face {
				t.Errorf("%s: run %d is set in %q, want %q", c.name, i, first[i].Name(), c.face)
			}
			if first[i] == second[i] {
				t.Errorf("%s: run %d is set in one *shape.Face in two documents; this test is about the case where it is not",
					c.name, i)
			}
			if first[i].ID() != second[i].ID() {
				t.Errorf("%s: run %d is set in a face whose ID differs between two documents of the same markup",
					c.name, i)
			}
		}
	}
}

func TestAnInstanceIsNotTheFaceItWasCutFrom(t *testing.T) {
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}
	regular := textFaces(Compose(Input{Fonts: set, HTML: `<p style="font-family: T">words</p>`}, Options{}).Ops)
	bold := textFaces(Compose(Input{Fonts: set,
		HTML: `<p style="font-family: T; font-variation-settings: 'wght' 700">words</p>`}, Options{}).Ops)
	if len(regular) != 1 || len(bold) != 1 {
		t.Fatalf("%d and %d runs, want one each", len(regular), len(bold))
	}
	if regular[0].ID() != face.ID() {
		t.Error("the caller's face, cloned for the document, does not have the caller's face's ID")
	}
	if bold[0].ID() == face.ID() {
		t.Error("an instance at wght 700 has the ID of the default instance")
	}
}
