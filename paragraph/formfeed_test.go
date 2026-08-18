package paragraph

import "testing"

// A form feed is not white space.
//
// CSS Text 3 §4.1.1 names three things "white space" for the purpose of
// collapsing: spaces (U+0020), tabs (U+0009), and segment breaks. A form feed is
// none of them, and §3's rule that a control character "must be rendered as a
// visible glyph" names only tab, line feed and carriage return out of itself.
//
// It was collapsed here on CSS 2.1's reading, which counts a form feed among the
// white space a document may be written with. That reading is superseded, and
// following it did more than leave a character unrendered: a form feed *removed*
// itself and its neighbours' spacing from the line, so a document with one in it
// came out with the words either side of it joined.
func TestAFormFeedIsNotCollapsibleWhiteSpace(t *testing.T) {
	for _, c := range []byte{' ', '\t', '\n', '\r'} {
		if !isCollapsibleSpace(c) {
			t.Errorf("U+%04X is not collapsible white space", c)
		}
	}
	if isCollapsibleSpace('\f') {
		t.Errorf("a form feed is collapsible white space; CSS Text 3's set is " +
			"spaces, tabs and segment breaks, and a form feed is none of them")
	}
	// A no-break space was never in the set and must stay out of it: not being
	// white space for this purpose is the whole reason an author writes one.
	if isCollapsibleSpace(0xA0) {
		t.Errorf("a no-break space is collapsible white space")
	}
}
