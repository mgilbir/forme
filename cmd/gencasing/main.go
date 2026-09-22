// Command gencasing generates Unicode's case mappings, from Unicode's own
// UnicodeData.txt and SpecialCasing.txt: the simple ones, which map one
// character to one character, and the full ones, which turn one character into
// more than one.
//
// Go's strings.ToUpper and its neighbours apply the *simple* mappings of
// UnicodeData.txt, which are one character to one character by construction. The
// case of a great deal of ordinary text is not: "straße" uppercases to "STRASSE"
// in every dictionary and every browser, and a one-to-one mapping cannot say so,
// so Go leaves the ß alone and produces "STRAßE". CSS Text §2.1.1 asks for the
// full mappings by name — "the Unicode Default Case Conversion algorithm" — and
// this is the table they need.
//
// # Why the simple mappings are generated too
//
// They used to be Go's. The full table held only the mappings that differ from
// the simple one, and the simple one was whatever package unicode said — which
// is the release the Go toolchain shipped, 15.0.0 for Go 1.26, while this table
// said 17.0.0. The hundred and ten characters Unicode 16 and 17 gave case mappings were in
// neither: "ᲊ" and "ꟍ" came back from "text-transform: uppercase" as they went
// in, under a header naming the release that gave them capitals. The table also
// depended on the toolchain that ran the generator, so a Go release that
// updated package unicode would have changed what the drift test expects.
//
// So both halves come from the release the header names, and the one the
// consumer asks is this one: every character UnicodeData.txt gives a simple
// mapping to is in a simple table, and every character SpecialCasing.txt gives
// an unconditional full mapping to that is not that simple mapping is in a full
// table. A character in neither maps to itself.
//
// # What SpecialCasing.txt says and what is left out
//
// It states each mapping as
//
//	<code>; <lower>; <title>; <upper>; (<condition>;)? # <name>
//
// and the conditional entries are left out here, for two different reasons.
// Three of the conditions are language tags — Turkish and Azeri's dotted and
// dotless i, Lithuanian's retained dot — and a language-tailored mapping needs
// the element's language, which is a question about the document rather than
// about a character. The fourth is Final_Sigma, which needs the characters
// around the one being mapped: a lowercase sigma ending a word is ς and one
// inside a word is σ. Neither is a table of this shape, and pretending
// otherwise would apply a Turkish rule to English; paragraph/localecasing.go
// applies all four.
//
//	go run ./cmd/gencasing -version <X.Y.Z> <UnicodeData.txt> <SpecialCasing.txt> > paragraph/casingtable.go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mgilbir/forme/cmd/internal/ucd"
)

// entry is one character's full mapping in one of the three cases.
type entry struct {
	r rune
	s string
}

// pair is one character's simple mapping in one of the three cases.
type pair struct {
	r, to rune
}

func main() {
	version := flag.String("version", "", "the Unicode version the files came from")
	flag.Parse()
	args := flag.Args()
	if len(args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gencasing -version <X.Y.Z> <UnicodeData.txt> <SpecialCasing.txt>")
		os.Exit(2)
	}
	if err := ucd.Check(*version, args...); err != nil {
		fail(err.Error())
	}
	chars, err := ucd.UnicodeData(args[0])
	if err != nil {
		fail(err.Error())
	}

	// The simple mappings, one table per case. A character's mapping is listed
	// wherever it is not the character itself, which for the titlecase is
	// UnicodeData.txt's uppercase when the titlecase field is empty — see
	// ucd.Char.
	var simple [3][]pair
	for r, c := range chars {
		for i, to := range [3]rune{c.Lower, c.Title, c.Upper} {
			if to != 0 && to != r {
				simple[i] = append(simple[i], pair{r, to})
			}
		}
	}
	simpleOf := func(i int, r rune) rune {
		c := chars[r]
		if to := [3]rune{c.Lower, c.Title, c.Upper}[i]; to != 0 {
			return to
		}
		return r
	}

	f, err := os.Open(args[1])
	if err != nil {
		fail(err.Error())
	}
	defer f.Close()

	var full [3][]entry
	var conditional int
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if i := strings.IndexByte(text, '#'); i >= 0 {
			text = text[:i]
		}
		fields := strings.Split(text, ";")
		// Four fields and a trailing empty one from the final semicolon. A fifth
		// non-empty field is a condition.
		if len(fields) < 5 || strings.TrimSpace(fields[0]) == "" {
			continue
		}
		if len(fields) > 5 && strings.TrimSpace(fields[4]) != "" {
			conditional++
			continue
		}
		r := parseRune(fields[0], line)
		for i := range full {
			mapped := parseRunes(fields[i+1], line)
			if mapped == string(simpleOf(i, r)) {
				continue
			}
			// A full mapping that is one character and not the simple one would
			// be the two files disagreeing about the same fact, which is not a
			// release of the database this can build a table from.
			if utf8.RuneCountInString(mapped) < 2 {
				fail(fmt.Sprintf("SpecialCasing.txt line %d maps U+%04X to %q, one "+
					"character, and UnicodeData.txt maps it to U+%04X", line, r, mapped, simpleOf(i, r)))
			}
			full[i] = append(full[i], entry{r, mapped})
		}
	}
	if err := sc.Err(); err != nil {
		fail(err.Error())
	}
	for i := range full {
		if len(full[i]) == 0 || len(simple[i]) == 0 {
			fail("a case with no mappings at all; these are not the files this reads")
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, `// Code generated by cmd/gencasing from Unicode's UnicodeData.txt and
// SpecialCasing.txt. DO NOT EDIT.

package paragraph

// The case mappings. Unicode %s.
//
// Two kinds, three cases each. The simple mappings are UnicodeData.txt's, one
// character to one: %d lowercased, %d titlecased, %d uppercased. The full
// mappings are the ones SpecialCasing.txt states unconditionally and that are
// not the simple mapping, which makes every one of them more than one
// character: %d lowercased, %d titlecased, %d uppercased. A character in
// neither kind maps to itself, so a character the tables do not name is one
// Unicode %s gives no case to — and not one the Go toolchain happens not to
// know. The %d conditional entries of SpecialCasing.txt are left out and are
// described in cmd/gencasing. Every table is sorted by character, and
// texttransform.go decides what to do with them.

// fullCase is one character's mapping, as a string because that is the point.
type fullCase struct {
	r rune
	s string
}

// simpleCase is one character's mapping to one character.
type simpleCase struct {
	r, to rune
}
`, *version, len(simple[0]), len(simple[1]), len(simple[2]),
		len(full[0]), len(full[1]), len(full[2]), *version, conditional)

	names := [3]struct{ full, simple, what string }{
		{"fullLowercase", "simpleLowercase", "lowercased"},
		{"fullTitlecase", "simpleTitlecase", "titlecased"},
		{"fullUppercase", "simpleUppercase", "uppercased"},
	}
	for i, n := range names {
		list := full[i]
		sort.Slice(list, func(a, b int) bool { return list[a].r < list[b].r })
		fmt.Fprintf(&b, "\n// %s: what these characters become when %s.\nvar %s = [...]fullCase{\n",
			n.full, n.what, n.full)
		for _, e := range list {
			fmt.Fprintf(&b, "\t{%#04X, %q}, // %s\n", e.r, e.s, runeName(e.r))
		}
		b.WriteString("}\n")
	}
	for i, n := range names {
		list := simple[i]
		sort.Slice(list, func(a, b int) bool { return list[a].r < list[b].r })
		fmt.Fprintf(&b, "\n// %s: what these characters become when %s, one for one.\n"+
			"var %s = [...]simpleCase{\n", n.simple, n.what, n.simple)
		for _, p := range list {
			fmt.Fprintf(&b, "\t{%#04X, %#04X},\n", p.r, p.to)
		}
		b.WriteString("}\n")
	}

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		fail("generated source does not parse: " + err.Error())
	}
	os.Stdout.Write(src)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "gencasing:", msg)
	os.Exit(1)
}

// runeName labels a row with the character itself, so a reader can see what the
// table is about without a code chart. The character is quoted because several
// of them are combining marks that would otherwise attach to the comment.
func runeName(r rune) string {
	return strconv.QuoteRune(r)
}

func parseRune(field string, line int) rune {
	v, err := strconv.ParseUint(strings.TrimSpace(field), 16, 32)
	if err != nil {
		fail(fmt.Sprintf("line %d: %v", line, err))
	}
	return rune(v)
}

func parseRunes(field string, line int) string {
	var out []rune
	for _, f := range strings.Fields(field) {
		out = append(out, parseRune(f, line))
	}
	return string(out)
}
