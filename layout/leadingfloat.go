package layout

// A float at the start of a line, and the marker that used to hide it.
//
// §9.5 puts a float that begins a line at the top of that line's box rather than
// beside it: there is nothing beside it yet. The line loop finds those by
// walking forward from where the line starts while the items are floats, and
// stops at the first item that is not one.
//
// An absolutely positioned box is an item that is not one. It takes no room, it
// is not on the line in any sense a reader would recognise, and it is placed
// once the line is settled — but it stopped that walk, so a float written after
// it was taken for a float met *part way along* the line and was placed beside
// content that was not there. The suite's css-text/text-indent/below-float and
// its two neighbours are that exactly:
//
//	<div style="text-indent:50px; width:100px">
//	  <div style="position:absolute; top:50px"></div>
//	  <div style="float:left; width:100px; height:50px"></div>
//	  x
//	</div>
//
// The float is as wide as the block, so nothing fits beside it and the first
// line goes below it. With the marker in front of it the float was measured
// against the room left after the indent, did not fit, and was pushed down a
// line — then the line was pushed below *that*, and the page came out a line
// short with the red background showing through.

// floatsBeforeOutOfFlow moves each float ahead of the out-of-flow markers
// immediately before it.
//
// Both are items that take no room, so their order among themselves is not
// something the flow can see: no width moves, no line changes height, and the
// absolutely positioned box is still on the same line and at the same pen
// position it was. What it does change is that the walk for the floats that
// begin a line reaches them.
//
// It is a stable partition of each run rather than a sort, so two floats keep
// their order and so do two markers — which is what decides the order they are
// painted in.
func floatsBeforeOutOfFlow(items []inlineItem) []inlineItem {
	for i := 0; i < len(items); {
		if items[i].Abs == nil {
			i++
			continue
		}
		// A run of markers, and whatever follows it.
		j := i
		for j < len(items) && items[j].Abs != nil {
			j++
		}
		k := j
		for k < len(items) && items[k].Float != nil {
			k++
		}
		if k == j {
			// Nothing to move: what follows the markers is not a float. The
			// scan has to be advanced past them by hand here, because the
			// rotation below is what advances it in the other case and a
			// rotation of nothing would leave it where it is.
			i = j + 1
			continue
		}
		rotate(items[i:k], j-i)
		i = i + (k - j)
	}
	return items
}

// rotate moves the first n items of a slice to the end of it, keeping the order
// within each part.
//
// Three reversals rather than a scratch slice: the runs here are short, and a
// paragraph may hold a great many of them.
func rotate(items []inlineItem, n int) {
	reverse(items[:n])
	reverse(items[n:])
	reverse(items)
}

func reverse(items []inlineItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
