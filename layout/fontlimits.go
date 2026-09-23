package layout

import (
	"fmt"

	"github.com/mgilbir/forme/shape"
)

// What reading a face's rules ran into.
//
// A font's layout tables are read under bounds — so much flattening per table,
// so many kerning pairs — because a font is untrusted input and a hundred
// kilobytes of it can ask for a billion glyphs. Where a bound trips, the part
// of the table past it is not read, and text set in the face is set without
// those rules: it is legible and it is not what the font says. shape records
// each such refusal on the face (Face.LayoutLimits), and this is where a
// document is told: a font refused in part is a page that differs from the one
// every other reader draws, and saying so is the difference between a hostile
// font and a defect nobody can find.
//
// It is asked once layout is done: a face's tables are read at load and again
// per script and language as text is first set in them, and LayoutLimits
// answers for all of it. And it is asked of the faces the document set text in
// — the face each family resolved to and every fallback that set a run — so a
// font the document named and never used is not reported.

// noteFace records a face text is set in.
func (l *layouter) noteFace(f *shape.Face) {
	if f == nil || l.facesSeen[f] {
		return
	}
	if l.facesSeen == nil {
		l.facesSeen = map[*shape.Face]bool{}
	}
	l.facesSeen[f] = true
	l.facesUsed = append(l.facesUsed, f)
}

// reportFontLimits reports what reading each face's rules ran into, once per
// face and bound.
func (l *layouter) reportFontLimits() {
	for _, f := range l.facesUsed {
		for _, m := range f.LayoutLimits() {
			l.reportOnce("font-limit:"+f.Name()+":"+m, Finding{
				Rule:   RuleLimit,
				Source: NoSource,
				Message: fmt.Sprintf("the font %s is set without part of its rules: %s",
					quoteValue(f.Name()), m),
			})
		}
	}
}
