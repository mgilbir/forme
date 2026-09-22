// Package bidi is the Unicode Bidirectional Algorithm, UAX #9: which way each
// character of a paragraph runs, and the order the runs of a line are drawn in.
//
// A layout engine resolves a paragraph once and asks about lines afterwards:
// Resolve takes a paragraph's characters and its base direction — or Auto, for
// rules P2 and P3 — and returns a Paragraph; LineLevels applies rule L1 to one
// line of it once the lines are known, and VisualOrder is rule L2 over the
// levels that returns. A caller with a string and no lines of its own asks
// ResolveRuns or VisualRuns, which answer in runs of one direction; MirrorRunes
// is rule L4 over a right-to-left run, and FirstStrong is P2 without P3's
// default, for the caller that has somewhere else to look.
//
// The character properties are Unicode's: ClassOf, MirrorOf and BracketOf read
// tables.go, which cmd/genbidi generates from the Unicode Character Database
// release the Makefile pins. Shaping happens in logical order, before any of
// this reorders anything; bidi.go says why the two cannot be done the other way
// round, and which of the algorithm's rules are implemented and which are the
// caller's.
package bidi
