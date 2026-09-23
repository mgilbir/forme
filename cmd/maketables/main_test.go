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

// vowels is a table whose input is committed, so these tests need nothing
// fetched: shape/indicvowel.go, from testdata/ms-use.
var vowels = tables.Table{
	Out: "shape/indicvowel.go", Generator: "genvowel", Target: "t",
	Args: []string{"testdata/ms-use/IndicShapingInvalidCluster.txt"},
}

// broken is the same generator pointed at a file that is not there.
var broken = tables.Table{
	Out: "shape/never-written.go", Generator: "genvowel", Target: "t",
	Args: []string{"testdata/ms-use/no-such-file.txt"},
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
	err := run(root, []tables.Table{vowels, broken}, nil, []string{"t"}, wrote.write, io.Discard)
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
	wrote := recorder{}
	if err := run(root, []tables.Table{vowels}, nil, []string{"t"}, wrote.write, io.Discard); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join(root, vowels.Out))
	if err != nil {
		t.Fatal(err)
	}
	got, ok := wrote[root+"/"+vowels.Out]
	if !ok || len(wrote) != 1 {
		t.Fatalf("wrote %d files, and %s among them: %v", len(wrote), vowels.Out, ok)
	}
	if !bytes.Equal(got, want) {
		t.Error("what was written is not the committed table")
	}
}

// TestNothingRunsOnAWrongName: a target the manifest does not know, and a
// variable the manifest names that was not passed, are failures before any
// generator runs — not a run with an argument that expanded to nothing.
func TestNothingRunsOnAWrongName(t *testing.T) {
	wrote := recorder{}
	if err := run(root, []tables.Table{vowels}, nil, []string{"no-such-target"}, wrote.write, io.Discard); err == nil {
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
