package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mgilbir/forme/cmd/internal/tables"
)

// The tables are generated, and the only check that they still are is to
// generate them again.
//
// Three ways for that to stop being true were live at once, and none of them
// could be seen from inside the repository:
//
//   - cmd/genvowel's input, IndicShapingInvalidCluster.txt, was not vendored
//     with the other two ms-use files, so `make shapetables` stopped on it;
//   - the Makefile passed $(UCD)/DerivedBidiClass.txt, which is under
//     extracted/ in the database, so `make bidi-tables` could not have run;
//   - cmd/geneastasian takes four files and the Makefile gave it two, so
//     `make eastasian` printed its usage line and truncated the table.
//
// And with no target able to run, shape/canonical.go had drifted from what
// cmd/gencanonical emits: a comment naming a package that had been renamed.
//
// Which is the shape of the whole problem. A generated file whose generator
// cannot be run is a typed file with a comment on it, and the comment is the
// only thing saying otherwise. So this runs them.
//
// It used to find them by matching "go run ./cmd/gen" lines of the Makefile,
// and a line it could not expand it skipped, without saying so; the floor it
// checked the count against was below the real count, so a recipe could drop
// out of the check one Makefile edit at a time. It covered the tables read from
// the Unicode Character Database and nothing else: a hand edit to thaidict.go
// or englishhyphens.go would have passed. Now the list is
// cmd/internal/tables.Manifest, which is also what the Makefile runs, every
// generator under cmd/ has to be in it or say why not, and every table in it is
// checked here — the dictionaries, the hyphenation patterns and the phrase
// model with the rest. Nothing is written to the tree: each table is generated
// through the same tables.Generate the Makefile's cmd/maketables uses, and
// compared with the committed file.
//
// A table whose inputs are not in this checkout is skipped, and says which make
// target fetches them. Under TABLE_INPUTS=required — which `make test-corpora`
// sets, having fetched them — it is a failure instead, because there a skip
// would be a check that did not run.

// root is the repository, relative to this package.
const root = ".."

// TestEveryTableIsWhatItsGeneratorProduces.
func TestEveryTableIsWhatItsGeneratorProduces(t *testing.T) {
	vars := makeVars(t)
	required := os.Getenv("TABLE_INPUTS") == "required"
	var checked, skipped atomic.Int32
	t.Cleanup(func() {
		t.Logf("%d of %d tables regenerated and compared; %d skipped for want of their inputs",
			checked.Load(), len(tables.Manifest), skipped.Load())
	})
	for _, tb := range tables.Manifest {
		t.Run(filepath.Base(tb.Out), func(t *testing.T) {
			t.Parallel()
			inputs, err := tb.ExpandedInputs(vars)
			if err != nil {
				t.Fatal(err)
			}
			for _, in := range inputs {
				if _, err := os.Stat(filepath.Join(root, in)); err != nil {
					skipped.Add(1)
					if required {
						t.Fatalf("%s is not here, and TABLE_INPUTS=required says the "+
							"inputs were fetched; `make %s` fetches it", in, tb.Target)
					}
					t.Skipf("%s is not in this checkout, so %s cannot be checked; "+
						"`make %s` fetches it", in, tb.Out, tb.Target)
				}
			}
			got, err := tb.Generate(root, vars)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join(root, tb.Out))
			if err != nil {
				t.Fatalf("reading the committed table: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s is not what cmd/%s produces from its pinned inputs.\n"+
					"Either the generator changed and the table was not "+
					"regenerated, or the table was edited by hand — and a "+
					"table that cannot be regenerated is one nobody can check. "+
					"`make %s` regenerates it.\n%s",
					tb.Out, tb.Generator, tb.Target, firstDifference(got, want))
			}
			checked.Add(1)
		})
	}
}

// TestEveryGeneratorIsInTheManifest is what keeps the list whole. A generator
// that is in neither the manifest nor its exemptions is one nothing runs and
// nothing checks, which is how seven of them were before it.
func TestEveryGeneratorIsInTheManifest(t *testing.T) {
	dirs, err := filepath.Glob("gen*")
	if err != nil {
		t.Fatal(err)
	}
	inManifest := map[string]bool{}
	for _, tb := range tables.Manifest {
		inManifest[tb.Generator] = true
		if _, err := os.Stat(filepath.Join(tb.Generator, "main.go")); err != nil {
			t.Errorf("%s names cmd/%s, which is not here", tb.Out, tb.Generator)
		}
	}
	n := 0
	for _, d := range dirs {
		if info, err := os.Stat(d); err != nil || !info.IsDir() {
			continue
		}
		n++
		reason, exempt := tables.Exempt[d]
		switch {
		case inManifest[d] && exempt:
			t.Errorf("cmd/%s is in the manifest and exempt from it", d)
		case exempt && strings.TrimSpace(reason) == "":
			t.Errorf("cmd/%s is exempt and does not say why", d)
		case !inManifest[d] && !exempt:
			t.Errorf("cmd/%s is a generator nothing runs: it is not in "+
				"cmd/internal/tables.Manifest and not exempt from it", d)
		}
	}
	for d := range tables.Exempt {
		if _, err := os.Stat(d); err != nil {
			t.Errorf("cmd/%s is exempt and does not exist", d)
		}
	}
	if n < len(inManifest) {
		t.Fatalf("%d generator directories found and the manifest names %d; the glob "+
			"has stopped finding them", n, len(inManifest))
	}
}

// TestTheMakefileRunsTheManifest: every target the manifest names is a rule in
// the Makefile whose recipe runs cmd/maketables for that target, the variables
// the manifest refers to are the ones the Makefile passes, and nothing in the
// Makefile runs a generator any other way — which was the redirection that
// emptied a committed table when the generator failed.
func TestTheMakefileRunsTheManifest(t *testing.T) {
	lines := makefileLines(t)
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			continue
		}
		if strings.Contains(l, "./cmd/gen") {
			t.Errorf("Makefile line %d runs a generator directly: %s", i+1, strings.TrimSpace(l))
		}
	}
	targets := map[string]bool{}
	for _, tb := range tables.Manifest {
		targets[tb.Target] = true
	}
	for target := range targets {
		rule := regexp.MustCompile(`^` + regexp.QuoteMeta(target) + `:`)
		found := false
		for i, l := range lines {
			if !rule.MatchString(l) {
				continue
			}
			found = true
			if i+1 >= len(lines) || strings.TrimSpace(lines[i+1]) != "$(MAKETABLES) "+target {
				t.Errorf("the Makefile's %s rule does not run \"$(MAKETABLES) %s\"", target, target)
			}
		}
		if !found {
			t.Errorf("the manifest regenerates tables under the target %q, and the "+
				"Makefile has no such rule", target)
		}
	}

	passed := strings.Fields(makeVars(t)["TABLE_VARS"])
	sort.Strings(passed)
	named := tables.Names(tables.Manifest)
	sort.Strings(named)
	if strings.Join(passed, " ") != strings.Join(named, " ") {
		t.Errorf("the Makefile's TABLE_VARS are\n  %v\nand the manifest names\n  %v", passed, named)
	}
}

// TestEveryTableNamesItsPin. A table taken from a fetched upstream says which
// revision of it — the URL at its commit, or the digest — so that the table can
// be made again from what it was made from. This needs nothing fetched: it
// reads the Makefile's pins and the committed files.
func TestEveryTableNamesItsPin(t *testing.T) {
	vars := makeVars(t)
	pinned := 0
	for _, tb := range tables.Manifest {
		src, err := os.ReadFile(filepath.Join(root, tb.Out))
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range tb.Args {
			for _, flag := range []string{"-source=", "-sha256="} {
				v, ok := strings.CutPrefix(a, flag)
				if !ok {
					continue
				}
				want, err := tables.Expand(v, vars)
				if err != nil {
					t.Fatal(err)
				}
				pinned++
				if !bytes.Contains(src, []byte(want)) {
					t.Errorf("%s does not say it is from %s; it was generated from "+
						"another pin, or edited", tb.Out, want)
				}
			}
		}
	}
	if pinned < 14 {
		t.Fatalf("only %d pins found in the manifest; this has stopped reading them", pinned)
	}
}

// makefileLines is the Makefile, with continuation lines joined onto the line
// they continue.
func makefileLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	var out []string
	var joined string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasSuffix(line, "\\") {
			joined += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		out = append(out, joined+line)
		joined = ""
	}
	return out
}

// assignment is a Makefile variable being given a value.
var assignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\s*(?::=|\?=|=)\s*(.*)$`)

// makeRef is a reference to one in a value.
var makeRef = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z0-9_]*)\)`)

// makeVars are the Makefile's variables, with what they expand to here.
//
// They are read from the Makefile rather than written down, so that where the
// inputs are and which pin they are at cannot be one thing there and another
// here — which is the whole failure this file is about, one level up. The first
// assignment of a name is the one taken, as "?=" makes it for UCD, and a value
// that refers to a variable the Makefile does not define fails the test rather
// than expanding to nothing.
func makeVars(t *testing.T) map[string]string {
	t.Helper()
	raw := map[string]string{}
	for _, l := range makefileLines(t) {
		if m := assignment.FindStringSubmatch(l); m != nil {
			if _, seen := raw[m[1]]; !seen {
				raw[m[1]] = strings.TrimSpace(m[2])
			}
		}
	}
	var expand func(name string, depth int) string
	expand = func(name string, depth int) string {
		v, ok := raw[name]
		if !ok {
			t.Fatalf("the Makefile does not define %s", name)
		}
		if depth > 10 {
			t.Fatalf("%s refers to itself", name)
		}
		return makeRef.ReplaceAllStringFunc(v, func(ref string) string {
			return expand(makeRef.FindStringSubmatch(ref)[1], depth+1)
		})
	}
	out := map[string]string{}
	for _, name := range append(tables.Names(tables.Manifest), "TABLE_VARS") {
		v := expand(name, 0)
		if strings.Contains(v, "$(") {
			t.Fatalf("%s expands to %q, which this cannot finish expanding", name, v)
		}
		out[name] = v
	}
	return out
}

// unicodeVersion is the release the Makefile fetches.
func unicodeVersion(t *testing.T) string {
	t.Helper()
	return makeVars(t)["UNICODE_VERSION"]
}

// firstDifference names the first line that differs, since a table is tens of
// thousands of lines and "they are not equal" is not a place to look.
func firstDifference(got, want []byte) string {
	g := strings.Split(string(got), "\n")
	w := strings.Split(string(want), "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return "first difference at line " + itoa(i+1) + ":\n" +
				"  generated: " + g[i] + "\n" +
				"  committed: " + w[i]
		}
	}
	return "the shorter is a prefix of the longer: " +
		itoa(len(g)) + " lines generated, " + itoa(len(w)) + " committed"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
