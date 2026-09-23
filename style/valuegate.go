package style

import (
	"sort"

	"github.com/mgilbir/forme/css"
)

// CSS 2.1 §4.2's other half: a declaration whose *value* is not one the
// property takes is dropped whole, and what stands is whatever the cascade
// would have produced without it.
//
// This is where the cascade, @supports and the shorthands ask that question,
// and they ask it of one table: the value grammar in grammar.go and
// grammars.go. It used to be six checks written one after another for six
// properties — the colours, display, background-image, quotes, content and the
// non-negative lengths — and every other property let an invalid value win.
// The six are still here, as the reasons given for a drop: each was found by a
// test that asks for its own wording, and each names the rule an author broke
// more precisely than "not a value of".
//
// Two callers need the answer without the reports: @supports asks whether
// this engine understands a declaration, and the honest answer to "(display:
// grid)" is the one the cascade would give the declaration itself. Asked in two
// places from two lists, the two would drift, and the drift would be invisible
// — a condition answering yes about a declaration the very next rule drops is
// exactly the failure supportsDeclaration is written to avoid.

// judgement is the answer about one declaration.
type judgement struct {
	// drop says the declaration is not applied.
	drop bool
	// unsupported says why is this engine's and not the author's: the value is
	// valid CSS that names something not evaluated here.
	unsupported bool
	why         string
}

// judgeLonghand is the answer for a registered longhand or a logical one.
func judgeLonghand(name string, value []css.ComponentValue) judgement {
	v := judgeValue(name, value)
	switch {
	case !v.ok:
		return judgement{drop: true, why: invalidReason(name, value)}
	case v.unsupported != "":
		return judgement{drop: true, unsupported: true,
			why: unevaluatedReason(name, value, v.unsupported)}
	}
	return judgement{}
}

// judgeExpansion is the answer for a shorthand, given what its expander made of
// it: every longhand is asked, and the first that is not applied decides for
// the whole declaration, which §4.2 applies or drops as one.
func judgeExpansion(name string, value []css.ComponentValue,
	parts map[string][]css.ComponentValue) judgement {

	longhands := make([]string, 0, len(parts))
	for l := range parts {
		longhands = append(longhands, l)
	}
	sort.Strings(longhands)
	var first judgement
	for _, l := range longhands {
		v := judgeValue(l, parts[l])
		switch {
		case !v.ok:
			// Invalid wins over unevaluated wherever it is: the author has a
			// mistake to fix whatever else the value holds.
			return judgement{drop: true, why: invalidReason(name, value)}
		case v.unsupported != "" && !first.drop:
			first = judgement{drop: true, unsupported: true,
				why: unevaluatedReason(name, value, v.unsupported)}
		}
	}
	return first
}

// JudgeValue is the value grammar's answer for a property, for a reader outside
// the cascade that meets a value of it — an @page margin is margin-top's
// grammar. ok is false for a value that is not CSS; unsupported names what, in
// a valid one, this engine does not evaluate.
func JudgeValue(property string, vals []css.ComponentValue) (ok bool, unsupported string) {
	v := judgeValue(property, vals)
	return v.ok, v.unsupported
}

// invalidReason says why a value was dropped as the author's mistake.
func invalidReason(name string, value []css.ComponentValue) string {
	decl := "\"" + name + ": " + serialize(value) + "\""
	switch {
	case nonNegative[name] && hasNegativeNumber(value):
		// A declaration whose value is illegal is not a declaration with a
		// strange value: CSS 2.1 §4.2 says the whole declaration is dropped, and
		// what stands is whatever the cascade would have produced without it.
		//
		// That is why this cannot be done where the value is read. "height: 0;
		// height: -1px" has to compute to zero, and a layout that refuses the
		// negative number sees only the last declaration and falls back to
		// auto — which is a full-height box where the author asked for none.
		// The suite has thirty-five tests of exactly that shape, one per
		// property per unit, and they are what found it.
		return decl + " is negative, which " + name + " does not allow, so the " +
			"declaration was dropped"
	case isColourProperty(name):
		// colors-007 is four paragraphs that must each be green, and two came
		// out black while the invalid declaration was kept: the
		// lower-specificity "p.incorrect { color: green }" never got to apply,
		// because the declaration that should have been thrown away was still
		// standing in front of it.
		return decl + " is not a colour, so the declaration was dropped"
	case name == "background-image":
		// CSS2/backgrounds/background-image-005 asks for green text and gets
		// it, and was counted as a vacuous pass for years because the engine
		// claimed to be missing an image no engine draws. This is not that
		// report.
		return decl + " is not an image, so the declaration was dropped"
	case name == "display":
		// CSS2/abspos/static-fixed-inside-abspos writes "display: absolute" —
		// the author meant "position" — on the div whose background is the
		// green square the test is about; read as inline, nothing of it
		// paints. The declaration never happened, so the UA's block stands.
		return decl + " is not a display value, so the declaration was dropped"
	case name == "quotes":
		// Dropped here rather than where it is read, so that the inherited
		// pairs stand.
		return decl + " is not a list of pairs of strings, so the declaration " +
			"was dropped"
	case name == "content" && !legalCounterFunctions(value):
		// content-counter-016 writes "content: counter(c)" and then four
		// malformed ones after it, and requires the numbering to come from the
		// first.
		return decl + " calls a counter function with arguments it does not " +
			"take, so the declaration was dropped"
	}
	return decl + " is not a valid value of " + name + ", so the declaration " +
		"was dropped"
}

// unevaluatedReason says why a valid value was dropped as this engine's gap.
func unevaluatedReason(name string, value []css.ComponentValue, what string) string {
	return "\"" + name + ": " + serialize(value) + "\" uses " + what +
		", which this engine does not evaluate, so the declaration was dropped " +
		"and whatever it would have overridden stands"
}

// isColourProperty reports whether a property's whole value is a colour,
// logical longhands included — the gap the colour check had, since the rename
// to a physical name happens per element, after the drop (audit C109).
func isColourProperty(name string) bool {
	if sides, ok := logicalSides[name]; ok {
		name = sides[0]
	}
	switch name {
	case "color", "background-color", "border-top-color", "border-right-color",
		"border-bottom-color", "border-left-color", "outline-color",
		"text-decoration-color":
		return true
	}
	return false
}
