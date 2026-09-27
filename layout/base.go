package layout

import (
	"net/url"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// The document's base URL: HTML §4.2.3.
//
// A reference written in the markup — an <img src>, a <link href>, an <a href>,
// an <object data> — is relative to the document's base URL, and so is a url()
// in a <style> element or a style attribute, whose base is the document's (CSS
// Values 4 §4.5.1). That base is the frozen base URL of the first <base>
// element in the document with an href, in tree order, and the document's
// fallback base URL — its own address — where there is none. A second <base>,
// and a <base> with no href, change nothing.
//
// This engine never knows the document's own address. A resolver is handed
// references relative to the document (see ResourceResolver) and a link that
// is relative is handed to the backend as written (see link.go), and both are
// the same statement: the fallback base is the caller's to know. So the frozen
// base URL is held here as a reference relative to the document, which is what
// resolving its href against the fallback base is when the fallback is left
// unnamed: <base href="assets/"> is "assets/", and "logo.png" under it is
// "assets/logo.png" — a reference relative to the document again, which the
// resolver is handed as every other one is. The join is resolveAgainst, the
// one a stylesheet's references are made relative to their sheet with, and for
// the same reason: a base that is a path is a directory to join onto.
//
// # What a base may not do
//
// resource.go's boundary is about what the engine reads, and a <base> is one
// string in an untrusted document that every other reference in it is joined
// onto. So it is asked before anything is joined, and a base that names what
// a reference may not name makes every reference relative to it name it too:
//
//   - A base with a scheme — <base href="https://cdn.example/"> — makes each
//     relative reference a URL with that scheme. Nothing is fetched from one,
//     so none of them is loaded, and each is reported as the URL it became.
//     Reading "logo.png" beside the document instead, as though there were
//     no base, is loading a file the document did not name.
//   - A base naming a host with its scheme left out — <base href="//cdn/"> —
//     makes each a reference to that host, and is refused as one. Joined as a
//     path, "//cdn/" and "x.png" came to "/cdn/x.png" once path.Clean had
//     folded the slashes: a host turned into a path on the document's root.
//   - A base that is a path, "../" or "/static/" included, joins into
//     references a document could have written itself, and the resolver's
//     containment answers them as it answers those. It is not refused here,
//     because refusing it refuses an ordinary document.
//
// A reference that is itself a whole URL, or a data: URL, is not relative to
// any base, and is read as it always was.
//
// A link is the one reference that is not read, and for it a base with a
// scheme is exactly what the element is for: a saved page's
// <base href="https://example.com/docs/"> makes <a href="intro.html"> a link to
// https://example.com/docs/intro.html. So a link is resolved as a URL against
// an http or https base, and the target is then held to link.go's list of what
// a link may be — which is what keeps a base of "javascript:" from making every
// relative link in the document run script. Against a base with any other
// scheme a relative link is refused, since the URL it would become has that
// scheme, and none but http, https and mailto is a link; mailto's is not a base
// anything but a fragment can be resolved against.
//
// # What it costs
//
// Each reference joined onto a base is as long as the base, and the base is
// the document's to choose: a megabyte <base href> under ten thousand images
// is ten gigabytes of joined references. Each join is charged to the work
// budget by its length, as a url() made relative to a long stylesheet name is,
// and a reference the budget refuses is not loaded and not a link.
type documentBase struct {
	// href is the frozen base URL, as the URL standard reads the attribute's
	// value (see referenceText) and relative to the document. Empty when no
	// <base> gave one, which is the document itself.
	href string
}

// baseOf names the base in a refusal.
const baseOf = "the document's base URL"

// documentBaseOf finds the first <base> element with an href, in tree order.
//
// Its own walk, which stops at the first one, rather than a question put to a
// walk made for something else: the element is in the <head> in every
// document that has one, and a document with none is walked once.
func documentBaseOf(doc *html.Node) documentBase {
	if doc == nil {
		return documentBase{}
	}
	stack := []*html.Node{doc}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		// The name as the document's language gives it: HTML lowercases it as
		// it parses, and in XHTML only "base" is HTML's element.
		if n.Type == html.ElementNode && n.Name == "base" {
			if href, ok := n.AttrExact("href"); ok {
				return documentBase{href: referenceText(href)}
			}
		}
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, n.Children[i])
		}
	}
	return documentBase{}
}

// resolve makes a reference written in the markup, or in a sheet whose base is
// the document's, relative to the document again: joined onto the base, or
// refused with why it could not be. what names the reference in the refusal —
// "image", "stylesheet".
//
// With no base it is the reference as the URL standard reads it, which is what
// every loader was handed before there was one.
func (d documentBase) resolve(ref, what string, rec *Recorder) (string, *loadFailure) {
	if d.href == "" {
		return referenceText(ref), nil
	}
	got, why := resolveAgainst(ref, d.href, baseOf, d.afford(rec))
	if why != "" {
		return "", &loadFailure{rule: RuleResourceBlocked, message: "the " + what + " " + why}
	}
	if got == "" {
		return "", &loadFailure{
			rule: RuleLimit,
			message: "the " + what + " at " + quoteValue(ref) + " was not resolved against " +
				baseOf + ", because the document had used up the work this engine does " +
				"for one document, so it was not loaded",
		}
	}
	return got, nil
}

// afford charges a join onto the base before it is made. See documentBase and
// resolveAgainst.
func (d documentBase) afford(rec *Recorder) func(n int) bool {
	return func(n int) bool {
		return rec.charge(int64(n), "the references resolved against "+baseOf+" past that point")
	}
}

// link is an href's target, or why it is not a link. See documentBase for why
// a link is resolved differently from a reference that is read.
func (d documentBase) link(href string, rec *Recorder) (string, string) {
	if d.href == "" {
		return linkTarget(href)
	}
	ref := referenceText(href)
	scheme, named := schemeOf(d.href)
	if refScheme, ok := schemeOf(ref); ok {
		// A whole URL, relative to nothing — unless it names the base's own
		// special scheme and no host, which the URL standard reads as a
		// reference relative to the base: "https:intro.html" against
		// "https://example.com/docs/" is intro.html there.
		rest := ref[len(refScheme)+1:]
		if !named || refScheme != scheme || !specialScheme(scheme) || namesAHost(rest) {
			return linkTarget(ref)
		}
		ref = rest
	}
	var got string
	switch {
	case scheme == "http" || scheme == "https":
		if !d.afford(rec)(len(d.href) + len(ref)) {
			return "", budgetRefusedLink
		}
		resolved, ok := resolveURL(d.href, ref)
		if !ok {
			return "", "could not be resolved against " + baseOf + ", " + quoteValue(d.href)
		}
		got = resolved
	case named:
		return "", "is relative to " + baseOf + ", a " + quoteValue(scheme) +
			" URL, which would make it one; a link is made for none but http, https and mailto"
	case namesAHost(d.href):
		return "", "is relative to " + baseOf + ", which names a host with its scheme left out"
	case ref == "" || ref[0] == '#':
		// Nothing, or only a fragment: the document the base names, and that
		// place in it. HTML resolves an href as a URL, and a URL that is only
		// a fragment keeps its base's path — which is why a link to "#top"
		// under <base href="other.html"> leaves the page it is on. CSS's rule
		// that url(#x) stays in the document is CSS's, and resolveAgainst
		// keeps it for the stylesheets.
		base := d.href
		if i := strings.IndexByte(base, '#'); i >= 0 {
			base = base[:i]
		}
		if !d.afford(rec)(len(base) + len(ref)) {
			return "", budgetRefusedLink
		}
		got = base + ref
	default:
		// No refusal is possible but the budget's: the base is a path, and
		// the reference has no scheme.
		if got, _ = resolveAgainst(ref, d.href, baseOf, d.afford(rec)); got == "" {
			return "", budgetRefusedLink
		}
	}
	return linkTarget(got)
}

// budgetRefusedLink is why a link the work budget would not resolve is not one.
const budgetRefusedLink = "was not resolved against " + baseOf + ", because the document " +
	"had used up the work this engine does for one document"

// specialScheme is the URL standard's special schemes: the ones in which a
// backslash is a slash, and in which a reference naming the base's own scheme
// and no host is relative to it.
func specialScheme(s string) bool {
	switch s {
	case "http", "https", "ws", "wss", "ftp", "file":
		return true
	}
	return false
}

// resolveURL resolves a reference against an absolute base with a special
// scheme, as the URL standard does for the parts that differ from RFC 3986
// here: a backslash is a slash, before the query. What remains is RFC 3986's
// §5.2 merge, which net/url implements.
//
// It fails where either does not parse, and where the base has no host to
// resolve against — "https:x", which the URL standard would repair and which a
// document writing it as its base has already got wrong.
func resolveURL(base, ref string) (string, bool) {
	b, err := url.Parse(specialSlashes(base))
	if err != nil || b.Opaque != "" || b.Host == "" {
		return "", false
	}
	r, err := url.Parse(specialSlashes(ref))
	if err != nil {
		return "", false
	}
	return b.ResolveReference(r).String(), true
}

// specialSlashes turns each backslash before the query or the fragment into a
// slash, which is how the URL standard reads a URL with a special scheme.
func specialSlashes(s string) string {
	end := len(s)
	if i := strings.IndexAny(s, "?#"); i >= 0 {
		end = i
	}
	if !strings.Contains(s[:end], `\`) {
		return s
	}
	return strings.ReplaceAll(s[:end], `\`, "/") + s[end:]
}

// inlineURLs makes the url()s in one declaration of a style attribute relative
// to the document's base URL, where the attribute is on n. It is handed to the
// cascade, which reads the attribute; see style.InlineURLs.
func (d documentBase) inlineURLs(rec *Recorder) func(n *html.Node, values []css.ComponentValue) {
	if d.href == "" {
		return nil
	}
	return func(n *html.Node, values []css.ComponentValue) {
		resolveURLsIn(values, d.href, baseOf, func(int) Source { return AtHTML(n.Offset) }, rec)
	}
}
