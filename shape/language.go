package shape

import (
	"sort"
	"strings"
)

// Which of a font's language systems a run is set in.
//
// A font states some of its rules per language: Serbian and Russian write the
// same Cyrillic letters and draw five of them differently, a Romanian ș is not
// a Turkish ş, and Marathi's eyelash ra is a form Hindi does not use. The font
// says which rules are whose through the language systems each script names —
// 'SRB ', 'ROM ', 'MAR ' — and a run set without asking is set in the font's
// default one, which is right for most text and wrong for exactly the text the
// font went to the trouble of correcting.
//
// Which language a run is in cannot be read off its characters. The document
// says it, as an HTML lang attribute, in BCP 47: "sr", "ro-MD", "zh-Hant-HK".
// So the language is a shaping input like the features a document turned off
// — Features.Language, carried the whole way from the element to the backend
// that draws the run — and this file turns it into the OpenType tags a font's
// ScriptList is searched for.
//
// # The mapping is HarfBuzz's
//
// BCP 47's registry and OpenType's are two lists maintained by two bodies, and
// the join between them is a long list of corrections: a macrolanguage whose
// OpenType tag is one of its members', a retired code, a region that picks the
// traditional forms of Chinese. HarfBuzz keeps that join as a generated table,
// fonts are tested against HarfBuzz, and a join made again here would disagree
// with it exactly where the corrections are. So langtags.go is HarfBuzz's own
// table, generated from its hb-ot-tag-table.hh at a pinned release by
// cmd/genlangtags, and openTypeLanguages is hb_ot_tags_from_script_and_language
// read line for line — checked against HarfBuzz itself, called through its C
// API, in language_test.go.
//
// Two private-use subtags of HarfBuzz's are read as HarfBuzz reads them: "-hbot",
// which names a language system tag outright, and "-hbsc", which names the
// script tag a run's rules are read under in place of the ones its characters'
// script has — "und-x-hbscdflt" sets Devanagari text by a font's 'DFLT' rules,
// and so by the default model, as HarfBuzz sets it. See otLanguage.

// langTagEntry is one row of the generated tables: a primary subtag and the
// language system tags it selects, most specific first. Chinese as written in
// Macao is a row with more than one — 'ZHTM', then Hong Kong's 'ZHH ' for a font
// that has no Macao system. HarfBuzz reads at most three
// (HB_OT_MAX_TAGS_PER_LANGUAGE), and cmd/genlangtags refuses a row with more,
// so every row is read whole.
type langTagEntry struct {
	lang string
	tags []string
}

// langRuleKind is which of the four conditions of HarfBuzz's
// hb_ot_tags_from_complex_language a rule tests.
type langRuleKind uint8

const (
	// langRuleSubtag matches a subtag anywhere after the primary one:
	// "-polyton" in "el-polyton" or "grc-polyton".
	langRuleSubtag langRuleKind = iota
	// langRuleExact matches the whole tag after its first letter.
	langRuleExact
	// langRulePrefix matches the start of the tag after its first letter, up
	// to the end of a subtag: "h-hant" in "zh-hant-tw".
	langRulePrefix
	// langRuleStartAndSubtag matches the start of the tag after its first
	// letter, anywhere, and a subtag anywhere after the primary one: "zh-" and
	// "-hk".
	langRuleStartAndSubtag
)

// langRule is one rule of hb_ot_tags_from_complex_language, in the order that
// function tries them.
type langRule struct {
	kind langRuleKind
	// first is the tag's first letter, which the rules after the subtag ones
	// are switched on; spec is matched against what follows it.
	first  byte
	spec   string
	subtag string
	tags   []string
}

// languageKey is a run's language as the selection sees it: the language system
// tags it is looked up under, joined. Two languages a font is searched for
// under the same tags — "zh" and "zh-CN", "sr" and "sr-Cyrl" — are one key, so
// they share the layout the search arrives at.
func languageKey(tags []string) string {
	switch len(tags) {
	case 0:
		return ""
	case 1:
		return tags[0]
	}
	return strings.Join(tags, ",")
}

// otLanguage is what a run's language asks of a font's ScriptList: the
// language system tags it is looked up under, most specific first, and the
// script tag HarfBuzz's "-hbsc" names in place of the run's own, or "" where
// the tag names none.
//
// The script tag is here rather than beside the script because it comes from
// the same attribute and is read by the same parse, and because what it
// replaces is a question the script alone used to answer: which script tables
// are searched. The run's script still decides everything else — its model is
// chosen from the script and the tag found, as HarfBuzz chooses it
// (hb_ot_shaper_categorize is handed the buffer's script and the chosen tag).
type otLanguage struct {
	tags   []string
	script string
}

// scriptTags is the script tags a run of the given script is looked up under
// in this language: the one "-hbsc" names, or the script's own. HarfBuzz tries
// the default tags after either, and so does scriptSelection.
func (l otLanguage) scriptTags(script uint16) []string {
	if l.script != "" {
		return []string{l.script}
	}
	return layoutTags(script)
}

// openTypeLanguages is the language system tags a BCP 47 language tag is
// looked up under, most specific first, or none for a language OpenType has no
// tag for and for no language at all. The slice may be the generated table's
// own, and is not to be written to.
func openTypeLanguages(bcp47 string) []string { return openTypeLanguage(bcp47).tags }

// openTypeLanguage is hb_ot_tags_from_script_and_language: the language system
// tags a BCP 47 language tag is looked up under, and the script tag its
// private-use part names, if it names one.
func openTypeLanguage(bcp47 string) otLanguage {
	lang := canonicalLanguage(bcp47)
	if lang == "" {
		return otLanguage{}
	}
	// Where the tag's own subtags end: at the first singleton — a one-letter
	// subtag, which introduces an extension ("-u-") or the private-use part
	// ("-x-") — because what follows one is not about the language. A tag that
	// is private use from its start has no language subtags at all.
	limit, private := len(lang), -1
	if strings.HasPrefix(lang, "x-") {
		private = 0
	} else {
		limitSet := false
		for i := 1; i < len(lang); i++ {
			if lang[i-1] != '-' || i+1 >= len(lang) || lang[i+1] != '-' {
				continue
			}
			if !limitSet {
				limit, limitSet = i-1, true
			}
			if lang[i] == 'x' {
				private = i
				break
			}
		}
	}
	var out otLanguage
	if private >= 0 {
		out.script, _ = privateTag(lang[private:], "-hbsc", false)
		if tag, ok := privateTag(lang[private:], "-hbot", true); ok {
			out.tags = []string{tag}
			return out
		}
		if private == 0 {
			// HarfBuzz reads a wholly private tag with no end to its
			// language subtags, and its search can find nothing in one: no
			// rule starts with 'x', and a one-letter primary subtag is in
			// neither table.
			return out
		}
	}
	if tags, ok := complexLanguage(lang, limit); ok {
		out.tags = tags
		return out
	}
	out.tags = primaryLanguage(lang, limit)
	return out
}

// canonicalLanguage is a language tag as HarfBuzz stores one: lowercase, with
// "_" read as "-", and ending at the first character that cannot be in a tag.
func canonicalLanguage(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			continue
		case c >= 'A' && c <= 'Z', c == '_':
			return canonicalLanguageSlow(s)
		}
		return s[:i]
	}
	return s
}

func canonicalLanguageSlow(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
		case c >= 'A' && c <= 'Z':
			c += 'a' - 'A'
		case c == '_':
			c = '-'
		default:
			return string(b)
		}
		b = append(b, c)
	}
	return string(b)
}

// privateTag reads one of HarfBuzz's private-use subtags, which name a tag
// outright: "-hbot" a language system tag and "-hbsc" a script tag, either as
// eight hexadecimal digits after a hyphen ("x-hbot-4d4f4c20") or spelled
// ("x-hbotmol", "x-hbscdev2"), and a spelled tag padded with spaces to four
// characters. A spelled language system tag is uppercased, which is how the
// registry writes them; a script tag is left as the canonical language tag
// has it, in lower case, which is HarfBuzz's lowercasing of it. It is HarfBuzz's
// parse_private_use_subtag, and like it finds the prefix anywhere in the
// private-use part, not only at the start of a subtag.
func privateTag(private, prefix string, upper bool) (string, bool) {
	i := strings.Index(private, prefix)
	if i < 0 {
		return "", false
	}
	s := private[i+len(prefix):]
	var tag [4]byte
	if strings.HasPrefix(s, "-") {
		s = s[1:]
		n := 0
		for n < 8 && n < len(s) && isHex(s[n]) {
			v := fromHex(s[n])
			if n%2 == 0 {
				tag[n/2] = v << 4
			} else {
				tag[n/2] += v
			}
			n++
		}
		if n != 8 {
			return "", false
		}
	} else {
		n := 0
		for n < 4 && n < len(s) && isAlnum(s[n]) {
			c := s[n]
			if upper && c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			tag[n] = c
			n++
		}
		if n == 0 {
			return "", false
		}
		for ; n < 4; n++ {
			tag[n] = ' '
		}
	}
	// 'DFLT' in any case is the default *script* tag, and HarfBuzz flips the
	// case of every letter of it: the language system tag "-hbotDFLT" names is
	// the default language system's spelling, 'dflt', and the script tag
	// "-hbscdflt" names is 'DFLT'.
	if tag[0]&0xDF == 'D' && tag[1]&0xDF == 'F' && tag[2]&0xDF == 'L' && tag[3]&0xDF == 'T' {
		for k := range tag {
			tag[k] ^= 0x20
		}
	}
	return string(tag[:]), true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}

func fromHex(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return c - 'A' + 10
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

func isAlpha(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// complexLanguage is hb_ot_tags_from_complex_language: the rules that read
// more of a tag than its primary subtag, a variant or a script or a region.
func complexLanguage(lang string, limit int) ([]string, bool) {
	// The rules that look for a subtag anywhere are tried only for a tag long
	// enough to hold one, and only where there is a subtag after the primary
	// one of at least four characters inside the limit.
	subtags := false
	p := strings.IndexByte(lang, '-')
	if limit >= 7 && p >= 0 && p < limit && limit-p >= 5 {
		subtags = true
	}
	for i := range langRules {
		r := &langRules[i]
		var ok bool
		switch r.kind {
		case langRuleSubtag:
			ok = subtags && subtagMatches(lang, p, limit, r.spec)
		case langRuleExact:
			ok = lang[0] == r.first && lang[1:] == r.spec
		case langRulePrefix:
			ok = lang[0] == r.first && languageMatches(lang, limit, r.spec)
		case langRuleStartAndSubtag:
			ok = lang[0] == r.first && strings.HasPrefix(lang[1:], r.spec) &&
				subtagMatches(lang, 0, limit, r.subtag)
		}
		if ok {
			return r.tags, true
		}
	}
	return nil, false
}

// subtagMatches is HarfBuzz's subtag_matches: whether subtag, which begins with
// its hyphen, occurs in lang at or after from, starting before limit and not
// running on into a longer subtag.
func subtagMatches(lang string, from, limit int, subtag string) bool {
	if limit-from < len(subtag) {
		return false
	}
	for {
		i := strings.Index(lang[from:], subtag)
		if i < 0 || from+i >= limit {
			return false
		}
		end := from + i + len(subtag)
		if end >= len(lang) || !isAlnum(lang[end]) {
			return true
		}
		from = end
	}
}

// languageMatches is HarfBuzz's lang_matches over the tag after its first
// letter: spec is its start, ending where a subtag does.
func languageMatches(lang string, limit int, spec string) bool {
	rest := lang[1:]
	if limit-1 < len(spec) || !strings.HasPrefix(rest, spec) {
		return false
	}
	return len(rest) == len(spec) || rest[len(spec)] == '-'
}

// primaryLanguage is the rest of hb_ot_tags_from_language: the tags the primary
// subtag selects, or the extended language subtag where there is one.
func primaryLanguage(lang string, limit int) []string {
	// "zh-yue" is Cantonese, written with the macrolanguage first: a
	// three-letter second subtag is an extended language subtag, and is what
	// is looked up.
	start := 0
	if s := strings.IndexByte(lang, '-'); s >= 0 && limit >= 6 {
		ext := lang[s+1:]
		n := len(ext)
		if e := strings.IndexByte(ext, '-'); e >= 0 {
			n = e
		}
		if n == 3 && isAlpha(ext[0]) {
			start = s + 1
		}
	}
	first := limit - start
	if d := strings.IndexByte(lang[start:], '-'); d >= 0 {
		first = d
	}
	if first < 0 {
		return nil
	}
	sub := lang[start : start+first]
	switch first {
	case 2:
		if e, ok := findLangTag(langTags2[:], sub); ok {
			return e.tags
		}
	case 3:
		if e, ok := findLangTag(langTags3[:], sub); ok {
			return e.tags
		}
		if i := sort.SearchStrings(langTags3Blocked[:], sub); i < len(langTags3Blocked) && langTags3Blocked[i] == sub {
			return nil
		}
		// Any other three letters are taken to be an ISO 639-3 code, and the
		// OpenType tag for most of those is the same letters in capitals.
		// HarfBuzz capitalises by clearing a bit, which is what is done here
		// too, so that a subtag with a digit in it becomes the same tag
		// there as here — one no font declares.
		return []string{string([]byte{sub[0] &^ 0x20, sub[1] &^ 0x20, sub[2] &^ 0x20, ' '})}
	}
	return nil
}

// findLangTag looks a primary subtag up in one of the generated tables, which
// are sorted by it.
func findLangTag(table []langTagEntry, sub string) (langTagEntry, bool) {
	i := sort.Search(len(table), func(i int) bool { return table[i].lang >= sub })
	if i < len(table) && table[i].lang == sub {
		return table[i], true
	}
	return langTagEntry{}, false
}
