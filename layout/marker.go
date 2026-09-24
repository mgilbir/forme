package layout

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/bidi"
	"github.com/mgilbir/forme/internal/ascii"
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

	// pieces is Text as it is drawn, where that is not one left-to-right run:
	// the stretches of one direction, in the order they go from left to
	// right. See markerPieces. Text stays the marker in logical order, which
	// is what a reader copying it out of the page wants.
	pieces []markerPiece
}

// markerPiece is one stretch of an outside marker's text that runs one way.
type markerPiece struct {
	text string
	// x is where it starts, from the marker's At.
	x   style.Unit
	rtl bool
}

// markerFor works out the marker an outside list item generates, or nil.
//
// The number is the item's "list-item" counter, which the box builder read into
// Box.ListValue: the user-agent sheet increments it and every list resets it, so
// it is the item's position for a plain list and what the document said for
// <ol start="5">, <li value="3"> or a counter reset of its own.
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
	rtl := isRTL(b)
	pieces, width := l.markerPieces(face, text, size, rtl)
	lineHeight := l.lineHeight(b)
	// The marker sits on the item's *first formatted line*, the same line its x
	// is measured against — so where there is one, its baseline is the marker's.
	//
	// It used to be derived from the strut alone, which is the right answer for
	// an item whose first line is an ordinary one and gives no answer at all
	// about which line it is on. The difference shows where the two disagree: a
	// first line taller than the strut — an image, a larger span, a line-height
	// of its own — puts its baseline further down, and the bullet stayed level
	// with a strut nothing was set in.
	//
	// And the first formatted line is not always the item's own. §5.12.1 finds
	// it "inside a block-level descendant in the same flow" when the item's
	// content is block-level, which is how most list items are written once they
	// hold more than a word: "<li><p>", "<li><h2>". Reading only the item's own
	// lines found none there and fell back to the strut, so a bullet beside a
	// 40px heading sat level with a 16px line nobody set.
	first, found := firstLineIn(frag)
	baseline := first.baseline
	if !found {
		baseline = frag.Border.Top.Add(frag.Padding.Top).Add(l.baselineOf(b, lineHeight))
	}
	// Where the marker sits from: the item's *border* box, moved along by
	// whatever a float has taken off its first line.
	//
	// §12.5.1 says only that an outside marker is "outside the principal box",
	// and outside means outside the border box rather than outside the content
	// box. The item's own padding and border are inside it, so neither moves the
	// bullet: padding-left-applies-to-010 puts fifty pixels of padding and ten
	// of border on a list item and asks for the marker "on the left-hand side of
	// the blue line", which is the border it named.
	//
	// A float is the other half and is why this is not simply the border box. It
	// shortens the first line without moving the box around it — a block's
	// border box is not displaced by a float, only the lines inside it are — so
	// a marker placed from the box alone is left behind under the float, an inch
	// away from its own text.
	//
	// Which side is the item's inline-start side, and that is not always the
	// left. css-lists-3 puts an outside marker before the first line in the
	// inline direction, so in a right-to-left item it is past the *right* border
	// edge and the gap is on its left: an Arabic or Hebrew list drawn with its
	// bullets on the left has them at the far end of the line from the words
	// they number. start is the distance the float pushes the line in from that
	// side, and the two sides are mirror images of each other.
	start := l.firstLineStart(frag, origin, first, found, rtl)
	gap := markerGap(size)
	// before is where a thing w wide goes so that it ends a gap short of the
	// line: to its left in a left-to-right item, to its right in the other.
	before := func(w style.Unit) style.Unit {
		if rtl {
			return frag.BorderRect.W.Sub(start).Add(gap)
		}
		return start.Sub(w).Sub(gap)
	}

	m := &Marker{
		Text: text, Face: face, Size: size,
		// "outside" puts the marker in the margin, clear of the content box,
		// with a gap of half an em between it and the text — which is what
		// keeps a bullet from touching the word after it.
		At:     Point{X: before(width), Y: baseline},
		Color:  markerColour(b),
		pieces: pieces,
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
			X: before(img.Width),
			Y: baseline.Sub(img.Height),
			W: img.Width, H: img.Height,
		}
	}
	return m
}

// markerPieces is an outside marker's text in the order it is drawn, and the
// width of all of it.
//
// css-lists-3 gives ::marker "unicode-bidi: isolate" in the user agent's
// sheet, and its direction is the list item's, which it inherits: so the
// marker's text is a paragraph of its own, resolved by UAX #9 with the item's
// direction as its base. In a right-to-left item "12." is a European number
// and a common separator, and rule W4 does not join them — the stop is
// resolved to the paragraph's direction and goes on the far side of the
// number, which is to its left. Drawn as one run in logical order, every
// numbered item in an Arabic or Hebrew list read "12." where the reader looks
// for ".12".
//
// The stretches are cut where the resolved level changes and put in visual
// order by rule L2, each carrying its own direction for the backend, which
// sets a right-to-left one with its brackets mirrored (L4). A marker that is
// one left-to-right run is returned as no pieces at all, and drawn exactly as
// it always was.
func (l *layouter) markerPieces(face *shape.Face, text string, size style.Unit, rtl bool) ([]markerPiece, style.Unit) {
	if text == "" || (!rtl && !bidi.NeedsAlgorithm(text)) {
		return nil, l.br.Measure(face, text, size)
	}
	dir := bidi.LeftToRight
	if rtl {
		dir = bidi.RightToLeft
	}
	runes := []rune(text)
	levels := bidi.Resolve(runes, dir).Levels()
	type stretch struct{ from, to, level int }
	var stretches []stretch
	for i, lv := range levels {
		if n := len(stretches); n > 0 && stretches[n-1].level == lv {
			stretches[n-1].to = i + 1
			continue
		}
		stretches = append(stretches, stretch{from: i, to: i + 1, level: lv})
	}
	order := make([]int, len(stretches))
	for i, s := range stretches {
		order[i] = s.level
	}
	var pieces []markerPiece
	var x style.Unit
	for _, k := range bidi.VisualOrder(order) {
		s := stretches[k]
		t := string(runes[s.from:s.to])
		pieces = append(pieces, markerPiece{text: t, x: x, rtl: s.level%2 == 1})
		x = x.Add(l.br.Measure(face, t, size))
	}
	if len(pieces) == 1 && !pieces[0].rtl {
		return nil, l.br.Measure(face, text, size)
	}
	return pieces, x
}

// markerInside reports "list-style-position: inside".
func markerInside(b *Box) bool {
	return b.ListItem &&
		ascii.EqualFold(strings.TrimSpace(b.Style.Get("list-style-position")), "inside")
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
//
// The text may be empty and the marker still be there, which is §12.5.1's order
// of precedence: "list-style-image" replaces the marker "list-style-type" would
// have drawn, so a picture with "list-style-type: none" beside it is a picture
// and not nothing. Reading the type first and giving up on an empty one lost
// every such marker — and list-style-021 is that written out, a shorthand
// "list-style: none" and then the image set again by a later longhand.
//
// The face is still needed for a picture, because the gap between a marker and
// its text is half an em of the item's own font whichever the marker is.
func (l *layouter) markerRun(b *Box) (string, *shape.Face, bool) {
	if !b.ListItem {
		return "", nil, false
	}
	text := markerText(b.Style.Get("list-style-type"), b.ListValue)
	if text == "" && b.MarkerImage == nil {
		return "", nil, false
	}
	face, ok := l.fontFor(b)
	if !ok {
		return "", nil, false
	}
	// And in a face that has the character, which the box's own is not always.
	//
	// A marker is text and is chosen the way every other character is: §5.2 of
	// CSS Fonts picks the first available font that can set it, and a family
	// that cannot is passed over. The box's font was used whatever it held, so
	// "list-style-type: square" in a serif face drew that face's notdef — U+25AA
	// is not in the standard PDF fourteen — where every browser draws a small
	// black square from somewhere else.
	//
	// It is the same question a tab stop asks — see tabStop, which wants the face
	// that has U+0020 rather than the one the box named — and a different call,
	// because a marker's fallback has to reach past the declared families into
	// the rest of the library: a bullet is exactly the character a text face is
	// most likely not to have. faceRunsFor is what an ordinary run is set
	// through, so a marker is now set through it too.
	//
	// The coverage test is a short-circuit and not part of the rule.
	// faceRunsFor hands back the primary face when the primary can set the text,
	// so dropping the test changes no rendering — a planted defect that did so
	// moved no test and no reftest. It is here because it is one lookup against
	// a walk, on a path every list item goes through.
	if _, covered := face.GlyphID(firstRune(text)); !covered {
		if runs := l.faceRunsFor(b, face, text); len(runs) > 0 && runs[0].Face != nil {
			face = runs[0].Face
		}
	}
	return text, face, true
}

// firstRune is the character a marker's face is chosen for. A marker is one
// character for every list style that is a bullet and several for every one that
// is a number, and the numbers are digits and letters that any text face has —
// so the first is the one that decides.
func firstRune(s string) rune {
	for _, r := range s {
		return r
	}
	return 0
}

// markerItems is what a block container's inline content starts with, which for
// a list item numbering itself inside is its marker and for everything else is
// nothing.
//
// It seeds both the line building and the intrinsic-width measurement, and it
// has to seed both: a shrink-to-fit list item whose width was measured without
// its marker is narrower than the marker it then draws.
func (l *layouter) markerItems(b *Box, para *bidiBuilder) []inlineItem {
	// The marker belongs to the list item and is drawn by whichever box §12.5.1
	// makes its first inline box — which for an item whose content is
	// block-level is an anonymous block rather than the item. See
	// Box.InsideMarker.
	owner := b
	if b.InsideMarker != nil {
		owner = b.InsideMarker
	}
	item, ok := l.markerItem(owner, para)
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
// Its text is registered with the bidi builder, and used not to be. The reason
// it was left out — "a marker is its own box rather than part of the run of text
// after it" — is about where the marker goes on the line, and leaving it out
// bought that at the price of never resolving the marker's own characters. In a
// right-to-left list "1." is a European number and a common separator, and UAX
// #9 puts the stop on the far side of the digit; unregistered it came out in
// logical order, which is the order nothing reads it in. CSS2/lists/list-style-
// position-024 is that list, checked against the same two characters written as
// text.
func (l *layouter) markerItem(b *Box, para *bidiBuilder) (inlineItem, bool) {
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
	if text == "" {
		// A picture and "list-style-type: none": there is a marker and this
		// path cannot draw it, so what is left is nothing rather than an item
		// carrying no text. An empty item would still spend the half-em gap
		// below and push the item's first line along by it, which is a marker's
		// room with no marker in it.
		return inlineItem{}, false
	}
	size := b.FontSize
	// The metrics of the face the marker is *set* in, which is not always the
	// box's own — see markerRun, which passes over a family that has no glyph
	// for the bullet. §10.8.1 measures leading against "the font", and a run the
	// declared face could not set is not in that font; leadingInFace is the same
	// call an ordinary fallback run is measured by. Without it a square bullet
	// borrowed from another face sat on a line the size of the face that could
	// not draw it.
	above, below := l.leadingInFace(b, face)
	item := inlineItem{
		Text: text, Box: b, Face: face, Size: size,
		// The same half-em the outside marker leaves, spent as width rather than
		// as an offset: here what it separates is the next item on the line.
		//
		// Declared as Room as well as counted in the width, because it is room
		// between two runs and two rules ask about that rather than about the
		// number: a measurement taken again from the text would not know the gap
		// was there, and §8.1 does not shape across it. See Item.Room.
		Width: l.br.Measure(face, text, size).Add(markerGap(size)),
		Room:  markerGap(size),
		Leads: true, Above: above, Below: below,
	}
	if para != nil {
		// In an isolate of its own, with the item's direction: the user
		// agent's "::marker { unicode-bidi: isolate }" of css-lists-3, which
		// markerPieces reads the same way for an outside marker. Without it the
		// marker's characters were resolved with the text after them, and in a
		// right-to-left item whose text begins with a number the stop between
		// the two — a common separator between two European numbers, which
		// rule W4 makes a number — joined "1." to "2024" as one left-to-right
		// number, with the marker on its left, at the end of the line.
		open := []rune{runeLRI}
		if isRTL(b) {
			open = []rune{runeRLI}
		}
		para.Enter(open)
		item.BidiPara, item.BidiStart, item.BidiEnd = para.Add(text)
		para.Leave(open, []rune{runePDI})
	}
	return item, true
}

// markerText renders the marker for a list-style-type and a position.
func markerText(listStyle string, index int) string {
	switch ascii.Lower(strings.TrimSpace(listStyle)) {
	case "none":
		return ""
	case "circle":
		return "◦" // ◦
	case "square":
		return "▪" // ▪
	case "decimal-leading-zero":
		return leadingZero(index) + "."
	case "decimal":
		return strconv.Itoa(index) + "."
	case "lower-alpha", "lower-latin":
		return alphabetic(index, 'a') + "."
	case "upper-alpha", "upper-latin":
		return alphabetic(index, 'A') + "."
	case "lower-roman":
		return ascii.Lower(roman(index)) + "."
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
		// Outside the system's range, which for every alphabetic and additive
		// style is one upwards. CSS Counter Styles §7 gives each of them
		// "decimal" as its fallback, and that is what roman, additive and
		// alphabeticIn beside this already do; this returned the empty string,
		// so a list starting at nought or counting down was marked with a bare
		// full stop and the number was gone.
		return strconv.Itoa(index)
	}
	var out []rune
	for index > 0 {
		index--
		out = append([]rune{first + rune(index%26)}, out...)
		index /= 26
	}
	return string(out)
}

// leadingZero is "decimal-leading-zero": the decimal representation padded to
// two digits.
//
// The pad goes on the digits and the sign goes in front of the pad, which is
// the order CSS Counter Styles §3.1.4 gives — so -1 is "-01" and not "0-1",
// which is what came out of padding the whole thing.
func leadingZero(index int) string {
	digits := strconv.Itoa(index)
	sign := ""
	if index < 0 {
		sign, digits = "-", digits[1:]
	}
	if len(digits) < 2 {
		digits = "0" + digits
	}
	return sign + digits
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

// firstLineStart is how far in from the item's inline-start content edge its
// first line begins, which is what a float on that side pushes along. In a
// left-to-right item that is the left edge; in a right-to-left one the right.
//
// Zero when there is no line at all, and that is not the same as "no float": an
// item with an outside marker and no content of its own has no line box to be
// shortened, so there is nothing here that knows about the float. See the note
// on markerNeedsALine — the two are the same missing line box seen from two
// sides, and this half is the one that can be fixed without deciding how tall an
// empty list item is.
func (l *layouter) firstLineStart(frag *Fragment, origin flow, first firstLine, found, rtl bool) style.Unit {
	if frag == nil {
		return 0
	}
	if len(frag.Lines) > 0 {
		// The item's own line, whose rectangle is the band the floats left it
		// in the item's own content coordinates — so the distance from either
		// content edge is read straight off it.
		line := frag.Lines[0].Rect
		if rtl {
			return style.Max(frag.ContentRect().W.Sub(line.X).Sub(line.W), 0)
		}
		return line.X
	}
	// No line of the item's own, and the marker still has to go where one
	// would have started.
	//
	// An item with no content of its own is not a rare shape: it is how the
	// suite writes "does this property apply to a list item", and a browser puts
	// its bullet where the first line box would have been. That is not the
	// content edge whenever a float is in the way — a block's border box is not
	// displaced by a float, only the lines inside it are — so an empty item
	// beside a one-inch float had its marker an inch to the left of where every
	// renderer puts it, out past the page's own margin.
	//
	// An item whose first line is in a block child asks the same question at
	// that line's height rather than at its own top. The child's line is not
	// read directly, because the child's own margin, border and padding are
	// between it and the item and none of them moves a marker, which belongs to
	// the item: what the floats leave at that height, between the item's own
	// content edges, is the whole of what moves it.
	if origin.ctx == nil {
		return 0
	}
	// The band at the item's first line, measured between the item's own content
	// edges: bandAt clamps to them, so what comes back is where a line inside
	// *this* box would begin.
	lo := origin.x.Add(frag.BorderRect.X).Add(frag.Border.Left).Add(frag.Padding.Left)
	hi := lo.Add(frag.ContentRect().W)
	y := origin.y.Add(frag.BorderRect.Y)
	if found {
		y = y.Add(first.top)
	} else {
		y = y.Add(frag.Border.Top).Add(frag.Padding.Top)
	}
	left, right := origin.ctx.bandAt(y, lo, hi)
	if rtl {
		return style.Max(hi.Sub(right), 0)
	}
	return style.Max(left.Sub(lo), 0)
}

// firstLine is where a subtree's first formatted line is, as distances from the
// top of the border box it was asked about: the top of the line box and the
// baseline on it.
type firstLine struct {
	top, baseline style.Unit
}

// firstLineIn finds the first formatted line of a block container, which §5.12.1
// defines as its own first line box or, when its content is block-level, the
// first formatted line of its first in-flow block-level child.
//
// It is firstBaseline with the line's top kept, and without the box's own marker:
// a list item asking where its marker goes cannot be told "where your marker
// is", which is what firstBaseline answers for an item with no line and a marker
// already placed — and a fragment the layout cache handed back is exactly that.
// A *descendant's* marker is another matter: a nested list item with no content
// still has a marker on a line box, and firstBaseline's reason for counting it is
// this function's too.
func firstLineIn(f *Fragment) (firstLine, bool) {
	inset := f.Border.Top.Add(f.Padding.Top)
	if len(f.Lines) > 0 {
		top := inset.Add(f.Lines[0].Rect.Y)
		return firstLine{top: top, baseline: top.Add(f.Lines[0].Baseline)}, true
	}
	for _, c := range f.Children {
		if c.Box != nil && c.Box.outOfFlow() {
			continue
		}
		at := inset.Add(c.BorderRect.Y)
		if in, ok := firstLineIn(c); ok {
			return firstLine{top: at.Add(in.top), baseline: at.Add(in.baseline)}, true
		}
		if c.Marker != nil {
			top := at.Add(c.Border.Top).Add(c.Padding.Top)
			return firstLine{top: top, baseline: at.Add(c.Marker.At.Y)}, true
		}
	}
	return firstLine{}, false
}
