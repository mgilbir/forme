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
// It does nothing to a document with no ideographs in it, which is almost every
// document: the first test is on the characters, and a run of Latin beside a run
// of Latin is two comparisons and no work.
func (l *layouter) insertAutospace(items []inlineItem) []inlineItem {
	for j := range items {
		if !isSpacedRun(items[j]) {
			continue
		}
		first, ok := paragraph.FirstAutospaceBase(items[j].Text)
		if !ok {
			continue
		}
		// The run the gap goes into, and the run that decides whether there is
		// one. They are the same run except where something with no base
		// character stands between.
		gapAt, decides := -1, -1
		for k := j - 1; k >= 0 && decides < 0; k-- {
			switch {
			case items[k].Abs != nil || items[k].Float != nil || items[k].Inset:
				// Out of flow, or an inline box's own edge. Neither is a
				// character, and the edge is exactly the boundary being asked
				// about — see nextSpacedRun, which looks past both for §8.2.
				continue
			case !isSpacedRun(items[k]):
				// A tab, an atomic inline, a forced break: something that is not
				// a character stands between, so there is no boundary here.
				k = -1
			default:
				if gapAt < 0 {
					gapAt = k
				}
				if _, ok := paragraph.LastAutospaceBase(items[k].Text); ok {
					decides = k
				}
			}
		}
		if decides < 0 {
			continue
		}
		gap, ok := l.autospaceBetween(items[decides], items[j], first)
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
func (l *layouter) autospaceBetween(a, b inlineItem, first rune) (style.Unit, bool) {
	last, ok := paragraph.LastAutospaceBase(a.Text)
	if !ok {
		return 0, false
	}
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
