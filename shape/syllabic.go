package shape

// What the syllabic shapers share.
//
// Four models in this package set text that is not drawn in the order its
// characters are stored: Devanagari and its relatives (indic.go), Khmer
// (khmer.go), Myanmar (myanmar.go) and the Universal Shaping Engine (use.go).
// They are four models, not one — each has its own categories, its own
// syllable grammar and its own reordering, and the OpenType script development
// specifications state them separately because they *are* separate.
//
// What they do share is the machinery underneath: a per-glyph record kept in
// step with a buffer that substitutions are reshaping, features applied to one
// syllable at a time so that a ligature cannot join two of them, and the
// placeholder shown for a syllable with nothing to hang off. That is what is
// here. Which of them, if any, a run belongs to is decided before any of them
// runs, from the script and the font together — see categorize in plan.go.

// usesSyllabicShaper reports whether a script has one of the models that
// segment text into syllables and reorder them — the model a run of it gets from
// a font that states its rules for the script.
//
// Whether a particular run *is* set by one is a question about the font as
// well, and categorize answers it: a font that states its rules only under
// 'DFLT' or 'latn' was written for text in stored order, and its runs are set
// by the default model whatever their script. Everything that has to agree
// about one run — normalisation, the characters nothing is drawn for, the
// shaper — asks the run's model (shaperModel.syllabic), so that the answer is
// taken once and not three times. This is the script-level half of it, which
// categorize is built on.
func usesSyllabicShaper(script uint16) bool {
	return indicConfigFor(script) != nil || isKhmerScript(script) ||
		isMyanmarScript(script) || usesUniversalShaper(script)
}

// shapeSyllabic shapes a run by the syllabic model its plan names.
//
// It is the whole of the substitution pass for a run it handles: the reordering
// decides which of the font's rules apply where, so it cannot be a step before
// the general substitutions and has to be them.
func (sh shaper) shapeSyllabic(buf []Glyph, runes []rune, script uint16, p *plan,
	before, after []rune) []Glyph {

	switch p.model {
	case modelIndic:
		cfg := indicConfigFor(script)
		return sh.shapeIndic(buf, runes, before, sh.indicPlan(cfg, sh.f.indicOldSpec(cfg, script, sh.lang)), p)
	case modelKhmer:
		return sh.shapeKhmer(buf, runes, p)
	case modelMyanmar:
		return sh.shapeMyanmar(buf, runes, p)
	case modelUniversal:
		return sh.shapeUniversal(buf, runes, before, after, p)
	}
	return buf
}

// scriptSelects reports whether a script's OpenType tags include the given one.
//
// A script is identified by the tag a font declares its rules under rather than
// by an index into the generated table, for the same reason indicConfigFor does
// it that way: the tag is what the font and the shaper have in common.
func scriptSelects(script uint16, tag string) bool {
	for _, t := range scriptTags(script) {
		if t == tag {
			return true
		}
	}
	return false
}

// splitCharacters replaces each character that is drawn as several marks by the
// marks it is drawn as, as reported by of.
//
// The parts of such a sign go to different places — one before the letter and
// one after — so there is no single place the sign itself could be given, and
// taking it apart has to happen before anything is placed.
//
// A sign is only taken apart when the face has a glyph for every part. A face
// that draws the sign whole and has no glyph for one of its halves would
// otherwise lose that half altogether, which is worse than drawing the sign
// where the model would rather it were not.
func (sh shaper) splitCharacters(buf []Glyph, runes []rune, of func(rune) ([]rune, bool)) ([]Glyph, []rune) {
	outBuf := make([]Glyph, 0, len(buf))
	outRunes := make([]rune, 0, len(runes))
	for i, r := range runes {
		parts, ok := of(r)
		if !ok {
			outBuf = append(outBuf, buf[i])
			outRunes = append(outRunes, r)
			continue
		}
		gids := make([]int, 0, len(parts))
		for _, p := range parts {
			gid, have := sh.f.GlyphID(p)
			if !have {
				gids = nil
				break
			}
			gids = append(gids, gid)
		}
		if gids == nil {
			outBuf = append(outBuf, buf[i])
			outRunes = append(outRunes, r)
			continue
		}
		// The parts share the sign's cluster: several glyphs standing for one
		// character is exactly what a cluster records.
		// Each part is classified as the character it is, as every glyph
		// made from a character is: in HarfBuzz the parts come from
		// normalization, before glyph classes are inferred.
		for k, gid := range gids {
			outBuf = append(outBuf, Glyph{
				GID: gid, Cluster: buf[i].Cluster, XAdvance: sh.f.advanceGID(gid),
				class: classOfRune(parts[k]),
			})
			outRunes = append(outRunes, parts[k])
		}
	}
	return outBuf, outRunes
}

// insertGlyphAt puts one glyph, and the record that describes it, into a buffer
// at a position. It is how a placeholder reaches a syllable that has nothing of
// its own to hang off.
//
// The glyph takes the cluster of what it is inserted before, so that it maps
// back to the character whose mark it is standing in for.
func (sh shaper) insertGlyphAt(buf []Glyph, info []indicInfo, at, gid int, what indicInfo) ([]Glyph, []indicInfo) {
	cluster := 0
	switch {
	case at < len(buf):
		cluster = buf[at].Cluster
	case len(buf) > 0:
		cluster = buf[len(buf)-1].Cluster
	}
	g := Glyph{GID: gid, Cluster: cluster, XAdvance: sh.f.advanceGID(gid), class: classUnclassified}

	buf = append(buf, Glyph{})
	copy(buf[at+1:], buf[at:])
	buf[at] = g

	info = append(info, indicInfo{})
	copy(info[at+1:], info[at:])
	info[at] = what
	return buf, info
}

// oneCluster gives every glyph of a syllable the cluster of its first
// character.
//
// It has to: once the glyphs are in drawing order they no longer correspond
// one-for-one to the characters, and a syllable is the smallest piece of these
// scripts that can honestly be mapped back to a position in the text.
func oneCluster(buf []Glyph, start, end int) {
	if start >= end {
		return
	}
	cluster := buf[start].Cluster
	for i := start; i < end; i++ {
		if buf[i].Cluster < cluster {
			cluster = buf[i].Cluster
		}
	}
	for i := start; i < end; i++ {
		buf[i].Cluster = cluster
	}
}

// moveGlyphToFront moves the glyph at from to at, shifting what lies between
// forward by one. It is the mirror of rotateIndicLeft, and is what the Khmer
// reordering is made of: a pre-base vowel sign and a subscript Ro are both
// drawn at the front of the syllable whatever stands between.
func moveGlyphToFront(buf []Glyph, info []indicInfo, at, from int) {
	if from <= at || from >= len(buf) || from >= len(info) || at < 0 {
		return
	}
	g, f := buf[from], info[from]
	copy(buf[at+1:from+1], buf[at:from])
	copy(info[at+1:from+1], info[at:from])
	buf[at], info[at] = g, f
}
