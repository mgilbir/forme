package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// What a replaced element contributes to the width of the box around it, when
// its own width comes from a percentage height and a ratio.
//
// The shape of the case: a shrink-to-fit box — a float, an inline-block, a
// cell — is measured before it is laid out, and a replaced child inside it has
// an auto width. §10.3.2 says that width is the used height times the intrinsic
// ratio, and the height here is a *percentage*. A percentage width would have
// to be given up at this point, because the width being measured is the one it
// would be a percentage of; a percentage height is a percentage of something
// else entirely, and whenever a stylesheet wrote that something down there is
// a real answer.
//
// Getting it wrong is not a missing number but a different one: the element
// falls back to its intrinsic width and the box shrink-wraps to that, which is
// a box of the wrong size holding a picture of the right one.

// TestAPercentageHeightDecidesAnIntrinsicWidth is the plain case: an
// inline-block a hundred tall around a two-to-one picture asked to fill it.
func TestAPercentageHeightDecidesAnIntrinsicWidth(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
		`#s { display: inline-block; height: 100px } #i { height: 100% }`)
	w, h := contentSize(find(t, root, "i"))
	px(t, "the picture's width, from its height and its two-to-one ratio", w, 200)
	px(t, "the picture's height", h, 100)
	sw, _ := contentSize(find(t, root, "s"))
	px(t, "the shrink-to-fit width around it", sw, 200)
}

// TestAPercentageHeightIsResolvedThroughAnAnonymousBlock is §9.2.1.1's note,
// which the layout pass already honours and which this walk has to honour the
// same way: an anonymous box has no declarations and so no height, and a
// percentage inside one resolves against the block the anonymous one is inside.
//
// The mixture is what makes the anonymous block: an inline child and a block
// child together, which is exactly what
// CSS2/normal-flow/intrinsic-size-with-anonymous-block writes.
func TestAPercentageHeightIsResolvedThroughAnAnonymousBlock(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div><span id="s"><img id="i" src="wide.png"><p></p></span></div>`, noDefaults,
		`#s { display: inline-block; height: 100px } #i { height: 100% } p { margin: 0 }`)
	px(t, "the picture's width, resolved past the anonymous block",
		widthOf(t, root, "i"), 200)
	px(t, "the shrink-to-fit width around it", widthOf(t, root, "s"), 200)
}

// TestAPercentageHeightChainIsFollowed: the ancestor's own height may be a
// percentage, and §10.5 resolves that one the same way. Half of two hundred is
// a hundred, half of that is fifty, and fifty of a two-to-one picture is a
// hundred wide.
func TestAPercentageHeightChainIsFollowed(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div id="outer"><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
		`#outer { height: 200px } #s { display: inline-block; height: 50% } #i { height: 50% }`)
	px(t, "the picture's width, two percentages down", widthOf(t, root, "i"), 100)
	// And the box around it, which is what the walk decides: the used width is
	// layout's answer and would be right whether or not the walk existed.
	px(t, "the shrink-to-fit width around it", widthOf(t, root, "s"), 100)
}

// TestAnIndefiniteAncestorHeightLeavesTheIntrinsicWidth is the other half of
// §10.5, and the reason this is a walk that can decline rather than a lookup.
// A percentage of a height nothing has decided is indefinite, so there is no
// width to compute from it and the picture is its own size.
func TestAnIndefiniteAncestorHeightLeavesTheIntrinsicWidth(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
		`#s { display: inline-block } #i { height: 100% }`)
	px(t, "the picture's own width, the percentage having nothing to be of",
		widthOf(t, root, "i"), 40)
	px(t, "and the box around it", widthOf(t, root, "s"), 40)
}

// TestAnAncestorsOwnLimitsAreApplied.
//
// Layout puts a box's declared height through §10.7's maximum and then its
// minimum before handing it down, so a walk that read the height and ignored
// the limits would answer a number the box is not. Both are applied here, and
// the observable is the shrink-to-fit box around the picture: a hundred-tall
// ancestor capped at fifty makes a two-to-one picture a hundred wide, and one
// floored at three hundred makes it six hundred.
func TestAnAncestorsOwnLimitsAreApplied(t *testing.T) {
	for _, tc := range []struct {
		decl string
		want float64
	}{
		{"max-height: 50px", 100},
		{"min-height: 300px", 600},
		// A limit written as a percentage is settled by the same walk, one
		// level further up: half of the four-hundred-tall block outside is two
		// hundred, which does not cap a hundred, so the height stands.
		{"max-height: 50%", 200},
		// And one that does cap it: a quarter of four hundred is a hundred, and
		// half of *that* is the fifty this leaves.
		{"max-height: 12.5%", 100},
	} {
		root := replacedLayout(t, 500,
			`<div id="outer"><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
			`#outer { height: 400px } #s { display: inline-block; height: 100px; `+tc.decl+` } #i { height: 100% }`)
		px(t, "with \""+tc.decl+"\" on the ancestor, the shrink-to-fit width",
			widthOf(t, root, "s"), tc.want)
	}
}

// TestALimitThatCannotBeSettledIsDeclined is the other side of the same rule.
// A percentage maximum against an ancestor whose own height is auto resolves to
// nothing, and a height that ignored it would be a height the box is not — so
// the walk stops and the picture keeps its own width.
func TestALimitThatCannotBeSettledIsDeclined(t *testing.T) {
	root := replacedLayout(t, 500,
		`<div id="outer"><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
		`#s { display: inline-block; height: 100px; max-height: 50% } #i { height: 100% }`)
	px(t, "the picture's own width, the maximum having nothing to be of",
		widthOf(t, root, "s"), 40)
}

// TestTheInitialMinimumAndMaximumAreNotConstraints is the trap, and it is a
// test rather than a comment because the first version of the walk fell into it
// and answered *nothing at all*.
//
// Every computed style in every document carries "min-height: auto" and
// "max-height: none". A walk that declines at the sight of either declines
// always, and the failure is silent: every box keeps its intrinsic width, which
// is what it had before the walk was written.
func TestTheInitialMinimumAndMaximumAreNotConstraints(t *testing.T) {
	for _, decl := range []string{
		"",
		"min-height: auto",
		"min-height: 0",
		"max-height: none",
		"min-height: 0; max-height: none",
	} {
		root := replacedLayout(t, 500,
			`<div><span id="s"><img id="i" src="wide.png"></span></div>`, noDefaults,
			`#s { display: inline-block; height: 100px; `+decl+` } #i { height: 100% }`)
		px(t, "with \""+decl+"\" on the ancestor, the width its height gives it",
			widthOf(t, root, "i"), 200)
	}
}

// widthOf is the content width of the fragment for an id.
func widthOf(t *testing.T, root *Fragment, id string) style.Unit {
	t.Helper()
	w, _ := contentSize(find(t, root, id))
	return w
}
