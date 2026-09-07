package layout

import (
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// notoSet answers every family with the embedded Noto Sans, which is a face
// whose context can change what it draws — it has kerning, ligatures and
// joining forms — and so is a face the merge group is built for. The base-14
// faces have none of the three, and a run set in one is waved past this whole
// file by the gate at the top of it.
type notoSet struct{ face *shape.Face }

func (s notoSet) Face(string, bool, bool) (*shape.Face, bool) { return s.face, true }

// linkedItemsCost lays a document out in Noto Sans and returns how long it took
// and how many bytes it allocated.
func linkedItemsCost(t *testing.T, htmlSrc, cssSrc string) (time.Duration, uint64) {
	t.Helper()
	set := notoSet{face: embeddedFallback(t)}
	built := Build(Input{HTML: htmlSrc, CSS: []Stylesheet{{Source: cssSrc}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(100000)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	if Layout(built.Root, Size{W: w, H: h}, set, NewRecorder(nil)) == nil {
		t.Fatal("the document produced no fragments")
	}
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	return elapsed, after.TotalAlloc - before.TotalAlloc
}

// TestManyAdjacentSpansAreShapedOnce is the cost of the merge group, which was
// paid once per run of it rather than once for the group.
//
// Every run gathered its whole group from itself outwards, rebuilding the
// group's text from scratch and concatenating it a neighbour at a time. That is
// quadratic in the runs and worse in their bytes: the audit measured four
// thousand adjacent spans at twenty-nine seconds and about 1.6 GB of strings.
// Four thousand adjacent spans is one line of generated markup.
//
// The group's text is built once now and every run's context is a slice of it,
// so a group of a thousand runs costs one copy of its text.
func TestManyAdjacentSpansAreShapedOnce(t *testing.T) {
	var html strings.Builder
	html.WriteString(`<p id="p">`)
	for i := 0; i < 2000; i++ {
		html.WriteString(`<span>a</span>`)
	}
	html.WriteString(`</p>`)
	css := noDefaults + `#p { font-size: 10px; width: 400px }`

	elapsed, allocated := linkedItemsCost(t, html.String(), css)
	if elapsed > 5*time.Second {
		t.Errorf("two thousand adjacent runs took %v; each of them used to shape "+
			"the whole group and now the group is shaped once", elapsed)
	}
	// And a bound on memory, because the group's glyphs were kept per run as
	// well as shaped per run. Two thousand runs allocated 1.2 GB; shaping the
	// group once is 34 MB, most of it the document itself.
	const cap = 64 << 20
	if allocated > cap {
		t.Errorf("two thousand adjacent runs allocated %d bytes; the group is "+
			"gathered once and shaped once, so both of those are linear", allocated)
	}
}

// TestSpansSeparatedBySpacesCostTheSame is the shape that was always fast,
// because a space ends the group. It is here so that a change which made every
// group of one expensive would show.
func TestSpansSeparatedBySpacesCostTheSame(t *testing.T) {
	var html strings.Builder
	html.WriteString(`<p id="p">`)
	for i := 0; i < 2000; i++ {
		html.WriteString(`<span>a</span> `)
	}
	html.WriteString(`</p>`)
	css := noDefaults + `#p { font-size: 10px; width: 400px }`

	if elapsed, _ := linkedItemsCost(t, html.String(), css); elapsed > 20*time.Second {
		t.Errorf("two thousand separated runs took %v", elapsed)
	}
}

// TestAWordSplitAcrossSpansIsShapedAsOneString is what the group is for, kept.
//
// Every run of a group has to shape the same string, because two runs of one
// word that shape different strings disagree about where a ligature begins and
// a character between them is drawn by neither. The group's text is a slice of
// one string now, so the thing to say is that every run of the group still sees
// the whole of it.
func TestAWordSplitAcrossSpansIsShapedAsOneString(t *testing.T) {
	built := Build(Input{HTML: `<p id="p">of<span>f</span>ice</p>`,
		CSS: []Stylesheet{{Source: noDefaults + `#p { font-size: 10px }`}}})
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(10000)
	frag := Layout(built.Root, Size{W: w, H: h}, notoSet{face: embeddedFallback(t)},
		NewRecorder(nil))
	var runs []TextRun
	var walk func(*Fragment)
	walk = func(f *Fragment) {
		for _, line := range f.Lines {
			runs = append(runs, line.Runs...)
		}
		for _, c := range f.Children {
			walk(c)
		}
	}
	walk(frag)
	if len(runs) < 3 {
		t.Fatalf("the word came out as %d runs, want three", len(runs))
	}
	for _, r := range runs {
		if r.MergePre == "" && r.MergePost == "" {
			continue // a run outside any group
		}
		if whole := r.MergePre + r.Text + r.MergePost; whole != "office" {
			t.Errorf("the run %q is shaped with %q, want the whole group \"office\"",
				r.Text, whole)
		}
	}
}
