package shape

import (
	"errors"
	"fmt"
	"math"

	"github.com/mgilbir/forme/font"
)

// 'gvar', the table that says how a glyph's points move across a variable
// font's design space.
//
// A glyph's entry is a list of tuples. Each names a region of the design space —
// a peak, and either the implied region around it or an explicit one — and gives
// a delta for some or all of the glyph's points. At a given location every tuple
// is weighted by how far into its region that location falls, and the weighted
// deltas are summed onto the stored outline. A tuple whose region does not
// contain the location weighs zero and is skipped, which is why the default
// instance draws exactly what glyf stores: every tuple's region is built around
// a peak away from zero.
//
// # Inferred points
//
// A tuple may list the points it moves rather than all of them, and that is the
// part that cannot be left out. The points it does not list are not "unmoved":
// they are inferred from the listed points on either side of them along the same
// contour, which is what the specification calls IUP. Skipping the inference
// leaves the unlisted points where the default instance had them while their
// neighbours move, which does not fail — it draws a glyph with dents in it.
//
// The inference is per contour and per axis, and the phantom points and a
// composite's components are each a contour of their own, so a point of theirs
// that no tuple lists stays put rather than being interpolated from a
// neighbouring one it has nothing to do with.

// gvarTable is the parsed table: the shared tuples every glyph may point at, and
// each glyph's variation data.
type gvarTable struct {
	axisCount int
	shared    [][]float64
	glyphs    [][]byte
}

// Tuple header flags (OpenType, "Tuple variation store").
const (
	tupleEmbeddedPeak  = 0x8000
	tupleIntermediate  = 0x4000
	tuplePrivatePoints = 0x2000
	tupleIndexMask     = 0x0FFF

	gvarSharedPointNumbers = 0x8000
	gvarCountMask          = 0x0FFF
)

// parseGvar reads the table far enough to find each glyph's data. The data
// itself is read per glyph, when the glyph is instanced.
func parseGvar(t []byte, numGlyphs, axisCount int) (*gvarTable, error) {
	if len(t) < 20 {
		return nil, errors.New("fonts: the gvar table is too short to hold its header")
	}
	if font.Be16(t, 0) != 1 {
		return nil, fmt.Errorf("fonts: gvar is version %d, which this does not read", font.Be16(t, 0))
	}
	if n := font.Be16(t, 4); n != axisCount {
		return nil, fmt.Errorf("fonts: gvar states %d axes and fvar states %d", n, axisCount)
	}
	sharedCount := font.Be16(t, 6)
	sharedOff := int(font.Be32(t, 8))
	glyphCount := font.Be16(t, 12)
	longOffsets := font.Be16(t, 14)&1 != 0
	dataOff := int(font.Be32(t, 16))

	if glyphCount > numGlyphs {
		return nil, fmt.Errorf("fonts: gvar carries data for %d glyphs and the font has %d", glyphCount, numGlyphs)
	}

	v := &gvarTable{axisCount: axisCount, glyphs: make([][]byte, numGlyphs)}
	if sharedCount > 0 {
		// The count needs no cap: every tuple costs two bytes an axis in the
		// file, and nothing is allocated until they are known to be there.
		need := sharedOff + 2*axisCount*sharedCount
		if sharedOff < 0 || need > len(t) {
			return nil, errors.New("fonts: gvar's shared tuples lie outside the table")
		}
		v.shared = make([][]float64, sharedCount)
		for i := range v.shared {
			tuple := make([]float64, axisCount)
			for a := range tuple {
				tuple[a] = f2Dot14At(t, sharedOff+2*axisCount*i+2*a)
			}
			v.shared[i] = tuple
		}
	}

	// The per-glyph offsets are into the data array, and the last one bounds
	// the one before it, so there are glyphCount+1 of them.
	size := 2
	if longOffsets {
		size = 4
	}
	if 20+size*(glyphCount+1) > len(t) {
		return nil, errors.New("fonts: gvar's offset array is shorter than its glyph count")
	}
	if dataOff < 0 || dataOff > len(t) {
		return nil, errors.New("fonts: gvar's data array lies outside the table")
	}
	data := t[dataOff:]
	offset := func(i int) int {
		if longOffsets {
			return int(font.Be32(t, 20+4*i))
		}
		return font.Be16(t, 20+2*i) * 2
	}
	for gid := 0; gid < glyphCount; gid++ {
		start, end := offset(gid), offset(gid+1)
		if start == end {
			continue // this glyph does not vary
		}
		if start > end || end > len(data) {
			return nil, fmt.Errorf("fonts: gvar's entry for glyph %d lies outside the table", gid)
		}
		v.glyphs[gid] = data[start:end]
	}
	return v, nil
}

// applyGlyph moves a glyph's points to the given normalized location.
//
// budget is the shared allowance this and every other glyph draw from: a tuple
// costs one unit per point of the glyph whatever it lists, because inferring the
// points it does not list walks all of them. A font can otherwise state a
// glyph of sixty thousand points and four thousand tuples in a few kilobytes,
// and ask for hundreds of millions of operations per glyph.
func (v *gvarTable) applyGlyph(gid int, g *varGlyph, coords []float64, budget *int64) error {
	if gid < 0 || gid >= len(v.glyphs) {
		return nil
	}
	gd := v.glyphs[gid]
	if len(gd) == 0 {
		return nil
	}
	if len(gd) < 4 {
		return fmt.Errorf("fonts: glyph %d's variation data is %d bytes, too few for its header", gid, len(gd))
	}
	numPoints := g.numPoints()
	if numPoints == 0 {
		return nil
	}
	count := font.Be16(gd, 0)
	tupleCount := count & gvarCountMask
	serialAt := font.Be16(gd, 2)
	if serialAt > len(gd) {
		return fmt.Errorf("fonts: glyph %d's variation data starts past the end of its entry", gid)
	}
	serial := gd[serialAt:]
	headers := gd[4:]

	// The shared point numbers, when the glyph has them, sit at the front of
	// the serialized data and stand in for every tuple that does not carry its
	// own.
	var sharedPoints []int
	sharedAll := false
	at := 0
	if count&gvarSharedPointNumbers != 0 {
		var err error
		sharedPoints, sharedAll, at, err = readPackedPoints(serial, numPoints)
		if err != nil {
			return fmt.Errorf("fonts: glyph %d's shared point numbers: %w", gid, err)
		}
	}

	// Every tuple's contribution is summed here and added to the outline once,
	// at the end, and the outline itself is left alone until then. Two reasons,
	// and the second is the one that bites:
	//
	//   - Inference is stated against the *default* instance's points. A tuple
	//     applied before this one may have moved a point onto its neighbour, and
	//     interpolating between two points that now coincide answers a question
	//     about a font nobody asked for.
	//   - Deltas are small and coordinates are not. Adding each tuple's delta
	//     straight onto the coordinate rounds a large number a dozen times;
	//     summing the deltas first rounds a small one instead, and lands a whole
	//     unit away often enough to see.
	tx := make([]float64, numPoints)
	ty := make([]float64, numPoints)

	dx := make([]float64, numPoints)
	dy := make([]float64, numPoints)
	known := make([]bool, numPoints)
	head := 0
	for i := 0; i < tupleCount; i++ {
		if head+4 > len(headers) {
			return fmt.Errorf("fonts: glyph %d states %d tuples and carries %d headers", gid, tupleCount, i)
		}
		size := font.Be16(headers, head)
		index := font.Be16(headers, head+2)
		head += 4

		var peak, start, end []float64
		if index&tupleEmbeddedPeak != 0 {
			if head+2*v.axisCount > len(headers) {
				return fmt.Errorf("fonts: glyph %d's tuple %d has no peak", gid, i)
			}
			peak = make([]float64, v.axisCount)
			for a := range peak {
				peak[a] = f2Dot14At(headers, head+2*a)
			}
			head += 2 * v.axisCount
		} else {
			n := index & tupleIndexMask
			if n >= len(v.shared) {
				return fmt.Errorf("fonts: glyph %d's tuple %d names shared tuple %d of %d", gid, i, n, len(v.shared))
			}
			peak = v.shared[n]
		}
		intermediate := index&tupleIntermediate != 0
		if intermediate {
			if head+4*v.axisCount > len(headers) {
				return fmt.Errorf("fonts: glyph %d's tuple %d has no region", gid, i)
			}
			start = make([]float64, v.axisCount)
			end = make([]float64, v.axisCount)
			for a := range start {
				start[a] = f2Dot14At(headers, head+2*a)
				end[a] = f2Dot14At(headers, head+2*v.axisCount+2*a)
			}
			head += 4 * v.axisCount
		}

		if at+size > len(serial) {
			return fmt.Errorf("fonts: glyph %d's tuple %d claims %d bytes past the end of its data", gid, i, size)
		}
		body := serial[at : at+size]
		at += size

		scalar := tupleScalar(peak, start, end, coords, intermediate)
		if scalar == 0 {
			continue
		}
		if *budget -= int64(numPoints); *budget < 0 {
			return fmt.Errorf("fonts: instancing this font needs more than %d point operations", int64(maxInstanceWork))
		}

		points, all := sharedPoints, sharedAll
		bodyAt := 0
		if index&tuplePrivatePoints != 0 {
			var err error
			points, all, bodyAt, err = readPackedPoints(body, numPoints)
			if err != nil {
				return fmt.Errorf("fonts: glyph %d's tuple %d point numbers: %w", gid, i, err)
			}
		} else if points == nil && !all {
			// No shared list, and none of its own: the specification gives the
			// tuple nothing to apply its deltas to.
			return fmt.Errorf("fonts: glyph %d's tuple %d names no points and the glyph has no shared list", gid, i)
		}

		n := len(points)
		if all {
			n = numPoints
		}
		for j := range known {
			known[j] = false
		}
		xs, bodyAt, err := readPackedDeltas(body, bodyAt, n)
		if err != nil {
			return fmt.Errorf("fonts: glyph %d's tuple %d x deltas: %w", gid, i, err)
		}
		ys, _, err := readPackedDeltas(body, bodyAt, n)
		if err != nil {
			return fmt.Errorf("fonts: glyph %d's tuple %d y deltas: %w", gid, i, err)
		}
		// The tuple's weight is applied before the unlisted points are inferred
		// from the listed ones, not after. The two orders are the same
		// arithmetic and not the same floating-point number, and this is the
		// one both reference implementations take.
		if all {
			for j := range dx {
				dx[j], dy[j] = xs[j]*scalar, ys[j]*scalar
				known[j] = true
			}
		} else {
			for j := range dx {
				dx[j], dy[j] = 0, 0
			}
			for j, p := range points {
				dx[p], dy[p] = xs[j]*scalar, ys[j]*scalar
				known[p] = true
			}
			inferPoints(g.ends, g.x, g.y, dx, dy, known)
		}
		for j := range dx {
			tx[j] += dx[j]
			ty[j] += dy[j]
		}
	}
	for j := range tx {
		g.x[j] += tx[j]
		g.y[j] += ty[j]
	}
	return nil
}

// tupleScalar is how much of a tuple's deltas apply at a location: one when the
// location is at the tuple's peak, zero outside its region, and a linear ramp
// between, multiplied over the axes.
//
// An axis whose peak is zero does not take part, which is not the same as an
// axis whose *coordinate* is zero: the second is the default instance, where a
// tuple built around any other peak contributes nothing at all. That is the one
// place where zero has two meanings in this format, and reading it as "no peak
// stated" would apply every tuple in the font at the default instance.
func tupleScalar(peak, start, end, coords []float64, intermediate bool) float64 {
	scalar := 1.0
	for i, p := range peak {
		if p == 0 {
			continue
		}
		var v float64
		if i < len(coords) {
			v = coords[i]
		}
		if v == p {
			continue
		}
		if !intermediate {
			if v == 0 || v < math.Min(0, p) || v > math.Max(0, p) {
				return 0
			}
			scalar *= v / p
			continue
		}
		s, e := start[i], end[i]
		if s > p || p > e || (s < 0 && e > 0) {
			// A region that does not contain its own peak, or one that spans the
			// default instance: the specification calls both invalid, and every
			// implementation ignores the axis rather than the tuple.
			continue
		}
		if v < s || v > e {
			return 0
		}
		if v < p {
			if p != s {
				scalar *= (v - s) / (p - s)
			}
		} else if p != e {
			scalar *= (e - v) / (e - p)
		}
	}
	return scalar
}

// inferPoints fills in the deltas of the points no tuple listed, per contour.
//
// Each contour is treated on its own and each axis on its own: a point between
// two listed points moves as far as its position between them says, and one
// outside them moves with the nearer. A contour with no listed point at all does
// not move, and one with a single listed point moves whole.
func inferPoints(ends []int, ox, oy, dx, dy []float64, known []bool) {
	// The phantom points and a composite's components are each their own
	// contour, and a single point that nothing listed keeps its zero delta —
	// so only the real contours have anything to infer.
	start := 0
	for _, end := range ends {
		if end < start || end >= len(known) {
			return
		}
		inferContour(ox, oy, dx, dy, known, start, end)
		start = end + 1
	}
}

func inferContour(ox, oy, dx, dy []float64, known []bool, start, end int) {
	n := end - start + 1
	if n <= 0 {
		return
	}
	first, count := -1, 0
	for i := start; i <= end; i++ {
		if known[i] {
			if first < 0 {
				first = i
			}
			count++
		}
	}
	switch {
	case count == 0:
		return
	case count == n:
		return
	case count == 1:
		for i := start; i <= end; i++ {
			dx[i], dy[i] = dx[first], dy[first]
		}
		return
	}
	next := func(i int) int {
		if i == end {
			return start
		}
		return i + 1
	}
	// Walk the listed points in order, filling the run of unlisted points after
	// each from that point and the next listed one, wrapping round the contour.
	a := first
	for {
		b := next(a)
		for !known[b] {
			b = next(b)
		}
		for i := next(a); i != b; i = next(i) {
			dx[i] = infer(ox[i], ox[a], dx[a], ox[b], dx[b])
			dy[i] = infer(oy[i], oy[a], dy[a], oy[b], dy[b])
		}
		a = b
		if a == first {
			return
		}
	}
}

// infer is one coordinate of one inferred point: where it sat between its two
// listed neighbours in the default instance is where it sits between them now.
func infer(o, o1, d1, o2, d2 float64) float64 {
	if o1 == o2 {
		if d1 == d2 {
			return d1
		}
		// Two listed points at the same coordinate that move apart say nothing
		// about where a point between them goes, so it does not move.
		return 0
	}
	if o1 > o2 {
		o1, o2 = o2, o1
		d1, d2 = d2, d1
	}
	if o <= o1 {
		return d1
	}
	if o >= o2 {
		return d2
	}
	scale := (d2 - d1) / (o2 - o1)
	// The conversion is not redundant: it forbids the compiler from fusing the
	// multiply into the add, which some architectures otherwise do. A fused
	// result is a *better* one, and it would differ from what every other
	// implementation computes here and from what this package computes on the
	// architectures that do not fuse — so the numbers would depend on the
	// machine, which is the one property an oracle-checked table cannot have.
	return d1 + float64((o-o1)*scale)
}

// readPackedPoints reads a point-number list. An empty list is the format's way
// of saying "every point", which is not the same as a list of none.
//
// maxPoints is the glyph's own point count: a list naming a point past it is a
// list this cannot apply, and is refused rather than skipped, because the deltas
// that follow it are then being read against the wrong points.
func readPackedPoints(b []byte, maxPoints int) (points []int, all bool, n int, err error) {
	if len(b) < 1 {
		return nil, false, 0, errors.New("the list is empty")
	}
	at := 1
	count := int(b[0])
	if count&0x80 != 0 {
		if len(b) < 2 {
			return nil, false, 0, errors.New("the two-byte count is cut short")
		}
		count = (count&0x7F)<<8 | int(b[1])
		at = 2
	}
	if count == 0 {
		return nil, true, at, nil
	}
	if count > maxPoints {
		return nil, false, 0, fmt.Errorf("it names %d points and the glyph has %d", count, maxPoints)
	}
	points = make([]int, 0, count)
	value := 0
	for len(points) < count {
		if at >= len(b) {
			return nil, false, 0, fmt.Errorf("it ends after %d of %d points", len(points), count)
		}
		control := b[at]
		at++
		run := int(control&0x7F) + 1
		if run > count-len(points) {
			return nil, false, 0, fmt.Errorf("a run of %d points overruns the %d it declared", run, count)
		}
		wide := control&0x80 != 0
		size := 1
		if wide {
			size = 2
		}
		if at+size*run > len(b) {
			return nil, false, 0, errors.New("a run of point numbers runs past the end")
		}
		for i := 0; i < run; i++ {
			if wide {
				value += font.Be16(b, at)
				at += 2
			} else {
				value += int(b[at])
				at++
			}
			// Point numbers are stored as differences and so never decrease;
			// the bound is what keeps the deltas indexing this glyph.
			if value >= maxPoints {
				return nil, false, 0, fmt.Errorf("it names point %d and the glyph has %d", value, maxPoints)
			}
			points = append(points, value)
		}
	}
	return points, false, at, nil
}

// readPackedDeltas reads count deltas from the run-length form the format
// stores them in: a run of zeroes carries no data at all, which is how a tuple
// that moves a few points states the rest.
func readPackedDeltas(b []byte, at, count int) ([]float64, int, error) {
	out := make([]float64, 0, count)
	for len(out) < count {
		if at >= len(b) {
			return nil, 0, fmt.Errorf("they end after %d of %d", len(out), count)
		}
		control := b[at]
		at++
		run := int(control&0x3F) + 1
		if run > count-len(out) {
			return nil, 0, fmt.Errorf("a run of %d overruns the %d declared", run, count)
		}
		switch {
		case control&0x80 != 0: // DELTAS_ARE_ZERO
			for i := 0; i < run; i++ {
				out = append(out, 0)
			}
		case control&0x40 != 0: // DELTAS_ARE_WORDS
			if at+2*run > len(b) {
				return nil, 0, errors.New("a run of sixteen-bit deltas runs past the end")
			}
			for i := 0; i < run; i++ {
				out = append(out, float64(int16(uint16(font.Be16(b, at)))))
				at += 2
			}
		default:
			if at+run > len(b) {
				return nil, 0, errors.New("a run of eight-bit deltas runs past the end")
			}
			for i := 0; i < run; i++ {
				out = append(out, float64(int8(b[at])))
				at++
			}
		}
	}
	return out, at, nil
}
