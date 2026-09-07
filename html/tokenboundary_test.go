package html

import "testing"

// Two rules that are stated over the token stream and were applied to the
// characters.

// TestOnlyTheNewlineRightAfterThePreStartTagIsDropped.
//
// §13.2.6.4.7: "if the next token is a U+000A LINE FEED character token, then
// ignore that token". A comment is a token, so a comment written between the
// start tag and the newline makes the newline not the next one — and the
// newline is the author's.
//
// The tokenizer consumed a comment in silence and produced nothing for it, so
// nothing counted it and the flag survived to the text after it. It is a token
// now, carrying nothing, for that reason alone.
func TestOnlyTheNewlineRightAfterThePreStartTagIsDropped(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"<pre>\nx</pre>", "x"},
		{"<pre><!-- c -->\nx</pre>", "\nx"},
		{"<textarea>\nx</textarea>", "x"},
		// <textarea> holds raw text, so a comment inside it is not a comment
		// at all — it is the first characters of the one text token, which does
		// not begin with a newline and so has nothing stripped.
		{"<textarea><!-- c -->\nx</textarea>", "<!-- c -->\nx"},
		// A comment after the newline changes nothing: the newline was still
		// the next token.
		{"<pre>\n<!-- c -->x</pre>", "x"},
		// And the rule is one newline, whatever comes after it.
		{"<pre>\n\nx</pre>", "\nx"},
	} {
		doc, _, _ := Parse(tc.src)
		if got := textOf(doc); got != tc.want {
			t.Errorf("%q gave %q, want %q", tc.src, got, tc.want)
		}
	}
}

// TestACommentDoesNotDivideTheTextAroundIt is the other half of making a comment
// a token: nothing in this model records one, so the runs either side of it are
// still one run.
func TestACommentDoesNotDivideTheTextAroundIt(t *testing.T) {
	doc, _, ok := Parse("<p>a<!-- c -->b</p>")
	if !ok {
		t.Error("a document with a comment in it was refused")
	}
	n := 0
	doc.Walk(func(c *Node) bool {
		if c.Type == TextNode {
			n++
		}
		return true
	})
	if n != 1 {
		t.Errorf("%d text nodes, want one: adjacent runs are merged", n)
	}
	if got := textOf(doc); got != "ab" {
		t.Errorf("the text is %q, want \"ab\"", got)
	}
}

// TestLeadingZerosAreNotLength. A numeric character reference too long to be a
// code point is refused before it is parsed, so that a million digits cost
// nothing — but a zero is not a digit of the number. "&#000000065;" is the
// letter A, and it was refused as "far outside Unicode" while the same
// character written in hexadecimal with one zero fewer was not.
func TestLeadingZerosAreNotLength(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"&#65;", "A"},
		{"&#000000065;", "A"},
		{"&#00000000000000000000065;", "A"},
		{"&#x41;", "A"},
		{"&#x00000041;", "A"},
	} {
		doc, errs, ok := Parse("<p>" + tc.src + "</p>")
		if got := textOf(doc); got != tc.want {
			t.Errorf("%q gave %q, want %q (%v)", tc.src, got, tc.want, errs)
		}
		if !ok {
			t.Errorf("%q was refused: %v", tc.src, errs)
		}
	}
	// And a number that really is too large is still refused before it is
	// parsed, zeros or no zeros — as is one whose digits are all zeros, which
	// names U+0000 and is refused for being that rather than for its length.
	for _, src := range []string{"&#0000000123456789012345;", "&#0000000000000000000000000;"} {
		if _, _, ok := Parse("<p>" + src + "</p>"); ok {
			t.Errorf("%q was accepted", src)
		}
	}
}
