package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/cmd/internal/tables"
)

// The repository, from here.
const root = "../.."

// vowels is a table these tests can generate with nothing fetched: cmd/genvowel,
// the smallest generator, over a list the test writes itself. It used to be
// shape/indicvowel.go from the committed testdata/ms-use; those files are
// fetched at a pin now, and whether the committed table is what the pinned
// input makes is cmd/regenerate_test.go's question, not this file's.
var vowels = tables.Table{
	Out: "shape/indicvowel.go", Generator: "genvowel", Target: "t",
	Args: []string{"-source=fixture", "${FIXTURE}/IndicShapingInvalidCluster.txt"},
}

// broken is the same generator pointed at a file that is not there.
var broken = tables.Table{
	Out: "shape/never-written.go", Generator: "genvowel", Target: "t",
	Args: []string{"-source=fixture", "${FIXTURE}/no-such-file.txt"},
}

// fixture writes the list vowels reads, and returns the variables that find it.
func fixture(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	list := "# Date: fixture\n0905 0946 ; # DEVANAGARI LETTER A, VOWEL SIGN SHORT E\n" +
		"0905 093E ; # DEVANAGARI LETTER A, VOWEL SIGN AA\n"
	if err := os.WriteFile(filepath.Join(dir, "IndicShapingInvalidCluster.txt"), []byte(list), 0o644); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"FIXTURE": dir}
}

type recorder map[string][]byte

func (r recorder) write(path string, data []byte) error {
	r[path] = data
	return nil
}

// TestAFailureWritesNothing is the recipe this replaced, the other way round.
// "go run ./cmd/genX > table.go" emptied the table and then ran the generator;
// a target of several ran on past a failure and reported success. Here the
// second of two tables fails, and neither is written.
func TestAFailureWritesNothing(t *testing.T) {
	wrote := recorder{}
	err := run(root, []tables.Table{vowels, broken}, fixture(t), []string{"t"}, wrote.write, io.Discard)
	if err == nil {
		t.Fatal("a target with a generator that failed reported success")
	}
	if len(wrote) != 0 {
		t.Errorf("a target with a generator that failed wrote %d files", len(wrote))
	}
	t.Log(err)
}

// TestWhatIsWrittenIsTheTable: the success path writes the generator's output,
// formatted, to the file the manifest names.
func TestWhatIsWrittenIsTheTable(t *testing.T) {
	vars := fixture(t)
	wrote := recorder{}
	if err := run(root, []tables.Table{vowels}, vars, []string{"t"}, wrote.write, io.Discard); err != nil {
		t.Fatal(err)
	}
	want, err := vowels.Generate(root, vars)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(want, []byte("0x0905, 0x0946")) {
		t.Fatalf("the fixture's clusters are not in what the generator made:\n%s", want)
	}
	got, ok := wrote[root+"/"+vowels.Out]
	if !ok || len(wrote) != 1 {
		t.Fatalf("wrote %d files, and %s among them: %v", len(wrote), vowels.Out, ok)
	}
	if !bytes.Equal(got, want) {
		t.Error("what was written is not what the generator made")
	}
}

// TestNothingRunsOnAWrongName: a target the manifest does not know, and a
// variable the manifest names that was not passed, are failures before any
// generator runs — not a run with an argument that expanded to nothing.
func TestNothingRunsOnAWrongName(t *testing.T) {
	wrote := recorder{}
	if err := run(root, []tables.Table{vowels}, fixture(t), []string{"no-such-target"}, wrote.write, io.Discard); err == nil {
		t.Error("an unknown target was accepted")
	}
	needs := vowels
	needs.Args = []string{"${WHERE}/IndicShapingInvalidCluster.txt"}
	if err := run(root, []tables.Table{needs}, map[string]string{}, []string{"t"}, wrote.write, io.Discard); err == nil {
		t.Error("a table naming a variable with no value was generated")
	}
	if len(wrote) != 0 {
		t.Errorf("%d files written", len(wrote))
	}
}

// TestReplaceLeavesAWholeFileOrTheOldOne.
func TestReplaceLeavesAWholeFileOrTheOldOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "table.go")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tables.Replace(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new" {
		t.Errorf("the file holds %q", got)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*"))
	if len(left) != 1 {
		t.Errorf("Replace left %v behind", left)
	}
	if err := tables.Replace(filepath.Join(dir, "missing", "table.go"), []byte("x")); err == nil {
		t.Error("a write into a directory that is not there succeeded")
	}
}
