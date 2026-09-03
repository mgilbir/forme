package paragraph

import (
	"github.com/mgilbir/forme/segment"
	"unicode"

	"unicode/utf8"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What letter-spacing and word-spacing add to a run.
//
// CSS Text §8 states both over typographic character units and word-separator
// characters, which are questions about the text. Which values a particular box
// asked for is a question about the document, and stays with the layout engine.

// TextSpacing is the pair of extra advances, in layout units.
//
// The zero value is what "normal" means for both, which is what makes it cheap:
// a document that sets neither compares equal to the zero key and shares one
// cache entry per word with every other such document.
type TextSpacing struct {
	// Letter is added after every typographic character unit, including the last
	// one of a run. That is what CSS Text §8.2 says and what browsers do, and it
	// is why "Letter-spacing: 1em" on a single word leaves a gap after it.
	Letter style.Unit
	// Word is added to every Word-separator character.
	Word style.Unit
}

// SpacingAdvance is what the two properties add to a run of text.
//
// It walks the string once, without decoding it into runes: a text node is
// untrusted and arbitrarily large, and counting characters through a []rune
// would buffer four bytes per character to answer a question about a length.
// utf8.RuneCountInString does the same walk without the copy.
func SpacingAdvance(text string, sp TextSpacing) style.Unit {
	var out style.Unit
	if sp.Letter != 0 {
		out = out.Add(sp.Letter.Mul(float64(SpacedUnits(text))))
	}
	if sp.Word != 0 {
		out = out.Add(sp.Word.Mul(float64(countWordSeparators(text))))
	}
	return out
}

// SpacedUnits counts what letter-spacing is added between.
//
// CSS Text §8.2 spaces "adjacent typographic character units", and a code point
// the shaper drops before choosing any glyph is not one of them: it draws
// nothing and it is in the text only because the text is what a reader copies
// out of the page. Counting one gave a bidi override an em of its own under
// "letter-spacing: 1em", which pushed every letter after it along — the suite
// writes that document eight times over, as the second half of each
// bidi-text/bidi-00N pair.
//
// The unit is the grapheme cluster, which is what CSS Text §2 means by
// "typographic character unit" and is not what this counted for a long time. It
// counted runes, and a rune is not a unit: a Thai letter carries its vowel sign
// and its tone mark, a Khmer consonant carries the vowel that follows it, and a
// spacing inserted between a base and a mark that belongs to it moves the mark
// off the letter it is drawn on. The suite's letter-spacing-bengali-yaphala-001
// is a syllable of three code points asking to stay one.
//
// # Cursive tracking
//
// §8.2 again: "spacing must not be inserted between the characters of a cursive
// script, as it would break the cursive connection". So a character of one is
// not a unit spacing goes after — not because it joins to what follows, which
// depends on the pair, but because it is *in* such a script, which does not.
// The suite states the rule that way and not the other:
// letter-spacing-cursive-001 asks for Arabic set with and without a
// letter-spacing to render identically, and "مرحباً" has an unjoined pair in the
// middle of it.
//
// It applies to the character rather than to its neighbours, so an Arabic word
// followed by a space gets no spacing after its last letter and one after the
// space. letter-spacing-cursive-002 is exactly that arithmetic — two Arabic
// words, "letter-spacing: 1em", and a reference that inserts one em and not two.
func SpacedUnits(text string) int {
	n := 0
	scanCursiveTracking(text, func(_, _ int, suppressed bool) {
		if !suppressed {
			n++
		}
	})
	return n
}

// IsCursiveScript reports whether a character belongs to a script whose letters
// join. See shape.InCursiveScript for why it is the script and not the pair.
func IsCursiveScript(r rune) bool { return shape.InCursiveScript(r) }

// CursiveTrackingSuppresses reports whether §8.2 takes the letter-spacing off a
// run of text entirely, so that it carries none after any of its characters —
// the last one included.
//
// It is asked of a run of text and answers false for the empty string, which is
// not one. An inline box's edge is not a character and an atomic inline is a
// character unit letter-spacing goes after like any other; neither is what this
// rule is about.
func CursiveTrackingSuppresses(text string) bool {
	return text != "" && SpacedUnits(text) == 0
}

// scanCursiveTracking walks the characters letter-spacing could go after and
// says of each whether §8.2 forbids it, so that the count and the cut cannot
// answer differently.
//
// A combining mark is the case neither of them could answer alone: Unicode's
// joining table names the letters of the cursive scripts and not the marks
// written on them, so a fathatan reads as an ordinary character and collects a
// spacing of its own — which is what left "مرحباً" ten pixels wider than the same
// word without a letter-spacing, in letter-spacing-cursive-001. So a mark takes
// the answer of the base it sits on.
//
// It takes it *only* from a cursive base, which keeps the change to the script
// this rule is about. A mark on a Latin letter still counts as a unit of its
// own, as it always has — that is the grapheme-cluster question SpacedUnits
// records as a separate one, and it is still separate.
func scanCursiveTracking(text string, fn func(start, last int, suppressed bool)) {
	cursive := false
	bounds := segment.Boundaries(nil, text)
	for k, start := 0, 0; start < len(text); k++ {
		end := len(text)
		if k < len(bounds) {
			end = bounds[k]
		}
		// The cluster's own last character, which is where the spacing goes,
		// and its base, which is what decides whether the spacing goes there at
		// all. A cluster of nothing but characters that draw nothing is not a
		// unit and is passed over entirely.
		last := -1
		for i, r := range text[start:end] {
			switch {
			case IsDefaultIgnorable(r):
				continue
			case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
				// A mark does not decide the script — it inherits the base's,
				// which the base has already set — but it can be the last thing
				// in the cluster, and the spacing goes after the whole of it.
				last = start + i
				continue
			}
			cursive = IsCursiveScript(r)
			last = start + i
		}
		if last >= 0 {
			fn(start, last, cursive)
		}
		start = end
	}
}

// AllIgnorable reports whether a run is nothing but characters nothing is drawn
// for — the bidi controls, the joiners, the invisible operators.
//
// Such a run is not a run of text, and the question §8.2 asks of the last item
// on a line is what spacing hangs past the end of it. For this one the answer is
// not "none": there is no character here for a spacing to follow, so what hangs
// is the spacing after the letter in *front* of it — and that is the same
// number, because a run is cut wherever the letter-spacing changes, so the
// letter in front carries what this run declares.
//
// Reading it as suppressed instead is what the cursive clause of TrailingSpacing
// gives it, and it is wrong in the expensive direction: the line counts a
// spacing it does not draw. A float shrink-wrapped around two letters with a
// formatting character behind them came out one tracking wider than its text,
// and then broke the second letter onto a line of its own to fit the width it
// had just been given. The suite writes it as letter-spacing-202.
//
// The empty string is not one of these. An item with no text at all is an
// inline box's edge or an atomic inline, and both are answered elsewhere and
// differently; saying yes here would take a spacing off a run that has one.
func AllIgnorable(text string) bool {
	if text == "" {
		return false
	}
	for _, r := range text {
		if !IsDefaultIgnorable(r) {
			return false
		}
	}
	return true
}

// SpacingAfterOffsets is the byte offsets of the characters after which §8.2's
// letter-spacing is added — the last character of each typographic character
// unit that carries one.
//
// It is the same answer SpacingAfter gives, indexed the way a caller that has
// byte offsets rather than rune positions needs it: the reftest comparison,
// which walks shaped glyphs and has each one's offset into the text it came
// from. See layout's spacingAfterGlyph.
func SpacingAfterOffsets(text string) map[int]bool {
	out := map[int]bool{}
	scanCursiveTracking(text, func(_, last int, suppressed bool) {
		if !suppressed {
			out[last] = true
		}
	})
	return out
}

// SpacingAfter reports, rune by rune, whether §8.2's letter-spacing is added
// after that rune — the same question SpacedUnits answers in the aggregate, for
// a caller that has to place each character and not only measure the run.
//
// It exists because there is such a caller and it got it wrong by counting
// runes: the reconstruction that turns a run of rectangle glyphs back into the
// rectangles it draws, which the reftest comparison uses to compare a run of
// Ahem against a fill of the same square. Adding a tracking after a character
// that has none put those rectangles a tracking further along for every
// formatting character in front of them — twenty-six of them in the suite's
// letter-spacing-202, which is thirteen ems, and a picture the engine never
// drew. See layout/blockglyph_test.go.
//
// The answer is per rune and the slice is indexed by rune, not by byte: the
// caller is already walking runes to place glyphs.
func SpacingAfter(text string) []bool {
	spaced := map[int]bool{}
	scanCursiveTracking(text, func(_, last int, suppressed bool) { spaced[last] = !suppressed })
	out := make([]bool, 0, len(text))
	for i := range text {
		out = append(out, spaced[i])
	}
	return out
}

// IsDefaultIgnorable reports whether nothing is drawn for a character and it
// occupies no width, which for §8.2 is the question "is this a typographic
// character unit letter-spacing goes after".
//
// It is Unicode's Default_Ignorable_Code_Point, less the Hangul fillers, which
// are letters. The answer comes from the shaping package, which holds the
// property as a generated table, because this used to be a list written here by
// hand: it had the characters somebody had met — the bidi controls, the joiners,
// the word joiner and the invisible operators — and stopped at U+2064, so each
// of the six deprecated format controls above it collected a letter-spacing of
// its own and a document using one came out wider than it should be.
func IsDefaultIgnorable(r rune) bool { return shape.DrawsNothing(r) }

// IsBidiControl reports whether a character is one of Unicode's Bidi_Control
// characters: a mark whose whole function is to instruct the bidirectional
// algorithm.
//
// It is narrower than IsDefaultIgnorable and the difference matters. Both sets
// hold characters nothing is drawn for, and only these are *transparent to the
// text*: a zero-width space is default-ignorable and is a character that stands
// between its neighbours — §4.1.1 does not collapse two spaces across one — while
// a bidi control is not there at all as far as the text is concerned, and two
// spaces with one between them are adjacent and collapse.
//
// The suite tests both, in opposite directions and with the same shape of
// document. See Piece.ZeroWidth for the other half.
func IsBidiControl(r rune) bool {
	switch r {
	case 0x061C, // arabic letter mark
		0x200E, 0x200F: // left-to-right and right-to-left marks
		return true
	}
	return (r >= 0x202A && r <= 0x202E) || // the embeddings and overrides
		(r >= 0x2066 && r <= 0x2069) // the isolates
}

// countWordSeparators counts the characters word-spacing applies to.
//
// CSS Text §8.1 names a short list of word-separator characters. The two that
// occur in real documents are the ordinary space and the no-break space; the
// remaining four are Ethiopic and Aegean word separators, which are counted too
// because leaving them out would make the property silently do nothing in the
// documents that need it most.
func countWordSeparators(text string) int {
	n := 0
	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		i += size
		if isWordSeparator(r) {
			n++
		}
	}
	return n
}

// SplitAtCursiveTracking cuts text where §8.2's cursive tracking begins or ends,
// so that every run either takes letter-spacing after each of its characters or
// takes none.
//
// It exists for the backend and not for the rule. A display list carries one
// letter-spacing per run — an advance added after every glyph — so a run holding
// an Arabic letter beside a Latin one cannot say that only one of them is
// followed by a gap. Cutting at the change makes each run uniform, and the
// question the drawing has to answer becomes the question this file answers.
//
// It is the same argument, and the same shape, as the cut §8.1's ideograph
// spacing needs. See SplitAtAutospace.
//
// Nothing is returned where there is nothing to cut, which is every run of every
// document that does not mix a cursive script with another — so a caller pays a
// scan and no allocation.
func SplitAtCursiveTracking(text string) []string {
	var out []string
	start, prev, havePrev := 0, false, false
	scanCursiveTracking(text, func(at, _ int, suppressed bool) {
		// The unit's *start*, because that is where the run is cut: a piece
		// ends before the first character of the unit whose answer differs. The
		// spacing goes after a unit's last character and the cut goes before
		// its first, which is why the scan reports both.
		if havePrev && suppressed != prev {
			out = append(out, text[start:at])
			start = at
		}
		prev, havePrev = suppressed, true
	})
	if out == nil {
		return nil
	}
	return append(out, text[start:])
}

// isWordSeparator reports whether §8.3's word-spacing goes after a character.
func isWordSeparator(r rune) bool {
	switch r {
	case ' ', '\u00a0', // space, and the no-break space an author writes
		'\u1361',                   // Ethiopic wordspace
		'\U00010100', '\U00010101', // Aegean word separators
		'\U0001039F', '\U0001091F': // Ugaritic and Phoenician
		return true
	}
	return false
}

// SplitAtWordSeparators cuts text after each word-separator character, so that
// the spacing §8.3 puts after one falls at the end of a run.
//
// It exists for the backend and not for the rule, exactly as
// SplitAtCursiveTracking does. A display list carries a run's width and its
// letter-spacing, and nothing that says "and thirty-two pixels after the third
// character": the width a run is measured to already holds the word-spacing, and
// the glyphs inside it are drawn at their own advances, so a separator in the
// middle of a run is measured wide and drawn narrow. Everything after it on the
// line then sits where it would have without the spacing.
//
// It is not needed for an ordinary space, which SplitAtBreaks already gives a
// piece of its own — a line may end after one. The characters this is for are
// the ones that separate words and offer no break: a no-break space above all,
// which is what an author writes to keep two words together, and the four
// ancient word separators beside it.
//
// Nothing is returned where there is nothing to cut, which is every run of every
// document with no word-spacing on it — the caller asks only then.
func SplitAtWordSeparators(text string) []string {
	var out []string
	start := 0
	for i, r := range text {
		if !isWordSeparator(r) {
			continue
		}
		end := i + utf8.RuneLen(r)
		if end < len(text) {
			out = append(out, text[start:end])
			start = end
		}
	}
	if out == nil {
		return nil
	}
	return append(out, text[start:])
}
