package style

import (
	"testing"

	"github.com/mgilbir/forme/html"
)

// TestAClassIsSplitOnHTMLsWhiteSpaceAndNotUnicodes.
//
// HTML says the class attribute is a set of tokens split on *ASCII* white space
// — tab, line feed, form feed, carriage return and space. strings.Fields splits
// on unicode.IsSpace, which takes the no-break space and every other space
// separator with it, so `class="a<NBSP>b"` was read as two classes where the
// document has one, and ".a" selected an element the author never put in that
// class. The same set decides "~=", which HTML defines the same way.
func TestAClassIsSplitOnHTMLsWhiteSpaceAndNotUnicodes(t *testing.T) {
	for _, tc := range []struct {
		class string
		want  string
		match bool
		what  string
	}{
		{"a b", "a", true, "an ordinary space"},
		{"a\tb", "b", true, "a tab"},
		{"a\nb", "a", true, "a line feed"},
		{"a\fb", "b", true, "a form feed"},
		{"a\rb", "a", true, "a carriage return"},

		{"a b", "a", false, "a no-break space, which is not a separator"},
		{"a b", "a b", true, "and the whole of it is the class name"},
		{"a b", "a", false, "an em space"},
		{"a　b", "a", false, "an ideographic space"},
	} {
		n := &html.Node{Type: html.ElementNode, Name: "p",
			Attrs: []html.Attribute{{Name: "class", Value: tc.class}}}
		if got := hasClass(n, tc.want); got != tc.match {
			t.Errorf("%s: class=%q has class %q = %v, want %v",
				tc.what, tc.class, tc.want, got, tc.match)
		}
	}
}
