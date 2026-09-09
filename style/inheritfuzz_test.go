package style

import (
	"testing"

	"github.com/mgilbir/forme/html"
)

// What a document with no stylesheet computes to.
//
// The cascade's registry says of every property whether it inherits, and that
// one bit decides what a document looks like: a colour set on <body> reaches
// every word under it, and a margin set there reaches nothing. Nothing asserts
// the bit is acted on. The tests next door check particular properties on
// particular documents; what is missing is the rule.
//
// It is exactly checkable with no stylesheet at all, which is why this is a
// target of its own rather than another assertion inside FuzzApply. Given no
// rules, the only thing that can declare a value on an element is its own style
// attribute — so for every other element the answer is settled: an inherited
// property is the parent's computed value, and one that does not inherit is its
// initial value. Both halves matter and they fail differently. A property that
// inherits and should not carries a margin down a tree; one that does not and
// should leaves a paragraph black inside a white-on-black page.
//
// The document is the untrusted half here and the stylesheet is absent, which is
// the opposite of FuzzApply's shape and finds different things: what is being
// exercised is the walk that carries values down a tree the parser built out of
// whatever it was given.
//
// # What this does not say
//
// That the registry's bit is *right* for any given property. It reads the same
// table the cascade reads, so flipping an entry moves the assertion with the
// engine and a planted defect doing exactly that passes — which is how the limit
// was established rather than assumed. What each property inherits is pinned by
// the fixture tests next door, one property at a time, against the
// specification. What is asserted here is that the cascade *acts* on the bit,
// over every tree the fuzzer can build: a planted defect taking the initial value
// for everything fails, and so does one inheriting everything.
func FuzzInheritance(f *testing.F) {
	for _, src := range []string{
		"<p>x</p>",
		"<div><p><span>x</span></p></div>",
		`<div style="color: red"><p>x</p></div>`,
		`<p style="margin: 4px">x</p>`,
		// A property that does *not* inherit, declared on a parent and asked
		// about on the child. Without one of these the whole tree is at its
		// initial values and inheriting a property that should not is
		// indistinguishable from not inheriting it — which a planted defect
		// showed before these were here.
		`<div style="margin-top: 4px"><p>x</p></div>`,
		`<div style="border-top-width: 3px"><p><span>x</span></p></div>`,
		`<div style="color: red; margin-left: 9px"><p>x</p></div>`,
		"<ul><li>a<li>b</ul>",
		"<table><tr><td>a<td>b</table>",
		"<b><i>x</b></i>",
		"<div>x",
		"",
		"<p><br><wbr><img></p>",
		"<svg><g><text>x</text></g></svg>",
	} {
		f.Add(src)
	}
	f.Fuzz(func(t *testing.T, src string) {
		checkInheritance(t, src)
	})
}

// checkInheritance is the body, as a function so a test can drive it.
func checkInheritance(t *testing.T, src string) {
	if len(src) > 1<<16 {
		return
	}
	doc, _, _ := html.Parse(src)
	if doc == nil {
		return
	}
	got := Apply(doc, nil)

	parent := map[*html.Node]*html.Node{}
	var link func(*html.Node)
	link = func(n *html.Node) {
		for _, c := range n.Children {
			parent[c] = n
			link(c)
		}
	}
	link(doc)

	doc.Walk(func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if _, declared := n.Attr("style"); declared {
			// The one thing that can set a value here. Its own properties are
			// whatever it said, and there is nothing to check them against.
			return true
		}
		cs := got.Styles[n]
		if cs == nil {
			return true
		}
		// The parent's computed style, or none where the element is the root.
		// An element with no element above it inherits from nothing, which is
		// the initial value — the same answer the other branch gives.
		var ps ComputedStyle
		if p := parent[n]; p != nil && p.Type == html.ElementNode {
			ps = got.Styles[p]
		}
		for name, prop := range properties {
			if name == "font-size" {
				// The one property the cascade resolves to a length rather than
				// carrying the value the author wrote. "medium" is its initial
				// value and 16px is what a computed style holds, because every
				// other length in the element is absolutised against it and only
				// the cascade can do that — see fontSizeOf. So the registry's
				// initial value is not the answer here and there is nothing to
				// compare against that is not this file reimplementing that
				// resolution.
				continue
			}
			want := prop.initial
			if prop.inherits && ps != nil {
				want = ps[name]
			}
			if cs[name] != want {
				kind := "does not inherit, so it is its initial value"
				if prop.inherits {
					kind = "inherits, so it is the parent's computed value"
				}
				t.Fatalf("%q: <%s> has %s %q; the property %s, which is %q",
					src, n.Name, name, cs[name], kind, want)
			}
		}
		return true
	})
}
