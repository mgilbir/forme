package shape

import (
	"slices"
	"sort"

	"github.com/mgilbir/forme/internal/charprop"
)

// Indic reordering: setting text whose characters are not stored in the order
// they are drawn.
//
// Every other script this package sets is drawn in the order it is written.
// Devanagari and its relatives are not. Three things move:
//
//   - A *pre-base matra*. The vowel sign ि (U+093F) is stored after the
//     consonant it belongs to and drawn before it, so कि — ka then the i-sign —
//     is two glyphs in the opposite order to its two characters.
//   - A *reph*. A syllable opening with र् — Ra followed by a virama — is not
//     drawn as two letters at the front. It becomes a single stroke drawn at
//     the *end* of the syllable, above it.
//   - A *conjunct*. Consonants joined by viramas are drawn as one compound
//     letterform, which the font supplies and which the shaper has to ask for
//     by putting the pieces where the font's rules expect them.
//
// None of this is decidable from the glyphs. It needs the syllable's structure:
// which character is the base consonant, which are its dependents, and where
// each dependent sits. So the run is cut into syllables, each syllable is
// classified from Unicode's Indic categories (indiccategory.go), its glyphs are
// put into drawing order, and the font's features are applied to the parts of
// it each feature is for. This is the model OpenType calls Indic2 — the one a
// font declaring 'dev2' is written against.
//
// # What is covered
//
// The nine scripts that share the model: Bengali, Devanagari, Gujarati,
// Gurmukhi, Kannada, Malayalam, Oriya, Tamil and Telugu.
//
// The model is shared; the data is not, and the data is most of the work. Each
// script names its own virama and its own letter that can become a reph, states
// where in the syllable that reph is drawn and what sequence asks for one, says
// which consonants the below-base forms feature is for, and disagrees with the
// others about how far out from the base a vowel sign is drawn. All of that is
// in indicConfigs and indicMatraPosition, stated per script rather than branched
// on, and a script with none of it stated is not reordered at all.
//
// Both generations of each script's rules are covered. A script that has two
// OpenType specifications has two tags, and the tag a font declares its rules
// under says which of the two it was written against — see indicOldSpec.
//
// Khmer and Myanmar do *not* share this model and are deliberately absent from
// it. Each is its own shaper, with its own categories, its own syllable grammar
// and its own reordering — see khmer.go and myanmar.go. Shaping either by these
// rules would be worse than leaving it, since it would move glyphs by a grammar
// that is not theirs.
//
// The scripts the Universal Shaping Engine covers have a shaper of their own
// too — use.go, which is a fourth model and not a fallback: its clusters are a
// grammar, its features are applied in three groups with a pass between them,
// and it reorders. syllabic.go is what chooses between the four.
//
// These are not done:
//
//   - Asking the font about a consonant *with* surrounding context, which the
//     first-generation rules allow and the second do not. The question is always
//     put as a bare pair of glyphs, so a font that states its below-base or
//     post-base forms only as a contextual rule is read as stating none — its
//     conjuncts then come out as loose letters rather than in the wrong place.
//
// # Where this runs
//
// Before every other substitution, from ShapeGlyphs, in place of the joining
// pass — the two are alternatives, since no script both joins cursively and
// reorders. 'liga' is not applied to an Indic run: a font's discretionary Latin
// ligatures are not written about these glyphs, and the features that *are* —
// 'pres', 'abvs', 'blws', 'psts', 'haln' — are applied here instead.
//
// # Clusters
//
// A syllable's glyphs all take the cluster of its first character. They have to:
// once the glyphs are in drawing order they no longer correspond one-for-one to
// the characters, and a syllable is the smallest piece of these scripts that can
// honestly be mapped back to a position in the text.

// indicCat is what a character is within a syllable — the shaping category,
// which is Unicode's Indic_Syllabic_Category collapsed onto the distinctions
// the reordering actually makes.
type indicCat uint8

const (
	catOther        indicCat = iota // not part of any syllable
	catConsonant                    // C
	catRa                           // the consonant that can become a reph
	catVowel                        // an independent vowel: a syllable of its own
	catMatra                        // a dependent vowel sign
	catNukta                        // N, a dot that modifies the letter before it
	catHalant                       // H, the virama
	catStacker                      // an invisible stacker: a virama that is never drawn
	catZWJ                          // zero width joiner
	catZWNJ                         // zero width non-joiner
	catSM                           // bindu, visarga, gemination, syllable modifier, tone
	catVD                           // a cantillation (Vedic) mark
	catPlaceholder                  // something a syllable can hang off that is not a letter
	catDottedCircle                 // U+25CC, what a syllable with no base is shown against
	catSymbol                       // avagraha and its kind: a cluster of its own
	catRepha                        // a repha written as its own character
	catCM                           // a medial consonant
	catCS                           // a consonant that carries its own stacker
	catRS                           // a register shifter
	catMPst                         // a matra a syllable modifier may stand before
	catSMPst                        // a modifier with no side of its own

	// Khmer and Myanmar group some characters differently from the Indic
	// model, and name groups it has none of. The categories below are theirs
	// (khmer.go, myanmar.go); they are here because they are the same kind of
	// statement — what a character is within its syllable — and because the
	// per-glyph record and the feature machinery are shared.
	//
	// The four vowel-sign categories say which side of the letter a sign is
	// drawn on, which those two models read off the character rather than
	// asking the font, as the Indic one does.
	catVAbv     // a vowel sign drawn above the letter
	catVBlw     // one drawn below it
	catVPre     // one drawn before it, stored after
	catVPst     // one drawn after it
	catRobatic  // Khmer: a mark that may stand between a letter and its subscripts
	catXgroup   // Khmer: a mark that may stand before a vowel sign
	catYgroup   // Khmer: a mark that may stand only at the end of a syllable
	catAsat     // Myanmar: the asat, which kills the vowel of the letter before it
	catMedialY  // Myanmar: medial Ya, and the Mon letters written like it
	catMedialR  // Myanmar: medial Ra, which is drawn before the base
	catMedialW  // Myanmar: medial Wa, and the Shan Wa
	catMedialH  // Myanmar: medial Ha
	catMedialL  // Myanmar: the Mon medial La
	catPTone    // Myanmar: a Pwo or other tone mark
	catVS       // a variation selector, which takes the place of what it follows
	catAnusvara // Myanmar: a sign drawn over the syllable that the reordering counts
)

// indicPos is where a glyph goes within its syllable. The order of these is the
// order the glyphs are drawn in, and sorting a syllable by them *is* the initial
// reordering — which is why they are numbered rather than named alone.
type indicPos uint8

const (
	posStart indicPos = iota
	posRaToBecomeReph
	posPreM  // a pre-base matra: drawn before the base, stored after it
	posPreC  // anything else that precedes the base
	posBaseC // the base consonant
	posAfterMain
	posAboveC
	posBeforeSub
	posBelowC
	posAfterSub
	posBeforePost
	posPostC
	posAfterPost
	posFinalC
	posSMVD // a syllable modifier or Vedic mark: always last
	posEnd
)

// The characters Devanagari's rules name directly. Everything else this file
// decides from Unicode's categories; these three it cannot, because what they
// mean is particular to the script rather than general to their category.
const (
	// devanagariRa is the one consonant that becomes a reph. Its category is
	// plain Consonant like every other letter's, so it can only be named.
	devanagariRa = 0x0930
	// devanagariVirama is the character the font's conjunct rules are written
	// against, and so the one to ask those rules about.
	devanagariVirama = 0x094D
	// dottedCircle is what a reader shows a syllable against when the syllable
	// has no base of its own. Unicode calls it a consonant placeholder; the
	// shaping model treats it as its own thing.
	dottedCircle = 0x25CC
)

// indicBlwfMode says which consonants a font's below-base forms feature is for.
type indicBlwfMode uint8

const (
	// blwfPreAndPost: below-base forms are asked for on both sides of the base.
	blwfPreAndPost indicBlwfMode = iota
	// blwfPostOnly: only after it.
	blwfPostOnly
)

// indicRephMode says how a script writes the reph — the stroke that stands for
// a syllable-initial Ra.
type indicRephMode uint8

const (
	// rephImplicit: any syllable-opening Ra and virama makes one, if the font
	// has a form for the pair.
	rephImplicit indicRephMode = iota
	// rephExplicit: only Ra, virama and a zero-width joiner. Telugu writes a
	// bare Ra and virama for something else, so the joiner is how a writer asks
	// for the reph.
	rephExplicit
	// rephLogRepha: the script has a character of its own for the stroke, which
	// is written where it is read — before the syllable — and drawn where the
	// reph goes. Malayalam's chillu Ra is the one.
	rephLogRepha
)

// indicConfig is what one script states about its own reordering: the two
// characters the rules name directly, and the ways its behaviour differs from
// the others'.
//
// The model is shared; the data is not. Keeping the data here rather than in the
// code is what lets a second script be added by stating what it does rather than
// by branching on which script it is.
type indicConfig struct {
	// tag is the script's second-generation OpenType tag, which is how a config
	// is found: it is the first tag the script selects, so a run finds its
	// config without this file naming an index into the generated script table.
	tag string
	// virama is the character the font's conjunct rules are written against, and
	// so the one to ask those rules about. It is a plain letter by every Unicode
	// property it has, so it can only be named. (Which letter can become a reph
	// is named too, but per character rather than per script — see
	// indicCatOverrides.)
	virama rune
	// hasOldSpec says the script had a first-generation OpenType specification,
	// and so that a font declaring the older of its two tags means the older
	// rules. A script with only one specification never does.
	hasOldSpec bool
	// blwfMode says which consonants 'blwf' is asked for.
	blwfMode indicBlwfMode
	// rephPos is where in the syllable the reph is drawn, and rephMode how the
	// script writes it.
	rephPos  indicPos
	rephMode indicRephMode
	// doubleHalantBlocksMove says a virama already standing after the last
	// consonant stops the first-generation post-base virama move. Reports
	// differ script by script, so only the one known to want it says so.
	doubleHalantBlocksMove bool
	// hasEyelashRa says the script's first-generation rules ask for a below-base
	// form of a pre-base Ra.
	hasEyelashRa bool
	// swapsRaHalantJoiner says the script is written with the joiner after the
	// virama where the model expects it before, and that the two are to be
	// swapped. Kannada is written that way often enough that every shaper
	// accepts it.
	swapsRaHalantJoiner bool
	// hasHalfForms says the script draws half forms at all. Malayalam and Tamil
	// do not: what their 'half' feature makes is a chillu or a ligated virama,
	// which a pre-base vowel sign is drawn *after* rather than before, so
	// there is nothing for the sign to be moved back past.
	hasHalfForms bool
	// skipsUnformedBelowForms says the base moves on past a below-base consonant
	// the font declined to make a form for. Malayalam alone asks for this.
	skipsUnformedBelowForms bool
}

// indicConfigs is every script this file reorders, by its second-generation tag.
//
// Every one of them had a first-generation specification, so every one reads its
// font's tag to decide which rules the font means.
var indicConfigs = map[string]*indicConfig{
	"dev2": {
		tag: "dev2", virama: 0x094D, hasOldSpec: true,
		rephPos: posBeforePost, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
		hasEyelashRa: true, hasHalfForms: true,
	},
	"bng2": {
		tag: "bng2", virama: 0x09CD, hasOldSpec: true,
		rephPos: posAfterSub, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
		hasHalfForms: true,
	},
	"gur2": {
		tag: "gur2", virama: 0x0A4D, hasOldSpec: true,
		rephPos: posBeforeSub, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
		hasHalfForms: true,
	},
	"gjr2": {
		tag: "gjr2", virama: 0x0ACD, hasOldSpec: true,
		rephPos: posBeforePost, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
		hasHalfForms: true,
	},
	"ory2": {
		tag: "ory2", virama: 0x0B4D, hasOldSpec: true,
		rephPos: posAfterMain, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
		hasHalfForms: true,
	},
	"tml2": {
		tag: "tml2", virama: 0x0BCD, hasOldSpec: true,
		rephPos: posAfterPost, rephMode: rephImplicit, blwfMode: blwfPreAndPost,
	},
	"tel2": {
		tag: "tel2", virama: 0x0C4D, hasOldSpec: true,
		rephPos: posAfterPost, rephMode: rephExplicit, blwfMode: blwfPostOnly,
		hasHalfForms: true,
	},
	"knd2": {
		tag: "knd2", virama: 0x0CCD, hasOldSpec: true,
		rephPos: posAfterPost, rephMode: rephImplicit, blwfMode: blwfPostOnly,
		hasHalfForms: true, swapsRaHalantJoiner: true, doubleHalantBlocksMove: true,
	},
	"mlm2": {
		tag: "mlm2", virama: 0x0D4D, hasOldSpec: true,
		rephPos: posAfterMain, rephMode: rephLogRepha, blwfMode: blwfPreAndPost,
		skipsUnformedBelowForms: true,
	},
}

// indicConfigFor reports the reordering model for a run's script, or nil for a
// script this file does not reorder.
//
// It asks the script's OpenType tags rather than naming an index into the
// generated script table, so that each script is identified by the tag a font
// declares its rules under.
func indicConfigFor(script uint16) *indicConfig {
	for _, tag := range scriptTags(script) {
		if c, ok := indicConfigs[tag]; ok {
			return c
		}
	}
	return nil
}

// maxIndicSyllable bounds how many characters one syllable may hold.
//
// The reordering sorts a syllable by insertion, which is quadratic, and the
// grammar below lets a syllable grow without limit — a consonant and a virama
// repeated is a legal, if meaningless, sequence, and text is untrusted input
// exactly as a font is. A syllable longer than this is cut, and the remainder
// starts a new one, which changes where its glyphs go. Real Devanagari
// syllables run to a handful of characters; the longest conjunct anyone writes
// is nowhere near this.
const maxIndicSyllable = 64

// indicOldSpec reports whether the font means the first-generation rules for
// this script.
//
// A script that has two OpenType specifications has two tags, and the tag a font
// declares its rules under says which of the two it was written against: a font
// carrying only 'deva' was written before 'dev2' existed. The difference is not
// cosmetic — the two disagree about where the reph goes and about which
// consonants the below-base forms feature is for — so a font written for the
// older rules and shaped by the newer ones sets some conjuncts differently from
// the way its author drew them.
//
// The question is asked of the font rather than of the text, and asked the same
// way feature selection asks it, so that the rules applied are the rules of the
// script table they came from. A font that declares nothing for the script falls
// back to the default table, which is not a second-generation declaration and so
// means the older rules — the same reading every other shaper takes.
func (f *Face) indicOldSpec(cfg *indicConfig, script uint16, lang otLanguage) bool {
	if !cfg.hasOldSpec {
		return false
	}
	tag := f.chosenScriptTag(script, lang)
	return len(tag) != 4 || tag[3] != '2'
}

// indicProperties reports a character's shaping category and where it sits.
//
// Only a character of the blocks HarfBuzz's model is written for has one: see
// indicModelBlocks. Any other is Other, whatever Unicode says of it, unless it
// is one of the overrides below.
func indicProperties(r rune) (indicCat, indicPos) {
	syl, pos := indicCategories(r)
	if !inIndicModelBlock(r) {
		syl = indicSylOther
	}
	cat := catOther
	switch syl {
	case indicSylConsonant, indicSylConsonantDead,
		indicSylConsonantHeadLetter, indicSylConsonantInitialPostfixed:
		cat = catConsonant
	// A final, a subjoined and a succeeding repha are all consonants that hang
	// off the base rather than being one, which is what a medial consonant is.
	case indicSylConsonantMedial, indicSylConsonantFinal,
		indicSylConsonantSubjoined, indicSylConsonantSucceedingRepha:
		cat = catCM
	case indicSylConsonantWithStacker:
		cat = catCS
	case indicSylConsonantPrecedingRepha:
		cat = catRepha
	case indicSylConsonantPlaceholder, indicSylNumber,
		indicSylNumberJoiner, indicSylBrahmiJoiningNumber:
		cat = catPlaceholder
	case indicSylVowelIndependent, indicSylVowel:
		cat = catVowel
	// A killer takes the vowel off the letter before it, which is what a vowel
	// sign does to the letter's inherent vowel, so it is placed like one.
	case indicSylVowelDependent, indicSylPureKiller, indicSylConsonantKiller:
		cat = catMatra
	case indicSylNukta, indicSylToneMark:
		cat = catNukta
	case indicSylVirama:
		cat = catHalant
	case indicSylInvisibleStacker:
		cat = catStacker
	case indicSylJoiner:
		cat = catZWJ
	case indicSylNonJoiner:
		cat = catZWNJ
	case indicSylBindu, indicSylVisarga, indicSylGeminationMark,
		indicSylSyllableModifier:
		cat = catSM
		if pos == indicPosNotApplicable {
			// A modifier with no side of its own is drawn after the syllable
			// rather than over it, and the grammar lets one stand where a matra
			// would: the Gurmukhi bindi before its vowel sign is the case.
			cat = catSMPst
		}
	case indicSylCantillationMark:
		cat = catVD
	case indicSylAvagraha:
		cat = catSymbol
	case indicSylRegisterShifter:
		cat = catRS
	}
	if o, ok := indicCatOverrides[r]; ok {
		cat = o
	}
	return cat, indicPositionOf(r, cat, pos)
}

// indicModelBlocks are the blocks whose characters the Indic model gives a
// category, and indicModelSingles two more characters it gives one: HarfBuzz's
// ALLOWED_BLOCKS and ALLOWED_SINGLES (gen-indic-table.py). The Myanmar and
// Khmer blocks are there because HarfBuzz reads one table for three models;
// they change nothing here, where no Indic run holds their characters.
//
// Unicode gives an Indic syllabic category to characters elsewhere too, and to
// the model those are Other. The two an Indic run can hold are marks of the
// Inherited script: U+1DFB COMBINING DELETION MARK and U+20F0 COMBINING
// ASTERISK ABOVE. Read as Unicode categorises it, the asterisk was a
// cantillation mark that the syllable before took in, and a Vedic sign after
// it went with them; to HarfBuzz it ends the syllable, and the sign is shown
// against a dotted circle. TestIndicCategoriesAreHarfBuzzs holds every
// character an Indic run can hold to HarfBuzz's generator.
var indicModelBlocks = [...]struct{ lo, hi rune }{
	{0x0000, 0x007F},   // Basic Latin
	{0x0080, 0x00FF},   // Latin-1 Supplement
	{0x0900, 0x097F},   // Devanagari
	{0x0980, 0x09FF},   // Bengali
	{0x0A00, 0x0A7F},   // Gurmukhi
	{0x0A80, 0x0AFF},   // Gujarati
	{0x0B00, 0x0B7F},   // Oriya
	{0x0B80, 0x0BFF},   // Tamil
	{0x0C00, 0x0C7F},   // Telugu
	{0x0C80, 0x0CFF},   // Kannada
	{0x0D00, 0x0D7F},   // Malayalam
	{0x1000, 0x109F},   // Myanmar
	{0x1780, 0x17FF},   // Khmer
	{0x1CD0, 0x1CFF},   // Vedic Extensions
	{0x2000, 0x206F},   // General Punctuation
	{0x2070, 0x209F},   // Superscripts and Subscripts
	{0xA8E0, 0xA8FF},   // Devanagari Extended
	{0xA9E0, 0xA9FF},   // Myanmar Extended-B
	{0xAA60, 0xAA7F},   // Myanmar Extended-A
	{0x116D0, 0x116FF}, // Myanmar Extended-C
}

var indicModelSingles = [...]rune{0x00A0, dottedCircle}

// inIndicModelBlock reports whether the Indic model gives a character a
// category of its own. See indicModelBlocks.
func inIndicModelBlock(r rune) bool {
	for _, b := range indicModelBlocks {
		if r >= b.lo && r <= b.hi {
			return true
		}
	}
	return slices.Contains(indicModelSingles[:], r)
}

// indicCatOverrides are the characters whose shaping category is not the one
// their Unicode category implies.
//
// Each is a statement about that character in particular, which is why it can
// only be named. They come from the same reading of the script development
// specifications that every shaper works from.
var indicCatOverrides = map[rune]indicCat{
	// The one consonant of each script that can become a reph. Its Unicode
	// category is plain Consonant, like every other letter's. Bengali has two,
	// the second being the Assamese letter.
	0x0930: catRa, // Devanagari
	0x09B0: catRa, // Bengali
	0x09F0: catRa, // Bengali, Assamese
	0x0A30: catRa, // Gurmukhi
	0x0AB0: catRa, // Gujarati
	0x0B30: catRa, // Oriya
	0x0BB0: catRa, // Tamil
	0x0C30: catRa, // Telugu
	0x0CB0: catRa, // Kannada
	0x0D30: catRa, // Malayalam

	dottedCircle: catDottedCircle,

	// The two Devanagari accents behave as the bindus do, not as the
	// cantillation marks their category would suggest.
	0x0953: catSM,
	0x0954: catSM,

	// The Gurmukhi vowel sign II may be preceded by the bindi, which is what
	// the post-matra category is for.
	0x0A40: catMPst,
	// Two Gurmukhi letters that Unicode classifies as neither consonant nor
	// vowel but that a syllable is built on exactly as it is on a consonant.
	0x0A72: catConsonant,
	0x0A73: catConsonant,
	// A Gurmukhi sign that behaves as a vowel sign rather than a mark.
	0x0A51: catMatra,

	// Marks that take their own cluster, as the avagraha does.
	0xA8F2: catSymbol, 0xA8F3: catSymbol, 0xA8F4: catSymbol,
	0xA8F5: catSymbol, 0xA8F6: catSymbol, 0xA8F7: catSymbol,
	0x1CE9: catSymbol, 0x1CEA: catSymbol, 0x1CEB: catSymbol,
	0x1CEC: catSymbol, 0x1CEE: catSymbol, 0x1CEF: catSymbol,
	0x1CF0: catSymbol, 0x1CF1: catSymbol,

	// Vedic marks that are only valid after particular signs. Treating them as
	// ordinary tone marks is not right, but it is what every shaper does, and
	// the alternative is a rule about which sign each may follow.
	0x1CE2: catVD, 0x1CE3: catVD, 0x1CE4: catVD, 0x1CE5: catVD,
	0x1CE6: catVD, 0x1CE7: catVD, 0x1CE8: catVD, 0x1CED: catVD,

	// Grantha marks that Tamil also uses, so the Indic model has to know them.
	0x11301: catSM, 0x11302: catSM, 0x11303: catSM,
	0x1133B: catNukta, 0x1133C: catNukta,

	// Signs that modify the letter before them rather than standing alone.
	0x0AFB: catNukta, // Gujarati
	0x0B55: catNukta, // Oriya

	// Marks a syllable can be built on that are not letters.
	0x09FC: catPlaceholder, // Bengali
	0x0C80: catPlaceholder, // Kannada
	0x0D04: catPlaceholder, // Malayalam

	// Placeholders the Myanmar specification names, which are not Myanmar's
	// and which HarfBuzz gives the Indic model too: a vowel sign written on a
	// bullet or a dash is shown on it, as on a dotted circle.
	0x2015: catPlaceholder, 0x2022: catPlaceholder,
	0x25FB: catPlaceholder, 0x25FC: catPlaceholder, 0x25FD: catPlaceholder, 0x25FE: catPlaceholder,
}

// indicPosOverrides are the characters drawn somewhere other than where their
// Unicode positional category puts them.
var indicPosOverrides = map[rune]indicPos{
	0x0A51: posBelowC,    // Gurmukhi udaat
	0x0B01: posBeforeSub, // the Oriya bindu, which the specification places here
}

// indicCategories reports Unicode's two Indic categories for a character.
func indicCategories(r rune) (indicSyllabic, indicPosition) {
	i := sort.Search(len(indicRanges), func(i int) bool { return indicRanges[i].hi >= r })
	if i < len(indicRanges) && r >= indicRanges[i].lo {
		return indicRanges[i].syl, indicRanges[i].pos
	}
	return indicSylOther, indicPosNotApplicable
}

// indicPositionOf turns a character, its category and Unicode's positional
// category into the place a glyph takes in its syllable.
//
// A consonant is the base until the reordering decides otherwise; a syllable
// modifier or Vedic mark is always last; and a mark that attaches to the letter
// takes its place from which side of the letter it is drawn on. The compound
// positions — top-and-right, bottom-and-right — resolve to the last part of
// what they name, because that is the part whose place in the sequence decides
// where the whole thing goes.
//
// Only the categories that are drawn *relative to* the letter keep a place of
// their own: a medial consonant, a modifier, a register shifter, a virama and a
// vowel sign. Everything else has none, because nothing in the model asks — a
// nukta and a joiner take the place of what they follow, and a consonant's is
// decided by the base search.
//
// A vowel sign is the one that is not general: which side of the letter it is
// written on is Unicode's to say, but how far out from the base it is *drawn* is
// each script's own, and the scripts disagree — see indicMatraPosition.
func indicPositionOf(r rune, cat indicCat, pos indicPosition) indicPos {
	if p, ok := indicPosOverrides[r]; ok {
		return p
	}
	side := posEnd
	switch pos {
	case indicPosLeft:
		side = posPreC
	case indicPosTop, indicPosTopAndLeft:
		side = posAboveC
	case indicPosBottom, indicPosTopAndBottom, indicPosBottomAndLeft, indicPosTopAndBottomAndLeft:
		side = posBelowC
	case indicPosRight, indicPosBottomAndRight, indicPosLeftAndRight,
		indicPosTopAndRight, indicPosTopAndLeftAndRight, indicPosTopAndBottomAndRight:
		side = posPostC
	case indicPosOverstruck:
		side = posAfterMain
	case indicPosVisualOrderLeft:
		side = posPreM
	}
	switch {
	case indicIsBaseCandidate(cat):
		return posBaseC
	case indicIsMatra(cat):
		return indicMatraPosition(r, side)
	case cat == catSM || cat == catSMPst || cat == catVD || cat == catSymbol:
		return posSMVD
	// Not catCM: a consonant medial is a base candidate, which the first case
	// above answers, so naming it here reached nothing.
	case cat == catRS || indicIsHalant(cat):
		return side
	}
	return posEnd
}

// indicScript names the blocks whose vowel signs are placed differently. It is
// the character's block rather than the run's script because the question is
// about the character: a Bengali vowel sign is drawn where Bengali draws it
// wherever it is written.
type indicScript uint8

const (
	scriptNotIndic indicScript = iota
	scriptDevanagariBlock
	scriptBengaliBlock
	scriptGurmukhiBlock
	scriptGujaratiBlock
	scriptOriyaBlock
	scriptTamilBlock
	scriptTeluguBlock
	scriptKannadaBlock
	scriptMalayalamBlock
)

// indicBlockOf reports which of the nine blocks a character is in. They are
// contiguous and 128 apart, which is why this is arithmetic rather than a table.
func indicBlockOf(r rune) indicScript {
	if r < 0x0900 || r > 0x0D7F {
		return scriptNotIndic
	}
	return scriptDevanagariBlock + indicScript((r-0x0900)/0x80)
}

// indicMatraPosition reports how far out from the base a vowel sign is drawn,
// given which side of the letter Unicode says it is written on.
//
// This is where the scripts disagree most, and it is not derivable: the same
// side means a different place in the drawing order in each of them. A Bengali
// right-side sign is drawn after everything, a Devanagari one after the
// below-base forms, a Telugu one before them. Getting it wrong puts a vowel sign
// on the wrong side of a conjunct, which every reader of the script sees at
// once and no test of Devanagari alone would catch.
//
// A sign written to the left is drawn before the base in every script, and that
// is the one rule they all share.
func indicMatraPosition(r rune, side indicPos) indicPos {
	if side == posPreC || side == posPreM {
		return posPreM
	}
	block := indicBlockOf(r)
	switch side {
	case posPostC:
		switch block {
		case scriptBengaliBlock, scriptGurmukhiBlock, scriptGujaratiBlock,
			scriptOriyaBlock, scriptTamilBlock, scriptMalayalamBlock:
			return posAfterPost
		case scriptTeluguBlock:
			if r <= 0x0C42 {
				return posBeforeSub
			}
			return posAfterSub
		case scriptKannadaBlock:
			if r < 0x0CC3 || r > 0x0CD6 {
				return posBeforeSub
			}
			return posAfterSub
		}
	case posAboveC:
		// Bengali and Malayalam have no above-base vowel signs, so neither
		// states a place for one.
		switch block {
		case scriptGurmukhiBlock:
			return posAfterPost
		case scriptOriyaBlock:
			return posAfterMain
		case scriptTeluguBlock, scriptKannadaBlock:
			return posBeforeSub
		}
	case posBelowC:
		switch block {
		case scriptGurmukhiBlock, scriptGujaratiBlock, scriptTamilBlock,
			scriptMalayalamBlock:
			return posAfterPost
		case scriptTeluguBlock, scriptKannadaBlock:
			return posBeforeSub
		}
	}
	// Devanagari's place, and the one every script falls back to.
	return posAfterSub
}

// indicIsBaseCandidate reports whether a character can be a syllable's base.
// A vowel and a placeholder can: a syllable does not need a consonant.
func indicIsBaseCandidate(c indicCat) bool {
	switch c {
	case catConsonant, catRa, catCS, catCM, catVowel, catPlaceholder, catDottedCircle:
		return true
	}
	return false
}

// indicIsHalant reports whether a character kills the vowel of the consonant
// before it, whether or not it is itself drawn.
func indicIsHalant(c indicCat) bool { return c == catHalant || c == catStacker }

// indicIsMatra reports whether a character is a vowel sign — including the one
// kind a syllable modifier may stand in front of, which is a matra in every
// respect but where the grammar admits it.
func indicIsMatra(c indicCat) bool { return c == catMatra || c == catMPst }

// indicIsModifier reports whether a character is a syllable modifier.
func indicIsModifier(c indicCat) bool { return c == catSM || c == catSMPst }

// indicIsJoiner reports whether a character is one of the two zero-width
// controls, which a syllable's structure has to step over but which also change
// what the font is asked for.
func indicIsJoiner(c indicCat) bool { return c == catZWJ || c == catZWNJ }

// indicIsAttached reports whether a character has no place of its own and takes
// the one of whatever it follows.
func indicIsAttached(c indicCat) bool {
	switch c {
	case catZWJ, catZWNJ, catNukta, catRS, catCM, catHalant, catStacker:
		return true
	}
	return false
}

// indicInfo is what the reordering knows about one glyph: what character it
// came from and where it goes. Which features are for it is on the glyph
// itself — see glyphMask.
type indicInfo struct {
	cat indicCat
	pos indicPos
	// syllable numbers the syllable the glyph belongs to, so that the last
	// stage, which runs over the whole run once the syllables are reassembled,
	// can still hold the features that are for one syllable to it. See
	// applyStageBySyllable.
	syllable int32
	// wordStart says the character before this one in the text ends a word,
	// which is what the word-initial form 'init' is for. It is recorded while
	// the glyphs still correspond to the characters, because a lookup applied
	// before the syllables are cut may change how many glyphs there are.
	wordStart bool
	// ignorable says the character this glyph came from is one nothing is drawn
	// for. It reaches the shaper — a syllable model has to see it to be broken
	// by it — and must not reach the page, so it is remembered here and dropped
	// at the end, exactly as a joiner is.
	ignorable bool
	// ligated says the glyph is what a ligature substitution made of several
	// others, and has not since been taken apart again.
	//
	// It is a property of the *glyph*, not of the character, and only the
	// pre-base-reordering Ra needs it: a font may declare that consonant's
	// pre-base form generally and then decline to make it in some context, and
	// the only way to tell whether it declined is to look at what came out.
	// Moving a Ra the font left as an ordinary letter would draw a plain
	// consonant before the base.
	ligated bool
}

// indicBasicFeatures are applied to one syllable at a time, in this order,
// before the syllable is finally reordered, each a stage of its own. A zero mask
// means the feature is for the whole syllable; the others are for part of it.
// A font declares each of those for a range the syllable's structure decides —
// the half-forms feature is for the consonants before the base and nothing else
// — and applying one where it was not meant substitutes glyphs the font never
// intended to appear together. The initial reordering marks which glyph each is
// for, on the glyph: see glyphMask.
//
// The order is the OpenType Indic2 order and is not negotiable: 'nukt'
// composes a letter with its dot so the rest see one glyph, 'rphf' makes the
// reph before anything can consume its Ra, the half and conjunct forms are
// built from what is left, and 'cjct' comes last so that it sees the forms the
// earlier ones made.
var indicBasicFeatures = []struct {
	tag  string
	mask glyphMask
}{
	{"nukt", 0},
	{"akhn", 0},
	{"rphf", maskRphf},
	{"rkrf", 0},
	{"pref", maskPref},
	{"blwf", maskBlwf},
	{"abvf", maskAbvf},
	{"half", maskHalf},
	{"pstf", maskPstf},
	{"vatu", 0},
	{"cjct", 0},
}

// indicPresentationFeatures turn the reordered pieces of one syllable into the
// shapes a reader sees, and they see that syllable and nothing else.
//
// They were applied to the whole run, once every syllable was in drawing order,
// which is what the Khmer model asks for and not what this one does. The
// difference is in the two models as written: Khmer's other features are
// "applied all at once after clearing syllables", and the Indic model's are
// "applied all at once, after final reordering, *constrained to the syllable*".
//
// It is not a distinction without a difference. A lookup that ran over the whole
// run could join the end of one syllable to the start of the next — a below-base
// form reaching past its own cluster into the letter after it — and no font
// writes those rules meaning that. What made it hard to see is that a font whose
// rules are narrow enough never produces one, so most text comes out the same
// either way.
//
// They are one stage with 'init' — for a pre-base matra that opens a word — and
// with the ligatures and contextual alternates every script gets, which are not
// held to a syllable: the lot is applied in lookup order, as HarfBuzz applies it
// and as a font that interleaves the two was written against. See collectIndic.
var indicPresentationFeatures = []string{"pres", "abvs", "blws", "psts", "haln"}

// shapeIndic is the whole Indic pass: it replaces both the joining pass and the
// default substitutions for a run it handles.
func (sh shaper) shapeIndic(buf []Glyph, runes, before []rune, plan *indicPlan, p *plan) []Glyph {
	// A vowel followed by a sign that spells a different vowel has already
	// been shown against a dotted circle, before normalisation: see
	// markInvalidVowels.
	buf, runes = sh.splitMatras(buf, runes)

	info := make([]indicInfo, len(runes))
	for i, r := range runes {
		info[i].cat, info[i].pos = indicProperties(r)
		info[i].ignorable = hiddenAfterShaping(r)
		info[i].wordStart = indicWordStart(before, runes, i)
	}
	hooks := indicHooks(&info)

	// The stages before the syllables are cut: 'rvrn', and what the direction
	// selects. They are the whole run's.
	for s := 0; s < p.syllables; s++ {
		buf, _, _ = sh.applyLookups(buf, p.stage(s), 0, len(buf), 0, len(buf), hooks)
	}
	cats := make([]indicCat, len(info))
	for i := range info {
		cats[i] = info[i].cat
	}

	// Each syllable is shaped on its own and the run is put back together from
	// what comes out.
	//
	// Shaping them where they lay was the arrangement here, with the length
	// each one changed by carried forward as a shift into the bounds of the
	// next. It is the same answer and it is quadratic: a syllable that ligates
	// moves every glyph after it, and a run is n/3 syllables of three glyphs, so
	// a page of Devanagari copied itself n/3 times over. Nothing needs it —
	// every rule a syllable is put through is bounded by the syllable, floor and
	// ceiling both, so a syllable is shaped from what is in it and nothing else.
	// Appending the answers costs each glyph one copy.
	out := make([]Glyph, 0, len(buf))
	outInfo := make([]indicInfo, 0, len(info))
	dotted, hasDotted := sh.f.GlyphID(dottedCircle)
	var serial int32
	// The cut covers the run, one syllable after another, so every glyph is
	// shaped as part of one. What the model does not reorder — a symbol cluster
	// and a character of no Indic category — is a syllable too. It used to be
	// passed through a glyph at a time, so that no feature held to a syllable
	// reached it at all; see shapeIndicUnordered.
	for _, syl := range indicSyllables(cats) {
		syllable := append([]Glyph(nil), buf[syl.start:syl.end]...)
		record := append([]indicInfo(nil), info[syl.start:syl.end]...)
		if syl.kind == sylNonIndic || syl.kind == sylSymbol {
			syllable = sh.shapeIndicUnordered(syllable, &record, p)
		} else {
			placeholder := -1
			if syl.kind == sylBroken && hasDotted {
				placeholder = dotted
			}
			syllable = sh.shapeIndicSyllable(syllable, &record, plan, p, info[syl.start].wordStart, placeholder)
		}
		serial++
		for i := range record {
			record[i].syllable = serial
		}
		out = append(out, syllable...)
		outInfo = append(outInfo, record...)
	}
	buf, info = out, outInfo

	// The last stage, over the whole run: the presentation features held to
	// their syllable, and the ligatures and contextual alternates that are not.
	// See indicPresentationFeatures.
	for s := p.after; s < len(p.stages); s++ {
		buf = sh.applyStageBySyllable(buf, p.stage(s), hooks, func() [][2]int {
			return indicSyllableWindows(info)
		})
	}

	// The joiners have now done everything they are for: the forms they forced
	// or forbade are made, and nothing below is written about them. What is left
	// is a character with no shape, which must not reach the page.
	return dropUnsubstituted(buf, func(i int) bool {
		return i < len(info) && (indicIsJoiner(info[i].cat) || info[i].ignorable)
	})
}

// indicSyllableWindows is where each syllable of a run lies, as the records say:
// a window per stretch of glyphs carrying one syllable number.
func indicSyllableWindows(info []indicInfo) [][2]int {
	var out [][2]int
	for i := 0; i < len(info); {
		j := i + 1
		for j < len(info) && info[j].syllable == info[i].syllable {
			j++
		}
		out = append(out, [2]int{i, j})
		i = j
	}
	return out
}

// markInvalidVowels shows a dotted circle inside any sequence that spells a
// vowel nobody writes — an independent vowel followed by a sign that would make
// it look like a different vowel (indicvowel.go). It is HarfBuzz's
// _hb_preprocess_text_vowel_constraints, which its Indic and universal engines
// both run.
//
// It runs on the characters, before normalisation and before anything is
// classified, because it is a claim about which characters were written rather
// than about the syllable they form, and because it changes the run everything
// below is built from: the circle it inserts is an ordinary character of the
// text from that point on, and the syllable cut sees it as one. It ran after
// normalisation, which reorders marks, so a sequence written with a virama
// between the vowel and the sign was read as the vowel and the sign once the
// sign had been sorted in front of the virama: Chathura's "ఒ్ౕ" came out with a
// circle HarfBuzz does not draw.
//
// The circle goes in whether or not the face can draw one, as HarfBuzz puts it
// in: where the face has none it is drawn as .notdef, which is still a mark of
// malformed text where the reader can see it, and setting the sequence as
// though it were meant says the opposite. It is not counted missing — the text
// has no such character to be missing — which is why it is given the offset of
// the character after it, as HarfBuzz gives it that character's cluster: see
// isInsertedCircle. It used to be left out of a face without one.
//
// A match takes the characters it matched with it: the sign after the circle
// does not open another. Each circle goes before the sign, after the letter and
// the virama a three-character entry opens with.
func markInvalidVowels(runes []rune, offsets []int) ([]rune, []int) {
	var out []rune
	var off []int
	i := 0
	for i+1 < len(runes) {
		n := indicInvalidClusterAt(runes, i)
		if n == 0 {
			if out != nil {
				out = append(out, runes[i])
				off = append(off, offsets[i])
			}
			i++
			continue
		}
		if out == nil {
			out = append(make([]rune, 0, len(runes)+4), runes[:i]...)
			off = append(make([]int, 0, len(runes)+4), offsets[:i]...)
		}
		out = append(out, runes[i:i+n-1]...)
		off = append(off, offsets[i:i+n-1]...)
		out = append(out, dottedCircle, runes[i+n-1])
		off = append(off, offsets[i+n-1], offsets[i+n-1])
		i += n
	}
	if out == nil {
		return runes, offsets
	}
	out = append(out, runes[i:]...)
	off = append(off, offsets[i:]...)
	return out, off
}

// markVowelCircles gives each dotted circle markInvalidVowels put into a run
// the character properties of the sign after it, as HarfBuzz gives them: it
// makes the circle out of that sign's record, so the circle before a
// non-spacing sign is a mark, to the lookups that step over marks and to
// everything else. buf is one to one with runes.
func markVowelCircles(buf []Glyph, runes []rune, offsets []int) {
	for i := range runes {
		if isInsertedCircle(runes, offsets, i) {
			buf[i].class = classOfRune(runes[i+1])
			buf[i].umark = unicodeMarkOf(runes[i+1])
		}
	}
}

// sharedIndicCategory is what HarfBuzz's one category table says of a
// character a Khmer or Myanmar run takes in from outside its own blocks — a
// digit, a no-break space, a dash or a bullet to hang a vowel sign on, a Vedic
// tone mark, a superscript digit — narrowed by the model to the categories its
// grammar names. HarfBuzz reads one table for its Indic, Khmer and Myanmar
// models, and what the Indic model says of these characters is what the table
// says; the Khmer and Myanmar tables here name only their own blocks, so a
// digit a Myanmar vowel sign was written on was a character of no syllable,
// and the sign stood alone against a dotted circle.
func sharedIndicCategory(r rune, narrow func(indicCat) indicCat) indicCat {
	c, _ := indicProperties(r)
	return narrow(c)
}

// isInsertedCircle reports whether the character at i is a dotted circle
// markInvalidVowels put into the run, rather than one the text holds: it
// shares the offset of the character after it, which no two characters of the
// text do.
func isInsertedCircle(runes []rune, offsets []int, i int) bool {
	return runes[i] == dottedCircle && i+1 < len(runes) && offsets[i+1] == offsets[i]
}

// splitMatras replaces each vowel sign that is written as one character and
// drawn as two or three marks by the marks it is drawn as (indicmatra.go).
//
// It is the shaping model's own second step, and it has to happen before
// anything is placed: the parts of a split sign go to *different* places, one
// before the letter and one after, so there is no single place the sign itself
// could be given. Tamil's o-sign is the plain case — U+0BCA is one character and
// two marks, and a shaper that kept it whole would draw the letter with both
// marks on the same side of it.
//
// A sign is only taken apart when the face has a glyph for every part. A face
// that draws the sign whole and has no glyph for one of its halves would
// otherwise lose that half altogether, which is worse than drawing the sign
// where the model would rather it were not.
func (sh shaper) splitMatras(buf []Glyph, runes []rune) ([]Glyph, []rune) {
	return sh.splitCharacters(buf, runes, indicSplitMatraOf)
}

// indicSplitMatraOf reports the marks a vowel sign is drawn as, if it is one of
// the signs drawn as more than one.
func indicSplitMatraOf(r rune) ([]rune, bool) {
	i := sort.Search(len(indicSplitMatras), func(i int) bool {
		return indicSplitMatras[i].r >= r
	})
	if i >= len(indicSplitMatras) || indicSplitMatras[i].r != r {
		return nil, false
	}
	parts := indicSplitMatras[i].parts[:]
	for len(parts) > 0 && parts[len(parts)-1] == 0 {
		parts = parts[:len(parts)-1]
	}
	return parts, true
}

// indicInvalidClusterAt reports the length of the invalid cluster starting at a
// position, or zero if none does.
func indicInvalidClusterAt(runes []rune, at int) int {
	i := sort.Search(len(indicInvalidClusters), func(i int) bool {
		return indicInvalidClusters[i][0] >= runes[at]
	})
	for ; i < len(indicInvalidClusters) && indicInvalidClusters[i][0] == runes[at]; i++ {
		c := indicInvalidClusters[i]
		n := len(c)
		if c[n-1] == 0 {
			n--
		}
		if at+n > len(runes) {
			continue
		}
		match := true
		for k := 1; k < n; k++ {
			if runes[at+k] != c[k] {
				match = false
				break
			}
		}
		if match {
			return n
		}
	}
	return 0
}

// insertDottedCircle puts U+25CC at the front of a syllable that has no base
// consonant of its own.
//
// A matra or a virama written with nothing to attach to is not text anyone
// meant to write, but it has to be *shown* — and a mark drawn on its own floats
// at the height it would have sat at, over nothing, where a reader cannot tell
// it from a mark on the letter before. The dotted circle is the placeholder
// every reader of these scripts knows: it says "a mark, and the letter it
// belongs to is missing".
//
// It goes after a repha, which is written before the letter it belongs to and
// so belongs before the placeholder too. Its own place is deliberately left at
// the end of the syllable rather than set to the base: the font is asked which
// consonants it draws below the base before the syllable is reordered, and the
// dotted circle is not a consonant the font has anything to say about. The base
// search picks it up from there.
//
// A face with no U+25CC cannot show one, and the caller checks that first.
func (sh shaper) insertDottedCircle(buf []Glyph, info []indicInfo, start, end, gid int) ([]Glyph, []indicInfo) {
	at := start
	for at < end && at < len(info) && info[at].cat == catRepha {
		at++
	}
	return sh.insertGlyphAt(buf, info, at, gid, indicInfo{cat: catDottedCircle, pos: posEnd})
}

// shapeIndicSyllable puts one syllable into drawing order and applies the
// stages written for its parts, returning the syllable.
//
// wordStart says the character before the syllable in the text ends a word,
// which the word-initial rule below needs and the syllable can no longer say.
// placeholder, when it is not -1, is the dotted circle glyph a broken cluster
// is shown against.
func (sh shaper) shapeIndicSyllable(buf []Glyph, info *[]indicInfo, plan *indicPlan, p *plan,
	wordStart bool, placeholder int) []Glyph {

	hooks := indicHooks(info)
	apply := func(stage []planLookup) {
		buf, _, _ = sh.applyLookups(buf, stage, 0, len(buf), 0, len(buf), hooks)
	}

	// 'locl' and 'ccmp' first, one stage: the one corrects letterforms for the
	// language, the other composes and decomposes, and everything after them is
	// written against what they produce. A script whose letters differ from the
	// shapes Unicode's chart shows — Odia is the case — states nearly all of
	// that difference in 'locl', so a run that skipped it would be set in
	// letters no reader of the language writes.
	//
	// They are applied per syllable, like the stages below, so that neither can
	// join one syllable to the next.
	for s := p.syllables; s < p.basic; s++ {
		apply(p.stage(s))
	}

	// The placeholder for a syllable that is not one goes in after them, where
	// HarfBuzz puts it: those two are written about the characters the text
	// has, not about a glyph the shaper added, and a font whose 'ccmp' composed
	// a dotted circle with a vowel sign would otherwise compose one the text
	// never had. The universal engine's goes in at the same point.
	if placeholder >= 0 {
		buf, *info = sh.insertDottedCircle(buf, *info, 0, len(buf), placeholder)
	}

	// Which consonants the font draws below or after the base, which is what
	// the base search needs and only the font can say. It has to come after
	// 'ccmp', because a consonant that rule composed is the one to ask about.
	plan.refine(buf, *info, 0, len(buf))

	// The base index it reports is not kept: the font may ligate the base away
	// while the features below run, so the final reordering finds it again from
	// the positions rather than from a remembered number.
	sh.indicInitialReorder(buf, *info, plan, 0, len(buf))

	// The basic features, a stage each. A masked one is for the glyphs the
	// reordering marked and starts nowhere else; its context is the syllable.
	for s := p.basic; s < p.after; s++ {
		apply(p.stage(s))
	}

	sh.indicFinalReorder(buf, *info, plan, 0, len(buf))

	// 'init' is for a pre-base matra that opens a word — the i-sign at the
	// start of a word is drawn differently from the same sign mid-word. What
	// counts as a word start is what precedes the syllable in the *text*: a
	// letter or a mark continues a word, a space or a stop does not. The
	// feature itself is applied with the presentation features; this marks the
	// glyph it is for.
	if len(buf) > 0 && (*info)[0].pos == posPreM && wordStart {
		buf[0].mask |= maskInit
	}

	// One cluster for the syllable: its glyphs are no longer in the order its
	// characters are, so the syllable is the smallest piece that can be mapped
	// back to the text at all.
	oneCluster(buf, 0, len(buf))
	return buf
}

// shapeIndicUnordered shapes a syllable the Indic model does not reorder: a
// symbol cluster — an avagraha with the modifiers and cantillation marks
// written on it — or a character of no Indic category. It is put through the
// same stages as every other syllable, but the reordering neither moves its
// glyphs nor marks any of them for a feature, so only the features that are
// for every glyph apply: 'locl', 'ccmp', 'nukt', 'akhn', 'rkrf', 'vatu' and
// 'cjct'. That is HarfBuzz's initial_reordering_syllable, which does nothing
// for these two kinds, over stages that are applied to every syllable alike.
//
// They were passed through untouched, a glyph to a syllable of its own. So
// none of those features reached them, and the presentation features could
// not read a symbol cluster whole: Noto Sans's 'abvs' writes a visarga before
// an udatta, on an avagraha, as a null mark, the udatta and the visarga, and
// it did not apply.
func (sh shaper) shapeIndicUnordered(buf []Glyph, info *[]indicInfo, p *plan) []Glyph {
	hooks := indicHooks(info)
	for s := p.syllables; s < p.after; s++ {
		buf, _, _ = sh.applyLookups(buf, p.stage(s), 0, len(buf), 0, len(buf), hooks)
	}
	return buf
}

// indicWordStart reports whether the character before a syllable ends a word.
//
// The character before the *text*, not before the run. A run is a stretch of
// one face, one direction and one style, and none of those is a word boundary:
// a word split across two of them by a change of colour, or shaped a second
// time to measure where a line may break, opens no new word. The caller says
// what preceded it, and where it says nothing the run's start is the text's.
func indicWordStart(before, runes []rune, at int) bool {
	if at > len(runes) {
		return true
	}
	if at <= 0 {
		if len(before) == 0 {
			return true
		}
		return endsWordForIndic(before[len(before)-1])
	}
	return endsWordForIndic(runes[at-1])
}

// endsWordForIndic reports whether a character closes a word: a space, a stop,
// a digit, a symbol or a control does, and a letter, a mark or a formatting
// character does not.
//
// Nor does a character that says nothing about itself: a private-use one, a
// surrogate, one not yet assigned. That is HarfBuzz's test, which is a range of
// general categories — Cf to Mn in its numbering — that takes those three in
// with the letters and marks, and so a word ends at exactly the categories
// outside it. A private-use character is most often an icon font's glyph set
// among the text, and it is not a space.
func endsWordForIndic(r rune) bool {
	return charprop.Is(r, charprop.N|charprop.P|charprop.S|charprop.Z|charprop.Cc)
}

// indicHooks keep the Indic record in step with a buffer a stage is reshaping.
// The Khmer and Myanmar models keep the same record and use them too.
//
// A ligature that swallows three glyphs into one has to swallow their three
// records too, or every position after it would describe the wrong glyph. The
// record is also what says where the joiners are, since a face commonly gives
// them the same glyph as the space.
func indicHooks(info *[]indicInfo) recordHooks {
	return recordHooks{
		resize: func(at, d int) {
			*info = respliceIndicInfo(*info, at, d)
			// Which glyphs are ligatures, which the pre-base-reordering Ra and
			// the reph turn on. A lookup that shortened the run ligated what it
			// consumed; one that lengthened it took a glyph apart, and the pieces
			// are not ligatures whatever the glyph they came from was.
			switch {
			case d < 0 && at < len(*info):
				(*info)[at].ligated = true
			case d > 0:
				for k := 0; k <= d && at+k < len(*info); k++ {
					(*info)[at+k].ligated = false
				}
			}
		},
		remove: func(at int) {
			if at >= 0 && at < len(*info) {
				*info = append((*info)[:at], (*info)[at+1:]...)
			}
		},
		joiner: func(at int) joinerKind {
			if at < 0 || at >= len(*info) {
				return notJoiner
			}
			switch (*info)[at].cat {
			case catZWJ:
				return joinerZWJ
			case catZWNJ:
				return joinerZWNJ
			}
			return notJoiner
		},
	}
}

// respliceIndicInfo does to the per-glyph record what a lookup did to the
// buffer: a negative delta means glyphs after at were ligated into it, so their
// records go; a positive one means the glyph at at became several, and the new
// ones are the same thing said more than once.
func respliceIndicInfo(info []indicInfo, at, delta int) []indicInfo {
	if at < 0 || at >= len(info) || delta == 0 {
		return info
	}
	if delta < 0 {
		n := -delta
		if at+1+n > len(info) {
			n = len(info) - at - 1
		}
		if n <= 0 {
			return info
		}
		return append(info[:at+1], info[at+1+n:]...)
	}
	out := make([]indicInfo, 0, len(info)+delta)
	out = append(out, info[:at+1]...)
	for k := 0; k < delta; k++ {
		out = append(out, info[at])
	}
	return append(out, info[at+1:]...)
}

// indicPlan is what one run of one script needs that neither the text nor the
// shared model can say: the script's own data, which generation of the
// specification this font was written against, and what the font answers when
// asked about a consonant.
//
// The last of those is what the base search turns on and cannot decide from the
// characters. In त्र — Ta, virama, Ra — the Ra is an ordinary consonant by
// every Unicode property it has, and yet a Devanagari font draws it as a stroke
// under the Ta, which makes the *Ta* the base. What settles it is whether the
// font's below-base, post-base or pre-base-form features cover the consonant
// alongside a virama, which is a question only the font can be asked.
//
// The answers are cached because they are properties of the font, and a page of
// Devanagari asks about the same forty consonants over and over.
type indicPlan struct {
	sh                     shaper
	cfg                    *indicConfig
	oldSpec                bool
	virama                 int
	haveVirama             bool
	blwf, pstf, pref, vatu []int
	cache                  map[int]indicPos
}

func (sh shaper) indicPlan(cfg *indicConfig, oldSpec bool) *indicPlan {
	p := &indicPlan{
		sh:      sh,
		cfg:     cfg,
		oldSpec: oldSpec,
		blwf:    sh.l.featureLookups["blwf"],
		pstf:    sh.l.featureLookups["pstf"],
		pref:    sh.l.featureLookups["pref"],
		vatu:    sh.l.featureLookups["vatu"],
		cache:   map[int]indicPos{},
	}
	p.virama, p.haveVirama = sh.f.GlyphID(cfg.virama)
	return p
}

// refine replaces the place of every consonant in a stretch of the buffer with
// what the font says about it.
func (p *indicPlan) refine(buf []Glyph, info []indicInfo, start, end int) {
	// 'vatu' is among the features asked — see of — so a font stating its
	// below-base forms under it alone has something to ask, and returning here
	// for want of the other three left its consonants all bases.
	if !p.haveVirama || len(p.blwf)+len(p.pstf)+len(p.pref)+len(p.vatu) == 0 {
		return
	}
	for i := start; i < end && i < len(info); i++ {
		if info[i].pos != posBaseC {
			continue
		}
		info[i].pos = p.of(buf[i].GID)
	}
}

func (p *indicPlan) of(gid int) indicPos {
	if pos, ok := p.cache[gid]; ok {
		return pos
	}
	// Both orders are tried. The second-generation rules are written virama
	// first and the first-generation ones consonant first, and enough fonts
	// carry the older lookups under the newer tag that every shaper matches
	// either.
	covers := func(lookups []int) bool {
		return p.sh.wouldSubstitute(lookups, []int{p.virama, gid}, !p.oldSpec) ||
			p.sh.wouldSubstitute(lookups, []int{gid, p.virama}, !p.oldSpec)
	}
	// 'vatu' is asked alongside 'blwf' because it is the other way a font says
	// "this consonant is drawn under the base": the vattu is a below-base Ra, and
	// a font that declares its below-base forms only under 'vatu' means the same
	// thing about them.
	pos := posBaseC
	switch {
	case covers(p.blwf), covers(p.vatu):
		pos = posBelowC
	case covers(p.pstf), covers(p.pref):
		pos = posPostC
	}
	p.cache[gid] = pos
	return pos
}

// indicInitialReorder puts a syllable's characters into the order the font's
// rules are written against, and marks which glyphs each feature is for. It
// returns the index of the base consonant.
//
// This is the first of the two reorderings, and the one that moves characters:
// a pre-base matra written after its consonant is put before it, and an opening
// Ra is marked as the reph it is going to become. The second reordering, after
// the font's rules have run, moves *glyphs* — by then the Ra may be one stroke
// and three consonants may be one conjunct, and where those go depends on what
// the font actually made.
func (sh shaper) indicInitialReorder(buf []Glyph, info []indicInfo, plan *indicPlan, start, end int) int {
	if start >= end {
		return end
	}
	base := end
	hasReph := false
	limit := start

	// Kannada writes the eyelash Ra as Ra, virama, joiner where the model
	// expects Ra, joiner, virama, and enough text does that the specification
	// accepts it. The two are swapped so that the base search sees the sequence
	// it is written against.
	if plan.cfg.swapsRaHalantJoiner && start+3 <= end &&
		info[start].cat == catRa && info[start+1].cat == catHalant &&
		info[start+2].cat == catZWJ {
		buf[start+1], buf[start+2] = buf[start+2], buf[start+1]
		info[start+1], info[start+2] = info[start+2], info[start+1]
	}

	// A syllable opening with Ra + virama, with something after it, draws that
	// Ra as a reph — provided the font has one, which only the font can say.
	//
	// Which sequence asks for it is the script's. Most scripts take any Ra and
	// virama; Telugu writes that pair for something else, so a writer asks for
	// the reph with a joiner after it and the font is asked about all three.
	// Malayalam has a character of its own for the stroke, written before the
	// syllable and drawn at the end of it, so there is nothing to ask the font.
	rphf := sh.l.featureLookups["rphf"]
	switch {
	case plan.cfg.rephMode == rephLogRepha && info[start].cat == catRepha:
		limit++
		for limit < end && indicIsJoiner(info[limit].cat) {
			limit++
		}
		base = start
		hasReph = true

	case len(rphf) > 0 && start+3 <= end && info[start].cat == catRa &&
		indicIsHalant(info[start+1].cat) &&
		(plan.cfg.rephMode == rephImplicit && !indicIsJoiner(info[start+2].cat) ||
			plan.cfg.rephMode == rephExplicit && info[start+2].cat == catZWJ):

		probe := []int{buf[start].GID, buf[start+1].GID}
		if plan.cfg.rephMode == rephExplicit {
			probe = append(probe, buf[start+2].GID)
		}
		if sh.wouldSubstitute(rphf, probe[:2], !plan.oldSpec) ||
			(plan.cfg.rephMode == rephExplicit && sh.wouldSubstitute(rphf, probe, !plan.oldSpec)) {
			limit += 2
			for limit < end && indicIsJoiner(info[limit].cat) {
				limit++
			}
			base = start
			hasReph = true
		}
	}

	// The base is found from the end of the syllable backwards: the first
	// consonant the font does not draw below or after the base, or failing
	// that the first consonant there is. Everything before it is a half form or
	// a conjunct piece; everything after it hangs off it.
	{
		i := end
		seenBelow := false
		for {
			i--
			if indicIsBaseCandidate(info[i].cat) {
				if info[i].pos != posBelowC && (info[i].pos != posPostC || seenBelow) {
					base = i
					break
				}
				if info[i].pos == posBelowC {
					seenBelow = true
				}
				base = i
			} else if start < i && info[i].cat == catZWJ && indicIsHalant(info[i-1].cat) {
				// A joiner written after a virama asks for an explicit half
				// form, which settles the base: the search stops here.
				break
			}
			if i <= limit {
				break
			}
		}
	}
	if hasReph && base == start && limit-base <= 2 {
		// Nothing but the Ra: with no other consonant there is no syllable for
		// a reph to sit above, so the Ra is an ordinary letter and the base.
		hasReph = false
	}

	for i := start; i < base; i++ {
		if info[i].pos > posPreC {
			info[i].pos = posPreC
		}
	}
	if base < end {
		info[base].pos = posBaseC
	}
	// A consonant written after a matra is a final consonant, and is drawn
	// after everything the matra brings with it.
	for i := base + 1; i < end; i++ {
		if !indicIsMatra(info[i].cat) {
			continue
		}
		for j := i + 1; j < end; j++ {
			if indicIsBaseCandidate(info[j].cat) {
				info[j].pos = posFinalC
				break
			}
		}
		break
	}
	if hasReph {
		info[start].pos = posRaToBecomeReph
	}

	// The first-generation rules expected the shaper to move a post-base virama
	// to after the last consonant, and fonts written against them declare their
	// conjunct lookups in that order. Leaving it where it stands sets those
	// conjuncts as loose letters with a visible virama between them.
	//
	// Reports differ on whether a virama already sitting after the last consonant
	// suppresses the move. It is known to for Kannada and known not to for
	// Devanagari, Bengali and Malayalam, so only the script known to want it is
	// given it, and the rest move unconditionally.
	if plan.oldSpec {
		for i := base + 1; i < end; i++ {
			if info[i].cat != catHalant {
				continue
			}
			j := end - 1
			for ; j > i; j-- {
				if indicIsBaseCandidate(info[j].cat) ||
					(plan.cfg.doubleHalantBlocksMove && info[j].cat == catHalant) {
					break
				}
			}
			if info[j].cat != catHalant && j > i {
				moved := info[i]
				copy(info[i:j], info[i+1:j+1])
				info[j] = moved
				g := buf[i]
				copy(buf[i:j], buf[i+1:j+1])
				buf[j] = g
			}
			break
		}
	}

	// A virama, a nukta or a joiner has no place of its own: it belongs to the
	// character before it and has to move with it, or the sort below would
	// strand it among glyphs it says nothing about.
	lastPos := posStart
	for i := start; i < end; i++ {
		if indicIsAttached(info[i].cat) {
			info[i].pos = lastPos
			if indicIsHalant(info[i].cat) && info[i].pos == posPreM {
				// A virama does not travel with a pre-base matra: it belongs to
				// the consonant, which stays where it is. U+092B U+093F U+094D
				// is the case, and every shaper agrees on it.
				for j := i; j > start; j-- {
					if info[j-1].pos != posPreM {
						info[i].pos = info[j-1].pos
						break
					}
				}
			}
		} else if info[i].pos != posSMVD {
			// A modifier written *before* the vowel sign it belongs with — the
			// Gurmukhi bindi and its II sign — goes where the sign goes, rather
			// than to the end with the other modifiers.
			if info[i].cat == catMPst && i > start && indicIsModifier(info[i-1].cat) {
				info[i-1].pos = info[i].pos
			}
			lastPos = info[i].pos
		}
	}
	// A consonant after the base owns whatever lies between it and the last
	// consonant or matra, for the same reason.
	last := base
	for i := base + 1; i < end; i++ {
		if indicIsBaseCandidate(info[i].cat) {
			for j := last + 1; j < i; j++ {
				if info[j].pos < posSMVD {
					info[j].pos = info[i].pos
				}
			}
			last = i
		} else if indicIsMatra(info[i].cat) {
			last = i
		}
	}

	sortIndicByPosition(buf, info, start, end)

	// Find the base again, and turn round a run of more than one pre-base matra
	// while looking.
	//
	// The sort is stable, so two vowel signs written before the same letter come
	// out in the order they were written. They are drawn in the other one: the
	// second is drawn furthest from the letter, in front of the first. It is the
	// same rule the universal engine needs, and the same reading that looked
	// like an off-by-one there — U+0909 U+093F U+094E is the case, and both
	// HarfBuzz and this now answer it the same way round.
	//
	// What is turned round is the matras, not everything among them. A nukta or
	// a virama written after a matra belongs to it and travels with it, so each
	// matra's own run is turned back afterwards.
	base = end
	firstMatra, lastMatra := end, end
	for i := start; i < end; i++ {
		if info[i].pos == posBaseC {
			base = i
			break
		}
		if info[i].pos == posPreM {
			if firstMatra == end {
				firstMatra = i
			}
			lastMatra = i
		}
	}
	if firstMatra < lastMatra {
		reverseIndicRange(buf, info, firstMatra, lastMatra+1)
		at := firstMatra
		for j := at; j <= lastMatra; j++ {
			if indicIsMatra(info[j].cat) {
				reverseIndicRange(buf, info, at, j+1)
				at = j + 1
			}
		}
	}

	// Which feature is for which glyph. The reph is made from the front of the
	// syllable; the half and below-base forms from the consonants before the
	// base; the below-, above- and post-base forms from those after it.
	//
	// Whether the consonants *before* the base are asked for a below-base form
	// is the other half of what the two generations disagree about. The
	// first-generation rules asked for it only after the base; a script whose
	// second-generation rules also say so states blwfPostOnly.
	preBase := maskHalf
	if !plan.oldSpec && plan.cfg.blwfMode == blwfPreAndPost {
		preBase |= maskBlwf
	}
	for i := start; i < end && info[i].pos == posRaToBecomeReph; i++ {
		buf[i].mask |= maskRphf
	}
	for i := start; i < base; i++ {
		buf[i].mask |= preBase
	}
	for i := base + 1; i < end; i++ {
		buf[i].mask |= maskBlwf | maskAbvf | maskPstf
	}

	// The pre-base-reordering Ra: a consonant standing *after* the base that is
	// nonetheless drawn before it. Which consonant that is, is the font's to
	// say and not the script's — Telugu and Kannada have one and Devanagari has
	// none — so the question put here is whether the font's 'pref' rules cover a
	// virama and the consonant after it, anywhere after the base. Only the first
	// such pair in a syllable is one: a syllable has at most one base to be
	// drawn before.
	//
	// Marking it is all that happens now. Whether it *moves* is settled after
	// the feature has run, since a font may decline to make the form.
	if len(plan.pref) > 0 && base+2 < end {
		for i := base + 1; i+1 < end; i++ {
			if sh.wouldSubstitute(plan.pref, []int{buf[i].GID, buf[i+1].GID}, !plan.oldSpec) {
				buf[i].mask |= maskPref
				buf[i+1].mask |= maskPref
				break
			}
		}
	}

	// The eyelash Ra, which only the first-generation Devanagari rules produce.
	// Their below-base forms feature is stated as applying to consonants that
	// follow the base — "the exception is vattu, which may appear below half
	// forms as well as below the base glyph", and Ra is exactly that exception,
	// so a Ra bound by a virama before the base is asked for its below-base form
	// too. A joiner after the virama is the way to ask for the eyelash instead,
	// and is left alone.
	if plan.oldSpec && plan.cfg.hasEyelashRa {
		for i := start; i+1 < base; i++ {
			if info[i].cat == catRa && info[i+1].cat == catHalant &&
				(i+2 == base || info[i+2].cat != catZWJ) {
				buf[i].mask |= maskBlwf
				buf[i+1].mask |= maskBlwf
			}
		}
	}

	// A non-joiner asks for the letters around it *not* to be joined, which for
	// Devanagari means the half form is not to be made.
	for i := start + 1; i < end; i++ {
		if info[i].cat != catZWNJ {
			continue
		}
		for j := i; ; {
			j--
			buf[j].mask &^= maskHalf
			if j <= start || indicIsBaseCandidate(info[j].cat) {
				break
			}
		}
	}
	return base
}

// indicFinalReorder moves glyphs into their drawn positions once the font's
// rules have made whatever forms it has.
//
// It is separate from the first reordering because it can only be done now. A
// reph's final place depends on whether the syllable has an explicit virama
// left in it, and a pre-base matra's on how far the half forms reach — both of
// which are answers the font gave by substituting, or declining to substitute,
// a moment ago.
func (sh shaper) indicFinalReorder(buf []Glyph, info []indicInfo, plan *indicPlan, start, end int) {
	if start >= end {
		return
	}

	// The base again: the font may have ligated it into something else, so it
	// is found from the positions rather than remembered.
	// Whether there is still a pre-base-reordering Ra to move. The marked pair
	// may have been marked and then not formed, and the search below is where
	// that is found out.
	tryPref := len(plan.pref) > 0

	base := start
	for ; base < end; base++ {
		if info[base].pos >= posBaseC {
			// A pair marked as a pre-base-reordering Ra that the font declined
			// to make a form for is not one, and what stands there is an
			// ordinary consonant — which means the base is further on than the
			// search had it. The virama before that consonant is stepped over,
			// since a virama is never a base.
			if tryPref && base+1 < end {
				for i := base + 1; i < end; i++ {
					if buf[i].mask&maskPref == 0 {
						continue
					}
					if !info[i].ligated {
						base = i
						for base < end && indicIsHalant(info[base].cat) {
							base++
						}
						if base < end {
							info[base].pos = posBaseC
						}
						tryPref = false
					}
					break
				}
				if base == end {
					break
				}
			}
			// Malayalam draws no half forms, so a consonant the font declined to
			// give a below-base form to is still a letter and is the base — the
			// search moves on to it rather than stopping at what precedes it.
			if plan.cfg.skipsUnformedBelowForms {
				for i := base + 1; i < end; i++ {
					for i < end && indicIsJoiner(info[i].cat) {
						i++
					}
					if i == end || !indicIsHalant(info[i].cat) {
						break
					}
					i++
					for i < end && indicIsJoiner(info[i].cat) {
						i++
					}
					if i < end && indicIsBaseCandidate(info[i].cat) && info[i].pos == posBelowC {
						base = i
						info[base].pos = posBaseC
					}
				}
			}
			if start < base && info[base].pos > posBaseC {
				base--
			}
			break
		}
	}
	if base == end && start < base && info[base-1].cat == catZWJ {
		base--
	}
	if base < end {
		for start < base && (info[base].cat == catNukta || indicIsHalant(info[base].cat)) {
			base--
		}
	}

	// The pre-base matras were put at the very front of the syllable so that
	// the font's rules would see them there. They belong closer in than that:
	// after the half forms, immediately before the base. Which glyph that is,
	// is the last virama before the base — if the font left one.
	if start+1 < end && start < base {
		newPos := base - 1
		if base == end {
			newPos = base - 2
		}
		// A script with no half forms has nothing for the sign to be moved back
		// past: what its 'half' feature makes is a chillu or a ligated virama,
		// and the sign is drawn after that rather than before it.
		for plan.cfg.hasHalfForms {
			for newPos > start && !indicIsMatra(info[newPos].cat) && !indicIsHalant(info[newPos].cat) {
				newPos--
			}
			if !(indicIsHalant(info[newPos].cat) && info[newPos].pos != posPreM) {
				newPos = start // no virama survived, so nothing moves
				break
			}
			// A joiner after that virama asked for the letters there to be
			// joined, so the sign does not stop at it and the search goes on.
			// A non-joiner is the opposite and stops it — which the syllable cut
			// has already taken care of, a virama and a non-joiner ending a
			// syllable, so any sign after one belongs to the next.
			if newPos+1 < end && info[newPos+1].cat == catZWJ && newPos > start {
				newPos--
				continue
			}
			break
		}
		if start < newPos && info[newPos].pos != posPreM {
			for i := newPos; i > start; i-- {
				if info[i-1].pos != posPreM {
					continue
				}
				old := i - 1
				if old < base && base <= newPos {
					base--
				}
				rotateIndicLeft(buf, info, old, newPos)
				newPos--
			}
		}
	}

	// The reph. It was left at the front through the substitutions, because
	// that is where the font's rule for making it is written; where it is drawn
	// is each script's own answer.
	//
	// It moves only if there is one, and the two ways of having one are
	// opposite tests. A repha written as its own character already is the mark
	// — 'rphf' has nothing to make of it — so it moves unless the font ligated
	// it into something else. A Ra and a virama are a reph only once 'rphf' has
	// made one of them, so that pair moves only if the font did: a font may
	// declare the form generally and block it in context, and a Ra it left as
	// an ordinary letter is an ordinary letter. Moving that pair rotated a bare
	// virama to the front of the syllable and left the consonant where the mark
	// should be, which is not a thing the script writes. The pre-base Ra beside
	// this has had the same test since it was written; the reph had none.
	if start+1 < end && info[start].pos == posRaToBecomeReph &&
		(info[start].cat == catRepha) != info[start].ligated {
		if newPos := indicRephPosition(info, plan, start, end, base); newPos > start {
			if start < base && base <= newPos {
				base--
			}
			rotateIndicLeft(buf, info, start, newPos)
		}
	}

	// The pre-base-reordering Ra. It was left where it was written through the
	// substitutions, because that is where the font's rule for making it is
	// written; it is drawn before the base, in the same place a pre-base vowel
	// sign is drawn — after the half forms, immediately before the base.
	//
	// It moves only if the font actually made the form. A font may declare the
	// pre-base form for a consonant generally and block it in some context, and
	// a Ra it left as an ordinary letter is an ordinary letter: moving it would
	// draw a plain consonant before the base, which is not a thing the script
	// writes.
	if tryPref && base+1 < end {
		for i := base + 1; i < end; i++ {
			if buf[i].mask&maskPref == 0 {
				continue
			}
			if info[i].ligated {
				newPos := base
				// A script with no half forms has nothing for the consonant to
				// be moved back past, exactly as for a pre-base vowel sign.
				if plan.cfg.hasHalfForms {
					for newPos > start && !indicIsMatra(info[newPos-1].cat) &&
						info[newPos-1].cat != catHalant {
						newPos--
					}
				}
				// A joiner after that virama asked for the letters there to be
				// joined, so the consonant is drawn after it.
				if newPos > start && indicIsHalant(info[newPos-1].cat) &&
					newPos < end && indicIsJoiner(info[newPos].cat) {
					newPos++
				}
				if newPos < i {
					rotateIndicRight(buf, info, newPos, i)
					if newPos <= base && base < i {
						base++
					}
				}
			}
			break
		}
	}
}

// rotateIndicRight moves the glyph at from back to to, shifting what lies
// between forward by one. It is the mirror of rotateIndicLeft, and the
// pre-base-reordering Ra is the one thing that travels this way: everything
// else the final reordering moves is drawn later than it was written.
func rotateIndicRight(buf []Glyph, info []indicInfo, to, from int) {
	if to >= from || to < 0 || from >= len(buf) || from >= len(info) {
		return
	}
	g, f := buf[from], info[from]
	copy(buf[to+1:from+1], buf[to:from])
	copy(info[to+1:from+1], info[to:from])
	buf[to], info[to] = g, f
}

// indicRephPosition reports where in a syllable the reph is drawn.
//
// The scripts disagree, and the disagreement is the point of the reph_pos field:
// Oriya and Malayalam draw it straight after the main consonant, Bengali after
// the subjoined forms, Gurmukhi before them, Devanagari and Gujarati before the
// post-base forms, and Tamil, Telugu and Kannada after everything.
//
// The steps below are the specification's, in its order. Reading them:
//
//   - A script that draws the reph after everything skips straight to the last
//     two, since no earlier place can apply to it.
//   - Otherwise the first explicit virama still standing between the reph and
//     the base takes it: that is where a half form ends and the reph can sit on
//     what the half form made. This is the usual answer for a modern font.
//   - Failing that, the script's own class decides: after the main consonant, or
//     after the subjoined forms, whichever it states.
//   - Failing that, the end of the syllable — but inside the modifiers, which
//     are drawn last of all. An anusvara is drawn over the syllable and the reph
//     belongs under it, and a font commonly has one glyph for the two together
//     which it can only make if they are in that order.
func indicRephPosition(info []indicInfo, plan *indicPlan, start, end, base int) int {
	// The first virama still standing before the base. Two of the steps want it,
	// so it is asked for once.
	afterFirstHalant := func() (int, bool) {
		at := start + 1
		for at < base && !indicIsHalant(info[at].cat) {
			at++
		}
		if at >= base || !indicIsHalant(info[at].cat) {
			return 0, false
		}
		// A joiner after that virama belongs with it, and the reph goes past
		// both — the joiner asked for the form the reph is to sit on.
		if at+1 < base && indicIsJoiner(info[at+1].cat) {
			at++
		}
		return at, true
	}

	if plan.cfg.rephPos != posAfterPost {
		if at, ok := afterFirstHalant(); ok {
			return at
		}
		switch plan.cfg.rephPos {
		case posAfterMain:
			at := base
			for at+1 < end && info[at+1].pos <= posAfterMain {
				at++
			}
			return at
		case posAfterSub:
			at := base
			for at+1 < end && !indicAfterReph(info[at+1].pos) {
				at++
			}
			return at
		}
	} else if at, ok := afterFirstHalant(); ok {
		return at
	}

	// The end of the syllable, before the modifiers.
	at := end - 1
	for at > start && info[at].pos == posSMVD {
		at--
	}
	// A reph that would land after a matra and its virama goes before that
	// virama instead, so that it can combine with the matra. A plain consonant
	// and virama are not this case.
	if indicIsHalant(info[at].cat) {
		for i := base + 1; i < at; i++ {
			if indicIsMatra(info[i].cat) {
				at--
			}
		}
	}
	return at
}

// indicAfterReph reports whether a position is one the reph must be drawn
// before, which is what decides where it stops when no virama placed it.
func indicAfterReph(p indicPos) bool {
	return p == posPostC || p == posAfterPost || p == posSMVD
}

// reverseIndicRange turns a stretch of the buffer round, carrying the per-glyph
// record with it.
func reverseIndicRange(buf []Glyph, info []indicInfo, start, end int) {
	if start < 0 || end > len(buf) || end > len(info) {
		return
	}
	for i, j := start, end-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
		info[i], info[j] = info[j], info[i]
	}
}

// sortIndicByPosition puts a syllable into drawing order. The sort is stable
// and by insertion: a syllable is a handful of glyphs, and stability is not an
// optimisation here but the definition — two glyphs in the same position keep
// the order they were written in, which is what makes a run of consonants
// before the base stay a run rather than a shuffle.
func sortIndicByPosition(buf []Glyph, info []indicInfo, start, end int) {
	for i := start + 1; i < end; i++ {
		g, f := buf[i], info[i]
		j := i
		for j > start && info[j-1].pos > f.pos {
			buf[j], info[j] = buf[j-1], info[j-1]
			j--
		}
		buf[j], info[j] = g, f
	}
}

// rotateIndicLeft moves the glyph at from to to, shifting what lies between
// back by one. It is how both reorderings move a single glyph.
func rotateIndicLeft(buf []Glyph, info []indicInfo, from, to int) {
	if from >= to || from < 0 || to >= len(buf) || to >= len(info) {
		return
	}
	g, f := buf[from], info[from]
	copy(buf[from:to], buf[from+1:to+1])
	copy(info[from:to], info[from+1:to+1])
	buf[to], info[to] = g, f
}
