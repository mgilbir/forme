package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS Writing Modes 4: a block whose lines run down the page.
//
// # What is laid out
//
// One mode, "vertical-rl", and only for a box turnable() accepts. Everything
// else — the other three vertical modes, and a vertical-rl box this cannot lay
// out — is reported per box and laid out horizontally, which is the page that
// was already there.
//
// # Why a rotation and not an axis abstraction
//
// The usual way to write this is to give layout a pair of abstract axes and say
// "inline" and "block" everywhere it currently says width and height. That is
// the right shape for an engine that lays out both modes at once, and it is a
// change to every arithmetic line in the package — each of which is verified by
// the reftest suite today and would have to be re-verified after.
//
// This does the other thing. A vertical-rl box lays its content out in the
// *horizontal* engine, in a frame whose width is the box's content height, and
// the finished subtree is then turned ninety degrees clockwise onto the page.
// The two are the same picture: turning a horizontal page clockwise puts its
// first line down the right-hand edge and runs its text from top to bottom,
// which is what vertical-rl is.
//
// It works because of an accident of the number ninety. A quarter turn maps an
// axis-aligned rectangle to an axis-aligned rectangle, so every background,
// border, outline and clip in the display list survives it unchanged in kind —
// and the display list needs one new fact rather than a transform stack. That
// fact is DrawText.Sideways, because glyphs are the one thing a rotation does
// not leave alone.
//
// The limit of the trick is exactly the set of things that are not a rotation of
// the horizontal page, and turnable() is the list. "text-orientation: upright"
// is the biggest of them: an upright ideograph on a vertical line is not any
// rotation of a horizontal line, and no amount of turning produces one.
//
// # Why the report is per box
//
// It used to be per stylesheet — "writing-mode" was an unimplemented property
// and one declaration of it made a whole document unsupported. That answer is
// no longer available, because the same declaration is now laid out on one box
// and refused on the next: a div with a height is turned and the div beside it
// with an automatic one is not. Which of the two the reader has is the whole of
// what they need to know, and only a finding about the box can say it.

// writingMode is which way a box's lines stack and its text runs along one.
type writingMode uint8

const (
	// horizontalTB is the initial value and what every box was before this:
	// lines stack downwards and text runs across them.
	horizontalTB writingMode = iota
	// verticalRL is the one that is laid out: lines stack from the right edge
	// leftwards, text runs from the top downwards.
	verticalRL
	// The three that are read and reported. They are told apart rather than
	// lumped together because the finding names the value the author wrote, and
	// "this is not laid out" is a more useful sentence when it is about the mode
	// in front of them.
	verticalLR
	sidewaysRL
	sidewaysLR
)

// writingModeOf reads the computed value.
//
// An unknown keyword is horizontal-tb rather than a fifth case: the cascade only
// admits a value the property's grammar accepts, so anything else here is the
// initial value arriving by another route, and treating it as the page that is
// already there is what every other reader of a computed style in this package
// does.
func writingModeOf(b *Box) writingMode {
	if b == nil {
		return horizontalTB
	}
	if b.Anonymous() || b.IsText() {
		// A box the engine made rather than the author. It has no declarations
		// of its own and it is not what a writing mode changes at: asking its
		// parent is what makes the anonymous block that wraps a paragraph's
		// text run the same way the paragraph does, rather than looking like a
		// second box declaring horizontal-tb inside a vertical one.
		return writingModeOf(b.Parent)
	}
	switch strings.ToLower(strings.TrimSpace(b.Style["writing-mode"])) {
	case "vertical-rl":
		return verticalRL
	case "vertical-lr":
		return verticalLR
	case "sideways-rl":
		return sidewaysRL
	case "sideways-lr":
		return sidewaysLR
	}
	return horizontalTB
}

func (m writingMode) vertical() bool { return m != horizontalTB }

// String is the keyword, for the finding. It is the author's spelling of the
// value and not a description of it, because the sentence it goes into is about
// the declaration they wrote.
func (m writingMode) String() string {
	switch m {
	case verticalRL:
		return "vertical-rl"
	case verticalLR:
		return "vertical-lr"
	case sidewaysRL:
		return "sideways-rl"
	case sidewaysLR:
		return "sideways-lr"
	}
	return "horizontal-tb"
}

// turnEdges maps the four sides of a box through the quarter turn.
//
// Clockwise: what was the left side of the horizontal frame is the top of the
// page, what was the top is the right, and so round. The margins, borders and
// padding of a box inside a turned one are physical by the time anything paints
// them, and this is where they become physical.
func turnEdges(e Edges) Edges {
	return Edges{Top: e.Left, Right: e.Top, Bottom: e.Right, Left: e.Bottom}
}

// turnRect maps one rectangle of the horizontal frame onto the page.
//
// mirror is the physical width of the content box the rectangle is positioned
// inside — the block axis of a vertical-rl box runs leftwards from its
// block-start edge, which is its right one, so the block coordinate is measured
// back from there.
//
// The inline coordinate needs no mirror: text runs from the top downwards in
// both vertical modes, which is the same direction a horizontal line's own
// coordinate already runs in.
func turnRect(r Rect, mirror style.Unit) Rect {
	return Rect{
		X: mirror.Sub(r.Y).Sub(r.H),
		Y: r.X,
		W: r.H,
		H: r.W,
	}
}

// turnContent turns everything inside a box that was laid out sideways.
//
// The box itself is not turned. It sits in its parent's horizontal flow and its
// own margins, borders and padding are the physical ones the author declared —
// "padding-top" is the top of the page whichever way the text inside runs. What
// turns is what the box holds.
//
// The walk carries each level's own physical content width rather than the
// outermost one, because the mirror is per containing block: a line inside a
// nested block starts at *that* block's right edge. Composing the turn level by
// level like this is what makes it a rigid motion of the whole subtree rather
// than of its first generation.
func turnContent(f *Fragment) {
	mirror := f.ContentRect().W
	for i := range f.Lines {
		turnLine(&f.Lines[i], mirror)
	}
	for _, c := range f.Children {
		turnFragment(c, mirror)
	}
}

func turnFragment(f *Fragment, mirror style.Unit) {
	if f == nil {
		return
	}
	f.BorderRect = turnRect(f.BorderRect, mirror)
	for i := range f.bgBands {
		f.bgBands[i] = turnRect(f.bgBands[i], mirror)
	}
	f.Margin = turnEdges(f.Margin)
	f.Border = turnEdges(f.Border)
	f.Padding = turnEdges(f.Padding)
	// contentH is a block-axis extent, which is a width on the page. It is read
	// only by the overflow arithmetic, which asks it of the box it belongs to
	// and never compares it across a turn, so it keeps its name and changes its
	// meaning here along with everything else.
	turnContent(f)
}

// turnLine turns one line box and the inline boxes hanging off it.
//
// The runs on the line are left as they are. A run's X is its offset along the
// line, which is an inline coordinate in both frames, and where that offset ends
// up on the page is a question the painter answers from the line's Sideways flag
// — see painter.lines. Doing it here instead would mean turning a position that
// is not a rectangle and has no extent, which is exactly the arithmetic that
// belongs beside the baseline it is measured from.
func turnLine(l *LineFragment, mirror style.Unit) {
	l.Rect = turnRect(l.Rect, mirror)
	l.Sideways = true
	for _, ib := range l.Boxes {
		// An inline box's fragment is positioned in the block's content
		// coordinates, the same ones the line is, so it takes the same mirror.
		// Its own children are the boxes further in, which turnFragment reaches
		// through turnContent.
		ib.BorderRect = turnRect(ib.BorderRect, mirror)
		for i := range ib.bgBands {
			ib.bgBands[i] = turnRect(ib.bgBands[i], mirror)
		}
		ib.Margin = turnEdges(ib.Margin)
		ib.Border = turnEdges(ib.Border)
		ib.Padding = turnEdges(ib.Padding)
	}
}

// turns decides whether a box is laid out sideways, and says so about the ones
// that are not.
//
// It is called once per block-level box, from blockIn, and answers three
// questions in order: does this box change the writing mode at all, is the mode
// it changes to one that is laid out, and is the box one the turn is exact for.
// Only the first has a quiet "no": a box that does not change the mode is a box
// whose parent already answered, and a box inside an untuned vertical parent is
// laid out horizontally because its parent was — the finding belongs on the
// parent, and repeating it on every descendant would bury it.
func (l *layouter) turns(b *Box, containing style.Unit, hasHeight bool) bool {
	mode := writingModeOf(b)
	if mode == writingModeOf(b.Parent) {
		return false
	}
	if mode == horizontalTB {
		// A box that turns *back*, inside a vertical ancestor. There is nothing
		// to report — horizontal-tb is the mode this engine lays out — and
		// nothing to do either: the ancestor it sits in was refused, because
		// refusesToTurn will not turn a box holding one of these, so the whole
		// subtree is already being laid out horizontally.
		return false
	}
	if mode == verticalRL {
		why := l.refusesToTurn(b, containing, hasHeight)
		if why == "" {
			return true
		}
		l.reportWritingMode(b, mode, why)
		return false
	}
	l.reportWritingMode(b, mode, "only \"vertical-rl\" is laid out")
	return false
}

func (l *layouter) reportWritingMode(b *Box, mode writingMode, why string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "\"writing-mode: " + mode.String() + "\" was not applied to this box because " +
			why + "; its lines run across the page rather than down it",
		Path:     PathOf(b.Element),
		Property: "writing-mode",
	})
}

// refusesToTurn is why a vertical-rl box is laid out horizontally anyway, or the
// empty string if it is turned.
//
// Every clause is a way for the quarter turn to stop being the same picture as
// the vertical mode it stands in for, and each is stated as a condition on the
// box rather than as a list of features because that is the form the reader of
// the finding needs: "this box has a floated child" tells them what to change.
//
// The set is deliberately narrow, and narrowing is the safe direction. A box
// refused here is laid out exactly as it was before this file existed and is
// reported, which is the honest answer; a box turned that should not have been
// is a page that is quietly wrong.
func (l *layouter) refusesToTurn(b *Box, containing style.Unit, hasHeight bool) string {
	if b.Outer != OuterBlock || b.Inner != InnerFlow {
		// A table, a flow root, an inline-block. Each has sizing rules of its
		// own that resolve the two axes together, and turning the result would
		// mean turning arithmetic this has not been through.
		return "it is not an ordinary block box"
	}
	if b.Float != FloatNone || b.Position != PositionStatic || b.Replaced != nil {
		return "it is floated, positioned or replaced"
	}
	if !hasHeight {
		// The height of a vertical box is the length of its lines, and lines
		// have to be broken against a length that is known before they are
		// broken. CSS Writing Modes §7.3 has an answer for the automatic case —
		// the orthogonal flow rules, which fall back to the size of the viewport
		// — and it is a different feature from this one.
		return "its height is automatic, so there is no length to break its lines against"
	}
	if _, ok := l.explicitWidth(b, containing); !ok {
		// And the width is the block axis, which an automatic value would fill
		// the containing block with rather than fit to the lines inside.
		return "its width is automatic, so there is no room for its lines to stack in"
	}
	return l.subtreeRefusesToTurn(b, b)
}

// The properties whose meaning is a side of the page rather than a side of the
// text, which is what makes them the ones a turn cannot carry.
//
// "width" is the clearest case. Inside a turned box the engine is laying out a
// horizontal page, so it reads "width" as the length of a line; on the finished
// page that length runs down rather than across, and CSS still means by "width"
// the distance from the box's left edge to its right one. The two are at right
// angles, and the second is what the author asked for.
//
// The properties that are *not* here are the other half of the argument, and
// they are not an oversight. text-align, text-indent, letter-spacing and
// word-spacing are along the line; line-height and vertical-align are across it;
// direction is along it. Every one of those is defined in the text's own axes
// and comes through the turn saying the same thing it said before, which is why
// a turned box can centre its text and raise its superscripts and be right.
// Each is paired with its own initial value, because that is the value a box
// that declared nothing carries: computed styles hold every registered property
// and not only the ones an author wrote, so a check against the empty string
// finds "min-width: 0" on every element in the document and refuses to turn
// anything at all. That was not a hypothetical — it is what the <br> in the
// suite's hyphens-vertical-001 did on the first run of this file.
var physicalGeometry = [...]struct{ name, initial string }{
	{"width", "auto"},
	{"height", "auto"},
	{"min-width", "0"},
	{"min-height", "0"},
	{"max-width", "none"},
	{"max-height", "none"},
	{"top", "auto"},
	{"right", "auto"},
	{"bottom", "auto"},
	{"left", "auto"},
}

// subtreeRefusesToTurn walks what a box holds, looking for the reasons the turn
// would stop being exact.
//
// The walk is over boxes and not over fragments because it runs before layout:
// what it decides is how the box is laid out, so nothing has been laid out yet.
func (l *layouter) subtreeRefusesToTurn(root, b *Box) string {
	if b != root {
		if b.Float != FloatNone || b.Position != PositionStatic {
			return "it holds a floated or positioned box, whose sides are the page's"
		}
		if b.Replaced != nil || b.Control != nil || b.ListItem || b.MarkerImage != nil {
			return "it holds a replaced element, a form control or a list marker"
		}
		switch b.Inner {
		case InnerFlow, InnerText:
		default:
			return "it holds a table or another box with sizing rules of its own"
		}
		if writingModeOf(b) != verticalRL {
			return "it holds a box that changes the writing mode again"
		}
		// Only for a box an author wrote. An anonymous box and a text box have
		// no declarations of their own, and the style they carry is the one
		// they inherited from the box that does — so reading a width off the
		// text inside a div finds the div's, and refuses to turn every box that
		// declares its own size. Which is every box this can turn at all.
		if b.Element != nil {
			if why := l.refusesPhysicalGeometry(b); why != "" {
				return why
			}
		}
	}
	if b.IsText() && hasUprightText(b.Text) {
		// UAX #50. An ideograph stands upright on a vertical line and a rotated
		// page cannot produce one — see paragraph/vertical.go, and see
		// text-orientation below for the property that says the same thing.
		return "its text has characters in it that stand upright on a vertical line"
	}
	if orientation := trimmedLower(b.Style["text-orientation"]); orientation != "" &&
		orientation != "mixed" && orientation != "sideways" {
		// "mixed" and "sideways" agree wherever no character is upright, and
		// the clause above is what makes sure none is. "upright" and the rest
		// disagree with both.
		return "\"text-orientation: " + orientation + "\" asks for characters this engine cannot set on a vertical line"
	}
	if combine := trimmedLower(b.Style["text-combine-upright"]); combine != "" && combine != "none" {
		return "\"text-combine-upright: " + combine + "\" asks for a run set across the line, which this engine does not do"
	}
	for _, c := range b.Children {
		if why := l.subtreeRefusesToTurn(root, c); why != "" {
			return why
		}
	}
	return ""
}

// refusesPhysicalGeometry is the box-model half of the walk.
//
// The edges are asked of the resolvers rather than read out of the style, so
// that "border: 1px none" — a width with no border to draw — is the zero it
// really is. A percentage is the one thing they cannot answer here, because the
// containing block it would resolve against is the box being laid out, so a
// declaration with one in it is refused on sight.
func (l *layouter) refusesPhysicalGeometry(b *Box) string {
	for _, p := range physicalGeometry {
		switch v := trimmedLower(b.Style[p.name]); v {
		case "", p.initial:
		case "0px", "0%":
			// The same nothing, spelled the way a stylesheet spells it. Only
			// where zero *is* the initial value: "width: 0" is a declaration
			// that the box is not there, and is refused with the rest.
			if p.initial != "0" {
				return "\"" + p.name + "\" is declared inside it, and a side of the page is not a side of the text"
			}
		default:
			return "\"" + p.name + "\" is declared inside it, and a side of the page is not a side of the text"
		}
	}
	if l.edges(b, "margin", 0) != (Edges{}) || l.paddingOf(b, 0) != (Edges{}) ||
		l.borderWidths(b) != (Edges{}) {
		return "a margin, border or padding is declared inside it, and a side of the page is not a side of the text"
	}
	for _, prefix := range [...]string{"margin", "padding"} {
		for _, side := range [...]string{"-top", "-right", "-bottom", "-left"} {
			if strings.ContainsRune(b.Style[prefix+side], '%') {
				return "a percentage margin or padding is declared inside it, which is resolved against an axis the turn swaps"
			}
		}
	}
	return ""
}

func trimmedLower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
