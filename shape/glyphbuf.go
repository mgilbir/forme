package shape

import (
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/bidi"
)

// The shaped-glyph model.
//
// Shape returns spans, which can say only one thing about a glyph: move the pen
// horizontally before drawing it. That is all kerning needs and all a
// left-to-right run of unmarked Latin needs, and it is not enough for anything
// else. An accent has to sit *over* the letter it belongs to — up and across by
// an amount the font states — and a span cannot say so.
//
// So positioning produces glyphs, not spans: a glyph index, where it goes
// relative to the pen, and how far the pen then moves. Shape is written over
// this, taking the horizontal part and discarding the rest, which is why it is
// still the right call for text that carries no marks.

// Glyph is one positioned glyph of a shaped run. Distances are in thousandths
// of an em, the unit the font's own metrics are in, so they are independent of
// the size the text is finally set at.
type Glyph struct {
	// GID is the glyph to draw.
	GID int

	// Cluster is the byte offset, in the input string, of the first character
	// this glyph came from. Several glyphs may share a cluster — a letter and
	// its accent — and one glyph may stand for several characters, as a
	// ligature does. It is what maps a position in the text to a position on
	// the page, for selection, search and hit-testing.
	Cluster int

	// XAdvance is how far the pen moves after this glyph is drawn. It starts as
	// the font's own advance and is what kerning changes. A mark's is zero,
	// which is what makes it sit on the glyph before it rather than after.
	XAdvance float64

	// XOffset and YOffset displace the glyph from the pen without moving the
	// pen. This is how a mark is placed over its base.
	XOffset, YOffset float64

	// lig records this glyph's part in a ligature, and is unexported because it
	// is bookkeeping between the substitution pass and the positioning one
	// rather than anything a caller can use.
	lig ligatureRef

	// class is what the character this glyph came from says it is — a mark or a
	// letter — in GDEF's own numbering. It is what a lookup flag is read
	// against when the font classifies nothing itself; see layout.classOf.
	class int

	// mask is which of the masked features this glyph is for: the positional
	// form its letter takes in a cursive script, the part of an Indic syllable
	// it is, the fraction it stands in. See glyphMask, and plan.go for how a
	// lookup reads it.
	//
	// It is carried on the glyph rather than worked out when it is needed
	// because by then it cannot be: the substitutions that come first change how
	// many glyphs there are, so nothing maps back to the characters a form was
	// decided from. A glyph a substitution makes takes the mask of the glyph it
	// was made from. See arabic.go.
	mask glyphMask

	// substituted says a substitution has touched the glyph: replaced it,
	// made it from several, or taken it apart. It is HarfBuzz's SUBSTITUTED
	// glyph property, and what it decides is whether a character nothing is
	// drawn for is still one once the font has had its say — see
	// dropUnsubstitutedIgnorables.
	substituted bool
}

// ligatureRef says what a glyph has to do with a ligature.
//
// It exists because a mark inside a ligature has to be placed against the part
// of it the mark belongs to. A dot written under the first f of "ffi" and a dot
// written under the second are the same glyph attaching to the same glyph, and
// the font gives them different anchors; the only thing that tells them apart is
// which part of the text each came from, and that is what forming the ligature
// is the last moment to record.
type ligatureRef struct {
	// id is shared by a ligature glyph and every mark that was inside it. Zero
	// means the glyph has nothing to do with any ligature — which is also what
	// a "ligature" of a base and its own marks gets, since that is not a
	// ligature in the sense this is about.
	id int
	// comp is which part of the ligature this mark belongs to, counting from
	// one. It is zero on the ligature glyph itself.
	comp int
	// comps is how many parts this glyph counts as when it becomes part of a
	// larger ligature: one for an ordinary glyph, and its own component count
	// for a ligature that is then joined again.
	comps int
}

// ShapeGlyphs turns a string into positioned glyphs, applying everything this
// package reads: ligatures, contextual substitution, kerning, mark attachment,
// and the direction each part of the text runs in.
//
// The glyphs come back in *visual* order — the order the pen draws them, left to
// right — so a caller can draw them as they are, at a pen that only moves
// forward, whatever scripts the string mixes. That is not the order the string
// is written in: Hebrew and Arabic read the other way, and a PDF text-showing
// operator has no way to say so. bidi.go decides where each stretch belongs.
//
// It is the full result. Shape is the same pipeline with the vertical part
// dropped, and is enough whenever the text carries no marks.
func (f *Face) ShapeGlyphs(s string) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil, shapeContext{})
}

// ShapeGlyphsInContext is ShapeGlyphs with the text either side of the run.
//
// A cursive script chooses each letter's shape from its neighbours, and a run is
// not always the whole word: CSS Text §8.1 says the boundary between two inline
// elements does not break shaping, so "\u0639<span>\u0639</span>\u0639" is one
// joined word set as three runs. Without the context each run is shaped alone
// and every letter comes out in its isolated form, which for a reader of Arabic
// is the difference between a word and three letters standing apart.
//
// The context decides the *forms*, and nothing else. A ligature that spans the
// boundary is not formed — a lam-alef written with the lam in one run and the
// alef in another stays two letters — because the glyph for it would belong to
// both runs at once and neither could carry it. That is a real limitation and
// the suite has a test of it, shaping_lig-000.
//
// Either side may be empty, which is what the start and end of a paragraph are.
func (f *Face) ShapeGlyphsInContext(s, before, after string, off Features) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil,
		shapeContext{before: before, after: after, kerns: true, features: off})
}

// ShapeGlyphsAcrossFaces is ShapeGlyphsInContext for a neighbour that is set in
// a *different* face.
//
// Which of its four shapes a letter takes is decided by the characters beside
// it, and a character is the same character whichever font sets it: Unicode's
// joining enforcement, and the suite's shaping-join-002 and
// shaping-tatweel-002 and -003, where a zero width joiner or a tatweel is
// pulled from another font by unicode-range and the Arabic letters either side
// must still take their joined forms.
//
// A kerning pair is not. It is stated by one font over two of its own glyphs,
// and a font change is a change in formatting: the pair across such a boundary
// is not this font's to apply. So the context reaches the joining scan and not
// the boundary kern.
func (f *Face) ShapeGlyphsAcrossFaces(s, before, after string, off Features) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, nil,
		shapeContext{before: before, after: after, features: off})
}

// ShapeGlyphsMerged is ShapeGlyphsInContext where a neighbour may contribute
// glyphs to this run and not only forms.
//
// §8.1's boundary "does not break shaping", and a ligature is shaping: "of<span
// >f</span>ice" is one word and the face's ffi is what a reader of it expects.
// The two named sides are shaped together with this run and the glyphs divided
// afterwards, by the cluster each came from — so the ligature belongs to
// whichever run holds its first character, and the other draws nothing for the
// characters it swallowed and takes none of its width.
//
// The sides are named separately from the context because the two questions have
// different answers. A form crosses a change of colour and a raised baseline —
// the suite's shaping-023 sets the middle Mongolian letter blue and asks for the
// three to join — and a *glyph* cannot: one glyph is drawn once, in one colour,
// on one baseline, so a ligature across such a boundary would paint half a word
// in the wrong colour. kerns is the same distinction drawn a third time and is
// left where it was.
func (f *Face) ShapeGlyphsMerged(s, before, after, mergeBefore, mergeAfter string,
	kerns bool, off Features) ([]Glyph, int) {

	return f.shapeGlyphsWith(s, nil, shapeContext{
		before: before, after: after,
		mergeBefore: mergeBefore, mergeAfter: mergeAfter,
		kerns: kerns, features: off,
	})
}

// shapeContext is the text either side of the run being shaped, in logical
// order: before is what precedes it and after is what follows.
//
// kerns says the neighbours are set in this run's own face, so a pair that
// spans the boundary is this font's pair. See ShapeGlyphsAcrossFaces for the
// case where they are not.
type shapeContext struct {
	before, after string
	// mergeBefore and mergeAfter are the text either side that may contribute
	// *glyphs* and not only forms: the run and they are shaped as one string
	// and the glyphs divided afterwards, so a ligature that spans the boundary
	// is formed. Each is the whole of its side of the group rather than the
	// neighbour alone — every run of a group has to shape the same string, or
	// two of them disagree about where a ligature begins and a character
	// belongs to neither. See shapeMerged.
	mergeBefore, mergeAfter string
	kerns                   bool
	// cutBefore and cutAfter say the text on that side is in another script:
	// the run is a piece of a string scriptRuns cut, and the side is where it
	// was cut. The text there is still context for the forms a letter takes —
	// the characters are beside each other whatever they are written in — but
	// a script change ends a run of the font's rules, so no pair is kerned
	// across it, as none is across the runs Stack.ShapeRuns cuts at the same
	// place. It is also what keeps the cut cheap: a boundary pair is found by
	// shaping the neighbour, and a Japanese sentence changes script every few
	// characters.
	cutBefore, cutAfter bool
	// features is what the document turned off. See Features, and note that it
	// travels with the context rather than beside it because it is the same
	// kind of fact: something about the run that its own text does not say.
	features Features
}

// runes returns the two sides as the shortest slices that still answer the
// question a joining scan asks of them.
//
// That scan walks outward past the transparent characters — the marks, which
// take no form of their own — until it meets one that is not, and then stops. So
// the useful context is everything up to and including the first non-transparent
// character on each side, and carrying more would be decoding characters whose
// answer is already settled.
//
// The side before is read from its end backwards, so that what it costs is
// what the scan uses and not the length of everything a caller put in front of
// the run.
func (c shapeContext) runes() (before, after []rune) {
	for s := c.before; s != ""; {
		r, size := utf8.DecodeLastRuneInString(s)
		s = s[:len(s)-size]
		before = append(before, r)
		if joiningTypeOf(r) != joinT {
			break
		}
	}
	for i, j := 0, len(before)-1; i < j; i, j = i+1, j-1 {
		before[i], before[j] = before[j], before[i]
	}
	for _, r := range c.after {
		after = append(after, r)
		if joiningTypeOf(r) != joinT {
			break
		}
	}
	return before, after
}

// contextRunes is how much of the text either side of a run is carried to it as
// context when a string is cut into runs by direction.
//
// Everything that reads a context reads an end of it: the joining scan walks
// out to the first character that is not transparent, and the pairs across a
// run's edge are looked up in boundaryWindow characters. So a bounded end is
// all that is needed, and carrying each whole side cost a copy of the string
// per run. Twice boundaryWindow leaves the joining scan room to step over the
// marks a letter carries, which HarfBuzz bounds at five characters of context
// in all; a letter with more than sixty marks between it and the edge of a run
// is joined as though they were the whole of its context.
const contextRunes = 2 * boundaryWindow

// contextBefore is the end of outer+inner that a run after them needs, built
// without copying either whole.
func contextBefore(outer, inner string) string {
	if tail := lastRunes(inner, contextRunes); len(tail) < len(inner) {
		return tail
	}
	return lastRunes(outer, contextRunes-utf8.RuneCountInString(inner)) + inner
}

// contextAfter is the start of inner+outer that a run before them needs.
func contextAfter(inner, outer string) string {
	if head := firstRunes(inner, contextRunes); len(head) < len(inner) {
		return head
	}
	return inner + firstRunes(outer, contextRunes-utf8.RuneCountInString(inner))
}

// ShapeGlyphsWith is ShapeGlyphs with extra features named by the caller: the
// optional ones a font offers and nothing turns on by itself, small capitals or
// oldstyle figures. A tag the font does not declare is ignored rather than
// refused, because a caller asking for small capitals of a face that has none
// wants the text, not an error.
func (f *Face) ShapeGlyphsWith(s string, features ...string) ([]Glyph, int) {
	return f.shapeGlyphsWith(s, features, shapeContext{})
}

func (f *Face) shapeGlyphsWith(s string, extra []string, ctx shapeContext) ([]Glyph, int) {
	runs := bidiVisualRuns(s)
	if len(runs) <= 1 {
		// One direction throughout, which is nearly all text: shaped as one run
		// per script, which for nearly all of that is the string whole — and
		// whole keeps a ligature or a kern pair that spans it.
		rtl := len(runs) == 1 && runs[0].RTL()
		return f.shapeDirection(s, scriptBehind(ctx.before), scriptAhead(ctx.after), rtl, extra, ctx)
	}
	var (
		out     []Glyph
		missing int
	)
	// The script beside every run at once — see scriptsBeside for why not one
	// by one.
	pieces := make([][2]int, len(runs))
	for i, r := range runs {
		pieces[i] = [2]int{r.Start, r.End}
	}
	behind, ahead := scriptsBeside(s, pieces, scriptBehind(ctx.before), scriptAhead(ctx.after))
	for i, r := range runs {
		piece := s[r.Start:r.End]
		// A run inside the string has the rest of the string for context, and
		// the caller's context outside that. The two are concatenated rather
		// than one replacing the other: the caller's is what comes before all of
		// s, so it belongs before the part of s that precedes this run.
		//
		// Replacing was the first version and was wrong in the one case that
		// matters. A right-to-left run reaches a backend as an override
		// character followed by the text — see ShapedText — so the text is never
		// the first run of the string, and the override alone stood in for the
		// word the letters were supposed to join to.
		//
		// Only the ends that touch the run are kept — see contextRunes. The
		// whole of each side was concatenated for every run, which copied the
		// string once per run: a run of digits every other character is a run
		// per two characters, and the copies were quadratic in the text.
		inner := shapeContext{
			before: contextBefore(ctx.before, s[:r.Start]),
			after:  contextAfter(s[r.End:], ctx.after),
			kerns:  ctx.kerns,
			// What the caller turned off is off for every run of the string.
			// It was dropped here, so a document that said "font-kerning: none"
			// got it for a Latin word and not for the same word beside a Hebrew
			// one — the same declaration, honoured or not by whether the
			// paragraph happened to change direction.
			features: ctx.features,
		}
		// The sides that may contribute *glyphs* belong to the pieces they
		// touch: what precedes the whole string precedes its first run, and
		// what follows it follows its last. They were dropped as well, so a
		// ligature across an element boundary was formed for a run of one
		// direction and not for the same run beside text of the other.
		if r.Start == 0 {
			inner.mergeBefore = ctx.mergeBefore
		}
		if r.End == len(s) {
			inner.mergeAfter = ctx.mergeAfter
		}
		glyphs, gone := f.shapeDirection(piece, behind[i], ahead[i], r.RTL(), extra, inner)
		missing += gone
		for i := range glyphs {
			glyphs[i].Cluster += r.Start
		}
		out = append(out, glyphs...)
	}
	return out, missing
}

// shapeDirection shapes a string that runs one way throughout, as one run per
// script — see scriptRuns. behind and ahead are the scripts of the text either
// side of the string, for its characters that decide none.
//
// Nearly every string is in one script, and is shaped whole, as it always was.
// One that changes script is shaped a piece at a time, each piece with the rest
// of the string either side as its context, the way the bidi loop above gives
// each direction its neighbours: the forms a letter takes still see across the
// cut, and the pairs kerned do not. The pieces come back in the order they are
// drawn, which in a right-to-left run is the last piece first.
func (f *Face) shapeDirection(s string, behind, ahead uint16, rtl bool, extra []string, ctx shapeContext) ([]Glyph, int) {
	if !f.composite() {
		// A face set by character code has no rules to read per script, and
		// nothing to merge a neighbour's glyphs into.
		return f.shapeByCode(s, rtl)
	}
	if ctx.mergeBefore != "" || ctx.mergeAfter != "" {
		return f.shapeMerged(s, rtl, extra, ctx)
	}
	var one [1]scriptRun
	pieces := scriptRuns(s, behind, ahead, one[:0])
	if len(pieces) == 1 {
		return f.shapeGlyphsIn(s, pieces[0].script, rtl, extra, ctx)
	}
	var (
		out     []Glyph
		missing int
	)
	for k := range pieces {
		p := pieces[k]
		if rtl {
			p = pieces[len(pieces)-1-k]
		}
		inner := ctx
		if p.start > 0 {
			inner.before = contextBefore(ctx.before, s[:p.start])
			inner.cutBefore = true
		}
		if p.end < len(s) {
			inner.after = contextAfter(s[p.end:], ctx.after)
			inner.cutAfter = true
		}
		glyphs, gone := f.shapeGlyphsIn(s[p.start:p.end], p.script, rtl, extra, inner)
		missing += gone
		for i := range glyphs {
			glyphs[i].Cluster += p.start
		}
		out = append(out, glyphs...)
	}
	return out, missing
}

// shapeGlyphsIn is ShapeGlyphs with the run's script and direction already
// decided, and shapes one run rather than a whole string.
//
// A caller that split the text into runs knows more about a run's script than
// the run's own characters say: a stretch of digits between two Greek words is
// Greek, and shaping it as if it were scriptless would select the font's
// default rules where its Greek ones were meant. Stack.ShapeRuns made that
// decision when it cut the runs, and passes it here rather than having it
// guessed again from less. The same holds for direction, which is a property of
// the whole paragraph and cannot be read off one run of it.
func (f *Face) shapeGlyphsIn(s string, script uint16, rtl bool, extra []string, ctx shapeContext) ([]Glyph, int) {
	if !f.composite() {
		return f.shapeByCode(s, rtl)
	}
	// Which model sets the run is decided by the script and by the tag the
	// font's rules for it were read under — see categorize — and it decides
	// everything below: how the characters are normalised, whether the ones
	// nothing is drawn for are taken out now, and what is done with the glyphs.
	langs := openTypeLanguages(ctx.features.Language)
	l := f.layoutFor(script, langs)
	chosen := f.chosenScriptTag(script, langs)
	model := categorize(script, chosen)
	// Rule L4: a bracket in a right-to-left run is drawn as the bracket that
	// mirrors it, and the substitution is on the character, before the font is
	// asked for a glyph at all. Where the font has no glyph for the mirror the
	// character is kept, and its own glyph is what 'rtlm' is asked about.
	runes, offsets := bidiRunCharacters(s, rtl)
	if rtl {
		f.keepUnmirrorable(s, runes, offsets)
	}
	// Thai and Lao's one rearrangement of the text, which every shaper makes
	// whatever the font says, and for a Thai font with no Thai rules of its own
	// the older fonts' way of stacking marks. See thai.go.
	if model == modelThai {
		runes, offsets = thaiPreprocess(runes, offsets)
		if scriptSelects(script, "thai") && chosen != "thai" {
			runes = f.thaiPUAShape(runes)
		}
	}
	// Then normalisation, which is about the characters too and has to see the
	// mirrored ones: it puts the run into the spelling this face draws best and
	// each cluster's marks into canonical order. It runs before any glyph is
	// chosen because it decides which characters the font is asked about at all.
	// See normalize.go.
	runes, offsets = f.normalize(runes, offsets, model.syllabic(), model == modelIndic,
		scriptSelects(script, "arab"))
	// The characters nothing is drawn for, for every run but a syllabic one.
	//
	// Removing them here means no rule of the font is ever asked about a glyph
	// that will not be there, and for a script whose rules are lookups that is
	// the same answer as keeping them and having every lookup step over them,
	// which is what HarfBuzz does. Measurement agrees: Latin, Greek, Cyrillic
	// and Arabic differ in nothing either way.
	//
	// A syllable model is not a lookup and cannot step over anything. Whether
	// such a character breaks a syllable is a question the model has to be
	// allowed to answer, and it can only answer it if it is given the character
	// — so a syllabic run keeps them, and the shaper that gets them drops them
	// once they have said which cluster they broke. See ignorable.go.
	if !model.syllabic() {
		runes, offsets = dropHiddenCharacters(runes, offsets)
	}
	if len(runes) == 0 {
		return nil, 0
	}
	var (
		buf     []Glyph
		missing int
	)
	for i, r := range runes {
		gid, ok := f.GlyphID(r)
		if !ok {
			// A character nothing draws is not one the face is missing.
			//
			// The join controls reach here because the joining scan has to see
			// them — dropHiddenCharacters keeps them back for exactly that —
			// and the shaper takes them out again before any rule or any pen
			// sees the buffer: hideJoiners on the path that chooses cursive
			// forms, the syllable model's own pass on the other. Counting them
			// was counting a glyph that was never going to be asked for.
			//
			// It decides which face sets a word. A caller's fallback asks "can
			// this face set the whole of this text" and reads the answer here,
			// and an emoji sequence is one grapheme cluster with a zero width
			// joiner inside it: a face holding every visible character of
			// "\U0001F468\u200D\U0001F4BB" reported one missing and was passed
			// over for a face holding none of them, which then set both emoji as
			// spaces.
			//
			// The Hangul fillers are not among these and still count: they are
			// default-ignorable and they are *drawn*, which is what
			// hiddenAfterShaping is the list of.
			if !hiddenAfterShaping(r) {
				missing++
			}
			gid = 0
		}
		buf = append(buf, Glyph{
			GID: gid, Cluster: offsets[i], XAdvance: f.advanceGID(gid),
			class: classOfRune(runes[i]),
		})
	}
	if len(buf) == 0 {
		return nil, missing
	}
	// The run's script decides which of the font's rules apply, and everything
	// below reads the tables through it.
	sh := shaper{f: f, l: l, rtl: rtl, ligIDs: new(int),
		zeroMarks: model.zeroMarks(), features: ctx.features, langs: langs,
		ops: lookupBudget(len(buf))}
	// What the run applies, and in which stages: see plan.go. It covers every
	// entry point — the features a document turned off or asked for, and the
	// ones a caller named by tag, are requests to the same plan and not passes
	// of their own.
	p := sh.planFor(model, scriptSelects(script, "arab"), extra)
	// The glyphs particular features are for, where the plan has any: the
	// numerator and denominator around a fraction slash, and the glyphs whose
	// mirrored form is the font's to give.
	if p.fractions {
		maskFractions(buf, runes, rtl)
	}
	if p.rtlm {
		maskUnmirrored(buf, runes, s)
	}
	// A script whose characters are not in the order they are drawn is shaped
	// whole by its own pass: the reordering decides which of the font's rules
	// apply where, so it cannot be a step before the general substitutions and
	// has to be the substitutions. No script both joins cursively and reorders,
	// which is why these are alternatives rather than stages.
	before, after := ctx.runes()
	if model.syllabic() {
		buf = sh.shapeSyllabic(buf, runes, script, p, before, after)
	} else {
		// Which form each letter takes is decided now, while the glyphs still
		// correspond to the characters it is decided from, and recorded on the
		// glyphs so that it survives what follows. The join controls have said
		// all they have to say once that is done, and are taken out before any
		// substitution can see them — see ignorable.go.
		if model == modelArabic {
			markJoiningForms(buf, runes, before, after)
		}
		buf = hideJoiners(buf, runes)
		for _, stage := range p.stages {
			buf = sh.applyStage(buf, stage)
		}
	}
	sh.position(buf)
	// The pair that spans the boundary to the next run, which the pass above
	// cannot see because the glyph on the far side of it is not in this buffer.
	// See boundarykern.go.
	if len(sh.l.kern) > 0 && ctx.kerns && !ctx.features.NoKerning {
		kctx := ctx
		if ctx.cutBefore {
			kctx.before = ""
		}
		if ctx.cutAfter {
			kctx.after = ""
		}
		if kctx.before != "" || kctx.after != "" {
			before, after := f.boundaryGlyphs(kctx, script, rtl)
			sh.kernAcross(buf, before, after)
		}
	}
	if rtl {
		// Last, and only now. Everything above is stated by the font in terms of
		// the order the text is written in; the pen will meet these glyphs in the
		// other one.
		reverseGlyphs(buf)
	}
	for _, g := range buf {
		f.used[g.GID] = true
	}
	return buf, missing
}

// keepUnmirrorable undoes rule L4 for a character whose mirror this face has no
// glyph for, so that the character is drawn as itself rather than as .notdef.
// It is what HarfBuzz does, and the character's own glyph is then what the
// font's 'rtlm' is asked about — see maskUnmirrored.
//
// runes is one to one with the characters of s at offsets, which is what
// bidiRunCharacters hands back, and is changed in place.
func (f *Face) keepUnmirrorable(s string, runes []rune, offsets []int) {
	for i, r := range runes {
		orig, _ := utf8.DecodeRuneInString(s[offsets[i]:])
		if r != orig && !f.hasGlyph(r) {
			runes[i] = orig
		}
	}
}

// maskUnmirrored marks, in a right-to-left run, every glyph Unicode's mirroring
// did not replace, as one 'rtlm' is for. That is every glyph but the mirrored
// ones: a character with no mirror, and one whose mirror the face could not
// draw, both leave the choice of a mirrored form to the font.
//
// buf is one to one with runes, and each glyph's cluster is the offset in s of
// the character it came from.
func maskUnmirrored(buf []Glyph, runes []rune, s string) {
	for i := range buf {
		if i >= len(runes) || buf[i].Cluster < 0 || buf[i].Cluster >= len(s) {
			continue
		}
		orig, _ := utf8.DecodeRuneInString(s[buf[i].Cluster:])
		if m, ok := bidi.MirrorOf(orig); ok && m == runes[i] && m != orig {
			continue
		}
		buf[i].mask |= maskRtlm
	}
}

// maskFractions marks the glyphs of each fraction written with U+2044 FRACTION
// SLASH between runs of decimal digits: the digits before it for the
// numerator, the ones after for the denominator, and all of it for 'frac'. It
// is HarfBuzz's automatic fractions, which it applies by default, so that
// "1\u20442" is set as a fraction by any font that has the forms.
//
// buf is one to one with runes. In a right-to-left run the two sides swap, as
// HarfBuzz swaps them: the run is in logical order and the numerator is still
// what was written first.
func maskFractions(buf []Glyph, runes []rune, rtl bool) {
	pre, post := maskNumr|maskFrac, maskFrac|maskDnom
	if rtl {
		pre, post = maskFrac|maskDnom, maskNumr|maskFrac
	}
	for i := 0; i < len(runes) && i < len(buf); i++ {
		if runes[i] != fractionSlash {
			continue
		}
		start, end := i, i+1
		for start > 0 && unicode.Is(unicode.Nd, runes[start-1]) {
			start--
		}
		for end < len(runes) && end < len(buf) && unicode.Is(unicode.Nd, runes[end]) {
			end++
		}
		if start == i || end == i+1 {
			continue
		}
		for j := start; j < i; j++ {
			buf[j].mask |= pre
		}
		buf[i].mask |= maskFrac
		for j := i + 1; j < end; j++ {
			buf[j].mask |= post
		}
		i = end - 1
	}
}

// fractionSlash is U+2044, the character that makes a fraction of the digits
// either side of it.
const fractionSlash = 0x2044

// shapeByCode is the shaping path for a face whose codes are characters rather
// than glyph indices: the fourteen standard faces, and any face embedded as a
// simple font.
//
// It applies no substitution and no positioning, and that is not a shortcut. A
// one-byte code addresses at most 256 glyphs, so a ligature the font has cannot
// generally be named at all; and every layout table is keyed by glyph index,
// which the code is not — looking a kern pair up by code finds either nothing
// or the wrong pair. What such a face can do correctly is one code per
// character at the width the font publishes, and that is what this does.
//
// Callers get the same Glyph values either way, so Draw, Measure and the
// fallback stack do not have to know which kind of face they were given.
func (f *Face) shapeByCode(s string, rtl bool) ([]Glyph, int) {
	runes, offsets := bidiRunCharacters(s, rtl)
	// A simple face draws nothing for these either. It is more visible here, if
	// anything: WinAnsi gives U+00AD a code of its own, so a soft hyphen without
	// this reaches the page as a hyphen — and a simple face has no shaping pass
	// later on that could take it back out.
	//
	// The join controls go too, which is what makes this the drawing predicate
	// and not the shaping one. There is nothing here for them to instruct, and
	// left in they take the substitution an unmapped character gets and reach the
	// page as a space.
	runes, offsets = dropHiddenBeforeDrawing(runes, offsets)
	var (
		buf     []Glyph
		missing int
	)
	for i, r := range runes {
		code, ok := f.GlyphID(r)
		if !ok {
			missing++
			// The same substitution Encode makes: an unmapped character is set
			// as a space, which is what a reader shows for an undefined code.
			if space, spaceOK := f.GlyphID(' '); spaceOK {
				code = space
			} else {
				code = 0
			}
		}
		width, _ := f.Advance(r)
		if !ok {
			width, _ = f.Advance(' ')
		}
		buf = append(buf, Glyph{GID: code, Cluster: offsets[i], XAdvance: width})
	}
	if rtl {
		// There is nothing here for the direction to interfere with — no marks,
		// no joining, no kerning — but the run still has to come back in the
		// order it is drawn, so that a caller need not ask which kind of face it
		// was given.
		reverseGlyphs(buf)
	}
	for _, g := range buf {
		f.used[g.GID] = true
	}
	return buf, missing
}

// MeasureGlyphs is the width a shaped run occupies at a given size, which is
// the sum of its advances — offsets displace glyphs without moving the pen and
// so contribute nothing.
func MeasureGlyphs(glyphs []Glyph, size float64) float64 {
	var total float64
	for _, g := range glyphs {
		total += g.XAdvance
	}
	return total * size / 1000
}

// reverseGlyphs puts a shaped run into visual order.
//
// It is the last step of shaping a right-to-left run and cannot be an earlier
// one. Everything before it — joining, ligatures, contextual rules, kerning,
// cursive attachment, marks — is stated by the font in terms of the order the
// text is written in, and applying any of it to a reversed buffer applies it to
// the wrong neighbours.
func reverseGlyphs(buf []Glyph) {
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
}

// shapeMerged shapes a run together with the neighbours that may contribute
// glyphs to it, and keeps the glyphs that belong to the run.
//
// The whole is shaped once and cut by cluster: a glyph's Cluster is the byte
// offset of the first character it came from, so a ligature that swallows the
// start of this run has a cluster in the run before it and is left there. The
// run then draws nothing for those characters and takes none of their width,
// which is the arithmetic that makes the two halves add up to what one run
// would have measured.
//
// The sides that may *not* merge stay outside as ordinary context, so a run
// with a mergeable neighbour on one side and a plain one on the other still
// takes its forms from both.
//
// The whole is cut by script as any string is, and it is the whole that is
// cut and not the run: ShapeGroup shapes the same concatenation for the same
// group, with the same context outside it, and the two have to agree about
// which characters are one run of the font's rules or they disagree about the
// glyphs.
//
// It is reached only where something may merge, which is few runs of few
// documents; every other run takes shapeDirection's ordinary path.
func (f *Face) shapeMerged(s string, rtl bool, extra []string,
	ctx shapeContext) ([]Glyph, int) {

	pre, post := ctx.mergeBefore, ctx.mergeAfter
	// What is merged already carries the forms of that side, so the context
	// left outside is the other one's — and only where nothing merged there.
	outer := shapeContext{kerns: ctx.kerns, features: ctx.features}
	if pre == "" {
		outer.before = ctx.before
	}
	if post == "" {
		outer.after = ctx.after
	}
	glyphs, _ := f.shapeDirection(pre+s+post, scriptBehind(outer.before), scriptAhead(outer.after),
		rtl, extra, outer)
	lo, hi := len(pre), len(pre)+len(s)
	out := glyphs[:0:0]
	for _, g := range glyphs {
		if g.Cluster < lo || g.Cluster >= hi {
			continue
		}
		g.Cluster -= lo
		out = append(out, g)
	}
	// The count of characters no glyph was found for is the whole string's, and
	// this run is a part of it. Reporting the whole would have a run named for
	// its neighbour's missing characters as well as its own.
	return out, f.missingIn(s)
}

// missingIn counts the characters of a string this face has no glyph for.
func (f *Face) missingIn(s string) int {
	n := 0
	for _, r := range s {
		if _, ok := f.GlyphID(r); !ok {
			n++
		}
	}
	return n
}

// ShapeGroup shapes a whole merge group — the runs that shape as one string,
// concatenated — so that every run of it can take its own slice of the result
// rather than shaping the group again for itself.
//
// The group and not the run is what is shaped, because two runs of one word
// that shape different strings disagree about where a ligature begins and a
// character between them is drawn by neither. That the group is the same string
// for every run of it is also what makes it memoizable: shaped once per run, a
// group of a thousand runs shapes a thousand characters a thousand times over.
//
// before and after are the context the group itself does not hold. See
// GroupContext, and GroupSpan for cutting one run out of the result.
func (f *Face) ShapeGroup(whole, before, after string, kerns bool, off Features) []Glyph {
	glyphs, _ := f.shapeGlyphsWith(whole, nil,
		shapeContext{before: before, after: after, kerns: kerns, features: off})
	return glyphs
}
