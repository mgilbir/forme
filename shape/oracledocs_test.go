package shape

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What the oracle's own documentation claims, checked against the oracle.
//
// testdata/harfbuzz/README.md is the account of how the shaping is held to
// HarfBuzz, and it had gone out of date in the two ways prose does: numbers that
// were true when they were written — three corpora, thirteen deliberate
// differences — and paths that moved when the package was renamed, so that every
// file it sent a reader to was one that does not exist.
//
// Both are checkable, so both are checked. The numbers are read off the test
// tables, and every Go file the documents name has to be there.

// oracleDocs are the documents that describe this oracle.
var oracleDocs = []string{
	"../testdata/harfbuzz/README.md",
	"../testdata/coretext/README.md",
	"../testdata/coretext/RESULT.md",
}

// goPathPattern matches a backticked reference to a file in this repository.
// A directory is required: a bare "indic.go" names no place, and the mistake
// this is about was a path to a package that had been renamed.
var goPathPattern = regexp.MustCompile("`([a-z0-9]+(?:/[a-z0-9_]+)+\\.go)`")

// spelledNumbers are the counts the prose writes out rather than in figures.
var spelledNumbers = map[int]string{
	1: "one", 2: "two", 3: "three", 4: "four", 5: "five", 6: "six",
	7: "seven", 8: "eight", 9: "nine", 10: "ten",
}

// TestTheOracleDocumentsNameFilesThatExist.
func TestTheOracleDocumentsNameFilesThatExist(t *testing.T) {
	found := 0
	for _, doc := range oracleDocs {
		src, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range goPathPattern.FindAllStringSubmatch(string(src), -1) {
			found++
			if _, err := os.Stat(filepath.Join("..", m[1])); err != nil {
				t.Errorf("%s names %s, which is not there: %v", doc, m[1], err)
			}
		}
	}
	// The pattern is what the check rests on, so a pattern that stopped matching
	// must not pass in silence.
	if found < 6 {
		t.Errorf("only %d file references were found across %d documents; the "+
			"pattern that reads them off is not matching what it should",
			found, len(oracleDocs))
	}
}

// TestTheOracleReadmeCountsWhatIsThere. The two numbers in it that the code can
// answer: how many corpora there are and how many cases are excused.
func TestTheOracleReadmeCountsWhatIsThere(t *testing.T) {
	src, err := os.ReadFile("../testdata/harfbuzz/README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(src)

	corpora := len(harfbuzzCases)
	lower := strings.ToLower(readme)
	if want := spelledNumbers[corpora] + " corpora"; !strings.Contains(lower, want) {
		t.Errorf("the README does not say %q; there are %d corpora in "+
			"harfbuzzCases", want, corpora)
	}
	// And says it nowhere else with another number. A document that has been
	// half updated says both, which is how this one came to open with "three
	// corpora" and list six of them thirty lines further down.
	for n, word := range spelledNumbers {
		if n == corpora {
			continue
		}
		if phrase := word + " corpora"; strings.Contains(lower, phrase) {
			t.Errorf("the README also says %q, and there are %d", phrase, corpora)
		}
	}

	excused := 0
	for _, byString := range deliberateDifferences {
		excused += len(byString)
	}
	if want := spelledNumbers[excused] + " cases that differ on purpose"; !strings.Contains(lower, want) {
		t.Errorf("the README does not say %q; deliberateDifferences holds %d",
			want, excused)
	}
}
