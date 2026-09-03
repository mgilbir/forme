package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// word-break: auto-phrase over a document.
//
// paragraph/wordbreakautophrase_test.go states the rule over one run of text:
// which opportunities the value keeps, which it gives up, and that giving up is
// not deleting. What a document adds is the two places the rank is read — the
// line fill, which reaches for a given-up opportunity only when it has no
// other, and the intrinsic widths, which do not reach for one at all.

// autoPhraseFaces is the fallback library with a check that it can set the
// fixture, which is Japanese and so needs a face the standard fourteen do not
// include.
func autoPhraseFaces(t *testing.T) []*shape.Face {
	t.Helper()
	faces := kernFaces(t)
	if _, ok := faceWithGlyphFor(faces, "楽しいドライブ。"); !ok {
		t.Skip("no face here can set the fixture")
	}
	return faces
}

// The fixture is the suite's own: "楽しいドライブ。" is two phrases, 楽しい and
// ドライブ。, and every auto-phrase test that measures anything is built on it.
const autoPhraseText = `<div id="d" lang="ja">楽しいドライブ。</div>`

const autoPhraseCSS = `#d { font-family: monospace; font-size: 20px; ` +
	`text-autospace: no-autospace; word-break: auto-phrase }`

// A box that shrinks to fit is as wide as the widest phrase, not as wide as the
// widest character.
//
// That is what the suite's word-break-auto-phrase-intrinsic-001 asks for, and
// it is the whole of why the rank exists rather than a second set of
// opportunities: an opportunity the line gives up is not one a minimum may be
// measured against, because measuring against it would return a width the
// content only fits in by doing the thing the value asked it not to.
func TestAutoPhraseShrinksToItsWidestPhrase(t *testing.T) {
	faces := autoPhraseFaces(t)
	got := cjkFaceLines(t, faces, autoPhraseText,
		autoPhraseCSS+` #d { width: min-content }`)
	if len(got) != 2 || got[0] != "楽しい" || got[1] != "ドライブ。" {
		t.Errorf("at width: min-content the lines are %q, want the two phrases", got)
	}
	// And the control, which is what makes the assertion about this value: the
	// same text under "normal" narrows to one character, because every
	// boundary between two ideographs is an opportunity a minimum counts.
	normal := cjkFaceLines(t, faces, autoPhraseText,
		strings.Replace(autoPhraseCSS, "auto-phrase", "normal", 1)+
			` #d { width: min-content }`)
	if len(normal) <= len(got) {
		t.Errorf("at width: min-content, normal gave %q and auto-phrase gave %q — "+
			"the value narrowed the box no less than normal did", normal, got)
	}
}

// A phrase that does not fit is divided anyway, and the suppression is given up
// one line at a time rather than for the whole box.
//
// word-break-auto-phrase-009's assert is the sentence: "auto-phrase's must give
// up on suppressing wrapping opportunities when that would lead to overflow."
// The box here is exactly as wide as the first phrase, so the first line ends
// where the value wants it to and the second has nowhere to end at all.
func TestAPhraseWiderThanItsLineIsDividedAnyway(t *testing.T) {
	faces := autoPhraseFaces(t)
	width := cjkNaturalWidth(t, faces, `<div id="d" lang="ja">楽しい</div>`, autoPhraseCSS)
	css := autoPhraseCSS + ` #d { width: ` + fmtPx(width) + ` }`

	// The fixture assumes the three ideographs of "ドライ" are as wide as the
	// three of "楽しい" and that a fourth is not, which is true of every face
	// that sets Japanese and is checked rather than assumed.
	if cjkNaturalWidth(t, faces, `<div id="d" lang="ja">ドライ</div>`, autoPhraseCSS) > width ||
		cjkNaturalWidth(t, faces, `<div id="d" lang="ja">ドライブ</div>`, autoPhraseCSS) <= width {
		t.Skip("this face does not set the fixture on an even body")
	}

	got := cjkFaceLines(t, faces, autoPhraseText, css)

	// Line by line: the first ends where the value wants it to, the second has
	// nowhere it wants to end and ends as late as it can anyway, and the third
	// is what UAX #14 would not let the second take — a line may not begin with
	// an ideographic full stop, so the suppression is given up and the
	// prohibition is not. The suite writes that half as a comment of its own:
	// "Only one character can fit to 1em, but no break before the period".
	want := []string{"楽しい", "ドライ", "ブ。"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("the lines are %q, want %q", got, want)
	}
	// The control is keep-all, whose suppression is a prohibition: the same
	// text in the same box has nowhere to break and overflows on one line.
	keepAll := cjkFaceLines(t, faces, autoPhraseText,
		strings.Replace(autoPhraseCSS, "auto-phrase", "keep-all", 1)+
			` #d { width: `+fmtPx(width)+` }`)
	if len(keepAll) != 1 {
		t.Errorf("under keep-all the lines are %q, want one — the value forbids "+
			"every opportunity in the text, so it overflows rather than dividing "+
			"anything, and if it divides the phrase too then the test above says "+
			"nothing about auto-phrase", keepAll)
	}
}

// An opportunity the author wrote is not the value's to give up, so a line ends
// at it in preference to nothing and without waiting for an overflow.
// word-break-auto-phrase-007: "UAs must not suppress wrapping opportunities
// introduced by wbr or ZWSP."
func TestAWbrInsideAPhraseStillEndsALine(t *testing.T) {
	faces := autoPhraseFaces(t)
	width := cjkNaturalWidth(t, faces, `<div id="d" lang="ja">ドラ</div>`, autoPhraseCSS)
	for _, tc := range []struct{ html, what string }{
		{`<div id="d" lang="ja">ドラ<wbr>イブ</div>`, "a <wbr>"},
		{"<div id=\"d\" lang=\"ja\">ドラ​イブ</div>", "a zero width space"},
	} {
		got := cjkFaceLines(t, faces, tc.html, autoPhraseCSS+` #d { width: `+fmtPx(width)+` }`)
		if len(got) != 2 || !strings.HasPrefix(got[1], "イ") {
			t.Errorf("%s inside a phrase: the lines are %q, want the break the "+
				"author asked for", tc.what, got)
		}
	}
}
