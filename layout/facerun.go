package layout

import (
	"maps"
	"slices"
	"strings"

	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/shape"
)

// Falling back from one face to the next *within* a run of text.
//
// faceForText answers per box: it asks whether the family's face can set the
// whole of the box's text and, if not, whether some other face can set the whole
// of it instead. That is the right answer for a box written in one script and
// the wrong one for the case this file exists for — a box written in one script
// with a word or a letter of another in it, which is what almost every document
// with a citation, a name or a technical term in it is.
//
// It is the wrong answer twice over. No single face covers "the following two
// lines (Force bidi: א)": the pan-Unicode face this engine falls back to has
// every character but the alef, and the Hebrew face has the alef and no Latin at
// all. So the whole-box question has no answer, the box keeps the family's face,
// and the alef is set as a space and reported missing. Twenty-five of the
// reftest suite's documents are exactly that sentence.
//
// # What is not done here
//
// The unit of choice is a grapheme cluster and not a character, because a
// combining mark positioned by a font that never saw its base lands at the
// origin. It is not a *word*: a run of Latin inside a Hebrew sentence is chosen
// per cluster like everything else, and the clusters merge into one run because
// they chose the same face.
//
// Shaping still happens per run, so a ligature or a kern that would have crossed
// a face boundary is lost. Nothing can be done about that — the two glyphs are
// in different fonts — and it is why the split is made as coarsely as possible:
// maximal stretches, and a single run whenever one face can set everything.

// faceRun is a stretch of text and the face it is set in.
type faceRun struct {
	Text string
	Face *shape.Face
	// substituted marks a run whose face came from the fallback set rather than
	// from a family the document named.
	//
	// The difference matters to one caller and matters a lot: a run set in a
	// face the author asked for is the page working, and a run set in a face
	// nobody chose is worth reporting. A document naming two webfonts with
	// disjoint unicode-ranges uses its *second* family for half its text, which
	// looks like a substitution from the outside and is the opposite of one.
	substituted bool
}

// faceRunsFor splits text into the stretches each face can set.
//
// The primary face is preferred everywhere it can be used, so a document that
// does not mix scripts gets exactly one run holding exactly the text it started
// with — which is what makes this change invisible to every document that was
// already right.
func (l *layouter) faceRunsFor(b *Box, primary *shape.Face, text string) []faceRun {
	one := []faceRun{{Text: text, Face: primary}}
	if text == "" || primary == nil {
		return one
	}
	// A missing fallback set is not a reason to stop: the control-character cut
	// below does not need one, and a caller with no fallback faces still gets a
	// visible glyph for a character no face has.
	set, canFall := l.fontSet.(FallbackFontSet)
	ranged, hasRanges := l.fontSet.(RangedFontSet)
	if hasRanges {
		// Only worth walking the family list per cluster when some face in it
		// is actually restricted. A document with no unicode-range anywhere —
		// which is almost every document — asks the same question for every
		// cluster and gets the same answer, so it is not asked at all.
		hasRanges = l.familyListIsRestricted(b)
	}

	// Nothing to do: the primary face can set everything, no character has to be
	// drawn as a box, and no family in the list is restricted to part of Unicode.
	//
	// The last of those three is not the same question as the first, and putting
	// it here was where this went wrong once. A unicode-range says which
	// characters a face is *for*, not which it has glyphs for — a Latin webfont
	// scoped to U+0-7F usually has a full Latin-1 repertoire — so missesVisible
	// answers "no, it is fine" for exactly the text the descriptor exists to
	// move, and the run was returned whole before the walk below ever ran.
	if !missesVisible(primary, text) && !hasVisibleControl(text) && !hasRanges {
		return one
	}
	if !canFall && !hasRanges && !hasVisibleControl(text) {
		return one
	}
	bold := isBold(b.Style["font-weight"])
	italic := isItalic(b.Style["font-style"])

	// The cluster starts, so every cluster is [at[i], at[i+1]).
	//
	// segment.Boundaries gives the offsets *inside* the string, deliberately
	// leaving out both ends because neither is a place a caller may cut. Both
	// ends are exactly what is wanted here, where the question is not where to
	// cut but which cluster each byte belongs to — and leaving the leading zero
	// off skips the first cluster of every run, which is a whole word when the
	// run is one word long.
	at := append([]int{0}, append(segment.Boundaries(nil, text), len(text))...)

	var runs []faceRun
	// The stretch being accumulated, the face it is going to, and whether that
	// face came from outside the document's own list.
	start, cur := 0, primary
	curSub := false
	flush := func(end int) {
		if end > start {
			runs = append(runs, faceRun{Text: text[start:end], Face: cur, substituted: curSub})
		}
		start = end
	}
	for i := 0; i+1 < len(at); i++ {
		lo, hi := at[i], at[i+1]
		cluster := text[lo:hi]
		want := primary
		if _, isControl := controlOf(cluster); isControl {
			// Its own run, always: it is neither the face before it nor the
			// face after it, and the painter recognises it by being alone.
			flush(lo)
			runs = append(runs, faceRun{Text: cluster, Face: primary})
			start, cur, curSub = hi, primary, false
			continue
		}
		if drawsNoPaper(cluster) {
			// A character that sets no paper stays in the run being gathered.
			//
			// Which face it is "in" decides nothing a reader can see and
			// everything about whether the run is cut in two. A soft hyphen
			// inside an Arabic word is the case: the standard faces encode
			// U+00AD through WinAnsi, so the primary face was held to have it
			// while the letters around it moved to a face that has *them* — and
			// the word came out as three runs, the letters lost the context
			// that gives them their joined forms, and the primary face's
			// quarter-em advance for a character that should take no room
			// opened a gap in the middle of the word.
			continue
		}
		fromFallback := false
		if hasRanges {
			// The document's own list, one cluster at a time. A family whose
			// faces all exclude this character has nothing for it and the next
			// one the author named is asked — which is what a unicode-range is
			// written to make happen.
			if named, found := l.namedFaceFor(ranged, b, cluster); found {
				want = named
			}
		}
		if canFall && missesVisible(want, cluster) {
			// This cluster is not one the primary face can set. Ask for a face
			// that can — for the cluster alone, because asking for the rest of
			// the text would be the whole-box question again and would have the
			// same non-answer.
			if alt, found := set.FaceFor(cluster, bold, italic); found {
				want, fromFallback = alt, true
			}
			// Not found: the cluster stays with the primary face and is
			// reported missing by checkGlyphs, which is what happened before
			// this file existed and is still the right answer — there is no
			// face to move it to.
		}
		if want != cur || fromFallback != curSub {
			flush(lo)
			cur, curSub = want, fromFallback
		}
	}
	flush(len(text))
	if len(runs) == 0 {
		return one
	}
	if len(runs) == 1 && runs[0].Face == primary {
		return one
	}
	return runs
}

// The font-fallback finding: a family that set *none* of a paragraph's text.
//
// That is a different thing from a fallback, and only one of the two is worth a
// caller's attention. A word of Hebrew in an English sentence is set in a Hebrew
// face because that is what fallback *is* — every renderer does it, the page is
// right, and the English around it keeps the metrics the author asked for. Text
// the family contributed nothing to is text set in a font nobody chose, and for
// a caller who has to embed one that is the thing to know: the family they named
// cannot do this job.
//
// The distinction only became available when the fallback started working per
// run. Before, the whole box moved whenever one character was missing, so the
// two cases were the same event and the question — can one face set the whole of
// this text — could not tell them apart.
//
// It is also what makes a broad fallback face safe to have. A face covering most
// of Unicode can set the whole of almost any text, so the old question found an
// answer almost every time it was asked: adding GNU Unifont as a last resort
// reported a substitution on eighty-eight documents that had nothing wrong with
// them. Asking which characters actually moved reports none of those.

// noteSubstitution records what a family did with one box's text, for the
// finding below.
//
// It is a note and not a report because the question is about a *paragraph* and
// this is called with a box: "high<span lang=he>א</span>way" is one line of
// English with one Hebrew letter in it, and the span is a box whose family set
// none of its text. Reporting there said the family could not do the job over a
// line it does almost all of — and the suite writes exactly that shape, six
// times over, as a one-character <span> holding a currency sign inside a
// Japanese paragraph.
//
// So the answer is gathered per inline formatting context and given at the end
// of one. See flushSubstitutions.
func (l *layouter) noteSubstitution(b *Box, primary *shape.Face, runs []faceRun) {
	if len(runs) == 0 || primary == nil {
		return
	}
	// The document named no particular face, only a kind. Choosing one that can
	// set the text is what a generic family *is* — see namesOnlyGenericFamilies.
	families := b.Style["font-family"]
	if namesOnlyGenericFamilies(families) {
		return
	}
	if l.substituted == nil {
		l.substituted = map[string]*substitution{}
	}
	got := l.substituted[families]
	if got == nil {
		got = &substitution{}
		l.substituted[families] = got
	}
	for _, r := range runs {
		if r.Face == primary {
			// The family set something here, which is the whole of what the
			// finding asks. One run anywhere in the paragraph is enough.
			got.set = true
			continue
		}
		if r.substituted && got.alt == nil && r.Face != primary {
			got.alt, got.at = r.Face, b
		}
	}
}

// substitution is what one family did with the text of one inline formatting
// context: whether it set any of it, and what set the rest.
type substitution struct {
	set bool
	alt *shape.Face
	at  *Box
}

// gatherSubstitutions starts a paragraph's tally and returns the call that ends
// it, which reports and puts back whatever was being gathered around it.
//
// Around it, because an inline formatting context can be laid out inside
// another: an inline-block is laid out where it sits on the line, so its own
// paragraphs are finished while the paragraph holding it is half gathered.
// Flushing there would answer the outer paragraph's question from the first half
// of it, and the family it had not reached yet is exactly the one that would be
// reported.
func (l *layouter) gatherSubstitutions() func() {
	outer := l.substituted
	l.substituted = nil
	return func() {
		l.flushSubstitutions()
		l.substituted = outer
	}
}

// flushSubstitutions reports the families that set nothing in the paragraph just
// finished, and forgets what it gathered.
//
// Once per family per document, because the finding is about the family and a
// page that names one in a hundred paragraphs has one gap and not a hundred.
func (l *layouter) flushSubstitutions() {
	// In name order, because two families both going unset in one paragraph
	// would otherwise be reported in whichever order the map happened to be
	// walked in, and a finding list a document reproduces is worth more than
	// the few nanoseconds the sort costs.
	lists := slices.Sorted(maps.Keys(l.substituted))
	for _, families := range lists {
		got := l.substituted[families]
		delete(l.substituted, families)
		if got.set || got.alt == nil {
			// Every run went to a face the document itself named — a second
			// family in the font-family list, reached because the first
			// declared a unicode-range that excluded this text. That is the
			// list working, not a substitution, and saying "no face for these
			// families could set any of this" of a face one of those families
			// provided would be untrue as well as alarming.
			continue
		}
		if l.reportedFamilies == nil {
			l.reportedFamilies = map[string]bool{}
		}
		if l.reportedFamilies[families] {
			continue
		}
		l.reportedFamilies[families] = true
		l.rec.ReportDetail(Finding{
			Rule: RuleFontSubstituted,
			Message: "no face for " + quoteValue(families) +
				" has a glyph for any of this text, so " + quoteValue(got.alt.Name()) +
				" set it; the metrics and the line breaks are that face's",
			Path:     PathOf(boxElement(got.at)),
			Property: "font-family",
		})
	}
}

// familyListIsRestricted reports whether any face the box's font-family list
// could resolve to carries a unicode-range.
//
// It is the guard that keeps the per-cluster walk off every other document. The
// answer depends only on the family list, so it is memoized per list rather than
// per box — a page of ten thousand paragraphs in one family asks once.
func (l *layouter) familyListIsRestricted(b *Box) bool {
	set, ok := l.fontSet.(*documentFonts)
	if !ok {
		if f, isFallback := l.fontSet.(fallbackDocumentFonts); isFallback {
			set = f.documentFonts
		} else {
			return false
		}
	}
	families := b.Style["font-family"]
	if got, cached := l.restrictedFamilies[families]; cached {
		return got
	}
	restricted := false
	for _, family := range parseFamilyList(families) {
		key := strings.TrimSpace(strings.Trim(strings.TrimSpace(strings.ToLower(family)), `"'`))
		for _, c := range set.byFamily[key] {
			if len(c.rule.ranges) > 0 {
				restricted = true
			}
		}
	}
	if l.restrictedFamilies == nil {
		l.restrictedFamilies = map[string]bool{}
	}
	l.restrictedFamilies[families] = restricted
	return restricted
}

// namedFaceFor walks the box's font-family list for a face the document named
// that may set this cluster.
//
// It stops at the first family that offers one, which is the cascade's own rule
// for a font-family list and is what makes "high-a-only, deep-b-only" mean what
// it says. A cluster no named family covers comes back false and is left to the
// primary face and the fallback set, exactly as before.
func (l *layouter) namedFaceFor(ranged RangedFontSet, b *Box, cluster string) (*shape.Face, bool) {
	bold := isBold(b.Style["font-weight"])
	italic := isItalic(b.Style["font-style"])
	for _, family := range parseFamilyList(b.Style["font-family"]) {
		if face, ok := ranged.FaceForFamily(family, cluster, bold, italic); ok {
			return face, true
		}
	}
	return nil, false
}

// namesOnlyGenericFamilies reports whether a font-family list asks for a kind of
// face rather than for any face in particular.
//
// CSS Fonts §5.1: a generic family is a keyword the user agent maps to a family
// of its choosing, and the choice may depend on the script. So a document that
// says "serif" and gets a face that can set its text has been given what it
// asked for — the mapping *is* the answer — and reporting a substitution says
// something went wrong when nothing did.
//
// Twenty-eight of the CSS Working Group's reftests were held back by exactly
// that finding, and they are the strongest case for it there is: they name no
// font at all. They set "content: counter(test, georgian)", inherit the initial
// font-family, and are told that no face for "serif" could set text the document
// itself generated. A browser picks a Georgian face without comment.
//
// A list naming even one real family is a different matter and keeps its
// finding. An author who wrote "Kartuli, serif" asked for Kartuli, and a page set
// in something else is a page they would want to know about — the generic behind
// it is a fallback they wrote, not the whole of their request.
func namesOnlyGenericFamilies(list string) bool {
	names := parseFamilyList(list)
	if len(names) == 0 {
		// Nothing stated at all, which is the initial value: a generic by
		// another name.
		return true
	}
	for _, name := range names {
		if !genericFamilies[strings.ToLower(strings.TrimSpace(name))] {
			return false
		}
	}
	return true
}

// genericFamilies are CSS Fonts §5.1's keywords: the ones that name a kind of
// face rather than a face.
//
// It is deliberately not "everything in standardFamilies". That map answers a
// different question — which of the fourteen standard faces to use for a name —
// and it holds "Arial" and "Georgia", which are families a document really did
// ask for by name. Reading it as the generic list would treat a document that
// asked for Georgia as one that asked for nothing in particular.
var genericFamilies = map[string]bool{
	"serif": true, "sans-serif": true, "monospace": true,
	"cursive": true, "fantasy": true, "system-ui": true,
	"ui-serif": true, "ui-sans-serif": true, "ui-monospace": true,
	"ui-rounded": true, "math": true, "emoji": true, "fangsong": true,
}

// drawsNoPaper reports whether a cluster is characters that set no paper: the
// joiners, the bidi controls, the soft hyphen and the zero widths.
//
// An empty cluster answers yes and cannot happen: the boundaries the caller
// walks are strictly increasing, and a run with no text returned before any of
// this. Guarding it was written and taken out again — the guard could not be
// made to fail, and a condition nothing can reach is a condition a reader has
// to work out is unreachable.
func drawsNoPaper(cluster string) bool {
	for _, r := range cluster {
		if !isDefaultIgnorable(r) {
			return false
		}
	}
	return true
}
