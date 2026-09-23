package shape

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestUseCategoriesAreHarfBuzzs holds the universal engine's table, which
// cmd/genuse derives, to the table HarfBuzz's own generator derives from the
// same Unicode release and the same correction files, recorded by
// testdata/harfbuzz/usecategories.py.
//
// Two implementations of one derivation, over one input: where they disagree,
// one of them has a correction the other lacks. That is how audit C187 was
// settled — the Grantha anusvara and visarga and the Tirhuta visarga, which
// HarfBuzz places above and the published position puts to the right — and
// how the Lepcha final modifier the correction file names in a word Unicode
// does not have was found beside it.
//
// Two differences are not disagreements, and are named:
//
//   - The scripts HarfBuzz never sets with this engine — Arabic, Lao,
//     Samaritan, Syriac, Thai — it leaves out of its table; this package's
//     categorize does not send them here either, and the table may say what
//     it likes about them.
//   - Egyptian hieroglyph clusters (G, J, SB, SE, HM, HR) are a part of the
//     engine this package does not implement: their characters read as Other
//     here, and a quadrat is set as its signs in a row.
func TestUseCategoriesAreHarfBuzzs(t *testing.T) {
	path := filepath.Join(harfbuzzDir, "usecategories.expected.txt")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	defer f.Close()

	names := [...]string{
		useO: "O", useB: "B", useN: "N", useGB: "GB", useCGJ: "CGJ", useF: "F",
		useFM: "FM", useM: "M", useCM: "CM", useSUB: "SUB", useCS: "CS", useH: "H",
		useHVM: "HVM", useHN: "HN", useIS: "IS", useZWNJ: "ZWNJ", useRK: "RK",
		useR: "R", useSk: "Sk", useSM: "SM", useV: "V", useVM: "VM", useWJ: "WJ",
	}
	positions := [...]string{usePosNone: "", usePosPre: "Pre", usePosAbv: "Abv",
		usePosBlw: "Blw", usePosPst: "Pst"}
	unimplemented := map[string]bool{"G": true, "J": true, "SB": true, "SE": true, "HM": true, "HR": true}
	ours := func(r rune) string {
		cat, pos := useCategoryOf(r)
		return names[cat] + positions[pos]
	}

	theirs := map[rune]string{}
	skipped := 0
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
			if unimplemented[cat] {
				skipped++
				theirs[r] = "O"
				continue
			}
			theirs[r] = cat
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(theirs) < 2000 {
		t.Fatalf("only %d characters read from %s; the file or its reader is broken", len(theirs), path)
	}
	// The hieroglyph categories are a known gap and not a place for others to
	// hide: this is how many characters they are, and a table that grew more
	// of them would be a change worth seeing.
	if skipped != 5117 {
		t.Errorf("%d characters are of the hieroglyph categories this package does not "+
			"implement; the count was 5117", skipped)
	}

	disabled := map[string]bool{"arab": true, "lao ": true, "samr": true, "syrc": true, "thai": true}
	bad := 0
	for r := rune(0); r <= 0x10FFFF; r++ {
		want, named := theirs[r]
		if !named {
			if tags := scriptTags(scriptOf(r)); len(tags) > 0 && disabled[tags[len(tags)-1]] {
				continue
			}
			want = "O"
		}
		if got := ours(r); got != want {
			bad++
			if bad <= 20 {
				t.Errorf("U+%04X: %s here, %s in HarfBuzz's derivation", r, got, want)
			}
		}
	}
	if bad > 0 {
		t.Errorf("%d characters are in another category from HarfBuzz's", bad)
	}
}
