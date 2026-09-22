package layout

import (
	"math/rand/v2"
	"sort"
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS Color 4 §3.1: opacity.
//
// # What the property asks for, and what this engine does
//
// "opacity: 0.5" is not "paint everything in here half-transparent". It is
// "paint everything in here, as a group, onto a surface of its own, and then
// blend that surface into the page at half strength". The difference shows the
// moment two marks in the group overlap: as a group, the upper mark hides the
// lower one and only the result is dimmed; mark by mark, the lower one shows
// through the upper. A green square under a blue square is blue in the first
// reading and a blend of the two in the second.
//
// This engine does the second, and applies the alpha to each mark. The reason is
// the one visualeffects.go gives for clipping, and it is the same reason twice:
//
//	a clip travels *on* the operation it applies to, and nothing in the
//	display list has any state that spans two operations
//
// A group is a push/pop pair by nature — "begin compositing here, end it there"
// — and an unbalanced pair in a content stream does not lose a box, it changes
// how the rest of the page is drawn. The list is built so that no operation can
// do that to another, and a group operation would be the one exception. So the
// alpha is folded into the colour of every mark the group paints, which is a
// rewrite of a span of the list exactly like the one a clip is (see clipping).
//
// # Where the two readings differ, the engine says so
//
// Folding is not always an approximation. A group that paints one mark is
// *exactly* the group: there is nothing for the mark to hide, so dimming it and
// dimming the surface it would have been alone on give the same colour. A group
// at "opacity: 0" is exact too, whatever it paints, because everything in it is
// dropped. Those two cases are most of what documents write — a translucent
// panel, a faded caption, an element being hidden — and each of them comes out
// right.
//
// The rest are reported, per box, saying which of the two it is. That is the
// same shape as the writing-mode report: a property this engine applies to some
// boxes and approximates on others is a fact about the box and not about the
// declaration, so a table keyed by property has no way to say it and the finding
// is raised where the box is painted. See writingmode.go, whose report moved out
// of style/unimplemented.go for exactly this reason — as this one just has.

// opacityOf is the fraction of a box's own paint that reaches the page.
//
// Anything that is not a number or a percentage is 1, which is the initial
// value: a declaration this engine cannot read is a declaration it has not
// applied, and the value that stands for "not applied" is the one that changes
// nothing. Out-of-range values are clamped rather than rejected, because §3.1
// says to clamp them — "opacity: 2" is opaque and "opacity: -1" is invisible.
func opacityOf(cs style.ComputedStyle) float64 {
	raw := strings.TrimSpace(cs.Get("opacity"))
	if raw == "" {
		return 1
	}
	scale := 1.0
	if pct, ok := strings.CutSuffix(raw, "%"); ok {
		raw, scale = strings.TrimSpace(pct), 100
	}
	n, ok := parseNumber(raw)
	if !ok {
		return 1
	}
	switch n /= scale; {
	case n <= 0:
		return 0
	case n >= 1:
		return 1
	}
	return n
}

// groupsItsPaint reports whether a box's own opacity makes a group of it.
func groupsItsPaint(b *Box) bool { return b != nil && opacityOf(b.Style) < 1 }

// groupMark is one mark a group painted, in the terms the faithfulness question
// needs: where it is, and whether the fold could be done to it at all.
type groupMark struct {
	// rect is where the mark is, when the display list states it. A text run
	// does not state one — a DrawText carries a pen position and the glyphs are
	// measured from the face — so a run's rect is empty and the check below
	// treats it as able to overlap anything.
	rect  Rect
	text  bool
	image bool
}

// group is one box that asked for opacity, together with what it painted.
//
// marks are the group's own: what was painted by fragments whose innermost
// group this is. What its nested groups painted is theirs, and reached through
// children — a mark is held once, however deeply opacity nests around it.
type group struct {
	box      *Box
	alpha    float64
	marks    []groupMark
	children []*group

	// What settle found, for this group and everything inside it: how many
	// marks there are, whether one is a picture, and whether any two lie over
	// each other — or whether that was not checked. See settle.
	count   int
	image   bool
	overlap overlapAnswer
	settled bool
}

// overlapAnswer is what the faithfulness check found about a group's marks.
type overlapAnswer uint8

const (
	marksApart overlapAnswer = iota
	marksOverlap
	marksUnchecked
)

// maxGroupMarks bounds the marks one group's check compares.
//
// The comparison is a sweep, n log n in the marks, and a group is compared with
// everything inside it, so a mark is read once for each group around it that
// has more than one thing inside. A box that tiles a gradient at a pixel is a
// million marks; past this many the check is not made, and the group is
// reported as unchecked rather than silently as faithful.
//
// A variable so that a test can lower it.
var maxGroupMarks = 1 << 16

// dimOps folds an alpha into every mark from at onwards, and says what it had to
// work with.
//
// A picture is the one mark that cannot take an alpha: a DrawImage and a
// TileImage carry pixels rather than a colour, and there is nothing in either to
// multiply. At an alpha of zero there is still an exact answer — the picture is
// not painted — and that is the one case where dropping an operation is right
// rather than a way of hiding one that could not be handled.
func dimOps(ops []Op, at int, alpha float64) ([]Op, []groupMark) {
	marks := make([]groupMark, 0, len(ops)-at)
	kept := ops[:at]
	for _, op := range ops[at:] {
		switch v := op.(type) {
		case FillRect:
			if v.Rect.Empty() || v.Color.A == 0 {
				kept = append(kept, op)
				continue
			}
			marks = append(marks, groupMark{rect: v.Rect})
			if alpha == 0 {
				continue
			}
			v.Color.A *= alpha
			kept = append(kept, v)
		case DrawText:
			if v.Color.A == 0 || v.Text == "" {
				kept = append(kept, op)
				continue
			}
			marks = append(marks, groupMark{text: true})
			if alpha == 0 {
				continue
			}
			v.Color.A *= alpha
			kept = append(kept, v)
		case DrawImage:
			marks = append(marks, groupMark{rect: v.Rect, image: true})
			if alpha == 0 {
				continue
			}
			kept = append(kept, op)
		case TileImage:
			marks = append(marks, groupMark{rect: v.Clip, image: true})
			if alpha == 0 {
				continue
			}
			kept = append(kept, op)
		default:
			kept = append(kept, op)
		}
	}
	return kept, marks
}

// settleGroups works out, for every group, what unfaithful reports.
//
// Inner groups first — p.order has every group after the one around it, so
// backwards is children before parents — so that a group reads what its
// children already found rather than finding it again.
func (p *painter) settleGroups() {
	for i := len(p.order) - 1; i >= 0; i-- {
		p.groups[p.order[i]].settle(p.rec)
	}
}

// settle decides whether a group's marks, and its children's, lie over each
// other.
//
// Overlap is the whole rule, and one mark falls out of it rather than being a
// case of its own: a group of one has no pair to test, which is right, because
// there is nothing under that mark to show through and dimming it is dimming
// the surface it was alone on. A group of none is the same.
//
// It was every pair, which is n² in the marks and exits early only on a text
// mark, a picture or an overlap: a box tiling a gradient at four pixels was
// 60,000 marks that lie side by side and took eleven seconds, and a pixel
// would have been most of an hour (audit C21). Now it is a sweep, n log n; a
// group whose child already overlaps overlaps too, since its marks are a
// superset; and a group with only one thing inside it has the answer that
// thing has. What is left is charged to the document's work budget, and is
// reported as unchecked past maxGroupMarks or the budget, rather than as
// faithful.
func (g *group) settle(rec *Recorder) {
	g.settled = true
	g.count = len(g.marks)
	g.overlap = marksApart
	parts := 0
	if len(g.marks) > 0 {
		parts++
	}
	for _, m := range g.marks {
		g.image = g.image || m.image
	}
	for _, c := range g.children {
		g.count += c.count
		g.image = g.image || c.image
		if c.count > 0 {
			parts++
		}
		switch c.overlap {
		case marksOverlap:
			g.overlap = marksOverlap
		case marksUnchecked:
			if g.overlap == marksApart {
				g.overlap = marksUnchecked
			}
		}
	}
	if g.image || g.overlap != marksApart || g.count < 2 {
		// A picture is reported whatever else is true of the group, and an
		// answer read from a child needs nothing more.
		return
	}
	if parts == 1 && len(g.marks) == 0 {
		// Everything is inside one child, which has answered for it.
		return
	}
	if g.count > maxGroupMarks ||
		!rec.charge(int64(g.count)*costMarkCompared, "the opacity checks past that point") {
		g.overlap = marksUnchecked
		return
	}
	all := make([]groupMark, 0, g.count)
	var gather func(*group)
	gather = func(h *group) {
		all = append(all, h.marks...)
		for _, c := range h.children {
			gather(c)
		}
	}
	gather(g)
	for _, m := range all {
		if m.text {
			// A run states no rectangle, so it may lie over anything, and
			// there is at least one other mark for it to lie over.
			g.overlap = marksOverlap
			return
		}
	}
	if anyOverlap(all) {
		g.overlap = marksOverlap
	}
}

// anyOverlap reports whether any two of the rectangles share area, in
// n log n.
//
// A sweep across the page from left to right, holding the rectangles the
// sweep line crosses. Those must be apart from each other — the moment two are
// not, the answer is yes and the sweep stops — so they are disjoint intervals
// down the page, kept in order, and a new one can only meet the one above it
// or the one below. Leaving comes before entering at the same x, because two
// rectangles that only touch share no area and Rect.Intersect says so.
func anyOverlap(marks []groupMark) bool {
	type event struct {
		x     style.Unit
		enter bool
		at    int
	}
	events := make([]event, 0, 2*len(marks))
	for i, m := range marks {
		if m.rect.Empty() {
			continue
		}
		events = append(events, event{m.rect.X, true, i}, event{m.rect.Right(), false, i})
	}
	sort.Slice(events, func(i, j int) bool {
		a, b := events[i], events[j]
		if a.x != b.x {
			return a.x < b.x
		}
		return !a.enter && b.enter
	})
	var active intervalSet
	for _, e := range events {
		r := marks[e.at].rect
		if !e.enter {
			active.remove(r.Y)
			continue
		}
		if !active.insert(r.Y, r.Bottom()) {
			return true
		}
	}
	return false
}

// intervalSet is a set of disjoint intervals down the page, ordered by where
// they begin: a treap, whose priorities are random so that no order of
// insertion a document can arrange makes it deep.
type intervalSet struct {
	root *interval
	rng  *rand.Rand
}

type interval struct {
	lo, hi      style.Unit
	pri         uint64
	left, right *interval
}

// insert adds [lo, hi) and reports whether it is apart from every interval
// already there. One that is not is not added: the answer is known.
func (s *intervalSet) insert(lo, hi style.Unit) bool {
	// The last interval beginning at or before lo, and the first beginning
	// after it. Only those two can meet [lo, hi): the rest are disjoint from
	// them and further away on the far side of one of them.
	var below, above *interval
	for n := s.root; n != nil; {
		if n.lo <= lo {
			below, n = n, n.right
		} else {
			above, n = n, n.left
		}
	}
	if below != nil && below.hi > lo {
		return false
	}
	if above != nil && above.lo < hi {
		return false
	}
	if s.rng == nil {
		s.rng = rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64()))
	}
	a, b := splitIntervals(s.root, lo)
	s.root = mergeIntervals(mergeIntervals(a, &interval{lo: lo, hi: hi, pri: s.rng.Uint64()}), b)
	return true
}

// remove takes out the interval beginning at lo.
func (s *intervalSet) remove(lo style.Unit) {
	a, b := splitIntervals(s.root, lo)
	_, c := splitIntervals(b, lo+1)
	s.root = mergeIntervals(a, c)
}

// splitIntervals divides a treap into the intervals beginning before at and
// the rest.
func splitIntervals(n *interval, at style.Unit) (*interval, *interval) {
	if n == nil {
		return nil, nil
	}
	if n.lo < at {
		l, r := splitIntervals(n.right, at)
		n.right = l
		return n, r
	}
	l, r := splitIntervals(n.left, at)
	n.left = r
	return l, n
}

// mergeIntervals joins two treaps, every interval of a before every one of b.
func mergeIntervals(a, b *interval) *interval {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a.pri > b.pri:
		a.right = mergeIntervals(a.right, b)
		return a
	default:
		b.left = mergeIntervals(a, b.left)
		return b
	}
}

// unfaithful is why folding the alpha into a group's marks is not the group, or
// the empty string when it is. It reads what settle found.
//
// The order of the tests is the order an author needs to hear them in. A picture
// in the group is the one the engine could do nothing at all about, so it is
// said first even where the group overlaps as well: "the picture is opaque" is
// what they will see, and "the marks blend into each other" would send them
// looking at the wrong thing.
func (g group) unfaithful() string {
	if !g.settled {
		// A group built by hand, which nothing has settled.
		g.settle(nil)
	}
	if g.alpha == 0 {
		// Nothing was painted, which is what the group would have come to.
		return ""
	}
	switch {
	case g.image:
		return "a picture carries its own pixels and there is no colour in " +
			"it to dim, so it is painted as though the box were opaque"
	case g.overlap == marksOverlap:
		return "the box paints marks that lie over each other, and each " +
			"is dimmed on its own, so the lower one shows through the " +
			"upper instead of being hidden by it"
	case g.overlap == marksUnchecked:
		return "the box paints more marks than this engine compares, so " +
			"whether any lie over each other was not checked, and where they " +
			"do the lower one shows through the upper"
	}
	return ""
}

// report says what a group could not express, at the box that asked for it.
func (g group) report(rec *Recorder) {
	why := g.unfaithful()
	if why == "" || rec == nil {
		return
	}
	rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(g.box)),
		Message: "\"opacity: " + strings.TrimSpace(g.box.Style.Get("opacity")) +
			"\" was applied to each mark this box paints rather than to the box " +
			"as a group, because " + why,
		Path:     PathOf(g.box.Element),
		Property: "opacity",
	})
}
