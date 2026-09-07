package layout

import (
	"strings"
	"sync"
	"testing"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Laying two documents out at once.
//
// Build and Layout are the whole of this package's public surface, and nothing
// says whether calling them from two goroutines is allowed. Anything holding a
// server open — a report generator, a print service — will do it on the second
// day, and the answer needs to be a property rather than a habit: a layouter is
// made per call and owns everything it memoizes, so two of them share nothing.
//
// Which is a claim about a good deal of state. The layouter carries eight maps
// and a breaker, the box tree hangs off a document each call parses for itself,
// and the font set is handed in. Any one of those turning out to be shared would
// be a data race in a library whose callers have no reason to expect one, and it
// would show up as a torn page rather than as a crash.

// pageOf lays a document out and returns the text of every line, without
// touching *testing.T — reporting from a goroutine is what testing forbids, so
// the checking is all done by the caller.
func pageOf(htmlSrc, cssSrc string, width float64) []string {
	in := Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}}
	got := Build(in)
	if got.Root == nil {
		return nil
	}
	w, _ := style.FromPx(width)
	h, _ := style.FromPx(10000)
	frag := Layout(got.Root, Size{W: w, H: h}, nil, NewRecorder(nil))
	if frag == nil {
		return nil
	}
	var out []string
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		if f == nil {
			return
		}
		for _, line := range f.Lines {
			out = append(out, strings.Join(runTexts(line), ""))
		}
		for _, kid := range f.Children {
			walk(kid)
		}
	}
	walk(frag)
	return out
}

// raceDocuments are laid out together. They are different documents rather than
// one repeated, because a shared memo keyed on something too loose only shows
// itself when two *different* things ask it the same question — two copies of one
// page would agree even if they were sharing everything.
var raceDocuments = []struct {
	name, html, css string
	width           float64
}{
	{
		"plain prose",
		`<div id="p">the quick brown fox jumps over the lazy dog</div>`,
		noDefaults + `#p { font-family: Courier; font-size: 20px; width: 150px }`,
		600,
	},
	{
		"a different face and size",
		`<div id="p">the quick brown fox jumps over the lazy dog</div>`,
		noDefaults + `#p { font-family: Times; font-size: 13px; width: 150px }`,
		600,
	},
	{
		"letter-spacing, which is part of the measurement key",
		`<div id="p">the quick brown fox jumps over the lazy dog</div>`,
		noDefaults + `#p { font-family: Courier; font-size: 20px; width: 150px;
			letter-spacing: 3px }`,
		600,
	},
	{
		"a float for the lines to run around",
		`<div id="p"><span id="f">xx</span>the quick brown fox jumps over the lazy dog</div>`,
		noDefaults + `#p { font-family: Courier; font-size: 20px; width: 200px }
			#f { float: left; width: 60px; height: 40px }`,
		600,
	},
	{
		"a balanced paragraph, which lays itself out many times over",
		`<div id="p">the quick brown fox jumps over the lazy dog</div>`,
		noDefaults + `#p { font-family: Courier; font-size: 20px; width: 300px;
			text-wrap-style: balance }`,
		600,
	},
	{
		"right-to-left text",
		`<div id="p" dir="rtl">אב גד הו זח טי כל מנ סע פצ</div>`,
		noDefaults + `#p { font-family: Courier; font-size: 20px; width: 120px }`,
		600,
	},
}

// TestDocumentsLaidOutInParallelAgreeWithOnesLaidOutAlone is the property, and
// -race is where it earns its keep.
//
// Every document is laid out by itself first, then all of them together many
// times over, and every page must come out the same. Without the detector this
// still catches a memo shared between layouts — the pages would disagree — and
// with it, it catches one reached from two goroutines that happened to agree
// anyway, which is the ordinary way a race presents before it stops being
// ordinary.
func TestDocumentsLaidOutInParallelAgreeWithOnesLaidOutAlone(t *testing.T) {
	want := make([][]string, len(raceDocuments))
	for i, d := range raceDocuments {
		want[i] = pageOf(d.html, d.css, d.width)
		if len(want[i]) == 0 {
			t.Fatalf("%s produced no lines laid out alone; the fixture says nothing", d.name)
		}
	}

	const rounds = 4
	got := make([][][]string, len(raceDocuments)*rounds)
	var wg sync.WaitGroup
	for r := range rounds {
		for i, d := range raceDocuments {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got[r*len(raceDocuments)+i] = [][]string{pageOf(d.html, d.css, d.width)}
			}()
		}
	}
	wg.Wait()

	for k, out := range got {
		i := k % len(raceDocuments)
		d := raceDocuments[i]
		if len(out) != 1 {
			t.Fatalf("%s: no page came back", d.name)
		}
		page := out[0]
		if len(page) != len(want[i]) {
			t.Errorf("%s laid out beside the others has %d lines and alone has %d:\n"+
				"  alone:    %q\n  parallel: %q",
				d.name, len(page), len(want[i]), want[i], page)
			continue
		}
		for n := range page {
			if page[n] != want[i][n] {
				t.Errorf("%s laid out beside the others: line %d reads %q and alone it "+
					"reads %q — two layouts are sharing something",
					d.name, n, page[n], want[i][n])
			}
		}
	}
}

// TestOneFontSetServesManyLayoutsAtOnce is the one thing two layouts really do
// share, and so the one worth asking about separately.
//
// A FontSet is handed in by the caller and outlives any one layout — StandardFonts
// loads a face the first time it is asked for and keeps it — so it is reached
// from every goroutine at once by design rather than by accident. Everything else
// in a layout is made per call; this is not.
func TestOneFontSetServesManyLayoutsAtOnce(t *testing.T) {
	set := StandardFonts()
	// Ask for every family the set knows, from every goroutine, starting cold.
	families := []string{"serif", "sans-serif", "monospace", "Times", "Helvetica",
		"Courier", "Arial", "Georgia", "Verdana", "Consolas"}

	const goroutines = 8
	faces := make([][]bool, goroutines)
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out := make([]bool, 0, len(families)*4)
			for _, bold := range []bool{false, true} {
				for _, italic := range []bool{false, true} {
					for _, f := range families {
						_, ok := set.Face(f, bold, italic)
						out = append(out, ok)
					}
				}
			}
			faces[g] = out
		}()
	}
	wg.Wait()

	for g := 1; g < goroutines; g++ {
		if len(faces[g]) != len(faces[0]) {
			t.Fatalf("goroutine %d asked for %d faces and goroutine 0 for %d",
				g, len(faces[g]), len(faces[0]))
		}
		for i := range faces[g] {
			if faces[g][i] != faces[0][i] {
				t.Errorf("goroutine %d and goroutine 0 disagree about whether face %d "+
					"exists — the set is answering differently depending on who asks",
					g, i)
			}
		}
	}
}

// TestOneFontSetSetsTextFromManyLayoutsAtOnce is the assertion the test above
// leaves out, and the one that mattered.
//
// TestOneFontSetServesManyLayoutsAtOnce asks a set for faces from many
// goroutines, which is not what a layout does with a set: it takes a face and
// then *sets text in it*. A face records the glyphs it was asked for, because
// that is what a subset is computed from, and it records them while measuring —
// so the first word of the first paragraph writes to the face, and two
// documents sharing a set write to the same map. It reported a data race on the
// first document that had any text in it, which is every document.
//
// Documents rather than bare Face calls, and different documents rather than
// one repeated, for the reason raceDocuments gives.
func TestOneFontSetSetsTextFromManyLayoutsAtOnce(t *testing.T) {
	set := StandardFonts()
	alone := make([][]string, len(raceDocuments))
	for i, d := range raceDocuments {
		alone[i] = drawnTextOf(d.html, d.css, set)
		if len(alone[i]) == 0 {
			t.Fatalf("%s drew no text on its own; the fixture says nothing", d.name)
		}
	}

	const rounds = 4
	got := make([][]string, len(raceDocuments)*rounds)
	var wg sync.WaitGroup
	for r := range rounds {
		for i, d := range raceDocuments {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got[r*len(raceDocuments)+i] = drawnTextOf(d.html, d.css, set)
			}()
		}
	}
	wg.Wait()

	for k, out := range got {
		i := k % len(raceDocuments)
		if strings.Join(out, "\x00") != strings.Join(alone[i], "\x00") {
			t.Errorf("%s composed beside the others drew %q and alone drew %q — "+
				"two documents are sharing a face", raceDocuments[i].name, out, alone[i])
		}
	}
}

// drawnTextOf composes a document through a given set and returns the text of
// every run drawn, which is the whole of what the shaping produced.
func drawnTextOf(htmlSrc, cssSrc string, set FontSet) []string {
	out := []string{}
	for _, op := range Compose(Input{
		HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}, Fonts: set,
	}, Options{MinScale: 0.01}).Ops {
		if t, ok := op.(DrawText); ok {
			out = append(out, t.Text)
		}
	}
	return out
}

// TestAFaceRecordsOneDocumentsGlyphs is the other half of the same defect, and
// the half no detector would have found.
//
// The record of which glyphs were set is what a subset is built from and what
// /CIDSet is written from. Kept on a face the caller's library owns, it
// accumulated over every document the process had ever set — so a document
// asking for "a" embedded a font carrying the "z" of the document before it,
// and the set describing its glyphs described neither document's.
func TestAFaceRecordsOneDocumentsGlyphs(t *testing.T) {
	set := StandardFonts()
	library, ok := set.Face("serif", false, false)
	if !ok {
		t.Fatal("the standard set has no serif face")
	}
	if n := len(library.Used()); n != 0 {
		t.Fatalf("the library's face already records %d glyphs", n)
	}

	faceOf := func(src string) *shape.Face {
		t.Helper()
		for _, op := range Compose(Input{HTML: src, Fonts: set}, Options{}).Ops {
			if d, ok := op.(DrawText); ok && d.Face != nil {
				return d.Face
			}
		}
		t.Fatalf("%q drew no text", src)
		return nil
	}
	first := faceOf(`<p style="font-family:serif">aaa</p>`)
	second := faceOf(`<p style="font-family:serif">zzz</p>`)

	if first == second {
		t.Error("two documents were set in the same face, so each holds the other's glyphs")
	}
	if first == library || second == library {
		t.Error("a document was set in the caller's own face, so the library accumulates " +
			"every document the process ever set")
	}
	if n := len(library.Used()); n != 0 {
		t.Errorf("the caller's face records %d glyphs after two documents were composed", n)
	}

	glyphOf := func(f *shape.Face, r rune) int {
		t.Helper()
		gid, ok := f.GlyphID(r)
		if !ok {
			t.Fatalf("the serif face has no glyph for %q", r)
		}
		return gid
	}
	has := func(f *shape.Face, gid int) bool {
		for _, g := range f.Used() {
			if g == gid {
				return true
			}
		}
		return false
	}
	if !has(first, glyphOf(first, 'a')) {
		t.Error("the first document's face does not record the letter it set")
	}
	if has(first, glyphOf(first, 'z')) {
		t.Error("the first document's face records a letter only the second document set")
	}
	if has(second, glyphOf(second, 'a')) {
		t.Error("the second document's face records a letter only the first document set")
	}
}
