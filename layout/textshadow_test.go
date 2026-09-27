package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// text-shadow, CSS Text Decoration 3 §4.

func shadowsIn(ops []Op) []DrawTextShadow {
	var out []DrawTextShadow
	flat, _ := flattenGroups(ops, "")
	for _, op := range flat {
		if s, ok := op.(DrawTextShadow); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestAShadowIsTheRunMovedAndRecoloured: the offset, the colour — the text's
// when none is given — and a blur of half the radius.
func TestAShadowIsTheRunMovedAndRecoloured(t *testing.T) {
	ops := paintOf(t, `<p>word</p>`, noDefaults+`p { color: rgb(0, 0, 255); text-shadow: 3px 4px 6px, red -2px 1px }`)
	var run DrawText
	for _, op := range ops {
		if d, ok := op.(DrawText); ok && d.Text == "word" {
			run = d
		}
	}
	ss := shadowsIn(ops)
	if len(ss) != 2 {
		t.Fatalf("%d shadows, want 2: %v", len(ss), ops)
	}
	// The last shadow is painted first, so that the first is on top.
	second, first := ss[0], ss[1]
	if first.Run.At != (Point{X: run.At.X.Add(rpx(3)), Y: run.At.Y.Add(rpx(4))}) {
		t.Errorf("the first shadow is at %v, the run at %v", first.Run.At, run.At)
	}
	if first.Run.Color != (style.RGBA{B: 255, A: 1}) || first.StdDev != rpx(3) {
		t.Errorf("the first shadow is %v with a deviation of %v, want the text's blue and 3px", first.Run.Color, first.StdDev)
	}
	if second.Run.Color != red || second.StdDev != 0 ||
		second.Run.At != (Point{X: run.At.X.Sub(rpx(2)), Y: run.At.Y.Add(rpx(1))}) {
		t.Errorf("the second shadow is %+v", second)
	}
	if first.Run.Text != run.Text || first.Run.Face != run.Face || first.Run.Size != run.Size {
		t.Error("the shadow is not the run it shadows")
	}
}

// TestShadowsAreUnderTheTextAndItsDecorations is §5.1's order: shadows, then
// underlines and overlines, then the text, then line-throughs — and each shadow
// is that stack again, moved.
func TestShadowsAreUnderTheTextAndItsDecorations(t *testing.T) {
	ops := paintOf(t, `<p>word</p>`, noDefaults+`p { text-decoration: underline line-through;
		text-decoration-color: rgb(0, 128, 0); text-shadow: red 0 30px }`)
	var order []string
	for _, op := range ops {
		switch v := op.(type) {
		case DrawTextShadow:
			order = append(order, "shadow-text")
		case DrawText:
			order = append(order, "text")
		case FillRect:
			if v.Color == red {
				order = append(order, "shadow-line")
			} else if v.Color == green {
				order = append(order, "line")
			}
		}
	}
	want := "shadow-line shadow-text shadow-line line text line"
	if got := strings.Join(order, " "); got != want {
		t.Errorf("painted %s, want %s", got, want)
	}
}

// TestAShadowIsTheRunsOwn is the specification's example: the <span>'s green
// shadow overrides the <div>'s blue one, for the span's text and for the
// underline the div draws across it.
func TestAShadowIsTheRunsOwn(t *testing.T) {
	ops := paintOf(t, `<div>Help, help! <span>I am under a hat!</span></div>`, noDefaults+`
		div { color: black; font-size: 48px; text-decoration: underline; text-shadow: blue 0px 50px 0px }
		span { font-size: 20px; vertical-align: top; text-shadow: green 0px 100px 0px }`)
	for _, s := range shadowsIn(ops) {
		// The span's runs are the ones at its 20px.
		want := blue
		if s.Run.Size == rpx(20) {
			want = green
		}
		if s.Run.Color != want {
			t.Errorf("the shadow of %q is %v, want %v", s.Run.Text, s.Run.Color, want)
		}
	}
	lines := map[style.RGBA]int{}
	for _, op := range ops {
		if f, ok := op.(FillRect); ok {
			lines[f.Color]++
		}
	}
	if lines[blue] == 0 || lines[green] == 0 {
		t.Errorf("the underline's shadows are %v, want blue under the div's text and green under the span's", lines)
	}
}

// TestABlurredShadowOfALineIsAFilterGroup: a line's shadow is a rectangle, so
// it is a fill, blurred as a group of one.
func TestABlurredShadowOfALineIsAFilterGroup(t *testing.T) {
	ops := paintOf(t, `<p>word</p>`, noDefaults+`p { text-decoration: underline; text-shadow: red 2px 2px 8px }`)
	gs := filterGroupsOf(ops)
	if len(gs) != 1 || gs[0].Filters[0].StdDev != rpx(4) || len(gs[0].Ops) != 1 {
		t.Fatalf("the underline's blurred shadow is %+v", gs)
	}
	if f, ok := gs[0].Ops[0].(FillRect); !ok || f.Color != red || !f.Overhang {
		t.Errorf("the group holds %v, want the red line", gs[0].Ops)
	}
}

// TestAShadowOfWhatIsNotInkIsStillDrawn: transparent text casts its shadow —
// §4, "text shadows ... may show through if the text is partially-transparent"
// — and a transparent shadow is nothing.
func TestAShadowOfWhatIsNotInkIsStillDrawn(t *testing.T) {
	if n := len(shadowsIn(paintOf(t, `<p>word</p>`, noDefaults+`p { color: transparent; text-shadow: red 1px 1px }`))); n != 1 {
		t.Errorf("transparent text cast %d shadows, want 1", n)
	}
	if n := len(shadowsIn(paintOf(t, `<p>word</p>`, noDefaults+`p { text-shadow: transparent 1px 1px }`))); n != 0 {
		t.Errorf("a transparent shadow was drawn %d times", n)
	}
	// A line of a transparent colour is not drawn, and its shadow is: the
	// shadow is of the line's shape.
	ops := paintOf(t, `<p>word</p>`, noDefaults+`p { text-decoration: underline;
		text-decoration-color: transparent; text-shadow: red 1px 1px }`)
	lines := 0
	for _, op := range ops {
		if f, ok := op.(FillRect); ok {
			if f.Color.A == 0 {
				t.Error("a transparent line was painted")
			}
			if f.Color == red {
				lines++
			}
		}
	}
	if lines != 1 {
		t.Errorf("%d shadows of the transparent underline, want 1", lines)
	}
}

// TestAControlCharactersBoxCastsAShadow: the box drawn for a control character
// is its visible glyph, and is shadowed as one.
func TestAControlCharactersBoxCastsAShadow(t *testing.T) {
	ops := paintOf(t, "<p style=\"white-space: pre\">a\u0007b</p>", noDefaults+`p { text-shadow: red 5px 5px }`)
	n := 0
	for _, op := range ops {
		if f, ok := op.(FillRect); ok && f.Color == red {
			n++
		}
	}
	if n == 0 {
		t.Error("the control character's box cast no shadow")
	}
}

// TestTheFirstLineCastsItsOwnShadow: §5.12.1 lists text-shadow among what a
// ::first-line applies.
func TestTheFirstLineCastsItsOwnShadow(t *testing.T) {
	ops := paintOf(t, `<p>aaa bbb ccc</p>`, noDefaults+`p { width: 40px; font: 10px/20px monospace }
		p::first-line { text-shadow: red 1px 1px }`)
	var shadowed []string
	for _, s := range shadowsIn(ops) {
		shadowed = append(shadowed, strings.TrimSpace(s.Run.Text))
	}
	if strings.Join(shadowed, "") != "aaa" {
		t.Errorf("the shadowed runs are %q, want the first line's alone", shadowed)
	}
}

// TestTheShadowBoundFires lowers the bound on shadows per value.
func TestTheShadowBoundFires(t *testing.T) {
	was := maxTextShadows
	defer func() { maxTextShadows = was }()
	maxTextShadows = 2
	built := Build(Input{HTML: `<p>word</p>`, CSS: []Stylesheet{{Source: noDefaults +
		`p { text-shadow: red 1px 1px, blue 2px 2px, green 3px 3px }`}}})
	rec := NewRecorder(nil)
	ops := PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, rec), rec)
	ss := shadowsIn(ops)
	if len(ss) != 2 || ss[0].Run.Color != blue || ss[1].Run.Color != red {
		t.Errorf("the shadows drawn are %v, want the first two", ss)
	}
	if !reportedLimit(rec.Findings(), "drawn with its first 2") {
		t.Errorf("the bound fired silently: %v", rec.Findings())
	}
}

// TestAShadowIsClippedAndDimmedAsItsRunIs.
func TestAShadowIsClippedAndDimmedAsItsRunIs(t *testing.T) {
	ops := paintOf(t, `<div id="o"><p>word</p></div>`, noDefaults+`
		#o { width: 12px; overflow: hidden } p { text-shadow: red 1px 1px; opacity: 0.5 }`)
	ss := shadowsIn(ops)
	if len(ss) != 1 {
		t.Fatalf("%d shadows", len(ss))
	}
	if !ss[0].Run.Clip.Active || ss[0].Run.Clip.Rect.W != rpx(12) {
		t.Errorf("the shadow's clip is %v, want the 12px box", ss[0].Run.Clip)
	}
	if ss[0].Run.Color.A != 0.5 {
		t.Errorf("the shadow's alpha is %g, want 0.5", ss[0].Run.Color.A)
	}
	// Wholly outside the clip, it is not drawn.
	ops = paintOf(t, `<div id="o"><p>word</p></div>`, noDefaults+`
		#o { height: 40px; overflow: hidden } p { text-shadow: red 0 500px }`)
	if n := len(shadowsIn(ops)); n != 0 {
		t.Errorf("a shadow wholly outside its clip was drawn %d times", n)
	}
}

// TestPictureSeesAShadow: a sharp shadow is its glyphs in its colour, which is
// the same page as a run drawn there in that colour; a blurred one is a mark of
// its own, and two blurs are two.
func TestPictureSeesAShadow(t *testing.T) {
	run := picText("Test", 8, 29).(DrawText)
	moved := run
	moved.Color = picRed
	sharp := DrawTextShadow{Run: moved}
	if !pictureEqual([]Op{sharp}, []Op{moved}, picPage) {
		t.Error("a sharp shadow is not its glyphs")
	}
	blurred := DrawTextShadow{Run: moved, StdDev: picPx(2)}
	if pictureEqual([]Op{blurred}, []Op{moved}, picPage) {
		t.Error("a blurred shadow is its glyphs")
	}
	if pictureEqual([]Op{blurred}, []Op{DrawTextShadow{Run: moved, StdDev: picPx(3)}}, picPage) {
		t.Error("two blurs are one mark")
	}
}
