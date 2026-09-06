package style

import (
	"strings"

	"github.com/mgilbir/forme/css"
)

// Media Queries 4, for a medium that is a sheet of paper.
//
// # Why this engine answers "print"
//
// A media query asks what the output device is like, and every answer here is
// the same one: this is a rendering engine for paged media, so the medium is
// "print" and the surface is the sheet. "@media screen" therefore matches
// nothing and "@media print" matches everything — which is not a limitation
// being worked around but the whole point of the question. A browser printing
// the same document answers it the same way.
//
// # Why the rules inside were dropped before this
//
// Every at-rule was reported and skipped, so "@media print { .no-print {
// display: none } }" — the commonest line in any stylesheet meant for paper —
// left the box on the page. The finding said so, which is the honest half; the
// other half is that a print engine can answer this query exactly.
//
// # What is not answered
//
// A feature about a screen's abilities: a pointer, a colour gamut, a
// resolution, a preference the reader set in a browser. Media Queries 4 makes
// an unknown feature false, so the rules inside are dropped — the same answer
// this file gave before it could evaluate anything — but the *reason* is
// different and worth reporting: a browser printing the document may know what
// "prefers-color-scheme" is, and its page would differ from this one.

// Media is what a query is asked about: the sheet, and whether the medium is
// paper at all.
//
// Width and Height are the page box — the paper — rather than the area inside
// its margins. That is what CSS means by the viewport in paged media, and it is
// what an author means by "@media (min-width: 200mm)": a statement about the
// sheet they are printing on, not about the text column.
type Media struct {
	Width, Height Unit
}

// MatchesMedia reports whether a query list matches the medium, and names the
// first thing in it this engine could not answer.
//
// A list matches when any query in it does, which is what the comma means. The
// name is returned even when something else in the list matched, because the
// author still wrote a query this engine reads differently from a browser and
// that is worth one finding either way.
//
// It is exported because @page is decided outside the cascade and a print
// stylesheet writes its page rules inside "@media print": the stage that reads
// them has the same question to ask, and asking it with a second copy of this
// is how the two answers come to differ.
func MatchesMedia(prelude []css.ComponentValue, m Media) (bool, string) {
	matched, unknown := false, ""
	for _, query := range splitOnComma(prelude) {
		ok, why := oneMediaQuery(query, m)
		if why != "" && unknown == "" {
			unknown = why
		}
		if ok {
			matched = true
		}
	}
	return matched, unknown
}

// oneMediaQuery evaluates a single query: an optional "not" or "only", an
// optional media type, and any number of parenthesised features joined by
// "and".
//
// "only" is the keyword that hid a query from the CSS2 parsers of the 1990s. It
// means nothing to anything written since, and it means nothing here.
func oneMediaQuery(query []css.ComponentValue, m Media) (bool, string) {
	parts := splitOnWhitespace(query)
	negate := false
	i := 0
	if i < len(parts) && isIdentValue(parts[i], "not") {
		negate, i = true, i+1
	} else if i < len(parts) && isIdentValue(parts[i], "only") {
		i++
	}

	matched := true
	first := true
	for ; i < len(parts); i++ {
		part := parts[i]
		if !first {
			// Every part after the first is joined by "and", and anything else
			// joining them — "or", or nothing at all — is a query this does not
			// read. Media Queries 4 makes such a query false, and it says so.
			if !isIdentValue(part, "and") {
				return false, serialize(part)
			}
			i++
			if i >= len(parts) {
				return false, "and"
			}
			part = parts[i]
		}
		ok, why := mediaTerm(part, m, first)
		if why != "" {
			return false, why
		}
		matched = matched && ok
		first = false
	}
	if first {
		// An empty query, which is not one.
		return false, ""
	}
	return matched != negate, ""
}

// mediaTerm evaluates one part of a query: a media type where one may still
// appear, or a feature in parentheses.
func mediaTerm(part []css.ComponentValue, m Media, mayBeType bool) (bool, string) {
	if len(part) == 1 && part[0].IsToken() && part[0].Token.Kind == css.Ident && mayBeType {
		return mediaTypeMatches(strings.ToLower(part[0].Token.Value))
	}
	if len(part) == 1 && part[0].IsBlock() && part[0].Token.Kind == css.LeftParen {
		return mediaFeature(part[0].Values, m)
	}
	return false, serialize(part)
}

// mediaTypeMatches is §3: the medium is paper.
//
// "all" is every medium and "print" is this one. The rest are the media types
// CSS has ever named, and each of them is a device this is not — a screen, a
// teletype, a braille display. They are recognised rather than reported: a
// stylesheet that says "@media screen" has not asked for anything this engine
// failed to do, it has said which medium it was talking about and it was not
// this one.
func mediaTypeMatches(name string) (bool, string) {
	switch name {
	case "all", "print":
		return true, ""
	case "screen", "speech", "aural", "braille", "embossed", "handheld",
		"projection", "tty", "tv":
		return false, ""
	}
	return false, name
}

// mediaFeature evaluates one parenthesised feature.
//
// The three that a sheet of paper can answer are its width, its height and
// which way round it is. Everything else is about a device this is not, and
// Media Queries 4's answer for a feature the engine does not know is that the
// query is false — so the rules inside are dropped either way, and the name
// comes back so that the drop can be reported rather than silent.
func mediaFeature(vals []css.ComponentValue, m Media) (bool, string) {
	parts := splitOnColon(vals)
	name := strings.ToLower(strings.TrimSpace(serialize(parts[0])))
	if len(parts) == 1 {
		// A feature written with no value is true when the feature is not zero,
		// which for a length is a page that has one.
		switch name {
		case "width":
			return m.Width > 0, ""
		case "height":
			return m.Height > 0, ""
		}
		return false, name
	}
	if len(parts) != 2 {
		return false, name
	}

	bare := strings.TrimPrefix(strings.TrimPrefix(name, "min-"), "max-")
	var against Unit
	switch bare {
	case "width":
		against = m.Width
	case "height":
		against = m.Height
	case "orientation":
		want := strings.ToLower(strings.TrimSpace(serialize(parts[1])))
		switch want {
		case "portrait":
			return m.Height >= m.Width, ""
		case "landscape":
			return m.Width > m.Height, ""
		}
		return false, name
	default:
		return false, name
	}

	length, _, ok := ParseLength(parts[1], LengthContext{
		FontSize: mediaFontSize(), RootFontSize: mediaFontSize(),
		ViewportWidth: m.Width, ViewportHeight: m.Height, ViewportKnown: true,
	})
	if !ok || length.Kind != LengthAbsolute {
		return false, name
	}
	switch {
	case strings.HasPrefix(name, "min-"):
		return against >= length.Value, ""
	case strings.HasPrefix(name, "max-"):
		return against <= length.Value, ""
	}
	return against == length.Value, ""
}

// mediaFontSize is what an em in a media query is: the *initial* font size,
// never the document's own.
//
// §1.3 of Media Queries 4 says so and the reason is worth keeping: a query is
// evaluated before any element has a style, so there is no element whose font
// an em could be relative to — and a query that changed the rules that set the
// font size would then have been asked against a size those rules produced.
func mediaFontSize() Unit {
	v, _ := FromPx(DefaultFontSize)
	return v
}

// splitOnColon cuts a feature into its name and its value. A query list is cut
// on its commas by splitOnComma, which the shorthands already needed.
func splitOnColon(vals []css.ComponentValue) [][]css.ComponentValue {
	var out [][]css.ComponentValue
	start := 0
	for i, v := range vals {
		if v.IsToken() && v.Token.Kind == css.Colon {
			out = append(out, vals[start:i])
			start = i + 1
		}
	}
	return append(out, vals[start:])
}

// isIdentValue reports whether one part of a query is a given keyword.
func isIdentValue(part []css.ComponentValue, name string) bool {
	return len(part) == 1 && part[0].IsToken() && part[0].Token.Kind == css.Ident &&
		strings.EqualFold(part[0].Token.Value, name)
}
