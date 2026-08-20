package layout

import (
	"fmt"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
)

// The CSS a document carries, and the CSS it points at.
//
// A <style> element and a <link rel=stylesheet> are the same thing to the
// cascade — both are author stylesheets, and the only difference between them is
// where the bytes come from. The difference matters to everything else: the text
// of a <style> arrived inside the string the caller handed over, and the bytes
// behind a <link> did not.
//
// So the second one is a *resource*, and goes through resource.go's policy in
// full: no scheme, no absolute path, no escape from the directory the resolver
// was rooted at, and nothing at all when there is no resolver. There is no
// second path here that could disagree with the one <img> uses, which is the
// only way two policies stay the same — see the note on backgrounds in image.go
// for the same argument about the same resolver.
//
// # Why a stylesheet needs its own caps as well
//
// An image is bounded by what it decodes to. A stylesheet is bounded by nothing
// once it is read: every rule in it is matched against every element, so a
// megabyte of selectors is quadratic work the document did not have to carry.
// Two caps answer that — one on a sheet and one on how many a document may pull
// in — and both are checked here rather than left to the resolver, because a
// caller may supply a resolver of their own and the engine's limits must not
// depend on which one they wrote.

// maxStylesheetBytes is the largest linked stylesheet this engine will read.
//
// A megabyte is more CSS than any document has: the largest sheets on the web
// are a few hundred kilobytes, and a sheet past this is not a document's styles
// but a payload wearing their name. It bounds what is parsed and, through that,
// what the cascade has to match — which is the cost that matters, since every
// rule is tried against every element.
//
// It is a variable so that a test can lower it and watch it fire. A cap nobody
// has seen trip is one nobody knows works.
var maxStylesheetBytes = 1 << 20

// maxDocumentStylesheets is how many linked stylesheets one document may pull
// in.
//
// The per-sheet cap alone is not a bound on a document: a page with a thousand
// <link> elements is a thousand reads and a thousand parses, each of them
// legal. This is the count that makes the total finite, and it is deliberately
// low — a document needs a handful of stylesheets, and one that names twenty is
// already doing something other than styling itself.
//
// It counts sheets *fetched*, not <link> elements seen: a document may name the
// same file twice and be charged once, because it is read once.
var maxDocumentStylesheets = 20

// authorSheet is one author stylesheet in document order.
type authorSheet struct {
	// name identifies the sheet in a finding. It is the href for a linked
	// sheet and empty for a <style> element, matching the Source.Sheet
	// convention in finding.go.
	name string
	// source is the CSS.
	source string
}

// documentStylesheets collects every author stylesheet a document carries, in
// the order the cascade must see them.
//
// Document order is not a detail. The last tie-break in the cascade is which
// rule was written later, so a <link> that follows a <style> has to arrive after
// it — and a browser orders the two by their position in the markup rather than
// by their kind. Collecting them in one walk is what makes that true by
// construction instead of by a sort somebody has to keep right.
func documentStylesheets(doc *html.Node, res ResourceResolver, rec *Recorder) []authorSheet {
	l := &sheetLoader{res: res, rec: rec, failed: map[string]bool{}}
	var out []authorSheet
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		switch strings.ToLower(n.Name) {
		case "style":
			if text := n.TextContent(); text != "" {
				out = append(out, l.expandImports(authorSheet{source: text})...)
			}
			// A <style> element's content is raw text, so there is nothing
			// below it to walk.
			return false
		case "link":
			if s, ok := l.link(n); ok {
				out = append(out, l.expandImports(s)...)
			}
			return false
		}
		return true
	})
	return out
}

// sheetLoader fetches the stylesheets a document links to, under the caps.
type sheetLoader struct {
	res ResourceResolver
	rec *Recorder

	// cache holds the text of every reference already read, so a document
	// naming one sheet in ten <link> elements reads it once.
	//
	// It is a cache and not a suppression, and the difference is the cascade.
	// A repeated <link> is a stylesheet at *that* point in document order, and
	// dropping the repeat would let a <style> written between the two win a tie
	// it should lose. So the sheet is handed over again from here; what is
	// saved is the read.
	cache map[string]string
	// failed records the references already refused, so a document with ten
	// links to one missing file makes one attempt rather than ten. The
	// Recorder deduplicates the finding on its own; what this saves is the
	// system calls.
	failed map[string]bool
	// applied counts the stylesheets handed to the cascade from outside the
	// document, which is what maxDocumentStylesheets bounds.
	//
	// It counts sheets applied rather than files read, and that is what keeps
	// the cache above from being an amplifier: a document with a thousand links
	// to one megabyte reads it once and would otherwise parse and match it a
	// thousand times.
	applied int
	// capped records that the count cap was reported, so it is reported once.
	capped bool
}

// link turns one <link> element into a stylesheet, or explains why it did not.
func (l *sheetLoader) link(n *html.Node) (authorSheet, bool) {
	rel, _ := n.Attr("rel")
	if !relIsStylesheet(rel) {
		return authorSheet{}, false
	}
	href, _ := n.Attr("href")
	href = strings.TrimSpace(href)
	if href == "" {
		// A <link> with no href names nothing, exactly as an <img> with no src
		// does. There is no reference for a resolver to have refused.
		return authorSheet{}, false
	}
	if media, ok := n.Attr("media"); ok {
		apply, understood := mediaAppliesToPaper(media)
		if !understood {
			// A media query this engine cannot evaluate. Applying it would be a
			// guess and so would skipping it, and the guess that shows least is
			// the one nobody can see — so the sheet is left out and the reason
			// is named. A document whose whole appearance is behind a query
			// then reads as unstyled *and says so*, rather than reading as
			// styled by rules that may not have been meant for paper.
			l.rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: "the stylesheet at " + quoteValue(href) + " applies to " +
					quoteValue(strings.TrimSpace(media)) + ", and this engine evaluates no " +
					"media queries; it was not applied",
				Path:     PathOf(n),
				Property: "media",
			})
			return authorSheet{}, false
		}
		if !apply {
			// A sheet for a medium this is not. A page is printed, so a
			// screen-only sheet not applying is the correct answer rather than
			// a gap, and there is nothing to report.
			return authorSheet{}, false
		}
	}

	if l.failed[href] {
		// Already refused, and already reported. Retrying would be the same
		// answer at the cost of the same system calls.
		return authorSheet{}, false
	}
	// The document-wide count, checked before anything is read and against
	// sheets *applied*, so that the cache below cannot be used to get past it.
	if l.applied >= maxDocumentStylesheets {
		l.overCap(n, href)
		return authorSheet{}, false
	}
	if src, ok := l.cache[href]; ok {
		l.applied++
		return authorSheet{name: href, source: src}, true
	}

	src, fail := l.fetch(href)
	if fail != nil {
		l.failed[href] = true
		l.rec.ReportDetail(Finding{
			Rule:    fail.rule,
			Source:  AtHTML(n.Offset),
			Message: fail.message,
			Path:    PathOf(n),
		})
		return authorSheet{}, false
	}
	if l.cache == nil {
		l.cache = map[string]string{}
	}
	l.cache[href] = src
	l.applied++
	return authorSheet{name: href, source: src}, true
}

// overCap reports the document-wide count tripping.
//
// Two findings, because two different things are true and a caller filters on
// different ones: the guard tripped, which every other part of pdf0 reports as
// "limit"; and the document is missing styles it asked for, which is the thing
// that makes the page wrong. The first is raised once and the second names each
// sheet, because which stylesheet went missing is what an author needs.
func (l *sheetLoader) overCap(n *html.Node, href string) {
	if !l.capped {
		l.capped = true
		l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
			"this document applies more than the %d stylesheets this engine will load",
			maxDocumentStylesheets))
	}
	l.rec.ReportDetail(Finding{
		Rule:   RuleResourceBlocked,
		Source: AtHTML(n.Offset),
		Message: fmt.Sprintf("the stylesheet at %s was not applied: this document already "+
			"used the %d stylesheets this engine will load",
			quoteValue(href), maxDocumentStylesheets),
		Path: PathOf(n),
	})
}

// fetch obtains the text of one linked stylesheet, applying resource.go's
// policy and this file's size cap.
func (l *sheetLoader) fetch(href string) (string, *loadFailure) {
	data, fail := l.bytes(href)
	if fail != nil {
		return "", fail
	}
	if len(data) > maxStylesheetBytes {
		return "", &loadFailure{
			rule: RuleResourceBlocked,
			message: fmt.Sprintf(
				"the stylesheet at %s is %d bytes, more than the %d this engine will read",
				quoteValue(href), len(data), maxStylesheetBytes),
		}
	}
	return string(data), nil
}

// bytes is the fetch itself: the same three answers image.go's fetch gives, in
// the same order and for the same reasons.
func (l *sheetLoader) bytes(href string) ([]byte, *loadFailure) {
	if scheme, ok := schemeOf(href); ok {
		if scheme == "data" {
			return decodeDataURI(href, "stylesheet", RuleResourceBlocked)
		}
		return nil, &loadFailure{
			rule: RuleResourceBlocked,
			message: "the stylesheet at " + quoteValue(href) + " names the " + quoteValue(scheme) +
				" scheme; this engine resolves no URLs and fetches nothing, so it was not applied",
		}
	}
	if l.res == nil {
		return nil, &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the stylesheet at " + quoteValue(href) + " was not loaded: " + ErrNoResolver.Error(),
		}
	}
	data, err := l.res.Resolve(href)
	if err != nil {
		return nil, &loadFailure{
			rule:    RuleResourceBlocked,
			message: "the stylesheet at " + quoteValue(href) + " was not loaded: " + err.Error(),
		}
	}
	if len(data) == 0 {
		// An empty file is a stylesheet with no rules, which is a legal thing
		// for a document to link and is not a failure of anything.
		return nil, nil
	}
	return data, nil
}

// relIsStylesheet reports whether a link's rel names it a stylesheet that
// applies.
//
// The attribute is a set of space-separated keywords, so "stylesheet" has to be
// one of them rather than a substring — "no-stylesheet" is not a stylesheet, and
// neither is a rel of "prefetch stylesheet-ish".
//
// "alternate stylesheet" is a stylesheet the reader may choose and that no
// browser applies unless they do. Nothing here has a reader to ask, so it is not
// applied, and that is the same answer a browser gives rather than a gap: there
// is nothing to report.
func relIsStylesheet(rel string) bool {
	var stylesheet, alternate bool
	for _, f := range strings.Fields(rel) {
		switch strings.ToLower(f) {
		case "stylesheet":
			stylesheet = true
		case "alternate":
			alternate = true
		}
	}
	return stylesheet && !alternate
}

// mediaAppliesToPaper says whether a link's media attribute admits this engine's
// output, and whether it could be read at all.
//
// Only the simple form is answered: a comma-separated list of bare media types,
// which is what a document that wants to distinguish print from screen writes.
// Anything with a feature query in it — "screen and (min-width: 40em)", "not
// print", "only screen" — is *not* answered, because answering it would mean
// evaluating a media query and this engine evaluates none. Guessing would be
// silent either way, and silent is what the reporting layer exists to prevent.
//
// The medium is print. A PDF page is a sheet of paper with a fixed size and no
// scrolling, which is the medium "print" was defined for, and it is why "screen"
// is a *correct* refusal here rather than an unimplemented one.
func mediaAppliesToPaper(media string) (applies, understood bool) {
	media = strings.TrimSpace(media)
	if media == "" {
		return true, true
	}
	for _, part := range strings.Split(media, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		if !bareMediaTypes[part] {
			return false, false
		}
		if part == "all" || part == "print" {
			applies = true
		}
	}
	return applies, true
}

// bareMediaTypes is every media type that may stand alone in a media list.
//
// The deprecated ones are here because documents still carry them and because
// leaving them out would send a "media=tty" sheet down the unsupported path,
// where it would be reported as a query this engine cannot read. It can read it;
// the answer is simply no.
var bareMediaTypes = map[string]bool{
	"all": true, "print": true, "screen": true, "speech": true,
	"aural": true, "braille": true, "embossed": true, "handheld": true,
	"projection": true, "tty": true, "tv": true,
}

// @import, which is the other way a document names a stylesheet.
//
// It is a <link> written inside a sheet rather than inside the markup, and it
// exists because a sheet has no markup to write a <link> in: a document that
// keeps its font declarations in one file and its rules in another says so from
// the CSS, and the suite's letter-spacing tests are nineteen documents that do
// exactly that with "@import \"/fonts/ahem.css\"".
//
// What made it worth doing is not the count. A stylesheet that fails to arrive
// is a document rendered without its styles, and the engine reported the at-rule
// and carried on — so a page that had lost its font came out plausible, in the
// default face, with nothing on the page to say which of the two it was.
//
// # Where the imported rules go
//
// Before the rules of the sheet that imported them, which is what makes
// "@import" a way of saying "these are my defaults". The cascade's last
// tie-break is document order, so a sheet that imports another and then
// overrides one of its rules must have its own rule arrive later — and the way
// to make that true is to hand the two over as two sheets in that order rather
// than to splice text.
//
// # Why the at-rules are cut out of the source
//
// The importing sheet is handed on with its leading @import and @charset rules
// removed. Without that the cascade would meet an at-rule it does not apply and
// report it, which after this is not true: the rule *was* applied, by the loader,
// and the sheet it named is in the list. An @import written anywhere but at the
// top of a sheet is invalid CSS and is left where it is, so it is still reported
// — which is the right answer for a rule a browser also ignores.

// # What bounds a cycle
//
// A sheet may import a sheet that imports a sheet, and there is nothing in CSS
// to stop that being a cycle: "a.css" imports "b.css", which imports "a.css".
// The walk below is recursive, so something has to stop it.
//
// maxDocumentStylesheets is what does. Every level of the recursion spends one
// of that budget — fetchImport counts a sheet whether it was read or handed back
// from the cache, which is exactly so that a cycle is charged for going round —
// so the depth is bounded by the count, and a cycle is twenty steps rather than
// a stack overflow.
//
// A separate depth cap was written here first and could not be made to fail:
// with the count cap in place, planting its removal changed no output and hung
// nothing. It is gone, and a chain deeper than it allowed is now a test, because
// what it would have done to a real document is drop the sheet at the bottom.

// expandImports returns the sheets one author sheet stands for: everything it
// imports, in the order it imports them, and then what is left of it.
func (l *sheetLoader) expandImports(s authorSheet) []authorSheet {
	if !strings.Contains(s.source, "@import") {
		return []authorSheet{s}
	}
	rules, _ := css.ParseStylesheet(s.source)
	var out []authorSheet
	cut, found := 0, false
	for _, r := range rules {
		// Only the leading run. §4 of CSS Cascade puts @import before every
		// rule but @charset and @layer, and one written later is ignored.
		if !r.At || r.HasBlock {
			cut = r.Offset
			break
		}
		name := strings.ToLower(r.Name)
		if name == "charset" {
			cut = len(s.source)
			continue
		}
		if name != "import" {
			cut = r.Offset
			break
		}
		ref, ok := importReference(r.Prelude)
		if !ok {
			// A conditional import — "@import url(x) print" — or a prelude this
			// cannot read. Nothing is loaded and the rule is left where it is,
			// so the cascade reports it as the at-rule it did not apply.
			cut = r.Offset
			break
		}
		// Recognised, so it comes out of the source whether or not the file
		// behind it arrives: a reference that could not be read has been
		// reported by fetchImport, and leaving the rule in would have the
		// cascade report the same fact a second time and differently.
		cut, found = len(s.source), true
		if src, ok := l.fetchImport(ref, s.name); ok {
			out = append(out, l.expandImports(authorSheet{name: ref, source: src})...)
		}
	}
	if !found {
		return []authorSheet{s}
	}
	s.source = s.source[cut:]
	return append(out, s)
}

// importReference reads the URL out of an @import prelude, and declines
// anything with more in it than the URL.
//
// "@import url(x) print" is a conditional import, and this engine evaluates no
// media queries — see the <link media> case above for why guessing either way is
// worse than declining and saying so.
func importReference(prelude []css.ComponentValue) (string, bool) {
	ref, have := "", false
	for _, v := range prelude {
		switch {
		case v.IsToken() && v.Token.Kind == css.Whitespace:
			continue
		case v.IsToken() && v.Token.Kind == css.URL:
			if have {
				return "", false
			}
			ref, have = v.Token.Value, true
		case v.IsToken() && v.Token.Kind == css.String:
			if have {
				return "", false
			}
			ref, have = v.Token.Value, true
		case v.IsFunction() && strings.EqualFold(v.Token.Value, "url"):
			if have {
				return "", false
			}
			s, ok := singleString(v.Values)
			if !ok {
				return "", false
			}
			ref, have = s, true
		default:
			// A media query, a layer name, a supports() condition: more than a
			// reference, and more than this reads.
			return "", false
		}
	}
	return strings.TrimSpace(ref), have && strings.TrimSpace(ref) != ""
}

// fetchImport reads an imported sheet under the same caps and through the same
// cache as a linked one.
//
// from is the sheet the @import was written in, which is what a relative
// reference is relative to: "@import \"../base.css\"" inside "css/page.css"
// names "base.css", and resolving it against the document instead would name a
// file beside the document that is not there. A <style> element has no name and
// its imports are relative to the document, which is what an empty from means.
func (l *sheetLoader) fetchImport(ref, from string) (string, bool) {
	ref = resolveAgainstSheet(ref, from)
	if l.failed[ref] {
		// Already refused, and already reported. As with a <link> to the same
		// missing file, the Recorder deduplicates the finding on its own and
		// what this saves is the system calls — so removing it changes no
		// output, which is why there is no test below that it fails.
		return "", false
	}
	if src, ok := l.cache[ref]; ok {
		if l.applied >= maxDocumentStylesheets {
			return "", false
		}
		l.applied++
		return src, true
	}
	if l.applied >= maxDocumentStylesheets {
		l.overCapImport(ref)
		return "", false
	}
	src, fail := l.fetch(ref)
	if fail != nil {
		l.failed[ref] = true
		l.rec.ReportDetail(Finding{
			Rule:    fail.rule,
			Message: fail.message,
		})
		return "", false
	}
	if l.cache == nil {
		l.cache = map[string]string{}
	}
	l.cache[ref] = src
	l.applied++
	return src, true
}

// resolveAgainstSheet makes a reference written in one sheet relative to that
// sheet rather than to the document.
//
// A reference that begins at the root names itself, and a sheet with no name of
// its own — a <style> element — leaves the reference alone, because the document
// is what it is already relative to.
func resolveAgainstSheet(ref, from string) string {
	if from == "" || strings.HasPrefix(ref, "/") {
		return ref
	}
	i := strings.LastIndexByte(from, '/')
	if i < 0 {
		return ref
	}
	return from[:i+1] + ref
}

// overCapImport reports the document-wide count tripping on an @import. It is
// the same fact overCap reports for a <link> and is said the same way, without
// an element to point at.
func (l *sheetLoader) overCapImport(ref string) {
	if l.capped {
		return
	}
	l.capped = true
	l.rec.ReportDetail(Finding{
		Rule: RuleLimit,
		Message: fmt.Sprintf("this document reached the limit of %d stylesheets; "+
			"the @import of %s and any after it were not read",
			maxDocumentStylesheets, quoteValue(ref)),
	})
}
