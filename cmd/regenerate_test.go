package cmd

import (
	"bytes"
	"go/format"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
// It reads the Makefile rather than a list written here, because a list written
// here is a second place for the arguments to be wrong in. Nothing is written
// to the tree: each generator's output is captured, formatted the way the
// Makefile's `gofmt -w` would format it, and compared with the committed file.

// ucdDir is the fetched database, relative to this package.
const ucdDir = "../testdata/ucd"

// ucdMarker says the fetch finished.
const ucdMarker = "UnicodeData.txt"

// generatorRun is one `go run ./cmd/gen...` line of the Makefile.
type generatorRun struct {
	args []string // the generator's arguments, with $(UCD) expanded
	out  string   // the file it writes, relative to the repository root
	flag bool     // it writes through -out rather than through a redirection
}

// TestEveryTableIsWhatItsGeneratorProduces.
func TestEveryTableIsWhatItsGeneratorProduces(t *testing.T) {
	if _, err := os.Stat(filepath.Join(ucdDir, ucdMarker)); err != nil {
		t.Skip("the Unicode Character Database is not in this checkout; run `make ucd`")
	}
	runs := generatorRuns(t)
	if len(runs) < 15 {
		t.Fatalf("the Makefile states %d generator runs over the database, and "+
			"there are eighteen generators; the parse below has gone wrong "+
			"rather than the Makefile", len(runs))
	}
	for _, r := range runs {
		t.Run(filepath.Base(r.out), func(t *testing.T) {
			got := generate(t, r)
			want, err := os.ReadFile(filepath.Join("..", r.out))
			if err != nil {
				t.Fatalf("reading the committed table: %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s is not what %s produces from Unicode %s.\n"+
					"Either the generator changed and the table was not "+
					"regenerated, or the table was edited by hand — and a "+
					"table that cannot be regenerated is one nobody can check.\n"+
					"%s", r.out, r.args[0], unicodeVersion(t), firstDifference(got, want))
			}
		})
	}
}

// generate runs one generator and returns its output, formatted.
func generate(t *testing.T, r generatorRun) []byte {
	t.Helper()
	args := append([]string{"run"}, r.args...)
	var out, errs bytes.Buffer
	if r.flag {
		// A generator that writes the file itself is pointed at a temporary one,
		// so that a run of the tests never touches the tree.
		tmp := filepath.Join(t.TempDir(), filepath.Base(r.out))
		for i := range args {
			if args[i] == r.out {
				args[i] = tmp
			}
		}
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = "..", &errs, &errs
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", r.args[0], err, errs.String())
		}
		data, err := os.ReadFile(tmp)
		if err != nil {
			t.Fatalf("%s wrote nothing: %v", r.args[0], err)
		}
		out.Write(data)
	} else {
		cmd := exec.Command("go", args...)
		cmd.Dir, cmd.Stdout, cmd.Stderr = "..", &out, &errs
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s", r.args[0], err, errs.String())
		}
	}
	// The Makefile formats every one of these, so the comparison has to be
	// against the formatted text or every table would differ.
	formatted, err := format.Source(out.Bytes())
	if err != nil {
		t.Fatalf("%s emitted something that does not parse: %v", r.args[0], err)
	}
	return formatted
}

// generatorRuns reads the Makefile and returns every generator invocation that
// reads the database.
//
// Lines are joined across their continuations, and only the ones whose
// arguments are fully expanded by substituting $(UCD) are taken — the
// generators that read a different fetched corpus are somebody else's check.
func generatorRuns(t *testing.T) []generatorRun {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	var out []generatorRun
	var joined string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasSuffix(line, "\\") {
			joined += strings.TrimSuffix(line, "\\") + " "
			continue
		}
		full := strings.TrimSpace(joined + line)
		joined = ""
		if !strings.HasPrefix(full, "go run ./cmd/gen") {
			continue
		}
		for name, value := range makeVars(t) {
			full = strings.ReplaceAll(full, "$("+name+")", value)
		}
		if strings.Contains(full, "$(") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(full, "go run "))
		r := generatorRun{}
		for i := 0; i < len(fields); i++ {
			switch {
			case fields[i] == ">" && i+1 < len(fields):
				r.out = fields[i+1]
				i++
			case fields[i] == "-out" && i+1 < len(fields):
				r.out, r.flag = fields[i+1], true
				r.args = append(r.args, fields[i], fields[i+1])
				i++
			default:
				r.args = append(r.args, fields[i])
			}
		}
		if r.out == "" {
			t.Errorf("this recipe writes nowhere this can see: %s", full)
			continue
		}
		out = append(out, r)
	}
	return out
}

// makeVars are the Makefile variables a generator recipe names, with what they
// expand to here.
//
// They are read from the Makefile rather than written down, so that the
// database's directory and the release it holds cannot be one thing there and
// another here — which is the whole failure this file is about, one level up.
func makeVars(t *testing.T) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "Makefile"))
	if err != nil {
		t.Fatalf("reading the Makefile: %v", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		for _, name := range []string{"UCD", "UCD_DIR", "UNICODE_VERSION"} {
			for _, op := range []string{" ?= ", " := ", " = "} {
				if v, ok := strings.CutPrefix(line, name+op); ok {
					if _, seen := out[name]; !seen {
						out[name] = strings.TrimSpace(v)
					}
				}
			}
		}
	}
	// UCD defaults to UCD_DIR, and the recipes name UCD.
	if out["UCD"] == "$(UCD_DIR)" {
		out["UCD"] = out["UCD_DIR"]
	}
	for _, name := range []string{"UCD", "UNICODE_VERSION"} {
		if out[name] == "" {
			t.Fatalf("the Makefile no longer sets %s, so the recipes below "+
				"cannot be expanded", name)
		}
	}
	return out
}

// unicodeVersion is what the Makefile fetched, for the failure message: a table
// generated from one release and compared against another differs in thousands
// of lines, and the reason is not in the diff.
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
