package shape

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestIndicCategoriesAreHarfBuzzs holds the Indic model's categories to the
// ones HarfBuzz's own generator derives from the same Unicode release,
// recorded by testdata/harfbuzz/indiccategories.py, for every character an
// Indic run can hold: the nine scripts' own, and the Common and Inherited
// characters a run takes into itself.
//
// Unicode's file gives a category to characters of every Brahmic script and
// to a few elsewhere. HarfBuzz gives one only to the characters of the blocks
// its model is for, and every other character is Other to it. U+20F0
// COMBINING ASTERISK ABOVE is the case the sweep found: Unicode calls it a
// cantillation mark, it is in no block HarfBuzz reads, and so a Vedic sign
// after it starts a syllable of its own, shown against a dotted circle — which
// this package drew without. Five placeholders HarfBuzz takes from the Myanmar
// specification for all three of its models were Other here.
func TestIndicCategoriesAreHarfBuzzs(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "indiccategories.expected.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()
	theirs := map[rune]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}
		span, cat, ok := strings.Cut(line, " ")
		lo, hi, ok2 := strings.Cut(span, "..")
		if !ok || !ok2 {
			t.Fatalf("a line that is not a range and a category: %q", line)
		}
		a, err1 := strconv.ParseUint(lo, 16, 32)
		b, err2 := strconv.ParseUint(hi, 16, 32)
		if err1 != nil || err2 != nil || b < a {
			t.Fatalf("a range that does not read: %q", line)
		}
		for r := rune(a); r <= rune(b); r++ {
			theirs[r] = cat
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(theirs) < 1000 {
		t.Fatalf("only %d characters read from %s; it has stopped being read", len(theirs), path)
	}

	names := map[indicCat]string{
		catOther: "X", catConsonant: "C", catRa: "Ra", catVowel: "V", catMatra: "M",
		catNukta: "N", catHalant: "H", catStacker: "H", catZWJ: "ZWJ", catZWNJ: "ZWNJ",
		catSM: "SM", catVD: "A", catPlaceholder: "PLACEHOLDER", catDottedCircle: "DOTTEDCIRCLE",
		catSymbol: "Symbol", catRepha: "Repha", catCM: "CM", catCS: "CS", catRS: "RS",
		catMPst: "MPst", catSMPst: "SMPst",
		catVAbv: "VAbv", catVBlw: "VBlw", catVPre: "VPre", catVPst: "VPst",
		catRobatic: "Robatic", catXgroup: "Xgroup", catYgroup: "Ygroup",
		catAsat: "As", catMedialY: "MY", catMedialR: "MR", catMedialW: "MW", catMedialH: "MH",
		catMedialL: "ML", catPTone: "PT", catVS: "VS", catAnusvara: "A",
	}
	set := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, n := range strings.Fields(s) {
			out[n] = true
		}
		return out
	}
	// HarfBuzz reads one table for three models, and each model's grammar
	// names some of its categories: a category a grammar does not name is, to
	// that model, what Other is — a character no syllable takes in. So each
	// model is held to the categories its grammar reads, for the characters a
	// run it sets can hold. The names are the grammars' exports
	// (hb-ot-shaper-*-machine.rl).
	models := []struct {
		name  string
		runs  func(script uint16) bool
		cat   func(r rune) indicCat
		reads map[string]bool
	}{
		{"Indic", func(s uint16) bool { return indicConfigFor(s) != nil },
			func(r rune) indicCat { c, _ := indicProperties(r); return c },
			set("C V N H ZWNJ ZWJ M SM A PLACEHOLDER DOTTEDCIRCLE RS MPst Repha Ra CM Symbol CS SMPst")},
		{"Khmer", isKhmerScript, khmerCategory,
			set("C V H ZWNJ ZWJ PLACEHOLDER DOTTEDCIRCLE Ra VAbv VBlw VPre VPst Robatic Xgroup Ygroup")},
		{"Myanmar", isMyanmarScript, myanmarCategory,
			set("C V N H ZWNJ ZWJ SM PLACEHOLDER DOTTEDCIRCLE A Ra CS SMPst VAbv VBlw VPre VPst As MH MR MW MY PT VS ML")},
	}
	checked := 0
	for _, m := range models {
		differ := 0
		read := func(n string) string {
			if !m.reads[n] {
				return "X"
			}
			return n
		}
		for r := rune(0); r <= 0x10FFFF; r++ {
			s := scriptOf(r)
			if s != scriptCommon && s != scriptInherited && !m.runs(s) {
				continue
			}
			ours, known := names[m.cat(r)]
			if !known {
				ours = fmt.Sprintf("category %d", m.cat(r))
			}
			ours, want := read(ours), read(theirs[r])
			checked++
			if ours != want {
				differ++
				if differ <= 20 {
					t.Errorf("U+%04X is %s to this package's %s model and %s to HarfBuzz's", r, ours, m.name, want)
				}
			}
		}
		if differ > 20 {
			t.Errorf("and %d more in the %s model", differ-20, m.name)
		}
	}
	t.Logf("%d characters checked", checked)
}
