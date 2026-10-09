package style

import (
	"sort"

	"github.com/mgilbir/forme/internal/ascii"
)

// CSS Cascade 5 §7.3.3 and §7.3.4: the two CSS-wide keywords that roll the
// cascade back rather than naming a value.
//
// "revert" takes the value the property would have had if no rule of the
// declaration's own origin, or of any origin above it, had been written: an
// author's falls back to the user's sheet and then to the user agent's, a
// user's to the user agent's, and the user agent's to nothing at all, which is
// "unset". "revert-layer" takes the value it would have had if no rule of the
// declaration's own cascade layer had been written, and where nothing else in
// the origin is left that is the previous origin, as for revert.
//
// Both were read as "unset" and reported as not implemented. That is right only
// where nothing below set the property, and the commonest use is exactly where
// something did: "h1 { font-size: revert }" asks for the user agent's 2em and
// got the parent's size.
//
// # Which declarations a roll-back removes
//
// "As if no rules were specified": everything in the group goes, normal and
// important alike, not only the declaration that said revert. So an important
// author revert rolls back to the user's and user agent's declarations of
// either importance, not to the author's own normal ones. The roll-back then
// continues down the ordinary cascade order, and a winner that is itself a
// roll-back keyword rolls back again.
//
// Two places are not stylesheet rules and are placed by the specification:
//
//   - A presentational hint is in an origin of its own below the author's, and
//     part of the author origin for revert but not for revert-layer (§6.1). It
//     carries OriginAuthor and hintLayer, which says both.
//   - A style attribute is an element-attached style, in the author origin and
//     above every author rule of its importance. revert there removes the author
//     origin; revert-layer removes the attribute's own declaration only, which
//     §7.3.4 states for an important one ("it only reverts the element-attached
//     styles ... and not any of the intervening author-origin important rules")
//     and which the cascade order gives for a normal one, since nothing else is
//     in its step.
//
// # What is not here
//
// A var() whose custom property holds revert is not rolled back: this engine
// substitutes no custom property, so the declaration computes to "unset" and
// expand reports that, as it does for every other var().

// rollbackKeyword is the roll-back keyword a winning value is, or "".
func rollbackKeyword(text string) string {
	switch kw := ascii.Lower(ascii.TrimCSSSpace(text)); kw {
	case kwRevert, kwRevertLayer:
		return kw
	}
	return ""
}

// rollback is one element's candidates, indexed by property the first time a
// roll-back asks for one.
//
// The index is built at most once per element and only for an element where a
// roll-back keyword won, so a document that uses neither keyword pays nothing,
// and one that reverts many properties of an element pays one pass over its
// candidates rather than one per property.
type rollback struct {
	cands      []candidate
	byProperty map[string][]int
}

// ordered is a property's candidates, strongest first by beats.
func (r *rollback) ordered(name string) []candidate {
	if r.byProperty == nil {
		r.byProperty = map[string][]int{}
		for i, c := range r.cands {
			r.byProperty[c.property] = append(r.byProperty[c.property], i)
		}
	}
	idx := r.byProperty[name]
	out := make([]candidate, len(idx))
	for i, at := range idx {
		out[i] = r.cands[at]
	}
	sort.Slice(out, func(i, j int) bool { return beats(out[i], out[j]) })
	return out
}

// cascadeGroup is a cascade layer within an origin: what revert-layer removes.
type cascadeGroup struct {
	origin Origin
	layer  int
}

// rollBack is the declared value of a property whose winner is revert or
// revert-layer: the strongest declaration left once each roll-back has removed
// its group, or "unset" where nothing is left.
//
// list is the property's stylesheet candidates in cascade order, strongest
// first; inline is its style-attribute declaration where hasInline. fromInline
// says the answer is the attribute's text.
//
// It is one walk down the list. A roll-back only ever removes declarations, and
// it removes the one that asked for it, so the strongest declaration left can
// only move down: the walk never goes back, and the cost is the length of the
// list however many layers roll back in turn.
func rollBack(list []candidate, inline preparedDecl, hasInline bool) (text string, fromInline bool) {
	// Every origin at or above this one is removed. OriginAuthor+1 is none.
	removedFrom := OriginAuthor + 1
	var removed map[cascadeGroup]bool
	inlineRank := CascadeRank(OriginAuthor, inline.important)
	i := 0
	for {
		for i < len(list) && (list[i].origin >= removedFrom ||
			removed[cascadeGroup{list[i].origin, list[i].layer}]) {
			i++
		}
		// The style attribute, where it is still in the cascade and beats the
		// strongest rule left — the same comparison winning makes.
		if hasInline && OriginAuthor < removedFrom &&
			(i == len(list) || inlineRank >= cascadeRank(list[i])) {
			switch rollbackKeyword(inline.text) {
			case kwRevert:
				removedFrom = OriginAuthor
			case kwRevertLayer:
				// Its own step of the cascade, and nothing else is in it.
			default:
				return inline.text, true
			}
			hasInline = false
			continue
		}
		if i == len(list) {
			return kwUnset, false
		}
		c := list[i]
		switch rollbackKeyword(c.text) {
		case kwRevert:
			removedFrom = c.origin
		case kwRevertLayer:
			if removed == nil {
				removed = map[cascadeGroup]bool{}
			}
			removed[cascadeGroup{c.origin, c.layer}] = true
		default:
			return c.text, false
		}
	}
}
