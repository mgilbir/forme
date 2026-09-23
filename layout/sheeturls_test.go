package layout

import (
	"errors"
	"strings"
	"testing"
)

// Every reference in a stylesheet is relative to the stylesheet.
//
// CSS Values 4 §4.5.1 says so of every <url>, and the @import was the only one
// that did it: in "css/a.css", "@import \"base.css\"" found "css/base.css" and
// the font and the background on the next two lines asked for "f.ttf" and
// "bg.png" beside the document. A site laid out as "css/" beside "img/" — the
// commonest shape there is — had "../img/x.png" refused outright as leaving
// the directory (audit C34).

// servingResolver serves a fixed set of files and remembers every reference it
// was asked for, served or not.
type servingResolver struct {
	files map[string]string
	asked []string
}

func (r *servingResolver) Resolve(ref string) ([]byte, error) {
	r.asked = append(r.asked, ref)
	if s, ok := r.files[ref]; ok {
		return []byte(s), nil
	}
	return nil, errors.New("nothing here")
}

func (r *servingResolver) wasAsked(ref string) bool {
	for _, a := range r.asked {
		if a == ref {
			return true
		}
	}
	return false
}

// sheetWithEveryKindOfURL is a stylesheet naming one file through each thing
// in this engine that reads a url(), and one through a rule nested in an
// @media, which is in the sheet as much as a top-level rule is.
const sheetWithEveryKindOfURL = `@import "base.css";
@font-face { font-family: F; src: url(f.ttf) format("truetype") }
div { background-image: url('bg.png') }
li { list-style-image: url(../img/marker.png) }
p::before { content: url(sub/c.png) }
@media print { span { background-image: url("../img/print.png?v=../x") } }
`

func TestEveryURLInASheetIsRelativeToTheSheet(t *testing.T) {
	const doc = `<div>d</div><ul><li>i</li></ul><p style="font-family: F">p</p><span>s</span>`
	want := []string{
		"css/base.css",
		"css/f.ttf",
		"css/bg.png",
		"img/marker.png",
		"css/sub/c.png",
		// Only the path is cleaned: the query is the query, however it is
		// spelled.
		"img/print.png?v=../x",
	}
	for _, tc := range []struct {
		name string
		in   func(*servingResolver) Input
	}{
		{"a sheet the document links", func(r *servingResolver) Input {
			r.files["css/a.css"] = sheetWithEveryKindOfURL
			return Input{HTML: `<link rel=stylesheet href="css/a.css">` + doc, Resources: r}
		}},
		{"a sheet the caller names", func(r *servingResolver) Input {
			return Input{HTML: doc, Resources: r,
				CSS: []Stylesheet{{Name: "css/a.css", Source: sheetWithEveryKindOfURL}}}
		}},
		{"a sheet another sheet imports", func(r *servingResolver) Input {
			r.files["css/outer/o.css"] = `@import "../a.css";`
			r.files["css/a.css"] = sheetWithEveryKindOfURL
			return Input{HTML: `<link rel=stylesheet href="css/outer/o.css">` + doc, Resources: r}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res := &servingResolver{files: map[string]string{}}
			Build(tc.in(res))
			for _, ref := range want {
				if !res.wasAsked(ref) {
					t.Errorf("the resolver was never asked for %q; it was asked for %q", ref, res.asked)
				}
			}
			for _, wrong := range []string{"f.ttf", "bg.png", "../img/marker.png", "sub/c.png"} {
				if res.wasAsked(wrong) {
					t.Errorf("the resolver was asked for %q, relative to the document", wrong)
				}
			}
		})
	}
}

// TestAURLWithNoSheetIsRelativeToTheDocument is the control on the other side:
// a <style> element and a style attribute are in the document, so the document
// is what their references are relative to — and a url() that is only a
// fragment is a reference into the document whatever sheet it is in.
func TestAURLWithNoSheetIsRelativeToTheDocument(t *testing.T) {
	res := &servingResolver{files: map[string]string{}}
	Build(Input{Resources: res, HTML: `<style>div { background-image: url(a.png) }</style>` +
		`<div>d</div><p style="background-image: url(b.png)">p</p>`})
	for _, ref := range []string{"a.png", "b.png"} {
		if !res.wasAsked(ref) {
			t.Errorf("the resolver was never asked for %q; it was asked for %q", ref, res.asked)
		}
	}
}

// TestResolveAgainstSheet is the resolver on its own, against RFC 3986 §5.4's
// examples where they are about a path, with CSS Values 4's one exception: a
// reference that is only a fragment is into the document.
func TestResolveAgainstSheet(t *testing.T) {
	const base = "b/c/d;p?q"
	for ref, want := range map[string]string{
		"g":        "b/c/g",
		"./g":      "b/c/g",
		"g?y":      "b/c/g?y",
		"g#s":      "b/c/g#s",
		"g?y#s":    "b/c/g?y#s",
		";x":       "b/c/;x",
		"g;x":      "b/c/g;x",
		"?y":       "b/c/d;p?y",
		"":         "",
		".":        "b/c",
		"..":       "b",
		"../g":     "b/g",
		"../..":    ".",
		"../../g":  "g",
		"g/../h":   "b/c/h",
		"g?y/../x": "b/c/g?y/../x",
		// Beyond the base's own root the ".." is kept, for the resolver to
		// refuse: there is nowhere for a relative base to take it.
		"../../../g": "../g",
		// Each of these is already whatever it is going to be.
		"#s":                "#s",
		"/g":                "/g",
		`\g`:                `\g`,
		"//g":               "//g",
		"http://a/g":        "http://a/g",
		"data:,x":           "data:,x",
		"ht\ttp://a/g":      "http://a/g",
		" \n g \t":          "b/c/g",
		"//169.254.169.254": "//169.254.169.254",
	} {
		if got := resolveAgainstSheet(ref, base); got != want {
			t.Errorf("%q in %q resolved to %q, want %q", ref, base, got, want)
		}
	}
	// A sheet with no name, and a sheet that is a data: URL, leave a reference
	// relative to the document.
	for _, from := range []string{"", "data:text/css,p{}"} {
		if got := resolveAgainstSheet("g/h", from); got != "g/h" {
			t.Errorf("%q in %q resolved to %q, want it left relative to the document", "g/h", from, got)
		}
	}
}

// TestAResolvedReferenceIsChargedToTheWorkBudget: a sheet's name is the
// document's to choose — it is a <link>'s href — and resolving prefixes it to
// every url() in the sheet, so a long name times many references is work a
// small input multiplies. The resolved text is charged, and what the budget
// refuses is reported and not fetched from somewhere else.
func TestAResolvedReferenceIsChargedToTheWorkBudget(t *testing.T) {
	saved := workFloor
	workFloor = 1 << 16
	defer func() { workFloor = saved }()

	dir := strings.Repeat("d", 4096) + "/"
	var sheet strings.Builder
	for i := 0; i < 64; i++ {
		sheet.WriteString(".c { background-image: url(x.png) }\n")
	}
	res := &servingResolver{files: map[string]string{}}
	built := Build(Input{HTML: `<div class=c>x</div>`, Resources: res,
		CSS: []Stylesheet{{Name: dir + "a.css", Source: sheet.String()}}})
	requireFinding(t, built.Findings, RuleLimit, "the stylesheet references past that point")
	if res.wasAsked("x.png") {
		t.Error("a reference the budget refused was fetched relative to the document instead")
	}
}
