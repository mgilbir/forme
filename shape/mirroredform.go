package shape

import "sort"

// A glyph's right-to-left mirrored form, as the font states it: 'rtlm'.
//
// A right-to-left run of text is drawn with Unicode's mirrors first — "(" as
// ")" (rule L4 of UAX #9) — and the font's 'rtlm' only for the glyphs that
// left alone (see maskUnmirrored). That is HarfBuzz's order and a line of
// text's. MathML Core asks in the other order for the glyph it stretches and
// enlarges: "if there exists an OpenType rtlm variant of g in the first
// available font, then return it", and only otherwise the mirror character's
// glyph (§5.3.2, the algorithm to get a glyph corresponding to a character
// given a directionality). The font's 'rtlm' form is the one its designer drew
// the size variants and the assembly of — an integral that leans the other way
// is a glyph of its own with constructions of its own, and the glyph of a
// mirror character has the constructions of that character.
//
// So the question has to be put to the font alone: which glyph does 'rtlm'
// make of this one. It is asked of the lookups the feature names under the
// script the character's own text would be shaped in, with nothing else
// applied before or after them — no other feature decides what a glyph's
// mirrored form is.

// MirroredForm is the font's 'rtlm' form of the glyph r is drawn with: the
// glyph the lookups of the feature substitute for it, applied in their order
// to that glyph alone. ok is false where the face has no glyph for r, states no
// 'rtlm' for the script r is shaped in, or states one that leaves the glyph as
// it is. A face read by character code — one of the standard fonts — has no
// rules to state one with.
func (f *Face) MirroredForm(r rune) (gid int, ok bool) {
	if f == nil || !f.composite() {
		return 0, false
	}
	g, ok := f.GlyphID(r)
	if !ok {
		return 0, false
	}
	// The script and the language system the character is shaped under on
	// its own: a symbol decides no script, and is read under 'DFLT', 'dflt'
	// or 'latn' as HarfBuzz reads it (see readLayoutFor).
	s := string(r)
	var one [1]scriptRun
	pieces := scriptRuns(s, scriptUnknown, scriptUnknown, one[:0])
	if len(pieces) != 1 {
		return 0, false
	}
	lang := openTypeLanguage("")
	l := f.layoutFor(pieces[0].script, lang)
	indices := l.featureLookups["rtlm"]
	if len(indices) == 0 {
		return 0, false
	}
	// In the order of the lookup list, each once, as a plan applies the
	// lookups of the features in one stage (see plan.go).
	indices = append([]int(nil), indices...)
	sort.Ints(indices)
	lookups := make([]planLookup, 0, len(indices))
	for i, idx := range indices {
		if i > 0 && idx == indices[i-1] {
			continue
		}
		lookups = append(lookups, planLookup{index: idx})
	}
	buf := []Glyph{{GID: g, XAdvance: f.advanceGID(g), class: classOfRune(r), umark: unicodeMarkOf(r)}}
	sh := shaper{f: f, l: l, ligIDs: new(int), lang: lang, ops: lookupBudget(len(buf))}
	buf = sh.applyStage(buf, lookups)
	if len(buf) != 1 || buf[0].GID == g {
		// A lookup that took the glyph apart or joined it to nothing is not a
		// mirrored form of it: 'rtlm' is registered as a single
		// substitution, one glyph for one.
		return 0, false
	}
	return buf[0].GID, true
}
