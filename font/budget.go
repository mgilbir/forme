package font

import "strconv"

// Budget is the work one font may cost to read, shared by every parser that
// reads it.
//
// # Why one, and why per font
//
// Each parser here used to carry a bound of its own: the cmap walk had a work
// limit per subtable, the charstring interpreter a limit on call depth, and
// FDSelect none at all. Every one of those is local — per call, per subtable, per
// frame — and a font is not obliged to make only one call. N encoding records
// naming one expensive subtable cost N full walks under a budget that each walk
// passed (audit C9); a subroutine calling the next one k times, nine deep, is
// k^8 steps under a depth limit that is never reached (C4). A local bound says
// how much one piece of the font may cost and nothing about what the font as a
// whole may cost, which is the only number that protects anything.
//
// So there is one counter, and a font's parsers all draw on it. It is charged
// for work *attempted* — a mapping walked whether or not it named a glyph, a
// charstring step whether or not it decided a width — because what is being
// bounded is the time spent, and a step that achieved nothing took as long as
// one that did.
//
// A unit is one small step of whatever the parser does: one code a cmap walk
// visits, one charstring operand or operator, one INDEX entry, one DICT byte,
// one composite component. The steps do not cost exactly the same, but they are
// all a handful of instructions and a memory access or two, and one scale is
// what makes one knob possible.
//
// # What it does not count
//
// Work bounded by the font's size once over is not charged — a single pass over
// the hmtx table or the charset — because it cannot be multiplied: nothing in
// the file can make it happen twice. The budget is for the work a font can
// *repeat*: the same bytes named again, a structure walked once per reference
// to it, a call tree fanned out.
//
// FDSelect is the exception, and is charged a unit a glyph written. Read in
// order it is one pass like the others and costs the glyph count, which no
// real font notices; charged, the order is something a test can count rather
// than time, and a read that lost it would show as a count sixteen times over
// rather than as a clock that the race detector's allocation costs also move.
//
// # When it runs out
//
// The parser that found it empty stops, and the budget remembers that it did and
// where. What was read is still correct, but it is not all of the font, and the
// parsers say so in their own terms — ParseSFNT sets Program.BudgetExhausted and,
// where the cmap was cut, CmapPartial; ParseCFF returns nil — so a consumer that
// ignores the budget still cannot mistake a truncated font for a whole one.
//
// A Budget is not safe for concurrent use; it belongs to one parse of one font.
type Budget struct {
	left  int
	limit int
	// what is the part of the font that was being read when the budget ran
	// out, for the error that reports it. Empty until then.
	what string
}

// NewBudget returns a budget of the given number of units. A negative number is
// read as nought: a budget that allows nothing, not one that allows everything.
func NewBudget(units int) *Budget {
	if units < 0 {
		units = 0
	}
	return &Budget{left: units, limit: units}
}

// charge spends n units on reading what, and reports whether there were n to
// spend. Once it has said no it keeps saying no, so a parser that ignores one
// refusal is stopped by the next.
func (b *Budget) charge(n int, what string) bool {
	if b.what != "" {
		return false
	}
	if n > b.left {
		b.left = 0
		b.what = what
		return false
	}
	b.left -= n
	return true
}

// Charge is charge, for a reader of a font's structures outside this package:
// shape's subsetter walks the CFF charstrings and Private DICTs again when it
// cuts a font down, and that walk is as repeatable as the ones here — a
// thousand Font DICTs naming one Private DICT is a thousand readings of it —
// so it draws on an allowance of the same kind and says the same thing when
// the allowance runs out.
func (b *Budget) Charge(n int, what string) bool { return b.charge(n, what) }

// Exhausted reports whether a parser asked this budget for more than it had,
// which means some part of the font was not read.
func (b *Budget) Exhausted() bool { return b.what != "" }

// Spent is the number of units charged so far.
func (b *Budget) Spent() int { return b.limit - b.left }

// Err is the error for a budget that ran out, naming the part of the font that
// was being read when it did, and nil for one that did not.
func (b *Budget) Err() error {
	if b.what == "" {
		return nil
	}
	return budgetError{what: b.what, limit: b.limit}
}

type budgetError struct {
	what  string
	limit int
}

func (e budgetError) Error() string {
	return "fonts: reading " + e.what + " needs more than the " +
		strconv.Itoa(e.limit) + " units of work one font may cost, so the font was not read in full"
}
