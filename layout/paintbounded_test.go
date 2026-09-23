package layout

import (
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// What one declaration can make the painter emit, and what an opacity group
// costs to check. Audit C14 and C21.

// fillCount is how many rectangles a display list paints.
func fillCount(ops []Op) int { return len(fillsOfAny(ops)) }

// TestAFineTilingIsOneRectanglePerStripe is C14's own document: two hard
// stops tiled at a pixel over a 600 by 800 box was 960,000 fills. The stripes
// are horizontal and the tiles abut across the box, so each stripe of each row
// is one rectangle the width of the box: two per row.
func TestAFineTilingIsOneRectanglePerStripe(t *testing.T) {
	ops := paintOf(t, bandBox, `#d { width: 600px; height: 800px;
		background-image: linear-gradient(red 50%, green 50%); background-size: 1px 1px }`)
	red := fillsOf(ops, style.RGBA{R: 255, A: 1})
	if len(red) != 800 {
		t.Fatalf("%d red fills, want one per row, 800", len(red))
	}
	if red[0].W.Px() != 600 {
		t.Errorf("a red stripe is %v px wide, want the box's 600", red[0].W.Px())
	}
	if n := fillCount(ops); n > 2000 {
		t.Errorf("the box painted %d fills, want about 1600", n)
	}
}

// TestATilingPastTheBoundIsItsAverage: past what a layer is drawn as tile
// by tile, the layer is the colour a reader sees there — the average, alpha
// premultiplied — and the box is told the drawing is not exact.
//
// The bound is lowered to reach it. At its real value a layer has to be tens of
// thousands of rectangles after merging, and layout's own tile cap refuses a
// tiling of more than a million tiles before it gets here; a test document
// that sits between the two is a test that takes seconds to say this.
func TestATilingPastTheBoundIsItsAverage(t *testing.T) {
	old := maxLayerMarks
	maxLayerMarks = 100
	defer func() { maxLayerMarks = old }()
	root := layoutOf(t, A4.Content().W.Px(), bandBox, `#d { width: 600px; height: 800px;
		background-image: linear-gradient(red 50%, transparent 50%);
		background-size: 1px 1px }`)
	rec := NewRecorder(nil)
	ops := PaintReporting(root, rec)
	if n := fillCount(ops); n > 10 {
		t.Fatalf("%d fills for a tiling past the bound", n)
	}
	var avg []FillRect
	for _, f := range fillsOfAny(ops) {
		if f.Color.R == 255 && f.Color.G == 0 {
			avg = append(avg, f)
		}
	}
	if len(avg) != 1 || avg[0].Color.A < 0.45 || avg[0].Color.A > 0.55 {
		t.Fatalf("want one half-transparent red fill, got %v", avg)
	}
	requireFinding(t, rec.Findings(), RuleLimit, "drawn as its average colour")
}

// TestAGappedTilingIsAveragedWithItsGaps: "space" leaves gaps between tiles
// that no merge can close, and the gaps count as transparent in the average.
func TestAGappedTilingIsAveragedWithItsGaps(t *testing.T) {
	old := maxLayerMarks
	maxLayerMarks = 16
	defer func() { maxLayerMarks = old }()
	l := bgPaint{
		Clip: Rect{W: bgpx(100), H: bgpx(100)}, Tile: Rect{W: bgpx(1), H: bgpx(2)},
		StepX: bgpx(2), StepY: bgpx(2),
	}
	p := &painter{rec: NewRecorder(nil)}
	p.tiling(l, []bgBand{{Rect: Rect{W: bgpx(1), H: bgpx(2)}, Color: style.RGBA{G: 128, A: 0.5}}}, nil)
	if len(p.ops) != 1 {
		t.Fatalf("%d ops, want the one average", len(p.ops))
	}
	if f := p.ops[0].(FillRect); f.Color.A != 0.25 || f.Color.G != 128 {
		t.Errorf("the average is %v, want half-transparent green at half again", f.Color)
	}
}

// TestTilingsAreChargedToTheDocument: many elements sharing one tiling are
// the product no per-layer bound sees. The document's budget cuts the tilings
// past it to their average, and the rest of the page is drawn.
func TestTilingsAreChargedToTheDocument(t *testing.T) {
	root := layoutOf(t, A4.Content().W.Px(), `<p>words</p>`+strings.Repeat(`<div class="t"></div>`, 50),
		`.t { width: 100px; height: 400px; background-image:
			linear-gradient(to right, red 50%, green 50%); background-size: 1px 1px;
			background-repeat: space }`)
	lowWork(t, 1<<20)
	rec := NewRecorder(nil)
	ops := PaintReporting(root, rec)
	requireCut(t, rec.Findings(), "the background tilings past that point, drawn as their average colour")
	if !strings.Contains(drawnText(ops), "words") {
		t.Error("the page's text was not drawn")
	}
}

// TestASubPixelDottedBorderIsDrawnInPixels is C14's other document: a dotted
// border 0.02px wide round a 1000px box was 128,000 fills, a dot every
// twenty-fifth of a pixel. A dot is a pixel long now, as browsers draw it.
func TestASubPixelDottedBorderIsDrawnInPixels(t *testing.T) {
	ops := paintOf(t, `<div id="d"></div>`,
		`#d { width: 1000px; height: 1000px; border: 0.02px dotted blue }`)
	if n := fillCount(ops); n > 2100 || n < 1000 {
		t.Errorf("the border is %d fills; a pixel-long dot every two pixels round "+
			"4000 pixels of edge is about 2000", n)
	}
}

// TestDashesAreChargedToTheDocument: past the budget an edge is drawn solid,
// the budget says so, and the page is still drawn.
func TestDashesAreChargedToTheDocument(t *testing.T) {
	root := layoutOf(t, A4.Content().W.Px(), `<p>words</p>`+strings.Repeat(`<div class="b"></div>`, 50),
		`.b { width: 600px; height: 600px; border: 1px dotted blue }`)
	lowWork(t, 1<<20)
	rec := NewRecorder(nil)
	ops := PaintReporting(root, rec)
	requireCut(t, rec.Findings(), "the dashes and dots of the borders past that point, drawn solid")
	if !strings.Contains(drawnText(ops), "words") {
		t.Error("the page's text was not drawn")
	}
}

// TestTheMarksAreChargedToTheDocument: every operation is paid for, and past
// the budget the marks are what is left out, and said to be.
func TestTheMarksAreChargedToTheDocument(t *testing.T) {
	root := layoutOf(t, A4.Content().W.Px(), strings.Repeat(`<p>words</p>`, 200), ``)
	lowWork(t, 50*costOp)
	rec := NewRecorder(nil)
	ops := PaintReporting(root, rec)
	requireCut(t, rec.Findings(), "the marks past that point")
	if len(ops) == 0 || len(ops) > 50 {
		t.Errorf("%d operations on a budget for 50", len(ops))
	}
}

// gridMarks is n rectangles side by side, a hundred to a row, none touching
// another's area.
func gridMarks(n int) []groupMark {
	out := make([]groupMark, n)
	for i := range out {
		x, y := float64(i%100)*2, float64(i/100)*2
		out[i] = groupMark{rect: Rect{X: bgpx(x), Y: bgpx(y), W: bgpx(2), H: bgpx(2)}}
	}
	return out
}

// TestAnOpacityGroupIsCheckedInLinearithmicTime is C21: the faithfulness
// check compared every pair of marks, and a group of marks that lie side by
// side — a tiled background, a table of cells — never exits early.
//
// Timed, where the nested groups below are counted: a group is charged for its
// marks once, before it compares them, so the budget sees how many marks were
// checked and not how many comparisons checking them took.
func TestAnOpacityGroupIsCheckedInLinearithmicTime(t *testing.T) {
	made := map[int][]groupMark{}
	requireLinear(t, "an opacity group's marks", 16000, func(n int) {
		if made[n] == nil {
			made[n] = gridMarks(n)
		}
		g := &group{box: &Box{}, alpha: 0.5, marks: made[n]}
		g.settle(NewRecorder(nil))
		if g.overlap != marksApart {
			t.Fatalf("%d marks side by side were found to overlap", n)
		}
	})
}

// TestTheSweepAgreesWithEveryPair is the sweep checked against the rule it
// replaced, on rectangles that touch, nest, share edges and cross.
func TestTheSweepAgreesWithEveryPair(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for trial := 0; trial < 2000; trial++ {
		n := 2 + r.IntN(8)
		marks := make([]groupMark, n)
		for i := range marks {
			marks[i] = groupMark{rect: Rect{
				X: bgpx(float64(r.IntN(10))), Y: bgpx(float64(r.IntN(10))),
				W: bgpx(float64(1 + r.IntN(4))), H: bgpx(float64(1 + r.IntN(4))),
			}}
		}
		want := false
		for i, a := range marks {
			for _, b := range marks[i+1:] {
				if !a.rect.Intersect(b.rect).Empty() {
					want = true
				}
			}
		}
		if got := anyOverlap(marks); got != want {
			t.Fatalf("the sweep says %v and every pair says %v for %v", got, want, marks)
		}
	}
}

// TestAMarkIsHeldOnceHoweverDeeplyOpacityNests: every mark was appended to
// every group around it, so nested opacity multiplied the memory by the
// depth. Each mark is its innermost group's now, and the answers are the same.
func TestAMarkIsHeldOnceHoweverDeeplyOpacityNests(t *testing.T) {
	const depth = 30
	root := layoutOf(t, A4.Content().W.Px(),
		strings.Repeat(`<div class="o">`, depth)+strings.Repeat(`<div class="f"></div>`, 20)+
			strings.Repeat(`</div>`, depth),
		`.o { opacity: 0.9 } .f { height: 10px; background: blue }`)
	p := &painter{colors: map[string]style.RGBA{}, rec: NewRecorder(nil)}
	p.dimming(root, 1, nil)
	p.stackingContext(root)
	held := 0
	for _, g := range p.groups {
		held += len(g.marks)
	}
	if held != 20 {
		t.Errorf("20 marks are held %d times over %d nested groups", held, depth)
	}
	p.settleGroups()
	for _, b := range p.order {
		if why := p.groups[b].unfaithful(); why != "" {
			t.Errorf("a group of fills side by side was reported: %s", why)
		}
	}
}

// TestAGroupPastTheBoundIsReportedUnchecked: a group with more marks than are
// compared is not silently faithful.
func TestAGroupPastTheBoundIsReportedUnchecked(t *testing.T) {
	old := maxGroupMarks
	maxGroupMarks = 100
	defer func() { maxGroupMarks = old }()
	g := &group{box: &Box{}, alpha: 0.5, marks: gridMarks(200)}
	g.settle(NewRecorder(nil))
	if why := g.unfaithful(); !strings.Contains(why, "not checked") {
		t.Errorf("a group past the bound says %q", why)
	}
}

// requireLinearWork fails when the shape at 4n charges the document's work
// budget more than eight times what the shape at n does. spend builds the
// shape at a size and returns the steps it charged.
//
// It is requireLinear for work the budget is charged for, counted rather than
// timed: a count is the same number on any machine under any load, and cannot
// be spoiled by the rest of the suite running beside it. See costtest.Count,
// which also fails a shape at n that charged nothing.
func requireLinearWork(t *testing.T, what string, n int, spend func(n int) int64) {
	t.Helper()
	a, b := spend(n), spend(4*n)
	if ratio := costtest.Count(t, what, a, b); ratio > 8 {
		t.Errorf("%s: four times the input charged %d steps to the budget against %d, "+
			"%.1f times; linear work is about four, and quadratic is about sixteen",
			what, b, a, ratio)
	}
}

// spentBy is the steps f charges to rec's work budget. A refusal fails the
// test: work that was refused was not done, so what was counted is the bound
// and not the work.
func spentBy(t *testing.T, rec *Recorder, f func()) int64 {
	t.Helper()
	before, cut := rec.workLeft(), len(rec.work.cut)
	f()
	if len(rec.work.cut) > cut {
		t.Fatalf("the work budget refused %q, so what was counted is the bound "+
			"and not the work", rec.work.cut[cut:])
	}
	return before - rec.workLeft()
}

// TestNestedGroupsCheckTheirMarksOnce: a group with one child and nothing of
// its own has its child's answer, so a chain of nested opacity reads its marks
// once and not once per level. Depth and marks both grow, because it was their
// product.
//
// Counted: a group that checks its marks charges the budget for each of them
// before it does, so a chain that checked them again at every level charges
// depth × marks, which is the product this is about.
func TestNestedGroupsCheckTheirMarksOnce(t *testing.T) {
	requireLinearWork(t, "nested opacity groups", 1, func(n int) int64 {
		depth, marks := 50*n, 1000*n
		groups := make([]*group, depth)
		for i := range groups {
			groups[i] = &group{box: &Box{}, alpha: 0.5}
			if i > 0 {
				groups[i-1].children = append(groups[i-1].children, groups[i])
			}
		}
		groups[depth-1].marks = gridMarks(marks)
		rec := NewRecorder(nil)
		spent := spentBy(t, rec, func() {
			for i := depth - 1; i >= 0; i-- {
				groups[i].settle(rec)
			}
		})
		if groups[0].overlap != marksApart {
			t.Fatal("marks side by side were found to overlap")
		}
		return spent
	})
}

// TestAnOverlapInsideIsAnOverlapOutside: a group's marks include its
// children's, so two marks that lie over each other inside a nested group lie
// over each other in every group around it too — and each of those is folded
// mark by mark as well, so each is reported.
func TestAnOverlapInsideIsAnOverlapOutside(t *testing.T) {
	inner := &group{box: &Box{}, alpha: 0.5, marks: []groupMark{
		{rect: Rect{W: bgpx(10), H: bgpx(10)}},
		{rect: Rect{X: bgpx(5), W: bgpx(10), H: bgpx(10)}},
	}}
	outer := &group{box: &Box{}, alpha: 0.5, children: []*group{inner}}
	rec := NewRecorder(nil)
	inner.settle(rec)
	outer.settle(rec)
	if inner.unfaithful() == "" || outer.unfaithful() == "" {
		t.Errorf("inner %q, outer %q; both lie over each other", inner.unfaithful(),
			outer.unfaithful())
	}
}
