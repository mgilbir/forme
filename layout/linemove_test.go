package layout

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// What a line carries moves with it. A line box is moved after it was laid out
// by a table cell's vertical-align, by each of a multi-column pour's three
// copies, by absolutise and by the quarter turn — and what hangs off it in the
// block's coordinates rather than the line's has to move by the same amount.
// See LineFragment.placed.
//
// The geometry is inlinepaint_test.go's: Courier at 250px, so every expected
// rectangle is derived from the drawn text's position rather than recorded.

var (
	moveRed    = style.RGBA{R: 255, A: 1}
	moveGreen  = style.RGBA{G: 128, A: 1}
	movePurple = style.RGBA{R: 128, B: 128, A: 1}
)

// movedLine is one line holding each thing that can hang off a line or sit on
// it: a background (blue), a border (red, 10px), an inline-block with a
// background (green) and an underline (purple). Eleven characters and 20px of
// border, which fits a 12-character column and leaves no room for anything else.
const movedLine = `<span style="background: rgb(0, 0, 255)">ab</span> ` +
	`<span style="border: 10px solid rgb(255, 0, 0)">cd</span> ` +
	`<span style="display: inline-block; background: rgb(0, 128, 0)">ef</span> ` +
	`<span style="text-decoration: underline; color: rgb(128, 0, 128)">uv</span>`

// filler is a word that fills a 12-character column on its own.
const filler = "xxxxxxxxxxx"

// checkMovedLine asserts that each of movedLine's marks is where its text is.
func checkMovedLine(t *testing.T, what string, ops []Op) {
	t.Helper()
	adv, asc := mustPx(inkAdvance), mustPx(inkAscent)
	_, ab := drawnAt(t, ops, "ab")

	// The background is the content area of "ab".
	want := Rect{X: ab.X, Y: ab.Y.Sub(asc), W: 2 * adv, H: mustPx(inkHeight)}
	if got := inkOf(ops, blue); len(got) != 1 || got[0] != want {
		t.Errorf("%s: the background is at %v, and its text's box is %v", what, got, want)
	}

	// The border is 10px round the content area of "cd", which is a space
	// and 10px of the border after "ab".
	_, cd := drawnAt(t, ops, "cd")
	ten := mustPx(10)
	if want := ab.X.Add(3 * adv).Add(ten); cd.X != want || cd.Y != ab.Y {
		t.Errorf("%s: \"cd\" is drawn at %v, want (%v, %v)", what, cd, want, ab.Y)
	}
	red := inkOf(ops, moveRed)
	if len(red) == 0 {
		t.Errorf("%s: no border", what)
	} else {
		box := red[0]
		for _, r := range red[1:] {
			x0, y0 := style.Min(box.X, r.X), style.Min(box.Y, r.Y)
			box = Rect{X: x0, Y: y0,
				W: style.Max(box.Right(), r.Right()).Sub(x0),
				H: style.Max(box.Bottom(), r.Bottom()).Sub(y0)}
		}
		want := Rect{X: cd.X.Sub(ten), Y: cd.Y.Sub(asc).Sub(ten),
			W: (2 * adv).Add(2 * ten), H: mustPx(inkHeight).Add(2 * ten)}
		if box != want {
			t.Errorf("%s: the border spans %v, and it should surround its text at %v",
				what, box, want)
		}
	}

	// The inline-block is a box of its own on the line: its left edge is
	// where the line put it and its text sits on the line's baseline.
	_, ef := drawnAt(t, ops, "ef")
	if want := cd.X.Add(2 * adv).Add(ten).Add(adv); ef.X != want || ef.Y != ab.Y {
		t.Errorf("%s: the inline-block's text is drawn at %v, want (%v, %v)", what, ef, want, ab.Y)
	}
	if g := inkOf(ops, moveGreen); len(g) != 1 || g[0].X != ef.X || g[0].W != 2*adv ||
		ef.Y <= g[0].Y || ef.Y >= g[0].Bottom() {
		t.Errorf("%s: the inline-block's background is %v, and its text is at %v", what, g, ef)
	}

	// The underline is ruled under "uv", which is measured from the line.
	_, uv := drawnAt(t, ops, "uv")
	if u := inkOf(ops, movePurple); len(u) != 1 || u[0].X != uv.X || u[0].W != 2*adv ||
		u[0].Y <= uv.Y || u[0].Y >= uv.Y.Add(mustPx(inkDescent)) {
		t.Errorf("%s: the underline is %v, and its text is at %v", what, u, uv)
	}
}

// TestWhatALineCarriesMovesWithIt puts that line through every site that moves
// a line after it was laid out, and asks that each mark be over its words.
//
// The pour's cases put the line in its middle column, which split copies and
// moves up and across, and in its last, and as the content of a block the pour
// cuts, which materialise copies. Each column holds one line: the filler is a
// word as wide as a column.
func TestWhatALineCarriesMovesWithIt(t *testing.T) {
	pour := func(words ...string) string {
		return `<div style="columns: 3; column-gap: 0; width: 5400px">` +
			strings.Join(words, " ") + `</div>`
	}
	cut := func(words ...string) string {
		return `<div style="columns: 3; column-gap: 0; width: 5400px"><p>` +
			strings.Join(words, " ") + `</p></div>`
	}
	for _, tc := range []struct{ what, doc string }{
		{"a middle-aligned cell", `<table><tr><td style="height: 2000px; vertical-align: middle">` +
			movedLine + `</td></tr></table>`},
		{"a bottom-aligned cell", `<table><tr><td style="height: 2000px; vertical-align: bottom">` +
			movedLine + `</td></tr></table>`},
		// A cell aligned on its baseline with a neighbour whose first line
		// sits much lower.
		{"a baseline-aligned cell", `<table><tr><td style="line-height: 1500px">` + filler +
			`</td><td>` + movedLine + `</td></tr></table>`},
		{"the middle column of a pour", pour(filler, filler, filler, movedLine, filler)},
		{"the last column of a pour", pour(filler, filler, filler, filler, movedLine)},
		{"the middle column of a cut block", cut(filler, filler, filler, movedLine, filler)},
		{"the last column of a cut block", cut(filler, filler, filler, filler, movedLine)},
		// A pour refused part of the way through: the third column's end goes
		// through a line an inline-block makes a thousand pixels tall, after
		// the second has been cut from the rest and moved up. "A pour that is
		// refused changes nothing", and the content stands as it was laid
		// out — which it does not if the second column's copy moved what hangs
		// off the original's lines.
		{"a pour that is refused", `<div style="column-count: 1; width: 1800px; height: 800px">` +
			strings.Join([]string{filler, filler, movedLine, filler,
				`<span style="display: inline-block; width: 10px; height: 1000px"></span>`,
				filler}, " ") + `</div>`},
		{"a relatively positioned block", `<p style="position: relative; left: 70px; top: 30px">` +
			movedLine + `</p>`},
	} {
		root := layoutOf(t, 6000, tc.doc, courierInk)
		checkMovedLine(t, tc.what, Paint(root))
	}

	// Turned, the text runs down the page from where it is drawn, and a
	// background is the column its content area makes.
	ops := paintOf(t, `<div style="writing-mode: vertical-rl; height: 1000px; width: 1000px">`+
		`<span style="background: rgb(0, 0, 255)">ab</span></div>`, courierInk)
	_, ab := drawnAt(t, ops, "ab")
	if got := inkOf(ops, blue); len(got) != 1 || got[0].W != mustPx(inkHeight) ||
		got[0].H != mustPx(2*inkAdvance) || got[0].Y != ab.Y ||
		ab.X <= got[0].X || ab.X >= got[0].Right() {
		t.Errorf("turned: the background is at %v, and its text starts at %v", got, ab)
	}
}

// TestAPositionedInlineMovesWithItsCell: a positioned inline box's fragments
// are its absolutely positioned descendants' containing block, and they are
// recorded by identity. A cell's vertical-align that moved a copy of them, or
// did not move them, left the box it contains positioned against where the line
// used to be.
func TestAPositionedInlineMovesWithItsCell(t *testing.T) {
	ops := paintOf(t, `<table><tr><td style="height: 2000px; vertical-align: middle">`+
		`<span style="position: relative">ab<span style="position: absolute; top: 0; left: 0; `+
		`width: 10px; height: 10px; background: rgb(255, 0, 0)"></span></span></td></tr></table>`,
		courierInk)
	_, ab := drawnAt(t, ops, "ab")
	want := Rect{X: ab.X, Y: ab.Y.Sub(mustPx(inkAscent)), W: mustPx(10), H: mustPx(10)}
	if got := inkOf(ops, moveRed); len(got) != 1 || got[0] != want {
		t.Errorf("the box positioned in the moved span is at %v, want its corner %v", got, want)
	}
}

// TestEveryLineFieldIsPlacedOrRelative keeps LineFragment.placed whole. A field
// added to LineFragment is either measured from the line box, and moves with
// Rect for free, or is in the block's coordinates and has to be in placed —
// and a field this test does not know is a question someone has to answer
// before it is shipped, which is the point.
func TestEveryLineFieldIsPlacedOrRelative(t *testing.T) {
	relative := map[string]bool{
		"Rect": true, "Baseline": true, "Runs": true, "Sideways": true, "Anticlockwise": true,
	}
	var line LineFragment
	placed := map[uintptr]bool{}
	for _, list := range line.placed() {
		placed[reflect.ValueOf(list).Pointer()] = true
	}
	v := reflect.ValueOf(&line).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		inPlaced := placed[v.Field(i).Addr().Pointer()]
		switch {
		case relative[f.Name] && inPlaced:
			t.Errorf("LineFragment.%s is said to be measured from the line and is in placed", f.Name)
		case relative[f.Name]:
		case inPlaced:
		default:
			t.Errorf("LineFragment.%s is neither measured from the line box nor in "+
				"LineFragment.placed; say which, or a line that moves leaves it behind", f.Name)
		}
	}
	if len(placed) != len(line.placed()) {
		t.Errorf("LineFragment.placed names a list twice")
	}
}
