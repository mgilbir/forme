package layout

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The blank assignment that keeps a name alive after nothing uses it.
//
// "_ = x" compiles, says nothing, and is what is left when the last real use of
// a variable is deleted. Seven of them were spread through this module: a loop
// variable nobody read, two lookup flags a comment explained were not needed
// here, an offsets slice thrown away by the line that took it, a parse count
// used two lines above, and two function parameters that had stopped being
// arguments to anything.
//
// None of them is wrong and every one of them is a lie about what the code
// needs. They are also invisible to the compiler and to go vet, which is why
// they last: the statement exists precisely to make the compiler stop asking.
//
// So the check is here instead, over the module's own sources rather than over
// this package's — a rule about how code is written belongs wherever the code
// is.

// blankAssign matches a statement whose whole content is discarding a name.
var blankAssign = regexp.MustCompile(`(?m)^\s*_ = [a-zA-Z][a-zA-Z0-9_.]*\s*$`)

// TestNothingIsKeptAliveByABlankAssignment.
func TestNothingIsKeptAliveByABlankAssignment(t *testing.T) {
	root := ".."
	files := 0
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Neither the fetched corpora nor the module cache is this
			// module's code.
			switch info.Name() {
			case ".git", "testdata", "docs":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files++
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range blankAssign.FindAllString(string(src), -1) {
			t.Errorf("%s has %q: either the name is needed, in which case use "+
				"it, or it is not, in which case take it out — a blank "+
				"assignment only stops the compiler asking",
				path, strings.TrimSpace(m))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// The walk is what everything above rests on.
	if files < 100 {
		t.Errorf("only %d source files were read; the walk is not reaching the "+
			"module", files)
	}
}
