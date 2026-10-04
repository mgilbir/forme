package shape

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/mgilbir/forme/font"
)

// Writing a CFF2 font again as a CFF one, cut at one location in its design
// space.
//
// A face is embedded as what it draws, and a document format that predates
// CFF2 cannot carry CFF2: PDF gained it only in 2.0, and PDF/A-1 to -3 and
// every reader before those have only CFF. So a face whose outlines are CFF2
// is loaded as the CFF font that draws what it draws at its location — its
// default for Load, the location asked for by LoadInstance — and everything
// downstream, from measuring its ink to subsetting and embedding it, is the
// CFF path it always was.
//
// # What is written
//
// A CID-keyed CFF, as fontTools' CFF2ToCFF writes one: the ROS
// Adobe-Identity-0, each glyph's CID its glyph index, the CFF2 font's Font
// DICTs and FDSelect as they were, and a Private DICT for each with every
// blended value resolved. A CID-keyed font is what a CFF2 font already is —
// every glyph drawn through a Font DICT — and Identity is the honest
// collection for it: a CFF2 font has no charset and no collection of its own,
// and its glyphs are numbered by index and nothing else.
//
// Each charstring is the glyph as the CFF2 one draws it at the location, with
// its hints, written in the operators every CFF reader reads: rmoveto, rlineto
// and rrcurveto for the outline, the stem operators and the masks as they were,
// and a width where the glyph's advance is not its Font DICT's default. Its
// subroutines are run rather than kept, as fontTools' instancer runs them: a
// blend in a subroutine can move numbers its caller pushed, so a subroutine at
// a location is not one thing. The outline is written from the points the
// charstring draws, each segment from the point before it, so every point of
// the CFF glyph is a point of the CFF2 one; the numbers are whole where the
// location's blends make them whole and 16.16 where the font's defaults are
// not, and both are exact.
//
// The blends are resolved as fontTools resolves them (cff2.go): its instancer
// rounds what each number's deltas come to at the location to a whole number
// before it is added to the number, and an instance cut here is its instance
// point for point (cff2_test.go). At the default instance nothing is blended
// and nothing rounded: the CFF font draws exactly what the CFF2 one does.
//
// A blended Private DICT number is resolved the same way, with one difference
// from fontTools. BlueValues and the other zone and stem arrays are written as
// differences, each from the one before, and a blend leaves its values for
// the operator after it — so a delta that moves one value moves every value
// after it. fontTools adds each value's own delta to its absolute value
// instead. The two agree wherever nothing moves, which is every default
// instance.
//
// # What a CFF charstring cannot say
//
// Type 2 holds 48 operands and 96 stems. A CFF2 charstring holds 513 and has no
// limit on stems. The outline is split into operators of at most 24 lines or 8
// curves, which says the same points; a stem operator into several, each
// restarting from zero as a new operator does, which declares the same stems.
// A glyph declaring more than 96 stems is written without its hints — its
// outline unchanged — because a CFF reader may refuse its masks; fontTools
// writes them anyway.
//
// A charstring HarfBuzz cannot run — one that underflows its stack, calls a
// subroutine that is not there, or runs past 200,000 operators — has no
// outline HarfBuzz would measure, and is written as an empty glyph at its
// advance; so is one that moves further in one step than a Type 2 number
// reaches. Which glyphs those were is reported (Face.LayoutLimits), since the
// font said something about them this does not.

// cff2Converted is a CFF2 font written as a CFF one.
type cff2Converted struct {
	cff []byte
	// bounds are each glyph's bounds as fontTools' BoundsPen measures them —
	// its points and the extremes of its curves, and a lone move's point —
	// in font units, and set false for a glyph that draws nothing.
	bounds []floatBounds
	// limits say which glyphs were written empty, and why.
	limits []string
}

// maxCFF2Stems is the stems a Type 2 charstring may declare.
const maxCFF2Stems = 96

// cffConvertWork bounds converting one font: every operator its charstrings
// run, subroutines included, and every operand its blends read, charged to one
// budget the size of the ink budget (cffInkWork) — sixteen a byte of the
// table. A real font runs each byte of its charstrings about once.
func cffConvertWork(tableLen int) int { return cffInkWork(tableLen) }

// maxCFF2Charstrings bounds the charstrings written from one CFF2 font, which
// a crafted one could make many times its own size by calling a subroutine
// over and over: half of what the CFF written may come to, leaving the rest
// for its offsets and DICTs.
const maxCFF2Charstrings = maxCFFSize / 2

// cff2Writer writes a CFF2 font's glyphs as CFF charstrings at one location,
// a glyph at a time: every glyph for an instance, and for a face read at its
// default only the glyphs something asks for (cff2Default).
type cff2Writer struct {
	f        *cff2Font
	blend    *cff2Blend
	advances []int
	// defaults and nominals are each Font DICT's default and nominal width:
	// the advance most of its glyphs have, so that most need no width of their
	// own, and what a width is written as a difference from.
	defaults, nominals []int
	run                t2Run
	events             []t2Event
	hintArgs           []float64
	// written is how many bytes of charstrings have been written, and
	// sizeLimit how many may be: maxCFF2Charstrings.
	written, sizeLimit int
	// Why glyphs were written empty: which could not be run, and whether the
	// work or the size ran out.
	failed      []int
	spent, full bool
}

func newCFF2Writer(f *cff2Font, blend *cff2Blend, advances []int, budget *font.Budget) (*cff2Writer, error) {
	n := len(f.charStrings)
	if len(advances) != n {
		return nil, fmt.Errorf("fonts: %d advances for %d CFF2 glyphs", len(advances), n)
	}
	if n == 0 {
		return nil, errors.New("fonts: a CFF2 font with no glyphs, not even .notdef")
	}
	nFD := len(f.locals)
	w := &cff2Writer{f: f, blend: blend, advances: advances, sizeLimit: maxCFF2Charstrings,
		defaults: make([]int, nFD), nominals: make([]int, nFD)}
	counts := make([]map[int]int, nFD)
	lo, hi := make([]int, nFD), make([]int, nFD)
	for g, fd := range f.fds {
		if counts[fd] == nil {
			counts[fd] = map[int]int{}
			lo[fd], hi[fd] = advances[g], advances[g]
		}
		counts[fd][advances[g]]++
		lo[fd], hi[fd] = min(lo[fd], advances[g]), max(hi[fd], advances[g])
	}
	for fd, c := range counts {
		best, bestN := 0, -1
		for adv, k := range c {
			if k > bestN || (k == bestN && adv < best) {
				best, bestN = adv, k
			}
		}
		w.defaults[fd], w.nominals[fd] = best, best
		// A width is one Type 2 number, a difference from the nominal width
		// of at most 32,767 either way. A Font DICT whose advances spread
		// further from its commonest is written against their middle, and one
		// they spread further than that from cannot be written at all.
		if lo[fd] < best-math.MaxInt16 || hi[fd] > best+math.MaxInt16 {
			mid := (lo[fd] + hi[fd]) / 2
			if lo[fd] < mid-math.MaxInt16 || hi[fd] > mid+math.MaxInt16 {
				return nil, fmt.Errorf("fonts: the advances of a CFF2 Font DICT run from %d to %d, "+
					"further apart than a CFF charstring can state a width", lo[fd], hi[fd])
			}
			w.nominals[fd] = mid
		}
	}
	w.run = t2Run{o: f.outlines(blend), budget: budget, draw: true,
		path: func(s t2Seg) { w.events = append(w.events, t2Event{seg: s}) },
		hints: func(op int, args []float64, mask []byte) {
			start := len(w.hintArgs)
			w.hintArgs = append(w.hintArgs, args...)
			w.events = append(w.events, t2Event{op: op, args: w.hintArgs[start:len(w.hintArgs):len(w.hintArgs)], mask: mask})
		},
	}
	return w, nil
}

// glyph writes glyph g as a CFF charstring, and returns its bounds. A glyph
// whose charstring cannot be run, or whose CFF form cannot state a number it
// draws, or that comes after the work or the size allowed ran out, is written
// as an empty glyph at its advance, and said to be (limits).
func (w *cff2Writer) glyph(g int) ([]byte, floatBounds) {
	fd := w.f.fds[g]
	w.events, w.hintArgs = w.events[:0], w.hintArgs[:0]
	ok := false
	if !w.spent && !w.full {
		w.run.capped = false
		_, ok = w.run.bounds(g, false)
		w.spent = w.run.spent
	}
	var code []byte
	var err error
	if ok {
		code, err = writeType2Glyph(w.events, w.advances[g], w.defaults[fd], w.nominals[fd])
		if err == nil && w.written+len(code) > w.sizeLimit {
			w.full, err = true, errors.New("past the size allowed")
		}
	}
	if !ok || err != nil {
		if !w.spent && !w.full {
			w.failed = append(w.failed, g)
		}
		w.events = w.events[:0]
		// Nothing but a width and endchar, which newCFF2Writer made sure
		// every advance can be written as.
		code, _ = writeType2Glyph(nil, w.advances[g], w.defaults[fd], w.nominals[fd])
	}
	w.written += len(code)
	return code, penBounds(w.events)
}

// privates writes each Font DICT's Private DICT in CFF's form.
func (w *cff2Writer) privates() ([][]byte, error) {
	out := make([][]byte, len(w.f.locals))
	for fd := range out {
		p, err := cff1Private(w.f.privates[fd], w.blend, w.f.ivs[fd], w.defaults[fd], w.nominals[fd])
		if err != nil {
			return nil, err
		}
		out[fd] = p
	}
	return out, nil
}

// limits says which glyphs were written empty, and why, in words a caller can
// report. See Face.LayoutLimits.
func (w *cff2Writer) limits() []string {
	var out []string
	if n := len(w.failed); n > 0 {
		shown := w.failed[:min(n, 8)]
		more := ""
		if n > len(shown) {
			more = ", …"
		}
		out = append(out, fmt.Sprintf("%d glyphs of the CFF2 outlines (%v%s) have charstrings HarfBuzz "+
			"cannot run, or that draw a number a CFF charstring cannot state, and are embedded as empty glyphs",
			n, shown, more))
	}
	if w.spent {
		out = append(out, "writing the CFF2 outlines as CFF ran past the work one font may cost, "+
			"so the glyphs written after that are embedded as empty glyphs")
	}
	if w.full {
		out = append(out, "writing the CFF2 outlines as CFF ran past the size one font may come to, "+
			"so the glyphs written after that are embedded as empty glyphs")
	}
	return out
}

// cff2ToCFF writes f at the location blend resolves, as a CFF font named name
// whose glyph g advances advances[g]. Running the charstrings is charged to
// budget.
func cff2ToCFF(f *cff2Font, blend *cff2Blend, advances []int, name string, budget *font.Budget) (*cff2Converted, error) {
	w, err := newCFF2Writer(f, blend, advances, budget)
	if err != nil {
		return nil, err
	}
	n := len(f.charStrings)
	conv := &cff2Converted{bounds: make([]floatBounds, n)}
	glyphs := make([][]byte, n)
	for g := range glyphs {
		glyphs[g], conv.bounds[g] = w.glyph(g)
	}
	privates, err := w.privates()
	if err != nil {
		return nil, err
	}
	var box floatBounds
	for _, b := range conv.bounds {
		if b.set {
			box.add(b.xMin, b.yMin)
			box.add(b.xMax, b.yMax)
		}
	}
	if conv.cff, err = assembleCIDCFF(name, glyphs, identityCIDs(n), f.fds, privates, roundOut(box), f.fontMatrix); err != nil {
		return nil, err
	}
	conv.limits = w.limits()
	return conv, nil
}

// roundOut is a box in whole units that holds b: fontTools' intRect.
func roundOut(b floatBounds) [4]int {
	if !b.set {
		return [4]int{}
	}
	return [4]int{int(math.Floor(b.xMin)), int(math.Floor(b.yMin)), int(math.Ceil(b.xMax)), int(math.Ceil(b.yMax))}
}

// identityCIDs is 0, 1, … n−1: each glyph's CID its own index.
func identityCIDs(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// t2Event is one thing a charstring does that its CFF form has to say again:
// a segment of its outline, or a hint — a stem operator and its operands, or
// a mask operator with the stems its operands declare and its bytes.
type t2Event struct {
	seg  t2Seg
	op   int
	args []float64
	mask []byte
}

// writeType2Glyph writes a glyph's events as a Type 2 charstring advancing
// width, in a Font DICT whose default and nominal widths are def and nominal.
func writeType2Glyph(events []t2Event, width, def, nominal int) ([]byte, error) {
	var out []byte
	var err error
	num := func(v float64) {
		if err == nil {
			out, err = appendType2Number(out, v)
		}
	}
	widthPending := width != def
	emitWidth := func() {
		if widthPending {
			num(float64(width - nominal))
			widthPending = false
		}
	}
	stems := 0
	for _, e := range events {
		if e.seg.op == 0 {
			stems += len(e.args) / 2
		}
	}
	hinted := stems <= maxCFF2Stems
	// stemOps writes a stem operator's operands in operators of at most 22
	// stems. Each operator's first edge is from zero, so a later one starts
	// from where the stems before it left off, added up.
	stemOps := func(op int, args []float64) {
		args = args[:len(args)&^1]
		pos := 0.0
		for start := 0; start < len(args); start += 44 {
			end := min(start+44, len(args))
			emitWidth()
			for i := start; i < end; i++ {
				v := args[i]
				if i == start && start > 0 {
					v += pos
				}
				num(v)
			}
			for i := start; i < end; i++ {
				pos += args[i]
			}
			out = append(out, byte(op))
		}
	}
	cx, cy := 0.0, 0.0
	moved := false
	for i := 0; i < len(events) && err == nil; i++ {
		e := events[i]
		switch e.seg.op {
		case 0:
			if !hinted {
				continue
			}
			if e.mask == nil {
				stemOps(e.op, e.args)
				continue
			}
			if len(e.args) >= 2 {
				stemOps(23, e.args) // the stems a first mask declares: vstemhm
			}
			emitWidth()
			out = append(out, byte(e.op))
			out = append(out, e.mask...)
		case 'M':
			emitWidth()
			num(e.seg.pts[0] - cx)
			num(e.seg.pts[1] - cy)
			out = append(out, 21) // rmoveto
			cx, cy, moved = e.seg.pts[0], e.seg.pts[1], true
		case 'L', 'C':
			if !moved {
				// A path that starts without a move starts at the origin.
				emitWidth()
				num(0)
				num(0)
				out = append(out, 21)
				moved = true
			}
			// The run of segments of this kind, as many as one operator holds:
			// 24 lines, or 8 curves, which is 48 operands either way.
			op, per := byte(5), 2 // rlineto
			if e.seg.op == 'C' {
				op, per = 8, 6 // rrcurveto
			}
			for k := 0; k < 48/per && i < len(events) && events[i].seg.op == e.seg.op; k, i = k+1, i+1 {
				p := events[i].seg.pts
				for j := 0; j < per; j += 2 {
					num(p[j] - cx)
					num(p[j+1] - cy)
					cx, cy = p[j], p[j+1]
				}
			}
			i--
			out = append(out, op)
		}
	}
	emitWidth()
	out = append(out, 14) // endchar
	return out, err
}

// appendType2Number appends v as a Type 2 charstring operand: an integer in
// its shortest form, anything else as 16.16 fixed. A number neither can hold —
// past what sixteen bits of integer part say, or finer than a sixty-five
// thousand five hundred and thirty-sixth — is refused rather than written as
// some other number.
func appendType2Number(dst []byte, v float64) ([]byte, error) {
	if v == math.Trunc(v) && v >= math.MinInt16 && v <= math.MaxInt16 {
		switch i := int(v); {
		case i >= -107 && i <= 107:
			return append(dst, byte(i+139)), nil
		case i >= 108 && i <= 1131:
			i -= 108
			return append(dst, byte(i>>8+247), byte(i)), nil
		case i >= -1131 && i <= -108:
			i = -i - 108
			return append(dst, byte(i>>8+251), byte(i)), nil
		default:
			return append(dst, 28, byte(i>>8), byte(i)), nil
		}
	}
	f := v * 65536
	if f != math.Trunc(f) || f < math.MinInt32 || f > math.MaxInt32 {
		return dst, fmt.Errorf("%v cannot be written as a Type 2 number", v)
	}
	return binary.BigEndian.AppendUint32(append(dst, 255), uint32(int32(f))), nil
}

// penBounds is what fontTools' BoundsPen measures of a glyph: every point a
// move, a line or a curve ends at, and each curve's extremes where its control
// points lie outside the box so far — its bounds, not its control box. It is
// what the instance's font-wide box and its side bearings are written from.
func penBounds(events []t2Event) floatBounds {
	var b floatBounds
	var cx, cy float64
	for _, e := range events {
		p := e.seg.pts
		switch e.seg.op {
		case 'M', 'L':
			b.add(p[0], p[1])
			cx, cy = p[0], p[1]
		case 'C':
			b.add(p[4], p[5])
			if !b.contains(p[0], p[1]) || !b.contains(p[2], p[3]) {
				cubicBounds(&b, cx, cy, p[0], p[1], p[2], p[3], p[4], p[5])
			}
			cx, cy = p[4], p[5]
		}
	}
	return b
}

func (f *floatBounds) contains(x, y float64) bool {
	return f.set && f.xMin <= x && x <= f.xMax && f.yMin <= y && y <= f.yMax
}

// cubicBounds adds a cubic's bounds to b: its end points and the points where
// its derivative is zero, found as fontTools' calcCubicBounds finds them, in
// its order of operations, so that an extreme lands on the same double.
func cubicBounds(b *floatBounds, x1, y1, x2, y2, x3, y3, x4, y4 float64) {
	// calcCubicParameters.
	cx := float64(x2-x1) * 3.0
	cy := float64(y2-y1) * 3.0
	bx := float64(float64(x3-x2)*3.0) - cx
	by := float64(float64(y3-y2)*3.0) - cy
	ax := float64(float64(x4-x1)-cx) - bx
	ay := float64(float64(y4-y1)-cy) - by
	var roots []float64
	for _, t := range solveQuadratic(ax*3.0, bx*2.0, cx) {
		if 0 <= t && t < 1 {
			roots = append(roots, t)
		}
	}
	for _, t := range solveQuadratic(ay*3.0, by*2.0, cy) {
		if 0 <= t && t < 1 {
			roots = append(roots, t)
		}
	}
	for _, t := range roots {
		x := float64(float64(float64(float64(float64(ax*t)*t)*t)+float64(float64(bx*t)*t))+float64(cx*t)) + x1
		y := float64(float64(float64(float64(float64(ay*t)*t)*t)+float64(float64(by*t)*t))+float64(cy*t)) + y1
		b.add(x, y)
	}
	b.add(x1, y1)
	b.add(x4, y4)
}

// solveQuadratic is fontTools' bezierTools.solveQuadratic: the real roots of
// a·x² + b·x + c, a linear equation's one root where a is within 1e-9 of zero,
// and none where b is too.
func solveQuadratic(a, b, c float64) []float64 {
	const epsilon = 1e-9
	if math.Abs(a) < epsilon {
		if math.Abs(b) < epsilon {
			return nil
		}
		return []float64{-c / b}
	}
	dd := float64(b*b) - float64(float64(4.0*a)*c)
	if dd < 0 {
		return nil
	}
	r := math.Sqrt(dd)
	return []float64{float64(float64(-b+r)/2.0) / a, float64(float64(-b-r)/2.0) / a}
}

// cff1Private writes a Font DICT's Private DICT in CFF's form: its entries as
// the CFF2 font wrote them, a blended one resolved at the location, with the
// default and nominal widths the glyphs' widths are written against. Only the
// entries a CFF Private DICT has are written (cff1PrivateOps): not the local
// subroutines, which are run rather than kept, nor vsindex, nor anything else
// a CFF2 DICT can hold and a CFF one cannot — vstore is operator 24 there, and
// 24 is a reserved byte in a CFF DICT, which a reader refuses.
func cff1Private(entries []cff2Entry, blend *cff2Blend, ivs, def, nominal int) ([]byte, error) {
	var out []byte
	for _, e := range entries {
		if !cff1PrivateOps[e.op] {
			continue
		}
		if !e.blended {
			out = append(out, e.raw...)
			continue
		}
		for _, v := range e.operands {
			x := v.v
			if v.deltas != nil {
				x = blend.resolve(v.v, v.deltas, ivs)
			}
			b, err := dictNumber(x)
			if err != nil {
				return nil, err
			}
			out = append(out, b...)
		}
		if e.op >= 1200 {
			out = append(out, 12, byte(e.op-1200))
		} else {
			out = append(out, byte(e.op))
		}
	}
	d, err := dictNumber(float64(def))
	if err != nil {
		return nil, err
	}
	nw, err := dictNumber(float64(nominal))
	if err != nil {
		return nil, err
	}
	out = append(append(out, d...), 20)  // defaultWidthX
	out = append(append(out, nw...), 21) // nominalWidthX
	return out, nil
}

// cff1PrivateOps are the operators of a CFF Private DICT this writer copies,
// as fontTools' privateDictOperators lists them: the blue zones, the standard
// and snapped stems, BlueScale, BlueShift, BlueFuzz, ForceBold,
// LanguageGroup, ExpansionFactor and initialRandomSeed. Subrs, defaultWidthX
// and nominalWidthX are not among them: the subroutines are run rather than
// kept, and the widths are written after the rest.
var cff1PrivateOps = map[int]bool{
	6: true, 7: true, 8: true, 9: true, // BlueValues, OtherBlues, FamilyBlues, FamilyOtherBlues
	10: true, 11: true, // StdHW, StdVW
	1209: true, 1210: true, 1211: true, // BlueScale, BlueShift, BlueFuzz
	1212: true, 1213: true, 1214: true, // StemSnapH, StemSnapV, ForceBold
	1217: true, 1218: true, 1219: true, // LanguageGroup, ExpansionFactor, initialRandomSeed
}

// dictNumber writes v as a DICT operand: an integer in its shortest form, and
// anything else as the binary-coded decimal of its shortest spelling.
func dictNumber(v float64) ([]byte, error) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return nil, errors.New("fonts: a blended CFF2 DICT value that is not a number")
	}
	if v == math.Trunc(v) && v >= math.MinInt32 && v <= math.MaxInt32 {
		i := int(v)
		switch {
		case i >= -107 && i <= 107:
			return []byte{byte(i + 139)}, nil
		case i >= 108 && i <= 1131:
			i -= 108
			return []byte{byte(i>>8 + 247), byte(i)}, nil
		case i >= -1131 && i <= -108:
			i = -i - 108
			return []byte{byte(i>>8 + 251), byte(i)}, nil
		case i >= math.MinInt16 && i <= math.MaxInt16:
			return []byte{28, byte(i >> 8), byte(i)}, nil
		}
		return cffInt(i), nil
	}
	s := strconv.FormatFloat(v, 'g', -1, 64)
	var nibbles []byte
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			nibbles = append(nibbles, c-'0')
		case c == '.':
			nibbles = append(nibbles, 0xA)
		case c == '-':
			nibbles = append(nibbles, 0xE)
		case c == 'e':
			if i+1 < len(s) && s[i+1] == '-' {
				nibbles = append(nibbles, 0xC)
				i++
			} else {
				nibbles = append(nibbles, 0xB)
				if i+1 < len(s) && s[i+1] == '+' {
					i++
				}
			}
		}
	}
	nibbles = append(nibbles, 0xF)
	if len(nibbles)%2 == 1 {
		nibbles = append(nibbles, 0xF)
	}
	out := []byte{30}
	for i := 0; i < len(nibbles); i += 2 {
		out = append(out, nibbles[i]<<4|nibbles[i+1])
	}
	return out, nil
}

// assembleCIDCFF writes a CID-keyed CFF: the ROS Adobe-Identity-0, glyph g's
// CID cids[g] and Font DICT fds[g], its charstring glyphs[g], each Font DICT's
// Private DICT, and the font's box and FontMatrix entry. CIDCount is one past
// the largest CID.
func assembleCIDCFF(name string, glyphs [][]byte, cids, fds []int, privates [][]byte, bbox [4]int, fontMatrix []byte) ([]byte, error) {
	charset := cidCharset(cids)
	cidCount := 1
	for _, c := range cids {
		cidCount = max(cidCount, c+1)
	}
	fdSelect := writeFDSelect(fds)
	charStrings := writeCFFIndex(glyphs)
	top := func(charsetAt, charStringsAt, fdArrayAt, fdSelectAt int) []byte {
		d := append(cffInt(391), cffInt(392)...)
		d = append(append(d, cffInt(0)...), 12, 30) // ROS Adobe-Identity-0
		for _, v := range bbox {
			d = append(d, cffInt(v)...)
		}
		d = append(d, 5) // FontBBox
		d = append(d, fontMatrix...)
		d = append(append(d, cffInt(cidCount)...), 12, 34) // CIDCount
		d = append(append(d, cffInt(charsetAt)...), 15)
		d = append(append(d, cffInt(charStringsAt)...), 17)
		d = append(append(d, cffInt(fdArrayAt)...), 12, 36)
		return append(append(d, cffInt(fdSelectAt)...), 12, 37)
	}
	fontDicts := func(privAt []int) []byte {
		dicts := make([][]byte, len(privates))
		for i, p := range privates {
			d := append(cffInt(len(p)), cffInt(privAt[i])...)
			dicts[i] = append(d, 18)
		}
		return writeCFFIndex(dicts)
	}
	head := func(topDict []byte) []byte {
		out := []byte{1, 0, 4, 4}
		out = append(out, writeCFFIndex([][]byte{[]byte(name)})...)
		out = append(out, writeCFFIndex([][]byte{topDict})...)
		out = append(out, writeCFFIndex([][]byte{[]byte("Adobe"), []byte("Identity")})...)
		return append(out, writeCFFIndex(nil)...) // no global subroutines
	}
	charsetAt := len(head(top(0, 0, 0, 0)))
	fdSelectAt := charsetAt + len(charset)
	charStringsAt := fdSelectAt + len(fdSelect)
	fdArrayAt := charStringsAt + len(charStrings)
	privAt := make([]int, len(privates))
	at := fdArrayAt + len(fontDicts(privAt))
	for i, p := range privates {
		privAt[i] = at
		at += len(p)
	}
	if at > maxCFFSize {
		return nil, fmt.Errorf("fonts: the CFF2 font written as CFF runs past %d bytes", maxCFFSize)
	}
	out := make([]byte, 0, at)
	out = append(out, head(top(charsetAt, charStringsAt, fdArrayAt, fdSelectAt))...)
	out = append(out, charset...)
	out = append(out, fdSelect...)
	out = append(out, charStrings...)
	out = append(out, fontDicts(privAt)...)
	for _, p := range privates {
		out = append(out, p...)
	}
	if len(out) != at {
		return nil, errors.New("fonts: internal: the CFF written from a CFF2 font did not come out where it was measured")
	}
	return out, nil
}

// cff2Tables is the sfnt of a CFF2 font as the CFF font cff, around its
// tables: the CFF2 table replaced, post brought to version 3 (a version 2 post
// names glyphs, and a CID-keyed CFF names none), and the variation tables
// dropped, since the outlines they vary no longer vary.
func cff2Tables(tables map[string][]byte, cff []byte) map[string][]byte {
	out := map[string][]byte{}
	for tag, b := range tables {
		if !instanceDropped[tag] {
			out[tag] = b
		}
	}
	out["CFF "] = cff
	if post := out["post"]; len(post) >= 32 && font.Be32(post, 0) == 0x00020000 {
		p := append([]byte(nil), post[:32]...)
		binary.BigEndian.PutUint32(p, 0x00030000)
		out["post"] = p
	}
	return out
}

// cff2Default is a CFF2 font read at its default instance, which is what Load
// reads one as: the CFF font it draws there, written a glyph at a time as
// something asks for one.
//
// Nothing is blended at the default, so each glyph's CFF charstring draws
// exactly what its CFF2 one does, and the font's own metrics — its advances,
// its box, its side bearings — are the default's already, and are used as the
// font states them.
//
// A glyph at a time because writing the whole font is the whole font's
// charstrings run: 370 ms for Noto Sans JP's variable font, forty times what
// loading a static CFF face of it costs. A face measures a glyph's ink when it
// is set, and a subset writes the glyphs it keeps; only Program writes them
// all, the first time it is asked, and keeps the result.
//
// It is shared by every Clone of the face, as the ink is; the lock is there
// because glyphs are written lazily.
type cff2Default struct {
	tables map[string][]byte
	name   string

	mu     sync.Mutex
	w      *cff2Writer
	glyphs map[int][]byte

	once    sync.Once
	program []byte
}

// newCFF2Default reads a CFF2 font's table, whose structures cost is charged
// to budget; its charstrings are left to be written as they are asked for,
// under a work budget of their own.
func newCFF2Default(tables map[string][]byte, budget *font.Budget) (*cff2Default, error) {
	maxp, hhea := tables["maxp"], tables["hhea"]
	if len(maxp) < 6 || len(hhea) < 36 || tables["hmtx"] == nil {
		return nil, errors.New("fonts: the font lacks hhea, hmtx or maxp")
	}
	n := font.Be16(maxp, 4)
	advances, _, err := parseHmtx(tables["hmtx"], font.Be16(hhea, 34), n)
	if err != nil {
		return nil, err
	}
	f, err := readCFF2(tables["CFF2"], n, budget)
	if err != nil {
		return nil, err
	}
	w, err := newCFF2Writer(f, newFontToolsBlend(f, nil, nil), advances,
		font.NewBudget(cffConvertWork(len(tables["CFF2"]))))
	if err != nil {
		return nil, err
	}
	name := postScriptName(tables["name"])
	if name == "" {
		name = "CFF2"
	}
	return &cff2Default{tables: tables, name: name, w: w, glyphs: map[int][]byte{}}, nil
}

// glyph is glyph g's CFF charstring, written the first time it is asked for.
func (d *cff2Default) glyph(g int) []byte {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.glyphLocked(g)
}

func (d *cff2Default) glyphLocked(g int) []byte {
	if code, ok := d.glyphs[g]; ok {
		return code
	}
	code, _ := d.w.glyph(g)
	d.glyphs[g] = code
	return code
}

// numGlyphs is how many glyphs the font has.
func (d *cff2Default) numGlyphs() int { return len(d.w.advances) }

// cff writes the CFF font of the glyphs order, in that order: CID-keyed, each
// glyph's CID its index in the CFF2 font, and each in the Font DICT it was in.
// The whole font is order 0, 1, … n−1; a subset is .notdef and the glyphs it
// keeps. Its box is the font's, which the default instance states.
func (d *cff2Default) cff(order []int) ([]byte, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	glyphs := make([][]byte, len(order))
	fds := make([]int, len(order))
	for i, g := range order {
		glyphs[i] = d.glyphLocked(g)
		fds[i] = d.w.f.fds[g]
	}
	privates, err := d.w.privates()
	if err != nil {
		return nil, err
	}
	var bbox [4]int
	if head := d.tables["head"]; len(head) >= 54 {
		for i := range bbox {
			bbox[i] = signed16(font.Be16(head, 36+2*i))
		}
	}
	return assembleCIDCFF(d.name, glyphs, order, fds, privates, bbox, d.w.f.fontMatrix)
}

// programBytes is the whole font written as CFF — Program's answer for the
// face — the first time it is asked for, and nil if it cannot be written,
// which a font Load accepted cannot come to: every glyph that cannot be is
// written empty and reported.
func (d *cff2Default) programBytes() []byte {
	d.once.Do(func() {
		cff, err := d.cff(identityCIDs(d.numGlyphs()))
		if err != nil {
			return
		}
		d.program = assembleOTTO(cff2Tables(d.tables, cff))
	})
	return d.program
}

// subset is a subset of the font as CFF, keeping the glyphs keep says: the
// CID-keyed subset cffrenumber.go describes, written directly from the kept
// glyphs — renumbered in their order, each keeping its glyph index as its
// CID, and the sfnt's glyph-indexed tables renumbered to match.
func (d *cff2Default) subset(keep []bool, cmap map[rune]int) ([]byte, error) {
	order := []int{0}
	for g := 1; g < len(keep); g++ {
		if keep[g] {
			order = append(order, g)
		}
	}
	cff, err := d.cff(order)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, tag := range []string{"cmap", "head", "hhea", "hmtx", "maxp", "name", "post", "OS/2"} {
		if b, ok := d.tables[tag]; ok {
			out[tag] = b
		}
	}
	out["CFF "] = cff
	if err := renumberSFNT(out, order, len(keep), cmap); err != nil {
		return nil, err
	}
	return assembleOTTO(out), nil
}

// limits says which glyphs written so far were written empty, and why.
func (d *cff2Default) limits() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.w.limits()
}

// instanceCFF2 is LoadInstance's rewrite for a font whose outlines are CFF2:
// the font written as the CFF font it draws at the location, with the metrics
// of what it draws there. It returns the program, the normalized location,
// and what the writing reports: the glyphs written empty, and why.
//
// What moves, and by what:
//
//   - The outlines, by the charstrings' own blends, resolved as fontTools'
//     instancer resolves them (cff2.go) and written as CFF (cff2ToCFF).
//   - The advances, by HVAR, and the vertical advances by VVAR, as HarfBuzz
//     reads them and as instance.go moves a glyf font's: the stated advance
//     plus the delta rounded half up. fontTools rounds the delta half to even;
//     the two differ only for a delta of exactly half a unit, and for such a
//     glyph this one is set where HarfBuzz sets it at the location.
//   - The vertical origins VORG states, by VVAR's origin deltas, as HarfBuzz
//     moves them. fontTools' instancer leaves VORG as the default instance's
//     and says so; a face cut at a heavy weight would then hang its glyphs
//     from where the Thin hangs them.
//   - The side bearings, the font's box and hhea's extremes, measured from the
//     instance's outlines — each glyph's bounds as fontTools' BoundsPen
//     measures them, rounded out to whole units. fontTools' instancer leaves
//     all three as the default instance's for a CFF2 font, and its box as
//     zero once the font is downgraded to CFF; a face reports its box to a
//     document (Descriptor), which is what it has to be right for.
//   - The font-wide numbers, by MVAR, and the weight, width and italic angle,
//     and the PostScript name, as for a glyf font (instance.go).
func instanceCFF2(tables map[string][]byte, want map[string]float64) ([]byte, []float64, []string, error) {
	fvar := tables["fvar"]
	if fvar == nil {
		return nil, nil, nil, errors.New("fonts: the font has no fvar table, so it is not a variable font and has no design space to be loaded at")
	}
	head, hhea, hmtx, maxp := tables["head"], tables["hhea"], tables["hmtx"], tables["maxp"]
	if len(head) < 54 || len(hhea) < 36 || len(maxp) < 6 || hmtx == nil {
		return nil, nil, nil, errors.New("fonts: the font lacks head, hhea, hmtx or maxp")
	}
	axes, err := parseFvar(fvar)
	if err != nil {
		return nil, nil, nil, err
	}
	coords, tags, err := cff2Location(fvar, tables["avar"], want)
	if err != nil {
		return nil, nil, nil, err
	}
	numGlyphs := font.Be16(maxp, 4)
	advances, _, err := parseHmtx(hmtx, font.Be16(hhea, 34), numGlyphs)
	if err != nil {
		return nil, nil, nil, err
	}
	if t := tables["HVAR"]; t != nil {
		hvar, err := parseHVAR(t)
		if err != nil {
			return nil, nil, nil, err
		}
		for g := range advances {
			advances[g] = max(advances[g]+otRound(hvar.advanceDelta(g, coords)), 0)
		}
	}

	var name []byte
	if name, err = instanceName(tables["name"], fvar, axes, want); err != nil {
		return nil, nil, nil, err
	}
	psName := postScriptName(tables["name"])
	if name != nil {
		psName = postScriptName(name)
	}
	budget := font.NewBudget(cffConvertWork(len(tables["CFF2"])))
	f, err := readCFF2(tables["CFF2"], numGlyphs, budget)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("fonts: the font's outlines are CFF2, and its CFF2 table cannot be read: %w", err)
	}
	if psName == "" {
		psName = "CFF2"
	}
	conv, err := cff2ToCFF(f, newFontToolsBlend(f, coords, tags), advances, psName, budget)
	if err != nil {
		return nil, nil, nil, err
	}
	out := cff2Tables(tables, conv.cff)
	if name != nil {
		out["name"] = name
	}

	bounds := make([]glyphBounds, numGlyphs)
	for g, b := range conv.bounds {
		if !b.set {
			bounds[g] = glyphBounds{empty: true}
			continue
		}
		bounds[g] = glyphBounds{
			xMin: int(math.Floor(b.xMin)), yMin: int(math.Floor(b.yMin)),
			xMax: int(math.Ceil(b.xMax)), yMax: int(math.Ceil(b.yMax)),
		}
	}
	out["hmtx"], out["hhea"] = buildMetrics(hhea, advances, make([]float64, numGlyphs), bounds)
	if err := instanceCFF2Vertical(tables, out, coords, numGlyphs, bounds); err != nil {
		return nil, nil, nil, err
	}
	h := append([]byte(nil), head...)
	var box floatBounds
	for _, b := range bounds {
		if !b.empty {
			box.add(float64(b.xMin), float64(b.yMin))
			box.add(float64(b.xMax), float64(b.yMax))
		}
	}
	if box.set {
		putBounds(h[34:], int(box.xMin), int(box.yMin), int(box.xMax), int(box.yMax))
	}
	out["head"] = h
	instanceDesign(out, axes, want)
	if err := applyMVAR(out, tables["MVAR"], coords); err != nil {
		return nil, nil, nil, err
	}
	return assembleOTTO(out), coords, conv.limits, nil
}

// instanceCFF2Vertical moves the vertical metrics of a CFF2 font's instance:
// each glyph's vertical advance by VVAR, and where the font states its origins
// in VORG, each origin by VVAR's origin deltas, rounded as HarfBuzz rounds the
// origin it reports (half up); the top side bearings vmtx states are then the
// origins less the tops of the instance's glyphs. A font with no VORG keeps
// the bearings it states: a CFF face without VORG hangs a glyph by its ink
// (vertical.go), and the bearing is not read.
func instanceCFF2Vertical(tables, out map[string][]byte, coords []float64, numGlyphs int, bounds []glyphBounds) error {
	vhea, vmtx := tables["vhea"], tables["vmtx"]
	if len(vhea) < 36 || vmtx == nil {
		return nil
	}
	vAdvances, vBearings, err := parseHmtx(vmtx, font.Be16(vhea, 34), numGlyphs)
	if err != nil {
		return fmt.Errorf("fonts: vmtx: %w", err)
	}
	var vvar *hvarTable
	var origins *deltaSetIndexMap
	if t := tables["VVAR"]; t != nil {
		if vvar, err = parseHVAR(t); err != nil {
			return fmt.Errorf("fonts: VVAR: %w", err)
		}
		for g := range vAdvances {
			vAdvances[g] = max(vAdvances[g]+otRound(vvar.advanceDelta(g, coords)), 0)
		}
		if len(t) >= 24 {
			if off := int(font.Be32(t, 20)); off != 0 {
				if off < 0 || off >= len(t) {
					return errors.New("fonts: VVAR's origin mapping lies outside it")
				}
				if origins, err = parseDeltaSetIndexMap(t[off:]); err != nil {
					return fmt.Errorf("fonts: VVAR's origin mapping: %w", err)
				}
			}
		}
	}
	tops := make([]int, numGlyphs)
	vorg := tables["VORG"]
	var v verticalTables
	if len(vorg) >= 8 && font.Be16(vorg, 0) == 1 {
		if n := font.Be16(vorg, 6); 8+4*n <= len(vorg) {
			v.vorg, v.vorgCount, v.vorgDefault = vorg, n, signed16(font.Be16(vorg, 4))
		}
	}
	for g := range tops {
		switch {
		case v.vorg != nil:
			y := float64(v.vorgOrigin(g))
			if origins != nil {
				outer, inner := origins.lookup(g)
				y += vvar.store.delta(outer, inner, coords)
			}
			tops[g] = otRound(y)
		case bounds[g].empty:
			tops[g] = vBearings[g]
		default:
			tops[g] = bounds[g].yMax + vBearings[g]
		}
	}
	out["vmtx"], out["vhea"] = buildVerticalMetrics(vhea, vAdvances, tops, bounds)
	if v.vorg != nil {
		out["VORG"] = writeVORG(tops)
	}
	return nil
}

// writeVORG writes VORG for the origins tops: the most common as the default,
// and a record for every glyph whose origin is another.
func writeVORG(tops []int) []byte {
	count := map[int]int{}
	for _, y := range tops {
		count[y]++
	}
	def, best := 0, -1
	for y, n := range count {
		if n > best || (n == best && y < def) {
			def, best = y, n
		}
	}
	out := []byte{0, 1, 0, 0}
	out = binary.BigEndian.AppendUint16(out, uint16(int16(clampI16(def))))
	var recs []byte
	n := 0
	for g, y := range tops {
		if y != def {
			recs = binary.BigEndian.AppendUint16(recs, uint16(g))
			recs = binary.BigEndian.AppendUint16(recs, uint16(int16(clampI16(y))))
			n++
		}
	}
	out = binary.BigEndian.AppendUint16(out, uint16(n))
	return append(out, recs...)
}

// subsetCFF2 is Subset for a face whose outlines are CFF2, read at its default
// instance: the CID-keyed subset of the CFF font it draws, written from the
// kept glyphs alone. See cff2Default.subset.
func (f *Face) subsetCFF2() ([]byte, []int, error) {
	n := f.prog.NumGlyphs
	keep := make([]bool, n)
	keep[0] = true // .notdef
	for gid := range f.used {
		if gid >= 0 && gid < n {
			keep[gid] = true
		}
	}
	out, err := f.cff2.subset(keep, f.prog.Cmap)
	if err != nil {
		return nil, nil, err
	}
	kept := make([]int, 0, len(keep))
	for gid, k := range keep {
		if k {
			kept = append(kept, gid)
		}
	}
	return out, kept, nil
}
