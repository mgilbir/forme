package paragraph

import (
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
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
// and is the shape of silent difference a finding exists to name.
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
	// EndsBidiParagraph says this break ends the bidi paragraph as well as the
	// line, which is true of a preserved newline and of two of the five
	// mandatory breaks and false of the other three. See endsBidiParagraph,
	// which reads UAX #9's class rather than listing the characters again.
	EndsBidiParagraph bool
	TrimAtEnd         bool
	Tab               bool
	Segment           bool
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
// They are UAX #14's, all of it — see uax14.go — with what CSS Text changes
// about them applied on top, each change named where it is made. The one place
// the engine knows it gives a poorer answer than it should is the scripts a
// dictionary breaks, and checkScript reports text that needs one it lacks.
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
	// Context is what UAX #14's rules need of the text before the boundary,
	// as that text left it: see Trailing.Context. The rules at this text's
	// first character are asked with it, so the boundary is decided exactly
	// as it would be inside one run.
	//
	// A caller that has no context to give and says only which character came
	// before — Prev and PrevBase — gets the context of that character alone.
	Context BreakContext
	// Ahead is the text after this one, as far as UAX #14's rules look past a
	// character: "× QU_Pf" asks what follows the quotation mark and "PR × OP
	// NU" what follows the bracket, and where the character is this text's
	// last, what follows is in another box. NeedsLookahead says when it is
	// wanted. Two units are enough, and a unit is a character and the marks
	// after it — see ContinuesUnit — so it is the text up to the third
	// character that begins one, however many marks come between.
	//
	// Where it ends short of that, the text ends there as far as the rules
	// are concerned: at the end of the paragraph, at a forced break — which
	// every rule that looks ahead reads as it reads the end of the text — or
	// at an atomic inline, which is not text. See BreakContext.AfterObject.
	Ahead string
	// Clusters is the grapheme cluster scan as the text before this one left
	// it: see Trailing.Clusters. Where it has read nothing and Prev is set, the
	// boundary at this text's first character is answered from Prev alone,
	// by the rules one character can answer.
	Clusters segment.Scanner
	// Orthography is the language's rules for a word hyphenated inside it,
	// which decide what a line broken at a soft hyphen begins with. See the
	// scan, and Orthography.HyphenateBetween.
	Orthography Orthography
	// Decided says the caller has already placed the opportunity in front of
	// this text somewhere else — at the margin edge of an inline box, where
	// CSS Text §5 puts a break before the first character of a box — so the
	// rules are not to offer it again between the margin and the text.
	Decided bool
	// Offered says the text before left an opportunity at the boundary at all.
	// The two below say which kind, and are meaningless without it.
	Offered bool
	// Deferred says the opportunity is UAX #14's, which the character after
	// the boundary — this text's first — decides with Context. The flag is
	// then only what the text before could say without that character, and
	// the rules here answer instead of it.
	Deferred bool
	// Held is what Deferred used to be split from, when a prohibition moved
	// an opportunity one character on rather than refusing it: the rules now
	// decide each boundary as UAX #14 does, and nothing is moved. It is read as
	// Deferred is.
	//
	// Neither set means the opportunity is not UAX #14's to decide — an atomic
	// inline left it, or a <wbr>, or a hyphenation point — and it is taken here
	// unless LB7 moves it past a space.
	Held bool
	// Prev is the last character before the boundary, for the grapheme cluster
	// rules at it and for a caller that gives no Context. It is zero at the start
	// of a paragraph, and after an atomic inline, which is not a character.
	Prev rune
	// PrevBase is the last *base* character before the boundary: Prev with the
	// marks and the invisibles stepped over, which is what the rules stated over
	// typographic character units read, and what a caller that gives no Context
	// has its context built from. It is zero at the start of a paragraph, and
	// where nothing before the boundary has a base.
	//
	// Prev cannot stand in for it where Prev is a mark, and a box's text may end
	// in one. "aࠩ踢" under keep-all has an opportunity in front of the
	// ideograph that the value demotes rather than removes, because the letter
	// unit in front of it is the "a" and not its mark; written
	// "<span>aࠩ</span><span>踢</span>" the scan of the second box saw only the
	// mark, found no letter unit, and the line could not break there at all.
	PrevBase rune
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
	// PhraseBefore and PhraseAfter are the text either side of this one as
	// "word-break: auto-phrase" needs it: up to PhraseContext characters of
	// each, which is as far as the phrase model reads from a boundary.
	//
	// They are Before and After's twins and not the same fields, because the
	// two questions reach different distances over different text. A
	// dictionary needs a word of its own script; the model needs three
	// characters of anything. Without them each box was scored alone, so the
	// boundary at a box's first character was never scored at all and the
	// ones near its edges were scored without their neighbours: in
	// "日本語を勉強します" the break between 勉 and 強 is inside a phrase and is
	// withheld, and written "日本語を勉<a>強します</a>" it was a full opportunity.
	// Audit C122.
	PhraseBefore, PhraseAfter string
	// Taken says the text before this one ended at an opportunity CSS gives
	// whatever the rules say. See Trailing.Taken.
	Taken bool
	// Next is the first character of the text that follows this one, or zero
	// where nothing does.
	//
	// Prev's counterpart, and got the other way about: the text before a box has
	// been flattened by the time the box is, and the text after it has not, so
	// this is read off the tree. See layout.textAfter.
	//
	// One character is all anything needs. The arms that read it ask a question
	// about the character on the far side of the boundary and nothing beyond it
	// — is there white space there, so that ending a line here would move
	// nothing down. See startsSpace.
	Next rune
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
	//
	// With the characters either side of the text, which the model reads: see
	// Carried.PhraseBefore.
	var phrases map[int]bool
	var phraseTail string
	if wb.AutoPhrase {
		phrases = PhraseBreaksBetween(at.PhraseBefore, text, at.PhraseAfter, w)
		phraseTail = lastRunes(at.PhraseBefore+text, PhraseContext)
	}

	// Grapheme cluster boundaries, walked in lockstep with the scan.
	//
	// It runs for every value of word-break and not only for break-all, because
	// the rule it enforces is not break-all's: CSS Text §5's line breaking
	// details say "CSS never allows soft wrap opportunities within typographic
	// character units", so no opportunity this function produces may fall
	// inside a cluster. UAX #14 alone would produce some — it breaks between a
	// space and a combining mark after it, which LB10 makes a letter of its own —
	// and the ideograph rule this package had before it cut a Hangul syllable
	// from its own trailing jamo.
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
	//
	// It begins where the text before this one left it (Carried.Clusters), so
	// a cluster written across a box boundary is one cluster by every rule of
	// UAX #29 — a conjunct, an emoji sequence, a flag — and not only by the
	// rules one character can answer.
	clusters := at.Clusters

	// UAX #14, with what CSS makes of it for this box's values. See uax14.go.
	//
	// The context is what the text before this one left, so a rule that looks
	// back — "OP SP* ×", a number, a pair of regional indicators — looks back
	// across the boundary as it would inside a run. A caller that says only
	// which character came before gets the context of that one character, which
	// is what every rule that reads one unit back needs.
	tl := lbTailoring{lb: lb, wb: wb}
	ctx := at.Context
	if !ctx.Started() && at.Prev != 0 {
		ctx = contextOf(tl, at.PrevBase, at.Prev, hy)
	}
	ctx = ctx.under(tl)
	// An opportunity the text before this one left that is not UAX #14's to
	// decide: an atomic inline's, a <wbr>'s, a hyphenation point's, a preserved
	// space's under break-spaces. It is taken at the first character rather
	// than here, because LB7 still applies to it — see below.
	explicit := at.Offered && (at.Taken || (!at.Deferred && !at.Held))
	// Whether there is text in front of this one at all, which is what decides
	// that an opportunity falling at the very first character is a real one:
	// a break in front of the first thing on the first line is not a break.
	afterText := explicit || ctx.Started()

	// explicitNext says breakNext is an opportunity CSS gives rather than one
	// the rules found at this boundary — an atomic inline's, a <wbr>'s, a
	// hyphenation point's, a preserved space's under break-spaces — which is
	// what the text hands on as Trailing.Taken when it ends there. keepNext says it survives a
	// white-space piece, which is LB7 moving it past a space: see emit.
	explicitNext, keepNext := false, false
	flush := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, Piece{Text: cur.String(), BreakBefore: breakNext, LastResort: giveUpNext})
		cur.Reset()
		breakNext, giveUpNext, explicitNext, keepNext = false, false, false, false
	}
	// flushHyphen is flush for a piece that ends at a soft hyphen. It is
	// separate rather than a parameter because every other caller passes false
	// and a bare boolean argument says nothing about which end of the line it
	// is about.
	flushHyphen := func() {
		if cur.Len() == 0 {
			return
		}
		out = append(out, Piece{Text: cur.String(), BreakBefore: breakNext,
			LastResort: giveUpNext, Hyphen: true})
		cur.Reset()
		breakNext, giveUpNext, explicitNext, keepNext = false, false, false, false
	}
	// A white-space piece takes the pending opportunity. It consumes one the
	// rules found, because the boundary after the space is a boundary of its
	// own and the rules decide it when they reach it: "ab-\u2007cd" may break
	// after the hyphen, and a no-break space glues the letters after it to
	// itself. It does not consume one CSS gave: that one belongs after the
	// space, LB7 being an earlier rule than whatever made it — an earlier
	// version that cleared the flag here lost the opportunity after "a<img> b".
	emit := func(p Piece) {
		p.BreakBefore, p.LastResort = breakNext, giveUpNext
		out = append(out, p)
		if !keepNext {
			breakNext, giveUpNext, explicitNext = false, false, false
		}
	}

	for i := 0; i < len(text); {
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		start := i
		i += size

		atBoundary := clusters.Boundary(r)
		// The first character's cluster boundary is a question about the text
		// in front of it, which is in another box. A caller that hands on the
		// Scanner has it answered by every rule; one that says only which
		// character came before gets the rules that character can answer — see
		// clusterContinues — and a scanner that starts here calls the first
		// character a boundary (GB1) otherwise. Without either an opportunity
		// carried over the boundary cut a syllable whose jamo were written in
		// two boxes, and an ideograph from a combining mark in the next box.
		if start == 0 && at.Prev != 0 && !at.Clusters.Started() {
			atBoundary = !clusterContinues(at.Prev, r)
		}
		// After a space, a combining mark or a joiner begins a unit of its own
		// for line breaking, though UAX #29 puts it in the space's cluster
		// (GB9). CSS Text lets the typographic character unit be tailored "as
		// required by typographic tradition ... differently depending on the
		// operation", and UAX #14 is the tradition for this one: LB9 does not
		// attach a mark to a space and LB10 makes it a letter of its own, so
		// the space is where the line ends. white-space-vs-joiners-002 is the
		// case: "&#x200d;is&#x200d;" between spaces, whose words must wrap as
		// if the joiners were not there.
		if ctx.lastRune == ' ' {
			atBoundary = true
		}

		c := tl.char(r)
		brk, rule := ctx.decide(c, lbAhead{t: tl, rest: text[i:], more: at.Ahead})
		// A break at a soft hyphen is a hyphenation, and CSS Text's hyphens
		// says a UA "should apply any appropriate spelling changes just as for
		// automatic hyphenation at the same point". Where the language takes
		// the characters after the hyphen off the next line — pinyin's
		// syllable apostrophe, "tú’àn" as "tú‐" and "àn" — the line would
		// begin with what follows them, and that is what the rules are asked
		// about: UAX #14 keeps a quotation mark with what precedes it, and the
		// apostrophe is not there to be kept (hyphens-i18n-manual-003).
		if brk == lbProhibited && ctx.lastRune == 0x00AD && hy.Soft() {
			if drop := at.Orthography.HyphenateBetween("", text[start:]).Dropped; drop > 0 &&
				start+drop < len(text) {
				after, _ := utf8.DecodeRuneInString(text[start+drop:])
				rest := text[start+drop+utf8.RuneLen(after):]
				probe := ctx
				brk, rule = probe.decide(tl.char(after), lbAhead{t: tl, rest: rest, more: at.Ahead})
			}
		}
		// The boundary in front of this text was already given to a box's
		// margin edge by the caller — CSS Text §5's "the break occurs
		// immediately before/after the box (at its margin edge)" — and a
		// second one here would put a break between the margin and the word it
		// pushes along.
		if start == 0 && at.Decided && brk == lbAllowed {
			brk = lbProhibited
		}
		offered := brk != lbProhibited
		// The opportunities a space and a zero width space make, LB18 and LB8,
		// and the mandatory ones. They are not *implicit*: word-break's keep-all
		// suppresses only "implicit soft wrap opportunities", and auto-phrase
		// "must not suppress wrapping opportunities introduced by wbr or ZWSP".
		separator := brk == lbMandatory || rule == ruleAfterSpace || rule == ruleAfterZW
		if brk == lbAllowed {
			offered = !cssRefuses(ctx, c)
		} else if brk == lbProhibited && rule == ruleHyphenNumber {
			offered = cssAllowsHyphenNumber(ctx)
		}
		// A soft hyphen under "hyphens: none" is not an opportunity. CSS Text
		// §5.4: "Words are not hyphenated, even if characters inside the word
		// explicitly define hyphenation opportunities". UAX #14 makes it class
		// BA, which breaks after; the property of the box the hyphen is in is
		// the one that decides, and the context carries it.
		if offered && brk != lbMandatory && ctx.noBreakAfter {
			offered = false
		}
		// §5's lexical breaking, for the scripts that write no spaces between
		// their words: UAX #14 resolves SA to AL, which never breaks, and CSS
		// Text says "a lexical resource is needed to correctly identify soft
		// wrap opportunities in such texts". Where this engine has the
		// language's vocabulary the opportunity is at a word boundary and
		// nowhere else — see DictionaryBreaks — and where it has not it is the
		// fallback §5.1 requires, "a soft wrap opportunity between pairs of
		// typographic letter units in that writing system", which is known to
		// be in the wrong place. UnsupportedScript is what says which of the two
		// a document got.
		//
		// Between two such letters and nowhere else. A digit or a Latin letter
		// in front of Thai is not a pair "in that writing system", and UAX #14's
		// answer there — AL × AL, NU × AL — stands.
		if !wb.KeepAll && !wb.Manual && !wb.BreakAll && brk == lbProhibited &&
			saLetter(ctx.unit.lbChar) && saLetter(c) {
			offered = true
			if HasDictionary(r) {
				offered = dictBreaks[dictAt+start]
			}
		}
		// line-break: anywhere, which is the widest: §5.3 puts an opportunity
		// around *every* typographic character unit, "disregarding any
		// prohibition against line breaks", so it needs nothing UAX #14 says.
		if lb.Anywhere {
			offered = true
		}
		if start == 0 {
			// LB7 at the boundary, for an opportunity that is not UAX #14's to
			// decide: a line may not end in front of a space, so the one an
			// atomic inline or a <wbr> leaves is taken after the space rather
			// than before it — unless break-spaces overrules LB7, which is §3's
			// sentence: "there is a soft wrap opportunity after every preserved
			// white space character, including between white space characters".
			// See betweenTwoSpaces.
			endsInFrontOfASpace := betweenTwoSpaces(at.Prev, r) && !at.SpaceMayTakeIt
			if explicit && !endsInFrontOfASpace {
				// Past a space, which LB7 says a line may not end in front of;
				// in front of anything else, here — a no-break space included,
				// which is content and goes to the next line with what follows.
				breakNext, explicitNext, keepNext = true, true, r == ' ' || r == '\t'
			}
		}
		// word-break: keep-all, which §5.2 makes a preference rather than a
		// rule:
		//
		//	Breaking is forbidden within "words": implicit soft wrap
		//	opportunities between typographic letter units (or other
		//	typographic character units belonging to the NU, AL, AI, or ID
		//	Unicode line breaking classes) are suppressed [...] Note: this
		//	value may be relaxed by the UA if there are no otherwise-acceptable
		//	break points in the line.
		//
		// The suite's overflow-wrap-normal-keep-all-001 asserts the note with
		// eight ideographs in a box of no width at all: nowhere else on the line
		// can the break go, so keep-all gives way and the column comes out one
		// character wide. "Relaxed if there is nothing else" is what
		// Piece.LastResort already means, so the opportunity is demoted rather
		// than withheld — the same two steps auto-phrase takes below.
		//
		// Both sides of the boundary are asked, and both are the typographic
		// character unit's base: the opportunity between an ideograph and the
		// comma after it is not between letter units and is not keep-all's to
		// take (word-break-keep-all-006), and a mark after a letter is part of
		// the letter.
		//
		// Demoted where the text is CJK — where either side breaks like an
		// ideograph, which is where the value is used and where the suite tests
		// the relaxation — and withheld outright elsewhere: between two aksara
		// clusters of Javanese, or a Latin letter and one, keep-all is the
		// author asking for the word to stay whole, and there is a word
		// boundary to break at instead.
		giveUp := false
		if offered && !separator && !lb.Anywhere && wb.KeepAll &&
			keepAllUnit(ctx.unit.lbChar) && keepAllUnit(c) {
			offered = false
			giveUp = breaksLikeAnIdeographClass(ctx.unit.lbChar) || breaksLikeAnIdeographClass(c)
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
		// line-break: anywhere is exempt, as it is from every other rule here:
		// §5.3 puts an opportunity around every typographic character unit and
		// says so in a sentence written to overrule the rest of §5. So is a
		// space and a zero width space — word-break-auto-phrase-007: "UAs must
		// not suppress wrapping opportunities introduced by wbr or ZWSP".
		if boundary, scored := phrases[start]; scored && !boundary &&
			offered && !separator && !lb.Anywhere {
			offered, giveUp = false, true
		}
		if (offered || giveUp) && atBoundary && (start > 0 || afterText) {
			// A piece that ends at a soft hyphen is marked so, which is what
			// prints the hyphen when the line ends here. See Piece.Hyphen.
			if offered && ctx.lastRune == 0x00AD && hy.Soft() {
				flushHyphen()
			} else {
				flush()
			}
			breakNext, giveUpNext = true, giveUp && !breakNext
		}
		ctx.advance(c, hy)

		switch {
		case IsMandatoryBreak(r):
			// UAX #14's BK and NL: a character that ends a line wherever it
			// appears, which is not the same thing as a segment break. A
			// segment break is collapsible — a newline under "white-space:
			// normal" becomes a space and the line goes on — and these are not:
			// LB4 and LB5 make the break mandatory, and CSS Text says so in as
			// many words: "any Unicode character with the BK and NL line
			// breaking class, must be treated as forced line breaks".
			//
			// They reached here as ordinary characters and were set as ordinary
			// characters, so "1<FF>2" came out on one line with a notdef box
			// between the digits. line-breaking-022 writes all five between
			// spans in a column one character wide and asks for six lines.
			//
			// Where the character goes is the difference between a control
			// character and a separator.
			//
			// §5.1's note is written about the first — "control characters other
			// than [tab, newline] ... are otherwise rendered as a visible glyph"
			// — and the suite asks for it by name: three of the
			// white-space/control-chars-0XX documents are mismatch references
			// against a blank page, one of them saying "U+000C, which is in the
			// unicode category CC, must be visible". So a control character is
			// written into the piece that ends the line and is set with it.
			//
			// U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR are not
			// control characters. They are categories Zl and Zp, they exist to
			// separate, and nothing draws a separator. They go where a newline
			// goes: on the break piece itself, whose text layout does not set —
			// so the character is not lost from the pieces and not put on the
			// page either. Written into the piece before them, they were two
			// glyphs a document does not contain, which is what CSS2's
			// bidi-breaking-003 sees.
			//
			// "The pieces spell the input" is the invariant that decides this
			// rather than either reading of §5.1, and it is one a fuzzer found:
			// see TestAMandatoryBreakKeepsItsCharacter and
			// testdata/fuzz/FuzzSplitAtBreaks. Swallowing the separator outright
			// satisfies the reftest and loses a character.
			carried := ""
			if charprop.Is(r, charprop.Cc) {
				cur.WriteRune(r)
			} else {
				carried = string(r)
			}
			flush()
			emit(Piece{Text: carried, Space: true, Segment: true,
				EndsBidiParagraph: endsBidiParagraph(r)})
			breakNext = true

		case r == '\n' || r == '\r':
			// Only a *preserved* break reaches here: Phase I turned a
			// collapsible one into a space. A CR is folded with the LF that may
			// follow it, so that text which reached this stage without going
			// through Phase I — a caller measuring raw content — still counts
			// one break rather than two. The LF goes through the rules too,
			// which is LB5's "CR × LF": the break is after the pair.
			if r == '\r' && i < len(text) && text[i] == '\n' {
				i++
				ctx.advance(tl.char('\n'), hy)
				clusters.Boundary('\n')
			}
			flush()
			emit(Piece{Text: "\n", Space: true, Segment: true, EndsBidiParagraph: true})
			breakNext = true

		case r == '\t' && !ws.Collapse:
			// A preserved tab is its own Piece because each one advances to its
			// own tab stop, so two of them are not one run of a doubled width.
			//
			// A line may end after it where UAX #14 says so — a tab is class
			// BA — and after every one under break-spaces, whose opportunity
			// "after every preserved white space character" is CSS's and not
			// the rules'.
			flush()
			emit(Piece{Text: "\t", Space: true, Tab: true})
			if ws.BreakSpaces {
				breakNext, explicitNext, keepNext = true, true, true
			}

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
			//
			// Whether a line may end after one is UAX #14's — most are class BA
			// and U+2007 and U+202F are GL, which glues what follows to what
			// precedes — except under break-spaces, which puts "a soft wrap
			// opportunity ... after every other space separator (including
			// between adjacent spaces)". UAX #14 would keep two ideographic
			// spaces together (× BA), and trailing-ideographic-space-break-spaces
			// asks for a run of them to wrap one at a time.
			//
			// The two GL separators are the exception, and it is the suite's
			// rather than the sentence's: trailing-other-space-separators-
			// break-spaces-009 and -013 are those two, the only two of the
			// fifteen where the answers part company, and both keep what follows
			// on the line.
			flush()
			emit(Piece{
				Text: text[start:i], Space: true,
				TrimAtEnd: r == 0x1680 && ws.Collapse,
			})
			if ws.BreakSpaces && SeparatorBreaksAfter(r) {
				breakNext, explicitNext, keepNext = true, true, true
			}

		case r == ' ' || r == '\t':
			flush()
			if ws.Collapse {
				// Phase I already reduced the run to a single space and turned
				// any tab into one, so there is nothing left to gather.
				emit(Piece{Text: " ", Space: true, Collapsible: true, TrimAtEnd: true})
				break
			}
			// Preserved. Under pre and pre-wrap the run hangs or wraps as a
			// unit, so it is gathered — unless break-spaces or line-break:
			// anywhere says a line may end between any two of them, which is a
			// run that is no longer one thing.
			if !ws.BreakSpaces && !lb.Anywhere {
				for i < len(text) && text[i] == ' ' {
					ctx.advance(c, hy)
					clusters.Boundary(' ')
					i++
				}
			}
			emit(Piece{Text: text[start:i], Space: true})
			if ws.BreakSpaces {
				breakNext, explicitNext, keepNext = true, true, true
			}

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
			// what it does is stand between its neighbours. The opportunity
			// after it is LB8's, "ZW SP* ÷", which the next character asks.
			flush()
			emit(Piece{Text: text[start:i], ZeroWidth: true})

		case BreaksLikeAnIdeograph(r):
			// An ideograph begins a piece of its own where a cluster begins,
			// whether or not a line may break in front of it. A piece is what a
			// line is built of, and the pieces were cut this way when the
			// ideograph was the only character this package offered an
			// opportunity around; cutting them the same way keeps a piece from
			// changing shape under a change that is about where lines may end.
			//
			// Not inside a cluster, for the reason the opportunity is not taken
			// there: a piece that began inside a syllable would carry part of
			// one typographic character unit — a run of its own, spaced by
			// letter-spacing as a unit of its own.
			if atBoundary {
				flush()
			}
			cur.WriteRune(r)

		case r == 0x00AD && hy.Soft() && i == len(text) && !startsSpace(text, i, at.Next):
			// A soft hyphen at the end of the text. CSS Text's hyphens: a
			// "conditional hyphenation point", where a hyphen is printed if the
			// line breaks. Inside the text the piece is cut and marked where the
			// next character takes the opportunity, above; at the end the next
			// character is in another box, and it is that box's rules that
			// decide whether a line may end here — so the piece is marked now,
			// and the opportunity goes with the context.
			//
			// The character stays in the piece rather than being dropped. It
			// takes no room and sets no paper — every face here shapes it to
			// nothing, and shape/ignorable.go is where that is decided — so
			// keeping it costs nothing on the page, and it keeps the text of the
			// document the text the author wrote.
			//
			// The suite's hyphens-span-001 writes the same word nine ways —
			// "<span>high&shy;</span>way", "high<span>&shy;</span>way",
			// "high&shy;<span>way</span>" — and asks for one answer from all of
			// them. A space after it is not an opportunity at all: there would
			// be nothing to move to the next line.
			cur.WriteRune(r)
			flushHyphen()

		default:
			cur.WriteRune(r)
		}
	}
	flush()
	// What the boundary after this text will be, as far as this text can say:
	// whether UAX #14 and CSS would let a line end in front of an ordinary letter
	// written next. The box that holds the next character decides the boundary
	// for itself, from the context; this answers the callers that have to say
	// something before there is a next character — an inline box's margin, which
	// takes the opportunity in front of it at its edge.
	probe := ctx
	next := tl.char('a')
	end, endRule := probe.decide(next, lbAhead{t: tl})
	deferred := end != lbProhibited && !cssRefuses(ctx, next) && !ctx.noBreakAfter
	if end == lbProhibited && endRule == ruleHyphenNumber {
		deferred = false
	}
	return out, Trailing{
		DictTail:   dictionaryTail(dictSeg, dictBreaks),
		PhraseTail: phraseTail,
		Context:    ctx,
		Clusters:   clusters,
		Offered:    (breakNext && explicitNext) || deferred,
		// Taken is an opportunity this text left that is not UAX #14's to
		// decide — a preserved space's under break-spaces — and Deferred is one
		// the next character decides. Both can be true at
		// once, and the one CSS gives wins: a preserved space under
		// break-spaces followed by a closing bracket is an opportunity CSS
		// gives and one UAX #14 refuses.
		Taken:    breakNext && explicitNext,
		Deferred: deferred,
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
	// Context is what UAX #14's rules need of this text at its end, for the
	// box that holds the next character: see Carried.Context. It is the only
	// field the next box's rules read; the rest are for callers that have to
	// say something about the boundary before there is a next character.
	Context BreakContext
	// Clusters is the grapheme cluster scan at the end of this text, for the
	// box that holds the next character. UAX #29's rules for a conjunct
	// (GB9c), an emoji sequence (GB11) and a pair of regional indicators (GB12,
	// GB13) look further back than one character, and a box boundary may fall
	// anywhere inside what they look at: "<span>🇷🇺🇸</span><span>🇪</span>" is
	// two flags, and the second box's first character ends the second.
	Clusters segment.Scanner
	// Offered says the text ended at an opportunity the next box may take:
	// Taken, or Deferred.
	Offered bool
	// Deferred says UAX #14 and CSS would let a line end after this text in
	// front of an ordinary letter. The next character decides for itself from
	// Context; this is the answer for an inline box's margin, which takes the
	// opportunity at its edge before its first character is known.
	//
	// It cannot be read off the last character, and an attempt to is what an
	// earlier form of this field replaced: "0|!" is one unbreakable run, and
	// "<span>0|</span><span>!</span>" broke in two because nothing asked LB13
	// about the exclamation mark.
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
	// PhraseTail is what the next box needs as its Carried.PhraseBefore: the
	// last PhraseContext characters of this text, reaching back into the text
	// before it where this one is shorter. Empty unless "auto-phrase" asked.
	PhraseTail string
	// Taken says the text ended at an opportunity CSS gives whatever UAX #14
	// says of the character after it: a preserved space's under break-spaces,
	// which is "a soft wrap opportunity after every preserved white space
	// character", or one of the separators' it names beside it.
	//
	// It is a field rather than the absence of Deferred, because a boundary can
	// be both at once. See the note where Trailing is returned.
	//
	// Offered is still the switch over both. An inline box's own margin takes
	// the opportunity in front of it and clears the flag — a line may end
	// before "<span style='margin-left: 99px'>word</span>" and may not end
	// between that margin and the word — and reading this one without asking
	// Offered first put the break back, with the margin left on the line above.
	Taken bool
	// Held is never set. It said an opportunity had been moved past a
	// character a line may not begin with, which is how this package
	// approximated UAX #14's pair rules before it ran them; the rules decide
	// each boundary now and nothing is moved. It is kept because it is part of
	// what callers outside this package build a Carried from.
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
	return charprop.Is(r, charprop.L|charprop.N)
}

// IsLetterUnit is isLetterUnit for the layout package, which asks the same
// question about the character on the far side of a box boundary.
func IsLetterUnit(r rune) bool { return isLetterUnit(r) }

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

// startsSpace reports whether white space follows the text at i, where next is
// the first character of whatever follows the text itself.
//
// The soft hyphen's arm is gated on it: a line that ends in front of white space
// has nothing to move down to the next one, so a piece that ends at a soft
// hyphen with white space after it is not marked for a hyphen. The gate has to
// see across a box boundary for the same reason the rest of Carried does —
// "high&shy;<span> way</span>" is "high&shy; way", and the hyphen in it ends no
// line.
//
// It used to answer false at the end of the text, on the grounds that what comes
// after it is in another box and not this function's to say. That was true when
// nothing could tell it: the cost was an opportunity a box invented at its own
// last character, and "⭋‐&#x2000;" written in two boxes was a sixty-fourth of a
// pixel wider than the same text written in one.
//
// "White space" is the kind a line may not end in front of, and it is not
// unicode.IsSpace, which was the test. That one holds the no-break spaces —
// U+00A0, U+2007 FIGURE SPACE and U+202F NARROW NO-BREAK SPACE — which are
// class GL, and UAX #14's LB12a exempts a soft hyphen (class BA) from GL:
// "[^SP BA HY] × GL", so a hyphen or a soft hyphen may end a line in front of a
// no-break space. The no-break space is content that goes to the next line
// with what follows it, so ending here moves something down after all. "ab-
// cd" with a no-break space was one unbreakable piece. Audit C175.
func startsSpace(text string, i int, next rune) bool {
	if i >= len(text) {
		return spaceFollows(next)
	}
	r, _ := utf8.DecodeRuneInString(text[i:])
	return spaceFollows(r)
}

// spaceFollows is startsSpace's question about one character: is it CSS's white
// space — a space, a tab, a segment break — or one of the other space separators
// a line may not end in front of (class BA: noBreakBeforeRanges holds it), or a
// break a line takes anyway. Everything unicode.IsSpace says yes to except the
// three no-break spaces.
func spaceFollows(r rune) bool {
	switch {
	case r == ' ' || r == '\t' || r == '\n' || r == '\r':
		return true
	case IsMandatoryBreak(r):
		return true
	}
	return charprop.Is(r, charprop.Zs) && inLineBreakRanges(r, noBreakBeforeRanges[:])
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

// clusterContinues reports whether r continues the grapheme cluster prev is the
// last character of, as far as the two characters alone decide it: UAX #29's
// GB3 to GB9b, in their order. GB3, GB4 and GB5 put a boundary round a
// control; GB6, GB7 and GB8 hold a Hangul syllable together; GB9, GB9a and
// GB9b hold a mark, a joiner and a spacing mark to what precedes them and a
// prepended character to what follows it.
//
// The rules after those — GB9c's conjuncts, GB11's emoji sequences and the
// regional indicator pairs of GB12 and GB13 — are decided by more of the text
// than one character, and are answered as a boundary here. It is asked only
// where a caller gives the character before the boundary and not the scan
// (Carried.Clusters), which answers all of them.
func clusterContinues(prev, r rune) bool {
	p, c := segment.BreakOf(prev), segment.BreakOf(r)
	switch {
	case p == segment.CR && c == segment.LF:
		return true
	case p == segment.Control || p == segment.CR || p == segment.LF,
		c == segment.Control || c == segment.CR || c == segment.LF:
		return false
	case p == segment.HangulL && (c == segment.HangulL || c == segment.HangulV ||
		c == segment.HangulLV || c == segment.HangulLVT):
		return true
	case (p == segment.HangulLV || p == segment.HangulV) &&
		(c == segment.HangulV || c == segment.HangulT):
		return true
	case (p == segment.HangulLVT || p == segment.HangulT) && c == segment.HangulT:
		return true
	case c == segment.Extend || c == segment.ZWJ || c == segment.SpacingMark:
		return true
	case p == segment.Prepend:
		return true
	}
	return false
}

// BreaksLikeAnIdeograph reports whether a rune takes part in the ideograph's
// line breaking: IsIdeographic, and the Hangul conjoining jamo.
//
// The jamo are UAX #14's JL, JV and JT, a syllable spelt in its letters. Between
// syllables they break as the precomposed syllables H2 and H3 do — LB31 allows
// it, and LB27 keeps them to a postfix and a prefix as LB23a keeps ID — and
// inside one LB26 forbids it. LB26 is GB6, GB7 and GB8 class for class, so a
// syllable is a grapheme cluster and the scan's cluster boundary is the
// syllable boundary: an opportunity offered after every jamo is taken only
// where a syllable ends. keep-all and normal treat them as they treat the
// syllables, because the jamo are letters as the syllables are.
//
// Without them "각각" spelt in six jamo had no opportunity in it at all, where
// the same two syllables precomposed had one. It is a separate predicate from
// IsIdeographic because that one is asked other questions — whether a line is
// justified between its characters — that are about characters and not about
// where a syllable ends.
func BreaksLikeAnIdeograph(r rune) bool {
	return IsIdeographic(r) || inLineBreakRanges(r, jamoRanges[:])
}

// NeedsFollowingCharacter reports whether the scan's answer for a text ending in
// r depends on the character after it.
//
// One arm does: a soft hyphen that ends a text marks its piece for a hyphen
// unless white space follows, and at the end of a box the white space is in the
// next one. It is asked so that the walk that fetches that character can be
// skipped for every other character. See startsSpace and Carried.Next.
//
// The hyphen and §5.3's loose-break characters asked it too, when they took an
// opportunity after themselves unless white space followed. UAX #14 decides the
// boundary after them now, in the box that holds the character after it, and
// the rules that look further than one character past a box's end read
// Carried.Ahead instead. The parameters are kept because callers pass them.
func NeedsFollowingCharacter(r rune, lb LineBreak, hy Hyphens) bool {
	return r == 0x00AD && hy.Soft()
}
