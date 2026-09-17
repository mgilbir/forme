package style

// Properties this engine does not implement and that could not change a page if
// it did.
//
// # The claim this narrows
//
// Finding.Unsupported means the page differs from the one the stylesheet
// describes. For a property that was dropped that is usually exactly right: a
// "border-radius" nobody applied leaves square corners where the author asked
// for round ones, and an author has no other way to find out.
//
// It is not right for a property whose whole effect is on something a page laid
// out once does not have. Nobody puts a text cursor in a printed paragraph, so
// "caret-color: red" colours nothing there — and a browser printing the same
// document applies it exactly as little. The page *is* the one CSS describes.
// Saying otherwise makes an ordinary stylesheet look defective, and a caller
// with a strict policy would refuse a document over a declaration that changed
// nothing.
//
// This is the same distinction css/selector.go draws about a selector that
// matches nothing in this medium, and it is drawn the same way: by whether the
// *medium* answers the declaration, not by whether this engine implements it.
// The finding is still reported and its message is unchanged — an author who
// wrote only this has still had it dropped, and may want to know.
//
// # What is deliberately not here
//
// "animation" is the near miss and is the reason this is a list rather than a
// rule about interaction. An animation with a delay, a fill mode or an infinite
// duration puts the element in a state that is not the one the other
// declarations describe, and a page rendered once shows that state. So a dropped
// animation really can leave the page different, and it stays unsupported.
//
// "overflow" and "resize" are not here either, for the opposite reason: both
// change the *layout* of a page nobody scrolls or drags. Only the act is
// missing, not the effect.
//
// # How this differs from inert.go, which says several of the same things
//
// inert.go asks whether a *value* asks for the page that is already there, and
// where it does the finding is not raised at all — there is nothing to tell an
// author about "user-select: auto". Several entries there carry the very
// argument this file is about, in their reasons: "there is no pointer", "there
// is no selection", "there is nothing to scroll".
//
// This is the stronger claim about the same properties: for these, *no* value
// can change a page, so a declaration of one is still worth reporting and is
// still not a page this engine got wrong. "user-select: none" is dropped and
// says so; what it does not say is that the page differs from the one the
// stylesheet describes, because it does not.
var noEffectOnAPage = map[string]bool{
	// The insertion point. This engine paints no caret anywhere — a form
	// control is drawn as the box it occupies, see layout/control.go — so
	// nothing about one can be styled.
	"caret": true, "caret-color": true, "caret-shape": true,
	"caret-animation": true,

	// The pointer and the selection. A page has neither: there is nothing to
	// hover, nothing to hit-test and nothing selected.
	"cursor": true, "pointer-events": true, "user-select": true,
	"touch-action": true,

	// Scrolling. A page is not a scroll container; it is as tall as it is, and
	// what these describe is how it would behave while somebody moved it.
	"scroll-behavior":     true,
	"overscroll-behavior": true, "overscroll-behavior-x": true,
	"overscroll-behavior-y": true, "overscroll-behavior-inline": true,
	"overscroll-behavior-block": true,
	"scroll-snap-type":          true, "scroll-snap-align": true,
	"scroll-snap-stop": true,
	// And where a box would come to rest if anyone ever scrolled it there.
	// CSS Scroll Snap 1 §5 and §6 define both as geometry on the scrollport:
	// scroll-margin grows the box's snap area, scroll-padding shrinks the
	// region it snaps into, and neither is consulted by anything that lays a
	// box out or paints one. "scroll-margin: 20px" is not a margin.
	//
	// The logical forms are here with the physical ones because a stylesheet
	// that writes one writes the other, and because a table that named only
	// half of them would report the same declaration in two spellings
	// differently.
	"scroll-margin": true, "scroll-margin-top": true, "scroll-margin-right": true,
	"scroll-margin-bottom": true, "scroll-margin-left": true,
	"scroll-margin-block": true, "scroll-margin-block-start": true,
	"scroll-margin-block-end": true, "scroll-margin-inline": true,
	"scroll-margin-inline-start": true, "scroll-margin-inline-end": true,
	"scroll-padding": true, "scroll-padding-top": true, "scroll-padding-right": true,
	"scroll-padding-bottom": true, "scroll-padding-left": true,
	"scroll-padding-block": true, "scroll-padding-block-start": true,
	"scroll-padding-block-end": true, "scroll-padding-inline": true,
	"scroll-padding-inline-start": true, "scroll-padding-inline-end": true,
	// CSS Scroll Anchoring 1 §3. The property says whether a box may be the one
	// a scroll position is kept against while content above it changes. Nothing
	// here scrolls and nothing changes after it is laid out, so there is no
	// anchor to opt out of.
	"overflow-anchor": true,
	// CSS Scrollbars 1 §3. The colours of a scrollbar, on a page that has none.
	//
	// "scrollbar-width" and "scrollbar-gutter" are deliberately *not* here, and
	// the line between them is the same one this file draws everywhere: those
	// two change the room the content has — a browser reserves a gutter for
	// "stable" and takes the bar away for "none", and either moves text — while
	// a colour moves nothing. See caret-color above, which is the same case and
	// the one this file was written around.
	"scrollbar-color": true,

	// Change over time. A transition describes what happens *between* two
	// states; a document rendered once has one state, and every transition
	// property is a description of a journey nobody takes.
	"transition": true, "transition-property": true,
	"transition-duration": true, "transition-timing-function": true,
	"transition-delay": true, "transition-behavior": true,

	// And the hint that is defined as having no rendering effect at all.
	// css-will-change says so in as many words: "this property has no direct
	// effect on the element it is specified on beyond the browser's
	// optimizations". An engine that ignores it renders the page the stylesheet
	// describes, which is the whole of what this table is about.
	"will-change": true,
}

// hasNothingToApplyTo reports whether a property could not change this medium's
// rendering whatever its value.
func hasNothingToApplyTo(name string) bool { return noEffectOnAPage[name] }
