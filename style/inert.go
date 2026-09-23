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
	// inherits says the property is an inherited one, which decides what three
	// of the four CSS-wide keywords stand for. "unset", "revert" and
	// "revert-layer" are the initial value on a property that does not inherit,
	// and the parent's on one that does — and the parent's is not knowable
	// here, so an inherited property's is left as written and reported.
	//
	// It is a field rather than a lookup because these are the properties this
	// engine does *not* implement: none of them is in the property table, which
	// is where inheritance is recorded for the ones that are.
	inherits bool
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
	"font-variation-settings": {inherits: true, produced: "normal", because: "no variation is applied beyond the instance"},

	// CSS Fragmentation 3's break properties are not here any more, and the
	// reason they were is the one TestNothingIsFragmented was written to
	// catch going stale. "avoid" was inert on the ground that "this engine does
	// not fragment at all", and that stopped being true when multicol.go began
	// cutting boxes across columns: a box that asked not to be split was split,
	// and the declaration that said so was the one thing kept quiet. They are
	// registered properties now and layout/multicol.go honours "avoid"; the
	// forced values nothing makes are reported where they are declared — see
	// unimplementedValues.

	// CSS Multi-column 1's four are not here any more. They are registered
	// properties now and layout reads them — see layout/multicol.go — so the
	// question "is this page missing something" is about the box that asked for
	// columns and not about the declaration, exactly as it is for writing-mode.

	// CSS Writing Modes 4's three properties are not here, and where they went is
	// worth recording. All three were inert for one reason — this engine laid out
	// horizontal-tb and nothing else, so a property that "has no effect in
	// horizontal writing modes" had no effect here at all — and that reason has
	// stopped being true. They are registered properties now, they cascade, and
	// layout reads them.
	//
	// The report moved with them, from the stylesheet to the box. Whether a page
	// came out wrong is a question about the box that declared a vertical mode and
	// not about the declaration: the same "writing-mode: vertical-rl" is laid out
	// on one box and reported on the next, and a table keyed by property has no
	// way to say that. See layout/writingmode.go.
	//
	// The two suite documents this table was built around are still silent, and
	// for the reason they always were rather than by luck.
	// text-autospace-elements-005b declares "text-orientation: upright" with the
	// comment "should NOT affect auto-spacing in horizontal mode", and
	// text-autospace-003 writes it beside text-combine-upright under "these
	// properties have no effect on horizontal text": both are horizontal, so
	// neither box is turned and neither property changes anything, which is what
	// the layout-time check asks before it says a word.

	// CSS Backgrounds 3 §5.1: corners are square.
	"border-radius": {produced: "0", because: "every corner is square"},

	// The identities: CSS Filter Effects 1 §5 and CSS Transforms 2. CSS Color
	// 4 §3's opacity was here and is not any more — it is implemented, and what
	// it cannot express is reported at the box that asked for it rather than at
	// the declaration. See layout/opacity.go.
	"filter":              {produced: "none", because: "nothing is filtered"},
	"transform":           {produced: "none", because: "nothing is transformed"},
	"transform-style":     {produced: "flat", because: "there is no 3D rendering context"},
	"backface-visibility": {produced: "visible", because: "nothing is rotated away from the viewer"},

	// CSS Text Decoration 4 §2.6. Decorations are drawn straight through, which
	// is a choice "auto" permits and "none" asks for outright — so both are
	// inert. "auto" is the produced value because it is also the initial;
	// "none" is the one documents actually write, and writing it is the author
	// making sure of the very thing this engine has no other way of doing. See
	// TestADecorationIsDrawnStraightThroughADescender.
	"text-decoration-skip-ink": {inherits: true, produced: "auto", also: "none",
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
	"pointer-events":      {inherits: true, produced: "auto", because: "there is no pointer"},
	"user-select":         {produced: "auto", because: "there is no selection"},
	"touch-action":        {produced: "auto", because: "there is no touch"},
	"scroll-behavior":     {produced: "auto", because: "there is nothing to scroll"},
	"overscroll-behavior": {produced: "auto", because: "there is nothing to scroll"},

	// The rest of the defensive reset, and it is the same shape as the textarea
	// rule at the top of this file: a sheet that says "nothing here is doing
	// anything" property by property. Every one of these was reported, and each
	// report said a declaration had been dropped when there was no effect in it
	// to drop.

	// CSS Backgrounds 3 §6 and CSS Text Decoration 4 §6. Nothing is drawn behind
	// a box or behind a glyph, so a declaration asking for no shadow asks for the
	// page that is already there. A shadow that asks for something stays
	// reported: an author who wrote one gets a flat page instead.
	"box-shadow":  {produced: "none", because: "no shadow is drawn behind a box"},
	"text-shadow": {inherits: true, produced: "none", because: "no shadow is drawn behind text"},

	// CSS UI 4 §8.1 and CSS Contain 2 §4. A page laid out once has no pointer to
	// put a cursor under, and it renders every box it lays out rather than
	// skipping any — "visible" and "none" are what that comes to. The values
	// these decline stay reported: "content-visibility: hidden" asks for a
	// subtree not to be painted, and this paints it.
	"cursor":             {inherits: true, produced: "auto", because: "there is no pointer, so no cursor is chosen"},
	"content-visibility": {produced: "visible", because: "every box is laid out and painted"},
	"contain":            {produced: "none", because: "nothing is contained"},

	// CSS Compositing 1 §3 and §4, and CSS Filter Effects 2 §2. Fills are
	// composited in source order and nothing is blended with what is under it,
	// which is what "normal" asks for; with no blending there is nothing for an
	// isolated group to hold apart, and nothing filters what is behind a box.
	"mix-blend-mode":  {produced: "normal", because: "nothing is blended with what is under it"},
	"isolation":       {produced: "auto", because: "nothing is blended, so there is no group to isolate"},
	"backdrop-filter": {produced: "none", because: "nothing behind a box is filtered"},

	// CSS Masking 1 §4 and §6. A box is painted whole.
	"clip-path": {produced: "none", because: "nothing is clipped to a shape"},
	"mask":      {produced: "none", because: "nothing is masked"},

	// CSS Transforms 2 §3 and §5. The engine transforms nothing — "transform"
	// above says so — and a perspective with nothing to see through it is the
	// same fact again.
	//
	// transform-origin is the one property here whose *every* value is inert, and
	// it is inert for a reason rather than by luck: the property does not do
	// anything on its own. It names the point a transform turns about, so a
	// document that declares it either declares a transform too — which is
	// reported, at the declaration, by the entry above — or declares an origin for
	// a transformation that was never asked for. Either way nothing is lost by
	// this being silent, and every spelling of the same point ("center", "50%
	// 50%", "top left", "0 0") is one fewer report of a difference that is not
	// there. If transform is ever implemented, this entry has to go with it.
	"perspective":      {produced: "none", because: "there is no perspective to see through"},
	"transform-origin": {always: true, because: "nothing is transformed, so no transformation has an origin"},

	// CSS Text Decoration 4 §3.2 and §2.5, and CSS Fonts 4 §4.5 and §6.9.
	//
	// Two of these are the hyphens trap and the text-decoration-skip-ink case
	// respectively, which is why they are written out rather than listed.
	//
	// text-underline-position: "auto" leaves the position to the UA, and
	// "from-font" requires it to come from the face's own metrics. This engine
	// takes it from the face's post table whenever the face states one, so it
	// satisfies both — the permitting value and the demanding one, which is what
	// "also" is for. See TestUnderlineComesFromTheFaceThatStatesOne. "under" asks
	// for the line below the descenders and is still reported.
	//
	// font-optical-sizing: the initial value is "auto", and it is *not* what this
	// engine produces. "auto" asks for the face's optical size axis to be set from
	// the font size, and this engine applies no variation beyond the instance it
	// was given — see font-variation-settings above, and TestKerningIsApplied's
	// neighbours in the shape package. So what it produces is "none", and "auto"
	// is the value that is still reported. Exactly the hyphens case, found by
	// looking for it.
	"text-underline-position": {inherits: true, produced: "auto", also: "from-font",
		because: "the underline is placed from the face's own metrics"},
	"font-optical-sizing": {inherits: true, produced: "none", initial: "auto",
		because: "no variation is applied beyond the instance, optical sizing included"},
	"text-emphasis":       {inherits: true, produced: "none", because: "no emphasis mark is drawn"},
	"text-emphasis-style": {inherits: true, produced: "none", because: "no emphasis mark is drawn"},
	"font-variant-alternates": {inherits: true, produced: "normal",
		because: "no alternate glyphs are selected"},

	// CSS Scroll Snap 1 §6 and CSS UI 4 §5.2. There is nothing to scroll, and an
	// outline drawn at the border edge is what a zero offset asks for. A non-zero
	// offset moves the ring and is still reported.
	// CSS UI 4 §6.1. This engine's controls take their chrome from its own user
	// agent stylesheet — a field is a bordered inline-block, a button a raised
	// box — and it draws that whether or not a document asks. "auto" is a
	// document asking for it, which is the page that is already there.
	//
	// "none" is the opposite request and stays reported, because it is the one
	// that would change something: an author writing it wants the field stripped
	// back to a plain box, and the border and the padding stay. See uastyle.go's
	// text-entry chrome.
	"appearance": {produced: "auto", because: "a control keeps the chrome the user agent sheet gives it"},

	"scroll-snap-type": {produced: "none", because: "there is nothing to scroll, so nothing snaps"},

	// The rest of the scrolling geometry, all of which nomedium.go names too.
	//
	// These four are about the *report* rather than about the difference. The
	// table next door already says no value of them can change a page, so
	// nothing here claims a page came out wrong either way; what these add is
	// silence for the value that asks for the geometry that is already there,
	// which is the value a reset writes. A stylesheet saying "scroll-margin: 0"
	// is telling a browser not to hold a box off the edge it snaps to, and there
	// is no edge.
	"scroll-margin":   {produced: "0", because: "nothing scrolls, so no box has a snap area"},
	"scroll-padding":  {produced: "auto", because: "nothing scrolls, so there is no scrollport to inset"},
	"overflow-anchor": {produced: "auto", because: "nothing scrolls and nothing moves after layout"},
	"scrollbar-color": {produced: "auto", because: "there is no scrollbar to colour"},
	"outline-offset":  {produced: "0", because: "an outline is drawn at the border edge"},
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
	// A CSS-wide keyword stands for a value rather than being one, so it is
	// resolved before the comparison — and there are four of them, not one.
	//
	// "initial" is the property's initial value, which is not always what this
	// engine produces. "unset", "revert" and "revert-layer" are the same value
	// again for a property that does not inherit, which is what "as though
	// nothing had been said" comes to when there is no parent value to fall back
	// to; for one that *does* inherit they are the parent's value, which this
	// cannot know — so they are left alone, and a declaration written with one
	// is not called inert.
	//
	// Only "initial" was resolved. So "resize: unset" — the same page as
	// "resize: none", which is listed — was reported as a difference from a
	// browser that there is not.
	initial := entry.initial
	if initial == "" {
		initial = entry.produced
	}
	switch value {
	case kwInitial:
		value = initial
	case kwUnset, kwRevert, kwRevertLayer:
		if !entry.inherits {
			value = initial
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
