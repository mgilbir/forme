package layout

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// The layout cache and the checkpoint. See speculative.go.
//
// The cache's whole claim is that it changes nothing but the cost, and the
// claim is tested the only way it can be: every document laid out with the
// cache and without it, and the two trees compared field by field. The corpus
// below is the speculative sites one by one; the suite, where it is present,
// is six thousand documents nobody wrote with the cache in mind.

// speculativeCorpus exercises every site that lays a box out more than once,
// and every side effect a reused layout has to repeat.
var speculativeCorpus = map[string]string{
	"nested columns": strings.Repeat(`<div style="display:flex;flex-direction:column"><div>`, 6) +
		"a b c" + strings.Repeat(`</div></div>`, 6),
	"nested grids": strings.Repeat(`<div style="display:grid;grid-template-columns:1fr 2fr"><div>`, 5) +
		"a b c" + strings.Repeat(`</div><div>d</div></div>`, 5),
	"stretched rows": `<div style="display:flex"><div style="height:120px"></div><div>` +
		`<div style="display:flex"><div style="height:80px"></div><div>` +
		`<div style="display:flex"><div style="height:40px"></div><div>x</div></div>` +
		`</div></div></div></div>`,
	// A percentage height inside an item of an item: the middle box is measured
	// with no height and kept at the one the column gave it, so the inner box
	// is asked the same question in two containing blocks that differ only in
	// the height a percentage resolves against.
	"percentage heights": `<div style="display:flex;flex-direction:column;height:400px">` +
		`<div style="display:flex;flex:1"><div style="height:50%;width:40px">x</div>` +
		`<div style="height:25%">y</div></div><div>z</div></div>`,
	// Out-of-flow boxes inside items, which are queued by the layout that
	// found them and must be queued again when that layout is reused; and a
	// positioned item, which is what they are placed against.
	"out of flow in items": `<div style="display:flex;flex-direction:column">` +
		`<div style="position:relative;margin-left:30px">a<div style="position:absolute;left:5px;top:7px;width:10px;height:10px"></div></div>` +
		`<div>b<span style="position:relative;left:3px">c<span style="position:absolute;left:0;top:0">d</span></span></div></div>` +
		`<div style="display:grid"><div style="position:relative">e<div style="position:absolute;right:0;bottom:0">f</div></div></div>`,
	// Floats beside text, beside a flow root, and inside items: the float
	// layout is re-asked on every attempt at a line.
	"floats": `<div style="width:300px"><div style="float:left;width:100px;height:40px"></div>` +
		`<p>one two three four five six seven eight nine ten eleven twelve</p>` +
		`<div style="overflow:hidden">beside</div>` +
		`<p>text <span style="float:right;width:50px">floated in a line</span> more text here and there</p></div>` +
		`<div style="display:flex;flex-direction:column"><div><div style="float:left;width:30px;height:30px"></div>` +
		`<p>wrapping text that goes around the float for a while</p></div></div>`,
	// A box that is not a formatting context of its own, placed at a predicted
	// position, moved by a margin that escaped a child, laid out again where it
	// settled, and holding a float the text after it runs round — all inside an
	// item that is laid out twice. Its layout reads the floats around it and
	// adds one of its own, which is what the cache must never keep.
	"settled beside floats": `<div style="display:flex;flex-direction:column"><div style="padding-top:1px">` +
		`<div style="float:left;width:100px;height:100px"></div>` +
		`<div><div style="margin-top:20px"><div style="float:left;width:50px;height:150px"></div>` +
		`text text text text</div></div><p>after after after after after after after</p>` +
		`</div></div>`,
	"inline blocks": `<p>a <span style="display:inline-block;width:50px;padding-left:10%">b c d</span> e ` +
		`<span style="display:inline-block">f<span style="display:inline-block">g</span></span></p>`,
	"tables in items": `<div style="display:flex"><table><tr><td>a</td><td style="width:40px">b c</td></tr></table>` +
		`<div>x</div></div>`,
	// Clamps, which the cache must stay out of: a line under a clamp is
	// charged to it, and a reused layout would not be.
	"nested clamps": `<div style="line-clamp:3"><div style="line-clamp:2"><div style="display:flow-root">a<br>b<br>c</div></div>` +
		`<div>d</div><div>e</div><div>f</div></div>`,
	"multicol": `<div style="column-count:2;width:400px"><div style="float:left;width:50px;height:50px"></div>` +
		`<p>` + strings.Repeat("word ", 60) + `</p></div><p>after</p>` +
		`<div style="column-count:2;width:400px;height:10px;column-fill:auto">` +
		`<div style="float:left;width:50px;height:50px"></div><p>` + strings.Repeat("word ", 60) + `</p></div><p>after</p>`,
	"balance and fit": `<p style="text-wrap:balance;width:200px">one two three <span style="float:left;width:40px;height:30px"></span>four five six seven eight nine</p>` +
		`<p style="text-fit:grow;width:300px">short line</p>`,
}

// laidTwice is a document laid out with the cache and without it.
type laidTwice struct {
	with, without          *Fragment
	withFound, noneFound   []Finding
	hits, dangling, stored int
}

func layOutTwice(t *testing.T, built Built, avail Size) laidTwice {
	t.Helper()
	var out laidTwice
	recA := NewRecorder(nil)
	a := newLayouter(built.Root, avail, built.Fonts, recA)
	out.with = a.layout()
	out.withFound = recA.Findings()
	out.hits, out.dangling = a.cache.hits, a.cache.dangling
	for _, e := range a.cache.entries {
		if e.frag != nil {
			out.stored++
		}
	}

	recB := NewRecorder(nil)
	b := newLayouter(built.Root, avail, built.Fonts, recB)
	b.noCache = true
	out.without = b.layout()
	out.noneFound = recB.Findings()
	return out
}

// compare says how the two layouts differ, or nothing.
func (d laidTwice) compare() string {
	if diff := fragmentDiff("root", d.with, d.without); diff != "" {
		return diff
	}
	if !reflect.DeepEqual(d.withFound, d.noneFound) {
		return "the findings differ:\n  with the cache:    " + findingList(d.withFound) +
			"\n  without the cache: " + findingList(d.noneFound)
	}
	return ""
}

func findingList(fs []Finding) string {
	var out []string
	for _, f := range fs {
		out = append(out, string(f.Rule)+": "+f.Message)
	}
	return strings.Join(out, "; ")
}

// TestTheLayoutCacheChangesNothing is the cache's invariant on the corpus of
// speculative sites: a reused layout is the layout, with the side effects it
// had — or the page with the cache and the page without it would differ.
func TestTheLayoutCacheChangesNothing(t *testing.T) {
	names := make([]string, 0, len(speculativeCorpus))
	for name := range speculativeCorpus {
		names = append(names, name)
	}
	sort.Strings(names)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	hits := 0
	for _, name := range names {
		built := Build(Input{HTML: speculativeCorpus[name]})
		d := layOutTwice(t, built, Size{W: w, H: h})
		if diff := d.compare(); diff != "" {
			t.Errorf("%s: the cache changed the page: %s", name, diff)
		}
		if d.dangling > 0 {
			t.Errorf("%s: %d side effects pointed outside the tree the layout that "+
				"had them made; something threw a layout away and left what it did", name, d.dangling)
		}
		hits += d.hits
	}
	// A cache the corpus never hits has been compared with itself.
	if hits == 0 {
		t.Fatal("the corpus never reused a layout; the comparison proves nothing")
	}
}

// TestTheLayoutCacheIsBounded lowers the cache's bound far enough that the
// corpus evicts, and asks for the same pages: an answer let go is laid out
// again, which is what happened before there was a cache. And what the cache
// holds stays inside the bound, less the one answer that set it.
func TestTheLayoutCacheIsBounded(t *testing.T) {
	defer func(a, b int) { minCacheWeight, cacheWeightFactor = a, b }(minCacheWeight, cacheWeightFactor)
	minCacheWeight, cacheWeightFactor = 4, 1

	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	evicted := 0
	for name, doc := range speculativeCorpus {
		built := Build(Input{HTML: doc})
		l := newLayouter(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
		got := l.layout()
		plain := newLayouter(built.Root, Size{W: w, H: h}, built.Fonts, NewRecorder(nil))
		plain.noCache = true
		if diff := fragmentDiff("root", got, plain.layout()); diff != "" {
			t.Errorf("%s: with answers evicted the page changed: %s", name, diff)
		}
		evicted += l.cache.evicted
		if limit := max(minCacheWeight, cacheWeightFactor*l.cache.largest); l.cache.weight > limit+l.cache.largest {
			t.Errorf("%s: the cache holds %d against a bound of %d", name, l.cache.weight, limit)
		}
	}
	if evicted == 0 {
		t.Fatal("nothing was evicted; the bound was not tested")
	}
}

// TestTheLayoutCacheChangesNothingInTheSuite is the same comparison on every
// document of the Web Platform Tests reftest corpus, tests and references.
func TestTheLayoutCacheChangesNothingInTheSuite(t *testing.T) {
	root := wptDir(t)
	seen := map[string]bool{}
	var files []string
	for _, rt := range findReftests(t, root) {
		for _, f := range append([]string{rt.test}, refPaths(rt)...) {
			if !seen[f] {
				seen[f] = true
				files = append(files, f)
			}
		}
	}
	sort.Strings(files)
	var laid, hitDocs, hits, stored int
	for _, file := range files {
		built, done, err := buildSuiteDocument(root, file)
		if err != nil || built.Root == nil {
			continue
		}
		d := layOutTwice(t, built, wptViewport())
		done()
		laid++
		if diff := d.compare(); diff != "" {
			t.Errorf("%s: the cache changed the page: %s", file, diff)
		}
		if d.dangling > 0 {
			t.Errorf("%s: %d side effects pointed outside the tree their layout made", file, d.dangling)
		}
		if d.hits > 0 {
			hitDocs++
		}
		hits += d.hits
		stored += d.stored
	}
	t.Logf("%d documents laid out twice; %d reused a layout, %d reuses in all, %d "+
		"layouts kept", laid, hitDocs, hits, stored)
	if laid < 1000 || hitDocs == 0 {
		t.Fatalf("%d documents compared and %d reused anything; that is not a test "+
			"of the cache", laid, hitDocs)
	}
}

func refPaths(rt reftest) []string {
	out := make([]string, 0, len(rt.refs))
	for _, r := range rt.refs {
		out = append(out, r.path)
	}
	return out
}

// buildSuiteDocument builds one suite document the way renderForCompareDetail
// does, and returns what undoes the fonts it registered.
func buildSuiteDocument(root, file string) (Built, func(), error) {
	data, err := os.ReadFile(file)
	if err != nil {
		return Built{}, nil, err
	}
	src := string(data)
	if ext := strings.ToLower(filepath.Ext(file)); ext == ".xht" || ext == ".xhtml" {
		src = expandEmptyElements(src)
	}
	res, err := newSuiteResolver(root, filepath.Dir(file))
	if err != nil {
		return Built{}, nil, err
	}
	built := Build(Input{HTML: src, Resources: res, Fonts: fontSetForWPT()})
	faces := registerDocumentBlockFonts(built, res)
	return built, func() {
		unregisterBlockFonts(faces)
		res.Close()
	}, nil
}

// TestTheLayoutKeyHoldsEveryInput asks one box the same question with one
// input changed at a time, cache on, and holds each answer to the one a
// layouter without a cache gives. A key missing any of its fields answers the
// second question with the first question's layout.
func TestTheLayoutKeyHoldsEveryInput(t *testing.T) {
	built := Build(Input{HTML: `<div id=x style="padding-left:10%;height:50%;margin-top:5%">` +
		`one two three four five six seven eight nine ten</div>`})
	x := findBox(t, built.Root, "x")
	px := func(v float64) style.Unit { u, _ := style.FromPx(v); return u }
	type ask struct {
		name       string
		containing style.Unit
		cbHeight   style.Unit
		cbDefinite bool
		forced     *forcedGeometry
	}
	base := ask{"base", px(300), px(200), true, &forcedGeometry{width: px(100)}}
	asks := []ask{
		base,
		{"containing", px(500), px(200), true, &forcedGeometry{width: px(100)}},
		{"cbHeight", px(300), px(80), true, &forcedGeometry{width: px(100)}},
		{"cbDefinite", px(300), px(200), false, &forcedGeometry{width: px(100)}},
		{"forced width", px(300), px(200), true, &forcedGeometry{width: px(60)}},
		{"forced height", px(300), px(200), true, &forcedGeometry{width: px(100), height: px(9), hasHeight: true}},
		{"forced margin", px(300), px(200), true, &forcedGeometry{width: px(100), margin: Edges{Top: px(3)}}},
		// A forced geometry of all zeroes and none at all are different
		// questions: the first is a box forced to no width.
		{"forced zero", px(300), px(200), true, &forcedGeometry{}},
		{"nothing forced", px(300), px(200), true, nil},
	}
	w, _ := style.FromPx(600)
	cached := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
	for _, a := range asks {
		// Asked twice so that the answer is kept; the kept one is asked for by
		// every later question whose key matches it.
		for i := 0; i < 2; i++ {
			at := aloneFlow(a.cbHeight, a.cbDefinite)
			at.again = true
			got, _ := cached.blockIn(x, a.containing, at, a.forced)
			plain := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
			plain.noCache = true
			want, _ := plain.blockIn(x, a.containing, aloneFlow(a.cbHeight, a.cbDefinite), a.forced)
			if diff := fragmentDiff(a.name, got, want); diff != "" {
				t.Errorf("asked with a different %s, the cache answered: %s", a.name, diff)
			}
		}
	}
	if cached.cache.hits == 0 {
		t.Fatal("no question was answered from the cache; the comparison proves nothing")
	}
}

// TestFragmentClonerCopiesEveryField keeps fragmentCloner's list of what it
// deep-copies in step with Fragment. A field added to Fragment is either a
// value, which the struct copy carries, or something the clone has to copy
// itself — and a slice it shares with the kept original is one a caller can
// write into the cache through.
func TestFragmentClonerCopiesEveryField(t *testing.T) {
	deep := map[string]bool{
		"Children": true, "Lines": true, "Marker": true, "collapsed": true,
		"background": true, "bgBands": true, "canvasLayers": true,
	}
	shared := map[string]bool{"Box": true, "canvasColor": true}
	ft := reflect.TypeOf(Fragment{})
	for i := 0; i < ft.NumField(); i++ {
		f := ft.Field(i)
		switch f.Type.Kind() {
		case reflect.Slice, reflect.Pointer, reflect.Map, reflect.Interface:
			if !deep[f.Name] && !shared[f.Name] {
				t.Errorf("Fragment.%s is a %v, and fragmentCloner neither copies it nor "+
					"says why it may be shared", f.Name, f.Type)
			}
		}
	}
	lt := reflect.TypeOf(LineFragment{})
	for i := 0; i < lt.NumField(); i++ {
		f := lt.Field(i)
		switch f.Type.Kind() {
		case reflect.Slice, reflect.Pointer, reflect.Map, reflect.Interface:
			if f.Name != "Runs" && f.Name != "Boxes" && f.Name != "links" {
				t.Errorf("LineFragment.%s is a %v, and fragmentCloner does not copy it",
					f.Name, f.Type)
			}
		}
	}

	// And that what it copies is copied: a write through the copy does not
	// reach the original.
	built := Build(Input{HTML: `<ul><li>a <span style="background:red;position:relative">b</span>
		<a href="x">c</a></li></ul>`})
	w, _ := style.FromPx(300)
	orig := Layout(built.Root, Size{W: w, H: w}, nil, nil)
	c := fragmentCloner{seen: map[*Fragment]*Fragment{}}
	cp := c.clone(orig)
	var mutate func(f *Fragment)
	mutate = func(f *Fragment) {
		f.BorderRect.X += 7
		for i := range f.Lines {
			f.Lines[i].Rect.X += 7
			for j := range f.Lines[i].Runs {
				f.Lines[i].Runs[j].X += 7
			}
			for _, b := range f.Lines[i].Boxes {
				mutate(b)
			}
			for _, b := range f.Lines[i].links {
				mutate(b)
			}
		}
		if f.Marker != nil {
			f.Marker.At.X += 7
		}
		for i := range f.bgBands {
			f.bgBands[i].X += 7
		}
		for _, k := range f.Children {
			mutate(k)
		}
	}
	again := c
	again.seen = map[*Fragment]*Fragment{}
	before := again.clone(orig)
	// And every inline fragment of the copy is its own: a copy that shared one
	// with the original would share it with before as well, and the comparison
	// below could not see the write.
	origs := map[*Fragment]bool{}
	var mine func(f *Fragment, into map[*Fragment]bool)
	mine = func(f *Fragment, into map[*Fragment]bool) {
		for _, line := range f.Lines {
			for _, b := range append(append([]*Fragment(nil), line.Boxes...), line.links...) {
				into[b] = true
			}
		}
		for _, k := range f.Children {
			mine(k, into)
		}
	}
	mine(orig, origs)
	copies := map[*Fragment]bool{}
	mine(cp, copies)
	if len(origs) < 2 {
		t.Fatalf("the document has %d inline fragments, so this checks nothing", len(origs))
	}
	for f := range copies {
		if origs[f] {
			t.Errorf("the copy shares the inline fragment of %v with the original", f.Box.Element)
		}
	}
	mutate(cp)
	if diff := fragmentDiff("original", orig, before); diff != "" {
		t.Fatalf("writing into a copy changed the original: %s", diff)
	}
}

// nested is d containers opened by open, one inside the next, around a little
// text, each with a sibling built by sibling from its level.
func nested(d int, open string, sibling func(k int) string) string {
	var b strings.Builder
	for k := 0; k < d; k++ {
		b.WriteString(open)
		b.WriteString(sibling(k))
		b.WriteString(`<div>`)
	}
	b.WriteString("a b c")
	for k := 0; k < d; k++ {
		b.WriteString(`</div></div>`)
	}
	return b.String()
}

// TestNestingIsNotExponential is audit C3. Every flex and grid level laid each
// item out at least twice, and each of those laid the level below out twice
// again: eighteen nested columns took nineteen seconds, a grid the same, and a
// row whose items are stretched to a taller sibling the same again.
//
// The cost is bounded as a ratio: twice the depth. It was 2^12 = 4096 times
// the work; it is about four now, because a reused layout is copied and the
// copy is as deep as what is below it — quadratic in the depth, which the box
// depth cap bounds.
//
// Counted, not timed: the layouter counts every box it lays out and every
// fragment the cache copies in place of one (layouter.work), which is the work
// an exponential multiplies, and a count is not moved by whatever else the
// machine is doing. The count stops where the bound on layout work stops the
// layout, though, and so did the time: with the cache off the deeper nest
// reaches the bound, the ratio reads about four (the bound against a smaller
// nest that also grew), and what fails is the check below that nothing was
// cut short. The two together are the guard.
func TestNestingIsNotExponential(t *testing.T) {
	none := func(int) string { return "" }
	for _, shape := range []struct {
		name, open string
		sibling    func(d int) func(k int) string
	}{
		{"columns", `<div style="display:flex;flex-direction:column">`, func(int) func(int) string { return none }},
		{"grids", `<div style="display:grid">`, func(int) func(int) string { return none }},
		{"stretched rows", `<div style="display:flex">`, func(d int) func(int) string {
			return func(k int) string { return `<div style="height:` + strconv.Itoa(30*(d-k)) + `px"></div>` }
		}},
	} {
		w, _ := style.FromPx(600)
		h, _ := style.FromPx(100000)
		small := Build(Input{HTML: nested(12, shape.open, shape.sibling(12))})
		large := Build(Input{HTML: nested(24, shape.open, shape.sibling(24))})
		work := func(b Built) (int, *Fragment) {
			l := newLayouter(b.Root, Size{W: w, H: h}, nil, nil)
			f := l.layout()
			return l.work, f
		}
		lo, sf := work(small)
		hi, lf := work(large)
		if sf == nil || lf == nil {
			t.Fatalf("%s: nothing was laid out", shape.name)
		}
		ratio := costtest.Count(t, shape.name, int64(lo), int64(hi))
		// Laid out whole, too, and not cut short by the bound on layout work:
		// an exponential that the bound stops is cheap, and is also a page
		// with its content missing.
		rec := NewRecorder(nil)
		Layout(large.Root, Size{W: w, H: h}, nil, rec)
		for _, f := range rec.Findings() {
			if f.Rule == RuleLimit {
				t.Errorf("%s: %s", shape.name, f.Message)
			}
		}
		if ratio > 8 {
			t.Errorf("%s: twice the depth did %.1f times the layout work (%d units "+
				"against %d); with every layout asked twice at every level it is 4096",
				shape.name, ratio, hi, lo)
		}
	}
}

// TestLayoutWorkIsBounded is the bound that holds where the cache has nothing
// to reuse: clamps nested inside clamps, each laid out twice on each of its
// ancestor's two passes. It fires, says so, and stops the work near the bound.
func TestLayoutWorkIsBounded(t *testing.T) {
	defer func(a, b int) { maxLayoutWork, minLayoutWork = a, b }(maxLayoutWork, minLayoutWork)
	maxLayoutWork, minLayoutWork = 4, 2000

	var doc strings.Builder
	const d = 14
	for k := 0; k < d; k++ {
		doc.WriteString(`<div style="line-clamp:` + strconv.Itoa(50+2*(d-k)) + `">`)
	}
	doc.WriteString(strings.Repeat("a<br>", 100))
	for k := 0; k < d; k++ {
		doc.WriteString(`<div>b</div><div>b</div><div>b</div></div>`)
	}
	built := Build(Input{HTML: doc.String()})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, rec)
	l.layout()
	said := false
	for _, f := range rec.Findings() {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "box layouts and lines") {
			said = true
		}
	}
	if !said {
		t.Errorf("the document did %d units of layout work against a bound of %d and "+
			"nothing was reported", l.work, l.workLimit)
	}
	// Past the bound every charge is refused, so what is done beyond it is the
	// walk to the end of the tree and nothing more: a charge per box and per
	// block still to be visited, not another pass.
	if l.work > 2*l.workLimit {
		t.Errorf("the bound is %d and %d units were done", l.workLimit, l.work)
	}
}

// clampedAbsoluteDoc is the shape of the review finding against the cache: an
// absolutely positioned box inside an inline-grid that a clamp's ellipsis
// pushes off its last line, inside a flex item, inside d rows each stretched
// to a taller sibling. abs false is the same page with the box in flow, which
// is laid out the same way and was never refused anything.
func clampedAbsoluteDoc(d int, abs bool) string {
	pos := "position:absolute;"
	if !abs {
		pos = ""
	}
	var b strings.Builder
	for k := 0; k < d; k++ {
		b.WriteString(`<div style="display:flex"><div style="height:` + strconv.Itoa(30*(d-k)) +
			`px"></div><div>`)
	}
	b.WriteString(`<div style="display:flex"><div id=clamp style="flex:0 0 30%;line-clamp:2">w ` +
		`<span style="display:inline-grid;width:10em"><div id=abs style="font-size:20px;` + pos +
		`">ccc dd ccc dd  </div></span> a </div></div>`)
	b.WriteString(strings.Repeat(`</div></div>`, d))
	return b.String()
}

// fragmentsAndLines counts what a tree holds.
func fragmentsAndLines(f *Fragment) int {
	if f == nil {
		return 0
	}
	n := 1 + len(f.Lines)
	for _, ln := range f.Lines {
		for _, b := range ln.Boxes {
			n += fragmentsAndLines(b)
		}
	}
	for _, c := range f.Children {
		n += fragmentsAndLines(c)
	}
	return n
}

// TestAThrownAwayOutOfFlowBoxDoesNotUnkeepItsAncestors is the review's finding
// against the cache. The clamp's ellipsis leaves the inline-grid on no line,
// and the absolutely positioned box its layout queued stayed queued, against
// the grid's fragment, which is on no page. keep refused every answer whose
// side effects pointed outside the tree it made, and that fragment is outside
// the tree of every box around it: every stretched row above it was refused,
// and the nest was exponential again. Fourteen levels did 65,545 units of
// work, reached the bound and lost the page's content.
//
// Held three ways: the work at twice the depth is a small multiple of the work
// at the depth, not the bound; the deeper page is not cut short; and the page
// is the one a layouter without a cache makes.
func TestAThrownAwayOutOfFlowBoxDoesNotUnkeepItsAncestors(t *testing.T) {
	w, _ := style.FromPx(400)
	h, _ := style.FromPx(600)
	work := func(d int, abs bool) (*layouter, *Fragment, []Finding) {
		built := Build(Input{HTML: clampedAbsoluteDoc(d, abs)})
		rec := NewRecorder(nil)
		l := newLayouter(built.Root, Size{W: w, H: h}, built.Fonts, rec)
		f := l.layout()
		return l, f, rec.Findings()
	}
	const n = 5
	small, _, _ := work(n, true)
	large, lf, found := work(2*n, true)
	for _, f := range found {
		if f.Rule == RuleLimit {
			t.Errorf("at depth %d: %s", 2*n, f.Message)
		}
	}
	if ratio := float64(large.work) / float64(small.work); ratio > 8 {
		t.Errorf("depth %d did %d units of layout work and depth %d did %d: %.1f times as "+
			"much for twice the depth", n, small.work, 2*n, large.work, ratio)
	}
	if large.cache.dangling > 0 || large.unplaced > 0 {
		t.Errorf("%d side effects pointed outside their layout's tree and %d out-of-flow "+
			"boxes were queued against a fragment not on the page", large.cache.dangling,
			large.unplaced)
	}
	_, plain, _ := work(2*n, false)
	if got, want := fragmentsAndLines(lf), fragmentsAndLines(plain); got != want {
		t.Errorf("the page holds %d fragments and lines, and the same page with the box "+
			"in flow holds %d", got, want)
	}
	built := Build(Input{HTML: clampedAbsoluteDoc(2*n, true)})
	if diff := layOutTwice(t, built, Size{W: w, H: h}).compare(); diff != "" {
		t.Errorf("with the cache and without it the page differs: %s", diff)
	}
}

// TestAnOutOfFlowBoxPastAClampIsTakenBack is the root of the finding above.
// The clamp's cut discards what comes after its last line, and the atomic
// inlines there were laid out before any line was made: what their layouts did
// stayed done. The absolutely positioned box inside one was queued against a
// fragment that is on no page, placed against coordinates that were never made
// absolute, and hung from a tree nothing paints.
//
// Now the layout of an atomic inline no line holds is taken back with the
// inline: the box is not on the page, nothing is queued against a fragment
// that is not, and the answer of the item around it has no side effect
// pointing outside itself — so it is kept, and a second asker is answered from
// it with the page the first one got.
func TestAnOutOfFlowBoxPastAClampIsTakenBack(t *testing.T) {
	doc := `<div style="display:flex;flex-direction:column"><div id=clamp style="width:115px;line-clamp:2">w ` +
		`<span style="display:inline-grid;width:10em"><div id=abs style="font-size:20px;position:absolute">` +
		`ccc dd ccc dd  </div></span> a </div></div>`
	built := Build(Input{HTML: doc})
	w, _ := style.FromPx(400)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
	page := l.layout()

	abs := findBox(t, built.Root, "abs")
	var onPage func(f *Fragment) bool
	onPage = func(f *Fragment) bool {
		if f == nil {
			return false
		}
		if f.Box == abs {
			return true
		}
		for _, ln := range f.Lines {
			for _, b := range ln.Boxes {
				if onPage(b) {
					return true
				}
			}
		}
		return slices.ContainsFunc(f.Children, onPage)
	}
	if onPage(page) {
		t.Error("the box inside the content the clamp discarded is on the page")
	}
	for _, d := range l.deferred {
		if !d.parent.absolute {
			t.Errorf("an out-of-flow box (%s) was queued against a fragment that is not on "+
				"the page", PathOf(d.box.Element))
		}
	}
	if l.unplaced > 0 || l.cache.dangling > 0 {
		t.Errorf("%d out-of-flow boxes were left queued against a fragment not on the page, "+
			"and %d side effects pointed outside the tree their layout made",
			l.unplaced, l.cache.dangling)
	}

	// The item's answer is kept, and asking for it again gives the page back.
	clamp := findBox(t, built.Root, "clamp")
	var kept *layoutEntry
	for k, e := range l.cache.entries {
		if k.box == clamp && e.frag != nil {
			kept = e
		}
	}
	if kept == nil {
		t.Fatal("the clamped item's layout was not kept")
	}
	if len(kept.deferred) != 0 {
		t.Errorf("the kept answer carries %d out-of-flow boxes; the only one inside it was "+
			"discarded with the clamp's content", len(kept.deferred))
	}
	hits := l.cache.hits
	again, _ := l.blockIn(clamp, kept.key.containing,
		flow{ctx: &floatContext{}, cbHeight: kept.key.cbHeight, cbDefinite: kept.key.cbDefinite, alone: true},
		&kept.key.forced)
	if l.cache.hits != hits+1 {
		t.Fatal("asking the kept question again was not answered from the cache")
	}
	plain := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
	plain.noCache = true
	want, _ := plain.blockIn(clamp, kept.key.containing,
		flow{ctx: &floatContext{}, cbHeight: kept.key.cbHeight, cbDefinite: kept.key.cbDefinite, alone: true},
		&kept.key.forced)
	if diff := fragmentDiff("item", again, want); diff != "" {
		t.Errorf("the reused answer is not the layout: %s", diff)
	}
}

// TestADanglingSideEffectIsLeftOutNotRefused holds keep and placeAbsolutes to
// the rule they share, with a side effect no document now makes: a box queued
// against a fragment its layout made and threw away. The answer is kept
// without it, and the box is not placed — with the cache or without, so the
// two agree — rather than the answer being refused, which refused every
// enclosing answer with it.
func TestADanglingSideEffectIsLeftOutNotRefused(t *testing.T) {
	built := Build(Input{HTML: `<div id=x>a</div><div id=y style="position:absolute">b</div>`})
	x, y := findBox(t, built.Root, "x"), findBox(t, built.Root, "y")
	w, _ := style.FromPx(400)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))

	thrownAway := &Fragment{}
	kept := &Fragment{Box: x}
	l.deferred = append(l.deferred, absCandidate{box: y, parent: thrownAway})
	l.setPositioned(y, thrownAway)
	e := &layoutEntry{key: layoutKey{box: x}}
	l.keep(e, kept, collapsed{}, 0, 0)
	if e.frag == nil {
		t.Fatal("an answer with a side effect outside its tree was refused")
	}
	if len(e.deferred) != 0 || len(e.writes) != 0 {
		t.Errorf("the kept answer carries %d out-of-flow boxes and %d writes pointing at a "+
			"fragment outside it", len(e.deferred), len(e.writes))
	}
	if l.cache.dangling != 2 {
		t.Errorf("dangling is %d, not the 2 side effects left out", l.cache.dangling)
	}

	l.placeAbsolutes(Rect{W: w, H: w})
	if len(thrownAway.Children) != 0 || l.unplaced != 1 {
		t.Errorf("a box queued against a fragment that was never made absolute was placed "+
			"(%d children, %d left unplaced)", len(thrownAway.Children), l.unplaced)
	}
}

// TestTheStretchLaysOutOnlyWhatMoved is the flex row's stretch pass asking
// again only for the items the stretch changed. It used to take the whole
// first pass back and ask every item again, and the ones that had not moved
// were answered from the cache — with a copy of each one's subtree, made at
// every level of a nest of rows, charged to nothing. A nest of rows each
// holding a hundred paragraphs was 1.7 times slower than with no cache at all.
//
// So the work one stretched item costs is its own layout, however many items
// sit beside it unmoved; and the out-of-flow boxes inside the items are still
// queued in item order, which is the order they are placed in.
func TestTheStretchLaysOutOnlyWhatMoved(t *testing.T) {
	row := func(stretch string) string {
		var b strings.Builder
		b.WriteString(`<div style="display:flex"><div style="height:200px"></div>`)
		b.WriteString(`<div style="align-self:flex-start;position:relative"><p>x</p><span id=a0 style="position:absolute">a</span></div>`)
		b.WriteString(`<div style="position:relative` + stretch + `">short<span id=a1 style="position:absolute">b</span></div>`)
		b.WriteString(`<div style="align-self:flex-start;position:relative"><p>x</p><span id=a2 style="position:absolute">c</span></div>`)
		b.WriteString(strings.Repeat(`<div style="align-self:flex-start"><p>x</p><p>y</p></div>`, 40))
		b.WriteString(`</div>`)
		return b.String()
	}
	w, _ := style.FromPx(2000)
	lay := func(doc string) (*layouter, Built) {
		built := Build(Input{HTML: doc})
		l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
		l.layout()
		return l, built
	}
	moved, built := lay(row(""))
	still, _ := lay(row(";align-self:flex-start"))
	if extra := moved.work - still.work; extra > 4 {
		t.Errorf("stretching one item beside forty-two that did not move cost %d more units "+
			"of layout work than stretching none; the item is one box and one line", extra)
	}
	if moved.cache.hits > 0 {
		t.Errorf("the stretch was answered from the cache %d times; nothing that did not "+
			"move needs asking again", moved.cache.hits)
	}

	var order []string
	for _, d := range moved.deferred {
		id, _ := d.box.Element.Attr("id")
		order = append(order, id)
	}
	if got := strings.Join(order, " "); got != "a0 a1 a2" {
		t.Errorf("the out-of-flow boxes are queued as %q; the items are in the order a0 a1 a2", got)
	}
	if diff := layOutTwice(t, built, Size{W: w, H: w}).compare(); diff != "" {
		t.Errorf("with the cache and without it the page differs: %s", diff)
	}
}

// copyingClamps is clamps nested inside clamps, each laid out twice on each of
// its ancestor's two passes, around a flex item of n paragraphs. The item is
// its own formatting context and no clamp counts its lines, so its answer is
// kept and every one of the exponentially many asks for it is a copy of it.
func copyingClamps(d, n int) string {
	var doc strings.Builder
	for k := 0; k < d; k++ {
		doc.WriteString(`<div style="line-clamp:` + strconv.Itoa(4+2*(d-k)) + `">`)
	}
	doc.WriteString(`<div style="display:flex"><div>` + strings.Repeat(`<p>x</p>`, n) + `</div></div>`)
	doc.WriteString(strings.Repeat("a<br>", 8))
	for k := 0; k < d; k++ {
		doc.WriteString(`<div>b</div><div>b</div><div>b</div></div>`)
	}
	return doc.String()
}

// TestCopiesAreChargedToTheBound is the other half of the stretch finding. A
// reuse was not charged to the bound on layout work, because it is not a
// layout; but it is a copy as deep as the answer, and where the asks for an
// answer are exponential — a clamp inside a clamp, around a large item — the
// layouts stopped at the bound and the copying did not. Twelve levels around
// a thousand paragraphs copied eight million fragments.
//
// Charged at copiesPerUnit to a unit, and refused past the bound, the copying
// stops with everything else: what is copied is at most the bound's worth,
// plus the one answer that crossed it.
func TestCopiesAreChargedToTheBound(t *testing.T) {
	defer func(a int) { maxLayoutWork = a }(maxLayoutWork)
	maxLayoutWork = 8

	built := Build(Input{HTML: copyingClamps(12, 500)})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, rec)
	l.layout()
	if allowed := copiesPerUnit*(l.workLimit+1) + l.cache.largest; l.cache.copied > allowed {
		t.Errorf("%d fragments and lines were copied against a bound of %d units, which "+
			"allows %d", l.cache.copied, l.workLimit, allowed)
	}
	if l.cache.hits == 0 {
		t.Fatal("nothing was reused, so nothing was copied; the test proves nothing")
	}
	said := false
	for _, f := range rec.Findings() {
		said = said || f.Rule == RuleLimit
	}
	if !said {
		t.Errorf("the document did %d units of work against a bound of %d and nothing was "+
			"reported", l.work, l.workLimit)
	}
}

// TestAKeptAnswerIsNotHandedOutPastTheBound: once the bound is reached a box
// is laid out empty whether or not an answer for it is kept, because handing
// the answer out is a copy and the copies are what the bound now stops.
func TestAKeptAnswerIsNotHandedOutPastTheBound(t *testing.T) {
	built := Build(Input{HTML: `<div id=x style="display:flow-root">one two three</div>`})
	x := findBox(t, built.Root, "x")
	w, _ := style.FromPx(400)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
	at := aloneFlow(w, true)
	at.again = true
	for i := 0; i < 2; i++ {
		l.blockIn(x, w, at, nil)
	}
	l.blockIn(x, w, at, nil)
	if l.cache.hits != 1 {
		t.Fatalf("the third ask was answered from the cache %d times, not once", l.cache.hits)
	}
	l.work = l.workLimit
	f, _ := l.blockIn(x, w, at, nil)
	if l.cache.hits != 1 {
		t.Error("an answer was handed out from the cache past the bound")
	}
	if len(f.Lines) != 0 || l.overWork.boxes != 1 {
		t.Errorf("past the bound the box has %d lines and %d boxes were laid out empty; "+
			"want it laid out empty", len(f.Lines), l.overWork.boxes)
	}
}

// TestTheLimitSaysWhatWasNotDone is the finding the bound makes. It was
// written at the first refusal and said what that refusal was — "the lines
// after that were not made" — although every box laid out after it was laid
// out empty as well, which is the half that empties a page. It is one finding,
// made once the layout is over, and it counts both.
func TestTheLimitSaysWhatWasNotDone(t *testing.T) {
	defer func(a, b int) { maxLayoutWork, minLayoutWork = a, b }(maxLayoutWork, minLayoutWork)
	maxLayoutWork, minLayoutWork = 4, 2000

	var doc strings.Builder
	const d = 14
	for k := 0; k < d; k++ {
		doc.WriteString(`<div style="line-clamp:` + strconv.Itoa(50+2*(d-k)) + `">`)
	}
	doc.WriteString(strings.Repeat("a<br>", 100))
	for k := 0; k < d; k++ {
		doc.WriteString(`<div>b</div><div>b</div><div>b</div></div>`)
	}
	built := Build(Input{HTML: doc.String()})
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, rec)
	l.layout()
	var said []string
	for _, f := range rec.Findings() {
		if f.Rule == RuleLimit {
			said = append(said, f.Message)
		}
	}
	if len(said) != 1 {
		t.Fatalf("the bound was reported %d times: %q", len(said), said)
	}
	o := l.overWork
	if o.boxes == 0 || o.blocks == 0 {
		t.Fatalf("the bound left %d boxes empty and cut %d blocks short; this document "+
			"is meant to do both", o.boxes, o.blocks)
	}
	for _, want := range []string{
		strconv.Itoa(o.boxes) + " " + choosePlural(o.boxes, "box was", "boxes were") +
			" laid out without content",
		strconv.Itoa(o.blocks) + " " + choosePlural(o.blocks, "block was", "blocks were") +
			" cut short",
	} {
		if !strings.Contains(said[0], want) {
			t.Errorf("the finding does not say %q: %s", want, said[0])
		}
	}
}

// TestAnAnswerAskedForOnceIsNotCopied is the other cost the stretch finding
// measured: every flex and grid item's layout was kept the first time it was
// asked for, on the caller's word that it would be asked again — a copy of the
// item's whole subtree, at every level of a nest, for items that in a row are
// never asked the same question twice. The copy is now taken the first time
// only for a box that has already been laid out under another question, which
// is the box that is being laid out repeatedly.
func TestAnAnswerAskedForOnceIsNotCopied(t *testing.T) {
	const d = 16
	var doc strings.Builder
	for k := 0; k < d; k++ {
		doc.WriteString(`<div style="display:flex"><div style="height:` + strconv.Itoa(30*(d-k)) +
			`px"></div><div>` + strings.Repeat(`<p>x</p>`, 20))
	}
	doc.WriteString("a" + strings.Repeat(`</div></div>`, d))
	built := Build(Input{HTML: doc.String()})
	w, _ := style.FromPx(600)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))
	l.layout()
	if n := onePass(built.Root); l.cache.copied > n {
		t.Errorf("%d fragments and lines were copied into and out of the cache for a "+
			"document of %d boxes and characters, none of it asked for twice",
			l.cache.copied, n)
	}
}

// TestTakingBackFromTheMiddle is takeBack on side state that has more after
// it: a stretch's out-of-flow boxes come out of the queue and the rest keep
// their order, and its writes are undone although later ones stand — a later
// write for the same positioned box keeps its fragment and undoes, in turn,
// to what was there before either; a positioned inline keeps the fragments
// recorded after the stretch.
func TestTakingBackFromTheMiddle(t *testing.T) {
	built := Build(Input{HTML: `<div id=x style="position:relative">a<span id=y style="position:relative">b</span></div>` +
		`<div id=p style="position:absolute"></div><div id=q style="position:absolute"></div>` +
		`<div id=r style="position:absolute"></div>`})
	x, y := findBox(t, built.Root, "x"), findBox(t, built.Root, "y")
	p, q, r := findBox(t, built.Root, "p"), findBox(t, built.Root, "q"), findBox(t, built.Root, "r")
	w, _ := style.FromPx(400)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, NewRecorder(nil))

	f0, f1, f2 := &Fragment{}, &Fragment{}, &Fragment{}
	g1, g2 := &Fragment{}, &Fragment{}
	l.setPositioned(x, f0)
	cp := l.checkpoint(nil, nil)
	l.deferred = append(l.deferred, absCandidate{box: p})
	mid := l.sideMark()
	l.deferred = append(l.deferred, absCandidate{box: q})
	l.setPositioned(x, f1)
	l.addInlineFragment(y, g1)
	span := l.sideSince(mid)
	l.deferred = append(l.deferred, absCandidate{box: r})
	l.setPositioned(x, f2)
	l.addInlineFragment(y, g2)

	l.takeBack([]sideSpan{span})
	var boxes []*Box
	for _, d := range l.deferred {
		boxes = append(boxes, d.box)
	}
	if !slices.Equal(boxes, []*Box{p, r}) {
		t.Errorf("the queue holds %d boxes after the middle one was taken back; want p, r", len(boxes))
	}
	if l.positioned[x] != f2 {
		t.Error("taking back an earlier write undid the later one")
	}
	if got := l.inlineFragments[y]; len(got) != 1 || got[0] != g2 {
		t.Errorf("the positioned inline holds %d fragments; want only the one recorded after", len(got))
	}
	// And the later write, taken back in its turn, restores what was there
	// before the stretch: the write the stretch made is gone from the history.
	l.rollback(cp)
	if l.positioned[x] != f0 {
		t.Error("rolling the later write back restored the fragment of the stretch taken back")
	}
	if len(l.inlineFragments[y]) != 0 || len(l.deferred) != 0 || len(l.journal) != 1 {
		t.Errorf("after the rollback: %d inline fragments, %d queued, %d journalled; want 0, 0, 1",
			len(l.inlineFragments[y]), len(l.deferred), len(l.journal))
	}
}
