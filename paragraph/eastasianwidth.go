package paragraph

// Which characters a width feature could act on.
//
// CSS Fonts 4 §6.9's "full-width" and "proportional-width" ask a font for the
// other of a character's two widths, and the question a caller has before it
// reports that a face could not is whether the run held anything with two. That
// is what these answer, and they answer it from the same table
// "text-transform: full-width" is applied from — see widthtable.go, which is
// generated from Unicode's <wide> and <narrow> decompositions.
//
// It is one table read in both directions, and that is why they are here rather
// than written out where they are used. A list of ranges typed beside the caller
// would be a second statement of the same fact, right on the day it was written
// and wrong at the next Unicode revision: the table is regenerated and a
// hand-written twin is not.

// HasFullWidthForm reports whether a character has a full-width form — an
// ASCII letter, a halfwidth kana, a space that has U+3000 for its wide twin.
//
// It is the necessary condition for 'fwid' to change anything: a font's
// full-width feature covers the characters that have a full-width form, and a
// run holding none of them is set identically with the feature and without.
func HasFullWidthForm(r rune) bool {
	_, ok := lookupWidth(r, fullWidthForms[:])
	return ok
}

// IsFullWidthForm reports whether a character *is* the full-width form of
// another, which is the same question for 'pwid' and is the table read the
// other way.
//
// The reverse index is built once rather than searched linearly, because the
// table is sorted by the character transformed and not by what it becomes.
func IsFullWidthForm(r rune) bool { return fullWidthOf[r] }

// fullWidthOf is every character that appears as a table entry's wide form.
//
// Built at load rather than on first use: it is two hundred-odd entries, the
// cost is a fraction of what parsing one font costs, and a package-level sync
// would be a lock taken on a path that measures text.
var fullWidthOf = func() map[rune]bool {
	out := make(map[rune]bool, len(fullWidthForms))
	for _, pair := range fullWidthForms {
		out[pair.to] = true
	}
	return out
}()
