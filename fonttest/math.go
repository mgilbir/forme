package fonttest

import (
	"encoding/binary"
	"fmt"
)

// A MATH table, for the fixtures of a reader of it and of the layout that sets
// mathematics with one.
//
// Everything is written as the OpenType specification lays it out, with the
// subtables after the headers that point at them and every coverage table in
// format 1. No device table is written: every MathValueRecord's device offset
// is nought, which is what a static font states.

// MathConstantNames are the fifty-six MathConstants in the order the table
// stores them, spelled as the OpenType specification and fontTools spell them.
// MathOptions.Constants is keyed by these.
var MathConstantNames = [...]string{
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

// MathKern is the cut-ins at one corner of a glyph: kern Kerns[i] applies
// below height Heights[i], and the last kern above the last height, so there
// is one more kern than there are heights.
type MathKern struct {
	Heights, Kerns []int
}

// MathVariant is one size variant of a glyph and how far it reaches.
type MathVariant struct {
	Glyph, Advance int
}

// MathPart is one part of a glyph assembly: its glyph, its start and end
// connector lengths, its full advance, and whether it is an extender.
type MathPart struct {
	Glyph, Start, End, Full int
	Extender                bool
}

// MathAssembly is a glyph assembly: its italics correction and its parts, from
// the bottom (or the left) up.
type MathAssembly struct {
	ItalicsCorrection int
	Parts             []MathPart
}

// MathOptions is what a MATH table states. A nil Constants writes no
// MathConstants subtable, and an empty one writes a subtable of zeros. A
// table with no glyph information writes no MathGlyphInfo, and one with no
// variants, no assembly and no connector overlap writes no MathVariants.
type MathOptions struct {
	// Constants by name (see MathConstantNames); a name not listed is zero.
	Constants map[string]int

	ItalicsCorrection   map[int]int
	TopAccentAttachment map[int]int
	ExtendedShapes      []int
	// Kerns is each glyph's cut-ins at its top right, top left, bottom right
	// and bottom left corners, in that order; nil is a corner that states none.
	Kerns map[int][4]*MathKern

	MinConnectorOverlap         int
	VertVariants, HorizVariants map[int][]MathVariant
	VertAssembly, HorizAssembly map[int]MathAssembly
}

// MATH builds the table. It panics on a constant name it does not know and on
// a kern list whose lengths disagree, rather than write a table that says
// something other than the fixture meant.
func MATH(o MathOptions) []byte {
	var constants, glyphInfo, variants []byte
	if o.Constants != nil {
		constants = mathConstants(o.Constants)
	}
	if len(o.ItalicsCorrection) > 0 || len(o.TopAccentAttachment) > 0 ||
		len(o.ExtendedShapes) > 0 || len(o.Kerns) > 0 {
		glyphInfo = mathGlyphInfo(o)
	}
	if len(o.VertVariants) > 0 || len(o.HorizVariants) > 0 || len(o.VertAssembly) > 0 ||
		len(o.HorizAssembly) > 0 || o.MinConnectorOverlap != 0 {
		variants = mathVariants(o)
	}
	out := make([]byte, 10)
	binary.BigEndian.PutUint16(out[0:], 1) // majorVersion
	binary.BigEndian.PutUint16(out[2:], 0) // minorVersion
	for i, sub := range [][]byte{constants, glyphInfo, variants} {
		if sub == nil {
			continue
		}
		binary.BigEndian.PutUint16(out[4+2*i:], uint16(len(out)))
		out = append(out, sub...)
	}
	return out
}

func mathConstants(named map[string]int) []byte {
	index := map[string]int{}
	for i, n := range MathConstantNames {
		index[n] = i
	}
	var v [len(MathConstantNames)]int
	for name, value := range named {
		i, ok := index[name]
		if !ok {
			panic(fmt.Sprintf("fonttest: %q is not a MathConstants name", name))
		}
		v[i] = value
	}
	out := make([]byte, 8+4*51+2)
	putI16(out[0:], int16(v[0]))
	putI16(out[2:], int16(v[1]))
	binary.BigEndian.PutUint16(out[4:], uint16(v[2]))
	binary.BigEndian.PutUint16(out[6:], uint16(v[3]))
	for i := 4; i < 55; i++ {
		putI16(out[8+4*(i-4):], int16(v[i]))
	}
	putI16(out[8+4*51:], int16(v[55]))
	return out
}

// valueTable is a coverage offset, a count and that many MathValueRecords,
// for the glyphs of a map in ascending order, with the coverage after them.
func valueTable(values map[int]int) []byte {
	glyphs := intKeys(values)
	out := make([]byte, 4+4*len(glyphs))
	binary.BigEndian.PutUint16(out[0:], uint16(len(out)))
	binary.BigEndian.PutUint16(out[2:], uint16(len(glyphs)))
	for i, g := range glyphs {
		putI16(out[4+4*i:], int16(values[g]))
	}
	return append(out, coverageFormat1(glyphs)...)
}

func mathGlyphInfo(o MathOptions) []byte {
	out := make([]byte, 8)
	put := func(slot int, sub []byte) {
		binary.BigEndian.PutUint16(out[2*slot:], uint16(len(out)))
		out = append(out, sub...)
	}
	if len(o.ItalicsCorrection) > 0 {
		put(0, valueTable(o.ItalicsCorrection))
	}
	if len(o.TopAccentAttachment) > 0 {
		put(1, valueTable(o.TopAccentAttachment))
	}
	if len(o.ExtendedShapes) > 0 {
		put(2, sortedCoverage(o.ExtendedShapes))
	}
	if len(o.Kerns) > 0 {
		put(3, mathKernInfo(o.Kerns))
	}
	return out
}

func mathKernInfo(kerns map[int][4]*MathKern) []byte {
	glyphs := intKeys(kerns)
	out := make([]byte, 4+8*len(glyphs))
	binary.BigEndian.PutUint16(out[2:], uint16(len(glyphs)))
	for i, g := range glyphs {
		for corner, k := range kerns[g] {
			if k == nil {
				continue
			}
			if len(k.Kerns) != len(k.Heights)+1 {
				panic(fmt.Sprintf("fonttest: glyph %d corner %d has %d heights and %d kerns; "+
					"a MathKern has one more kern than heights", g, corner, len(k.Heights), len(k.Kerns)))
			}
			binary.BigEndian.PutUint16(out[4+8*i+2*corner:], uint16(len(out)))
			rec := make([]byte, 2+4*(len(k.Heights)+len(k.Kerns)))
			binary.BigEndian.PutUint16(rec[0:], uint16(len(k.Heights)))
			for j, h := range k.Heights {
				putI16(rec[2+4*j:], int16(h))
			}
			for j, v := range k.Kerns {
				putI16(rec[2+4*(len(k.Heights)+j):], int16(v))
			}
			out = append(out, rec...)
		}
	}
	binary.BigEndian.PutUint16(out[0:], uint16(len(out)))
	return append(out, coverageFormat1(glyphs)...)
}

func mathVariants(o MathOptions) []byte {
	vert := constructed(o.VertVariants, o.VertAssembly)
	horiz := constructed(o.HorizVariants, o.HorizAssembly)
	out := make([]byte, 10+2*(len(vert)+len(horiz)))
	binary.BigEndian.PutUint16(out[0:], uint16(o.MinConnectorOverlap))
	binary.BigEndian.PutUint16(out[6:], uint16(len(vert)))
	binary.BigEndian.PutUint16(out[8:], uint16(len(horiz)))
	for i, g := range append(append([]int(nil), vert...), horiz...) {
		variants, assembly, hasAssembly := o.VertVariants[g], o.VertAssembly[g], false
		if i < len(vert) {
			_, hasAssembly = o.VertAssembly[g]
		} else {
			variants, assembly = o.HorizVariants[g], o.HorizAssembly[g]
			_, hasAssembly = o.HorizAssembly[g]
		}
		binary.BigEndian.PutUint16(out[10+2*i:], uint16(len(out)))
		out = append(out, glyphConstruction(variants, assembly, hasAssembly)...)
	}
	if len(vert) > 0 {
		binary.BigEndian.PutUint16(out[2:], uint16(len(out)))
		out = append(out, coverageFormat1(vert)...)
	}
	if len(horiz) > 0 {
		binary.BigEndian.PutUint16(out[4:], uint16(len(out)))
		out = append(out, coverageFormat1(horiz)...)
	}
	return out
}

// constructed is every glyph with variants or an assembly on one axis.
func constructed(variants map[int][]MathVariant, assemblies map[int]MathAssembly) []int {
	set := map[int]bool{}
	for g := range variants {
		set[g] = true
	}
	for g := range assemblies {
		set[g] = true
	}
	return intKeys(set)
}

func glyphConstruction(variants []MathVariant, a MathAssembly, hasAssembly bool) []byte {
	out := make([]byte, 4+4*len(variants))
	binary.BigEndian.PutUint16(out[2:], uint16(len(variants)))
	for i, v := range variants {
		binary.BigEndian.PutUint16(out[4+4*i:], uint16(v.Glyph))
		binary.BigEndian.PutUint16(out[6+4*i:], uint16(v.Advance))
	}
	if !hasAssembly {
		return out
	}
	binary.BigEndian.PutUint16(out[0:], uint16(len(out)))
	asm := make([]byte, 6+10*len(a.Parts))
	putI16(asm[0:], int16(a.ItalicsCorrection))
	binary.BigEndian.PutUint16(asm[4:], uint16(len(a.Parts)))
	for i, p := range a.Parts {
		rec := asm[6+10*i:]
		binary.BigEndian.PutUint16(rec[0:], uint16(p.Glyph))
		binary.BigEndian.PutUint16(rec[2:], uint16(p.Start))
		binary.BigEndian.PutUint16(rec[4:], uint16(p.End))
		binary.BigEndian.PutUint16(rec[6:], uint16(p.Full))
		if p.Extender {
			binary.BigEndian.PutUint16(rec[8:], 1)
		}
	}
	return append(out, asm...)
}
