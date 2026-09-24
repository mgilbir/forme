package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// Generated content: the boxes ::before and ::after produce.
//
// These are the only boxes in the tree that no element generated and no
// anonymous-box rule required. They come from a declaration — "content" — which
// makes them the one place where the *stylesheet* adds to the document rather
// than describing it.
//
// A pseudo-element with no content generates nothing at all, which is not the
// same as generating an empty box: an empty inline would still contribute a line
// box, and a list of items each preceded by an invisible one would be spaced as
// though something were there.

// contentValue is what a "content" declaration resolves to.
//
// It is a *list* because a declaration is one: "content: counter(n) url(x.png)
// 'Before'" is a number, a picture and a word, in that order, and the picture is
// a box of its own with a size of its own between two runs of text. A single
// string could hold the words and could not hold the picture, which is what the
// value used to be and why an image in generated content was refused rather than
// drawn.
type contentValue struct {
	pieces []contentPiece
	// none reports that the declaration asks for no box at all, which "none"
	// and "normal" both do.
	none bool
	// unsupported reports a value that is correct CSS this engine cannot
	// produce.
	unsupported string
}

// contentPiece is one run of generated content: some text, or one image.
type contentPiece struct {
	// text is the run's characters, for a piece that is text.
	text string
	// image is the reference an image piece names. Exactly one of the two is
	// set, and image is the reference rather than the picture because nothing
	// here reads files — the loader that fetches every other picture in the
	// document fetches this one too, under the same caps and the same policy.
	image string
}

// text is everything the value sets, with the images left out. It is what a
// caller asking "does this produce any words" wants, and it is the whole value
// for the declarations that name no picture — which is nearly all of them.
func (v contentValue) text() string {
	if len(v.pieces) == 1 {
		return v.pieces[0].text
	}
	var b strings.Builder
	for _, p := range v.pieces {
		b.WriteString(p.text)
	}
	return b.String()
}

// maxContentLength bounds the text one pseudo-element's content may produce.
//
// Every other source of text in a document is at most as long as the document:
// a literal string is its own length, an attr() is the attribute's. The quote
// keywords are the first that are not, and the amplification is worth writing
// down rather than trusting to good taste. "quotes" holds two strings of the
// author's choosing and "content: open-quote open-quote …" draws one of them per
// keyword, so eleven bytes of declaration fetch a megabyte of mark — a factor of
// a hundred thousand, from a stylesheet, before any element has been matched.
//
// A megabyte of generated content is already past anything a page can show. The
// refusal is reported rather than truncated, because a marker cut off in the
// middle is a page that looks finished and is not.
//
// It is checked as each piece is written, and a piece is not written if it would
// cross it. It was checked between the tokens of the value instead, which let
// one token produce as much as it liked: counters() writes its separator once
// per level, so "counters(c, <ten kilobytes>)" two hundred levels deep was two
// megabytes from one token, and over two hundred nested elements 199 MB of text
// from ten kilobytes of stylesheet, all of it under a cap of one (audit C15).
//
// And it is per pseudo-element, so it bounds nothing about the document: the
// same declaration on every element is the cap times the elements. What bounds
// the document is its work budget, which every generated character is charged
// to — see boxBuilder.generated.
const maxContentLength = 1 << 20

// resolveContent reads a "content" declaration.
//
// Strings, attr(), counters, quotes and images are produced — an image as a
// piece of its own, between the runs of text either side of it. A url() this
// cannot resolve is refused and named, because it would otherwise be silently
// dropped and leave a marker missing from a page that still looks finished.
//
// depth is the level of quotation the value begins at and quotes the pairs to
// draw from; both come from the document walk, because neither is anything the
// pseudo-element can answer about itself. See quotes.go.
func resolveContent(raw string, el *html.Node, counters counterValues,
	quotes quoteList, depth int) contentValue {

	trimmed := ascii.TrimCSSSpace(raw)
	switch ascii.Lower(trimmed) {
	case "", "normal", "none":
		return contentValue{none: true}
	}

	vals, _ := css.ParseComponentValues(trimmed)
	// pieces are what the value produces, in order. text gathers the characters
	// of the run being built; an image ends that run and starts a new one, which
	// is what keeps "a" url(x) "b" three things rather than the two words with
	// the picture lost between them.
	var pieces []contentPiece
	var text strings.Builder
	// total is everything produced so far, which the loop below checks against
	// the cap. See flush.
	total := 0
	flush := func() {
		if text.Len() == 0 {
			return
		}
		// The run being ended is charged to the total, which is what makes the
		// total mean "everything produced so far". It did not: the run was
		// discarded from the counting when it was flushed, so a url() between
		// two quote keywords reset the only thing the cap could see. Four
		// megabytes of marks came out of a two-hundred-kilobyte stylesheet
		// with no finding, which is the amplification the cap's own comment
		// says it exists to stop.
		total += text.Len()
		pieces = append(pieces, contentPiece{text: text.String()})
		text.Reset()
	}
	// fits says whether n more bytes may be written, and every write asks it
	// first. See maxContentLength for why asking once per token was not enough.
	fits := func(n int) bool { return total+text.Len()+n <= maxContentLength }
	tooLong := contentValue{unsupported: "the content is longer than this engine will generate"}
	for _, v := range vals {
		if !fits(0) {
			return tooLong
		}
		switch {
		case v.IsToken() && v.Token.Kind == css.Whitespace:
			// Whitespace separates the parts of the value and contributes
			// nothing of its own.

		case v.IsToken() && v.Token.Kind == css.String:
			if !fits(len(v.Token.Value)) {
				return tooLong
			}
			text.WriteString(v.Token.Value)

		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "attr"):
			name := attrArgument(v)
			if name == "" {
				return contentValue{unsupported: "attr() without an attribute name"}
			}
			// A missing attribute contributes the empty string, which is what
			// the specification says and is why attr() is safe to use for
			// optional data.
			//
			// And "missing" is a different question in the two languages. HTML
			// lowercases an attribute name, so "attr(Title)" selects the title
			// attribute; XML does not, so it selects nothing. §12.2 leaves it to
			// the document language and the suite writes the same document twice
			// to say so — content-attr-case-001 in HTML asks for the match and
			// -002 in XHTML asks for its absence.
			value, _ := el.AttrNamed(name, el.XMLDocument())
			if !fits(len(value)) {
				return tooLong
			}
			text.WriteString(value)

		// The two refusals below no longer answer anything a stylesheet can
		// write: §12.2's grammar for these functions is checked where the sheet
		// is prepared, and a call with the wrong arguments takes the whole
		// declaration with it so that an earlier one stands. They are kept for
		// the reason parseQuotes gives for the same shape of check — a computed
		// style can be built by hand, and the initial value travels this path —
		// and TestResolveContentRefusesAMalformedCounterCall is the fixture that
		// keeps them from being a guard nobody has ever seen decide anything.
		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "counter"):
			name, listStyle, _, ok := counterArguments(v)
			if !ok {
				return contentValue{unsupported: "counter() without a counter name"}
			}
			// The innermost counter of that name. A counter that was never
			// created reads as zero, which is what §12.4.3 says and is better
			// than refusing: a document referring to a counter it forgot to
			// reset still numbers from its increments.
			vals := counters[name]
			n := 0
			if len(vals) > 0 {
				n = vals[len(vals)-1]
			}
			s := formatCounter(n, listStyle)
			if !fits(len(s)) {
				return tooLong
			}
			text.WriteString(s)

		case v.IsFunction() && ascii.EqualFold(v.Token.Value, "counters"):
			name, listStyle, sep, ok := counterArguments(v)
			if !ok || sep == nil {
				return contentValue{unsupported: "counters() needs a name and a separator"}
			}
			// Every counter of that name in scope, outermost first. This is what
			// numbers a nested list "2.1.3" — the three values are three
			// counters alive at once, one per level.
			//
			// Written a part at a time, each asked for first: the separator is
			// the stylesheet's and the number of levels is the document's, and
			// what they come to together is neither's to decide.
			for i, n := range counters[name] {
				s := formatCounter(n, listStyle)
				if i > 0 {
					if !fits(len(*sep)) {
						return tooLong
					}
					text.WriteString(*sep)
				}
				if !fits(len(s)) {
					return tooLong
				}
				text.WriteString(s)
			}

		case v.IsToken() && v.Token.Kind == css.URL,
			v.IsFunction() && ascii.EqualFold(v.Token.Value, "url"):
			// A picture, which is a box of its own between whatever runs of
			// text surround it. What is kept is the reference: the loader that
			// fetches every other picture in the document fetches this one too,
			// under the same caps and the same policy, and a second path to a
			// file would be a second policy — see image.go on backgrounds for
			// the same argument.
			ref := v.Token.Value
			if v.IsFunction() {
				s, ok := singleString(v.Values)
				if !ok {
					return contentValue{unsupported: "url() without a reference"}
				}
				ref = s
			}
			if ref = ascii.TrimSpace(ref); ref == "" {
				// url("") names nothing, exactly as an <img> with an empty src
				// does. There is no reference for a resolver to have refused,
				// and nothing to report.
				continue
			}
			if !fits(len(ref)) {
				return tooLong
			}
			flush()
			total += len(ref)
			pieces = append(pieces, contentPiece{image: ref})

		case v.IsToken() && v.Token.Kind == css.Ident:
			// §12.3.1's four keywords. The depth they move is the document's, not
			// this value's, which is why it arrived as an argument — but within one
			// value they still run in order, so "open-quote close-quote" draws a
			// matched pair and ends where it began.
			if op, isQuote := quoteKeyword(v.Token.Value); isQuote {
				var mark string
				mark, depth = applyQuote(op, depth, quotes)
				if !fits(len(mark)) {
					return tooLong
				}
				text.WriteString(mark)
				continue
			}
			return contentValue{unsupported: "\"" + v.Token.Value + "\" is not content this engine can produce"}

		default:
			return contentValue{unsupported: "a value this engine cannot produce"}
		}
	}
	flush()
	return contentValue{pieces: pieces}
}

// attrArgument reads the attribute name out of attr(name).
func attrArgument(fn css.ComponentValue) string {
	for _, v := range fn.Values {
		if v.IsToken() && v.Token.Kind == css.Ident {
			return v.Token.Value
		}
	}
	return ""
}

// addGenerated puts what a pseudo-element generates into box.
//
// That is its box, except where its display is "contents". css-display-3
// says the value makes an element generate no box while "its children and
// pseudo-elements still generate boxes and text runs as normal", and a
// pseudo-element is an element for the purpose: its content is what it would
// have held, and it goes into the box around it with nothing of the
// pseudo-element's own — no background, border or padding, no line of its
// own decoration — exactly as appendContents does for an element's children.
// The text keeps the pseudo-element's style, which is what it inherits. The
// value used to be read as "inline", and the box it made drew all of those.
//
// A pseudo-element whose content is one picture is a replaced element, whose
// content is not boxes, and the value is not honoured on it any more than on
// an <img>: see contentsIsHonoured. It keeps its box, and says so.
func (b *boxBuilder) addGenerated(box *Box, n *html.Node, name string, fontSize style.Unit) {
	g := b.generated(n, name, fontSize)
	if g == nil {
		return
	}
	kids := []*Box{g}
	if ascii.EqualFold(ascii.TrimCSSSpace(g.Style.Get("display")), "contents") {
		if g.ContentImage == "" {
			kids = g.Children
		} else {
			b.rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: "\"display: contents\" is not implemented on a ::" + name +
					" whose content is a picture; it was laid out as an inline box",
				Path:     PathOf(n),
				Property: "display",
			})
		}
	}
	for _, c := range kids {
		c.Parent = box
		box.Children = append(box.Children, c)
	}
}

// generated builds the box a pseudo-element produces, or nil.
func (b *boxBuilder) generated(n *html.Node, name string, fontSize style.Unit) *Box {
	key := style.PseudoKey{Node: n, Name: name}
	cs, ok := b.pseudo[key]
	if !ok {
		return nil
	}
	outer, inner, listItem := displayOf(cs)
	if outer == OuterNone {
		return nil
	}

	if b.generatedCut || b.counters.cut[key] {
		// The work budget refused generated content earlier, or refused this
		// pseudo-element's counters — and a number read as nought would be a
		// wrong number rather than a missing one. The budget has said so.
		return nil
	}
	raw := cs.Get("content")
	value := resolveContent(raw, n, b.counters.pseudo[key],
		parseQuotes(cs.Get("quotes")), b.counters.quoteDepth[key])
	// Charged to the document once it is known, which is before a box is made
	// of it: the value's own text read once, and what it produced at the rate
	// of everything downstream that will be done to it. maxContentLength bounds
	// one pseudo-element and nothing else; this is what bounds the document,
	// where the same declaration on every element is the whole cost.
	//
	// A refusal ends generated content for the document rather than for this
	// pseudo-element. The value has been resolved by now, which maxContentLength
	// bounds once; resolving the next one to be refused as well would be that
	// bound times the elements, which is the cost this is here to stop.
	produced := 0
	for _, p := range value.pieces {
		produced += len(p.text) + len(p.image)
	}
	if !b.rec.charge(satAdd(int64(len(raw)), satMul(int64(produced), costGeneratedByte)),
		"the generated content past that point") {
		b.generatedCut = true
		return nil
	}
	if value.unsupported != "" {
		b.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedValue,
			Source:   AtHTML(n.Offset),
			Message:  "the content of ::" + name + " was not produced: " + value.unsupported,
			Path:     PathOf(n),
			Property: "content",
		})
		return nil
	}
	if value.none {
		return nil
	}
	if !b.room(n) {
		return nil
	}
	order := b.count

	size := b.fontSizeOfStyle(cs, fontSize, b.ownPseudoFontSize[key])
	// A pseudo-element floats like any other box, and "p::before { content: '';
	// float: left }" is how a stylesheet makes a drop cap or a decorative rule
	// without adding an element to the markup — so the §9.7 blockification has
	// to reach here too.
	float := floatOf(cs)
	// A pseudo-element is positioned like any other box too, and the everyday
	// use of that is the same one: "::before { content: ''; position: absolute }"
	// is how a stylesheet draws an overlay without an element to hang it on.
	position := positionOf(cs)
	if position.outOfFlow() {
		float = FloatNone
	}
	// Whether it was inline-level *before* §9.7 blockified it for being out of
	// flow, which is what decides where its static position is. A pseudo-element
	// is inline by default, so an "::after { position: absolute }" has a
	// hypothetical static box on the line it was written on rather than a block
	// below it — see Box.staticInline. The element walk records this and this
	// walk did not, so every positioned pseudo-element read as block-level.
	staticInline := outer == OuterInline
	outer, inner = outOfFlowDisplay(outer, inner, float, position)
	z, zAuto := zIndexOf(cs)
	box := &Box{
		Outer: outer, Inner: inner, Element: n, Style: cs, Pseudo: name,
		ListItem: listItem, FontSize: size, fontSizeKnown: true,
		Float: float, Clear: clearOf(cs),
		Position: position, ZIndex: z, ZAuto: zAuto, Order: order,
		staticInline: staticInline,
	}
	// The text is collapsed exactly as document text is, by the pseudo-element's
	// own "white-space". Generated content is put in an anonymous inline box and
	// nothing exempts that box from CSS Text §4, so "content: 'a  b'" sets one
	// space and "content: 'a\Ab'" sets one line unless white-space says
	// otherwise — the same two answers the same characters would get in the
	// markup.
	//
	// This file used to say the opposite, and to pin it with a test: that the
	// author had written the characters they wanted, so "content: '  '" was a
	// legitimate way to indent a marker. It reads well and it is not what any
	// engine does; the indent is written with padding, and the suite says so
	// directly — content-white-space-004 puts a run of tabs and newlines in a
	// content string and asserts it sets identically to the same words in the
	// document.
	//
	// Only Phase I, as in textBox: the rules that cross a box boundary and the
	// rules that need a line are applied later, by the same passes that apply
	// them to everything else.
	// A picture that is the whole of the content makes the pseudo-element
	// itself a replaced element. CSS 2.1 §12.2: "if the value is a URI, the
	// pseudo-element is replaced by the object"; css-content-3 says the same of
	// a single <image>, and only of a single one.
	//
	// So the reference goes on this box rather than on a child of it, and what
	// follows from that is everything the property means: a width or a height
	// sizes the *picture*, and the border, padding and background are drawn once
	// around it. Built as a child carrying the pseudo-element's own style, they
	// were drawn twice — the pseudo-element's box around the image's box, both
	// bordered, both padded — and only the coincidence that a height applied to
	// the child stretches the image the same way it would stretch a replaced
	// pseudo-element kept the sizing right.
	if len(value.pieces) == 1 && value.pieces[0].image != "" {
		box.ContentImage = value.pieces[0].image
		return box
	}

	wst := b.wordSpaceTransformFor(cs)
	for _, piece := range value.pieces {
		if !b.room(n) {
			return box
		}
		if piece.image != "" {
			// A picture among other content is a replaced inline box of its
			// own: it has an intrinsic size and it sits on the line like an
			// <img>. The reference is carried on the box rather than fetched
			// here — see Box.ContentImage.
			//
			// Anonymous, so it takes the inherited properties and no others.
			// The pseudo-element's box is the pseudo-element's: a height on it
			// is the height of the line's box and not of the picture in it, and
			// its border is drawn once, around everything the content produced.
			// Handed the pseudo-element's whole style, "content: 'a' url(x) 'b'"
			// with a height stretched the picture to it — which is what a
			// *replaced* pseudo-element does, and this is not one.
			box.Children = append(box.Children, &Box{
				Outer: OuterInline, Inner: InnerFlow,
				Style: style.Inherited(cs), FontSize: size, fontSizeKnown: true,
				Parent: box, ContentImage: piece.image,
			})
			continue
		}
		text := collapseWhitespaceAfter(piece.text, cs.Get("white-space-collapse"), wst,
			textBoundary{}, b.writingSystemAt(n))
		if text == "" {
			continue
		}
		box.Children = append(box.Children, &Box{
			Outer: OuterInline, Inner: InnerText,
			Style: cs, Text: text, FontSize: size, fontSizeKnown: true, Parent: box,
		})
	}
	return box
}

// counterArguments reads the name, the optional list style and, for counters(),
// the separator out of a counter function.
//
// The two functions differ in their second argument: counter(name, style) takes
// a style there, counters(name, sep, style) takes the separator. Telling them
// apart by *type* rather than by position is what makes one reader serve both —
// a string is a separator and an identifier is a style, and neither can be
// mistaken for the other.
func counterArguments(fn css.ComponentValue) (name, listStyle string, sep *string, ok bool) {
	for _, v := range fn.Values {
		if !v.IsToken() {
			continue
		}
		switch v.Token.Kind {
		case css.Whitespace, css.Comma:
		case css.Ident:
			if name == "" {
				name = v.Token.Value
				continue
			}
			listStyle = v.Token.Value
		case css.String:
			s := v.Token.Value
			sep = &s
		}
	}
	return name, listStyle, sep, name != ""
}
