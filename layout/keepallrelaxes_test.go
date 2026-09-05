package layout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// word-break: keep-all, relaxed where a line has nothing else, CSS Text §5.2.
//
//	Note: this value may be relaxed by the UA if there are no
//	otherwise-acceptable break points in the line.
//
// The value's job is to keep a word whole, and it does that by ranking rather
// than by refusing: the opportunities it takes away come back as ones a line
// reaches for only when it has no other. A line with a space on it never sees
// them; a line with nothing else takes one rather than running off the page.
//
// The suite writes it as overflow-wrap/overflow-wrap-normal-keep-all-001 —
// eight ideographs, "width: 0", and a reference that is a column one character
// wide. Refusing outright put all eight on one line, seven of them outside the
// box, and "overflow-wrap: normal" left nothing to cut them with either.

// cjkFace is a font set that answers every family with Noto Sans JP, so that an
// ideograph is one em and a width in pixels is a number of characters.
func cjkFace(t *testing.T) FontSet {
	t.Helper()
	dir := os.Getenv("NOTO_FONTS")
	if dir == "" {
		t.Skip("set NOTO_FONTS (or run `make test-wpt`) for a face with ideographs")
	}
	data, err := os.ReadFile(filepath.Join(dir, "NotoSansJP-VF.ttf"))
	if err != nil {
		t.Skipf("reading the CJK face: %v", err)
	}
	face, err := loadSuiteFace(data)
	if err != nil {
		t.Skipf("parsing the CJK face: %v", err)
	}
	return oneFace{face}
}

// keepAllLines lays text out in a box of the given width and returns its lines.
func keepAllLines(t *testing.T, set FontSet, text string, widthPx int, wordBreak string) []string {
	t.Helper()
	root := layoutWithFonts(t, set, `<div id="p" lang="ja">`+text+`</div>`,
		`#p { font-size: 20px; width: `+itoa(widthPx)+`px; word-break: `+wordBreak+` }`)
	return lineTextsOf(t, root, "p")
}

// TestKeepAllGivesWayWhenTheLineHasNothingElse.
func TestKeepAllGivesWayWhenTheLineHasNothingElse(t *testing.T) {
	set := cjkFace(t)
	for _, tc := range []struct {
		width int
		want  []string
		what  string
	}{
		{0, []string{"文", "文", "文", "文"}, "no room at all, which is the suite's own fixture"},
		{20, []string{"文", "文", "文", "文"}, "one character of room"},
		{40, []string{"文文", "文文"}, "two characters of room"},
		// Room for the whole run, so there is nothing to give way to: the
		// relaxation must not break a line that fits.
		{200, []string{"文文文文"}, "room for all four"},
	} {
		got := keepAllLines(t, set, "文文文文", tc.width, "keep-all")
		if strings.Join(got, "|") != strings.Join(tc.want, "|") {
			t.Errorf("%s: keep-all set %q, want %q", tc.what, got, tc.want)
		}
	}
}

// TestKeepAllStillHoldsAWordTogetherWhereTheLineHasSomewhereElseToBreak is the
// half that matters more: the relaxation must not turn keep-all into normal.
//
// Each row has an ordinary opportunity — the space — and demoted ones inside
// the words. keep-all ends the line at the space and lets the word that follows
// overflow; normal fills the line and breaks inside that word. A demotion that
// was not a demotion would answer alike, so each row checks that too.
func TestKeepAllStillHoldsAWordTogetherWhereTheLineHasSomewhereElseToBreak(t *testing.T) {
	set := cjkFace(t)
	for _, tc := range []struct {
		text  string
		width int
		keep  []string
	}{
		// Two characters of room and a two-character word after the space:
		// keep-all holds it whole on a line of its own, normal takes its first
		// character up to fill the line above.
		{"文 文文", 50, []string{"文", "文文"}},
		// Three characters of room and a three-character word, which keep-all
		// keeps whole even though it does not fit at all.
		{"文 文文文", 60, []string{"文", "文文文"}},
	} {
		keep := keepAllLines(t, set, tc.text, tc.width, "keep-all")
		normal := keepAllLines(t, set, tc.text, tc.width, "normal")
		if strings.Join(keep, "|") == strings.Join(normal, "|") {
			t.Errorf("%q in %dpx: keep-all and normal both set %q, so this "+
				"document does not distinguish the values", tc.text, tc.width, keep)
			continue
		}
		if strings.Join(keep, "|") != strings.Join(tc.keep, "|") {
			t.Errorf("%q in %dpx: keep-all set %q, want %q — the space is the "+
				"opportunity the line reaches for, and the ones inside the word "+
				"are not", tc.text, tc.width, keep, tc.keep)
		}
	}
}
