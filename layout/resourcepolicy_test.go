package layout

import (
	"errors"
	"strings"
	"testing"
)

// recordingResolver answers nothing and remembers every reference it was asked
// for, which is the whole of what these tests are about: what the engine hands
// a resolver, rather than what the resolver does with it.
type recordingResolver struct{ asked []string }

func (r *recordingResolver) Resolve(ref string) ([]byte, error) {
	r.asked = append(r.asked, ref)
	return nil, errors.New("nothing here")
}

// TestAResolverIsNeverHandedAReferenceOutsideTheDocument is the promise
// ResourceResolver makes, which was made by its documentation alone.
//
// "A resolver is handed the reference exactly as the document wrote it, with no
// scheme and no leading slash — those are refused before it is called." Only
// the scheme was. A recording resolver was handed "/etc/passwd",
// "../../x.css" and "../a.png" verbatim, and the same paragraph invites a
// caller to write os.ReadFile(filepath.Join(dir, ref)) — which turns those
// three into a file-disclosure primitive with a friendly interface. The
// documents this engine renders are untrusted, and "src" is a string in one of
// them.
func TestAResolverIsNeverHandedAReferenceOutsideTheDocument(t *testing.T) {
	for _, tc := range []struct{ name, html, css string }{
		{"an image above the directory", `<img src="../../x.png">`, ""},
		{"an image at an absolute path", `<img src="/etc/passwd">`, ""},
		{"an image with a backslash", `<img src="\windows\win.ini">`, ""},
		{"an image whose traversal is escaped", `<img src="%2e%2e/secret.png">`, ""},
		{"an image whose traversal is buried", `<img src="a/b/../../../secret.png">`, ""},
		{"a stylesheet above the directory", `<link rel=stylesheet href="../../x.css">`, ""},
		{"a stylesheet at an absolute path", `<link rel=stylesheet href="/etc/passwd">`, ""},
		{"a background above the directory", `<div id="d"></div>`,
			`#d { background-image: url(../../x.png); width: 10px; height: 10px }`},
		{"a font above the directory", `<p>x</p>`,
			`@font-face { font-family: T; src: url(../../secret.ttf) }`},
		{"a font at an absolute path", `<p>x</p>`,
			`@font-face { font-family: T; src: url(/root/.ssh/id_rsa) }`},
	} {
		res := &recordingResolver{}
		in := Input{HTML: tc.html, Resources: res}
		if tc.css != "" {
			in.CSS = []Stylesheet{{Source: tc.css}}
		}
		built := Build(in)
		if len(res.asked) != 0 {
			t.Errorf("%s: the resolver was handed %q", tc.name, res.asked)
		}
		if !hasRule(built.Findings, RuleResourceBlocked) {
			t.Errorf("%s: raised %v, want the blocked-resource finding",
				tc.name, ruleNames(built.Findings))
		}
	}
}

// TestAResolverIsStillHandedAnOrdinaryReference is the other half. A policy
// that refuses everything satisfies the test above and renders nothing.
func TestAResolverIsStillHandedAnOrdinaryReference(t *testing.T) {
	for _, tc := range []struct {
		name, html, css string
		want            string
	}{
		{"an image", `<img src="pictures/logo.png">`, "", "pictures/logo.png"},
		{"an image beside the document", `<img src="./logo.png">`, "", "./logo.png"},
		{"a reference with an escape", `<img src="blue%20sky.png">`, "", "blue%20sky.png"},
		{"a reference with a query", `<img src="logo.png?v=2">`, "", "logo.png?v=2"},
		{"a name that only looks like a traversal", `<img src="..secret/logo.png">`, "",
			"..secret/logo.png"},
		{"a stylesheet", `<link rel=stylesheet href="theme/main.css">`, "", "theme/main.css"},
		{"a font", `<p>x</p>`, `@font-face { font-family: T; src: url(fonts/t.ttf) }`,
			"fonts/t.ttf"},
	} {
		res := &recordingResolver{}
		in := Input{HTML: tc.html, Resources: res}
		if tc.css != "" {
			in.CSS = []Stylesheet{{Source: tc.css}}
		}
		Build(in)
		var found bool
		for _, ref := range res.asked {
			if ref == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the resolver was handed %q, want %q among them",
				tc.name, res.asked, tc.want)
		}
	}
}

// TestTheReferenceHandedOverIsTheOneTheDocumentWrote pins the other half of the
// contract: the check is made on the decoded form and the resolver still gets
// what the document wrote, because DirResolver decodes for itself and decoding
// twice reads a different file.
func TestTheReferenceHandedOverIsTheOneTheDocumentWrote(t *testing.T) {
	res := &recordingResolver{}
	Build(Input{HTML: `<img src="blue%2520sky.png">`, Resources: res})
	if len(res.asked) == 0 {
		t.Fatal("the resolver was handed nothing")
	}
	if res.asked[0] != "blue%2520sky.png" {
		t.Errorf("the resolver was handed %q, want the reference the document wrote; "+
			"decoding it here as well makes it a different file", res.asked[0])
	}
}

// TestCheckingAReferenceAgreesWithResolvingIt keeps the two from drifting: the
// engine's gate and DirResolver's own check are the same policy, and a
// reference either of them refuses must be refused by both.
func TestCheckingAReferenceAgreesWithResolvingIt(t *testing.T) {
	for _, ref := range []string{
		"a.png", "./a.png", "sub/a.png", "blue%20sky.png", "a.png?v=1", "a.png#x",
		"../a.png", "/a.png", `\a.png`, "%2e%2e/a.png", "sub/../../a.png",
		"http://example.com/a.png", "file:///etc/hostname", "c:/x.png", "", "  ",
	} {
		_, pathErr := resourcePath(ref)
		checkErr := checkResourceRef(ref)
		if (pathErr == nil) != (checkErr == nil) {
			t.Errorf("%q: resourcePath says %v and the gate says %v", ref, pathErr, checkErr)
			continue
		}
		if pathErr != nil && !strings.Contains(checkErr.Error(), pathErr.Error()) {
			t.Errorf("%q: the gate reports %q and resourcePath %q", ref, checkErr, pathErr)
		}
	}
}
