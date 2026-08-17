package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
)

// Declarations of a property this engine does not implement, whose value asks
// for nothing.
//
// # The finding this narrows
//
// A declaration naming a property the engine does not implement is reported,
// because a page where a property was dropped is plausible and wrong and the
// author has no other way to learn it. That is the right default and it stays.
//
// But it is not true of every such declaration. A property this engine does not
// implement still produces *some* behaviour — usually the one its initial value
// describes, which is why properties have the initial values they do — and a
// declaration asking for exactly that behaviour asks for the page that is
// already there. Reporting one says a declaration did not take effect when there
// was no effect to take.
//
// # Why it is worth the file
//
// Because documents write these constantly, and not by accident. The pattern is
// defensive: an author who wants a control to look like plain text writes
//
//	textarea { margin: 0; padding: 0; border: none; outline: none; resize: none }
//
// and every one of those is "make sure nothing here is doing anything". Four of
// the five are implemented and silent; the fifth was reported, and nineteen of
// the CSS Working Group's reftests could prove nothing because of it.
//
// # The value this engine produces is not always the initial value
//
// This is the trap, and it caught the first version of this file. "An engine
// that does not implement a property renders as though nobody had declared it"
// is very nearly true and reads as obviously true, so the table was written as a
// list of initial values from the specifications.
//
// It is wrong for hyphens. The initial value is "manual", which means *do*
// hyphenate, but only where the text asks with a soft hyphen — and a browser at
// "manual" breaks a line there. This engine breaks at no soft hyphen at all, so
// what it produces is "none", and "manual" is the one value of the three that
// differs from it. Reading the specification alone would have marked the
// difference inert and the match reportable, exactly backwards.
//
// So an entry is a claim about *this engine's behaviour*, checked against it.
// Where that behaviour is a fact a future change could alter — as the hyphens
// entry is — the fact has a test of its own, named beside the entry, so that
// implementing the feature fails the test rather than quietly making the table a
// lie.
//
// # Why this is not the same as going quiet
//
// The rule is about the value, not the property. "resize: none" asks for the
// engine's behaviour and is inert; "resize: both" asks for a resizable box and
// is still reported, because a browser gives one a grab handle and this does
// not.
//
// # The CSS-wide keywords
//
// "initial" is resolved through the table, and is inert only where a property's
// initial value is also what this engine produces. For hyphens it is not, so
// "hyphens: initial" is reported like the "manual" it stands for.
//
// "inherit" takes the parent's value, which this cannot know. "unset" is inherit
// or initial depending on whether the property inherits, and "revert" depends on
// the cascade origin. None is resolved here, so none is treated as inert.

// inertValue describes an unimplemented property whose declarations are
// sometimes inert.
type inertValue struct {
	// produced is the value whose behaviour this engine already produces. A
	// declaration of it asks for the page that is already there.
	produced string
	// initial is the property's initial value, when it differs from produced.
	// Empty means the two are the same.
	//
	// It exists only to resolve the CSS-wide keyword "initial", and the fact
	// that it can differ is the whole reason this is a struct rather than a
	// string. See the note above on hyphens.
	initial string
	// because is the behaviour the entry claims, for a reader checking it.
	because string
}

// inertValues is what this engine produces for each unimplemented property a
// document is likely to declare.
var inertValues = map[string]inertValue{
	// CSS UI 4 §5.1. The property says whether a *user* may resize a box, and
	// a page laid out once offers no way to.
	"resize": {produced: "none", because: "nothing here is resizable by anyone"},

	// CSS Text 4 §7.1. The initial value is "manual", which hyphenates at a
	// soft hyphen; this engine breaks at none, which is "none".
	// TestNoLineBreaksAtASoftHyphen in the layout package is what holds that.
	"hyphens": {produced: "none", initial: "manual",
		because: "no line is broken at a soft hyphen, so nothing is ever hyphenated"},

	// CSS Fonts 4 §6.4 and §6.5. Shaping applies the face's own kerning and its
	// default features, which is what "auto" and "normal" ask for.
	// TestKerningIsApplied in the shape package is what holds the first.
	"font-kerning":            {produced: "auto", because: "the face's kerning is applied"},
	"font-feature-settings":   {produced: "normal", because: "the face's default features are applied"},
	"font-variation-settings": {produced: "normal", because: "no variation is applied beyond the instance"},

	// CSS Fragmentation 3 §3.1. Nothing constrains where a break may fall.
	"page-break-inside": {produced: "auto", because: "no break is avoided"},
	"break-inside":      {produced: "auto", because: "no break is avoided"},

	// CSS Multi-column 1. Content is laid out in one column, which is what a
	// column-count and column-width of "auto" produce.
	"column-count": {produced: "auto", because: "content is laid out in one column"},
	"column-width": {produced: "auto", because: "content is laid out in one column"},
	"column-gap":   {produced: "normal", because: "there is no second column to leave a gap before"},
	"column-fill":  {produced: "balance", because: "there is one column to fill"},

	// CSS Backgrounds 3 §5.1: corners are square.
	"border-radius": {produced: "0", because: "every corner is square"},

	// The identities: CSS Filter Effects 1 §5, CSS Color 4 §3, CSS Transforms 2.
	"filter":              {produced: "none", because: "nothing is filtered"},
	"opacity":             {produced: "1", because: "every box is painted fully opaque"},
	"transform":           {produced: "none", because: "nothing is transformed"},
	"transform-style":     {produced: "flat", because: "there is no 3D rendering context"},
	"backface-visibility": {produced: "visible", because: "nothing is rotated away from the viewer"},

	// CSS Text Decoration 4 §2.6. Decorations are drawn straight through, which
	// is a choice "auto" permits and "none" asks for outright — so both are
	// inert, and "auto" is the one recorded because it is also the initial.
	"text-decoration-skip-ink": {produced: "auto",
		because: "decorations are drawn straight through descenders"},

	// Properties about interaction and animation, none of which a page laid out
	// once has any of.
	"will-change":         {produced: "auto", because: "nothing is optimised for change"},
	"transition":          {produced: "none", because: "nothing transitions"},
	"animation":           {produced: "none", because: "nothing animates"},
	"pointer-events":      {produced: "auto", because: "there is no pointer"},
	"user-select":         {produced: "auto", because: "there is no selection"},
	"touch-action":        {produced: "auto", because: "there is no touch"},
	"scroll-behavior":     {produced: "auto", because: "there is nothing to scroll"},
	"overscroll-behavior": {produced: "auto", because: "there is nothing to scroll"},
}

// isInertDeclaration reports whether a declaration of an unimplemented property
// asks for the page the engine already produces.
//
// The comparison is on the *declared* value rather than a computed one, and that
// is deliberate for the inherited properties here. A child that sets an
// inherited property back to its initial value differs from a browser only if
// the browser would have done something with the inherited value — and since the
// engine does nothing with it either way, the two pages agree. What is still
// reported is the ancestor's declaration, which is where the difference is.
func isInertDeclaration(name string, vals []css.ComponentValue) bool {
	entry, ok := inertValues[name]
	if !ok {
		return false
	}
	value := strings.ToLower(strings.TrimSpace(serialize(vals)))
	if value == "" {
		return false
	}
	// The CSS-wide keyword stands for the property's initial value, which is
	// not always what this engine produces — so it is resolved rather than
	// accepted, and then compared like any other value.
	if value == "initial" {
		value = entry.initial
		if value == "" {
			value = entry.produced
		}
	}
	if value == entry.produced {
		return true
	}
	// A length written as a bare zero and one written with a unit are the same
	// length, and "border-radius: 0px" is as inert as "border-radius: 0".
	return entry.produced == "0" && isZeroLength(value)
}

// isZeroLength reports whether a value is a zero length however it is spelled.
func isZeroLength(value string) bool {
	for _, unit := range []string{"", "px", "pt", "pc", "cm", "mm", "in", "q", "em", "rem", "ex", "ch", "%"} {
		if value == "0"+unit {
			return true
		}
	}
	return false
}
