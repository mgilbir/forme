package layout

import (
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The pair kerning at the end of a line, which is a pair that is not there.
//
// §8.1's boundary shaping gives every run the text either side of it, and a face
// that kerns then shrinks a run's last glyph against the first of the run after
// it. That is right while the two are next to each other and wrong once a line
// break has come between them: two characters on different lines are not
// adjacent, which is the sentence §8.2 states for letter-spacing and §8.1 for
// its own gap.
//
// What changes is the advance and not the ink, so an inline box's background is
// where it shows: the suite's hanging-punctuation-inline-bound-001 wraps three
// lines of Japanese in a <span> with a background, and the first line's box came
// out 1.8px short — one kern at 60px.

// kernLines lays out CJK text narrow enough to wrap and returns the runs of each
// line.
func kernLines(t *testing.T, faces []*shape.Face, text, css string) [][]TextRun {
	t.Helper()
	built := Build(Input{HTML: `<div id="d" lang="ja">` + text + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + `#d { font-family: monospace; ` + css + ` }`}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(1000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	var out [][]TextRun
	for _, ln := range linesOf(t, frag, "d") {
		out = append(out, ln.Runs)
	}
	return out
}

// TestALineEndTakesNoKernFromTheNextLine, and the same pair on one line keeps
// it. Two fixtures over one text, so what differs between them is the break and
// nothing else.
func TestALineEndTakesNoKernFromTheNextLine(t *testing.T) {
	faces := kernFaces(t)
	const size = 60
	face, ok := faceWithGlyphFor(faces, "くて")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	kern := kernOf(face, "く", "て", size)
	if kern == 0 {
		t.Skip("this face does not kern the fixture")
	}
	alone, _ := style.FromPx(face.MeasureShaped("く", size))
	kerned, _ := style.FromPx(face.MeasureShaped("く", size) + kern)

	// Wide enough for both: the two are adjacent and the pair is kerned.
	wide := kernLines(t, faces, "くて", `font-size: 60px; width: 400px`)
	if len(wide) != 1 {
		t.Fatalf("the wide fixture came out on %d lines", len(wide))
	}
	if got := wide[0][0].Width; got != kerned {
		t.Errorf("on one line the run is %v wide and the kerned pair is %v",
			got, kerned)
	}

	// Narrow enough to break between them: they are on different lines, so
	// there is no pair.
	narrow := kernLines(t, faces, "くて", `font-size: 60px; width: 70px`)
	if len(narrow) != 2 {
		t.Fatalf("the narrow fixture came out on %d lines, want 2", len(narrow))
	}
	if got := narrow[0][0].Width; got != alone {
		t.Errorf("at the end of a line the run is %v wide and the character alone "+
			"is %v; what it was kerned against is on the next line", got, alone)
	}
}

// TestOnlyTheLastRunOfALineLosesItsKern is the containment half: the rule is
// about the pair a line break separates, and every other pair on the line is
// exactly what §8.1 made it.
func TestOnlyTheLastRunOfALineLosesItsKern(t *testing.T) {
	faces := kernFaces(t)
	const size = 60
	face, ok := faceWithGlyphFor(faces, "くて")
	if !ok {
		t.Skip("no face here can set the fixture")
	}
	kern := kernOf(face, "く", "て", size)
	if kern == 0 {
		t.Skip("this face does not kern the fixture")
	}
	// Three characters in room for two: the pair inside the first line keeps
	// its kern and the one the break separates does not.
	lines := kernLines(t, faces, "くてく", `font-size: 60px; width: 130px`)
	if len(lines) != 2 || len(lines[0]) != 2 {
		t.Fatalf("the fixture came out as %d line(s) with %d run(s) on the first",
			len(lines), len(lines[0]))
	}
	kerned, _ := style.FromPx(face.MeasureShaped("く", size) + kern)
	if got := lines[0][0].Width; got != kerned {
		t.Errorf("the first run of the line is %v wide and the kerned pair is %v; "+
			"the character it is kerned against is beside it", got, kerned)
	}
	alone, _ := style.FromPx(face.MeasureShaped("て", size))
	if got := lines[0][1].Width; got != alone {
		t.Errorf("the last run of the line is %v wide and the character alone is %v",
			got, alone)
	}
}
