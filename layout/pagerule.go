package layout

import (
	"strings"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/internal/ascii"
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
// the cascade's walk over the sheets hands the rules over — see pagesOf — as it
// does @font-face, and this file decides between them before anything is
// styled.
//
// # What the caller still decides
//
// The sheet the caller passed in Options is where this starts. A document that
// says nothing about its margins is printed with the caller's, and one that
// does is printed with its own, on the caller's paper. That is the useful order
// for an engine whose job is to print somebody else's HTML: the caller chose
// the paper and the document knows its own design.
//
// The size of the sheet is read, and so is a rule that selects a page: this
// engine composes one page and that page is the first, so ":first" applies and
// is more particular than a rule without it — see pageSelector, which returns a
// specificity for exactly that reason. What is refused is a selector needing a
// sequence of pages to mean anything: ":left", ":right", ":blank" and a named
// page. Those are reported rather than ignored.

// pendingPage is an @page rule with what deciding between two of them needs:
// the stylesheet it was written in, so a finding can say where it came from,
// the origin of that sheet, because origin is the strongest term in the cascade
// and it is no weaker here, and the cascade layer it was written in, which is
// the next term. Whether the @media and @supports around it hold was settled
// by the walk that found it.
type pendingPage struct {
	rule   css.Rule
	sheet  string
	origin style.Origin
	layer  int
}

// at is the place a finding about one rule points to.
func (p pendingPage) at() Source {
	return Source{HTMLOffset: -1, CSSOffset: p.rule.Offset, Sheet: p.sheet}
}

// pagesOf is the document's @page rules as the cascade's walk handed them over.
//
// The walk is the cascade's own, so an @page is found wherever the cascade
// would apply a rule — at the top of a sheet, or inside an @media, an
// @supports or an @layer whose condition held — and nowhere else. This file had
// a walker of its own that descended into @media and nothing more, and whose
// comment said the cascade reported the other enclosing at-rules as unapplied;
// once the cascade applied @supports and @layer that stopped being true, and
// "@supports (display: block) { @page { size: A5 } }" and "@layer print {
// @page { … } }" left the page at the caller's size with nothing said (audit
// C33). One written inside a style rule is not a page rule at all, and the
// walk reports it.
func pagesOf(rules []style.AtRule) []pendingPage {
	out := make([]pendingPage, 0, len(rules))
	for _, r := range rules {
		out = append(out, pendingPage{rule: r.Rule, sheet: r.Sheet, origin: r.Origin, layer: r.Layer})
	}
	return out
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
	pageTerms
}

// pageTerms are the cascade terms one @page declaration is decided by, and
// the order they are consulted in is the cascade's own: importance and origin,
// then the layer, then how particular the page selector was, then order.
type pageTerms struct {
	rank  int
	layer int
	spec  int
	order int
	set   bool
}

// beats reports whether a declaration with these terms wins over one already
// held. The size and the margins are decided the same way, because they are
// declarations in the same rule and nothing about either is a different kind
// of question.
func (d pageTerms) beats(o pageTerms) bool {
	if !o.set {
		return true
	}
	if d.rank != o.rank {
		return d.rank > o.rank
	}
	if d.layer != o.layer {
		return d.layer > o.layer
	}
	if d.spec != o.spec {
		return d.spec > o.spec
	}
	return d.order > o.order
}

func takeSize(held *pageSizeDeclaration, d pageSizeDeclaration) {
	if d.beats(held.pageTerms) {
		*held = d
	}
}

// pageDeclaration is one side's margin as one rule declared it, with what
// deciding against another declaration of the same side needs.
type pageDeclaration struct {
	length style.Length
	pageTerms
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
	// The queries around the rules were answered already, by the walk that
	// found them, about the sheet the caller asked for — the only answer that
	// does not depend on what the rules inside the queries decide. See asked
	// in pipeline.go.
	var got pageDeclarations
	order := 0
	for _, p := range pages {
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

// readPageRule reads the descriptors of one @page rule into the set.
func readPageRule(p pendingPage, base PageSize, got *pageDeclarations, order *int, rec *Recorder) {
	spec, ok := pageSelector(p.rule.Prelude)
	if !ok {
		// ":left", ":right", ":blank" and a named page each pick pages out of a
		// sequence this engine does not have: a document is one page, and which
		// side of a sheet it would be printed on, or whether a page break left
		// it empty, are questions about a run of pages. There is no honest way
		// to decide whether such a rule applies to the one page there is.
		rec.ReportDetail(Finding{
			Rule:     RuleUnsupportedAtRule,
			Source:   p.at(),
			Message:  "@page " + quoteValue(ascii.TrimCSSSpace(pageText(p.rule.Prelude))) + " selects some pages rather than all of them, which this engine does not do; the rule was not applied",
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
		terms := pageTerms{rank: style.CascadeRank(p.origin, d.Important),
			layer: style.LayerRank(p.layer, d.Important), spec: spec, order: *order, set: true}
		switch ascii.Lower(d.Name) {
		case "margin":
			if spread, ok := pageMarginShorthand(d.Value); ok {
				for i, l := range spread {
					take(&got.sides[i], pageDeclaration{length: l, pageTerms: terms})
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
			}[ascii.Lower(d.Name)]
			if l, ok := pageMarginValue(d.Value); ok {
				take(&got.sides[at], pageDeclaration{length: l, pageTerms: terms})
			} else {
				badPageMargin(rec, p, d)
			}
		case "size":
			if w, h, ok := pageSizeValue(d.Value, base); ok {
				takeSize(&got.size, pageSizeDeclaration{width: w, height: h, pageTerms: terms})
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
				Message:  "the @page descriptor " + quoteValue(ascii.Lower(d.Name)) + " is not applied",
				Property: ascii.Lower(d.Name),
			})
		}
	}
}

// take keeps whichever of two declarations for the same side the cascade
// prefers.
func take(held *pageDeclaration, d pageDeclaration) {
	if d.beats(held.pageTerms) {
		*held = d
	}
}

func badPageSize(rec *Recorder, p pendingPage, d css.Declaration) {
	rule, why := RuleInvalidCSS, " is not a sheet this engine can read"
	if style.UsesVar(d.Value) {
		rule, why = RuleUnsupportedValue, " refers to a custom property, which this "+
			"engine does not substitute"
	}
	rec.ReportDetail(Finding{
		Rule:   rule,
		Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
		Message: "the @page size " + quoteValue(ascii.TrimCSSSpace(pageText(d.Value))) +
			why + "; the page kept the size it had",
		Property: "size",
	})
}

// badPageMargin reports a margin descriptor this could not use, as the cascade
// would report the same value on an element: a var() and a value the value
// grammar calls valid and unevaluated — min(), "auto", which the paper has no
// calculation for — are this engine's gap and not the author's mistake. The
// var() case was reported as invalid CSS while "margin: var(--m)" on an
// element was unsupported, so the page was counted clean with its margin
// wrong (audit C110).
func badPageMargin(rec *Recorder, p pendingPage, d css.Declaration) {
	rule, why := RuleInvalidCSS, " is not a margin"
	switch unsupported := pageMarginUnsupported(d.Value); {
	case style.UsesVar(d.Value):
		rule, why = RuleUnsupportedValue, " refers to a custom property, which this "+
			"engine does not substitute"
	case unsupported != "":
		rule, why = RuleUnsupportedValue, " uses "+unsupported+", which this engine "+
			"does not apply to the page"
	}
	rec.ReportDetail(Finding{
		Rule:   rule,
		Source: Source{HTMLOffset: -1, CSSOffset: d.Offset, Sheet: p.sheet},
		Message: "the @page " + ascii.Lower(d.Name) + " " + quoteValue(ascii.TrimCSSSpace(pageText(d.Value))) +
			why + "; the page kept the margin it had",
		Property: ascii.Lower(d.Name),
	})
}

// pageMarginUnsupported names what, in a margin value that is valid CSS, this
// engine does not apply to the page, or is empty when the value is not valid
// or holds nothing of the kind. Each part is asked of margin-top's grammar,
// which is what an @page margin's value is (css-page-3 §3.1).
func pageMarginUnsupported(vals []css.ComponentValue) string {
	parts := splitValuesOnWhitespace(vals)
	if len(parts) == 0 || len(parts) > 4 {
		return ""
	}
	what := ""
	for _, part := range parts {
		ok, unsupported := style.JudgeValue("margin-top", part)
		if !ok {
			return ""
		}
		if name, isIdent := identName(part); isIdent && name == "auto" && what == "" {
			what = "auto"
		}
		if unsupported != "" && what == "" {
			what = unsupported
		}
	}
	return what
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

// Both take the sheet to the nearest unit rather than downwards, which is what
// PageSizePt does to the four sizes above them — the alternative is a table
// where "a4" and "a3" are quantised two different ways. See style.RoundPx.
func paperMm(w, h float64) Size {
	return Size{W: ptToSheet(w * 72 / 25.4), H: ptToSheet(h * 72 / 25.4)}
}

func paperIn(w, h float64) Size {
	return Size{W: ptToSheet(w * 72), H: ptToSheet(h * 72)}
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
	return ascii.Lower(part[0].Token.Value), true
}

// pageSelector reads an @page rule's prelude: whether it selects the page this
// engine composes, and how particular it was about it.
//
// A document is one page here, and that page is the first one — so ":first",
// the rule a title page is written with, applies. What it is not is the *only*
// page selector, and CSS 2.1 §13.2.4 orders them: a rule with a pseudo-page
// beats one without, whichever was written first. So this returns a
// specificity rather than a yes, and the cascade term goes between origin and
// order exactly where the specification puts it.
//
// The ones refused are the ones that need a sequence: ":left" and ":right" are
// the two sides of a leaf, ":blank" is a page a break left empty, and a named
// page is chosen by a "page" property on content that would have to break onto
// it. None of those is a question a single page can answer.
func pageSelector(prelude []css.ComponentValue) (int, bool) {
	parts := nonWhitespace(prelude)
	if len(parts) == 0 {
		return 0, true
	}
	// ":first" is a colon and an identifier, and nothing else this engine reads
	// is two tokens long — so anything of another shape is refused before its
	// spelling is looked at.
	if len(parts) == 2 && parts[0].IsToken() && parts[0].Token.Kind == css.Colon &&
		parts[1].IsToken() && parts[1].Token.Kind == css.Ident &&
		ascii.EqualFold(parts[1].Token.Value, "first") {
		return 1, true
	}
	return 0, false
}
