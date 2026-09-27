package layout

import (
	"fmt"
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// Filter Effects 1: the filter property.
//
// # What a filter is, and why it is a group
//
// A filter is applied to an element and everything in it "as a group", the
// same way opacity is (§5): the element is drawn on a surface of its own, the
// surface is filtered, and the result is composited onto the page. Opacity is
// folded into each mark instead of being a group (see opacity.go), and that is
// exact wherever the marks do not overlap. Blur cannot be folded that way at all:
// a blurred mark spreads beyond its own rectangle, so two marks that were apart
// blur into each other, and the order they are clipped and blurred in is
// visible. So a filter is a group, FilterGroup, which holds what it filters —
// like ClipPath, it cannot be left open, because what it applies to is inside it.
//
// # What is applied
//
// blur() and opacity(), which are what the CSS Working Group's suite uses and
// the two a vector backend can say exactly what to do with. Both are linear and
// they commute, and two blurs are one — a Gaussian of deviation a after one of
// deviation b is one of deviation √(a² + b²) — so any chain of them is one blur
// followed by one opacity, and that is what the operation carries.
//
// The other eight functions — brightness(), contrast(), grayscale(),
// hue-rotate(), invert(), saturate(), sepia() and drop-shadow() — and a url()
// reference to an SVG filter are reported where they are declared, and the
// element is drawn without them. The element is still a stacking context, which
// §5 makes it whatever the filter is.
//
// # What else a filter does, and what is not done
//
// §5 also makes the element the containing block of the absolutely and fixed
// positioned boxes inside it. That is layout rather than painting, and this
// engine does not do it: such a box is positioned against the containing block
// it would have without the filter, and that is reported for the box.
//
// A filtered box's own clip and every clip around it are applied to the group
// after it is filtered — §5's "first any filter effect is applied, then any
// clipping" — so a blurred box inside an "overflow: hidden" one is blurred and
// then cut, and its blur does not stop at a hard edge inside it. That is exact
// for a block. For an inline box, whose marks are cut by the clip of the block
// whose lines they are on before the box's group is formed, it is not, and a
// blurred inline box inside a box that clips is reported.

// FilterKind is one filter function.
type FilterKind uint8

const (
	// FilterBlur is blur(): a Gaussian blur of the group, StdDev its standard
	// deviation on both axes, with nothing but transparency outside what the
	// group paints (Filter Effects 1 §13.1.9's edgeMode "none").
	FilterBlur FilterKind = iota + 1
	// FilterOpacity is opacity(): the group's alpha multiplied by Amount.
	FilterOpacity
)

// FilterFunction is one step of a filter chain.
type FilterFunction struct {
	Kind FilterKind
	// StdDev is a blur's standard deviation, greater than zero. It is
	// blur()'s own argument: Filter Effects 1 §6.1 defines it as the standard
	// deviation, where a shadow's blur radius is twice one.
	StdDev style.Unit
	// Amount is an opacity's multiplier, in [0, 1).
	Amount float64
}

// FilterGroup paints its operations as one group, filters the group, and
// composites the result onto the page.
//
// Filters are applied in order, the first to the group as painted. The engine
// emits at most one blur, first, and at most one opacity after it; a chain that
// filters nothing is not a group at all. A PDF backend draws it as a
// transparency group: an opacity is the group's constant alpha, and PDF has no
// Gaussian blur, so a blur is rasterised by the backend — the group drawn at a
// resolution of its choosing and convolved with this deviation — or refused and
// reported. Extent says how far the blurred group reaches.
//
// Clip is applied after the filters, and is set only where something cuts the
// group; the operations inside carry only the clips inside the filtered box.
type FilterGroup struct {
	Filters []FilterFunction
	Ops     []Op
	Clip    Clip

	// ink is where Ops mark, before the blur and the clip, when inkKnown:
	// worked out once when the group is made, since a group inside a group is
	// asked for it by every group around it. See newFilterGroup.
	ink      Rect
	inkKnown bool
}

// newFilterGroup is a group with its ink worked out, which is how the engine
// makes one. A group written as a literal works it out when asked.
func newFilterGroup(filters []FilterFunction, ops []Op) FilterGroup {
	g := FilterGroup{Filters: filters, Ops: ops}
	g.ink, g.inkKnown = opsInk(ops), true
	return g
}

func (FilterGroup) isOp() {}

// blurReach is how far past what it blurs a blur is drawn, in standard
// deviations. A Gaussian has no edge; three deviations out, what is left of a
// mark's edge is 1 - Φ(3) ≈ 0.00135 of it, under the half of an 8-bit step
// (0.00196) that any device rounds to nothing.
const blurReach = 3

// stdDev is the group's blur, or zero.
func (g FilterGroup) stdDev() style.Unit {
	var s style.Unit
	for _, f := range g.Filters {
		if f.Kind == FilterBlur {
			s = style.Max(s, f.StdDev)
		}
	}
	return s
}

// Extent is where the filtered group may put ink: what its operations mark,
// grown by three standard deviations of its blur, and cut by its clip. Past
// that a blur's ink is under half an 8-bit step.
func (g FilterGroup) Extent() Rect {
	out := g.ink
	if !g.inkKnown {
		out = opsInk(g.Ops)
	}
	if out.Empty() {
		return Rect{}
	}
	if s := g.stdDev(); s > 0 {
		d := s.Mul(blurReach)
		out = out.Outset(Edges{Top: d, Right: d, Bottom: d, Left: d})
	}
	if g.Clip.Active {
		out = out.Intersect(g.Clip.Rect)
	}
	return out
}

// opsInk is the smallest rectangle holding everything a list of operations
// may mark.
func opsInk(ops []Op) Rect {
	var out Rect
	have := false
	for _, op := range ops {
		r, ok := opBounds(op)
		if !ok || r.Empty() {
			continue
		}
		if !have {
			out, have = r, true
			continue
		}
		x0, y0 := style.Min(out.X, r.X), style.Min(out.Y, r.Y)
		x1, y1 := style.Max(out.Right(), r.Right()), style.Max(out.Bottom(), r.Bottom())
		out = Rect{X: x0, Y: y0, W: x1.Sub(x0), H: y1.Sub(y0)}
	}
	return out
}

// filtersItsPaint reports whether a box has a filter, which makes it a stacking
// context whatever the filter is (Filter Effects 1 §5).
func filtersItsPaint(b *Box) bool {
	if b == nil || b.IsText() {
		return false
	}
	raw := ascii.TrimCSSSpace(b.Style.Get("filter"))
	return raw != "" && !ascii.EqualFold(raw, "none")
}

// maxFilterFunctions bounds the functions one filter may list. The chain is
// reduced to two before anything is drawn, so this bounds the reading of it
// and what a report about it can say; a filter past it is not applied, and
// says so.
//
// A variable so that a test can lower it.
var maxFilterFunctions = 256

// filterChain reads a box's filter as the chain the operation carries: one blur
// and one opacity at most, with what it could not apply listed.
func (l *layouter) filterChain(b *Box) (chain []FilterFunction, dropped []string, tooMany bool) {
	vals, _ := css.ParseComponentValues(b.Style.Get("filter"))
	var fns []css.ComponentValue
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		fns = append(fns, v)
	}
	if len(fns) > maxFilterFunctions {
		return nil, nil, true
	}
	variance, alpha := 0.0, 1.0
	for _, v := range fns {
		if v.IsToken() {
			// A url(), the only thing that is not a function. The cascade has
			// checked the rest of the grammar.
			dropped = append(dropped, "url()")
			continue
		}
		name := ascii.Lower(v.Token.Value)
		args := splitValueParts(v.Values)
		switch name {
		case "blur":
			if len(args) == 0 {
				continue
			}
			length, ok := l.lengthOfValues(b, args[0])
			if !ok || length.Kind != style.LengthAbsolute || length.Value < 0 {
				dropped = append(dropped, "blur()")
				continue
			}
			s := length.Value.Px()
			variance += s * s
		case "opacity":
			if len(args) == 0 {
				continue
			}
			a, ok := filterAmount(args[0])
			if !ok {
				dropped = append(dropped, "opacity()")
				continue
			}
			// "Values of amount over 100% are allowed but UAs must clamp the
			// values to 1."
			alpha *= math.Min(a, 1)
		default:
			dropped = append(dropped, name+"()")
		}
	}
	if variance > 0 {
		s, _ := style.FromPx(math.Sqrt(variance))
		if s > 0 {
			chain = append(chain, FilterFunction{Kind: FilterBlur, StdDev: s})
		}
	}
	if alpha < 1 {
		chain = append(chain, FilterFunction{Kind: FilterOpacity, Amount: alpha})
	}
	return chain, dropped, false
}

// filterAmount reads a <number> or a <percentage> as a number.
func filterAmount(part []css.ComponentValue) (float64, bool) {
	if len(part) != 1 || !part[0].IsToken() {
		return 0, false
	}
	t := part[0].Token
	switch t.Kind {
	case css.Number:
		return t.Number, t.Number >= 0
	case css.Percentage:
		return t.Number / 100, t.Number >= 0
	}
	return 0, false
}

// resolveFilters reads every filter in the document into the chains the painter
// wraps groups in, and reports what it cannot do: the functions it does not
// apply, a chain past the bound, and a positioned box the filter should have
// been the containing block of.
func (l *layouter) resolveFilters(root *Fragment) {
	if root == nil || root.Box == nil {
		return
	}
	var register func(b *Box)
	var walk func(b *Box, sincePositioned, any bool)
	walk = func(b *Box, sincePositioned, any bool) {
		// A box whose containing block a filter above it should have been.
		switch {
		case b.Position == PositionFixed && any,
			b.Position == PositionAbsolute && sincePositioned:
			l.rec.ReportDetail(Finding{
				Rule:   RulePositionApproximated,
				Source: AtHTML(offsetOf(b)),
				Message: "a box with a filter is the containing block of the positioned " +
					"boxes inside it, which this engine does not make it; this one was " +
					"positioned against the containing block it would have without the filter",
				Path:     PathOf(b.Element),
				Property: "filter",
			})
		}
		// The root's filter is applied like any other; what §5 exempts the
		// root from is only being the containing block.
		filtered := filtersItsPaint(b) && b.Parent != nil
		// And the inline boxes a block was lifted out of, which §9.2.1.1 left
		// out of the tree above it and which paint it all the same: a filtered
		// <span> around a <div> is a group holding the <div>, and may be in
		// the tree nowhere else.
		for _, from := range b.splitFrom {
			register(from)
		}
		register(b)
		positioned := b.Position.positioned()
		childSince := sincePositioned
		childAny := any || filtered
		for _, from := range b.splitFrom {
			if from.Position.positioned() {
				childSince = false
			} else if filtersItsPaint(from) {
				childSince, childAny = true, true
			}
		}
		if positioned {
			childSince = false
		}
		if filtered && !positioned {
			childSince = true
		}
		for _, c := range b.Children {
			walk(c, childSince, childAny)
		}
	}
	// One report per element, however many boxes §9.2.1.1 made of it.
	reported := map[groupKey]bool{}
	register = func(b *Box) {
		if !filtersItsPaint(b) {
			return
		}
		if _, ok := root.filters[b]; ok {
			return
		}
		chain, dropped, tooMany := l.filterChain(b)
		if root.filters == nil {
			root.filters = map[*Box][]FilterFunction{}
		}
		root.filters[b] = chain
		k := groupKey{b.Element, b.Pseudo}
		if reported[k] || (!tooMany && len(dropped) == 0) {
			return
		}
		reported[k] = true
		if tooMany {
			l.rec.ReportDetail(Finding{
				Rule:   RuleLimit,
				Source: AtHTML(offsetOf(b)),
				Message: fmt.Sprintf("a filter of more than %d functions was not applied",
					maxFilterFunctions),
				Path:     PathOf(b.Element),
				Property: "filter",
			})
			return
		}
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   AtHTML(offsetOf(b)),
			Message:  "the filter functions " + strings.Join(dropped, ", ") + " were not applied",
			Path:     PathOf(b.Element),
			Property: "filter",
		})
	}
	walk(root.Box, false, false)
}

// filtering paints what a filtered box paints and puts it in one FilterGroup,
// cut afterwards by the clip and the curves around the box.
//
// The links among what it painted are moved out ahead of the group: a link is
// an area and not ink, a filter does nothing to it, and gatherLinks reads the
// top of the list. They are cut by the same clip, since nothing inside the
// group was.
func (p *painter) filtering(b *Box, clip Clip, round *roundClip, paint func()) {
	at := len(p.ops)
	if clip.blocks() {
		return
	}
	paint()
	chain := p.filters[b]
	var marks, links []Op
	for _, op := range p.ops[at:] {
		if _, ok := op.(Link); ok {
			links = append(links, op)
			continue
		}
		marks = append(marks, op)
	}
	p.ops = p.ops[:at]
	if len(links) > 0 {
		p.ops = append(p.ops, links...)
		if clip.Active {
			p.ops = clipOps(p.ops, at, clip)
		}
	}
	if len(marks) > 0 {
		if len(chain) == 0 {
			// A filter that filters nothing — blur(0), opacity(1), or only
			// functions this engine does not apply, which were reported: what
			// is inside is what the page shows, cut as it would have been.
			from := len(p.ops)
			p.ops = append(p.ops, marks...)
			if clip.Active {
				p.ops = clipOps(p.ops, from, clip)
			}
		} else if !transparentChain(chain) {
			g := newFilterGroup(chain, marks)
			if clip.Active && !clip.admits(g.Extent()) {
				g.Clip = clip
			}
			if !clip.hides(g.Extent()) {
				p.emit(g)
			}
		}
	}
	p.rounding(at, round)
}

// transparentChain reports whether a chain leaves nothing of what it filters:
// an opacity of nothing.
func transparentChain(chain []FilterFunction) bool {
	for _, f := range chain {
		if f.Kind == FilterOpacity && f.Amount <= 0 {
			return true
		}
	}
	return false
}

// filteredLevel paints a filtered inline box's level as one group.
//
// What the level holds was cut by the clips of the blocks it is painted from
// before the group was formed, which is the order §5 reverses: a blur spreads
// past a clip it should have been cut by, and stops at one it should have run
// past. Where that could show — the chain blurs and something the level holds
// is clipped — it is reported.
func (p *painter) filteredLevel(l *inlineLevel) {
	blurs := false
	for _, f := range p.filters[l.box] {
		blurs = blurs || f.Kind == FilterBlur
	}
	if blurs {
		for _, pt := range l.parts {
			c, r := pt.frag.clipSelf, pt.frag.roundSelf
			if pt.lines {
				c, r = pt.frag.clipContent, pt.frag.roundContent
			}
			if c.Active || r != nil {
				p.reportOnce(l.box, "filtered-inline-clipped", Finding{
					Rule:   RuleUnsupportedValue,
					Source: AtHTML(offsetOf(l.box)),
					Message: "an inline box with a blur inside a box that clips was cut " +
						"before it was blurred rather than after",
					Path:     PathOf(l.box.Element),
					Property: "filter",
				})
				break
			}
		}
	}
	p.filtering(l.box, Clip{}, nil, func() { p.paintLevel(l) })
}

// withOpacity is a chain with an alpha folded into its opacity, which is where
// an opacity around the group goes: after everything the chain does, and
// multiplied into an opacity it already ends with.
func withOpacity(chain []FilterFunction, alpha float64) []FilterFunction {
	out := append([]FilterFunction(nil), chain...)
	if n := len(out); n > 0 && out[n-1].Kind == FilterOpacity {
		out[n-1].Amount *= alpha
		return out
	}
	if alpha >= 1 {
		return out
	}
	return append(out, FilterFunction{Kind: FilterOpacity, Amount: alpha})
}
