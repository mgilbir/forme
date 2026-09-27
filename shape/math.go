package shape

import (
	"errors"
	"fmt"
	"math"

	"github.com/mgilbir/forme/font"
)

// The OpenType MATH table: what a math font says about setting mathematics.
//
// A math font is an ordinary font with one more table, and that table is what
// makes it a math font. It has three parts, and MathML Core (§5) reads all of
// them:
//
//   - MathConstants, fifty-six numbers the layout of a formula is written in:
//     where the fraction bar sits (the math axis), how far a superscript is
//     raised, how thick a radical's overbar is.
//   - MathGlyphInfo, per glyph: how far its ink leans past its advance (the
//     italics correction), where an accent above it is centred, whether it is
//     an "extended shape", and the cut-ins a script may kern into.
//   - MathVariants, per glyph and per axis: larger versions of it — the
//     parenthesis a tall fraction needs — and, beyond the largest, how to
//     build one of any size out of parts: a top, a bottom and an extender
//     repeated between them.
//
// The table is read where it lies, a question at a time, rather than turned
// into maps when the face loads. A face is loaded for every document that
// names it and most documents never set a formula; and every question here is
// a coverage lookup and a record read, which is what HarfBuzz does too. Every
// read is bounded by the table: an offset or a count that points past the end
// answers "not stated", never a read out of range.
//
// The answers are the table's own numbers, in font units. The device tables a
// MathValueRecord may point at are not applied: a hinting device adjusts a
// value at one pixel size on a screen, which a page has no use for, and a
// variation device moves it across a variable font's design space, which this
// reader does not follow — so a variable math font is read at its default
// instance, and Limits says so where the table carries one. HarfBuzz at its
// defaults (no pixel size, the default instance) answers the same numbers,
// which is what testdata/harfbuzz/math.py holds this reader to.

// MathConstant names one of the MathConstants values, in the order the table
// stores them — which is also HarfBuzz's hb_ot_math_constant_t.
type MathConstant int

const (
	MathScriptPercentScaleDown MathConstant = iota
	MathScriptScriptPercentScaleDown
	MathDelimitedSubFormulaMinHeight
	MathDisplayOperatorMinHeight
	MathLeading
	MathAxisHeight
	MathAccentBaseHeight
	MathFlattenedAccentBaseHeight
	MathSubscriptShiftDown
	MathSubscriptTopMax
	MathSubscriptBaselineDropMin
	MathSuperscriptShiftUp
	MathSuperscriptShiftUpCramped
	MathSuperscriptBottomMin
	MathSuperscriptBaselineDropMax
	MathSubSuperscriptGapMin
	MathSuperscriptBottomMaxWithSubscript
	MathSpaceAfterScript
	MathUpperLimitGapMin
	MathUpperLimitBaselineRiseMin
	MathLowerLimitGapMin
	MathLowerLimitBaselineDropMin
	MathStackTopShiftUp
	MathStackTopDisplayStyleShiftUp
	MathStackBottomShiftDown
	MathStackBottomDisplayStyleShiftDown
	MathStackGapMin
	MathStackDisplayStyleGapMin
	MathStretchStackTopShiftUp
	MathStretchStackBottomShiftDown
	MathStretchStackGapAboveMin
	MathStretchStackGapBelowMin
	MathFractionNumeratorShiftUp
	MathFractionNumeratorDisplayStyleShiftUp
	MathFractionDenominatorShiftDown
	MathFractionDenominatorDisplayStyleShiftDown
	MathFractionNumeratorGapMin
	MathFractionNumDisplayStyleGapMin
	MathFractionRuleThickness
	MathFractionDenominatorGapMin
	MathFractionDenomDisplayStyleGapMin
	MathSkewedFractionHorizontalGap
	MathSkewedFractionVerticalGap
	MathOverbarVerticalGap
	MathOverbarRuleThickness
	MathOverbarExtraAscender
	MathUnderbarVerticalGap
	MathUnderbarRuleThickness
	MathUnderbarExtraDescender
	MathRadicalVerticalGap
	MathRadicalDisplayStyleVerticalGap
	MathRadicalRuleThickness
	MathRadicalExtraAscender
	MathRadicalKernBeforeDegree
	MathRadicalKernAfterDegree
	MathRadicalDegreeBottomRaisePercent

	// MathConstantCount is how many constants there are.
	MathConstantCount
)

// mathConstantNames are the constants as the OpenType specification spells
// them, which is also how fontTools and fonttest.MATH name them.
var mathConstantNames = [MathConstantCount]string{
	"ScriptPercentScaleDown", "ScriptScriptPercentScaleDown",
	"DelimitedSubFormulaMinHeight", "DisplayOperatorMinHeight",
	"MathLeading", "AxisHeight", "AccentBaseHeight", "FlattenedAccentBaseHeight",
	"SubscriptShiftDown", "SubscriptTopMax", "SubscriptBaselineDropMin",
	"SuperscriptShiftUp", "SuperscriptShiftUpCramped", "SuperscriptBottomMin",
	"SuperscriptBaselineDropMax", "SubSuperscriptGapMin",
	"SuperscriptBottomMaxWithSubscript", "SpaceAfterScript",
	"UpperLimitGapMin", "UpperLimitBaselineRiseMin", "LowerLimitGapMin",
	"LowerLimitBaselineDropMin", "StackTopShiftUp", "StackTopDisplayStyleShiftUp",
	"StackBottomShiftDown", "StackBottomDisplayStyleShiftDown", "StackGapMin",
	"StackDisplayStyleGapMin", "StretchStackTopShiftUp",
	"StretchStackBottomShiftDown", "StretchStackGapAboveMin",
	"StretchStackGapBelowMin", "FractionNumeratorShiftUp",
	"FractionNumeratorDisplayStyleShiftUp", "FractionDenominatorShiftDown",
	"FractionDenominatorDisplayStyleShiftDown", "FractionNumeratorGapMin",
	"FractionNumDisplayStyleGapMin", "FractionRuleThickness",
	"FractionDenominatorGapMin", "FractionDenomDisplayStyleGapMin",
	"SkewedFractionHorizontalGap", "SkewedFractionVerticalGap",
	"OverbarVerticalGap", "OverbarRuleThickness", "OverbarExtraAscender",
	"UnderbarVerticalGap", "UnderbarRuleThickness", "UnderbarExtraDescender",
	"RadicalVerticalGap", "RadicalDisplayStyleVerticalGap",
	"RadicalRuleThickness", "RadicalExtraAscender", "RadicalKernBeforeDegree",
	"RadicalKernAfterDegree", "RadicalDegreeBottomRaisePercent",
}

// String is the constant's name as the OpenType specification spells it.
func (c MathConstant) String() string {
	if c < 0 || c >= MathConstantCount {
		return fmt.Sprintf("MathConstant(%d)", int(c))
	}
	return mathConstantNames[c]
}

// The layout of MathConstants: two int16 percentages, two uint16 heights,
// fifty-one MathValueRecords of four bytes, and one int16 percentage — 214
// bytes in all.
const (
	mathConstantsSize  = 8 + 4*51 + 2
	mathValueRecordLen = 4
	mathGlyphPartLen   = 10
)

// The bounds on what a font may ask this reader to walk or to build.
//
// Each count in the table is sixteen bits and bounded by the bytes behind it,
// so none of them can make a read leave the table. What they can do is make
// work that multiplies: every stretchy operator in a document walks its
// glyph's variants, so a font listing sixty-five thousand of them would charge
// a document of a thousand operators sixty-five million steps. Real fonts list
// a handful — STIX Two Math's longest list is eleven, Noto Sans Math's twelve —
// and build an assembly from at most five parts. The bounds are far above
// that, and a font that reaches one is reported through Limits rather than
// trimmed in silence.
const (
	// maxMathVariants is how many size variants of one glyph are considered.
	maxMathVariants = 64
	// maxMathAssemblyParts is how many parts one assembly may have; a longer
	// one is refused, because an assembly with some of its parts missing is
	// not a smaller version of the glyph but a broken one.
	maxMathAssemblyParts = 32
	// MaxMathAssemblyGlyphs is how many glyphs one stretched assembly may be
	// drawn with: its parts, and its extenders repeated. The count is what a
	// target size costs — an extender repeated to cover a thousand-em
	// operator is thousands of glyphs to position and to draw — and the target
	// is the document's to choose. An assembly that would need more is built
	// with the most extenders that fit, which leaves it short of its target,
	// and MathStretch.Capped says so.
	MaxMathAssemblyGlyphs = 4096
)

// MathTable is a face's MATH table, read where it lies.
//
// It is nil where the face has none. The zero offsets below mean the table
// does not have that part, whether because it says so or because the offset
// it gave points outside it; Limits says which parts were refused, and why.
type MathTable struct {
	face *Face
	data []byte

	constants int // MathConstants, or 0

	italics, italicsCount int // MathItalicsCorrectionInfo and its record count
	accents, accentsCount int // MathTopAccentAttachment and its record count
	extended              int // ExtendedShapeCoverage
	kerns, kernsCount     int // MathKernInfo and its record count

	variants                        int // MathVariants
	minOverlap                      int
	vertCoverage, horizCoverage     int // absolute
	vertCount, horizCount           int
	constructions                   int // the offset array, absolute
	variationDevices                bool
	limits                          []string
	reportedVariants, reportedParts map[int]bool
}

// MathTable reads the face's MATH table. It returns nil and no error where the
// face has none — which is every face that is not a math font — and an error
// where it has one this reader cannot read at all: shorter than its header, or
// of a major version other than 1, whose layout this reader does not know.
//
// It reads the table directory each time it is asked, so a caller asking about
// many glyphs should keep the answer.
func (f *Face) MathTable() (*MathTable, error) {
	if f == nil || f.data == nil {
		return nil, nil
	}
	tables := font.SFNTTables(f.data)
	data, ok := tables["MATH"]
	if !ok {
		return nil, nil
	}
	if len(data) < 10 {
		return nil, errors.New("the font's MATH table is shorter than its header")
	}
	if major := font.Be16(data, 0); major != 1 {
		return nil, fmt.Errorf("the font's MATH table is version %d.%d, and this engine reads version 1",
			major, font.Be16(data, 2))
	}
	m := &MathTable{face: f, data: data}
	m.readConstants(font.Be16(data, 4))
	m.readGlyphInfo(font.Be16(data, 6))
	m.readVariants(font.Be16(data, 8))
	return m, nil
}

// refuse notes a part of the table that could not be read.
func (m *MathTable) refuse(what string) {
	m.limits = append(m.limits, "the font's MATH table has "+what)
}

// Limits is what reading the table could not do: the parts refused because
// their offsets or counts ran past the table, the lists longer than this
// reader walks, and the values that vary across a design space this reader
// does not follow. Empty for a well-formed table of a static font, which is
// every math font in the corpora.
func (m *MathTable) Limits() []string {
	if m == nil {
		return nil
	}
	out := append([]string(nil), m.limits...)
	if m.variationDevices {
		out = append(out, "the font's MATH table varies its values across the font's design "+
			"axes, and this engine reads them at the font's default instance")
	}
	return out
}

// sub is where a subtable at off from base begins, when it is inside the table
// and holds at least n bytes; 0 otherwise.
func (m *MathTable) sub(base, off, n int) int {
	if off == 0 {
		return 0
	}
	at := base + off
	if at <= 0 || at+n > len(m.data) {
		return -1
	}
	return at
}

func (m *MathTable) readConstants(off int) {
	switch at := m.sub(0, off, mathConstantsSize); at {
	case 0:
	case -1:
		m.refuse("a MathConstants offset that runs past the table, so none of its constants are read")
	default:
		m.constants = at
		for i := 0; i < 51; i++ {
			m.noteDevice(at, at+8+mathValueRecordLen*i)
		}
	}
}

func (m *MathTable) readGlyphInfo(off int) {
	at := m.sub(0, off, 8)
	switch at {
	case 0:
		return
	case -1:
		m.refuse("a MathGlyphInfo offset that runs past the table, so no glyph's italics correction, " +
			"accent attachment, shape or kerning is read")
		return
	}
	// Each subtable is a coverage offset, a count, and that many records; one
	// whose records run past the table is refused whole, as HarfBuzz's
	// sanitizer refuses it.
	counted := func(rel int, recLen int, what string) (int, int) {
		sub := m.sub(at, font.Be16(m.data, at+rel), 4)
		switch sub {
		case 0:
			return 0, 0
		case -1:
			m.refuse("a " + what + " offset that runs past the table, so it is not read")
			return 0, 0
		}
		n := font.Be16(m.data, sub+2)
		if sub+4+recLen*n > len(m.data) {
			m.refuse(fmt.Sprintf("a %s whose %d records run past the table, so it is not read", what, n))
			return 0, 0
		}
		return sub, n
	}
	m.italics, m.italicsCount = counted(0, mathValueRecordLen, "MathItalicsCorrectionInfo")
	m.accents, m.accentsCount = counted(2, mathValueRecordLen, "MathTopAccentAttachment")
	switch ext := m.sub(at, font.Be16(m.data, at+4), 4); ext {
	case 0:
	case -1:
		m.refuse("an ExtendedShapeCoverage offset that runs past the table, so it is not read")
	default:
		m.extended = ext
	}
	m.kerns, m.kernsCount = counted(6, 8, "MathKernInfo")
	for i := 0; i < m.italicsCount; i++ {
		m.noteDevice(m.italics, m.italics+4+mathValueRecordLen*i)
	}
	for i := 0; i < m.accentsCount; i++ {
		m.noteDevice(m.accents, m.accents+4+mathValueRecordLen*i)
	}
}

func (m *MathTable) readVariants(off int) {
	at := m.sub(0, off, 10)
	switch at {
	case 0:
		return
	case -1:
		m.refuse("a MathVariants offset that runs past the table, so no glyph is stretched")
		return
	}
	v, h := font.Be16(m.data, at+6), font.Be16(m.data, at+8)
	if at+10+2*(v+h) > len(m.data) {
		m.refuse(fmt.Sprintf("a MathVariants whose %d constructions run past the table, so no glyph is stretched", v+h))
		return
	}
	m.variants = at
	m.minOverlap = font.Be16(m.data, at)
	m.vertCoverage, m.horizCoverage = font.Be16(m.data, at+2), font.Be16(m.data, at+4)
	m.vertCount, m.horizCount = v, h
	m.constructions = at + 10
}

// noteDevice records whether the MathValueRecord at rec, whose device offset
// is from base, points at a VariationIndex table — deltaFormat 0x8000, the
// one kind of device table that moves a value across a variable font's
// design space. A hinting device (formats 1 to 3) is for one pixel size on a
// screen and changes nothing this engine draws.
func (m *MathTable) noteDevice(base, rec int) {
	if m.variationDevices {
		return
	}
	d := font.Be16(m.data, rec+2)
	if d == 0 {
		return
	}
	at := base + d
	if at+6 <= len(m.data) && font.Be16(m.data, at+4) == 0x8000 {
		m.variationDevices = true
	}
}

// value reads a MathValueRecord's value: its first two bytes, signed.
func (m *MathTable) value(rec int) int { return signed16(font.Be16(m.data, rec)) }

// Constant is one of the MathConstants, in font units — or, for the three
// percentages, as the percentage the font states. ok is false where the table
// has no MathConstants, which is what MathML Core means by a constant that is
// "not available" and the one case its fallbacks are for: a font that has the
// subtable states every constant in it, zero included.
func (m *MathTable) Constant(c MathConstant) (v int, ok bool) {
	if m == nil || m.constants == 0 || c < 0 || c >= MathConstantCount {
		return 0, false
	}
	at := m.constants
	switch {
	case c == MathScriptPercentScaleDown, c == MathScriptScriptPercentScaleDown:
		return signed16(font.Be16(m.data, at+2*int(c))), true
	case c == MathDelimitedSubFormulaMinHeight, c == MathDisplayOperatorMinHeight:
		return font.Be16(m.data, at+2*int(c)), true
	case c == MathRadicalDegreeBottomRaisePercent:
		return signed16(font.Be16(m.data, at+8+mathValueRecordLen*51)), true
	}
	return m.value(at + 8 + mathValueRecordLen*(int(c)-4)), true
}

// ItalicsCorrection is how far a glyph's ink leans past its advance, which is
// where a superscript attaches to it — and ok false where the font states none
// for the glyph, which MathML Core reads as zero.
func (m *MathTable) ItalicsCorrection(gid int) (int, bool) {
	if m == nil || m.italics == 0 {
		return 0, false
	}
	i, ok := m.coverage(m.italics, gid)
	return m.indexed(m.italics, m.italicsCount, i, ok)
}

// TopAccentAttachment is where along its advance an accent above the glyph is
// centred, and ok false where the font does not say — MathML Core then takes
// half the advance. (HarfBuzz's hb_ot_math_get_glyph_top_accent_attachment
// answers that half itself; this leaves it to the caller, which is the one
// that knows what the glyph is being measured for.)
func (m *MathTable) TopAccentAttachment(gid int) (int, bool) {
	if m == nil || m.accents == 0 {
		return 0, false
	}
	i, ok := m.coverage(m.accents, gid)
	return m.indexed(m.accents, m.accentsCount, i, ok)
}

// coverage is a glyph's index in the coverage table a subtable names in its
// first two bytes, relative to itself. A subtable naming none covers nothing.
func (m *MathTable) coverage(sub, gid int) (int, bool) {
	rel := font.Be16(m.data, sub)
	if rel == 0 {
		return 0, false
	}
	return coverageIndex(m.data, sub+rel, gid)
}

// indexed reads record i of a subtable whose records follow its four-byte
// header. A coverage index at or past the count is a malformed table: the
// coverage names a glyph the records do not reach. It is read as the glyph
// not being covered.
func (m *MathTable) indexed(sub, count, i int, covered bool) (int, bool) {
	if !covered || i < 0 || i >= count {
		return 0, false
	}
	return m.value(sub + 4 + mathValueRecordLen*i), true
}

// IsExtendedShape reports whether the font marks a glyph as an extended shape:
// one whose ink a script should be attached to by its height rather than by
// the ordinary rules, which is what a stretched operator is. MathML Core does
// not use it; it is read because the table states it.
func (m *MathTable) IsExtendedShape(gid int) bool {
	if m == nil || m.extended == 0 {
		return false
	}
	_, ok := coverageIndex(m.data, m.extended, gid)
	return ok
}

// MathKernCorner names one of the four corners a glyph states cut-ins at, in
// the order MathKernInfoRecord stores them — also HarfBuzz's.
type MathKernCorner int

const (
	MathKernTopRight MathKernCorner = iota
	MathKernTopLeft
	MathKernBottomRight
	MathKernBottomLeft
)

// Kern is the kerning at a corner of a glyph for a script whose edge is at
// height, in font units, and ok false where the font states none for that
// corner.
//
// The table states a step function: heights h[0] < h[1] < … < h[n-1] and kerns
// k[0] … k[n], and the kern at a height is k[i] where h[i-1] ≤ height < h[i]
// — so a height that is exactly one of the h[i] takes the step above it, as
// HarfBuzz reads it too. MathML Core itself asks for no kerning.
func (m *MathTable) Kern(gid int, corner MathKernCorner, height int) (int, bool) {
	if m == nil || m.kerns == 0 || corner < 0 || corner > MathKernBottomLeft {
		return 0, false
	}
	i, ok := m.coverage(m.kerns, gid)
	if !ok || i >= m.kernsCount {
		return 0, false
	}
	off := font.Be16(m.data, m.kerns+4+8*i+2*int(corner))
	at := m.sub(m.kerns, off, 2)
	if at <= 0 {
		return 0, false
	}
	n := font.Be16(m.data, at)
	if at+2+mathValueRecordLen*(2*n+1) > len(m.data) {
		return 0, false
	}
	heights := at + 2
	kerns := heights + mathValueRecordLen*n
	lo, count := 0, n
	for count > 0 {
		half := count / 2
		if m.value(heights+mathValueRecordLen*(lo+half)) <= height {
			lo += half + 1
			count -= half + 1
		} else {
			count = half
		}
	}
	return m.value(kerns + mathValueRecordLen*lo), true
}

// MinConnectorOverlap is how far two parts of an assembly must overlap at
// least, in font units: MathVariants.minConnectorOverlap, or 0 where the table
// has no MathVariants.
func (m *MathTable) MinConnectorOverlap() int {
	if m == nil {
		return 0
	}
	return m.minOverlap
}

// MathGlyphVariant is one size variant of a glyph: the glyph, and how far it
// reaches along the axis it is a variant on, in font units.
type MathGlyphVariant struct {
	Glyph, Advance int
}

// MathGlyphPart is one part of a glyph assembly, as the table states it: the
// glyph; how long its connectors are at its start and its end, which is how
// far it may overlap its neighbours; how far it reaches along the axis; and
// whether it is an extender, which may be repeated.
type MathGlyphPart struct {
	Glyph                        int
	StartConnector, EndConnector int
	FullAdvance                  int
	Extender                     bool
}

// MathGlyphAssembly is how to build a glyph of any size along one axis: its
// parts in order from the start of the axis — the bottom for a vertical
// assembly, the left for a horizontal one — and the italics correction of
// whatever is built from them.
type MathGlyphAssembly struct {
	ItalicsCorrection int
	Parts             []MathGlyphPart
}

// construction is where a glyph's MathGlyphConstruction for an axis begins,
// or 0 where the font has none.
func (m *MathTable) construction(gid int, vertical bool) int {
	if m == nil || m.variants == 0 {
		return 0
	}
	cov, count, first := m.horizCoverage, m.horizCount, m.vertCount
	if vertical {
		cov, count, first = m.vertCoverage, m.vertCount, 0
	}
	if cov == 0 {
		return 0
	}
	i, ok := coverageIndex(m.data, m.variants+cov, gid)
	if !ok || i >= count {
		return 0
	}
	at := m.sub(m.variants, font.Be16(m.data, m.constructions+2*(first+i)), 4)
	if at <= 0 {
		return 0
	}
	if at+4+4*font.Be16(m.data, at+2) > len(m.data) {
		return 0
	}
	return at
}

// HasConstruction reports whether the font says anything about stretching a
// glyph along an axis: size variants, an assembly, or both. MathML Core's
// stretching fails for a glyph it does not, and the operator is set as text.
func (m *MathTable) HasConstruction(gid int, vertical bool) bool {
	return m.construction(gid, vertical) != 0
}

// Variants is a glyph's size variants along an axis, in the table's order —
// which the specification asks to be increasing. At most maxMathVariants are
// returned, and Limits says so of a glyph that lists more.
func (m *MathTable) Variants(gid int, vertical bool) []MathGlyphVariant {
	at := m.construction(gid, vertical)
	if at == 0 {
		return nil
	}
	n := font.Be16(m.data, at+2)
	if n > maxMathVariants {
		m.noteOnce(&m.reportedVariants, gid, fmt.Sprintf(
			"%d size variants for glyph %d, and the first %d are considered", n, gid, maxMathVariants))
		n = maxMathVariants
	}
	out := make([]MathGlyphVariant, n)
	for i := range out {
		rec := at + 4 + 4*i
		out[i] = MathGlyphVariant{Glyph: font.Be16(m.data, rec), Advance: font.Be16(m.data, rec+2)}
	}
	return out
}

// Assembly is how to build a glyph of any size along an axis, and false where
// the font gives none — or gives one this reader refuses: one whose parts run
// past the table, or one of more than maxMathAssemblyParts parts, which Limits
// reports.
func (m *MathTable) Assembly(gid int, vertical bool) (MathGlyphAssembly, bool) {
	at := m.construction(gid, vertical)
	if at == 0 {
		return MathGlyphAssembly{}, false
	}
	a := m.sub(at, font.Be16(m.data, at), 6)
	if a <= 0 {
		return MathGlyphAssembly{}, false
	}
	n := font.Be16(m.data, a+4)
	if a+6+mathGlyphPartLen*n > len(m.data) {
		return MathGlyphAssembly{}, false
	}
	if n > maxMathAssemblyParts {
		m.noteOnce(&m.reportedParts, gid, fmt.Sprintf(
			"an assembly of %d parts for glyph %d, more than the %d this engine builds from, so the glyph is not assembled",
			n, gid, maxMathAssemblyParts))
		return MathGlyphAssembly{}, false
	}
	m.noteDevice(a, a)
	out := MathGlyphAssembly{ItalicsCorrection: m.value(a), Parts: make([]MathGlyphPart, n)}
	for i := range out.Parts {
		rec := a + 6 + mathGlyphPartLen*i
		out.Parts[i] = MathGlyphPart{
			Glyph:          font.Be16(m.data, rec),
			StartConnector: font.Be16(m.data, rec+2),
			EndConnector:   font.Be16(m.data, rec+4),
			FullAdvance:    font.Be16(m.data, rec+6),
			Extender:       font.Be16(m.data, rec+8)&1 != 0,
		}
	}
	return out, true
}

func (m *MathTable) noteOnce(seen *map[int]bool, gid int, what string) {
	if *seen == nil {
		*seen = map[int]bool{}
	}
	if (*seen)[gid] {
		return
	}
	(*seen)[gid] = true
	m.refuse(what)
}

// MathStretch is a glyph stretched to a size along one axis, by MathML Core
// §5.3.2's algorithm: the glyph itself, one of its size variants, or an
// assembly of its parts.
//
// Everything is in font units, and y is up, as the font measures it. The
// box is where the construction's origin is: the left end of its baseline.
type MathStretch struct {
	// Glyph is the glyph drawn when the answer is one glyph, and Parts is empty.
	Glyph int
	// Parts is the assembly, each part's glyph with where its origin goes,
	// when the answer is an assembly.
	Parts []MathStretchPart

	// Width is the construction's advance, and Ascent and Descent how far its
	// ink reaches above and below its baseline — for one glyph, the glyph's
	// advance and ink; for an assembly, what MathML Core §5.3.1 calls the
	// glyph assembly width, ascent and descent.
	Width, Ascent, Descent float64
	// ItalicsCorrection is the glyph's, or the assembly's.
	ItalicsCorrection int

	// Capped says the assembly would have needed more than
	// MaxMathAssemblyGlyphs glyphs to reach the target, and was built short of
	// it with as many as that allows.
	Capped bool
}

// MathStretchPart is one glyph of an assembly and where its origin goes.
type MathStretchPart struct {
	Glyph int
	X, Y  float64
}

// Stretch is MathML Core §5.3.2's "algorithm to shape a stretchy glyph" to a
// target size along an axis, in font units: along the inline axis — across
// the page — when vertical is false, and along the block axis when it is true.
//
// ok is false where the font has no construction for the glyph on that axis,
// which is the algorithm's "exit with failure": the caller sets the glyph as
// ordinary text.
//
// Otherwise it is the first of these that reaches the target: the glyph
// itself, measured by its advance across or by its ink's height down; each of
// its size variants in the table's order, measured by the advance the table
// states; and the assembly. Where none does — no assembly, or one the §5.3.1
// conditions refuse — it is the last of them that was tried.
func (m *MathTable) Stretch(gid int, vertical bool, target float64) (MathStretch, bool) {
	if !m.HasConstruction(gid, vertical) {
		return MathStretch{}, false
	}
	f := m.face
	last := gid
	if m.glyphSize(gid, vertical) >= target {
		return m.single(gid), true
	}
	for _, v := range m.Variants(gid, vertical) {
		last = v.Glyph
		if float64(v.Advance) >= target {
			return m.single(v.Glyph), true
		}
	}
	if a, ok := m.Assembly(gid, vertical); ok {
		if s, ok := m.assemble(f, a, vertical, target); ok {
			return s, true
		}
	}
	return m.single(last), true
}

// glyphSize is how far a glyph reaches along an axis: its advance across, and
// the height of its ink down, since a glyph has no advance down a line of
// horizontal text.
func (m *MathTable) glyphSize(gid int, vertical bool) float64 {
	if !vertical {
		return float64(m.face.advanceUnits(gid))
	}
	e, ok := m.face.glyphExtents(gid)
	if !ok {
		return 0
	}
	return float64(-e.height)
}

// single is the answer for one glyph: drawn at the origin, measured by its
// advance and its ink, with its own italics correction.
func (m *MathTable) single(gid int) MathStretch {
	s := MathStretch{Glyph: gid, Width: float64(m.face.advanceUnits(gid))}
	if e, ok := m.face.glyphExtents(gid); ok {
		s.Ascent = float64(e.yBearing)
		s.Descent = float64(-(e.yBearing + e.height))
	}
	s.ItalicsCorrection, _ = m.ItalicsCorrection(gid)
	return s
}

// assemble builds an assembly to a target by MathML Core §5.3.1, and false
// where the section refuses the assembly.
func (m *MathTable) assemble(f *Face, a MathGlyphAssembly, vertical bool, target float64) (MathStretch, bool) {
	c, ok := m.connectors(a)
	if !ok {
		return MathStretch{}, false
	}
	omin := float64(m.minOverlap)

	// rmin, the fewest repetitions of each extender that reach the target at
	// the least overlap.
	r := math.Max(0, math.Ceil((target-c.sNonExt+omin*float64(c.nNonExt-1))/c.sExtNonOverlapping))
	capped := false
	if maxR := float64((MaxMathAssemblyGlyphs - c.nNonExt) / c.nExt); r > maxR {
		r, capped = math.Max(0, maxR), true
	}
	reps := int(r)
	count := c.nNonExt + reps*c.nExt

	// omax, the most overlap that still reaches the target: the extra length
	// of an assembly with no overlap, shared evenly between its joins, and no
	// more than any connector that joins allows. It is never less than the
	// least overlap: the connectors are at least that long, and the share is
	// at least that much wherever rmin reached the target — a capped assembly
	// did not, and is built at the least overlap, which is the longest it can
	// be.
	overlap := 0.0
	if count > 1 {
		theoretical := (c.sNonExt + r*c.sExt - target) / float64(count-1)
		overlap = math.Max(omin, math.Min(c.maxOverlap, theoretical))
	}
	size := c.sNonExt + r*c.sExt - overlap*float64(count-1)

	s := MathStretch{ItalicsCorrection: a.ItalicsCorrection, Capped: capped,
		Parts: make([]MathStretchPart, 0, count)}
	if !vertical {
		// The tallest of the parts, which may all sit above the baseline or
		// all below it: the maximum starts from nothing, not from zero.
		s.Ascent, s.Descent = math.Inf(-1), math.Inf(-1)
	}
	at := 0.0
	for _, p := range a.Parts {
		n := 1
		if p.Extender {
			n = reps
		}
		for ; n > 0; n-- {
			part := MathStretchPart{Glyph: p.Glyph}
			e, _ := f.glyphExtents(p.Glyph)
			if vertical {
				// Its bottom at the point reached: §5.3.1 draws a vertical part
				// by its (left, bottom) where it draws a horizontal one by its
				// (left, baseline), and the bottom of a glyph is where its ink
				// ends. A part drawn from its origin up — as nearly every font
				// draws one — has the two in the same place.
				part.Y = at - float64(e.yBearing+e.height)
				s.Width = math.Max(s.Width, float64(f.advanceUnits(p.Glyph)))
			} else {
				part.X = at
				s.Ascent = math.Max(s.Ascent, float64(e.yBearing))
				s.Descent = math.Max(s.Descent, float64(-(e.yBearing + e.height)))
			}
			s.Parts = append(s.Parts, part)
			at += float64(p.FullAdvance) - overlap
		}
	}
	if vertical {
		s.Ascent, s.Descent = size, 0
	} else {
		s.Width = size
	}
	return s, true
}

// assemblySums is what §5.3.1 computes from an assembly's parts before it
// builds anything.
type assemblySums struct {
	nExt, nNonExt      int
	sExt, sNonExt      float64
	sExtNonOverlapping float64
	// maxOverlap is the shortest connector that joins two parts.
	maxOverlap float64
}

// connectors adds up an assembly's parts, and reports whether §5.3.1 lets it be
// built: it has an extender, its extenders grow it when they are joined, and
// every connector that joins two parts is at least the least overlap.
//
// "Every connector that joins two parts" is the reading every implementation
// takes of the section's "for each GlyphPartRecord": the start of the first
// part and the end of the last are the ends of the assembly, which join
// nothing unless the part is an extender and so repeated. Read as every
// connector of every part, it would refuse the assemblies of nearly every
// font — a bottom piece states a start connector of nought — and of the ones
// the suite's own fonts are built with. Which of the two ends is left out is
// the specification's o_max rule read with OpenType's meaning of start and end
// (the bottom and the top of a vertical part, the left and the right of a
// horizontal one); Chromium's implementation reads it the same way.
func (m *MathTable) connectors(a MathGlyphAssembly) (assemblySums, bool) {
	c := assemblySums{maxOverlap: math.Inf(1)}
	for i, p := range a.Parts {
		if p.Extender {
			c.nExt++
			c.sExt += float64(p.FullAdvance)
		} else {
			c.nNonExt++
			c.sNonExt += float64(p.FullAdvance)
		}
		if p.Extender || i > 0 {
			c.maxOverlap = math.Min(c.maxOverlap, float64(p.StartConnector))
		}
		if p.Extender || i < len(a.Parts)-1 {
			c.maxOverlap = math.Min(c.maxOverlap, float64(p.EndConnector))
		}
	}
	omin := float64(m.minOverlap)
	c.sExtNonOverlapping = c.sExt - omin*float64(c.nExt)
	return c, c.nExt > 0 && c.maxOverlap >= omin && c.sExtNonOverlapping > 0
}

// PreferredStretchWidth is MathML Core §5.3.2's "preferred inline size of a
// glyph stretched along the block axis": the widest of the glyph, its
// vertical size variants and its vertical assembly, in font units — the
// width a stretchy operator is given before anything has said how tall it
// has to be.
func (m *MathTable) PreferredStretchWidth(gid int) float64 {
	w := float64(m.face.advanceUnits(gid))
	if !m.HasConstruction(gid, true) {
		return w
	}
	for _, v := range m.Variants(gid, true) {
		w = math.Max(w, float64(m.face.advanceUnits(v.Glyph)))
	}
	if a, ok := m.Assembly(gid, true); ok && m.validAssembly(a) {
		for _, p := range a.Parts {
			w = math.Max(w, float64(m.face.advanceUnits(p.Glyph)))
		}
	}
	return w
}

// validAssembly is §5.3.1's conditions on their own.
func (m *MathTable) validAssembly(a MathGlyphAssembly) bool {
	_, ok := m.connectors(a)
	return ok
}
