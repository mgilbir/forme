package shape

import (
	"fmt"
	"sort"

	"github.com/mgilbir/forme/font"
)

// Reading the OpenType layout tables: what the font says about how its glyphs
// combine, and turning that into the substitutions and positions a run of text
// needs.
//
// # What is here
//
//   - Positioning: every GPOS lookup type — single and pair adjustment,
//     cursive attachment, mark-to-base, mark-to-ligature, mark-to-mark and the
//     contextual rules — applied lookup by lookup in the order the font lists
//     them, so an accent sits over the letter it belongs to and a second
//     stacks on the first (position.go, contextpos.go); the legacy kern table
//     for fonts that predate GPOS (legacykern.go); and, for a face that
//     positions nothing itself, the marks placed by their combining classes
//     (fallback.go).
//   - Substitution: single (GSUB 1), multiple (GSUB 2), alternate (GSUB 3) and
//     ligature (GSUB 4). An alternate set is taken at its first entry, which is
//     the font's own preference and the only answer available to a lookup no
//     caller asked for.
//   - Contextual and chained-contextual substitution (GSUB 5 and 6), all six
//     formats, which is what makes 'calt' — and any rule that depends on
//     surroundings — do anything at all. See context.go. And reverse chaining
//     single substitution (GSUB 8), applied from the end of a run back to its
//     start, as the format requires. See stage.go.
//   - The plan: which model sets a run — chosen from its script and from the tag
//     the font's rules were read under — and the stages its features are applied
//     in, each stage's lookups in the font's lookup order and each once, with
//     'rvrn' first and the language system's required feature applied whatever
//     its tag. See plan.go.
//   - Cursive joining: the positional forms Arabic and its neighbours are
//     written in, chosen from Unicode joining types. See arabic.go.
//   - Cursive attachment (GPOS 3), which makes those forms' connecting strokes
//     actually meet — joining picks the shapes, this places them. See
//     position.go.
//   - Indic reordering, for the nine scripts that share the model: cutting a run
//     into syllables, finding each one's base consonant, and putting its glyphs
//     into the order they are drawn — the pre-base vowel sign before its
//     consonant, the reph over the end of the syllable — before applying the
//     features an Indic font declares for each part. See indic.go and
//     indicsyllable.go.
//   - Every feature the font declares that is not on by default, applied only
//     when a caller names it (ShapeGlyphsWith, or Features from a document):
//     'smcp', 'onum' and the rest, which change what the text says it is and so
//     wait to be asked for.
//   - FeatureVariations, the GSUB and GPOS table that gives a feature different
//     lookups at different points in a variable font's design space. A face may
//     state a feature's lookups only there — Noto Sans Oriya states its 'rclt'
//     that way — and reading it as stating none costs that feature entirely.
//   - Normalisation: the run put into whichever spelling this face draws best —
//     composed where the face has the composed character, decomposed where it
//     does not — and each cluster's marks put into canonical order, so that a
//     font's rules match text written either way. See normalize.go.
//   - GDEF glyph classes and the lookup flags that use them, so that a lookup
//     declaring it ignores marks does.
//   - The zero-width joiner and non-joiner: obeyed where they are written about,
//     stepped over by every rule that is not about them, and removed before
//     anything is positioned or drawn. See ignorable.go, which also says which
//     of Unicode's other default-ignorable characters are *not* handled.
//   - Reordering for every syllabic model this engine sets: Devanagari and its
//     eight relatives (indic.go), Khmer (khmer.go), Myanmar (myanmar.go) and
//     the Universal Shaping Engine (use.go), which covers Tibetan, Javanese,
//     Balinese, Buginese, Tai Tham, Cham, Sinhala and a long tail. plan.go
//     chooses between them, and each file says what within its own is left
//     out. And Thai and Lao's decomposition of SARA AM (thai.go).
//   - A variable font at any point in its design space. LoadInstance rewrites
//     the outlines for the coordinates asked for, and FeatureVariations is read
//     at those coordinates rather than at the default's — so a record whose
//     conditions cover the instance is applied, which is how a font states
//     different lookups for a weight.
//
// # What is not, and what each absence costs
//
//   - Of what HarfBuzz does for a font whose tables do not cover what a model
//     needs, the Windows-1256 Arabic fallback. The rest is done: placing the
//     marks of a face that positions none (fallback.go), drawing the Arabic
//     joining forms out of the character map (arabicfallback.go), and
//     composing Hebrew presentation forms for a face with no mark positioning
//     (hebrew.go). See plan.go.
//   - Choosing a language from the text. Which script a run is in is decidable
//     from its characters; which language it is in is not — "colour" and "color"
//     are the same letters — so the default language system is used unless the
//     caller says which language the run is in (Features.Language).
//
// # Script and language selection
//
// A font states its rules per script: a ScriptList names each script it covers,
// each script names its language systems, and each language system names the
// features that apply. Shaping resolves the run's script from Unicode (see
// scripts.go), selects the features that script and language name, and reads
// the tables from those alone — so a Greek run is not given a rule the font
// declares only for Arabic.
//
// A table with no ScriptList, or one that declares nothing for the run's
// script and no default either, selects nothing for the run, as HarfBuzz
// selects nothing from it. The unselected reading — every feature, whatever
// declares it — is kept as f.layout for the questions about what a face has
// at all (HasLigatures, HasKerning), which are not about a run.
//
// # Bounds
//
// A font is untrusted input. Every table here is offset-driven and
// self-referential, so each walk is bounded: the number of lookups, subtables,
// pairs, ligatures, variation records and conditions a font may declare are all
// capped, and a malformed offset truncates the walk rather than reaching outside
// the table.

// Layout bounds. They exist so that a crafted font cannot turn a few bytes of
// declaration into unbounded work.
//
// Each is above any real face — but "above any real face" is a claim about
// fonts nobody has looked at, and it was wrong once: 512 was the cap on a
// lookup list, and Noto Serif Tibetan declares 1190 lookups. Truncating that
// list is worse than truncating any other, because a lookup is named by *index*
// and a contextual rule reaching past the cut silently does nothing. A third of
// that font's Tibetan was set wrongly and nothing said so. See maxDeclaredList.
const (
	maxSubtables = 256
	maxPairs     = 1 << 18
	maxLigatures = 1 << 14
	maxScripts   = 256
	maxLangSys   = 256
	// maxDeclaredList bounds every counted list these tables hold — the
	// lookups, the features, the lookup indices a feature names, the feature
	// indices a language system names — and is the format's own maximum rather
	// than a guess at what a font might hold: each of those counts is a uint16,
	// so no valid font can exceed it and no valid font is ever truncated.
	//
	// It has to be all of them, because everything in these tables is named by
	// its index in one of these lists. Truncating any of them does not lose a
	// tail: it silently breaks every reference past the cut, and the rule
	// naming one does nothing at all.
	//
	// It is not what stops a crafted font. That is the walk itself, which needs
	// each entry's bytes present in the table and stops when they run out — so
	// the work a font can ask for is bounded by its own size, which is the
	// bound that means something. This is the backstop.
	maxDeclaredList = 0xFFFF
	// maxSubtableList bounds the subtables of one lookup, and is the format's
	// own maximum for the same reason.
	maxSubtableList = 0xFFFF
	// maxCoverageGlyphs bounds the glyphs one coverage table may name.
	//
	// A coverage table lists the glyphs a lookup applies to, so no valid one
	// names more glyphs than the font has — and a font has at most 65,536.
	// Format 2 states them as ranges, six bytes each, so six bytes can name the
	// whole space: the bound cannot come from the table's own size and has to
	// be this.
	maxCoverageGlyphs = 1 << 16
	// The FeatureVariations walk. A real face states a handful of records — Noto
	// Sans Oriya states one — and each names a few conditions and a few
	// substituted features.
	maxVariationRecords = 256
	maxConditions       = 64
	maxFeatureSubsts    = 256
)

// featureSet is the set of FeatureList indices a script and language select.
//
// A nil set means no selection was made — the table declares no scripts, or
// none that matched — and every feature is taken, which is what this package
// did before it read the ScriptList. An empty but non-nil set means the
// selection was made and chose nothing, which is a different thing and has to
// stay distinguishable from it.
type featureSet map[int]bool

// selects reports whether a feature at the given index in the FeatureList
// applies.
func (s featureSet) selects(index int) bool { return s == nil || s[index] }

// featureSubst is the lookup list a FeatureVariations record puts in place of
// the one a feature names for itself, keyed by FeatureList index.
//
// An entry present but empty is not the same as no entry: it says the record
// gives that feature *no* lookups here, which silences a feature the FeatureList
// declares. So every reader asks with the two-value form.
type featureSubst map[int][]int

// tableFeatures is what one layout table offers a run: the FeatureList indices
// the run's script and language selected, and the lookup lists FeatureVariations
// substitutes at the coordinates in force.
//
// The two travel together because every reader below needs both, and because
// taking one without the other is a mistake that produces plausible output — a
// feature applied with the wrong lookups, or a feature the run never selected.
type tableFeatures struct {
	sel    featureSet
	varied featureSubst
}

// readFeatureVariations reads the FeatureVariations table: the GSUB or GPOS
// table that gives a feature different lookups at different points in a variable
// font's design space.
//
// # Which coordinates are in force
//
// The ones the face was loaded at, which for Load is the default instance and
// for LoadInstance is the location it was asked for. They arrive normalized, and
// a nil set is the default instance — zero on every axis by construction, since
// fvar normalizes each axis's default to zero and avar's segment map is required
// to carry (0, 0) through.
//
// That construction is why nil and zero mean the same thing here and may. It is
// also why the axis index has to be read rather than ignored: at the default
// instance every coordinate is zero whichever axis a condition names, but at any
// other location the axes differ, and a condition on the width axis evaluated
// against the weight coordinate applies a rule for a font nobody asked for.
//
// A record stated for a part of the design space this face was not cut at is a
// rule for another weight, and is not applied.
//
// This is not only a variable-font concern. A static instance cut from a
// variable font may keep the table, and a face may state at the default instance
// — Noto Sans Oriya states its whole 'rclt' feature in a record that covers it —
// exactly the case that made this worth reading.
//
// The first record whose conditions hold is the one that applies, and the rest
// are not consulted: the specification makes the records an ordered search, not
// a set to merge.
func readFeatureVariations(t []byte, coords []float64) featureSubst {
	// Version 1.1 of the table header is what carries the offset; a 1.0 header
	// ends before it.
	if len(t) < 14 || font.Be16(t, 0) != 1 || font.Be16(t, 2) < 1 {
		return nil
	}
	off := int(font.Be32(t, 10))
	if off <= 0 || off+8 > len(t) {
		return nil
	}
	fv := t[off:]
	if font.Be16(fv, 0) != 1 {
		return nil
	}
	n := int(font.Be32(fv, 4))
	if n > maxVariationRecords {
		n = maxVariationRecords
	}
	for i := 0; i < n; i++ {
		rec := 8 + 8*i
		if rec+8 > len(fv) {
			break
		}
		if !conditionSetHolds(fv, int(font.Be32(fv, rec)), coords) {
			continue
		}
		return featureTableSubstitution(fv, int(font.Be32(fv, rec+4)))
	}
	return nil
}

// conditionSetHolds reports whether every condition of a set holds at the
// coordinates in force.
//
// A null offset is the empty set, which the specification says matches
// everywhere — a record that applies unconditionally. A set this cannot read
// whole does not hold: half a condition set is not a weaker condition set, it is
// no knowledge of what the record was for.
func conditionSetHolds(fv []byte, off int, coords []float64) bool {
	if off == 0 {
		return true
	}
	if off < 0 || off+2 > len(fv) {
		return false
	}
	cs := fv[off:]
	n := font.Be16(cs, 0)
	if n > maxConditions {
		return false
	}
	for i := 0; i < n; i++ {
		if 2+4*i+4 > len(cs) {
			return false
		}
		co := int(font.Be32(cs, 2+4*i))
		if co <= 0 || co+2 > len(cs) {
			return false
		}
		if !conditionHolds(cs[co:], coords) {
			return false
		}
	}
	return true
}

// conditionHolds reports whether one condition holds at the coordinates in
// force.
//
// Only format 1, an axis range, is read. The formats OpenType added later state
// a condition on a variable value, or combine other conditions with and, or and
// not; a condition this cannot evaluate is treated as not holding, because a
// record applied on a guess is a record applied at the wrong weight.
//
// An axis index the font does not have reads as zero, which the specification
// requires: a condition naming it holds exactly when its range spans the default
// instance.
func conditionHolds(c []byte, coords []float64) bool {
	if len(c) < 8 || font.Be16(c, 0) != 1 {
		return false
	}
	axis := font.Be16(c, 2)
	lo := f2Dot14At(c, 4)
	hi := f2Dot14At(c, 6)
	v := 0.0
	if axis < len(coords) {
		v = coords[axis]
	}
	return lo <= v && v <= hi
}

// featureTableSubstitution reads the lookup lists a matching record substitutes,
// by FeatureList index.
func featureTableSubstitution(fv []byte, off int) featureSubst {
	if off <= 0 || off+6 > len(fv) {
		return nil
	}
	ts := fv[off:]
	if font.Be16(ts, 0) != 1 {
		return nil
	}
	n := font.Be16(ts, 4)
	if n > maxFeatureSubsts {
		n = maxFeatureSubsts
	}
	out := make(featureSubst, n)
	for i := 0; i < n; i++ {
		rec := 6 + 6*i
		if rec+6 > len(ts) {
			break
		}
		index := font.Be16(ts, rec)
		ao := int(font.Be32(ts, rec+2))
		if ao <= 0 || ao+4 > len(ts) {
			continue
		}
		alt := ts[ao:]
		m := font.Be16(alt, 2)
		if m > maxDeclaredList {
			m = maxDeclaredList
		}
		lookups := make([]int, 0, min(m, max(0, (len(alt)-4)/2)))
		for j := 0; j < m; j++ {
			if 4+2*j+2 > len(alt) {
				break
			}
			lookups = append(lookups, font.Be16(alt, 4+2*j))
		}
		out[index] = lookups
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// featureLookupList is the lookup indices one FeatureList entry names — its own,
// unless a FeatureVariations record put another list in its place.
func featureLookupList(list []byte, index int, varied featureSubst) []int {
	if lookups, ok := varied[index]; ok {
		return lookups
	}
	rec := 2 + 6*index
	if rec+6 > len(list) {
		return nil
	}
	off := font.Be16(list, rec+4)
	if off <= 0 || off+4 > len(list) {
		return nil
	}
	feature := list[off:]
	n := font.Be16(feature, 2)
	if n > maxDeclaredList {
		n = maxDeclaredList
	}
	out := make([]int, 0, min(n, max(0, (len(feature)-4)/2)))
	for j := 0; j < n; j++ {
		if 4+2*j+2 > len(feature) {
			break
		}
		out = append(out, font.Be16(feature, 4+2*j))
	}
	return out
}

// layout holds what was read out of a font's layout tables.
type layout struct {
	// glyphClass is GDEF's classification of each glyph: 1 base, 2 ligature,
	// 3 mark, 4 component. A glyph GDEF does not name is class 0, unknown.
	glyphClass classTable
	// covWork is what is left of this layout's allowance for turning its table
	// into the flat form the rest of this file keeps, and is spent only while
	// the layout is being read. See spend.
	//
	// It is written only by the reader that builds the layout, before the
	// layout is shared, so it is not state two documents can reach.
	covWork int
	// limits says which bounds reading this layout ran into, in words a caller
	// can pass on: each is something the font states that was not read. See
	// Face.LayoutLimits.
	limits []string
	// workSpent records that the allowance above ran out, so that the reader
	// can say so once rather than once per refusal.
	workSpent bool
	// markAttach is GDEF's mark attachment class per glyph, used by the
	// MarkAttachmentType field of a lookup flag.
	markAttach classTable
	// halfWidth holds 'halt' by glyph, and is applied to nothing: it is the
	// font's statement of what a full-width punctuation's trimmed form is, for
	// a caller that has a character to trim to ask about. See halfwidth.go.
	halfWidth map[int]singleAdjust
	// markSets are GDEF's mark glyph sets, which a lookup names to narrow what
	// it looks at to the marks in one of them. Each is the set's coverage table,
	// asked whether it covers a glyph where a lookup needs to know, rather than
	// expanded into a set of every glyph it names.
	markSets []coverageTable
	// kern is the pair-positioning lookups, in the order the font lists them,
	// read flat for the two things that ask about a pair outside a positioning
	// pass: the pair across a run boundary (boundarykern.go) and whether a face
	// kerns at all. The pass itself applies the lookups as they are, in
	// order, among every other positioning lookup — see position.go.
	//
	// One entry per lookup rather than one table for all of them, because three
	// things a lookup states about itself are lost by merging. Its *flags* say
	// which glyphs it steps over, and a font states some kerning that ignores
	// marks beside some that does not — merged, the mark-ignoring one silences
	// every other for marks. Its *subtables* are alternatives in which the first
	// match wins, so a pair named by two of them takes the earlier value —
	// merged into one map the later one overwrites it. And lookups *accumulate*,
	// which one value per pair cannot express.
	//
	// Noto Sans states both of the first two in one lookup: an explicit pair
	// list giving be+TE -20 and a class table giving the same pair -40, of which
	// only the first should apply.
	kern []kernLookup
	// kernPairs is how many pairs the legacy kern table's lookups hold together,
	// against maxPairs — the only pairs still listed; see kernLookup — and
	// pairsCapped records that a pair was refused for want of room.
	kernPairs   int
	pairsCapped bool
	// ligatures maps a first glyph to the substitutions that may start with it,
	// longest first so that a greedy match prefers ffi over ff.
	//
	// It answers HasLigatures and nothing else: shaping applies 'liga'
	// through the lookup list below, which honours the lookup's flags — so an
	// accent written between two letters does not stop them ligating. The span
	// path that set ligatures from this table is gone.
	ligatures map[int][]ligature
	// single holds one-for-one substitutions per feature tag: small capitals,
	// oldstyle figures and the rest. They are read for every feature the font
	// declares, and applied only when a caller asks for that feature by name.
	single map[string]map[int]int
	// gsub is the substitution lookup list kept whole, indexed as the font
	// indexes it. A contextual rule names a lookup by its index here and has it
	// applied at a position, so these cannot be flattened the way the tables
	// above are — see context.go.
	gsub []rawLookup
	// gpos is the positioning lookup list kept whole and addressable by index.
	// A positioning pass applies its lookups from here, each over the whole
	// run in the order the font lists them, and a contextual rule names one by
	// its index here.
	gpos []rawLookup
	// gposFeatures maps each positioning feature the run's script and language
	// selected to the lookups it names, as featureLookups does for GSUB; a
	// plan chooses from it which of them apply. gposRequired is the language
	// system's required positioning feature, which applies whatever it is
	// called: its tag and its lookups. See plan.compile.
	gposFeatures    map[string][]int
	gposRequiredTag string
	gposRequired    []int
	// legacyKern is the kern table, read for the model that applies it when
	// GPOS has no kerning of its own to offer. See legacykern.go.
	legacyKern legacyKern
	// featureLookups maps a feature tag to the lookup indices it names, which is
	// how a feature is turned into work to do.
	featureLookups map[string][]int
	// requiredTag and requiredLookups are the language system's required
	// feature, which a plan applies whether or not its model asks for the tag.
	// Empty for a selection with none. See plan.compile.
	requiredTag     string
	requiredLookups []int
	// plans are the shaping plans built from this layout, so that a run does not
	// collect and sort its stages again. See planFor.
	plans *planCache
}

// Lookup flags (ISO/IEC 14496-22, LookupFlag). The high byte is a mark
// attachment class rather than a flag, and is handled separately.
const (
	// flagRightToLeft relates only to cursive attachment (GPOS 3): it says the
	// *last* glyph of a joined run stays on the baseline and the earlier ones
	// move to meet it, rather than the first.
	flagRightToLeft         = 0x0001
	flagIgnoreBaseGlyphs    = 0x0002
	flagIgnoreLigatures     = 0x0004
	flagIgnoreMarks         = 0x0008
	flagUseMarkFilteringSet = 0x0010
	flagMarkAttachType      = 0xFF00
)

// Glyph classes as GDEF defines them.
const (
	classBase     = 1
	classLigature = 2
	classMark     = 3
	// classComponent is named for completeness: GDEF defines it, and a reader
	// of this list should see the whole set rather than wonder what 4 means.
	classComponent = 4
	// classUnclassified is not one of GDEF's. It is a glyph a shaper put in
	// — the dotted circle a broken cluster is shown against — which HarfBuzz
	// gives no class at all, whatever GDEF says of the glyph, until a
	// substitution touches it: no lookup flag steps over it and it is not a
	// mark. Nothing compares a class with it; it is here so that a glyph
	// carrying it is none of the others.
	classUnclassified = 5
)

// ignores reports whether a lookup with the given flags skips a glyph.
//
// This is the correctness fix these flags exist for. A kerning lookup almost
// always declares that it ignores marks, because the pair it means to adjust is
// two base letters — and an accent written between them must not break it.
// Reading the pairs and not the flag kerns "A" and "V" but not "Ä" and "V",
// which is a difference a reader sees.
func (l *layout) ignores(flags int, g Glyph) bool {
	return l.ignoresIn(flags, -1, g)
}

// mergedFlags is what a flag word means once it has been merged across the
// lookups of a feature, which is what the flat positioning tables do.
//
// The mark filtering set is dropped, and has to be: the set is named per
// lookup, and a merged word has no room to say which. Keeping the bit would
// read as "filter by a set nobody named", and ignoresIn answers that by
// stepping over every mark — so a font with one such lookup would lose the mark
// positioning of every other. Dropping it costs the narrowing that lookup asked
// for; keeping it costs the whole feature.
func mergedFlags(flags int) int { return flags &^ flagUseMarkFilteringSet }

// ignoresIn is ignores for a lookup that also names a mark glyph set. A set
// index of -1 means it names none, which is every lookup but the few that do.
func (l *layout) ignoresIn(flags, markSet int, g Glyph) bool {
	class := l.classOf(g)
	// A lookup that names a set sees the marks in it and no others. It is
	// checked before the flags below because it is the narrower statement: a
	// lookup naming a set is saying which marks it means, not which kinds.
	if flags&flagUseMarkFilteringSet != 0 && class == classMark {
		if markSet < 0 || markSet >= len(l.markSets) || !l.markSets[markSet].covers(g.GID) {
			return true
		}
	}
	switch {
	case flags&flagIgnoreMarks != 0 && class == classMark:
		return true
	case flags&flagIgnoreBaseGlyphs != 0 && class == classBase:
		return true
	case flags&flagIgnoreLigatures != 0 && class == classLigature:
		return true
	}
	// A mark attachment class in the high byte narrows the rule to marks of one
	// class; marks of any other are skipped.
	if attach := (flags & flagMarkAttachType) >> 8; attach != 0 && class == classMark {
		return l.markAttach.of(g.GID) != attach
	}
	return false
}

// classOf is what a lookup flag is read against: GDEF's classification of the
// glyph, or — for a font that declares none — what the character it came from
// says.
//
// The fallback is not a nicety. Without it, a font with no GDEF glyph class
// table has no glyph that any flag recognises, so IgnoreMarks ignores nothing:
// "Ä" and "V" go unkerned where "A" and "V" kern, a ligature written over an
// accent never forms, and a lookup that was written to step over marks steps
// over nothing. Plenty of fonts declare no GDEF, and the character is the best
// authority available when the font is silent — it is, after all, where the
// font's own classification came from.
//
// GDEF wins wherever it exists, including for a glyph it does not list: a font
// that classified its glyphs and left this one out has said something about it.
func (l *layout) classOf(g Glyph) int {
	if g.class == classUnclassified && !(g.substituted && l.glyphClass.named) {
		return classUnclassified
	}
	if l.glyphClass.named {
		return l.glyphClass.of(g.GID)
	}
	return g.class
}

// classOfRune is GDEF's classification as the character itself implies it: a
// non-spacing mark is a mark and everything else is a base. Nothing implies
// "ligature" — that is a fact about a glyph, and is set where one is made.
//
// Non-spacing (Mn), not every combining mark, and never a default-ignorable
// one: that is HarfBuzz's hb_synthesize_glyph_classes, which is what a font
// with no GDEF is shaped by everywhere it is tested. A spacing mark (Mc) takes
// room of its own — the Sinhala anusvara is drawn after its letter, not over
// it — and read as a mark it lost its advance to mark zeroing and was drawn
// back over the letter; an enclosing mark (Me) is drawn around what it
// encloses and is spaced as a base. A default-ignorable mark — Mongolian's
// variation selectors, the combining grapheme joiner — is not in the way of a
// lookup either way, and read as a mark it is stepped over by one that skips
// marks, which Mongolian fonts with no GDEF are written not to expect.
func classOfRune(r rune) int {
	if isNonSpacingMark(r) && !isDefaultIgnorable(r) {
		return classMark
	}
	return classBase
}

// readGDEF reads the glyph classification, which is what makes a lookup flag
// mean anything: without it there is no way to know which glyphs are marks.
func (l *layout) readGDEF(gdef []byte) {
	if len(gdef) < 12 {
		return
	}
	if off := font.Be16(gdef, 4); off > 0 && off < len(gdef) {
		l.glyphClass = readClassTable(gdef, off)
	}
	l.readMarkGlyphSets(gdef)
	if off := font.Be16(gdef, 10); off > 0 && off < len(gdef) {
		l.markAttach = readClassTable(gdef, off)
	}
}

// ligature is one substitution: a run of glyphs replaced by a single one.
type ligature struct {
	components []int // the glyphs after the first
	glyph      int   // what they become, together with the first
}

// readPositioning parses everything GDEF, GPOS and the legacy kern table say,
// taking the GPOS features the given selection admits.
//
// It is separate from the substitution half because the two are cached
// separately, and because it is the expensive one: a large face states tens of
// thousands of kern pairs, and it commonly states the same ones for every
// script it covers while stating *different* substitutions for each. Reading it
// once per script would multiply the largest table in the font by the number of
// scripts a document sets, to no end.
//
// It never fails: a table that cannot be understood contributes nothing,
// because text set without kerning is correct text set plainly, while text set
// from a misread table is wrong.
func readPositioning(tables map[string][]byte, sel featureSet, required int, coords []float64) *layout {
	allowance := coverageBudget(tables["GPOS"], tables["GDEF"], tables["kern"])
	l := &layout{covWork: allowance}
	l.readGDEF(tables["GDEF"])
	if gpos := tables["GPOS"]; len(gpos) >= 10 {
		varied := readFeatureVariations(gpos, coords)
		feats := tableFeatures{sel: sel, varied: varied}
		// The feature and lookup lists are read once here and asked about per
		// tag below, rather than walked again for every tag — see featureIndex.
		idx := indexFeatures(gpos, feats)
		l.readGPOSPairs(gpos, idx)
		l.readHalfWidth(gpos, idx)
		// The lookups themselves, kept whole: a positioning pass applies them
		// in order at each glyph, as the font states them, rather than from
		// tables flattened out of them at load. See position.go.
		l.gpos = gposLookups(gpos)
		l.gposFeatures = idx.lookupIndices()
		// A feature the language system declares with no lookups is still
		// declared, and whether the font offers 'kern' at all is a question a
		// plan asks — see plan.gposKern.
		for _, tag := range idx.tags {
			if _, ok := l.gposFeatures[tag]; !ok {
				l.gposFeatures[tag] = nil
			}
		}
		l.readRequiredPositioning(gpos, required, varied)
	}
	if len(l.kern) == 0 {
		// Only as a fallback: a font with both should be read through GPOS,
		// which is the one a modern shaper honours. This is the flat reading
		// the boundary pair asks; the pass reads the table as legacyKern.
		l.readKernTable(tables["kern"])
	}
	l.legacyKern = readLegacyKern(tables["kern"])
	l.noteLimits("GPOS, GDEF and kern", allowance)
	return l
}

// readRequiredPositioning records a language system's required positioning
// feature, as readRequired does for substitution.
func (l *layout) readRequiredPositioning(gpos []byte, index int, varied featureSubst) {
	if index == noRequiredFeature || len(gpos) < 10 {
		return
	}
	off := font.Be16(gpos, 6)
	if off <= 0 || off+2 > len(gpos) {
		return
	}
	list := gpos[off:]
	rec := 2 + 6*index
	if index < 0 || index >= font.Be16(list, 0) || rec+6 > len(list) {
		return
	}
	l.gposRequiredTag = string(list[rec : rec+4])
	l.gposRequired = featureLookupList(list, index, varied)
}

// readLayout reads the substitution tables on top of an already-read
// positioning half, taking the GSUB features the given selection admits. The
// selection is the FeatureList indices the run's script and language chose; a
// nil one takes every feature, which is the face's own unselected reading.
//
// The positioning half is copied whole and then the substitution fields are
// reset, rather than the other way about, so that a positioning field added to
// layout later is carried across without this having to be remembered. The maps
// it copies are shared with every other layout built on the same half, and are
// never written to once read.
func readLayout(tables map[string][]byte, gsubSel featureSet, pos *layout, coords []float64) *layout {
	l := new(layout)
	*l = *pos
	// Its own allowance: the positioning half spent one on its own tables, and
	// this reads a different table. Copying what was left would make how much
	// of GSUB is read depend on how large GPOS happened to be.
	allowance := coverageBudget(tables["GSUB"])
	l.covWork = allowance
	l.workSpent, l.pairsCapped = false, false
	// The positioning half's limits are this layout's too, and are copied
	// rather than shared: the half is shared with every layout built on it.
	l.limits = append([]string(nil), pos.limits...)
	l.ligatures = map[int][]ligature{}
	l.single = map[string]map[int]int{}
	l.gsub = nil
	l.featureLookups = nil
	l.requiredTag, l.requiredLookups = "", nil
	l.plans = &planCache{}
	if gsub := tables["GSUB"]; len(gsub) >= 10 {
		feats := tableFeatures{sel: gsubSel, varied: readFeatureVariations(gsub, coords)}
		idx := indexFeatures(gsub, feats)
		l.readGSUBLigatures(gsub, idx)
		l.readSingleSubstitutions(gsub, idx)
		l.gsub = gsubLookups(gsub)
		l.featureLookups = idx.lookupIndices()
		// A feature declared with no lookups is declared all the same, as it
		// is for GPOS above: whether the font states a joining form at all is
		// what decides HarfBuzz's Arabic fallback (see arabicfallback.go).
		for _, tag := range idx.tags {
			if _, ok := l.featureLookups[tag]; !ok {
				l.featureLookups[tag] = nil
			}
		}
	}
	l.noteLimits("GSUB", allowance)
	return l
}

// readRequired records a language system's required feature: its tag, and the
// lookups it names at the coordinates in force.
func (l *layout) readRequired(gsub []byte, index int, coords []float64) {
	if index == noRequiredFeature || len(gsub) < 10 {
		return
	}
	off := font.Be16(gsub, 6)
	if off <= 0 || off+2 > len(gsub) {
		return
	}
	list := gsub[off:]
	rec := 2 + 6*index
	if index < 0 || index >= font.Be16(list, 0) || rec+6 > len(list) {
		return
	}
	l.requiredTag = string(list[rec : rec+4])
	l.requiredLookups = featureLookupList(list, index, readFeatureVariations(gsub, coords))
}

// gsubLookups reads the substitution lookup list whole: each lookup's type, its
// flags and its subtable bytes, at the index the font gives it.
//
// This is the list a contextual rule indexes into. It duplicates what the
// flattened readers above take from the same bytes, which is deliberate: those
// serve the common path cheaply, and a lookup that may be invoked from inside
// another has to survive as something applicable rather than as a map entry.
// gposLookups is gsubLookups for the positioning table, whose extension lookup
// type is 9 rather than 7.
func gposLookups(gpos []byte) []rawLookup { return lookupList(gpos, 9) }

func gsubLookups(gsub []byte) []rawLookup { return lookupList(gsub, 7) }

func lookupList(gsub []byte, extension int) []rawLookup {
	off := font.Be16(gsub, 8)
	if off <= 0 || off+2 > len(gsub) {
		return nil
	}
	list := gsub[off:]
	n := font.Be16(list, 0)
	if n > maxDeclaredList {
		n = maxDeclaredList
	}
	// The capacity is what the table could actually hold — two bytes of offset
	// each — rather than what it claims to. A font declaring sixty thousand
	// lookups in twenty bytes gets one allocation of twenty bytes' worth, and
	// the loop below stops when the offsets run out.
	out := make([]rawLookup, 0, min(n, max(0, (len(list)-2)/2)))
	// One budget for every subtable the whole list may name — see subtables.
	budget := subtableBudget(gsub)
	for i := 0; i < n; i++ {
		if 2+2*i+2 > len(list) {
			break
		}
		lo := font.Be16(list, 2+2*i)
		if lo <= 0 || lo >= len(list) {
			// Keep the slot: a rule names a lookup by index, so the indices of
			// the ones that follow must not shift.
			out = append(out, rawLookup{markSet: -1})
			continue
		}
		kind, flags, markSet, subs := subtables(list[lo:], extension, &budget)
		out = append(out, rawLookup{kind: kind, flags: flags, markSet: markSet, subs: subs})
	}
	return out
}

// featureIndex is one layout table's FeatureList and LookupList, read once per
// table read and then asked about tag after tag.
//
// It exists because the questions were answered by walking both lists again
// for every tag. The single-substitution reader asks about every distinct tag
// the font declares, so a table of n features naming n lookups cost n walks of
// n entries — sixty-four kilobytes of GSUB took 1.7 seconds and quadrupled per
// doubling, on the Load path and again per script a document sets.
type featureIndex struct {
	// list is the FeatureList, and varied what FeatureVariations puts in place
	// of a feature's own lookups.
	list   []byte
	varied featureSubst
	// lookups is each lookup's bytes by its index in the LookupList, nil where
	// its offset is unusable. It is nil as a whole where the LookupList itself
	// is, which is what a reader that resolves lookups asks about separately.
	lookups [][]byte
	hasList bool
	// tags is each tag the selection admits, once and in list order; byTag is
	// the FeatureList indices each of them is declared at.
	tags  []string
	byTag map[string][]int
}

// indexFeatures reads a table's FeatureList and LookupList, keeping the
// features the selection admits.
func indexFeatures(t []byte, feats tableFeatures) *featureIndex {
	x := &featureIndex{varied: feats.varied, byTag: map[string][]int{}}
	if off := font.Be16(t, 6); off > 0 && off+2 <= len(t) {
		x.list = t[off:]
		n := min(font.Be16(x.list, 0), maxDeclaredList)
		for i := 0; i < n; i++ {
			rec := 2 + 6*i
			if rec+6 > len(x.list) {
				break
			}
			if !feats.sel.selects(i) {
				continue
			}
			tag := string(x.list[rec : rec+4])
			if _, seen := x.byTag[tag]; !seen {
				x.tags = append(x.tags, tag)
			}
			x.byTag[tag] = append(x.byTag[tag], i)
		}
	}
	// The lookup list, so a feature's indices can be resolved.
	//
	// The bound is the format's own and not a guess, for the reason
	// maxDeclaredList gives: a lookup is named by index, and truncating the list
	// does not lose its tail — it silently breaks every reference into it. That
	// was fixed once, in the reader beside this one, and this reader kept the
	// old cap: a font's kerning, its mark and cursive attachment, the ligatures
	// HasLigatures reports and every single substitution all come through here,
	// and any of them past lookup 512 did nothing at all with nothing said.
	if off := font.Be16(t, 8); off > 0 && off+2 <= len(t) && x.list != nil {
		lookupList := t[off:]
		x.hasList = true
		n := min(font.Be16(lookupList, 0), maxDeclaredList)
		// The capacity is what the table could hold — two bytes of offset each
		// — rather than what it claims to, so a declared count with no data
		// behind it costs nothing. The walk below stops when the offsets run
		// out.
		x.lookups = make([][]byte, 0, min(n, max(0, (len(lookupList)-2)/2)))
		for i := 0; i < n; i++ {
			if 2+2*i+2 > len(lookupList) {
				break
			}
			lo := font.Be16(lookupList, 2+2*i)
			if lo <= 0 || lo >= len(lookupList) {
				x.lookups = append(x.lookups, nil)
				continue
			}
			x.lookups = append(x.lookups, lookupList[lo:])
		}
	}
	return x
}

// lookupsFor returns the lookup-table byte slices reachable from every feature
// with the given tag that the selection admits, and where each sits in the
// font's lookup list — which is the order the font means them to apply in,
// and what says whether two features have named the same one.
//
// A tag may appear in the FeatureList many times — a face with a dozen 'locl'
// features is ordinary, one per language it corrects letterforms for — and it
// is the selection, not the tag, that says which of them this run gets.
func (x *featureIndex) lookupsFor(tag string) ([][]byte, []int) {
	if !x.hasList {
		return nil, nil
	}
	var out [][]byte
	var indices []int
	for _, i := range x.byTag[tag] {
		for _, idx := range featureLookupList(x.list, i, x.varied) {
			if idx >= 0 && idx < len(x.lookups) && x.lookups[idx] != nil {
				out = append(out, x.lookups[idx])
				indices = append(indices, idx)
			}
		}
	}
	return out, indices
}

// lookupIndices maps each feature tag to the lookup indices it names, merging
// every feature the selection admits — which for a font that declares its
// scripts is every one the run's script and language chose, and for one that
// does not is every feature in the table. Each index appears once per tag.
func (x *featureIndex) lookupIndices() map[string][]int {
	out := map[string][]int{}
	for _, tag := range x.tags {
		// The duplicates are tracked in a set rather than by scanning what has
		// been kept. Several feature records may carry the same tag and name
		// overlapping lookups, so the scan is inside two loops — and a font with
		// a few thousand features naming a few thousand lookups turns that into
		// millions of comparisons. It was three quarters of the time spent
		// reading one that the fuzzer produced.
		kept := map[int]bool{}
		for _, i := range x.byTag[tag] {
			for _, idx := range featureLookupList(x.list, i, x.varied) {
				if kept[idx] {
					continue
				}
				kept[idx] = true
				out[tag] = append(out[tag], idx)
			}
		}
	}
	return out
}

// sortInts sorts a list of lookup indices.
//
// It was an insertion sort, on the grounds that there are a handful of them.
// There are a handful in a real font; a feature may name every one of 65,535,
// in descending order, and an insertion sort of that is two billion swaps from
// a hundred and thirty kilobytes of FeatureList.
func sortInts(a []int) { sort.Ints(a) }

// subtables returns a lookup's subtables, resolving the extension indirection
// that lets a large font place them beyond the 16-bit offset range.
// subtableBudget is how many subtables a reader may take from one table.
//
// It is proportional to the table's size because that is what a well-formed
// font's subtables cost: each is named by two bytes of offset, and the lookups
// naming them do not overlap. A crafted font's do, which is the whole reason
// this exists — see subtables.
//
// The constant on the end is so that a small table with one dense lookup is not
// refused: a font may reasonably state seven hundred subtables in a lookup, and
// Noto Serif Tibetan does.
func subtableBudget(table []byte) int { return len(table)/2 + maxSubtableList }

func subtables(lookup []byte, extensionType int, budget *int) (kind, flags, markSet int, out [][]byte) {
	if len(lookup) < 6 {
		return 0, 0, -1, nil
	}
	kind = font.Be16(lookup, 0)
	flags = font.Be16(lookup, 2)
	markSet = -1
	// The subtables of one lookup are alternatives tried in order, and the first
	// that applies wins — so a font states a general rule that matches and does
	// nothing, then the particular ones after it. Keeping only the first few
	// hundred therefore does not lose the rare cases at the end: it loses
	// whichever cases the font happened to list late, while the blocking rules
	// at the front still match. Noto Serif Tibetan states one lookup in 738
	// subtables.
	//
	// The bound is the format's own, for the reason maxDeclaredList is: the count
	// is a uint16, so no valid font is truncated, and what stops a crafted one
	// is that each subtable needs two bytes of offset present in the lookup.
	// The declared count, kept apart from the clamped one: the mark filtering
	// set below is written *after* the offsets the lookup declares, so where it
	// sits is decided by that number and not by how many of them are read.
	// Reading it at the clamped position took two bytes of an offset instead —
	// a mark glyph set index out of the middle of the table.
	declared := font.Be16(lookup, 4)
	count := declared
	if count > maxSubtableList {
		count = maxSubtableList
	}
	// And no more than the table has left to give.
	//
	// The per-lookup bound above is not enough on its own, and the reason is
	// worth stating: a lookup is a slice that runs to the *end* of the table,
	// not to the end of itself, because nothing says where one stops. So every
	// lookup in a large font appears to have room for tens of thousands of
	// subtables, and a crafted font declaring the maximum in each of the maximum
	// number of lookups asks for their product — which is how a 533 KB file came
	// to take half a minute to read. The budget is shared across the table, so
	// the work is bounded by its size rather than by its size squared.
	if budget != nil {
		if count > *budget {
			count = *budget
		}
		*budget -= count
	}
	// Whether the *lookup* is an extension has to be decided once. Reading it
	// from kind inside the loop stops unwrapping after the first subtable, since
	// unwrapping is what replaces kind with the real type.
	extension := kind == extensionType
	for i := 0; i < count; i++ {
		if 6+2*i+2 > len(lookup) {
			break
		}
		off := font.Be16(lookup, 6+2*i)
		if off <= 0 || off >= len(lookup) {
			continue
		}
		sub := lookup[off:]
		if extension {
			// Extension: format(2), real lookup type(2), 32-bit offset.
			if len(sub) < 8 {
				continue
			}
			kind = font.Be16(sub, 2)
			delta := int(font.Be32(sub, 4))
			if delta <= 0 || delta >= len(sub) {
				continue
			}
			sub = sub[delta:]
		}
		out = append(out, sub)
	}
	// A lookup that filters by a mark glyph set names it after the subtable
	// offsets, which is why this is read last: where the number sits depends on
	// how many subtables there are.
	if flags&flagUseMarkFilteringSet != 0 {
		if at := 6 + 2*declared; at+2 <= len(lookup) {
			markSet = font.Be16(lookup, at)
		}
	}
	return kind, flags, markSet, out
}

// pairFeatures are the features whose pair adjustments this reads.
//
// 'kern' is the one everybody knows. 'dist' is the other, and leaving it out is
// not a small omission: it is the feature the complex scripts state their
// spacing under, and for a Devanagari run it is often the *only* one — Noto
// Sans declares no 'kern' at all under deva or dev2, so a reader that asked only
// for 'kern' got a layout with zero pairs in it and set every conjunct at its
// nominal width. Measured against HarfBuzz, that was every Devanagari cluster
// in the sample, out by up to 73 units of the em.
//
// Both are on for every script rather than for the complex ones alone, which is
// what the feature registry says and what HarfBuzz does: 'dist' is one of its
// global horizontal features, beside 'kern' and 'curs'.
var pairFeatures = [...]string{"kern", "dist"}

// kernLookup is one pair-positioning lookup: the flags it states its pairs
// under, and the pairs themselves.
//
// # The pairs are looked up, not listed
//
// A GPOS lookup's pairs stay in its subtables and are searched for when a pair
// is met, as HarfBuzz does it: a class-pair subtable states in a few hundred
// bytes that every glyph of one class kerns against every glyph of another,
// and listing what it states means one map entry per glyph pair. That is how
// this read them, and it went wrong three ways at once.
//
//   - Cost. The listing was built at load by walking every covered glyph
//     against every second class, re-decoding the same row of the class
//     matrix for each glyph that shared it: eight kilobytes of GPOS took seven
//     seconds to load, and a lookup of four hundred offsets to one subtable
//     read it four hundred times.
//   - Memory. A real class-kerned face lists a quarter of a million pairs, at
//     some forty bytes each, per script selection a document reads.
//   - Correctness. The listing was capped at maxPairs, and 176 of the 3,824
//     faces in the Google Fonts checkout reach the cap: their kerning past it
//     was silently dropped, and which pairs survived depended on map order.
//
// What is kept instead is the subtables and, per glyph that can begin a pair,
// which of them to search (byFirst) — one entry per covered glyph, found by
// walking the coverage once — so a pair costs a search in one or two
// subtables rather than in every one.
//
// The legacy kern table is still a list, because it is one: it states each pair
// explicitly, so its pairs cost what its bytes do.
type kernLookup struct {
	flags int
	// pairs is an explicit list of pairs, which is what the legacy kern table
	// states; a GPOS lookup leaves it nil.
	pairs map[[2]int]pairAdjust
	// subs are a GPOS lookup's pair subtables, in order, and byFirst is where
	// in them each glyph that can begin a pair is found.
	subs    [][]byte
	byFirst map[int][]pairStart
}

// pairStart is one subtable's statement that a glyph may begin a pair: which
// subtable, and where in it the glyph's pairs are — its coverage index in the
// explicit form, its first class in the class form.
type pairStart struct {
	sub, at int
}

// pair is what the lookup states about a pair of glyphs, if anything.
//
// Within a lookup the subtables are alternatives, and the first that names the
// pair is the one that counts: a zero adjustment in an explicit list is still a
// match, and it is how a font states that a class rule later in the same lookup
// does not apply to this pair.
func (kl *kernLookup) pair(first, second int) (pairAdjust, bool) {
	if kl.pairs != nil {
		adj, ok := kl.pairs[[2]int{first, second}]
		return adj, ok
	}
	for _, ps := range kl.byFirst[first] {
		if adj, ok := pairIn(kl.subs[ps.sub], ps.at, second); ok {
			return adj, true
		}
	}
	return pairAdjust{}, false
}

// pairIn searches one pair subtable for the pair beginning at a first glyph
// whose place in the subtable is at.
func pairIn(sub []byte, at, second int) (pairAdjust, bool) {
	fmt1, fmt2 := font.Be16(sub, 4), font.Be16(sub, 6)
	switch font.Be16(sub, 0) {
	case 1:
		// The pair set of the first glyph, searched for the second: the
		// records are in second-glyph order, which the format requires and
		// HarfBuzz relies on in the same way.
		set := sub[font.Be16(sub, 10+2*at):]
		size := 2 + valueSize(fmt1) + valueSize(fmt2)
		n := min(font.Be16(set, 0), (len(set)-2)/size)
		lo, hi := 0, n-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			rec := 2 + mid*size
			switch g := font.Be16(set, rec); {
			case second < g:
				hi = mid - 1
			case second > g:
				lo = mid + 1
			default:
				return pairAdjustFrom(set[rec+2:], fmt1, fmt2), true
			}
		}
	case 2:
		// Every second glyph has a class, class 0 for one the table does not
		// name, and the pair applies whatever it adjusts: see
		// pairStartsFormat2.
		c2 := classAt(sub, font.Be16(sub, 10), second)
		n2 := font.Be16(sub, 14)
		if c2 >= n2 {
			return pairAdjust{}, false
		}
		recSize := valueSize(fmt1) + valueSize(fmt2)
		off := 16 + (at*n2+c2)*recSize
		if off+recSize > len(sub) {
			return pairAdjust{}, false
		}
		return pairAdjustFrom(sub[off:], fmt1, fmt2), true
	}
	return pairAdjust{}, false
}

// add records a pair of the legacy kern table, and reports whether there was
// room for it.
//
// The first subtable to name a pair is the one that counts, so an entry already
// there is left alone. A zero adjustment is still an entry: a subtable that
// names a pair has matched, and what a later one says about it is not reached.
func (l *layout) add(kl *kernLookup, first, second int, adj pairAdjust) bool {
	if l.kernPairs >= maxPairs {
		l.pairsCapped = true
		return false
	}
	key := [2]int{first, second}
	if _, taken := kl.pairs[key]; taken {
		return true
	}
	kl.pairs[key] = adj
	l.kernPairs++
	return true
}

// readGPOSPairs reads the pair-positioning lookups of every feature in
// pairFeatures that this run's script selected.
//
// Each lookup is read once however many features name it — Noto Serif Tibetan
// lists two of them under both 'kern' and 'dist' — and they are kept in
// lookup-list order, which is the order a font means its lookups to apply in.
func (l *layout) readGPOSPairs(gpos []byte, idx *featureIndex) {
	// One budget for every subtable this reader may take, shared across the
	// whole table — see subtables.
	budget := subtableBudget(gpos)
	var order []int
	byIndex := map[int][]byte{}
	for _, tag := range pairFeatures {
		lookups, idxs := idx.lookupsFor(tag)
		for i, lookup := range lookups {
			if _, seen := byIndex[idxs[i]]; seen {
				continue
			}
			byIndex[idxs[i]] = lookup
			order = append(order, idxs[i])
		}
	}
	sortInts(order)
	for _, i := range order {
		kind, flags, _, subs := subtables(byIndex[i], 9, &budget) // 9 = extension positioning
		if kind != 2 {                                            // 2 = pair adjustment
			continue
		}
		kl := kernLookup{flags: mergedFlags(flags), byFirst: map[int][]pairStart{}}
		// A subtable named twice in one lookup is kept once. The first
		// subtable to name a pair wins, so the second copy has nothing left
		// to say — and reading it again once per offset is how an 844-byte
		// lookup of four hundred offsets to one subtable took ten seconds to
		// load. Subtables are told apart by length, which is where they
		// start: each runs to the end of the table.
		seen := map[int]bool{}
		for _, sub := range subs {
			if len(sub) < 2 || seen[len(sub)] {
				continue
			}
			seen[len(sub)] = true
			var ok bool
			switch font.Be16(sub, 0) {
			case 1:
				ok = l.pairStartsFormat1(&kl, len(kl.subs), sub)
			case 2:
				ok = l.pairStartsFormat2(&kl, len(kl.subs), sub)
			}
			if ok {
				kl.subs = append(kl.subs, sub)
			}
		}
		if len(kl.byFirst) > 0 {
			l.kern = append(l.kern, kl)
		}
	}
}

// pairStartsFormat1 finds where each first glyph's pairs are in an explicit
// pair list, reporting whether any glyph begins one.
func (l *layout) pairStartsFormat1(kl *kernLookup, at int, sub []byte) bool {
	if len(sub) < 10 {
		return false
	}
	// Only a horizontal advance on the first glyph is kerning; anything else in
	// the record is a positioning this package does not apply, and it is
	// skipped over rather than misread.
	size := 2 + valueSize(font.Be16(sub, 4)) + valueSize(font.Be16(sub, 6))
	pairSetCount := font.Be16(sub, 8)
	any := false
	l.eachCovered(sub, font.Be16(sub, 2), func(i, first int) bool {
		if i >= pairSetCount || 10+2*i+2 > len(sub) {
			return true
		}
		off := font.Be16(sub, 10+2*i)
		// A pair set with no whole record in it names no pair.
		if off <= 0 || off+2+size > len(sub) || font.Be16(sub, off) == 0 {
			return true
		}
		kl.byFirst[first] = append(kl.byFirst[first], pairStart{sub: at, at: i})
		any = true
		return true
	})
	return any
}

// pairStartsFormat2 finds the first class of each covered glyph of a
// class-pair subtable, reporting whether any glyph begins a pair.
//
// Every covered glyph begins one, whatever its row of the class matrix
// states: a class subtable pairs a covered first glyph with every second glyph
// — one its class table does not name is in class 0 — and a pair of zeroes is
// a pair that applied, which stops the lookup's later subtables. That is
// HarfBuzz's reading and the positioning pass's (pairPosAt), and this reading
// has to be the same one, since the pair across a run boundary is found here
// and must be the pair the run itself would have found.
//
// It used to read a row that adjusted nothing as no pair and a second glyph
// the class table left out as unpaired, which let a later subtable of the
// lookup apply where HarfBuzz stops.
func (l *layout) pairStartsFormat2(kl *kernLookup, at int, sub []byte) bool {
	if len(sub) < 16 {
		return false
	}
	class1Off := font.Be16(sub, 8)
	n1, n2 := font.Be16(sub, 12), font.Be16(sub, 14)
	if n1 <= 0 || n2 <= 0 {
		return false
	}
	any := false
	l.eachCovered(sub, font.Be16(sub, 2), func(_, first int) bool {
		// The class is searched for in the class table, glyph by glyph,
		// rather than read out of a map of every glyph the table names.
		if c1 := classAt(sub, class1Off, first); c1 < n1 {
			kl.byFirst[first] = append(kl.byFirst[first], pairStart{sub: at, at: c1})
			any = true
		}
		return true
	})
	return any
}

// valueSize is the byte length of a ValueRecord with the given format: two
// bytes per bit set (ISO/IEC 14496-22, ValueFormat).
func valueSize(format int) int {
	n := 0
	for b := 0; b < 8; b++ {
		if format&(1<<b) != 0 {
			n += 2
		}
	}
	return n
}

// pairAdjust is what a font states about a pair of glyphs: a placement and an
// advance for each of the two.
//
// Reading only the first glyph's advance covers Latin kerning and loses Arabic.
// A right-to-left font states a pair as a placement *and* an advance — Noto Sans
// Arabic writes XPlacement -25 beside XAdvance -25 for reh before an alef —
// because in a run drawn right to left the two do different things: the advance
// moves what comes next, and the placement moves this glyph. Applying half of it
// leaves the letters a hair apart in a script where they are meant to touch.
//
// The fields are 16-bit because the format's are: a ValueRecord holds signed
// 16-bit numbers, and a font this size states seventy thousand pairs, so the
// difference between this and six machine words is a megabyte of a shared table.
type pairAdjust struct {
	firstX, firstY, firstAdvance    int16
	secondX, secondY, secondAdvance int16
	// takesSecond records that the subtable stated a second ValueRecord at all,
	// which decides where the *next* pair is looked for and not what this one
	// does.
	//
	// The specification says a pair positioning lookup moves past both glyphs
	// where ValueFormat2 is non-zero and past only the first where it is zero,
	// so the second glyph of a pair that adjusted it is not the first glyph of
	// the next pair. It cannot be read off the numbers: a font is free to state
	// a second record of all zeroes, and that is not the same as stating none.
	takesSecond bool
}

func (p pairAdjust) zero() bool { return p == pairAdjust{} }

// pairAdjustFrom reads the two ValueRecords of a pair.
func pairAdjustFrom(rec []byte, format1, format2 int) pairAdjust {
	first := readValueRecord(rec, format1)
	size1 := valueSize(format1)
	var second singleAdjust
	if size1 <= len(rec) {
		second = readValueRecord(rec[size1:], format2)
	}
	return pairAdjust{
		firstX: clamp16(first.xPlacement), firstY: clamp16(first.yPlacement),
		firstAdvance: clamp16(first.xAdvance),
		secondX:      clamp16(second.xPlacement), secondY: clamp16(second.yPlacement),
		secondAdvance: clamp16(second.xAdvance),
		takesSecond:   format2 != 0,
	}
}

// clamp16 narrows a value the format stated in sixteen bits back to sixteen.
// Nothing read from a ValueRecord can be outside the range; the bound is here so
// that a future caller of this cannot silently truncate.
func clamp16(v int) int16 {
	switch {
	case v > 0x7FFF:
		return 0x7FFF
	case v < -0x8000:
		return -0x8000
	}
	return int16(v)
}

// # Reading a table without expanding it
//
// A coverage table and a class definition are both ways of stating something
// about a *range* of glyphs in a few bytes: six bytes of a format 2 record name
// sixty-five thousand glyphs. Everything that went wrong with their cost here
// went wrong the same way — a reader turned the statement into a structure the
// size of what it named. A class definition became a map of every glyph it
// named, once per subtable at load and three times per glyph during shaping; a
// coverage became a slice indexed by coverage index and zero-filled up to the
// first index a record gave, however far that was; and the budgets put on
// them metered the symptom each time — glyphs named, not slice length; one
// call, not the table — so the next shape of the same thing walked past them.
//
// HarfBuzz never expands. "What class is this glyph" and "is this glyph
// covered, and at which index" are answered by searching the table's bytes: a
// binary search over format 2's ranges, an index into format 1's array. That is
// what classAt and coverageIndex do, and it is all the shaping path uses.
//
// Pair kerning is not flattened at all any more — see kernLookup. The readers
// that do build flat tables at load — the anchors, the single adjustments, the
// cursive entry and exit points — have to visit what a table names, since the
// flat table is exactly that. They do it by walking the table's own records
// (eachCovered), so what they allocate is what they store, and the walk is
// charged to one allowance for the whole table read (spend). That allowance
// meters work the font genuinely asks for, and it is the only budget left on
// this path.

// coverageBudget is how much work turning a set of tables into flat form may
// take: the glyphs its coverages name, and the records read for them.
//
// Proportional to their size for the reason subtableBudget is: a well-formed
// table's flat form costs about what its bytes cost, and a crafted one's does
// not. The multiplier is generous — a coverage record is six bytes and real
// ranges are short — and the floor is there so that a small table naming one
// long range is not cut short.
func coverageBudget(tables ...[]byte) int {
	n := maxCoverageGlyphs
	for _, t := range tables {
		n += 8 * len(t)
	}
	return n
}

// spend takes n units of the reading allowance, and reports whether there was
// that much left. Once it runs out every reader stops where it is, and the
// layout remembers that it did, so that the reader can say so.
func (l *layout) spend(n int) bool {
	if n > l.covWork {
		l.covWork = 0
		l.workSpent = true
		return false
	}
	l.covWork -= n
	return true
}

// noteLimits records, once per read, the bounds that reading a table ran into.
//
// Saying so is the point. A bound that trips silently turns a hostile font into
// one that is merely shaped a little wrong, which is indistinguishable from a
// defect; one that is reported is a font that was refused in part, for a
// reason, and a caller can pass that on. See Face.LayoutLimits.
func (l *layout) noteLimits(tables string, allowance int) {
	if l.workSpent {
		l.limits = append(l.limits, fmt.Sprintf(
			"reading the font's %s tables took the whole of the %d-unit allowance for "+
				"turning them into flat form; what they state past that point was not read",
			tables, allowance))
		l.workSpent = false
	}
	if l.pairsCapped {
		l.limits = append(l.limits, fmt.Sprintf(
			"the font's kern table states more than the %d kerning pairs this engine "+
				"lists for kerning across a boundary between runs; the rest are not "+
				"applied there", maxPairs))
		l.pairsCapped = false
	}
}

// coverageTable is a coverage table kept as the bytes it is, and asked about
// one glyph at a time.
type coverageTable struct {
	base []byte
	off  int
}

// covers reports whether the table names a glyph.
func (c coverageTable) covers(gid int) bool {
	_, ok := coverageIndex(c.base, c.off, gid)
	return ok
}

// eachCovered calls fn with every glyph a coverage table names and the coverage
// index it names it at, in the table's own order, spending one unit of the
// allowance per glyph. fn returning false stops the walk.
//
// Nothing is built. The old reader returned a slice indexed by coverage index,
// and a format 2 record may start its range at any index: one record naming
// glyph 1 at index 65535 zero-filled sixty-five thousand entries for one unit of
// allowance, and named glyph 0 at every one of them.
func (l *layout) eachCovered(base []byte, off int, fn func(index, gid int) bool) {
	if off <= 0 || off+4 > len(base) {
		return
	}
	c := base[off:]
	switch font.Be16(c, 0) {
	case 1:
		n := font.Be16(c, 2)
		for i := 0; i < n && 4+2*i+2 <= len(c); i++ {
			if !l.spend(1) || !fn(i, font.Be16(c, 4+2*i)) {
				return
			}
		}
	case 2:
		n := font.Be16(c, 2)
		for i := 0; i < n && 4+6*i+6 <= len(c); i++ {
			rec := 4 + 6*i
			start, end, idx := font.Be16(c, rec), font.Be16(c, rec+2), font.Be16(c, rec+4)
			for g := start; g <= end; g++ {
				if !l.spend(1) || !fn(idx+(g-start), g) {
					return
				}
			}
		}
	}
}

// classAt reads the class a class-definition table gives a glyph, by searching
// the table rather than building anything from it: an index into format 1's
// array, a binary search over format 2's ranges. A glyph the table does not
// name is class 0, which is the specification's default.
//
// The search is HarfBuzz's, comparison for comparison, so a malformed table
// whose ranges overlap or run backwards answers the same way in both. A reader
// that took the last range naming a glyph — which is what building a map in
// record order did — could disagree with it there.
func classAt(base []byte, off, gid int) int {
	c, _ := classNamed(base, off, gid)
	return c
}

// classNamed is classAt, also reporting whether the table names the glyph at
// all — which class 0 alone cannot say, since a table may state class 0 for a
// glyph explicitly.
func classNamed(base []byte, off, gid int) (int, bool) {
	if off <= 0 || off+4 > len(base) || gid < 0 {
		return 0, false
	}
	c := base[off:]
	switch font.Be16(c, 0) {
	case 1:
		if len(c) < 6 {
			return 0, false
		}
		i := gid - font.Be16(c, 2)
		if i < 0 || i >= font.Be16(c, 4) || 6+2*i+2 > len(c) {
			return 0, false
		}
		return font.Be16(c, 6+2*i), true
	case 2:
		n := min(font.Be16(c, 2), (len(c)-4)/6)
		lo, hi := 0, n-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			rec := 4 + 6*mid
			switch {
			case gid < font.Be16(c, rec):
				hi = mid - 1
			case gid > font.Be16(c, rec+2):
				lo = mid + 1
			default:
				return font.Be16(c, rec+4), true
			}
		}
	}
	return 0, false
}

// classRangesSorted reports whether a format 2 table's ranges are what the
// specification requires — each well formed, in order, and apart — in which
// case the class of a glyph is simply the class of the range naming it and no
// search is needed to settle it.
func classRangesSorted(c []byte, n int) bool {
	prev := -1
	for i := 0; i < n && 4+6*i+6 <= len(c); i++ {
		first, last := font.Be16(c, 4+6*i), font.Be16(c, 4+6*i+2)
		if first > last || first <= prev {
			return false
		}
		prev = last
	}
	return true
}

// classTable is a class-definition table read for the questions asked of it
// many times a glyph: GDEF's glyph classes, which every lookup flag is checked
// against, and its mark attachment classes.
//
// Those are asked on every glyph of every lookup, so they are answered from an
// array rather than by a search. The array reaches only as far as the highest
// glyph the table names, so it is at most one entry per glyph there can be, and
// it is built once per table read.
type classTable struct {
	dense []uint16
	// named says the table names any glyph at all, which is what decides
	// whether a font has classified its glyphs — see classOf.
	named bool
}

// of is the class the table gives a glyph.
func (c classTable) of(gid int) int {
	if gid >= 0 && gid < len(c.dense) {
		return int(c.dense[gid])
	}
	return 0
}

// readClassTable reads a class definition into a classTable.
//
// The array is filled from the ranges where they are in order and apart, which
// is every real font; a malformed table is filled glyph by glyph from classAt,
// so that the two ways of asking agree. Either way the work is bounded by the
// glyph space and not by the records: a thousand records each naming every
// glyph cost what one does.
func readClassTable(base []byte, off int) classTable {
	if off <= 0 || off+4 > len(base) {
		return classTable{}
	}
	c := base[off:]
	switch font.Be16(c, 0) {
	case 1:
		if len(c) < 6 {
			return classTable{}
		}
		start := font.Be16(c, 2)
		n := min(font.Be16(c, 4), (len(c)-6)/2)
		if n <= 0 {
			return classTable{}
		}
		dense := make([]uint16, start+n)
		for i := 0; i < n; i++ {
			dense[start+i] = uint16(font.Be16(c, 6+2*i))
		}
		return classTable{dense: dense, named: true}
	case 2:
		n := min(font.Be16(c, 2), (len(c)-4)/6)
		highest := -1
		for i := 0; i < n; i++ {
			first, last := font.Be16(c, 4+6*i), font.Be16(c, 4+6*i+2)
			if first <= last && last > highest {
				highest = last
			}
		}
		if highest < 0 {
			return classTable{}
		}
		dense := make([]uint16, highest+1)
		if classRangesSorted(c, n) {
			for i := 0; i < n; i++ {
				rec := 4 + 6*i
				first, last, class := font.Be16(c, rec), font.Be16(c, rec+2), font.Be16(c, rec+4)
				for g := first; g <= last; g++ {
					dense[g] = uint16(class)
				}
			}
		} else {
			for g := range dense {
				dense[g] = uint16(classAt(base, off, g))
			}
		}
		return classTable{dense: dense, named: true}
	}
	return classTable{}
}

// readGSUBLigatures reads ligature substitutions from every 'liga' feature this
// run's script selected.
func (l *layout) readGSUBLigatures(gsub []byte, idx *featureIndex) {
	// One budget for every subtable this reader may take, shared across the
	// whole table — see subtables.
	budget := subtableBudget(gsub)
	// Longest first, once everything is read — see sortLigaturesLongestFirst.
	defer func() {
		for _, ligs := range l.ligatures {
			sortLigaturesLongestFirst(ligs)
		}
	}()
	lookups, _ := idx.lookupsFor("liga")
	for _, lookup := range lookups {
		kind, _, _, subs := subtables(lookup, 7, &budget) // 7 = extension substitution
		if kind != 4 {                                    // 4 = ligature substitution
			continue
		}
		// The flags are not kept. They were, OR-ed together across every
		// lookup of every feature into one int that nothing ever read — and
		// OR-ing them is not a thing that can be right: a lookup flag holds a
		// mark attachment *class* in its top eight bits and a mark filtering
		// set index elsewhere, so two lookups' flags merged are a third
		// lookup's that neither font declared. Of the fetched faces, Noto Sans
		// merged to IgnoreMarks and Noto Sans Arabic to UseMarkFilteringSet.
		//
		// What honours them is the path that applies the lookups, which has
		// each lookup's own flags to hand: see shaper.ignores, and
		// nogdef_test.go for what IgnoreMarks does there. This table is read
		// only by HasLigatures.
		for _, sub := range subs {
			l.ligatureSubst(sub)
		}
	}
}

func (l *layout) ligatureSubst(sub []byte) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return
	}
	setCount := font.Be16(sub, 4)
	l.eachCovered(sub, font.Be16(sub, 2), func(i, first int) bool {
		if i >= setCount || 6+2*i+2 > len(sub) {
			return true
		}
		off := font.Be16(sub, 6+2*i)
		if off <= 0 || off+2 > len(sub) {
			return true
		}
		set := sub[off:]
		n := font.Be16(set, 0)
		for j := 0; j < n; j++ {
			if 2+2*j+2 > len(set) {
				break
			}
			lo := font.Be16(set, 2+2*j)
			if lo <= 0 || lo+4 > len(set) {
				continue
			}
			lig := set[lo:]
			glyph := font.Be16(lig, 0)
			compCount := font.Be16(lig, 2)
			if compCount < 2 || compCount > maxLigatureComponents || 4+2*(compCount-1) > len(lig) {
				continue
			}
			// Charged per ligature read: glyphs may share one ligature set,
			// and every one of them reads it all.
			if !l.spend(compCount) {
				return false
			}
			if len(l.ligatures) >= maxLigatures {
				continue
			}
			comps := make([]int, compCount-1)
			for k := range comps {
				comps[k] = font.Be16(lig, 4+2*k)
			}
			l.ligatures[first] = append(l.ligatures[first], ligature{components: comps, glyph: glyph})
		}
		return true
	})
}

// sortLigaturesLongestFirst puts the ligatures starting with one glyph longest
// first, so that a greedy match prefers ffi to ff. It is stable, so that two of
// one length keep the order the font lists them in.
//
// It is done once, after every subtable is read. It was done after each
// subtable, over every list read so far, and as an insertion sort — a glyph
// starting ten thousand ligatures sorted them once per subtable.
func sortLigaturesLongestFirst(ligs []ligature) {
	sort.SliceStable(ligs, func(a, b int) bool {
		return len(ligs[a].components) > len(ligs[b].components)
	})
}

// readKernTable reads the legacy kern table, format 0, for fonts written before
// GPOS. Only horizontal, non-cross-stream, override-free subtables are taken;
// the rest describe positioning this package does not apply.
func (l *layout) readKernTable(kern []byte) {
	if len(kern) < 4 {
		return
	}
	nTables := font.Be16(kern, 2)
	off := 4
	for i := 0; i < nTables && i < maxSubtables; i++ {
		if off+6 > len(kern) {
			return
		}
		length := font.Be16(kern, off+2)
		coverage := font.Be16(kern, off+4)
		// Bit 0 horizontal, bit 1 minimum, bit 2 cross-stream, bit 3 override;
		// format is the high byte.
		if coverage&0x0001 != 0 && coverage&0x000E == 0 && coverage>>8 == 0 {
			l.kernFormat0(kern[off+6:])
		}
		if length <= 0 {
			return
		}
		off += length
	}
}

// kernFormat0 reads one subtable of the legacy table into a lookup of its own.
// It has no flags to state, so it steps over nothing.
func (l *layout) kernFormat0(t []byte) {
	if len(t) < 8 {
		return
	}
	kl := kernLookup{pairs: map[[2]int]pairAdjust{}}
	defer func() {
		if len(kl.pairs) > 0 {
			l.kern = append(l.kern, kl)
		}
	}()
	n := font.Be16(t, 0)
	for i := 0; i < n; i++ {
		rec := 8 + 6*i
		if rec+6 > len(t) {
			return
		}
		left, right := font.Be16(t, rec), font.Be16(t, rec+2)
		if v := signed16(font.Be16(t, rec+4)); v != 0 {
			// The old 'kern' table states one number: an advance adjustment.
			if !l.add(&kl, left, right, pairAdjust{firstAdvance: clamp16(v)}) {
				return
			}
		}
	}
}

// readSingleSubstitutions reads the one-for-one substitutions of every feature
// this run's script selected, keyed by tag.
//
// A font's 'smcp' turns letters into small capitals and its 'onum' turns
// lining figures into oldstyle ones; both are correct only when a caller asks
// for them. What is applied when one is asked for is the feature's lookups,
// through the plan, as for every other feature — see plan.go. This table
// answers a narrower question, which HasJoiningForms asks: whether the font
// has one-for-one forms under a feature at all.
func (l *layout) readSingleSubstitutions(gsub []byte, idx *featureIndex) {
	// One budget for every subtable this reader may take — see subtables.
	budget := subtableBudget(gsub)
	if len(gsub) < 10 {
		return
	}
	// Every tag the selection admits, once, each answered from the lists read
	// once — see featureIndex, which this reader is the reason for.
	for _, tag := range idx.tags {
		lookups, _ := idx.lookupsFor(tag)
		for _, lookup := range lookups {
			kind, _, _, subs := subtables(lookup, 7, &budget)
			if kind != 1 { // 1 = single substitution
				continue
			}
			// See the note beside the ligature reader: the flags belong to
			// the lookup, and the pass that applies it has them.
			for _, sub := range subs {
				l.singleSubst(tag, sub)
			}
		}
	}
}

// singleSubst reads one single-substitution subtable. Format 1 shifts every
// covered glyph by a constant; format 2 lists a replacement for each.
func (l *layout) singleSubst(tag string, sub []byte) {
	if len(sub) < 6 {
		return
	}
	if l.single[tag] == nil {
		l.single[tag] = map[int]int{}
	}
	switch font.Be16(sub, 0) {
	case 1:
		delta := signed16(font.Be16(sub, 4))
		l.eachCovered(sub, font.Be16(sub, 2), func(_, gid int) bool {
			if to := gid + delta; to >= 0 && to < 0xFFFF {
				l.single[tag][gid] = to
			}
			return true
		})
	case 2:
		n := font.Be16(sub, 4)
		l.eachCovered(sub, font.Be16(sub, 2), func(i, gid int) bool {
			if i < n && 6+2*i+2 <= len(sub) {
				l.single[tag][gid] = font.Be16(sub, 6+2*i)
			}
			return true
		})
	}
	if len(l.single[tag]) == 0 {
		delete(l.single, tag)
	}
}

// emptyLayout is a layout that says nothing, for a face with no tables to read
// — a standard font, whose metrics are published rather than embedded.
func emptyLayout() *layout {
	return &layout{
		ligatures: map[int][]ligature{},
		single:    map[string]map[int]int{},
	}
}

// readMarkGlyphSets reads GDEF's mark glyph sets, which a lookup names to say
// that it looks at those marks and steps over every other.
//
// It is a finer thing than the mark attachment class the flags carry: a class
// partitions the marks, while a set is any collection of them, and a font that
// needs two overlapping groups can only say so this way. The table arrived in
// GDEF 1.2, so a font that predates it has none and the offset is not there to
// read.
func (l *layout) readMarkGlyphSets(gdef []byte) {
	// Version 1.2 or later, and the offset is the fifth in the header.
	if len(gdef) < 14 || font.Be16(gdef, 0) != 1 || font.Be16(gdef, 2) < 2 {
		return
	}
	// An Offset16, like the four before it in the header. The offsets *inside*
	// the table are 32-bit, which is the trap: reading this one the same way
	// finds nothing, and a lookup that filters by a set it cannot find steps
	// over every mark rather than over none.
	off := font.Be16(gdef, 12)
	if off <= 0 || off+4 > len(gdef) {
		return
	}
	sets := gdef[off:]
	if font.Be16(sets, 0) != 1 {
		return
	}
	n := font.Be16(sets, 2)
	if n > maxSubtableList {
		n = maxSubtableList
	}
	for i := 0; i < n; i++ {
		rec := 4 + 4*i
		if rec+4 > len(sets) {
			break
		}
		co := int(font.Be32(sets, rec))
		if co <= 0 || co >= len(sets) {
			// Kept, so that the indices of the sets after it do not shift; a
			// zero offset covers nothing.
			l.markSets = append(l.markSets, coverageTable{})
			continue
		}
		l.markSets = append(l.markSets, coverageTable{base: sets, off: co})
	}
}
