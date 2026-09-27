package layout

import (
	"github.com/mgilbir/forme/internal/ascii"
)

// css-will-change 1: will-change.
//
// # What a hint does to a page
//
// will-change tells a browser what is about to change, and almost all of what
// it asks for is preparation: nothing on a page rendered once changes. But §3
// gives it two effects on the rendering itself, which hold whether or not
// anything ever changes:
//
//	If any non-initial value of a property would create a stacking context
//	on the element, specifying that property in will-change must create a
//	stacking context on the element.
//
//	If any non-initial value of a property would cause the element to
//	generate a containing block for absolutely positioned elements,
//	specifying that property in will-change must cause the element to
//	generate a containing block for absolutely positioned elements.
//
// and the same again for fixed positioned elements. So "will-change: filter"
// makes a box the stacking context and the containing block a filter makes it,
// with nothing filtered.
//
// # What is done
//
// A will-change naming filter is both, here as it is for a filter: see
// filterContains and stacksAsAFilter. A name whose property this engine makes
// neither from — transform, opacity and the others in willChangeEffects — is
// reported, once per name, since the stacking context or the containing block
// it asks for is not made. A name of a property that makes neither, a colour or
// a margin, asks for nothing on a page and is not reported; nor are
// scroll-position and contents, which are about scrolling and content that
// changes.

// willChangeNames reports whether a box's will-change names a property, which
// is given in lower case. Property names are ASCII case-insensitive, so the
// value's are compared as such.
func willChangeNames(b *Box, property string) bool {
	if b == nil || b.IsText() {
		return false
	}
	raw := b.Style.Get("will-change")
	if raw == "" || ascii.EqualFold(raw, "auto") {
		return false
	}
	for _, name := range splitCommaValues(raw) {
		if ascii.EqualFold(ascii.TrimCSSSpace(name), property) {
			return true
		}
	}
	return false
}

// willChangeEffects are the properties some non-initial value of which makes a
// box a stacking context or a containing block for positioned boxes, which
// will-change naming them asks for too, and which this engine does not make
// from will-change. filter is not here: it is done.
//
// Most do both, as a transform does (CSS Transforms, and Motion Path for
// offset-path); opacity, isolation, mix-blend-mode, clip-path and the mask
// properties make a stacking context (CSS Color 4, Compositing and Blending,
// CSS Masking); position makes a containing block for absolutely positioned
// boxes and, fixed, a stacking context (CSS 2.1 §10.1 and §9.9); z-index a
// stacking context where it applies; contain and content-visibility both,
// through layout and paint containment (CSS Containment); backdrop-filter both
// (Filter Effects 2); and view-transition-name a stacking context (CSS View
// Transitions). An entry here that is not quite right is reported where
// nothing was lost; one missing is a page drawn without a stacking context
// nobody was told about, which is the worse error.
var willChangeEffects = map[string]bool{
	"opacity": true, "transform": true, "translate": true, "rotate": true,
	"scale": true, "perspective": true, "transform-style": true,
	"offset-path": true, "backdrop-filter": true, "clip-path": true,
	"mask": true, "mask-image": true, "mask-border": true,
	"mask-border-source": true, "isolation": true, "mix-blend-mode": true,
	"position": true, "z-index": true, "contain": true,
	"content-visibility": true, "view-transition-name": true,
}

// reportWillChange reports the names in a box's will-change whose stacking
// context or containing block this engine does not make, once for each name in
// the document.
func reportWillChange(l *layouter, b *Box) {
	if b == nil || b.IsText() {
		return
	}
	raw := b.Style.Get("will-change")
	if raw == "" || ascii.EqualFold(raw, "auto") {
		return
	}
	for _, name := range splitCommaValues(raw) {
		name = ascii.Lower(ascii.TrimCSSSpace(name))
		if !willChangeEffects[name] {
			continue
		}
		l.reportOnce("will-change:"+name, Finding{
			Rule:   RuleUnsupportedValue,
			Source: AtHTML(offsetOf(b)),
			Message: "\"will-change: " + name + "\" makes a box the stacking context or " +
				"the containing block a value of " + name + " would, which this engine " +
				"does not make it",
			Path:     PathOf(b.Element),
			Property: "will-change",
		})
	}
}
