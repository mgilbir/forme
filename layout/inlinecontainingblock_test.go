package layout

import "testing"

// CSS 2.1 §10.1's second case, for an absolutely positioned box whose nearest
// positioned ancestor is an *inline* one: "the containing block is the bounding
// box around the padding boxes of the first and last inline boxes generated for
// that element".
//
// This engine used to skip such an ancestor and resolve against the next
// candidate up — usually the page — and report having done so. The report was
// honest but the page was wrong, and it was wrong in the way that is hardest to
// see: "position: relative" on a span with an absolutely positioned child inside
// it is the commonest positioning idiom there is, and against the page the child
// still lands somewhere sensible-looking, just not beside the words it was
// written next to.
//
// The reason the case was skipped is that an inline box has no fragment of its
// own here in the way a block does — its geometry lives in the line boxes it was
// broken across. It does have fragments, though: inlinepaint.go makes one per
// line for any inline that draws a background or a border. What this needed was
// for a *positioned* inline to get them too, whether or not it draws anything.
//
// Every fixture below puts the inline somewhere the page is not, so that a box
// resolved against the wrong rectangle lands at a different number and not
// merely at the same one by luck. Courier at 20px advances 12px a character,
// which is where each expectation's arithmetic starts.

const inlineCB = `div, span { font-family: Courier; font-size: 20px; line-height: 30px }`

// inlineFragsOf returns, in line order, the fragments one inline box produced.
//
// They hang off the line boxes rather than off the fragment tree, so find() does
// not reach them. Reading them here gives the tests below an independent measure
// of where the inline actually is, to say that the absolutely positioned box
// landed on it rather than merely at a number someone wrote down.
func inlineFragsOf(root *Fragment, id string) []*Fragment {
	var out []*Fragment
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		for i := range f.Lines {
			for _, ib := range f.Lines[i].Boxes {
				if ib.Box != nil && ib.Box.Element != nil {
					if got, _ := ib.Box.Element.Attr("id"); got == id {
						out = append(out, ib)
					}
				}
			}
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(root)
	return out
}

// TestAnAbsoluteBoxResolvesAgainstItsInlineAncestor is the bug, at its smallest.
//
// Four Courier characters precede the span, so it begins 48px in. "left: 0"
// against the span puts the box at 48; against the page — which is what happened
// before — it puts it at 0. The two answers are as far apart as the fixture can
// make them, and the second is a box that looks placed and is not.
func TestAnAbsoluteBoxResolvesAgainstItsInlineAncestor(t *testing.T) {
	root := layoutOf(t, 600, `<div style="width:300px">oooo`+
		`<span id="s" style="position:relative">xx`+
		`<span id="a" style="position:absolute; left:0; top:0; width:10px; height:10px">`+
		`</span></span></div>`, noDefaults+inlineCB)

	a := find(t, root, "a")
	if want := bgpx(48); a.BorderRect.X != want {
		t.Errorf("the box is at x=%v, want %v: four Courier characters at 20px are "+
			"48px wide, so the span's padding box begins there. x=0 is the page, "+
			"which is what a skipped inline ancestor resolves against", a.BorderRect.X, want)
	}

	// And it is on the span, not merely at a number: the span's own fragment says
	// where its padding box is, and "left: 0; top: 0" is that box's origin.
	frags := inlineFragsOf(root, "s")
	if len(frags) != 1 {
		t.Fatalf("the span produced %d fragments on one line, want 1", len(frags))
	}
	want := frags[0].PaddingRect()
	if a.BorderRect.X != want.X || a.BorderRect.Y != want.Y {
		t.Errorf("the box is at %v,%v and the span's padding box begins at %v,%v",
			a.BorderRect.X, a.BorderRect.Y, want.X, want.Y)
	}
}

// TestTheInlineContainingBlockIsThePaddingBox, and not the content box.
//
// This is the same distinction §10.1 draws for a positioned block, and it is the
// one every "position: relative" wrapper depends on: a child at "left: 0; top: 0"
// sits inside the padding rather than inset by it. 5px of padding must move the
// child's *top* up by exactly 5 and must leave its left alone — the padding box's
// left edge is where the span begins either way, while its top edge rises above
// the text by the padding.
func TestTheInlineContainingBlockIsThePaddingBox(t *testing.T) {
	at := func(css string) Rect {
		t.Helper()
		root := layoutOf(t, 600, `<div style="width:300px">oooo`+
			`<span id="s" style="position:relative; `+css+`">xx`+
			`<span id="a" style="position:absolute; left:0; top:0; width:10px; height:10px">`+
			`</span></span></div>`, noDefaults+inlineCB)
		return find(t, root, "a").BorderRect
	}
	bare, padded := at(""), at("padding: 5px")

	if padded.X != bare.X {
		t.Errorf("padding moved the box's left from %v to %v; the padding box's left "+
			"edge is where the inline begins, with or without padding",
			bare.X, padded.X)
	}
	if got, want := bare.Y.Sub(padded.Y), bgpx(5); got != want {
		t.Errorf("5px of padding raised the box by %v, want %v: a box resolved "+
			"against the *content* box would not move at all", got, want)
	}
}

// TestTheContainingBlockSpansTheFirstAndLastFragments is the sentence's real
// content, and the half a one-line fixture cannot see.
//
// The div holds 10 characters a line. "aaaa bb cccc" breaks after "bb", so the
// span runs from x=60 to x=84 on the first line and from x=0 to x=48 on the
// second. Its containing block is the two together: x from 0 to 84.
//
// Each offset below names a different wrong answer. "left: 0" is 60 if only the
// first fragment is read and 0 if both are; "right: 0" is 38 if only the last is
// read and 74 if both are. An implementation that takes one fragment and calls
// it the box passes exactly one of these two.
func TestTheContainingBlockSpansTheFirstAndLastFragments(t *testing.T) {
	at := func(offsets string) *Fragment {
		t.Helper()
		root := layoutOf(t, 600, `<div style="width:120px">aaaa `+
			`<span id="s" style="position:relative">bb cccc`+
			`<span id="a" style="position:absolute; `+offsets+`; width:10px; height:10px">`+
			`</span></span></div>`, noDefaults+inlineCB)
		if got := len(inlineFragsOf(root, "s")); got != 2 {
			t.Fatalf("the span produced %d fragments, want 2 — the fixture is meant "+
				"to break it across two lines", got)
		}
		return find(t, root, "a")
	}

	// The left edge is the second line's, which begins at the div's own.
	if got, want := at("left: 0").BorderRect.X, bgpx(0); got != want {
		t.Errorf("left: 0 put the box at %v, want %v: the second fragment starts at "+
			"the line's beginning, and %v is the first fragment's left alone",
			got, want, bgpx(60))
	}
	// The right edge is the first line's: "aaaa bb" is seven characters, 84px.
	if got, want := at("right: 0").BorderRect.X, bgpx(74); got != want {
		t.Errorf("right: 0 put the box at %v, want %v: the containing block's right "+
			"edge is 84, and %v is the last fragment's right alone",
			got, want, bgpx(38))
	}
}

// TestTheVerticalEdgesSpanTheFragmentsToo. The same rule down the page, which is
// the axis a broken inline stretches along and the one where reading a single
// fragment is most obviously wrong: the two lines are 30px apart.
func TestTheVerticalEdgesSpanTheFragmentsToo(t *testing.T) {
	root := layoutOf(t, 600, `<div style="width:120px">aaaa `+
		`<span id="s" style="position:relative">bb cccc`+
		`<span id="a" style="position:absolute; bottom:0; width:10px; height:10px">`+
		`</span></span></div>`, noDefaults+inlineCB)

	frags := inlineFragsOf(root, "s")
	if len(frags) != 2 {
		t.Fatalf("the span produced %d fragments, want 2", len(frags))
	}
	first, last := frags[0].PaddingRect(), frags[1].PaddingRect()
	if last.Y.Sub(first.Y) != bgpx(30) {
		t.Fatalf("the two fragments are %v apart and the line height is 30px; the "+
			"fixture is not testing what it claims to", last.Y.Sub(first.Y))
	}

	// "bottom: 0" sits the 10px box on the containing block's bottom edge, which
	// is the *second* fragment's. Against the first it would be a line higher.
	want := last.Y.Add(last.H).Sub(bgpx(10))
	if got := find(t, root, "a").BorderRect.Y; got != want {
		t.Errorf("bottom: 0 put the box at y=%v, want %v: the containing block's "+
			"bottom edge is the last fragment's", got, want)
	}
}

// TestAnInlineThatGeneratedNoFragmentsIsStillReported keeps the finding honest
// where it is still true.
//
// An inline box with no content on any line generates no fragments, so §10.1 has
// no padding boxes to form a rectangle from and there is nothing this can do but
// go on up — which is the old behaviour, and must go on being reported, because
// the page is still not the one the author asked for.
func TestAnInlineThatGeneratedNoFragmentsIsStillReported(t *testing.T) {
	empty := findingsOf(t, `<div style="width:300px">oooo`+
		`<span style="position:relative">`+
		`<span style="position:absolute; left:0; width:10px; height:10px"></span>`+
		`</span></div>`, inlineCB)
	if !hasRule(empty, RulePositionApproximated) {
		t.Errorf("an inline ancestor with no fragments was not reported; there is no " +
			"rectangle to resolve against and the box was placed against the page")
	}

	// And the case that *can* now be answered is not reported, or every page
	// using the idiom carries a finding about something this gets right.
	solved := findingsOf(t, `<div style="width:300px">oooo`+
		`<span style="position:relative">xx`+
		`<span style="position:absolute; left:0; width:10px; height:10px"></span>`+
		`</span></div>`, inlineCB)
	if hasRule(solved, RulePositionApproximated) {
		t.Errorf("an inline ancestor that has fragments was reported as approximated, " +
			"and its containing block is now formed exactly as §10.1 says")
	}
}

// TestAStaticPositionIsUnaffected is a containment case, and the one most likely
// to break quietly.
//
// A box with no offsets is placed at its static position — where it would have
// been in the flow — and §10.3.7's static position has nothing to do with the
// containing block. It is 72px in: 48 for "oooo" and 24 for the "xx" before it.
// If forming the inline's rectangle had leaked into that path the box would jump
// to the span's left edge, which is a box in a plausible wrong place.
func TestAStaticPositionIsUnaffected(t *testing.T) {
	root := layoutOf(t, 600, `<div style="width:300px">oooo`+
		`<span style="position:relative">xx`+
		`<span id="a" style="position:absolute; width:10px; height:10px">`+
		`</span></span></div>`, noDefaults+inlineCB)
	if got, want := find(t, root, "a").BorderRect.X, bgpx(72); got != want {
		t.Errorf("the box is at x=%v, want %v: six Courier characters precede it and "+
			"a box with no offsets stays where it was written", got, want)
	}
}

// TestAPositionedInlineWithNothingToDrawStillDrawsNothing is the other
// containment case, and it is about the change made to reach the geometry rather
// than about the geometry.
//
// Fragments for an inline box used to be made only when the box painted a
// background or a border, because that was all they were for. Making them for a
// positioned inline as well puts fragments into the line boxes of a great many
// documents that never had any, and a fragment that draws nothing must go on
// drawing nothing — otherwise every relatively positioned span in the corpus
// grows an invisible box that is not quite invisible.
func TestAPositionedInlineWithNothingToDrawStillDrawsNothing(t *testing.T) {
	ops := func(pos string) int {
		return len(paintOf(t, `<div>oooo<span style="position:`+pos+`">xx</span>yy</div>`,
			noDefaults+inlineCB))
	}
	if got, want := ops("relative"), ops("static"); got != want {
		t.Errorf("a relatively positioned span emitted %d operations and a static one "+
			"%d; neither has a background or a border, so neither draws anything",
			got, want)
	}
}
