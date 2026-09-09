package layout

import "testing"

// An inside marker's own characters, and the algorithm that orders them.
//
// §12.5.1 puts an inside marker "as the first inline box in the principal block
// box, before the element's content" — a box on the line. Its text is text, and
// in a right-to-left list the two characters of "1." are a European number and a
// common separator: UAX #9 resolves the separator to the paragraph's direction
// and puts the stop on the far side of the digit.
//
// The marker used to be kept out of the bidi paragraph altogether, to stop it
// joining the directional run of the text after it. That bought where the marker
// sits at the price of never resolving what is *in* it, and a right-to-left list
// numbered its items in an order nothing reads them in. The suite writes it as
// CSS2/lists/list-style-position-024, checked against the same two characters
// written as ordinary text.

// markerRuns is the runs of a list item's first line, in the order they are
// drawn.
func markerRuns(t *testing.T, css string) []TextRun {
	t.Helper()
	root := layoutOf(t, 500, `<ol id="o"><li id="d">x</li></ol>`, noDefaults+mono+css)
	var out []TextRun
	for _, line := range find(t, root, "d").Lines {
		out = append(out, line.Runs...)
	}
	return out
}

// TestAnInsideMarkerIsBidiResolved.
//
// Two runs and not one: the digit and the stop take different levels, so they
// cannot be one run, and the stop is drawn to the *left* of the digit because
// left is where a right-to-left paragraph puts what comes second.
func TestAnInsideMarkerIsBidiResolved(t *testing.T) {
	runs := markerRuns(t, `ol { direction: rtl } li { list-style-position: inside }`)
	if len(runs) != 3 {
		t.Fatalf("the line has %d runs, want three — the digit, the stop and the "+
			"item's own text: %v", len(runs), textsOfRuns(runs))
	}
	digit, stop := runs[0], runs[1]
	if digit.Text != "1" || stop.Text != "." {
		t.Fatalf("the marker came out as %q and %q, want \"1\" and \".\": a marker "+
			"kept out of the paragraph is one run of \"1.\" in logical order",
			digit.Text, stop.Text)
	}
	if !(stop.X < digit.X) {
		t.Errorf("the stop is at %v and the digit at %v; in a right-to-left "+
			"paragraph the separator after a number is resolved to the "+
			"paragraph's direction and is drawn on the far side of it",
			stop.X, digit.X)
	}
}

// TestAnInsideMarkerInALeftToRightListIsOneRun is the control, and it is the
// case every ordinary document is: nothing is reordered, so the two characters
// stay one run and the marker is where it always was.
func TestAnInsideMarkerInALeftToRightListIsOneRun(t *testing.T) {
	runs := markerRuns(t, `ol { direction: ltr } li { list-style-position: inside }`)
	if len(runs) != 2 || runs[0].Text != "1." {
		t.Errorf("the line is %v, want the marker as one run of \"1.\" followed "+
			"by the item's text", textsOfRuns(runs))
	}
	if runs[0].X != 0 {
		t.Errorf("the marker begins at %v, want the start of the line", runs[0].X)
	}
}

// textsOfRuns is what each run sets, for a failure message.
func textsOfRuns(runs []TextRun) []string {
	out := make([]string, 0, len(runs))
	for _, r := range runs {
		out = append(out, r.Text)
	}
	return out
}
