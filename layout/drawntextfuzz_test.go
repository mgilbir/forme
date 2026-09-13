package layout

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/mgilbir/forme/fonts/notosans"
	"github.com/mgilbir/forme/shape"
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
	// And text that no one face can set, which is the only way a piece is cut
	// into more than one *face* run. Without it cutRunsAt is a function this
	// target walks past: a planted version that drops a run moves nothing,
	// because every run has one face and there is nothing to drop.
	//
	// Cyrillic and Greek are what make this work with the fonts in the
	// repository. None of the base fourteen covers either and the bundled Noto
	// Sans covers both, so a box asking for Courier gets Courier for its Latin
	// and Noto Sans for the rest — see fallbackFaceSet. 153 of the lines these
	// seeds set hold more than one face, where the corpus without them held
	// none at all.
	//
	// cutRunsAt is still not reached, and that is worth writing down rather than
	// leaving as a thing somebody re-derives. It cuts a piece where §8.1 wants a
	// gap inside one, where §8.2's cursive tracking starts, and after a word
	// separator — and each of those needs a piece that has *both* more than one
	// face run and more than one part. A piece with two faces is a script
	// boundary, and a script boundary is where the pieces are cut already, so
	// the two conditions have not been made to meet. Planted both ways with
	// nothing moving; what does move is a face run dropped or truncated where
	// the piece is split by face, which is the path these seeds opened.
	for _, text := range []string{
		"aЖb", "Жab", "abЖ", "aЖЖb", "a Жb", "aαb", "αaα", "aЖ αb",
	} {
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
	set := fallbackFaceSet{family: "T", face: face, standard: StandardFonts()}

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

// fallbackFaceSet is the test font set with the one method that makes a
// substitution possible.
//
// namedFaceSet, which the other targets here use, answers only "give me this
// family" — so every run is set in one face, every piece is one face run, and a
// whole stage of the engine is walked past. This adds FallbackFontSet's second
// question, "give me something that can set *this text*", which is what cuts a
// piece into runs of its own.
//
// It is here rather than on namedFaceSet because being a FallbackFontSet changes
// what the engine *reports*: a substitution is a finding, and the tests beside
// that type are about which findings a document raises. This target does not
// read findings at all.
type fallbackFaceSet struct {
	family   string
	face     *shape.Face
	standard FontSet
}

func (s fallbackFaceSet) Face(family string, bold, italic bool) (*shape.Face, bool) {
	if strings.EqualFold(strings.TrimSpace(family), s.family) {
		return s.face, true
	}
	return s.standard.Face(family, bold, italic)
}

// FaceFor offers the bundled face for text the named family could not set, and
// only where it can set the whole of it — which is what the interface asks for.
func (s fallbackFaceSet) FaceFor(text string, bold, italic bool) (*shape.Face, bool) {
	if _, missing := s.face.ShapeGlyphs(text); missing == 0 {
		return s.face, true
	}
	return nil, false
}
