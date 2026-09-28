package layout

import (
	"math"

	"github.com/mgilbir/forme/style"
)

// Filter Effects 1's colour functions and drop-shadow(), applied.
//
// # The colour functions are a matrix
//
// brightness(), contrast(), grayscale(), hue-rotate(), invert(), saturate() and
// sepia() are each §13.1's filter primitive, an feColorMatrix or an
// feComponentTransfer of type linear or table with two values, and each of
// those is an affine map of a pixel's non-premultiplied colour — a 4 by 5
// matrix — whose result is clamped to [0, 1] (§9.1: "The RGBA result from each
// filter primitive will be clamped into the allowable ranges"). None of the
// seven reads or changes alpha. They are in sRGB, gamma-encoded: "Filter
// Functions must operate in the sRGB color space" (§5).
//
// # Where a matrix is folded into the colours, and why that is exact
//
// A filter is applied to the element's rendered image, pixel by pixel. What a
// group of marks composites to at a pixel is, in non-premultiplied colour, a
// convex combination of the colours of the marks there — source-over weighs
// each by its coverage and alpha, and the weights sum to one — and a blur
// inside the group is another convex combination, of nearby pixels. An affine
// map takes a convex combination of colours to the same combination of their
// images. So where the map sends every colour the group holds inside [0, 1],
// it sends every combination of them inside too, the clamp does nothing, and
// applying the map to each mark's colour is the filter, exactly: to a fill, a
// path, a gradient's stops (which are interpolated by the same convex
// combination), a run of text, a shadow.
//
// Where some colour is clamped, the fold is still exact wherever no two
// colours mix: at every point of the page one mark's colour, or that colour
// over transparency, which is the same colour at less alpha. That holds when
// no mark that lets what is under it show through — a colour with alpha, a
// gradient with a stop that has, a blurred shadow — lies over another, when no
// gradient's own colours are clamped, and when nothing inside is a group that
// blurs or composites. Asked of the marks' rectangles, which is the cautious
// answer: rectangles that meet are taken to overlap. Where it does not hold,
// two colours mixed and then clamped are not the two clamped and mixed, and the
// function stays in the group, FilterColorMatrix, for the backend. So does one
// over a picture, whose pixels the engine does not rewrite, and one the
// document's allowance for these passes does not cover (see filterPass). A
// PDF backend draws such a group by rasterising it, as it does a blur.
//
// # Why the colours, and not a field for the backend
//
// The display list could have carried every colour function as a field of
// FilterGroup and left all of them to the backend. For a PDF that is the worse
// statement of the same page: PDF has nothing that maps a group's colours
// through a matrix — the transfer functions of its graphics state act on one
// component at a time, on device colours, and PDF 2.0 deprecates them — so
// every filtered element would be rasterised, text and all. Folded,
// a filter is fills, paths, gradients and text in other colours, which a PDF
// draws exactly and a reader can still select. The field is kept for what
// cannot be folded exactly, and only for that.
//
// # A drop shadow is a shadow of the group
//
// §13.1.10: the input's alpha, blurred by the deviation, offset, flooded with
// the colour and composited under the input. That is the group's marks, each
// drawn in the shadow's colour at its own alpha and moved by the offset,
// composited among themselves — which makes the union of their alphas, as the
// input's alpha is — and then blurred and faded by the colour's alpha as one: a
// FilterGroup of the moved marks, drawn before the marks. A run of text's
// shadow is a DrawTextShadow, which is what a text-shadow is and is not text.
// Over a picture, whose alpha the engine does not have as marks, the function
// stays in the group, FilterDropShadow, for the backend.

// maxFilterMatrixError is how far past [0, 1] a mapped colour may be and still
// count as inside: the arithmetic of a matrix whose rows sum to one lands a
// hair either side of an end, and a clamp of a hair is no clamp.
const maxFilterMatrixError = 1e-9

// identityMatrix is the colour matrix that changes nothing.
var identityMatrix = [20]float64{1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 0, 0, 1, 0}

// colourMatrix is a 3 by 3 map of r, g and b and an offset for each, as a
// filter's 4 by 5 matrix that leaves alpha alone.
func colourMatrix(m [3][3]float64, offset float64) [20]float64 {
	return [20]float64{
		m[0][0], m[0][1], m[0][2], 0, offset,
		m[1][0], m[1][1], m[1][2], 0, offset,
		m[2][0], m[2][1], m[2][2], 0, offset,
		0, 0, 0, 1, 0,
	}
}

// grayscaleMatrix and the rest are §13.1's primitives for one amount.
func grayscaleMatrix(amount float64) [20]float64 {
	s := 1 - amount
	return colourMatrix([3][3]float64{
		{0.2126 + 0.7874*s, 0.7152 - 0.7152*s, 0.0722 - 0.0722*s},
		{0.2126 - 0.2126*s, 0.7152 + 0.2848*s, 0.0722 - 0.0722*s},
		{0.2126 - 0.2126*s, 0.7152 - 0.7152*s, 0.0722 + 0.9278*s},
	}, 0)
}

func sepiaMatrix(amount float64) [20]float64 {
	s := 1 - amount
	return colourMatrix([3][3]float64{
		{0.393 + 0.607*s, 0.769 - 0.769*s, 0.189 - 0.189*s},
		{0.349 - 0.349*s, 0.686 + 0.314*s, 0.168 - 0.168*s},
		{0.272 - 0.272*s, 0.534 - 0.534*s, 0.131 + 0.869*s},
	}, 0)
}

// saturateMatrix is feColorMatrix type="saturate" (§9.6).
func saturateMatrix(s float64) [20]float64 {
	return colourMatrix([3][3]float64{
		{0.213 + 0.787*s, 0.715 - 0.715*s, 0.072 - 0.072*s},
		{0.213 - 0.213*s, 0.715 + 0.285*s, 0.072 - 0.072*s},
		{0.213 - 0.213*s, 0.715 - 0.715*s, 0.072 + 0.928*s},
	}, 0)
}

// hueRotateMatrix is feColorMatrix type="hueRotate" (§9.6), for an angle in
// degrees.
func hueRotateMatrix(deg float64) [20]float64 {
	sin, cos := math.Sincos(deg * math.Pi / 180)
	return colourMatrix([3][3]float64{
		{0.213 + cos*0.787 - sin*0.213, 0.715 - cos*0.715 - sin*0.715, 0.072 - cos*0.072 + sin*0.928},
		{0.213 - cos*0.213 + sin*0.143, 0.715 + cos*0.285 + sin*0.140, 0.072 - cos*0.072 - sin*0.283},
		{0.213 - cos*0.213 - sin*0.787, 0.715 - cos*0.715 + sin*0.715, 0.072 + cos*0.928 + sin*0.072},
	}, 0)
}

// invertMatrix is the table [amount, 1 − amount] of §13.1.5: a linear map from
// amount at 0 to 1 − amount at 1.
func invertMatrix(amount float64) [20]float64 {
	d := 1 - 2*amount
	return colourMatrix([3][3]float64{{d, 0, 0}, {0, d, 0}, {0, 0, d}}, amount)
}

// brightnessMatrix is §13.1.7's linear transfer of slope amount.
func brightnessMatrix(amount float64) [20]float64 {
	return colourMatrix([3][3]float64{{amount, 0, 0}, {0, amount, 0}, {0, 0, amount}}, 0)
}

// contrastMatrix is §13.1.8's linear transfer of slope amount and intercept
// −(0.5 × amount) + 0.5.
func contrastMatrix(amount float64) [20]float64 {
	return colourMatrix([3][3]float64{{amount, 0, 0}, {0, amount, 0}, {0, 0, amount}}, 0.5-0.5*amount)
}

// applyMatrix maps a colour through a filter matrix, unclamped: the components
// as fractions, alpha not premultiplied.
func applyMatrix(m [20]float64, c style.RGBA) (r, g, b, a float64) {
	in := [5]float64{c.R / 255, c.G / 255, c.B / 255, c.A, 1}
	var out [4]float64
	for row := 0; row < 4; row++ {
		for col := 0; col < 5; col++ {
			out[row] += m[row*5+col] * in[col]
		}
	}
	return out[0], out[1], out[2], out[3]
}

// matrixKeeps reports whether a matrix maps a colour inside [0, 1] and leaves
// its alpha as it was, which is when folding the matrix into it is exact.
func matrixKeeps(m [20]float64, c style.RGBA) bool {
	r, g, b, a := applyMatrix(m, c)
	for _, v := range [3]float64{r, g, b} {
		if v < -maxFilterMatrixError || v > 1+maxFilterMatrixError {
			return false
		}
	}
	return math.Abs(a-c.A) <= maxFilterMatrixError
}

// recoloured is a colour mapped through a matrix that keeps it.
func recoloured(m [20]float64, c style.RGBA) style.RGBA {
	r, g, b, _ := applyMatrix(m, c)
	clamp := func(v float64) float64 { return math.Max(0, math.Min(1, v)) * 255 }
	return style.RGBA{R: clamp(r), G: clamp(g), B: clamp(b), A: c.A}
}

// eachColour calls f with every colour a list of operations paints in, and
// reports false for an operation whose colours are not all its own to state —
// a picture — or a group whose filter is one only a backend applies.
func eachColour(ops []Op, f func(style.RGBA) bool) bool {
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			if !f(v.Color) {
				return false
			}
		case FillPath:
			if !f(v.Color) {
				return false
			}
		case FillGradient:
			for _, s := range v.Gradient.Stops {
				if !f(s.Color) {
					return false
				}
			}
		case DrawText:
			if !f(v.Color) {
				return false
			}
		case DrawTextShadow:
			if !f(v.Run.Color) {
				return false
			}
		case DrawEmphasisMark:
			if !f(v.Mark.Color) {
				return false
			}
		case DrawGlyphs:
			if !f(v.Color) {
				return false
			}
		case ClipPath:
			if !eachColour(v.Ops, f) {
				return false
			}
		case FilterGroup:
			if !onlyBlurAndOpacity(v.Filters) || !eachColour(v.Ops, f) {
				return false
			}
		case Link:
		default:
			// A picture, or anything else whose colours are not stated here.
			return false
		}
	}
	return true
}

// onlyBlurAndOpacity reports whether a chain holds nothing but blurs and
// opacities, which commute with a matrix that clamps nothing.
func onlyBlurAndOpacity(chain []FilterFunction) bool {
	for _, f := range chain {
		if f.Kind != FilterBlur && f.Kind != FilterOpacity {
			return false
		}
	}
	return true
}

// mapColours is a copy of ops with every colour mapped by f; see eachColour
// for which operations it expects.
func mapColours(ops []Op, f func(style.RGBA) style.RGBA) []Op {
	out := make([]Op, 0, len(ops))
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			v.Color = f(v.Color)
			op = v
		case FillPath:
			v.Color = f(v.Color)
			op = v
		case FillGradient:
			stops := make([]GradientStop, len(v.Gradient.Stops))
			for i, s := range v.Gradient.Stops {
				s.Color = f(s.Color)
				stops[i] = s
			}
			v.Gradient.Stops = stops
			op = v
		case DrawText:
			v.Color = f(v.Color)
			op = v
		case DrawTextShadow:
			v.Run.Color = f(v.Run.Color)
			op = v
		case DrawEmphasisMark:
			v.Mark.Color = f(v.Mark.Color)
			op = v
		case DrawGlyphs:
			v.Color = f(v.Color)
			op = v
		case ClipPath:
			v.Ops = mapColours(v.Ops, f)
			op = v
		case FilterGroup:
			v.Ops = mapColours(v.Ops, f)
			op = v
		}
		out = append(out, op)
	}
	return out
}

// foldMatrix is ops with a colour matrix applied to their colours, when that
// is exact (see the note at the top of this file), and false when it is not.
func foldMatrix(ops []Op, m [20]float64) ([]Op, bool) {
	clamps := false
	if !eachColour(ops, func(c style.RGBA) bool {
		clamps = clamps || !matrixKeeps(m, c)
		return true
	}) {
		return nil, false
	}
	if clamps && !unmixed(ops, m) {
		return nil, false
	}
	return mapColours(ops, func(c style.RGBA) style.RGBA { return recoloured(m, c) }), true
}

// maxUnmixedComparisons bounds how many pairs of marks unmixed compares; past
// it the answer is no, and the matrix stays in the group, which is always
// exact.
const maxUnmixedComparisons = 4096

// unmixed reports whether no two colours of ops mix anywhere on the page, so
// that a matrix that clamps some of them is still exact folded into each: no
// group inside, no gradient whose own colours the matrix clamps, and no mark
// that lets another show through lying over one, asked of their rectangles.
func unmixed(ops []Op, m [20]float64) bool {
	type mark struct {
		r     Rect
		shows bool // whether what is under it shows through
	}
	var marks []mark
	var gather func(ops []Op) bool
	gather = func(ops []Op) bool {
		for _, op := range ops {
			r, _ := opBounds(op)
			switch v := op.(type) {
			case FillRect:
				marks = append(marks, mark{r, v.Color.A < 1})
			case FillPath:
				marks = append(marks, mark{r, v.Color.A < 1})
			case DrawText:
				marks = append(marks, mark{r, v.Color.A < 1})
			case DrawEmphasisMark:
				marks = append(marks, mark{r, v.Mark.Color.A < 1})
			case DrawGlyphs:
				marks = append(marks, mark{r, v.Color.A < 1})
			case DrawTextShadow:
				marks = append(marks, mark{r, v.Run.Color.A < 1 || v.StdDev > 0})
			case FillGradient:
				shows := false
				for _, s := range v.Gradient.Stops {
					if !matrixKeeps(m, s.Color) {
						return false
					}
					shows = shows || s.Color.A < 1
				}
				marks = append(marks, mark{r, shows})
			case ClipPath:
				// A curve only takes ink away.
				if !gather(v.Ops) {
					return false
				}
			case Link:
			default:
				return false
			}
		}
		return true
	}
	if !gather(ops) {
		return false
	}
	n := 0
	for i, a := range marks {
		if !a.shows || a.r.Empty() {
			continue
		}
		for j, b := range marks {
			if i == j || b.r.Empty() {
				continue
			}
			if n++; n > maxUnmixedComparisons {
				return false
			}
			if a.r.X < b.r.Right() && b.r.X < a.r.Right() && a.r.Y < b.r.Bottom() && b.r.Y < a.r.Bottom() {
				return false
			}
		}
	}
	return true
}

// Drop shadows.

// shadowable reports whether ops are marks whose alpha a drop shadow can be
// made of: no picture, and no group whose filter is one only a backend applies
// that could change an alpha — a drop shadow. A group's colour matrix changes
// no alpha and is dropped from the shadow's copy.
func shadowable(ops []Op) bool {
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect, FillPath, FillGradient, DrawText, DrawTextShadow, DrawEmphasisMark, DrawGlyphs, Link:
		case ClipPath:
			if !shadowable(v.Ops) {
				return false
			}
		case FilterGroup:
			for _, f := range v.Filters {
				if f.Kind == FilterDropShadow {
					return false
				}
			}
			if !shadowable(v.Ops) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// shadowOps is the drop shadow of ops: each mark moved by the offset and drawn
// in the shadow's colour at its own alpha, a run of text as a DrawTextShadow,
// and all of it blurred and faded by the colour's alpha as one group. Links are
// not ink and have no shadow.
//
// Every fill of a shadow is an Overhang: it is ink the filter's offset put
// where no layout decision placed anything, as a text decoration and an
// outline are, and the overflow-page guardrail, which refuses a document whose
// boxes leave the page, must not read a shadow past the edge as one of them.
func shadowOps(ops []Op, f FilterFunction) []Op {
	marks := shadowMarks(ops, f.Offset, f.Color)
	if len(marks) == 0 {
		return nil
	}
	var chain []FilterFunction
	if f.StdDev > 0 {
		chain = append(chain, FilterFunction{Kind: FilterBlur, StdDev: f.StdDev})
	}
	if f.Color.A < 1 {
		chain = append(chain, FilterFunction{Kind: FilterOpacity, Amount: f.Color.A})
	}
	if len(chain) == 0 {
		// One opaque colour, sharp: the marks drawn in it are the union of
		// their alphas already.
		return marks
	}
	return []Op{newFilterGroup(chain, marks)}
}

func shadowMarks(ops []Op, d Point, c style.RGBA) []Op {
	tint := func(a float64) style.RGBA { return style.RGBA{R: c.R, G: c.G, B: c.B, A: a} }
	moveRect := func(r Rect) Rect { return Rect{X: r.X.Add(d.X), Y: r.Y.Add(d.Y), W: r.W, H: r.H} }
	moveClip := func(cl Clip) Clip {
		if cl.Active {
			cl.Rect = moveRect(cl.Rect)
		}
		return cl
	}
	movePoint := func(p Point) Point { return Point{X: p.X.Add(d.X), Y: p.Y.Add(d.Y)} }
	moveRun := func(r DrawText) DrawText {
		r.At = movePoint(r.At)
		r.Clip = moveClip(r.Clip)
		r.Color = tint(r.Color.A)
		return r
	}
	var out []Op
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			v.Rect = moveRect(v.Rect)
			v.Color = tint(v.Color.A)
			v.Overhang = true
			out = append(out, v)
		case FillPath:
			v.Path = movePath(v.Path, d)
			v.Clip = moveClip(v.Clip)
			v.Color = tint(v.Color.A)
			v.Overhang = true
			out = append(out, v)
		case FillGradient:
			v.Clip, v.Tile = moveRect(v.Clip), moveRect(v.Tile)
			v.Overhang = true
			stops := make([]GradientStop, len(v.Gradient.Stops))
			for i, s := range v.Gradient.Stops {
				s.Color = tint(s.Color.A)
				stops[i] = s
			}
			v.Gradient.Stops = stops
			out = append(out, v)
		case DrawText:
			out = append(out, DrawTextShadow{Run: moveRun(v)})
		case DrawTextShadow:
			v.Run = moveRun(v.Run)
			out = append(out, v)
		case DrawEmphasisMark:
			out = append(out, DrawTextShadow{Run: moveRun(v.Mark)})
		case DrawGlyphs:
			// The same glyphs, moved and in the shadow's colour, and standing
			// for no text: a shadow is not the document's text twice.
			v.At, v.Clip, v.Color, v.Text = movePoint(v.At), moveClip(v.Clip), tint(v.Color.A), ""
			out = append(out, v)
		case ClipPath:
			inner := shadowMarks(v.Ops, d, c)
			if len(inner) > 0 {
				out = append(out, ClipPath{Path: movePath(v.Path, d), Ops: inner})
			}
		case FilterGroup:
			inner := shadowMarks(v.Ops, d, c)
			if len(inner) == 0 {
				continue
			}
			// The group's chain less its colour matrices, which change no
			// alpha; what is left is blurs and opacities, merged as a group
			// states them.
			var chain []FilterFunction
			for _, f := range v.Filters {
				if f.Kind != FilterColorMatrix {
					chain = appendFilter(chain, f)
				}
			}
			g := newFilterGroup(chain, inner)
			g.Clip = moveClip(v.Clip)
			out = append(out, g)
		}
	}
	return out
}

// movePath is a path moved by an offset.
func movePath(p Path, d Point) Path {
	out := make(Path, len(p))
	for i, s := range p {
		s.Point = Point{X: s.Point.X.Add(d.X), Y: s.Point.Y.Add(d.Y)}
		s.Center = Point{X: s.Center.X.Add(d.X), Y: s.Center.Y.Add(d.Y)}
		out[i] = s
	}
	return out
}
