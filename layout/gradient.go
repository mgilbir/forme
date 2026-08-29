package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// Gradients, and the one shape of them this engine paints.
//
// # What is here
//
// A linear-gradient whose colour stops are all the same colour. That is not an
// approximation of a gradient: a gradient's colour at a point is interpolated
// between the stops around it, so a gradient whose every stop is green is green
// everywhere, and painting it as a solid fill of that green is exact.
//
// # Why that shape and not the general one
//
// Because it is the one the web actually writes. CSS has no way to say "an image
// that is a solid colour", and an author who needs one — to place a swatch at a
// stated size and position with background-size and background-position, which
// background-color cannot do — writes linear-gradient(green, green). Forty of
// the forty-two gradients in the CSS Working Group's suite are of that form, and
// none of them wants a gradient.
//
// A real gradient needs a paint operation this display list does not have: an
// axial shading, interpolating between stops along a line. FillRect cannot
// express it and decomposing it into bands would be a rasteriser writing a
// thousand rectangles where a backend has one primitive. So a gradient with two
// different colours in it is still reported as unpainted, and reported exactly
// as it was — this narrows what is unsupported rather than papering over it.
//
// # Why a fill and not a synthesised image
//
// Because a solid fill is what it is. Routing it through a one-pixel picture
// stretched over the area would paint the same pixels and produce a different
// display list — DrawImage where the same page written with background-color
// produces FillRect — and the two documents would then compare unequal while
// looking identical. The op set's own rule is that anything a backend cannot
// draw directly is decomposed here, and this decomposes to the primitive it
// already has.

// uniformGradient reads a CSS image value and returns the single colour it
// paints, when it is a gradient that paints one.
//
// The direction is not read, and does not need to be: a gradient of one colour
// is that colour whichever way its line points, so "to right" and "45deg" and no
// direction at all give the same page. Anything with two colours in it, or a
// stop this engine cannot resolve, returns false and is left to the caller to
// report.
func uniformGradient(raw string) (style.RGBA, bool) {
	vals, errs := css.ParseComponentValues(raw)
	if len(errs) > 0 {
		return style.RGBA{}, false
	}
	fn, ok := soleFunction(vals)
	if !ok {
		return style.RGBA{}, false
	}
	switch strings.ToLower(fn.Token.Value) {
	case "linear-gradient", "repeating-linear-gradient":
		// A repeating gradient of one colour is that colour too: every
		// repetition paints the same thing.
	default:
		// radial and conic gradients are the same argument and would work the
		// same way, but nothing in the suite writes one and an untested path
		// that silently paints is worse than a reported one that does not.
		return style.RGBA{}, false
	}

	var found *style.RGBA
	for _, arg := range splitTopLevelCommas(fn.Values) {
		colour, ok := stopColour(arg)
		if !ok {
			// The first argument of a linear-gradient may be its direction —
			// "to right", "45deg" — which is not a stop and not a colour.
			// Anything else that is not a colour is a stop this engine cannot
			// read, and a gradient with an unreadable stop is not known to be
			// uniform.
			if found == nil && isGradientDirection(arg) {
				continue
			}
			return style.RGBA{}, false
		}
		if found == nil {
			found = &colour
			continue
		}
		if colour != *found {
			return style.RGBA{}, false // a real gradient
		}
	}
	// One stop is not a gradient any specification allows, so it is refused
	// rather than treated as a fill: a value this engine cannot parse the way a
	// browser does must not be painted as though it could.
	if found == nil || countStops(fn.Values) < 2 {
		return style.RGBA{}, false
	}
	return *found, true
}

// soleFunction returns the single function a value consists of, ignoring the
// white space around it. A background-image layer that is a gradient is exactly
// one function and nothing else.
func soleFunction(vals []css.ComponentValue) (css.ComponentValue, bool) {
	var fn css.ComponentValue
	found := false
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if !v.IsFunction() || found {
			return css.ComponentValue{}, false
		}
		fn, found = v, true
	}
	return fn, found
}

// stopColour reads one colour stop, which is a colour and optionally one or two
// positions. The positions are not read: where a stop sits cannot change the
// colour of a gradient whose stops are all one colour.
func stopColour(arg []css.ComponentValue) (style.RGBA, bool) {
	var colour []css.ComponentValue
	for _, v := range arg {
		if v.IsToken() {
			switch v.Token.Kind {
			case css.Whitespace:
				if len(colour) > 0 {
					// The colour has ended; what follows is its position.
					return style.ParseColor(colour)
				}
				continue
			case css.Percentage, css.Dimension, css.Number:
				if len(colour) == 0 {
					return style.RGBA{}, false
				}
				return style.ParseColor(colour)
			}
		}
		colour = append(colour, v)
	}
	if len(colour) == 0 {
		return style.RGBA{}, false
	}
	return style.ParseColor(colour)
}

// isGradientDirection reports whether an argument is a linear-gradient's leading
// direction rather than a colour stop.
//
// It is recognised only to be skipped. What it must not do is let a *colour*
// through as a direction, because that would drop a stop from the comparison and
// call a two-colour gradient uniform — so it names the two forms the grammar
// allows and nothing else.
func isGradientDirection(arg []css.ComponentValue) bool {
	var words []string
	for _, v := range arg {
		if !v.IsToken() {
			return false
		}
		switch v.Token.Kind {
		case css.Whitespace:
			continue
		case css.Ident:
			words = append(words, strings.ToLower(v.Token.Value))
		case css.Dimension:
			// An angle: "45deg", "0.25turn", "100grad", "1.5rad".
			switch strings.ToLower(v.Token.Unit) {
			case "deg", "grad", "rad", "turn":
				words = append(words, "<angle>")
			default:
				return false
			}
		default:
			return false
		}
	}
	if len(words) == 1 {
		return words[0] == "<angle>"
	}
	// "to <side-or-corner>": "to right", "to bottom left".
	if len(words) < 2 || len(words) > 3 || words[0] != "to" {
		return false
	}
	for _, w := range words[1:] {
		switch w {
		case "top", "bottom", "left", "right":
		default:
			return false
		}
	}
	return true
}

// countStops counts the arguments that are not the leading direction, which is
// how many colour stops a gradient declares.
func countStops(args []css.ComponentValue) int {
	n := 0
	for i, arg := range splitTopLevelCommas(args) {
		if i == 0 && isGradientDirection(arg) {
			continue
		}
		n++
	}
	return n
}

// splitTopLevelCommas splits a function's arguments on the commas between them.
func splitTopLevelCommas(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	var cur []css.ComponentValue
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Comma {
			out = append(out, cur)
			cur = nil
			continue
		}
		cur = append(cur, v)
	}
	return append(out, cur)
}

// The second shape: a gradient whose colour never interpolates.
//
// A gradient's colour between two stops is interpolated, and this display list
// has no operation for that. But between two stops of the *same colour* there is
// nothing to interpolate, and between two stops at the *same position* there is
// nowhere to do it — so a gradient where every neighbouring pair is one or the
// other is a stack of solid bands, exactly, with no rasterising and no
// approximation. That is the shape the working group's suite writes when it
// wants a two-colour marker: "red 50%, green 50%" is a red half above a green
// half and nothing else.
//
// The direction is read here, unlike in uniformGradient, and only the four side
// keywords are taken. A gradient down or across a box has bands that are
// rectangles; one at 45 degrees has bands that are diagonal strips, which
// FillRect cannot express any better than the interpolation could. An angle is
// therefore left reported rather than approximated, and so is a corner.

// bandedGradient is a linear gradient that paints solid bands: its stops, fixed
// up per CSS Images 3 §3.4.2, together with the axis they are measured along.
type bandedGradient struct {
	// vertical says the gradient line runs down the box rather than across it.
	vertical bool
	// reverse says it runs from the far edge back: "to top" and "to left".
	reverse bool
	// stops are in the order written, with every position placed and
	// non-decreasing. There are always at least two.
	stops []bandStop
}

// bandStop is one colour stop with its position along the gradient line.
type bandStop struct {
	at     style.Length
	placed bool
	colour style.RGBA
}

// gradientBand is one solid stripe of the finished picture: where it starts and
// ends along the gradient line, and what colour it is.
type gradientBand struct {
	from, to style.Unit
	colour   style.RGBA
}

// bandsOf reads a CSS image value as a gradient of solid bands.
//
// b is needed because a stop may be written in em, which is a length only once
// the element's font size is known. The box is the one the background is on,
// which is the element those units are relative to.
func (l *layouter) bandsOf(b *Box, raw string) (*bandedGradient, bool) {
	vals, errs := css.ParseComponentValues(raw)
	if len(errs) > 0 {
		return nil, false
	}
	fn, ok := soleFunction(vals)
	if !ok {
		return nil, false
	}
	// repeating-linear-gradient is not here: its bands repeat along the line,
	// which is a tiling of stripes rather than a stack of them, and nothing in
	// the suite writes one with hard stops.
	if !strings.EqualFold(fn.Token.Value, "linear-gradient") {
		return nil, false
	}

	g := &bandedGradient{vertical: true} // no direction means "to bottom"
	args := splitTopLevelCommas(fn.Values)
	if len(args) > 0 {
		if vertical, reverse, isSide := gradientSide(args[0]); isSide {
			g.vertical, g.reverse = vertical, reverse
			args = args[1:]
		}
		// An angle or a corner is not taken, and needs no branch to refuse it:
		// it is not one of the four sides, so it stays in the list and is read
		// as a colour stop, where "45deg" is a position with no colour and
		// "to bottom right" is not a colour. Either way the gradient is refused.
		// A planted defect proved the branch that used to say so was never the
		// thing doing it.
	}
	if len(args) < 2 {
		return nil, false
	}
	for _, arg := range args {
		stop, ok := l.bandStop(b, arg)
		if !ok {
			return nil, false
		}
		g.stops = append(g.stops, stop)
	}
	if !g.fixUpPositions() || !g.constant() {
		return nil, false
	}
	return g, true
}

// gradientSide reads "to top", "to bottom", "to left" or "to right", which are
// the directions whose bands are rectangles.
func gradientSide(arg []css.ComponentValue) (vertical, reverse, ok bool) {
	var words []string
	for _, v := range arg {
		if !v.IsToken() {
			return false, false, false
		}
		if v.Token.Kind == css.Whitespace {
			continue
		}
		if v.Token.Kind != css.Ident {
			return false, false, false
		}
		words = append(words, strings.ToLower(v.Token.Value))
	}
	if len(words) != 2 || words[0] != "to" {
		return false, false, false
	}
	switch words[1] {
	case "bottom":
		return true, false, true
	case "top":
		return true, true, true
	case "right":
		return false, false, true
	case "left":
		return false, true, true
	}
	return false, false, false
}

// bandStop reads one colour stop: a colour, and at most one position.
//
// The two-position form css-images-4 allows — a colour written with both ends of
// its own band — is refused rather than expanded, and so is a bare position,
// which is an interpolation hint and says the colour between two stops is *not*
// what either of them is.
func (l *layouter) bandStop(b *Box, arg []css.ComponentValue) (bandStop, bool) {
	colour, pos, ok := splitStop(arg)
	if !ok {
		return bandStop{}, false
	}
	c, ok := style.ParseColor(colour)
	if !ok {
		return bandStop{}, false
	}
	if len(pos) == 0 {
		return bandStop{colour: c}, true
	}
	length, ok := l.lengthOfValues(b, pos)
	if !ok {
		return bandStop{}, false
	}
	switch length.Kind {
	case style.LengthAbsolute, style.LengthPercent:
	default:
		// auto is not a position, and a calc() that came out as both a length
		// and a percentage cannot be ordered against its neighbours until the
		// gradient line has a length. Neither is read here.
		return bandStop{}, false
	}
	return bandStop{at: length, placed: true, colour: c}, true
}

// splitStop divides a stop into its colour and its position.
func splitStop(arg []css.ComponentValue) (colour, pos []css.ComponentValue, ok bool) {
	for _, v := range arg {
		if v.IsToken() {
			switch v.Token.Kind {
			case css.Whitespace:
				if len(colour) > 0 {
					continue
				}
				continue
			case css.Percentage, css.Dimension, css.Number:
				if len(colour) == 0 {
					return nil, nil, false // an interpolation hint
				}
				pos = append(pos, v)
				continue
			}
		}
		if len(pos) > 0 {
			return nil, nil, false // a second position, or a colour after one
		}
		colour = append(colour, v)
	}
	if len(colour) == 0 {
		return nil, nil, false
	}
	return colour, pos, true
}

// fixUpPositions is CSS Images 3 §3.4.2: the first and last stops are placed at
// the ends of the line if they were not placed, an unplaced stop between two
// placed ones is spread evenly between them, and a stop that would sit before
// one written earlier is moved up to it.
//
// It reports false when a position cannot be worked out without knowing how long
// the gradient line is, which is the case a length and a percentage in the same
// gradient produce: "green 4em, red 50%" has an order that depends on the box.
// Every stop must therefore end up the same kind of length as every other.
func (g *bandedGradient) fixUpPositions() bool {
	if !g.stops[0].placed {
		g.stops[0] = bandStop{at: style.Length{Kind: style.LengthPercent}, placed: true, colour: g.stops[0].colour}
	}
	if last := len(g.stops) - 1; !g.stops[last].placed {
		g.stops[last] = bandStop{
			at:     style.Length{Kind: style.LengthPercent, Percent: 100},
			placed: true,
			colour: g.stops[last].colour,
		}
	}
	kind := g.stops[0].at.Kind
	for _, s := range g.stops {
		if s.placed && s.at.Kind != kind {
			return false
		}
	}
	// Spread each run of unplaced stops evenly between the placed stops around
	// it. Both ends are placed by now, so every run has two.
	for i := 0; i < len(g.stops); i++ {
		if g.stops[i].placed {
			continue
		}
		j := i
		for !g.stops[j].placed {
			j++
		}
		lo, hi := value(g.stops[i-1].at), value(g.stops[j].at)
		step := (hi - lo) / float64(j-i+1)
		for k := i; k < j; k++ {
			g.stops[k].at = withValue(kind, lo+step*float64(k-i+1))
			g.stops[k].placed = true
		}
		i = j
	}
	// §3.4.2's last rule: a stop never moves backwards.
	max := value(g.stops[0].at)
	for i := range g.stops {
		if v := value(g.stops[i].at); v < max {
			g.stops[i].at = withValue(kind, max)
		} else {
			max = v
		}
	}
	return true
}

// value is the number a fixed-up stop position carries, in whichever kind the
// gradient's stops are all written in.
func value(l style.Length) float64 {
	if l.Kind == style.LengthPercent {
		return l.Percent
	}
	return l.Value.Px()
}

func withValue(kind style.LengthKind, v float64) style.Length {
	if kind == style.LengthPercent {
		return style.Length{Kind: style.LengthPercent, Percent: v}
	}
	u, _ := style.FromPx(v)
	return style.Length{Kind: style.LengthAbsolute, Value: u}
}

// constant reports whether the gradient interpolates anywhere: between every
// neighbouring pair of stops, either the colour does not change or there is no
// distance for it to change over.
func (g *bandedGradient) constant() bool {
	for i := 0; i+1 < len(g.stops); i++ {
		if g.stops[i].colour == g.stops[i+1].colour {
			continue
		}
		if value(g.stops[i].at) == value(g.stops[i+1].at) {
			continue
		}
		return false
	}
	return true
}

// bandsIn is the finished picture for a tile of the given size: the stripes,
// measured from the tile's top or left edge.
//
// The colour before the first stop is the first stop's and the colour after the
// last is the last stop's, so the bands always cover the whole line however the
// stops are placed.
func (g *bandedGradient) bandsIn(w, h style.Unit) []gradientBand {
	line := w
	if g.vertical {
		line = h
	}
	if line <= 0 {
		return nil
	}
	at := make([]style.Unit, len(g.stops))
	for i, s := range g.stops {
		u, _ := s.at.Resolve(line, true)
		at[i] = clampTo(u, 0, line)
	}
	edges := make([]style.Unit, 0, len(g.stops)+1)
	edges = append(edges, 0)
	edges = append(edges, at...)
	edges = append(edges, line)

	out := make([]gradientBand, 0, len(g.stops)+1)
	for i := 0; i+1 < len(edges); i++ {
		from, to := edges[i], edges[i+1]
		if to <= from {
			continue
		}
		// Band i lies after stop i-1 and before stop i, so its colour is the
		// stop it starts at — which for the first band, before any stop, is the
		// first stop's.
		s := i - 1
		if s < 0 {
			s = 0
		}
		if s > len(g.stops)-1 {
			s = len(g.stops) - 1
		}
		if g.reverse {
			from, to = line.Sub(to), line.Sub(from)
		}
		out = append(out, gradientBand{from: from, to: to, colour: g.stops[s].colour})
	}
	return out
}

func clampTo(u, lo, hi style.Unit) style.Unit {
	if u < lo {
		return lo
	}
	if u > hi {
		return hi
	}
	return u
}
