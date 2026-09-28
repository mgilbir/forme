package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// css-will-change 1 §3: a will-change naming a property some value of which
// would make the box a stacking context, or the containing block of the
// absolutely or of the fixed positioned boxes inside it, makes it one. See
// willChangeRules for which properties, and on which boxes.

// The names, by what they ask of an ordinary block.
var (
	// A stacking context and the containing block of every positioned box.
	willChangeTransforms = []string{"transform", "translate", "rotate", "scale", "perspective",
		"transform-style", "offset-path", "offset", "filter", "backdrop-filter", "contain",
		"content-visibility"}
	// A stacking context and nothing else.
	willChangeGroups = []string{"opacity", "isolation", "mix-blend-mode", "clip-path", "mask",
		"mask-image", "mask-border", "mask-border-source", "view-transition-name"}
	// Neither: a property that makes neither, a keyword, a custom property, a
	// vendor's name, a name no property has, and one longer than any
	// property's. z-index is one on a block it does not apply to.
	willChangeNothing = []string{"auto", "color", "margin-left", "scroll-position", "contents",
		"container-type", "container", "backface-visibility", "clip", "z-index", "--transform",
		"-webkit-transform", "transforms", "transform-transform-transform-transform"}
)

// paintedColours is the colour of every FillRect a paint made, in the order
// it made them, groups and clips included.
func paintedColours(ops []Op) []style.RGBA {
	var out []style.RGBA
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			switch v := op.(type) {
			case FillRect:
				out = append(out, v.Color)
			case FilterGroup:
				walk(v.Ops)
			case ClipPath:
				walk(v.Ops)
			}
		}
	}
	walk(ops)
	return out
}

// colourNames spells a paint order for a failure message.
func colourNames(cs []style.RGBA) string {
	names := map[style.RGBA]string{red: "red", blue: "blue", green: "green"}
	var parts []string
	for _, c := range cs {
		if n, ok := names[c]; ok {
			parts = append(parts, n)
		}
	}
	return strings.Join(parts, ", ")
}

// TestWillChangeMakesTheStackingContextItsPropertyWould: a block #f, blue,
// holds a "z-index: -1" box, red, and is followed by a block, green, pulled up
// over it by a negative margin. Where #f is a stacking context the red box is
// sealed in it and painted over its background, and #f is painted where a
// "z-index: 0" box is — CSS 2's step 8, after every in-flow block's background
// at step 4 — so the order is green, blue, red. Where it is not, the red box is
// hoisted to the root's step 3 and #f's background painted with the blocks, in
// tree order: red, blue, green. A positioned #f is painted at step 8 either
// way, and seals the red box in only as a stacking context: green, blue, red,
// against red, green, blue; and z-index, which applies to a positioned box,
// makes one there.
func TestWillChangeMakesTheStackingContextItsPropertyWould(t *testing.T) {
	const doc = `<div id="f"><div id="z"></div></div><div id="g"></div>`
	const css = noDefaults + `
		#f { width: 100px; height: 100px; background: blue }
		#z { position: relative; z-index: -1; width: 50px; height: 50px; background: red }
		#g { margin-top: -100px; width: 100px; height: 100px; background: rgb(0, 128, 0) }`
	stacking := append(append([]string{"position", "Opacity", "color, TRANSFORM"},
		willChangeTransforms...), willChangeGroups...)
	for _, positioned := range []bool{false, true} {
		extra, want, not := "", "green, blue, red", "red, blue, green"
		if positioned {
			extra, not = "position: relative;", "red, green, blue"
		}
		for _, wc := range stacking {
			got := colourNames(paintedColours(paintOf(t, doc, css+`#f { `+extra+` will-change: `+wc+` }`)))
			if got != want {
				t.Errorf("%s will-change: %s painted %s, want %s", extra, wc, got, want)
			}
		}
		for _, wc := range willChangeNothing {
			got := colourNames(paintedColours(paintOf(t, doc, css+`#f { `+extra+` will-change: `+wc+` }`)))
			if wc == "z-index" && positioned {
				// Which applies to a positioned box, and makes it one.
				if got != want {
					t.Errorf("%s will-change: %s painted %s, want %s", extra, wc, got, want)
				}
				continue
			}
			if got != not {
				t.Errorf("%s will-change: %s painted %s, want %s", extra, wc, got, not)
			}
		}
	}
}

// TestWillChangeZIndexIsAStackingContextWhereZIndexApplies: z-index makes a
// stacking context only on a box it applies to — a positioned box, or a flex
// or grid item — so naming it seals the red box in only there. On a plain
// block it asks for nothing, which the test above has.
func TestWillChangeZIndexIsAStackingContextWhereZIndexApplies(t *testing.T) {
	for _, tc := range []struct {
		name, doc, css string
		want           string
	}{
		{"a flex item", `<div style="display: flex"><div id="f"><div id="z"></div></div></div>`,
			`#f { will-change: z-index }`, "blue, red"},
		{"a grid item", `<div style="display: grid"><div id="f"><div id="z"></div></div></div>`,
			`#f { will-change: z-index }`, "blue, red"},
		{"a positioned box", `<div id="f"><div id="z"></div></div>`,
			`#f { position: absolute; will-change: z-index }`, "blue, red"},
		{"a flex item, without", `<div style="display: flex"><div id="f"><div id="z"></div></div></div>`,
			``, "red, blue"},
		{"a positioned box, without", `<div id="f"><div id="z"></div></div>`,
			`#f { position: absolute }`, "red, blue"},
		{"a block", `<div id="f"><div id="z"></div></div>`,
			`#f { will-change: z-index }`, "red, blue"},
	} {
		got := colourNames(paintedColours(paintOf(t, tc.doc, noDefaults+`
			#f { width: 100px; height: 100px; background: blue }
			#z { position: relative; z-index: -1; width: 50px; height: 50px; background: red }`+tc.css)))
		if got != tc.want {
			t.Errorf("%s: painted %s, want %s", tc.name, got, tc.want)
		}
	}
}

// TestWillChangeOnAnInlineBox: a span #f, blue, holds a "z-index: -1" span,
// red, in a block, green. Where #f is a stacking context the red span is
// painted inside it, and so after the green block's background; where it is
// not, the red span is hoisted to the root's step 3, before it. A transform,
// and containment, do not apply to a non-atomic inline box, so naming them
// makes none; a filter, an opacity and a position do.
func TestWillChangeOnAnInlineBox(t *testing.T) {
	for _, tc := range []struct {
		wc       string
		stacking bool
	}{
		{"filter", true},
		{"backdrop-filter", true},
		{"opacity", true},
		{"isolation", true},
		{"position", true},
		{"view-transition-name", true},
		{"transform", false},
		{"perspective", false},
		{"contain", false},
		{"content-visibility", false},
		{"z-index", false},
		{"color", false},
	} {
		got := colourNames(paintedColours(paintOf(t, `<div id="c"><span id="f">ab<span id="z">cd</span></span></div>`,
			noDefaults+`#c { background: rgb(0, 128, 0); font: 20px/20px Courier }
			#f { background: blue; will-change: `+tc.wc+` }
			#z { position: relative; z-index: -1; background: red }`)))
		green, red := strings.Index(got, "green"), strings.Index(got, "red")
		if green < 0 || red < 0 || !strings.Contains(got, "blue") {
			t.Fatalf("will-change: %s on a span painted %s", tc.wc, got)
		}
		if (green < red) != tc.stacking {
			t.Errorf("will-change: %s on a span painted %s: a stacking context=%v, want %v", tc.wc, got, green < red, tc.stacking)
		}
	}
}

// TestWillChangeMakesTheContainingBlockItsPropertyWould: a 10px box at "top:
// 0; left: 0" inside #f, 30 down and 50 across with a 2px border, is at #f's
// padding box, (52, 32), where #f is its containing block, and at the page's
// corner where it is not. Every name that makes a transform's containing block
// makes it for an absolutely and a fixed positioned box alike; position makes
// it for an absolutely positioned one only (CSS Positioned Layout 3 §2); and
// the names that make only a stacking context, or nothing, make neither.
func TestWillChangeMakesTheContainingBlockItsPropertyWould(t *testing.T) {
	const box = `#f { margin: 30px 0 0 50px; border: 2px solid; padding: 5px; height: 40px }
		#a { top: 0; left: 0; width: 10px; height: 10px }`
	const doc = `<div id="f"><div id="a"></div></div>`
	type want struct{ absolute, fixed bool }
	cases := map[string]want{"position": {true, false}, "Position, color": {true, false}}
	for _, n := range willChangeTransforms {
		cases[n] = want{true, true}
	}
	cases["color, BACKDROP-FILTER"] = want{true, true}
	for _, n := range willChangeGroups {
		cases[n] = want{false, false}
	}
	for _, n := range willChangeNothing {
		cases[n] = want{false, false}
	}
	for wc, w := range cases {
		for _, scheme := range []string{"absolute", "fixed"} {
			contained := w.absolute
			if scheme == "fixed" {
				contained = w.fixed
			}
			x, y := 0.0, 0.0
			if contained {
				x, y = 52, 32
			}
			a := cbFragment(t, doc, box+`#f { will-change: `+wc+` } #a { position: `+scheme+` }`, "a")
			if a.BorderRect.X != rpx(x) || a.BorderRect.Y != rpx(y) {
				t.Errorf("will-change: %s, %s: the box is at %v, want (%v, %v)", wc, scheme, a.BorderRect, x, y)
			}
		}
	}
}

// TestWillChangeContainingBlocksAreThePropertys: each one is made only on the
// boxes its property would make it on. The root has a margin, 20 down and 40
// across, which a containing block there shows: a transform's and
// containment's are made on it, and a filter's and a backdrop filter's are not,
// as Filter Effects exempts the root. A span with padding on the second line
// is a containing block for a filter and a position, which apply to it, and
// not for a transform or containment, which do not. contain and a transform
// apply to a table cell, and content-visibility, which applies where size
// containment does, does not; nor to a table, where contain does. A caption
// is not an internal table box, and an inline-block is atomic: both apply.
func TestWillChangeContainingBlocksAreThePropertys(t *testing.T) {
	const pos = ` #a { top: 0; left: 0; width: 10px; height: 10px }`
	for _, tc := range []struct {
		name, doc, css string
		x, y           float64
	}{
		{"the root's transform, fixed", `<div id="a"></div>`, `html { will-change: transform; margin: 20px 0 0 40px } #a { position: fixed }`, 40, 20},
		{"the root's contain", `<div id="a"></div>`, `html { will-change: contain; margin: 20px 0 0 40px } #a { position: fixed }`, 40, 20},
		{"the root's filter", `<div id="a"></div>`, `html { will-change: filter; margin: 20px 0 0 40px } #a { position: absolute }`, 0, 0},
		{"the root's backdrop-filter", `<div id="a"></div>`, `html { will-change: backdrop-filter; margin: 20px 0 0 40px } #a { position: fixed }`, 0, 0},
		{"the root's position", `<div id="a"></div>`, `html { will-change: position; margin: 20px 0 0 40px } #a { position: absolute }`, 40, 20},
		{"the root's position, fixed", `<div id="a"></div>`, `html { will-change: position; margin: 20px 0 0 40px } #a { position: fixed }`, 0, 0},
	} {
		a := cbFragment(t, tc.doc, tc.css+pos, "a")
		if a.BorderRect.X != rpx(tc.x) || a.BorderRect.Y != rpx(tc.y) {
			t.Errorf("%s: the box is at %v, want (%v, %v)", tc.name, a.BorderRect, tc.x, tc.y)
		}
	}

	// Boxes of the kinds that apply the property or not, whose padding box is
	// where a containing block there puts the box, and is not the page's
	// corner.
	const cell = `<div style="display: table"><div id="f" style="display: table-cell"><div id="a"></div></div></div>`
	const table = `<div id="f" style="display: table"><div style="display: table-cell"><div id="a"></div></div></div>`
	const caption = `<table><caption id="f"><div id="a"></div></caption><tr><td>x</td></tr></table>`
	const inlineBlock = `<div><span id="f"><div id="a"></div></span></div>`
	for _, tc := range []struct {
		name, doc, css string
		contains       bool
	}{
		{"a cell's contain", cell, `#f { will-change: contain } #a { position: fixed }`, true},
		{"a cell's transform", cell, `#f { will-change: transform } #a { position: fixed }`, true},
		{"a cell's content-visibility", cell, `#f { will-change: content-visibility } #a { position: fixed }`, false},
		{"a table's contain", table, `#f { will-change: contain } #a { position: absolute }`, true},
		{"a table's content-visibility", table, `#f { will-change: content-visibility } #a { position: absolute }`, false},
		{"a caption's content-visibility", caption, `#f { will-change: content-visibility } #a { position: absolute }`, true},
		{"an inline-block's transform", inlineBlock, `#f { display: inline-block; width: 100px; will-change: transform } #a { position: fixed }`, true},
		{"an inline-block's contain", inlineBlock, `#f { display: inline-block; width: 100px; will-change: contain } #a { position: absolute }`, true},
	} {
		css := `#f { margin-left: 50px; border: 2px solid; padding: 3px } ` + tc.css + pos
		a := cbFragment(t, tc.doc, css, "a")
		pad := cbFragment(t, tc.doc, css, "f").PaddingRect()
		if pad.X <= 0 {
			t.Fatalf("%s: #f is at the page's edge, which proves nothing", tc.name)
		}
		at := a.BorderRect.X == pad.X && a.BorderRect.Y == pad.Y
		corner := a.BorderRect.X == 0 && a.BorderRect.Y == 0
		if tc.contains && !at || !tc.contains && !corner {
			t.Errorf("%s: the box is at %v; #f's padding box is %v, want it there=%v", tc.name, a.BorderRect, pad, tc.contains)
		}
	}

	// The span, whose padding box is where a containing block there puts the
	// box, and is not the page's corner.
	const span = `<p>Filler text.</p><div><span id="cb">Some words in the span.<span id="a"></span></span></div>`
	for _, tc := range []struct {
		wc, scheme string
		contains   bool
	}{
		{"filter", "fixed", true},
		{"backdrop-filter", "fixed", true},
		{"position", "absolute", true},
		{"position", "fixed", false},
		{"transform", "absolute", false},
		{"offset-path", "absolute", false},
		{"contain", "absolute", false},
		{"content-visibility", "fixed", false},
		{"opacity", "absolute", false},
	} {
		css := `p, div { font: 10px/20px Courier } #cb { padding: 3px; background: blue; will-change: ` + tc.wc + ` }
			#a { position: ` + tc.scheme + `; top: 0; left: 0; width: 10px; height: 10px }`
		a := cbFragment(t, span, css, "a")
		pad := cbFragment(t, span, css, "cb").PaddingRect()
		if pad.Y <= 0 {
			t.Fatalf("will-change: %s: the span is at the top of the page, which proves nothing", tc.wc)
		}
		at := a.BorderRect.X == pad.X && a.BorderRect.Y == pad.Y
		corner := a.BorderRect.X == 0 && a.BorderRect.Y == 0
		if tc.contains && !at || !tc.contains && !corner {
			t.Errorf("will-change: %s on a span, %s: the box is at %v; the span's padding box is %v, want it there=%v",
				tc.wc, tc.scheme, a.BorderRect, pad, tc.contains)
		}
	}
}

// TestWillChangeIsWhereAPositionedBoxEscapesAClipTo: an out-of-flow box is
// clipped by what clips its containing block, and not by a box that clips
// between the two; so a fixed box inside a box that clips and whose
// will-change names transform is clipped by it, and one inside such a box
// naming position escapes it — position makes no containing block for a fixed
// box — while an absolutely positioned one does not.
func TestWillChangeIsWhereAPositionedBoxEscapesAClipTo(t *testing.T) {
	for _, tc := range []struct {
		wc, scheme string
		clipped    bool
	}{
		{"transform", "fixed", true},
		{"contain", "fixed", true},
		{"position", "absolute", true},
		{"position", "fixed", false},
		{"opacity", "fixed", false},
	} {
		ops := paintOf(t, `<div id="f"><div id="a"></div></div>`, noDefaults+`
			#f { width: 50px; height: 50px; overflow: hidden; will-change: `+tc.wc+` }
			#a { position: `+tc.scheme+`; left: 60px; top: 0; width: 40px; height: 40px; background: red }`)
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
					walk(v.Ops, clip)
				case ClipPath:
					walk(v.Ops, clip)
				}
			}
		}
		walk(ops, Clip{})
		if visible == tc.clipped {
			t.Errorf("will-change: %s, %s: the square visible=%v, want %v: %v", tc.wc, tc.scheme, visible, !tc.clipped, ops)
		}
	}
}

// TestAWillChangeIsNotReported: what naming a property asks for is made, so
// nothing is reported about the hint, whatever it names; and naming a property
// asks for none of what the property itself would do, so a will-change naming
// transform reports no transform either. A transform declared is reported
// where it is declared, as it was.
func TestAWillChangeIsNotReported(t *testing.T) {
	names := append(append(append([]string{}, willChangeTransforms...), willChangeGroups...), willChangeNothing...)
	names = append(names, "position", "z-index")
	for _, n := range names {
		built := Build(Input{HTML: `<div class="w">x</div><div class="w">y</div>`,
			CSS: []Stylesheet{{Source: `.w { will-change: ` + n + ` }`}}})
		rec := NewRecorder(nil)
		PaintReporting(Layout(built.Root, Size{W: rpx(600), H: rpx(1000)}, nil, rec), rec)
		for _, f := range append(built.Findings, rec.Findings()...) {
			if f.Unsupported() || f.Property == "will-change" {
				t.Errorf("will-change: %s: reported %v", n, f)
			}
		}
	}
	for _, decl := range []string{"transform: rotate(10deg)", "backdrop-filter: blur(2px)", "backdrop-filter: grayscale(1)"} {
		built := Build(Input{HTML: `<div id="d">x</div>`, CSS: []Stylesheet{{Source: `#d { ` + decl + ` }`}}})
		name, _, _ := strings.Cut(decl, ":")
		reported := false
		for _, f := range built.Findings {
			reported = reported || (f.Property == name && f.Unsupported())
		}
		if !reported {
			t.Errorf("%s was not reported: %v", decl, built.Findings)
		}
	}
}

// TestAWillChangeContainingBlockLiftedOutOfIsReported: a positioned box in a
// block §9.2.1.1 lifted out of an inline box is positioned against the next
// containing block up where the inline box is its containing block, and that
// is reported, naming what made the inline box one: a will-change naming
// backdrop-filter or position, which apply to an inline box. Naming transform
// makes none there, and there is nothing to report.
func TestAWillChangeContainingBlockLiftedOutOfIsReported(t *testing.T) {
	for _, tc := range []struct {
		css      string
		property string
	}{
		{`#f { will-change: backdrop-filter } #a { position: fixed }`, "will-change"},
		{`#f { will-change: position } #a { position: absolute }`, "will-change"},
		{`#f { will-change: filter } #a { position: absolute }`, "will-change"},
		{`#f { filter: blur(1px); will-change: filter } #a { position: absolute }`, "filter"},
		{`#f { will-change: position } #a { position: fixed }`, ""},
		{`#f { will-change: transform } #a { position: absolute }`, ""},
		{`#f { will-change: opacity } #a { position: fixed }`, ""},
	} {
		_, findings := filterFindings(t, `<span id="f"><div><div id="a"></div></div></span>`, tc.css)
		got := ""
		for _, f := range findings {
			if f.Rule == RulePositionApproximated {
				got = f.Property
			}
		}
		if got != tc.property {
			t.Errorf("%s: reported for %q, want %q", tc.css, got, tc.property)
		}
	}
}

// TestAContainingBlockNotFormedIsNamedForWhatItIs: a table row is a containing
// block this engine does not form, positioned or asked for by a will-change,
// and the report says it is a row, not an inline box with no fragments.
func TestAContainingBlockNotFormedIsNamedForWhatItIs(t *testing.T) {
	for _, css := range []string{`#r { position: relative }`, `#r { will-change: transform }`} {
		_, findings := filterFindings(t, `<table><tr id="r"><td>x<div id="a"></div></td></tr></table>`,
			css+` #a { position: absolute; top: 0; left: 0 }`)
		var msg string
		for _, f := range findings {
			if f.Rule == RulePositionApproximated {
				msg = f.Message
			}
		}
		if !strings.Contains(msg, "is a table-row box, which this engine does not form a containing block from") {
			t.Errorf("%s: reported %q", css, msg)
		}
	}
}

// TestWillChangeAsksWhatItsRulesSay reads willChangeAsksOf on one box of each
// kind for every name, exactly; and checks the one thing the three
// predicates rely on of every rule: a box asked to be the containing block of
// the fixed boxes inside it is asked to be that of the absolute ones as well.
func TestWillChangeAsksWhatItsRulesSay(t *testing.T) {
	root := &Box{Outer: OuterBlock}
	kinds := map[string]*Box{
		"root":         root,
		"block":        {Outer: OuterBlock, Parent: root},
		"inline":       {Outer: OuterInline, Parent: root},
		"inline-block": {Outer: OuterInline, Inner: InnerFlowRoot, Parent: root},
		"replaced":     {Outer: OuterInline, Replaced: &ReplacedContent{}, Parent: root},
		"table":        {Outer: OuterBlock, Inner: InnerTable, Parent: root},
		"row":          {Outer: OuterBlock, Inner: InnerTableRow, Parent: root},
		"cell":         {Outer: OuterBlock, Inner: InnerTableCell, Parent: root},
		"column":       {Outer: OuterBlock, Inner: InnerTableColumn, Parent: root},
		"caption":      {Outer: OuterBlock, Inner: InnerTableCaption, Parent: root},
		"positioned":   {Outer: OuterBlock, Position: PositionRelative, Parent: root},
	}
	const all, sc, abs = asksEverything, asksStackingContext, asksStackingContext | asksAbsoluteContainer
	want := map[string]map[string]willChangeAsks{
		"transform": {"root": all, "block": all, "inline-block": all, "replaced": all, "table": all,
			"row": all, "cell": all, "caption": all, "positioned": all},
		"filter": {"root": sc, "block": all, "inline": all, "inline-block": all, "replaced": all,
			"table": all, "row": all, "cell": all, "column": all, "caption": all, "positioned": all},
		"opacity": {"root": sc, "block": sc, "inline": sc, "inline-block": sc, "replaced": sc,
			"table": sc, "row": sc, "cell": sc, "column": sc, "caption": sc, "positioned": sc},
		"contain": {"root": all, "block": all, "inline-block": all, "replaced": all, "table": all,
			"cell": all, "caption": all, "positioned": all},
		"content-visibility": {"root": all, "block": all, "inline-block": all, "replaced": all,
			"caption": all, "positioned": all},
		"position": {"root": abs, "block": abs, "inline": abs, "inline-block": abs, "replaced": abs,
			"table": abs, "row": abs, "cell": abs, "caption": abs, "positioned": abs},
		"z-index":             {"positioned": sc},
		"backface-visibility": {},
		"container-type":      {},
	}
	for name, byKind := range want {
		for kind, b := range kinds {
			b.Style = style.Initial().With("will-change", "color, "+strings.ToUpper(name))
			if got := willChangeAsksOf(b); got != byKind[kind] {
				t.Errorf("will-change: %s on a %s asks %03b, want %03b", name, kind, got, byKind[kind])
			}
		}
	}
	for name, rule := range willChangeRules {
		for kind, b := range kinds {
			if asks := rule(b); asks&asksFixedContainer != 0 && asks&asksAbsoluteContainer == 0 {
				t.Errorf("%s on a %s is a containing block for fixed boxes and not for absolute ones", name, kind)
			}
		}
	}
}

// TestReadingAWillChangeAllocatesNothing: it is asked of every box at every
// step of the paint, so it is walked in place, capitals and all.
func TestReadingAWillChangeAllocatesNothing(t *testing.T) {
	b := &Box{Outer: OuterBlock, Parent: &Box{}}
	b.Style = style.Initial().With("will-change", "Color, Margin-Left, OPACITY, transform, --x, "+strings.Repeat("x", 40))
	var asks willChangeAsks
	if n := testing.AllocsPerRun(100, func() { asks = willChangeAsksOf(b) }); n != 0 {
		t.Errorf("reading a will-change made %v allocations", n)
	}
	if asks != asksEverything {
		t.Errorf("it asked %03b, want %03b", asks, asksEverything)
	}
}
