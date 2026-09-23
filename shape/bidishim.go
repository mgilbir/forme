package shape

import "github.com/mgilbir/forme/bidi"

// The bidirectional algorithm lives in package bidi, and these are the names
// this package calls it by.
//
// It was written here, and moved out because it is not about shaping: the
// algorithm is stated over Unicode character properties and has nothing to say
// about glyphs, so a caller laying out a paragraph needs it without needing any
// of this package. Keeping it unexported here is what caused it to be written a
// second time elsewhere.
//
// The four below are what shaping asks of it: the runs of a string in the order
// they are written and in the order they are drawn, the order of a set of runs,
// and a run's characters with rule L4's mirroring applied. There were twenty-six
// more — the class type and every class constant — under a note that shaping
// read a character's class in several hundred places; nothing read any of them.
var (
	bidiLogicalRuns   = bidi.LogicalRuns
	bidiVisualRuns    = bidi.VisualRuns
	bidiVisualOrder   = bidi.VisualOrder
	bidiRunCharacters = bidi.RunCharacters
)
