package html

import "testing"

// TestACharsetLabelEndsAtASCIIWhiteSpace. The label in a <meta> is read
// between HTML's white space, all five of it: "windows-1252" followed by a
// line feed or a form feed is windows-1252, and a document declaring it is
// reported like any other that does. The reader stopped only at a space or a
// tab, so the newline became part of the label, and the finding named an
// encoding nobody declared — and "utf-8" followed by a newline, which is what
// this engine reads, was reported as an encoding it cannot.
func TestACharsetLabelEndsAtASCIIWhiteSpace(t *testing.T) {
	for _, meta := range []string{
		"<meta charset=\"utf-8\n\">",
		"<meta charset=\"\futf-8\r\n\">",
	} {
		if errs, ok := errorsOf(t, "<html><head>"+meta+"</head><body>caf\u00e9</body></html>"); !ok || len(errs) > 0 {
			t.Errorf("%q: a document declaring UTF-8 was reported: %v", meta, errs)
		}
	}
	for _, meta := range []string{
		"<meta charset=\"windows-1252\n\">",
		"<meta charset=\"windows-1252\f\">",
		"<meta charset=\"\r\nwindows-1252\r\n\">",
		"<meta http-equiv=content-type content=\"text/html; charset=windows-1252\n\">",
	} {
		errs, _ := errorsOf(t, "<html><head>"+meta+"</head><body>caf\u00e9</body></html>")
		if !hasMessage(errs, `"windows-1252"`) {
			t.Errorf("%q: the declaration was not read; errors: %v", meta, errs)
		}
	}
	// A no-break space is not white space to HTML, so it is part of what the
	// label says, and "\u00a0windows-1252" is not windows-1252.
	for _, ch := range []string{"\u00a0", "\u2003", "\u3000"} {
		errs, _ := errorsOf(t, "<html><head><meta charset=\""+ch+"windows-1252\"></head><body>caf\u00e9</body></html>")
		if hasMessage(errs, `"windows-1252"`) {
			t.Errorf("%q before the label: read as windows-1252; errors: %v", ch, errs)
		}
	}
}
