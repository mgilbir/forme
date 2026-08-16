package layout

import (
	"github.com/mgilbir/forme/segment"
	"github.com/mgilbir/forme/shape"
)

// Falling back from one face to the next *within* a run of text.
//
// faceForText answers per box: it asks whether the family's face can set the
// whole of the box's text and, if not, whether some other face can set the whole
// of it instead. That is the right answer for a box written in one script and
// the wrong one for the case this file exists for — a box written in one script
// with a word or a letter of another in it, which is what almost every document
// with a citation, a name or a technical term in it is.
//
// It is the wrong answer twice over. No single face covers "the following two
// lines (Force bidi: א)": the pan-Unicode face this engine falls back to has
// every character but the alef, and the Hebrew face has the alef and no Latin at
// all. So the whole-box question has no answer, the box keeps the family's face,
// and the alef is set as a space and reported missing. Twenty-five of the
// reftest suite's documents are exactly that sentence.
//
// # What is not done here
//
// The unit of choice is a grapheme cluster and not a character, because a
// combining mark positioned by a font that never saw its base lands at the
// origin. It is not a *word*: a run of Latin inside a Hebrew sentence is chosen
// per cluster like everything else, and the clusters merge into one run because
// they chose the same face.
//
// Shaping still happens per run, so a ligature or a kern that would have crossed
// a face boundary is lost. Nothing can be done about that — the two glyphs are
// in different fonts — and it is why the split is made as coarsely as possible:
// maximal stretches, and a single run whenever one face can set everything.

// faceRun is a stretch of text and the face it is set in.
type faceRun struct {
	Text string
	Face *shape.Face
}

// faceRunsFor splits text into the stretches each face can set.
//
// The primary face is preferred everywhere it can be used, so a document that
// does not mix scripts gets exactly one run holding exactly the text it started
// with — which is what makes this change invisible to every document that was
// already right.
func (l *layouter) faceRunsFor(b *Box, primary *shape.Face, text string) []faceRun {
	one := []faceRun{{Text: text, Face: primary}}
	if text == "" || primary == nil {
		return one
	}
	// A control character is cut out of the text around it whatever the faces
	// say, because what is drawn for it is not a glyph from any face — see
	// controlchar.go. It reaches painting as a run of its own or not at all.
	if !missesVisible(primary, text) && !hasVisibleControl(text) {
		return one
	}
	// A missing fallback set is not a reason to stop: the control-character cut
	// below does not need one, and a caller with no fallback faces still gets a
	// visible glyph for a character no face has.
	set, canFall := l.fontSet.(FallbackFontSet)
	if !canFall && !hasVisibleControl(text) {
		return one
	}
	bold := isBold(b.Style["font-weight"])
	italic := isItalic(b.Style["font-style"])

	// The cluster starts, so every cluster is [at[i], at[i+1]).
	//
	// segment.Boundaries gives the offsets *inside* the string, deliberately
	// leaving out both ends because neither is a place a caller may cut. Both
	// ends are exactly what is wanted here, where the question is not where to
	// cut but which cluster each byte belongs to — and leaving the leading zero
	// off skips the first cluster of every run, which is a whole word when the
	// run is one word long.
	at := append([]int{0}, append(segment.Boundaries(nil, text), len(text))...)

	var runs []faceRun
	// The stretch being accumulated, and the face it is going to.
	start, cur := 0, primary
	flush := func(end int) {
		if end > start {
			runs = append(runs, faceRun{Text: text[start:end], Face: cur})
		}
		start = end
	}
	for i := 0; i+1 < len(at); i++ {
		lo, hi := at[i], at[i+1]
		cluster := text[lo:hi]
		want := primary
		if _, isControl := controlOf(cluster); isControl {
			// Its own run, always: it is neither the face before it nor the
			// face after it, and the painter recognises it by being alone.
			flush(lo)
			runs = append(runs, faceRun{Text: cluster, Face: primary})
			start, cur = hi, primary
			continue
		}
		if canFall && missesVisible(primary, cluster) {
			// This cluster is not one the primary face can set. Ask for a face
			// that can — for the cluster alone, because asking for the rest of
			// the text would be the whole-box question again and would have the
			// same non-answer.
			if alt, found := set.FaceFor(cluster, bold, italic); found {
				want = alt
			}
			// Not found: the cluster stays with the primary face and is
			// reported missing by checkGlyphs, which is what happened before
			// this file existed and is still the right answer — there is no
			// face to move it to.
		}
		if want != cur {
			flush(lo)
			cur = want
		}
	}
	flush(len(text))
	if len(runs) == 0 {
		return one
	}
	if len(runs) == 1 && runs[0].Face == primary {
		return one
	}
	return runs
}
