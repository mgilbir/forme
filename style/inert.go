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
// It was wrong for hyphens, and that entry is worth recording even though it is
// no longer here. The initial value is "manual", which means *do* hyphenate, but
// only where the text asks with a soft hyphen. This engine used to break at no
// soft hyphen at all, so what it produced was "none" — and reading the
// specification alone would have marked "manual" inert and "none" reportable,
// exactly backwards.
//
// So an entry is a claim about *this engine's behaviour*, checked against it.
// Where that behaviour is a fact a future change could alter, the fact has a
// test of its own, named beside the entry, so that implementing the feature
// fails the test rather than quietly making the table a lie. That is not a
// hypothetical: breaking at a soft hyphen was implemented, the test the hyphens
// entry named failed and pointed here, and the property left this file — it is
// implemented now, and what is left unimplemented is one *value* of it, which is
// reported where word-break's and line-break's are.
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
	// also is a second value the engine's behaviour satisfies, where a property
	// has one. It is not a convenience: a value that *permits* a behaviour and a
	// value that *requires* it are both satisfied by an engine that always does
	// it, and those are two different declarations rather than two spellings of
	// one. text-decoration-skip-ink is the case — "auto" lets a decoration be
	// drawn straight through and "none" asks for it — and it is the only entry
	// here that needs two, which is why this is one field and not a list.
	also string
	// always marks a property whose *every* value asks for the page that is
	// already there, so that there is nothing to compare. It is a different
	// claim from produced and a rarer one — see text-orientation, which is the
	// only entry that makes it.
	always bool
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

	// CSS Fonts 4 §6.4 and §6.5. Shaping applies the face's own kerning and its
	// default features, which is what "auto" and "normal" ask for.
	// TestKerningIsApplied in the shape package is what holds the first.
	"font-variation-settings": {produced: "normal", because: "no variation is applied beyond the instance"},

	// CSS Fragmentation 3 §3.1, and the two values are inert for opposite
	// reasons that meet in the same page.
	//
	// "auto" permits a break inside the box. "avoid" asks for none — and this
	// engine puts none inside any box, because it does not fragment at all: a
	// document that does not fit is *scaled* to the page rather than broken
	// across two of them (see page.go). So the box the author did not want split
	// is not split, which is what the declaration asked for.
	//
	// The other break properties are not here and must not join them.
	// "page-break-before: always" asks for a break this engine cannot make, and
	// an author who wrote one would get a page that runs on.
	"page-break-inside": {produced: "auto", also: "avoid",
		because: "nothing is fragmented, so no box is broken inside"},
	"break-inside": {produced: "auto", also: "avoid",
		because: "nothing is fragmented, so no box is broken inside"},

	// CSS Multi-column 1. Content is laid out in one column, which is what a
	// column-count and column-width of "auto" produce.
	"column-count": {produced: "auto", because: "content is laid out in one column"},
	"column-width": {produced: "auto", because: "content is laid out in one column"},
	"column-gap":   {produced: "normal", because: "there is no second column to leave a gap before"},
	"column-fill":  {produced: "balance", because: "there is one column to fill"},

	// CSS Writing Modes 4 §4.1, whose own note is the entry: "this property has
	// no effect in horizontal writing modes". This engine has no other kind — a
	// document that asks for one is told so by the writing-mode finding, which
	// is where that gap belongs — so every value of this one asks for the page
	// that is already there, and there is no value to compare against.
	//
	// The suite writes it to say exactly that. text-autospace-elements-005b
	// declares "text-orientation: upright" with the comment "should NOT affect
	// auto-spacing in horizontal mode".
	"text-orientation": {always: true,
		because: "there are no vertical writing modes here for it to have an effect in"},

	// And the property that decides the mode, at the value every page here is
	// laid out in. The other four ask for a page turned on its side, which is as
	// different as a page can be, and are reported.
	"writing-mode": {produced: "horizontal-tb",
		because: "every page here is laid out in horizontal-tb"},

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
	// inert. "auto" is the produced value because it is also the initial;
	// "none" is the one documents actually write, and writing it is the author
	// making sure of the very thing this engine has no other way of doing. See
	// TestADecorationIsDrawnStraightThroughADescender.
	"text-decoration-skip-ink": {produced: "auto", also: "none",
		because: "decorations are drawn straight through descenders"},

	// CSS Text Decoration 3 §2.2. A decoration is drawn as a solid line, which
	// is what the property's initial value asks for. The other four — double,
	// dotted, dashed, wavy — are not here: each asks for a line this engine does
	// not draw, and an author who wrote one would see a solid one instead.
	"text-decoration-style": {produced: "solid",
		because: "every decoration is drawn as a solid line"},

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
	if entry.always {
		return true
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
	if value == entry.produced || (entry.also != "" && value == entry.also) {
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
