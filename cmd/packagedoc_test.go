package cmd

import (
	"go/parser"
	"go/token"
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

// TestEveryPackageHasADoc is the half the test above could not see. It checks
// the doc a file begins with, so a package with no doc at all passed it — font,
// bidi, paragraph and fonts/notosans had none, and `go doc` showed nothing for
// the four packages a reader of the engine most needs explained.
//
// Every package with code in it has a doc comment on some file's package
// clause: "Package x …" for a library, and "Command x …" for a program, which
// is the form `go doc` and the Go project's own tooling expect of one.
func TestEveryPackageHasADoc(t *testing.T) {
	type pkg struct {
		name string
		docs []string
	}
	pkgs := map[string]*pkg{}
	err := filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case info.IsDir():
			if name := info.Name(); name == "testdata" || strings.HasPrefix(name, ".") && name != ".." {
				return filepath.SkipDir
			}
			return nil
		case !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go"):
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil,
			parser.PackageClauseOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		dir := filepath.Dir(path)
		p := pkgs[dir]
		if p == nil {
			p = &pkg{name: f.Name.Name}
			pkgs[dir] = p
		}
		if f.Doc != nil {
			p.docs = append(p.docs, f.Doc.Text())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	libraries, commands := 0, 0
	for dir, p := range pkgs {
		want := "Package " + p.name + " "
		if p.name == "main" {
			want = "Command " + filepath.Base(dir) + " "
			commands++
		} else {
			libraries++
		}
		ok := false
		for _, d := range p.docs {
			if strings.HasPrefix(d, want) {
				ok = true
			}
		}
		if !ok {
			t.Errorf("%s has no doc beginning %q, so `go doc` says nothing about what it is", dir, want)
		}
	}
	if libraries < 10 || commands < 10 {
		t.Fatalf("%d libraries and %d commands were found, so the walk has stopped finding them",
			libraries, commands)
	}
	t.Logf("%d libraries and %d commands, each with a doc", libraries, commands)
}
