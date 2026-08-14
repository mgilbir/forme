package layout

import (
	"strings"
	"sync"
	"testing"

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
