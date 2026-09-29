package shape

import (
	"sync"

	"github.com/mgilbir/forme/font"
)

// The glyphs a positioning lookup can start at.
//
// A positioning pass walks each lookup over the whole run and, at every glyph,
// asks each subtable whether it applies there. Every subtable type begins that
// question with one coverage table — the glyph the lookup is applied at must be
// in it — and a lookup for marks, or for one script's letters, or for a rare
// pair, covers almost nothing a Latin label holds. The answer is "no" for every
// glyph of every lookup, and it was found the long way: the flags read against
// the glyph's class, then a binary search of each subtable's coverage. That was
// nine tenths of the time to shape thirty characters cold (issue 863).
//
// A gate is the union of the first coverage tables of a lookup's subtables, as
// a bitset by glyph. A glyph outside it is a glyph no subtable can apply at, so
// the walk steps over it without asking anything — which is what asking would
// have concluded. It is HarfBuzz's per-lookup digest, kept exact rather than
// approximate: the digest is a superset of the glyphs the lookup can act on,
// and that is all it needs to be.
//
// Which coverage is the first differs by subtype, and the list here is the
// contract with the functions that apply them (contextpos.go, context.go):
//
//   - Single, pair, cursive, mark-to-base, mark-to-ligature and mark-to-mark
//     all read the coverage at offset 2 of the subtable — for the mark lookups
//     it is the *mark* coverage, and the glyph the lookup is applied at is the
//     mark. A subtable applies to a glyph only through it.
//   - Sequence context formats 1 and 2 read it at offset 2, and format 3 reads
//     the coverage of the first input glyph, at offset 6.
//   - Chained context formats 1 and 2 read it at offset 2, and format 3 reads
//     the coverage of the first input glyph, after the backtrack list.
//
// A subtype the applying code does not know applies nothing, and so adds
// nothing here. A lookup kind that is not one of these, or whose coverages ask
// for more work than gateWork to read, has no gate and is walked as before.
//
// What a gate cannot say is which glyphs a lookup acts on *through another
// lookup*: a contextual lookup's records apply other lookups at positions it
// matched, and those go through applyGPOSAt, which is not gated. Only the walk
// of a lookup a plan names is.
type gposGate struct {
	once sync.Once
	// words is the bitset, or nil where the lookup has no gate. Empty and not
	// nil means the lookup starts at no glyph at all.
	words []uint64
}

// gposGateOff turns every gate open, so that a test can shape the same text
// with and without and require the two to agree. Nothing else sets it.
var gposGateOff bool

// gateWork is how many steps reading one lookup's coverages may take before the
// lookup is left ungated. A coverage's size is bounded by the table, but a font
// may name one coverage from every subtable of a lookup, and a range may say
// sixty-five thousand glyphs in twelve bytes. What is walked is bounded; what
// is left over is answered by the walk it would have been.
const gateWork = 1 << 20

// gateFor is the gate of the positioning lookup at index, built the first time
// it is asked for. A layout's gates are shared by every layout copied from it,
// as its lookups are, and built under the gate's own Once because two documents
// may be laid out at once.
func (l *layout) gateFor(index int) *gposGate {
	if index < 0 || index >= len(l.gposGates) {
		return nil
	}
	g := &l.gposGates[index]
	g.once.Do(func() { g.words = buildGate(l.gpos[index]) })
	return g
}

// startsAt reports whether the lookup may apply at a glyph. It is false only
// where no subtable can.
func (g *gposGate) startsAt(gid int) bool {
	if g == nil || g.words == nil || gposGateOff {
		return true
	}
	w := uint(gid) >> 6
	return w < uint(len(g.words)) && g.words[w]&(1<<(uint(gid)&63)) != 0
}

// buildGate reads the union of a lookup's first coverages, or nil where it
// cannot be stated.
func buildGate(lk rawLookup) []uint64 {
	switch lk.kind {
	case 1, 2, 3, 4, 5, 6, 7, 8:
	default:
		return nil
	}
	words := []uint64{}
	work := 0
	for _, sub := range lk.subs {
		off, ok := firstCoverageAt(lk.kind, sub)
		if !ok {
			continue
		}
		var fits bool
		if words, work, fits = addCoverage(words, work, sub, off); !fits {
			return nil
		}
	}
	return words
}

// firstCoverageAt is where a subtable of a kind keeps the coverage its first
// glyph must be in, and false where the subtable is one nothing applies.
func firstCoverageAt(kind int, sub []byte) (int, bool) {
	switch kind {
	case 1, 2, 3, 4, 5, 6:
		return font.Be16(sub, 2), true
	case 7:
		switch font.Be16(sub, 0) {
		case 1, 2:
			return font.Be16(sub, 2), true
		case 3:
			return font.Be16(sub, 6), true
		}
	case 8:
		switch font.Be16(sub, 0) {
		case 1, 2:
			return font.Be16(sub, 2), true
		case 3:
			// backtrack count, that many coverages, the input count, and the
			// input's first coverage.
			inputAt := 4 + 2*font.Be16(sub, 2) + 2
			return font.Be16(sub, inputAt), true
		}
	}
	return 0, false
}

// addCoverage sets in words every glyph the coverage table at sub[off:] names,
// counting the steps it takes against work. A table coverageIndex would find
// nothing in adds nothing; one it would search wrongly, being out of order, is
// read whole here, which can only add glyphs it might have found.
func addCoverage(words []uint64, work int, sub []byte, off int) ([]uint64, int, bool) {
	if off <= 0 || off+4 > len(sub) {
		return words, work, true
	}
	c := sub[off:]
	grow := func(gid int) {
		for len(words) <= gid>>6 {
			words = append(words, 0)
		}
	}
	switch font.Be16(c, 0) {
	case 1:
		n := font.Be16(c, 2)
		for i := 0; i < n && 4+2*i+2 <= len(c); i++ {
			if work++; work > gateWork {
				return nil, work, false
			}
			g := font.Be16(c, 4+2*i)
			grow(g)
			words[g>>6] |= 1 << (uint(g) & 63)
		}
	case 2:
		n := min(font.Be16(c, 2), (len(c)-4)/6)
		for i := 0; i < n; i++ {
			rec := 4 + 6*i
			start, end := font.Be16(c, rec), font.Be16(c, rec+2)
			if start > end {
				continue
			}
			// A range is set a word at a time, and its two ends a bit at a
			// time; the charge is for both, so that a coverage of sixty-five
			// thousand one-glyph ranges costs what its glyphs would.
			if work += 64 + (end-start)>>6; work > gateWork {
				return nil, work, false
			}
			grow(end)
			for g := start; g <= end; {
				if g&63 == 0 && g+63 <= end {
					words[g>>6] = ^uint64(0)
					g += 64
					continue
				}
				words[g>>6] |= 1 << (uint(g) & 63)
				g++
			}
		}
	}
	return words, work, true
}
