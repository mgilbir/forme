package layout

import (
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// CSS Sizing 4 §4.1's aspect-ratio: a preferred ratio between a box's two axes,
// so that one of them can be worked out from the other rather than from the
// content.
//
// It is the shape a document reaches for wherever a box has to keep its
// proportions with nothing inside it to give them — a placeholder, a map, a
// chart area, the sixteen-by-nine frame a video would have gone in. Before
// this, such a box was as tall as its content, which for an empty one is
// nothing at all.
//
// # What this slice does, and what it reports
//
// The height from the width, which is the direction documents are written in:
// the width comes from the containing block and the height follows. A box whose
// height is declared and whose width is auto is the other direction, and that
// one is *not* done — a block's auto width fills its containing block by §10.3.3
// rather than being shrunk to a ratio, and changing that reaches into the width
// arithmetic rather than sitting after it.
//
// So the second direction is reported rather than half-applied, on the model of
// every other narrowing here: the page a document gets is the one it would have
// had, and the author is told which half of their declaration did nothing.
//
// For a non-replaced box, that is all. The ratio does not take part in
// intrinsic sizing either — a float or a table cell shrunk to fit around such a
// box measures its content, not its ratio — and that needs no report of its
// own, because the one case where CSS Sizing 4 has the ratio answer an
// intrinsic width is the same case: a box whose width is auto and whose height
// is not, which is reported when that box is laid out, as every box is.
//
// # Replaced elements
//
// A replaced element is sized from a ratio already — its picture's — and §5.1
// makes the property the ratio that sizing uses: "<ratio>" replaces the natural
// one, and "auto && <ratio>" keeps the natural one where there is one. So
// replaced.go asks preferredRatio for the ratio instead of taking the content's,
// and every case of CSS 2.1's table then transfers a size through it, in both
// directions, with the limits of §10.4. It was never read there at all, so the
// ubiquitous "width: 100%; aspect-ratio: 16 / 9; object-fit: cover" drew the
// picture's own shape (audit C80).
//
// # Which box the ratio is of
//
// §5.1: a "<ratio>" relates the dimensions of the box box-sizing names, and
// "auto && <ratio>" — and a natural ratio — relates the content box's, always.
// Under "box-sizing: border-box" the two differ by the padding and the border,
// and a ratio of one on a box a hundred wide with forty pixels of padding either
// side is a box a hundred tall, not twenty: preferredRatio says by how much the
// ratio's box is larger than the content box, and the arithmetic adds that on
// before it transfers and takes it off after. It was applied to the content box
// whatever box-sizing said.

// aspectRatioOf reads the property into the ratio of width to height, for a
// box that has no ratio of its own.
//
// The grammar is "auto || <ratio>", and a <ratio> is one number or two with a
// solidus between them. "auto" alone is no ratio at all; "auto" beside one is
// the replaced-element form, where the element's own ratio wins and the
// declared one is the fallback — preferredRatio makes that choice for a box
// that has a ratio of its own, and for one that has none the declared one is
// what is left.
func aspectRatioOf(raw string) (float64, bool) {
	ratio, _, ok := aspectRatioParts(raw)
	return ratio, ok
}

// aspectRatioParts is aspectRatioOf and whether the value also says "auto",
// which decides which box the ratio is of and whether a natural ratio wins.
func aspectRatioParts(raw string) (ratio float64, auto, ok bool) {
	value := ascii.TrimCSSSpace(ascii.Lower(raw))
	if value == "" || value == "auto" {
		return 0, value == "auto", false
	}
	// The keyword may sit either side of the ratio. Taking it out leaves the
	// ratio to be read on its own.
	fields := ascii.CSSFields(strings.ReplaceAll(value, "/", " / "))
	var parts []string
	for _, f := range fields {
		if f == "auto" {
			auto = true
			continue
		}
		parts = append(parts, f)
	}
	switch len(parts) {
	case 1:
		ratio, ok = positiveRatio(parts[0], "1")
	case 3:
		if parts[1] == "/" {
			ratio, ok = positiveRatio(parts[0], parts[2])
		}
	}
	return ratio, auto, ok
}

// preferredRatio is the ratio a box is sized by, CSS Sizing 4 §5.1, and how
// much larger than its content box the box the ratio is of is.
//
// natural is the ratio the box's content has, zero for none — a picture's, for
// a replaced element, and nothing for anything else. declared says the ratio is
// the property's rather than the content's, which is what decides for a
// replaced element whether its natural sizes still stand where both of its
// dimensions are auto: see replacedSize.
func (l *layouter) preferredRatio(b *Box, natural float64,
	containing style.Unit) (ratio float64, offH, offV style.Unit, declared bool) {

	r, auto, ok := aspectRatioParts(b.Style.Get("aspect-ratio"))
	switch {
	case !ok, auto && natural > 0:
		// "auto", or "auto && <ratio>" on content with a ratio of its own:
		// the natural ratio, of the content box.
		return natural, 0, 0, false
	case auto:
		// "auto && <ratio>" on content with none: the declared ratio, still
		// of the content box.
		return r, 0, 0, true
	}
	// "<ratio>": of the box box-sizing names, which sizingInset measures —
	// nothing under content-box.
	offH, offV = l.sizingInset(b, containing)
	return r, offH, offV, true
}

// transferredHeight and transferredWidth carry a content-box size across a
// ratio that is of a box offH and offV larger than the content box.
func transferredHeight(w style.Unit, ratio float64, offH, offV style.Unit) style.Unit {
	return maxZero(w.Add(offH).Div(ratio).Sub(offV))
}

func transferredWidth(h style.Unit, ratio float64, offH, offV style.Unit) style.Unit {
	return maxZero(h.Add(offV).Mul(ratio).Sub(offH))
}

// positiveRatio is w/h, and only where both are numbers above zero.
//
// §4.1 makes a ratio with a zero or a negative in it invalid rather than
// degenerate, which matters: read as a number it would be an infinite or a
// negative height, and a box of either is not what the declaration asked for.
func positiveRatio(w, h string) (float64, bool) {
	wn, ok := parseNumber(w)
	if !ok || wn <= 0 {
		return 0, false
	}
	hn, ok := parseNumber(h)
	if !ok || hn <= 0 {
		return 0, false
	}
	return wn / hn, true
}

// aspectHeight is the height a box's ratio gives it for a width, for a box that
// declared no height of its own.
//
// The width is the content box's, and so is the height; the ratio is of the box
// preferredRatio says, which under "box-sizing: border-box" is the border box
// unless the value also says "auto".
func (l *layouter) aspectHeight(b *Box, width, containing style.Unit) (style.Unit, bool) {
	// Validity is the reader's to decide and is not repeated here. A second
	// "ratio > 0" test beside this one made the reader's own checks dead: a
	// plant that let a zero through was caught by the duplicate rather than by
	// the rule, so the rule was never the thing under test.
	ratio, offH, offV, ok := l.preferredRatio(b, 0, containing)
	if !ok {
		return 0, false
	}
	return transferredHeight(width, ratio, offH, offV), true
}

// reportAspectRatio names the half of the property this engine does not apply.
//
// Once per document, on the model of the other value reports: a stylesheet rule
// on four hundred elements is one thing to be told.
func (l *layouter) reportAspectRatio(b *Box, why string) {
	if l.reportedAspect == nil {
		l.reportedAspect = map[string]bool{}
	}
	if l.reportedAspect[why] {
		return
	}
	l.reportedAspect[why] = true
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Property: "aspect-ratio",
		Message:  "aspect-ratio " + why,
		Source:   sourceOf(boxElement(b)),
		Path:     PathOf(boxElement(b)),
	})
}
