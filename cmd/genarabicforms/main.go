// Command genarabicforms generates the presentation forms HarfBuzz's Arabic
// fallback shaping is built from, out of Unicode's own UnicodeData.txt.
//
// An Arabic font with no joining forms of its own — no 'init', 'medi', 'fina'
// or 'isol' in the rules it states for the run's script — may still carry the
// forms as characters: Unicode has a presentation form for every joining
// letter in each position, in the Arabic Presentation Forms blocks, and older
// fonts and some newer ones map them. HarfBuzz builds the missing features out
// of the character map for such a font (hb-ot-shaper-arabic-fallback.hh): a
// letter's initial form is the glyph of the character UnicodeData.txt says is
// its <initial> form, and so on, and a handful of ligatures are made the same
// way from the presentation forms of their parts. This is the table it reads,
// generated as HarfBuzz's gen-arabic-table.py generates its own.
//
// # What is read
//
// Every compatibility decomposition tagged <initial>, <medial>, <final> or
// <isolated>:
//
//   - One character long, it is a letter's form in that position. Where the
//     file gives a letter two forms in one position — the Presentation
//     Forms-A block repeats some of the -B block — the later line is the one
//     kept, as it is in HarfBuzz.
//   - Longer, it is a ligature, and only the ones HarfBuzz lists are taken:
//     the lam-alef ligatures, the "Allah" ligature, the shadda-with-vowel marks
//     and a few more, in harfbuzzLigatures below. A ligature's parts are
//     written as base letters; they are matched as the forms the fallback
//     features have already made of them, so the parts of an isolated
//     two-letter ligature are the first letter's initial form and the second's
//     final form, and so on.
//   - A decomposition that starts with a space is a mark written over a space
//     in visual order: the shadda ligatures. Its marks, reversed, are the
//     parts, and they are matched as they are — marks take no joining form.
//
// Three private-use ligatures HarfBuzz adds to the file are added here too,
// because HarfBuzz reads them and a font that maps them is drawn with them.
//
//	go run ./cmd/genarabicforms -version <X.Y.Z> <UnicodeData.txt> > shape/arabicforms.go
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
)

// harfbuzzLigatures is gen-arabic-table.py's LIGATURES: the ligatures its
// fallback makes, out of every one UnicodeData.txt describes. The selection is
// HarfBuzz's, and so is the order it reads the file in, which is what decides
// the order of the ligatures that share a first part.
var harfbuzzLigatures = map[rune]bool{
	0xF2EE: true, 0xFC08: true, 0xFC0E: true, 0xFC12: true, 0xFC32: true, 0xFC3F: true,
	0xFC40: true, 0xFC41: true, 0xFC42: true, 0xFC43: true, 0xFC44: true, 0xFC4E: true,
	0xFC5E: true, 0xFC60: true, 0xFC61: true, 0xFC62: true, 0xFC6A: true, 0xFC6D: true,
	0xFC6F: true, 0xFC70: true, 0xFC73: true, 0xFC75: true, 0xFC86: true, 0xFC8F: true,
	0xFC91: true, 0xFC94: true, 0xFC9C: true, 0xFC9D: true, 0xFC9E: true, 0xFC9F: true,
	0xFCA1: true, 0xFCA2: true, 0xFCA3: true, 0xFCA4: true, 0xFCA8: true, 0xFCAA: true,
	0xFCAC: true, 0xFCB0: true, 0xFCC9: true, 0xFCCA: true, 0xFCCB: true, 0xFCCC: true,
	0xFCCD: true, 0xFCCE: true, 0xFCCF: true, 0xFCD0: true, 0xFCD1: true, 0xFCD2: true,
	0xFCD3: true, 0xFCD5: true, 0xFCDA: true, 0xFCDB: true, 0xFCDC: true, 0xFCDD: true,
	0xFD30: true, 0xFD88: true, 0xFEF5: true, 0xFEF6: true, 0xFEF7: true, 0xFEF8: true,
	0xFEF9: true, 0xFEFA: true, 0xFEFB: true, 0xFEFC: true, 0xF201: true, 0xF211: true,
}

// harfbuzzPrivateUse are the lines gen-arabic-table.py appends to the file:
// three ligatures at private-use code points that some fonts map.
var harfbuzzPrivateUse = []string{
	"F201;PUA ARABIC LIGATURE LELLAH ISOLATED FORM;Lo;0;AL;<isolated> 0644 0644 0647;;;;N;;;;;",
	"F211;PUA ARABIC LIGATURE LAM WITH MEEM WITH JEEM INITIAL FORM;Lo;0;AL;<initial> 0644 0645 062C;;;;N;;;;;",
	"F2EE;PUA ARABIC LIGATURE SHADDA WITH FATHATAN ISOLATED FORM;Lo;0;AL;<isolated> 0020 064B 0651;;;;N;;;;;",
}

// The four positions, in the order a row of the table gives them.
var positions = []string{"initial", "medial", "final", "isolated"}

// ligature is a ligature as the fallback matches it: its first part, the
// rest, and what they become.
type ligature struct {
	first, lig rune
	rest       []rune
}

func main() {
	version := flag.String("version", "", "the Unicode version the file came from")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 || *version == "" {
		fmt.Fprintln(os.Stderr, "usage: genarabicforms -version <X.Y.Z> <UnicodeData.txt>")
		os.Exit(2)
	}
	if err := ucd.Check(*version, args...); err != nil {
		fail(err.Error())
	}
	f, err := os.Open(args[0])
	if err != nil {
		fail(err.Error())
	}
	defer f.Close()
	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	if err := sc.Err(); err != nil {
		fail(err.Error())
	}
	lines = append(lines, harfbuzzPrivateUse...)

	// What each line says, in the file's order.
	shapes := map[rune]map[string]rune{}
	type ligKey string
	type ligEntry struct {
		parts  []rune
		shapes []string // in the order first met
		byName map[string]rune
	}
	var ligOrder []ligKey
	ligs := map[ligKey]*ligEntry{}
	for n, line := range lines {
		fields := strings.Split(line, ";")
		if len(fields) != 15 {
			fail(fmt.Sprintf("line %d: %d fields where UnicodeData.txt has 15", n+1, len(fields)))
		}
		d := strings.TrimSpace(fields[5])
		if !strings.HasPrefix(d, "<") {
			continue
		}
		items := strings.Fields(d)
		shape := strings.Trim(items[0], "<>")
		switch shape {
		case "initial", "medial", "final", "isolated":
		default:
			continue
		}
		c := parse(fields[0])
		parts := make([]rune, 0, len(items)-1)
		for _, it := range items[1:] {
			parts = append(parts, parse(it))
		}
		if len(parts) == 1 {
			if shapes[parts[0]] == nil {
				shapes[parts[0]] = map[string]rune{}
			}
			shapes[parts[0]][shape] = c // the later line wins
			continue
		}
		// A mark ligature is written over a space in visual order: the space
		// goes, the marks are put back into logical order, and it has no
		// position of its own.
		if parts[0] == ' ' {
			rev := make([]rune, 0, len(parts)-1)
			for i := len(parts) - 1; i >= 1; i-- {
				rev = append(rev, parts[i])
			}
			parts, shape = rev, ""
		}
		if !harfbuzzLigatures[c] {
			continue
		}
		k := ligKey(fmt.Sprint(parts))
		e := ligs[k]
		if e == nil {
			e = &ligEntry{parts: parts, byName: map[string]rune{}}
			ligs[k] = e
			ligOrder = append(ligOrder, k)
		}
		if _, seen := e.byName[shape]; !seen {
			e.shapes = append(e.shapes, shape)
		}
		e.byName[shape] = c
	}

	form := func(r rune, shape string) rune {
		s, ok := shapes[r][shape]
		if !ok {
			fail(fmt.Sprintf("a ligature names U+%04X in its %s form, which the file does not give", r, shape))
		}
		return s
	}
	var two, three, marks []ligature
	for _, k := range ligOrder {
		e := ligs[k]
		for _, shape := range e.shapes {
			c := e.byName[shape]
			p := e.parts
			var got []rune
			switch {
			case shape == "" && len(p) == 2:
				marks = append(marks, ligature{first: p[0], rest: []rune{p[1]}, lig: c})
				continue
			case len(p) == 2 && shape == "isolated":
				got = []rune{form(p[0], "initial"), form(p[1], "final")}
			case len(p) == 2 && shape == "final":
				got = []rune{form(p[0], "medial"), form(p[1], "final")}
			case len(p) == 2 && shape == "initial":
				got = []rune{form(p[0], "initial"), form(p[1], "medial")}
			case len(p) == 3 && shape == "isolated":
				got = []rune{form(p[0], "initial"), form(p[1], "medial"), form(p[2], "final")}
			case len(p) == 3 && shape == "final":
				got = []rune{form(p[0], "medial"), form(p[1], "medial"), form(p[2], "final")}
			case len(p) == 3 && shape == "initial":
				got = []rune{form(p[0], "initial"), form(p[1], "medial"), form(p[2], "medial")}
			default:
				fail(fmt.Sprintf("U+%04X is a %d-part %q ligature, which HarfBuzz's fallback does not make",
					c, len(p), shape))
			}
			l := ligature{first: got[0], rest: got[1:], lig: c}
			if len(p) == 2 {
				two = append(two, l)
			} else {
				three = append(three, l)
			}
		}
	}
	// By first part, and in the order they were met among those that share one.
	for _, s := range [][]ligature{two, three, marks} {
		sort.SliceStable(s, func(i, j int) bool { return s[i].first < s[j].first })
	}

	lo, hi := rune(-1), rune(-1)
	for r := range shapes {
		if lo < 0 || r < lo {
			lo = r
		}
		if r > hi {
			hi = r
		}
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, `// Code generated by cmd/genarabicforms from Unicode's UnicodeData.txt. DO NOT EDIT.

package shape

// The Arabic presentation forms, from Unicode %s, as HarfBuzz's Arabic
// fallback shaping reads them: see arabicfallback.go and cmd/genarabicforms.

// presentationFormsFirst and presentationFormsLast are the letters a row of
// presentationForms is for.
const (
	presentationFormsFirst = 0x%04X
	presentationFormsLast  = 0x%04X
)

// presentationForms is each letter's initial, medial, final and isolated form,
// in that order, or 0 where the file gives it none.
var presentationForms = [...][4]rune{
`, *version, lo, hi)
	for r := lo; r <= hi; r++ {
		fmt.Fprintf(&b, "\t{")
		for i, shape := range positions {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "0x%04X", shapes[r][shape])
		}
		fmt.Fprintf(&b, "}, // U+%04X\n", r)
	}
	b.WriteString("}\n")
	writeLigatures(&b, "presentationLigatures", two,
		"the two-part ligatures, each matched as the presentation forms of its parts")
	writeLigatures(&b, "presentationLigatures3", three,
		"the three-part ligatures, each matched as the presentation forms of its parts")
	writeLigatures(&b, "presentationMarkLigatures", marks,
		"the ligatures of two marks, matched as the marks themselves")
	src, err := format.Source(b.Bytes())
	if err != nil {
		fail(err.Error())
	}
	os.Stdout.Write(src)
}

func writeLigatures(b *bytes.Buffer, name string, ls []ligature, what string) {
	b.WriteString("\n")
	b.WriteString(comment(fmt.Sprintf("%s is %s: %d of them, by first part.", name, what, len(ls))))
	fmt.Fprintf(b, "var %s = [...]presentationLigature{\n", name)
	for _, l := range ls {
		rest := [2]rune{}
		copy(rest[:], l.rest)
		fmt.Fprintf(b, "\t{0x%04X, [2]rune{0x%04X, 0x%04X}, 0x%04X},\n", l.first, rest[0], rest[1], l.lig)
	}
	b.WriteString("}\n")
}

// comment is text as Go comment lines no wider than the rest of the package's.
func comment(text string) string {
	var b strings.Builder
	line := "//"
	for _, w := range strings.Fields(text) {
		if len(line)+1+len(w) > 78 && line != "//" {
			b.WriteString(line + "\n")
			line = "//"
		}
		line += " " + w
	}
	b.WriteString(line + "\n")
	return b.String()
}

func parse(s string) rune {
	v, err := strconv.ParseUint(strings.TrimSpace(s), 16, 32)
	if err != nil || v > 0x10FFFF {
		fail(fmt.Sprintf("%q is not a code point", s))
	}
	return rune(v)
}

func fail(msg string) {
	fmt.Fprintln(os.Stderr, "genarabicforms:", msg)
	os.Exit(1)
}
