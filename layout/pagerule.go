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
// and the origin of that sheet, because origin is the strongest term in the
// cascade and it is no weaker here.
type pendingPage struct {
	rule   css.Rule
	sheet  string
	origin style.Origin
}

// at is the place a finding about one rule points to.
func (p pendingPage) at() Source {
	return Source{HTMLOffset: -1, CSSOffset: p.rule.Offset, Sheet: p.sheet}
}

// splitPageRules separates the @page rules of one stylesheet from the rest.
//
// It is splitFontFaces for a different at-rule and the reasoning there applies
// unchanged: an at-rule never contributed a declaration, so removing it leaves
// the ordering the cascade counts exactly as it was.
func splitPageRules(rules []css.Rule, sheet string, origin style.Origin, pages *[]pendingPage) []css.Rule {
	found := false
	for _, r := range rules {
		if isPageRule(r) {
			found = true
			break
		}
	}
	if !found {
		return rules
	}
	out := make([]css.Rule, 0, len(rules))
	for _, r := range rules {
		if isPageRule(r) {
			*pages = append(*pages, pendingPage{rule: r, sheet: sheet, origin: origin})
			continue
		}
		out = append(out, r)
	}
	return out
}

func isPageRule(r css.Rule) bool {
	return r.At && strings.EqualFold(r.Name, "page")
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
	var sides [4]pageDeclaration
	order := 0
	for _, p := range pages {
		readPageRule(p, &sides, &order, rec)
	}

	// A percentage is of the page box, which is the whole sheet: the left and
	// right margins against its width and the top and bottom against its
	// height. Every one of them is definite — this is paper, and its size is
	// known before anything is laid out on it — so the second return is not a
	// case that can arise, and a length that could not resolve was reported
	// where it was read rather than silently becoming nought here.
	basis := [4]style.Unit{page.Height, page.Width, page.Height, page.Width}
	out := [4]style.Unit{page.Margin.Top, page.Margin.Right, page.Margin.Bottom, page.Margin.Left}
	for i, d := range sides {
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

// readPageRule reads the descriptors of one @page rule into the sides.
func readPageRule(p pendingPage, sides *[4]pageDeclaration, order *int, rec *Recorder) {
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
			if got, ok := pageMarginShorthand(d.Value); ok {
				for i, l := range got {
					take(&sides[i], pageDeclaration{length: l, rank: rank, order: *order, set: true})
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
				take(&sides[at], pageDeclaration{length: l, rank: rank, order: *order, set: true})
			} else {
				badPageMargin(rec, p, d)
			}
		default:
			// Everything else an @page block can hold changes the page in a way
			// this engine does not make: "size" chooses the paper, "marks" and
			// "bleed" are for a press, and a property like "background" paints
			// the sheet rather than the document on it. Passing over one
			// silently would print a page the author did not ask for with
			// nothing saying so.
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
	ctx := style.LengthContext{
		// An em on the page is the initial font size, for the reason a media
		// query's em is: there is no element here whose font it could be
		// relative to. The page's own font-size descriptor would change that,
		// and it is one of the descriptors reported as unapplied above.
		FontSize:     pageFontSize(),
		RootFontSize: pageFontSize(),
	}
	l, _, ok := style.ParseLength(vals, ctx)
	if !ok || l.Kind == style.LengthAuto {
		return l, false
	}
	return l, true
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
