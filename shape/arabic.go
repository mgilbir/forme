package shape

import (
	"slices"
	"sort"

	"github.com/mgilbir/forme/internal/charprop"
)

// Cursive joining: choosing which of a letter's four shapes to draw.
//
// A cursive script — Arabic above all, and Syriac, Mongolian, Adlam and others
// — writes a letter differently depending on what it joins to. The font
// supplies the shapes and Unicode says which letters can join in which
// direction; putting the two together is what turns a row of disconnected
// letterforms into writing.
//
// It is decidable from the characters alone, it needs no reordering, and its
// absence is not subtle: Arabic set without it is not merely ugly but hard to
// read, in a way a reader will notice immediately and a developer who does not
// read Arabic will not.
//
// # What this does not do
//
// It chooses forms. It does not reorder, which Indic scripts require — a vowel
// written after a consonant may belong before it — and it does not join the
// strokes themselves, which is cursive attachment (GPOS 3). Reordering is
// indic.go's, and the two are alternatives rather than stages: no script both
// joins cursively and reorders.
//
// Three things every other shaper does were absent here, and absent without
// being written down, which is worse than being absent. All three are done
// now:
//
//   - Syriac's Alaph. The letter U+0710 takes a final form chosen by what
//     precedes it rather than by what it joins to, which is a rule of its own
//     over and above the four shapes — HarfBuzz spells it as states of its
//     joining scan with feature tags of their own ('fin2', 'fin3', 'med2').
//     joinForms is that scan.
//   - 'stch', the stretching feature Syriac writes its abbreviation mark with:
//     a bar over the word, stretched to its width. See stch.go.
//   - The fallback for an Arabic font that declares none of 'init', 'medi',
//     'fina' or 'isol': the forms are drawn out of the Arabic Presentation
//     Forms the face maps, as HarfBuzz draws them. See arabicfallback.go.

// joiningType is what a character can join to.
type joiningType uint8

const (
	joinU joiningType = iota // non-joining
	joinL                    // joins to the left only
	joinR                    // joins to the right only
	joinD                    // dual-joining: both sides
	joinC                    // join-causing: joins neighbours without a shape of its own
	joinT                    // transparent: skipped, and does not break a join
)

// joiningTypeOf reports a character's joining type.
//
// A character the table does not name is non-joining, except a non-spacing
// mark, an enclosing mark or a format character, which is transparent — a
// vowel sign written between two letters must not break their join, and
// treating it as an ordinary character would.
//
// Which characters those are is the generated defaultTransparentRanges, from
// the release the joining table is from. It was package unicode, whose release
// is older: U+0897 ARABIC PEPET, a mark since Unicode 16, broke the join of the
// letters either side of it, and U+1171E, a mark in Unicode 15 and not since,
// was stepped over.
func joiningTypeOf(r rune) joiningType {
	i := sort.Search(len(joiningRanges), func(i int) bool { return joiningRanges[i].hi >= r })
	if i < len(joiningRanges) && r >= joiningRanges[i].lo {
		return joiningRanges[i].t
	}
	i = sort.Search(len(defaultTransparentRanges), func(i int) bool { return defaultTransparentRanges[i].hi >= r })
	if i < len(defaultTransparentRanges) && r >= defaultTransparentRanges[i].lo {
		return joinT
	}
	return joinU
}

// The form a joining scan chooses for a character, in the order of the
// features that draw them: HarfBuzz's arabic_action_t. formNone is a character
// that takes no form of its own — one that cannot join, or a transparent one.
const (
	formIsol uint8 = iota
	formFina
	formFin2
	formFin3
	formMedi
	formMed2
	formInit
	formNone
)

// formMasks is the mask of the feature that draws each form.
var formMasks = [...]glyphMask{
	formIsol: maskIsol, formFina: maskFina, formFin2: maskFin2, formFin3: maskFin3,
	formMedi: maskMedi, formMed2: maskMed2, formInit: maskInit,
}

// The columns of the joining state table: the four joining types a letter can
// have, with join-causing read as dual-joining, and the two joining groups the
// Syriac Alaph's forms turn on. A transparent character has no column; it is
// stepped over.
const (
	colU = iota
	colL
	colR
	colD
	colAlaph
	colDalathRish
	colTransparent = -1
)

// joiningColumnOf is a character's column in the joining state table.
func joiningColumnOf(r rune) int {
	switch t := joiningTypeOf(r); t {
	case joinT:
		return colTransparent
	case joinL:
		return colL
	case joinD, joinC:
		return colD
	case joinR:
		if slices.Contains(alaphGroup[:], r) {
			return colAlaph
		}
		if slices.Contains(dalathRishGroup[:], r) {
			return colDalathRish
		}
		return colR
	}
	return colU
}

// joiningStep is one entry of the table: the form the letter before now takes,
// if it changes; the form this letter takes; and the state after it.
type joiningStep struct{ prev, cur, next uint8 }

// joiningStates is HarfBuzz's arabic_state_table, row by state and column by
// joiningColumnOf. The states are what the letter before says about joining:
//
//	0  it does not join forward (or there is none)
//	1  it is a right-joining letter, or an isolated Alaph
//	2  it is a dual- or left-joining letter, isolated so far, and would join
//	3  it is a dual-joining letter, final so far, and would join
//	4  it is a final Alaph
//	5  it is an Alaph in its second or third final form
//	6  it is a Dalath or a Rish
//
// An Alaph is final (fin2) after a letter that does not join forward and fin3
// after a Dalath or a Rish; an Alaph after one that does is final and makes the
// letter before it medial (med2) where it would otherwise be initial.
var joiningStates = [7][6]joiningStep{
	//            U                          L                          R                          D                          Alaph                      Dalath-Rish
	/* 0 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formNone, formIsol, 1}, {formNone, formIsol, 2}, {formNone, formIsol, 1}, {formNone, formIsol, 6}},
	/* 1 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formNone, formIsol, 1}, {formNone, formIsol, 2}, {formNone, formFin2, 5}, {formNone, formIsol, 6}},
	/* 2 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formInit, formFina, 1}, {formInit, formFina, 3}, {formInit, formFina, 4}, {formInit, formFina, 6}},
	/* 3 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formMedi, formFina, 1}, {formMedi, formFina, 3}, {formMedi, formFina, 4}, {formMedi, formFina, 6}},
	/* 4 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formMed2, formIsol, 1}, {formMed2, formIsol, 2}, {formMed2, formFin2, 5}, {formMed2, formIsol, 6}},
	/* 5 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formIsol, formIsol, 1}, {formIsol, formIsol, 2}, {formIsol, formFin2, 5}, {formIsol, formIsol, 6}},
	/* 6 */ {{formNone, formNone, 0}, {formNone, formIsol, 2}, {formNone, formIsol, 1}, {formNone, formIsol, 2}, {formNone, formFin3, 5}, {formNone, formIsol, 6}},
}

// joinForms decides, for each character of a run, which form it takes: the
// joining scan of HarfBuzz's Arabic shaper (arabic_joining), which the
// universal engine runs too for the cursive scripts it sets.
//
// It walks the run with a state that says what the letter before can do, and
// each letter it meets settles its own form and, where it joins backwards, the
// form of the letter before it. A transparent character — a vowel sign, a
// join control's neighbour in the text — is stepped over in both directions,
// which is the whole reason it has a type of its own. The text either side of
// the run is read as HarfBuzz reads a buffer's context: the nearest letter
// before sets the state the run starts in, and the nearest after may change the
// form of the run's last letter.
//
// A join-causing character takes forms the way a dual-joining letter does.
// U+0640 TATWEEL is the one anybody writes: it is the stroke that stretches a
// word, it connects on both sides, and a font draws it differently at the start
// of a join than in the middle — Noto Sans Arabic carries uni0640.init and
// uni0640.medi for exactly that.
//
// It used to be a symmetric rule — join to the letter before if it joins
// forward, to the one after if it joins backward — which is the same answer for
// the four joining types and has no room for the Syriac Alaph, whose final form
// depends on what came before it and not on whether that joins. Syriac text
// was set with the ordinary final Alaph wherever its 'fin2', 'fin3' or 'med2'
// was meant.
func joinForms(runes, before, after []rune) []uint8 {
	forms := make([]uint8, len(runes))
	state := uint8(0)
	for i := len(before) - 1; i >= 0; i-- {
		if c := joiningColumnOf(before[i]); c != colTransparent {
			state = joiningStates[state][c].next
			break
		}
	}
	prev := -1
	for i, r := range runes {
		c := joiningColumnOf(r)
		if c == colTransparent {
			forms[i] = formNone
			continue
		}
		step := joiningStates[state][c]
		if step.prev != formNone && prev >= 0 {
			forms[prev] = step.prev
		}
		forms[i] = step.cur
		prev = i
		state = step.next
	}
	for _, r := range after {
		c := joiningColumnOf(r)
		if c == colTransparent {
			continue
		}
		if step := joiningStates[state][c]; step.prev != formNone && prev >= 0 {
			forms[prev] = step.prev
		}
		break
	}
	return forms
}

// markJoiningForms records, on each glyph, which positional form its character
// takes from its neighbours, as the bit of the feature that states the form.
//
// It only decides; nothing is substituted here. The two have to be separate
// because they happen at different moments and the run is a different shape at
// each. Deciding needs the characters, so it has to happen while the glyphs
// still correspond to them one for one — before any substitution. Substituting
// needs the glyphs the font's rules are written against, which in a real Arabic
// font are not the letters at all: Noto Sans Arabic composes 'ccmp' rules that
// split every letter into a skeleton and its dots, and states the four forms
// over the skeletons. A shaper that substitutes the forms first finds nothing to
// substitute, and every letter comes out in its isolated shape.
//
// The substitution is the plan's: each form is a stage of its own after 'ccmp'
// and 'locl', applied to the glyphs carrying its bit — see collectArabic. The
// lookups go through the lookup list rather than a flattened table of single
// substitutions, because a font may state a form as anything a lookup can be —
// a contextual rule, or a ligature that joins a letter to the one before it.
func markJoiningForms(buf []Glyph, runes, before, after []rune) {
	if len(runes) != len(buf) {
		// Nothing has been substituted yet where this is called, so this cannot
		// happen; the guard is here so that moving the call fails visibly rather
		// than assigning forms to the wrong glyphs.
		return
	}
	for i, form := range joinForms(runes, before, after) {
		if form != formNone {
			buf[i].mask |= formMasks[form]
		}
	}
}

// HasJoiningForms reports whether the font carries the positional forms a
// cursive script needs. A caller can use it to tell a face that can set Arabic
// from one that merely has the letters.
//
// The forms may be in its rules or in its character map: a face that maps the
// Arabic presentation forms and declares no joining forms has them drawn out
// of the map, as HarfBuzz draws them (see arabicfallback.go).
func (f *Face) HasJoiningForms() bool {
	l := f.layout
	for _, form := range arabicForms {
		if len(l.featureLookups[form.tag]) > 0 || len(l.single[form.tag]) > 0 {
			return true
		}
	}
	return f.composite() && arabicFallbackPlan(l) != nil && f.hasFallbackForms()
}

// cursiveScripts is the set of scripts whose letters join, indexed by script.
//
// Derived rather than named. A script is cursive when some character of it
// joins — has a joining type other than transparent or non-joining — and
// ArabicShaping.txt gives a type to every character of every such script, so
// the set is exactly what the generated joining table already says. Thirteen of
// them: Arabic, Syriac, Mandaic, Mongolian, N'Ko, Phags-pa, Manichaean, Psalter
// Pahlavi, Hanifi Rohingya, Sogdian, Adlam, Chorasmian and Old Uyghur. A script
// that gains cursive joining arrives with the next generated table rather than
// having to be remembered here.
var cursiveScripts = func() []bool {
	out := make([]bool, len(scriptOpenTypeTags))
	for _, jr := range joiningRanges {
		if jr.t == joinT || jr.t == joinU {
			continue
		}
		for r := jr.lo; r <= jr.hi; r++ {
			if s := scriptOf(r); decides(s) && int(s) < len(out) {
				out[s] = true
			}
		}
	}
	return out
}()

// hasDeclaredJoiningType reports whether ArabicShaping.txt gives a character a
// joining type of its own, rather than the type its category implies.
func hasDeclaredJoiningType(r rune) bool {
	i := sort.Search(len(joiningRanges), func(i int) bool { return joiningRanges[i].hi >= r })
	return i < len(joiningRanges) && r >= joiningRanges[i].lo
}

// InCursiveScript reports whether a character belongs to a script whose letters
// join, which is the question CSS Text §8.2's cursive tracking asks.
//
// It is the script and not a joining type, and the difference is the whole of
// why this is here: what §8.2 forbids is spacing *within cursive text*, not
// spacing between two letters that happen to touch. A hamza does not join and a
// Latin "b" does not join, and joiningTypeOf answers joinU for both — so the
// joining type cannot tell an Arabic letter from a Latin one, which is the only
// thing this has to do.
//
// It was membership of ArabicShaping.txt, which is close but is not the
// property. The file leaves out an Arabic script's digits, its punctuation and
// the whole of the Arabic Presentation Forms — a thousand and some characters
// of cursive text that letter-spacing was being inserted into — and it takes in
// a zero-width joiner, a narrow no-break space, the bidi isolates and the
// Kaithi number signs, none of which is text of a cursive script at all.
//
// Common and Inherited say nothing about what a character is written among, so
// a character of one of them counts only where Unicode gives it a joining type
// of its own — the tatweel, the Arabic number signs — and not where it is a
// general-purpose formatting or spacing character that happens to have one. A
// zero-width joiner is listed because it joins in *any* script, which is not a
// script of its own.
//
// A combining mark is the case that cannot be answered here at all. Unicode
// calls an Arabic fatha Inherited, because a mark takes the script of the
// letter it is written on, and no predicate over one character can know what
// that letter was. Every reader resolves it the same way instead: a mark does
// not decide, and the base it hangs off has already decided. See the cluster
// walk in paragraph/spacing.go.
func InCursiveScript(r rune) bool {
	if r < joiningRanges[0].lo {
		return false // every letter of an ASCII document, in one comparison
	}
	if s := scriptOf(r); decides(s) {
		return int(s) < len(cursiveScripts) && cursiveScripts[s]
	}
	return hasDeclaredJoiningType(r) && !isDefaultIgnorable(r) &&
		!charprop.Is(r, charprop.Zs)
}
