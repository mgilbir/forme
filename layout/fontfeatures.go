package layout

import (
	"slices"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/internal/ascii"
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
	lig, _ := ligaturesOf(b.Style.Get("font-variant-ligatures"))
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
	out.Caps, _ = capsOf(b.Style.Get("font-variant-caps"))
	out.Numeric, _ = numericOf(b.Style.Get("font-variant-numeric"))
	out.EastAsian, _ = eastAsianOf(b.Style.Get("font-variant-east-asian"))
	out.Position, _ = variantPositionOf(b.Style.Get("font-variant-position"))
	// And the escape hatch: the face's own features by tag, for everything the
	// descriptors above have no keyword for.
	out.Tags, _ = featureSettingsOf(b.Style.Get("font-feature-settings"))
	// And the language the text is in, which chooses among the font's
	// language systems: lang="sr" has a font draw its Serbian forms. It is the
	// attribute as written — shape reads BCP 47 as HarfBuzz does — and it is
	// here with the rest because it has to travel with them, to the painter
	// that shapes the run again.
	if v, ok := l.Of(boxElement(b)); ok {
		out.Language = v
	}
	return out
}

// featureSettingsOf reads CSS Fonts 4 §6.11's font-feature-settings into the
// tags it turns on, and the ones it turns off.
//
// The two are not symmetrical and that is the property rather than a choice
// here. A tag turned *on* is a request this engine can carry out for any face:
// the shaping layer takes a list of tags and runs their lookups, whatever they
// are. A tag turned *off* is only meaningful for a feature something would
// otherwise have applied, and the ones this engine applies by default have
// switches of their own — font-variant-ligatures and font-kerning — reached
// through the fields above rather than through a tag list. So the off list is
// returned for reporting and not acted on.
//
// The tags come back sorted and deduplicated, as one comma-separated string.
// The order a document writes them in is not the order they are applied in —
// that is the font's, by lookup index — so two declarations naming the same
// features are the same request, and settling the order lets them share the
// memo entry the shaped group is kept under.
func featureSettingsOf(raw string) (on string, off []string) {
	value := ascii.TrimCSSSpace(raw)
	if value == "" || ascii.EqualFold(value, "normal") {
		return "", nil
	}
	var enabled []string
	for _, part := range strings.Split(value, ",") {
		tag, setting, ok := featureSetting(part)
		if !ok {
			continue
		}
		if setting {
			enabled = append(enabled, tag)
			continue
		}
		off = append(off, tag)
	}
	if len(enabled) == 0 {
		return "", off
	}
	slices.Sort(enabled)
	out := enabled[:0]
	for i, tag := range enabled {
		if i == 0 || tag != enabled[i-1] {
			out = append(out, tag)
		}
	}
	return strings.Join(out, ","), off
}

// featureSetting reads one "<tag> [<setting>]" of the list.
//
// A tag is four characters in quotation marks and the setting that follows is
// absent, "on", "off", or an integer. §6.11 makes an integer above zero select
// an alternate *within* the feature rather than merely enable it — "salt" 2 is
// the second alternate — and this engine applies a feature or does not, so any
// positive setting reads as on. That is the same answer for every face that
// offers one alternate, which is nearly all of them, and a narrowing rather than
// a wrong answer where it is not.
func featureSetting(part string) (tag string, on, ok bool) {
	field := ascii.TrimCSSSpace(part)
	quote := strings.IndexAny(field, "\"'")
	if quote < 0 {
		return "", false, false
	}
	rest := field[quote+1:]
	end := strings.IndexAny(rest, "\"'")
	if end < 0 {
		return "", false, false
	}
	tag, rest = rest[:end], ascii.TrimCSSSpace(rest[end+1:])
	// A tag is four characters, and the range is the format's: a face names its
	// features in printable ASCII.
	if len(tag) != 4 {
		return "", false, false
	}
	for i := 0; i < len(tag); i++ {
		if tag[i] < 0x20 || tag[i] > 0x7E {
			return "", false, false
		}
	}
	switch {
	case rest == "" || ascii.EqualFold(rest, "on"):
		return tag, true, true
	case ascii.EqualFold(rest, "off"):
		return tag, false, true
	}
	n, err := strconv.Atoi(rest)
	if err != nil {
		return "", false, false
	}
	return tag, n > 0, true
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
	switch ascii.Lower(ascii.TrimCSSSpace(raw)) {
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
	return ascii.EqualFold(ascii.TrimCSSSpace(b.Style.Get("font-kerning")), "none")
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
	value := ascii.Lower(ascii.TrimCSSSpace(raw))
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
	value := ascii.Lower(ascii.TrimCSSSpace(raw))
	if value == "" || value == "normal" {
		return 0, ""
	}
	var (
		out  shape.Numeric
		seen = map[shape.Numeric]bool{}
	)
	for _, word := range ascii.CSSFields(value) {
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
	value := ascii.Lower(ascii.TrimCSSSpace(raw))
	if value == "" || value == "normal" {
		return 0, ""
	}
	var (
		out  shape.EastAsian
		seen = map[shape.EastAsian]bool{}
	)
	for _, word := range ascii.CSSFields(value) {
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
	value := ascii.Lower(ascii.TrimCSSSpace(raw))
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
