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
	kerns bool) float64 {

	if !f.composite() {
		// A face whose codes are characters substitutes nothing and has no
		// positional forms, so its context cannot change an advance.
		return f.Measure(s, size)
	}
	var glyphs []Glyph
	if kerns {
		glyphs, _ = f.ShapeGlyphsInContext(s, before, after)
	} else {
		glyphs, _ = f.ShapeGlyphsAcrossFaces(s, before, after)
	}
	return MeasureGlyphs(glyphs, size)
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
