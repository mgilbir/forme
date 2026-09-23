package layout

import (
	"strings"
	"testing"
)

// Which boxes ::first-letter reaches (audit C94). css-pseudo-4 applies it to a
// block container — every one, not only a plain block — and finds the letter on
// the first formatted line, which never lies inside an atomic inline or a box
// that is not a block container.

func TestEveryBlockContainerHasAFirstLetter(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"a plain block", `<div class="c">Hello</div>`},
		{"a flow root", `<div class="c" style="display:flow-root">Hello</div>`},
		{"a float", `<div class="c" style="float:left">Hello</div>`},
		{"an overflow box", `<div class="c" style="overflow:hidden">Hello</div>`},
		{"an inline-block", `<div><span class="c" style="display:inline-block">Hello</span></div>`},
		{"a table cell", `<table><tr><td class="c">Hello</td></tr></table>`},
		{"a list item", `<ul><li class="c">Hello</li></ul>`},
	} {
		got := textsOf(letterBoxes(t, tc.html, `.c::first-letter { font-size: 40px }`))
		if strings.Join(got, "|") != "H|ello" {
			t.Errorf("%s: the text is %q; the letter belongs in a box of its own", tc.what, got)
		}
	}
	// And not a flex container, which is not a block container: its text is
	// in an anonymous item, a block container of its own that no rule named.
	got := textsOf(letterBoxes(t, `<div class="c" style="display:flex">Hello</div>`,
		`.c::first-letter { font-size: 40px }`))
	if strings.Join(got, "|") != "Hello" {
		t.Errorf("a flex container's ::first-letter split its text: %q", got)
	}
}

func TestAFirstLetterIsNotLookedForInsideAnAtomicInline(t *testing.T) {
	for _, tc := range []struct{ what, html string }{
		{"an inline-block", `<div id="d"><span style="display:inline-block">Inner</span> outer</div>`},
		{"an inline-flex", `<div id="d"><span style="display:inline-flex">Inner</span> outer</div>`},
		{"an inline-table", `<div id="d"><span style="display:inline-table">Inner</span> outer</div>`},
		{"a table", `<div id="d"><table><tr><td>Inner</td></tr></table>outer</div>`},
		{"a flex container", `<div id="d"><div style="display:flex">Inner</div>outer</div>`},
	} {
		boxes := letterBoxes(t, tc.html, `#d::first-letter { font-size: 40px }`)
		for _, b := range boxes {
			if b.Text == "I" {
				t.Errorf("%s: the \"I\" inside it was made the first letter of the "+
					"element around it: %q", tc.what, textsOf(boxes))
			}
		}
	}
	// A plain inline box is on the element's own line and is looked inside.
	got := textsOf(letterBoxes(t, `<div id="d"><span>Inner</span> outer</div>`,
		`#d::first-letter { font-size: 40px }`))
	if len(got) == 0 || got[0] != "I" {
		t.Errorf("the letter inside a plain span was not found: %q", got)
	}
}
