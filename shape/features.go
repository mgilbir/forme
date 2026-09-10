package shape

// What a caller asks a face to apply and not to apply.
//
// Every other request to this package is a question about the text: which glyphs
// it needs, which forms its letters take, how wide it is. This is the one thing
// the caller knows that the text does not say — CSS has properties that turn a
// font's own rules off and one that turns a rule on, and a font has no way of
// knowing either that it has been overruled or that it has been asked.
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
//
// # The one that goes the other way
//
// "font-variant-caps: small-caps" asks for a rule the font states and no run
// gets by default: 'smcp', the small capitals a designer drew for the lowercase
// letters. It is here rather than in ShapeGlyphsWith's caller-named list
// because it is the same kind of fact as the other three — something about the
// run that its own text does not say, decided by a declaration — and because
// it has to travel the whole way to the backend that draws the run. Everything
// between layout and the pen already carries a Features, and nothing carries a
// list of tags.
//
// It is *not* the same in the one way that matters to a report: turning a rule
// off is right whether or not the face has it, and turning one on is only
// possible when it does. A face with no small capitals sets the text in
// ordinary letters, which is a page the document did not ask for. Nothing here
// says so — this is the shaping layer, and the run still comes out — but the
// caller can ask Features() before it draws and say so itself.

// Features is what a caller has turned off in a font's own rules, and the one
// it has turned on.
//
// The zero value applies exactly the rules the font states for the run, which
// is what almost every run wants and what every caller that has no opinion
// should pass.
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
	// SmallCaps applies "smcp": the capitals a designer drew at lowercase size,
	// which a font states and no run is given unless it asks.
	//
	// A face that does not declare it is unaffected, and the run is set in
	// ordinary letters. See Face.Features for the question to ask first.
	SmallCaps bool
}

// adds returns the tags this set turns on, in the order they are applied.
func (f Features) adds() []string {
	if !f.SmallCaps {
		return nil
	}
	return smallCapsFeatures
}

// The features "font-variant-caps: small-caps" asks a face for.
//
// One tag, and where it is applied is the part worth stating. It goes after
// 'ccmp' and 'locl' and *before* the ligatures, which is the order HarfBuzz
// produces and is not the order a caller-named feature gets: "office" set in
// Noto Sans with small capitals is six small capitals and no ffi ligature,
// because the ligature is stated over the lowercase glyphs and by the time
// 'liga' is reached there are none left. Applying it last instead leaves the
// ffi ligature standing in the middle of a line of capitals — three letters
// that did not get the rule the other three did.
var smallCapsFeatures = []string{"smcp"}

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
