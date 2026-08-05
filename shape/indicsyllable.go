package shape

// Cutting a run of Devanagari into syllables.
//
// A syllable is the unit everything in indic.go works on: the base consonant is
// found within one, a matra moves within one, a reph belongs to one, and each
// of the font's Indic features is applied to one at a time so that a ligature
// cannot join two of them together. So the run has to be cut before anything
// else can happen, and cut from the characters alone — there are no glyphs yet
// that would say where a syllable ends.
//
// # The grammar
//
// OpenType's Indic model states the shape of a syllable as a regular grammar,
// and the productions below are that grammar written out, one function each.
// Reading them against the model:
//
//	c            = C | Ra
//	n            = N N?                        a nukta, and Unicode allows two
//	z            = ZWJ | ZWNJ
//	cn           = c ZWJ? n?
//	halant_group = z? H (ZWJ n?)?
//	matra_group  = z* M N? H?
//	syllable_tail= (z? SM SM? ZWNJ?)? VD*
//	complex_tail = (halant_group cn)* CM? (halant_group | H ZWNJ | matra_group*) syllable_tail
//
//	REPH               = Ra H | Repha
//
//	consonant_syllable = (Repha|CS)? cn complex_tail
//	vowel_syllable     = REPH? V n? (ZWJ | complex_tail)
//	standalone_cluster = (Repha|CS)? PLACEHOLDER n? complex_tail
//	                   | REPH? DOTTEDCIRCLE n? complex_tail
//	symbol_cluster     = Symbol N? syllable_tail
//	broken_cluster     = REPH? n? complex_tail
//
// The alternatives are tried in that order, which is what makes the grammar
// decidable by a plain left-to-right scan rather than by backtracking: only one
// of them can start at any character, because the first symbol of each is
// disjoint from the others. The one place that is not true — a vowel followed
// by a joiner, where the joiner may end the syllable or may open a virama group
// — is decided by looking one character further, which is noted where it
// happens.
//
// A leading Ra + virama is a REPH, and three of the alternatives admit one. In a
// *consonant* syllable it needs no alternative of its own — Ra is a consonant, so
// the syllable opens with one and the virama that follows is the first of its
// virama groups, and whether that Ra becomes a reph is settled when the base is
// found: it is excluded from the candidates when the syllable has another
// consonant to be the base instead.
//
// The other three have no base consonant to run that rule over, so the reph has
// to be in the cut or it is nowhere. This file said for a long time that the
// consonant syllable covered every case, and it does not: it stops at the virama
// and leaves what follows to open a syllable of its own, so
//
//	Ra H AA        was cut as [Ra H][AA]  and is one vowel syllable
//	Ra H <matra>   was cut as [Ra H][M]   and is one broken cluster
//
// which is a different reph, a different base and — where the cluster is broken —
// a different place for the dotted circle. Both are the shape differential
// fuzzing kept reporting against HarfBuzz.
//
// The placeholder takes no REPH and the dotted circle does, which is why the
// standalone alternative is written as two. Ra + virama + NBSP is a consonant
// syllable and then a standalone cluster, and both engines agree that it is.
//
// # What is not a syllable
//
// A character of no Indic category at all — a Latin letter, a space, a danda —
// is its own one-character cluster and is left exactly as it was. So is a symbol
// cluster. Neither is reordered.

// indicSyllableKind says what a cluster is, which decides whether it is
// reordered at all.
type indicSyllableKind uint8

const (
	sylNonIndic   indicSyllableKind = iota // left alone
	sylSymbol                              // left alone
	sylConsonant                           // the ordinary case
	sylVowel                               // an independent vowel and its dependents
	sylStandalone                          // hung off a placeholder or a dotted circle
	sylBroken                              // dependents with no base of their own
)

// indicSyllable is one cluster of a run: the half-open range of characters it
// covers, and what kind it is.
type indicSyllable struct {
	start, end int
	kind       indicSyllableKind
}

// indicSyllables cuts a run of characters into syllables. The result covers the
// input exactly and in order, and every entry is at least one character long,
// so a caller walking it always makes progress.
func indicSyllables(cats []indicCat) []indicSyllable {
	var out []indicSyllable
	for i := 0; i < len(cats); {
		s := indicScanSyllable(cats, i)
		if s.end-s.start > maxIndicSyllable {
			s.end = s.start + maxIndicSyllable
		}
		if s.end <= s.start {
			s.end = s.start + 1 // a scanner that consumed nothing would not terminate
		}
		out = append(out, s)
		i = s.end
	}
	return out
}

// indicScanSyllable matches one syllable starting at a position, taking the
// grammar's alternatives in order.
func indicScanSyllable(cats []indicCat, start int) indicSyllable {
	n := len(cats)

	if cats[start] == catSymbol {
		// One nukta after a symbol, not two — N? rather than n. The avagraha is
		// not a letter and does not take the pair a consonant does; a second
		// nukta belongs to no syllable, so it opens a broken one of its own and
		// is shown against a dotted circle. Both HarfBuzz and CoreText break
		// here, and this took the pair until they were asked.
		i := indicTakeOneNukta(cats, start+1)
		return indicSyllable{start, indicTakeTail(cats, i), sylSymbol}
	}

	// The consonant syllable's extent, measured before it is taken.
	//
	// A leading Ra can open a consonant syllable and a reph-led one both, so the
	// two have to be compared rather than ordered: the grammar takes the longer
	// match, and where they are the same length the alternatives' own order
	// decides, which puts the consonant syllable first. Ra + virama + anusvara is
	// that tie — the consonant syllable takes the modifier in its tail and the
	// broken cluster takes it in the same tail, three characters either way — and
	// it is a consonant syllable with the Ra as its base, not a reph over
	// nothing.
	consEnd, consOK := start, false
	p := start
	if cats[p] == catRepha || cats[p] == catCS {
		p++
	}
	if j, ok := indicTakeConsonant(cats, p); ok {
		consEnd, consOK = indicTakeComplexTail(cats, j), true
	}

	// The three alternatives that admit a leading reph. A vowel and a dotted
	// circle are each past anything the consonant syllable could reach, since it
	// can take neither, so those two win outright; the broken cluster has to be
	// strictly longer.
	//
	// An ordinary conjunct — a Ra whose virama is followed by a consonant —
	// matches none of them and falls through, and its reph is settled when the
	// base is found instead.
	if r, ok := indicTakeReph(cats, start); ok {
		if r < n && cats[r] == catVowel {
			return indicSyllable{start, indicTakeVowelTail(cats, r), sylVowel}
		}
		if r < n && cats[r] == catDottedCircle {
			i := indicTakeNukta(cats, r+1)
			return indicSyllable{start, indicTakeComplexTail(cats, i), sylStandalone}
		}
		i := indicTakeNukta(cats, r)
		if j := indicTakeComplexTail(cats, i); j > consEnd {
			return indicSyllable{start, j, sylBroken}
		}
	}

	// consonant_syllable, and with it every syllable that opens with a Ra whose
	// virama is followed by a consonant.
	if consOK {
		return indicSyllable{start, consEnd, sylConsonant}
	}
	if p < n && (cats[p] == catPlaceholder || cats[p] == catDottedCircle) {
		i := indicTakeNukta(cats, p+1)
		return indicSyllable{start, indicTakeComplexTail(cats, i), sylStandalone}
	}

	p = start
	if cats[p] == catRepha {
		p++
	}
	if p < n && cats[p] == catVowel {
		return indicSyllable{start, indicTakeVowelTail(cats, p), sylVowel}
	}

	// broken_cluster: dependents with nothing to depend on. It still gets the
	// full treatment, because the font's rules are written about the marks
	// whatever they are attached to.
	i := indicTakeNukta(cats, p)
	if j := indicTakeComplexTail(cats, i); j > start {
		return indicSyllable{start, j, sylBroken}
	}
	return indicSyllable{start, start + 1, sylNonIndic}
}

// n = N N?
func indicTakeNukta(cats []indicCat, i int) int {
	if i < len(cats) && cats[i] == catNukta {
		i = indicTakeOneNukta(cats, i)
		i = indicTakeOneNukta(cats, i)
	}
	return i
}

// N? — a single nukta, which is what everything but a letter admits. The pair
// belongs to a consonant or an independent vowel; a vowel sign and a symbol
// each take one.
func indicTakeOneNukta(cats []indicCat, i int) int {
	if i < len(cats) && cats[i] == catNukta {
		i++
	}
	return i
}

// REPH = Ra H | Repha
//
// The virama is taken with indicIsHalant, which is the same reading the base
// rule uses when it decides whether a consonant syllable's Ra becomes a reph, so
// the two cannot disagree about what opens one.
//
// Nothing here excludes Ra + virama + ZWJ, the eyelash Ra that must not become a
// reph. It does not need to be excluded: the joiner is not a vowel and not a
// dotted circle, and a complex tail that begins with one takes nothing, so every
// branch that consults this declines and the sequence falls to the consonant
// syllable — which is where the eyelash was already handled.
func indicTakeReph(cats []indicCat, i int) (int, bool) {
	if i < len(cats) && cats[i] == catRepha {
		return i + 1, true
	}
	if i+1 < len(cats) && cats[i] == catRa && indicIsHalant(cats[i+1]) {
		return i + 2, true
	}
	return i, false
}

// V n? (ZWJ | complex_tail), taken from the vowel.
//
// A joiner right after the vowel may end the syllable, or may open the tail and
// carry it on: a virama group, a matra group and the modifier tail each admit a
// leading joiner. The two alternatives are not disjoint, so this is the one
// place the cut takes the *longer* of them rather than the first that matches —
// which is what the grammar's scanner does everywhere, and what every other
// production here gets without having to say so.
func indicTakeVowelTail(cats []indicCat, v int) int {
	i := indicTakeNukta(cats, v+1)
	if j := indicTakeComplexTail(cats, i); j > i {
		return j
	}
	if i < len(cats) && cats[i] == catZWJ {
		return i + 1
	}
	return i
}

// cn = c ZWJ? n?
func indicTakeConsonant(cats []indicCat, i int) (int, bool) {
	if i >= len(cats) || (cats[i] != catConsonant && cats[i] != catRa) {
		return i, false
	}
	i++
	if i < len(cats) && cats[i] == catZWJ {
		i++
	}
	return indicTakeNukta(cats, i), true
}

// halant_group = z? H (ZWJ n?)?
func indicTakeHalant(cats []indicCat, i int) (int, bool) {
	j := i
	if j < len(cats) && indicIsJoiner(cats[j]) {
		j++
	}
	if j >= len(cats) || !indicIsHalant(cats[j]) {
		return i, false
	}
	j++
	if j < len(cats) && cats[j] == catZWJ {
		j = indicTakeNukta(cats, j+1)
	}
	return j, true
}

// final_halant_group = halant_group | H ZWNJ
func indicTakeFinalHalant(cats []indicCat, i int) (int, bool) {
	if i+1 < len(cats) && indicIsHalant(cats[i]) && cats[i+1] == catZWNJ {
		return i + 2, true
	}
	return indicTakeHalant(cats, i)
}

// matra_group = z* (M | SM? MPst) N? H?
//
// The modifier before the vowel sign is Gurmukhi's: the bindi is written before
// the II sign it belongs with, and only that kind of sign admits one.
func indicTakeMatra(cats []indicCat, i int) (int, bool) {
	j := i
	for j < len(cats) && indicIsJoiner(cats[j]) {
		j++
	}
	if j+1 < len(cats) && indicIsModifier(cats[j]) && cats[j+1] == catMPst {
		j++
	}
	if j >= len(cats) || !indicIsMatra(cats[j]) {
		return i, false
	}
	// One nukta after a vowel sign, not two. Two is what a *consonant* admits —
	// n is N N? there — and a matra admits a single one. A second belongs to no
	// syllable, so it starts a broken one of its own and is shown against a
	// dotted circle, which is how a reader is told the text is malformed.
	j = indicTakeOneNukta(cats, j+1)
	if j < len(cats) && indicIsHalant(cats[j]) {
		j++
	}
	return j, true
}

// syllable_tail = (z? SM SM? ZWNJ?)? VD*
//
// The cantillation marks are a plain repetition rather than a bounded pair: a
// Vedic line carries several over one syllable, and the syllable cap in
// indicSyllables is what keeps the repetition from running away.
func indicTakeTail(cats []indicCat, i int) int {
	j := i
	if j < len(cats) && indicIsJoiner(cats[j]) {
		j++
	}
	if j < len(cats) && indicIsModifier(cats[j]) {
		j++
		if j < len(cats) && indicIsModifier(cats[j]) {
			j++
		}
		if j < len(cats) && cats[j] == catZWNJ {
			j++
		}
		i = j
	}
	for i < len(cats) && cats[i] == catVD {
		i++
	}
	return i
}

// complex_tail = (halant_group cn)* CM? (final_halant_group | matra_group*) syllable_tail
//
// The loop is where conjuncts come from: a virama binds the consonant after it
// into the same syllable, however many times it is repeated. A virama with no
// consonant after it does not — it ends the syllable instead, which is the
// final virama group below.
func indicTakeComplexTail(cats []indicCat, i int) int {
	for {
		j, ok := indicTakeHalant(cats, i)
		if !ok {
			break
		}
		k, ok := indicTakeConsonant(cats, j)
		if !ok {
			break
		}
		i = k
	}
	if i < len(cats) && cats[i] == catCM {
		i++
	}
	if j, ok := indicTakeFinalHalant(cats, i); ok {
		i = j
	} else {
		for {
			j, ok := indicTakeMatra(cats, i)
			if !ok {
				break
			}
			i = j
		}
	}
	return indicTakeTail(cats, i)
}
