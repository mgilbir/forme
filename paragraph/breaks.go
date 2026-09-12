package paragraph

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/style"
)

// Where a line may be broken, and what the text between two such places is.
//
// This is CSS Text §5 over a string: the opportunities that the white-space,
// word-break, line-break and overflow-wrap values allow, and the pieces they cut
// the text into. It needs no font — where a break may fall is a property of the
// characters rather than of how wide they turn out to be — and no box.

// TabAdvance is the distance from x to the next tab stop.
//
// Tab stops are at multiples of the tab size from the block's content edge, so
// a tab's advance is a property of where it lands rather than of the text it
// sits in — which is why it cannot be measured with the rest of a run.
//
// The arithmetic is exact rather than floating point, because a layout unit is
// a fixed-point integer and a tab stop computed in floats would drift along a
// line of them until two columns that should align did not.
//
// A tab size of zero renders no tab at all, which is what §4.1.2 says and is
// the only way to ask for a tab that takes no room.
//
// floor is §4.1.2's threshold: "if this distance is less than 0.5ch, then the
// subsequent tab stop is used instead". It is a rule about the *shift* and not
// about where the tab lands, which is why it is applied to the remainder rather
// than to the position: a tab already sitting a hair before a stop is a tab that
// would otherwise be invisible, and the paragraph it is in would lose the column
// it was written to make. Without it a tab at 7.9ch of an 8ch stop advances a
// tenth of a character and the text after it is a tenth of a character from the
// text before it — which looks like no tab at all rather than like a wrong one,
// and is the shape of silent difference §6 is about.
//
// A floor of zero is *absent* rather than "no distance is short enough": the
// comparison is strict, so a zero floor can never fire, and a caller that could
// not measure a "0" passes zero to say exactly that. The two readings agree
// here, which is why there is one parameter and not two.
func TabAdvance(x, stop, floor style.Unit) style.Unit {
	if stop <= 0 {
		return 0
	}
	// The stops run in both directions from the block's content edge, and a pen
	// position may be on the other side of it: "text-indent: -3ch" starts the
	// line three characters outside. So the distance is measured from the stop
	// *below* x, and below a negative x that is a negative multiple.
	//
	// Go's remainder takes the sign of the dividend, so this is the floored
	// modulus written out. Clamping x to zero instead — which is what was here —
	// answers a full stop from wherever the line began, so an outdented line's
	// first tab landed a whole stop past the column every other line in the
	// block put it in. text-indent-tab-positions-001 is three paragraphs of the
	// same tabbed text asking for exactly that alignment.
	r := x % stop
	if r < 0 {
		r = r.Add(stop)
	}
	d := stop.Sub(r)
	if d < floor {
		d = d.Add(stop)
	}
	return d
}

// Piece is a run of text between two break opportunities, together with what
// §4.1.2 has to know about it once it lands on a line.
type Piece struct {
	Text        string
	BreakBefore bool
	// ZeroWidth marks a piece that is a character and nothing more: it sets no
	// paper, takes no room and produces nothing to put on a line.
	//
	// It exists because it *separates*. §4.1.1 collapses white space that is
	// adjacent, and a zero-width space between two spaces is a character
	// standing between them — so they are not adjacent and do not collapse. The
	// suite writes it as a comment of its own, "U+00A0 is exactly equivalent to
	// U+200B U+0020 U+200B", and tests it four times.
	//
	// Dropping the character outright is what made it invisible to that rule.
	// Building an item from it instead would put a draw operation on every page
	// that has one, for a glyph with no advance and no ink.
	ZeroWidth bool
	// Space marks white Space of any kind, collapsible marks the subset of it
	// Phase I folds together, trimAtEnd the subset a line edge removes, and tab
	// and segment the two preserved characters that are not simply text of their
	// own width.
	Space       bool
	Collapsible bool
	TrimAtEnd   bool
	Tab         bool
	Segment     bool
	// LastResort marks an opportunity a line reaches for only when it has no
	// other: it is offered, and everything else on the line is preferred to it.
	//
	// CSS Text §5.2's "auto-phrase" is what makes one. The value withholds the
	// implicit opportunities inside a phrase, and withholding is not deleting:
	// a phrase wider than the line it is on still has to break somewhere, and
	// the suite says so by name — word-break-auto-phrase-009's assert is
	// "auto-phrase's must give up on suppressing wrapping opportunities when
	// that would lead to overflow", and the reference for its narrowest box
	// divides the phrase between two characters no phrase boundary falls
	// between.
	//
	// It is BreakBefore's rank rather than its replacement: a piece with this
	// set has BreakBefore set too, and a reader of BreakBefore alone sees an
	// opportunity, which is what it is.
	LastResort bool
	// Hyphen marks a piece that ends at a soft hyphen: a line may end after it,
	// and a hyphen is drawn when one does.
	//
	// It is a property of the piece before the opportunity rather than of the
	// one after it, because what it changes is the *end* of a line. Everything
	// else here that offers a break says so on the piece that may begin the
	// next line, and that is the wrong end for this: the hyphen is printed on
	// the line that broke, and how wide it is decides whether that line could
	// break at all.
	Hyphen bool
}

// SplitAtBreaks cuts text at the break opportunities this engine implements.
//
// The subset is stated in the file comment. Each rule below is one of UAX #14's,
// named by what it does rather than by its class letters, and the ones left out
// are left out loudly — checkScript reports text that needs them.
//
// It takes the white-space value because two of the rules depend on it: a
// preserved space is a Piece of its own rather than a collapsed one, and
// break-spaces wants each space separately because a line may end after any one
// of them.
//
// The text is walked rune by rune rather than through a []rune, which is not a
// micro-optimisation: a text node is untrusted and arbitrarily large, and a
// decoded copy of one is four bytes per character of buffering nobody asked for.
func SplitAtBreaks(text string, ws WhiteSpace, wb WordBreak, lb LineBreak, hy Hyphens,
	w WritingSystem) ([]Piece, Trailing) {
	return SplitAtBreaksAfter(text, ws, wb, lb, hy, w, Carried{})
}

// Carried is what the text before this one left at the boundary between them,
// and what this one needs to finish the rules that boundary interrupted.
//
// CSS Text §8.1's boundary between two inline elements does not break shaping,
// and it does not break line breaking either. UAX #14 is a pair algorithm: what
// decides an opportunity is the character on each side of it, and a boundary
// puts those two characters in different boxes. Neither box can answer on its
// own, so the box before says what it left and the box after runs the rules.
//
// Telling layout afterwards is not the same thing and was tried first. An
// opportunity may have to be taken up at the *second* character of the next box
// — "<span>|</span><span>!0</span>", where the exclamation mark refuses the
// break and the digit takes it — and layout resumes at the next Piece, of which
// "!0" is one. Only the scan can look inside a piece, so the boundary comes in
// here rather than being corrected there. boundarybreak_test.go holds the five
// defects that made the case.
type Carried struct {
	// Offered says the text before left an opportunity at the boundary at all.
	// The two below say which kind, and are meaningless without it.
	Offered bool
	// Deferred says the opportunity has not been through the prohibitions yet:
	// it was offered by the character before the boundary, and whether a line
	// may actually begin here is a question about the character after it, which
	// is this text's first. word-break still gets to suppress it.
	Deferred bool
	// Held says it has been through them once and was *moved* rather than
	// refused — a prohibition shifts an opportunity past the character a line
	// may not begin with rather than deleting it. word-break does not get a
	// second say on the far side of the character that displaced it, which is
	// what word-break-keep-all-006 asks for.
	//
	// Neither set means the opportunity was *taken*: a space left it, the rules
	// have had their say, and only LB7 still applies.
	Held bool
	// Prev is the last character before the boundary, for the pair rules and
	// for the rules that need to know there is any text in front of this at all.
	// It is zero at the start of a paragraph and nowhere else.
	Prev rune
	// Before is the text in front of this one, for the scripts whose words are
	// found with a dictionary rather than by a rule.
	//
	// Those need more than the character before: a segmentation is a statement
	// about a stretch of text, and "ภาษาไทย" divides after "ภาษา" because of
	// what follows, which no walk that has only reached that point can know.
	// DictionaryBreaks is given this and the text together, so a word written
	// across a box boundary is one word — "<span>ภาษา</span><span>ไทย</span>"
	// set one line where "ภาษาไทย" sets two, because each box was segmented on
	// its own and the offset the break falls at is the second box's first, which
	// DictionaryBreaks excludes on purpose.
	//
	// About a word of it, which is exact rather than a bound that is usually
	// enough: text on the far side of a space or of another script would not
	// have been segmented with this anyway, and inside the run the words already
	// found have been decided. See Trailing.DictTail, which is what fills this
	// in and where the measurement is.
	Before string
	// After is the text that follows this one, for the same scripts Before is
	// for and for the other half of the same problem.
	//
	// Before carries the context backwards, so a word written across a boundary
	// keeps the division between its halves. This carries it forwards, so a box
	// does not invent a division its own text only appears to have: segmentWords
	// is greedy, and a word that runs past the end of a box is a word the box
	// cannot match. "ด๗ไษภหทย" has no break at 18 and "ด๗ไษภหท" — the same text
	// with the last character in another box — has one, because the first box
	// looked for a word, found none, and fell back to breaking between
	// typographic character units.
	//
	// DictionaryLookahead says how much is enough, and it is exact rather than
	// generous: a probe reads at most the longest word in the language.
	After string
	// SpaceMayTakeIt says a space at this text's start may take the opportunity
	// rather than withholding it, which is white-space: break-spaces overruling
	// LB7. See boundaryWhiteSpace: the value that decides it belongs to the box
	// the space is in.
	SpaceMayTakeIt bool
}

// SplitAtBreaksAfter is SplitAtBreaks for text that follows other text in the
// same paragraph, with the boundary between them.
func SplitAtBreaksAfter(text string, ws WhiteSpace, wb WordBreak, lb LineBreak, hy Hyphens,
	w WritingSystem, at Carried) ([]Piece, Trailing) {
	var out []Piece
	var cur strings.Builder
	breakNext := false
	// giveUpNext is breakNext's rank: the opportunity is there and it is the
	// last one the line will reach for. See Piece.LastResort.
	giveUpNext := false

	// Where the words are, for the scripts whose words a rule cannot find. It
	// is computed once for the whole run rather than asked per character,
	// because a segmentation is a statement about a stretch of text and not
	// about the character in front of it: the word "กรุงเทพ" is one word because
	// of what follows its first character, and no walk that has only reached
	// that character can know. Nil for the overwhelming majority of documents,
	// which have no such script in them at all. See DictionaryBreaks.
	// The boundary is part of the stretch: see Carried.Before. dictAt is where
	// this text begins in what was segmented, and is zero for every document
	// that has no such script in it.
	dictAt := len(at.Before)
	// The stretch this text is responsible for, and the stretch that has to be
	// segmented to decide it. They differ by the lookahead, which is read and
	// then thrown away: a break beyond the end of dictSeg belongs to the box
	// that holds the text it falls in.
	dictSeg := at.Before + text
	dictBreaks := DictionaryBreaks(dictSeg + at.After)

	// And where the phrases are, for the value that ends a line only at one.
	// Computed once for the same reason and nil for the same documents — see
	// PhraseBreaks, whose three answers are what tells a place inside a phrase
	// from a place the model was never about.
	var phrases map[int]bool
	if wb.AutoPhrase {
		phrases = PhraseBreaks(text, w)
	}

	// Grapheme cluster boundaries, walked in lockstep with the scan.
	//
	// It runs for every value of word-break and not only for break-all, because
	// the rule it enforces is not break-all's: CSS Text §2 puts a soft wrap
	// opportunity *between* typographic character units, so no opportunity this
	// function produces may fall inside a cluster. The ideograph rule below used
	// to produce one — a Hangul syllable followed by its own trailing jamo was
	// cut in two, which put half a syllable at the end of a line.
	//
	// A Scanner rather than a list of offsets: the scan is already linear, and a
	// list would allocate one int per character for Latin text, where every
	// character is its own cluster and nothing is learned.
	//
	// A Scanner is also the *right* reading here, and not merely the cheap one.
	// segment.Boundaries would answer differently about a byte that is not
	// UTF-8: it has the bytes, so it calls one its own cluster on both sides,
	// and segment.InvalidByte is how a caller walking a string asks for that
	// answer. This walk must not ask. It does not hand the bytes on — it writes
	// the rune it decoded, so an invalid byte leaves here as a U+FFFD the pieces
	// really contain, and the unit a line may be cut at is the cluster of the
	// text that is *emitted*. Under segment's reading a combining mark after an
	// invalid byte would begin a piece of its own, and a line would be allowed
	// to start with it.
	var clusters segment.Scanner
	// deferBreak says the previous character allows a line to end after it, and
	// the opportunity has not been taken yet.
	//
	// It is deferred because whether the cut is legal depends on the character
	// that *follows*: only that one says whether the cluster ended. Taking the
	// opportunity where it is offered is what cut the syllable open.
	deferBreak := at.Offered && at.Deferred
	// heldBreak is an opportunity that was offered and moved rather than
	// refused: the character in front of it is one a line may not begin with, so
	// the break belongs after that character instead. It is kept apart from
	// deferBreak because it has already been through the rules once — word-break
	// does not get to suppress it a second time on the far side of the character
	// that displaced it.
	heldBreak := at.Offered && at.Held
	carried := deferBreak || heldBreak
	// The character before this one, for the pair rules. See gluedPair.
	prev := at.Prev
	// An opportunity the text before this one *took* rather than offered — a
	// space left it — which the rules have already had their say over. It marks
	// the first Piece rather than going through the scan, which is what the
	// switch below does for a space inside a run.
	//
	// Taken at the first character rather than here, because LB7 still applies
	// to it: a line may not end in front of a space, so an opportunity arriving
	// at one is withheld unless break-spaces says otherwise. Setting it here
	// broke a run of preserved spaces in two — white-space-mixed-001, whose
	// spans hand a pre div a space apiece.
	takenAtStart := at.Offered && !at.Deferred && !at.Held
	// Whether there is text in front of this one at all, which is what decides
	// that an opportunity falling at the very first character is a real one.
	//
	// The opportunities above travel forward: the previous box says what it
	// left, and carried is that. This one is made by the *next* box's own first
	// character and nothing before it knows about it — the rules that put a
	// break in front of a character rather than after one, which are the
	// ideograph's, the aksara's, the dictionary's, and break-all's and
	// anywhere's every-character pair.
	//
	// "0ᦤ" is the shape. New Tai Lue is a script this engine has no dictionary
	// for, so §5.1's fallback puts an opportunity at every typographic character
	// unit, and the text breaks between the two. Written as
	// "<span>0</span><span>ᦤ</span>" the opportunity is at the second box's
	// first character, where cur is empty and the box before left nothing — so
	// it was dropped, the two spans were one unbreakable run, and a ligature was
	// free to cross a boundary a line may fall on.
	//
	// The paragraph's own first character is excluded by the same test rather
	// than by a special case: a break in front of the first thing on the first
	// line is not a break, and Prev is zero exactly there.
	afterText := carried || at.Prev != 0

	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, Piece{Text: cur.String(), BreakBefore: breakNext, LastResort: giveUpNext})
		cur.Reset()
		breakNext, giveUpNext = false, false
	}
	// flushHyphen is flush for a piece that ends at a soft hyphen. It is
	// separate rather than a parameter because every other caller passes false
	// and a bare boolean argument at nine call sites says nothing about which
	// end of the line it is about.
	flushHyphen := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, Piece{Text: cur.String(), BreakBefore: breakNext,
			LastResort: giveUpNext, Hyphen: true})
		cur.Reset()
		breakNext, giveUpNext = false, false
	}
	// A white-space Piece takes the pending opportunity but does not consume
	// it: what follows a space may begin a line whatever came before it, and an
	// earlier version that cleared the flag here lost the opportunity after
	// "a- b" entirely.
	emit := func(p Piece) {
		p.BreakBefore, p.LastResort = breakNext, giveUpNext
		out = append(out, p)
	}

	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		start := i
		i += size

		atBoundary := clusters.Boundary(r)

		// The opportunity that may fall before this character: one deferred from
		// the character before, or — under break-all, CSS Text §5.2 — one at
		// every typographic character unit boundary inside a word.
		//
		// White space is excluded from break-all's half, and that exclusion is
		// UAX #14's LB7 rather than a simplification: a line may not end between
		// a word and the space after it, so the space stays on its word's line.
		// Without it, "X XX X" in four characters of room breaks after the
		// fourth — which fits more text and is the wrong answer. The other
		// separators are excluded with it, which errs towards fewer
		// opportunities and so overflows a line rather than breaking it in a
		// place the algorithm did not sanction.
		// line-break: anywhere is the third source and the widest: §5.3 puts an
		// opportunity around *every* typographic character unit, so it needs
		// neither break-all's exclusion of white space nor anything deferred. It
		// is what makes "X XX X" in four characters of room break after the
		// fourth — the answer break-all must not give, and the one the suite's
		// break-spaces-before-first-char-007 asks for by name.
		// word-break: keep-all withholds the deferred one, and only where the
		// character it is offered to is a letter.
		//
		// §5.2: "implicit soft wrap opportunities between typographic letter
		// units (or other typographic character units belonging to the NU, AL,
		// AI, or ID Line Breaking Classes) are suppressed". Both sides have to
		// be one, which is why this reads the character rather than the value
		// alone: the opportunity between an ideograph and the comma after it is
		// not between letter units and is not keep-all's to take. It is not
		// taken by anyone else either — LB13 moves it past the comma — and what
		// arrives on the far side is a held one, which is the second term above
		// and is not offered to keep-all a second time.
		//
		// The suite tests each half: word-break-keep-all-005 asks for the break
		// after U+3000 to survive, -006 for the one after an ideographic comma,
		// and -011 for every implicit one inside "中文english中文english" to go.
		// The fourth source is the ideograph rule's other half. An ideograph
		// defers an opportunity to the character *after* it, and UAX #14 allows
		// one before it as well: nothing prohibits a break between a letter or a
		// number and an ideograph, so "abc永" may break either side of the 永.
		//
		// It fires only where the character before is a letter unit and is not
		// itself an ideograph, which is the boundary the deferred half cannot
		// reach: between two ideographs the deferred opportunity is already
		// there, and offering a second one at the same place answers nothing and
		// — measured — costs 63 clean passes, because every opportunity this
		// grants that a prohibition then refuses is *held* and reappears one
		// character further on.
		//
		// It is here as well as in layout's boundary rule so that the two agree.
		// The same text has to break the same way whether or not the author
		// wrote a <span> between the letter and the ideograph.
		// keep-all used to be a conjunct here and is now handled with the rest
		// of its prohibition, below: the value relaxes, so what it forbids has
		// to be *demoted* rather than deleted, and an opportunity deleted at
		// this line could not be.
		beforeIdeograph := IsIdeographic(r) && prev != 0 &&
			!IsIdeographic(prev) && isLetterUnit(prev)
		// And the same shape for the Brahmic scripts, which write without
		// spaces and whose only opportunity is the boundary between two aksara
		// clusters. See isAksara: LB28a is a set of prohibitions inside a
		// cluster and LB31 allows the break between them.
		//
		// Offered before rather than deferred after, because a cluster is
		// several characters and a deferred opportunity survives one: the
		// boundary wanted is the one in front of the next cluster, and asking
		// there is asking for it directly. "keep-all" suppresses it for the
		// reason it suppresses the ideograph's — §5.2 forbids the implicit
		// opportunities between typographic letter units.
		beforeAksara := isAksara(r) && prev != 0 && !wb.KeepAll
		// §5.1's lexical breaking, for the scripts that write no spaces between
		// their words. Where this engine has the language's vocabulary the
		// opportunity is at a word boundary and nowhere else — see
		// DictionaryBreaks — and where it has not, the fallback the section
		// allows is every typographic character unit, which is the same
		// boundary for a different reason and is known to be in the wrong
		// place. UnsupportedScript is what says which of the two a document got.
		beforeDictionary := NeedsDictionaryBreaking(r) && prev != 0 &&
			!wb.KeepAll && !wb.Manual
		if beforeDictionary && HasDictionary(r) {
			beforeDictionary = dictBreaks[dictAt+start]
		}
		// §5.3's "breaks are allowed ... between inseparable characters (such as
		// U+2025 and U+2026)", which is an opportunity nothing else here makes.
		//
		// It is the other half of a sentence whose first half was already
		// implemented, and the two are easy to mistake for one. A line may
		// *begin* with an ellipsis under loose, which is UAX #14's LB22 relaxed
		// and lives in looseBreakRanges — but a relaxed prohibition still needs
		// an opportunity to relax, and between two ellipses there is none: the
		// ideograph rule makes one beside 中 and nothing makes one between "‥"
		// and "‥". So "中中‥‥中" broke in front of the pair and never inside it.
		//
		// line-break-loose-015 is the suite's statement of it, and its assert
		// names the two characters.
		//
		// The "loose" test is the rule §5.3 states and no document can see it,
		// which is worth saying rather than leaving to be rediscovered. Class IN
		// is in looseBreakRanges, so noBreakBefore forbids a line to begin with
		// an ellipsis at every other value — an opportunity offered here would be
		// refused there, and held to the same place it was already held. A
		// planted defect that dropped the conjunct moved no test and no reftest.
		// It stays because the two facts come from one table and a rule that
		// depends on that coincidence is a rule nobody can check.
		betweenInseparable := lb.Loose && isInseparable(prev) && isInseparable(r)
		// keep-all's own prohibition, which §5.2 makes a preference rather than
		// a rule. It is the only one here that is written down as relaxable:
		//
		//	In this style, sequences of NU, AL, AI, and ID characters [...] are
		//	not broken. [...] Note: this value may be relaxed by the UA if there
		//	are no otherwise-acceptable break points in the line.
		//
		// §6.2 says the same from the other side, and the suite's
		// overflow-wrap-normal-keep-all-001 asserts it with eight ideographs in
		// a box of no width at all: nowhere else on the line can the break go,
		// so keep-all gives way and the column comes out one character wide.
		//
		// "Relaxed if there is nothing else" is what Piece.LastResort already
		// means, so this is offered rather than withheld and demoted below —
		// which is the same two steps the auto-phrase value takes, in the same
		// order and for the same reason.
		spaceStops := startsSpacePiece(r, ws)
		if start == 0 {
			// LB7 at the boundary, which is a rule about the two characters on
			// either side of it rather than about the one after — see
			// betweenTwoSpaces. break-spaces overrules it, and that is the same
			// overruling spaceStops gets below.
			endsInFrontOfASpace := betweenTwoSpaces(at.Prev, r) && !at.SpaceMayTakeIt
			if at.SpaceMayTakeIt {
				spaceStops = false
			}
			if takenAtStart && !endsInFrontOfASpace {
				breakNext = true
			}
		}
		keptAll := wb.KeepAll &&
			((deferBreak && isLetterUnit(r) && !spaceStops) || beforeIdeograph)
		offered := (deferBreak && !(wb.KeepAll && isLetterUnit(r)) && !spaceStops) ||
			(heldBreak && !spaceStops) ||
			(wb.BreakAll && !spaceStops) || lb.Anywhere ||
			beforeIdeograph || beforeAksara || beforeDictionary ||
			betweenInseparable || keptAll
		// UAX #14 forbids a line beginning with a closing bracket, a hyphen or
		// a non-starter, and an opportunity offered in front of one is not one.
		// See linebreak.go for which rules that is and which it is not.
		//
		// line-break: anywhere is exempt, and by name: §5.3 puts an opportunity
		// around every typographic character unit "including around any
		// punctuation character or preserved white space", which is a value
		// whose whole purpose is to overrule this.
		//
		// A prohibition *moves* an opportunity rather than deleting one, which is
		// the whole shape of a pair rule: "× CL" says a line may not begin with a
		// closing bracket, and says nothing against a line beginning with what
		// comes after it. So the deferred opportunity is held rather than
		// dropped, and the next character is asked in its turn.
		//
		// Without that, "字字、字字" had a break between the two ideographs and
		// none after the comma, so a four-character box set it as three
		// characters and one. word-break-keep-all-006 asks for the two-by-two
		// square, and the same text answers it at every value of word-break: the
		// opportunity the comma stands in front of is the one after it.
		held := false
		if offered && !lb.Anywhere && noBreakBefore(r, lb) {
			offered, held = false, true
		}
		// And the pair rules, which are the other half of the same paragraph of
		// UAX #14 and are not held: a rule that says a line may not *end* after
		// this character has nothing to say about the next boundary, so an
		// opportunity it refuses is gone rather than moved. Holding one forward
		// would put a break after a no-break space one character further along,
		// which is the answer the rule exists to prevent.
		if offered && !lb.Anywhere && gluedPair(prev, r) {
			offered = false
		}
		// §5.2's "auto-phrase", which is keep-all with the phrase boundaries let
		// back in: the implicit opportunities inside a phrase are withheld and
		// the one at its edge is not.
		//
		// Withheld and not removed, which is the difference between this and
		// keep-all. A phrase wider than its line still has to break, so what
		// happens here is a demotion: the opportunity stands and every other
		// opportunity on the line is preferred to it. See Piece.LastResort.
		//
		// After the prohibitions, and that is not tidiness. A prohibition moves
		// an opportunity rather than deleting one, so the place a break may fall
		// is not always the place it was offered — and it is the place it falls
		// that a phrase boundary is or is not at. "ドライブ、楽しい" is the shape:
		// the opportunity after "ブ" is inside a phrase and would be withheld,
		// UAX #14 will not let a line begin with the comma so it moves past it,
		// and where it lands is exactly where the model says the next phrase
		// starts. Ranking it before the move suppressed it, and the line then
		// broke three characters early.
		//
		// line-break: anywhere is exempt, as it is from every other rule here:
		// §5.3 puts an opportunity around every typographic character unit and
		// says so in a sentence written to overrule the rest of §5.
		//
		// A space, a zero width space and a <wbr> never reach this at all —
		// they set the opportunity in the switch below rather than offering one
		// here, which is what word-break-auto-phrase-007 asks for: "UAs must not
		// suppress wrapping opportunities introduced by wbr or ZWSP".
		giveUp := false
		// keep-all's, demoted after the prohibitions for the reason the phrase
		// demotion below is: a prohibition moves an opportunity rather than
		// deleting one, and what is being ranked is where the break may fall.
		if keptAll && offered && !lb.Anywhere {
			offered, giveUp = false, true
		}
		if boundary, scored := phrases[start]; scored && !boundary &&
			offered && !lb.Anywhere {
			offered, giveUp = false, true
		}
		if (offered || giveUp) && atBoundary && (cur.Len() > 0 || (start == 0 && afterText)) {
			if cur.Len() > 0 {
				flush()
			}
			breakNext, giveUpNext = true, giveUp
		}
		deferBreak, heldBreak = false, held
		prev = r

		switch {
		case IsMandatoryBreak(r):
			// UAX #14's BK and NL: a character that ends a line wherever it
			// appears, which is not the same thing as a segment break. A
			// segment break is collapsible — a newline under "white-space:
			// normal" becomes a space and the line goes on — and these are not:
			// LB4 and LB5 make the break mandatory, and no value of white-space
			// is written over them.
			//
			// They reached here as ordinary characters and were set as ordinary
			// characters, so "1<FF>2" came out on one line with a notdef box
			// between the digits. line-breaking-022 writes all five between
			// spans in a column one character wide and asks for six lines.
			//
			// The character is written into the piece that ends the line and
			// the break is emitted after it, rather than the character being
			// swallowed the way a newline is. §5.1's note asks for both:
			//
			//	Control characters other than [tab, newline] ... are ignored
			//	for the purpose of ... but are otherwise rendered as a visible
			//	glyph
			//
			// and the suite asks for it twice over, by the same author. Three
			// of the white-space/control-chars-0XX documents are mismatch
			// references against a blank page — "U+000C, which is in the
			// unicode category CC, must be visible" — and line-breaking-022
			// wants the same character to end a line. Swallowing it satisfies
			// the second and fails the first three.
			cur.WriteRune(r)
			flush()
			emit(Piece{Space: true, Segment: true})
			breakNext = true

		case r == '\n' || r == '\r':
			// Only a *preserved* break reaches here: Phase I turned a
			// collapsible one into a space. A CR is folded with the LF that may
			// follow it, so that text which reached this stage without going
			// through Phase I — a caller measuring raw content — still counts
			// one break rather than two.
			if r == '\r' && i < len(text) && text[i] == '\n' {
				i++
			}
			flush()
			emit(Piece{Text: "\n", Space: true, Segment: true})
			breakNext = true

		case r == '\t' && !ws.Collapse:
			// A preserved tab is its own Piece because each one advances to its
			// own tab stop, so two of them are not one run of a doubled width.
			flush()
			emit(Piece{Text: "\t", Space: true, Tab: true})
			breakNext = true

		case IsOtherSpaceSeparator(r):
			// §4.1's "other space separators". Phase I never saw them — it is
			// defined over U+0020, U+0009 and the segment breaks and nothing else
			// — so what arrives here is exactly what the author wrote, and it is
			// §4.1.2's fourth rule that has something to say about it: a run of
			// them at the end of a line hangs just as a run of preserved spaces
			// does, whatever the white-space value, because the rule is written
			// over "white space, other space separators, and/or preserved tabs".
			//
			// One character each rather than a run, because a run of them is not
			// one thing: U+3000 offers an opportunity after it and U+202F does
			// not, so two adjacent separators can differ in the only property
			// that would justify gathering them.
			//
			// The ogham space mark is the exception §4.1.2's *third* rule carves
			// out: where white space collapses it is removed at the end of a line
			// rather than hung, which is trimAtEnd. It is still not collapsible —
			// a run of ogham space marks is a run of stemlines and folding them
			// into one would shorten the line.
			flush()
			emit(Piece{
				Text: text[start:i], Space: true,
				TrimAtEnd: r == 0x1680 && ws.Collapse,
			})
			// §5.3 again, and it is the value's whole purpose: line-break:
			// anywhere puts an opportunity around every typographic character
			// unit "including around any punctuation character or preserved
			// white space", so the classes that would refuse one after this
			// separator do not get to. U+202F NARROW NO-BREAK SPACE is class GL
			// and glues what follows it to what precedes it — which is the right
			// answer everywhere else and is exactly what the value overrules.
			//
			// break-spaces is *not* beside it, and used to be. That value puts a
			// soft wrap opportunity "after every preserved white space
			// character", and CSS Text means its own term by that: white space
			// is U+0020, the tab and the segment breaks, and these are the
			// characters §4.1.2 has to name separately as "other space
			// separators" precisely because they are not it. Phase I never sees
			// one and phase II only hangs it; nothing in the value reaches its
			// line-breaking class, so UAX #14 decides it here as it does
			// everywhere else. The suite writes the two that differ as
			// trailing-other-space-separators-break-spaces-009 and -013, which
			// are the two GL separators and the only two of the fifteen where
			// the answers part company.
			breakNext = lb.Anywhere || SeparatorBreaksAfter(r)

		case r == ' ' || r == '\t':
			flush()
			if ws.Collapse {
				// Phase I already reduced the run to a single space and turned
				// any tab into one, so there is nothing left to gather.
				emit(Piece{Text: " ", Space: true, Collapsible: true, TrimAtEnd: true})
				breakNext = true
				break
			}
			// Preserved. Under pre and pre-wrap the run hangs or wraps as a
			// unit, so it is one Piece; under break-spaces a line may end after
			// any single space, so each is its own.
			// Under pre and pre-wrap the run hangs or wraps as a unit, so it is
			// gathered — unless line-break: anywhere says a line may end between
			// any two of them, which is a run that is no longer one thing.
			if !ws.BreakSpaces && !lb.Anywhere {
				for i < len(text) && text[i] == ' ' {
					i++
				}
			}
			emit(Piece{Text: text[start:i], Space: true})
			breakNext = true

		case r == '​':
			// A zero-width space is a break opportunity, and it is also a
			// character. The opportunity is what an author writes one for — it
			// is how a break is marked inside a word — and the character is
			// what §4.1.1's collapsing has to see: two spaces with one between
			// them are not adjacent, so they do not collapse into one. The
			// suite says it in a comment of its own, "U+00A0 is exactly
			// equivalent to U+200B U+0020 U+200B", and tests it four times.
			//
			// So it is emitted rather than dropped, and marked ZeroWidth: it
			// sets no paper and takes no room, so nothing is built from it, and
			// what it does is stand between its neighbours.
			flush()
			emit(Piece{Text: text[start:i], ZeroWidth: true})
			breakNext = true

		case IsIdeographic(r):
			// CJK breaks between ideographs, which is why it needs no spaces.
			//
			// The opportunity after it is deferred rather than taken, because a
			// Hangul syllable can be followed by a trailing jamo that belongs to
			// it and by a combining mark that belongs to it, and neither is a
			// place a line may end. The next character's boundary decides.
			flush()
			cur.WriteRune(r)
			deferBreak = true

		case lb.Loose && BreaksAfterUnderLoose(r) && !startsSpace(text, i):
			// §5.3's one rule the other way round: under "loose" a line may end
			// after a currency sign or a number sign, which belongs to the
			// figure following it and which no other value lets go of.
			//
			// It is written beside the hyphen below because it is the same
			// shape of rule — a character that ends a run and lets the next one
			// begin a line — and it carries the same guard: a space after it is
			// not an opportunity, because the space already is one and a line
			// may not end in front of it.
			//
			// The end of this text is *not* that guard, which is what it used to
			// ask. A prefix that ends a text node has whatever comes after it in
			// another box, and the flag this function returns is how the
			// opportunity gets there — which is what the soft hyphen below says
			// in full and what every other opportunity here already does. The
			// suite writes the prefix in an element of its own so that it can be
			// coloured: line-break-loose-018 is
			// "サンプル文サンプル<span>€</span>サンプル文", and asking for the
			// end of the node meant the opportunity was offered in none of its
			// five pairs.
			cur.WriteRune(r)
			flush()
			breakNext = true

		case (r == '-' || isLatinHyphen(r)) && !startsSpace(text, i):
			// A hyphen ends a run and the next may begin a line — which is what
			// lets a hyphenated compound break where it is written.
			//
			// Including where the next line's half is in another box.
			// "high-<span>way</span>" and "<span>high-</span>way" are the same
			// word as "high-way" and have to break the same way; asking for the
			// end of the *text* rather than for a space after it, they broke
			// nowhere and the compound overflowed its box. It is the rule the
			// soft hyphen below states in full, and the two are one rule.
			//
			// All three of them. U+002D HYPHEN-MINUS is class HY and U+2010
			// HYPHEN and U+2013 EN DASH are class HH, and what the classes differ
			// about is the *start* of a line: see isLatinHyphen, which is the
			// other half of the same pair and was written first. A line may end
			// after any of the three, and only U+002D was ending one — so a
			// document that spells its hyphen with the character meant for it,
			// which is what "&#x2010;" is for, had its compounds overflow
			// instead of break.
			//
			// It is not the hyphens property's business. §6.1 is about where a
			// word may be broken *without* a hyphen written in it; a hyphen that
			// is there is an ordinary break opportunity whatever the value.
			// hyphens-none-013's assert is that "hyphens: none does not suppress
			// line wrapping after encountering an actual hyphen character
			// (U+2010)".
			cur.WriteRune(r)
			flush()
			breakNext = true

		case breaksAfter(r):
			// UAX #14's class BA: a line may end after this character whatever
			// follows it. See breaksAfter for which characters those are and
			// which of the class are handled above instead.
			//
			// Deferred rather than taken, for the reason the ideograph arm
			// gives: the opportunity is at the *next* boundary, and only the
			// character after this one can say whether a cluster ended there or
			// whether a rule forbids a line to begin with it. A danda followed
			// by a closing bracket offers nothing, which is LB13, and the
			// deferral is what runs that rule.
			cur.WriteRune(r)
			deferBreak = true

		case r == 0x00AD && hy.Soft() && !startsSpace(text, i):
			// A soft hyphen. §6.1: the author has marked a place the word may be
			// broken, and a hyphen is printed there if it is.
			//
			// The character stays in the piece rather than being dropped. It
			// takes no room and sets no paper — every face here shapes it to
			// nothing, and shape/ignorable.go is where that is decided — so
			// keeping it costs nothing on the page, and it keeps the text of the
			// document the text the author wrote. Dropping characters to make
			// layout tidier is how a paragraph comes out of a PDF missing pieces
			// of its words.
			//
			// startsSpace and not "the end of this text", which is the rule the
			// ordinary hyphen above now shares and once did not: the end of
			// *this text* is not the end of the word. The suite's
			// hyphens-span-001 writes the same word nine ways —
			// "<span>high&shy;</span>way", "high<span>&shy;</span>way",
			// "high&shy;<span>way</span>" — and asks for one answer from all of
			// them, so a soft hyphen that ends a text node has to offer its
			// opportunity to whatever box comes next. That is what the returned
			// flag is for and what every other opportunity here already does.
			//
			// A space after it is still not one, for the reason the hyphen above
			// has: there would be nothing to move to the next line, and a hyphen
			// printed there would be one in the middle of nothing.
			//
			// That conjunct is the correct reading of the rule and has no test,
			// which is a different thing from being covered. Removing it prints
			// no hyphen anywhere — a line that ends at a space ends *after* the
			// space, so the item a hyphen would hang off is never the last one —
			// and what it does leave is an opportunity in front of a space, which
			// LB7 forbids and which every path that would use one already
			// declines. Measured: with the conjunct gone, all 6250 of the suite's
			// reftests give the same answer, 5388 of them cleanly. It is recorded
			// here rather than left as an implied claim.
			cur.WriteRune(r)
			flushHyphen()
			breakNext = true

		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out, Trailing{
		DictTail: dictionaryTail(dictSeg, dictBreaks),
		Offered:  breakNext || deferBreak || heldBreak,
		// A break the text *took* is what the boundary is, whatever else is
		// still pending at it. The three are not exclusive, which is easy to
		// miss because two of them are: "|-" ends with a deferred opportunity
		// the vertical line offered and the hyphen then held — a line may not
		// begin with a hyphen — and with the unconditional one the hyphen itself
		// takes. Both are at the same offset, which is the end of the text.
		//
		// Reporting the held one lost the break. "|-!" sets two lines, because
		// the opportunity the hyphen takes is not one an exclamation mark
		// refuses; "<span>|-</span><span>!</span>" set one, because the next box
		// was handed a hold, ran the prohibitions over it as a hold is meant to
		// be, and had nothing left.
		//
		// Deferred needs no such test and does not get one. Both arms that defer
		// write the character to cur, so the final flush above has emitted it
		// and cleared breakNext — the two cannot both be true here. A planted
		// "&& !breakNext" on this line moved nothing, which is what says the
		// pair is impossible rather than merely unwritten.
		Deferred: deferBreak,
		Held:     heldBreak && !breakNext,
	}
}

// Trailing is what a run of text leaves for whatever follows it in another box.
//
// CSS Text §8.1's boundary between two inline elements does not break shaping,
// and it does not break line breaking either: the character on the far side has
// to be asked the same questions it would have been asked inside a run. A caller
// that has one box's text and then another's cannot ask them without this,
// because neither fact can be read back off the text.
type Trailing struct {
	// Offered says the text ended at an opportunity the next box may take.
	//
	// A deferred one counts, and has to: text ending in an ideograph offers a
	// break to whatever box comes next, and the character that would have
	// confirmed it is in that box rather than this one.
	Offered bool
	// Deferred says that opportunity is one the next character may still
	// refuse. It was *offered* rather than taken, so UAX #14's "a line may not
	// begin with this character" has yet to run over it — which inside a run is
	// what turns "字字、字字" into two lines of two rather than three and one.
	//
	// It cannot be read off the last character, and the attempt to is what this
	// field replaced. An ideograph defers an opportunity and so does every class
	// BA character — a danda, a vertical line — while a hyphen does not, because
	// the arm that handles it takes the opportunity instead, and nor does a
	// space. Which arm ran is a fact about the scan and not about the character,
	// and a caller testing the character alone gets the ideographs right and the
	// rest wrong: "0|!" is one unbreakable run and "<span>0|</span><span>!</span>"
	// broke in two, because nothing asked LB13 about the exclamation mark.
	Deferred bool
	// DictTail is what the next box needs to be segmented together with this
	// text, for the scripts whose words a dictionary finds. It is empty for
	// every document with no such script in it.
	//
	// The text since the last word boundary, and no more, which is exact rather
	// than a bound that is usually enough. segmentWords is greedy and runs left
	// to right: once it has put a boundary at an offset, how the text after that
	// offset divides depends on that text alone. So the words already found can
	// be dropped, and what is carried is the part-word in progress.
	//
	// That is what keeps it from being quadratic. Carrying the whole script run
	// instead — which is the obvious reading of "the dictionary needs the
	// context" — made two thousand Thai words in two thousand spans take 845ms
	// against 279ms, because each box re-segmented everything before it. This
	// carries about a word.
	DictTail string
	// Held says the opportunity has already been through the rules once: it was
	// offered, a prohibition moved it past the character in front of it rather
	// than deleting it, and the character it lands on is in the next box.
	//
	// It is Offered and not Deferred, and the difference is which rules still
	// get to run. UAX #14's prohibitions run again — a line may begin with
	// neither of "|!!"'s exclamation marks — but word-break does not, because it
	// already suppressed or allowed this opportunity where it was offered.
	// Reporting a held one as deferred is what broke word-break-keep-all-006.
	Held bool
}

// isLetterUnit reports whether a character is a typographic letter unit in
// §5.2's sense — "the NU, AL, AI, or ID Line Breaking Classes" — which is what
// word-break: keep-all suppresses an opportunity *between*.
//
// A letter or a number, which is those four classes as closely as this engine
// distinguishes them: an ideograph is a letter in Unicode's own categories, so
// ID needs no separate test. Punctuation, spaces and symbols are not, which is
// the half the value's tests are about.
func isLetterUnit(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

// IsLetterUnit is isLetterUnit for the layout package, which asks the same
// question about the character on the far side of a box boundary.
func IsLetterUnit(r rune) bool { return isLetterUnit(r) }

// startsSpacePiece reports whether a character is one SplitAtBreaks gives a
// white-space Piece of its own.
//
// It is the set break-all's opportunities are withheld before — see the call
// site — and it is written as a predicate rather than inlined so the two places
// cannot drift apart: a character that grew a branch below without being added
// here would silently gain a break opportunity before it.
func startsSpacePiece(r rune, ws WhiteSpace) bool {
	switch {
	case r == '\n' || r == '\r':
		return true
	case r == '\t':
		return true
	case r == ' ':
		return true
	case r == '​':
		return true
	}
	return IsOtherSpaceSeparator(r)
}

// betweenTwoSpaces reports whether the boundary between prev and r is one the
// scan would not have broken at inside a run, so a line may not end there.
//
// UAX #14's LB7 is "× SP" and reads as a rule about the character *after* the
// boundary. Asked that way at a box boundary it withholds opportunities the run
// offers, because the run's own answer depends on both characters: under a
// collapsing value every space is a Piece of its own and a break falls between
// any two of them, and under pre and pre-wrap the scan gathers a run of U+0020
// and nothing else — not the tab, and not §4.1's other space separators, which
// are class BA and which a line may perfectly well end in front of.
//
// Two ordinary spaces is the one arrangement it never breaks between, and this
// is that. Asking only about r withheld a break "a&#x2000; &#x2000;" has in
// front of its ordinary space, because the character before it is an EN QUAD
// and the rule never looked; asking about the white-space value as well — is
// this an element that gathers — went the other way and cost
// white-space-mixed-001 a line, because the two spaces at such a boundary can be
// in elements that answer differently and §5.1's common ancestor is not the one
// the run would have consulted.
//
// The zero width space is not here, and LB7 names it. The scan offers a break in
// front of one after anything at all, so withholding it at a boundary would be
// this rule disagreeing with the code it exists to agree with.
func betweenTwoSpaces(prev, r rune) bool { return prev == ' ' && r == ' ' }

// startsSpace reports whether white space follows the text at i. The end of the
// text is not white space: what comes after it is in another box, and whether
// there is anything there at all is not this function's to say.
func startsSpace(text string, i int) bool {
	if i >= len(text) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return unicode.IsSpace(r)
}

// IsIdeographic reports whether a rune breaks on both sides, which is what makes
// CJK line breaking possible without word boundaries.
//
// UAX #14's classes ID and CJ, and the Hangul syllables H2 and H3 — read off
// LineBreak.txt by cmd/genlinebreak, like every other class this package asks
// about. It was six ranges typed out here: the two main CJK blocks, the
// compatibility ideographs, kana, the Hangul syllables and everything from
// U+20000 to U+2FA1F. Halfwidth katakana, the fullwidth Latin letters,
// extensions G and H, Yi, Bopomofo, the Kangxi radicals and the enclosed CJK
// numerals are class ID and were in none of them, so a paragraph of any of them
// was one unbreakable run and overflowed its box — which is what §5.1 forbids
// outright.
func IsIdeographic(r rune) bool { return inLineBreakRanges(r, ideographicRanges[:]) }
