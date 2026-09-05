package layout

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"image"
	"net/url"
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
	res ResourceResolver
	rec *Recorder

	// loaded memoizes by reference, so a document that repeats one src reads,
	// decodes and charges the budget once.
	loaded map[string]*ReplacedContent
	// failed records the references already reported, so a page of a hundred
	// broken images is one finding rather than a hundred.
	failed map[string]bool

	// budget is how many pixels the document may still decode.
	budget int64
	// exhausted records that the budget ran out, reported once.
	exhausted bool
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
		loaded: map[string]*ReplacedContent{},
		failed: map[string]bool{},
		budget: maxDocumentPixels,
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
	refs := backgroundImageRefs(b.Style["background-image"])
	if len(refs) == 0 {
		return
	}
	for _, ref := range refs {
		if got, ok := l.loaded[ref]; ok {
			l.attachBackground(b, ref, got)
			continue
		}
		if l.failed[ref] {
			continue
		}
		content, why := l.load(ref, "background image", svgAsImage)
		if content == nil {
			l.failed[ref] = true
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
		l.loaded[ref] = content
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
		l.notReplaced(b, nil)
		return
	}

	if got, ok := l.loaded[src]; ok {
		b.Replaced = got
		return
	}
	if l.failed[src] {
		l.altOnly(b)
		return
	}

	content, why := l.load(src, "image", svgAsImage)
	if content == nil {
		l.failed[src] = true
		l.notReplaced(b, why)
		return
	}
	l.loaded[src] = content
	b.Replaced = content
}

// object gives an <object> the picture its data names, or reports that it could
// not and lays out the fallback content instead.
//
// HTML §4.8.7 hands the resource to a plugin, a nested browsing context or an
// image decoder, and which of the three depends on what arrived. The first two
// are what §4.1 of the proposal refuses outright — a plugin is arbitrary code
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
	if got, ok := l.loaded[data]; ok {
		l.embed(b, got)
		return
	}
	if l.failed[data] {
		l.fallbackTo(b, nil, data)
		return
	}
	content, why := l.load(data, "object", svgAsDocument)
	if content == nil {
		l.failed[data] = true
		l.fallbackTo(b, why, data)
		return
	}
	l.loaded[data] = content
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
	ref, ok := urlValue(b.Style["list-style-image"])
	if !ok || strings.TrimSpace(ref) == "" {
		return
	}
	ref = strings.TrimSpace(ref)
	if got, seen := l.loaded[ref]; seen {
		b.MarkerImage = got
		return
	}
	if l.failed[ref] {
		return
	}
	content, _ := l.load(ref, "list marker image", svgAsImage)
	if content == nil {
		l.failed[ref] = true
		return
	}
	l.loaded[ref] = content
	b.MarkerImage = content
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
	if got, seen := l.loaded[ref]; seen {
		b.Replaced = got
		return
	}
	if l.failed[ref] {
		return
	}
	content, why := l.load(ref, "generated content image", svgAsImage)
	if content == nil {
		l.failed[ref] = true
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
	l.loaded[ref] = content
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
func (l *replacedLoader) load(src, what string, as svgAs) (*ReplacedContent, *loadFailure) {
	data, fail := l.fetch(src, what)
	if fail != nil {
		return nil, fail
	}
	return l.decode(src, what, data, as)
}

// fetch obtains the bytes a reference names, applying the policy of resource.go.
func (l *replacedLoader) fetch(src, what string) ([]byte, *loadFailure) {
	if scheme, ok := schemeOf(src); ok {
		if scheme == "data" {
			return decodeDataURI(src, what, RuleImageUndecodable)
		}
		return nil, &loadFailure{
			rule: RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " names the " + quoteValue(scheme) +
				" scheme; this engine resolves no URLs and fetches nothing, so it was not drawn",
		}
	}
	if l.res == nil {
		return nil, &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " was not loaded: " + ErrNoResolver.Error(),
		}
	}
	data, err := l.res.Resolve(src)
	if err != nil {
		return nil, &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the " + what + " at " + quoteValue(src) + " was not loaded: " + err.Error(),
		}
	}
	if len(data) == 0 {
		return nil, &loadFailure{
			rule:    RuleImageUndecodable,
			message: "the " + what + " at " + quoteValue(src) + " is empty",
		}
	}
	return data, nil
}

// decode reads a header, checks it against the caps, and only then decodes.
func (l *replacedLoader) decode(src, what string, data []byte, as svgAs) (*ReplacedContent, *loadFailure) {
	// An SVG is not a picture and never becomes one. It is read for its
	// intrinsic size and, when its content reduces to one, its colour — see
	// svg.go, which is explicit about how narrow that is and why the rest keeps
	// its finding. It has to be tried before image.DecodeConfig because no
	// decoder here reads XML, so an SVG would otherwise be an unknown format.
	if looksLikeSVG(data) {
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
		// decoded bounds win and the budget is charged for what was really
		// allocated.
		if int64(bounds.Dx())*int64(bounds.Dy()) > maxImagePixels {
			return nil, &loadFailure{
				rule: RuleImageUndecodable,
				message: "the " + what + " at " + quoteValue(src) +
					" decoded to more pixels than its header declared, past what this engine will hold",
			}
		}
		cfg.Width, cfg.Height = bounds.Dx(), bounds.Dy()
		pixels = int64(cfg.Width) * int64(cfg.Height)
	}
	l.budget -= pixels

	sum := sha256.Sum256(data)
	return &ReplacedContent{
		Image:  img,
		Width:  mustPx(float64(cfg.Width)),
		Height: mustPx(float64(cfg.Height)),
		Ratio:  float64(cfg.Width) / float64(cfg.Height),
		Key:    fmt.Sprintf("%x", sum[:8]),
		Pixels: pixels,
	}, nil
}

// decodeDataURI reads a "data:" reference.
//
// The bytes never left the document, so there is no policy question — only the
// caps, which apply exactly as they do to a file. The length is checked before
// the decode rather than after, because base64 expands by three quarters and a
// cap applied to the result is a cap applied to an allocation already made.
//
// what names the kind of thing being read and bad is the rule to raise when it
// cannot be, because the two callers report under different ones: an image that
// will not decode is undecodable, and a stylesheet that will not decode was
// never loaded. The policy and the caps are one piece of code either way, which
// is the point.
func decodeDataURI(src, what string, bad Rule) ([]byte, *loadFailure) {
	const prefix = "data:"
	rest := src[len(prefix):]
	comma := strings.IndexByte(rest, ',')
	if comma < 0 {
		return nil, &loadFailure{
			rule:    bad,
			message: "a data: " + what + " has no comma separating its type from its content",
		}
	}
	meta, payload := rest[:comma], rest[comma+1:]
	if len(payload) > maxDataURIBytes {
		return nil, &loadFailure{
			rule: bad,
			message: fmt.Sprintf(
				"a data: %s carries %d encoded bytes, more than the %d this engine will read",
				what, len(payload), maxDataURIBytes),
		}
	}

	if isBase64Meta(meta) {
		// Strict decoding of a fixed-length string: there is no stream to
		// bound, because the bound was applied to the input above and base64
		// shrinks.
		data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(payload))
		if err != nil {
			// Some documents pad badly or use the URL alphabet; try the two
			// tolerant spellings before giving up, and no further.
			data, err = base64.RawStdEncoding.DecodeString(strings.TrimSpace(payload))
			if err != nil {
				return nil, &loadFailure{
					rule:    bad,
					message: "a data: " + what + " is not valid base64: " + err.Error(),
				}
			}
		}
		return data, nil
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil {
		return nil, &loadFailure{
			rule:    bad,
			message: "a data: " + what + " is not readable: " + err.Error(),
		}
	}
	return []byte(decoded), nil
}

// isBase64Meta reports whether a data URI's parameters end in ";base64".
func isBase64Meta(meta string) bool {
	for _, part := range strings.Split(meta, ";") {
		if strings.EqualFold(strings.TrimSpace(part), "base64") {
			return true
		}
	}
	return false
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
	text := collapseWhitespaceAfter(alt, b.Style["white-space-collapse"],
		wordSpaceTransformValue(b.Style), textBoundary{}, writingSystemAt(b.Element))
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

// looksLikeSVG reports whether the bytes are meant to be an SVG.
//
// It reads the start of the file rather than the file name, because the name is
// what a document says and the bytes are what arrived. A leading XML declaration
// or doctype may come first, so this looks for the root tag within the opening
// stretch rather than at offset zero.
func looksLikeSVG(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	return bytes.Contains(head, []byte("<svg")) || bytes.Contains(head, []byte("<SVG"))
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
func attrSource(n *html.Node) string {
	var b strings.Builder
	for _, a := range n.Attrs {
		if a.Name == "" || strings.ContainsAny(a.Name, `"'<>`) {
			continue
		}
		b.WriteString(a.Name)
		b.WriteString(`="`)
		b.WriteString(strings.NewReplacer(`"`, "&quot;", "&", "&amp;", "<", "&lt;").Replace(a.Value))
		b.WriteString(`" `)
	}
	return b.String()
}
