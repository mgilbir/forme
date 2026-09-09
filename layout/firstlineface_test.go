package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §5.12.1's first line and the face its runs are actually set in.
//
// Restyling the first line re-asks the box for its font, because a ::first-line
// rule may change the family or the size and the runs have to be measured in
// what they will be drawn in. What it must not do is force the box's declared
// face back onto a run the *fallback* found another face for: the declared
// family never set those characters — that is why the fallback ran — and a run
// handed a face with no glyphs for it is drawn as a row of notdefs and measured
// against metrics that belong to some other script.
//
// The height goes the same way. §10.8.1 measures leading against "the font",
// and a run the declared family could not set is not in that font, which is what
// leadingInFace exists for. The restyled path asked l.leading — the box's own
// face — so a Japanese run on a restyled first line was given the leading of the
// Latin face beside it, and the line came out as much too short as the two faces
// differ.

// firstLineRuns is the runs of a block's first line, restyled by a ::first-line
// rule, with the fallback library behind the standard faces.
func firstLineRuns(t *testing.T, css, text string) []TextRun {
	t.Helper()
	faces := fallbackLibrary(t)
	built := Build(Input{HTML: `<div id="d">` + text + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + css}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(2000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	lines := linesOf(t, frag, "d")
	if len(lines) == 0 {
		t.Fatal("the block made no lines")
	}
	return lines[0].Runs
}

// TestAFirstLineKeepsAFallbackFace.
//
// The block asks for Courier, which has no ideographs; the fallback library
// does. A ::first-line rule that changes only the size must leave that run in
// the face that can set it.
func TestAFirstLineKeepsAFallbackFace(t *testing.T) {
	const css = `#d { font-family: Courier; font-size: 20px }
		#d::first-line { font-size: 200% }`

	var fell bool
	for _, r := range firstLineRuns(t, css, "漢x") {
		if r.Face == nil {
			continue
		}
		if r.Text == "漢" {
			fell = true
			if r.Face.Name() == "Courier" {
				t.Errorf("the ideograph on the restyled first line is set in "+
					"Courier, which has no glyph for it; the fallback found %q "+
					"for the same characters and a ::first-line rule about the "+
					"size does not take that back", r.Face.Name())
			}
		}
	}
	if !fell {
		t.Skip("no run of the ideograph alone, so the fallback did not split it out")
	}
}

// TestAFirstLineMeasuresLeadingAgainstTheRunsOwnFace is the height half.
//
// Two blocks of the same text: one with a ::first-line rule that changes only
// the colour, one with none at all. The rule says nothing about type, so the
// line is set in the same faces at the same sizes and has to come out the same
// height — and it does not if the restyled runs were measured against the box's
// declared face instead of the fallback's.
func TestAFirstLineMeasuresLeadingAgainstTheRunsOwnFace(t *testing.T) {
	const base = `#d { font-family: Courier; font-size: 20px }`

	plain := firstLineRuns(t, base, "漢x")
	styled := firstLineRuns(t, base+` #d::first-line { color: green }`, "漢x")
	if len(plain) != len(styled) {
		t.Fatalf("the two blocks made %d and %d runs, so they are not the same "+
			"line and the heights below compare nothing", len(plain), len(styled))
	}
	if got, want := heightOfFirstLine(t, base+` #d::first-line { color: green }`, "漢x"),
		heightOfFirstLine(t, base, "漢x"); got != want {
		t.Errorf("with a ::first-line rule the line is %v tall and without one "+
			"%v; the rule changes only the colour, and how far a run reaches "+
			"from the baseline is the face's to say", got, want)
	}
}

// heightOfFirstLine is how tall a block's first line box is.
func heightOfFirstLine(t *testing.T, css, text string) style.Unit {
	t.Helper()
	faces := fallbackLibrary(t)
	built := Build(Input{HTML: `<div id="d">` + text + `</div>`,
		CSS: []Stylesheet{{Source: noDefaults + css}}})
	w, _ := style.FromPx(2000)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h},
		suiteFonts{standard: StandardFonts(), fallback: faces}, NewRecorder(nil))
	lines := linesOf(t, frag, "d")
	if len(lines) == 0 {
		t.Fatal("the block made no lines")
	}
	return lines[0].Rect.H
}
