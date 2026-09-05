package layout

import (
	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/style"
)

// text-autospace at a boundary between two runs, CSS Text 4 §8.1.
//
// Japanese and Chinese are set without word spaces, so a Latin word or a number
// dropped into a line of ideographs has nothing separating it from them.
// Typography's answer is a thin space, and the specification's is an eighth of
// the ideographic advance — one eighth of the width of 水 in the ideograph's own
// font, which for every face that has the character is an eighth of the em.
//
// The property's initial value asks for it, so this is not an opt-in: a document
// that says nothing still gets the spacing, because a page of Japanese set
// without it is a page set wrong. The suite's references say the same thing from
// the other side — they write "no-autospace" and put the spacing in by hand,
// with "margin: 0.125ic" on a span.
//
// # Why it is added to the run before the boundary
//
// The same reason letter-spacing's boundary rule adds it there. A line places
// every run itself, from the accumulated advance of the ones before it, so
// widening a run by an eighth of an em moves the next run along by exactly that
// and moves nothing else: the glyphs inside the widened run are drawn where they
// always were, and the gap opens between the two. A backend is handed one
// spacing per run and could not express a one-off gap, and does not need to.
//
// It is spacing and not a character. Nothing is added to the text a reader
// copies out of the page, no line may break at the gap — the runs on either side
// keep whatever opportunity they had — and word-spacing does not see it, because
// there is no word separator there to see.
//
// # Which element decides
//
// The one containing both characters, which is the answer §5.1 gives for a soft
// wrap opportunity and §8.2 for letter-spacing, and the same walk finds it. The
// suite's text-autospace-elements-001 is the fixture: a "no-autospace" div
// holding a "normal" span gets the spacing at the boundaries inside the span,
// because the span is what contains both sides of them.

// visualOrder is the order the items are drawn in, left to right.
//
// The walk below needs it because the gap is between two characters a *reader*
// sees side by side, and after the bidi resolution "side by side" is not "next
// to each other in the text". In "国国<span>אב</span>ג国国" the two logical
// neighbours at the second boundary are ג and 国, and ג is at the far *left* of
// the Hebrew — so a gap opened after it lands in the middle of the word instead
// of at its end. With the whole word in one item there is nothing to get wrong,
// which is why only a word split across an inline boundary shows it.
//
// It is the same answer §8.2's letter-spacing came to: the gap goes between two
// visually adjacent characters. See layout/inlinepaint.go's gapNeighbour, and
// the warning that goes with it — the gap is at a run's right edge whichever way
// the run reads, and a direction-dependent side reads plausibly and is wrong.
//
// The items are segmented per bidi paragraph, because a forced break ends one
// and starts another and LineVisualOrder answers about a single paragraph: given
// two, it would give the second paragraph's items the first's levels and reorder
// across a break that nothing crosses.
func visualOrder(items []inlineItem) []int {
	out := make([]int, 0, len(items))
	lo := 0
	flush := func(hi int) {
		seg := items[lo:hi]
		order := paragraph.LineVisualOrder(seg)
		for i := range seg {
			if order == nil {
				out = append(out, lo+i)
			} else {
				out = append(out, lo+order[i])
			}
		}
		lo = hi
	}
	var para = items[0].Para
	for i, it := range items {
		if it.Para != nil && para != nil && it.Para != para {
			flush(i)
		}
		if it.Para != nil {
			para = it.Para
		}
	}
	flush(len(items))
	return out
}

// leadingBase and trailingBase are the characters at an item's visual left and
// right ends.
//
// For a left-to-right run they are its first and its last, which is what the
// rule was written with. For a right-to-left one they are the other way round:
// the shaper hands back glyphs in visual order and the run is drawn from its
// origin rightwards, so the character a reader sees at the run's right-hand end
// is the *first* one in the text.
func leadingBase(it inlineItem) (rune, bool) {
	if it.Level%2 != 0 {
		return paragraph.LastAutospaceBase(it.Text)
	}
	return paragraph.FirstAutospaceBase(it.Text)
}

func trailingBase(it inlineItem) (rune, bool) {
	if it.Level%2 != 0 {
		return paragraph.FirstAutospaceBase(it.Text)
	}
	return paragraph.LastAutospaceBase(it.Text)
}

// insertAutospace widens the run in front of each ideograph boundary.
//
// The walk is from the *later* side of a boundary rather than the earlier one,
// because the two runs the rule is about need not be next to each other. A
// variation selector gets a run of its own whenever the face for it differs, and
// "国<VS>A" is then three runs with the boundary spanning the middle one: the
// characters that decide are 国 and A, and the gap belongs in front of A. So the
// character walk and the geometry walk are separate — one looks back for the
// last run with a base character in it, the other for the run the gap goes into.
//
// Both walks are in *visual* order. See visualOrder.
//
// It does nothing to a document with no ideographs in it, which is almost every
// document: the first test is on the characters, and a run of Latin beside a run
// of Latin is two comparisons and no work.
func (l *layouter) insertAutospace(items []inlineItem) []inlineItem {
	if len(items) == 0 {
		return items
	}
	order := visualOrder(items)
	for vj := range order {
		j := order[vj]
		if !isSpacedRun(items[j]) {
			continue
		}
		first, ok := leadingBase(items[j])
		if !ok {
			continue
		}
		// The run the gap goes into, and the run that decides whether there is
		// one. They are the same run except where something with no base
		// character stands between.
		gapAt, decides := -1, -1
		for vk := vj - 1; vk >= 0 && decides < 0; vk-- {
			k := order[vk]
			switch {
			case items[k].Abs != nil || items[k].Float != nil || items[k].Inset:
				// Out of flow, or an inline box's own edge. Neither is a
				// character, and the edge is exactly the boundary being asked
				// about — see nextSpacedRun, which looks past both for §8.2.
				continue
			case !isSpacedRun(items[k]):
				// A tab, an atomic inline, a forced break: something that is not
				// a character stands between, so there is no boundary here.
				vk = 0
			default:
				if gapAt < 0 {
					gapAt = k
				}
				if _, ok := trailingBase(items[k]); ok {
					decides = k
				}
			}
		}
		if decides < 0 {
			continue
		}
		last, ok := trailingBase(items[decides])
		if !ok {
			continue
		}
		gap, ok := l.autospaceBetween(items[decides], items[j], last, first)
		if !ok {
			continue
		}
		items[gapAt].Width = items[gapAt].Width.Add(gap)
		// Recorded as well as added, so that a line ending here can leave it
		// out again. See Item.Autospace.
		items[gapAt].Autospace = items[gapAt].Autospace.Add(gap)
	}
	return items
}

// autospaceBetween is what §8.1 puts between the last character of one run and
// the first of the next, or false where it puts nothing.
func (l *layouter) autospaceBetween(a, b inlineItem, last, first rune) (style.Unit, bool) {
	// Cheap first: the characters decide, and for a document with no ideographs
	// in it the answer is no before anything is looked up.
	if !paragraph.IsAutospaceIdeograph(last) && !paragraph.IsAutospaceIdeograph(first) {
		return 0, false
	}
	box := commonAncestor(heldBox(a.Box), heldBox(b.Box))
	if box == nil {
		return 0, false
	}
	as, _ := autospaceOf(box.Style["text-autospace"])
	if !paragraph.AutospaceAt(last, first, as) {
		return 0, false
	}
	// An eighth of the *ideograph's* ideographic advance. Which of the two runs
	// holds the ideograph decides whose font is measured, and it matters: the
	// suite writes its references as "0.125ic" on the element holding the
	// ideographs, and a document that sets its Latin larger than its Japanese
	// would otherwise get a gap sized to the wrong one.
	ideograph := a
	if !paragraph.IsAutospaceIdeograph(last) {
		ideograph = b
	}
	ib := heldBox(ideograph.Box)
	if ib == nil {
		return 0, false
	}
	adv, ok := l.icAdvance(ib)
	if !ok {
		// §5.1.4's fallback for a face with no water ideograph, which is what
		// every Latin face is — one em. It is the same answer the "ic" unit
		// gives, and it is reached here for the same reason.
		adv = ib.FontSize
	}
	return adv.Div(8), true
}
