package shape

import "slices"

// Arabic joining forms for a face that states none: HarfBuzz's fallback
// shaping (hb-ot-shaper-arabic-fallback.hh).
//
// The joining model decides which form each letter takes and marks it for
// 'init', 'medi', 'fina' or 'isol'; the font's lookups for those features draw
// the form. A font whose rules for the run's script state none of the four
// has nothing to draw them with, and every letter stays in its isolated shape.
// Such fonts are not rare: an older font that maps the Arabic Presentation
// Forms and has no GSUB at all, and — more often — a font that states its
// Arabic rules under 'arab' only, set in a run whose script or language asks
// for another tag, which selects none of them. Thabit, Playpen Sans Arabic and
// Noto Naskh Arabic UI are all like that in und-x-hbscdflt.
//
// HarfBuzz draws those forms anyway, out of the character map: where Unicode
// says U+FE91 is the initial form of U+0628 and the face maps both, the face's
// glyph for U+0628 becomes its glyph for U+FE91 wherever the joining model
// marked the letter initial. It builds one single substitution per form, and
// three ligature substitutions under 'rlig' for the ligatures it knows (the
// lam-alefs, "Allah", the shadda-with-vowel marks and a few more), each out of
// the glyphs the face maps for the characters in presentationForms and its
// ligature tables, which cmd/genarabicforms generates from UnicodeData.txt as
// HarfBuzz's gen-arabic-table.py does.
//
// The lookups are built as lookups — bytes in the format a font would write
// them in, coverage and all — and applied by the same code that applies a
// font's, so that what they step over, how a ligature records its parts and
// where a mark over one goes are what they are for any lookup. The bytes are
// HarfBuzz's too: the coverage format is chosen as it chooses it, so that a
// face mapping two letters to one glyph resolves the duplicate the same way.
//
// # When
//
// For the Arabic script only, and only when the rules the run selected declare
// none of 'init', 'medi', 'fina' and 'isol', in GSUB or in GPOS — declared is
// enough, with or without lookups, as it is for HarfBuzz. The lookups apply
// after the stage 'rlig' is in and before 'calt', which is where HarfBuzz
// pauses to apply them, and each is for the glyphs its feature is: the joining
// forms for the letters marked with them, 'rlig' for every glyph.
//
// HarfBuzz also leaves the fallback out where a caller has turned one of the
// five features off. Nothing here can: Features turns off only the optional
// ligatures, the contextual alternates and kerning.
//
// # What is not here
//
// HarfBuzz has a second fallback, for a face that maps no presentation form
// the first could use and whose character map looks like Windows-1256's —
// alef at glyph 199, lam at 225, and so on. It applies a hand-written table of
// lookups for that encoding. No face in the corpora this package is measured
// on is shaped that way, so there is nothing to measure it against, and it is
// not done: such a face draws its letters unjoined, as it did.

// presentationLigature is a ligature the fallback makes: its first part, the
// rest, 0 where there are fewer, and what they become. See arabicforms.go.
type presentationLigature struct {
	first rune
	rest  [2]rune
	lig   rune
}

// arabicFallbackFeatures are the fallback's lookups in the order HarfBuzz
// applies them — arabic_fallback_features — with the glyphs each is for. The
// first four are presentationForms' columns; the last three are the three
// ligature tables.
var arabicFallbackFeatures = [...]struct {
	tag  string
	mask glyphMask
}{
	{"init", maskInit}, {"medi", maskMedi}, {"fina", maskFina}, {"isol", maskIsol},
	{"rlig", 0}, {"rlig", 0}, {"rlig", 0},
}

// arabicFallbackPlan is the fallback's lookups as a plan applies them, where
// the fallback applies to a plan at all: see "When" above. The indices are
// into the face's synthesized lookups, which a plan cannot see — a plan is the
// layout's and the lookups are the character map's — so a lookup the face
// cannot build is left in and skipped when it is applied.
func arabicFallbackPlan(l *layout) []planLookup {
	for _, f := range arabicFallbackFeatures[:4] {
		_, sub := l.featureLookups[f.tag]
		_, pos := l.gposFeatures[f.tag]
		if sub || pos {
			return nil
		}
	}
	out := make([]planLookup, len(arabicFallbackFeatures))
	for i, f := range arabicFallbackFeatures {
		out[i] = planLookup{index: i, mask: f.mask}
	}
	return out
}

// applyArabicFallback applies the fallback's lookups to a run.
func (sh shaper) applyArabicFallback(buf []Glyph, fallback []planLookup) []Glyph {
	lookups := sh.f.arabicFallbackLookups()
	var stage []planLookup
	for _, pl := range fallback {
		if lookups[pl.index].kind != 0 {
			stage = append(stage, pl)
		}
	}
	if len(stage) == 0 {
		return buf
	}
	// The lookups are applied as the font's are, over a layout that is this
	// one's classification of the glyphs with the synthesized lookups in
	// place of the font's.
	fsh := sh
	fsh.l = &layout{glyphClass: sh.l.glyphClass, markAttach: sh.l.markAttach,
		markSets: sh.l.markSets, gsub: lookups}
	return fsh.applyStage(buf, stage)
}

// arabicFallbackLookups is the face's synthesized lookups, one per entry of
// arabicFallbackFeatures and of kind 0 where the face maps nothing that one
// could be built from. They depend on the character map alone, so they are
// built once per face and shared by its clones.
func (f *Face) arabicFallbackLookups() []rawLookup {
	c := f.cache
	c.arabicOnce.Do(func() {
		out := make([]rawLookup, len(arabicFallbackFeatures))
		for i := range 4 {
			out[i] = f.presentationFormLookup(i)
		}
		out[4] = f.presentationLigatureLookup(presentationLigatures3[:], 2, flagIgnoreMarks)
		out[5] = f.presentationLigatureLookup(presentationLigatures[:], 1, flagIgnoreMarks)
		out[6] = f.presentationLigatureLookup(presentationMarkLigatures[:], 1, 0)
		c.arabicLookups = out
	})
	return c.arabicLookups
}

// hasFallbackForms reports whether the fallback can draw any joining form in
// this face: whether it maps a letter and one of its forms.
func (f *Face) hasFallbackForms() bool {
	for _, lk := range f.arabicFallbackLookups()[:4] {
		if lk.kind != 0 {
			return true
		}
	}
	return false
}

// presentationFormLookup is the single substitution for one of the four
// forms, column of presentationForms: arabic_fallback_synthesize_lookup_single.
// Each letter the face maps goes to the glyph of its form, where the face maps
// that and it is a different glyph; the pairs are in the order of the letters'
// glyphs, a stable sort, as HarfBuzz orders them. The lookup ignores marks.
func (f *Face) presentationFormLookup(column int) rawLookup {
	type pair struct{ from, to int }
	var pairs []pair
	for u := rune(presentationFormsFirst); u <= presentationFormsLast; u++ {
		s := presentationForms[u-presentationFormsFirst][column]
		if s == 0 {
			continue
		}
		ug, ok1 := f.GlyphID(u)
		sg, ok2 := f.GlyphID(s)
		if !ok1 || !ok2 || ug == sg || ug > 0xFFFF || sg > 0xFFFF {
			continue
		}
		pairs = append(pairs, pair{ug, sg})
	}
	if len(pairs) == 0 {
		return rawLookup{markSet: -1}
	}
	slices.SortStableFunc(pairs, func(a, b pair) int { return a.from - b.from })
	from := make([]int, len(pairs))
	for i, p := range pairs {
		from[i] = p.from
	}
	cov := serializeCoverage(from)
	// SingleSubstFormat2: format, coverage offset, count, substitutes, and the
	// coverage after them.
	sub := make([]byte, 6+2*len(pairs))
	put16(sub, 0, 2)
	put16(sub, 2, len(sub))
	put16(sub, 4, len(pairs))
	for i, p := range pairs {
		put16(sub, 6+2*i, p.to)
	}
	sub = append(sub, cov...)
	return rawLookup{kind: 1, flags: flagIgnoreMarks, markSet: -1, subs: [][]byte{sub}}
}

// presentationLigatureLookup is a ligature substitution built from one of the
// ligature tables, whose ligatures have n parts after the first:
// arabic_fallback_synthesize_lookup_ligature. A table's ligatures sharing a
// first part are one set, as HarfBuzz's table has them; the sets whose first
// part the face maps are taken in the order of that glyph, a stable sort, and
// within a set every ligature the face maps, with every part, is kept in the
// table's order. A set left with none is kept, empty, as HarfBuzz keeps it.
func (f *Face) presentationLigatureLookup(table []presentationLigature, n int, flags int) rawLookup {
	type set struct {
		first int
		ligs  [][]int // the ligature glyph, then the parts after the first
	}
	var sets []set
	for i := 0; i < len(table); {
		j := i
		for j < len(table) && table[j].first == table[i].first {
			j++
		}
		fg, ok := f.GlyphID(table[i].first)
		if ok {
			s := set{first: fg}
			for _, l := range table[i:j] {
				lg, ok := f.GlyphID(l.lig)
				if !ok {
					continue
				}
				lig := []int{lg}
				for _, r := range l.rest[:n] {
					g, ok := f.GlyphID(r)
					if r == 0 || !ok {
						lig = nil
						break
					}
					lig = append(lig, g)
				}
				if lig != nil {
					s.ligs = append(s.ligs, lig)
				}
			}
			sets = append(sets, s)
		}
		i = j
	}
	count := 0
	for _, s := range sets {
		count += len(s.ligs)
	}
	if count == 0 {
		return rawLookup{markSet: -1}
	}
	slices.SortStableFunc(sets, func(a, b set) int { return a.first - b.first })

	// LigatureSubstFormat1: format, coverage offset, set count, the set
	// offsets; then each set — a count and its ligatures' offsets — and each
	// ligature — its glyph, its part count and the parts after the first;
	// then the coverage.
	sub := make([]byte, 6+2*len(sets))
	put16(sub, 0, 1)
	put16(sub, 4, len(sets))
	firsts := make([]int, len(sets))
	for i, s := range sets {
		firsts[i] = s.first
		setAt := len(sub)
		put16(sub, 6+2*i, setAt)
		sub = append(sub, make([]byte, 2+2*len(s.ligs))...)
		put16(sub, setAt, len(s.ligs))
		for k, lig := range s.ligs {
			put16(sub, setAt+2+2*k, len(sub)-setAt)
			rec := make([]byte, 4+2*(len(lig)-1))
			put16(rec, 0, lig[0])
			put16(rec, 2, len(lig))
			for m, g := range lig[1:] {
				put16(rec, 4+2*m, g)
			}
			sub = append(sub, rec...)
		}
	}
	put16(sub, 2, len(sub))
	sub = append(sub, serializeCoverage(firsts)...)
	return rawLookup{kind: 4, flags: flags, markSet: -1, subs: [][]byte{sub}}
}

// serializeCoverage writes a coverage table for glyphs in the order given,
// choosing its format as HarfBuzz's Coverage::serialize does: a list of the
// glyphs where that is no more than three times the number of runs of
// consecutive glyphs, and the runs otherwise. A glyph given twice is kept
// twice, as it is there, which is what decides which of two letters sharing a
// glyph a lookup answers for.
func serializeCoverage(glyphs []int) []byte {
	runs, last := 0, -2
	for _, g := range glyphs {
		if last+1 != g {
			runs++
		}
		last = g
	}
	if len(glyphs) <= 3*runs {
		out := make([]byte, 4+2*len(glyphs))
		put16(out, 0, 1)
		put16(out, 2, len(glyphs))
		for i, g := range glyphs {
			put16(out, 4+2*i, g)
		}
		return out
	}
	out := make([]byte, 4, 4+6*runs)
	put16(out, 0, 2)
	put16(out, 2, runs)
	last = -2
	for i, g := range glyphs {
		if last+1 != g {
			out = append(out, 0, 0, 0, 0, 0, 0)
			rec := len(out) - 6
			put16(out, rec, g)
			put16(out, rec+4, i)
		}
		put16(out, len(out)-4, g)
		last = g
	}
	return out
}

// put16 writes a big-endian sixteen-bit value.
func put16(b []byte, at, v int) {
	b[at] = byte(v >> 8)
	b[at+1] = byte(v)
}
