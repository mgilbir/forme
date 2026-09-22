package shape

import "github.com/mgilbir/forme/font"

// Contextual substitution: rules that fire only where a glyph has particular
// neighbours.
//
// Everything else this package reads can be flattened at load — a kern pair is
// a pair, a ligature is a run — because each rule stands alone. A contextual
// rule does not. It says "where this sequence occurs, apply lookup number seven
// at position two", and lookup seven is another lookup with its own type and
// its own subtables. So the lookups have to be kept as things that can be
// *applied*, at a position, on demand, and possibly from inside one another.
//
// That is what this file adds, and why it could not be bolted onto the flat
// tables: the indirection is the feature. 'calt' — contextual alternates, the
// feature that swaps a glyph for a better-fitting variant next to particular
// neighbours — does nothing without it, and it is the most widely used feature
// in modern Latin fonts after kerning and ligatures.
//
// # What is matched
//
// All six forms: sequence context by glyph, by class and by coverage, and the
// chained versions of each, which also match what comes before and after the
// part being replaced. Backtrack sequences are stored nearest-first, which is
// the format's convention and reads backwards from every other list here.

// rawLookup is a lookup kept whole, so that a contextual rule can name it.
type rawLookup struct {
	kind  int
	flags int
	// markSet is the index of the mark glyph set this lookup filters by, or -1.
	// A lookup that names one sees only the marks in it and steps over every
	// other, which is a narrower thing than the mark attachment class in the
	// flags and is stated separately from them.
	markSet int
	subs    [][]byte
}

// applyContextual runs the substitution lookups of a feature over a buffer,
// left to right, applying each where it matches.
//
// A lookup may replace a run with fewer glyphs, so the position advances by
// what the lookup consumed rather than by one.
func (sh shaper) applyContextual(buf []Glyph, lookups []int) []Glyph {
	for _, idx := range lookups {
		// The walk is always at the front of what is left: what it has passed
		// is settled, and a lookup that changes the run's length gives its room
		// back to the gap between the two rather than closing it. See runBuf.
		rb := newRunBuf(buf, 0)
		sh.run = rb
		for len(rb.pending()) > 0 {
			was := len(rb.pending())
			consumed, _ := sh.applyGSUBAt(idx, rb.pending(), 0, 0)
			if consumed > 0 {
				rb.settle(consumed)
				continue
			}
			// A lookup that consumed nothing and yet shortened the buffer took
			// a glyph out. The position is not advanced past what followed it,
			// because what followed it is now here and has not been looked at.
			if len(rb.pending()) < was {
				continue
			}
			rb.settle(1)
		}
		buf = rb.flatten()
	}
	return buf
}

// maxLookupRecursion bounds how deeply a contextual rule may call into another.
// A font can describe a cycle — rule A applying lookup B which applies A — and
// nothing in the format forbids it, so the depth is what stops it.
const maxLookupRecursion = 8

// The allowance for applying a lookup from inside another, spent across one
// run.
//
// Depth alone does not bound this work, and for a while it was all that did. A
// matched rule names a list of lookup records, each saying "apply lookup N at
// position P", and nothing stops those records from naming the rule's own
// lookup — forty of them, forty times over, eight levels deep, is forty to the
// eighth applications from a few hundred bytes of GSUB. It is not a cycle the
// depth catches; it is a tree, and the depth bounds its height while the
// records decide its width.
//
// So the run carries an allowance and every recursion spends one unit of it,
// which is what HarfBuzz does and with the same numbers. The size is chosen to
// be unreachable by a font that means well: a thousand nested applications per
// glyph is orders of magnitude beyond what the most elaborate real script needs,
// and the floor keeps a short run from being held to a small number. When it
// runs out the remaining nested lookups do nothing, which is the same answer as
// a rule that did not match.
const (
	lookupOpsPerGlyph = 1024
	lookupOpsFloor    = 16384
	lookupOpsCeiling  = 1 << 29
)

// lookupBudget is one run's allowance, sized from the glyphs it holds.
func lookupBudget(glyphs int) *int {
	n := glyphs * lookupOpsPerGlyph
	if n < lookupOpsFloor {
		n = lookupOpsFloor
	}
	if n > lookupOpsCeiling {
		n = lookupOpsCeiling
	}
	return &n
}

// recurse reports whether a matched rule may apply another lookup, spending one
// unit of the run's allowance when it may.
//
// A shaper with no allowance at all cannot recurse. That is the safe answer for
// a value assembled by hand rather than by the one place that builds one: an
// unbudgeted shaper is the state this bound exists to make impossible, so it is
// treated as one that has already spent everything.
func (sh shaper) recurse() bool {
	if sh.ops == nil || *sh.ops <= 0 {
		return false
	}
	*sh.ops--
	return true
}

// applyGSUBAt applies one GSUB lookup at a position, returning how many glyphs
// from the position it consumed — for a contextual rule, the whole span it
// matched, see runRecords — (zero when it did not match) and the resulting
// buffer.
//
// A position outside the buffer, on either side, applies nothing. Nothing is
// meant to ask for one — runRecords keeps its positions inside the buffer — but
// every position a nested rule reaches here was computed from counts a font
// stated, and a guard at the one door costs nothing beside an index out of range
// in the middle of Compose, which is what the missing lower bound once was.
func (sh shaper) applyGSUBAt(idx int, buf []Glyph, at, depth int) (int, []Glyph) {
	if depth > maxLookupRecursion || idx < 0 || idx >= len(sh.l.gsub) || at < 0 || at >= len(buf) {
		return 0, buf
	}
	lk := sh.l.gsub[idx]
	// A lookup's flags say which glyphs it does not look at, and that includes
	// the one it would start at. A rule that ignores marks does not apply *to* a
	// mark either — the whole of its input is chosen by the flag, not just the
	// part it steps over on the way.
	sh.markSet = lk.markSet
	if sh.ignores(lk.flags, buf[at]) {
		return 0, buf
	}
	for _, sub := range lk.subs {
		switch lk.kind {
		case 1:
			if gid, ok := singleSubstAt(sub, buf[at].GID); ok {
				buf[at].GID = gid
				buf[at].XAdvance = sh.f.advanceGID(gid)
				return 1, buf
			}
		case 2:
			// One glyph becomes several: a decomposition, which is what 'ccmp'
			// is usually written with. They share the original's cluster —
			// several glyphs standing for one character is exactly the case
			// clusters exist to record.
			reps, ok := multipleSubstAt(sub, buf[at].GID)
			// A sequence of no glyphs at all is a deletion.
			//
			// The format's own text forbids using this to delete a glyph, and
			// fonts do it anyway: Noto Sans Javanese carries a placeholder mark
			// through its rules and takes it off again with exactly this, under
			// 'rlig'. Reading the prohibition as permission to ignore the rule
			// leaves that placeholder on the page — a mark the font said to
			// remove, drawn beside every letter that went through the rule.
			//
			// Nothing was consumed, so the caller stays where it is: what
			// followed has moved into this place and has not been looked at.
			if ok && len(reps) == 0 {
				out := sh.replace(buf, at, 1, nil)
				sh.deleted(at)
				return 0, out
			}
			// The guard is what keeps the two apart. Without it this branch
			// would take the empty sequence too and quietly do the right thing
			// to the buffer and the wrong thing to the record beside it,
			// reporting a ligature where a glyph was removed.
			if ok && len(reps) > 0 {
				product := sh.product(len(reps))
				for _, gid := range reps {
					// Each part still stands for the character the whole stood
					// for, so it is classified as that character was and takes
					// the same positional form. The second is what makes a
					// cursive script work at all: 'ccmp' splitting a letter into
					// a skeleton and its dots must leave the skeleton still
					// knowing it is the first letter of a word, because that is
					// the glyph the font states the form over.
					product = append(product, Glyph{
						GID: gid, Cluster: buf[at].Cluster, XAdvance: sh.f.advanceGID(gid),
						class: buf[at].class, join: buf[at].join,
					})
				}
				out := sh.replace(buf, at, 1, product)
				sh.resized(at, len(reps)-1)
				return len(reps), out
			}
		case 3:
			if gid, ok := alternateSubstAt(sub, buf[at].GID); ok {
				buf[at].GID = gid
				buf[at].XAdvance = sh.f.advanceGID(gid)
				return 1, buf
			}
		case 4:
			if comps, gid, ok := sh.ligatureAt(sub, buf, at, lk.flags); ok {
				return sh.formLigature(buf, at, gid, comps)
			}
		case 5:
			if n, out, ok := sh.sequenceContext(sub, buf, at, lk.flags, depth); ok {
				return n, out
			}
		case 6:
			if n, out, ok := sh.chainedContext(sub, buf, at, lk.flags, depth); ok {
				return n, out
			}
		}
	}
	return 0, buf
}

// singleSubstAt reads a type 1 subtable and reports the replacement for a
// glyph, if it covers one.
func singleSubstAt(sub []byte, gid int) (int, bool) {
	if len(sub) < 6 {
		return 0, false
	}
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gid)
	if !ok {
		return 0, false
	}
	switch font.Be16(sub, 0) {
	case 1:
		to := gid + signed16(font.Be16(sub, 4))
		if to < 0 || to > 0xFFFF {
			return 0, false
		}
		return to, true
	case 2:
		if 6+2*i+2 > len(sub) || i >= font.Be16(sub, 4) {
			return 0, false
		}
		return font.Be16(sub, 6+2*i), true
	}
	return 0, false
}

// multipleSubstAt reads a type 2 subtable and reports the glyphs that replace
// one, if it covers it.
func multipleSubstAt(sub []byte, gid int) ([]int, bool) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return nil, false
	}
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gid)
	if !ok || i >= font.Be16(sub, 4) || 6+2*i+2 > len(sub) {
		return nil, false
	}
	off := font.Be16(sub, 6+2*i)
	if off <= 0 || off+2 > len(sub) {
		return nil, false
	}
	seq := sub[off:]
	n := font.Be16(seq, 0)
	// A sequence long enough to be a decompression bomb is malformed; a real
	// one is two or three glyphs.
	if n < 0 || n > maxSubstitutionLength || 2+2*n > len(seq) {
		return nil, false
	}
	out := make([]int, n)
	for k := range out {
		out[k] = font.Be16(seq, 2+2*k)
	}
	return out, true
}

// maxSubstitutionLength bounds what one glyph may become. A decomposition is a
// handful of glyphs; a font declaring thousands is describing an attack, and
// nothing stops it from doing so at every position in a run.
const maxSubstitutionLength = 64

// alternateSubstAt reads a type 3 subtable, which offers a choice of glyphs.
//
// The first is taken. Choosing among them is what 'aalt' is for and what a
// caller asks for by name; a lookup reached through a default feature has no
// one to ask, and the font lists its own preference first.
func alternateSubstAt(sub []byte, gid int) (int, bool) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return 0, false
	}
	i, ok := coverageIndex(sub, font.Be16(sub, 2), gid)
	if !ok || i >= font.Be16(sub, 4) || 6+2*i+2 > len(sub) {
		return 0, false
	}
	off := font.Be16(sub, 6+2*i)
	if off <= 0 || off+4 > len(sub) {
		return 0, false
	}
	set := sub[off:]
	if font.Be16(set, 0) < 1 {
		return 0, false
	}
	return font.Be16(set, 2), true
}

// ligatureAt reads a type 4 subtable and reports the ligature starting at a
// position, together with the positions of the glyphs it is made of.
//
// The positions rather than a count, because a lookup that ignores marks
// matches its components *across* them and the ones in between are not part of
// the rule. Only what the font named is replaced; formLigature keeps the rest.
func (sh shaper) ligatureAt(sub []byte, buf []Glyph, at, flags int) ([]int, int, bool) {
	if len(sub) < 6 || font.Be16(sub, 0) != 1 {
		return nil, 0, false
	}
	i, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
	if !ok || i >= font.Be16(sub, 4) || 6+2*i+2 > len(sub) {
		return nil, 0, false
	}
	off := font.Be16(sub, 6+2*i)
	if off <= 0 || off+2 > len(sub) {
		return nil, 0, false
	}
	set := sub[off:]
	for j := 0; j < font.Be16(set, 0); j++ {
		if 2+2*j+2 > len(set) {
			break
		}
		lo := font.Be16(set, 2+2*j)
		if lo <= 0 || lo+4 > len(set) {
			continue
		}
		lig := set[lo:]
		compCount := font.Be16(lig, 2)
		if compCount < 1 || compCount > maxLigatureComponents {
			continue
		}
		// Match the components against the glyphs after this one, skipping
		// those the lookup ignores.
		//
		// The positions are collected into an array on the stack and copied out
		// only if the whole ligature matched. Most candidates do not — a font
		// lists every ligature beginning with a glyph, and a run matches at most
		// one — so allocating per candidate is allocating for the failures.
		var found [maxLigatureComponents]int
		found[0] = at
		n := 1
		pos := at
		matched := true
		for k := 0; k < compCount-1; k++ {
			if 4+2*k+2 > len(lig) {
				matched = false
				break
			}
			want := font.Be16(lig, 4+2*k)
			pos = sh.nextNotIgnored(buf, pos+1, flags, want)
			if pos >= sh.end(buf) || buf[pos].GID != want {
				matched = false
				break
			}
			found[n] = pos
			n++
		}
		if matched {
			comps := make([]int, n)
			copy(comps, found[:n])
			return comps, font.Be16(lig, 0), true
		}
	}
	return nil, 0, false
}

// formLigature replaces the glyphs a ligature names with the ligature glyph,
// keeping everything the lookup stepped over.
//
// The skipped glyphs are moved to after the ligature, in the order they were
// in. They have to go somewhere — a mark cannot stay between two glyphs that
// are now one — and after is where OpenType puts them and where a mark
// attachment lookup will then find them, since it looks back for its base.
//
// All of it becomes one cluster. The ligature and the marks that were inside it
// came from a stretch of text that can no longer be divided: the accent belongs
// to a letter that is now half a glyph, so there is no position between them for
// a caret to sit at, and saying otherwise would let a caller offer one.
//
// It reports 1 consumed, not the span. The walk continues at the glyph after
// the ligature, which is the first thing that was kept — the next lookup gets to
// see it, and the walk always moves, so a font whose ligature product matches
// its own rule again cannot hold it in place.
func (sh shaper) formLigature(buf []Glyph, at, gid int, comps []int) (int, []Glyph) {
	last := comps[len(comps)-1]

	// The cluster is the earliest of everything the ligature spans, kept glyphs
	// included: it is where the indivisible stretch of text begins.
	cluster := buf[at].Cluster
	for i := at; i <= last; i++ {
		if buf[i].Cluster < cluster {
			cluster = buf[i].Cluster
		}
	}

	isComponent := make([]bool, last-at+1)
	for _, p := range comps {
		isComponent[p-at] = true
	}

	// A ligature of a letter and its own marks is not a ligature in the sense
	// that matters here. Nothing was joined that a mark could belong to *part*
	// of, so there is no component for a later mark to be placed against, and
	// giving it a number would only make the marks inside it look like they
	// belonged to something they do not.
	joined := false
	for _, p := range comps[1:] {
		if !sh.l.isMark(buf[p]) {
			joined = true
			break
		}
	}
	id, comps0 := 0, 1
	if joined {
		id = sh.nextLigatureID()
		for _, p := range comps {
			comps0 += componentsOf(buf[p])
		}
		comps0-- // the count started at one for the first component
	}

	product := sh.product(last - at + 1)
	// What the product is, for a font that classifies nothing itself. Several
	// letters drawn as one glyph is a ligature; a letter drawn together with its
	// own marks is still that letter, which is the same distinction the
	// numbering above turns on.
	class := buf[at].class
	if joined {
		class = classLigature
	}
	product = append(product, Glyph{
		GID: gid, Cluster: cluster, XAdvance: sh.f.advanceGID(gid),
		lig: ligatureRef{id: id, comps: comps0}, class: class,
	})

	// Walking the components in order, so that each kept glyph is given the
	// part of the ligature it stood between. A mark before the second component
	// belongs to the first, and so on; a ligature made of ligatures counts each
	// of their parts, which is why the running total is of components rather
	// than of glyphs.
	soFar := componentsOf(buf[at])
	for i := at + 1; i <= last; i++ {
		if isComponent[i-at] {
			soFar += componentsOf(buf[i])
			continue
		}
		g := buf[i]
		g.Cluster = cluster
		if id != 0 {
			g.lig = ligatureRef{id: id, comp: soFar, comps: componentsOf(g)}
		}
		product = append(product, g)
	}

	span := last - at + 1
	out := sh.replace(buf, at, span, product)
	sh.resized(at, len(product)-span)
	return 1, out
}

// nextNotIgnored is the next position a lookup with these flags looks at while
// matching its input.
//
// want is the glyph the caller is about to compare against. A join control
// standing in the way is stepped over — unless the lookup names that very glyph,
// in which case it is what the lookup was looking for and is matched rather than
// skipped. A face that declares a ligature over a joiner means it.
func (sh shaper) nextNotIgnored(buf []Glyph, from, flags, want int) int {
	end := sh.end(buf)
	for i := from; i < end; i++ {
		if sh.ignores(flags, buf[i]) {
			continue
		}
		if buf[i].GID != want && sh.stepsOverJoiner(i, false) {
			continue
		}
		return i
	}
	return end
}

// # Matching without building
//
// A rule states three sequences — what must precede, what is replaced, what
// must follow — as counts followed by items, and every one of those counts is
// a number the font wrote. The matchers here read each item from the rule's
// bytes at the moment it is compared, and stop at the first that differs.
//
// They used to collect first and compare after: each rule allocated a list the
// length of each of its three counts before any byte behind the count was
// checked, and gathered a position for every item before comparing any. A
// chained rule set of two thousand offsets to one two-byte rule declaring a
// backtrack of 65,535 allocated half a megabyte per rule per glyph — 174 ms a
// glyph, from four kilobytes of GSUB. Now a rule whose parts are not all
// present in its bytes is skipped before anything is matched, and one that is
// costs what comparing it costs.

// ruleInput is a place to put the positions a rule's input matched at: the
// longest input that can match, on the stack of the matcher that fills it.
type ruleInput [maxContextLength]int

// matchInput walks the glyphs a lookup with these flags looks at, starting at
// a position, and compares the k-th of them with match. It stops at the first
// that differs, and fills out with where each one was.
//
// An input longer than maxContextLength does not match, as in HarfBuzz.
//
// Unlike the ligature walk above this cannot tell whether a joiner in the way is
// the glyph the rule wanted: the rule's items are compared by the caller, and
// may be classes rather than glyphs. A joiner the feature allows to be stepped
// over is therefore always stepped over here, never matched. It costs a font's
// contextual rule that names a joiner explicitly *and* is declared under a
// feature that steps over joiners, which is a combination that contradicts
// itself.
func (sh shaper) matchInput(buf []Glyph, at, count, flags int, out *ruleInput,
	match func(k, pos int) bool) bool {

	if count < 1 || count > maxContextLength {
		return false
	}
	end := sh.end(buf)
	pos := at
	for k := 0; k < count; k++ {
		for {
			if pos >= end {
				return false
			}
			if !sh.ignores(flags, buf[pos]) && !sh.stepsOverJoiner(pos, false) {
				break
			}
			pos++
		}
		if !match(k, pos) {
			return false
		}
		out[k] = pos
		pos++
	}
	return true
}

// matchLookahead is matchInput for the part of a rule that says what must
// *follow* what it replaces, which steps over joiners in every case and whose
// positions nothing needs afterwards.
func (sh shaper) matchLookahead(buf []Glyph, from, count, flags int, match func(k int, g Glyph) bool) bool {
	end := sh.end(buf)
	pos := from
	for k := 0; k < count; k++ {
		for {
			if pos >= end {
				return false
			}
			if !sh.ignores(flags, buf[pos]) && !sh.stepsOverJoiner(pos, true) {
				break
			}
			pos++
		}
		if !match(k, buf[pos]) {
			return false
		}
		pos++
	}
	return true
}

// matchBacktrack compares the glyphs before a position, nearest first, which
// is the order the format stores a backtrack sequence in. It is context, so it
// steps over joiners.
//
// A position may be negative, which is a glyph the pass has already settled and
// so is behind the ones the lookup was given: a rule at the front of what is
// left still has the run before it as its context. glyphAt is what reads one.
func (sh shaper) matchBacktrack(buf []Glyph, before, count, flags int, match func(k int, g Glyph) bool) bool {
	low := sh.floor - sh.base()
	if settled := -len(sh.settledRun()); low < settled {
		low = settled
	}
	pos := before - 1
	for k := 0; k < count; k++ {
		for {
			if pos < low {
				return false
			}
			if !sh.ignores(flags, sh.glyphAt(buf, pos)) && !sh.stepsOverJoiner(pos, true) {
				break
			}
			pos--
		}
		if !match(k, sh.glyphAt(buf, pos)) {
			return false
		}
		pos--
	}
	return true
}

// sequenceContext matches a GSUB type 5 subtable and applies its lookups.
func (sh shaper) sequenceContext(sub []byte, buf []Glyph, at, flags, depth int) (int, []Glyph, bool) {
	if len(sub) < 4 {
		return 0, buf, false
	}
	switch font.Be16(sub, 0) {
	case 1:
		i, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
		if !ok {
			return 0, buf, false
		}
		return sh.contextRuleSet(sub, 6, i, buf, at, flags, depth, func(item, pos int) bool {
			return buf[pos].GID == item
		})
	case 2:
		if len(sub) < 8 {
			return 0, buf, false
		}
		if _, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID); !ok {
			return 0, buf, false
		}
		// Each glyph's class is searched for in the class table where it is
		// compared, rather than read out of a map of every glyph the table
		// names — which this built at every glyph the rule was tried on.
		classes := font.Be16(sub, 4)
		return sh.contextRuleSet(sub, 8, classAt(sub, classes, buf[at].GID), buf, at, flags, depth,
			func(item, pos int) bool {
				return classAt(sub, classes, buf[pos].GID) == item
			})
	case 3:
		glyphCount := font.Be16(sub, 2)
		recCount := font.Be16(sub, 4)
		if glyphCount < 1 || 6+2*glyphCount > len(sub) {
			return 0, buf, false
		}
		var in ruleInput
		if !sh.matchInput(buf, at, glyphCount, flags, &in, func(k, pos int) bool {
			_, covered := coverageIndex(sub, font.Be16(sub, 6+2*k), buf[pos].GID)
			return covered
		}) {
			return 0, buf, false
		}
		return sh.runRecords(sub, 6+2*glyphCount, recCount, in[:glyphCount], buf, at, depth)
	}
	return 0, buf, false
}

// contextRuleSet walks the rule sets of a format 1 or 2 sequence context, whose
// only difference is whether a rule's items are glyphs or classes.
func (sh shaper) contextRuleSet(sub []byte, setsAt, index int, buf []Glyph, at, flags, depth int,
	match func(item, pos int) bool) (int, []Glyph, bool) {

	count := font.Be16(sub, setsAt-2)
	if index < 0 || index >= count || setsAt+2*index+2 > len(sub) {
		return 0, buf, false
	}
	off := font.Be16(sub, setsAt+2*index)
	if off <= 0 || off+2 > len(sub) {
		return 0, buf, false
	}
	set := sub[off:]
	var in ruleInput
	for r := 0; r < font.Be16(set, 0); r++ {
		if 2+2*r+2 > len(set) {
			break
		}
		ro := font.Be16(set, 2+2*r)
		if ro <= 0 || ro+4 > len(set) {
			continue
		}
		rule := set[ro:]
		glyphCount := font.Be16(rule, 0)
		recCount := font.Be16(rule, 2)
		if glyphCount < 1 || 4+2*(glyphCount-1) > len(rule) {
			continue
		}
		// The first item is the glyph the rule set was chosen by, which the
		// rule does not state again.
		if !sh.matchInput(buf, at, glyphCount, flags, &in, func(k, pos int) bool {
			return k == 0 || match(font.Be16(rule, 4+2*(k-1)), pos)
		}) {
			continue
		}
		return sh.runRecords(rule, 4+2*(glyphCount-1), recCount, in[:glyphCount], buf, at, depth)
	}
	return 0, buf, false
}

// chainedContext matches a GSUB type 6 subtable, which also constrains what
// comes before and after the part being replaced.
func (sh shaper) chainedContext(sub []byte, buf []Glyph, at, flags, depth int) (int, []Glyph, bool) {
	if len(sub) < 4 {
		return 0, buf, false
	}
	switch font.Be16(sub, 0) {
	case 1, 2:
		setsAt := 6
		byClass := font.Be16(sub, 0) == 2
		if byClass && len(sub) < 12 {
			return 0, buf, false
		}
		// A rule set is chosen by coverage index in the glyph form and by input
		// class in the class form; coverage still gates whether any rule
		// applies, and it is asked first, so that a glyph no rule starts with
		// costs one search and nothing else.
		index, ok := coverageIndex(sub, font.Be16(sub, 2), buf[at].GID)
		if !ok {
			return 0, buf, false
		}
		var classes chainClasses
		if byClass {
			classes = chainClasses{
				back:  font.Be16(sub, 4),
				input: font.Be16(sub, 6),
				ahead: font.Be16(sub, 8),
			}
			setsAt = 12
			index = classAt(sub, classes.input, buf[at].GID)
		}
		return sh.chainedRuleSet(sub, setsAt, index, buf, at, flags, depth, byClass, classes)
	case 3:
		return sh.chainedFormat3(sub, buf, at, flags, depth)
	}
	return 0, buf, false
}

// chainClasses is where a class-form chained subtable's three class tables
// sit: one each for what precedes, what is matched and what follows, since a
// glyph may belong to a different group in each role.
type chainClasses struct{ back, input, ahead int }

// chainedRuleSet walks the rule sets of a chained context in glyph or class
// form. A rule states three sequences — what must precede, what is matched, and
// what must follow — and the first is stored nearest-first.
func (sh shaper) chainedRuleSet(sub []byte, setsAt, index int, buf []Glyph, at, flags, depth int,
	byClass bool, classes chainClasses) (int, []Glyph, bool) {

	count := font.Be16(sub, setsAt-2)
	if index < 0 || index >= count || setsAt+2*index+2 > len(sub) {
		return 0, buf, false
	}
	off := font.Be16(sub, setsAt+2*index)
	if off <= 0 || off+2 > len(sub) {
		return 0, buf, false
	}
	set := sub[off:]
	// What a glyph is to a rule: itself in the glyph form, its class in the
	// class table for its role in the class form.
	item := func(classOff int, g Glyph) int {
		if byClass {
			return classAt(sub, classOff, g.GID)
		}
		return g.GID
	}

	var in ruleInput
	for r := 0; r < font.Be16(set, 0); r++ {
		if 2+2*r+2 > len(set) {
			break
		}
		ro := font.Be16(set, 2+2*r)
		if ro <= 0 || ro+2 > len(set) {
			continue
		}
		rule := set[ro:]
		// Where each part of the rule is, and whether all of it is there: a
		// count with too few bytes behind it is a rule that is not read, and
		// it is settled here, before anything is matched.
		backCount := font.Be16(rule, 0)
		backAt := 2
		p := backAt + 2*backCount
		if p+2 > len(rule) {
			continue
		}
		inputCount := font.Be16(rule, p)
		inputAt := p + 2
		if inputCount < 1 {
			continue
		}
		p = inputAt + 2*(inputCount-1)
		if p+2 > len(rule) {
			continue
		}
		aheadCount := font.Be16(rule, p)
		aheadAt := p + 2
		p = aheadAt + 2*aheadCount
		if p+2 > len(rule) {
			continue
		}
		recCount := font.Be16(rule, p)

		if !sh.matchInput(buf, at, inputCount, flags, &in, func(k, pos int) bool {
			return k == 0 || item(classes.input, buf[pos]) == font.Be16(rule, inputAt+2*(k-1))
		}) {
			continue
		}
		if backCount > 0 && !sh.matchBacktrack(buf, at, backCount, flags, func(k int, g Glyph) bool {
			return item(classes.back, g) == font.Be16(rule, backAt+2*k)
		}) {
			continue
		}
		if aheadCount > 0 && !sh.matchLookahead(buf, in[inputCount-1]+1, aheadCount, flags, func(k int, g Glyph) bool {
			return item(classes.ahead, g) == font.Be16(rule, aheadAt+2*k)
		}) {
			continue
		}
		return sh.runRecords(rule, p+2, recCount, in[:inputCount], buf, at, depth)
	}
	return 0, buf, false
}

// chainedFormat3 is the coverage-based chained context, the form a modern font
// uses most: three lists of coverage tables rather than rule sets.
func (sh shaper) chainedFormat3(sub []byte, buf []Glyph, at, flags, depth int) (int, []Glyph, bool) {
	// Where each list is, and whether all of it is there, before anything is
	// matched; the coverage offsets are read where each is used.
	backCount := font.Be16(sub, 2)
	backAt := 4
	p := backAt + 2*backCount
	if p+2 > len(sub) {
		return 0, buf, false
	}
	inputCount := font.Be16(sub, p)
	inputAt := p + 2
	p = inputAt + 2*inputCount
	if inputCount < 1 || p+2 > len(sub) {
		return 0, buf, false
	}
	aheadCount := font.Be16(sub, p)
	aheadAt := p + 2
	p = aheadAt + 2*aheadCount
	if p+2 > len(sub) {
		return 0, buf, false
	}
	recCount := font.Be16(sub, p)
	p += 2

	covers := func(listAt, k, gid int) bool {
		_, ok := coverageIndex(sub, font.Be16(sub, listAt+2*k), gid)
		return ok
	}
	var in ruleInput
	if !sh.matchInput(buf, at, inputCount, flags, &in, func(k, pos int) bool {
		return covers(inputAt, k, buf[pos].GID)
	}) {
		return 0, buf, false
	}
	if backCount > 0 && !sh.matchBacktrack(buf, at, backCount, flags, func(k int, g Glyph) bool {
		return covers(backAt, k, g.GID)
	}) {
		return 0, buf, false
	}
	if aheadCount > 0 && !sh.matchLookahead(buf, in[inputCount-1]+1, aheadCount, flags, func(k int, g Glyph) bool {
		return covers(aheadAt, k, g.GID)
	}) {
		return 0, buf, false
	}
	return sh.runRecords(sub, p, recCount, in[:inputCount], buf, at, depth)
}

// maxContextLength bounds how many glyphs a contextual rule's input may match,
// and how many positions its records may come to address once the lookups they
// name have lengthened the run.
//
// It is HarfBuzz's HB_MAX_CONTEXT_LENGTH, with the same number, and for the same
// two reasons. A rule's input is a count the font states, so without a bound a
// few bytes of rule ask for a position list as long as the run; and a record
// that decomposes a glyph inserts positions, so a rule whose records each
// decompose again would grow the list without end. A rule longer than this
// does not match in HarfBuzz either, and no real font writes one: the longest
// inputs in the corpus fonts are a handful of glyphs.
const maxContextLength = 64

// runRecords applies the lookups a matched rule names, each at the position it
// names, and reports where the walk resumes: how many glyphs from the rule's
// start, at, it accounted for.
//
// The positions are those of the *matched* glyphs, so a record naming index two
// means the third thing the rule matched — not the third glyph in the buffer,
// which may differ when the lookup skips marks.
//
// # Keeping the positions in step
//
// A rule may name several lookups, and one of them may change the buffer's
// length: a ligature makes it shorter, a decomposition longer, a deletion takes
// a glyph out. A later record's index then has to mean something in the buffer
// as it now is, and this follows HarfBuzz's apply_lookup exactly, because the
// OpenType text leaves the question open and HarfBuzz's answer is what fonts
// are tested against:
//
//   - A record whose lookup lengthened the buffer by n is taken to have put n
//     new glyphs straight after its own position. They become matched positions
//     of their own, and every index after it moves up by n — so a later record
//     naming index one after a decomposition at index zero reaches the second
//     piece of the decomposition, not the glyph that was matched second.
//   - One that shortened it by n is taken to have consumed the n matched
//     positions after its own. They are dropped, the rest move down, and a
//     later record naming one that no longer exists does nothing. This is
//     HarfBuzz's own simplification — a deletion takes out the glyph at the
//     position rather than the ones after it — and it is kept, since matching
//     it is the point.
//
// The first version kept the list its original length and moved every position
// past the change by the change. A ligature of three at index zero then moved
// index one to minus one, and the next record applied a lookup at position -1:
// an index out of range, from one font, in the middle of Compose.
//
// # Where the walk resumes
//
// After the last glyph the rule matched, as it stands once the records have
// run — not after as many glyphs as it matched. The two differ wherever the
// lookup steps over glyphs: a rule matching c, c across an accent it ignores
// spans three glyphs, and resuming two in lands on its own second c, which it
// then matches again as the start of a new rule. HarfBuzz moves past the whole
// span, and so does this.
//
// A rule whose records took out everything from its start on has consumed
// nothing that is still there, and reports zero with a shorter buffer, which is
// how a deletion tells the walk to look at what moved into its place.
func (sh shaper) runRecords(base []byte, recAt, count int, positions []int, buf []Glyph, at, depth int) (int, []Glyph, bool) {
	if len(positions) == 0 {
		return 0, buf, false
	}
	// One past the last matched glyph, which the records move as they change
	// the buffer's length.
	end := positions[len(positions)-1] + 1
	startLen := len(buf)
	for i := 0; i < count; i++ {
		rec := recAt + 4*i
		if rec+4 > len(base) {
			break
		}
		seqIndex := font.Be16(base, rec)
		lookupIndex := font.Be16(base, rec+2)
		if seqIndex < 0 || seqIndex >= len(positions) {
			continue
		}
		target := positions[seqIndex]
		// A position the records before this one took out from under it. The
		// bookkeeping below keeps every position inside the buffer, so this is a
		// guard on that and not a case the arithmetic produces.
		if target < 0 || target >= len(buf) {
			continue
		}
		if !sh.recurse() {
			// The run has spent its allowance. Every remaining record does
			// nothing, which is what a rule that did not match does.
			break
		}
		if sh.positioning {
			// Positioning never changes how many glyphs there are, so there is
			// nothing to keep in step.
			sh.applyGPOSAt(lookupIndex, buf, target, depth+1)
			continue
		}
		before := len(buf)
		_, out := sh.applyGSUBAt(lookupIndex, buf, target, depth+1)
		buf = out

		delta := len(buf) - before
		if delta == 0 {
			continue
		}
		// The end moves with the change, but never back past the position the
		// lookup was applied at: nothing a lookup does at a position can reach
		// behind it.
		end += delta
		if end < target {
			delta += target - end
			end = target
		}
		next := seqIndex + 1
		if delta > 0 {
			if len(positions)+delta > maxContextLength {
				break
			}
			grown := make([]int, len(positions)+delta)
			copy(grown, positions[:next])
			for k := 0; k < delta; k++ {
				grown[next+k] = target + 1 + k
			}
			for k, p := range positions[next:] {
				grown[next+delta+k] = p + delta
			}
			positions = grown
			continue
		}
		// Shrinking: the positions after this one that the change consumed are
		// dropped, as many as it shortened the buffer by and no more than there
		// are, and the rest move down by as much.
		drop := min(-delta, len(positions)-next)
		kept := positions[next+drop:]
		for k, p := range kept {
			positions[next+k] = p + delta
		}
		positions = positions[:next+len(kept)]
	}
	consumed := end - at
	if consumed < 1 {
		// Nothing the rule matched is left from its start on. If the buffer is
		// shorter the walk looks again at what moved into the place; if it is
		// not, the walk moves on by one so that it always moves.
		if len(buf) < startLen {
			return 0, buf, true
		}
		consumed = 1
	}
	return consumed, buf, true
}

// coverageIndex reports a glyph's index within a coverage table, which is what
// the tables using one are indexed by.
func coverageIndex(base []byte, off, gid int) (int, bool) {
	if off <= 0 || off+4 > len(base) {
		return 0, false
	}
	c := base[off:]
	switch font.Be16(c, 0) {
	case 1:
		n := font.Be16(c, 2)
		lo, hi := 0, n-1
		for lo <= hi {
			mid := (lo + hi) / 2
			if 4+2*mid+2 > len(c) {
				return 0, false
			}
			g := font.Be16(c, 4+2*mid)
			switch {
			case g == gid:
				return mid, true
			case g < gid:
				lo = mid + 1
			default:
				hi = mid - 1
			}
		}
	case 2:
		// A binary search over the ranges, as HarfBuzz does it and for the
		// reason format 1 is searched: this is asked for every glyph a rule is
		// tried on, and a coverage of a thousand ranges was read front to back
		// each time. The comparisons are HarfBuzz's, so a malformed table whose
		// ranges are out of order answers the same way in both.
		n := min(font.Be16(c, 2), (len(c)-4)/6)
		lo, hi := 0, n-1
		for lo <= hi {
			mid := int(uint(lo+hi) >> 1)
			rec := 4 + 6*mid
			start, end := font.Be16(c, rec), font.Be16(c, rec+2)
			switch {
			case gid < start:
				hi = mid - 1
			case gid > end:
				lo = mid + 1
			default:
				return font.Be16(c, rec+4) + (gid - start), true
			}
		}
	}
	return 0, false
}

// componentsOf is how many parts of a ligature a glyph counts as: one for an
// ordinary glyph, and its own count for a ligature being joined again.
func componentsOf(g Glyph) int {
	if g.lig.comps > 1 {
		return g.lig.comps
	}
	return 1
}
