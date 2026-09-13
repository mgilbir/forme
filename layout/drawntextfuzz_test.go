package layout

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/fonts/notosans"
)

// The page draws the document's characters, all of them, once each, in order.
//
// It is the plainest thing a layout engine promises and the one nothing here
// checked. FuzzRunTiling and FuzzBoundaryLines both compare *two spellings of
// one document* to each other, so a character lost or drawn twice in both of
// them is a page they agree about and neither can see; FuzzRender compares a
// document to *itself*, run twice, which is determinism and not correctness.
// This is the third question — what the page holds against what the source said
// — and it is the one that catches a character going missing.
//
// It is written against the *visible* characters: white space is left out of
// both sides, and so are the characters that draw nothing. That is what makes
// the claim true rather than nearly true. §4.1.1 collapses, trims and removes
// white space by the rule and by the line, so a comparison that counted spaces
// would be a second implementation of white-space processing rather than a check
// on anything; and a zero width space or a bidi control is in the source and on
// no page.
//
// # What is left out, and why each is not a weakening
//
// A soft hyphen is excluded by precondition. A line that breaks at one draws a
// hyphen that is in no source — "high­way" comes out "high‐way" — which is
// §6.1 working, not a character invented.
//
// text-transform and font-variant-caps are left out of the declarations for the
// same reason from the other end: both rewrite the characters on purpose, which
// is what they are for. They have tests that check they rewrite them *correctly*,
// which is a different question from this one and better asked where it is.
//
// # It was planted against, because a new assertion that finds nothing is a
// tautology
//
// A line break inside a word that loses a character off the tail: 82 cases of
// the seed corpus fail it. That is the class this exists for, and it is the
// class the two-spelling targets are structurally blind to.
func FuzzDrawnText(f *testing.F) {
	for _, text := range tilingTexts {
		for which := range drawnDecls {
			f.Add(text, uint8(which))
		}
	}
	f.Fuzz(func(t *testing.T, text string, which uint8) {
		checkDrawnText(t, text, drawnDecls[int(which)%len(drawnDecls)])
	})
}

// drawnDecls are the declarations each case is laid out under.
//
// boundaryDecls without the two that rewrite characters. Everything here changes
// where a line ends or how wide a run is, and none of it changes what the
// characters *are*.
var drawnDecls = []string{
	"",
	"letter-spacing: 3px",
	"letter-spacing: -1px",
	"word-spacing: 5px",
	"text-indent: 10px",
	"text-align: justify",
	"text-align: right",
	"text-align: center",
	"hanging-punctuation: last",
	"hanging-punctuation: first",
	"word-break: break-all",
	"word-break: keep-all",
	"line-break: anywhere",
	"white-space: pre-wrap",
	"white-space: break-spaces",
	"overflow-wrap: break-word",
	"overflow-wrap: anywhere",
}

// checkDrawnText lays the text out and compares what is drawn with what was
// given.
func checkDrawnText(t testing.TB, text, decl string) {
	if len(text) > 512 {
		// Bounded for the reason every target here is: a long input finds the
		// memory it takes to hold one and no logic fault.
		return
	}
	if strings.ContainsAny(text, "<>&\x00") {
		// Markup this would have to escape, and the null — see checkRunTiling,
		// where what HTML parsing does to it is written out.
		return
	}
	if strings.ContainsRune(text, 0x00AD) {
		// A soft hyphen draws a hyphen where a line breaks at it. See above.
		return
	}
	if !utf8.ValidString(text) {
		// What reaches layout is then not the text that was asked about.
		return
	}
	want := visibleRunes(text)

	face, err := notosans.Face()
	if err != nil {
		t.Fatalf("loading the embedded Noto Sans: %v", err)
	}
	set := namedFaceSet{family: "T", face: face, standard: StandardFonts()}

	for _, family := range tilingFaces {
		sheet := `#d { font-family: ` + family + `; font-size: 16px; ` + decl + ` }`
		for _, px := range boundaryLineWidths {
			lines, ok := linesOfSpanned(t, set, text, sheet, px)
			if !ok {
				continue
			}
			var drawn strings.Builder
			for _, line := range lines {
				drawn.WriteString(line.Text)
			}
			if got := visibleRunes(drawn.String()); got != want {
				t.Fatalf("in %s at %gpx under %q, %q drew %q and the source says "+
					"%q\n  lines: %v\nevery character of a document is on its "+
					"page, once, in the order it was written.",
					family, px, decl, text, got, want, lines)
			}
		}
	}
}

// visibleRunes is the characters of a string a reader would see: the white space
// and the characters that draw nothing taken out.
//
// Both sides of the comparison go through it, so what is compared is a sequence
// of real characters in order — which survives collapsing, trimming, wrapping and
// reordering, and does not survive one going missing or being drawn twice.
func visibleRunes(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsSpace(r) || isBidiControl(r) || isDefaultIgnorable(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
