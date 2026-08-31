package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// A line may not end after nothing but an inline box's leading margin.
//
// The rule is already written down in this engine — insetItems says "a line may
// end before <span style='margin-left: 99px'>word</span>, and it may not end
// between that margin and the word it pushes along" — and the rewind that
// implements it needs an opportunity to go back to. Where the margin *begins*
// the line there is none: nothing precedes it, so no opportunity was recorded,
// and the line ended between the margin and the box it opens. The margin stayed
// on the first line with nothing beside it and the box began the second without
// the margin that was supposed to push it along.
//
// text-indent-overflow's reference is the case: a "margin-left: 200px" span
// holding a 50px inline-block, in a container exactly 200px wide. The pair
// overflows together and the box begins 200px in.

// greenAt is where a fixture paints its one green box.
func greenAt(t *testing.T, htmlSrc, cssSrc string) (x, y style.Unit) {
	t.Helper()
	for _, op := range paintOf(t, htmlSrc, cssSrc) {
		if f, ok := op.(FillRect); ok && f.Color.G > 0.4 && f.Color.R < 0.2 {
			return f.Rect.X, f.Rect.Y
		}
	}
	t.Fatalf("the fixture painted no green box: %s", htmlSrc)
	return 0, 0
}

const leadingMarginCSS = `.c { width: 200px; margin: 0 }
	.b { display: inline-block; width: 50px; height: 20px; background: green }`

func TestALeadingMarginKeepsTheBoxItOpens(t *testing.T) {
	// The margin is as wide as the whole line, so the pair cannot fit. It
	// overflows together rather than being split across two lines.
	x, y := greenAt(t, `<div class=c><span style="margin-left:200px"><span class=b></span></span></div>`,
		leadingMarginCSS)
	fits, fitsY := greenAt(t, `<div class=c><span style="margin-left:100px"><span class=b></span></span></div>`,
		leadingMarginCSS)
	if y != fitsY {
		t.Errorf("the box is on a line at y=%v where a margin it fits beside puts "+
			"it at y=%v; a line does not end between a margin and the box it "+
			"opens", y, fitsY)
	}
	// And it is where the margin put it: a hundred pixels further along than the
	// one whose margin is a hundred pixels narrower.
	if want := fits.Add(mustPx(100)); x != want {
		t.Errorf("the box is at x=%v, want %v — the margin that did not fit is "+
			"still the margin it has", x, want)
	}
}

func TestALineStillEndsWhereItHasContent(t *testing.T) {
	// The rule is about a line with nothing but the margin on it. Where a word
	// precedes the box, the line may end before the box — and the rewind takes
	// the margin along with it, which is the case that already worked.
	_, withText := greenAt(t, `<div class=c>x<span style="margin-left:200px"><span class=b></span></span></div>`,
		leadingMarginCSS)
	_, alone := greenAt(t, `<div class=c><span style="margin-left:200px"><span class=b></span></span></div>`,
		leadingMarginCSS)
	if withText <= alone {
		t.Errorf("with a word in front of it the box is at y=%v and without one "+
			"at y=%v; the word gives the line something to end after", withText, alone)
	}
}

func TestAnOrdinaryOverflowStillBreaks(t *testing.T) {
	// Nothing here may stop a line ending at a real opportunity: two words too
	// wide for the line still make two lines.
	var ys []style.Unit
	for _, op := range paintOf(t, `<div style="width:40px;font-size:16px">aaa bbb</div>`, "") {
		if v, ok := op.(DrawText); ok && v.Text != " " {
			ys = append(ys, v.At.Y)
		}
	}
	if len(ys) < 2 || ys[0] == ys[len(ys)-1] {
		t.Errorf("the words are all on one line at %v; a line still ends at a "+
			"space it cannot hold past", ys)
	}
}
