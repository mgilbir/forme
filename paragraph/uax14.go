package paragraph

import (
	"sort"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
)

// UAX #14, the Unicode line breaking algorithm, in full.
//
// This package used to implement a subset of it: the prohibitions that can be
// answered by looking at one character ("× CL", "× NS"), the opportunity after
// an ideograph and around a space, and a table of pairs for the few places
// word-break: break-all needed one. The rest was left out on the grounds that
// it depends on more than one character, and the jamo work (#764) measured
// what that cost against Unicode's own LineBreakTest.txt: of its 19,338 cases,
// 5,768 broke somewhere the algorithm does not or did not break somewhere it
// does. A per-cent sign could begin a line after an ideograph, a quotation mark
// after one, a vertical tab could be broken in front of; a line could not end
// between a closing bracket and an ideograph, or between two regional
// indicator pairs.
//
// So the rules are here as UAX #14 states them, in LineBreakTest.html's
// numbering — which is the file's own, and which the conformance test reports
// per rule — and SplitAtBreaks asks them at every boundary. What CSS Text
// changes about them is applied on top, in the scan and in lbTailoring, and
// paragraph/linebreakconformance_test.go holds the two apart: every case of the
// file is run through SplitAtBreaks, and a disagreement is allowed only where a
// named CSS rule makes it.
//
// # The shape of it
//
// The rules are stated over *units*: LB9 makes a combining mark or a zero width
// joiner part of the character before it, so "X CM* → X", and every rule after
// it sees the unit's base. Some rules look back further than one unit — "OP
// SP* ×" over a run of spaces, "NU (SY | IS)* × NU" over a number, the regional
// indicator pairs by counting — and a few look ahead: "× QU_Pf" depends on what
// follows the quotation mark, "PR × OP NU" on what follows the bracket. The
// history is kept in a BreakContext, which is small and is carried across a box
// boundary exactly as the text before it left it; the lookahead is read from
// the text, and past the end of a box from Carried.Ahead.

// lbClass is a Line_Break class. The zero value is lbNone, which is no class at
// all: the start of the text, where UAX #14 writes "sot".
type lbClass uint8

const (
	lbNone lbClass = iota
	lbAI
	lbAK
	lbAL
	lbAP
	lbAS
	lbB2
	lbBA
	lbBB
	lbBK
	lbCB
	lbCJ
	lbCL
	lbCM
	lbCP
	lbCR
	lbEB
	lbEM
	lbEX
	lbGL
	lbH2
	lbH3
	lbHH
	lbHL
	lbHY
	lbID
	lbIN
	lbIS
	lbJL
	lbJT
	lbJV
	lbLF
	lbNL
	lbNS
	lbNU
	lbOP
	lbPO
	lbPR
	lbQU
	lbRI
	lbSA
	lbSG
	lbSP
	lbSY
	lbVF
	lbVI
	lbWJ
	lbXX
	lbZW
	lbZWJ
)

// lineBreakRange is one run of characters of one class; see
// lineBreakClassRanges.
type lineBreakRange struct {
	lo, hi rune
	class  lbClass
}

// latin1Classes is lineBreakClassRanges for the first 256 code points, which is
// most of the characters most documents have and is worth an array to avoid a
// search per character.
var latin1Classes = func() (t [256]lbClass) {
	for r := range t {
		t[r] = searchLineBreakClass(rune(r))
	}
	return t
}()

// lineBreakClass is a character's Line_Break property, as LineBreak.txt states
// it and before any of it is resolved.
func lineBreakClass(r rune) lbClass {
	if r >= 0 && r < 256 {
		return latin1Classes[r]
	}
	return searchLineBreakClass(r)
}

func searchLineBreakClass(r rune) lbClass {
	t := lineBreakClassRanges[:]
	i := sort.Search(len(t), func(i int) bool { return t[i].hi >= r })
	if i < len(t) && t[i].lo <= r {
		return t[i].class
	}
	return lbXX
}

// lbChar is one character as the rules see it: its class resolved, and the
// other properties some of the rules read.
type lbChar struct {
	r     rune
	class lbClass
	// orig is the class LineBreak.txt gives, before LB1 and CSS resolved it.
	// word-break: keep-all is stated over the original classes, and the
	// dictionary over SA.
	orig lbClass
	// ea is UAX #14's $EastAsian: East_Asian_Width F, W or H. LB19a and LB30
	// read it.
	ea bool
	// pi and pf are General_Category Pi and Pf, which LB15 and LB19 read of a
	// quotation mark.
	pi, pf bool
	// pictCn is an unassigned Extended_Pictographic code point, for LB30b.
	pictCn bool
	// dotted is U+25CC DOTTED CIRCLE, which LB28a counts as a base.
	dotted bool
	// mayBegin and mayEnd are CSS's relaxations: a character "line-break:
	// loose" (or "normal") lets a line begin with, and one it lets a line end
	// after. They switch off the rules that forbid those breaks and nothing
	// else, so a rule earlier in the order — a word joiner, a no-break space,
	// an opening bracket — still holds. See lbTailoring.
	mayBegin, mayEnd bool
}

// lbTailoring is how CSS resolves the classes UAX #14 leaves to the
// implementation, and which of its prohibitions CSS relaxes, for one box's
// values of word-break and line-break.
type lbTailoring struct {
	lb LineBreak
	wb WordBreak
}

// char resolves one character.
//
// LB1 first: AI, SG and XX are AL; SA is a combining mark where it is one and
// AL otherwise; CJ is NS or ID, which is a tailoring and is CSS's to choose.
// Then word-break: break-all, which treats letters, numbers and the complex
// context class as ideographs. Then line-break's relaxations.
func (t lbTailoring) char(r rune) lbChar {
	orig := lineBreakClass(r)
	c := lbChar{r: r, class: orig, orig: orig}
	switch orig {
	case lbAI, lbSG, lbXX:
		c.class = lbAL
	case lbSA:
		if charprop.Is(r, charprop.Mn|charprop.Mc) {
			c.class = lbCM
		} else {
			c.class = lbAL
		}
	case lbCJ:
		// LB1 leaves CJ to the implementation: "resolve CJ to NS" for strict
		// line breaking and to ID for normal. CSS Text's line-break is the
		// tailoring, and this engine resolves it to NS under strict and to ID
		// under every other value. See lineBreakTailoring's note on CJ for the
		// sentence the current draft has changed.
		if t.lb.Strict {
			c.class = lbNS
		} else {
			c.class = lbID
		}
	case lbQU:
		gc := charprop.Of(r)
		c.pi, c.pf = gc&charprop.Pi != 0, gc&charprop.Pf != 0
	}
	// word-break: break-all. "any typographic letter units (and any
	// typographic character units resolving to the NU ("numeric"), AL
	// ("alphabetic"), or SA ("South East Asian") line breaking classes) are
	// instead treated as ID." A mark is not a letter unit of its own: it is
	// part of the unit before it, which LB9 already says.
	if t.wb.BreakAll && c.class != lbCM && c.class != lbZWJ &&
		(c.class == lbNU || c.class == lbAL || orig == lbSA ||
			charprop.Is(r, charprop.L|charprop.N)) {
		c.class = lbID
	}
	c.ea = inRanges(r, eastAsianWideRanges[:])
	c.pictCn = inLineBreakRanges(r, extPictUnassignedRanges[:])
	c.dotted = r == 0x25CC
	c.mayBegin, c.mayEnd = t.relaxed(r, orig)
	return c
}

// relaxed is line-break's list, as CSS Text 3 (§5.2 of the 2026 draft, "Line
// Breaking Strictness") states it. Each entry switches off the UAX #14 rules
// that forbid the break and nothing else; the rule numbers are in decide.
//
//   - normal and loose, where the writing system is Chinese or Japanese: a
//     line may begin with 〜 U+301C or ゠ U+30A0, which are class NS.
//   - loose: a line may begin with an iteration mark (々 〻 ゝ ゞ ヽ ヾ, class
//     NS), and may break between two inseparable characters (class IN).
//   - loose, where the writing system is Chinese or Japanese: a line may begin
//     with the centred punctuation (・ ： ； ･ ‼ ⁇ ⁈ ⁉ ！ ？), with a suffix — class
//     PO whose East Asian Width is A, F or W — and may end after a prefix, class
//     PR with the same widths.
//
// The one rule stated about the character *before* — loose lets a line begin
// with ‐ U+2010 or – U+2013 after an ideograph — is in decide, where the
// character before is known.
func (t lbTailoring) relaxed(r rune, orig lbClass) (mayBegin, mayEnd bool) {
	if r < 0x2000 || t.lb.Strict || t.lb.Anywhere {
		// Every character the list names is at or above U+2000 except the
		// suffixes and prefixes, which it names by width — and the widths it
		// names are A, F and W, which no character below U+2000 of those two
		// classes has except U+00A2, U+00A3, U+00A5, U+00B0 and their
		// neighbours. So those are answered here too.
		if r < 0x2000 && t.lb.Loose && t.lb.ChineseOrJapanese &&
			(orig == lbPO || orig == lbPR) && wideOrAmbiguous(r) {
			return orig == lbPO, orig == lbPR
		}
		return false, false
	}
	cj := t.lb.ChineseOrJapanese
	switch {
	case (t.lb.Normal || t.lb.Loose) && cj && isEastAsianHyphen(r):
		return true, false
	case !t.lb.Loose:
		return false, false
	case isIterationMark(r):
		return true, false
	case cj && isCentredPunctuation(r):
		return true, false
	case cj && (orig == lbPO || orig == lbPR) && wideOrAmbiguous(r):
		return orig == lbPO, orig == lbPR
	}
	return false, false
}

// isIterationMark is §5.3's list: 々 U+3005, 〻 U+303B, ゝ U+309D, ゞ U+309E,
// ヽ U+30FD, ヾ U+30FE.
func isIterationMark(r rune) bool {
	switch r {
	case 0x3005, 0x303B, 0x309D, 0x309E, 0x30FD, 0x30FE:
		return true
	}
	return false
}

// isCentredPunctuation is §5.3's list of "certain centered punctuation marks".
func isCentredPunctuation(r rune) bool {
	switch r {
	case 0x30FB, 0xFF1A, 0xFF1B, 0xFF65, 0x203C, 0x2047, 0x2048, 0x2049, 0xFF01, 0xFF1F:
		return true
	}
	return false
}

// wideOrAmbiguous is East_Asian_Width A, F or W, which is how §5.3 picks the
// suffixes and prefixes loose relaxes: the fullwidth ones and the ambiguous ones
// East Asian text sets wide, and not the halfwidth ones.
func wideOrAmbiguous(r rune) bool {
	if inRanges(r, eastAsianAmbiguousRanges[:]) {
		return true
	}
	return inRanges(r, eastAsianWideRanges[:]) && !inRanges(r, eastAsianHalfwidthRanges[:])
}

// lbBreak is the answer at one boundary.
type lbBreak uint8

const (
	lbProhibited lbBreak = iota
	lbAllowed
	lbMandatory
)

// lbUnit is one unit as LB9 makes them: a base and the marks and joiners after
// it, which the rules see as the base. The flags are the context a rule reads
// of the unit before it, recorded when the unit began because by the time the
// rule is asked that unit is gone.
type lbUnit struct {
	lbChar
	// quPiOpens is LB15a's left half: an initial quotation mark after the
	// start of the text, a break, an opening bracket, another quotation mark,
	// a no-break character, a space or a zero width space. "(sot | BK | CR |
	// LF | NL | OP | QU | GL | SP | ZW) QU_Pi SP* ×".
	quPiOpens bool
	// quAfterNonEA is LB19a's "([^EastAsian] | sot) QU ×": the unit before
	// this quotation mark was not East Asian, or there was none.
	quAfterNonEA bool
	// hyAtStart is LB20a's "(sot | BK | CR | LF | NL | SP | ZW | CB | GL) (HY |
	// HH) × (AL | HL)": a hyphen that begins a word.
	hyAtStart bool
	// hyAfterHL is LB21a's "HL (HY | HH) × [^HL]".
	hyAfterHL bool
	// viAfterAksara is LB28a's "(AK | ◌ | AS) VI × (AK | ◌)".
	viAfterAksara bool
	// afterLetterOrDigit is what the deviation CSS Text's note allows for a
	// hyphen-minus before a digit asks of it: the code point before the hyphen
	// was a letter or a decimal digit. See cssDeviation.
	afterLetterOrDigit bool
}

// BreakContext is what UAX #14's rules need to know of the text before a
// boundary: the unit in front of it, the unit in front of a run of spaces, and
// the two counts that reach further back, a number and a run of regional
// indicators.
//
// It is what one box leaves the next, the way Carried.Prev is: a box boundary
// is not a boundary to the rules, and a box that could not see what came
// before it would answer every rule that looks back as though the paragraph
// began there. The zero value is the start of the text.
type BreakContext struct {
	started bool
	// last is the class of the last character itself, which LB8a asks of a
	// zero width joiner and LB4 to LB6 of a break.
	last lbClass
	// lastRune is that character, for the one CSS rule stated about a
	// character that disappears: a soft hyphen under "hyphens: none".
	lastRune rune
	// unit is the unit in front of the boundary; before is the unit in front
	// of the spaces, where unit is a space.
	unit, before lbUnit
	// num is LB25's "NU (SY | IS)*" and "NU (SY | IS)* (CL | CP)".
	num lbNumber
	// riOdd is LB30a's count: an odd number of regional indicators ends here.
	riOdd bool
	// noBreakAfter says a CSS rule of the last character's own box refuses the
	// break after it: a soft hyphen under "hyphens: none". CSS Text §5.1: for
	// an opportunity made by a character that disappears at the break, the
	// properties of the box containing that character decide.
	noBreakAfter bool
}

type lbNumber uint8

const (
	numNone lbNumber = iota
	// numIn is a number so far: NU (SY | IS)*.
	numIn
	// numClosed is a number closed by a bracket: NU (SY | IS)* (CL | CP).
	numClosed
)

// Started reports whether there is text before the boundary at all.
func (s BreakContext) Started() bool { return s.started }

// AfterObject is the context after an atomic inline: the start of the text.
//
// An atomic inline is not a character, and the rules are run over text. What
// CSS Text says about the boundary on either side of one is its own rule — "for
// Web-compatibility there is a soft wrap opportunity before and after each
// replaced element or other atomic inline, even when adjacent to a character
// that would normally suppress them", with GL, WJ and ZWJ excepted — and layout
// applies it; the rules begin again after it, as they began at the start of
// the paragraph.
//
// Reading it as a character of some class would reach the text after it
// through LB9: a combining mark there would join the picture's unit, and the
// letter after the mark would be broken from it by whatever pair the class
// made. line-breaking-atomic-017 writes exactly that — "<span
// class=inline-block>A</span>&#x034F;B" — and asks for no break anywhere in
// it: the grapheme joiner holds to the picture by §5.1's exception, and the
// letter after it is not broken from the joiner.
func (s BreakContext) AfterObject() BreakContext {
	return BreakContext{}
}

// under is the context as the rules at a box's first character read it: the
// units before the boundary resolved under this box's values of line-break and
// word-break rather than their own.
//
// CSS Text §5 leaves it open — "which elements' line-break, word-break, and
// overflow-wrap properties control the determination of soft wrap
// opportunities at such boundaries is undefined in this level" — and this
// engine's answer has always been the later character's element, because the
// later character is the one whose box is asking. Resolved under their own
// values, the characters of a break-all box would still be ideographs to the
// box after it, and "<span style='word-break: break-all'>bbb</span>ccc" would
// break after the span: word-break-break-all-inline-007 says it does not.
func (s BreakContext) under(t lbTailoring) BreakContext {
	s.unit.lbChar = t.again(s.unit)
	s.before.lbChar = t.again(s.before)
	return s
}

// again is a unit's base resolved under t, as LB10 left it.
func (t lbTailoring) again(u lbUnit) lbChar {
	if u.class == lbNone {
		return u.lbChar
	}
	c := t.char(u.r)
	if c.class == lbCM || c.class == lbZWJ {
		c.class = lbAL
	}
	return c
}

// lbAhead is the text after the character a boundary is before, for the rules
// that look past it: what is left of the box, and then Carried.Ahead. The text
// ends where they do.
type lbAhead struct {
	t          lbTailoring
	rest, more string
}

// peek returns the first n units after the one c begins, resolved; ok is false
// where the text ends first. A unit is a base and the marks after it, so the
// marks after c are stepped over, and the marks after each unit peeked.
func (a lbAhead) peek(c lbChar, n int) (units [2]lbChar, got int) {
	absorbs := absorbsMarks(c.class)
	s := a.rest
	inMore := false
	for got < n {
		if s == "" {
			if !inMore && a.more != "" {
				s, inMore = a.more, true
				continue
			}
			return units, got
		}
		r, size := utf8.DecodeRuneInString(s)
		s = s[size:]
		d := a.t.char(r)
		if absorbs && (d.class == lbCM || d.class == lbZWJ) {
			continue
		}
		if d.class == lbCM || d.class == lbZWJ {
			d.class = lbAL
		}
		units[got] = d
		got++
		absorbs = absorbsMarks(d.class)
	}
	return units, got
}

// absorbsMarks is LB9's condition: the marks after a character of this class
// are part of it, and after any other they are LB10's AL.
func absorbsMarks(c lbClass) bool {
	switch c {
	case lbBK, lbCR, lbLF, lbNL, lbSP, lbZW:
		return false
	}
	return true
}

// Rule numbers, as LineBreakTest.html numbers them: the rule times a hundred,
// so 13.01 is 1301 and LB31's "÷ Any" is 99900.
const (
	ruleSot            = 2
	ruleAfterZW        = 800
	ruleAfterSpace     = 1800
	ruleAny            = 99900
	ruleBeforeIS       = 1540
	ruleBeforeNS       = 2104
	ruleAfterCLSpaceNS = 1600
	ruleHyphenNumber   = 2513
)

// decide is UAX #14 at the boundary between the text s describes and c.
func (s *BreakContext) decide(c lbChar, ahead lbAhead) (lbBreak, int) {
	if !s.started {
		return lbProhibited, ruleSot
	}
	switch s.last {
	case lbBK:
		return lbMandatory, 400
	case lbCR:
		if c.class == lbLF {
			return lbProhibited, 501
		}
		return lbMandatory, 502
	case lbLF:
		return lbMandatory, 503
	case lbNL:
		return lbMandatory, 504
	}
	switch c.class {
	case lbBK, lbCR, lbLF, lbNL:
		return lbProhibited, 600
	case lbSP:
		return lbProhibited, 701
	case lbZW:
		return lbProhibited, 702
	}
	left := s.unit
	if left.class == lbZW || (left.class == lbSP && s.before.class == lbZW) {
		return lbAllowed, ruleAfterZW
	}
	if s.last == lbZWJ {
		return lbProhibited, 810
	}
	if (c.class == lbCM || c.class == lbZWJ) && absorbsMarks(left.class) {
		return lbProhibited, 900
	}
	right := c
	if right.class == lbCM || right.class == lbZWJ {
		right.class = lbAL // LB10
	}
	L, R := left.class, right.class
	spaced := L == lbSP // the boundary is after a run of spaces
	before := s.before.class
	switch {
	case R == lbWJ:
		return lbProhibited, 1101
	case L == lbWJ:
		return lbProhibited, 1102
	case L == lbGL:
		return lbProhibited, 1200
	case R == lbGL && L != lbSP && L != lbBA && L != lbHY && L != lbHH:
		return lbProhibited, 1210
	case R == lbEX && !right.mayBegin:
		return lbProhibited, 1301
	case R == lbCL:
		return lbProhibited, 1302
	case R == lbCP:
		return lbProhibited, 1303
	case R == lbSY:
		return lbProhibited, 1304
	case L == lbOP || (spaced && before == lbOP):
		return lbProhibited, 1400
	case (L == lbQU && left.quPiOpens) || (spaced && before == lbQU && s.before.quPiOpens):
		return lbProhibited, 1511
	}
	if R == lbQU && right.pf {
		u, n := ahead.peek(c, 1)
		if n == 0 {
			return lbProhibited, 1521
		}
		switch u[0].class {
		case lbSP, lbGL, lbWJ, lbCL, lbQU, lbCP, lbEX, lbIS, lbSY, lbBK, lbCR, lbLF,
			lbNL, lbZW:
			return lbProhibited, 1521
		}
	}
	if spaced && R == lbIS {
		if u, n := ahead.peek(c, 1); n == 1 && u[0].class == lbNU {
			return lbAllowed, 1530
		}
	}
	switch {
	case R == lbIS && !right.mayBegin:
		return lbProhibited, ruleBeforeIS
	case R == lbNS && !right.mayBegin && (L == lbCL || L == lbCP ||
		(spaced && (before == lbCL || before == lbCP))):
		return lbProhibited, ruleAfterCLSpaceNS
	case R == lbB2 && (L == lbB2 || (spaced && before == lbB2)):
		return lbProhibited, 1700
	case spaced:
		return lbAllowed, ruleAfterSpace
	case R == lbQU && !right.pi:
		return lbProhibited, 1901
	case L == lbQU && !left.pf:
		return lbProhibited, 1902
	case R == lbQU && !left.ea:
		return lbProhibited, 1910
	}
	if R == lbQU {
		u, n := ahead.peek(c, 1)
		if n == 0 || !u[0].ea {
			return lbProhibited, 1911
		}
	}
	switch {
	case L == lbQU && !right.ea:
		return lbProhibited, 1912
	case L == lbQU && left.quAfterNonEA:
		return lbProhibited, 1913
	case R == lbCB:
		return lbAllowed, 2001
	case L == lbCB:
		return lbAllowed, 2002
	case (L == lbHY || L == lbHH) && left.hyAtStart && (R == lbAL || R == lbHL):
		return lbProhibited, 2010
	case R == lbBA:
		return lbProhibited, 2101
	case R == lbHH && !(ahead.t.lb.Loose && L == lbID):
		// loose lets a line begin with ‐ or – after an ideograph: "if the
		// preceding character belongs to the Unicode line breaking class ID
		// (including when the preceding character is treated as ID due to
		// word-break: break-all)". L is the class after break-all resolved it.
		return lbProhibited, 2102
	case R == lbHY:
		return lbProhibited, 2103
	case R == lbNS && !right.mayBegin:
		return lbProhibited, ruleBeforeNS
	case L == lbBB:
		return lbProhibited, 2105
	case (L == lbHY || L == lbHH) && left.hyAfterHL && R != lbHL:
		return lbProhibited, 2110
	case L == lbSY && R == lbHL:
		return lbProhibited, 2120
	case R == lbIN && !(ahead.t.lb.Loose && L == lbIN):
		return lbProhibited, 2200
	case (L == lbAL || L == lbHL) && R == lbNU:
		return lbProhibited, 2302
	case L == lbNU && (R == lbAL || R == lbHL):
		return lbProhibited, 2303
	case L == lbPR && (R == lbID || R == lbEB || R == lbEM) && !left.mayEnd:
		return lbProhibited, 2312
	case (L == lbID || L == lbEB || L == lbEM) && R == lbPO && !right.mayBegin:
		return lbProhibited, 2313
	case L == lbPR && (R == lbAL || R == lbHL) && !left.mayEnd:
		return lbProhibited, 2402
	case L == lbPO && (R == lbAL || R == lbHL):
		return lbProhibited, 2402
	case (L == lbAL || L == lbHL) && R == lbPR:
		return lbProhibited, 2403
	case (L == lbAL || L == lbHL) && R == lbPO && !right.mayBegin:
		return lbProhibited, 2403
	case s.num == numClosed && R == lbPO && !right.mayBegin:
		return lbProhibited, 2501
	case s.num == numClosed && R == lbPR:
		return lbProhibited, 2503
	case s.num == numIn && R == lbPO && !right.mayBegin:
		return lbProhibited, 2505
	case s.num == numIn && R == lbPR:
		return lbProhibited, 2506
	}
	if (L == lbPO || (L == lbPR && !left.mayEnd)) && R == lbOP {
		u, n := ahead.peek(c, 2)
		if n >= 1 && (u[0].class == lbNU || (n == 2 && u[0].class == lbIS && u[1].class == lbNU)) {
			if L == lbPO {
				return lbProhibited, 2507
			}
			return lbProhibited, 2510
		}
	}
	switch {
	case L == lbPO && R == lbNU:
		return lbProhibited, 2509
	case L == lbPR && R == lbNU && !left.mayEnd:
		return lbProhibited, 2512
	case L == lbHY && R == lbNU:
		return lbProhibited, ruleHyphenNumber
	case L == lbIS && R == lbNU:
		return lbProhibited, 2514
	case s.num == numIn && R == lbNU:
		return lbProhibited, 2515
	case L == lbJL && (R == lbJL || R == lbJV || R == lbH2 || R == lbH3):
		return lbProhibited, 2601
	case (L == lbJV || L == lbH2) && (R == lbJV || R == lbJT):
		return lbProhibited, 2602
	case (L == lbJT || L == lbH3) && R == lbJT:
		return lbProhibited, 2603
	case isHangul(L) && R == lbPO && !right.mayBegin:
		return lbProhibited, 2701
	case L == lbPR && isHangul(R) && !left.mayEnd:
		return lbProhibited, 2702
	case (L == lbAL || L == lbHL) && (R == lbAL || R == lbHL):
		return lbProhibited, 2800
	case L == lbAP && isAksaraBase(right):
		return lbProhibited, 2811
	case isAksaraBase(left.lbChar) && (R == lbVF || R == lbVI):
		return lbProhibited, 2812
	case L == lbVI && left.viAfterAksara && (R == lbAK || right.dotted):
		return lbProhibited, 2813
	}
	if isAksaraBase(left.lbChar) && isAksaraBase(right) {
		if u, n := ahead.peek(c, 1); n == 1 && u[0].class == lbVF {
			return lbProhibited, 2814
		}
	}
	switch {
	case L == lbIS && (R == lbAL || R == lbHL):
		return lbProhibited, 2900
	case (L == lbAL || L == lbHL || L == lbNU) && R == lbOP && !right.ea:
		return lbProhibited, 3001
	case L == lbCP && !left.ea && (R == lbAL || R == lbHL || R == lbNU):
		return lbProhibited, 3002
	case L == lbRI && R == lbRI && s.riOdd:
		return lbProhibited, 3011
	case L == lbRI && R == lbRI:
		return lbAllowed, 3013
	case L == lbEB && R == lbEM:
		return lbProhibited, 3021
	case left.pictCn && R == lbEM:
		return lbProhibited, 3022
	}
	return lbAllowed, ruleAny
}

func isHangul(c lbClass) bool {
	switch c {
	case lbJL, lbJV, lbJT, lbH2, lbH3:
		return true
	}
	return false
}

// isAksaraBase is LB28a's "(AK | ◌ | AS)".
func isAksaraBase(c lbChar) bool {
	return c.class == lbAK || c.class == lbAS || (c.class == lbAL && c.dotted)
}

// advance moves the context past c. hy is the hyphens value of c's own box,
// for the soft hyphen; see BreakContext.noBreakAfter.
func (s *BreakContext) advance(c lbChar, hy Hyphens) {
	prevRune := s.lastRune
	s.lastRune = c.r
	s.noBreakAfter = c.r == 0x00AD && !hy.Soft()
	if s.started && (c.class == lbCM || c.class == lbZWJ) && absorbsMarks(s.unit.class) {
		// LB9: part of the unit before, which the rules go on seeing.
		s.last = c.class
		return
	}
	u := lbUnit{lbChar: c}
	if u.class == lbCM || u.class == lbZWJ {
		u.class = lbAL // LB10
	}
	prev, sot := s.unit.class, !s.started
	switch u.class {
	case lbQU:
		u.quPiOpens = u.pi && (sot || prev == lbBK || prev == lbCR || prev == lbLF ||
			prev == lbNL || prev == lbOP || prev == lbQU || prev == lbGL ||
			prev == lbSP || prev == lbZW)
		u.quAfterNonEA = sot || !s.unit.ea
	case lbHY, lbHH:
		u.hyAtStart = sot || prev == lbBK || prev == lbCR || prev == lbLF || prev == lbNL ||
			prev == lbSP || prev == lbZW || prev == lbCB || prev == lbGL
		u.hyAfterHL = prev == lbHL
		u.afterLetterOrDigit = s.started &&
			charprop.Is(prevRune, charprop.L|charprop.Nd)
	case lbVI:
		u.viAfterAksara = !sot && isAksaraBase(s.unit.lbChar)
	}
	switch {
	case u.class == lbNU:
		s.num = numIn
	case (u.class == lbSY || u.class == lbIS) && s.num == numIn:
	case (u.class == lbCL || u.class == lbCP) && s.num == numIn:
		s.num = numClosed
	default:
		s.num = numNone
	}
	s.riOdd = u.class == lbRI && !(prev == lbRI && s.riOdd)
	if u.class == lbSP && prev != lbSP {
		s.before = s.unit
	}
	if !sot && u.class != lbSP {
		s.before = lbUnit{}
	}
	s.unit = u
	s.last = c.class
	s.started = true
}

// contextOf is the context a caller gets that names only the character before
// the boundary, and the base it belongs to where that character is a mark.
func contextOf(t lbTailoring, base, last rune, hy Hyphens) BreakContext {
	var s BreakContext
	if base != 0 && base != last {
		s.advance(t.char(base), hy)
	}
	s.advance(t.char(last), hy)
	return s
}

// keepAllUnit is what word-break: keep-all suppresses an opportunity between:
// "typographic letter units (or other typographic character units belonging
// to the NU, AL, AI, or ID Unicode line breaking classes)". A letter unit is a
// letter or a number, CSS Text §1.4's definition.
func keepAllUnit(c lbChar) bool {
	switch c.orig {
	case lbNU, lbAL, lbAI, lbID:
		return true
	case lbNone:
		return false
	}
	return charprop.Is(c.r, charprop.L|charprop.N)
}

// cssRefuses is the part of CSS Text 3's note to line-break that withdraws an
// opportunity UAX #14 offers. The note lists "deviations [that] could be
// desirable for maximum interoperability with existing implementations", and
// three of them are this one shape:
//
//	Not introducing a line break opportunity between U+0021 (Exclamation
//	Mark, !) and a letter (Unicode general category L). This prevents a
//	break in the string "!important".
//
// and the same for U+002F SOLIDUS, which keeps "23/Jan/2024" whole, and U+007C
// VERTICAL LINE. The character before is the unit's base, so a mark after the
// exclamation mark does not hide it.
//
// A letter word-break: break-all has made an ideograph is not one for this:
// the value says letters are "treated as ID" for line breaking, and
// word-break-break-all-028 asks for "XXX/X" to break after the slash.
func cssRefuses(s BreakContext, c lbChar) bool {
	switch s.unit.r {
	case '!', '/', '|':
		return s.started && s.unit.class != lbSP && c.class != lbID &&
			charprop.Is(c.r, charprop.L)
	}
	return false
}

// cssAllowsHyphenNumber is the note's fourth deviation, the one that allows
// what UAX #14 refuses. LB25 keeps a hyphen with the digit after it — "HY ×
// NU" — which is right for a minus sign and wrong for a hyphen inside a
// catalogue number, so the note keeps the rule only "if the codepoint prior to
// the hyphen was _not_ a letter or digit (Unicode general category L or Nd).
// This prevents breaking after the minus sign before a number such as in
// "-13" whilst allowing breaks after the hyphen in "ABCD-1234" and
// "1234-5678"". It is U+002D alone that it names.
func cssAllowsHyphenNumber(s BreakContext) bool {
	return s.unit.r == '-' && s.unit.afterLetterOrDigit
}

// NeedsLookahead reports whether the rules at a text's own last characters
// read past its end, so that the caller has to find the text that follows and
// pass it as Carried.Ahead.
//
// Four rules look ahead, and every one is asked in front of a character of a
// few classes: a quotation mark (LB15b, LB19a), an infix separator after a
// space (LB15c), an opening bracket after a prefix or a postfix (LB25, which
// reads two units on), and an aksara base (LB28a). So the answer is yes where
// one of the last two units begins with one of them, and no otherwise — which
// is almost always, and lets the caller skip the walk.
func NeedsLookahead(text string) bool {
	seen := 0
	for i := len(text); i > 0 && seen < 2; {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		i -= size
		switch lineBreakClass(r) {
		case lbCM, lbZWJ:
			continue
		case lbQU, lbIS, lbOP, lbAK, lbAS:
			return true
		}
		if r == 0x25CC {
			return true
		}
		seen++
	}
	return false
}

// breaksLikeAnIdeographClass is BreaksLikeAnIdeograph asked of a resolved
// character: an ideograph, a Hangul syllable or jamo, or a small kana — and
// anything word-break: break-all has made an ideograph.
func breaksLikeAnIdeographClass(c lbChar) bool {
	switch c.class {
	case lbID, lbH2, lbH3, lbJL, lbJV, lbJT:
		return true
	}
	return c.orig == lbCJ
}

// saLetter is a letter of one of the scripts UAX #14 leaves to a dictionary:
// class SA and not one of its marks, which LB1 makes combining marks and which
// are part of the letter before them rather than letters of their own.
func saLetter(c lbChar) bool {
	return c.orig == lbSA && !charprop.Is(c.r, charprop.Mn|charprop.Mc)
}

// ContinuesUnit reports whether a character is part of the unit before it as
// UAX #14's LB9 makes them — a combining mark or a zero width joiner — and so
// does not begin one of its own. A caller reading ahead for NeedsLookahead
// counts the characters that do.
func ContinuesUnit(r rune) bool {
	switch lineBreakClass(r) {
	case lbCM, lbZWJ:
		return true
	case lbSA:
		return charprop.Is(r, charprop.Mn|charprop.Mc)
	}
	return false
}
