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
// "font-variant-caps" and "font-variant-numeric" ask for rules the font states
// and no run gets by default: 'smcp', the small capitals a designer drew for the
// lowercase letters, and the five other ways §6.6 has of setting a run in
// capitals; 'onum' and 'tnum' and the six other things §6.7 does to a figure.
// They are here rather than in ShapeGlyphsWith's caller-named list because they
// are the same kind of fact as the other three — something about the run that
// its own text does not say, decided by a declaration — and because they have to
// travel the whole way to the backend that draws the run. Everything between
// layout and the pen already carries a Features, and nothing carries a list of
// tags.
//
// It is *not* the same in the one way that matters to a report: turning a rule
// off is right whether or not the face has it, and turning one on is only
// possible when it does. A face with no small capitals sets the text in
// ordinary letters, which is a page the document did not ask for. Nothing here
// says so — this is the shaping layer, and the run still comes out — but the
// caller can compare Caps.Features against Face.Features before it draws and
// say so itself.

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
	// Caps is the capitals a run is set in: the first request here that asks a
	// face for a rule rather than taking one away.
	//
	// A face that declares none of the value's features is unaffected, and the
	// run is set in the letters it is written with. See Caps.Features for what
	// each value asks for, and Face.Features for the question to ask first.
	Caps Caps
	// Numeric is the figures a run is set in: which of the face's digits, how
	// they are spaced, and what it does with a fraction, an ordinal and a zero.
	//
	// It is a *set* where Caps is one value, and that is the property and not a
	// choice made here: CSS Fonts 4 §6.7 lets a document ask for oldstyle
	// figures, tabular spacing and a slashed zero at once, and the three are
	// three of the font's rules over the same digits. See Numeric.
	Numeric Numeric
}

// Caps is CSS Fonts 4 §6.6's font-variant-caps, as a set of features to ask a
// face for.
//
// All six values are exactly that — a tag or a pair of tags the font states and
// no run is given unless it asks — which is why the property is one field here
// and not six. What separates them is which letters they act on and what they
// turn those letters into, and the font knows both; nothing in this package has
// to.
type Caps uint8

const (
	// CapsNormal is the letters the text is written with.
	CapsNormal Caps = iota
	// CapsSmall is "small-caps": the capitals a designer drew at lowercase
	// height, put in place of the lowercase letters. The capitals are left
	// alone, which is the whole difference between this and CapsAllSmall.
	CapsSmall
	// CapsAllSmall is "all-small-caps": the same, and the capitals lowered to
	// match, so that a line has one height of letter throughout.
	CapsAllSmall
	// CapsPetite and CapsAllPetite are the same pair for a second, shorter set
	// of capitals — petite capitals are cut to x-height where small capitals
	// stand a little above it. Few faces draw them.
	CapsPetite
	CapsAllPetite
	// CapsUnicase mixes the two cases at one height: the capitals kept and the
	// lowercase letters left as they are, with the face's own single-height
	// forms for both.
	CapsUnicase
	// CapsTitling is capitals cut for a line that is all capitals — lighter,
	// and spaced for a title rather than for a word inside a sentence. It
	// replaces no letter with a letter of the other case.
	CapsTitling
)

// Capitals is the feature a value asks a face to apply to the capitals, and
// Lowercase the one it asks for the lowercase letters. Either is empty where
// the value leaves that case alone: "small-caps" does not touch the capitals,
// and "titling-caps" does not touch the lowercase letters.
//
// They are the same tag for "unicase", which is one feature that puts both
// cases at one height rather than two that meet in the middle.
//
// The pair is what the property *is*, and it is what a caller needs rather than
// the list: which case a face has failed to cover decides which letters come out
// wrong, and a list of tags cannot say. See Features, which is derived from
// this so that the two cannot disagree.
func (c Caps) Capitals() string  { return capsFeatures[c].capitals }
func (c Caps) Lowercase() string { return capsFeatures[c].lowercase }

// Features are the tags a value asks a face for, in the order they are applied.
//
// The order inside a pair does not decide anything: 'c2sc' covers the capitals
// and 'smcp' the lowercase letters, which are disjoint sets, so neither can see
// what the other did. It is §6.6's order — the capitals first — because that is
// the order the property is defined in and there is no reason to write a
// different one.
func (c Caps) Features() []string {
	pair := capsFeatures[c]
	switch {
	case pair.capitals == "" && pair.lowercase == "":
		return nil
	case pair.capitals == "":
		return []string{pair.lowercase}
	case pair.lowercase == "" || pair.capitals == pair.lowercase:
		return []string{pair.capitals}
	}
	return []string{pair.capitals, pair.lowercase}
}

// capsFeatures is what each value asks of each case, declared once so that a
// caller asking what a value needs and the shaper applying it cannot answer
// differently.
//
// Where the tags are applied is the part worth stating. They go after 'ccmp'
// and 'locl' and *before* the ligatures, which is the order HarfBuzz produces
// and is not the order a caller-named feature gets: "office" set in Noto Sans
// with small capitals is six small capitals and no ffi ligature, because the
// ligature is stated over the lowercase glyphs and by the time 'liga' is
// reached there are none left. Applying them last instead leaves the ffi
// ligature standing in the middle of a line of capitals — three letters that
// did not get the rule the other three did.
var capsFeatures = [...]struct{ capitals, lowercase string }{
	CapsNormal:    {},
	CapsSmall:     {lowercase: "smcp"},
	CapsAllSmall:  {capitals: "c2sc", lowercase: "smcp"},
	CapsPetite:    {lowercase: "pcap"},
	CapsAllPetite: {capitals: "c2pc", lowercase: "pcap"},
	CapsUnicase:   {capitals: "unic", lowercase: "unic"},
	CapsTitling:   {capitals: "titl"},
}

// adds returns the tags this set turns on, in the order they are applied.
//
// The capitals before the figures, which is the order §6.6 and §6.7 are written
// in and is not a decision this can make well: the two act on disjoint sets of
// characters — letters and digits — so no rule of one can see what the other
// did, and the order between them is unobservable.
func (f Features) adds() []string {
	caps, numeric := f.Caps.Features(), f.Numeric.Features()
	switch {
	case len(numeric) == 0:
		return caps
	case len(caps) == 0:
		return numeric
	}
	out := make([]string, 0, len(caps)+len(numeric))
	return append(append(out, caps...), numeric...)
}

// Numeric is CSS Fonts 4 §6.7's font-variant-numeric, as the set of features it
// asks a face for.
//
// A set and not a value, because the property is: §6.7's grammar is three
// independent pairs and two independent keywords, and a document may ask for
// oldstyle figures, tabular spacing and a slashed zero at once. What separates
// the eight is which of the font's rules they name, and the font knows what each
// of those does; nothing in this package has to.
//
// None of them is synthesised anywhere, by this engine or by a browser. An
// oldstyle figure is a shape a designer drew, and there is nothing to make one
// out of — which is the difference between this property and small capitals, and
// the reason a face that declares none of these is simply reported.
type Numeric uint16

const (
	// The figures §6.7 calls <numeric-figure-values>: which set of digits, of
	// the two a face may draw. Lining figures stand at cap height and are the
	// default of almost every face; oldstyle figures have ascenders and
	// descenders and sit with the lowercase letters.
	NumericLining Numeric = 1 << iota
	NumericOldstyle
	// <numeric-spacing-values>: whether the digits are set at one width so that
	// a column of figures lines up, or each at its own. It is the pair that
	// matters most in a table and the one an author is most likely to write.
	NumericProportional
	NumericTabular
	// <numeric-fraction-values>: a fraction set on a diagonal, "1/2" with the
	// numerator raised and the denominator lowered around a slash, or stacked
	// one above the other.
	NumericDiagonalFractions
	NumericStackedFractions
	// The two that stand alone. "ordinal" is the raised letters after a number
	// — the "st" of "1st", the "ª" of a Spanish ordinal — and "slashed-zero" is
	// the zero with a stroke through it, which is what tells it from a capital
	// O in a serial number.
	NumericOrdinal
	NumericSlashedZero
)

// Features are the tags this set asks a face for, in the order they are applied.
//
// §6.7's own order: the figures, the spacing, the fraction, then the two that
// stand alone. Whether it decides anything depends on the face and cannot be
// settled here — a font may state its slashed zero over the lining zero, over
// the oldstyle one, or over both — so the order is the specification's, which is
// the one a font is most likely to have been tested against.
func (n Numeric) Features() []string {
	if n == 0 {
		return nil
	}
	out := make([]string, 0, 5)
	for _, each := range numericFeatures {
		if n&each.bit != 0 {
			out = append(out, each.tag)
		}
	}
	return out
}

// Has reports whether a set asks for one of §6.7's features.
func (n Numeric) Has(bit Numeric) bool { return n&bit != 0 }

// numericFeatures is the tag each bit names, in the order Features returns them.
var numericFeatures = [...]struct {
	bit Numeric
	tag string
}{
	{NumericLining, "lnum"},
	{NumericOldstyle, "onum"},
	{NumericProportional, "pnum"},
	{NumericTabular, "tnum"},
	{NumericDiagonalFractions, "frac"},
	{NumericStackedFractions, "afrc"},
	{NumericOrdinal, "ordn"},
	{NumericSlashedZero, "zero"},
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
