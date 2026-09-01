package paragraph

import (
	"github.com/mgilbir/forme/bidi"
)

// bidiParagraph is one paragraph with its levels resolved. The alias keeps the
// algorithm's package out of inline.go, which carries enough already.
type bidiParagraph = bidi.Paragraph

// BidiMode is what unicode-bidi asks for.
type BidiMode uint8

const (
	// BidiNormal is the initial value: the box is transparent to the algorithm,
	// and a direction declared on it does nothing at all. That last part
	// surprises everyone once — "direction: rtl" on a <span> with no
	// unicode-bidi is inert by design, because an inline box that changed the
	// direction without opening an embedding would reorder text outside itself.
	BidiNormal BidiMode = iota
	BidiEmbed
	BidiIsolate
	BidiOverride
	BidiIsolateOverride
	BidiPlaintext
)

// BidiBuilder collects the text of an inline formatting context in logical
// order, as the flattening walks it.
//
// It is one rune slice per bidi paragraph rather than one for the context,
// because rule P1 resolves each paragraph on its own: a <br> in the middle of a
// block starts a new one, and running the algorithm across the break would let
// the first word of the document decide the direction of the last.
type BidiBuilder struct {
	Paras [][]rune
	// stack is the formatting codes currently open, so that a paragraph started
	// in the middle of them begins with them again. An isolate that spans a
	// <br> is still open on the line after it.
	stack [][]rune
	// Needed records that something in the context requires the algorithm: a
	// right-to-left character, an Arabic number, or a formatting code. A
	// left-to-right paragraph with none of them resolves to all-zero levels, so
	// the whole of the algorithm can be skipped for it — which is nearly every
	// paragraph nearly every document sets.
	Needed bool
}

func NewBidiBuilder(open []rune) *BidiBuilder {
	p := &BidiBuilder{Paras: [][]rune{nil}}
	if len(open) > 0 {
		p.Enter(open)
	}
	return p
}

// cur is the paragraph being built.
func (p *BidiBuilder) cur() *[]rune { return &p.Paras[len(p.Paras)-1] }

// Add appends a run of text and returns where it landed: which paragraph, and
// the range of runes within it. The paragraph number counts from one; see
// inlineItem.bidiPara.
//
// This runs over every character of every document, including the great majority
// that will turn out not to need the algorithm at all: the paragraph has to be
// built before anything can know whether it was worth building. It costs a
// little under three per cent of the layout of a page of Latin prose, measured,
// and two things that look like improvements are not:
//
//   - Sizing the slice for the run rather than letting append double it makes it
//     three times worse. Every call then reallocates and copies the paragraph so
//     far, because a slice grown to exactly the length it needs has no room for
//     the next word.
//   - Testing each rune as it is copied, instead of scanning the run again for a
//     character that needs the algorithm, is slower too. The second scan stops
//     at the first byte below U+0590 without a call, and the great majority of
//     text is nothing else.
func (p *BidiBuilder) Add(text string) (para, start, end int) {
	if p == nil {
		return 0, 0, 0
	}
	cur := p.cur()
	start = len(*cur)
	for _, r := range text {
		*cur = append(*cur, r)
	}
	if !p.Needed {
		p.Needed = bidi.NeedsAlgorithm(text)
	}
	return len(p.Paras), start, len(*cur)
}

// Object appends the one character an atomic inline counts as.
func (p *BidiBuilder) Object() (para, start, end int) {
	if p == nil {
		return 0, 0, 0
	}
	cur := p.cur()
	start = len(*cur)
	*cur = append(*cur, runeObject)
	return len(p.Paras), start, len(*cur)
}

// Enter opens an inline box's formatting codes, and leave closes them.
func (p *BidiBuilder) Enter(open []rune) {
	if p == nil || len(open) == 0 {
		return
	}
	p.stack = append(p.stack, open)
	cur := p.cur()
	*cur = append(*cur, open...)
	p.Needed = true
}

func (p *BidiBuilder) Leave(open, close []rune) {
	if p == nil || len(open) == 0 {
		return
	}
	p.stack = p.stack[:len(p.stack)-1]
	cur := p.cur()
	*cur = append(*cur, close...)
}

// BreakParagraph ends the current bidi paragraph at a forced line break and
// starts the next one, reopening whatever formatting codes were still open.
func (p *BidiBuilder) BreakParagraph() {
	if p == nil {
		return
	}
	p.Paras = append(p.Paras, nil)
	cur := p.cur()
	for _, open := range p.stack {
		*cur = append(*cur, open...)
	}
}

// LineVisualOrder is rules L1 and L2 over one finished line: the order its items
// are drawn in, left to right.
//
// It returns nil where the order is the logical one, which is what every line of
// a left-to-right document gets and what the caller treats as "no reordering".
func LineVisualOrder(runs []Item) []int {
	para := (*bidi.Paragraph)(nil)
	for _, item := range runs {
		if item.Para != nil {
			para = item.Para
			break
		}
	}
	if para == nil {
		return nil
	}

	// The line's extent in the paragraph. Every item on a line belongs to the
	// same bidi paragraph: a forced break ends both the line and the paragraph,
	// so the two can never be crossed by one line.
	start, end := -1, -1
	for _, item := range runs {
		if item.Para != para {
			continue
		}
		if start < 0 || item.BidiStart < start {
			start = item.BidiStart
		}
		if item.BidiEnd > end {
			end = item.BidiEnd
		}
	}
	if start < 0 || end <= start {
		return nil
	}

	// Rule L1, which is why this is done per line and not once per paragraph: a
	// trailing space takes the paragraph's own direction so that it hangs on the
	// side the next word would have been on.
	lineLevels := para.LineLevels(start, end)

	// An item's level, or levelUnset where it has no characters to get one from:
	// an inline box's own inset, a float marker, the record of an absolutely
	// positioned box.
	//
	// The sentinel is the whole reason this is two passes. Zero is a real
	// embedding level — the left-to-right one — so an item left at zero because
	// nothing had filled it in is indistinguishable from one the algorithm put
	// there, and it lands in the middle of a right-to-left run as an
	// left-to-right island that the reordering then moves to the wrong end. That
	// is exactly what happened to the first item on a line: it had no previous
	// item to copy, so it kept the zero, and a margin at the start of a
	// right-to-left line was placed at the left of the line instead of beside the
	// box it belongs to.
	//
	// 255 cannot collide: UAX #9 caps an embedding level at MaxDepth + 1.
	const levelUnset = -1

	levels := make([]int, len(runs))
	for i, item := range runs {
		switch {
		case item.Para == para && item.BidiStart-start < len(lineLevels):
			levels[i] = lineLevels[item.BidiStart-start]
		case item.InsetLevelKnown && !item.Inset:
			// Kept for a caller that sets the field on something other than an
			// inset; nothing in this engine does.
			levels[i] = item.InsetLevel
		default:
			levels[i] = levelUnset
		}
	}

	// Whatever is left has nothing to say about direction — a float marker, the
	// record of an absolutely positioned box, an inline box's own margin, border
	// and padding — and takes the level of what precedes it, so that it never
	// splits a run in two.
	//
	// An inset used to take a level of its own, worked out by insetSides from
	// the lowest level inside its box, and that is what splitting a run does:
	// dropping a level-2 item into the middle of a level-4 run cuts the run in
	// half, and rule L2 then reverses the halves separately. A span holding
	// three characters at three different levels — which is what an explicit
	// override makes of "c j e" — came out with its letters in an order the
	// algorithm never asked for, and the letters between them moved with it.
	//
	// So an inset takes no part in the reordering at all now. Where it goes is
	// §8.6's question rather than UAX #9's, and placeInsetsBySide answers it
	// afterwards, over the order this returns.
	for i := range runs {
		if levels[i] != levelUnset {
			continue
		}
		if i > 0 {
			levels[i] = levels[i-1]
			continue
		}
		// The first item on the line, with nothing before it to copy. Rule L1
		// gives the paragraph's own embedding level to a line's leading and
		// trailing separators, and this is that position — so the base level is
		// what an item with no characters gets, rather than the zero that a
		// left-to-right paragraph happens to share with it.
		levels[i] = para.Level()
	}

	plain := true
	for i := range levels {
		if levels[i] == levelUnset {
			// Nothing on the line has a level at all, which can only mean the
			// line holds no text. There is nothing to reorder.
			return nil
		}
		if levels[i] != levels[0] {
			plain = false
		}
	}
	if plain && levels[0]&1 == 0 {
		// One even level throughout: the line reads left to right and the
		// reordering is the identity.
		return nil
	}
	return visualOrderAcrossGaps(runs, levels, lineLevels, start, para.Level())
}

// visualOrderAcrossGaps is rule L2 over the items, with the characters that lie
// *between* them kept in the reckoning.
//
// L2 reverses "any contiguous sequence of characters" at or above each level,
// and contiguous is a fact about the text rather than about the items. An
// explicit formatting code is a character with a level of its own and no item to
// carry it: "לום<span style='unicode-bidi: isolate'>x</span>" resolves to levels
// 1 1 1 0 2 0, where the level-0 isolate initiator stands between the Hebrew at
// level 1 and the "x" at level 2 and keeps them in two separate runs. Ordering
// the two items alone sees 1 2, reverses them together as one run at level one
// or higher, and puts the isolated "x" in front of the Hebrew.
//
// So each gap gets a slot of its own, carrying the *lowest* level in it —
// the level that decides which reversals the gap survives — and the slots are
// dropped again once the order is known.
func visualOrderAcrossGaps(runs []Item, levels, lineLevels []int, start, paraLevel int) []int {
	const gap = -1
	slots := make([]int, 0, len(levels)+4)
	owner := make([]int, 0, len(levels)+4)
	prevEnd := -1
	for i, item := range runs {
		if item.Para != nil && item.BidiEnd > item.BidiStart {
			if prevEnd >= 0 && item.BidiStart > prevEnd {
				if lo, ok := lowestLevel(lineLevels, prevEnd-start, item.BidiStart-start); ok {
					slots = append(slots, lo)
					owner = append(owner, gap)
				}
			}
			prevEnd = item.BidiEnd
		}
		slots = append(slots, levels[i])
		owner = append(owner, i)
	}
	if len(slots) == len(levels) {
		// No gap anywhere, which is every ordinary line: the slots are the
		// items and the order is the one the items alone give.
		return bidi.VisualOrder(levels)
	}
	order := bidi.VisualOrder(slots)
	out := make([]int, 0, len(levels))
	for _, k := range order {
		if owner[k] != gap {
			out = append(out, owner[k])
		}
	}
	return out
}

// lowestLevel is the smallest level over a range of a line's characters, and
// whether the range held any.
//
// The smallest because that is the one that decides: L2 reverses a run of
// characters at or above a level, so a gap breaks such a run exactly when
// something in it is below that level, and the lowest is the first to do so.
func lowestLevel(lineLevels []int, from, to int) (int, bool) {
	lo, found := 0, false
	for i := from; i < to && i < len(lineLevels); i++ {
		if i < 0 {
			continue
		}
		if !found || lineLevels[i] < lo {
			lo, found = lineLevels[i], true
		}
	}
	return lo, found
}

// The explicit formatting codes, by name rather than by number: this is the one
// file where a wrong code point would produce text in the wrong order with
// nothing else to say so.
const (
	RuneLRE = '‪'
	RuneRLE = '‫'
	RunePDF = '‬'
	RuneLRO = '‭'
	RuneRLO = '‮'
	RuneLRI = '⁦'
	RuneRLI = '⁧'
	RuneFSI = '⁨'
	RunePDI = '⁩'
	// runeObject is U+FFFC OBJECT REPLACEMENT CHARACTER, which is what an
	// atomic inline counts as in the paragraph: CSS Writing Modes says an
	// element that is not text takes part in the algorithm as one neutral
	// character, so an image between two Hebrew words does not split them.
	runeObject = '￼'
)
