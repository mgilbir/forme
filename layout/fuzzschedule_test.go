package layout

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every job in the weekly fuzz workflow runs one target, and runs it.
//
// The workflow's own note says why it exists: "go test" already executes a
// target's seed corpus, so the ordinary build catches whatever a seed reaches,
// and what the weekly run adds is the search. A job that names a target which is
// not in the package it names, or that names one ambiguously, adds nothing — and
// says nothing about it either, because the failure looks like any other red
// job.
//
// That is not hypothetical. "-fuzz" takes a *regular expression*, so
// "-fuzz FuzzDecodeWOFF" matched FuzzDecodeWOFF2 as well, and the toolchain
// refuses rather than choosing: "will not fuzz, -fuzz matches more than one fuzz
// test". The job had been failing every week without fuzzing a byte, and the
// target it was supposed to cover — the WOFF 1 decoder, added because an audit
// found it had no target at all — was covered by nothing.
//
// Two facts hold it now. The command anchors the expression, so a name that is a
// prefix of another is still exactly one target; and every scheduled pair names
// a target that is really in that package.
func TestTheFuzzScheduleNamesOneTargetEach(t *testing.T) {
	flow := string(readFileOrFail(t, filepath.Join("..", ".github", "workflows", "fuzz.yml")))

	// The command must anchor the target, which is what keeps a prefix from
	// matching two.
	if !strings.Contains(flow, `-fuzz '^${{ matrix.target }}$'`) {
		t.Errorf("the fuzz workflow does not anchor the target in its go test " +
			"command; -fuzz takes a regular expression, and an unanchored name " +
			"that is a prefix of another target matches both and fuzzes neither")
	}

	inPackage := declaredFuzzTargets(t)
	pairs := regexp.MustCompile(`- package: (\S+)\s*\n\s*target: (\S+)`).
		FindAllStringSubmatch(flow, -1)
	if len(pairs) == 0 {
		t.Fatal("the fuzz workflow schedules nothing, so this test says nothing")
	}
	for _, p := range pairs {
		pkg, target := p[1], p[2]
		found := false
		for _, name := range inPackage[pkg] {
			if name == target {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the fuzz workflow runs %s in %s and that package declares "+
				"%v; a job that names a target which is not there fuzzes nothing "+
				"and reports it as a failed build", target, pkg, inPackage[pkg])
		}
	}
}

// declaredFuzzTargets is every fuzz target in the repository, by the package
// path the workflow would name it with.
func declaredFuzzTargets(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	decl := regexp.MustCompile(`\nfunc (Fuzz\w*)`)
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		// "../font" is "./font" to the workflow, and ".." is ".".
		pkg := "./" + strings.TrimPrefix(strings.TrimPrefix(dir, ".."), "/")
		pkg = strings.TrimSuffix(pkg, "/")
		for _, m := range decl.FindAllStringSubmatch(string(b), -1) {
			out[pkg] = append(out[pkg], m[1])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no fuzz targets were found at all, so this test says nothing")
	}
	return out
}

func readFileOrFail(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}
