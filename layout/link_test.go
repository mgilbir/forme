package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// The display list's Link: where an <a href> is, and where it goes. See
// link.go for what is decided and why.
//
// The geometry is inlinepaint_test.go's: Courier at 250px, so every advance and
// every extent is a whole number of layout units and each expected rectangle
// below is derived rather than recorded.

// linksIn is the Links of a display list, in order, with the index of each.
func linksIn(ops []Op) ([]Link, []int) {
	var out []Link
	var at []int
	for i, op := range ops {
		if l, ok := op.(Link); ok {
			out = append(out, l)
			at = append(at, i)
		}
	}
	return out, at
}

// oneLink is the single Link of a display list, or a failure naming what there
// was instead.
func oneLink(t *testing.T, ops []Op) (Link, int) {
	t.Helper()
	got, at := linksIn(ops)
	if len(got) != 1 {
		t.Fatalf("%d links in the display list, want 1: %+v", len(got), got)
	}
	return got[0], at[0]
}

// drawnAt is the index and position of the first run drawing text.
func drawnAt(t *testing.T, ops []Op, text string) (int, Point) {
	t.Helper()
	for i, op := range ops {
		if d, ok := op.(DrawText); ok && d.Text == text {
			return i, d.At
		}
	}
	t.Fatalf("no run draws %q", text)
	return 0, Point{}
}

// TestALinkIsItsBorderBox is one <a> on one line: one Link, one area, and the
// area is the <a>'s border box — §10.6.1's content area with the vertical
// padding and border outside it, the horizontal ones inside, and the margin
// not at all. It is where its line is in the list: after the paragraph before
// it and before its own text.
func TestALinkIsItsBorderBox(t *testing.T) {
	root := layoutOf(t, 4000, `<p>before</p><p id="p">x<a href="https://example.test/a"
		style="margin-left: 7px; padding: 30px 10px; border-left: 5px solid">ab</a>y</p>`,
		courierInk)
	ops := Paint(root)
	l, at := oneLink(t, ops)

	if l.Href != "https://example.test/a" {
		t.Errorf("the link goes to %q", l.Href)
	}
	if l.of != nil {
		t.Error("the Link left Paint still carrying the key it was gathered by")
	}
	base := baselineOfFirstRun(t, root, "p")
	want := Rect{
		X: mustPx(inkAdvance + 7),
		Y: base.Sub(mustPx(inkAscent + 30)),
		W: mustPx(5 + 10 + 2*inkAdvance + 10),
		H: mustPx(30 + inkHeight + 30),
	}
	if len(l.Rects) != 1 || l.Rects[0] != want {
		t.Errorf("the link's area is %v, want the <a>'s border box %v", l.Rects, want)
	}
	if before, _ := drawnAt(t, ops, "before"); at < before {
		t.Errorf("the link is at %d in the list, before the paragraph above it at %d", at, before)
	}
	if own, _ := drawnAt(t, ops, "ab"); at > own {
		t.Errorf("the link is at %d in the list, after its own text at %d", at, own)
	}
}

// TestALinkBrokenAcrossLinesHasAnAreaPerLine: an inline <a> is a fragment per
// line it is on, and the link is one Link with an area for each.
func TestALinkBrokenAcrossLinesHasAnAreaPerLine(t *testing.T) {
	root := layoutOf(t, 400, `<p id="p"><a href="w">ab ab ab</a></p>`, courierInk)
	l, _ := oneLink(t, Paint(root))
	if len(l.Rects) != 3 {
		t.Fatalf("the link has %d areas over three lines: %v", len(l.Rects), l.Rects)
	}
	base := baselineOfFirstRun(t, root, "p")
	for i, r := range l.Rects {
		want := Rect{
			Y: base.Sub(mustPx(inkAscent)).Add(mustPx(400 * float64(i))),
			W: mustPx(2 * inkAdvance), H: mustPx(inkHeight),
		}
		if r != want {
			t.Errorf("area %d is %v, want %v — the word on line %d", i, r, want, i+1)
		}
	}
}

// TestALinkCoversWhatItHolds: an image, an inline-block, a float and a block
// lifted out of an inline <a> each have a fragment of their own, which the
// <a>'s line fragments do not enclose, and a click on any of them is a click
// on the link.
func TestALinkCoversWhatItHolds(t *testing.T) {
	res := mapResolver{"i.svg": bgSVG(`width="10" height="10"`)}
	in := Input{
		HTML: `<div id="p">x<a href="h"><img id="im" src="i.svg" style="width: 40px; height: 700px">` +
			`<span id="ib" style="display: inline-block; width: 50px; height: 900px"></span>` +
			`<span id="fl" style="float: right; width: 60px; height: 70px"></span>` +
			`ab<div id="d">cd</div>ef</a></div>`,
		CSS:       []Stylesheet{{Source: courierInk}},
		Resources: res,
	}
	built := Build(in)
	w, _ := style.FromPx(4000)
	h, _ := style.FromPx(10000)
	root := Layout(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
	l, _ := oneLink(t, Paint(root))

	has := func(r Rect) bool {
		for _, got := range l.Rects {
			if got == r {
				return true
			}
		}
		return false
	}
	if find(t, root, "im").Box.Replaced == nil {
		t.Fatal("the image did not load, so this is not a link around a picture")
	}
	for _, id := range []string{"im", "ib", "fl", "d"} {
		if r := find(t, root, id).BorderRect; !has(r) {
			t.Errorf("#%s's border box %v is not one of the link's areas %v", id, r, l.Rects)
		}
	}
	// And the lines either side of the lifted block, which are the <a>'s own.
	if len(l.Rects) != 6 {
		t.Errorf("the link has %d areas, want 6 — two lines, an image, an "+
			"inline-block, a float and a block: %v", len(l.Rects), l.Rects)
	}
}

// TestABlockOrAtomicLinkIsItsBox: an <a> that is a box of its own is one area,
// its border box, and what is inside it is not added again.
func TestABlockOrAtomicLinkIsItsBox(t *testing.T) {
	for _, display := range []string{"block", "inline-block"} {
		root := layoutOf(t, 4000, `<div><a id="a" href="b" style="display: `+display+`;
			width: 100px; height: 50px; margin: 7px; border: 3px solid; padding: 2px">
			<span style="display: inline-block; width: 10px; height: 10px"></span></a></div>`,
			courierInk)
		l, _ := oneLink(t, Paint(root))
		if want := find(t, root, "a").BorderRect; len(l.Rects) != 1 || l.Rects[0] != want {
			t.Errorf("display: %s: the link's areas are %v, want its border box %v",
				display, l.Rects, want)
		}
	}
}

// TestANestedLinkIsLaterThanTheOneAroundIt: the inner link is the one a click
// follows, and it is later in the list, so a backend that lets the last
// annotation win agrees. XHTML nests <a> in <a> as written; HTML's parser
// closes the outer one first, and nests them only through a table.
func TestANestedLinkIsLaterThanTheOneAroundIt(t *testing.T) {
	for _, tc := range []struct {
		what string
		in   Input
	}{
		{"xhtml", Input{XHTML: true, HTML: `<html xmlns="http://www.w3.org/1999/xhtml"><body>` +
			`<p><a href="o">x <a href="i">y</a> z</a></p></body></html>`}},
		{"a table", Input{HTML: `<a href="o">x<table><tr><td><a href="i">y</a></td></tr></table>z</a>`}},
	} {
		tc.in.CSS = []Stylesheet{{Source: courierInk}}
		built := Build(tc.in)
		w, _ := style.FromPx(4000)
		h, _ := style.FromPx(10000)
		got, at := linksIn(Paint(Layout(built.Root, Size{W: w, H: h}, nil, NewRecorder(nil))))
		if len(got) != 2 || got[0].Href != "o" || got[1].Href != "i" {
			t.Errorf("%s: the links are %+v, want the outer and then the inner", tc.what, got)
			continue
		}
		if at[0] > at[1] {
			t.Errorf("%s: the inner link is at %d and the one around it at %d", tc.what, at[1], at[0])
		}
		inner := got[1].Rects[0]
		var inside bool
		for _, r := range got[0].Rects {
			inside = inside || r.Contains(inner)
		}
		if !inside {
			t.Errorf("%s: the inner link's area %v is in none of the outer's %v",
				tc.what, inner, got[0].Rects)
		}
	}
}

// TestALinkIsMadeOfOnlyTheSchemesALinkIsFor: http, https, mailto and a
// reference with no scheme are links; every other scheme, and a host with its
// scheme left out, is refused and reported, and the <a>'s content is drawn
// either way. The scheme is read as the URL standard reads it, so a tab inside
// one, or a space before it, does not hide it.
func TestALinkIsMadeOfOnlyTheSchemesALinkIsFor(t *testing.T) {
	compose := func(href string) (Composed, bool) {
		out := Compose(Input{HTML: `<p><a href="` + href + `">words</a></p>`}, Options{})
		var refused bool
		for _, f := range out.Findings {
			if f.Rule == RuleLinkRefused {
				refused = true
				fired[RuleLinkRefused] = true
			}
		}
		if !strings.Contains(drawnText(out.Ops), "words") {
			t.Errorf("%q: the link's content was not drawn", href)
		}
		return out, refused
	}
	for _, href := range []string{
		"javascript:alert(1)", "JavaScript:alert(1)", "java&#9;script:alert(1)",
		" data:text/html,&lt;b>x", "file:///etc/passwd", "vbscript:x", "ms-msdt:/id",
		"//evil.test/x", `\\evil.test\x`, "/\\evil.test/x",
	} {
		out, refused := compose(href)
		if got, _ := linksIn(out.Ops); len(got) != 0 {
			t.Errorf("%q is a link to %q", href, got[0].Href)
		}
		if !refused {
			t.Errorf("%q was not made a link and nothing said so", href)
		}
	}
	for href, want := range map[string]string{
		"http://a.test/x":       "http://a.test/x",
		"HTTPS://a.test/x?q#f":  "HTTPS://a.test/x?q#f",
		"mailto:someone@a.test": "mailto:someone@a.test",
		"#terms":                "#terms",
		"":                      "",
	} {
		out, refused := compose(href)
		got, _ := linksIn(out.Ops)
		if len(got) != 1 || got[0].Href != want {
			t.Errorf("%q: the links are %+v, want one to %q", href, got, want)
		}
		if refused {
			t.Errorf("%q is a link and was reported as refused", href)
		}
	}
}

// TestARelativeLinkIsRelativeToTheDocument: a reference with no scheme is
// handed over as the document wrote it, less what the URL standard strips
// before it reads one — the document is its base, and its address is the
// caller's to know. It is not resolved against a stylesheet, as a url() in one
// is, and nothing in it is decoded.
func TestARelativeLinkIsRelativeToTheDocument(t *testing.T) {
	for href, want := range map[string]string{
		"sub/page.html":             "sub/page.html",
		"../up.html?a=1&amp;b=%20c": "../up.html?a=1&b=%20c",
		"/root.html#top":            "/root.html#top",
		" \tsub/pa&#10;ge.html\n ":  "sub/page.html",
	} {
		out := Compose(Input{
			HTML:      `<link rel="stylesheet" href="css/site.css"><p><a href="` + href + `">words</a></p>`,
			Resources: mapResolver{"css/site.css": []byte(`p { margin: 0 }`)},
		}, Options{})
		got, _ := linksIn(out.Ops)
		if len(got) != 1 || got[0].Href != want {
			t.Errorf("%q: the links are %+v, want one to %q", href, got, want)
		}
	}
}

// TestALinkIsCutByWhatClipsIt: an area is narrowed to what is visible of it,
// exactly as a fill there would be, an area clipped away entirely is gone, and
// a link with no area left is not in the list.
func TestALinkIsCutByWhatClipsIt(t *testing.T) {
	// The first line half visible across, the second and third below the box.
	root := layoutOf(t, 4000, `<div style="overflow: hidden; width: 200px; height: 400px">
		<p id="p"><a href="c">ab ab ab</a></p></div>`, courierInk)
	l, _ := oneLink(t, Paint(root))
	base := baselineOfFirstRun(t, root, "p")
	want := Rect{Y: base.Sub(mustPx(inkAscent)), W: mustPx(200), H: mustPx(inkHeight)}
	if len(l.Rects) != 1 || l.Rects[0] != want {
		t.Errorf("the clipped link's areas are %v, want the visible part of the first line, %v",
			l.Rects, want)
	}

	// A box of its own is cut by the clip on it too.
	root = layoutOf(t, 4000, `<div style="overflow: hidden; width: 50px">
		<a href="b" style="display: block; width: 100px; height: 20px"></a></div>`, courierInk)
	l, _ = oneLink(t, Paint(root))
	if len(l.Rects) != 1 || l.Rects[0].W != mustPx(50) || l.Rects[0].H != mustPx(20) {
		t.Errorf("the clipped block link's areas are %v, want 50 by 20", l.Rects)
	}

	// Clipped away entirely, a link is not in the list at all.
	for _, doc := range []string{
		`<div style="overflow: hidden; height: 0"><a href="z">ab</a></div>`,
		`<div style="overflow: hidden; height: 100px"><p>x</p><p><a href="z">ab</a></p></div>`,
	} {
		if got, _ := linksIn(Paint(layoutOf(t, 4000, doc, courierInk))); len(got) != 0 {
			t.Errorf("%s: a link nobody can see is in the list: %+v", doc, got)
		}
	}
}

// TestAHiddenLinkIsNotALink: §11.2's hidden box is not drawn and not clicked,
// and a transparent one is still both, as far as a click is concerned.
func TestAHiddenLinkIsNotALink(t *testing.T) {
	if got, _ := linksIn(paintOf(t, `<p><a href="v" style="visibility: hidden">ab</a></p>`,
		courierInk)); len(got) != 0 {
		t.Errorf("a hidden link is in the list: %+v", got)
	}
	if got, _ := linksIn(paintOf(t, `<p><a href="v" style="opacity: 0">ab</a></p>`,
		courierInk)); len(got) != 1 {
		t.Errorf("a transparent link is not in the list: %+v", got)
	}
}

// TestALinkMovesWithItsText: everything that moves a line after it was laid
// out — a table cell's vertical-align, a multi-column pour, §9.4.3's relative
// offset, a quarter turn — moves the link on it, so the area stays over its
// words.
func TestALinkMovesWithItsText(t *testing.T) {
	for _, tc := range []struct{ what, doc string }{
		{"a middle-aligned cell", `<table><tr><td style="height: 2000px; vertical-align: middle">` +
			`<a href="ab">ab</a></td></tr></table>`},
		// Five lines of one block poured into three columns of two, two and
		// one: the middle column is cut from the rest and the last is what
		// is left, and each is moved up and across from where it was laid out.
		{"a multi-column pour", `<div style="columns: 3; column-gap: 0; width: 900px">` +
			`cd cd <a href="ab">ab</a> cd <a href="ef">ef</a></div>`},
		// The same block as the child of the one poured, which is cut in its
		// turn and whose last piece is what is left of it.
		{"a block cut by a pour", `<div style="columns: 3; column-gap: 0; width: 900px">` +
			`<p>cd cd <a href="ab">ab</a> cd <a href="ef">ef</a></p></div>`},
		{"a relative offset", `<p>x<a href="ab" style="position: relative; left: 10px; top: 5px">ab</a></p>`},
	} {
		ops := paintOf(t, tc.doc, courierInk)
		got, _ := linksIn(ops)
		if len(got) == 0 {
			t.Errorf("%s: no link", tc.what)
		}
		for _, l := range got {
			_, at := drawnAt(t, ops, l.Href)
			want := Rect{X: at.X, Y: at.Y.Sub(mustPx(inkAscent)),
				W: mustPx(2 * inkAdvance), H: mustPx(inkHeight)}
			if len(l.Rects) != 1 || l.Rects[0] != want {
				t.Errorf("%s: the link over %q is at %v and its text's box is %v",
					tc.what, l.Href, l.Rects, want)
			}
		}
	}

	// Turned, the text runs down the page from At and the area is the column
	// it is set in.
	ops := paintOf(t, `<div style="writing-mode: vertical-rl; height: 1000px; width: 1000px">`+
		`<a href="m">ab</a></div>`, courierInk)
	l, _ := oneLink(t, ops)
	_, at := drawnAt(t, ops, "ab")
	if len(l.Rects) != 1 {
		t.Fatalf("the turned link's areas are %v", l.Rects)
	}
	r := l.Rects[0]
	if r.W != mustPx(inkHeight) || r.H != mustPx(2*inkAdvance) || r.Y != at.Y ||
		at.X <= r.X || at.X >= r.Right() {
		t.Errorf("the turned link's area is %v, and its text starts at %v", r, at)
	}
}

// TestLinksAreChargedToTheDocument: an area is a mark as far as the budget is
// concerned, so past it the links are left out with everything else, and said
// to be.
func TestLinksAreChargedToTheDocument(t *testing.T) {
	root := layoutOf(t, A4.Content().W.Px(), strings.Repeat(`<p><a href="x">w</a></p>`, 200), ``)
	lowWork(t, 50*costOp)
	rec := NewRecorder(nil)
	ops := PaintReporting(root, rec)
	requireCut(t, rec.Findings(), "the marks past that point")
	got, _ := linksIn(ops)
	var areas int
	for _, l := range got {
		areas += len(l.Rects)
	}
	if len(ops) > 50 || areas > 50 {
		t.Errorf("%d operations and %d link areas on a budget for 50", len(ops), areas)
	}
	if areas == 0 {
		t.Error("no link was painted at all, so the budget was not what cut them")
	}
}

// TestFindingALinkAboveIsLinear: the link an inline-block is an area of is
// found by walking up through the inline boxes around it, and a document of d
// nested spans with k inline-blocks inside asks it k times. Walked afresh each
// time that is k·d boxes; each box is walked once.
func TestFindingALinkAboveIsLinear(t *testing.T) {
	const d, k = 100, 100
	doc := `<p><a href="x">` + strings.Repeat(`<span>`, d) +
		strings.Repeat(`<span style="display: inline-block"></span>`, k) +
		strings.Repeat(`</span>`, d) + `</a></p>`
	built := Build(Input{HTML: doc})
	var blocks []*Box
	var walk func(b *Box)
	walk = func(b *Box) {
		if isAtomicInline(b) {
			blocks = append(blocks, b)
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if len(blocks) != k {
		t.Fatalf("%d inline-blocks, want %d", len(blocks), k)
	}
	p := &painter{rec: NewRecorder(nil)}
	for _, b := range blocks {
		if l := p.linkOf(b); l == nil || l.href != "x" {
			t.Fatalf("an inline-block inside the link is an area of %+v", l)
		}
	}
	if p.linkSteps > 2*(d+k) {
		t.Errorf("finding the link above %d inline-blocks inside %d spans walked %d boxes, "+
			"want about %d", k, d, p.linkSteps, d+1)
	}
}

// TestALinkDrawsNothing: an <a> that is a link and a <span> that is not paint
// the same page — the same marks, and the same natural size, which is what the
// scale is computed from. The <a>'s areas are made by the same slicing as an
// inline box's background, and one that reached the list of backgrounds would
// be read as ink: at a line-height below the font's, the content area it
// covers reaches past the line, and the page would be scaled for it.
func TestALinkDrawsNothing(t *testing.T) {
	const sheet = `p { font: 16px/4px serif; margin: 0 } a { color: black; text-decoration: none }`
	link := Compose(Input{HTML: `<p>x <a href="h">ab</a> y</p>`,
		CSS: []Stylesheet{{Source: sheet}}}, Options{})
	span := Compose(Input{HTML: `<p>x <span>ab</span> y</p>`,
		CSS: []Stylesheet{{Source: sheet}}}, Options{})
	if got, _ := linksIn(link.Ops); len(got) != 1 {
		t.Fatalf("the link is not in the list: %+v", got)
	}
	var boxes int
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		for _, line := range f.Lines {
			boxes += len(line.Boxes)
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(link.Root)
	if boxes != 0 {
		t.Errorf("the link made %d inline box fragments of ink", boxes)
	}
	var marks []Op
	for _, op := range link.Ops {
		if _, ok := op.(Link); !ok {
			marks = append(marks, op)
		}
	}
	if a, b := renderKey(marks), renderKey(span.Ops); a != b {
		t.Errorf("a link paints differently from a span:\n%s\n%s", a, b)
	}
	if link.NaturalSize != span.NaturalSize || link.Scale != span.Scale {
		t.Errorf("a link's page is %v at %v and a span's %v at %v",
			link.NaturalSize, link.Scale, span.NaturalSize, span.Scale)
	}
}
