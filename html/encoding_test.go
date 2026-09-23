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
//
// The document holds a byte above 0x7F, which is what makes the declaration
// matter: see TestAnASCIIDocumentReadsTheSameUnderAnyASCIIEncoding for one that
// does not.
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
		errs, ok := errorsOf(t, "<html><head>"+tc.src+"</head><body>café</body></html>")
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
	errs, _ := errorsOf(t, `<meta charset="shift_jis"><meta charset="big5">café`)
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
	errs, ok := errorsOf(t, pad+`<meta charset="shift_jis">café`)
	if !ok || hasMessage(errs, "shift_jis") {
		t.Errorf("a declaration past the sniffing bound was read: %v", errs)
	}
}

// TestAnASCIIDocumentReadsTheSameUnderAnyASCIIEncoding. A legacy page that
// declares windows-1252 and writes everything outside ASCII as a reference holds
// the same text in every encoding that reads the bytes below 0x80 as ASCII,
// which is all but a few of them. Telling its author that "the text it read is
// not the text the document holds" was false.
func TestAnASCIIDocumentReadsTheSameUnderAnyASCIIEncoding(t *testing.T) {
	for _, label := range []string{
		"iso-8859-1", "windows-1252", "us-ascii", "shift_jis", "gb18030", "koi8-r",
		// Not a label the standard names: a browser ignores it.
		"x-made-up",
	} {
		src := `<meta charset="` + label + `"><p>caf&eacute; &#8212; na&#239;ve</p>`
		if errs, ok := errorsOf(t, src); !ok {
			t.Errorf("%s: an all-ASCII document was reported: %v", label, errs)
		}
	}
	// The ones that do not read ASCII as ASCII are still reported, ASCII bytes
	// or not: ISO-2022-JP writes Japanese in them.
	for _, label := range []string{"iso-2022-jp", "iso-2022-kr", "hz-gb-2312"} {
		errs, _ := errorsOf(t, `<meta charset="`+label+`"><p>x</p>`)
		if !hasMessage(errs, `"`+label+`"`) {
			t.Errorf("%s: an encoding that does not read ASCII bytes as ASCII was "+
				"passed over: %v", label, errs)
		}
	}
}

// TestAMetaDeclaringUTF16IsUTF8. The Encoding Standard's prescan turns a <meta>
// naming UTF-16 into UTF-8, because a "<meta" readable as ASCII bytes is not
// UTF-16 — so the document is read as UTF-8 by every browser, as it is here.
func TestAMetaDeclaringUTF16IsUTF8(t *testing.T) {
	for _, label := range []string{"utf-16", "UTF-16LE", "utf-16be", "unicode", "ucs-2"} {
		if errs, ok := errorsOf(t, `<meta charset="`+label+`"><p>café</p>`); !ok {
			t.Errorf("%s: reported: %v", label, errs)
		}
	}
}

// TestAByteOrderMarkOutranksTheDeclaration. Encoding sniffing reads the byte
// order mark first and never looks for a <meta> after one.
func TestAByteOrderMarkOutranksTheDeclaration(t *testing.T) {
	if errs, ok := errorsOf(t, bom+`<meta charset="windows-1252"><p>café</p>`); !ok {
		t.Errorf("a document with a UTF-8 byte order mark was reported: %v", errs)
	}
	// Without the mark, the same document is what the check is for.
	if errs, ok := errorsOf(t, `<meta charset="windows-1252"><p>café</p>`); ok {
		t.Errorf("a UTF-8 document declaring windows-1252 was not reported: %v", errs)
	}
}
