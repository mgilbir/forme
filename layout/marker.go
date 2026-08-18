package layout

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// List markers: the bullet or the number a list item generates.
//
// A marker is not in the document. Nothing in the markup of "<li>one</li>" says
// "•", and nothing in the box tree did either until here — which is why the box
// carries a flag rather than a child, and why the text is worked out at layout
// time. The number in particular cannot be known earlier: it is the item's
// position among its siblings, and an item does not know its own position.

// Marker is the bullet or number drawn beside a list item.
type Marker struct {
	Text string
	Face *shape.Face
	Size style.Unit
	// At is the origin of the marker's baseline, relative to the fragment's
	// border box.
	At Point
	// Color is the item's own text colour: a marker takes the colour of the
	// text it belongs to, which is why an author never sets it separately.
	Color style.RGBA

	// Image is list-style-image's picture, drawn instead of the text above.
	//
	// When it is set the text is not drawn at all — §12.6.2 says the image
	// *replaces* the marker the type would have made — but Text is still filled
	// in, because it is what the marker falls back to and a caller extracting
	// the text of the page still wants to know what the item was numbered.
	Image *ReplacedContent
	// ImageRect is where the picture goes, relative to the fragment's border
	// box, at the image's own intrinsic size. §12.6.2 gives no way to scale it.
	ImageRect Rect
}

// markerFor works out the marker a list item generates, or nil.
//
// index is the item's one-based position among the list items of its parent. It
// is a fallback only: what a numbered list counts is the "list-item" counter,
// which the user-agent sheet increments and every list resets. The two agree for
// a plain list and disagree the moment a document says <ol start="5"> or
// <li value="3"> or resets the counter itself.
func (l *layouter) markerFor(b *Box, frag *Fragment, origin flow) *Marker {
	if markerInside(b) {
		// An inside marker is not drawn beside the box: it is the first thing on
		// the box's first line, and markerItem puts it there. See §12.5.1 and the
		// note on markerItem for why the two positions are different mechanisms
		// rather than two x coordinates.
		return nil
	}
	text, face, ok := l.markerRun(b)
	if !ok {
		return nil
	}

	size := b.FontSize
	width := l.br.Measure(face, text, size)
	lineHeight := l.lineHeight(b)
	// The marker sits on the item's *first line*, the same line its x is
	// measured against — so where there is one, its baseline is the marker's.
	//
	// It used to be derived from the strut alone, which is the right answer for
	// an item whose first line is an ordinary one and gives no answer at all
	// about which line it is on. The difference shows where the two disagree: a
	// first line taller than the strut — an image, a larger span, a line-height
	// of its own — puts its baseline further down, and the bullet stayed level
	// with a strut nothing was set in.
	baseline := frag.Border.Top.Add(frag.Padding.Top).Add(firstLineBaseline(frag, l.baselineOf(b, lineHeight)))
	// Where the item's *first line* starts, which is not where its content box
	// does when a float is in the way.
	//
	// §12.5.1 leaves the marker box's position unspecified and every renderer
	// puts it before the first line box, which is the only answer that keeps a
	// bullet next to the words it belongs to. A float shortens that line without
	// moving the box around it — a block's border box is not displaced by a
	// float, only the lines inside it are — so a marker placed from the content
	// edge is left behind under the float, an inch away from its own text.
	inner := frag.Border.Left.Add(frag.Padding.Left).Add(l.firstLineStart(frag, origin))

	m := &Marker{
		Text: text, Face: face, Size: size,
		// "outside" puts the marker in the margin, clear of the content box,
		// with a gap of half an em between it and the text — which is what
		// keeps a bullet from touching the word after it.
		At:    Point{X: inner.Sub(width).Sub(markerGap(size)), Y: baseline},
		Color: markerColour(b),
	}
	if img := b.MarkerImage; img != nil {
		// The picture goes where the text would have gone, at its own size,
		// sitting on the baseline. §12.6.2 says only that the image replaces
		// the marker; where a browser puts it is convention, and every one of
		// them rests it on the baseline rather than centring it on the line,
		// which is what keeps a tall image from lifting off the text it belongs
		// to.
		m.Image = img
		m.ImageRect = Rect{
			X: inner.Sub(img.Width).Sub(markerGap(size)),
			Y: baseline.Sub(img.Height),
			W: img.Width, H: img.Height,
		}
	}
	return m
}

// markerInside reports "list-style-position: inside".
func markerInside(b *Box) bool {
	return b.ListItem &&
		strings.EqualFold(strings.TrimSpace(b.Style["list-style-position"]), "inside")
}

// markerGap is the space between a marker and the text it belongs to.
func markerGap(size style.Unit) style.Unit { return size.Mul(0.5) }

// markerColour is the item's own text colour: a marker takes the colour of the
// text it belongs to, which is why an author never sets it separately.
func markerColour(b *Box) style.RGBA {
	if c, ok := (&painter{colors: map[string]style.RGBA{}}).color(b, "color"); ok {
		return c
	}
	return style.RGBA{A: 1}
}

// markerRun is the marker's text and the face to set it in, for either position.
func (l *layouter) markerRun(b *Box) (string, *shape.Face, bool) {
	if !b.ListItem {
		return "", nil, false
	}
	text := markerText(b.Style["list-style-type"], b.ListValue)
	if text == "" {
		return "", nil, false
	}
	face, ok := l.fontFor(b)
	if !ok {
		return "", nil, false
	}
	return text, face, true
}

// markerItems is what a block container's inline content starts with, which for
// a list item numbering itself inside is its marker and for everything else is
// nothing.
//
// It seeds both the line building and the intrinsic-width measurement, and it
// has to seed both: a shrink-to-fit list item whose width was measured without
// its marker is narrower than the marker it then draws.
func (l *layouter) markerItems(b *Box) []inlineItem {
	item, ok := l.markerItem(b)
	if !ok {
		return nil
	}
	return []inlineItem{item}
}

// markerItem is the line item an inside marker contributes, if there is one.
//
// §12.5.1 puts an inside marker "as the first inline box in the principal block
// box, before the element's content" — a *box on the line*, not a mark beside
// one. The difference is not cosmetic and shows in three places at once: the
// marker pushes the first line's text along, it takes part in the line's height
// and its width, and — the case the suite is full of — it makes an *empty* list
// item generate a line box at all. An item with no content and an inside marker
// is one line tall and shows its background; drawn as a mark beside the box it
// was zero-tall, and the background of a dozen tests went missing.
//
// It is deliberately not registered with the bidi builder. A marker is its own
// box rather than part of the run of text after it, so it does not join that
// text's directional run — which is what leaves it at the start of the line in a
// right-to-left list rather than reordered into the middle of the first word.
func (l *layouter) markerItem(b *Box) (inlineItem, bool) {
	if !markerInside(b) {
		return inlineItem{}, false
	}
	text, face, ok := l.markerRun(b)
	if !ok {
		return inlineItem{}, false
	}
	if b.MarkerImage != nil {
		// An inside marker is a box *on the line*, and a line item in this
		// engine carries text and not a picture — an inline image reaches a line
		// by the atomic-inline path, which a marker does not go through because
		// a marker is not in the document.
		//
		// So the image is not drawn here, and the type's marker is used instead.
		// That is the same fallback §12.6.2 gives for an image that did not
		// load, which makes the page a legitimate rendering rather than a
		// broken one — but it is not what was asked for, and the difference is
		// exactly what a finding is for.
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "a \"list-style-image\" on an inside marker is not drawn by this " +
				"engine, because an inside marker is a box on the line and a line " +
				"carries text; the marker from \"list-style-type\" was used instead",
			Path:     PathOf(b.Element),
			Property: "list-style-image",
		})
	}
	size := b.FontSize
	above, below := l.leading(b)
	return inlineItem{
		Text: text, Box: b, Face: face, Size: size,
		// The same half-em the outside marker leaves, spent as width rather than
		// as an offset: here what it separates is the next item on the line.
		Width: l.br.Measure(face, text, size).Add(markerGap(size)),
		Leads: true, Above: above, Below: below,
	}, true
}

// markerText renders the marker for a list-style-type and a position.
func markerText(listStyle string, index int) string {
	switch strings.ToLower(strings.TrimSpace(listStyle)) {
	case "none":
		return ""
	case "circle":
		return "◦" // ◦
	case "square":
		return "▪" // ▪
	case "decimal-leading-zero":
		if index < 10 {
			return "0" + strconv.Itoa(index) + "."
		}
		return strconv.Itoa(index) + "."
	case "decimal":
		return strconv.Itoa(index) + "."
	case "lower-alpha", "lower-latin":
		return alphabetic(index, 'a') + "."
	case "upper-alpha", "upper-latin":
		return alphabetic(index, 'A') + "."
	case "lower-roman":
		return strings.ToLower(roman(index)) + "."
	case "upper-roman":
		return roman(index) + "."
	case "lower-greek":
		return alphabeticIn(index, lowerGreek) + "."
	case "armenian":
		return additive(index, armenianNumerals, 1, 9999) + "."
	case "georgian":
		return additive(index, georgianNumerals, 1, 19999) + "."
	default:
		// "disc" and anything unrecognised. An unknown type falling back to a
		// bullet is what browsers do, and a list with no marker at all would
		// look like a deliberate "none".
		return "•" // •
	}
}

// alphabetic numbers a list a, b, … z, aa, ab, …
//
// It is bijective base 26, not ordinary base 26: there is no zero digit, so
// after "z" comes "aa" rather than "ba". Ordinary base-26 arithmetic gets this
// wrong at exactly the 26th item, which is far enough into a list that nobody
// notices until a document has one.
func alphabetic(index int, first rune) string {
	if index < 1 {
		return ""
	}
	var out []rune
	for index > 0 {
		index--
		out = append([]rune{first + rune(index%26)}, out...)
		index /= 26
	}
	return string(out)
}

// lowerGreek is the alphabet §12.6.2's "lower-greek" counts in.
//
// Twenty-four letters and not twenty-five: final sigma, U+03C2, is the same
// letter as U+03C3 in a different position in a word, so it is not a numeral and
// the sequence steps straight over it. An implementation that walked the code
// points from alpha to omega would number every list one out from the eighteenth
// item on.
var lowerGreek = []rune("αβγδεζηθικλμνξοπρστυφχψω")

// armenianNumerals and georgianNumerals are the two additive systems §12.6.2
// names, as CSS Counter Styles §6.2 spells them out.
//
// Additive rather than positional: the number is written as the sum of the
// largest numerals that fit, so 1979 in Armenian is Ռ (1000) Ջ (900) Հ (70) Թ
// (9) and not four digits. Roman numerals are the same idea with subtractive
// pairs on top, which is why roman is a separate function rather than a call to
// this one.
var armenianNumerals = []additiveNumeral{
	{9000, "Ք"}, {8000, "Փ"}, {7000, "Ւ"}, {6000, "Ց"}, {5000, "Ր"},
	{4000, "Տ"}, {3000, "Վ"}, {2000, "Ս"}, {1000, "Ռ"},
	{900, "Ջ"}, {800, "Պ"}, {700, "Չ"}, {600, "Ո"}, {500, "Շ"},
	{400, "Ն"}, {300, "Յ"}, {200, "Մ"}, {100, "Ճ"},
	{90, "Ղ"}, {80, "Ձ"}, {70, "Հ"}, {60, "Կ"}, {50, "Ծ"},
	{40, "Խ"}, {30, "Լ"}, {20, "Ի"}, {10, "Ժ"},
	{9, "Թ"}, {8, "Ը"}, {7, "Է"}, {6, "Զ"}, {5, "Ե"},
	{4, "Դ"}, {3, "Գ"}, {2, "Բ"}, {1, "Ա"},
}

var georgianNumerals = []additiveNumeral{
	{10000, "ჵ"},
	{9000, "ჰ"}, {8000, "ჯ"}, {7000, "ჴ"}, {6000, "ხ"}, {5000, "ჭ"},
	{4000, "წ"}, {3000, "ძ"}, {2000, "ც"}, {1000, "ჩ"},
	{900, "შ"}, {800, "ყ"}, {700, "ღ"}, {600, "ქ"}, {500, "ფ"},
	{400, "ჳ"}, {300, "ტ"}, {200, "ს"}, {100, "რ"},
	{90, "ჟ"}, {80, "პ"}, {70, "ო"}, {60, "ჲ"}, {50, "ნ"},
	{40, "მ"}, {30, "ლ"}, {20, "კ"}, {10, "ი"},
	{9, "თ"}, {8, "ჱ"}, {7, "ზ"}, {6, "ვ"}, {5, "ე"},
	{4, "დ"}, {3, "გ"}, {2, "ბ"}, {1, "ა"},
}

// additiveNumeral is one weight and the mark that stands for it.
type additiveNumeral struct {
	weight int
	symbol string
}

// alphabeticIn numbers a list in an arbitrary alphabet, bijectively.
//
// It is alphabetic's argument with the alphabet given rather than derived from a
// first letter, because Greek's is not a run of consecutive code points. The two
// share the bijective arithmetic and nothing else: there is no zero digit, so
// after the last letter comes the first letter doubled.
func alphabeticIn(index int, alphabet []rune) string {
	n := len(alphabet)
	if index < 1 || n == 0 {
		return strconv.Itoa(index)
	}
	var out []rune
	for index > 0 {
		index--
		out = append([]rune{alphabet[index%n]}, out...)
		index /= n
	}
	return string(out)
}

// additive writes a number as the sum of the largest numerals that fit.
//
// Outside the system's range it falls back to the decimal, which is §12.6.2's
// own instruction for a marker a style cannot represent — Armenian stops at 9999
// and Georgian at 19999, and a list longer than that numbered in nothing at all
// would lose its numbering silently.
func additive(index int, numerals []additiveNumeral, lo, hi int) string {
	if index < lo || index > hi {
		return strconv.Itoa(index)
	}
	var b strings.Builder
	for _, n := range numerals {
		for index >= n.weight {
			b.WriteString(n.symbol)
			index -= n.weight
		}
	}
	return b.String()
}

// roman renders a number in Roman numerals, for the two list styles that ask.
//
// Values outside what the numerals express fall back to the decimal, which is
// what the specification requires — MMMM is not a numeral, and a list of four
// thousand items numbered in Roman is not what the author was imagining anyway.
func roman(index int) string {
	if index < 1 || index > 3999 {
		return strconv.Itoa(index)
	}
	values := [...]int{1000, 900, 500, 400, 100, 90, 50, 40, 10, 9, 5, 4, 1}
	symbols := [...]string{"M", "CM", "D", "CD", "C", "XC", "L", "XL", "X", "IX", "V", "IV", "I"}
	var b strings.Builder
	for i, v := range values {
		for index >= v {
			b.WriteString(symbols[i])
			index -= v
		}
	}
	return b.String()
}

// firstLineStart is how far into the content box the item's first line begins,
// which is what a float on that side pushes along.
//
// Zero when there is no line at all, and that is not the same as "no float": an
// item with an outside marker and no content of its own has no line box to be
// shortened, so there is nothing here that knows about the float. See the note
// on markerNeedsALine — the two are the same missing line box seen from two
// sides, and this half is the one that can be fixed without deciding how tall an
// empty list item is.
func (l *layouter) firstLineStart(frag *Fragment, origin flow) style.Unit {
	if frag == nil {
		return 0
	}
	if len(frag.Lines) > 0 {
		return frag.Lines[0].Rect.X
	}
	// No line, and the marker still has to go where one would have started.
	//
	// An item with no content of its own is not a rare shape: it is how the
	// suite writes "does this property apply to a list item", and a browser puts
	// its bullet where the first line box would have been. That is not the
	// content edge whenever a float is in the way — a block's border box is not
	// displaced by a float, only the lines inside it are — so an empty item
	// beside a one-inch float had its marker an inch to the left of where every
	// renderer puts it, out past the page's own margin.
	if origin.ctx == nil {
		return 0
	}
	// The band at the item's first line, measured between the item's own content
	// edges: bandAt clamps to them, so what comes back is where a line inside
	// *this* box would begin.
	lo := origin.x.Add(frag.BorderRect.X).Add(frag.Border.Left).Add(frag.Padding.Left)
	hi := lo.Add(frag.ContentRect().W)
	y := origin.y.Add(frag.BorderRect.Y).Add(frag.Border.Top).Add(frag.Padding.Top)
	left, _ := origin.ctx.bandAt(y, lo, hi)
	return style.Max(left.Sub(lo), 0)
}

// firstLineBaseline is where the item's first line puts its baseline, or the
// given fallback when the item has no line at all.
//
// The fallback is the strut's, which is what an item with no line has instead of
// one — and an item with an outside marker and no content of its own is exactly
// that. See the note on issue #23: the marker being placed from the strut
// whether or not a line exists is why a zero-tall item and a one-line item put
// their bullets in the same place, and why that agreement is not evidence that
// their boxes agree.
func firstLineBaseline(frag *Fragment, fallback style.Unit) style.Unit {
	if frag == nil || len(frag.Lines) == 0 {
		return fallback
	}
	return frag.Lines[0].Rect.Y.Add(frag.Lines[0].Baseline)
}
