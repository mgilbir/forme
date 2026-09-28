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
// # Windows-1256
//
// HarfBuzz has a second fallback, for a face the first builds nothing for —
// one that maps no presentation form and none of the ligatures — and whose
// character map looks like Windows-1256's: a face whose glyphs are in the
// order of that code page, so that the glyph of a letter is its byte there.
// HarfBuzz takes such a face for a font made for that encoding, whose joining
// forms are at glyphs the code page gives to other characters or to none, so
// that nothing in its character map says which glyph is which form. It knows
// where they are from a table written by hand for that encoding
// (hb-ot-shaper-arabic-win1256.hh), and applies it in place of the lookups it
// could not build: see arabicWin1256.

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

// applyArabicFallback applies the fallback's lookups to a run: those the face's
// character map builds, or — where it builds none and the face is
// Windows-1256's — HarfBuzz's table for that encoding.
func (sh shaper) applyArabicFallback(buf []Glyph, fallback []planLookup) []Glyph {
	lookups := sh.f.arabicFallbackLookups()
	var stage []planLookup
	for _, pl := range fallback {
		if lookups[pl.index].kind != 0 {
			stage = append(stage, pl)
		}
	}
	if len(stage) == 0 && len(lookups) > len(arabicFallbackFeatures) {
		stage = arabicWin1256Plan[:]
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

// arabicFallbackLookups is the face's fallback lookups: first its synthesized
// ones, one per entry of arabicFallbackFeatures and of kind 0 where the face
// maps nothing that one could be built from; and after them, for a face that
// builds none of those and whose character map is Windows-1256's, the lookups
// of arabicWin1256, one per entry of arabicWin1256Plan. They depend on the
// character map alone, so they are built once per face and shared by its
// clones.
//
// Which of the two a face is given is HarfBuzz's order,
// arabic_fallback_plan_create: the character map's lookups if it builds any —
// a ligature alone is enough — and the table otherwise.
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
		built := false
		for _, lk := range out {
			built = built || lk.kind != 0
		}
		if !built && f.looksLikeWin1256() {
			out = append(out, arabicWin1256Lookups()...)
		}
		c.arabicLookups = out
	})
	return c.arabicLookups
}

// hasFallbackForms reports whether the fallback can draw any joining form in
// this face: whether it maps a letter and one of its forms, or is a
// Windows-1256 face, for which the table draws them.
func (f *Face) hasFallbackForms() bool {
	lookups := f.arabicFallbackLookups()
	for _, lk := range lookups[:4] {
		if lk.kind != 0 {
			return true
		}
	}
	return len(lookups) > len(arabicFallbackFeatures)
}

// arabicWin1256Plan is arabicWin1256's lookups as a plan applies them, in the
// order of HarfBuzz's manifest, each for the glyphs its feature is: the
// lam-alef ligatures under 'rlig', then the three forms, then the shadda
// ligatures under 'rlig' again. There is no 'isol': an isolated letter is the
// glyph the character map gives it. The indices are into the face's fallback
// lookups, after the seven arabicFallbackFeatures names.
var arabicWin1256Plan = [...]planLookup{
	{index: len(arabicFallbackFeatures) + 0, mask: 0},
	{index: len(arabicFallbackFeatures) + 1, mask: maskInit},
	{index: len(arabicFallbackFeatures) + 2, mask: maskMedi},
	{index: len(arabicFallbackFeatures) + 3, mask: maskFina},
	{index: len(arabicFallbackFeatures) + 4, mask: 0},
}

// looksLikeWin1256 is HarfBuzz's test for a face encoded as Windows-1256
// (arabic_fallback_plan_init_win1256): its character map puts alef, lam, alef
// maksura, yeh and sukun at the glyphs whose numbers are their bytes in that
// code page. Five characters, all five exactly, and nothing else is asked.
func (f *Face) looksLikeWin1256() bool {
	for _, c := range [...]struct {
		r   rune
		gid int
	}{{0x0627, 199}, {0x0644, 225}, {0x0649, 236}, {0x064A, 237}, {0x0652, 250}} {
		if g, ok := f.GlyphID(c.r); !ok || g != c.gid {
			return false
		}
	}
	return true
}

// win1256Single is a single substitution of HarfBuzz's Windows-1256 table: the
// glyphs it covers, in the table's order, and what each becomes.
type win1256Single struct{ from, to []int }

// win1256Ligature is a ligature of that table: its parts, and what they become.
type win1256Ligature struct {
	parts []int
	lig   int
}

// The subtables of hb-ot-shaper-arabic-win1256.hh, glyph for glyph. Every
// number is a glyph, and a glyph's number is its byte in Windows-1256 where
// the face's letters are: 198 is yeh with hamza above, 225 lam, 199 alef. The
// forms they become the table names by number alone — numbers that are other
// characters in the code page, or none — which is where such a face drew
// them. The final alef becomes glyph 0, as the table says.
var (
	win1256InitMedi = win1256Single{
		from: []int{198, 200, 201, 202, 203, 204, 205, 206, 211, 212, 213, 214, 223, 225, 227, 228, 236, 237},
		to:   []int{162, 4, 5, 5, 6, 7, 9, 11, 13, 14, 15, 26, 140, 141, 142, 143, 154, 154},
	}
	win1256Init = win1256Single{
		from: []int{218, 219, 221, 222, 229},
		to:   []int{27, 30, 128, 131, 144},
	}
	win1256Medi = win1256Single{
		from: []int{218, 219, 221, 222, 229},
		to:   []int{28, 31, 129, 138, 149},
	}
	win1256Fina = win1256Single{
		from: []int{194, 195, 197, 198, 199, 201, 204, 205, 206, 218, 219, 229, 236, 237},
		to:   []int{2, 1, 3, 181, 0, 159, 8, 10, 12, 29, 127, 152, 160, 156},
	}
	win1256MediFinaLamAlef = win1256Single{
		from: []int{165, 178, 180, 252},
		to:   []int{170, 179, 185, 255},
	}
	// The lam-alefs: lam with alef, with alef with hamza above and below, and
	// with alef with madda.
	win1256LamAlef = []win1256Ligature{
		{[]int{225, 199}, 165}, {[]int{225, 195}, 178}, {[]int{225, 194}, 180}, {[]int{225, 197}, 252},
	}
	// Shadda with fatha, damma and kasra.
	win1256Shadda = []win1256Ligature{
		{[]int{248, 243}, 172}, {[]int{248, 245}, 173}, {[]int{248, 246}, 175},
	}
)

// arabicWin1256 is HarfBuzz's Windows-1256 table as lookups, one per entry of
// arabicWin1256Plan: the lam-alef ligatures, 'init', 'medi' and 'fina', and
// the shadda ligatures. A lookup that is more than one subtable tries them in
// order, as a font's does, and the table shares one subtable between 'init'
// and 'medi' and another between 'medi' and 'fina'. Every lookup but the
// shadda ligatures ignores marks. The bytes are the table's own: format 1
// coverage in the order written, whatever serializeCoverage would choose.
var arabicWin1256 = func() [5]rawLookup {
	single := func(subs ...win1256Single) rawLookup {
		lk := rawLookup{kind: 1, flags: flagIgnoreMarks, markSet: -1}
		for _, st := range subs {
			lk.subs = append(lk.subs, singleSubstFormat2(st.from, st.to, coverageFormat1))
		}
		return lk
	}
	ligature := func(ligs []win1256Ligature, flags int) rawLookup {
		// Each of the table's two has one first glyph, so one set.
		set := make([][]int, len(ligs))
		for i, l := range ligs {
			set[i] = append([]int{l.lig}, l.parts[1:]...)
		}
		sub := ligatureSubstFormat1([]int{ligs[0].parts[0]}, [][][]int{set}, coverageFormat1)
		return rawLookup{kind: 4, flags: flags, markSet: -1, subs: [][]byte{sub}}
	}
	return [5]rawLookup{
		ligature(win1256LamAlef, flagIgnoreMarks),
		single(win1256InitMedi, win1256Init),
		single(win1256InitMedi, win1256Medi, win1256MediFinaLamAlef),
		single(win1256Fina, win1256MediFinaLamAlef),
		ligature(win1256Shadda, 0),
	}
}()

// arabicWin1256Lookups is arabicWin1256 as a slice a face's lookups can be
// extended by. The lookups hold no state, so every face shares them.
func arabicWin1256Lookups() []rawLookup { return arabicWin1256[:] }

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
	from, to := make([]int, len(pairs)), make([]int, len(pairs))
	for i, p := range pairs {
		from[i], to[i] = p.from, p.to
	}
	sub := singleSubstFormat2(from, to, serializeCoverage)
	return rawLookup{kind: 1, flags: flagIgnoreMarks, markSet: -1, subs: [][]byte{sub}}
}

// singleSubstFormat2 writes a single substitution taking each glyph of from to
// the glyph of to beside it: format, coverage offset, count, the substitutes,
// and the coverage after them, written by cov.
func singleSubstFormat2(from, to []int, cov func([]int) []byte) []byte {
	sub := make([]byte, 6+2*len(to))
	put16(sub, 0, 2)
	put16(sub, 2, len(sub))
	put16(sub, 4, len(to))
	for i, g := range to {
		put16(sub, 6+2*i, g)
	}
	return append(sub, cov(from)...)
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
	firsts, ligs := make([]int, len(sets)), make([][][]int, len(sets))
	for i, s := range sets {
		firsts[i], ligs[i] = s.first, s.ligs
	}
	sub := ligatureSubstFormat1(firsts, ligs, serializeCoverage)
	return rawLookup{kind: 4, flags: flags, markSet: -1, subs: [][]byte{sub}}
}

// ligatureSubstFormat1 writes a ligature substitution with one set of
// ligatures for each glyph of firsts, each ligature its glyph and then its
// parts after the first: format, coverage offset, set count, the set offsets;
// then each set — a count and its ligatures' offsets — and each ligature — its
// glyph, its part count and the parts after the first; then the coverage,
// written by cov.
func ligatureSubstFormat1(firsts []int, sets [][][]int, cov func([]int) []byte) []byte {
	sub := make([]byte, 6+2*len(sets))
	put16(sub, 0, 1)
	put16(sub, 4, len(sets))
	for i, ligs := range sets {
		setAt := len(sub)
		put16(sub, 6+2*i, setAt)
		sub = append(sub, make([]byte, 2+2*len(ligs))...)
		put16(sub, setAt, len(ligs))
		for k, lig := range ligs {
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
	return append(sub, cov(firsts)...)
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
		return coverageFormat1(glyphs)
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

// coverageFormat1 writes a coverage table that lists the glyphs, in the order
// given.
func coverageFormat1(glyphs []int) []byte {
	out := make([]byte, 4+2*len(glyphs))
	put16(out, 0, 1)
	put16(out, 2, len(glyphs))
	for i, g := range glyphs {
		put16(out, 4+2*i, g)
	}
	return out
}

// put16 writes a big-endian sixteen-bit value.
func put16(b []byte, at, v int) {
	b[at] = byte(v >> 8)
	b[at+1] = byte(v)
}
