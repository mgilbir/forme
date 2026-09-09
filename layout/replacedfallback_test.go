package layout

import (
	"strings"
	"testing"
)

// The children of a replaced element are not laid out, and are not laid out
// early enough.
//
// A canvas is its bitmap and a video is its poster or its default box; what is
// written inside either is what a user agent that could not draw it would show
// instead, and this one draws it. That much was already true — both loaders
// threw the children away — but they threw them away once the box tree was
// built, which is one step too late.
//
// A *block* among the fallback splits the inline box around it before any of
// that runs, and what is left is the block and no replaced box at all.
// "<canvas><p>x</p></canvas>" drew the paragraph and lost the canvas: both
// halves of the rule the wrong way round, the element's own content gone and its
// fallback on the page.
func TestAReplacedElementLaysOutNoFallback(t *testing.T) {
	for _, tc := range []struct{ markup, name, what string }{
		{`<canvas id="v"><span>fallback</span></canvas>`, "canvas", "inline fallback in a canvas"},
		{`<canvas id="v"><p>fallback</p></canvas>`, "canvas", "block fallback in a canvas"},
		{`<video id="v"><span>fallback</span></video>`, "video", "inline fallback in a video"},
		{`<video id="v"><p>fallback</p></video>`, "video", "block fallback in a video"},
		{`<video id="v"><source src="f.mp4"/><p>fallback</p></video>`, "video",
			"a source and a block fallback"},
	} {
		built := Build(Input{HTML: `<div>` + tc.markup + `</div>`})
		box := boxFor(built.Root, tc.name)
		if box == nil {
			t.Errorf("%s: the element produced no box at all", tc.what)
			continue
		}
		if box.Replaced == nil {
			t.Errorf("%s: the box is not a replaced one", tc.what)
		}
		if len(box.Children) != 0 {
			t.Errorf("%s: %d boxes of fallback survived inside it",
				tc.what, len(box.Children))
		}
		if got := textOfTree(built.Root); strings.Contains(got, "fallback") {
			t.Errorf("%s: the fallback reached the page: %q", tc.what, got)
		}
	}
}

// TestAnObjectStillShowsItsFallback is the distinction rather than an omission.
//
// An <object>'s fallback is what a reader sees when the data cannot be read,
// which is a thing that happens — so its children are content and are laid out.
// The rule above is for the two elements whose replacement is settled by the
// element itself and can never fail.
func TestAnObjectStillShowsItsFallback(t *testing.T) {
	built := Build(Input{HTML: `<div><object data="x.swf"><span>fallback</span></object></div>`})
	if got := textOfTree(built.Root); !strings.Contains(got, "fallback") {
		t.Errorf("an object whose data could not be read showed no fallback: %q", got)
	}
}
