package paragraph

import "strings"

// hyphens, CSS Text §6.1: whether a word may be broken where it says it may.
//
// A soft hyphen — U+00AD, "&shy;" — is a character an author writes *inside* a
// word to say "break here if you must, and print a hyphen when you do". It is
// invisible everywhere else. That is the whole of the "manual" value, which is
// the property's initial value, so this is not an opt-in feature: it is what
// every document asks for unless it says otherwise, and a page with &shy; in a
// long word was until now a page where the word did not break.
//
// The three values differ in one thing each:
//
//	none      no break at a soft hyphen, and no hyphen ever drawn
//	manual    break at a soft hyphen (and at U+2010, which is visible anyway)
//	auto      the same, and the engine may hyphenate words that ask for nothing
//
// "auto" needs a hyphenation dictionary for the document's language — Liang's
// patterns, one set per language, which is a table this engine does not carry
// and cannot derive. So it is read as "manual" and reported: what it produces
// is right as far as it goes and stops short of what was asked for, which is
// exactly the case the unimplemented-property finding exists for.

// Hyphens is the part of the property that changes what this engine does.
type Hyphens struct {
	// None is the "none" value: no line is broken at a soft hyphen and no hyphen
	// is ever printed.
	//
	// The field is the deviation rather than the behaviour so that the zero value
	// is the property's initial value, which is what WordBreak and LineBreak do
	// and is the difference between a caller that says nothing getting what the
	// specification says and getting the opposite of it.
	None bool
}

// Soft reports whether a soft hyphen offers a break opportunity.
func (h Hyphens) Soft() bool { return !h.None }

// HyphensOf reads the property, and names the value it could not honour.
//
// The initial value is "manual", so an absent or unreadable declaration is one:
// a document that says nothing about hyphens is asking for soft hyphens to
// work.
func HyphensOf(value string) (Hyphens, string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "none":
		return Hyphens{None: true}, ""
	case "auto":
		// Honoured as far as the soft hyphens go, and named, because the rest of
		// it — breaking a word that asked for nothing — is not done.
		return Hyphens{}, "auto"
	}
	return Hyphens{}, ""
}
