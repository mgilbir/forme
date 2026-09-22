package layout

import (
	"strings"

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
// Nor does the ratio take part in intrinsic sizing — a float or a table cell
// shrunk to fit does not consult it — which is the same report.

// aspectRatioOf reads the property into the ratio of width to height.
//
// The grammar is "auto || <ratio>", and a <ratio> is one number or two with a
// solidus between them. "auto" alone is no ratio at all; "auto" beside one is
// the replaced-element form, where the element's own ratio wins and the
// declared one is the fallback — this engine has that ratio already, so the
// declared one is what is left for a box that has none.
func aspectRatioOf(raw string) (float64, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if value == "" || value == "auto" {
		return 0, false
	}
	// The keyword may sit either side of the ratio. Taking it out leaves the
	// ratio to be read on its own.
	fields := strings.Fields(strings.ReplaceAll(value, "/", " / "))
	var parts []string
	for _, f := range fields {
		if f == "auto" {
			continue
		}
		parts = append(parts, f)
	}
	switch len(parts) {
	case 1:
		return positiveRatio(parts[0], "1")
	case 3:
		if parts[1] != "/" {
			return 0, false
		}
		return positiveRatio(parts[0], parts[2])
	}
	return 0, false
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
// The width is the content box's, and so is the height: §4.1 applies the ratio
// to the box that box-sizing names, which is the content box unless a document
// says otherwise. A box whose sizing is the border box is not handled here and
// is reported with the rest.
func (l *layouter) aspectHeight(b *Box, width style.Unit) (style.Unit, bool) {
	// Validity is the reader's to decide and is not repeated here. A second
	// "ratio > 0" test beside this one made the reader's own checks dead: a
	// plant that let a zero through was caught by the duplicate rather than by
	// the rule, so the rule was never the thing under test.
	ratio, ok := aspectRatioOf(b.Style.Get("aspect-ratio"))
	if !ok {
		return 0, false
	}
	return maxZero(width.Div(ratio)), true
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
		Path:     PathOf(boxElement(b)),
	})
}
