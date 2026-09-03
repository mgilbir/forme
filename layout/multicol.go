package layout

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS Multi-column Layout 1: a block whose content is poured into columns.
//
// # What it is
//
// A multicol container lays its content out once, in a box one column wide, and
// then cuts the result into columns and stands them side by side. That is not an
// implementation shortcut — it is what the specification describes, and it is
// why the feature is called fragmentation rather than layout: the content is
// laid out and *then* divided, so nothing inside it needs to know how many
// columns there are or where they end.
//
// It is also this engine's first fragmentation of any kind. Nothing else here
// cuts a laid-out box in two — the page is one page and a block is one block —
// so the pieces below are the primitive rather than a use of one.
//
// # What is not done
//
// A great deal, and each of it is refused with a finding rather than
// approximated. Anything that would have to be cut *through* is refused: a box
// with a border, a background or an outline crossing a column boundary is two
// halves of a drawn thing, and CSS says which edges each half keeps. A float
// inside the columns is refused because a float is placed against a formatting
// context that has no notion of which column it is in. "column-span" is refused
// because a spanning element divides the multicol into two of them.
//
// The gate is the same shape as the writing-mode one and for the same reason:
// what is refused is laid out exactly as it was before this file existed, in one
// column, and is reported — which is the honest answer — while what is accepted
// is the answer CSS gives.

// columnFill is CSS Multi-column §3.5's column-fill.
type columnFill uint8

const (
	// columnBalance is the initial value: the columns are as short as they can
	// be while holding the content, and equal.
	columnBalance columnFill = iota
	// columnAuto fills each column to the available height in turn, so the last
	// one is as short as what is left.
	columnAuto
)

// columns is what §3.4's algorithm resolves a box's declarations to.
type columns struct {
	// n is the used column count and width the used column width. The two are
	// resolved together: §3.4 takes the count and the width as constraints and
	// produces a pair that fills the available space exactly.
	n     int
	width style.Unit
	gap   style.Unit
	fill  columnFill
}

// columnGap is §6.1's gap, whose initial value "normal" is one em for a multicol
// container.
//
// Resolved here rather than in the cascade because an em is the box's own, and a
// computed value of "1em" would resolve against whatever font the reader of the
// computed style happened to have.
func (l *layouter) columnGap(b *Box) style.Unit {
	if v, ok := l.lengthOf(b, "column-gap", 0); ok && v >= 0 {
		return v
	}
	return b.FontSize
}

// columnsFor resolves §3.4's algorithm for a box, or says the box is not a
// multicol container.
//
// available is the width the content has: the box's content box, which the
// columns and the gaps between them divide exactly.
//
// The algorithm is the specification's, written out in its own order. Both
// declarations are constraints and neither is a result: a count says how many
// columns there are to be and a width says how narrow they may be, and where
// both are given the count is a maximum that the width may reduce.
func (l *layouter) columnsFor(b *Box, available style.Unit) (columns, bool) {
	count, hasCount := columnCount(b)
	width, hasWidth := l.lengthOf(b, "column-width", 0)
	if hasWidth && width <= 0 {
		// A zero or negative width is not a length this can divide by. §3.2
		// makes it invalid; the cascade admits it, so it is declined here.
		hasWidth = false
	}
	if !hasCount && !hasWidth {
		// "column-width: auto" and "column-count: auto" together: not a
		// multicol container at all, which is the ordinary case for every box
		// in every document.
		return columns{}, false
	}
	gap := l.columnGap(b)
	out := columns{gap: gap, fill: columnFillOf(b)}
	switch {
	case !hasWidth:
		out.n = count
	case !hasCount:
		out.n = fitCount(available, width, gap)
	default:
		if n := fitCount(available, width, gap); n < count {
			out.n = n
		} else {
			out.n = count
		}
	}
	if out.n < 1 {
		out.n = 1
	}
	// The used width, which is what is left once the gaps are taken out, shared
	// equally. It is not the declared column-width: that is a minimum the used
	// value is at least as large as, and the columns fill the box.
	out.width = available.Sub(gap.Mul(float64(out.n - 1))).Div(float64(out.n))
	if out.width < 0 {
		out.width = 0
	}
	return out, true
}

// fitCount is how many columns of a given width fit in the available space with
// a gap between each pair: §3.4's "floor((available + gap) / (width + gap))".
func fitCount(available, width, gap style.Unit) int {
	if width+gap <= 0 {
		return 1
	}
	n := int((available.Add(gap)).Px() / (width.Add(gap)).Px())
	if n < 1 {
		return 1
	}
	return n
}

// columnCount reads §3.1's property: a positive integer, or auto.
func columnCount(b *Box) (int, bool) {
	raw := strings.TrimSpace(b.Style["column-count"])
	if raw == "" || strings.EqualFold(raw, "auto") {
		return 0, false
	}
	n, ok := positiveInteger(raw)
	if !ok || n < 1 {
		return 0, false
	}
	return n, true
}

// columnFillOf reads §3.5's property.
func columnFillOf(b *Box) columnFill {
	if strings.EqualFold(strings.TrimSpace(b.Style["column-fill"]), "auto") {
		return columnAuto
	}
	return columnBalance
}

// splitAt divides a fragment's content at a height, and is this engine's only
// fragmentation.
//
// The fragment given is the one holding the content — a multicol container, or
// a box inside one — and y is a distance down its *content* box. What comes back
// is the part above y left in place, and the part below it moved up so that y
// becomes its own zero. Either may be nil: content entirely above the cut leaves
// nothing below, and content entirely below it leaves nothing above.
//
// A box that has to be cut through is copied, once for each side, and each copy
// keeps the lines and the children that fall on its side. That is what CSS
// describes for a fragmented box, and it is why the two halves are fragments of
// one box rather than two boxes: they share a Box, so everything that asks what
// generated them gets one answer.
//
// It refuses — false — where the cut would go through something drawn. A border,
// a background or an outline crossing a column boundary is a picture with edges
// CSS says which half keeps, and guessing is worse than reporting.
func splitAt(f *Fragment, y style.Unit) (top, bottom *Fragment, ok bool) {
	if f == nil {
		return nil, nil, true
	}
	above, below := *f, *f
	above.Lines, above.Children = nil, nil
	below.Lines, below.Children = nil, nil

	for _, line := range f.Lines {
		switch {
		case line.Rect.Bottom() <= y:
			above.Lines = append(above.Lines, line)
		case line.Rect.Y >= y:
			line.Rect.Y = line.Rect.Y.Sub(y)
			below.Lines = append(below.Lines, line)
		default:
			// A line box straddling the cut. A line is not divisible — it is
			// the unit fragmentation works in — so this is not a height the
			// caller may cut at, and columnBreaks is what stops it choosing one.
			return nil, nil, false
		}
	}
	for _, c := range f.Children {
		switch {
		case c.BorderRect.Bottom() <= y:
			kept := *c
			above.Children = append(above.Children, &kept)
		case c.BorderRect.Y >= y:
			moved := *c
			moved.BorderRect.Y = moved.BorderRect.Y.Sub(y)
			below.Children = append(below.Children, &moved)
		default:
			// A box the cut goes through. Its content is divided by the same
			// rule, one level down, and the two halves keep the box between
			// them: a fragmented box is one box in two pieces, so they share a
			// Box and everything that asks what generated them gets one answer.
			//
			// The cut is in the parent's content coordinates and the child's
			// content is in the child's, so it is moved by the distance between
			// the two. With no border and no padding — which is all this
			// accepts — that distance is the child's own top edge.
			if drawsAnything(c) || c.Padding != (Edges{}) {
				return nil, nil, false
			}
			t, b, fine := splitAt(c, y.Sub(c.BorderRect.Y))
			if !fine {
				return nil, nil, false
			}
			if t != nil {
				t.BorderRect.H = y.Sub(t.BorderRect.Y)
				above.Children = append(above.Children, t)
			}
			if b != nil {
				b.BorderRect.H = c.BorderRect.Bottom().Sub(y)
				b.BorderRect.Y = 0
				below.Children = append(below.Children, b)
			}
		}
	}
	if len(above.Lines) == 0 && len(above.Children) == 0 {
		return nil, &below, true
	}
	if len(below.Lines) == 0 && len(below.Children) == 0 {
		return &above, nil, true
	}
	return &above, &below, true
}

// columnBreaks collects the heights at which a column may end, in increasing
// order and measured down the fragment's content box.
//
// They are the bottoms of the line boxes and of the boxes that hold none: a line
// is the unit fragmentation works in, so a cut anywhere else would go through
// one. Everything the balancing below chooses comes from this list, which is
// what makes splitAt's refusal of a straddled line a guard rather than a case.
func columnBreaks(f *Fragment, at style.Unit, out []style.Unit) []style.Unit {
	if f == nil {
		return out
	}
	for _, line := range f.Lines {
		out = append(out, at.Add(line.Rect.Bottom()))
	}
	for _, c := range f.Children {
		if len(c.Lines) == 0 && len(c.Children) == 0 {
			out = append(out, at.Add(c.BorderRect.Bottom()))
			continue
		}
		out = columnBreaks(c, at.Add(c.BorderRect.Y), out)
		out = append(out, at.Add(c.BorderRect.Bottom()))
	}
	return out
}

// fillColumns pours a subtree laid out in one tall column into n columns of a
// given height, side by side.
//
// It is the whole of the layout half: the content was laid out once, at the
// column width, by the ordinary block code that knows nothing about columns, and
// this cuts the result into bands and stands them beside each other.
func fillColumns(f *Fragment, c columns, height style.Unit) bool {
	if height <= 0 {
		return false
	}
	bands := make([]*Fragment, 0, c.n)
	rest := f
	for i := 0; i < c.n && rest != nil; i++ {
		top, bottom, ok := splitAt(rest, height)
		if !ok {
			return false
		}
		bands = append(bands, top)
		rest = bottom
	}
	if rest != nil {
		// More content than the columns hold. §3.6 overflows it out of the last
		// column, which is a fragmentation of its own and is not done here.
		return false
	}
	f.Lines, f.Children = nil, nil
	for i, band := range bands {
		if band == nil {
			continue
		}
		dx := c.width.Add(c.gap).Mul(float64(i))
		for _, line := range band.Lines {
			line.Rect.X = line.Rect.X.Add(dx)
			f.Lines = append(f.Lines, line)
		}
		for _, child := range band.Children {
			child.BorderRect.X = child.BorderRect.X.Add(dx)
			f.Children = append(f.Children, child)
		}
	}
	return true
}

// balancedHeight is §3.5's "balance": the shortest the columns can be while
// still holding the content between them.
//
// The candidates are the breakpoints and nothing else, because a column ends at
// one: a height between two of them holds exactly as much as the lower of the
// two and is taller for nothing. So the search is over the list rather than over
// the numbers, and the answer is the first candidate the content fits inside.
func balancedHeight(breaks []style.Unit, n int) (style.Unit, bool) {
	for _, h := range breaks {
		if fitsColumns(breaks, n, h) {
			return h, true
		}
	}
	return 0, false
}

// fitsColumns reports whether content whose breakpoints are these fits in n
// columns of the given height, filled greedily.
func fitsColumns(breaks []style.Unit, n int, height style.Unit) bool {
	if len(breaks) == 0 {
		return true
	}
	used, start := 1, style.Unit(0)
	for _, at := range breaks {
		if at.Sub(start) <= height {
			continue
		}
		// This piece does not fit in the column being filled, so the column
		// ended at the breakpoint before it. Nothing here needs to know which
		// one that was: what is counted is the columns, and the piece that did
		// not fit begins the next.
		used++
		start = previousBreak(breaks, at)
		if at.Sub(start) > height {
			// One piece taller than a whole column. No number of columns holds
			// it, and a taller column is the only answer.
			return false
		}
	}
	return used <= n
}

// previousBreak is the breakpoint before this one, or zero.
func previousBreak(breaks []style.Unit, at style.Unit) style.Unit {
	prev := style.Unit(0)
	for _, b := range breaks {
		if b >= at {
			break
		}
		prev = b
	}
	return prev
}

// drawsAnything reports whether a fragment puts ink on the page of its own —
// a background, a border, an outline or a banded background.
//
// It is the question splitAt asks before it cuts a box in half, and it errs
// towards yes: a box that draws nothing can be divided without anybody seeing
// where, and a box that draws is a picture whose edges CSS assigns.
func drawsAnything(f *Fragment) bool {
	if f == nil || f.Box == nil {
		return false
	}
	if len(f.bgBands) > 0 || f.Outline > 0 || f.Border != (Edges{}) {
		return true
	}
	if v := strings.TrimSpace(f.Box.Style["background-color"]); v != "" &&
		!strings.EqualFold(v, "transparent") {
		return true
	}
	return strings.TrimSpace(f.Box.Style["background-image"]) != "" &&
		!strings.EqualFold(strings.TrimSpace(f.Box.Style["background-image"]), "none")
}

// canColumn is whether a box's content is of a kind this engine can pour into
// columns, and why not where it is not.
//
// It is asked of the box tree before anything is laid out, for the reason the
// writing-mode gate is: the content of a multicol container is laid out at the
// *column* width, so a box that turns out to be unfragmentable afterwards has
// been measured against the wrong line length. What is refused here is laid out
// in one column at the box's own width, exactly as it was before this file
// existed, and is reported.
func (l *layouter) canColumn(b *Box) string {
	if b.Outer != OuterBlock || (b.Inner != InnerFlow && b.Inner != InnerFlowRoot) {
		return "it is not an ordinary block box"
	}
	if writingModeOf(b).vertical() {
		// Two fragmentations at once. The columns of a vertical multicol
		// container run down the page and its lines run across them, and the
		// turn is written for a box whose content is one column.
		return "its writing mode is vertical, and this engine turns a box or " +
			"columns it, not both"
	}
	return l.subtreeCanColumn(b, b)
}

func (l *layouter) subtreeCanColumn(root, b *Box) string {
	if b != root {
		if b.Float != FloatNone {
			// A float is placed against a formatting context, and a formatting
			// context has no notion of which column it is in. §5.2 says what
			// should happen and it is a fragmentation of the float itself.
			return "it holds a float, which is placed against a formatting " +
				"context that does not know about columns"
		}
		if b.Position != PositionStatic {
			return "it holds a positioned box"
		}
		if b.Replaced != nil || b.Control != nil || b.ListItem || b.MarkerImage != nil {
			return "it holds a replaced element, a form control or a list marker"
		}
		switch b.Inner {
		case InnerFlow, InnerText:
		default:
			return "it holds a table or another box with sizing rules of its own"
		}
		if spansColumns(b) {
			// §6.3's "column-span: all" divides the multicol container into two
			// of them with the spanning element between, which is a second
			// container and not a column.
			return "\"column-span: all\" is declared inside it, which divides " +
				"the container rather than filling a column"
		}
	}
	for _, c := range b.Children {
		if why := l.subtreeCanColumn(root, c); why != "" {
			return why
		}
	}
	return ""
}

// spansColumns reads §6.3's column-span.
func spansColumns(b *Box) bool {
	return strings.EqualFold(strings.TrimSpace(b.Style["column-span"]), "all")
}

// reportColumns says a box asked for columns and did not get them.
func (l *layouter) reportColumns(b *Box, n int, why string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "this box asked for " + plural(n, "column") + " and was laid out in " +
			"one, because " + why,
		Path:     PathOf(b.Element),
		Property: "column-count",
	})
}

func plural(n int, what string) string {
	if n == 1 {
		return "one " + what
	}
	return strconv.Itoa(n) + " " + what + "s"
}

// pourIntoColumns divides a multicol container's content and returns the height
// the container comes to, or says the content could not be divided.
//
// The height is §3.5's, and which of the two it is depends on the box: a
// container told how tall to be and asked to fill its columns in turn takes that
// height and lets the last column end short, and one that is not takes the
// shortest height its content fits in — which is what "balance", the initial
// value, asks for.
func (l *layouter) pourIntoColumns(b *Box, frag *Fragment, cols columns,
	contentHeight, declared style.Unit, hasHeight bool) (style.Unit, bool) {

	breaks := sortedBreaks(columnBreaks(frag, 0, nil))
	if len(breaks) == 0 {
		// Nothing to divide. The columns are as tall as nothing, which is what
		// an empty container is either way.
		return contentHeight, true
	}
	height := declared
	if !hasHeight || cols.fill != columnAuto {
		got, ok := balancedHeight(breaks, cols.n)
		if !ok {
			l.reportColumns(b, cols.n, "its content does not divide into that "+
				"many columns of any height this engine can choose")
			return 0, false
		}
		height = got
	}
	if !fillColumns(frag, cols, height) {
		l.reportColumns(b, cols.n, "its content cannot be divided where a column "+
			"would end without cutting through something that is drawn")
		return 0, false
	}
	return height, true
}

// sortedBreaks puts the breakpoints in order and drops the repeats, which the
// walk produces where a box ends at its last line.
func sortedBreaks(in []style.Unit) []style.Unit {
	if len(in) < 2 {
		return in
	}
	sort.Slice(in, func(i, j int) bool { return in[i] < in[j] })
	out := in[:1]
	for _, v := range in[1:] {
		if v != out[len(out)-1] {
			out = append(out, v)
		}
	}
	return out
}
