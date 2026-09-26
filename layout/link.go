package layout

import (
	"github.com/mgilbir/forme/html"
)

// Hyperlinks: where an <a href> is on the page, for a backend to make a link
// annotation of.
//
// Nothing here is drawn. A link is an area of the page and a target, and it
// reaches the display list as a Link so that a backend does not have to find
// the <a>'s boxes again in the fragment tree — which would be re-deriving
// layout's geometry, the thing the display list exists so that nothing
// downstream does.
//
// # Which elements are links
//
// An HTML <a> element with an href attribute, and nothing else. That is
// HTML's definition of a hyperlink an element generates boxes for (<area> is
// the other, and it is an image map's region rather than a box), and it is
// the same test the user agent sheet's "a[href]" makes for the underline: an
// <a> with no href is a placeholder, and is neither underlined nor a link.
//
// # What a link's area is
//
// The border boxes of the fragments the <a> generates: one per line an inline
// <a> is broken across, one for a block-level or atomic <a>. It is the border
// box because that is the area CSSOM View's getClientRects() returns for an
// element — one rectangle per box fragment — and the area a browser hit-tests
// to decide that a click landed on it. An inline <a>'s fragment is §10.6.1's
// content area with its vertical padding and border outside it, measured the
// way its background is, so a link whose <a> has a background is exactly the
// area the background covers.
//
// That area misses what an inline <a> holds and does not enclose: an image
// taller than the line, an inline-block, a float, an absolutely positioned
// box, and a block §9.2.1.1 lifted out of it. Each of those has a fragment of
// its own, and a browser sends a click on any of them to the <a> around it —
// so their border boxes are the link's too. A block or atomic <a>'s own
// border box already encloses what is laid out inside it, and what overflows
// it is not added: that is content outside the box, and a link rectangle for
// every box inside a link around a whole table would be a rectangle per cell
// for an area one rectangle already covers.
//
// # What its target is
//
// The href as the URL standard reads it — see referenceText — and otherwise
// as written. A relative reference is relative to the document, which is what
// every reference in the markup is (see ResourceResolver), and the document's
// own URL is the caller's to know and not this engine's: it is handed over
// unresolved, for the backend to resolve against whatever it knows the
// document's address to be. A reference that is only a fragment, "#terms",
// is a link into the document itself.
//
// # Which targets are refused
//
// A link is not fetched by this engine, so resource.go's boundary — which is
// about what the engine reads — is not the question here. The question is what
// a reader's viewer does when the link is followed, and for a document whose
// author is untrusted the answer has to be decided by a list of what is
// allowed, because the list of what is dangerous is open-ended: "javascript:"
// runs script in whatever follows it, "data:" puts a page of the author's
// choosing at an address that looks like nothing, "file:" names the reader's
// own disk, and an operating system registers handlers for schemes nobody has
// heard of until one of them is an exploit ("ms-msdt:" was one). So:
//
//   - http, https and mailto are links. They are what a link in a document is
//     for, and a viewer follows each by handing it to a browser or a mail
//     client, which is where a reader expects it to go.
//   - A reference with no scheme is a link, because it resolves against the
//     document's own address and so has the document's scheme.
//   - Every other scheme is refused, and so is a reference naming a host with
//     its scheme left out ("//host/x"): against a file: document that is a
//     file share on another machine, which is a credential leak on a system
//     that authenticates to one when asked. It is the same refusal
//     resource.go makes of the same spelling.
//
// A refused link is reported, under RuleLinkRefused, and the <a> is laid out
// and drawn exactly as it would have been: its content is on the page, and
// only the Link is not in the display list. The scheme is asked of the
// reference as the URL standard reads it, so "java\tscript:" is refused as the
// "javascript:" it is to every URL parser a viewer might use.

// hyperlink is the target of one <a>. Every box the element generates carries
// the same pointer, which is what makes it the element's identity in the
// painter.
type hyperlink struct {
	href string
}

// linkSchemes are the schemes a link may name. See above for why it is a
// list of what is allowed.
var linkSchemes = map[string]bool{"http": true, "https": true, "mailto": true}

// hyperlinkOf is the link an element is, or nil when it is not one or its
// href is refused — which is reported here, once per element, because this is
// the one place every element passes through once.
func (b *boxBuilder) hyperlinkOf(n *html.Node) *hyperlink {
	if n == nil || n.Type != html.ElementNode || n.Name != "a" {
		return nil
	}
	href, ok := n.AttrExact("href")
	if !ok {
		return nil
	}
	target, why := linkTarget(href)
	if why != "" {
		b.rec.ReportDetail(Finding{
			Rule: RuleLinkRefused,
			Message: "the link to " + quoteValue(href) + " " + why +
				", so it is not a link in the display list; its content is drawn",
			Source: sourceOf(n),
			Path:   PathOf(n),
		})
		return nil
	}
	return &hyperlink{href: target}
}

// linkTarget is an href as a link's target, or why it is refused.
func linkTarget(href string) (string, string) {
	ref := referenceText(href)
	if scheme, named := schemeOf(ref); named {
		if !linkSchemes[scheme] {
			return "", "names the " + quoteValue(scheme) + " scheme, which a link is not made for"
		}
		return ref, ""
	}
	if namesAHost(ref) {
		return "", "names a host with its scheme left out"
	}
	return ref, ""
}

// linkOf is the link a fragment of b is an area of, or nil.
//
// Its own, when b is the <a>. Otherwise the innermost inline <a> it is inside
// without an atomic box or a block between: the fragment of an image, an
// inline-block, a float or an absolutely positioned box written inside an
// inline <a> is not enclosed by the <a>'s own fragments, which are its content
// area on each line, and a click on it is a click on the link. A block
// §9.2.1.1 lifted out of the <a> is the same case, and is found through
// splitFrom, because the split made it a sibling of the <a>'s pieces rather
// than a child. Inside a block or an atomic box, the <a> above is that box's
// business and not this one's: its own area already encloses this.
func (p *painter) linkOf(b *Box) *hyperlink {
	if b == nil || b.IsText() {
		return nil
	}
	if b.link != nil {
		return b.link
	}
	if l := p.linkAbove(b.Parent); l != nil {
		return l
	}
	for i := len(b.splitFrom) - 1; i >= 0; i-- {
		if l := p.linkAbove(b.splitFrom[i]); l != nil {
			return l
		}
	}
	return nil
}

// linkAbove is the innermost link among b and the non-atomic inline boxes
// around it, up to the block container whose lines they are on.
//
// It is memoized per box and built the way paintedInlines builds its chains —
// up to the first box answered, and down again — because a box inside d
// nested spans, each holding an inline-block, asks it d times, and a walk of
// the whole chain from each would be the square of the depth.
func (p *painter) linkAbove(b *Box) *hyperlink {
	var path []*Box
	var out *hyperlink
	for cur := b; ; cur = cur.Parent {
		if cur == nil || cur.Outer != OuterInline || cur.Replaced != nil || isAtomicInline(cur) {
			// The block container, or an atomic inline, which is a box of
			// its own: nothing above it is this fragment's link.
			out = nil
			break
		}
		if got, ok := p.inlineLinks[cur]; ok {
			out = got
			break
		}
		p.linkSteps++
		path = append(path, cur)
	}
	if p.inlineLinks == nil && len(path) > 0 {
		p.inlineLinks = map[*Box]*hyperlink{}
	}
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].link != nil {
			out = path[i].link
		}
		p.inlineLinks[path[i]] = out
	}
	return out
}

// linkArea emits one area of a link: f's border box, cut by clip.
//
// A box §11.2 hides is not an area, because it is not drawn and a browser does
// not hit-test it. What is emitted here is one Link per area, charged to the
// work budget as any mark is; gatherLinks makes them one per <a> when the
// paint is done.
func (p *painter) linkArea(f *Fragment, clip Clip, l *hyperlink) {
	if l == nil || f == nil || isHidden(f.Box) || f.BorderRect.Empty() {
		return
	}
	p.clipping(clip, func() {
		p.emit(Link{Rects: []Rect{f.BorderRect}, Href: l.href, of: l})
	})
}

// gatherLinks makes each <a>'s areas one Link, at the place of the first.
//
// The areas are emitted one at a time because each is painted, and clipped,
// where its fragment is: a link broken across three lines is three areas met
// in three places, and a link around a block lifted out of it is areas under
// two different clips. Gathering them afterwards is what lets each be cut by
// its own clip and still reach a backend as one link, which is what a link
// annotation is.
//
// The work is one pass and a map, in the number of operations, and the list
// only shrinks.
func gatherLinks(ops []Op) []Op {
	var first map[*hyperlink]int
	kept := ops[:0]
	for _, op := range ops {
		l, ok := op.(Link)
		if !ok || l.of == nil {
			kept = append(kept, op)
			continue
		}
		if i, seen := first[l.of]; seen {
			into := kept[i].(Link)
			into.Rects = append(into.Rects, l.Rects...)
			kept[i] = into
			continue
		}
		if first == nil {
			first = map[*hyperlink]int{}
		}
		first[l.of] = len(kept)
		kept = append(kept, l)
	}
	for _, i := range first {
		l := kept[i].(Link)
		l.of = nil
		kept[i] = l
	}
	// What the gathering left behind is dropped rather than left for the
	// caller to see past the end of the slice.
	for i := len(kept); i < len(ops); i++ {
		ops[i] = nil
	}
	return kept
}
