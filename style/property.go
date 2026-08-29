package style

import (
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
)

// The property registry, and the values a property can hold.
//
// # Why a registry at all
//
// Two questions have to be answerable for every property before a cascade can
// run, and neither can be worked out from a declaration: does this property
// inherit, and what is its value when nothing sets it. "color" inherits and
// "margin-top" does not, and no amount of looking at "color: red" reveals which.
//
// A third question is answerable only here and is the reason this file matters
// more than its size suggests: *is this property one the engine implements*. A
// renderer for a subset will meet declarations it does not act on, and §6.3 of
// the rendering proposal argues — correctly, and this is the cheapest guardrail
// it names — that dropping them silently is the worst available option. A page
// where "flex-wrap" was ignored is plausible and wrong, which is harder to
// notice than one that is obviously broken. So every declaration that is parsed
// and not applied is recorded.

// A property this engine knows about.
type property struct {
	// inherits says whether the computed value passes to children.
	inherits bool
	// initial is the value used when nothing in the cascade sets one, written
	// as it would be in a stylesheet.
	initial string
}

// properties is what the engine implements.
//
// A declaration whose name is not here is reported as unsupported and dropped —
// never quietly kept, and never quietly discarded. The set grows with the layout
// engine: a property is added here when something downstream acts on it, not
// when it is recognised, because a property that is stored and never read is
// indistinguishable to an author from one that was ignored.
var properties = map[string]property{
	// Box model.
	"display":        {false, "inline"},
	"width":          {false, "auto"},
	"height":         {false, "auto"},
	"min-width":      {false, "0"},
	"min-height":     {false, "0"},
	"max-width":      {false, "none"},
	"max-height":     {false, "none"},
	"margin-top":     {false, "0"},
	"margin-right":   {false, "0"},
	"margin-bottom":  {false, "0"},
	"margin-left":    {false, "0"},
	"padding-top":    {false, "0"},
	"padding-right":  {false, "0"},
	"padding-bottom": {false, "0"},
	"padding-left":   {false, "0"},
	"box-sizing":     {false, "content-box"},

	// Out-of-flow positioning by the one mechanism CSS had before there was a
	// positioning scheme. Neither inherits: a float that inherited would make
	// every descendant of a floated sidebar float too, which is the opposite of
	// what an author writing "float: left" on one box means.
	"float": {false, "none"},
	"clear": {false, "none"},

	// The positioning schemes of CSS 2.1 §9.3. None of them inherits, and
	// "position" is the one worth saying why about: a relative position that
	// reached every descendant would offset the subtree once per level, so a
	// paragraph three elements deep inside a box nudged 10px down would land
	// 30px down. The offset belongs to the box the author wrote it on.
	//
	// The initial value of the four offsets is "auto" rather than "0", and the
	// difference between those two is the whole of §10.3.7. A box with "left:
	// auto" is placed where the flow would have put it; one with "left: 0" is
	// pinned to its containing block's left padding edge. Reading auto as zero
	// would send every absolutely positioned box that names only a "top" to the
	// left edge of its containing block, which is a plausible-looking page and
	// the wrong one.
	"position": {false, "static"},
	"top":      {false, "auto"},
	"right":    {false, "auto"},
	"bottom":   {false, "auto"},
	"left":     {false, "auto"},

	// z-index's initial value is "auto" and not "0" for a reason of the same
	// shape: "0" makes the box a stacking context and "auto" leaves it in its
	// parent's, so a descendant with a negative z-index paints behind an
	// ancestor with "z-index: auto" and in front of one with "z-index: 0".
	// Collapsing the two would make that descendant unreachable.
	"z-index": {false, "auto"},

	// Borders.
	"border-top-width":    {false, "medium"},
	"border-right-width":  {false, "medium"},
	"border-bottom-width": {false, "medium"},
	"border-left-width":   {false, "medium"},
	"border-top-style":    {false, "none"},
	"border-right-style":  {false, "none"},
	"border-bottom-style": {false, "none"},
	"border-left-style":   {false, "none"},
	// CSS 2.1 §18.4. An outline is drawn just outside the border edge and takes
	// no space, so it is three properties and no layout at all — which is what
	// makes it separable from the border it otherwise resembles.
	//
	// The initial colour is "invert", not "currentcolor". CSS 2.1 asks for the
	// pixels underneath to be inverted, which a display list of fills cannot
	// express without reading back what it has already drawn; it is reported
	// rather than approximated, because an outline in the wrong colour is worse
	// than a caller being told it could not be drawn. See checkOutline.
	// CSS 2.1 §12.6.2. An image marker replaces the one list-style-type would
	// have drawn — and only while it is *available*, which is why the type is
	// still cascaded and still read: a url that does not load falls back to it.
	"list-style-image": {true, "none"},

	"outline-width": {false, "medium"},
	"outline-style": {false, "none"},
	"outline-color": {false, "invert"},

	"border-top-color":    {false, "currentcolor"},
	"border-right-color":  {false, "currentcolor"},
	"border-bottom-color": {false, "currentcolor"},
	"border-left-color":   {false, "currentcolor"},

	// Text and fonts. Most of these inherit, which is the whole reason
	// inheritance exists: setting a font on <body> has to reach the text.
	"color":          {true, "black"},
	"font-family":    {true, "serif"},
	"font-size":      {true, "medium"},
	"font-style":     {true, "normal"},
	"font-weight":    {true, "normal"},
	"line-height":    {true, "normal"},
	"letter-spacing": {true, "normal"},
	"word-spacing":   {true, "normal"},
	// text-align-all is where every line but the last is aligned — CSS Text 4
	// §7.1. The property an author writes is "text-align", which is the
	// shorthand for this and text-align-last; see the shorthands table.
	//
	// Splitting them is not a reshuffle for its own sake, and the argument is
	// white-space's exactly. "text-align: center" and "text-align-all: center"
	// set the same thing, and unless they set the same *longhand* the cascade
	// cannot decide between them: the answer would come down to which of the
	// two layout happened to read second, and an author who wrote one after the
	// other would get whichever this engine preferred rather than the later one.
	"text-align-all": {true, "start"},
	// text-align-last is the last line's own alignment — CSS Text 3 §7.2 — and
	// it inherits, so a block sets it for the paragraphs inside it. "auto"
	// means "whatever the block is aligned as", except that a justified block's
	// last line is placed where "start" would put it rather than stretched.
	"text-align-last": {true, "auto"},
	// text-justify chooses *how* a justified line is stretched — §7.3. The
	// engine spreads the word spaces, which is what "auto" and "inter-word"
	// ask for; "none" turns justification off and is acted on; the rest are
	// read as auto and reported, because a page justified the wrong way is a
	// page that looks right and is not.
	"text-justify":   {true, "auto"},
	"text-indent":    {true, "0"},
	"text-transform": {true, "none"},
	// white-space is a shorthand in CSS Text 4 — see the shorthands table — and
	// these are the two longhands it sets that this engine acts on. The third,
	// white-space-trim, is not registered because nothing trims yet, and
	// registering a property nothing reads is what the note above this table
	// warns against.
	"white-space-collapse": {true, "collapse"},
	// text-wrap-mode is the wrapping half of white-space, and it is a property in
	// its own right because "text-wrap: nowrap" sets it without saying anything
	// about collapsing. Splitting them is what lets the cascade decide between
	// the two spellings by ordinary means, rather than by one of them being
	// consulted after the other in the layout code.
	"text-wrap-mode": {true, "wrap"},
	// text-wrap-style chooses *how* the breaks are picked among the ones
	// text-wrap-mode allows. "balance" is implemented; the others are read as
	// auto and reported.
	"text-wrap-style": {true, "auto"},
	// line-clamp cuts a block off after a number of lines and marks the cut.
	// CSS Overflow 4 defines it as a shorthand for max-lines, block-ellipsis and
	// continue; the three are not registered separately because the only
	// combination this engine acts on is the one the shorthand's integer form
	// sets, and a longhand nothing reads is worse than no longhand.
	//
	// It does not inherit: it says where *this* block stops, and a paragraph
	// inside a clamped one is not itself clamped.
	"line-clamp": {false, "none"},
	// The prefixed form, and the property that makes it apply. CSS Overflow 4's
	// compatibility section defines the legacy behaviour as this trio —
	// "display: -webkit-box", "-webkit-box-orient: vertical" and
	// "-webkit-line-clamp: <integer>" — so all three are read, and reading them
	// is what keeps a document that writes the old spelling from being reported
	// as using three things this engine ignores.
	"-webkit-line-clamp": {false, "none"},
	"-webkit-box-orient": {false, "horizontal"},
	// overflow-wrap inherits. word-wrap is the name Internet Explorer shipped it
	// under and is a legal alias in CSS Text §5.5, so it is registered rather
	// than reported: a document using it is not using an unsupported property.
	"overflow-wrap": {true, "normal"},
	"word-wrap":     {true, "normal"},
	// word-break inherits, which is what makes a rule on a container reach the
	// text in it. Only "normal" and "break-all" are acted on; the two values
	// this engine does not distinguish are read as normal and reported, because
	// "keep-all" changes where CJK text may break and getting it wrong silently
	// is a line broken in the middle of a word.
	"word-break": {true, "normal"},
	// word-space-transform inherits, which is how a rule on a container reaches
	// the marks inside it — the property's own test -003 asks for exactly that.
	// Its "auto-phrase" half is reported where it is read: inventing word
	// boundaries a document did not mark needs a dictionary.
	"word-space-transform": {true, "none"},
	// line-break inherits too, and only "anywhere" is acted on. The other three
	// — loose, normal and strict — differ from auto in how strictly CJK text may
	// break around small kana and punctuation, which this engine does not model
	// at all; they are read as auto and reported only over text where the
	// difference could show, since the suite has tests asserting in as many words
	// that they change nothing about Latin text.
	"line-break": {true, "auto"},
	// hyphens inherits, so a rule on an article reaches every word in it —
	// which is how a document turns hyphenation off, and the only reason
	// "hyphens: none" on a container means anything. "manual" is implemented
	// and is the initial value; "auto" is read as manual and reported, because
	// hyphenating a word that contains no soft hyphen needs a dictionary for
	// the document's language.
	"hyphens": {true, "manual"},
	// The two kerning properties inherit, and both are decided in layout rather
	// than here, because whether either asks for anything depends on the face
	// that will set the text — which the cascade has not chosen yet. A face with
	// no kerning in it cannot have its kerning turned off. See
	// layout/textchecks.go.
	"font-kerning":          {true, "auto"},
	"font-feature-settings": {true, "normal"},
	// text-autospace inherits, which is what lets a document turn it off once
	// on the body. Its initial value is "normal", and "normal" asks for the
	// spacing — a page of Japanese with Latin words in it is set wrong without
	// it, so the property is on until something says otherwise. See
	// paragraph/autospace.go.
	"text-autospace": {true, "normal"},
	// hanging-punctuation inherits, which is what lets a document ask for it
	// once on the body and mean it for every paragraph. "first" and "last" are
	// implemented; the two end values are read as none and reported.
	"hanging-punctuation": {true, "none"},
	// hyphenate-character inherits, and it is a <string> rather than a keyword:
	// "auto" lets the engine choose and anything else is what to print, an empty
	// string included — which is how a document asks for a word to be broken with
	// no mark at all, and is a real value rather than a way of writing nothing.
	"hyphenate-character": {true, "auto"},
	// hyphenate-limit-chars inherits, and it is three numbers: how long a word
	// must be before it may be broken at all, how many of its letters must stay
	// on the first line, and how many must go to the next. "auto" for any of
	// them leaves that one to the engine, which takes the hyphenmins the
	// language's own pattern file states — see paragraph/hyphenate.go.
	"hyphenate-limit-chars": {true, "auto"},
	// tab-size inherits, which is the answer that makes a <pre> inside a
	// styled <article> keep the tab width the author set on the article. A
	// number is a count of space advances and a length is itself; the initial
	// value is 8, which is what a tab has meant since terminals had one.
	"tab-size": {true, "8"},
	// css-text-5's text-fit scales the size a block's text is set in so that
	// its lines fill the box. Inherited, because a block container inside one
	// that fits its text fits its own — see layout/textfit.go.
	"text-fit":              {true, "none"},
	"text-decoration-line":  {false, "none"},
	"text-decoration-color": {false, "currentcolor"},
	"vertical-align":        {false, "baseline"},
	"direction":             {true, "ltr"},
	"unicode-bidi":          {false, "normal"},

	// Generated content. It does not inherit — a ::before on a parent must not
	// give every descendant the same marker.
	"content": {false, "normal"},
	// The quotation marks §12.3.1's keywords draw from. This one *does* inherit,
	// and has to: a quotation is nested inside the element that set the pairs, and
	// the ::before that draws the mark is a child of a child of it.
	//
	// CSS 2.1 leaves the initial value to the user agent. The English typographic
	// pairs are what is chosen here rather than "none", because "none" would make
	// "content: open-quote" in a document that never set the property draw nothing
	// at all — a silent no-op that looks exactly like the feature being missing,
	// which is the failure mode this engine's whole reporting design is against.
	// Nothing in the CSS 2.1 test suite depends on the choice: every test that uses
	// the keywords sets the property, save one that uses no-open-quote and so
	// draws nothing whatever the list says.
	"quotes": {true, `"“" "”" "‘" "’"`},

	// Backgrounds.
	//
	// The two initial values worth pausing on are the ones a reader assumes are
	// the same and are not: the *origin* is the padding box and the *clip* is the
	// border box. So a background image starts at the inside edge of the border
	// and is painted out under the border as well — which is what makes a dashed
	// border show the image through its gaps, and what makes a box with a wide
	// transparent border tile from a different place than an implementation that
	// collapsed the two would put it.
	"background-color":      {false, "transparent"},
	"background-image":      {false, "none"},
	"background-repeat":     {false, "repeat"},
	"background-attachment": {false, "scroll"},
	"background-position":   {false, "0% 0%"},
	"background-size":       {false, "auto"},
	"background-origin":     {false, "padding-box"},
	"background-clip":       {false, "border-box"},
	// The counters. Neither inherits: a counter's value comes from the walk in
	// counter.go, and inheriting the declaration would make every descendant
	// increment it again.
	"counter-reset":     {false, "none"},
	"counter-increment": {false, "none"},

	// Lists.
	"list-style-type":     {true, "disc"},
	"list-style-position": {true, "outside"},

	// Tables.
	"border-collapse": {true, "separate"},
	"border-spacing":  {true, "0"},
	"caption-side":    {true, "top"},
	"empty-cells":     {true, "show"},
	"table-layout":    {false, "auto"},

	// Visibility, overflow and clipping.
	"visibility": {true, "visible"},
	"overflow-x": {false, "visible"},
	"overflow-y": {false, "visible"},
	// CSS 2.1 §11.1.2. It does not inherit, which is what makes "clip: inherit"
	// on a positioned child of a box that declared a rect() a test worth having
	// — the value has to travel by the keyword rather than by default.
	"clip":    {false, "auto"},
	"opacity": {false, "1"},
}

// Inherited returns the style an anonymous box has: everything that inherits
// taken from the box it was generated inside, and everything that does not at
// its initial value.
//
// This is what the specification means by an anonymous box having no style of
// its own. It matters far more than it sounds, because the obvious shortcut —
// giving the anonymous box its parent's whole computed style — makes it a copy
// of the parent's *box model* as well: the anonymous block wrapped around a run
// of text inside <body> would take body's 8px margin, indent the text by it, and
// separate it from the block after it by a gap the author never wrote. Every
// number in that document is then plausible and wrong.
// Undeclared is the value a property has on a box whose style declares nothing
// about it, given the parent's computed value: the parent's where the property
// inherits and the property's initial value where it does not.
//
// It exists so that a caller can tell a value the author *wrote* from one that
// merely arrived. A pseudo-element's computed style holds every property in the
// registry, and comparing it against the originating element's says the wrong
// thing for the properties that do not inherit — a "::first-line" of an element
// with a red background reads as declaring "background-color: transparent",
// which nobody wrote. layout/firstline.go is the caller.
func Undeclared(name, parent string) string {
	p, ok := properties[name]
	if !ok {
		return ""
	}
	if p.inherits {
		return parent
	}
	return p.initial
}

func Inherited(cs ComputedStyle) ComputedStyle {
	out := make(ComputedStyle, len(properties))
	for name, prop := range properties {
		if prop.inherits {
			if v, ok := cs[name]; ok {
				out[name] = v
				continue
			}
		}
		out[name] = prop.initial
	}
	return out
}

// shorthands expands a shorthand into the longhands it sets.
//
// Expansion happens before the cascade rather than after, and that ordering is
// not arbitrary: "margin: 0" followed by "margin-top: 1em" must leave the top
// margin at 1em, which only works if the shorthand has already become four
// declarations competing individually. Cascading the shorthand as a unit would
// make the later longhand lose to it or win over all four.
// expander turns a shorthand's value into the longhands it sets.
//
// It returns three things rather than two, and the third is the point:
// unsupported names the parts of the value this engine understood and cannot
// produce — a background image, a font variant. Dropping those silently is the
// failure §6.3 is written about, and only the expander knows what it saw.
type expander func(vals []css.ComponentValue) (longhands map[string][]css.ComponentValue, unsupported []string, ok bool)

// shorthand is an expander together with the longhands it controls.
//
// The list is declared rather than discovered. An earlier version asked the
// expander itself, by handing it a value and seeing which keys came back — which
// worked only while every expander accepted the same probe value, and stopped
// the moment one of them started rejecting values it could not identify. The
// list is needed for the CSS-wide keywords ("border: inherit" sets all twelve
// longhands to inherit), where there is no value to probe with at all.
type shorthand struct {
	expand    expander
	longhands []string
}

var shorthands = map[string]shorthand{
	"margin":  boxShorthand("margin-top", "margin-right", "margin-bottom", "margin-left"),
	"padding": boxShorthand("padding-top", "padding-right", "padding-bottom", "padding-left"),
	"border-width": boxShorthand("border-top-width", "border-right-width",
		"border-bottom-width", "border-left-width"),
	"border-style": boxShorthand("border-top-style", "border-right-style",
		"border-bottom-style", "border-left-style"),
	"border-color": boxShorthand("border-top-color", "border-right-color",
		"border-bottom-color", "border-left-color"),
	"overflow": boxShorthand("overflow-x", "overflow-y"),

	// The shorthands whose parts are told apart by type rather than position.
	// They live in shorthand.go, with the reset rule explained there.
	"border":        borderSides("top", "right", "bottom", "left"),
	"outline":       {outlineShorthand, []string{"outline-width", "outline-style", "outline-color"}},
	"border-top":    borderSides("top"),
	"border-right":  borderSides("right"),
	"border-bottom": borderSides("bottom"),
	"border-left":   borderSides("left"),

	"background": {backgroundShorthand, []string{
		"background-color", "background-image", "background-repeat",
		"background-attachment", "background-position", "background-size",
		"background-origin", "background-clip"}},
	"list-style": {listStyleShorthand,
		[]string{"list-style-type", "list-style-position", "list-style-image"}},
	"font": {fontShorthand, []string{
		"font-style", "font-weight", "font-size", "font-family", "line-height"}},
	"text-decoration": {textDecorationShorthand,
		[]string{"text-decoration-line", "text-decoration-color"}},

	// CSS Text 4 makes white-space a shorthand, and that is not a reshuffle for
	// its own sake: "text-wrap: nowrap" and "white-space: nowrap" set the same
	// thing, and unless they set the same *longhand* the cascade cannot decide
	// between them and the answer comes down to which one layout happens to read
	// second.
	"white-space": {whiteSpaceShorthand,
		[]string{"white-space-collapse", "text-wrap-mode"}},
	"text-wrap":  {textWrapShorthand, []string{"text-wrap-mode", "text-wrap-style"}},
	"text-align": {textAlignShorthand, []string{"text-align-all", "text-align-last"}},
}

func init() {
	// The logical shorthands, declared in logical.go beside the longhands they
	// expand into. They are added here rather than written into the table above
	// so that the whole of css-logical is in one file: the table above is what
	// CSS 2.1 has, and this is the level on top of it.
	for name, sh := range logicalShorthands {
		shorthands[name] = sh
	}
}

// boxShorthand builds the expander for a property written as one to four values
// in the order top, right, bottom, left — where one value sets all four, two set
// the vertical and horizontal pairs, and three leave the left to mirror the
// right.
//
// The two-name form is the same rule with two slots, which is what "overflow"
// needs.
// borderSides builds the "border" family, whose longhands are three per side.
func borderSides(sides ...string) shorthand {
	var names []string
	for _, side := range sides {
		names = append(names,
			"border-"+side+"-width", "border-"+side+"-style", "border-"+side+"-color")
	}
	return shorthand{borderShorthand(sides...), names}
}

func boxShorthand(names ...string) shorthand {
	return shorthand{boxExpander(names...), names}
}

func boxExpander(names ...string) expander {
	return func(vals []css.ComponentValue) (map[string][]css.ComponentValue, []string, bool) {
		parts := splitOnWhitespace(vals)
		if len(parts) == 0 || len(parts) > len(names) {
			return nil, nil, false
		}
		out := make(map[string][]css.ComponentValue, len(names))
		switch len(names) {
		case 2:
			out[names[0]] = parts[0]
			out[names[1]] = parts[len(parts)-1]
		default:
			top := parts[0]
			right := top
			if len(parts) > 1 {
				right = parts[1]
			}
			bottom := top
			if len(parts) > 2 {
				bottom = parts[2]
			}
			left := right
			if len(parts) > 3 {
				left = parts[3]
			}
			out[names[0]], out[names[1]] = top, right
			out[names[2]], out[names[3]] = bottom, left
		}
		return out, nil, true
	}
}

// splitOnWhitespace divides a value into its space-separated parts.
func splitOnWhitespace(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	var cur []css.ComponentValue
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			if len(cur) > 0 {
				out = append(out, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, v)
	}
	if len(cur) > 0 {
		out = append(out, cur)
	}
	return out
}

// The four CSS-wide keywords, which every property accepts and which mean the
// same thing for all of them. They are handled by the cascade rather than by any
// property's own parsing.
const (
	kwInherit = "inherit"
	kwInitial = "initial"
	kwUnset   = "unset"
	kwRevert  = "revert"
)

// wideKeyword returns the CSS-wide keyword a value consists of, or "".
//
// It has to be the *whole* value: "border: 1px solid initial" is not a use of
// the keyword, it is a declaration with a stray word in it.
func wideKeyword(vals []css.ComponentValue) string {
	parts := splitOnWhitespace(vals)
	if len(parts) != 1 || len(parts[0]) != 1 {
		return ""
	}
	v := parts[0][0]
	if !v.IsToken() || v.Token.Kind != css.Ident {
		return ""
	}
	switch kw := strings.ToLower(v.Token.Value); kw {
	case kwInherit, kwInitial, kwUnset, kwRevert:
		return kw
	}
	return ""
}

// serialize renders component values back to text.
//
// The cascade stores winning values as text rather than as component values, for
// one reason worth stating: a computed value has to be *comparable*, and two
// slices of component values that mean the same thing are not equal in Go. The
// styled tree is compared in tests, cached on, and eventually diffed against a
// reference, and all three want a value that can be a map key.
func serialize(vals []css.ComponentValue) string {
	var b strings.Builder
	writeValues(&b, vals)
	return strings.TrimSpace(b.String())
}

func writeValues(b *strings.Builder, vals []css.ComponentValue) {
	for _, v := range vals {
		writeValue(b, v)
	}
}

func writeValue(b *strings.Builder, v css.ComponentValue) {
	t := v.Token
	switch {
	case v.IsFunction():
		b.WriteString(t.Value)
		b.WriteByte('(')
		writeValues(b, v.Values)
		b.WriteByte(')')
		return
	case v.IsBlock():
		open, close := "(", ")"
		switch t.Kind {
		case css.LeftSquare:
			open, close = "[", "]"
		case css.LeftBrace:
			open, close = "{", "}"
		}
		b.WriteString(open)
		writeValues(b, v.Values)
		b.WriteString(close)
		return
	}

	switch t.Kind {
	case css.Ident, css.Delim:
		b.WriteString(t.Value)
	case css.AtKeyword:
		b.WriteString("@" + t.Value)
	case css.Hash:
		b.WriteString("#" + t.Value)
	case css.String:
		// Quoted, because a string that serialised bare would be
		// indistinguishable from an identifier — and "none" the keyword is not
		// "none" the font family.
		writeCSSString(b, t.Value)
	case css.URL:
		b.WriteString("url(" + t.Value + ")")
	case css.Number:
		b.WriteString(t.Repr)
	case css.Percentage:
		b.WriteString(t.Repr + "%")
	case css.Dimension:
		b.WriteString(t.Repr + t.Unit)
	case css.Whitespace:
		b.WriteByte(' ')
	case css.Colon:
		b.WriteByte(':')
	case css.Semicolon:
		b.WriteByte(';')
	case css.Comma:
		b.WriteByte(',')
	case css.BadString, css.BadURL:
		// A value that did not tokenize cannot be rendered back; anything
		// written here would be a value the author did not type.
		b.WriteString("<invalid>")
	}
}

// writeCSSString renders a string value as a CSS <string> token.
//
// It is CSS Syntax's "serialize a string" and not a shorter approximation of it,
// because this text is *re-parsed*: the cascade keeps a winning value as text,
// and every reader of that value tokenizes it again. Anything the escaping loses
// is lost for good, and it is lost quietly — the value comes back as something
// the author could have typed, so nothing anywhere reports a problem.
//
// Two characters make that concrete, and both were live faults before this
// existed:
//
//   - A newline. "content: 'a\Ab'" is how a stylesheet puts a line break in
//     generated content, and a raw newline written back between quotes does not
//     tokenize at all — a CSS string may not span lines. The value came back as
//     a bad-string and the declaration was reported as one this engine cannot
//     produce, which was true of the serialisation and not of the engine.
//
//   - A backslash. "content: 'a\\b'" is one backslash between two letters;
//     written back unescaped it reads as "\b", which is the escape for U+000B,
//     so the value silently became a vertical tab. That one is worse than the
//     newline, because nothing is reported: the page renders, with the wrong
//     character in it.
//
// The rules are the specification's: NULL becomes U+FFFD, a control character
// becomes a hexadecimal escape, and a quote or a backslash is escaped with a
// backslash. The space after a hexadecimal escape is required — without it "\a"
// followed by a literal "b" would read back as the single escape "\ab".
func writeCSSString(b *strings.Builder, s string) {
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == 0:
			// A NULL never survives tokenizing, so this cannot arise from a
			// parsed stylesheet; it is here because the rule is part of the
			// definition, and a caller may hand over a string from elsewhere.
			b.WriteRune('�')
		case r <= 0x1F || r == 0x7F:
			b.WriteByte('\\')
			b.WriteString(strconv.FormatInt(int64(r), 16))
			b.WriteByte(' ')
		case r == '"' || r == '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
}
