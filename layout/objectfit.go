package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// Fitting a replaced element's content into the box it was given.
//
// The box and the picture in it are sized by two different rules and CSS Images
// 3 §5.5 is the second. Everything upstream — the intrinsic sizing of §10.3.2,
// a declared width and height, a flex item stretched by its line — decides the
// content box; object-fit decides what the picture does with it when the two
// are not the same shape.
//
// The default is that it is stretched, which is what this engine did and always
// said it did: DrawImage's rectangle is where the picture goes and a backend
// scales to it. That is right for "fill" and wrong for every other value, and a
// photograph stretched to a box that is not its shape is the ordinary, visible
// failure — a portrait squashed into a square thumbnail.
//
// # Why it is here and not in layout
//
// Because it changes nothing about the layout. The box is where it was, the
// space it takes is the same, and the text around it does not move: what
// changes is the rectangle one operation is drawn into and whether it is
// clipped. So this runs at paint, reads the fragment's own content rect, and
// hands back a rectangle and a clip.

// objectFit is what §5.5 asks for.
type objectFit uint8

const (
	// objectFill stretches the content to the box, ignoring its own shape. It is
	// the initial value, and it is what a replaced element did before this file.
	objectFill objectFit = iota
	// objectContain is the largest rectangle of the content's own shape that fits
	// inside the box.
	objectContain
	// objectCover is the smallest rectangle of the content's own shape that covers
	// the box, which overflows it and is clipped to it.
	objectCover
	// objectNone is the content's own size, neither grown nor shrunk.
	objectNone
	// objectScaleDown is whichever of objectNone and objectContain is smaller, which is
	// "shrink to fit but never enlarge".
	objectScaleDown
)

// objectFitOf reads the property.
//
// An unknown value is the initial one, which is what a declaration this engine
// does not read has to mean. It is reported where it is read rather than here,
// because a value is worth one finding and not one per box that has it.
func objectFitOf(value string) (objectFit, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "fill":
		return objectFill, true
	case "contain":
		return objectContain, true
	case "cover":
		return objectCover, true
	case "none":
		return objectNone, true
	case "scale-down":
		return objectScaleDown, true
	}
	return objectFill, false
}

// fitContent places a replaced element's content inside its content box.
//
// box is the content box, and natural is the content's own size — zero in
// either dimension when it does not state one. The returned rectangle is where
// the content is drawn, and the clip is the box: replaced content is cut to the
// element's box whatever its overflow says, which is what "cover" needs and
// what "none" needs when the picture is larger than the box it was put in.
//
// The clip is returned for every fit but "fill" rather than only when the
// content reaches outside, because the display list makes that distinction for
// itself: an operation wholly inside its clip carries none by the time it is
// emitted, so a clip that cuts nothing and no clip at all are the same list.
// Deciding it here as well would be a second answer to a settled question, and
// one no test could tell from the first.
//
// The content is centred in the box, which is object-position's initial value
// of "50% 50%" and the only placement this engine reads. A half-pixel is not
// rounded away: the layout unit is finer than a pixel and a backend that wants
// a whole one is the backend that should round.
func fitContent(box Rect, natural Size, fit objectFit) (Rect, Clip) {
	if fit == objectFill {
		return box, Clip{}
	}
	if natural.W <= 0 || natural.H <= 0 {
		// Nothing to preserve the shape of. §5.5 sizes the content with the
		// default sizing algorithm, whose answer with no intrinsic dimensions
		// and no ratio is the box itself — which is what "fill" already does.
		return box, Clip{}
	}

	size := natural
	switch fit {
	case objectContain, objectScaleDown:
		size = scaleToFit(natural, box, false)
		if fit == objectScaleDown && natural.W <= size.W {
			// "scale-down" is the smaller of the content's own size and the
			// contained one, and comparing one dimension is enough: both are
			// the same shape, so one is smaller in both or in neither.
			size = natural
		}
	case objectCover:
		size = scaleToFit(natural, box, true)
	}

	out := Rect{
		X: box.X.Add(box.W.Sub(size.W).Div(2)),
		Y: box.Y.Add(box.H.Sub(size.H).Div(2)),
		W: size.W, H: size.H,
	}
	return out, Clip{Rect: box, Active: true}
}

// scaleToFit scales a size to a box, keeping its shape: to the smaller of the
// two ratios so that it fits inside, or to the larger so that it covers.
func scaleToFit(natural Size, box Rect, cover bool) Size {
	byWidth := box.W.Px() / natural.W.Px()
	byHeight := box.H.Px() / natural.H.Px()
	factor := min(byWidth, byHeight)
	if cover {
		factor = max(byWidth, byHeight)
	}
	return Size{W: natural.W.Mul(factor), H: natural.H.Mul(factor)}
}

// naturalSizeOf is the size a replaced element's content states for itself.
//
// A picture that states only a ratio still has a shape to preserve, and §5.5
// works from the ratio rather than from the dimensions — so a ratio with no
// dimensions is turned into a size of the right shape here, and only the shape
// of it is ever used.
func naturalSizeOf(r *ReplacedContent) Size {
	if r == nil {
		return Size{}
	}
	if r.Width > 0 && r.Height > 0 {
		return Size{W: r.Width, H: r.Height}
	}
	if r.Ratio > 0 {
		// One unit tall and Ratio wide is the same shape as anything else with
		// that ratio, and every use of this scales it.
		u, _ := style.FromPx(1)
		return Size{W: u.Mul(r.Ratio), H: u}
	}
	return Size{}
}

// checkObjectFit reports an object-fit this engine cannot read.
//
// It is raised in layout, where every other finding about a box is raised, even
// though the property is acted on at paint: the painter has no recorder, and
// giving it one to say this would be the wrong trade — the value is knowable
// from the style alone and needs none of the geometry paint has.
//
// The report is once per value rather than once per box: a stylesheet saying
// "object-fit: fit" on a rule that matches forty pictures has made one mistake.
func (l *layouter) checkObjectFit(b *Box) {
	raw := b.Style["object-fit"]
	if _, ok := objectFitOf(raw); ok {
		return
	}
	l.reportOnce("object-fit:"+raw, Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "the value " + quoteValue(strings.TrimSpace(raw)) + " of object-fit" +
			" is not one this engine reads; the content was stretched to its box",
		Path:     PathOf(b.Element),
		Property: "object-fit",
	})
}
