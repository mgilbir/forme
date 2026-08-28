package style

// CSS Logical Properties, for the axes this engine has.
//
// A logical property names a side by where the text starts rather than by where
// the page's left is. "margin-inline-start" is the margin before the first
// character of a line — the left margin in English and the right margin in
// Arabic — and "block-size" is the height of a paragraph, which is a height
// because lines stack downwards.
//
// # Which physical property each one sets
//
// The mapping needs two facts: which way the lines stack, and which way the text
// runs along one. This engine lays out one writing mode, horizontal-tb, and says
// so about any other — see style/inert.go and the finding for "writing-mode" —
// so the first fact is fixed here: the block axis is vertical and the inline
// axis horizontal. What is left is "direction", and it is the element's own,
// which is why this cannot be done where the shorthands are expanded: that
// happens once for a stylesheet, and direction is a per-element answer.
//
// When a vertical writing mode arrives, this file is where it lands: the tables
// below become functions of the pair rather than of direction alone, and
// nothing else about the cascade has to know.
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

// logicalSides maps a logical longhand to the physical longhand it sets: first
// the left-to-right answer, then the right-to-left one.
//
// The block-axis entries are the same in both, because direction is about the
// inline axis and nothing else. They are still listed rather than special-cased,
// so that the table reads as one statement about every logical longhand this
// engine knows.
var logicalSides = map[string][2]string{
	// css-sizing's logical sizes. The block axis is the vertical one here, so
	// "block-size" is a height and there is nothing for direction to flip.
	"inline-size":     {"width", "width"},
	"block-size":      {"height", "height"},
	"min-inline-size": {"min-width", "min-width"},
	"max-inline-size": {"max-width", "max-width"},
	"min-block-size":  {"min-height", "min-height"},
	"max-block-size":  {"max-height", "max-height"},

	"margin-block-start":  {"margin-top", "margin-top"},
	"margin-block-end":    {"margin-bottom", "margin-bottom"},
	"margin-inline-start": {"margin-left", "margin-right"},
	"margin-inline-end":   {"margin-right", "margin-left"},

	"padding-block-start":  {"padding-top", "padding-top"},
	"padding-block-end":    {"padding-bottom", "padding-bottom"},
	"padding-inline-start": {"padding-left", "padding-right"},
	"padding-inline-end":   {"padding-right", "padding-left"},

	"inset-block-start":  {"top", "top"},
	"inset-block-end":    {"bottom", "bottom"},
	"inset-inline-start": {"left", "right"},
	"inset-inline-end":   {"right", "left"},

	"border-block-start-width": {"border-top-width", "border-top-width"},
	"border-block-start-style": {"border-top-style", "border-top-style"},
	"border-block-start-color": {"border-top-color", "border-top-color"},
	"border-block-end-width":   {"border-bottom-width", "border-bottom-width"},
	"border-block-end-style":   {"border-bottom-style", "border-bottom-style"},
	"border-block-end-color":   {"border-bottom-color", "border-bottom-color"},

	"border-inline-start-width": {"border-left-width", "border-right-width"},
	"border-inline-start-style": {"border-left-style", "border-right-style"},
	"border-inline-start-color": {"border-left-color", "border-right-color"},
	"border-inline-end-width":   {"border-right-width", "border-left-width"},
	"border-inline-end-style":   {"border-right-style", "border-left-style"},
	"border-inline-end-color":   {"border-right-color", "border-left-color"},
}

// isLogicalLonghand reports whether a property is one of the names above.
//
// It is asked where the registry is asked, so that a logical declaration is not
// reported as a property nobody implements: it is implemented, by being renamed
// a few steps later.
func isLogicalLonghand(name string) bool {
	_, ok := logicalSides[name]
	return ok
}

// physicalName is the property a logical longhand sets, given the direction the
// inline axis runs in.
func physicalName(name string, rtl bool) (string, bool) {
	sides, ok := logicalSides[name]
	if !ok {
		return "", false
	}
	if rtl {
		return sides[1], true
	}
	return sides[0], true
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
func renameLogical(cands []candidate, inline map[string]preparedDecl, rtl bool) bool {
	changed := false
	for i, c := range cands {
		if name, ok := physicalName(c.property, rtl); ok {
			cands[i].property = name
			changed = true
		}
	}
	for logical, d := range inline {
		name, ok := physicalName(logical, rtl)
		if !ok {
			continue
		}
		delete(inline, logical)
		// A style attribute cannot say the same thing twice — the parser keeps
		// one declaration per name — but it can say it once logically and once
		// physically, and the later of the two wins. preparedDecl carries the
		// order it was written in, so the comparison is the same one the
		// cascade makes everywhere else.
		if was, clash := inline[name]; !clash || d.order > was.order {
			inline[name] = d
		}
		changed = true
	}
	return changed
}
