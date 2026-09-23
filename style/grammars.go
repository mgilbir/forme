package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
)

// Every registered property's value definition, from its specification. See
// grammar.go for what the table is for and what it deliberately does not say.
//
// Each entry cites where its grammar is written. A range such as [0,∞] is the
// specification's, and it binds a literal only: a math function is clamped at
// computed-value time instead of making the declaration invalid.

func init() {
	g := valueGrammars

	lp := num(lengthPctSlot)
	lpNonNeg := num(lengthPctSlot.nonNeg())
	lengthNonNeg := num(lengthSlot.nonNeg())

	// css-sizing-3 §3.1 and css-sizing-4 §3.1: the preferred sizes.
	sizing := []term{
		kw("min-content", "max-content", "fit-content", "stretch"), lpNonNeg, fitContentFn,
	}
	g["width"] = single(either(append([]term{kw("auto")}, sizing...)...))
	g["height"] = g["width"]
	g["min-width"] = g["width"]
	g["min-height"] = g["width"]
	g["max-width"] = single(either(append([]term{kw("none")}, sizing...)...))
	g["max-height"] = g["max-width"]
	// css-sizing-4 §4.1: auto || <ratio>.
	g["aspect-ratio"] = aspectRatio

	// CSS 2.1 §8.3, §8.4: margins may be negative, paddings may not.
	for _, side := range []string{"top", "right", "bottom", "left"} {
		g["margin-"+side] = single(either(kw("auto"), lp))
		g["padding-"+side] = single(lpNonNeg)
		// §9.3.2's offsets.
		g[side] = single(either(kw("auto"), lp))
		// css-backgrounds-3 §4.3, §4.2, §4.1.
		g["border-"+side+"-width"] = single(lineWidth)
		g["border-"+side+"-style"] = single(lineStyle)
		g["border-"+side+"-color"] = single(colour)
	}
	g["box-sizing"] = single(kw("content-box", "border-box"))

	// CSS 2.1 §9.5.1-2, with css-logical-1 §2.3's flow-relative values.
	g["float"] = single(kw("left", "right", "none", "inline-start", "inline-end"))
	g["clear"] = single(kw("none", "left", "right", "both", "inline-start", "inline-end"))
	// css-position-3 §2.
	g["position"] = single(kw("static", "relative", "absolute", "fixed", "sticky"))
	// CSS 2.1 §9.9.1.
	g["z-index"] = single(either(kw("auto"), num(integerSlot)))

	// css-ui-4 §3: auto | <outline-line-style>, the border styles without hidden.
	g["outline-width"] = single(lineWidth)
	g["outline-style"] = single(kw("auto", "none", "dotted", "dashed", "solid",
		"double", "groove", "ridge", "inset", "outset"))
	// css-ui-4 §3.4 has auto; CSS 2.1 §18.4 has invert.
	g["outline-color"] = single(either(kw("auto", "invert"), colour))
	g["list-style-image"] = single(either(kw("none"), image))

	// css-color-4 §3.
	g["color"] = single(colour)
	g["background-color"] = single(colour)
	g["text-decoration-color"] = single(colour)

	// css-fonts-4.
	g["font-family"] = commaList(familyName)
	g["font-size"] = single(either(fontSizeKeyword, lpNonNeg))
	g["font-style"] = fontStyle
	g["font-weight"] = single(fontWeight)
	g["font-kerning"] = single(kw("auto", "normal", "none"))
	g["font-variant-ligatures"] = oneOf(single(kw("normal", "none")), groups(ligatureKeywordGroup))
	g["font-variant-caps"] = single(kw("normal", "small-caps", "all-small-caps",
		"petite-caps", "all-petite-caps", "unicase", "titling-caps"))
	g["font-variant-numeric"] = oneOf(single(kw("normal")), groups(numericKeywordGroup))
	g["font-variant-east-asian"] = oneOf(single(kw("normal")), groups(eastAsianKeywordGroup))
	g["font-variant-position"] = single(kw("normal", "sub", "super"))
	g["font-feature-settings"] = oneOf(single(kw("normal")), commaList(featureTag))

	// CSS 2.1 §10.8.1 and css-inline-3.
	g["line-height"] = single(either(kw("normal"), num(numeric{number: true,
		length: true, percent: true}.nonNeg())))
	g["vertical-align"] = single(either(kw("baseline", "sub", "super", "text-top",
		"text-bottom", "middle", "top", "bottom"), lp))

	// css-text-4.
	g["letter-spacing"] = single(either(kw("normal"), lp))
	g["word-spacing"] = single(either(kw("normal"), lp))
	g["text-align-all"] = single(kw("start", "end", "left", "right", "center",
		"justify", "match-parent"))
	g["text-align-last"] = single(kw("auto", "start", "end", "left", "right",
		"center", "justify", "match-parent"))
	// §7.3; "distribute" is the legacy alias §7.3.1 keeps.
	g["text-justify"] = single(kw("auto", "none", "inter-word", "inter-character",
		"distribute"))
	// §8.1: <length-percentage> && hanging? && each-line?
	g["text-indent"] = textIndent
	g["text-transform"] = textTransform
	g["text-spacing-trim"] = single(kw("space-all", "normal", "space-first",
		"trim-start", "trim-both", "trim-all", "auto"))
	g["white-space-collapse"] = single(kw("collapse", "discard", "preserve",
		"preserve-breaks", "preserve-spaces", "break-spaces"))
	g["text-wrap-mode"] = single(kw("wrap", "nowrap"))
	g["text-wrap-style"] = single(kw("auto", "balance", "stable", "pretty",
		"avoid-orphans"))
	g["overflow-wrap"] = single(kw("normal", "break-word", "anywhere"))
	g["word-break"] = single(kw("normal", "keep-all", "break-all", "break-word", "manual",
		"auto-phrase"))
	// §4.4: none | [ space | ideographic-space ] && auto-phrase?
	g["word-space-transform"] = oneOf(single(kw("none")),
		anyOrderRequired(kw("space", "ideographic-space"), kw("auto-phrase")))
	g["line-break"] = single(kw("auto", "loose", "normal", "strict", "anywhere"))
	g["hyphens"] = single(kw("none", "manual", "auto"))
	// §6.2: normal | <autospace> | auto.
	g["text-autospace"] = oneOf(single(kw("normal", "auto", "no-autospace")),
		anyOrder(kw("ideograph-alpha"), kw("ideograph-numeric"), kw("punctuation"),
			kw("insert", "replace")))
	// §8.2: none | [ first || [ force-end | allow-end ] || last ].
	g["hanging-punctuation"] = oneOf(single(kw("none")),
		anyOrder(kw("first"), kw("force-end", "allow-end"), kw("last")))
	g["hyphenate-character"] = single(either(kw("auto"), str))
	g["hyphenate-limit-chars"] = repeated(either(kw("auto"), num(integerSlot)), 1, 3)
	// §6.2: <number [0,∞]> | <length [0,∞]>.
	g["tab-size"] = single(num(numeric{number: true, length: true}.nonNeg()))
	// css-text-5 text-fit: [ none | grow | shrink ] [ consistent | per-line |
	// per-line-all ]? <percentage>?
	g["text-fit"] = textFit

	// css-text-decor-4.
	g["text-decoration-line"] = oneOf(single(kw("none", "spelling-error", "grammar-error")),
		anyOrder(kw("underline"), kw("overline"), kw("line-through"), kw("blink")))
	g["text-decoration-thickness"] = single(either(kw("auto", "from-font"), lp))
	g["text-underline-offset"] = single(either(kw("auto"), lp))

	// css-overflow-4 §5.
	g["line-clamp"] = oneOf(single(kw("none")), lineClamp)
	g["-webkit-line-clamp"] = single(either(kw("none"),
		num(numeric{number: true, integer: true, min: 1, hasMin: true})))
	g["-webkit-box-orient"] = single(kw("horizontal", "vertical", "inline-axis", "block-axis"))

	// css-writing-modes-4. The SVG 1.1 spellings of writing-mode are §3.2's
	// obsolete values, which browsers accept; "sideways-right" is the old name
	// of "sideways".
	g["direction"] = single(kw("ltr", "rtl"))
	g["writing-mode"] = single(kw("horizontal-tb", "vertical-rl", "vertical-lr",
		"sideways-rl", "sideways-lr", "lr", "lr-tb", "rl", "rl-tb", "tb", "tb-rl"))
	g["text-orientation"] = single(kw("mixed", "upright", "sideways", "sideways-right"))
	g["text-combine-upright"] = oneOf(single(kw("none", "all")), combineDigits)
	g["unicode-bidi"] = single(kw("normal", "embed", "isolate", "bidi-override",
		"isolate-override", "plaintext"))

	// css-flexbox-1.
	g["flex-direction"] = single(kw("row", "row-reverse", "column", "column-reverse"))
	g["flex-wrap"] = single(kw("nowrap", "wrap", "wrap-reverse"))
	g["flex-grow"] = single(num(numberSlot.nonNeg()))
	g["flex-shrink"] = single(num(numberSlot.nonNeg()))
	g["flex-basis"] = oneOf(single(kw("content")), g["width"])
	g["order"] = single(num(integerSlot))

	// css-align-3.
	g["justify-content"] = justifyContent
	g["align-content"] = alignContent
	g["align-items"] = alignItems
	g["align-self"] = oneOf(single(kw("auto")), alignItems)
	g["justify-items"] = justifyItems
	g["justify-self"] = oneOf(single(kw("auto")), justifySelf)

	// css-grid-2.
	g["grid-template-columns"] = gridTemplate
	g["grid-template-rows"] = gridTemplate
	g["grid-template-areas"] = oneOf(single(kw("none")), repeated(str, 1, 1<<30))
	g["grid-auto-flow"] = anyOrder(kw("row", "column"), kw("dense"))
	g["grid-auto-rows"] = repeated(trackSize, 1, 1<<30)
	g["grid-auto-columns"] = g["grid-auto-rows"]
	for _, p := range []string{"grid-row-start", "grid-row-end",
		"grid-column-start", "grid-column-end"} {
		g[p] = gridLine
	}

	// css-multicol-1.
	g["column-count"] = single(either(kw("auto"),
		num(numeric{number: true, integer: true, min: 1, hasMin: true})))
	g["column-width"] = single(either(kw("auto"), lengthNonNeg))
	g["column-gap"] = single(either(kw("normal"), lpNonNeg))
	g["row-gap"] = g["column-gap"]
	g["column-fill"] = single(kw("auto", "balance", "balance-all"))
	g["column-span"] = single(kw("none", "all"))
	// css-break-3 §5.4.
	g["box-decoration-break"] = single(kw("slice", "clone"))
	// css-break-4 §3.1 and §3.2.
	g["break-before"] = single(kw(breakBetweenValues...))
	g["break-after"] = g["break-before"]
	g["break-inside"] = single(kw("auto", "avoid", "avoid-page", "avoid-column", "avoid-region"))

	// css-content-3 §1 and CSS 2.1 §12.
	g["content"] = content
	g["quotes"] = oneOf(single(kw("auto", "match-parent")), fromBool(legalQuotes))
	// css-lists-3 §4.
	g["counter-reset"] = counters(true)
	g["counter-increment"] = counters(false)
	g["counter-set"] = counters(false)
	g["list-style-type"] = single(either(str, customIdent(), symbolsFn))
	g["list-style-position"] = single(kw("inside", "outside"))

	// css-backgrounds-3.
	g["background-image"] = fromBool(legalBackgroundImage)
	g["background-repeat"] = commaList(repeatStyle)
	g["background-attachment"] = commaList(single(kw("scroll", "fixed", "local")))
	g["background-position"] = commaList(position)
	g["background-size"] = commaList(bgSize)
	g["background-origin"] = commaList(single(kw("border-box", "padding-box", "content-box")))
	g["background-clip"] = commaList(single(kw("border-box", "padding-box",
		"content-box", "text", "border-area")))

	// CSS 2.1 §17.
	g["border-collapse"] = single(kw("collapse", "separate"))
	g["border-spacing"] = repeated(lengthNonNeg, 1, 2)
	g["caption-side"] = single(kw("top", "bottom"))
	g["empty-cells"] = single(kw("show", "hide"))
	g["table-layout"] = single(kw("auto", "fixed"))

	// CSS 2.1 §11 and css-overflow-3; "overlay" is the legacy alias of auto
	// §3.1 keeps.
	g["visibility"] = single(kw("visible", "hidden", "collapse"))
	g["overflow-x"] = single(kw("visible", "hidden", "clip", "scroll", "auto", "overlay"))
	g["overflow-y"] = g["overflow-x"]
	g["clip"] = single(either(kw("auto"), rectFn))
	// css-color-4 §11.2.
	g["opacity"] = single(num(numeric{number: true, percent: true}))
	// css-images-3 §5.5-6.
	g["object-fit"] = single(kw("fill", "contain", "cover", "none", "scale-down"))
	g["object-position"] = position

	// css-display-3, kept as it was written: see legalDisplay.
	g["display"] = fromBool(legalDisplay)
}

// lineWidth is <line-width>: a non-negative length or one of three keywords.
var lineWidth = either(kw("thin", "medium", "thick"), num(lengthSlot.nonNeg()))

// lineStyle is <line-style>.
var lineStyle = kw("none", "hidden", "dotted", "dashed", "solid", "double",
	"groove", "ridge", "inset", "outset")

// fitContentFn is fit-content(<length-percentage [0,∞]>).
func fitContentFn(v css.ComponentValue) verdict {
	if !v.IsFunction() || !strings.EqualFold(v.Token.Value, "fit-content") {
		return invalid
	}
	return single(num(lengthPctSlot.nonNeg()))(items(v.Values))
}

// aspectRatio is "auto || <ratio>", where <ratio> is "<number [0,∞]> [ /
// <number [0,∞]> ]?".
func aspectRatio(it []css.ComponentValue) verdict {
	auto := false
	if len(it) > 0 {
		if name, ok := identOf(it[0]); ok && name == "auto" {
			auto, it = true, it[1:]
		} else if name, ok := identOf(it[len(it)-1]); ok && name == "auto" {
			auto, it = true, it[:len(it)-1]
		}
	}
	if len(it) == 0 {
		if auto {
			return valid
		}
		return invalid
	}
	n := num(numberSlot.nonNeg())
	switch len(it) {
	case 1:
		return n(it[0])
	case 3:
		if !it[1].IsToken() || !it[1].Token.IsDelim('/') {
			return invalid
		}
		return n(it[0]).and(n(it[2]))
	}
	return invalid
}

// fontSizeKeyword is <absolute-size> | <relative-size> | math.
func fontSizeKeyword(v css.ComponentValue) verdict {
	name, ok := identOf(v)
	if !ok {
		return invalid
	}
	if _, abs := absoluteFontSizes[name]; abs {
		return valid
	}
	if _, rel := relativeFontSizes[name]; rel {
		return valid
	}
	if name == "math" {
		return valid
	}
	return invalid
}

// fontWeight is <font-weight-absolute> | bolder | lighter, where the absolute
// weights are normal, bold and <number [1,1000]>.
var fontWeight = either(kw("normal", "bold", "bolder", "lighter"),
	num(numeric{number: true, min: 1, hasMin: true, max: 1000, hasMax: true}))

// fontStyle is "normal | italic | left | right | oblique <angle [-90deg,90deg]>?".
func fontStyle(it []css.ComponentValue) verdict {
	if len(it) == 0 {
		return invalid
	}
	name, ok := identOf(it[0])
	if !ok {
		return invalid
	}
	switch name {
	case "normal", "italic", "left", "right":
		if len(it) == 1 {
			return valid
		}
	case "oblique":
		switch len(it) {
		case 1:
			return valid
		case 2:
			return num(numeric{angle: true, min: -90, hasMin: true, max: 90,
				hasMax: true})(it[1])
		}
	}
	return invalid
}

// familyName is one entry of font-family: a string, a run of identifiers, or
// generic().
func familyName(it []css.ComponentValue) verdict {
	if len(it) == 1 {
		if str(it[0]).ok {
			return valid
		}
		if it[0].IsFunction() && strings.EqualFold(it[0].Token.Value, "generic") {
			return valid
		}
	}
	for _, v := range it {
		if !customIdent()(v).ok {
			return invalid
		}
	}
	return valid
}

// featureTag is one entry of font-feature-settings: "<opentype-tag> [ <integer
// [0,∞]> | on | off ]?", where the tag is a string of four printable ASCII
// characters.
func featureTag(it []css.ComponentValue) verdict {
	if len(it) == 0 || len(it) > 2 || !str(it[0]).ok {
		return invalid
	}
	tag := it[0].Token.Value
	if len(tag) != 4 {
		return invalid
	}
	for i := 0; i < 4; i++ {
		if tag[i] < 0x20 || tag[i] > 0x7e {
			return invalid
		}
	}
	if len(it) == 2 {
		return either(kw("on", "off"), num(integerSlot.nonNeg()))(it[1])
	}
	return valid
}

// groups is a "||" of keyword groups, each written at most once: the numeric,
// ligature and East Asian variant grammars, whose groups the font-variant
// shorthand's tables already name.
func groups(of map[string]string) grammar {
	return func(it []css.ComponentValue) verdict {
		if len(it) == 0 {
			return invalid
		}
		seen := map[string]bool{}
		for _, v := range it {
			name, ok := identOf(v)
			if !ok {
				return invalid
			}
			group := of[name]
			if group == "" || seen[group] {
				return invalid
			}
			seen[group] = true
		}
		return valid
	}
}

// anyOrderRequired is "a && b?": the first term once, the second at most once.
func anyOrderRequired(required, optional term) grammar {
	return func(it []css.ComponentValue) verdict {
		switch len(it) {
		case 1:
			return required(it[0])
		case 2:
			if got := required(it[0]).and(optional(it[1])); got.ok {
				return got
			}
			return optional(it[0]).and(required(it[1]))
		}
		return invalid
	}
}

// textIndent is "<length-percentage> && hanging? && each-line?".
func textIndent(it []css.ComponentValue) verdict {
	if len(it) == 0 || len(it) > 3 {
		return invalid
	}
	out := invalid
	seenLength, seenHanging, seenEach := false, false, false
	for _, v := range it {
		name, isIdent := identOf(v)
		switch {
		case isIdent && name == "hanging" && !seenHanging:
			seenHanging = true
		case isIdent && name == "each-line" && !seenEach:
			seenEach = true
		case !seenLength:
			got := num(lengthPctSlot)(v)
			if !got.ok {
				return invalid
			}
			out, seenLength = got, true
		default:
			return invalid
		}
	}
	if !seenLength {
		return invalid
	}
	return out
}

// textTransform is "none | [ capitalize | uppercase | lowercase ] ||
// full-width || full-size-kana | math-auto".
func textTransform(it []css.ComponentValue) verdict {
	if len(it) == 1 {
		if name, ok := identOf(it[0]); ok && (name == "none" || name == "math-auto") {
			return valid
		}
	}
	return anyOrder(kw("capitalize", "uppercase", "lowercase"), kw("full-width"),
		kw("full-size-kana"))(it)
}

// textFit is the grammar layout/textfit.go reads.
func textFit(it []css.ComponentValue) verdict {
	if len(it) == 0 || len(it) > 3 || !kw("none", "grow", "shrink")(it[0]).ok {
		return invalid
	}
	rest := it[1:]
	if len(rest) > 0 && kw("consistent", "per-line", "per-line-all")(rest[0]).ok {
		rest = rest[1:]
	}
	if len(rest) > 0 {
		if !rest[0].IsToken() || rest[0].Token.Kind != css.Percentage {
			return invalid
		}
		rest = rest[1:]
	}
	if len(rest) > 0 {
		return invalid
	}
	return valid
}

// lineClamp is "<integer [1,∞]> || <'block-ellipsis'>" with css-overflow-4's
// -webkit-legacy after it.
func lineClamp(it []css.ComponentValue) verdict {
	if n := len(it); n > 0 {
		if name, ok := identOf(it[n-1]); ok && name == "-webkit-legacy" {
			it = it[:n-1]
		}
	}
	return anyOrder(num(numeric{number: true, integer: true, min: 1, hasMin: true}),
		either(kw("no-ellipsis", "auto"), str))(it)
}

// combineDigits is "digits <integer [2,4]>?".
func combineDigits(it []css.ComponentValue) verdict {
	if len(it) == 0 || len(it) > 2 || !kw("digits")(it[0]).ok {
		return invalid
	}
	if len(it) == 2 {
		return num(numeric{number: true, integer: true, min: 2, hasMin: true,
			max: 4, hasMax: true})(it[1])
	}
	return valid
}

// Box alignment, css-align-3 §4.

var (
	overflowPosition     = kw("unsafe", "safe")
	contentDistribution  = kw("space-between", "space-around", "space-evenly", "stretch")
	contentPosition      = kw("center", "start", "end", "flex-start", "flex-end")
	selfPosition         = kw("center", "start", "end", "self-start", "self-end", "flex-start", "flex-end")
	leftRight            = kw("left", "right")
	leftRightCenter      = kw("left", "right", "center")
	firstLast            = kw("first", "last")
	baselineKeyword      = kw("baseline")
	contentPositionOrLR  = either(contentPosition, leftRight)
	selfPositionOrLR     = either(selfPosition, leftRight)
	selfPositionOrAnchor = either(selfPosition, kw("anchor-center"))
)

// baselinePosition is "[ first | last ]? baseline".
func baselinePosition(it []css.ComponentValue) verdict {
	switch len(it) {
	case 1:
		return baselineKeyword(it[0])
	case 2:
		return firstLast(it[0]).and(baselineKeyword(it[1]))
	}
	return invalid
}

// positioned is "<overflow-position>? pos".
func positioned(pos term) grammar {
	return func(it []css.ComponentValue) verdict {
		switch len(it) {
		case 1:
			return pos(it[0])
		case 2:
			return overflowPosition(it[0]).and(pos(it[1]))
		}
		return invalid
	}
}

var (
	justifyContent = oneOf(single(kw("normal")), single(contentDistribution),
		positioned(contentPositionOrLR))
	alignContent = oneOf(single(kw("normal")), baselinePosition,
		single(contentDistribution), positioned(contentPosition))
	alignItems = oneOf(single(kw("normal", "stretch")), baselinePosition,
		positioned(selfPositionOrAnchor))
	justifySelf = oneOf(single(kw("normal", "stretch")), baselinePosition,
		positioned(selfPositionOrLR), single(kw("anchor-center")))
	justifyItems = oneOf(justifySelf, single(kw("legacy")),
		anyOrderRequired(kw("legacy"), leftRightCenter))
)

// Grid, css-grid-2 §7.

// gridTemplate is "none | <track-list> | <auto-track-list> | subgrid
// <line-name-list>? | masonry". Whether an auto-repeat appears once and only
// beside fixed sizes is left to the reader, which refuses what it cannot place.
func gridTemplate(it []css.ComponentValue) verdict {
	if len(it) == 1 {
		if name, ok := identOf(it[0]); ok && (name == "none" || name == "masonry") {
			return valid
		}
	}
	if len(it) > 0 {
		if name, ok := identOf(it[0]); ok && name == "subgrid" {
			for _, v := range it[1:] {
				if !lineNames(v).ok && !repeatFn(v).ok {
					return invalid
				}
			}
			return valid
		}
	}
	return trackList(it)
}

// trackList is line names, track sizes and repeat()s, with at least one track.
func trackList(it []css.ComponentValue) verdict {
	out, tracks := valid, 0
	for _, v := range it {
		if lineNames(v).ok {
			continue
		}
		got := either(trackSize, repeatFn)(v)
		if !got.ok {
			return invalid
		}
		out = out.and(got)
		tracks++
	}
	if tracks == 0 {
		return invalid
	}
	return out
}

// lineNames is "[ <custom-ident>* ]".
func lineNames(v css.ComponentValue) verdict {
	if !v.IsBlock() || v.Token.Kind != css.LeftSquare {
		return invalid
	}
	for _, n := range items(v.Values) {
		if !customIdent("span", "auto")(n).ok {
			return invalid
		}
	}
	return valid
}

// trackBreadth is "<length-percentage [0,∞]> | <flex [0,∞]> | min-content |
// max-content | auto".
func trackBreadth(v css.ComponentValue) verdict {
	if v.IsToken() && v.Token.Kind == css.Dimension &&
		strings.EqualFold(v.Token.Unit, "fr") {
		if v.Token.Number >= 0 {
			return valid
		}
		return invalid
	}
	return either(kw("min-content", "max-content", "auto"),
		num(lengthPctSlot.nonNeg()))(v)
}

// trackSize is "<track-breadth> | minmax( <inflexible-breadth>,
// <track-breadth> ) | fit-content( <length-percentage [0,∞]> )".
func trackSize(v css.ComponentValue) verdict {
	if got := trackBreadth(v); got.ok {
		return got
	}
	if fitContentFn(v).ok {
		return valid
	}
	if v.IsFunction() && strings.EqualFold(v.Token.Value, "minmax") {
		args := splitOnComma(v.Values)
		if len(args) != 2 {
			return invalid
		}
		a, b := items(args[0]), items(args[1])
		if len(a) != 1 || len(b) != 1 {
			return invalid
		}
		if a[0].IsToken() && a[0].Token.Kind == css.Dimension &&
			strings.EqualFold(a[0].Token.Unit, "fr") {
			// The minimum may not be flexible.
			return invalid
		}
		return trackBreadth(a[0]).and(trackBreadth(b[0]))
	}
	return invalid
}

// repeatFn is "repeat( [ <integer [1,∞]> | auto-fill | auto-fit ] ,
// <track-list> )".
func repeatFn(v css.ComponentValue) verdict {
	if !v.IsFunction() || !strings.EqualFold(v.Token.Value, "repeat") {
		return invalid
	}
	args := splitOnComma(v.Values)
	if len(args) != 2 {
		return invalid
	}
	count := items(args[0])
	if len(count) != 1 || !either(kw("auto-fill", "auto-fit"),
		num(numeric{number: true, integer: true, min: 1, hasMin: true}))(count[0]).ok {
		return invalid
	}
	inner := items(args[1])
	if len(inner) == 0 {
		return invalid
	}
	for _, t := range inner {
		if !lineNames(t).ok && !trackSize(t).ok {
			return invalid
		}
	}
	return valid
}

// gridLine is "auto | <custom-ident> | [ <integer> && <custom-ident>? ] |
// [ span && [ <integer [1,∞]> || <custom-ident> ] ]", where the integer is
// not zero.
func gridLine(it []css.ComponentValue) verdict {
	name := customIdent("span", "auto")
	integer := func(v css.ComponentValue) bool {
		return v.IsToken() && v.Token.Kind == css.Number && v.Token.IsInteger &&
			v.Token.Number != 0
	}
	switch len(it) {
	case 1:
		if kw("auto")(it[0]).ok || name(it[0]).ok || integer(it[0]) {
			return valid
		}
		if m := it[0]; m.IsFunction() {
			return num(integerSlot)(m)
		}
		return invalid
	case 2, 3:
		span, count, ident := 0, 0, 0
		for _, v := range it {
			switch {
			case kw("span")(v).ok:
				span++
			case integer(v):
				if span > 0 && v.Token.Number < 0 {
					return invalid
				}
				count++
			case name(v).ok:
				ident++
			default:
				return invalid
			}
		}
		if span > 1 || count > 1 || ident > 1 {
			return invalid
		}
		if span == 1 {
			// "span" goes first or last, and a negative count does not follow it.
			if !kw("span")(it[0]).ok && !kw("span")(it[len(it)-1]).ok {
				return invalid
			}
			for _, v := range it {
				if integer(v) && v.Token.Number < 0 {
					return invalid
				}
			}
			return valid
		}
		if count == 1 && len(it) == 2 {
			return valid
		}
	}
	return invalid
}

// Generated content, css-content-3 §1.

// content is "normal | none | [ <content-replacement> | <content-list> ] [ /
// [ <string> | <counter> | <attr()> ]+ ]?".
func content(it []css.ComponentValue) verdict {
	if len(it) == 1 {
		if name, ok := identOf(it[0]); ok && (name == "normal" || name == "none") {
			return valid
		}
	}
	list, alt := it, []css.ComponentValue(nil)
	for i, v := range it {
		if v.IsToken() && v.Token.IsDelim('/') {
			list, alt = it[:i], it[i+1:]
			if len(alt) == 0 {
				return invalid
			}
			break
		}
	}
	if len(list) == 0 {
		return invalid
	}
	if !legalCounterFunctions(list) || !legalCounterFunctions(alt) {
		return invalid
	}
	out := valid
	for _, v := range list {
		if out = out.and(contentItem(v)); !out.ok {
			return invalid
		}
	}
	for _, v := range alt {
		if !str(v).ok && !isContentFunction(v, "counter", "counters", "attr") {
			return invalid
		}
	}
	return out
}

// contentItem is one entry of a <content-list>.
func contentItem(v css.ComponentValue) verdict {
	if str(v).ok || image(v).ok {
		return valid
	}
	if name, ok := identOf(v); ok {
		switch name {
		case "open-quote", "close-quote", "no-open-quote", "no-close-quote", "contents":
			return valid
		}
		return invalid
	}
	if isContentFunction(v, "counter", "counters", "attr", "target-counter",
		"target-counters", "target-text", "leader", "string", "content") {
		return valid
	}
	return invalid
}

func isContentFunction(v css.ComponentValue, names ...string) bool {
	if !v.IsFunction() {
		return false
	}
	for _, n := range names {
		if strings.EqualFold(v.Token.Value, n) {
			return true
		}
	}
	return false
}

// counters is css-lists-3 §4's "none | [ <counter-name> <integer>? ]+", with
// counter-reset's reversed(<counter-name>).
func counters(reset bool) grammar {
	return func(it []css.ComponentValue) verdict {
		if len(it) == 1 {
			if name, ok := identOf(it[0]); ok && name == "none" {
				return valid
			}
		}
		if len(it) == 0 {
			return invalid
		}
		name := customIdent("none")
		for i := 0; i < len(it); i++ {
			v := it[i]
			switch {
			case name(v).ok:
			case reset && v.IsFunction() && strings.EqualFold(v.Token.Value, "reversed"):
				inner := items(v.Values)
				if len(inner) != 1 || !name(inner[0]).ok {
					return invalid
				}
			default:
				return invalid
			}
			if i+1 < len(it) && it[i+1].IsToken() && it[i+1].Token.Kind == css.Number {
				if !it[i+1].Token.IsInteger {
					return invalid
				}
				i++
			}
		}
		return valid
	}
}

// symbolsFn is css-counter-styles-3's symbols(), an anonymous counter style.
func symbolsFn(v css.ComponentValue) verdict {
	if v.IsFunction() && strings.EqualFold(v.Token.Value, "symbols") {
		return valid
	}
	return invalid
}

// Backgrounds, css-backgrounds-3 §3.

// repeatStyle is "repeat-x | repeat-y | [ repeat | space | round | no-repeat
// ]{1,2}".
func repeatStyle(it []css.ComponentValue) verdict {
	if len(it) == 1 && kw("repeat-x", "repeat-y")(it[0]).ok {
		return valid
	}
	return repeated(kw("repeat", "space", "round", "no-repeat"), 1, 2)(it)
}

// bgSize is "[ <length-percentage [0,∞]> | auto ]{1,2} | cover | contain".
func bgSize(it []css.ComponentValue) verdict {
	if len(it) == 1 && kw("cover", "contain")(it[0]).ok {
		return valid
	}
	return repeated(either(kw("auto"), num(lengthPctSlot.nonNeg())), 1, 2)(it)
}

// position is css-values-4's <position>, one to four components.
func position(it []css.ComponentValue) verdict {
	lp := num(lengthPctSlot)
	horiz, vert := kw("left", "right"), kw("top", "bottom")
	center := kw("center")
	isX := either(horiz, center)
	isY := either(vert, center)
	switch len(it) {
	case 1:
		return either(horiz, vert, center, lp)(it[0])
	case 2:
		// A keyword pair in either order, or an x then a y where either may
		// be a length.
		if got := either(isX, lp)(it[0]).and(either(isY, lp)(it[1])); got.ok {
			return got
		}
		return isY(it[0]).and(isX(it[1]))
	case 3, 4:
		// "[ center | [ left | right ] <length-percentage>? ] && [ center | [
		// top | bottom ] <length-percentage>? ]".
		first, rest, ok := edgeOffset(it, horiz, vert)
		if !ok {
			return invalid
		}
		second, rest, ok := edgeOffset(rest, horiz, vert)
		if !ok || len(rest) != 0 {
			return invalid
		}
		if first == second && first != "center" {
			return invalid
		}
		return valid
	}
	return invalid
}

// edgeOffset reads "center" or "edge <length-percentage>?" and says which axis
// it was, "x", "y" or "center".
func edgeOffset(it []css.ComponentValue, horiz, vert term) (string, []css.ComponentValue, bool) {
	if len(it) == 0 {
		return "", nil, false
	}
	axis := ""
	switch {
	case kw("center")(it[0]).ok:
		return "center", it[1:], true
	case horiz(it[0]).ok:
		axis = "x"
	case vert(it[0]).ok:
		axis = "y"
	default:
		return "", nil, false
	}
	it = it[1:]
	if len(it) > 0 && num(lengthPctSlot)(it[0]).ok {
		it = it[1:]
	}
	return axis, it, true
}

// rectFn is CSS 2.1 §11.1.2's rect(), whose four offsets are lengths or auto,
// separated by commas or, in the older form, by spaces.
func rectFn(v css.ComponentValue) verdict {
	if !v.IsFunction() || !strings.EqualFold(v.Token.Value, "rect") {
		return invalid
	}
	var parts []css.ComponentValue
	for _, p := range splitOnComma(v.Values) {
		parts = append(parts, items(p)...)
	}
	if len(parts) != 4 {
		return invalid
	}
	return repeated(either(kw("auto"), num(lengthSlot)), 4, 4)(parts)
}
