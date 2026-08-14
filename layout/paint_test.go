package layout

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Painting: what a laid-out page turns into as a display list, and the page-fit
// checks around it. These came over with the layout engine — they were written
// beside the PDF backend and are about neither PDF nor any other backend.

// firstMatrix returns the operands of the first "cm" in a content stream.
// paintOf lays a document out on the default sheet and paints it, which is what
// almost every test about a mark on the page starts with.

// firstMatrix returns the operands of the first "cm" in a content stream.
// paintOf lays a document out on the default sheet and paints it, which is what
// almost every test about a mark on the page starts with.
func paintOf(t *testing.T, htmlSrc, cssSrc string) []Op {
	t.Helper()
	root := layoutOf(t, A4.Content().W.Px(), htmlSrc, cssSrc)
	return Paint(root)
}

// TestTheTransformIsWrittenOnce pins the one "cm" this stage exists to emit, and
// every conversion folded into it.
//
// Nothing above pdfout has ever seen PDF's coordinate system, so this matrix is
// the only place the flip happens — and it was entirely untested until a planted
// defect showed that inverting it, dropping the unit conversion, dropping the
// scale and dropping the page margin all left every other test passing.
// TestBackgroundCoversTheBorderBoxNotTheMargin pins where a background stops.
//
// It runs *under* the border and stops at the border box, which is
// background-clip's initial value and is why a dashed border shows the
// background through its gaps rather than the page. It never reaches the margin,
// which is the space meant to show the page through.
//
// This test asserted the padding box until background-clip was implemented, and
// the engine agreed with it. Both were wrong: CSS 2.1 §14.2 says the background
// covers "the content, padding and border areas", and the two only look alike
// while every border is opaque and solid.
func TestBackgroundCoversTheBorderBoxNotTheMargin(t *testing.T) {
	find := func(ops []Op) *FillRect {
		for i := range ops {
			if r, ok := ops[i].(FillRect); ok && r.Color.R == 255 {
				c := r
				return &c
			}
		}
		return nil
	}
	const box = `#a { background-color: #ff0000; height: 50px; margin: 20px;
			border-top-style: solid; border-top-width: 5px }`

	bg := find(paintOf(t, `<div id="a"></div>`, noDefaults+box))
	if bg == nil {
		t.Fatal("the background did not paint")
	}
	// The margin puts the border box at 20, and that is where the background
	// starts: the 5px border is painted on top of it.
	want, _ := style.FromPx(20)
	if bg.Rect.Y != want {
		t.Errorf("the background starts at y=%v, want 20 — the border box, after "+
			"20px of margin", bg.Rect.Y.Px())
	}
	if bg.Rect.X < want {
		t.Errorf("the background starts at x=%v, inside the 20px margin", bg.Rect.X.Px())
	}

	// And background-clip moves it in, which is the property's whole purpose.
	clipped := find(paintOf(t, `<div id="a"></div>`,
		noDefaults+box+` #a { background-clip: padding-box }`))
	if clipped == nil {
		t.Fatal("the clipped background did not paint")
	}
	want, _ = style.FromPx(25)
	if clipped.Rect.Y != want {
		t.Errorf("with background-clip: padding-box the background starts at y=%v, want 25",
			clipped.Rect.Y.Px())
	}
}

// TestTextPaintsAtItsBaseline pins that a text op carries the baseline rather
// than the top of the line box, which is what a text backend takes.
func TestTextPaintsAtItsBaseline(t *testing.T) {
	ops := paintOf(t, `<p id="p">text</p>`,
		noDefaults+`p { font-size: 100px; font-family: Helvetica; line-height: 200px }`)

	var text *DrawText
	for i := range ops {
		if d, ok := ops[i].(DrawText); ok {
			c := d
			text = &c
			break
		}
	}
	if text == nil {
		t.Fatal("no text painted")
	}
	if text.Text != "text" {
		t.Errorf("the run reads %q", text.Text)
	}
	// The baseline is inside the line box and below its top, which a value of
	// zero or of the full line height would not be.
	if text.At.Y <= 0 {
		t.Errorf("the baseline is at y=%v", text.At.Y.Px())
	}
	if text.At.Y.Px() >= 200 {
		t.Errorf("the baseline at %v is below the 200px line box", text.At.Y.Px())
	}
}

// TestSpacesArePainted pins that the gap between two words is drawn rather than
// skipped, and the reason is text extraction rather than ink.
//
// A space marks no paper, so skipping it looks free. But the words either side
// then become separate text operations with only a position jump between them,
// and a reader copying the text gets them run together. This was written the
// other way round first, and the end-to-end test caught it: "A heading" came
// back from the finished PDF as "Aheading".
func TestSpacesArePainted(t *testing.T) {
	ops := paintOf(t, `<p>one two three</p>`,
		noDefaults+`p { font-size: 20px; font-family: Helvetica }`)

	var spaces int
	for _, op := range ops {
		if d, ok := op.(DrawText); ok && strings.TrimSpace(d.Text) == "" {
			spaces++
		}
	}
	if spaces != 2 {
		t.Errorf("%d spaces were painted, want the 2 between the three words", spaces)
	}
}

// TestScaleToFit pins §5: one factor, computed from the natural size, applied to
// everything. It is not re-layout — the line breaks do not move — which is what
// makes the threshold checks exact.
func ftoa(v float64) string {
	n := int(v)
	return itoa(n)
}

// TestScalingUpIsOffByDefault pins that an underfull page is left alone. Growing
// it is surprising and it degrades images, so it is opt-in.
// sketchOps renders a display list as text, so a difference names itself.
func sketchOps(ops []Op) string {
	var b strings.Builder
	for _, op := range ops {
		switch v := op.(type) {
		case FillRect:
			b.WriteString("fill " + v.Rect.String() + " " + v.Color.String() + "\n")
		case DrawText:
			b.WriteString("text " + strconv.Quote(v.Text) + " at " +
				strconv.FormatFloat(v.At.X.Px(), 'f', 2, 64) + "," +
				strconv.FormatFloat(v.At.Y.Px(), 'f', 2, 64) + " " +
				v.Face.Name() + " " + strconv.FormatFloat(v.Size.Px(), 'f', 2, 64) +
				" " + v.Color.String() + "\n")
		}
	}
	return b.String()
}

// TestBorderStylesDiffer pins that each border-style paints something different.
//
// Layout only ever asks a border how wide it is, and every style is the same
// width — so a renderer that ignored the style produced a page that was wrong in
// a way an author sees at once and a test suite sees as a hundred failures.
func TestBorderStylesDiffer(t *testing.T) {
	// All four edges, because the 3-D styles differ from solid only in which
	// edges are lit: an "outset" top edge *is* the plain colour, so a test with
	// a top border alone would report outset and solid as the same thing — and
	// be right about it.
	sheet := func(kind string) string {
		return noDefaults + `#a { height: 50px;
			border-top-width: 9px; border-right-width: 9px;
			border-bottom-width: 9px; border-left-width: 9px;
			border-top-color: #808080; border-right-color: #808080;
			border-bottom-color: #808080; border-left-color: #808080;
			border-top-style: ` + kind + `; border-right-style: ` + kind + `;
			border-bottom-style: ` + kind + `; border-left-style: ` + kind + ` }`
	}
	seen := map[string]string{}
	for _, kind := range []string{
		"solid", "double", "dashed", "dotted", "groove", "ridge", "inset", "outset",
	} {
		ops := paintOf(t, `<div id="a"></div>`, sheet(kind))
		got := sketchOps(ops)
		if got == "" {
			t.Errorf("border-style:%s painted nothing", kind)
			continue
		}
		if other, ok := seen[got]; ok {
			t.Errorf("border-style:%s paints exactly what %s does", kind, other)
		}
		seen[got] = kind
	}

	// "none" and "hidden" paint nothing at all, which is the contrast that makes
	// the assertions above about style rather than about painting in general.
	for _, kind := range []string{"none", "hidden"} {
		ops := paintOf(t, `<div id="a"></div>`, sheet(kind))
		for _, op := range ops {
			if r, ok := op.(FillRect); ok && r.Color.R == 128 {
				t.Errorf("border-style:%s painted a border", kind)
			}
		}
	}
}

// TestDoubleBorderIsTwoLines pins the style whose whole point is the gap. One
// band would be a solid border by another name.
func TestDoubleBorderIsTwoLines(t *testing.T) {
	ops := paintOf(t, `<div id="a"></div>`,
		noDefaults+`#a { height: 50px; border-top-width: 9px;
			border-top-color: #808080; border-top-style: double }`)

	var bands []Rect
	for _, op := range ops {
		if r, ok := op.(FillRect); ok && r.Color.R == 128 {
			bands = append(bands, r.Rect)
		}
	}
	if len(bands) != 2 {
		t.Fatalf("a double border painted %d bands, want 2", len(bands))
	}
	// Each is a third of the width, and there is a third between them.
	px(t, "the first band", bands[0].H, 3)
	px(t, "the second band", bands[1].H, 3)
	px(t, "the gap", bands[1].Y.Sub(bands[0].Bottom()), 3)
}

// TestDashedAndDottedAreRuns pins that these paint many marks rather than one,
// and that a dot is shorter than a dash — the ratio is left open by the
// specification and the difference is not.
func TestDashedAndDottedAreRuns(t *testing.T) {
	count := func(kind string) (marks int, markLen float64) {
		ops := paintOf(t, `<div id="a"></div>`,
			noDefaults+`#a { height: 50px; border-top-width: 4px;
				border-top-color: #808080; border-top-style: `+kind+` }`)
		for _, op := range ops {
			if r, ok := op.(FillRect); ok && r.Color.R == 128 {
				marks++
				markLen = r.Rect.W.Px()
			}
		}
		return
	}
	dashes, dashLen := count("dashed")
	dots, dotLen := count("dotted")

	if dashes < 5 {
		t.Errorf("a dashed border painted %d marks, want a run of them", dashes)
	}
	if dots <= dashes {
		t.Errorf("dotted painted %d marks and dashed %d; a dot is shorter so there "+
			"are more of them", dots, dashes)
	}
	if dotLen >= dashLen {
		t.Errorf("a dot is %v wide and a dash %v; a dot is the shorter", dotLen, dashLen)
	}
}

// TestThreeDBordersUseTwoTones pins that groove, ridge, inset and outset light
// some edges and shadow others — which is the whole of what makes them look
// three-dimensional, and what a single-tone renderer loses.
func TestThreeDBordersUseTwoTones(t *testing.T) {
	for _, kind := range []string{"groove", "ridge", "inset", "outset"} {
		ops := paintOf(t, `<div id="a"></div>`,
			noDefaults+`#a { height: 50px;
				border-top-width: 8px; border-right-width: 8px;
				border-bottom-width: 8px; border-left-width: 8px;
				border-top-color: #808080; border-right-color: #808080;
				border-bottom-color: #808080; border-left-color: #808080;
				border-top-style: `+kind+`; border-right-style: `+kind+`;
				border-bottom-style: `+kind+`; border-left-style: `+kind+` }`)

		tones := map[float64]bool{}
		for _, op := range ops {
			if r, ok := op.(FillRect); ok {
				tones[r.Color.R] = true
			}
		}
		if len(tones) < 2 {
			t.Errorf("border-style:%s used %d tone(s), want two", kind, len(tones))
		}
	}
}

// TestBlackThreeDBorderStaysVisible pins the case a naive darkening loses. Half
// of black is black, so a groove on the colour authors use most would vanish
// into one tone; the second tone is a lightening instead.
func TestBlackThreeDBorderStaysVisible(t *testing.T) {
	black := style.RGBA{A: 1}
	if shade(black, 0.5) == black {
		t.Error("a black border's second tone is also black, so the style disappears")
	}
	// An ordinary colour does darken, so the special case is only for black.
	grey := style.RGBA{R: 200, G: 200, B: 200, A: 1}
	if got := shade(grey, 0.5); got.R >= grey.R {
		t.Errorf("shading grey gave %v, which is no darker", got)
	}
}

// composeOf lays a document out on a sheet and paints it, which is every step
// the guardrails below are about. It replaces a helper that drove the same
// steps through the PDF backend, back when they lived there.
func composeOf(t *testing.T, htmlSrc string, opts Options, cssSrc ...string) Composed {
	t.Helper()
	in := Input{HTML: htmlSrc}
	for _, c := range cssSrc {
		in.CSS = append(in.CSS, Stylesheet{Source: c})
	}
	return Compose(in, opts)
}

// TestBackgroundCoversTheBorderBoxNotTheMargin pins where a background stops.
//
// It runs *under* the border and stops at the border box, which is
// background-clip's initial value and is why a dashed border shows the
// background through its gaps rather than the page. It never reaches the margin,
// which is the space meant to show the page through.
//
// This test asserted the padding box until background-clip was implemented, and
// the engine agreed with it. Both were wrong: CSS 2.1 §14.2 says the background
// covers "the content, padding and border areas", and the two only look alike
// while every border is opaque and solid.
// TestScaleToFit pins §5: one factor, computed from the natural size, applied to
// everything. It is not re-layout — the line breaks do not move — which is what
// makes the threshold checks exact.
func TestScaleToFit(t *testing.T) {
	// Content that fits needs no scaling.
	got := composeOf(t, `<div id="a"></div>`, Options{Page: A4},
		noDefaults+"#a { height: 100px }")
	if got.Scale != 1 {
		t.Errorf("content that fits was scaled by %v", got.Scale)
	}

	// Content that is smaller in *both* axes — and so could be grown — is still
	// left alone. Using a full-width box here would prove nothing, since an auto
	// width already fills the page and no such document can grow.
	got = composeOf(t, `<div id="a"></div>`, Options{Page: A4},
		noDefaults+"html, body { width: 50px } #a { height: 10px }")
	if got.Scale != 1 {
		t.Errorf("content that could have been grown was grown by %v without being asked",
			got.Scale)
	}

	// Content twice as tall as the page is scaled to about half.
	avail := A4.Content()
	tall := avail.H.Px() * 2
	got = composeOf(t, `<div id="a"></div>`, Options{Page: A4, MinScale: 0.1},
		noDefaults+"#a { height: "+ftoa(tall)+"px }")
	if got.Scale >= 1 {
		t.Fatalf("content twice the page height was not scaled: %v", got.Scale)
	}
	if math.Abs(got.Scale-0.5) > 0.02 {
		t.Errorf("the scale is %v, want about 0.5", got.Scale)
	}
	// The natural size is reported unscaled, which is what a caller adjusting a
	// template needs.
	if math.Abs(got.NaturalSize.H.Px()-tall) > 1 {
		t.Errorf("the natural height is %v, want %v", got.NaturalSize.H.Px(), tall)
	}
}

// TestScalingUpIsOffByDefault pins that an underfull page is left alone. Growing
// it is surprising and it degrades images, so it is opt-in.
func TestScalingUpIsOffByDefault(t *testing.T) {
	got := composeOf(t, `<div id="a"></div>`, Options{Page: A4},
		noDefaults+"#a { height: 10px }")
	if got.Scale != 1 {
		t.Errorf("a nearly empty page was scaled by %v", got.Scale)
	}

	// Growing needs room in *both* axes. An auto width already fills the page,
	// so only content that is narrower as well as shorter can grow — which is
	// worth stating, because a test using a full-width box would report that
	// scaling up does not work when it is the content that cannot.
	got = composeOf(t, `<div id="a"></div>`, Options{Page: A4, AllowScaleUp: true},
		noDefaults+"html, body { width: 50px } #a { height: 10px }")
	if got.Scale <= 1 {
		t.Errorf("content smaller than the page in both axes was not grown: %v", got.Scale)
	}
}

// TestMinScaleIsAnError pins the blunt guardrail of §6.1, and that it stops the
// document being produced: a page that only fitted by being made illegible is
// one where no document is better than the document.
func TestMinScaleIsAnError(t *testing.T) {
	fired[RuleMinScale] = true

	avail := A4.Content()
	got := composeOf(t, `<div id="a"></div>`, Options{Page: A4},
		noDefaults+"#a { height: "+ftoa(avail.H.Px()*10)+"px }")

	var found *Finding
	for i := range got.Findings {
		if got.Findings[i].Rule == RuleMinScale {
			f := got.Findings[i]
			found = &f
		}
	}
	if found == nil {
		t.Fatalf("content ten times the page height did not trip min-scale: %v", got.Findings)
	}
	if found.Severity != Error {
		t.Errorf("min-scale was reported as %v, want an error", found.Severity)
	}
	if !got.Refused {
		t.Error("an error-severity finding did not refuse the document")
	}
	// The message says what the scale was and what the floor is, so an author
	// can decide which to change.
	if !strings.Contains(found.Message, "%") {
		t.Errorf("the message %q does not give the numbers", found.Message)
	}

	// Content that fits says nothing.
	got = composeOf(t, `<div id="a"></div>`, Options{Page: A4}, noDefaults+"#a { height: 10px }")
	for _, f := range got.Findings {
		if f.Rule == RuleMinScale {
			t.Errorf("content that fits tripped min-scale: %v", f)
		}
	}
}

// TestMinFontSizeIsAnError pins the other §6.1 threshold, and the property that
// makes it exact: because the scaling is geometric, the effective size is the
// natural size times one number, so this is a multiplication rather than an
// iteration.
func TestMinFontSizeIsAnError(t *testing.T) {
	fired[RuleMinFontSize] = true

	// 4px is 3pt, below the 6pt floor, with no scaling involved.
	got := composeOf(t, `<p>tiny</p>`, Options{Page: A4},
		noDefaults+"p { font-size: 4px; font-family: Helvetica }")

	var found *Finding
	for i := range got.Findings {
		if got.Findings[i].Rule == RuleMinFontSize {
			f := got.Findings[i]
			found = &f
		}
	}
	if found == nil {
		t.Fatalf("3pt text did not trip min-font-size: %v", got.Findings)
	}
	if found.Severity != Error {
		t.Errorf("min-font-size was reported as %v, want an error", found.Severity)
	}
	if !got.Refused {
		t.Error("an error-severity finding did not refuse the document")
	}

	// Ordinary text says nothing.
	got = composeOf(t, `<p>ordinary</p>`, Options{Page: A4},
		noDefaults+"p { font-size: 16px; font-family: Helvetica }")
	for _, f := range got.Findings {
		if f.Rule == RuleMinFontSize {
			t.Errorf("16px text tripped min-font-size: %v", f)
		}
	}
}

// TestScalingMakesTextTooSmall pins the interaction between the two thresholds,
// which is the case §6.1 is really about: text that is legible on its own
// becomes illegible once the page is shrunk to fit, and the check has to be
// against the *effective* size rather than the declared one.
func TestScalingMakesTextTooSmall(t *testing.T) {
	avail := A4.Content()
	// 10px is 7.5pt, above the floor. Scaled to a fifth it is 1.5pt, well below.
	got := composeOf(t, `<p id="p">text</p><div id="tall"></div>`,
		Options{Page: A4, MinScale: 0.01},
		noDefaults+`p { font-size: 10px; font-family: Helvetica }
		#tall { height: `+ftoa(avail.H.Px()*5)+`px }`)

	var found bool
	for _, f := range got.Findings {
		if f.Rule == RuleMinFontSize {
			found = true
			if !strings.Contains(f.Message, "before the page scaling") {
				t.Errorf("the message %q does not say the size was legible before scaling",
					f.Message)
			}
		}
	}
	if !found {
		t.Errorf("text made illegible by scaling was not reported: %v", got.Findings)
	}
}

// TestRenderIsTotal pins that no document and no options panic it.
func TestOverflowPageIsASelfCheck(t *testing.T) {
	fired[RuleOverflowPage] = true

	// An ordinary document does not trip it, which is the claim.
	got := composeOf(t, `<h1>Title</h1><p>Some text.</p><ul><li>a<li>b</ul>`,
		Options{Page: A4})
	for _, f := range got.Findings {
		if f.Rule == RuleOverflowPage {
			t.Errorf("an ordinary document reported page overflow: %v", f)
		}
	}

	// Nor does one that had to be scaled, which is the case the scale exists
	// for and the one where a wrong calculation would show.
	avail := A4.Content()
	got = composeOf(t, `<div id="a"></div>`, Options{Page: A4, MinScale: 0.05},
		noDefaults+"#a { height: "+ftoa(avail.H.Px()*4)+"px; background-color: red }")
	for _, f := range got.Findings {
		if f.Rule == RuleOverflowPage {
			t.Errorf("a scaled document reported page overflow, so the scale is "+
				"not doing its job: %v", f)
		}
	}
	if got.Scale >= 1 {
		t.Fatalf("the test document was not scaled: %v", got.Scale)
	}

	// And the check itself sees an overflow when there is one, which is what
	// makes the two assertions above worth anything.
	u := func(v float64) style.Unit { r, _ := style.FromPx(v); return r }
	rec := NewRecorder(nil)
	checkPageOverflow(rec, []Op{
		FillRect{Rect: Rect{u(0), u(0), u(10000), u(10)}, Color: style.RGBA{R: 255, A: 1}},
	}, Size{W: u(100), H: u(100)}, 1)
	if len(rec.Findings()) == 0 {
		t.Error("a box ten thousand pixels wide on a hundred-pixel page was not reported")
	}
}

func TestClippedAwayMarkDoesNotTripTheOverflowPageGuard(t *testing.T) {
	doc := `<div id="a"><div id="i"></div></div>`
	css := noDefaults + `
		#a { width: 100px; height: 50px; overflow: hidden }
		#i { background-color: #ff0000; width: 4000px; height: 4000px }`

	got := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: css}}}, Options{})
	for _, f := range got.Findings {
		if f.Rule == RuleOverflowPage {
			t.Errorf("a clipped-away box tripped the page-overflow guard: %s", f.Message)
		}
	}
	if got.Refused {
		t.Fatalf("the document was refused: %v", got.Findings)
	}
	// The control: the same box without the clip does reach off the page, so
	// the guard is one that can fire on this document.
	loose := Compose(Input{HTML: doc, CSS: []Stylesheet{{Source: noDefaults + `
		#a { width: 100px; height: 50px }
		#i { background-color: #ff0000; width: 4000px; height: 4000px }`}}}, Options{})
	var fired bool
	for _, f := range loose.Findings {
		if f.Rule == RuleOverflowPage {
			fired = true
		}
	}
	if !fired {
		t.Error("the unclipped control did not trip the page-overflow guard, so the " +
			"assertion above proves nothing")
	}
}
