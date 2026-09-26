package style

import (
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
)

// CSS Logical Properties.
//
// A logical property names a side by where the text starts rather than by where
// the page's left is. "margin-inline-start" is the margin before the first
// character of a line — the left margin in English and the right margin in
// Arabic, and the top margin in vertical Japanese — and "block-size" is the
// extent of a paragraph in the direction its lines stack: a height in English,
// a width in a vertical writing mode.
//
// # Which physical property each one sets
//
// The mapping needs two facts: which way the lines stack, and which way the text
// runs along one — writing-mode and direction, CSS Writing Modes 4 §6 — and both
// are the element's own, which is why this cannot be done where the shorthands
// are expanded: that happens once for a stylesheet, and these are per-element
// answers. physicalSide is the one function that answers "which physical side
// is inline-start, or block-end, for this pair"; everything below asks it.
//
// It used to answer for direction alone, on the assumption that this engine
// lays out one writing mode. That stopped being true when layout/writingmode.go
// arrived, and the note here that said "when a vertical writing mode arrives,
// this file is where it lands" was not acted on: inside "writing-mode:
// vertical-rl", "margin-inline-start: 50px" set the left margin, where inline
// runs top to bottom and the start is the top, and "margin-block-start" set the
// top, where the lines stack right to left and block-start is the right. Nothing
// was reported, because layout's guard sees a physical margin and cannot know
// it started life as a logical one (audit C40).
//
// # Why the rename rather than a second set of properties
//
// Because the cascade has to decide between them. css-logical says a logical
// property and its physical counterpart set the same thing and compete in the
// declaration order they were written in: "margin-left: 1px; margin-inline-start:
// 2px" is a 2px left margin in English, and swapping the two lines makes it 1px.
// An engine that kept them apart and read one after the other would answer by
// which one layout happened to look at, which is not an answer at all.
//
// So a logical declaration is renamed to the physical property it sets, before
// the winner is chosen, and the two then compete like any two declarations of
// one property.

// flowSide is one of the four flow-relative sides of a box.
type flowSide uint8

const (
	blockStart flowSide = iota
	blockEnd
	inlineStart
	inlineEnd
)

// physicalSide is the physical side — "top", "right", "bottom" or "left" — that
// a flow-relative side is for a writing mode and a direction, from CSS Writing
// Modes 4 §6.4's table.
//
// The block axis is set by the writing mode alone: lines stack downwards in
// horizontal-tb, leftwards in vertical-rl and sideways-rl, and rightwards in
// vertical-lr and sideways-lr. The inline axis runs left to right, top to bottom
// or — in sideways-lr, whose glyphs are turned the other way — bottom to top, and
// direction reverses it.
//
// The SVG 1.1 spellings are §3.2's obsolete values: "lr", "lr-tb", "rl" and
// "rl-tb" are horizontal-tb and "tb" and "tb-rl" are vertical-rl.
func physicalSide(side flowSide, writingMode string, rtl bool) string {
	var start, end, lineStart, lineEnd string
	switch ascii.Lower(ascii.TrimCSSSpace(writingMode)) {
	case "vertical-rl", "sideways-rl", "tb", "tb-rl":
		start, end, lineStart, lineEnd = "right", "left", "top", "bottom"
	case "vertical-lr":
		start, end, lineStart, lineEnd = "left", "right", "top", "bottom"
	case "sideways-lr":
		start, end, lineStart, lineEnd = "left", "right", "bottom", "top"
	default:
		start, end, lineStart, lineEnd = "top", "bottom", "left", "right"
	}
	if rtl {
		lineStart, lineEnd = lineEnd, lineStart
	}
	switch side {
	case blockStart:
		return start
	case blockEnd:
		return end
	case inlineStart:
		return lineStart
	}
	return lineEnd
}

// isVertical reports whether a writing mode stacks its lines horizontally, so
// that the inline axis is the vertical one and an inline size is a height.
func isVertical(writingMode string) bool {
	return physicalSide(blockStart, writingMode, false) != "top"
}

// logicalLonghand is what one logical longhand is: a side of a box property
// or a size.
type logicalLonghand struct {
	// pattern is the physical name with "%s" where the side goes —
	// "margin-%s", "border-%s-color", or "%s" for the inset properties, whose
	// physical names are the sides themselves. Empty for a size.
	pattern string
	side    flowSide
	// size is the size's physical name in horizontal-tb, and inline says
	// whether it is on the inline axis; a vertical writing mode swaps width and
	// height. Empty for a side.
	size   string
	inline bool
}

// logicalLonghands is every logical longhand this engine knows.
//
// A variable built by a function rather than filled by an init: the cascade's
// own init derives its non-negative list from this one, and a package's init
// functions run in file order, where a package-level variable is initialised
// before any of them.
var logicalLonghands = buildLogicalLonghands()

func buildLogicalLonghands() map[string]logicalLonghand {
	out := map[string]logicalLonghand{}
	sides := []struct {
		name string
		side flowSide
	}{
		{"block-start", blockStart}, {"block-end", blockEnd},
		{"inline-start", inlineStart}, {"inline-end", inlineEnd},
	}
	for _, s := range sides {
		for _, family := range []struct{ logical, physical string }{
			{"margin-%s", "margin-%s"},
			{"padding-%s", "padding-%s"},
			{"inset-%s", "%s"},
			{"border-%s-width", "border-%s-width"},
			{"border-%s-style", "border-%s-style"},
			{"border-%s-color", "border-%s-color"},
		} {
			name := strings.Replace(family.logical, "%s", s.name, 1)
			out[name] = logicalLonghand{pattern: family.physical, side: s.side}
		}
	}
	// css-sizing's logical sizes.
	for _, prefix := range []string{"", "min-", "max-"} {
		out[prefix+"inline-size"] = logicalLonghand{size: prefix + "width", inline: true}
		out[prefix+"block-size"] = logicalLonghand{size: prefix + "height"}
	}
	return out
}

// isLogicalLonghand reports whether a property is one of the names above.
//
// It is asked where the registry is asked, so that a logical declaration is not
// reported as a property nobody implements: it is implemented, by being renamed
// a few steps later.
func isLogicalLonghand(name string) bool {
	_, ok := logicalLonghands[name]
	return ok
}

// physicalName is the property a logical longhand sets, for the writing mode
// and direction of the element it is on.
func physicalName(name, writingMode string, rtl bool) (string, bool) {
	l, ok := logicalLonghands[name]
	if !ok {
		return "", false
	}
	if l.size != "" {
		if !isVertical(writingMode) {
			return l.size, true
		}
		// The inline axis is the vertical one, so an inline size is a height
		// and a block size a width.
		if strings.HasSuffix(l.size, "width") {
			return strings.TrimSuffix(l.size, "width") + "height", true
		}
		return strings.TrimSuffix(l.size, "height") + "width", true
	}
	return strings.Replace(l.pattern, "%s", physicalSide(l.side, writingMode, rtl), 1), true
}

// logicalProxy is a physical property whose value grammar and range a logical
// longhand shares: every side of one family takes the same values, and so do
// width and height. It is what the checks made once per stylesheet ask, before
// any element's writing mode is known.
func logicalProxy(name string) (string, bool) {
	return physicalName(name, "horizontal-tb", false)
}

// logicalShorthands are the shorthands whose parts are logical.
//
// They expand into logical *longhands* rather than straight to physical ones,
// which keeps the direction out of the stylesheet-wide pass: what "margin-inline:
// 1px 2px" means is "the start margin is 1px and the end margin is 2px" whichever
// way the text runs, and which side that is is settled per element.
//
// "inset" is here for company rather than because it is logical: it is the
// shorthand for the four physical offsets, it is written wherever these are, and
// leaving it out would report it missing beside properties that work.
var logicalShorthands = map[string]shorthand{
	"margin-block":   boxShorthand("margin-block-start", "margin-block-end"),
	"margin-inline":  boxShorthand("margin-inline-start", "margin-inline-end"),
	"padding-block":  boxShorthand("padding-block-start", "padding-block-end"),
	"padding-inline": boxShorthand("padding-inline-start", "padding-inline-end"),
	"inset-block":    boxShorthand("inset-block-start", "inset-block-end"),
	"inset-inline":   boxShorthand("inset-inline-start", "inset-inline-end"),
	"inset":          boxShorthand("top", "right", "bottom", "left"),

	"border-block-width": boxShorthand("border-block-start-width", "border-block-end-width"),
	"border-block-style": boxShorthand("border-block-start-style", "border-block-end-style"),
	"border-block-color": boxShorthand("border-block-start-color", "border-block-end-color"),
	"border-inline-width": boxShorthand("border-inline-start-width",
		"border-inline-end-width"),
	"border-inline-style": boxShorthand("border-inline-start-style",
		"border-inline-end-style"),
	"border-inline-color": boxShorthand("border-inline-start-color",
		"border-inline-end-color"),

	"border-block-start":  borderSides("block-start"),
	"border-block-end":    borderSides("block-end"),
	"border-inline-start": borderSides("inline-start"),
	"border-inline-end":   borderSides("inline-end"),
	"border-block":        borderSides("block-start", "block-end"),
	"border-inline":       borderSides("inline-start", "inline-end"),
}

// renameLogical turns every logical declaration among an element's candidates
// into the physical one it sets, and reports whether it changed anything.
//
// In place, because the candidates are this element's own: they were gathered
// for it and are thrown away after it.
func renameLogical(cands []candidate, inline map[string]preparedDecl,
	writingMode string, rtl bool) bool {

	changed := false
	for i, c := range cands {
		if name, ok := physicalName(c.property, writingMode, rtl); ok {
			cands[i].property = name
			changed = true
		}
	}
	for logical, d := range inline {
		name, ok := physicalName(logical, writingMode, rtl)
		if !ok {
			continue
		}
		delete(inline, logical)
		// A style attribute cannot say the same thing twice — the parser keeps
		// one declaration per name — but it can say it once logically and once
		// physically, and then the two are decided the way any two
		// declarations in one attribute are: importance first, and then the
		// later one. It compared only the order, so "margin-left: 1px
		// !important; margin-inline-start: 2px" came out 2px (audit C155).
		if was, clash := inline[name]; !clash || inlineBeats(d, was) {
			inline[name] = d
		}
		changed = true
	}
	return changed
}
