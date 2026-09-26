package shape

import "unicode/utf8"

// What the text either side of a run can change about it.
//
// A caller laying out text cut into runs — by elements, by colour, by a change
// of size — gives each run the characters beside it, so that a word written
// across three runs is shaped as a word. It pays for that: the run is measured
// again with the context, and the context is kept on it until it is drawn. So
// it asks first whether the context can change anything, and the answer has to
// be right in both directions. A "no" that is wrong sets a letter in the wrong
// form; a "yes" that is wrong costs a measurement.
//
// The question used to be answered for the face, from the tables read with no
// script selected: whether the font had any positional forms, any pair
// kerning, any 'liga' ligature. That is three facts about the font, and the
// answer the caller needs is about the run — about the rules its script and
// language select, and about what this package actually reads from a context:
//
//   - The script of the characters that decide none. A run that opens with a
//     digit, a space or a punctuation mark sets those in the script of the text
//     before it (see scriptRuns), so a span holding only "：" between two
//     Chinese words is read under the font's Chinese rules. Which rules, which
//     model and which of the font's features apply are all the neighbour's to
//     say, and the face-wide question never asked about it.
//   - The joining forms. The Arabic model and the universal engine choose each
//     letter's form from the letters either side of it, and that decides
//     something only where the plan the run is set by has a lookup for one of
//     the forms (isol, init, medi, fina and the Syriac ones).
//   - The Indic word-initial form: a pre-base vowel sign that opens a word is
//     drawn with 'init', and whether it opens one is what precedes the run.
//   - The pair kerned across the edge of the run, where the rules the run's
//     script selects have pair positioning.
//
// A ligature is not among them, and neither is a contextual substitution: this
// package never shapes the glyphs of a context, it reads its characters. A
// ligature or a 'calt' rule that spans two runs is formed by shaping them as
// one string — the merge group, which a caller forms whatever this says (see
// ShapeGlyphsMerged). HasLigatures was part of the old answer and gated
// nothing that a ligature needed.

// ContextCanChange reports whether the text either side of s can change how
// this face shapes s: which glyphs it is set in, or where they sit. off is what
// the run turned off, and carries its language, which chooses the rules.
//
// A face set by character code reads no context at all, so the answer for one
// is no.
func (f *Face) ContextCanChange(s string, off Features) bool {
	if s == "" || !f.composite() {
		return false
	}
	if borrowsScript(s) {
		// The neighbour chooses the script, and with it the model and the
		// rules. Nothing about s can say which, so the answer is the one that
		// is safe whatever the neighbour turns out to be.
		return true
	}
	lang := openTypeLanguage(off.Language)
	var one [1]scriptRun
	for _, p := range scriptRuns(s, scriptUnknown, scriptUnknown, one[:0]) {
		l := f.layoutFor(p.script, lang)
		if len(l.kern) > 0 && !off.NoKerning {
			return true
		}
		if f.formsFollowIn(s[p.start:p.end], p.script, lang, off) {
			return true
		}
	}
	return false
}

// FormsFollowNeighbours reports whether the form a character of s is drawn in
// can depend on the characters either side of s — the joining forms of a
// cursive script, or the Indic word-initial form — as the rules the run's
// script and language select say. It is ContextCanChange without the kerning.
//
// A run whose script is its neighbour's to choose is answered for every script
// the face has rules for, since any of them may be the one: HasJoiningForms.
func (f *Face) FormsFollowNeighbours(s string, off Features) bool {
	if s == "" || !f.composite() {
		return false
	}
	if borrowsScript(s) {
		return f.HasJoiningForms()
	}
	lang := openTypeLanguage(off.Language)
	var one [1]scriptRun
	for _, p := range scriptRuns(s, scriptUnknown, scriptUnknown, one[:0]) {
		if f.formsFollowIn(s[p.start:p.end], p.script, lang, off) {
			return true
		}
	}
	return false
}

// formsFollowIn reports whether a run of one script reads its context to choose
// a form, and has a rule in its plan for the form it would choose.
//
// It asks the plan rather than the font's feature list because the plan is
// what is applied: a form the model marks but no lookup of the plan is for
// changes nothing, and a font whose forms are declared under one script's
// rules does not give them to a run of another.
func (f *Face) formsFollowIn(piece string, script uint16, lang otLanguage, off Features) bool {
	l := f.layoutFor(script, lang)
	model := categorize(script, f.chosenScriptTag(script, lang))
	var mask glyphMask
	switch model {
	case modelArabic:
		mask = joiningMasks
	case modelUniversal:
		// The universal engine chooses joining forms only for a run with a
		// letter of a cursive script in it. See shapeUniversal.
		if !anyCursiveIn(piece) {
			return false
		}
		mask = joiningMasks
	case modelIndic:
		mask = maskInit
	default:
		return false
	}
	sh := shaper{f: f, l: l, features: off, lang: lang}
	return sh.planFor(model, scriptSelects(script, "arab"), nil).hasMask(mask)
}

// joiningMasks is every form a joining scan can choose. See markJoiningForms.
const joiningMasks = maskIsol | maskFina | maskFin2 | maskFin3 | maskMedi | maskMed2 | maskInit

// borrowsScript reports whether the first character of s, with the marks on
// it, decides no script, so that scriptRuns sets it in the script of the text
// before s. It is the first unit scriptRuns reads, read the same way.
func borrowsScript(s string) bool {
	r, n := utf8.DecodeRuneInString(s)
	if decides(scriptOf(r)) {
		return false
	}
	for i := n; i < len(s); {
		m, k := utf8.DecodeRuneInString(s[i:])
		if !isCombiningMark(m) {
			break
		}
		if decides(scriptOf(m)) {
			return false
		}
		i += k
	}
	return true
}

func anyCursiveIn(s string) bool {
	for _, r := range s {
		if InCursiveScript(r) {
			return true
		}
	}
	return false
}
