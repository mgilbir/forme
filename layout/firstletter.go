package layout

import (
	"unicode/utf8"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §5.12.2 and css-pseudo-4 §4.1's ::first-letter: the first letter of
// the first formatted line of a block container, styled as though an inline box
// wrapped it.
//
// It is done here, in the box builder, and not as a pass over the tree — which
// is the one decision in this file that is not obvious. text-transform is a
// property that applies to a ::first-letter, and the transform runs when a text
// box is built; a pass that split the text afterwards would either miss the
// transform or apply it to text that had already been through another one. The
// split belongs where the text is made.
//
// It is also not a box. §5.12.1's ::first-line is not one either, and the reason
// is the same: what changes is the type a stretch of text is *set* in, and a
// second box would need an identity that a background, a border and an offset
// are recorded under. That is the half of ::first-letter this engine does not
// do, and it is reported rather than left silent — a drop cap is written with
// "float" and a border, and an author who writes one has to be told the letter
// came out ordinary.
//
// # Which properties
//
// css-pseudo-4 lists them: the font properties, colour, the background
// properties, margin, padding, border, text-decoration, vertical-align (where
// the letter is not floated), text-transform, line-height, float, opacity and
// the two spacings. Everything else does not apply at all, and a declaration of
// one is not a dropped value but a value CSS gives no meaning — there is nothing
// to tell an author about it.

// firstLetterApplies are the ones this engine takes from a ::first-letter: the
// ones that decide what the letter is set in and what it says.
var firstLetterApplies = []string{
	"font-family", "font-size", "font-weight", "font-style",
	"line-height", "letter-spacing", "word-spacing", "color", "text-transform",
}

// firstLetterReports are the ones css-pseudo-4 says apply and this engine does
// not act on, because each of them needs the letter to have a box of its own.
var firstLetterReports = []string{
	"float", "background-color", "background-image", "vertical-align",
	"margin-left", "margin-right", "margin-top", "margin-bottom",
	"padding-left", "padding-right", "padding-top", "padding-bottom",
	"border-left-width", "border-right-width",
	"border-top-width", "border-bottom-width",
	"text-decoration-line",
}

// applyFirstLetter divides a block container's first text box so that its first
// letter is set by the pseudo-element's own style.
//
// Nothing happens where the rule said nothing this engine acts on, which keeps
// the walk below off every document that writes one for its border alone.
func (b *boxBuilder) applyFirstLetter(box *Box, n *html.Node, fontSize style.Unit) {
	fl := b.pseudo[style.PseudoKey{Node: n, Name: "first-letter"}]
	if fl.IsZero() {
		return
	}
	b.reportFirstLetter(n, box, fl)

	declared := firstLetterDeclared(fl, box.Style)
	if declared == nil {
		return
	}
	parent, at := firstTextBox(box)
	if parent == nil {
		return
	}
	text := parent.Children[at].Text
	cut := firstLetterLen(text)
	if cut <= 0 {
		return
	}

	head := *parent.Children[at]
	head.Style = mergedOver(head.Style, declared)
	head.Text = text[:cut]
	if kind := transformOf(head.Style.Get("text-transform")); kind != transformNone {
		// The element's own transform has already run over this text; the
		// pseudo-element's is a second one, over the letter alone, and it is
		// what "text-transform: uppercase" on a ::first-letter means. It is
		// given no word boundary, because a first letter begins one.
		if got, _ := transformText(head.Text, kind, wordClosed, b.languageAt(n)); got != "" {
			head.Text = got
		}
	}
	if _, ok := declared["font-size"]; ok {
		// Resolved again, because the size the box builder gave this text box is
		// the element's and this style is not the element's. The cascade has
		// already made the value absolute — a ::first-letter font-size is
		// relative to the element's own, which is a question only the cascade
		// can answer — so what is left is a length.
		head.FontSize = b.fontSizeOfStyle(head.Style, fontSize, true)
	}

	if cut >= len(text) {
		parent.Children[at] = &head
		return
	}
	tail := *parent.Children[at]
	tail.Text = text[cut:]
	rest := make([]*Box, 0, len(parent.Children)+1)
	rest = append(rest, parent.Children[:at]...)
	rest = append(rest, &head, &tail)
	rest = append(rest, parent.Children[at+1:]...)
	for _, c := range rest {
		c.Parent = parent
	}
	parent.Children = rest
}

// firstLetterDeclared is what a ::first-letter rule actually said, reduced to
// the properties this engine acts on.
//
// It is firstLineDeclared's argument exactly: a pseudo-element's computed style
// holds every property in the registry, so comparing it against the element's
// own reads an initial value as a declaration nobody wrote. style.Undeclared is
// what answers that.
//
// It is a set of declarations and not a computed style, so it is a plain map:
// the properties it does not name are not at any value, they are not said.
func firstLetterDeclared(fl, own style.ComputedStyle) map[string]string {
	var out map[string]string
	for _, name := range firstLetterApplies {
		v, ok := fl.Lookup(name)
		if !ok || v == style.Undeclared(name, own.Get(name)) {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[name] = v
	}
	return out
}

// reportFirstLetter names the ::first-letter declarations that were not applied.
//
// Once per property per document, like every other value finding: a stylesheet
// naming one is one thing the author has to know and not one per paragraph.
func (b *boxBuilder) reportFirstLetter(n *html.Node, box *Box, fl style.ComputedStyle) {
	for _, name := range firstLetterReports {
		v := ascii.TrimCSSSpace(fl.Get(name))
		if v == "" || v == style.Undeclared(name, box.Style.Get(name)) {
			continue
		}
		b.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(n.Offset),
			Message: "the ::first-letter " + name + " " + quoteValue(v) +
				" was not applied: this engine sets the letter in the " +
				"pseudo-element's type and gives it no box of its own",
			Path:     PathOf(n),
			Property: name,
		})
	}
}

// mergedOver is a style with declarations written over it. The base is not
// changed: With copies what it writes to.
func mergedOver(base style.ComputedStyle, over map[string]string) style.ComputedStyle {
	out := base
	for k, v := range over {
		out = out.With(k, v)
	}
	return out
}

// firstTextBox finds the box holding the first letter of a block container's
// first formatted line, and where in its parent it sits.
//
// In document order, descending into everything in flow: §5.12.2 puts the first
// letter in the first *block container* of the element where the element's own
// first child is one, and inside an inline box where the letter is written in a
// <span>. Walking in order answers both without asking which case it is.
//
// An out-of-flow box is passed over, because it is not on the line at all — a
// float or an absolutely positioned box before the text does not hold the
// paragraph's first letter. A replaced box *ends* the walk: a picture is not a
// letter and the first formatted line has one in front of the text.
//
// So does every other atomic inline, and every block-level box that is not a
// block container. An inline-block is a box on the line, like a picture, and
// CSS 2.1 §5.12.1 says in as many words that it "cannot be the first formatted
// line of an ancestor": its letters are its own, for its own ::first-letter.
// The walk went into it, and "<div><span style=display:inline-block>Inner</span>
// outer</div>" enlarged the "I" inside the inline-block. A table, a flex and a
// grid container hold no first formatted line of their parent either — css-
// pseudo-4 §2.3 descends only into a block container — so they end the walk too.
func firstTextBox(box *Box) (parent *Box, at int) {
	for i, c := range box.Children {
		if c == nil || c.outOfFlow() {
			continue
		}
		if c.Replaced != nil || c.Control != nil {
			return nil, 0
		}
		if !c.IsText() && !holdsParentsFirstLine(c) {
			return nil, 0
		}
		if c.IsText() {
			if blank(c.Text) {
				// White space alone is not the first letter and does not end
				// the search: "<p> <span>x</span>" has its letter in the span.
				continue
			}
			return box, i
		}
		if p, k := firstTextBox(c); p != nil {
			return p, k
		}
	}
	return nil, 0
}

// firstLetterLen is how much of a text a ::first-letter covers, in bytes.
//
// css-pseudo-4: the first *typographic character unit* of the line, together
// with any punctuation that precedes or follows it. The unit is the grapheme
// cluster, which is what makes "e" and a combining acute one letter — the suite
// writes that as combining-characters-002, whose whole assertion is that the two
// are styled as one character.
//
// Nought where there is no letter to cover, which is text that is white space or
// punctuation and nothing else.
func firstLetterLen(text string) int {
	i := 0
	for i < len(text) {
		r, n := utf8.DecodeRuneInString(text[i:])
		if !charprop.WhiteSpace(r) {
			break
		}
		i += n
	}
	i = skipPunctuation(text, i)
	if i >= len(text) {
		return 0
	}
	cut := i + firstClusterLen(text[i:])
	return skipPunctuation(text, cut)
}

// skipPunctuation advances over the punctuation at i, which css-pseudo-4
// includes in the first letter on either side of it.
func skipPunctuation(text string, i int) int {
	for i < len(text) {
		r, n := utf8.DecodeRuneInString(text[i:])
		if !charprop.Is(r, charprop.P) {
			break
		}
		i += n
	}
	return i
}

// firstClusterLen is the length of the first grapheme cluster of a text.
func firstClusterLen(text string) int {
	if at := segment.Boundaries(nil, text); len(at) > 0 {
		return at[0]
	}
	return len(text)
}

// holdsParentsFirstLine reports whether a box in flow may hold its parent's
// first formatted line: an inline box that is not atomic, whose content is on
// the parent's lines, or a block-level block container in the same flow.
func holdsParentsFirstLine(b *Box) bool {
	if b.Outer == OuterInline {
		return b.Inner == InnerFlow
	}
	return !b.TableWrapper && isBlockContainer(b)
}
