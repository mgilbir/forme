package layout

import (
	"fmt"
	"strings"
	"testing"
)

// TestTheLanguageIsReadOncePerElement is the cost of the language questions the
// layout asks of each piece of text: which casing it takes, which hyphenation
// patterns and orthography, which writing system it is typeset as.
//
// Each was html.Node.Language, a walk to the root reading every attribute of
// every element on the way, and each text node asks several of them when its
// box is built and again when it is laid out. So a document of nested elements
// with a few words in each cost the square of its depth times the attributes on
// each element:
//
//	<div lang="tr"> + n × <div data-a0="v" … data-a249="v">i <b>i</b> i + closing tags
//
// laid out in 24 ms at fifty levels and 285 ms at two hundred, twelve times the
// time for four times the document. The builder, the layouter and the loader of
// replaced content each answer the questions from one html.Languages now, which
// works each element's language out once.
//
// Elements with many attributes because the attributes are what each step of the
// walk read, and blocks rather than spans so that nothing about inline nesting
// is measured here as well: the painter's chain of inline boxes is the square of
// the nesting by construction, since each run carries the list of boxes it is
// inside.
func TestTheLanguageIsReadOncePerElement(t *testing.T) {
	var attrs strings.Builder
	for i := 0; i < 250; i++ {
		fmt.Fprintf(&attrs, ` data-a%d="v"`, i)
	}
	doc := func(depth int) string {
		return `<div lang="tr">` + strings.Repeat(`<div`+attrs.String()+`>i <b>i</b> i `, depth) +
			strings.Repeat(`</div>`, depth) + `</div>`
	}
	compose := func(src string) func() {
		return func() {
			if got := Compose(Input{HTML: src}, Options{}); got.Root == nil {
				panic("the document produced nothing")
			}
		}
	}
	// The Turkish casing reached the text at the bottom, or the language was
	// never read there and nothing above measured the walk.
	deepest := Compose(Input{HTML: doc(40), CSS: []Stylesheet{{Source: `div { text-transform: uppercase }`}}},
		Options{})
	if text := drawnText(deepest.Ops); !strings.Contains(text, "İ") || strings.Contains(text, "I") {
		t.Fatalf("the text was set as %q; it is Turkish all the way down and every i "+
			"is a dotted capital", text)
	}

	lo, hi, ratio := layoutScaling(compose(doc(40)), compose(doc(160)))
	if ratio > 8 {
		t.Errorf("forty levels took %v and a hundred and sixty %v, a factor of %.1f: "+
			"linear is four, and a walk to the root per question is sixteen",
			lo, hi, ratio)
	}
}
