package layout

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestEveryFuzzTargetIsScheduled: a target the weekly workflow does not name
// has never been fuzzed for longer than its seed corpus takes.
//
// The count beside this — "twenty-eight of them scheduled weekly" — is not this
// question. It compares two numbers, and two numbers agree for the wrong reason
// as easily as for the right one: the matrix named twenty-three entries and the
// repository held twenty-eight targets, and the README's "twenty-three" was
// correct about the matrix while three decoders that read untrusted bytes had
// never been fuzzed at all.
//
// What hid it is worth writing down, because it is a trap a count cannot see.
// Two packages declare a target called FuzzParse — css and html — so the
// repository's twenty-eight targets carry twenty-seven distinct *names*. A list
// checked by name looks complete while covering one of the two, and the one it
// covered was not the HTML parser.
//
// So this compares the sets, by package and name together, and says which way
// they differ. A target that should genuinely not be scheduled goes in the list
// below with the reason beside it — there are none today, and an empty list is
// the honest state rather than a placeholder.
var fuzzTargetsNotScheduled = map[string]string{}

func TestEveryFuzzTargetIsScheduled(t *testing.T) {
	root := filepath.Join("..")

	// Every target in the repository, as "package/Name".
	declared := map[string]bool{}
	decl := regexp.MustCompile(`(?m)^func (Fuzz[A-Za-z0-9_]+)\(`)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		pkg := filepath.Base(filepath.Dir(path))
		for _, m := range decl.FindAllStringSubmatch(string(b), -1) {
			declared[pkg+"/"+m[1]] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if len(declared) == 0 {
		t.Fatal("no fuzz targets were found at all, so this test says nothing")
	}

	flow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "fuzz.yml"))
	if err != nil {
		t.Fatalf("reading the fuzz workflow: %v", err)
	}
	entry := regexp.MustCompile(`package:\s*\./(\S+)\s*\n\s*target:\s*(\S+)`)
	scheduled := map[string]bool{}
	for _, m := range entry.FindAllStringSubmatch(string(flow), -1) {
		scheduled[m[1]+"/"+m[2]] = true
	}
	if len(scheduled) == 0 {
		t.Fatal("the workflow schedules nothing, so this test says nothing")
	}

	var missing, unknown []string
	for k := range declared {
		if !scheduled[k] && fuzzTargetsNotScheduled[k] == "" {
			missing = append(missing, k)
		}
	}
	for k := range scheduled {
		if !declared[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(unknown)

	if len(missing) > 0 {
		t.Errorf("%d fuzz targets are never scheduled: %s.\nA target nothing "+
			"schedules is one that has never run for longer than its seeds "+
			"take. Add it to .github/workflows/fuzz.yml, or to "+
			"fuzzTargetsNotScheduled with the reason it should not be",
			len(missing), strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		t.Errorf("the workflow schedules %s, which no package declares; the "+
			"run would match nothing and pass in ten seconds",
			strings.Join(unknown, ", "))
	}
	for k, why := range fuzzTargetsNotScheduled {
		if !declared[k] {
			t.Errorf("%s is excused from scheduling (%q) and does not exist", k, why)
		}
		if scheduled[k] {
			t.Errorf("%s is both scheduled and excused (%q)", k, why)
		}
	}
}
