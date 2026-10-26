package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// §8.2's trailing letter-spacing hangs off the end of a right-to-left line the
// same way it hangs off a left-to-right one.
//
// The fill discounts it from the line's *measure* — "a line's measure ends at
// its last glyph", as the comment beside that subtraction puts it, and without
// the discount a centred or right-aligned line of tracked text sits a tracking
// width off. What it must not also do is *move* the line by it.
//
// A right-to-left line is shifted by everything discounted from its measure,
// because a right-to-left line ends at its left and what was discounted hangs
// off that edge: a trailing space does, and so does §8.4's hanging punctuation.
// The tracking does not. A run is drawn from its own origin accumulating
// advances with the spacing added after each glyph, so the one after the last
// glyph drawn is at the run's **right** edge whatever the run's direction —
// letterspacingboundary.go establishes that against the display list rather than
// reasoning about it. Including it in the shift discounted it and then handed it
// straight back, which cancelled the fix for exactly the lines that needed it.
//
// The invariant is a symmetry rather than an absolute, so it does not restate
// the arithmetic it is checking: the tracking hangs past the right edge of a
// right-aligned left-to-right line, and it must hang past the right edge of a
// right-to-left line by the same amount. Under the old shift the left-to-right
// line overhung by the tracking and the right-to-left one by nothing.
//
// The suite measures the same thing at letter-spacing-bidi-003, whose "dir=rtl"
// line was displaced by exactly 1ch while the left-to-right line above it was
// correct glyph for glyph.

// farEdgeOf is the rightmost X+Width over a line's runs, which is where the
// content reaches including anything hanging past the block's edge.
func farEdgeOf(t *testing.T, markup, css string) style.Unit {
	t.Helper()
	root := layoutOf(t, 4000, `<div id="p">`+markup+`</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 3000px }`+css)
	runs := runsOf(t, root, "p")
	if len(runs) == 0 {
		t.Fatalf("%q produced no runs", markup)
	}
	edge := runs[0].X.Add(runs[0].Width)
	for _, r := range runs[1:] {
		if e := r.X.Add(r.Width); e > edge {
			edge = e
		}
	}
	return edge
}

func TestTheTrailingTrackingHangsOffEitherDirectionsEnd(t *testing.T) {
	const tracking = `letter-spacing: 20px`
	for _, tc := range []struct {
		markup, ltr, rtl, what string
	}{
		{"abc", `text-align: right`, `direction: rtl`,
			"left-to-right text"},
		{"&#x5d0;&#x5d1;&#x5d2;", `text-align: right`, `direction: rtl`,
			"right-to-left text"},
	} {
		ltrPlain := farEdgeOf(t, tc.markup, `#p {`+tc.ltr+`}`)
		ltrTracked := farEdgeOf(t, tc.markup, `#p {`+tc.ltr+`;`+tracking+`}`)
		rtlPlain := farEdgeOf(t, tc.markup, `#p {`+tc.rtl+`}`)
		rtlTracked := farEdgeOf(t, tc.markup, `#p {`+tc.rtl+`;`+tracking+`}`)

		ltrHang := ltrTracked.Sub(ltrPlain)
		rtlHang := rtlTracked.Sub(rtlPlain)
		if ltrHang == 0 {
			t.Fatalf("%s: the tracking hangs off nothing even left-to-right, so "+
				"this measures the wrong thing", tc.what)
		}
		if ltrHang != rtlHang {
			t.Errorf("%s: the tracking hangs %v past a right-aligned "+
				"left-to-right line and %v past a right-to-left one — the same "+
				"spacing sits at the same edge whatever the direction",
				tc.what, ltrHang, rtlHang)
		}
	}
}

// TestAnAlignedLineIsSetByTheSpacingThatHangs is the setting of a line held to
// the account its measuring already keeps: what hangs past the right of a line
// is the letter-spacing after the letter drawn furthest right, and nothing else.
//
// The fill and the intrinsic pass find that letter with paragraph.Rightmost and
// ask paragraph.TrailingSpacing what follows it. The alignment asked a question
// of its own — the declared letter-spacing of the *logically last* run — and
// the two answers part in three ways, each of which set the line off its edge:
//
//   - A run of a cursive script takes no spacing at all (§8.2), so nothing hangs
//     after it. Under "letter-spacing: 2px" a right-aligned line ending in
//     Arabic was set two pixels past the edge of its box, and a centred one a
//     pixel right of centre.
//   - A run of nothing but a formatting character draws no letter. The line
//     "&#x61C;<a>ع</a>" was set by the mark's declared 2px, and so was
//     "x&#x61C;<a>ع</a>", which is drawn x, ع, mark.
//   - The last run of a right-to-left line is its leftmost. "אב<span
//     letter-spacing:10px>cd</span>" under "direction: rtl" draws the Hebrew at
//     the right with 2px after it, and was set by the span's 10px instead:
//     eight pixels past the right edge of its box.
//
// The oracle is the display list and not the function under test: a run's ink
// is its extent less the letter-spacing it was drawn with after its last glyph,
// which is TextRun.LetterSpacing and is already nothing for a cursive run. A
// right-aligned line, and a right-to-left one at its start, puts its ink exactly
// at the right edge; a centred one puts it as far from the left as from the
// right. The two controls are shapes the old walk already got right, and are
// here so that a change which moves every line cannot pass.
func TestAnAlignedLineIsSetByTheSpacingThatHangs(t *testing.T) {
	const ten = `<span style="letter-spacing: 10px">`
	for _, tc := range []struct {
		markup, css, why string
	}{
		{"ع", `text-align: right`, "a cursive letter alone"},
		{"ع", `text-align: center`, "a cursive letter alone, centred"},
		{"ع", `direction: rtl`, "a cursive letter alone, right to left"},
		{"abc علم", `text-align: right`, "Latin, then an Arabic word"},
		{"abc علم", `text-align: center`, "Latin, then an Arabic word, centred"},
		{"؜<a>ع</a>", `text-align: right`, "a letter mark, then a cursive letter"},
		{"x؜<a>ع</a>", `text-align: right`,
			"x, a letter mark and a cursive letter, drawn x, letter, mark"},
		{"x<b>؜</b><a>ع</a>", `text-align: right`,
			"the same with the mark in an element of its own"},
		// A mark in front of letters that do take spacing. It is drawn to their
		// right and draws nothing, so the Hebrew's 2px is what hangs: a walk that
		// let the mark hold the place hung none and set the line 2px short.
		{"‏<a>אב</a>", `text-align: right`,
			"a right-to-left mark, then Hebrew"},
		{"אב" + ten + "cd</span>", `direction: rtl`,
			"a right-to-left line whose last run is its leftmost"},
		{"ab" + ten + "אב</span>", `direction: rtl`,
			"the same with the directions the other way round"},
		// The controls.
		{"abc", `text-align: right`, "plain Latin"},
		{"ab" + ten + "אב</span>", `text-align: right`,
			"a left-to-right line ending in a right-to-left run"},
	} {
		root := layoutOf(t, 600, `<div id="p">`+tc.markup+`</div>`, noDefaults+
			`#p { font-family: Courier; font-size: 20px; width: 300px; `+
			`letter-spacing: 2px; `+tc.css+` }`)
		f := find(t, root, "p")
		if len(f.Lines) != 1 {
			t.Fatalf("%s: %d lines, want 1", tc.why, len(f.Lines))
		}
		line := f.Lines[0]
		var left, right style.Unit
		has := false
		for _, r := range line.Runs {
			if r.Width == 0 {
				// A formatting character, which draws nothing to be aligned.
				continue
			}
			l, e := r.X, r.X.Add(r.Width).Sub(r.LetterSpacing)
			if !has || l < left {
				left = l
			}
			if !has || e > right {
				right = e
			}
			has = true
		}
		if !has {
			t.Fatalf("%s: the line draws nothing", tc.why)
		}
		if tc.css == `text-align: center` {
			if got, want := left, line.Rect.W.Sub(right); got != want {
				t.Errorf("%s: ink from %v to %v on a line %v wide, which is %v from "+
					"the left and %v from the right", tc.why, left, right,
					line.Rect.W, got, want)
			}
			continue
		}
		if right != line.Rect.W {
			t.Errorf("%s: the ink ends at %v on a line %v wide — the spacing that "+
				"hangs is the one after the letter drawn furthest right", tc.why,
				right, line.Rect.W)
		}
	}
}
