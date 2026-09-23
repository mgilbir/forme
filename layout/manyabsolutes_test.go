package layout

import (
	"strings"
	"testing"
)

// TestADocumentOfManyAbsoluteBoxesIsPlacedWhole is audit C129. A converter such
// as pdf2htmlEX places every line of a page absolutely, and a long document has
// more such lines than the old bound of 16384: everything past it was left off
// the page. The bound is the box cap now, which no document can pass legitimately,
// so every one of these is placed.
func TestADocumentOfManyAbsoluteBoxesIsPlacedWhole(t *testing.T) {
	if maxAbsolutes < maxBoxes {
		t.Fatalf("the placement bound is %d and the box cap %d; a document of that "+
			"many positioned boxes would lose the rest", maxAbsolutes, maxBoxes)
	}
	const n = 1<<14 + 100
	doc := `<div style="position:relative">` +
		strings.Repeat(`<i style="position:absolute;left:0;top:0;width:1px;height:1px"></i>`, n) +
		`</div>`
	got := Compose(Input{HTML: doc}, Options{})
	for _, f := range got.Findings {
		if f.Rule == RuleLimit {
			t.Fatalf("a document of %d positioned boxes reached a limit: %s", n, f.Message)
		}
	}
	placed := 0
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f.Box != nil && f.Box.Position == PositionAbsolute {
			placed++
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(got.Root)
	if placed != n {
		t.Errorf("%d of %d positioned boxes were placed", placed, n)
	}
}
