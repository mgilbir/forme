package shape

// The array a substitution pass edits, and why it is not a plain slice.
//
// A lookup that changes how many glyphs there are has to make room or close a
// hole, and the obvious way to do that — build a new slice out of the part
// before, the replacement, and the part after — copies the whole run every time.
// One ligature in a run of n glyphs is n glyphs copied; a run of "fi" is n/2
// ligatures and so n²/2. Measured on the bundled face, 16,000 characters of
// "fi" took 786 ms and 32,000 took four times that, against 10 ms for the same
// length of text with nothing to ligate.
//
// It is not a constant to be shaved. Copying only the tail rather than the whole
// buffer, or moving whichever side is shorter, halves it and leaves it
// quadratic: the work is Σ min(at, n-at) either way.
//
// # What is done instead
//
// The array is in three parts. Everything before w is settled — the pass has
// finished with it and only a backtrack will read it again. Everything from r on
// is pending. Between them is a gap, which is where the room a shrinking
// substitution gives back is kept rather than closed.
//
//	a: [ settled            |  gap  | pending                    ]
//	                        w       r
//
// A ligature at the front of pending writes its product at the *right* of the
// glyphs it consumed and moves r forward: the tail is not touched at all, and
// the cost is the size of the product rather than the size of the run. The gap
// grows by what the run lost. A substitution that lengthens the run spends the
// gap instead, and grows the array when the gap is not enough — with slack, so
// that a font taking every glyph apart does not pay for it once per glyph.
//
// The pass advances by settling: the glyphs it has finished with are moved down
// into the settled part, one memmove of what was settled rather than of the run.
// Over a whole pass that is O(n) however many substitutions happened in it, and
// the pass ends by closing the gap once.
//
// Positions inside a lookup are relative to pending, and the record some passes
// keep beside the buffer is not — see shaper.base, which is what maps one to the
// other, and backtrackPositions, which walks off the front of pending into the
// settled part where a chained rule's context is.
type runBuf struct {
	a    []Glyph
	w, r int
	// scratch is where a substitution builds what it is about to write, kept
	// across the pass so that a run of ligatures is not a run of allocations.
	// It is never what the pass reads: replace copies out of it.
	scratch []Glyph
}

// product is an empty slice with room for n glyphs, for a substitution to build
// its replacement in.
func (rb *runBuf) product(n int) []Glyph {
	if cap(rb.scratch) < n {
		rb.scratch = make([]Glyph, 0, 2*n+8)
	}
	return rb.scratch[:0]
}

// newRunBuf starts a pass over buf at a position, which is settled up to there.
func newRunBuf(buf []Glyph, at int) *runBuf {
	if at > len(buf) {
		at = len(buf)
	}
	return &runBuf{a: buf, w: at, r: at}
}

// pending is what the pass has still to look at. Lookup positions index it.
func (rb *runBuf) pending() []Glyph { return rb.a[rb.r:] }

// settled is what it has finished with, which is what a backtrack reads.
func (rb *runBuf) settled() []Glyph { return rb.a[:rb.w] }

// settle finishes with n glyphs at the front of pending.
func (rb *runBuf) settle(n int) {
	if n > len(rb.a)-rb.r {
		n = len(rb.a) - rb.r
	}
	if n <= 0 {
		return
	}
	if rb.w != rb.r {
		copy(rb.a[rb.w:rb.w+n], rb.a[rb.r:rb.r+n])
	}
	rb.w += n
	rb.r += n
}

// replace puts product where the span glyphs at pending[at:at+span] were, and
// reports the pending glyphs as they now stand.
//
// The glyphs it overwrites are read before anything is written, so a caller may
// build its product out of them.
func (rb *runBuf) replace(at, span int, product []Glyph) []Glyph {
	switch d := span - len(product); {
	case d > 0:
		// Shrinking. The product goes at the far end of what it replaces and
		// the glyphs in front of it move up, so that the run's tail — which is
		// everything this substitution is not about — stays where it is.
		copy(rb.a[rb.r+at+d:], product)
		copy(rb.a[rb.r+d:rb.r+d+at], rb.a[rb.r:rb.r+at])
		rb.r += d
	case d < 0:
		// Growing, out of the gap. The product still *ends* where what it
		// replaced ended, and the glyphs in front of it move down.
		need := -d
		rb.room(need)
		copy(rb.a[rb.r-need:rb.r-need+at], rb.a[rb.r:rb.r+at])
		copy(rb.a[rb.r-need+at:], product)
		rb.r -= need
	default:
		copy(rb.a[rb.r+at:], product)
	}
	return rb.pending()
}

// room makes the gap at least need glyphs wide.
//
// Growing takes a new array rather than moving the tail up, because the tail is
// what this whole arrangement exists not to move. The slack is the length of the
// run, so a font that takes every glyph apart grows a bounded number of times
// rather than once per glyph.
func (rb *runBuf) room(need int) {
	if rb.r-rb.w >= need {
		return
	}
	grow := need + len(rb.a)
	b := make([]Glyph, len(rb.a)+grow)
	copy(b, rb.a[:rb.w])
	copy(b[rb.r+grow:], rb.a[rb.r:])
	rb.a = b
	rb.r += grow
}

// flatten closes the gap and reports the run, which is what a pass returns.
func (rb *runBuf) flatten() []Glyph {
	if rb.w == rb.r {
		return rb.a
	}
	n := copy(rb.a[rb.w:], rb.a[rb.r:])
	rb.r = rb.w
	rb.a = rb.a[:rb.w+n]
	return rb.a
}
