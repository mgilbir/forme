package shape

import "github.com/mgilbir/forme/bidi"

// The bidirectional algorithm lives in package bidi, and these are the names
// this package knows it by.
//
// It was written here, and moved out because it is not about shaping: the
// algorithm is stated over Unicode character properties and has nothing to say
// about glyphs, so a caller laying out a paragraph needs it without needing any
// of this package. Keeping it unexported here is what caused it to be written a
// second time elsewhere.
//
// Aliases rather than a rewrite of the call sites. Shaping reads a character's
// class in several hundred places — the joining rules, the mark ordering, the
// Indic and Universal engines all ask it — and rewriting each of them to say
// bidi.NSM would be a large diff through code whose correctness rests on a
// comparison against HarfBuzz. The names below cost nothing and leave that code
// exactly as it was verified.
type (
	bidiClass = bidi.Class
	bidiRun   = bidi.Run
)

const (
	bidiL   = bidi.L
	bidiR   = bidi.R
	bidiAL  = bidi.AL
	bidiEN  = bidi.EN
	bidiES  = bidi.ES
	bidiET  = bidi.ET
	bidiAN  = bidi.AN
	bidiCS  = bidi.CS
	bidiNSM = bidi.NSM
	bidiBN  = bidi.BN
	bidiB   = bidi.B
	bidiS   = bidi.S
	bidiWS  = bidi.WS
	bidiON  = bidi.ON
	bidiLRE = bidi.LRE
	bidiRLE = bidi.RLE
	bidiLRO = bidi.LRO
	bidiRLO = bidi.RLO
	bidiPDF = bidi.PDF
	bidiLRI = bidi.LRI
	bidiRLI = bidi.RLI
	bidiFSI = bidi.FSI
	bidiPDI = bidi.PDI
)

var (
	bidiClassOf       = bidi.ClassOf
	bidiLogicalRuns   = bidi.LogicalRuns
	bidiVisualRuns    = bidi.VisualRuns
	bidiVisualOrder   = bidi.VisualOrder
	bidiRunCharacters = bidi.RunCharacters
)
