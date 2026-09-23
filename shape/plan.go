package shape

import (
	"sort"
	"sync"
)

// The shaping plan: which model sets a run, and which of the font's lookups it
// applies, in what groups and in what order.
//
// # Why there is one
//
// Everything a shaper does with a font's substitutions comes down to two
// decisions, and both used to be taken piecemeal. Which *model* sets the run was
// read off the text's script alone — a Devanagari run went to the Indic
// reordering whatever the font said — and which *features* apply, in what order,
// was a list of tags written out separately in each of five places, each applied
// tag by tag.
//
// Neither is how a font is written against. A font is tested against HarfBuzz,
// and HarfBuzz decides both from the font as well as the text:
//
//   - The model is chosen from the script *and the tag the font's rules were
//     read under* (hb_ot_shaper_categorize). A Tai Tham or Devanagari font that
//     declares its rules only under 'DFLT' or 'latn' was written for text in
//     stored order, and does its own work in 'ccmp' and 'liga'; reordering it
//     first breaks every ligature it states. A Myanmar font under 'mymr' predates
//     the Myanmar model and is shaped without it.
//   - The features are collected into *stages* (hb_ot_shape_collect_features).
//     Within a stage the lookups of every feature in it are merged, each lookup
//     once, and applied in the order of their index in the font's LookupList —
//     because that is the order the designer wrote them in and the only one the
//     font was tested against. Between stages the model runs its own passes: the
//     syllable cut, the reorderings, the joining forms.
//
// Applying tag by tag gets the same answer only while no two features of a
// stage name overlapping lookups, and fonts do name them. Where two features
// name lookups over the same glyphs, the order decides which sees the other's
// output, and Noto Sans has three pairs where it does:
//
//	onum + zero  gives the oldstyle slashed zero, a glyph neither alone reaches
//	onum + pnum  gives the proportional oldstyle figures, likewise
//	onum + frac  gives the fraction's numerators, because 'frac' is stated
//	             later and covers what 'onum' produced
//
// Tag by tag in §6.7's order gets the third wrong: the oldstyle figures win and
// the fraction never forms. And a feature named twice — a document's
// font-feature-settings: "liga" on a face that applies liga anyway — ran its
// lookups twice, the second time over their own output. So a plan is built once
// per (layout, model, direction, requested features), as data, and every entry
// point reads its stages from it.
//
// # The direction's forms
//
// A mirrored form is the glyph a character is drawn with when the line runs the
// other way. Unicode mirrors a bracket by character, and a font may state the
// same thing by glyph, which is what 'rtlm' is for — it covers what the
// character property cannot, such as an integral sign or an arrow that leans.
// 'rtla' is a letterform a right-to-left line wants rather than a mirror, and
// 'ltra' and 'ltrm' are both of those for a left-to-right line. They are in the
// first stage after 'rvrn', with everything the default model applies; 'rtlm'
// is for the glyphs Unicode's mirroring did not already replace. See
// maskUnmirrored.
//
// # What is mirrored and what is not
//
// The stage structure, the per-feature flags (which glyphs a feature applies
// to, whether its lookups step over the join controls, whether they are held to
// one syllable), the merge of a feature named more than once, the order of the
// lookups inside a stage, and the language system's required feature. The
// automatic fractions around U+2044 and the right-to-left mirrored forms are
// masked the way HarfBuzz masks them.
//
// Not mirrored, besides the parts of two models named at modelHebrew: 'rand',
// which HarfBuzz applies with a pseudo-random choice of alternate and this
// package applies as the first alternate like any other alternate
// substitution; 'stch', whose substitution means nothing without the
// stretching HarfBuzz does after it (see arabic.go); the fallback shaping
// HarfBuzz does for an Arabic font with no GSUB; and 'vert', since nothing here
// sets text vertically.
//
// Where HarfBuzz changed between versions, what is mirrored is what the
// version the oracle runs does: HarfBuzz 8 turned 'calt' off for Hangul, and the
// HarfBuzz the corpus and the sweep are compared against does not.

// shaperModel is which model sets a run: how its characters are cut, reordered
// and put through the font's features.
type shaperModel uint8

const (
	// modelDefault applies the font's features to the run as it stands.
	modelDefault shaperModel = iota
	// modelArabic chooses each letter's joining form first. See arabic.go.
	modelArabic
	// modelIndic is the Devanagari family's syllable model. See indic.go.
	modelIndic
	modelKhmer
	modelMyanmar
	// modelUniversal is the Universal Shaping Engine. See use.go.
	modelUniversal
	// modelThai is the default model with Thai and Lao's one rearrangement of
	// the text first. See thai.go.
	modelThai
	// modelHebrew and modelHangul are the default model's features, named
	// apart because HarfBuzz sets them apart: Hangul cancels no mark's advance.
	// What else HarfBuzz's two models do is not done here — Hebrew's
	// presentation forms for a font without mark positioning and Hangul's
	// composition of old jamo sequences — and each is named in the plan's
	// header.
	modelHebrew
	modelHangul
)

// syllabic reports whether the model cuts a run into syllables and reorders
// them. It is what decides how the run is normalised and when the characters
// nothing is drawn for are taken out — see usesSyllabicShaper.
func (m shaperModel) syllabic() bool {
	switch m {
	case modelIndic, modelKhmer, modelMyanmar, modelUniversal:
		return true
	}
	return false
}

// categorize chooses the model for a run, from its script and from the tag the
// font's substitutions were read under. It is hb_ot_shaper_categorize.
//
// chosen is that tag, or "" for a font whose GSUB declares nothing the run
// could use (or has no GSUB at all). The empty tag is not 'DFLT', and the
// difference matters: a font with no substitutions for a script has no opinion
// about the order its text is drawn in, and the script's own model is the only
// thing that can put a vowel sign before its consonant. A font that states its
// rules under 'DFLT' or 'latn' does have an opinion — it states them over the
// text in stored order — and the model would undo it.
func categorize(script uint16, chosen string) shaperModel {
	fallback := chosen == "DFLT" || chosen == "latn"
	switch {
	case scriptSelects(script, "arab"):
		// Arabic is set by its own model whatever the font declares: the forms
		// are chosen from the characters, and a font with no 'arab' still has
		// them to be chosen.
		return modelArabic
	case scriptSelects(script, "syrc"):
		// Syriac only where the font was read under something other than
		// 'DFLT'. HarfBuzz tests 'DFLT' alone here, not 'latn'.
		if chosen == "DFLT" {
			return modelDefault
		}
		return modelArabic
	case scriptSelects(script, "thai"), scriptSelects(script, "lao "):
		return modelThai
	case scriptSelects(script, "hang"):
		return modelHangul
	case scriptSelects(script, "hebr"):
		return modelHebrew
	case indicConfigFor(script) != nil:
		switch {
		case fallback:
			return modelDefault
		case len(chosen) == 4 && chosen[3] == '3':
			// The third-generation tags are the Indic scripts written for the
			// universal engine rather than for the Indic model.
			return modelUniversal
		}
		return modelIndic
	case isKhmerScript(script):
		return modelKhmer
	case isMyanmarScript(script):
		// 'mymr' is the tag from before the Myanmar model was specified, and a
		// font written under it expects no reordering: the Zawgyi-era fonts are
		// the case, and HarfBuzz tells them apart by exactly this.
		if fallback || chosen == "mymr" {
			return modelDefault
		}
		return modelMyanmar
	case usesUniversalShaper(script):
		if fallback {
			return modelDefault
		}
		return modelUniversal
	}
	return modelDefault
}

// zeroMarks is when the model cancels a mark's own advance. See zeroMarkWidths.
func (m shaperModel) zeroMarks() zeroMarkWidths {
	switch m {
	case modelIndic, modelKhmer, modelHangul:
		return zeroMarksNone
	case modelMyanmar, modelUniversal:
		return zeroMarksEarly
	}
	return zeroMarksLate
}

// glyphMask is which of the masked features a glyph is for.
//
// Most features apply to every glyph of a run: a font's 'liga' or 'ccmp' is
// meant wherever its lookups match. Some are for particular glyphs only, and the
// model decides which: 'init' for the letter that opens a word, 'half' for the
// consonants before an Indic base, 'rphf' for the Ra that becomes a reph. A
// glyph carries the bit of each such feature it is for, and a masked feature's
// lookup starts only at a glyph that carries its bit and matches its input only
// over glyphs that carry it — what is before and after, the rule's context, is
// read whatever it carries. That is HarfBuzz's own arrangement.
//
// The bits are fixed per feature rather than handed out per plan, because no
// plan uses more than a handful and every model's code names them directly.
// 'init' is one bit for the Arabic, universal and Indic models alike: each gives
// it its own meaning and no run is set by two models.
type glyphMask uint32

const (
	maskIsol glyphMask = 1 << iota
	maskFina
	maskFin2
	maskFin3
	maskMedi
	maskMed2
	maskInit
	maskRphf
	maskPref
	maskBlwf
	maskAbvf
	maskHalf
	maskPstf
	maskCfar
	// maskRtlm is the right-to-left mirrored forms, for the characters Unicode's
	// own mirroring left alone.
	maskRtlm
	// The automatic fractions, around a fraction slash between digits.
	maskFrac
	maskNumr
	maskDnom
)

// featureFlags is how a feature is applied, apart from which glyphs it is for.
type featureFlags uint8

const (
	// flagManualZWJ says the feature's lookups see a zero width joiner in their
	// input rather than stepping over it; flagManualZWNJ says their context
	// sees a non-joiner. See stepsOverJoiner.
	flagManualZWJ featureFlags = 1 << iota
	flagManualZWNJ
	// flagPerSyllable holds the feature's lookups to one syllable: neither
	// what they match nor the context they read may cross into the next.
	flagPerSyllable

	flagManualJoiners = flagManualZWJ | flagManualZWNJ
)

// planFeature is one feature as a model or a caller asked for it.
type planFeature struct {
	tag string
	// mask is the bit of a feature for particular glyphs, and zero for one that
	// is for every glyph.
	mask  glyphMask
	flags featureFlags
	// on is false for a feature turned off: by a document, or by a model that
	// does not want one the general list turns on.
	on bool
	// stage is where the feature's lookups are applied, and seq the order it
	// was asked for in, which is what settles two requests for one tag.
	stage, seq int
}

// planLookup is one lookup as a stage applies it.
type planLookup struct {
	index int
	// mask is the glyphs the lookup is for, or zero for every glyph. A lookup
	// two features name is for the glyphs either is for, and a lookup a global
	// feature names is for every glyph.
	mask glyphMask
	// The join controls, as the features naming the lookup asked: a lookup two
	// features name steps over a joiner only if both of them do.
	manualZWJ, manualZWNJ bool
	perSyllable           bool
}

// plan is the model for a run and the stages its substitutions are applied in.
//
// A stage is a list of lookups in index order, each once. The model's own passes
// run between stages, and which stage a pass comes after is recorded in the
// model's fields below, so that the code applying them can name the point it is
// at rather than count.
type plan struct {
	model  shaperModel
	stages [][]planLookup
	// syllables is the first stage applied a syllable at a time, and reorder the
	// first stage after the model's reordering; after is the first stage after
	// the syllables are done with. See each model's collect function.
	syllables, reorder, after int
	// basic is the Indic and Myanmar basic features, one stage each, starting
	// here, and the universal engine's orthographic stage; rphf and pref are
	// the universal engine's two reordering stages.
	basic, rphf, pref int
	// fractions and rtlm say whether the plan has the automatic fractions and
	// the mirrored forms at all, so that a run need not be scanned for glyphs to
	// mask when nothing would read the mask.
	fractions, rtlm bool
}

// planBuilder collects features into stages, in the order a model asks for
// them. It is HarfBuzz's map builder: a feature asked for is recorded against
// the current stage, and a pause ends the stage.
type planBuilder struct {
	features []planFeature
	stage    int
}

// enable asks for a feature for every glyph.
func (b *planBuilder) enable(tag string, flags featureFlags) {
	b.features = append(b.features, planFeature{tag: tag, flags: flags, on: true,
		stage: b.stage, seq: len(b.features)})
}

// add asks for a feature for the glyphs that carry its bit.
func (b *planBuilder) add(tag string, mask glyphMask, flags featureFlags) {
	b.features = append(b.features, planFeature{tag: tag, mask: mask, flags: flags, on: true,
		stage: b.stage, seq: len(b.features)})
}

// disable turns a feature off, whatever asked for it before.
func (b *planBuilder) disable(tag string) {
	b.features = append(b.features, planFeature{tag: tag, stage: b.stage, seq: len(b.features)})
}

// pause ends the current stage and reports the number of the next one.
func (b *planBuilder) pause() int {
	b.stage++
	return b.stage
}

// userFeature is a feature a caller asked for or turned off.
type userFeature struct {
	tag string
	on  bool
}

// requested is what a Features value and a caller's extra tags ask of a plan:
// the features turned off first and the ones turned on after, so that a tag
// both turns on and off comes out on. That is CSS Fonts 4's precedence — what
// font-feature-settings asks for by tag overrides what the font-variant
// properties and the spacing rule turned off — and it is the only one of the two
// orders that lets an author who wrote "liga" 1 have the ligatures.
func (f Features) requested(extra []string) []userFeature {
	var out []userFeature
	for _, tag := range [...]string{"liga", "clig", "dlig", "hlig", "calt"} {
		if f.suppresses(tag) {
			out = append(out, userFeature{tag, false})
		}
	}
	for _, tag := range f.adds() {
		out = append(out, userFeature{tag, true})
	}
	for _, tag := range extra {
		out = append(out, userFeature{tag, true})
	}
	return out
}

// planKey is everything a plan depends on beyond the layout it is read from.
type planKey struct {
	model shaperModel
	rtl   bool
	// arabicScript separates Arabic from the other scripts the Arabic model
	// sets: HarfBuzz pauses after 'rlig' for Arabic alone.
	arabicScript bool
	features     Features
	extra        string
}

// planCache holds a layout's plans. It is bounded because a key carries what a
// document asked for, and a document may ask for any number of distinct
// font-feature-settings: past the bound a plan is built for the run and not
// kept, which costs time and never correctness.
type planCache struct {
	mu    sync.Mutex
	plans map[planKey]*plan
}

// maxCachedPlans is how many plans one layout keeps. A document sets a face in
// a handful of scripts, two directions and a few feature settings; the bound is
// far above that and far below what a hostile one could ask for.
const maxCachedPlans = 64

// planFor is the plan for a run: from the cache where it has been built before,
// and built and cached where it has not.
func (sh shaper) planFor(model shaperModel, arabicScript bool, extra []string) *plan {
	key := planKey{model: model, rtl: sh.rtl, arabicScript: arabicScript, features: sh.features}
	if len(extra) > 0 {
		key.extra = joinTags(extra)
	}
	c := sh.l.plans
	if c != nil {
		c.mu.Lock()
		p, ok := c.plans[key]
		c.mu.Unlock()
		if ok {
			return p
		}
	}
	p := buildPlan(sh.l, key, extra)
	if c != nil {
		c.mu.Lock()
		if len(c.plans) < maxCachedPlans {
			if c.plans == nil {
				c.plans = map[planKey]*plan{}
			}
			c.plans[key] = p
		}
		c.mu.Unlock()
	}
	return p
}

func joinTags(tags []string) string {
	n := 0
	for _, t := range tags {
		n += len(t) + 1
	}
	b := make([]byte, 0, n)
	for i, t := range tags {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, t...)
	}
	return string(b)
}

// buildPlan collects a model's features and compiles them against a layout.
// It is hb_ot_shape_collect_features followed by the map builder's compile.
func buildPlan(l *layout, key planKey, extra []string) *plan {
	p := &plan{model: key.model}
	b := &planBuilder{}

	// 'rvrn' first, in a stage of its own: the substitutions a variable font
	// requires at the location it was cut at, which every other rule is written
	// against.
	b.enable("rvrn", 0)
	b.pause()

	// What the direction selects, and the forms HarfBuzz applies by default to
	// particular characters: the fractions and, for a right-to-left run, the
	// mirrored forms of the characters Unicode did not mirror itself.
	if key.rtl {
		b.enable("rtla", 0)
		b.add("rtlm", maskRtlm, 0)
	} else {
		b.enable("ltra", 0)
		b.enable("ltrm", 0)
	}
	b.add("frac", maskFrac, 0)
	b.add("numr", maskNumr, 0)
	b.add("dnom", maskDnom, 0)

	switch key.model {
	case modelArabic:
		collectArabic(b, l, key.arabicScript)
	case modelIndic:
		collectIndic(b, p)
	case modelKhmer:
		collectKhmer(b, p)
	case modelMyanmar:
		collectMyanmar(b, p)
	case modelUniversal:
		collectUniversal(b, p)
	}

	// The features every script gets, in the stage whatever the model left
	// open. A tag a model already asked for keeps the model's stage and flags
	// — see compile — so naming 'ccmp' here again moves nothing.
	for _, tag := range [...]string{"ccmp", "locl", "rlig"} {
		b.enable(tag, 0)
	}
	for _, tag := range [...]string{"calt", "clig", "liga", "rclt"} {
		b.enable(tag, 0)
	}

	for _, u := range key.features.requested(extra) {
		if u.on {
			b.enable(u.tag, 0)
		} else {
			b.disable(u.tag)
		}
	}

	// What a model turns off after everything else has been asked for, which
	// is why it comes last: an author's "liga" does not turn it back on in a
	// script whose model says a font's 'liga' is not for it.
	switch key.model {
	case modelIndic:
		b.disable("liga")
	case modelKhmer:
		b.enable("clig", 0)
		b.disable("liga")
	}

	p.compile(l, b)
	// As HarfBuzz decides it: 'frac' alone is enough, and without it both
	// halves are needed, since a numerator with no denominator is not a
	// fraction.
	p.fractions = p.hasMask(maskFrac) || p.hasMask(maskNumr) && p.hasMask(maskDnom)
	p.rtlm = p.hasMask(maskRtlm)
	return p
}

// hasMask reports whether any lookup of the plan is for glyphs carrying a bit.
func (p *plan) hasMask(m glyphMask) bool {
	for _, st := range p.stages {
		for _, lk := range st {
			if lk.mask&m != 0 {
				return true
			}
		}
	}
	return false
}

// collectArabic is collect_features_arabic. The joining forms are a stage each,
// in the specification's order, because a font may state one as a contextual
// rule that reads what an earlier one made.
func collectArabic(b *planBuilder, l *layout, arabicScript bool) {
	// HarfBuzz enables 'stch' here and then stretches what it produced across
	// the rest of the word. The stretching is not implemented (see arabic.go),
	// and the substitution without it would draw a letter's pieces unstretched,
	// so the feature is left out; the stage it would be in is kept, so that the
	// stages after it are where HarfBuzz has them.
	b.pause()
	b.enable("ccmp", flagManualZWJ)
	b.enable("locl", flagManualZWJ)
	b.pause()
	for _, form := range arabicForms {
		b.add(form.tag, form.mask, flagManualZWJ)
		b.pause()
	}
	b.pause()
	b.enable("rlig", flagManualZWJ)
	if arabicScript {
		b.pause()
	}
	b.enable("calt", flagManualZWJ)
	// 'rclt' goes with 'calt' where the font has it; where it does not, the
	// pause that stands in its place puts the ligatures in a stage after the
	// alternates. It is what HarfBuzz does (its issue 1573), and it asks the
	// font rather than the requests.
	if len(l.featureLookups["rclt"]) == 0 {
		b.pause()
		b.enable("rclt", flagManualZWJ)
	}
	b.enable("liga", flagManualZWJ)
	b.enable("clig", flagManualZWJ)
	b.enable("mset", flagManualZWJ)
}

// arabicForms is the joining forms in the order they are applied, with the bit
// each is masked by.
var arabicForms = [...]struct {
	tag  string
	mask glyphMask
}{
	{"isol", maskIsol}, {"fina", maskFina}, {"fin2", maskFin2}, {"fin3", maskFin3},
	{"medi", maskMedi}, {"med2", maskMed2}, {"init", maskInit},
}

// collectIndic is collect_features_indic.
func collectIndic(b *planBuilder, p *plan) {
	p.syllables = b.pause()
	b.enable("locl", flagPerSyllable)
	b.enable("ccmp", flagPerSyllable)
	p.basic = b.pause()
	for _, f := range indicBasicFeatures {
		if f.mask == 0 {
			b.enable(f.tag, flagManualJoiners|flagPerSyllable)
		} else {
			b.add(f.tag, f.mask, flagManualJoiners|flagPerSyllable)
		}
		b.pause()
	}
	p.after = b.pause()
	p.reorder = p.after
	b.add("init", maskInit, flagManualJoiners|flagPerSyllable)
	for _, tag := range indicPresentationFeatures {
		b.enable(tag, flagManualJoiners|flagPerSyllable)
	}
}

// collectKhmer is collect_features_khmer: the reordering comes before any of
// the font's rules, and the basic features are one stage with 'locl' and 'ccmp'.
func collectKhmer(b *planBuilder, p *plan) {
	p.syllables = b.pause()
	p.reorder = b.pause()
	b.enable("locl", flagPerSyllable)
	b.enable("ccmp", flagPerSyllable)
	for _, f := range khmerBasicFeatures {
		b.add(f.tag, f.mask, flagManualJoiners|flagPerSyllable)
	}
	p.after = b.pause()
	for _, tag := range [...]string{"pres", "abvs", "blws", "psts"} {
		b.enable(tag, flagManualJoiners)
	}
}

// collectMyanmar is collect_features_myanmar.
func collectMyanmar(b *planBuilder, p *plan) {
	p.syllables = b.pause()
	b.enable("locl", flagPerSyllable)
	b.enable("ccmp", flagPerSyllable)
	p.basic = b.pause()
	p.reorder = p.basic
	for _, tag := range myanmarBasicFeatures {
		b.enable(tag, flagManualZWJ|flagPerSyllable)
		b.pause()
	}
	p.after = b.pause()
	for _, tag := range [...]string{"pres", "abvs", "blws", "psts"} {
		b.enable(tag, flagManualZWJ)
	}
}

// collectUniversal is collect_features_use.
func collectUniversal(b *planBuilder, p *plan) {
	p.syllables = b.pause()
	b.enable("locl", flagPerSyllable)
	b.enable("ccmp", flagPerSyllable)
	b.enable("nukt", flagPerSyllable)
	b.enable("akhn", flagManualZWJ|flagPerSyllable)
	p.rphf = b.pause()
	b.add("rphf", maskRphf, flagManualZWJ|flagPerSyllable)
	b.pause()
	p.pref = b.pause()
	b.enable("pref", flagManualZWJ|flagPerSyllable)
	p.basic = b.pause()
	for _, tag := range useShapeFeatures {
		b.enable(tag, flagManualZWJ|flagPerSyllable)
	}
	p.reorder = b.pause()
	b.pause()
	for _, form := range [...]struct {
		tag  string
		mask glyphMask
	}{{"isol", maskIsol}, {"init", maskInit}, {"medi", maskMedi}, {"fina", maskFina}} {
		b.add(form.tag, form.mask, 0)
	}
	p.after = b.pause()
	for _, tag := range usePresentationFeatures {
		b.enable(tag, flagManualZWJ)
	}
}

// compile turns the requests into stages: each tag once, with the stage and
// flags of its first request and the on-off of its last; then each stage's
// lookups gathered, sorted by index and merged.
func (p *plan) compile(l *layout, b *planBuilder) {
	stages := b.stage + 1
	feats := append([]planFeature(nil), b.features...)
	sort.SliceStable(feats, func(i, j int) bool {
		if feats[i].tag != feats[j].tag {
			return feats[i].tag < feats[j].tag
		}
		return feats[i].seq < feats[j].seq
	})
	merged := feats[:0]
	for _, f := range feats {
		if n := len(merged); n > 0 && merged[n-1].tag == f.tag {
			m := &merged[n-1]
			// A later request for every glyph decides whether the feature is on,
			// and if it is, makes it a feature for every glyph; a later request
			// for particular glyphs turns it on for those. The earliest request's
			// stage and joiner modes stand. That is hb_ot_map_builder_t::compile's
			// merge, and it is what keeps a document's "liga" in the stage the
			// model put liga in rather than running it a second time elsewhere.
			if f.mask == 0 {
				m.on = f.on
				if f.on {
					m.mask = 0
				}
			} else {
				m.on = m.on || f.on
				m.mask = f.mask
			}
			if f.stage < m.stage {
				m.stage = f.stage
			}
			continue
		}
		merged = append(merged, f)
	}

	byStage := make([][]planLookup, stages)
	enabled := map[string]bool{}
	for _, f := range merged {
		if !f.on {
			continue
		}
		lookups := l.featureLookups[f.tag]
		if len(lookups) == 0 {
			continue
		}
		enabled[f.tag] = true
		for _, idx := range lookups {
			byStage[f.stage] = append(byStage[f.stage], planLookup{
				index:       idx,
				mask:        f.mask,
				manualZWJ:   f.flags&flagManualZWJ != 0,
				manualZWNJ:  f.flags&flagManualZWNJ != 0,
				perSyllable: f.flags&flagPerSyllable != 0,
			})
		}
	}
	// The language system's required feature applies whatever asked for it.
	// Where its tag is one of the plan's features it is applied with them —
	// its lookups are among that tag's — and otherwise in the first stage.
	if l.requiredTag != "" && !enabled[l.requiredTag] {
		for _, idx := range l.requiredLookups {
			byStage[0] = append(byStage[0], planLookup{index: idx})
		}
	}

	for s, st := range byStage {
		if len(st) < 2 {
			continue
		}
		sort.SliceStable(st, func(i, j int) bool { return st[i].index < st[j].index })
		out := st[:1]
		for _, lk := range st[1:] {
			last := &out[len(out)-1]
			if lk.index != last.index {
				out = append(out, lk)
				continue
			}
			// One lookup named by two features is one piece of work: it is for
			// the glyphs either feature is for, and steps over a joiner only
			// where both would.
			if lk.mask == 0 || last.mask == 0 {
				last.mask = 0
			} else {
				last.mask |= lk.mask
			}
			last.manualZWJ = last.manualZWJ || lk.manualZWJ
			last.manualZWNJ = last.manualZWNJ || lk.manualZWNJ
			last.perSyllable = last.perSyllable && lk.perSyllable
		}
		byStage[s] = out
	}
	p.stages = byStage
}

// stage is the lookups of one stage, or nothing for a stage the plan does not
// have.
func (p *plan) stage(i int) []planLookup {
	if i < 0 || i >= len(p.stages) {
		return nil
	}
	return p.stages[i]
}
