package layout

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Setting a variable face at the point of its design space a box asks for.
//
// A variable face loads at its default instance — the outlines its glyf table
// stores — and that is rarely the one wanted: Noto Sans JP's default is Thin,
// and the bundled Noto Sans's bold text was drawn at its Regular weight. CSS
// Fonts 4 §7.2 says which values place a face in its design space and in what
// order, each later step overriding an earlier one on the axes it sets:
//
//  2. font-weight, font-width and font-style, as 'wght', 'wdth' and 'slnt' or
//     'ital' — at most one of the last two — at the values §5.2's search came
//     to rest at, clamped to the @font-face rule's descriptors and then to
//     the face's own axes;
//  5. the rule's font-named-instance: every axis of the instance the font
//     names so;
//  6. the rule's font-variation-settings descriptor;
//  9. font-optical-sizing: auto, 'opsz' at the used font size in CSS pixels;
//  12. the font-variation-settings property.
//
// (The steps between are features and a language, not variations.) Every
// value is clamped to the face's axes, and a tag the face has no axis for is
// ignored, which is what the specification asks of each.
//
// The face that comes out is shape.LoadInstance's: a static font cut at the
// location, which is what is shaped — its advances, its kerning and anchors
// and its font-wide metrics are the location's — and what a backend draws,
// since a DrawText carries it as its Face and it carries its own program and
// its own PostScript name. Nothing new goes on the display list.
//
// # What it costs, and the bound
//
// Cutting an instance rewrites the font's outlines: 23 ms for the bundled Noto
// Sans and 155 ms for Noto Sans JP's 17,936 glyphs, and about as many
// megabytes of garbage as the font is long, several times over. So each
// (face, location) is cut once per document and kept, and a document may cut
// at most maxDocumentInstances of them, from at most maxDocumentInstanceBytes
// of font program between them. Past either the face is set at its default
// instance and the document is told so.
//
// # Synthesis
//
// A face that is not variable is used as it is: this engine synthesizes no
// bold and no oblique, and nothing here changes that.

// maxDocumentInstances bounds how many instances of variable faces one
// document may cut. A document needs a handful — a family at three weights and
// its italic — and a variable face whose optical size follows the font size
// needs one per size. A variable so a test can lower it.
var maxDocumentInstances = 64

// maxDocumentInstanceBytes bounds the font program one document's instances
// are cut from, summed over them: the work and the allocation instancing
// costs go with the length of the font, and sixty-four cuts of a
// ten-megabyte CJK face are not the sixty-four of a Latin one.
var maxDocumentInstanceBytes = 64 << 20

// maxVariationSettings bounds a font-variation-settings list. A face has at
// most shape's 64 axes and the fonts that exist have five; a list longer than
// this names some axis twice or names axes no face has. The last ones are the
// ones kept, since a later setting of a tag overrides an earlier one.
const maxVariationSettings = 64

// variationSetting is one entry of a font-variation-settings list.
type variationSetting struct {
	tag   string
	value float64
}

// variationSettingsIn reads a font-variation-settings value — the property's
// or the descriptor's; "normal" and anything unreadable are no settings. over
// is how many settings were left out for maxVariationSettings.
func variationSettingsIn(vals []css.ComponentValue) (settings []variationSetting, over int) {
	for _, part := range splitOnComma(vals) {
		items := nonWhitespace(part)
		if len(items) != 2 || !items[0].IsToken() || items[0].Token.Kind != css.String ||
			!items[1].IsToken() || items[1].Token.Kind != css.Number {
			continue
		}
		tag, v := items[0].Token.Value, items[1].Token.Number
		if !featureTagIsCSS(tag) || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		settings = append(settings, variationSetting{tag: tag, value: v})
	}
	if n := len(settings); n > maxVariationSettings {
		return settings[n-maxVariationSettings:], n - maxVariationSettings
	}
	return settings, 0
}

// variationAsk is everything besides the face that decides where it is set:
// the request, the @font-face rule it came from and where §5.2 came to rest
// (df nil for a caller's face), and the three properties of §7.2's later steps.
type variationAsk struct {
	r        FontRequest
	df       *documentFace
	match    faceMatch
	size     float64 // the used font size, in CSS pixels
	optical  bool    // font-optical-sizing: auto
	settings []variationSetting
}

// instanceCoords is where §7.2 places a variable face, by axis tag, in the
// user coordinates shape.LoadInstance takes: every axis the steps set, each
// clamped to the face's range. See the note at the top of this file.
func instanceCoords(face *shape.Face, ask variationAsk) map[string]float64 {
	axes := map[string]shape.Axis{}
	for _, a := range face.Axes() {
		axes[a.Tag] = a
	}
	coords := map[string]float64{}
	set := func(tag string, v float64) {
		if a, ok := axes[tag]; ok {
			v = math.Max(a.Min, math.Min(a.Max, v))
			if v == 0 {
				v = 0 // not -0, which a negated slant is and which spells another key
			}
			coords[tag] = v
		}
	}
	var rule *fontFaceRule
	if ask.df != nil {
		rule = &ask.df.rule
	}

	// 2. The weight and the width, clamped to the rule's descriptors where it
	// states them (auto clamps nothing, §4.4), and then to the face.
	weight, width := ask.r.Weight, ask.r.Width
	if rule != nil && !rule.weight.auto {
		weight = math.Max(rule.weight.lo, math.Min(rule.weight.hi, weight))
	}
	if rule != nil && !rule.width.auto {
		width = math.Max(rule.width.lo, math.Min(rule.width.hi, width))
	}
	set("wght", weight)
	set("wdth", width)
	// And the style: the value §5.2 came to rest at, which is one the rule's
	// descriptor offers, where the rule states one; where it does not — auto,
	// or a face of the caller's — the search over what the face itself offers:
	// its 'ital' axis as italic values, and its 'slnt' axis as oblique angles,
	// whose sign is the opposite of CSS's (§2.4). Only one of the two is set.
	var slope slopeMatch
	if rule != nil && rule.style.kind != styleAuto {
		slope = ask.match.slope
	} else {
		slope, _ = searchSlope(ask.r, []slopeOffer{ownSlopeOffer(axes)})
	}
	if slope.oblique {
		set("slnt", -slope.value)
	} else {
		set("ital", slope.value)
	}

	if rule != nil {
		// 5. The named instance, every axis of it, found when the face loaded.
		for tag, v := range ask.df.named {
			set(tag, v)
		}
		// 6. The descriptor's settings.
		for _, s := range rule.variations {
			set(s.tag, s.value)
		}
	}
	// 9. The optical size, from the used font size.
	if ask.optical && ask.size > 0 {
		set("opsz", ask.size)
	}
	// 12. The property's settings.
	for _, s := range ask.settings {
		set(s.tag, s.value)
	}
	return coords
}

// ownSlopeOffer is what a variable face offers of font-style by its own axes:
// its 'ital' range as italic values and its 'slnt' range, negated, as oblique
// angles; upright, where it has neither.
func ownSlopeOffer(axes map[string]shape.Axis) slopeOffer {
	o := slopeOffer{italic: []valueRange{{0, 0}}, oblique: []valueRange{{0, 0}}}
	if a, ok := axes["ital"]; ok {
		o.italic = []valueRange{{a.Min, a.Max}}
	}
	if a, ok := axes["slnt"]; ok {
		o.oblique = []valueRange{{-a.Max, -a.Min}}
	}
	return o
}

// atDefault reports whether a location is the face's default instance: every
// axis it sets at its default. That face is the one it already has.
func atDefault(face *shape.Face, coords map[string]float64) bool {
	for _, a := range face.Axes() {
		if v, ok := coords[a.Tag]; ok && v != a.Default {
			return false
		}
	}
	return true
}

// instancer is one document's instances of variable faces, and its budget.
//
// It is the document's and not the font set's: an instance records the glyphs
// the document set in it, as every face does, which is what the document's
// subset is made from. It is shared by the cascade — which asks a face for an
// "ex" — and by layout, so that the two measure by the same instance.
type instancer struct {
	mu     sync.Mutex
	made   map[instanceKey]*shape.Face
	failed map[instanceKey]bool
	// count and bytes are what the document has spent: instances cut, and
	// the length of the font programs they were cut from.
	count, bytes int
	// capped and refused record what has been reported, so that each is said
	// once and not once per box.
	capped  bool
	refused map[*shape.Face]bool
}

// instanceKey is a face and a location, spelled in axis order.
type instanceKey struct {
	face   *shape.Face
	coords string
}

func newInstancer() *instancer {
	return &instancer{made: map[instanceKey]*shape.Face{}, failed: map[instanceKey]bool{},
		refused: map[*shape.Face]bool{}}
}

// instanced is the face a box's text is set in: face itself where it is not
// variable or the box asks for its default instance, and otherwise its
// instance at the location §7.2 arrives at — cut once per document. src is
// what a finding points at.
func (in *instancer) instanced(face *shape.Face, ask variationAsk, rec *Recorder, src Source) *shape.Face {
	if in == nil || face == nil || !face.IsVariable() {
		return face
	}
	coords := instanceCoords(face, ask)
	if atDefault(face, coords) {
		return face
	}
	key := instanceKey{face: face, coords: spellCoords(face, coords)}

	in.mu.Lock()
	defer in.mu.Unlock()
	if got := in.made[key]; got != nil {
		return got
	}
	if in.failed[key] {
		return face
	}
	if face.IsSimple() {
		// A face embedded as a simple font is addressed by character code, and
		// an instance of it would be a composite one: a different font to the
		// backend. It is the caller's choice of face, and is used as chosen.
		in.refuse(face, rec, src, "it is embedded as a simple font, which is not instanced")
		in.failed[key] = true
		return face
	}
	program := face.Program()
	if in.count >= maxDocumentInstances || in.bytes+len(program) > maxDocumentInstanceBytes {
		in.failed[key] = true
		if !in.capped {
			in.capped = true
			rec.ReportDetail(Finding{
				Rule:   RuleLimit,
				Source: src,
				Message: fmt.Sprintf("this document asks for more than the %d instances of variable faces "+
					"(or the %d bytes of font program to cut them from) this engine will make; "+
					"%q and the faces after it are set at their default instances where no instance was made",
					maxDocumentInstances, maxDocumentInstanceBytes, face.Name()),
				Property: "font-variation-settings",
			})
		}
		return face
	}
	in.count++
	in.bytes += len(program)
	inst, err := shape.LoadInstance(program, coords)
	if err != nil {
		in.failed[key] = true
		in.refuse(face, rec, src, err.Error())
		return face
	}
	if settings := face.FeatureSettings(); len(settings) > 0 {
		// An @font-face rule's font-feature-settings are on the face it
		// loaded (withFeatureSettings), and are the instance's too.
		inst = inst.WithFeatureSettings(settings)
	}
	in.made[key] = inst
	return inst
}

// refuse reports, once per face, that a variable face could not be set where
// a document asked.
func (in *instancer) refuse(face *shape.Face, rec *Recorder, src Source, why string) {
	if in.refused[face] {
		return
	}
	in.refused[face] = true
	rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedValue,
		Source: src,
		Message: "the variable face " + quoteValue(face.Name()) +
			" is set at its default instance, not where its weight, width, style or variations ask: " + why,
		Property: "font-variation-settings",
	})
}

// spellCoords is a location as a key: every axis the face has, in its order,
// at the value set or its default.
func spellCoords(face *shape.Face, coords map[string]float64) string {
	var b strings.Builder
	for _, a := range face.Axes() {
		v, ok := coords[a.Tag]
		if !ok {
			v = a.Default
		}
		b.WriteString(a.Tag)
		b.WriteByte('=')
		b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		b.WriteByte(';')
	}
	return b.String()
}

// instancerOf is the document's instancer where the set is the document's own
// (loadFontFaces makes one), and nil otherwise — a caller that lays out a box
// tree with a set of its own, through Layout, gets one of the layouter's.
func instancerOf(set FontSet) *instancer {
	if d := documentFontsOf(set); d != nil {
		return d.inst
	}
	return nil
}

// documentFontsOf is the document's own set, where set is it.
func documentFontsOf(set FontSet) *documentFonts {
	switch d := set.(type) {
	case *documentFonts:
		return d
	case fallbackDocumentFonts:
		return d.documentFonts
	}
	return nil
}

// variationAskOf reads the §7.2 inputs a computed style gives, besides the
// request: font-optical-sizing, font-variation-settings and the used size.
// over is how many settings maxVariationSettings left out.
func variationAskOf(cs style.ComputedStyle, r FontRequest, sizePx float64) (ask variationAsk, over int) {
	ask = variationAsk{r: r, size: sizePx}
	ask.optical = !ascii.EqualFold(ascii.TrimCSSSpace(cs.Get("font-optical-sizing")), "none")
	if v := cs.Get("font-variation-settings"); v != "" && !ascii.EqualFold(ascii.TrimCSSSpace(v), "normal") {
		vals, _ := css.ParseComponentValues(v)
		ask.settings, over = variationSettingsIn(vals)
	}
	return ask, over
}

// styledFace is the face a family gives a request, set where §7.2 places it:
// the one question every path that turns a family into a face asks. text
// narrows the family to the faces whose unicode-range covers it, as
// FaceForFamily does; empty asks FontSet's general question.
func styledFace(set FontSet, in *instancer, rec *Recorder, src Source, family, text string,
	ask variationAsk) (*shape.Face, bool) {

	if d := documentFontsOf(set); d != nil {
		df, m, defined := d.documentFaceFor(family, text, ask.r)
		if defined {
			if df == nil {
				return nil, false
			}
			ask.df, ask.match = df, m
			return in.instanced(df.face, ask, rec, src), true
		}
	}
	var face *shape.Face
	var ok bool
	if ranged := rangedLookup(set); ranged != nil && text != "" {
		face, ok = ranged(family, text, ask.r)
	} else {
		face, ok = faceIn(set, family, ask.r)
	}
	if !ok {
		return nil, false
	}
	return in.instanced(face, ask, rec, src), true
}
