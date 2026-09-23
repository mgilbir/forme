package shape

import (
	"sort"

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
	// Before the rules run, so that what they state about a mark survives, and
	// the offset moves with the advance so the mark does not shift. The
	// universal engine and Myanmar.
	zeroMarksEarly
	// After the rules run, discarding whatever they said about a mark's advance.
	// Arabic, Hebrew, Thai and every script with no model of its own.
	zeroMarksLate
)

// cancelMarkWidths takes the advance off every mark.
//
// Done before the rules, the offset moves with the advance: the glyph is drawn
// where it would have been, and only the pen stops moving. Done after, the
// offsets are already whatever the rules made them and must not be touched.
func (sh shaper) cancelMarkWidths(buf []Glyph, adjustOffsets bool) {
	for i := range buf {
		if !sh.l.isMark(buf[i]) {
			continue
		}
		if adjustOffsets {
			buf[i].XOffset -= buf[i].XAdvance
		}
		buf[i].XAdvance = 0
	}
}

// applyPositioning runs the GPOS lookups over a shaped buffer.
func (sh shaper) position(buf []Glyph) {
	l := sh.l
	if sh.zeroMarks == zeroMarksEarly {
		sh.cancelMarkWidths(buf, true)
	}
	// Pair kerning, which the buffer expresses as a change to the left glyph's
	// advance. Glyphs the lookup ignores do not break a pair.
	//
	// One pass per lookup, in the order the font lists them, because each states
	// for itself which glyphs it steps over and because their adjustments add
	// up. A font that kerns letters in a lookup that ignores marks and kerns a
	// mark against its base in one that does not — Noto Serif Tibetan does — has
	// the second silenced by the first if the two are run as one.
	//
	// None of them where the document turned kerning off. It is skipped here
	// rather than filtered out of the font's lookups because a lookup list is
	// what the face parsed and this is what one caller asked; the face is shared
	// between every run of the document and most of them kern.
	for _, kl := range l.kern {
		if sh.features.NoKerning {
			break
		}
		prev := -1
		for i := range buf {
			if l.ignores(kl.flags, buf[i]) {
				continue
			}
			if prev < 0 {
				prev = i
				continue
			}
			if k, ok := kl.pair(buf[prev].GID, buf[i].GID); ok {
				// Both glyphs, and both what a record can say about each. A
				// placement moves the glyph and an advance moves what comes
				// after it, and a right-to-left font uses both for what a Latin
				// one does with the advance alone.
				buf[prev].XOffset += sh.f.scale(int(k.firstX))
				buf[prev].YOffset += sh.f.scale(int(k.firstY))
				buf[prev].XAdvance += sh.f.scale(int(k.firstAdvance))
				buf[i].XOffset += sh.f.scale(int(k.secondX))
				buf[i].YOffset += sh.f.scale(int(k.secondY))
				buf[i].XAdvance += sh.f.scale(int(k.secondAdvance))
				if k.takesSecond {
					// The lookup moves past both glyphs where the subtable
					// stated a second ValueRecord, so the glyph this pair
					// adjusted is not the first glyph of the next one. Where it
					// stated none, only the first is consumed and the second
					// begins the next pair — which is the ordinary kerning
					// case and is why "AVA" kerns twice.
					//
					// It cannot be read off the numbers: a font may state a
					// second record of all zeroes, and that is not the same as
					// stating none. Advancing past one glyph either way applied
					// a pair a conforming shaper never looks for.
					prev = -1
					continue
				}
			}
			prev = i
		}
	}
	// Contextual rules, which name a positioning lookup to apply where a
	// sequence occurs. They run after the flat passes rather than in lookup
	// order — see contextpos.go for what that costs and why no font examined
	// pays it.
	if len(l.contextualPos) > 0 {
		sh.applyContextualPositioning(buf)
	}
	// Single adjustments: a glyph nudged wherever it appears.
	for i := range buf {
		if adj, ok := l.singlePos[buf[i].GID]; ok {
			buf[i].XOffset += sh.f.scale(adj.xPlacement)
			buf[i].YOffset += sh.f.scale(adj.yPlacement)
			buf[i].XAdvance += sh.f.scale(adj.xAdvance)
		}
	}
	// Cursive attachment before marks: a mark is placed relative to a base that
	// has already been moved onto the joining stroke, and doing it the other way
	// round leaves the accent where the letter used to be.
	sh.attachCursive(buf)
	sh.attachMarks(buf)
	if sh.zeroMarks == zeroMarksLate {
		sh.cancelMarkWidths(buf, false)
	}
}

// cursiveAnchors is where a glyph's connecting stroke leaves and arrives.
type cursiveAnchors struct {
	entry, exit       anchor
	hasEntry, hasExit bool
}

// attachCursive joins glyphs whose strokes are meant to connect.
//
// This is the other half of what a cursive script needs. Joining (arabic.go)
// picks the right *shape* for each position; this makes the shapes actually
// meet. The font gives each glyph an exit point and an entry point, and
// attaching them means placing the second so its entry lands exactly on the
// first's exit — horizontally by shortening the advance between them, and
// vertically by lifting one off the baseline, because a joining stroke does not
// generally leave a letter at the height it enters the next.
//
// Which one is lifted is what the RightToLeft lookup flag decides, and it is the
// only thing that flag means. Set, the last glyph of a run stays put and the
// earlier ones climb to meet it — which is what an Arabic font wants, since the
// word is read from the end this pass reaches last. Clear, the first stays and
// the rest follow. Getting it backwards keeps every joint correct relative to
// its neighbour and leaves the whole word sitting off the baseline.
//
// # Which glyph gives ground horizontally
//
// The joint is between the first glyph's exit and the second's entry whichever
// way the run is drawn, but which of the two has to move is not the same. Drawn
// left to right the first glyph is reached first, so it is cut short at its exit
// and the second is pulled back onto it. Drawn right to left the pen reaches the
// *second* glyph first — the run is reversed after this — so it is that one that
// stops at its entry, and the first that is pulled back onto it. Doing it the
// left-to-right way for a right-to-left run leaves every letter of an Arabic
// word displaced by the width of its neighbour.
func (sh shaper) attachCursive(buf []Glyph) {
	l := sh.l
	if len(l.cursive) == 0 {
		return
	}
	// A link is one joint: the two positions it connects and the height the
	// second sits at relative to the first.
	type link struct {
		from, to int
		dy       float64
	}
	var links []link

	prev := -1
	for i := range buf {
		if l.ignores(l.cursFlags, buf[i]) {
			continue
		}
		if prev >= 0 {
			a, okA := l.cursive[buf[prev].GID]
			b, okB := l.cursive[buf[i].GID]
			if okA && a.hasExit && okB && b.hasEntry {
				// The glyph the pen reaches first advances exactly to the joint,
				// and the other is pulled back so its own anchor lands there. The
				// offsets already in place are carried through: a glyph moved by
				// a single adjustment joins from where it now is.
				if sh.rtl {
					d := sh.f.scale(a.exit.x) + buf[prev].XOffset
					buf[prev].XAdvance -= d
					buf[prev].XOffset -= d
					buf[i].XAdvance = sh.f.scale(b.entry.x) + buf[i].XOffset
				} else {
					buf[prev].XAdvance = sh.f.scale(a.exit.x) + buf[prev].XOffset
					d := sh.f.scale(b.entry.x) + buf[i].XOffset
					buf[i].XAdvance -= d
					buf[i].XOffset -= d
				}
				links = append(links, link{from: prev, to: i, dy: sh.f.scale(a.exit.y - b.entry.y)})
			}
		}
		prev = i
	}

	// The heights are a chain, so they propagate from whichever end is anchored:
	// forwards from the first glyph, or backwards from the last.
	if l.cursFlags&flagRightToLeft != 0 {
		for k := len(links) - 1; k >= 0; k-- {
			buf[links[k].from].YOffset = buf[links[k].to].YOffset - links[k].dy
		}
		return
	}
	for _, ln := range links {
		buf[ln.to].YOffset = buf[ln.from].YOffset + ln.dy
	}
}

// attachMarks places every mark on the glyph it belongs to.
//
// A mark attaches to the nearest preceding glyph that is not itself a mark
// (mark-to-base), or to the nearest preceding mark (mark-to-mark), which is how
// two accents stack. Both are the same operation over different tables, so both
// are done here in one pass backwards from each mark.
//
// Cancelling the mark's own advance is not done here. It is a decision each
// script's model takes for itself, and taking it here would take it for all of
// them and at the one moment that is wrong for two — see zeroMarkWidths.
func (sh shaper) attachMarks(buf []Glyph) {
	l := sh.l
	if len(l.markGlyphs) == 0 {
		return
	}
	// Every attachment this run will make, gathered before any of them is made,
	// and then applied in the order the font states the lookups in.
	//
	// Not in buffer order, which is what this did. A mark is placed relative to
	// where its target stands *at that moment*, and a later lookup may move the
	// target afterwards — at which point the mark does not follow, because it
	// was placed already. Noto Serif Tibetan does exactly that: lookup 19
	// attaches a mark to a subjoined letter that lookup 18 has put at y -30, and
	// lookup 21 then moves that letter to y -367. The mark stays where 19 put
	// it. Placing every mark at the end instead reads the letter's final
	// position and drags the mark 337 units down with it.
	type pending struct {
		i, at, lookup int
		mark          markAnchor
		base          anchor
	}
	var todo []pending
	// The nearest glyph before i that is not a mark, carried forward rather than
	// searched for backwards at every mark. prevNonMark walked back over every
	// mark already placed, so one base carrying a long run of them read the run
	// again once per mark: "a" with sixteen thousand U+0301 after it took 1.8
	// seconds and climbed by four per doubling. A text node is untrusted and
	// that is a shape it can be handed.
	//
	// isMark and markGlyphs are different questions — a font's GDEF classes
	// answer for every glyph, and a glyph can be a mark to one lookup and the
	// base of another — so this tracks the one prevNonMark asked, and asks it
	// once per glyph instead of once per pair.
	lastNonMark := -1
	// And for mark-to-mark, the nearest glyph before i that each of its
	// subtables does not step over, carried forward the same way — see
	// markStackTracker.
	stack := newMarkStackTracker(l)
	for i := range buf {
		isMark := l.isMark(buf[i])
		if !l.markGlyphs[buf[i].GID] {
			if !isMark {
				lastNonMark = i
			}
			stack.passed(buf[i], i)
			continue
		}
		// The letter underneath, and the mark this one stacks on. The two tables
		// ask different questions and are not two attempts at one: mark-to-base
		// looks past any marks in the way, mark-to-mark looks past exactly the
		// glyphs its own lookup skips and attaches to what it lands on.
		if j := lastNonMark; j >= 0 {
			if mark, base, lookup, ok := attachmentFor(l.markBase, buf[i].GID,
				buf[j].GID, markComponent(buf, i, j)); ok {
				todo = append(todo, pending{i, j, lookup, mark, base})
			}
		}
		if mark, base, at, lookup, ok := l.markMarkAt(buf, i, stack); ok {
			todo = append(todo, pending{i, at, lookup, mark, base})
		}
		// After the glyph is dealt with, so that lastNonMark is always the
		// nearest one *before* i, which is what prevNonMark returned.
		if !isMark {
			lastNonMark = i
		}
		stack.passed(buf[i], i)
	}
	// Stable, so that two attachments a font states in one lookup are still made
	// in the order the glyphs are written in.
	//
	// The insertion sort is kept for a short list, which is every run of every
	// real document and is faster there than a sort call. Past that it is the
	// same quadratic as the walk above, on the same input: one base with a long
	// mark run gathers one attachment per mark, and a font that states them in
	// descending lookup order makes every one of them walk the whole list.
	if len(todo) <= markSortInsertionMax {
		for a := 1; a < len(todo); a++ {
			for b := a; b > 0 && todo[b].lookup < todo[b-1].lookup; b-- {
				todo[b], todo[b-1] = todo[b-1], todo[b]
			}
		}
	} else {
		sort.SliceStable(todo, func(a, b int) bool { return todo[a].lookup < todo[b].lookup })
	}
	// A mark is moved back over everything between it and its base, and that
	// stretch is read once per mark. One base carrying a long mark run makes the
	// k-th mark read k advances, which is the last of the three quadratics on
	// this path: after the other two "a" with sixteen thousand U+0301 after it
	// still climbed by 3.7 per doubling. A prefix sum answers each in constant
	// time. It is built only when there are enough attachments to pay for the
	// array, which no ordinary combining sequence reaches.
	var sums []float64
	if len(todo) > markSortInsertionMax {
		sums = advanceSums(buf)
	}
	for _, p := range todo {
		if sums != nil && p.at >= 0 {
			since := sinceFrom(sums, sh.rtl, p.i, p.at)
			if strictMarks && since != sh.advancesBetween(buf, p.i, p.at) {
				panic("attachMarks: the prefix sum disagrees with the walk")
			}
			sh.placeMarkSince(buf, p.i, p.at, p.mark.anchor, p.base, since)
			continue
		}
		sh.placeMark(buf, p.i, p.at, p.mark.anchor, p.base)
	}
}

// strictMarks turns the prefix sum's agreement with the walk it replaces into a
// panic. It is off, and it was on for a full run of the corpus suite — 6253
// reftests over real fonts and every script in them — without firing, which is
// where the sum was checked against the walk rather than assumed equal to it.
const strictMarks = false

// markSortInsertionMax is where an insertion sort over the gathered
// attachments stops being the cheaper of the two. Every combining sequence in
// ordinary text is far below it.
const markSortInsertionMax = 32

// markComponent is which part of a ligature a mark belongs to, if the thing it
// attaches to is one and the mark came from inside it. Zero otherwise, which
// anchorFor reads as "the last part".
func markComponent(buf []Glyph, i, j int) int {
	if lig := buf[i].lig; lig.id != 0 && lig.id == buf[j].lig.id {
		return lig.comp
	}
	return 0
}

// markMarkAt finds the mark that the one at i stacks on, if any.
//
// Each subtable looks back for itself, because which glyphs are in the way is
// its lookup's own statement. The Ignore bits are cleared first: they are about
// finding a *base* and would have mark-to-mark step over every mark there is,
// which is the one thing it exists to find. What is left is the mark filtering
// set and the mark attachment class, which are exactly the narrowing a font
// uses to say which marks stack on which.
func (l *layout) markMarkAt(buf []Glyph, i int, stack *markStackTracker) (mark markAnchor, base anchor, at, lookup int, ok bool) {
	matched := -1
	for k := range l.markMark {
		st := &l.markMark[k]
		if ok && st.lookup == matched {
			continue // this lookup already applied, by an earlier subtable
		}
		m, has := st.marks[buf[i].GID]
		if !has {
			continue
		}
		j := stack.nearest(k)
		if j < 0 || !l.isMark(buf[j]) {
			continue
		}
		// Two marks stack only if they belong to the same letter: the same base,
		// or the same component of the same ligature. A mark on the first half
		// of a split vowel and a mark on the second are written above the same
		// stretch of text and are not written above one another.
		//
		// The format does not say this — it is what every implementation does,
		// and without it a Khmer syllable whose split vowel 'ccmp' took apart
		// stacks a sign from one half onto a sign from the other. The exception
		// is a mark that is *itself* a ligature, whose component is zero: it
		// stands for the whole of what it was made from, so it matches anything.
		id1, id2 := buf[i].lig.id, buf[j].lig.id
		comp1, comp2 := buf[i].lig.comp, buf[j].lig.comp
		same := false
		if id1 == id2 {
			same = id1 == 0 || comp1 == comp2
		} else {
			same = (id1 != 0 && comp1 == 0) || (id2 != 0 && comp2 == 0)
		}
		if !same {
			continue
		}
		b, has := st.anchorFor(buf[j].GID, m.class, markComponent(buf, i, j))
		if !has {
			continue
		}
		mark, base, at, matched, ok = m, b, j, st.lookup, true
	}
	return mark, base, at, matched, ok
}

// markStackIgnore are the lookup flags mark-to-mark does not look back past:
// they are about finding a base, and would have it step over every mark there
// is, which is the one thing it exists to find. What is left is the mark
// filtering set and the mark attachment class.
const markStackIgnore = flagIgnoreBaseGlyphs | flagIgnoreLigatures | flagIgnoreMarks

// markStackTracker is, for each mark-to-mark subtable, the nearest glyph behind
// the one being placed that the subtable's lookup does not step over — which is
// the glyph a mark stacks on, if it is a mark.
//
// It is carried forward rather than searched for. The search walked back from
// each mark over every mark its lookup ignores, so a letter carrying a long run
// of marks outside a lookup's filtering set or attachment class read the run
// again for every mark in it: "a" with sixteen thousand U+0301 after it, under a
// lookup whose attachment class they were not in, took 2.8 seconds and climbed
// by four and a half per doubling. It is the fourth of that shape on this path;
// lastNonMark in attachMarks was the first.
//
// What a lookup with these flags steps over is only ever a mark: the filtering
// set and the attachment class narrow which *marks* it sees. So the nearest
// glyph it does not step over is whichever is nearer of the last glyph that is
// not a mark and the last mark it sees, and the second depends on the lookup
// only through its flags and set — subtables that share both share a tracker.
type markStackTracker struct {
	l *layout
	// lastNotMark is the last glyph passed that is not a mark as the flags
	// read it; keys are the distinct flag and set pairs of the subtables, keyOf
	// each subtable's among them, and lastSeen the last mark each key sees.
	lastNotMark int
	keys        []markStackKey
	keyOf       []int
	lastSeen    []int
}

type markStackKey struct{ flags, markSet int }

func newMarkStackTracker(l *layout) *markStackTracker {
	t := &markStackTracker{l: l, lastNotMark: -1, keyOf: make([]int, len(l.markMark))}
	index := map[markStackKey]int{}
	for k := range l.markMark {
		key := markStackKey{l.markMark[k].flags &^ markStackIgnore, l.markMark[k].markSet}
		at, seen := index[key]
		if !seen {
			at = len(t.keys)
			index[key] = at
			t.keys = append(t.keys, key)
			t.lastSeen = append(t.lastSeen, -1)
		}
		t.keyOf[k] = at
	}
	return t
}

// passed records a glyph the pass has finished with.
func (t *markStackTracker) passed(g Glyph, i int) {
	if len(t.keys) == 0 {
		return
	}
	if t.l.classOf(g) != classMark {
		t.lastNotMark = i
		return
	}
	for k, key := range t.keys {
		if !t.l.ignoresIn(key.flags, key.markSet, g) {
			t.lastSeen[k] = i
		}
	}
}

// nearest is the nearest glyph behind the current one that mark-to-mark
// subtable k does not step over, or -1.
func (t *markStackTracker) nearest(k int) int {
	return max(t.lastNotMark, t.lastSeen[t.keyOf[k]])
}

// anchor is a point in a glyph's own coordinate space, in font units.
type anchor struct{ x, y int }

// key2 is a glyph and a mark class, which is how a base states where marks of
// each class attach to it.
type key2 struct {
	gid   int
	class int
}

// markAttachment is one mark-attachment subtable, kept whole: which marks it
// covers, with the class and anchor of each, and where a base receives a mark
// of each class. lookup is the index of the lookup it came from, which is what
// tells subtables that are alternatives to each other from subtables that are
// applied one after another.
type markAttachment struct {
	lookup int
	// flags and markSet are the lookup's own, kept because mark-to-mark has to
	// look back past exactly the glyphs *this* lookup skips — see markMarkAt.
	// They must not go through mergedFlags, which drops the very bit that says
	// a filtering set is in use.
	flags, markSet int
	marks          map[int]markAnchor
	// bases is where a base receives a mark of each class: mark-to-base and
	// mark-to-mark. components is the same for mark-to-ligature, which states
	// one anchor per component of the ligature rather than one for the whole of
	// it. A subtable has one or the other, never both.
	bases      map[key2]anchor
	components map[key2][]anchor
}

// markAnchor is a mark's own attachment point and the class it belongs to.
// Classes let a font say that, for instance, a base's anchor for accents above
// is not the one for cedillas below.
type markAnchor struct {
	class  int
	anchor anchor
}

// singleAdjust is a GPOS type 1 adjustment: a nudge applied to a glyph wherever
// it occurs.
type singleAdjust struct {
	xPlacement, yPlacement, xAdvance int
}

// readGPOSAttachment reads the positioning lookups beyond pair kerning: single
// adjustments, cursive attachment, mark-to-base and mark-to-mark.
//
// They are read from every feature *tag* rather than from a named one, because
// attachment is not optional the way a stylistic feature is. A font that
// positions its marks through 'mark' and 'mkmk' — which is nearly all of them —
// and another that does it through a script-specific feature such as 'abvm'
// should both work, and applying an attachment that was not asked for cannot
// make text worse: a mark's place is a fact about the font, not a preference.
//
// That argument is about tags, and it survives script selection. The same
// argument for reading every *script* does not, and the selection is applied
// here for one concrete reason: the lookup flags are merged. cursFlags carries
// the RightToLeft bit, which decides which end of a joined run stays on the
// baseline, and a font with both Latin and Arabic cursive attachment states it
// one way for one and the other way for the other. Merging them sets a Latin
// word's whole cursive chain from the Arabic lookup's flag — precisely the "a
// rule meant for another script" this selection exists to stop.
func (l *layout) readGPOSAttachment(gpos []byte, idx *featureIndex) {
	// One budget for every subtable this reader may take, shared across the
	// whole table — see subtables.
	budget := subtableBudget(gpos)
	// In lookup-list order, and each lookup read once however many features name
	// it. Both matter: where two lookups place the same mark the *later* one is
	// the answer, so reading them in the order the features happen to be listed
	// in — mark, then abvm, then blwm, then mkmk — can settle it the wrong way,
	// and reading one twice would let it settle it against itself. A font states
	// its lookups in one list and their indices are the order it means.
	var order []int
	byIndex := map[int][]byte{}
	for _, tag := range idx.tags {
		if !defaultPositionFeatures[tag] {
			continue
		}
		lookups, idxs := idx.lookupsFor(tag)
		for i, lookup := range lookups {
			if _, seen := byIndex[idxs[i]]; seen {
				continue
			}
			byIndex[idxs[i]] = lookup
			order = append(order, idxs[i])
		}
	}
	sortInts(order)
	// Each mark subtable read so far, by where it sits in the table, so that a
	// subtable several lookups name is read once — see readMarkAttachment.
	read := map[int]*markAttachment{}
	for _, i := range order {
		lookup := byIndex[i]
		kind, flags, markSet, subs := subtables(lookup, 9, &budget)
		switch kind {
		case 4, 5:
			// Mark-to-base and mark-to-ligature go into one ordered set:
			// they are alternatives for the same mark, decided by which
			// lookup covers the glyph the mark is attaching to, and a
			// ligature glyph may be covered by either.
			l.readMarkAttachment(subs, flags, markSet, kind == 5, false, read)
		case 6:
			l.readMarkAttachment(subs, flags, markSet, false, true, read)
		default:
			for _, sub := range subs {
				switch kind {
				case 1:
					l.singlePosSubtable(sub)
				case 3:
					l.cursivePos(sub)
				}
			}
		}
		switch kind {
		case 3:
			l.cursFlags |= mergedFlags(flags)
		}
	}
}

// defaultPositionFeatures are the positioning features that apply without being
// asked for.
//
// A script's LangSys lists every feature *available* for that script, not every
// feature that is on: the optional ones are there to be requested, by
// font-feature-settings or by a font-variant property. Reading positioning
// lookups from all of them turns the optional ones on for every document.
//
// It is not a subtle difference. Noto Sans JP declares 'palt', proportional
// alternate widths, which is a type 1 adjustment narrowing each full-width kana
// to the width of the ink in it — that is the whole purpose of the feature, and
// it is opt-in precisely because Japanese is normally set full-width. Applied by
// default it made U+3042 971 units instead of 1000, so an ideographic space no
// longer covered the character beside it and a column of kana no longer lined up.
//
// The list is the horizontal one HarfBuzz applies: the mark features, the two
// spacing features, and the two that complex scripts state their positioning
// under. 'kern' and 'dist' are here as well as in pairFeatures because a font may
// state either as a single adjustment rather than as a pair.
var defaultPositionFeatures = map[string]bool{
	"abvm": true, // above-base marks
	"blwm": true, // below-base marks
	"curs": true, // cursive attachment
	"dist": true, // distances, which the Indic model states its spacing under
	"kern": true, // kerning
	"mark": true, // mark to base
	"mkmk": true, // mark to mark
}

// singlePosSubtable reads a GPOS type 1 subtable: one adjustment for every
// covered glyph (format 1) or one per glyph (format 2).
func (l *layout) singlePosSubtable(sub []byte) {
	if len(sub) < 6 {
		return
	}
	format := font.Be16(sub, 0)
	valueFormat := font.Be16(sub, 4)
	size := valueSize(valueFormat)
	switch format {
	case 1:
		adj := readValueRecord(sub[6:], valueFormat)
		if adj == (singleAdjust{}) {
			return
		}
		l.eachCovered(sub, font.Be16(sub, 2), func(_, gid int) bool {
			l.singlePos[gid] = adj
			return true
		})
	case 2:
		n := font.Be16(sub, 6)
		l.eachCovered(sub, font.Be16(sub, 2), func(i, gid int) bool {
			off := 8 + i*size
			if i >= n || off+size > len(sub) {
				return true
			}
			if adj := readValueRecord(sub[off:], valueFormat); adj != (singleAdjust{}) {
				l.singlePos[gid] = adj
			}
			return true
		})
	}
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

// cursivePos reads a cursive attachment subtable: an entry and an exit anchor
// for each covered glyph, either of which may be absent — a letter that begins a
// word has nothing to join back to.
func (l *layout) cursivePos(sub []byte) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return
	}
	n := font.Be16(sub, 4)
	l.eachCovered(sub, font.Be16(sub, 2), func(i, gid int) bool {
		rec := 6 + 4*i
		if i >= n || rec+4 > len(sub) {
			return true
		}
		var c cursiveAnchors
		if a, ok := readAnchor(sub, font.Be16(sub, rec)); ok {
			c.entry, c.hasEntry = a, true
		}
		if a, ok := readAnchor(sub, font.Be16(sub, rec+2)); ok {
			c.exit, c.hasExit = a, true
		}
		if c.hasEntry || c.hasExit {
			l.cursive[gid] = c
		}
		return true
	})
}

// readMarkAttachment reads all the subtables of one mark-to-base or
// mark-to-mark lookup, keeping each subtable whole.
//
// Keeping them apart is the whole point. A mark class is a number local to the
// subtable that declares it: Noto Sans has eighteen mark-to-base subtables, and
// class 0 means one thing in the one that places accents over Latin letters and
// something else entirely in each of the others. A reader that merges them into
// one table keyed by glyph and class pairs a mark's anchor from the subtable
// that covers the mark with a base's anchor from whichever subtable was read
// last — an anchor written for four particular marks, applied to all two
// hundred and fifty of them.
//
// The two kinds have the same shape — a mark array and an array of attachment
// points, one per class — and differ only in what the second array is indexed
// by, so one reader serves both.
func (l *layout) readMarkAttachment(subs [][]byte, flags, markSet int, ligature, mkmk bool,
	read map[int]*markAttachment) {

	lookup := l.markLookups
	l.markLookups++
	// A subtable this lookup names twice is kept once: within a lookup the
	// first subtable that applies wins, so the second copy can never be the
	// one that applies, and every copy kept is one more for every mark in
	// every run to be tried against.
	seen := map[int]bool{}
	for _, sub := range subs {
		// Where the subtable sits — every one runs to the end of the table —
		// and which of the two ways its base array is read, since a subtable
		// named as mark-to-ligature reads the same bytes differently.
		key := 2 * len(sub)
		if ligature {
			key++
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		// A subtable another lookup already named is not read again: the
		// anchors are the same bytes whoever names them, and only the lookup
		// they are applied under differs. Its maps are shared, and never
		// written once read.
		parsed, done := read[key]
		if !done {
			if st, ok := l.readMarkSubtable(sub, ligature); ok {
				parsed = &st
				for gid := range st.marks {
					l.markGlyphs[gid] = true
				}
			}
			read[key] = parsed
		}
		if parsed == nil {
			continue
		}
		st := *parsed
		st.lookup, st.flags, st.markSet = lookup, flags, markSet
		if mkmk {
			l.markMark = append(l.markMark, st)
		} else {
			l.markBase = append(l.markBase, st)
		}
	}
}

// readMarkSubtable reads one mark-attachment subtable.
//
// Each coverage is walked by its own records and each array read at the
// index a glyph's record gives it, so what is built is what the subtable
// states and nothing is filled in between.
func (l *layout) readMarkSubtable(sub []byte, ligature bool) (markAttachment, bool) {
	if len(sub) < 12 || font.Be16(sub, 0) != 1 {
		return markAttachment{}, false
	}
	classCount := font.Be16(sub, 6)
	markArrayOff := font.Be16(sub, 8)
	baseArrayOff := font.Be16(sub, 10)
	if classCount <= 0 || classCount > 1024 {
		return markAttachment{}, false
	}
	st := markAttachment{
		marks: map[int]markAnchor{},
		bases: map[key2]anchor{},
	}

	// The mark array: a class and an anchor for each covered mark.
	if markArrayOff > 0 && markArrayOff+2 <= len(sub) {
		ma := sub[markArrayOff:]
		n := font.Be16(ma, 0)
		l.eachCovered(sub, font.Be16(sub, 2), func(i, gid int) bool {
			rec := 2 + 4*i
			if i >= n || rec+4 > len(ma) {
				return true
			}
			class := font.Be16(ma, rec)
			a, ok := readAnchor(ma, font.Be16(ma, rec+2))
			if !ok || class >= classCount {
				return true
			}
			st.marks[gid] = markAnchor{class: class, anchor: a}
			return true
		})
	}

	switch {
	case ligature:
		l.readLigatureArray(sub, baseArrayOff, classCount, &st)
	default:
		l.readBaseArray(sub, baseArrayOff, classCount, &st)
	}
	if len(st.marks) == 0 || (len(st.bases) == 0 && len(st.components) == 0) {
		return markAttachment{}, false
	}
	return st, true
}

// readBaseArray reads a BaseArray: one anchor per class for each covered base.
func (l *layout) readBaseArray(sub []byte, off, classCount int, st *markAttachment) {
	if off <= 0 || off+2 > len(sub) {
		return
	}
	ba := sub[off:]
	n := font.Be16(ba, 0)
	l.eachCovered(sub, font.Be16(sub, 4), func(i, gid int) bool {
		if i >= n {
			return true
		}
		// A row of anchors per base, charged as a row: bases may share one.
		if !l.spend(classCount) {
			return false
		}
		for c := 0; c < classCount; c++ {
			rec := 2 + (i*classCount+c)*2
			if rec+2 > len(ba) {
				break
			}
			a, ok := readAnchor(ba, font.Be16(ba, rec))
			if !ok {
				continue
			}
			st.bases[key2{gid, c}] = a
		}
		return true
	})
}

// readLigatureArray reads a LigatureArray, which is the one thing that makes a
// mark-to-ligature subtable different from a mark-to-base one.
//
// A ligature is several letters drawn as one glyph, so a mark written under it
// has to say which of them it belongs to: a dot under the first f of "ffi" goes
// somewhere quite different from a dot under the second. The font answers by
// giving each ligature not one anchor per class but one per component per class,
// and the shaper picks the component from which part of the text the mark came
// from — which is why forming a ligature has to record that.
func (l *layout) readLigatureArray(sub []byte, off, classCount int, st *markAttachment) {
	if off <= 0 || off+2 > len(sub) {
		return
	}
	la := sub[off:]
	n := font.Be16(la, 0)
	st.components = map[key2][]anchor{}
	l.eachCovered(sub, font.Be16(sub, 4), func(i, gid int) bool {
		rec := 2 + 2*i
		if i >= n || rec+2 > len(la) {
			return true
		}
		attachOff := font.Be16(la, rec)
		if attachOff <= 0 || attachOff+2 > len(la) {
			return true
		}
		attach := la[attachOff:]
		count := font.Be16(attach, 0)
		// A ligature of more components than any font ever writes is malformed,
		// and the count is a length this would otherwise allocate from.
		if count < 1 || count > maxLigatureComponents {
			return true
		}
		// A table of anchors per ligature, charged as one: ligatures may share
		// one.
		if !l.spend(count * classCount) {
			return false
		}
		for c := 0; c < classCount; c++ {
			anchors := make([]anchor, 0, count)
			any := false
			for comp := 0; comp < count; comp++ {
				a, ok := readAnchor(attach, font.Be16(attach, 2+(comp*classCount+c)*2))
				if 2+(comp*classCount+c)*2+2 > len(attach) {
					break
				}
				anchors = append(anchors, a)
				any = any || ok
			}
			// A class the ligature says nothing about for any component is not
			// stored, so that attachmentFor can tell "no anchor" from "an
			// anchor at the origin".
			if any {
				st.components[key2{gid, c}] = anchors
			}
		}
		return true
	})
	if len(st.components) == 0 {
		st.components = nil
	}
}

// maxLigatureComponents bounds what a font may claim a ligature is made of. The
// longest anybody writes is a handful; a count near the format's ceiling is an
// allocation this would otherwise make on the strength of two untrusted bytes.
const maxLigatureComponents = 64

// attachmentFor finds where a mark meets a base, over a set of subtables read in
// lookup order.
//
// A subtable applies only when it covers *both* — the mark, so it knows the
// mark's class and its anchor, and that base for that class. Coverage of one
// without the other is a subtable that has nothing to say about this pair, and
// the search moves on.
//
// Which match wins follows how the lookups are applied. Within a lookup the
// subtables are alternatives and the first that applies is the one used; across
// lookups each runs in turn over the whole run, so a later lookup that applies
// overwrites what an earlier one placed. Hence: last applying lookup, first
// applying subtable within it.
func attachmentFor(set []markAttachment, markGID, baseGID, component int) (mark markAnchor, base anchor, lookup int, ok bool) {
	matched := -1
	for i := range set {
		st := &set[i]
		if ok && st.lookup == matched {
			continue // this lookup already applied, by an earlier subtable
		}
		m, has := st.marks[markGID]
		if !has {
			continue
		}
		b, has := st.anchorFor(baseGID, m.class, component)
		if !has {
			continue
		}
		mark, base, ok, matched = m, b, true, st.lookup
	}
	return mark, base, matched, ok
}

// anchorFor is where this subtable says a mark of a class attaches to a glyph,
// for a mark that came from a given component of it.
//
// component is 1-based and is zero for a mark that is not part of the glyph's
// own ligature — a mark written after it, or one the glyph is not a ligature
// for. Such a mark goes on the *last* component, which is what OpenType says and
// is the only sensible answer: a mark written after "ffi" belongs to the i.
func (st *markAttachment) anchorFor(gid, class, component int) (anchor, bool) {
	if st.components == nil {
		a, ok := st.bases[key2{gid, class}]
		return a, ok
	}
	anchors, ok := st.components[key2{gid, class}]
	if !ok || len(anchors) == 0 {
		return anchor{}, false
	}
	at := len(anchors) - 1
	if component > 0 && component <= len(anchors) {
		at = component - 1
	}
	return anchors[at], true
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

// isMark reports whether a glyph is a mark.
//
// GDEF's classification is the authority — it is the font saying so — and the
// mark arrays are the fallback for a font that positions marks without
// classifying them. Asking only the mark arrays gets mark-to-mark wrong: the
// first of two stacked accents is a mark that no *mark-to-mark* array lists as
// one, because in that lookup it is the base.
func (l *layout) isMark(g Glyph) bool {
	if l.glyphClass.named {
		// A font that classifies its glyphs has answered for all of them: one it
		// leaves out is not a mark, whatever else names it. Falling back to the
		// mark arrays here reads a glyph as a mark because *some* lookup places
		// it like one, and a glyph can be a mark in one lookup and the thing a
		// mark attaches to in another. Noto Sans Balinese has one — a conjunct
		// form that GDEF leaves unclassified and a mark-to-base lookup names as
		// the base — and calling it a mark hid it from the mark that belongs on
		// it.
		return l.glyphClass.of(g.GID) == classMark
	}
	// No GDEF at all: what the character said, falling back to the mark arrays
	// for a glyph that came from no character of its own — one a substitution
	// produced.
	if g.class != 0 {
		return g.class == classMark
	}
	return l.markGlyphs[g.GID]
}

// placeMark puts the mark at i against the base at j, so that their anchors
// meet.
//
// The pen is at the end of everything drawn since the base, so the advances
// between have to be taken back off — and the base's own displacement carried
// along, since a base moved by a single adjustment or lifted onto a joining
// stroke takes its accents with it.
//
// What has to be corrected for is where the pen will be when the mark is drawn,
// and that depends on which way the run is drawn. Left to right the pen has
// passed the base and everything between them, so those advances come off.
// Right to left the buffer is about to be reversed and the mark will be drawn
// *before* its base, so the same advances are still ahead of the pen and go on
// rather than off. Whether the mark's own advance is among them is decided by
// zeroMarkWidths, which may already have taken it off.
//
// It *sets* the offsets rather than adding to them, which is what the format
// says and what makes applying the same attachment twice harmless — a lookup
// both named by a feature and reached from a rule places the mark in the same
// place either time.
func (sh shaper) placeMark(buf []Glyph, i, j int, mark, base anchor) {
	sh.placeMarkSince(buf, i, j, mark, base, sh.advancesBetween(buf, i, j))
}

// advancesBetween is what stands between a base and the mark that attaches to
// it, which the mark has to be moved back over.
func (sh shaper) advancesBetween(buf []Glyph, i, j int) float64 {
	var since float64
	if sh.rtl {
		for k := j + 1; k <= i; k++ {
			since -= buf[k].XAdvance
		}
	} else {
		for k := j; k < i; k++ {
			since += buf[k].XAdvance
		}
	}
	return since
}

// placeMarkSince is placeMark once that sum is known, so that a caller placing
// many marks against one base can answer it from a prefix sum instead of
// reading the stretch again for each of them.
func (sh shaper) placeMarkSince(buf []Glyph, i, j int, mark, base anchor, since float64) {
	buf[i].XOffset = buf[j].XOffset + sh.f.scale(base.x-mark.x) - since
	buf[i].YOffset = buf[j].YOffset + sh.f.scale(base.y-mark.y)
}

// advanceSums is the running total of the advances in buf, so that what stands
// between any two glyphs is one subtraction.
//
// Sound here because placeMark writes offsets and never an advance: nothing in
// the placement loop changes what this summed.
func advanceSums(buf []Glyph) []float64 {
	sums := make([]float64, len(buf)+1)
	for k := range buf {
		sums[k+1] = sums[k] + buf[k].XAdvance
	}
	return sums
}

// sinceFrom is advancesBetween read off those sums.
func sinceFrom(sums []float64, rtl bool, i, j int) float64 {
	if rtl {
		return -(sums[i+1] - sums[j+1])
	}
	return sums[i] - sums[j]
}
