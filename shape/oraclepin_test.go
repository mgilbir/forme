package shape

import (
	"bufio"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The HarfBuzz oracle is one release, and the expectation files have to be its
// answers.
//
// They were not. The oracle was installed with an unpinned `pip install
// uharfbuzz`, so each file was HarfBuzz at whatever release PyPI served the day
// it was regenerated: five were 14.3.0, one 14.4.0 and two 14.5.0, beside a
// Makefile fetching HarfBuzz's data files at 14.5.0. Nothing was wrong while
// the three releases happened to agree on these strings, but nothing would have
// said so the day they did not — a regeneration picking up a new release looks
// exactly like this package changing, and a difference between two HarfBuzz
// releases is not a defect in either.
//
// The release is pinned now: the Makefile's HARFBUZZ_VERSION is the HarfBuzz,
// testdata/harfbuzz/requirements.txt the uharfbuzz carrying it, installed by
// digest (make hbenv). Every file the oracles write records both, and this
// refuses any file that records anything else. The files are found by what
// they are rather than listed, so a new oracle cannot escape it.

// pinnedOracle is the release the oracle is pinned to: HarfBuzz from the
// Makefile, uharfbuzz and fontTools from the requirements file.
type pinnedOracle struct {
	harfbuzz, uharfbuzz, fonttools string
}

func readPinnedOracle(t *testing.T) pinnedOracle {
	t.Helper()
	mk, err := os.ReadFile("../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	req, err := os.ReadFile(filepath.Join(harfbuzzDir, "requirements.txt"))
	if err != nil {
		t.Fatal(err)
	}
	one := func(src []byte, re, what string) string {
		m := regexp.MustCompile(re).FindAllSubmatch(src, -1)
		if len(m) != 1 {
			t.Fatalf("%d pins of %s, want exactly one", len(m), what)
		}
		return string(m[0][1])
	}
	// A pin with no digest after it is a version PyPI could serve other bytes
	// for, so a pin is only read with its first digest.
	pinned := func(name string) string {
		return one(req, `(?m)^`+name+`==(\S+) \\\n\s+--hash=sha256:[0-9a-f]{64}\b`,
			name+" with a digest in requirements.txt")
	}
	return pinnedOracle{
		harfbuzz:  one(mk, `(?m)^HARFBUZZ_VERSION := (\S+)$`, "HARFBUZZ_VERSION in the Makefile"),
		uharfbuzz: pinned("uharfbuzz"),
		fonttools: pinned("fonttools"),
	}
}

// oracleHeader reads what a generated file says produced it: the "# key value"
// lines at the top of the files testdata/harfbuzz holds, and the "key value"
// lines above the first glyph in testdata/varinstance's.
func oracleHeader(t *testing.T, path string) map[string][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	varinstance := filepath.Base(filepath.Dir(path)) == "varinstance"
	header := map[string][]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := sc.Text()
		text, comment := strings.CutPrefix(line, "#")
		if !comment {
			if !varinstance {
				break
			}
			if k, _, _ := strings.Cut(line, " "); k == "s" || k == "c" || k == "e" {
				break
			}
		}
		if k, v, ok := strings.Cut(strings.TrimSpace(text), " "); ok {
			header[k] = append(header[k], v)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return header
}

// oracleReleaseProblems is the refusal: what is wrong with a file's header, if
// it does not say, once, that it is the pinned release's answers.
func oracleReleaseProblems(pin pinnedOracle, path string, header map[string][]string) []string {
	want := map[string]string{"harfbuzz": pin.harfbuzz, "uharfbuzz": pin.uharfbuzz}
	switch {
	case filepath.Base(path) == "usecategories.expected.txt":
		// HarfBuzz's own generator, run from a source checkout rather than
		// through uharfbuzz; it records the checkout it was run from.
		want = map[string]string{"generator": "harfbuzz-" + pin.harfbuzz}
	case filepath.Base(filepath.Dir(path)) == "varinstance":
		want["fonttools"] = pin.fonttools
	}
	var problems []string
	for _, k := range slices.Sorted(maps.Keys(want)) {
		v := want[k]
		switch got := header[k]; {
		case len(got) == 0:
			problems = append(problems, fmt.Sprintf("%s does not say which %s produced it, "+
				"so nothing ties it to the pinned %s %s", path, k, k, v))
		case len(got) > 1:
			problems = append(problems, fmt.Sprintf("%s names %d %s releases: %q", path, len(got), k, got))
		case got[0] != v:
			problems = append(problems, fmt.Sprintf("%s is %s %s's answers, and the oracle is "+
				"pinned to %s %s", path, k, got[0], k, v))
		}
	}
	return problems
}

func TestTheOracleIsPinned(t *testing.T) {
	pin := readPinnedOracle(t)
	var files []string
	for _, pattern := range []string{
		filepath.Join(harfbuzzDir, "*.expected.txt"),
		filepath.Join(varInstanceDir, "*.txt"),
	} {
		m, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, m...)
	}
	// Nine in testdata/harfbuzz and eight in testdata/varinstance. Fewer means
	// the patterns stopped finding them, and the test would pass on nothing.
	if len(files) < 17 {
		t.Fatalf("found %d expectation files, want at least 17: %q", len(files), files)
	}
	for _, path := range files {
		for _, p := range oracleReleaseProblems(pin, path, oracleHeader(t, path)) {
			t.Errorf("%s.\nRegenerate it with the pinned oracle (make hbenv, then make hboracles), "+
				"or move the pin and regenerate every file.", p)
		}
	}
}

// TestTheOraclePinRefusesAnotherRelease is the test above shown to fail: the
// same check, handed headers that are wrong in each way it looks for.
func TestTheOraclePinRefusesAnotherRelease(t *testing.T) {
	pin := readPinnedOracle(t)
	path := filepath.Join(harfbuzzDir, "khmer.expected.txt")
	good := oracleHeader(t, path)
	for name, edit := range map[string]func(h map[string][]string){
		"another HarfBuzz":  func(h map[string][]string) { h["harfbuzz"] = []string{"14.3.0"} },
		"another uharfbuzz": func(h map[string][]string) { h["uharfbuzz"] = []string{"0.56.0"} },
		"no HarfBuzz":       func(h map[string][]string) { delete(h, "harfbuzz") },
		"two HarfBuzzes": func(h map[string][]string) {
			h["harfbuzz"] = []string{pin.harfbuzz, "14.4.0"}
		},
	} {
		h := maps.Clone(good)
		edit(h)
		if len(oracleReleaseProblems(pin, path, h)) == 0 {
			t.Errorf("%s: a header naming %v passed", name, h)
		}
	}
	if p := oracleReleaseProblems(pin, path, good); len(p) > 0 {
		t.Errorf("%s as committed fails the check: %q", path, p)
	}
}

// refuseUnpinnedOracle stops a comparison against a file that is not the pinned
// release's answers. The test above says the same about every file at once;
// this says it in the test that would otherwise go on to assert them.
func refuseUnpinnedOracle(t *testing.T, path string) {
	t.Helper()
	if p := oracleReleaseProblems(readPinnedOracle(t), path, oracleHeader(t, path)); len(p) > 0 {
		t.Fatalf("%s.\nRegenerate it with the pinned oracle (make hbenv, then make hboracles).",
			strings.Join(p, ";\n"))
	}
}
