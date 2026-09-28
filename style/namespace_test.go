package style

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/html"
)

// @namespace, CSS Namespaces 3: a sheet declares a default namespace and
// prefixes, and its selectors are read against them. It was reported as an
// at-rule "not applied yet", and a prefixed selector was refused, so the one
// way a stylesheet can tell an HTML element from a MathML one of the same name
// was not there to use.

const namespaceDoc = `<mi id="hm" class="c">h</mi>` +
	`<math><mi id="mm" class="c">m</mi><mrow id="mr">r</mrow></math>`

func colorsOf(t *testing.T, sheets ...Sheet) (map[string]string, []Finding) {
	t.Helper()
	doc, _, _ := html.Parse(namespaceDoc)
	got := Apply(doc, sheets)
	out := map[string]string{}
	doc.Walk(func(n *html.Node) bool {
		if id, ok := n.Attr("id"); ok {
			out[id] = got.Styles[n].Get("color")
		}
		return true
	})
	return out, got.Findings
}

func TestNamespacePrefixesSelectByNamespace(t *testing.T) {
	for _, tc := range []struct {
		what, css      string
		hm, mm, mr     string
		reportContains string
	}{
		{"a declared prefix",
			`@namespace m url(http://www.w3.org/1998/Math/MathML); m|mi { color: red }`,
			"black", "red", "black", ""},
		{"a prefix declared with a quoted url()",
			`@namespace m url("http://www.w3.org/1998/Math/MathML"); m|mrow { color: red }`,
			"black", "black", "red", ""},
		{"a prefix declared with a string",
			`@namespace m "http://www.w3.org/1998/Math/MathML"; m|* { color: red }`,
			"black", "red", "red", ""},
		{"the default namespace, for names and for compounds with none",
			`@namespace url(http://www.w3.org/1999/xhtml); mi { color: red } .c { color: blue }`,
			"blue", "black", "black", ""},
		{"any namespace", `*|mi { color: red }`, "red", "red", "black", ""},
		{"no namespace, which nothing is in", `|mi { color: red }`, "black", "black", "black", ""},
		{"a prefix nobody declared invalidates the rule",
			`m|mi { color: red } mi { color: green }`, "green", "green", "black",
			`the namespace prefix "m" is not declared`},
		{"an @namespace after a rule is ignored",
			`mrow { color: blue } @namespace m url(http://www.w3.org/1998/Math/MathML); m|mi { color: red }`,
			"black", "black", "blue", "must be written before every other rule"},
		{"an @namespace inside a conditional is ignored",
			`@media print { @namespace m url(http://www.w3.org/1998/Math/MathML); } m|mi { color: red }`,
			"black", "black", "black", "must be written before every other rule"},
		{"a malformed @namespace is ignored",
			`@namespace m n o; m|mi { color: red }`,
			"black", "black", "black", "is not a prefix and a namespace name"},
		{"an @layer statement may come before an @namespace",
			`@layer a, b; @namespace m url(http://www.w3.org/1998/Math/MathML); m|mi { color: red }`,
			"black", "red", "black", ""},
		{"the later declaration of a prefix wins",
			`@namespace m url(urn:x); @namespace m url(http://www.w3.org/1998/Math/MathML); m|mi { color: red }`,
			"black", "red", "black", ""},
		// A MathML element's name is matched as written; an HTML element's
		// in an HTML document is not.
		{"type selectors are case-sensitive for MathML", `MI { color: red }`, "red", "black", "black", ""},
	} {
		got, findings := colorsOf(t, author(t, tc.css))
		if got["hm"] != tc.hm || got["mm"] != tc.mm || got["mr"] != tc.mr {
			t.Errorf("%s: html mi %s, mathml mi %s, mrow %s; want %s, %s, %s", tc.what,
				got["hm"], got["mm"], got["mr"], tc.hm, tc.mm, tc.mr)
		}
		var msgs []string
		for _, f := range findings {
			msgs = append(msgs, f.Message)
			if f.Property == "@namespace" && f.Unsupported {
				t.Errorf("%s: %q is reported as unsupported", tc.what, f.Message)
			}
		}
		if tc.reportContains != "" && !strings.Contains(strings.Join(msgs, "\n"), tc.reportContains) {
			t.Errorf("%s: findings %q do not say %q", tc.what, msgs, tc.reportContains)
		}
	}
}

// TestNamespacesAreScopedToTheirSheet: a prefix is the declaring sheet's.
func TestNamespacesAreScopedToTheirSheet(t *testing.T) {
	got, _ := colorsOf(t,
		author(t, `@namespace m url(http://www.w3.org/1998/Math/MathML); m|mi { color: red }`),
		author(t, `m|mrow { color: blue }`))
	if got["mm"] != "red" || got["mr"] != "black" {
		t.Errorf("mathml mi %s, mrow %s; want red, and black: the second sheet declared no m", got["mm"], got["mr"])
	}
}
