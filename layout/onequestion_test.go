package layout

import (
	"strings"
	"testing"
)

// The layout end of paragraph/onequestion_test.go: the same text questions,
// asked across the box boundaries only this package can see.

// textOfBoxes is the text of every text box under the root, in document order.
func textOfBoxes(t *testing.T, markup, css string) string {
	t.Helper()
	built := build(t, markup, css)
	if built.Root == nil {
		t.Fatalf("%q produced no boxes", markup)
	}
	var b strings.Builder
	var walk func(*Box)
	walk = func(box *Box) {
		if box.IsText() {
			b.WriteString(box.Text)
		}
		for _, c := range box.Children {
			walk(c)
		}
	}
	walk(built.Root)
	return b.String()
}

// TestAFinalSigmaSeesTheNextBox is audit C176: "ΟΔΟΣ<b>ΑΚΙ</b>" is one word, so
// its Σ is not final — and it was lowercased to ς, because Final_Sigma was
// decided inside each text node.
func TestAFinalSigmaSeesTheNextBox(t *testing.T) {
	const css = `div { text-transform: lowercase }`
	for _, tc := range []struct{ markup, want string }{
		{`<div>ΟΔΟΣ<b>ΑΚΙ</b></div>`, "οδοσακι"},
		{`<div>ΟΔΟΣ<b>'</b><i>ΑΚΙ</i></div>`, "οδοσ'ακι"},
		// The box after says nothing cased follows: final, as it was.
		{`<div>ΟΔΟΣ<b>.</b> ΑΚΙ</div>`, "οδος. ακι"},
		{`<div>ΟΔΟΣ<b>,</b>ΑΚΙ</div>`, "οδος,ακι"},
		{`<div>ΟΔΟΣ<b> ΑΚΙ</b></div>`, "οδος ακι"},
		// A line break and a block end the word.
		{`<div>ΟΔΟΣ<br>ΑΚΙ</div>`, "οδοςακι"},
		{`<div><p>ΟΔΟΣ</p><p>ΑΚΙ</p></div>`, "οδοςακι"},
		// The look back: a sigma at the start of a box after a cased letter.
		{`<div>ΟΔΟ<b>Σ</b></div>`, "οδος"},
		{`<div>ΟΔΟ<b>Σ</b>Ι</div>`, "οδοσι"},
		// The following box need not be transformed for its letters to count.
		{`<div>ΟΔΟΣ<b style="text-transform:none">ΑΚΙ</b></div>`, "οδοσΑΚΙ"},
	} {
		if got := textOfBoxes(t, tc.markup, css); got != tc.want {
			t.Errorf("%s: the text is %q, want %q", tc.markup, got, tc.want)
		}
	}
}

// TestAnAutoPhraseBoundaryDoesNotMoveWithTheMarkup is audit C122 end to end: an
// inline element written inside a Japanese phrase must not open a break the
// phrase withheld, so the sentence breaks into the same lines with and without
// it, at every width.
func TestAnAutoPhraseBoundaryDoesNotMoveWithTheMarkup(t *testing.T) {
	const css = `#d{word-break:auto-phrase}`
	differed := 0
	// The second is not Japanese anyone would write. It is there because the
	// model's reading of the boundary after its second character changes with
	// the characters *after* a cut three characters in — which is the one case
	// that needs the text after a box (Carried.PhraseAfter) and not only the
	// text before it, and no sentence of the first kind happens to reach it.
	for _, sentence := range []string{"日本語を勉強します。東京へ行きましょう。", "私ん東ノ。強語常"} {
		runes := []rune(sentence)
		for cut := 1; cut < len(runes); cut++ {
			split := string(runes[:cut]) + "<a>" + string(runes[cut:]) + "</a>"
			for px := 16.0; px <= 16*12; px += 8 {
				plain := linesOfMarkupCSS(t, `<span lang="ja">`+sentence+`</span>`, px, css)
				marked := linesOfMarkupCSS(t, `<span lang="ja">`+split+`</span>`, px, css)
				if strings.Join(plain, "|") != strings.Join(marked, "|") {
					differed++
					if differed <= 5 {
						t.Errorf("an <a> after %q at %gpx: %q, and without it %q",
							string(runes[:cut]), px, marked, plain)
					}
				}
			}
		}
	}
}

// TestASpaceBeforeABidiControlDoesNotPushTheControlDown is audit C118 on the
// page: "aaaa &#x202C;" in a box exactly as wide as the word was two lines, the
// second holding nothing but the control.
func TestASpaceBeforeABidiControlDoesNotPushTheControlDown(t *testing.T) {
	for _, markup := range []string{
		"aaaa \u202c", "aaaa \u2069", "aaaa <span> \u202c</span>", "aaaa",
	} {
		if n := len(linesOfMarkupCSS(t, markup, 26, `#d{font:10px Courier}`)); n != 1 {
			t.Errorf("%q in a box one word wide is %d lines, want 1", markup, n)
		}
	}
}
