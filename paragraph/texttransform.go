package paragraph

import (
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/forme/internal/charprop"
)

// text-transform: changing the case of the text before anything measures it.
//
// CSS Text §2.1. The property was registered and unread, so "text-transform:
// uppercase" on a heading produced lower-case text and no finding — and this one
// is worse than most of its neighbours, because it is the sort of declaration a
// house style is built on: every heading in a template is wrong at once.
//
// # Why this happens in the box tree and not at paint time
//
// It changes the text, and the text is what everything downstream measures,
// breaks, sets and *extracts*. Transforming at paint time would leave the line
// breaking measuring "internationalization" and the page showing
// "INTERNATIONALIZATION", which is wider in every face this engine has — so the
// lines would overflow by the difference, silently, because layout never saw the
// wider string.
//
// It also means the PDF carries the transformed text, so a reader copying a
// transformed heading out of the page gets it in the case it was drawn in rather
// than the case it was written in. A browser copies the source text, because a
// browser still has the DOM beside the rendering; a PDF has only what was drawn.
// Carrying the original as well would need /ActualText on a marked-content span
// around every transformed run, which is a change to how text is emitted rather
// than an addition to it.
//
// # What "word" means here
//
// "capitalize" titlecases the first letter of each word, and CSS Text defines a
// word by UAX #29's word boundaries. What is implemented is narrower and stated
// rather than discovered: a letter starts a word when the character before it is
// not a letter, a digit or an apostrophe. That gives "well-known" two capitals
// and "don't" one, which is what browsers produce for both, and it differs from
// UAX #29 for scripts that mark word boundaries some other way — the same
// scripts inline.go already refuses to break at all.
//
// The boundary is carried *across* text nodes, because a word can be: in
// "<b>e</b>xample" the "x" does not begin a word, and a version that started
// each text node afresh would set "EXample". That is what boxBuilder.afterWord
// is for.
//
// # Why the case mappings are not Go's alone
//
// strings.ToUpper applies Unicode's *simple* mappings, which are one character
// to one character by construction. A great deal of ordinary text does not have
// a one-to-one case: "straße" uppercases to "STRASSE", and a simple mapping
// cannot say so, so Go leaves the ß alone and produces "STRAßE". CSS Text
// §2.1.1 asks for the full mappings by name, and casingtable.go holds them —
// see cmd/gencasing for where they come from and which were left out.
//
// The tables are consulted first and the simple mapping is the fallback, so a
// character with no full mapping — which is all but a hundred of them — costs a
// binary search over a table of a hundred entries and nothing else. Text that
// contains no such character takes the whole-string path, upperString or
// lowerString. The simple mappings are generated too, from the same release,
// and not Go's — see simplecasing.go for what Go's cost.
//
// # The conditional mappings
//
// All four are applied, and each needed something the tables above do not have.
// Three are language tailorings — Turkish and Azeri map i and I to their dotted
// and dotless forms, Lithuanian keeps a dot above a lowercased vowel — and take
// the element's declared language, which reaches here as a Language. The fourth
// is Final_Sigma: a lowercased Σ is ς at the end of a word and σ inside one,
// which needs the characters either side rather than a table. See
// localecasing.go for all four, and greekcasing.go for the tailoring that is a
// whole-run rule rather than a per-character one.
//
// # What is done and looks like a fault
//
// Two of the suite's tests assert the *simple* mapping for characters Unicode
// gives a full one, and both are left failing rather than special-cased.
// text-transform-upperlower-016 wants "ᾀ" to uppercase to "ᾈ" and
// text-transform-upperlower-006 wants "İ" to lowercase to "i"; Unicode says
// "ἈΙ" and "i̇", and so do the two newer tests beside them —
// text-transform-upperlower-035 spells out the same mappings and
// text-transform-lowercase-102 is exactly the "İ" case. ᾈ is the *titlecase*
// of ᾀ, which is a third mapping and is applied where a third mapping belongs.
// The suite contradicts itself here and the specification does not.
//
// Uppercasing Georgian Mkhedruli would produce Mtavruli, so "ა" would become
// "Ა": Unicode 11 gave every Mkhedruli letter an uppercase mapping into the
// Mtavruli block. The suite's text-transform-unicase-001 asserts that it must
// not — "verifies that text-transform does not capitalize a unicase script" —
// and this engine agrees: Mtavruli is a display style and not a case, so the
// uppercase mapping leaves Mkhedruli alone. See isMkhedruli, and caseMapping,
// which is where every path that uppercases asks it.

// TextTransform is what the property asks for, as a set rather than a choice.
//
// CSS Text 3 §2.1.1 states the grammar as
//
//	none | [ capitalize | uppercase | lowercase ] || full-width || full-size-kana
//
// which is one case change *and* either remapping, in any combination and in any
// order — "text-transform: full-width full-size-kana lowercase" is a declaration
// the suite writes. So the value is a set of bits and not one of five things.
type TextTransform uint8

const (
	TransformNone TextTransform = 0

	// The three case changes, which the grammar makes mutually exclusive.
	TransformUppercase TextTransform = 1 << iota
	TransformLowercase
	TransformCapitalize

	// The two remappings, which combine with a case change and with each other.
	TransformFullWidth
	TransformFullSizeKana

	// TransformMathAuto is CSS Text 4's math-auto, which is a keyword of its
	// own: the grammar puts it beside "none" rather than among the three that
	// combine, so it is valid alone and invalid beside anything else. See
	// mathauto.go.
	TransformMathAuto

	// transformCase is the part of a value that changes case, for the places
	// that need to ask which of the three was given without naming all three.
	transformCase = TransformUppercase | TransformLowercase | TransformCapitalize
)

// TransformOf reads the property.
//
// An unrecognised value is "none", which is what the cascade would have produced
// had the declaration been thrown out — and that goes for the whole declaration
// rather than the keyword that was not recognised, because a declaration with
// one bad keyword in it is invalid CSS and is dropped entire. Two case changes
// are refused for the same reason: the grammar allows one.
func TransformOf(value string) TextTransform {
	var out TextTransform
	for _, word := range strings.Fields(strings.ToLower(value)) {
		var bit TextTransform
		switch word {
		case "none":
			// Valid on its own, and invalid beside anything else. Both answers
			// are the same one, so there is nothing to distinguish here.
			return TransformNone
		case "uppercase":
			bit = TransformUppercase
		case "lowercase":
			bit = TransformLowercase
		case "capitalize":
			bit = TransformCapitalize
		case "full-width":
			bit = TransformFullWidth
		case "full-size-kana":
			bit = TransformFullSizeKana
		case "math-auto":
			// Alone or not at all. §2.1.1's grammar is
			// "none | math-auto | [ [capitalize|uppercase|lowercase] ||
			// full-width || full-size-kana ]", so math-auto is its own branch
			// and shares the alternation with none rather than the set.
			if len(strings.Fields(strings.ToLower(value))) != 1 {
				return TransformNone
			}
			return TransformMathAuto
		default:
			return TransformNone
		}
		if out&bit != 0 || (bit&transformCase != 0 && out&transformCase != 0) {
			return TransformNone
		}
		out |= bit
	}
	return out
}

// TransformText applies the property to one text node.
//
// inWord says whether the character before this text — which may be in another
// element — was part of a word, so that "capitalize" does not capitalise the
// middle of one. It returns the same answer for the text it produced, for the
// node after it.
//
// It allocates once at most: "none" returns the string it was given, and the
// case transforms build one buffer of the size of the input. A megabyte of text
// is a megabyte of work and not a rune of garbage per character.
func TransformText(text string, kind TextTransform, state WordState, lang Language) (string, WordState) {
	out, next, _, _ := TransformTextIn(text, kind, state, lang, CaseContext{})
	return out, next
}

// CaseContext is what one text node leaves the next for Final_Sigma, whose
// context crosses a box boundary as capitalize's does: "ΟΔΟΣ<b>ΑΚΙ</b>" is one
// word, and the Σ at the end of the first node is inside it.
//
// It carries the half of the condition that looks back — whether the last
// character that is not case-ignorable was cased. The half that looks forward
// cannot be carried, because the text after a node has not been seen when the
// node is transformed: a sigma whose following context runs off the end of its
// node is lowercased as final, and TransformTextIn says where it put it, so that
// the caller can correct it with UnfinalSigma once CasedAhead of the next node
// says a cased letter follows.
type CaseContext struct {
	// CasedBefore says the last character written that is not case-ignorable
	// was cased.
	CasedBefore bool
}

// TransformTextIn is TransformText with the Final_Sigma context carried: ctx is
// what the text before this node left, next is what this node leaves, and
// openSigma is the byte offset in out of a ς whose finality the next node has to
// confirm, or -1.
func TransformTextIn(text string, kind TextTransform, state WordState, lang Language,
	ctx CaseContext) (out string, next WordState, nextCtx CaseContext, openSigma int) {

	nextCtx = caseContextAfter(text, ctx)
	openSigma = -1
	if text == "" {
		return text, state, nextCtx, openSigma
	}
	if kind&transformCase == TransformLowercase {
		// The one case change Final_Sigma is part of. The rest of what
		// TransformText does after the case change is the same for it.
		text, openSigma = localeLowercased(text, lang, ctx)
		kind &^= transformCase
		out, next = transformRest(text, kind, state)
		if out != text {
			// A width remapping changed the bytes, so the offset no longer
			// names the sigma. It is a sigma a fullwidth or kana remapping
			// leaves alone, but the offset is not worth defending: the
			// correction is dropped, and the sigma stays final.
			openSigma = -1
		}
		return out, next, nextCtx, openSigma
	}
	out, next = transformText(text, kind, state, lang)
	return out, next, nextCtx, openSigma
}

// caseContextAfter is the CaseContext a node leaves: its last character that is
// not case-ignorable, or what it was given where it has none.
func caseContextAfter(text string, ctx CaseContext) CaseContext {
	for i := len(text); i > 0; {
		r, size := utf8.DecodeLastRuneInString(text[:i])
		i -= size
		if caseIgnorable(r) {
			continue
		}
		return CaseContext{CasedBefore: cased(r)}
	}
	return ctx
}

// transformRest is TransformText after the case change: the two remappings, in
// the specification's order, and the word state the text leaves.
func transformRest(text string, kind TextTransform, state WordState) (string, WordState) {
	if kind&TransformFullWidth != 0 {
		text = remapped(text, fullWidthForms[:])
	}
	if kind&TransformFullSizeKana != 0 {
		text = remapped(text, fullSizeKana[:])
	}
	return text, WordStateAfter(text, state)
}

// transformText is TransformText without the Final_Sigma context.
func transformText(text string, kind TextTransform, state WordState, lang Language) (string, WordState) {
	if text == "" {
		return text, state
	}
	// The order is the specification's and is not the order the keywords were
	// written in: case first, then width, then size. §2.1.1's own example is
	// "full-width full-size-kana lowercase", which lowercases first.
	//
	// Only the first of the two orderings is observable, and that was measured
	// rather than assumed: over every character of Unicode, seventeen tell case
	// from width apart — ß among them, because "SS" has a fullwidth form and ß
	// has none — and *none* tells full-width from full-size-kana, or either of
	// them from a case change. The code follows the specification's order all the
	// same; texttransform_test.go says which part of it a test can hold.
	if kind == TransformMathAuto {
		// On its own by construction — see TransformOf — so it is answered
		// first and nothing else runs. A text node of anything but exactly one
		// character comes back as it went in.
		return mathAuto(text), WordStateAfter(text, state)
	}
	switch kind & transformCase {
	case TransformUppercase:
		text = localeCased(text, lang, true)
	case TransformLowercase:
		text, _ = localeLowercased(text, lang, CaseContext{})
	case TransformCapitalize:
		text = capitalizeWords(text, state, lang)
	}
	return transformRest(text, kind, state)
}

// remapped replaces every character that one of the width tables names.
//
// Unlike a case change this is one character for one character, so the result
// is the same number of characters as the text — though not the same number of
// bytes, since "a" is one and "ａ" is three. Text that names none of them is
// returned as it arrived rather than copied, which is the ordinary case for
// full-size-kana in particular: a page setting it has kana on some of its lines
// and not on the rest.
func remapped(text string, table []widthPair) string {
	i := firstRemapped(text, table)
	if i < 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text) + 8)
	out.WriteString(text[:i])
	for _, r := range text[i:] {
		if to, ok := lookupWidth(r, table); ok {
			out.WriteRune(to)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// firstRemapped returns the byte offset of the first character the table names,
// or -1 if it names none of them.
//
// There is no ASCII shortcut here, and that is the difference from
// firstFullCase: nearly half of the fullwidth table is ASCII, because turning
// "6" into "６" is what the value is for.
func firstRemapped(text string, table []widthPair) int {
	for i, r := range text {
		if _, ok := lookupWidth(r, table); ok {
			return i
		}
	}
	return -1
}

// lookupWidth searches one of the generated width tables, which are sorted.
func lookupWidth(r rune, table []widthPair) (rune, bool) {
	i, j := 0, len(table)
	for i < j {
		h := int(uint(i+j) >> 1)
		if table[h].from < r {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(table) && table[i].from == r {
		return table[i].to, true
	}
	return 0, false
}

// localeCased is the case change with the conditional mappings applied.
//
// The conditions are a handful of characters in a handful of languages plus the
// Greek final sigma — see localecasing.go — so the whole of this is skipped
// unless the text really contains one of the characters they are about. Every
// other run takes the same path it always did.
func localeCased(text string, lang Language, upper bool) string {
	if !upper {
		out, _ := localeLowercased(text, lang, CaseContext{})
		return out
	}
	if lang == "el" {
		// Greek drops its accents in capitals, which is a whole-run rule rather
		// than a per-character mapping: an accent removed from one vowel puts a
		// dialytika on the next. See greekcasing.go.
		if got := greekUppercase(text); got != "" {
			return got
		}
	}
	if i := firstConditional(text, lang, true); i >= 0 {
		out, _ := conditionalCased(text, lang, true, i, CaseContext{})
		return out
	}
	return uppercasing.cased(text)
}

// localeLowercased is localeCased's lowercase, with the Final_Sigma context and
// the offset of a sigma left open at the end. See TransformTextIn.
func localeLowercased(text string, lang Language, ctx CaseContext) (string, int) {
	if i := firstConditional(text, lang, false); i >= 0 {
		return conditionalCased(text, lang, false, i, ctx)
	}
	return lowercasing.cased(text), -1
}

// caseMapping is one case change as a mapping of one character: the full
// mapping where the tables have one, the simple one otherwise, and a character
// the change must leave alone left alone.
//
// It is the one answer every path gives. There were three, and they parted at
// the edges: the whole-string path left Georgian Mkhedruli alone under
// uppercase, the Greek path did too, and the per-character path that takes over
// once a Turkish i or a Lithuanian dot is found in the text did not — so "i ა"
// under lang="tr" uppercased the Georgian letter to Mtavruli, and the same text
// in any other language did not. Audit C120.
type caseMapping struct {
	table  []fullCase
	simple func(rune) rune
	whole  func(string) string
	// keep is what the change leaves as it is. See isMkhedruli.
	keep func(rune) bool
}

var (
	uppercasing = caseMapping{fullUppercase[:], simpleUpper, upperString, isMkhedruli}
	lowercasing = caseMapping{fullLowercase[:], simpleLower, lowerString, nil}
)

// of is the mapping of r where it is not the simple one: r itself where the
// change keeps it, or its full mapping.
func (m caseMapping) of(r rune) (string, bool) {
	if m.keep != nil && m.keep(r) {
		return string(r), true
	}
	return lookupFullCase(r, m.table)
}

// write appends the mapping of r.
func (m caseMapping) write(out *strings.Builder, r rune) {
	if s, ok := m.of(r); ok {
		out.WriteString(s)
		return
	}
	out.WriteRune(m.simple(r))
}

// cased maps a whole string. See fullCased.
func (m caseMapping) cased(text string) string {
	return fullCased(text, m.table, m.simple, m.whole, m.keep)
}

// firstConditional is the byte offset of the first character a conditional
// mapping could be about, or -1.
//
// It asks the mappings themselves rather than carrying a second list of
// characters that would have to be kept in step with them: for the first
// character that answers, the answer is the whole test.
func firstConditional(text string, lang Language, upper bool) int {
	for i, r := range text {
		if upper {
			// With the text in front of it, because one of the uppercase
			// conditions reads it: Lithuanian removes a dot above only where a
			// soft-dotted letter is what it is above.
			if _, ok := localeUpper(r, text[:i], lang); ok {
				return i
			}
			continue
		}
		// The backward-looking conditions cannot fire on a character the
		// forward-looking test would miss, because every one of them names the
		// character itself: it is I, or the dotted capital, or a combining dot,
		// or a sigma. So the cheap test is whether this character is one of
		// those at all, which is what asking with empty context does — except
		// for the two whose condition is *absence*, which answer true there.
		if r == 0x0130 || r == 'I' || r == 0x0307 || r == 0x03A3 ||
			r == 'J' || r == 0x012E || r == 0x00CC || r == 0x00CD || r == 0x0128 {
			return i
		}
	}
	return -1
}

// conditionalCased maps the text a character at a time from the first character
// a condition could be about, which is where the cheap whole-string path stops
// being available.
//
// ctx and the second result are Final_Sigma's, for lowercase: see
// TransformTextIn.
func conditionalCased(text string, lang Language, upper bool, from int, ctx CaseContext) (string, int) {
	m := uppercasing
	if !upper {
		m = lowercasing
	}
	var out strings.Builder
	out.Grow(len(text) + 8)
	out.WriteString(m.cased(text[:from]))
	open := -1
	for i, r := range text[from:] {
		at := from + i
		after := text[at+utf8.RuneLen(r):]
		var (
			s  string
			ok bool
		)
		switch {
		case upper:
			s, ok = localeUpper(r, text[:at], lang)
		case r == 0x03A3:
			// Final_Sigma, which is not a tailoring: it is decided the same in
			// every language, and its context reaches into the nodes either
			// side of this one.
			final, undecided := finalSigma(text[:at], after, ctx.CasedBefore)
			if final {
				if undecided {
					open = out.Len()
				}
				out.WriteString("ς")
				continue
			}
		default:
			s, ok = localeLower(r, text[:at], after, lang)
		}
		if ok {
			out.WriteString(s)
			continue
		}
		m.write(&out, r)
	}
	return out.String(), open
}

// fullCased maps every character of a string, preferring the full mapping.
//
// The whole-string function is the fast path and does the work whenever no
// character of the text has a full mapping — which is the ordinary case, and
// keeps an ASCII heading on the byte-wise loop inside upperString rather
// than on a rune-by-rune one here. Only text that really does contain one of
// the hundred characters in the table is rebuilt.
func fullCased(text string, table []fullCase, simple func(rune) rune, whole func(string) string,
	keep func(rune) bool) string {

	i := firstFullCase(text, table, keep)
	if i < 0 {
		return whole(text)
	}
	var out strings.Builder
	// The mappings are longer than what they replace, so this is a floor rather
	// than a guess; it saves the first growth and not the rest.
	out.Grow(len(text) + 8)
	out.WriteString(whole(text[:i]))
	m := caseMapping{table: table, simple: simple, keep: keep}
	for _, r := range text[i:] {
		m.write(&out, r)
	}
	return out.String()
}

// firstFullCase returns the byte offset of the first character of the text that
// has a full mapping, or -1 if none has.
//
// Every character in the tables is above U+007F, so ASCII — which is most text
// this will ever see — is rejected a byte at a time without decoding.
func firstFullCase(text string, table []fullCase, keep func(rune) bool) int {
	for i, r := range text {
		if r < utf8.RuneSelf {
			continue
		}
		if keep != nil && keep(r) {
			// Not a full mapping but a character the whole-string path would
			// get wrong just the same, so the walk has to start here too.
			return i
		}
		if _, ok := lookupFullCase(r, table); ok {
			return i
		}
	}
	return -1
}

// lookupFullCase searches one of the generated tables, which are sorted.
func lookupFullCase(r rune, table []fullCase) (string, bool) {
	i, j := 0, len(table)
	for i < j {
		h := int(uint(i+j) >> 1)
		if table[h].r < r {
			i = h + 1
		} else {
			j = h
		}
	}
	if i < len(table) && table[i].r == r {
		return table[i].s, true
	}
	return "", false
}

// capitalizeWords titlecases the first letter of every word.
//
// Titlecase rather than uppercase, which matters for exactly the digraphs it was
// invented for: U+01F3 "ǳ" titlecases to "ǲ" and uppercases to "Ǳ", and a name
// set in the second form is set wrongly. It is also a third mapping rather than
// a variation on the other two — "ß" titlecases to "Ss" and uppercases to "SS" —
// so it has a table of its own.
//
// "Letter" is §1.3's typographic letter unit and not unicode.IsLetter: "a
// typographic character unit belonging to one of the Letter or Number general
// categories". The difference is the letter-numbers — the Roman numerals of
// U+2160, the Suzhou numerals, the Hangzhou ones — which are letters that count.
// U+2170 SMALL ROMAN NUMERAL ONE titlecases to U+2160 and was left as it stood,
// while still ending the word for everything after it: "ⅰⅰⅰ" came out unchanged
// where "Ⅰⅰⅰ" was asked for. It is the same set isWordRune uses two lines below,
// which is what makes the two agree about where a word begins.
func capitalizeWords(text string, state WordState, lang Language) string {
	var out strings.Builder
	out.Grow(len(text))
	// titled says the unit being written began with a letter this titlecased,
	// so that the marks on it are titlecased with it.
	titled := false
	for i := 0; i < len(text); {
		if state == WordClosed && lang == "nl" {
			// IJ is one letter of the Dutch alphabet written as two, so a word
			// beginning with it takes two capitals. See dutchCapitalize.
			if got, ok := dutchCapitalize(text, i); ok {
				out.WriteString(got)
				i += 2
				state = WordOpen
				continue
			}
		}
		r, size := rune(text[i]), 1
		if r >= utf8.RuneSelf {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		i += size

		switch {
		case state == WordClosed && isWordRune(r):
			// The language's tailoring first, as uppercase has it: CSS Text
			// §2.1 makes all three case changes language-sensitive, and a
			// Turkish "istanbul" capitalises to "İstanbul". It was
			// "Istanbul", a word spelled with the other letter. Audit C121.
			if s, ok := localeUpper(r, text[:i-size], lang); ok {
				out.WriteString(s)
			} else if s, ok := lookupFullCase(r, fullTitlecase[:]); ok {
				out.WriteString(s)
			} else {
				out.WriteRune(simpleTitle(r))
			}
			titled = true
		case titled && isCombiningMark(r):
			// A mark on the letter just titlecased is part of the unit that
			// was, and the tailoring may be about it rather than about the
			// letter: Lithuanian removes the dot above a titlecased i
			// ("0307; 0307; ; ; lt After_Soft_Dotted" — its titlecase field is
			// empty).
			if s, ok := localeUpper(r, text[:i-size], lang); ok {
				out.WriteString(s)
			} else {
				out.WriteRune(r)
			}
		default:
			titled = false
			out.WriteRune(r)
		}
		state = stepWord(state, r, text[i:])
	}
	return out.String()
}

// WordState is what one text node leaves behind for the next: whether a word is
// still open, and — the case a yes-or-no cannot hold — whether the character
// that would settle it has not been seen yet.
//
// The third state is not a refinement. A text node ending in an apostrophe
// leaves a word open if a letter follows it and closes one if anything else
// does, and what follows is in the next node. Carried as a yes it set
// "<span>a'</span><span>'b</span>" as one word and gave it one capital where
// "a”b" in one node takes two; carried as a no it broke "don" and "'t".
type WordState uint8

const (
	// WordClosed: the last character ended a word, so the next letter begins one.
	WordClosed WordState = iota
	// WordOpen: the last character was a letter or a number.
	WordOpen
	// WordPending: the last character was a joiner after a letter, and whether
	// it joined depends on what the next node begins with.
	WordPending
)

// stepWord advances "is a word still open" past one character, given what
// follows it in the same text.
//
// It is one function because it is one question, and it used to be two answers.
// capitalizeWords tracked the state as it went and EndsInWord read the last
// character on its own, and the two disagreed about an apostrophe with nothing
// in front of it: "'a" in one text node capitalised to "'A", and the same two
// characters in two nodes — "<span>'</span><span>a</span>" — came out "'a",
// because the first node was read as ending inside a word. FuzzBoundaryLines
// found it, which is what that target is for: the same characters, cut two ways,
// have to set the same line.
func stepWord(state WordState, r rune, rest string) WordState {
	if isWordRune(r) {
		return WordOpen
	}
	if isCombiningMark(r) || charprop.Is(r, charprop.Cf) {
		// UAX #29's WB4: a mark, a format character or a joiner belongs to the
		// character before it and changes nothing about where the word is.
		// Read as ending the word, a decomposed "résumé" capitalised to
		// "RéSumé" — the combining acute closed the word and the "s" opened
		// another.
		return state
	}
	if !isMidWord(r) {
		return WordClosed
	}
	// UAX #29's WB6 and WB7: a MidLetter or MidNumLet joins a word only
	// *between* two letters. Both sides are conditions — "cancel·lar" is one
	// Catalan word and "cancel· lar" is two, and "'a" is one word whose letter
	// is its first, because an apostrophe with nothing in front of it joins
	// nothing.
	//
	// The side in front is WordOpen and not "a word is somehow still going":
	// arriving here in WordPending means the character before this one was a
	// joiner too, and a joiner is not a letter for WB6 to be between. "a''b" is
	// two words and takes two capitals.
	if state != WordOpen {
		return WordClosed
	}
	if rest == "" {
		// Nothing after it *here*, and the letter that would settle it is in the
		// next text node. This is the state that cannot be written as a
		// yes-or-no: "don'" leaves a word open if a letter follows and closes one
		// if a joiner does, and which it is belongs to the node after this.
		return WordPending
	}
	if beginsWithWordRune(rest) {
		return WordOpen
	}
	return WordClosed
}

// beginsWithWordRune reports whether the next character continues a word, which
// is what WB6's "between" needs to know.
func beginsWithWordRune(text string) bool {
	if text == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text)
	return isWordRune(r)
}

// isMidWord is UAX #29's MidLetter and MidNumLet: the characters that join two
// halves of one word rather than separating two words.
//
// The list is Unicode's and is short enough to write out. The two apostrophes
// are on it — without them "don't" comes out "Don'T", which is a real word set
// wrongly rather than a theoretical one — and so is the middle dot, which is a
// letter of Catalan: "cancel·lar" is one word and came out "Cancel·Lar". The
// suite's text-transform-capitalize-035 is six of those, in four languages.
//
// The full stop is MidNumLet and is here for the same reason: "e.g." is one
// word, and "E.G." is not what capitalising it gives.
func isMidWord(r rune) bool {
	switch r {
	// MidLetter.
	case ':', '\u00b7', '\u0387', '\u055f', '\u05f4', '\u2027',
		'\ufe13', '\ufe55', '\uff1a':
		return true
	// MidNumLet.
	case '\'', '.', '\u2018', '\u2019', '\u2024', '\ufe52', '\uff07', '\uff0e':
		return true
	}
	return false
}

// isCombiningMark reports whether a character is a combining mark: one that
// belongs to the character before it.
func isCombiningMark(r rune) bool {
	return charprop.Is(r, charprop.M)
}

// isWordRune reports whether a character is one a word is made of.
//
// The characters that *join* two halves of a word without being letters
// themselves — the apostrophe of "don't", the middle dot of "cancel·lar" — are
// not here: they continue a word only when a letter follows, which is a question
// about the next character and belongs to the caller. See isMidWord.
func isWordRune(r rune) bool {
	return charprop.Is(r, charprop.L|charprop.N)
}

// WordStateAfter is what a text node leaves behind for the next one.
//
// It takes the state the node began in as well as its text, and both halves of
// that are the fix for one defect. Its predecessor read the last character alone
// and called every joining character a word, apostrophe included, without asking
// what was in front of it — so "'" on its own ended inside a word and the "a"
// after it in the next node was not capitalised, while the same "'a" in one node
// was. The last character cannot answer this: whether an apostrophe joins
// anything depends on what is in front of it, and what is in front of it can be
// in the node before.
//
// The walk is the same stepWord capitalizeWords uses, which is what keeps the
// two from drifting apart again. See TestTheTwoWalksAgreeAboutWhereAWordEnds.
func WordStateAfter(text string, state WordState) WordState {
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		i += size
		state = stepWord(state, r, text[i:])
	}
	return state
}

// FreezesSpace reports whether a text-transform turns an ordinary space into a
// character white space processing is not about.
//
// "full-width" does, and it is the only one that does: its remapping includes
// U+0020 to U+3000 IDEOGRAPHIC SPACE, which §4.1 counts among the "other space
// separators" rather than among the collapsible white space, so nothing after
// the transform may collapse it. See Boundary.Collapsed for what has to happen
// before it because of that.
func FreezesSpace(kind TextTransform) bool {
	return kind&TransformFullWidth != 0
}

// isMkhedruli reports whether a character is a Georgian Mkhedruli letter, which
// "text-transform: uppercase" must leave exactly as it is.
//
// Unicode 11 added the Mtavruli capitals at U+1C90 and gave each Mkhedruli
// letter a simple uppercase mapping to one — so unicode.ToUpper turns ა into Ა,
// and every uppercased word of Georgian comes out in a case the script does not
// use in running text. Georgian is *unicase*: Mtavruli is a display style, set
// deliberately for a heading or a sign, and never something a case conversion
// should produce. The suite says so in one line — text-transform-unicase-001
// writes the letter twice, uppercases one of them, and asks for the two to look
// exactly alike.
//
// Uppercase only. The Asomtavruli capitals at U+10A0 keep their traditional
// mapping to Nuskhuri, and lowercasing a Mtavruli capital that a document really
// does contain is the mapping Unicode added the characters for.
//
// Passing it to the lowercase call as well would change no text — a Mkhedruli
// letter is already lower case and unicode.ToLower leaves it exactly where this
// would — so a planted defect that does so moves nothing. It is still wrong to
// write: it says the carve-out is about Georgian rather than about the mapping
// that is not wanted, and it would take every Georgian document off the
// whole-string fast path for no reading of any rule.
//
// The two runs rather than one are Unicode's own: U+10FB and U+10FC are a
// paragraph separator and a modifier letter, and neither has a Mtavruli form.
func isMkhedruli(r rune) bool {
	return r >= 0x10D0 && r <= 0x10FA || r >= 0x10FD && r <= 0x10FF
}
