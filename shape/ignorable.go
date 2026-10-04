package shape

// The zero-width joiner and non-joiner: characters that say something about
// their neighbours and are themselves never drawn.
//
// U+200D ZERO WIDTH JOINER asks for the letters either side of it to be joined
// — in Devanagari, for an explicit half form where the font would otherwise
// have made a conjunct. U+200C ZERO WIDTH NON-JOINER asks for the opposite.
// Neither is a letter, neither has a shape, and both have to be obeyed.
//
// Those two facts pull in opposite directions, and getting either wrong is
// visible:
//
//   - The instruction cannot be obeyed unless the character is *there*. Cursive
//     joining reads them as join-causing and non-joining (arabic.go); the Indic
//     reordering reads a joiner after a virama as a demand for a half form and
//     a non-joiner as a refusal of one (indic.go). Dropping them early would
//     silently ignore what the writer asked for.
//   - Nothing may be drawn for them, and no rule of the font may be broken by
//     one standing in the way. A font's rule that measures what follows a vowel
//     sign is written about letters; a joiner between the sign and the letter
//     must not hide the letter from it, or the rule picks a plainer form than
//     its author meant.
//
// So a joiner is present while the rules that are about it run, invisible to
// the rules that are not, and gone before anything is positioned or drawn.
//
// # What is covered
//
// Unicode's Default_Ignorable_Code_Point property, which covers far more than
// the two join controls: the bidirectional controls, the variation selectors,
// the word joiner, the musical beam marks, the soft hyphen. Every one of them
// is an instruction rather than a letter, and a font that maps one to a visible
// glyph is not asking for it to be drawn — it is saying what it would look like
// if it were, which is a question nobody asked. A font whose rules substitute
// one has asked, and that glyph is drawn (dropUnsubstituted).
//
// Getting this wrong is not subtle. A soft hyphen is written to mark where a
// word *may* break, and is the ordinary way HTML says so; before this was
// handled, a run carrying one had a hyphen drawn in the middle of the word,
// whether it broke there or not.
//
// The table is Unicode's (ignorabletable.go, generated). What a font's rules
// make of each character in it is HarfBuzz's, below, and is the reason the two
// are kept apart: HarfBuzz leaves out of it the characters Unicode lists that
// fonts draw.

// The two join controls. They are named rather than derived because what they
// mean is particular to them: every other format character this package sees is
// simply passed through.
const (
	zeroWidthNonJoiner = 0x200C
	zeroWidthJoiner    = 0x200D
)

// joinerKind is what stands at a buffer position, as far as a lookup's rules
// for stepping over things are concerned.
type joinerKind uint8

const (
	notJoiner joinerKind = iota
	joinerZWJ
	joinerZWNJ
)

// isDefaultIgnorable reports whether Unicode says nothing should be drawn for a
// character.
//
// The ranges are few and sorted, and the first of them is above almost every
// character of ordinary text, so the common answer costs one comparison.
func isDefaultIgnorable(r rune) bool {
	if r < defaultIgnorableRanges[0].lo {
		return false
	}
	lo, hi := 0, len(defaultIgnorableRanges)-1
	for lo <= hi {
		mid := (lo + hi) / 2
		switch {
		case r < defaultIgnorableRanges[mid].lo:
			hi = mid - 1
		case r > defaultIgnorableRanges[mid].hi:
			lo = mid + 1
		default:
			return true
		}
	}
	return false
}

// # Kept, and stepped over
//
// Every one of them is kept in the buffer while the font's substitutions run,
// as HarfBuzz keeps them, and the ones no substitution touched are taken out
// before anything is positioned (dropIgnorables). A lookup matching a rule
// steps over one — unless the rule names its glyph, in which case it is what
// the rule was looking for and is matched. That is HarfBuzz's may_skip and
// may_match: a default-ignorable is "maybe" skipped, and a maybe is decided by
// whether it matches.
//
// Three kinds are not stepped over by every rule, and the differences are what
// the characters are for (ignorableKind):
//
//   - The join controls, as stepsOver says.
//   - U+034F COMBINING GRAPHEME JOINER, where it stands between two marks it
//     kept from being reordered, or at either end of the run; the Mongolian
//     free variation selectors; and the tag characters U+E0020..U+E007F.
//     HarfBuzz calls these hidden: a substitution never steps over one, so a
//     rule written about what follows a letter sees the joiner there, and a
//     flag sequence of tags is ligated whole. Positioning steps over them,
//     and since nothing positions a glyph that is about to be taken out,
//     taking them out first is the same answer.
//
// It was otherwise. Every default-ignorable but the join controls was taken
// out of a run before its buffer was built, on the argument that every lookup
// steps over them, and the join controls before the substitutions ran. Neither
// is so. A non-joiner is not stepped over by a substitution's input — it is
// written between f and i to stop the ligature, and HarfBuzz sets "f\u200Ci"
// as two glyphs where this set one. And Gentium Book Plus's 'ccmp' draws a
// dotless i before a combining grapheme joiner, because its rule for "i
// before a mark above" lists the joiner's glyph among the marks: HarfBuzz
// matches it, and with the joiner gone there was nothing to match.
//
// A syllabic run keeps them too, and for a reason of its own. Whether a
// character breaks a syllable is the syllable model's question, and it can
// answer only if it is given the character; so one written *inside* a
// cluster — between a consonant and its virama, say (क U+00AD ् ष) — breaks
// the syllable here as it does in HarfBuzz, and the orphaned virama gets the
// dotted circle in both. The shaper then drops the ones no substitution
// touched (dropUnsubstituted), as HarfBuzz does.
//
// The Hangul fillers, U+115F, U+1160, U+3164 and U+FFA0, are not among them.
// Unicode marks them default-ignorable, and they are the one part of the
// property a text renderer should not act on: they are letters (category Lo),
// used to write an incomplete syllable — a jamo with a deliberately empty slot
// — and they occupy width on the page. Hiding them collapses the syllable.
// HarfBuzz excludes them for the same reason, and the four shorthand format
// controls U+1BCA0..U+1BCA3 as well, which Duployan fonts draw.

// ignorableKind is what a character nothing is drawn for is to a lookup
// matching a rule. See stepsOver.
type ignorableKind uint8

const (
	notIgnorable ignorableKind = iota
	// ignorableStepped is stepped over by every rule that does not name it.
	ignorableStepped
	// ignorableHidden is not stepped over by a substitution.
	ignorableHidden
	// The join controls, which each feature decides about.
	ignorableZWJ
	ignorableZWNJ
)

// combiningGraphemeJoiner is U+034F, the one hidden character whose standing
// depends on its neighbours.
const combiningGraphemeJoiner = 0x034F

// ignorableKinds says what each character of a run is to a lookup, once the
// run is normalised: nil where none is ignorable, which is every ordinary run.
//
// The joiner is hidden only where it did something. HarfBuzz unhides one that
// stands between two characters and kept no marks from being reordered — the
// one after it is not a mark, or would not have gone before the one before it
// anyway — and leaves one at either end of the run hidden, because it has no
// two neighbours to have kept apart. Its reordering is by the classes it sorts
// marks by, which are reorderClass's.
func ignorableKinds(runes []rune) []ignorableKind {
	var kinds []ignorableKind
	for i, r := range runes {
		k := ignorableKindOf(r)
		if k == notIgnorable {
			continue
		}
		if r == combiningGraphemeJoiner && i > 0 && i+1 < len(runes) {
			if next := reorderClass(runes[i+1]); next == 0 || reorderClass(runes[i-1]) <= next {
				k = ignorableStepped
			}
		}
		if kinds == nil {
			kinds = make([]ignorableKind, len(runes))
		}
		kinds[i] = k
	}
	return kinds
}

// ignorableKindOf is what one character is to a lookup, before its neighbours
// are asked: HarfBuzz's is_default_ignorable, and its hidden set.
func ignorableKindOf(r rune) ignorableKind {
	switch {
	case !hiddenAfterShaping(r), r >= 0x1BCA0 && r <= 0x1BCA3:
		return notIgnorable
	case r == zeroWidthJoiner:
		return ignorableZWJ
	case r == zeroWidthNonJoiner:
		return ignorableZWNJ
	case r == combiningGraphemeJoiner,
		r >= 0x180B && r <= 0x180D, r == 0x180F,
		r >= 0xE0020 && r <= 0xE007F:
		return ignorableHidden
	}
	return ignorableStepped
}

// hiddenAfterShaping reports whether a character is one nothing is drawn for,
// and so has to be taken back out once the rules about it have run.
//
// The Hangul fillers are not among them for the reason given above: they are
// letters and they occupy width.
func hiddenAfterShaping(r rune) bool {
	if !isDefaultIgnorable(r) {
		return false
	}
	switch r {
	case hangulChoseongFiller, hangulJungseongFiller, hangulFiller, halfwidthHangulFiller:
		return false
	}
	return true
}

// dropHiddenBeforeDrawing removes the characters nothing is drawn for from a
// run set in a face that will never shape, the join controls with the rest.
//
// A simple face maps one character to one code and positions nothing, so there
// is no cursive joining for a joiner to ask for and no syllable for it to be
// read inside — and, crucially, no shaping pass afterwards to take it back out
// again. Left in, it falls through to the substitution an unmapped character
// gets and reaches the page as a space, so "let\u200Cter" came out a character
// wider than "letter" and a document using a non-joiner to spell a word
// correctly was set with a gap in it.
func dropHiddenBeforeDrawing(runes []rune, offsets []int) ([]rune, []int) {
	return dropHidden(runes, offsets, hiddenAfterShaping)
}

// dropHidden removes the characters a predicate names, keeping the rest of a run
// and the offsets that map it back to the text.
//
// It returns the input unchanged when there is nothing to drop, which is every
// ordinary string: the scan is one comparison per character against the lowest
// code point the property covers, and allocating a copy of every run to remove
// nothing would cost more than the property is worth.
func dropHidden(runes []rune, offsets []int, hidden func(rune) bool) ([]rune, []int) {
	first := -1
	for i, r := range runes {
		if hidden(r) {
			first = i
			break
		}
	}
	if first < 0 {
		return runes, offsets
	}
	outR := append(make([]rune, 0, len(runes)-1), runes[:first]...)
	outO := append(make([]int, 0, len(offsets)-1), offsets[:first]...)
	for i := first + 1; i < len(runes); i++ {
		if hidden(runes[i]) {
			continue
		}
		outR = append(outR, runes[i])
		outO = append(outO, offsets[i])
	}
	return outR, outO
}

// DrawsNothing reports whether nothing at all is drawn for a character and it
// occupies no width: Unicode's Default_Ignorable_Code_Point, less the Hangul
// fillers, which are letters.
//
// It is exported for the line breaking, which has the same question to answer
// for a different reason — CSS Text §8.2 adds letter-spacing after each
// typographic character unit, and a character nothing is drawn for is not one.
// Answering it there from a hand-written list is what this replaces: the list
// had the characters somebody had met and stopped at U+2064, so the six
// deprecated format controls above it each collected a letter-spacing of their
// own.
func DrawsNothing(r rune) bool { return hiddenAfterShaping(r) }

// The Hangul fillers, named for the reason "Kept, and stepped over" gives.
const (
	hangulChoseongFiller  = 0x115F
	hangulJungseongFiller = 0x1160
	hangulFiller          = 0x3164
	halfwidthHangulFiller = 0xFFA0
)

func joinerKindOf(r rune) joinerKind {
	switch r {
	case zeroWidthJoiner:
		return joinerZWJ
	case zeroWidthNonJoiner:
		return joinerZWNJ
	}
	return notJoiner
}

// stepsOver reports whether a substitution matching a rule may step over the
// glyph at a buffer position rather than compare it: whether it is one of the
// characters nothing is drawn for, and one this kind of matching steps over.
// A glyph it may step over is still matched where the rule names it — the
// callers compare first — which is HarfBuzz's "maybe".
//
// A glyph a substitution has touched is none of these any more, whatever it
// came from: it is the font's now, and it is drawn.
//
// For the join controls the rule is asymmetric, and the asymmetry is the
// specification's rather than a simplification of it:
//
//   - Matching *context* — what a rule requires to precede or follow the glyphs
//     it replaces — always steps over a zero-width joiner, and steps over a
//     non-joiner unless the feature asked to see it. Context is a claim about
//     the letters around a rule, and a joiner is not a letter.
//   - Matching *input* — the glyphs a rule actually replaces — steps over a
//     joiner only where the feature allows it, and never steps over a
//     non-joiner. A non-joiner's whole purpose is to stop the letters either
//     side of it from being joined, and a rule that joined them by stepping
//     over it would do exactly what it was written to prevent.
//
// Which joiner a feature wants to see is the feature's to say, and the two are
// separate. The Indic features ask to see both: half forms and conjuncts are
// precisely what a joiner is written to force or forbid, so their lookups must
// see it. The Myanmar, universal and Arabic features ask to see a zero width
// joiner and leave a non-joiner in their context to be stepped over — which is
// how HarfBuzz enables them, and what lets a rule whose context spans a ZWNJ
// still match. Everything else — the ligatures, the contextual alternates —
// steps over a joiner and not a non-joiner.
//
// A syllabic model says where its join controls are itself (joinerAt), since
// it keeps its own record of what each glyph is.
func (sh shaper) stepsOver(g Glyph, at int, context bool) bool {
	kind := g.ignorable
	if sh.joinerAt != nil {
		switch sh.joinerAt(sh.base() + at) {
		case joinerZWJ:
			kind = ignorableZWJ
		case joinerZWNJ:
			kind = ignorableZWNJ
		}
	}
	if g.substituted {
		return false
	}
	switch kind {
	case ignorableStepped:
		return true
	case ignorableZWJ:
		return context || !sh.manualZWJ
	case ignorableZWNJ:
		return context && !sh.manualZWNJ
	}
	return false
}

// dropGlyphs removes the glyphs a predicate names, keeping the rest in order.
//
// The predicate is by *position* rather than by glyph index, which is the whole
// point: a face commonly maps a join control to the same glyph as the space, or
// to no glyph at all, and a pass that deleted every glyph with that index would
// delete the spaces of the text along with the joiners.
func dropGlyphs(buf []Glyph, drop func(i int) bool) []Glyph {
	return dropGlyphsIf(buf, drop, false, false)
}

// dropUnsubstituted is what a syllabic shaper does with the characters nothing
// is drawn for once the font's rules have run: takes out the ones hidden says
// are such characters — unless a substitution touched the glyph.
//
// A font may give one a shape. Noto Sans Mongolian substitutes the vowel
// separator U+180E with a narrow or a wide space before a final A or E, and
// the gap is what the font is for there; HarfBuzz keeps any such glyph a
// lookup replaced, and hides only the ones left as the character's own
// (_hb_glyph_info_is_default_ignorable, which is false once substituted).
// Taking them all out closed the gap.
//
// The shaper says which way the run is drawn and what is drawn before it on
// the page, which decide whose cluster a glyph taken out leaves behind: see
// dropGlyphsIf.
func (sh shaper) dropUnsubstituted(buf []Glyph, hidden func(i int) bool) []Glyph {
	return dropGlyphsIf(buf, func(i int) bool { return hidden(i) && !buf[i].substituted }, sh.keptAhead, sh.rtl)
}

// dropGlyphsIf takes out the glyphs drop names, as HarfBuzz's
// delete_glyphs_inplace does: each one's cluster merged into a neighbour's
// where no other glyph is left standing for it. See cluster.go.
//
// HarfBuzz does it once the run is in the order it is drawn, so a run drawn
// right to left (rtl) is walked last glyph first: a joiner taken out of a
// right-to-left run gives its cluster to the glyph drawn before it, which is
// the one written after it. keptAhead says something of the same buffer is
// drawn before the run, so that a glyph taken out at its drawn start has its
// cluster kept by that rather than merged into the glyph after it.
func dropGlyphsIf(buf []Glyph, drop func(i int) bool, keptAhead, rtl bool) []Glyph {
	flags := make([]bool, len(buf))
	any := false
	for i := range buf {
		flags[i] = drop(i)
		any = any || flags[i]
	}
	if !any {
		return buf
	}
	if rtl {
		reverseGlyphs(buf)
		for i, j := 0, len(flags)-1; i < j; i, j = i+1, j-1 {
			flags[i], flags[j] = flags[j], flags[i]
		}
	}
	n := 0
	for i := range buf {
		if flags[i] {
			if n > 0 || !keptAhead {
				deleteClusterInPlace(buf, i, n)
			}
			continue
		}
		buf[n] = buf[i]
		n++
	}
	if rtl {
		reverseGlyphs(buf[:n])
	}
	return buf[:n]
}

// dropIgnorables takes out, once a run's substitutions have run, the glyphs of
// the characters nothing is drawn for that no substitution touched: the general
// path's end of the story above. Nothing positions a glyph that is about to
// go, so it goes before positioning; HarfBuzz takes them out after, and its
// positioning steps over them.
func (sh shaper) dropIgnorables(buf []Glyph) []Glyph {
	return sh.dropUnsubstituted(buf, func(i int) bool { return buf[i].ignorable != notIgnorable })
}
