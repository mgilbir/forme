package layout

import (
	"bufio"
	"math"
	"os"
	"strconv"
	"strings"
	"testing"
)

// The operator dictionary, held to itself.
//
// MathML Core states its dictionary twice: B.1's compact tables, which the
// engine looks operators up in by B.1's algorithm, and B.2's human-readable
// listing, one operator and form a line. testdata/mathml/opdict.py transcribed
// both out of the same pinned snapshot; this asks the lookup every line of
// B.2 and requires B.2's answer, and asks every other character in every form
// and requires "not in the dictionary". A transcription error in either table,
// or a step of the algorithm read wrongly, shows as a disagreement.
func TestTheOperatorDictionaryAgreesWithItsHumanReadableForm(t *testing.T) {
	f, err := os.Open("../testdata/mathml/operator-dictionary.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	type key struct {
		content string
		form    mathForm
	}
	listed := map[key]bool{}
	forms := map[string]mathForm{"infix": formInfix, "prefix": formPrefix, "postfix": formPostfix}
	sc := bufio.NewScanner(f)
	rows := 0
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 6 {
			t.Fatalf("%q", line)
		}
		var content []rune
		for _, cp := range strings.Fields(fields[0]) {
			v, err := strconv.ParseUint(strings.TrimPrefix(cp, "U+"), 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			content = append(content, rune(v))
		}
		form := forms[fields[2]]
		listed[key{string(content), form}] = true
		rows++
		cat := mathOpCategoryOf(string(content), form)
		d := mathOpDefaults[cat]
		em := func(s string) float64 {
			v, err := strconv.ParseFloat(strings.TrimSuffix(s, "em"), 64)
			if err != nil {
				t.Fatalf("%q: %v", s, err)
			}
			return v
		}
		props := " " + fields[5] + " "
		has := func(p string) bool { return strings.Contains(props, " "+p+" ") }
		if math.Abs(d.lspace-em(fields[3])) > 1e-12 || math.Abs(d.rspace-em(fields[4])) > 1e-12 ||
			d.stretchy != has("stretchy") || d.symmetric != has("symmetric") ||
			d.largeop != has("largeop") || d.movablelimits != has("movablelimits") {
			t.Errorf("%s %s: category %d gives %+v, and B.2 says %s %s %s", fields[0], fields[2],
				cat, d, fields[3], fields[4], fields[5])
		}
		if len(content) == 1 && mathOpIsInlineAxis(content[0]) != (fields[1] == "inline") {
			t.Errorf("%s: inline axis %v, B.2 says %s", fields[0], mathOpIsInlineAxis(content[0]), fields[1])
		}
	}
	if rows != 1177 {
		t.Fatalf("%d rows, want B.2's 1177", rows)
	}
	for _, form := range []mathForm{formInfix, formPrefix, formPostfix} {
		for c := rune(0); c <= 0x2BFF; c++ {
			if listed[key{string(c), form}] {
				continue
			}
			if cat := mathOpCategoryOf(string(c), form); cat != opDefault {
				t.Errorf("U+%04X in form %d is category %d, and B.2 does not list it", c, form, cat)
			}
		}
	}
	// A negated operator is the operator it negates, and a two-character
	// ASCII operator the entry its index names.
	if mathOpCategoryOf("≠", formInfix) != mathOpCategoryOf("=", formInfix) {
		t.Error("a negated = is not looked up as =")
	}
	if got := mathOpCategoryOf("x̸", formInfix); got != opDefault {
		t.Errorf("a negated x is category %d, want the default", got)
	}
	if got := mathOpCategoryOf("ab", formInfix); got != opDefault {
		t.Errorf("an unlisted pair is category %d, want the default", got)
	}
	if got := mathOpCategoryOf("abc", formInfix); got != opDefault {
		t.Errorf("three characters are category %d, want the default", got)
	}
}
