package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/style"
)

// The sheet the document asks for.
//
// @page is the one rule in a stylesheet that is not about the content: it
// describes the paper. CSS 2.1 §13.2 gives the sheet a box of its own — the
// page box, whose margin area is the blank the printed part sits inside — and
// what this file reads is that margin.
//
// # Why it is not in the cascade
//
// An @page rule selects no element, computes no value on one and inherits
// nothing. What it does is change the surface every element is then laid out
// on, and that has to be settled before layout begins rather than during it. So
// the rules are taken out of the stylesheet in pipeline.go, exactly where
// @font-face is and for the same reason: leaving them in would have the cascade
// report each as an at-rule it does not apply, which after this file is untrue.
//
// # What the caller still decides
//
// The sheet the caller passed in Options is where this starts. A document that
// says nothing about its margins is printed with the caller's, and one that
// does is printed with its own, on the caller's paper. That is the useful order
// for an engine whose job is to print somebody else's HTML: the caller chose
// the paper and the document knows its own design.
//
// The size of the sheet is not read here yet, and neither is a rule that
// selects only some pages — a document is one page in this engine, so ":first"
// is a distinction it does not have. Both are reported rather than ignored.

// pendingPage is an @page rule with what deciding between two of them needs:
// the stylesheet it was written in, so a finding can say where it came from,
// the origin of that sheet, because origin is the strongest term in the cascade
// and it is no weaker here, and the media queries it was written inside.
type pendingPage struct {
	rule   css.Rule
	sheet  string
	origin style.Origin
	// media is the prelude of every @media block enclosing the rule, outermost
	// first. All of them have to match for the rule to apply, which is what
	// nesting them means. It is carried rather than evaluated where it was
	// found because the answer depends on the sheet, and the sheet is not
	// settled until every stylesheet has been read.
	media [][]css.ComponentValue
}

// at is the place a finding about one rule points to.
func (p pendingPage) at() Source {
	return Source{HTMLOffset: -1, CSSOffset: p.rule.Offset, Sheet: p.sheet}
}

// collectPageRules gathers the @page rules of one stylesheet, including the
// ones written inside a media query.
//
// They are left in the stylesheet rather than taken out of it. The cascade
// skips an @page of its own accord — it selects no element and computes no
// value on one, so there is nothing there for it to do — and that is what makes
// this able to reach the ones inside an @media block, which are component
// values in the enclosing rule rather than rules in the list this walks.
//
// A print stylesheet is where those are: "@media print { @page { margin: 0 } }"
// is how a page rule is written by anyone whose document is also read on a
// screen, and reading only the top-level ones would miss most of the @page
// rules that exist.
func collectPageRules(rules []css.Rule, sheet string, origin style.Origin,
	media [][]css.ComponentValue, pages *[]pendingPage) {

	for _, r := range rules {
		switch {
		case isPageRule(r):
			*pages = append(*pages, pendingPage{
				rule: r, sheet: sheet, origin: origin, media: media})
		case r.At && strings.EqualFold(r.Name, "media") && r.HasBlock:
			// Only @media is descended into. An @page written inside anything
			// else — a style rule, @supports, a nested rule — is not a page
			// rule this engine has a way to decide, and the cascade reports the
			// enclosing at-rule as one it does not apply.
			inner, _ := css.ParseRulesFromValues(r.Block)
			// A fresh slice rather than an append to this one: two @media
			// blocks at the same depth would otherwise append into the same
			// spare capacity, and the second would overwrite the query the
			// first had already handed to a rule it enclosed.
			within := make([][]css.ComponentValue, len(media)+1)
			copy(within, media)
			within[len(media)] = r.Prelude
			collectPageRules(inner, sheet, origin, within, pages)
		}
	}
}

func isPageRule(r css.Rule) bool {
	return r.At && strings.EqualFold(r.Name, "page")
}

// pageDeclarations is what a document's @page rules said, before any of it is
// applied. They are gathered rather than applied as they are read because the
// descriptors are not independent: a margin may be a percentage of the size,
// and the size is not settled until the last rule has been seen.
type pageDeclarations struct {
	sides [4]pageDeclaration
	size  pageSizeDeclaration
}

// pageSizeDeclaration is the size descriptor as one rule declared it, already
// resolved to a sheet. It is one value and not two — "size" declares the pair,
// so a later rule saying "landscape" replaces the width and the height together
// rather than turning what an earlier one chose.
type pageSizeDeclaration struct {
	width, height style.Unit
	rank          int
	order         int
	set           bool
}

func takeSize(held *pageSizeDeclaration, d pageSizeDeclaration) {
	if !held.set || d.rank > held.rank || (d.rank == held.rank && d.order > held.order) {
		*held = d
	}
}

// pageDeclaration is one side's margin as one rule declared it, with what
// deciding against another declaration of the same side needs.
type pageDeclaration struct {
	length style.Length
	// rank is the importance-and-origin term of CSS Cascade 4 §6, and order is
	// where the declaration was written. There is no specificity term between
	// them: every rule read here selects every page, so all of them are equally
	// specific and appearance is the only tie-break left.
	rank  int
	order int
	set   bool
}

// beats reports whether this declaration wins over one already held.
func (d pageDeclaration) beats(o pageDeclaration) bool {
	if !o.set {
		return true
	}
	if d.rank != o.rank {
		return d.rank > o.rank
	}
	return d.order > o.order
}

// applyPageRules returns the sheet the document is laid out on: the one the
// caller asked for, with what the document's @page rules said applied over it.
//
// A side no rule declared keeps the caller's margin rather than becoming zero.
// The two are not the same and the difference is the whole of this engine's
// contract about the page: a stylesheet that only says "@page { margin-top:
// 3cm }" has said nothing about the other three, and printing them at nothing
// would take a document off the edge of the paper on the strength of a rule
// that never mentioned it.
func applyPageRules(page PageSize, pages []pendingPage, rec *Recorder) PageSize {
	if len(pages) == 0 {
		return page
	}
	// The queries are answered about the sheet the caller asked for, because
	// the answer is part of deciding what the sheet becomes. A print
	// stylesheet's "@media print" is the case that matters and asks nothing
	// about the paper's size; one that does ask — "@media (min-width: 200mm)
	// { @page { size: A3 } }" — is answered about the page before the rule
	// inside it could change it, which is the only order that terminates.
	// Everything *else* in the document is then styled against the sheet this
	// chose, which is the answer an author means.
	asked := style.Media{Width: page.Width, Height: page.Height}

	var got pageDeclarations
	order := 0
	for _, p := range pages {
		if !pageRuleApplies(p, asked, rec) {
			continue
		}
		readPageRule(p, page, &got, &order, rec)
	}

	// The size first, because a margin may be a percentage of it. A rule
	// saying "size: A5; margin: 10%" means a tenth of the A5 it just asked
	// for, not a tenth of the sheet the caller happened to pass in — the two
	// descriptors are one statement about one page and reading them in the
	// order they happen to be written would make the second depend on it.
	if got.size.set {
		page.Width, page.Height = got.size.width, got.size.height
	}

	// A percentage is of the page box, which is the whole sheet: the left and
	// right margins against its width and the top and bottom against its
	// height. Every one of them is definite — this is paper, and its size is
	// known before anything is laid out on it — so the second return is not a
	// case that can arise, and a length that could not resolve was reported
	// where it was read rather than silently becoming nought here.
	basis := [4]style.Unit{page.Height, page.Width, page.Height, page.Width}
	out := [4]style.Unit{page.Margin.Top, page.Margin.Right, page.Margin.Bottom, page.Margin.Left}
	for i, d := range got.sides {
		if !d.set {
			continue
		}
		if u, ok := d.length.Resolve(basis[i], true); ok {
			out[i] = u
		}
	}
	page.Margin = Edges{Top: out[sideTop], Right: out[sideRight], Bottom: out[sideBottom], Left: out[sideLeft]}
	return page
}

// pageRuleApplies answers the media queries an @page rule was written inside.
//
// A query that does not match drops the rule, and that is not a failure to
// report: the stylesheet said it was for another medium and this is not it. A
// query naming something this engine cannot answer is reported for the reason
// the cascade reports one — a browser printing the same document may know the
// feature, and its page would differ from this one.
func pageRuleApplies(p pendingPage, asked style.Media, rec *Recorder) bool {
	applies := true
	for _, query := range p.media {
		matches, unknown := style.MatchesMedia(query, asked)
		if unknown != "" {
			rec.ReportDetail(Finding{
				Rule:   RuleUnsupportedAtRule,
				Source: p.at(),
				Message: "the media query " + quoteValue(strings.TrimSpace(pageText(query))) +
					" around an @page rule asks about " + quoteValue(unknown) +
					", which this engine cannot answer, so the rule was not applied",
				Property: "@media",
			})
		}
		if !matches {
			applies = false
		}
	}
	return applies
}

// readPageRule reads the descriptors of one @page rule into the set.
func readPageRule(p pendingPage, base PageSize, got *pageDeclarations, order *int, rec *Recorder) {
	if len(nonWhitespace(p.rule.Prelude)) > 0 {
		// ":first", ":left", ":right" and a named page all select some pages
		// and not others. This engine composes one page, so there is no
		// sequence for them to pick out of and no honest way to decide whether
		// the rule applies to it.
		rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedAtRule,
			Source:   p.at(),
			Message:  "@page " + quoteValue(strings.TrimSpace(pageText(p.rule.Prelude))) + " selects some pages rather than all of them, which this engine does not do; the rule was not applied",
			Property: "@page",
		})
		return
	}
	if !p.rule.HasBlock {
		rec.ReportDetail(Finding{
			Rule:     RuleInvalidCSS,
			Source:   p.at(),
			Message:  "@page has no block, so it says nothing about the page",
			Property: "@page",
		})
		return
	}

	decls, _, errs := css.ParseDeclarationValues(p.rule.Block)
	for _, e := range errs {
		rec.ReportDetail(Finding{
			Rule:    RuleInvalidCSS,
			Source:  Source{HTMLOffset: -1, CSSOffset: e.Offset, Sheet: p.sheet},
			Message: e.Message,
		})
	}

	for _, d := range decls {
		*order++
		rank := style.CascadeRank(p.origin, d.Important)
		switch strings.ToLower(d.Name) {
		case "margin":
			if spread, ok := pageMarginShorthand(d.Value); ok {
				for i, l := range spread {
					take(&got.sides[i], pageDeclaration{length: l, rank: rank, order: *order, set: true})
				}
			} else {
				badPageMargin(rec, p, d)
			}
		case "margin-top", "margin-right", "margin-bottom", "margin-left":
			// border.go's side, in the order the shorthand writes them, which
			// is why the loop above can spread four values over it by index.
			at := map[string]side{
				"margin-top": sideTop, "margin-right": sideRight,
				"margin-bottom": sideBottom, "margin-left": sideLeft,
			}[strings.ToLower(d.Name)]
			if l, ok := pageMarginValue(d.Value); ok {
				take(&got.sides[at], pageDeclaration{length: l, rank: rank, order: *order, set: true})
			} else {
				badPageMargin(rec, p, d)
			}
		case "size":
			if w, h, ok := pageSizeValue(d.Value, base); ok {
				takeSize(&got.size, pageSizeDeclaration{
					width: w, height: h, rank: rank, order: *order, set: true})
			} else {
				badPageSize(rec, p, d)
			}
		default:
			// Everything else an @page block can hold changes the page in a way
			// this engine does not make: "marks" and "bleed" are for a press,
			// and a property like "background" paints the sheet rather than the
			// document on it. Passing over one silently would print a page the
			// author did not ask for with nothing saying so.
			rec.ReportDetail(Finding{
				Rule:     RuleUnsupportedProperty,
				Source:   Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
				Message:  "the @page descriptor " + quoteValue(strings.ToLower(d.Name)) + " is not applied",
				Property: strings.ToLower(d.Name),
			})
		}
	}
}

// take keeps whichever of two declarations for the same side the cascade
// prefers.
func take(held *pageDeclaration, d pageDeclaration) {
	if d.beats(*held) {
		*held = d
	}
}

func badPageSize(rec *Recorder, p pendingPage, d css.Declaration) {
	rec.ReportDetail(Finding{
		Rule:   RuleInvalidCSS,
		Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
		Message: "the @page size " + quoteValue(strings.TrimSpace(pageText(d.Value))) +
			" is not a sheet this engine can read; the page kept the size it had",
		Property: "size",
	})
}

func badPageMargin(rec *Recorder, p pendingPage, d css.Declaration) {
	rec.ReportDetail(Finding{
		Rule:   RuleInvalidCSS,
		Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
		Message: "the @page " + strings.ToLower(d.Name) + " " + quoteValue(strings.TrimSpace(pageText(d.Value))) +
			" is not a margin this engine can read; the page kept the margin it had",
		Property: strings.ToLower(d.Name),
	})
}

// pageMarginShorthand reads the one-to-four value form, which sets the sides in
// the order top, right, bottom, left with the missing ones taken from the
// opposite side.
func pageMarginShorthand(vals []css.ComponentValue) ([4]style.Length, bool) {
	var out [4]style.Length
	parts := splitValuesOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 4 {
		return out, false
	}
	var got [4]style.Length
	for i, part := range parts {
		l, ok := pageMarginValue(part)
		if !ok {
			return out, false
		}
		got[i] = l
	}
	switch len(parts) {
	case 1:
		return [4]style.Length{got[0], got[0], got[0], got[0]}, true
	case 2:
		return [4]style.Length{got[0], got[1], got[0], got[1]}, true
	case 3:
		return [4]style.Length{got[0], got[1], got[2], got[1]}, true
	}
	return got, true
}

// pageMarginValue reads one side's value.
//
// "auto" is not among them. On an element it means "let the layout work it
// out", and the layout that would is the one centring a block in its containing
// block — there is no such calculation for the paper, which is why CSS leaves
// an auto page margin to the printer. Reading it as nought would print to the
// edge of the sheet, so it is refused and the caller's margin stands.
func pageMarginValue(vals []css.ComponentValue) (style.Length, bool) {
	l, _, ok := pageMarginLength(vals)
	if !ok || l.Kind == style.LengthAuto {
		return l, false
	}
	return l, true
}

// pageMarginLength parses one length in the page context, which is what both
// the margins and the size are written in.
func pageMarginLength(vals []css.ComponentValue) (style.Length, bool, bool) {
	ctx := style.LengthContext{
		// An em on the page is the initial font size, for the reason a media
		// query's em is: there is no element here whose font it could be
		// relative to. The page's own font-size descriptor would change that,
		// and it is one of the descriptors reported as unapplied above.
		FontSize:     pageFontSize(),
		RootFontSize: pageFontSize(),
	}
	return style.ParseLength(vals, ctx)
}

func pageFontSize() style.Unit {
	u, _ := style.FromPx(style.DefaultFontSize)
	return u
}

// nonWhitespace drops the whitespace tokens of a value list, which is what
// asking whether a prelude is empty means.
func nonWhitespace(vals []css.ComponentValue) []css.ComponentValue {
	out := make([]css.ComponentValue, 0, len(vals))
	for _, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Whitespace {
			continue
		}
		out = append(out, v)
	}
	return out
}

// pageText writes a value back out for a finding to quote.
//
// It is for a message and not for parsing anything: a value this engine could
// not read is exactly the value most worth showing the author, so a token it
// has no spelling for becomes its own kind rather than making the whole thing
// unquotable.
func pageText(vals []css.ComponentValue) string {
	var b strings.Builder
	for _, v := range vals {
		t := v.Token
		switch {
		case v.IsFunction():
			b.WriteString(t.Value + "(" + pageText(v.Values) + ")")
		case v.IsBlock():
			open, close := blockDelimiters(t.Kind)
			b.WriteString(open + pageText(v.Values) + close)
		default:
			b.WriteString(tokenText(t))
		}
	}
	return b.String()
}

func tokenText(t css.Token) string {
	switch t.Kind {
	case css.Ident, css.Delim, css.URL:
		return t.Value
	case css.AtKeyword:
		return "@" + t.Value
	case css.Hash:
		return "#" + t.Value
	case css.String:
		return `"` + t.Value + `"`
	case css.Number:
		return t.Repr
	case css.Percentage:
		return t.Repr + "%"
	case css.Dimension:
		return t.Repr + t.Unit
	case css.Whitespace:
		return " "
	case css.Colon:
		return ":"
	case css.Semicolon:
		return ";"
	case css.Comma:
		return ","
	}
	return ""
}

func blockDelimiters(k css.Kind) (string, string) {
	switch k {
	case css.LeftSquare:
		return "[", "]"
	case css.LeftBrace:
		return "{", "}"
	}
	return "(", ")"
}

// The paper a stylesheet can name, from CSS Paged Media 3 §5.1.
//
// The four this engine already exports are taken from those rather than
// restated, so that "@page { size: A4 }" and layout.A4 cannot come to mean two
// different sheets. The rest are their own dimensions: the ISO B series, the
// JIS B series — which is a different paper of the same name, and is why the
// specification lists both — and the two North American sizes past letter.
//
// Every one of them is given portrait, which is how the specification lists
// them and what makes "landscape" a turn rather than a size of its own.
var pageSizes = map[string]Size{
	"a5":     {W: A5.Width, H: A5.Height},
	"a4":     {W: A4.Width, H: A4.Height},
	"a3":     paperMm(297, 420),
	"b5":     paperMm(176, 250),
	"b4":     paperMm(250, 353),
	"jis-b5": paperMm(182, 257),
	"jis-b4": paperMm(257, 364),
	"letter": {W: Letter.Width, H: Letter.Height},
	"legal":  {W: Legal.Width, H: Legal.Height},
	"ledger": paperIn(11, 17),
}

func paperMm(w, h float64) Size {
	return Size{W: ptToUnit(w * 72 / 25.4), H: ptToUnit(h * 72 / 25.4)}
}

func paperIn(w, h float64) Size {
	return Size{W: ptToUnit(w * 72), H: ptToUnit(h * 72)}
}

// pageSizeValue reads the size descriptor, which is what chooses the paper.
//
// The grammar of §5.1 is "auto | <length>{1,2} | <page-size> || <orientation>",
// and the "||" is the part worth stating: a named size and an orientation may
// be written in either order, and either may appear without the other. So this
// classifies the one or two parts rather than matching a sequence, which is
// also what makes "size: A4 A5" a value it refuses instead of one it half
// reads.
//
// base is the sheet the caller asked for, which is what "auto" is and what an
// orientation with no size of its own turns.
func pageSizeValue(vals []css.ComponentValue, base PageSize) (style.Unit, style.Unit, bool) {
	parts := splitValuesOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 2 {
		return 0, 0, false
	}

	var (
		sheet          = Size{W: base.Width, H: base.Height}
		named, turned  bool
		landscape      bool
		lengths        []style.Unit
		sawAutoKeyword bool
	)
	for _, part := range parts {
		if name, ok := identName(part); ok {
			switch {
			case name == "auto":
				sawAutoKeyword = true
			case name == "portrait" || name == "landscape":
				if turned {
					return 0, 0, false
				}
				turned, landscape = true, name == "landscape"
			default:
				size, known := pageSizes[name]
				if !known || named {
					return 0, 0, false
				}
				sheet, named = size, true
			}
			continue
		}
		// Not a keyword, so it is one of the one or two lengths. A percentage
		// has nothing to be a percentage of here — the page is what everything
		// else is measured against — and ParseLength's other kinds are refused
		// with it.
		l, _, ok := pageMarginLength(part)
		if !ok || l.Kind != style.LengthAbsolute || l.Value <= 0 {
			return 0, 0, false
		}
		lengths = append(lengths, l.Value)
	}

	switch {
	case len(lengths) > 0:
		// A length cannot be combined with any of the keywords: the pair is the
		// sheet outright.
		if named || turned || sawAutoKeyword {
			return 0, 0, false
		}
		if len(lengths) == 1 {
			// One length is a square page, which §5.1 says and which is the
			// only way to ask for one.
			return lengths[0], lengths[0], true
		}
		return lengths[0], lengths[1], true
	case sawAutoKeyword:
		// "auto" is the caller's sheet, and it is not a value that combines
		// with anything either.
		if named || turned {
			return 0, 0, false
		}
		return base.Width, base.Height, true
	}

	w, h := sheet.W, sheet.H
	if turned && (landscape == (h > w)) {
		// The named sizes are listed portrait, so turning is a swap and asking
		// for the orientation a sheet already has is not.
		w, h = h, w
	}
	return w, h, true
}

// identName reads a part that is one keyword.
func identName(part []css.ComponentValue) (string, bool) {
	if len(part) != 1 || !part[0].IsToken() || part[0].Token.Kind != css.Ident {
		return "", false
	}
	return strings.ToLower(part[0].Token.Value), true
}
