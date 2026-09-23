// Command genlangtags turns HarfBuzz's table of BCP 47 language tags into the
// OpenType language system tags each selects, for package shape.
//
// Which language system of a font a run is set in is a question about the
// run's language, and a document states that as a BCP 47 tag — lang="sr",
// "zh-Hant-HK", "ro" — where a font names its language systems by the OpenType
// registry's four-letter tags: 'SRB ', 'ZHH ', 'ROM '. The mapping is two
// registries joined, the OpenType one and IANA's, with a long list of
// corrections for the places the two disagree; HarfBuzz keeps that join as a
// generated header, and fonts are tested against HarfBuzz. So this reads that
// header rather than joining the registries again and arriving at a different
// answer for the corrections.
//
// The header is taken at a pinned HarfBuzz release and checked against a pinned
// SHA-256, which the Makefile's fetch checks too and the generated file records:
//
//	make language-tags
//	go run ./cmd/genlangtags -source <url> -sha256 <hex> -in testdata/harfbuzz-langtags/hb-ot-tag-table.hh -out shape/langtags.go
//
// What it reads, and refuses to guess at:
//
//   - ot_languages2 and ot_languages3, the tags a two- or three-letter primary
//     subtag selects, in HarfBuzz's order;
//   - ot_languages3_multi, the three-letter subtags that select more than one;
//   - ot_languages3_blocked, the three-letter subtags that select none, which is
//     not the same as being absent — an absent one is used as a tag itself;
//   - hb_ot_tags_from_complex_language, the rules that read more than the
//     primary subtag (a script, a region, a variant), in the order HarfBuzz
//     tries them. That one is C, not a table, and it is read by its four
//     condition shapes; a statement of any other shape stops the generator
//     rather than being skipped, so a HarfBuzz that writes a fifth shape cannot
//     produce a table that quietly lacks its rules.
//
// HarfBuzz is under the MIT ("Old MIT") licence, as its COPYING says.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"regexp"
	"sort"
	"strings"
)

// tagRE is one HB_TAG(...) with its four characters.
var tagRE = regexp.MustCompile(`HB_TAG\('(.)','(.)','(.)','(.)'\)`)

func tagOf(m []string) string { return m[1] + m[2] + m[3] + m[4] }

// block returns the text between "static const <type> <name>[] = {" and the
// "};" that closes it.
func block(src, decl string) string {
	i := strings.Index(src, decl)
	if i < 0 {
		log.Fatalf("the header has no %q", decl)
	}
	rest := src[i+len(decl):]
	j := strings.Index(rest, "\n};")
	if j < 0 {
		log.Fatalf("%q is not closed", decl)
	}
	return rest[:j]
}

type entry struct {
	lang string
	tags []string
}

// pairs reads a LangTag array: {HB_TAG(language), HB_TAG(tag)} per line, several
// lines for a language that selects several tags, in the order they are tried.
func pairs(body string) []entry {
	var out []entry
	for _, line := range strings.Split(body, "\n") {
		ms := tagRE.FindAllStringSubmatch(line, -1)
		if len(ms) == 0 {
			continue
		}
		if len(ms) != 2 {
			log.Fatalf("a table line with %d tags: %q", len(ms), line)
		}
		lang := strings.TrimRight(tagOf(ms[0]), " ")
		tag := tagOf(ms[1])
		if n := len(out); n > 0 && out[n-1].lang == lang {
			out[n-1].tags = append(out[n-1].tags, tag)
			continue
		}
		out = append(out, entry{lang: lang, tags: []string{tag}})
	}
	return out
}

// rule is one of hb_ot_tags_from_complex_language's.
type rule struct {
	kind   string
	first  byte
	spec   string
	subtag string
	tags   []string
	note   string
}

var (
	caseRE   = regexp.MustCompile(`^  case '(.)':$`)
	anyRE    = regexp.MustCompile(`^    if \(subtag_matches \(p, limit, "([^"]+)", (\d+)\)\)$`)
	exactRE  = regexp.MustCompile(`^    if \(0 == strcmp \(&lang_str\[1\], "([^"]+)"\)\)$`)
	prefixRE = regexp.MustCompile(`^    if \(lang_matches \(&lang_str\[1\], limit, "([^"]+)", (\d+)\)\)$`)
	startRE  = regexp.MustCompile(`^    if \(0 == strncmp \(&lang_str\[1\], "([^"]+)", (\d+)\)$`)
	andSubRE = regexp.MustCompile(`^\t&& subtag_matches \(lang_str, limit, "([^"]+)", (\d+)\)\)$`)
	noteRE   = regexp.MustCompile(`^      /\* (.*) \*/$`)
	loopRE   = regexp.MustCompile(`^      for \(i = 0; i < (\d+) && i < \*count; i\+\+\)$`)
)

// complexRules reads the function line by line. Its shape is HarfBuzz's
// generator's and regular: a guarded block of rules that look for a subtag
// anywhere, then a switch on the first letter whose cases are rules that match
// the rest of the tag. Each rule is an if with one of four conditions, a
// comment, and either one tag or a list of them.
func complexRules(fn string, guards map[string]int) []rule {
	lines := strings.Split(fn, "\n")
	var (
		out      []rule
		first    byte
		cur      *rule
		inSwitch bool
		inList   bool
		count    string
	)
	check := func(n string, s string) {
		if fmt.Sprint(len(s)) != n {
			log.Fatalf("%q is given length %s", s, n)
		}
	}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		t := strings.TrimSpace(l)
		switch {
		case t == "switch (lang_str[0])":
			inSwitch = true
			continue
		case caseRE.MatchString(l):
			first = caseRE.FindStringSubmatch(l)[1][0]
			continue
		}
		if m := anyRE.FindStringSubmatch(l); m != nil && !inSwitch {
			check(m[2], m[1])
			out = append(out, rule{kind: "subtag", spec: m[1]})
			cur = &out[len(out)-1]
			continue
		}
		if m := exactRE.FindStringSubmatch(l); m != nil && inSwitch {
			out = append(out, rule{kind: "exact", first: first, spec: m[1]})
			cur = &out[len(out)-1]
			continue
		}
		if m := prefixRE.FindStringSubmatch(l); m != nil && inSwitch {
			check(m[2], m[1])
			out = append(out, rule{kind: "prefix", first: first, spec: m[1]})
			cur = &out[len(out)-1]
			continue
		}
		if m := startRE.FindStringSubmatch(l); m != nil && inSwitch {
			check(m[2], m[1])
			if i+1 >= len(lines) {
				log.Fatal("a strncmp condition at the end of the function")
			}
			s := andSubRE.FindStringSubmatch(lines[i+1])
			if s == nil {
				log.Fatalf("a strncmp condition not followed by a subtag test: %q", lines[i+1])
			}
			check(s[2], s[1])
			out = append(out, rule{kind: "start", first: first, spec: m[1], subtag: s[1]})
			cur = &out[len(out)-1]
			i++
			continue
		}
		if strings.HasPrefix(t, "if (") {
			switch t {
			case "if (limit - lang_str >= 7)", "if (!p || p >= limit || limit - p < 5) goto out;":
				// The guards on the rules that look for a subtag anywhere,
				// which shape/language.go applies in code of its own. main
				// checks each appears exactly once, so that the guards written
				// there are still HarfBuzz's.
				guards[t]++
				continue
			}
			log.Fatalf("a condition this generator does not read: %q", l)
		}
		if cur == nil {
			continue
		}
		if m := noteRE.FindStringSubmatch(l); m != nil && cur.note == "" && len(cur.tags) == 0 {
			cur.note = m[1]
			continue
		}
		switch {
		case strings.HasPrefix(t, "hb_tag_t possible_tags[] = {"):
			inList = true
		case inList && t == "};":
			inList = false
		case inList:
			m := tagRE.FindStringSubmatch(l)
			if m == nil {
				log.Fatalf("a possible_tags line with no tag: %q", l)
			}
			cur.tags = append(cur.tags, tagOf(m))
		case strings.HasPrefix(t, "tags[0] = "):
			m := tagRE.FindStringSubmatch(l)
			if m == nil {
				log.Fatalf("a tag assignment with no tag: %q", l)
			}
			cur.tags = append(cur.tags, tagOf(m))
		case loopRE.MatchString(l):
			count = loopRE.FindStringSubmatch(l)[1]
		case t == "return true;":
			if len(cur.tags) == 0 {
				log.Fatalf("a rule for %q with no tags", cur.spec)
			}
			if count != "" && count != fmt.Sprint(len(cur.tags)) {
				log.Fatalf("a rule for %q lists %d tags and copies %s", cur.spec, len(cur.tags), count)
			}
			cur, count = nil, ""
		}
	}
	if cur != nil {
		log.Fatalf("the rule for %q never returns", cur.spec)
	}
	return out
}

func main() {
	in := flag.String("in", "testdata/harfbuzz-langtags/hb-ot-tag-table.hh", "HarfBuzz's hb-ot-tag-table.hh")
	outPath := flag.String("out", "shape/langtags.go", "the Go file to write")
	source := flag.String("source", "", "the URL the file was fetched from")
	pin := flag.String("sha256", "", "the SHA-256 the file is pinned to")
	flag.Parse()
	if *source == "" || *pin == "" {
		log.Fatal("no -source or -sha256: the table has to say which header it is from")
	}
	raw, err := os.ReadFile(*in)
	if err != nil {
		log.Fatalf("reading %s: %v (run `make language-tags`)", *in, err)
	}
	if sum := sha256.Sum256(raw); hex.EncodeToString(sum[:]) != *pin {
		log.Fatalf("%s has SHA-256 %x, and the table is pinned to %s", *in, sum, *pin)
	}
	src := string(raw)

	two := pairs(block(src, "static const LangTag ot_languages2[] = {"))
	three := pairs(block(src, "static const LangTag ot_languages3[] = {"))
	// HarfBuzz takes one tag from this table, whichever row its search lands
	// on, so a subtag with two rows here would be a table it reads differently
	// from the way this one would be read.
	for _, e := range three {
		if len(e.tags) != 1 {
			log.Fatalf("ot_languages3 gives %q %d tags; HarfBuzz reads one", e.lang, len(e.tags))
		}
	}
	var blocked []string
	for _, m := range tagRE.FindAllStringSubmatch(block(src, "static const hb_tag_t ot_languages3_blocked[] = {"), -1) {
		blocked = append(blocked, strings.TrimRight(tagOf(m), " "))
	}
	var values []string
	for _, m := range tagRE.FindAllStringSubmatch(block(src, "static const hb_tag_t ot_languages3_multi_values[] = {"), -1) {
		values = append(values, tagOf(m))
	}
	multiRE := regexp.MustCompile(`\{HB_TAG\('(.)','(.)','(.)','(.)'\),\s*(\d+),\s*(\d+)\}`)
	for _, m := range multiRE.FindAllStringSubmatch(block(src, "static const LangTagRange ot_languages3_multi[] = {"), -1) {
		var off, n int
		fmt.Sscan(m[5], &off)
		fmt.Sscan(m[6], &n)
		if off+n > len(values) {
			log.Fatalf("a multi range past the end of its values: %v", m[0])
		}
		three = append(three, entry{lang: strings.TrimRight(tagOf(m[:5]), " "), tags: values[off : off+n]})
	}
	sort.SliceStable(three, func(i, j int) bool { return three[i].lang < three[j].lang })
	for i := 1; i < len(three); i++ {
		if three[i].lang == three[i-1].lang {
			log.Fatalf("%q is in both the single and the multiple table", three[i].lang)
		}
	}
	for i := 1; i < len(two); i++ {
		if two[i].lang <= two[i-1].lang {
			log.Fatalf("ot_languages2 is not sorted at %q", two[i].lang)
		}
	}
	sort.Strings(blocked)

	fnStart := strings.Index(src, "hb_ot_tags_from_complex_language (const char")
	if fnStart < 0 {
		log.Fatal("the header has no hb_ot_tags_from_complex_language")
	}
	fnEnd := strings.Index(src[fnStart:], "\n}\n")
	if fnEnd < 0 {
		log.Fatal("hb_ot_tags_from_complex_language is not closed")
	}
	guards := map[string]int{}
	rules := complexRules(src[fnStart:fnStart+fnEnd], guards)
	for _, g := range []string{"if (limit - lang_str >= 7)", "if (!p || p >= limit || limit - p < 5) goto out;"} {
		if guards[g] != 1 {
			log.Fatalf("hb_ot_tags_from_complex_language has %q %d times, and the matcher in "+
				"package shape is written for exactly one", g, guards[g])
		}
	}
	for i, r := range rules {
		if r.kind == "subtag" && i > 0 && rules[i-1].kind != "subtag" {
			log.Fatalf("a rule looking for %q anywhere after the switch; package shape tries "+
				"those first", r.spec)
		}
	}
	returns := strings.Count(src[fnStart:fnStart+fnEnd], "return true;")
	if returns != len(rules) {
		log.Fatalf("read %d rules and the function returns true %d times", len(rules), returns)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "// Code generated by cmd/genlangtags from HarfBuzz's hb-ot-tag-table.hh. DO NOT EDIT.\n")
	fmt.Fprintf(&b, "//\n// Source: %s\n// SHA-256: %s\n//\n", *source, *pin)
	fmt.Fprintf(&b, "// HarfBuzz is under the MIT (\"Old MIT\") licence; see its COPYING.\n\npackage shape\n\n")
	fmt.Fprintf(&b, "// langTags2 is the OpenType language system tags each two-letter primary\n")
	fmt.Fprintf(&b, "// subtag selects, most specific first, sorted by subtag.\n")
	fmt.Fprintf(&b, "var langTags2 = [...]langTagEntry{\n")
	for _, e := range two {
		fmt.Fprintf(&b, "\t{%q, %s},\n", e.lang, goTags(e.tags))
	}
	fmt.Fprintf(&b, "}\n\n// langTags3 is the same for the three-letter subtags.\n")
	fmt.Fprintf(&b, "var langTags3 = [...]langTagEntry{\n")
	for _, e := range three {
		fmt.Fprintf(&b, "\t{%q, %s},\n", e.lang, goTags(e.tags))
	}
	fmt.Fprintf(&b, "}\n\n// langTags3Blocked are the three-letter subtags that select no language system\n")
	fmt.Fprintf(&b, "// at all, sorted.\n")
	fmt.Fprintf(&b, "var langTags3Blocked = [...]string{\n")
	for _, s := range blocked {
		fmt.Fprintf(&b, "\t%q,\n", s)
	}
	fmt.Fprintf(&b, "}\n\n// langRules are the rules that read more of a tag than its primary subtag,\n")
	fmt.Fprintf(&b, "// in the order they are tried.\n")
	fmt.Fprintf(&b, "var langRules = [...]langRule{\n")
	for _, r := range rules {
		kind := map[string]string{"subtag": "langRuleSubtag", "exact": "langRuleExact",
			"prefix": "langRulePrefix", "start": "langRuleStartAndSubtag"}[r.kind]
		first := "0"
		if r.first != 0 {
			first = fmt.Sprintf("'%c'", r.first)
		}
		fmt.Fprintf(&b, "\t{%s, %s, %q, %q, %s}, // %s\n", kind, first, r.spec, r.subtag, goTags(r.tags), r.note)
	}
	fmt.Fprintf(&b, "}\n")

	// Through gofmt, so that running this again over the same header writes
	// the same file byte for byte and a stale table is a diff.
	src2, err := format.Source([]byte(b.String()))
	if err != nil {
		log.Fatalf("the generated source does not parse: %v", err)
	}
	if err := os.WriteFile(*outPath, src2, 0o644); err != nil {
		log.Fatal(err)
	}
}

// maxTags is HB_OT_MAX_TAGS_PER_LANGUAGE, how many language system tags
// HarfBuzz's shaper asks for. A row with more would be read in part there, and
// in whole by a reader of this table that did not know to stop.
const maxTags = 3

func goTags(tags []string) string {
	if len(tags) > maxTags {
		log.Fatalf("a language selects %d tags, %v, and HarfBuzz reads %d", len(tags), tags, maxTags)
	}
	q := make([]string, len(tags))
	for i, t := range tags {
		q[i] = fmt.Sprintf("%q", t)
	}
	return "[]string{" + strings.Join(q, ", ") + "}"
}
