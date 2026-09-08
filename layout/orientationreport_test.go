package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A text-orientation this engine does not apply, and saying so.
//
// A turned subtree has one orientation here: refusesToTurn asks the box that
// changes the writing mode which way its characters face, and a line needing
// characters upright and characters along it at once is the one thing a quarter
// turn cannot draw — so such a box is not turned at all rather than half turned.
// That is a real limit, and it was a silent one. A "text-orientation: upright"
// on a div inside a vertical container did nothing and said nothing, which is
// the class of failure the finding vocabulary exists for.

// orientationFindings is what a document says about text-orientation.
func orientationFindings(t *testing.T, markup, css string) []string {
	t.Helper()
	built := Build(Input{HTML: markup, CSS: []Stylesheet{{Source: noDefaults + css}}})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, nil, rec)
	var out []string
	for _, f := range append(append([]Finding{}, built.Findings...), rec.Findings()...) {
		if strings.Contains(f.Message, "text-orientation") {
			out = append(out, f.Message)
		}
	}
	return out
}

// TestAnOrientationOnADescendantIsReported.
func TestAnOrientationOnADescendantIsReported(t *testing.T) {
	got := orientationFindings(t,
		`<div id="v"><div id="u">国X国</div></div>`,
		`#v { writing-mode: vertical-rl } #u { text-orientation: upright }`)
	if len(got) != 1 {
		t.Fatalf("the document said %d things about text-orientation, want one: %v", len(got), got)
	}
	if !strings.Contains(got[0], "upright") {
		t.Errorf("the finding does not name the value that was not applied: %q", got[0])
	}
}

// TestAnOrientationThatMatchesTheTurnIsNotReported. A document that writes the
// property on every box in a subtree is told once, about the box that decided —
// telling it again about each box that agrees would bury that one.
func TestAnOrientationThatMatchesTheTurnIsNotReported(t *testing.T) {
	got := orientationFindings(t,
		`<div id="v"><div id="u">国国国</div></div>`,
		`#v { writing-mode: vertical-rl; text-orientation: upright }
		 #u { text-orientation: upright }`)
	if len(got) != 0 {
		t.Errorf("a descendant asking for the orientation the turn already has "+
			"was reported: %v", got)
	}
}

// TestAnOrientationInASidewaysModeIsNotReported is §5.1's rule and not this
// engine's: the property has no effect in a horizontal typographic mode, and
// both sideways modes are one. Reporting a declaration CSS itself ignores would
// be reporting CSS.
//
// The suite says it in a document's own comment — text-autospace-004 writes
// text-orientation inside a "sideways-lr" container and notes that it has no
// effect there — and that document renders correctly, so a finding on it would
// be a claim about nothing.
func TestAnOrientationInASidewaysModeIsNotReported(t *testing.T) {
	for _, mode := range []string{"sideways-lr", "sideways-rl"} {
		got := orientationFindings(t,
			`<div id="v"><div id="u">国X国</div></div>`,
			`#v { writing-mode: `+mode+` } #u { text-orientation: upright }`)
		if len(got) != 0 {
			t.Errorf("in %s a text-orientation was reported: %v", mode, got)
		}
	}
}

// TestAnOrientationInAHorizontalModeIsNotReported, for the same reason and the
// commoner case: a document that sets the property without ever turning
// anything is one CSS ignores.
func TestAnOrientationInAHorizontalModeIsNotReported(t *testing.T) {
	got := orientationFindings(t, `<div id="u">国X国</div>`,
		`#u { text-orientation: upright }`)
	if len(got) != 0 {
		t.Errorf("a text-orientation with nothing turned was reported: %v", got)
	}
}
