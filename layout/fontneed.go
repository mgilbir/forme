package layout

import (
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// What a font finding asks the caller to supply.
//
// A finding about a font says in its message what went wrong, for a person.
// A caller that wants to act on it — load another face and lay the document out
// again — needs the same facts as data, in the terms its own FontSet is asked
// in: which family name, which weight, width and style, which characters, which
// tables or features. FontNeed is those facts. Every finding a different face
// would clear carries one in Finding.Font, and Composed.FontNeeds gathers them,
// so the retry is mechanical:
//
//	out := layout.Compose(in, opts)
//	for _, need := range out.FontNeeds() {
//		// find a face for need.Families[0] (or any of them) that answers
//		// need.Request, covers need.Text and has need.Features, and add it
//		// to the set under that family name
//	}
//	out = layout.Compose(in, opts) // with the larger set
//
// The family names are exactly the strings this package passes to
// FontSet.Face and StyledFontSet.FaceStyled — unquoted, a generic family as its
// keyword — so a set that answers a need's family for its request meets it.

// FontNeed is what would clear a font finding: a face, supplied under any one
// of Families, that answers Request, has a glyph for every character in Text,
// and has every table or feature in Features.
type FontNeed struct {
	// Families are the alternatives, in the order the document asked for them:
	// a face for any one of them clears the finding. Each is a family name as
	// FontSet.Face is called with it. Where layout fell back to font-family's
	// initial value, "serif" is the last of them, and a face for it clears the
	// finding too.
	Families []string
	// Request is the weight, width and style the face must answer. A
	// FontSet answers Request.Bold() and Request.Italic(); a StyledFontSet is
	// asked the request itself.
	Request FontRequest
	// Text is the characters the face must have glyphs for, each once. Empty
	// means any face of the family will do.
	Text string
	// Features are what the face must have besides glyphs: OpenType feature
	// tags such as "smcp", "onum" or "sups", or "MATH" for a MATH table.
	Features []string
	// Used is the name of the face layout set the text in instead, where it set
	// it in one; empty where the text was not drawn.
	Used string
}

// The bounds on a FontNeed. Every part of one comes from the document, and a
// finding is charged to the recorder's budget by its size, so each is capped
// where a real document never reaches: a font-family list of more than 32
// names, a family name longer than 256 bytes, more than 256 characters a face
// is missing in one finding, or more than 16 features.
const (
	maxNeedFamilies    = 32
	maxNeedFamilyBytes = 256
	maxNeedRunes       = 256
	maxNeedFeatures    = 16
	maxNeedScanBytes   = 1 << 16
)

// newFontNeed builds a need from a box's style: its font-family list and its
// request, with the characters and features the finding is about. initial
// says layout tried font-family's initial value after the list, which is then
// one of the alternatives too.
func newFontNeed(b *Box, initial bool, text string, features ...string) *FontNeed {
	var families []string
	r := normalRequest
	if b != nil {
		families = parseFamilyList(b.Style.Get("font-family"))
		r = fontRequestOf(b.Style)
	}
	return needFor(families, initial, r, text, features...)
}

// needFor is newFontNeed from its parts, bounded.
func needFor(families []string, initial bool, r FontRequest, text string, features ...string) *FontNeed {
	n := &FontNeed{Request: r}
	add := func(f string) {
		if f == "" || len(f) > maxNeedFamilyBytes || len(n.Families) >= maxNeedFamilies {
			return
		}
		for _, have := range n.Families {
			if familyKey(have) == familyKey(f) {
				return
			}
		}
		n.Families = append(n.Families, f)
	}
	for _, f := range families {
		add(f)
	}
	if initial {
		add(initialFamily)
	}
	n.Text = needRunes("", text)
	for _, f := range features {
		if f != "" && len(n.Features) < maxNeedFeatures && !slices.Contains(n.Features, f) {
			n.Features = append(n.Features, f)
		}
	}
	return n
}

// needRunes is have with the characters of text it does not hold appended,
// each once, up to maxNeedRunes in all. Only the first maxNeedScanBytes of
// text are read: it is called per box with a run's text, which can be a whole
// paragraph set in a fallback face, and a need that lists the first few hundred
// characters to cover has done its job — the next Compose reports the rest.
func needRunes(have, text string) string {
	if text == "" {
		return have
	}
	n := utf8.RuneCountInString(have)
	if n >= maxNeedRunes {
		return have
	}
	seen := make(map[rune]bool, n+8)
	for _, r := range have {
		seen[r] = true
	}
	if len(text) > maxNeedScanBytes {
		text = text[:maxNeedScanBytes]
	}
	var b strings.Builder
	b.WriteString(have)
	for _, r := range text {
		if n >= maxNeedRunes {
			break
		}
		if r == utf8.RuneError || seen[r] {
			continue
		}
		seen[r] = true
		b.WriteRune(r)
		n++
	}
	return b.String()
}

// usedBy says which face set the text instead, and returns the need.
func (n *FontNeed) usedBy(name string) *FontNeed {
	n.Used = name
	return n
}

// size is the bytes a need holds, for the recorder's charge.
func (n *FontNeed) size() int {
	if n == nil {
		return 0
	}
	s := len(n.Text) + len(n.Used)
	for _, f := range n.Families {
		s += len(f)
	}
	for _, f := range n.Features {
		s += len(f)
	}
	return s
}

// key is a need as a string, for the recorder's deduplication: two findings
// whose messages read alike but that ask for different faces are two findings.
func (n *FontNeed) key() string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString(n.groupKey())
	b.WriteByte(0)
	b.WriteString(n.Text)
	b.WriteByte(0)
	b.WriteString(n.Used)
	return b.String()
}

// groupKey is what two needs must share to be one need for FontNeeds: the
// same alternatives, request and features. The characters are then united.
func (n *FontNeed) groupKey() string {
	var b strings.Builder
	for _, f := range n.Families {
		b.WriteString(strconv.Itoa(len(f)))
		b.WriteByte(':')
		b.WriteString(f)
	}
	b.WriteByte(0)
	r := n.Request
	b.WriteString(strconv.FormatFloat(r.Weight, 'g', -1, 64) + "/" +
		strconv.FormatFloat(r.Width, 'g', -1, 64) + "/" + strconv.Itoa(int(r.Slope)) + "/" +
		strconv.FormatFloat(r.Angle, 'g', -1, 64))
	b.WriteByte(0)
	b.WriteString(strings.Join(n.Features, ","))
	return b.String()
}

// FontNeeds is every FontNeed the findings carry, merged: needs with the same
// families, request and features are one need, with their characters united.
// They are in the order of the first finding each came from, in
// Composed.Findings.
//
// It is the caller's retry list. A face for each need, added to the font set,
// clears the findings that need came from on the next Compose — though the new
// faces may set text differently, so the next Compose's findings are the ones
// to read rather than a promise that there will be none.
func (c Composed) FontNeeds() []FontNeed {
	return fontNeedsOf(c.Findings)
}

// fontNeedsOf is FontNeeds over a list of findings.
func fontNeedsOf(findings []Finding) []FontNeed {
	var out []FontNeed
	at := map[string]int{}
	for _, f := range findings {
		if f.Font == nil {
			continue
		}
		k := f.Font.groupKey()
		if i, ok := at[k]; ok {
			out[i].Text = needRunes(out[i].Text, f.Font.Text)
			if out[i].Used == "" {
				out[i].Used = f.Font.Used
			}
			continue
		}
		at[k] = len(out)
		n := *f.Font
		n.Families = slices.Clone(n.Families)
		n.Features = slices.Clone(n.Features)
		out = append(out, n)
	}
	return out
}
