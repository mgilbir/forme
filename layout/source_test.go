package layout

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Which box painted each operation: see source.go.

// misattributed is every operation in ops, the contents of groups included,
// that does not say which box painted it, or says it in a way its kind cannot
// be: a group that is not PartGroup, a mark that is, and the run inside a
// shadow or an emphasis mark that claims to be what it is a copy of.
func misattributed(ops []Op) []string {
	var out []string
	var walk func(ops []Op, at string)
	walk = func(ops []Op, at string) {
		for i, op := range ops {
			where := fmt.Sprintf("%s%d %T", at, i, op)
			by := OpPaintedBy(op)
			var inner []Op
			group := false
			switch v := op.(type) {
			case ClipPath:
				inner, group = v.Ops, true
			case FilterGroup:
				inner, group = v.Ops, true
			case TransformGroup:
				inner, group = v.Ops, true
			case DrawTextShadow:
				if v.Run.PaintedBy != (PaintedBy{}) {
					out = append(out, where+": its run says it was painted by "+partOf(v.Run.PaintedBy))
				}
			case DrawEmphasisMark:
				if v.Mark.PaintedBy != (PaintedBy{}) {
					out = append(out, where+": its mark says it was painted by "+partOf(v.Mark.PaintedBy))
				}
			}
			switch {
			case by.Box == nil:
				out = append(out, where+": no box")
			case group && by.Part != PartGroup:
				out = append(out, where+": a group, as "+by.Part.String())
			case !group && (by.Part == PartNone || by.Part == PartGroup):
				out = append(out, where+": a mark, as "+by.Part.String())
			}
			walk(inner, where+" > ")
		}
	}
	walk(ops, "")
	return out
}

func partOf(by PaintedBy) string { return by.Part.String() }

// paintedKinds is each kind of operation in ops, groups' contents included,
// with the part it was painted as: "FillRect/border".
func paintedKinds(ops []Op) map[string]bool {
	out := map[string]bool{}
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			out[strings.TrimPrefix(fmt.Sprintf("%T", op), "layout.")+"/"+OpPaintedBy(op).Part.String()] = true
			switch v := op.(type) {
			case ClipPath:
				walk(v.Ops)
			case FilterGroup:
				walk(v.Ops)
			case TransformGroup:
				walk(v.Ops)
			}
		}
	}
	walk(ops)
	return out
}

// attributionResources is what the documents below load.
func attributionResources(t *testing.T) mapResolver {
	t.Helper()
	return mapResolver{
		"a.png": encodePNG(t, 4, 4),
		"s.svg": []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">` +
			`<rect x="2" y="2" width="10" height="10" fill="red"/></svg>`),
	}
}

func composeAttributed(t *testing.T, htmlSrc, cssSrc string, opts Options) Composed {
	t.Helper()
	in := Input{HTML: htmlSrc, Resources: attributionResources(t)}
	if cssSrc != "" {
		in.CSS = []Stylesheet{{Source: cssSrc}}
	}
	out := Compose(in, opts)
	if len(out.Ops) == 0 {
		t.Fatalf("%q painted nothing", htmlSrc)
	}
	return out
}

// TestEveryOperationSaysWhichBoxPaintedIt is the contract of #963 across the
// painter: every operation that leaves it, inside a group or not, names a box
// and what of the box it is, whichever of the painter's routes made it.
//
// Each document is one of those routes, and names the operations it has to
// produce, so that a document that stopped exercising its route — a fixture
// the engine came to draw some other way — fails rather than passing on
// nothing. The kinds are also the part each route is painted as, so a step
// attributed to the wrong part fails here too.
func TestEveryOperationSaysWhichBoxPaintedIt(t *testing.T) {
	for _, c := range []struct {
		name, html, css string
		opts            Options
		want            []string
	}{
		{name: "text", html: `<p>Hello</p>`, want: []string{"DrawText/content"}},
		{name: "canvas", html: `<body style="background: silver"><p>x</p></body>`,
			want: []string{"FillRect/background", "DrawText/content"}},
		{name: "background and border",
			html: `<div style="background: red; border: 2px solid blue">x</div>`,
			want: []string{"FillRect/background", "FillRect/border"}},
		{name: "dashed border", html: `<div style="border: 3px dashed blue">x</div>`,
			want: []string{"FillRect/border"}},
		{name: "ridged border", html: `<div style="border: 6px ridge gray">x</div>`,
			want: []string{"FillRect/border"}},
		{name: "rounded background and border",
			html: `<div style="background: red; border: 3px solid blue; border-radius: 10px">x</div>`,
			want: []string{"FillPath/background", "FillPath/border"}},
		{name: "rounded dotted border",
			html: `<div style="border: 4px dotted blue; border-radius: 12px; height: 40px">x</div>`,
			want: []string{"FillPath/border"}},
		{name: "tiled image", html: `<div style="background: url(a.png); height: 20px">x</div>`,
			want: []string{"TileImage/background"}},
		{name: "tiled image, rounded",
			html: `<div style="background: url(a.png); border-radius: 10px; height: 40px">x</div>`,
			want: []string{"TileImage/background", "ClipPath/group"}},
		{name: "gradient", html: `<div style="background: linear-gradient(red, blue); height: 20px">x</div>`,
			want: []string{"FillGradient/background"}},
		{name: "outline", html: `<div style="outline: 2px solid red">x</div>`,
			want: []string{"FillRect/outline"}},
		{name: "outline of an inline box",
			html: `<p style="width: 40px">a <span style="outline: 1px solid red">b c d e f g</span></p>`,
			want: []string{"FillRect/outline"}},
		{name: "bullet", html: `<ul><li>a</li></ul>`, want: []string{"DrawText/marker"}},
		{name: "number", html: `<ol><li>a</li></ol>`, want: []string{"DrawText/marker"}},
		{name: "marker picture", html: `<ul style="list-style-image: url(a.png)"><li>a</li></ul>`,
			want: []string{"DrawImage/marker"}},
		{name: "image", html: `<img src="a.png" alt="A">`, want: []string{"DrawImage/content"}},
		{name: "image, rounded and cut",
			html: `<img src="a.png" style="width: 40px; height: 20px; object-fit: cover; border-radius: 8px">`,
			want: []string{"DrawImage/content", "ClipPath/group"}},
		{name: "svg", html: `<img src="s.svg">`, want: []string{"FillRect/content"}},
		{name: "inline box",
			html: `<p>a <span style="background: yellow; border: 1px solid red">b</span></p>`,
			want: []string{"FillRect/background", "FillRect/border", "DrawText/content"}},
		{name: "decoration", html: `<p style="text-decoration: underline line-through">x <em>y</em></p>`,
			want: []string{"FillRect/decoration"}},
		{name: "text shadow", html: `<p style="text-shadow: 1px 1px red; text-decoration: underline">x</p>`,
			want: []string{"DrawTextShadow/shadow", "FillRect/shadow"}},
		{name: "blurred text shadow",
			html: `<p style="text-shadow: 1px 1px 2px red; text-decoration: underline">x</p>`,
			want: []string{"DrawTextShadow/shadow", "FilterGroup/group", "FillRect/shadow"}},
		{name: "emphasis", html: `<p style="text-emphasis: dot">xy</p>`,
			want: []string{"DrawEmphasisMark/decoration"}},
		{name: "blur", html: `<div style="filter: blur(2px); background: red">x</div>`,
			want: []string{"FilterGroup/group", "FillRect/background"}},
		{name: "drop shadow", html: `<div style="filter: drop-shadow(2px 2px red); background: red">x</div>`,
			want: []string{"FillRect/shadow", "DrawTextShadow/shadow", "FillRect/background"}},
		{name: "blurred drop shadow",
			html: `<div style="filter: drop-shadow(2px 2px 2px red); background: red">x</div>`,
			want: []string{"FilterGroup/group", "FillRect/shadow"}},
		{name: "drop shadow of a rounded box",
			html: `<div style="filter: drop-shadow(2px 2px red); background: red; border-radius: 9px">x</div>`,
			want: []string{"FillPath/shadow", "FillPath/background"}},
		{name: "filtered inline box",
			html: `<p>a <span style="filter: blur(1px); background: yellow">b</span></p>`,
			want: []string{"FilterGroup/group"}},
		{name: "opacity", html: `<div style="opacity: 0.5; background: red">x <span style="opacity: 0.5">y</span></div>`,
			want: []string{"FillRect/background", "DrawText/content"}},
		{name: "turned through a matrix", opts: Options{TransformGroups: true},
			html: `<div style="transform: rotate(30deg); background: red; width: 40px">x</div>`,
			want: []string{"TransformGroup/group", "FillRect/background", "DrawText/content"}},
		{name: "turned by a quarter",
			html: `<div style="transform: rotate(90deg); background: red; width: 40px">x</div>`,
			want: []string{"FillRect/background", "DrawText/content"}},
		{name: "a turned group inside a translucent one", opts: Options{TransformGroups: true},
			html: `<div style="opacity: 0.5"><div style="transform: rotate(30deg); background: red">x</div></div>`,
			want: []string{"TransformGroup/group"}},
		{name: "a filter inside a turn", opts: Options{TransformGroups: true},
			html: `<div style="transform: rotate(90deg)"><div style="filter: blur(1px); border-radius: 4px; overflow: hidden; background: red">x</div></div>`,
			want: []string{"FilterGroup/group"}},
		{name: "rounded overflow",
			html: `<div style="overflow: hidden; border-radius: 20px; width: 50px; height: 50px">` +
				`<div style="background: red; width: 100px; height: 100px">x</div></div>`,
			want: []string{"ClipPath/group", "FillRect/background"}},
		{name: "collapsed table",
			html: `<table style="border-collapse: collapse; border: 2px solid"><tr>` +
				`<td style="border: 1px solid red">x</td><td>y</td></tr></table>`,
			want: []string{"FillRect/border", "DrawText/content"}},
		{name: "link", html: `<p><a href="http://example.com/">t</a></p>`,
			want: []string{"Link/content"}},
		{name: "block link", html: `<a href="http://example.com/" style="display: block">t</a>`,
			want: []string{"Link/content"}},
		{name: "a link with no box",
			html: `<p><a href="http://example.com/" style="display: contents"><span>t</span></a></p>`,
			want: []string{"Link/content"}},
		{name: "generated", html: `<p>x</p>`, css: `p::before { content: "a"; background: red }`,
			want: []string{"DrawText/content", "FillRect/background"}},
		{name: "first letter", html: `<p>xyz</p>`, css: `p::first-letter { color: red }`,
			want: []string{"DrawText/content"}},
		{name: "stacked", html: `<div style="float: left; background: red">f</div>` +
			`<div style="position: relative; z-index: -1; background: blue">p</div>` +
			`<p>a <span style="position: relative; background: yellow">b</span></p>`,
			want: []string{"FillRect/background", "DrawText/content"}},
		{name: "vertical", html: `<p style="writing-mode: vertical-rl">abc</p>`,
			want: []string{"DrawText/content"}},
		{name: "fraction", html: `<math><mfrac><mi>a</mi><mi>b</mi></mfrac></math>`,
			want: []string{"FillRect/content", "DrawText/content"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := composeAttributed(t, c.html, c.css, c.opts)
			for _, bad := range misattributed(out.Ops) {
				t.Error(bad)
			}
			got := paintedKinds(out.Ops)
			for _, w := range c.want {
				if !got[w] {
					var have []string
					for k := range got {
						have = append(have, k)
					}
					sort.Strings(have)
					t.Errorf("no %s among %v", w, have)
				}
			}
		})
	}
}

// paintedElement is the element an operation belongs to, by name, for a test that
// asks which: "h1", "body".
func paintedElement(op Op) string {
	if el := OpPaintedBy(op).Element(); el != nil {
		return el.Name
	}
	return ""
}

// TestAnOperationLeadsToItsElement is #963's first acceptance case: each run of
// text leads to the element it is in, and the picture to its <img>, whose
// attributes a backend reads from there.
func TestAnOperationLeadsToItsElement(t *testing.T) {
	out := composeAttributed(t, `<h1>Title</h1><p>Text <img src="a.png" alt="A"></p>`, "", Options{})
	texts, images := map[string]string{}, 0
	for _, op := range out.Ops {
		switch v := op.(type) {
		case DrawText:
			if strings.TrimSpace(v.Text) != "" {
				texts[strings.TrimSpace(v.Text)] = paintedElement(v)
			}
			if by := v.PaintedBy; by.Box == nil || !by.Box.IsText() {
				t.Errorf("the run %q was attributed to %+v, not to the text box it came from", v.Text, by)
			}
		case DrawImage:
			images++
			el := v.PaintedBy.Element()
			if el == nil || el.Name != "img" {
				t.Fatalf("the picture leads to %v, not to the <img>", el)
			}
			if alt, _ := el.AttrExact("alt"); alt != "A" {
				t.Errorf("the <img>'s alt reads %q, not %q", alt, "A")
			}
			if v.PaintedBy.Part != PartContent {
				t.Errorf("the picture is %s, not content", v.PaintedBy.Part)
			}
		}
	}
	if want := map[string]string{"Title": "h1", "Text": "p"}; fmt.Sprint(texts) != fmt.Sprint(want) {
		t.Errorf("the runs lead to %v, not %v", texts, want)
	}
	if images != 1 {
		t.Errorf("%d pictures were drawn, not one", images)
	}
}

// TestAMarkerAndACellLeadToTheirElements is the second: a list marker leads to
// its <li>, as its marker and not its content, and a cell's text to its <td>.
func TestAMarkerAndACellLeadToTheirElements(t *testing.T) {
	out := composeAttributed(t, `<ul><li>item</li></ul><table><tr><td>cell</td></tr></table>`, "", Options{})
	var marker, item, cell bool
	for _, op := range out.Ops {
		v, ok := op.(DrawText)
		if !ok {
			continue
		}
		switch {
		case v.PaintedBy.Part == PartMarker:
			marker = true
			if el := paintedElement(v); el != "li" {
				t.Errorf("the marker %q leads to <%s>, not to the <li>", v.Text, el)
			}
		case v.Text == "item":
			item = true
			if el := paintedElement(v); el != "li" {
				t.Errorf("the item's text leads to <%s>", el)
			}
		case v.Text == "cell":
			cell = true
			if el := paintedElement(v); el != "td" {
				t.Errorf("the cell's text leads to <%s>, not to the <td>", el)
			}
		}
		if v.PaintedBy.Part != PartMarker && v.PaintedBy.Part != PartContent {
			t.Errorf("the run %q is %s", v.Text, v.PaintedBy.Part)
		}
	}
	if !marker || !item || !cell {
		t.Errorf("drew the marker %v, the item %v, the cell %v", marker, item, cell)
	}
}

// TestDressIsToldFromContent is the third: what one box paints as its
// background and border is told apart from the text it paints, and a line
// text-decoration rules is the box that declared it, which is not the box
// whose words it crosses.
func TestDressIsToldFromContent(t *testing.T) {
	out := composeAttributed(t,
		`<p style="background: red; border: 1px solid blue; text-decoration: underline">a <em>b</em></p>`,
		"", Options{})
	parts := map[Part]int{}
	for _, op := range out.Ops {
		by := OpPaintedBy(op)
		if el := paintedElement(op); el != "p" && el != "em" {
			continue
		}
		parts[by.Part]++
		switch v := op.(type) {
		case FillRect:
			if el := paintedElement(v); el != "p" {
				t.Errorf("a %s fill leads to <%s>, not to the <p> that declared it", by.Part, el)
			}
			if by.Part == PartContent {
				t.Errorf("a fill at %v is the <p>'s content", v.Rect)
			}
		case DrawText:
			if by.Part != PartContent {
				t.Errorf("the run %q is %s", v.Text, by.Part)
			}
			if want := map[string]string{"a": "p", "b": "em"}[strings.TrimSpace(v.Text)]; want != "" && paintedElement(v) != want {
				t.Errorf("the run %q leads to <%s>, not <%s>", v.Text, paintedElement(v), want)
			}
		}
	}
	for _, p := range []Part{PartBackground, PartBorder, PartDecoration, PartContent} {
		if parts[p] == 0 {
			t.Errorf("nothing of the <p> was painted as its %s: %v", p, parts)
		}
	}
}

// TestThePageBackgroundIsBodys: the canvas takes <body>'s background when the
// root has none, and is attributed to <body>, whose background it is.
func TestThePageBackgroundIsBodys(t *testing.T) {
	out := composeAttributed(t, `<body style="background: silver"><p>x</p></body>`, "", Options{})
	first, ok := out.Ops[0].(FillRect)
	if !ok {
		t.Fatalf("the first operation is a %T, not the page's background", out.Ops[0])
	}
	if el := paintedElement(first); el != "body" || first.PaintedBy.Part != PartBackground {
		t.Errorf("the page's background is <%s>'s %s, not <body>'s background", el, first.PaintedBy.Part)
	}
}

// TestALinkIsItsAnchors: a Link names the <a>'s own box, even where the
// first of its areas is another box's — a float it holds and nothing else —
// and an <a> with "display: contents", which has no box, names the box of its
// first area, whose element is inside it.
func TestALinkIsItsAnchors(t *testing.T) {
	for _, c := range []struct {
		name, html string
		contents   bool
	}{
		{name: "inline", html: `<p><a href="http://example.com/">t</a></p>`},
		{name: "around a picture", html: `<p><a href="http://example.com/"><img src="a.png"></a></p>`},
		{name: "around a float", html: `<p><a href="http://example.com/"><img src="a.png" style="float: left"></a></p>`},
		{name: "with no box", contents: true,
			html: `<p><a href="http://example.com/" style="display: contents"><span>t</span></a></p>`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := composeAttributed(t, c.html, "", Options{})
			links := 0
			for _, op := range out.Ops {
				l, ok := op.(Link)
				if !ok {
					continue
				}
				links++
				a := l.PaintedBy.Element()
				for c.contents && a != nil && a.Name != "a" {
					a = a.Parent
				}
				if a == nil || a.Name != "a" {
					t.Fatalf("the link leads to <%s>, not to its <a>", paintedElement(l))
				}
				if href, _ := a.AttrExact("href"); href != l.Href {
					t.Errorf("the link goes to %q and leads to an <a> to %q", l.Href, href)
				}
			}
			if links != 1 {
				t.Errorf("%d links, not one", links)
			}
		})
	}
}

// TestACollapsedBorderIsTheWinners: a stretch of a collapsed grid line is the
// border of the box whose border won it, which here is the cell's on every
// side — the table declares none — and not the table's, which paints the grid.
func TestACollapsedBorderIsTheWinners(t *testing.T) {
	out := composeAttributed(t, `<table style="border-collapse: collapse"><tr>`+
		`<td style="border: 3px solid red">x</td></tr></table>`, "", Options{})
	bands := 0
	for _, op := range out.Ops {
		if f, ok := op.(FillRect); ok && f.PaintedBy.Part == PartBorder {
			bands++
			if el := paintedElement(f); el != "td" {
				t.Errorf("a band of the grid at %v leads to <%s>, not to the <td>", f.Rect, el)
			}
		}
	}
	if bands == 0 {
		t.Error("the collapsed grid drew no band")
	}
}

// TestARewriteKeepsWhoPainted: every pass that copies or rebuilds operations
// after they are painted — opacity folded in, a clip cut, an overhang marked, a
// quarter turn mapped, a colour matrix folded, a curve cut, links gathered —
// keeps what each operation says painted it, groups' included. Some of the
// rebuilds are reached by no document today (a TransformGroup is never inside
// what opacity folds into), and a box dropped there would go unseen until one
// is, so they are asked here directly.
func TestARewriteKeepsWhoPainted(t *testing.T) {
	boxes := map[string]*Box{}
	by := func(kind string, part Part) PaintedBy {
		if boxes[kind] == nil {
			boxes[kind] = &Box{}
		}
		return PaintedBy{Box: boxes[kind], Part: part}
	}
	r := Rect{X: rpx(10), Y: rpx(10), W: rpx(40), H: rpx(20)}
	inner := func() []Op {
		return []Op{FillRect{Rect: r, Color: style.RGBA{R: 1, A: 1}, PaintedBy: by("inner", PartContent)}}
	}
	text := DrawText{At: Point{X: rpx(12), Y: rpx(25)}, Text: "x", Size: rpx(10), Color: style.RGBA{A: 1}}
	every := func() []Op {
		run := text
		run.PaintedBy = by("DrawText", PartContent)
		return []Op{
			FillRect{Rect: r, Color: style.RGBA{R: 1, A: 1}, PaintedBy: by("FillRect", PartBackground)},
			run,
			DrawTextShadow{Run: text, PaintedBy: by("DrawTextShadow", PartShadow)},
			DrawEmphasisMark{Mark: text, PaintedBy: by("DrawEmphasisMark", PartDecoration)},
			DrawGlyphs{At: text.At, Size: rpx(10), Color: style.RGBA{A: 1},
				Glyphs: []shape.Glyph{{GID: 1}}, PaintedBy: by("DrawGlyphs", PartContent)},
			FillGradient{Clip: r, Tile: r, StepX: r.W, StepY: r.H, PaintedBy: by("FillGradient", PartBackground),
				Gradient: Gradient{Kind: LinearGradient, End: Point{Y: rpx(20)}, Stops: []GradientStop{{Color: style.RGBA{A: 1}}, {Offset: 1, Color: style.RGBA{B: 1, A: 1}}}}},
			FillPath{Path: roundedRect(r, Radii{}), Color: style.RGBA{G: 1, A: 1}, PaintedBy: by("FillPath", PartBorder)},
			Link{Rects: []Rect{r}, Href: "#x", PaintedBy: by("Link", PartContent)},
			ClipPath{Path: roundedRect(r, Radii{}), Ops: inner(), PaintedBy: by("ClipPath", PartGroup)},
			newGroupBy(newFilterGroup([]FilterFunction{{Kind: FilterBlur, StdDev: rpx(1)}}, inner()), by("FilterGroup", PartGroup)),
			newGroupBy(newTransformGroup(rotation(30), inner()), by("TransformGroup", PartGroup)),
		}
	}
	roundBy := &Box{}
	q, _, _ := classify(affine{a: 1, d: 1, e: 5})
	for _, c := range []struct {
		name string
		pass func(*testing.T, []Op) []Op
	}{
		{"opacity", func(_ *testing.T, ops []Op) []Op { out, _ := dimOps(ops, 0, 0.5); return out }},
		{"a clip", func(_ *testing.T, ops []Op) []Op {
			return clipOps(ops, 0, Clip{Rect: Rect{X: rpx(20), Y: rpx(0), W: rpx(100), H: rpx(100)}, Active: true})
		}},
		{"an overhang", func(_ *testing.T, ops []Op) []Op { markOverhang(ops); return ops }},
		{"a quarter turn", func(t *testing.T, ops []Op) []Op {
			out, why := q.mapOps(ops)
			if why != "" {
				t.Fatal(why)
			}
			return out
		}},
		{"a colour matrix", func(t *testing.T, ops []Op) []Op {
			out, ok := foldMatrix(ops, grayscaleMatrix(1))
			if !ok {
				t.Skip("the matrix was not folded into this list")
			}
			return out
		}},
		{"a curve", func(_ *testing.T, ops []Op) []Op {
			var chain *roundClip
			out, _ := roundOps(ops, 0, chain.with(Rect{X: rpx(0), Y: rpx(0), W: rpx(30), H: rpx(30)},
				Radii{TopLeft: Corner{X: rpx(15), Y: rpx(15)}}, roundBy))
			return out
		}},
		{"the links gathered", func(_ *testing.T, ops []Op) []Op { return gatherLinks(ops) }},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := c.pass(t, every())
			if len(out) == 0 {
				t.Fatal("the pass left nothing to check")
			}
			var walk func([]Op)
			walk = func(ops []Op) {
				for _, op := range ops {
					kind := strings.TrimPrefix(fmt.Sprintf("%T", op), "layout.")
					got := OpPaintedBy(op).Box
					switch {
					case got == boxes[kind]:
					case kind == "FillRect" && got == boxes["inner"]:
					case kind == "ClipPath" && got == roundBy:
					default:
						t.Errorf("a %s came out painted by %p, not by the box it went in with", kind, got)
					}
					switch v := op.(type) {
					case ClipPath:
						walk(v.Ops)
					case FilterGroup:
						walk(v.Ops)
					case TransformGroup:
						walk(v.Ops)
					}
				}
			}
			walk(out)
		})
	}
}

// newGroupBy is a group made the engine's way, attributed.
func newGroupBy[G FilterGroup | TransformGroup](g G, by PaintedBy) Op {
	switch v := any(g).(type) {
	case FilterGroup:
		v.PaintedBy = by
		return v
	case TransformGroup:
		v.PaintedBy = by
		return v
	}
	return nil
}

// TestDisplayContentsKeepsItsElement is the review of #964 from pdf0: an
// element with "display: contents" generates no box, and a walk of the box
// tree resolved its text to the paragraph around it — "bonjour" declared in
// the paragraph's language, and a contents <a>'s text and Link placed outside
// the link. Element resolves through the document's tree instead.
func TestDisplayContentsKeepsItsElement(t *testing.T) {
	out := composeAttributed(t, `<p lang=en>a <span style="display: contents" lang=fr>bonjour</span> b `+
		`<a style="display: contents" href="https://e.org/">dc <b>bold</b></a></p>`+
		`<p><a style="display: contents" href="https://f.org/"><b>first</b> then</a></p>`, "", Options{})
	texts := map[string]string{}
	links := map[string]string{}
	for _, op := range out.Ops {
		switch v := op.(type) {
		case DrawText:
			if w := strings.TrimSpace(v.Text); w != "" {
				texts[w] = paintedElement(v)
			}
		case Link:
			links[v.Href] = paintedElement(v)
		}
	}
	for word, want := range map[string]string{"bonjour": "span", "dc": "a", "bold": "b", "a": "p", "then": "a"} {
		if got := texts[word]; got != want {
			t.Errorf("%q leads to <%s>, not <%s>", word, got, want)
		}
	}
	for href, want := range map[string]string{"https://e.org/": "a", "https://f.org/": "a"} {
		if got := links[href]; got != want {
			t.Errorf("the link to %s leads to <%s>, not <%s>", href, got, want)
		}
	}
	for _, op := range out.Ops {
		if v, ok := op.(DrawText); ok && strings.TrimSpace(v.Text) == "bonjour" {
			if lang, _ := v.PaintedBy.Element().AttrExact("lang"); lang != "fr" {
				t.Errorf("\"bonjour\" is in an element whose lang is %q, not fr", lang)
			}
		}
	}
}
