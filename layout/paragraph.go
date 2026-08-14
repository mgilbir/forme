package layout

import "github.com/mgilbir/forme/paragraph"

// The names this package knew the paragraph rules by, kept as aliases.
//
// The white space processing, the text transforms and the rest of what moved to
// forme/paragraph are called from several hundred places here, and rewriting
// each of them to say paragraph.CollapseWhitespace would be a large mechanical
// diff through code whose correctness rests on the reftest suite. The aliases
// cost nothing and leave that code exactly as it was verified — the same trade
// the shaping engine made when the bidirectional algorithm moved out of it.
//
// A name here is a name that crossed the boundary. Anything the layout engine
// kept — reading a property off a box, resolving it against a font, saying where
// a finding belongs — is still declared where it always was, because those are
// the questions that need a document.

// White space, from CSS Text §3 and §4.1.
type (
	whiteSpace   = paragraph.WhiteSpace
	wordBreak    = paragraph.WordBreak
	lineBreak    = paragraph.LineBreak
	overflowWrap = paragraph.OverflowWrap
)

var (
	whiteSpaceFor      = paragraph.WhiteSpaceFor
	whiteSpaceOf       = paragraph.WhiteSpaceOf
	wordBreakOf        = paragraph.WordBreakOf
	lineBreakOf        = paragraph.LineBreakOf
	overflowWrapOf     = paragraph.OverflowWrapOf
	collapseWhitespace = paragraph.CollapseWhitespace
	// The two rune predicates the line breaking shares with the collapsing.
	isOtherSpaceSeparator = paragraph.IsOtherSpaceSeparator
	separatorBreaksAfter  = paragraph.SeparatorBreaksAfter
)

// text-transform, from CSS Text §2.1.
type textTransform = paragraph.TextTransform

const (
	transformNone       = paragraph.TransformNone
	transformUppercase  = paragraph.TransformUppercase
	transformLowercase  = paragraph.TransformLowercase
	transformCapitalize = paragraph.TransformCapitalize
)

var (
	transformOf   = paragraph.TransformOf
	transformText = paragraph.TransformText
	endsInWord    = paragraph.EndsInWord
)

// Break opportunities and the pieces between them, from CSS Text §5.
type (
	piece       = paragraph.Piece
	textSpacing = paragraph.TextSpacing
)

var (
	splitAtBreaks      = paragraph.SplitAtBreaks
	tabAdvance         = paragraph.TabAdvance
	isIdeographic      = paragraph.IsIdeographic
	spacingAdvance     = paragraph.SpacingAdvance
	spacedUnits        = paragraph.SpacedUnits
	isDefaultIgnorable = paragraph.IsDefaultIgnorable
)
