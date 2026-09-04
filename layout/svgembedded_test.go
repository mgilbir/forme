package layout

import (
	"testing"

	"github.com/mgilbir/forme/style"
)

// An SVG embedded as a document, and what its own percentages are a percentage
// of.
//
// SVG gives the root element's width and height an initial value of 100%, so a
// file that states neither states both. What that comes to depends on how the
// file reaches the page, and the two answers are not near each other:
//
//   - As a *picture* — an <img>, a background — CSS Images §5.4 makes a
//     percentage no intrinsic dimension at all, and the element falls back to
//     CSS 2.1 §10.3.2's 300 by 150.
//   - As a *document* — an <object> — the file has a viewport of its own, and
//     that viewport is the box CSS gives the element. The percentage resolves
//     against the containing block, exactly as "width: 100%" would.
//
// The suite writes both: replaced-elements-all-auto for the first and
// replaced-intrinsic-001 for the second, whose own comment says "intrinsic size
// is 100%x100%, which is equivalent to width:100%".

// sizeOfReplacedWith lays a document out against a resolver and returns the
// border box of the element with id="r".
func sizeOfReplacedWith(t *testing.T, res ResourceResolver, htmlSrc, cssSrc string) Size {
	t.Helper()
	built := Build(Input{HTML: htmlSrc, Resources: res,
		CSS: []Stylesheet{{Source: noDefaults + cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	root := Layout(built.Root, Size{W: w, H: h}, StandardFonts(), NewRecorder(nil))
	f := find(t, root, "r")
	return Size{W: f.BorderRect.W, H: f.BorderRect.H}
}

const sizelessSVG = `<svg xmlns="http://www.w3.org/2000/svg">` +
	`<rect x="0" y="0" width="300" height="200" fill="green"/></svg>`

// TestASizelessSVGInAnObjectFillsItsContainingBlock.
func TestASizelessSVGInAnObjectFillsItsContainingBlock(t *testing.T) {
	res := mapResolver{"s.svg": []byte(sizelessSVG)}
	got := sizeOfReplacedWith(t, res,
		`<div id="d"><object id="r" data="s.svg" type="image/svg+xml"></object></div>`,
		`#d { width: 150px } #r { width: auto; height: auto }`)
	// The width is the containing block's; the height has no percentage to
	// resolve against an auto-height parent and no ratio, so it is the default.
	if want := (Size{W: bgpx(150), H: bgpx(150)}); got != want {
		t.Errorf("the object is %v by %v, want %v by %v — an SVG stating no size "+
			"states 100%%, and an embedded document's 100%% is its containing block",
			got.W.Px(), got.H.Px(), want.W.Px(), want.H.Px())
	}
}

// TestASizelessSVGInAnImageTakesTheDefaultObjectSize is the other half, and the
// one ten of the suite's documents depend on: as a picture the same file has no
// intrinsic dimension at all.
func TestASizelessSVGInAnImageTakesTheDefaultObjectSize(t *testing.T) {
	res := mapResolver{"s.svg": []byte(sizelessSVG)}
	got := sizeOfReplacedWith(t, res,
		`<div id="d"><img id="r" src="s.svg"></div>`,
		`#d { width: 150px } #r { width: auto; height: auto }`)
	if want := (Size{W: bgpx(300), H: bgpx(150)}); got != want {
		t.Errorf("the image is %v by %v, want %v by %v — a picture's percentage "+
			"is no dimension, and the element takes the default object size",
			got.W.Px(), got.H.Px(), want.W.Px(), want.H.Px())
	}
}

// TestAStatedSizeBeatsThePercentage. The percentage stands in for a declaration
// the document did not make, so a document that made one keeps it.
func TestAStatedSizeBeatsThePercentage(t *testing.T) {
	res := mapResolver{"s.svg": []byte(sizelessSVG)}
	got := sizeOfReplacedWith(t, res,
		`<div id="d"><object id="r" data="s.svg" type="image/svg+xml"></object></div>`,
		`#d { width: 150px } #r { width: 40px; height: 30px }`)
	if want := (Size{W: bgpx(40), H: bgpx(30)}); got != want {
		t.Errorf("the object is %v by %v, want %v by %v", got.W.Px(), got.H.Px(),
			want.W.Px(), want.H.Px())
	}
}

// TestAnSVGThatStatesItsOwnSizeIsNotAPercentage, whichever way it arrives: the
// initial value is only reached where the root said nothing.
func TestAnSVGThatStatesItsOwnSizeIsNotAPercentage(t *testing.T) {
	const sized = `<svg xmlns="http://www.w3.org/2000/svg" width="40" height="30">` +
		`<rect x="0" y="0" width="40" height="30" fill="green"/></svg>`
	res := mapResolver{"s.svg": []byte(sized)}
	for _, tc := range []struct{ markup, what string }{
		{`<object id="r" data="s.svg" type="image/svg+xml"></object>`, "an object"},
		{`<img id="r" src="s.svg">`, "an image"},
	} {
		got := sizeOfReplacedWith(t, res, `<div id="d">`+tc.markup+`</div>`,
			`#d { width: 150px } #r { width: auto; height: auto }`)
		if want := (Size{W: bgpx(40), H: bgpx(30)}); got != want {
			t.Errorf("%s is %v by %v, want %v by %v", tc.what,
				got.W.Px(), got.H.Px(), want.W.Px(), want.H.Px())
		}
	}
}
