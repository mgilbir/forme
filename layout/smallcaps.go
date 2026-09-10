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

// capsAreSynthesised reports whether a value is one this engine will fake where
// the face cannot carry it out.
//
// The four that are a letter drawn smaller: "small-caps" and "all-small-caps",
// and the two petite values, which §6.6 sends to the small capitals first and
// which are synthesised the same way when those are absent too — a petite
// capital and a small capital differ in how far the designer cut them down, and
// a synthesised one has no designer.
//
// "unicase" and "titling-caps" are not. Neither replaces a letter with a
// smaller one: unicase asks for a face's own single-height forms of both cases,
// and titling capitals are cut lighter for a line that is all capitals. There is
// nothing to scale a capital into, so a face without them is reported.
func capsAreSynthesised(want shape.Caps) bool {
	switch want {
	case shape.CapsSmall, shape.CapsAllSmall, shape.CapsPetite, shape.CapsAllPetite:
		return true
	}
	return false
}

// resolveCaps is §6.6's one fallback between values: "if petite capital glyphs
// are not available, small capital glyphs are used".
//
// Read at the value rather than at the tag, which is the sentence as written: a
// face is asked whether it has *petite capitals*, and one that has none of them
// is asked for small ones instead. A per-tag reading — this half petite and that
// half small — would produce a line in two designs, which is not what either
// value means.
//
// Everything else is itself. Small capitals do not fall back to petite ones:
// §6.6 states the chain in one direction, and it is the direction where the
// substitute is the commoner cut.
func resolveCaps(want shape.Caps, face *shape.Face) shape.Caps {
	if face == nil {
		return want
	}
	var instead shape.Caps
	switch want {
	case shape.CapsPetite:
		instead = shape.CapsSmall
	case shape.CapsAllPetite:
		instead = shape.CapsAllSmall
	default:
		return want
	}
	if declaresAnyOf(face, want.Features()) || !declaresAnyOf(face, instead.Features()) {
		// It has petite capitals, or it has neither and there is nothing to
		// fall back to — in which case the value stays what the document wrote,
		// so that the report names what was asked for.
		return want
	}
	return instead
}

// declaresAnyOf reports whether a face offers any of a set of features.
func declaresAnyOf(face *shape.Face, tags []string) bool {
	for _, tag := range tags {
		if faceDeclares(face, tag) {
			return true
		}
	}
	return false
}

// synthesisedCases is which of a value's two halves this face cannot carry out,
// and so which this engine will make itself.
//
// The halves are independent because the features are: a face may declare
// 'smcp' and not 'c2sc', which is most of "all-small-caps" done and the capitals
// left standing at full height. What the synthesis then has to do is the *other*
// half, over the letters the face did not cover — and nothing to the letters it
// did, or they would be lowered twice.
func synthesisedCases(want shape.Caps, face *shape.Face) (lower, capitals bool) {
	if face == nil || !capsAreSynthesised(want) {
		return false, false
	}
	if tag := want.Lowercase(); tag != "" && !faceDeclares(face, tag) {
		lower = true
	}
	if tag := want.Capitals(); tag != "" && !faceDeclares(face, tag) {
		capitals = true
	}
	return lower, capitals
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
		lower, capitals := synthesisedCases(resolveCaps(want, run.Face), run.Face)
		if !lower && !capitals {
			out = append(out, run)
			continue
		}
		parts := cutAtCase(run.Text)
		if !anySynthesised(parts, lower, capitals) {
			out = append(out, run)
			continue
		}
		changed = true
		at := len(out)
		for _, part := range parts {
			next := faceRun{Text: part.text, Face: run.Face, substituted: run.substituted}
			switch {
			case part.kind == caseLower && lower:
				// The same case mapping text-transform uses, so that a page
				// where both apply cannot disagree with itself: the full
				// mappings rather than Go's simple ones — "straße" is "STRASSE"
				// — and the language tailorings with them.
				next.Text, _ = transformText(part.text, paragraph.TransformUppercase, false, lang)
				next.synthesised = true
			case part.kind == caseUpper && capitals:
				// Already the right letter, and the wrong size. "all-small-caps"
				// lowers the capitals as well, and a capital lowered is the same
				// capital drawn smaller — there is nothing to rewrite.
				next.synthesised = true
			}
			// Joined to the stretch before it where the two are set the same
			// way, which is what makes the cut fall at the boundaries that
			// matter rather than at every change of case.
			//
			// Three of them are not boundaries at all. Under "small-caps" a
			// capital and the space beside it are both left alone; under
			// "all-small-caps" a capital and the lowercase letters beside it
			// are both shrunk, and "Filler" is one run and not two. Every
			// boundary costs a measurement, a shaping and a place a kern
			// cannot cross, so the ones that buy nothing are not made.
			if len(out) > at && out[len(out)-1].synthesised == next.synthesised {
				out[len(out)-1].Text += next.Text
				continue
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

// anySynthesised reports whether any of a run's stretches is one of the halves
// this face cannot carry out.
//
// Asked before the run is rebuilt, so that a run with nothing to do to it comes
// back as itself: "1234" under "all-small-caps" has no letter of either case in
// it, and cutting it into pieces that are all set the same way would cost a
// measurement and a shaping for nothing.
func anySynthesised(parts []casePart, lower, capitals bool) bool {
	for _, part := range parts {
		if (part.kind == caseLower && lower) || (part.kind == caseUpper && capitals) {
			return true
		}
	}
	return false
}

// casePart is a stretch of text whose characters are all of one case.
type casePart struct {
	text string
	kind caseKind
}

// The three kinds of character §6.6's features tell apart.
//
// Not unicode.IsLower and IsUpper, which are true of characters no face maps
// anywhere, and not "is a letter", which is true of the scripts that have one
// case only. What these features cover is a letter with a form of the other case
// to be replaced by, and having a case mapping is exactly that.
//
// caseNone is the third and is why this is not a bool. A space, a digit and a
// full stop are not lowered by "small-caps" and are not *shrunk* by
// "all-small-caps" either — a line whose spaces were three-quarters of a space
// wide would be spaced wrong between every pair of words.
type caseKind uint8

const (
	caseNone caseKind = iota
	caseLower
	caseUpper
)

func caseOf(r rune) caseKind {
	switch {
	case unicode.ToUpper(r) != r:
		return caseLower
	case unicode.ToLower(r) != r:
		return caseUpper
	}
	return caseNone
}

// cutAtCase splits text into maximal stretches of one case.
//
// Maximal, because every boundary costs a run: a run is measured, shaped and
// drawn on its own, and a word cut into one run per letter loses every kern and
// every ligature inside it. "Filler Text" is five stretches and not eleven.
//
// Five and not four, though only four of the boundaries can matter to any one
// value: the caller joins back the stretches its value sets the same way. What
// this has to produce is every boundary that *could* be one, which is a change
// of case wherever it falls.
func cutAtCase(text string) []casePart {
	var out []casePart
	start := 0
	var cur caseKind
	for i, r := range text {
		kind := caseOf(r)
		if i == 0 {
			cur = kind
			continue
		}
		if kind == cur {
			continue
		}
		out = append(out, casePart{text: text[start:i], kind: cur})
		start, cur = i, kind
	}
	if start < len(text) || len(out) == 0 {
		out = append(out, casePart{text: text[start:], kind: cur})
	}
	return out
}
