package shape

// Pair kerning across the boundary between two runs.
//
// CSS Text §8.1 says an inline element boundary does not break shaping, and a
// run is not always the whole word: "す<span>。</span>" is two runs of one
// character each, and so is the same text after §8.4 cuts the full stop out to
// hang it. NotoSansJP kerns that pair — the す loses a tenth of an em before a
// full stop — and a run shaped alone never sees it, so the two characters are
// drawn a tenth of an em further apart than the font asks for. The suite's
// hanging-punctuation-block-bound-001 is exactly that, at 60px, where the tenth
// of an em is six visible pixels.
//
// It is separate from the context that ShapeGlyphsInContext already carried,
// which decides cursive *forms*: that one is answered while the glyphs still
// correspond to characters, before any substitution, and this one is a
// positioning rule and has to be answered after. Splicing the neighbouring
// glyph into the buffer and running the whole positioning pass over it would
// answer both at once and would also let a ligature form across the boundary,
// which is the one thing ShapeGlyphsInContext documents that it does not do.
// So the neighbour is shaped separately and only the pair lookup consults it.
//
// A pair record says something about each of its two glyphs, and the glyph on
// the far side belongs to the next run — which is shaped with this one as its
// own context and collects its own half of the adjustment there. Taking only
// this side's half is what makes the two runs agree rather than move apart by
// twice the kern.

// boundaryWindow bounds each side of the pass below. See boundaryGlyphs.
const boundaryWindow = 32

// boundaryGlyphs shapes the text standing either side of a run, so that a pair
// spanning the boundary can be looked up.
//
// The whole of each side is shaped rather than the single character a joining
// scan would stop at, because what is wanted here is a *glyph* and a glyph is
// what shaping decides: a neighbour beginning "fi" is one ligature and not an f,
// and a pair the font states on the ligature is not found by looking for the
// letter. It is shaping and not a GlyphID call for the same reason.
//
// The two sides are capped. Nothing about a boundary needs more than the glyphs
// at it, and the caller's context is the neighbouring run's whole text — which
// in the bidi path is the rest of the string as well — so an uncapped pass would
// make the work quadratic in a paragraph set as one long run. The window is far
// wider than the longest ligature any font here declares.
//
// The context passed on is empty, which is what stops this recurring: a run with
// no neighbours takes no boundary pass.
func (f *Face) boundaryGlyphs(ctx shapeContext, script uint16, rtl bool) (before, after []Glyph) {
	if b := lastRunes(ctx.before, boundaryWindow); b != "" {
		before, _ = f.shapeGlyphsIn(b, script, rtl, nil, shapeContext{})
	}
	if a := firstRunes(ctx.after, boundaryWindow); a != "" {
		after, _ = f.shapeGlyphsIn(a, script, rtl, nil, shapeContext{})
	}
	if rtl {
		// shapeGlyphsIn hands back visual order, and everything below is stated
		// in the order the text is written in.
		reverseGlyphs(before)
		reverseGlyphs(after)
	}
	return before, after
}

// firstRunes and lastRunes are the ends of a string, counted in characters
// rather than bytes so that neither can cut one in half.
func firstRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

func lastRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	// The starts of the last n characters, kept in a ring so that the string is
	// walked once and nothing is allocated per character.
	starts, at, seen := make([]int, n), 0, 0
	for i := range s {
		starts[at] = i
		at = (at + 1) % n
		seen++
	}
	if seen <= n {
		return s
	}
	return s[starts[at]:]
}

// kernAcross applies the pair kerning between a run's edge glyph and its
// neighbour, keeping the half of the adjustment that falls on this run.
//
// buf is in logical order, which is the order the font's pairs are stated in;
// before ends where buf begins and after begins where buf ends.
func (sh shaper) kernAcross(buf, before, after []Glyph) {
	if len(buf) == 0 {
		return
	}
	for _, kl := range sh.l.kern {
		if len(before) > 0 {
			// The neighbour is the last glyph of what precedes, and the run's
			// edge is its first — each skipping what this lookup steps over,
			// because a glyph a lookup ignores does not break a pair.
			if p, ok := lastNotIgnored(sh.l, kl.flags, before); ok {
				if i, ok := firstNotIgnored(sh.l, kl.flags, buf); ok {
					if k, ok := kl.pairs[[2]int{before[p].GID, buf[i].GID}]; ok {
						buf[i].XOffset += sh.f.scale(int(k.secondX))
						buf[i].YOffset += sh.f.scale(int(k.secondY))
						buf[i].XAdvance += sh.f.scale(int(k.secondAdvance))
					}
				}
			}
		}
		if len(after) > 0 {
			if i, ok := lastNotIgnored(sh.l, kl.flags, buf); ok {
				if n, ok := firstNotIgnored(sh.l, kl.flags, after); ok {
					if k, ok := kl.pairs[[2]int{buf[i].GID, after[n].GID}]; ok {
						buf[i].XOffset += sh.f.scale(int(k.firstX))
						buf[i].YOffset += sh.f.scale(int(k.firstY))
						buf[i].XAdvance += sh.f.scale(int(k.firstAdvance))
					}
				}
			}
		}
	}
}

// firstNotIgnored and lastNotIgnored are the ends of a buffer as one kerning
// lookup sees it.
func firstNotIgnored(l *layout, flags int, buf []Glyph) (int, bool) {
	for i := range buf {
		if !l.ignores(flags, buf[i]) {
			return i, true
		}
	}
	return 0, false
}

func lastNotIgnored(l *layout, flags int, buf []Glyph) (int, bool) {
	for i := len(buf) - 1; i >= 0; i-- {
		if !l.ignores(flags, buf[i]) {
			return i, true
		}
	}
	return 0, false
}
