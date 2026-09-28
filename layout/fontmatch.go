package layout

import "math"

// CSS Fonts 4 §5.2: choosing a face from a family by width, style and weight.
//
// A family defined by @font-face rules is a set of faces, each declaring the
// ranges of font-width, font-style and font-weight it is for. §5.2 narrows the
// set one property at a time — width, then style, then weight — and at each
// step finds the value nearest the one asked for that some face in the set
// offers, by a search whose direction depends on the request, and keeps the
// faces that offer it. A face whose range is continuous — a variable face
// declared "font-weight: 100 900" — offers every value in it, which is what
// "the algorithm proceeds as if each supported combination of values is a
// unique font in the set" asks.
//
// It used to score the faces instead: the style counted a million, the weight
// its distance, and the lowest score won. That tried the style before the
// width it had no notion of, compared an oblique face to an italic request as
// a mismatch whatever its angle, and ranked the weights by a distance that
// agreed with the search only while every face stated one weight.
//
// # What a face offers
//
// A descriptor of "auto" — every descriptor's initial value — is selected as
// the property's normal value (§4.4): weight 400, width 100%, upright. An
// upright face has an italic value of 0 and an oblique angle of 0; an italic
// one an italic value of 1 and no oblique angle; an oblique one its angles and
// no italic value. "left" and "right" are italic here: nothing in a rule says
// which way the face leans, and §5.2 gives them no search of their own.

// valueRange is an inclusive range a face offers of one property.
type valueRange struct{ lo, hi float64 }

func (r valueRange) contains(v float64) bool { return v >= r.lo && v <= r.hi }

// probe is one step of a §5.2 search: from a value, up or down, as far as a
// limit. strict leaves the limit itself out — "until 0 is hit", which is
// §5.2's way of saying only positive (or only negative) values are looked at.
type probe struct {
	from   float64
	up     bool
	limit  float64
	strict bool
}

// nearest is the value a probe reaches first among ranges, if any.
func (p probe) nearest(ranges []valueRange) (float64, bool) {
	best, found := 0.0, false
	for _, r := range ranges {
		var v float64
		if p.up {
			if r.hi < p.from {
				continue
			}
			v = math.Max(r.lo, p.from)
			if v > p.limit || p.strict && v == p.limit {
				continue
			}
			if !found || v < best {
				best, found = v, true
			}
			continue
		}
		if r.lo > p.from {
			continue
		}
		v = math.Min(r.hi, p.from)
		if v < p.limit || p.strict && v == p.limit {
			continue
		}
		if !found || v > best {
			best, found = v, true
		}
	}
	return best, found
}

func up(from float64) probe   { return probe{from: from, up: true, limit: math.Inf(1)} }
func down(from float64) probe { return probe{from: from, limit: math.Inf(-1)} }

// search runs probes in order and answers with the first value one reaches.
func search(ranges []valueRange, probes ...probe) (float64, bool) {
	for _, p := range probes {
		if v, ok := p.nearest(ranges); ok {
			return v, true
		}
	}
	return 0, false
}

// widthProbes are §5.2's width search: at or below 100% narrower first, above
// it wider first.
func widthProbes(want float64) []probe {
	if want <= 100 {
		return []probe{down(want), up(want)}
	}
	return []probe{up(want), down(want)}
}

// weightProbes are §5.2's weight search. Between 400 and 500 the heavier
// weights up to 500 come first, then the lighter ones, then the rest of the
// heavier ones; below 400 lighter first; above 500 heavier first.
func weightProbes(want float64) []probe {
	switch {
	case want >= 400 && want <= 500:
		return []probe{{from: want, up: true, limit: 500}, down(want), up(500)}
	case want < 400:
		return []probe{down(want), up(want)}
	}
	return []probe{up(want), down(want)}
}

// slopeOffer is what a face offers of font-style: its italic values and its
// oblique angles, either of which may be absent.
type slopeOffer struct {
	italic, oblique []valueRange
}

// slopeStep is one step of the style search: which of the two scales it looks
// at, and the probe.
type slopeStep struct {
	oblique bool
	probe
}

// slopeProbes are §5.2's style search for a request.
//
// The one threshold is 11 degrees, which is where §5.2 puts an italic on the
// oblique scale ("an italic value of 1 must map to the same value that an
// oblique angle of 11deg maps to"): a request at or past it looks at steeper
// angles first, one short of it at shallower ones. A negative request is the
// same search mirrored, "with the negated values and opposite directions".
func slopeProbes(r FontRequest) []slopeStep {
	// Down to zero and not past it, and its mirror: §5.2's "until 0 is hit",
	// which keeps a positive request among positive values.
	positive := func(from float64) probe { return probe{from: from, limit: 0, strict: true} }
	negative := func(from float64) probe { return probe{from: from, up: true, limit: 0, strict: true} }
	// Exactly the value asked for, which is where the searches that start by
	// looking the other way begin.
	exactly := func(v float64) probe { return probe{from: v, up: true, limit: v} }
	switch r.Slope {
	case SlopeItalic:
		return []slopeStep{
			{false, up(1)}, {false, positive(1)},
			{true, up(11)}, {true, positive(11)},
			{false, down(0)}, {true, down(0)},
		}
	case SlopeOblique:
		a := r.Angle
		switch {
		case a >= 11:
			return []slopeStep{
				{true, up(a)}, {true, positive(a)},
				{false, up(1)}, {false, positive(1)},
				{true, down(0)}, {false, down(0)},
			}
		case a >= 0:
			return []slopeStep{
				{true, exactly(a)},
				{true, positive(a)}, {true, up(a)},
				{false, positive(1)}, {false, up(1)},
				{true, down(0)}, {false, down(0)},
			}
		case a > -11:
			return []slopeStep{
				{true, exactly(a)},
				{true, negative(a)}, {true, down(a)},
				{false, negative(-1)}, {false, down(-1)},
				{true, up(0)}, {false, up(0)},
			}
		default:
			return []slopeStep{
				{true, down(a)}, {true, negative(a)},
				{false, down(-1)}, {false, negative(-1)},
				{true, up(0)}, {false, up(0)},
			}
		}
	}
	// Normal: an upright face, whose oblique angle and italic value are both
	// zero, then leaning right, then left.
	return []slopeStep{{true, up(0)}, {false, up(0)}, {true, down(0)}, {false, down(0)}}
}

// slopeMatch is where the style search came to rest: an oblique angle, or an
// italic value.
type slopeMatch struct {
	oblique bool
	value   float64
}

// searchSlope runs the style search over what the faces offer.
func searchSlope(r FontRequest, offers []slopeOffer) (slopeMatch, bool) {
	var italic, oblique []valueRange
	for _, o := range offers {
		italic = append(italic, o.italic...)
		oblique = append(oblique, o.oblique...)
	}
	for _, s := range slopeProbes(r) {
		ranges := italic
		if s.oblique {
			ranges = oblique
		}
		if v, ok := s.nearest(ranges); ok {
			return slopeMatch{oblique: s.oblique, value: v}, true
		}
	}
	return slopeMatch{}, false
}

// offers reports whether a face offers the style the search came to rest at.
func (o slopeOffer) offers(m slopeMatch) bool {
	ranges := o.italic
	if m.oblique {
		ranges = o.oblique
	}
	for _, r := range ranges {
		if r.contains(m.value) {
			return true
		}
	}
	return false
}

// faceMatch is where §5.2 came to rest for a request: the width, the style and
// the weight the chosen faces offer. It is what a variable face among them is
// then set at (§7.2's second step, "the closest matching value as determined
// by the font matching algorithm").
type faceMatch struct {
	width, weight float64
	slope         slopeMatch
}

// matchable is one face of a family as §5.2 sees it.
type matchable struct {
	width, weight valueRange
	slope         slopeOffer
}

// matchFaces narrows a family to the faces §5.2 chooses for a request, and
// says where it came to rest. keep holds the indices of the faces kept, in
// the order they were given. A family of no faces keeps none.
func matchFaces(faces []matchable, r FontRequest) (keep []int, m faceMatch) {
	keep = make([]int, len(faces))
	for i := range keep {
		keep[i] = i
	}
	if len(faces) == 0 {
		return nil, m
	}
	ranges := func(of func(matchable) valueRange) []valueRange {
		out := make([]valueRange, len(keep))
		for i, k := range keep {
			out[i] = of(faces[k])
		}
		return out
	}
	narrow := func(ok func(matchable) bool) {
		out := keep[:0]
		for _, k := range keep {
			if ok(faces[k]) {
				out = append(out, k)
			}
		}
		keep = out
	}

	// Every face offers a width and a weight, so each search finds one of
	// the values offered and at least one face survives it.
	m.width, _ = search(ranges(func(f matchable) valueRange { return f.width }), widthProbes(r.Width)...)
	narrow(func(f matchable) bool { return f.width.contains(m.width) })

	offers := make([]slopeOffer, len(keep))
	for i, k := range keep {
		offers[i] = faces[k].slope
	}
	if s, ok := searchSlope(r, offers); ok {
		m.slope = s
		narrow(func(f matchable) bool { return f.slope.offers(s) })
	}

	m.weight, _ = search(ranges(func(f matchable) valueRange { return f.weight }), weightProbes(r.Weight)...)
	narrow(func(f matchable) bool { return f.weight.contains(m.weight) })
	return keep, m
}
