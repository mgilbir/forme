package layout

import (
	"strings"
	"testing"
)

// TestGeneratedContentIsCappedAcrossImages is the hole in the cap.
//
// maxContentLength exists because the quote keywords are the one source of text
// in a document that is not bounded by the document: "quotes" holds two strings
// of the author's choosing and "content: open-quote open-quote …" draws one per
// keyword, so eleven bytes of declaration fetch a megabyte of mark.
//
// The running total was only ever increased by an image's reference. The text
// built between images was written into a builder that a url() flushed and
// reset, so the cap looked at whatever had accumulated since the last picture —
// which for "open-quote url(a) open-quote url(a) …" is one quote string. Four
// megabytes of marks came out of a two-hundred-kilobyte stylesheet, and the
// finding the cap exists to raise was never raised.
func TestGeneratedContentIsCappedAcrossImages(t *testing.T) {
	// A quote pair whose opening mark is 200 KB, and a value that draws it
	// twenty times with a picture between each.
	big := strings.Repeat("q", 200<<10)
	quotes := quoteList{{open: big, close: big}}

	var value strings.Builder
	for i := 0; i < 20; i++ {
		value.WriteString(`open-quote url(a.png) `)
	}

	got := resolveContent(value.String(), nil, nil, quotes, 0)
	if got.unsupported == "" {
		t.Fatalf("a value producing %d bytes of marks was accepted with no finding",
			len(got.text()))
	}
	if !strings.Contains(got.unsupported, "longer than this engine will generate") {
		t.Errorf("it was refused as %q, want the length message", got.unsupported)
	}
}

// TestGeneratedContentStaysUnderTheCap is what the cap has to be measured
// against: whatever comes back, the text of it is bounded.
func TestGeneratedContentStaysUnderTheCap(t *testing.T) {
	big := strings.Repeat("q", 200<<10)
	quotes := quoteList{{open: big, close: big}}
	for _, tc := range []struct{ name, value string }{
		{"quotes alone", strings.Repeat("open-quote ", 40)},
		{"quotes between pictures", strings.Repeat("open-quote url(a.png) ", 40)},
		{"a picture then quotes", "url(a.png) " + strings.Repeat("open-quote ", 40)},
		{"strings between pictures", strings.Repeat(`"`+big+`" url(a.png) `, 40)},
	} {
		got := resolveContent(tc.value, nil, nil, quotes, 0)
		if n := len(got.text()); n > 2*maxContentLength {
			t.Errorf("%s produced %d bytes, past twice the cap of %d",
				tc.name, n, maxContentLength)
		}
	}
}

// TestOrdinaryGeneratedContentIsNotRefused is the control: a cap that refuses
// everything would pass the tests above and produce no marker at all.
func TestOrdinaryGeneratedContentIsNotRefused(t *testing.T) {
	quotes := quoteList{{open: "“", close: "”"}}
	for _, tc := range []struct{ name, value, want string }{
		{"a string", `"hello"`, "hello"},
		{"a quoted pair", `open-quote "hello" close-quote`, "“hello”"},
		{"text either side of a picture", `"a" url(x.png) "b"`, "ab"},
		{"many quotes", strings.Repeat("open-quote ", 500), strings.Repeat("“", 500)},
	} {
		got := resolveContent(tc.value, nil, nil, quotes, 0)
		if got.unsupported != "" {
			t.Errorf("%s was refused: %s", tc.name, got.unsupported)
			continue
		}
		if got.text() != tc.want {
			t.Errorf("%s produced %q, want %q", tc.name, got.text(), tc.want)
		}
	}
}
