package layout

import (
	"path/filepath"
	"testing"

	"github.com/mgilbir/forme/style"
)

// CSS 2.1 §12.6.2's list-style-image: a picture drawn where the bullet would
// have been.
//
// The sentence the whole property turns on is that it applies only while the
// image is *available*. A url that does not load is not an error and not a
// missing marker — it is the marker list-style-type would have drawn, which is
// why the type is still cascaded and still read for an item naming an image.

// markerLayout lays a document out against a directory holding one 15x15
// picture, which is the shape the suite's own fixtures use.
func markerLayout(t *testing.T, htmlSrc, cssSrc string) (*Fragment, []Finding) {
	t.Helper()
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "blue15x15.png"), 15, 15)

	res, err := NewDirResolver(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Close() })

	built := Build(Input{HTML: htmlSrc, Resources: res, CSS: []Stylesheet{{Source: cssSrc}}})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(A4.Content().W.Px())
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, nil, rec)
	if frag == nil {
		t.Fatal("layout produced no fragment")
	}
	return frag, append(append([]Finding(nil), built.Findings...), rec.Findings()...)
}

// markerOf returns the marker of the fragment with the given id.
func markerOf(t *testing.T, root *Fragment, id string) *Marker {
	t.Helper()
	f := find(t, root, id)
	if f.Marker == nil {
		t.Fatalf("#%s generated no marker", id)
	}
	return f.Marker
}

// TestAnImageMarkerIsDrawnAtItsOwnSize. §12.6.2 gives no way to scale the
// picture, so it goes on the page at its intrinsic size.
func TestAnImageMarkerIsDrawnAtItsOwnSize(t *testing.T) {
	root, findings := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: url(blue15x15.png) }`)
	for _, f := range findings {
		t.Errorf("an image marker that loaded reported %s", f.Error())
	}
	m := markerOf(t, root, "i")
	if m.Image == nil {
		t.Fatal("the marker carries no image")
	}
	px(t, "the marker's width", m.ImageRect.W, 15)
	px(t, "the marker's height", m.ImageRect.H, 15)
}

// TestAnImageMarkerSitsInTheMarginOnTheBaseline: an outside marker is clear of
// the content box, and the picture rests on the baseline rather than floating
// beside it.
func TestAnImageMarkerSitsInTheMarginOnTheBaseline(t *testing.T) {
	root, _ := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: url(blue15x15.png) }`)
	m := markerOf(t, root, "i")
	f := find(t, root, "i")

	inner := f.Border.Left.Add(f.Padding.Left)
	if m.ImageRect.Right() >= inner {
		t.Errorf("the marker reaches x=%v and the content box starts at %v; an "+
			"outside marker is in the margin, clear of the content",
			m.ImageRect.Right(), inner)
	}
	// Its bottom is the baseline the text marker would have been set on.
	if got, want := m.ImageRect.Bottom(), m.At.Y; got != want {
		t.Errorf("the picture's bottom is at %v and the baseline is at %v", got, want)
	}
}

// TestAnImageMarkerReplacesTheBullet is the word §12.6.2 uses. Drawing both
// would put a picture and a disc side by side, which no browser does and no
// author asked for.
func TestAnImageMarkerReplacesTheBullet(t *testing.T) {
	root, _ := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: url(blue15x15.png) }`)
	ops := Paint(root)

	var images, bullets int
	for _, op := range ops {
		switch o := op.(type) {
		case DrawImage:
			images++
		case DrawText:
			if o.Text == "•" {
				bullets++
			}
		}
	}
	if images != 1 {
		t.Errorf("%d images drawn, want the one marker", images)
	}
	if bullets != 0 {
		t.Errorf("the bullet was drawn as well as the picture, %d times", bullets)
	}
}

// TestAMarkerImageThatDoesNotLoadFallsBackSilently.
//
// §12.6.2 makes the property conditional on the image being available, so a url
// that does not load leaves a page that is *correct* — the marker is the one
// list-style-type asks for. Reporting it would be this engine calling a
// conformant rendering a limitation, and the suite writes "url(404)" on purpose.
func TestAMarkerImageThatDoesNotLoadFallsBackSilently(t *testing.T) {
	root, findings := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: url(nothing-here.png) }`)
	m := markerOf(t, root, "i")
	if m.Image != nil {
		t.Error("a marker image that did not load was drawn anyway")
	}
	if m.Text != "•" {
		t.Errorf("the marker fell back to %q, want the disc list-style-type asks for", m.Text)
	}
	for _, f := range findings {
		if f.Rule == RuleResourceBlocked || f.Rule == RuleImageUndecodable {
			t.Errorf("the fallback §12.6.2 requires was reported as a failure: %s", f.Error())
		}
	}
}

// TestListStyleImageNoneKeepsTheType is the initial value and the other half of
// the same rule.
func TestListStyleImageNoneKeepsTheType(t *testing.T) {
	root, _ := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: none; list-style-type: square }`)
	m := markerOf(t, root, "i")
	if m.Image != nil {
		t.Error("\"list-style-image: none\" drew a picture")
	}
	if m.Text != "▪" {
		t.Errorf("the marker is %q, want the square the type asks for", m.Text)
	}
}

// TestTheListStyleShorthandSetsTheImage: the shorthand carries three longhands
// now, and a url in it is the image rather than an unsupported part.
func TestTheListStyleShorthandSetsTheImage(t *testing.T) {
	root, findings := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style: square outside url(blue15x15.png) }`)
	for _, f := range findings {
		t.Errorf("the shorthand reported %s", f.Error())
	}
	if m := markerOf(t, root, "i"); m.Image == nil {
		t.Error("a url in the list-style shorthand did not become the image")
	}
}

// TestAnInsideImageMarkerSaysItWasNotDrawn.
//
// An inside marker is a box on the line, and a line item in this engine carries
// text rather than a picture. Falling back to the type's marker is what §12.6.2
// gives for an image that did not load, so the page is a legitimate rendering —
// but it is not the one asked for, and the difference is what a finding is for.
func TestAnInsideImageMarkerSaysItWasNotDrawn(t *testing.T) {
	_, findings := markerLayout(t,
		`<ul><li id="i">one</li></ul>`,
		`#i { list-style-image: url(blue15x15.png); list-style-position: inside }`)
	var said bool
	for _, f := range findings {
		if f.Rule == RuleUnsupportedValue && f.Property == "list-style-image" {
			said = true
		}
	}
	if !said {
		t.Errorf("an inside image marker was dropped without a word: %v", findings)
	}
}
