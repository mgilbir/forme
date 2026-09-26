package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/mgilbir/forme/font"
)

// Renumbering a CID-keyed CFF when it is subsetted.
//
// A CID-keyed CFF is the one kind of program whose subset is renumbered: the
// glyphs kept are packed down to glyph indices 0, 1, 2… in their original
// order, and everything numbered by glyph index is rewritten to match — the
// charset, the FDSelect, the charstrings' calls into subroutine INDEXes that
// lose every subroutine no kept glyph reaches, and in the sfnt around it hmtx,
// hhea's count of metrics, maxp, cmap and post. See cffsubset.go for why, and
// for why no other program is.
//
// None of this changes a number a document holds. A CIDFontType0 is addressed
// by CID, which the charset maps to a glyph, and the charset written here gives
// each kept glyph the CID it had; a /W, a /CIDSet and the codes in a content
// stream are all keyed on CIDs (GlyphCode), and the face's own glyph indices,
// which SubsetGlyphs reports, are unchanged.

// cidCharset writes the charset of a renumbered CID-keyed CFF: cids[g] is the
// CID of the subset's glyph g. Glyph 0 is .notdef and is never listed.
//
// Format 0 is two bytes a glyph and format 2 four bytes a run of consecutive
// CIDs; the smaller is written. A few scattered ideographs are format 0, and a
// subset that keeps a block of Latin is format 2.
func cidCharset(cids []int) []byte {
	f0 := make([]byte, 1, 1+2*len(cids))
	for _, c := range cids[1:] {
		f0 = binary.BigEndian.AppendUint16(f0, uint16(c))
	}
	f2 := []byte{2}
	for i := 1; i < len(cids); {
		j := i
		for j+1 < len(cids) && cids[j+1] == cids[j]+1 && j+1-i <= 0xFFFF {
			j++
		}
		f2 = binary.BigEndian.AppendUint16(f2, uint16(cids[i]))
		f2 = binary.BigEndian.AppendUint16(f2, uint16(j-i)) // nLeft
		i = j + 1
	}
	if len(f2) < len(f0) {
		return f2
	}
	return f0
}

// fdSelectFDs reads an FDSelect — the bytes sliceFDSelect measured — into the
// Font DICT of each of n glyphs, -1 for a glyph it assigns none.
//
// A glyph left unassigned or assigned a Font DICT past the FDArray is not an
// error here: the glyph may be one the subset drops. Whoever keeps it asks.
func fdSelectFDs(b []byte, n int) []int {
	fds := make([]int, n)
	for g := range fds {
		fds[g] = -1
	}
	switch b[0] {
	case 0:
		for g := 0; g < n && 1+g < len(b); g++ {
			fds[g] = int(b[1+g])
		}
	case 3:
		ranges := int(binary.BigEndian.Uint16(b[1:]))
		for i := 0; i < ranges; i++ {
			first := int(binary.BigEndian.Uint16(b[3+3*i:]))
			fd := int(b[5+3*i])
			// The next range's first glyph, or for the last range the
			// sentinel, which sits where the next range's first would.
			next := int(binary.BigEndian.Uint16(b[3+3*(i+1):]))
			for g := first; g < next && g < n; g++ {
				fds[g] = fd
			}
		}
	}
	return fds
}

// writeFDSelect writes the FDSelect of a renumbered font: fds[g] is the Font
// DICT of the subset's glyph g. Format 3 is three bytes a run of glyphs sharing
// a Font DICT and format 0 one byte a glyph; the smaller is written.
func writeFDSelect(fds []int) []byte {
	f3 := []byte{3, 0, 0}
	ranges := 0
	for g := range fds {
		if g == 0 || fds[g] != fds[g-1] {
			f3 = binary.BigEndian.AppendUint16(f3, uint16(g))
			f3 = append(f3, byte(fds[g]))
			ranges++
		}
	}
	binary.BigEndian.PutUint16(f3[1:], uint16(ranges))
	f3 = binary.BigEndian.AppendUint16(f3, uint16(len(fds))) // sentinel
	if 1+len(fds) < len(f3) {
		f0 := make([]byte, 1, 1+len(fds))
		for _, fd := range fds {
			f0 = append(f0, byte(fd))
		}
		return f0
	}
	return f3
}

// # Dropping the subroutines only dropped glyphs used
//
// A subroutine is named in a charstring by a number pushed just before the call
// — its index less a bias that depends on how many the INDEX holds — so taking
// subroutines out of an INDEX renumbers the rest, changes the bias, and means
// rewriting that number at every call site that survives. What a call reaches
// is only known by running the charstring: the number may be pushed by the
// caller, and a hintmask's length depends on how many stems were declared
// before it, anywhere in the glyph and its subroutines. So each kept glyph is
// run, as far as knowing its operand stack's depth and which subroutines it
// calls, and every call is recorded against the bytes of the number that named
// it.
//
// Where that cannot be done exactly the subroutines are kept whole instead, as
// they were before any of this: a call whose number is not a literal written
// immediately before it (computed, or pushed by another subroutine), an
// operator whose effect on the stack is not simply to clear it (the arithmetic
// and storage operators, which no CJK face in the corpora uses), a charstring
// that runs off its INDEX, or one subroutine body whose one call site would
// need two different numbers — a global subroutine calling a local one is read
// against the local subroutines of whichever glyph is being drawn, and two
// Font DICTs can renumber theirs differently. The subset is then larger than it
// could be and exactly as correct; the glyphs are renumbered either way.

// errKeepSubrs is how the walk says the subroutines cannot be renumbered and
// have to be kept whole. It is never returned to a caller.
var errKeepSubrs = errors.New("fonts: the CFF's subroutines cannot be renumbered")

// What a charstring body is: a glyph's own, by its index in the subset, or a
// subroutine, by its index in the original INDEX — the global one, or the local
// one of a Private DICT region.
const (
	bodyGlyph = iota
	bodyGlobal
	bodyLocal
)

type csBody struct{ kind, region, index int }

// subrCall is one call site: the number naming the subroutine runs from the
// map key that holds it to end, and the call operator follows it. targets is
// every subroutine it was seen to name, which is more than one only for the
// shape described above.
type subrCall struct {
	end     int
	targets []csBody
}

// subrPruning is what the walks found: which subroutines are used, and where
// each is called from.
type subrPruning struct {
	global [][]byte
	locals [][][]byte // the local subroutines of each Private DICT region
	usedG  []bool
	usedL  [][]bool
	calls  map[csBody]map[int]*subrCall
	exits  map[subrEntry]subrExit
}

// cffOperand is a number a body pushed: its value, and — when it was written
// as a literal integer — where its bytes are.
type cffOperand struct {
	v          int
	lit        bool
	start, end int
}

func newSubrPruning(global [][]byte, locals [][][]byte) *subrPruning {
	p := &subrPruning{
		global: global,
		locals: locals,
		usedG:  make([]bool, len(global)),
		usedL:  make([][]bool, len(locals)),
		calls:  map[csBody]map[int]*subrCall{},
		exits:  map[subrEntry]subrExit{},
	}
	for r, l := range locals {
		p.usedL[r] = make([]bool, len(l))
	}
	return p
}

// walk runs the subset's glyph g, whose charstring is code and whose local
// subroutines are those of Private DICT region (-1 for none), recording the
// subroutines it calls and from where.
//
// Each operator read is charged to budget, and a budget that runs out is
// returned as its error; errKeepSubrs says this glyph cannot be read exactly
// enough.
func (p *subrPruning) walk(g int, code []byte, region int, budget *font.Budget) error {
	ops := 0
	_, _, _, err := p.run(csBody{bodyGlyph, 0, g}, code, region, 0, 0, 0, &ops, budget)
	return err
}

// subrEntry is the state a subroutine is entered in, which is everything its
// run depends on: how deep the operand stack is (a hintmask after operands
// counts them as stems, and the stack has a limit), how many stems have been
// declared (a hintmask's length), how deeply calls are nested (the limit on
// that), and whose local subroutines a call in it reaches.
type subrEntry struct {
	body                 csBody
	region               int
	depth, stems, nested int
}

// subrExit is what a subroutine run from a given subrEntry leaves behind.
type subrExit struct {
	depth, stems int
	ended        bool // it reached endchar, which ends the glyph
}

// run reads one body — a glyph's charstring or a subroutine — entered with
// depth numbers already on the stack, stems declared, and nested calls
// outstanding. It returns the stack's depth and the stems declared when the
// body returns, and whether it ended the glyph instead.
//
// A subroutine run from the same entry state as before does exactly what it
// did then, so it is not run again: its exit is remembered. A font that calls
// one subroutine from another many times over — each of a chain calling the
// next several times, which read in full is exponential in the chain's length —
// is then read in time linear in its bytes.
//
// What is charged to the budget is the subroutines' operators, which are what
// a font can make this read again and again. A glyph's own charstring is read
// once, as the subset copies it once, and is bounded by the CharStrings INDEX
// it came from; charging it as well put a whole-font subset of Noto Sans SC at
// 7.1 million units against a budget of 4.2 million, for work that is one pass
// over the font. As charged, that subset spends 1.25 million, and the most any
// Noto CJK face in the corpora spends on its whole-font subset is Noto Serif
// JP's 1.56 million, well inside the budget. The bound on one glyph,
// maxCharstringOps, counts both, and is reported as an error.
//
// The numbers on the stack at entry belong to the caller, and a call in the
// body that names its subroutine with one of them cannot be renumbered here —
// its bytes are in another body. Nor can a caller use a number a subroutine
// left behind. Both are refused (errKeepSubrs), which is also why only the
// depth is remembered and not the numbers.
func (p *subrPruning) run(body csBody, code []byte, region, depth, stems, nested int,
	count *int, budget *font.Budget) (int, int, bool, error) {

	var local [][]byte
	if region >= 0 {
		local = p.locals[region]
	}
	below := depth // numbers on the stack this body did not push
	var ops []cffOperand
	for at := 0; at < len(code); {
		if *count++; *count > maxCharstringOps {
			return 0, 0, false, fmt.Errorf("fonts: a CFF glyph runs past %d charstring "+
				"operators, subroutines included", maxCharstringOps)
		}
		if body.kind != bodyGlyph && !budget.Charge(1, "CFF subroutines, run for the calls they make") {
			return 0, 0, false, budget.Err()
		}
		v := int(code[at])
		size, val, lit := 0, 0, true
		switch {
		case v == 28:
			size = 3
			if at+size <= len(code) {
				val = int(int16(binary.BigEndian.Uint16(code[at+1:])))
			}
		case v == 255:
			size, lit = 5, false // 16.16 fixed: never a subroutine's number
		case v >= 32 && v <= 246:
			size, val = 1, v-139
		case v >= 247 && v <= 250:
			size = 2
			if at+size <= len(code) {
				val = (v-247)*256 + int(code[at+1]) + 108
			}
		case v >= 251 && v <= 254:
			size = 2
			if at+size <= len(code) {
				val = -(v-251)*256 - int(code[at+1]) - 108
			}
		}
		if size > 0 {
			if at+size > len(code) || below+len(ops) >= 48 {
				return 0, 0, false, errKeepSubrs // truncated, or past the stack's limit
			}
			ops = append(ops, cffOperand{v: val, lit: lit, start: at, end: at + size})
			at += size
			continue
		}
		switch v {
		case 10, 29: // callsubr, callgsubr
			if len(ops) == 0 || nested+1 >= maxCharstringDepth {
				return 0, 0, false, errKeepSubrs
			}
			top := ops[len(ops)-1]
			ops = ops[:len(ops)-1]
			// The number is the last this body pushed — every operator but a
			// number empties what the body has pushed — so it sits
			// immediately before the call. It has to be an integer written as
			// one: a 16.16 fixed number is not rewritten.
			if !top.lit {
				return 0, 0, false, errKeepSubrs
			}
			var target csBody
			var sub []byte
			if v == 29 {
				k := top.v + font.CFFSubrBias(len(p.global))
				if k < 0 || k >= len(p.global) {
					return 0, 0, false, errKeepSubrs
				}
				target, sub = csBody{bodyGlobal, 0, k}, p.global[k]
				p.usedG[k] = true
			} else {
				k := top.v + font.CFFSubrBias(len(local))
				if k < 0 || k >= len(local) {
					return 0, 0, false, errKeepSubrs
				}
				target, sub = csBody{bodyLocal, region, k}, local[k]
				p.usedL[region][k] = true
			}
			if err := p.record(body, top.start, top.end, target); err != nil {
				return 0, 0, false, err
			}
			entry := subrEntry{target, region, below + len(ops), stems, nested + 1}
			exit, seen := p.exits[entry]
			if !seen {
				d, s, ended, err := p.run(target, sub, region, entry.depth, stems, nested+1, count, budget)
				if err != nil {
					return 0, 0, false, err
				}
				exit = subrExit{d, s, ended}
				p.exits[entry] = exit
			}
			if exit.ended {
				return 0, 0, true, nil
			}
			// Whatever is on the stack now, the subroutine may have taken from
			// it or added to it, and none of it can name a subroutine here.
			below, ops, stems = exit.depth, ops[:0], exit.stems
			at++
		case 11: // return
			if body.kind == bodyGlyph {
				return 0, 0, false, errKeepSubrs // a glyph has nothing to return to
			}
			return below + len(ops), stems, false, nil
		case 14: // endchar: the glyph is drawn
			return 0, 0, true, nil
		case 1, 3, 18, 23: // hstem, vstem, hstemhm, vstemhm
			stems += (below + len(ops)) / 2 // an odd one out in front is the width
			below, ops = 0, ops[:0]
			at++
		case 19, 20: // hintmask, cntrmask
			// Numbers still on the stack are an implicit vstem, and the mask
			// that follows has a bit for every stem declared so far.
			stems += (below + len(ops)) / 2
			below, ops = 0, ops[:0]
			at += 1 + (stems+7)/8
			if at > len(code) {
				return 0, 0, false, errKeepSubrs
			}
		case 4, 5, 6, 7, 8, 21, 22, 24, 25, 26, 27, 30, 31: // path operators
			below, ops = 0, ops[:0]
			at++
		case 12:
			if at+2 > len(code) {
				return 0, 0, false, errKeepSubrs
			}
			switch code[at+1] {
			case 0, 34, 35, 36, 37: // dotsection, hflex, flex, hflex1, flex1
				below, ops = 0, ops[:0]
				at += 2
			default:
				// Arithmetic, storage, a conditional or random: what is on the
				// stack afterwards is not simply nothing.
				return 0, 0, false, errKeepSubrs
			}
		default:
			return 0, 0, false, errKeepSubrs // reserved in Type 2
		}
	}
	// A body that runs out without return or endchar ends there, as a reader
	// treats it.
	return below + len(ops), stems, false, nil
}

// record notes that the number at body[start:end] named target.
func (p *subrPruning) record(body csBody, start, end int, target csBody) error {
	m := p.calls[body]
	if m == nil {
		m = map[int]*subrCall{}
		p.calls[body] = m
	}
	c := m[start]
	if c == nil {
		m[start] = &subrCall{end: end, targets: []csBody{target}}
		return nil
	}
	if c.end != end {
		return errKeepSubrs // one body read two ways
	}
	for _, t := range c.targets {
		if t == target {
			return nil
		}
	}
	c.targets = append(c.targets, target)
	return nil
}

// renumbered is the outcome: each INDEX with only its used subroutines, and
// every body's calls rewritten to the new numbers.
type renumbered struct {
	global [][]byte
	locals [][][]byte
	p      *subrPruning
	newG   []int   // original index → new, -1 dropped
	newL   [][]int // the same, per region
	gCount int     // how many subroutines each INDEX keeps, which sets its bias
	lCount []int
}

// operand is the number that names subroutine t in the renumbered font: its
// new index less the bias of the INDEX it is now in.
func (r *renumbered) operand(t csBody) int {
	if t.kind == bodyGlobal {
		return r.newG[t.index] - font.CFFSubrBias(r.gCount)
	}
	return r.newL[t.region][t.index] - font.CFFSubrBias(r.lCount[t.region])
}

// renumber builds the pruned INDEXes. It returns errKeepSubrs when some call
// site would need two different numbers.
func (p *subrPruning) renumber() (*renumbered, error) {
	r := &renumbered{p: p}
	number := func(used []bool) ([]int, int) {
		out := make([]int, len(used))
		k := 0
		for i, u := range used {
			out[i] = -1
			if u {
				out[i] = k
				k++
			}
		}
		return out, k
	}
	r.newG, r.gCount = number(p.usedG)
	r.newL = make([][]int, len(p.usedL))
	r.lCount = make([]int, len(p.usedL))
	for reg, used := range p.usedL {
		r.newL[reg], r.lCount[reg] = number(used)
	}
	for _, calls := range p.calls {
		for _, c := range calls {
			want := r.operand(c.targets[0])
			for _, t := range c.targets[1:] {
				if r.operand(t) != want {
					return nil, errKeepSubrs
				}
			}
		}
	}

	r.global = make([][]byte, 0, r.gCount)
	for i, u := range p.usedG {
		if u {
			b, err := r.rewrite(csBody{bodyGlobal, 0, i}, p.global[i])
			if err != nil {
				return nil, err
			}
			r.global = append(r.global, b)
		}
	}
	r.locals = make([][][]byte, len(p.locals))
	for reg, used := range p.usedL {
		for i, u := range used {
			if u {
				b, err := r.rewrite(csBody{bodyLocal, reg, i}, p.locals[reg][i])
				if err != nil {
					return nil, err
				}
				r.locals[reg] = append(r.locals[reg], b)
			}
		}
	}
	return r, nil
}

// glyph rewrites the calls in the subset's glyph g.
func (r *renumbered) glyph(g int, code []byte) ([]byte, error) {
	return r.rewrite(csBody{bodyGlyph, 0, g}, code)
}

// rewrite replaces the number at each of a body's call sites with the one that
// names the same subroutine in the renumbered INDEX. Every other byte is copied
// as it was.
func (r *renumbered) rewrite(body csBody, code []byte) ([]byte, error) {
	calls := r.p.calls[body]
	if len(calls) == 0 {
		return code, nil
	}
	starts := make([]int, 0, len(calls))
	for s := range calls {
		starts = append(starts, s)
	}
	sort.Ints(starts)
	out := make([]byte, 0, len(code))
	at := 0
	for _, s := range starts {
		c := calls[s]
		if s < at {
			return nil, errKeepSubrs // two call sites overlap: the body was read two ways
		}
		out = append(out, code[at:s]...)
		out = append(out, type2Int(r.operand(c.targets[0]))...)
		at = c.end
	}
	return append(out, code[at:]...), nil
}

// type2Int writes an integer as a Type 2 charstring operand, in the shortest
// form that holds it.
func type2Int(v int) []byte {
	switch {
	case v >= -107 && v <= 107:
		return []byte{byte(v + 139)}
	case v >= 108 && v <= 1131:
		v -= 108
		return []byte{byte(v>>8 + 247), byte(v)}
	case v >= -1131 && v <= -108:
		v = -v - 108
		return []byte{byte(v>>8 + 251), byte(v)}
	}
	return []byte{28, byte(v >> 8), byte(v)}
}

// privateParts reads a Private DICT region — as cffPrivateRegion returned it,
// the DICT of size bytes and everything to the end of its subroutines — into
// the DICT's entries and its local subroutines.
func privateParts(region []byte, size int) ([]cffOp, [][]byte, [][]byte, error) {
	ops, raw, err := parseCFFDict(region[:size])
	if err != nil {
		return nil, nil, nil, err
	}
	var subrs [][]byte
	for _, e := range ops {
		if e.op == opSubrs && len(e.operands) == 1 && e.operands[0] > 0 {
			idx, err := readCFFIndex(region, e.operands[0])
			if err != nil {
				return nil, nil, nil, err
			}
			subrs = idx.items
		}
	}
	return ops, raw, subrs, nil
}

// rebuildPrivate writes a Private DICT region around a new set of local
// subroutines: the DICT, its Subrs operand naming the INDEX that immediately
// follows it, or no Subrs at all when there are none. It returns the region and
// the DICT's size, which is what a Font DICT states.
func rebuildPrivate(ops []cffOp, raw [][]byte, subrs [][]byte) ([]byte, int) {
	build := func(at int) []byte {
		var d []byte
		for i, e := range ops {
			if e.op != opSubrs {
				d = append(d, raw[i]...)
				continue
			}
			if len(subrs) > 0 {
				d = append(d, cffInt(at)...) // five bytes whatever the value
				d = append(d, byte(opSubrs))
			}
		}
		return d
	}
	d := build(len(build(0)))
	size := len(d)
	if len(subrs) > 0 {
		d = append(d, writeCFFIndex(subrs)...)
	}
	return d, size
}

// renumberSFNT rewrites the tables of the sfnt around a renumbered CFF that are
// indexed by glyph: maxp's count, hmtx and the count in hhea, cmap, and post.
// order[g] is the original glyph index of the subset's glyph g, and numGlyphs
// the original font's count; cmap is the face's character map.
func renumberSFNT(out map[string][]byte, order []int, numGlyphs int, cmap map[rune]int) error {
	newIndex := make([]int, numGlyphs)
	for g := range newIndex {
		newIndex[g] = -1
	}
	for g, old := range order {
		newIndex[old] = g
	}

	if maxp := out["maxp"]; len(maxp) >= 6 {
		m := append([]byte(nil), maxp...)
		binary.BigEndian.PutUint16(m[4:], uint16(len(order)))
		out["maxp"] = m
	}
	hhea, hmtx := out["hhea"], out["hmtx"]
	if len(hhea) >= 36 && hmtx != nil {
		adv, lsb, err := parseHmtx(hmtx, font.Be16(hhea, 34), numGlyphs)
		if err != nil {
			return err
		}
		h := make([]byte, 0, 4*len(order))
		for _, old := range order {
			h = binary.BigEndian.AppendUint16(h, uint16(adv[old]))
			h = binary.BigEndian.AppendUint16(h, uint16(int16(lsb[old])))
		}
		out["hmtx"] = h
		nh := append([]byte(nil), hhea...)
		binary.BigEndian.PutUint16(nh[34:], uint16(len(order)))
		out["hhea"] = nh
	}
	// The character map, of the characters whose glyph was kept. The face's
	// own map is the one read — the best Unicode subtable — and it is written
	// back as Unicode subtables: format 4 for the BMP, and format 12 as well
	// where a character lies past it.
	if _, had := out["cmap"]; had {
		m := make(map[rune]int)
		for r, old := range cmap {
			if old > 0 && old < numGlyphs && newIndex[old] > 0 {
				m[r] = newIndex[old]
			}
		}
		out["cmap"] = writeCmap(m)
	}
	// A version 2 post names glyphs by index, and a CFF carries its own names
	// anyway; version 3 is the header alone and names none.
	if post := out["post"]; len(post) >= 32 {
		p := append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(p, 0x00030000)
		out["post"] = p
	}
	return nil
}

// writeCmap writes a cmap for m: a (3,1) format 4 subtable for the BMP, and a
// (3,10) format 12 one for the whole of it when a character lies past the BMP
// or the format 4 would not fit its sixteen-bit length.
func writeCmap(m map[rune]int) []byte {
	runes := make([]rune, 0, len(m))
	for r := range m {
		runes = append(runes, r)
	}
	sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })

	type sub struct {
		enc  uint16
		body []byte
	}
	var subs []sub
	f4 := cmapFormat4(runes, m)
	if f4 != nil {
		subs = append(subs, sub{1, f4})
	}
	if f4 == nil || (len(runes) > 0 && runes[len(runes)-1] > 0xFFFF) {
		subs = append(subs, sub{10, cmapFormat12(runes, m)})
	}
	out := []byte{0, 0}
	out = binary.BigEndian.AppendUint16(out, uint16(len(subs)))
	at := 4 + 8*len(subs)
	for _, s := range subs {
		out = binary.BigEndian.AppendUint16(out, 3)
		out = binary.BigEndian.AppendUint16(out, s.enc)
		out = binary.BigEndian.AppendUint32(out, uint32(at))
		at += len(s.body)
	}
	for _, s := range subs {
		out = append(out, s.body...)
	}
	return out
}

// cmapFormat4 writes the BMP characters of runes (sorted) as a format 4
// subtable: a segment for each run of consecutive characters whose glyphs are
// consecutive too, and the closing segment for U+FFFF the format requires. It
// returns nil when the subtable would not fit its sixteen-bit length.
func cmapFormat4(runes []rune, m map[rune]int) []byte {
	type seg struct{ start, end, delta int }
	var segs []seg
	for _, r := range runes {
		if r >= 0xFFFF {
			break // U+FFFF is the closing segment's, and is not a character
		}
		c, delta := int(r), (m[r]-int(r))&0xFFFF
		if n := len(segs); n > 0 && segs[n-1].end == c-1 && segs[n-1].delta == delta {
			segs[n-1].end = c
			continue
		}
		segs = append(segs, seg{c, c, delta})
	}
	segs = append(segs, seg{0xFFFF, 0xFFFF, 1})
	n := len(segs)
	length := 16 + 8*n
	if length > 0xFFFF {
		return nil
	}
	entrySelector := 0
	for 1<<(entrySelector+1) <= n {
		entrySelector++
	}
	searchRange := 2 << entrySelector
	out := make([]byte, 0, length)
	for _, v := range []int{4, length, 0, 2 * n, searchRange, entrySelector, 2*n - searchRange} {
		out = binary.BigEndian.AppendUint16(out, uint16(v))
	}
	for _, s := range segs {
		out = binary.BigEndian.AppendUint16(out, uint16(s.end))
	}
	out = append(out, 0, 0) // reservedPad
	for _, s := range segs {
		out = binary.BigEndian.AppendUint16(out, uint16(s.start))
	}
	for _, s := range segs {
		out = binary.BigEndian.AppendUint16(out, uint16(s.delta))
	}
	for range segs {
		out = append(out, 0, 0) // idRangeOffset: the delta is the whole mapping
	}
	return out
}

// cmapFormat12 writes runes (sorted) as a format 12 subtable: a group for each
// run of consecutive characters whose glyphs are consecutive too.
func cmapFormat12(runes []rune, m map[rune]int) []byte {
	type group struct{ start, end, glyph int }
	var groups []group
	for _, r := range runes {
		c, g := int(r), m[r]
		if n := len(groups); n > 0 && groups[n-1].end == c-1 &&
			groups[n-1].glyph+(c-groups[n-1].start) == g {
			groups[n-1].end = c
			continue
		}
		groups = append(groups, group{c, c, g})
	}
	out := []byte{0, 12, 0, 0}
	out = binary.BigEndian.AppendUint32(out, uint32(16+12*len(groups)))
	out = binary.BigEndian.AppendUint32(out, 0) // language
	out = binary.BigEndian.AppendUint32(out, uint32(len(groups)))
	for _, g := range groups {
		out = binary.BigEndian.AppendUint32(out, uint32(g.start))
		out = binary.BigEndian.AppendUint32(out, uint32(g.end))
		out = binary.BigEndian.AppendUint32(out, uint32(g.glyph))
	}
	return out
}

// privateRegion is a Private DICT region taken apart: the DICT's entries, their
// bytes, and its local subroutines.
type privateRegion struct {
	ops   []cffOp
	raw   [][]byte
	subrs [][]byte
}

// pruneSubrs runs the subset's glyphs — glyphs[g] the charstring of its glyph
// g, drawn with the local subroutines of Private DICT region regions[g] — and
// returns them with every INDEX cut down to the subroutines they reach and the
// calls renumbered to match. ok is false when that cannot be done exactly, and
// the caller keeps the subroutines and the charstrings as they were; err is
// the budget's, when running the glyphs exhausts it.
func pruneSubrs(glyphs [][]byte, regions []int, global [][]byte, locals [][][]byte,
	budget *font.Budget) (newGlyphs, newGlobal [][]byte, newLocals [][][]byte, ok bool, err error) {

	p := newSubrPruning(global, locals)
	for g, code := range glyphs {
		if err := p.walk(g, code, regions[g], budget); err != nil {
			if errors.Is(err, errKeepSubrs) {
				return nil, nil, nil, false, nil
			}
			return nil, nil, nil, false, err
		}
	}
	r, err := p.renumber()
	if err != nil {
		return nil, nil, nil, false, nil // only ever errKeepSubrs
	}
	newGlyphs = make([][]byte, len(glyphs))
	for g, code := range glyphs {
		b, err := r.glyph(g, code)
		if err != nil {
			return nil, nil, nil, false, nil
		}
		newGlyphs[g] = b
	}
	return newGlyphs, r.global, r.locals, true, nil
}
