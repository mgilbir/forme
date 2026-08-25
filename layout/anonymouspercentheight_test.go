package layout

import "testing"

// A percentage height inside an anonymous block box.
//
// CSS 2.1 §9.2.1.1 puts the rule in a note, and the suite's
// CSS2/visuren/anonymous-boxes-001a quotes that note in its own source: "if the
// child of the anonymous block box inside the DIV above needs to know the height
// of its containing block to resolve a percentage height, then it will use the
// height of the containing block formed by the DIV, not of the anonymous block
// box".
//
// An anonymous box has no declarations of its own and height is not inherited,
// so its height is auto — and a percentage against an auto height computes to
// auto. Every percentage height inside one therefore vanished. The test puts
// "height: 50%" on an image beside some text, which is exactly what puts the
// image in an anonymous block, inside a div two hundred pixels tall, and asks
// for a hundred-pixel square; it drew at the image's intrinsic one pixel.

// atomicHeight is the height of the atomic inline #a, which is a child of the
// block rather than a run on its line.
func atomicHeight(t *testing.T, htmlSrc, cssSrc string) float64 {
	t.Helper()
	return find(t, layoutOf(t, 600, htmlSrc, cssSrc), "a").BorderRect.H.Px()
}

// TestAPercentageHeightInAnAnonymousBlockResolves is the bug.
//
// An inline-block stands in for the test's image: it is an atomic inline, so
// putting one beside text makes the same anonymous block, and its height does
// not need a picture to be loaded.
func TestAPercentageHeightInAnAnonymousBlockResolves(t *testing.T) {
	got := atomicHeight(t,
		`<div id="d">Some <span id="a"></span> text<p>More</p></div>`,
		`#d { height: 200px } #a { display: inline-block; width: 10px; height: 50% }`)
	if got != 100 {
		t.Errorf("the inline-block is %gpx tall, want 100 — half of the div's 200, "+
			"not half of the anonymous block's auto height", got)
	}
}

// TestItIsTheNonAnonymousAncestorsHeight, and not simply "whatever height is
// available": a div with no height of its own gives a percentage nothing to
// resolve against, and the answer is auto exactly as it was.
func TestItIsTheNonAnonymousAncestorsHeight(t *testing.T) {
	got := atomicHeight(t,
		`<div id="d">Some <span id="a"></span> text<p>More</p></div>`,
		`#d { } #a { display: inline-block; width: 10px; height: 50% }`)
	if got != 0 {
		t.Errorf("with no height on the div the inline-block is %gpx tall, want 0 — "+
			"a percentage of an auto height is auto", got)
	}
}

// TestTheAnonymousBoxsOwnEdgesAreUnchanged is the containment argument, and it
// is what the first attempt got wrong: the height handed *down* is the parent's,
// and nothing about the anonymous box's own height, own bottom edge or own
// margin collapsing changes because its parent declared one.
func TestTheAnonymousBoxsOwnEdgesAreUnchanged(t *testing.T) {
	// The margin of the paragraph after the anonymous block escapes through the
	// anonymous block's open bottom edge and collapses with the div's content,
	// which it can only do while that edge is open — that is, while the
	// anonymous box is still a box with no height of its own.
	root := layoutOf(t, 600, `<div id="d">text<div id="k"></div></div>`,
		`#d { height: 200px } #k { margin-top: 40px; height: 10px }`)
	d := find(t, root, "d")
	if len(d.Children) == 0 || len(d.Children[0].Lines) != 1 {
		t.Fatalf("the div's first child is not the anonymous block holding one line")
	}
	anon := d.Children[0]
	// One line of text, then the forty pixels of margin, and no more: the margin
	// left the anonymous block through an edge that is still open.
	want := anon.BorderRect.Y.Add(anon.BorderRect.H).Px() + 40
	if got := find(t, root, "k").BorderRect.Y.Px(); got != want {
		t.Errorf("the block after the anonymous one is at %gpx, want %g; its margin "+
			"did not pass through the edges it used to", got, want)
	}
}

// TestAnOrdinaryBlockIsUnaffected: the rule is about anonymous boxes, and a
// named block with no height still gives its children nothing to resolve
// against.
func TestAnOrdinaryBlockIsUnaffected(t *testing.T) {
	got := atomicHeight(t,
		`<div id="d"><div id="mid">Some <span id="a"></span></div></div>`,
		`#d { height: 200px } #a { display: inline-block; width: 10px; height: 50% }`)
	if got != 0 {
		t.Errorf("the inline-block inside a named block with no height is %gpx "+
			"tall, want 0", got)
	}
}
