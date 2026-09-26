package layout

import (
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// placeInsetsByRebuilding is placeInsetsBySide as it was first written, kept as
// the statement of what the placement is: for each box with an inset on the
// line, innermost first, take its two insets out of the order, find the first
// and the last item of the box's own content in what is left, and put the insets
// at those two ends.
//
// It rebuilds the whole order for every box, and asks every item on the line
// whether it is inside the box, so a line of n boxes costs n times the line.
// That is why it is not the implementation. It is kept because it is short
// enough to be obviously what §8.6 asks, and the implementation is not.
func placeInsetsByRebuilding(runs []inlineItem, order []int) []int {
	depth := func(b *Box) int {
		n := 0
		for c := b; c != nil; c = c.Parent {
			n++
		}
		return n
	}
	var boxes []*Box
	seen := map[*Box]bool{}
	for _, k := range order {
		if b := heldBox(runs[k].Box); runs[k].Inset && b != nil && !seen[b] {
			seen[b] = true
			boxes = append(boxes, b)
		}
	}
	sort.SliceStable(boxes, func(a, c int) bool {
		return depth(boxes[a]) > depth(boxes[c])
	})
	for _, b := range boxes {
		lead, trail := -1, -1
		rest := make([]int, 0, len(order))
		for _, k := range order {
			if runs[k].Inset && runs[k].Box == b {
				if runs[k].InsetLead {
					lead = k
				} else {
					trail = k
				}
				continue
			}
			rest = append(rest, k)
		}
		lo, hi := -1, -1
		for i, k := range rest {
			if !itemInside(runs[k], b) || runs[k].Width == 0 {
				continue
			}
			if lo < 0 {
				lo = i
			}
			hi = i
		}
		if lo < 0 {
			continue
		}
		left, right := lead, trail
		if beginsAtRight(b) {
			left, right = trail, lead
		}
		out := make([]int, 0, len(order))
		out = append(out, rest[:lo]...)
		if left >= 0 {
			out = append(out, left)
		}
		out = append(out, rest[lo:hi+1]...)
		if right >= 0 {
			out = append(out, right)
		}
		out = append(out, rest[hi+1:]...)
		order = out
	}
	return order
}

// randomInsetLine is one line of a paragraph of randomly nested inline boxes:
// its items in logical order, and a visual order for them.
//
// The shapes are the ones the placement has cases for — boxes with and without
// content, content of no width, atomic inlines, "direction: rtl" boxes whose
// lead goes at the right, and a box whose lead or trail is on another line —
// and the visual order is either the logical one with stretches of it reversed,
// which is what the bidi algorithm makes, or any permutation at all, which is
// what the placement is defined over.
func randomInsetLine(r *rand.Rand) ([]inlineItem, []int) {
	block := &Box{Style: style.Initial()}
	px := func(n int) style.Unit {
		u, _ := style.FromPx(float64(n))
		return u
	}
	var runs []inlineItem
	var emit func(parent *Box, depth int)
	emit = func(parent *Box, depth int) {
		for n := r.Intn(4); n > 0; n-- {
			switch pick := r.Intn(10); {
			case pick < 4 && depth < 5:
				st := style.Initial()
				if r.Intn(3) == 0 {
					st = st.With("direction", "rtl")
				}
				b := &Box{Style: st, Parent: parent}
				// A lead or a trail on another line is left out of this one.
				if r.Intn(8) != 0 {
					runs = append(runs, inlineItem{Box: b, Inset: true, InsetLead: true,
						Width: px(r.Intn(3))})
				}
				emit(b, depth+1)
				if r.Intn(8) != 0 {
					runs = append(runs, inlineItem{Box: b, Inset: true, Width: px(r.Intn(3))})
				}
			case pick < 5:
				atomic := &Box{Style: style.Initial(), Parent: parent}
				runs = append(runs, inlineItem{Box: atomic, AtomicBox: atomic,
					Width: px(r.Intn(3))})
			default:
				text := &Box{Style: style.Initial(), Parent: parent}
				runs = append(runs, inlineItem{Box: text, Text: "x", Width: px(r.Intn(3))})
			}
		}
	}
	emit(block, 0)

	order := make([]int, len(runs))
	for i := range order {
		order[i] = i
	}
	if r.Intn(2) == 0 {
		r.Shuffle(len(order), func(i, j int) { order[i], order[j] = order[j], order[i] })
	} else {
		for n := r.Intn(4); n > 0 && len(order) > 1; n-- {
			i := r.Intn(len(order))
			j := i + r.Intn(len(order)-i)
			slices.Reverse(order[i : j+1])
		}
	}
	return runs, order
}

// TestInsetPlacementIsWhatRebuildingSays holds placeInsetsBySide to the
// statement of it above, over lines of randomly nested boxes. The suite reaches
// the placement through a few dozen documents; this reaches the orderings those
// documents do not have, and every one of them is a place where two ways of
// arriving at "the ends of the box's content" can disagree.
func TestInsetPlacementIsWhatRebuildingSays(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	l := &layouter{}
	moved := 0
	for i := 0; i < 20000; i++ {
		runs, order := randomInsetLine(r)
		want := placeInsetsByRebuilding(runs, slices.Clone(order))
		got := l.placeInsetsBySide(runs, slices.Clone(order))
		if !slices.Equal(got, want) {
			t.Fatalf("line %d, %s\nfrom %v\n got %v\nwant %v", i, describeInsetLine(runs),
				order, got, want)
		}
		if !slices.Equal(want, order) {
			moved++
		}
	}
	// A comparison of two answers that never differ from the question proves
	// nothing about the moving, which is the whole of the work.
	if moved < 5000 {
		t.Fatalf("only %d of the lines moved an inset; the lines are not exercising "+
			"the placement", moved)
	}
}

func describeInsetLine(runs []inlineItem) string {
	ids := map[*Box]int{}
	id := func(b *Box) int {
		if _, ok := ids[b]; !ok {
			ids[b] = len(ids)
		}
		return ids[b]
	}
	var sb strings.Builder
	for k, item := range runs {
		b := heldBox(item.Box)
		switch {
		case item.Inset && item.InsetLead:
			fmt.Fprintf(&sb, "%d:lead(b%d rtl=%v w=%v) ", k, id(b), isRTL(b), item.Width)
		case item.Inset:
			fmt.Fprintf(&sb, "%d:trail(b%d w=%v) ", k, id(b), item.Width)
		default:
			fmt.Fprintf(&sb, "%d:item(in b%d w=%v) ", k, id(b.Parent), item.Width)
		}
	}
	return sb.String()
}

// TestInsetPlacementIsLinearInTheBoxesOnALine is the cost of placeInsetsBySide,
// which was the product of the boxes on a line and the items on it.
//
// For every box with an inset, the order was rebuilt and every item on the line
// was asked whether it sat inside the box — a walk up the box tree for each. A
// paragraph of empty padded spans is the plainest document with that shape:
//
//	<p> + n × <span style="padding:0 2px"></span> + x</p>
//
// took 34 ms through Compose at a thousand spans and 326 ms at four thousand,
// nine and a half times the time for four times the spans. The same held for
// borders and margins, for spans with a letter in each and nothing between them,
// for a <bdo> of them, and for letter-spacing — any line holding many boxes.
//
// Each box's work is its own content now: the items whose nearest box it is, and
// the two ends of each box directly inside it. The shapes below are the flat
// line, the nested one, and one in which every box's insets have to move.
func TestInsetPlacementIsLinearInTheBoxesOnALine(t *testing.T) {
	px := func(n int) style.Unit {
		u, _ := style.FromPx(float64(n))
		return u
	}
	identity := func(n int) []int {
		order := make([]int, n)
		for i := range order {
			order[i] = i
		}
		return order
	}
	shapes := []struct {
		name string
		n    int
		line func(n int) ([]inlineItem, []int)
	}{
		{"n empty padded boxes and a letter", 1000, func(n int) ([]inlineItem, []int) {
			block := &Box{Style: style.Initial()}
			var runs []inlineItem
			for i := 0; i < n; i++ {
				b := &Box{Style: style.Initial(), Parent: block}
				runs = append(runs,
					inlineItem{Box: b, Inset: true, InsetLead: true, Width: px(2)},
					inlineItem{Box: b, Inset: true, Width: px(2)})
			}
			runs = append(runs, inlineItem{Box: &Box{Parent: block}, Text: "x", Width: px(5)})
			return runs, identity(len(runs))
		}},
		{"n padded boxes holding a letter each", 1000, func(n int) ([]inlineItem, []int) {
			block := &Box{Style: style.Initial()}
			var runs []inlineItem
			for i := 0; i < n; i++ {
				b := &Box{Style: style.Initial(), Parent: block}
				runs = append(runs,
					inlineItem{Box: b, Inset: true, InsetLead: true, Width: px(2)},
					inlineItem{Box: &Box{Parent: b}, Text: "w", Width: px(5)},
					inlineItem{Box: b, Inset: true, Width: px(2)})
			}
			return runs, identity(len(runs))
		}},
		{"n right-to-left boxes, each of whose insets moves", 1000, func(n int) ([]inlineItem, []int) {
			block := &Box{Style: style.Initial()}
			rtl := style.Initial().With("direction", "rtl")
			var runs []inlineItem
			for i := 0; i < n; i++ {
				b := &Box{Style: rtl, Parent: block}
				runs = append(runs,
					inlineItem{Box: b, Inset: true, InsetLead: true, Width: px(2)},
					inlineItem{Box: &Box{Parent: b}, Text: "w", Width: px(5)},
					inlineItem{Box: b, Inset: true, Width: px(5)})
			}
			return runs, identity(len(runs))
		}},
		// Fewer, because the old cost of this one was the cube: each of n
		// boxes asked each of 2n items a question that walked n boxes up.
		{"n boxes nested in one another", 100, func(n int) ([]inlineItem, []int) {
			parent := &Box{Style: style.Initial()}
			var runs []inlineItem
			var boxes []*Box
			for i := 0; i < n; i++ {
				b := &Box{Style: style.Initial(), Parent: parent}
				boxes = append(boxes, b)
				runs = append(runs,
					inlineItem{Box: b, Inset: true, InsetLead: true, Width: px(1)},
					inlineItem{Box: &Box{Parent: b}, Text: "a", Width: px(5)})
				parent = b
			}
			for i := n - 1; i >= 0; i-- {
				runs = append(runs, inlineItem{Box: boxes[i], Inset: true, Width: px(1)})
			}
			return runs, identity(len(runs))
		}},
	}
	for _, shape := range shapes {
		t.Run(shape.name, func(t *testing.T) {
			small, large := shape.n, 4*shape.n
			place := func(n int) func() {
				runs, order := shape.line(n)
				l := &layouter{}
				return func() { l.placeInsetsBySide(runs, slices.Clone(order)) }
			}
			r := costtest.Time(t, shape.name, place(small), place(large))
			if r.Ratio > 8 {
				t.Errorf("placing the insets of %d boxes took %v and of %d took %v, "+
					"a factor of %.1f: linear is four and the product of the boxes and "+
					"the items is sixteen", small, r.Small, large, r.Large, r.Ratio)
			}
		})
	}
}
