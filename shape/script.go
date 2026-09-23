package shape

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/font"
)

// Choosing which of a font's rules apply to a run of text.
//
// A font that covers several scripts states different rules for each, and says
// so: its GSUB and GPOS tables begin with a ScriptList, which names each script
// the font covers, the language systems within it, and the features each of
// those selects. Reading the features without reading that list gives a Greek
// word the rules written for Arabic — which is what this package used to do, and
// what this file exists to stop.
//
// # Three questions, answered separately
//
// Which script is the text in? That is Unicode's Script property, generated
// into scripts.go from the UCD. Common and Inherited characters — a space, a
// digit, a combining accent — are not text in a script of their own and take
// the script of what they are written among.
//
// What does OpenType call that script? A four-byte tag, derived from the ISO
// 15924 code, also in scripts.go. Some scripts have two, and both are tried.
//
// What does the font select for it? The ScriptList walk below. A script that
// the font does not declare falls back to 'DFLT', the conventional tag for "any
// script"; a font with no ScriptList at all, or one that declares nothing this
// run can use, falls back to taking every feature — the behaviour this package
// had before, so that no font that worked stops working.
//
// # Language
//
// A language system narrows a script's selection further, and its *required*
// feature applies whether or not anything asked for it. Which language a run is
// in cannot be read off its characters: the document says it, and it reaches
// here as Features.Language, a BCP 47 tag that language.go turns into the
// language system tags to look for.

// defaultScriptTags are tried, in order, after the run's own script.
//
// 'DFLT' is the registered tag for "whatever script this is". 'dflt' is what a
// font written by a tool that got the case wrong carries, and enough of them
// exist that every shaper accepts it. 'latn' is the last resort, and is there
// for the same reason every shaper has it: a great many fonts state all their
// features under Latin and nowhere else, meaning them generally, and a reader
// that stopped at 'dflt' would set their text with no features at all.
var defaultScriptTags = []string{"DFLT", "dflt", "latn"}

// noRequiredFeature is the value a language system uses to say it has no
// feature that applies unconditionally.
const noRequiredFeature = 0xFFFF

// scriptOf reports the Unicode script of a character.
func scriptOf(r rune) uint16 {
	i := sort.Search(len(scriptRanges), func(i int) bool { return scriptRanges[i].hi >= r })
	if i < len(scriptRanges) && r >= scriptRanges[i].lo {
		return scriptRanges[i].script
	}
	return scriptUnknown
}

// decides reports whether a script settles what a run is written in. Common,
// Inherited and Unknown do not: a run of digits, or an accent on its own, is
// written in whatever surrounds it.
func decides(script uint16) bool {
	return script != scriptCommon && script != scriptInherited && script != scriptUnknown
}

// runScript reports the script of a run of text: the first one that decides
// anything, or scriptUnknown if none does.
//
// The first rather than the most common, because a run it is asked about is in
// one script: Stack.ShapeRuns cuts its runs where the script changes, and so
// does a Face — see scriptRuns.
func runScript(s string) uint16 {
	for _, r := range s {
		if sc := scriptOf(r); decides(sc) {
			return sc
		}
	}
	return scriptUnknown
}

// scriptRun is a stretch of a string set in one script: the bytes [start, end).
type scriptRun struct {
	start, end int
	script     uint16
}

// scriptRuns cuts a string where its script changes, appending the pieces to
// out.
//
// A font states its rules per script, and a string that changes script part
// way through is two runs of rules: "PDFकि" shaped as one run is shaped as
// Latin, because Latin is what it opens with, and the vowel sign that is
// written after its consonant and drawn before it is left where it was
// written. Stack.ShapeRuns has always cut there. A caller shaping through a
// Face directly — which is what layout does, with a face that covers both
// scripts — got the one run, so a Latin acronym with a Hindi suffix and no space
// between them was set wrong by any font covering both, Noto Sans among them.
//
// The unit is a character and the combining marks after it, and no cut falls
// inside one: a mark belongs to what it is written on, whatever script the mark
// is from. A unit whose characters decide no script — a space, a digit,
// punctuation — belongs to the script before it: at the start of the string,
// behind, the script of the text before the string where the caller has some,
// and failing that the first script after it in the string, and failing that
// ahead, the script of the text after the string. So a span holding only "：",
// between Chinese text and more of it, is Chinese, as it is when the same
// characters are one string; set alone as Common it was read under 'DFLT', and
// a font's Chinese forms are not stated there.
//
// A character used in more than one script continues a run in any of them —
// UAX #24's Script_Extensions. A Devanagari digit in Kaithi, a Tamil digit in
// Grantha, an Arabic full stop in Hanifi Rohingya, a Vedic sign in Tamil: each
// has one Script and is written in the others, and cutting there sets a
// number, or a word's last sign, apart from the run it is part of. So a run
// carries the scripts it could still be — the extensions of what it opened
// with, narrowed by each character after — and is cut only where a character
// shares none of them. It is set in the one it opened with where that is still
// among them, and otherwise the first that is. And two scripts the font is
// asked about under the same OpenType tags — Hiragana and Katakana, both
// 'kana' — are one script here, since nothing the font says can tell them
// apart.
//
// Stack.ShapeRuns cuts its runs here too. It reads each character once.
func scriptRuns(s string, behind, ahead uint16, out []scriptRun) []scriptRun {
	var run scriptCandidates
	start := 0
	// Characters at the start that decide nothing belong to the text before
	// the string, where there is some: the run is open in its script before
	// anything in the string has said a word.
	borrowed := decides(behind)
	if borrowed {
		run.open(behind, nil)
	}
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		unit, first := scriptOf(r), r
		j := i + n
		for j < len(s) {
			m, n := utf8.DecodeRuneInString(s[j:])
			if !unicode.Is(unicode.M, m) {
				break
			}
			if !decides(unit) {
				unit, first = scriptOf(m), m
			}
			j += n
		}
		if decides(unit) {
			switch ext := scriptExtensionsOf(first); {
			case run.n == 0:
				run.open(unit, ext)
			case borrowed && i == start:
				// Nothing before this in the string, so nothing was set in the
				// script borrowed from before it.
				run.open(unit, ext)
			case !run.narrow(unit, ext):
				out = append(out, scriptRun{start: start, end: i, script: run.script()})
				start = i
				run.open(unit, ext)
			}
			borrowed = false
		}
		i = j
	}
	script := ahead
	if run.n > 0 {
		script = run.script()
	}
	return append(out, scriptRun{start: start, end: len(s), script: script})
}

// scriptCandidates is the scripts a run could still be in, and the one it
// opened with. It is an array rather than a slice so that cutting a string
// allocates nothing.
type scriptCandidates struct {
	opened uint16
	n      int
	s      [maxScriptExtensions]uint16
}

// open starts a run at a character of script sc used in the scripts ext, or in
// sc alone where ext is empty.
func (c *scriptCandidates) open(sc uint16, ext []uint16) {
	c.opened = sc
	if len(ext) == 0 {
		c.s[0], c.n = sc, 1
		return
	}
	c.n = copy(c.s[:], ext)
}

// narrow keeps the candidates the next character is also used in, and
// reports false — changing nothing — where it is used in none of them.
func (c *scriptCandidates) narrow(sc uint16, ext []uint16) bool {
	if len(ext) == 0 {
		ext = []uint16{sc}
	}
	var kept [maxScriptExtensions]uint16
	k := 0
	for _, have := range c.s[:c.n] {
		for _, e := range ext {
			if sameShaping(have, e) {
				kept[k] = have
				k++
				break
			}
		}
	}
	if k == 0 {
		return false
	}
	c.s, c.n = kept, k
	return true
}

// script is the one the run is set in: the one it opened with, where that is
// still a candidate, and otherwise the first that is.
func (c *scriptCandidates) script() uint16 {
	for _, have := range c.s[:c.n] {
		if have == c.opened {
			return have
		}
	}
	return c.s[0]
}

// scriptExtensionsOf is the scripts a character is used in besides — or
// rather than only — its Script, or nothing for one used in its Script alone.
func scriptExtensionsOf(r rune) []uint16 {
	i := sort.Search(len(scriptExtensionRanges), func(i int) bool { return scriptExtensionRanges[i].hi >= r })
	if i < len(scriptExtensionRanges) && r >= scriptExtensionRanges[i].lo {
		return scriptExtensionRanges[i].scripts
	}
	return nil
}

// sameShaping reports whether two scripts are one run to a font: the same
// script, or two the font can only be asked about under the same tags.
func sameShaping(a, b uint16) bool {
	if a == b {
		return true
	}
	ta, tb := scriptTags(a), scriptTags(b)
	if len(ta) == 0 || len(ta) != len(tb) {
		return false
	}
	for i := range ta {
		if ta[i] != tb[i] {
			return false
		}
	}
	return true
}

// scriptsBeside is the script of the text on either side of each piece of a
// string — the pieces given as byte ranges that do not overlap — for scriptRuns
// to give a piece's characters that decide nothing: behind is the last
// character before the piece that decides a script, and ahead the first after
// it. Where the string has none on that side, the caller's before or after
// answers, which is the script of the text the string was taken from.
//
// A bidirectional run is cut by *direction*, and a run of digits inside Arabic
// is a run of its own: Arabic-Indic digits are class AN and the letters around
// them are AL, so the digits come out as a piece whose every character is
// Common. Taken alone, that piece selects DFLT — so a font stating its digit
// forms under 'arab', as Arabic fonts do, had them selected away by the
// direction the digits are read in.
//
// UAX #24's resolution: a character of Common or Inherited takes the script
// around it. Backwards first, because a run of digits belongs to the word it
// follows, and forwards where there is nothing behind.
//
// The pieces are answered together. Answering one at a time walked everything
// before the piece to find the last character there that decides a script, and
// a bidirectional run is cut into a piece per stretch of digits: "ب1" sixteen
// thousand times over is sixteen thousand pieces, and it took four and a half
// seconds, three quarters of it here. Taken in the order they stand in the
// string, the pieces share one walk forward for the script behind each and one
// walk back for the script ahead, so each character is read a bounded number
// of times however many pieces there are.
func scriptsBeside(s string, pieces [][2]int, before, after uint16) (behind, ahead []uint16) {
	behind, ahead = make([]uint16, len(pieces)), make([]uint16, len(pieces))
	order := make([]int, len(pieces))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return pieces[order[a]][0] < pieces[order[b]][0] })

	pos, last := 0, before
	for _, k := range order {
		for pos < pieces[k][0] {
			r, size := utf8.DecodeRuneInString(s[pos:])
			if sc := scriptOf(r); decides(sc) {
				last = sc
			}
			pos += size
		}
		behind[k] = last
	}
	pos, next := len(s), after
	for i := len(order) - 1; i >= 0; i-- {
		k := order[i]
		for pos > pieces[k][1] {
			r, size := utf8.DecodeLastRuneInString(s[:pos])
			if sc := scriptOf(r); decides(sc) {
				next = sc
			}
			pos -= size
		}
		ahead[k] = next
	}
	return behind, ahead
}

// scriptBehind is the script of the text before a run, the caller's context:
// the last character there that decides one. scriptAhead is the first after
// it.
//
// Each reads at most contextRunes characters, which is what a caller hands a
// run of the text beside it anyway, and a caller may hand it the whole of a
// paragraph. A run separated from the nearest letter by more punctuation or
// digits than that is shaped as though nothing were beside it.
func scriptBehind(before string) uint16 {
	for n := 0; before != "" && n < contextRunes; n++ {
		r, size := utf8.DecodeLastRuneInString(before)
		if sc := scriptOf(r); decides(sc) {
			return sc
		}
		before = before[:len(before)-size]
	}
	return scriptUnknown
}

func scriptAhead(after string) uint16 {
	n := 0
	for _, r := range after {
		if n == contextRunes {
			break
		}
		if sc := scriptOf(r); decides(sc) {
			return sc
		}
		n++
	}
	return scriptUnknown
}

// scriptTags is the OpenType tags a script selects, most specific first.
func scriptTags(script uint16) []string {
	if int(script) < len(scriptOpenTypeTags) {
		return scriptOpenTypeTags[script]
	}
	return nil
}

// scriptFeatures returns the FeatureList indices the given script tags and
// language select in a GSUB or GPOS table, and whether the table settled the
// question at all.
//
// A false second result means the table declares no scripts, or none that
// matched even the default — so there is nothing to select by and the caller
// should take every feature.
func scriptFeatures(t []byte, tags, langs []string) (featureSet, bool) {
	sel, ok := scriptSelection(t, tags, langs)
	return sel.features, ok
}

// selection is what one language system of a script selects: its features,
// and which of them is the required one.
type selection struct {
	features featureSet
	// required is the FeatureList index of the language system's required
	// feature, or noRequiredFeature. It is among features as well; it is named
	// apart because it applies whether or not a shaper asks for its tag.
	required int
}

// scriptSelection is scriptFeatures, also reporting the required feature.
func scriptSelection(t []byte, tags, langs []string) (selection, bool) {
	if len(t) < 10 {
		return selection{required: noRequiredFeature}, false
	}
	off := font.Be16(t, 4)
	if off <= 0 || off+2 > len(t) {
		return selection{required: noRequiredFeature}, false
	}
	list := t[off:]
	byTag := scriptOffsets(list)
	for _, tag := range append(append(make([]string, 0, len(tags)+2), tags...), defaultScriptTags...) {
		so, ok := byTag[tag]
		if !ok {
			continue
		}
		ls, ok := readLangSys(list[so:], langs)
		if !ok {
			continue
		}
		sel := make(featureSet, len(ls.features)+1)
		// The required feature applies whether or not anything asked for it,
		// which is the whole of what "required" means here.
		if ls.required != noRequiredFeature {
			sel[ls.required] = true
		}
		for _, i := range ls.features {
			sel[i] = true
		}
		return selection{features: sel, required: ls.required}, true
	}
	return selection{required: noRequiredFeature}, false
}

// layoutTags is the script tags a run's script is looked up under, most
// specific first: the generated table's, with the third-generation tag of each
// Indic script ahead of its second-generation one.
//
// The third-generation tags are what a font says when its Indic script is
// written for the universal engine rather than the Indic model, and HarfBuzz
// tries them first — so a font declaring 'dev3' beside 'dev2' is read under
// 'dev3' and set by the universal engine. Myanmar's 'mym2' has no third
// generation.
func layoutTags(script uint16) []string {
	tags := scriptTags(script)
	if len(tags) == 0 || len(tags[0]) != 4 || tags[0][3] != '2' || tags[0] == "mym2" {
		return tags
	}
	out := make([]string, 0, len(tags)+1)
	out = append(out, tags[0][:3]+"3")
	return append(out, tags...)
}

// chosenScriptTag reports which of a script's tags the font's substitutions were
// actually read under — the same tag scriptFeatures settled on, by the same walk.
//
// A caller needs it when the tag itself carries meaning beyond selection. The
// Indic scripts are the case: a font declaring 'deva' rather than 'dev2' was
// written against the first-generation specification and means its rules, and
// nothing but the tag says so. And which model sets a run at all turns on it —
// see categorize.
//
// A font whose GSUB declares nothing this run can use, or that has no GSUB,
// gets "": it chose no tag, which is not the same as choosing 'DFLT'.
//
// Every run asks, since the answer decides the run's model, so it is kept
// beside the layout the same question selects: walking the ScriptList again
// per run is what the layout cache was made to stop.
//
// The language is asked about too, because a script table the run's language
// system is not in and that has no default one is passed over for the next
// tag — so which tag is chosen can turn on it.
func (f *Face) chosenScriptTag(script uint16, langs []string) string {
	if f.cache == nil {
		return f.readChosenScriptTag(script, langs)
	}
	key := languageKey(langs)
	if tag, ok := f.cache.chosenFor(script, key); ok {
		return tag
	}
	tag := f.readChosenScriptTag(script, langs)
	f.cache.rememberChosen(script, key, tag)
	return tag
}

func (f *Face) readChosenScriptTag(script uint16, langs []string) string {
	list := scriptList(f.layoutTables["GSUB"])
	if len(list) == 0 {
		return ""
	}
	tags := layoutTags(script)
	byTag := scriptOffsets(list)
	for _, tag := range append(append(make([]string, 0, len(tags)+len(defaultScriptTags)), tags...), defaultScriptTags...) {
		so, ok := byTag[tag]
		if !ok {
			continue
		}
		if _, ok := readLangSys(list[so:], langs); ok {
			return tag
		}
	}
	return ""
}

// scriptOffsets maps each script tag a ScriptList names to its Script table's
// offset within the list. A tag declared twice keeps its first table, which is
// the one a reader walking the list in order would find.
func scriptOffsets(list []byte) map[string]int {
	n := font.Be16(list, 0)
	if n > maxScripts {
		n = maxScripts
	}
	out := make(map[string]int, n)
	for i := 0; i < n; i++ {
		rec := 2 + 6*i
		if rec+6 > len(list) {
			break
		}
		tag := string(list[rec : rec+4])
		off := font.Be16(list, rec+4)
		if off <= 0 || off+4 > len(list) {
			continue
		}
		if _, dup := out[tag]; !dup {
			out[tag] = off
		}
	}
	return out
}

// langSys is what one language system of a script selects.
type langSys struct {
	// required is the feature that applies unconditionally, or
	// noRequiredFeature.
	required int
	features []int
}

// readLangSys reads a Script table's language system: the first of langs the
// script declares, and failing that the default.
//
// "The default" is two things, tried in HarfBuzz's order. A LangSysRecord
// tagged 'dflt' is not in the specification — the default is the Script
// table's DefaultLangSys — but the registry's own page once spelled it that
// way, enough fonts carry one that HarfBuzz looks for it first, and a font is
// tested against HarfBuzz. Then DefaultLangSys. A run with no language, or one the font does not name, is set
// in whichever it finds.
//
// A script that declares neither is unusable, and reports so rather than
// selecting nothing — the caller then tries the next script tag, and failing
// that falls back to taking every feature. Selecting nothing would set the text
// with no ligatures, no kerning and no joining at all, which is a worse answer
// than the one this package gave before it read scripts.
func readLangSys(script []byte, langs []string) (langSys, bool) {
	if len(script) < 4 {
		return langSys{}, false
	}
	off := 0
	for _, lang := range langs {
		if off = langSysOffset(script, lang); off != 0 {
			break
		}
	}
	if off == 0 {
		off = langSysOffset(script, "dflt")
	}
	if off == 0 {
		off = font.Be16(script, 0) // DefaultLangSys
	}
	if off <= 0 || off+6 > len(script) {
		return langSys{}, false
	}
	ls := script[off:]
	out := langSys{required: font.Be16(ls, 2)}
	n := font.Be16(ls, 4)
	if n > maxDeclaredList {
		n = maxDeclaredList
	}
	for i := 0; i < n; i++ {
		if 6+2*i+2 > len(ls) {
			break
		}
		out.features = append(out.features, font.Be16(ls, 6+2*i))
	}
	return out, true
}

// langSysOffset is the offset of the language system a Script table names
// with a tag, or zero where it names none. A tag named twice is the first
// record's, which is the one a reader walking the list in order finds.
func langSysOffset(script []byte, tag string) int {
	n := font.Be16(script, 2)
	if n > maxLangSys {
		n = maxLangSys
	}
	for i := 0; i < n; i++ {
		rec := 4 + 6*i
		if rec+6 > len(script) {
			break
		}
		if string(script[rec:rec+4]) == tag {
			return font.Be16(script, rec+4)
		}
	}
	return 0
}

// Scripts lists the OpenType script tags the face declares layout rules for,
// in sorted order — "latn", "cyrl", "deva" and so on, plus "DFLT" where the
// font names a default.
//
// It is the question a caller asks when assembling a fallback stack: a face may
// have the *glyphs* for a script and none of the rules that make it legible,
// and for Devanagari or Arabic the difference between the two is a row of
// unjoined letters. Covers answers the first question; this answers the second.
//
// A face whose tables name no scripts at all returns nothing, which is not the
// same as covering nothing: such a font's features apply to everything.
func (f *Face) Scripts() []string {
	seen := map[string]bool{}
	for _, table := range []string{"GSUB", "GPOS"} {
		for tag := range scriptOffsets(scriptList(f.layoutTables[table])) {
			seen[tag] = true
		}
	}
	out := make([]string, 0, len(seen))
	for tag := range seen {
		out = append(out, tag)
	}
	sortStrings(out)
	return out
}

// HasScript reports whether the face declares layout rules for a script tag.
func (f *Face) HasScript(tag string) bool {
	for _, table := range []string{"GSUB", "GPOS"} {
		if _, ok := scriptOffsets(scriptList(f.layoutTables[table]))[tag]; ok {
			return true
		}
	}
	return false
}

// scriptList is the ScriptList of a GSUB or GPOS table, which is where the
// script tags live — the table itself only points at it.
//
// It returns nothing rather than the table when the offset is missing or out of
// range, so a caller reading tags from a malformed font finds none rather than
// reading the header as though it were a list of tags.
func scriptList(t []byte) []byte {
	if len(t) < 10 {
		return nil
	}
	off := font.Be16(t, 4)
	if off <= 0 || off+2 > len(t) {
		return nil
	}
	return t[off:]
}

// shaper is a face together with the rules that apply to the run being shaped.
//
// The two are separate because a face has more than one set of rules: one per
// script it covers, and per language within that. Everything that reads the
// layout tables hangs off this rather than off the face, so that it cannot
// reach the wrong one — a Face has no single layout to reach for.
type shaper struct {
	f *Face
	l *layout

	// rtl says the run will be drawn right to left.
	//
	// Positioning needs it because it states where a glyph sits relative to the
	// pen, and the pen will meet the run's glyphs in the opposite order.
	// Substitution needs it for one thing only: which of 'rtlm'/'rtla' and
	// 'ltrm'/'ltra' a font's direction-selected forms are read from. Everything
	// else in substitution works in the order the text is written and is the
	// same either way.
	rtl bool

	// zeroMarks says when a mark's own advance is cancelled, which each script's
	// model decides for itself — see position.go.
	zeroMarks zeroMarkWidths

	// features is what the document turned off: the font's own rules a CSS
	// property or a CSS Text rule has overruled. See Features.
	features Features

	// langs is the language system tags the run's language is looked up
	// under, most specific first: features.Language, read once per run. See
	// language.go.
	langs []string

	// floor and limit bound the glyphs a lookup may look at: it may not match,
	// or backtrack, outside [floor, limit). They exist for the Indic pass,
	// which applies a font's features one syllable at a time — a ligature
	// reaching into the next syllable would join glyphs the font never meant to
	// see together. A zero limit means the whole buffer, which is what every
	// other script gets.
	floor, limit int

	// onResize, when set, is told wherever a lookup changes the buffer's
	// length: at which position, and by how much. The Indic pass sets it to
	// keep its per-glyph record — categories, positions, feature masks — in
	// step with a buffer that ligatures and decompositions are reshaping under
	// it. Nothing else needs it, and it is nil everywhere else.
	onResize func(at, delta int)

	// onDelete, when set, is told that the glyph at a position has been taken
	// out of the buffer altogether.
	//
	// It is separate from onResize because the two mean different things about
	// the *record* at that position. A lookup that shortens a run has ligated
	// what it consumed, so the record at the position survives and describes
	// what the ligature became; a deletion leaves nothing there, so the record
	// has to go with the glyph. Told through onResize, a deletion would keep the
	// vanished glyph's record and hang it on whatever moved up into its place.
	onDelete func(at int)

	// joinerAt, when set, says whether a join control stands at a buffer
	// position, so that a lookup can step over one — see ignorable.go. It is a
	// function of the position rather than of the glyph because a face commonly
	// gives a join control the same glyph as the space.
	//
	// A nil one means no position holds a joiner, which is true of every run
	// this package shapes outside the Indic pass: there the joiners are taken
	// out before any lookup runs.
	joinerAt func(at int) joinerKind

	// manualZWJ says the lookup being applied asked to see a zero width joiner
	// in its input rather than have it stepped over, and manualZWNJ says it
	// asked to see a non-joiner in its context. The Indic features ask for both,
	// because a joiner is written precisely to force or forbid the forms they
	// make; the Myanmar, universal and Arabic features ask for the first alone.
	// See stepsOverJoiner, and plan.go for where each lookup's come from.
	manualZWJ, manualZWNJ bool

	// lookupMask is the glyphs the lookup being applied is for, or zero for
	// every glyph: a lookup of a masked feature starts only at a glyph carrying
	// its bit and matches its input only over such glyphs. See glyphMask.
	lookupMask glyphMask

	// ops is what is left of the run's allowance for applying one lookup from
	// inside another. It is a pointer because a shaper is copied per lookup and
	// the allowance belongs to the run, not to a lookup: a rule that names
	// forty lookups which each name it again would otherwise be bounded only by
	// the recursion depth, and eight levels of forty is a hang from a few
	// hundred bytes of font. See lookupBudget.
	ops *int

	// markSet is the mark glyph set the lookup being applied names, or -1. It
	// travels on the shaper rather than through every matcher's arguments
	// because it belongs to the lookup, and a shaper is copied per lookup — so
	// a nested one gets its own and cannot leak it back out.
	markSet int

	// positioning says the contextual matching below is running for GPOS rather
	// than GSUB, so that a rule which matched applies a *positioning* lookup.
	// The matching is identical for the two — the subtable formats are the same
	// — and only what a matched rule then does differs, which is why one set of
	// matchers serves both.
	positioning bool

	// run is the array a substitution pass is editing, or nil where nothing is
	// being edited — positioning, and the probes that ask whether a lookup
	// would do anything.
	//
	// It is a pointer because every method here takes a shaper by value: the
	// pass's own cursors have to be the same cursors in a lookup that a
	// contextual rule called into. See runBuf, which is where the arrangement
	// is written down.
	run *runBuf

	// ligIDs hands out the numbers that tie a ligature glyph to the marks that
	// were inside it, so that positioning can put each mark against the part of
	// the ligature it belongs to. It is a pointer because a shaper is copied
	// freely — every method takes it by value — and the numbers have to stay
	// distinct across the whole run rather than per copy.
	//
	// A nil one means nothing is tracking ligatures, which is what the paths
	// that only measure get: they do not position marks, so there is nothing
	// for the number to be used by.
	ligIDs *int
}

// nextLigatureID hands out the next number tying a ligature to its marks.
//
// Zero is never handed out: it is what a glyph that has nothing to do with any
// ligature carries, and a real ligature must not be mistaken for one.
func (sh shaper) nextLigatureID() int {
	if sh.ligIDs == nil {
		return 0
	}
	*sh.ligIDs++
	return *sh.ligIDs
}

// base is where the glyphs a lookup sees begin, counted from the start of the
// run.
//
// A lookup's positions are relative to the pending part of the buffer, and
// everything a pass keeps *beside* the buffer — the per-glyph record, the two
// bounds, where the joiners are — is counted from the start of the run. This is
// what maps one to the other, and it is zero wherever no pass is editing.
func (sh shaper) base() int {
	if sh.run == nil {
		return 0
	}
	return sh.run.w
}

// glyphAt reads a position a lookup is working with, which may be behind the
// glyphs it was given: a backtrack walks off the front of them into what the
// pass has already settled. See runBuf.
func (sh shaper) glyphAt(buf []Glyph, at int) Glyph {
	if at >= 0 {
		return buf[at]
	}
	settled := sh.run.settled()
	return settled[len(settled)+at]
}

// product is an empty slice with room for n glyphs, for a substitution to build
// its replacement in before replace writes it.
func (sh shaper) product(n int) []Glyph {
	if sh.run == nil {
		return make([]Glyph, 0, n)
	}
	return sh.run.product(n)
}

// replace puts product where the span glyphs at buf[at:at+span] were, and
// reports the glyphs the lookup is working with as they now stand.
//
// It is the one place a substitution changes how many glyphs there are. See
// runBuf for why that is not a new slice built out of the three parts.
func (sh shaper) replace(buf []Glyph, at, span int, product []Glyph) []Glyph {
	if sh.run == nil {
		// No pass is editing, so there is no array to edit in place and no
		// promise about whose it is. A copy is what this used to do everywhere.
		sh.run = newRunBuf(append([]Glyph(nil), buf...), 0)
	}
	return sh.run.replace(at, span, product)
}

// settledRun is what the pass has finished with, and nothing where no pass is
// editing.
func (sh shaper) settledRun() []Glyph {
	if sh.run == nil {
		return nil
	}
	return sh.run.settled()
}

// resized reports a change in the buffer's length at a position, for a caller
// keeping something in step with it.
func (sh shaper) resized(at, delta int) {
	if sh.onResize != nil && delta != 0 {
		sh.onResize(sh.base()+at, delta)
	}
}

// deleted tells a per-glyph record that the glyph at a position is gone.
func (sh shaper) deleted(at int) {
	if sh.onDelete != nil {
		sh.onDelete(sh.base() + at)
	}
}

// end is one past the last glyph a lookup may look at.
func (sh shaper) end(buf []Glyph) int {
	if sh.limit > 0 {
		if n := sh.limit - sh.base(); n < len(buf) {
			if n < 0 {
				return 0
			}
			return n
		}
	}
	return len(buf)
}

// layoutFor reads the font's layout tables as the given script selects them,
// caching the result.
//
// The caches are keyed by what was *selected* rather than by the script, so two
// scripts a font treats identically — the common case, a face declaring the
// same features for 'latn', 'grek' and 'cyrl' — share one reading of tables
// that run to tens of kilobytes. The positioning half is cached on its own key,
// because a font that varies its substitutions per script usually does not vary
// its kerning, and the kerning is the large table.
func (f *Face) layoutFor(script uint16, langs []string) *layout {
	if len(f.layoutTables) == 0 {
		return f.layout
	}
	// Which features a script selects is a fact about the font, and answering it
	// means walking the ScriptList, reading a LangSys and building the two keys
	// below. That was half the memory a shaped word cost, spent every time, to
	// arrive at a cached answer — so the script and the language are the first
	// key, and the selection is worked out only when it is not yet known.
	key := languageKey(langs)
	if l, ok := f.cache.forScript(script, key); ok {
		return l
	}
	l := f.readLayoutFor(script, langs)
	f.cache.rememberScript(script, key, l)
	return l
}

func (f *Face) readLayoutFor(script uint16, langs []string) *layout {
	tags := layoutTags(script)
	gsub, gsubOK := scriptSelection(f.layoutTables["GSUB"], tags, langs)
	gposSel, gposOK := scriptFeatures(f.layoutTables["GPOS"], tags, langs)
	if !gsubOK && !gposOK {
		// Neither table says anything about scripts, so there is nothing to
		// select by: every feature applies, which is what f.layout already is.
		return f.layout
	}
	gsubKey, gposKey := selectionKey(gsub.features), selectionKey(gposSel)
	if gsub.required != noRequiredFeature {
		// The same features with a different one required are a different
		// selection: the required one applies whether or not it is asked for.
		gsubKey += "r" + strconv.Itoa(gsub.required)
	}
	return f.cache.layoutFor(gsubKey, gposKey, func() *layout {
		pos := f.cache.positioningFor(gposKey, func() *layout {
			return readPositioning(f.layoutTables, gposSel, f.varCoords)
		})
		l := readLayout(f.layoutTables, gsub.features, pos, f.varCoords)
		l.readRequired(f.layoutTables["GSUB"], gsub.required, f.varCoords)
		return l
	})
}

// selectionKey names a selection, so that two scripts selecting the same
// features share one layout. A nil selection — take everything — is named
// distinctly from an empty one, which takes nothing.
func selectionKey(sel featureSet) string {
	if sel == nil {
		return "*"
	}
	idx := make([]int, 0, len(sel))
	for i := range sel {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	var b strings.Builder
	for _, i := range idx {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte(',')
	}
	return b.String()
}

// layoutCache holds a face's readings of its own layout tables, keyed by what
// each selection came to.
//
// It is guarded because faces made for separate documents share one — the
// tables are the font's and are read the same way every time, so reading them
// per document is waste. A layout is written only by the readers that build it,
// so the lock protects the map rather than the values in it, and is taken once
// per distinct selection rather than per run.
type layoutCache struct {
	mu sync.Mutex
	// byScript is the first thing asked, keyed by the question a run actually
	// arrives with. The two below are keyed by what a script *selected*, so that
	// scripts a font treats identically — the common case, a face declaring the
	// same features for latn, grek and cyrl — share one reading of tables that
	// run to tens of kilobytes.
	byScript      map[scriptKey]*layout
	positionings  map[string]*layout
	scriptLayouts map[string]*layout
	// chosen is the script tag each run's question chose — see
	// chosenScriptTag — which a layout cannot say, since two scripts a font
	// treats alike share one.
	chosen map[scriptKey]string
}

// scriptKey is a run's script and its language, as the language system tags it
// is looked up under (see languageKey), which together are everything the
// selection depends on.
type scriptKey struct {
	script uint16
	lang   string
}

func (c *layoutCache) forScript(script uint16, lang string) (*layout, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	l, ok := c.byScript[scriptKey{script, lang}]
	return l, ok
}

func (c *layoutCache) rememberScript(script uint16, lang string, l *layout) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byScript == nil {
		c.byScript = map[scriptKey]*layout{}
	}
	c.byScript[scriptKey{script, lang}] = l
}

func (c *layoutCache) chosenFor(script uint16, lang string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	tag, ok := c.chosen[scriptKey{script, lang}]
	return tag, ok
}

func (c *layoutCache) rememberChosen(script uint16, lang, tag string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.chosen == nil {
		c.chosen = map[scriptKey]string{}
	}
	c.chosen[scriptKey{script, lang}] = tag
}

func (c *layoutCache) layoutFor(gsubKey, gposKey string, build func() *layout) *layout {
	key := gsubKey + "/" + gposKey
	c.mu.Lock()
	if l, ok := c.scriptLayouts[key]; ok {
		c.mu.Unlock()
		return l
	}
	c.mu.Unlock()

	// Built outside the lock: reading a large font's tables takes milliseconds,
	// and holding the lock across it would serialise every goroutine shaping in
	// any script. Two arriving together may both build; they build the same
	// value from the same immutable bytes, and the first one stored wins.
	l := build()

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.scriptLayouts[key]; ok {
		return existing
	}
	if c.scriptLayouts == nil {
		c.scriptLayouts = map[string]*layout{}
	}
	c.scriptLayouts[key] = l
	return l
}

// LayoutLimits reports the bounds reading this face's layout tables has run
// into so far, each once, in words that say what was not read.
//
// A face's tables are read when it is loaded and again for each script a
// document sets in it, so a limit can appear after the face has been in use; a
// caller wanting the whole story asks after shaping. An empty answer means
// every table read so far was read whole.
func (f *Face) LayoutLimits() []string {
	var out []string
	seen := map[string]bool{}
	add := func(l *layout) {
		if l == nil {
			return
		}
		for _, m := range l.limits {
			if !seen[m] {
				seen[m] = true
				out = append(out, m)
			}
		}
	}
	add(f.layout)
	if f.cache != nil {
		f.cache.mu.Lock()
		defer f.cache.mu.Unlock()
		for _, m := range []map[string]*layout{f.cache.positionings, f.cache.scriptLayouts} {
			keys := make([]string, 0, len(m))
			for k := range m {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				add(m[k])
			}
		}
	}
	return out
}

func (c *layoutCache) positioningFor(key string, build func() *layout) *layout {
	c.mu.Lock()
	if l, ok := c.positionings[key]; ok {
		c.mu.Unlock()
		return l
	}
	c.mu.Unlock()

	l := build()

	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.positionings[key]; ok {
		return existing
	}
	if c.positionings == nil {
		c.positionings = map[string]*layout{}
	}
	c.positionings[key] = l
	return l
}

// ignores reports whether the lookup being applied steps over a glyph, taking
// the mark glyph set from the lookup rather than from the caller.
func (sh shaper) ignores(flags int, g Glyph) bool {
	return sh.l.ignoresIn(flags, sh.markSet, g)
}
