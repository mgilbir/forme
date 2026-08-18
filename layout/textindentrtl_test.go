package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §16.1's indent is measured from the line's *start* edge.
//
// "Text-indent: specifies the indentation of the first line of text in a block
// container" — the *first* line, from where its text begins, and in a
// right-to-left block that is the right edge. The engine shifted every indented
// line to the right, so a right-to-left first line was pushed away from the edge
// it should have been pushed in from.
//
// The room the line has for text is shortened by the indent either way — the
// line box still spans the band and only its content starts further in — so on a
// right-to-left line the alignment has already put the content an indent short
// of the right edge. Adding the shift again moved it by twice what the author
// wrote, and in the wrong direction.

// firstLineStartsAt is where the first run of a block's text was drawn.
func firstLineStartsAt(t *testing.T, css string) style.Unit {
	t.Helper()
	for _, op := range paintOf(t, `<div id="d">XXXXX</div>`,
		`#d { font-family: Courier; font-size: 20px; width: 400px; `+css+` }`) {
		if v, ok := op.(DrawText); ok {
			return v.At.X
		}
	}
	t.Fatalf("no text for %q", css)
	return 0
}

// TestAnIndentIsMeasuredFromTheStartEdge is the rule, stated as the symmetry it
// is: the same indent moves a left-to-right line in from the left and a
// right-to-left line in from the right, by the same distance.
func TestAnIndentIsMeasuredFromTheStartEdge(t *testing.T) {
	const indent = `text-indent: 60px`
	ltrPlain := firstLineStartsAt(t, `direction: ltr`)
	ltrIndented := firstLineStartsAt(t, `direction: ltr; `+indent)
	if got := ltrIndented.Sub(ltrPlain); got != bgpx(60) {
		t.Errorf("a left-to-right line moved %v, want 60 — in from the left", got)
	}
	rtlPlain := firstLineStartsAt(t, `direction: rtl`)
	rtlIndented := firstLineStartsAt(t, `direction: rtl; `+indent)
	if got := rtlIndented.Sub(rtlPlain); got != bgpx(-60) {
		t.Errorf("a right-to-left line moved %v, want -60 — in from the right", got)
	}
}

// TestOnlyTheFirstLineMoves, which is what the property is named for, and is the
// half a fixture of one line cannot see.
func TestOnlyTheFirstLineMoves(t *testing.T) {
	starts := func(css string) []style.Unit {
		var out []style.Unit
		seen := map[style.Unit]bool{}
		for _, op := range paintOf(t, `<div id="d">XXXXX XXXXX</div>`,
			`#d { font-family: Courier; font-size: 20px; width: 130px; `+css+` }`) {
			if v, ok := op.(DrawText); ok && !seen[v.At.Y] {
				seen[v.At.Y] = true
				out = append(out, v.At.X)
			}
		}
		return out
	}
	for _, dir := range []string{"ltr", "rtl"} {
		plain := starts(`direction: ` + dir)
		indented := starts(`direction: ` + dir + `; text-indent: 40px`)
		if len(plain) != 2 || len(indented) != 2 {
			t.Fatalf("direction: %s: %d and %d lines, want two of each",
				dir, len(plain), len(indented))
		}
		if indented[0] == plain[0] {
			t.Errorf("direction: %s: the first line did not move", dir)
		}
		if indented[1] != plain[1] {
			t.Errorf("direction: %s: the second line moved from %v to %v; the indent "+
				"is the first line's alone", dir, plain[1], indented[1])
		}
	}
}

// TestAnIndentedLineStillEndsAtTheEdgeItIsAlignedTo is the containment case, and
// it is the reason the indent is taken off the room rather than off the box.
//
// A right-aligned first line in a left-to-right block still ends at the right
// edge: the indent shortens what the line may hold, not where it finishes. The
// mirror holds in a right-to-left block for a line aligned left.
func TestAnIndentedLineStillEndsAtTheEdgeItIsAlignedTo(t *testing.T) {
	end := func(css string) style.Unit {
		var last style.Unit
		var width style.Unit
		for _, op := range paintOf(t, `<div id="d">XXXXX</div>`,
			`#d { font-family: Courier; font-size: 20px; width: 400px; `+css+` }`) {
			if v, ok := op.(DrawText); ok {
				last = v.At.X
				w, _ := style.FromPx(v.Face.Measure(v.Text, v.Size.Px()))
				width = w
			}
		}
		return last.Add(width)
	}
	if got, want := end(`direction: ltr; text-align: right; text-indent: 60px`),
		end(`direction: ltr; text-align: right`); got != want {
		t.Errorf("a right-aligned indented line ends at %v and an unindented one at "+
			"%v; the indent shortens the room, not the line box", got, want)
	}
	if got, want := firstLineStartsAt(t, `direction: rtl; text-align: left; text-indent: 60px`),
		firstLineStartsAt(t, `direction: rtl; text-align: left`); got != want {
		t.Errorf("a left-aligned line in a right-to-left block starts at %v indented "+
			"and %v not", got, want)
	}
}

// TestTheIndentFollowsTheLinesOwnDirection. The shift is decided per line by the
// base direction the line came out with, which is what makes a block of mixed
// content behave: it is the same question the alignment asks, and asking it a
// second way here would let the two disagree.
func TestTheIndentFollowsTheLinesOwnDirection(t *testing.T) {
	// A block that says nothing about direction, holding right-to-left text:
	// the line's own base direction is right-to-left, so the indent comes off
	// the right.
	plain := firstLineStartsAt(t, `unicode-bidi: plaintext`)
	_ = plain
	got := func(css string) style.Unit {
		for _, op := range paintOf(t, `<div id="d">שלום</div>`,
			`#d { font-size: 20px; width: 400px; unicode-bidi: plaintext; `+css+` }`) {
			if v, ok := op.(DrawText); ok {
				return v.At.X
			}
		}
		t.Fatalf("no text")
		return 0
	}
	if moved := got(`text-indent: 60px`).Sub(got(``)); moved != bgpx(-60) {
		t.Errorf("a right-to-left line in a block with no direction of its own moved "+
			"%v, want -60", moved)
	}
}
