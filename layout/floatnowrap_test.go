package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A float met part-way along a line that cannot break.
//
// §9.5.1 shifts a float down "if there is not enough horizontal room", and the
// room is what the line does not take. Ordinarily that is the band less what the
// line held when the float was reached, because everything after that point can
// go on the next line and leave the float where it is. A line that cannot break
// is the case that is not ordinary: nothing on it can go anywhere else, so it
// takes the whole band and more, and a float placed beside it is drawn over the
// words.
//
// float-nowrap-7 and -8 are the suite's pair and both name float-nowrap-1 as
// their reference: the same nowrap line and the same float, written once in the
// middle of the text and once after it, which have to draw the same page.
//
// float-nowrap-4 is the other side of it and is why the rule asks about the
// break rather than about the overflow. It is float-nowrap-8 with "nowrap" moved
// from the block to the span, so there *is* an opportunity in front of the
// float — the line can end at it and the float can begin the next one — and the
// float stays where it was written. float-nowrap-3-ref asserts the difference
// outright, declaring rel=mismatch against float-nowrap-4.

// floatTop is the y of the one float a fixture paints, which is the whole of
// what these ask about.
func floatTop(t *testing.T, htmlSrc, cssSrc string) style.Unit {
	t.Helper()
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if f, ok := op.(FillRect); ok && f.Color.B > 0.5 && f.Color.R < 0.5 {
			return f.Rect.Y
		}
	}
	t.Fatalf("the fixture painted no float: %s", htmlSrc)
	return 0
}

const floatNowrapCSS = `div { width: 10ch; font-family: monospace }
	.f { float: right; width: 5ch; height: 5ch; background: blue }
	.nowrap { white-space: nowrap }`

func TestAFloatOnANowrapLineGoesBelowIt(t *testing.T) {
	// The line cannot break anywhere, so it takes the whole band and the float
	// has to clear it. Written in the middle of the text and written after it,
	// the two have to agree — which is what the suite asserts by giving them one
	// reference.
	mid := floatTop(t, `<div class=nowrap>Some <span class=f></span> text that `+
		`overflows my parent.</div>`, floatNowrapCSS)
	end := floatTop(t, `<div class=nowrap>Some text that overflows my parent.`+
		`<span class=f></span></div>`, floatNowrapCSS)
	if mid != end {
		t.Errorf("a float written in the middle of a nowrap line is at y=%v and "+
			"the same float written after it at y=%v; neither fits beside a line "+
			"that takes the whole band, so both clear it", mid, end)
	}
	// And below rather than beside: the line is on the first line of the block.
	top := floatTop(t, `<div class=nowrap><span class=f></span>x</div>`, floatNowrapCSS)
	if mid <= top {
		t.Errorf("the float is at y=%v, no lower than a float that starts its "+
			"own line at y=%v", mid, top)
	}
}

func TestAFloatStaysWhereTheLineCanBreakBeforeIt(t *testing.T) {
	// The same document with "nowrap" on the span rather than on the block.
	// There is an opportunity in front of the float, so the line can end there
	// and the float needs no room beside the text that follows.
	got := floatTop(t, `<div>Some <span class=nowrap><span class=f></span> text `+
		`that overflows my parent.</span></div>`, floatNowrapCSS)
	below := floatTop(t, `<div>Some <span class=nowrap>text that overflows my `+
		`parent.</span> <span class=f></span></div>`, floatNowrapCSS)
	if got >= below {
		t.Errorf("a float in front of an unbreakable span is at y=%v and one "+
			"after it at y=%v; the first has an opportunity before it and stays "+
			"on the line, the second does not and clears it", got, below)
	}
}

func TestAFloatBesideALineThatFitsIsUnmoved(t *testing.T) {
	// The ordinary case, which none of this may disturb: a short line leaves
	// room beside it and the float sits on it.
	short := floatTop(t, `<div class=nowrap>ab<span class=f></span></div>`, floatNowrapCSS)
	alone := floatTop(t, `<div class=nowrap><span class=f></span></div>`, floatNowrapCSS)
	if short != alone {
		t.Errorf("a float after two characters of a ten-character band is at "+
			"y=%v and one on an empty line at y=%v; there is room for it beside "+
			"the text and it does not move", short, alone)
	}
}

func TestRoomBesideALine(t *testing.T) {
	band := mustPx(100)
	fits := []inlineItem{{Text: "ab", Width: mustPx(20)}}
	over := []inlineItem{
		{Text: "ab", Width: mustPx(20), NoWrap: true},
		{Text: " ", Width: mustPx(10), Space: true, NoWrap: true},
		{Text: "cd", Width: mustPx(200), NoWrap: true},
	}
	breakable := []inlineItem{
		{Text: "ab", Width: mustPx(20)},
		{Text: " ", Width: mustPx(10), Space: true},
		{Text: "cd", Width: mustPx(200), NoWrap: true},
	}
	for _, tc := range []struct {
		name string
		runs []inlineItem
		used style.Unit
		want style.Unit
	}{
		{"a line that fits", fits, mustPx(20), mustPx(80)},
		{"a line that cannot break", over, mustPx(20), mustPx(100 - 230)},
		{"a line that can break before the float", breakable, mustPx(20), mustPx(80)},
		{"a float that starts the line", over, 0, mustPx(100 - 230)},
	} {
		if got := roomBeside(tc.runs, band, tc.used); got != tc.want {
			t.Errorf("%s: roomBeside = %v, want %v", tc.name, got, tc.want)
		}
	}
}
