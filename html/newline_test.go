package html

import "testing"

// HTML's input preprocessing: "any LF character that immediately follows a CR
// character must be ignored, and all CR characters must then be converted to LF
// characters".
//
// It is a rule about the input *stream* — the bytes the author wrote — and a
// character reference is resolved after it. So a document written on Windows
// has line feeds in its tree and "&#x0D;" puts a real U+000D there, which is
// the distinction the whole of the suite's control-chars-00D turns on: CSS Text
// makes that surviving return a space and not a segment break.
func TestASourceNewlineIsFoldedByTheParser(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"crlf", "<pre>a\r\nb</pre>", "a\nb"},
		{"lone cr", "<pre>a\rb</pre>", "a\nb"},
		{"cr cr", "<pre>a\r\rb</pre>", "a\n\nb"},
		{"lf cr", "<pre>a\n\rb</pre>", "a\n\nb"},
		{"crlf crlf", "<pre>a\r\n\r\nb</pre>", "a\n\nb"},
		// The leading newline a <pre> drops is the folded one, so a file
		// written on Windows loses its first line like everyone else's.
		{"leading crlf", "<pre>\r\nhello</pre>", "hello"},
	} {
		doc := mustParseHTML(t, tc.src)
		if got := doc.Element("pre").TextContent(); got != tc.want {
			t.Errorf("%s: %q parsed to %q, want %q", tc.name, tc.src, got, tc.want)
		}
	}
}

// TestACharacterReferenceToAReturnSurvives is the other side of it, and it is
// the side the suite tests: the preprocessing has already run by the time a
// reference is resolved, so what it produces is not folded.
func TestACharacterReferenceToAReturnSurvives(t *testing.T) {
	for _, src := range []string{
		"<pre>a&#x0D;b</pre>",
		"<pre>a&#13;b</pre>",
	} {
		doc := mustParseHTML(t, src)
		if got := doc.Element("pre").TextContent(); got != "a\rb" {
			t.Errorf("%q parsed to %q, want %q", src, got, "a\rb")
		}
	}
}

// TestAnAttributeValueIsPreprocessedToo, because the rule is about the input
// stream and an attribute value is part of it.
func TestAnAttributeValueIsPreprocessedToo(t *testing.T) {
	doc := mustParseHTML(t, "<p title=\"a\r\nb\">x</p>")
	got, _ := doc.Element("p").Attr("title")
	if got != "a\nb" {
		t.Errorf("the title is %q, want %q", got, "a\nb")
	}
}
