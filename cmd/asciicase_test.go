package cmd

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// ASCII case-insensitive means ASCII.
//
// CSS Syntax 3 §2.1 and the Infra standard define the comparison every keyword,
// property name, at-rule name, pseudo-class, unit, function name, media
// feature and named colour in CSS is made with, and HTML makes its element
// names, attribute names and listed attribute values the same way: A–Z match
// a–z, and nothing else matches anything but itself.
//
// Go's strings.EqualFold, ToLower and ToUpper are Unicode's case mapping, and
// the engine compared its syntax with them at some three hundred and thirty
// places. So U+212A KELVIN SIGN was a "k" and U+017F LONG S an "s" wherever a
// keyword was read — "@\u212Aeyframes", "border-style: ſolid", "10\u212AHz" —
// and a dotted capital I lowercased to two code points, which put every offset
// found in the lowered copy of a document one byte past the text it named.
// They go through internal/ascii now.
//
// This keeps them there. Nothing outside a test may call a Unicode case mapping
// of package strings or bytes unless it is in asciiCaseExempt, which says why.
// The check is by call site and by name, like the one for package unicode in
// pinnedunicode_test.go, and the unit of exemption is a function rather than a
// file, so that an exemption does not quietly cover the next keyword someone
// reads in the same file.

// unicodeCaseMappings are the functions of packages strings and bytes that map
// or fold case by Unicode's tables.
var unicodeCaseMappings = map[string]bool{
	"EqualFold": true, "ToLower": true, "ToUpper": true, "ToTitle": true, "Title": true,
	"ToLowerSpecial": true, "ToUpperSpecial": true, "ToTitleSpecial": true,
}

// asciiCaseExempt are the functions that fold by Unicode on purpose, keyed by
// file and function, and why. Each must still do so: an entry that has stopped
// is an exemption waiting for a use nobody meant.
var asciiCaseExempt = map[string]string{
	// Font family names are not syntax. CSS Fonts 4 §5.1 matches them by
	// Unicode's Default Caseless Matching, full case folding with no
	// tailoring, so a family named with a KELVIN SIGN *is* the same family as
	// one named with a "k". strings.ToLower is not that algorithm — it lowers
	// rather than folds, so "ß" and "SS" do not meet, and it answers from the
	// toolchain's Unicode rather than the pinned one — and the pinned
	// CaseFolding.txt is not among the tables. Until it is, these keep the
	// nearer of the two answers rather than taking ASCII's, which is further.
	"layout/fontface.go:(*documentFonts).faceFor": "matches a font family name, which CSS Fonts 4 §5.1 " +
		"folds by Unicode's Default Caseless Matching",
	"layout/fontface.go:loadFontFaces": "keys an @font-face family name, which CSS Fonts 4 §5.1 " +
		"folds by Unicode's Default Caseless Matching",
	"layout/facerun.go:(*layouter).familyListIsRestricted": "matches a font family name, which " +
		"CSS Fonts 4 §5.1 folds by Unicode's Default Caseless Matching",
	"layout/font.go:(*standardFonts).Face": "matches a font family name, which CSS Fonts 4 §5.1 " +
		"folds by Unicode's Default Caseless Matching",
}

// TestSyntaxIsComparedASCIICaseInsensitively.
func TestSyntaxIsComparedASCIICaseInsensitively(t *testing.T) {
	found, checked := unicodeCaseMappingsIn(t, root)
	used := map[string]bool{}
	var refused []string
	for _, f := range found {
		if _, ok := asciiCaseExempt[f.where]; ok {
			used[f.where] = true
			continue
		}
		refused = append(refused, f.String())
	}
	sort.Strings(refused)
	for _, r := range refused {
		t.Errorf("%s: Unicode case mapping, where CSS and HTML syntax is ASCII "+
			"case-insensitive; use internal/ascii, or add the function to "+
			"asciiCaseExempt with the reason it is not syntax", r)
	}
	for where, why := range asciiCaseExempt {
		if !used[where] {
			t.Errorf("%s is excused from this check because it %s, and no longer "+
				"maps case by Unicode; take it off the list", where, why)
		}
	}
	// There are nearly three hundred Go files that are not tests; a walk that
	// reached a handful has stopped looking.
	if checked < 250 {
		t.Fatalf("only %d Go files were read", checked)
	}
}

// TestTheASCIICaseCheckSeesACall holds the check to a tree it must refuse: a
// walk that parsed the files and looked at the wrong nodes would pass the real
// tree for the wrong reason.
func TestTheASCIICaseCheckSeesACall(t *testing.T) {
	dir := t.TempDir()
	src := `package p

import (
	"bytes"
	str "strings"
)

func keyword(v string) bool { return str.EqualFold(v, "auto") }

type r struct{}

func (r) name(b []byte) []byte { return bytes.ToLower(b) }

var upper = str.ToUpper

func fine(v string) bool { return str.HasPrefix(v, "a") }
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	// A test file is not the engine, and is not read.
	if err := os.WriteFile(filepath.Join(dir, "p_test.go"),
		[]byte("package p\n\nimport \"strings\"\n\nvar _ = strings.ToLower(\"A\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	found, checked := unicodeCaseMappingsIn(t, dir)
	var got []string
	for _, f := range found {
		got = append(got, f.String())
	}
	sort.Strings(got)
	want := []string{
		"p.go:(r).name calls bytes.ToLower",
		"p.go:keyword calls strings.EqualFold",
		"p.go:package level calls strings.ToUpper",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") || checked != 1 {
		t.Errorf("the check found, in %d files,\n  %s\nwant, in 1,\n  %s",
			checked, strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}

type caseMapping struct {
	where string // file:function
	call  string // strings.EqualFold
}

func (c caseMapping) String() string { return c.where + " calls " + c.call }

// unicodeCaseMappingsIn walks the Go files under dir that are not tests or
// test data and reports every use of a Unicode case mapping of package
// strings or bytes, by the function it is in.
func unicodeCaseMappingsIn(t *testing.T, dir string) (found []caseMapping, checked int) {
	t.Helper()
	return stringsCallsIn(t, dir, unicodeCaseMappings)
}

// stringsCallsIn walks the Go files under dir that are not tests or test data
// and reports every use of one of the named functions of package strings or
// bytes, by the function it is in. whitespace_test.go asks it for the ones
// that split and trim on Unicode's white space.
func stringsCallsIn(t *testing.T, dir string, names map[string]bool) (found []caseMapping, checked int) {
	t.Helper()
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if name := info.Name(); path != dir && (name == "testdata" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		checked++
		// The names the two packages are imported under in this file.
		pkgs := map[string]string{}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if p != "strings" && p != "bytes" {
				continue
			}
			name := p
			if imp.Name != nil {
				name = imp.Name.Name
			}
			if name == "." || name == "_" {
				t.Errorf("%s imports package %s as %q, which this check cannot follow", rel, p, name)
				continue
			}
			pkgs[name] = p
		}
		if len(pkgs) == 0 {
			return nil
		}
		for _, decl := range f.Decls {
			where := rel + ":package level"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				where = rel + ":" + funcName(fn)
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				id, ok := sel.X.(*ast.Ident)
				if !ok {
					return true
				}
				if p, ok := pkgs[id.Name]; ok && names[sel.Sel.Name] {
					found = append(found, caseMapping{where, p + "." + sel.Sel.Name})
				}
				return true
			})
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found, checked
}

// funcName is a function's name as a reader looks for it: "f", or "(T).m" and
// "(*T).m" for a method.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	var recv string
	switch t := fn.Recv.List[0].Type.(type) {
	case *ast.StarExpr:
		if id, ok := t.X.(*ast.Ident); ok {
			recv = "*" + id.Name
		}
	case *ast.Ident:
		recv = t.Name
	}
	return "(" + recv + ")." + fn.Name.Name
}
