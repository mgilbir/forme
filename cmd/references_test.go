package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// What the repository's own words point at has to be here.
//
// The code grew in another repository and was moved, and its comments came
// with it. They sent a reader to a design document ("the rendering proposal
// §4.1"), to architecture records under docs/adr/, to files under render/, and
// to `make corpus` and `make arlington` — none of which this repository has.
// And a search-and-replace of the old repository's name for this one turned
// its history into nonsense: "it came from forme", written in forme.
//
// So those spellings are refused wherever this repository's own text is, which
// is every tracked file that is not test data. A reference that has to be made
// to the old repository can say what it was without naming what is not here.

// trackedText lists the tracked files a reader reads as this repository's
// own words: code, the Makefile, the workflows and the prose.
func trackedText(t *testing.T) []string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on this machine to list the tracked files")
	}
	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout (%v), so there is no list of tracked files", err)
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		switch {
		case f == "", strings.HasPrefix(f, "testdata/"), strings.Contains(f, "/testdata/"):
			continue
		case strings.HasSuffix(f, ".go"), strings.HasSuffix(f, ".md"),
			strings.HasSuffix(f, ".yml"), f == "Makefile", f == ".gitignore":
			files = append(files, f)
		}
	}
	if len(files) < 100 {
		t.Fatalf("only %d tracked text files were listed, so the listing has failed", len(files))
	}
	return files
}

// TestNothingPointsAtWhatIsNotHere.
func TestNothingPointsAtWhatIsNotHere(t *testing.T) {
	dead := regexp.MustCompile(`rendering proposal|docs/adr/|\bADR [0-9]|\brender/[a-z]+\.go|` +
		`\bpdf0\b|make corpus\b|make arlington\b|(came from|moved to|moved into) forme\b`)
	for _, f := range trackedText(t) {
		if f == "cmd/references_test.go" {
			continue // the patterns themselves
		}
		b, err := os.ReadFile(filepath.Join("..", f))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if m := dead.FindString(line); m != "" {
				t.Errorf("%s:%d says %q, which names something this repository does not "+
					"have", f, i+1, m)
			}
		}
	}
}

// TestEveryFuzzCorpusBelongsToATarget is the other half of the move. Go reads a
// fuzz target's seed corpus from <package>/testdata/fuzz/<Target> and from
// nowhere else, so a corpus file anywhere else is read by nothing. Two crashers
// of FuzzLoadAndUse sat under testdata/testdata/fuzz for the whole life of this
// repository — the path the move made of the old package's
// fonts/testdata/fuzz — and were never replayed.
//
// A tracked file under a fuzz corpus path has to be in a package that
// declares the target it is filed under.
func TestEveryFuzzCorpusBelongsToATarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git on this machine to list the tracked files")
	}
	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout (%v), so there is no list of tracked files", err)
	}
	const marker = "testdata/fuzz/"
	seeds := 0
	declared := map[string]string{} // package directory → its test files, joined
	for _, f := range strings.Split(string(out), "\x00") {
		i := strings.LastIndex(f, marker)
		if i < 0 || (i > 0 && f[i-1] != '/') {
			continue
		}
		seeds++
		pkg := strings.TrimSuffix(f[:i], "/")
		target, _, _ := strings.Cut(f[i+len(marker):], "/")
		src, ok := declared[pkg]
		if !ok {
			tests, _ := filepath.Glob(filepath.Join("..", pkg, "*_test.go"))
			var all []string
			for _, tf := range tests {
				b, err := os.ReadFile(tf)
				if err != nil {
					t.Fatal(err)
				}
				all = append(all, string(b))
			}
			src = strings.Join(all, "\n")
			declared[pkg] = src
		}
		if !strings.Contains(src, "\nfunc "+target+"(") {
			where := pkg
			if where == "" {
				where = "the repository root"
			}
			t.Errorf("%s is a seed for %s, and %s declares no such fuzz target, so "+
				"nothing replays it", f, target, where)
		}
	}
	if seeds == 0 {
		t.Fatal("no fuzz corpus files are tracked at all, so this test says nothing")
	}
	t.Logf("%d tracked seed files, each filed under a target its package declares", seeds)
}
