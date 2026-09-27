package layout

import (
	"math"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// What a box asks a family for: the numbers CSS Fonts 4 computes.
//
// FontSet's Face takes two booleans, bold and italic, and that is the whole of
// what layout used to ask. CSS asks for more: a weight anywhere from 1 to 1000
// (§2.2), a width as a percentage (§2.3), and a style that is normal, italic or
// oblique at an angle (§2.4) — and §5.2 chooses between a family's faces by all
// three, in that order, against the ranges each @font-face rule declares (see
// fontmatch.go).
//
// The booleans stay: FontSet is implemented outside this module, and a set
// that has a regular and a bold face and nothing else is still answered
// correctly by them. FontRequest.Bold and FontRequest.Italic are what §5.2
// chooses from such a family. A set that can do better implements the Styled
// interfaces below and is asked the numbers.

// FontRequest is what CSS asks of a family: font-weight, font-width and
// font-style, computed.
type FontRequest struct {
	// Weight is font-weight: 1 to 1000, 400 for normal and 700 for bold.
	Weight float64
	// Width is font-width (font-stretch), a percentage of the face's normal
	// width: 100 is normal, 75 condensed, 125 expanded.
	Width float64
	// Slope is font-style's keyword, and Angle its oblique angle in degrees,
	// for SlopeOblique: CSS's sign, positive leaning to the right. "oblique"
	// with no angle is 14 degrees.
	Slope FontSlope
	Angle float64
}

// FontSlope is font-style's keyword.
type FontSlope int

const (
	// SlopeNormal is an upright face.
	SlopeNormal FontSlope = iota
	// SlopeItalic is an italic face, or an oblique one if the family has no
	// italic.
	SlopeItalic
	// SlopeOblique is an oblique face at FontRequest.Angle.
	SlopeOblique
)

// normalRequest is the request every property's initial value makes: weight 400,
// width 100%, upright.
var normalRequest = FontRequest{Weight: 400, Width: 100}

// Bold is the request as FontSet.Face's bold flag: what CSS Fonts 4 §5.2
// chooses from a family of a regular (400) face and a bold (700) one. Above
// 500 that is the bold face — at 550 the search looks at weights of 550 and up
// first — and at 500 and below the regular one.
//
// It is not 600, which is where a renderer starts to *synthesize* bold for a
// family that has none. Choosing between two faces that exist is §5.2's
// question, and it answers 501.
func (r FontRequest) Bold() bool { return r.Weight > 500 }

// Italic is the request as FontSet.Face's italic flag: what §5.2 chooses from
// a family of an upright face and an italic one. Italic takes the italic face,
// and so does an oblique leaning to the right at any angle, since the upright
// face's oblique angle is zero and a positive request looks only at positive
// angles before it looks at italics. "oblique 0deg" and an oblique leaning
// left take the upright one.
func (r FontRequest) Italic() bool {
	return r.Slope == SlopeItalic || r.Slope == SlopeOblique && r.Angle > 0
}

// StyledFontSet is a FontSet that can be asked for a face by the numbers CSS
// computes rather than by FontSet's two booleans. Layout asks it in place of
// Face wherever it would have asked Face.
type StyledFontSet interface {
	FontSet
	FaceStyled(family string, r FontRequest) (*shape.Face, bool)
}

// StyledRangedFontSet is RangedFontSet asked by the numbers: the face a family
// offers for a piece of text at a weight, width and style.
type StyledRangedFontSet interface {
	FaceForFamilyStyled(family, text string, r FontRequest) (*shape.Face, bool)
}

// StyledFallbackFontSet is FallbackFontSet asked by the numbers: a face that
// can set the whole of text, chosen with the weight, width and style the box
// asked for in mind. A set is as free to ignore them as FaceFor's booleans.
type StyledFallbackFontSet interface {
	FaceForStyled(text string, r FontRequest) (*shape.Face, bool)
}

// faceIn asks a set for a family, by the numbers where it takes them.
func faceIn(set FontSet, family string, r FontRequest) (*shape.Face, bool) {
	if s, ok := set.(StyledFontSet); ok {
		return s.FaceStyled(family, r)
	}
	return set.Face(family, r.Bold(), r.Italic())
}

// requestFromFlags is the request two booleans make: 700 or 400, italic or
// upright. It is what a caller of the boolean methods of this package's own
// sets is taken to mean.
func requestFromFlags(bold, italic bool) FontRequest {
	r := normalRequest
	if bold {
		r.Weight = 700
	}
	if italic {
		r.Slope = SlopeItalic
	}
	return r
}

// fontRequestOf reads a computed style's request. A value the cascade let
// through that this cannot read is the property's initial value — which is
// what it was before, read as a boolean: nothing the grammar admits falls
// there but math functions, which the cascade reports and drops.
func fontRequestOf(cs style.ComputedStyle) FontRequest {
	r := normalRequest
	if w, ok := parseFontWeight(cs.Get("font-weight")); ok {
		r.Weight = w
	}
	if w, ok := parseFontWidth(cs.Get("font-width")); ok {
		r.Width = w
	}
	r.Slope, r.Angle, _ = parseFontStyle(cs.Get("font-style"))
	return r
}

// parseFontWeight reads a computed font-weight. "bolder" and "lighter" have
// been resolved by the cascade (style's fontweight.go) and are not computed
// values; were one to reach here, it is read against the initial weight, which
// is what the cascade reads it against at the root.
func parseFontWeight(value string) (float64, bool) {
	switch v := ascii.Lower(ascii.TrimCSSSpace(value)); v {
	case "", "normal":
		return 400, true
	case "bold":
		return 700, true
	case "bolder":
		return 700, true
	case "lighter":
		return 100, true
	default:
		n, ok := cssNumber(v)
		if !ok || n < 1 || n > 1000 {
			return 0, false
		}
		return n, true
	}
}

// fontWidthKeywords are §2.3's eight keywords and normal, as percentages.
var fontWidthKeywords = map[string]float64{
	"ultra-condensed": 50, "extra-condensed": 62.5, "condensed": 75,
	"semi-condensed": 87.5, "normal": 100, "semi-expanded": 112.5,
	"expanded": 125, "extra-expanded": 150, "ultra-expanded": 200,
}

// parseFontWidth reads a computed font-width as a percentage.
func parseFontWidth(value string) (float64, bool) {
	v := ascii.Lower(ascii.TrimCSSSpace(value))
	if v == "" {
		return 100, true
	}
	if w, ok := fontWidthKeywords[v]; ok {
		return w, true
	}
	pct, ok := strings.CutSuffix(v, "%")
	if !ok {
		return 0, false
	}
	n, ok := cssNumber(pct)
	if !ok || n < 0 {
		return 0, false
	}
	return n, true
}

// defaultObliqueAngle is what "oblique" with no angle means: §2.4, "the lack
// of an <angle> represents 14deg".
const defaultObliqueAngle = 14

// parseFontStyle reads a computed font-style.
//
// "left" and "right" are §2.4's italic with a clockwise slant and with a
// counter-clockwise one. The first is what an italic face is — it leans right
// — and is read as italic. The second is read as italic too, and ok is false
// for it, because nothing here knows which way a face leans: a caller that can
// report says the direction was not honoured.
func parseFontStyle(value string) (slope FontSlope, angle float64, ok bool) {
	parts := ascii.CSSFields(ascii.Lower(value))
	if len(parts) == 0 {
		return SlopeNormal, 0, true
	}
	switch parts[0] {
	case "normal":
		return SlopeNormal, 0, len(parts) == 1
	case "italic", "left":
		return SlopeItalic, 0, len(parts) == 1
	case "right":
		return SlopeItalic, 0, false
	case "oblique":
		if len(parts) == 1 {
			return SlopeOblique, defaultObliqueAngle, true
		}
		if len(parts) != 2 {
			return SlopeNormal, 0, false
		}
		vals, _ := css.ParseComponentValues(parts[1])
		deg, good := gradientAngle(vals)
		if !good || deg < -90 || deg > 90 {
			return SlopeNormal, 0, false
		}
		return SlopeOblique, deg, true
	}
	return SlopeNormal, 0, false
}

// cssNumber reads a CSS number: digits, a point and an exponent, and not the
// "nan", "inf" and hexadecimal strconv also takes (see the note on Go number
// syntax in fontface.go's descriptors).
func cssNumber(s string) (float64, bool) {
	vals, _ := css.ParseComponentValues(s)
	if len(vals) != 1 || !vals[0].IsToken() || vals[0].Token.Kind != css.Number {
		return 0, false
	}
	n := vals[0].Token.Number
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// rangedLookup is how a set is asked what a family offers for a piece of text,
// through whichever of the two interfaces it implements, the numeric one first;
// nil for a set that implements neither.
func rangedLookup(set FontSet) func(family, text string, r FontRequest) (*shape.Face, bool) {
	if s, ok := set.(StyledRangedFontSet); ok {
		return s.FaceForFamilyStyled
	}
	if s, ok := set.(RangedFontSet); ok {
		return func(family, text string, r FontRequest) (*shape.Face, bool) {
			return s.FaceForFamily(family, text, r.Bold(), r.Italic())
		}
	}
	return nil
}

// fallbackLookup is rangedLookup for the fallback question.
func fallbackLookup(set FontSet) func(text string, r FontRequest) (*shape.Face, bool) {
	if s, ok := set.(StyledFallbackFontSet); ok {
		return s.FaceForStyled
	}
	if s, ok := set.(FallbackFontSet); ok {
		return func(text string, r FontRequest) (*shape.Face, bool) {
			return s.FaceFor(text, r.Bold(), r.Italic())
		}
	}
	return nil
}

// fontRequest is a box's request, with the one thing about it the request
// cannot carry reported: "font-style: right", an italic leaning left, which is
// read as italic because nothing here knows which way a face leans. Once per
// document, since the finding is about the value and not the box.
func (l *layouter) fontRequest(b *Box) FontRequest {
	r := fontRequestOf(b.Style)
	if _, _, ok := parseFontStyle(b.Style.Get("font-style")); !ok && !l.reportedOnce["font-style"] {
		l.reportedOnce["font-style"] = true
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: sourceOf(boxElement(b)),
			Message: "font-style " + quoteValue(b.Style.Get("font-style")) +
				" was read as italic: a face is chosen without regard to which way it leans",
			Property: "font-style",
		})
	}
	return r
}
