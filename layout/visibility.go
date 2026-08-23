package layout

import "strings"

// visibility: a box that is laid out and not painted.
//
// CSS 2.1 §11.2. The property was registered and unread, so "visibility: hidden"
// showed the box — which is the one failure mode in this whole cluster that can
// leak information rather than merely look wrong.
//
// # Why this cannot prune the box tree
//
// The obvious implementation is to drop the box, and it is wrong twice over.
//
// A hidden box still takes part in layout: it occupies its space, its margins
// collapse, it makes room for itself among its siblings, and a float inside it
// still shortens the lines beside it. That is the whole difference between
// "visibility: hidden" and "display: none", and it is why authors reach for the
// first when they want to reserve room for something that is not there yet.
//
// And visibility *inherits*, so a descendant may set "visibility: visible" and
// reappear inside a hidden ancestor. That is not a corner case — it is how a
// hidden container with one visible child is written — and it means the decision
// cannot be made once for a subtree at all. Every box answers for itself, from
// the value the cascade already gave it, and the answer for a parent says nothing
// about its children.
//
// So the property is read at painting time and nowhere else, per box and per
// run: a run of text belongs to the inline box it came from, which may be
// visible inside a hidden block.
//
// # collapse
//
// "visibility: collapse" on a table row or column removes the track and lets the
// rest of the table close up, which is a layout effect rather than a painting
// one. That is done where the sizes are — see rowHeights and
// tableColumnDemands — and isCollapsedTrack below is what those ask.
//
// Everywhere else the value means "hidden", which is what the specification says
// it computes to, so there is nothing to report about it: a collapsed inline is
// a hidden inline and was drawn as one.
//
// The half that is not done is §17.5.5's clipping — a cell spanning into a
// collapsed track keeps its span, and the content that fell in the track should
// be cut. reportCollapsedSpans names the cells it happens to.

// isHidden reports whether a box's own marks are suppressed.
func isHidden(b *Box) bool {
	if b == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(b.Style["visibility"])) {
	case "hidden", "collapse":
		return true
	}
	return false
}

// isCollapsedTrack reports whether a box is a table row or column the author
// asked to be taken out of the table.
//
// §17.5.5: "If a table row or column is collapsed, the row or column is not
// rendered, and the table is laid out as if the row or column did not exist."
// It is the one place "collapse" means something other than "hidden", and the
// difference is a layout effect: a hidden row is invisible and still holds its
// height open, a collapsed one is gone and the table closes up.
//
// A row *group* or a column group carries the value down to what is in it, which
// is what makes "visibility: collapse" on a <tbody> hide the whole body — the
// property inherits, so each row inside it answers "collapse" for itself and
// there is nothing extra to do here.
func isCollapsedTrack(b *Box) bool {
	if b == nil {
		return false
	}
	switch b.Inner {
	case InnerTableRow, InnerTableColumn:
	default:
		return false
	}
	return strings.EqualFold(strings.TrimSpace(b.Style["visibility"]), "collapse")
}

// reportCollapsedSpans names the half of §17.5.5 this engine does not do.
//
// The rule has two halves. The first is that a collapsed row or column is not
// rendered and the table closes up, and that is done — see rowHeights and
// tableColumnDemands. The second is about a cell that spans *into* a collapsed
// track: it keeps its span, and "the contents of spanned cells that intersect
// with a collapsed column are clipped". Nothing here clips them, so a cell
// spanning a collapsed column comes out with its text whole and one column
// narrower to draw it in.
//
// It is reported per cell rather than per track, because *which* cell is drawing
// text it should have lost is the actionable part — and it is reported at all
// because the difference is a few characters in the wrong place, which is
// exactly the kind of wrong that reads as deliberate.
//
// A row group or column group is never asked. The property inherits, so each row
// or column inside one answers "collapse" for itself and the group has nothing
// left to say.
func (l *layouter) reportCollapsedSpans(table *Box, g *tableGrid) {
	for _, c := range g.cells {
		if !spansCollapsed(g, c) {
			continue
		}
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(c.box)),
			Message: "this cell spans a table row or column with \"visibility: " +
				"collapse\"; the track is taken out and the table closes up, but the " +
				"part of the cell's content that fell in it is drawn rather than clipped",
			Path:     PathOf(c.box.Element),
			Property: "visibility",
		})
	}
}

// spansCollapsed reports whether a cell is *partly* in a track that is not
// rendered, which is the only shape the clipping rule is about.
//
// A cell wholly inside a collapsed row is not clipped, it is gone: the row is
// not rendered and neither is anything in it. A cell that reaches from a
// collapsed track into a rendered one is the case §17.5.5 describes, and is the
// only case where a reader would see content that should have been cut.
func spansCollapsed(g *tableGrid, c *tableCell) bool {
	return partlyCollapsed(c.col, c.colSpan, len(g.colBoxes), func(k int) bool {
		return isCollapsedTrack(g.colBoxes[k])
	}) || partlyCollapsed(c.row, c.rowSpan, len(g.rows), func(k int) bool {
		return isCollapsedTrack(g.rows[k].box)
	})
}

// whollyCollapsed reports whether every track a cell covers is one that is not
// rendered, which is a cell that is not rendered either.
//
// It is what keeps a descendant that sets "visibility: visible" inside a
// collapsed row off the page. The property inherits and a child may turn it back
// on — that is the whole reason visibility is asked per box at painting time —
// but §17.5.5 is not about visibility any more once the row is collapsed: the
// row "is not rendered", and a box that is not rendered has no inside for a
// child to reappear in.
//
// Leaving the fragment out is how that is said, and it is the same thing
// "empty-cells: hide" does a few lines below: the cell's width and height have
// already been counted, so there is nothing left of it but marks.
func whollyCollapsed(g *tableGrid, c *tableCell) bool {
	return allCollapsed(c.col, c.colSpan, len(g.colBoxes), func(k int) bool {
		return isCollapsedTrack(g.colBoxes[k])
	}) || allCollapsed(c.row, c.rowSpan, len(g.rows), func(k int) bool {
		return isCollapsedTrack(g.rows[k].box)
	})
}

// allCollapsed reports whether every track in a run is collapsed. An empty run
// is not, which is what keeps a cell outside the grid from vanishing.
func allCollapsed(from, span, n int, collapsed func(int) bool) bool {
	seen := false
	for k := from; k < from+span && k < n; k++ {
		if !collapsed(k) {
			return false
		}
		seen = true
	}
	return seen
}

// partlyCollapsed reports whether a run of tracks holds both kinds.
func partlyCollapsed(from, span, n int, collapsed func(int) bool) bool {
	any, all := false, true
	for k := from; k < from+span && k < n; k++ {
		if collapsed(k) {
			any = true
		} else {
			all = false
		}
	}
	return any && !all
}
