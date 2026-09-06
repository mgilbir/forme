package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A package's doc comment names the package it is on.
//
// `go doc ./layout` showed "Package render lays HTML and CSS out onto a PDF
// page" and `go doc ./shape` showed "Package fonts embeds font programs into a
// PDF" — two names for packages this repository does not have, describing a
// program it is not. Both came from the repository this code grew out of, and
// both are the first thing a reader of the package is told.
//
// It is checked here, in cmd, because cmd is the one package that already
// spans the tree — see generators_test.go, which is the same idea about the
// generators.

// TestEveryPackageDocNamesItsOwnPackage.
func TestEveryPackageDocNamesItsOwnPackage(t *testing.T) {
	checked := 0
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case info.IsDir():
			if name := info.Name(); name == "testdata" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		case !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		// The doc comment is the one immediately above the package clause, so
		// it is the file's first line that says "// Package".
		if !strings.HasPrefix(text, "// Package ") {
			return nil
		}
		said := strings.Fields(strings.TrimPrefix(text, "// Package "))[0]
		// And what the file's own package clause says.
		i := strings.Index(text, "\npackage ")
		if i < 0 {
			t.Errorf("%s begins with a package doc and has no package clause", path)
			return nil
		}
		is := strings.Fields(text[i+len("\npackage "):])[0]
		checked++
		if said != is {
			t.Errorf("%s: the doc says \"Package %s\" and the file is in package %s — "+
				"which is what `go doc` shows anyone who asks what this package is",
				path, said, is)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if checked < 5 {
		t.Fatalf("only %d package docs were found, so this test says almost nothing", checked)
	}
	t.Logf("%d package docs checked", checked)
}
