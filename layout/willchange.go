package layout

import (
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
)

// css-will-change 1: will-change.
//
// # What a hint does to a page
//
// will-change tells a browser what is about to change, and almost all of what
// it asks for is preparation: nothing on a page rendered once changes. But §3
// gives it effects on the rendering itself, which hold whether or not anything
// ever changes:
//
//	If any non-initial value of a property would create a stacking context
//	on the element, specifying that property in will-change must create a
//	stacking context on the element.
//
//	If any non-initial value of a property would cause the element to
//	generate a containing block for absolutely positioned elements,
//	specifying that property in will-change must cause the element to
//	generate a containing block for absolutely positioned elements.
//
// and the same again for fixed positioned elements. "The will-change property
// has no direct effect on the element it is specified on, beyond the creation
// of stacking contexts and containing blocks as specified above." So
// "will-change: transform" makes a box the stacking context and the containing
// block a transform makes it, with nothing transformed; and a name "is
// identical to" naming every longhand of it when it is a shorthand, which is
// how "mask" and "offset" are here.
//
// # What is done
//
// All of it. Each name a property's specification gives either effect to is in
// willChangeRules below, with the boxes it would have it on, and the three
// questions the rest of the engine asks — is this box a stacking context
// (formsAStackingContext, in paint.go), and is it the containing block of the
// absolutely or of the fixed positioned boxes inside it (containsAbsolutes and
// containsFixed, in position.go) — each ask willChangeAsksOf. There is nothing
// to report about the hint itself: what it asks for is made. What the property
// it names would do beyond that, a transform's transforming, is the property's
// and is reported where the property is declared, and nowhere when it is only
// named here, since naming it asks for none of it.
//
// Nothing else is made, and nothing else is reported: a name whose property
// makes neither — a colour, a margin — asks for nothing on a page, nor do
// scroll-position and contents, which are about scrolling and content that
// changes. Nor does a custom property ("Specifying a custom property must have
// no effect"), a name no property has ("Specifying a value that's not
// recognized as a property is fine; it simply has no effect"), or a vendor's
// prefixed name, which this engine treats as that vendor's wherever it meets
// one (see style's vendorPrefixed) and does not read as the property it
// shadows.

// willChangeAsks is what naming a property in will-change asks of a box: a
// set of the three effects §3 gives the hint.
type willChangeAsks uint8

const (
	// asksStackingContext: the box is a stacking context.
	asksStackingContext willChangeAsks = 1 << iota
	// asksAbsoluteContainer: the box is the containing block of the absolutely
	// positioned boxes inside it.
	asksAbsoluteContainer
	// asksFixedContainer: the box is the containing block of the fixed
	// positioned boxes inside it. Every property that makes a box this makes
	// it asksAbsoluteContainer as well — position makes the second and not
	// the first, but nothing makes the first and not the second — and
	// containsAbsolutes counts it as both.
	asksFixedContainer
)

// asksEverything is a stacking context and a containing block for every
// positioned box inside, which is what a transform makes a box.
const asksEverything = asksStackingContext | asksAbsoluteContainer | asksFixedContainer

// willChangeRules says, for each property some non-initial value of which
// makes a box a stacking context or a containing block, what naming it asks of
// a given box: nothing where the property does not apply to the box, or where
// no value of it would have the effect there. The names are in lower case, as
// willChangeAsksOf compares them.
//
// The list is the specifications', property by property:
//
//   - transform: CSS Transforms 1 §2, "any value other than none for the
//     transform property results in the creation of a stacking context", and
//     "also causes the element to establish a containing block for all
//     descendants. Its padding box will be used to layout for all of its
//     absolute-position descendants, fixed-position descendants". translate,
//     rotate and scale are CSS Transforms 2 §5's "all other values ... create
//     a stacking context and containing block for all descendants, per usual
//     for transforms". perspective, §8: "any value other than none establishes
//     a stacking context. It also establishes a containing block for all
//     descendants, just like the transform property does". transform-style,
//     §7: "A computed value of preserve-3d ... on a transformable element
//     establishes both a stacking context and a containing block for all
//     descendants". offset-path, CSS Motion Path 1 §2.1: "All the usual
//     effects of having a transform apply (such as creating a stacking
//     context, etc.)", and the offset shorthand holds it. All of them apply to
//     transformable elements only.
//   - filter: Filter Effects 1 §5, "a containing block for absolute and fixed
//     positioned descendants unless the element it applies to is a document
//     root element", and "A computed value of other than none results in the
//     creation of a stacking context". backdrop-filter, Filter Effects 2 §2,
//     the same both, "unless the element it applies to is a document root
//     element". Both apply to all elements, an inline box included.
//   - opacity: CSS Color 4 §3.3, "If a box has opacity less than 1, it forms a
//     stacking context for its children". isolation and mix-blend-mode:
//     Compositing and Blending 1 §3.4.2 and §3.4.1, "setting isolation to
//     isolate will turn the element into a stacking context" and "Applying a
//     blendmode other than normal to the element must establish a new
//     stacking context". clip-path, mask-image and mask-border-source: CSS
//     Masking 1 §5.1, §7.1 and §8.1, "A computed value of other than none
//     results in the creation of a stacking context"; and the shorthands mask
//     (§7.9), which sets mask-image and resets mask-border, and mask-border
//     (§8.7). view-transition-name: CSS View Transitions 1 §2.1.1, an element
//     "whose view-transition-name computed value is not none (at any time)"
//     forms a stacking context. (The property's own sentence that it "has no
//     effect" on a fragmented box is about taking part in a transition —
//     "Fragmented elements don't participate in view transitions", issue 8339
//     — and not about the stacking context.) A stacking context each, and no
//     containing block, on all elements.
//   - contain: CSS Containment 2 §3.3 and §3.5, a layout and a paint
//     containment box each "establishes an absolute positioning containing
//     block and a fixed positioning containing block" and "creates a stacking
//     context" — "layout", "paint", "content" and "strict" — except where the
//     containment "has no effect": an internal table box other than a cell, an
//     internal ruby box, a non-atomic inline box. content-visibility, §4:
//     "auto" turns on layout and paint containment, and it applies to
//     "elements for which size containment can apply", which excludes a table
//     and every internal table box as well (§3.1).
//   - position: CSS Positioned Layout 3 §2, "Values other than static make the
//     box a positioned box, and cause it to establish an absolute positioning
//     containing block for its descendants", and §2.2, "Fixed and sticky
//     positioned boxes nonetheless form a stacking context". No value of it
//     makes a containing block for a fixed box: §2.1 lists "transform,
//     will-change, contain" as what does. It applies to every box but a table
//     column or column group.
//   - z-index: CSS 2 §9.9.1, a value other than auto makes a stacking context
//     where z-index applies, which is a positioned box or a flex or grid item
//     (zIndexApplies); elsewhere it is ignored, and no value of it makes one.
//
// Three that might be expected are not here. backface-visibility, CSS
// Transforms 2 §10: "hidden ... on a transformable element that participates
// in a 3D rendering context establishes both a stacking context and a
// containing block for all descendants"; but a box participates only where it
// or its parent has a used transform-style of preserve-3d (§4.1.2), and this
// engine gives no box that — transform-style is a property it does not
// implement, reported where "preserve-3d" is declared, and every box is flat.
// container-type once applied layout containment; CSS Conditional 5 §5.1 now
// has "size" and "inline-size" apply style and size containment, neither of
// which makes a stacking context or a containing block, and "scroll-state" no
// containment at all. And "clip", whose value forces a flat transform-style
// (CSS Transforms 2 §7.1), makes neither.
var willChangeRules = map[string]func(b *Box) willChangeAsks{
	"transform": asATransform, "translate": asATransform, "rotate": asATransform,
	"scale": asATransform, "perspective": asATransform, "transform-style": asATransform,
	"offset-path": asATransform, "offset": asATransform,

	"filter": asAFilter, "backdrop-filter": asAFilter,

	"opacity": asAGroup, "isolation": asAGroup, "mix-blend-mode": asAGroup,
	"clip-path": asAGroup, "mask": asAGroup, "mask-image": asAGroup,
	"mask-border": asAGroup, "mask-border-source": asAGroup,
	"view-transition-name": asAGroup,

	"contain":            asContainment,
	"content-visibility": asContentVisibility,

	"position": asPosition,
	"z-index":  asZIndex,
}

// asATransform is what a transform, or a property that has a transform's
// effects, makes a box: a stacking context and the containing block of
// everything positioned inside it, on a transformable box.
func asATransform(b *Box) willChangeAsks {
	if !transformable(b) {
		return 0
	}
	return asksEverything
}

// asAFilter is what a filter or a backdrop filter makes a box: a stacking
// context, and the containing block of everything positioned inside it
// unless it is the root.
func asAFilter(b *Box) willChangeAsks {
	if b.Parent == nil {
		return asksStackingContext
	}
	return asksEverything
}

// asAGroup is a stacking context and nothing else, on any box.
func asAGroup(*Box) willChangeAsks { return asksStackingContext }

// asContainment is what layout or paint containment makes a box, where it has
// any effect.
func asContainment(b *Box) willChangeAsks {
	if isInlineBox(b) || internalTableBox(b) && b.Inner != InnerTableCell {
		return 0
	}
	return asksEverything
}

// asContentVisibility is asContainment on the boxes size containment can
// apply to, which are fewer: not a table, and no internal table box.
func asContentVisibility(b *Box) willChangeAsks {
	if isInlineBox(b) || internalTableBox(b) || b.Inner == InnerTable {
		return 0
	}
	return asksEverything
}

// asPosition is what a position other than static makes a box: a stacking
// context (fixed does) and the containing block of the absolutely positioned
// boxes inside it (every one of them does), but not of the fixed ones.
func asPosition(b *Box) willChangeAsks {
	if b.Inner == InnerTableColumn || b.Inner == InnerTableColumnGroup {
		return 0
	}
	return asksStackingContext | asksAbsoluteContainer
}

// asZIndex is a stacking context where z-index applies.
func asZIndex(b *Box) willChangeAsks {
	if !zIndexApplies(b) {
		return 0
	}
	return asksStackingContext
}

// transformable reports whether a box is CSS Transforms 1 §1.2's transformable
// element: every box "whose layout is governed by the CSS box model except for
// non-replaced inline boxes, table-column boxes, and table-column-group boxes".
func transformable(b *Box) bool {
	return !isInlineBox(b) && b.Inner != InnerTableColumn && b.Inner != InnerTableColumnGroup
}

// isInlineBox reports whether a box is a non-atomic, non-replaced inline box:
// one a line breaks, which has no box of its own to transform or contain.
func isInlineBox(b *Box) bool { return !b.IsText() && onALine(b) }

// internalTableBox reports whether a box is one of CSS Tables 3's internal
// table boxes: a row group, a row, a cell, a column or a column group. A
// caption is not one, and nor is the table.
func internalTableBox(b *Box) bool {
	switch b.Inner {
	case InnerTableRowGroup, InnerTableRow, InnerTableCell, InnerTableColumn, InnerTableColumnGroup:
		return true
	}
	return false
}

// willChangeAsksOf is what a box's will-change asks of it: everything any
// name in it asks.
//
// It is asked by every step of the paint and of the containing-block search,
// of every box, so it allocates nothing: the names are walked in place and a
// name in capitals is lowered on the stack. A box with no will-change, which
// is almost every box, costs one style lookup.
func willChangeAsksOf(b *Box) willChangeAsks {
	if b == nil || b.IsText() {
		return 0
	}
	raw := b.Style.Get("will-change")
	if raw == "" {
		return 0
	}
	var asks willChangeAsks
	var buf [32]byte
	for rest := raw; rest != ""; {
		var name string
		name, rest, _ = strings.Cut(rest, ",")
		name = ascii.TrimCSSSpace(name)
		// No property's name is longer than the buffer, so a longer name is
		// none of them and is not looked up.
		if len(name) > len(buf) {
			continue
		}
		for i := 0; i < len(name); i++ {
			buf[i] = ascii.LowerByte(name[i])
		}
		if rule, ok := willChangeRules[string(buf[:len(name)])]; ok {
			asks |= rule(b)
		}
	}
	return asks
}

// willChangeStacks reports whether a box's will-change makes it a stacking
// context.
func willChangeStacks(b *Box) bool { return willChangeAsksOf(b)&asksStackingContext != 0 }
