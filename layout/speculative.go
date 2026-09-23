package layout

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/style"
)

// Speculative layout: the passes that lay a box out to find something out, and
// then either keep the answer or throw it away.
//
// A great deal of this engine sizes things by laying them out. A flex column
// measures each item's height by laying it out at its width, then lays it out
// again at the height §9.7 settled on; a row lays its items out, finds each
// line's cross size, and lays out again the ones stretched to it; a grid lays
// every item out to size its rows and again in its cell; a clamp lays its
// content out to count the lines; a float is re-laid on every attempt at the
// line it sits on; a box that settles somewhere other than where it was
// predicted is laid out again at the place it settled. Each of those is right
// on its own. Together they had two faults, and both were the shape of the
// design rather than of any one of them.
//
// # The cost
//
// Nothing remembered a layout, so a pass that laid a box out twice laid its
// whole subtree out twice, and a subtree holding another such box paid that
// twice again. Nesting was exponential: eighteen flex columns inside one
// another took nineteen seconds, a grid the same, and the box depth cap allows
// a thousand. Tables alone memoised the answers they asked twice for, and said
// why in a comment that names the 2^n.
//
// A layout here is a function. What a box comes to is decided by the box, the
// width of its containing block, the height a percentage inside it resolves
// against and whether that height is definite, and the geometry a caller forced
// on it — and by nothing else, provided the box does not read the float
// geometry of a formatting context it shares and no line-clamp is counting its
// lines. Where those two conditions hold the answer is kept, keyed by exactly
// those inputs, and handed back the next time the same question is asked. The
// conditions are the whole of the argument and are checked where the key is
// made: see layoutKeyFor.
//
// # The side state
//
// A layout does not only return a fragment. It records the out-of-flow boxes it
// met, for placing once the tree is absolute; it records which fragment each
// positioned box became, for the boxes positioned against it; it records the
// fragments a positioned inline produced; it places floats in the formatting
// context it shares; and it charges every line it makes to the clamps in force.
// A pass that is thrown away has to take all of that back, and every site that
// threw one away took back its own selection of it by hand. The multicol
// fallback forgot the floats and the out-of-flow boxes, so a failed pour left a
// float in the context that nothing drew and the text after it indented round
// the hole. A nested clamp forgot its ancestors' counts, so a discarded
// counting pass was counted twice and content after it vanished. And nothing
// ever took back a positioned inline's fragments, so an absolutely positioned
// box inside a flex column item was placed against the fragment of a pass that
// had been discarded.
//
// So there is one checkpoint and one rollback, and they cover everything a pass
// can change. What a cached answer changed is recorded alongside it and done
// again when the answer is reused, because the second asker is owed the same
// side effects the first one got.
//
// And a layout thrown away from the middle of what was done — an atomic inline
// that no line holds, past a clamp's cut or beside its ellipsis; a flex item
// its row's stretch lays out again — is taken back from the middle with
// takeBack, because a rollback is to a point and what came after the point is
// being kept. The rule is one rule: nothing that throws layout away may leave
// what that layout did behind.

// checkpoint is every piece of per-run state a speculative pass can change,
// taken before the pass so that the pass can be taken back.
//
// What is deliberately not here is the state that is a function of the box it
// is keyed by — the memos of lengths, fonts, intrinsic widths, measured runs
// and table grids — which a discarded pass fills with the same answers the
// kept one would, and the budgets: relayouts, inlineDecorations and layouts
// count work that was *done*, and a pass that was thrown away was still done.
type checkpoint struct {
	// ctx and floats are the formatting context the pass placed floats in and
	// how many it held before. nil where the pass had a context of its own that
	// is thrown away with it.
	ctx    *floatContext
	floats int
	// frag, kids and lines are the fragment the pass appended to, where it
	// appended to one it did not make itself.
	frag        *Fragment
	kids, lines int
	// deferred is the length of the out-of-flow queue.
	deferred int
	// journal is the length of the record of writes to the positioned map and
	// the positioned inlines' fragment lists. See sideWrite.
	journal int
	// clamps is where every clamp in force had got to. Pushed and popped by the
	// pass itself, the stack is the same length again by the time a rollback
	// can happen, and what the pass changed is the counts on it.
	clamps []clampMark
}

type clampMark struct {
	c       *lineClamp
	seen    int
	reached bool
}

// checkpoint records the state a pass is about to change.
//
// ctx is the formatting context the pass places its floats in, and frag the
// fragment it appends its children and lines to; either is nil where the pass
// has none that outlives it.
func (l *layouter) checkpoint(ctx *floatContext, frag *Fragment) checkpoint {
	cp := checkpoint{ctx: ctx, frag: frag, deferred: len(l.deferred), journal: len(l.journal)}
	if ctx != nil {
		cp.floats = ctx.mark()
	}
	if frag != nil {
		cp.kids, cp.lines = len(frag.Children), len(frag.Lines)
	}
	if len(l.clamps) > 0 {
		cp.clamps = make([]clampMark, len(l.clamps))
		for i, c := range l.clamps {
			cp.clamps[i] = clampMark{c: c, seen: c.seen, reached: c.reached}
		}
	}
	return cp
}

// rollback puts everything a checkpoint recorded back as it was.
func (l *layouter) rollback(cp checkpoint) {
	if cp.ctx != nil {
		cp.ctx.truncate(cp.floats)
	}
	if cp.frag != nil {
		cp.frag.Children = cp.frag.Children[:cp.kids]
		cp.frag.Lines = cp.frag.Lines[:cp.lines]
	}
	l.deferred = l.deferred[:cp.deferred]
	for i := len(l.journal) - 1; i >= cp.journal; i-- {
		l.forget(i, nil)
	}
	l.journal = l.journal[:cp.journal]
	for _, m := range cp.clamps {
		m.c.seen, m.c.reached = m.seen, m.reached
	}
}

// sideWrite is one write layout made to a map that outlives the pass that made
// it: a positioned box's fragment, or one more fragment of a positioned inline.
//
// They are journalled rather than written directly for two reasons, which are
// the two halves of this file. A rollback has to be able to undo them, which
// needs what was there before; and a cached layout has to be able to do them
// again, which needs what was written.
type sideWrite struct {
	box  *Box
	frag *Fragment
	// inline says the write appended frag to inlineFragments[box] rather than
	// setting positioned[box].
	inline bool
	// prev and had are what positioned[box] held before, for the undo.
	prev *Fragment
	had  bool
}

// setPositioned records which fragment a positioned box became.
func (l *layouter) setPositioned(b *Box, f *Fragment) {
	prev, had := l.positioned[b]
	l.positioned[b] = f
	l.journal = append(l.journal, sideWrite{box: b, frag: f, prev: prev, had: had})
}

// addInlineFragment records one more fragment of a positioned inline box.
func (l *layouter) addInlineFragment(b *Box, f *Fragment) {
	if l.inlineFragments == nil {
		l.inlineFragments = map[*Box][]*Fragment{}
	}
	l.inlineFragments[b] = append(l.inlineFragments[b], f)
	l.journal = append(l.journal, sideWrite{box: b, frag: f, inline: true})
}

// forget undoes one journalled write, which need not be the last one made.
//
// A rollback undoes the journal from the end, and for it this is the plain
// inverse of the write. takeBack undoes a stretch from the middle, with the
// writes of whatever was laid out after it still standing, and those are why
// the two cases below are not simply "put back what was there": a positioned
// box written again later keeps the later write, which now replaces what this
// one replaced; and a positioned inline's fragment is removed by identity, not
// by being the last in its list.
//
// Neither happens in a document. The stretches taken back from the middle are
// one box's layout each, the boxes they write for are inside that box, and
// nothing laid out after it writes for those boxes. The cases are handled
// rather than assumed because a wrong undo here is a containing block silently
// answered from a fragment that is not on the page.
//
// gone says which entries after i are being taken back with it, and so are not
// the later write that stands; nil where none are.
func (l *layouter) forget(i int, gone func(k int) bool) {
	w := l.journal[i]
	if w.inline {
		frags := l.inlineFragments[w.box]
		for k := len(frags) - 1; k >= 0; k-- {
			if frags[k] == w.frag {
				frags = slices.Delete(frags, k, k+1)
				break
			}
		}
		if len(frags) == 0 {
			delete(l.inlineFragments, w.box)
		} else {
			l.inlineFragments[w.box] = frags
		}
		return
	}
	if l.positioned[w.box] != w.frag {
		for k := i + 1; k < len(l.journal); k++ {
			if gone != nil && gone(k) {
				continue
			}
			if later := &l.journal[k]; !later.inline && later.box == w.box && later.prev == w.frag {
				later.prev, later.had = w.prev, w.had
				return
			}
		}
		return
	}
	if w.had {
		l.positioned[w.box] = w.prev
		return
	}
	delete(l.positioned, w.box)
}

// sideSpan is the stretch of the out-of-flow queue and of the journal that one
// piece of layout added: from and to, as lengths before and after it.
//
// A checkpoint is a point, and a rollback takes back everything after it. That
// is the shape of most speculative passes, which throw away the last thing they
// did. It is not the shape of the two that throw away something in the middle
// of what they did: a line box that is never made leaves the atomic inlines it
// would have held laid out and unplaced, and those were laid out before the
// lines, with every line that was made after them; and a flex row lays every
// item out and then again only the ones its stretch moved. Each of those has
// what it did as a stretch rather than a suffix, and takes it back with
// takeBack.
type sideSpan struct {
	deferredFrom, deferredTo int
	journalFrom, journalTo   int
}

// sideMark is where a span begins.
func (l *layouter) sideMark() sideSpan {
	return sideSpan{deferredFrom: len(l.deferred), journalFrom: len(l.journal)}
}

// sideSince closes a span begun at s.
func (l *layouter) sideSince(s sideSpan) sideSpan {
	s.deferredTo, s.journalTo = len(l.deferred), len(l.journal)
	return s
}

// takeBack undoes what the given spans added: their out-of-flow boxes come out
// of the queue and their journalled writes are undone and come out of the
// journal, wherever in the two they are.
//
// The spans are in order and do not overlap, and none of them may begin before
// a checkpoint that is still open, since a checkpoint is a length and taking
// entries out below it would move its mark. Every caller takes back what it
// did itself after taking its own checkpoints and before anything else has.
//
// Why this exists at all is the rule the rest of the file keeps: nothing that
// throws layout away may leave what that layout did behind. An out-of-flow box
// queued by a layout nobody kept is placed against a fragment that is never
// made absolute and never painted, spends the bound on out-of-flow boxes, and
// is a side effect no kept answer can account for — see keep, which is where
// the last of those costs used to fall.
func (l *layouter) takeBack(spans []sideSpan) {
	if len(spans) == 0 {
		return
	}
	gone := func(k int) bool {
		n, _ := slices.BinarySearchFunc(spans, k, func(s sideSpan, k int) int {
			return cmp.Compare(s.journalTo, k+1)
		})
		return n < len(spans) && spans[n].journalFrom <= k
	}
	for k := len(spans) - 1; k >= 0; k-- {
		for i := spans[k].journalTo - 1; i >= spans[k].journalFrom; i-- {
			l.forget(i, gone)
		}
	}
	l.journal = cutSpans(l.journal, spans, func(s sideSpan) (int, int) { return s.journalFrom, s.journalTo })
	l.deferred = cutSpans(l.deferred, spans, func(s sideSpan) (int, int) { return s.deferredFrom, s.deferredTo })
}

// cutSpans removes the stretches of s that spans name, in one pass.
func cutSpans[T any](s []T, spans []sideSpan, bounds func(sideSpan) (int, int)) []T {
	w, _ := bounds(spans[0])
	for k, sp := range spans {
		_, to := bounds(sp)
		next := len(s)
		if k+1 < len(spans) {
			next, _ = bounds(spans[k+1])
		}
		w += copy(s[w:], s[to:next])
	}
	var zero T
	for i := w; i < len(s); i++ {
		s[i] = zero
	}
	return s[:w]
}

// layoutKey is everything a cacheable layout depends on.
//
// Every field is an input blockIn reads, and nothing blockIn reads is missing,
// under the two conditions layoutKeyFor checks: the float context is the box's
// own, and no clamp is in force. The flow's position (x, y and carriedTop) is
// not here because it only positions floats in a context the box shares, and a
// box this key is made for shares none.
type layoutKey struct {
	box        *Box
	containing style.Unit
	cbHeight   style.Unit
	cbDefinite bool
	forced     forcedGeometry
	hasForced  bool
}

// layoutKeyFor makes the key for a layout, or says the layout cannot be reused.
//
// A layout can be reused when it depends on nothing outside its arguments, and
// there are two things outside them it could depend on.
//
// One is the floats of a formatting context it shares. A box that establishes a
// formatting context of its own — a float, a flow root, a table, a cell, a flex
// or grid container, a multicol container — makes a fresh context for its
// content and never reads the one it is placed in; blockIn's "sealed" is that
// fact, and sealsFloats is the half of it that does not depend on the width.
// A caller that hands a box a fresh context of its own says so with flow.alone,
// which is what a flex item, a grid item, a table cell and an inline-block are
// given: the context is made for the call and dropped after it.
//
// The other is the clamps in force. A line made under a clamp is charged to it
// and may be where it stops, so a layout under one is a function of how many
// lines came before it anywhere in the clamp's subtree. Every caller that lays
// out something no clamp counts says so with outOfClamp, and those are the ones
// that are cached.
//
// The root is not cached because it is laid out once.
func (l *layouter) layoutKeyFor(b *Box, containing style.Unit, at flow,
	forced *forcedGeometry) (layoutKey, bool) {

	if l.noCache || len(l.clamps) > 0 || b == l.root || b.Parent == nil {
		return layoutKey{}, false
	}
	if !at.alone && !l.sealsFloats(b) {
		return layoutKey{}, false
	}
	key := layoutKey{box: b, containing: containing, cbHeight: at.cbHeight,
		cbDefinite: at.cbDefinite}
	if forced != nil {
		key.forced, key.hasForced = *forced, true
	}
	return key, true
}

// layoutEntry is one layout the cache knows about.
type layoutEntry struct {
	key layoutKey
	// asked is how many times the layout has been computed. An answer is kept
	// from the second time it is asked for, or from the first where the caller
	// said it may ask again and the box has been laid out before: most boxes
	// are laid out once, and keeping a copy of every one of them would cost a
	// copy of the document for nothing. See blockIn.
	asked int

	// frag and out are the answer, frag in a copy nothing else holds. It is
	// never handed out: every reuse gets a copy of it, because every caller
	// moves what it is given and some — a table cell's alignment, a writing
	// mode's turn — move what is inside it too.
	frag *Fragment
	out  collapsed
	// deferred and writes are the side effects the layout had, pointing into
	// frag. See replay.
	deferred []absCandidate
	writes   []sideWrite
	// weight is how much frag holds, for the bound on the cache. See
	// layoutCache.
	weight int

	// prev and next are the entry's place in the order of use, most recent at
	// the head.
	prev, next *layoutEntry
}

// layoutCache holds the answers, and bounds what it holds.
//
// The bound is on the fragments kept rather than on the entries, because an
// entry is a subtree and the subtrees nest: a flex column fifty deep keeps an
// answer at every level and each holds all the levels below it, which is
// quadratic in the depth if every one is kept. Only the most recent few are
// ever asked for again — a box is re-laid while its parent is being decided,
// and once the parent's answer is kept the child's are not needed — so the
// least recently used are let go past the bound.
//
// The bound is proportional to the largest answer kept, so that no answer is
// too large to keep and the working set of a nest is always inside it, with a
// floor so that a document of small boxes is not evicting answers it is about
// to ask for.
type layoutCache struct {
	entries    map[layoutKey]*layoutEntry
	boxes      map[*Box]bool
	head, tail *layoutEntry
	weight     int
	largest    int

	// hits, dangling, evicted and copied count what happened, for the tests: a
	// cache the corpus never hits proves nothing; a side effect pointing
	// outside the tree its layout made is one a discard forgot to take back
	// (see keep); evicted counts the answers let go past the bound, and copied
	// the fragments and lines copied into and out of the cache.
	hits, dangling, evicted, copied int
}

// minCacheWeight is the floor of the cache's bound, in fragments and lines.
//
// Both are variables so that a test can lower them and watch eviction happen:
// a bound that has only ever been observed not to trip is one nobody knows
// works.
var minCacheWeight = 1 << 16

// cacheWeightFactor is how many of the largest answer the cache keeps.
var cacheWeightFactor = 16

// blockIn is block layout with the option of having the box's width, margins and
// height decided by the caller instead of by its own declarations.
//
// The only caller that supplies them is the absolute placement of position.go,
// whose §10.3.7 constraint resolves a width against a containing block this walk
// cannot see. Routing it through the same function rather than giving it a
// layout of its own is deliberate: margin collapsing, floats, line breaking,
// list markers and the height rules are identical for an absolutely positioned
// box, and a second implementation of them would agree with this one on the day
// it was written and on no day after.
//
// Where the answer is one the cache may keep, it is asked there first. See
// layoutKeyFor for when that is, and layBlock for the layout itself.
func (l *layouter) blockIn(b *Box, containing style.Unit, at flow,
	forced *forcedGeometry) (*Fragment, collapsed) {

	key, ok := l.layoutKeyFor(b, containing, at, forced)
	if !ok {
		return l.layBlock(b, containing, at, forced)
	}
	if l.cache.entries == nil {
		l.cache.entries = map[layoutKey]*layoutEntry{}
	}
	e := l.cache.entries[key]
	if e == nil {
		e = &layoutEntry{key: key}
		l.cache.entries[key] = e
	}
	if e.frag != nil && !l.overBudget() {
		l.cache.hits++
		l.cache.touch(e)
		return l.replay(e)
	}
	e.asked++
	// The caller's word that it may ask again is taken only for a box that has
	// already been laid out under some other question. A row asks each item
	// once and again only for the ones its stretch moved, at a different size;
	// a column measures an item and then lays it out at a height it did not
	// have. Neither asks the same question twice. What repeats a question is
	// the container being laid out again by the one around it, and that is a
	// box that has been asked before. Taking the word for every item kept a
	// copy of each one's subtree at every level of a nest, and a nest of rows
	// no question was ever repeated in copied a million fragments for nothing.
	askedBefore := l.cache.boxes[b]
	if l.cache.boxes == nil {
		l.cache.boxes = map[*Box]bool{}
	}
	l.cache.boxes[b] = true
	if e.asked < 2 && !(at.again && askedBefore) {
		return l.layBlock(b, containing, at, forced)
	}
	absMark, journalMark := len(l.deferred), len(l.journal)
	f, out := l.layBlock(b, containing, at, forced)
	l.keep(e, f, out, absMark, journalMark)
	return f, out
}

// keep stores an answer that was just computed, with the side effects it had.
//
// The fragment kept is a copy taken now, before the caller moves anything in
// it. The side effects are the ones the layout left behind — the out-of-flow
// boxes after absMark and the journalled writes after journalMark, which a
// rollback inside the layout has already taken back where it threw a pass
// away — and each points at a fragment of the answer, so each is translated to
// the copy.
//
// A side effect pointing anywhere else points at a fragment this layout made
// and threw away without taking back what it did, because a layout that has a
// key has a context of its own and writes into no tree but its own. Such a
// fragment is on no page: nothing but the layout that made it ever held it,
// and it is never made absolute. So the side effect is left out of what is
// kept, and the answer is kept anyway. The box it queued is not placed with
// the cache or without it — see placeAbsolutes — and it stays in the queue of
// the pass that made it, where every enclosing answer leaves it out in turn.
//
// This used to refuse the answer instead, which is safe for the box and ruin
// for everything around it: the fragment is outside the tree of every box that
// encloses it too, so one such side effect refused every answer up to the
// root, and the nesting the cache exists for was exponential again. A clamp
// that cut an inline-grid holding an absolutely positioned box, inside
// fourteen stretched flex rows, did 65,000 units of work, reached the bound
// and lost its content. dangling counts these for the tests, which hold it at
// nothing: it is the mark of a site that throws layout away and forgets
// takeBack.
func (l *layouter) keep(e *layoutEntry, f *Fragment, out collapsed, absMark, journalMark int) {
	c := fragmentCloner{seen: map[*Fragment]*Fragment{}}
	copied := c.clone(f)
	l.chargeCopy(f.Box, c.weight)
	deferred := make([]absCandidate, 0, len(l.deferred)-absMark)
	for _, d := range l.deferred[absMark:] {
		p, ok := c.seen[d.parent]
		if !ok {
			l.cache.dangling++
			continue
		}
		d.parent = p
		deferred = append(deferred, d)
	}
	writes := make([]sideWrite, 0, len(l.journal)-journalMark)
	for _, w := range l.journal[journalMark:] {
		p, ok := c.seen[w.frag]
		if !ok {
			l.cache.dangling++
			continue
		}
		writes = append(writes, sideWrite{box: w.box, frag: p, inline: w.inline})
	}
	e.frag, e.out, e.deferred, e.writes, e.weight = copied, out, deferred, writes, c.weight
	l.cache.add(e)
}

// replay hands out a kept answer: a copy of it, with the side effects the
// layout had done again against the copy.
func (l *layouter) replay(e *layoutEntry) (*Fragment, collapsed) {
	c := fragmentCloner{seen: map[*Fragment]*Fragment{}}
	f := c.clone(e.frag)
	l.chargeCopy(f.Box, c.weight)
	for _, d := range e.deferred {
		d.parent = c.seen[d.parent]
		l.deferred = append(l.deferred, d)
	}
	for _, w := range e.writes {
		if w.inline {
			l.addInlineFragment(w.box, c.seen[w.frag])
		} else {
			l.setPositioned(w.box, c.seen[w.frag])
		}
	}
	return f, e.out
}

func (c *layoutCache) add(e *layoutEntry) {
	c.weight += e.weight
	if e.weight > c.largest {
		c.largest = e.weight
	}
	c.pushFront(e)
	limit := max(minCacheWeight, cacheWeightFactor*c.largest)
	for c.weight > limit && c.tail != nil && c.tail != e {
		c.evict(c.tail)
	}
}

func (c *layoutCache) touch(e *layoutEntry) {
	if c.head == e {
		return
	}
	c.unlink(e)
	c.pushFront(e)
}

func (c *layoutCache) evict(e *layoutEntry) {
	c.evicted++
	c.unlink(e)
	c.weight -= e.weight
	// The entry stays in the map with its count, so that the next time it is
	// asked for it is kept again at once: it has been asked for twice already.
	e.frag, e.deferred, e.writes, e.weight = nil, nil, nil, 0
}

func (c *layoutCache) pushFront(e *layoutEntry) {
	e.prev, e.next = nil, c.head
	if c.head != nil {
		c.head.prev = e
	}
	c.head = e
	if c.tail == nil {
		c.tail = e
	}
}

func (c *layoutCache) unlink(e *layoutEntry) {
	if e.prev != nil {
		e.prev.next = e.next
	} else {
		c.head = e.next
	}
	if e.next != nil {
		e.next.prev = e.prev
	} else {
		c.tail = e.prev
	}
	e.prev, e.next = nil, nil
}

// fragmentCloner copies a fragment tree deep enough that nothing done to the
// copy reaches the original, and remembers which copy each fragment became.
//
// Deep enough is every slice and every pointer layout or painting writes
// through: the children and the lines, each line's runs and inline boxes, the
// marker, and the rectangle lists. What is shared is what nothing writes to
// once made — the boxes, the faces, the images and a run's decorations, which
// are memoized and shared across runs already.
//
// A slice that was nil stays nil and one that was empty stays empty, so a copy
// is indistinguishable from the original to anything that compares them.
// TestFragmentClonerCopiesEveryField is what keeps this list complete when a
// field is added.
type fragmentCloner struct {
	seen   map[*Fragment]*Fragment
	weight int
}

func (c *fragmentCloner) clone(f *Fragment) *Fragment {
	if f == nil {
		return nil
	}
	if g, ok := c.seen[f]; ok {
		return g
	}
	g := new(Fragment)
	*g = *f
	c.seen[f] = g
	c.weight++
	if f.Children != nil {
		g.Children = make([]*Fragment, len(f.Children))
		for i, k := range f.Children {
			g.Children[i] = c.clone(k)
		}
	}
	if f.Lines != nil {
		g.Lines = make([]LineFragment, len(f.Lines))
		for i := range f.Lines {
			g.Lines[i] = c.line(f.Lines[i])
		}
	}
	if f.Marker != nil {
		m := *f.Marker
		g.Marker = &m
	}
	g.collapsed = slices.Clone(f.collapsed)
	g.background = slices.Clone(f.background)
	g.bgBands = slices.Clone(f.bgBands)
	g.canvasLayers = slices.Clone(f.canvasLayers)
	return g
}

func (c *fragmentCloner) line(in LineFragment) LineFragment {
	c.weight++
	out := in
	out.Runs = slices.Clone(in.Runs)
	if in.Boxes != nil {
		out.Boxes = make([]*Fragment, len(in.Boxes))
		for i, b := range in.Boxes {
			out.Boxes[i] = c.clone(b)
		}
	}
	return out
}

// maxLayoutWork bounds the layout work one document may ask for, as a multiple
// of its size.
//
// The cache makes the passes above linear where their questions repeat, and
// they repeat in every document written by hand. They need not repeat in one
// written to defeat it. A clamp inside a clamp is laid out in a different state
// on each of its ancestor's two passes, and so on down: a dozen nested clamps
// over a hundred lines took four and a half seconds, doubling at every level,
// and no cache answer exists because every question really is new. A box whose
// width depends on its parent's height, in a parent laid out at two heights, is
// the same shape.
//
// So the work is bounded as well, per document and in proportion to what one
// pass over it costs. The work is what layout makes — a box laid out, a line
// box built — and the size is what one pass is bounded by: a pass lays each
// box out once and makes at most a line per character. A document that reaches
// the bound has the rest of its boxes laid out empty and the rest of its lines
// left unmade, and is told so: a page with content missing and a finding,
// rather than one that does not arrive.
//
// Sixty-four passes is far above what a document needs — the deepest honest
// case is a line re-fitted inside a balanced paragraph inside a measured item,
// and each of those is a small bound of its own. A variable so that a test can
// lower it and watch it fire.
var maxLayoutWork = 64

// minLayoutWork is the floor of that bound, so that a small document is never
// near it.
var minLayoutWork = 1 << 16

// layoutWorkLimit is the bound for one document.
func layoutWorkLimit(root *Box) int {
	return max(minLayoutWork, maxLayoutWork*onePass(root))
}

// onePass is what one pass over a box tree can make at most: each box once,
// and a line box per byte of its text.
func onePass(b *Box) int {
	if b == nil {
		return 0
	}
	n := 1 + len(b.Text)
	for _, c := range b.Children {
		n += onePass(c)
	}
	return n
}

// starved charges one box layout to the document's bound, and says whether the
// bound has been reached and the box is to be laid out without its content.
func (l *layouter) starved(b *Box) bool {
	if !l.charge(b, 1) {
		return false
	}
	l.overWork.boxes++
	return true
}

// lineStarved charges one line box, and says whether the bound has been
// reached and the rest of the block's lines are not to be made.
func (l *layouter) lineStarved(b *Box) bool {
	if !l.charge(b, 1) {
		return false
	}
	l.overWork.blocks++
	return true
}

// chargeCopy charges n fragments and lines copied into or out of the cache.
//
// A copy is work the cache does in place of a layout, and it is not free: a
// kept answer is as deep as the subtree it is the answer for, and an answer
// asked for once per layout of its parent is copied once per layout of its
// parent. Where the parent is laid out a bounded number of times that is a
// cost of the kind the bound already allows. Where it is not — a clamp inside
// a clamp, re-laid at every level and every time asking for the same large
// item — nothing else bounds it, because a reuse is not a layout and was not
// charged: the bound on layouts held and the copying went on.
//
// A copy is charged at a fraction of a layout, because it is a fraction of
// one: a fragment copied is a struct and its slices, a fragment laid out is a
// style resolved, text broken and shaped and a position decided. The fraction
// is measured — see copiesPerUnit.
func (l *layouter) chargeCopy(b *Box, n int) {
	l.cache.copied += n
	l.copyCredit += n
	for l.copyCredit >= copiesPerUnit {
		l.copyCredit -= copiesPerUnit
		l.charge(b, 1)
	}
}

// copiesPerUnit is how many fragments and lines copied are charged as one unit
// of layout work.
var copiesPerUnit = 16

// overBudget says the bound has been reached, so that the cache hands out no
// more copies: past it a box is laid out empty whether or not an answer for it
// is kept, which is what stops the copying where the bound stops the layouts.
func (l *layouter) overBudget() bool {
	return l.work >= l.workLimit
}

// charge adds n units to the document's work and says whether it went past the
// bound. The first to go past is remembered for the finding; see
// reportOverWork.
func (l *layouter) charge(b *Box, n int) bool {
	l.work += n
	if l.work <= l.workLimit {
		return false
	}
	if l.overWork.first == nil {
		l.overWork.first = b
	}
	return true
}

// overWork is what the bound on layout work stopped: where it was reached, how
// many boxes were laid out empty after that, and how many blocks had lines
// left unmade.
type overWork struct {
	first         *Box
	boxes, blocks int
}

// reportOverWork says, once and after the layout, what the bound stopped.
//
// Once rather than at every refusal, and after rather than at the first,
// because what the reader needs is the whole of what is missing and that is
// not known until the walk is over: the first refusal might be a line and
// every one after it a box, and a finding written at the first said only
// "lines", which was not the half that emptied the page.
func (l *layouter) reportOverWork() {
	o := l.overWork
	if o.first == nil {
		return
	}
	var lost []string
	if o.boxes > 0 {
		lost = append(lost, strconv.Itoa(o.boxes)+" "+choosePlural(o.boxes, "box was", "boxes were")+
			" laid out without content")
	}
	if o.blocks > 0 {
		lost = append(lost, strconv.Itoa(o.blocks)+" "+choosePlural(o.blocks, "block was", "blocks were")+
			" cut short with lines still to make")
	}
	what := "nothing was left out"
	if len(lost) > 0 {
		what = strings.Join(lost, " and ") + ", and that content is not on the page"
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleLimit,
		Source: AtHTML(offsetOf(o.first)),
		Message: "laying this document out took more than " + strconv.Itoa(l.workLimit) +
			" box layouts and lines, counting copies of reused layouts at " +
			strconv.Itoa(copiesPerUnit) + " to one, which is more than this engine will do " +
			"for a document of its size; after that " + what,
		Path: PathOf(o.first.Element),
	})
}

func choosePlural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
