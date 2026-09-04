package layout

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The corpus is pinned, and this is the check that it is.
//
// A ratchet has to be measured against a fixed thing. The Web Platform Tests are
// fetched rather than committed, from a repository that moves every day, so the
// only thing making one run comparable with the next is that both check out the
// same commit — and for a long time nothing did: the clone took the tip of a
// branch, and a CI cache keyed on the Makefile was what held it still. Any edit
// to that file swapped the corpus underneath the number, which is the one thing
// a ratchet must never do.
//
// So the Makefile names a commit, and this says the checkout is at it. It fails
// on the two ways that can go wrong that nothing else would notice: a checkout
// made before the pin existed, and a pin edited without the corpus being taken
// again.

// TestTheCorpusIsAtTheCommitTheMakefileNames.
func TestTheCorpusIsAtTheCommitTheMakefileNames(t *testing.T) {
	root := os.Getenv("WPT_TESTS")
	if root == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`)")
	}
	want := pinnedCommit(t)
	got, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Skipf("the checkout at %s is not a git repository: %v", root, err)
	}
	if at := strings.TrimSpace(string(got)); at != want {
		t.Errorf("the corpus is at %s and the Makefile pins %s.\n"+
			"The number this suite ratchets on is only comparable between runs "+
			"that count the same tests, so a checkout at another revision is not "+
			"a measurement of this engine. Run `make clean-wpt wpt` to take the "+
			"pinned one.", at, want)
	}
}

// TestTheCorpusHoldsWhatTheMakefileAsksFor.
//
// The commit is half of what makes a checkout reproducible and the sparse list
// is the other. "make wpt" writes the list once, when it first clones, and a
// checkout made before a directory was added to WPT_DIRS keeps the old list
// however many times the target is asked for again — so a local run and a fresh
// one can count different tests at the same commit.
//
// That is not hypothetical either: it is how a stray directory left in a local
// checkout made every measurement here disagree with CI by one document, quietly,
// for as long as it took to notice.
func TestTheCorpusHoldsWhatTheMakefileAsksFor(t *testing.T) {
	root := os.Getenv("WPT_TESTS")
	if root == "" {
		t.Skip("set WPT_TESTS (or run `make test-wpt`)")
	}
	want := makefileVariable(t, "WPT_DIRS")
	if want == nil {
		t.Fatal("the Makefile names no WPT_DIRS, so there is nothing to check against")
	}
	out, err := exec.Command("git", "-C", root, "sparse-checkout", "list").Output()
	if err != nil {
		t.Skipf("the checkout at %s has no sparse list: %v", root, err)
	}
	got := strings.Fields(string(out))
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("the checkout holds\n  %s\nand the Makefile asks for\n  %s\n"+
			"A directory in one and not the other is a different corpus, and the "+
			"number this suite ratchets on is only comparable between runs that "+
			"count the same tests. Run `make clean-wpt wpt`.",
			strings.Join(got, " "), strings.Join(want, " "))
	}
}

// makefileVariable reads a make variable's words, following the backslash
// continuations the list is written across.
func makefileVariable(t *testing.T, name string) []string {
	t.Helper()
	data := makefile(t)
	m := regexp.MustCompile(`(?ms)^` + name + `\s*:?=\s*(.*?[^\\])$`).FindSubmatch(data)
	if m == nil {
		return nil
	}
	return strings.Fields(strings.ReplaceAll(string(m[1]), "\\\n", " "))
}

// pinnedCommit reads WPT_COMMIT out of the Makefile.
//
// From the Makefile rather than from a constant beside this test, because two
// places naming the revision is two places to disagree — and the one that
// decides what is fetched is the Makefile.
func pinnedCommit(t *testing.T) string {
	t.Helper()
	m := regexp.MustCompile(`(?m)^WPT_COMMIT\s*:?=\s*([0-9a-f]{40})`).FindSubmatch(makefile(t))
	if m == nil {
		t.Fatal("the Makefile names no WPT_COMMIT, so the corpus is not pinned")
	}
	return string(m[1])
}

// makefile finds and reads the Makefile.
//
// The test binary runs in the package directory and the Makefile is one level
// up. Walking rather than assuming keeps this working if the package moves.
func makefile(t *testing.T) []byte {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		data, err := os.ReadFile(filepath.Join(dir, "Makefile"))
		if err == nil {
			return data
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("no Makefile above the package directory")
		}
		dir = parent
	}
}

// Every fetch goes through one command, and this is the check that it does.
//
// The rule is not tidiness. The corpora come from a dozen hosts and any of them
// can be slow: a CI run failed on unifoundry.com with "Failed to connect after
// 132634 ms", which stopped the build before a line of the engine had run. The
// FETCH variable is where that is dealt with — a connect timeout and five
// retries — and a bare curl added later is a fetch that quietly opts out of it,
// which is exactly the kind of thing nobody notices until a Tuesday afternoon.
//
// It is a rule about the Makefile and it lives beside the other one for the same
// reason: this is where the Makefile is already read.
func TestEveryFetchInTheMakefileRetries(t *testing.T) {
	lines := strings.Split(string(makefile(t)), "\n")
	def := regexp.MustCompile(`^FETCH\s*:?=\s*curl\b`)
	defined := false
	for i, line := range lines {
		if def.MatchString(line) {
			defined = true
			continue
		}
		// A comment may say "curl" — the note beside the variable does.
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		if regexp.MustCompile(`(^|[;&|\s(])curl\b`).MatchString(line) {
			t.Errorf("Makefile line %d fetches with a bare curl rather than "+
				"$(FETCH), so it has no connect timeout and no retries:\n\t%s",
				i+1, strings.TrimSpace(line))
		}
	}
	if !defined {
		t.Error("the Makefile defines no FETCH, so there is nothing for the check " +
			"above to be about")
	}
	for _, want := range []string{"--retry", "--connect-timeout", "--retry-all-errors"} {
		if !strings.Contains(strings.Join(makefileVariable(t, "FETCH"), " "), want) {
			t.Errorf("FETCH does not pass %s, so a host that never answers still "+
				"fails the build outright", want)
		}
	}
}
