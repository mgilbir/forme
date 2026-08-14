package bidi

import (
	"sort"
	"unicode/utf8"
)

// Direction: the Unicode Bidirectional Algorithm, UAX #9.
//
// A PDF text-showing operator advances the pen left to right and offers no way
// to say otherwise. Hebrew and Arabic are written right to left. So somewhere
// between the string a caller hands over and the bytes in the content stream,
// the order has to be turned around — and "turned around" is not reversing the
// string, because a line of Arabic containing a phone number, a Latin brand name
// and a bracketed aside has stretches running each way, nested, and the nesting
// is decided by the characters themselves.
//
// That decision is what this file implements. It is Unicode's algorithm, not an
// approximation of it: the algorithm is what every other text system uses, it is
// exhaustively specified, and Unicode publishes a conformance suite of some six
// hundred thousand cases for it (bidi_conformance_test.go runs both files). An
// approximation would disagree with every reader the document is opened in.
//
// # Where this sits in the pipeline
//
// Shaping happens in logical order and reordering happens after it. That order
// is not interchangeable. Arabic joining asks what a letter's *neighbours in the
// text* are, and after reversal they are the wrong ones — a shaper that reverses
// first joins each letter to the wrong side and then draws the result backwards,
// which looks very nearly right and is wrong twice.
//
// # What is here and what is not
//
// P2-P3 (paragraph level), X1-X10 (explicit embeddings, overrides and isolates),
// W1-W7 (weak types), N0 (paired brackets), N1-N2 (neutrals), I1-I2 (implicit
// levels), L1-L2 (reordering) and L4 (mirroring) are implemented.
//
// P1 is not: splitting text into paragraphs on a paragraph separator is the
// caller's, because a PDF text object is a line and a caller that wanted two
// paragraphs would be drawing two of them. A paragraph separator inside the
// string is handled where it falls (X8, L1) rather than cutting the string.
//
// L3 is not: it says a combining mark may be reordered relative to its base in a
// right-to-left run, and what it should do is font- and platform-specific.
// Unicode excludes it from the conformance data for that reason.

// Class is a character's Bidi_Class: what the algorithm knows about it.
//
// The constants are in the order cmd/genbidi writes them, and L is zero
// because it is the default — a character no range in bidiclass.go names is
// left-to-right, and there are far more of those than of everything else.
type Class uint8

const (
	L   Class = iota // left-to-right
	R                // right-to-left
	AL               // right-to-left Arabic
	EN               // European number
	ES               // European number separator
	ET               // European number terminator
	AN               // Arabic number
	CS               // common number separator
	NSM              // non-spacing mark
	BN               // boundary neutral
	B                // paragraph separator
	S                // segment separator
	WS               // whitespace
	ON               // other neutral
	LRE              // left-to-right embedding
	RLE              // right-to-left embedding
	LRO              // left-to-right override
	RLO              // right-to-left override
	PDF              // pop directional format
	LRI              // left-to-right isolate
	RLI              // right-to-left isolate
	FSI              // first strong isolate
	PDI              // pop directional isolate
)

// MaxDepth is the deepest embedding the algorithm recognises (UAX #9, BD2).
// It is a cap in the specification rather than one this package chose: an
// embedding beyond it is an overflow and is ignored, which is what stops a
// string of a million RLEs from costing a million stack frames.
const MaxDepth = 125

// maxBracketPairs is the bracket stack's depth (UAX #9, BD16). Also the
// specification's: past it, rule N0 stops looking for pairs in that sequence
// rather than growing without bound.
const maxBracketPairs = 63

// ClassOf reports a character's Bidi_Class. A character the table does not
// name is left-to-right, which is the property's default value and most of the
// code space.
func ClassOf(r rune) Class {
	i := sort.Search(len(classRanges), func(i int) bool { return classRanges[i].hi >= r })
	if i < len(classRanges) && r >= classRanges[i].lo {
		return classRanges[i].class
	}
	return L
}

// MirrorOf reports the character drawn in place of this one in a
// right-to-left run: rule L4. A parenthesis that opens a left-to-right aside
// closes a right-to-left one, and drawing the glyph as written points it the
// wrong way.
func MirrorOf(r rune) (rune, bool) {
	i := sort.Search(len(mirrors), func(i int) bool { return mirrors[i].from >= r })
	if i < len(mirrors) && mirrors[i].from == r {
		return mirrors[i].to, true
	}
	return r, false
}

// BracketOf reports whether a character is a paired bracket, which one it
// pairs with, and whether this is the opening half.
func BracketOf(r rune) (paired rune, open, ok bool) {
	i := sort.Search(len(brackets), func(i int) bool { return brackets[i].ch >= r })
	if i < len(brackets) && brackets[i].ch == r {
		return brackets[i].paired, brackets[i].open, true
	}
	return r, false, false
}

// Canonical folds the two angle brackets that Unicode deprecated onto the
// ones that replaced them.
//
// Rule N0 matches an opening bracket to a closing one by identity, and U+3008
// and U+2329 are canonically equivalent characters — the same bracket written
// twice in the standard. A document that opens with one and closes with the
// other is a matched pair to every reader, and matching by code point alone
// would miss it.
func Canonical(r rune) rune {
	switch r {
	case 0x3008:
		return 0x2329
	case 0x3009:
		return 0x232A
	}
	return r
}

// isRemoved reports whether rule X9 removes a character from the sequence
// the later rules see: the explicit embedding and override codes, their pop, and
// the boundary neutrals.
//
// They are removed rather than resolved because they have no width and no
// direction of their own — they are instructions, and X1-X8 has already carried
// them out. What is done with them afterwards is the caller's: this package
// keeps them in the glyph buffer, because a zero-width joiner is a boundary
// neutral and Arabic joining needs to see it.
func isRemoved(c Class) bool {
	switch c {
	case RLE, LRE, RLO, LRO, PDF, BN:
		return true
	}
	return false
}

// isIsolateInitiator reports whether a character opens an isolate.
func isIsolateInitiator(c Class) bool {
	return c == LRI || c == RLI || c == FSI
}

// isNeutral reports whether a character is a "neutral or isolate formatting
// character" — NI in the wording of rules N0 to N2.
func isNeutral(c Class) bool {
	switch c {
	case B, S, WS, ON, FSI, LRI, RLI, PDI:
		return true
	}
	return false
}

// strong reduces a resolved type to the direction it contributes when the
// neutral rules look either side of a gap: numbers count as right-to-left,
// because they are written left to right *within* right-to-left text and the
// text around them is what N1 is deciding about. It returns ON for a type
// that contributes nothing.
func strong(c Class) Class {
	switch c {
	case L:
		return L
	case R, EN, AN:
		return R
	}
	return ON
}

// dirOf is the direction a level runs in: even is left-to-right.
func dirOf(level int) Class {
	if level&1 == 1 {
		return R
	}
	return L
}

// Paragraph is a resolved paragraph: an embedding level for every character
// and the paragraph's own level.
type Paragraph struct {
	// classes is each character's original Bidi_Class, which rules L1 and N0
	// both need after the earlier rules have overwritten the working copy.
	classes []Class

	// levels is the resolved embedding level of each character. A character
	// rule X9 removed has no level of its own and carries -1; callers that must
	// place it anyway take the level of what precedes it.
	levels []int

	// para is the paragraph embedding level, from P2/P3 or from the caller.
	para int
}

// resolveClasses runs the algorithm over one paragraph.
//
// text may be nil, and then rule N0 is skipped: the paired-bracket rule is the
// one place the algorithm looks at characters rather than at classes, and
// Unicode's own BidiTest.txt supplies classes with no characters behind them.
// A caller with real text always passes it.
//
// paraLevel is 0 or 1 to impose a direction, or negative to derive one with
// P2/P3 — which is what a caller who does not know the language wants: the
// paragraph runs whichever way its first strongly-directional character does.
func resolveClasses(classes []Class, text []rune, paraLevel int) Paragraph {
	n := len(classes)
	p := Paragraph{classes: classes, levels: make([]int, n)}

	pdi, initiator := matchPDI(classes)
	if paraLevel < 0 {
		paraLevel = paraLevelOf(classes, pdi, 0, n)
	}
	p.para = paraLevel

	// types is the working copy every rule from X6 on rewrites. The originals
	// stay in p.classes: L1 restores whitespace by what it *was*, and N0 asks
	// what a character was before W1 touched it.
	types := make([]Class, n)
	copy(types, classes)

	explicit(classes, types, p.levels, paraLevel, pdi)

	// X9: the explicit codes have done their work and leave the sequence.
	retained := make([]int, 0, n)
	for i := 0; i < n; i++ {
		if !isRemoved(classes[i]) {
			retained = append(retained, i)
		}
	}

	for _, seq := range sequences(retained, p.levels, classes, pdi, initiator, paraLevel) {
		seq.resolveWeak(types)
		seq.resolveBrackets(types, classes, text)
		seq.resolveNeutral(types)
		seq.resolveImplicit(types, p.levels)
	}

	for i := 0; i < n; i++ {
		if isRemoved(classes[i]) {
			p.levels[i] = -1
		}
	}
	p.resolveL1(retained)
	return p
}

// matchPDI pairs each isolate initiator with the PDI that ends it (BD9).
//
// An initiator with no PDI is matched to one past the end, which is what the
// rule says: the isolate runs to the end of the paragraph. A PDI with no
// initiator matches -1 and is an ordinary neutral.
func matchPDI(classes []Class) (pdi, initiator []int) {
	n := len(classes)
	pdi = make([]int, n)
	initiator = make([]int, n)
	for i := 0; i < n; i++ {
		pdi[i] = n
		initiator[i] = -1
	}
	var stack []int
	for i := 0; i < n; i++ {
		switch classes[i] {
		case LRI, RLI, FSI:
			stack = append(stack, i)
		case PDI:
			if len(stack) > 0 {
				j := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				pdi[j] = i
				initiator[i] = j
			}
		}
	}
	return pdi, initiator
}

// paraLevelOf is P2 and P3, and doubles as X5c: the level implied by the first
// strongly-directional character in a range, skipping anything inside an
// isolate. Nothing strong means left-to-right.
func paraLevelOf(classes []Class, pdi []int, start, end int) int {
	for i := start; i < end; i++ {
		switch classes[i] {
		case L:
			return 0
		case R, AL:
			return 1
		case LRI, RLI, FSI:
			// The contents of an isolate say nothing about the text around it —
			// that is what isolating means — so skip to its end.
			i = pdi[i]
		}
	}
	return 0
}

// explicit is X1 to X8: it carries out the embedding, override and isolate
// codes, giving every character a level and rewriting the ones an override
// covers.
//
// The three counters are the whole of the overflow handling, and they are why
// this cannot be a plain stack. Past 125 levels, or once anything has
// overflowed, further codes have to be counted rather than pushed so that their
// pops can be matched to them — otherwise a pop that belongs to an ignored
// embedding would pop a real one, and the rest of the paragraph would come out
// at the wrong level.
func explicit(classes, types []Class, levels []int, paraLevel int, pdi []int) {
	type status struct {
		level    int
		override Class // ON where the text is not overridden
		isolate  bool
	}
	stack := make([]status, 1, MaxDepth+2)
	stack[0] = status{level: paraLevel, override: ON}
	top := func() status { return stack[len(stack)-1] }

	overflowIsolate, overflowEmbedding, validIsolate := 0, 0, 0

	// nextLevel is the least level above the current one that runs the given
	// way: odd for right-to-left, even for left-to-right.
	nextLevel := func(rtl bool) int {
		if rtl {
			return (top().level + 1) | 1
		}
		return (top().level + 2) &^ 1
	}

	for i := range classes {
		switch c := classes[i]; c {
		case RLE, LRE, RLO, LRO:
			// X2-X5. The code itself takes the level in force before it, and is
			// then removed by X9 — the level only matters to a caller that keeps
			// the character in a buffer.
			levels[i] = top().level
			rtl := c == RLE || c == RLO
			level := nextLevel(rtl)
			if level <= MaxDepth && overflowIsolate == 0 && overflowEmbedding == 0 {
				override := ON
				switch c {
				case RLO:
					override = R
				case LRO:
					override = L
				}
				stack = append(stack, status{level: level, override: override})
			} else if overflowIsolate == 0 {
				overflowEmbedding++
			}

		case RLI, LRI, FSI:
			// X5a-X5c. An isolate initiator is a character in its own right: it
			// keeps the level outside the isolate and takes any override in
			// force, and only then does the new level begin.
			rtl := c == RLI
			if c == FSI {
				rtl = paraLevelOf(classes, pdi, i+1, min(pdi[i], len(classes))) == 1
			}
			levels[i] = top().level
			if o := top().override; o != ON {
				types[i] = o
			}
			level := nextLevel(rtl)
			if level <= MaxDepth && overflowIsolate == 0 && overflowEmbedding == 0 {
				validIsolate++
				stack = append(stack, status{level: level, override: ON, isolate: true})
			} else {
				overflowIsolate++
			}

		case PDI:
			// X6a. A PDI ends the innermost isolate, and with it any embeddings
			// opened inside that isolate and never popped — which is what makes
			// an isolate isolating rather than merely nested.
			if overflowIsolate > 0 {
				overflowIsolate--
			} else if validIsolate > 0 {
				overflowEmbedding = 0
				for !top().isolate {
					stack = stack[:len(stack)-1]
				}
				stack = stack[:len(stack)-1]
				validIsolate--
			}
			levels[i] = top().level
			if o := top().override; o != ON {
				types[i] = o
			}

		case PDF:
			// X7. The level is the one still in force: a pop takes effect after
			// the character that asks for it.
			levels[i] = top().level
			switch {
			case overflowIsolate > 0:
				// An embedding inside an overflowed isolate was never pushed.
			case overflowEmbedding > 0:
				overflowEmbedding--
			case !top().isolate && len(stack) >= 2:
				stack = stack[:len(stack)-1]
			}

		case B:
			// X8. A paragraph separator belongs to no embedding: it takes the
			// paragraph's own level whatever is open around it.
			levels[i] = paraLevel

		case BN:
			levels[i] = top().level

		default:
			// X6.
			levels[i] = top().level
			if o := top().override; o != ON {
				types[i] = o
			}
		}
	}
}

// sequence is an isolating run sequence (BD13): the level runs that an
// isolate initiator and its matching PDI join into one, together with the
// directions assumed to lie either side of it.
//
// It exists because the weak and neutral rules have to see an isolate's
// surroundings as continuous. "abc <RLI> عربي <PDI> def" is one stretch of
// left-to-right text with a hole in it, and a rule that stopped at the RLI would
// resolve what follows the PDI against nothing.
type sequence struct {
	// pos are the positions in the original text, in logical order, that this
	// sequence covers. Characters X9 removed are not among them.
	pos []int

	// level is the embedding level every run in the sequence shares.
	level int

	// sos and eos are the directions to assume before and after the sequence:
	// L or R. They are what the rules use where they would otherwise
	// look past the end.
	sos, eos Class
}

// sequences builds the isolating run sequences of a paragraph, in the order
// their first characters appear.
func sequences(retained []int, levels []int, classes []Class, pdi, initiator []int, paraLevel int) []sequence {
	if len(retained) == 0 {
		return nil
	}

	// The level runs: maximal stretches of the retained characters that share a
	// level.
	type run struct{ from, to int } // indices into retained, half-open
	var runs []run
	for i := 0; i < len(retained); {
		j := i + 1
		for j < len(retained) && levels[retained[j]] == levels[retained[i]] {
			j++
		}
		runs = append(runs, run{i, j})
		i = j
	}
	// Which run each position starts, so that a PDI can be found from the
	// initiator that matches it.
	startsRun := make(map[int]int, len(runs))
	for k, r := range runs {
		startsRun[retained[r.from]] = k
	}

	used := make([]bool, len(runs))
	var out []sequence
	for k, r := range runs {
		if used[k] {
			continue
		}
		// A run beginning with a PDI that matches an initiator is the tail of
		// some earlier sequence, and is reached from there.
		if first := retained[r.from]; classes[first] == PDI && initiator[first] >= 0 {
			continue
		}
		seq := sequence{level: levels[retained[r.from]]}
		for cur := k; ; {
			used[cur] = true
			for i := runs[cur].from; i < runs[cur].to; i++ {
				seq.pos = append(seq.pos, retained[i])
			}
			// A run that ends in an isolate initiator continues at the PDI that
			// closes it, and the text between the two — which is the isolate —
			// is a sequence of its own.
			last := seq.pos[len(seq.pos)-1]
			if !isIsolateInitiator(classes[last]) {
				break
			}
			next, ok := startsRun[pdi[last]]
			if !ok || used[next] {
				break
			}
			cur = next
		}
		out = append(out, seq)
	}

	// sos and eos. Either is the stronger of the sequence's own level and the
	// level of the text it adjoins — which is the paragraph's where there is no
	// adjoining text, and also where the sequence ends in an isolate that was
	// never closed, because what follows such an isolate is outside it.
	position := make(map[int]int, len(retained))
	for i, p := range retained {
		position[p] = i
	}
	for si := range out {
		seq := &out[si]
		first, last := seq.pos[0], seq.pos[len(seq.pos)-1]

		before := paraLevel
		if i := position[first]; i > 0 {
			before = levels[retained[i-1]]
		}
		seq.sos = dirOf(max(seq.level, before))

		// A sequence ending in an isolate initiator that nothing closes runs to
		// the end of the paragraph, so what follows it in the text is outside it
		// and the paragraph's own level is what it adjoins.
		after := paraLevel
		unclosed := isIsolateInitiator(classes[last]) && pdi[last] >= len(classes)
		if i := position[last]; !unclosed && i+1 < len(retained) {
			after = levels[retained[i+1]]
		}
		seq.eos = dirOf(max(seq.level, after))
	}
	return out
}

// resolveWeak is rules W1 to W7: the types that take their direction from what
// surrounds them rather than carrying one.
func (s sequence) resolveWeak(types []Class) {
	// W1: a non-spacing mark takes the type of what it is written on. After an
	// isolate initiator or a PDI it has nothing to take, and becomes neutral —
	// a mark cannot inherit through a boundary that exists to stop inheritance.
	prev := s.sos
	for _, p := range s.pos {
		if types[p] == NSM {
			types[p] = prev
			continue
		}
		switch types[p] {
		case LRI, RLI, FSI, PDI:
			prev = ON
		default:
			prev = types[p]
		}
	}

	// W2: a European number written after Arabic script is an Arabic number.
	// The digits are the same characters; which they are depends on the letters
	// before them.
	strong := s.sos
	for _, p := range s.pos {
		switch types[p] {
		case L, R, AL:
			strong = types[p]
		case EN:
			if strong == AL {
				types[p] = AN
			}
		}
	}

	// W3: Arabic letters have served their purpose above and are now simply
	// right-to-left.
	for _, p := range s.pos {
		if types[p] == AL {
			types[p] = R
		}
	}

	// W4: a single separator between two numbers of the same kind belongs to the
	// number — the decimal point in "1.23", the comma in "1,234".
	for i := 1; i+1 < len(s.pos); i++ {
		prev, cur, next := types[s.pos[i-1]], types[s.pos[i]], types[s.pos[i+1]]
		switch cur {
		case ES:
			if prev == EN && next == EN {
				types[s.pos[i]] = EN
			}
		case CS:
			if prev == next && (prev == EN || prev == AN) {
				types[s.pos[i]] = prev
			}
		}
	}

	// W5: a run of terminators next to a European number joins it, which is what
	// makes "$42" and "42%" one thing rather than three.
	for i := 0; i < len(s.pos); {
		if types[s.pos[i]] != ET {
			i++
			continue
		}
		j := i
		for j < len(s.pos) && types[s.pos[j]] == ET {
			j++
		}
		before := s.sos
		if i > 0 {
			before = types[s.pos[i-1]]
		}
		after := s.eos
		if j < len(s.pos) {
			after = types[s.pos[j]]
		}
		if before == EN || after == EN {
			for k := i; k < j; k++ {
				types[s.pos[k]] = EN
			}
		}
		i = j
	}

	// W6: whatever separators and terminators are left were not part of a
	// number, so they are ordinary neutrals.
	for _, p := range s.pos {
		switch types[p] {
		case ET, ES, CS:
			types[p] = ON
		}
	}

	// W7: a European number in left-to-right text is simply left-to-right, and
	// stops being a thing the neutral rules have to reason about.
	strong = s.sos
	for _, p := range s.pos {
		switch types[p] {
		case L, R:
			strong = types[p]
		case EN:
			if strong == L {
				types[p] = L
			}
		}
	}
}

// resolveBrackets is rule N0: a pair of brackets resolves as a unit, so that
// both halves point the same way.
//
// Without it "(‏עברית‏)" comes out with one parenthesis mirrored and the other
// not — each resolved against its own neighbour, which is a different neighbour
// for each. The rule is the one place in the algorithm that looks at characters
// rather than at classes, and it is skipped when there are none.
func (s sequence) resolveBrackets(types, original []Class, text []rune) {
	if text == nil {
		return
	}
	pairs := s.bracketPairs(types, text)
	if len(pairs) == 0 {
		return
	}
	e := dirOf(s.level)
	o := L
	if e == L {
		o = R
	}

	for _, pair := range pairs {
		// What direction, if any, the text inside the brackets carries. The
		// embedding direction wins outright wherever it appears; the opposite
		// only counts if the embedding direction is absent entirely.
		found := ON
		for i := pair.open + 1; i < pair.close; i++ {
			d := strong(types[s.pos[i]])
			if d == ON {
				continue
			}
			if d == e {
				found = e
				break
			}
			found = o
		}
		if found == ON {
			continue // nothing strong inside: the brackets stay neutral
		}
		dir := found
		if found == o {
			// Text opposite the embedding direction takes the brackets with it
			// only if the text *before* the brackets ran that way too.
			// Otherwise the brackets belong to the surrounding direction and
			// the opposite-direction phrase sits inside them.
			prior := s.sos
			for i := pair.open - 1; i >= 0; i-- {
				if d := strong(types[s.pos[i]]); d != ON {
					prior = d
					break
				}
			}
			if prior != o {
				dir = e
			}
		}
		types[s.pos[pair.open]] = dir
		types[s.pos[pair.close]] = dir

		// A mark written on a bracket goes with it. The test is on what the
		// character was before W1 rewrote it, because W1 has by now given every
		// mark the type of its neighbour and there is no mark left to find.
		for _, from := range [2]int{pair.open, pair.close} {
			for i := from + 1; i < len(s.pos); i++ {
				if original[s.pos[i]] != NSM {
					break
				}
				types[s.pos[i]] = dir
			}
		}
	}
}

// bracketPair is one matched pair, as offsets into a sequence's positions.
type bracketPair struct{ open, close int }

// bracketPairs is BD16: the matched bracket pairs of a sequence, sorted by
// opening position.
//
// Only characters still neutral after the weak rules can be brackets — a
// parenthesis that a stronger rule has already settled is not up for
// reconsideration — and the stack is bounded at 63, past which the rule stops
// looking. Both are the specification's, and the bound is what keeps a string of
// nothing but opening brackets from costing memory in proportion to its length.
func (s sequence) bracketPairs(types []Class, text []rune) []bracketPair {
	type opening struct {
		closer rune
		at     int
	}
	var (
		stack []opening
		pairs []bracketPair
	)
	for i, p := range s.pos {
		if types[p] != ON || p >= len(text) {
			continue
		}
		paired, open, ok := BracketOf(text[p])
		if !ok {
			continue
		}
		if open {
			if len(stack) >= maxBracketPairs {
				// BD16 stops processing this sequence entirely rather than
				// dropping one bracket, so that the pairs it did find are not a
				// biased sample of the ones it did not.
				break
			}
			stack = append(stack, opening{closer: Canonical(paired), at: i})
			continue
		}
		for k := len(stack) - 1; k >= 0; k-- {
			if stack[k].closer != Canonical(text[p]) {
				continue
			}
			pairs = append(pairs, bracketPair{open: stack[k].at, close: i})
			stack = stack[:k]
			break
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].open < pairs[j].open })
	return pairs
}

// resolveNeutral is rules N1 and N2: what a run of neutral characters — spaces,
// punctuation, an unresolved bracket — does between text on either side.
func (s sequence) resolveNeutral(types []Class) {
	e := dirOf(s.level)
	for i := 0; i < len(s.pos); {
		if !isNeutral(types[s.pos[i]]) {
			i++
			continue
		}
		j := i
		for j < len(s.pos) && isNeutral(types[s.pos[j]]) {
			j++
		}
		before := s.sos
		if i > 0 {
			before = strong(types[s.pos[i-1]])
		}
		after := s.eos
		if j < len(s.pos) {
			after = strong(types[s.pos[j]])
		}
		// N1 where both sides agree; N2 — the embedding direction — where they
		// do not, which is the case a space between a Latin and a Hebrew word
		// falls into.
		dir := e
		if before == after && (before == L || before == R) {
			dir = before
		}
		for k := i; k < j; k++ {
			types[s.pos[k]] = dir
		}
		i = j
	}
}

// resolveImplicit is rules I1 and I2: text running against its embedding
// direction is pushed one level deeper, and numbers inside left-to-right text
// two — a number is written left to right whichever way the text around it runs,
// so inside right-to-left text it needs its own level, and inside left-to-right
// text it needs to be a level above right-to-left text that may surround it.
func (s sequence) resolveImplicit(types []Class, levels []int) {
	for _, p := range s.pos {
		level := s.level
		if level&1 == 0 {
			switch types[p] {
			case R:
				level++
			case AN, EN:
				level += 2
			}
		} else {
			switch types[p] {
			case L, EN, AN:
				level++
			}
		}
		levels[p] = level
	}
}

// resolveL1 is rule L1: separators and the whitespace that trails them go back
// to the paragraph's own direction.
//
// It is what puts the space at the end of a right-to-left line on the left,
// where the next line begins, rather than stranding it in the middle of the
// page. The types it tests are the original ones, because by now every space has
// been given the direction of its neighbours and there is no whitespace left to
// find.
func (p *Paragraph) resolveL1(retained []int) {
	trailing := true
	for i := len(retained) - 1; i >= 0; i-- {
		pos := retained[i]
		switch p.classes[pos] {
		case B, S:
			p.levels[pos] = p.para
			trailing = true
		case WS, LRI, RLI, FSI, PDI:
			if trailing {
				p.levels[pos] = p.para
			}
		default:
			trailing = false
		}
	}
}

// reorder is rule L2: the visual order of a resolved paragraph, as a list of
// positions in the original text from left to right.
//
// Characters rule X9 removed are not in it. They have no width and no place on
// the line; a caller that draws them anyway draws them where their neighbours
// are.
func (p *Paragraph) reorder() []int {
	var (
		order  []int
		levels []int
	)
	for i, l := range p.levels {
		if l < 0 {
			continue
		}
		order = append(order, i)
		levels = append(levels, l)
	}
	reorderLevels(order, levels)
	return order
}

// reorderLevels applies L2 in place: from the deepest level down to the
// shallowest odd one, reverse every contiguous stretch at that level or above.
//
// The levels are indexed by position on the line and stay put; it is the order
// that is permuted. Reversing repeatedly rather than sorting is not an
// inefficiency — a level of 2 inside a level of 1 has to be reversed twice, once
// on its own account and once with the run that contains it, and that is what
// puts a Latin phrase inside a Hebrew sentence back the right way round.
func reorderLevels(order, levels []int) {
	if len(order) == 0 {
		return
	}
	highest, lowestOdd := 0, MaxDepth+2
	for _, l := range levels {
		highest = max(highest, l)
		if l&1 == 1 {
			lowestOdd = min(lowestOdd, l)
		}
	}
	for level := highest; level >= lowestOdd; level-- {
		for i := 0; i < len(order); {
			if levels[i] < level {
				i++
				continue
			}
			j := i
			for j < len(order) && levels[j] >= level {
				j++
			}
			for a, b := i, j-1; a < b; a, b = a+1, b-1 {
				order[a], order[b] = order[b], order[a]
			}
			i = j
		}
	}
}

// Run is a stretch of a string that runs one way.
type Run struct {
	// Start and End are byte offsets into the string.
	Start, End int

	// Level is the resolved embedding level: odd runs right to left.
	Level int
}

// rtl reports whether the run is written right to left.
func (r Run) RTL() bool { return r.Level&1 == 1 }

// LogicalRuns splits a string into stretches that each run one way, in the
// order they are written.
//
// This and VisualOrder are the whole of what the rest of the package needs
// from UAX #9: shape each run — which is still in logical order, as shaping
// requires — and then draw the runs in the order VisualOrder gives, at a pen
// that only ever moves forward. Nothing outside this file has to know that
// direction exists.
//
// A character rule X9 removed keeps the level of what precedes it rather than
// dropping out. It has no width, so where it lands does not show — but a
// zero-width joiner is one of them, and Arabic joining needs it to still be
// between the letters it joins.
func LogicalRuns(text string) []Run {
	if text == "" {
		return nil
	}
	if !NeedsAlgorithm(text) {
		return []Run{{Start: 0, End: len(text), Level: 0}}
	}
	return ResolveRuns(text)
}

// NeedsAlgorithm reports whether a string contains anything that could make
// part of it run other than plain left-to-right.
//
// It is a shortcut, and it is worth having because the pipeline asks this
// question of every string it sets, most of which are Latin. Answering it costs
// one pass and no allocation; answering it by running the algorithm costs
// twenty. Measured on a sixty-character English sentence: 55ns and one
// allocation, against 5.2µs and twenty-one.
//
// The shortcut is sound because the rules can only move a character off level
// zero through a right-to-left or Arabic character, an Arabic number, or one of
// the explicit codes — and with none of those present, sos is left-to-right, so
// W7 turns every European number into plain left-to-right text and I1 has
// nothing left to lift. No ASCII character is any of those, which is why the
// scan can skip an ASCII prefix a byte at a time.
//
// bidi_test.go checks the two paths agree, because a shortcut that disagrees
// with the algorithm it is short-cutting is worse than no shortcut.
func NeedsAlgorithm(text string) bool {
	i := 0
	for i < len(text) && text[i] < utf8.RuneSelf {
		i++
	}
	for _, r := range text[i:] {
		switch ClassOf(r) {
		case R, AL, AN,
			LRE, RLE, LRO, RLO, PDF,
			LRI, RLI, FSI, PDI:
			return true
		}
	}
	return false
}

// ResolveRuns is LogicalRuns with the algorithm actually run.
func ResolveRuns(text string) []Run {
	runes := make([]rune, 0, len(text))
	offsets := make([]int, 0, len(text)+1)
	for i, r := range text {
		runes = append(runes, r)
		offsets = append(offsets, i)
	}
	offsets = append(offsets, len(text))

	classes := make([]Class, len(runes))
	for i, r := range runes {
		classes[i] = ClassOf(r)
	}
	p := resolveClasses(classes, runes, -1)

	// A removed character has no level of its own; give it the one before it so
	// that it stays inside the run it was written in.
	levels := make([]int, len(runes))
	carry := p.para
	for i, l := range p.levels {
		if l < 0 {
			l = carry
		}
		levels[i] = l
		carry = l
	}

	var runs []Run
	for i := 0; i < len(runes); {
		j := i + 1
		for j < len(runes) && levels[j] == levels[i] {
			j++
		}
		runs = append(runs, Run{Start: offsets[i], End: offsets[j], Level: levels[i]})
		i = j
	}
	return runs
}

// VisualOrder is rule L2 applied to whole runs rather than to characters:
// given the levels of a line's runs in the order they are written, it returns
// the order they are drawn in.
//
// Reordering runs and then reversing each right-to-left run's own contents is
// exactly L2 on the characters, because a stretch at a level or above is always
// a whole number of runs — and doing it this way leaves each run's characters in
// the logical order that shaping needs until shaping is done with them.
//
// The runs may be cut finer than the level runs, by face or by script, and the
// result is still right for the same reason: the finer cuts subdivide a level
// run without crossing it.
func VisualOrder(levels []int) []int {
	order := make([]int, len(levels))
	for i := range order {
		order[i] = i
	}
	reorderLevels(order, levels)
	return order
}

// VisualRuns is LogicalRuns and VisualOrder together: the stretches
// of a string that each run one way, in the order they are drawn. It is what a
// caller wants when it has nothing of its own to cut the runs by.
func VisualRuns(text string) []Run {
	runs := LogicalRuns(text)
	if len(runs) <= 1 {
		return runs
	}
	levels := make([]int, len(runs))
	for i, r := range runs {
		levels[i] = r.Level
	}
	out := make([]Run, len(runs))
	for i, k := range VisualOrder(levels) {
		out[i] = runs[k]
	}
	return out
}

// MirrorRunes applies rule L4 to a right-to-left run: each character that
// has a mirror is drawn as its mirror.
//
// It returns the slice it was given when nothing in it mirrors, which is the
// common case and saves the copy. It works on runes rather than on the string
// because a mirrored character need not be the same length in UTF-8 as the one
// it replaces, and the byte offsets are what map a glyph back to the text.
func MirrorRunes(runes []rune) []rune {
	var out []rune
	for i, r := range runes {
		m, ok := MirrorOf(r)
		if !ok {
			continue
		}
		if out == nil {
			out = make([]rune, len(runes))
			copy(out, runes)
		}
		out[i] = m
	}
	if out == nil {
		return runes
	}
	return out
}

// RunCharacters decomposes one run into the characters to be set and the
// byte offset each came from, mirroring where the run is right-to-left.
//
// The offsets are of the original characters, not of the mirrored ones: a glyph
// maps back to what the caller wrote, and rule L4 changes what is drawn without
// changing what the text says.
func RunCharacters(s string, rtl bool) (runes []rune, offsets []int) {
	runes = make([]rune, 0, len(s))
	offsets = make([]int, 0, len(s))
	for i, r := range s {
		runes = append(runes, r)
		offsets = append(offsets, i)
	}
	if rtl {
		runes = MirrorRunes(runes)
	}
	return runes, offsets
}
