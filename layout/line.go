package layout

import (
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// A finished line, as a backend reads it.
//
// These are the output of inline layout and the input to painting, which is why
// they carry things a line does not need in order to be broken: the box a run
// came from, for its colour; the decorations ruled across it, which belong to
// whichever ancestor declared them; the displacement §9.4.3 gave it.

// LineFragment is a line box: one row of text within a block.
type LineFragment struct {
	// Rect is the line box in the same coordinates as the fragment holding it.
	Rect Rect
	// Baseline is the distance from the top of the line box to the baseline the
	// text sits on. Painting needs it, and it is not derivable afterwards —
	// half-leading is split above and below the text.
	Baseline style.Unit
	// Runs are the pieces of text on the line, in *reading* order — the order
	// they were written, which on a line that mixes directions is not the order
	// they are drawn in. Where each one goes is its own X.
	//
	// The two are kept apart on purpose. The order here is the order the runs
	// reach the content stream, and so the order a reader extracting text from
	// the finished page gets them in; a right-to-left paragraph has to be drawn
	// the way it reads and copied out the way it was written, and only the X
	// decides the first.
	Runs []TextRun
	// Boxes are the fragments of the inline boxes that have a background or a
	// border on this line, in tree order — so an inner box's decoration is
	// painted over the box it is inside.
	//
	// They are fragments rather than a shape of their own because everything that
	// paints a background already works on one, and they hang from the line
	// rather than from the block's children for two reasons: Appendix E paints
	// them with the line's own content and not with the block backgrounds, and
	// one inline box produces one of them *per line* — see inlinepaint.go for
	// §8.6's slice model, which is the whole of why this is plural.
	//
	// A box with nothing to draw has none. The overwhelming majority of the
	// inline boxes in a document are an <em> or an <a> with no background and no
	// border, and a rectangle for each of them on each line would be work in
	// proportion to the document that nothing would ever read.
	Boxes []*Fragment
	// Sideways says this line runs down the page rather than across it: its
	// Baseline is measured leftwards from the line box's right edge, and each
	// run's X is a distance downwards from its top.
	//
	// It is on the line rather than on the block because it is what the painter
	// needs at the moment it turns a run's offsets into a point, and the line is
	// what it has in its hand there. See layout/writingmode.go for why a quarter
	// turn is expressible as one flag at all.
	Sideways bool
	// Anticlockwise says the quarter turn went the other way, which is what
	// "writing-mode: sideways-lr" asks for. It goes with Sideways rather than
	// instead of it: the line still runs along the page's vertical axis, and
	// what differs is which way. Its Baseline is measured rightwards from the
	// line box's *left* edge, because the glyphs' own up points left after this
	// turn, and each run's X is a distance upwards from the line box's foot.
	Anticlockwise bool
}

// TextRun is a piece of text on a line, set in one face at one size.
type TextRun struct {
	Text string
	// Face is what it is set in, and Size the font size.
	Face *shape.Face
	Size style.Unit
	// X is the offset from the left of the line box, and Width the advance.
	X, Width style.Unit
	// Box is the inline box the text came from, which carries the colour and
	// the decoration painting will need.
	Box *Box
	// Decorations are the lines ruled across this run: CSS 2.1 §16.3.1's
	// underline, overline and line-through. They are on the run rather than
	// derived from Box at paint time because a decoration belongs to whichever
	// *ancestor* declared it, and that box's colour is the line's colour — see
	// textdecoration.go, where the difference between propagating and inheriting
	// is worked through.
	Decorations []textDecoration
	// Features is what the document turned off: a font's own rules that a CSS
	// property or a CSS Text rule has overruled. It travels to the display list
	// because a backend shapes the run for itself. See shape.Features.
	Features shape.Features
	// Upright says each of the run's characters stands the way it does in the
	// code charts and the pen moves one em to the next one, rather than the run
	// being turned with the page. It is what "text-orientation: upright" asks
	// for, and it is a fact about the measurement as much as about the drawing.
	// See paragraph.Item.Upright.
	Upright bool
	// RTL says the run reads right to left, so its glyphs are drawn from the
	// right edge of its box towards the left and its brackets are mirrored.
	//
	// It is on the run rather than derived from the text because it is not a
	// property of the text: a run of punctuation between two Hebrew words is
	// right-to-left and has nothing in it that says so. The algorithm decided it
	// from the neighbours, which are other runs by the time anything paints this
	// one.
	RTL bool
	// PreContext and PostContext are the text either side of this run, where the
	// boundary between it and its neighbour did not break shaping.
	//
	// They are carried into painting for the reason LetterSpacing is: a backend
	// shapes this run from its text and its face, and a cursive letter's shape
	// comes from its neighbours. Without them the run is measured joined and
	// drawn isolated — the two disagree, and the page shows a word broken into
	// letters standing apart. See shapingcontext.go.
	PreContext, PostContext string
	// MergePre and MergePost say that side may contribute glyphs and not only
	// forms, so a ligature that spans the boundary is formed and drawn by
	// whichever run holds its first character. See paragraph.Item.MergePre.
	MergePre, MergePost string
	// ContextKerns says the neighbours above are set in this run's own face, so
	// a pair that spans the boundary is this font's pair. A font change is a
	// change in formatting and its pairs do not cross one; a letter's joined
	// shape does, because a character is the same character whichever font sets
	// it. See paragraph.Item.ContextKerns.
	ContextKerns bool
	// LetterSpacing is what letter-spacing added after each character of this
	// run. It is carried into painting as well as into the width because the two
	// have to agree: the width decided where the next run starts, and glyphs
	// drawn without the same spacing would leave a gap the size of the whole
	// run's spacing before it.
	LetterSpacing style.Unit
	// Offset is §9.4.3's relative displacement, accumulated over the inline
	// boxes this run sits inside.
	//
	// It is on the run rather than on a single fragment because a <span> that
	// spans a line break has one fragment per line: a line box holds runs, and
	// the box's own background and border are a Boxes entry on each of the lines
	// it reaches. Both are moved by the same displacement and are given it
	// separately — this one at paint time, the fragment's in absolutise, because
	// a background image is placed against the rectangle its box is drawn at.
	Offset Point
	// Shift is how far this run's own baseline sits below the line box's,
	// which is §10.8.1's vertical-align applied to the inline box the run came
	// from. It is negative for a run that is raised.
	//
	// It is a length on the run rather than a line of its own because a line
	// box has exactly one baseline — §10.8's alignment is *against* that
	// baseline — and every run on the line is placed relative to it. Folding it
	// into Offset would have been shorter and is wrong: Offset is §9.4.3's
	// relative positioning, which moves a box after layout without changing
	// anything about the line, whereas this displacement is part of what
	// decided the line's height.
	Shift style.Unit
}
