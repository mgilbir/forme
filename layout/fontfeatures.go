package layout

import (
	"strings"

	"github.com/mgilbir/forme/shape"
)

// CSS Fonts 4's two properties that turn a font's own rules off, and CSS Text
// §8.2's rule that does the same thing for a different reason.
//
// A font states what it wants done to its glyphs and a document may overrule it.
// That is the whole of what these are: nothing here adds a feature, and nothing
// here asks a face whether it has one — a face that declares no ligatures is
// unaffected by a rule that turns ligatures off, so the question never has to be
// asked.
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

// featuresFor is what a box's declarations turn off.
//
// It is asked of the box the text is in, which is the box the properties are on:
// all three inherit or are inherited through the inline box that carries the
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
