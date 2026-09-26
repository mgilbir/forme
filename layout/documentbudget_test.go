package layout

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/css"
	"github.com/mgilbir/forme/html"
	"github.com/mgilbir/forme/internal/costtest"
	"github.com/mgilbir/forme/style"
)

// The document's work budget, and the algorithms it is not the answer for.
//
// Most of these are cost tests, and they bound a *ratio*: the same shape of
// document at n and at 4n. Linear work comes out at about four; the quadratic
// it replaced at about sixteen. The bound is eight, which is far from both, and
// a ratio does not care how fast the machine is or whether the race detector
// is making everything fifteen times slower.
//
// The ratios here are timed, and not counted off the budget, because the work
// they guard is not charged: finding a drop-down's chosen option, a ruby's
// walk, the counter walk's scope keeping. The budget is for the work a small input
// can amplify, and these are algorithms that were made linear where they stand
// rather than bounded (see budget.go); charging them so that a test could
// count them would be instrumenting the engine for its tests. So the timing is
// built so that a busy machine cannot decide it; see costtest.Time.

// requireLinear fails when build(4n) takes more than eight times as long as
// build(n), timed by costtest.Time.
func requireLinear(t *testing.T, what string, n int, build func(n int)) {
	t.Helper()
	r := costtest.Time(t, what, func() { build(n) }, func() { build(4 * n) })
	if r.Ratio > 8 {
		t.Errorf("%s: four times the input took %v; linear work is about four, and "+
			"quadratic is about sixteen", what, r)
	}
}

// TestADropDownIsBuiltOnceNotOncePerOption is audit C47: the chosen option of a
// <select> was found afresh for each of its children, and finding it walks
// every option.
func TestADropDownIsBuiltOnceNotOncePerOption(t *testing.T) {
	requireLinear(t, "a drop-down's options", 1000, func(n int) {
		Build(Input{HTML: "<select>" + strings.Repeat("<option>x", n) + "</select>"})
	})
	// And it is still the one it was: the last option marked selected.
	built := Build(Input{HTML: `<select id="s"><option>a<option selected>b<option>c</select>`})
	sel := findBox(t, built.Root, "s")
	var text strings.Builder
	var walk func(*Box)
	walk = func(b *Box) {
		text.WriteString(b.Text)
		for _, c := range b.Children {
			walk(c)
		}
	}
	walk(sel)
	if got := strings.TrimSpace(text.String()); got != "b" {
		t.Errorf("the drop-down shows %q, want the selected option %q", got, "b")
	}
}

// TestNestedRubiesAreEachWalkedOnce is audit C134: every ruby looked for an
// annotation through the whole of its subtree, so each element was visited
// once for every ruby above it. The depth and the width both grow here,
// because it is their product that was the cost.
//
// The report is timed on its own, over documents built beforehand: the rest of
// Build is linear in the leaves and would dilute the curve being measured.
func TestNestedRubiesAreEachWalkedOnce(t *testing.T) {
	built := map[int]Built{}
	requireLinear(t, "nested rubies", 1, func(n int) {
		b, ok := built[n]
		if !ok {
			depth, leaves := 60*n, 3000*n
			b = Build(Input{
				HTML: strings.Repeat("<s>", depth) + strings.Repeat("<i></i>", leaves) +
					strings.Repeat("</s>", depth),
				CSS: []Stylesheet{{Source: "s { display: ruby }"}},
			})
			built[n] = b
		}
		reportUnsupportedDisplays(b.Document, b.Styles, nil, NewRecorder(nil))
	})
}

// TestTheInnermostRubyAloneReportsItsAnnotation is what stopping the walk at a
// nested ruby decides: the annotation belongs to the ruby it is in, and an
// outer ruby with none of its own lays out as the base it is.
func TestTheInnermostRubyAloneReportsItsAnnotation(t *testing.T) {
	built := Build(Input{
		HTML: `<s id="outer"><s id="inner">base<u>note</u></s></s>`,
		CSS:  []Stylesheet{{Source: "s { display: ruby } u { display: ruby-text }"}},
	})
	var paths []string
	for _, f := range built.Findings {
		if strings.Contains(f.Message, "display: ruby") {
			paths = append(paths, f.Path)
		}
	}
	if len(paths) != 1 || !strings.HasSuffix(paths[0], "s#inner") {
		t.Errorf("the ruby findings are at %q; want the inner ruby's alone", paths)
	}
}

// TestCountersCostWhatTheirScopesHold is audit C16 and C17. A counter-reset of
// many names on the body, and an element count below it: the walk used to
// visit every name at every element, snapshot every name for every
// pseudo-element, and — for reversed() — carry a vector of every name through
// every node and keep a map of them for every element. Names and elements
// both grow, because it was their product.
//
// The walk is timed on its own, over documents styled beforehand, as the ruby
// report is: the rest of Build is linear in the elements and the names and
// would dilute the curve being measured. Timed through Build, a walk that only
// looked at every name at every element came out at 7.3, inside the bound.
func TestCountersCostWhatTheirScopesHold(t *testing.T) {
	for _, tc := range []struct {
		name, reset, content string
	}{
		{"counters in scope, and a snapshot per pseudo-element", "c%d 0", `""`},
		{"reversed counters with no number", "reversed(c%d)", `counter(c0)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type styledDoc struct {
				doc    *html.Node
				styled style.Styled
			}
			made := map[int]styledDoc{}
			requireLinear(t, tc.name, 1, func(n int) {
				d, ok := made[n]
				if !ok {
					names, elements := 256*n, 1000*n
					var src strings.Builder
					src.WriteString("body { counter-reset:")
					for i := 0; i < names; i++ {
						src.WriteString(" " + strings.Replace(tc.reset, "%d", strconv.Itoa(i), 1))
					}
					src.WriteString(" } i::before { content: " + tc.content + "; counter-increment: c0 }")
					rules, _ := css.ParseStylesheet(src.String())
					d.doc, _, _ = html.Parse("<body>" + strings.Repeat("<i></i>", elements))
					d.styled = style.Apply(d.doc, []style.Sheet{{Origin: style.OriginAuthor, Rules: rules}})
					got := computeCounters(d.doc, d.styled.Styles, d.styled.Pseudo, NewRecorder(nil))
					if len(got.pseudo) != elements || len(got.cut) != 0 {
						t.Fatalf("%d of %d pseudo-elements counted and %d cut; the fixture "+
							"does not reach the walk it is measuring", len(got.pseudo),
							elements, len(got.cut))
					}
					made[n] = d
				}
				computeCounters(d.doc, d.styled.Styles, d.styled.Pseudo, NewRecorder(nil))
			})
		})
	}
}

// TestAReversedCounterStillCountsItsScope is the reversed() walk's answer,
// kept: five items incrementing by one begin at six and count down, and a
// nested list with a scope of its own does not count toward the outer one.
func TestAReversedCounterStillCountsItsScope(t *testing.T) {
	built := Build(Input{HTML: `<ol reversed id="o"><li id="a">a<li id="b">b` +
		`<ol reversed><li>x<li>y</ol><li id="c">c<li>d<li>e</ol>`})
	for id, want := range map[string]int{"a": 5, "b": 4, "c": 3} {
		if got := findBox(t, built.Root, id).ListValue; got != want {
			t.Errorf("item %s is numbered %d, want %d", id, got, want)
		}
	}
}

// TestCountersWritesAPieceAtATimeAgainstTheCap is audit C15: counters() joined
// every level with its separator and wrote the lot, and the cap on one
// pseudo-element's content was only asked between tokens. So one token could
// write as much as it liked.
func TestCountersWritesAPieceAtATimeAgainstTheCap(t *testing.T) {
	sep := strings.Repeat("x", 64<<10)
	vals := make([]int, 20) // 20 levels of 64 KB is over the megabyte.
	got := resolveContent(`counters(c, "`+sep+`")`, nil,
		counterValues{"c": vals}, nil, 0)
	if got.unsupported == "" {
		t.Fatalf("one counters() produced %d bytes, past the %d one pseudo-element may",
			len(got.text()), maxContentLength)
	}
	// And the last token of a value is asked too, which the check between
	// tokens never reached.
	got = resolveContent(`"a" counters(c, "`+sep+`")`, nil,
		counterValues{"c": vals}, nil, 0)
	if got.unsupported == "" {
		t.Fatalf("a trailing counters() produced %d bytes, past the cap", len(got.text()))
	}
	// Under the cap it is still what it was.
	got = resolveContent(`counters(c, ".")`, nil, counterValues{"c": {1, 2, 3}}, nil, 0)
	if got.text() != "1.2.3" {
		t.Errorf("counters(c, \".\") is %q, want %q", got.text(), "1.2.3")
	}
}

// lowWork lowers the document's work budget for one test, so that it can be
// crossed by a document a test can build.
func lowWork(t *testing.T, steps int64) {
	t.Helper()
	old := workFloor
	workFloor = steps
	t.Cleanup(func() { workFloor = old })
}

// requireCut asserts the budget's one finding, naming what was cut.
func requireCut(t *testing.T, findings []Finding, what string) {
	t.Helper()
	requireFinding(t, findings, RuleLimit, "left out: "+what)
}

// TestGeneratedContentIsChargedToTheDocument is the other half of C15: the cap
// on one pseudo-element bounds nothing about a document that puts the same
// content on every element. The generated text is charged to the document's
// budget, and when it runs out the rest of the document is still laid out and
// painted — only the generated content past that point is missing, and a
// finding says so.
func TestGeneratedContentIsChargedToTheDocument(t *testing.T) {
	lowWork(t, 1<<20)
	gen := strings.Repeat("g", 1000)
	in := Input{
		HTML: "<p>first</p>" + strings.Repeat("<i></i>", 100) + "<p>last</p>",
		CSS:  []Stylesheet{{Source: `i::before { content: "` + gen + `" }`}},
	}
	got := Compose(in, Options{})
	requireCut(t, got.Findings, "the generated content past that point")
	text := drawnText(got.Ops)
	if !strings.Contains(text, "first") || !strings.Contains(text, "last") {
		t.Errorf("the document's own text did not survive the budget: %.80q", text)
	}
	if n := strings.Count(text, "g"); n == 0 || n >= 100*len(gen) {
		t.Errorf("%d bytes of generated content were drawn; want some and not all "+
			"of the %d asked for", n, 100*len(gen))
	}
}

// TestTheBudgetIsTheDocumentsNotTheCallers is the grant: a larger document
// gets more work, so the same generated content that is cut from a short
// document is drawn in full in a long one.
func TestTheBudgetIsTheDocumentsNotTheCallers(t *testing.T) {
	lowWork(t, 1<<20)
	gen := strings.Repeat("g", 1000)
	in := Input{
		HTML: "<p>" + strings.Repeat("filler ", 40000) + "</p>" + strings.Repeat("<i></i>", 100),
		CSS:  []Stylesheet{{Source: `i::before { content: "` + gen + `" }`}},
	}
	built := Build(in)
	for _, f := range built.Findings {
		if f.Rule == RuleLimit {
			t.Fatalf("a document of %d bytes was cut short by a budget that grows "+
				"with it: %s", len(in.HTML), f.Message)
		}
	}
}

// TestTheRecorderRemembersOnlyWhatItKeeps is audit C18. Past the bound on the
// list, nothing more is remembered — the memo was the thing growing without
// one — and the key is a digest, not the finding's text.
func TestTheRecorderRemembersOnlyWhatItKeeps(t *testing.T) {
	r := NewRecorder(nil)
	long := strings.Repeat("p", 1<<16)
	for i := 0; i < 4*maxFindings; i++ {
		r.ReportDetail(Finding{Rule: RuleInvalidCSS, Message: strconv.Itoa(i), Path: long})
	}
	if len(r.seen) > maxFindings {
		t.Errorf("the recorder remembers %d findings and keeps %d", len(r.seen), maxFindings)
	}
	if !r.Truncated() || r.Count(RuleInvalidCSS) != 4*maxFindings {
		t.Errorf("truncated %v, counted %d; want true and %d", r.Truncated(),
			r.Count(RuleInvalidCSS), 4*maxFindings)
	}
	// A repeat of a finding in the list is still a repeat, and does not make
	// the list look cut when it is not.
	r = NewRecorder(nil)
	for i := 0; i < 3; i++ {
		r.ReportDetail(Finding{Rule: RuleInvalidCSS, Message: "same", Path: long})
	}
	if len(r.Findings()) != 1 || r.Truncated() {
		t.Errorf("three identical findings made %d, truncated %v", len(r.Findings()), r.Truncated())
	}
}

// TestAPathIsBounded is the other half of C18: a finding's path was every
// ancestor with its whole id, so it was as long as the document made its ids
// and as deep as it nested them.
func TestAPathIsBounded(t *testing.T) {
	id := strings.Repeat("d", 5000)
	var b strings.Builder
	for i := 0; i < 200; i++ {
		b.WriteString(`<div id="` + id + strconv.Itoa(i) + `">`)
	}
	b.WriteString(`<i id="leaf"></i>`)
	doc, _, _ := html.Parse(b.String())
	leaf := elementByID(doc, "leaf")
	if leaf == nil {
		t.Fatal("no leaf")
	}
	path := PathOf(leaf)
	if len(path) > (pathOuter+pathInner)*(pathPartBytes+8)+32 {
		t.Errorf("the path is %d bytes", len(path))
	}
	if !strings.HasPrefix(path, "html > body > div#") || !strings.HasSuffix(path, "i#leaf") ||
		!strings.Contains(path, "more…") {
		t.Errorf("the path does not say where the element is: %.200q", path)
	}
	// A shallow path is what it always was.
	doc, _, _ = html.Parse(`<div class="a b"><p id="x">t</p></div>`)
	if got := PathOf(elementByID(doc, "x")); got != "html > body > div.a > p#x" {
		t.Errorf("PathOf = %q", got)
	}
}

// TestADataStylesheetIsNamedShort is the scenario that made C18 quadratic: a
// data: stylesheet was named by its whole URL, and every finding about a rule
// in it carried that name into the recorder.
//
// Timed and not counted, although the recorder charges the budget for every
// byte of a finding it reads, the sheet's name among them. The cascade reports
// two hundred problems a sheet and no more, so the findings that reach the
// recorder are linear in the name however long it is. What stays quadratic
// with the name at full length is work nothing charges — the styler, for one,
// keys its memo of what it has reported by the sheet's name and asks it about
// every declaration. With the name at full length, counted, this came out at
// 4.1; timed, at 13.4.
func TestADataStylesheetIsNamedShort(t *testing.T) {
	requireLinear(t, "findings about a data: stylesheet", 500, func(n int) {
		var css strings.Builder
		for i := 0; i < n; i++ {
			css.WriteString("x { foo-" + strconv.Itoa(i) + ": 1 }")
		}
		href := "data:text/css;base64," + base64.StdEncoding.EncodeToString([]byte(css.String()))
		Build(Input{HTML: `<link rel="stylesheet" href="` + href + `"><p>x</p>`})
	})
	href := "data:text/css," + strings.Repeat("x%7Bfoo:1%7D", 100)
	built := Build(Input{HTML: `<link rel="stylesheet" href="` + href + `"><p>x</p>`})
	for _, f := range built.Findings {
		if len(f.Source.Sheet) > 200 {
			t.Errorf("a finding names its sheet in %d bytes", len(f.Source.Sheet))
		}
	}
}

// truncatedPNG is a picture whose header decodes and whose body does not: a
// real PNG of the given size with the end cut off. A decoder allocates the
// whole picture before it finds out.
func truncatedPNG(t *testing.T, w, h int) []byte {
	data := encodePNG(t, w, h)
	return data[:len(data)-16]
}

// TestAPictureThatFailsIsDecodedOnceAndCharged is audit C19. The pixel budget
// was charged only when a decode succeeded, so a picture whose body failed cost
// its whole size in allocation and nothing in budget; and the memo was by the
// reference's spelling, so "b.png?1", "b.png?2" were a decode each.
func TestAPictureThatFailsIsDecodedOnceAndCharged(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "b.png"), truncatedPNG(t, 1000, 1000), 0o600); err != nil {
		t.Fatal(err)
	}
	// DirResolver does not read a query as part of the name, which is the
	// case the audit found: one file, many spellings.
	const refs = 40
	var markup strings.Builder
	for i := 0; i < refs; i++ {
		markup.WriteString(`<img src="b.png?` + strconv.Itoa(i) + `">`)
		markup.WriteString(`<video poster="b.png"></video>`)
	}
	loadInDir(t, dir, "<p>warm</p>")

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	built := loadInDir(t, dir, markup.String())
	runtime.ReadMemStats(&after)

	// One decode of a megapixel picture allocates four megabytes; a decode per
	// reference is eighty of them.
	if grew := after.TotalAlloc - before.TotalAlloc; grew > 24<<20 {
		t.Errorf("%d references to one broken picture allocated %d MB; it is decoded "+
			"once, which is about 4 MB", 2*refs, grew>>20)
	}
	requireFinding(t, built.Findings, RuleImageUndecodable, "readable header and unreadable content")
	requireFinding(t, built.Findings, RuleImageUndecodable, "is the same file as")
}

// TestTheDeclaredPixelsAreChargedWhateverTheDecodeDoes is the charge itself: a
// picture that fails spends the budget as a picture that succeeds does, so a
// good picture after two broken ones finds it spent.
func TestTheDeclaredPixelsAreChargedWhateverTheDecodeDoes(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"a.png", "b.png"} {
		// Two different broken pictures, so that neither is the other's memo.
		if err := os.WriteFile(filepath.Join(dir, name), truncatedPNG(t, 20, 20+i), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writePNG(t, filepath.Join(dir, "good.png"), 20, 20)
	old := maxDocumentPixels
	maxDocumentPixels = 1000 // Room for two of the three and not the third.
	defer func() { maxDocumentPixels = old }()

	built := loadInDir(t, dir, `<img src="a.png"><img src="b.png"><img id="g" src="good.png">`)
	if findBox(t, built.Root, "g").Replaced != nil {
		t.Error("the good picture was decoded, so the broken ones were not charged")
	}
	requireFinding(t, built.Findings, RuleLimit, "pixels this engine will decode")
}

// TestTheBudgetCutsPicturesAndSaysSo is the work budget refusing a read: the
// pictures past that point are not drawn, the rest of the page is.
func TestTheBudgetCutsPicturesAndSaysSo(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 8; i++ {
		writePNG(t, filepath.Join(dir, "p"+strconv.Itoa(i)+".png"), 100, 100+i)
	}
	lowWork(t, 40000)
	var markup strings.Builder
	markup.WriteString("<p>words</p>")
	for i := 0; i < 8; i++ {
		markup.WriteString(`<img src="p` + strconv.Itoa(i) + `.png">`)
	}
	res, err := NewDirResolver(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	got := Compose(Input{HTML: markup.String(), Resources: res}, Options{})
	requireCut(t, got.Findings, "the pictures past that point")
	if !strings.Contains(drawnText(got.Ops), "words") {
		t.Error("the page's text was not drawn")
	}
	pictures := 0
	for _, op := range got.Ops {
		if _, ok := op.(DrawImage); ok {
			pictures++
		}
	}
	if pictures == 0 || pictures == 8 {
		t.Errorf("%d of 8 pictures were drawn; want some and not all", pictures)
	}
}

// elementByID is the element with an id, or nil.
func elementByID(doc *html.Node, id string) *html.Node {
	var found *html.Node
	doc.Walk(func(n *html.Node) bool {
		if found == nil && n.Type == html.ElementNode {
			if v, ok := n.Attr("id"); ok && v == id {
				found = n
			}
		}
		return found == nil
	})
	return found
}

// readCounter counts the reads a document asks for.
type readCounter struct {
	ResourceResolver
	reads map[string]int
}

func (c *readCounter) Resolve(ref string) ([]byte, error) {
	c.reads[ref]++
	return c.ResourceResolver.Resolve(ref)
}

// TestAPosterIsReadOnce is the <video poster> half of C19: posters were loaded
// afresh for every <video>, with no memo at all.
func TestAPosterIsReadOnce(t *testing.T) {
	dir := t.TempDir()
	writePNG(t, filepath.Join(dir, "p.png"), 20, 20)
	res, err := NewDirResolver(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Close()
	counting := &readCounter{ResourceResolver: res, reads: map[string]int{}}
	built := Build(Input{HTML: strings.Repeat(`<video poster="p.png"></video>`, 30),
		Resources: counting})
	if n := counting.reads["p.png"]; n != 1 {
		t.Errorf("one poster on thirty videos was read %d times", n)
	}
	var walk func(*Box) int
	walk = func(b *Box) int {
		n := 0
		if b.Replaced != nil && b.Replaced.Image != nil {
			n++
		}
		for _, c := range b.Children {
			n += walk(c)
		}
		return n
	}
	if n := walk(built.Root); n != 30 {
		t.Errorf("%d of thirty videos show their poster", n)
	}
}

// TestWhatIsReadIsChargedToTheDocument is the read, charged by its length: an
// SVG is not decoded into pixels, so this is the only charge it meets.
func TestWhatIsReadIsChargedToTheDocument(t *testing.T) {
	dir := t.TempDir()
	for i := 0; i < 8; i++ {
		svg := `<svg xmlns="http://www.w3.org/2000/svg" width="10" height="10"><!--` +
			strings.Repeat("x", 20000) + strconv.Itoa(i) + `--><rect width="10" height="10" fill="green"/></svg>`
		if err := os.WriteFile(filepath.Join(dir, "s"+strconv.Itoa(i)+".svg"), []byte(svg), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	lowWork(t, 70000)
	var markup strings.Builder
	for i := 0; i < 8; i++ {
		markup.WriteString(`<img id="s` + strconv.Itoa(i) + `" src="s` + strconv.Itoa(i) + `.svg">`)
	}
	built := loadInDir(t, dir, markup.String())
	requireCut(t, built.Findings, "the pictures past that point")
	if findBox(t, built.Root, "s0").Replaced == nil || findBox(t, built.Root, "s7").Replaced != nil {
		t.Error("want the first picture drawn and the last cut")
	}
}

// TestTheBoxTreeIsChargedToTheDocument is the box, charged: the tree stops at
// the budget, once, and says so.
//
// It is asked of the builder directly. Through a document it is hard to reach
// at all, and that is by design: markup earns its own boxes through the grant,
// and the thing that makes boxes out of little markup — generated content — is
// refused by its own charge first.
func TestTheBoxTreeIsChargedToTheDocument(t *testing.T) {
	lowWork(t, 10*costBox)
	rec := NewRecorder(nil)
	b := &boxBuilder{rec: rec}
	made := 0
	for i := 0; i < 20; i++ {
		if b.roomAt(0) {
			made++
		}
	}
	if made == 0 || made > 10 {
		t.Errorf("%d boxes were made on a budget for 10", made)
	}
	requireCut(t, rec.Findings(), "the rest of the box tree")
}

// TestCountersAreChargedToTheDocument is the counter walk, charged: it stops,
// says so, and a pseudo-element whose counters were not taken is marked cut —
// so that it generates nothing, rather than a number read as nought.
func TestCountersAreChargedToTheDocument(t *testing.T) {
	lowWork(t, 1<<16)
	var inc strings.Builder
	for i := 0; i < 500; i++ {
		inc.WriteString(" c" + strconv.Itoa(i))
	}
	doc, _, _ := html.Parse(strings.Repeat("<i></i>", 200))
	styles := blockStyles(doc, style.Initial().With("display", "block").
		With("counter-increment", inc.String()))
	pseudo := map[style.PseudoKey]style.ComputedStyle{}
	for n := range styles {
		pseudo[style.PseudoKey{Node: n, Name: "before"}] = style.Initial().
			With("content", "counter(c0)")
	}
	rec := NewRecorder(nil)
	got := computeCounters(doc, styles, pseudo, rec)
	requireCut(t, rec.Findings(), "the counters past that point")
	if len(got.pseudo) == 0 || len(got.cut) == 0 {
		t.Fatalf("%d pseudo-elements counted and %d cut; want some of each",
			len(got.pseudo), len(got.cut))
	}
	for key := range pseudo {
		_, counted := got.pseudo[key]
		if counted == got.cut[key] {
			t.Fatalf("a pseudo-element is counted %v and cut %v; it is one or the other",
				counted, got.cut[key])
		}
	}
}

// TestFindingsAreChargedToTheDocument is the recorder's reading of a finding,
// charged: past the budget a finding is counted and not kept, and the list
// says it is not the whole story.
func TestFindingsAreChargedToTheDocument(t *testing.T) {
	lowWork(t, 1<<16)
	r := NewRecorder(nil)
	long := strings.Repeat("m", 1000)
	for i := 0; i < 200; i++ {
		r.ReportDetail(Finding{Rule: RuleInvalidCSS, Message: long + strconv.Itoa(i)})
	}
	requireCut(t, r.Findings(), "some findings")
	kept := 0
	for _, f := range r.Findings() {
		if f.Rule == RuleInvalidCSS {
			kept++
		}
	}
	if kept == 0 || kept == 200 || !r.Truncated() || r.Count(RuleInvalidCSS) != 200 {
		t.Errorf("kept %d of 200, truncated %v, counted %d", kept, r.Truncated(),
			r.Count(RuleInvalidCSS))
	}
}

// TestAStageThatAmplifiesLeavesTheRestOfThePage is the reserve. The counter
// walk runs before a single box exists, and a document that spends the budget
// there must still have its own content built and painted: the counters past
// the budget are what is missing, not the page.
func TestAStageThatAmplifiesLeavesTheRestOfThePage(t *testing.T) {
	lowWork(t, 1<<16)
	var css strings.Builder
	css.WriteString("i { counter-increment:")
	for i := 0; i < 500; i++ {
		css.WriteString(" c" + strconv.Itoa(i))
	}
	css.WriteString(" }")
	got := Compose(Input{HTML: "<p>words</p>" + strings.Repeat("<i></i>", 200) + "<p>more</p>",
		CSS: []Stylesheet{{Source: css.String()}}}, Options{})
	requireCut(t, got.Findings, "the counters past that point")
	if text := drawnText(got.Ops); !strings.Contains(text, "words") || !strings.Contains(text, "more") {
		t.Errorf("the page's own text was not drawn: %q", text)
	}
}
