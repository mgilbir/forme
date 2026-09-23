package layout

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"strings"

	// The decoders. Registering them is what makes image.DecodeConfig able to
	// read a header without the caller knowing the format, and the three here
	// are the ones the web is made of. Nothing else is registered on purpose:
	// every additional decoder is another parser reading untrusted bytes, and
	// the formats that would need one — TIFF, WebP, BMP — are not what a
	// document embeds.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/style"
)

// Loading the images an <img> names.
//
// # Why the header is read before the pixels
//
// A PNG is a header saying how large the image is and a compressed stream of
// pixels. The two are not related by anything the format enforces, so ten
// kilobytes of markup can declare sixty thousand by sixty thousand — and an
// engine that decodes first and checks afterwards has already allocated
// fourteen gigabytes by the time it can object. image.DecodeConfig reads the
// header alone, which is what makes the pixel cap below a cap rather than a
// post-mortem.
//
// The same reasoning gives the three caps their shapes. One bounds the encoded
// bytes, because those are allocated before anything can look at them. One
// bounds a single image's pixels, because that is what a decode allocates. One
// bounds the whole document's pixels, because a page of ten thousand small
// images costs the same as one large one and no per-image cap sees it.
//
// # What a failure is
//
// Not an error. CSS says an element whose image cannot be shown is simply not a
// replaced element, so it becomes an ordinary inline box and its alt text is
// what the reader gets. That is what happens here, and the finding is what
// stops it being silent — a page with a hole in it and no other symptom is
// exactly the failure §6 exists to name.

// maxImagePixels bounds one image.
//
// Sixteen megapixels is a 4000 × 4000 photograph, which is past anything a
// document embeds and a long way short of what a decoder can be talked into
// allocating. The check is against the *declared* size, before any decode.
//
// A variable so that a test can lower it; the cap is also exercised at its real
// value by a PNG header declaring more pixels than this, which is what a
// decompression bomb is.
var maxImagePixels int64 = 1 << 24

// maxDocumentPixels bounds every image in one document together.
//
// A per-image cap does not bound a document: ten thousand images of a megapixel
// each are ten gigapixels, and each one passes the per-image check. Sixty-seven
// megapixels is four of the largest single image this will take.
//
// It is a budget rather than a count, so a document of many small images is not
// penalised for having many.
var maxDocumentPixels int64 = 1 << 26

// maxDataURIBytes bounds the encoded length of a "data:" reference.
//
// The bytes are in the document, so they are already bounded by the html
// package's input cap — but that cap is sixty-four megabytes and applies to the
// whole document, so it is not a bound on one image. This is.
var maxDataURIBytes = 16 << 20

// ReplacedContent is what makes an element replaced: content with dimensions of
// its own that layout sizes rather than lays out.
//
// It is a value on the box rather than a subclass of it because "replaced" is a
// property of the element's content and not of its box type — a replaced
// element is still inline or block, still floats, still positions, and every
// rule about those applies unchanged. The one thing that differs is where its
// width and height come from, which is what this carries.
type ReplacedContent struct {
	// Image is the decoded picture.
	Image image.Image

	// Width and Height are the intrinsic dimensions: the image's own pixel
	// size, one image pixel to one CSS pixel.
	Width, Height style.Unit

	// Stated says the two above are the content's own even where they are
	// nought.
	//
	// Zero is how this spells "no intrinsic dimension", which is what a decoded
	// picture never has and what an iframe always has, so for nearly every kind
	// of content the two readings are the same. They part company for content
	// that *states* a size and states it as nothing: "<canvas width=0>" is a
	// valid non-negative integer and HTML keeps it, and an <img> naming no file
	// is an element HTML says represents nothing at all. Read as "no intrinsic
	// dimension" those become §10.3.2's 300 by 150, which is a box a document
	// asked for the absence of.
	//
	// A flag rather than a pair of them, because the two kinds of content that
	// set it state both dimensions or neither; and set by those two alone, so
	// that everything already here keeps the reading it had.
	Stated bool

	// WidthPercent and HeightPercent are the dimensions an SVG states as a
	// percentage, as a fraction, and are zero when it states none.
	//
	// They are not intrinsic dimensions and are kept apart from Width and
	// Height for that reason: CSS Images §5.4 makes a percentage "no intrinsic
	// dimension" for everything that asks whether the image has one, and then
	// resolves it against the *default object size* when there is a concrete
	// one to resolve against. For a background layer that is the positioning
	// area, which is where they are read — see tileSize. Nothing else reads
	// them, so an <img> holding such a file is sized as it always was.
	//
	// background-intrinsic-006 is what needs them: an SVG of "width: 40%;
	// height: 60%" in an eighty-by-a-hundred positioning area is thirty-two by
	// sixty, and the test covers exactly that rectangle with a green box and
	// asks for no red anywhere.
	WidthPercent, HeightPercent float64

	// Ratio is the intrinsic ratio, width divided by height, and is zero when
	// there is none. It is kept as a number rather than recomputed from the two
	// dimensions because CSS 2.1 §10.3.2 distinguishes an element that has a
	// ratio from one that has dimensions, and a format can supply either
	// without the other.
	Ratio float64

	// Key identifies the source bytes, so that a document naming one file
	// twenty times embeds one image. It is a hash of the bytes rather than the
	// reference, which also collapses the same picture reached by two names.
	Key string

	// Pixels is the image's pixel count, which is what the document budget was
	// charged.
	Pixels int64

	// SVG is the picture an SVG carries: its rectangles, and the coordinate
	// system they are stated in. It is nil for everything else.
	//
	// It is kept beside Solid rather than instead of it because the two answer
	// different callers. A replaced element is drawn once, at a known place, so
	// it can place each rectangle; a background layer is tiled and positioned,
	// and has nowhere to put geometry — for that, only a picture that is one
	// colour all over can be drawn at all.
	SVG *svgPicture

	// Solid is set when the content is exactly one colour, and Image is then
	// nil: there are no pixels because none are needed.
	//
	// Two kinds of content reach this. A gradient whose stops are all one
	// colour is that colour everywhere, and an SVG whose only drawable content
	// is a rectangle covering its viewport is that rectangle's fill. Neither is
	// an approximation — see gradient.go and svg.go for why each is exact and
	// where the line is drawn against the general case.
	//
	// It is a colour rather than a one-pixel picture so that the display list
	// says what the page says. A page written with background-color and a page
	// written with linear-gradient(green, green) paint the same thing, and a
	// stretched picture would make the two compare unequal while looking
	// identical.
	Solid *style.RGBA

	// Bands is set when the content is a linear gradient whose colour never
	// interpolates, and is then the stripes it paints. Like Solid it carries no
	// pixels, and unlike Solid it needs a size before it is a picture: where a
	// band's edges fall depends on how long the gradient line is. See
	// gradient.go.
	Bands *bandedGradient
}

// Paints reports whether this content puts anything on the page.
func (r *ReplacedContent) Paints() bool {
	return r != nil && (r.Image != nil || r.Solid != nil || r.SVG != nil || r.Bands != nil)
}

// replacedLoader turns the references in a box tree into loaded content.
type replacedLoader struct {
	// languageMemo is the writing system an alt text is collapsed in. See
	// languageMemo.
	languageMemo

	res ResourceResolver
	rec *Recorder

	// loaded memoizes by reference, so a document that repeats one src reads,
	// decodes and charges the budget once.
	//
	// By reference *and* by how it is read, which is what the key is. The
	// same SVG is a picture in an <img> and a document in an <object>, and
	// the two readings differ in what its own percentages are of — see
	// svgAs. Keyed by the reference alone, whichever element came first
	// decided the other's size: an SVG stating no size was 300 by 150 in an
	// <img> and as wide as its containing block in an <object>, until a
	// document had both, when the second took the first's (audit C83).
	loaded map[refKey]*ReplacedContent
	// failed records the references already reported, so a page of a hundred
	// broken images is one finding rather than a hundred.
	failed map[refKey]bool

	// byContent memoizes by what a reference *read*, which is the memo that
	// decides what decoding costs. The one above is by the reference's
	// spelling, and a spelling is the document's to vary: "b.png?1", "b.png?2"
	// and so on name one file to a resolver that ignores the query, and were a
	// fresh read and a fresh decode each — a hundred of them over a picture
	// whose body fails took eight seconds from 1790 bytes of markup (audit
	// C19). The same bytes decode the same way, so they are decoded once.
	byContent map[contentKey]decoded

	// budget is how many pixels the document may still decode.
	budget int64
	// exhausted records that the budget ran out, reported once.
	exhausted bool
	// cut records that the document's work budget refused a read or a decode.
	// Nothing more is read after that: every read costs the resolver's work
	// before anything can be charged for it, and a document whose next read
	// would also be refused would otherwise be read in full to be told so.
	cut bool
}

// refKey is a reference and how it is read: loaded's key. See there.
type refKey struct {
	ref string
	as  svgAs
}

// contentKey is what a reference read, and everything the reading depends on:
// an SVG is a different thing as a picture and as a document, and must not
// come back from the memo as the other; and the same bytes are an SVG under
// one declared type and not under another. See decode.
type contentKey struct {
	sum  [sha256.Size]byte
	as   svgAs
	mime string
}

// decoded is one memoized decode: the content, or the first reference whose
// bytes did not decode and what was said about it.
type decoded struct {
	content *ReplacedContent
	failed  *loadFailure
	src     string
	what    string
}

// resolveReplaced loads the content of every replaced element in a box tree.
//
// It runs after the tree is built rather than during, because whether an
// element is replaced changes nothing about the shape of the tree: an <img> is
// a box either way, and only its sizing differs. Doing it as a pass keeps the
// box builder from having to carry a resolver through every recursion.
func resolveReplaced(root *Box, res ResourceResolver, rec *Recorder) {
	if root == nil {
		return
	}
	l := &replacedLoader{
		res: res, rec: rec,
		loaded:    map[refKey]*ReplacedContent{},
		failed:    map[refKey]bool{},
		byContent: map[contentKey]decoded{},
		budget:    maxDocumentPixels,
	}
	l.walk(root)
}

func (l *replacedLoader) walk(b *Box) {
	if b.Element != nil && strings.EqualFold(b.Element.Name, "img") {
		l.image(b)
	}
	if b.Element != nil && strings.EqualFold(b.Element.Name, "object") {
		l.object(b)
	}
	if b.Element != nil && strings.EqualFold(b.Element.Name, "iframe") {
		l.iframe(b)
	}
	if b.Element != nil && strings.EqualFold(b.Element.Name, "canvas") {
		l.canvas(b)
	}
	if b.Element != nil && strings.EqualFold(b.Element.Name, "video") {
		l.video(b)
	}
	if b.Element != nil && b.Element.Foreign != "" {
		l.foreign(b)
	}
	l.markerImage(b)
	l.contentImage(b)
	l.backgrounds(b)
	for _, c := range b.Children {
		l.walk(c)
	}
}

// backgrounds loads the pictures a box's background-image names.
//
// It goes through the same fetch, the same caps and the same document-wide
// decode budget as an <img>, which is the whole reason it is here rather than in
// background.go: a document with a hundred boxes naming one texture must read
// and decode it once, and a document whose backgrounds together ask for ten
// gigapixels must be refused by the same counter that refuses ten gigapixels of
// <img>. A second loading path would be a second policy, and the second one is
// always the one that is missing a check.
func (l *replacedLoader) backgrounds(b *Box) {
	refs := backgroundImageRefs(b.Style.Get("background-image"))
	if len(refs) == 0 {
		return
	}
	for _, ref := range refs {
		content, why := l.memoized(ref, "background image", svgAsImage)
		if content == nil {
			if why != nil {
				l.rec.ReportDetail(Finding{
					Rule:     why.rule,
					Source:   AtHTML(offsetOf(b)),
					Message:  why.message,
					Path:     PathOf(b.Element),
					Property: "background-image",
				})
			}
			continue
		}
		l.attachBackground(b, ref, content)
	}
}

func (l *replacedLoader) attachBackground(b *Box, ref string, content *ReplacedContent) {
	if b.BackgroundImages == nil {
		b.BackgroundImages = map[string]*ReplacedContent{}
	}
	b.BackgroundImages[ref] = content
}

// image loads one <img>, or explains why it did not.
func (l *replacedLoader) image(b *Box) {
	el := b.Element
	if v, ok := el.Attr("srcset"); ok && strings.TrimSpace(v) != "" {
		// A srcset offers several files and rules for choosing between them —
		// by pixel density, by rendered width, by what a <picture> above it
		// says. This engine takes "src" and says so, because the failure
		// otherwise is the quietest kind there is: the page carries a picture,
		// it is simply not the one the author's rules would have chosen, and
		// nothing about the document says which was used.
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(el.Offset),
			Message: "this image offers several files in its \"srcset\" and this engine " +
				"chooses between none of them; the \"src\" was used",
			Path:     PathOf(el),
			Property: "srcset",
		})
	}
	src, _ := el.Attr("src")
	src = strings.TrimSpace(src)
	if src == "" {
		// An <img> with no src is not a broken image, it is an element that
		// names nothing. HTML says it represents nothing at all, and there is
		// no reference for a resolver to have refused.
		//
		// Nothing is still *replaced* nothing. The element has no content to
		// take a size from, so its intrinsic dimensions are nought and stated —
		// and that is not the same as having none, which would make it
		// §10.3.2's 300 by 150. What it buys is the two things a replaced
		// element is: CSS may size it, since width and height do not apply to a
		// non-replaced inline, and it is content, so the white space either side
		// of it does not collapse together across it. The suite writes the
		// second as text-wrap-balance-word-spacing-001, whose reference keeps
		// both spaces around an <img> that names no file.
		//
		// Unless the element carries alt text, which is a different case with a
		// different answer: HTML says what an image that cannot be shown
		// contains is that text, and CSS says an element whose replaced content
		// is unavailable is not a replaced element at all. altOnly is where that
		// is decided, and this asks it first.
		if l.altOnly(b); len(b.Children) == 0 {
			b.Replaced = &ReplacedContent{Stated: true}
		}
		return
	}

	// A reference that failed before comes back with no finding, and the
	// element still gets its alt text.
	content, why := l.memoized(src, "image", svgAsImage)
	if content == nil {
		l.notReplaced(b, why)
		return
	}
	b.Replaced = content
}

// object gives an <object> the picture its data names, or reports that it could
// not and lays out the fallback content instead.
//
// HTML §4.8.7 hands the resource to a plugin, a nested browsing context or an
// image decoder, and which of the three depends on what arrived. The first two
// are what this engine refuses outright — a plugin is arbitrary code
// and a browsing context is a document of its own — but the third is the same
// decoder <img> already uses, so an <object> naming a picture is a picture. The
// suite's replaced-intrinsic-001 to -005 are five of them, and every one is an
// SVG or a PNG.
//
// Where the data cannot be decoded, HTML says the element is represented by its
// *fallback content*, which is its children and is ordinary markup — so the box
// is laid out like any other and the children are on the page. The finding is
// the half that matters there: the children of an <object> are what an author
// wrote for the case where the object could not be shown, so a page that draws
// them is deliberately showing its second choice, and a caller has to be able to
// know that rather than infer it from a paragraph reading "your browser cannot
// show this".
//
// An <object> with no data names nothing, so there is nothing to have failed and
// nothing to report: it is a box holding its children, and that is all it ever
// was.
func (l *replacedLoader) object(b *Box) {
	data, ok := b.Element.Attr("data")
	data = strings.TrimSpace(data)
	if !ok || data == "" {
		return
	}
	content, why := l.memoized(data, "object", svgAsDocument)
	if content == nil {
		l.fallbackTo(b, why, data)
		return
	}
	l.embed(b, content)
}

// embed replaces an object with the data it named, and takes the fallback
// content off it.
//
// HTML: an object that could be shown is represented by the data and *not* by
// its children — they are what an author wrote for the case where it could not
// be, which is the case fallbackTo is about. Leaving them laid out the box out
// of the object's own size and drew a paragraph reading "FAIL (SVG not
// supported)" over a picture that was there, which is the suite's
// replaced-intrinsic-003 exactly.
//
// The children are dropped rather than hidden. A hidden box is still a box —
// it takes part in the sizing and keeps its own out-of-flow descendants — and
// what HTML says is that the fallback content is not rendered at all.
func (l *replacedLoader) embed(b *Box, content *ReplacedContent) {
	b.Replaced = content
	b.Children = nil
}

// fallbackTo says an object's data could not be used, so what is on the page is
// the markup the author wrote for that case.
func (l *replacedLoader) fallbackTo(b *Box, fail *loadFailure, data string) {
	msg := "the object at " + quoteValue(data) + " was not embedded, so the " +
		"element's fallback content was laid out in its place"
	rule := RuleResourceBlocked
	if fail != nil {
		msg = fail.message + "; the element's fallback content was laid out in its place"
		rule = fail.rule
	}
	l.rec.ReportDetail(Finding{
		Rule:    rule,
		Source:  AtHTML(b.Element.Offset),
		Message: msg,
		Path:    PathOf(b.Element),
	})
}

// canvas makes a <canvas> the replaced element it is.
//
// A canvas is a bitmap, and its intrinsic dimensions are that bitmap's: HTML
// §4.12.5 puts them on the element's own width and height attributes and gives
// them a default of 300 by 150 — the same two numbers CSS 2.1 §10.3.2 uses, and
// not a coincidence. Those are *intrinsic* dimensions and not HTML's dimension
// attributes: they are not in style/hints.go's table and must not be, because
// mapping them to the width and height properties would make "<canvas width=10
// height=10 style='height: 100%'>" ten pixels wide where its ratio makes it as
// wide as it is tall. The suite writes that as
// normal-flow/intrinsic-size-with-anonymous-block.
//
// Nothing is drawn and nothing is reported. The bitmap of a canvas nobody
// scripted is transparent black — a browser with scripting turned off lays out
// a blank canvas of exactly this size rather than omitting it — so the page has
// what it should have and there is nothing missing to name. A canvas a script
// would have painted is a page whose <script> was thrown away, and that is
// already reported where it happened.
//
// The fallback children never arrive. A canvas's children are what a user agent
// that cannot do canvas would show instead, and one that can never renders them
// — so the box builder does not build them at all. See layout.replacedFallback,
// and note that it is the *builder* that has to do it: this pass runs after the
// tree is built, and a block among the fallback has split the inline box around
// it by then, taking the canvas with it. Clearing the children here as well was
// written first, and a planted defect removing it now changes nothing in the
// unit tests or in the suite, so it is gone rather than kept as a second answer
// to a question with one.
func (l *replacedLoader) canvas(b *Box) {
	w, wideOver := canvasDimensionRead(b.Element, "width", 300)
	h, tallOver := canvasDimensionRead(b.Element, "height", 150)
	if wideOver || tallOver {
		// A bitmap larger than any length this engine can lay out. The value is
		// the document's and it is a real one, so leaving it at the default
		// has to be said: it used to be, silently, for any number over ten
		// digits long.
		l.rec.ReportDetail(Finding{
			Rule:   RuleLimit,
			Source: AtHTML(b.Element.Offset),
			Message: "this canvas states a size larger than any this engine can lay out; " +
				"it was laid out at its default size",
			Path: PathOf(b.Element),
		})
	}
	content := &ReplacedContent{Width: w, Height: h, Stated: true}
	if w > 0 && h > 0 {
		// Only a bitmap with area has a ratio. A canvas may state a zero
		// dimension — "width=0" is a valid non-negative integer and HTML keeps
		// it — and a ratio computed from one would be zero or infinite, which
		// §10.3.2 would then solve the other dimension from.
		content.Ratio = w.Px() / h.Px()
	}
	// Stated, so that a canvas of no area lays out as one rather than falling
	// through to §10.3.2's default size: "width=0" is a valid non-negative
	// integer and HTML keeps it. See ReplacedContent.Stated.
	b.Replaced = content
}

// video makes a <video> the replaced element it is, and reports what a reader
// would have seen and does not.
//
// The box first, because that is the half that was missing. HTML §4.8.9: a video
// element's intrinsic dimensions are the video's, or the poster image's while
// there is no video, and where there is neither it takes the default object size
// — CSS 2.1 §10.3.2's 300 by 150, the same two numbers an <iframe> with nothing
// in it takes and for the same reason.
//
// The poster is a picture this engine can draw and is exactly what a browser
// shows before anything plays, so it is loaded and it is the content. A poster
// that cannot be read is a blocked resource like any other.
//
// Then the two things that are refused, each reported only when the document
// asked for it:
//
//   - The film. A "src", or a <source> child, names media that is not on the
//     page. That is a blocked resource in the sense an <object>'s data is, and a
//     page laid out once genuinely cannot show it.
//   - The controls. "controls" asks for a player a reader operates, and this
//     page is not operated; the box is drawn and the bar in it is not, which is
//     what RuleControlApproximated is for.
//
// A <video> that names no media, has no poster and asks for no controls has
// nothing missing from it, and nothing is reported. That is the iframe's rule
// again — "an iframe naming nothing has nothing missing" — and it is the half
// that decides whether a reftest about a video's box is evidence of anything:
// video-paint-order draws a green block over an empty video and asks that the
// video not show through, and a finding about a film nobody named would have
// held the answer out of the count.
//
// The fallback children go, for the reason canvas drops its own: they are what a
// user agent that cannot play video would show instead, and this one draws the
// element rather than replacing it.
func (l *replacedLoader) video(b *Box) {
	// No intrinsic width, height or ratio: replacedSize then falls through to
	// §10.3.2's default dimensions rather than to a box of no size.
	b.Replaced = &ReplacedContent{}
	named := false
	if src, ok := b.Element.Attr("src"); ok && strings.TrimSpace(src) != "" {
		named = true
	}
	// The <source> children are read from the *element* and not from the box,
	// because the box has none: a replaced element's fallback is not laid out,
	// and the box builder leaves it out rather than this pass throwing it away.
	// See layout.replacedFallback.
	for _, c := range b.Element.Children {
		if c.Type == html.ElementNode && strings.EqualFold(c.Name, "source") {
			if src, ok := c.Attr("src"); ok && strings.TrimSpace(src) != "" {
				named = true
			}
		}
	}

	// Through the memos every other reference goes through. It was loaded
	// afresh for every <video>, so a document repeating one poster read and
	// decoded it once per element — the one path where naming a file again
	// needed no trick to cost a decode again (audit C19). A poster that failed
	// is reported for the first element that names it, as a background is.
	if poster, ok := b.Element.Attr("poster"); ok && strings.TrimSpace(poster) != "" {
		content, why := l.memoized(strings.TrimSpace(poster), "video poster", svgAsImage)
		switch {
		case content != nil:
			b.Replaced = content
		case why != nil:
			l.rec.ReportDetail(Finding{
				Rule:     why.rule,
				Source:   AtHTML(offsetOf(b)),
				Message:  why.message,
				Path:     PathOf(b.Element),
				Property: "poster",
			})
		}
	}

	if named {
		l.rec.ReportDetail(Finding{
			Rule:   RuleResourceBlocked,
			Source: AtHTML(offsetOf(b)),
			Message: "the video this <video> names is not played, because a page " +
				"laid out once has no time in it; the element's box is on the page " +
				"and the frames are not",
			Path:     PathOf(b.Element),
			Property: "video",
		})
	}
	if _, ok := b.Element.Attr("controls"); ok {
		l.rec.ReportDetail(Finding{
			Rule:   RuleControlApproximated,
			Source: AtHTML(offsetOf(b)),
			Message: "the controls this <video> asks for are not drawn: a player is " +
				"operated and this page is not, so the box is here and the bar in " +
				"it is not",
			Path:     PathOf(b.Element),
			Property: "controls",
		})
	}
}

// canvasDimension reads one of a canvas's two bitmap dimensions.
//
// HTML's *rules for parsing non-negative integers*, which are not Atoi: leading
// white space is skipped, a leading plus is allowed, digits are collected, and
// anything after them is ignored — so "10px" is ten. Anything that yields no
// digits at all, or a negative, is not an error to report but a value the
// attribute does not have, and the element takes its default. The rules are
// html.ParseNonNegativeInteger, which every integer attribute is read by.
func canvasDimension(n *html.Node, name string, fallback int) style.Unit {
	u, _ := canvasDimensionRead(n, name, fallback)
	return u
}

// canvasDimensionRead is canvasDimension, and whether the attribute stated a
// size too large to lay out — which is a value, and one this engine cannot
// hold, so the default stands and the caller says so.
func canvasDimensionRead(n *html.Node, name string, fallback int) (style.Unit, bool) {
	def := mustPx(float64(fallback))
	if n == nil {
		return def, false
	}
	raw, ok := n.Attr(name)
	if !ok {
		return def, false
	}
	v, ok := html.ParseNonNegativeInteger(raw)
	if !ok {
		return def, false
	}
	u, fits := style.FromPx(float64(v))
	if !fits || v >= html.MaxInteger {
		return def, true
	}
	return u, false
}

// markerImage loads the picture list-style-image names, for a box that draws a
// marker.
//
// §12.6.2 makes the property conditional on the image being *available*: a url
// that does not load is not an error to report and stop at, it is a marker that
// falls back to list-style-type. So a failure here is silent by design, which is
// the one place in this file that is true — everywhere else a resource that did
// not arrive is something the page is missing, and here the page has exactly
// what the specification says it should.
//
// It goes through the same fetch, the same caps and the same document-wide
// decode budget as an <img>, for the reason backgrounds do: one policy, and a
// second one would be the one missing a check.
func (l *replacedLoader) markerImage(b *Box) {
	if !b.ListItem {
		return
	}
	ref, ok := urlValue(b.Style.Get("list-style-image"))
	if !ok || strings.TrimSpace(ref) == "" {
		return
	}
	ref = strings.TrimSpace(ref)
	if content, _ := l.memoized(ref, "list marker image", svgAsImage); content != nil {
		b.MarkerImage = content
	}
}

// contentImage loads the picture a "content: url(...)" names.
//
// It is markerImage's shape and for the same reason: a box whose picture comes
// from a stylesheet rather than from an attribute still has to go through this
// loader, so that a document naming one file in a marker, a background and a
// pseudo-element reads and decodes it once and is charged for it once.
//
// The failure is reported, unlike a marker's. A marker that loses its picture
// falls back to the list's own bullet and the page still says "this is a list";
// generated content that loses its picture leaves a gap in a line with nothing
// to say a picture was meant to be there, which is the quiet kind of wrong the
// findings exist for.
func (l *replacedLoader) contentImage(b *Box) {
	ref := b.ContentImage
	if ref == "" {
		return
	}
	content, why := l.memoized(ref, "generated content image", svgAsImage)
	if content == nil {
		if why != nil {
			l.rec.ReportDetail(Finding{
				Rule:     why.rule,
				Source:   AtHTML(offsetOf(b)),
				Message:  why.message,
				Path:     PathOf(b.Element),
				Property: "content",
			})
		}
		return
	}
	b.Replaced = content
}

// iframe makes an iframe the replaced box it is, and reports the document that
// was not rendered inside it.
//
// The two halves are separate and only one of them is a limitation. An iframe is
// a replaced element, so it has a box on the page whether or not a browsing
// context was ever created for it — and with no intrinsic dimensions that box is
// CSS 2.1 §10.3.2's 300 by 150, which is the number the specification took from
// this element. Nothing about drawing that box requires a network or a renderer
// for a nested document, and the element was dropped for years on the grounds
// that it does, which cost the box as well.
//
// The nested document is the half that is refused, permanently, by §4.1. An
// iframe naming one is reporting that something a reader would have seen is not
// on the page, and that is a blocked resource in exactly the sense an <object>'s
// data is.
//
// An iframe naming nothing has nothing missing. A browser handed "<iframe>" with
// no src shows an empty frame of the default size, and so does this: the box is
// right, the content is right because there is none, and there is nothing
// truthful to report. Reporting it anyway is what made twenty-seven reftests
// count as tainted while drawing exactly the right picture.
func (l *replacedLoader) iframe(b *Box) {
	// No intrinsic width, height or ratio: replacedSize then falls through to
	// the default dimensions rather than to a box of no size.
	b.Replaced = &ReplacedContent{}

	src, hasSrc := b.Element.Attr("src")
	if _, hasDoc := b.Element.Attr("srcdoc"); hasDoc {
		l.rec.ReportDetail(Finding{
			Rule:    RuleResourceBlocked,
			Source:  AtHTML(b.Element.Offset),
			Message: "this iframe carries a document in its \"srcdoc\"; this engine creates no nested browsing context, so the frame was laid out empty",
			Path:    PathOf(b.Element),
		})
		return
	}
	if !hasSrc || strings.TrimSpace(src) == "" {
		return
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleResourceBlocked,
		Source: AtHTML(b.Element.Offset),
		Message: "the document at " + quoteValue(strings.TrimSpace(src)) +
			" was not loaded into this iframe: this engine creates no nested " +
			"browsing context, so the frame was laid out at its own size and left empty",
		Path: PathOf(b.Element),
	})
}

// load fetches and decodes one reference, returning nil and a finding to raise
// when it could not.
type loadFailure struct {
	rule    Rule
	message string
}

// what names the thing being loaded — "image" for an <img>, "background image"
// for a background — because every message below says what did not arrive, and
// an author told "the image at paper.png was not loaded" while every <img> on
// the page is fine looks for the wrong element.
//
// Everything read is charged to the document's work budget, by its length and
// whether or not it turns out to be a picture: the reading is done either way.
// Then the bytes are looked up by what they are, so that a second name for
// the same file — or the same data: URL written twice — is not decoded twice,
// and a file that did not decode is not decoded again to fail again.
func (l *replacedLoader) load(src, what string, as svgAs) (*ReplacedContent, *loadFailure) {
	if l.cut {
		return nil, l.cutShort(src, what)
	}
	data, mime, fail := l.fetch(src, what)
	if fail != nil {
		return nil, fail
	}
	if !l.rec.charge(int64(len(data))*costFetchedByte, "the pictures past that point") {
		l.cut = true
		return nil, l.cutShort(src, what)
	}
	key := contentKey{sum: sha256.Sum256(data), as: as, mime: mime}
	if got, ok := l.byContent[key]; ok {
		if got.content != nil {
			return got.content, nil
		}
		return nil, &loadFailure{
			rule: got.failed.rule,
			message: "the " + what + " at " + quoteValue(src) + " is the same file as the " +
				got.what + " at " + quoteValue(got.src) + ", which was not drawn",
		}
	}
	// A refusal by a budget is memoized with the rest. Neither budget grows,
	// so the same bytes asked for again would be refused again.
	content, why := l.decode(src, what, data, key.sum, as, mime)
	l.byContent[key] = decoded{content: content, failed: why, src: src, what: what}
	return content, why
}

// memoized is load behind the reference memos: the content a reference already
// loaded, nothing for one that already failed — it was reported the first time
// — and otherwise a load, remembered either way.
//
// Every reference in the document is loaded through it, so that the memo is
// one memo: there were six copies of these ten lines, one per kind of element,
// and each had to be told separately what the key was.
func (l *replacedLoader) memoized(ref, what string, as svgAs) (*ReplacedContent, *loadFailure) {
	key := refKey{ref: ref, as: as}
	if got, ok := l.loaded[key]; ok {
		return got, nil
	}
	if l.failed[key] {
		return nil, nil
	}
	content, why := l.load(ref, what, as)
	if content == nil {
		l.failed[key] = true
		return nil, why
	}
	l.loaded[key] = content
	return content, nil
}

// cutShort is the finding for a picture the work budget refused. The budget has
// reported itself; this is the per-reference half, so that the element is
// still told why it has no picture.
func (l *replacedLoader) cutShort(src, what string) *loadFailure {
	return &loadFailure{
		rule: RuleImageUndecodable,
		message: "the " + what + " at " + quoteValue(src) +
			" was not drawn: the document had used up the work this engine does for one document",
	}
}

// fetch obtains the bytes a reference names, applying the policy of
// resource.go, and the type a data: URL declares for them.
func (l *replacedLoader) fetch(src, what string) ([]byte, string, *loadFailure) {
	data, mime, fail := fetchReference(l.res, src, what, "so it was not drawn", RuleImageUndecodable)
	if fail != nil {
		return nil, "", fail
	}
	if len(data) == 0 {
		return nil, "", &loadFailure{
			rule:    RuleImageUndecodable,
			message: "the " + what + " at " + quoteValue(src) + " is empty",
		}
	}
	return data, mime, nil
}

// decode reads a header, checks it against the caps, charges what it declares,
// and only then decodes. sum is the bytes' digest, which load has already
// taken, and mime is the type a data: URL declared for them, empty for bytes a
// resolver returned, which declare none.
func (l *replacedLoader) decode(src, what string, data []byte, sum [sha256.Size]byte,
	as svgAs, mime string) (*ReplacedContent, *loadFailure) {
	// An SVG is not a picture and never becomes one. It is read for its
	// intrinsic size and, when its content reduces to one, its colour — see
	// svg.go, which is explicit about how narrow that is and why the rest keeps
	// its finding. It has to be tried before image.DecodeConfig because no
	// decoder here reads XML, so an SVG would otherwise be an unknown format.
	//
	// Which of the two the bytes are is the MIME Sniffing standard's question
	// for an image context (§8.2), and its answer starts from the type the
	// bytes arrived with. A type that is XML is the type: the bytes are read as
	// an SVG whatever they look like. Any other type sends the bytes to the
	// image signatures — which is what image.DecodeConfig matches — and never
	// to the SVG reader, because no signature is an SVG's. Only bytes that came
	// with no type at all — a resolver's, which hands back bytes and nothing
	// else — are looked at for an SVG root. See looksLikeSVG.
	isSVG := looksLikeSVG(data)
	if mime != "" {
		isSVG = isXMLMIMEType(mime)
	}
	if isSVG {
		if c := svgContent(data, as); c != nil {
			return c, nil
		}
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: "the " + what + " at " + quoteValue(src) +
				" is an SVG this engine cannot reduce to a size and a colour; " +
				"it draws something there is no operation for, so nothing was drawn",
		}
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: "the " + what + " at " + quoteValue(src) +
				" is not one this engine can read: " + err.Error(),
		}
	}
	if cfg.Width <= 0 || cfg.Height <= 0 {
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: fmt.Sprintf("the %s %s at %s declares a size of %d by %d, which has no area",
				format, what, quoteValue(src), cfg.Width, cfg.Height),
		}
	}

	pixels := int64(cfg.Width) * int64(cfg.Height)
	if pixels > maxImagePixels {
		// The header said so; nothing has been decoded. This is the whole
		// reason the header is read first.
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: fmt.Sprintf(
				"the %s %s at %s declares %d by %d pixels, more than the %d "+
					"this engine will decode; it was not drawn",
				format, what, quoteValue(src), cfg.Width, cfg.Height, maxImagePixels),
		}
	}
	if pixels > l.budget {
		if !l.exhausted {
			l.exhausted = true
			l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
				"the document's images together need more than the %d pixels this "+
					"engine will decode for one document; the rest were not drawn",
				maxDocumentPixels))
		}
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: fmt.Sprintf(
				"the %s %s at %s was not drawn: the document's decode budget was already spent",
				format, what, quoteValue(src)),
		}
	}

	// Charged now, for what the header declares, and kept whatever the decode
	// does. A decoder allocates what the header says before it reads a row,
	// so a picture whose body fails has cost what one that succeeds costs —
	// sixty-four megabytes for a 4000 by 4000 PNG with its last chunk cut off —
	// and charging only a success let a document do that as often as it named
	// the file (audit C19).
	if !l.rec.charge(pixels*costPixel, "the pictures past that point") {
		l.cut = true
		return nil, l.cutShort(src, what)
	}
	l.budget -= pixels

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		// A header that parsed and a body that did not. It is a finding rather
		// than a crash, which is the property that matters here: these decoders
		// run on bytes an author did not write.
		return nil, &loadFailure{
			rule: RuleImageUndecodable,
			message: "the " + what + " at " + quoteValue(src) +
				" has a readable header and unreadable content: " + err.Error(),
		}
	}
	bounds := img.Bounds()
	if bounds.Dx() != cfg.Width || bounds.Dy() != cfg.Height {
		// The header and the pixels disagree. Trusting the header would mean
		// sizing a box from a number the decoder itself did not honour, so the
		// decoded bounds win. The budget was charged for the header's pixels
		// before the decode, and is charged the difference when the decoder
		// allocated more than that.
		if int64(bounds.Dx())*int64(bounds.Dy()) > maxImagePixels {
			return nil, &loadFailure{
				rule: RuleImageUndecodable,
				message: "the " + what + " at " + quoteValue(src) +
					" decoded to more pixels than its header declared, past what this engine will hold",
			}
		}
		cfg.Width, cfg.Height = bounds.Dx(), bounds.Dy()
		if got := int64(cfg.Width) * int64(cfg.Height); got > pixels {
			l.budget -= got - pixels
			l.rec.chargeOwn((got-pixels)*costPixel, "the pictures past that point")
			pixels = got
		}
	}

	return &ReplacedContent{
		Image:  img,
		Width:  mustPx(float64(cfg.Width)),
		Height: mustPx(float64(cfg.Height)),
		Ratio:  float64(cfg.Width) / float64(cfg.Height),
		Key:    fmt.Sprintf("%x", sum[:8]),
		Pixels: pixels,
	}, nil
}

// decodeDataURI reads a "data:" reference, by the Fetch standard's data: URL
// processor: the type is everything before the first comma, the body is what
// follows it percent-decoded, and the body is base64 when — and only when — the
// type ends in ";base64".
//
// The bytes never left the document, so there is no policy question — only the
// caps, which apply exactly as they do to a file. The length is checked before
// the decode rather than after, because base64 expands by three quarters and a
// cap applied to the result is a cap applied to an allocation already made.
//
// Three things the processor says that a looser reading gets wrong, each of
// which a document meets:
//
//   - The body is percent-decoded the URL standard's way, which leaves a "%"
//     that begins no escape as it is. net/url's PathUnescape refused the whole
//     URL at one, and a hand-written SVG says "100%" as often as it says
//     anything.
//   - The fragment is not part of the body. A URL ends its path at the first
//     "#", and a data: URL is a URL: "data:image/svg+xml,<svg fill='#f00'…"
//     is the body "<svg fill='" in every browser, which is why such documents
//     write "%23".
//   - ";base64" counts only at the end of the type. "data:text/plain;base64;
//     charset=x," is not base64, and the decode is Infra's forgiving one: white
//     space anywhere is dropped and the padding may be left off, and anything
//     else outside the alphabet is not base64 at all.
//
// src has been through referenceText, so the tabs and newlines an attribute
// wrapped it across are already gone, as the URL parser removes them.
//
// what names the kind of thing being read and bad is the rule to raise when it
// cannot be, because the callers report under different ones: an image that
// will not decode is undecodable, and a stylesheet that will not decode was
// never loaded. The policy and the caps are one piece of code either way, which
// is the point. The type is returned lowercased and without its parameters —
// its essence — or as "text/plain" when the URL names none, as the processor
// defaults it.
func decodeDataURI(src, what string, bad Rule) ([]byte, string, *loadFailure) {
	const prefix = "data:"
	rest := src[len(prefix):]
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		rest = rest[:i]
	}
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return nil, "", &loadFailure{
			rule:    bad,
			message: "a data: " + what + " has no comma separating its type from its content",
		}
	}
	meta, payload := strings.Trim(rest[:comma], " \t\n\f\r"), rest[comma+1:]
	if len(payload) > maxDataURIBytes {
		return nil, "", &loadFailure{
			rule: bad,
			message: fmt.Sprintf(
				"a data: %s carries %d encoded bytes, more than the %d this engine will read",
				what, len(payload), maxDataURIBytes),
		}
	}
	body := percentDecode(payload)

	meta, isBase64 := cutBase64Meta(meta)
	if isBase64 {
		data, ok := forgivingBase64(body)
		if !ok {
			return nil, "", &loadFailure{
				rule:    bad,
				message: "a data: " + what + " says it is base64 and is not",
			}
		}
		body = string(data)
	}
	return []byte(body), dataURIEssence(meta), nil
}

// cutBase64Meta is step 11 of the data: URL processor: a type ending in ";",
// any number of spaces, and "base64" in any case, says the body is base64, and
// that suffix is taken off the type.
func cutBase64Meta(meta string) (string, bool) {
	const word = "base64"
	if len(meta) < len(word) || !strings.EqualFold(meta[len(meta)-len(word):], word) {
		return meta, false
	}
	head := strings.TrimRight(meta[:len(meta)-len(word)], " ")
	if !strings.HasSuffix(head, ";") {
		return meta, false
	}
	return head[:len(head)-1], true
}

// forgivingBase64 is Infra's forgiving-base64 decode: ASCII white space is
// dropped wherever it is, one or two "=" may end a body whose length is a
// multiple of four, a body one character past a multiple of four is refused,
// and so is any character outside the base64 alphabet. The bits left over at
// the end are discarded.
func forgivingBase64(s string) ([]byte, bool) {
	clean := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			clean = append(clean, c)
		}
	}
	if len(clean)%4 == 0 {
		for n := 0; n < 2 && len(clean) > 0 && clean[len(clean)-1] == '='; n++ {
			clean = clean[:len(clean)-1]
		}
	}
	if len(clean)%4 == 1 {
		return nil, false
	}
	for _, c := range clean {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '+' || c == '/') {
			return nil, false
		}
	}
	out := make([]byte, base64.RawStdEncoding.DecodedLen(len(clean)))
	n, err := base64.RawStdEncoding.Decode(out, clean)
	if err != nil {
		return nil, false
	}
	return out[:n], true
}

// dataURIEssence is a data: URL's type without its parameters, lowercased: the
// part a reader decides what the bytes are by. A URL that names no type, or
// only parameters, is "text/plain", which is what the processor makes of it.
func dataURIEssence(meta string) string {
	if i := strings.IndexByte(meta, ';'); i >= 0 {
		meta = meta[:i]
	}
	meta = strings.ToLower(strings.Trim(meta, " \t\n\f\r"))
	if meta == "" || !strings.Contains(meta, "/") {
		return "text/plain"
	}
	return meta
}

// notReplaced reports why an element is not replaced and gives it its alt text.
func (l *replacedLoader) notReplaced(b *Box, fail *loadFailure) {
	if fail != nil {
		l.rec.ReportDetail(Finding{
			Rule:    fail.rule,
			Source:  AtHTML(b.Element.Offset),
			Message: fail.message,
			Path:    PathOf(b.Element),
		})
	}
	l.altOnly(b)
}

// altOnly gives an element that could not be replaced the text that stands in
// for it.
//
// CSS is explicit that an element whose replaced content is unavailable is not
// a replaced element at all, so it is an ordinary inline box — and HTML is
// equally explicit that what it then contains is the alt text. An engine that
// left the box empty would produce a page with a silent gap where a caption
// was, which is worse than either the image or the words.
func (l *replacedLoader) altOnly(b *Box) {
	if len(b.Children) > 0 {
		// Already given one; a document that repeats a src reaches here twice.
		return
	}
	alt, ok := b.Element.Attr("alt")
	if !ok {
		return
	}
	text := collapseWhitespaceAfter(alt, b.Style.Get("white-space-collapse"),
		wordSpaceTransformValue(b.Style), textBoundary{}, l.writingSystemAt(b.Element))
	if strings.TrimSpace(text) == "" {
		// alt="" is a deliberate statement that the image carries no
		// information, and generating a box for it would put a space on the
		// line the author asked to be empty.
		return
	}
	child := &Box{
		Outer: OuterInline, Inner: InnerText,
		Style: b.Style, Text: text, FontSize: b.FontSize,
		fontSizeKnown: b.fontSizeKnown, Parent: b,
	}
	b.Children = append(b.Children, child)
}

// isXMLMIMEType is the MIME Sniffing standard's XML MIME type (§4.6): a
// subtype ending in "+xml", or text/xml or application/xml. It is asked of an
// essence, which is lowercased and has no parameters.
func isXMLMIMEType(essence string) bool {
	return strings.HasSuffix(essence, "+xml") || essence == "text/xml" || essence == "application/xml"
}

// looksLikeSVG reports whether bytes that came with no type are meant to be an
// SVG.
//
// It reads the start of the file rather than the file name, because the name is
// what a document says and the bytes are what arrived. The MIME Sniffing
// standard has no pattern for an SVG — a browser knows one from the type it was
// served with — so what is asked is what makes a file an SVG in the first
// place: an XML document whose root element is <svg>.
//
// So the XML prolog is read structurally, as XML 1.0 §2.8 writes it: a byte
// order mark, then white space, processing instructions (the XML declaration
// among them), comments and one doctype, in any order, and then the root. It
// used to be a search for "<svg" within the first kilobyte, which gave up on
// an SVG opening with a licence comment longer than that and reported it as an
// unknown format — the wrong reason, and a picture not drawn (audit C137).
//
// The scan is bounded by maxSVGBytes, which is the most the reader will read
// at all, and it is linear: every construct it skips is skipped by searching
// for its end once. A prolog longer than the bound is not an SVG this engine
// would read anyway.
func looksLikeSVG(data []byte) bool {
	if len(data) > maxSVGBytes {
		data = data[:maxSVGBytes]
	}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // a byte order mark
	if head := bytes.TrimLeft(data, " \t\r\n\f"); len(head) == 0 || head[0] != '<' {
		// Every binary format this reads begins with bytes of its own — PNG
		// with an 0x89, JPEG with an 0xFF, GIF with a "G" — and none of them
		// begins with "<". A search for "<svg" anywhere in the bytes is a
		// search for four bytes that occur in compressed data as often as any
		// other four: a PNG with them in its first chunk was read as a picture
		// this engine cannot draw and refused.
		return false
	}
	for doctype := false; ; {
		data = bytes.TrimLeft(data, " \t\r\n\f")
		switch {
		case hasFoldPrefix(data, "<?"):
			// The XML declaration and any other processing instruction.
			end := bytes.Index(data, []byte("?>"))
			if end < 0 {
				return false
			}
			data = data[end+2:]
		case hasFoldPrefix(data, "<!--"):
			end := bytes.Index(data[4:], []byte("-->"))
			if end < 0 {
				return false
			}
			data = data[4+end+3:]
		case hasFoldPrefix(data, "<!doctype"):
			if doctype {
				return false
			}
			doctype = true
			end := doctypeEnd(data)
			if end < 0 {
				return false
			}
			data = data[end:]
		case hasFoldPrefix(data, "<svg"):
			// The root, if the name ends there: "<svgx>" is another element.
			// A prefixed "<svg:svg>" is not read, because the reader does not
			// read one either.
			rest := data[len("<svg"):]
			return len(rest) > 0 && (rest[0] == '>' || rest[0] == '/' || isXMLSpace(rest[0]))
		default:
			return false
		}
	}
}

// doctypeEnd is the offset just past a doctype declaration, or -1 when the
// bytes end first.
//
// A doctype ends at the first ">" that is not inside a quoted literal or its
// internal subset, and the subset holds declarations, comments and literals of
// its own — an entity whose value is "<svg>" is a string, not the root. So the
// one pass tracks the three, and it is still one pass.
func doctypeEnd(d []byte) int {
	depth := 0
	for i := len("<!doctype"); i < len(d); i++ {
		switch c := d[i]; {
		case c == '"' || c == '\'':
			end := bytes.IndexByte(d[i+1:], c)
			if end < 0 {
				return -1
			}
			i += 1 + end
		case depth > 0 && hasFoldPrefix(d[i:], "<!--"):
			end := bytes.Index(d[i+4:], []byte("-->"))
			if end < 0 {
				return -1
			}
			i += 4 + end + 2
		case depth > 0 && hasFoldPrefix(d[i:], "<?"):
			end := bytes.Index(d[i+2:], []byte("?>"))
			if end < 0 {
				return -1
			}
			i += 2 + end + 1
		case c == '[':
			depth++
		case c == ']':
			if depth > 0 {
				depth--
			}
		case c == '>' && depth == 0:
			return i + 1
		}
	}
	return -1
}

// isXMLSpace is XML 1.0's S: space, tab, carriage return and line feed.
func isXMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n'
}

// hasFoldPrefix reports whether b begins with an ASCII prefix, ignoring case.
func hasFoldPrefix(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		c := b[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != prefix[i] {
			return false
		}
	}
	return true
}

// foreign reads an inline <svg> as replaced content.
//
// It is the same reader an <img src=x.svg> goes through, on the same subset: an
// intrinsic size from the root element's attributes, and a colour when the whole
// picture reduces to one rectangle. See svg.go, which is explicit about how
// narrow that is.
//
// Nothing is fetched, so nothing is charged to the decode budget and no resolver
// is needed: the picture arrived with the document. What it shares with the file
// case is the *rules*, not the plumbing — one answer to "what may an SVG be" for
// both, rather than a second one here that would drift.
func (l *replacedLoader) foreign(b *Box) {
	name := strings.ToLower(b.Element.Name)
	if name != "svg" {
		l.rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedElement,
			Source:   AtHTML(offsetOf(b)),
			Message:  "<" + name + "> is not a picture this engine can draw; the element was laid out and left empty",
			Path:     PathOf(b.Element),
			Property: name,
		})
		b.Replaced = &ReplacedContent{}
		return
	}
	// The element and its content together are the document, which is what the
	// reader expects: the intrinsic size is on the root's own attributes.
	doc := "<svg " + attrSource(b.Element) + ">" + b.Element.Foreign + "</svg>"
	if c := svgContent([]byte(doc), svgAsImage); c != nil {
		b.Replaced = c
		return
	}
	// Nothing this can draw. The box is still a box — dropping it was what made
	// twenty-seven iframes pass by painting nothing — and it is still the box
	// the element asked for, because the size is on the element and not in the
	// picture. Only when the root says nothing either does it fall back to the
	// 300 by 150 of CSS 2.1 §10.3.2.
	if size := svgIntrinsicSize([]byte(doc), svgAsImage); size != nil {
		b.Replaced = size
	} else {
		b.Replaced = &ReplacedContent{}
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleUnsupportedElement,
		Source: AtHTML(offsetOf(b)),
		Message: "this <svg> draws something there is no operation for, so the " +
			"element was laid out and left empty",
		Path:     PathOf(b.Element),
		Property: "svg",
	})
}

// attrSource writes an element's attributes back as source, so that the SVG
// reader sees the root element it would have seen in a file.
//
// Less the two the document's own cascade has already read: "style", whose
// declarations were applied to the element's box and reported there if they are
// not implemented, and "hidden", which the user agent sheet turns into
// "display: none" before there is a box to read. Handed to the reader as well,
// they would be read a second time, as an SVG file's own CSS it does not have.
func attrSource(n *html.Node) string {
	var b strings.Builder
	for _, a := range n.Attrs {
		if a.Name == "" || strings.ContainsAny(a.Name, `"'<>`) {
			continue
		}
		if strings.EqualFold(a.Name, "style") || strings.EqualFold(a.Name, "hidden") {
			continue
		}
		b.WriteString(a.Name)
		b.WriteString(`="`)
		b.WriteString(strings.NewReplacer(`"`, "&quot;", "&", "&amp;", "<", "&lt;").Replace(a.Value))
		b.WriteString(`" `)
	}
	return b.String()
}
