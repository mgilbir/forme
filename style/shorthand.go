package style

import (
	"math"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
)

// The shorthands whose parts are told apart by *type* rather than by position.
//
// "margin: 1px 2px" is positional: the first value is the top whatever it is.
// "border: 1px solid red" is not — the three may be written in any order, and
// which longhand each belongs to is decided by what it is. A length is the
// width, a keyword from a closed set is the style, anything that parses as a
// colour is the colour.
//
// # Why a shorthand resets what it does not mention
//
// This is the rule that makes shorthands worth having and the one most often got
// wrong. "border: solid" does not only set the style: it sets the width and the
// colour to their initial values as well. So a rule that says "border-width: 5px"
// and then "border: solid" has a *medium* border, not a five-pixel one.
//
// An expander that returned only the parts it saw would leave the others at
// whatever an earlier declaration had set, which is a page that is wrong in a way
// the stylesheet does not explain.

// ident builds a component value for a keyword, which is how an expander says
// "this longhand takes its initial value".
func ident(name string) []css.ComponentValue {
	return []css.ComponentValue{{Token: css.Token{Kind: css.Ident, Value: name}}}
}

// borderShorthand expands "border" and the four per-side forms.
//
// sides is which edges it sets: all four for "border", one for "border-top".
func borderShorthand(sides ...string) expander {
	return func(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
		width, style, colour := ident("medium"), ident("none"), ident("currentcolor")
		var seenWidth, seenStyle, seenColour bool

		for _, part := range splitOnWhitespace(vals) {
			switch {
			case isBorderStyle(part) && !seenStyle:
				style, seenStyle = part, true
			case isBorderWidth(part) && !seenWidth:
				width, seenWidth = part, true
			case isColour(part) && !seenColour:
				colour, seenColour = part, true
			default:
				// A part that is none of the three, or a second of one kind.
				// Either way the declaration is invalid, and an invalid
				// shorthand sets nothing at all rather than the parts that did
				// parse — half a border is not what was asked for.
				return nil, nil, false
			}
		}
		if !seenWidth && !seenStyle && !seenColour {
			return nil, nil, false
		}

		out := map[string][]css.ComponentValue{}
		for _, side := range sides {
			out["border-"+side+"-width"] = width
			out["border-"+side+"-style"] = style
			out["border-"+side+"-color"] = colour
		}
		return out, nil, true
	}
}

// outlineShorthand is CSS 2.1 §18.4's "outline", which is the border shorthand
// with two differences and is written out rather than parameterised over them.
//
// An outline has no sides. It is one width, one style and one colour for the
// whole ring, so there is nothing to expand into four.
//
// "hidden" is not a legal outline style — §18.4 says so in as many words, and it
// is the one border style that is missing here. The word means "this border
// loses to its neighbour" in the collapsing table model, and an outline has no
// neighbours to lose to. And the colour accepts "invert", which no border does.
func outlineShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	width, styleVal, colour := ident("medium"), ident("none"), ident("invert")
	var seenWidth, seenStyle, seenColour bool

	for _, part := range splitOnWhitespace(vals) {
		switch {
		case isOutlineStyle(part) && !seenStyle:
			styleVal, seenStyle = part, true
		case isBorderWidth(part) && !seenWidth:
			width, seenWidth = part, true
		case judgeValue("outline-color", part).ok && !seenColour:
			colour, seenColour = part, true
		default:
			// As for the border: half an outline is not what was asked for.
			return nil, nil, false
		}
	}
	if !seenWidth && !seenStyle && !seenColour {
		return nil, nil, false
	}
	return map[string][]css.ComponentValue{
		"outline-width": width,
		"outline-style": styleVal,
		"outline-color": colour,
	}, nil, true
}

// isOutlineStyle is outline-style's own value: the border styles without
// "hidden", per §18.4, and css-ui-4's "auto".
func isOutlineStyle(part []css.ComponentValue) bool {
	return judgeValue("outline-style", part).ok
}

// isBorderStyle is <line-style>. It and isBorderWidth are two closed sets that
// do not overlap — "none" is a style and "medium" is a width — which is what
// lets "border: none" mean one thing.
func isBorderStyle(part []css.ComponentValue) bool {
	return len(part) == 1 && lineStyle(part[0]).ok
}

// The slot predicates below ask the value grammar's own terms — see
// grammar.go — and take a part the grammar calls valid whether or not this
// engine evaluates it. Which slot a part belongs to is a question about what it
// is, and "calc(1px + 1px)" is a width and "oklch(…)" a colour whatever can be
// done with them. Each used to recognise only what the engine computes, so
// "border: 2px solid oklch(…)" matched no slot, failed as a whole, and was
// reported as the author's mistake; now the part lands in its slot, and the
// cascade judges the longhand it was put in and says the engine is missing it
// (audit C59).

// isBorderWidth is <line-width>.
func isBorderWidth(part []css.ComponentValue) bool {
	return len(part) == 1 && lineWidth(part[0]).ok
}

// isColour is <color>, including "currentcolor", which is a colour the cascade
// resolves and which ParseColor does not read.
func isColour(part []css.ComponentValue) bool {
	return len(part) == 1 && colour(part[0]).ok
}

// backgroundShorthand expands "background".
//
// It is the widest shorthand this engine has: eight longhands, seven of which
// may be written in any order within a layer, and a layer list separated by
// commas on top of that. Three things about its grammar are worth stating
// because each is a place an expander goes quietly wrong.
//
// The colour belongs to the *last* layer only. CSS puts it there because there
// is one background colour however many images are stacked over it, and an
// expander that accepted a colour in any layer would silently let
// "background: red url(a), url(b)" through — a declaration a browser rejects
// whole, so the page it produces would be one no browser shows.
//
// Two <box> values are the origin and then the clip, in that order; one sets
// both. So "background: url(x) content-box" clips to the content box as well as
// starting there, which is not what the two properties' *initial* values do —
// they differ, padding-box against border-box — and is the single most
// surprising line in the grammar.
//
// The size follows the position after a slash, and only there. That is what
// makes the slash load-bearing rather than decorative: "center / cover" and
// "center cover" are a valid declaration and an invalid one.
//
// The reset happens in full for every longhand the declaration does not mention,
// which is what makes "background: red" undo an image an earlier rule set.
func backgroundShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	layers := splitOnComma(vals)
	if len(layers) == 0 {
		return nil, nil, false
	}
	if len(layers) > maxBackgroundLayers {
		// A declaration is untrusted input, and every layer here becomes a
		// painting pass over a box. The bound is far past any design and far
		// short of a stylesheet that asks for a hundred thousand of them.
		return nil, nil, false
	}

	colour := ident("transparent")
	per := map[string][]([]css.ComponentValue){}
	add := func(name string, v []css.ComponentValue) {
		per[name] = append(per[name], v)
	}

	for i, layer := range layers {
		last := i == len(layers)-1
		got, ok := backgroundLayer(layer, last)
		if !ok {
			return nil, nil, false
		}
		if got.colour != nil {
			colour = got.colour
		}
		add("background-image", got.image)
		add("background-repeat", got.repeat)
		add("background-attachment", got.attachment)
		add("background-position", got.position)
		add("background-size", got.size)
		add("background-origin", got.origin)
		add("background-clip", got.clip)
	}

	out := map[string][]css.ComponentValue{"background-color": colour}
	for name, values := range per {
		out[name] = joinOnComma(values...)
	}
	return out, nil, true
}

// maxBackgroundLayers bounds one declaration's layer list.
//
// It is a variable rather than a constant so a test can lower it far enough to
// watch it fire without writing a stylesheet with a thousand commas in it.
var maxBackgroundLayers = 1024

// bgLayer is one layer's worth of longhand values, each already defaulted.
type bgLayer struct {
	image, repeat, attachment, position, size, origin, clip []css.ComponentValue
	// colour is nil unless this layer carried one, which only the last may.
	colour []css.ComponentValue
}

// backgroundLayer reads one comma-separated layer of the shorthand.
//
// last says whether a colour is allowed here. Everything else is identified by
// what it is rather than by where it sits, which is why this is a loop over
// parts with a case per longhand rather than a positional read.
func backgroundLayer(vals []css.ComponentValue, last bool) (bgLayer, bool) {
	out := bgLayer{
		image:      ident("none"),
		repeat:     ident("repeat"),
		attachment: ident("scroll"),
		position:   percentPair(0, 0),
		size:       ident("auto"),
		origin:     ident("padding-box"),
		clip:       ident("border-box"),
	}
	var seenImage, seenRepeat, seenAttachment, seenPosition, seenColour bool
	var boxes int

	parts := splitSlashes(splitOnWhitespace(vals))
	if len(parts) == 0 {
		// An empty layer is "background: ,". Nothing to set and nothing that
		// could have been meant.
		return bgLayer{}, false
	}

	for i := 0; i < len(parts); {
		part := parts[i]
		switch {
		case isBackgroundImage(part) && !seenImage:
			out.image, seenImage = part, true
			i++

		case isRepeatKeyword(part) && !seenRepeat:
			// "repeat-x" and "repeat-y" are whole values rather than per-axis
			// ones, so neither may be half of a pair: "repeat-x no-repeat" is
			// not a repeat style, it is an invalid declaration.
			if i+1 < len(parts) && isRepeatKeyword(parts[i+1]) &&
				!isAxisRepeatKeyword(part) && !isAxisRepeatKeyword(parts[i+1]) {
				out.repeat = joinParts(part, parts[i+1])
				i += 2
			} else {
				out.repeat = part
				i++
			}
			seenRepeat = true

		case isAttachmentKeyword(part) && !seenAttachment:
			out.attachment, seenAttachment = part, true
			i++

		case isBoxKeyword(part):
			switch boxes {
			case 0:
				// One <box> sets both, and the second overrides the clip.
				out.origin, out.clip = part, part
			case 1:
				out.clip = part
			default:
				return bgLayer{}, false
			}
			boxes++
			i++

		case isPositionComponent(part) && !seenPosition:
			j := i
			for j < len(parts) && j-i < 4 && isPositionComponent(parts[j]) {
				j++
			}
			out.position, seenPosition = joinParts(parts[i:j]...), true
			i = j
			if i < len(parts) && isSlash(parts[i]) {
				i++
				k := i
				for k < len(parts) && k-i < 2 && isSizeComponent(parts[k]) {
					k++
				}
				if k == i {
					// A slash with nothing after it that is a size.
					return bgLayer{}, false
				}
				out.size = joinParts(parts[i:k]...)
				i = k
			}

		case last && isColour(part) && !seenColour:
			out.colour, seenColour = part, true
			i++

		default:
			// A part that belongs to no slot, a second of one kind, or a colour
			// in a layer that is not the last. An invalid shorthand sets nothing
			// at all rather than the parts that happened to parse.
			return bgLayer{}, false
		}
	}
	return out, true
}

// splitOnComma divides a value at its top-level commas.
//
// A comma inside a function — rgb(1, 2, 3) — is not a top level one, and
// arrives here inside a single component value rather than as a token, so
// nothing has to be done to skip it.
func splitOnComma(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	start := 0
	for i, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Comma {
			out = append(out, vals[start:i])
			start = i + 1
		}
	}
	return append(out, vals[start:])
}

// joinOnComma is the inverse, used to rebuild one longhand's layer list.
func joinOnComma(parts ...[]css.ComponentValue) []css.ComponentValue {
	var out []css.ComponentValue
	for i, p := range parts {
		if i > 0 {
			out = append(out, css.ComponentValue{Token: css.Token{Kind: css.Comma}})
		}
		out = append(out, p...)
	}
	return out
}

// splitSlashes makes every "/" a part of its own.
//
// A slash is a delimiter rather than whitespace, so "center/cover" arrives as
// one part of three tokens while "center / cover" arrives as three parts. The
// grammar does not distinguish them, so neither does anything after this.
func splitSlashes(parts [][]css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	for _, part := range parts {
		start := 0
		for i, v := range part {
			if !v.IsToken() || !v.Token.IsDelim('/') {
				continue
			}
			if i > start {
				out = append(out, part[start:i])
			}
			out = append(out, part[i:i+1])
			start = i + 1
		}
		if start < len(part) {
			out = append(out, part[start:])
		}
	}
	return out
}

func isSlash(part []css.ComponentValue) bool {
	return len(part) == 1 && part[0].IsToken() && part[0].Token.IsDelim('/')
}

// percentPair builds "x% y%", which is how the position's initial value is
// written.
func percentPair(x, y float64) []css.ComponentValue {
	pct := func(v float64) css.ComponentValue {
		return css.ComponentValue{Token: css.Token{
			Kind: css.Percentage, Number: v, Repr: strconv.FormatFloat(v, 'f', -1, 64),
		}}
	}
	return []css.ComponentValue{
		pct(x), {Token: css.Token{Kind: css.Whitespace}}, pct(y),
	}
}

// isBackgroundImage reports whether a part is an <image> or the keyword that
// stands for none of one.
//
// A gradient is accepted here and refused later, by the stage that would have to
// paint it. That division is deliberate: the shorthand's job is to decide which
// longhand a part belongs to, and "linear-gradient(...)" belongs to
// background-image whether or not anything can draw it. Rejecting it here would
// make the whole declaration invalid, which would throw away the repeat and the
// position the author wrote beside it and report the wrong thing.
func isBackgroundImage(part []css.ComponentValue) bool {
	return len(part) == 1 && either(kw("none"), image)(part[0]).ok
}

func isRepeatKeyword(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "repeat", "repeat-x", "repeat-y", "no-repeat", "space", "round":
		return true
	}
	return false
}

// isAxisRepeatKeyword names the two that stand for a pair and so cannot be one
// half of one.
func isAxisRepeatKeyword(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "repeat-x", "repeat-y":
		return true
	}
	return false
}

func isAttachmentKeyword(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "scroll", "fixed", "local":
		return true
	}
	return false
}

func isBoxKeyword(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "border-box", "padding-box", "content-box":
		return true
	}
	return false
}

func isPositionComponent(part []css.ComponentValue) bool {
	if isIdentPart(part) {
		switch strings.ToLower(part[0].Token.Value) {
		case "left", "right", "top", "bottom", "center":
			return true
		}
		return false
	}
	return isLengthOrPercent(part)
}

func isSizeComponent(part []css.ComponentValue) bool {
	if isIdentPart(part) {
		switch strings.ToLower(part[0].Token.Value) {
		case "auto", "cover", "contain":
			return true
		}
		return false
	}
	return isLengthOrPercent(part)
}

// isLengthOrPercent accepts what a background position or size may be written
// as, which includes a bare zero and nothing else without a unit.
func isLengthOrPercent(part []css.ComponentValue) bool {
	return len(part) == 1 && num(lengthPctSlot)(part[0]).ok
}

func isNone(part []css.ComponentValue) bool {
	return len(part) == 1 && part[0].IsToken() &&
		part[0].Token.Kind == css.Ident &&
		strings.EqualFold(part[0].Token.Value, "none")
}

// listStyleShorthand expands "list-style": a type, a position, and an image.
//
// # The two slots "none" can fill
//
// "none" is a legal value of both list-style-type and list-style-image, and the
// shorthand's grammar gives no way to say which is meant — so the answer comes
// from the rest of the declaration. "list-style: none square" is a square marker
// with no image; "list-style: none url(dot.png)" is that image with no marker;
// "list-style: none" on its own is neither, since the slot it does not take is
// reset to its initial value and that value is none too.
//
// Where there is nothing left for a "none" to be, the declaration is not a
// list-style and §4.2 drops it whole. That is the case the suite's
// list-style-020 is written about: it declares nine of them in a row — two nones
// beside a type, two beside an image, one beside both — and asks for the
// inherited marker to survive every one.
func listStyleShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	var kind, position, image []css.ComponentValue
	nones := 0
	var unsupported []string

	for _, part := range splitOnWhitespace(vals) {
		switch {
		case isListPosition(part):
			if position != nil {
				return nil, nil, false
			}
			position = part
		case isURLPart(part):
			if image != nil {
				return nil, nil, false
			}
			image = part
		case isNone(part):
			nones++
		case isIdentPart(part):
			if kind != nil {
				return nil, nil, false
			}
			kind = part
		default:
			unsupported = append(unsupported, serialize(part))
		}
	}

	// How many of the two slots a "none" could fill, and whether the
	// declaration wrote more of them than there is room for.
	free := 0
	if kind == nil {
		free++
	}
	if image == nil {
		free++
	}
	if nones > free {
		return nil, nil, false
	}
	// "list-style: none" on its own is the value an author actually writes, and
	// it takes both slots: the one none becomes the type, and the image is reset
	// to its initial value, which is none as well. There is no second assignment
	// to make — writing one was tried, and planting its removal changed no
	// answer, because the reset below already says it.
	if nones > 0 && kind == nil {
		kind = ident("none")
		nones--
	}
	if nones > 0 && image == nil {
		image = ident("none")
		nones--
	}

	// The shorthand resets what it does not mention, so an omitted longhand
	// takes its initial value rather than keeping what the cascade had.
	if kind == nil {
		kind = ident("disc")
	}
	if position == nil {
		position = ident("outside")
	}
	if image == nil {
		image = ident("none")
	}
	return map[string][]css.ComponentValue{
		"list-style-type":     kind,
		"list-style-position": position,
		"list-style-image":    image,
	}, unsupported, true
}

// isURLPart is a single url() value, in either of the two shapes the tokenizer
// produces: a URL token for url(x) and a function for url("x").
func isURLPart(part []css.ComponentValue) bool {
	if len(part) != 1 {
		return false
	}
	v := part[0]
	if v.IsToken() && v.Token.Kind == css.URL {
		return true
	}
	return v.IsFunction() && strings.EqualFold(v.Token.Value, "url")
}

func isListPosition(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "inside", "outside":
		return true
	}
	return false
}

func isIdentPart(part []css.ComponentValue) bool {
	return len(part) == 1 && part[0].IsToken() && part[0].Token.Kind == css.Ident
}

// fontShorthand expands "font".
//
// Unlike the others this one *is* positional at the end: the size, an optional
// line-height after a slash, and then the family list, in that order. What comes
// before is CSS Fonts 4 §2.8's four optional parts in any order — the style,
// the CSS 2.1 variant (small-caps), the weight and the width — which is why it
// is here rather than with the box shorthands:
//
//	[ <'font-style'> || <font-variant-css2> || <'font-weight'> ||
//	  <font-width-css3> ]? <'font-size'> [ / <'line-height'> ]? <'font-family'>#
//
// The width (font-stretch in CSS 3, font-width in 4) is valid and this engine
// has no such property, so it is reported as a part it cannot produce and the
// rest is applied. It was not accepted at all, and nor was a numeric weight
// other than the nine hundreds or an oblique angle, so "font: condensed 12px
// serif", "font: 450 12px serif" and "font: oblique 10deg 12px serif" — valid
// declarations every browser applies — were dropped whole and called the
// author's mistake (audit C112).
//
// # What it resets
//
// §2.8 resets every subproperty to its initial value first, "including those
// listed above plus font-size-adjust, font-kerning, all subproperties of
// font-variant, font-feature-settings, font-language-override,
// font-optical-sizing, font-variation-settings and font-palette" — of which
// this engine has font-kerning, font-feature-settings and the five variant
// longhands. It set six longhands, so "font: 12px serif" left an earlier
// "font-variant-numeric: oldstyle-nums" and "font-kerning: none" in force, and
// "font: inherit" inherited only six of them.
func fontShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	parts := splitOnWhitespace(vals)
	if len(parts) == 0 {
		return nil, nil, false
	}
	// The system-font keywords set every part at once from something this
	// engine has no access to.
	if len(parts) == 1 && isIdentPart(parts[0]) {
		switch strings.ToLower(parts[0][0].Token.Value) {
		case "caption", "icon", "menu", "message-box", "small-caption", "status-bar":
			return nil, []string{"the system font " + serialize(parts[0])}, false
		}
	}

	style, weight, caps := ident("normal"), ident("normal"), ident("normal")
	var size, lineHeight, family []css.ComponentValue
	var unsupported []string
	var seenStyle, seenWeight, seenCaps, seenWidth bool
	normals := 0

	i := 0
	for ; i < len(parts); i++ {
		part := parts[i]
		if isFontSize(part) {
			break
		}
		name, isIdent := singleIdent(part)
		switch {
		case isIdent && name == "normal":
			// The initial value of every slot the prefix can set, so it says
			// nothing and changes nothing — but it does fill a slot, which is
			// what limits the prefix to four.
			normals++
		case isIdent && !seenStyle && (name == "italic" || name == "oblique"):
			style, seenStyle = part, true
			if name == "oblique" && i+1 < len(parts) &&
				num(numeric{angle: true, min: -90, hasMin: true, max: 90,
					hasMax: true})(parts[i+1][0]).ok && len(parts[i+1]) == 1 {
				style = joinParts(part, parts[i+1])
				i++
			}
		case isIdent && !seenCaps && name == "small-caps":
			// The one font-variant value "font" can carry: §2.8 lets the
			// shorthand take a caps keyword and nothing else from the variant
			// group. It sets the longhand, and — like every other slot here —
			// the shorthand resets it to "normal" when it is not written, which
			// is what makes "font: 12px serif" undo an inherited small-caps.
			caps, seenCaps = part, true
		case !seenWeight && len(part) == 1 && fontWeight(part[0]).ok:
			weight, seenWeight = part, true
		case isIdent && !seenWidth && fontWidthKeywords[name]:
			seenWidth = true
			if name != "normal" {
				unsupported = append(unsupported, "the font width "+name)
			}
		default:
			return nil, nil, false
		}
	}
	filled := normals
	for _, seen := range []bool{seenStyle, seenWeight, seenCaps, seenWidth} {
		if seen {
			filled++
		}
	}
	if filled > 4 {
		return nil, nil, false
	}
	if i >= len(parts) {
		// No size, so this is not a font shorthand at all — the size and the
		// family are the two required parts.
		return nil, nil, false
	}

	size = parts[i]
	// A line-height is "size / height", and CSS allows white space on either
	// side of the slash — so the three tokens arrive as one part, two or three
	// depending on where the author put the spaces:
	//
	//	font: 12px/1 monospace      one:   [12px/1]
	//	font: 12px/ 1 monospace     two:   [12px/] [1]
	//	font: 12px / 1 monospace    three: [12px] [/] [1]
	//
	// Only the first two were tried, and splitOnSlash refuses a part whose
	// slash is last — so the spaced form matched nothing and the whole of
	// "/ 1 monospace" became the family. The suite writes it that way, and a
	// font-family of "/ 1 monospace" matches no face at all.
	for span := 1; span <= 3 && i+span <= len(parts); span++ {
		if s, h, ok := splitOnSlash(joinParts(parts[i : i+span]...)); ok {
			size, lineHeight = s, h
			i += span - 1
			break
		}
	}
	i++
	if i >= len(parts) {
		return nil, nil, false
	}
	family = joinParts(parts[i:]...)

	out := map[string][]css.ComponentValue{
		"font-style":        style,
		"font-weight":       weight,
		"font-size":         size,
		"font-family":       family,
		"font-variant-caps": caps,
		// Reset whether or not anything was said about them — see above.
		"font-variant-ligatures":  ident("normal"),
		"font-variant-numeric":    ident("normal"),
		"font-variant-east-asian": ident("normal"),
		"font-variant-position":   ident("normal"),
		"font-kerning":            ident("auto"),
		"font-feature-settings":   ident("normal"),
	}
	// The shorthand resets line-height whether or not it was written, which is
	// what makes "font: 12px serif" undo an inherited one.
	if lineHeight != nil {
		out["line-height"] = lineHeight
	} else {
		out["line-height"] = ident("normal")
	}
	return out, unsupported, true
}

// fontWidthKeywords is CSS Fonts 4 §2.8's <font-width-css3>.
var fontWidthKeywords = map[string]bool{
	"normal": true, "ultra-condensed": true, "extra-condensed": true,
	"condensed": true, "semi-condensed": true, "semi-expanded": true,
	"expanded": true, "extra-expanded": true, "ultra-expanded": true,
}

// isFontSize reports whether a part begins with a font-size: a keyword, a
// length or percentage, or a math function. A part may carry the "/" and the
// line-height after the size, so only its first component is asked.
func isFontSize(part []css.ComponentValue) bool {
	return len(part) > 0 && either(fontSizeKeyword, num(lengthPctSlot))(part[0]).ok
}

func isNumberPart(part []css.ComponentValue) bool {
	return len(part) == 1 && num(numberSlot)(part[0]).ok
}

// splitOnSlash divides "12px/1.5" into its two halves.
func splitOnSlash(part []css.ComponentValue) (before, after []css.ComponentValue, ok bool) {
	for i, v := range part {
		if v.IsToken() && v.Token.IsDelim('/') {
			if i == 0 || i == len(part)-1 {
				return nil, nil, false
			}
			return part[:i], part[i+1:], true
		}
	}
	return nil, nil, false
}

func joinParts(parts ...[]css.ComponentValue) []css.ComponentValue {
	var out []css.ComponentValue
	for i, p := range parts {
		if i > 0 {
			out = append(out, css.ComponentValue{Token: css.Token{Kind: css.Whitespace}})
		}
		out = append(out, p...)
	}
	return out
}

// textDecorationShorthand expands "text-decoration": the lines, a colour and a
// thickness.
//
// The line part is a *set* rather than a single keyword — "text-decoration:
// underline overline" is one declaration asking for two lines — so the keywords
// are gathered rather than the first one taken. An earlier version kept only the
// first and reported the second as a part it could not produce, which was a
// finding about the engine's own reading rather than about anything unimplemented.
//
// "none" cannot be combined with anything, and a repeated keyword is not a valid
// value either; both make the whole declaration invalid, which sets nothing at
// all rather than the parts that happened to parse.
func textDecorationShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	var lines []css.ComponentValue
	colour := ident("currentcolor")
	thickness := ident("auto")
	var seenNone, seenColour, seenThickness bool
	seen := map[string]bool{}
	var unsupported []string

	for _, part := range splitOnWhitespace(vals) {
		switch {
		case isDecorationLine(part):
			name := strings.ToLower(part[0].Token.Value)
			if seen[name] || seenNone || (name == "none" && len(lines) > 0) {
				return nil, nil, false
			}
			seen[name] = true
			seenNone = name == "none"
			if len(lines) > 0 {
				lines = append(lines, css.ComponentValue{
					Token: css.Token{Kind: css.Whitespace},
				})
			}
			lines = append(lines, part...)
		case isColour(part) && !seenColour:
			colour, seenColour = part, true
		case isDecorationThickness(part) && !seenThickness:
			// §2.2's thickness, which is part of the shorthand in CSS Text
			// Decoration 4. Without this a declaration that named one — "text-
			// decoration: underline 2px" — was not a declaration this parser
			// recognised at all, so the underline went with the thickness.
			thickness, seenThickness = part, true
		case isIdentPart(part):
			if isInertDeclaration("text-decoration-style", part) {
				// The style component at its own initial value, which is the
				// line this engine draws. It is the same rule inert.go applies
				// to a whole declaration and for the same reason: an engine that
				// does not implement a property renders as though nobody had
				// said anything about it, and "solid" is what nobody saying
				// anything means. Reporting it told an author their underline
				// was dropped when it is on the page and is the line they asked
				// for — text-transform-capitalize-035 writes "text-decoration:
				// underline solid" and is a document about capitalisation.
				continue
			}
			// A keyword this engine understood as belonging to the shorthand and
			// cannot produce: "blink", or one of the CSS Text Decoration 3 styles
			// such as "wavy".
			unsupported = append(unsupported, serialize(part))
		default:
			return nil, nil, false
		}
	}
	if len(lines) == 0 {
		// The shorthand resets what it does not mention, so a declaration that
		// named only a colour still turns the lines off.
		lines = ident("none")
	}
	return map[string][]css.ComponentValue{
		"text-decoration-line":      lines,
		"text-decoration-color":     colour,
		"text-decoration-thickness": thickness,
	}, unsupported, true
}

// isDecorationThickness reports whether one part of the shorthand is a
// thickness: the two keywords, or a length or a percentage.
func isDecorationThickness(part []css.ComponentValue) bool {
	return len(part) == 1 && either(kw("auto", "from-font"), num(lengthPctSlot))(part[0]).ok
}

func isDecorationLine(part []css.ComponentValue) bool {
	if !isIdentPart(part) {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "none", "underline", "overline", "line-through":
		return true
	}
	return false
}

// whiteSpaceShorthand is CSS Text 4 §3's white-space: one of the legacy
// keywords, or its longhands' own values in any order.
//
//	normal | pre | pre-wrap | pre-line |
//	<'white-space-collapse'> || <'text-wrap-mode'> || <'white-space-trim'>
//
// The legacy keywords are a table:
//
//	white-space-collapse | text-wrap-mode
//	normal        collapse         wrap
//	pre           preserve         nowrap
//	nowrap        collapse         nowrap
//	pre-wrap      preserve         wrap
//	pre-line      preserve-breaks  wrap
//	break-spaces  break-spaces     wrap
//
// The longhand form — "white-space: preserve nowrap" — was refused on the
// grounds that nothing wrote it and that telling two idents apart would be
// inventing a grammar. It is the specification's grammar, the three keyword
// sets do not overlap, and the suite writes "preserve-breaks nowrap"; once the
// value grammar called a refused value the author's mistake, refusing it was a
// false report as well as a dropped declaration.
//
// white-space-trim is valid and this engine does not trim, so a trim keyword
// is reported as a part it cannot produce and the rest is applied; "none" is
// that property's initial value and asks for nothing.
func whiteSpaceShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	set := func(collapse, mode string) map[string][]css.ComponentValue {
		return map[string][]css.ComponentValue{
			"white-space-collapse": ident(collapse),
			"text-wrap-mode":       ident(mode),
		}
	}
	if name, ok := singleIdent(vals); ok {
		switch name {
		case "normal":
			return set("collapse", "wrap"), nil, true
		case "pre":
			return set("preserve", "nowrap"), nil, true
		case "nowrap":
			return set("collapse", "nowrap"), nil, true
		case "pre-wrap":
			return set("preserve", "wrap"), nil, true
		case "pre-line":
			return set("preserve-breaks", "wrap"), nil, true
		case "break-spaces":
			return set("break-spaces", "wrap"), nil, true
		}
	}
	collapse, mode := "", ""
	trim := map[string]bool{}
	var unsupported []string
	for _, part := range splitOnWhitespace(vals) {
		name, ok := singleIdent(part)
		if !ok {
			return nil, nil, false
		}
		switch {
		case collapse == "" && kw("collapse", "discard", "preserve", "preserve-breaks",
			"preserve-spaces", "break-spaces")(part[0]).ok:
			collapse = name
		case mode == "" && (name == "wrap" || name == "nowrap"):
			mode = name
		case name == "none" && len(trim) == 0:
			trim[name] = true
		case (name == "discard-before" || name == "discard-after" ||
			name == "discard-inner") && !trim[name] && !trim["none"]:
			trim[name] = true
			unsupported = append(unsupported, "the white-space-trim value "+name)
		default:
			return nil, nil, false
		}
	}
	if collapse == "" {
		collapse = "collapse"
	}
	if mode == "" {
		mode = "wrap"
	}
	return set(collapse, mode), unsupported, true
}

// textWrapShorthand is "<'text-wrap-mode'> || <'text-wrap-style'>": either, or
// both in either order.
//
// Whichever is absent is reset to its initial value, which is the rule the note
// at the top of this file is about — "text-wrap: balance" after "text-wrap:
// nowrap" wraps, because the shorthand set the mode back to wrap.
func textWrapShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	mode, style := "", ""
	for _, part := range splitOnWhitespace(vals) {
		name, ok := singleIdent(part)
		if !ok {
			return nil, nil, false
		}
		switch name {
		case "wrap", "nowrap":
			if mode != "" {
				return nil, nil, false
			}
			mode = name
		case "auto", "balance", "stable", "pretty":
			if style != "" {
				return nil, nil, false
			}
			style = name
		default:
			return nil, nil, false
		}
	}
	if mode == "" && style == "" {
		return nil, nil, false
	}
	if mode == "" {
		mode = "wrap"
	}
	if style == "" {
		style = "auto"
	}
	return map[string][]css.ComponentValue{
		"text-wrap-mode":  ident(mode),
		"text-wrap-style": ident(style),
	}, nil, true
}

// CSS Fonts 4 §6.10's "font-variant", for the longhands this engine has.
//
// The property is a shorthand for seven, and five of them are here:
// font-variant-ligatures, font-variant-caps, font-variant-numeric,
// font-variant-east-asian and font-variant-position. What is left is
// font-variant-alternates, whose values are mostly functional notations, and
// font-variant-emoji, whose three keywords the shorthand's grammar has taken on
// in the current draft. A declaration naming one of their values is refused
// whole and reported as an unsupported property — the right answer for a value
// nothing downstream can act on, and the same answer the property got as a whole
// until small capitals were implemented.
//
// # Why it is a shorthand at all rather than a keyword this reads
//
// Because of the reset. "font-variant: small-caps" sets font-variant-ligatures
// back to "normal", and a document that turns ligatures off on a paragraph and
// then writes "font-variant: small-caps" on a span inside it gets its ligatures
// back. A reader of the value on its own cannot produce that; only a shorthand
// with a declared longhand list can.
//
// # "none" is not "normal"
//
// §6.10 defines the bare "none" as font-variant-ligatures: none with everything
// else at its initial value — so it turns off the ligatures and does *not* turn
// off small capitals, which were not on. It is valid only on its own, which is
// why it is taken before the loop.
func fontVariantShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	parts := splitOnWhitespace(vals)
	if len(parts) == 0 {
		return nil, nil, false
	}
	// "normal" and "none" are the two values that may only be written alone, so
	// they are taken before the loop. Anything else with one part goes through
	// it like the rest — a lone "sub" is a value of a longhand this engine has
	// not got and has to be reported as one, and a lone "styleset()" is not an
	// ident at all.
	if name, ok := singleIdent(vals); ok {
		switch name {
		case "normal":
			return fontVariantLonghands(ident("normal"), ident("normal"),
				ident("normal"), ident("normal"), ident("normal")), nil, true
		case "none":
			return fontVariantLonghands(ident("none"), ident("normal"),
				ident("normal"), ident("normal"), ident("normal")), nil, true
		}
	}

	var (
		caps        []css.ComponentValue
		ligWords    [][]css.ComponentValue
		seenLig     = map[string]bool{}
		numWords    [][]css.ComponentValue
		seenNum     = map[string]bool{}
		eastWords   [][]css.ComponentValue
		seenEast    = map[string]bool{}
		position    []css.ComponentValue
		unsupported []string
	)
	for _, part := range parts {
		name, ok := singleIdent(part)
		if !ok {
			// A functional notation — stylistic(), styleset(), swash() and the
			// rest of font-variant-alternates. Valid CSS for a longhand this
			// engine has not got, which is the same answer as the keywords
			// below and not "the author wrote something wrong".
			if fn, isFn := variantFunction(part); isFn {
				unsupported = append(unsupported, fn)
				continue
			}
			return nil, nil, false
		}
		switch {
		case fontVariantOtherKeywords[name]:
			// A value of one of the five longhands this engine does not have.
			// The declaration is refused whole rather than partly applied: a
			// shorthand sets every longhand it controls, and setting two of
			// seven from a value that names a third is a page whose styling is
			// neither what was asked for nor what the cascade would produce.
			unsupported = append(unsupported, name)
		case name == "sub" || name == "super":
			// §6.5 is one group of two, so it needs no map: a value naming both
			// asks for the same character above and below the line at once.
			if position != nil {
				return nil, nil, false
			}
			position = part
		case fontVariantCapsKeywords[name]:
			// §6.10 takes one value from the caps group and no more: the six
			// name six different things to do to the same letters.
			if caps != nil {
				return nil, nil, false
			}
			caps = part
		case ligatureKeywordGroup[name] != "":
			// The four ligature pairs are independent of each other and each
			// may be written once. "common-ligatures no-common-ligatures" is
			// the pair written twice and is not a value.
			group := ligatureKeywordGroup[name]
			if seenLig[group] {
				return nil, nil, false
			}
			seenLig[group] = true
			ligWords = append(ligWords, part)
		case numericKeywordGroup[name] != "":
			// §6.7's three pairs and two lone keywords, by the same rule: each
			// group may be written once. "ordinal" and "slashed-zero" are
			// groups of one, which is what makes "ordinal ordinal" invalid the
			// same way "lining-nums oldstyle-nums" is.
			group := numericKeywordGroup[name]
			if seenNum[group] {
				return nil, nil, false
			}
			seenNum[group] = true
			numWords = append(numWords, part)
		case eastAsianKeywordGroup[name] != "":
			// §6.9's three groups, by the same rule: the six national forms are
			// alternatives to each other, the two widths are a pair, and "ruby"
			// is a group of one.
			group := eastAsianKeywordGroup[name]
			if seenEast[group] {
				return nil, nil, false
			}
			seenEast[group] = true
			eastWords = append(eastWords, part)
		default:
			return nil, nil, false
		}
	}
	// Only once the whole value has been read, and only if the rest of it was
	// well formed. A declaration this engine had a longhand for *and* could not
	// parse is a mistake the author made, and "\"font-variant: oldstyle-nums
	// bogus\" is not a value this engine can read" is the report for it —
	// naming the part that is merely unimplemented would suppress that one and
	// send the author looking for a missing feature instead of a typo.
	if len(unsupported) > 0 {
		return nil, unsupported, false
	}
	lig, numeric, east := ident("normal"), ident("normal"), ident("normal")
	if len(ligWords) > 0 {
		// Kept in the order they were written, which is the order
		// font-variant-ligatures' own grammar puts them in.
		lig = joinParts(ligWords...)
	}
	if len(numWords) > 0 {
		numeric = joinParts(numWords...)
	}
	if len(eastWords) > 0 {
		east = joinParts(eastWords...)
	}
	if caps == nil {
		caps = ident("normal")
	}
	if position == nil {
		position = ident("normal")
	}
	return fontVariantLonghands(lig, caps, numeric, east, position), nil, true
}

// variantFunction reads one of font-variant-alternates' functional notations.
//
// They are told apart from a keyword by being a function at all: nothing else in
// the property's grammar is one, so the name does not have to be checked against
// a list to know it belongs to a longhand this engine has not got.
func variantFunction(part []css.ComponentValue) (string, bool) {
	for _, v := range part {
		if v.IsFunction() {
			return strings.ToLower(v.Token.Value) + "()", true
		}
	}
	return "", false
}

// fontVariantOtherKeywords is every ident value of the two longhands
// "font-variant" controls that this engine does not have: font-variant-emoji's
// three, and font-variant-alternates' one keyword — the rest of that property
// being functional notations, which variantFunction reads.
//
// It is written out rather than left to the default branch because the two
// answers differ. A value in this list is correct CSS the engine cannot produce
// — the unsupported-property finding, which §7.1's companion signal counts — and
// a value outside it is a declaration the author got wrong, which is a different
// report and not that one.
var fontVariantOtherKeywords = map[string]bool{
	// font-variant-alternates §6.8. The rest of it is functional notations,
	// which variantFunction reads.
	"historical-forms": true,
	// font-variant-emoji, whose three keywords the shorthand's grammar has
	// taken on in the current draft. They ask which presentation a character
	// with two — a text one and an emoji one — is drawn in, which is a choice
	// between glyphs of *different fonts* rather than a feature of one, and so
	// is not the kind of request the rest of this family makes.
	"text": true, "emoji": true, "unicode": true,
}

// fontVariantLonghands is the set the shorthand always sets, written once so
// that the reset cannot be forgotten on one of the branches above.
func fontVariantLonghands(lig, caps, numeric, east, position []css.ComponentValue) map[string][]css.ComponentValue {
	return map[string][]css.ComponentValue{
		"font-variant-ligatures":  lig,
		"font-variant-caps":       caps,
		"font-variant-numeric":    numeric,
		"font-variant-east-asian": east,
		"font-variant-position":   position,
	}
}

// eastAsianKeywordGroup maps §6.9's nine keywords to the group each belongs to.
//
// Three groups, and the first of them is six alternatives rather than a pair:
// the Japanese standards were four revisions of one thing, so a document naming
// two of them is asking for the same ideograph in two shapes.
var eastAsianKeywordGroup = map[string]string{
	"jis78": "variant", "jis83": "variant", "jis90": "variant", "jis04": "variant",
	"simplified": "variant", "traditional": "variant",
	"full-width": "width", "proportional-width": "width",
	"ruby": "ruby",
}

// numericKeywordGroup maps §6.7's eight keywords to the group each belongs to,
// so that the shorthand can refuse a group written twice.
//
// Five groups: three pairs and two keywords that stand alone. The lone two are
// groups of their own rather than ungrouped, which is what makes "ordinal
// ordinal" invalid by the same rule that refuses "lining-nums oldstyle-nums" —
// §6.7's grammar is a "||" of five terms, and a term may appear once.
var numericKeywordGroup = map[string]string{
	"lining-nums": "figure", "oldstyle-nums": "figure",
	"proportional-nums": "spacing", "tabular-nums": "spacing",
	"diagonal-fractions": "fraction", "stacked-fractions": "fraction",
	"ordinal":      "ordinal",
	"slashed-zero": "slashed-zero",
}

// fontVariantCapsKeywords is CSS Fonts 4 §6.6's closed set.
//
// All six are accepted here and all six are applied, by asking the face for the
// features each of them names. Whether that produced the page the document asked
// for is a question about the face rather than about the declaration, so it is
// asked in layout and not here. See layout/fontfeatures.go.
var fontVariantCapsKeywords = map[string]bool{
	"small-caps": true, "all-small-caps": true, "petite-caps": true,
	"all-petite-caps": true, "unicase": true, "titling-caps": true,
}

// ligatureKeywordGroup maps §6.4's eight keywords to the pair each belongs to,
// so that the shorthand can refuse a pair written twice.
//
// "none" is not among them: it is the whole property's value rather than one of
// the four pairs, and inside "font-variant" it is not a value at all.
var ligatureKeywordGroup = map[string]string{
	"common-ligatures": "common", "no-common-ligatures": "common",
	"discretionary-ligatures":    "discretionary",
	"no-discretionary-ligatures": "discretionary",
	"historical-ligatures":       "historical", "no-historical-ligatures": "historical",
	"contextual": "contextual", "no-contextual": "contextual",
}

// singleIdent reads a value that is exactly one keyword, ignoring the whitespace
// either side of it.
func singleIdent(vals []css.ComponentValue) (string, bool) {
	name := ""
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		if !v.IsToken() || v.Token.Kind != css.Ident || name != "" {
			return "", false
		}
		name = strings.ToLower(v.Token.Value)
	}
	return name, name != ""
}

// textAlignShorthand is CSS Text 4 §7.1's table for the property that used to be
// one keyword.
//
//	                text-align-all | text-align-last
//	start etc.      the value        auto
//	justify-all     justify          justify
//	match-parent    match-parent     match-parent
//
// The first row is the ordinary case and the reason the split is worth having at
// all: "text-align: center" says nothing about the last line, so the last line
// goes back to following the rest.
//
// The second is where "justify-all" comes from. "text-align: justify" leaves the
// last line short, which is what a justified paragraph looks like; an author who
// wants the last line stretched too has to say so, and this is the spelling. It
// used to be a keyword layout looked for in a second place, and expanding it
// here is the same tidying the note at the top of this file is about — the value
// stops being something two readers have to agree about.
//
// The third is the one the suite tests and the one a table alone would get
// wrong. "match-parent" resolves against the parent, and setting the last line
// to "auto" would make it follow text-align-all instead — so an author who wrote
// "text-align: match-parent" and then overrode text-align-all would lose the
// match on the last line, which is exactly what text-align-match-parent-05 is
// built to catch: it sets the two on the same element and asks for the last line
// to stay matched.
func textAlignShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	name, ok := singleIdent(vals)
	if !ok {
		return nil, nil, false
	}
	all, last := name, "auto"
	switch name {
	case "start", "end", "left", "right", "center", "justify":
	case "justify-all":
		all, last = "justify", "justify"
	case "match-parent":
		last = "match-parent"
	default:
		return nil, nil, false
	}
	return map[string][]css.ComponentValue{
		"text-align-all":  ident(all),
		"text-align-last": ident(last),
	}, nil, true
}

// flexShorthand is CSS Flexible Box Layout §7.1's "flex".
//
// Its grammar is "none | [ <'flex-grow'> <'flex-shrink'>? || <'flex-basis'> ]",
// and the part worth writing down is that the shorthand's own defaults are not
// the longhands' initial values. "flex: 1" is "1 1 0%", not "1 1 auto" — an
// omitted basis in the shorthand is *zero*, so a row of "flex: 1" items comes
// out in equal parts however long their text is, which is the thing people
// reach for the shorthand to get. Setting the longhands by hand gives the other
// answer, and §7.1 says so in as many words: "the shorthand resets any omitted
// components to values other than their initial value".
//
// The zero is a percentage, "0%", and not the "0px" it was here. §7.1 writes
// it as a bare "0", and every browser expands it to "0%" — that is what
// getComputedStyle reports for "flex: 1" in Chrome, Firefox and Safari — and
// the two are not the same value. A percentage basis against a main size that
// is indefinite is "content" (§7.2.3), so a "flex: 1" pane in a column that
// was never told how tall to be is as tall as what it holds; "0px" is zero
// there, and the pane collapsed to nothing with its text clipped away or drawn
// over the next block (audit C37). Where the main size is definite — a row, or
// a column with a height — 0% of it is 0 and nothing changes.
//
// "initial" and "auto" are named here rather than left to the CSS-wide keyword
// machinery, because only one of them is a CSS-wide keyword: "flex: auto" is a
// value of this property meaning "1 1 auto", and "flex: initial" — which is
// "0 1 auto" — is the keyword, whose expansion to the longhands' own initials
// happens to be the same thing.
func flexShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	parts := splitOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 3 {
		return nil, nil, false
	}
	set := func(growN, shrinkN float64, grow, shrink, basis string) (map[string][]css.ComponentValue, []string, bool) {
		return map[string][]css.ComponentValue{
			"flex-grow":   number(growN, grow),
			"flex-shrink": number(shrinkN, shrink),
			"flex-basis":  ident(basis),
		}, nil, true
	}
	if len(parts) == 1 && len(parts[0]) == 1 && parts[0][0].IsToken() &&
		parts[0][0].Token.Kind == css.Ident {
		switch strings.ToLower(parts[0][0].Token.Value) {
		case "none":
			return set(0, 0, "0", "0", "auto")
		case "auto":
			return set(1, 1, "1", "1", "auto")
		case "content":
			return set(1, 1, "1", "1", "content")
		}
	}

	grow, shrink := number(1, "1"), number(1, "1")
	var basis []css.ComponentValue
	var seenGrow, seenShrink, seenBasis bool
	for _, part := range parts {
		switch {
		case isNumberPart(part) && !seenBasis && !seenGrow:
			grow, seenGrow = part, true
		case isNumberPart(part) && seenGrow && !seenShrink:
			shrink, seenShrink = part, true
		case !seenBasis && (isLengthOrPercent(part) || isFlexBasisKeyword(part)):
			basis, seenBasis = part, true
		default:
			return nil, nil, false
		}
	}
	if !seenBasis {
		// §7.1's reset: an omitted basis is zero and not "auto" — written as
		// the browsers write it. See the comment above.
		basis = zeroPercent()
	}
	return map[string][]css.ComponentValue{
		"flex-grow": grow, "flex-shrink": shrink, "flex-basis": basis,
	}, nil, true
}

// isFlexBasisKeyword accepts the two identifiers a flex-basis may be.
func isFlexBasisKeyword(part []css.ComponentValue) bool {
	if len(part) != 1 || !part[0].IsToken() || part[0].Token.Kind != css.Ident {
		return false
	}
	switch strings.ToLower(part[0].Token.Value) {
	case "auto", "content":
		return true
	}
	return false
}

// number and zeroPercent are the two literals the expansion above writes.
//
// A numeric token carries its value in Number and its text in Repr, and Value is
// empty for one — which is the trap here, because a token built with Value set
// and Number left at zero parses as nothing and serialises as nothing. It cost
// an afternoon: "flex: 1" expanded to a flex-basis that would not parse, every
// item fell back to "auto", and a row of "flex: 1" items came out sized from
// their text with the free space shared on top. The arithmetic was right and the
// input to it was not.
func number(v float64, repr string) []css.ComponentValue {
	return []css.ComponentValue{{Token: css.Token{
		Kind: css.Number, Number: v, Repr: repr, IsInteger: v == math.Trunc(v),
	}}}
}

func zeroPercent() []css.ComponentValue {
	return []css.ComponentValue{{Token: css.Token{
		Kind: css.Percentage, Number: 0, Repr: "0", IsInteger: true,
	}}}
}

// flexFlowShorthand is §5.1's "flex-flow", which is the two properties that
// decide the axis and whether it wraps. They are told apart by their keywords,
// which do not overlap.
func flexFlowShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	direction, wrap := ident("row"), ident("nowrap")
	var seenDirection, seenWrap bool
	for _, part := range splitOnWhitespace(vals) {
		if len(part) != 1 || !part[0].IsToken() || part[0].Token.Kind != css.Ident {
			return nil, nil, false
		}
		switch v := strings.ToLower(part[0].Token.Value); v {
		case "row", "row-reverse", "column", "column-reverse":
			if seenDirection {
				return nil, nil, false
			}
			direction, seenDirection = part, true
		case "nowrap", "wrap", "wrap-reverse":
			if seenWrap {
				return nil, nil, false
			}
			wrap, seenWrap = part, true
		default:
			return nil, nil, false
		}
	}
	if !seenDirection && !seenWrap {
		return nil, nil, false
	}
	return map[string][]css.ComponentValue{
		"flex-direction": direction, "flex-wrap": wrap,
	}, nil, true
}

// columnsShorthand is CSS Multi-column §3.3: "columns: <'column-width'> ||
// <'column-count'>".
//
// The two parts are told apart by *type* rather than by position — a length is
// the width and an integer is the count, in either order — which is the shape
// the border and outline shorthands have and the reason this is here rather than
// in the two-slot family.
//
// "auto" is the awkward one. It is the initial value of both and it names
// neither: "columns: auto" sets both to auto, and "columns: auto 12em" sets the
// width from the length and leaves the count auto. So it is read as "this slot
// is not being set", which is what the || grammar means by leaving a term out.
//
// The whole of this was missing. Both longhands are registered and both are
// read — they are what multicol.go sizes its tracks from — so a document writing
// the shorthand every author writes had its declaration reported as an
// unimplemented property and dropped, while the same thing written as two
// longhands worked.
func columnsShorthand(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
	width, count := ident("auto"), ident("auto")
	var seenWidth, seenCount, seenAuto bool

	parts := splitOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 2 {
		return nil, nil, false
	}
	for _, part := range parts {
		switch {
		case isAutoKeyword(part):
			if seenAuto {
				// "columns: auto auto" names one slot twice and neither of
				// them, which is not a value of this shorthand.
				return nil, nil, false
			}
			seenAuto = true
		case isColumnCount(part) && !seenCount:
			count, seenCount = part, true
		case isColumnWidth(part) && !seenWidth:
			width, seenWidth = part, true
		default:
			// Half a shorthand is not what was asked for: a declaration this
			// cannot read whole is dropped, and the author hears about it.
			return nil, nil, false
		}
	}
	return map[string][]css.ComponentValue{
		"column-width": width,
		"column-count": count,
	}, nil, true
}

// isAutoKeyword reports the one keyword both halves of "columns" share.
func isAutoKeyword(part []css.ComponentValue) bool {
	return len(part) == 1 && part[0].IsToken() && part[0].Token.Kind == css.Ident &&
		strings.EqualFold(part[0].Token.Value, "auto")
}

// isColumnCount is <integer>: written with no fractional part and no unit.
//
// The range is not checked here. §3.2 makes a count of zero or less invalid and
// the cascade drops it a step later with every other negative — see the list
// that names column-count for exactly that reason — and a shorthand that refused
// it here would drop the *width* along with it.
func isColumnCount(part []css.ComponentValue) bool {
	return len(part) == 1 && num(integerSlot)(part[0]).ok
}

// isColumnWidth is <length>, which a bare zero may spell.
func isColumnWidth(part []css.ComponentValue) bool {
	return len(part) == 1 && num(lengthSlot)(part[0]).ok
}
