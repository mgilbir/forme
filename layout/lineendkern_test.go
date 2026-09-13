package layout

import (
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A kern against a glyph on the next line is not charged to this one.
//
// §8.1's boundary shaping hands every run the text either side of it, and a face
// that kerns then shrinks the run's last glyph against the first character of
// what follows. That is right while the two are next to each other. Once a line
// break has come between them they are not adjacent, and there is nothing on
// this line for the shortened advance to make room for — the same sentence §8.2
// states for letter-spacing and §8.1 for its own gap.
//
// unkernLineEnd gives the amount back, and for a word broken *inside* by
// overflow-wrap it silently gave back nothing. It measured the run twice, once
// with the following context and once without, and took the difference; but a
// run of a merge group has a string that is not its own, and rebuilding one from
// the run's text and its merge neighbours gives a string the group never had.
// "A" with "ATAR" after it rebuilt the group as "AATAR", which holds no "V" — so
// both measurements were of the same thing and the difference was zero.
//
// What it looked like was two spellings of one document disagreeing: "AVATAR"
// and "<span>AV</span><span>ATAR</span>", broken one letter to a line, put the
// first line at 654 and 613. Two thirds of a pixel, and exactly the AV kern.
//
// FuzzBoundaryLines is what found it and cannot hold it. That target compares
// the two spellings against each other, and both now go through the same
// correction — so a fault in the correction moves both alike and it sees
// nothing. Three plants against it passed. The width itself has to be named,
// which is what this does.

// notoNamed is the bundled face under a family name a stylesheet can ask for.
func notoNamed(t *testing.T) (FontSet, *shape.Face) {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	return namedFaceSet{family: "T", face: face, standard: StandardFonts()}, face
}

func TestAKernAgainstTheNextLineIsNotCharged(t *testing.T) {
	set, face := notoNamed(t)
	size, _ := style.FromPx(16)

	// The premise: this face kerns A against V, and by enough to see. Without
	// it every assertion below is satisfied by a face that does not kern at all.
	lone := face.ShapeGroup("A", "", "", false, shape.Features{})
	kerned := face.ShapeGroup("A", "", "V", true, shape.Features{})
	if len(lone) != 1 || len(kerned) != 1 {
		t.Fatalf("the face shaped %d and %d glyphs for one letter", len(lone), len(kerned))
	}
	if kerned[0].XAdvance >= lone[0].XAdvance {
		t.Fatalf("the face advances %v for A alone and %v for A before V; it "+
			"does not kern the pair, so nothing below is a test of anything",
			lone[0].XAdvance, kerned[0].XAdvance)
	}

	// A's own advance, which is what a line holding nothing but an A is worth.
	want := paragraph.NewBreaker(nil).Measure(face, "A", size)

	const sheet = `#d { font-family: T; font-size: 16px; overflow-wrap: break-word }`
	for _, markup := range []string{
		"AVATAR",
		`<span>AV</span><span>ATAR</span>`,
		`<span>A</span><span>VATAR</span>`,
		`<span>AVATA</span><span>R</span>`,
	} {
		t.Run(markup, func(t *testing.T) {
			// Ten pixels: narrower than any letter, so the word breaks at every
			// character and the first line holds the A alone.
			lines, ok := linesOfSpanned(t, set, markup, sheet, 10)
			if !ok || len(lines) == 0 {
				t.Fatal("the document set no lines")
			}
			if lines[0].Text != "A" {
				t.Fatalf("the first line reads %q, want the A alone", lines[0].Text)
			}
			if lines[0].Width != want {
				t.Errorf("the line holding one A is %v wide, want %v — the A's "+
					"own advance. The V is on the next line, so the pair that "+
					"would have shortened this advance is not adjacent to it "+
					"and the kern is not this line's to pay", lines[0].Width, want)
			}
		})
	}
}

// The same rule the other way: a kern *inside* a line is still charged.
//
// The correction is for the far edge of the last run on a line and for nothing
// else. A version that took the kerning off every run, or off a run that has
// more of its own word beside it, would make every kerned pair in a paragraph a
// fraction wider — which is a document that no longer kerns.
func TestAKernInsideALineIsStillCharged(t *testing.T) {
	set, face := notoNamed(t)
	size, _ := style.FromPx(16)

	a := paragraph.NewBreaker(nil).Measure(face, "A", size)
	v := paragraph.NewBreaker(nil).Measure(face, "V", size)
	both := paragraph.NewBreaker(nil).Measure(face, "AV", size)
	if both >= a.Add(v) {
		t.Fatalf("AV measures %v and the two letters apart %v; the face does "+
			"not kern them together", both, a.Add(v))
	}

	const sheet = `#d { font-family: T; font-size: 16px; overflow-wrap: break-word }`
	for _, markup := range []string{"AV", `<span>A</span><span>V</span>`} {
		t.Run(markup, func(t *testing.T) {
			// Wide enough for the pair to stay together.
			lines, ok := linesOfSpanned(t, set, markup, sheet, 200)
			if !ok || len(lines) != 1 {
				t.Fatalf("the document set %d lines, want 1", len(lines))
			}
			if lines[0].Width != both {
				t.Errorf("the line reads %q at %v, want %v — the kerned pair. "+
					"%v is the two letters measured apart", lines[0].Text,
					lines[0].Width, both, a.Add(v))
			}
		})
	}
}
