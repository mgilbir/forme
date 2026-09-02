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
	whiteSpaceFor           = paragraph.WhiteSpaceFor
	whiteSpaceOf            = paragraph.WhiteSpaceOf
	wordBreakOf             = paragraph.WordBreakOf
	lineBreakOf             = paragraph.LineBreakOf
	overflowWrapOf          = paragraph.OverflowWrapOf
	collapseWhitespace      = paragraph.CollapseWhitespace
	collapseWhitespaceAfter = paragraph.CollapseWhitespaceAfter
	boundaryAfter           = paragraph.BoundaryAfter
	endsCollapsedSpace      = paragraph.EndsCollapsedSpace
	transformFreezesSpace   = paragraph.FreezesSpace
	wordSpaceTransformOf    = paragraph.WordSpaceTransformOf
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
	hyphens     = paragraph.Hyphens
)

type hangingPunctuation = paragraph.HangingPunctuation

var (
	hangingPunctuationOf = paragraph.HangingPunctuationOf
	leadingHang          = paragraph.LeadingHang
	trailingHang         = paragraph.TrailingHang
)

var (
	hyphensOf                     = paragraph.HyphensOf
	hyphenatePieces               = paragraph.HyphenatePieces
	hyphenatesLanguage            = paragraph.HyphenatesLanguage
	trailingStopOrComma           = paragraph.TrailingStopOrComma
	autospaceOf                   = paragraph.AutospaceOf
	splitAtAutospace              = paragraph.SplitAtAutospace
	splitAtCursiveTracking        = paragraph.SplitAtCursiveTracking
	splitAtWordSeparators         = paragraph.SplitAtWordSeparators
	mayNotBeginLine               = paragraph.MayNotBeginLine
	splitAtBreaks                 = paragraph.SplitAtBreaks
	tabAdvance                    = paragraph.TabAdvance
	isIdeographic                 = paragraph.IsIdeographic
	spacingAdvance                = paragraph.SpacingAdvance
	spacedUnits                   = paragraph.SpacedUnits
	spacingAfter                  = paragraph.SpacingAfter
	uprightUnits                  = paragraph.UprightUnits
	orientationMix                = paragraph.OrientationMix
	isCursiveScript               = paragraph.IsCursiveScript
	cursiveTrackingSuppressesText = paragraph.CursiveTrackingSuppresses
	isDefaultIgnorable            = paragraph.IsDefaultIgnorable
	needsPhraseBreaking           = paragraph.NeedsPhraseBreaking
)

// Geometry and the item type itself.
type (
	Point       = paragraph.Point
	breaker     = paragraph.Breaker
	strut       = paragraph.Strut
	vAlignState = paragraph.VAlignState

	// textDecoration is one line ruled across a run, together with the box that
	// asked for it. It travels with the item and the breaking never reads it.
	textDecoration = paragraph.Decoration
)

var newBreaker = paragraph.NewBreaker

// The item itself, and the paragraphs the bidirectional algorithm resolves over.
type (
	inlineItem  = paragraph.Item
	bidiBuilder = paragraph.BidiBuilder
	bidiMode    = paragraph.BidiMode
)

// wordSpaceTransform is what the property of that name sets: which character a
// virtual word separator becomes, or nothing at all.
type wordSpaceTransform = paragraph.WordSpaceTransform

// textBoundary is what §4.1.1's segment break rules need to know about the text
// a node follows. See paragraph.Boundary.
type textBoundary = paragraph.Boundary

// What the walk over an inline subtree carries, and what a line is made of.
type (
	itemRef     = paragraph.Ref
	inlineFrame = paragraph.Frame
	inlineState = paragraph.State
	vAlign      = paragraph.VAlign
)

// The rest of what crossed: the stacking, the ordering and the small
// predicates the two halves share.

type (
	decorationKind = paragraph.DecorationKind
	lineStack      = paragraph.LineStack
	midLineBox     = paragraph.MidLineBox
)

const (
	bidiNormal          = paragraph.BidiNormal
	decorationUnderline = paragraph.DecorationUnderline
	runeFSI             = paragraph.RuneFSI
	runeLRE             = paragraph.RuneLRE
	runeLRI             = paragraph.RuneLRI
	runeLRO             = paragraph.RuneLRO
	runePDF             = paragraph.RunePDF
	runePDI             = paragraph.RunePDI
	runeRLE             = paragraph.RuneRLE
	runeRLI             = paragraph.RuneRLI
	runeRLO             = paragraph.RuneRLO
	vAlignBaseline      = paragraph.VAlignBaseline
)

var (
	cursorAdvanced     = paragraph.CursorAdvanced
	describeRune       = paragraph.DescribeRune
	fmtPx              = paragraph.FmtPx
	lineCap            = paragraph.LineCap
	lineMetrics        = paragraph.LineMetrics
	lineVisualOrder    = paragraph.LineVisualOrder
	marksNoPaper       = paragraph.MarksNoPaper
	substitutesExactly = paragraph.SubstitutesExactly
	missesVisible      = paragraph.MissesVisible
	newBidiBuilder     = paragraph.NewBidiBuilder
	parseNumber        = paragraph.ParseNumber
	positiveInteger    = paragraph.PositiveInteger
	sameUnits          = paragraph.SameUnits
	stackLine          = paragraph.StackLine
	startOfContext     = paragraph.StartOfContext
	unsupportedScript  = paragraph.UnsupportedScript
)

const (
	vAlignTop             = paragraph.VAlignTop
	vAlignBottom          = paragraph.VAlignBottom
	vAlignMiddle          = paragraph.VAlignMiddle
	vAlignTextTop         = paragraph.VAlignTextTop
	vAlignTextBottom      = paragraph.VAlignTextBottom
	bidiEmbed             = paragraph.BidiEmbed
	bidiIsolate           = paragraph.BidiIsolate
	bidiOverride          = paragraph.BidiOverride
	bidiIsolateOverride   = paragraph.BidiIsolateOverride
	bidiPlaintext         = paragraph.BidiPlaintext
	decorationOverline    = paragraph.DecorationOverline
	decorationLineThrough = paragraph.DecorationLineThrough
)

// The bounds and the one string the two halves share. MaxLineFits and
// MaxBalancePasses are vars rather than consts because tests lower them to reach
// the truncation they guard, which is a thing this package's tests do.
const (
	blockEllipsis                  = paragraph.BlockEllipsis
	normalLineHeightFallbackFactor = paragraph.NormalLineHeightFallbackFactor
)
