package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
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
// This is not the final entry point. Render will be, once there is a layout to
// run and a page to run it onto; what is here is the prefix of it that exists,
// exposed because the stages are worth testing against real documents rather
// than against hand-built trees.

// Stylesheet is one stylesheet with a name to report against.
type Stylesheet struct {
	// Name identifies the sheet in a finding — a filename, usually. It is empty
	// for the document's own <style> content.
	Name string
	// Source is the CSS.
	Source string
}

// Input is a document and the stylesheets to apply to it.
type Input struct {
	// HTML is the document source.
	HTML string
	// CSS is the author's stylesheets, in the order they apply.
	CSS []Stylesheet
	// Policy chooses what each rule does. A nil policy uses the defaults.
	Policy Policy
	// UserCSS is a stylesheet applied on the reader's behalf, which sits
	// between the engine's defaults and the author's. It is separate from CSS
	// because its origin is different, and origin is the strongest term in the
	// cascade.
	UserCSS string

	// Resources supplies the bytes of the files the document refers to — the
	// images an <img> or a background-image names, and the stylesheets a
	// <link rel=stylesheet> does.
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
	rec := NewRecorder(in.Policy)

	doc, htmlErrs, _ := html.Parse(in.HTML)
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

	// The document's @font-face rules, collected as the sheets are parsed and
	// loaded once all of them are in. They are gathered rather than acted on
	// here because a rule in the last stylesheet may replace one in the first,
	// and because the caps below are on the document rather than on a sheet.
	var faces []pendingFontFace
	// The document's @page rules, gathered the same way and for the same
	// reason: which of two declarations of a margin wins depends on the origin
	// of the sheet each was written in, so all of them have to be in hand
	// before any of them is read.
	var pages []pendingPage

	sheets := make([]style.Sheet, 0, len(in.CSS)+2)
	sheets = append(sheets, parseSheet(rec, style.OriginUserAgent, "user agent", UserAgentCSS, &faces, &pages))
	if in.UserCSS != "" {
		sheets = append(sheets, parseSheet(rec, style.OriginUser, "user", in.UserCSS, &faces, &pages))
	}
	// A <style> element and a <link rel=stylesheet> are both author stylesheets,
	// and they come before the ones the caller passed only because they were
	// written first — order is the cascade's last tie-break and it has to be the
	// order the author would expect. documentStylesheets returns the two kinds
	// interleaved in document order for that reason; see stylesheet.go for what
	// a linked one is allowed to be read from.
	for _, s := range documentStylesheets(doc, in.Resources, rec) {
		sheets = append(sheets, parseSheet(rec, style.OriginAuthor, s.name, s.source, &faces, &pages))
	}
	// A caller's own sheets go through the same expansion as the document's, so
	// that "@import" means the same thing whichever side it was written on.
	importer := &sheetLoader{res: in.Resources, rec: rec, failed: map[string]bool{}}
	for _, s := range in.CSS {
		for _, e := range importer.expandImports(authorSheet{name: s.Name, source: s.Source}) {
			sheets = append(sheets, parseSheet(rec, style.OriginAuthor, e.name, e.source, &faces, &pages))
		}
	}

	base := in.Fonts
	if base == nil {
		base = StandardFonts()
	}
	fontSet := loadFontFaces(faces, in.Resources, base, rec)

	// The sheet the document asked for, settled before it is styled. A margin
	// does not change what a media query is answered with — a query asks about
	// the paper and the margin is inside it — but the page has to be decided
	// before layout either way, and deciding it here is what lets Compose lay
	// out on the sheet the document chose.
	page = applyPageRules(page, pages, rec)

	styled := style.ApplyIn(doc, sheets, fontMetrics{fontSet},
		style.Media{Width: page.Width, Height: page.Height})
	for _, f := range styled.Findings {
		rec.ReportDetail(Finding{
			Rule:     ruleForStyleFinding(f),
			Source:   AtCSS(f.Offset),
			Message:  f.Message,
			Property: f.Property,
		})
	}
	if styled.Incomplete {
		rec.Report(RuleLimit, NoSource,
			"selector matching stopped early, so some rules did not apply")
	}

	root := BuildBoxes(doc, styled, rec)
	// The one stage that reads anything from outside the two strings the caller
	// handed in. It runs after the box tree exists because whether an element
	// is replaced changes only how its box is sized, never whether there is
	// one — and it runs before layout because a size is what layout needs.
	resolveReplaced(root, in.Resources, rec)
	reportUnsupportedDisplays(doc, styled.Styles, rec)

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

// parseSheet reads one stylesheet, reporting what it could not read and setting
// aside the @font-face rules in it.
//
// The rules are taken out here rather than in the cascade because they are not
// a cascade matter at all: an @font-face selects nothing and computes nothing,
// it loads a file. Leaving them in would mean the styling stage reporting each
// as an at-rule it does not apply, which after fontface.go would be untrue.
func parseSheet(rec *Recorder, origin style.Origin, name, src string,
	faces *[]pendingFontFace, pages *[]pendingPage) style.Sheet {
	rules, errs := css.ParseStylesheet(src)
	for _, e := range errs {
		rec.ReportDetail(Finding{
			Rule:    RuleInvalidCSS,
			Source:  Source{HTMLOffset: -1, CSSOffset: e.Offset, Sheet: name},
			Message: e.Message,
		})
	}
	rules = splitFontFaces(rules, name, faces)
	collectPageRules(rules, name, origin, nil, pages)
	return style.Sheet{Origin: origin, Rules: rules}
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
func reportUnsupportedDisplays(doc *html.Node, styles map[*html.Node]style.ComputedStyle, rec *Recorder) {
	root := documentElementOf(doc)
	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		cs, ok := styles[n]
		if !ok {
			return true
		}
		if cs["display"] == "contents" && !contentsIsHonoured(n, cs, root) {
			rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedValue,
				Source:   AtHTML(n.Offset),
				Message:  "\"display: contents\" is not implemented; the element was laid out as an inline box",
				Path:     PathOf(n),
				Property: "display",
			})
		}
		if what, laid := unlaidFormattingContext(cs["display"]); what != "" &&
			unlaidBoxIsNotTheBoxAsked(n, styles, what) {
			rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedValue,
				Source:   AtHTML(n.Offset),
				Message:  "\"display: " + what + "\" is not implemented; " + laid,
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
		if strings.EqualFold(strings.TrimSpace(cs["position"]), "sticky") {
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

// unlaidFormattingContext names a display value whose *inner* layout this engine
// does not do, and says what the box was laid out as instead.
//
// The two are one omission with one shape: the value is recognised, the box is
// built, and then ordinary layout runs inside it — a grid becomes a column of
// full-width blocks where a table of tracks was asked for, and until this
// report existed it said nothing at all, which is the plausible, silent
// wrongness the whole findings vocabulary is against. See
// style/unimplemented.go, which makes the same argument about a property
// nothing reads.
//
// "flex" was here and is not any more, and "grid" has gone the same way:
// layout/flex.go and layout/grid.go arrange the containers they can and report
// the ones they cannot, at the box, with the reason. A value that is laid out
// has nothing to say here, and one whose *arrangement* is refused is a fact
// about the container rather than about the keyword.
//
// They are named rather than gathered by exclusion. A list of "everything this
// engine does not lay out" would go stale in the direction that matters: silent
// about a value that had stopped being laid out.
func unlaidFormattingContext(value string) (what, laid string) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ruby":
		return "ruby", "the box was laid out as an inline box, so the " +
			"annotation runs along the line instead of above it"
	}
	return "", ""
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
func unlaidBoxIsNotTheBoxAsked(n *html.Node, styles map[*html.Node]style.ComputedStyle,
	what string) bool {

	return what == "ruby" && hasRubyAnnotation(n, styles)
}

// hasRubyAnnotation reports whether a ruby box has anything to lift above its
// base, anywhere inside it.
//
// Anywhere, because the annotation need not be a child: HTML's own <ruby> puts
// the <rt> beside the base, and a document may wrap either in a span. What it
// must not do is look through a *nested* ruby, whose annotation belongs to that
// one — but a nested ruby is itself reported, so the outer one saying so as well
// is not a second finding about the same box.
//
// The walk takes in n itself, and nothing guards against it: the caller has
// already read n's display and found "ruby", so n cannot also be the
// "ruby-text" this is looking for. A guard that said so could not be made to
// fail.
func hasRubyAnnotation(n *html.Node, styles map[*html.Node]style.ComputedStyle) bool {
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
		switch strings.ToLower(strings.TrimSpace(cs["display"])) {
		case "ruby-text", "ruby-text-container":
			found = true
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
func PathOf(n *html.Node) string {
	if n == nil || n.Type != html.ElementNode {
		return ""
	}
	var parts []string
	for cur := n; cur != nil && cur.Type == html.ElementNode; cur = cur.Parent {
		part := cur.Name
		if id, ok := cur.Attr("id"); ok && id != "" {
			part += "#" + id
		} else if class, ok := cur.Attr("class"); ok {
			if fields := strings.Fields(class); len(fields) > 0 {
				part += "." + fields[0]
			}
		}
		parts = append(parts, part)
	}
	// Built innermost first; a path reads outermost first.
	for i, j := 0, len(parts)-1; i < j; i, j = i+1, j-1 {
		parts[i], parts[j] = parts[j], parts[i]
	}
	return strings.Join(parts, " > ")
}
