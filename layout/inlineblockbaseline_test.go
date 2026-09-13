package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §10.8.1: an inline-block whose overflow is not visible takes its baseline from
// its bottom margin edge, and not from its last line box.
//
//	The baseline of an 'inline-block' is the baseline of its last line box in
//	the normal flow, unless it has either no in-flow line boxes or if its
//	'overflow' property has a computed value other than 'visible', in which
//	case the baseline is the bottom margin edge.
//
// CSS 2.2 REC and css-inline-3 both say that, and every browser does it. This
// engine did something else: it took the *higher* of the two candidates, which
// is the reading four vendored CSS 2.1 tests assert in their own words —
// inline-block-baseline-003 through -006, "the higher of either its bottom
// margin edge or the baseline of its last line box". Those four fail in
// browsers; vertical-align-baseline-005a, which asserts the plain rule, passes.
//
// The difference is visible and it is not small. An inline-block with a bottom
// margin and a line inside it sat on that line's baseline under the old reading,
// so a word beside it lined up with the text *inside* the box rather than with
// the bottom of the box — and "overflow: hidden", a declaration about clipping,
// changed nothing at all about where the box sat.

// baselineUnder lays out one inline-block beside a word and returns where the
// line's baseline fell and how tall the inline-block's margin box is.
func baselineUnder(t *testing.T, overflow string) (baseline, marginBox style.Unit) {
	t.Helper()
	root := layoutOf(t, 600,
		`<div id="d">x<span id="ib">y</span></div>`,
		noDefaults+`#d { font-family: Courier; font-size: 20px; line-height: 1 }
		 #ib { display: inline-block; overflow: `+overflow+`; width: 40px;
		       height: 40px; margin-bottom: 90px; font-size: 20px; line-height: 1 }`)
	return baselineOfFirstRun(t, root, "d"), find(t, root, "ib").MarginRect().H
}

func TestAnInlineBlockThatClipsSitsOnItsBottomMarginEdge(t *testing.T) {
	// Every value that is not "visible" — the rule is about the computed value
	// and not about scrolling, so "hidden" is as much a part of it as "scroll".
	for _, overflow := range []string{"hidden", "auto", "scroll", "clip"} {
		t.Run(overflow, func(t *testing.T) {
			got, marginBox := baselineUnder(t, overflow)
			if got != marginBox {
				t.Errorf("the line's baseline is at %v and the inline-block's "+
					"margin box is %v tall; §10.8.1 puts the baseline at the "+
					"bottom margin edge, which is the whole of that box", got, marginBox)
			}
		})
	}
}

// The other half, which is what says the rule above is conditional rather than
// the only thing the engine does.
//
// With the overflow visible the baseline is the inner line box's, so the box
// hangs below the text beside it. A version that put every inline-block on its
// bottom margin edge would pass the test above and break every ordinary one.
func TestAnInlineBlockThatDoesNotClipSitsOnItsLastLine(t *testing.T) {
	got, marginBox := baselineUnder(t, "visible")
	if got >= marginBox {
		t.Fatalf("the baseline is at %v and the margin box is %v tall; with the "+
			"overflow visible it comes from the last line box inside, which is "+
			"above the bottom of the box", got, marginBox)
	}

	// And the two readings really do differ here, or neither test above says
	// anything: the fixture has to be one where the line box baseline and the
	// bottom margin edge are not the same number.
	clipped, _ := baselineUnder(t, "hidden")
	if clipped == got {
		t.Errorf("clipping moved the baseline nowhere — it is %v either way, so "+
			"this fixture cannot tell the two rules apart", got)
	}
}
