package layout

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mgilbir/forme/shape"
)

// ComposeContext: a document composed under a context and a budget for the
// shaping it costs (issue 917).

// trialDoc is a document set partly in a face its own @font-face loads, so
// that both kinds of face — the caller's and the document's — are shaped.
func trialDoc(text string) (Input, *fileResolver) {
	res := &fileResolver{files: map[string][]byte{"trial.ttf": realFont()}}
	return Input{
		HTML: `<style>@font-face { font-family: Trial; src: url(trial.ttf) }</style>` +
			`<p>` + text + `</p><p style="font-family: Trial">` + text + `</p>`,
		Resources: res,
	}, res
}

// TestComposeContextComposesWhatComposeDoes: under limits it does not reach,
// the composition is Compose's, op for op, and says what it cost.
func TestComposeContextComposesWhatComposeDoes(t *testing.T) {
	in, _ := trialDoc("The quarterly revenue of the office rose")
	plain := Compose(in, Options{})
	in, _ = trialDoc("The quarterly revenue of the office rose")
	got, err := ComposeContext(context.Background(), in, Options{}, shape.RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if got.ShapingWork <= 0 {
		t.Errorf("a document of text was charged %d units of shaping", got.ShapingWork)
	}
	if plain.ShapingWork != 0 {
		t.Errorf("Compose reported %d units of shaping, which it does not charge", plain.ShapingWork)
	}
	if len(got.Ops) != len(plain.Ops) {
		t.Fatalf("%d ops under a budget, %d without", len(got.Ops), len(plain.Ops))
	}
	for i := range got.Ops {
		// Each composition has its own copy of a face (see
		// documentFonts.own), so a run is compared by everything but which
		// copy it names.
		a, aok := got.Ops[i].(DrawText)
		b, bok := plain.Ops[i].(DrawText)
		if aok != bok {
			t.Fatalf("op %d is a %T under a budget and a %T without", i, got.Ops[i], plain.Ops[i])
		}
		if !aok {
			continue
		}
		a.Face, b.Face = nil, nil
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("op %d differs under a budget:\n%#v\nwithout:\n%#v", i, a, b)
		}
	}
}

// TestComposeContextRefusesWhatItCannotAfford: a document over its budget, a
// run over its length and a context already done are each an error and a
// zero composition, not a page that stopped part way.
func TestComposeContextRefusesWhatItCannotAfford(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		name   string
		ctx    context.Context
		limits shape.RunLimits
		want   error
	}{
		{"a budget of one unit", context.Background(), shape.RunLimits{MaxWork: 1}, shape.ErrRunLimit},
		{"runs of ten bytes", context.Background(), shape.RunLimits{MaxInputBytes: 10}, shape.ErrRunLimit},
		{"a cancelled context", cancelled, shape.RunLimits{}, context.Canceled},
	} {
		in, _ := trialDoc(strings.Repeat("revenue ", 20))
		got, err := ComposeContext(c.ctx, in, Options{}, c.limits)
		if !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
		if got.Root != nil || got.Ops != nil {
			t.Errorf("%s: an error came with a composition", c.name)
		}
	}
}

// TestTheDocumentsOwnFacesAreChargedToo: text set in a face the document's
// @font-face loaded costs the budget, as text in the caller's faces does — a
// budget the caller's faces alone fit in is not enough for both.
func TestTheDocumentsOwnFacesAreChargedToo(t *testing.T) {
	text := strings.Repeat("revenue ", 40)
	callers := Input{HTML: `<p>` + text + `</p>`}
	one, err := ComposeContext(context.Background(), callers, Options{}, shape.RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	in, _ := trialDoc(text)
	both, err := ComposeContext(context.Background(), in, Options{}, shape.RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if both.ShapingWork <= one.ShapingWork {
		t.Fatalf("the paragraph in the document's own face cost nothing: %d against %d",
			both.ShapingWork, one.ShapingWork)
	}
	in, _ = trialDoc(text)
	if _, err := ComposeContext(context.Background(), in, Options{}, shape.RunLimits{MaxWork: one.ShapingWork + 1}); !errors.Is(err, shape.ErrRunLimit) {
		t.Errorf("a budget the caller's paragraph alone fits in set both paragraphs: %v", err)
	}
}

// TestAComposedDocumentsFacesAreUnboundedAfter: the faces a composition's ops
// name are the document's own, as Compose's are, and are off the budget once
// it returns — a backend shaping with them again is not refused by a budget
// that is spent.
func TestAComposedDocumentsFacesAreUnboundedAfter(t *testing.T) {
	in, _ := trialDoc("revenue")
	got, err := ComposeContext(context.Background(), in, Options{}, shape.RunLimits{})
	if err != nil {
		t.Fatal(err)
	}
	faces := 0
	for _, op := range got.Ops {
		dt, ok := op.(DrawText)
		if !ok || dt.Face == nil {
			continue
		}
		faces++
		if _, err := dt.Face.ShapeGlyphsBounded(context.Background(), shape.RunInput{Text: "revenue"}, shape.RunLimits{}); err != nil {
			t.Errorf("a face of the composition is still under its budget: %v", err)
		}
	}
	if faces == 0 {
		t.Fatal("the composition drew no text, so this tests nothing")
	}
}

// TestComposeContextsDefaultsAdmitLongParagraphs: layout shapes a paragraph
// as one run, so a bound on a run's length was a bound on a paragraph's, and
// the defaults refused a paragraph or a <pre> of more than 4 KB however little
// it cost, while the same text cut into short paragraphs composed (issue 923).
// The budget's defaults grow with the text instead; see shape.ShapingBudget.
func TestComposeContextsDefaultsAdmitLongParagraphs(t *testing.T) {
	for _, c := range []struct{ name, html string }{
		{"a paragraph of 8 KB", "<p>" + strings.Repeat("revenue ", 1000) + "</p>"},
		{"a paragraph of 40 KB", "<p>" + strings.Repeat("revenue ", 5000) + "</p>"},
		{"a <pre> of 8 KB in lines of 80", "<pre>" + strings.Repeat(strings.Repeat("x", 79)+"\n", 100) + "</pre>"},
		{"a paragraph of 60 KB of Devanagari", "<p>" + strings.Repeat("क्षत्रिय नमस्ते हिन्दी ", 1000) + "</p>"},
	} {
		got, err := ComposeContext(context.Background(), Input{HTML: c.html}, Options{}, shape.RunLimits{})
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(got.Ops) == 0 {
			t.Errorf("%s composed nothing", c.name)
		}
	}
}
