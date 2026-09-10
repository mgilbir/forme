package layout

import (
	"unicode"

	"github.com/mgilbir/forme/paragraph"
	"github.com/mgilbir/forme/shape"
	"github.com/mgilbir/forme/style"
)

// Small capitals made out of the capitals, for a face that drew none.
//
// CSS Fonts 4 §6.6 lets a user agent do this and does not say how: "if the font
// does not support small-caps glyphs, the user agent may synthesize small-caps
// by scaling uppercase glyphs". It is what every browser does, and it is what
// the great majority of documents get — the fourteen standard PDF faces declare
// no OpenType feature at all, so "font-variant: small-caps" over a page set in
// the default serif has no 'smcp' to ask for and never will.
//
// # It is a cut, not a transform
//
// The lowercase letters are set at a smaller size than the rest, so a run that
// holds both is no longer one run: it is cut where the case changes, and each
// piece measured and drawn at its own size. That is why this is here rather than
// beside text-transform in the box tree, which changes the text of a whole node
// and cannot change its size.
//
// The cut is made after every other cut a piece takes — the faces, §8.1's
// autospace gaps, the cursive tracking, the word separators — because it is the
// only one that rewrites the text it cuts. Everything before it works in offsets
// into the piece, and "straße" uppercases to "STRASSE", which is a byte longer.
//
// # What the reader gets out of the PDF
//
// The letters drawn. A page whose small capitals were synthesised carries
// "FILLER" where the document said "Filler", and a reader copying it out gets
// the capitals. That is the same consequence text-transform has and for the same
// reason — a PDF has only what was drawn, where a browser still has the DOM
// beside the rendering — and it is worse here, because the author asked for
// small capitals rather than for uppercase. Carrying the original as well needs
// /ActualText on a marked-content span around each synthesised run, which is a
// change to how text is emitted rather than an addition to it. It is the reason
// the finding is raised even though the page is right.
//
// # Why the scale comes from the face
//
// A small capital is a capital cut to about the height of the lowercase
// letters, so the face states the number: its x-height over its cap height.
// Every face in the checkout puts that between 0.68 and 0.80 — Times 0.680,
// Helvetica 0.728, Noto Sans 0.751, Courier 0.758 — which is the band the
// browsers' constants sit in, Blink at 0.7 and Gecko at 0.8. Taking it from the
// face rather than picking one of those means a face with unusually large
// lowercase letters gets synthesised capitals that match them, which is the
// whole point of the shape.

// The band a synthesised capital is scaled into, and the number used where the
// face states nothing to derive one from.
//
// The clamp is not for the faces here — every one of them lands inside it — but
// for the arithmetic: a face declaring an x-height above its cap height would
// otherwise produce small capitals larger than the capitals, and one declaring
// a tiny x-height would produce a line of specks. Neither is a page worth
// drawing, and neither is a font this engine can refuse to load.
const (
	minSmallCapScale     = 0.5
	maxSmallCapScale     = 0.9
	defaultSmallCapScale = 0.75
)

// smallCapScale is how far a synthesised capital is shrunk in this face.
func smallCapScale(face *shape.Face) float64 {
	if face == nil {
		return defaultSmallCapScale
	}
	d := face.Descriptor()
	// Read as numbers rather than through Declared: the fourteen standard faces
	// carry a cap height from their AFM and do not set MetricCapHeight, which
	// is about what an *embedded* font's descriptor may state. See
	// layout/textdecoration.go, which asks the same question the same way.
	if d.CapHeight <= 0 || d.XHeight <= 0 {
		return defaultSmallCapScale
	}
	scale := float64(d.XHeight) / float64(d.CapHeight)
	if scale < minSmallCapScale {
		return minSmallCapScale
	}
	if scale > maxSmallCapScale {
		return maxSmallCapScale
	}
	return scale
}

// synthesisedSize is the size a run is set at.
func synthesisedSize(size style.Unit, run faceRun) style.Unit {
	if !run.synthesised {
		return size
	}
	return size.Mul(smallCapScale(run.Face))
}

// capsAreSynthesised reports whether this engine will fake a value that the face
// cannot carry out.
//
// "small-caps" and nothing else so far. §6.6's other five are left to the face:
// "all-small-caps" needs the capitals lowered as well, which is the same cut
// again over the other case; the two petite values fall back to small capitals
// before anything is synthesised; and "unicase" and "titling-caps" are not a
// letter drawn smaller at all, so there is nothing to scale a capital into.
func capsAreSynthesised(want shape.Caps) bool {
	return want == shape.CapsSmall
}

// synthesisesFor reports whether this face will have to fake the value, which is
// the question "does it declare what the value asks for" turned round.
func synthesisesFor(want shape.Caps, face *shape.Face) bool {
	if face == nil || !capsAreSynthesised(want) {
		return false
	}
	for _, tag := range want.Features() {
		if faceDeclares(face, tag) {
			return false
		}
	}
	return true
}

// smallCapsRuns cuts runs where synthesis begins and ends, and rewrites the
// stretches this engine will set as capitals.
//
// A run comes back marked rather than resized, because the size depends on the
// face and the face is on the run: see synthesisedSize, which is asked once
// where the item is built.
//
// The runs that are *not* rewritten come back untouched, which is every run of
// every document that does not ask for small capitals and every run set in a
// face that has them.
func (l *layouter) smallCapsRuns(b *Box, runs []faceRun, want shape.Caps,
	lang paragraph.Language) []faceRun {

	if !capsAreSynthesised(want) {
		return runs
	}
	var out []faceRun
	changed := false
	for _, run := range runs {
		if !synthesisesFor(want, run.Face) {
			out = append(out, run)
			continue
		}
		parts := cutAtCase(run.Text)
		if len(parts) == 1 && !parts[0].lowered {
			out = append(out, run)
			continue
		}
		changed = true
		for _, part := range parts {
			next := faceRun{Text: part.text, Face: run.Face, substituted: run.substituted}
			if part.lowered {
				// The same case mapping text-transform uses, so that a page
				// where both apply cannot disagree with itself: the full
				// mappings rather than Go's simple ones — "straße" is "STRASSE"
				// — and the language tailorings with them.
				next.Text, _ = transformText(part.text, paragraph.TransformUppercase, false, lang)
				next.synthesised = true
			}
			out = append(out, next)
		}
	}
	if !changed {
		// Nothing was rewritten, so the slice built above is a copy of the one
		// that came in and the caller may as well keep the original.
		return runs
	}
	return out
}

// casePart is a stretch of text that is all synthesised or all not.
type casePart struct {
	text string
	// lowered says the stretch is what small capitals replace: characters with
	// an uppercase form of their own.
	//
	// Not unicode.IsLower, which is true of characters no face maps anywhere,
	// and not "is a letter", which is true of the scripts that have one case
	// only. What 'smcp' covers is a letter with a capital to be replaced by, and
	// having an uppercase mapping is exactly that.
	lowered bool
}

// cutAtCase splits text into maximal stretches that are all lowercase or all
// not.
//
// Maximal, because every boundary costs a run: a run is measured, shaped and
// drawn on its own, and a word cut into one run per letter loses every kern and
// every ligature inside it. "Filler Text" is four stretches and not eleven.
func cutAtCase(text string) []casePart {
	var out []casePart
	start, cur := 0, false
	for i, r := range text {
		lowered := unicode.ToUpper(r) != r
		if i == 0 {
			cur = lowered
			continue
		}
		if lowered == cur {
			continue
		}
		out = append(out, casePart{text: text[start:i], lowered: cur})
		start, cur = i, lowered
	}
	if start < len(text) || len(out) == 0 {
		out = append(out, casePart{text: text[start:], lowered: cur})
	}
	return out
}
