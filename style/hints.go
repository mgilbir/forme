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
// # Which ones are here
//
// Only the attributes whose element this engine lays out. That was once only
// the ones that are a length, and the note here said so; it has grown as the
// elements have — bgcolor and cellpadding arrived with the table algorithm they
// needed, and the colour and font attributes with the elements that carry them.
//
// "align" is not here and is not missing: HTML states it as user agent
// stylesheet *rules* rather than as an attribute mapping, and it is in
// layout/uastyle.go with the rest of them. The difference is not observable —
// a hint carries zero specificity in the author origin and loses to every
// author declaration, and a user agent rule loses to them too — so the place to
// put it is the place the specification puts it.
//
// "border" is here now, and is written out below rather than added to the table
// because it is three declarations with a condition the table cannot express.
// The "frame" and "rules" attributes it interacts with are not: they are a
// dozen more rules about which edges of which cells are drawn, and they are
// plain keyword matches that belong in the user agent sheet.

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
	"table": {"cellspacing": "border-spacing", "width": "width", "height": "height",
		"bgcolor": "background-color", "background": "background-image"},
	// And on a cell, which the same section maps the same way: "maps to the
	// dimension property (ignoring zero)". They were missing, so
	// "<td width=50%>" — which is how a table said what proportion a column
	// takes, and is still how most tables in older documents say it — set
	// nothing at all and the column was sized by its content.
	//
	// The percentage is the whole point of them. A bare number is a pixel width
	// a stylesheet could have given instead; a percentage is a statement about
	// the table that nothing else in the markup can make.
	"td": {"width": "width", "height": "height",
		"bgcolor": "background-color", "background": "background-image"},
	"th": {"width": "width", "height": "height",
		"bgcolor": "background-color", "background": "background-image"},
	// <li value="3"> is the counter, written as an attribute. It takes a signed
	// integer rather than a dimension, so it is read by counterSetValue below
	// instead of the table's usual dimensionValue.
	//
	// It is counter-set and not counter-reset, which HTML says and which used
	// to be approximated: a reset *creates* a counter and a set writes the one
	// that is there. For a list that counts up the two are the same page, which
	// is why the approximation stood — the new counter's scope is the rest of
	// the list and it carries on from the number the attribute named. For one
	// that counts down they are not: a created counter is not the reversed one,
	// so the items after it would count upwards.
	//
	// <ol start> is not here beside it: what it sets depends on whether
	// "reversed" is written next to it, and the table is one attribute to one
	// property. See olCounterHint.
	"li": {"value": "counter-set"},
	// The presentational colour attributes of HTML's rendering section. They
	// are the oldest thing in this table and the only ones that are not a
	// length, which is why colourHintAttributes exists below.
	//
	"body": {"bgcolor": "background-color", "text": "color",
		"background": "background-image"},
	// And the table parts, which HTML maps bgcolor on in the same words it maps
	// it on <body>. It was on <body> alone, deliberately: the note that used to
	// be here said the cell backgrounds a table's bgcolor sets are painted by
	// machinery that would have to agree with it, and one element at a time,
	// each when it can be checked.
	//
	// It can be checked now — every part of a table paints its own background,
	// which was measured on the page before this was written — so "<table
	// bgcolor=...>" and "<td bgcolor=...>", which is how every document of a
	// certain age colours a table, mean something at last.
	//
	// "background" is beside it in the same sentence and is not a colour at
	// all: the value names a file, which becomes the url() the background-image
	// property takes. See urlHintValue.
	"thead": {"bgcolor": "background-color", "background": "background-image",
		"valign": "vertical-align"},
	"tbody": {"bgcolor": "background-color", "background": "background-image",
		"valign": "vertical-align"},
	"tfoot": {"bgcolor": "background-color", "background": "background-image",
		"valign": "vertical-align"},
	"tr": {"bgcolor": "background-color", "background": "background-image",
		"valign": "vertical-align"},
	// <font> is three presentational attributes and nothing else. HTML's
	// rendering section maps them by name: colour, family and — through a table
	// of seven steps — size. They are the reason the element is worth laying out
	// at all, since without them a <font> is a <span>.
	"font": {"color": "color", "face": "font-family", "size": "font-size"},
	// valign, which HTML's table rendering section maps to vertical-align on
	// every part of a table that can carry it. A cell's own is read by
	// cellHints, which is where td and th go; these are the rest.
	//
	// It reaches the cells through the user-agent stylesheet already there:
	// "tr, td, th { vertical-align: inherit }" is the rule that carries a row's
	// alignment down, because the property does not inherit on its own. So
	// "<tr valign=top>" sets the row and the cells take it, which is the
	// behaviour the attribute has always had.
	//
	// A hint rather than a rule, and the difference is a place in the cascade:
	// "td { vertical-align: middle }" in a stylesheet has to beat the markup,
	// and the user-agent's own "vertical-align: inherit" must not.
	"col":      {"valign": "vertical-align"},
	"colgroup": {"valign": "vertical-align"},
	// <br clear>, which is older than the property it sets and is the only
	// place the property applies to something that is not a block-level box.
	// HTML's rendering section maps it by name — "left", "right", "all" or
	// "both", and "none" — and every engine follows, because "<br clear=all>"
	// is how a page cleared a float before CSS existed.
	//
	// It is on this table rather than in layout because it is a *presentational
	// hint*, which is a place in the cascade: an author rule beats it and a
	// user-agent rule does not. A layout that read the attribute directly would
	// have "br { clear: none }" in a stylesheet lose to the markup.
	"br": {"clear": "clear"},
	// <hr color> and <hr width>, §15.3.6. The colour is a legacy colour value,
	// which colourValue already reads, and the width is the ordinary dimension
	// property — this one *without* "ignoring zero", which the section says by
	// not saying it.
	//
	// The size attribute is not here: what it sets depends on whether colour or
	// noshade is beside it, and the table above is one attribute to one
	// property. See hrSizeHint.
	"hr": {"color": "color", "width": "width"},
}

// clearHintAttributes are the entries whose value is one of a handful of
// keywords rather than a length, a colour or a number.
var clearHintAttributes = map[string]bool{"clear": true}

// valignHintAttributes are the entries whose value is HTML's table alignment
// keyword, which is not quite the CSS one — see valignValue.
var valignHintAttributes = map[string]bool{"valign": true}

// urlHintAttributes are the entries whose value is a file to fetch rather than
// anything CSS has a syntax for. It is written as a url() so that everything
// downstream reads it as the property it set — the loader that fetches a
// background image, the painter that tiles it, and the finding that says it did
// not arrive are the ones a stylesheet already goes through.
var urlHintAttributes = map[string]bool{"background": true}

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
// naming a counter, rather than a length. "start" is not here: it is read by
// olCounterHint, because what it sets depends on the attribute beside it.
var counterHintAttributes = map[string]bool{"value": true}

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
// The value syntax of the length ones is HTML's "dimension value" — see
// dimensionValue — and a value that is not one leaves the attribute ignored,
// which is what HTML requires: a value this cannot read must not become a
// length it guessed at.
func presentationalHints(n *html.Node) map[string][]css.ComponentValue {
	name := strings.ToLower(n.Name)
	out := attributeHints(name, n)
	if name == "table" {
		for property, vals := range tableBorderHint(n) {
			if out == nil {
				out = map[string][]css.ComponentValue{}
			}
			out[property] = vals
		}
	}
	if name == "ol" {
		for property, vals := range olCounterHint(n) {
			if out == nil {
				out = map[string][]css.ComponentValue{}
			}
			out[property] = vals
		}
	}
	if name == "hr" {
		for property, vals := range hrSizeHint(n) {
			if out == nil {
				out = map[string][]css.ComponentValue{}
			}
			out[property] = vals
		}
	}
	if name == "a" || name == "area" {
		for property, vals := range linkColourHint(n) {
			if out == nil {
				out = map[string][]css.ComponentValue{}
			}
			out[property] = vals
		}
	}
	if name != "td" && name != "th" {
		return out
	}
	// A cell takes two more that its own attribute table cannot express: its
	// *table's* cellpadding, and a boolean attribute that HTML states as a rule
	// rather than as a mapping. They are separate because they are found
	// differently, and they set properties the table above does not, so the
	// merge needs no order.
	for property, vals := range cellHints(n) {
		if out == nil {
			out = map[string][]css.ComponentValue{}
		}
		out[property] = vals
	}
	return out
}

// attributeHints is the table above, read for one element.
func attributeHints(name string, n *html.Node) map[string][]css.ComponentValue {
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
		if urlHintAttributes[attr] {
			value, ok = urlHintValue(raw)
		} else if colourHintAttributes[attr] {
			value, ok = colourValue(raw)
		} else if familyHintAttributes[attr] {
			value, ok = familyValue(raw)
		} else if sizeHintAttributes[attr] {
			value, ok = fontSizeValue(raw)
		} else if clearHintAttributes[attr] {
			value, ok = clearValue(raw)
		} else if valignHintAttributes[attr] {
			value, ok = valignValue(raw)
		} else if counterHintAttributes[attr] {
			// "value" names the number the item is to show, and
			// counter-set writes it after the increment has run — so unlike
			// "start", which is read by olCounterHint, no arithmetic is needed.
			value, ok = counterSetValue(raw)
		} else {
			value, ok = dimensionValue(raw)
			if ok && zeroIsNoDimension[name][attr] && isZeroDimension(value) {
				// "Maps to the dimension property (ignoring zero)", which is
				// the wording HTML uses for most of these and is not a detail:
				// a zero is parsed by "the rules for parsing *nonzero*
				// dimension values", which errors, so the attribute is absent
				// rather than zero. "<img width=0>" is an image at its own
				// width in every browser, and was an invisible one here.
				ok = false
			}
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

// urlHintValue turns a "background" attribute into the url() the property takes.
//
// The attribute is a file name and the property is a CSS value, so the name has
// to be quoted on the way across: a bare url() token has escaping rules of its
// own and a document may write anything at all in an attribute. A name holding
// a quote, a backslash or a newline is refused rather than escaped, which is
// familyValue's answer to the same question and for the same reason — escaping
// it properly is a pass this does not have, and a file by that name is not one
// anybody has.
//
// An empty value is not a file. HTML says so in as many words, "set to a
// non-empty value", and it matters: url("") is a reference to the document
// itself, so reading one as a file would have every document with an empty
// attribute fetch its own markup and fail to decode it.
func urlHintValue(raw string) (string, bool) {
	ref := strings.TrimSpace(raw)
	if ref == "" || strings.ContainsAny(ref, "\"\\\n\r") {
		return "", false
	}
	return "url(\"" + ref + "\")", true
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

// maxHintDigits bounds the number a dimension attribute or a font size may
// state.
//
// Ten digits cannot overflow the parse below and is already four orders of
// magnitude past any page; the bound is here because the attribute is untrusted
// text and a length is one multiplication away from a box the size of a
// continent. It is the bound for the two readers here that are not HTML's
// integer rules — a dimension (§2.3.4.4) and a legacy font size. The integer
// attributes are read by html.ParseInteger and html.ParseNonNegativeInteger,
// which saturate rather than refuse.
const maxHintDigits = 10

// dimensionValue turns an HTML dimension attribute into a CSS length, by HTML's
// "rules for parsing dimension values" (§2.3.4.4): leading white space, then
// digits, then optionally a full stop and more digits, then optionally a per-cent
// sign — and whatever follows is ignored. So "100px" is a hundred pixels,
// "50.5" is fifty and a half, "60%" is a percentage and "10.%" is too.
//
// It used to accept the digits and the per-cent sign and nothing else, on the
// reading that a length with a unit "is not a dimension", which HTML does not
// say: "<img width=100px>" and "<table width=600px>", common in legacy and
// e-mail markup, were laid out at their natural size with nothing reported
// (audit C115). The prefix reading is the one the integer readers in this
// file already use.
func dimensionValue(raw string) (string, bool) {
	s := strings.TrimLeft(raw, " \t\n\f\r")
	digits := 0
	for digits < len(s) && s[digits] >= '0' && s[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits > maxHintDigits {
		return "", false
	}
	value, rest := s[:digits], s[digits:]
	if len(rest) > 0 && rest[0] == '.' {
		rest = rest[1:]
		frac := 0
		for frac < len(rest) && rest[frac] >= '0' && rest[frac] <= '9' {
			frac++
		}
		if frac > 0 {
			// Past ten places a fraction is below anything a layout unit can
			// hold; the digits after that are read and not kept, which bounds
			// the length of what is written without changing its value.
			value += "." + rest[:min(frac, maxHintDigits)]
		}
		rest = rest[frac:]
	}
	if len(rest) > 0 && rest[0] == '%' {
		return value + "%", true
	}
	// A number with no per-cent sign after it is a length in CSS pixels.
	return value + "px", true
}

// zeroIsNoDimension names the attributes HTML maps "ignoring zero", by element.
//
// It is a list rather than a rule about dimensions because the wording is not
// uniform and the difference is deliberate: a table's *width* ignores a zero
// and its *height* does not, in the same sentence of the same section.
//
// <svg width> and <svg height> are absent, and that is SVG's rule rather than
// an omission: SVG 2 makes a zero width a statement that the element is not
// rendered, so ignoring it would turn "draw nothing" into "draw at whatever
// size the viewport gives".
var zeroIsNoDimension = map[string]map[string]bool{
	"img":   {"width": true, "height": true},
	"table": {"width": true},
	"td":    {"width": true, "height": true},
	"th":    {"width": true, "height": true},
}

// isZeroDimension reports whether a dimension this file produced is a zero.
//
// It reads what dimensionValue wrote rather than the attribute, because a zero
// arrives in many spellings — "0px", "00%", "0.00px" from "0.00em" — and what
// they share is digits and a full stop that are all zero.
func isZeroDimension(value string) bool {
	digits := strings.TrimSuffix(strings.TrimSuffix(value, "px"), "%")
	return digits != "" && strings.Trim(digits, "0.") == ""
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
	if border := cellBorderHint(n); border != nil {
		if out == nil {
			out = make(map[string][]css.ComponentValue, len(border)+3)
		}
		for property, vals := range border {
			out[property] = vals
		}
	}
	if raw, ok := n.Attr("valign"); ok {
		if value, ok := valignValue(raw); ok {
			if out == nil {
				out = make(map[string][]css.ComponentValue, 3)
			}
			out["vertical-align"] = ident(value)
		}
	}
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

// valignValue turns a table part's valign attribute into a vertical-align
// keyword.
//
// The four HTML names it and the property share are the same word, and the one
// that differs is the reason this is a function rather than a pass-through:
// "center" is what a document writes and "middle" is what the property calls
// it. Anything else is not one of the five and the attribute is ignored, which
// is what HTML asks for and is also the safe answer — a word this cannot read
// must not become an alignment it guessed at.
//
// Case-insensitively, because HTML attribute *values* are matched that way here
// even though their names are already folded: "<td VALIGN=Bottom>" is what a
// document written in 1998 looks like, and it is the reason the attribute is
// worth reading at all.
func valignValue(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "top":
		return "top", true
	case "middle", "center":
		return "middle", true
	case "bottom":
		return "bottom", true
	case "baseline":
		return "baseline", true
	}
	return "", false
}

// The table border attribute, which is three things at once and so is written
// out here rather than added to the table above.
//
// HTML's rendering section maps it to the four border widths on the table, and
// then gives two more rules whose condition the selector language cannot state:
//
//	table[border] { border-style: outset }  /* only if border is not equivalent to zero */
//	table[border] > tr > td, ... { border-width: 1px; border-style: inset }
//
// The comment is the specification's own, and it is a comment because "not
// equivalent to zero" means the value parsed as an integer, which no selector
// does: "0", "00" and " 0" are all zero and "[border=0]" tells them apart. So
// this is a presentational hint, where the value can be read properly, and the
// three parts of the attribute are decided together.
//
// The reader is not dimensionValue either, and the difference is the one thing
// about this attribute nobody expects: a value that does not parse is **one
// pixel**, not nothing. "<table border=yes>" is a bordered table in every
// browser, which is what the rendering section asks for in as many words, where
// every other dimension attribute drops what it cannot read.

// borderAttribute reads a table's border attribute into the width it states and
// whether it draws anything.
//
// HTML's "rules for parsing non-negative integers" take the leading digits and
// ignore what follows, so "1px" is one. What has no leading digits at all —
// including the empty string of "<table border>" — is the parse error the
// section gives a default of 1px for.
//
// The reading is html.ParseNonNegativeInteger, the one every integer attribute
// is read by. A value too long for any page is still a value — it saturates,
// and the border is as wide as a length can be — where it used to be refused
// past ten digits and drawn as the one-pixel default.
func borderAttribute(raw string) (width string, drawn bool) {
	n, ok := html.ParseNonNegativeInteger(raw)
	if !ok {
		return "1px", true
	}
	return strconv.Itoa(n) + "px", n != 0
}

// tableBorderHint is the border attribute read on the table that carries it.
func tableBorderHint(n *html.Node) map[string][]css.ComponentValue {
	raw, ok := n.Attr("border")
	if !ok {
		return nil
	}
	width, drawn := borderAttribute(raw)
	vals, _ := css.ParseComponentValues(width)
	out := map[string][]css.ComponentValue{
		"border-top-width": vals, "border-right-width": vals,
		"border-bottom-width": vals, "border-left-width": vals,
	}
	if drawn {
		style := ident("outset")
		out["border-top-style"] = style
		out["border-right-style"] = style
		out["border-bottom-style"] = style
		out["border-left-style"] = style
	}
	return out
}

// cellBorderHint is the same attribute read on a cell of the table that carries
// it, which is the second half of the rule above: a bordered table gives every
// one of its cells a one-pixel inset border.
//
// The walk is cellPaddingHint's and stops at the first table, so a nested
// table's cells take their own table's border rather than the one they happen
// to sit inside — which is what the specification's "table[border] > tr > td"
// says with a child combinator.
func cellBorderHint(n *html.Node) map[string][]css.ComponentValue {
	for anc := n.Parent; anc != nil; anc = anc.Parent {
		if anc.Type != html.ElementNode || !strings.EqualFold(anc.Name, "table") {
			continue
		}
		raw, ok := anc.Attr("border")
		if !ok {
			return nil
		}
		if _, drawn := borderAttribute(raw); !drawn {
			return nil
		}
		width, style := pixels(1), ident("inset")
		return map[string][]css.ComponentValue{
			"border-top-width": width, "border-right-width": width,
			"border-bottom-width": width, "border-left-width": width,
			"border-top-style": style, "border-right-style": style,
			"border-bottom-style": style, "border-left-style": style,
		}
	}
	return nil
}

// olCounterHint is §15.3.7's start and reversed attributes, which between them
// decide one counter-reset.
//
// The specification gives it as four steps and they are worth following exactly,
// because the off-by-one goes in opposite directions:
//
//	reversed, with a start   ->  reversed(list-item) <start+1>
//	reversed, no start       ->  reversed(list-item)
//	a start, not reversed    ->  list-item <start-1>
//	neither                  ->  nothing
//
// The two ones are the same one seen from either end. counter-reset sets the
// counter *before* the first increment, so a list that counts up from N starts
// at N-1 and one that counts down from N starts at N+1.
//
// A reversed list with no start takes its value from the counter itself: a
// reversed counter with no number begins at the number of things in its scope
// that increment it, which is the count of the items. That is CSS's rule rather
// than HTML's, and it is why the bare "reversed(list-item)" is a complete
// answer here.
func olCounterHint(n *html.Node) map[string][]css.ComponentValue {
	raw, hasStart := n.Attr("start")
	reversed := n.HasAttr("reversed")
	start, startOK := 0, false
	if hasStart {
		// HTML's rules for parsing integers (§2.3.4.1), which take the digits
		// at the front: "3px" starts at three. This read the whole string and
		// refused one with anything after its digits, which is a different
		// rule from the one HTML gives and from the one its own neighbours in
		// this file follow.
		start, startOK = html.ParseInteger(raw)
	}
	value := ""
	switch {
	case reversed && startOK:
		value = "reversed(list-item) " + strconv.Itoa(start+1)
	case reversed:
		value = "reversed(list-item)"
	case startOK:
		value = "list-item " + strconv.Itoa(start-1)
	default:
		return nil
	}
	vals, _ := css.ParseComponentValues(value)
	return map[string][]css.ComponentValue{"counter-reset": vals}
}

// hrSizeHint is §15.3.6's size attribute, which sets two different properties
// depending on what is written beside it.
//
// With a colour or a noshade the rule is drawn as a solid line, and the size is
// its thickness — halved, because the line is drawn as a border on both edges
// and the two have to add up to what was asked for. Without either it is drawn
// as a groove, the height is the gap between the two edges, and the size is the
// whole thing: one is a rule with no gap at all, and anything more is the size
// less the two edges.
//
// That is the specification's arithmetic and not an interpretation of it. It is
// here rather than in the attribute table because the table is one attribute to
// one property, and this is one attribute to two properties chosen by a third.
func hrSizeHint(n *html.Node) map[string][]css.ComponentValue {
	raw, ok := n.Attr("size")
	if !ok {
		return nil
	}
	size, ok := html.ParseNonNegativeInteger(raw)
	if !ok {
		return nil
	}
	_, hasColour := n.Attr("color")
	if hasColour || n.HasAttr("noshade") {
		half := pixels(size / 2)
		return map[string][]css.ComponentValue{
			"border-top-width": half, "border-right-width": half,
			"border-bottom-width": half, "border-left-width": half,
		}
	}
	switch {
	case size == 1:
		return map[string][]css.ComponentValue{"border-bottom-width": pixels(0)}
	case size > 1:
		return map[string][]css.ComponentValue{"height": pixels(size - 2)}
	}
	return nil
}

// pixels is a length written the way a stylesheet writes one.
//
// It is parsed rather than made, because a component value built by hand out of
// "2px" is an *identifier* whose name begins with a digit — and serialising one
// of those escapes the digit, so the cascade was handed "\32 px" where a
// document had asked for two pixels.
func pixels(n int) []css.ComponentValue {
	vals, _ := css.ParseComponentValues(strconv.Itoa(n) + "px")
	return vals
}

// linkColourHint is the body element's "link" attribute, read on the links it
// colours.
//
// It is the second hint that is not an attribute of the element it styles:
// written once on the body, it applies to "any element that is a link", which is
// the set :link selects and is asked with the same function so that the two
// cannot come to differ.
//
// Its two neighbours are deliberately absent. "vlink" is the colour of a
// *visited* link and "alink" of one being clicked, and on paper nothing is
// either: :visited is answered no here — see the note beside it — and there is
// no pointer to hold down. They are not reported, for the reason the engine
// reports anything: a browser printing the same document shows an unvisited,
// unclicked link too, so there is no difference to tell an author about.
func linkColourHint(n *html.Node) map[string][]css.ComponentValue {
	if !isLink(n) {
		return nil
	}
	for anc := n.Parent; anc != nil; anc = anc.Parent {
		if anc.Type != html.ElementNode || !strings.EqualFold(anc.Name, "body") {
			continue
		}
		raw, ok := anc.Attr("link")
		if !ok {
			return nil
		}
		value, ok := colourValue(raw)
		if !ok {
			return nil
		}
		vals, _ := css.ParseComponentValues(value)
		return map[string][]css.ComponentValue{"color": vals}
	}
	return nil
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

// counterSetValue turns a <li value> into a counter-set.
//
// The attribute names the number the item is to show, and counter-set is
// applied *after* the increment — css-lists-3 §4.3 calls that a deliberate
// choice — so the number goes across unchanged. The arithmetic that used to be
// here belonged to counter-reset, which is applied before the increment and so
// had to be one less; that reading is olCounterHint's now, where "start" still
// needs it.
func counterSetValue(raw string) (string, bool) {
	n, ok := html.ParseInteger(raw)
	if !ok {
		return "", false
	}
	return "list-item " + strconv.Itoa(n), true
}

// colourValue turns a presentational colour attribute into a CSS colour, by
// HTML's "rules for parsing a legacy colour value" (§2.3.6).
//
// The rule takes almost any string: a keyword is that colour, and anything
// else has what is not a hexadecimal digit replaced with a zero, is padded and
// split into three, and is read as red, green and blue — so
// "<font color=ff0000>" is red, "<body bgcolor=ffffff>" is white, and
// "chucknorris" is #c00000, in every browser.
//
// It was refused here on the grounds that a guessed colour would paint the page
// a colour nobody asked for and nothing would report it. The algorithm is not a
// guess — it is the specification, fully stated, and it is what a browser
// paints — and the refusal was not reported either, so "color=ff0000", the
// most common legacy spelling, was drawn black with nothing said (audit C116).
//
// A keyword keeps its spelling, and so does a well-formed hash, because both
// are already CSS; everything else is written as the #rrggbb it came to.
func colourValue(raw string) (string, bool) {
	// 1-3: empty, only white space, or "transparent" is not a colour.
	s := strings.Trim(raw, " \t\n\f\r")
	if s == "" || strings.EqualFold(s, "transparent") {
		return "", false
	}
	// 4: a named colour.
	if _, ok := namedColors[strings.ToLower(s)]; ok {
		return s, true
	}
	// 5: "#" and three hexadecimal digits.
	if len(s) == 4 && s[0] == '#' && isHexDigit(s[1]) && isHexDigit(s[2]) && isHexDigit(s[3]) {
		return s, true
	}
	if len(s) == 7 && s[0] == '#' {
		if _, ok := parseHex(s[1:]); ok {
			return s, true
		}
	}
	// 6-7: a character outside the Basic Multilingual Plane counts as "00",
	// and the value is cut to 128 characters.
	var in []byte
	for _, r := range s {
		if len(in) >= 128 {
			break
		}
		switch {
		case r > 0xFFFF:
			in = append(in, '0', '0')
		case r < 0x80 && isHexDigit(byte(r)):
			in = append(in, byte(r))
		case r == '#' && len(in) == 0:
			in = append(in, '#')
		default:
			// 9: anything that is not a hexadecimal digit is a zero.
			in = append(in, '0')
		}
	}
	if len(in) > 128 {
		in = in[:128]
	}
	// 8: a leading "#" is dropped.
	if len(in) > 0 && in[0] == '#' {
		in = in[1:]
	}
	// 10: padded with zeros to a non-zero multiple of three.
	for len(in) == 0 || len(in)%3 != 0 {
		in = append(in, '0')
	}
	// 11-14: three equal parts, each cut to its last eight characters, then
	// stripped of leading zeros they all share while longer than two, then cut
	// to its first two.
	n := len(in) / 3
	parts := [3][]byte{in[:n], in[n : 2*n], in[2*n:]}
	if n > 8 {
		for i := range parts {
			parts[i] = parts[i][n-8:]
		}
		n = 8
	}
	for n > 2 && parts[0][0] == '0' && parts[1][0] == '0' && parts[2][0] == '0' {
		for i := range parts {
			parts[i] = parts[i][1:]
		}
		n--
	}
	if n > 2 {
		for i := range parts {
			parts[i] = parts[i][:2]
		}
	}
	out := []byte{'#'}
	for _, p := range parts {
		if len(p) == 1 {
			// A one-digit component is that digit's value, not the doubled
			// shorthand "#rgb" gives it: "#1" reads as 0x01.
			out = append(out, '0')
		}
		out = append(out, strings.ToLower(string(p))...)
	}
	return string(out), true
}

// clearValue turns a <br clear> into the property's keyword.
//
// HTML's rendering section names four: "left", "right", "all" and "none", with
// "all" mapping to the property's "both". "both" is accepted as well because
// the attribute is old enough that documents write the property's own spelling
// into it, and reading it costs nothing.
//
// Anything else is not a value, and the attribute is ignored rather than
// guessed at — which is what every other hint here does with a value it cannot
// read, and is the safe direction: a <br> that clears nothing is the <br> the
// document would have had without the attribute at all.
func clearValue(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "left":
		return "left", true
	case "right":
		return "right", true
	case "all", "both":
		return "both", true
	case "none":
		// Mapped rather than dropped, and no document can tell the two apart.
		// "none" is the property's initial value, so a hint carrying it and no
		// hint at all produce the same computed value — a planted defect that
		// removed this case moved no test and no reftest. It is here because
		// HTML's rendering section names it, and because the day a user-agent
		// rule sets "clear" on a <br> the difference becomes real: a hint beats
		// a user-agent rule and an absent one does not.
		return "none", true
	}
	return "", false
}
