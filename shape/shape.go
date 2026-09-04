package shape

// Turning a string into positioned glyphs: ligature substitution, then pair
// kerning, then the character codes and displacements a PDF text operator takes.

// MeasureShaped is the width of a shaped string at the given size, in
// user-space units.
//
// It is what the text will occupy on the page: the same shaping Shape and
// DrawShaped do, measured rather than drawn. That is the whole contract, and it
// is the reason this measures by shaping rather than by a cheaper approximation
// of it. A layout engine measures a word to decide whether it fits the line and
// then draws it; if the two disagree the line is filled to one width and painted
// at another, and nothing in either call's own output shows it.
//
// This used to sum a flattened ligature table and a kerning map, which is most
// of shaping and not all of it — no contextual substitution, no syllabic
// reordering, no positioning beyond pair kerning. Over the HarfBuzz corpus that
// was wrong for 1920 of 5911 strings, by up to 17% on a Devanagari conjunct.
func (f *Face) MeasureShaped(s string, size float64) float64 {
	if !f.composite() {
		// Nothing is substituted or kerned for a face whose codes are
		// characters, so what it occupies is what Measure says — and asking the
		// shaped path would want a width by glyph index from a face that has no
		// font program to give one.
		return f.Measure(s, size)
	}
	glyphs, _ := f.ShapeGlyphs(s)
	return MeasureGlyphs(glyphs, size)
}

// MeasureShapedInContext is MeasureShaped with the text either side of the run.
//
// It exists because the two have to agree. A cursive letter's advance depends on
// the form it takes, and the form depends on its neighbours — a medial ain is
// not as wide as an isolated one — so a run measured alone and drawn in context
// is measured to one width and painted at another. See ShapeGlyphsInContext.
func (f *Face) MeasureShapedInContext(s string, size float64, before, after string,
	kerns bool, off Features) float64 {

	if !f.composite() {
		// A face whose codes are characters substitutes nothing and has no
		// positional forms, so its context cannot change an advance — and it
		// has no ligatures and no kerning to be turned off either, so the
		// features change nothing about it.
		return f.Measure(s, size)
	}
	var glyphs []Glyph
	if kerns {
		glyphs, _ = f.ShapeGlyphsInContext(s, before, after, off)
	} else {
		glyphs, _ = f.ShapeGlyphsAcrossFaces(s, before, after, off)
	}
	return MeasureGlyphs(glyphs, size)
}

// MeasureShapedMerged is MeasureShapedInContext where a neighbour may
// contribute glyphs to the run. See ShapeGlyphsMerged.
//
// It is the same call the painting goes through, which is the whole of why it
// exists: a run whose first characters were swallowed by its neighbour's
// ligature draws narrower than its text says, and a measure that did not agree
// would fill a line to one width and paint it at another.
func (f *Face) MeasureShapedMerged(s string, size float64,
	before, after, mergeBefore, mergeAfter string, kerns bool, off Features) float64 {

	head, through := f.MeasureShapedMergedSpan(s, size, before, after,
		mergeBefore, mergeAfter, kerns, off)
	return through - head
}

// MeasureShapedMergedSpan is MeasureShapedMerged as the pair of distances it is
// cut from: how far the pen has come when the run begins, and how far when it
// ends, both measured through the group the run was shaped with.
//
// The pair exists so that a caller rounding to a layout unit can round the two
// *ends* rather than the difference. Every run of a group then begins where the
// one before it ended, and the widths add up to the group's own rounded width
// instead of to a unit more or less of it — which is what a lam-alef written as
// two runs came out as, one sixty-fourth of a pixel wide of the same word
// written as one.
//
// head is zero for a run that merged nothing, which is every run of almost
// every document, and the difference is then the run's own width as it always
// was.
func (f *Face) MeasureShapedMergedSpan(s string, size float64,
	before, after, mergeBefore, mergeAfter string, kerns bool,
	off Features) (head, through float64) {

	if !f.composite() {
		return 0, f.Measure(s, size)
	}
	if mergeBefore == "" && mergeAfter == "" {
		glyphs, _ := f.ShapeGlyphsInContextOrAcross(s, before, after, kerns, off)
		return 0, MeasureGlyphs(glyphs, size)
	}
	whole, lo, hi := f.shapeWholeGroup(s, before, after, mergeBefore, mergeAfter, kerns, off)
	var headAdv, mine float64
	for _, g := range whole {
		switch {
		case g.Cluster < lo:
			headAdv += g.XAdvance
		case g.Cluster < hi:
			mine += g.XAdvance
		}
	}
	return headAdv * size / 1000, (headAdv + mine) * size / 1000
}

// ShapeGlyphsInContextOrAcross picks between the two by whether a pair spanning
// the boundary is this font's to apply. It is the choice every caller of the
// two made for itself.
func (f *Face) ShapeGlyphsInContextOrAcross(s, before, after string, kerns bool,
	off Features) ([]Glyph, int) {

	if kerns {
		return f.ShapeGlyphsInContext(s, before, after, off)
	}
	return f.ShapeGlyphsAcrossFaces(s, before, after, off)
}

// HasKerning reports whether the font carries pair kerning this package could
// read. A caller laying out text can use it to decide whether shaping is worth
// the extra spans, and a test can use it to notice a font whose kerning went
// unread.
func (f *Face) HasKerning() bool { return len(f.layout.kern) > 0 }

// HasLigatures reports whether the font carries ligature substitutions this
// package could read.
func (f *Face) HasLigatures() bool { return len(f.layout.ligatures) > 0 }

// Features lists the substitution features this face offers by name, sorted.
// A caller can present them, or check one before asking for it.
func (f *Face) Features() []string {
	out := make([]string, 0, len(f.layout.single))
	for tag := range f.layout.single {
		out = append(out, tag)
	}
	sortStrings(out)
	return out
}

func sortStrings(a []string) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
