package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A <video> is a replaced element, and the two halves of that are apart: the box
// is on the page whether or not anything could be played in it, and what is
// refused is reported only where the document asked for it.
//
// It was refused outright under "a page laid out once cannot play anything",
// which is true of playing and says nothing about layout — the same confusion
// the form controls and <iframe> were moved out of droppedElements for. HTML
// §4.8.9 gives the element the poster's intrinsic dimensions where there is one
// and the default object size where there is not.

// TestAVideoIsAReplacedBoxOfTheDefaultSize is CSS 2.1 §10.3.2's last row: no
// intrinsic width, no intrinsic height, no ratio.
func TestAVideoIsAReplacedBoxOfTheDefaultSize(t *testing.T) {
	root := replacedLayout(t, 500, `<div><video id="v"></video></div>`, noDefaults)
	w, h := contentSize(find(t, root, "v"))
	px(t, "the used width", w, 300)
	px(t, "the used height", h, 150)
}

// TestAVideoTakesItsDeclaredSize: the default is a fallback and not a fixed
// size, which is what the suite's video-paint-order needs — it draws a 95 by 95
// video and covers it with a 100 by 100 block.
func TestAVideoTakesItsDeclaredSize(t *testing.T) {
	root := replacedLayout(t, 500, `<div><video id="v"></video></div>`, noDefaults,
		`#v { width: 95px; height: 95px }`)
	w, h := contentSize(find(t, root, "v"))
	px(t, "the used width", w, 95)
	px(t, "the used height", h, 95)
}

// TestAnEmptyVideoReportsNothing.
//
// A video that names no media, has no poster and asks for no controls has
// nothing missing from it. Saying otherwise is not a harmless extra: a finding
// means a reader cannot trust the page, and it is what held video-paint-order
// out of the clean count while the engine drew exactly the right picture.
func TestAnEmptyVideoReportsNothing(t *testing.T) {
	built := Build(Input{HTML: `<div><video></video></div>`})
	for _, f := range built.Findings {
		t.Errorf("a video naming nothing reported %s", f.Error())
	}
	root := replacedLayout(t, 500, `<div><video></video></div>`, noDefaults)
	rec := NewRecorder(nil)
	_ = Paint(root)
	for _, f := range rec.Findings() {
		t.Errorf("laying one out reported %s", f.Error())
	}
}

// TestAVideoNamingMediaReportsIt is the half that is a real limitation, and it
// is a limitation of the medium rather than of this engine: a page laid out once
// has no time in it.
func TestAVideoNamingMediaReportsIt(t *testing.T) {
	for _, markup := range []string{
		`<video src="film.mp4"></video>`,
		`<video><source src="film.mp4"/></video>`,
	} {
		built := Build(Input{HTML: `<div>` + markup + `</div>`})
		var blocked bool
		for _, f := range built.Findings {
			if f.Rule == RuleResourceBlocked {
				blocked = true
			}
		}
		if !blocked {
			t.Errorf("%s reported no blocked resource: %v", markup, built.Findings)
		}
	}
}

// TestAVideoWithControlsSaysTheBarIsNotDrawn. The box is drawn and the player in
// it is not, which is what RuleControlApproximated is for — and it must fire,
// because a reftest comparing what paints over a control bar compares nothing
// when neither document drew one.
func TestAVideoWithControlsSaysTheBarIsNotDrawn(t *testing.T) {
	built := Build(Input{HTML: `<div><video controls></video></div>`})
	var said bool
	for _, f := range built.Findings {
		if f.Rule == RuleControlApproximated {
			said = true
		}
	}
	if !said {
		t.Errorf("a video asking for controls reported %v", built.Findings)
	}
}

// TestAVideoPosterIsTheContent. The still a browser shows before anything plays
// is a picture this engine can draw, so it is drawn — and it is the element's
// intrinsic size, which is what HTML §4.8.9 says while there is no video.
func TestAVideoPosterIsTheContent(t *testing.T) {
	res := mapResolver{"still.png": encodePNG(t, 40, 20)}
	ops := paintWith(t, res, `<div><video id="v" poster="still.png"></video></div>`, noDefaults)
	var drawn []Rect
	for _, op := range ops {
		if d, ok := op.(DrawImage); ok {
			drawn = append(drawn, d.Rect)
		}
	}
	if len(drawn) != 1 {
		t.Fatalf("%d pictures drawn for a video with a poster, want one", len(drawn))
	}
	w, _ := style.FromPx(40)
	h, _ := style.FromPx(20)
	if drawn[0].W != w || drawn[0].H != h {
		t.Errorf("the poster is %v by %v, want the picture's own 40 by 20",
			drawn[0].W, drawn[0].H)
	}
}

// TestAVideoDropsItsFallback. The children of a video element are what a user
// agent that cannot play video would show instead, and this one draws the
// element rather than replacing it — the same rule a canvas's fallback follows.
//
// Asked of the box tree and not of the page, because the page cannot tell: a
// replaced box draws its content and never descends into its children, so
// fallback left in the tree is invisible and still costs every box in it. That
// is what "dropped rather than hidden" means, and a hidden box is still a box.
//
// See TestAReplacedElementLaysOutNoFallback for the block case, which is the
// same rule and needs the children left out one step earlier.
func TestAVideoDropsItsFallback(t *testing.T) {
	built := Build(Input{HTML: `<div><video id="v"><span>no video here</span></video></div>`})
	var found *Box
	var walk func(*Box)
	walk = func(b *Box) {
		if b == nil || found != nil {
			return
		}
		if b.Element != nil && strings.EqualFold(b.Element.Name, "video") {
			found = b
			return
		}
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(built.Root)
	if found == nil {
		t.Fatal("no box for the video at all")
	}
	if len(found.Children) != 0 {
		t.Errorf("a video kept %d boxes of fallback content", len(found.Children))
	}
	// And the control: the same markup outside a video does make boxes, so the
	// assertion is about the element and not about the fixture.
	plain := Build(Input{HTML: `<div id="d"><span>no video here</span></div>`})
	var kids int
	var walk2 func(*Box)
	walk2 = func(b *Box) {
		if b == nil {
			return
		}
		if b.Element != nil && strings.EqualFold(b.Element.Name, "span") {
			kids++
		}
		for _, c := range b.Children {
			walk2(c)
		}
	}
	walk2(plain.Root)
	if kids == 0 {
		t.Fatal("the same content outside a video made no boxes either")
	}
}
