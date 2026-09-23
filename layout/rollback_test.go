package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// What a pass that is thrown away has to take back. See speculative.go's
// checkpoint, which is the one place it is written down, and the sites that
// used to write it down by hand — each with its own selection of it.

// TestAFailedPourLeavesNoGhostFloat is audit C88's second half. A pour that
// cannot be made is laid out again in one column, and the float the first
// layout placed was left in the context: the second layout put the real float
// beside the invisible one, fifty pixels in from where it belongs.
func TestAFailedPourLeavesNoGhostFloat(t *testing.T) {
	root := layoutOf(t, 400, `<div id=m style="column-count:2;width:400px;height:10px;column-fill:auto">`+
		`<div id=f style="float:left;width:50px;height:50px"></div>`+
		`<p>`+strings.Repeat("word ", 80)+`</p></div><p id=after>after</p>`, noDefaults)
	if f := find(t, root, "f"); f.BorderRect.X != 0 {
		t.Errorf("the float is at x=%v; it is the first thing in the box and belongs at "+
			"its left edge, not beside a copy of itself", f.BorderRect.X.Px())
	}
	if a := find(t, root, "after"); len(a.Lines) == 0 || a.Lines[0].Rect.X != 0 {
		t.Errorf("the paragraph after the box is indented round a float that is not " +
			"there")
	}
}

// TestAMulticolContainerKeepsItsFloats is audit C88's first half. The float is
// cut into the columns with everything else, and its uncut rectangle stayed in
// the parent's context: the paragraph after the container was indented round
// it. css-multicol-1 §2 makes a multicol container a formatting context of its
// own, so nothing inside it reaches the text after it.
func TestAMulticolContainerKeepsItsFloats(t *testing.T) {
	root := layoutOf(t, 400, `<div id=m style="column-count:2;width:400px">`+
		`<div style="float:left;width:50px">`+strings.Repeat("a<br>", 30)+`</div>`+
		`<p>`+strings.Repeat("word ", 5)+`</p></div><p id=after>after</p>`, noDefaults)
	m, after := find(t, root, "m"), find(t, root, "after")
	if after.BorderRect.Y < m.BorderRect.Bottom() {
		t.Fatalf("the paragraph after the box begins at %v, inside it (it ends at %v)",
			after.BorderRect.Y.Px(), m.BorderRect.Bottom().Px())
	}
	if len(after.Lines) == 0 || after.Lines[0].Rect.X != 0 {
		t.Errorf("the paragraph after a multicol container is indented round a float " +
			"inside it")
	}
}

// TestANestedClampCountsEachLineOnce is audit C90. The inner clamp's counting
// pass charged its lines to the outer clamp, and the inner's real pass charged
// them again, so the outer clamp reached its three lines at the inner box and
// dropped everything after it. The three lines are "aaa" and the next two.
func TestANestedClampCountsEachLineOnce(t *testing.T) {
	root := layoutOf(t, 400, `<div style="line-clamp:3"><div style="line-clamp:1">aaa<br>bbb</div>`+
		`<div id=c>ccc</div><div id=d>ddd</div><div id=e>eee</div></div>`, noDefaults)
	for _, id := range []string{"c", "d"} {
		if fragmentFor(root, id) == nil {
			t.Errorf("#%s is not on the page; the outer clamp counted a discarded pass "+
				"of the inner one", id)
		}
	}
	if fragmentFor(root, "e") != nil {
		t.Error("#e is on the page; it is the fourth line of a clamp to three")
	}
}

// TestADiscardedPassLeavesNoInlineFragment is the side state nothing took back
// at all. A column measures its item by laying it out, and a positioned inline
// inside it recorded its fragments there too — and the first of them, from the
// measuring pass, is never made absolute. An absolutely positioned box inside
// the inline was placed against it: a hundred pixels to the left of where the
// inline is, which is the margin the measured fragment never learned about.
func TestADiscardedPassLeavesNoInlineFragment(t *testing.T) {
	root := layoutOf(t, 600, `<div style="display:flex;flex-direction:column;margin-left:100px">`+
		`<div>oooo <span id=s style="position:relative">xx`+
		`<span id=a style="position:absolute;left:0;top:0;width:5px;height:5px"></span></span></div></div>`,
		noDefaults)
	s := inlineFragsOf(root, "s")
	a := find(t, root, "a")
	if len(s) != 1 {
		t.Fatalf("the span made %d fragments; it is on one line", len(s))
	}
	if want := s[0].PaddingRect().X; a.BorderRect.X != want {
		t.Errorf("the box inside the span is at x=%v, and the span's padding box begins "+
			"at %v", a.BorderRect.X.Px(), want.Px())
	}
}

// TestARelayoutForgetsAnOffsetThatBecameNothing is the one piece of per-box
// side state that is written by every pass rather than taken back: an inline
// box's relative offset, which its background is drawn at. A box beside a
// float that meets a second float lower down is laid out again at a width
// that makes "left: calc(100% - 400px)" nothing, and the first pass's -100px
// was still there for the background of the second.
func TestARelayoutForgetsAnOffsetThatBecameNothing(t *testing.T) {
	root := layoutOf(t, 400, `<div style="width:400px">`+
		`<div style="float:left;width:100px;height:10px"></div>`+
		`<div style="float:right;clear:left;width:300px;height:50px"></div>`+
		`<div id=b style="overflow:hidden;min-width:250px">one two three four five six seven eight `+
		`<span id=s style="position:relative;left:calc(100% - 400px);background:green">x</span>`+
		` nine ten eleven twelve thirteen fourteen fifteen</div></div>`, noDefaults)
	b := find(t, root, "b")
	w, _ := style.FromPx(400)
	if b.BorderRect.W != w {
		t.Fatalf("the box is %vpx wide; the fixture needs it to have dropped below the "+
			"floats to the full 400", b.BorderRect.W.Px())
	}
	for _, f := range inlineFragsOf(root, "s") {
		if f.Offset.X != 0 {
			t.Errorf("the span's background is drawn %vpx along; at this width its "+
				"offset is nothing", f.Offset.X.Px())
		}
	}
}

// fragmentsOf collects the fragments the element with the given id produced,
// wherever they are among the children.
func fragmentsOf(root *Fragment, id string) []*Fragment {
	var out []*Fragment
	var walk func(f *Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		if f.Box != nil && f.Box.Element != nil {
			if got, _ := f.Box.Element.Attr("id"); got == id {
				out = append(out, f)
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestOutOfFlowBoxesInItemsArePlacedOnce holds every flex and grid pass that
// lays an item out more than once to placing what is inside it once, against
// the item it is inside. Each pass that is thrown away has to take back the
// out-of-flow boxes it queued and the positioned fragments it recorded, and
// each item that is laid out again from the cache has to queue and record
// them again — the one without the other is a box placed twice, or not at all,
// or against a fragment nobody drew.
func TestOutOfFlowBoxesInItemsArePlacedOnce(t *testing.T) {
	abs := `<div id=i style="position:relative;margin-left:30px">x` +
		`<div id=a style="position:absolute;left:0;top:0;width:5px;height:5px"></div></div>`
	for name, doc := range map[string]string{
		// A row whose second item is stretched: the whole first pass is taken
		// back and every item asked again, the first from the cache.
		"a stretched row": `<div style="display:flex"><div style="align-self:start">` + abs + `</div>` +
			`<div><div style="height:90px"></div></div><div>stretched</div></div>`,
		"an unstretched row": `<div style="display:flex;align-items:start">` + abs +
			`<div style="height:90px"></div></div>`,
		"a column": `<div style="display:flex;flex-direction:column">` + abs + `</div>`,
		"a grid":   `<div style="display:grid">` + abs + `</div>`,
		// Nested, so that the inner items are answered from the cache on the
		// outer containers' later passes. Three deep, because the two passes
		// of the middle level differ in whether the height a percentage
		// resolves against is definite, which makes its items' questions
		// different ones; the level below asks the same questions again.
		"nested columns": strings.Repeat(`<div style="display:flex;flex-direction:column">`, 3) +
			abs + strings.Repeat(`</div>`, 3),
		"nested grids": strings.Repeat(`<div style="display:grid">`, 3) + abs + strings.Repeat(`</div>`, 3),
		"a stretched row inside a column": `<div style="display:flex;flex-direction:column">` +
			`<div style="display:flex"><div>` + abs + `</div><div style="height:90px"></div></div></div>`,
	} {
		built := Build(Input{HTML: doc, CSS: []Stylesheet{{Source: noDefaults}}})
		w, _ := style.FromPx(600)
		l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, nil)
		root := l.layout()
		// The queue as well as the page. A box queued by a pass that was thrown
		// away is placed against a fragment nobody draws, so the page does not
		// show it — the queue does, and a queue that grows with every pass is
		// one that a document of nested items fills past maxAbsolutes.
		if len(l.deferred) != 1 {
			t.Errorf("%s: %d out-of-flow boxes were queued for one in the document",
				name, len(l.deferred))
		}
		as, items := fragmentsOf(root, "a"), fragmentsOf(root, "i")
		if len(as) != 1 || len(items) != 1 {
			t.Errorf("%s: the item has %d fragments and the box inside it %d; one each",
				name, len(items), len(as))
			continue
		}
		if as[0].BorderRect.X != items[0].PaddingRect().X || as[0].BorderRect.Y != items[0].PaddingRect().Y {
			t.Errorf("%s: the box is at (%v, %v) and the item it is positioned against at "+
				"(%v, %v)", name, as[0].BorderRect.X.Px(), as[0].BorderRect.Y.Px(),
				items[0].PaddingRect().X.Px(), items[0].PaddingRect().Y.Px())
		}
	}
}

// TestEveryDiscardedPassIsTakenBack is the two passes no test watched: a box
// beside floats laid out again where it had to drop, and a paragraph set again
// at the size text-fit settled on. Each is taken back whole, and what shows it
// is a float or an out-of-flow box the first pass left behind.
func TestEveryDiscardedPassIsTakenBack(t *testing.T) {
	w, _ := style.FromPx(400)
	for name, tc := range map[string]struct {
		doc    string
		id     string
		queued int
	}{
		"a box that dropped below the floats": {
			doc: `<div style="width:400px"><div style="float:left;width:100px;height:10px"></div>` +
				`<div style="float:right;clear:left;width:300px;height:50px"></div>` +
				`<div style="overflow:hidden;min-width:250px;position:relative">one two three four five ` +
				`six seven eight nine ten<div id=a style="position:absolute;left:0;top:0"></div></div></div>`,
			id: "a", queued: 1,
		},
		"a paragraph set again at a fitted size": {
			doc: `<p style="text-fit:grow;width:300px"><span id=a style="float:left;width:50px;height:20px"></span>short</p>`,
			id:  "a",
		},
	} {
		built := Build(Input{HTML: tc.doc, CSS: []Stylesheet{{Source: noDefaults}}})
		l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, nil)
		root := l.layout()
		if n := len(fragmentsOf(root, tc.id)); n != 1 {
			t.Errorf("%s: #%s has %d fragments", name, tc.id, n)
		}
		if len(l.deferred) != tc.queued {
			t.Errorf("%s: %d out-of-flow boxes were queued and the document has %d",
				name, len(l.deferred), tc.queued)
		}
	}
}
