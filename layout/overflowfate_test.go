package layout

import (
	"strings"
	"testing"

	"github.com/mgilbir/forme/style"
)

// A finding about content that leaves its box says what became of it, and what
// it says is what the display list did.
//
// Two rules report this — the unbreakable run of §6.2 and the fixed table
// column of §17.5.2.1 — and both used to say "the part past the edge will not be
// drawn". That is one of the two answers. "overflow" is the property, its
// initial value is "visible", and a box whose overflow is visible draws its
// content wherever the content goes: every glyph is on the page, over whatever
// was beside it.
//
// Telling an author their text was cut off when it was drawn over the next
// column is wrong twice — they look for missing words and find them all, and
// they never look for the thing that is actually wrong.
//
// The assertion is not about the wording. It is that the wording and the
// drawing agree: where the finding says the text is cut off there is a clip on
// the run, and where it says the text is drawn there is none and the run reaches
// past the box.
func TestAnOverflowFindingSaysWhatTheDisplayListDid(t *testing.T) {
	for _, tc := range []struct {
		css, what string
		clipped   bool
	}{
		{"", "overflow left at its initial value", false},
		{"overflow: visible", "overflow: visible", false},
		{"overflow: hidden", "overflow: hidden", true},
		{"overflow: scroll", "overflow: scroll", true},
	} {
		frag, findings := bgLayoutWithFindings(t,
			`<div id="d">WWWWWWWWWW</div>`,
			noDefaults+`#d { width: 60px; font-family: Courier; font-size: 20px; `+tc.css+` }`)

		var msg string
		for _, f := range findings {
			if f.Rule == RuleUnbreakableOverflow {
				msg = f.Message
			}
		}
		if msg == "" {
			t.Errorf("%s: ten W's in a box six wide reported no overflow at all: %v",
				tc.what, findings)
			continue
		}

		var runs []DrawText
		for _, op := range Paint(frag) {
			if d, ok := op.(DrawText); ok {
				runs = append(runs, d)
			}
		}
		if len(runs) != 1 {
			t.Fatalf("%s: %d runs drawn, want one", tc.what, len(runs))
		}
		run := runs[0]

		saysCut := strings.Contains(msg, "is not drawn")
		saysDrawn := strings.Contains(msg, "is drawn past the edge")
		if saysCut == saysDrawn {
			t.Errorf("%s: the finding says neither or both of the two things: %q",
				tc.what, msg)
			continue
		}
		if saysCut != tc.clipped {
			t.Errorf("%s: the finding says %q", tc.what, msg)
		}
		// And where it says something clips, it names what: an author looking
		// for the declaration has to be told which element carries it, and the
		// box the text is in is an anonymous one that carries nothing.
		if tc.clipped && !strings.Contains(msg, "on <div> clips it") {
			t.Errorf("%s: the finding does not name the element that clips: %q",
				tc.what, msg)
		}
		if run.Clip.Active != tc.clipped {
			t.Errorf("%s: the run is drawn with clip %v and the finding says %q",
				tc.what, run.Clip, msg)
		}
		// And the run really does reach past the box, which is what makes the
		// two answers different rather than two names for one. Ten characters of
		// a monospace face at 20px are 120px wide and the box is 60.
		w, _ := style.FromPx(run.Face.Measure(run.Text, run.Size.Px()))
		if end := run.At.X.Add(w); end <= mustPx(68) {
			t.Errorf("%s: the run ends at %v, inside the 60px box at x=8 — there "+
				"is no overflow here to report", tc.what, end)
		}
	}
}

// TestAnOverflowFindingNamesOnlyAClipThatApplies: which box clips is asked
// the way the clip itself is resolved. An absolutely positioned box with no
// positioned ancestor is clipped by nothing on the page, however many
// "overflow: hidden" boxes it was written inside (§11.1.1), and its run is drawn
// whole; the finding said an ancestor outside its containing block chain cut it
// off. And a clipping ancestor far wider than the text does not make the text
// cut: the finding says what reaches past that box's edge is cut, and that the
// rest runs past its own box's edge.
func TestAnOverflowFindingNamesOnlyAClipThatApplies(t *testing.T) {
	const css = noDefaults + `#w { overflow: hidden; width: 600px }
		#t { width: 50px; font-family: Courier; font-size: 20px }`
	message := func(markup string) (string, []DrawText) {
		t.Helper()
		frag, findings := bgLayoutWithFindings(t, markup, css)
		var msg string
		for _, f := range findings {
			if f.Rule == RuleUnbreakableOverflow {
				msg = f.Message
			}
		}
		if msg == "" {
			t.Fatalf("%s: no overflow was reported: %v", markup, findings)
		}
		var runs []DrawText
		for _, op := range Paint(frag) {
			if d, ok := op.(DrawText); ok {
				runs = append(runs, d)
			}
		}
		return msg, runs
	}

	msg, runs := message(`<div id="w"><div id="t" style="position: absolute">WWWWWW</div></div>`)
	if strings.Contains(msg, "clips") || !strings.Contains(msg, "is drawn past the edge") {
		t.Errorf("an absolutely positioned box outside every clip's chain was reported as %q", msg)
	}
	for _, r := range runs {
		if r.Clip.Active {
			t.Errorf("the run %q carries a clip, so this fixture is not the case", r.Text)
		}
	}

	// "overflow" does not apply to an inline box, and a text box carries its
	// element's whole style: neither the span nor the text in it clips.
	msg, _ = message(`<div id="t"><span style="overflow: hidden">WWWWWW</span></div>`)
	if strings.Contains(msg, "clips") {
		t.Errorf("an inline box was named as clipping its own text: %q", msg)
	}

	msg, runs = message(`<div id="w"><div id="t">WWWWWW</div></div>`)
	if !strings.Contains(msg, "runs past the edge") || !strings.Contains(msg, "on <div> clips it") {
		t.Errorf("text inside a wide clipping box was reported as %q; it runs past its "+
			"own box and is cut only where it reaches the clipping one", msg)
	}
	for _, r := range runs {
		if r.Clip.Active {
			t.Errorf("the run %q carries a clip, so this fixture is not the case", r.Text)
		}
	}
}
