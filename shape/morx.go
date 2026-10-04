package shape

import "github.com/mgilbir/forme/font"

// Apple's extended glyph metamorphosis table, morx: the substitutions of an AAT
// font, which Apple's fonts state there and not in GSUB. Apple Color Emoji
// builds every emoji sequence — a skin tone, a family joined by zero width
// joiners, a flag, a keycap — as a ligature in its morx, and a shaper that
// reads only GSUB draws each character of the sequence on its own.
//
// What is here is HarfBuzz's AAT substitution at the release the oracle is
// pinned to (hb-aat-layout-morx-table.hh and hb-aat-layout-common.hh), which
// HarfBuzz runs, as CoreText does, wherever a font has a morx and the run is
// set across the page, GSUB or no GSUB:
//
//   - A morx is chains of subtables, each switched on or off by the chain's
//     flags. A chain runs with its default flags, changed by the features a
//     caller asks for where the face's feat table offers them: see
//     aatfeatures.go.
//   - A subtable is a lookup of glyph to glyph (noncontextual) or a finite
//     state machine walked over the run, glyph class by glyph class, whose
//     transitions rearrange glyphs, substitute them in context, form
//     ligatures, or insert glyphs. Each is applied to the run in the order its
//     coverage says.
//   - A ligature's components after the first, and a glyph a subtable names as
//     0xFFFF, are deleted: kept in the run, marked, while the subtables run,
//     and taken out with their clusters merged once they have.
//   - HarfBuzz skips a subtable whose state machine nothing in the run can
//     start, and so a subtable that acts only at the end of the text acts only
//     where the run has a glyph to start it; this does the same.
//
// The table is read as far as it is sound. A chain or subtable that does not
// fit the table refuses the whole table, as HarfBuzz's sanitizer does, and the
// run is set by GSUB instead. A state machine's states and entries are not
// checked ahead of use, as the sanitizer checks them: one that names a state,
// an entry or a glyph table past the end of the table reads as doing nothing
// there.
//
// Not here: the older mort table; kerx, AAT's positioning, and trak, its
// tracking, which HarfBuzz applies beside it; and the language tags of ltag,
// by which a chain's feature may follow the run's language.

// The values the subtables have in common.
const (
	// aatDeletedGlyph is the glyph a subtable deletes another with.
	aatDeletedGlyph = 0xFFFF
	// aatDontAdvance is the flag, in every kind of subtable, that keeps the
	// state machine on the glyph it is at.
	aatDontAdvance = 0x4000
	// The classes every state machine has: the end of the text, a glyph its
	// class table does not name, and a deleted glyph.
	aatClassEndOfText   = 0
	aatClassOutOfBounds = 1
	aatClassDeleted     = 2
	// aatMaxContext is HB_MAX_CONTEXT_LENGTH: how many components a ligature
	// remembers, and how long a stretch a rearrangement moves.
	aatMaxContext = 64
	// aatClassesTracked is hb_bit_page_t::BITS: a state machine with more
	// classes than this is taken to start at any glyph its class table names.
	aatClassesTracked = 512
	// morxOpsPerGlyph and morxOpsFloor are HB_BUFFER_MAX_OPS_FACTOR and _MIN:
	// the run's allowance for transitions that do not advance, glyphs a
	// rewind moves and glyphs inserted.
	morxOpsPerGlyph = 4096
	morxOpsFloor    = 65536
)

// The subtable kinds.
const (
	morxRearrangement = 0
	morxContextual    = 1
	morxLigature      = 2
	morxNoncontextual = 4
	morxInsertion     = 5
)

// The coverage bits of a subtable, the top byte of its coverage word.
const (
	morxVertical      = 0x80
	morxBackwards     = 0x40
	morxAllDirections = 0x20
	morxLogical       = 0x10
)

// morxTable is a face's morx, read into its chains.
type morxTable struct {
	chains    []morxChain
	numGlyphs int
}

// morxChain is a chain of subtables, the flags it runs them under by default,
// and its features: what a feature a caller asks for does to the flags.
type morxChain struct {
	defaultFlags uint32
	features     []aatChainFeature
	subtables    []morxSubtable
}

// aatChainFeature is one of a chain's features: an AAT feature type and
// setting, and the flags it turns on and leaves on where a caller asks for it.
type aatChainFeature struct {
	typ, setting    int
	enable, disable uint32
}

// morxSubtable is one subtable: its kind, its coverage bits, the flags that
// switch it on, and its body, which is the subtable past its twelve-byte
// header — for a state machine, starting at the state table, which its
// offsets are from.
type morxSubtable struct {
	kind     int
	coverage int
	flags    uint32
	body     []byte
	// starts is, for a state machine, which classes leave the start state
	// doing anything (collect_initial_glyphs), and nil where it has more
	// classes than are tracked, in which case every glyph its class table
	// names can start it.
	starts []bool
}

// readMorx reads a morx, and nil for a face with none, one whose version is
// zero, one HarfBuzz's sanitizer refuses for a chain or subtable that does not
// fit, and the one font HarfBuzz refuses by name.
func readMorx(tables map[string][]byte, numGlyphs int) *morxTable {
	b := tables["morx"]
	if len(b) < 8 || font.Be16(b, 0) == 0 {
		return nil
	}
	// HarfBuzz's issue 4108: AALMAGHRIBI.ttf, whose morx and GSUB disagree,
	// known by the lengths of its morx, GSUB and GDEF.
	if len(b) == 19892 && len(tables["GSUB"]) == 2794 && len(tables["GDEF"]) == 340 {
		return nil
	}
	m := &morxTable{numGlyphs: numGlyphs}
	at := 8
	for range font.Be32(b, 4) {
		if len(b)-at < 16 {
			return nil
		}
		length := int(font.Be32(b, at+4))
		features, count := int(font.Be32(b, at+8)), int(font.Be32(b, at+12))
		if length < 16 || length > len(b)-at || features > (length-16)/12 {
			return nil
		}
		end := at + length
		chain := morxChain{defaultFlags: font.Be32(b, at)}
		for i := range features {
			fe := at + 16 + 12*i
			chain.features = append(chain.features, aatChainFeature{
				typ: font.Be16(b, fe), setting: font.Be16(b, fe+2),
				enable: font.Be32(b, fe+4), disable: font.Be32(b, fe+8),
			})
		}
		sub := at + 16 + 12*features
		for range count {
			if end-sub < 12 {
				return nil
			}
			n := int(font.Be32(b, sub))
			if n < 12 || n > end-sub {
				return nil
			}
			coverage := font.Be32(b, sub+4)
			s := morxSubtable{
				kind:     int(coverage & 0xFF),
				coverage: int(coverage >> 24),
				flags:    font.Be32(b, sub+8),
				body:     b[sub+12 : sub+n : sub+n],
			}
			if s.kind != morxNoncontextual {
				if len(s.body) < 16 || font.Be32(s.body, 0) < 4 {
					return nil
				}
				s.starts = s.machine(numGlyphs).starts(s.kind)
			}
			chain.subtables = append(chain.subtables, s)
			sub += n
		}
		m.chains = append(m.chains, chain)
		at = end
	}
	return m
}

// aatLookup is Lookup<HBUINT16>::get_value: the value the lookup table at an
// offset in t gives a glyph, in each of the formats HarfBuzz reads — 0, a
// value per glyph; 2 and 4, segments of glyphs; 6, single glyphs; 8, a run of
// glyphs — and whether it gives one.
func aatLookup(t []byte, at, gid, numGlyphs int) (int, bool) {
	if at < 0 || len(t)-at < 2 {
		return 0, false
	}
	switch font.Be16(t, at) {
	case 0:
		p := at + 2 + 2*gid
		if gid < 0 || gid >= numGlyphs || len(t)-p < 2 {
			return 0, false
		}
		return font.Be16(t, p), true
	case 2:
		if u, ok := aatSearch(t, at, gid, 2); ok {
			return font.Be16(t, u+4), true
		}
	case 4:
		if u, ok := aatSearch(t, at, gid, 2); ok {
			p := at + font.Be16(t, u+4) + 2*(gid-font.Be16(t, u+2))
			if len(t)-p >= 2 {
				return font.Be16(t, p), true
			}
		}
	case 6:
		if u, ok := aatSearch(t, at, gid, 1); ok {
			return font.Be16(t, u+2), true
		}
	case 8:
		if len(t)-at < 6 {
			return 0, false
		}
		first, count := font.Be16(t, at+2), font.Be16(t, at+4)
		p := at + 6 + 2*(gid-first)
		if gid >= first && gid-first < count && len(t)-p >= 2 {
			return font.Be16(t, p), true
		}
	}
	return 0, false
}

// aatSearch is VarSizedBinSearchArrayOf::bsearch over a lookup's units: the
// unit whose glyphs hold gid, by the last and first glyph a segment states (or
// the one glyph a single unit does, where terms is one), less a last unit
// whose first terms words are 0xFFFF, which ends the search.
func aatSearch(t []byte, at, gid, terms int) (int, bool) {
	if len(t)-at < 12 {
		return 0, false
	}
	size, n := font.Be16(t, at+2), font.Be16(t, at+4)
	data := at + 12
	if size < 2*terms+2 || n == 0 {
		return 0, false
	}
	if room := (len(t) - data) / size; n > room {
		n = room
	}
	if n > 0 {
		last := data + (n-1)*size
		terminator := true
		for i := range terms {
			terminator = terminator && font.Be16(t, last+2*i) == 0xFFFF
		}
		if terminator {
			n--
		}
	}
	lo, hi := 0, n-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		u := data + mid*size
		lastGlyph, firstGlyph := font.Be16(t, u), font.Be16(t, u)
		if terms == 2 {
			firstGlyph = font.Be16(t, u+2)
		}
		switch {
		case gid < firstGlyph:
			hi = mid - 1
		case gid > lastGlyph:
			lo = mid + 1
		default:
			return u, true
		}
	}
	return 0, false
}

// aatMachine is an extended state table: its classes, and where its class
// table, state array and entries are in the subtable's body.
type aatMachine struct {
	t                                  []byte
	nClasses                           int
	classTable, stateArray, entryTable int
	entrySize, numGlyphs               int
}

// The size of each kind's entry: a new state and flags, then its data.
var aatEntrySize = [...]int{
	morxRearrangement: 4,
	morxContextual:    8,
	morxLigature:      6,
	3:                 4,
	morxNoncontextual: 4,
	morxInsertion:     8,
}

func (s *morxSubtable) machine(numGlyphs int) aatMachine {
	t := s.body
	return aatMachine{
		t: t, nClasses: int(font.Be32(t, 0)),
		classTable: int(font.Be32(t, 4)), stateArray: int(font.Be32(t, 8)), entryTable: int(font.Be32(t, 12)),
		entrySize: aatEntrySize[s.kind%len(aatEntrySize)], numGlyphs: numGlyphs,
	}
}

// class is a glyph's class: the class table's, the out-of-bounds class for a
// glyph it does not name, and the deleted class for a deleted glyph.
func (m aatMachine) class(gid int) int {
	if gid == aatDeletedGlyph {
		return aatClassDeleted
	}
	if v, ok := aatLookup(m.t, m.classTable, gid, m.numGlyphs); ok {
		return v
	}
	return aatClassOutOfBounds
}

// entry is the entry a state and class lead to: the state it moves to, its
// flags, and where its data is in the body. A class past the machine's is
// read as out of bounds; an entry the table does not hold reads as zeros.
func (m aatMachine) entry(state, class int) (next, flags, data int) {
	if class >= m.nClasses {
		class = aatClassOutOfBounds
	}
	p := m.stateArray + 2*(state*m.nClasses+class)
	if state < 0 || p < 0 || len(m.t)-p < 2 {
		return 0, 0, -1
	}
	e := m.entryTable + font.Be16(m.t, p)*m.entrySize
	if e < 0 || len(m.t)-e < m.entrySize {
		return 0, 0, -1
	}
	return font.Be16(m.t, e), font.Be16(m.t, e+2), e + 4
}

// data16 is the i-th word of an entry's data, and 0xFFFF — no action — for
// an entry the table does not hold.
func (m aatMachine) data16(data, i int) int {
	if data < 0 {
		return 0xFFFF
	}
	return font.Be16(m.t, data+2*i)
}

// starts is collect_initial_glyphs's filter: the classes whose entry from the
// start state moves out of it, or acts, or begins an action. nil where the
// machine has more classes than are tracked.
func (m aatMachine) starts(kind int) []bool {
	if m.nClasses > aatClassesTracked {
		return nil
	}
	out := make([]bool, m.nClasses)
	for c := range out {
		next, flags, data := m.entry(0, c)
		out[c] = next != 0 || morxInitiates(kind, flags) || morxActs(kind, m, flags, data)
	}
	return out
}

// morxInitiates is a kind's is_action_initiable: whether an entry begins
// what a later one acts on — marks a glyph, or pushes a component.
func morxInitiates(kind, flags int) bool {
	return flags&0x8000 != 0
}

// morxActs is a kind's is_actionable: whether an entry does anything to the
// glyphs.
func morxActs(kind int, m aatMachine, flags, data int) bool {
	switch kind {
	case morxRearrangement:
		return flags&0xF != 0
	case morxContextual:
		return m.data16(data, 0) != 0xFFFF || m.data16(data, 1) != 0xFFFF
	case morxLigature:
		return flags&0x2000 != 0
	case morxInsertion:
		return flags&(0x3E0|0x1F) != 0 && (m.data16(data, 0) != 0xFFFF || m.data16(data, 1) != 0xFFFF)
	}
	return false
}

// startsAt reports whether a glyph is in the subtable's initial glyph set: one
// whose class leaves the start state doing something, or, for a lookup, one
// the lookup names.
func (s *morxSubtable) startsAt(gid, numGlyphs int) bool {
	if s.kind == morxNoncontextual {
		_, ok := aatLookup(s.body, 0, gid, numGlyphs)
		return ok
	}
	m := s.machine(numGlyphs)
	if gid == aatDeletedGlyph {
		return s.starts != nil && aatClassDeleted < len(s.starts) && s.starts[aatClassDeleted]
	}
	v, ok := aatLookup(m.t, m.classTable, gid, numGlyphs)
	if !ok {
		return false
	}
	if s.starts == nil {
		return true
	}
	return v < len(s.starts) && s.starts[v]
}

// aatBuf is the run as HarfBuzz's buffer holds it while a state machine walks
// it: the glyphs, an output the subtables that change the run's length write
// into, the position, and the allowance.
type aatBuf struct {
	info, out  []Glyph
	idx        int
	haveOutput bool
	maxOps     int
	ok         bool
	f          *Face
	// seen is every glyph the run has held since the morx began, which is
	// what HarfBuzz asks whether a subtable can start in a run of four or
	// more; nil for a shorter run, which is asked itself.
	seen map[int]bool
	// hasDeleted says a glyph was deleted, so that they are taken out.
	hasDeleted bool
}

func (b *aatBuf) cur() *Glyph { return &b.info[b.idx] }

func (b *aatBuf) clearOutput() {
	b.haveOutput = true
	b.out = b.out[:0]
}

func (b *aatBuf) nextGlyph() {
	if b.haveOutput {
		b.out = append(b.out, b.info[b.idx])
	}
	b.idx++
}

func (b *aatBuf) copyGlyph() { b.out = append(b.out, b.info[b.idx]) }

func (b *aatBuf) skipGlyph() { b.idx++ }

// sync ends the output: the rest of the run is moved into it, and it becomes
// the run. A buffer the allowance ran out in keeps the run as it was.
func (b *aatBuf) sync() {
	if b.ok {
		b.out = append(b.out, b.info[b.idx:]...)
		b.info, b.out = b.out, b.info[:0]
	}
	b.haveOutput = false
	b.out = b.out[:0]
	b.idx = 0
}

func (b *aatBuf) backtrackLen() int {
	if b.haveOutput {
		return len(b.out)
	}
	return b.idx
}

// moveTo is hb_buffer_t::move_to: to a position in the output, moving glyphs
// from the run into it, or back out of it into the run.
func (b *aatBuf) moveTo(i int) bool {
	if !b.haveOutput {
		b.idx = i
		return true
	}
	if !b.ok {
		return false
	}
	switch n := len(b.out); {
	case n < i:
		count := i - n
		if b.maxOps -= count; b.maxOps < 0 || count > len(b.info)-b.idx {
			b.ok = false
			return false
		}
		b.out = append(b.out, b.info[b.idx:b.idx+count]...)
		b.idx += count
	case n > i:
		count := n - i
		if b.maxOps -= count; b.maxOps < 0 {
			b.ok = false
			return false
		}
		if b.idx < count {
			// shift_forward: room in front of the position.
			grow := count - b.idx
			b.info = append(b.info[:b.idx], append(make([]Glyph, grow), b.info[b.idx:]...)...)
			b.idx += grow
		}
		b.idx -= count
		copy(b.info[b.idx:], b.out[i:])
		b.out = b.out[:i]
	}
	return true
}

// outputGlyph writes a glyph into the output as a copy of the one at the
// position, or of the last written at the end of the run.
func (b *aatBuf) outputGlyph(gid int) {
	var g Glyph
	if b.idx < len(b.info) {
		g = b.info[b.idx]
	} else {
		g = b.out[len(b.out)-1]
	}
	b.setGlyph(&g, gid)
	b.out = append(b.out, g)
}

// replaceGlyph is hb_aat_apply_context_t::replace_glyph: the glyph at the
// position, as another, into the output.
func (b *aatBuf) replaceGlyph(gid int) {
	g := b.info[b.idx]
	b.setGlyph(&g, gid)
	b.out = append(b.out, g)
	b.idx++
}

// replaceInPlace is replace_glyph_inplace, for the subtables that change no
// glyph's position.
func (b *aatBuf) replaceInPlace(i, gid int) { b.setGlyph(&b.info[i], gid) }

// setGlyph makes a glyph another: its advance the new glyph's, and deleted
// where it is the deleted glyph. A glyph once deleted stays so, whatever it is
// made after: HarfBuzz keeps the flag with the character's properties, which
// nothing a substitution does reaches.
func (b *aatBuf) setGlyph(g *Glyph, gid int) {
	if gid == aatDeletedGlyph {
		g.aatDeleted = true
		b.hasDeleted = true
	}
	if b.seen != nil {
		b.seen[gid] = true
	}
	g.GID = gid
	g.substituted = true
	g.setNominalXAdvance(b.f.advanceGID(gid))
}

// mergeClusters is hb_buffer_t::merge_clusters over the run: the glyphs from
// start to end, and those either side sharing a cluster with them, made one
// cluster, continuing into the output where they reach the position.
func (b *aatBuf) mergeClusters(start, end int) {
	if end-start < 2 {
		return
	}
	if b.maxOps -= end - start; b.maxOps < 0 {
		b.ok = false
	}
	info := b.info
	cluster := info[start].Cluster
	for i := start + 1; i < end; i++ {
		cluster = min(cluster, info[i].Cluster)
	}
	if cluster != info[end-1].Cluster {
		for end < len(info) && info[end-1].Cluster == info[end].Cluster {
			end++
		}
	}
	if cluster != info[start].Cluster {
		for b.idx < start && info[start-1].Cluster == info[start].Cluster {
			start--
		}
	}
	if b.idx == start && info[start].Cluster != cluster {
		for i := len(b.out); i > 0 && b.out[i-1].Cluster == info[start].Cluster; i-- {
			b.out[i-1].Cluster = cluster
		}
	}
	for i := start; i < end; i++ {
		info[i].Cluster = cluster
	}
}

// mergeOutClusters is merge_out_clusters: mergeClusters over the output,
// continuing into the run where it reaches the end of the output.
func (b *aatBuf) mergeOutClusters(start, end int) {
	if end-start < 2 {
		return
	}
	if b.maxOps -= end - start; b.maxOps < 0 {
		b.ok = false
	}
	out := b.out
	cluster := out[start].Cluster
	for i := start + 1; i < end; i++ {
		cluster = min(cluster, out[i].Cluster)
	}
	for start > 0 && out[start-1].Cluster == out[start].Cluster {
		start--
	}
	for end < len(out) && out[end-1].Cluster == out[end].Cluster {
		end++
	}
	if end == len(out) {
		for i := b.idx; i < len(b.info) && b.info[i].Cluster == out[end-1].Cluster; i++ {
			b.info[i].Cluster = cluster
		}
	}
	for i := start; i < end; i++ {
		out[i].Cluster = cluster
	}
}

// reverse reverses the run, for a subtable that walks it the other way.
func (b *aatBuf) reverse() {
	for i, j := 0, len(b.info)-1; i < j; i, j = i+1, j-1 {
		b.info[i], b.info[j] = b.info[j], b.info[i]
	}
}

// removeDeleted is delete_glyphs_inplace: the deleted glyphs taken out of the
// run, each one's cluster merged into its neighbour's where no other glyph of
// it is left.
func removeDeleted(info []Glyph) []Glyph {
	j := 0
	for i := range info {
		if !info[i].aatDeleted {
			info[j] = info[i]
			j++
			continue
		}
		cluster := info[i].Cluster
		if i+1 < len(info) && cluster == info[i+1].Cluster {
			continue
		}
		if j > 0 {
			if cluster < info[j-1].Cluster {
				old := info[j-1].Cluster
				for k := j; k > 0 && info[k-1].Cluster == old; k-- {
					info[k-1].Cluster = cluster
				}
			}
			continue
		}
		if i+1 < len(info) {
			// merge_clusters (i, i + 2), with nothing before i.
			c := min(cluster, info[i+1].Cluster)
			end := i + 2
			if c != info[i+1].Cluster {
				for end < len(info) && info[end-1].Cluster == info[end].Cluster {
					end++
				}
			}
			for k := i; k < end; k++ {
				info[k].Cluster = c
			}
		}
	}
	return info[:j]
}

// applyMorx runs a face's morx over a run in the order its characters are
// written: each chain with the flags the features a caller asked for leave it
// (see aatfeatures.go), each of its subtables for the run's direction. rtl says
// the run is set right to left, which decides which way a subtable that walks
// the run in layout order walks it.
func (sh shaper) applyMorx(buf []Glyph, rtl, vertical bool, user []userFeature) []Glyph {
	m := sh.f.morx
	settings := sh.f.aatSettings(user)
	b := &aatBuf{info: buf, ok: true, f: sh.f, maxOps: max(len(buf)*morxOpsPerGlyph, morxOpsFloor)}
	if len(buf) >= 4 {
		b.seen = map[int]bool{}
		for _, g := range buf {
			b.seen[g.GID] = true
		}
	}
	reversed := false
	for _, chain := range m.chains {
		flags := chain.flagsFor(settings)
		for i := range chain.subtables {
			s := &chain.subtables[i]
			if s.flags&flags == 0 {
				continue
			}
			if s.coverage&morxAllDirections == 0 && vertical != (s.coverage&morxVertical != 0) {
				continue
			}
			if !b.intersects(s, m.numGlyphs) {
				continue
			}
			backwards := s.coverage&morxBackwards != 0
			reverse := backwards != rtl
			if s.coverage&morxLogical != 0 {
				reverse = backwards
			}
			if reverse != reversed {
				b.reverse()
				reversed = reverse
			}
			sh.applyMorxSubtable(b, s, m.numGlyphs)
			if !b.ok {
				break
			}
		}
		if !b.ok {
			break
		}
	}
	if reversed {
		b.reverse()
	}
	if b.hasDeleted {
		b.info = removeDeleted(b.info)
	}
	return b.info
}

// intersects is buffer_intersects_machine: whether any glyph of the run, or
// of every glyph it has held where the run is four or more, can start the
// subtable.
func (b *aatBuf) intersects(s *morxSubtable, numGlyphs int) bool {
	if b.seen != nil {
		for gid := range b.seen {
			if s.startsAt(gid, numGlyphs) {
				return true
			}
		}
		return false
	}
	for _, g := range b.info {
		if s.startsAt(g.GID, numGlyphs) {
			return true
		}
	}
	return false
}

// applyMorxSubtable runs one subtable over the run.
func (sh shaper) applyMorxSubtable(b *aatBuf, s *morxSubtable, numGlyphs int) {
	if s.kind == morxNoncontextual {
		for i := range b.info {
			sh.work().spend(1)
			if v, ok := aatLookup(s.body, 0, b.info[i].GID, numGlyphs); ok {
				b.replaceInPlace(i, v)
			}
		}
		return
	}
	m := s.machine(numGlyphs)
	var t morxTransition
	switch s.kind {
	case morxRearrangement:
		t = &aatRearrange{}
	case morxContextual:
		t = &aatContextual{m: m, subs: int(font.Be32(m.t, 16))}
	case morxLigature:
		t = &aatLigature{m: m, actions: int(font.Be32(m.t, 16)), components: int(font.Be32(m.t, 20)), ligatures: int(font.Be32(m.t, 24))}
	case morxInsertion:
		t = &aatInsertion{m: m, glyphs: int(font.Be32(m.t, 16))}
	default:
		return
	}
	sh.drive(b, m, s.kind, t)
}

// morxTransition is a kind of state machine's transition: what an entry does
// to the run, and whether it edits the run where it stands or writes an
// output.
type morxTransition interface {
	inPlace() bool
	transition(b *aatBuf, flags, data int)
}

// drive is StateTableDriver::drive: the state machine walked over the run,
// glyph by glyph and then the end of the text, each transition's entry handed
// to t, staying on a glyph where the entry says not to advance — while the
// allowance lasts.
func (sh shaper) drive(b *aatBuf, m aatMachine, kind int, t morxTransition) {
	if !t.inPlace() {
		b.clearOutput()
	}
	state := 0
	for b.idx = 0; b.ok; {
		sh.work().spend(1)
		class := aatClassEndOfText
		if b.idx < len(b.info) {
			class = m.class(b.info[b.idx].GID)
		}
		next, flags, data := m.entry(state, class)
		t.transition(b, flags, data)
		state = next
		if b.idx >= len(b.info) {
			break
		}
		if flags&aatDontAdvance == 0 {
			b.nextGlyph()
		} else {
			if b.maxOps <= 0 {
				b.nextGlyph()
			}
			b.maxOps--
		}
		sh.work().size(len(b.out) + len(b.info) - b.idx)
	}
	if !t.inPlace() {
		b.sync()
	}
}

// rearrangement is the rearrangement subtable: a stretch of the run, marked
// at its first and last glyph, with up to two glyphs at either end moved to
// the other, by one of fifteen verbs.
type aatRearrange struct{ start, end int }

func (*aatRearrange) inPlace() bool { return true }

// rearrangeVerbs is how many glyphs each verb moves from the start of the
// stretch to its end, in the high nibble, and from the end to the start, in
// the low one; three is two, swapped.
var rearrangeVerbs = [16]uint8{
	0x00, 0x10, 0x01, 0x11, 0x20, 0x30, 0x02, 0x03,
	0x12, 0x13, 0x21, 0x31, 0x22, 0x32, 0x23, 0x33,
}

func (r *aatRearrange) transition(b *aatBuf, flags, _ int) {
	if flags&0x8000 != 0 {
		r.start = b.idx
	}
	if flags&0x2000 != 0 {
		r.end = min(b.idx+1, len(b.info))
	}
	if flags&0xF == 0 || r.start >= r.end {
		return
	}
	v := rearrangeVerbs[flags&0xF]
	l, rr := min(2, int(v>>4)), min(2, int(v&0xF))
	reverseL, reverseR := v>>4 == 3, v&0xF == 3
	if r.end-r.start < l+rr || r.end-r.start > aatMaxContext {
		return
	}
	b.mergeClusters(r.start, min(b.idx+1, len(b.info)))
	b.mergeClusters(r.start, r.end)
	info := b.info
	var front, back [2]Glyph
	copy(front[:l], info[r.start:r.start+l])
	copy(back[:rr], info[r.end-rr:r.end])
	if l != rr {
		copy(info[r.start+rr:], info[r.start+l:r.end-rr])
	}
	copy(info[r.start:], back[:rr])
	copy(info[r.end-l:], front[:l])
	if reverseL {
		info[r.end-1], info[r.end-2] = info[r.end-2], info[r.end-1]
	}
	if reverseR {
		info[r.start], info[r.start+1] = info[r.start+1], info[r.start]
	}
}

// contextual is the contextual subtable: the marked glyph and the current
// one each replaced through a lookup an entry names.
type aatContextual struct {
	m       aatMachine
	subs    int
	mark    int
	markSet bool
}

func (*aatContextual) inPlace() bool { return true }

// lookup is where the i-th of the subtable's substitution lookups is.
func (c *aatContextual) lookup(i int) int {
	p := c.subs + 4*i
	if c.subs <= 0 || p < 0 || len(c.m.t)-p < 4 {
		return -1
	}
	return c.subs + int(font.Be32(c.m.t, p))
}

func (c *aatContextual) transition(b *aatBuf, flags, data int) {
	// CoreText applies neither substitution at the end of the text where no
	// glyph was marked.
	if b.idx == len(b.info) && !c.markSet {
		return
	}
	if mark := c.m.data16(data, 0); mark != 0xFFFF && c.mark < len(b.info) {
		if v, ok := aatLookup(c.m.t, c.lookup(mark), b.info[c.mark].GID, c.m.numGlyphs); ok {
			b.replaceInPlace(c.mark, v)
		}
	}
	at := min(b.idx, len(b.info)-1)
	if cur := c.m.data16(data, 1); cur != 0xFFFF && at >= 0 {
		if v, ok := aatLookup(c.m.t, c.lookup(cur), b.info[at].GID, c.m.numGlyphs); ok {
			b.replaceInPlace(at, v)
		}
	}
	if flags&0x8000 != 0 {
		c.markSet = true
		c.mark = b.idx
	}
}

// ligature is the ligature subtable: glyphs pushed as components, then an
// action list that pops them, sums an index into the component table from
// each, and stores the ligature that index names in place of the first.
type aatLigature struct {
	m                              aatMachine
	actions, components, ligatures int
	positions                      [aatMaxContext]int
	matched                        int
}

func (*aatLigature) inPlace() bool { return false }

func (l *aatLigature) transition(b *aatBuf, flags, data int) {
	if flags&0x8000 != 0 {
		// Never the same position twice, where the entry did not advance.
		if l.matched > 0 && l.positions[(l.matched-1)%aatMaxContext] == len(b.out) {
			l.matched--
		}
		l.positions[l.matched%aatMaxContext] = len(b.out)
		l.matched++
	}
	if flags&0x2000 == 0 {
		return
	}
	end := len(b.out)
	if l.matched == 0 || b.idx >= len(b.info) {
		return
	}
	t := l.m.t
	cursor := l.matched
	action := l.actions + 4*l.m.data16(data, 0)
	index := 0
	for {
		if cursor == 0 {
			// The stack is empty: start it again.
			l.matched = 0
			break
		}
		cursor--
		if !b.moveTo(l.positions[cursor%aatMaxContext]) {
			return
		}
		if action < 0 || len(t)-action < 4 {
			break
		}
		a := font.Be32(t, action)
		offset := int32(a & 0x3FFFFFFF)
		if offset&0x20000000 != 0 {
			offset |= -0x40000000 // sign-extended from thirty bits
		}
		component := l.components + 2*(b.cur().GID+int(offset))
		if component < 0 || len(t)-component < 2 {
			break
		}
		index += font.Be16(t, component)
		if a&0xC0000000 != 0 {
			lig := l.ligatures + 2*index
			if lig < 0 || len(t)-lig < 2 {
				break
			}
			b.replaceGlyph(font.Be16(t, lig))
			ligEnd := l.positions[(l.matched-1)%aatMaxContext] + 1
			// Every component after the first is deleted.
			for l.matched-1 > cursor {
				l.matched--
				if !b.moveTo(l.positions[l.matched%aatMaxContext]) {
					return
				}
				b.cur().aatDeleted = true
				b.hasDeleted = true
				b.replaceGlyph(aatDeletedGlyph)
			}
			if !b.moveTo(ligEnd) {
				return
			}
			b.mergeOutClusters(l.positions[cursor%aatMaxContext], len(b.out))
		}
		action += 4
		if a&0x80000000 != 0 {
			break
		}
	}
	b.moveTo(end)
}

// insertion is the insertion subtable: glyphs from its list inserted before
// or after the marked glyph and the current one.
type aatInsertion struct {
	m      aatMachine
	glyphs int
	mark   int
}

func (*aatInsertion) inPlace() bool { return false }

// insert writes count glyphs from the subtable's list, from start, before or
// after the glyph at the position — output_glyphs — and reports how many it
// wrote: none where the list does not hold them all.
func (n *aatInsertion) insert(b *aatBuf, start, count int, before bool) int {
	t := n.m.t
	p := n.glyphs + 2*start
	if n.glyphs <= 0 || p < 0 || len(t)-p < 2*count {
		count = 0
	}
	if b.idx < len(b.info) && !before {
		b.copyGlyph()
	}
	for i := range count {
		gid := font.Be16(t, p+2*i)
		if b.idx == len(b.info) {
			b.outputGlyph(gid)
			continue
		}
		// Deleting marks the glyph at the position as well as its copy.
		if gid == aatDeletedGlyph {
			b.cur().aatDeleted = true
			b.hasDeleted = true
		}
		b.outputGlyph(gid)
	}
	if b.idx < len(b.info) && !before {
		b.skipGlyph()
	}
	return count
}

func (n *aatInsertion) transition(b *aatBuf, flags, data int) {
	markLoc := len(b.out)
	if marked := n.m.data16(data, 1); marked != 0xFFFF {
		count := flags & 0x1F
		if b.maxOps -= count; b.maxOps <= 0 {
			return
		}
		end := len(b.out)
		if !b.moveTo(n.mark) {
			return
		}
		count = n.insert(b, marked, count, flags&0x0400 != 0)
		if !b.moveTo(end + count) {
			return
		}
	}
	if flags&0x8000 != 0 {
		n.mark = markLoc
	}
	if current := n.m.data16(data, 0); current != 0xFFFF {
		count := (flags & 0x3E0) >> 5
		if b.maxOps -= count; b.maxOps <= 0 {
			return
		}
		end := len(b.out)
		count = n.insert(b, current, count, flags&0x0800 != 0)
		// Past what was inserted, or onto it where the entry does not
		// advance, so that the inserted glyphs are walked next.
		to := end + count
		if flags&aatDontAdvance != 0 {
			to = end
		}
		b.moveTo(to)
	}
}
