package shape

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/mgilbir/forme/font"
)

// A glyf glyph taken apart far enough to move its points, and put back together
// again.
//
// Instancing a variable font is a rewrite, not a setting: the outlines a reader
// draws are the ones in glyf, and 'gvar' says how far each *point* moves at a
// point in the design space. So the only way to hand a caller another instance
// is to decode every glyph, move its points, and encode it again — which is what
// this file does and what instance.go drives.
//
// # The four phantom points
//
// Every glyph has four points beyond the ones its outline names: the two
// horizontal side-bearing points and the two vertical ones. They are not drawn.
// They exist because gvar varies them too, and that is how a variable font says
// its advances change — see the advance handling in instance.go. They are
// carried here so that a delta list indexes the same points the font counted
// when it was built; a point list four short reads every delta after the last
// outline point against the wrong point.
//
// # What is dropped
//
// Hinting instructions. An instance's control values live in 'cvt' and are
// varied by 'cvar', which this does not read, so keeping the instructions would
// hint the new outlines by the *default* instance's control values. The
// alternative to dropping them is applying cvar, and the alternative to both is
// hinting a bold face by a thin one's numbers. Dropping is the honest one, and
// costs nothing here: the subsetter drops cvt, fpgm and prep anyway.

// varGlyph is one glyph's geometry: its points, and enough of its structure to
// write it back out.
//
// x and y hold the outline points followed by the four phantom points, so
// len(x) is the point count gvar's deltas are indexed by. A composite's
// "points" are its components' offsets, one each, which is what the format
// varies for a composite.
type varGlyph struct {
	composite bool
	// ends is the index of the last point of each contour; empty for a
	// composite and for a glyph with no outline.
	ends []int
	// flags carries the per-point bits that are not about coordinate encoding:
	// ON_CURVE_POINT and OVERLAP_SIMPLE. The rest are recomputed on encoding,
	// because they say how the numbers were stored and the numbers change.
	flags []byte
	comps []varComponent
	x, y  []float64
}

// varComponent is one component of a composite glyph. The transform is kept as
// the bytes it arrived in: instancing moves components but never reshapes them,
// so re-deriving those bytes from a decoded matrix would be a rounding step
// taken for nothing.
type varComponent struct {
	flags     int
	glyph     int
	transform []byte
	// scale is the 2×2 the transform bytes decode to, which the bounding-box
	// pass needs; it is the identity when there are no transform bytes.
	scale [4]float64
}

// Component flags (OpenType, "Composite glyph description").
const (
	compArgsAreWords     = 0x0001
	compArgsAreXY        = 0x0002
	compHaveScale        = 0x0008
	compMoreComponents   = 0x0020
	compHaveXYScale      = 0x0040
	compHave2x2          = 0x0080
	compHaveInstructions = 0x0100
)

// numPoints is the count gvar indexes, phantom points included.
func (g *varGlyph) numPoints() int { return len(g.x) }

// numOutlinePoints is the count the outline itself names.
func (g *varGlyph) numOutlinePoints() int { return len(g.x) - 4 }

// decodeVarGlyph reads one glyph's glyf entry. An empty entry is a blank glyph,
// which is what a space is, and is not an error.
//
// numGlyphs bounds the component references: a composite naming a glyph the
// font does not have is a font that cannot be instanced, not a component to
// skip, because dropping it would silently redraw the glyph.
func decodeVarGlyph(b []byte, numGlyphs int) (*varGlyph, error) {
	g := &varGlyph{}
	if len(b) == 0 {
		return g, nil
	}
	if len(b) < 10 {
		return nil, fmt.Errorf("fonts: a glyph entry of %d bytes is too short for a glyph header", len(b))
	}
	nc := int(int16(uint16(font.Be16(b, 0))))
	switch {
	case nc == 0:
		return g, nil
	case nc < 0:
		if nc != -1 {
			return nil, fmt.Errorf("fonts: a glyph declares %d contours; only -1 names a composite", nc)
		}
		g.composite = true
		return g, decodeComposite(g, b, numGlyphs)
	}
	return g, decodeSimple(g, b, nc)
}

func decodeSimple(g *varGlyph, b []byte, nc int) error {
	at := 10
	if at+2*nc+2 > len(b) {
		return fmt.Errorf("fonts: a glyph declares %d contours but carries %d bytes", nc, len(b))
	}
	g.ends = make([]int, nc)
	last := -1
	for i := 0; i < nc; i++ {
		e := font.Be16(b, at+2*i)
		if e < last {
			return fmt.Errorf("fonts: contour %d ends at point %d, before the contour before it", i, e)
		}
		last = e
		g.ends[i] = e
	}
	// A point count needs no cap of its own: a contour end is a uint16, so the
	// count cannot exceed 65536, and the flags below are read one per point out
	// of the entry, which is what stops a glyph claiming points it has no bytes
	// for.
	n := last + 1
	at += 2 * nc
	instr := font.Be16(b, at)
	at += 2 + instr
	if at > len(b) {
		return fmt.Errorf("fonts: a glyph's %d instruction bytes run past its entry", instr)
	}

	// Flags, with the repeat encoding expanded. Every point has one.
	flags := make([]byte, n)
	for i := 0; i < n; {
		if at >= len(b) {
			return fmt.Errorf("fonts: a glyph's flags end after %d of %d points", i, n)
		}
		f := b[at]
		at++
		flags[i] = f
		i++
		if f&0x08 == 0 { // REPEAT_FLAG
			continue
		}
		if at >= len(b) {
			return fmt.Errorf("fonts: a glyph's repeat count is past the end of its entry")
		}
		repeat := int(b[at])
		at++
		for j := 0; j < repeat && i < n; j++ {
			flags[i] = f
			i++
		}
	}

	g.x = make([]float64, n+4)
	g.y = make([]float64, n+4)
	g.flags = make([]byte, n)
	for i, f := range flags {
		g.flags[i] = f & 0x41 // ON_CURVE_POINT and OVERLAP_SIMPLE
	}
	// Coordinates are stored as deltas from the point before, x for every point
	// and then y for every point.
	read := func(short, same byte, out []float64) error {
		v := 0
		for i, f := range flags {
			switch {
			case f&short != 0:
				if at >= len(b) {
					return fmt.Errorf("fonts: a glyph's coordinates end after %d of %d points", i, len(flags))
				}
				d := int(b[at])
				at++
				if f&same == 0 {
					d = -d
				}
				v += d
			case f&same == 0:
				if at+2 > len(b) {
					return fmt.Errorf("fonts: a glyph's coordinates end after %d of %d points", i, len(flags))
				}
				v += int(int16(uint16(font.Be16(b, at))))
				at += 2
			}
			out[i] = float64(v)
		}
		return nil
	}
	if err := read(0x02, 0x10, g.x); err != nil {
		return err
	}
	return read(0x04, 0x20, g.y)
}

// maxComponents bounds a composite's component count. The format states them as
// a chain rather than a count, so the only thing that ends the chain in a
// malformed font is running out of bytes; this stops a glyph from claiming more
// components than any real one has before the bytes are read.
const maxComponents = 4096

func decodeComposite(g *varGlyph, b []byte, numGlyphs int) error {
	at := 10
	for {
		if len(g.comps) >= maxComponents {
			return fmt.Errorf("fonts: a composite glyph names more than %d components", maxComponents)
		}
		if at+4 > len(b) {
			return fmt.Errorf("fonts: a composite glyph's component list runs past its entry")
		}
		flags := font.Be16(b, at)
		gid := font.Be16(b, at+2)
		at += 4
		if gid >= numGlyphs {
			return fmt.Errorf("fonts: a composite glyph names component %d, which the font does not have", gid)
		}
		if flags&compArgsAreXY == 0 {
			// The arguments are point numbers to match, not an offset: the
			// component is placed by making one of its points coincide with one
			// of the composite's. Nothing here can move that — the point it
			// would move is in another glyph — and a gvar delta for such a
			// component has no meaning this can honour. Refusing is the honest
			// answer; instancing it as though the arguments were an offset would
			// move the component to an arbitrary place on the page.
			return fmt.Errorf("fonts: a composite glyph places component %d by matching points, which cannot be instanced", gid)
		}
		var a1, a2 float64
		if flags&compArgsAreWords != 0 {
			if at+4 > len(b) {
				return fmt.Errorf("fonts: a composite glyph's component offsets run past its entry")
			}
			a1 = float64(int16(uint16(font.Be16(b, at))))
			a2 = float64(int16(uint16(font.Be16(b, at+2))))
			at += 4
		} else {
			if at+2 > len(b) {
				return fmt.Errorf("fonts: a composite glyph's component offsets run past its entry")
			}
			a1 = float64(int8(b[at]))
			a2 = float64(int8(b[at+1]))
			at += 2
		}
		c := varComponent{flags: flags, glyph: gid, scale: [4]float64{1, 0, 0, 1}}
		size := 0
		switch {
		case flags&compHaveScale != 0:
			size = 2
		case flags&compHaveXYScale != 0:
			size = 4
		case flags&compHave2x2 != 0:
			size = 8
		}
		if at+size > len(b) {
			return fmt.Errorf("fonts: a composite glyph's component transform runs past its entry")
		}
		c.transform = b[at : at+size]
		switch size {
		case 2:
			s := f2Dot14At(c.transform, 0)
			c.scale = [4]float64{s, 0, 0, s}
		case 4:
			c.scale = [4]float64{f2Dot14At(c.transform, 0), 0, 0, f2Dot14At(c.transform, 2)}
		case 8:
			c.scale = [4]float64{
				f2Dot14At(c.transform, 0), f2Dot14At(c.transform, 2),
				f2Dot14At(c.transform, 4), f2Dot14At(c.transform, 6),
			}
		}
		at += size
		g.comps = append(g.comps, c)
		g.x = append(g.x, a1)
		g.y = append(g.y, a2)
		if flags&compMoreComponents == 0 {
			break
		}
	}
	// The four phantom points follow the components, which are this glyph's
	// only variable points.
	g.x = append(g.x, 0, 0, 0, 0)
	g.y = append(g.y, 0, 0, 0, 0)
	return nil
}

// f2Dot14At reads a signed fixed-point number with fourteen fractional bits.
func f2Dot14At(b []byte, off int) float64 {
	if off+2 > len(b) {
		return 0
	}
	return float64(int16(binary.BigEndian.Uint16(b[off:]))) / 16384
}

// setPhantoms puts the four phantom points where the glyph's metrics say they
// are, which is where gvar expects to find them: the first at the glyph origin
// as the left side bearing places it, the second an advance further along, and
// the vertical pair, which this does not use but must still count.
func (g *varGlyph) setPhantoms(xMin, lsb, advance int) {
	if len(g.x) < 4 {
		g.x = make([]float64, 4)
		g.y = make([]float64, 4)
	}
	n := len(g.x)
	left := float64(xMin - lsb)
	g.x[n-4], g.y[n-4] = left, 0
	g.x[n-3], g.y[n-3] = left+float64(advance), 0
	g.x[n-2], g.y[n-2] = 0, 0
	g.x[n-1], g.y[n-1] = 0, 0
}

// advance is what the phantom points say the glyph's advance is, which after
// deltas have been applied is its advance in this instance.
func (g *varGlyph) advance() (left, adv float64) {
	n := len(g.x)
	if n < 4 {
		return 0, 0
	}
	return g.x[n-4], g.x[n-3] - g.x[n-4]
}

// otRound rounds a coordinate the way the format's own tools do: halves go up,
// on the positive and the negative side alike. Every implementation that writes
// glyf agrees on this, and a coordinate rounded the other way is a point in a
// different place from the one the font's publisher shipped.
func otRound(v float64) int {
	if math.IsNaN(v) {
		return 0
	}
	return int(math.Floor(v + 0.5))
}

// encodeVarGlyph writes the glyph back into a glyf entry. The bounding box is
// left at zero for a composite: it cannot be known until the components have
// been instanced, and instance.go fills it in afterwards.
func encodeVarGlyph(g *varGlyph) ([]byte, error) {
	if g.composite {
		return encodeComposite(g)
	}
	if len(g.ends) == 0 {
		return nil, nil // a blank glyph is a zero-length entry
	}
	n := g.numOutlinePoints()
	xs := make([]int, n)
	ys := make([]int, n)
	for i := 0; i < n; i++ {
		xs[i] = otRound(g.x[i])
		ys[i] = otRound(g.y[i])
		if xs[i] < math.MinInt16 || xs[i] > math.MaxInt16 || ys[i] < math.MinInt16 || ys[i] > math.MaxInt16 {
			// glyf stores coordinates as sixteen-bit integers, so a point this
			// far out cannot be written at all. It takes deltas far larger than
			// any real font's to get here.
			return nil, fmt.Errorf("fonts: an instanced point at (%d, %d) is outside the range glyf can store", xs[i], ys[i])
		}
	}

	out := make([]byte, 10+2*len(g.ends)+2)
	binary.BigEndian.PutUint16(out[0:], uint16(int16(len(g.ends))))
	xMin, yMin, xMax, yMax := intBounds(xs, ys)
	putBounds(out, xMin, yMin, xMax, yMax)
	for i, e := range g.ends {
		binary.BigEndian.PutUint16(out[10+2*i:], uint16(e))
	}
	// instructionLength: zero, see the note on hinting at the top of the file.

	// The coordinates are stored as two runs — every point's x, then every
	// point's y — and not as pairs.
	flags := make([]byte, n)
	var xCoords, yCoords []byte
	prevX, prevY := 0, 0
	for i := 0; i < n; i++ {
		f := g.flags[i] & 0x41
		dx, dy := xs[i]-prevX, ys[i]-prevY
		prevX, prevY = xs[i], ys[i]
		f, xCoords = appendCoord(f, xCoords, dx, 0x02, 0x10)
		f, yCoords = appendCoord(f, yCoords, dy, 0x04, 0x20)
		flags[i] = f
	}
	out = append(out, packFlags(flags)...)
	out = append(out, xCoords...)
	out = append(out, yCoords...)
	return out, nil
}

// appendCoord writes one coordinate delta in the shortest form that holds it
// and returns the flag bits that say which form was used.
func appendCoord(f byte, coords []byte, d int, short, same byte) (byte, []byte) {
	switch {
	case d == 0:
		return f | same, coords
	case d >= -255 && d <= 255:
		f |= short
		if d > 0 {
			f |= same // the "same" bit is the sign bit for the short form
		} else {
			d = -d
		}
		return f, append(coords, byte(d))
	default:
		return f, append(coords, byte(d>>8), byte(d))
	}
}

// packFlags applies the format's repeat encoding: a flag with REPEAT_FLAG set is
// followed by a count of the *further* points that carry it, so a run of 256
// identical flags is the longest one byte of count can state.
//
// It works from the finished list rather than byte by byte because the two
// cannot be told apart afterwards: a count byte and a flag byte look the same,
// and extending a run by comparing against the last byte written eventually
// compares a flag against a count that happens to equal it.
func packFlags(vals []byte) []byte {
	out := make([]byte, 0, len(vals))
	for i := 0; i < len(vals); {
		f := vals[i]
		j := i + 1
		for j < len(vals) && vals[j] == f && j-i < 256 {
			j++
		}
		if n := j - i - 1; n > 0 {
			out = append(out, f|0x08, byte(n))
		} else {
			out = append(out, f)
		}
		i = j
	}
	return out
}

func encodeComposite(g *varGlyph) ([]byte, error) {
	out := make([]byte, 10)
	binary.BigEndian.PutUint16(out[0:], 0xFFFF) // numberOfContours: -1, a composite
	for i, c := range g.comps {
		a1, a2 := otRound(g.x[i]), otRound(g.y[i])
		if a1 < math.MinInt16 || a1 > math.MaxInt16 || a2 < math.MinInt16 || a2 > math.MaxInt16 {
			return nil, fmt.Errorf("fonts: an instanced component offset of (%d, %d) is outside the range glyf can store", a1, a2)
		}
		// The flags are the original's, minus the two this rewrite decides:
		// whether the offsets need sixteen bits, and whether instructions
		// follow, which they no longer do.
		flags := c.flags &^ (compArgsAreWords | compHaveInstructions | compMoreComponents)
		words := a1 < -128 || a1 > 127 || a2 < -128 || a2 > 127
		if words {
			flags |= compArgsAreWords
		}
		if i < len(g.comps)-1 {
			flags |= compMoreComponents
		}
		var rec [4]byte
		binary.BigEndian.PutUint16(rec[0:], uint16(flags))
		binary.BigEndian.PutUint16(rec[2:], uint16(c.glyph))
		out = append(out, rec[:]...)
		if words {
			var args [4]byte
			binary.BigEndian.PutUint16(args[0:], uint16(int16(a1)))
			binary.BigEndian.PutUint16(args[2:], uint16(int16(a2)))
			out = append(out, args[:]...)
		} else {
			out = append(out, byte(int8(a1)), byte(int8(a2)))
		}
		out = append(out, c.transform...)
	}
	return out, nil
}

func putBounds(out []byte, xMin, yMin, xMax, yMax int) {
	binary.BigEndian.PutUint16(out[2:], uint16(int16(xMin)))
	binary.BigEndian.PutUint16(out[4:], uint16(int16(yMin)))
	binary.BigEndian.PutUint16(out[6:], uint16(int16(xMax)))
	binary.BigEndian.PutUint16(out[8:], uint16(int16(yMax)))
}

func intBounds(xs, ys []int) (xMin, yMin, xMax, yMax int) {
	if len(xs) == 0 {
		return 0, 0, 0, 0
	}
	xMin, xMax, yMin, yMax = xs[0], xs[0], ys[0], ys[0]
	for i := range xs {
		if xs[i] < xMin {
			xMin = xs[i]
		}
		if xs[i] > xMax {
			xMax = xs[i]
		}
		if ys[i] < yMin {
			yMin = ys[i]
		}
		if ys[i] > yMax {
			yMax = ys[i]
		}
	}
	return xMin, yMin, xMax, yMax
}
