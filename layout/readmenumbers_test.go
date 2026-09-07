package layout

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The numbers the README states about this repository, checked against the
// repository.
//
// They are the first thing a reader is told and the last thing anybody
// remeasures. The reftest row said 5,177 documents and 4,438 clean passes when
// the corpus held 6,253 and the ratchet stood at 5,956 — about fifteen hundred
// passes behind, because the ratchet only ever reported a rise with a Logf and
// nothing made anyone write it down. That is fixed at the other end too (see
// TestWPTReftests), and this is the half that keeps the prose honest.
//
// Only the numbers a test can compute are checked here. The bidi and grapheme
// counts come from files this repository does not carry, and asserting them
// from a constant would be checking the README against itself.

func readme(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "README.md"))
	if err != nil {
		t.Fatalf("reading the README: %v", err)
	}
	return string(b)
}

// readmeNumber pulls the one number matched by a pattern, with the commas a
// reader wants and a number does not.
func readmeNumber(t *testing.T, text, pattern string) int {
	t.Helper()
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("the README no longer says anything matching %q, so this test "+
			"cannot check it — fix the pattern or the prose", pattern)
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil {
		t.Fatalf("%q is not a number: %v", m[1], err)
	}
	return n
}

// TestTheReadmeSaysWhatTheRatchetSays.
func TestTheReadmeSaysWhatTheRatchetSays(t *testing.T) {
	text := readme(t)
	if got := readmeNumber(t, text, `\*\*([\d,]+) pass with nothing unsupported`); got != wptCleanPassBaseline {
		t.Errorf("the README says %d reftests pass cleanly and the ratchet stands "+
			"at %d", got, wptCleanPassBaseline)
	}
	// And how many there are to pass, which is the corpus and not a constant —
	// so it is counted where the corpus is, and skipped where it is not.
	root := os.Getenv(wptEnv)
	if root == "" {
		t.Skipf("set %s (or run `make test-wpt`) to check the corpus size too", wptEnv)
	}
	tests := findReftests(t, root)
	if len(tests) == 0 {
		t.Skip("the corpus holds no reftests")
	}
	if got := readmeNumber(t, text, `([\d,]+) documents rendered and compared`); got != len(tests) {
		t.Errorf("the README says %d documents are rendered and compared, and the "+
			"corpus holds %d", got, len(tests))
	}
}

// TestTheReadmeCountsTheFuzzTargets. "Ten fuzz targets" was written when there
// were ten, and six have been added since.
func TestTheReadmeCountsTheFuzzTargets(t *testing.T) {
	declared := 0
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		declared += strings.Count(string(b), "\nfunc Fuzz")
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if declared == 0 {
		t.Fatal("no fuzz targets were found at all, so this test says nothing")
	}
	if got := readmeNumber(t, readme(t), `([\d,]+) fuzz targets`); got != declared {
		t.Errorf("the README says there are %d fuzz targets and the repository "+
			"declares %d", got, declared)
	}

	// And how many of them a machine actually runs, which is the half that
	// matters: a target nothing schedules is a target that has never been run
	// for longer than its seeds take. The workflow's matrix is the answer, and
	// the README's word for it was written once and left behind — "eleven"
	// while the matrix named eleven, then still "eleven" after three were
	// added.
	flow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "fuzz.yml"))
	if err != nil {
		t.Fatalf("reading the fuzz workflow: %v", err)
	}
	scheduled := strings.Count(string(flow), "\n            target: ")
	if scheduled == 0 {
		t.Fatal("the fuzz workflow schedules nothing, so this test says nothing")
	}
	if got := readmeWord(t, readme(t), `([a-z]+) of them scheduled weekly`); got != scheduled {
		t.Errorf("the README says %d targets are scheduled weekly and the "+
			"workflow names %d", got, scheduled)
	}
}

// readmeWord pulls a number the README spells out in words.
//
// The prose says "fourteen of them", not "14 of them", and a number written as
// a word drifts exactly as easily as one written as digits — more easily, since
// nothing about it looks like a number to a reader skimming for one.
func readmeWord(t *testing.T, text, pattern string) int {
	t.Helper()
	m := regexp.MustCompile(pattern).FindStringSubmatch(text)
	if m == nil {
		t.Fatalf("the README no longer says %q, so this test cannot check it", pattern)
	}
	words := map[string]int{
		"one": 1, "two": 2, "three": 3, "four": 4, "five": 5, "six": 6,
		"seven": 7, "eight": 8, "nine": 9, "ten": 10, "eleven": 11,
		"twelve": 12, "thirteen": 13, "fourteen": 14, "fifteen": 15,
		"sixteen": 16, "seventeen": 17, "eighteen": 18, "nineteen": 19,
		"twenty": 20,
	}
	n, ok := words[m[1]]
	if !ok {
		t.Fatalf("the README says %q of them are scheduled, which this test "+
			"cannot read as a number", m[1])
	}
	return n
}
