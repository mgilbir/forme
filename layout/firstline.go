package layout

import (
	"strings"

	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §5.12.1's ::first-line: the first formatted line of a block container,
// styled as though an inline box wrapped it.
//
// It is not a box. Nothing is generated, nothing is inserted, and the tree is
// the same tree — what changes is the type the first line is *set* in, which is
// why it has to reach the breaking rather than the painting: a first line in
// twice the size holds half the words, and the second line begins where the
// first one ended.
//
// # Which properties
//
// §5.12.1 lists what may apply: the font properties, colour, the background
// properties, word-spacing, letter-spacing, text-decoration, vertical-align,
// text-transform, line-height and text-shadow. Everything else is not merely
// ignored here, it does not apply at all — "margin" on a ::first-line is not a
// dropped declaration, it is a declaration CSS says has no meaning, and there is
// nothing to tell an author about it.
//
// Of the ones that do apply, this engine acts on the font properties, the
// line-height, the two spacings and the colour: the ones that decide how the
// line is measured and what colour it comes out. The rest are reported, because
// an author who writes them will not see them and has no other way to find out.
//
// text-transform is on the reported list and it is worth saying why, since it
// looks like the others. The transform is applied when the text of a box is
// built, which is long before any line exists; applying it here would mean
// building the text twice and deciding which of the two the *second* line
// continues from.

// firstLineApplies are the properties this engine takes from a ::first-line.
var firstLineApplies = []string{
	"font-family", "font-size", "font-weight", "font-style",
	"line-height", "letter-spacing", "word-spacing", "color",
}

// firstLinePaints are the ones that are drawn behind the line rather than
// changing how it is set. They belong to the pseudo-element's own box — which
// is the one box §5.12.1 says it behaves like — and not to the runs on the line.
var firstLinePaints = []string{"background-color", "background-image"}

// firstLineReports are the ones §5.12.1 says apply and this engine does not act
// on, in the order an author is likely to have written them.
var firstLineReports = []string{
	"text-transform", "text-decoration-line", "text-decoration-color",
	"vertical-align",
}

// firstLineDeclared is what a ::first-line rule actually said, reduced to the
// properties this engine acts on.
//
// "Actually said" is the whole of it. A pseudo-element's computed style holds
// every property in the registry, so comparing it against the element's own says
// the wrong thing twice over: for a property that does not inherit it reads the
// initial value as a declaration nobody wrote, and for one that does it cannot
// tell a value the author repeated from one that arrived. style.Undeclared
// answers the first; the second is not answerable from a computed style and does
// not matter, since repeating a value asks for the page that is already there.
//
// Nil when the rule said nothing this engine can act on, which is what keeps the
// rest of this file off every document that writes one.
//
// Every property on firstLineApplies is inherited today, so style.Undeclared
// answers the element's own value for all of them and reads exactly as the plain
// comparison would: planted, the plain comparison passes every test here. It is
// the general form because the first non-inherited property to join the list —
// background-color, when the first line's background is drawn — is the one it
// would be wrong about, and being wrong there means reading "transparent" as a
// declaration on every ::first-line of every coloured paragraph.
func (l *layouter) firstLineDeclared(b *Box) style.ComputedStyle {
	fl := b.FirstLine
	if fl == nil {
		return nil
	}
	var out style.ComputedStyle
	for _, name := range firstLineApplies {
		v, ok := fl[name]
		if !ok || v == style.Undeclared(name, b.Style[name]) {
			continue
		}
		if out == nil {
			out = style.ComputedStyle{}
		}
		out[name] = v
	}
	return out
}

// reportFirstLine names the ::first-line declarations that were not applied.
//
// Once per property and per document, like the other value findings: a
// stylesheet naming one is one thing the author has to know, not one per
// paragraph that inherits it.
func (l *layouter) reportFirstLine(b *Box) {
	fl := b.FirstLine
	if fl == nil {
		return
	}
	for _, name := range firstLineReports {
		v := strings.TrimSpace(fl[name])
		if v == "" || v == style.Undeclared(name, b.Style[name]) {
			continue
		}
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "the ::first-line " + name + " " + quoteValue(v) +
				" was not applied, so the first line is set like the rest",
			Property: name,
		})
	}
}

// firstLineBox is a box's text box as the first line sets it: the same box with
// the ::first-line declarations that reach it merged over its style.
//
// "That reach it" is §5.12.1's model, which makes the pseudo-element behave like
// an inline-level element wrapping the line's content — so its properties are
// *inherited* by that content, and a descendant that declared its own value
// keeps it. A "::first-line { color: lime }" over a paragraph holding a red
// <span> leaves the span red.
//
// Whether a descendant declared one is not a question a computed style can
// answer, so what stands in for it is whether the run's value is still the
// block's. That is exact except where a descendant declared the value the block
// already had, and there the two answers are the same page.
//
// Only a *text* box is cloned. A run's font, spacing and colour all come from
// the box its characters are in, and that box has no identity anything else
// depends on — where an inline box's identity is the key its background, its
// border, its offset and its vertical-align are recorded under, and a second
// copy of one would leave the first line's <span> without any of them.
func (l *layouter) firstLineBox(b, block *Box, declared style.ComputedStyle) *Box {
	if b == nil || !b.IsText() || declared == nil {
		return b
	}
	if got, ok := l.firstLineBoxes[b]; ok {
		return got
	}
	var cs style.ComputedStyle
	for name, v := range declared {
		if b.Style[name] != block.Style[name] {
			// A descendant of the block declared this one, and the pseudo-element
			// is its ancestor rather than its replacement.
			continue
		}
		if cs == nil {
			cs = style.ComputedStyle{}
			for k, old := range b.Style {
				cs[k] = old
			}
		}
		cs[name] = v
	}
	if cs == nil {
		l.rememberFirstLineBox(b, b)
		return b
	}
	out := *b
	out.Style = cs
	// The size resolved again, because the box builder resolved the box's and
	// this style is not the box's. The cascade has already made the value
	// absolute — a ::first-line font-size is relative to the *element's* own, and
	// that is a question only the cascade can answer — so what is left here is a
	// length in pixels.
	if length, ok := l.parseLength(&out, "font-size"); ok {
		if u, ok := length.Resolve(b.FontSize, true); ok && u > 0 {
			out.FontSize = u
		}
	}
	l.rememberFirstLineBox(b, &out)
	return &out
}

func (l *layouter) rememberFirstLineBox(from, to *Box) {
	if l.firstLineBoxes == nil {
		l.firstLineBoxes = map[*Box]*Box{}
	}
	l.firstLineBoxes[from] = to
}

// firstLineItems is the paragraph's items as the first line sets them.
//
// The list is parallel to the one it is made from — same length, same order,
// same item at every index — so the index the breaker hands back after the first
// line is an index into either of them. That is what lets the second line
// continue from the ordinary items without anything having to be mapped.
func (l *layouter) firstLineItems(items []inlineItem, block *Box,
	declared style.ComputedStyle) []inlineItem {

	out := make([]inlineItem, len(items))
	copy(out, items)
	for i := range out {
		it := &out[i]
		if it.Atomic != nil || it.Float != nil || it.Abs != nil || it.Inset {
			continue
		}
		box := l.firstLineBox(heldBox(it.Box), block, declared)
		if box == nil || box == heldBox(it.Box) {
			continue
		}
		it.Box = box
		if it.Face != nil && it.Text != "" {
			if face, ok := l.fontFor(box); ok {
				it.Face = face
			}
			it.Size = box.FontSize
			it.Spacing = l.spacingFor(box)
			it.Width = l.br.MeasureSpacedInContext(it.Face, it.Text, it.Size,
				it.Spacing, it.PreContext, it.PostContext)
		}
		if it.Leads {
			it.Above, it.Below = l.leading(box)
		}
	}
	return out
}

// firstLinePaint is the box §5.12.1's pseudo-element paints, or nil where the
// rule asks for nothing to be painted.
//
// It is a box of the *block's* rather than a restyled run, because that is what
// the specification says the pseudo-element behaves like: one inline-level
// element wrapping the line's content, whose background covers the content area
// of its own font over the extent of what is on the line.
func (l *layouter) firstLinePaint(b *Box) *Box {
	fl := b.FirstLine
	if fl == nil {
		return nil
	}
	var cs style.ComputedStyle
	for _, name := range firstLinePaints {
		v, ok := fl[name]
		if !ok || v == style.Undeclared(name, b.Style[name]) {
			continue
		}
		if cs == nil {
			cs = style.ComputedStyle{}
			for k, old := range b.Style {
				cs[k] = old
			}
			// The block's own edges are not the pseudo-element's: §5.12.1 does
			// not let it have a border or a padding, and taking the block's
			// would draw the block's border a second time round one line of it.
			for _, edge := range []string{"top", "right", "bottom", "left"} {
				cs["border-"+edge+"-width"] = "0"
				cs["border-"+edge+"-style"] = "none"
				cs["padding-"+edge] = "0"
				cs["margin-"+edge] = "0"
			}
		}
		cs[name] = v
	}
	if cs == nil {
		return nil
	}
	// The font properties too, since the height of what is painted is the
	// pseudo-element's own content area and its font is what decides that.
	for name, v := range l.firstLineDeclared(b) {
		cs[name] = v
	}
	out := *b
	out.Style = cs
	out.Outer, out.Inner = OuterInline, InnerFlow
	if length, ok := l.parseLength(&out, "font-size"); ok {
		if u, ok := length.Resolve(b.FontSize, true); ok && u > 0 {
			out.FontSize = u
		}
	}
	return &out
}

// firstLineFragment is the piece that box occupies on the line it is on.
//
// The extent is the line's own content — where the runs actually are, after the
// alignment has moved them — because that is what the pseudo-element wraps. A
// line with nothing drawn on it gets nothing.
func (l *layouter) firstLineFragment(box *Box, line *LineFragment) *Fragment {
	if box == nil || len(line.Runs) == 0 {
		return nil
	}
	lo, hi := line.Runs[0].X, line.Runs[0].X.Add(line.Runs[0].Width)
	for _, r := range line.Runs[1:] {
		if r.X < lo {
			lo = r.X
		}
		if end := r.X.Add(r.Width); end > hi {
			hi = end
		}
	}
	if hi <= lo {
		return nil
	}
	st := l.strutAt(box, box.FontSize)
	base := line.Rect.Y.Add(line.Baseline)
	return &Fragment{
		Box: box,
		BorderRect: Rect{
			X: line.Rect.X.Add(lo), Y: base.Sub(st.Ascent),
			W: hi.Sub(lo), H: st.Ascent.Add(st.Descent),
		},
	}
}
