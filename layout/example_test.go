package layout_test

import (
	"fmt"

	"github.com/mgilbir/forme/layout"
)

// Example is the whole of what a backend does with a composed document, and it
// is here so that the README's version of it cannot go stale: the two are the
// same code, and this one is compiled.
//
// The three things beside the ops are not decoration. Refused is the verdict,
// Scale is a factor the backend applies because the ops are at the document's
// natural size, and Page is the sheet the document settled on — which is not
// always the sheet the caller asked for, because an @page rule may have changed
// it. The ops are measured from the top left of the content box, so a backend
// drawing on the whole sheet translates by the page's own margins.
func Example() {
	out := layout.Compose(layout.Input{
		HTML: "<h1>Invoice</h1><p>…</p>",
		CSS:  []layout.Stylesheet{{Source: "h1 { font: 24pt serif }"}},
	}, layout.Options{})

	// The verdict first: a refused document is one the caller was told not to
	// render, and the findings say why.
	for _, f := range out.Findings {
		fmt.Printf("%s: %s\n", f.Rule, f.Message)
	}
	if out.Refused {
		return
	}

	// Where the ops go on the paper, and how big.
	origin := layout.Point{X: out.Page.Margin.Left, Y: out.Page.Margin.Top}
	_ = origin
	_ = out.Scale

	var texts, fills, images, tiles int
	for _, op := range out.Ops {
		switch op.(type) {
		case layout.DrawText: // op.Text, op.Face, op.Size, op.At, op.RTL …
			texts++
		case layout.FillRect: // op.Rect, op.Color
			fills++
		case layout.DrawImage: // op.Image, op.Rect
			images++
		case layout.TileImage: // op.Image, op.Clip, op.Tile — a repeated background
			tiles++
		}
	}
	fmt.Println(texts > 0, fills, images, tiles)
	// Output: true 0 0 0
}
