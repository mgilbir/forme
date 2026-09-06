package layout

import (
	"errors"
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

// askedFor collects what a document's references make the engine ask a resolver
// for, across all three loaders.
func askedFor(html, css string) []string {
	res := &recordingResolver{}
	in := Input{HTML: html, Resources: res}
	if css != "" {
		in.CSS = []Stylesheet{{Source: css}}
	}
	Build(in)
	return res.asked
}

// TestNoSchemeEverReachesAResolver is the one refusal the engine makes on a
// resolver's behalf, and the reason it is the only one.
//
// An HTML-to-PDF engine that fetches URLs is a server-side request forgery
// primitive with a friendly interface: the attacker writes <img
// src="http://169.254.169.254/...">, the server fetches it, and the only
// question left is whether the response is visible in the PDF. Refusing the
// scheme here rather than leaving it to the resolver is what makes it
// impossible for a caller to build one by accident.
func TestNoSchemeEverReachesAResolver(t *testing.T) {
	for _, tc := range []struct{ name, html, css string }{
		{"an image over http", `<img src="http://169.254.169.254/latest/meta-data">`, ""},
		{"an image over https", `<img src="https://example.com/x.png">`, ""},
		{"an image as a file URL", `<img src="file:///etc/hostname">`, ""},
		{"an image at a drive letter", `<img src="c:/windows/win.ini">`, ""},
		{"a stylesheet over https", `<link rel=stylesheet href="https://example.com/x.css">`, ""},
		{"a background over http", `<div id="d"></div>`,
			`#d { background-image: url(http://example.com/x.png); width: 10px; height: 10px }`},
		{"a font over https", `<p>x</p>`,
			`@font-face { font-family: T; src: url(https://example.com/t.ttf) }`},
	} {
		if asked := askedFor(tc.html, tc.css); len(asked) != 0 {
			t.Errorf("%s: the resolver was handed %q", tc.name, asked)
		}
		built := Build(Input{HTML: tc.html, Resources: &recordingResolver{},
			CSS: []Stylesheet{{Source: tc.css}}})
		if !hasRule(built.Findings, RuleResourceBlocked) {
			t.Errorf("%s: raised %v, want the blocked-resource finding",
				tc.name, ruleNames(built.Findings))
		}
	}
}

// TestAResolverIsHandedTheReferenceAsWritten is the other half of the contract,
// and it is stated as a test because it used to be stated wrongly in prose.
//
// ResourceResolver said a resolver is handed the reference "with no scheme and
// no leading slash — those are refused before it is called". Only the scheme
// was, and the same paragraph invited a caller to write
// os.ReadFile(filepath.Join(dir, ref)), which turns "/etc/passwd" and
// "../../x" into a file-disclosure primitive.
//
// Refusing those in the engine is not the fix, because neither is refusable
// without refusing an ordinary document: "/css/x.png" is relative to wherever
// the document is served from and "../images/logo.png" to a sibling directory,
// and only the caller knows where either is. A browser resolves both, this
// engine's own reftest harness resolves both, and the documentation now says so
// — so this pins what really arrives, and it fails if the prose and the code
// ever drift apart again.
func TestAResolverIsHandedTheReferenceAsWritten(t *testing.T) {
	for _, tc := range []struct{ name, html, css, want string }{
		{"a plain name", `<img src="logo.png">`, "", "logo.png"},
		{"a path below", `<img src="pictures/logo.png">`, "", "pictures/logo.png"},
		{"a path beside", `<img src="./logo.png">`, "", "./logo.png"},
		{"a path above", `<img src="../images/logo.png">`, "", "../images/logo.png"},
		{"a path from the root", `<img src="/css/support/logo.png">`, "",
			"/css/support/logo.png"},
		{"an escape", `<img src="blue%20sky.png">`, "", "blue%20sky.png"},
		{"a doubled escape", `<img src="blue%2520sky.png">`, "", "blue%2520sky.png"},
		{"a query", `<img src="logo.png?v=2">`, "", "logo.png?v=2"},
		{"a stylesheet above", `<link rel=stylesheet href="../theme/main.css">`, "",
			"../theme/main.css"},
		{"a stylesheet from the root", `<link rel=stylesheet href="/theme/main.css">`, "",
			"/theme/main.css"},
		{"a font above", `<p>x</p>`, `@font-face { font-family: T; src: url(../fonts/t.ttf) }`,
			"../fonts/t.ttf"},
		{"a background from the root", `<div id="d"></div>`,
			`#d { background-image: url(/img/x.png); width: 10px; height: 10px }`, "/img/x.png"},
	} {
		var found bool
		asked := askedFor(tc.html, tc.css)
		for _, ref := range asked {
			if ref == tc.want {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: the resolver was handed %q, want %q among them",
				tc.name, asked, tc.want)
		}
	}
}

// TestDirResolverContainsWhatTheEngineDoesNot is the sentence the paragraph now
// ends on: containment is the resolver's job, and this is the resolver that
// does it. The references above that the engine passes through are all refused
// here.
func TestDirResolverContainsWhatTheEngineDoesNot(t *testing.T) {
	_, res := planted(t)
	for _, ref := range []string{
		"../secret.txt", "/etc/hostname", `..\secret.txt`, "sub/../../secret.txt",
		"%2e%2e/secret.txt",
	} {
		if got, err := res.Resolve(ref); err == nil {
			t.Errorf("%q was read and returned %q; the engine passes it through, so "+
				"this is the only thing standing between it and the file", ref, got)
		}
	}
}
