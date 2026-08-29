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
// patterns, one set per language, which cannot be derived from anything and has
// to be carried. One is: American English, in hyphentable.go, read by the
// algorithm in hyphenate.go. A document in any other language is read as
// "manual" and reported, which is §6.1's own condition — the UA is required to
// hyphenate only text "for which the author has declared a language ... and for
// which it has an appropriate hyphenation resource" — and is exactly the case
// the unimplemented-property finding exists for.

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
	// Auto is the "auto" value: a word may be broken where a hyphenation
	// dictionary says it may, whether or not the author marked the place.
	//
	// It is a field of its own rather than the absence of None because the
	// three values are three answers and not two: "none" breaks nowhere,
	// "manual" breaks where the author asked, and "auto" breaks there and
	// wherever the language allows.
	Auto bool
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
		// Whether it can be honoured depends on the language, which this does
		// not know: the patterns are for one language and a document in another
		// gets manual hyphens and a finding. So the value is returned as asked
		// for and the caller, which knows the language, decides whether there
		// was anything to report. See HyphenatesLanguage.
		return Hyphens{Auto: true}, ""
	}
	return Hyphens{}, ""
}
