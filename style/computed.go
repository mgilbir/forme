package style

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
)

// Font-relative lengths, turned into absolute ones where the cascade can.
//
// CSS makes the computed value of a length an absolute length: "the em unit is
// equal to the computed value of the font-size property of the element on which
// it is used", and by the time a value is computed that element is known and the
// unit is gone. That matters because computed values are what inheritance
// passes on. "div { font-size: 28px; margin-left: 4em }" computes to 112px, and
// a child that says "margin: inherit" takes 112px whatever its own font-size —
// the suite's margin-em-inherit-001 asserts exactly that, and says so in its own
// comment: "What is inherited is a computed length value: so it is 56px 84px
// 28px 112px and not 80px 120px 40px 160px".
//
// This engine stored what the author wrote and resolved it in layout, against
// the box's own font-size. That is the right answer for the element that made
// the declaration and the wrong one for every element that inherited it, and it
// is wrong in the direction that hides: a document where every size is set in
// em looks plausible at any scale, and only a fixture that nests to two
// different depths shows the difference.
//
// # What this cannot do
//
// Only em and rem. The other font-relative units need a face — ex is the
// x-height and ch is the advance of "0" — and the cascade has no face: which
// one will set an element is chosen in layout, after the font-family it computes
// here has been read. The viewport units need the page box, which is settled
// later still. Those keep the old behaviour, so a declaration in ex or ch is
// right on the element that made it and inherits as though it had been made
// again lower down.
//
// It is a gap rather than an oversight, and it is worth stating what closing it
// would take: the cascade would have to load fonts, which is a change to when
// fonts are loaded rather than a change to this file.

// absolutiseLengths rewrites every em and rem in a computed style into px.
//
// font-size is skipped because the caller has already resolved it: an em there
// is relative to the *parent's* size rather than the element's, which is the one
// exception CSS carves out, and ResolveFontSize is what knows it.
func absolutiseLengths(cs ComputedStyle, size, root Unit) {
	for name, v := range cs {
		if name == "font-size" || !mightHoldAFontRelativeLength(v) {
			continue
		}
		vals, errs := css.ParseComponentValues(v)
		if len(errs) != 0 {
			// A value that does not tokenize cannot be rewritten without
			// inventing something the author did not write. It is left as it is
			// and whatever reads it makes of it what it can.
			continue
		}
		if !absolutiseValues(vals, size, root) {
			continue
		}
		cs[name] = serialize(vals)
	}
}

// mightHoldAFontRelativeLength is the cheap test that keeps this off the great
// majority of declarations.
//
// Every unit it rewrites ends in "em", so a value without those two letters
// anywhere in it cannot hold one — and the parse below, which is the expensive
// half, never runs for it. A value that contains them and is not a length —
// "font-family: Emblem", "content: 'em'" — reaches the parse and is left alone
// there, because neither is a dimension token.
//
// It is an optimisation and nothing else, which is worth writing down because
// it is the one thing here that no test can catch: made to answer true always,
// every test in this package still passes and the package's own suite goes from
// 0.06s to 0.16s. Every property of every element in every document would be
// tokenized and serialized again, for the handful that hold a length in em.
//
// The consequence for the tests beside it is real, though, and they say so
// where it bites: a fixture written in ex or in per-cent alone never reaches the
// walk at all, so a test of what the walk leaves alone has to put an em beside
// it or it is testing this function instead.
func mightHoldAFontRelativeLength(v string) bool {
	return strings.Contains(v, "em") || strings.Contains(v, "EM") ||
		strings.Contains(v, "eM") || strings.Contains(v, "Em")
}

// absolutiseValues rewrites in place and reports whether it changed anything.
//
// It descends into functions and blocks because a length inside one is still a
// length — and because leaving them out would be a rule that held until the day
// something used them.
func absolutiseValues(vals []css.ComponentValue, size, root Unit) bool {
	changed := false
	for i := range vals {
		if len(vals[i].Values) > 0 && absolutiseValues(vals[i].Values, size, root) {
			changed = true
		}
		if vals[i].Token.Kind != css.Dimension {
			continue
		}
		var basis Unit
		switch strings.ToLower(vals[i].Token.Unit) {
		case "em":
			basis = size
		case "rem":
			basis = root
		default:
			continue
		}
		px := basis.Mul(vals[i].Token.Number)
		vals[i].Token.Unit = "px"
		vals[i].Token.Number = px.Px()
		vals[i].Token.Repr = strconv.FormatFloat(px.Px(), 'f', -1, 64)
		changed = true
	}
	return changed
}

// DefaultFontSize is where a document with no font-size at all is set, and what
// "rem" means on the root element's own font-size.
//
// CSS Values §5.1.1 carves that second case out by name: "when specified on the
// font-size property of the root element, the rem units refer to the property's
// initial value" — because the value it would otherwise refer to is the one
// being computed.
const DefaultFontSize = 16

// fontSizeOf resolves one element's font-size against its parent's.
//
// own says whether a rule set this element's font-size. An element that merely
// inherited one takes its parent's number unchanged: what it inherited is
// already a computed length, and resolving it a second time would compound a
// relative one at every level — a paragraph four elements inside a
// "font-size: 2em" wrapper came out at 256px.
//
// The second result says the value could not be resolved, so the caller leaves
// the declaration alone rather than writing an answer it does not have. Layout
// reports it, where there is an element to report it against.
func fontSizeOf(cs ComputedStyle, own bool, parent, root Unit) (Unit, bool) {
	if !own {
		return parent, true
	}
	vals, errs := css.ParseComponentValues(cs["font-size"])
	if len(errs) != 0 {
		return parent, false
	}
	size, _, ok := ResolveFontSize(vals, parent, root)
	if !ok {
		return parent, false
	}
	return size, true
}

// pxValue renders an absolute length the way a stylesheet would have written it.
func pxValue(u Unit) string {
	return strconv.FormatFloat(u.Px(), 'f', -1, 64) + "px"
}
