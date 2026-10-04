package shape

import "github.com/mgilbir/forme/font"

// Apple's extended kerning table, kerx: the positioning of an AAT font, which
// Apple's fonts state there and not in GPOS, beside their substitutions in
// morx (morx.go).
//
// What is here is HarfBuzz's at the release the oracle is pinned to
// (hb-aat-layout-kerx-table.hh), including when it applies the table at all,
// which is HarfBuzz's plan, not a choice of the table's: a face with a kerx is
// positioned by it unless it has both GSUB and GPOS (HarfBuzz's issue 3008),
// GSUB not counting where the run's substitutions were a morx; and where GPOS
// positions the run but offers no 'kern', kerx is applied in its place. Where
// a face has a kerx its legacy kern table is never applied, and no mark loses
// its advance.
//
// A table is subtables of five formats, each for a horizontal or a vertical
// line, each kerning along it or across it:
//
//   - formats 0, 2 and 6 kern pairs of glyphs — a sorted list of pairs, a
//     two-dimensional array by class, an array by row and column index — as
//     the legacy kern table's pairs are kerned (legacykern.go): half of the
//     value on each glyph's advance, the second glyph moved back by its half,
//     each glyph paired with the next that is not a mark. A pair the
//     subtable's lookups do not both name is not kerned;
//   - format 1 is a state machine pushing glyphs onto a stack of eight and
//     popping them with a list of values, which adjust each glyph's advance
//     and offset together;
//   - format 4 is a state machine attaching the current glyph to a marked one
//     by a point on each, as a mark is attached in GPOS: by an anchor of the
//     ankr table, or by coordinates the subtable states. Its third kind,
//     points of the glyphs' outlines, HarfBuzz's own font functions cannot
//     read, and neither attaches nor moves on to a new mark;
//   - a subtable kerning across the line ties the whole run into a chain as a
//     cursive joint would, the first time one is applied, so that a glyph
//     raised carries every glyph after it.
//
// A subtable walks the run in the order it is drawn, or the other way where
// it says so; a pair subtable that says so is not applied. Kerning along the
// line is applied only where the run's kerning feature is on, and a pair
// subtable not at all where it is off.
//
// Not here: the variation tuples beyond the first value, which HarfBuzz reads
// as the first value; and the outline points of format 4, as said above.

// kerxTable is a face's kerx, read into its subtables.
type kerxTable struct {
	subtables []kerxSubtable
	numGlyphs int
	// ankr is the face's anchor point table, which format 4 reads anchors
	// from, and nil for a face with none or one HarfBuzz refuses.
	ankr []byte
}

// kerxSubtable is one subtable: the whole of it, from its header, and what its
// header says.
type kerxSubtable struct {
	t          []byte
	format     int
	coverage   uint32
	tupleCount int
	// starts is, for a state machine, which classes leave its start state
	// doing anything, and nil where it has more classes than are tracked.
	starts []bool
	// lefts and rights are, for format 0, the glyphs its pairs name on each
	// side.
	lefts, rights map[int]bool
}

// The coverage bits of a kerx subtable.
const (
	kerxVertical    = 0x80000000
	kerxCrossStream = 0x40000000
	kerxBackwards   = 0x10000000
)

// kerxHeader is the size of a subtable's header: its length, coverage and
// tuple count. A state machine's offsets are from the end of it, the pair
// formats' from its start.
const kerxHeader = 12

// readKerx reads a kerx, and nil for a face with none, one whose version is
// below 2, and one HarfBuzz's sanitizer refuses for a subtable that does not
// fit.
func readKerx(tables map[string][]byte, numGlyphs int) *kerxTable {
	b := tables["kerx"]
	if len(b) < 8 || font.Be16(b, 0) < 2 {
		return nil
	}
	k := &kerxTable{numGlyphs: numGlyphs, ankr: readAnkr(tables["ankr"])}
	at := 8
	count := int(font.Be32(b, 4))
	for i := range count {
		if len(b)-at < kerxHeader {
			return nil
		}
		n := int(font.Be32(b, at))
		if n < kerxHeader || n > len(b)-at {
			return nil
		}
		// The last subtable is read to the end of the table, as HarfBuzz
		// reads it, whatever length it states.
		end := at + n
		if i == count-1 {
			end = len(b)
		}
		coverage := font.Be32(b, at+4)
		s := kerxSubtable{
			t:          b[at:end:end],
			format:     int(coverage & 0xFF),
			coverage:   coverage,
			tupleCount: int(font.Be32(b, at+8)),
		}
		if s.format == 1 || s.format == 4 {
			if len(s.t) < kerxHeader+20 {
				return nil
			}
			m := s.machine(numGlyphs)
			if m.nClasses < 4 {
				return nil
			}
			s.starts = m.kerxStarts(s.format)
		}
		if s.format == 0 {
			s.lefts, s.rights = map[int]bool{}, map[int]bool{}
			for p := range s.pairs() {
				s.lefts[font.Be16(s.t, p)] = true
				s.rights[font.Be16(s.t, p+2)] = true
			}
		}
		k.subtables = append(k.subtables, s)
		at += n
	}
	return k
}

// readAnkr is an ankr HarfBuzz's sanitizer admits, and nil for any other.
func readAnkr(b []byte) []byte {
	if len(b) < 12 || font.Be16(b, 0) != 0 || int(font.Be32(b, 8)) > len(b) {
		return nil
	}
	return b
}

// ankrAnchor is ankr::get_anchor: a glyph's i-th anchor point, and the origin
// where the table, the glyph or the point is missing.
func ankrAnchor(t []byte, gid, i, numGlyphs int) (x, y int) {
	if t == nil {
		return 0, 0
	}
	off, ok := aatLookup(t, int(font.Be32(t, 4)), gid, numGlyphs)
	if !ok {
		return 0, 0
	}
	a := int(font.Be32(t, 8)) + off
	if a < 0 || len(t)-a < 4 || i < 0 || i >= int(font.Be32(t, a)) {
		return 0, 0
	}
	p := a + 4 + 4*i
	if len(t)-p < 4 {
		return 0, 0
	}
	return signed16(font.Be16(t, p)), signed16(font.Be16(t, p+2))
}

// machine is a state machine subtable's state table, after the header.
func (s *kerxSubtable) machine(numGlyphs int) aatMachine {
	t := s.t[kerxHeader:]
	return aatMachine{
		t: t, extended: true, size: 4, nClasses: int(font.Be32(t, 0)),
		classTable: int(font.Be32(t, 4)), stateArray: int(font.Be32(t, 8)), entryTable: int(font.Be32(t, 12)),
		entrySize: 6, numGlyphs: numGlyphs,
	}
}

// kerxStarts is collect_initial_glyphs's filter for a state machine
// subtable: the classes whose entry from the start state moves out of it,
// pushes or marks a glyph, or acts.
func (m aatMachine) kerxStarts(format int) []bool {
	if m.nClasses > aatClassesTracked {
		return nil
	}
	out := make([]bool, m.nClasses)
	for c := range out {
		next, flags, data := m.entry(0, c)
		out[c] = next != 0 || flags&0x8000 != 0 || m.data16(data, 0) != 0xFFFF
	}
	return out
}

// aatValue is Lookup<T>::get_value_or_null for values of size bytes: what a
// lookup table gives a glyph, in each of the formats HarfBuzz reads and, with
// wide, format 10's values of any size; and whether it gives one.
func aatValue(t []byte, at, gid, numGlyphs, size int, wide bool) (int, bool) {
	if at < 0 || len(t)-at < 2 {
		return 0, false
	}
	read := func(p, n int) (int, bool) {
		if p < 0 || len(t)-p < n {
			return 0, false
		}
		v := 0
		for i := range n {
			v = v<<8 | int(t[p+i])
		}
		return v, true
	}
	switch font.Be16(t, at) {
	case 0:
		if gid < 0 || gid >= numGlyphs {
			return 0, false
		}
		return read(at+2+size*gid, size)
	case 2:
		if u, ok := aatSearch(t, at, gid, 2); ok {
			return read(u+4, size)
		}
	case 4:
		if u, ok := aatSearch(t, at, gid, 2); ok {
			return read(at+font.Be16(t, u+4)+size*(gid-font.Be16(t, u+2)), size)
		}
	case 6:
		if u, ok := aatSearch(t, at, gid, 1); ok {
			return read(u+2, size)
		}
	case 8:
		if len(t)-at < 6 {
			return 0, false
		}
		first, count := font.Be16(t, at+2), font.Be16(t, at+4)
		if gid >= first && gid-first < count {
			return read(at+6+size*(gid-first), size)
		}
	case 10:
		if !wide || len(t)-at < 8 {
			return 0, false
		}
		n, first, count := font.Be16(t, at+2), font.Be16(t, at+4), font.Be16(t, at+6)
		if gid >= first && gid-first < count && n <= 4 {
			return read(at+8+n*(gid-first), n)
		}
	}
	return 0, false
}

// pairSides reports whether a glyph is in a pair subtable's first and second
// sets: the glyphs its lookups, or its pairs, name on each side.
func (s *kerxSubtable) pairSides(left, right, numGlyphs int) bool {
	t := s.t
	switch s.format {
	case 0:
		return s.lefts[left] && s.rights[right]
	case 2:
		_, l := aatValue(t, int(font.Be32(t, 16)), left, numGlyphs, 2, false)
		_, r := aatValue(t, int(font.Be32(t, 20)), right, numGlyphs, 2, false)
		return l && r
	case 6:
		size := 2
		if font.Be32(t, 12)&1 != 0 {
			size = 4
		}
		_, l := aatValue(t, int(font.Be32(t, 20)), left, numGlyphs, size, true)
		_, r := aatValue(t, int(font.Be32(t, 24)), right, numGlyphs, size, true)
		return l && r
	}
	return false
}

// pairs is where each of a format 0 subtable's pairs is, as many as the
// subtable holds.
func (s *kerxSubtable) pairs() func(yield func(int) bool) {
	return func(yield func(int) bool) {
		for i := range s.pairCount() {
			if !yield(kerxHeader + 16 + 6*i) {
				return
			}
		}
	}
}

// pairCount is how many pairs a format 0 subtable states and holds.
func (s *kerxSubtable) pairCount() int {
	if len(s.t) < kerxHeader+16 {
		return 0
	}
	n := int(font.Be32(s.t, kerxHeader))
	if room := (len(s.t) - kerxHeader - 16) / 6; n > room {
		n = room
	}
	return n
}

// pairIndex finds a format 0 pair by binary search, and is its position or
// -1.
func (s *kerxSubtable) pairIndex(left, right int) int {
	t := s.t
	pairs := kerxHeader + 16
	lo, hi := 0, s.pairCount()-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		p := pairs + 6*mid
		l, r := font.Be16(t, p), font.Be16(t, p+2)
		switch {
		case left < l || left == l && right < r:
			hi = mid - 1
		case left > l || left == l && right > r:
			lo = mid + 1
		default:
			return mid
		}
	}
	return -1
}

// kerning is a pair subtable's get_kerning, in font units: zero for a pair it
// does not name, and the first value of a variation tuple where it states
// them.
func (s *kerxSubtable) kerning(left, right, numGlyphs int) int {
	if !s.pairSides(left, right, numGlyphs) {
		return 0
	}
	t := s.t
	switch s.format {
	case 0:
		i := s.pairIndex(left, right)
		if i < 0 {
			return 0
		}
		return s.tuple(signed16(font.Be16(t, kerxHeader+16+6*i+4)), 0)
	case 2:
		l, _ := aatValue(t, int(font.Be32(t, 16)), left, numGlyphs, 2, false)
		r, _ := aatValue(t, int(font.Be32(t, 20)), right, numGlyphs, 2, false)
		p := int(font.Be32(t, 24)) + 2*(l+r)
		if p < 0 || len(t)-p < 2 {
			return 0
		}
		return s.tuple(signed16(font.Be16(t, p)), 0)
	case 6:
		long := font.Be32(t, 12)&1 != 0
		size := 2
		if long {
			size = 4
		}
		l, _ := aatValue(t, int(font.Be32(t, 20)), left, numGlyphs, size, true)
		r, _ := aatValue(t, int(font.Be32(t, 24)), right, numGlyphs, size, true)
		p := int(font.Be32(t, 28)) + size*(l+r)
		vector := int(font.Be32(t, 32))
		if p < 0 || len(t)-p < size {
			return 0
		}
		if long {
			return s.tuple(int(int32(font.Be32(t, p))), vector)
		}
		return s.tuple(signed16(font.Be16(t, p)), vector)
	}
	return 0
}

// tuple is kerxTupleKern: where a subtable states variation tuples, a value
// is the offset, from base, of its tuple, whose first value is the one taken.
func (s *kerxSubtable) tuple(v, base int) int {
	if s.tupleCount == 0 {
		return v
	}
	p := base + v
	if v < 0 || p < 0 || len(s.t)-p < 2*s.tupleCount {
		return 0
	}
	return signed16(font.Be16(s.t, p))
}

// startsAt reports whether a glyph is in a subtable's first set: one that can
// start a pair, or start the state machine.
func (s *kerxSubtable) startsAt(gid, numGlyphs int) bool {
	switch s.format {
	case 0:
		return s.lefts[gid]
	case 2:
		_, ok := aatValue(s.t, int(font.Be32(s.t, 16)), gid, numGlyphs, 2, false)
		return ok
	case 6:
		size := 2
		if font.Be32(s.t, 12)&1 != 0 {
			size = 4
		}
		_, ok := aatValue(s.t, int(font.Be32(s.t, 20)), gid, numGlyphs, size, true)
		return ok
	case 1, 4:
		m := s.machine(numGlyphs)
		if gid == aatDeletedGlyph {
			return s.starts != nil && s.starts[aatClassDeleted]
		}
		v, ok := aatLookup(m.t, m.classTable, gid, numGlyphs)
		if !ok {
			return false
		}
		return s.starts == nil || v < len(s.starts) && s.starts[v]
	}
	return false
}

// kerxRun is what applying a kerx keeps beside the run: whether the run is
// turned round, and whether anything was attached, which is what decides
// whether attachments are resolved at all, as HarfBuzz's
// HB_BUFFER_SCRATCH_FLAG_HAS_GPOS_ATTACHMENT does.
type kerxRun struct {
	reversed, attached bool
}

// reverse turns the run round, and its attachments with it, unchanged: a
// distance to the glyph attached to is not negated, as HarfBuzz does not
// negate it.
func (sh shaper) reverseRun(buf []Glyph) {
	g := sh.gp
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
		g.chain[i], g.chain[j] = g.chain[j], g.chain[i]
		g.kind[i], g.kind[j] = g.kind[j], g.kind[i]
	}
}

// applyKerx applies a face's kerx over a run, in the order its characters are
// written, subtable by subtable: KerxTable::apply. pairs says the run's
// kerning feature is on. It reports whether anything was attached.
func (sh shaper) applyKerx(buf []Glyph, pairs bool) bool {
	k := sh.f.kerx
	vertical := sh.features.Vertical
	run := &kerxRun{}
	crossed := false
	for i := range k.subtables {
		s := &k.subtables[i]
		if vertical != (s.coverage&kerxVertical != 0) {
			continue
		}
		intersects := false
		for _, g := range buf {
			if s.startsAt(g.GID, k.numGlyphs) {
				intersects = true
				break
			}
		}
		if !intersects {
			continue
		}
		reverse := (s.coverage&kerxBackwards != 0) != sh.rtl
		cross := s.coverage&kerxCrossStream != 0
		if cross && !crossed {
			// Every glyph tied to the one before it, as a cursive joint ties
			// them, in the direction the run is written; this does not by
			// itself say anything is attached.
			crossed = true
			for j := range buf {
				sh.gp.kind[j] = attachCursive
				if sh.rtl {
					sh.gp.chain[j] = 1
				} else {
					sh.gp.chain[j] = -1
				}
			}
		}
		if reverse != run.reversed {
			sh.reverseRun(buf)
			run.reversed = reverse
		}
		switch s.format {
		case 0, 2, 6:
			if pairs && s.coverage&kerxBackwards == 0 {
				sh.kernPairs(buf, s, k.numGlyphs, cross, run)
			}
		case 1:
			if pairs || cross {
				m := s.machine(k.numGlyphs)
				t := &kerxFormat1{sh: sh, s: s, m: m, actions: int(font.Be32(m.t, 16)), cross: cross, pairs: pairs, run: run}
				sh.driveKerx(buf, m, t)
			}
		case 4:
			m := s.machine(k.numGlyphs)
			flags := font.Be32(m.t, 16)
			t := &kerxFormat4{sh: sh, k: k, m: m, action: int(flags >> 30), data: int(flags & 0x00FFFFFF), run: run}
			sh.driveKerx(buf, m, t)
		}
	}
	if run.reversed {
		sh.reverseRun(buf)
	}
	return run.attached
}

// kernPairs is hb_kern_machine_t over the run: each glyph paired with the
// next that is not a mark, kerned by the subtable.
func (sh shaper) kernPairs(buf []Glyph, s *kerxSubtable, numGlyphs int, cross bool, run *kerxRun) {
	vertical := sh.features.Vertical
	for i := 0; i < len(buf); {
		sh.work().spend(1)
		j := i + 1
		for j < len(buf) && sh.l.isMark(buf[j]) {
			j++
		}
		if j >= len(buf) {
			break
		}
		if v := s.kerning(buf[i].GID, buf[j].GID, numGlyphs); v != 0 {
			first := v >> 1
			second := v - first
			switch {
			case cross && !vertical:
				buf[j].YOffset = sh.f.scale(v)
				run.attached = true
			case cross:
				buf[j].XOffset = sh.f.scale(v)
				run.attached = true
			case !vertical:
				buf[i].addXAdvance(sh.f.scale(first))
				buf[j].addXAdvance(sh.f.scale(second))
				buf[j].XOffset += sh.f.scale(second)
			default:
				buf[i].addYAdvance(sh.f.scale(first))
				buf[j].addYAdvance(sh.f.scale(second))
				buf[j].YOffset += sh.f.scale(second)
			}
		}
		i = j
	}
}

// driveKerx walks a state machine subtable over the run, in place.
func (sh shaper) driveKerx(buf []Glyph, m aatMachine, t morxTransition) {
	b := &aatBuf{info: buf, ok: true, f: sh.f, maxOps: max(len(buf)*morxOpsPerGlyph, morxOpsFloor)}
	sh.drive(b, m, 0, t)
}

// kerxFormat1 is format 1's transition: a stack of up to eight glyphs, and a
// list of values popped onto them.
type kerxFormat1 struct {
	sh           shaper
	s            *kerxSubtable
	m            aatMachine
	actions      int
	cross, pairs bool
	run          *kerxRun
	stack        [8]int
	depth        int
}

func (*kerxFormat1) inPlace() bool { return true }

func (k *kerxFormat1) transition(b *aatBuf, flags, data int) {
	if flags&0x2000 != 0 {
		k.depth = 0
	}
	if flags&0x8000 != 0 {
		if k.depth < len(k.stack) {
			k.stack[k.depth] = b.idx
			k.depth++
		} else {
			k.depth = 0
		}
	}
	index := k.m.data16(data, 0)
	if index == 0xFFFF || k.depth == 0 {
		return
	}
	tuple := max(1, k.s.tupleCount)
	// HarfBuzz reads the index as a byte offset and halves it.
	p := k.actions + 2*(index/2)
	t := k.m.t
	if p < 0 || len(t)-p < 2*k.depth*tuple {
		k.depth = 0
		return
	}
	vertical := k.sh.features.Vertical
	last := false
	for !last && k.depth > 0 {
		k.depth--
		idx := k.stack[k.depth]
		v := signed16(font.Be16(t, p))
		p += 2 * tuple
		if idx >= len(b.info) {
			continue
		}
		// The end of the list is a value that is odd.
		last = v&1 != 0
		v &^= 1
		g := &b.info[idx]
		chain, kind := &k.sh.gp.chain[idx], &k.sh.gp.kind[idx]
		switch {
		case k.cross && v == -0x8000:
			*kind, *chain = 0, 0
			if vertical {
				g.XOffset = 0
			} else {
				g.YOffset = 0
			}
		case k.cross:
			if *kind != 0 {
				if vertical {
					g.XOffset += k.sh.f.scale(v)
				} else {
					g.YOffset += k.sh.f.scale(v)
				}
				k.run.attached = true
			}
		case !k.pairs:
		case vertical:
			g.addYAdvance(k.sh.f.scale(v))
			g.YOffset += k.sh.f.scale(v)
		default:
			g.addXAdvance(k.sh.f.scale(v))
			g.XOffset += k.sh.f.scale(v)
		}
	}
}

// kerxFormat4 is format 4's transition: the current glyph attached to the
// marked one, by an anchor of each or coordinates the subtable states.
type kerxFormat4 struct {
	sh           shaper
	k            *kerxTable
	m            aatMachine
	action, data int
	run          *kerxRun
	mark         int
	markSet      bool
}

func (*kerxFormat4) inPlace() bool { return true }

func (k *kerxFormat4) transition(b *aatBuf, flags, data int) {
	if index := k.m.data16(data, 0); k.markSet && index != 0xFFFF && b.idx < len(b.info) {
		t := k.m.t
		var dx, dy int
		set := true
		switch k.action {
		case 0:
			// Points of the outlines, which HarfBuzz's own font functions do
			// not read: no attachment, and no new mark either.
			return
		case 1:
			p := k.data + 4*index
			if p < 0 || len(t)-p < 4 {
				return
			}
			mx, my := ankrAnchor(k.k.ankr, b.info[k.mark].GID, font.Be16(t, p), k.k.numGlyphs)
			cx, cy := ankrAnchor(k.k.ankr, b.info[b.idx].GID, font.Be16(t, p+2), k.k.numGlyphs)
			dx, dy = mx-cx, my-cy
		case 2:
			p := k.data + 8*index
			if p < 0 || len(t)-p < 8 {
				return
			}
			dx = signed16(font.Be16(t, p)) - signed16(font.Be16(t, p+4))
			dy = signed16(font.Be16(t, p+2)) - signed16(font.Be16(t, p+6))
		default:
			// A kind of action HarfBuzz does not know attaches the glyph
			// where it is.
			set = false
		}
		g := &b.info[b.idx]
		if set {
			g.XOffset, g.YOffset = k.sh.f.scale(dx), k.sh.f.scale(dy)
		}
		chain := k.mark - b.idx
		if k.run.reversed {
			chain = -chain
		}
		k.sh.gp.kind[b.idx], k.sh.gp.chain[b.idx] = attachMark, chain
		k.run.attached = true
	}
	if flags&0x8000 != 0 {
		k.markSet = true
		k.mark = b.idx
	}
}
