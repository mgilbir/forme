package shape

import "github.com/mgilbir/forme/font"

// Positioning lookups at a glyph: what each GPOS lookup type does where it
// applies, whether the positioning pass reached the glyph walking the run or a
// contextual rule named the lookup there.
//
// Each type is HarfBuzz's reading of it, because a font is tested against
// HarfBuzz: a subtable applies or it does not, the first of a lookup's
// subtables to apply at a glyph is the only one that does, and what "applies"
// means is stated per type below — it is not always "changed something". A
// pair of glyphs a subtable names with a zero adjustment has applied, and a
// later subtable of the same lookup is not asked about it.
//
// # Contextual rules
//
// Types 7 and 8 are to positioning what types 5 and 6 are to substitution, with
// exactly the same subtable formats: "where this sequence occurs, apply
// positioning lookup six at position two". So the same matching serves both
// (context.go), and a matched rule applies the lookup it names here.
//
// A lookup both named by a feature and reached from a rule is applied by each,
// as HarfBuzz applies it: a nudge of 100 named both ways moves the glyph by 200
// in both implementations. The format says a feature's lookups run over the
// text and a rule's run where it matches, and nothing says the two may not be
// the same lookup. Noto Serif Tibetan has one.

// applyGPOSAt applies one positioning lookup at a position, reporting how many
// glyphs the walk moves on by — zero when no subtable applied.
//
// A position outside the buffer, on either side, applies nothing, for the
// reason applyGSUBAt gives.
func (sh shaper) applyGPOSAt(idx int, buf []Glyph, at, depth int) int {
	if depth > maxLookupRecursion || idx < 0 || idx >= len(sh.l.gpos) || at < 0 || at >= len(buf) {
		return 0
	}
	lk := sh.l.gpos[idx]
	sh.markSet = lk.markSet
	if sh.ignores(lk.flags, buf[at]) {
		return 0
	}
	for _, sub := range lk.subs {
		var n int
		switch lk.kind {
		case 1:
			n = sh.singlePosAt(sub, buf, at)
		case 2:
			n = sh.pairPosAt(sub, buf, at, lk.flags)
		case 3:
			n = sh.cursiveAt(sub, buf, at, lk.flags)
		case 4:
			n = sh.markToBaseAt(sub, buf, at, false)
		case 5:
			n = sh.markToBaseAt(sub, buf, at, true)
		case 6:
			n = sh.markToMarkAt(sub, buf, at, lk.flags)
		case 7:
			sh.positioning = true
			n, _, _ = sh.sequenceContext(sub, buf, at, lk.flags, depth)
		case 8:
			sh.positioning = true
			n, _, _ = sh.chainedContext(sub, buf, at, lk.flags, depth)
		}
		if n > 0 {
			return n
		}
	}
	return 0
}

// singlePosAt applies a type 1 subtable — one adjustment for every covered
// glyph (format 1) or one per glyph (format 2) — at a position.
//
// A covered glyph applies it whatever the adjustment is, a zero included.
func (sh shaper) singlePosAt(sub []byte, buf []Glyph, at int) int {
	if len(sub) < 6 {
		return 0
	}
	covered, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
	if !ok {
		return 0
	}
	format := font.Be16(sub, 4)
	var adj singleAdjust
	switch font.Be16(sub, 0) {
	case 1:
		adj = readValueRecord(sub[6:], format)
	case 2:
		size := valueSize(format)
		off := 8 + covered*size
		if covered >= font.Be16(sub, 6) || off+size > len(sub) {
			return 0
		}
		adj = readValueRecord(sub[off:], format)
	default:
		return 0
	}
	buf[at].XOffset += sh.f.scale(adj.xPlacement)
	buf[at].YOffset += sh.f.scale(adj.yPlacement)
	buf[at].XAdvance += sh.f.scale(adj.xAdvance)
	return 1
}

// pairPosAt applies a type 2 subtable to the pair beginning at a position: the
// glyph there and the next one the lookup does not step over.
//
// A pair the subtable names applies whatever its adjustment, and the walk
// moves to the second glyph — past it, where the subtable states a second
// value record at all, since then the second glyph's place is settled. What
// "names" means differs between the formats, and is the one thing about pairs
// a reader gets wrong most easily:
//
//   - Format 1 lists pairs. A second glyph the first's list does not name is
//     not a pair, and a later subtable is asked.
//   - Format 2 is a matrix of classes, and every glyph has a class: one a class
//     table does not list is in class 0. So a first glyph this subtable covers
//     pairs with *every* second glyph, and a later subtable of the lookup is
//     never asked about it. A font states an exception to a class rule as an
//     explicit pair in an earlier subtable, not a later one.
//
// This read class 0 of the second glyph as "not paired" and a zero record as
// "not applied", which let a later subtable apply where HarfBuzz stops.
func (sh shaper) pairPosAt(sub []byte, buf []Glyph, at, flags int) int {
	if len(sub) < 10 {
		return 0
	}
	covered, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
	if !ok {
		return 0
	}
	next := sh.nextNotIgnored(buf, at+1, flags, -1)
	if next >= sh.end(buf) {
		return 0
	}
	format1, format2 := font.Be16(sub, 4), font.Be16(sub, 6)
	var rec []byte
	switch font.Be16(sub, 0) {
	case 1:
		if covered >= font.Be16(sub, 8) || 10+2*covered+2 > len(sub) {
			return 0
		}
		off := font.Be16(sub, 10+2*covered)
		if off <= 0 || off+2 > len(sub) {
			return 0
		}
		set := sub[off:]
		size := 2 + valueSize(format1) + valueSize(format2)
		n := min(font.Be16(set, 0), (len(set)-2)/size)
		// The records are in order of their second glyph, which the format
		// requires, and are searched as HarfBuzz searches them.
		lo, hi := 0, n-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			r := 2 + mid*size
			switch g := font.Be16(set, r); {
			case buf[next].GID < g:
				hi = mid - 1
			case buf[next].GID > g:
				lo = mid + 1
			default:
				rec = set[r+2 : r+size]
				lo = hi + 1
			}
		}
		if rec == nil {
			return 0
		}
	case 2:
		if len(sub) < 16 {
			return 0
		}
		// Each class is searched for in its table rather than read out of a
		// map of every glyph the table names.
		c1 := classAt(sub, font.Be16(sub, 8), buf[at].GID)
		c2 := classAt(sub, font.Be16(sub, 10), buf[next].GID)
		n1, n2 := font.Be16(sub, 12), font.Be16(sub, 14)
		size := valueSize(format1) + valueSize(format2)
		off := 16 + (c1*n2+c2)*size
		if c1 >= n1 || c2 >= n2 || off+size > len(sub) {
			return 0
		}
		rec = sub[off : off+size]
	default:
		return 0
	}
	adj := pairAdjustFrom(rec, format1, format2)
	buf[at].XOffset += sh.f.scale(int(adj.firstX))
	buf[at].YOffset += sh.f.scale(int(adj.firstY))
	buf[at].XAdvance += sh.f.scale(int(adj.firstAdvance))
	buf[next].XOffset += sh.f.scale(int(adj.secondX))
	buf[next].YOffset += sh.f.scale(int(adj.secondY))
	buf[next].XAdvance += sh.f.scale(int(adj.secondAdvance))
	if format2 != 0 {
		return next - at + 1
	}
	return next - at
}

// cursiveAt applies a type 3 subtable at a position: the glyph there joins the
// one before it that the lookup does not step over, where the first has an
// entry and the second an exit. HarfBuzz's CursivePosFormat1::apply.
//
// This is the other half of what a cursive script needs. Joining (arabic.go)
// picks the right *shape* for each position; this makes the shapes actually
// meet. The joint is placed so the second glyph's entry lands exactly on the
// first's exit: along the line by shortening the advance between them, and
// across it by hanging one glyph off the other, because a joining stroke does
// not generally leave a letter at the height it enters the next.
//
// # Which glyph gives ground along the line
//
// The joint is between the first glyph's exit and the second's entry whichever
// way the run is drawn, but which of the two has to move is not the same. Drawn
// left to right the first glyph is reached first, so it is cut short at its exit
// and the second is pulled back onto it. Drawn right to left the pen reaches the
// *second* glyph first — the run is reversed after this — so it is that one that
// stops at its entry, and the first that is pulled back onto it.
//
// # Which glyph hangs from which
//
// Across the line the joint is an attachment, a child hanging from a parent,
// resolved at the end with every other (see propagate). The RightToLeft lookup
// flag says which is which, and it is the only thing that flag means: set, the
// second glyph is the parent, so the last glyph of a joined run stays on the
// baseline and the earlier ones climb to meet it — which is what an Arabic font
// wants. Clear, the first stays and the rest follow. A glyph that already hung
// from something else is turned round first, so that the whole of what it was
// joined to now hangs from its new parent.
func (sh shaper) cursiveAt(sub []byte, buf []Glyph, at, flags int) int {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return 0
	}
	entry, ok := cursiveAnchorAt(sub, buf[at].GID, 0)
	if !ok {
		return 0
	}
	prev := sh.visibleBefore(buf, at, flags, sh.markSet)
	if prev < 0 {
		return 0
	}
	exit, ok := cursiveAnchorAt(sub, buf[prev].GID, 2)
	if !ok {
		return 0
	}
	i, j := prev, at
	if sh.rtl {
		d := sh.f.scale(exit.x) + buf[i].XOffset
		buf[i].XAdvance -= d
		buf[i].XOffset -= d
		buf[j].XAdvance = sh.f.scale(entry.x) + buf[j].XOffset
	} else {
		buf[i].XAdvance = sh.f.scale(exit.x) + buf[i].XOffset
		d := sh.f.scale(entry.x) + buf[j].XOffset
		buf[j].XAdvance -= d
		buf[j].XOffset -= d
	}
	child, parent := i, j
	dy := sh.f.scale(entry.y - exit.y)
	if flags&flagRightToLeft == 0 {
		child, parent = parent, child
		dy = -dy
	}
	g := sh.gp
	sh.reverseCursive(buf, child, parent, 0)
	g.chain[child] = parent - child
	g.kind[child] = attachCursive
	buf[child].YOffset = dy
	// A parent that hung from this child is cut loose from it rather than
	// left in a loop.
	if g.chain[parent] == -g.chain[child] {
		g.chain[parent] = 0
		buf[parent].YOffset = 0
	}
	return 1
}

// reverseCursive turns round the chain a glyph hangs from, so that what it was
// attached to now hangs from it: HarfBuzz's reverse_cursive_minor_offset. It
// stops at the glyph's new parent, and where HarfBuzz stops following a chain.
func (sh shaper) reverseCursive(buf []Glyph, i, newParent, nesting int) {
	g := sh.gp
	if nesting > maxAttachmentNesting {
		return
	}
	chain := g.chain[i]
	if chain == 0 || g.kind[i] != attachCursive {
		return
	}
	g.chain[i] = 0
	j := i + chain
	if j < 0 || j >= len(buf) || j == newParent {
		return
	}
	sh.reverseCursive(buf, j, newParent, nesting+1)
	buf[j].YOffset = -buf[i].YOffset
	g.chain[j] = -chain
	g.kind[j] = attachCursive
}

// cursiveAnchorAt is a glyph's entry anchor (at 0) or exit anchor (at 2) in a
// cursive subtable, if the subtable covers the glyph and states one.
func cursiveAnchorAt(sub []byte, gid, which int) (anchor, bool) {
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gid)
	rec := 6 + 4*i + which
	if !ok || i >= font.Be16(sub, 4) || rec+2 > len(sub) {
		return anchor{}, false
	}
	return readAnchor(sub, font.Be16(sub, rec))
}

// markToBaseAt applies a mark-to-base (type 4) or mark-to-ligature (type 5)
// subtable to the mark at a position.
//
// # Finding the base
//
// The base is the nearest glyph before the mark that is not a mark, whatever
// the lookup's own flags say. With one exception, which is HarfBuzz's (its
// issues 740 and 4124): of the glyphs a multiple substitution made from one, a
// mark belongs to the first, so a later one the subtable does not cover is
// stepped over too. AlexBrush's 'ccmp' takes U+01C9 apart into l and j, and a
// cedilla after it goes on the l.
//
// The search is the one HarfBuzz makes and remembers: each mark looks back
// only as far as the last mark looked, and takes the base that search found
// when it meets nothing new. A letter with a long run of marks is walked once,
// not once per mark.
//
// # Which part of a ligature
//
// A ligature states an anchor per component, and a mark that was written
// inside it goes on the component it followed; one written after it goes on
// the last. See markLigatureComponent.
func (sh shaper) markToBaseAt(sub []byte, buf []Glyph, at int, ligature bool) int {
	if len(sub) < 12 || font.Be16(sub, 0) != 1 {
		return 0
	}
	classCount := font.Be16(sub, 6)
	if classCount <= 0 {
		return 0
	}
	mark, covered := markRecordAt(sub, buf[at].GID, classCount)
	if !covered {
		return 0
	}
	j := sh.markBaseFor(buf, at, sub)
	if j < 0 {
		return 0
	}
	baseIdx, ok := coverageIndex(sub, font.Be16(sub, 4), buf[j].GID)
	if !ok {
		return 0
	}
	var base anchor
	if ligature {
		base, ok = ligatureAnchorAt(sub, baseIdx, mark.class, classCount,
			markLigatureComponent(buf, at, j))
	} else {
		base, ok = baseAnchorAt(sub, baseIdx, mark.class, classCount)
	}
	if !ok {
		return 0
	}
	sh.placeMark(buf, at, j, mark.anchor, base)
	return 1
}

// markBaseFor is the glyph a mark at a position attaches to under mark-to-base
// or mark-to-ligature, or -1: HarfBuzz's search and its cache, which gposPass
// describes.
func (sh shaper) markBaseFor(buf []Glyph, at int, sub []byte) int {
	g := sh.gp
	if g.lastBaseUntil > at {
		g.lastBaseUntil, g.lastBase = 0, -1
	}
	for j := at; j > g.lastBaseUntil; j-- {
		c := buf[j-1]
		if sh.l.classOf(c) == classMark {
			continue
		}
		if !sh.l.acceptsMarks(buf, j-1) {
			if _, covered := coverageIndex(sub, font.Be16(sub, 4), c.GID); !covered {
				continue
			}
		}
		g.lastBase = j - 1
		break
	}
	g.lastBaseUntil = at
	return g.lastBase
}

// acceptsMarks reports whether a glyph a mark finds on its way back is one it
// may attach to: not one of the later parts of a multiple substitution, unless
// a mark stands between it and the part before. HarfBuzz's accept.
//
// Namdhinggo's 'ccmp' takes a Limbu vowel sign apart into a mark and a
// spacing part; a mark written after the sign finds the spacing part, which is
// the second of the two but follows a mark, and so is where it attaches — or,
// the subtable covering nothing there, where it is left unattached.
func (l *layout) acceptsMarks(buf []Glyph, i int) bool {
	g := buf[i]
	if !g.multiplied || g.lig.comp == 0 || i == 0 {
		return true
	}
	p := buf[i-1]
	return l.isMark(p) || !p.multiplied || p.lig.id != g.lig.id || g.lig.comp != p.lig.comp+1
}

// markLigatureComponent is which component of the ligature at j the mark at i
// goes on, counting from zero: the one it was written in, where it came from
// inside the ligature, and otherwise the last — which markToBaseAt reads as
// any value past the end.
func markLigatureComponent(buf []Glyph, i, j int) int {
	if lig := buf[j].lig.id; lig != 0 && lig == buf[i].lig.id && buf[i].lig.comp > 0 {
		return buf[i].lig.comp - 1
	}
	return -1
}

// markToMarkAt applies a mark-to-mark (type 6) subtable to the mark at a
// position.
//
// The mark it stacks on is the glyph immediately before it that its lookup
// does not step over — with the lookup's flags as they narrow which *marks* it
// sees, its filtering set and attachment class, and not as they ignore kinds of
// glyph, which would have it step over every mark there is. And that glyph has
// to be a mark: mark-to-mark attaches to what it lands on and never looks past
// a letter for one.
//
// Two marks stack only if they belong to the same letter: the same base, or
// the same component of the same ligature. A mark on the first half of a split
// vowel and a mark on the second are written above the same stretch of text and
// are not written above one another. The exception is a mark that is *itself*
// a ligature, whose component is zero: it stands for the whole of what it was
// made from, so it matches anything.
func (sh shaper) markToMarkAt(sub []byte, buf []Glyph, at, flags int) int {
	if len(sub) < 12 || font.Be16(sub, 0) != 1 {
		return 0
	}
	classCount := font.Be16(sub, 6)
	if classCount <= 0 {
		return 0
	}
	mark, covered := markRecordAt(sub, buf[at].GID, classCount)
	if !covered {
		return 0
	}
	j := sh.visibleBefore(buf, at, flags&^markStackIgnore, sh.markSet)
	if j < 0 || !sh.l.isMark(buf[j]) {
		return 0
	}
	id1, id2 := buf[at].lig.id, buf[j].lig.id
	comp1, comp2 := buf[at].lig.comp, buf[j].lig.comp
	same := false
	if id1 == id2 {
		same = id1 == 0 || comp1 == comp2
	} else {
		same = (id1 != 0 && comp1 == 0) || (id2 != 0 && comp2 == 0)
	}
	if !same {
		return 0
	}
	baseIdx, ok := coverageIndex(sub, font.Be16(sub, 4), buf[j].GID)
	if !ok {
		return 0
	}
	base, ok := baseAnchorAt(sub, baseIdx, mark.class, classCount)
	if !ok {
		return 0
	}
	sh.placeMark(buf, at, j, mark.anchor, base)
	return 1
}

// markStackIgnore are the lookup flags mark-to-mark does not look back past:
// they are about finding a base, and would have it step over every mark there
// is, which is the one thing it exists to find. What is left is the mark
// filtering set and the mark attachment class.
const markStackIgnore = flagIgnoreBaseGlyphs | flagIgnoreLigatures | flagIgnoreMarks

// markRecordAt is a mark's class and anchor in a mark-attachment subtable, if
// the subtable covers it.
func markRecordAt(sub []byte, gid, classCount int) (markAnchor, bool) {
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gid)
	off := font.Be16(sub, 8)
	if !ok || off <= 0 || off+2 > len(sub) {
		return markAnchor{}, false
	}
	ma := sub[off:]
	rec := 2 + 4*i
	if i >= font.Be16(ma, 0) || rec+4 > len(ma) {
		return markAnchor{}, false
	}
	class := font.Be16(ma, rec)
	a, ok := readAnchor(ma, font.Be16(ma, rec+2))
	if !ok || class >= classCount {
		return markAnchor{}, false
	}
	return markAnchor{class: class, anchor: a}, true
}

// baseAnchorAt is where a mark-to-base or mark-to-mark subtable says the glyph
// at a coverage index receives a mark of a class, if it states one.
func baseAnchorAt(sub []byte, i, class, classCount int) (anchor, bool) {
	off := font.Be16(sub, 10)
	if off <= 0 || off+2 > len(sub) {
		return anchor{}, false
	}
	ba := sub[off:]
	rec := 2 + (i*classCount+class)*2
	if i >= font.Be16(ba, 0) || rec+2 > len(ba) {
		return anchor{}, false
	}
	return readAnchor(ba, font.Be16(ba, rec))
}

// ligatureAnchorAt is where a mark-to-ligature subtable says a component of
// the ligature at a coverage index receives a mark of a class. A component
// out of range is the last one.
//
// A ligature is several letters drawn as one glyph, so a mark written under it
// has to say which of them it belongs to: a dot under the first f of "ffi" goes
// somewhere quite different from a dot under the second. The font answers by
// giving each ligature not one anchor per class but one per component per
// class.
func ligatureAnchorAt(sub []byte, i, class, classCount, component int) (anchor, bool) {
	off := font.Be16(sub, 10)
	if off <= 0 || off+2 > len(sub) {
		return anchor{}, false
	}
	la := sub[off:]
	rec := 2 + 2*i
	if i >= font.Be16(la, 0) || rec+2 > len(la) {
		return anchor{}, false
	}
	attachOff := font.Be16(la, rec)
	if attachOff <= 0 || attachOff+2 > len(la) {
		return anchor{}, false
	}
	attach := la[attachOff:]
	count := font.Be16(attach, 0)
	if count < 1 {
		return anchor{}, false
	}
	if component < 0 || component >= count {
		component = count - 1
	}
	at := 2 + (component*classCount+class)*2
	if at+2 > len(attach) {
		return anchor{}, false
	}
	return readAnchor(attach, font.Be16(attach, at))
}
