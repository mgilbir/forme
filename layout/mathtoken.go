package layout

import (
	"math"

	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The token elements: MathML Core §3.2.1's layout of <mtext>, which <mi>, <mn>,
// <ms>, the annotations and — where it is not stretched or enlarged — <mo>
// share.
//
// A token whose content is text and nothing else is one line of that text.
// It is laid out as the line it is — shaped, spaced, coloured, decorated, by
// the inline layout every paragraph goes through — and then measured by its
// ink rather than by its line box: the math content box reaches from the top
// of the highest glyph to the bottom of the lowest, and its baseline is the
// text's. That is what makes a superscript sit over an "x" and not over the
// empty space above the "x" a line of text reserves for capitals. Where the
// text is one glyph, its italic correction and top accent attachment are the
// ones the first available font's MATH table states for it.
//
// A token holding anything else — an HTML element, a forced line break — is
// laid out as the block box it then is, and measured by its first baseline.

// mathToken lays a token out: its fragment, holding its line, and its content
// box's measurements.
func (l *layouter) mathToken(b *Box, containing style.Unit, margin Edges, s mathStretch) (mathContent, *Fragment) {
	width := l.contentWidths(b).max
	held := l.mathTextBox
	l.mathTextBox = b
	frag, _ := l.layBlock(b, containing, aloneFlow(0, false), &forcedGeometry{margin: margin, width: width})
	l.mathTextBox = held
	c := l.mathTokenContent(b, frag, width)
	if s.block && (s.ascent > c.inkAscent || s.descent > c.inkDescent) || s.inline && s.size > c.width {
		// Asked to grow past its own glyph, which this change does not do.
		l.mathNotStretched(b)
	}
	if op, ok := l.mathOperator(b); ok && op.largeop && mathStyleNormal(b) {
		// §3.2.4.3's large operator, drawn larger in display mathematics:
		// not done by this change either.
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   sourceOf(boxElement(b)),
			Message:  "a large operator in display mathematics is drawn at its text size: this engine does not enlarge operators yet",
			Path:     PathOf(b.Element),
			Property: "largeop",
		})
	}
	return c, frag
}

// mathTokenContent measures a token's laid out lines as §3.2.1.1 measures
// them, moving the line so that its baseline is at the top of the content box
// plus the ascent the measurement arrived at.
func (l *layouter) mathTokenContent(b *Box, frag *Fragment, width style.Unit) mathContent {
	c := mathContent{width: width}
	inset := frag.Border.Top.Add(frag.Padding.Top)
	switch {
	case len(frag.Lines) == 1 && mathOnlyText(b):
		line := &frag.Lines[0]
		ink := l.mathLineInk(line)
		c.ascent, c.descent = ink.ascent, ink.descent
		line.move(0, c.ascent.Sub(line.Rect.Y.Add(line.Baseline)))
		if ink.single {
			m := l.mathFontFor(b)
			if ink.face == m.face && m.table != nil {
				if v, ok := m.table.ItalicsCorrection(ink.gid); ok {
					c.italic, c.hasItalic = ink.scale(v), true
				}
				if v, ok := m.table.TopAccentAttachment(ink.gid); ok {
					c.accent, c.hasAccent = ink.scale(v), true
				}
			}
		}
	default:
		// The block box it is: its height, and its first baseline. With no
		// line at all, the baseline is its bottom edge.
		h := frag.BorderRect.H.Sub(frag.Border.Vertical()).Sub(frag.Padding.Vertical())
		c.ascent = h
		if v, ok := firstBaseline(frag); ok {
			c.ascent = v.Sub(inset)
		}
		c.descent = h.Sub(c.ascent)
	}
	c.inkAscent, c.inkDescent = c.ascent, c.descent
	return c
}

// mathTokenBlockContent is a token that block layout reached — a token laid
// out as block math because its parent is a CSS box — measured and placed as
// mathToken places it, into the fragment block layout made for it, centred.
func (l *layouter) mathTokenBlockContent(b *Box, parent *Fragment, width style.Unit,
	topOpen, bottomOpen bool, origin flow) style.Unit {

	held := l.mathTextBox
	l.mathTextBox = b
	l.children(b, parent, width, topOpen, bottomOpen, origin)
	l.mathTextBox = held
	advance := l.contentWidths(b).max
	c := l.mathTokenContent(b, parent, advance)
	if dx := width.Sub(advance).Div(2); dx > 0 && len(parent.Lines) == 1 {
		parent.Lines[0].move(dx, 0)
	}
	parent.mathBaseline, parent.hasMathBaseline = c.ascent, true
	return c.ascent.Add(c.descent)
}

// mathInk is the ink of a line of text, and the one glyph it is drawn with
// where it is one.
type mathInk struct {
	ascent, descent style.Unit
	single          bool
	face            *shape.Face
	gid             int
	size            float64 // px, of the single glyph's run
}

// scale is a length in the single glyph's font units, in pixels.
func (m mathInk) scale(v int) style.Unit {
	if m.face == nil || m.face.UnitsPerEm() == 0 {
		return 0
	}
	u, _ := style.FromPx(float64(v) * m.size / float64(m.face.UnitsPerEm()))
	return u
}

// mathLineInk is how far a line's glyphs reach above and below its baseline:
// each run shaped as a backend shapes it (see ShapedGlyphs), and each glyph's
// ink box, moved by its offset. A face that cannot state a glyph's ink — one
// of the fourteen standard faces, which state their boxes by character — is
// measured by the character's box instead.
func (l *layouter) mathLineInk(line *LineFragment) mathInk {
	top, bottom := math.Inf(-1), math.Inf(1)
	var out mathInk
	glyphs := 0
	for _, run := range line.Runs {
		if run.Face == nil {
			continue
		}
		size := run.Size.Px()
		upem := float64(run.Face.UnitsPerEm())
		v := DrawText{Text: drawableText(run.Text), Features: run.Features,
			PreContext: run.PreContext, PostContext: run.PostContext,
			MergePre: run.MergePre, MergePost: run.MergePost,
			ContextKerns: run.ContextKerns, RTL: run.RTL, Face: run.Face, Size: run.Size}
		gs, _ := ShapedGlyphs(v)
		measured := true
		for _, g := range gs {
			glyphs++
			out.face, out.gid, out.size = run.Face, g.GID, size
			_, yb, w, h, ok := run.Face.GlyphExtents(g.GID)
			if !ok {
				measured = false
				break
			}
			if w == 0 && h == 0 {
				continue // a glyph with no ink
			}
			dy := g.YOffset * size / 1000
			top = math.Max(top, float64(yb)*size/upem+dy)
			bottom = math.Min(bottom, float64(yb+h)*size/upem+dy)
		}
		if !measured {
			if above, below, ok := run.Face.InkExtent(run.Text, size); ok {
				top, bottom = math.Max(top, above), math.Min(bottom, -below)
			}
		}
	}
	out.single = glyphs == 1
	if !math.IsInf(top, 0) {
		out.ascent, _ = style.FromPx(top)
		out.descent, _ = style.FromPx(-bottom)
	}
	return out
}

// mathNotStretched reports an operator that a formula asked to stretch and
// that is drawn at its text size.
func (l *layouter) mathNotStretched(b *Box) {
	l.rec.ReportDetail(Finding{
		Rule:     RuleUnsupportedValue,
		Source:   sourceOf(boxElement(b)),
		Message:  "a stretchy operator is drawn at its text size: this engine does not stretch operators yet",
		Path:     PathOf(b.Element),
		Property: "stretchy",
	})
}

// mathStyleNormal reports whether a box's math-style is normal: set as display
// mathematics rather than compact.
func mathStyleNormal(b *Box) bool {
	return !ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("math-style")), "compact")
}
