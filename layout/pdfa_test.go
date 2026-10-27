package layout

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"reflect"
	"strings"
	"testing"

	"github.com/mgilbir/forme/fonts/notosans"
)

// pdfaFonts is the standard set with one embedded family, "Embedded", over it:
// the embedded Noto Sans, which is a real font program a backend embeds.
func pdfaFonts(t *testing.T) FontSet {
	t.Helper()
	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	return namedFaceSet{family: "Embedded", face: face, standard: StandardFonts()}
}

func embeddedFindings(c Composed) []Finding {
	var out []Finding
	for _, f := range c.Findings {
		if f.Rule == RuleFontNotEmbedded {
			out = append(out, f)
		}
	}
	return out
}

// TestAPDFAPageRefusesAStandardFace is the acceptance of issue #955: a page
// that uses sans-serif, monospace and an embedded font is refused under PDF/A,
// with a finding per standard face that names the family the document wrote,
// and the embedded font is not one of them.
func TestAPDFAPageRefusesAStandardFace(t *testing.T) {
	in := Input{
		HTML: `<p id="s" style="font-family: sans-serif">Sans</p>` +
			`<p id="m" style="font-family: monospace">Mono</p>` +
			`<p id="e" style="font-family: Embedded">Embedded</p>`,
		Fonts: pdfaFonts(t),
	}
	got := Compose(in, Options{PDFA: true})
	if !got.Refused {
		t.Fatal("a PDF/A page set partly in Helvetica and Courier was not refused")
	}
	findings := embeddedFindings(got)
	want := map[string]string{"Helvetica": "sans-serif", "Courier": "monospace"}
	if len(findings) != len(want) {
		t.Fatalf("got %d %s findings, want one per standard face (%d): %v",
			len(findings), RuleFontNotEmbedded, len(want), findings)
	}
	// The engine raised the rule on its own, which is what §6.5's coverage
	// check asks some test to have seen.
	fired[RuleFontNotEmbedded] = true
	for face, family := range want {
		var hit *Finding
		for i, f := range findings {
			if strings.Contains(f.Message, " "+face+",") {
				hit = &findings[i]
			}
		}
		if hit == nil {
			t.Errorf("no finding names %s: %v", face, findings)
			continue
		}
		if !strings.Contains(hit.Message, `"font-family: `+family+`"`) {
			t.Errorf("the finding for %s does not quote the family the document wrote, %q: %s",
				face, family, hit.Message)
		}
		if hit.Path == "" {
			t.Errorf("the finding for %s points at no element", face)
		}
		if strings.Contains(hit.Path, `#e`) {
			t.Errorf("the finding for %s points at the paragraph in the embedded face: %s", face, hit.Path)
		}
	}
	if findings[0].Severity != Error {
		t.Errorf("%s is reported at %v, want Error", RuleFontNotEmbedded, findings[0].Severity)
	}
}

// TestAPDFAPageInEmbeddedFacesIsNotRefused is the other half: a page whose every
// run is in an embedded face passes, and is the same page with the flag off.
func TestAPDFAPageInEmbeddedFacesIsNotRefused(t *testing.T) {
	in := Input{
		HTML:  `<ul style="font-family: Embedded"><li>One <em>two</em></li></ul>`,
		CSS:   []Stylesheet{{Source: `html { font-family: Embedded }`}},
		Fonts: pdfaFonts(t),
	}
	on := Compose(in, Options{PDFA: true})
	if f := embeddedFindings(on); len(f) != 0 || on.Refused {
		t.Errorf("a page set entirely in an embedded face was refused under PDF/A: %v", on.Findings)
	}
	off := Compose(in, Options{})
	if !reflect.DeepEqual(on.Ops, off.Ops) {
		t.Error("the flag changed the display list of a page it had nothing to report on")
	}
}

// TestWithoutPDFAAStandardFaceIsTheDefault: the mode is opt-in, and outside it
// a page in the standard faces is laid out and painted exactly as before, with
// nothing reported.
func TestWithoutPDFAAStandardFaceIsTheDefault(t *testing.T) {
	in := Input{HTML: `<p style="font-family: sans-serif">Sans</p>`}
	off := Compose(in, Options{})
	if f := embeddedFindings(off); len(f) != 0 || off.Refused {
		t.Errorf("a page in Helvetica was reported or refused without PDF/A: %v", off.Findings)
	}
	on := Compose(in, Options{PDFA: true})
	if !reflect.DeepEqual(on.Ops, off.Ops) {
		t.Error("the flag changed the display list; it may only add a finding")
	}
}

// TestAStandardFaceIsFoundWhereverItIsDrawn: the check reads the display list,
// so text inside each group the painter nests ops in — a TransformGroup, a
// FilterGroup, a ClipPath — is still seen. Each case is set in the embedded
// face except for the one box in a standard face, so a walk that skipped the
// group would report nothing; each was seen to fail with its group's case
// taken out of standardFacesDrawn.
func TestAStandardFaceIsFoundWhereverItIsDrawn(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"a rotated box", `<div style="transform: rotate(30deg); font-family: sans-serif">x</div>`},
		{"a blurred box", `<div style="filter: blur(1px); font-family: sans-serif">x</div>`},
		{"a box clipped to its rounded corners",
			`<div style="overflow: hidden; border-radius: 10px; font-family: sans-serif">x</div>`},
	} {
		got := Compose(Input{
			HTML:  `<div style="font-family: Embedded">` + tc.html + `</div>`,
			CSS:   []Stylesheet{{Source: `html { font-family: Embedded }`}},
			Fonts: pdfaFonts(t),
		}, Options{PDFA: true, TransformGroups: true})
		if len(embeddedFindings(got)) == 0 || !got.Refused {
			t.Errorf("%s: text in Helvetica was not reported under PDF/A: %v", tc.what, got.Findings)
		}
	}
}

// TestTheWalkEntersEveryGroupOfOps keeps standardFacesDrawn complete. It reads
// this package's source for every op that holds ops of its own — a struct with
// an "Ops []Op" field and an isOp method — and requires the walk to enter each
// one, so a group added to the painter later cannot hide text from PDF/A.
func TestTheWalkEntersEveryGroupOfOps(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	groups := map[string]bool{}
	ops := map[string]bool{}
	var walkCases map[string]bool
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				switch d := n.(type) {
				case *ast.TypeSpec:
					st, ok := d.Type.(*ast.StructType)
					if !ok {
						return true
					}
					for _, f := range st.Fields.List {
						if at, ok := f.Type.(*ast.ArrayType); ok && at.Len == nil {
							if id, ok := at.Elt.(*ast.Ident); ok && id.Name == "Op" {
								for _, name := range f.Names {
									if name.Name == "Ops" {
										groups[d.Name.Name] = true
									}
								}
							}
						}
					}
				case *ast.FuncDecl:
					if d.Name.Name == "isOp" && d.Recv != nil && len(d.Recv.List) == 1 {
						if id, ok := d.Recv.List[0].Type.(*ast.Ident); ok {
							ops[id.Name] = true
						}
					}
					if d.Name.Name == "standardFacesDrawn" {
						walkCases = map[string]bool{}
						ast.Inspect(d, func(n ast.Node) bool {
							if cc, ok := n.(*ast.CaseClause); ok {
								for _, e := range cc.List {
									if id, ok := e.(*ast.Ident); ok {
										walkCases[id.Name] = true
									}
								}
							}
							return true
						})
					}
				}
				return true
			})
		}
	}
	if walkCases == nil {
		t.Fatal("standardFacesDrawn was not found in the package source")
	}
	found := 0
	for name := range groups {
		if !ops[name] {
			continue
		}
		found++
		if !walkCases[name] {
			t.Errorf("%s holds ops of its own and standardFacesDrawn does not enter it: "+
				"text inside one would reach a PDF/A file unchecked", name)
		}
	}
	if found < 3 {
		t.Errorf("found %d groups of ops in the source, want at least the three the walk "+
			"was written for; the scan itself is broken", found)
	}
}
