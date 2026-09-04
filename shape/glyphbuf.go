package shape

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

	// join is the positional form this glyph takes in a cursive script, decided
	// from the characters either side of it before anything was substituted.
	//
	// It is carried on the glyph rather than worked out when it is needed
	// because by then it cannot be: the substitutions that come first change how
	// many glyphs there are, so nothing maps back to the characters the form was
	// decided from. See arabic.go.
	join joinForm
}

// joinForm is the shape a letter takes from its neighbours.
type joinForm uint8

const (
	joinNone joinForm = iota // not a letter of a cursive script
	joinIsolated
	joinFinal
	joinMedial
	joinInitial
)

// tag is the feature a font states this form under.
func (j joinForm) tag() string {
	switch j {
	case joinIsolated:
		return featIsolated
	case joinFinal:
		return featFinal
	case joinMedial:
		return featMedial
	case joinInitial:
		return featInitial
	}
	return ""
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
func (c shapeContext) runes() (before, after []rune) {
	for _, r := range c.before {
		before = append(before, r)
	}
	for i := len(before) - 1; i >= 0; i-- {
		if joiningTypeOf(before[i]) != joinT {
			before = before[i:]
			break
		}
	}
	for _, r := range c.after {
		after = append(after, r)
		if joiningTypeOf(r) != joinT {
			break
		}
	}
	return before, after
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
		// One direction throughout, which is nearly all text. Shaping it whole
		// keeps a ligature or a kern pair that spans the string, which cutting
		// it into runs would lose.
		rtl := len(runs) == 1 && runs[0].RTL()
		return f.shapeGlyphsIn(s, runScript(s), rtl, extra, ctx)
	}
	var (
		out     []Glyph
		missing int
	)
	for _, r := range runs {
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
		inner := shapeContext{
			before: ctx.before + s[:r.Start],
			after:  s[r.End:] + ctx.after,
			kerns:  ctx.kerns,
		}
		glyphs, gone := f.shapeGlyphsIn(piece, runScript(piece), r.RTL(), extra, inner)
		missing += gone
		for i := range glyphs {
			glyphs[i].Cluster += r.Start
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
	if out, missing, ok := f.shapeMerged(s, script, rtl, extra, ctx); ok {
		return out, missing
	}
	// Rule L4: a bracket in a right-to-left run is drawn as the bracket that
	// mirrors it, and the substitution is on the character, before the font is
	// asked for a glyph at all.
	runes, offsets := bidiRunCharacters(s, rtl)
	// Then normalisation, which is about the characters too and has to see the
	// mirrored ones: it puts the run into the spelling this face draws best and
	// each cluster's marks into canonical order. It runs before any glyph is
	// chosen because it decides which characters the font is asked about at all.
	// See normalize.go.
	runes, offsets = f.normalize(runes, offsets, usesSyllabicShaper(script), indicConfigFor(script) != nil,
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
	if !usesSyllabicShaper(script) {
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
			missing++
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
	sh := shaper{f: f, l: f.layoutFor(script), rtl: rtl, ligIDs: new(int),
		zeroMarks: zeroMarkWidthsFor(script), features: ctx.features}
	// A script whose characters are not in the order they are drawn is shaped
	// whole by its own pass: the reordering decides which of the font's rules
	// apply where, so it cannot be a step before the general substitutions and
	// has to be the substitutions. No script both joins cursively and reorders,
	// which is why these are alternatives rather than stages.
	before, after := ctx.runes()
	if out, ok := sh.shapeSyllabic(buf, runes, script, before, after); ok {
		buf = out
	} else {
		// Which form each letter takes is decided now, while the glyphs still
		// correspond to the characters it is decided from, and recorded on the
		// glyphs so that it survives what follows. The join controls have said
		// all they have to say once that is done, and are taken out before any
		// substitution can see them — see ignorable.go.
		markJoiningForms(buf, runes, before, after)
		buf = hideJoiners(buf, runes)
		buf = sh.substitute(buf)
	}
	// Features the caller asked for, after the ones every run gets. They are
	// applied through the lookup list like any others rather than as a table of
	// single substitutions, so a face whose small capitals are a contextual rule
	// or a ligature gets them right.
	buf = sh.applyNamedFeatures(buf, extra)
	sh.position(buf)
	// The pair that spans the boundary to the next run, which the pass above
	// cannot see because the glyph on the far side of it is not in this buffer.
	// See boundarykern.go.
	if len(sh.l.kern) > 0 && ctx.kerns && !ctx.features.NoKerning &&
		(ctx.before != "" || ctx.after != "") {
		before, after := f.boundaryGlyphs(ctx, script, rtl)
		sh.kernAcross(buf, before, after)
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

// nominalAdvance is how far the text-showing operator will move the pen for one
// glyph, which is the font's own width for whatever the code names.
//
// For a composite face that is the width of the glyph index. For the others no
// positioning was applied, so the advance already in the buffer *is* the font's
// own — and asking for it by index would look the width up under a number that
// is a character code.
func (f *Face) nominalAdvance(g Glyph) float64 {
	if !f.composite() {
		return g.XAdvance
	}
	return f.advanceGID(g.GID)
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

// The substitution features applied to every run, in the order a shaper applies
// them, split by where the positional forms of a cursive script go between them.
//
// They are the ones that are not a matter of taste. 'ccmp' composes and
// decomposes so the later rules have the glyphs they are written against;
// 'locl' is the letterform a language expects; 'rlig' is required by the script;
// 'liga' and 'clig' are the ligatures a reader expects to see; 'calt' and 'rclt'
// pick the variant that fits its neighbours. A font that declares them means
// them, which is what separates these from 'smcp' or 'onum' — those change what
// the text says it is, and wait to be asked for (ShapeWith).
//
// The order matters and is not alphabetical: composition before the rules that
// read its output, required ligatures before optional ones, contextual
// alternates last so they see the glyphs that survived.
//
// Where the forms go is the part that is easy to get wrong and expensive to get
// wrong. They come *after* 'ccmp', because a real Arabic font does not state
// them over the letters: Noto Sans Arabic splits every letter into a skeleton
// and its dots in 'ccmp' and states the four forms over the skeletons. Applying
// the forms first finds nothing, and every letter is set in its isolated shape —
// which is legible only to someone who already knows what it should say.
var (
	beforeJoiningFeatures = []string{"ccmp", "locl"}
	afterJoiningFeatures  = []string{"rlig", "rclt", "calt", "liga", "clig"}
)

// substitute runs the GSUB lookups over a shaped buffer, preserving the cluster
// of the first glyph of each run it replaces so that a ligature still maps back
// to the text it came from.
// applyNamedFeatures runs the lookups of features a caller named, in the order
// they were named.
//
// A feature the font does not declare does nothing, which is the contract
// ShapeWith states: asking a face with no small capitals for small capitals
// should set the text plainly rather than fail.
func (sh shaper) applyNamedFeatures(buf []Glyph, tags []string) []Glyph {
	for _, tag := range tags {
		if lookups := sh.l.featureLookups[tag]; len(lookups) > 0 {
			buf = sh.applyContextual(buf, lookups)
		}
	}
	return buf
}

func (sh shaper) substitute(buf []Glyph) []Glyph {
	buf = sh.applyNamedFeatures(buf, beforeJoiningFeatures)
	buf = sh.applyJoiningForms(buf)
	// The features a document turned off are dropped from the list rather than
	// skipped inside the loop, so that what is left keeps the order the
	// specification requires. See Features.keeps.
	return sh.applyNamedFeatures(buf, sh.features.keeps(afterJoiningFeatures))
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
// It reports false where nothing may merge, which is every run of almost every
// document: the caller then takes the ordinary path and pays nothing for this.
func (f *Face) shapeMerged(s string, script uint16, rtl bool, extra []string,
	ctx shapeContext) ([]Glyph, int, bool) {

	if ctx.mergeBefore == "" && ctx.mergeAfter == "" {
		return nil, 0, false
	}
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
	glyphs, _ := f.shapeGlyphsIn(pre+s+post, script, rtl, extra, outer)
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
	return out, f.missingIn(s), true
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
