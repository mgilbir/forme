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
	w WritingSystem) ([]Piece, bool) {
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
	dictBreaks := DictionaryBreaks(text)

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
	var clusters segment.Scanner
	// deferBreak says the previous character allows a line to end after it, and
	// the opportunity has not been taken yet.
	//
	// It is deferred because whether the cut is legal depends on the character
	// that *follows*: only that one says whether the cluster ended. Taking the
	// opportunity where it is offered is what cut the syllable open.
	deferBreak := false
	// heldBreak is an opportunity that was offered and moved rather than
	// refused: the character in front of it is one a line may not begin with, so
	// the break belongs after that character instead. It is kept apart from
	// deferBreak because it has already been through the rules once — word-break
	// does not get to suppress it a second time on the far side of the character
	// that displaced it.
	heldBreak := false
	// The character before this one, for the pair rules. See gluedPair.
	var prev rune

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
		beforeIdeograph := IsIdeographic(r) && prev != 0 &&
			!IsIdeographic(prev) && isLetterUnit(prev) && !wb.KeepAll
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
			beforeDictionary = dictBreaks[start]
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
		offered := (deferBreak && !(wb.KeepAll && isLetterUnit(r)) && !startsSpacePiece(r, ws)) ||
			(heldBreak && !startsSpacePiece(r, ws)) ||
			(wb.BreakAll && !startsSpacePiece(r, ws)) || lb.Anywhere ||
			beforeIdeograph || beforeAksara || beforeDictionary ||
			betweenInseparable
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
		if boundary, scored := phrases[start]; scored && !boundary &&
			offered && !lb.Anywhere {
			offered, giveUp = false, true
		}
		if (offered || giveUp) && atBoundary && cur.Len() > 0 {
			flush()
			breakNext, giveUpNext = true, giveUp
		}
		deferBreak, heldBreak = false, held
		prev = r

		switch {
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
			breakNext = ws.BreakSpaces || lb.Anywhere || SeparatorBreaksAfter(r)

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

		case lb.Loose && BreaksAfterUnderLoose(r) && !endsRunOrSpace(text, i):
			// §5.3's one rule the other way round: under "loose" a line may end
			// after a currency sign or a number sign, which belongs to the
			// figure following it and which no other value lets go of.
			//
			// It is written beside the hyphen below because it is the same
			// shape of rule — a character that ends a run and lets the next one
			// begin a line — and it carries the same guard: a prefix with
			// nothing after it offers an opportunity nothing could take.
			cur.WriteRune(r)
			flush()
			breakNext = true

		case (r == '-' || isLatinHyphen(r)) && !endsRunOrSpace(text, i):
			// A hyphen ends a run and the next may begin a line — which is what
			// lets a hyphenated compound break where it is written.
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
			// Not endsRunOrSpace, which is what the ordinary hyphen above uses:
			// the end of *this text* is not the end of the word. The suite's
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
	// breakNext survives the last Piece: it says the text ended at an
	// opportunity, which matters when what follows is in another box. A deferred
	// one counts, and has to: text ending in an ideograph offers a break to
	// whatever box comes next, and the character that would have confirmed it is
	// in that box rather than this one.
	return out, breakNext || deferBreak
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

// endsRunOrSpace reports whether the text at i is the end of the run or white
// space, which is what stops a trailing hyphen being a break opportunity: there
// would be nothing after it to move to the next line.
func endsRunOrSpace(text string, i int) bool {
	if i >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return unicode.IsSpace(r)
}

// IsIdeographic reports whether a rune breaks on both sides, which is what makes
// CJK line breaking possible without word boundaries.
func IsIdeographic(r rune) bool {
	switch {
	case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
		return true
	case r >= 0x3400 && r <= 0x4DBF: // Extension A
		return true
	case r >= 0xF900 && r <= 0xFAFF: // Compatibility Ideographs
		return true
	case r >= 0x3040 && r <= 0x30FF: // Hiragana and Katakana
		return true
	case r >= 0xAC00 && r <= 0xD7AF: // Hangul syllables
		return true
	case r >= 0x20000 && r <= 0x2FA1F: // Extensions B and beyond
		return true
	}
	return false
}
