package shape

import (
	"sort"

	"github.com/mgilbir/forme/font"
)

// The features a caller asks for, as an AAT face's morx (morx.go) takes them.
//
// A morx chain runs its subtables under flags, and states, feature by feature,
// what a feature asked for does to them: a type and a setting — AAT's names
// for a feature and its value, ligatures and common ligatures on, say — and
// the flags that turns on and the ones it leaves on. A caller asks in OpenType
// tags, and HarfBuzz translates: hb-aat-layout.cc's table of tag to type and
// the two settings that turn it on and off, offered only where the face's feat
// table names the type, and hb_aat_map_builder_t, which keeps one request a
// type — one a setting pair, for a type whose settings are not exclusive — the
// first asked for. What is here is that, at the release the oracle is pinned
// to.
//
// Which requests are a caller's is HarfBuzz's question too: the features a
// caller hands hb_shape, not the ones on by default. Here they are what a run
// asks of a plan (Features.requested): a document's font-feature-settings,
// what the font-variant properties and font-kerning ask for, an @font-face
// rule's settings, and the tags named to ShapeGlyphsWith — each tag settled to
// the last request for it first, in CSS Fonts 4 §7.2's order, since HarfBuzz
// would keep the first of two that disagree. A face with no feat table runs
// every chain with its default flags whatever is asked, as HarfBuzz runs it.
//
// A chain's feature may also follow the run's language rather than a request:
// type 39, whose setting names a language tag of the ltag table. It is on where
// the tag is the run's language (Features.Language) or a less specific form of
// it — "zh" for "zh-Hant" — as hb_language_matches has it, whatever the caller
// asked for, as HarfBuzz turns it on in the flags it compiles for a plan.

// aatFeatureMapping is one row of HarfBuzz's feature_mappings: an OpenType tag,
// the AAT feature type it is, and the settings that turn it on and off.
type aatFeatureMapping struct {
	tag          string
	typ          int
	enable, dflt int
}

// aatFeatureMappings is hb-aat-layout.cc's feature_mappings at the pinned
// release, sorted by tag, its mnemonics written as the numbers they stand
// for in hb-aat-layout.h.
var aatFeatureMappings = [...]aatFeatureMapping{
	{"afrc", 11, 1, 0},
	{"c2pc", 38, 2, 0},
	{"c2sc", 38, 1, 0},
	{"calt", 36, 0, 1},
	{"case", 33, 0, 1},
	{"clig", 1, 18, 19},
	{"cpsp", 33, 2, 3},
	{"cswh", 36, 4, 5},
	{"dlig", 1, 4, 5},
	{"expt", 20, 10, 16},
	{"frac", 11, 2, 0},
	{"fwid", 22, 1, 7},
	{"halt", 22, 6, 7},
	{"hist", 40, 0, 1},
	{"hkna", 34, 0, 1},
	{"hlig", 1, 20, 21},
	{"hngl", 23, 1, 0},
	{"hojo", 20, 12, 16},
	{"hwid", 22, 2, 7},
	{"ital", 32, 2, 3},
	{"jp04", 20, 11, 16},
	{"jp78", 20, 2, 16},
	{"jp83", 20, 3, 16},
	{"jp90", 20, 4, 16},
	{"liga", 1, 2, 3},
	{"lnum", 21, 1, 2},
	{"mgrk", 15, 10, 11},
	{"nlck", 20, 13, 16},
	{"onum", 21, 0, 2},
	{"ordn", 10, 3, 0},
	{"palt", 22, 5, 7},
	{"pcap", 37, 2, 0},
	{"pkna", 22, 0, 7},
	{"pnum", 6, 1, 4},
	{"pwid", 22, 0, 7},
	{"qwid", 22, 4, 7},
	{"rlig", 1, 0, 1},
	{"ruby", 28, 2, 3},
	{"sinf", 10, 4, 0},
	{"smcp", 37, 1, 0},
	{"smpl", 20, 1, 16},
	{"ss01", 35, 2, 3},
	{"ss02", 35, 4, 5},
	{"ss03", 35, 6, 7},
	{"ss04", 35, 8, 9},
	{"ss05", 35, 10, 11},
	{"ss06", 35, 12, 13},
	{"ss07", 35, 14, 15},
	{"ss08", 35, 16, 17},
	{"ss09", 35, 18, 19},
	{"ss10", 35, 20, 21},
	{"ss11", 35, 22, 23},
	{"ss12", 35, 24, 25},
	{"ss13", 35, 26, 27},
	{"ss14", 35, 28, 29},
	{"ss15", 35, 30, 31},
	{"ss16", 35, 32, 33},
	{"ss17", 35, 34, 35},
	{"ss18", 35, 36, 37},
	{"ss19", 35, 38, 39},
	{"ss20", 35, 40, 41},
	{"subs", 10, 2, 0},
	{"sups", 10, 1, 0},
	{"swsh", 36, 2, 3},
	{"titl", 19, 4, 0},
	{"tnam", 20, 14, 16},
	{"tnum", 6, 0, 4},
	{"trad", 20, 0, 16},
	{"twid", 22, 3, 7},
	{"unic", 3, 14, 15},
	{"valt", 22, 5, 7},
	{"vert", 4, 0, 1},
	{"vhal", 22, 6, 7},
	{"vkna", 34, 2, 3},
	{"vpal", 22, 5, 7},
	{"vrt2", 4, 0, 1},
	{"vrtr", 4, 2, 3},
	{"zero", 14, 4, 5},
}

// The feature types and settings HarfBuzz names on its own.
const (
	// aatCharacterAlternatives is the type 'aalt' asks for, its setting the
	// request's value.
	aatCharacterAlternatives = 17
	// aatLetterCase and aatLetterCaseSmallCaps are small capitals as AAT
	// first stated them, deprecated for aatLowerCase and
	// aatLowerCaseSmallCaps, which a chain may still name.
	aatLetterCase          = 3
	aatLetterCaseSmallCaps = 3
	aatLowerCase           = 37
	aatLowerCaseSmallCaps  = 1
	// aatLanguageTag is the type whose setting is one more than the index of
	// a language tag in ltag.
	aatLanguageTag = 39
)

// findAATMapping is hb_aat_layout_find_feature_mapping.
func findAATMapping(tag string) (aatFeatureMapping, bool) {
	i := sort.Search(len(aatFeatureMappings), func(i int) bool { return aatFeatureMappings[i].tag >= tag })
	if i < len(aatFeatureMappings) && aatFeatureMappings[i].tag == tag {
		return aatFeatureMappings[i], true
	}
	return aatFeatureMapping{}, false
}

// aatFeat is a face's feat table: the AAT feature types it offers, each with
// its settings and whether they exclude one another. Nil for a face with none
// and for one HarfBuzz's sanitizer refuses, which offers nothing.
type aatFeat []byte

// readFeat reads a feat table as HarfBuzz's sanitizer admits it: version 1,
// and every feature's record and settings inside the table.
func readFeat(b []byte) aatFeat {
	if len(b) < 12 || font.Be16(b, 0) != 1 {
		return nil
	}
	n := font.Be16(b, 4)
	if len(b) < 12+12*n {
		return nil
	}
	for i := range n {
		rec := 12 + 12*i
		settings, count := int(font.Be32(b, rec+4)), font.Be16(b, rec+2)
		if settings > len(b) || len(b)-settings < 4*count {
			return nil
		}
	}
	return aatFeat(b)
}

// feature is get_feature: the record of a feature type, by binary search, and
// whether the table names it with any settings — FeatureName::has_data.
func (t aatFeat) feature(typ int) (rec int, ok bool) {
	if t == nil {
		return 0, false
	}
	lo, hi := 0, font.Be16(t, 4)-1
	for lo <= hi {
		mid := int(uint(lo+hi) >> 1)
		r := 12 + 12*mid
		switch f := font.Be16(t, r); {
		case typ < f:
			hi = mid - 1
		case typ > f:
			lo = mid + 1
		default:
			return r, font.Be16(t, r+2) != 0
		}
	}
	return 0, false
}

// exclusive is whether a feature's settings exclude one another.
func (t aatFeat) exclusive(rec int) bool { return font.Be16(t, rec+8)&0x8000 != 0 }

// aatSetting is a feature type and setting a caller's request comes to:
// hb_aat_map_builder_t::feature_info_t.
type aatSetting struct {
	typ, setting int
	exclusive    bool
	seq          int
}

// aatSettings is what a run's requests come to for a face's morx: each tag,
// settled, mapped to its type and setting where the face's feat table offers
// the type (add_feature), sorted and one kept a type or a setting pair, the
// first asked for (compile). Nil for a face with no feat table and for a run
// that asks for nothing it offers, which is every chain's default flags.
func (f *Face) aatSettings(user []userFeature) []aatSetting {
	if f.feat == nil || len(user) == 0 {
		return nil
	}
	var out []aatSetting
	for _, u := range settleFeatures(user) {
		value := 0
		if u.on {
			value = 1
		}
		if u.tag == "aalt" {
			if _, ok := f.feat.feature(aatCharacterAlternatives); ok {
				out = append(out, aatSetting{aatCharacterAlternatives, value, true, len(out) + 1})
			}
			continue
		}
		m, ok := findAATMapping(u.tag)
		if !ok {
			continue
		}
		rec, ok := f.feat.feature(m.typ)
		if !ok {
			// Small capitals may be offered under their deprecated type
			// alone, which a chain's own retry then finds.
			if m.typ != aatLowerCase || m.enable != aatLowerCaseSmallCaps {
				continue
			}
			if rec, ok = f.feat.feature(aatLetterCase); !ok {
				continue
			}
		}
		setting := m.dflt
		if u.on {
			setting = m.enable
		}
		out = append(out, aatSetting{m.typ, setting, f.feat.exclusive(rec), len(out) + 1})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.typ != b.typ {
			return a.typ < b.typ
		}
		if !a.exclusive && a.setting&^1 != b.setting&^1 {
			return a.setting < b.setting
		}
		return a.seq < b.seq
	})
	// One a type, or one a setting pair — a type's even and odd settings turn
	// one thing on and off — the first asked for.
	j := 0
	for i := 1; i < len(out); i++ {
		if out[i].typ != out[j].typ || !out[i].exclusive && out[i].setting&^1 != out[j].setting&^1 {
			j++
			out[j] = out[i]
		}
	}
	return out[:j+1]
}

// settleFeatures is each tag of a run's requests once, as the last request for
// it says, in the order of those last requests.
func settleFeatures(user []userFeature) []userFeature {
	last := map[string]int{}
	for i, u := range user {
		last[u.tag] = i
	}
	out := make([]userFeature, 0, len(last))
	for i, u := range user {
		if last[u.tag] == i {
			out = append(out, u)
		}
	}
	return out
}

// flagsFor is Chain::compile_flags: the chain's default flags, changed by each
// of its features a caller's settings ask for — a chain naming small capitals
// by their deprecated type is asked by the current one — and by each that names
// a language tag of ltag the run's language matches.
func (c *morxChain) flagsFor(settings []aatSetting, lang aatLanguage, ltag []aatLanguage) uint32 {
	flags := c.defaultFlags
	asked := func(typ, setting int) bool {
		for _, s := range settings {
			if s.typ == typ && s.setting == setting {
				return true
			}
		}
		return false
	}
	for _, fe := range c.features {
		if asked(fe.typ, fe.setting) ||
			fe.typ == aatLetterCase && fe.setting == aatLetterCaseSmallCaps && asked(aatLowerCase, aatLowerCaseSmallCaps) ||
			fe.typ == aatLanguageTag && fe.setting != 0 && hbLanguageMatches(ltagAt(ltag, fe.setting-1), lang) {
			flags &= fe.disable
			flags |= fe.enable
		}
	}
	return flags
}

// aatLanguage is an hb_language_t: a language tag in HarfBuzz's canonical
// form, or, where valid is false, HB_LANGUAGE_INVALID, which is what a run
// with no language has.
type aatLanguage struct {
	tag   string
	valid bool
}

// hbLanguageMax is how many bytes of a tag hbLanguage keeps.
const hbLanguageMax = 63

// hbLanguage is hb_language_from_string: no language for an empty tag; else
// the first 63 bytes, each letter lowered and an underscore made a hyphen, cut
// at the first byte that is none of letter, digit, hyphen or underscore.
func hbLanguage(s string) aatLanguage {
	if s == "" || s[0] == 0 {
		return aatLanguage{}
	}
	if len(s) > hbLanguageMax {
		s = s[:hbLanguageMax]
	}
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', '0' <= c && c <= '9', c == '-':
		case 'A' <= c && c <= 'Z':
			c += 'a' - 'A'
		case c == '_':
			c = '-'
		default:
			return aatLanguage{tag: string(b), valid: true}
		}
		b = append(b, c)
	}
	return aatLanguage{tag: string(b), valid: true}
}

// hbLanguageMatches is hb_language_matches: whether specific is lang, or a more
// specific form of it, lang followed by a hyphen. Two runs with no language
// match each other, and nothing else.
func hbLanguageMatches(lang, specific aatLanguage) bool {
	if lang == specific {
		return true
	}
	if !lang.valid || !specific.valid {
		return false
	}
	l, s := lang.tag, specific.tag
	return len(l) <= len(s) && s[:len(l)] == l && (len(s) == len(l) || s[len(l)] == '-')
}

// ltagAt is ltag::get_language: the i-th tag, and no language for one the
// table does not have.
func ltagAt(ltag []aatLanguage, i int) aatLanguage {
	if i < 0 || i >= len(ltag) {
		return aatLanguage{}
	}
	return ltag[i]
}

// readLtag reads an ltag table's language tags, and none for a table
// HarfBuzz's sanitizer refuses: a version before 1, or a tag whose bytes are
// not inside the table.
//
// A tag finds its bytes by an offset, and every tag may point at one string of
// up to 65,535 bytes. Each is cut to what hbLanguage keeps before it is copied:
// copied whole, n tags on one string cost n times its length, quadratic in the
// table, and a 76 KB font made Load allocate 250 MB. hbLanguage reads only the
// first byte and the first hbLanguageMax, so the cut tag reads as the whole.
func readLtag(b []byte) []aatLanguage {
	if len(b) < 12 || font.Be32(b, 0) < 1 {
		return nil
	}
	n := int(font.Be32(b, 8))
	if n > (len(b)-12)/4 {
		return nil
	}
	out := make([]aatLanguage, n)
	for i := range out {
		off, length := font.Be16(b, 12+4*i), font.Be16(b, 14+4*i)
		if off > len(b) || length > len(b)-off {
			return nil
		}
		out[i] = hbLanguage(string(b[off : off+min(length, hbLanguageMax)]))
	}
	return out
}
