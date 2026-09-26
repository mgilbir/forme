package font

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"
)

// Code this package keeps has a caller in it.
//
// The package came from a PDF/A validator, and kept that validator's readers
// after it came: a Type 1 parser, the code-to-glyph rule of ISO 32000-1
// 9.6.6.4, the Adobe standard Latin and Symbol repertoires. They were
// unexported when the validator went, so nothing outside could call them, and
// nothing inside did — every caller was a test. They were held to the same
// standard as the readers shape uses, fuzzed in CI and fixed when an audit
// found a fault in them, for no caller at all. They are deleted.
//
// This is what keeps the shape from coming back. An unexported function,
// variable, constant or type that no file of the package outside its tests
// names cannot be reached from anywhere, whatever its tests say about it; a
// test-only convenience belongs in a test file. The check is by name, so a
// local that shadows a declaration counts as a use of it: it can miss dead
// code, and it cannot call live code dead. Methods are not checked, because a
// method can be reached through an interface without being named.

// TestEveryUnexportedDeclarationHasACaller.
func TestEveryUnexportedDeclarationHasACaller(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	declared := map[string]token.Pos{}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, f)
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Recv == nil {
					declared[d.Name.Name] = d.Name.Pos()
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						for _, n := range s.Names {
							declared[n.Name] = n.Pos()
						}
					case *ast.TypeSpec:
						declared[s.Name.Name] = s.Name.Pos()
					}
				}
			}
		}
	}
	used := map[string]bool{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if id, ok := n.(*ast.Ident); ok && declared[id.Name] != id.Pos() {
				used[id.Name] = true
			}
			return true
		})
	}
	var dead []string
	for name := range declared {
		if name == "_" || name == "init" || ast.IsExported(name) || used[name] {
			continue
		}
		dead = append(dead, name)
	}
	sort.Strings(dead)
	for _, name := range dead {
		t.Errorf("%s: %s is declared and nothing in the package outside its tests "+
			"names it; delete it, or move it to a test file if only tests want it",
			fset.Position(declared[name]), name)
	}
	if len(declared) < 50 {
		t.Fatalf("only %d top-level declarations were read; this has stopped looking", len(declared))
	}
}
