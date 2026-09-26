package layout

import (
	"math/rand"
	"reflect"
	"sort"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// The pieces of the inline boxes that paint, recorded a line at a time
// (inlineDecor.addLine) and then given their insets (insetCarriers).
//
// Both were quadratic in how deeply painting boxes nest. addLine walked the
// whole chain of painting boxes round every item on the line, so a word inside
// d bordered spans — each span bringing its two inset items — asked about d
// boxes for each of some 3d items. insetCarriers then walked every piece for
// every box. Both are a walk of the items and a walk of the pieces now, and are
// held here to the walks they replaced.

// nestedPaintingLine is one line of d spans, each nested in the one before and
// each painting, with its two insets round a word: the shape of
// "<span style='border:1px solid'>" written d times.
func nestedPaintingLine(depth int) (*layouter, []inlineItem, []style.Unit, []style.Unit) {
	l := &layouter{inlineDraws: map[*Box]bool{}, inlineChains: map[*Box][]*Box{},
		inlineAligns: map[*Box]vAlignState{}}
	unit := func(n int) style.Unit { u, _ := style.FromPx(float64(n)); return u }
	parent := &Box{Outer: OuterBlock, Style: style.Initial()}
	var items []inlineItem
	var spans []*Box
	for i := 0; i < depth; i++ {
		span := &Box{Outer: OuterInline, Style: style.Initial(), Parent: parent}
		l.inlineDraws[span] = true
		spans = append(spans, span)
		items = append(items,
			inlineItem{Box: span, Inset: true, InsetLead: true, Width: unit(1)},
			inlineItem{Box: &Box{Outer: OuterInline, Inner: InnerText, Style: style.Initial(), Parent: span},
				Text: "w", Width: unit(5)})
		parent = span
	}
	for i := depth - 1; i >= 0; i-- {
		items = append(items, inlineItem{Box: spans[i], Inset: true, Width: unit(1)})
	}
	xs := make([]style.Unit, len(items))
	widths := make([]style.Unit, len(items))
	var x style.Unit
	for i, it := range items {
		xs[i], widths[i] = x, it.Width
		x = x.Add(it.Width)
	}
	return l, items, xs, widths
}

// TestInlineDecorationsAreLinearInTheNesting records a line and finds its inset
// carriers, nested and side by side. Planted, the walks this replaced read as
// 16.6 nested and 11.0 side by side, where recording is linear either way and
// only the carriers' walk is left to show.
//
// Timed through costtest.TimeCopies. A line of a thousand nested spans is three
// thousand items of half a kilobyte, and with memory being streamed on the
// machine's other cores — which is what a shared runner is — the linear
// recording of one line read as 5.5 to 8.6, and the side-by-side one as 6.5
// to 9.3: the smaller line stayed in the cache between calls and the larger
// did not. Four lines of the smaller size are the larger's memory.
func TestInlineDecorationsAreLinearInTheNesting(t *testing.T) {
	record := func(line func(int) (*layouter, []inlineItem, []style.Unit, []style.Unit), n int) func() {
		l, items, xs, widths := line(n)
		return func() {
			l.inlineDecorations = 0
			d := &inlineDecor{l: l}
			d.addLine(0, items, xs, widths, 0, 0, nil, 1)
			d.insetCarriers()
		}
	}
	c := costtest.TimeCopies(t, "a line of nested bordered spans",
		func(int) func() { return record(nestedPaintingLine, 250) }, record(nestedPaintingLine, 1000))
	if c.Ratio > 8 {
		t.Errorf("recording 250 nested painting spans took %v and 1000 took %v, a factor "+
			"of %.1f: linear is four and a walk of the chain per item is sixteen",
			c.Small, c.Large, c.Ratio)
	}
	// Side by side rather than nested: the chains are one box long, so
	// recording is linear however it is done, and what is left is giving each
	// box its insets — which walked every piece for every box.
	c = costtest.TimeCopies(t, "a line of bordered spans side by side",
		func(int) func() { return record(sideBySidePaintingLine, 1000) }, record(sideBySidePaintingLine, 4000))
	if c.Ratio > 8 {
		t.Errorf("1000 painting spans side by side took %v and 4000 took %v, a factor of "+
			"%.1f: linear is four and a walk of every piece for every box is sixteen",
			c.Small, c.Large, c.Ratio)
	}
}

// sideBySidePaintingLine is one line of n painting spans, one after another,
// each with its insets round a word.
func sideBySidePaintingLine(n int) (*layouter, []inlineItem, []style.Unit, []style.Unit) {
	l := &layouter{inlineDraws: map[*Box]bool{}, inlineChains: map[*Box][]*Box{},
		inlineAligns: map[*Box]vAlignState{}}
	unit := func(n int) style.Unit { u, _ := style.FromPx(float64(n)); return u }
	block := &Box{Outer: OuterBlock, Style: style.Initial()}
	var items []inlineItem
	for i := 0; i < n; i++ {
		span := &Box{Outer: OuterInline, Style: style.Initial(), Parent: block}
		l.inlineDraws[span] = true
		items = append(items,
			inlineItem{Box: span, Inset: true, InsetLead: true, Width: unit(1)},
			inlineItem{Box: &Box{Outer: OuterInline, Inner: InnerText, Style: style.Initial(), Parent: span},
				Text: "w", Width: unit(5)},
			inlineItem{Box: span, Inset: true, Width: unit(1)})
	}
	xs := make([]style.Unit, len(items))
	widths := make([]style.Unit, len(items))
	var x style.Unit
	for i, it := range items {
		xs[i], widths[i] = x, it.Width
		x = x.Add(it.Width)
	}
	return l, items, xs, widths
}

// addLineByWalking is addLine as it was: the chain of every item walked whole,
// a box's piece kept open while the item before held it.
func addLineByWalking(d *inlineDecor, index int, items []inlineItem, xs, widths []style.Unit,
	at, baseline style.Unit, stack *lineStack, scale float64) {
	order := make([]int, 0, len(items))
	for k := range items {
		if items[k].Width == 0 && !items[k].Inset {
			continue
		}
		order = append(order, k)
	}
	sort.SliceStable(order, func(a, b int) bool { return xs[order[a]] < xs[order[b]] })
	open := map[*Box]int{}
	lastAt := map[*Box]int{}
	for pos, k := range order {
		item := items[k]
		chain := d.l.inlineChain(item)
		if len(chain) == 0 {
			continue
		}
		left := at.Add(xs[k])
		right := left.Add(widths[k])
		if sp := item.EdgeLetterSpacing; sp != 0 && !item.Inset {
			right = right.Sub(sp)
		}
		for _, box := range chain {
			if pi, ok := open[box]; ok && lastAt[box] == pos-1 {
				if left < d.pieces[pi].left {
					d.pieces[pi].left = left
				}
				if right > d.pieces[pi].right {
					d.pieces[pi].right = right
				}
				lastAt[box] = pos
				continue
			}
			if !d.room(box) {
				continue
			}
			_, seen := d.last[box]
			d.pieces = append(d.pieces, inlinePiece{
				box: box, line: index, left: left, right: right,
				baseline: baseline, first: !seen, scale: scale,
			})
			if d.last == nil {
				d.last = make(map[*Box]int)
			}
			d.last[box] = len(d.pieces) - 1
			open[box] = len(d.pieces) - 1
			lastAt[box] = pos
		}
	}
}

// insetCarriersByWalking is insetCarriers as it was: every piece walked for
// every box.
func insetCarriersByWalking(d *inlineDecor) map[*Box]insetEnds {
	type span struct{ first, last int }
	lines := map[*Box]span{}
	for _, p := range d.pieces {
		s, ok := lines[p.box]
		if !ok {
			lines[p.box] = span{p.line, p.line}
			continue
		}
		s.first, s.last = min(s.first, p.line), max(s.last, p.line)
		lines[p.box] = s
	}
	out := map[*Box]insetEnds{}
	for b, s := range lines {
		startsRight := beginsAtRight(b)
		startAt, endAt := -1, -1
		for i, p := range d.pieces {
			if p.box != b {
				continue
			}
			if p.line == s.first && (startAt < 0 || further(p, d.pieces[startAt], startsRight)) {
				startAt = i
			}
			if p.line == s.last && (endAt < 0 || further(p, d.pieces[endAt], !startsRight)) {
				endAt = i
			}
		}
		out[b] = insetEnds{start: startAt, end: endAt}
	}
	return out
}

// TestInlineDecorationsAreWhatWalkingSays holds the two to the walks on random
// lines of random trees: boxes that paint and boxes that do not, left-to-right
// and right-to-left, items out of visual order, zero-width items, items with a
// letter-spacing gap, and a cap on the pieces low enough to be reached.
func TestInlineDecorationsAreWhatWalkingSays(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	rtl := style.Initial().With("direction", "rtl")
	unit := func(n int) style.Unit { u, _ := style.FromPx(float64(n)); return u }
	saved := maxInlineDecorations
	defer func() { maxInlineDecorations = saved }()
	compared := 0
	for trial := 0; trial < 2000; trial++ {
		l := &layouter{inlineDraws: map[*Box]bool{}, inlineChains: map[*Box][]*Box{},
			inlineAligns: map[*Box]vAlignState{}, rec: NewRecorder(nil)}
		block := &Box{Outer: OuterBlock, Style: style.Initial()}
		boxes := []*Box{block}
		for n := 1 + r.Intn(12); n > 0; n-- {
			b := &Box{Outer: OuterInline, Style: style.Initial(), Parent: boxes[r.Intn(len(boxes))]}
			if r.Intn(3) == 0 {
				b.Style = rtl
			}
			l.inlineDraws[b] = r.Intn(3) != 0
			boxes = append(boxes, b)
		}
		lines := make([][]inlineItem, 1+r.Intn(3))
		for k := range lines {
			for n := r.Intn(14); n > 0; n-- {
				b := boxes[r.Intn(len(boxes))]
				it := inlineItem{Box: b, Width: unit(r.Intn(6))}
				switch r.Intn(3) {
				case 0:
					it.Inset = true
				case 1:
					it.Box = &Box{Outer: OuterInline, Inner: InnerText, Style: style.Initial(), Parent: b}
					it.Text = "w"
					if r.Intn(3) == 0 {
						it.EdgeLetterSpacing = unit(1)
					}
				}
				lines[k] = append(lines[k], it)
			}
		}
		maxInlineDecorations = 1 << 16
		if r.Intn(5) == 0 {
			maxInlineDecorations = r.Intn(8)
		}
		run := func(add func(d *inlineDecor, k int, items []inlineItem, xs, ws []style.Unit)) *inlineDecor {
			l.inlineDecorations, l.inlineDecorCapped = 0, false
			d := &inlineDecor{l: l}
			rr := rand.New(rand.NewSource(int64(trial)))
			for k, items := range lines {
				xs := make([]style.Unit, len(items))
				ws := make([]style.Unit, len(items))
				for i, it := range items {
					xs[i], ws[i] = unit(rr.Intn(40)), it.Width
				}
				add(d, k, items, xs, ws)
			}
			return d
		}
		got := run(func(d *inlineDecor, k int, items []inlineItem, xs, ws []style.Unit) {
			d.addLine(k, items, xs, ws, unit(3), unit(7), nil, 1)
		})
		want := run(func(d *inlineDecor, k int, items []inlineItem, xs, ws []style.Unit) {
			addLineByWalking(d, k, items, xs, ws, unit(3), unit(7), nil, 1)
		})
		if !reflect.DeepEqual(got.pieces, want.pieces) || !reflect.DeepEqual(got.last, want.last) {
			t.Fatalf("trial %d: the pieces are\n%v\nand walking says\n%v", trial, got.pieces, want.pieces)
		}
		if g, w := got.insetCarriers(), insetCarriersByWalking(want); !reflect.DeepEqual(g, w) {
			t.Fatalf("trial %d: the inset carriers are %v and walking says %v", trial, g, w)
		}
		if len(want.pieces) > 1 {
			compared++
		}
	}
	if compared < 500 {
		t.Fatalf("only %d trials made more than one piece; the lines are not exercising the walk", compared)
	}
}
