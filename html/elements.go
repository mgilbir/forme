package html

// The element tables: what this engine knows, what it refuses, and the few
// places where HTML lets an end tag be left out.
//
// These are data rather than code on purpose. The subset boundary is the design
// decision this package exists to enforce, and a boundary spread through
// conditionals is one nobody can read off.

// knownElements is everything this package has something to say about: an
// element whose name decides how it nests, what it may contain, or what box it
// makes.
//
// An element outside this set is *not* refused. It is kept, and laid out as the
// ordinary inline box HTML gives it — see the note at insertUnknown, which is
// where that decision is argued. Refusing it is what this used to do, on the
// reading that an element the engine does not know is one it cannot lay out.
// That is true of <video>, which needs something this engine does not have and
// is refused above by name; it was never true of a custom element, where
// dropping the tag threw away every rule the author had written for it, and it
// was not true of <canvas> either — see the entry for it, and the one for
// <iframe> beside it.
//
// So membership here is a statement about parsing and not about acceptance. The
// elements this engine will not render are droppedElements, each with its own
// reason, and they are named rather than left to be inferred from an absence.
var knownElements = map[string]bool{
	// Document structure.
	"html": true, "head": true, "body": true,
	// Foreign roots. They are replaced elements: a box with content that is
	// not HTML. See foreignElements.
	"svg": true, "math": true,
	"title": true, "meta": true, "link": true, "base": true, "style": true,
	// <noscript> is what an author writes for a reader whose engine runs no
	// script, and this engine runs no script. It was dropped, with the reason
	// "there is no script for this to be an alternative to" — which is exactly
	// backwards: a document is *always* in the case the element was written
	// for, so its content is always what should be shown. HTML says the same
	// thing structurally, by parsing a noscript's content as ordinary markup
	// wherever scripting is disabled.
	//
	// It has no presentation of its own. Like a <span> it is an inline box that
	// inherits everything and that a stylesheet may select, which is what a
	// browser with scripting off gives it.
	"noscript": true,

	// Sections and grouping.
	"address": true, "article": true, "aside": true, "blockquote": true,
	"div": true, "dl": true, "dt": true, "dd": true, "figcaption": true,
	"figure": true, "footer": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "header": true, "hgroup": true,
	"hr": true, "li": true, "main": true, "nav": true, "ol": true, "p": true,
	"pre": true, "section": true, "ul": true,

	// Tables.
	"table": true, "caption": true, "colgroup": true, "col": true,
	"thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true,
	"th": true,

	// Text-level semantics.
	"a": true, "abbr": true, "b": true, "bdi": true, "bdo": true, "br": true,
	"cite": true, "code": true, "data": true, "del": true, "dfn": true,
	"em": true, "i": true, "ins": true, "kbd": true, "mark": true, "q": true,
	"rp": true, "rt": true, "ruby": true, "s": true, "samp": true,
	"small": true, "span": true, "strong": true, "sub": true, "sup": true,
	"time": true, "u": true, "var": true, "wbr": true,

	// The obsolete presentational elements, which are not unknown tags.
	//
	// The rule above is about an element nobody has defined: rendering a
	// <fancy-callout> as a generic inline produces a page that looks nearly
	// right and says nothing. These are the opposite case. HTML's rendering
	// section still gives each of them a box and a rule — <tt> is monospace,
	// <nobr> does not wrap, <big> is larger, <center> is a centred block, <font>
	// carries three presentational attributes — so laying one out is following
	// the specification rather than guessing at it.
	//
	// Refusing them cost the content as well as the presentation: content-063,
	// -076 and -136 each put a ::before on a <font> and ask for the attribute it
	// names, and an element that is not laid out has no ::before at all.
	"font": true, "tt": true, "nobr": true, "big": true, "center": true,
	"strike": true, "acronym": true,

	// Images.
	"img": true, "picture": true, "source": true,

	// <iframe>, for its box. The nested browsing context is refused and always
	// will be — see §4.1 — but the element is a replaced one whose box is on the
	// page whether or not anything was loaded into it, and that box is what
	// every reftest using an iframe is actually about. See
	// contentSkippedElements.
	"iframe": true,

	// Three more the same argument reaches, each refused for what it *does* and
	// each an ordinary box while it does it.
	//
	// <output> is an inline element and nothing else. "An output is computed by
	// script" describes what fills one, not what one is: a document that writes
	// "<output>42</output>" has written the 42, and dropping the element threw
	// the reader's own text away.
	//
	// <slot> renders its children where there is no shadow tree to fill it, and
	// there never is one here. HTML gives it "display: contents", which is a
	// value this engine honours, so the fallback content it holds reaches the
	// page as the specification asks.
	//
	// <marquee> animates, and a page laid out once shows it standing still —
	// which is what a browser asked to print one does. What it must not do is
	// lose the words.
	//
	// What stays refused, and why it is not this: <details> and <summary> need a
	// disclosure triangle and a rule that hides a closed element's content;
	// <dialog> needs the same for a closed one; <audio>, <progress> and <meter>
	// each need a widget drawn from a state. Every one of those is a thing to
	// build rather than a refusal to lift.
	//
	// Membership here is documentation for these three, as it is for <map>
	// below: the parser has no rule about how any of them nests. What changed is
	// that none is in droppedElements.
	"output": true, "slot": true, "marquee": true,

	// <map> and <area>, which are markup about *where a reader may click* and
	// nothing else — and this engine's pages are not clicked. The image map is
	// refused and always will be, on the same footing as an iframe's browsing
	// context: nothing here turns a rectangle into a link.
	//
	// What was refused with it is a box, and it should not have been. A <map> is
	// an ordinary inline box holding whatever the author put in it, and an
	// <area> is hidden by HTML's own rendering section rather than by anything
	// this engine decided — which is a rule a stylesheet may overrule, and one
	// the suite's content-100 does overrule: "area { display: block }" with a
	// ":before" on it, checked for the word its attribute holds. Dropping the
	// element threw away that content along with the click.
	//
	// Membership here changes nothing on its own for these two: the parser has
	// no rule about how either nests, and <area> is already among the void
	// elements. What changed is that neither is in droppedElements any more.
	// They are listed because this table is where that boundary is argued — the
	// form controls and <iframe> above are here for the same reason — and a
	// removal leaves no place to say why.
	"map": true, "area": true,

	// <canvas>, for the same reason and by the same argument, which this table
	// has now made three times.
	//
	// It was dropped, under "a canvas is drawn by script, which is never run".
	// That is a true sentence about the *bitmap* and a false one about the
	// element: a canvas is a replaced element whose intrinsic dimensions are its
	// own width and height attributes — 300 by 150 when it states none — and
	// that box is on the page whether or not anything was ever drawn into it. A
	// browser with scripting turned off does not omit the canvas and does not
	// show its fallback content; it lays out a blank one of exactly that size,
	// and so does this.
	//
	// Nothing is missing from such a page, so nothing is reported. A canvas a
	// script would have painted is a page whose <script> was thrown away, and
	// droppedElements already says so where it happened; saying it twice would
	// taint every document holding an empty canvas with a finding about a
	// picture that was never going to exist.
	"canvas": true,

	// <video>, for its box, which is the same argument the <iframe> above is
	// here for and the same one that moved the form controls.
	//
	// It was refused under "a page laid out once cannot play anything", and that
	// is true of *playing* and says nothing about layout. A video element is a
	// replaced element: HTML §4.8.9 gives it the poster's intrinsic dimensions
	// where there is one and the default object size — CSS 2.1 §10.3.2's 300 by
	// 150 — where there is not, and a browser asked to print a page with a video
	// on it prints that box. Dropping the element threw the box away, and a
	// reftest about what paints over a video cannot be about anything when
	// nothing was painted.
	//
	// What is still refused is the film and the control bar, and each is
	// reported where it is refused rather than here. A <video> naming no media
	// and asking for no controls has nothing missing from it at all, which is
	// the half that matters: see layout's video loader.
	"video": true,

	// Forms, as static boxes.
	//
	// These are here for what they *are* on a page rather than for what they do
	// on a screen. The reason they used to be refused — "form controls are not
	// interactive here" — is about interactivity, and interactivity is not what
	// a printed page has: a <textarea> has an intrinsic size from its cols and
	// rows, it has text in it, and a browser asked to print one puts that text
	// on the paper inside a box. Refusing the element meant a document lost
	// content and said only that an element had been dropped, which is the
	// class of fault this engine reports everywhere else rather than commits.
	//
	// What stays refused is the interaction itself, and it is not a boundary
	// this moves: nothing is submitted, nothing is typed into, no value a reader
	// would have entered is invented, and no PDF form field is produced. See
	// layout/control.go for what each control is drawn as and for the findings
	// that name the places where a static box is an approximation of a widget.
	"form": true, "label": true, "fieldset": true, "legend": true,
	"input": true, "button": true, "select": true, "option": true,
	"optgroup": true, "textarea": true,

	// <object>, for its fallback content. Nothing is embedded — see
	// resolveObjects — but HTML says an object whose data cannot be used is
	// represented by its children, and those children are ordinary markup this
	// engine can lay out. <param> comes with it and generates no box.
	"object": true, "param": true,
}

// voidElements have no content and no end tag. Writing one — "</br>" — is an
// error rather than something to be ignored.
//
// The last four are obsolete and are void all the same: the tree builder
// inserts each and pops it at once, so "<basefont>A" is an element and then a
// letter beside it, not a letter inside an element that is never closed.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
	"basefont": true, "bgsound": true, "frame": true, "keygen": true,
}

// rawTextElements have content that is not markup at all: it runs to the
// matching end tag, and neither "<" nor "&" means anything inside.
//
// The four beyond style and script are the ones HTML kept from before it had a
// parser worth the name. <xmp> is what <pre> used to be and is still written by
// generators that predate it; <noembed> and <noframes> hold what to show
// instead of something this engine does not do; <plaintext> is the oldest of
// all and turns the rest of the document into text.
//
// They were read as markup, so "<xmp><b>not bold</b></xmp>" — which says on its
// face that the tags in it are to be shown — came out with a bold word in it and
// the tags gone.
var rawTextElements = map[string]bool{
	"style": true, "script": true,
	"xmp": true, "noembed": true, "noframes": true, "plaintext": true,
}

// rcdataElements have content with no markup but with character references,
// so "&amp;" in a <title> is an ampersand.
var rcdataElements = map[string]bool{
	"title": true, "textarea": true,
}

// contentSkippedElements are laid out as boxes and have their content thrown
// away, which is not the same thing as being dropped.
//
// <iframe> is the one, and it was in droppedElements until the numbers said what
// that cost. An iframe is a *replaced element*: it has a box, and with no
// intrinsic dimensions that box is 300 by 150 — which is where those two numbers
// in layout/replaced.go came from in the first place. Dropping the element threw
// the box away with the browsing context, and twenty-seven reftests passed
// because a border nobody drew cannot be the wrong colour.
//
// That is the same mistake the form controls were moved out of droppedElements
// for, recorded below: not being able to do the dynamic half is not a reason to
// pretend the static half is not there.
//
// Its content is skipped rather than laid out, because that is what the content
// *is*: an iframe's children are what a browser without frame support would show
// instead, and a browser with them never renders it. Skipping it as raw text is
// also what the tokenizer must do regardless — the content of an iframe is not
// markup.
var contentSkippedElements = map[string]bool{
	"iframe": true,
}

// droppedElements are the ones refused for what they *do* rather than for being
// unknown, and each has its own reason.
//
// The first three are the entirety of the code-execution and remote-content
// surface, which this engine refuses outright. A renderer that ignored them
// silently would still be one that had read them, and an author who embedded a
// <script> expecting it to be inert deserves to be told it was thrown away
// rather than left to assume it ran.
//
// The form controls used to be here, under "form controls are not interactive
// here". That reason confused interactivity with layout: a control has a size
// and content on a printed page whether or not anything can be clicked, and
// dropping it lost the content. They are in knownElements now, and the
// interactivity boundary is stated there rather than deleted.
var droppedElements = map[string]string{
	"script": "scripts are never run, and never will be",
	"embed":  "an embedded plugin would need a plugin",
	"applet": "applets would need a virtual machine",
	// <audio> stays, and the reason above holds for it where it did not for
	// <video>: HTML renders an audio element with no "controls" attribute as
	// "display: none", so the page is the same either way, and one *with*
	// controls is a player whose size no specification states. There is no box
	// to lose by refusing it.
	"audio":    "a page laid out once cannot play anything",
	"details":  "a disclosure widget needs somewhere to click",
	"summary":  "a disclosure widget needs somewhere to click",
	"dialog":   "a dialog is opened by script, which is never run",
	"template": "a template's content is instantiated by script",
	"progress": "a progress bar reflects a state that does not change here",
	"meter":    "a meter reflects a state that does not change here",
}

// The optional end tags.
//
// This is HTML's *optional end tags* (§13.1.2.4), not error recovery. Leaving
// out "</li>" or "</p>" is correct HTML, and every template in the world does
// it, so refusing it would refuse the input this engine exists to read.
//
// They used to be a table keyed by the open element — "a <p> is closed by these
// start tags, an <li> by that one" — consulted against the top of the stack
// alone. That is a restatement of the rules which is right exactly when the
// element being ended is the innermost one, and the documents where it is not
// are ordinary ones: "<li><p>a<li>" leaves a paragraph open inside the first
// item, and so the second item went inside the paragraph, and "<td><p>a<td>"
// put the second cell inside the first cell's paragraph and the next row inside
// that — all of it silently, because the final end tag closed the whole
// mis-nest as optional end tags. A browser's tree has two items and two cells.
//
// So the rules are now written the way §13.2.6.4.7 writes them, over the whole
// stack of open elements: "has an element in scope", "generate implied end
// tags", "close a p element", "close the cell". Those four primitives are in
// parse.go, and the sets below are the ones they are stated in terms of, each
// copied from the standard's own list. A rule written in them reaches past
// whatever the author left open for the same reason the standard's does, and
// stops where the standard's stops.

// impliedEndTags are the elements "generate implied end tags" pops: the ones
// whose end tag is implied by whatever comes next, wherever they are.
var impliedEndTags = setOf(
	"dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc",
)

// closedAtEnd are the elements that may still be open when the document ends
// without that being a mistake: the implied ones, and the table's rows, cells
// and row groups, whose end tags are implied by the end of their parent. It is
// the list HTML's "in body" mode checks at the end of the file.
var closedAtEnd = setOf(
	"dd", "dt", "li", "optgroup", "option", "p", "rb", "rp", "rt", "rtc",
	"tbody", "td", "tfoot", "th", "thead", "tr",
)

// specialElements are HTML's "special" category: the elements an end tag for
// some other name does not reach past, and the ones that end the search for an
// open <li>, <dd> or <dt>. §13.2.4.2, without the MathML and SVG entries —
// foreign content is never on this parser's stack.
var specialElements = setOf(
	"address", "applet", "area", "article", "aside", "base", "basefont",
	"bgsound", "blockquote", "body", "br", "button", "caption", "center", "col",
	"colgroup", "dd", "details", "dir", "div", "dl", "dt", "embed", "fieldset",
	"figcaption", "figure", "footer", "form", "frame", "frameset", "h1", "h2",
	"h3", "h4", "h5", "h6", "head", "header", "hgroup", "hr", "html", "iframe",
	"img", "input", "keygen", "li", "link", "listing", "main", "marquee",
	"menu", "meta", "nav", "noembed", "noframes", "noscript", "object", "ol",
	"p", "param", "plaintext", "pre", "script", "search", "section", "select",
	"source", "style", "summary", "table", "tbody", "td", "template",
	"textarea", "tfoot", "th", "thead", "title", "tr", "track", "ul", "wbr",
	"xmp",
)

// The four scopes of §13.2.4.2. Each is the set of elements at which the search
// for an open element stops, looking outward from the innermost: an element
// outside one of these is not "in scope", and nothing inside may close it.
//
// That is what keeps a cell's content from ending things outside the cell: a
// "<p>" written in a table cell does not close a paragraph the table itself is
// inside, because td is a boundary of every scope that p is looked for in.
var (
	defaultScope = setOf(
		"applet", "caption", "html", "table", "td", "th", "marquee", "object",
		"template",
	)
	listItemScope = setOf(
		"applet", "caption", "html", "table", "td", "th", "marquee", "object",
		"template", "ol", "ul",
	)
	buttonScope = setOf(
		"applet", "caption", "html", "table", "td", "th", "marquee", "object",
		"template", "button",
	)
	tableScope = setOf("html", "table", "template")

	// formattingMarkers are where the standard puts a marker on its list of
	// active formatting elements, which is as far back as a nested <a> or
	// <nobr> looks for the one it would end. <html> is here because the stack
	// ends there.
	formattingMarkers = setOf(
		"applet", "object", "marquee", "template", "td", "th", "caption",
		"button", "html",
	)
)

// closesParagraph are the start tags that "close a p element" if one is in
// button scope: the list §13.2.6.4.7 gives the rule against, gathered from its
// several entries — the block containers, the headings, the list items, <pre>
// and <listing>, <form>, <plaintext>, <hr>, <table> and <xmp>.
//
// <table> is here as the standard has it for a document in no-quirks mode. In
// quirks mode a table does not end a paragraph, and this engine has no quirks
// mode at all: every document is read as though it declared <!DOCTYPE html>.
var closesParagraph = setOf(
	"address", "article", "aside", "blockquote", "center", "details", "dialog",
	"dir", "div", "dl", "fieldset", "figcaption", "figure", "footer", "header",
	"hgroup", "main", "menu", "nav", "ol", "p", "search", "section", "summary",
	"ul",
	"h1", "h2", "h3", "h4", "h5", "h6",
	"pre", "listing", "form", "plaintext", "hr", "table", "xmp",
	"li", "dd", "dt",
)

// headings are h1 to h6, which the standard treats as one name in two places:
// a heading's start tag ends a heading that is the current node, and a heading's
// end tag closes whichever heading is open.
var headings = setOf("h1", "h2", "h3", "h4", "h5", "h6")

// blockEndTags are the end tags that close their element if it is in scope,
// generating implied end tags on the way — the list §13.2.6.4.7 gives for
// "address, article, aside, …", with <form>, <applet>, <marquee>, <object>,
// <dd>, <dt> and <head> alongside, which are closed by the same steps.
//
// Everything outside this list and the table's own names is "any other end
// tag", which does not reach past a special element: see parser.endTag.
var blockEndTags = setOf(
	"address", "article", "aside", "blockquote", "button", "center",
	"details", "dialog", "dir", "div", "dl", "fieldset", "figcaption",
	"figure", "footer", "header", "hgroup", "listing", "main", "menu", "nav",
	"ol", "pre", "search", "section", "summary", "ul",
	"form", "applet", "marquee", "object", "dd", "dt", "head",
)

// tableStructure are the elements that decide which of HTML's table insertion
// modes applies — "in table", "in table body", "in row", "in cell", "in
// caption", "in column group" — and so what a table's own tags close. They are
// what an incoming table tag is resolved against, and what a table end tag
// closes without a word on its way to its element.
var tableStructure = setOf(
	"table", "caption", "colgroup", "tbody", "thead", "tfoot", "tr", "td", "th",
)

// tableParts are the start tags that the table modes resolve against the table
// structure open around them: see parser.closeForTablePart.
var tableParts = setOf(
	"caption", "col", "colgroup", "tbody", "thead", "tfoot", "tr", "td", "th",
)

// tablePartEnds reports whether an incoming table tag ends an open table
// structure element — the rule of the insertion mode that element puts the
// parser in, which acts as though that element's end tag had been seen and
// reprocesses the tag.
//
//   - "in cell" and "in caption" end on any table part: a cell written
//     straight after a cell is the next cell, and one written after a caption
//     is a cell of the table and not something inside the caption.
//   - "in row" ends on any table part but a cell.
//   - "in table body" ends on a caption, a column or a row group.
//   - "in column group" ends on anything but a <col>. A <td> straight after a
//     <colgroup> is therefore a cell of the table, and the suite's
//     border-conflict-style-107 writes exactly that and loses the whole table
//     when the cell goes inside the column group instead.
//
// A <table> is resolved here too, because in every table mode but a cell's and
// a caption's it ends the table it was written in rather than nesting: HTML has
// no table directly inside a table.
func tablePartEnds(incoming, open string) bool {
	switch open {
	case "td", "th", "caption":
		return tableParts[incoming]
	case "tr":
		return tableParts[incoming] && incoming != "td" && incoming != "th" ||
			incoming == "table"
	case "tbody", "thead", "tfoot":
		return incoming == "caption" || incoming == "col" || incoming == "colgroup" ||
			incoming == "tbody" || incoming == "thead" || incoming == "tfoot" ||
			incoming == "table"
	case "colgroup":
		return incoming != "col"
	case "table":
		return incoming == "table"
	}
	return false
}

// tableContexts are the elements whose children HTML restricts to table
// content, and out of which anything else is moved.
var tableContexts = setOf("table", "thead", "tbody", "tfoot", "tr")

// tableContent is what may be written inside one of those.
//
// It is the table roles and the elements that produce nothing visible. It is
// deliberately more permissive than any one insertion mode: a <td> written
// straight inside a <table> is not what HTML says either, and the row group it
// implies is generated by the anonymous-box rules of CSS 2.1 §17.2.1 rather
// than here — so moving the cell out of the table would lose it, and leaving it
// in is what the layout already repairs.
//
// What is *not* here is ordinary flow content, which is the case foster
// parenting exists for and which no anonymous box can put right: a <div>
// between two rows belongs before the table, and reading it as a table child
// gives it a row of its own.
var tableContent = setOf(
	"caption", "colgroup", "col", "thead", "tbody", "tfoot", "tr", "td", "th",
	"style", "link", "script", "template", "meta", "base", "title",
)

// headElements belong in <head> when no <body> has begun.
//
// <style> and <link> are deliberately absent: both are legal in the body, and
// moving one there into the head would change the order stylesheets apply in,
// which changes which declaration wins.
var headElements = setOf("title", "meta", "base")

// metadataElements produce nothing visible. They may appear in the body without
// starting one, so that "<meta charset=utf-8><p>x</p>" does not put the <meta>
// in the body and the <p> after it.
var metadataElements = setOf("title", "meta", "base", "link", "style")

func setOf(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// foreignElements are the roots of subtrees that are not HTML.
//
// An unknown HTML element keeps its place in the tree and its content is parsed
// on (see insertUnknown), which is right: the content *is* HTML, a browser
// shows it, and a <fancy-callout> this engine has no style for has not lost its
// words. A foreign element is the opposite case. Its children are SVG or
// MathML, they mean nothing to an HTML layout, and their text is not text of
// the document — so parsing on splices it into the flow, which is what
// "<svg><text>x</text></svg>" did: an x in the surrounding paragraph's font, on
// the paragraph's baseline, nowhere near the picture.
//
// That is worse than the missing picture. A hole is visibly a hole; a stray
// letter reads as the document's own and is what a reader would have to know the
// source to catch.
//
// The subtree is skipped by name-matched depth, which is what makes a nested
// <svg> inside an <svg> end the right one.
var foreignElements = map[string]bool{
	"svg":  true,
	"math": true,
}
