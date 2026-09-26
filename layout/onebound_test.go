package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// TestGeneratedContentEarnsNoLayoutAllowance is the rule the two allowances
// share (budget.go, "The other bound"): an allowance grows with what the
// document wrote and not with what it made.
//
// The bound on layout work was a multiple of one pass over the box tree, and
// the tree holds generated content — which is what the document's work budget
// lets a small input make a great deal of. Ten paragraphs whose ::before repeats
// a kilobyte attribute sixty-four times make 640 kilobytes of text from ten of
// markup, and the layout allowance grew with the 640.
func TestGeneratedContentEarnsNoLayoutAllowance(t *testing.T) {
	val := strings.Repeat("x ", 500)
	css := `p::before { content: ` + strings.Repeat("attr(data-x) ", 64) + `}`
	html := strings.Repeat(`<p data-x="`+val+`">a</p>`, 10)
	built := Build(Input{HTML: html, CSS: []Stylesheet{{Source: css}}})

	// The generated text is there, or nothing above was measured.
	generated := 0
	var walk func(*Box, bool)
	walk = func(b *Box, inPseudo bool) {
		inPseudo = inPseudo || b.Pseudo != ""
		if inPseudo {
			generated += len(b.Text)
		}
		for _, c := range b.Children {
			walk(c, inPseudo)
		}
	}
	walk(built.Root, false)
	input := len(html) + len(css)
	if generated < 40*input {
		t.Fatalf("the document generated %d bytes from %d; the test needs the "+
			"generated content to dwarf the markup", generated, input)
	}

	if limit, most := layoutWorkLimit(built.Root), max(minLayoutWork, maxLayoutWork*input); limit > most {
		t.Errorf("a document of %d bytes that generated %d is allowed %d layouts; "+
			"what it wrote earns at most %d", input, generated, limit, most)
	}
}

// TestTheLayoutCutIsTheBudgetsFinding is the other thing the two allowances
// share: a cut is reported one way. The layout's finding says what was left out
// in the budget's words, is placed where the first refusal was, is counted once
// per refusal — and, being the budget's own finding, is not itself refused when
// the document's work budget has nothing left to pay for findings with. It was
// an ordinary finding charged to that budget, and a document that had spent
// both lost the finding saying so.
func TestTheLayoutCutIsTheBudgetsFinding(t *testing.T) {
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
	// Nothing left to pay for a finding with.
	rec.work.left, rec.work.reserve = 0, 0
	w, _ := style.FromPx(600)
	l := newLayouter(built.Root, Size{W: w, H: w}, built.Fonts, rec)
	l.layout()

	o := l.overWork
	if o.first == nil || o.boxes+o.blocks == 0 {
		t.Fatal("the bound did not fire, so the test proves nothing")
	}
	var cut []Finding
	for _, f := range rec.Findings() {
		if f.Rule == RuleLimit && strings.Contains(f.Message, "box layouts and lines") {
			cut = append(cut, f)
		}
	}
	if len(cut) != 1 {
		t.Fatalf("the layout's cut was reported %d times with the work budget spent: %v",
			len(cut), rec.Findings())
	}
	f := cut[0]
	if !strings.HasPrefix(f.Message, "this document asks for more work than this engine "+
		"does for one document; left out: ") {
		t.Errorf("the layout's cut is not in the budget's words: %s", f.Message)
	}
	if el := boxElement(o.first); el == nil || f.Source.HTMLOffset != el.Offset ||
		f.Path != PathOf(el) || f.Path == "" {
		t.Errorf("the cut is placed at %+v, %q; want the element of the first box "+
			"refused", f.Source, f.Path)
	}
	if got, want := rec.Counts()[RuleLimit], o.boxes+o.blocks; got != want {
		t.Errorf("RuleLimit was counted %d times for %d boxes and blocks cut", got, want)
	}
}
