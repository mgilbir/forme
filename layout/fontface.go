package layout

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// The faces a document brings with it.
//
// An @font-face rule is a document saying "here is a font, call it this". It is
// the only way a document can be set in a face the caller did not supply, and
// it is what makes an HTML-to-PDF engine able to render a page the way its
// author designed it rather than the way the fourteen standard PDF faces allow.
//
// # Why this is a resource and not a stylesheet matter
//
// The rule is written in CSS and everything interesting about it happens
// outside the cascade: it does not select anything, it does not compute a
// value, and it does not inherit. What it does is *load a file*, and that puts
// it in this package rather than in style, behind resource.go's policy in full —
// no scheme, no absolute path, no escape from the resolver's directory, and
// nothing at all when there is no resolver. There is no second loading path
// here that could disagree with the one <img> and <link rel=stylesheet> use.
//
// # Why the caps are tighter than the ones on an image
//
// The bytes go to shape.Load, which parses an sfnt: a table directory, a
// character map, a set of outlines and — for anything with shaping — a stack of
// layout tables full of offsets into each other. That is a larger parser
// reading more attacker-controlled structure than a PNG decoder, and it is
// reached from a *stylesheet*, which is one indirection further from anything
// the caller wrote than an <img src> is.
//
// So a document gets a budget, in the shape image.go already uses: a cap on one
// file, a cap on how many faces it may end up with, a cap on the total bytes
// handed to the parser, and a cap on how many rules are looked at at all. The
// last is the one that answers a page declaring a thousand @font-face rules —
// the byte budget alone would not, because a rule whose file is missing costs a
// resolver call and no bytes, and a thousand of those is a thousand system
// calls the document did not have to pay for.

// maxFontBytes is the largest single font program this engine will parse.
//
// Eight megabytes is past every web font in use — a full-coverage Noto face is
// around five, a subset Latin one is tens of kilobytes — and small enough that
// one file cannot be the whole budget. It bounds what reaches shape.Load, which
// is the point: the cap exists to bound a parser, not a download.
//
// A variable rather than a constant so a test can lower it and watch it fire. A
// cap nobody has seen trip is one nobody knows works.
var maxFontBytes = 8 << 20

// maxDocumentFontBytes is the total this engine will hand to the font parser
// for one document.
//
// A per-file cap does not bound a document: fifty files of eight megabytes are
// four hundred megabytes, and each one passes the per-file check. This is the
// budget that makes the total finite, and it is charged for bytes *read*,
// whether or not the parse then succeeded — a font program that fails to parse
// cost the same read and the same allocation as one that did not.
var maxDocumentFontBytes = 32 << 20

// maxDocumentFaces is how many faces one document may end up with.
//
// It bounds the number of font programs parsed, which is the expensive and
// dangerous half. A document needs a handful — a family in four weights is
// four — and one that names twenty is doing something other than setting text.
var maxDocumentFaces = 20

// maxFontFaceRules is how many @font-face rules one document's stylesheets may
// have looked at.
//
// This is the cap that answers the thousand-rule page. The two above bound the
// parsing and the reading; neither bounds a rule whose every src fails, which
// costs a resolver call apiece and no bytes at all. Fifty is more @font-face
// rules than any real document has and is a bound on the resolver traffic a
// stylesheet can direct.
var maxFontFaceRules = 50

// maxFontSources is how many entries of one rule's src list are tried.
//
// The list is a fallback chain and is meant to be short: a woff2, a woff, a
// ttf, and a local() before them. Sixteen is well past that and stops one rule
// from being a loop.
var maxFontSources = 16

// fontSource is one entry of an @font-face rule's src list.
type fontSource struct {
	// local marks a local(name) entry, whose name is looked up in the font set
	// the caller supplied rather than loaded from anywhere.
	local bool
	// ref is the reference a url() entry names, or the face name a local() one
	// does.
	ref string
	// format is the format() hint, lowercased, or empty when there was none.
	format string
}

// fontFaceRule is one @font-face, as far as this engine reads one.
type fontFaceRule struct {
	family string
	srcs   []fontSource

	// weight, width and style are the three descriptors CSS Fonts 4 §5.2
	// chooses between a family's faces by, each a range: a single value is a
	// range of one, which is what makes the matching uniform over
	// "font-weight: 700" and "font-weight: 100 900". A descriptor the rule
	// leaves out is its initial value, "auto" — selected as the normal value,
	// and not a range a variable face's axes are clamped to (§4.4).
	weight, width faceRange
	style         faceStyle

	// ranges is the unicode-range descriptor: the characters this face is for.
	// nil means the descriptor was absent or covered the whole of Unicode,
	// which are the same thing and are the common case — a face with no
	// restriction is asked no questions.
	ranges []unicodeSpan

	// features is the font-feature-settings descriptor, in the order it was
	// written, and featuresAt is where in the sheet it was: the features the
	// rule asks of every run set in the face it loads, at CSS Fonts 4 §7.2's
	// second step. nil where the descriptor was absent, "normal", or not a
	// value that could be read. See withFeatureSettings.
	features   []shape.FeatureSetting
	featuresAt int

	// namedInstance is the font-named-instance descriptor, the name of one of
	// a variable face's named instances, or empty for auto; and variations
	// the font-variation-settings descriptor. They are §7.2's fifth and sixth
	// steps, applied where the face is set (fontinstance.go).
	namedInstance string
	variations    []variationSetting
}

// faceRange is a font-weight or font-width descriptor: a range, or auto.
type faceRange struct {
	valueRange
	auto bool
}

// selected is the range §5.2 selects the face by: its own, or for auto the
// property's normal value.
func (r faceRange) selected(normal float64) valueRange {
	if r.auto {
		return valueRange{normal, normal}
	}
	return r.valueRange
}

// faceStyle is a font-style descriptor.
type faceStyle struct {
	kind faceStyleKind
	// angles are an oblique face's, in CSS's sign: positive leans right.
	angles valueRange
}

type faceStyleKind int

const (
	styleAuto faceStyleKind = iota
	styleNormal
	styleItalic
	styleOblique
)

// offer is what the descriptor offers §5.2's style search. See fontmatch.go.
func (s faceStyle) offer() slopeOffer {
	switch s.kind {
	case styleItalic:
		return slopeOffer{italic: []valueRange{{1, 1}}}
	case styleOblique:
		return slopeOffer{oblique: []valueRange{s.angles}}
	}
	return slopeOffer{italic: []valueRange{{0, 0}}, oblique: []valueRange{{0, 0}}}
}

// matchable is the rule as §5.2 sees it.
func (r fontFaceRule) matchable() matchable {
	return matchable{
		width:  r.width.selected(100),
		weight: r.weight.selected(400),
		slope:  r.style.offer(),
	}
}

// covers reports whether this face may be used for a character.
//
// A rule with no ranges covers everything, which is what an absent unicode-range
// means and is why the nil case is not special: the loop over an empty list
// would answer "no" for every character, and the descriptor's absence must
// answer "yes".
func (r fontFaceRule) covers(c rune) bool {
	if len(r.ranges) == 0 {
		return true
	}
	for _, span := range r.ranges {
		if c >= span.lo && c <= span.hi {
			return true
		}
	}
	return false
}

// coversText reports whether this face may be used for every character of text.
func (r fontFaceRule) coversText(text string) bool {
	if len(r.ranges) == 0 {
		return true
	}
	for _, c := range text {
		if !r.covers(c) {
			return false
		}
	}
	return true
}

// pendingFontFace is an @font-face rule and the stylesheet it was written in,
// carried from the pipeline to the loader so a finding can say where it came
// from.
type pendingFontFace struct {
	rule  css.Rule
	sheet string
	// layer is the cascade layer the rule was written in; see fontFacesOf.
	layer int
}

// fontFacesOf is the document's @font-face rules as the cascade's walk handed
// them over, in the order the loader is to take them.
//
// The walk finds them wherever they are live — at the top of a sheet or inside
// an @media, @supports or @layer whose condition held — which is what makes
// "@media print { @font-face { … } }" a face and not, as it was, an at-rule
// reported "not applied yet" (audit C138). It took only the top-level ones out
// of the sheet before, and every other one reached the cascade as an unknown.
//
// The order is the cascade's. The loader lets the last rule declared win a tie
// between two faces of one family, and Cascade 5 §6.4.3 makes a name-defining
// at-rule in a later layer — or outside every layer — the later one, whatever
// order the text puts them in. So the rules are sorted by their layer's rank,
// stably, and an unlayered stylesheet keeps the order it was written in.
func fontFacesOf(rules []style.AtRule) []pendingFontFace {
	out := make([]pendingFontFace, 0, len(rules))
	for _, r := range rules {
		out = append(out, pendingFontFace{rule: r.Rule, sheet: r.Sheet, layer: r.Layer})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return style.LayerRank(out[i].layer, false) < style.LayerRank(out[j].layer, false)
	})
	return out
}

// documentFace is one loaded face together with what the rule said about it.
type documentFace struct {
	rule fontFaceRule
	// match is the rule's descriptors as §5.2 reads them, worked out once.
	match matchable
	face  *shape.Face
	// named is where the rule's font-named-instance is in the face's design
	// space, found once when the face loads; nil where the rule names none or
	// the face has none by that name.
	named map[string]float64
	// ref is the src entry that produced the face — the url for a url() entry,
	// the name for a local() one. It is kept so that a caller can say which
	// file a family came from.
	ref string
}

// documentFonts is the font set a document's own @font-face rules make, over
// the set the caller supplied.
//
// A family the document defined shadows one the caller has, which is what CSS
// says: an @font-face for "Helvetica" is the document's Helvetica for the rest
// of that document, whatever the system has under that name.
type documentFonts struct {
	base  FontSet
	faces []*documentFace
	// byFamily indexes faces by lowercased family, in declaration order.
	byFamily map[string][]*documentFace

	// mu guards mine and matched, which are filled as families are resolved.
	mu sync.Mutex
	// matched memoizes §5.2 by family and request: the faces it keeps, in
	// declaration order, and where it came to rest. A paragraph asks the
	// question once per grapheme cluster where its families carry a
	// unicode-range, and the answer depends on neither the cluster nor the
	// paragraph.
	matched map[familyRequest]familyMatch
	// mine is this document's own copy of every face it has been handed,
	// keyed by the face it was made from. See own.
	mine map[*shape.Face]*shape.Face

	// inst is the document's instances of variable faces, shared by the
	// cascade and layout. See fontinstance.go.
	inst *instancer
}

// own returns this document's copy of a face.
//
// A face remembers which glyphs it was asked to set, because that is what a
// subset is computed from — and it is written by shaping, which means by
// *measuring*, so a face is written to long before anything is drawn in it. A
// caller's font set outlives any one document and is documented as shared
// across goroutines, so handing its faces straight through made two things
// wrong at once: two documents laid out at the same time wrote the same map,
// which the race detector reports at the first word of text; and a face's tally
// accumulated over every document the process had ever set, so a subset built
// from it carried other documents' glyphs.
//
// Both are the same mistake, and shape has said what to do about it since
// before this was written: share the parse, not the face. Clone keeps the
// program, the tables and every reading of them — which is all of what loading
// a face costs — and gives the copy a tally of its own.
//
// Keyed by the source face, so a set answering "arial" and "helvetica" with one
// face gives this document one face for both, and the identity a run is grouped
// by holds.
func (d *documentFonts) own(f *shape.Face, ok bool) (*shape.Face, bool) {
	if !ok || f == nil {
		return f, ok
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if got := d.mine[f]; got != nil {
		return got, true
	}
	if d.mine == nil {
		d.mine = map[*shape.Face]*shape.Face{}
	}
	c := f.Clone()
	d.mine[f] = c
	return c, true
}

// fallbackDocumentFonts is documentFonts over a base that answers the
// coverage question too.
//
// The two types exist so that wrapping a FallbackFontSet does not lose the
// interface and wrapping a plain FontSet does not invent one. A wrapper that
// implemented FaceFor unconditionally would answer "no" for every base that
// cannot, which reads to inline.go as a set that was asked and had nothing —
// the same outcome by luck rather than by construction, and the sort of thing
// that stops being the same the day the caller of FaceFor grows a second
// branch.
type fallbackDocumentFonts struct{ *documentFonts }

func (d fallbackDocumentFonts) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	return d.FaceForStyled(text, requestFromFlags(bold, italic))
}

// FaceForStyled implements StyledFallbackFontSet, asking the base set by the
// numbers where it takes them.
func (d fallbackDocumentFonts) FaceForStyled(text string, r FontRequest) (*shape.Face, bool) {
	// The document's own faces are deliberately not offered here. FaceFor is
	// the question "what can set this text at all", and answering it with a
	// face the document loaded for some *other* family would substitute a
	// webfont for a script it was never chosen for. The base set is the one
	// that was given coverage as its job.
	return d.own(fallbackLookup(d.base)(text, r))
}

// Face answers for a family the document defined, and defers otherwise.
func (d *documentFonts) Face(family string, bold, italic bool) (*shape.Face, bool) {
	return d.faceFor(family, "", requestFromFlags(bold, italic))
}

// FaceStyled implements StyledFontSet: Face, by the numbers.
func (d *documentFonts) FaceStyled(family string, r FontRequest) (*shape.Face, bool) {
	return d.faceFor(family, "", r)
}

// FaceForFamily implements RangedFontSet: the face a family offers for a
// particular piece of text, which is not always the one it offers in general.
//
// A family whose faces carry unicode-range descriptors has a different answer
// per character — that is the whole of what the descriptor is for — so the
// question cannot be asked through FontSet, which has no text to ask about. See
// the note on RangedFontSet.
func (d *documentFonts) FaceForFamily(family, text string, bold, italic bool) (*shape.Face, bool) {
	return d.faceFor(family, text, requestFromFlags(bold, italic))
}

// FaceForFamilyStyled implements StyledRangedFontSet: FaceForFamily, by the
// numbers.
func (d *documentFonts) FaceForFamilyStyled(family, text string, r FontRequest) (*shape.Face, bool) {
	return d.faceFor(family, text, r)
}

// faceFor is the family lookup, optionally narrowed to the faces that may set a
// given text.
//
// An empty text asks the general question and considers every face, which is
// what FontSet's Face means and what a caller with no particular text in mind
// wants. It is deliberately not the same as "text no face covers": that comes
// back false, because a family whose every face excludes the text has nothing to
// offer and the next family in the document's list should be asked.
func (d *documentFonts) faceFor(family, text string, r FontRequest) (*shape.Face, bool) {
	df, _, defined := d.documentFaceFor(family, text, r)
	if df != nil {
		return df.face, true
	}
	if defined {
		return nil, false
	}
	// A family the document did not define is the caller's, and the caller's
	// set is asked the question it can answer. A plain FontSet knows nothing
	// of ranges, so a family it holds covers whatever it has glyphs for, which
	// is the question faceRunsFor asks next and not this one. A ranged set
	// does know, and is asked with the text: this wrapper is built for every
	// document, and answering a ranged caller from its Face alone made the
	// interface one that Layout honoured and Build and Compose never did (audit
	// C43). Either way the answer is one of the caller's faces, and this
	// document takes its own copy of it: see own.
	if ranged := rangedLookup(d.base); ranged != nil && text != "" {
		return d.own(ranged(family, text, r))
	}
	return d.own(faceIn(d.base, family, r))
}

// documentFaceFor is the face the document's own rules give a family for a
// request, and where §5.2 came to rest choosing it. defined reports whether
// the document defines the family at all: when it does not, the face is nil
// and the family is the caller's to answer.
//
// §5.2 chooses among the whole family and then, among the faces it kept, the
// first whose unicode-range covers the text, in reverse order of declaration —
// the composite face of §4.5.1, and the last rule declared winning a tie, which
// is the cascade's last term too. So a family whose bold face covers only
// Latin has nothing for bold Greek: the Greek falls to the next family the
// document named, as a browser sets it, rather than to the family's regular
// face, as this used to — it asked which faces covered the text before it asked
// which was bold.
func (d *documentFonts) documentFaceFor(family, text string, r FontRequest) (df *documentFace, m faceMatch, defined bool) {
	key := familyKey(family)
	candidates := d.byFamily[key]
	if len(candidates) == 0 {
		return nil, m, false
	}
	fm := d.match(key, candidates, r)
	for i := len(fm.keep) - 1; i >= 0; i-- {
		c := candidates[fm.keep[i]]
		if text == "" || c.rule.coversText(text) {
			return c, fm.at, true
		}
	}
	// Every face the match kept excludes the text. The family has nothing for
	// it, which is not the same as the document having nothing — the caller
	// walks on to the next family it named.
	return nil, fm.at, true
}

// familyRequest is what matched is keyed by.
type familyRequest struct {
	family string
	r      FontRequest
}

// familyMatch is one family's §5.2 answer for one request.
type familyMatch struct {
	keep []int
	at   faceMatch
}

// match is matchFaces over a family's faces, memoized.
func (d *documentFonts) match(key string, candidates []*documentFace, r FontRequest) familyMatch {
	d.mu.Lock()
	defer d.mu.Unlock()
	if got, ok := d.matched[familyRequest{key, r}]; ok {
		return got
	}
	faces := make([]matchable, len(candidates))
	for i, c := range candidates {
		faces[i] = c.match
	}
	keep, at := matchFaces(faces, r)
	fm := familyMatch{keep: keep, at: at}
	if d.matched == nil {
		d.matched = map[familyRequest]familyMatch{}
	}
	d.matched[familyRequest{key, r}] = fm
	return fm
}

// loadFontFaces turns a document's @font-face rules into the font set it is set
// in.
//
// base is what the caller supplied and is never nil by the time this is called.
//
// The wrapper is always built, even for a document with no @font-face rule of
// its own, because it has a second job besides holding those rules: it is what
// makes the faces this document is set in *this document's*. See own. A
// document that declared nothing still sets text in the caller's library, and
// the caller's library is shared.
func loadFontFaces(pending []pendingFontFace, res ResourceResolver, base FontSet, rec *Recorder) FontSet {
	set := &documentFonts{base: base, byFamily: map[string][]*documentFace{}, inst: newInstancer()}
	if len(pending) == 0 {
		return wrapDocumentFonts(set)
	}
	l := &fontFaceLoader{
		res: res, rec: rec, base: base, set: set,
		loaded: map[string]*shape.Face{},
		failed: map[string]bool{},
		budget: maxDocumentFontBytes,
	}
	for _, p := range pending {
		if l.rules >= maxFontFaceRules {
			l.overRuleCap(p, len(pending))
			break
		}
		l.rules++
		rule, ok := l.parse(p)
		if !ok {
			continue
		}
		face, ref, ok := l.face(p, rule)
		if !ok {
			continue
		}
		face = l.withFeatureSettings(p, rule, face)
		df := &documentFace{rule: rule, match: rule.matchable(), face: face, ref: ref,
			named: l.namedInstance(p, rule, face)}
		set.faces = append(set.faces, df)
		key := familyKey(rule.family)
		set.byFamily[key] = append(set.byFamily[key], df)
	}
	return wrapDocumentFonts(set)
}

// wrapDocumentFonts keeps the FallbackFontSet interface where the base had one
// and does not invent it where it did not. See fallbackDocumentFonts.
func wrapDocumentFonts(set *documentFonts) FontSet {
	if fallbackLookup(set.base) != nil {
		return fallbackDocumentFonts{set}
	}
	return set
}

// fontFaceLoader loads the faces, under the caps.
type fontFaceLoader struct {
	res  ResourceResolver
	rec  *Recorder
	base FontSet
	// set is the document's set, which is where a face taken from the caller's
	// library is turned into one of this document's. See documentFonts.own.
	set *documentFonts

	// loaded memoizes by reference, so a document naming one file in four
	// @font-face rules reads and parses it once. Sharing a face between two
	// families within one document is right: a face records the glyphs it was
	// asked to show, and that record is per document.
	loaded map[string]*shape.Face
	// failed records the references already reported, so a stylesheet with
	// twenty rules pointing at one missing file makes one attempt.
	failed map[string]bool
	// featured is the copies of loaded faces that font-feature-settings
	// descriptors asked for. See withFeatureSettings.
	featured map[featuredFace]*shape.Face

	// budget is how many bytes of font program the document may still read.
	budget int
	// faces counts the faces loaded, and rules the @font-face rules looked at.
	faces int
	rules int

	cappedFaces, cappedBytes, cappedRules bool
}

// at is the place a finding about one rule points to.
func (p pendingFontFace) at() Source {
	return Source{HTMLOffset: -1, CSSOffset: p.rule.Offset, Sheet: p.sheet}
}

// parse reads the descriptors of one @font-face rule.
//
// A rule missing either of the two descriptors that make it a font — the family
// it is called and the file it comes from — defines nothing, and that is
// reported as malformed CSS rather than as an unsupported feature: the author
// wrote a rule that cannot mean anything, which is a different thing from this
// engine not doing something.
func (l *fontFaceLoader) parse(p pendingFontFace) (fontFaceRule, bool) {
	out := fontFaceRule{weight: faceRange{auto: true}, width: faceRange{auto: true}}
	if !p.rule.HasBlock {
		l.rec.ReportDetail(Finding{
			Rule:     RuleInvalidCSS,
			Source:   p.at(),
			Message:  "@font-face has no block, so it declares no font",
			Property: "@font-face",
		})
		return out, false
	}
	decls, _, errs := css.ParseDeclarationValues(p.rule.Block)
	for _, e := range errs {
		l.rec.ReportDetail(Finding{
			Rule:    RuleInvalidCSS,
			Source:  Source{HTMLOffset: -1, CSSOffset: e.Offset, Sheet: p.sheet},
			Message: e.Message,
		})
	}

	for _, d := range decls {
		switch ascii.Lower(d.Name) {
		case "font-family":
			out.family = descriptorFamily(d.Value)
		case "src":
			out.srcs = l.parseSrc(p, d)
		case "font-weight":
			if r, ok := parseWeightDescriptor(d.Value); ok {
				out.weight = r
			} else {
				l.badDescriptor(p, d, "font-weight")
			}
		case "font-width", "font-stretch":
			// font-stretch is font-width's legacy name, as a descriptor as it
			// is as a property (§4.4.1): one descriptor, the later declaration
			// of it winning.
			if r, ok := parseWidthDescriptor(d.Value); ok {
				out.width = r
			} else {
				l.badDescriptor(p, d, ascii.Lower(d.Name))
			}
		case "font-style":
			if st, ok := parseStyleDescriptor(d.Value); ok {
				out.style = st
			} else {
				l.badDescriptor(p, d, "font-style")
			}
		case "unicode-range":
			out.ranges = l.unicodeRange(p, d)
		case "font-feature-settings":
			// A declaration that cannot be read is dropped and the one before
			// it stands, as it does for the two descriptors above.
			if settings, ok := l.featureSettings(p, d); ok {
				out.features, out.featuresAt = settings, d.Offset
			}
		case "font-variation-settings":
			// §7.2's sixth step, between the variations font-weight, font-width
			// and font-style ask for and the property's. Applied where the face
			// is set, in that order — see fontinstance.go.
			if settings, ok := l.variationSettings(p, d); ok {
				out.variations = settings
			}
		case "font-named-instance":
			if name, ok := parseNamedInstance(d.Value); ok {
				out.namedInstance = name
			} else {
				l.badDescriptor(p, d, "font-named-instance")
			}
		case "font-display":
			// A hint about what to show while a font is downloading. There is
			// no download here and no moment at which a page is half-drawn, so
			// there is nothing for it to change and nothing to report.
		default:
			// Every other descriptor changes how the face is used —
			// size-adjust and the override descriptors change its metrics
			// outright. Ignoring one silently would move the text on the page
			// with nothing saying so.
			l.rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedProperty,
				Source:   Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
				Message:  "the @font-face descriptor " + quoteValue(d.Name) + " is not applied",
				Property: ascii.Lower(d.Name),
			})
		}
	}

	if out.family == "" {
		l.rec.ReportDetail(Finding{
			Rule:     RuleInvalidCSS,
			Source:   p.at(),
			Message:  "@font-face names no font-family, so nothing can ask for it",
			Property: "@font-face",
		})
		return out, false
	}
	if len(out.srcs) == 0 {
		l.rec.ReportDetail(Finding{
			Rule:   RuleInvalidCSS,
			Source: p.at(),
			Message: "@font-face for " + quoteValue(out.family) +
				" names no usable src, so there is no font to load",
			Property: "@font-face",
		})
		return out, false
	}
	return out, true
}

// featureSettings reads the font-feature-settings descriptor, whose grammar is
// the property's (CSS Fonts 4 §4.6) and is judged by the same code the cascade
// judges the property with.
//
// ok is false where the value is not one this engine can read — not CSS, or CSS
// holding something it does not evaluate — and each is reported as such, the
// rule's earlier value standing. "normal" is read, as asking for nothing.
func (l *fontFaceLoader) featureSettings(p pendingFontFace, d css.Declaration) ([]shape.FeatureSetting, bool) {
	valid, unsupported := style.JudgeValue("font-feature-settings", d.Value)
	switch {
	case !valid:
		l.badDescriptor(p, d, "font-feature-settings")
		return nil, false
	case unsupported != "":
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
			Message: "the @font-face descriptor \"font-feature-settings\" uses " + unsupported +
				", which this engine does not evaluate; the face was loaded with no settings of its own",
			Property: "font-feature-settings",
		})
		return nil, false
	}
	settings, unusable := featureSettingsIn(d.Value)
	if len(unusable) > 0 {
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
			Message: "the @font-face descriptor \"font-feature-settings\" names " +
				quoteTags(unusable) + ", which cannot name a feature here; the rest of it was applied",
			Property: "font-feature-settings",
		})
	}
	return settings, true
}

// withFeatureSettings is the face a rule's font-feature-settings asks for: the
// face, with the settings stated on it, so that every run set in it — measured
// here or shaped again by a backend — is shaped with them. See
// shape.Face.WithFeatureSettings.
//
// One per face and settings. A file several rules name is loaded once and
// shared (see load), and the rules that state the same settings share its
// copy; a rule that states none keeps the face as it was loaded.
//
// A feature the settings turn on that the face has nothing under is reported
// here, where the face and the rule are both known, for the reason reportKerning
// reports one the property asks for: the text is set in the letters it was
// written with, which is not the page the rule asked for.
func (l *fontFaceLoader) withFeatureSettings(p pendingFontFace, r fontFaceRule, face *shape.Face) *shape.Face {
	if len(r.features) == 0 {
		return face
	}
	on, off := settleFeatureSettings(r.features)
	if on == "" && len(off) == 0 {
		return face
	}
	key := featuredFace{face: face, on: on, off: strings.Join(off, ",")}
	if got := l.featured[key]; got != nil {
		return got
	}
	out := face.WithFeatureSettings(r.features)
	if l.featured == nil {
		l.featured = map[featuredFace]*shape.Face{}
	}
	l.featured[key] = out

	var lacking []string
	if on != "" {
		for _, tag := range strings.Split(on, ",") {
			if tag == "kern" && face.HasKerning() || faceDeclares(face, tag) {
				continue
			}
			lacking = append(lacking, tag)
		}
	}
	if len(lacking) > 0 {
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: Source{HTMLOffset: -1, CSSOffset: r.featuresAt, Sheet: p.sheet},
			Message: "the @font-face for " + quoteValue(r.family) + " asks for " + quoteTags(lacking) +
				", which its face does not declare; its text is set in the letters it was written with",
			Property: "font-feature-settings",
		})
	}
	return out
}

// variationSettings reads the font-variation-settings descriptor, whose grammar
// is the property's and is judged by the same code; ok is false, and the
// rule's earlier value stands, where it cannot be read. A list longer than
// maxVariationSettings keeps its last entries and says so.
func (l *fontFaceLoader) variationSettings(p pendingFontFace, d css.Declaration) ([]variationSetting, bool) {
	valid, unsupported := style.JudgeValue("font-variation-settings", d.Value)
	switch {
	case !valid:
		l.badDescriptor(p, d, "font-variation-settings")
		return nil, false
	case unsupported != "":
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
			Message: "the @font-face descriptor \"font-variation-settings\" uses " + unsupported +
				", which this engine does not evaluate; the face was given no settings of its own",
			Property: "font-variation-settings",
		})
		return nil, false
	}
	settings, over := variationSettingsIn(d.Value)
	if over > 0 {
		l.rec.ReportDetail(Finding{
			Rule:   RuleLimit,
			Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
			Message: fmt.Sprintf("the @font-face descriptor \"font-variation-settings\" lists %d settings, "+
				"more than the %d this engine reads; the first %d were not applied",
				len(settings)+over, maxVariationSettings, over),
			Property: "font-variation-settings",
		})
	}
	return settings, true
}

// namedInstance finds the rule's font-named-instance in its face, by §5.1's
// localized name matching — familyKey, over every spelling the font gives the
// name. A name the face does not have applies nothing (§7.2) and is reported,
// since the author asked for an instance and the text is not set at it.
func (l *fontFaceLoader) namedInstance(p pendingFontFace, r fontFaceRule, face *shape.Face) map[string]float64 {
	if r.namedInstance == "" {
		return nil
	}
	want := familyKey(r.namedInstance)
	coords, ok := face.NamedInstance(func(name string) bool { return familyKey(name) == want })
	if !ok {
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: p.at(),
			Message: "the @font-face for " + quoteValue(r.family) + " names the instance " +
				quoteValue(r.namedInstance) + ", which its face does not have; no named instance was applied",
			Property: "font-named-instance",
		})
		return nil
	}
	return coords
}

// parseNamedInstance reads the font-named-instance descriptor: auto, which is
// no instance and the empty string, or a string naming one.
func parseNamedInstance(vals []css.ComponentValue) (string, bool) {
	toks, ok := descriptorTokens(vals)
	if !ok || len(toks) != 1 {
		return "", false
	}
	switch t := toks[0]; {
	case t.Kind == css.Ident && ascii.EqualFold(t.Value, "auto"):
		return "", true
	case t.Kind == css.String:
		return t.Value, true
	}
	return "", false
}

// featuredFace is what withFeatureSettings keeps a face's copy under.
type featuredFace struct {
	face    *shape.Face
	on, off string
}

// quoteTags renders feature tags for a message.
func quoteTags(tags []string) string {
	quoted := make([]string, len(tags))
	for i, t := range tags {
		quoted[i] = quoteValue(t)
	}
	return strings.Join(quoted, ", ")
}

func (l *fontFaceLoader) badDescriptor(p pendingFontFace, d css.Declaration, name string) {
	l.rec.ReportDetail(Finding{
		Rule:     RuleInvalidCSS,
		Source:   Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
		Message:  "the @font-face descriptor " + quoteValue(name) + " is not a value this engine can read; the default was used",
		Property: name,
	})
}

// unicodeRange reads the descriptor, which says which characters the face is for.
//
// It is honoured: a face restricted to a range is used for the characters in it
// and passed over for the rest, which fall to the next family the document
// named. That is what makes a document declaring one webfont for Latin and
// another for Greek get both, and it is the ordinary way a page with a large
// script is served.
//
// It was not always. The descriptor was parsed and reported, on the reasoning
// that the engine chose one face per box and cutting a run into per-face pieces
// was a change through measurement, line breaking and the content stream. That
// change happened for a different reason — a box of English with one Hebrew
// letter in it, see facerun.go — and once the runs could be cut, the obstacle
// this stood behind was gone. The comment outlived it by several months, which
// is the ordinary fate of a note saying why something cannot be done.
//
// A range covering the whole of Unicode restricts nothing and is dropped here,
// so that the common case carries no list to walk and asks no questions.
func (l *fontFaceLoader) unicodeRange(p pendingFontFace, d css.Declaration) []unicodeSpan {
	ranges, ok := parseUnicodeRange(d.Value)
	if !ok {
		l.badDescriptor(p, d, "unicode-range")
		return nil
	}
	if coversAllOfUnicode(ranges) {
		return nil
	}
	return ranges
}

// parseSrc reads the src descriptor's list of alternatives.
//
// An entry this engine cannot read is dropped rather than invalidating the
// list, which is what CSS Fonts asks for: the list is a chain of alternatives
// written for readers with different capabilities, and one written for a reader
// this is not must not take the others down with it.
func (l *fontFaceLoader) parseSrc(p pendingFontFace, d css.Declaration) []fontSource {
	var out []fontSource
	for _, item := range splitOnComma(d.Value) {
		if len(out) >= maxFontSources {
			l.rec.ReportDetail(Finding{
				Rule:   RuleLimit,
				Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
				Message: fmt.Sprintf("this @font-face offers more than the %d src entries "+
					"this engine will try; the rest were not tried", maxFontSources),
				Property: "src",
			})
			break
		}
		if s, ok := parseSrcEntry(item); ok {
			out = append(out, s)
		}
	}
	return out
}

// parseSrcEntry reads one alternative of a src list.
func parseSrcEntry(vals []css.ComponentValue) (fontSource, bool) {
	var out fontSource
	have := false
	for _, v := range vals {
		switch {
		case v.IsToken() && v.Token.Kind == css.Whitespace:
			continue
		case v.IsToken() && v.Token.Kind == css.URL:
			if have {
				return fontSource{}, false
			}
			out = fontSource{ref: v.Token.Value}
			have = true
		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "url"):
			if have {
				return fontSource{}, false
			}
			ref, ok := singleString(v.Values)
			if !ok {
				return fontSource{}, false
			}
			out = fontSource{ref: ref}
			have = true
		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "local"):
			if have {
				return fontSource{}, false
			}
			name, ok := singleString(v.Values)
			if !ok {
				// local() with a bare unquoted name is legal and common:
				// local(Ahem). The name is the concatenation of the idents.
				name = ascii.TrimCSSSpace(descriptorFamily(v.Values))
				if name == "" {
					return fontSource{}, false
				}
			}
			out = fontSource{local: true, ref: name}
			have = true
		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "format"):
			if !have {
				return fontSource{}, false
			}
			f, ok := singleString(v.Values)
			if !ok {
				f = ascii.TrimCSSSpace(descriptorFamily(v.Values))
			}
			out.format = ascii.Lower(ascii.TrimCSSSpace(f))
		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "tech"):
			// A capability list — colour tables, variations, palettes. It
			// narrows when an entry may be used and never widens it, so an
			// engine that ignores it can only try a font it might have skipped,
			// and the try either parses or moves to the next entry.
			continue
		default:
			// Anything else in an entry makes it one this engine does not
			// understand, and the specification's own rule for a src entry it
			// cannot parse is to skip to the next.
			return fontSource{}, false
		}
	}
	return out, have
}

// singleString returns the one string a function's arguments consist of.
func singleString(vals []css.ComponentValue) (string, bool) {
	var got string
	found := false
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if !v.IsToken() || v.Token.Kind != css.String || found {
			return "", false
		}
		got, found = v.Token.Value, true
	}
	return got, found
}

// face loads the first src entry that yields one.
//
// The list is a fallback chain and is walked as one: an entry that cannot be
// loaded is not a failure of the rule, it is the next entry's turn. Only when
// none of them worked has the rule contributed nothing, and that is the single
// finding raised — one per rule rather than one per entry, because an author
// who wrote four alternatives expects three of them to be unused.
func (l *fontFaceLoader) face(p pendingFontFace, r fontFaceRule) (*shape.Face, string, bool) {
	var why []string
	for _, s := range r.srcs {
		if s.local {
			// local() names a *face*, not a family in a weight, so the
			// descriptors on the rule are what say how the face is to be
			// treated and are not asked for again here. The set is asked for
			// the upright regular of the name, which is the only thing a set
			// keyed by family can answer to a full face name.
			// Through the document's own copy, because a local() face is one
			// of the caller's and the caller's faces are shared. See own.
			if face, ok := l.set.own(l.base.Face(s.ref, false, false)); ok {
				return face, "local(" + s.ref + ")", true
			}
			// Not having a local face is the ordinary case and not a fault:
			// the whole point of writing local() first is that most readers
			// will not have it. It is the next entry's turn, silently.
			continue
		}
		face, fail := l.load(p, r, s)
		if face != nil {
			return face, s.ref, true
		}
		if fail != nil {
			why = append(why, fail.message)
		}
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleResourceBlocked,
		Source: p.at(),
		Message: "the @font-face for " + quoteValue(r.family) +
			" loaded no font, so the family is not available: " + joinReasons(why),
		Property: "@font-face",
	})
	return nil, "", false
}

// joinReasons renders why every alternative failed, or says that none was
// usable at all.
func joinReasons(why []string) string {
	if len(why) == 0 {
		return "none of its sources was one this engine could use"
	}
	return strings.Join(why, "; ")
}

// load fetches and parses one url() entry.
func (l *fontFaceLoader) load(p pendingFontFace, r fontFaceRule, s fontSource) (*shape.Face, *loadFailure) {
	if face, ok := l.loaded[s.ref]; ok {
		return face, nil
	}
	if l.failed[s.ref] {
		return nil, nil
	}
	if !usableFontFormat(s.format) {
		// A format this engine has no parser for. Saying so before the read is
		// not an optimisation, it is the difference between skipping an entry
		// the author expected most readers to skip and reporting a failure.
		return nil, &loadFailure{
			rule: RuleFontUndecodable,
			message: "the font at " + quoteValue(s.ref) + " is declared as " +
				quoteValue(s.format) + ", which this engine does not read",
		}
	}
	if l.faces >= maxDocumentFaces {
		if !l.cappedFaces {
			l.cappedFaces = true
			l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
				"this document loads more than the %d font faces this engine will parse; "+
					"the rest were not loaded", maxDocumentFaces))
		}
		return nil, &loadFailure{
			rule: RuleResourceBlocked,
			message: fmt.Sprintf("the font at %s was not loaded: this document already "+
				"used the %d faces this engine will parse", quoteValue(s.ref), maxDocumentFaces),
		}
	}

	data, fail := l.fetch(s.ref)
	if fail != nil {
		l.failed[s.ref] = true
		return nil, fail
	}
	if len(data) > maxFontBytes {
		l.failed[s.ref] = true
		return nil, &loadFailure{
			rule: RuleResourceBlocked,
			message: fmt.Sprintf("the font at %s is %d bytes, more than the %d this engine will parse",
				quoteValue(s.ref), len(data), maxFontBytes),
		}
	}
	if len(data) > l.budget {
		l.failed[s.ref] = true
		if !l.cappedBytes {
			l.cappedBytes = true
			l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
				"this document's fonts together need more than the %d bytes this engine "+
					"will parse for one document; the rest were not loaded", maxDocumentFontBytes))
		}
		return nil, &loadFailure{
			rule: RuleResourceBlocked,
			message: "the font at " + quoteValue(s.ref) +
				" was not loaded: the document's font budget was already spent",
		}
	}
	// Charged before the parse, because the read is what allocated and the
	// parse is what the budget exists to bound. A program that fails to parse
	// cost exactly as much as one that did not.
	l.budget -= len(data)

	face, err := shape.Load(data)
	if err != nil {
		l.failed[s.ref] = true
		return nil, &loadFailure{
			rule: RuleFontUndecodable,
			message: "the font at " + quoteValue(s.ref) +
				" is not one this engine can read: " + err.Error(),
		}
	}
	l.faces++
	l.loaded[s.ref] = face
	return face, nil
}

// fetch obtains the bytes of one font, applying resource.go's policy — the one
// every reference in a document is read through.
func (l *fontFaceLoader) fetch(ref string) ([]byte, *loadFailure) {
	if referenceText(ref) == "" {
		return nil, &loadFailure{
			rule:    RuleResourceBlocked,
			message: "an @font-face src names an empty reference",
		}
	}
	data, _, fail := fetchReference(l.res, ref, "font", "so it was not loaded", RuleFontUndecodable)
	if fail != nil {
		return nil, fail
	}
	if len(data) == 0 {
		return nil, &loadFailure{
			rule:    RuleFontUndecodable,
			message: "the font at " + quoteValue(ref) + " is empty",
		}
	}
	return data, nil
}

// overRuleCap reports the document-wide rule count tripping.
//
// Two findings, on the model of stylesheet.go's: the guard tripped, which every
// other part of forme reports as "limit", and the document is missing fonts it
// asked for, which is what makes the page wrong.
func (l *fontFaceLoader) overRuleCap(p pendingFontFace, total int) {
	if !l.cappedRules {
		l.cappedRules = true
		l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
			"this document declares %d @font-face rules, more than the %d this engine "+
				"will look at; the rest were not read", total, maxFontFaceRules))
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleResourceBlocked,
		Source: p.at(),
		Message: fmt.Sprintf("this @font-face was not read: the document already declared "+
			"the %d this engine will look at", maxFontFaceRules),
		Property: "@font-face",
	})
}

// usableFontFormat says whether a format() hint names something shape.Load can
// parse.
//
// An absent hint is usable: the file is tried and either parses or does not.
// The named ones are the two sfnt spellings and the two WOFF versions, all of
// which shape.Load unwraps into one. svg and embedded-opentype are not sfnt at
// all and there is nothing here that could read them.
//
// woff2 was on the other side of this list until the decoder for it existed,
// and stayed there after it did — which is the failure mode a hint has: nothing
// tried the bytes, so nothing found out they would have parsed. The suite is
// full of it. woff2 is what the web serves, so a document declaring one got a
// blocked-resource finding and a fallback face, and the reftests that supply a
// font precisely so that the shaping and the metrics are pinned were being
// compared against whatever face happened to be lying about.
//
// The hint is only a hint. A source with none is still tried, and one that says
// "woff" is still read by whether the bytes are a WOFF rather than by what the
// document claimed they are — what this decides is which sources in a src list
// are worth fetching at all.
func usableFontFormat(f string) bool {
	switch f {
	case "", "truetype", "opentype", "truetype-variations", "opentype-variations",
		"woff", "woff2":
		return true
	}
	return false
}

// descriptorFamily renders the font-family descriptor's value as a name.
//
// It is a string or a sequence of identifiers, and the two are the same name:
// `font-family: Times New Roman` and `font-family: "Times New Roman"` name one
// family. Anything else — a number, a function — is not a family name and
// yields nothing.
func descriptorFamily(vals []css.ComponentValue) string {
	var parts []string
	for _, v := range vals {
		if !v.IsToken() {
			return ""
		}
		switch v.Token.Kind {
		case css.Whitespace:
			continue
		case css.String:
			if len(parts) > 0 {
				return ""
			}
			parts = append(parts, v.Token.Value)
		case css.Ident:
			parts = append(parts, v.Token.Value)
		default:
			return ""
		}
	}
	return ascii.TrimCSSSpace(strings.Join(parts, " "))
}

// descriptorTokens is a descriptor's value without its white space, or false
// where it holds anything that is not a token.
func descriptorTokens(vals []css.ComponentValue) ([]css.Token, bool) {
	var out []css.Token
	for _, v := range vals {
		if !v.IsToken() {
			return nil, false
		}
		if v.Token.Kind == css.Whitespace {
			continue
		}
		out = append(out, v.Token)
	}
	return out, true
}

// isAutoDescriptor reports whether a descriptor's value is the one word
// "auto", which each of the three range descriptors takes alone.
func isAutoDescriptor(toks []css.Token) bool {
	return len(toks) == 1 && toks[0].Kind == css.Ident && ascii.EqualFold(toks[0].Value, "auto")
}

// rangeOf makes a range of one or two values, swapped where they are given
// the wrong way round: §4.4 has a user agent "swap the computed value of the
// startpoint and endpoint of the range in order to forbid decreasing ranges".
// They were refused, and the rule's face then selected as a normal one.
func rangeOf(nums []float64) (valueRange, bool) {
	switch len(nums) {
	case 1:
		return valueRange{nums[0], nums[0]}, true
	case 2:
		lo, hi := nums[0], nums[1]
		if lo > hi {
			lo, hi = hi, lo
		}
		return valueRange{lo, hi}, true
	}
	return valueRange{}, false
}

// parseWeightDescriptor reads the font-weight descriptor: auto, or one or two
// of normal, bold and a number from 1 to 1000. "bolder" and "lighter" are
// relative to an inherited value and mean nothing here.
func parseWeightDescriptor(vals []css.ComponentValue) (faceRange, bool) {
	toks, ok := descriptorTokens(vals)
	if !ok {
		return faceRange{}, false
	}
	if isAutoDescriptor(toks) {
		return faceRange{auto: true}, true
	}
	var nums []float64
	for _, t := range toks {
		switch {
		case t.Kind == css.Ident && ascii.EqualFold(t.Value, "normal"):
			nums = append(nums, 400)
		case t.Kind == css.Ident && ascii.EqualFold(t.Value, "bold"):
			nums = append(nums, 700)
		case t.Kind == css.Number && t.Number >= 1 && t.Number <= 1000:
			nums = append(nums, t.Number)
		default:
			return faceRange{}, false
		}
	}
	r, ok := rangeOf(nums)
	return faceRange{valueRange: r}, ok
}

// parseWidthDescriptor reads the font-width descriptor, or its legacy name
// font-stretch: auto, or one or two of font-width's keywords and non-negative
// percentages.
func parseWidthDescriptor(vals []css.ComponentValue) (faceRange, bool) {
	toks, ok := descriptorTokens(vals)
	if !ok {
		return faceRange{}, false
	}
	if isAutoDescriptor(toks) {
		return faceRange{auto: true}, true
	}
	var nums []float64
	for _, t := range toks {
		switch t.Kind {
		case css.Ident:
			w, known := fontWidthKeywords[ascii.Lower(t.Value)]
			if !known {
				return faceRange{}, false
			}
			nums = append(nums, w)
		case css.Percentage:
			if !(t.Number >= 0) || math.IsInf(t.Number, 0) {
				return faceRange{}, false
			}
			nums = append(nums, t.Number)
		default:
			return faceRange{}, false
		}
	}
	r, ok := rangeOf(nums)
	return faceRange{valueRange: r}, ok
}

// parseStyleDescriptor reads the font-style descriptor: auto, normal, italic,
// left, right, or oblique with one or two angles between -90 and 90 degrees,
// or none — which is 14 degrees, as it is for the property.
//
// An oblique face is an oblique face: it used to be read as italic whatever
// its angle, because there was no slnt axis to set and nothing to choose
// between. §5.2 distinguishes the two, and a rule declaring "oblique 0deg
// 20deg" for a variable face is declaring the angles its slnt axis is to be
// set at.
func parseStyleDescriptor(vals []css.ComponentValue) (faceStyle, bool) {
	toks, ok := descriptorTokens(vals)
	if !ok || len(toks) == 0 || toks[0].Kind != css.Ident {
		return faceStyle{}, false
	}
	kw := ascii.Lower(toks[0].Value)
	rest := toks[1:]
	switch kw {
	case "auto", "normal", "italic", "left", "right":
		if len(rest) != 0 {
			return faceStyle{}, false
		}
		switch kw {
		case "auto":
			return faceStyle{kind: styleAuto}, true
		case "normal":
			return faceStyle{kind: styleNormal}, true
		}
		return faceStyle{kind: styleItalic}, true
	case "oblique":
		if len(rest) == 0 {
			return faceStyle{kind: styleOblique,
				angles: valueRange{defaultObliqueAngle, defaultObliqueAngle}}, true
		}
		var angles []float64
		for _, t := range rest {
			deg, ok := gradientAngle([]css.ComponentValue{{Token: t}})
			if !ok || deg < -90 || deg > 90 {
				return faceStyle{}, false
			}
			angles = append(angles, deg)
		}
		r, ok := rangeOf(angles)
		return faceStyle{kind: styleOblique, angles: r}, ok
	}
	return faceStyle{}, false
}

// unicodeSpan is one span of the unicode-range descriptor.
type unicodeSpan struct{ lo, hi rune }

// parseUnicodeRange reads the unicode-range descriptor.
//
// The descriptor's syntax predates CSS Syntax Level 3's token set, which has no
// unicode-range token: "U+0025-00FF" tokenizes as an identifier, a number and a
// dimension whose unit happens to be hexadecimal digits. So the text is
// reconstructed from the tokens — every one of which preserves what the author
// typed — and read as the grammar it is. Reading it off the token kinds instead
// would be reading it off an accident of where the tokenizer split it.
func parseUnicodeRange(vals []css.ComponentValue) ([]unicodeSpan, bool) {
	var out []unicodeSpan
	for _, item := range splitOnComma(vals) {
		text := ascii.TrimCSSSpace(rawText(item))
		if text == "" {
			return nil, false
		}
		span, ok := parseUnicodeSpan(text)
		if !ok {
			return nil, false
		}
		out = append(out, span)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// parseUnicodeSpan reads one of the three forms: a single code point, a
// wildcard, or a range.
func parseUnicodeSpan(text string) (unicodeSpan, bool) {
	if len(text) < 3 || (text[0] != 'u' && text[0] != 'U') || text[1] != '+' {
		return unicodeSpan{}, false
	}
	body := text[2:]
	if i := strings.IndexByte(body, '-'); i >= 0 {
		lo, ok1 := parseHex(body[:i])
		hi, ok2 := parseHex(body[i+1:])
		if !ok1 || !ok2 || lo > hi {
			return unicodeSpan{}, false
		}
		return unicodeSpan{lo, hi}, true
	}
	if q := strings.IndexByte(body, '?'); q >= 0 {
		// A wildcard: every "?" stands for one hexadecimal digit, and they must
		// all be at the end. "4??" is U+400 to U+4FF.
		if len(body) > 6 {
			return unicodeSpan{}, false
		}
		prefix := body[:q]
		for i := q; i < len(body); i++ {
			if body[i] != '?' {
				return unicodeSpan{}, false
			}
		}
		digits := len(body) - q
		lo, ok := parseHex(prefix + strings.Repeat("0", digits))
		hi, ok2 := parseHex(prefix + strings.Repeat("F", digits))
		if !ok || !ok2 {
			return unicodeSpan{}, false
		}
		return unicodeSpan{lo, hi}, true
	}
	v, ok := parseHex(body)
	if !ok {
		return unicodeSpan{}, false
	}
	return unicodeSpan{v, v}, true
}

// parseHex reads one to six hexadecimal digits as a code point.
func parseHex(s string) (rune, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	if n > 0x10FFFF {
		return 0, false
	}
	return rune(n), true
}

// coversAllOfUnicode reports whether the spans between them leave nothing out.
func coversAllOfUnicode(spans []unicodeSpan) bool {
	sorted := append([]unicodeSpan(nil), spans...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].lo < sorted[j].lo })
	next := rune(0)
	for _, s := range sorted {
		if s.lo > next {
			return false
		}
		if s.hi >= next {
			next = s.hi + 1
		}
	}
	return next > 0x10FFFF
}

// rawText renders component values back to the text the author wrote, as far as
// the tokens preserve it.
//
// It exists for unicode-range and for nothing else: every other descriptor here
// is read off the token kinds, which is the right way round. A number's Repr
// and a dimension's Unit are what the author typed, so "U+0025-00FF" comes back
// as itself.
func rawText(vals []css.ComponentValue) string {
	var b strings.Builder
	for _, v := range vals {
		if !v.IsToken() {
			// A function or a block in a unicode-range is not the grammar, and
			// rendering something for it would produce text that then failed to
			// parse for the wrong reason.
			return ""
		}
		t := v.Token
		switch t.Kind {
		case css.Ident, css.Delim:
			b.WriteString(t.Value)
		case css.Number:
			b.WriteString(t.Repr)
		case css.Dimension:
			b.WriteString(t.Repr + t.Unit)
		case css.Whitespace:
			b.WriteByte(' ')
		default:
			return ""
		}
	}
	return b.String()
}

// splitOnComma cuts a value list at its top-level commas.
func splitOnComma(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	start := 0
	for i, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Comma {
			out = append(out, vals[start:i])
			start = i + 1
		}
	}
	out = append(out, vals[start:])
	return out
}
