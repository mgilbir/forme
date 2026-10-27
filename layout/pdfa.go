package layout

import (
	"github.com/mgilbir/forme/internal/ascii"
	"github.com/mgilbir/forme/shape"
)

// A page laid out for PDF/A shows no font it does not embed.
//
// ISO 19005-2 requires every font a conforming file uses to be embedded
// (6.2.11.4.1), so that the file renders in fifty years as it does today. The
// fourteen standard faces are the one kind of font this engine does not embed:
// shape.Standard gives metrics and no program, and a backend writes the face as
// a name for the reader to resolve. They are what StandardFonts answers for the
// three generic families and for the names documents ask for constantly —
// "sans-serif" is Helvetica, "Courier" is Courier — so an ordinary document that
// supplies no fonts of its own is set entirely in them.
//
// Outside PDF/A that is the right default and nothing here runs. Under
// Options.PDFA each standard face the page draws is reported under
// RuleFontNotEmbedded, an Error, and the document is refused, so that the
// caller learns which family to supply before a validator tells them the file
// is not what it claims to be.
//
// # What is checked, and why there
//
// The display list, after painting. Every run of text a page shows is a
// DrawText there, whatever drew it — a line, a list marker, generated content,
// the text of a formula — so a face cannot reach the file without being seen.
// Checking where faces are chosen instead would have to be repeated at each of
// those, and the one that was missed would be the file that failed validation.
//
// Only DrawText is read, and the other ops that carry a face are left out on
// purpose. A text shadow is drawn with its run, which is a DrawText in the
// same face even when the text is transparent; an emphasis mark is set in its
// element's own face or a fallback for it, never a standard face the run did
// not already use; and a stretched operator's glyphs come from a MATH table,
// which no standard face has. Each would be a case no page can reach.
//
// The display list does not say which element a run belongs to, and the
// finding has to name the family the author wrote, since that is what they
// will change. So the laid-out lines are read as well, for the box each face
// was first drawn for. A face no line holds — a marker, a formula's glyphs —
// is still reported, by its name alone.

// checkEmbedded reports every standard face the page draws, once per face.
func checkEmbedded(rec *Recorder, ops []Op, root *Fragment, applied transformsApplied) {
	drawn := standardFacesDrawn(ops)
	if len(drawn) == 0 {
		return
	}
	// The box each standard face was first set for, in document order, so the
	// finding points at an element and quotes its font-family.
	first := map[*shape.Face]*Box{}
	underTransforms(root, applied, func(f *Fragment, _ *affine, _ Clip) {
		for _, line := range f.Lines {
			for _, run := range line.Runs {
				if run.Face == nil || !run.Face.IsStandard() || run.Box == nil {
					continue
				}
				if _, ok := first[run.Face]; !ok {
					first[run.Face] = run.Box
				}
			}
		}
	})
	for _, face := range drawn {
		what := "text on this page is set in " + face.Name()
		var f Finding
		if b, ok := first[face]; ok {
			family := ascii.TrimCSSSpace(b.Style.Get("font-family"))
			what = "the text is set in " + face.Name() + ", which \"font-family: " +
				family + "\" resolved to"
			el := boxElement(b)
			f.Source, f.Path = sourceOf(el), PathOf(el)
		}
		f.Rule = RuleFontNotEmbedded
		f.Message = what + "; that is one of the fourteen standard PDF fonts, which " +
			"are named in the file rather than embedded, and PDF/A requires every " +
			"font to be embedded (ISO 19005-2, 6.2.11.4.1). Supply the family with " +
			"an @font-face rule or in the font set passed to Compose"
		rec.ReportDetail(f)
	}
}

// standardFacesDrawn is every standard face the display list draws text in, in
// the order it is first drawn, each once.
func standardFacesDrawn(ops []Op) []*shape.Face {
	var out []*shape.Face
	seen := map[*shape.Face]bool{}
	note := func(f *shape.Face) {
		if f != nil && f.IsStandard() && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	var walk func([]Op)
	walk = func(ops []Op) {
		for _, op := range ops {
			switch o := op.(type) {
			case DrawText:
				note(o.Face)
			case FilterGroup:
				walk(o.Ops)
			case ClipPath:
				walk(o.Ops)
			case TransformGroup:
				walk(o.Ops)
			}
		}
	}
	walk(ops)
	return out
}
