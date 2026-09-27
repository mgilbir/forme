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
// Every filter function, in the order written, the first taking the element
// as drawn and each the one before's result (§5). blur() and opacity() are
// linear and commute, and two blurs are one — a Gaussian of deviation a after
// one of deviation b is one of deviation √(a² + b²) — so a run of them is one
// blur followed by one opacity, and a group carries them. The seven colour
// functions are each a colour matrix, and drop-shadow() a shadow of the group:
// see filtercolour.go for when each is folded into the marks exactly and when
// it is left in the group for the backend.
//
// A url() reference to an SVG filter is reported where it is declared, and the
// element is drawn with the rest of its chain. The element is still a stacking
// context, which §5 makes it whatever the filter is.
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
	// FilterColorMatrix is one of the seven colour functions (brightness(),
	// contrast(), grayscale(), hue-rotate(), invert(), saturate(), sepia()):
	// each pixel's non-premultiplied, gamma-encoded sRGB colour and alpha, as
	// fractions, times Matrix, and the result clamped to [0, 1] (Filter
	// Effects 1 §9.1). The engine emits only matrices whose last row keeps the
	// alpha as it was, so a colour matrix never changes where a group has ink.
	// It is left in a group only where folding it into the colours of what the
	// group holds would not be exact, or would cost more than the document's
	// allowance for it; see filtercolour.go and filterPass.
	FilterColorMatrix
	// FilterDropShadow is drop-shadow(): the group's alpha blurred by StdDev,
	// moved by Offset, flooded with Color and composited under the group
	// (Filter Effects 1 §13.1.10). It is left in a group only over what the
	// engine cannot make a shadow of as marks, a picture, or past the
	// allowance filterPass keeps; see filtercolour.go.
	FilterDropShadow
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
	// Matrix is a colour matrix's 4 rows of 5, row by row: r', g', b' and a'
	// from r, g, b, a and 1.
	Matrix [20]float64
	// Offset and Color are a drop shadow's; its StdDev is StdDev above, and
	// zero is a sharp shadow.
	Offset Point
	Color  style.RGBA
}

// FilterGroup paints its operations as one group, filters the group, and
// composites the result onto the page.
//
// Filters are applied in order, the first to the group as painted. A run of
// blurs and opacities is at most one blur and then one opacity; a chain that
// filters nothing is not a group at all. A PDF backend draws it as a
// transparency group: an opacity is the group's constant alpha, and PDF has no
// Gaussian blur, so a blur is rasterised by the backend — the group drawn at a
// resolution of its choosing and convolved with this deviation — or refused and
// reported. A colour matrix and a drop shadow are in a group only where the
// engine did not apply them to the marks (see filtercolour.go), and a backend
// rasterises such a group as it does a blur — the group drawn, then each
// sample's colour mapped or its alpha shadowed — or refuses it and reports.
// Extent says how far the filtered group reaches.
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

// spreads reports whether a chain moves ink past where it was drawn: a blur
// or a drop shadow.
func spreads(chain []FilterFunction) bool {
	for _, f := range chain {
		if f.Kind == FilterBlur || f.Kind == FilterDropShadow {
			return true
		}
	}
	return false
}

// Extent is where the filtered group may put ink: what its operations mark,
// taken through the chain in order — grown by three standard deviations of a
// blur, and joined by itself moved by a drop shadow's offset and grown by three
// of its deviations — and cut by its clip. Past three deviations a blur's ink
// is under half an 8-bit step. An opacity and a colour matrix leave it where it
// is.
func (g FilterGroup) Extent() Rect {
	out := g.ink
	if !g.inkKnown {
		out = opsInk(g.Ops)
	}
	if out.Empty() {
		return Rect{}
	}
	grow := func(r Rect, s style.Unit) Rect {
		if s <= 0 {
			return r
		}
		d := s.Mul(blurReach)
		return r.Outset(Edges{Top: d, Right: d, Bottom: d, Left: d})
	}
	for _, f := range g.Filters {
		switch f.Kind {
		case FilterBlur:
			out = grow(out, f.StdDev)
		case FilterDropShadow:
			shadow := grow(Rect{X: out.X.Add(f.Offset.X), Y: out.Y.Add(f.Offset.Y), W: out.W, H: out.H}, f.StdDev)
			out = unionRect(out, shadow)
		}
	}
	if g.Clip.Active {
		out = out.Intersect(g.Clip.Rect)
	}
	return out
}

// unionRect is the smallest rectangle holding two.
func unionRect(a, b Rect) Rect {
	x0, y0 := style.Min(a.X, b.X), style.Min(a.Y, b.Y)
	x1, y1 := style.Max(a.Right(), b.Right()), style.Max(a.Bottom(), b.Bottom())
	return Rect{X: x0, Y: y0, W: x1.Sub(x0), H: y1.Sub(y0)}
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

// maxFilterFunctions bounds the functions one filter may list. Each is a group
// or a pass over what the group holds, so this bounds the work a filter asks
// of the painter and what a report about it can say; a filter past it is not
// applied, and says so.
//
// A variable so that a test can lower it.
var maxFilterFunctions = 256

// filterChain reads a box's filter as the chain the painter applies, in order:
// runs of blurs and opacities each made one blur and one opacity, the seven
// colour functions as colour matrices, and drop shadows with their lengths and
// colour resolved. A function that changes nothing is left out, and what could
// not be applied is listed.
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
	// The run of blurs and opacities being gathered.
	variance, alpha := 0.0, 1.0
	flush := func() {
		if variance > 0 {
			s, _ := style.FromPx(math.Sqrt(variance))
			if s > 0 {
				chain = append(chain, FilterFunction{Kind: FilterBlur, StdDev: s})
			}
		}
		if alpha < 1 {
			chain = append(chain, FilterFunction{Kind: FilterOpacity, Amount: alpha})
		}
		variance, alpha = 0, 1
	}
	matrix := func(m [20]float64) {
		if m == identityMatrix {
			return
		}
		flush()
		chain = append(chain, FilterFunction{Kind: FilterColorMatrix, Matrix: m})
	}
	for _, v := range fns {
		if v.IsToken() {
			// A url(), the only thing that is not a function. The cascade has
			// checked the rest of the grammar.
			dropped = append(dropped, "url()")
			continue
		}
		name := ascii.Lower(v.Token.Value)
		args := splitValueParts(v.Values)
		// §6.1: an omitted amount is 1, except for an angle, which is 0.
		amount := func(clamp bool) (float64, bool) {
			if len(args) == 0 {
				return 1, true
			}
			a, ok := filterAmount(args[0])
			if !ok {
				return 0, false
			}
			// "Values of amount over 100% are allowed but UAs must clamp
			// the values to 1", for the four whose amount is a proportion.
			if clamp {
				a = math.Min(a, 1)
			}
			return a, true
		}
		switch name {
		case "blur":
			if len(args) == 0 {
				continue
			}
			length, ok := l.lengthOfValues(b, args[0])
			if !ok || length.Kind != style.LengthAbsolute {
				dropped = append(dropped, "blur()")
				continue
			}
			// A negative length can only be a calc(), whose range CSS Values
			// 4 enforces by clamping.
			s := math.Max(0, length.Value.Px())
			variance += s * s
		case "opacity", "grayscale", "sepia", "invert", "brightness", "contrast", "saturate":
			clamp := name == "opacity" || name == "grayscale" || name == "sepia" || name == "invert"
			a, ok := amount(clamp)
			if !ok {
				dropped = append(dropped, name+"()")
				continue
			}
			switch name {
			case "opacity":
				alpha *= a
			case "grayscale":
				matrix(grayscaleMatrix(a))
			case "sepia":
				matrix(sepiaMatrix(a))
			case "invert":
				matrix(invertMatrix(a))
			case "brightness":
				matrix(brightnessMatrix(a))
			case "contrast":
				matrix(contrastMatrix(a))
			case "saturate":
				matrix(saturateMatrix(a))
			}
		case "hue-rotate":
			deg := 0.0
			if len(args) > 0 {
				var ok bool
				if deg, ok = filterAngle(args[0]); !ok {
					dropped = append(dropped, "hue-rotate()")
					continue
				}
			}
			matrix(hueRotateMatrix(deg))
		case "drop-shadow":
			f, ok := l.dropShadow(b, args)
			if !ok {
				dropped = append(dropped, "drop-shadow()")
				continue
			}
			if f.Color.A <= 0 {
				// A transparent shadow: flooded with nothing, it adds nothing
				// under the group, and changes nothing.
				continue
			}
			flush()
			chain = append(chain, f)
		default:
			dropped = append(dropped, name+"()")
		}
	}
	flush()
	return chain, dropped, false
}

// filterAngle reads hue-rotate()'s argument: an <angle>, a calc() of one, or
// <zero>, in degrees.
func filterAngle(part []css.ComponentValue) (float64, bool) {
	if len(part) == 1 && part[0].IsToken() && part[0].Token.Kind == css.Number {
		return 0, part[0].Token.Number == 0
	}
	return style.ParseAngle(part)
}

// dropShadow reads drop-shadow()'s arguments, "<color>? && <length>{2,3}": the
// offset, and the third length as the deviation itself (§6.1: "the optional
// 3rd <length> value being the standard deviation instead of blur radius").
// A colour left out is the box's colour.
func (l *layouter) dropShadow(b *Box, args [][]css.ComponentValue) (FilterFunction, bool) {
	f := FilterFunction{Kind: FilterDropShadow}
	colour, haveColour := style.RGBA{}, false
	var lengths []style.Unit
	for _, a := range args {
		if c, ok := l.gradientColour(b, a); ok && !haveColour {
			colour, haveColour = c, true
			continue
		}
		length, ok := l.lengthOfValues(b, a)
		if !ok || length.Kind != style.LengthAbsolute {
			return FilterFunction{}, false
		}
		lengths = append(lengths, length.Value)
	}
	if len(lengths) < 2 || len(lengths) > 3 {
		return FilterFunction{}, false
	}
	if !haveColour {
		c, ok := parseColorValue(b.Style.Get("color"))
		if !ok {
			c = style.RGBA{A: 1}
		}
		colour = c
	}
	f.Offset = Point{X: lengths[0], Y: lengths[1]}
	if len(lengths) == 3 {
		f.StdDev = style.Max(0, lengths[2])
	}
	f.Color = colour
	return f, true
}

// filterAmount reads a <number> or a <percentage>, or a calc() of either, as a
// number. The cascade refuses a negative one written out; one a calc() comes to
// is clamped to nothing, as CSS Values 4 enforces a math function's range.
func filterAmount(part []css.ComponentValue) (float64, bool) {
	v, ok := style.ParseNumberPercentage(part)
	return math.Max(0, v), ok
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
//
// d is what the box's own marks were dimmed by: an opacity around the box, or
// its own, is folded into each mark as it is painted (see opacity.go), so the
// marks the filter is given carry it already. A colour matrix is indifferent
// to that — it reads no alpha — and a shadow of dimmed marks is where the
// shadow of the group would be, dimmed. Where the shadow lies over the marks,
// or over itself, the two are not the same, which is the question opacity asks
// of every group, so the shadow's marks are handed to the group that dimmed
// them and are checked with the rest.
func (p *painter) filtering(b *Box, d dim, clip Clip, round *roundClip, paint func()) {
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
	if len(marks) > 0 && !transparentChain(chain) {
		// What the filter makes of the marks: the marks themselves where it
		// filters nothing — blur(0), opacity(1), or only what this engine
		// does not apply, which was reported — and otherwise its groups and
		// the marks it recoloured and cast shadows of. It is cut afterwards,
		// as the page shows it.
		//
		// The groups it made are charged as emit charges a mark, and the
		// shadows' marks before they are made; the marks themselves were
		// charged when they were painted.
		out, groups, cast := p.applyFilters(chain, marks)
		if groups > 0 && !p.rec.chargeMark(int64(groups)*costOp, "the marks past that point") {
			out, cast = nil, nil
		}
		if d.dimmed() && len(cast) > 0 {
			_, shadows := dimOps(cast, 0, 1)
			g := p.groups[d.owners.box]
			g.marks = append(g.marks, shadows...)
		}
		if len(out) == 1 {
			if g, ok := out[0].(FilterGroup); ok {
				if clip.Active && !clip.admits(g.Extent()) {
					g.Clip = clip
				}
				if !clip.hides(g.Extent()) {
					p.ops = append(p.ops, g)
				}
				out = nil
			}
		}
		if len(out) > 0 {
			from := len(p.ops)
			p.ops = append(p.ops, out...)
			if clip.Active {
				p.ops = clipOps(p.ops, from, clip)
			}
		}
	}
	p.rounding(at, round)
}

// applyFilters is a chain applied to marks, in order: a colour matrix folded
// into their colours and a drop shadow cast as marks where that is exact (see
// filtercolour.go), and everything else — and those, where it is not, or where
// filterPass will not pay for them — a group around what came before. groups is
// how many groups it made, and cast is the shadows it cast as marks.
func (p *painter) applyFilters(chain []FilterFunction, ops []Op) (out []Op, groups int, cast []Op) {
	// own is set while ops is one group this made, to which the next
	// function is added rather than a group wrapped round it: a group applies
	// its chain in order, which is what nesting would.
	own := false
	wrap := func(f FilterFunction) {
		if own {
			g := ops[0].(FilterGroup)
			g.Filters = appendFilter(g.Filters, f)
			ops[0] = g
			return
		}
		ops = []Op{newFilterGroup([]FilterFunction{f}, ops)}
		own = true
		groups++
	}
	for _, f := range chain {
		switch f.Kind {
		case FilterColorMatrix:
			if _, ok := p.filterPass(ops); ok {
				if folded, ok := foldMatrix(ops, f.Matrix); ok {
					ops = folded
					continue
				}
			}
			wrap(f)
		case FilterDropShadow:
			if !shadowable(ops) {
				wrap(f)
				continue
			}
			n, ok := p.filterPass(ops)
			if !ok {
				wrap(f)
				continue
			}
			// The shadow is as many operations again as what it is a
			// shadow of, at most, and the group that blurs or fades it, and
			// they are charged before they are made. Refused, the shadow is
			// not drawn, and the budget says so.
			if !p.rec.chargeMark((n+1)*costOp, "the marks past that point") {
				continue
			}
			shadow := shadowOps(ops, f)
			cast = append(cast, shadow...)
			ops = append(shadow, ops...)
			own = false
		default:
			wrap(f)
		}
	}
	return ops, groups, cast
}

// filterRewriteFloor and filterRewritesPerMark are the allowance filterPass
// pays from: a floor, and so many operations for each mark the document has
// painted. A variable so that a test can lower it.
var filterRewriteFloor int64 = 4096

const filterRewritesPerMark = 8

// filterPass asks to pass over ops once for a filter — to fold a colour matrix
// into their colours or to cast a shadow of them — and says how many
// operations that is, and whether it may be done.
//
// Each such pass reads every operation the filtered box holds, its
// descendants' included, and a filtered box inside a filtered box is read
// again by every function of every filter around it: boxes nested n deep, each
// filtered by k functions, would be read k·n²/2 times. The parser bounds n and
// maxFilterFunctions bounds k, and what they leave is still tens of thousands
// of passes over each mark. So the passes are paid from an allowance that
// grows with what the document paints, filterRewritesPerMark operations for
// each mark beyond filterRewriteFloor, which keeps the whole of it linear in
// the marks.
// Past it, a matrix or a shadow is left in its group for the backend to apply
// — what it is over a picture, and exactly the filter still — and the
// refusal is reported, once, as what was not done.
//
// Counting stops at what is left, so a refused pass costs no more than the
// allowance it was refused by, and every pass after it next to nothing.
func (p *painter) filterPass(ops []Op) (int64, bool) {
	left := satAdd(filterRewriteFloor, satMul(p.painted, filterRewritesPerMark)) - p.filterPasses
	n, within := countOpsUpTo(ops, max(left, 0))
	p.filterPasses = satAdd(p.filterPasses, n)
	if !within {
		p.rec.refuse("the filters folded into the marks past that point, which were left in their groups")
		return n, false
	}
	return n, true
}

// countOpsUpTo is how many operations a list holds, those inside groups
// included, and whether that is at most limit. It stops counting past the
// limit.
func countOpsUpTo(ops []Op, limit int64) (int64, bool) {
	var n int64
	var count func(ops []Op) bool
	count = func(ops []Op) bool {
		for _, op := range ops {
			if n++; n > limit {
				return false
			}
			switch v := op.(type) {
			case ClipPath:
				if !count(v.Ops) {
					return false
				}
			case FilterGroup:
				if !count(v.Ops) {
					return false
				}
			}
		}
		return true
	}
	ok := count(ops)
	return n, ok
}

// appendFilter adds a function to the end of a group's chain, merging a blur
// or an opacity into the run of them it ends: blurs by their variances,
// opacities by their product, the blur ahead of the opacity it commutes with.
func appendFilter(chain []FilterFunction, f FilterFunction) []FilterFunction {
	out := append([]FilterFunction(nil), chain...)
	n := len(out)
	switch f.Kind {
	case FilterOpacity:
		if n > 0 && out[n-1].Kind == FilterOpacity {
			out[n-1].Amount *= f.Amount
			return out
		}
	case FilterBlur:
		i := n
		for i > 0 && out[i-1].Kind == FilterOpacity {
			i--
		}
		if i > 0 && out[i-1].Kind == FilterBlur {
			a, b := out[i-1].StdDev.Px(), f.StdDev.Px()
			out[i-1].StdDev, _ = style.FromPx(math.Sqrt(a*a + b*b))
			return out
		}
		out = append(out, FilterFunction{})
		copy(out[i+1:], out[i:])
		out[i] = f
		return out
	}
	return append(out, f)
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
	if spreads(p.filters[l.box]) {
		for _, pt := range l.parts {
			c, r := pt.frag.clipSelf, pt.frag.roundSelf
			if pt.lines {
				c, r = pt.frag.clipContent, pt.frag.roundContent
			}
			if c.Active || r != nil {
				p.reportOnce(l.box, "filtered-inline-clipped", Finding{
					Rule:   RuleUnsupportedValue,
					Source: AtHTML(offsetOf(l.box)),
					Message: "an inline box with a blur or a drop shadow inside a box that " +
						"clips was cut before it was filtered rather than after",
					Path:     PathOf(l.box.Element),
					Property: "filter",
				})
				break
			}
		}
	}
	p.filtering(l.box, p.inlineDims[l.box].dim, Clip{}, nil, func() { p.paintLevel(l) })
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
