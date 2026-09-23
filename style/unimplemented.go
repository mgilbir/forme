package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
)

// Properties the registry accepts and nothing acts on.
//
// # Why this file exists
//
// The registry in property.go is the engine's statement about what it
// understands. A property in it is parsed, cascaded, inherited and resolved —
// and a declaration naming it is *not* reported, precisely because being in the
// registry is what "supported" means.
//
// That statement was wrong for sixteen properties at once, and the way it went
// wrong is worth recording because nothing in the design caught it. Adding a
// property to the registry is the first step of implementing one, and it is the
// step that makes the guardrail go quiet. Whoever adds the entry intends to
// write the code that reads it; if they stop there, the engine claims the
// property, silently ignores it, and reports nothing. "text-align: center"
// produced a flush-left heading with a clean bill of health for as long as the
// engine has existed.
//
// So an entry here is a promise that is not yet kept, and it restores the
// finding that the registry entry suppressed. The value is still cascaded, so
// inheritance and the computed value are right and the day someone implements
// the property there is nothing to undo — the entry is deleted and the report
// stops.
//
// # Why the message is per property
//
// "not implemented" tells an author nothing about what they will see. What they
// need is the consequence: whether the page will be laid out as though the
// declaration were absent, and what that looks like. Each string below finishes
// the sentence "the property was not applied, so ...".
//
// # It is empty, and that is the point
//
// Every property that was here has been implemented. The last was "opacity",
// and where its report went is the thing to read before adding an entry: not
// away, but to the box. An engine that applies a property to most boxes and
// approximates it on the rest has nothing true to say in a table keyed by
// property — the same declaration is honoured on one box and not on the next —
// so the finding is raised where the box is. See layout/opacity.go and
// layout/writingmode.go, which went the same way for the same reason.
//
// An entry here is still the right thing for a property nothing reads at all.
var unimplementedProperties = map[string]string{}

// readByConstruction lists properties whose names are built rather than written.
//
// The guard in unimplemented_test.go looks for each registered property as a
// literal in the source, which cannot see "border-" + side + "-width". Every
// entry here is a property that is genuinely read, by code that assembles its
// name from a side or an edge, and each one is checked by naming the site.
//
// It is a short list on purpose. A property that is neither found as a literal
// nor listed here is one the engine has quietly stopped applying, and that is
// what the guard is for.
//
// The value is the literal fragment the name is assembled from, and the guard
// requires *that* to appear in the source. Without it this map would be an
// unchecked way to wave anything through — which it was, until a planted entry
// for "text-indent" went unnoticed. The padding and margin edges were in here
// too and did not belong: they are read by their full names.
var readByConstruction = map[string]string{
	// layout/layout.go's borderWidths reads "border-" + side + "-width" and
	// "-style"; layout/paint.go's borders reads "border-" + edge + "-color".
	"border-top-width": "border-", "border-right-width": "border-",
	"border-bottom-width": "border-", "border-left-width": "border-",
	"border-top-style": "border-", "border-right-style": "border-",
	"border-bottom-style": "border-", "border-left-style": "border-",
	"border-top-color": "border-", "border-right-color": "border-",
	"border-bottom-color": "border-", "border-left-color": "border-",
}

// unimplementedValues lists registered properties that are read, and the values
// of them nothing acts on, with what that comes to.
//
// It is the value-sized version of the table above, and exists for the break
// properties. Their "avoid" is honoured — layout/multicol.go keeps a column
// from ending where it is asked not to — and their forced values are not
// honoured anywhere: this engine does not break a document into pages (a
// document that does not fit is scaled to the one page), and a multicol pour
// does not end a column where a box asks for one. That is a fact about the
// value and not about the box, so it is said where the value is declared.
var unimplementedValues = map[string]struct {
	values map[string]bool
	reason string
}{
	"break-before": {forcedBreaks, forcedBreakReason},
	"break-after":  {forcedBreaks, forcedBreakReason},
}

var forcedBreaks = map[string]bool{
	"always": true, "all": true, "page": true, "left": true, "right": true,
	"recto": true, "verso": true, "column": true, "region": true,
}

const forcedBreakReason = "no break is made there: this engine does not break a " +
	"document into pages, and does not end a column where a box asks for one"

// unimplementedValueReason returns a declared value of a registered property,
// and why it does nothing, if so. The value is only read for a property that
// has an entry, which is what keeps this off the path of every declaration.
func unimplementedValueReason(name string, vals []css.ComponentValue) (value, reason string, ok bool) {
	entry, listed := unimplementedValues[name]
	if !listed {
		return "", "", false
	}
	value = strings.ToLower(strings.TrimSpace(serialize(vals)))
	if !entry.values[value] {
		return "", "", false
	}
	return value, entry.reason, true
}

// unimplementedReason returns why a registered property does nothing, if so.
func unimplementedReason(name string) (string, bool) {
	reason, ok := unimplementedProperties[name]
	return reason, ok
}
