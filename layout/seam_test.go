package layout

import (
	"strconv"
	"strings"
	"testing"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Whether the items forme/paragraph is tested against are the items this engine
// builds.
//
// paragraph's own tests construct items by hand — text, a face, a width, some
// flags — and prove things about how they break. Every one of those proofs is
// worth exactly as much as the resemblance between those items and the ones a
// document produces, and nothing over there can check it: the package cannot see
// a box tree, which is the whole point of it.
//
// So the check belongs here, where both halves are visible. A document is laid
// out through the engine, the same text is put through paragraph directly, and
// the two are required to break to the same lines. If they ever part, one of two
// things is true and both are worth knowing: this engine is doing something to
// its items that the model does not, or the model has drifted and paragraph's
// tests are proving things about a shape nobody builds.
//
// What this cannot catch, and is not for: a fault in the breaking itself. Both
// sides call the same BreakOneLine, so a breaker that filled every line to four
// fifths of its measure would move both of them together and this would still
// pass — planted, it does. The breaking is what paragraph's own invariants are
// for; what they cannot reach is whether the items are real, and that is this.

// paragraphLines breaks a run of text through forme/paragraph alone, with no
// document anywhere: the same construction paragraph's own tests use.
func paragraphLines(t *testing.T, face *shape.Face, text string, size, width style.Unit,
	ws paragraph.WhiteSpace) []string {

	t.Helper()
	br := paragraph.NewBreaker(nil)
	pieces, _ := paragraph.SplitAtBreaks(text, ws, paragraph.WordBreak{}, paragraph.LineBreak{})

	items := make([]paragraph.Item, 0, len(pieces))
	afterCollapsible := true
	for _, p := range pieces {
		if p.Segment {
			items = append(items, paragraph.Item{Face: face, Size: size, Forced: true})
			afterCollapsible = true
			continue
		}
		if p.Collapsible && afterCollapsible {
			continue
		}
		it := paragraph.Item{
			Text: p.Text, Face: face, Size: size,
			BreakBefore: p.BreakBefore,
			Space:       p.Space, Collapsible: p.Collapsible, TrimAtEnd: p.TrimAtEnd,
			Tab:       p.Tab,
			Hangs:     p.Space && !p.Collapsible && !ws.BreakSpaces && (ws.Collapse || ws.Wrap),
			HangsHard: p.Space && !p.Collapsible && ws.Collapse,
			NoWrap:    !ws.Wrap,
		}
		if !p.Tab {
			it.Width = br.MeasureSpaced(face, p.Text, size, paragraph.TextSpacing{})
		}
		items = append(items, it)
		afterCollapsible = p.Collapsible
	}

	var out []string
	from, fromByte := 0, 0
	for from < len(items) {
		line, next, nextByte, _, _ := br.BreakOneLine(items, from, fromByte, width, 0)
		var b strings.Builder
		for _, it := range line {
			b.WriteString(it.Text)
		}
		out = append(out, b.String())
		if !paragraph.CursorAdvanced(from, fromByte, next, nextByte) {
			break
		}
		from, fromByte = next, nextByte
		if len(out) > len(items)+2 {
			t.Fatalf("breaking %q directly did not terminate", text)
		}
	}
	return out
}

// TestTheEngineBreaksAParagraphWhereParagraphDoes is the seam checked from the
// outside.
//
// The texts are the awkward ones rather than the ordinary ones, because the two
// constructions agree trivially on "one two three" and part, if they part
// anywhere, on a trailing space or a run of ideographs.
func TestTheEngineBreaksAParagraphWhereParagraphDoes(t *testing.T) {
	face, err := shape.Standard("Courier")
	if err != nil {
		t.Fatalf("loading Courier: %v", err)
	}
	size := mustPx(20)

	for _, text := range []string{
		"the quick brown fox jumps over the lazy dog",
		"a b supercalifragilisticexpialidocious c d",
		"one  two   three",
		"trailing space at the end ",
		"日本語のテキストです and some English",
		"one two three four five six seven eight nine ten eleven twelve",
	} {
		for _, width := range []float64{30, 60, 100, 200, 400} {
			// The document is stripped of everything that would add a rule the
			// model does not have: no margins, no borders, one face, one size.
			css := noDefaults + `#p { font-family: Courier; font-size: 20px; width: ` +
				fmtFloat(width) + `px }`
			root := layoutOf(t, 800, `<div id="p">`+text+`</div>`, css)
			f := find(t, root, "p")

			var engine []string
			for _, line := range f.Lines {
				engine = append(engine, strings.Join(runTexts(line), ""))
			}
			direct := paragraphLines(t, face, text, size, mustPx(width),
				paragraph.WhiteSpaceOf("collapse"))

			if len(engine) != len(direct) {
				t.Errorf("%q at %gpx: the engine set %d lines and paragraph alone set %d\n"+
					"  engine:    %q\n  paragraph: %q",
					text, width, len(engine), len(direct), engine, direct)
				continue
			}
			for i := range engine {
				if engine[i] != direct[i] {
					t.Errorf("%q at %gpx: line %d reads %q through the engine and %q "+
						"through paragraph alone — the items paragraph is tested against "+
						"are not the items this engine builds",
						text, width, i, engine[i], direct[i])
				}
			}
		}
	}
}

// fmtFloat writes a width into a stylesheet as a plain decimal.
func fmtFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
