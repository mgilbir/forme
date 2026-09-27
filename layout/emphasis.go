package layout

import (
	"strconv"
	"unicode/utf8"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/internal/charprop"
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// CSS Text Decoration 3 §3: emphasis marks.
//
// # What is drawn
//
// A small mark beside each typographic character unit of the element's text —
// a dot, a circle, a sesame, or a character the document names — half the
// element's font size, in text-emphasis-color, centred on the character's
// advance. Centred on the *character's* advance and not on the advance plus the
// letter-spacing after it: letter-spacing-211 is that sentence as a reftest.
// §3.1 lists the units that get none: word separators and the rest of Unicode's
// separators, punctuation other than the handful of symbols it names, and the
// control, format and unassigned characters. takesEmphasisMark is that list.
//
// The properties inherit, which is what separates them from the decorations
// beside them. A run's marks are its own box's, as its colour is: a <span>
// saying "text-emphasis: none" inside an emphasised paragraph has none.
//
// # Where
//
// §3.4 draws the marks "exactly as if each character was assigned the mark as
// its ruby annotation", centred, on the side text-emphasis-position names —
// over or under in a horizontal typographic mode, right or left in a vertical
// one. The mark's line sits against the text's: its bottom (its descent below
// its baseline) at the text's ascent for a mark over it, and its top at the
// text's descent for a mark under it. Both lines are measured in the element's
// first available face, the text's at the element's size and the mark's at
// half of it, so every mark of an element is on one line whatever face each
// character happens to be set in — the metrics Gecko places them by, and what
// keeps a line of marks straight across a word set in two fonts.
//
// In a vertical typographic mode a mark stands upright — §3.1: "like CJK
// characters, they do not rotate to match the writing mode" — so it is drawn as
// an upright run, centred across the band the mark's line takes. The two
// sideways modes are horizontal typographic modes: their marks are over or under
// and lie along the line with its text.
//
// # Two kinds of run on one vertical line
//
// The text a mark sits against is not always one thing on a vertical line.
// Under "text-orientation: mixed" a paragraph's ideographs stand upright and
// its Latin lies along the line (layout/writingmode.go), and the two reach
// across the line differently: a run lying along it reaches the element's
// ascent over its alphabetic baseline and its descent under it, and an upright
// one is hung from the central baseline and reaches half an em either side of
// that (DrawText.Upright). So a mark is set against the glyphs of its own run
// as they are drawn — reach says how far that is — and measured from the box's
// alphabetic baseline, which every run of the box is laid out on. It is not
// measured from the run's own pen: an upright run's pen is on the central
// baseline, and a run the fallback stack set in another face has its pen on
// its own alphabetic baseline, moved to align the two faces' central ones
// (centralShift). Measured from the box's baseline, every mark lying beside a
// run of one kind is on one line, whatever face each character is in.
//
// # The line's height
//
// §3.4: "The effect of emphasis marks on the line height is the same as for
// ruby text", and CSS Ruby 1 §3.6 adds leading only where the line-height
// leaves no room for the annotation — enough that three such lines one above
// the other would not overlap. See withEmphasis, which also says why that is
// Gecko's reading and not Blink's.
//
// # What the display list says
//
// A mark is DrawEmphasisMark. Its geometry is a DrawText's exactly — the mark's
// character, in a face, at a size, at a pen position, in a colour — and it is
// not a DrawText, because a DrawText is text: its string is what a reader
// copying the page gets back, and "d•o•t" is not the word. It is the reason a
// control character's visible box is not a DrawText, and a text shadow is not.

// DrawEmphasisMark draws one emphasis mark.
//
// Mark is the mark as it is drawn: its character in Mark.Text, set in Mark.Face
// at Mark.Size and Mark.Color, with its pen at Mark.At. A backend shapes and
// places it exactly as it would a DrawText with those fields — Sideways and
// Anticlockwise turn it with its line, and Upright stands it upright on a
// vertical one — and does not put it among the page's text: it is not a
// character of the document.
type DrawEmphasisMark struct {
	Mark DrawText
}

func (DrawEmphasisMark) isOp() {}

// maxEmphasisMarkBytes bounds the character a <string> mark is drawn as.
//
// §3.1 lets a UA "truncate or ignore strings consisting of more than one
// grapheme cluster", and this one truncates to the first. A grapheme cluster
// has no length limit of its own, though — a letter may carry any number of
// combining marks — and the mark is shaped and drawn once for every character
// of the element. So a first cluster past this length is not drawn at all, and
// is reported. It is far past any mark a document means: the longest emoji
// sequences are around thirty bytes.
const maxEmphasisMarkBytes = 64

// The marks §3.1 names, filled and open.
var emphasisShapes = map[string][2]string{
	"dot":           {"\u2022", "\u25E6"},
	"circle":        {"\u25CF", "\u25CB"},
	"double-circle": {"\u25C9", "\u25CE"},
	"triangle":      {"\u25B2", "\u25B3"},
	"sesame":        {"\uFE45", "\uFE46"},
}

// runEmphasis is what layout decided about a box's marks at one size, for
// painting to draw: the mark, the face and size it is set in, which side it
// goes on, and the two lines it is placed against. It is decided in layout
// because the line's height depends on it — see withEmphasis — and the painter
// must place the marks by the numbers the line was made to hold.
type runEmphasis struct {
	mark     string
	face     *shape.Face
	size     style.Unit
	features shape.Features
	// over is the side the marks go on, in the line's own axes: over its
	// text, towards the line box's top before any turn, or under it.
	over bool
	// upright says the marks stand upright on a vertical line.
	upright bool
	// ascent and descent are how far the text reaches over and under its
	// baseline, and markAscent and markDescent the same of the marks' own
	// line. The marks take the band markAscent+markDescent deep beyond the
	// text's reach on their side.
	//
	// ascent and descent are the element's first available face's, which is
	// what a run lying along the line reaches. A run standing upright on a
	// vertical one reaches half of em either side of the central baseline,
	// which is central below the alphabetic one (negative: above it). See
	// reach.
	ascent, descent         style.Unit
	markAscent, markDescent style.Unit
	em, central             style.Unit
	// width is the mark's advance, for a mark lying along the line; along,
	// above and below are its upright extent (see uprightExtent), for one
	// standing on a vertical line.
	width               style.Unit
	along, above, below style.Unit
}

// band is how deep the marks' line is.
func (e *runEmphasis) band() style.Unit { return e.markAscent.Add(e.markDescent) }

// reach is how far a run of the element's text reaches over and under its
// alphabetic baseline: the element's ascent and descent for a run lying along
// the line, and for one standing upright on a vertical line its em box hung
// from the central baseline. The marks go beyond it on their side.
func (e *runEmphasis) reach(upright bool) (over, under style.Unit) {
	if upright && e.upright {
		half := e.em.Div(2)
		return half.Sub(e.central), half.Add(e.central)
	}
	return e.ascent, e.descent
}

type emphasisKey struct {
	b    *Box
	size style.Unit
}

// emphasisOf is a box's emphasis marks, for text set at size; nil where it has
// none, or where none can be drawn. size is the element's font size as its
// text is set — scaled, where text-fit scaled it — and the mark is half of it.
func (l *layouter) emphasisOf(b *Box, size style.Unit) *runEmphasis {
	if b == nil {
		return nil
	}
	raw := ascii.TrimCSSSpace(b.Style.Get("text-emphasis-style"))
	if raw == "" || ascii.EqualFold(raw, "none") {
		return nil
	}
	key := emphasisKey{b, size}
	if e, ok := l.emphases[key]; ok {
		return e
	}
	e := l.readEmphasis(b, raw, size)
	if l.emphases == nil {
		l.emphases = map[emphasisKey]*runEmphasis{}
	}
	l.emphases[key] = e
	return e
}

func (l *layouter) readEmphasis(b *Box, raw string, size style.Unit) *runEmphasis {
	vertical := l.verticalTypography(b)
	mark := l.emphasisMark(b, raw, vertical)
	if mark == "" {
		return nil
	}
	primary, ok := l.fontFor(b)
	if !ok || primary == nil {
		return nil
	}
	e := &runEmphasis{mark: mark, size: size.Div(2), upright: vertical,
		over: emphasisOver(b.Style.Get("text-emphasis-position"), vertical)}
	e.features = l.featuresFor(b)
	e.features.EastAsian |= shape.EastAsianRuby
	e.face = l.emphasisFace(b, primary, mark)
	if e.face == nil {
		return nil
	}
	e.markAscent, e.markDescent = extentsOr(primary, e.size)
	e.ascent, e.descent = extentsOr(primary, size)
	if vertical {
		// An upright character is hung from the middle of the line, one em
		// across it: see DrawText.Upright and uprightExtent. The middle is the
		// central baseline, where an upright run of this box is drawn from.
		// See reach.
		e.em = size
		e.central, _ = l.centralShift(b, primary, size, true)
	}
	m := e.drawn(Point{}, style.RGBA{}, runTurn{sideways: vertical})
	if vertical {
		e.along, e.above, e.below = uprightExtent(m)
	} else {
		e.width = shapedAdvance(m)
	}
	return e
}

// extentsOr is lineExtentsAt, or the four fifths and one fifth of an em
// baselineInFaceAt assumes for a face that states no metrics.
func extentsOr(face *shape.Face, size style.Unit) (ascent, descent style.Unit) {
	if a, d, ok := lineExtentsAt(face, size); ok {
		return a, d
	}
	return size.Mul(0.8), size.Mul(0.2)
}

// shapedAdvance is how far a run's glyphs advance, as a backend draws them.
func shapedAdvance(v DrawText) style.Unit {
	glyphs, _ := ShapedGlyphs(v)
	var w float64
	for _, g := range glyphs {
		w += g.XAdvance
	}
	u, _ := style.FromPx(w * v.Size.Px() / 1000)
	return u
}

// drawn is the mark as a run, at a pen position and in a colour.
func (e *runEmphasis) drawn(at Point, colour style.RGBA, turn runTurn) DrawText {
	return DrawText{
		At: at, Text: e.mark, Face: e.face, Size: e.size, Color: colour,
		Features: e.features,
		Sideways: turn.sideways, Anticlockwise: turn.anticlockwise,
		Upright: e.upright,
	}
}

// verticalTypography reports whether a box's text is in a vertical typographic
// mode: laid out in a vertical-rl or vertical-lr box that was turned. The two
// sideways modes are vertical writing modes and horizontal typographic ones,
// and a box whose turn was refused is laid out across the page.
func (l *layouter) verticalTypography(b *Box) bool {
	for at := b; at != nil; at = at.Parent {
		if mode, turned := l.turnedMode[at]; turned {
			return mode == verticalRL || mode == verticalLR
		}
	}
	return false
}

// emphasisOver reads text-emphasis-position into the side of the line the marks
// go on. "right" is the line's over side in both vertical typographic modes,
// since both turn their lines clockwise (see writingmode.go), and "left" its
// under side; right is also what an omitted second keyword means.
func emphasisOver(raw string, vertical bool) bool {
	over, right := true, true
	for _, w := range ascii.CSSFields(ascii.Lower(raw)) {
		switch w {
		case "under":
			over = false
		case "left":
			right = false
		}
	}
	if vertical {
		return right
	}
	return over
}

// emphasisMark is the character a text-emphasis-style draws, or "" for none.
func (l *layouter) emphasisMark(b *Box, raw string, vertical bool) string {
	vals, errs := css.ParseComponentValues(raw)
	if len(errs) != 0 {
		return ""
	}
	fill, shapeName := 0, ""
	for _, v := range vals {
		if !v.IsToken() {
			return ""
		}
		switch v.Token.Kind {
		case css.Whitespace:
		case css.String:
			mark, ok := firstCluster(v.Token.Value)
			if !ok {
				l.reportOnce("emphasis-mark-long:"+raw, Finding{
					Rule:   RuleLimit,
					Source: AtHTML(offsetOf(b)),
					Message: "the text-emphasis-style string begins with a character more than " +
						strconv.Itoa(maxEmphasisMarkBytes) + " bytes long, so no emphasis marks were drawn",
					Path:     PathOf(b.Element),
					Property: "text-emphasis-style",
				})
				return ""
			}
			return mark
		case css.Ident:
			switch w := ascii.Lower(v.Token.Value); w {
			case "filled":
			case "open":
				fill = 1
			default:
				shapeName = w
			}
		default:
			return ""
		}
	}
	if shapeName == "" {
		// §3.1: "If only filled or open is specified, the shape keyword
		// computes to circle in horizontal typographic modes and sesame in
		// vertical typographic modes."
		shapeName = "circle"
		if vertical {
			shapeName = "sesame"
		}
	}
	marks, ok := emphasisShapes[shapeName]
	if !ok {
		return ""
	}
	return marks[fill]
}

// firstCluster is the first grapheme cluster of s, and false where it is longer
// than maxEmphasisMarkBytes.
//
// It reads no more of s than it needs to. A cluster boundary is decided by the
// characters before it and the one after it, so a boundary inside a prefix that
// ends between two characters is a boundary of the whole string, and the
// prefix need only reach one character past the bound.
func firstCluster(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	end := len(s)
	if end > maxEmphasisMarkBytes {
		end = maxEmphasisMarkBytes + 1
		for end < len(s) && !utf8.RuneStart(s[end]) {
			end++
		}
	}
	prefix := s[:end]
	cut := end
	if bounds := segment.Boundaries(nil, prefix); len(bounds) > 0 {
		cut = bounds[0]
	} else if end < len(s) {
		// No boundary in the prefix, and more after it: the cluster runs past
		// the bound.
		return "", false
	}
	if cut > maxEmphasisMarkBytes {
		return "", false
	}
	return s[:cut], true
}

// emphasisFace is the face that sets a mark: the element's own where it has the
// mark, and otherwise the one font fallback finds for it, as for any other
// character — §3.1 lets a UA use "a font known to be good for emphasis marks"
// where the element's has none, and the fallback set is the fonts this engine
// knows.
//
// None, where no face has it or no one face has all of it, and that is
// reported: a mark drawn as a missing-glyph box beside every character is not
// what anybody asked for, so the marks are not drawn.
func (l *layouter) emphasisFace(b *Box, primary *shape.Face, mark string) *shape.Face {
	if !hasVisibleControl(mark) {
		if runs := l.faceRunsFor(b, primary, mark); len(runs) == 1 && runs[0].Face != nil &&
			!missesVisible(runs[0].Face, mark) {
			return runs[0].Face
		}
	}
	l.reportOnce("emphasis-mark-missing:"+mark+"\x00"+b.Style.Get("font-family"), Finding{
		Rule:   RuleGlyphMissing,
		Source: AtHTML(offsetOf(b)),
		Message: "no face has a glyph for the emphasis mark " + quoteValue(mark) +
			", so the marks were not drawn",
		Path:     PathOf(b.Element),
		Property: "text-emphasis-style",
	})
	return nil
}

// withEmphasis grows how far an inline box reaches over or under its baseline
// to hold its emphasis marks.
//
// §3.4: "The effect of emphasis marks on the line height is the same as for
// ruby text", and CSS Ruby 1 §3.6 says what that is. The annotation does not
// count towards the line's height as long as the line-height leaves room for
// it; where it does not — "if the line-height specified ... is less than the
// distance between the top of the top ruby annotation container and the bottom
// of the bottom ruby annotation container" — leading is added "on the
// appropriate side(s) ... such that if a block consisted of three lines each
// containing ruby identical to this, none of the ruby containers would
// overlap". So the question is the box's *total* leading against the marks'
// band, and not the leading on the marks' side alone: a line-height half again
// the font's leaves the band's worth of leading, split either side, and a
// line of marks over the line below it does not reach the text of the line
// above.
//
// Which side the missing leading goes on is Gecko's nsLineLayout::
// AdjustLeadings, which is the implementation of that sentence: the side that
// already has what the marks need gives its surplus up and the other side gets
// the rest, and where neither has, the marks' side takes the band and the
// other none. Gecko passes all thirteen of the suite's
// text-emphasis-line-height tests. Blink grows each side on its own to hold
// what is over it, which is more than §3.6 asks for wherever the half-leading
// is short and the total is not, and fails them.
//
// On a vertical line the box's text may hold both kinds of run (see the note
// at the top of this file), and the marks of each are beyond its own reach:
// the box's leading has to hold both, so it is the larger of what each kind
// asks on each side, for every kind the box's text holds (emphasisKinds).
//
// It is asked of the box and not of each run, and that is not a loss of
// precision anybody could see. Every line a box's text is on carries the box's
// own leading — its strut, for a block, and the edges of its inline box for
// any other — so a run of one kind on a line is already under the leading of
// every kind its box holds, and a run asking for less would change nothing.
func (l *layouter) withEmphasis(b *Box, size, above, below style.Unit) (style.Unit, style.Unit) {
	e := l.emphasisOf(b, size)
	if e == nil {
		return above, below
	}
	if !e.upright {
		return e.lead(false, above, below)
	}
	upright, sideways := l.emphasisKinds(b)
	switch {
	case upright && sideways:
		a1, b1 := e.lead(true, above, below)
		a2, b2 := e.lead(false, above, below)
		return style.Max(a1, a2), style.Max(b1, b2)
	case upright:
		return e.lead(true, above, below)
	}
	return e.lead(false, above, below)
}

// lead is withEmphasis for one kind of run: the text reaching reach(upright)
// over and under the baseline, and the box above and below it.
func (e *runEmphasis) lead(upright bool, above, below style.Unit) (style.Unit, style.Unit) {
	over, under := e.reach(upright)
	// The leading either side of the text, over its baseline first, and what
	// the marks ask of each.
	start, end := above.Sub(over), below.Sub(under)
	needStart, needEnd := e.band(), style.Unit(0)
	if !e.over {
		needStart, needEnd = 0, e.band()
	}
	missing := needStart.Add(needEnd).Sub(start.Add(end))
	if missing <= 0 {
		return above, below
	}
	switch {
	case needStart < start:
		end = end.Add(missing)
	case needEnd < end:
		start = start.Add(missing)
	default:
		start, end = needStart, needEnd
	}
	return over.Add(start), under.Add(end)
}

// emphasisKinds is which kinds of run a box's text holds on a vertical line
// — standing upright, lying along it — counting only the characters that take
// a mark, since those are what the marks are set against. A box holding no
// marked character is counted as lying along the line, which is what the
// marks of an empty line were reserved for before a line held two kinds.
//
// It is the box's own text for a text box and everything under it for any
// other, and it is kept per box, so each box is asked once and the walk under
// a block is linear in its text.
func (l *layouter) emphasisKinds(b *Box) (upright, sideways bool) {
	k := l.kindsUnder(b)
	if k == 0 {
		return false, true
	}
	return k&kindUpright != 0, k&kindSideways != 0
}

const (
	kindUpright uint8 = 1 << iota
	kindSideways
)

func (l *layouter) kindsUnder(b *Box) uint8 {
	if k, ok := l.runKinds[b]; ok {
		return k
	}
	var k uint8
	if b.IsText() {
		k = l.textKinds(b)
	} else {
		for _, c := range b.Children {
			k |= l.kindsUnder(c)
		}
	}
	if l.runKinds == nil {
		l.runKinds = map[*Box]uint8{}
	}
	l.runKinds[b] = k
	return k
}

// textKinds is kindsUnder for a text box: which way each of its marked
// characters faces, asked as its run will be (uprightRun).
func (l *layouter) textKinds(b *Box) uint8 {
	facing, vertical := l.facingOf(b)
	switch {
	case vertical && l.combinesText(b):
		// A text-combine-upright composition is one upright character, U+FFFC,
		// which takes a mark, whatever its text is. See paintRun.
		return kindUpright
	case !vertical || facing == orientationSideways:
		return kindSideways
	case facing == orientationUpright:
		return kindUpright
	}
	var k uint8
	bounds := segment.Boundaries(nil, b.Text)
	for i, start := 0, 0; start < len(b.Text); i++ {
		end := len(b.Text)
		if i < len(bounds) {
			end = bounds[i]
		}
		if unit := b.Text[start:end]; takesEmphasisMark(unit) {
			if paragraph.UprightInMixed(unit) {
				k |= kindUpright
			} else {
				k |= kindSideways
			}
		}
		start = end
	}
	return k
}

// leadsByRun reports whether a run of b's text in face measures its leading
// for itself rather than taking its box's (runLeading): where the fallback
// stack set it in a face the box did not declare, under "line-height:
// normal". See leadingInFace.
func (l *layouter) leadsByRun(b *Box, face, boxFace *shape.Face) bool {
	return face != nil && face != boxFace && usesNormalLineHeight(b)
}

// runLeading is how far one run of b's text, in face, reaches over and under
// the baseline it is laid out on, marks included.
//
// The order is the point. A run the fallback stack set in another face reaches
// that face's extents (leadingInFace), moved across the line where its central
// baseline is aligned with the box's (centralShift); the marks are then asked
// what they need beyond the element's text, measured from the box's baseline,
// against extents that are where the run is. Asked before the move, the marks'
// leading was measured against extents the run does not have, and a run whose
// face reaches less far under the baseline than the element's took leading on
// the wrong side.
func (l *layouter) runLeading(b *Box, face *shape.Face) (above, below style.Unit) {
	above, below = l.textLeadingInFaceAt(b, face, b.FontSize)
	// On a vertical line that run is aligned by its central baseline and not
	// its alphabetic one, so its extents sit that much further down the
	// frame. Zero on a horizontal line. See centralShift.
	_, moved := l.centralShift(b, face, b.FontSize, false)
	above, below = above.Sub(moved), below.Add(moved)
	return l.withEmphasis(b, b.FontSize, above, below)
}

// takesEmphasisMark reports whether a typographic character unit gets a mark:
// every one but those §3.1 lists.
func takesEmphasisMark(unit string) bool {
	r, n := utf8.DecodeRuneInString(unit)
	if r == utf8.RuneError && n <= 1 {
		return false
	}
	switch c := charprop.Of(r); {
	case c&charprop.Z != 0 || isWordSeparator(r):
		// "Word separators or other characters that belong to the Unicode
		// separator classes (Z*). (But note that emphasis marks are drawn for a
		// space that combines with any combining characters.)" The word
		// separators CSS Text 3 §4.3 lists are all separators or punctuation,
		// so the second question changes no answer; it is asked so that the
		// rule reads as §3.1 writes it.
		for _, m := range unit[n:] {
			if charprop.Is(m, charprop.M) {
				return true
			}
		}
		return false
	case c&charprop.P != 0:
		// "Punctuation—specifically, any characters that belong to the
		// Unicode P* general category and do not NFKD normalize to" the
		// symbols named.
		return markedPunctuation[r]
	case c&(charprop.Cc|charprop.Cf|charprop.Cn) != 0:
		return false
	}
	return true
}

// markedPunctuation is the punctuation §3.1 marks: the P* characters whose NFKD
// form is one of the fifteen symbols it lists, as UnicodeData.txt 17.0.0
// decomposes them. TestMarkedPunctuationIsTheNFKDList derives it from the file.
var markedPunctuation = map[rune]bool{
	// The fifteen themselves.
	'#': true, '%': true, '&': true, '@': true, '\u00A7': true, '\u00B6': true,
	'\u0609': true, '\u060A': true, '\u066A': true, '\u2030': true, '\u2031': true,
	'\u204A': true, '\u204B': true, '\u2053': true, '\u303D': true,
	// The small and fullwidth forms that decompose to four of them.
	'\uFE5F': true, '\uFE60': true, '\uFE6A': true, '\uFE6B': true,
	'\uFF03': true, '\uFF05': true, '\uFF06': true, '\uFF20': true,
}

// unitSpan is where one typographic character unit of a run is along it: from
// lo to hi, measured from the run's pen position, without the letter-spacing
// after it.
type unitSpan struct {
	text   string
	lo, hi style.Unit
	have   bool
}

func (s *unitSpan) add(lo, hi style.Unit) {
	if !s.have {
		s.lo, s.hi, s.have = lo, hi, true
		return
	}
	s.lo, s.hi = min(s.lo, lo), max(s.hi, hi)
}

// unitSpans is where each typographic character unit of a run is, from the
// glyphs a backend draws for it and the pen that draws them: each glyph's
// advance, and after the last glyph of a shaping cluster the letter-spacing of
// every unit that ends in it (the comparison's spacingAfterGlyph is the
// reference reading of that).
//
// A unit is a grapheme cluster, as for letter-spacing. A shaping cluster that
// holds several — a ligature — is shared between them evenly, in the order
// they are drawn; one unit drawn as several clusters — a Thai letter with its
// marks — spans them all.
//
// It is linear in the text and its glyphs: what each glyph asks — which unit
// its cluster begins in, where the cluster ends, how many spacings fall inside
// it — is read from a table over the text's bytes, built in one pass each,
// rather than searched for. A mark is drawn per character, so a search per
// glyph would make a long run of marked text cost more per character the
// longer it is. See TestMarksAreLinearInTheText.
func unitSpans(v DrawText) []unitSpan {
	if v.Face == nil || v.Text == "" {
		return nil
	}
	text := ShapedText(v)
	if v.Upright {
		text = v.Text
	}
	glyphs, _ := ShapedGlyphs(v)
	if len(glyphs) == 0 {
		return nil
	}
	n := len(text)
	// unitOf is the unit each byte of the text is in.
	bounds := segment.Boundaries(nil, text)
	spans := make([]unitSpan, len(bounds)+1)
	unitOf := make([]int, n)
	for k, start := 0, 0; k < len(spans); k++ {
		end := n
		if k < len(bounds) {
			end = bounds[k]
		}
		spans[k].text = text[start:end]
		for b := start; b < end; b++ {
			unitOf[b] = k
		}
		start = end
	}
	// endAfter is where the shaping cluster holding a byte ends: the next
	// byte some glyph's cluster begins at.
	begins := make([]bool, n)
	for _, g := range glyphs {
		if g.Cluster >= 0 && g.Cluster < n {
			begins[g.Cluster] = true
		}
	}
	endAfter := make([]int, n)
	for b, next := n-1, n; b >= 0; b-- {
		endAfter[b] = next
		if begins[b] {
			next = b
		}
	}
	// spacedBefore counts the characters before a byte that letter-spacing
	// follows.
	var spacedBefore []int
	if v.CharSpacing != 0 {
		spaced := spacingAfterOffsets(text)
		spacedBefore = make([]int, n+1)
		for b := 0; b < n; b++ {
			spacedBefore[b+1] = spacedBefore[b]
			if spaced[b] {
				spacedBefore[b+1]++
			}
		}
	}
	scale := v.Size.Px() / 1000
	var pen style.Unit
	for i, g := range glyphs {
		adv, _ := style.FromPx(g.XAdvance * scale)
		if v.Upright {
			adv, _ = style.FromPx(-g.YAdvance * scale)
		}
		c := g.Cluster
		if c < 0 || c >= n {
			pen = pen.Add(adv)
			continue
		}
		ce := endAfter[c]
		first, last := unitOf[c], unitOf[ce-1]
		units := last - first + 1
		for k := 0; k < units; k++ {
			// The k-th unit of the cluster in logical order is drawn k-th along
			// the line, or k-th from its far end where the run reads right to
			// left.
			j := k
			if v.RTL && !v.Upright {
				j = units - 1 - k
			}
			spans[first+k].add(pen.Add(adv.Mul(float64(j)/float64(units))),
				pen.Add(adv.Mul(float64(j+1)/float64(units))))
		}
		pen = pen.Add(adv)
		if spacedBefore != nil && (i == len(glyphs)-1 || glyphs[i+1].Cluster != c) {
			if k := spacedBefore[ce] - spacedBefore[c]; k > 0 {
				pen = pen.Add(v.CharSpacing.Mul(float64(k)))
			}
		}
	}
	return spans
}

// marks is a run's marks, each placed: one per unit that takes a mark,
// centred on it along the line and set on the marks' line beside it.
//
// spans are the run's units along the line from its pen, and at is the pen on
// the box's alphabetic baseline — the run's own pen before centralShift moved
// it across the line, which is where every mark of the box is measured from.
// upright is whether the run stands upright on a vertical line, which decides
// what it reaches (reach).
func (e *runEmphasis) marks(spans []unitSpan, at Point, upright bool, colour style.RGBA,
	turn runTurn) []DrawText {
	over, under := e.reach(upright)
	var out []DrawText
	for _, s := range spans {
		if !s.have || !takesEmphasisMark(s.text) {
			continue
		}
		centre := s.lo.Add(s.hi).Div(2)
		var x, y style.Unit
		if e.upright {
			// Centred across the band the marks' line takes, which is the
			// middle of the upright mark's own extent: see uprightExtent,
			// which measures above towards the line's over side.
			mid := style.Unit(0).Sub(over.Add(e.band().Div(2)))
			if !e.over {
				mid = under.Add(e.band().Div(2))
			}
			x, y = centre.Sub(e.along.Div(2)), mid.Sub(e.below.Sub(e.above).Div(2))
		} else {
			y = style.Unit(0).Sub(over.Add(e.markDescent))
			if !e.over {
				y = under.Add(e.markAscent)
			}
			x = centre.Sub(e.width.Div(2))
		}
		p := placeRun(Rect{X: x, Y: y}, at, turn)
		out = append(out, e.drawn(Point{X: p.X, Y: p.Y}, colour, turn))
	}
	return out
}
