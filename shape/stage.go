package shape

import "github.com/mgilbir/forme/font"

// Applying a stage of a plan: its lookups one after another, each over the part
// of the run it is for.
//
// Every model applies its stages through here. What differs between them is
// only the bounds and the per-glyph record a model keeps beside the buffer — the
// syllabic ones keep categories and positions that a ligature or a
// decomposition has to be carried through — and those arrive as a window and a
// set of hooks rather than as a copy of this loop per model, which is what
// there used to be.

// maskAllows reports whether the lookup being applied is for a glyph.
func (sh shaper) maskAllows(g Glyph) bool {
	return sh.lookupMask == 0 || g.mask&sh.lookupMask != 0
}

// recordHooks keep a model's per-glyph record in step with a buffer a stage is
// reshaping: resize and remove are told what each lookup did to the buffer's
// length (see shaper.onResize and shaper.onDelete), and joiner says where the
// join controls are (shaper.joinerAt). A model that keeps no record passes none.
type recordHooks struct {
	resize func(at, delta int)
	remove func(at int)
	joiner func(at int) joinerKind
}

// applyStage applies a stage's lookups over the whole of a run that keeps no
// per-glyph record.
func (sh shaper) applyStage(buf []Glyph, stage []planLookup) []Glyph {
	if len(stage) == 0 || len(buf) == 0 {
		return buf
	}
	out, _, _ := sh.applyLookups(buf, stage, 0, len(buf), 0, len(buf), recordHooks{})
	return out
}

// applyLookups applies lookups in order, each starting at the positions
// [from, to) and seeing [floor, ceil): a lookup held to a syllable is given the
// syllable for both, and a lookup for a whole run the whole run.
//
// It reports the buffer, how much its length changed, and the earliest position
// a lookup applied at, or -1 — which is how the universal engine reads which
// glyph its 'rphf' and 'pref' were for.
//
// Each lookup is applied with its own mask and its own joiner modes, because a
// stage merges the lookups of several features and each keeps what its feature
// asked for. See planLookup.
func (sh shaper) applyLookups(buf []Glyph, lookups []planLookup, from, to, floor, ceil int,
	h recordHooks) ([]Glyph, int, int) {

	total, step, first := 0, 0, -1
	sh.onResize = func(at, d int) {
		if h.resize != nil {
			h.resize(at, d)
		}
		step += d
	}
	sh.onDelete = func(at int) {
		if h.remove != nil {
			h.remove(at)
		}
		step--
	}
	sh.joinerAt = h.joiner
	sh.floor = floor
	for _, lk := range lookups {
		if lk.index < 0 || lk.index >= len(sh.l.gsub) || from >= to {
			continue
		}
		sh.lookupMask = lk.mask
		sh.manualZWJ, sh.manualZWNJ = lk.manualZWJ, lk.manualZWNJ
		if sh.l.gsub[lk.index].kind == 8 {
			sh.limit = ceil
			sh.run = nil
			if at := sh.applyReverse(buf, lk.index, from, to); at >= 0 && (first < 0 || at < first) {
				first = at
			}
			continue
		}
		// The walk is always at the front of what is left: what it has passed
		// is settled, and a lookup that changes the run's length gives its room
		// back to the gap between the two rather than closing it. See runBuf.
		rb := newRunBuf(buf, from)
		sh.run = rb
		for rb.w < to && len(rb.pending()) > 0 {
			step = 0
			// The far edge, as it stands now. It moves: a lookup that takes a
			// glyph apart makes the window longer, and the next position has to
			// be allowed to see what it produced.
			sh.limit = ceil
			if !sh.maskAllows(rb.pending()[0]) {
				rb.settle(1)
				continue
			}
			consumed, _ := sh.applyGSUBAt(lk.index, rb.pending(), 0, 0)
			to += step
			ceil += step
			total += step
			if consumed > 0 {
				if first < 0 || rb.w < first {
					first = rb.w
				}
				rb.settle(consumed)
				continue
			}
			// A lookup that consumed nothing and shortened the run took a glyph
			// out; what followed it is now here and has not been looked at.
			if step >= 0 {
				rb.settle(1)
			}
		}
		buf = rb.flatten()
	}
	return buf, total, first
}

// applyReverse applies a reverse chaining single substitution — GSUB lookup
// type 8 — over [from, to), from the last position back to the first, and
// reports the first position it substituted at in the text's order, or -1.
//
// It is the one lookup type that runs backwards, and that is its whole point: a
// rule whose context is what *follows* sees what follows already substituted,
// so a form propagates from the end of a word to its start. Nastaliq fonts join
// a word that way, and Noto Sans Coptic puts its combining overlines into their
// capped forms with one under 'ccmp'. It substitutes one glyph for one, so the
// run's length never changes, and it is never applied from inside another
// lookup — the format says so and HarfBuzz refuses it there.
//
// It was not applied at all, and nothing said so: a lookup of this type matched
// nothing and the forms it states were silently absent.
func (sh shaper) applyReverse(buf []Glyph, idx, from, to int) int {
	lk := sh.l.gsub[idx]
	sh.markSet = lk.markSet
	if to > len(buf) {
		to = len(buf)
	}
	first := -1
	for at := to - 1; at >= from; at-- {
		if !sh.maskAllows(buf[at]) || sh.ignores(lk.flags, buf[at]) {
			continue
		}
		for _, sub := range lk.subs {
			gid, ok := sh.reverseChainAt(sub, buf, at, lk.flags)
			if !ok {
				continue
			}
			buf[at].GID = gid
			buf[at].XAdvance = sh.f.advanceGID(gid)
			first = at
			break
		}
	}
	return first
}

// reverseChainAt reads a type 8 subtable at a position: the glyph it is
// replaced by, if the subtable covers it and its context matches.
//
// The layout is a chained context of coverage tables with no input sequence
// but the one glyph: format, coverage, the backtrack coverages nearest first,
// the lookahead coverages, and a substitute per coverage index.
func (sh shaper) reverseChainAt(sub []byte, buf []Glyph, at, flags int) (int, bool) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return 0, false
	}
	index, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
	if !ok {
		return 0, false
	}
	backCount := font.Be16(sub, 4)
	backAt := 6
	p := backAt + 2*backCount
	if p+2 > len(sub) {
		return 0, false
	}
	aheadCount := font.Be16(sub, p)
	aheadAt := p + 2
	p = aheadAt + 2*aheadCount
	if p+2 > len(sub) {
		return 0, false
	}
	glyphCount := font.Be16(sub, p)
	substAt := p + 2
	if index >= glyphCount || substAt+2*index+2 > len(sub) {
		return 0, false
	}
	covers := func(listAt, k, gid int) bool {
		_, ok := coverageIndex(sub, font.Be16(sub, listAt+2*k), gid)
		return ok
	}
	if backCount > 0 && !sh.matchBacktrack(buf, at, backCount, flags, func(k int, g Glyph) bool {
		return covers(backAt, k, g.GID)
	}) {
		return 0, false
	}
	if aheadCount > 0 && !sh.matchLookahead(buf, at+1, aheadCount, flags, func(k int, g Glyph) bool {
		return covers(aheadAt, k, g.GID)
	}) {
		return 0, false
	}
	return font.Be16(sub, substAt+2*index), true
}

// applyStageBySyllable applies a stage over a whole run whose glyphs belong to
// syllables: a lookup held to one syllable is applied syllable by syllable, and
// any other over the run.
//
// It is how the Indic model's last stage is applied. HarfBuzz puts the Indic
// presentation features — held to a syllable — in one stage with the ligatures
// and contextual alternates every script gets, which are not, and applies the
// lot in lookup order. Applying the presentation features per syllable first and
// the rest afterwards, which is what stood here, reorders any font whose lookups
// for the two interleave.
//
// syllables reports the run's syllables as they stand, as windows of the
// buffer; it is asked again for every lookup held to them, since the lookups
// before it may have changed the run's length.
func (sh shaper) applyStageBySyllable(buf []Glyph, stage []planLookup, h recordHooks,
	syllables func() [][2]int) []Glyph {

	for i := range stage {
		lk := stage[i : i+1]
		if !lk[0].perSyllable {
			buf, _, _ = sh.applyLookups(buf, lk, 0, len(buf), 0, len(buf), h)
			continue
		}
		shift := 0
		for _, w := range syllables() {
			lo, hi := w[0]+shift, w[1]+shift
			var d int
			buf, d, _ = sh.applyLookups(buf, lk, lo, hi, lo, hi, h)
			shift += d
		}
	}
	return buf
}
