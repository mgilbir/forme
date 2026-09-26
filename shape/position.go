package shape

import (
	"math/bits"

	"github.com/mgilbir/forme/font"
)

// Positioning: kerning, single adjustments, joining glyphs to each other, and
// attaching marks to what they belong to.
//
// Mark attachment is the piece the span model could not express, and the reason
// this package now shapes into glyphs. A font states, for each mark and each
// base, an *anchor* — a point in the glyph's own space — and attaching them
// means placing the mark so that its anchor coincides with the base's. That is
// two coordinates the font supplies and one subtraction, and without it an
// accent is drawn at its nominal advance: for most fonts, at the origin, so
// every accent in a run piles up in the same place.
//
// # The lookups in the order the font lists them
//
// A font's positioning is a list of lookups, and a shaper applies each of them
// over the whole run in the order of that list: a kerning lookup, then a mark
// lookup, then a contextual one that moves a mark the earlier one placed, then
// a single adjustment that nudges it again. Each sees what the ones before it
// did. The font was tested against exactly that, because HarfBuzz does exactly
// that.
//
// This package used to read the lookups into flat tables at load — the pairs,
// the anchors, the single adjustments merged by glyph — and apply the tables in
// passes of its own choosing: kerning, contextual rules, single adjustments,
// cursive attachment, marks. The passes agreed with the list wherever a font's
// lookups happened to be in that order, and a font is under no obligation to
// be. Over the Google Fonts tree they disagreed in three ways, all found by
// asking HarfBuzz which lookup moved what:
//
//   - A contextual rule that moves a mark after a mark lookup placed it. Arimo,
//     Tinos and Cousine place the Hebrew points with one lookup and correct
//     some of them with a chained rule after it; Mukta attaches its reph and a
//     rule then moves it by fifteen units. The passes ran every rule first and
//     every mark after, so the mark lookup undid the correction.
//   - A single adjustment after the mark it adjusts is attached. Noto Sans
//     Gujarati UI attaches a vowel sign below and lowers it by twenty units in
//     'dist'; attaching *sets* the mark's offset, and the pass that attached
//     last threw the adjustment away.
//   - Single adjustments merged by glyph, so that of two lookups adjusting one
//     glyph only the later counted, where both add up.
//
// So the lookups are applied as they are: the plan gathers every enabled
// feature's positioning lookups into one list in index order (plan.compile),
// and each is applied over the run, subtable by subtable, the first that
// applies at a glyph winning. What each type does at a glyph is in
// contextpos.go.
//
// # Attachment is a chain, resolved last
//
// A mark lookup does not put a mark where its base is; it records which glyph
// the mark hangs from and where relative to it, because a later lookup may move
// the base. HarfBuzz settles the two axes at different times — the height when
// the mark is attached, from where its base stands then, and the position
// along the line only at the very end, from where the base finally is — and a
// font is tested against that. Noto Serif Tibetan needs both: lookup 20 stacks
// a subjoined ra on the mark before it, and lookup 21 then moves that mark onto
// another; the ra keeps its height and follows the mark along. Cursive
// attachment is a chain of the same kind across the line. See propagate.

// zeroMarkWidths says when a mark's own advance is cancelled.
//
// A mark is drawn on the letter before it and must not move the pen, and a font
// gives its marks an advance of zero — usually. What differs between scripts is
// *when* a shaper insists on it, and it is not a detail: a positioning rule may
// give a mark an advance on purpose, and whether that survives depends on
// whether the cancelling happens before the rules or after them.
//
// The specification for the universal engine says why a font would: a base glyph
// classified as a mark, so that contextual rules can skip it, has its width put
// back with 'dist' — "necessary because OpenType processing cancels the width
// associated with a mark". Cancelling afterwards would take it away again.
//
// Which a run gets is its model's choice (shaperModel.zeroMarks), not its
// script's: a Devanagari run set by the default model, because its font states
// its rules under 'DFLT', cancels late as the default model does.
type zeroMarkWidths uint8

const (
	// Never: the font is trusted to have given its marks no width, and anything
	// a rule states about one stands. Indic, Khmer and Hangul.
	zeroMarksNone zeroMarkWidths = iota
	// Before the rules run, so that what they state about a mark survives. The
	// universal engine and Myanmar.
	zeroMarksEarly
	// After the rules run, discarding whatever they said about a mark's advance.
	// Arabic, Hebrew, Thai and every script with no model of its own.
	zeroMarksLate
)

// cancelMarkWidths takes the advance off every mark, and with adjustOffsets
// moves the offset with it, so that the mark is drawn where it would have been
// and only the pen stops moving. Both advances, as HarfBuzz cancels both: a
// run set upright advances down the page.
func (sh shaper) cancelMarkWidths(buf []Glyph, adjustOffsets bool) {
	for i := range buf {
		if !sh.l.isMark(buf[i]) {
			continue
		}
		if adjustOffsets {
			buf[i].XOffset -= buf[i].XAdvance
			buf[i].YOffset -= buf[i].YAdvance
		}
		buf[i].XAdvance, buf[i].YAdvance = 0, 0
	}
}

// hasPositioning reports whether the face has a GPOS table at all, whatever it
// selects for the run: HarfBuzz's hb_ot_layout_has_positioning.
func (f *Face) hasPositioning() bool {
	t := f.layoutTables["GPOS"]
	return len(t) >= 4 && (t[0]|t[1]|t[2]|t[3]) != 0
}

// positioning is what a run's positioning pass does, decided from the face,
// the plan and the model before any glyph is moved. It is the half of
// HarfBuzz's plan compilation that decides who positions: GPOS, the legacy
// kern table, or the model's own fallback.
type positioning struct {
	// gpos says the plan's positioning lookups are applied at all.
	gpos bool
	// kern says the legacy kern table is.
	kern bool
	// zero says the model cancels mark advances, at the moment the shaper's
	// zeroMarks says.
	zero bool
	// adjust says a cancelled advance moves the mark's offset with it: see
	// positioningFor.
	adjust bool
	// fallback says the model places the marks itself. See fallback.go.
	fallback bool
}

// positioningFor decides who positions a run.
//
// # GPOS, unless the model refuses the font's
//
// A face with a GPOS table is positioned by it, whatever it selects for the
// run — a font that states positioning for other scripts and none for this one
// has still said how it wants to be positioned. The Hebrew model is the one
// exception: it applies GPOS only where the font's positioning was read under
// 'hebr' (HarfBuzz's issue 347), because a face with Latin kerning and no
// Hebrew positioning does not place its points, and applying its Latin lookups
// to Hebrew places nothing while stopping the model from placing them.
//
// # The legacy kern table
//
// Where GPOS offers the run no 'kern' feature, the kern table is applied in its
// place — beside GPOS, if there is one — by the models that position a font
// with no positioning of their own. See legacykern.go.
//
// # Moving a mark with its cancelled advance
//
// Only a font that places no marks of its own is given that. Where the font has
// GPOS, it positions its marks relative to where the pen is once their advances
// are gone, and a mark shifted as well is drawn a whole advance short of where
// the font put it: Padauk's medial ra, 219 units left of its letter. A font
// with a legacy kern table that kerns across the line — which moves glyphs off
// the baseline — is taken to have placed them too. And right to left the mark
// is not moved: the pen meets it before its base, and it hangs over the glyph
// after it once the run is reversed. That is HarfBuzz's
// adjust_mark_positioning_when_zeroing.
//
// # Placing marks without the font
//
// A model that has a fallback — the default, Arabic, Hebrew and Hangul — places
// the marks of a face that does not, by their combining classes, in exactly the
// case above: nothing of the font's own positions them.
func (sh shaper) positioningFor(p *plan, model shaperModel) positioning {
	var out positioning
	out.gpos = sh.f.hasPositioning() && !(model == modelHebrew && sh.gposScript != "hebr")
	lk := &sh.l.legacyKern
	out.kern = lk.present() && !(p.gposKern && out.gpos) && model.fallbackPosition() &&
		!sh.features.kerningOff()
	out.zero = sh.zeroMarks != zeroMarksNone && (!out.kern || !lk.stateMachine)
	out.adjust = !out.gpos && (!out.kern || !lk.crossStream)
	out.fallback = out.adjust && model.fallbackPosition()
	if sh.rtl {
		out.adjust = false
	}
	return out
}

// fallbackPosition is HarfBuzz's fallback_position: whether the model applies
// the legacy kern table and places the marks of a face that positions nothing
// itself. The syllabic models and Thai do neither — their fonts are expected
// to state their positioning, and a placement by combining class would put a
// Devanagari vowel sign somewhere no font meant it.
func (m shaperModel) fallbackPosition() bool {
	switch m {
	case modelDefault, modelArabic, modelHebrew, modelHangul:
		return true
	}
	return false
}

// Attachment kinds, as HarfBuzz records them on a glyph that hangs from
// another.
const (
	attachMark    = 1
	attachCursive = 2
)

// gposPass is what one positioning pass keeps beside the buffer: for each
// glyph, which glyph it hangs from and how, and the cache mark-to-base uses to
// find a mark's base without walking back over a run of marks for each. It is
// shared by every lookup of the pass and every lookup a
// contextual rule reaches from one, which is why the shaper holds it by
// pointer.
type gposPass struct {
	// chain is, for each glyph, the distance to the glyph it is attached to —
	// negative for one before it — or zero; kind says whether that is a mark's
	// attachment or a cursive joint.
	chain []int
	kind  []uint8
	// lastBase and lastBaseUntil are HarfBuzz's cache for mark-to-base and
	// mark-to-ligature: the base the last search found, and how far back the
	// searches have already looked. A letter with a long run of marks is
	// searched from each mark back to the previous mark and no further, so the
	// search costs the run once rather than once per mark. Reset at the start
	// of every lookup; see markBaseFor.
	lastBase, lastBaseUntil int
}

// position runs the run's positioning: the plan's GPOS lookups, or the legacy
// kern table, or the model's fallback, as positioningFor decides; then every
// attachment is resolved against where its target finally is.
func (sh shaper) position(buf []Glyph, p *plan, model shaperModel) {
	how := sh.positioningFor(p, model)
	vertical := sh.features.Vertical
	// A run set upright advances down the page and hangs each glyph from its
	// vertical origin, before anything moves it. See vertical.go.
	if vertical {
		sh.f.setVertical(buf)
	}
	// The widths of the spaces the face's own space stands in for come first,
	// as HarfBuzz sets them with the font's own advances: every rule below
	// adjusts the width the separator has. See spacefallback.go.
	sh.f.setStandInSpaces(buf, vertical)
	pass := &gposPass{chain: make([]int, len(buf)), kind: make([]uint8, len(buf))}
	sh.gp = pass
	if how.zero && sh.zeroMarks == zeroMarksEarly {
		sh.cancelMarkWidths(buf, how.adjust)
	}
	if how.gpos {
		for _, lk := range p.gpos {
			sh.applyPositioningLookup(lk, buf)
		}
	}
	// The kern table's pairs are read from its horizontal subtables alone, and
	// HarfBuzz applies none of those to a run set upright.
	if how.kern && !vertical {
		sh.applyLegacyKern(buf)
	}
	if how.zero && sh.zeroMarks == zeroMarksLate {
		sh.cancelMarkWidths(buf, how.adjust)
	}
	// Last, once every advance is what it will be: each glyph that hangs from
	// another is put where that one is.
	sh.propagate(buf)
	if how.fallback {
		sh.fallbackMarkPositions(buf, how.adjust)
	}
}

// applyPositioningLookup applies one lookup over the whole run, from the
// first glyph to the last: at each glyph the lookup is for and does not step
// over, its subtables are tried in order and the first that applies decides
// where the walk goes next. HarfBuzz's apply_forward.
func (sh shaper) applyPositioningLookup(pl planLookup, buf []Glyph) {
	if pl.index < 0 || pl.index >= len(sh.l.gpos) {
		return
	}
	lk := sh.l.gpos[pl.index]
	sh.lookupMask = pl.mask
	sh.manualZWJ, sh.manualZWNJ = pl.manualZWJ, pl.manualZWNJ
	sh.gp.lastBase, sh.gp.lastBaseUntil = -1, 0
	for i := 0; i < len(buf); {
		sh.markSet = lk.markSet
		if !sh.maskAllows(buf[i]) || sh.ignores(lk.flags, buf[i]) {
			i++
			continue
		}
		if n := sh.applyGPOSAt(pl.index, buf, i, 0); n > 0 {
			i += n
			continue
		}
		i++
	}
}

// propagate puts each glyph that hangs from another where that one finally
// is. It is HarfBuzz's propagate_attachment_offsets.
//
// A mark keeps the height it was given when it was attached and takes its
// target's offset along the line, less the advances the pen makes between the
// two — which are final only now: a mark between a base and the mark stacked
// on it may have had its advance cancelled since, and the stacked one is drawn
// against a pen that did not move for it. A glyph cursively attached takes its
// target's height, which is how a joined word climbs onto its strokes. In a
// run set upright the line runs down the page, so "along" is the vertical
// offset and "height" the horizontal one, as HarfBuzz swaps them.
//
// A target is resolved before anything that hangs from it, following the
// chain as far as HarfBuzz follows it. The walk goes in the direction the run
// is written — backwards for a right-to-left one, as HarfBuzz walks it since
// its issue 5514 — so a cursive chain, whose root is at the end a
// right-to-left lookup anchors, is resolved from its root.
func (sh shaper) propagate(buf []Glyph) {
	g := sh.gp
	any := false
	for _, c := range g.chain {
		if c != 0 {
			any = true
			break
		}
	}
	if !any {
		return
	}
	// A letter carrying a long run of marks moves each of them back over the
	// marks before it, and that stretch was read once per mark: "a" with
	// sixteen thousand U+0301 after it climbed by 3.7 per doubling. A prefix
	// sum answers each in constant time. It sums the advances along the line,
	// which for a run set upright are the vertical ones.
	sums := advanceSums(buf, sh.features.Vertical)
	if !sh.rtl {
		for i := range buf {
			if g.chain[i] != 0 {
				sh.propagateAt(buf, sums, i, maxAttachmentNesting)
			}
		}
		return
	}
	for i := len(buf) - 1; i >= 0; i-- {
		if g.chain[i] != 0 {
			sh.propagateAt(buf, sums, i, maxAttachmentNesting)
		}
	}
}

// maxAttachmentNesting is how far a chain of attachments is followed, which is
// HarfBuzz's HB_MAX_NESTING_LEVEL. A chain longer than that is a font asking
// for it; where it is cut is where HarfBuzz cuts it.
const maxAttachmentNesting = 64

func (sh shaper) propagateAt(buf []Glyph, sums []float64, i, nesting int) {
	g := sh.gp
	chain, kind := g.chain[i], g.kind[i]
	g.chain[i] = 0
	j := i + chain
	if j < 0 || j >= len(buf) || nesting == 0 {
		return
	}
	if g.chain[j] != 0 {
		sh.propagateAt(buf, sums, j, nesting-1)
	}
	// Along the line and across it, which in a run set upright are the
	// vertical axis and the horizontal one.
	along, across := &buf[i].XOffset, &buf[i].YOffset
	targetAlong, targetAcross := buf[j].XOffset, buf[j].YOffset
	if sh.features.Vertical {
		along, across = &buf[i].YOffset, &buf[i].XOffset
		targetAlong, targetAcross = buf[j].YOffset, buf[j].XOffset
	}
	if kind == attachCursive {
		*across += targetAcross
		return
	}
	*along += targetAlong
	switch {
	case j < i && !sh.rtl:
		*along -= sums[i] - sums[j]
	case j < i:
		*along += sums[i+1] - sums[j+1]
	case !sh.rtl:
		*along += sums[j] - sums[i]
	default:
		*along -= sums[j+1] - sums[i+1]
	}
}

// advanceSums is the running total of the advances in buf along the line —
// the vertical ones for a run set upright — so that what stands between any
// two glyphs is one subtraction.
func advanceSums(buf []Glyph, vertical bool) []float64 {
	sums := make([]float64, len(buf)+1)
	for k := range buf {
		advance := buf[k].XAdvance
		if vertical {
			advance = buf[k].YAdvance
		}
		sums[k+1] = sums[k] + advance
	}
	return sums
}

// crossOffset is the offset across the line a mark attached to the glyph at j
// takes from it — the height, or for a run set upright the offset to the side:
// its own offset, and that of every glyph it is cursively attached to, since
// those are added to it only at the end. HarfBuzz's resolve_cross_offset.
func (sh shaper) crossOffset(buf []Glyph, j int) float64 {
	g := sh.gp
	across := func(k int) float64 {
		if sh.features.Vertical {
			return buf[k].XOffset
		}
		return buf[k].YOffset
	}
	off := across(j)
	for steps := 0; g.kind[j] == attachCursive && g.chain[j] != 0 && steps < len(buf); steps++ {
		j += g.chain[j]
		if j < 0 || j >= len(buf) {
			break
		}
		off += across(j)
	}
	return off
}

// placeMark attaches the mark at i to the glyph at j, so that their anchors
// meet: the height from where j stands now, and along the line its offset from
// j and that it hangs from j, for propagate to finish once every lookup has
// run. HarfBuzz's MarkArray::apply.
//
// It *sets* the offset rather than adding to it, which is what the format says
// and what makes applying the same attachment twice harmless — a lookup both
// named by a feature and reached from a rule places the mark in the same place
// either time. A later attachment of the same mark replaces an earlier one, as
// a later lookup's does in HarfBuzz.
func (sh shaper) placeMark(buf []Glyph, i, j int, mark, base anchor) {
	buf[i].XOffset = sh.f.scale(base.x - mark.x)
	buf[i].YOffset = sh.f.scale(base.y - mark.y)
	if sh.features.Vertical {
		buf[i].XOffset += sh.crossOffset(buf, j)
	} else {
		buf[i].YOffset += sh.crossOffset(buf, j)
	}
	sh.gp.chain[i] = j - i
	sh.gp.kind[i] = attachMark
}

// visibleBefore is the nearest glyph before a position that a lookup with these
// flags does not step over, or -1: HarfBuzz's skippy_iter.prev, for a lookup
// that matches whatever it lands on.
//
// It walks, as HarfBuzz walks, and the walk is not quadratic however long a run
// of marks it steps over: a lookup is applied only at the glyphs its flags do
// not step over, so each walk ends at the glyph the previous one started from,
// and a pass walks the run once. A run of sixteen thousand marks a mark-to-mark
// lookup steps over is TestMarkToMarkDoesNotWalkBackOverTheMarksItIgnores.
func (sh shaper) visibleBefore(buf []Glyph, at, flags, markSet int) int {
	j := at - 1
	for j >= 0 && sh.l.ignoresIn(flags, markSet, buf[j]) {
		j--
	}
	return j
}

// anchor is a point in a glyph's own coordinate space, in font units.
type anchor struct{ x, y int }

// markAnchor is a mark's own attachment point and the class it belongs to.
// Classes let a font say that, for instance, a base's anchor for accents above
// is not the one for cedillas below.
type markAnchor struct {
	class  int
	anchor anchor
}

// singleAdjust is a ValueRecord's placement and advance: a nudge to a glyph.
type singleAdjust struct {
	xPlacement, yPlacement, xAdvance int
}

// readValueRecord reads the placement and advance fields a ValueRecord may
// carry, in the fixed order the format defines.
func readValueRecord(rec []byte, format int) singleAdjust {
	var out singleAdjust
	off := 0
	take := func(bit int) int {
		if format&bit == 0 {
			return 0
		}
		if off+2 > len(rec) {
			return 0
		}
		v := signed16(font.Be16(rec, off))
		off += 2
		return v
	}
	out.xPlacement = take(0x0001)
	out.yPlacement = take(0x0002)
	out.xAdvance = take(0x0004)
	return out
}

// valueYAdvance is a ValueRecord's YAdvance, the field readValueRecord does
// not read: an advance down the page, which only a run set upright applies.
// Zero where the format has none or the record is cut short.
func valueYAdvance(rec []byte, format int) int {
	if format&0x0008 == 0 {
		return 0
	}
	off := 2 * bits.OnesCount(uint(format&0x0007))
	if off+2 > len(rec) {
		return 0
	}
	return signed16(font.Be16(rec, off))
}

// readAnchor reads an anchor table. All three formats begin with the same two
// coordinates; the later formats add hinting information this ignores, which
// affects rendering at small sizes and not where the anchor is.
func readAnchor(base []byte, off int) (anchor, bool) {
	if off <= 0 || off+6 > len(base) {
		return anchor{}, false
	}
	a := base[off:]
	switch font.Be16(a, 0) {
	case 1, 2, 3:
		return anchor{x: signed16(font.Be16(a, 2)), y: signed16(font.Be16(a, 4))}, true
	}
	return anchor{}, false
}

// isMark reports whether a glyph is a mark: what GDEF's classification says,
// or for a font with none, what the character the glyph came from says. It is
// HarfBuzz's glyph property, which is what every lookup flag and every mark
// lookup reads.
//
// It used to fall back further, to the glyphs any mark lookup names as a mark,
// for a glyph that came from no character. That was a guess HarfBuzz does not
// make — a glyph a substitution produced carries the class of the one it was
// produced from — and it made a glyph a mark to every lookup because one
// lookup placed it like one.
func (l *layout) isMark(g Glyph) bool {
	return l.classOf(g) == classMark
}
