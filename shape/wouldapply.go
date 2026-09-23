package shape

import "github.com/mgilbir/forme/font"

// Whether a substitution lookup would apply to a sequence of glyphs — the
// question the Indic model asks a font about a consonant before any of its
// rules have run.
//
// Whether a consonant after a virama is drawn below the base, after it or
// before it, and whether a Ra that opens a syllable becomes a reph, is not
// something the characters say: it is whether this font has the form. So the
// model asks the font's 'blwf', 'pstf', 'pref' and 'rphf' whether they would
// substitute the virama and the consonant, and the base search turns on the
// answer.
//
// The question is HarfBuzz's would_apply, and it is narrower than "would
// anything change". A lookup answers yes only if it would take exactly the
// sequence asked about: a ligature of those glyphs and no others, a contextual
// rule whose input is that sequence. A single substitution never does — it
// takes one glyph — so a font whose 'pstf' holds a lone alternate for a
// consonant is not saying that consonant is a post-base form; applying the
// lookup to the probe and looking for a change answered yes to it, and made the
// consonant post-base in every syllable.
//
// With zeroContext, which is how HarfBuzz asks a font written to the
// second-generation specification, a chained rule that needs anything before
// or after the sequence does not count either: the probe has nothing around it.
// Without it the rule's context is not asked about at all.
func (sh shaper) wouldSubstitute(lookups []int, gids []int, zeroContext bool) bool {
	if len(gids) == 0 {
		return false
	}
	for _, idx := range lookups {
		if idx < 0 || idx >= len(sh.l.gsub) {
			continue
		}
		lk := sh.l.gsub[idx]
		for _, sub := range lk.subs {
			if wouldApply(lk.kind, sub, gids, zeroContext) {
				return true
			}
		}
	}
	return false
}

// wouldApply is one subtable's would_apply.
//
// Where HarfBuzz reads a subtable without asking its coverage — the
// class-based context formats pick a rule set by the first glyph's class, and
// the coverage-based ones check only the glyphs after the first — it has asked
// the lookup's coverage beforehand, through a filter that says yes to every
// glyph the coverage names. Asking this subtable's coverage of the first
// glyph is that question put exactly.
func wouldApply(kind int, sub []byte, gids []int, zeroContext bool) bool {
	if len(sub) < 4 {
		return false
	}
	switch kind {
	case 1, 2, 3, 8:
		// One glyph in, whatever comes out; all four keep their coverage at
		// the same place.
		if len(gids) != 1 {
			return false
		}
		_, ok := coverageIndex(sub, font.Be16(sub, 2), gids[0])
		return ok
	case 4:
		return wouldLigate(sub, gids)
	case 5:
		return wouldMatchContext(sub, gids)
	case 6:
		return wouldMatchChained(sub, gids, zeroContext)
	}
	return false
}

// wouldLigate: a ligature whose components are the sequence, all of it.
func wouldLigate(sub []byte, gids []int) bool {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return false
	}
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gids[0])
	if !ok || i >= font.Be16(sub, 4) || 6+2*i+2 > len(sub) {
		return false
	}
	so := font.Be16(sub, 6+2*i)
	if so <= 0 || so+2 > len(sub) {
		return false
	}
	set := sub[so:]
	for k := 0; k < font.Be16(set, 0) && 2+2*k+2 <= len(set); k++ {
		lo := font.Be16(set, 2+2*k)
		if lo <= 0 || lo+4 > len(set) {
			continue
		}
		lig := set[lo:]
		n := font.Be16(lig, 2)
		if n != len(gids) || 4+2*(n-1) > len(lig) {
			continue
		}
		if sequenceIs(lig, 4, gids, func(item, gid int) bool { return item == gid }) {
			return true
		}
	}
	return false
}

// sequenceIs reports whether the items stored from at match the glyphs after
// the first, one each.
func sequenceIs(b []byte, at int, gids []int, match func(item, gid int) bool) bool {
	for k := 1; k < len(gids); k++ {
		p := at + 2*(k-1)
		if p+2 > len(b) || !match(font.Be16(b, p), gids[k]) {
			return false
		}
	}
	return true
}

// wouldMatchContext: a sequence context rule whose input is the sequence.
func wouldMatchContext(sub []byte, gids []int) bool {
	switch font.Be16(sub, 0) {
	case 1, 2:
		byClass := font.Be16(sub, 0) == 2
		setsAt := 6
		if byClass {
			setsAt = 8
		}
		if len(sub) < setsAt {
			return false
		}
		index, ok := coverageIndex(sub, font.Be16(sub, 2), gids[0])
		if !ok {
			return false
		}
		match := func(item, gid int) bool { return item == gid }
		if byClass {
			classes := font.Be16(sub, 4)
			index = classAt(sub, classes, gids[0])
			match = func(item, gid int) bool { return classAt(sub, classes, gid) == item }
		}
		return forEachRule(sub, setsAt, index, func(rule []byte) bool {
			n := font.Be16(rule, 0)
			return n == len(gids) && 4+2*(n-1) <= len(rule) && sequenceIs(rule, 4, gids, match)
		})
	case 3:
		n := font.Be16(sub, 2)
		if n != len(gids) || 6+2*n > len(sub) {
			return false
		}
		if _, ok := coverageIndex(sub, font.Be16(sub, 6), gids[0]); !ok {
			return false
		}
		return sequenceIs(sub, 8, gids, func(cov, gid int) bool {
			_, ok := coverageIndex(sub, cov, gid)
			return ok
		})
	}
	return false
}

// wouldMatchChained: a chained context rule whose input is the sequence, and
// with zeroContext, one that asks for nothing before or after it.
func wouldMatchChained(sub []byte, gids []int, zeroContext bool) bool {
	switch font.Be16(sub, 0) {
	case 1, 2:
		byClass := font.Be16(sub, 0) == 2
		setsAt := 6
		if byClass {
			setsAt = 12
		}
		if len(sub) < setsAt {
			return false
		}
		index, ok := coverageIndex(sub, font.Be16(sub, 2), gids[0])
		if !ok {
			return false
		}
		match := func(item, gid int) bool { return item == gid }
		if byClass {
			classes := font.Be16(sub, 6)
			index = classAt(sub, classes, gids[0])
			match = func(item, gid int) bool { return classAt(sub, classes, gid) == item }
		}
		return forEachRule(sub, setsAt, index, func(rule []byte) bool {
			back := font.Be16(rule, 0)
			p := 2 + 2*back
			if p+2 > len(rule) {
				return false
			}
			n := font.Be16(rule, p)
			inputAt := p + 2
			p = inputAt + 2*(n-1)
			if n != len(gids) || p+2 > len(rule) {
				return false
			}
			ahead := font.Be16(rule, p)
			if zeroContext && (back != 0 || ahead != 0) {
				return false
			}
			return sequenceIs(rule, inputAt, gids, match)
		})
	case 3:
		back := font.Be16(sub, 2)
		p := 4 + 2*back
		if p+2 > len(sub) {
			return false
		}
		n := font.Be16(sub, p)
		inputAt := p + 2
		p = inputAt + 2*n
		if n != len(gids) || p+2 > len(sub) {
			return false
		}
		ahead := font.Be16(sub, p)
		if zeroContext && (back != 0 || ahead != 0) {
			return false
		}
		if _, ok := coverageIndex(sub, font.Be16(sub, inputAt), gids[0]); !ok {
			return false
		}
		return sequenceIs(sub, inputAt+2, gids, func(cov, gid int) bool {
			_, ok := coverageIndex(sub, cov, gid)
			return ok
		})
	}
	return false
}

// forEachRule walks the rules of one rule set of a context or chained context
// subtable in glyph or class form, and reports whether any satisfies ok.
func forEachRule(sub []byte, setsAt, index int, ok func(rule []byte) bool) bool {
	count := font.Be16(sub, setsAt-2)
	if index < 0 || index >= count || setsAt+2*index+2 > len(sub) {
		return false
	}
	off := font.Be16(sub, setsAt+2*index)
	if off <= 0 || off+2 > len(sub) {
		return false
	}
	set := sub[off:]
	for r := 0; r < font.Be16(set, 0) && 2+2*r+2 <= len(set); r++ {
		ro := font.Be16(set, 2+2*r)
		if ro <= 0 || ro+4 > len(set) {
			continue
		}
		if ok(set[ro:]) {
			return true
		}
	}
	return false
}
