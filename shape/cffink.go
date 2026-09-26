package shape

import (
	"encoding/binary"
	"errors"
	"math"
	"sync"

	"github.com/mgilbir/forme/font"
)

// The ink of a CFF glyph: where its outline puts marks on the page, read by
// running its charstring.
//
// A TrueType glyph states its box in its header, and fallback.go and
// vertical.go read it there. A CFF glyph states nothing of the kind: its
// outline is a program — moves, lines and curves, hints, calls into
// subroutines shared with other glyphs, and for an accented letter a seac
// naming two other glyphs — and the only way to know where it draws is to run
// it. Without that, a face with CFF outlines could not answer the three
// questions its ink answers: where a mark goes in a face that positions none
// of its own (fallback.go), where a glyph set upright is hung in a face with
// no VORG (vertical.go), and how far a run reaches above and below its
// baseline (Face.InkExtent). Unifont, which has no VORG and no mark
// positioning, was hung from its ascender where HarfBuzz centres its ink.
//
// What is measured is HarfBuzz's answer, hb-ot-cff1-table.cc's get_extents at
// the release the oracle is pinned to, because the places it is used are held
// to HarfBuzz unit for unit:
//
//   - The box is the control box: every point a line reaches and every point
//     of a curve, its two control points included, and not the tighter box of
//     the curve itself. A move alone draws nothing, and the point a path
//     starts from counts only once something is drawn from it.
//   - The numbers are the charstring's own, a double each as HarfBuzz keeps
//     them, a 16.16 fixed operand exactly; the box's edges are rounded as
//     HarfBuzz rounds them, half up — its roundf is its own, floor(x + .5),
//     so -10.5 is -10 and not the C library's -11 — and an axis with no
//     extent is zero. The box is then scaled through floats, as HarfBuzz
//     scales it, which loses the last unit of an edge past 2^24.
//   - A seac draws its two glyphs, the accent moved by the offset it names,
//     and a charstring that fails in any way — an operator it runs off the
//     stack, a subroutine that is not there, calls nested past ten, a flex
//     with the wrong count of operands, a seac inside a seac, a component
//     that is not in the font, or more than HarfBuzz's 200,000 operators —
//     has no ink at all, as HarfBuzz gives it none.
//   - The font as a whole is read as HarfBuzz's accelerator reads it, and one
//     it would refuse — a charstring count that is not the font's glyph
//     count, a CID-keyed font with no charset or no FDSelect, a Private DICT
//     that is not there — has no ink for any glyph.
//
// # The cost
//
// A charstring is a program the font wrote, so its cost is the font's to
// choose. Each glyph's run is capped where HarfBuzz caps it, at 200,000
// operators, which makes the answer HarfBuzz's for a glyph that asks for more;
// and each glyph is run at most once, when it is first asked about, and the
// answer kept. What is left is a font whose every glyph asks for the cap and
// a document that names every glyph, and that is what the face's own budget is
// for: every operator any glyph runs is charged to it, and one that runs out
// leaves the glyphs after it with no ink, which Face.LayoutLimits reports. See
// cffInkWork for how large it is and why no real font comes near it.
//
// # What is not here
//
// CFF2. A face whose outlines are a CFF2 table is not loaded at all (see
// Load), so there is no such glyph to measure.

// cffMaxOps is how many operators HarfBuzz runs of one charstring, the
// subroutines it calls included, before it gives up on it:
// HB_CFF_MAX_OPS. The run that reaches the cap fails, so a charstring may run
// one fewer.
const cffMaxOps = 200000

// cffMaxCalls is how deeply subroutine calls may nest, and cffMaxArgs how many
// operands the stack holds: HarfBuzz's kMaxCallLimit and its argument stack's
// size, which decide which charstrings it refuses.
const (
	cffMaxCalls = 10
	cffMaxArgs  = 513
)

// cffInkWork is the budget every glyph of one face shares, in operators run.
//
// It is sized to the font: sixteen operators a byte of its CFF table, and a
// floor of maxFontWork for a small one. A glyph's charstring and the
// subroutines it calls are bytes of the table, and a real font runs each byte
// about once across all its glyphs, not thousands of times: measuring every
// glyph of every CFF face in the corpora costs at most 1.15 operators a byte
// (Noto Serif JP, 6.7 million for its 17,923 glyphs and 5.9 MB of table; Noto
// Sans SC, the largest, 7.5 million for 8.0 MB), so a document could use every
// glyph of any of them and spend a fourteenth of it. What reaches it is a
// charstring that loops through its subroutines — a few hundred bytes of font
// asking for as much as HarfBuzz's per-glyph cap allows, in every glyph.
func cffInkWork(tableLen int) int {
	return max(maxFontWork, 16*tableLen)
}

// cffInk is what a face with CFF outlines keeps to measure its glyphs' ink:
// the table, read into its charstrings when a glyph is first asked about, and
// each glyph's answer once it has one.
//
// It is shared by every Clone of the face, as the layout cache is, and for
// the same reason: the answers are the font's and no document changes them.
// The lock is there because they are filled in lazily.
type cffInk struct {
	table     []byte
	numGlyphs int

	once sync.Once
	// outlines is nil where HarfBuzz's accelerator would refuse the table.
	outlines *cffOutlines

	mu      sync.Mutex
	answers map[int]cffInkAnswer
	budget  *font.Budget
	// capped and spent record the two ways a glyph was left without ink that
	// were bounds rather than malformed charstrings, for LayoutLimits.
	capped, spent bool
}

// cffInkAnswer is one glyph's ink, and whether it has any to state.
type cffInkAnswer struct {
	ext extents
	ok  bool
}

func newCFFInk(table []byte, numGlyphs int) *cffInk {
	return &cffInk{table: table, numGlyphs: numGlyphs}
}

// extents is a glyph's ink in font units, HarfBuzz's hb_glyph_extents_t for it
// at a scale of one unit to the unit, and false where HarfBuzz has none.
func (c *cffInk) extents(gid int) (extents, bool) {
	c.once.Do(func() {
		c.outlines, _ = readCFFOutlines(c.table, c.numGlyphs)
		c.budget = font.NewBudget(cffInkWork(len(c.table)))
	})
	o := c.outlines
	if o == nil || gid < 0 || gid >= len(o.charStrings) {
		return extents{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if a, ok := c.answers[gid]; ok {
		return a.ext, a.ok
	}
	r := t2Run{o: o, budget: c.budget}
	b, ok := r.bounds(gid, false)
	switch {
	case r.spent:
		c.spent = true
	case r.capped:
		c.capped = true
	}
	a := cffInkAnswer{ok: ok}
	if ok {
		a.ext = b.extents()
	}
	if c.answers == nil {
		c.answers = map[int]cffInkAnswer{}
	}
	c.answers[gid] = a
	return a.ext, a.ok
}

// limits are the bounds measuring glyphs has run into so far, in words that
// say what was not done. See Face.LayoutLimits.
func (c *cffInk) limits() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	if c.capped {
		out = append(out, "a glyph's CFF charstring runs past the 200,000 operators "+
			"HarfBuzz runs of one, so, as HarfBuzz does, it is placed as a glyph with no ink")
	}
	if c.spent {
		out = append(out, "measuring the ink of its CFF glyphs ran past the work one "+
			"face may cost, so the glyphs measured after that are placed as glyphs with no ink")
	}
	return out
}

// cffOutlines is a CFF table read for its charstrings: what running a glyph
// needs, and what a seac needs to find the two glyphs it names.
type cffOutlines struct {
	charStrings [][]byte
	global      [][]byte
	// locals are each Font DICT's local subroutines — a font that is not
	// CID-keyed has one — and fds the Font DICT of each glyph, nil where there
	// is only the one. A glyph whose FDSelect names a Font DICT past the
	// FDArray is run with no local subroutines, as HarfBuzz runs it.
	locals [][][]byte
	fds    []byte
	// sids is the charset, the SID of each glyph (for a CID-keyed font, its
	// CID), and nil for a predefined one; charsetOff is which predefined one.
	// firstGID is sids inverted, built the first time a seac asks.
	sids       []int
	charsetOff int
	firstGID   map[int]int
}

// readCFFOutlines reads a CFF table as HarfBuzz's cff1 accelerator does, and
// refuses it where the accelerator would — in which case HarfBuzz measures
// none of its glyphs, and neither does this.
func readCFFOutlines(cff []byte, numGlyphs int) (*cffOutlines, error) {
	top, err := topDictOf(cff)
	if err != nil {
		return nil, err
	}
	var csOff, charsetOff, encodingOff, privSize, privOff, fdArrayOff, fdSelectOff int
	isCID := false
	for _, e := range top {
		last := func() int {
			if len(e.operands) == 0 {
				return 0
			}
			return e.operands[len(e.operands)-1]
		}
		switch e.op {
		case opROS:
			isCID = true
		case opCharStrings:
			csOff = last()
		case opCharset:
			charsetOff = last()
		case opEncoding:
			encodingOff = last()
		case opPrivate:
			if len(e.operands) >= 2 {
				privSize, privOff = e.operands[len(e.operands)-2], e.operands[len(e.operands)-1]
			}
		case opFDArray:
			fdArrayOff = last()
		case opFDSelect:
			fdSelectOff = last()
		}
	}
	o := &cffOutlines{charsetOff: charsetOff}
	if charsetOff > 2 {
		if o.sids, err = cffCharsetSIDs(cff, charsetOff, numGlyphs); err != nil {
			return nil, err
		}
	} else if isCID {
		return nil, errors.New("fonts: a CID-keyed CFF with a predefined charset")
	}
	if !isCID && encodingOff > 1 {
		if _, err := sliceEncoding(cff, encodingOff); err != nil {
			return nil, err
		}
	}
	// The String INDEX has to be there, and the Global Subr INDEX after it
	// need not be: HarfBuzz refuses a font without the first and reads the
	// second, where it cannot, as no subroutines.
	at := int(cff[2])
	for range 3 {
		idx, err := readCFFIndex(cff, at)
		if err != nil {
			return nil, err
		}
		at = idx.end
	}
	if idx, err := readCFFIndex(cff, at); err == nil {
		o.global = idx.items
	}
	if csOff <= 0 {
		return nil, errors.New("fonts: the CFF names no CharStrings")
	}
	cs, err := readCFFIndex(cff, csOff)
	if err != nil {
		return nil, err
	}
	if len(cs.items) != numGlyphs {
		return nil, errors.New("fonts: the CFF's glyph count is not the font's")
	}
	o.charStrings = cs.items

	if !isCID {
		subrs, err := cffLocalSubrs(cff, privOff, privSize)
		if err != nil {
			return nil, err
		}
		o.locals = [][][]byte{subrs}
		return o, nil
	}
	// A CID-keyed font with no FDArray is refused here, and one with no
	// FDSelect by cffFDSelect. The first has to be asked outright: an offset
	// of zero would read the font's own header as the FDArray's INDEX.
	if fdArrayOff <= 0 {
		return nil, errors.New("fonts: a CID-keyed CFF with no FDArray")
	}
	fdArray, err := readCFFIndex(cff, fdArrayOff)
	if err != nil {
		return nil, err
	}
	if o.fds, err = cffFDSelect(cff, fdSelectOff, numGlyphs, len(fdArray.items)); err != nil {
		return nil, err
	}
	for _, fd := range fdArray.items {
		ops, _, err := parseCFFDict(fd)
		if err != nil {
			return nil, err
		}
		size, off := 0, 0
		for _, e := range ops {
			if e.op == opPrivate && len(e.operands) >= 2 {
				size, off = e.operands[len(e.operands)-2], e.operands[len(e.operands)-1]
			}
		}
		subrs, err := cffLocalSubrs(cff, off, size)
		if err != nil {
			return nil, err
		}
		o.locals = append(o.locals, subrs)
	}
	return o, nil
}

// cffLocalSubrs is the local subroutines a Private DICT names, relative to
// its own start. A Private DICT that is not where its Font DICT says refuses
// the font, as it does HarfBuzz's accelerator; subroutines that are not where
// the Private DICT says are no subroutines, which is how HarfBuzz reads them.
func cffLocalSubrs(cff []byte, off, size int) ([][]byte, error) {
	if size == 0 {
		return nil, nil
	}
	if off <= 0 || size < 0 || off > len(cff) || size > len(cff)-off {
		return nil, errors.New("fonts: a CFF Private DICT lies outside the font")
	}
	ops, _, err := parseCFFDict(cff[off : off+size])
	if err != nil {
		return nil, err
	}
	for _, e := range ops {
		if e.op != opSubrs || len(e.operands) == 0 {
			continue
		}
		at := e.operands[len(e.operands)-1]
		if at == 0 {
			return nil, nil
		}
		idx, err := readCFFIndex(cff, off+at)
		if err != nil {
			return nil, nil
		}
		return idx.items, nil
	}
	return nil, nil
}

// cffFDSelect is the Font DICT of each glyph, from an FDSelect held to what
// HarfBuzz's sanitizer holds it to: format 0 must have a byte for every glyph,
// and format 3's ranges must start at glyph zero, rise, name Font DICTs the
// FDArray has, and end at a sentinel that is the glyph count. Format 0's bytes
// are not checked against the FDArray, and HarfBuzz does not check them
// either.
func cffFDSelect(cff []byte, off, numGlyphs, fdCount int) ([]byte, error) {
	b, err := sliceFDSelect(cff, off, numGlyphs)
	if err != nil {
		return nil, err
	}
	if b[0] == 0 {
		return b[1:], nil
	}
	ranges := int(binary.BigEndian.Uint16(b[1:]))
	if ranges == 0 {
		return nil, errors.New("fonts: a CFF FDSelect with no ranges")
	}
	fds := make([]byte, numGlyphs)
	for i := 0; i < ranges; i++ {
		first := int(binary.BigEndian.Uint16(b[3+3*i:]))
		fd := b[5+3*i]
		next := int(binary.BigEndian.Uint16(b[3+3*(i+1):]))
		if (i == 0 && first != 0) || first >= numGlyphs || int(fd) >= fdCount || next <= first ||
			(i == ranges-1 && next != numGlyphs) {
			return nil, errors.New("fonts: a CFF FDSelect whose ranges do not cover the glyphs")
		}
		for g := first; g < next; g++ {
			fds[g] = fd
		}
	}
	return fds, nil
}

// localsOf is the local subroutines a glyph's charstring calls into.
func (o *cffOutlines) localsOf(gid int) [][]byte {
	fd := 0
	if o.fds != nil {
		fd = int(o.fds[gid])
	}
	if fd >= len(o.locals) {
		return nil
	}
	return o.locals[fd]
}

// stdCodeToGlyph is the glyph a seac names by a StandardEncoding code, and 0
// for none: HarfBuzz's std_code_to_glyph. The code is a SID by way of
// StandardEncoding, and the SID a glyph by way of the charset — the first glyph
// it names, or for the ISOAdobe charset, which numbers its glyphs by SID, the
// SID itself for the codes up to 228. HarfBuzz reads the Expert charsets as
// naming nothing, and so does this.
func (o *cffOutlines) stdCodeToGlyph(code int) int {
	if code < 0 || code > 0xFF {
		return 0
	}
	sid := standardEncodingSID(byte(code))
	if o.sids == nil {
		if o.charsetOff == 0 && code <= 228 {
			return sid
		}
		return 0
	}
	if sid == 0 {
		return 0
	}
	if o.firstGID == nil {
		o.firstGID = make(map[int]int, len(o.sids))
		for g := len(o.sids) - 1; g >= 1; g-- {
			o.firstGID[o.sids[g]] = g
		}
	}
	return o.firstGID[sid]
}

// standardEncodingSID is the SID of the glyph StandardEncoding puts at a code,
// and 0 for a code it leaves empty.
func standardEncodingSID(code byte) int {
	name, ok := font.StandardEncodingName(code)
	if !ok {
		return 0
	}
	sid, _ := font.CFFStandardSID(name)
	return sid
}

// cffBounds is the box a charstring's drawing reaches, as HarfBuzz keeps it: in
// doubles, starting inverted so that the first point drawn sets it.
type cffBounds struct {
	minX, minY, maxX, maxY float64
}

func newCFFBounds() cffBounds {
	return cffBounds{math.MaxInt32, math.MaxInt32, math.MinInt32, math.MinInt32}
}

func (b *cffBounds) update(x, y float64) {
	if x < b.minX {
		b.minX = x
	}
	if x > b.maxX {
		b.maxX = x
	}
	if y < b.minY {
		b.minY = y
	}
	if y > b.maxY {
		b.maxY = y
	}
}

// empty is HarfBuzz's bounds_t::empty, which a box that is only a horizontal
// or a vertical line is, as well as one with nothing in it: a seac's merge
// replaces such a box rather than extending it.
func (b cffBounds) empty() bool { return b.minX >= b.maxX || b.minY >= b.maxY }

func (b *cffBounds) merge(o cffBounds) {
	switch {
	case b.empty():
		*b = o
	case !o.empty():
		b.minX, b.maxX = min(b.minX, o.minX), max(b.maxX, o.maxX)
		b.minY, b.maxY = min(b.minY, o.minY), max(b.maxY, o.maxY)
	}
}

func (b *cffBounds) offset(dx, dy float64) {
	if !b.empty() {
		b.minX += dx
		b.maxX += dx
		b.minY += dy
		b.maxY += dy
	}
}

// extents is the box as hb_glyph_extents_t: each edge rounded as HarfBuzz
// rounds it, an axis with no extent zero, and then scaled at one unit to the
// unit, which HarfBuzz does in floats.
func (b cffBounds) extents() extents {
	var e [4]int32 // x bearing, y bearing, width, height
	if b.minX < b.maxX {
		xb := hbRound(b.minX)
		e[0] = clampToInt32(xb)
		e[2] = clampToInt32(hbRound(b.maxX) - xb)
	}
	if b.minY < b.maxY {
		yb := hbRound(b.maxY)
		e[1] = clampToInt32(yb)
		e[3] = clampToInt32(hbRound(b.minY) - yb)
	}
	// hb_font_t::scale_glyph_extents, whose multiplier is one here.
	x1, y1 := float32(e[0]), float32(e[1])
	x2, y2 := float32(e[0]+e[2]), float32(e[1]+e[3])
	xb := int32(math.Floor(float64(x1)))
	yb := int32(math.Floor(float64(y1)))
	return extents{
		xBearing: int(xb),
		yBearing: int(yb),
		width:    int(int32(float32(math.Ceil(float64(x2))) - float32(xb))),
		height:   int(int32(float32(math.Ceil(float64(y2))) - float32(yb))),
	}
}

// hbRound is what roundf is in HarfBuzz, which defines it for itself: half up,
// towards positive infinity, in the double it is given. The C library's rounds
// half away from zero, and a glyph whose lowest point is -10.5 is -10 deep in
// HarfBuzz and would be -11 in it.
func hbRound(v float64) float64 { return math.Floor(v + .5) }

func clampToInt32(v float64) int32 {
	switch {
	case v <= math.MinInt32:
		return math.MinInt32
	case v >= math.MaxInt32:
		return math.MaxInt32
	}
	return int32(v)
}

// t2Run is the work of measuring one glyph and whatever a seac in it names:
// the outlines, and the face's budget each operator is charged to.
type t2Run struct {
	o      *cffOutlines
	budget *font.Budget
	// capped is set when a charstring ran into HarfBuzz's cap, and spent when
	// the budget ran out; either leaves the glyph without ink.
	capped, spent bool
}

// t2Frame is one charstring or subroutine being run, and how far into it.
type t2Frame struct {
	code []byte
	at   int
}

// t2Interp is one charstring's run: HarfBuzz's cff1_cs_interp_env_t and the
// extents parameters it is run with.
type t2Interp struct {
	run    *t2Run
	locals [][]byte
	inSeac bool

	args [cffMaxArgs]float64
	n    int
	// err is any of the things HarfBuzz's interpreter calls an error: an
	// operand read past the end of the charstring, a stack pushed past its
	// size or popped past its bottom, a call it cannot make, a return with
	// nowhere to return to.
	err bool

	cur   t2Frame
	calls [cffMaxCalls]t2Frame
	depth int

	stems        int
	seenHintmask bool
	hintmaskSize int

	x, y    float64
	open    bool
	bounds  cffBounds
	endchar bool
}

// bounds runs glyph gid's charstring for the box it draws: HarfBuzz's
// _get_bounds. inSeac is set for a glyph a seac names.
func (r *t2Run) bounds(gid int, inSeac bool) (cffBounds, bool) {
	if gid < 0 || gid >= len(r.o.charStrings) {
		return cffBounds{}, false
	}
	in := &t2Interp{run: r, locals: r.o.localsOf(gid), inSeac: inSeac, bounds: newCFFBounds()}
	in.cur = t2Frame{code: r.o.charStrings[gid]}
	for left := cffMaxOps; ; {
		if !r.budget.Charge(1, "the ink of the CFF glyphs") {
			r.spent = true
			return cffBounds{}, false
		}
		in.step(in.fetch())
		left--
		if in.err || r.spent {
			return cffBounds{}, false
		}
		if left == 0 {
			r.capped = true
			return cffBounds{}, false
		}
		if in.endchar {
			return in.bounds, true
		}
	}
}

// t2Invalid is the operator read where a charstring has run out, which does
// nothing but clear the stack: a charstring that ends without endchar reads it
// until the cap.
const t2Invalid = 0xFFFF

// The two-byte operators this reads, by their second byte plus 256.
const (
	t2DotSection = 256 + 0
	t2HFlex      = 256 + 34
	t2Flex       = 256 + 35
	t2HFlex1     = 256 + 36
	t2Flex1      = 256 + 37
)

func (in *t2Interp) fetch() int {
	f := &in.cur
	if f.at >= len(f.code) {
		return t2Invalid
	}
	op := int(f.code[f.at])
	f.at++
	if op == 12 {
		if f.at >= len(f.code) {
			return t2Invalid
		}
		op = 256 + int(f.code[f.at])
		f.at++
	}
	return op
}

// operand is the byte i past the operator just read, and an error past the
// end of the charstring.
func (in *t2Interp) operand(i int) int {
	if in.cur.at+i >= len(in.cur.code) {
		in.err = true
		return 0
	}
	return int(in.cur.code[in.cur.at+i])
}

func (in *t2Interp) push(v float64) {
	if in.n >= cffMaxArgs {
		in.err = true
		return
	}
	in.args[in.n] = v
	in.n++
}

func (in *t2Interp) pop() float64 {
	if in.n == 0 {
		in.err = true
		return 0
	}
	in.n--
	return in.args[in.n]
}

func (in *t2Interp) clear() { in.n = 0 }

// moveTo, lineTo and curveTo are the extents path procedures: a move ends the
// path and draws nothing, and the first line or curve after it takes in the
// point it starts from.
func (in *t2Interp) moveTo(x, y float64) {
	in.open = false
	in.x, in.y = x, y
}

func (in *t2Interp) lineTo(x, y float64) {
	if !in.open {
		in.open = true
		in.bounds.update(in.x, in.y)
	}
	in.x, in.y = x, y
	in.bounds.update(x, y)
}

func (in *t2Interp) curveTo(x1, y1, x2, y2, x3, y3 float64) {
	if !in.open {
		in.open = true
		in.bounds.update(in.x, in.y)
	}
	in.bounds.update(x1, y1)
	in.bounds.update(x2, y2)
	in.x, in.y = x3, y3
	in.bounds.update(x3, y3)
}

// step runs one operator: cff1_cs_opset_t's process_op, with the extents
// path procedures.
func (in *t2Interp) step(op int) {
	a := func(i int) float64 { return in.args[i] }
	switch {
	case op >= 32 && op <= 246:
		in.push(float64(op - 139))
		return
	case op >= 247 && op <= 250:
		v := int16((op-247)*256 + in.operand(0) + 108)
		in.cur.at++
		in.push(float64(v))
		return
	case op >= 251 && op <= 254:
		v := -(op-251)*256 - in.operand(0) - 108
		in.cur.at++
		in.push(float64(v))
		return
	}
	switch op {
	case 28: // shortint
		v := int16(in.operand(0)<<8 | in.operand(1))
		in.cur.at += 2
		in.push(float64(v))
	case 255: // a 16.16 fixed-point number; too few bytes for one, and nothing
		if in.cur.at+4 <= len(in.cur.code) {
			v := int32(binary.BigEndian.Uint32(in.cur.code[in.cur.at:]))
			in.cur.at += 4
			in.push(float64(v) / 65536)
		}

	case 11: // return
		if in.depth == 0 {
			in.err = true
			return
		}
		in.depth--
		in.cur = in.calls[in.depth]
	case 10, 29: // callsubr, callgsubr
		subrs := in.locals
		if op == 29 {
			subrs = in.run.o.global
		}
		v := in.pop()
		if in.err || v < math.MinInt32 || v > math.MaxInt32 {
			in.err = true
			return
		}
		n := int(v) + font.CFFSubrBias(len(subrs))
		if n < 0 || n >= len(subrs) || in.depth >= cffMaxCalls {
			in.err = true
			return
		}
		in.calls[in.depth] = in.cur
		in.depth++
		in.cur = t2Frame{code: subrs[n]}

	case 14: // endchar, which with four operands or more is a seac
		if in.n >= 4 {
			in.seac()
		}
		in.clear()
		in.endchar = true

	case 1, 18, 3, 23: // hstem, hstemhm, vstem, vstemhm
		in.stems += in.n / 2
		in.clear()
	case 19, 20: // hintmask, cntrmask
		// The mask's length is fixed by the stems declared before the first
		// mask, and a mask the charstring does not have room for is not read
		// at all: its bytes are read as operators, and the operands stay.
		if !in.seenHintmask {
			in.stems += in.n / 2
			in.hintmaskSize = (in.stems + 7) >> 3
			in.seenHintmask = true
		}
		if in.cur.at+in.hintmaskSize <= len(in.cur.code) {
			in.clear()
			in.cur.at += in.hintmaskSize
		}

	case 21: // rmoveto
		dy := in.pop()
		dx := in.pop()
		in.moveTo(in.x+dx, in.y+dy)
		in.clear()
	case 22: // hmoveto
		in.moveTo(in.x+in.pop(), in.y)
		in.clear()
	case 4: // vmoveto
		in.moveTo(in.x, in.y+in.pop())
		in.clear()

	case 5: // rlineto
		for i := 0; i+2 <= in.n; i += 2 {
			in.lineTo(in.x+a(i), in.y+a(i+1))
		}
		in.clear()
	case 6, 7: // hlineto, vlineto: alternating, starting across or down
		horizontal := op == 6
		i := 0
		for ; i+2 <= in.n; i += 2 {
			if horizontal {
				in.lineTo(in.x+a(i), in.y)
				in.lineTo(in.x, in.y+a(i+1))
			} else {
				in.lineTo(in.x, in.y+a(i))
				in.lineTo(in.x+a(i+1), in.y)
			}
		}
		if i < in.n {
			if horizontal {
				in.lineTo(in.x+a(i), in.y)
			} else {
				in.lineTo(in.x, in.y+a(i))
			}
		}
		in.clear()
	case 8: // rrcurveto
		for i := 0; i+6 <= in.n; i += 6 {
			in.rcurve(i)
		}
		in.clear()
	case 24: // rcurveline
		if in.n >= 8 {
			i := 0
			for ; i+6 <= in.n-2; i += 6 {
				in.rcurve(i)
			}
			in.lineTo(in.x+a(i), in.y+a(i+1))
		}
		in.clear()
	case 25: // rlinecurve
		if in.n >= 8 {
			i := 0
			for ; i+2 <= in.n-6; i += 2 {
				in.lineTo(in.x+a(i), in.y+a(i+1))
			}
			in.rcurve(i)
		}
		in.clear()
	case 26: // vvcurveto
		i := 0
		x1, y1 := in.x, in.y
		if in.n&1 != 0 {
			x1 += a(0)
			i++
		}
		for ; i+4 <= in.n; i += 4 {
			y1 += a(i)
			x2, y2 := x1+a(i+1), y1+a(i+2)
			in.curveTo(x1, y1, x2, y2, x2, y2+a(i+3))
			x1, y1 = in.x, in.y
		}
		in.clear()
	case 27: // hhcurveto
		i := 0
		x1, y1 := in.x, in.y
		if in.n&1 != 0 {
			y1 += a(0)
			i++
		}
		for ; i+4 <= in.n; i += 4 {
			x1 += a(i)
			x2, y2 := x1+a(i+1), y1+a(i+2)
			in.curveTo(x1, y1, x2, y2, x2+a(i+3), y2)
			x1, y1 = in.x, in.y
		}
		in.clear()
	case 30, 31: // vhcurveto, hvcurveto
		in.alternatingCurves(op == 30)
		in.clear()

	case t2HFlex, t2Flex, t2HFlex1, t2Flex1:
		in.flex(op)
		in.clear()

	default:
		// dotsection, every operator the format reserves or CFF2 alone
		// defines, the arithmetic and storage operators HarfBuzz does not
		// run, and the end of a charstring: the stack is cleared and the run
		// goes on.
		in.clear()
	}
}

// rcurve draws the curve whose six relative coordinates start at argument i.
func (in *t2Interp) rcurve(i int) {
	a := in.args[i : i+6]
	x1, y1 := in.x+a[0], in.y+a[1]
	x2, y2 := x1+a[2], y1+a[3]
	in.curveTo(x1, y1, x2, y2, x2+a[4], y2+a[5])
}

// alternatingCurves is vhcurveto and hvcurveto, which are one operator with
// the axes swapped: curves whose tangents alternate between vertical and
// horizontal, starting vertical for vhcurveto. It follows HarfBuzz's two
// branches as they are written — one for an argument count whose remainder by
// eight is four or more, which starts with a single curve, and one for the
// rest — since what each does with arguments the specification does not
// provide for is the answer being matched.
func (in *t2Interp) alternatingCurves(verticalFirst bool) {
	a := func(i int) float64 { return in.args[i] }
	// along moves a point by d along the tangent that is first on this
	// operator's curves when first is true, and along the other when not.
	along := func(x, y *float64, d float64, first bool) {
		if first == verticalFirst {
			*y += d
		} else {
			*x += d
		}
	}
	n := in.n
	i := 0
	if n%8 >= 4 {
		x1, y1 := in.x, in.y
		along(&x1, &y1, a(0), true)
		x2, y2 := x1+a(1), y1+a(2)
		x3, y3 := x2, y2
		along(&x3, &y3, a(3), false)
		i = 4
		for ; i+8 <= n; i += 8 {
			in.curveTo(x1, y1, x2, y2, x3, y3)
			x1, y1 = in.x, in.y
			along(&x1, &y1, a(i), false)
			x2, y2 = x1+a(i+1), y1+a(i+2)
			x3, y3 = x2, y2
			along(&x3, &y3, a(i+3), true)
			in.curveTo(x1, y1, x2, y2, x3, y3)

			x1, y1 = x3, y3
			along(&x1, &y1, a(i+4), true)
			x2, y2 = x1+a(i+5), y1+a(i+6)
			x3, y3 = x2, y2
			along(&x3, &y3, a(i+7), false)
		}
		if i < n {
			along(&x3, &y3, a(i), true)
		}
		in.curveTo(x1, y1, x2, y2, x3, y3)
		return
	}
	for ; i+8 <= n; i += 8 {
		x1, y1 := in.x, in.y
		along(&x1, &y1, a(i), true)
		x2, y2 := x1+a(i+1), y1+a(i+2)
		x3, y3 := x2, y2
		along(&x3, &y3, a(i+3), false)
		in.curveTo(x1, y1, x2, y2, x3, y3)

		x1, y1 = x3, y3
		along(&x1, &y1, a(i+4), false)
		x2, y2 = x1+a(i+5), y1+a(i+6)
		x3, y3 = x2, y2
		along(&x3, &y3, a(i+7), true)
		if n-i < 16 && n&1 != 0 {
			along(&x3, &y3, a(i+8), false)
		}
		in.curveTo(x1, y1, x2, y2, x3, y3)
	}
}

// flex is the four flex operators, each two curves, which HarfBuzz draws as
// the curves whatever flex depth they ask for. A flex with any other count of
// operands than its own is an error.
func (in *t2Interp) flex(op int) {
	a := func(i int) float64 { return in.args[i] }
	want := 0
	switch op {
	case t2HFlex:
		want = 7
	case t2Flex:
		want = 13
	case t2HFlex1:
		want = 9
	case t2Flex1:
		want = 11
	}
	if in.n != want {
		in.err = true
		return
	}
	var p [6][2]float64
	x0, y0 := in.x, in.y
	switch op {
	case t2HFlex:
		p[0] = [2]float64{x0 + a(0), y0}
		p[1] = [2]float64{p[0][0] + a(1), p[0][1] + a(2)}
		p[2] = [2]float64{p[1][0] + a(3), p[1][1]}
		p[3] = [2]float64{p[2][0] + a(4), p[2][1]}
		p[4] = [2]float64{p[3][0] + a(5), p[0][1]}
		p[5] = [2]float64{p[4][0] + a(6), p[4][1]}
	case t2Flex:
		x, y := x0, y0
		for k := range 6 {
			x, y = x+a(2*k), y+a(2*k+1)
			p[k] = [2]float64{x, y}
		}
	case t2HFlex1:
		p[0] = [2]float64{x0 + a(0), y0 + a(1)}
		p[1] = [2]float64{p[0][0] + a(2), p[0][1] + a(3)}
		p[2] = [2]float64{p[1][0] + a(4), p[1][1]}
		p[3] = [2]float64{p[2][0] + a(5), p[2][1]}
		p[4] = [2]float64{p[3][0] + a(6), p[3][1] + a(7)}
		p[5] = [2]float64{p[4][0] + a(8), y0}
	case t2Flex1:
		var dx, dy float64
		x, y := x0, y0
		for k := range 5 {
			dx, dy = dx+a(2*k), dy+a(2*k+1)
			x, y = x+a(2*k), y+a(2*k+1)
			p[k] = [2]float64{x, y}
		}
		if math.Abs(dx) > math.Abs(dy) {
			p[5] = [2]float64{p[4][0] + a(10), y0}
		} else {
			p[5] = [2]float64{x0, p[4][1] + a(10)}
		}
	}
	in.curveTo(p[0][0], p[0][1], p[1][0], p[1][1], p[2][0], p[2][1])
	in.curveTo(p[3][0], p[3][1], p[4][0], p[4][1], p[5][0], p[5][1])
}

// seac draws the two glyphs endchar names when it has four operands or more:
// the last four are the accent's offset and the StandardEncoding codes of the
// base and the accent. A seac inside a glyph a seac names, a code that names
// no glyph, and a component that cannot be measured are each an error, as
// they are in HarfBuzz.
func (in *t2Interp) seac() {
	n := in.n
	dx, dy := in.args[n-4], in.args[n-3]
	o := in.run.o
	base := o.stdCodeToGlyph(toIntClamped(in.args[n-2]))
	accent := o.stdCodeToGlyph(toIntClamped(in.args[n-1]))
	if in.inSeac || base == 0 || accent == 0 {
		in.err = true
		return
	}
	bb, ok := in.run.bounds(base, true)
	if !ok {
		in.err = true
		return
	}
	ab, ok := in.run.bounds(accent, true)
	if !ok {
		in.err = true
		return
	}
	in.bounds.merge(bb)
	ab.offset(dx, dy)
	in.bounds.merge(ab)
}

// toIntClamped is HarfBuzz's number_t::to_int: the integer part, and the
// nearer end of an int's range for a number past it.
func toIntClamped(v float64) int {
	switch {
	case v < math.MinInt32:
		return math.MinInt32
	case v > math.MaxInt32:
		return math.MaxInt32
	}
	return int(v)
}
