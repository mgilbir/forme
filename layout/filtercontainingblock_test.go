package layout

import (
	"strings"
	"testing"
)

// Filter Effects 1 §5: "A value other than none for the filter property
// results in the creation of a containing block for absolute and fixed
// positioned descendants unless the element it applies to is a document root
// element", and css-will-change 1 §3's will-change naming filter, which makes
// the same one. See containsAbsolutes.

// cbFragment lays a document out with no default margins on a page 600 wide
// and returns the fragment of the element with an id, from the boxes and from
// the lines: the first of an inline box's.
func cbFragment(t *testing.T, htmlSrc, cssSrc, id string) *Fragment {
	t.Helper()
	root := layoutOf(t, 600, htmlSrc, noDefaults+cssSrc)
	var found *Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if found != nil || f == nil {
			return
		}
		if f.Box != nil && attrIs(f.Box, id) && f.Box.Pseudo == "" {
			found = f
			return
		}
		for _, line := range f.Lines {
			for _, b := range line.Boxes {
				walk(b)
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("no fragment for #%s", id)
	}
	return found
}

// TestAFilterIsTheContainingBlockOfWhatIsPositionedInIt: an absolutely or a
// fixed positioned box at "top: 0; left: 0" inside a filtered box is at the
// filtered box's padding box, not the page's corner, and where it would be
// without the filter otherwise: a positioned box between is its containing
// block, and the root's filter makes none. A will-change naming filter is the
// same containing block, and one naming anything else is not.
func TestAFilterIsTheContainingBlockOfWhatIsPositionedInIt(t *testing.T) {
	const box = `#f { margin: 30px 0 0 50px; border: 2px solid; padding: 5px; height: 40px }
		#a { top: 0; left: 0; width: 10px; height: 10px }`
	for _, tc := range []struct {
		name, doc, css string
		x, y           float64
	}{
		{"absolute", `<div id="f"><div id="a"></div></div>`, `#f { filter: blur(1px) } #a { position: absolute }`, 52, 32},
		{"fixed", `<div id="f"><div id="a"></div></div>`, `#f { filter: blur(1px) } #a { position: fixed }`, 52, 32},
		{"a colour function", `<div id="f"><div id="a"></div></div>`, `#f { filter: brightness(100%) } #a { position: absolute }`, 52, 32},
		{"a url()", `<div id="f"><div id="a"></div></div>`, `#f { filter: url(#x) } #a { position: absolute }`, 52, 32},
		{"will-change", `<div id="f"><div id="a"></div></div>`, `#f { will-change: filter } #a { position: fixed }`, 52, 32},
		{"will-change in a list", `<div id="f"><div id="a"></div></div>`, `#f { will-change: opacity, FILTER } #a { position: absolute }`, 52, 32},
		{"deeper", `<div id="f"><p><span><div id="a"></div></span></p></div>`, `#f { filter: blur(1px) } #a { position: absolute }`, 52, 32},
		{"an inline-block", `<div><span id="f"><div id="a"></div></span></div>`,
			`#f { display: inline-block; width: 100px; filter: blur(1px) } #a { position: absolute }`, 52, 32},
		{"a flex item", `<div style="display: flex"><div id="f"><div id="a"></div></div></div>`,
			`#f { width: 100px; filter: blur(1px) } #a { position: fixed }`, 52, 32},
		// A cell has no margins: its padding box is inside its border.
		{"a table cell", `<div style="display: table"><div id="f" style="display: table-cell"><div id="a"></div></div></div>`,
			`#f { width: 100px; filter: blur(1px) } #a { position: absolute }`, 2, 2},
		{"no filter", `<div id="f"><div id="a"></div></div>`, `#a { position: absolute }`, 0, 0},
		{"no filter, fixed", `<div id="f"><div id="a"></div></div>`, `#a { position: fixed }`, 0, 0},
		{"filter none", `<div id="f"><div id="a"></div></div>`, `#f { filter: none } #a { position: absolute }`, 0, 0},
		{"will-change of something else", `<div id="f"><div id="a"></div></div>`, `#f { will-change: color } #a { position: absolute }`, 0, 0},
		{"a positioned box between", `<div id="f"><div id="p"><div id="a"></div></div></div>`,
			`#f { filter: blur(1px) } #p { position: relative; margin-left: 20px } #a { position: absolute }`, 77, 37},
		{"a positioned box between, fixed", `<div id="f"><div id="p"><div id="a"></div></div></div>`,
			`#f { filter: blur(1px) } #p { position: relative; margin-left: 20px } #a { position: fixed }`, 52, 32},
		{"a filter inside a positioned box", `<div id="p"><div id="f"><div id="a"></div></div></div>`,
			`#p { position: relative; margin-left: 20px } #f { filter: blur(1px) } #a { position: absolute }`, 72, 32},
		// The root's filter makes no containing block: the box is at the
		// page's corner, and not at the root's padding box 20 down and 40
		// across.
		{"the root's", `<div id="f"><div id="a"></div></div>`,
			`html { filter: blur(1px); margin: 20px 0 0 40px } #a { position: absolute }`, 0, 0},
		{"the root's, fixed", `<div id="f"><div id="a"></div></div>`,
			`html { filter: blur(1px); margin: 20px 0 0 40px } #a { position: fixed }`, 0, 0},
		{"the root's will-change", `<div id="f"><div id="a"></div></div>`,
			`html { will-change: filter; margin: 20px 0 0 40px } #a { position: fixed }`, 0, 0},
	} {
		a := cbFragment(t, tc.doc, box+tc.css, "a")
		if a.BorderRect.X != rpx(tc.x) || a.BorderRect.Y != rpx(tc.y) {
			t.Errorf("%s: the positioned box is at %v, want (%v, %v)", tc.name, a.BorderRect, tc.x, tc.y)
		}
	}
}

// TestAFilterIsTheContainingBlockOfTheOffsetsToo: the offsets and a
// percentage size are measured from the filtered box's padding box — the
// right and bottom edges as well as the left and top.
func TestAFilterIsTheContainingBlockOfTheOffsetsToo(t *testing.T) {
	a := cbFragment(t, `<div id="f"><div id="a"></div></div>`, `
		#f { margin: 30px 0 0 50px; width: 200px; height: 100px; padding: 10px; filter: blur(1px) }
		#a { position: fixed; right: 0; bottom: 0; width: 50%; height: 25% }`, "a")
	// The padding box is 50..270 across and 30..150 down.
	if want := (Rect{X: rpx(160), Y: rpx(120), W: rpx(110), H: rpx(30)}); a.BorderRect != want {
		t.Errorf("the box is %v, want %v", a.BorderRect, want)
	}
}

// TestAFilteredInlineBoxIsAContainingBlock is the suite's
// filter-cb-abspos-inline-001, which Chrome, Firefox and Safari pass: a box at
// "top: 0; left: 0" inside a span filtered by brightness(100%) is at the span's
// first fragment's padding box and not at the page's corner. And one at "top:
// 0; right: 0", as -003 writes it, is at the right of the box the span's
// fragments make (-003 itself takes the filter away with a script, and asks
// for the page's corner).
func TestAFilteredInlineBoxIsAContainingBlock(t *testing.T) {
	const span = `<p>Filler text.</p><div><span id="cb">Blue box should cover top-left corner of this sentence.<span id="a"></span></span></div>`
	// And a link, whose fragments were kept for its area and not as a
	// containing block's.
	const link = `<p>Filler text.</p><div><a id="cb" href="#x">Blue box should cover top-left corner of this sentence.<span id="a"></span></a></div>`
	for _, tc := range []struct {
		name, doc, css string
		right          bool
	}{
		{"brightness(100%)", span, `#cb { filter: brightness(100%) } #a { left: 0 }`, false},
		{"brightness(100%), right", span, `#cb { filter: brightness(100%) } #a { right: 0 }`, true},
		{"fixed", span, `#cb { filter: brightness(100%) } #a { left: 0; position: fixed }`, false},
		{"a link", link, `#cb { filter: brightness(100%) } #a { left: 0 }`, false},
	} {
		doc := tc.doc
		css := `p, div { font: 10px/20px monospace } #cb { padding: 3px } #a { position: absolute; top: 0; width: 10px; height: 10px } ` + tc.css
		a := cbFragment(t, doc, css, "a")
		cb := cbFragment(t, doc, css, "cb")
		pad := cb.PaddingRect()
		if tc.right {
			if a.BorderRect.Right() != pad.Right() || a.BorderRect.Y != pad.Y {
				t.Errorf("%s: the box is %v, want its top right at the span's padding box %v", tc.name, a.BorderRect, pad)
			}
			continue
		}
		if a.BorderRect.X != pad.X || a.BorderRect.Y != pad.Y {
			t.Errorf("%s: the box is %v, want it at the span's padding box %v", tc.name, a.BorderRect, pad)
		}
		if pad.Y <= 0 {
			t.Errorf("%s: the span is at the top of the page, which proves nothing", tc.name)
		}
	}
}

// TestAFilterIsWhereAPositionedBoxEscapesAClipTo: an out-of-flow box is
// clipped by its containing block's overflow and what clips that, and not by
// an "overflow: hidden" box between the two. So one inside a filtered box that
// clips is clipped by it, a fixed one included, where a fixed one used to be
// clipped by nothing; and one inside a box that clips inside a filtered box is
// not. Each draws a 40px square 60px into a box 50px wide.
func TestAFilterIsWhereAPositionedBoxEscapesAClipTo(t *testing.T) {
	for _, tc := range []struct {
		name, doc, css string
		clipped        bool
	}{
		{"absolute, in a filtered box that clips", `<div id="f"><div id="a"></div></div>`,
			`#f { filter: blur(1px); width: 50px; height: 50px; overflow: hidden } #a { position: absolute; left: 60px }`, true},
		{"fixed, in a filtered box that clips", `<div id="f"><div id="a"></div></div>`,
			`#f { filter: blur(1px); width: 50px; height: 50px; overflow: hidden } #a { position: fixed; left: 60px }`, true},
		{"fixed, in a box that clips with no filter", `<div id="f"><div id="a"></div></div>`,
			`#f { width: 50px; height: 50px; overflow: hidden } #a { position: fixed; left: 60px }`, false},
		{"absolute, a clip between", `<div id="f"><div id="o"><div id="a"></div></div></div>`,
			`#f { filter: blur(1px) } #o { width: 50px; height: 50px; overflow: hidden } #a { position: absolute; left: 60px }`, false},
		{"fixed, a clip around the filter", `<div id="o"><div id="f"><div id="a"></div></div></div>`,
			`#o { width: 50px; height: 50px; overflow: hidden } #f { filter: blur(1px) } #a { position: fixed; left: 60px }`, true},
		{"fixed, a clip around no filter", `<div id="o"><div id="f"><div id="a"></div></div></div>`,
			`#o { width: 50px; height: 50px; overflow: hidden } #a { position: fixed; left: 60px }`, false},
	} {
		ops := paintOf(t, tc.doc, noDefaults+tc.css+` #a { top: 0; width: 40px; height: 40px; background: red }`)
		visible := false
		var walk func([]Op, Clip)
		walk = func(ops []Op, clip Clip) {
			for _, op := range ops {
				switch v := op.(type) {
				case FillRect:
					if v.Color == red {
						r := v.Rect
						if clip.Active {
							r = r.Intersect(clip.Rect)
						}
						visible = visible || !r.Empty()
					}
				case FilterGroup:
					c := clip
					if v.Clip.Active {
						c = v.Clip
					}
					walk(v.Ops, c)
				case ClipPath:
					walk(v.Ops, clip)
				}
			}
		}
		walk(ops, Clip{})
		if visible == tc.clipped {
			t.Errorf("%s: the square visible=%v, want %v: %v", tc.name, visible, !tc.clipped, ops)
		}
	}
}

// TestWillChangeFilterIsAStackingContext: css-will-change 1 §3 makes the box
// the stacking context a filter would, so a "z-index: -1" inside it is painted
// inside it, over its background, and not hoisted under it.
func TestWillChangeFilterIsAStackingContext(t *testing.T) {
	for _, tc := range []struct {
		wc     string
		inside bool
	}{
		{"filter", true},
		{"transform, filter", true},
		{"auto", false},
		{"color", false},
	} {
		ops := paintOf(t, `<div id="f"><div id="z"></div></div>`, noDefaults+`
			#f { width: 100px; height: 100px; background: blue; will-change: `+tc.wc+` }
			#z { position: relative; z-index: -1; width: 50px; height: 50px; background: red }`)
		iBlue, iRed := -1, -1
		for i, op := range ops {
			if f, ok := op.(FillRect); ok {
				switch f.Color {
				case blue:
					iBlue = i
				case red:
					iRed = i
				}
			}
		}
		if iBlue < 0 || iRed < 0 {
			t.Fatalf("will-change: %s painted %v", tc.wc, ops)
		}
		if got := iRed > iBlue; got != tc.inside {
			t.Errorf("will-change: %s: the z-index -1 box painted over the background=%v, want %v", tc.wc, got, tc.inside)
		}
	}
}

// A will-change naming anything else is TestAWillChangeIsNotReported's, in
// willchange_test.go, and what it makes is the rest of that file's.

// TestAnOverflowFindingFollowsTheFilter: the finding about text that runs out
// of a positioned box names the clip the box is under, and that clip is its
// containing block's — here the filtered box's, which an "overflow: hidden"
// box around it clips. Without the filter a fixed box escapes every clip, and
// the finding says the text is drawn past the edge.
func TestAnOverflowFindingFollowsTheFilter(t *testing.T) {
	for _, tc := range []struct {
		name, css string
		clipped   bool
	}{
		{"fixed", `#f { filter: blur(1px) } #a { position: fixed }`, true},
		{"absolute", `#f { filter: blur(1px) } #a { position: absolute }`, true},
		{"fixed, no filter", `#a { position: fixed }`, false},
		{"absolute, no filter", `#a { position: absolute }`, false},
	} {
		_, findings := bgLayoutWithFindings(t, `<div id="o"><div id="f"><div id="a">WWWWWWWWWW</div></div></div>`,
			noDefaults+`#o { width: 200px; height: 100px; overflow: hidden }
			#a { top: 0; left: 0; width: 60px; font-family: Courier; font-size: 20px } `+tc.css)
		var msg string
		for _, f := range findings {
			if f.Rule == RuleUnbreakableOverflow {
				msg = f.Message
			}
		}
		if msg == "" {
			t.Fatalf("%s: no overflow reported: %v", tc.name, findings)
		}
		if got := strings.Contains(msg, `"overflow" on <div> clips it`); got != tc.clipped {
			t.Errorf("%s: the finding says %q", tc.name, msg)
		}
	}
}
