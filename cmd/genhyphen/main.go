// Command genhyphen generates the hyphenation patterns css-text's
// "hyphens: auto" needs, from a hyph-utf8 pattern file.
//
// The patterns are Liang's, and so is the algorithm that reads them: a word is
// wrapped in the word-boundary marker, every substring of it is looked up, and
// the numbers the matching patterns carry are taken at their maximum at each
// position between two letters. An odd number there is a place the word may
// break. That is the whole of it, and it is why a table of five thousand
// strings is a hyphenation dictionary for a language rather than an index of
// its words.
//
// # What is taken from the file
//
// Three things, and the licence notice is one of them. The pattern files carry
// their own copyright and their own permission to redistribute, and the terms
// differ from file to file — Dutch is MIT, Hungarian offers MPL, GPL or LGPL —
// so the header is copied *entire* rather than summarised or picked over. A
// table with the terms stripped off is a table nobody may ship, and a generator
// that reads one file's way of writing them will quietly drop another's.
//
// The other two are the \patterns{} block and the \hyphenation{} block. The
// second is not an optimisation of the first — it is the list of words the
// patterns get wrong, and a word on it takes its breaks from the list and from
// nowhere else. Some of its entries have no hyphens in them at all ("present",
// "project"), which is the list saying that a word the patterns would break must
// not be broken.
//
// The hyphenmins come from the header's "typesetting" block, where a language
// states how many letters it wants left at each end of a line. They are the
// language's answer and not CSS's: hyphenate-limit-chars' initial value is
// "auto", which css-text defines as the UA's choice, and the choice a
// hyphenation dictionary ships with is the one its author made. A file that
// states a "generation" pair as well — pinyin does — is not asked about it: that
// pair is for building the patterns, not for setting type with them.
//
// The generated file names the URL the patterns were fetched from, at a pinned
// commit of tex-hyphen — the Makefile's TEX_HYPHEN_COMMIT — so that the table
// can be generated again from the same patterns:
//
//	go run ./cmd/genhyphen -source <url> <name> <key> <hyph-LANG.tex> > paragraph/<name>hyphens.go
//
// name is the Go identifier the table is declared under, and key is the tag
// paragraph.HyphenationOf resolves a document's lang attribute to.
//
// # A file offered under a choice of licences
//
// Hungarian's is offered under MPL 1.1, GPL 2.0 or LGPL 2.1, at the
// recipient's option, and this repository takes it under the MPL 1.1. That
// licence asks for more than the header: its Exhibit A notice in each file of
// the Source Code (§3.5), with its blanks filled, and a note of what was
// changed (§3.3). -mpl names the licence's text, pinned by -mpl-sha256, and
// the generator writes the notice from it — the fixed sentences quoted from
// the licence, the blanks filled from the pattern file's own header, and
// refusing a file whose header does not offer the MPL 1.1 or does not name
// its initial developer:
//
//	go run ./cmd/genhyphen -source <url> -mpl <MPL-1.1.txt> -mpl-source <url> -mpl-sha256 <hex> \
//		<name> <key> <hyph-LANG.tex>
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func main() {
	source := flag.String("source", "", "the URL the patterns were fetched from, at their pinned commit")
	mpl := flag.String("mpl", "", "the MPL 1.1's text, when this repository takes the file under it")
	mplSource := flag.String("mpl-source", "", "the URL the MPL 1.1's text was fetched from")
	mplPin := flag.String("mpl-sha256", "", "the SHA-256 the MPL 1.1's text is pinned to")
	flag.Parse()
	args := flag.Args()
	if len(args) != 3 || *source == "" {
		fmt.Fprintln(os.Stderr, "usage: genhyphen -source <url> <name> <key> <hyph-LANG.tex>")
		os.Exit(2)
	}
	name, key, path := args[0], args[1], args[2]
	f, err := os.Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	var header []string
	var patterns, exceptions []string
	left, right := 0, 0

	const (
		outside = iota
		inPatterns
		inExceptions
	)
	where := outside
	// inHeader is the leading run of comment lines, which is the block the file
	// states its copyright and its licence in. A pattern file has comments
	// further down as well — Dutch has sixty-seven of them among its patterns —
	// and those are notes about the patterns, not terms.
	inHeader := true
	inTypesetting := false

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 0, 1<<20), 1<<20)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "%") {
			text := strings.TrimSpace(strings.TrimPrefix(line, "%"))
			if inHeader {
				header = append(header, strings.TrimRight(strings.TrimPrefix(line, "%"), " \t"))
				switch {
				case text == "typesetting:":
					inTypesetting = true
				case inTypesetting && strings.HasPrefix(text, "left:"):
					left = number(text)
				case inTypesetting && strings.HasPrefix(text, "right:"):
					right = number(text)
				case strings.HasSuffix(text, ":") && text != "typesetting:":
					// Another key at any depth ends the block: the two numbers
					// sit directly under "typesetting" and nothing else does.
					inTypesetting = false
				}
			}
			continue
		}
		if strings.TrimSpace(line) != "" {
			inHeader = false
		}
		switch {
		case strings.HasPrefix(line, `\patterns{`):
			where = inPatterns
			continue
		case strings.HasPrefix(line, `\hyphenation{`):
			where = inExceptions
			continue
		case strings.TrimSpace(line) == "}":
			where = outside
			continue
		}
		// A line holds one entry or several. English and Dutch and Hungarian
		// write one per line; pinyin writes the five tone marks of a syllable
		// side by side — "a1b ā1b á1b ǎ1b à1b" is five patterns, and reading it
		// as one is a pattern that matches nothing, which is a table that
		// silently hyphenates nothing.
		for _, word := range strings.Fields(line) {
			if strings.HasPrefix(word, `\`) {
				continue
			}
			switch where {
			case inPatterns:
				patterns = append(patterns, word)
			case inExceptions:
				exceptions = append(exceptions, word)
			}
		}
	}
	if err := s.Err(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if len(patterns) == 0 {
		fmt.Fprintln(os.Stderr, "no \\patterns{} block in "+path)
		os.Exit(1)
	}
	if left <= 0 || right <= 0 {
		fmt.Fprintln(os.Stderr, "no typesetting hyphenmins in "+path)
		os.Exit(1)
	}
	// The two blocks go into raw string literals, which have no escapes at all,
	// so a backtick anywhere in them would end the literal and produce a file
	// that does not compile — or, worse, one that does and means something else.
	// No pattern file has one; a new one that did would stop here.
	for _, block := range [][]string{patterns, exceptions} {
		for _, w := range block {
			if strings.ContainsRune(w, '`') {
				fmt.Fprintln(os.Stderr, "a backtick in "+path+": "+w)
				os.Exit(1)
			}
		}
	}

	var b strings.Builder
	// The base name and not the path it was read from: the generated file is
	// checked in and has to say the same thing whoever ran the generator.
	fmt.Fprintf(&b, "// Code generated by cmd/genhyphen from %s.\n// DO NOT EDIT.\n//\n// Source: %s\n\n",
		filepath.Base(path), *source)
	fmt.Fprintln(&b, "package paragraph")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "// %sHyphenation is the hyph-utf8 pattern table for %q.\n", name, key)
	fmt.Fprintln(&b, "//")
	fmt.Fprintf(&b, "// %d patterns and %d exceptions. See cmd/genhyphen for what each is\n",
		len(patterns), len(exceptions))
	fmt.Fprintln(&b, "// and hyphenate.go for the algorithm that reads them.")
	fmt.Fprintln(&b, "//")
	fmt.Fprintln(&b, "// The pattern file's own header follows, entire. It carries the copyright and")
	fmt.Fprintln(&b, "// the licence, and both have to travel with the table.")
	fmt.Fprintln(&b, "//")
	for _, line := range header {
		fmt.Fprintln(&b, strings.TrimRight("//"+line, " "))
	}
	if *mpl != "" {
		if *mplSource == "" || *mplPin == "" {
			fmt.Fprintln(os.Stderr, "-mpl without -mpl-source and -mpl-sha256: the notice has to say which text it is from")
			os.Exit(2)
		}
		writeMPLNotice(&b, *mpl, *mplSource, *mplPin, filepath.Base(path), header)
	}
	fmt.Fprintf(&b, "var %sHyphenation = hyphenSource{\n", name)
	fmt.Fprintf(&b, "\tkey:   %q,\n", key)
	fmt.Fprintf(&b, "\tleft:  %d,\n", left)
	fmt.Fprintf(&b, "\tright: %d,\n", right)
	fmt.Fprintf(&b, "\t// patterns is Liang's \\patterns{} block, one pattern per line.\n")
	fmt.Fprintf(&b, "\tpatterns: `%s`,\n", strings.Join(patterns, "\n"))
	fmt.Fprintf(&b, "\t// exceptions is the \\hyphenation{} block: the words the patterns get\n")
	fmt.Fprintf(&b, "\t// wrong, each written with the breaks it is allowed and no others.\n")
	fmt.Fprintf(&b, "\texceptions: `%s`,\n", strings.Join(exceptions, "\n"))
	fmt.Fprintln(&b, "}")

	out, err := format.Source([]byte(b.String()))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
}

// writeMPLNotice writes the notice the MPL 1.1 asks for in each file of Covered
// Code: Exhibit A, its fixed sentences read from the licence's own text and its
// blanks from the pattern file's header, and the §3.3 note of the change.
func writeMPLNotice(b *strings.Builder, licence, source, pin, file string, header []string) {
	data, err := os.ReadFile(licence)
	if err != nil {
		fail("reading " + licence + ": " + err.Error())
	}
	if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != pin {
		fail(fmt.Sprintf("%s has SHA-256 %x, and the MPL 1.1 is pinned to %s", licence, sum, pin))
	}
	text := string(data)
	if !strings.Contains(text, "MOZILLA PUBLIC LICENSE") || !strings.Contains(text, "Version 1.1") {
		fail(licence + " is not the Mozilla Public License 1.1")
	}
	// Exhibit A's two fixed paragraphs: from the opening quote to the first
	// blank the licensee fills in.
	i := strings.Index(text, "EXHIBIT A")
	if i < 0 {
		fail(licence + " has no Exhibit A")
	}
	exhibit := text[i:]
	start := strings.Index(exhibit, "``")
	end := strings.Index(exhibit, "The Original Code is")
	if start < 0 || end < start {
		fail(licence + "'s Exhibit A is not the shape this reads")
	}
	var fixed []string
	for _, l := range strings.Split(exhibit[start+2:end], "\n") {
		fixed = append(fixed, strings.TrimSpace(l))
	}
	for len(fixed) > 0 && fixed[len(fixed)-1] == "" {
		fixed = fixed[:len(fixed)-1]
	}

	// The blanks, from the header.
	field := func(key string) string {
		for _, l := range header {
			if v, ok := strings.CutPrefix(strings.TrimSpace(l), key+":"); ok && strings.TrimSpace(v) != "" {
				return strings.TrimSpace(v)
			}
		}
		fail(file + "'s header has no " + key + ", which the MPL notice needs")
		return ""
	}
	offered := false
	for k, l := range header {
		if strings.TrimSpace(l) == "name: MPL" && k+1 < len(header) &&
			strings.TrimSpace(header[k+1]) == "version: 1.1" {
			offered = true
		}
	}
	if !offered {
		fail(file + "'s header does not offer the MPL 1.1")
	}
	title := field("title")
	developer := field("initial_developer")
	copyright := strings.TrimPrefix(field("copyright"), "Copyright (C) ")
	var contributors []string
	for k, l := range header {
		if strings.TrimSpace(l) != "contributors:" {
			continue
		}
		for _, c := range header[k+1:] {
			v, ok := strings.CutPrefix(strings.TrimSpace(c), "- ")
			if !ok {
				break
			}
			contributors = append(contributors, v)
		}
	}
	if len(contributors) == 0 {
		fail(file + "'s header lists no MPL contributors")
	}

	fmt.Fprintln(b, "//")
	fmt.Fprintln(b, "// The file is offered under a choice of licences, and this repository takes")
	fmt.Fprintln(b, "// it, and this table made from it, under the Mozilla Public License 1.1.")
	fmt.Fprintln(b, "// The notice its Exhibit A asks for in each file, with the blanks filled")
	fmt.Fprintln(b, "// from the header above; the licence's text is in THIRD_PARTY_NOTICES.")
	fmt.Fprintf(b, "//\n// Licence text: %s\n// SHA-256: %s\n//\n", source, pin)
	for _, l := range fixed {
		if l == "" {
			fmt.Fprintln(b, "//")
		} else {
			fmt.Fprintln(b, "//\t"+l)
		}
	}
	fmt.Fprintln(b, "//")
	fmt.Fprintf(b, "//\tThe Original Code is %s, %s.\n", file, title)
	fmt.Fprintln(b, "//")
	fmt.Fprintf(b, "//\tThe Initial Developer of the Original Code is %s.\n", developer)
	fmt.Fprintf(b, "//\tPortions created by the Initial Developer are Copyright (C) %s.\n", copyright)
	fmt.Fprintln(b, "//\tAll Rights Reserved.")
	fmt.Fprintln(b, "//")
	fmt.Fprintf(b, "//\tContributor(s): %s.\n", strings.Join(contributors, ", "))
	fmt.Fprintln(b, "//")
	fmt.Fprintf(b, "// Modified, as §3.3 asks it be said: cmd/genhyphen copies %s's\n", file)
	fmt.Fprintln(b, "// \\patterns and \\hyphenation blocks into this Go file, one entry per line,")
	fmt.Fprintln(b, "// and adds nothing to them, removes nothing and changes no entry. The date of")
	fmt.Fprintln(b, "// each change is the commit that made it.")
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "genhyphen:", msg)
	os.Exit(1)
}

// number reads the integer at the end of a "left: 2" header line.
func number(text string) int {
	i := strings.IndexByte(text, ':')
	n, err := strconv.Atoi(strings.TrimSpace(text[i+1:]))
	if err != nil {
		return 0
	}
	return n
}
