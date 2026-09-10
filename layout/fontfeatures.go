package layout

import (
	"strings"

	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4's properties that turn a font's own rules off, CSS Text §8.2's
// rule that does the same thing for a different reason, and the one property
// that turns a rule on.
//
// A font states what it wants done to its glyphs and a document may overrule it.
// That is the whole of what the first three are, and nothing here asks a face
// whether it has a feature: a face that declares no ligatures is unaffected by a
// rule that turns ligatures off, so the question never has to be asked.
//
// "font-variant-caps" is the exception in both halves of that. It *adds* rules —
// 'smcp', the capitals a designer drew at lowercase height, and the five other
// ways §6.6 has of setting a run in capitals — and it is the one request a face
// can fail to carry out, because a face that does not declare the feature sets
// the text in ordinary letters. The question is still not asked here; it is
// asked once per face run, beside the report that depends on it. See reportCaps
// in textchecks.go.
//
// # Why the two properties are not one flag
//
// They name different sets, and the sets are not nested. "font-variant-ligatures:
// none" is defined by CSS Fonts 4 §6.4 to expand to "no-common-ligatures
// no-discretionary-ligatures no-historical-ligatures no-contextual", so it turns
// off the contextual alternates as well as the ligatures. CSS Text §8.2's rule
// turns off "optional ligatures" and says nothing about alternates: an alternate
// is a different shape for one character, not two characters set as one, so a
// spacing inserted between them has nothing to be between.
//
// Folding the two together would answer one rule's question with the other's
// set, on documents where only one of them applies. See shape.Features.

// featuresFor is what a box's declarations turn off, and the one they turn on.
//
// It is asked of the box the text is in, which is the box the properties are on:
// all four inherit or are inherited through the inline box that carries the
// text, so the answer is the one the run is set with.
func (l *layouter) featuresFor(b *Box) shape.Features {
	if b == nil {
		return shape.Features{}
	}
	var out shape.Features
	lig, _ := ligaturesOf(b.Style["font-variant-ligatures"])
	switch lig {
	case ligaturesNone:
		out.NoOptionalLigatures = true
		out.NoContextualAlternates = true
	case ligaturesNoCommon:
		out.NoOptionalLigatures = true
	case ligaturesNoContextual:
		out.NoContextualAlternates = true
	}
	if noKerning(b) {
		out.NoKerning = true
	}
	if l.spacingSuppressesLigatures(b) {
		out.NoOptionalLigatures = true
	}
	// And the one that goes the other way: rules the face states and no run
	// gets unless it is asked for. Whether the face *has* them is not asked
	// here — a run set in a face without small capitals comes out in ordinary
	// letters at the same width, which is the shaping layer's own contract —
	// and it is asked once per face run by reportCaps, which is where the
	// answer can be reported.
	out.Caps, _ = capsOf(b.Style["font-variant-caps"])
	out.Numeric, _ = numericOf(b.Style["font-variant-numeric"])
	out.EastAsian, _ = eastAsianOf(b.Style["font-variant-east-asian"])
	out.Position, _ = variantPositionOf(b.Style["font-variant-position"])
	return out
}

// spacingSuppressesLigatures is CSS Text §8.2's rule: "when the effective
// spacing between two characters is not zero (due to either justification or
// non-zero computed letter-spacing), user agents should not apply optional
// ligatures".
//
// The reason is what a ligature is. Two letters set as one glyph have no
// boundary between them for a spacing to be inserted at, so a run of "office"
// with the ffi ligature applied and a spacing after every character gets four
// spacings where its neighbours get six — and the letters that were ligated end
// up closer together than the ones that were not, which is the opposite of what
// spacing them apart was for.
//
// # Where the answer comes from, and why not from the spacing itself
//
// From the *declarations*, before anything is laid out. Letter-spacing is
// decidable there and justification is not: how much slack a line has depends on
// how wide its runs are, which depends on whether they ligated, which is the
// question being asked. Deciding it from the measured slack would not terminate.
//
// So a box that will be justified between characters loses its optional
// ligatures whether or not any particular line turns out to have slack in it.
// That is broader than the rule as written, and the case it is broader by is a
// line that fills its measure exactly — where the spacing really is zero and the
// ligature really should apply. It is the direction to err in: §8.2 is a
// "should", the suite's own fixtures for it are marked "should", and a page
// whose ligature comes and goes with the width of its last line is worse than
// one that consistently has none.
func (l *layouter) spacingSuppressesLigatures(b *Box) bool {
	if l.spacingFor(b).Letter != 0 {
		return true
	}
	// Justification is a property of the block container the line belongs to,
	// and this may be an inline box inside one — so the walk goes up to the
	// nearest box that is not an inline box.
	//
	// Not "the nearest block-level box": an inline-block is inline-level and is
	// a block container all the same, and it is justified on its own terms.
	// The suite's letter-spacing-ligatures-001 writes exactly that — an
	// inline-block with "text-justify: auto" inside a container justifying
	// between characters — and asks for the ligature to survive inside it.
	for at := b; at != nil; at = at.Parent {
		if at.Outer != OuterBlock && at.Inner != InnerFlowRoot {
			continue
		}
		if alignmentFrom(at, false, false) != alignJustify {
			return false
		}
		method, _, _ := justificationOf(at)
		return method == justifyCharacters
	}
	return false
}

// The values of font-variant-ligatures this engine reads.
//
// It is the keyword and the two longhand words the suite writes, and not the
// whole grammar: the property takes four independent pairs and a document may
// write any of them in any order. What is here is what a document that turns
// ligatures off writes, and a value outside it is reported rather than guessed
// at — see checkFontFeatures.
type ligatures uint8

const (
	ligaturesNormal ligatures = iota
	ligaturesNone
	ligaturesNoCommon
	ligaturesNoContextual
)

// ligaturesOf reads the property. The second result says whether the value was
// one this engine understands, which checkFontFeatures reports on.
func ligaturesOf(raw string) (ligatures, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "normal":
		return ligaturesNormal, true
	case "none":
		return ligaturesNone, true
	case "no-common-ligatures":
		return ligaturesNoCommon, true
	case "no-contextual":
		return ligaturesNoContextual, true
	}
	return ligaturesNormal, false
}

// noKerning reads CSS Fonts 4 §6.5's font-kerning.
//
// "auto" and "normal" both leave the face's kerning on, and the difference
// between them is about whether a UA may turn it off for performance — which
// this engine never does, so the two are one answer here.
func noKerning(b *Box) bool {
	return strings.EqualFold(strings.TrimSpace(b.Style["font-kerning"]), "none")
}

// capsOf reads CSS Fonts 4 §6.6's font-variant-caps.
//
// All six of its values are a request for features the face declares, so all
// six are read and the answer is shape's own enumeration rather than one of this
// package's: there is nothing for a second enumeration to say. What each value
// asks a face for is Caps.Features, and it is stated there rather than here
// because the shaper and the report have to agree about it.
//
// The second result is the value where it is not one of the six, which reportCaps
// names. A face that declares none of what a value asks for is a different
// answer and a different report: the property was read and the page still came
// out as it is written. See reportCaps.
func capsOf(raw string) (shape.Caps, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "normal":
		return shape.CapsNormal, ""
	case "small-caps":
		return shape.CapsSmall, ""
	case "all-small-caps":
		return shape.CapsAllSmall, ""
	case "petite-caps":
		return shape.CapsPetite, ""
	case "all-petite-caps":
		return shape.CapsAllPetite, ""
	case "unicase":
		return shape.CapsUnicase, ""
	case "titling-caps":
		return shape.CapsTitling, ""
	}
	return shape.CapsNormal, value
}

// numericOf reads CSS Fonts 4 §6.7's font-variant-numeric.
//
// Eight keywords in five independent groups — the figures, their spacing, the
// fraction, the ordinal and the slashed zero — and a value is any combination
// with at most one from each. So the answer is a set and not a value, and it is
// shape's own set rather than one of this package's: every keyword is a request
// for a feature the face declares, which is the whole of what the property does,
// and there is nothing for a second enumeration to say. See shape.Numeric.
//
// The second result is the first word that is not one of the eight, which
// reportNumeric names. A word that *is* one of the eight and repeats a group is
// the other kind of mistake — "lining-nums oldstyle-nums" asks for both sets of
// figures at once — and is reported the same way, because the two are equally
// declarations this engine cannot act on and neither is more the author's fault
// than the other.
func numericOf(raw string) (shape.Numeric, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "normal" {
		return 0, ""
	}
	var (
		out  shape.Numeric
		seen = map[shape.Numeric]bool{}
	)
	for _, word := range strings.Fields(value) {
		bit, group, ok := numericKeyword(word)
		if !ok || seen[group] {
			return 0, word
		}
		seen[group] = true
		out |= bit
	}
	return out, ""
}

// numericKeyword reads one of §6.7's keywords: which feature it asks for, and
// which of the five groups it belongs to.
//
// The group is returned as the *pair* of bits rather than as a name, because
// that is what it is used for — a value may name one member of a group and the
// check is whether the group has been named already.
func numericKeyword(word string) (bit, group shape.Numeric, ok bool) {
	const (
		figures  = shape.NumericLining | shape.NumericOldstyle
		spacing  = shape.NumericProportional | shape.NumericTabular
		fraction = shape.NumericDiagonalFractions | shape.NumericStackedFractions
	)
	switch word {
	case "lining-nums":
		return shape.NumericLining, figures, true
	case "oldstyle-nums":
		return shape.NumericOldstyle, figures, true
	case "proportional-nums":
		return shape.NumericProportional, spacing, true
	case "tabular-nums":
		return shape.NumericTabular, spacing, true
	case "diagonal-fractions":
		return shape.NumericDiagonalFractions, fraction, true
	case "stacked-fractions":
		return shape.NumericStackedFractions, fraction, true
	case "ordinal":
		return shape.NumericOrdinal, shape.NumericOrdinal, true
	case "slashed-zero":
		return shape.NumericSlashedZero, shape.NumericSlashedZero, true
	}
	return 0, 0, false
}

// eastAsianOf reads CSS Fonts 4 §6.9's font-variant-east-asian.
//
// Nine keywords in three groups — the national form, the width, and the ruby
// kana — and a value is any combination with at most one from each. So the
// answer is a set, and shape's own set for the reason numericOf gives: every
// keyword is a request for a feature the face declares, which is the whole of
// what the property does.
//
// The first group is six alternatives rather than a pair, which is the one thing
// about this grammar that surprises. JIS78, JIS83, JIS90 and JIS04 were four
// revisions of one standard and "simplified" and "traditional" are two forms of
// one character, so all six are answers to the same question and a value naming
// two of them asks for one ideograph in two shapes.
//
// The second result is the first word that is not one of the nine, or the first
// that repeats a group, which reportEastAsian names.
func eastAsianOf(raw string) (shape.EastAsian, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" || value == "normal" {
		return 0, ""
	}
	var (
		out  shape.EastAsian
		seen = map[shape.EastAsian]bool{}
	)
	for _, word := range strings.Fields(value) {
		bit, group, ok := eastAsianKeyword(word)
		if !ok || seen[group] {
			return 0, word
		}
		seen[group] = true
		out |= bit
	}
	return out, ""
}

// eastAsianKeyword reads one of §6.9's keywords: which feature it asks for, and
// which of the three groups it belongs to.
//
// The group is the set of bits it competes with, for the reason numericKeyword
// returns one: what it is used for is whether the group has been named already.
func eastAsianKeyword(word string) (bit, group shape.EastAsian, ok bool) {
	const (
		variant = shape.EastAsianJis78 | shape.EastAsianJis83 |
			shape.EastAsianJis90 | shape.EastAsianJis04 |
			shape.EastAsianSimplified | shape.EastAsianTraditional
		width = shape.EastAsianFullWidth | shape.EastAsianProportionalWidth
	)
	switch word {
	case "jis78":
		return shape.EastAsianJis78, variant, true
	case "jis83":
		return shape.EastAsianJis83, variant, true
	case "jis90":
		return shape.EastAsianJis90, variant, true
	case "jis04":
		return shape.EastAsianJis04, variant, true
	case "simplified":
		return shape.EastAsianSimplified, variant, true
	case "traditional":
		return shape.EastAsianTraditional, variant, true
	case "full-width":
		return shape.EastAsianFullWidth, width, true
	case "proportional-width":
		return shape.EastAsianProportionalWidth, width, true
	case "ruby":
		return shape.EastAsianRuby, shape.EastAsianRuby, true
	}
	return 0, 0, false
}

// variantPositionOf reads CSS Fonts 4 §6.5's font-variant-position.
//
// The name is not positionOf, which layout/position.go already has for the
// property that takes a box out of the flow. Two properties of CSS are called
// "position" and neither of them will give the name up.
//
// One value of three, which is what makes it the odd one of the family: a run is
// a subscript or a superscript or neither, and the two are not independent the
// way §6.7's five groups are. So the answer is a value and not a set, and it is
// shape's own for the reason capsOf's is — every value is a request for a
// feature the face declares.
//
// The second result is the value where it is not one of the three, which
// reportPosition names.
func variantPositionOf(raw string) (shape.Position, string) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "", "normal":
		return shape.PositionNormal, ""
	case "sub":
		return shape.PositionSub, ""
	case "super":
		return shape.PositionSuper, ""
	}
	return shape.PositionNormal, value
}
