package style

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// Presentational hints: the handful of HTML attributes that mean a CSS
// declaration.
//
// "<img width=5 height=96>" is not markup this engine may ignore. It is the
// oldest way to size an image and it is still what half the web platform's own
// reference documents use to draw a rectangle of a known size — so an engine
// that read the attribute as decoration would lay those documents out at the
// image's own pixel size and be wrong in a way that looks like a layout bug.
//
// # Where they sit in the cascade, and why it matters
//
// A hint is *not* an inline style. "img { width: 10px }" beats
// "<img width=5>", and the ordering is what makes a stylesheet able to take
// control of a document it did not write. CSS Cascade puts a hint in the author
// origin with a specificity of zero, ahead of every declaration an author
// actually wrote — which is what is done here: origin OriginAuthor, zero
// specificity, and an order number below every real declaration's.
//
// The consequences are worth stating, because both directions surprise someone:
// a user-agent rule can never beat a hint, and any author rule at all can,
// including "* { width: auto }".
//
// # Why so few
//
// Only the attributes whose element this engine lays out, and only the ones
// that are a length. The rest of HTML's presentational attributes — align,
// bgcolor, border, cellpadding — belong to elements that are refused or to a
// table algorithm that does not exist yet, and a table of hints for boxes that
// are never built would read as coverage.

// hintOrder is the cascade order number every hint carries.
//
// Declarations from stylesheets are numbered from zero upwards, so any negative
// number is below all of them — which is exactly where a hint belongs, and
// stating it as a constant is what keeps that relationship from being an
// accident of two files agreeing.
const hintOrder = -1

// hintedAttributes lists, per element, which attribute sets which property.
//
// Keyed by lower-case element name. The map exists rather than a run of
// conditionals so that a new hint is a line of data rather than a new branch —
// which is how <table width> arrived, once there was a table algorithm for it
// to mean anything to.
var hintedAttributes = map[string]map[string]string{
	"img": {"width": "width", "height": "height"},
	// <iframe width> and <iframe height> are the same two dimension properties,
	// and they arrived the same way <table width> did: the element became one
	// this engine lays out, so its attributes acquired something to mean.
	//
	// They matter more here than on an <img>, because an iframe has no
	// intrinsic size for them to override — without them the box is CSS 2.1
	// §10.3.2's 300 by 150 and nothing else can move it. Eighteen of the
	// suite's reftests write <iframe height="50%"> and check the result against
	// a div of the height that resolves to.
	"iframe": {"width": "width", "height": "height"},
	// <svg width> and <svg height> are not HTML's dimension attributes at all:
	// SVG calls them *presentation attributes*, and its own rendering section
	// maps each to the CSS property of the same name. The effect here is the
	// same and the reader below is the same, because the syntax a document
	// actually writes — "50", "50%" — is the same syntax.
	//
	// They matter beyond the sizes they state. An intrinsic dimension is a
	// number a picture carries; a *percentage* is not one, because it is a
	// proportion of something the picture cannot see. "height=50%" has no
	// intrinsic meaning and every reading of it as one is wrong — it is the CSS
	// height property, resolved against the containing block, which is what
	// absolute-replaced-height-013 says in as many words.
	"svg": {"width": "width", "height": "height"},
	// The HTML Standard's table rendering section maps the width attribute to
	// the width property as a "dimension property", which is the same syntax
	// <img width> takes and so the same reader below: a bare number is pixels
	// and a trailing per-cent sign is a percentage of the containing block.
	//
	// The height attribute is beside it because the standard does describe it,
	// in the same list and the same words — "maps to the dimension property
	// ... on the table element" — and an earlier note here that said otherwise
	// was wrong. The suite settles it without needing the prose: the reference
	// for floats-wrap-bfc-005 draws with a plain "height: 20px" div what the
	// test writes as <table height="20">, so a browser that ignored the
	// attribute would fail its own reftest.
	"table": {"cellspacing": "border-spacing", "width": "width", "height": "height"},
	// <ol start="5"> and <li value="3"> are the counter, written as attributes.
	// They take a signed integer rather than a dimension, so they are read by
	// integerAttr below instead of the table's usual dimensionValue.
	"ol": {"start": "counter-reset"},
	"li": {"value": "counter-reset"},
	// The presentational colour attributes of HTML's rendering section. They
	// are the oldest thing in this table and the only ones that are not a
	// length, which is why colourHintAttributes exists below.
	//
	// bgcolor is on <body> here and not on <table> and its parts, which map it
	// the same way. The reason is the same one <table width> waited for: the
	// attribute needs somewhere to mean something, and the cell backgrounds a
	// table's bgcolor sets are painted by machinery that would have to agree
	// with it. One element at a time, each when it can be checked.
	"body": {"bgcolor": "background-color", "text": "color"},
	// <font> is three presentational attributes and nothing else. HTML's
	// rendering section maps them by name: colour, family and — through a table
	// of seven steps — size. They are the reason the element is worth laying out
	// at all, since without them a <font> is a <span>.
	"font": {"color": "color", "face": "font-family", "size": "font-size"},
}

// colourHintAttributes are the entries above whose value is a colour rather than
// a length or a counter.
var colourHintAttributes = map[string]bool{"bgcolor": true, "text": true, "color": true}

// familyHintAttributes are the entries whose value is a font family list, which
// is neither a length nor a colour: <font face="Courier, monospace"> is the
// font-family property written as an attribute, commas and all.
var familyHintAttributes = map[string]bool{"face": true}

// sizeHintAttributes are the entries whose value is HTML's font size number.
var sizeHintAttributes = map[string]bool{"size": true}

// counterHintAttributes are the entries above whose value is a plain integer
// naming a counter, rather than a length.
var counterHintAttributes = map[string]bool{"start": true, "value": true}

// There is deliberately no cache of parsed hint values.
//
// One was written here first, on the reasoning that a document of a thousand
// thumbnails asks for "150px" a thousand times — and it was a package-level
// map, keyed on text taken straight out of an untrusted document, which is a
// leak that outlives the render that filled it. The parse it saved is two
// tokens, asked at most twice per element, so what it bought was nothing and
// what it cost was unbounded memory across a process's lifetime.

// presentationalHints returns the declarations an element's attributes imply.
//
// The value syntax is HTML's "dimension value": a run of digits, optionally
// followed by a per-cent sign. Anything else — a negative number, a length with
// a unit, a word — is not a dimension and the attribute is ignored, which is
// what HTML requires and is also the safe answer: a value this cannot read must
// not become a length it guessed at.
func presentationalHints(n *html.Node) map[string][]css.ComponentValue {
	name := strings.ToLower(n.Name)
	if name == "td" || name == "th" {
		return cellHints(n)
	}
	attrs, ok := hintedAttributes[name]
	if !ok {
		return nil
	}
	var out map[string][]css.ComponentValue
	for attr, property := range attrs {
		raw, ok := n.Attr(attr)
		if !ok {
			continue
		}
		var value string
		if colourHintAttributes[attr] {
			value, ok = colourValue(raw)
		} else if familyHintAttributes[attr] {
			value, ok = familyValue(raw)
		} else if sizeHintAttributes[attr] {
			value, ok = fontSizeValue(raw)
		} else if counterHintAttributes[attr] {
			// "start" and "value" set the counter to one *below* the number
			// they name, because the item increments it on the way in. That is
			// what makes <li value="3"> show a 3 rather than a 4.
			value, ok = counterResetValue(raw)
		} else {
			value, ok = dimensionValue(raw)
		}
		if !ok {
			continue
		}
		if out == nil {
			out = make(map[string][]css.ComponentValue, len(attrs))
		}
		vals, _ := css.ParseComponentValues(value)
		out[property] = vals
	}
	return out
}

// familyValue turns a <font face> into a font-family list.
//
// The attribute's syntax is the property's: a comma-separated list of family
// names. It is quoted here rather than passed through, because an unquoted
// family name in CSS is a sequence of identifiers and an attribute may hold
// anything at all — "PASS PASS" is a family name in an attribute and two
// identifiers in a stylesheet, and content-076 writes exactly that.
func familyValue(raw string) (string, bool) {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.TrimSpace(part)
		if name == "" || strings.ContainsAny(name, "\"\\") {
			// A quote or a backslash in an attribute cannot be quoted here
			// without an escaping pass, and a family by that name is not one
			// anybody has. The whole list is refused rather than half of it.
			return "", false
		}
		out = append(out, "\""+name+"\"")
	}
	if len(out) == 0 {
		return "", false
	}
	return strings.Join(out, ", "), true
}

// maxFontSizeSteps is how far HTML's font size scale runs.
const maxFontSizeSteps = 7

// fontSizeSteps is HTML's rendering section's table, from step 1 to step 7.
//
// The scale is the one <font> has had since it was invented, and the keywords
// are what the section maps it to. There is no eighth: a size above seven is
// clamped to seven and one below one to one, which is what "the seventh entry"
// and "the first entry" mean in the prose.
var fontSizeSteps = [maxFontSizeSteps]string{
	"x-small", "small", "medium", "large", "x-large", "xx-large", "xxx-large",
}

// fontSizeValue turns a <font size> into a font-size keyword.
//
// A bare number is a step on the scale. A signed one is relative to step 3,
// which is the default and is what "medium" means — so "+1" is large and "-1" is
// small, and a document that nests them does not compound, because each element
// reads the attribute afresh rather than the size it inherited.
func fontSizeValue(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	relative := 0
	switch s[0] {
	case '+':
		relative, s = +1, s[1:]
	case '-':
		relative, s = -1, s[1:]
	}
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > maxHintDigits || digits != len(s) {
		return "", false
	}
	n := 0
	for _, c := range []byte(s) {
		n = n*10 + int(c-'0')
		if n > 1000 {
			// Past anything the scale can say. It is clamped below either way,
			// and stopping here keeps the arithmetic away from an overflow.
			n = 1000
			break
		}
	}
	if relative != 0 {
		n = 3 + relative*n
	}
	if n < 1 {
		n = 1
	}
	if n > maxFontSizeSteps {
		n = maxFontSizeSteps
	}
	return fontSizeSteps[n-1], true
}

// maxHintDigits bounds the number a dimension attribute may state.
//
// Ten digits cannot overflow the parse below and is already four orders of
// magnitude past any page; the bound is here because the attribute is untrusted
// text and a length is one multiplication away from a box the size of a
// continent. A longer run of digits is not a large image, it is a value nobody
// meant, so it is refused rather than saturated.
const maxHintDigits = 10

// dimensionValue turns an HTML dimension attribute into a CSS length.
//
// Leading and trailing white space is allowed, because HTML's attribute values
// are commonly written with it and every browser strips it. Everything else is
// exact: the digits, then optionally a per-cent sign, then the end.
func dimensionValue(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > maxHintDigits {
		return "", false
	}
	switch s[digits:] {
	case "":
		// A bare number is a length in CSS pixels.
		return s + "px", true
	case "%":
		return s, true
	}
	return "", false
}

// cellHints are the hints a table cell takes: its table's cellpadding, and its
// own nowrap.
//
// HTML's table rendering section states the second as a rule rather than as an
// attribute mapping — "td[nowrap], th[nowrap] { white-space: nowrap }" — and it
// is a boolean attribute, so what matters is that it is there at all and not
// what it says.
//
// It sets the two longhands rather than the shorthand, because a hint goes
// straight into the cascade without passing through the expander: naming
// white-space here would set a property nothing reads. Both of them, and not
// only the wrapping half, because that is what the rule says — a cell inside a
// "white-space: pre" table with nowrap on it collapses its spaces.
func cellHints(n *html.Node) map[string][]css.ComponentValue {
	out := cellPaddingHint(n)
	if _, ok := n.Attr("nowrap"); !ok {
		return out
	}
	if out == nil {
		out = make(map[string][]css.ComponentValue, 2)
	}
	out["white-space-collapse"] = ident("collapse")
	out["text-wrap-mode"] = ident("nowrap")
	return out
}

// cellPaddingHint reads the cellpadding an ancestor table declares.
//
// It is the one hint that is not an attribute of the element it styles:
// cellpadding is written once on the table and applies to every cell in it. The
// walk stops at the first table, which is what makes a nested table's cells take
// their own table's padding rather than the one they happen to sit inside.
//
// A percentage is refused here even though the dimension syntax allows one,
// because a percentage padding resolves against the containing block's width and
// that is not what an author writing cellpadding="10%" is asking for. A value
// this cannot read leaves the stylesheet's answer standing, which is the same as
// the attribute not being there.
func cellPaddingHint(n *html.Node) map[string][]css.ComponentValue {
	for anc := n.Parent; anc != nil; anc = anc.Parent {
		if anc.Type != html.ElementNode || !strings.EqualFold(anc.Name, "table") {
			continue
		}
		raw, ok := anc.Attr("cellpadding")
		if !ok {
			return nil
		}
		value, ok := dimensionValue(raw)
		if !ok || strings.HasSuffix(value, "%") {
			return nil
		}
		vals, _ := css.ParseComponentValues(value)
		return map[string][]css.ComponentValue{
			"padding-top": vals, "padding-right": vals,
			"padding-bottom": vals, "padding-left": vals,
		}
	}
	return nil
}

// counterResetValue turns a start or value attribute into a counter-reset.
//
// The attribute names the number the item is to show. The list-item counter is
// incremented as the item is entered, so the reset has to be one less — and
// "one less" is why this is not simply the integer: a value of the most negative
// integer would wrap, and an attribute is untrusted text.
func counterResetValue(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	neg := false
	if len(s) > 0 && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" || len(s) > maxHintDigits {
		return "", false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return "", false
		}
		n = n*10 + int(s[i]-'0')
	}
	if neg {
		n = -n
	}
	return "list-item " + strconv.Itoa(n-1), true
}

// colourValue turns a presentational colour attribute into a CSS colour.
//
// It reads the two spellings a document actually writes — a hash followed by
// three or six hexadecimal digits, and a colour keyword — and refuses everything
// else. HTML's own rule is far wider than that: its "legacy colour value" takes
// any string at all, strips what it cannot use and pads what is left, so
// "chucknorris" is a colour and comes out #C00000. That is a real algorithm and
// it is deliberately not here.
//
// The reason is what a wrong answer costs. A hint that refuses leaves the
// property at its initial value, which is what a document that did not write the
// attribute would have got; a hint that guesses paints the page a colour nobody
// asked for and nothing reports it. Legacy parsing can be added the day a
// document needs it, with the specification open, rather than approximated now.
func colourValue(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	if s[0] == '#' {
		digits := s[1:]
		if len(digits) != 3 && len(digits) != 6 {
			return "", false
		}
		for i := 0; i < len(digits); i++ {
			if !isHexDigit(digits[i]) {
				return "", false
			}
		}
		return s, true
	}
	// A keyword, and only a keyword. It is checked against the same table the
	// colour property reads, so the attribute cannot name a colour a
	// declaration could not — and it must be a bare identifier, because a
	// function is not something HTML's rule would ever have produced here:
	// "rgb(1,2,3)" in a bgcolor is a string to be mangled, not a colour.
	vals := mustValues(strings.ToLower(s))
	if len(vals) != 1 || !vals[0].IsToken() || vals[0].Token.Kind != css.Ident {
		return "", false
	}
	if _, ok := ParseColor(vals); !ok {
		return "", false
	}
	return s, true
}

// mustValues parses a value that is a single token, for the keyword check above.
func mustValues(s string) []css.ComponentValue {
	vals, _ := css.ParseComponentValues(s)
	return vals
}
