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
// What was the left side of the horizontal frame is the top of the page in both
// modes, because both run their text from the top downwards. Where they differ
// is the block axis, exactly as turnRect differs: vertical-rl stacks back from
// the right, so the horizontal top becomes the page's right, and vertical-lr
// stacks forwards from the left.
//
// The margins, borders and padding of a box inside a turned one are physical by
// the time anything paints them, and this is where they become physical.
func turnEdges(e Edges, mode writingMode) Edges {
	if mode == verticalLR {
		return Edges{Top: e.Left, Left: e.Top, Bottom: e.Right, Right: e.Bottom}
	}
	return Edges{Top: e.Left, Right: e.Top, Bottom: e.Right, Left: e.Bottom}
}

// untuneEdges is turnEdges backwards: the horizontal frame's sides that come
// out of the turn as the physical ones an author declared.
//
// It is what lets a box *inside* a turned one declare a margin at all. "margin-
// top" on such a box is the top of the page, which after the turn is the side
// the text starts at — so the horizontal engine has to be given it as a
// margin-*left*, and the turn puts it back where the author asked for it. The
// composition is exact because a quarter turn is a permutation of the four
// sides and this is its inverse: untuneEdges then turnEdges is the identity,
// which is what TestTurningTheEdgesBackAndForthChangesNothing asserts.
//
// Without it a margin inside a turned box was a refusal, and the refusal was
// wider than it looked: the user agent sheet gives every heading and every
// paragraph one, so a vertical box holding an <h1> was a vertical box this
// engine would not lay out.
func untuneEdges(e Edges, mode writingMode) Edges {
	if mode == verticalLR {
		return Edges{Left: e.Top, Top: e.Left, Right: e.Bottom, Bottom: e.Right}
	}
	return Edges{Left: e.Top, Top: e.Right, Right: e.Bottom, Bottom: e.Left}
}

// insideTurn is the writing mode of the nearest turned box strictly above this
// one, for the boxes whose declarations have to be read in that box's frame.
//
// Strictly above: the box that starts a turn keeps its own edges physical,
// because they are in its parent's frame and its parent is not turned. It is
// what it *holds* that is laid out sideways.
func (l *layouter) insideTurn(b *Box) (writingMode, bool) {
	if b == nil {
		return horizontalTB, false
	}
	for at := b.Parent; at != nil; at = at.Parent {
		if mode, turned := l.turnedMode[at]; turned {
			return mode, true
		}
	}
	return horizontalTB, false
}

// turnRect maps one rectangle of the horizontal frame onto the page.
//
// mirror is the physical width of the content box the rectangle is positioned
// inside, and it is what the two vertical modes differ by. Both run their text
// from the top downwards and turn their glyphs the same quarter turn clockwise;
// what one does and the other does not is stack its lines *back* from the right
// edge. So vertical-rl measures the block coordinate from the mirror and
// vertical-lr takes it as it stands.
//
// That is the whole of the difference, and it is worth saying why it is so
// small. The block-start side of a vertical-lr box is its left, so a reading of
// CSS Writing Modes §4.3 that follows "line-over is the block-start side"
// through to the glyphs would have this mode set its text the other way up.
// It does not: §5.1's "mixed" turns a rotatable character ninety degrees
// *clockwise* in both vertical modes, which is exactly why sideways-lr had to
// be added for the authors who want the other one. So the glyphs' own up is to
// the right in vertical-lr as it is in vertical-rl, and everything measured
// from a baseline — half-leading, ascent, §10.8.1's raise — is measured the
// same way in both. Only the lines stack the other way.
//
// The inline coordinate never takes a mirror. Text runs from the top downwards
// in both, which is the direction a horizontal line's own coordinate already
// runs in.
func turnRect(r Rect, mode writingMode, mirror style.Unit) Rect {
	x := r.Y
	if mode == verticalRL {
		x = mirror.Sub(r.Y).Sub(r.H)
	}
	return Rect{
		X: x,
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
func turnContent(f *Fragment, mode writingMode) {
	mirror := f.ContentRect().W
	for i := range f.Lines {
		turnLine(&f.Lines[i], mode, mirror)
	}
	for _, c := range f.Children {
		turnFragment(c, mode, mirror)
	}
}

func turnFragment(f *Fragment, mode writingMode, mirror style.Unit) {
	if f == nil {
		return
	}
	f.BorderRect = turnRect(f.BorderRect, mode, mirror)
	for i := range f.bgBands {
		f.bgBands[i] = turnRect(f.bgBands[i], mode, mirror)
	}
	f.Margin = turnEdges(f.Margin, mode)
	f.Border = turnEdges(f.Border, mode)
	f.Padding = turnEdges(f.Padding, mode)
	// contentH is a block-axis extent, which is a width on the page. It is read
	// only by the overflow arithmetic, which asks it of the box it belongs to
	// and never compares it across a turn, so it keeps its name and changes its
	// meaning here along with everything else.
	turnContent(f, mode)
}

// turnLine turns one line box and the inline boxes hanging off it.
//
// The runs on the line are left as they are. A run's X is its offset along the
// line, which is an inline coordinate in both frames, and where that offset ends
// up on the page is a question the painter answers from the line's Sideways flag
// — see painter.lines. Doing it here instead would mean turning a position that
// is not a rectangle and has no extent, which is exactly the arithmetic that
// belongs beside the baseline it is measured from.
func turnLine(l *LineFragment, mode writingMode, mirror style.Unit) {
	l.Rect = turnRect(l.Rect, mode, mirror)
	l.Sideways = true
	for _, ib := range l.Boxes {
		// An inline box's fragment is positioned in the block's content
		// coordinates, the same ones the line is, so it takes the same mirror.
		// Its own children are the boxes further in, which turnFragment reaches
		// through turnContent.
		ib.BorderRect = turnRect(ib.BorderRect, mode, mirror)
		for i := range ib.bgBands {
			ib.bgBands[i] = turnRect(ib.bgBands[i], mode, mirror)
		}
		ib.Margin = turnEdges(ib.Margin, mode)
		ib.Border = turnEdges(ib.Border, mode)
		ib.Padding = turnEdges(ib.Padding, mode)
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
func (l *layouter) turns(b *Box, containing style.Unit, hasHeight bool) writingMode {
	mode := writingModeOf(b)
	if mode == writingModeOf(b.Parent) {
		return horizontalTB
	}
	if mode == horizontalTB {
		// A box that turns *back*, inside a vertical ancestor. There is nothing
		// to report — horizontal-tb is the mode this engine lays out — and
		// nothing to do either: the ancestor it sits in was refused, because
		// refusesToTurn will not turn a box holding one of these, so the whole
		// subtree is already being laid out horizontally.
		return horizontalTB
	}
	if mode == verticalRL || mode == verticalLR {
		facing, why := l.refusesToTurn(b, mode, containing, hasHeight)
		if why == "" {
			l.turnedUpright[b] = facing == orientationUpright
			l.turnedMode[b] = mode
			return mode
		}
		l.reportWritingMode(b, mode, why)
		return horizontalTB
	}
	// The two sideways modes. They are the same quarter turn as these with the
	// upright characters taken out — "sideways-rl" is exactly what this file
	// does, for every character rather than only the rotatable ones — and
	// "sideways-lr" is the other quarter turn, which nothing here expresses.
	// Neither is laid out yet.
	l.reportWritingMode(b, mode, "only \"vertical-rl\" and \"vertical-lr\" are laid out")
	return horizontalTB
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
func (l *layouter) refusesToTurn(b *Box, mode writingMode, containing style.Unit, hasHeight bool) (textOrientation, string) {
	if b.Outer != OuterBlock || (b.Inner != InnerFlow && b.Inner != InnerFlowRoot) {
		// A table, an inline-block, a table part. Each has sizing rules of its
		// own that resolve the two axes together, and turning the result would
		// mean turning arithmetic this has not been through.
		//
		// A flow root is not one of those. It is ordinary block layout that
		// seals its own formatting context, which is the one thing a turned box
		// needs from the box it is — and it is what a float is, so refusing it
		// would refuse every floated vertical box in the suite.
		return orientationMixed, "it is not an ordinary block box"
	}
	if b.Position != PositionStatic || b.Replaced != nil {
		return orientationMixed, "it is positioned or replaced"
	}
	_ = hasHeight
	if _, declared := l.explicitWidth(b, containing); !declared && l.widthAskedOfTheContent(b) {
		// Shrink-to-fit measures the *horizontal* widths of the content, which
		// for a turned box are its inline extents and not the block axis its
		// width is. A box whose width is answered after its content is laid out
		// is fine — the block extent is known by then, and that is what an
		// automatic width is set from — but a box that has to be *measured*
		// before it is laid out is not.
		//
		// The box itself is one case, when it is a float. The other is a box
		// above it: a float wrapped round a vertical div came out as wide as
		// that div's text laid end to end, which is the length of its lines and
		// not the room its lines stack in.
		return orientationMixed, "its width would be measured along the wrong axis, " +
			"by shrinking a box around its content"
	}
	// Which of the two orientations the text actually needs, where the property
	// leaves it to the text. "mixed" is the initial value and the only one that
	// can ask for both on one line — and a page with both is the one thing a
	// quarter turn cannot draw. Where the text needs only one, it is the one
	// this engine sets.
	//
	// It matters because of what "text-transform: full-width" does. The suite's
	// text-transform-fullwidth-002 writes "Text sample" in a vertical box with
	// no orientation at all and asks for it upright, which is what mixed says
	// once the transform has turned every letter into a fullwidth form — and
	// the box tree carries the transformed text, so this is asked of what will
	// be drawn rather than of what was written.
	facing := orientationOf(b)
	if facing == orientationMixed {
		upright, rotated := l.subtreeOrientationMix(b)
		switch {
		case upright && rotated:
			return orientationMixed, "its text needs characters standing upright and " +
				"characters lying along the line at once"
		case upright:
			facing = orientationUpright
		default:
			facing = orientationSideways
		}
	}
	return facing, l.subtreeRefusesToTurn(b, mode, b)
}

// subtreeOrientationMix asks paragraph.OrientationMix of everything a box holds.
func (l *layouter) subtreeOrientationMix(b *Box) (upright, rotated bool) {
	if b.IsText() {
		return orientationMix(b.Text)
	}
	for _, c := range b.Children {
		u, r := l.subtreeOrientationMix(c)
		upright, rotated = upright || u, rotated || r
	}
	return upright, rotated
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
func (l *layouter) subtreeRefusesToTurn(root *Box, mode writingMode, b *Box) string {
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
		if writingModeOf(b) != mode {
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
	if orientation := trimmedLower(b.Style["text-orientation"]); orientation != "" &&
		orientationOf(b) != orientationOf(root) {
		// The subtree has to agree with the box the turn started at, because the
		// turn is one decision for the whole of it: one run set upright inside a
		// box whose lines are turned is a second typesetting mode on the same
		// line, and this file has one.
		//
		// "mixed" and "sideways" are the same answer here and are not told
		// apart. They differ only over the characters UAX #50 calls upright, and
		// the clause above has already refused a box that has one.
		return "\"text-orientation: " + orientation + "\" inside it is not the orientation the box is set in"
	}
	if combine := trimmedLower(b.Style["text-combine-upright"]); combine != "" && combine != "none" {
		return "\"text-combine-upright: " + combine + "\" asks for a run set across the line, which this engine does not do"
	}
	for _, c := range b.Children {
		if why := l.subtreeRefusesToTurn(root, mode, c); why != "" {
			return why
		}
	}
	return ""
}

// refusesPhysicalGeometry is the box-model half of the walk.
//
// The sizes are refused and the edges are not, and the difference is which of
// them the turn can carry. A margin is four numbers on four sides, and a
// permutation of the sides puts each of them back where the author asked for it
// — see untuneEdges, which is the permutation. A width is one number on one
// axis, and the axis the author named is the one the turn swaps: "width" inside
// a vertical box is the block axis, and the engine laying it out reads it as
// the length of a line.
//
// A percentage margin or padding is refused with the sizes rather than carried
// with the edges, and for the same reason: it resolves against the containing
// block's width, and which of the two axes that is depends on which way the
// containing block runs.
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

// uprightText reports whether the text of a box is set upright, standing the
// way it does in the code charts, rather than turned with the page.
//
// It is asked of the box the text is *in* and answered by the box the turn
// started at, which is somewhere above it — so the walk goes up until it meets a
// box that was turned. A box with no turned ancestor is on a horizontal page and
// its text is set the way it always was.
//
// Reading the style instead would be the obvious thing and would be wrong. A box
// may declare "text-orientation: upright" and not be turned at all, because the
// turn was refused for a reason that has nothing to do with orientation — an
// automatic width, a floated child — and the text of such a box is laid out
// across the page like any other. Measuring it a character to the em would make
// a horizontal line out of numbers that only mean anything on a vertical one.
func (l *layouter) uprightText(b *Box) bool {
	for at := b; at != nil; at = at.Parent {
		if upright, turned := l.turnedUpright[at]; turned {
			return upright
		}
	}
	return false
}

// textOrientation is which way the characters of a vertical line face.
type textOrientation uint8

const (
	// orientationMixed is the initial value: a character is turned with the
	// page where UAX #50 calls it rotatable and stands upright where it does
	// not. It is the only one of the three that can ask for both on one line,
	// which is why it is the only one the upright-character check applies to.
	orientationMixed textOrientation = iota
	// orientationSideways turns every character with the page, upright ones
	// included. That is exactly what this file's quarter turn does, so a box
	// asking for it needs no check at all.
	orientationSideways
	// orientationUpright stands every character the way it does in the code
	// charts and moves the pen one em to the next. It is not a rotation of
	// anything, and it is the reason paragraph.Item carries Upright.
	orientationUpright
)

// orientationOf reads CSS Writing Modes §5.1's property.
//
// "sideways-right" is the same value as "sideways" — it is the name the
// property had in css-writing-modes-3, kept as a legacy alias — and is read as
// one rather than reported, because a document writing it is asking for
// something this engine does.
func orientationOf(b *Box) textOrientation {
	switch trimmedLower(b.Style["text-orientation"]) {
	case "upright":
		return orientationUpright
	case "sideways", "sideways-right":
		return orientationSideways
	}
	return orientationMixed
}

// widthAskedOfTheContent reports whether this box's intrinsic width will be
// asked for — by the box itself if it shrinks to fit, or by an ancestor that
// does.
//
// It is asked only of a turned box with an automatic width, and it is asked
// because the answer would be wrong. The intrinsic pass measures a box's
// content the way a horizontal engine measures it, so for a turned box it
// returns the length of its lines; the width such a box wants is the room those
// lines stack in, which is known only once they have been broken. Nothing can
// answer it before the box is laid out, so a box that has to be measured before
// it is laid out is refused and reported.
//
// The walk stops at the first ancestor whose width does not come from its
// content, because from there down every width is already decided. An ancestor
// this cannot classify is treated as one that shrinks: refusing turns a page
// that would have been right into a page that says so, and the other way round
// is a page that is quietly wrong.
func (l *layouter) widthAskedOfTheContent(b *Box) bool {
	if shrinksToFit(b) {
		return true
	}
	for at := b.Parent; at != nil; at = at.Parent {
		if _, declared := l.explicitWidth(at, 0); declared {
			return false
		}
		if shrinksToFit(at) {
			return true
		}
		if at.Outer != OuterBlock || (at.Inner != InnerFlow && at.Inner != InnerFlowRoot) {
			return true
		}
	}
	return false
}

// shrinksToFit reports whether a box's own width is CSS 2.1 §10.3.5's
// shrink-to-fit: the narrower of what its content needs and what it is offered.
func shrinksToFit(b *Box) bool {
	return b.Float != FloatNone || b.Position.outOfFlow() ||
		b.Outer == OuterInline || b.Inner == InnerTableCell
}
