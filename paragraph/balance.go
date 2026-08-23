package paragraph

import "github.com/mgilbir/forme/style"

// Choosing between the ways a paragraph could be broken.
//
// CSS Text §5.1's "text-wrap-style: balance" and CSS Overflow 4's line-clamp are
// the same shape of problem: break the paragraph many times over and keep the
// arrangement that reads best, under a constraint that must not be traded away —
// the line count for balancing, how far the reader gets for a clamp.
//
// Three searches, because the constraint differs and so does what can be
// reached. Filling greedily in a narrower measure cannot produce every
// arrangement once a float has made the lines different lengths, which is what
// the scored search is for.

// maxClampLines bounds the count an untrusted stylesheet can state.
//
// The number is only ever compared against a line count, so a large one clamps
// nothing — but it is parsed from a document and multiplied by nothing, and a
// bound costs one line. It is far above any block a reader would call clamped.
const maxClampLines = 1 << 20

// MaxBalanceLines bounds how many lines this engine will balance.
//
// Balancing costs a binary search over the width, and each probe breaks the
// whole paragraph again — so a page of running prose set to "text-wrap: balance"
// would be laid out sixteen times over. CSS Text §5.1 allows the bound in as
// many words ("UAs may disable balancing when the number of lines exceeds some
// threshold"), and balancing is a display effect: it is what a headline of two
// or three lines is for, and nobody can see it in a paragraph of thirty.
//
// It is a variable so that a test can lower it far enough to watch it decide
// something. A bound that has only ever been observed not to trip is one nobody
// knows works.
var MaxBalanceLines = 6

// BalanceWidth is CSS Text §5.1's "text-wrap-style: balance", computed as the
// narrowest width that still fits the text in the same number of lines.
//
//	balance: Line breaks are chosen to balance the remaining (empty) space in
//	each line box, if a better balance than block-progression-first filling is
//	possible.
//
// The specification gives no algorithm, and the one below is the one every
// implementation uses, because its two statements turn out to be the same: a
// greedy break at the narrowest width that still makes N lines is the greedy
// break whose longest line is as short as it can be, which is exactly "the
// remaining space is as even as it can be made". "The quickest brown fox jumped
// over the lazy dog" in thirty-five characters greedily fills the first line to
// thirty-three and leaves twelve on the second; the narrowest width that still
// takes two lines is twenty-four, and there it reads "The quickest brown fox /
// jumped over the lazy dog", which is what the suite's text-wrap-balance-003
// draws with an explicit <br>.
//
// The search needs the count to fall as the width grows, and it does: a wider
// line takes at least what a narrower one took.
//
// Returns MaxUnit — no cap at all — when the box does not balance, when it is
// one line already, or when it is longer than this engine will balance.
func (br *Breaker) BalanceWidth(items []Item, width, indent style.Unit) style.Unit {
	full := br.countLines(items, width, indent, MaxBalanceLines+1)
	if full < 2 || full > MaxBalanceLines {
		return style.MaxUnit
	}
	// One unit is the finest distinction the geometry can hold, so the search
	// stops when the bracket is that wide and there is nothing left to choose
	// between.
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		if br.countLines(items, mid, indent, full+1) <= full {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// capAt is the balanced width for a line beginning at an item.
func capAt(caps []style.Unit, i int) style.Unit {
	if i < 0 || i >= len(caps) {
		return style.MaxUnit
	}
	return caps[i]
}

// BalanceClampedWidth is §5.1's balancing where CSS Overflow 4's clamp has
// already cut the block off: the narrowest width that still shows everything the
// full width showed.
//
// The suite states the rule as a picture rather than as prose, and both of its
// halves are in the diagrams. line-clamp-002 balances "1 2 3 4 5 6 7 8 9 0 1 2"
// into two lines of thirteen characters where the second carries a four-
// character ellipsis — so the ellipsis is part of what is being evened out, not
// something added afterwards. And line-clamp-003 shows *more* text balanced than
// unbalanced: three lines of "1 2 3", "4 5 6", "7 8 9…" against an unbalanced
// "1 2 3 4 5", "6 7 8 9", "…", because the narrower measure lets the last line
// hold something beside the mark.
//
// So the search is over how far into the content the clamped layout reaches,
// and the answer is the narrowest width that reaches as far as the full width
// did. Reaching *further* is fine and is what the third line above does.
func (br *Breaker) BalanceClampedWidth(items []Item,
	width, indent, ellipsis style.Unit, maxLines int) style.Unit {

	wantI, wantByte := br.clampedReach(items, width, indent, ellipsis, maxLines)
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		i, iByte := br.clampedReach(items, mid, indent, ellipsis, maxLines)
		if i > wantI || (i == wantI && iByte >= wantByte) {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// clampedReach is how far into the items a clamped block gets: the cursor after
// the last line it shows.
//
// The last line is the one the ellipsis sits on, so it is broken in a narrower
// measure than the rest — and it is the one line that does not overflow. A word
// too long for its line is set anyway everywhere else in this engine, because
// the alternative is losing it; here the alternative is exactly what the clamp
// asks for, since what does not fit beside the mark is what the mark stands for.
// "unbreakable" against nine characters less an ellipsis shows nothing at all,
// which is what the suite's line-clamp-003 draws.
func (br *Breaker) clampedReach(items []Item,
	width, indent, ellipsis style.Unit, maxLines int) (int, int) {

	i, iByte := 0, 0
	for n := 0; n < maxLines; n++ {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := width
		if n == 0 {
			room = room.Sub(indent)
		}
		last := n == maxLines-1
		if last {
			room = room.Sub(ellipsis)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, _ := br.BreakOneLine(items, i, iByte, room, 0)
		if last {
			var used style.Unit
			for _, r := range runs {
				used = used.Add(r.Width)
			}
			if used > room {
				// The breaker only overflows when a single unit left it no
				// choice, so a line wider than its room is one unit that did not
				// fit — and on the clamped line that unit is not shown.
				break
			}
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			break
		}
	}
	return i, iByte
}

// BalanceWidthInBands is §5.1's balancing where a float has shortened some of
// the lines: the same search, over the widths the lines actually had.
//
// The bands come from laying the box out once, which is the only way to know
// them — a float inside the box is placed as the lines are built, and what
// shortens a line is decided by the lines above it. They are the *greedy*
// layout's bands and the balanced one may differ slightly, since a line that
// changes height meets a different set of floats; the difference is a line's
// worth of a float's edge, and browsers make the same approximation.
func (br *Breaker) BalanceWidthInBands(items []Item, bands []style.Unit,
	width, indent style.Unit) style.Unit {

	full := br.countLinesInBands(items, bands, width, indent, MaxBalanceLines+1)
	if full < 2 || full > MaxBalanceLines {
		return style.MaxUnit
	}
	lo, hi := style.Unit(1), width
	for hi.Sub(lo) > 1 {
		mid := lo.Add(hi.Sub(lo).Div(2))
		if br.countLinesInBands(items, bands, mid, indent, full+1) <= full {
			hi = mid
			continue
		}
		lo = mid
	}
	return hi
}

// countLinesInBands is countLines with a width per line rather than one for all
// of them.
//
// A line's room is the narrower of the band it is in and the width being
// probed — the cap chooses break points inside the room the floats leave, it
// does not widen a line past them.
func (br *Breaker) countLinesInBands(items []Item, bands []style.Unit,
	cap, indent style.Unit, limit int) int {

	n := 0
	iByte := 0
	for i := 0; i < len(items); {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := style.Min(bandAt(bands, n), cap)
		if n == 0 {
			room = room.Sub(indent)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, forced := br.BreakOneLine(items, i, iByte, room, 0)
		if len(runs) > 0 || forced {
			n++
		}
		if n >= limit {
			return n
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			break
		}
	}
	return n
}

// MaxBalancePasses bounds how many times a balanced box beside a float is laid
// out.
//
// Each pass measures the widths the last one produced and balances in them, and
// the two agree after one round for every box whose floats are all above its
// text. Where a float sits part-way down, moving the lines can move the float
// and the answer chases itself; three attempts is where that stops. It is a
// variable so that a test can lower it and watch the bound decide something.
var MaxBalancePasses = 3

// SameUnits reports whether two runs of measurements are the same.
func SameUnits(a, b []style.Unit) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// LineCap is the width this line may be broken at.
//
// Two sources, and the per-line one wins where it has an answer: the scored
// search names a width for each line, and the width search names one for the
// whole box. A line past the end of either is not capped by it.
func LineCap(perItem, perLine []style.Unit, item, line int) style.Unit {
	if line >= 0 && line < len(perLine) {
		return perLine[line]
	}
	if len(perLine) > 0 {
		return style.MaxUnit
	}
	return capAt(perItem, item)
}

// bandAt is the width of the nth line, or of the last one recorded once the
// probe runs past what was measured.
//
// A probe that makes more lines than the layout did is asking about lines that
// were never laid out, and the band below the last float is the best answer
// there is: it is what every line after it had.
func bandAt(bands []style.Unit, n int) style.Unit {
	if len(bands) == 0 {
		return style.MaxUnit
	}
	if n >= len(bands) {
		return bands[len(bands)-1]
	}
	return bands[n]
}

// maxScoredItems bounds the paragraph the scored search will look at.
//
// The search is quadratic in the break opportunities — every position is a state
// and every state enumerates the lines that can start at it — so a long
// paragraph is left to the width search, which is linear and gives the same
// answer wherever the lines all have the same room. Six lines of ordinary prose
// is well under this; a paragraph that is not is one nobody can see the
// balancing of anyway.
var maxScoredItems = 400

// BalanceScoredCaps is §5.1's balancing as a choice between break sets rather
// than as a narrower measure to fill greedily in.
//
// The two are the same question when every line has the same room, and they part
// when a float shortens some of them. The suite's text-wrap-balance-float-001 is
// the case: three lines with sixteen and a half characters of room, sixteen and
// a half, and twenty-three and a half. Filling greedily in a narrower measure can
// reach two arrangements there and no others — thirteen, twelve and eight
// characters, or thirteen, sixteen and four — while the reference is nine, seven
// and seventeen. What that one minimises is the sum of the squares of the space
// left over: 188.75 against 272.75 and 392.75.
//
// So that is what is minimised, over break sets that make the same number of
// lines. The count is a constraint rather than part of the score because
// balancing may not cost a line — a paragraph that grew one to look tidier would
// be balancing at the expense of the thing being balanced.
//
// The square is what makes it *balance* rather than merely fit: it charges one
// line left half empty far more than two lines left a quarter empty each, which
// is the difference a reader sees.
//
// Returns the width to break each line at, or nil where there is nothing to
// choose or too much to choose between.
func (br *Breaker) BalanceScoredCaps(items []Item, bands []style.Unit,
	indent style.Unit, lines int) []style.Unit {

	if lines < 2 || lines > MaxBalanceLines || len(items) > maxScoredItems {
		return nil
	}

	type state struct {
		i, iByte, n int
	}
	type answer struct {
		score float64
		used  style.Unit
		// at is the width to break this line at to get this line back, which is
		// the width it was *found* at rather than the width it fills.
		//
		// The two are not the same, and the difference is the white space at the
		// end of a line. §4.1.2 takes it off before the line is measured, so it
		// does not count towards what the line fills — but whether it is on the
		// line at all still depends on the room, because a space that will be
		// trimmed is still a space the breaker had to fit. "0" followed by three
		// spaces fills one character and consumes four in a wide enough line and
		// two in a line one character wide, and both of those are the same
		// "used".
		//
		// The old answer was the used width, and the fuzzer found both shapes it
		// gets wrong: a first line of nothing but spaces, whose used width is
		// zero and which then breaks after one space instead of after all of
		// them; and a line whose trailing spaces are dropped when it is re-broken
		// at what it filled. Each costs a line, and balancing may not cost a
		// line — that is the one thing the count is a constraint for.
		//
		// Nothing wants the tight width. A cap is a *break* width and only that:
		// the caller takes the narrower of it and the band and breaks there, so
		// a cap equal to the band means "this line was found greedily", which is
		// true and is what should happen.
		at         style.Unit
		next       state
		ok, walked bool
	}
	memo := map[state]answer{}

	room := func(n int) style.Unit {
		r := bandAt(bands, n)
		if n == 0 {
			r = r.Sub(indent)
		}
		return r
	}

	var best func(st state) answer
	best = func(st state) answer {
		if got, ok := memo[st]; ok {
			return got
		}
		// Guard against a cycle: a state is marked as being worked on, and a
		// candidate that leads back to it is refused rather than followed. The
		// enumeration below only ever moves the cursor forward, so this cannot
		// fire — it is here because the alternative to refusing is a hang on an
		// untrusted document.
		memo[st] = answer{}
		if st.i >= len(items) {
			out := answer{ok: st.n == lines}
			memo[st] = out
			return out
		}
		if st.n >= lines {
			memo[st] = answer{}
			return answer{}
		}

		out := answer{}
		r := room(st.n)
		for w := r; w >= 0; {
			runs, next, nextByte, _, _ := br.BreakOneLine(items, st.i, st.iByte, w, 0)
			if !CursorAdvanced(st.i, st.iByte, next, nextByte) {
				break
			}
			var used style.Unit
			for _, run := range runs {
				used = used.Add(run.Width)
			}
			rest := best(state{next, nextByte, st.n + 1})
			if rest.ok {
				slack := float64(r.Sub(used).Px())
				score := slack*slack + rest.score
				if !out.ok || score <= out.score {
					out = answer{
						score: score, used: used, at: w,
						next: state{next, nextByte, st.n + 1}, ok: true,
					}
				}
			}
			// The next candidate is the widest line strictly shorter than this
			// one, which is this one measured a layout unit narrower. The
			// minimum is what keeps it strictly shorter: a line holding a single
			// unit too wide for it is set anyway, so its used width is *more*
			// than the width it was asked for, and stepping down from that would
			// step back up.
			shorter := style.Min(used, w).Sub(1)
			if shorter >= w || shorter < 0 {
				break
			}
			w = shorter
		}
		memo[st] = out
		return out
	}

	first := best(state{0, 0, 0})
	if !first.ok {
		return nil
	}
	caps := make([]style.Unit, 0, lines)
	for st, cur := (state{0, 0, 0}), first; cur.ok && len(caps) < lines; {
		caps = append(caps, cur.at)
		st = cur.next
		if st.i >= len(items) {
			break
		}
		cur = memo[st]
	}
	if len(caps) == 0 {
		return nil
	}
	return caps
}

// countLines is how many lines the greedy breaker makes of these items in a
// given width.
//
// It stops counting at limit, because the caller only ever needs to know whether
// the count is above a number: a two-character probe width over a page of text
// would otherwise break every word in it to answer a question already settled.
//
// Floats are not consulted. Balancing chooses break points within the room the
// content has, and what that room is on each line is decided by the real loop
// against the real bands; a count that placed floats would have to place them
// once per probe and roll them back once per probe.
func (br *Breaker) countLines(items []Item, width, indent style.Unit, limit int) int {
	n := 0
	iByte := 0
	for i := 0; i < len(items); {
		for iByte == 0 && i < len(items) && items[i].Float != nil {
			i++
		}
		if i >= len(items) {
			break
		}
		room := width
		if n == 0 {
			room = width.Sub(indent)
		}
		wasI, wasByte := i, iByte
		runs, next, nextByte, _, forced := br.BreakOneLine(items, i, iByte, room, 0)
		if len(runs) > 0 || forced {
			n++
		}
		if n >= limit {
			return n
		}
		i, iByte = next, nextByte
		if !CursorAdvanced(wasI, wasByte, i, iByte) {
			// The same forward-progress guard the real loop carries. A probe
			// width of one unit is narrower than any glyph, and a breaker that
			// cannot fit even one would otherwise be asked for ever.
			break
		}
	}
	return n
}
