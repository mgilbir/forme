package layout

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/style"
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
// Caps answer that — on the bytes of a sheet fetched, on how many sheets a
// document may pull in, and on the tokens of one sheet and of all of them — and
// all are checked here rather than left to the resolver, because a caller may
// supply a resolver of their own and the engine's limits must not depend on
// which one they wrote. The token caps are on every stylesheet and not only the
// fetched ones, because what a sheet costs is its tokens and not where they came
// from.

// maxStylesheetBytes is the largest linked or imported stylesheet this engine
// will read.
//
// A megabyte is more CSS than any document has: the largest sheets on the web
// are a few hundred kilobytes, and a sheet past this is not a document's styles
// but a payload wearing their name. It bounds what is fetched, which is bytes
// the document did not carry and the caller's resolver has to produce.
//
// It is not the bound on what a sheet costs to *parse*, which is the tokens in
// it; see maxStylesheetTokens.
//
// It is a variable so that a test can lower it and watch it fire. A cap nobody
// has seen trip is one nobody knows works.
var maxStylesheetBytes = 1 << 20

// maxStylesheetTokens is the most tokens of CSS this engine will parse from one
// stylesheet, from any source: a linked or imported file, a <style> element, a
// style attribute, and the sheets a caller passes in Input.CSS and
// Input.UserCSS.
//
// Parsing is paid for in tokens. Each becomes a ComponentValue of close to a
// hundred bytes, and a sheet can be a token per byte, so a megabyte of commas
// is a hundred megabytes of tree. Only fetched sheets were bounded, and by
// their bytes: a <style> had no bound at all, a document may be sixty-four
// megabytes of markup, and one sixteen-megabyte <style> was killed for memory
// under a four-gigabyte limit.
//
// Tokens rather than bytes, because the bytes are not the cost and bounding
// them refuses what costs nothing: an @font-face carrying its font as a base64
// data: URL is one string token and megabytes of text, and is the ordinary way
// a single-file document brings its face. A megabyte of tokens is what the
// byte cap on a fetched sheet already allowed.
var maxStylesheetTokens = 1 << 20

// maxDocumentStylesheetTokens is how many tokens of stylesheet one document may
// have applied, counted across every source maxStylesheetTokens bounds except
// style attributes — see styleAttributeTooLarge for why those are not counted.
//
// The per-sheet bound is not a bound on a document, for the reason
// maxDocumentStylesheets gives about the number of sheets: twenty linked sheets
// at the per-sheet bound are twenty legal reads, and <style> elements have no
// count at all. Every sheet applied is parsed and held until the cascade has
// run, so what a document holds is the sum of them. Four times the per-sheet
// bound is four of the largest sheet this engine reads, and ten times what the
// largest real document carries.
//
// A sheet is charged when it is applied, not when it is read, so a file linked
// ten times is charged ten times — it is parsed and held ten times.
var maxDocumentStylesheetTokens = 4 << 20

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
	// sheet — shortened, when the href is a long URL; see sheetName — and
	// empty for a <style> element, matching the Source.Sheet convention in
	// finding.go.
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
//
// The loader is the document's, shared with the sheets the caller passed, so
// that the bounds on a document are bounds on the document and not on each of
// the two places its stylesheets come from.
func documentStylesheets(doc *html.Node, l *sheetLoader) []authorSheet {
	var out []authorSheet
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		l.styleAttributeTooLarge(n)
		switch strings.ToLower(n.Name) {
		case "style":
			// HTML §4.2.6 gives <style> a media attribute and means by it what
			// <link> does. It was not read at all, so a document that kept its
			// screen rules in "<style media=screen>" — which is what a
			// single-file document writes instead of a second stylesheet — had
			// every one of them applied to the paper.
			if text := n.TextContent(); text != "" && l.mediaApplies(n, "this <style> element") &&
				l.admit(text, "this <style> element", AtHTML(n.Offset), PathOf(n)) {
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

	// media is the sheet a query is asked about: the page this document is
	// being printed on, before its own @page rules have narrowed it.
	media style.Media

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
	// open is the sheets being expanded right now, innermost last: the chain of
	// @imports that led here.
	//
	// A cycle — a.css imports b.css imports a.css — is not stopped by the cache,
	// because the cache is what makes the second read succeed. It ran until the
	// document-wide count refused the twenty-first sheet, applied the two sheets
	// ten times each, and said nothing: the refusal on the cached path returns
	// without a finding, so an author saw a document styled by rules applied ten
	// times over and no word about why.
	open []string
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
	// spent is the tokens of stylesheet applied so far, which is what
	// maxDocumentStylesheetTokens bounds, and tokensCapped records that the
	// bound was reported, so it is reported once.
	spent        int
	tokensCapped bool
}

// sheetName is what a fetched stylesheet is called in a finding.
//
// A path is its own name, and it has to be: it is also what an @import in the
// sheet is resolved against. A URL is not a directory to resolve against — see
// resolveAgainstSheet — so its name is only a name, and a "data:" URL's name
// was the whole stylesheet. Every finding about a rule in it carried the sheet
// in Source.Sheet, and the recorder read that out again for each one, so a
// data: sheet of three hundred kilobytes cost sixteen gigabytes of copying and
// one at the one-megabyte cap about a minute (audit C18).
//
// So a long URL is named by its beginning, which says what it is, its length,
// and a digest of the whole, which keeps two different sheets two names: the
// name is also what tells an @import cycle apart from two sheets that begin
// alike.
func sheetName(ref string) string {
	const keep = 48
	if len(ref) <= 2*keep {
		return ref
	}
	if _, named := schemeOf(ref); !named {
		return ref
	}
	sum := sha256.Sum256([]byte(ref))
	return ref[:cutAt(ref, keep)] + "… (" + strconv.Itoa(len(ref)) + " bytes, sha256 " +
		hex.EncodeToString(sum[:8]) + ")"
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
	if !l.mediaApplies(n, "the stylesheet at "+quoteValue(href)) {
		return authorSheet{}, false
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
		if why := l.charge(src); why != "" {
			l.overTokens(href, why, AtHTML(n.Offset), PathOf(n))
			return authorSheet{}, false
		}
		l.applied++
		return authorSheet{name: sheetName(href), source: src}, true
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
	if why := l.charge(src); why != "" {
		l.overTokens(href, why, AtHTML(n.Offset), PathOf(n))
		return authorSheet{}, false
	}
	l.applied++
	return authorSheet{name: sheetName(href), source: src}, true
}

// admit decides whether a stylesheet that was not fetched — a <style> element,
// or one of the caller's — may be applied, under the same two token bounds a
// fetched one is, and reports it by name when it may not.
//
// The finding is a limit, because that is what happened: the sheet is correct
// CSS the engine implements, and it was not read because a guard said so. It
// names the sheet, because "a stylesheet was dropped" sends an author looking
// through all of them.
func (l *sheetLoader) admit(src, what string, at Source, path string) bool {
	why := l.charge(src)
	if why == "" {
		return true
	}
	l.rec.ReportDetail(Finding{
		Rule:    RuleLimit,
		Source:  at,
		Message: what + " was not applied: " + why,
		Path:    path,
	})
	return false
}

// charge counts a stylesheet about to be applied against both token bounds, and
// says why it may not be applied, or nothing when it may. A sheet refused is not
// counted.
//
// The counting reads the sheet once before it is parsed, which is the price of
// knowing before the parse is paid for. It stops one past the bound, so a sheet
// too large costs no more to refuse than one at the bound.
func (l *sheetLoader) charge(src string) string {
	n := css.CountTokens(src, maxStylesheetTokens)
	if n > maxStylesheetTokens {
		return fmt.Sprintf("it is more than the %d tokens of CSS this engine will read "+
			"from one stylesheet (%d bytes)", maxStylesheetTokens, len(src))
	}
	if l.spent+n > maxDocumentStylesheetTokens {
		return fmt.Sprintf("with its %d tokens of CSS this document's stylesheets would come "+
			"to %d, more than the %d this engine will read", n, l.spent+n,
			maxDocumentStylesheetTokens)
	}
	l.spent += n
	return ""
}

// overTokens reports a fetched sheet refused by a token bound. It is overCap's
// two findings for the other bounds on what a document reads, and for the same
// reasons: the guard tripped, said once, and a file the document named was not
// applied, said for each.
func (l *sheetLoader) overTokens(href, why string, at Source, path string) {
	if !l.tokensCapped {
		l.tokensCapped = true
		l.rec.Report(RuleLimit, NoSource, fmt.Sprintf(
			"this document's stylesheets are more than this engine will read: %s", why))
	}
	l.rec.ReportDetail(Finding{
		Rule:    RuleResourceBlocked,
		Source:  at,
		Message: "the stylesheet at " + quoteValue(href) + " was not applied: " + why,
		Path:    path,
	})
}

// styleAttributeTooLarge refuses a style attribute longer than any stylesheet
// this engine reads, and reports it.
//
// A style attribute is a stylesheet without a selector, and parsing one costs
// what parsing a sheet of its tokens costs; the markup cap lets one be sixty
// megabytes of them. The attribute is emptied rather than removed, so that "[style]"
// still selects the element — the element has the attribute; what was not read
// is the declarations in it — and emptied here, before the cascade, because the
// cascade reads it for every element and has no bound of its own to apply.
//
// It is not charged against maxDocumentStylesheetTokens. A sheet is held from
// when it is parsed until every element has been styled, which is why the
// document's sheets are bounded together; a style attribute is parsed when its
// element is styled and let go when that element is done, so what one costs is
// its own tokens and never the sum.
//
// No token has fewer than one byte, so an attribute no longer than the bound
// in bytes is inside it in tokens and is not counted at all: counting is paid
// only by the attributes long enough to need it.
func (l *sheetLoader) styleAttributeTooLarge(n *html.Node) {
	for i := range n.Attrs {
		a := &n.Attrs[i]
		if a.Name != "style" || len(a.Value) <= maxStylesheetTokens ||
			css.CountTokens(a.Value, maxStylesheetTokens) <= maxStylesheetTokens {
			continue
		}
		l.rec.ReportDetail(Finding{
			Rule:   RuleLimit,
			Source: AtHTML(n.Offset),
			Message: fmt.Sprintf("this element's style attribute is more than the %d tokens "+
				"of CSS this engine will read from one stylesheet (%d bytes); it was not applied",
				maxStylesheetTokens, len(a.Value)),
			Path: PathOf(n),
		})
		a.Value = ""
	}
}

// overCap reports the document-wide count tripping.
//
// Two findings, because two different things are true and a caller filters on
// different ones: the guard tripped, which every other part of forme reports as
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

// bytes is the fetch itself: resource.go's policy, the one every reference in
// a document is read through.
func (l *sheetLoader) bytes(href string) ([]byte, *loadFailure) {
	data, _, fail := fetchReference(l.res, href, "stylesheet", "so it was not applied", RuleResourceBlocked)
	if fail != nil {
		return nil, fail
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

// mediaApplies answers the media attribute of a <link> or a <style> — and
// answers it with the engine's own media query evaluator, which is the point.
//
// There used to be a second reader here that took a comma-separated list of
// bare media types and refused everything else, because at the time this engine
// really did evaluate no media queries. Once style.MatchesMedia existed, that
// left one question with two answers: "@media print and (min-width: 200mm)"
// inside a sheet was evaluated exactly, and the identical query on the <link>
// that fetched the sheet was refused and reported. "not screen", "only print"
// and a bare "(min-width: 1px)" — every one of them the ordinary way to mark a
// sheet for paper — dropped the whole stylesheet.
//
// MatchesMedia is exported so that the stage outside the cascade asks the same
// question rather than keeping a copy of it. This is that stage.
//
// What is reported is what it is reported for anywhere else: a query naming
// something this engine cannot answer, because there a browser printing the
// same document may apply rules this page does not have. A query it answers —
// including "no" — is not a gap and says nothing.
func (l *sheetLoader) mediaApplies(n *html.Node, what string) bool {
	media, ok := n.Attr("media")
	if !ok {
		return true
	}
	vals, _ := css.ParseComponentValues(media)
	applies, unknown := style.MatchesMedia(vals, l.media)
	if unknown != "" {
		did := "it was not applied"
		if applies {
			did = "it was applied anyway, because another query in the list matched"
		}
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(n.Offset),
			Message: what + " applies to " + quoteValue(strings.TrimSpace(media)) +
				", which asks about " + quoteValue(unknown) + " — a question this " +
				"engine cannot answer, so " + did,
			Path:     PathOf(n),
			Property: "media",
		})
	}
	return applies
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
	if !containsFold(s.source, "@import") {
		return []authorSheet{s}
	}
	// This sheet is open while its imports are read, so that one of them naming
	// it again is seen as the ring it is. A sheet with no name cannot be named
	// by an import and so cannot be in one.
	if s.name != "" {
		l.open = append(l.open, s.name)
		defer func() { l.open = l.open[:len(l.open)-1] }()
	}
	rules, _ := css.ParseStylesheet(s.source)
	var out []authorSheet
	// The stretches of this sheet the imports occupied, which come out of it
	// once they have been read. Only those: what sits between them stays where
	// it was written.
	//
	// It used to be one cut at the end of the leading run, which threw away
	// everything before it — and a @layer statement is allowed to be there.
	// "@layer a, b; @import url(x);" is the shape a stylesheet written in
	// layers begins with, and the scan stopped at the @layer, so the import was
	// never read at all. Cutting only the imports keeps a statement written
	// after them and still lifts the rules that were imported; one written
	// before them is handed over separately, by flush below.
	var holes []sourceSpan
	end := func(i int) int {
		if i+1 < len(rules) {
			return rules[i+1].Offset
		}
		return len(s.source)
	}
	// The @layer statements written among the imports, waiting to be handed
	// over as a sheet of their own.
	//
	// They cannot simply stay where they are. An imported sheet is lifted out
	// and applied *before* what is left of the sheet that imported it, so a
	// statement left behind would fix the layer order after the imported rules
	// had already fixed it themselves — and "@layer theme, base; @import
	// url(x);" over a sheet defining both layers gave the win to theme, which
	// is the layer the author named first to make it lose. The statement is
	// handed over ahead of the sheets it orders instead, which is where it was
	// written.
	var pending []sourceSpan
	flush := func() {
		if len(pending) == 0 {
			return
		}
		var b strings.Builder
		for _, p := range pending {
			b.WriteString(s.source[p.from:p.to])
		}
		out = append(out, authorSheet{name: s.name, source: b.String()})
		// And out of the sheet, so the names are declared once. Re-declaring
		// them would change no order, but a malformed one would be reported
		// twice and at two different offsets.
		holes = append(holes, pending...)
		pending = nil
	}
	cut, found := 0, false
	for i, r := range rules {
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
		if name == "layer" {
			// A @layer *statement* — the block form was taken by HasBlock above
			// — names an order and is allowed among the imports. The order it
			// fixes is the whole of its effect, so it is kept rather than cut,
			// and kept ahead of the imports it was written ahead of.
			pending = append(pending, sourceSpan{from: r.Offset, to: end(i)})
			cut = len(s.source)
			continue
		}
		if name != "import" {
			cut = r.Offset
			break
		}
		// Ahead of the import, so the spans stay in order and any layer names
		// this import's sheet uses are already declared.
		flush()
		holes = append(holes, sourceSpan{from: r.Offset, to: end(i)})
		ref, media, ok := importReference(r.Prelude)
		if !ok {
			// A prelude this cannot read — a layer name, a supports() condition
			// — so nothing is loaded and the rule is left where it is, and the
			// cascade reports it as the at-rule it did not apply.
			cut = r.Offset
			break
		}
		// Recognised, so it comes out of the source whether or not the file
		// behind it arrives: a reference that could not be read has been
		// reported by fetchImport, and leaving the rule in would have the
		// cascade report the same fact a second time and differently. A media
		// query that answers no is recognised too — the rule was read and
		// correctly not applied, and leaving it in would have the cascade call
		// that a gap.
		cut, found = len(s.source), true
		if !l.importMedia(media, ref, r.Offset, s.name) {
			continue
		}
		at := Source{HTMLOffset: -1, CSSOffset: r.Offset, Sheet: s.name}
		if src, ok := l.fetchImport(ref, s.name, at); ok {
			resolved, _ := resolveAgainstSheet(ref, s.name)
			next := authorSheet{name: sheetName(resolved), source: src}
			if why := l.cycle(next.name); why != "" {
				l.rec.ReportDetail(Finding{
					Rule:    RuleInvalidCSS,
					Source:  at,
					Message: why,
				})
				continue
			}
			out = append(out, l.expandImports(next)...)
		}
	}
	if !found {
		return []authorSheet{s}
	}
	s.source = withoutSpans(s.source, cut, holes)
	return append(out, s)
}

// importReference reads an @import prelude: the URL, and the media query list
// that may follow it.
//
// CSS Cascade 5 §3.1 writes the prelude as the url, then an optional layer, then
// an optional supports() condition, then a media query list. The media query is
// read — it is the same query style.MatchesMedia answers inside a sheet, and
// "@import url(print.css) print" is how a stylesheet says which medium a file
// is for — and the other two are not, so a prelude carrying either is declined.
//
// Declining layer() is not a narrowing to be tidied away later. An imported
// sheet whose layer name was dropped would arrive *unlayered*, and an unlayered
// rule beats every layered one, so a sheet the author put at the bottom of the
// order would win against all of them. Left in the stylesheet, it is reported as
// an at-rule that was not applied, which is the answer that cannot mislead.
func importReference(prelude []css.ComponentValue) (ref string, media []css.ComponentValue, ok bool) {
	have := false
	for i, v := range prelude {
		switch {
		case v.IsToken() && v.Token.Kind == css.Whitespace:
			continue
		case v.IsToken() && v.Token.Kind == css.URL:
			if have {
				return "", nil, false
			}
			ref, have = v.Token.Value, true
		case v.IsToken() && v.Token.Kind == css.String:
			if have {
				return "", nil, false
			}
			ref, have = v.Token.Value, true
		case v.IsFunction() && strings.EqualFold(v.Token.Value, "url"):
			if have {
				return "", nil, false
			}
			s, ok := singleString(v.Values)
			if !ok {
				return "", nil, false
			}
			ref, have = s, true
		case !have:
			// Something before the URL, which the grammar has no place for.
			return "", nil, false
		case isLayerOrSupports(v):
			// A layer name or a supports() condition: more than this reads, and
			// the sheet is left where it is and reported.
			return "", nil, false
		default:
			// Everything after the URL that is not one of those is the media
			// query list, which runs to the end of the prelude.
			media = prelude[i:]
			ref = strings.TrimSpace(ref)
			return ref, media, ref != ""
		}
	}
	ref = strings.TrimSpace(ref)
	return ref, nil, have && ref != ""
}

// isLayerOrSupports reports the two prelude pieces that are not a media query.
//
// "layer" bare and "layer(name)" are the two spellings of the first, and
// supports() is the second. Neither can be mistaken for the start of a media
// query: "layer" and "supports" are media types this engine would answer "no"
// to, and answering them that way would silently drop a sheet the author asked
// for under a name that means something else entirely.
func isLayerOrSupports(v css.ComponentValue) bool {
	if v.IsToken() && v.Token.Kind == css.Ident && strings.EqualFold(v.Token.Value, "layer") {
		return true
	}
	return v.IsFunction() && (strings.EqualFold(v.Token.Value, "layer") ||
		strings.EqualFold(v.Token.Value, "supports"))
}

// importMedia answers the media query list on an @import, with the same
// evaluator every other media query in this engine is answered with.
//
// An empty list is every medium, which is what an @import with nothing after the
// URL means and what §2.1 says an empty list evaluates to anyway.
func (l *sheetLoader) importMedia(media []css.ComponentValue, ref string, offset int, sheet string) bool {
	if len(media) == 0 {
		return true
	}
	applies, unknown := style.MatchesMedia(media, l.media)
	if unknown != "" {
		did := "it was not read"
		if applies {
			did = "it was read anyway, because another query in the list matched"
		}
		l.rec.ReportDetail(Finding{
			Rule:   RuleUnsupportedValue,
			Source: Source{HTMLOffset: -1, CSSOffset: offset, Sheet: sheet},
			Message: "the import of " + quoteValue(ref) + " applies to " +
				quoteValue(strings.TrimSpace(pageText(media))) + ", which asks about " +
				quoteValue(unknown) + " — a question this engine cannot answer, so " + did,
			Property: "media",
		})
	}
	return applies
}

// fetchImport reads an imported sheet under the same caps and through the same
// cache as a linked one.
//
// from is the sheet the @import was written in, which is what a relative
// reference is relative to: "@import \"../base.css\"" inside "css/page.css"
// names "base.css", and resolving it against the document instead would name a
// file beside the document that is not there. A <style> element has no name and
// its imports are relative to the document, which is what an empty from means.
// at is where the @import was written, which is where a finding about it
// points.
func (l *sheetLoader) fetchImport(ref, from string, at Source) (string, bool) {
	ref, unresolved := resolveAgainstSheet(ref, from)
	if unresolved != "" {
		l.rec.ReportDetail(Finding{
			Rule:    RuleResourceBlocked,
			Source:  at,
			Message: "the @import of " + unresolved,
		})
		return "", false
	}
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
		if why := l.charge(src); why != "" {
			l.overTokens(ref, why, at, "")
			return "", false
		}
		l.applied++
		return src, true
	}
	if l.applied >= maxDocumentStylesheets {
		l.overCapImport(ref, at)
		return "", false
	}
	src, fail := l.fetch(ref)
	if fail != nil {
		l.failed[ref] = true
		l.rec.ReportDetail(Finding{
			Rule:    fail.rule,
			Source:  at,
			Message: fail.message,
		})
		return "", false
	}
	if l.cache == nil {
		l.cache = map[string]string{}
	}
	l.cache[ref] = src
	if why := l.charge(src); why != "" {
		l.overTokens(ref, why, at, "")
		return "", false
	}
	l.applied++
	return src, true
}

// containsFold is strings.Contains for an ASCII needle, ignoring case.
//
// The fast path in front of the parse, and it has to ignore case because an
// at-rule's name does: "@IMPORT" is an @import and a sheet holding one was
// handed to the cascade with the rule still in it, to be reported as an at-rule
// nothing applied. strings.ToLower would copy every stylesheet in the document
// to answer a question that is almost always no.
func containsFold(s, needle string) bool {
	if len(needle) == 0 || len(s) < len(needle) {
		return len(needle) == 0
	}
	fold := func(c byte) byte {
		if c >= 'A' && c <= 'Z' {
			return c + 'a' - 'A'
		}
		return c
	}
	for i := 0; i+len(needle) <= len(s); i++ {
		if fold(s[i]) != needle[0] {
			continue
		}
		match := true
		for j := 1; j < len(needle); j++ {
			if fold(s[i+j]) != needle[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// cycle says why a sheet may not be expanded again, or the empty string.
//
// A sheet already open is a sheet importing itself, directly or round a ring,
// and expanding it again produces nothing a reader wants: the same rules a
// second time, in the same place in the cascade, until a bound elsewhere stops
// it. The chain is named in the message because "a.css imports itself" and
// "a.css imports b.css imports a.css" are different mistakes to go and find.
func (l *sheetLoader) cycle(name string) string {
	for i, open := range l.open {
		if open != name {
			continue
		}
		chain := append(append([]string(nil), l.open[i:]...), name)
		return "the stylesheet at " + strconv.Quote(name) + " imports itself: " +
			strings.Join(chain, " imports ") + ", so the second time round was not applied"
	}
	return ""
}

// resolveAgainstSheet makes a reference written in one sheet relative to that
// sheet rather than to the document.
//
// It is the one resolver for every reference a stylesheet makes — an @import,
// and through resolveSheetURLs every url() in the sheet, an @font-face src and
// a background-image alike — because CSS Values 4 §4.5.1 has one rule for all
// of them: a relative URL in a stylesheet is relative to the stylesheet. Only
// the @import used to be; a font and a background in the same sheet were
// resolved against the document, so "css/a.css" importing "base.css" found
// "css/base.css" and in the next line asked for a font "f.ttf" beside the
// document, which was not there (audit C34).
//
// What it leaves alone, and why each is itself already:
//
//   - A reference beginning at the root, with either slash, names itself from
//     wherever the document is served, whichever sheet wrote it. That includes
//     "//host/x", which is refused later as the host it names; joining it onto
//     a directory would have hidden the host inside a path.
//   - A reference with a scheme is a whole URL. It used to be joined like a
//     path, so "http://evil/a.css" imported from "css/page.css" reached a
//     resolver as "css/http:/evil/a.css" — no longer a URL, and no longer
//     refused as one.
//   - A reference that is only a fragment is not a file: CSS Values 4 §4.5.1
//     makes "url(#x)" a reference into the document whatever sheet it is in.
//   - The empty reference names nothing, in any sheet.
//   - A sheet with no name — a <style> element — has the document as its base
//     already.
//
// And what it cannot resolve at all: any other reference in a sheet that
// arrived as a URL rather than as a path. See the note in the body; the second
// result says why, and is empty when the reference resolved.
//
// The join is cleaned, and that is the whole of what a resolver can be handed.
// "../base.css" written in "css/page.css" names "base.css", a file beside the
// document; joined and left alone it named "css/../base.css", which is the same
// file to anything that resolves paths and a parent-relative reference to
// anything that inspects them. DirResolver inspects them — it refuses a ".."
// segment outright, before os.Root ever sees the path — so an ordinary @import
// one directory up was refused as an attempt to leave the document's
// directory, which is a thing it was not doing.
//
// Only the path is cleaned. A query and a fragment are not path segments, and
// cleaning them with it turned "x.png?a=../b" in "css/" into "css/b".
//
// A reference that really does go above the sheet's own root keeps its "..":
// path.Clean has nowhere to take it, and the resolver refuses it as before.
func resolveAgainstSheet(ref, from string) (string, string) {
	ref = referenceText(ref)
	if from == "" || ref == "" || ref[0] == '#' {
		return ref, ""
	}
	if _, named := schemeOf(ref); named {
		return ref, ""
	}
	if scheme, named := schemeOf(from); named {
		// The sheet arrived as a URL rather than as a path — a "data:"
		// stylesheet is the one this engine can have — and a URL is not a
		// directory to join onto. "data:text/css,…" holds a slash in its media
		// type, so joining produced "data:text/theme.css", which is a
		// reference to nothing.
		//
		// Nor is the document the base instead, which is what this used to
		// answer. A data: URL's path is opaque, and the URL standard's basic
		// parser fails a reference with no scheme against a base like that —
		// a root-relative one as much as a relative one; only a fragment
		// survives it — so a browser loads nothing and the rule names nothing.
		// Reading it against the document instead loaded a file the sheet
		// never named. A sheet named by any other URL is the same case one
		// step later: the reference would resolve to a URL with that scheme,
		// which is refused.
		if scheme == "data" {
			return "", quoteValue(ref) + " is relative, and a data: stylesheet has no base " +
				"to resolve it against — a data: URL's path is opaque, so the URL " +
				"standard fails the reference — and nothing was loaded"
		}
		return "", quoteValue(ref) + " is relative to a stylesheet named by a " +
			quoteValue(scheme) + " URL, which would make it one; this engine resolves no " +
			"URLs, so nothing was loaded"
	}
	if ref[0] == '/' || ref[0] == '\\' {
		return ref, ""
	}
	base := from
	if i := strings.IndexAny(base, "?#"); i >= 0 {
		base = base[:i]
	}
	rel, suffix := ref, ""
	if i := strings.IndexAny(ref, "?#"); i >= 0 {
		rel, suffix = ref[:i], ref[i:]
	}
	if rel == "" {
		// "?v=2" alone: RFC 3986 §5.2.2 keeps the base's path and takes the
		// reference's query, so it is the sheet itself asked for again.
		return base + suffix, ""
	}
	i := strings.LastIndexByte(base, '/')
	if i < 0 {
		return path.Clean(rel) + suffix, ""
	}
	return path.Clean(base[:i+1]+rel) + suffix, ""
}

// resolveSheetURLs resolves every url() in a parsed stylesheet against the
// sheet, once, where the sheet is read — so that nothing downstream has a
// relative reference left to resolve against the wrong base.
//
// Here and not at each consumer, because by the time a consumer runs the sheet
// is gone: a background-image is a computed value, inherited and copied, and
// nothing in it says which of a document's sheets wrote it. A computed <url>
// is the resolved URL — CSS Values 4 §4.5.1, and every property's "computed
// value: as specified, with url values made absolute" — so resolving it where
// the sheet is still known is what a browser does too. Everything that reads a
// url() — backgrounds, list markers, generated content, @font-face — reads the
// resolved one, and the next thing that learns to read one will too.
//
// Only inside blocks. A url() in a declaration or in an @font-face descriptor
// is in the block of the rule it belongs to, nested rules included; the one
// url() in a prelude that is a resource is an @import's, which the loader has
// already resolved and taken out, and the other — @namespace — is a name that
// must not be resolved at all.
//
// The resolved text is charged to the work budget, because it is longer than
// what was written by the sheet's own name and that is the document's to
// choose: a <link> whose href is a megabyte of directory prefixes it to every
// url() in the sheet behind it. A reference the budget refuses is emptied
// rather than left as written, because as written it is relative to the
// document and names some other file; empty, it names nothing, and the budget
// has said what was cut.
func resolveSheetURLs(rules []css.Rule, sheet string, rec *Recorder) {
	if sheet == "" {
		return
	}
	for i := range rules {
		resolveURLsIn(rules[i].Block, sheet, rec)
	}
}

func resolveURLsIn(vals []css.ComponentValue, sheet string, rec *Recorder) {
	resolve := func(t *css.Token) {
		got, why := resolveAgainstSheet(t.Value, sheet)
		if why != "" {
			// Nothing to load, and said so where the reference was written.
			// The url() is emptied, which names nothing, rather than left
			// relative to the document, which names some other file.
			rec.ReportDetail(Finding{
				Rule:    RuleResourceBlocked,
				Source:  Source{HTMLOffset: -1, CSSOffset: t.Offset, Sheet: sheet},
				Message: "the url() " + why,
			})
			t.Value = ""
			return
		}
		if got == t.Value {
			return
		}
		if !rec.charge(int64(len(got)), "the stylesheet references past that point") {
			got = ""
		}
		t.Value = got
	}
	for i := range vals {
		v := &vals[i]
		switch {
		case v.IsToken() && v.Token.Kind == css.URL:
			resolve(&v.Token)
		case v.IsFunction() && isURLFunction(v.Token.Value):
			// url("x") and src("x"): the string inside is the reference, and
			// what follows it — CSS Values 4's url modifiers — is not.
			for j := range v.Values {
				if w := &v.Values[j]; w.IsToken() && w.Token.Kind == css.String {
					resolve(&w.Token)
					break
				}
			}
		default:
			// image-set() takes a bare string as a URL too (CSS Images 4
			// §2.2); this engine draws none, but the value it computes is
			// still the resolved one.
			if v.IsFunction() && isImageSet(v.Token.Value) {
				for j := range v.Values {
					if w := &v.Values[j]; w.IsToken() && w.Token.Kind == css.String {
						resolve(&w.Token)
					}
				}
			}
			resolveURLsIn(v.Values, sheet, rec)
		}
	}
}

// isURLFunction reports the two spellings of a <url> as a function: url() and
// CSS Values 4's src().
func isURLFunction(name string) bool {
	return strings.EqualFold(name, "url") || strings.EqualFold(name, "src")
}

func isImageSet(name string) bool {
	return strings.EqualFold(name, "image-set") || strings.EqualFold(name, "-webkit-image-set")
}

// overCapImport reports the document-wide count tripping on an @import. It is
// the same fact overCap reports for a <link> and is said the same way, pointing
// at the @import rather than at an element.
func (l *sheetLoader) overCapImport(ref string, at Source) {
	if l.capped {
		return
	}
	l.capped = true
	l.rec.ReportDetail(Finding{
		Rule:   RuleLimit,
		Source: at,
		Message: fmt.Sprintf("this document reached the limit of %d stylesheets; "+
			"the @import of %s and any after it were not read",
			maxDocumentStylesheets, quoteValue(ref)),
	})
}

// sourceSpan is a stretch of a stylesheet's own text.
type sourceSpan struct{ from, to int }

// withoutSpans is a sheet's source up to cut with the given stretches removed,
// followed by everything from cut onwards.
//
// The stretches are the rules already handed over as sheets of their own: the
// @imports, and the @layer statements written ahead of them. What is left
// between them — a @layer statement written after the imports, which §3.1
// allows — stays where it was, because the order it fixes is what the rest of
// the sheet is read against.
func withoutSpans(source string, cut int, holes []sourceSpan) string {
	if len(holes) == 0 {
		return source[cut:]
	}
	var b strings.Builder
	at := 0
	for _, h := range holes {
		if h.from > cut {
			break
		}
		if h.from > at {
			b.WriteString(source[at:h.from])
		}
		if h.to > at {
			at = h.to
		}
	}
	if cut > at {
		b.WriteString(source[at:cut])
	}
	if cut < len(source) {
		b.WriteString(source[cut:])
	}
	return b.String()
}
