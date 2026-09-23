package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// Three unsupported-value reports that nothing had ever raised.
//
// Each names a value the engine reads and does not act on, so that a document
// asking for it is told rather than silently given something else. All three
// were at 0% coverage with the reftest corpus on as well as off: the corpus
// writes these properties only with values this engine implements, so the
// reporting arm of each had never run.
//
// A report that has never been raised is worth no more than one that is not
// there. The value it carries is entirely in what it says on the day a document
// asks for something unimplemented, and that is the day nobody is watching —
// which is the argument for raising each one here, deliberately, and reading the
// message back.

// findingsFor lays a document out and returns what was reported.
func findingsFor(t *testing.T, html, css string) []Finding {
	t.Helper()
	built := Build(Input{HTML: html, CSS: []Stylesheet{{Source: css}}})
	if built.Root == nil {
		t.Fatal("the document produced no boxes")
	}
	rec := NewRecorder(nil)
	w, _ := style.FromPx(600)
	h, _ := style.FromPx(10000)
	Layout(built.Root, Size{W: w, H: h}, nil, rec)
	return rec.Findings()
}

// unsupportedValueFor returns the message of the first unsupported-value
// finding mentioning the given text, and says so when there is none.
func unsupportedValueFor(t *testing.T, findings []Finding, mentions string) string {
	t.Helper()
	for _, f := range findings {
		if f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, mentions) {
			return f.Message
		}
	}
	return ""
}

// text-autospace's other half. §8.1 gives the property four spacing kinds and
// this engine inserts spacing for two of them; "punctuation" and "replace" are
// read, understood and not acted on.
//
// The grammar allows several at once, and the reader takes the first unhandled
// word so that a document naming two gets one finding rather than a list. That
// is worth pinning: a reader that appended would report the same declaration
// twice over, and one that reported the last would name a different word each
// time the author reordered a value that means the same thing.
func TestAnAutospaceThisEngineDoesNotInsertIsReported(t *testing.T) {
	for _, c := range []struct{ value, want string }{
		{"punctuation", "punctuation"},
		{"replace", "replace"},
		{"ideograph-alpha punctuation", "punctuation"},
		{"punctuation replace", "punctuation"},
		{"replace punctuation", "replace"},
	} {
		t.Run(c.value, func(t *testing.T) {
			got := unsupportedValueFor(t,
				findingsFor(t, `<p id="p">日本語とABC</p>`,
					`#p { text-autospace: `+c.value+` }`), c.want)
			if got == "" {
				t.Fatalf("text-autospace: %s was not reported; the engine reads "+
					"it and inserts no spacing for it, so a document asking for "+
					"it is given ideograph spacing and told nothing", c.value)
			}
		})
	}

	// And the values it does act on are not reported, which is what keeps the
	// finding worth reading.
	for _, value := range []string{"normal", "no-autospace", "ideograph-alpha",
		"ideograph-numeric", "ideograph-alpha ideograph-numeric", "ideograph-alpha insert"} {
		t.Run("quiet: "+value, func(t *testing.T) {
			for _, f := range findingsFor(t, `<p id="p">日本語とABC</p>`,
				`#p { text-autospace: `+value+` }`) {
				if f.Rule == RuleUnsupportedValue && f.Property == "text-autospace" {
					t.Errorf("text-autospace: %s was reported as unsupported: %s",
						value, f.Message)
				}
			}
		})
	}
}

// text-fit's grammar is a mode, a granularity and a limit, and a word that is
// none of the three makes the declaration not a text-fit at all — so nothing is
// scaled and nothing is claimed.
//
// The report is what tells the two apart from outside. "text-fit: grow 50%"
// scales and says nothing; "text-fit: grow sideways" scales nothing, and without
// the finding the only difference a caller can see is that the text came out the
// size it already was.
func TestATextFitThisEngineCannotReadIsReported(t *testing.T) {
	for _, value := range []string{"grow sideways", "shrink upside-down", "wobble"} {
		t.Run(value, func(t *testing.T) {
			// Not CSS, so it is the cascade that drops it and says so; see
			// TestAValueThatIsNotCSSIsDroppedByTheCascade, which this asks too.
			got := ""
			if droppedAsInvalid(t, `<p id="p">text that is long enough to want fitting</p>`,
				`#p { width: 200px; text-fit: `+value+` }`, "text-fit") {
				got = "reported"
			}
			if got == "" {
				t.Fatalf("text-fit: %s was not reported; the word is not in the "+
					"grammar, so nothing was scaled and the document was not told",
					value)
			}
		})
	}

	for _, value := range []string{"none", "grow", "shrink", "grow per-line",
		"shrink 50%", "grow consistent"} {
		t.Run("quiet: "+value, func(t *testing.T) {
			for _, f := range findingsFor(t, `<p id="p">text that is long enough to want fitting</p>`,
				`#p { width: 200px; text-fit: `+value+` }`) {
				if f.Rule == RuleUnsupportedValue && strings.Contains(f.Message, "text-fit") {
					t.Errorf("text-fit: %s was reported as unsupported: %s",
						value, f.Message)
				}
			}
		})
	}
}
