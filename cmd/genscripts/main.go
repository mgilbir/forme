// Command genscripts generates the Unicode script ranges, the scripts each
// character is also used with, and the OpenType script tags each script
// selects, from Unicode's own Scripts.txt, ScriptExtensions.txt and
// PropertyValueAliases.txt.
//
// A font's GSUB and GPOS tables state their rules per script: a Greek run must
// be given the features 'grek' declares and not the ones 'arab' does. Which
// script a character belongs to is Unicode's to say, so it is read from the UCD
// rather than guessed; which four-byte tag OpenType calls that script by is the
// OpenType registry's to say, and is derived here.
//
// The derivation is the one every shaper uses: an OpenType script tag is the
// character's ISO 15924 code with its first letter lowercased — Grek becomes
// 'grek', Deva becomes 'deva'. Only a handful of scripts break that rule, and
// they are listed below. The Indic scripts carry two tags, an older one and a
// second-generation one a font declares when it wants the reordering rules a
// modern shaper applies; the newer is tried first, which is what a shaper does.
//
// A character's Script is one script, and some characters are written in
// several: the Devanagari digits in Kaithi, the Tamil ones in Grantha, the
// Arabic full stop in Hanifi Rohingya. ScriptExtensions.txt lists those, and a
// run of text is cut where its script changes by reading them (UAX #24), so
// that a Kaithi word with a digit in it stays one run.
//
//	go run ./cmd/genscripts -version <X.Y.Z> <Scripts.txt> <ScriptExtensions.txt> <PropertyValueAliases.txt> > shape/scripts.go
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mgilbir/forme/cmd/internal/ucd"
	"github.com/mgilbir/forme/internal/ascii"
)

// otTagOverrides names the scripts whose OpenType tag is not their ISO 15924
// code lowercased, and those that carry more than one tag.
//
// The key is the Unicode script's long name, and the generator fails if one is
// not in the data — so a script renamed or removed upstream is noticed here
// rather than silently losing its tag.
var otTagOverrides = map[string][]string{
	// OpenType has one tag for the two Japanese kana scripts, and it is
	// Katakana's. Hiragana's own code, 'hira', names nothing in any font.
	"Hiragana": {"kana"},

	// ISO 15924 pads a short code with nothing; OpenType pads it with spaces,
	// because a tag is always four bytes.
	"Lao": {"lao "},
	"Nko": {"nko "},
	"Vai": {"vai "},
	"Yi":  {"yi  "},

	// The Indic scripts each have a second-generation tag. A font declares it
	// to say its rules are written for a shaper that reorders, which is what
	// shape/indic.go is; the tag is the one such a font declares its features
	// under, so it is tried first and the older one after.
	"Bengali":    {"bng2", "beng"},
	"Devanagari": {"dev2", "deva"},
	"Gujarati":   {"gjr2", "gujr"},
	"Gurmukhi":   {"gur2", "guru"},
	"Kannada":    {"knd2", "knda"},
	"Malayalam":  {"mlm2", "mlym"},
	"Myanmar":    {"mym2", "mymr"},
	"Oriya":      {"ory2", "orya"},
	"Tamil":      {"tml2", "taml"},
	"Telugu":     {"tel2", "telu"},
}

// noTag are the scripts that select no tag of their own: they are not scripts a
// font declares rules for. Text in them takes the default script, which is what
// a run of digits and punctuation should get.
var noTag = map[string]bool{"Common": true, "Inherited": true, "Unknown": true}

func main() {
	version := flag.String("version", "", "the Unicode version the files came from")
	flag.Parse()
	args := flag.Args()
	if len(args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: genscripts -version <X.Y.Z> <Scripts.txt> <ScriptExtensions.txt> <PropertyValueAliases.txt>")
		os.Exit(2)
	}
	if err := ucd.Check(*version, args...); err != nil {
		fmt.Fprintln(os.Stderr, "genscripts:", err)
		os.Exit(1)
	}
	ranges := readScripts(args[0])
	extensions := readScripts(args[1])
	codes := readAliases(args[2])

	// Every script the ranges name, in a stable order, with Common, Inherited
	// and Unknown first so the reader can name them as constants.
	names := map[string]bool{}
	for _, r := range ranges {
		names[r.name] = true
	}
	for n := range noTag {
		names[n] = true
	}
	ordered := []string{"Common", "Inherited", "Unknown"}
	rest := make([]string, 0, len(names))
	for n := range names {
		if !noTag[n] {
			rest = append(rest, n)
		}
	}
	sort.Strings(rest)
	ordered = append(ordered, rest...)

	index := map[string]int{}
	for i, n := range ordered {
		index[n] = i
	}

	// Tags per script, with the overrides checked against the data.
	for n := range otTagOverrides {
		if !names[n] {
			fmt.Fprintf(os.Stderr, "genscripts: override names script %q, which the data does not have\n", n)
			os.Exit(1)
		}
	}
	tags := make([][]string, len(ordered))
	for i, n := range ordered {
		switch {
		case noTag[n]:
			tags[i] = nil
		case otTagOverrides[n] != nil:
			tags[i] = otTagOverrides[n]
		default:
			code, ok := codes[n]
			if !ok {
				fmt.Fprintf(os.Stderr, "genscripts: no ISO 15924 code for script %q\n", n)
				os.Exit(1)
			}
			tags[i] = []string{ascii.Lower(code[:1]) + code[1:]}
		}
	}

	// Collapse to ranges. A script runs in long blocks, so a range table is a
	// couple of thousand entries where a per-character map would be a million.
	sort.Slice(ranges, func(i, j int) bool { return ranges[i].lo < ranges[j].lo })
	var merged []scriptRange
	for _, r := range ranges {
		if n := len(merged); n > 0 && merged[n-1].name == r.name && merged[n-1].hi+1 == r.lo {
			merged[n-1].hi = r.hi
			continue
		}
		merged = append(merged, r)
	}

	// The extensions name scripts by their ISO 15924 codes, several to a line.
	// Each is turned into the index above, and a code the data does not have
	// stops the generator: a script dropped from the list would otherwise be
	// dropped from every character that names it.
	byCode := map[string]string{}
	for name, code := range codes {
		byCode[code] = name
	}
	sort.Slice(extensions, func(i, j int) bool { return extensions[i].lo < extensions[j].lo })
	type extRange struct {
		lo, hi  rune
		scripts []int
		names   string
	}
	var exts []extRange
	widest := 0
	for _, r := range extensions {
		var idx []int
		for _, code := range strings.Fields(r.name) {
			name, ok := byCode[code]
			if !ok || !names[name] {
				fmt.Fprintf(os.Stderr, "genscripts: ScriptExtensions.txt names %q, which is no script here\n", code)
				os.Exit(1)
			}
			idx = append(idx, index[name])
		}
		if len(idx) == 0 {
			fmt.Fprintf(os.Stderr, "genscripts: a ScriptExtensions.txt line for %04X names no script\n", r.lo)
			os.Exit(1)
		}
		if len(idx) > widest {
			widest = len(idx)
		}
		if n := len(exts); n > 0 && exts[n-1].names == r.name && exts[n-1].hi+1 == r.lo {
			exts[n-1].hi = r.hi
			continue
		}
		if n := len(exts); n > 0 && exts[n-1].hi >= r.lo {
			fmt.Fprintf(os.Stderr, "genscripts: ScriptExtensions.txt lists %04X twice\n", r.lo)
			os.Exit(1)
		}
		exts = append(exts, extRange{lo: r.lo, hi: r.hi, scripts: idx, names: r.name})
	}

	// The table is written into a buffer and formatted before it is emitted, so
	// that the committed file is gofmt-clean however the generator was run.
	w := &bytes.Buffer{}
	fmt.Fprintf(w, `// Code generated by cmd/genscripts from Unicode's Scripts.txt,
// ScriptExtensions.txt and PropertyValueAliases.txt. DO NOT EDIT.

package shape

// Unicode scripts, and the OpenType script tags each one selects. Unicode %s.
//
// A font states its layout rules per script, so shaping a run has to know which
// script the run is in — that is Unicode's Script property, and these %d ranges
// are it. A character no range names is of unknown script, which selects the
// default tag, as do Common and Inherited: a digit, a space and a combining
// accent are not text in a script of their own and take the script of what they
// are written among.
//
// A script's tags are in the order a shaper tries them, which matters only for
// the Indic scripts, where the second-generation tag comes first.

type scriptRange struct {
	lo, hi rune
	script uint16
}

// The three scripts that decide nothing, at fixed indices so the resolver can
// name them.
const (
	scriptCommon    = 0
	scriptInherited = 1
	scriptUnknown   = 2
)

// scriptOpenTypeTags gives the tags each script selects, indexed as
// scriptRanges indexes scripts. A nil entry selects no tag of its own.
var scriptOpenTypeTags = [...][]string{
`, *version, len(merged))
	for i, n := range ordered {
		if tags[i] == nil {
			fmt.Fprintf(w, "\t%d: nil, // %s\n", i, n)
			continue
		}
		quoted := make([]string, len(tags[i]))
		for k, t := range tags[i] {
			quoted[k] = strconv.Quote(t)
		}
		fmt.Fprintf(w, "\t%d: {%s}, // %s\n", i, strings.Join(quoted, ", "), n)
	}
	fmt.Fprint(w, "}\n\n// scriptRanges maps a character to its script, sorted by code point.\nvar scriptRanges = [...]scriptRange{\n")
	for _, r := range merged {
		fmt.Fprintf(w, "\t{0x%04X, 0x%04X, %d}, // %s\n", r.lo, r.hi, index[r.name], r.name)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintf(w, `
// scriptExtensionRange is a range of characters used in more than one script,
// and the scripts, indexed as scriptOpenTypeTags is.
type scriptExtensionRange struct {
	lo, hi  rune
	scripts []uint16
}

// maxScriptExtensions is the most scripts any character is used in.
const maxScriptExtensions = %d

// scriptExtensionRanges is Unicode's Script_Extensions, for the %d ranges of
// characters that have one other than their Script, sorted by code point. A
// character in none of them is used in its Script alone.
var scriptExtensionRanges = [...]scriptExtensionRange{
`, widest, len(exts))
	for _, r := range exts {
		q := make([]string, len(r.scripts))
		for k, i := range r.scripts {
			q[k] = strconv.Itoa(i)
		}
		fmt.Fprintf(w, "\t{0x%04X, 0x%04X, []uint16{%s}}, // %s\n", r.lo, r.hi, strings.Join(q, ", "), r.names)
	}
	fmt.Fprintln(w, "}")

	src, err := format.Source(w.Bytes())
	if err != nil {
		fmt.Fprintln(os.Stderr, "genscripts:", err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(src); err != nil {
		fmt.Fprintln(os.Stderr, "genscripts:", err)
		os.Exit(1)
	}
}

type scriptRange struct {
	lo, hi rune
	name   string
}

// readScripts parses Scripts.txt, or ScriptExtensions.txt, which is written the
// same way: one entry per range it lists, with the second field whole — a
// script's name in the one, a list of codes in the other.
func readScripts(path string) []scriptRange {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var out []scriptRange
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 2 {
			continue
		}
		lo, hi, ok := parseRange(strings.TrimSpace(fields[0]))
		if !ok {
			continue
		}
		out = append(out, scriptRange{lo: lo, hi: hi, name: strings.TrimSpace(fields[1])})
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return out
}

// parseRange reads "0041..005A" or "0041".
func parseRange(s string) (rune, rune, bool) {
	lo, hi := s, s
	if i := strings.Index(s, ".."); i >= 0 {
		lo, hi = s[:i], s[i+2:]
	}
	a, err := strconv.ParseUint(lo, 16, 32)
	if err != nil {
		return 0, 0, false
	}
	b, err := strconv.ParseUint(hi, 16, 32)
	if err != nil {
		return 0, 0, false
	}
	return rune(a), rune(b), true
}

// readAliases parses the sc property of PropertyValueAliases.txt: the ISO 15924
// code for each script's long name.
func readAliases(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Split(line, ";")
		if len(fields) < 3 || strings.TrimSpace(fields[0]) != "sc" {
			continue
		}
		out[strings.TrimSpace(fields[2])] = strings.TrimSpace(fields[1])
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	return out
}
