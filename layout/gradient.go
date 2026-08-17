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
