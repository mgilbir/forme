package html

import (
	"strings"
	"testing"
)

// What a document says it is encoded in, and what it actually is.
//
// Neither was checked. A page in Shift-JIS or windows-1252 was read as though
// its bytes were UTF-8, came out as replacement characters, and reported
// success — every byte consumed, every element in its place, ok == true. The
// mojibake is not the failure; the silence is.

// errorsOf parses a document and returns its errors and whether it succeeded.
func errorsOf(t *testing.T, src string) ([]Error, bool) {
	t.Helper()
	_, errs, ok := Parse(src)
	return errs, ok
}

func hasMessage(errs []Error, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e.Message, substr) {
			return true
		}
	}
	return false
}

// TestBytesThatAreNotUTF8AreReported.
func TestBytesThatAreNotUTF8AreReported(t *testing.T) {
	// "café" in windows-1252: the é is one byte, 0xE9, which begins no UTF-8
	// character. This is what a page written in a European editor looks like.
	src := "<p>caf\xe9</p>"
	errs, ok := errorsOf(t, src)
	if ok {
		t.Error("a document with a byte that is not UTF-8 parsed with no errors")
	}
	if !hasMessage(errs, "begins no UTF-8 character") {
		t.Errorf("nothing was reported about the byte; errors: %v", errs)
	}
	for _, e := range errs {
		if strings.Contains(e.Message, "begins no UTF-8") && e.Offset != 6 {
			t.Errorf("the byte was reported at offset %d, want 6", e.Offset)
		}
	}
}

// TestTheCountOfBadBytesIsReported, because "how much of this is not text" is
// the question an author asks next — and because one finding per byte would
// bury the document's real problems under four thousand copies of one.
func TestTheCountOfBadBytesIsReported(t *testing.T) {
	src := "<p>\xe9\xe8\xea</p>"
	errs, _ := errorsOf(t, src)
	if !hasMessage(errs, "3 such bytes") {
		t.Errorf("the count was not reported; errors: %v", errs)
	}
	n := 0
	for _, e := range errs {
		if strings.Contains(e.Message, "UTF-8") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d findings for three bad bytes, want one", n)
	}
}

// TestValidUTF8IsPassedOverInSilence is the control, and it is the one that
// matters most: almost every document is UTF-8 and must gain no finding.
func TestValidUTF8IsPassedOverInSilence(t *testing.T) {
	for _, src := range []string{
		"<p>plain ASCII</p>",
		"<p>café — naïve</p>",
		"<p>日本語のテキスト</p>",
		"<p>\U0001F600</p>",
		"\ufeff<p>with a byte order mark</p>",
		"<p>שלום</p>",
		"",
	} {
		errs, ok := errorsOf(t, src)
		if !ok || len(errs) != 0 {
			t.Errorf("%q was reported: %v", src, errs)
		}
	}
}

// TestADeclaredEncodingThisEngineCannotReadIsReported.
func TestADeclaredEncodingThisEngineCannotReadIsReported(t *testing.T) {
	for _, tc := range []struct{ src, label, what string }{
		{`<meta charset="shift_jis">`, "shift_jis", "the short spelling"},
		{`<meta charset=windows-1252>`, "windows-1252", "unquoted"},
		{`<META CHARSET="ISO-8859-1">`, "iso-8859-1", "in capitals"},
		{`<meta http-equiv="content-type" content="text/html; charset=gb18030">`,
			"gb18030", "the http-equiv spelling"},
		{`<meta http-equiv=Content-Type content='text/html;charset=euc-kr'>`,
			"euc-kr", "http-equiv with single quotes and no space"},
	} {
		errs, ok := errorsOf(t, "<html><head>"+tc.src+"</head><body>x</body></html>")
		if ok {
			t.Errorf("%s: a document declaring %s parsed with no errors", tc.what, tc.label)
		}
		if !hasMessage(errs, `"`+tc.label+`"`) {
			t.Errorf("%s: nothing named %s; errors: %v", tc.what, tc.label, errs)
		}
		var unsupported bool
		for _, e := range errs {
			if strings.Contains(e.Message, tc.label) {
				unsupported = e.Unsupported
			}
		}
		if !unsupported {
			t.Errorf("%s: the finding is not marked Unsupported — the markup is "+
				"correct and it is the engine that cannot read it", tc.what)
		}
	}
}

// TestADeclaredUTF8IsNotReported. A document saying what is already true is
// every well-formed page on the web, and must gain nothing.
func TestADeclaredUTF8IsNotReported(t *testing.T) {
	for _, src := range []string{
		`<meta charset="utf-8">`,
		`<meta charset=UTF-8>`,
		`<meta charset='utf8'>`,
		`<meta http-equiv="content-type" content="text/html; charset=utf-8">`,
		// A <meta> that is about something else entirely.
		`<meta name="viewport" content="width=device-width">`,
		`<meta name="description" content="a page about charsets">`,
	} {
		errs, ok := errorsOf(t, "<html><head>"+src+"</head><body>x</body></html>")
		if !ok || len(errs) != 0 {
			t.Errorf("%q was reported: %v", src, errs)
		}
	}
}

// TestOnlyTheFirstDeclarationIsReported. A document is in one encoding; two
// findings about it say nothing the first did not.
func TestOnlyTheFirstDeclarationIsReported(t *testing.T) {
	errs, _ := errorsOf(t, `<meta charset="shift_jis"><meta charset="big5">`)
	n := 0
	for _, e := range errs {
		if strings.Contains(e.Message, "encoding") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d findings about the encoding, want one: %v", n, errs)
	}
}

// TestADeclarationTooFarInIsNotLookedFor pins the bound. The standard's own
// sniffing gives up after 1024 bytes, so a declaration past it is one no
// browser would have honoured either — and the walk is over raw bytes before
// the parse, so it has to end somewhere.
func TestADeclarationTooFarInIsNotLookedFor(t *testing.T) {
	pad := strings.Repeat("<p>x</p>", 300) // well past 1024 bytes
	errs, ok := errorsOf(t, pad+`<meta charset="shift_jis">`)
	if !ok || hasMessage(errs, "shift_jis") {
		t.Errorf("a declaration past the sniffing bound was read: %v", errs)
	}
}
