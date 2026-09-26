package layout

import (
	"strings"
	"testing"
)

// fit-content(), which the gate refused.
//
// CSS Grid 2 §7.2 defines a fit-content(<length-percentage>) track as
// max(minimum, min(limit, max-content)): "auto" at the low end, and at the high
// end the content's max-content size held to the argument. §12.5 gets there
// by treating it as "max-content" except in three places — a growth limit it
// accommodates is clamped by the argument; the argument is the limit a
// distribution of extra space freezes it at; and past its limits it takes
// space as a max-content track only until it reaches the argument, and then as
// a fixed track of that size. It is also not an automatic track, so §12.8
// stretches it no further.
//
// Courier at 20px, so every letter is 12px and every line 20px.

// TestAFitContentTrackIsItsContentHeldToTheLimit is §7.2's formula, a clause
// at a time.
func TestAFitContentTrackIsItsContentHeldToTheLimit(t *testing.T) {
	two := func(first string) string {
		return `<div id="g"><div>` + first + `</div><div>a</div></div>`
	}
	for _, c := range []struct {
		what, first, css string
		want             [][4]float64
	}{
		{
			// Content narrower than the limit: the track is the content, 12px,
			// and the automatic track beside it is stretched over the rest,
			// which fit-content() is not.
			"content under the limit", "a",
			`#g { width: 300px; grid-template-columns: fit-content(100px) auto }`,
			[][4]float64{{0, 0, 12, 20}, {12, 0, 288, 20}},
		},
		{
			// Content wider than the limit: 180px of words, the longest 36.
			// The track is the limit, 100, and the words wrap in it onto two
			// lines, "aaa aaa" being 84 and "aaa aaa aaa" 132.
			"content over the limit", "aaa aaa aaa aaa",
			`#g { width: 300px; grid-template-columns: fit-content(100px) auto }`,
			[][4]float64{{0, 0, 100, 40}, {100, 0, 200, 40}},
		},
		{
			// The minimum wins over the limit: a 120px word in fit-content(50px)
			// is 120, and §6.6's note says the argument does not clamp the
			// content-based minimum the way a fixed maximum does.
			"a word longer than the limit", "aaaaaaaaaa",
			`#g { width: 300px; grid-template-columns: fit-content(50px) auto }`,
			[][4]float64{{0, 0, 120, 20}, {120, 0, 180, 20}},
		},
		{
			// A percentage is of the container's width: 50% of 300 is 150, and
			// the 180px of words wrap in it after "aaa aaa aaa".
			"a percentage limit", "aaa aaa aaa aaa",
			`#g { width: 300px; grid-template-columns: fit-content(50%) auto }`,
			[][4]float64{{0, 0, 150, 40}, {150, 0, 150, 40}},
		},
		{
			// Inside a repeat(), which is a spelling of the list.
			"in a repeat()", "aaa aaa aaa aaa",
			`#g { width: 300px; grid-template-columns: repeat(2, fit-content(100px)) }`,
			[][4]float64{{0, 0, 100, 40}, {100, 0, 12, 40}},
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			arrangedWithoutFinding(t, two(c.first), c.css)
			wantCells(t, gridCells(t, two(c.first), c.css), c.want, c.what)
		})
	}
}

// TestASpanningItemGrowsFitContentTracksOnlyToTheirLimits is §12.5.1. An item
// of 180px of words across two fit-content(50px) tracks asks their growth
// limits for 180 between them; each is frozen at 50, and what is left goes
// past the limits only to a track still below its argument, of which there is
// none — so the tracks are 50 and 50 and the item is 100 wide, its words on
// two lines. Treated as max-content maximums throughout, the tracks came out
// 90 and 90.
func TestASpanningItemGrowsFitContentTracksOnlyToTheirLimits(t *testing.T) {
	const doc = `<div id="g"><div style="grid-column: span 2">aaa aaa aaa aaa</div></div>`
	css := `#g { width: 300px; grid-template-columns: fit-content(50px) fit-content(50px) }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css), [][4]float64{{0, 0, 100, 40}}, "a span across two limits")

	// Beside an automatic track the space past the limits has somewhere to
	// go: the automatic track takes it all — 180 less the 50 the fit-content
	// track holds is 130 — and is then stretched over the rest of the room.
	css = `#g { width: 300px; grid-template-columns: fit-content(50px) auto }`
	got := gridCells(t, doc, css)
	if got[0].W.Px() != 300 {
		t.Errorf("a span across fit-content(50px) and auto is %v wide, want 300", got[0].W.Px())
	}
	measured := fragmentFor(layoutOf(t, 1000, `<div id="f" style="float: left">`+doc+`</div>`,
		gridCSS+`#g { grid-template-columns: fit-content(50px) auto }`), "f")
	if measured == nil || measured.BorderRect.W.Px() != 180 {
		t.Errorf("a float round the span across fit-content(50px) and auto: %v, want 180 wide",
			measured)
	}
}

// TestAFitContentRowIsHeldToItsLimit. The rows are sized by the same algorithm:
// an item three lines deep, 60px, whose minimum contribution is nothing —
// "min-height: 0" — makes a fit-content(30px) row 30 high, where "auto" would
// be 60. The row after it begins at 30. grid-auto-rows is the same list for
// the rows nobody drew.
func TestAFitContentRowIsHeldToItsLimit(t *testing.T) {
	const doc = `<div id="g"><div style="min-height: 0">a<br>b<br>c</div><div>d</div></div>`
	for _, css := range []string{
		`#g { width: 300px; grid-template-rows: fit-content(30px) auto }`,
		`#g { width: 300px; grid-auto-rows: fit-content(30px) }`,
	} {
		arrangedWithoutFinding(t, doc, css)
		wantCells(t, gridCells(t, doc, css),
			[][4]float64{{0, 0, 300, 30}, {0, 30, 300, 20}}, css)
	}
	// With its automatic minimum the item asks for its whole height, and the
	// row is 60: the minimum is the formula's first term.
	css := `#g { width: 300px; grid-template-rows: fit-content(30px) auto }`
	got := gridCells(t, strings.Replace(doc, ` style="min-height: 0"`, "", 1), css)
	wantCells(t, got, [][4]float64{{0, 0, 300, 60}, {0, 60, 300, 20}}, "an automatic minimum")
}

// TestAGridOfFitContentTracksIsMeasuredToTheLimit. A float round a grid asks it
// how wide it would like to be, and a fit-content(50px) column holding 132px
// of words would like to be 50: under a max-content constraint §12.5 grows an
// "auto" minimum to the item's limited max-content contribution, which the
// argument limits. Under a min-content one it is the longest word, 36.
func TestAGridOfFitContentTracksIsMeasuredToTheLimit(t *testing.T) {
	width := func(float string) float64 {
		f := fragmentFor(layoutOf(t, 1000, `<div id="f" style="`+float+`"><div id="g">`+
			`<div>aaa aaa aaa</div></div></div>`,
			gridCSS+`#g { grid-template-columns: fit-content(50px) }`), "f")
		if f == nil {
			t.Fatalf("no fragment for the float")
		}
		return f.BorderRect.W.Px()
	}
	if got := width("float: left"); got != 50 {
		t.Errorf("a float round fit-content(50px) is %v wide, want 50", got)
	}
	if got := width("float: left; width: min-content"); got != 36 {
		t.Errorf("a min-content float round fit-content(50px) is %v wide, want 36", got)
	}
}

// TestBelowItsArgumentAFitContentTrackTakesWhatIsPastTheLimits. §12.5.1's
// "distribute space beyond limits" gives what an item still needs, once every
// track it spans is at its limit, to the tracks with an intrinsic maximum — and
// a fit-content() maximum is one until the track reaches its argument.
//
// "aa" sets the first track's growth limit at 24. A 240px word then spans it
// and a track of at most 20px: the second takes its 20, the first is at its
// limit of 24, and the 196 still needed goes to the first alone, which is
// below its argument of 100 and so still intrinsic: 220 and 20. Read as a
// fixed track at every size, it had no claim before the second, and the two
// shared the 196, 122 and 118.
func TestBelowItsArgumentAFitContentTrackTakesWhatIsPastTheLimits(t *testing.T) {
	doc := `<div id="g"><div>aa</div><div style="grid-column: span 2">` +
		strings.Repeat("a", 20) + `</div></div>`
	css := `#g { width: 300px; grid-template-columns: fit-content(100px) minmax(auto, 20px) }`
	arrangedWithoutFinding(t, doc, css)
	wantCells(t, gridCells(t, doc, css),
		[][4]float64{{0, 0, 220, 20}, {0, 20, 240, 20}}, "a word past the limits")
}
