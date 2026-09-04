package paragraph

import "testing"

// white-space-collapse: discard, CSS Text §4.1.
//
// "This value directs user agents to discard all white space in the element."
// It is the far end of the scale the other values are on — preserve keeps every
// space, collapse keeps one, this keeps none — and it is not a combination of
// them, because collapsing a run of spaces between two words leaves one space
// and separates the words.
//
// It was read as "collapse" and reported as nothing, so a document asking for it
// got a sentence with the spaces still in it and no word about why. The suite's
// white-space-collapse-discard-001 writes the sentence and asks for it to read
// as one word.
func TestDiscardRemovesEveryWhiteSpaceCharacter(t *testing.T) {
	ws := WhiteSpaceOf("discard")
	if !ws.Discard {
		t.Fatal("\"discard\" was not read as itself")
	}
	if !ws.Collapse {
		t.Error("\"discard\" does not collapse; every rule that asks whether a " +
			"space may be removed at a line edge has to answer yes")
	}
	for _, tc := range []struct{ text, want, what string }{
		{"All White Space.", "AllWhiteSpace.", "the suite's own sentence"},
		{"a  b", "ab", "a run of them"},
		{"a\tb", "ab", "a tab"},
		{"a\nb", "ab", "a segment break"},
		{"a\r\nb", "ab", "a carriage return and a line feed"},
		{" a ", "a", "at both ends"},
		{"ab", "ab", "text with none in it at all"},
		// The other space separators are not white space to §4.1, and Phase I
		// never sees them: an ideographic space between two words is a
		// character of the text.
		{"a　b", "a　b", "an ideographic space"},
		{"a b", "a b", "a no-break space"},
		{"a​b", "a​b", "a zero width space, which is a mark"},
	} {
		if got := CollapseWhitespace(tc.text, "discard", WordSpaceTransform{}); got != tc.want {
			t.Errorf("%s: %q became %q, want %q", tc.what, tc.text, got, tc.want)
		}
	}
	// And the separators the document asked to be made visible survive it: they
	// are not white space, and the property puts the space in rather than the
	// source having one.
	space, _ := WordSpaceTransformOf("space")
	if got := CollapseWhitespace("a​b c", "discard", space); got != "a bc" {
		t.Errorf("with word-space-transform: space the text became %q, want %q "+
			"— the space the source wrote goes and the one the property asked "+
			"for stays", got, "a bc")
	}
	// The value is not the others: the same text under each of them is what
	// says this one is doing something of its own.
	for _, other := range []string{"collapse", "preserve", "preserve-breaks", "break-spaces"} {
		if got := CollapseWhitespace("a  b", other, WordSpaceTransform{}); got == "ab" {
			t.Errorf("%q also removed the spaces, so the rows above say nothing "+
				"about discard", other)
		}
	}
}
