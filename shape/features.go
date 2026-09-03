package shape

// What a caller asks a face *not* to apply.
//
// Every other request to this package is a question about the text: which glyphs
// it needs, which forms its letters take, how wide it is. This is the one thing
// the caller knows that the text does not say — CSS has properties that turn a
// font's own rules off, and a font has no way of knowing it has been overruled.
//
// The three are apart rather than one flag because the rules that ask for them
// name different sets, and folding two of them together would answer one
// question with the other's set:
//
//   - "font-variant-ligatures: none" turns off the common, discretionary and
//     historical ligatures *and* the contextual alternates, which is what CSS
//     Fonts 4 defines that keyword to expand to.
//   - CSS Text §8.2's spacing rule turns off the *optional ligatures* and says
//     nothing about contextual alternates: an alternate is a different shape for
//     one character, not two characters set as one, so a spacing between them
//     has nothing to be between.
//   - "font-kerning: none" turns off the kerning and neither of the others.
//
// A face that declares none of them is unaffected by all three, which is why
// nothing here has to ask whether the font has the feature before turning it
// off.

// Features is the set of a font's own rules a caller has turned off.
//
// The zero value applies everything, which is what almost every run wants and
// what every caller that has no opinion should pass.
type Features struct {
	// NoOptionalLigatures suppresses "liga", "clig", "dlig" and "hlig": the
	// ligatures a font offers rather than the ones a script requires.
	//
	// The required ones — "rlig" — are not among them and cannot be turned off
	// by anything here. A lam-alef in Arabic is not an embellishment: the pair
	// is written as one letter, and setting it as two is not the word.
	NoOptionalLigatures bool
	// NoContextualAlternates suppresses "calt": the shapes a font substitutes
	// for a character because of its neighbours, without joining anything.
	NoContextualAlternates bool
	// NoKerning suppresses the pair adjustments of the "kern" feature and of
	// GPOS pair positioning, including the pair that spans a run boundary.
	NoKerning bool
}

// suppresses reports whether a feature tag is one this set turns off.
func (f Features) suppresses(tag string) bool {
	switch tag {
	case "liga", "clig", "dlig", "hlig":
		return f.NoOptionalLigatures
	case "calt":
		return f.NoContextualAlternates
	}
	return false
}

// keeps returns the tags of a list this set leaves on, in the same order.
//
// The order is the one the caller gave, which for the default lists is the
// order the specification requires: composition before the rules that read its
// output, required ligatures before optional ones, contextual alternates last
// so that they see the glyphs which survived. Dropping a tag from the middle
// must not disturb that, which is why this filters rather than rebuilds.
func (f Features) keeps(tags []string) []string {
	if f == (Features{}) {
		return tags
	}
	out := tags[:0:0]
	for _, tag := range tags {
		if !f.suppresses(tag) {
			out = append(out, tag)
		}
	}
	return out
}
