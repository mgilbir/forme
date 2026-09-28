package layout

import (
	"fmt"
	"math"
	"strings"

	"github.com/mgilbir/forme/style"
)

// A gradient interpolated in a colour space other than sRGB.
//
// CSS Images 4 §3 lets a gradient name the space its colours are interpolated
// in — "linear-gradient(in oklch longer hue, red, blue)" — and CSS Color 4 §13
// says what that is: between two stops, the colour at each point is the two
// stop colours converted to the space, a hue fixed up by the hue interpolation
// method, the rest premultiplied and interpolated linearly, and the result
// converted back. Every colour this engine reads is a legacy sRGB colour, which
// is why "in srgb" was the only method read until now: it is §13.2's default
// for them, and it is what a FillGradient's stops are interpolated in.
//
// # Why the operation's stops stay in sRGB
//
// There were two ways to say this to a backend. A field could name the space
// and the hue method, and every backend learn to interpolate in it; or the
// gradient could be restated as stops a backend already interpolates, in
// premultiplied sRGB, placed closely enough that the straight lines between
// them are the curve the space draws. The second is what is done, because it is
// the one a PDF backend can draw exactly as stated.
//
// A PDF shading's colour is a function of one variable, and a type 4
// (PostScript calculator) function could compute any of these conversions but
// one: a colour the interpolation carries outside sRGB — an oklch "longer hue"
// through saturated blues does — has to be brought back by §14.2's gamut
// mapping, which is a search, and a calculator function has no loop. A field
// would therefore hand every backend a second colour engine and a part of it
// that no PDF function can express; restated stops hand it nothing new. The
// stops are FillGradient's own, with the meaning they always had, so a backend
// written before this draws these gradients too, and ColorAtOffset remains the
// whole definition of what is drawn.
//
// What that costs is a stated error: half an 8-bit step, in every
// premultiplied component and in alpha, which is the error blurReach's note
// calls one any device rounds to nothing. Each segment between two of the
// author's stops is cut into pieces until the straight premultiplied-sRGB line
// across every piece is within four fifths of that of the true colour at seven
// points inside it, which leaves the margin for the points between; a piece is
// not cut below a millionth of its segment (1/2^20), which only a jump in the
// gamut mapping can ask for. The count of stops this makes is bounded by
// maxInterpolatedStops, past which the gradient is not drawn and says so, and
// the work by the document's budget.

// interpolationTolerance is how far the straight line across a piece may be
// from the true colour: half an 8-bit step, with components as fractions.
const interpolationTolerance = 0.5 / 255

// pieceTolerance is what a piece is held to at the points it is checked at:
// less than interpolationTolerance, so that between those points, where it is
// not checked, the line is still within it. TestTheStatedToleranceHolds
// measures the margin over every space at four thousand points a gradient.
const pieceTolerance = 0.4 / 255

// maxInterpolatedStops bounds the stops one gradient is restated as. It is a
// count a stylesheet controls through its stops and its hue methods, and each
// is a function a backend builds, so it is maxGradientStops sixteen times over:
// room for every one of the most stops a gradient may have to be cut in
// sixteen, which a longer hue round a wide chroma needs.
//
// A variable so that a test can lower it.
var maxInterpolatedStops = 16 * maxGradientStops

// minPieceShift is how many times a segment is halved at most: a piece is
// never shorter than 1/2^20 of its segment.
const minPieceShift = 20

// restated is how restating a gradient's stops ended.
type restated uint8

const (
	restatedAll restated = iota
	// restatedTooMany is a gradient that needs more than maxInterpolatedStops.
	restatedTooMany
	// restatedRefused is one whose work the document's budget refused.
	restatedRefused
)

// interpolateStops restates stops interpolated in space s with hue method h
// as stops interpolated in premultiplied sRGB, every Exponent 1, to within
// interpolationTolerance; see the note at the top of this file.
//
// charge is asked for each colour the true interpolation is worked out at,
// before it is, and a refusal ends the work; nil charges nothing.
func interpolateStops(stops []GradientStop, s colorSpace, h hueMethod, charge func() bool) ([]GradientStop, restated) {
	out := make([]GradientStop, 0, 2*len(stops))
	out = append(out, GradientStop{Offset: stops[0].Offset, Color: stops[0].Color, Exponent: 1})
	for i := 1; i < len(stops); i++ {
		a, b := stops[i-1], stops[i]
		if !(b.Offset > a.Offset) {
			// A hard stop: the colour changes here at once and there is
			// nothing between the two to interpolate.
			out = append(out, GradientStop{Offset: b.Offset, Color: b.Color, Exponent: 1})
			continue
		}
		mix := newColourMix(s, h, a.Color, b.Color)
		e := b.Exponent
		if e <= 0 || math.IsNaN(e) || math.IsInf(e, 0) {
			e = 1
		}
		// The true colour at a fraction of the segment. A piece's checks at
		// its quarters are its halves' checks at their halves, so each colour
		// is worked out once: every point asked for is a multiple of a power
		// of two, exact as a key.
		refused := false
		seen := map[float64]style.RGBA{}
		truth := func(p float64) style.RGBA {
			switch {
			case p <= 0:
				return a.Color
			case p >= 1:
				return b.Color
			}
			if c, ok := seen[p]; ok {
				return c
			}
			if refused || (charge != nil && !charge()) {
				refused = true
				return style.RGBA{}
			}
			w := p
			if e != 1 {
				w = math.Pow(p, e)
			}
			c := mix.at(w)
			seen[p] = c
			return c
		}
		span := b.Offset - a.Offset
		tooMany := false
		var cut func(p0, p1 float64, c0, c1 style.RGBA, depth int) bool
		cut = func(p0, p1 float64, c0, c1 style.RGBA, depth int) bool {
			if depth < minPieceShift && !withinTolerance(p0, p1, c0, c1, truth) {
				mid := (p0 + p1) / 2
				cm := truth(mid)
				return !refused && cut(p0, mid, c0, cm, depth+1) && cut(mid, p1, cm, c1, depth+1)
			}
			if refused {
				return false
			}
			if len(out) >= maxInterpolatedStops {
				tooMany = true
				return false
			}
			out = append(out, GradientStop{Offset: a.Offset + span*p1, Color: c1, Exponent: 1})
			return true
		}
		if !cut(0, 1, a.Color, b.Color, 0) {
			if tooMany {
				return nil, restatedTooMany
			}
			return nil, restatedRefused
		}
		// The last stop of the segment at its own offset exactly, rather than
		// at the first's plus the span, which a float need not add back to.
		out[len(out)-1].Offset = b.Offset
	}
	return out, restatedAll
}

// withinTolerance reports whether the straight premultiplied-sRGB line from c0
// to c1 is within pieceTolerance of the true colour at seven points evenly
// inside the piece from p0 to p1.
func withinTolerance(p0, p1 float64, c0, c1 style.RGBA, truth func(float64) style.RGBA) bool {
	for k := 1; k < 8; k++ {
		f := float64(k) / 8
		if premultipliedDistance(mixPremultiplied(c0, c1, f), truth(p0+(p1-p0)*f)) > pieceTolerance {
			return false
		}
	}
	return true
}

// premultipliedDistance is the largest difference between two colours'
// premultiplied components and alphas, as fractions.
func premultipliedDistance(x, y style.RGBA) float64 {
	d := math.Abs(x.A - y.A)
	d = math.Max(d, math.Abs(x.R*x.A-y.R*y.A)/255)
	d = math.Max(d, math.Abs(x.G*x.A-y.G*y.A)/255)
	d = math.Max(d, math.Abs(x.B*x.A-y.B*y.A)/255)
	return d
}

// interpolatedStops is one restatement, as the memo keeps it.
type interpolatedStops struct {
	stops []GradientStop
	how   restated
}

// costColour is what working out one colour of an interpolation is charged:
// sixteen operations. Measured, a colour the gamut search has to bring inside
// sRGB takes about 270 times what emitting an operation does, and one already
// inside it about eleven; at sixteen, the budget's floor pays for some 130,000
// colours, which is a hundred of the costliest two-stop gradients there are
// (an oklch longer hue from red to blue works out about 1,300), and holds a
// document that asks for nothing else to about a second.
const costColour = 16 * costOp

// restater is interpolateStops for a gradient laid out in this document:
// memoized, since one rule that puts one gradient on four hundred boxes of one
// size restates the same stops four hundred times, and charged to the
// document's work budget, costColour for each colour the true interpolation is
// worked out at. The work is a document's to multiply — a longer hue round
// many stops is thousands of colours, each converted and perhaps
// gamut-mapped — and past the budget the gradient is not drawn and the budget
// says so.
func (l *layouter) restater(spec *gradientSpec) func([]GradientStop) ([]GradientStop, restated) {
	if spec.space == spaceSRGB {
		return nil
	}
	return func(stops []GradientStop) ([]GradientStop, restated) {
		var key strings.Builder
		fmt.Fprintf(&key, "%d %d", spec.space, spec.hue)
		for _, s := range stops {
			fmt.Fprintf(&key, " %v %v %v %v %v %v", s.Offset, s.Color.R, s.Color.G, s.Color.B, s.Color.A, s.Exponent)
		}
		if got, ok := l.interpolated[key.String()]; ok {
			return got.stops, got.how
		}
		out, how := interpolateStops(stops, spec.space, spec.hue, func() bool {
			return l.rec.charge(costColour, "the colour interpolation of the gradients past that point, which were not drawn")
		})
		if l.interpolated == nil {
			l.interpolated = map[string]interpolatedStops{}
		}
		if how != restatedRefused {
			l.interpolated[key.String()] = interpolatedStops{stops: out, how: how}
		}
		return out, how
	}
}
