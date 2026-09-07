package layout

import (
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestCounterSnapshotsCostOneNumberPerListItem is what the snapshots are for,
// measured.
//
// Every element used to be given a copy of every counter in scope — a map and a
// slice per counter name, per element — and exactly one integer of that was
// ever read: a list item's marker number. A stylesheet naming a thousand
// counters over two thousand elements came to 229 MB of live snapshots, and the
// document need not have a single list in it.
//
// The bound is memory because memory is the quantity: a document of this shape
// is a page of prose with a long stylesheet, and it must not cost megabytes.
func TestCounterSnapshotsCostOneNumberPerListItem(t *testing.T) {
	const elements, names = 2000, 1024

	var css strings.Builder
	css.WriteString(noDefaults)
	css.WriteString("body { counter-reset:")
	for i := 0; i < names; i++ {
		css.WriteString(" c" + strconv.Itoa(i) + " 0")
	}
	css.WriteString(" }")

	var html strings.Builder
	for i := 0; i < elements; i++ {
		html.WriteString("<p>x</p>")
	}
	in := Input{HTML: html.String(), CSS: []Stylesheet{{Source: css.String()}}}

	// Warm every lazily-built table first, so what is measured is the document.
	Build(Input{HTML: "<p>x</p>"})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	built := Build(in)
	runtime.ReadMemStats(&after)

	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	const cap = 64 << 20
	if grew := after.TotalAlloc - before.TotalAlloc; grew > cap {
		t.Errorf("%d elements under %d counter names allocated %d bytes; nothing here "+
			"is a list, so the counters should cost nothing at all", elements, names, grew)
	}
}

// TestAListItemStillKnowsItsNumber is what the snapshots exist for, kept: a
// change that stored nothing would pass the measurement above and number every
// item "1".
func TestAListItemStillKnowsItsNumber(t *testing.T) {
	built := Build(Input{HTML: `<ol><li id="a">a</li><li id="b">b</li><li id="c">c</li></ol>`})
	for i, id := range []string{"a", "b", "c"} {
		box := findBox(t, built.Root, id)
		if box.ListValue != i+1 {
			t.Errorf("item %s is numbered %d, want %d", id, box.ListValue, i+1)
		}
	}
}
