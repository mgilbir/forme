package shape

import (
	"sort"
	"unicode/utf8"

	"github.com/mgilbir/forme/bidi"
	"github.com/mgilbir/forme/internal/charprop"
)

// The shaped-glyph model.
//
// A span — a code and how far to move the pen before it — can say only one
// thing about a glyph: move horizontally. That is all kerning needs and all a
// left-to-right run of unmarked Latin needs, and it is not enough for anything
// else. An accent has to sit *over* the letter it belongs to — up and across by
// an amount the font states — and a span cannot say so.
//
// So positioning produces glyphs, not spans: a glyph index, where it goes
// relative to the pen, and how far the pen then moves. Every shaping entry
// point returns them; a caller that draws with spans takes the horizontal
// part, and has to know that it is discarding the rest.

// Glyph is one positioned glyph of a shaped run. Distances are in thousandths
// of an em, the unit the font's own metrics are in, so they are independent of
// the size the text is finally set at.
type Glyph struct {
	// GID is the glyph to draw.
	GID int

	// Cluster is the byte offset, in the input string, of the first character
	// of the cluster this glyph belongs to, and is HarfBuzz's cluster at its
	// default level. A cluster is at least a grapheme: a letter and its
	// accents, an emoji and its skin tone, a sequence joined by zero width
	// joiners, a pair of regional indicators. It grows wherever shaping joins
	// or moves glyphs: a ligature takes the clusters of what it joined, a
	// syllable of a script whose vowel signs are drawn before their consonant
	// is one cluster, and a character nothing is drawn for joins the cluster
	// beside it. Several glyphs may share a cluster and one glyph may stand for
	// several characters; the clusters of a run never go back in the text, in
	// logical order. It is what maps a position in the text to a position on
	// the page, for selection, search and hit-testing. See cluster.go.
	Cluster int

	// XAdvance is how far the pen moves after this glyph is drawn. It starts as
	// the font's own advance and is what kerning changes. A mark's is zero,
	// which is what makes it sit on the glyph before it rather than after.
	XAdvance float64

	// XOffset and YOffset displace the glyph from the pen without moving the
	// pen. This is how a mark is placed over its base.
	//
	// In a run set upright they displace it from where it is hung — see
	// VOriginX — rather than from the pen itself.
	XOffset, YOffset float64

	// YAdvance is how far the pen moves after this glyph along the page's
	// vertical axis, in the font's own orientation: up is positive, so a run
	// set down the page advances by a negative number, as HarfBuzz and a PDF
	// W2 array both state it. It is zero in a run set across the page, and
	// in a run set upright (Features.Vertical) it is the pen's only advance —
	// XAdvance is zero there. It starts as the glyph's vertical advance: vmtx's,
	// or where the face has none the height of its line. See vertical.go.
	YAdvance float64

	// VOriginX and VOriginY are, in a run set upright, where the glyph is hung
	// from: the point of the glyph, measured from its own horizontal origin,
	// that is put at the pen. Half the glyph's horizontal advance across, and
	// down from the top by what VORG or vmtx state, or the fallback HarfBuzz
	// takes where they state nothing. Both are zero in a run set across the
	// page.
	//
	// So an upright glyph's horizontal origin is drawn at the pen plus
	// (XOffset-VOriginX, YOffset-VOriginY); a format that places a glyph by
	// its vertical origin itself, as PDF's vertical writing does from W2, is
	// given the origin and the offsets separately.
	VOriginX, VOriginY float64

	// XAdjust and YAdjust are the part of XAdvance and YAdvance that shaping
	// changed, so that the two are always the font's own advance plus this:
	//
	//	XAdvance == nominal x advance + XAdjust
	//	YAdvance == nominal y advance + YAdjust
	//
	// The nominal advance is the one HarfBuzz starts a glyph's position from
	// before any lookup runs. In a run set across the page it is the glyph's
	// horizontal advance, hmtx's, with HVAR's variation where the face is
	// varied (an instance's hmtx already carries it, and Face.GlyphAdvance is
	// it), and the nominal y advance is zero. In a run set upright (see
	// Features.Vertical) it is the other way about: the nominal x advance is
	// zero and the nominal y advance is the vertical one, vmtx's with VVAR's
	// variation, or the height of the line where the face has none (see
	// Face.GlyphVerticalMetrics, which states it the same way YAdvance does).
	//
	// Everything after that is the adjustment, whichever mechanism made it: a
	// GPOS single, pair or cursive adjustment, the legacy kern table, kerning
	// across the edge of a run (ShapeGlyphsInContext), the width a face with no
	// glyph for a space separator gives the stand-in it uses, and the advance a
	// mark loses when it is made to move the pen not at all, which is the
	// mark's own advance negated: XAdjust is minus the nominal advance for a
	// glyph shaping cancelled the width of. A glyph a substitution made is
	// nominal again, whatever its predecessor had been adjusted by.
	//
	// It exists for a caller that must convert the two to a pixel grid by
	// different rules: an engine that has to reproduce the widths of a
	// HarfBuzz port rounds the nominal advance and truncates the adjustment,
	// and the sum cannot be rounded to the same result. Such a caller has
	// XAdvance and XAdjust, and the nominal advance is their difference, or is
	// Face.GlyphAdvance(GID) in a run set across the page.
	//
	// The identity is exact where the face's font units convert exactly to
	// these ones, which they do for a units-per-em of a thousand or any power
	// of two, and to within floating-point rounding for any other. It is a
	// statement about what shaping returned: a caller that changes XAdvance
	// afterwards, to add letter-spacing or to cancel a width, owns XAdjust
	// from then on. A Glyph that shaping did not make, as layout's math
	// stretch makes them, has zero here and no nominal advance to be measured
	// against.
	XAdjust, YAdjust float64

	// lig records this glyph's part in a ligature, and is unexported because it
	// is bookkeeping between the substitution pass and the positioning one
	// rather than anything a caller can use.
	lig ligatureRef

	// class is what the character this glyph came from says it is — a mark or a
	// letter — in GDEF's own numbering. It is what a lookup flag is read
	// against when the font classifies nothing itself; see layout.classOf.
	class int

	// mask is which of the masked features this glyph is for: the positional
	// form its letter takes in a cursive script, the part of an Indic syllable
	// it is, the fraction it stands in. See glyphMask, and plan.go for how a
	// lookup reads it.
	//
	// It is carried on the glyph rather than worked out when it is needed
	// because by then it cannot be: the substitutions that come first change how
	// many glyphs there are, so nothing maps back to the characters a form was
	// decided from. A glyph a substitution makes takes the mask of the glyph it
	// was made from. See arabic.go.
	mask glyphMask

	// substituted says a substitution has touched the glyph: replaced it,
	// made it from several, or taken it apart. It is HarfBuzz's SUBSTITUTED
	// glyph property, and what it decides is whether a character nothing is
	// drawn for is still one once the font has had its say — see
	// dropUnsubstituted.
	substituted bool

	// cont says the glyph continues the grapheme before it, as the
	// character it came from does: HarfBuzz's continuation bit, which
	// tracking reads (trak.go).
	cont bool

	// aatDeleted says a morx subtable deleted the glyph: it is taken out of
	// the run, its cluster merged, once the morx has run. See morx.go.
	aatDeleted bool

	// multiplied says the glyph is one of several a multiple substitution
	// made from one, and was not ligated since: HarfBuzz's MULTIPLIED glyph
	// property. A mark goes on the first of them and steps over the rest —
	// see acceptsMarks.
	multiplied bool

	// umark is what the character this glyph came from says about marks, for
	// the one reader that asks the character rather than the font: placing
	// the marks of a face that places none of its own. See fallback.go.
	umark unicodeMark

	// ignorable is what the character this glyph came from is to the font's
	// rules if it is one nothing is drawn for: stepped over unless a rule
	// names it, not stepped over, or a join control. See ignorable.go.
	ignorable ignorableKind

	// space says the glyph is the face's space standing in for a space
	// separator it has no glyph for, and which one, so that it can be given
	// that separator's width. See spacefallback.go.
	space spaceKind

	// stch says the glyph is a piece of a stretching mark, fixed or repeated,
	// and word that the character it came from is one a stretch spans. See
	// stch.go.
	stch uint8
	word bool
}

// The advance is only ever changed through these, so that XAdjust and YAdjust
// cannot fall behind it: each moves the adjustment by exactly what it moves the
// advance by, and a glyph that is nominal again says so with setNominal.

// addAdvance changes the advance by dx and dy: an adjustment.
func (g *Glyph) addAdvance(dx, dy float64) {
	g.XAdvance += dx
	g.XAdjust += dx
	g.YAdvance += dy
	g.YAdjust += dy
}

// addXAdvance and addYAdvance are addAdvance along one axis.
func (g *Glyph) addXAdvance(dx float64) { g.XAdvance += dx; g.XAdjust += dx }
func (g *Glyph) addYAdvance(dy float64) { g.YAdvance += dy; g.YAdjust += dy }

// setXAdvance and setYAdvance state the advance outright, as cursive
// attachment and mark cancellation do: the difference from what it was is the
// adjustment's.
func (g *Glyph) setXAdvance(x float64) { g.XAdjust += x - g.XAdvance; g.XAdvance = x }
func (g *Glyph) setYAdvance(y float64) { g.YAdjust += y - g.YAdvance; g.YAdvance = y }

// setNominalXAdvance gives a glyph the horizontal advance its font states for
// it, as a substitution does: whatever positioning had done to the glyph it
// replaced does not carry to this one.
func (g *Glyph) setNominalXAdvance(x float64) { g.XAdvance, g.XAdjust = x, 0 }

// ligatureRef says what a glyph has to do with a ligature.
//
// It exists because a mark inside a ligature has to be placed against the part
// of it the mark belongs to. A dot written under the first f of "ffi" and a dot
// written under the second are the same glyph attaching to the same glyph, and
// the font gives them different anchors; the only thing that tells them apart is
// which part of the text each came from, and that is what forming the ligature
// is the last moment to record.
type ligatureRef struct {
	// id is shared by a ligature glyph and every mark that was inside it. Zero
	// means the glyph has nothing to do with any ligature — which is also what
	// a "ligature" of a base and its own marks gets, since that is not a
	// ligature in the sense this is about.
	id int
	// comp is which part of the ligature this mark belongs to, counting from
	// one. It is zero on the ligature glyph itself.
	comp int
	// comps is how many parts this glyph counts as when it becomes part of a
	// larger ligature: one for an ordinary glyph, and its own component count
	// for a ligature that is then joined again.
	comps int
}

// ShapeGlyphs turns a string into positioned glyphs, applying everything this
// package reads: ligatures, contextual substitution, kerning, mark attachment,
// and the direction each part of the text runs in.
//
// The glyphs come back in *visual* order — the order the pen draws them, left to
// right — so a caller can draw them as they are, at a pen that only moves
// forward, whatever scripts the string mixes. That is not the order the string
// is written in: Hebrew and Arabic read the other way, and a PDF text-showing
// operator has no way to say so. Package bidi decides where each stretch
// belongs.
func (f *Face) ShapeGlyphs(s string) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil, shapeContext{})
}

// ShapeGlyphsInContext is ShapeGlyphs with the text either side of the run.
//
// A cursive script chooses each letter's shape from its neighbours, and a run is
// not always the whole word: CSS Text §8.1 says the boundary between two inline
// elements does not break shaping, so "\u0639<span>\u0639</span>\u0639" is one
// joined word set as three runs. Without the context each run is shaped alone
// and every letter comes out in its isolated form, which for a reader of Arabic
// is the difference between a word and three letters standing apart.
//
// The context decides the *forms*, and nothing else. A ligature that spans the
// boundary is not formed — a lam-alef written with the lam in one run and the
// alef in another stays two letters — because the glyph for it would belong to
// both runs at once and neither could carry it. That is a real limitation and
// the suite has a test of it, shaping_lig-000.
//
// Either side may be empty, which is what the start and end of a paragraph are.
func (f *Face) ShapeGlyphsInContext(s, before, after string, off Features) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil,
		shapeContext{before: before, after: after, kerns: true, features: off})
}

// ShapeGlyphsAcrossFaces is ShapeGlyphsInContext for a neighbour that is set in
// a *different* face.
//
// Which of its four shapes a letter takes is decided by the characters beside
// it, and a character is the same character whichever font sets it: Unicode's
// joining enforcement, and the suite's shaping-join-002 and
// shaping-tatweel-002 and -003, where a zero width joiner or a tatweel is
// pulled from another font by unicode-range and the Arabic letters either side
// must still take their joined forms.
//
// A kerning pair is not. It is stated by one font over two of its own glyphs,
// and a font change is a change in formatting: the pair across such a boundary
// is not this font's to apply. So the context reaches the joining scan and not
// the boundary kern.
func (f *Face) ShapeGlyphsAcrossFaces(s, before, after string, off Features) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil,
		shapeContext{before: before, after: after, features: off})
}

// ShapeGlyphsMerged is ShapeGlyphsInContext where a neighbour may contribute
// glyphs to this run and not only forms.
//
// §8.1's boundary "does not break shaping", and a ligature is shaping: "of<span
// >f</span>ice" is one word and the face's ffi is what a reader of it expects.
// The two named sides are shaped together with this run and the glyphs divided
// afterwards, by the cluster each came from — so the ligature belongs to
// whichever run holds its first character, and the other draws nothing for the
// characters it swallowed and takes none of its width.
//
// The sides are named separately from the context because the two questions have
// different answers. A form crosses a change of colour and a raised baseline —
// the suite's shaping-023 sets the middle Mongolian letter blue and asks for the
// three to join — and a *glyph* cannot: one glyph is drawn once, in one colour,
// on one baseline, so a ligature across such a boundary would paint half a word
// in the wrong colour. kerns is the same distinction drawn a third time and is
// left where it was.
func (f *Face) ShapeGlyphsMerged(s, before, after, mergeBefore, mergeAfter string,
	kerns bool, off Features) ([]Glyph, int) {

	return f.shapeGlyphsWith(s, nil, shapeContext{
		before: before, after: after,
		mergeBefore: mergeBefore, mergeAfter: mergeAfter,
		kerns: kerns, features: off,
	})
}

// shapeContext is the text either side of the run being shaped, in logical
// order: before is what precedes it and after is what follows.
//
// kerns says the neighbours are set in this run's own face, so a pair that
// spans the boundary is this font's pair. See ShapeGlyphsAcrossFaces for the
// case where they are not.
type shapeContext struct {
	before, after string
	// mergeBefore and mergeAfter are the text either side that may contribute
	// *glyphs* and not only forms: the run and they are shaped as one string
	// and the glyphs divided afterwards, so a ligature that spans the boundary
	// is formed. Each is the whole of its side of the group rather than the
	// neighbour alone — every run of a group has to shape the same string, or
	// two of them disagree about where a ligature begins and a character
	// belongs to neither. See shapeMerged.
	mergeBefore, mergeAfter string
	kerns                   bool
	// cutBefore and cutAfter say the text on that side is in another script:
	// the run is a piece of a string scriptRuns cut, and the side is where it
	// was cut. The text there is still context for the forms a letter takes —
	// the characters are beside each other whatever they are written in — but
	// a script change ends a run of the font's rules, so no pair is kerned
	// across it, as none is across the runs Stack.ShapeRuns cuts at the same
	// place. It is also what keeps the cut cheap: a boundary pair is found by
	// shaping the neighbour, and a Japanese sentence changes script every few
	// characters.
	cutBefore, cutAfter bool
	// features is what the document turned off. See Features, and note that it
	// travels with the context rather than beside it because it is the same
	// kind of fact: something about the run that its own text does not say.
	features Features
	// missed, where it is set, collects where each character the missing
	// count counts is: its byte offset in the string the collector shaped, of
	// which this string begins at. It is how shapeMerged gives a run the count
	// of its own characters out of the count for the whole it shaped. Only
	// shapeMerged sets it, and the string it shapes runs one way, so only the
	// cut by script (shapeDirection) moves at.
	missed *[]int
	at     int
	// keptBefore, keptAfter, dropped and justDropped say what the text
	// either side of this piece is, where the piece is part of a string cut
	// by direction or by script, as HarfBuzz, which shapes the string as one
	// buffer, sees it when it takes out a character nothing is drawn for
	// (delete_glyphs_inplace): whether any of the text before and after is
	// drawn; where none before is, how many bytes of it there are; and how
	// many bytes of characters nothing is drawn for stand immediately before
	// the piece. See clustersAfterCut.
	keptBefore, keptAfter bool
	dropped, justDropped  int
	// within is, where the cut fell inside a grapheme — a mark after a
	// character of the piece before, say — how many bytes back that grapheme
	// starts, so that the piece's first glyphs are of its cluster as
	// HarfBuzz's would be.
	within int
}

// keptAhead is whether anything of the piece's buffer is drawn before it on
// the page: the text before it, or for a run drawn right to left the text
// after it.
func (ctx shapeContext) keptAhead(rtl bool) bool {
	if rtl {
		return ctx.keptAfter
	}
	return ctx.keptBefore
}

// cutScan is what the pieces a string is cut into are told of the text
// either side of each, worked out once for the string: where its first and
// last characters that are drawn are, where the grapheme each character is
// in starts, and where each stretch of characters nothing is drawn for
// starts.
type cutScan struct {
	firstKept, lastKept int
	offsets             []int
	starts              []int
	dropFrom            []int
}

func newCutScan(s string) *cutScan {
	c := &cutScan{firstKept: len(s), lastKept: -1}
	var runes []rune
	for i, r := range s {
		runes, c.offsets = append(runes, r), append(c.offsets, i)
		if ignorableKindOf(r) == notIgnorable {
			if c.firstKept == len(s) {
				c.firstKept = i
			}
			c.lastKept = i
		}
	}
	cont := graphemeContinues(runes)
	c.starts = make([]int, len(runes))
	c.dropFrom = make([]int, len(runes))
	for k, r := range runes {
		c.starts[k] = c.offsets[k]
		if k > 0 && cont[k] {
			c.starts[k] = c.starts[k-1]
		}
		// A character nothing is drawn for that continues a grapheme shares
		// its cluster, which survives it, so it starts no stretch.
		c.dropFrom[k] = -1
		if ignorableKindOf(r) != notIgnorable && !cont[k] {
			c.dropFrom[k] = c.offsets[k]
			if k > 0 && c.dropFrom[k-1] >= 0 {
				c.dropFrom[k] = c.dropFrom[k-1]
			}
		}
	}
	return c
}

// context is ctx for the piece of the string from one byte offset to
// another: see shapeContext.keptBefore and shapeContext.within.
func (c *cutScan) context(ctx shapeContext, at, end int) shapeContext {
	if c.lastKept >= end {
		ctx.keptAfter = true
	}
	if at == 0 {
		return ctx
	}
	ctx.within, ctx.justDropped = 0, 0
	if k := sort.SearchInts(c.offsets, at); k < len(c.offsets) && c.offsets[k] == at {
		ctx.within = at - c.starts[k]
		if k > 0 && c.dropFrom[k-1] >= 0 {
			ctx.justDropped = at - c.dropFrom[k-1]
		}
	}
	if ctx.keptBefore {
		return ctx
	}
	if c.firstKept < at {
		ctx.keptBefore, ctx.dropped = true, 0
		return ctx
	}
	ctx.dropped += at
	return ctx
}

// clustersAfterCut is what HarfBuzz's taking out of the characters nothing
// is drawn for, before a piece in its string, does to the piece's clusters:
// the clusters of its first glyphs, its earliest in the text, merged back to
// where those characters start — where nothing before them is drawn; or, in
// a run drawn right to left, which HarfBuzz walks in the order it is drawn,
// where they stand immediately before the piece. buf is in the order the
// text is written.
func clustersAfterCut(buf []Glyph, ctx shapeContext, rtl bool) {
	back := ctx.dropped
	if ctx.keptBefore {
		back = 0
	}
	if rtl {
		back = ctx.justDropped
	}
	if back == 0 || len(buf) == 0 {
		return
	}
	first := buf[0].Cluster
	for i := 0; i < len(buf) && buf[i].Cluster == first; i++ {
		buf[i].Cluster = -back
	}
}

// runes returns the two sides as the shortest slices that still answer the
// question a joining scan asks of them.
//
// That scan walks outward past the transparent characters — the marks, which
// take no form of their own — until it meets one that is not, and then stops. So
// the useful context is everything up to and including the first non-transparent
// character on each side, and carrying more would be decoding characters whose
// answer is already settled.
//
// The side before is read from its end backwards, so that what it costs is
// what the scan uses and not the length of everything a caller put in front of
// the run.
func (c shapeContext) runes() (before, after []rune) {
	for s := c.before; s != ""; {
		r, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
		before = append(before, r)
		if joiningTypeOf(r) != joinT {
			break
		}
	}
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}
	for _, r := range c.after {
		after = append(after, r)
		if joiningTypeOf(r) != joinT {
			break
		}
	}
	return before, after
}

// contextRunes is how much of the text either side of a run is carried to it as
// context when a string is cut into runs by direction.
//
// Everything that reads a context reads an end of it: the joining scan walks
// out to the first character that is not transparent, and the pairs across a
// run's edge are looked up in boundaryWindow characters. So a bounded end is
// all that is needed, and carrying each whole side cost a copy of the string
// per run. Twice boundaryWindow leaves the joining scan room to step over the
// marks a letter carries, which HarfBuzz bounds at five characters of context
// in all; a letter with more than sixty marks between it and the edge of a run
// is joined as though they were the whole of its context.
const contextRunes = 2 * boundaryWindow

// contextBefore is the end of outer+inner that a run after them needs, built
// without copying either whole.
func contextBefore(outer, inner string) string {
	if tail := lastRunes(inner, contextRunes); len(tail) < len(inner) {
		return tail
	}
	return lastRunes(outer, contextRunes-utf8.RuneCountInString(inner)) + inner
}

// contextAfter is the start of inner+outer that a run before them needs.
func contextAfter(inner, outer string) string {
	if head := firstRunes(inner, contextRunes); len(head) < len(inner) {
		return head
	}
	return inner + firstRunes(outer, contextRunes-utf8.RuneCountInString(inner))
}

// ShapeGlyphsWith is ShapeGlyphs with extra features named by the caller: the
// optional ones a font offers and nothing turns on by itself, small capitals or
// oldstyle figures. A tag the font does not declare is ignored rather than
// refused, because a caller asking for small capitals of a face that has none
// wants the text, not an error.
func (f *Face) ShapeGlyphsWith(s string, features ...string) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, features, shapeContext{})
}

func (f *Face) shapeGlyphsWith(s string, extra []string, ctx shapeContext) ([]Glyph, int) {
	f.runWork.checkInput(f, s, extra, ctx)
	f.runWork.spend(int64(len(s)) + 1)
	if ctx.features.Vertical {
		// An upright run is not cut by direction: CSS Writing Modes §5.1 has
		// every character of it treated as strong left-to-right, and HarfBuzz
		// sets a top-to-bottom run in the order it is written, mirroring
		// nothing. See Features.Vertical.
		return f.shapeDirection(s, scriptBehind(ctx.before), scriptAhead(ctx.after), false, extra, ctx)
	}
	runs := bidiVisualRuns(s)
	if len(runs) <= 1 {
		// One direction throughout, which is nearly all text: shaped as one run
		// per script, which for nearly all of that is the string whole — and
		// whole keeps a ligature or a kern pair that spans it.
		rtl := len(runs) == 1 && runs[0].RTL()
		return f.shapeDirection(s, scriptBehind(ctx.before), scriptAhead(ctx.after), rtl, extra, ctx)
	}
	var (
		out     []Glyph
		missing int
	)
	// The script beside every run at once — see scriptsBeside for why not one
	// by one.
	pieces := make([][2]int, len(runs))
	for i, r := range runs {
		pieces[i] = [2]int{r.Start, r.End}
	}
	behind, ahead := scriptsBeside(s, pieces, scriptBehind(ctx.before), scriptAhead(ctx.after))
	scan := newCutScan(s)
	for i, r := range runs {
		piece := s[r.Start:r.End]
		// A run inside the string has the rest of the string for context, and
		// the caller's context outside that. The two are concatenated rather
		// than one replacing the other: the caller's is what comes before all of
		// s, so it belongs before the part of s that precedes this run.
		//
		// Replacing was the first version and was wrong in the one case that
		// matters. A right-to-left run reaches a backend as an override
		// character followed by the text — see ShapedText — so the text is never
		// the first run of the string, and the override alone stood in for the
		// word the letters were supposed to join to.
		//
		// Only the ends that touch the run are kept — see contextRunes. The
		// whole of each side was concatenated for every run, which copied the
		// string once per run: a run of digits every other character is a run
		// per two characters, and the copies were quadratic in the text.
		inner := shapeContext{
			before: contextBefore(ctx.before, s[:r.Start]),
			after:  contextAfter(s[r.End:], ctx.after),
			kerns:  ctx.kerns,
			// What the caller turned off is off for every run of the string.
			// It was dropped here, so a document that said "font-kerning: none"
			// got it for a Latin word and not for the same word beside a Hebrew
			// one — the same declaration, honoured or not by whether the
			// paragraph happened to change direction.
			features:   ctx.features,
			keptBefore: ctx.keptBefore,
			keptAfter:  ctx.keptAfter,
			dropped:    ctx.dropped,
		}
		inner = scan.context(inner, r.Start, r.End)
		// The sides that may contribute *glyphs* belong to the pieces they
		// touch: what precedes the whole string precedes its first run, and
		// what follows it follows its last. They were dropped as well, so a
		// ligature across an element boundary was formed for a run of one
		// direction and not for the same run beside text of the other.
		if r.Start == 0 {
			inner.mergeBefore = ctx.mergeBefore
		}
		if r.End == len(s) {
			inner.mergeAfter = ctx.mergeAfter
		}
		glyphs, gone := f.shapeDirection(piece, behind[i], ahead[i], r.RTL(), extra, inner)
		missing += gone
		for i := range glyphs {
			glyphs[i].Cluster += r.Start
		}
		f.runWork.size(len(out) + len(glyphs))
		out = append(out, glyphs...)
	}
	return out, missing
}

// shapeDirection shapes a string that runs one way throughout, as one run per
// script — see scriptRuns. behind and ahead are the scripts of the text either
// side of the string, for its characters that decide none.
//
// Nearly every string is in one script, and is shaped whole, as it always was.
// One that changes script is shaped a piece at a time, each piece with the rest
// of the string either side as its context, the way the bidi loop above gives
// each direction its neighbours: the forms a letter takes still see across the
// cut, and the pairs kerned do not. The pieces come back in the order they are
// drawn, which in a right-to-left run is the last piece first.
func (f *Face) shapeDirection(s string, behind, ahead uint16, rtl bool, extra []string, ctx shapeContext) ([]Glyph, int) {
	f.runWork.checkInput(f, s, extra, ctx)
	if !f.composite() {
		// A face set by character code has no rules to read per script, and
		// nothing to merge a neighbour's glyphs into.
		return f.shapeByCode(s, rtl, ctx.features.Vertical)
	}
	if ctx.mergeBefore != "" || ctx.mergeAfter != "" {
		return f.shapeMerged(s, rtl, extra, ctx)
	}
	var one [1]scriptRun
	pieces := scriptRuns(s, behind, ahead, one[:0])
	if len(pieces) == 1 {
		return f.shapeGlyphsIn(s, pieces[0].script, rtl, extra, ctx)
	}
	var (
		out     []Glyph
		missing int
	)
	scan := newCutScan(s)
	for k := range pieces {
		p := pieces[k]
		if rtl {
			p = pieces[len(pieces)-1-k]
		}
		inner := scan.context(ctx, p.start, p.end)
		inner.at = ctx.at + p.start
		if p.start > 0 {
			inner.before = contextBefore(ctx.before, s[:p.start])
			inner.cutBefore = true
		}
		if p.end < len(s) {
			inner.after = contextAfter(s[p.end:], ctx.after)
			inner.cutAfter = true
		}
		glyphs, gone := f.shapeGlyphsIn(s[p.start:p.end], p.script, rtl, extra, inner)
		missing += gone
		for i := range glyphs {
			glyphs[i].Cluster += p.start
		}
		f.runWork.size(len(out) + len(glyphs))
		out = append(out, glyphs...)
	}
	return out, missing
}

// shapeGlyphsIn is ShapeGlyphs with the run's script and direction already
// decided, and shapes one run rather than a whole string.
//
// A caller that split the text into runs knows more about a run's script than
// the run's own characters say: a stretch of digits between two Greek words is
// Greek, and shaping it as if it were scriptless would select the font's
// default rules where its Greek ones were meant. Stack.ShapeRuns made that
// decision when it cut the runs, and passes it here rather than having it
// guessed again from less. The same holds for direction, which is a property of
// the whole paragraph and cannot be read off one run of it.
func (f *Face) shapeGlyphsIn(s string, script uint16, rtl bool, extra []string, ctx shapeContext) ([]Glyph, int) {
	f.runWork.checkInput(f, s, extra, ctx)
	f.runWork.spend(int64(len(s)) + 1)
	if !f.composite() {
		return f.shapeByCode(s, rtl, ctx.features.Vertical)
	}
	// The face's own settings, which every run it shapes is asked with. This
	// is the one place every way of shaping a run passes through — the public
	// calls, a Stack's runs, the neighbour a boundary pair is found by — so it
	// is the one place they are put in. See Face.withSettings.
	ctx.features = f.withSettings(ctx.features)
	// Which model sets the run is decided by the script and by the tag the
	// font's rules for it were read under — see categorize — and it decides
	// everything below: how the characters are normalised, whether the ones
	// nothing is drawn for are taken out now, and what is done with the glyphs.
	lang := openTypeLanguage(ctx.features.Language)
	l := f.layoutFor(script, lang)
	chosen := f.chosenScriptTag(script, lang)
	vertical := ctx.features.Vertical
	model := categorize(script, chosen, vertical)
	// A face with a morx is set by it, as HarfBuzz and CoreText set it, in
	// a run across the page whatever else it has, and down the page where it
	// has no GSUB; and a script with a model of its own is set by the dumber
	// one, since the morx does what the model would. See morx.go.
	morx := f.morx != nil && (!vertical || len(f.layoutTables["GSUB"]) == 0)
	if morx && model != modelDefault {
		model = modelDumber
	}
	// Rule L4: a bracket in a right-to-left run is drawn as the bracket that
	// mirrors it, and the substitution is on the character, before the font is
	// asked for a glyph at all. Where the font has no glyph for the mirror the
	// character is kept, and its own glyph is what 'rtlm' is asked about.
	runes, offsets := bidiRunCharacters(s, rtl)
	if rtl {
		f.keepUnmirrorable(s, runes, offsets)
	}
	// The same substitution for a run set upright, in a face with no 'vert'
	// to give the vertical forms by glyph: the character's own vertical form,
	// where the face has one. See rotateForVertical.
	if vertical && !l.offersVert() {
		f.rotateForVertical(runes)
	}
	// Thai and Lao's one rearrangement of the text, which every shaper makes
	// whatever the font says, and for a Thai font with no Thai rules of its own
	// the older fonts' way of stacking marks. See thai.go.
	if model == modelThai {
		runes, offsets = thaiPreprocess(runes, offsets)
		if scriptSelects(script, "thai") && chosen != "thai" {
			runes = f.thaiPUAShape(runes)
		}
	}
	// Hangul's syllables into the spelling the face draws, and its tone marks
	// in front of them. See hangul.go.
	if model == modelHangul {
		runes, offsets = f.hangulPreprocess(runes, offsets)
	}
	// A vowel followed by a sign that spells another, against a dotted circle.
	// See markInvalidVowels.
	if model == modelIndic || model == modelUniversal {
		runes, offsets = markInvalidVowels(runes, offsets)
	}
	// Then normalisation, which is about the characters too and has to see the
	// mirrored ones: it puts the run into the spelling this face draws best and
	// each cluster's marks into canonical order. It runs before any glyph is
	// chosen because it decides which characters the font is asked about at all.
	// See normalize.go.
	runes, offsets = f.normalize(runes, offsets, normalization{
		syllabic: model.syllabic(), indic: model == modelIndic,
		arabic: model == modelArabic,
		hebrew: model == modelHebrew, hebrewForms: model == modelHebrew && !l.hasMarkFeature(),
		none: model == modelHangul,
	})
	// Which jamo feature each character is for, which a joiner between two
	// jamo decides by keeping them apart. See hangulFeatures.
	var jamo []uint8
	if model == modelHangul {
		jamo = hangulFeatures(runes)
	}
	// What each character nothing is drawn for is to the font's rules. They are
	// all kept, as HarfBuzz keeps them, until the substitutions have run: a
	// rule may name one, and some are not stepped over. See ignorable.go.
	f.runWork.size(len(runes))
	ignorables := ignorableKinds(runes)
	continues := graphemeContinues(runes)
	if len(runes) == 0 {
		return nil, 0
	}
	var (
		buf     = make([]Glyph, 0, len(runes))
		missing int
	)
	for i, r := range runes {
		gid, ok := f.GlyphID(r)
		var space spaceKind
		if !ok {
			// A space separator or a non-breaking hyphen the face draws
			// with a glyph it has; see spacefallback.go.
			gid, space, ok = f.standIn(r)
		}
		if !ok {
			// A character nothing draws is not one the face is missing.
			//
			// Every one of them reaches here, since the font's rules may name
			// them, and the shaper takes the ones no rule touched out again
			// before any pen sees the buffer: dropIgnorables on the general
			// path, the syllable model's own pass on the other. Counting them
			// was counting a glyph that was never going to be drawn.
			//
			// It decides which face sets a word. A caller's fallback asks "can
			// this face set the whole of this text" and reads the answer here,
			// and an emoji sequence is one grapheme cluster with a zero width
			// joiner inside it: a face holding every visible character of
			// "\U0001F468\u200D\U0001F4BB" reported one missing and was passed
			// over for a face holding none of them, which then set both emoji as
			// spaces.
			//
			// The Hangul fillers are not among these and still count: they are
			// default-ignorable and they are *drawn*, which is what
			// hiddenAfterShaping is the list of.
			//
			// Nor is a dotted circle the shaper put into the text, where the
			// face has none to draw it with: the text has no such character
			// to be missing. See markInvalidVowels.
			if !hiddenAfterShaping(r) && !isInsertedCircle(runes, offsets, i) {
				missing++
				if ctx.missed != nil {
					*ctx.missed = append(*ctx.missed, ctx.at+offsets[i])
				}
			}
			gid = 0
		}
		g := Glyph{
			GID: gid, Cluster: offsets[i], XAdvance: f.advanceGID(gid),
			class: classOfRune(runes[i]), umark: unicodeMarkOf(runes[i]),
			space: space, word: isStchWord(runes[i]), cont: continues[i],
		}
		if ignorables != nil {
			g.ignorable = ignorables[i]
		}
		f.runWork.size(len(buf) + 1)
		buf = append(buf, g)
	}
	if len(buf) == 0 {
		return nil, missing
	}
	// Each grapheme is one cluster from here on, a grapheme the run starts in
	// the middle of included. See cluster.go.
	formClusters(buf, func(i int) (bool, bool) {
		switch {
		case isInsertedCircle(runes, offsets, i):
			return true, true
		case model == modelHangul:
			return hangulJoins(runes, offsets, i)
		}
		return false, false
	})
	if ctx.within > 0 {
		first := buf[0].Cluster
		for i := 0; i < len(buf) && buf[i].Cluster == first; i++ {
			buf[i].Cluster = -ctx.within
		}
	}
	if model == modelIndic || model == modelUniversal {
		markVowelCircles(buf, runes, offsets)
	}
	// The run's script decides which of the font's rules apply, and everything
	// below reads the tables through it.
	sh := shaper{f: f, l: l, rtl: rtl, ligIDs: new(int), morx: morx, keptAhead: ctx.keptAhead(rtl),
		zeroMarks: model.zeroMarks(), features: ctx.features, lang: lang,
		ops: lookupBudget(len(buf))}
	// What the run applies, and in which stages: see plan.go. It covers every
	// entry point — the features a document turned off or asked for, and the
	// ones a caller named by tag, are requests to the same plan and not passes
	// of their own.
	p := sh.planFor(model, scriptSelects(script, "arab"), extra)
	// The glyphs particular features are for, where the plan has any: the
	// numerator and denominator around a fraction slash, and the glyphs whose
	// mirrored form is the font's to give.
	if p.fractions {
		maskFractions(buf, runes, rtl)
	}
	if p.rtlm {
		maskUnmirrored(buf, runes, offsets, s)
	}
	// A script whose characters are not in the order they are drawn is shaped
	// whole by its own pass: the reordering decides which of the font's rules
	// apply where, so it cannot be a step before the general substitutions and
	// has to be the substitutions. No script both joins cursively and reorders,
	// which is why these are alternatives rather than stages.
	before, after := ctx.runes()
	if morx {
		buf = sh.applyMorx(buf, rtl, vertical, ctx.features.requested(extra))
		buf = sh.dropIgnorables(buf)
	} else if model.syllabic() {
		buf = sh.shapeSyllabic(buf, runes, script, p, before, after)
	} else {
		// Which form each letter takes is decided now, while the glyphs still
		// correspond to the characters it is decided from, and recorded on the
		// glyphs so that it survives what follows.
		if model == modelArabic {
			markJoiningForms(buf, runes, before, after, scriptSelects(script, "mong"))
		}
		if model == modelHangul {
			markJamo(buf, runes, jamo)
			markToneCircles(buf, runes, offsets)
		}
		for i, stage := range p.stages {
			buf = sh.applyStage(buf, stage)
			if p.stch && i == p.stchAfter {
				recordStch(buf)
			}
			if p.arabicFallback != nil && i == p.arabicAfter {
				buf = sh.applyArabicFallback(buf, p.arabicFallback)
			}
		}
		// The characters nothing is drawn for have said all they have to say
		// once the substitutions are done. See ignorable.go.
		buf = sh.dropIgnorables(buf)
	}
	clustersAfterCut(buf, ctx, rtl)
	if model == modelHebrew {
		sh.gposScript = f.chosenPositioningTag(script, lang)
	}
	f.runWork.size(len(buf))
	sh.position(buf, p, model)
	// The pair that spans the boundary to the next run, which the pass above
	// cannot see because the glyph on the far side of it is not in this buffer.
	// See boundarykern.go. Not for a run set upright, which is not kerned:
	// the pairs are the 'kern' feature's, and a vertical run applies none.
	if len(sh.l.kern) > 0 && ctx.kerns && !ctx.features.kerningOff() && !vertical {
		kctx := ctx
		if ctx.cutBefore {
			kctx.before = ""
		}
		if ctx.cutAfter {
			kctx.after = ""
		}
		if kctx.before != "" || kctx.after != "" {
			before, after := f.boundaryGlyphs(kctx, script, rtl)
			sh.kernAcross(buf, before, after)
		}
	}
	if rtl {
		// Last, and only now. Everything above is stated by the font in terms of
		// the order the text is written in; the pen will meet these glyphs in the
		// other one.
		reverseGlyphs(buf)
	}
	// A stretching mark is stretched over its word once the word has its
	// widths, in the order it is drawn. See stch.go.
	if p.stch {
		buf = f.applyStch(buf, rtl)
	}
	if vertical {
		unhangFromOrigin(buf)
	}
	for _, g := range buf {
		f.used[g.GID] = true
	}
	return buf, missing
}

// keepUnmirrorable undoes rule L4 for a character whose mirror this face has no
// glyph for, so that the character is drawn as itself rather than as .notdef.
// It is what HarfBuzz does, and the character's own glyph is then what the
// font's 'rtlm' is asked about — see maskUnmirrored.
//
// runes is one to one with the characters of s at offsets, which is what
// bidiRunCharacters hands back, and is changed in place.
func (f *Face) keepUnmirrorable(s string, runes []rune, offsets []int) {
	for i, r := range runes {
		orig, _ := utf8.DecodeRuneInString(s[offsets[i]:])
		if r != orig && !f.hasGlyph(r) {
			runes[i] = orig
		}
	}
}

// maskUnmirrored marks, in a right-to-left run, every glyph Unicode's mirroring
// did not replace, as one 'rtlm' is for. That is every glyph but the mirrored
// ones: a character with no mirror, and one whose mirror the face could not
// draw, both leave the choice of a mirrored form to the font.
//
// buf is one to one with runes, and offsets are where in s each character
// is: a glyph's cluster is its grapheme's and not always its own character's.
func maskUnmirrored(buf []Glyph, runes []rune, offsets []int, s string) {
	for i := range buf {
		if i >= len(runes) || offsets[i] < 0 || offsets[i] >= len(s) {
			continue
		}
		orig, _ := utf8.DecodeRuneInString(s[offsets[i]:])
		if m, ok := bidi.MirrorOf(orig); ok && m == runes[i] && m != orig {
			continue
		}
		buf[i].mask |= maskRtlm
	}
}

// maskFractions marks the glyphs of each fraction written with U+2044 FRACTION
// SLASH between runs of decimal digits: the digits before it for the
// numerator, the ones after for the denominator, and all of it for 'frac'. It
// is HarfBuzz's automatic fractions, which it applies by default, so that
// "1\u20442" is set as a fraction by any font that has the forms.
//
// buf is one to one with runes. In a right-to-left run the two sides swap, as
// HarfBuzz swaps them: the run is in logical order and the numerator is still
// what was written first.
func maskFractions(buf []Glyph, runes []rune, rtl bool) {
	pre, post := maskNumr|maskFrac, maskFrac|maskDnom
	if rtl {
		pre, post = maskFrac|maskDnom, maskNumr|maskFrac
	}
	for i := 0; i < len(runes) && i < len(buf); i++ {
		if runes[i] != fractionSlash {
			continue
		}
		start, end := i, i+1
		for start > 0 && charprop.Is(runes[start-1], charprop.Nd) {
			start--
		}
		for end < len(runes) && end < len(buf) && charprop.Is(runes[end], charprop.Nd) {
			end++
		}
		if start == i || end == i+1 {
			continue
		}
		for j := start; j < i; j++ {
			buf[j].mask |= pre
		}
		buf[i].mask |= maskFrac
		for j := i + 1; j < end; j++ {
			buf[j].mask |= post
		}
		i = end - 1
	}
}

// fractionSlash is U+2044, the character that makes a fraction of the digits
// either side of it.
const fractionSlash = 0x2044

// shapeByCode is the shaping path for a face whose codes are characters rather
// than glyph indices: the fourteen standard faces, and any face embedded as a
// simple font.
//
// It applies no substitution and no positioning, and that is not a shortcut. A
// one-byte code addresses at most 256 glyphs, so a ligature the font has cannot
// generally be named at all; and every layout table is keyed by glyph index,
// which the code is not — looking a kern pair up by code finds either nothing
// or the wrong pair. What such a face can do correctly is one code per
// character at the width the font publishes, and that is what this does.
//
// Callers get the same Glyph values either way, so Draw, Measure and the
// fallback stack do not have to know which kind of face they were given — and
// a run set upright gets its vertical metrics here too, by the same rules and
// from the same tables where the face has them. See verticalRune.
func (f *Face) shapeByCode(s string, rtl, vertical bool) ([]Glyph, int) {
	runes, offsets := bidiRunCharacters(s, rtl)
	// A simple face draws nothing for these either. It is more visible here, if
	// anything: WinAnsi gives U+00AD a code of its own, so a soft hyphen without
	// this reaches the page as a hyphen — and a simple face has no shaping pass
	// later on that could take it back out.
	//
	// The join controls go too, which is what makes this the drawing predicate
	// and not the shaping one. There is nothing here for them to instruct, and
	// left in they take the substitution an unmapped character gets and reach the
	// page as a space.
	runes, offsets = dropHiddenBeforeDrawing(runes, offsets)
	var (
		buf     = make([]Glyph, 0, len(runes))
		missing int
	)
	var parts []rune
	// drew records what was drawn, as Encode records it. For a simple face
	// that is the glyph the character is drawn with, which is what its subset
	// has to keep: the codes in buf are not glyph indices, and they were
	// recorded as though they were — "A" is code 65, and the subset of Noto
	// Sans kept glyph 65 and not the glyph an A is drawn with. A standard
	// face has no program and no glyph indices, and its record is the codes it
	// set, which is what tells one document's use of it from another's.
	drew := func(r rune, code int) {
		if f.simple {
			f.used[f.prog.Cmap[r]] = true
			return
		}
		f.used[code] = true
	}
	for i, r := range runes {
		// What the face draws for it, which is its decomposition where the
		// face has that and not the character — as Measure and Encode say.
		var drawn bool
		if parts, drawn = f.drawnAs(r, 0, parts[:0]); drawn {
			for _, p := range parts {
				code, _ := f.GlyphID(p)
				width, _ := f.Advance(p)
				f.runWork.size(len(buf) + 1)
				buf = append(buf, f.byCode(code, offsets[i], width, p, vertical))
				drew(p, code)
			}
			continue
		}
		// The same substitution Measure and Encode make: see missingByCode.
		missing++
		code, width := f.missingByCode()
		f.runWork.size(len(buf) + 1)
		buf = append(buf, f.byCode(code, offsets[i], width, ' ', vertical))
		drew(' ', code)
	}
	if rtl {
		// There is nothing here for the direction to interfere with — no marks,
		// no joining, no kerning — but the run still has to come back in the
		// order it is drawn, so that a caller need not ask which kind of face it
		// was given.
		reverseGlyphs(buf)
	}
	return buf, missing
}

// byCode is one glyph of a face set by character code: the code, drawn for the
// character r, at the width the face publishes for it — or, in a run set
// upright, with the vertical metrics the face has for the character in its
// place. The metrics are asked by character because the code names no glyph
// the face's tables are indexed by. See verticalRune.
func (f *Face) byCode(code, cluster int, width float64, r rune, vertical bool) Glyph {
	g := Glyph{GID: code, Cluster: cluster, XAdvance: width}
	if vertical {
		advance, x, y := f.verticalRune(r)
		g.XAdvance, g.YAdvance = 0, -f.scale(advance)
		g.VOriginX, g.VOriginY = f.scale(x), f.scale(y)
	}
	return g
}

// MeasureGlyphs is the width a shaped run occupies at a given size, which is
// the sum of its advances — offsets displace glyphs without moving the pen and
// so contribute nothing.
func MeasureGlyphs(glyphs []Glyph, size float64) float64 {
	var total float64
	for _, g := range glyphs {
		total += g.XAdvance
	}
	return total * size / 1000
}

// reverseGlyphs puts a shaped run into visual order.
//
// It is the last step of shaping a right-to-left run and cannot be an earlier
// one. Everything before it — joining, ligatures, contextual rules, kerning,
// cursive attachment, marks — is stated by the font in terms of the order the
// text is written in, and applying any of it to a reversed buffer applies it to
// the wrong neighbours.
func reverseGlyphs(buf []Glyph) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}

// shapeMerged shapes a run together with the neighbours that may contribute
// glyphs to it, and keeps the glyphs that belong to the run.
//
// The whole is shaped once and cut by cluster: a glyph's Cluster is the byte
// offset of the first character it came from, so a ligature that swallows the
// start of this run has a cluster in the run before it and is left there. The
// run then draws nothing for those characters and takes none of their width,
// which is the arithmetic that makes the two halves add up to what one run
// would have measured.
//
// The sides that may *not* merge stay outside as ordinary context, so a run
// with a mergeable neighbour on one side and a plain one on the other still
// takes its forms from both.
//
// The whole is cut by script as any string is, and it is the whole that is
// cut and not the run: ShapeGroup shapes the same concatenation for the same
// group, with the same context outside it, and the two have to agree about
// which characters are one run of the font's rules or they disagree about the
// glyphs.
//
// It is reached only where something may merge, which is few runs of few
// documents; every other run takes shapeDirection's ordinary path.
func (f *Face) shapeMerged(s string, rtl bool, extra []string,
	ctx shapeContext) ([]Glyph, int) {

	pre, post := ctx.mergeBefore, ctx.mergeAfter
	// What is merged already carries the forms of that side, so the context
	// left outside is the other one's — and only where nothing merged there.
	var missed []int
	outer := shapeContext{kerns: ctx.kerns, features: ctx.features, missed: &missed}
	if pre == "" {
		outer.before = ctx.before
	}
	if post == "" {
		outer.after = ctx.after
	}
	glyphs, _ := f.shapeDirection(pre+s+post, scriptBehind(outer.before), scriptAhead(outer.after),
		rtl, extra, outer)
	lo, hi := len(pre), len(pre)+len(s)
	out := glyphs[:0:0]
	for _, g := range glyphs {
		if g.Cluster < lo || g.Cluster >= hi {
			continue
		}
		g.Cluster -= lo
		out = append(out, g)
	}
	// The count of characters no glyph was found for is the whole string's, and
	// this run is a part of it. Reporting the whole would have a run named for
	// its neighbour's missing characters as well as its own, so it is the ones
	// the shaping of the whole counted inside the run.
	//
	// It is the count the shaping made, and not the run's characters asked
	// again one by one. That was the first version, and it counted what the
	// shaping does not: a character nothing is drawn for (the override a
	// right-to-left run reaches a backend behind, a joiner), and a character
	// the face draws as its decomposition. The same run reported a different
	// count by whether it had a neighbour to merge with.
	missing := 0
	for _, at := range missed {
		if at >= lo && at < hi {
			missing++
		}
	}
	return out, missing
}

// ShapeGroup shapes a whole merge group — the runs that shape as one string,
// concatenated — so that every run of it can take its own slice of the result
// rather than shaping the group again for itself.
//
// The group and not the run is what is shaped, because two runs of one word
// that shape different strings disagree about where a ligature begins and a
// character between them is drawn by neither. That the group is the same string
// for every run of it is also what makes it memoizable: shaped once per run, a
// group of a thousand runs shapes a thousand characters a thousand times over.
//
// before and after are the context the group itself does not hold. See
// GroupContext, and GroupSpan for cutting one run out of the result.
func (f *Face) ShapeGroup(whole, before, after string, kerns bool, off Features) []Glyph {
	glyphs, _ := f.shapeGlyphsWith(whole, nil,
		shapeContext{before: before, after: after, kerns: kerns, features: off})
	return glyphs
}
