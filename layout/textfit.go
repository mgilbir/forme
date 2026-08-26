package layout

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// css-text-5's text-fit: scaling the size a block's text is *set* in so that its
// lines fill the box.
//
//	text-fit: [ none | grow | shrink ] [ consistent | per-line | per-line-all ]? <percentage>?
//
// The property does not change the computed font-size, and that is the whole of
// what makes it implementable here rather than a second cascade: every
// font-relative length the document wrote — a line-height in ems, a margin, a
// border — goes on meaning what it meant, and only the type is drawn bigger. See
// linemetrics.go's strutAt, which is the same distinction on the vertical axis.
//
// Fitting happens "after text wrapping and before text justification", so the
// lines are broken at the declared size and never broken again. Under
// "consistent" that is not merely permitted but exact: one factor over the whole
// block scales every line's content and every line's room by the same number, so
// a break that fell where it did still falls there.
//
// # What scales and what does not
//
// A line's factor is "(A + B) / A" where A is the inline size of its *scalable*
// parts and B the room left over. What is not scalable is everything on the line
// that is not type: an atomic inline, an inline box's own margin, border and
// padding, and a letter-spacing or word-spacing the document wrote as a length.
// Those are subtracted first and the rest of the line is what the factor
// multiplies.
//
// A spacing written as a *percentage* is the one approximation here. CSS
// resolves it against the font-size at computed-value time, so by the time a
// line exists it is a length like any other and this treats it as unscalable —
// where css-text-5's own example scales it with the type. Naming it is the
// honest half; the suite's text-fit/spacing is the test that turns on it, and it
// wants per-line-all as well.

// fitMode is which direction the scaling may go.
type fitMode int

const (
	fitNone fitMode = iota
	fitGrow
	fitShrink
)

// textFit is a resolved text-fit declaration.
type textFit struct {
	mode fitMode
	// perLine and perLineAll are the two granularities that are not
	// "consistent", which is the default and the one this engine implements.
	perLine, perLineAll bool
	// limit is the maximum factor for grow and the minimum for shrink, and
	// hasLimit says whether the document wrote one. Without it there is no
	// limit on the scaling factor.
	limit    float64
	hasLimit bool
}

// scales reports whether this declaration asks for any scaling this engine does.
func (f textFit) scales() bool {
	return f.mode != fitNone && !f.perLine && !f.perLineAll
}

// clamp turns the factor the lines asked for into the one to use.
//
// "grow" never shrinks and "shrink" never grows, which is what makes the two
// values different requests rather than two names for the same one: a block that
// already overflows gets nothing from "grow", and one with room to spare gets
// nothing from "shrink".
func (f textFit) clamp(want float64) float64 {
	switch f.mode {
	case fitGrow:
		if want < 1 {
			return 1
		}
		if f.hasLimit && want > f.limit {
			return f.limit
		}
	case fitShrink:
		if want > 1 {
			return 1
		}
		if f.hasLimit && want < f.limit {
			return f.limit
		}
	default:
		return 1
	}
	return want
}

// textFitOf reads the property, and returns what it could not act on.
//
// The unhandled part is a value this engine understands and does not produce,
// which is a different thing from a value it cannot read: "per-line" is in the
// grammar and asks for a factor a line at a time, and answering it with the
// block's own factor would be a page the author did not ask for. So it is
// reported and nothing is scaled.
func textFitOf(b *Box) (textFit, string) {
	raw := strings.ToLower(strings.TrimSpace(b.Style["text-fit"]))
	if raw == "" || raw == "none" {
		return textFit{}, ""
	}
	var out textFit
	unhandled := ""
	for _, word := range strings.Fields(raw) {
		switch word {
		case "none":
			out.mode = fitNone
		case "grow":
			out.mode = fitGrow
		case "shrink":
			out.mode = fitShrink
		case "consistent":
			// The initial granularity, and the one implemented.
		case "per-line":
			out.perLine, unhandled = true, word
		case "per-line-all":
			out.perLineAll, unhandled = true, word
		default:
			if p, ok := percentValue(word); ok {
				out.limit, out.hasLimit = p, true
				continue
			}
			// A word that is not in the grammar. The declaration is not a
			// text-fit at all, so nothing is scaled and nothing is claimed.
			return textFit{}, word
		}
	}
	if out.mode == fitNone {
		return textFit{}, ""
	}
	return out, unhandled
}

// percentValue reads "75%" as 0.75.
func percentValue(word string) (float64, bool) {
	if !strings.HasSuffix(word, "%") {
		return 0, false
	}
	n, err := strconv.ParseFloat(strings.TrimSuffix(word, "%"), 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n / 100, true
}

// reportTextFit names a text-fit this engine read and did not act on.
func (l *layouter) reportTextFit(b *Box, unhandled string) {
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: AtHTML(offsetOf(b)),
		Message: "the text-fit " + quoteValue(unhandled) + " was not applied, so " +
			"the block's lines were left at the size they were set in",
		Property: "text-fit",
	})
}

// fitRuns scales the text on one line, and is the whole of what text-fit does to
// a run: the size it is set in, and therefore its width and how far it reaches
// above and below the baseline.
//
// Nothing else on the line is touched. An atomic inline is a box with a
// formatting context of its own — its text-fit is its own to apply — and an
// inline box's margin, border and padding are lengths the document wrote.
func (l *layouter) fitRuns(runs []inlineItem, scale float64) []inlineItem {
	if scale == 1 {
		return runs
	}
	out := make([]inlineItem, len(runs))
	copy(out, runs)
	for i := range out {
		it := &out[i]
		if it.Atomic != nil || it.Float != nil || it.Abs != nil {
			continue
		}
		if it.Face != nil && it.Text != "" {
			it.Size = it.Size.Mul(scale)
			it.Width = l.br.MeasureSpacedInContext(it.Face, it.Text, it.Size,
				it.Spacing, it.PreContext, it.PostContext)
		}
		// How far the item reaches above and below the baseline, which is the
		// same question on the vertical axis and has the same answer: the type
		// is bigger, so under "line-height: normal" the line is. An item that
		// carries no text still carries this — a forced break and an inline
		// box's own leading edge both do — and it is the box's leading either
		// way, so it is asked of the box rather than derived from the numbers
		// already there.
		if it.Leads {
			if box := heldBox(it.Box); box != nil {
				it.Above, it.Below = l.leadingAt(box, box.FontSize.Mul(scale))
			}
		}
	}
	return out
}

// fitStrut is strutOver over items text-fit has scaled.
//
// The forced break that raises a line's strut is found in the paragraph's own
// item stream, which is measured at the declared size and stays that way: the
// breaking is done there and must not move. So the one item the strut is taken
// from is scaled here instead.
func (l *layouter) fitStrut(st strut, items []inlineItem, next int, forced bool,
	scale float64) strut {

	if scale == 1 || !forced || next <= 0 || next > len(items) {
		return strutOver(st, items, next, forced)
	}
	one := []inlineItem{items[next-1]}
	return strutOver(st, l.fitRuns(one, scale), 1, forced)
}

// fitScalable is the part of a line's width the factor multiplies: css-text-5's
// A, with everything that is not type left out.
func (l *layouter) fitScalable(runs []inlineItem) style.Unit {
	var out style.Unit
	for _, it := range runs {
		if it.Face == nil || it.Text == "" || it.Inset || it.Atomic != nil ||
			it.Float != nil || it.Abs != nil {
			continue
		}
		out = out.Add(l.br.MeasureSpacedInContext(it.Face, it.Text, it.Size,
			paragraph.TextSpacing{}, it.PreContext, it.PostContext))
	}
	return out
}
