package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS Color 4 §3.1: opacity.
//
// # What the property asks for, and what this engine does
//
// "opacity: 0.5" is not "paint everything in here half-transparent". It is
// "paint everything in here, as a group, onto a surface of its own, and then
// blend that surface into the page at half strength". The difference shows the
// moment two marks in the group overlap: as a group, the upper mark hides the
// lower one and only the result is dimmed; mark by mark, the lower one shows
// through the upper. A green square under a blue square is blue in the first
// reading and a blend of the two in the second.
//
// This engine does the second, and applies the alpha to each mark. The reason is
// the one visualeffects.go gives for clipping, and it is the same reason twice:
//
//	a clip travels *on* the operation it applies to, and nothing in the
//	display list has any state that spans two operations
//
// A group is a push/pop pair by nature — "begin compositing here, end it there"
// — and an unbalanced pair in a content stream does not lose a box, it changes
// how the rest of the page is drawn. The list is built so that no operation can
// do that to another, and a group operation would be the one exception. So the
// alpha is folded into the colour of every mark the group paints, which is a
// rewrite of a span of the list exactly like the one a clip is (see clipping).
//
// # Where the two readings differ, the engine says so
//
// Folding is not always an approximation. A group that paints one mark is
// *exactly* the group: there is nothing for the mark to hide, so dimming it and
// dimming the surface it would have been alone on give the same colour. A group
// at "opacity: 0" is exact too, whatever it paints, because everything in it is
// dropped. Those two cases are most of what documents write — a translucent
// panel, a faded caption, an element being hidden — and each of them comes out
// right.
//
// The rest are reported, per box, saying which of the two it is. That is the
// same shape as the writing-mode report: a property this engine applies to some
// boxes and approximates on others is a fact about the box and not about the
// declaration, so a table keyed by property has no way to say it and the finding
// is raised where the box is painted. See writingmode.go, whose report moved out
// of style/unimplemented.go for exactly this reason — as this one just has.

// opacityOf is the fraction of a box's own paint that reaches the page.
//
// Anything that is not a number or a percentage is 1, which is the initial
// value: a declaration this engine cannot read is a declaration it has not
// applied, and the value that stands for "not applied" is the one that changes
// nothing. Out-of-range values are clamped rather than rejected, because §3.1
// says to clamp them — "opacity: 2" is opaque and "opacity: -1" is invisible.
func opacityOf(cs style.ComputedStyle) float64 {
	raw := strings.TrimSpace(cs["opacity"])
	if raw == "" {
		return 1
	}
	scale := 1.0
	if pct, ok := strings.CutSuffix(raw, "%"); ok {
		raw, scale = strings.TrimSpace(pct), 100
	}
	n, ok := parseNumber(raw)
	if !ok {
		return 1
	}
	switch n /= scale; {
	case n <= 0:
		return 0
	case n >= 1:
		return 1
	}
	return n
}

// groupsItsPaint reports whether a box's own opacity makes a group of it.
func groupsItsPaint(b *Box) bool { return b != nil && opacityOf(b.Style) < 1 }

// groupMark is one mark a group painted, in the terms the faithfulness question
// needs: where it is, and whether the fold could be done to it at all.
type groupMark struct {
	// rect is where the mark is, when the display list states it. A text run
	// does not state one — a DrawText carries a pen position and the glyphs are
	// measured from the face — so a run's rect is empty and the check below
	// treats it as able to overlap anything.
	rect  Rect
	text  bool
	image bool
}

// group is one box that asked for opacity, together with what it painted.
type group struct {
	box   *Box
	alpha float64
	marks []groupMark
}

// dimOps folds an alpha into every mark from at onwards, and says what it had to
// work with.
//
// A picture is the one mark that cannot take an alpha: a DrawImage and a
// TileImage carry pixels rather than a colour, and there is nothing in either to
// multiply. At an alpha of zero there is still an exact answer — the picture is
// not painted — and that is the one case where dropping an operation is right
// rather than a way of hiding one that could not be handled.
func dimOps(ops []Op, at int, alpha float64) ([]Op, []groupMark) {
	marks := make([]groupMark, 0, len(ops)-at)
	kept := ops[:at]
	for _, op := range ops[at:] {
		switch v := op.(type) {
		case FillRect:
			if v.Rect.Empty() || v.Color.A == 0 {
				kept = append(kept, op)
				continue
			}
			marks = append(marks, groupMark{rect: v.Rect})
			if alpha == 0 {
				continue
			}
			v.Color.A *= alpha
			kept = append(kept, v)
		case DrawText:
			if v.Color.A == 0 || v.Text == "" {
				kept = append(kept, op)
				continue
			}
			marks = append(marks, groupMark{text: true})
			if alpha == 0 {
				continue
			}
			v.Color.A *= alpha
			kept = append(kept, v)
		case DrawImage:
			marks = append(marks, groupMark{rect: v.Rect, image: true})
			if alpha == 0 {
				continue
			}
			kept = append(kept, op)
		case TileImage:
			marks = append(marks, groupMark{rect: v.Clip, image: true})
			if alpha == 0 {
				continue
			}
			kept = append(kept, op)
		default:
			kept = append(kept, op)
		}
	}
	return kept, marks
}

// unfaithful is why folding the alpha into a group's marks is not the group, or
// the empty string when it is.
//
// Overlap is the whole rule, and one mark falls out of it rather than being a
// case of its own: a group of one has no pair to test, so the loop below says
// nothing about it — which is right, because there is nothing under that mark
// to show through and dimming it is dimming the surface it was alone on. A
// group of none is the same. Both were once written out above the loop as an
// early return, and neither could be made to fail: the loop already had them.
//
// The order of the tests is the order an author needs to hear them in. A picture
// in the group is the one the engine could do nothing at all about, so it is
// said first even where the group overlaps as well: "the picture is opaque" is
// what they will see, and "the marks blend into each other" would send them
// looking at the wrong thing.
func (g group) unfaithful() string {
	if g.alpha == 0 {
		// Nothing was painted, which is what the group would have come to.
		return ""
	}
	for _, m := range g.marks {
		if m.image {
			return "a picture carries its own pixels and there is no colour in " +
				"it to dim, so it is painted as though the box were opaque"
		}
	}
	for i, a := range g.marks {
		for _, b := range g.marks[i+1:] {
			if a.text || b.text || !a.rect.Intersect(b.rect).Empty() {
				return "the box paints marks that lie over each other, and each " +
					"is dimmed on its own, so the lower one shows through the " +
					"upper instead of being hidden by it"
			}
		}
	}
	return ""
}

// report says what a group could not express, at the box that asked for it.
func (g group) report(rec *Recorder) {
	why := g.unfaithful()
	if why == "" || rec == nil {
		return
	}
	rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(g.box)),
		Message: "\"opacity: " + strings.TrimSpace(g.box.Style["opacity"]) +
			"\" was applied to each mark this box paints rather than to the box " +
			"as a group, because " + why,
		Path:     PathOf(g.box.Element),
		Property: "opacity",
	})
}
