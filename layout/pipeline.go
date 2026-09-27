package layout

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/style"
)

// Running the stages that exist, and translating what each of them says into
// the guardrail vocabulary.
//
// The translation is the part worth attention. The html, css and style packages
// each report in their own terms, and each already distinguishes "the input is
// wrong" from "this engine does not do that" — but a caller should not have to
// learn three vocabularies to find that out. Everything arrives here as a rule
// identifier and a place in the author's input.
//
// This is not the entry point most callers want. Compose is: it runs these
// stages, lays the result out on a sheet, decides the scale and paints it. What
// is here is the prefix of that, exposed because the stages are worth testing
// against real documents rather than against hand-built trees, and because a
// caller laying out its own pages needs the box tree without the page.

// Stylesheet is one stylesheet with a name to report against.
type Stylesheet struct {
	// Name identifies the sheet in a finding — a filename, usually. It is empty
	// for the document's own <style> content.
	//
	// It is also what every relative reference inside this sheet is resolved
	// against — an @import, an @font-face src, a background-image, any url() —
	// because that is what a reference in a stylesheet is relative to (CSS
	// Values 4 §4.5.1): an "@import \"base.css\"" or a "url(bg.png)" in a
	// sheet named "css/page.css" asks for "css/base.css" or "css/bg.png", and
	// the same in a sheet with no name asks for a file beside the document. So
	// the name is a path and not a label — naming a sheet "the caller's theme"
	// would send its references looking in a directory called that.
	Name string
	// Source is the CSS.
	Source string
}

// Input is a document and the stylesheets to apply to it.
type Input struct {
	// HTML is the document source.
	HTML string
	// XHTML says the document is served as application/xhtml+xml, and is read
	// as XHTML whatever it says about itself: <style> holds character data,
	// "<div/>" is empty, element and attribute names are case-sensitive. It is
	// what a browser learns from the MIME type, and what a caller knows from
	// where the file came from — its content type, or an ".xht" extension.
	//
	// Left false, the document decides: an XML declaration or a doctype naming
	// XHTML makes it XHTML, and anything else is HTML. See html.ParseXHTML.
	XHTML bool
	// CSS is the author's stylesheets, in the order they apply.
	CSS []Stylesheet
	// Policy chooses what each rule does. A nil policy uses the defaults.
	Policy Policy
	// UserCSS is a stylesheet applied on the reader's behalf, which sits
	// between the engine's defaults and the author's. It is separate from CSS
	// because its origin is different, and origin is the strongest term in the
	// cascade.
	UserCSS string

	// Resources supplies the bytes of the files the document refers to: the
	// pictures an <img>, an <object>, a video's poster, a background, a list
	// marker or generated content names, the stylesheets a <link
	// rel=stylesheet> or an @import does, and the fonts an @font-face does.
	//
	// A nil resolver loads nothing, which is the deliberate default: a document
	// is untrusted input, and "src" and "href" are strings in it. See
	// resource.go for what a resolver may and may not do, and NewDirResolver
	// for the contained filesystem one.
	Resources ResourceResolver

	// Fonts is the caller's font library: the faces a document may name and
	// have, before it brings any of its own. A nil set is the fourteen standard
	// PDF faces.
	//
	// It is on the input rather than on the render options because an
	// @font-face is part of the *document*, and the set a document is laid out
	// in is the caller's library with the document's own faces over it. Build
	// puts the two together and returns the result as Built.Fonts, which is
	// what a caller calling Layout directly must pass on.
	Fonts FontSet
}

// Built is the result of the stages that exist.
type Built struct {
	// Document is the parsed tree, present even when findings were raised.
	Document *html.Node
	// Root is the root of the box tree, nil when the document produces no
	// boxes — which "html { display: none }" legitimately does.
	Root *Box
	// Styles is every element's computed style.
	Styles map[*html.Node]style.ComputedStyle
	// Page is the sheet the document is to be laid out on: the one the caller
	// asked for, with what the document's own @page rules said about it
	// applied. A caller calling Layout directly should measure against this
	// rather than against the page it passed in, or a document that set its own
	// margins is laid out in the space it did not ask for.
	Page PageSize
	// Fonts is the set the document is to be laid out in: the caller's library
	// with the faces the document's own @font-face rules loaded over it. It is
	// never nil, and it is what a caller calling Layout directly must hand it —
	// passing Input.Fonts instead would lay the document out without the fonts
	// it brought.
	Fonts FontSet
	// Findings is everything worth telling the caller, ordered deterministically.
	Findings []Finding
	// Failed reports that something fired at Error severity, so a caller that
	// went on to render would be rendering something it was told not to.
	Failed bool
	// Truncated reports that the finding list was cut.
	Truncated bool
}

// Build parses, styles and boxes a document.
//
// The media queries in the document are answered against A4, because Build has
// no sheet of its own and A4 is the sheet Compose uses when it is not told
// otherwise. A caller laying the boxes out on something else should call
// BuildFor, or the two features a query can ask about — the width and the
// height of the paper — will be answered about a page it is not printing on.
func Build(in Input) Built {
	return BuildFor(in, A4)
}

// BuildFor is Build for a known sheet, which is what a media query is asked
// about and what the document's own @page rules are applied over. The sheet it
// settled on is Built.Page, and that — not the one passed here — is what the
// boxes are to be laid out in. See Build.
func BuildFor(in Input, page PageSize) Built {
	return buildWith(in, page, NewRecorder(in.Policy))
}

// buildWith is BuildFor into a recorder the caller already has.
//
// Compose has one: it raises findings of its own about the options and the
// sheet before the document is read, and more about the layout and the paint
// after. Replaying Build's finished list into it instead — which is what it
// did — loses three things.
//
// The counts, because a replay carries the *deduplicated* list: a stylesheet
// that used one unimplemented property four hundred times comes back as one
// finding, and the second recorder counts one. What Count is for is saying "and
// 399 more", and after a replay it says "and none more".
//
// The bound, because a document whose build filled the five hundred hands the
// second recorder five hundred findings before layout begins — so every finding
// about the layout and the paint is dropped, and the page that overflowed its
// box is not reported. The two stages shared a bound they did not share a list
// with.
//
// And the work: every finding is deduplicated twice, once in each recorder.
func buildWith(in Input, page PageSize, rec *Recorder) Built {
	// The document's share of the work budget, for what the caller handed in.
	// A sheet the document fetches for itself earns nothing: a budget that
	// grew with what a document chose to link would be one a document could
	// raise. See budget.go.
	input := len(in.HTML) + len(in.UserCSS)
	for _, s := range in.CSS {
		input += len(s.Source)
	}
	rec.work.grant(input)

	parse := html.Parse
	if in.XHTML {
		parse = html.ParseXHTML
	}
	doc, htmlErrs, _ := parse(in.HTML)
	for _, e := range htmlErrs {
		// Three kinds, and they are three because they send an author to three
		// different places: fix the markup, the engine does not do this, or the
		// engine stopped short. A bound that was reached is the third — the
		// document is correct and part of it was not read anyway — and
		// reporting it as invalid markup sent an author looking for a mistake
		// that was not there.
		rule := RuleInvalidMarkup
		switch {
		case e.Limit:
			rule = RuleLimit
		case e.Unsupported:
			rule = RuleUnsupportedElement
		}
		rec.Report(rule, AtHTML(e.Offset), e.Message)
	}

	sheets := make([]style.Sheet, 0, len(in.CSS)+2)
	sheets = append(sheets, userAgentSheet(rec))
	// The sheet every media query in the document is asked about: the page the
	// caller named, and not the one the document's @page rules go on to
	// choose. Those rules are inside the sheets and the @media blocks being
	// chosen here, so a query answered about their answer is circular — "@media
	// (min-width: 250mm) { @page { size: A3 } }" would decide itself — and the
	// only definition that is not is the medium's own. It is the one answer for
	// a <link media>, an @import's media, an @media around an @page and an
	// @media around a style rule alike. They used to be two: every in-sheet
	// @media was asked again about the sheet after @page, so with the caller on
	// A4 and "@page { size: A3 }" a <link media="(min-width: 250mm)"> was not
	// applied and the same query in a <style> was (audit C139).
	asked := style.Media{Width: page.Width, Height: page.Height}
	// One loader for every stylesheet the document is given, whichever side it
	// came from, because the bounds it applies are on the document: two loaders
	// were two budgets, and the caller's sheets and the document's own could
	// each spend a whole one.
	// The document's base URL, which every reference in its markup and in its
	// own stylesheets is relative to. See base.go.
	base := documentBaseOf(doc)
	importer := &sheetLoader{res: in.Resources, rec: rec, base: base, media: asked,
		failed: map[string]bool{}}
	if in.UserCSS != "" && importer.admit(in.UserCSS, "the user stylesheet", NoSource, "") {
		// Through the importer like every other author-supplied sheet. A user
		// stylesheet is CSS a person wrote, and an @import in one is the same
		// request it is anywhere else — left unexpanded it was reported as an
		// at-rule this engine does not apply, which is not what happens to the
		// identical line in the document's own sheet.
		for _, e := range importer.expandImports(authorSheet{name: "user", base: "user", source: in.UserCSS}) {
			sheets = append(sheets, parseSheet(rec, style.OriginUser, e.name, e.base, e.source))
		}
	}
	// A <style> element and a <link rel=stylesheet> are both author stylesheets,
	// and they come before the ones the caller passed only because they were
	// written first — order is the cascade's last tie-break and it has to be the
	// order the author would expect. documentStylesheets returns the two kinds
	// interleaved in document order for that reason; see stylesheet.go for what
	// a linked one is allowed to be read from.
	for _, s := range documentStylesheets(doc, importer) {
		sheets = append(sheets, parseSheet(rec, style.OriginAuthor, s.name, s.base, s.source))
	}
	// A caller's own sheets go through the same expansion as the document's, so
	// that "@import" means the same thing whichever side it was written on.
	for i, s := range in.CSS {
		what := "the stylesheet " + quoteValue(s.Name)
		if s.Name == "" {
			what = "the stylesheet at Input.CSS[" + strconv.Itoa(i) + "]"
		}
		if !importer.admit(s.Source, what, NoSource, "") {
			continue
		}
		// Relative to the name the caller gave it, and not to the document's
		// <base>: a <base> is the document's word about its own references,
		// and a sheet the caller passed is not one of them.
		for _, e := range importer.expandImports(authorSheet{name: s.Name, base: s.Name, source: s.Source}) {
			sheets = append(sheets, parseSheet(rec, style.OriginAuthor, e.name, e.base, e.source))
		}
	}

	// Every sheet read the way the cascade reads it, once: the @media,
	// @supports and @layer blocks evaluated, and the @font-face and @page rules
	// they hold handed over. The fonts and the page are decided from those
	// before anything is styled — an ex in a font-size needs the faces, and
	// layout needs the page — and there is no second walk over the sheets to
	// disagree with this one about which blocks are live. See style.Prepared.
	prepared := style.Prepare(sheets, asked)

	fonts := in.Fonts
	if fonts == nil {
		fonts = StandardFonts()
	}
	fontSet := loadFontFaces(fontFacesOf(prepared.FontFaces), in.Resources, fonts, rec)

	// The sheet the document asked for, settled before it is styled. It
	// changes no media query's answer — see asked — but the page has to be
	// decided before layout either way, and deciding it here is what lets
	// Compose lay out on the sheet the document chose.
	page = applyPageRules(page, pagesOf(prepared.Pages), rec)

	// The page area is what a viewport-relative length is a percentage of on
	// paper, and it is the one layout resolves "3vw" against for every other
	// property (lengthContext); a font-size in vw is resolved in the cascade,
	// because it is inherited as a number, so the cascade is told the same page.
	area := page.Content()
	styled := prepared.ApplyOnPageWith(doc, fontMetrics{fontSet}, style.Media{Width: area.W, Height: area.H},
		base.inlineURLs(rec))
	for _, f := range styled.Findings {
		rec.ReportDetail(Finding{
			Rule:     ruleForStyleFinding(f),
			Source:   styleFindingSource(f),
			Message:  f.Message,
			Property: f.Property,
		})
	}
	if styled.Incomplete {
		rec.Report(RuleLimit, NoSource,
			"selector matching stopped early, so some rules did not apply")
	}

	root := buildBoxes(doc, styled, base, rec)
	// The one stage that reads anything from outside the two strings the caller
	// handed in. It runs after the box tree exists because whether an element
	// is replaced changes only how its box is sized, never whether there is
	// one — and it runs before layout because a size is what layout needs.
	resolveReplaced(root, in.Resources, base, rec)
	reportUnsupportedDisplays(doc, styled.Styles, styled.Pseudo, rec)

	return Built{
		Document:  doc,
		Root:      root,
		Page:      page,
		Styles:    styled.Styles,
		Fonts:     fontSet,
		Findings:  rec.Findings(),
		Failed:    rec.Failed(),
		Truncated: rec.Truncated(),
	}
}

// parseSheet reads one stylesheet and reports what it could not read.
//
// Its @font-face and @page rules stay in it: the cascade's walk hands them
// over, from wherever in the sheet they are live. See style.Prepared.
//
// Every url() in it is resolved against its base here, which is what a
// relative reference in a stylesheet is relative to: its own name, or the
// document's base URL for a <style> element. See resolveSheetURLs.
func parseSheet(rec *Recorder, origin style.Origin, name, base, src string) style.Sheet {
	p := readSheet(origin, name, src)
	resolveSheetURLs(p.rules, base, name, rec)
	return p.handOver(rec, origin, name)
}

// parsedSheet is everything reading one stylesheet produced, kept apart from
// the recorder and the lists it is reported into so that the reading can be
// done once and the reporting every time.
type parsedSheet struct {
	rules []css.Rule
	errs  []css.Error
}

// readSheet parses one stylesheet.
func readSheet(origin style.Origin, name, src string) parsedSheet {
	rules, errs := css.ParseStylesheet(src)
	return parsedSheet{rules: rules, errs: errs}
}

// handOver reports what the reading found and returns the sheet for the
// cascade. It is separate from readSheet so that a sheet read once is still
// reported once per document.
func (p parsedSheet) handOver(rec *Recorder, origin style.Origin, name string) style.Sheet {

	for _, e := range p.errs {
		rec.ReportDetail(Finding{
			Rule:    RuleInvalidCSS,
			Source:  Source{HTMLOffset: -1, CSSOffset: e.Offset, Sheet: name},
			Message: e.Message,
		})
	}
	return style.Sheet{Origin: origin, Rules: p.rules, Name: name}
}

// userAgentSheetName is what a finding about the default stylesheet points at.
const userAgentSheetName = "user agent"

// userAgentSheet is UserAgentCSS, parsed once for the process rather than once
// for every document.
//
// It is fourteen kilobytes of the same CSS every time, and it was tokenized,
// parsed and had its two hundred selectors read again for every single Build.
// For a document of any size that is amortised; for a small one it *is* the
// work. "<p>hello <b>world</b></p>" took 714 microseconds and allocated 1.4
// megabytes, nearly all of it this, and the layout package's fuzz corpora are
// tens of thousands of documents that size.
//
// What made it worth finding: adding two kilobytes of selectors to the sheet —
// the attribute rules of HTML's rendering section — cost thirty per cent of
// that Build and half a megabyte of allocation, which is not a proportion any
// stylesheet should cost a renderer. A default sheet that is re-read per
// document is a default sheet nobody can add to.
//
// The reading is memoized and the *reporting* is not: a finding about the
// default sheet is still raised into each document's recorder, and any
// @font-face or @page in it is handed over by each document's preparation — the
// style package's memo of this sheet replays them. So this is a
// memo of a pure function and not a change of behaviour — which is why it is
// written as readSheet and handOver rather than as a cached style.Sheet.
func userAgentSheet(rec *Recorder) style.Sheet {
	return parsedUserAgentCSS().handOver(rec, style.OriginUserAgent, userAgentSheetName)
}

var parsedUserAgentCSS = sync.OnceValue(func() parsedSheet {
	return readSheet(style.OriginUserAgent, userAgentSheetName, UserAgentCSS)
})

// styleFindingSource says where a styling finding happened, in the terms a
// caller points an author with.
//
// Three answers, and each of them was one before: an offset into a named
// stylesheet, an offset into the markup where the declaration was written in a
// style attribute, and nowhere at all for a finding about the styling as a
// whole. All three used to come out as "byte N of the stylesheet" with no name
// on it — which for a document with a <style>, three <link>s and their imports
// is an offset into one of five files and no way to tell which, and for the
// other two an offset into a file it is not an offset into.
func styleFindingSource(f style.Finding) Source {
	switch {
	case f.Offset < 0:
		return NoSource
	case f.InMarkup:
		return AtHTML(f.Offset)
	}
	return Source{HTMLOffset: -1, CSSOffset: f.Offset, Sheet: f.Sheet}
}

// ruleForStyleFinding maps the styling stage's report onto a rule.
//
// The stage knows three things about a finding — whether it is unsupported,
// whether it names a property, and whether that name is an at-rule — and those
// three answer which rule it is without the stage having to know the catalogue.
func ruleForStyleFinding(f style.Finding) Rule {
	if !f.Unsupported {
		return RuleInvalidCSS
	}
	switch {
	case len(f.Property) > 0 && f.Property[0] == '@':
		return RuleUnsupportedAtRule
	case f.Property != "":
		return RuleUnsupportedProperty
	}
	// Unsupported with no property named is a selector: the styling stage
	// reports those straight from the selector parser.
	return RuleUnsupportedSelector
}

// reportUnsupportedDisplays names the display and position values the box tree
// recognised and could not honour.
//
// "display: contents" is the one that matters, and what is left of it is the
// elements it cannot be honoured on. An element whose layout is not decided by
// CSS box generation — a replaced element, a form control — has no contents to
// be replaced by, and the root is blockified by §2.7 before the value is
// reached. Those keep the box they had, and the box they had is an inline one,
// which takes part in layout when the author asked for it not to.
//
// Which elements those are is contentsIsHonoured's answer and not a second copy
// of it. A guardrail that decided for itself which declarations the engine
// applies would go stale in the direction that matters: silent about a value
// that had stopped being honoured.
//
// The display gaps of a ::before and an ::after are reported beside their
// element's, from the same reading of the value: a pseudo-element is a box
// like any other, and one whose display was laid out as something else went
// unsaid because this walk only met elements. Neither is reported under a
// "display: none", which lays nothing out to be wrong about.
func reportUnsupportedDisplays(doc *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle, rec *Recorder) {

	root := documentElementOf(doc)
	scope := rubyScope{styles: styles}
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		cs, ok := styles[n]
		if !ok {
			return true
		}
		if displayIsNone(cs) {
			return false
		}
		if cs.Get("display") == "contents" && !contentsIsHonoured(n, cs, root) {
			rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedValue,
				Source:   AtHTML(n.Offset),
				Message:  "\"display: contents\" is not implemented; the element was laid out as an inline box",
				Path:     PathOf(n),
				Property: "display",
			})
		}
		if gap := parseDisplay(cs.Get("display")).gap; gap != displayGapNone &&
			unlaidBoxIsNotTheBoxAsked(n, cs, styles, pseudo, gap, &scope, n == root) {
			value := ascii.Lower(ascii.TrimCSSSpace(cs.Get("display")))
			rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: quoteValue("display: "+value) + " is not implemented; " +
					unlaidDisplay(gap),
				Path:     PathOf(n),
				Property: "display",
			})
		}
		for _, name := range [2]string{"before", "after"} {
			pcs, ok := pseudo[style.PseudoKey{Node: n, Name: name}]
			if !ok || !generatesPseudoBox(pcs) {
				continue
			}
			gap := parseDisplay(pcs.Get("display")).gap
			if gap == displayGapNone || !pseudoIsNotTheBoxAsked(n, cs, pcs, gap, &scope) {
				continue
			}
			value := ascii.Lower(ascii.TrimCSSSpace(pcs.Get("display")))
			rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: quoteValue("display: "+value) + " on ::" + name +
					" is not implemented; " + unlaidDisplay(gap),
				Path:     PathOf(n),
				Property: "display",
			})
		}
		// The legacy flexible box. This engine implements exactly the part CSS
		// Overflow 4's compatibility section needs — a block that
		// "-webkit-line-clamp" can be written on — and the old flexbox layout
		// it otherwise asks for is not implemented at all.
		//
		// Read as a block, which is what every engine does for the vertical,
		// single-column case the clamp is used in. Under the *horizontal*
		// orient it is a row in a browser and a stack of blocks here, and that
		// went unsaid: a navigation bar written the old way came out as one
		// item per line with nothing to show which of the two the page was.
		if ascii.EqualFold(ascii.TrimCSSSpace(cs.Get("display")), "-webkit-box") &&
			!ascii.EqualFold(ascii.TrimCSSSpace(cs.Get("-webkit-box-orient")), "vertical") {
			rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: "\"display: -webkit-box\" lays its children out in a row here " +
					"only under \"-webkit-box-orient: vertical\"; the old flexible box is " +
					"not implemented, so the element was laid out as a block",
				Path:     PathOf(n),
				Property: "display",
			})
		}
		// "position: sticky" is the one positioning scheme this engine cannot
		// answer, and it is the one that proves the scope boundary is about
		// dynamism rather than about difficulty: sticky is defined by where a
		// scroll container has been scrolled to, and a page does not scroll. It
		// falls back to static, which is where the box would sit before any
		// scrolling had happened — the right half of the answer, and silent
		// about the other half unless this says so.
		if ascii.EqualFold(ascii.TrimCSSSpace(cs.Get("position")), "sticky") {
			rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedValue,
				Source: AtHTML(n.Offset),
				Message: "\"position: sticky\" is defined by a scroll position, which a " +
					"page does not have; the element was laid out in the normal flow",
				Path:     PathOf(n),
				Property: "position",
			})
		}
		return true
	})
}

// unlaidDisplay says what a box was laid out as, for the part of its display
// value this engine does not lay out as asked. parseDisplay decides
// which part that is, from the same reading of the value that built the box, so
// the report cannot disagree with the layout about what was asked.
//
// The omission has one shape each time: the value is recognised, the box is
// built, and ordinary layout runs where something else was asked for — and
// until this report existed it said nothing at all, which is the plausible,
// silent wrongness the whole findings vocabulary is against. See
// style/unimplemented.go, which makes the same argument about a property
// nothing reads.
//
// "flex" was here and is not any more, and "grid" has gone the same way:
// layout/flex.go and layout/grid.go arrange the containers they can and report
// the ones they cannot, at the box, with the reason. A value that is laid out
// has nothing to say here, and one whose *arrangement* is refused is a fact
// about the container rather than about the keyword.
func unlaidDisplay(gap displayGap) string {
	switch gap {
	case displayGapRuby:
		return "the box was laid out as an inline box, so the annotation runs " +
			"along the line instead of above it"
	case displayGapRunIn:
		return "the box was laid out as an inline box and not run into the " +
			"block after it"
	case displayGapInlineListItem:
		return "an inline-level list item was laid out as the inline box it is, " +
			"and its marker was not drawn"
	case displayGapAnnotation:
		return "an annotation outside any ruby was laid out as an inline box, " +
			"where it belongs above an empty base in a ruby of its own"
	}
	return ""
}

// unlaidBoxIsNotTheBoxAsked reports whether laying the box out as this engine
// does actually produces a different page.
//
// Reporting every such box would be crying wolf, and the suite said so in six
// documents: five of them write "display: ruby" on a span with no annotation in
// it, to make an element boundary for a text rule to be asked about, and the
// sixth wrote an *empty* "display: inline-flex" as an atomic inline for
// letter-spacing to space. In each of them the box this engine builds is the box
// the specification asks for, and a finding would say the page was wrong when it
// was right. That is the argument style/inert.go makes for a declaration asking
// for the behaviour that is already there.
//
// Only the five are left, because the boxes that were counted by content are
// laid out now and answer for themselves. A ruby box is different where there
// is an annotation in it: ruby lays a "ruby-text" above its base, and with no
// annotation there is nothing to lift — §3.1's own answer for a base alone is
// the base.
//
// An annotation is the ruby's report where there is a ruby around it, and its
// own where there is none: css-ruby-1 §2.2 wraps one that stands outside any
// ruby in an anonymous ruby of its own, above an empty base. Not where it has
// been blockified — a float, an absolutely positioned box, a flex or grid item
// or the root — because css-display-3 §2.7 turns a blockified layout-internal
// box into a block, which is what this engine lays out.
func unlaidBoxIsNotTheBoxAsked(n *html.Node, cs style.ComputedStyle,
	styles map[*html.Node]style.ComputedStyle, pseudo map[style.PseudoKey]style.ComputedStyle,
	gap displayGap, scope *rubyScope, isRoot bool) bool {

	switch gap {
	case displayGapRuby:
		return hasRubyAnnotation(n, styles, pseudo)
	case displayGapAnnotation:
		return !isRoot && !blockifiedOnItsOwn(cs) &&
			!itemOfItsParent(n.Parent, styles) && !scope.inside(n.Parent)
	}
	return true
}

// pseudoIsNotTheBoxAsked is unlaidBoxIsNotTheBoxAsked for a ::before or an
// ::after of n, whose style is cs, with the style pcs.
//
// A ruby pseudo-element holds no element, so no annotation, and is the base
// it lays out as. An annotation pseudo-element is inside a ruby when its
// element is one or is inside one.
func pseudoIsNotTheBoxAsked(n *html.Node, cs, pcs style.ComputedStyle, gap displayGap,
	scope *rubyScope) bool {

	switch gap {
	case displayGapRuby:
		return false
	case displayGapAnnotation:
		return !blockifiedOnItsOwn(pcs) && !isFlexOrGridContainer(cs) &&
			!scope.inside(n)
	}
	return true
}

// blockifiedOnItsOwn reports the two reasons §2.7 blockifies a box that are
// its own style's: a float and an absolutely positioned box.
func blockifiedOnItsOwn(cs style.ComputedStyle) bool {
	return floatOf(cs) != FloatNone || positionOf(cs).outOfFlow()
}

// itemOfItsParent reports whether an element's box is an item of a flex or
// grid container: whether the nearest ancestor that generates a box is one.
func itemOfItsParent(up *html.Node, styles map[*html.Node]style.ComputedStyle) bool {
	for ; up != nil && up.Type == html.ElementNode; up = up.Parent {
		cs := styles[up]
		if ascii.EqualFold(ascii.TrimCSSSpace(cs.Get("display")), "contents") {
			continue
		}
		return isFlexOrGridContainer(cs)
	}
	return false
}

// isFlexOrGridContainer reports a style whose inner display is flex or grid.
func isFlexOrGridContainer(cs style.ComputedStyle) bool {
	d := parseDisplay(cs.Get("display"))
	return d.inner == InnerFlex || d.inner == InnerGrid
}

// rubyScope answers whether an element is a ruby or inside one, memoized per
// element: an annotation asks of its parent, and a page of annotations each a
// hundred elements deep would otherwise walk the hundred for every one.
type rubyScope struct {
	styles map[*html.Node]style.ComputedStyle
	memo   map[*html.Node]bool
}

// inside reports whether n or an element above it is a ruby.
func (s *rubyScope) inside(n *html.Node) bool {
	var path []*html.Node
	found := false
	for cur := n; cur != nil && cur.Type == html.ElementNode; cur = cur.Parent {
		if v, ok := s.memo[cur]; ok {
			found = v
			break
		}
		path = append(path, cur)
		if parseDisplay(s.styles[cur].Get("display")).gap == displayGapRuby {
			found = true
			break
		}
	}
	if s.memo == nil {
		s.memo = map[*html.Node]bool{}
	}
	for _, c := range path {
		s.memo[c] = found
	}
	return found
}

// hasRubyAnnotation reports whether a ruby box has anything to lift above its
// base, anywhere inside it.
//
// Anywhere, because the annotation need not be a child: HTML's own <ruby> puts
// the <rt> beside the base, and a document may wrap either in a span. What it
// must not do is look through a *nested* ruby, whose annotation belongs to that
// one, and it does not: the walk stops at one. The nested ruby is asked about
// itself, and is reported if its annotation is there.
//
// Stopping there is also what makes the question cheap. It is asked of every
// ruby, and a walk that went on through the rubies inside visited each element
// once for every ruby above it: 250 nested rubies over a hundred thousand
// elements was seventeen seconds of this (audit C134). Stopped, each element is
// visited by the walk of the one ruby nearest above it.
//
// The walk takes in n itself, and nothing guards against it: the caller has
// already read n's display and found "ruby", so n cannot also be the
// "ruby-text" this is looking for. A guard that said so could not be made to
// fail.
//
// A ::before or an ::after of an element in the walk is an annotation too when
// its display says so and it generates a box, which is the one kind of
// annotation the elements alone do not show.
func hasRubyAnnotation(n *html.Node, styles map[*html.Node]style.ComputedStyle,
	pseudo map[style.PseudoKey]style.ComputedStyle) bool {
	found := false
	n.Walk(func(c *html.Node) bool {
		// The element test is an optimisation and nothing else, and it is worth
		// saying so because no test can catch it: a text node is not in styles,
		// so the lookup below would answer "no display" for one anyway. What it
		// saves is a map lookup per character of a ruby's text. See
		// style/computed.go's mightHoldAFontRelativeLength, which is the same
		// kind of check with the same kind of note on it.
		if found || c.Type != html.ElementNode {
			return !found
		}
		cs, ok := styles[c]
		if !ok {
			return true
		}
		if displayIsNone(cs) {
			// Nothing under it is laid out, so nothing is lifted or not.
			return false
		}
		// The value as parseDisplay reads it, so that "inline ruby" and
		// "block ruby" are rubies here as they are everywhere else: matched
		// as the one word "ruby", a nested "block ruby" was looked through and
		// its annotation reported against the ruby around it as well.
		switch parseDisplay(cs.Get("display")).gap {
		case displayGapAnnotation:
			found = true
			return false
		case displayGapRuby:
			if c != n {
				// A nested ruby, whose annotation is its own. See above.
				return false
			}
		}
		for _, name := range [2]string{"before", "after"} {
			pcs, ok := pseudo[style.PseudoKey{Node: c, Name: name}]
			if ok && generatesPseudoBox(pcs) &&
				parseDisplay(pcs.Get("display")).gap == displayGapAnnotation {
				found = true
			}
		}
		return !found
	})
	return found
}

// PathOf renders an element's position in the document, so a finding about a
// stylesheet shared by many elements can still say which one it is about.
//
// It is a readable path rather than a selector that would round-trip: it names
// the element chain with the identifiers and classes that distinguish it, which
// is what someone reading a report needs.
//
// It is bounded, the way quoteValue bounds a value, and for the reason it
// does: every part of it is the document's. Unbounded, a path was as long as
// the document made its ids and as deep as it nested them — 250 ancestors with
// two-thousand-character ids made a half-megabyte path for every finding
// beneath them, and a finding carries one (audit C18). So a part is cut at
// pathPartBytes, and a path deeper than pathDepth keeps the elements a reader
// finds it by — the outermost few, which say where in the page, and the
// innermost, which say which element — with an ellipsis for those between.
func PathOf(n *html.Node) string {
	if n == nil || n.Type != html.ElementNode {
		return ""
	}
	// Innermost first, as the walk meets them, and only as many as are kept:
	// the innermost pathInner, and then a count of the rest, of which only the
	// outermost pathOuter are named once the walk reaches the top.
	var inner []string
	var chain []*html.Node
	for cur := n; cur != nil && cur.Type == html.ElementNode; cur = cur.Parent {
		if len(inner) < pathInner {
			inner = append(inner, pathPart(cur))
			continue
		}
		chain = append(chain, cur)
	}
	parts := make([]string, 0, pathOuter+1+len(inner))
	// chain holds the elements above the innermost few, innermost first; the
	// outermost pathOuter of them are its last entries.
	elided := len(chain) > pathOuter
	for i := len(chain) - 1; i >= 0 && i >= len(chain)-pathOuter; i-- {
		parts = append(parts, pathPart(chain[i]))
	}
	if elided {
		parts = append(parts, "…"+strconv.Itoa(len(chain)-pathOuter)+" more…")
	}
	for i := len(inner) - 1; i >= 0; i-- {
		parts = append(parts, inner[i])
	}
	return strings.Join(parts, " > ")
}

// The bounds on PathOf. Sixteen levels named is more than a reader follows, and
// sixty-four bytes is longer than any identifier a person chose.
const (
	pathOuter     = 4
	pathInner     = 12
	pathPartBytes = 64
)

// pathPart is one element's step in a path: its name, and the id or first
// class that distinguishes it, cut at pathPartBytes on a character boundary.
//
// Nothing longer than the cut is read. The attribute is the document's, and a
// path is asked for once per finding: reading a megabyte of class to keep
// sixty-four bytes of it is the cost the bound is there to remove, paid anyway.
func pathPart(n *html.Node) string {
	part := n.Name
	if id, ok := n.Attr("id"); ok && id != "" {
		part += "#" + id[:cutAt(id, pathPartBytes+1)]
	} else if class, ok := n.Attr("class"); ok {
		// The first class, found within a window: a class list that opens with
		// more white space than that names nothing a reader would recognise.
		start := 0
		for start < len(class) && start < pathPartBytes && ascii.IsSpace(class[start]) {
			start++
		}
		end := start
		for end < len(class) && end-start <= pathPartBytes && !ascii.IsSpace(class[end]) {
			end++
		}
		if end > start {
			part += "." + class[start:start+cutAt(class[start:end], pathPartBytes+1)]
		}
	}
	if len(part) <= pathPartBytes {
		return part
	}
	return part[:cutAt(part, pathPartBytes)] + "…"
}

// cutAt is the length of s's longest prefix of at most n bytes that ends on a
// character boundary.
func cutAt(s string, n int) int {
	if len(s) <= n {
		return len(s)
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return n
}
