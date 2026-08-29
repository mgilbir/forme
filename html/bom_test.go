package html

import (
	"strings"
	"testing"
)

// The byte order mark, and why a document that starts with one is not a
// document that starts with a character.
//
// A Windows editor writes EF BB BF at the front of a UTF-8 file, and 111 of the
// working group's own documents have one. HTML's encoding sniffing reads those
// three bytes as the statement "this file is UTF-8" and removes them, and its
// input preprocessing says it again for a document that arrived already decoded:
// "one leading U+FEFF byte order mark character must be ignored".
//
// Left in, it is a character in an otherwise empty inline formatting context in
// front of the document's first element — so the page gets a line of text nobody
// wrote at the top and everything below it moves down by a line.
// position-relative-nested-001 is that: a test with a mark against a reference
// without one, and 25.86px between them.

func TestALeadingByteOrderMarkIsNotContent(t *testing.T) {
	for _, src := range []string{
		"\ufeff<p>x</p>",
		"\ufeff<!DOCTYPE html><p>x</p>",
		"\ufeff<html><body><p>x</p></body></html>",
	} {
		doc, _, _ := Parse(src)
		if got := doc.TextContent(); strings.ContainsRune(got, '\ufeff') {
			t.Errorf("%q kept the mark: the document's text is %q", src, got)
		}
		if got := doc.TextContent(); got != "x" {
			t.Errorf("%q produced the text %q, want \"x\"", src, got)
		}
	}
}

// TestAMarkAnywhereElseIsACharacter. U+FEFF away from the front is ZERO WIDTH
// NO-BREAK SPACE: a character of the text that sets no paper and holds two words
// together, and dropping one is dropping content.
func TestAMarkAnywhereElseIsACharacter(t *testing.T) {
	doc := mustParseHTML(t, "<p>a\ufeffb</p>")
	if got := doc.TextContent(); got != "a\ufeffb" {
		t.Errorf("the text is %q, want \"a\\ufeffb\" — the mark is inside a word", got)
	}
	// Not even one that follows the leading one: "one leading" is one.
	doc, _, _ = Parse("\ufeff\ufeff<p>x</p>")
	if got := doc.TextContent(); got != "\ufeffx" {
		t.Errorf("two marks left %q, want the second one kept", got)
	}
	// And not one that merely appears early: a mark after the doctype is text.
	doc, _, _ = Parse("<!DOCTYPE html>\ufeff<p>x</p>")
	if got := doc.TextContent(); !strings.ContainsRune(got, '\ufeff') {
		t.Errorf("a mark after the doctype was dropped; the text is %q", got)
	}
}

// TestTheMarkDoesNotMoveTheOffsets. It is ignored where the reading starts
// rather than by cutting the string, so that every offset in a finding is still
// an offset into the bytes the author has in front of them.
func TestTheMarkDoesNotMoveTheOffsets(t *testing.T) {
	const src = "\ufeff<p>a</br>b</p>"
	_, errs, _ := Parse(src)
	if len(errs) != 1 {
		t.Fatalf("%d findings, want the one about </br>: %v", len(errs), errs)
	}
	want := strings.Index(src, "</br>")
	if errs[0].Offset != want {
		t.Errorf("the finding is at byte %d, want %d — the mark is three bytes and "+
			"the offsets are into the document as it was written", errs[0].Offset, want)
	}
}
