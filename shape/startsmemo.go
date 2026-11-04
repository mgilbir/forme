package shape

import (
	"sync"
	"sync/atomic"
)

// startsMemo is what an AAT subtable's startsAt has answered, two bits a
// glyph — not yet asked, no, or yes — so that a glyph is asked once for the
// face and not at every run that holds it. Before a morx or kerx subtable is
// applied, each glyph of the run is asked whether it can start it; for a state
// machine that is a search of its class table and a read of its start state,
// for a lookup a search of the lookup. That was from a tenth to a seventh of
// the time a line took in the four faces with a morx in Google Fonts that were
// measured, and nearly a tenth in Apple's kerx faces, where many subtables
// are asked about every glyph of a run and find none that starts them.
//
// It is the face's, as the table is, and so a clone's too, which may shape at
// the same time as the face it was made from: the words are set by atomic
// OR, which is safe because an answer, once stated, is the same whoever states
// it.
//
// Its words are taken from the table's startsMemoAllowance the first time the
// subtable is asked about, and a subtable that finds too little left is
// asked as before, every time. What is charged to the run is unchanged: a
// glyph asked is a glyph asked, answered from the memo or not.
type startsMemo struct {
	once  sync.Once
	words []atomic.Uint64
}

// startsMemoAllowance is how many words of startsMemo a morx or kerx of n
// bytes may take, for all its subtables: its own size again, and 128 KiB. A
// memo is a quarter of a byte a glyph a subtable, and a font states its
// subtables in its bytes; the largest morx in the font corpora, Siemreap's,
// takes 32 KiB of memo for 90 KiB of table, and a table of twelve-byte
// subtables stating 65,535 glyphs would otherwise take 16 KiB of memo for
// each.
func startsMemoAllowance(n int) int64 { return int64(n/8) + 1<<14 }

// starts is startsAt, the subtable's own answer to whether glyph gid can start
// it, answered from the memo where it can be. left is what the table's
// allowance still has, from which the memo is made the first time it is asked.
func (m *startsMemo) starts(left *atomic.Int64, numGlyphs, gid int, startsAt func(gid, numGlyphs int) bool) bool {
	words := m.take(left, numGlyphs)
	if words == nil || gid < 0 || gid >= numGlyphs {
		return startsAt(gid, numGlyphs)
	}
	w, shift := &words[gid>>5], uint(gid&31)*2
	switch (w.Load() >> shift) & 3 {
	case 1:
		return false
	case 2:
		return true
	}
	ok := startsAt(gid, numGlyphs)
	state := uint64(1)
	if ok {
		state = 2
	}
	w.Or(state << shift)
	return ok
}

// take is the memo's words, made the first time they are asked for, and nil
// where left has too little for them, or for a subtable with no memo.
func (m *startsMemo) take(left *atomic.Int64, numGlyphs int) []atomic.Uint64 {
	if m == nil || startsMemoOff {
		return nil
	}
	m.once.Do(func() {
		n := int64(numGlyphs+31) / 32
		if left.Add(-n) < 0 {
			left.Add(n)
			return
		}
		m.words = make([]atomic.Uint64, n)
	})
	return m.words
}

// startsMemoOff turns every memo off, so that a test can shape the same text
// with and without and require the two to agree. Nothing else sets it.
var startsMemoOff bool
