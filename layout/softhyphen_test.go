package layout

import (
	"strings"
	"testing"
)

// TestNoLineBreaksAtASoftHyphen pins a fact another package depends on.
//
// U+00AD asks a renderer to hyphenate *here if the line must break here*, and a
// browser under the initial "hyphens: manual" does exactly that. This engine
// does not: it breaks at no soft hyphen, so what it produces is "hyphens: none".
//
// style/inert.go records that, and it is the entry there that is a claim about
// behaviour rather than a reading of a specification — so it is checked here,
// where the behaviour is, rather than asserted there where it would rot. If
// someone implements breaking at a soft hyphen, this fails and points at the
// table that has to change with it. Without that, the table would go on saying
// "hyphens: none is inert" after it had stopped being true, and would suppress a
// finding about a real difference.
//
// Courier at 20px is 12px a character, so a 60px box holds five: "aaaa­bbbb"
// is eight characters and must break somewhere if it breaks at all.
func TestNoLineBreaksAtASoftHyphen(t *testing.T) {
	const soft = "­"
	root := layoutOf(t, 600, `<div id="p">aaaa`+soft+`bbbb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px }`)
	f := find(t, root, "p")
	if len(f.Lines) != 1 {
		t.Fatalf("%d lines: the text was broken, and style/inert.go records that a "+
			"soft hyphen is never a break here — if breaking at one is now "+
			"implemented, the hyphens entry there must change with it", len(f.Lines))
	}

	// And the fixture can break, so the single line above is the engine's answer
	// about the soft hyphen rather than a box that was wide enough all along.
	spaced := layoutOf(t, 600, `<div id="p">aaaa bbbb</div>`,
		`#p { font-family: Courier; font-size: 20px; width: 60px }`)
	if got := len(find(t, spaced, "p").Lines); got != 2 {
		t.Fatalf("the control broke into %d lines, want 2; the box is not narrow "+
			"enough for this test to be about anything", got)
	}
}

// TestASoftHyphenDrawsNoHyphen is the other half. A soft hyphen that is not at a
// break is invisible, so a page carrying one must look exactly like a page
// without it — otherwise "hyphens: none" would not be what this engine produces
// and the entry in style/inert.go would be wrong for a second reason.
func TestASoftHyphenDrawsNoHyphen(t *testing.T) {
	const css = `#p { font-family: Courier; font-size: 20px; width: 400px }`
	with := paintOf(t, "<div id=\"p\">aaaa­bbbb</div>", css)
	without := paintOf(t, `<div id="p">aaaabbbb</div>`, css)
	textOf := func(ops []Op) string {
		var b strings.Builder
		for _, op := range ops {
			if d, ok := op.(DrawText); ok {
				b.WriteString(d.Text)
			}
		}
		return b.String()
	}
	if strings.ContainsRune(textOf(with), '-') {
		t.Errorf("a hyphen was drawn for a soft hyphen that is not at a break")
	}
	if len(fillsOfAny(with)) != len(fillsOfAny(without)) {
		t.Errorf("a soft hyphen changed what is painted")
	}
}

// fillsOfAny counts the fills in a display list, whatever their colour.
func fillsOfAny(ops []Op) []FillRect {
	var out []FillRect
	for _, op := range ops {
		if r, ok := op.(FillRect); ok {
			out = append(out, r)
		}
	}
	return out
}
